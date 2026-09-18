// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	core "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func TestCrossPlatformCoverageDiffComparisonBoundaries(t *testing.T) {
	identity := &identityMapFile{Nodes: map[string]string{"logical": "real", "parent": "p"}}
	before := map[string]any{"id": "real", "type": "frame", "parentId": "p", "refs": []any{map[string]any{"nodeRef": map[string]any{"scope": "page", "id": "p"}}}}
	after := map[string]any{"id": "logical", "type": "frame", "parentId": "parent", "refs": []any{map[string]any{"nodeRef": map[string]any{"scope": "request", "id": "parent"}}}}
	if changes, missing := compareMappedNodes(before, after, identity, "exact"); len(changes) != 0 || len(missing) != 0 {
		t.Fatalf("equivalent refs=%v %v", changes, missing)
	}
	before["parentId"] = "missing"
	before["refs"] = []any{map[string]any{"nodeRef": map[string]any{"id": "missing"}}}
	if changes, missing := compareMappedNodes(before, after, identity, "exact"); changes != nil || !reflect.DeepEqual(missing, []string{"missing"}) {
		t.Fatalf("unresolved=%v %v", changes, missing)
	}
	for _, tc := range []struct {
		a, b  any
		equal bool
	}{
		{nil, nil, true}, {nil, 1, false}, {1, "1", false}, {1, 2, false}, {true, true, true}, {false, true, false},
		{map[string]any{"x": 1}, map[string]any{}, false},
		{[]any{1, "x"}, []any{1, "x"}, true}, {[]any{1}, []any{2}, false}, {[]any{}, []any{1}, false}, {[]any{}, "x", false},
		{struct{ X int }{1}, struct{ X int }{1}, true},
	} {
		if got := valuesEqual("", tc.a, tc.b, "exact"); got != tc.equal {
			t.Fatalf("equal(%#v,%#v)=%v", tc.a, tc.b, got)
		}
	}
	var changes []map[string]any
	diffJSONValue("", []any{1, 2}, []any{2, 3}, "exact", &changes)
	diffJSONValue("", true, false, "exact", &changes)
	if len(changes) != 3 || changes[2]["path"] != "/" {
		t.Fatalf("array/root changes=%v", changes)
	}
	if got := uniqueSortedStrings([]string{"b", "a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatal(got)
	}
}

func TestCrossPlatformCoverageDiffRuntimeFailurePropagation(t *testing.T) {
	source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"text"}]}}`
	for _, args := range [][]string{
		{"--source", "{"},
		{"--source", source, "--node", ""},
		{"--source", source, "--node", "n", "--page-id", "p", "--identity-map", "{"},
		{"--source", source, "--node", "n", "--page-id", "p", "--detail-limit", "0"},
	} {
		rt := directWhiteboardRuntime(t, Diff, &whiteboardCoverageCaller{}, args...)
		if Diff.Validate(rt) == nil || Diff.Execute(rt) == nil {
			t.Fatalf("invalid runtime accepted: %v", args)
		}
	}
	for _, tc := range []struct {
		response string
		fail     bool
		extra    []string
	}{
		{"", true, nil}, {`{}`, false, nil},
		{validStandaloneWhiteboardQueryResponse("wb", "page", 1, `[]`), false, []string{"--identity-map", `{"version":1,"target":{"nodeId":"other","pageId":"page-1"},"nodes":{}}`}},
	} {
		caller := &whiteboardCoverageCaller{responses: map[string][]string{toolQueryStandalone: {tc.response}}}
		if tc.fail {
			caller.responses = nil
		}
		args := append([]string{"--node", "wb", "--page-id", "page-1", "--source", source}, tc.extra...)
		if err := Diff.Execute(directWhiteboardRuntime(t, Diff, caller, args...)); err == nil {
			t.Fatal("runtime failure lost")
		}
	}
	if err := Update.Execute(directWhiteboardRuntime(t, Update, &whiteboardCoverageCaller{}, "--node", "wb", "--page-id", "page-1", "--source", source, "--expected-source-digest", "invalid")); err == nil {
		t.Fatal("update digest validation skipped")
	}
	for _, tc := range []struct {
		request  map[string]any
		response map[string]any
	}{
		{map[string]any{"nodeId": "n", "view": "invalid"}, map[string]any{"success": true, "nodeId": "n", "revision": 1}},
		{map[string]any{"nodeId": "n", "view": "summary"}, map[string]any{"success": true, "nodeId": "n", "revision": 1, "resultSummary": map[string]any{"nodeCount": 0, "pageCount": 0}}},
	} {
		if _, err := projectStandaloneWhiteboardQuery(tc.response, tc.request); err == nil {
			t.Fatal("invalid view/summary accepted")
		}
	}
}

func TestCrossPlatformCoverageDiffIdentityValidation(t *testing.T) {
	for _, raw := range []string{"", "{", `{} {}`, `{}`, `{"version":1,"target":{"nodeId":"n"},"nodes":{"":"r"}}`, `{"version":1,"target":{"nodeId":"n","revision":-1},"nodes":{}}`} {
		if _, err := parseIdentityMap(raw, true); err == nil {
			t.Fatalf("accepted identity %s", raw)
		}
	}
	for _, tc := range []struct {
		kind   core.Kind
		target identityMapTarget
	}{
		{core.KindEmbedded, identityMapTarget{NodeID: "n", PartID: "part", PageID: "page"}},
		{core.KindStandalone, identityMapTarget{NodeID: "n", PageID: "page", PartID: "part"}},
		{core.KindStandalone, identityMapTarget{NodeID: "wrong", PageID: "page"}},
		{core.KindStandalone, identityMapTarget{NodeID: "n", PageID: "page"}},
	} {
		id := &identityMapFile{Target: tc.target, Nodes: map[string]string{"a": "absent"}}
		if err := validateIdentityMapForSnapshot(id, map[string]any{"kind": string(tc.kind), "nodeId": "n", "pageId": "page", "partId": "part"}, nil); err == nil {
			t.Fatal("accepted invalid target/mapping")
		}
	}
	if err := validateIdentityMapForSnapshot(&identityMapFile{Target: identityMapTarget{NodeID: "n", PartID: "p"}}, map[string]any{"kind": string(core.KindEmbedded), "nodeId": "n", "partId": "p"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDiffSnapshotAndDigestFailures(t *testing.T) {
	for _, source := range []map[string]any{{}, {"pages": []any{1}}, {"pages": []any{map[string]any{}}}, {"pages": []any{map[string]any{"nodes": []any{1}}}}} {
		if _, _, err := singleDiffPage(source); err == nil {
			t.Fatalf("snapshot accepted: %v", source)
		}
	}
	call := core.Call{Kind: core.KindStandalone, Args: map[string]any{"pageId": "page"}}
	valid := func() map[string]any {
		return map[string]any{"nodeId": "n", "revision": 1, "source": map[string]any{"pages": []any{map[string]any{"id": "page", "nodes": []any{}}}}}
	}
	proposed := &parsedUpdate{Nodes: []map[string]any{{"id": "p", "type": "text"}}}
	for _, tc := range []struct {
		p    map[string]any
		mode string
	}{{nil, ""}, {map[string]any{"source": map[string]any{}}, ""}, {valid(), "invalid"}} {
		if _, err := buildWhiteboardDiff(tc.p, call, proposed, nil, tc.mode, 10); err == nil {
			t.Fatal("invalid diff accepted")
		}
	}
	wrong := valid()
	wrong["source"].(map[string]any)["pages"].([]any)[0].(map[string]any)["id"] = "wrong"
	if _, err := buildWhiteboardDiff(wrong, call, proposed, nil, "", 10); err == nil {
		t.Fatal("wrong page accepted")
	}
	if _, err := buildWhiteboardDiff(valid(), call, proposed, &identityMapFile{Target: identityMapTarget{NodeID: "other"}}, "", 10); err == nil {
		t.Fatal("wrong identity accepted")
	}
	bad := &parsedUpdate{Nodes: []map[string]any{{"id": "p", "bad": make(chan int)}}}
	if _, err := buildWhiteboardDiff(valid(), call, bad, nil, "", 10); err == nil {
		t.Fatal("unencodable source accepted")
	}
	if err := validateExpectedSourceDigest("sha256:"+strings.Repeat("0", 64), true, bad); err == nil {
		t.Fatal("digest encoding failure lost")
	}
	invalidSnapshot := valid()
	invalidSnapshot["source"].(map[string]any)["bad"] = make(chan int)
	if _, err := buildWhiteboardDiff(invalidSnapshot, call, proposed, nil, "", 10); err == nil {
		t.Fatal("unencodable snapshot accepted")
	}
	invalidResult := valid()
	invalidResult["nodeId"] = make(chan int)
	if _, err := buildWhiteboardDiff(invalidResult, call, proposed, nil, "", 10); err == nil {
		t.Fatal("unencodable result accepted")
	}
	large := valid()
	large["nodeId"] = strings.Repeat("x", maximumDiffOutputBytes)
	if _, err := buildWhiteboardDiff(large, call, proposed, nil, "", 10); err == nil {
		t.Fatal("unbounded summary accepted")
	}
	huge := &parsedUpdate{Nodes: []map[string]any{{"id": strings.Repeat("x", maximumDiffOutputBytes), "type": "text"}}}
	result, err := buildWhiteboardDiff(valid(), call, huge, nil, "", 1)
	if err != nil || result["detailsTruncated"] != true {
		t.Fatalf("bounded detail=%v %v", result, err)
	}
}

func TestCrossPlatformCoverageDiffLogicalMediaTransitions(t *testing.T) {
	for _, tc := range []struct {
		before, after map[string]any
		category      string
	}{
		{map[string]any{"type": "text"}, nil, "logicalDeleted"},
		{map[string]any{"type": "image"}, map[string]any{"type": "image"}, "logicalUnmatched"},
		{map[string]any{"type": "text", "parentId": "unknown"}, map[string]any{"type": "text"}, "logicalUnmatched"},
		{map[string]any{"type": "text"}, map[string]any{"type": "text"}, "unchanged"},
	} {
		tc.before["id"] = "r"
		var after []map[string]any
		if tc.after != nil {
			tc.after["id"] = "l"
			after = append(after, tc.after)
		}
		a := newDiffAccumulator()
		computeLogicalDiff(a, []map[string]any{tc.before}, after, &identityMapFile{Nodes: map[string]string{"l": "r"}}, "exact")
		if tc.category == "unchanged" {
			if a.Unchanged != 1 {
				t.Fatal(a)
			}
		} else if a.Summary[tc.category] != 1 {
			t.Fatalf("%s: %#v", tc.category, a)
		}
	}
	node := func(id, resource, url string) map[string]any {
		return map[string]any{"id": id, "type": "vector", "resource": map[string]any{"resourceId": resource, "url": url}}
	}
	for _, tc := range []struct{ left, right, url, category string }{{"", "a", "", "mediaAdded"}, {"a", "", "", "mediaRemoved"}, {"a", "b", "", "mediaReplaced"}, {"a", "a", "new", "mediaMetadataChanged"}} {
		a := newDiffAccumulator()
		computeMediaDiff(a, "overwrite", []map[string]any{node("r", tc.left, "")}, []map[string]any{node("l", tc.right, tc.url)}, &identityMapFile{Nodes: map[string]string{"l": "r", "absent": "absent"}})
		if a.Summary[tc.category] != 1 {
			t.Fatalf("%s=%#v", tc.category, a.Summary)
		}
	}
	a := newDiffAccumulator()
	master := node("master", "asset", "")
	master["source"] = "master"
	locked := map[string]any{"id": "locked", "type": "text", "locked": true}
	computeExecutionDiff(a, "overwrite", []map[string]any{master, locked}, nil)
	if a.Preserved != 1 || a.Blockers["locked_node_would_be_deleted"] != 1 {
		t.Fatalf("master/locked=%#v", a)
	}
	computeMediaDiff(a, "overwrite", []map[string]any{master}, []map[string]any{node("new", "asset", "")}, &identityMapFile{Nodes: map[string]string{}})
	computeMediaDiff(a, "append", nil, []map[string]any{{"id": "icon", "type": "icon", "catalogId": "task"}}, nil)
	if a.Summary["mediaAdded"] != 2 {
		t.Fatal(a.Summary)
	}
	computeLogicalDiff(a, nil, []map[string]any{{"id": "new", "type": "text"}}, &identityMapFile{Nodes: map[string]string{}}, "exact")
	if a.Summary["logicalAdded"] != 1 {
		t.Fatal(a.Summary)
	}
}

func TestCrossPlatformCoverageDiffSelectionBudgetAndDeterminism(t *testing.T) {
	var all []diffCandidate
	for _, cat := range []string{"a", "b", "c"} {
		for _, key := range []string{"z", "a", "b"} {
			p := 1
			if cat == "c" {
				p = 0
			}
			all = append(all, diffCandidate{Category: cat, Priority: p, SortKey: key})
		}
	}
	for _, limit := range []int{1, 3, 4, 7, 8, 20} {
		got := selectDiffCandidates(all, limit)
		want := limit
		if want > len(all) {
			want = len(all)
		}
		if len(got) != want {
			t.Fatalf("limit=%d got=%v", limit, got)
		}
		encoded, _ := json.Marshal(got)
		again, _ := json.Marshal(selectDiffCandidates(all, limit))
		if string(encoded) != string(again) {
			t.Fatal("nondeterministic")
		}
		if limit >= 3 {
			seen := map[string]bool{}
			for _, c := range got {
				seen[c.Category] = true
			}
			if len(seen) != 3 {
				t.Fatal("starved category")
			}
		}
	}
}
