// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package helpers

import "testing"

func TestCrossPlatformCoverageDocMediaViewReadbackUsesExactIdentityAndEditorValue(t *testing.T) {
	blocks := []any{map[string]any{"blockId": "exact", "jsonml": `["card",{"uuid":"exact","viewType":"wideCard"}]`}, map[string]any{"blockId": "other", "jsonml": `["card",{"uuid":"other","viewType":"preview"}]`}}
	if !docMediaViewMatches(blocks, "exact", "summary") || docMediaViewMatches(blocks, "exact", "preview") || docMediaViewMatches(blocks, "missing", "summary") {
		t.Fatal("view readback accepted wrong identity/value")
	}
	if !docMediaViewMatches(blocks, "other", "preview") || !docMediaViewMatches(nil, "missing", "") {
		t.Fatal("explicit preview/default handling")
	}
	if docMediaViewMatches([]any{map[string]any{"jsonml": "not json"}}, "exact", "summary") {
		t.Fatal("invalid JSONML accepted")
	}
}

func TestCrossPlatformCoverageDocMediaViewNestedReadback(t *testing.T) {
	value := map[string]any{"data": []any{"card", map[string]any{"uuid": "wanted", "viewType": "preview"}}}
	if !docMediaViewMatches(value, "wanted", "preview") {
		t.Fatal("nested readback not recognized")
	}
}
