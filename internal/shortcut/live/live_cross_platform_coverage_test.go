// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package live

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type liveCaller struct{ calls int }

func (caller *liveCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"result":{"total":0,"hasFinish":true,"liveDetailModelList":[]}}`}}}, nil
}
func (*liveCaller) Format() string { return "json" }
func (*liveCaller) DryRun() bool   { return false }
func (*liveCaller) Fields() string { return "" }
func (*liveCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageLiveListStrictProjection(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{"total": float64(1), "hasFinish": true, "liveDetailModelList": []any{map[string]any{"liveId": "stable-live"}}}}
	projected, err := projectLiveList(valid)
	if err != nil || projected["count"] != 1 {
		t.Fatalf("valid list rejected: projected=%v err=%v", projected, err)
	}
	empty, err := projectLiveList(map[string]any{"success": true, "result": map[string]any{"total": float64(0), "hasFinish": true, "liveDetailModelList": []any{}}})
	if err != nil || empty["count"] != 0 {
		t.Fatalf("explicit zero rejected: projected=%v err=%v", empty, err)
	}

	broken := []map[string]any{
		{},
		{"result": map[string]any{}},
		{"success": false, "result": map[string]any{}},
		{"success": true, "errorCode": "FAILED", "result": map[string]any{"total": float64(0), "hasFinish": true, "liveDetailModelList": []any{}}},
		{"success": true},
		{"success": true, "result": []any{}},
		{"success": true, "result": map[string]any{"total": float64(0), "hasFinish": true}},
		{"success": true, "result": map[string]any{"total": "0", "hasFinish": true, "liveDetailModelList": []any{}}},
		{"success": true, "result": map[string]any{"total": float64(0), "hasFinish": "true", "liveDetailModelList": []any{}}},
		{"success": true, "result": map[string]any{"total": float64(0), "hasFinish": false, "liveDetailModelList": []any{}}},
		{"success": true, "result": map[string]any{"total": float64(4), "hasFinish": true, "liveDetailModelList": []any{}}},
		{"success": true, "result": map[string]any{"total": float64(1), "hasFinish": true, "liveDetailModelList": []any{"bad"}}},
		{"success": true, "result": map[string]any{"total": float64(1), "hasFinish": true, "liveDetailModelList": []any{map[string]any{"title": "no-id"}}}},
		{"success": true, "result": map[string]any{"total": float64(2), "hasFinish": true, "liveDetailModelList": []any{map[string]any{"liveId": "duplicate"}, map[string]any{"liveId": "duplicate"}}}},
	}
	for index, data := range broken {
		if got, projectErr := projectLiveList(data); projectErr == nil {
			t.Errorf("broken response %d returned success: %v", index, got)
		}
	}
}

func TestCrossPlatformCoverageUnavailableLiveMakesNoRemoteCall(t *testing.T) {
	caller := &liveCaller{}
	helpers.InitDepsForTest(t, caller)
	err := ListMyLives.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: ListMyLives.Command}, ListMyLives))
	if err == nil || !strings.Contains(err.Error(), "cannot prove") {
		t.Fatalf("unavailable error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("unavailable Live shortcut made %d remote calls", caller.calls)
	}
}

func TestCrossPlatformCoverageLiveContract(t *testing.T) {
	if ListMyLives.OutputRollout != output.RolloutUnifiedActive || ListMyLives.Contract.Result == nil || strings.TrimSpace(ListMyLives.Safety.Effect) == "" {
		t.Fatal("Live shortcut lacks Contract/Result/Safety/unified output")
	}
	var schema map[string]any
	if err := json.Unmarshal(ListMyLives.Contract.Result.DataSchema, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("invalid Result schema: schema=%v err=%v", schema, err)
	}
}
