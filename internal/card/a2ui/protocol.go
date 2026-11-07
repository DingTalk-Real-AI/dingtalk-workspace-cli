// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

// Package a2ui provides offline structural validation using the bundled protocol.
package a2ui

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed assets.json
var assetsJSON []byte

type assets struct {
	ExplainSHA256   string            `json:"explainSha256"`
	Manifest        map[string]string `json:"manifest"`
	ChildRefs       map[string][]any  `json:"childRefs"`
	UnicodeClasses  map[string]string `json:"unicodeClasses"`
	ValidationRules string            `json:"validationRules"`
}

type Protocol struct {
	docs   map[string]map[string]any
	assets assets
	schema *jsonschema.Schema
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Pointer  string `json:"pointer"`
	Message  string `json:"message"`
	Hint     string `json:"hint"`
}

type Report struct {
	Preflight         map[string]any `json:"preflight,omitempty"`
	Valid             bool           `json:"valid"`
	Diagnostics       []Diagnostic   `json:"diagnostics"`
	Metrics           map[string]any `json:"metrics"`
	RenderingVerified bool           `json:"renderingVerified"`
	A2UIMessages      []string       `json:"a2uiMessages,omitempty"`
}

func decode(data []byte) (any, error) {
	// Tolerate a leading UTF-8 BOM; embedded protocol resources remain hash-checked.
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("JSON file must use valid UTF-8 encoding")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("file must contain exactly one JSON value")
	}
	return v, nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func str(v any) string            { s, _ := v.(string); return s }
func pointer(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteByte('/')
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(p, "~", "~0"), "/", "~1"))
	}
	return b.String()
}

type localLoader struct{}

func (localLoader) Load(location string) (any, error) {
	return nil, fmt.Errorf("protocol reference is outside the embedded package: %s", location)
}

type protocolRegexp struct {
	source   string
	compiled *regexp2.Regexp
}

func (r protocolRegexp) String() string { return r.source }
func (r protocolRegexp) MatchString(s string) bool {
	ok, err := r.compiled.MatchString(s)
	if err != nil {
		panic(err)
	} // Never turn an engine failure into a protocol verdict.
	return ok
}

// Load requires only the embedded Skill filesystem, never an installed Skill or Python.
func Load(source fs.FS) (*Protocol, error) {
	return load(source, assetsJSON)
}

func load(source fs.FS, assetData []byte) (*Protocol, error) {
	p := &Protocol{docs: map[string]map[string]any{}}
	if err := json.Unmarshal(assetData, &p.assets); err != nil {
		return nil, err
	}
	store, err := BundledExplain()
	if err != nil {
		return nil, err
	}
	var rules map[string]any
	for name, want := range p.assets.Manifest {
		var data []byte
		var err error
		if name == "a2ui-validation-rules.json" {
			data = []byte(p.assets.ValidationRules)
		} else {
			data, err = fs.ReadFile(source, "references/protocol/"+name)
		}
		if err != nil {
			return nil, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data))[:16] != want {
			return nil, fmt.Errorf("protocol and generated index have drifted: %s", name)
		}
		v, err := decode(data)
		if err != nil {
			return nil, err
		}
		if name == "a2ui-validation-rules.json" {
			rules = object(v)
		} else {
			p.docs[name] = object(v)
		}
	}
	if err := p.applyRules(rules); err != nil {
		return nil, err
	}
	for name, doc := range p.docs {
		p.docs[name] = object(guardTags(doc))
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(localLoader{})
	c.UseRegexpEngine(func(expression string) (jsonschema.Regexp, error) {
		expanded := expression
		for name, ranges := range p.assets.UnicodeClasses {
			expanded = strings.ReplaceAll(expanded, "\\p{"+name+"}", ranges)
		}
		re, err := regexp2.Compile(expanded, 0)
		if err != nil {
			return nil, err
		}
		return protocolRegexp{expression, re}, nil
	})
	for _, doc := range p.docs {
		if err := c.AddResource(str(doc["$id"]), doc); err != nil {
			return nil, err
		}
	}
	p.schema, err = c.Compile(str(p.docs["agent-to-renderer.json"]["$id"]))
	if err != nil {
		return nil, err
	}
	// Compile even references not reachable from the envelope. A stale fragment is a package error.
	for _, doc := range p.docs {
		// AddResource already parsed and accepted every document ID above.
		base, _ := url.Parse(str(doc["$id"]))
		var walk func(any) error
		walk = func(v any) error {
			switch n := v.(type) {
			case map[string]any:
				if ref, ok := n["$ref"].(string); ok {
					u, err := url.Parse(ref)
					if err != nil {
						return err
					}
					if _, err = c.Compile(base.ResolveReference(u).String()); err != nil {
						return err
					}
				}
				for _, child := range n {
					if err := walk(child); err != nil {
						return err
					}
				}
			case []any:
				for _, child := range n {
					if err := walk(child); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(doc); err != nil {
			return nil, err
		}
	}
	// Preserve specific resource/Schema diagnostics before reporting cross-bundle drift.
	if err := store.CheckManifest(p.assets.Manifest); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Protocol) applyRules(rules map[string]any) error {
	version, ok := rules["version"].(json.Number)
	if !ok || version != json.Number("1") {
		return fmt.Errorf("unsupported supplementary validation-rule version")
	}
	for file, definitions := range object(rules["definitions"]) {
		defs := object(p.docs[file]["$defs"])
		if defs == nil {
			return fmt.Errorf("supplementary rule target does not exist: %s", file)
		}
		for name, definition := range object(definitions) {
			if _, exists := defs[name]; exists {
				return fmt.Errorf("supplementary rule conflicts with public definition: %s", name)
			}
			defs[name] = definition
		}
	}
	for _, entry := range array(rules["references"]) {
		r := object(entry)
		var node any = p.docs[str(r["file"])]
		for _, segment := range array(r["path"]) {
			switch part := segment.(type) {
			case string:
				node = object(node)[part]
			case json.Number:
				i, err := part.Int64()
				a := array(node)
				if err != nil || i < 0 || i >= int64(len(a)) {
					return fmt.Errorf("supplementary rule array path mismatch")
				}
				node = a[i]
			default:
				return fmt.Errorf("malformed supplementary rule path")
			}
		}
		m := object(node)
		if m == nil || m["$ref"] != r["publicRef"] {
			return fmt.Errorf("supplementary rule reference mismatch: %v", r["path"])
		}
		m["$ref"] = r["validationRef"]
	}
	return nil
}

// A mismatched constant makes a branch false regardless of recursive args.
// The guard preserves successful branch annotations and full validation for absent tags.
func guardTags(value any) any {
	switch n := value.(type) {
	case []any:
		for i, v := range n {
			n[i] = guardTags(v)
		}
	case map[string]any:
		for key, v := range n {
			n[key] = guardTags(v)
		}
		for _, tag := range []string{"component", "call"} {
			prop := object(object(n["properties"])[tag])
			if _, ok := prop["const"].(string); ok {
				guard := map[string]any{"properties": map[string]any{tag: prop}}
				return map[string]any{"if": guard, "then": n, "else": guard}
			}
		}
	}
	return value
}

func (p *Protocol) Manifest() map[string]string {
	out := map[string]string{}
	for k, v := range p.assets.Manifest {
		out[k] = v
	}
	return out
}

// CheckExplainAssets verifies every named definition for the full package self-check.
func (p *Protocol) CheckExplainAssets() error {
	store, err := BundledExplain()
	if err != nil {
		return err
	}
	return store.CheckAll(p.assets.Manifest, p.assets.ExplainSHA256)
}

// Explain returns loading or decoding errors to the caller without panicking.
func (p *Protocol) Explain(name string) (map[string]any, error) {
	store, err := BundledExplain()
	if err != nil {
		return nil, err
	}
	return store.Lookup(name)
}

func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	prev := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, c := range x {
		next := make([]int, len(y)+1)
		next[0] = i + 1
		for j, d := range y {
			cost := 1
			if c == d {
				cost = 0
			}
			next[j+1] = min(next[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = next
	}
	return prev[len(y)]
}

func (p *Protocol) Lint(data []byte, fragment, emit bool) (Report, error) {
	report := Report{Diagnostics: []Diagnostic{}, Metrics: map[string]any{}}
	v, err := decode(data)
	if err != nil {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{"input.invalid_json", "error", "", err.Error(), "Correct the JSON syntax"})
		return report, nil
	}
	var prefix string
	if fragment {
		if m, ok := v.(map[string]any); ok {
			if _, ok := m["component"]; ok {
				v = []any{map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "fragment", "components": []any{m}}}}
				prefix = "/0/updateComponents/components/0"
			} else {
				v = []any{m}
				prefix = "/0"
			}
		} else if a := array(v); len(a) > 0 {
			all := true
			for _, c := range a {
				if _, ok := object(c)["component"]; !ok {
					all = false
				}
			}
			if all {
				v = []any{map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "fragment", "components": a}}}
				prefix = "/0/updateComponents/components"
			}
		}
	}
	messages := array(v)
	if len(messages) == 0 {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{"input.not_array", "error", "", "Input must be a nonempty A2UI message array", ""})
		return report, nil
	}
	for i, message := range messages {
		if err := p.schema.Validate(message); err != nil {
			// jsonschema.Schema.Validate wraps every failure in ValidationError.
			report.Diagnostics = append(report.Diagnostics, p.diagnostics(err.(*jsonschema.ValidationError), message, fmt.Sprintf("/%d", i))...)
		}
	}
	if prefix != "" {
		for i := range report.Diagnostics {
			d := &report.Diagnostics[i]
			if d.Pointer == prefix || strings.HasPrefix(d.Pointer, prefix+"/") {
				d.Pointer = strings.TrimPrefix(d.Pointer, prefix)
			} else {
				d.Pointer = ""
			}
		}
	}
	report.Valid = len(report.Diagnostics) == 0
	if report.Valid && emit {
		for _, m := range messages {
			// All messages came from decode, so their values are JSON encodable.
			b, _ := json.Marshal(m)
			report.A2UIMessages = append(report.A2UIMessages, string(b))
		}
	}
	return report, nil
}

// Bundled keeps the compatibility entry point without caching an arbitrary
// filesystem; the caller that owns a known immutable embed may cache Load.
func Bundled(source fs.FS) (*Protocol, error) {
	return Load(source)
}
