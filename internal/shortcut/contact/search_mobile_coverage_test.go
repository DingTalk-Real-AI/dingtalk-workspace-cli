// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package contact

import "testing"

func TestCrossPlatformCoverageSearchMobileProjectsObjectEnvelope(t *testing.T) {
	users := searchUserProject(map[string]any{
		"success": true,
		"result": map[string]any{
			"name":             "Alice",
			"userId":           "u1",
			"flowerName":       "A",
			"openDingTalkId":   "open-1",
			"title":            "Engineer",
			"unrelatedPrivate": "drop-me",
		},
	})
	if len(users) != 1 || users[0]["userId"] != "u1" || users[0]["name"] != "Alice" {
		t.Fatalf("object envelope projection = %#v, want one user", users)
	}
	if _, exists := users[0]["unrelatedPrivate"]; exists {
		t.Fatalf("projection leaked unrelated field: %#v", users[0])
	}
}

func TestCrossPlatformCoverageSearchMobilePreservesEmptyObjectSemantics(t *testing.T) {
	users := searchUserProject(map[string]any{"success": true, "result": map[string]any{}})
	if len(users) != 0 {
		t.Fatalf("empty result projected users: %#v", users)
	}
}

func TestCrossPlatformCoverageSearchMobileProjectsArrayAndMissingResult(t *testing.T) {
	users := searchUserProject(map[string]any{
		"result": []any{map[string]any{"name": "Alice", "userId": "u1"}},
	})
	if len(users) != 1 || users[0]["userId"] != "u1" || users[0]["name"] != "Alice" {
		t.Fatalf("array envelope projection = %#v, want one user", users)
	}
	if users := searchUserProject(map[string]any{"success": true}); len(users) != 0 {
		t.Fatalf("missing result projected users: %#v", users)
	}
}
