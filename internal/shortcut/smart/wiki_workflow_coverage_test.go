// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageResolveSpaceStrictContracts(t *testing.T) {
	invalid := []map[string]any{
		nil,
		{"success": false},
		{"success": "yes"},
		{"result": "bad"},
		{"wikiSpaces": "bad"},
		{"wikiSpaces": []any{"bad"}},
		{"wikiSpaces": []any{map[string]any{"workspaceId": "w"}}},
		{"wikiSpaces": []any{map[string]any{"name": "Docs"}}},
		{"success": true},
	}
	for index, data := range invalid {
		if _, err := resolveSpaceItemsStrict(data); err == nil {
			t.Fatalf("invalid response %d succeeded: %#v", index, data)
		}
	}

	for _, data := range []map[string]any{
		{"wikiSpaces": []any{}},
		{"spaces": []any{map[string]any{"spaceId": "s", "spaceName": "Docs"}}},
		{"result": map[string]any{"items": []any{map[string]any{"id": "i", "title": "Plan"}}}},
		{"data": map[string]any{"records": []any{map[string]any{"workspaceId": "w", "name": "Roadmap"}}}},
	} {
		if _, err := resolveSpaceItemsStrict(data); err != nil {
			t.Fatalf("valid response rejected: %#v: %v", data, err)
		}
	}

	for _, tc := range []struct {
		data map[string]any
		id   string
		name string
	}{
		{map[string]any{"workspaceId": "w", "name": "N"}, "w", "N"},
		{map[string]any{"spaceId": "s", "spaceName": "S"}, "s", "S"},
		{map[string]any{"space_id": "legacy", "title": "T"}, "legacy", "T"},
		{map[string]any{"id": "i"}, "i", ""},
		{map[string]any{}, "", ""},
	} {
		if got := resolveSpaceID(tc.data); got != tc.id {
			t.Fatalf("resolveSpaceID(%#v)=%q want %q", tc.data, got, tc.id)
		}
		if got := resolveSpaceName(tc.data); got != tc.name {
			t.Fatalf("resolveSpaceName(%#v)=%q want %q", tc.data, got, tc.name)
		}
	}
}

func TestCrossPlatformCoverageResolveSpaceExecution(t *testing.T) {
	for _, tc := range []struct {
		name string
		fake *stubMailboxCaller
		ok   bool
	}{
		{"transport error", &stubMailboxCaller{errTool: "search_wikiSpaces"}, false},
		{"malformed result", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": `{"success":true}`}}, false},
		{"zero result", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": `{"wikiSpaces":[]}`}}, false},
		{"one result", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": `{"wikiSpaces":[{"workspaceId":"w","name":"Docs"}]}`}}, true},
		{"multiple results", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": `{"wikiSpaces":[{"workspaceId":"w1","name":"Docs"},{"workspaceId":"w2","name":"Docs 2"}]}`}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runShortcutErr(t, tc.fake, "wiki", "+resolve-space", "--name", "Docs", "--format", "json")
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v want %v; err=%v", err == nil, tc.ok, err)
			}
		})
	}
}

func TestCrossPlatformCoverageWikiNewDocParsing(t *testing.T) {
	invalid := []map[string]any{
		nil,
		{"success": false},
		{"success": "yes"},
		{"result": map[string]any{"wikiSpaces": "bad"}},
		{"wikiSpaces": []any{"bad"}},
		{"wikiSpaces": []any{map[string]any{"workspaceId": "w"}}},
		{"success": true},
	}
	for index, data := range invalid {
		if _, err := wikiNewDocExtractSpaces(data); err == nil {
			t.Fatalf("invalid response %d succeeded: %#v", index, data)
		}
	}

	for _, data := range []map[string]any{
		{"wikiSpaces": []any{}},
		{"list": []any{map[string]any{"workspaceId": "w", "name": "Docs"}}},
		{"data": map[string]any{"spaces": []any{map[string]any{"spaceId": "s", "spaceName": "Plan"}}}},
		{"result": map[string]any{"result": []any{map[string]any{"id": "i", "title": "Roadmap"}}}},
	} {
		if _, err := wikiNewDocExtractSpaces(data); err != nil {
			t.Fatalf("valid response rejected: %#v: %v", data, err)
		}
	}

	if got := wikiNewDocFirstString(map[string]any{"nodeId": " n "}, "nodeId"); got != "n" {
		t.Fatalf("direct node id=%q", got)
	}
	if got := wikiNewDocFirstString(map[string]any{"result": map[string]any{"fileId": " f "}}, "nodeId", "fileId"); got != "f" {
		t.Fatalf("nested node id=%q", got)
	}
	if got := wikiNewDocFirstString(map[string]any{"data": map[string]any{"id": " i "}}, "id"); got != "i" {
		t.Fatalf("data node id=%q", got)
	}
	if got := wikiNewDocFirstString(map[string]any{"id": 1}, "id"); got != "" {
		t.Fatalf("invalid node id=%q", got)
	}
	labels := wikiNewDocLabels([]wikiSpaceCandidate{{id: "w1", name: "Docs"}, {id: "w2", name: "Plan"}})
	if strings.Join(labels, ",") != "Docs(w1),Plan(w2)" {
		t.Fatalf("labels=%v", labels)
	}
}

func TestCrossPlatformCoverageWikiNewDocResolution(t *testing.T) {
	cases := []struct {
		name      string
		data      map[string]any
		spaceName string
		want      string
		ok        bool
	}{
		{"parser error", nil, "Docs", "", false},
		{"zero", map[string]any{"wikiSpaces": []any{}}, "Docs", "", false},
		{"unique exact", map[string]any{"wikiSpaces": []any{map[string]any{"workspaceId": "w1", "name": " docs "}, map[string]any{"workspaceId": "w2", "name": "Plan"}}}, "Docs", "w1", true},
		{"unique fallback", map[string]any{"wikiSpaces": []any{map[string]any{"workspaceId": "w1", "name": "Docs Team"}}}, "Docs", "w1", true},
		{"multiple", map[string]any{"wikiSpaces": []any{map[string]any{"workspaceId": "w1", "name": "Docs 1"}, map[string]any{"workspaceId": "w2", "name": "Docs 2"}}}, "Docs", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wikiNewDocResolveSpaceID(tc.data, tc.spaceName)
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("got=%q err=%v want=%q ok=%v", got, err, tc.want, tc.ok)
			}
		})
	}
}

func TestCrossPlatformCoverageWikiNewDocExecution(t *testing.T) {
	if err := runShortcutErr(t, &stubMailboxCaller{}, "wiki", "+wiki-new-doc", "--space", " ", "--title", "Doc"); err == nil {
		t.Fatal("blank space succeeded")
	}
	if err := runShortcutErr(t, &stubMailboxCaller{}, "wiki", "+wiki-new-doc", "--space", "Docs", "--title", " "); err == nil {
		t.Fatal("blank title succeeded")
	}

	validSearch := `{"wikiSpaces":[{"workspaceId":"w","name":"Docs"}]}`
	cases := []struct {
		name string
		fake *stubMailboxCaller
		args []string
		ok   bool
	}{
		{"search error", &stubMailboxCaller{errTool: "search_wikiSpaces"}, nil, false},
		{"resolve error", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": `{"wikiSpaces":[]}`}}, nil, false},
		{"dry run", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch}}, []string{"--dry-run"}, true},
		{"create error", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch}, errTool: "create_file"}, nil, false},
		{"missing terminal success", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"result":{"fileId":"n"}}`}}, nil, false},
		{"missing created id", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true}`}}, nil, false},
		{"readback error", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true,"fileId":"n"}`}, errTool: "get_document_info"}, nil, false},
		{"readback failed", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true,"fileId":"n"}`, "get_document_info": `{"success":false}`}}, nil, false},
		{"readback malformed success", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true,"fileId":"n"}`, "get_document_info": `{"success":"yes"}`}}, nil, false},
		{"readback id mismatch", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true,"fileId":"n"}`, "get_document_info": `{"success":true,"nodeId":"other"}`}}, nil, false},
		{"verified", &stubMailboxCaller{byTool: map[string]string{"search_wikiSpaces": validSearch, "create_file": `{"success":true,"data":{"fileId":"n"}}`, "get_document_info": `{"success":true,"result":{"nodeId":"n"}}`}}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"wiki", "+wiki-new-doc", "--space", "Docs", "--title", "Doc", "--format", "json"}
			args = append(args, tc.args...)
			err := runShortcutErr(t, tc.fake, args...)
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v want %v; err=%v", err == nil, tc.ok, err)
			}
		})
	}
}
