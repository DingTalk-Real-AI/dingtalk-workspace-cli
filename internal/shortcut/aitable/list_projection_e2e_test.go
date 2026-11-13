// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAITableBaseListAndSearchPreserveNavigationE2E(t *testing.T) {
	const baseURL = "https://alidocs.dingtalk.com/i/nodes/%62ase?z=%2f&z=%3D&a=%25%2b"
	links := []any{
		map[string]any{
			"objectType": "base", "objectId": "base", "name": "Base", "relation": "target",
			"url":   "https://alidocs.dingtalk.com/i/nodes/%62ase?b=%3d&a=%2F&b=%2525",
			"extra": map[string]any{"label": "保留扩展字段"},
		},
		map[string]any{
			"objectType": "folder", "objectId": "folder", "relation": "parent",
			"url": "https://alidocs.dingtalk.com/i/desktop/folders/folder?from=%2f%3D",
		},
	}
	for _, command := range []struct {
		name, tool string
		args       []string
	}{
		{name: "+base-list", tool: "list_bases"},
		{name: "+base-search", tool: "search_bases", args: []string{"--query", "Base"}},
	} {
		t.Run(command.name, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				navigation map[string]any
			}{
				{name: "url only", navigation: map[string]any{"url": baseURL}},
				{name: "links only", navigation: map[string]any{"links": links}},
				{name: "url and links", navigation: map[string]any{"url": baseURL, "links": links}},
				{name: "missing stays missing"},
				{name: "explicit null", navigation: map[string]any{"url": nil, "links": nil}},
				{name: "explicit empty", navigation: map[string]any{"url": "", "links": []any{}}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					base := map[string]any{"baseId": "base", "baseName": "Base"}
					for key, value := range tc.navigation {
						base[key] = value
					}
					// A neighboring Base must not inherit another Base's links or a generated URL.
					wantBases := []map[string]any{base, {"baseId": "other", "baseName": "Other"}}
					payload := map[string]any{"bases": wantBases, "hasMore": true, "nextCursor": "next%2Fpage"}
					if command.name == "+base-search" {
						payload = map[string]any{"data": payload}
					}
					encoded, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: string(encoded)}}}
					out, err := runAITableCompositeCLI(t, caller, command.name, command.args...)
					if err != nil || len(caller.calls) != 1 {
						t.Fatalf("base command = output:%q err:%v calls:%#v", out, err, caller.calls)
					}
					if caller.calls[0].product != "aitable" || caller.calls[0].tool != command.tool {
						t.Fatalf("base command called wrong tool: %#v", caller.calls)
					}
					var got struct {
						Bases      []map[string]any `json:"bases"`
						Count      int              `json:"count"`
						HasMore    bool             `json:"hasMore"`
						NextCursor string           `json:"nextCursor"`
					}
					if err := json.Unmarshal([]byte(out), &got); err != nil {
						t.Fatal(err, out)
					}
					if !reflect.DeepEqual(got.Bases, wantBases) {
						t.Fatalf("base navigation changed: got=%#v want=%#v output=%s", got.Bases, wantBases, out)
					}
					if got.Count != len(wantBases) || !got.HasMore || got.NextCursor != "next%2Fpage" {
						t.Fatalf("base page metadata changed: %s", out)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageAITableListProjectionExplicitEmptyIsSuccessE2E(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		payload string
	}{
		{name: "base list", command: "+base-list", payload: `{"bases":[]}`},
		{name: "base search", command: "+base-search", args: []string{"--query", "none"}, payload: `{"data":{"bases":[]}}`},
		{name: "template search", command: "+template-search", payload: `{"data":{"templates":[]}}`},
		{name: "view get", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"views":[]}`},
		{name: "form list", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"data":{"formViews":[]}}`},
		{name: "workflow list", command: "+workflow-list", args: []string{"--base-id", "base"}, payload: `{"data":{"list":[]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: test.payload}}}
			out, err := runAITableCompositeCLI(t, caller, test.command, test.args...)
			if err != nil || !strings.Contains(out, `"count": 0`) || len(caller.calls) != 1 {
				t.Fatalf("explicit empty list = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableBaseSearchPreservesPaginationFactsE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"bases":[],"hasMore":true,"nextCursor":"next-page"}}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+base-search", "--query", "none")
	if err != nil || !strings.Contains(out, `"hasMore": true`) || !strings.Contains(out, `"nextCursor": "next-page"`) {
		t.Fatalf("base search pagination = output:%q err:%v", out, err)
	}

	caller = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"bases":[],"hasMore":false,"nextCursor":"stale"}}`}}}
	out, err = runAITableCompositeCLI(t, caller, "+base-search", "--query", "none")
	if err != nil || !strings.Contains(out, `"hasMore": false`) || strings.Contains(out, `"nextCursor"`) {
		t.Fatalf("base search terminal pagination = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageAITableListProjectionUnknownIsNotEmptySuccessE2E(t *testing.T) {
	commands := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "base list", command: "+base-list"},
		{name: "base search", command: "+base-search", args: []string{"--query", "none"}},
		{name: "template search", command: "+template-search"},
		{name: "view get", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}},
		{name: "form list", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}},
		{name: "workflow list", command: "+workflow-list", args: []string{"--base-id", "base"}},
	}
	payloads := []struct {
		name string
		text string
	}{
		{name: "empty response", text: ""},
		{name: "missing collection", text: `{"success":true}`},
		{name: "wrong collection type", text: `{"data":{"list":{}}}`},
	}
	for _, command := range commands {
		for _, payload := range payloads {
			t.Run(command.name+"/"+payload.name, func(t *testing.T) {
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: payload.text}}}
				out, err := runAITableCompositeCLI(t, caller, command.command, command.args...)
				if err == nil || out != "" || len(caller.calls) != 1 {
					t.Fatalf("unknown list result was accepted = output:%q err:%v calls:%#v", out, err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAITableListProjectionMalformedItemIsNotSilentlyDroppedE2E(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		payload string
	}{
		{name: "base without id", command: "+base-list", payload: `{"bases":[{"baseName":"orphan"}]}`},
		{name: "template scalar", command: "+template-search", payload: `{"templates":["bad"]}`},
		{name: "view without id", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"views":[{"viewName":"orphan"}]}`},
		{name: "form without id", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"forms":[{"viewName":"orphan"}]}`},
		{name: "workflow without id", command: "+workflow-list", args: []string{"--base-id", "base"}, payload: `{"workflows":[{"name":"orphan"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: test.payload}}}
			out, err := runAITableCompositeCLI(t, caller, test.command, test.args...)
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("malformed list item was accepted = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}
