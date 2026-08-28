// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitabletarget

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParseAITableURLRemainingEdges(t *testing.T) {
	for _, raw := range []string{
		"https://alidocs.dingtalk.com/i/nodes/base%2Fchild",
		"https://alidocs.dingtalk.com/i/nodes/base?iframeQuery=%25",
		"https://alidocs.dingtalk.com/i/nodes/base?viewId=one&viewId=two&sheetId=table",
		"https://alidocs.dingtalk.com/i/nodes/base?recordId=one&recordId=two&sheetId=table",
	} {
		if _, err := ParseURL(raw); err == nil {
			t.Fatalf("ParseURL(%q) succeeded", raw)
		}
	}
	if _, err := parseIframeQuery("%"); err == nil {
		t.Fatal("invalid iframe escape succeeded")
	}
	if values, err := parseIframeQuery("preview"); err != nil || !values.Has("preview") {
		t.Fatalf("plain iframe query = %#v, %v", values, err)
	}
	if _, err := parseIframeQuery("key=%zz"); err == nil {
		t.Fatal("invalid iframe query value succeeded")
	}
	values := url.Values{"id": {" ", "bad/id"}}
	if _, err := uniqueQueryID(values, "id"); err == nil {
		t.Fatal("invalid query ID succeeded")
	}
	if validID("ok\x7fno") {
		t.Fatal("control character ID succeeded")
	}
}

func TestCrossPlatformCoverageResolveBaseRemainingFailures(t *testing.T) {
	if _, err := ResolveBaseName(&resolverReader{}, " ", false); err == nil {
		t.Fatal("empty base name succeeded")
	}
	reader := &resolverReader{steps: []resolverStep{{err: errors.New("search failed")}}}
	if _, err := ResolveBaseName(reader, "name", false); err == nil {
		t.Fatal("search error was swallowed")
	}
	reader = &resolverReader{steps: []resolverStep{
		{data: map[string]any{"bases": []any{}, "nextCursor": "same"}},
		{data: map[string]any{"bases": []any{}, "nextCursor": "same"}},
	}}
	if _, err := ResolveBaseName(reader, "name", false); errorReason(err) != "target_incomplete" {
		t.Fatalf("cursor cycle = %v", err)
	}

	steps := make([]resolverStep, maxResolutionPages)
	for index := range steps {
		steps[index] = resolverStep{data: map[string]any{"bases": []any{}, "nextCursor": fmt.Sprintf("cursor-%d", index)}}
	}
	reader = &resolverReader{steps: steps}
	if _, err := ResolveBaseName(reader, "name", false); errorReason(err) != "target_incomplete" || len(reader.calls) != maxResolutionPages {
		t.Fatalf("page bound = err:%v calls:%d", err, len(reader.calls))
	}
}

func TestCrossPlatformCoverageResolveTableRemainingFailures(t *testing.T) {
	for _, pair := range [][2]string{{"bad/id", "name"}, {"base", " "}} {
		if _, err := ResolveTableName(&resolverReader{}, pair[0], pair[1], false); err == nil {
			t.Fatalf("invalid table args %#v succeeded", pair)
		}
	}
	reader := &resolverReader{steps: []resolverStep{{err: errors.New("tables failed")}}}
	if _, err := ResolveTableName(reader, "base", "name", false); err == nil {
		t.Fatal("table error was swallowed")
	}
	reader = &resolverReader{steps: []resolverStep{{data: map[string]any{"tables": "bad"}}}}
	if _, err := ResolveTableName(reader, "base", "name", false); errorReason(err) != "target_invalid_response" {
		t.Fatalf("invalid table list = %v", err)
	}
	reader = &resolverReader{steps: []resolverStep{{data: map[string]any{"success": true}}}}
	if _, err := ResolveTableName(reader, "base", "name", false); errorReason(err) != "target_invalid_response" {
		t.Fatalf("missing table list = %v", err)
	}
}

func TestCrossPlatformCoverageResolverPureShapeHelpers(t *testing.T) {
	if list, found, err := findObjectList(nil, "items"); err != nil || found || list != nil {
		t.Fatalf("nil list = %#v, %v, %v", list, found, err)
	}
	if _, _, err := findObjectList(map[string]any{"items": "bad"}, "items"); err == nil {
		t.Fatal("invalid list field succeeded")
	}
	if list, found, err := findObjectList(map[string]any{"items": map[string]any{"items": []any{}}}, "items"); err != nil || !found || len(list) != 0 {
		t.Fatalf("nested keyed list = %#v, %v, %v", list, found, err)
	}
	cursor, more, known := Pagination(nil)
	if cursor != "" || more || known {
		t.Fatalf("nil pagination = %q, %v, %v", cursor, more, known)
	}
	cursor, more, known = Pagination(map[string]any{"cursor": "outer", "data": map[string]any{"nextCursor": "inner", "hasMore": true}})
	if cursor != "outer" || !more || !known {
		t.Fatalf("nested pagination = %q, %v, %v", cursor, more, known)
	}
	got := dedupe([]Candidate{{}, {ID: "one", Name: "first"}, {ID: "one", Name: "duplicate"}, {ID: "two", Name: "second"}})
	if len(got) != 2 || strings.Join([]string{got[0].ID, got[1].ID}, ",") != "one,two" {
		t.Fatalf("dedupe = %#v", got)
	}
}
