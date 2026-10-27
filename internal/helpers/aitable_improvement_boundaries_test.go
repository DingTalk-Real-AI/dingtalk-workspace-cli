// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAitableImprovementInputBoundaries(t *testing.T) {
	for _, types := range []string{strings.Repeat("LINE,", 20) + "PIE", "LINE,LINE", "line"} {
		caller := &aitableAppCaller{}
		if err := executeAitableImprovementForTest(t, caller, "chart", "templates", "--chart-types", types); err == nil || len(caller.calls) != 0 {
			t.Fatal("invalid types dispatched")
		}
	}
	for _, config := range []string{`{"chartType":"LINE","schemaVersion":2}`, `{"chartType":"LINE"}`} {
		layout := `{"x":0,"y":0,"w":1,"h":1}`
		if !strings.Contains(config, "schemaVersion") {
			layout = `{"x":0,"y":0,"w":1,"h":1,"schemaVersion":2}`
		}
		caller := &aitableAppCaller{}
		if err := executeAitableImprovementForTest(t, caller, "chart", "batch-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":`+config+`,"layout":`+layout+`}]`, "--yes"); err == nil || len(caller.calls) != 0 {
			t.Fatal("metadata persisted")
		}
	}
	for _, items := range [][]map[string]any{
		{{"taskId": "", "fieldId": "F", "recordIds": []any{"R"}}},
		{{"taskId": "TASK", "fieldId": "F", "recordIds": []any{"R", "R"}}},
		{{"taskId": "TASK", "fieldId": "F", "recordIds": []any{false}}},
	} {
		if err := validateAitableTaskTargets(items); err == nil {
			t.Fatal("invalid task accepted")
		}
	}
	if err := validateAitableTaskTargets([]map[string]any{{"taskId": "TASK", "fieldId": "F", "recordIds": []any{"R"}}}); err != nil {
		t.Fatal(err)
	}
}
func TestCrossPlatformCoverageAitableImprovementReceiptBoundaries(t *testing.T) {
	caller := &aitableAppCaller{response: `{"data":{"PIE":{"chartType":"PIE"}}}`}
	InitDepsForTest(t, caller)
	cmd := &cobra.Command{}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if _, err := callAitableImprovementResult(cmd, "get_dashboard_widget_templates", map[string]any{"chartTypes": []string{"LINE"}}); err == nil {
		t.Fatal("missing template accepted")
	}
	body := map[string]any{"status": "error", "error": map[string]any{"code": "CREATE_CHART_FAILED", "details": map[string]any{"mutationSubmitted": true}}}
	caller.response = `{"status":"error","error":{"code":"CREATE_CHART_FAILED","details":{"mutationSubmitted":true}}}`
	if err := CallMCPToolOnServer("aitable", "create_chart", nil); err == nil {
		t.Fatal("mutation error rendered as success")
	}
	if classifyAitableMutationError(body) == nil {
		t.Fatal("known mutation evidence lost")
	}
	if safeAitableChartCandidates(map[string]any{"baseId": "B", "dashboardId": "D", "results": []any{false}}) != nil {
		t.Fatal("malformed candidate projected")
	}
}

type boundaryDryAitableCaller struct{ aitableAppCaller }

func (*boundaryDryAitableCaller) DryRun() bool { return true }
func TestCrossPlatformCoverageAitableImprovementDryRun(t *testing.T) {
	caller := &boundaryDryAitableCaller{}
	InitDepsForTest(t, caller)
	if _, err := callAitableImprovementResult(&cobra.Command{}, "create_charts", map[string]any{"baseId": "B"}); err != nil || len(caller.calls) != 0 {
		t.Fatalf("dry-run called remote: %v %#v", err, caller.calls)
	}
}
