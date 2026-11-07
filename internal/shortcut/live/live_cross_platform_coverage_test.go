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
	if ListMyLives.OutputRollout != output.RolloutLegacyOnly || ListMyLives.Contract.Result != nil || strings.TrimSpace(ListMyLives.Safety.Effect) == "" || ListMyLives.Contract.Interface == nil || ListMyLives.Contract.Interface.Availability != "unavailable" {
		t.Fatalf("Live unavailable contract drift: rollout=%q result=%v interface=%#v", ListMyLives.OutputRollout, ListMyLives.Contract.Result, ListMyLives.Contract.Interface)
	}
}

func TestCrossPlatformCoverageLivePrimitiveMatrices(t *testing.T) {
	for index, test := range []struct {
		value any
		want  bool
	}{
		{nil, false}, {"", false}, {"0", false}, {"success", false}, {"bad", true},
		{float64(0), false}, {float64(1), true}, {0, false}, {1, true}, {false, false}, {true, true},
		{map[string]any{}, false}, {map[string]any{"x": 1}, true}, {[]any{}, false}, {[]any{1}, true}, {struct{}{}, true},
	} {
		if got := liveFailureValue(test.value); got != test.want {
			t.Errorf("failure matrix %d = %v, want %v", index, got, test.want)
		}
	}
	for index, test := range []struct {
		value any
		want  int
		ok    bool
	}{
		{float64(7), 7, true}, {float64(-1), 0, false}, {float64(7.5), 0, false},
		{8, 8, true}, {-1, -1, false}, {json.Number("9"), 9, true}, {json.Number("bad"), 0, false}, {"9", 0, false},
	} {
		got, ok := liveNonNegativeInt(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("integer matrix %d = (%d,%v), want (%d,%v)", index, got, ok, test.want, test.ok)
		}
	}
	for _, field := range []string{"liveId", "liveUuid", "uuid", "id"} {
		if got := liveStableID(map[string]any{field: " stable "}); got != "stable" {
			t.Errorf("stable ID field %s = %q", field, got)
		}
	}
}
