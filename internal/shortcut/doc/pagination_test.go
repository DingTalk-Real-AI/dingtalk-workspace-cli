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

func TestCrossPlatformCoverageTemplateLimitPreservesTotalResultCap(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		args        []string
	}{
		{name: "list", declaration: TemplateList, tool: "list_doc_templates"},
		{name: "search", declaration: TemplateSearch, tool: "search_doc_templates", args: []string{"--query", "周报"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				tc.tool: {
					{"templates": []any{map[string]any{"templateId": "a", "name": "周报 A"}}, "hasMore": true, "nextCursor": "p2"},
					{"templates": []any{map[string]any{"templateId": "b", "name": "周报 B"}}, "hasMore": false},
				},
			}}
			args := append(append([]string{}, tc.args...), "--limit", "1")
			if err := runDocCoverage(t, tc.declaration, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.history) != 1 || caller.history[0].params["maxResults"] != 1 {
				t.Fatalf("template limit calls = %#v", caller.history)
			}

			caller = &docCoverageCaller{responses: map[string][]map[string]any{
				tc.tool: {
					{"templates": []any{map[string]any{"templateId": "a", "name": "周报 A"}}, "hasMore": true, "nextCursor": "p2"},
					{"templates": []any{map[string]any{"templateId": "b", "name": "周报 B"}}, "hasMore": false},
				},
			}}
			args = append(args, "--max-items", "2")
			if err := runDocCoverage(t, tc.declaration, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.history) != 2 || caller.history[1].params["nextCursor"] != "p2" {
				t.Fatalf("template max-items calls = %#v", caller.history)
			}
		})
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
	if err := runDocCoverage(t, Search, caller, "--query", "report", "--page-all", "--limit", "2", "--max-items", "3"); err != nil {
		t.Fatal(err)
	}
	if len(caller.history) != 2 || caller.history[1].params["pageSize"] != 1 || caller.history[1].params["pageToken"] != "p2" {
		t.Fatalf("max-items pagination calls = %#v", caller.history)
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
		declaration := Search
		declaration.Execute = func(rt *shortcut.RuntimeContext) error {
			result, err := collectDocPages(rt, "search_documents", "documents", map[string]any{"base": true}, searchDocsProject, options)
			if err == nil {
				err = rt.Output(result)
			}
			return err
		}
		err := runDocCoverage(t, declaration, caller)
		return nil, err
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

func TestCrossPlatformCoverageTemplatePaginationRemainingBranchCoverage(t *testing.T) {
	run := func(t *testing.T, caller *docCoverageCaller, options docPageOptions) error {
		t.Helper()
		declaration := Search
		declaration.Execute = func(rt *shortcut.RuntimeContext) error {
			items, complete, truncated, cursor, stopReason, pages, err := collectTemplatePages(rt, "search_doc_templates", map[string]any{"templateSource": "PUBLIC"}, options)
			if err != nil {
				return err
			}
			return rt.Output(map[string]any{"items": items, "complete": complete, "truncated": truncated, "cursor": cursor, "stopReason": stopReason, "pages": pages})
		}
		return runDocCoverage(t, declaration, caller)
	}
	if err := run(t, &docCoverageCaller{responses: map[string][]map[string]any{
		"search_doc_templates": {{"templates": []any{map[string]any{"templateId": "a"}}}},
	}}, docPageOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := run(t, &docCoverageCaller{responses: map[string][]map[string]any{
		"search_doc_templates": {
			{"templates": []any{map[string]any{"templateId": "a"}}, "hasMore": true, "nextCursor": "p2"},
			{"templates": []any{map[string]any{"templateId": "a"}}, "hasMore": false},
		},
	}}, docPageOptions{PageAll: true, PageSize: 2, MaxPages: 2, MaxItems: 10}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		responses []map[string]any
		options   docPageOptions
		wantErr   bool
	}{
		{"unproven", []map[string]any{{"templates": []any{map[string]any{"templateId": "a"}}}}, docPageOptions{PageAll: true, PageSize: 1, MaxPages: 2, MaxItems: 10}, true},
		{"max items", []map[string]any{{"templates": []any{map[string]any{"templateId": "a"}}, "hasMore": true, "nextCursor": "p2"}}, docPageOptions{PageAll: true, PageSize: 1, MaxPages: 2, MaxItems: 1}, false},
		{"stalled", []map[string]any{{"templates": []any{}, "hasMore": true}}, docPageOptions{PageAll: true, PageSize: 1, MaxPages: 2, MaxItems: 10}, true},
		{"max pages", []map[string]any{{"templates": []any{}, "hasMore": true, "nextCursor": "p2"}}, docPageOptions{PageAll: true, PageSize: 1, MaxPages: 1, MaxItems: 10}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(t, &docCoverageCaller{responses: map[string][]map[string]any{"search_doc_templates": tc.responses}}, tc.options)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageTemplateSinglePageReportsUnprovenPagination(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"search_doc_templates": {{
			"templates": []any{map[string]any{"templateId": "a"}},
		}},
	}}
	var (
		gotItems      []map[string]any
		gotComplete   bool
		gotTruncated  bool
		gotNextCursor string
		gotStopReason string
		gotPages      int
	)
	declaration := Search
	declaration.Execute = func(rt *shortcut.RuntimeContext) error {
		var err error
		gotItems, gotComplete, gotTruncated, gotNextCursor, gotStopReason, gotPages, err = collectTemplatePages(
			rt,
			"search_doc_templates",
			map[string]any{"templateSource": "PUBLIC"},
			docPageOptions{PageAll: false, PageSize: 1, MaxPages: 2, MaxItems: 10},
		)
		return err
	}
	if err := runDocCoverage(t, declaration, caller); err != nil {
		t.Fatal(err)
	}
	if len(gotItems) != 1 || gotComplete || gotTruncated || gotNextCursor != "" || gotStopReason != "pagination_unproven" || gotPages != 1 {
		t.Fatalf(
			"single-page result = items:%d complete:%v truncated:%v cursor:%q stop:%q pages:%d",
			len(gotItems), gotComplete, gotTruncated, gotNextCursor, gotStopReason, gotPages,
		)
	}
}

func TestCrossPlatformCoverageTemplatePaginationRejectsServerPageOverflow(t *testing.T) {
	for _, tool := range []string{"search_doc_templates", "list_doc_templates"} {
		t.Run(tool, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				tool: {{
					"templates": []any{
						map[string]any{"templateId": "a"},
						map[string]any{"templateId": "b"},
					},
					"hasMore":    false,
					"nextCursor": "server-next",
				}},
			}}
			declaration := Search
			declaration.Execute = func(rt *shortcut.RuntimeContext) error {
				_, _, _, _, _, _, err := collectTemplatePages(
					rt,
					tool,
					map[string]any{"templateSource": "PUBLIC"},
					docPageOptions{PageAll: true, PageSize: 10, MaxPages: 2, MaxItems: 1},
				)
				return err
			}
			err := runDocCoverage(t, declaration, caller)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "doc_pagination_page_size_exceeded" {
				t.Fatalf("template overflow error = %#v", err)
			}
			if got, ok := typed.Details["items"].([]map[string]any); !ok || len(got) != 0 {
				t.Fatalf("template overflow partial items = %#v", typed.Details["items"])
			}
			if len(caller.history) != 1 || caller.history[0].params["maxResults"] != 1 {
				t.Fatalf("overflow request = %#v", caller.history)
			}
		})
	}
}
