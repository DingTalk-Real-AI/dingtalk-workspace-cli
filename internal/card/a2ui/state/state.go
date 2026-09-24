// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Surface struct {
	Version    string                    `json:"version"`
	SurfaceID  string                    `json:"surfaceId"`
	CatalogID  string                    `json:"catalogId"`
	Components map[string]map[string]any `json:"components"`
	Data       any                       `json:"dataModel"`
}

type Snapshot struct {
	Version    string                    `json:"version"`
	SurfaceID  string                    `json:"surfaceId"`
	CatalogID  string                    `json:"catalogId,omitempty"`
	Components map[string]map[string]any `json:"components"`
	DataModel  any                       `json:"dataModel"`
}

func ParseSnapshot(reader io.Reader) (Surface, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 4<<20+1))
	if err != nil {
		return Surface{}, err
	}
	if len(raw) > 4<<20 {
		return Surface{}, fmt.Errorf("A2UI snapshot exceeds 4 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Surface{}, fmt.Errorf("invalid A2UI snapshot: %w", err)
	}
	if err := ensureSnapshotEOF(decoder); err != nil {
		return Surface{}, err
	}
	if snapshot.Version != "v1.0" {
		return Surface{}, fmt.Errorf("snapshot version must be v1.0")
	}
	if strings.TrimSpace(snapshot.SurfaceID) == "" {
		return Surface{}, fmt.Errorf("snapshot surfaceId is required")
	}
	if len(snapshot.Components) == 0 {
		return Surface{}, fmt.Errorf("snapshot components must be a non-empty object")
	}
	if snapshot.DataModel == nil {
		return Surface{}, fmt.Errorf("snapshot dataModel is required")
	}
	if _, ok := snapshot.DataModel.(map[string]any); !ok {
		return Surface{}, fmt.Errorf("snapshot dataModel must be an object")
	}
	return Surface{Version: snapshot.Version, SurfaceID: snapshot.SurfaceID, CatalogID: snapshot.CatalogID, Components: snapshot.Components, Data: snapshot.DataModel}, nil
}

func ensureSnapshotEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid A2UI snapshot: %w", err)
	}
	return fmt.Errorf("invalid A2UI snapshot: multiple JSON values")
}

func Reduce(base Surface, messages []map[string]any) (Surface, error) {
	base = cloneSurface(base)
	// Surface.Version is the stable snapshot envelope contract, not the
	// per-operation wire version (appendDataModel may use v0.9.1).
	if base.Version == "" {
		base.Version = "v1.0"
	}
	if base.Components == nil {
		base.Components = map[string]map[string]any{}
	}
	for i, message := range messages {
		operation, body, err := operation(message)
		if err != nil {
			return Surface{}, fmt.Errorf("message %d: %w", i, err)
		}
		surfaceID, _ := body["surfaceId"].(string)
		if base.SurfaceID != "" && surfaceID != base.SurfaceID {
			return Surface{}, fmt.Errorf("message %d targets surface %q, expected %q", i, surfaceID, base.SurfaceID)
		}
		if surfaceID != "" {
			base.SurfaceID = surfaceID
		}
		switch operation {
		case "createSurface":
			base.CatalogID, _ = body["catalogId"].(string)
		case "updateComponents":
			components, _ := body["components"].([]any)
			for _, item := range components {
				component, ok := item.(map[string]any)
				if !ok {
					continue
				}
				id, _ := component["id"].(string)
				if id != "" {
					base.Components[id] = cloneMap(component)
				}
			}
		case "updateDataModel":
			path, _ := body["path"].(string)
			base.Data, err = setPointer(base.Data, path, body["value"])
			if err != nil {
				return Surface{}, fmt.Errorf("message %d: %w", i, err)
			}
		case "appendDataModel":
			path, _ := body["path"].(string)
			value, _ := body["value"].(string)
			current, ok := getPointer(base.Data, path).(string)
			if !ok {
				return Surface{}, fmt.Errorf("message %d: append target %s is not a string", i, path)
			}
			base.Data, err = setPointer(base.Data, path, current+value)
			if err != nil {
				return Surface{}, fmt.Errorf("message %d: %w", i, err)
			}
		case "deleteSurface":
			return Surface{}, nil
		}
	}
	return base, nil
}

func cloneSurface(value Surface) Surface {
	raw, _ := json.Marshal(value)
	var out Surface
	_ = json.Unmarshal(raw, &out)
	return out
}

func Diff(previous, desired Surface) ([]map[string]any, error) {
	if previous.SurfaceID == "" || desired.SurfaceID != previous.SurfaceID {
		return nil, fmt.Errorf("snapshot diff requires the same non-empty surfaceId")
	}
	if desired.CatalogID != "" && previous.CatalogID != "" && desired.CatalogID != previous.CatalogID {
		return nil, fmt.Errorf("snapshot diff requires the same catalogId")
	}
	for id := range previous.Components {
		if _, ok := desired.Components[id]; !ok {
			return nil, fmt.Errorf("snapshot removal of component %q is unsupported; send an explicit supported update instead", id)
		}
	}
	var messages []map[string]any
	changed := make([]any, 0)
	ids := make([]string, 0, len(desired.Components))
	for id := range desired.Components {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		component := desired.Components[id]
		if !equalJSON(component, previous.Components[id]) {
			changed = append(changed, component)
		}
	}
	if len(changed) > 0 {
		messages = append(messages, map[string]any{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": desired.SurfaceID, "components": changed}})
	}
	protected := inputBindingPaths(previous.Components)
	for path := range inputBindingPaths(desired.Components) {
		protected[path] = true
	}
	updates, err := diffDataModel(previous.Data, desired.Data, "", protected)
	if err != nil {
		return nil, err
	}
	for _, update := range updates {
		messages = append(messages, map[string]any{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": desired.SurfaceID, "path": update.path, "value": update.value}})
	}
	return messages, nil
}

type dataUpdate struct {
	path  string
	value any
}

func diffDataModel(previous, desired any, path string, protected map[string]bool) ([]dataUpdate, error) {
	desiredObject, desiredIsObject := desired.(map[string]any)
	if desiredIsObject {
		previousObject, _ := previous.(map[string]any)
		keys := make([]string, 0, len(desiredObject))
		for key := range desiredObject {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var updates []dataUpdate
		for _, key := range keys {
			childPath := path + "/" + escapeToken(key)
			child, err := diffDataModel(previousObject[key], desiredObject[key], childPath, protected)
			if err != nil {
				return nil, err
			}
			updates = append(updates, child...)
		}
		return updates, nil
	}
	if path == "" {
		return nil, fmt.Errorf("snapshot dataModel must be an object")
	}
	if overlapsProtectedPath(path, protected) {
		return nil, nil
	}
	if equalJSON(previous, desired) {
		return nil, nil
	}
	return []dataUpdate{{path: path, value: desired}}, nil
}

func inputBindingPaths(components map[string]map[string]any) map[string]bool {
	paths := map[string]bool{}
	for _, component := range components {
		name, _ := component["component"].(string)
		if !inputComponentNames[name] {
			continue
		}
		binding, _ := component["value"].(map[string]any)
		path, _ := binding["path"].(string)
		if strings.HasPrefix(path, "/") && path != "/" {
			paths[path] = true
		}
	}
	return paths
}

var inputComponentNames = map[string]bool{
	"CheckBox":           true,
	"CheckableImageList": true,
	"CheckboxListMulti":  true,
	"ChoicePicker":       true,
	"ConversationPicker": true,
	"DateTimeInput":      true,
	"ImageUpload":        true,
	"InputList":          true,
	"NumberInput":        true,
	"Rating":             true,
	"Slider":             true,
	"Switch":             true,
	"TextField":          true,
	"UserPicker":         true,
}

func overlapsProtectedPath(path string, protected map[string]bool) bool {
	for protectedPath := range protected {
		if path == protectedPath || strings.HasPrefix(path, protectedPath+"/") || strings.HasPrefix(protectedPath, path+"/") {
			return true
		}
	}
	return false
}

func escapeToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func operation(message map[string]any) (string, map[string]any, error) {
	for _, name := range []string{"createSurface", "updateComponents", "updateDataModel", "appendDataModel", "deleteSurface"} {
		if raw, ok := message[name]; ok {
			body, ok := raw.(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("%s body must be an object", name)
			}
			return name, body, nil
		}
	}
	return "", nil, fmt.Errorf("unsupported operation")
}

func cloneMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
func equalJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func tokens(pointer string) ([]string, error) {
	if pointer == "/" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("invalid JSON Pointer %q", pointer)
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func getPointer(root any, pointer string) any {
	parts, err := tokens(pointer)
	if err != nil {
		return nil
	}
	current := root
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}
	return current
}

func setPointer(root any, pointer string, value any) (any, error) {
	parts, err := tokens(pointer)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return value, nil
	}
	if root == nil {
		root = map[string]any{}
	}
	return setPointerParts(root, parts, value)
}

func setPointerParts(current any, parts []string, value any) (any, error) {
	if len(parts) == 0 {
		return value, nil
	}
	token := parts[0]
	switch typed := current.(type) {
	case map[string]any:
		child, exists := typed[token]
		if !exists {
			if len(parts) > 1 {
				if _, err := strconv.Atoi(parts[1]); err == nil {
					return nil, fmt.Errorf("pointer cannot create missing array at %q", token)
				}
			}
			child = map[string]any{}
		}
		updated, err := setPointerParts(child, parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[token] = updated
		return typed, nil
	case []any:
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || index >= len(typed) {
			return nil, fmt.Errorf("pointer array index %q is out of bounds", token)
		}
		updated, err := setPointerParts(typed[index], parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[index] = updated
		return typed, nil
	default:
		return nil, fmt.Errorf("pointer parent %q is neither an object nor an array", token)
	}
}
