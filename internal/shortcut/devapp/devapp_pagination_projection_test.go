// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package devapp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type paginationCaller struct{ text string }

func (c *paginationCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}
func (*paginationCaller) Format() string { return "json" }
func (*paginationCaller) DryRun() bool   { return false }
func (*paginationCaller) Fields() string { return "" }
func (*paginationCaller) JQ() string     { return "" }

func TestDevAppListProjectionPreservesPaginationEvidence(t *testing.T) {
	items := []map[string]any{{"id": "one"}}
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "top level", data: map[string]any{"success": true, "hasMore": true, "nextCursor": "next"}},
		{name: "nested result", data: map[string]any{"success": true, "result": map[string]any{"hasMore": true, "nextCursor": "next"}}},
		{name: "exhausted", data: map[string]any{"success": true, "hasMore": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := devAppListProjection(tc.data, "items", items, "devapp/test")
			if err != nil {
				t.Fatal(err)
			}
			if got["hasMore"] != devAppPaginationCandidates(tc.data)[len(devAppPaginationCandidates(tc.data))-1]["hasMore"] && tc.name == "nested result" {
				t.Fatalf("hasMore not preserved: %#v", got)
			}
			if tc.name != "exhausted" && got["nextCursor"] != "next" {
				t.Fatalf("nextCursor not preserved: %#v", got)
			}
			if got["count"] != 1 {
				t.Fatalf("projection count=%#v", got["count"])
			}
		})
	}
	if got := devAppPaginationCandidates(nil); got != nil {
		t.Fatalf("nil candidates=%#v", got)
	}
	deep := map[string]any{"content": map[string]any{"success": true, "result": map[string]any{"hasMore": true, "nextCursor": "deep"}}}
	if got, err := devAppListProjection(deep, "items", nil, "devapp/test"); err != nil || got["nextCursor"] != "deep" {
		t.Fatalf("deep projection=%#v", got)
	}
	deeper := map[string]any{"success": true, "content": map[string]any{"result": map[string]any{"data": map[string]any{"hasMore": true, "nextCursor": "deeper"}}}}
	if got, err := devAppListProjection(deeper, "items", nil, "devapp/test"); err != nil || got["nextCursor"] != "deeper" {
		t.Fatalf("deeper projection=%#v", got)
	}
	cyclic := map[string]any{"success": true}
	cyclic["content"] = cyclic
	if got := devAppPaginationCandidates(cyclic); len(got) != 13 {
		t.Fatalf("cyclic candidate walk length=%d, want bounded length 13", len(got))
	}
}

func TestFrameworkPaginatedShortcutExecutionMovesCursorToMeta(t *testing.T) {
	caller := &paginationCaller{text: `{"content":{"success":true,"result":{"hasMore":true,"nextCursor":"next","list":[]}}}`}
	helpers.InitDepsForTest(t, caller)
	helpers.GetFormatter().SetWriters(&bytes.Buffer{}, &bytes.Buffer{})
	for _, tc := range []struct {
		declaration shortcut.Shortcut
		args        []string
	}{
		{frameworkUnified(ListApp), nil},
		{frameworkUnified(PermissionList), []string{"--unified-app-id", "app"}},
		{frameworkUnified(EventList), []string{"--unified-app-id", "app"}},
		{frameworkUnified(VersionList), []string{"--unified-app-id", "app"}},
	} {
		cmd := corecmd.New(shortcut.FromShortcut(tc.declaration))
		ctx, _ := output.WithResultStore(context.Background())
		cmd.SetContext(ctx)
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s: %v", tc.declaration.Command, err)
		}
		code, emitted, err := output.EmitStoredResult(cmd)
		if err != nil || !emitted || code != 0 {
			t.Fatalf("%s emit: code=%d emitted=%v err=%v stderr=%s", tc.declaration.Command, code, emitted, err, stderr.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
			Meta struct {
				Pagination *struct {
					EndpointExhausted bool   `json:"endpoint_exhausted"`
					NextToken         string `json:"next_token"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("%s output is not JSON: %v\n%s", tc.declaration.Command, err, stdout.String())
		}
		if envelope.Meta.Pagination == nil || envelope.Meta.Pagination.EndpointExhausted || envelope.Meta.Pagination.NextToken != "next" {
			t.Fatalf("%s pagination=%+v output=%s", tc.declaration.Command, envelope.Meta.Pagination, stdout.String())
		}
		if _, exists := envelope.Data["hasMore"]; exists {
			t.Fatalf("%s data leaked hasMore: %s", tc.declaration.Command, stdout.String())
		}
		if _, exists := envelope.Data["nextCursor"]; exists {
			t.Fatalf("%s data leaked nextCursor: %s", tc.declaration.Command, stdout.String())
		}
	}
}
