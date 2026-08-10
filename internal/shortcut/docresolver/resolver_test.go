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

func TestResolveStableTargetDoesNotSearch(t *testing.T) {
	reader := &scriptedReader{}
	resolution, err := Resolve(reader, "node-1", "")
	if err != nil || reader.calls != 0 || resolution.Selected.CanonicalID != "node-1" || !resolution.Complete {
		t.Fatalf("resolution=%#v calls=%d err=%v", resolution, reader.calls, err)
	}
}

func TestResolveNaturalTitleExhaustsPagesBeforeSelecting(t *testing.T) {
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

func TestResolveFailsClosedForAmbiguousIncompleteAndWrongType(t *testing.T) {
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
