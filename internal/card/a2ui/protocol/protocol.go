// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package protocol

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	BundleVersion     = "1.0-daf4b7e7"
	CatalogResource   = "https://dingtalk.local/a2ui/a2ui-catalog.json"
	CommonResource    = "https://dingtalk.local/a2ui/a2ui-common-types.json"
	CatalogDigest     = "daf4b7e7585c28bae2c95ac477a835a6d68b719b1278c1cf408845d9f9cefb6c"
	CommonTypesDigest = "2a80d4d8968be55d95c23d48b5cee98bba441181144f46c04a1dd27dbb6f548f"
)

//go:embed assets/*.json
var assets embed.FS

type Catalog struct {
	Schema          string                     `json:"$schema"`
	CatalogID       string                     `json:"catalogId"`
	ProtocolVersion string                     `json:"protocolVersion"`
	Title           string                     `json:"title"`
	Description     string                     `json:"description"`
	Instructions    string                     `json:"instructions"`
	Components      map[string]json.RawMessage `json:"components"`
	Functions       map[string]json.RawMessage `json:"functions"`
	Defs            map[string]json.RawMessage `json:"$defs"`
}

type Diagnostic struct {
	Severity     string `json:"severity"`
	Code         string `json:"code"`
	Phase        string `json:"phase"`
	MessageIndex int    `json:"messageIndex"`
	InstancePath string `json:"instancePath,omitempty"`
	ComponentID  string `json:"componentId,omitempty"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
}

type Validation struct {
	Valid         bool         `json:"valid"`
	Mode          string       `json:"mode"`
	BundleVersion string       `json:"bundleVersion"`
	ManifestHash  string       `json:"manifestHash"`
	CatalogID     string       `json:"catalogId"`
	SurfaceID     string       `json:"surfaceId,omitempty"`
	MessageCount  int          `json:"messageCount"`
	Components    int          `json:"componentCount"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}

type Registry struct {
	catalog    Catalog
	catalogRaw []byte
	commonRaw  []byte
	compiler   *jsonschema.Compiler
	mu         sync.Mutex
}

func Load() (*Registry, error) {
	catalogRaw, err := assets.ReadFile("assets/a2ui-catalog.json")
	if err != nil {
		return nil, err
	}
	commonRaw, err := assets.ReadFile("assets/a2ui-common-types.json")
	if err != nil {
		return nil, err
	}
	if digest(catalogRaw) != CatalogDigest || digest(commonRaw) != CommonTypesDigest {
		return nil, fmt.Errorf("embedded A2UI bundle integrity check failed")
	}
	var catalog Catalog
	if err := json.Unmarshal(catalogRaw, &catalog); err != nil {
		return nil, err
	}
	var catalogDocument any
	if err := json.Unmarshal(catalogRaw, &catalogDocument); err != nil {
		return nil, err
	}
	var commonDocument any
	if err := json.Unmarshal(commonRaw, &commonDocument); err != nil {
		return nil, err
	}
	// The published schemas use ECMA-262 look-ahead expressions. Go's regexp
	// engine intentionally does not implement look-ahead, so omit only those
	// pattern assertions for the embedded validator. The message-level semantic
	// validator owns the corresponding DingTalk restrictions.
	dropUnsupportedPatterns(catalogDocument)
	dropUnsupportedPatterns(commonDocument)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(CommonResource, commonDocument); err != nil {
		return nil, err
	}
	if err := compiler.AddResource(CatalogResource, catalogDocument); err != nil {
		return nil, err
	}
	return &Registry{catalog: catalog, catalogRaw: catalogRaw, commonRaw: commonRaw, compiler: compiler}, nil
}

func dropUnsupportedPatterns(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if pattern, ok := typed["pattern"].(string); ok && (strings.Contains(pattern, "(?=") || strings.Contains(pattern, "(?!")) {
			delete(typed, "pattern")
		}
		if patternProperties, ok := typed["patternProperties"].(map[string]any); ok {
			for pattern, child := range patternProperties {
				if strings.Contains(pattern, "(?=") || strings.Contains(pattern, "(?!") {
					delete(patternProperties, pattern)
					continue
				}
				dropUnsupportedPatterns(child)
			}
		}
		for _, child := range typed {
			dropUnsupportedPatterns(child)
		}
	case []any:
		for _, child := range typed {
			dropUnsupportedPatterns(child)
		}
	}
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func ManifestHash() string {
	return digest([]byte(CatalogDigest + ":" + CommonTypesDigest + ":" + BundleVersion))
}

func (r *Registry) Catalog() Catalog { return r.catalog }

func (r *Registry) ComponentNames() []string {
	names := make([]string, 0, len(r.catalog.Components))
	for name := range r.catalog.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Component(name string) (json.RawMessage, bool) {
	raw, ok := r.catalog.Components[name]
	return append(json.RawMessage(nil), raw...), ok
}

func (r *Registry) CommonTypes() json.RawMessage { return append(json.RawMessage(nil), r.commonRaw...) }

func ParseMessages(reader io.Reader) ([]map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("A2UI input exceeds 4 MiB")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("A2UI input is empty")
	}
	if trimmed[0] == '[' {
		var objects []map[string]any
		if err := json.Unmarshal(trimmed, &objects); err == nil && len(objects) > 0 {
			return objects, nil
		}
		var wire []string
		if err := json.Unmarshal(trimmed, &wire); err != nil {
			return nil, fmt.Errorf("A2UI array must contain message objects or JSON strings: %w", err)
		}
		objects = make([]map[string]any, 0, len(wire))
		for i, item := range wire {
			var msg map[string]any
			if err := json.Unmarshal([]byte(item), &msg); err != nil {
				return nil, fmt.Errorf("wire message %d: %w", i, err)
			}
			objects = append(objects, msg)
		}
		if len(objects) == 0 {
			return nil, fmt.Errorf("A2UI input must contain at least one message")
		}
		return objects, nil
	}
	var messages []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(text), &msg); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("A2UI input must contain at least one message")
	}
	return messages, nil
}

func MarshalWire(messages []map[string]any) ([]string, error) {
	wire := make([]string, len(messages))
	for i, msg := range messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		wire[i] = string(raw)
	}
	return wire, nil
}

func (r *Registry) Validate(messages []map[string]any, mode string) Validation {
	result := Validation{Mode: mode, BundleVersion: BundleVersion, ManifestHash: ManifestHash(), CatalogID: r.catalog.CatalogID, MessageCount: len(messages), Diagnostics: []Diagnostic{}}
	seenSurface := ""
	created := false
	finalComponents := map[string]map[string]any{}
	var dataRoot any
	for i, msg := range messages {
		version, _ := msg["version"].(string)
		ops := []string{"createSurface", "updateComponents", "updateDataModel", "deleteSurface", "appendDataModel", "callFunction", "actionResponse"}
		var operation string
		for _, op := range ops {
			if _, ok := msg[op]; ok {
				if operation != "" {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_MESSAGE_MULTIPLE_OPERATIONS", "message", "/", "each message must contain exactly one operation", "split operations into separate messages"))
					operation = "!"
					break
				}
				operation = op
			}
		}
		if operation == "" {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_MESSAGE_OPERATION_MISSING", "message", "/", "message has no supported operation", "use createSurface, updateComponents, updateDataModel, appendDataModel, or deleteSurface"))
			continue
		}
		if operation == "!" {
			continue
		}
		if operation == "appendDataModel" {
			if version != "v0.9.1" {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_APPEND_VERSION", "message", "/version", "appendDataModel requires version v0.9.1", "use the DingTalk extension envelope"))
			}
		} else if version != "v1.0" {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_MESSAGE_VERSION", "message", "/version", "standard A2UI messages require version v1.0", "set version to v1.0"))
		}
		if operation == "callFunction" || operation == "actionResponse" {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_MESSAGE_NOT_ACCEPTED", "message", "/"+operation, operation+" is not accepted by the current DingTalk outbound profile", "update the data model or components instead"))
			continue
		}
		body, ok := msg[operation].(map[string]any)
		if !ok {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_MESSAGE_BODY_TYPE", "message", "/"+operation, "operation body must be an object", ""))
			continue
		}
		surface, _ := body["surfaceId"].(string)
		surface = strings.TrimSpace(surface)
		if surface == "" {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_SURFACE_REQUIRED", "message", "/"+operation+"/surfaceId", "surfaceId is required", ""))
		} else if seenSurface == "" {
			seenSurface = surface
		} else if surface != seenSurface {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_SURFACE_MISMATCH", "message", "/"+operation+"/surfaceId", "all messages in a batch must target one surface", ""))
		}
		if operation == "createSurface" {
			created = true
			catalogID, _ := body["catalogId"].(string)
			if strings.TrimSpace(catalogID) == "" {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_CATALOG_REQUIRED", "message", "/createSurface/catalogId", "catalogId is required for DingTalk surfaces", "use the bundle catalogId"))
			}
		}
		if operation == "appendDataModel" {
			path, _ := body["path"].(string)
			value, vok := body["value"].(string)
			if !strings.HasPrefix(path, "/") || path == "/" || path == "/card/flowStatus" {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_APPEND_PATH", "binding", "/appendDataModel/path", "append path must be a non-root JSON Pointer outside /card/flowStatus", ""))
			}
			if !vok || value == "" {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_APPEND_VALUE", "binding", "/appendDataModel/value", "append value must be a non-empty string", ""))
			}
		}
		if operation == "updateDataModel" {
			if _, ok := body["value"]; !ok || body["value"] == nil {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_DATA_VALUE_REQUIRED", "binding", "/updateDataModel/value", "updateDataModel value must be present and non-null", ""))
			}
			if path, _ := body["path"].(string); path == "/" {
				dataRoot = body["value"]
			}
		}
		rawComponents, componentsExist := body["components"]
		if operation == "updateComponents" && !componentsExist {
			result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENTS_REQUIRED", "component", "/updateComponents/components", "updateComponents.components is required", "provide a non-empty array of complete component definitions"))
			continue
		}
		if componentsExist {
			components, ok := rawComponents.([]any)
			if !ok {
				path := fmt.Sprintf("/%s/components", operation)
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENTS_TYPE", "component", path, operation+".components must be an array", "provide a non-empty array of complete component definitions"))
				continue
			}
			if operation == "updateComponents" && len(components) == 0 {
				result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENTS_EMPTY", "component", "/updateComponents/components", "updateComponents.components must not be empty", "omit the operation when no component changed"))
				continue
			}
			componentIDs := map[string]bool{}
			for j, item := range components {
				comp, ok := item.(map[string]any)
				if !ok {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_TYPE", "component", fmt.Sprintf("/%s/components/%d", operation, j), "component must be an object", ""))
					continue
				}
				id, _ := comp["id"].(string)
				name, _ := comp["component"].(string)
				if id == "" {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_ID_REQUIRED", "component", fmt.Sprintf("/%s/components/%d/id", operation, j), "component id is required", ""))
				} else if componentIDs[id] {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_DUPLICATE", "component", fmt.Sprintf("/%s/components/%d/id", operation, j), "duplicate component id in one component operation", ""))
				} else {
					componentIDs[id] = true
					finalComponents[id] = comp
				}
				if name == "RadioButton" {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_UNKNOWN", "component", fmt.Sprintf("/%s/components/%d/component", operation, j), "RadioButton is not in the public catalog", "use ChoicePicker with mutuallyExclusive"))
					continue
				}
				if _, ok := r.catalog.Components[name]; !ok {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_UNKNOWN", "component", fmt.Sprintf("/%s/components/%d/component", operation, j), "component is not in the public catalog", "query dws card catalog get"))
					continue
				}
				if err := r.validateComponent(name, comp); err != nil {
					result.Diagnostics = append(result.Diagnostics, diag(i, "A2UI_COMPONENT_SCHEMA", "component", fmt.Sprintf("/%s/components/%d", operation, j), err.Error(), "query the component contract"))
				}
				result.Diagnostics = append(result.Diagnostics, validateDingTalkExtensions(i, fmt.Sprintf("/%s/components/%d", operation, j), comp)...)
				result.Components++
			}
		}
	}
	if mode == "create" && !created {
		result.Diagnostics = append(result.Diagnostics, diag(0, "A2UI_CREATE_REQUIRED", "message", "/", "create mode requires createSurface", ""))
	}
	if mode == "create" && created {
		result.Diagnostics = append(result.Diagnostics, validateGraph(finalComponents)...)
		result.Diagnostics = append(result.Diagnostics, validateBindings(finalComponents, dataRoot)...)
	}
	if mode == "update" && created {
		result.Diagnostics = append(result.Diagnostics, diag(0, "A2UI_UPDATE_CREATE_FORBIDDEN", "message", "/createSurface", "update mode must not create a surface", ""))
	}
	result.SurfaceID = seenSurface
	result.Valid = true
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == "error" {
			result.Valid = false
			break
		}
	}
	return result
}

func validateDingTalkExtensions(messageIndex int, basePath string, value any) []Diagnostic {
	var diagnostics []Diagnostic
	var walk func(any, string, bool)
	walk = func(current any, path string, actionBindings bool) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				childPath := path + "/" + escapeJSONPointerToken(key)
				if strings.HasPrefix(key, "dt_actionBindings") && key != "dt_actionBindingsV1" {
					diagnostics = append(diagnostics, diag(messageIndex, "A2UI_ACTION_BINDINGS_VERSION", "component", childPath, "unsupported DingTalk action binding key "+key, "use dt_actionBindingsV1"))
				}
				nextBindings := actionBindings || key == "dt_actionBindingsV1"
				if nextBindings && key == "resultPath" {
					resultPath, ok := child.(string)
					if !ok || !validHostResultPath(resultPath) {
						diagnostics = append(diagnostics, diag(messageIndex, "A2UI_HOST_RESULT_PATH", "component", childPath, "resultPath must be an ASCII JSON Pointer below /ui with 2-16 non-numeric segments", "use a path such as /ui/action/result"))
					}
				}
				walk(child, childPath, nextBindings)
			}
		case []any:
			for index, child := range typed {
				walk(child, fmt.Sprintf("%s/%d", path, index), actionBindings)
			}
		}
	}
	walk(value, basePath, false)
	return diagnostics
}

func validHostResultPath(path string) bool {
	if len(path) == 0 || len(path) > 256 || !strings.HasPrefix(path, "/ui/") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) < 2 || len(segments) > 16 || segments[0] != "ui" {
		return false
	}
	for _, segment := range segments[1:] {
		if segment == "" {
			return false
		}
		hasNonDigit := false
		for _, char := range segment {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || char == '-' {
				hasNonDigit = true
				continue
			}
			if char < '0' || char > '9' {
				return false
			}
		}
		if !hasNonDigit {
			return false
		}
	}
	return true
}

func escapeJSONPointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func validateBindings(components map[string]map[string]any, dataRoot any) []Diagnostic {
	var diagnostics []Diagnostic
	primaryButtons := 0
	for id, component := range components {
		if component["component"] == "Button" && component["variant"] == "primary" {
			primaryButtons++
		}
		var paths []string
		collectPaths(component, &paths)
		for _, path := range paths {
			if _, ok := pointerValue(dataRoot, path); !ok {
				diagnostic := diag(0, "A2UI_BINDING_PATH_MISSING", "binding", "/components/"+id, "binding path "+path+" is missing from the initial DataModel", "initialize the path before rendering")
				diagnostic.ComponentID = id
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	if primaryButtons > 1 {
		diagnostic := diag(0, "A2UI_MULTIPLE_PRIMARY_ACTIONS", "design", "/components", "card contains more than one primary Button", "keep one primary action and render alternatives as default or borderless")
		diagnostic.Severity = "warning"
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

func collectPaths(value any, paths *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if path, ok := typed["path"].(string); ok && strings.HasPrefix(path, "/") {
			*paths = append(*paths, path)
		}
		for key, child := range typed {
			if key != "path" {
				collectPaths(child, paths)
			}
		}
	case []any:
		for _, child := range typed {
			collectPaths(child, paths)
		}
	case []map[string]any:
		for _, child := range typed {
			collectPaths(child, paths)
		}
	}
}

func pointerValue(root any, pointer string) (any, bool) {
	if pointer == "/" {
		return root, root != nil
	}
	current := root
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func validateGraph(components map[string]map[string]any) []Diagnostic {
	if len(components) == 0 {
		return []Diagnostic{diag(0, "A2UI_COMPONENTS_REQUIRED", "component", "/", "create mode must define components", "add updateComponents with a root component")}
	}
	if _, ok := components["root"]; !ok {
		return []Diagnostic{diag(0, "A2UI_ROOT_REQUIRED", "component", "/components", "component id root is required", "make the outer Card or Column id root")}
	}
	graph := map[string][]string{}
	var diagnostics []Diagnostic
	for id, component := range components {
		refs := componentRefs(component)
		graph[id] = refs
		for _, ref := range refs {
			if _, ok := components[ref]; !ok {
				diagnostic := diag(0, "A2UI_COMPONENT_REF_MISSING", "component", "/components/"+id, "component references missing child "+ref, "define the child or remove the reference")
				diagnostic.ComponentID = id
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	visiting, visited, reachable := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		if visiting[id] {
			diagnostic := diag(0, "A2UI_COMPONENT_CYCLE", "component", "/components/"+id, "component graph contains a cycle", "remove the cyclic child reference")
			diagnostic.ComponentID = id
			diagnostics = append(diagnostics, diagnostic)
			return
		}
		if visited[id] {
			return
		}
		visiting[id], reachable[id] = true, true
		for _, child := range graph[id] {
			if _, ok := components[child]; ok {
				walk(child)
			}
		}
		visiting[id], visited[id] = false, true
	}
	walk("root")
	for id := range components {
		if !reachable[id] {
			diagnostic := diag(0, "A2UI_COMPONENT_UNREACHABLE", "design", "/components/"+id, "component is not reachable from root", "attach it to the component tree or remove it")
			diagnostic.Severity = "warning"
			diagnostic.ComponentID = id
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return diagnostics
}

func componentRefs(component map[string]any) []string {
	var refs []string
	if child, ok := component["child"].(string); ok && child != "" {
		refs = append(refs, child)
	}
	if trailing, ok := component["trailing"].(string); ok && trailing != "" {
		refs = append(refs, trailing)
	}
	switch children := component["children"].(type) {
	case []any:
		for _, child := range children {
			if id, ok := child.(string); ok && id != "" {
				refs = append(refs, id)
			}
		}
	case []string:
		refs = append(refs, children...)
	}
	return refs
}

// ValidateDeliveryResources applies transport constraints that are stricter
// than the portable A2UI schema. DingTalk clients fetch Image resources from
// HTTPS URLs; data/file/blob URLs may work in the reference preview but must
// fail before a real send or update.
func ValidateDeliveryResources(messages []map[string]any) error {
	for messageIndex, message := range messages {
		body, ok := message["updateComponents"].(map[string]any)
		if !ok {
			continue
		}
		components, _ := body["components"].([]any)
		for componentIndex, raw := range components {
			component, _ := raw.(map[string]any)
			if component["component"] != "Image" {
				continue
			}
			resource, _ := component["url"].(string)
			parsed, err := url.Parse(resource)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				id, _ := component["id"].(string)
				return fmt.Errorf("message %d component %d Image %q uses a preview-only or unsafe URL; real delivery requires an absolute HTTPS URL", messageIndex, componentIndex, id)
			}
		}
	}
	return nil
}

func (r *Registry) validateComponent(name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	schema, err := r.compiler.Compile(CatalogResource + "#/components/" + name)
	if err != nil {
		return err
	}
	return schema.Validate(document)
}

func diag(index int, code, phase, path, message, suggestion string) Diagnostic {
	return Diagnostic{Severity: "error", Code: code, Phase: phase, MessageIndex: index, InstancePath: path, Message: message, Suggestion: suggestion}
}
