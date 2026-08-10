// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package docresolver

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type scriptedReader struct {
	pages []map[string]any
	err   error
	calls int
}

func (r *scriptedReader) CallMCPData(_ string, _ string, _ map[string]any) (map[string]any, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if len(r.pages) == 0 {
		return map[string]any{}, nil
	}
	page := r.pages[0]
	r.pages = r.pages[1:]
	return page, nil
}

func row(id, name, typ string) any {
	return map[string]any{"nodeId": id, "name": name, "docType": typ, "url": "https://alidocs.dingtalk.com/i/nodes/" + id}
}

func TestCrossPlatformCoverageResolveStableTargetDoesNotSearch(t *testing.T) {
	reader := &scriptedReader{}
	resolution, err := Resolve(reader, "node-1", "")
	if err != nil || reader.calls != 0 || resolution.Selected.CanonicalID != "node-1" || !resolution.Complete {
		t.Fatalf("resolution=%#v calls=%d err=%v", resolution, reader.calls, err)
	}
}

func TestCrossPlatformCoverageResolveNaturalTitleExhaustsPagesBeforeSelecting(t *testing.T) {
	first := make([]any, 0, searchPageSize)
	for i := 0; i < searchPageSize; i++ {
		first = append(first, row(fmt.Sprintf("other-%d", i), "其他", "adoc"))
	}
	reader := &scriptedReader{pages: []map[string]any{
		{"documents": first, "hasMore": true, "nextPageToken": "p2"},
		{"documents": []any{row("wanted", "项目周报", "adoc")}, "hasMore": false},
	}}
	resolution, err := Resolve(reader, "", "项目周报")
	if err != nil || reader.calls != 2 || resolution.Selected.CanonicalID != "wanted" || resolution.MatchedBy != "exact_title" {
		t.Fatalf("resolution=%#v calls=%d err=%v", resolution, reader.calls, err)
	}
}

func TestCrossPlatformCoverageResolveFailsClosedForAmbiguousIncompleteAndWrongType(t *testing.T) {
	tests := []struct {
		name   string
		pages  []map[string]any
		query  string
		reason string
	}{
		{"ambiguous", []map[string]any{{"documents": []any{row("1", "周报", "adoc"), row("2", "周报", "adoc")}, "hasMore": false}}, "周报", "ambiguous"},
		{"not found", []map[string]any{{"documents": []any{}, "hasMore": false}}, "missing", "not_found"},
		{"stalled", []map[string]any{{"documents": []any{row("1", "周报", "adoc")}, "hasMore": true, "nextPageToken": "p1"}, {"documents": []any{}, "hasMore": true, "nextPageToken": "p1"}}, "周报", "incomplete"},
		{"wrong type", []map[string]any{{"documents": []any{row("1", "周报", "pdf")}, "hasMore": false}}, "周报", "type_mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(&scriptedReader{pages: tc.pages}, "", tc.query)
			var typed *apperrors.Error
			if err == nil || !errors.As(err, &typed) || !strings.Contains(typed.Reason, tc.reason) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	reader := &scriptedReader{err: errors.New("backend")}
	if _, err := Resolve(reader, "", "x"); !errors.Is(err, reader.err) {
		t.Fatalf("transport error=%v", err)
	}
}

func TestCrossPlatformCoverageResolverFinalBranchMatrix(t *testing.T) {
	for _, target := range [][2]string{{"", ""}, {"node", "query"}} {
		if _, err := Resolve(&scriptedReader{}, target[0], target[1]); err == nil {
			t.Fatalf("invalid target %#v succeeded", target)
		}
	}

	full := make([]any, searchPageSize)
	for i := range full {
		full[i] = row(fmt.Sprintf("full-%d", i), "other", "adoc")
	}
	for _, tc := range []struct {
		name  string
		pages []map[string]any
	}{
		{"unproven full page", []map[string]any{{"documents": full}}},
		{"missing cursor", []map[string]any{{"documents": []any{}, "hasMore": true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Resolve(&scriptedReader{pages: tc.pages}, "", "wanted"); err == nil {
				t.Fatal("incomplete pagination succeeded")
			}
		})
	}
	if _, err := Resolve(&scriptedReader{pages: []map[string]any{{"documents": []any{}}}}, "", "wanted"); err == nil {
		t.Fatal("underfilled implicit final page should resolve to not-found")
	}
	pages := make([]map[string]any, searchPageLimit)
	for i := range pages {
		pages[i] = map[string]any{"documents": []any{}, "hasMore": true, "nextPageToken": fmt.Sprintf("p-%d", i+1)}
	}
	if _, err := Resolve(&scriptedReader{pages: pages}, "", "wanted"); err == nil {
		t.Fatal("max-page pagination succeeded")
	}

	nested := map[string]any{
		"result": map[string]any{
			"data": map[string]any{
				"records": []any{
					"invalid",
					map[string]any{"id": "", "title": "empty"},
					map[string]any{"doc_id": "d", "fileName": "Doc", "fileType": ".ADOC", "webUrl": " u ", "parentId": "p"},
				},
				"pagination": map[string]any{"has_more": false, "next_page_token": " next "},
			},
		},
	}
	rows := extractRows(nested)
	candidates := extractCandidates(rows)
	if len(candidates) != 1 || candidates[0].CanonicalID != "d" || normalizeType(candidates[0].ResourceType) != "adoc" {
		t.Fatalf("nested candidates = %#v", candidates)
	}
	if hasMore, known, next := extractPage(nested); !known || hasMore || next != "next" {
		t.Fatalf("nested page = %v/%v/%q", hasMore, known, next)
	}
	if rows := extractRows(map[string]any{"items": "not-a-list"}); len(rows) != 0 {
		t.Fatalf("non-list rows = %#v", rows)
	}
	deduped := dedupe([]Candidate{{}, {CanonicalID: "d"}, {CanonicalID: "d"}})
	if len(deduped) != 1 {
		t.Fatalf("deduped = %#v", deduped)
	}
	selected, matchedBy := selectCandidates([]Candidate{{CanonicalID: "d", Name: "Different"}}, "query")
	if len(selected) != 1 || matchedBy != "search_candidate" {
		t.Fatalf("fallback selection = %#v/%q", selected, matchedBy)
	}
}
