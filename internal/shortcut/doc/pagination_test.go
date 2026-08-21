// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

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

func TestCrossPlatformCoverageDocSearchPublishesExplicitStopReasons(t *testing.T) {
	for _, tc := range []struct {
		name       string
		response   map[string]any
		wantReason string
		complete   bool
	}{
		{
			name:       "single page with continuation",
			response:   map[string]any{"documents": []any{map[string]any{"nodeId": "a"}}, "hasMore": true, "nextPageToken": "p2"},
			wantReason: "single_page",
		},
		{
			name:       "source exhausted",
			response:   map[string]any{"documents": []any{map[string]any{"nodeId": "a"}}, "hasMore": false},
			wantReason: "source_complete",
			complete:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{"search_documents": {tc.response}}}
			var captured map[string]any
			declaration := Search
			declaration.Execute = func(rt *shortcut.RuntimeContext) error {
				var err error
				captured, err = collectDocPages(rt, "search_documents", "documents", nil, searchDocsProject, docPageOptions{PageSize: 10})
				return err
			}
			if err := runDocCoverage(t, declaration, caller); err != nil {
				t.Fatal(err)
			}
			if captured["stopReason"] != tc.wantReason || captured["complete"] != tc.complete {
				t.Fatalf("pagination ledger = %#v", captured)
			}
			if captured["pagesRead"] != 1 {
				t.Fatalf("page counters = %#v", captured)
			}
			for _, redundant := range []string{"pagesFetched", "paginationKnown", "truncatedByPageLimit", "truncatedByResultLimit", "resumeCursorReliable", "failures"} {
				if _, exists := captured[redundant]; exists {
					t.Fatalf("redundant field %q remains in result: %#v", redundant, captured)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDocTemplateListPageAll(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_doc_templates": {
			{"templates": []any{map[string]any{"templateId": "a", "templateName": "A"}}, "hasMore": true, "nextCursor": "p2"},
			{"templates": []any{map[string]any{"templateId": "a", "templateName": "A"}, map[string]any{"templateId": "b", "templateName": "B"}}, "hasMore": false},
		},
	}}
	if err := runDocCoverage(t, TemplateList, caller, "--source", "PUBLIC", "--page-all", "--limit", "2"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[0].params["maxResults"] != 2 || caller.history[1].params["nextCursor"] != "p2" {
		t.Fatalf("template pagination calls = %#v", caller.history)
	}
	if _, exists := caller.history[0].params["pageSize"]; exists {
		t.Fatalf("template list used doc search pageSize: %#v", caller.history[0].params)
	}
}

func TestCrossPlatformCoverageDocTemplateListDefaultsAndFailure(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"list_doc_templates": {{"templates": []any{}, "hasMore": false}},
	}}
	if err := runDocCoverage(t, TemplateList, caller, "--source", "PUBLIC"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 1 || caller.history[0].params["maxResults"] != 20 {
		t.Fatalf("template defaults = %#v", caller.history)
	}

	failure := &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}
	if err := runDocCoverage(t, TemplateList, failure, "--source", "PUBLIC"); err == nil {
		t.Fatal("template-list MCP failure succeeded")
	}
}

func TestCrossPlatformCoverageDocPaginationControlsRequirePageAll(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
	if err := runDocCoverage(t, Search, caller, "--query", "report", "--max-items", "1"); err == nil {
		t.Fatal("--max-items without --page-all succeeded")
	}
	if len(caller.history) != 0 {
		t.Fatalf("invalid pagination reached MCP: %#v", caller.history)
	}
}

func TestCrossPlatformCoverageDocPaginationRejectsNonPositiveBounds(t *testing.T) {
	for _, args := range [][]string{
		{"--query", "report", "--page-all", "--max-pages", "0"},
		{"--query", "report", "--page-all", "--max-items", "0"},
	} {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
		if err := runDocCoverage(t, Search, caller, args...); err == nil {
			t.Fatalf("invalid pagination bounds %v succeeded", args)
		}
		if len(caller.history) != 0 {
			t.Fatalf("invalid pagination bounds %v reached MCP: %#v", args, caller.history)
		}
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

func TestCrossPlatformCoverageDocPaginationMaxItemsStopsAtPageBoundary(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {
			{"documents": []any{map[string]any{"nodeId": "a"}, map[string]any{"nodeId": "b"}}, "hasMore": true, "nextPageToken": "p2"},
			{"documents": []any{map[string]any{"nodeId": "c"}}, "hasMore": true, "nextPageToken": "p3"},
		},
	}}
	var result map[string]any
	declaration := Search
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		var err error
		result, err = collectDocPages(rt, "search_documents", "documents", nil, searchDocsProject, docPageOptions{
			PageAll: rt.Bool("page-all"), PageSize: rt.Int("limit"), MaxPages: rt.Int("max-pages"), MaxItems: rt.Int("max-items"),
		})
		return err
	}
	if err := runDocCoverage(t, declaration, caller, "--query", "report", "--page-all", "--limit", "2", "--max-items", "3"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[1].params["pageSize"] != 1 || caller.history[1].params["pageToken"] != "p2" {
		t.Fatalf("max-items pagination calls = %#v", caller.history)
	}
	if result["count"] != 3 || result["complete"] != false || result["truncated"] != true || result["stopReason"] != "max_items" || result["nextCursor"] != "p3" {
		t.Fatalf("max-items result = %#v", result)
	}
}

func TestCrossPlatformCoverageDocPaginationRejectsServerPageOverflow(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {{
			"documents": []any{map[string]any{"nodeId": "a"}, map[string]any{"nodeId": "b"}},
			"hasMore":   true, "nextPageToken": "p2",
		}},
	}}
	err := runDocCoverage(t, Search, caller, "--query", "report", "--page-all", "--limit", "2", "--max-items", "1")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "doc_pagination_page_size_exceeded" {
		t.Fatalf("page overflow error = %#v", err)
	}
	if len(caller.history) != 1 || caller.history[0].params["pageSize"] != 1 {
		t.Fatalf("page overflow calls = %#v", caller.history)
	}
}

func TestCrossPlatformCoverageDocPaginationFinalBranchMatrix(t *testing.T) {
	run := func(t *testing.T, caller *docCoverageCaller, options docPageOptions) (map[string]any, error) {
		t.Helper()
		var result map[string]any
		declaration := Search
		declaration.Execute = func(rt *shortcut.RuntimeContext) error {
			var err error
			result, err = collectDocPages(rt, "search_documents", "documents", map[string]any{"base": true}, searchDocsProject, options)
			if err == nil {
				err = rt.Output(result)
			}
			return err
		}
		err := runDocCoverage(t, declaration, caller)
		return result, err
	}

	defaults := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {{"documents": []any{map[string]any{"url": "u"}}}},
	}}
	if _, err := run(t, defaults, docPageOptions{Cursor: " initial "}); err != nil {
		t.Fatal(err)
	}
	if len(defaults.history) != 1 || defaults.history[0].params["pageSize"] != 30 || defaults.history[0].params["pageToken"] != "initial" {
		t.Fatalf("default pagination call = %#v", defaults.history)
	}

	readFailure := &docCoverageCaller{failAt: 1, responses: map[string][]map[string]any{}}
	if _, err := run(t, readFailure, docPageOptions{PageAll: true}); err == nil {
		t.Fatal("page read failure succeeded")
	}

	duplicate := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_documents": {{
			"documents": []any{
				map[string]any{"nodeId": "a"}, map[string]any{"nodeId": "a"}, map[string]any{"name": "no-key"},
			},
			"nextPageToken": "p2",
		}},
	}}
	if _, err := run(t, duplicate, docPageOptions{PageAll: false, PageSize: 3, MaxPages: 2, MaxItems: 10}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		response  map[string]any
		maxPages  int
		wantError bool
	}{
		{"unproven", map[string]any{"documents": []any{map[string]any{"nodeId": "a"}}}, 2, true},
		{"missing cursor", map[string]any{"documents": []any{}, "hasMore": true}, 2, true},
		{"max pages", map[string]any{"documents": []any{}, "hasMore": true, "nextPageToken": "p2"}, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{"search_documents": {tc.response}}}
			_, err := run(t, caller, docPageOptions{PageAll: true, PageSize: 1, MaxPages: tc.maxPages, MaxItems: 10})
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v wantError=%v", err, tc.wantError)
			}
		})
	}

	if got := pageItemKey(map[string]any{"url": " u "}); got != "url:u" {
		t.Fatalf("URL page key = %q", got)
	}
	if got := pageItemKey(map[string]any{"id": 1}); got != "" {
		t.Fatalf("invalid page key = %q", got)
	}
	if more, known, next := docPageState(map[string]any{"data": map[string]any{"has_more": true, "nextCursor": "c"}}); !more || !known || next != "c" {
		t.Fatalf("nested page state = %v/%v/%q", more, known, next)
	}
}
