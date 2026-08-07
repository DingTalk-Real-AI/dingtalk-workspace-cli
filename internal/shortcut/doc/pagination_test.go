// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import "testing"

func TestCrossPlatformCoverageDocSearchPageAllContract(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {
			{"documents": []any{map[string]any{"nodeId": "a", "name": "A"}}, "hasMore": true, "nextPageToken": "p2"},
			{"documents": []any{map[string]any{"nodeId": "b", "name": "B"}, map[string]any{"nodeId": "a", "name": "A"}}, "hasMore": false},
		},
	}}
	if err := runDocCoverage(t, Search, caller, "--query", "report", "--page-all", "--limit", "2"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[1].params["pageToken"] != "p2" {
		t.Fatalf("pagination calls = %#v", caller.history)
	}
}

func TestCrossPlatformCoverageDocPaginationFailsClosedOnStalledCursor(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_nodes": {
			{"nodes": []any{map[string]any{"nodeId": "a"}}, "hasMore": true, "nextPageToken": "p2"},
			{"nodes": []any{map[string]any{"nodeId": "b"}}, "hasMore": true, "nextPageToken": "p2"},
		},
	}}
	err := runDocCoverage(t, List, caller, "--folder", "f", "--page-all", "--limit", "1")
	if err == nil || len(caller.history) != 2 {
		t.Fatalf("stalled pagination err=%v history=%#v", err, caller.history)
	}
}
