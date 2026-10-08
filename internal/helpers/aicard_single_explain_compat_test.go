// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package helpers

import (
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageAicardSingleExplainCompactUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		code int
	}{
		{"Text", "component", 0},
		{"promptText", "function", 0},
		{"ColorToken", "token", 0},
		{"common_red1_color", "token-item", 0},
		{"Action", "type", 0},
		{"Tabss", "unknown", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aicardCaller{}
			ordinary, code, err := executeAicard(t, caller, "explain", tc.name)
			if err != nil || code != tc.code {
				t.Fatalf("ordinary: code=%d err=%v result=%v", code, err, ordinary)
			}
			var contract map[string]any
			if tc.code == 0 {
				contract = ordinary["data"].(map[string]any)
			} else {
				contract = ordinary["error"].(map[string]any)["details"].(map[string]any)
			}
			if contract["kind"] != tc.kind || contract["name"] != tc.name {
				t.Fatalf("unexpected single-name contract: %v", contract)
			}
			compact, compactCode, err := executeAicard(t, caller, "explain", tc.name, "--compact")
			if err != nil || compactCode != code {
				t.Fatalf("compact: code=%d err=%v result=%v", compactCode, err, compact)
			}
			if !reflect.DeepEqual(ordinary, compact) {
				t.Fatal("--compact changed the single-name response")
			}
			if len(caller.calls) != 0 {
				t.Fatal("offline explain must not call MCP", caller.calls)
			}
		})
	}
}
