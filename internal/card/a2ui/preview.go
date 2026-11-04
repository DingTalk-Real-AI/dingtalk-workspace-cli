// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// resourceLeadingSpace matches the characters the standalone Skill skips before
// a resource URL scheme: C0 controls, space, and Unicode whitespace.
func resourceLeadingSpace(r rune) bool {
	return r <= ' ' || unicode.IsSpace(r)
}

// Preflight is an explicit offline delivery/resource check, separate from Schema validity.
func (p *Protocol) Preflight(messages []string, mode string) (map[string]any, error) {
	if mode != "new-card" && mode != "resources" {
		return nil, fmt.Errorf("unknown preflight mode %s", mode)
	}
	diagnostics := []Diagnostic{}
	add := func(code, ptr, msg string) {
		diagnostics = append(diagnostics, Diagnostic{code, "error", ptr, msg, ""})
	}
	type state struct {
		data, components bool
		refs             referenceSnapshot
	}
	states := map[string]*state{}
	docs := []map[string]any{}
	for i, raw := range messages {
		v, err := decode([]byte(raw))
		if err != nil {
			return nil, err
		}
		m := object(v)
		docs = append(docs, m)
		if mode != "new-card" {
			continue
		}
		op := ""
		for _, k := range []string{"createSurface", "updateDataModel", "updateComponents", "deleteSurface"} {
			if _, ok := m[k]; ok {
				op = k
				break
			}
		}
		body := object(m[op])
		sid := str(body["surfaceId"])
		ptr := fmt.Sprintf("/%d/%s", i, op)
		switch op {
		case "createSurface":
			if states[sid] != nil {
				add("delivery.surface_duplicated", ptr, "A new-card snapshot cannot create the same Surface twice")
			}
			if body["catalogId"] != p.docs["catalog-components-common.json"]["catalogId"] {
				add("delivery.catalog_id", ptr+"/catalogId", "A new card must explicitly use the bundled catalogId")
			}
			_, hasData := body["dataModel"].(map[string]any)
			_, hasComponents := body["components"].([]any)
			states[sid] = &state{data: hasData, components: hasComponents,
				refs: referenceSnapshot{map[string]referenceNode{}, ptr + "/surfaceId"}}
			diagnostics = append(diagnostics, states[sid].refs.update(body, ptr)...)
		case "updateDataModel", "updateComponents":
			if op == "updateDataModel" && !strings.HasPrefix(str(body["path"]), "/") {
				add("delivery.data_model_path", ptr+"/path", "The current DingTalk creation route requires an explicit data path beginning with /")
			}
			s := states[sid]
			if s == nil {
				add("delivery.surface_not_created", ptr+"/surfaceId", "Create the Surface before updating it in a new-card snapshot")
				continue
			}
			if op == "updateComponents" {
				s.components = true
				diagnostics = append(diagnostics, s.refs.update(body, ptr)...)
			} else {
				_, value := body["value"]
				if body["path"] == "/" {
					s.data = value
				} else if value {
					s.data = true
				}
			}
		default:
			add("delivery.unsupported_create_operation", ptr, "The current new-card route rejects this operation; send lifecycle deltas separately")
		}
	}
	if mode == "new-card" {
		if len(states) == 0 {
			add("delivery.create_surface_missing", "", "The new-card file is missing createSurface")
		}
		ids := []string{}
		for id := range states {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, sid := range ids {
			s := states[sid]
			if !s.data {
				add("delivery.data_model_missing", "", fmt.Sprintf("Surface %s lacks data initialization; a static new card may set root path / to an empty object", sid))
			}
			if !s.components {
				add("delivery.components_missing", "", fmt.Sprintf("Surface %s is missing updateComponents", sid))
			} else {
				diagnostics = append(diagnostics, p.checkReferences(s.refs, sid)...)
			}
		}
	}
	resourceKeys := map[string]bool{"url": true, "darkUrl": true, "imageUrl": true, "posterUrl": true, "coverUrl": true, "images": true, "icon": true, "darkIcon": true}
	var walk func(any, string, string)
	walk = func(node any, ptr, field string) {
		switch n := node.(type) {
		case map[string]any:
			markdownFields := []string{}
			if n["component"] == "Markdown" {
				markdownFields = append(markdownFields, "content")
			}
			if _, ok := n["component"].(string); ok {
				markdownFields = append(markdownFields, "fallbackMarkdown")
			}
			for _, key := range markdownFields {
				if content, ok := n[key].(string); ok && strings.Contains(strings.ToLower(content), "data:") {
					diagnostics = append(diagnostics, Diagnostic{"resource.markdown_data_unverified", "warning", ptr + "/" + key, "Markdown contains data: text; review image destinations before delivery. This check does not parse Markdown or verify resource use", ""})
				}
			}
			keys := []string{}
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				// Table rows and chart data/config are business values, not component definitions.
				if k == "data" && (n["component"] == "Table" || n["component"] == "Chart") {
					continue
				}
				if k == "metadata" || k == "action" || k == "checks" || strings.HasPrefix(k, "on") {
					continue
				}
				walk(n[k], ptr+pointer([]string{k}), k)
			}
		case []any:
			for i, v := range n {
				walk(v, fmt.Sprintf("%s/%d", ptr, i), field)
			}
		case string:
			if !resourceKeys[field] || !strings.HasPrefix(strings.ToLower(strings.TrimLeftFunc(n, resourceLeadingSpace)), "data:") {
				return
			}
			add("resource.inline_data_unsupported", ptr, "Inline data URLs are disallowed by resource preflight; use a verified, host-accessible asset URL")
		}
	}
	for i, m := range docs {
		for _, op := range []string{"createSurface", "updateComponents"} {
			for j, c := range array(object(m[op])["components"]) {
				walk(c, fmt.Sprintf("/%d/%s/components/%d", i, op, j), "")
			}
		}
	}
	valid := true
	for _, d := range diagnostics {
		if d.Severity == "error" {
			valid = false
		}
	}
	unverified := []string{"component-reference closure and binding evaluation"}
	if mode == "new-card" {
		unverified = []string{"template expansion and binding evaluation", "intermediate-frame references and runtime state"}
	}
	unverified = append(unverified, "media decoding and client resource loading", "client rendering and interaction")
	return map[string]any{"mode": mode, "valid": valid, "diagnostics": diagnostics, "renderingVerified": false, "unverified": unverified}, nil
}

func (p *Protocol) CheckNewCard(messages []string) error {
	report, err := p.Preflight(messages, "new-card")
	if err != nil {
		return err
	}
	diagnostics := report["diagnostics"].([]Diagnostic)
	for _, d := range diagnostics {
		if d.Severity == "error" {
			return fmt.Errorf("%s: %s", d.Code, d.Message)
		}
	}
	return nil
}
