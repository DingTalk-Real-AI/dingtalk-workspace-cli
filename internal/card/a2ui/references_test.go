// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestCrossPlatformCoverageAicardReferencePreflightSharedCases(t *testing.T) {
	p := testProtocol(t)
	// Case names are English; some card text remains localized to cover Unicode payloads.
	data, err := os.ReadFile("testdata/preflight-references.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string            `json:"name"`
		Messages    []json.RawMessage `json:"messages"`
		Mode        string            `json:"mode"`
		Valid       bool              `json:"valid"`
		Diagnostics []Diagnostic      `json:"diagnostics"`
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	project := func(ds []Diagnostic) []string {
		keys := []string{}
		for _, d := range ds {
			keys = append(keys, d.Code+"|"+d.Severity+"|"+d.Pointer)
		}
		sort.Strings(keys)
		return keys
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.Messages)
			r, err := p.Lint(raw, false, false)
			if err != nil || !r.Valid {
				t.Fatalf("Schema: %v %+v", err, r)
			}
			messages := []string{}
			for _, m := range tc.Messages {
				messages = append(messages, string(m))
			}
			result, err := p.Preflight(messages, tc.Mode)
			if err != nil || result["valid"] != tc.Valid {
				t.Fatalf("preflight: %v %+v", err, result)
			}
			if got := project(result["diagnostics"].([]Diagnostic)); !reflect.DeepEqual(got, project(tc.Diagnostics)) {
				t.Fatalf("diagnostics: got %v, want %v", got, tc.Diagnostics)
			}
			if tc.Mode == "new-card" && (p.CheckNewCard(messages) == nil) != tc.Valid {
				t.Fatal("preview entry point did not distinguish warnings from errors")
			}
		})
	}
}

func TestCrossPlatformCoverageAicardDeepReferenceGraph(t *testing.T) {
	p := testProtocol(t)
	nodes := []any{}
	for i := 0; i < 1500; i++ {
		id := fmt.Sprint(i)
		if i == 0 {
			id = "root"
		}
		nodes = append(nodes, map[string]any{"id": id, "component": "Column", "children": []any{fmt.Sprint(i + 1)}})
	}
	nodes = append(nodes, map[string]any{"id": "1500", "component": "Text", "text": "Last node"})
	s := referenceSnapshot{map[string]referenceNode{}, "/0/createSurface/surfaceId"}
	s.update(map[string]any{"components": nodes}, "/0/createSurface")
	if ds := p.checkReferences(s, "s"); len(ds) != 0 {
		t.Fatal(ds)
	}
}
