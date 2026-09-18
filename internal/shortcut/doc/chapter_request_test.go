// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageDocChapterReadsFullTreeBeforeSelection(t *testing.T) {
	const body = `["root",{},["p",{"uuid":"before"},"Before"],["h2",{"uuid":"start"},"Chapter"],["p",{"uuid":"body"},"Body"],["h3",{"uuid":"child"},"Subchapter"],["p",{"uuid":"child-body"},"Child body"],["h2",{"uuid":"next"},"Next chapter"],["p",{"uuid":"outside"},"Outside"]]`
	for _, tc := range []struct {
		name  string
		extra []string
		ids   []string
	}{
		{"chapter-only", nil, []string{"start", "body", "child", "child-body"}},
		{"both-contexts", []string{"--context-before", "1", "--context-after", "1"}, []string{"before", "start", "body", "child", "child-body", "next"}},
		{"remote-filters-do-not-truncate", []string{"--context-before", "1", "--context-after", "1", "--end-block-id", "body", "--tags", "p"}, []string{"before", "start", "body", "child", "child-body", "next"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"nodeId": "n", "jsonml": body}}}}
			args := append([]string{"--node", "n", "--scope", "chapter", "--start-block-id", "start", "--version", "0", "--password", "test-password"}, tc.extra...)
			got := runDocCoverageEnvelope(t, Fetch, c, args...)
			if len(c.history) != 1 || c.history[0].tool != "get_document_content" || !reflect.DeepEqual(c.history[0].params, map[string]any{"nodeId": "n", "format": "jsonml", "historyVersion": 0, "password": "test-password"}) {
				t.Fatal(c.history)
			}
			content := got["content"].(map[string]any)
			var fragment []any
			if err := json.Unmarshal([]byte(content["jsonml"].(string)), &fragment); err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, b := range fragment[2:] {
				ids = append(ids, jsonMLBlockIdentity(b.([]any)))
			}
			if fragment[0] != "fragment" || !reflect.DeepEqual(ids, tc.ids) || content["nodeId"] != "n" {
				t.Fatal(content)
			}
		})
	}
}

func TestCrossPlatformCoverageDocRemoteSelectionsStillForwardFilters(t *testing.T) {
	for _, scope := range []string{"range", "section", "tags"} {
		t.Run(scope, func(t *testing.T) {
			c := &docCoverageCaller{}
			runDocCoverageEnvelope(t, Fetch, c, "--node", "n", "--scope", scope, "--start-block-id", "a", "--end-block-id", "b", "--tags", "p,h1")
			want := map[string]any{"nodeId": "n", "format": "jsonml", "scope": scope, "startBlockId": "a", "endBlockId": "b", "tags": "p,h1"}
			if len(c.history) != 1 || !reflect.DeepEqual(c.history[0].params, want) {
				t.Fatal(c.history)
			}
		})
	}
}
