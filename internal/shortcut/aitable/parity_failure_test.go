// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParityAppFailureBoundaries(t *testing.T) {
	fail := upsertByKeyStep{err: fmt.Errorf("upstream unavailable")}
	created := parityStep(map[string]any{"app": map[string]any{"appId": "a"}, "created": true})
	cases := []struct {
		name, cmd string
		args      []string
		steps     []upsertByKeyStep
	}{
		{"invalid-json", "+app-block-create", []string{"--page-id", "p", "--config", "{", "--layout", `{"x":0,"y":0,"w":1,"h":1}`}, nil},
		{"persistent-meta", "+app-block-create", []string{"--page-id", "p", "--config", `{"chartType":"AI_ANALYZE","schemaVersion":2}`, "--layout", `{"x":0,"y":0,"w":1,"h":1}`}, nil},
		{"bad-layout", "+app-block-create", []string{"--page-id", "p", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":49,"h":1}`}, nil},
		{"missing-type", "+app-block-create", []string{"--page-id", "p", "--config", `{}`, "--layout", `{"x":0,"y":0,"w":1,"h":1}`}, nil},
		{"empty-name", "+app-page-create", []string{"--name", " "}, nil},
		{"delete-preflight-failure", "+app-block-delete", []string{"--page-id", "p", "--widget-id", "w"}, []upsertByKeyStep{fail}},
		{"delete-wrong-parent", "+app-block-delete", []string{"--page-id", "p", "--widget-id", "w"}, []upsertByKeyStep{parityStep(map[string]any{"widgetId": "w", "pageId": "other"})}},
		{"call-failure", "+app-get", nil, []upsertByKeyStep{fail}},
		{"missing-app-id", "+app-get", nil, []upsertByKeyStep{{text: `{}`}}},
		{"app-read-failure", "+app-get", nil, []upsertByKeyStep{created, fail}},
		{"app-still-creating", "+app-get", nil, []upsertByKeyStep{created, created}},
		{"missing-pages", "+app-page-list", nil, []upsertByKeyStep{{text: `{}`}}},
		{"malformed-page", "+app-page-list", nil, []upsertByKeyStep{{text: `{"pages":[{}]}`}}},
		{"duplicate-pages", "+app-page-list", nil, []upsertByKeyStep{{text: `{"pages":[{"pageId":"p"},{"pageId":"p"}]}`}}},
		{"wrong-page", "+app-page-get", []string{"--page-id", "p"}, []upsertByKeyStep{{text: `{"pageId":"other"}`}}},
		{"delete-read-fails", "+app-page-delete", []string{"--page-id", "p"}, []upsertByKeyStep{{text: `{"deletedPageId":"p"}`}, fail}},
		{"delete-read-missing", "+app-page-delete", []string{"--page-id", "p"}, []upsertByKeyStep{{text: `{"deletedPageId":"p"}`}, {text: `{}`}}},
		{"delete-read-bad-item", "+app-page-delete", []string{"--page-id", "p"}, []upsertByKeyStep{{text: `{"deletedPageId":"p"}`}, {text: `{"pages":[{}]}`}}},
		{"create-read-fails", "+app-page-create", []string{"--name", "N"}, []upsertByKeyStep{{text: `{"pageId":"p"}`}, fail}},
		{"create-read-wrong-id", "+app-page-create", []string{"--name", "N"}, []upsertByKeyStep{{text: `{"pageId":"p"}`}, {text: `{"pageId":"q","pageName":"N"}`}}},
		{"widget-config-mismatch", "+app-block-update", []string{"--page-id", "p", "--widget-id", "w", "--config", `{"chartType":"AI_ANALYZE"}`}, []upsertByKeyStep{{text: `{"widgetId":"w"}`}, {text: `{"widgetId":"w","pageId":"p","config":{}}`}}},
		{"widget-read-wrong-parent", "+app-block-get", []string{"--page-id", "p", "--widget-id", "w"}, []upsertByKeyStep{{text: `{"widgetId":"w","pageId":"q"}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &upsertByKeyCaller{steps: tc.steps}
			out, err := runAITableCompositeCLI(t, c, tc.cmd, append([]string{"--base-id", "b", "--yes"}, tc.args...)...)
			if err == nil || out != "" {
				t.Fatal("false success", out, err)
			}
			if len(c.calls) != len(tc.steps) {
				t.Fatal("unexpected call boundary", len(c.calls), len(tc.steps))
			}
		})
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+app-page-create", "--base-id", "b", "--name", "N", "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err, c.calls)
	}
}

func TestCrossPlatformCoverageParityWorkflowIncompleteNeverSucceeds(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		steps []upsertByKeyStep
	}{
		{[]string{"--limit", "0"}, nil}, {[]string{"--offset", "-1"}, nil}, {[]string{"--page-limit", "0"}, nil},
		{[]string{"--all"}, []upsertByKeyStep{{err: fmt.Errorf("offline")}}},
		{[]string{"--all"}, []upsertByKeyStep{{text: `{"workflows":[{"workflowId":"a"},{"workflowId":"a"}]}`}}},
		{[]string{"--all", "--status", "enabled"}, []upsertByKeyStep{{text: `{"workflows":[{"workflowId":"a","status":"mystery"}],"hasMore":false}`}}},
		{[]string{"--all"}, []upsertByKeyStep{{text: `{"workflows":[],"hasMore":true}`}}},
		{[]string{"--all", "--page-limit", "1"}, []upsertByKeyStep{{text: `{"workflows":[{"workflowId":"a"}],"hasMore":true}`}}},
	} {
		c := &upsertByKeyCaller{steps: tc.steps}
		out, err := runAITableCompositeCLI(t, c, "+workflow-list", append([]string{"--base-id", "b"}, tc.args...)...)
		if err == nil || out != "" {
			t.Fatal(out, err, tc.args)
		}
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"workflows":[{"workflowId":"a"}],"hasMore":true}`}}}
	out, err := runAITableCompositeCLI(t, c, "+workflow-list", "--base-id", "b")
	if err != nil || !strings.Contains(out, `"nextOffset": 1`) {
		t.Fatal(out, err)
	}
	for _, v := range []bool{true, false} {
		if got, known := workflowEnabled(v); !known || got != v {
			t.Fatal(got, known)
		}
	}
	if _, known := workflowEnabled(5); known {
		t.Fatal("unknown status inferred")
	}
}
func TestCrossPlatformCoverageParityResolversAndNodeErrors(t *testing.T) {
	for _, kind := range []string{"field", "view"} {
		key, idKey, nameKey := "fields", "fieldId", "fieldName"
		if kind == "view" {
			key, idKey, nameKey = "views", "viewId", "viewName"
		}
		for _, empty := range []bool{false, true} {
			list := []any{}
			if !empty {
				list = append(list, map[string]any{idKey: "id", nameKey: "Name"})
			}
			c := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{key: list})}}
			out, err := runAITableCompositeCLI(t, c, "+resolve-"+kind, "--base-id", "b", "--table-id", "t", "--name", "Name")
			if (err != nil) != empty || (!empty && !strings.Contains(out, `"id": "id"`)) {
				t.Fatal(out, err)
			}
		}
	}
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: fmt.Errorf("offline")}}}
	if _, err := runAITableCompositeCLI(t, c, "+base-block-list", "--base-id", "b", "--type", "sheet"); err == nil {
		t.Fatal("ignored error")
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"items":[{"nodeId":"a","nodeType":"sheet","parentSectionId":"other"}]}`}}}
	out, err := runAITableCompositeCLI(t, c, "+base-block-list", "--base-id", "b", "--parent-id", "p")
	if err != nil || !strings.Contains(out, `"count": 0`) {
		t.Fatal(out, err)
	}
}
