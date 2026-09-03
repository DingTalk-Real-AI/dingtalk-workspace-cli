// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const workflowDSLFixture = `{"version":"workflow-dsl/v1","name":"提醒","nodes":[]}`

func disableViewPresetSleep(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &viewPresetSleep, func(time.Duration) {})
}

func TestCrossPlatformCoverageViewPresetCreateUpdateAndVerificationE2E(t *testing.T) {
	disableViewPresetSleep(t)
	t.Run("create and verify exact config", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`},
			{text: `{"data":{"viewId":"v1"}}`},
			{text: `{"views":[{"viewId":"v1","viewName":"待处理","viewType":"Grid","config":{"visibleFieldIds":["f1"],"extra":true}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "待处理", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
		if err != nil || !strings.Contains(out, `"viewId": "v1"`) || !strings.Contains(out, `"status": "verified"`) {
			t.Fatalf("view preset create = output:%q err:%v", out, err)
		}
		if len(caller.calls) != 3 || caller.calls[1].tool != "create_view" {
			t.Fatalf("view create calls = %#v", caller.calls)
		}
	})

	t.Run("normalizes live filter and sort projection", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`},
			{text: `{"data":{"viewId":"v-live"}}`},
			{text: `{"views":[{"viewId":"v-live","viewName":"线上形态","viewType":"Grid","filter":{"operator":"and","operands":[]},"sort":[{"fieldId":"f1","direction":"asc"}]}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "线上形态", "--view-type", "Grid",
			"--config", `{"filter":[],"sort":[{"fieldId":"f1","direction":"asc"}]}`, "--yes")
		if err != nil || !strings.Contains(out, `"status": "verified"`) || !strings.Contains(out, `"viewId": "v-live"`) {
			t.Fatalf("live view projection = output:%q err:%v", out, err)
		}
	})

	t.Run("projected columns and hidden flags verify visible field config", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`},
			{text: `{"data":{"viewId":"v1"}}`},
			{text: `{"views":[{"viewId":"v1","viewName":"投影","viewType":"Grid","columns":["f1","f2","f3"],"custom":{"hiddenFields":[false,true,false]}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "投影", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1","f3"]}`, "--yes")
		if err != nil || !strings.Contains(out, `"status": "verified"`) {
			t.Fatalf("projected view preset = output:%q err:%v", out, err)
		}
	})

	t.Run("projected columns and keyed hidden flags verify visible field config", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`},
			{text: `{"data":{"viewId":"v1"}}`},
			{text: `{"views":[{"viewId":"v1","viewName":"投影对象","viewType":"Grid","columns":["f1","f2","f3"],"custom":{"hiddenFields":{"f1":true,"f2":false,"f3":false}}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "投影对象", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f2","f3"]}`, "--yes")
		if err != nil || !strings.Contains(out, `"status": "verified"`) {
			t.Fatalf("keyed projected view preset = output:%q err:%v", out, err)
		}
	})

	t.Run("unchanged preset makes no write", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"views":[{"viewId":"v1","viewName":"待处理","viewType":"Grid","config":{"visibleFieldIds":["f1"]}}]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "待处理", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
		if err != nil || len(caller.calls) != 1 || !strings.Contains(out, `"status": "unchanged"`) || !strings.Contains(out, `"executed": false`) {
			t.Fatalf("unchanged view = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("empty write response recovers only from read-back", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`}, {text: ""},
			{text: `{"views":[{"viewId":"v2","viewName":"看板","viewType":"Kanban","config":{"groupFieldId":"f1"}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "看板", "--view-type", "Kanban", "--config", `{"groupFieldId":"f1"}`, "--yes")
		if err != nil || !strings.Contains(out, `"status": "recovered"`) {
			t.Fatalf("recovered view = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageViewPresetRetriesUntilTerminalStateE2E(t *testing.T) {
	disableViewPresetSleep(t)
	steps := []upsertByKeyStep{
		{text: `{"views":[]}`},
		{text: `{"data":{"viewId":"v1"}}`},
	}
	for attempt := 0; attempt < viewPresetReadbackAttempts-1; attempt++ {
		steps = append(steps, upsertByKeyStep{text: `{"views":[]}`})
	}
	steps = append(steps, upsertByKeyStep{text: `{"views":[{"viewId":"v1","viewName":"X","viewType":"Grid","config":{"visibleFieldIds":["f1"]}}]}`})
	caller := &upsertByKeyCaller{steps: steps}
	out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
		"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) || len(caller.calls) != viewPresetReadbackAttempts+2 {
		t.Fatalf("eventual view preset = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageViewPresetAmbiguousOrMismatchedIsNotSuccessE2E(t *testing.T) {
	disableViewPresetSleep(t)
	t.Run("duplicate name stops before write", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"views":[{"viewId":"v1","viewName":"X"},{"viewId":"v2","viewName":"X"}]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
		if err == nil || out != "" || len(caller.calls) != 1 {
			t.Fatalf("ambiguous view = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("read-back config mismatch is partial when create ID is known", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"views":[]}`}, {text: `{"viewId":"v1"}`},
			{text: `{"views":[{"viewId":"v1","viewName":"X","viewType":"Grid","config":{"visibleFieldIds":["wrong"]}}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
		if err == nil || out != "" {
			t.Fatalf("mismatched view = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_partial_success" || typed.Retryable {
			t.Fatalf("mismatched view error = %#v", err)
		}
	})
}

func TestCrossPlatformCoverageViewPresetNormalizationHelpers(t *testing.T) {
	if !presetViewMatches(
		map[string]any{"viewType": "Grid"},
		"Grid",
		map[string]any{"filter": []any{}, "sort": []any{}, "group": []any{}},
	) {
		t.Fatal("missing persisted empty filter/sort/group should match requested empties")
	}
	normalized := normalizePresetViewConfig(map[string]any{"sort": nil, "group": nil, "other": "value"})
	if sortItems, ok := normalized["sort"].([]any); !ok || len(sortItems) != 0 {
		t.Fatalf("normalized nil sort = %#v", normalized["sort"])
	}
	if groupItems, ok := normalized["group"].([]any); !ok || len(groupItems) != 0 || normalized["other"] != "value" {
		t.Fatalf("normalized nil group/default = %#v", normalized)
	}
	if filter, ok := normalizePresetViewFilter(nil).(map[string]any); !ok || filter["operator"] != "and" {
		t.Fatalf("normalized nil filter = %#v", filter)
	}
	group := map[string]any{"operator": "and", "operands": []any{map[string]any{"fieldId": "f1"}}}
	if filter, ok := normalizePresetViewFilter([]any{group}).(map[string]any); !ok || filter["operator"] != "and" {
		t.Fatalf("normalized single filter group = %#v", filter)
	}
	if items, ok := normalizePresetViewFilter([]any{"not-a-group"}).([]any); !ok || len(items) != 1 {
		t.Fatalf("non-group filter list changed = %#v", items)
	}
	if !emptyPresetViewConfigValue("sort", []any{}) || emptyPresetViewConfigValue("sort", "bad") {
		t.Fatal("sort empty equivalence mismatch")
	}
	if !emptyPresetViewConfigValue("filter", map[string]any{"operator": "and", "operands": []any{}}) {
		t.Fatal("empty filter group was not recognized")
	}
	if emptyPresetViewConfigValue("filter", map[string]any{"operator": "or", "operands": []any{}}) ||
		emptyPresetViewConfigValue("filter", map[string]any{"operator": "and", "operands": "bad"}) ||
		emptyPresetViewConfigValue("other", nil) {
		t.Fatal("non-empty preset config was accepted as empty")
	}
}

func TestCrossPlatformCoverageWorkflowDeployCreateEnableAndVerifyE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"valid":true,"flowId":"w1","issues":[]}}`},
		{text: `{"data":{"name":"提醒","flowSchema":{"name":"提醒"}}}`},
		{text: `{"workflowId":"w1","enabled":true}`},
		{text: `{"data":{"list":[{"flowId":"w1","name":"提醒","status":"RUNNING"}]}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
	if err != nil {
		t.Fatalf("workflow deploy error = %v", err)
	}
	for _, want := range []string{`"workflowId": "w1"`, `"valid": true`, `"running": true`, `"status": "verified"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow output missing %s: %s", want, out)
		}
	}
	if len(caller.calls) != 4 || caller.calls[0].product != serverMain || caller.calls[2].product != serverHelper || caller.calls[3].tool != "list_workflows" {
		t.Fatalf("workflow call routing = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWorkflowDeployReportsActualStateWithoutEnableE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"valid":true,"flowId":"w1","issues":[]}}`},
		{text: `{"data":{"name":"提醒","flowSchema":{}}}`},
		{text: `{"data":{"list":[{"flowId":"w1","name":"提醒","status":"RUNNING"}]}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
	if err != nil {
		t.Fatalf("workflow deploy without enable error = %v", err)
	}
	for _, want := range []string{`"enableRequested": false`, `"running": true`, `"workflowStatus": "RUNNING"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow output missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, `"enable": false`) || len(caller.calls) != 3 {
		t.Fatalf("workflow request intent/status projection = output:%q calls:%#v", out, caller.calls)
	}
}

func TestCrossPlatformCoverageWorkflowDeployAcceptsEchoedIDWithoutFlowSchemaE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"valid":true,"flowId":"w1","issues":[]}}`},
		{text: `{"data":{"flowId":"w1","name":"提醒(1)"}}`},
		{text: `{"data":{"list":[{"flowId":"w1","status":"STOP"}]}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) || !strings.Contains(out, `"running": false`) {
		t.Fatalf("workflow echoed-ID detail = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageWorkflowDeployWithoutEnableToleratesStatusReadFailureE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"valid":true,"flowId":"w1","issues":[]}}`},
		{text: `{"data":{"name":"提醒","flowSchema":{}}}`},
		{err: errors.New("list unavailable")},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
	if err != nil || !strings.Contains(out, "workflow status could not be read from list") || strings.Contains(out, `"running"`) {
		t.Fatalf("workflow status fallback = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageWorkflowDeployUpdateAndInvalidResponsesE2E(t *testing.T) {
	t.Run("update preflights and reads back", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"data":{"name":"旧名称","flowSchema":{}}}`},
			{text: `{"valid":true,"flowId":"w1"}`},
			{text: `{"data":{"name":"提醒","flowSchema":{"name":"提醒"}}}`},
			{text: `{"list":[{"flowId":"w1","status":"RUNNING"}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--workflow-id", "w1", "--dsl", workflowDSLFixture, "--yes")
		if err != nil || !strings.Contains(out, `"action": "update"`) || !strings.Contains(out, `"running": true`) || len(caller.calls) != 4 || caller.calls[1].tool != "update_workflow" {
			t.Fatalf("workflow update = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("update accepts echoed ID without flowSchema and normalized name", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"data":{"flowId":"w1","name":"旧名称"}}`},
			{text: `{"valid":true,"flowId":"w1"}`},
			{text: `{"flowId":"w1","name":"提醒(1)"}`},
			{text: `{"list":[{"flowId":"w1","status":"RUNNING"}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--workflow-id", "w1", "--dsl", workflowDSLFixture, "--yes")
		if err != nil || !strings.Contains(out, `"action": "update"`) || !strings.Contains(out, `"running": true`) {
			t.Fatalf("workflow update echoed-ID detail = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("status success with valid false is rejected", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"status":"success","data":{"valid":false,"flowId":"w1","issues":[{"message":"bad dsl"}]}}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
		if err == nil || out != "" || len(caller.calls) != 1 {
			t.Fatalf("invalid workflow = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_rejected" {
			t.Fatalf("invalid workflow error = %#v", err)
		}
	})

	for name, response := range map[string]string{
		"conflicting valid": `{"valid":true,"data":{"valid":false,"flowId":"w1"}}`,
		"conflicting id":    `{"valid":true,"flowId":"w1","data":{"workflowId":"w2"}}`,
		"valid wrong type":  `{"valid":"true","flowId":"w1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
			out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("conflicting workflow response = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "aitable_composite_rejected" || typed.Retryable {
				t.Fatalf("conflicting workflow response error = %#v", err)
			}
		})
	}

	t.Run("empty create response is unknown", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: ""}}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--yes")
		if err == nil || out != "" {
			t.Fatalf("empty workflow = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_unknown" || typed.Retryable {
			t.Fatalf("empty workflow error = %#v", err)
		}
	})

	t.Run("enable reply is not enough when list says STOP", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"valid":true,"flowId":"w1"}`}, {text: `{"flowSchema":{}}`},
			{text: `{"enabled":true,"workflowId":"w1"}`},
			{text: `{"list":[{"flowId":"w1","status":"STOP"}]}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
		if err == nil || out != "" {
			t.Fatalf("stopped workflow = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_partial_success" || typed.Retryable {
			t.Fatalf("stopped workflow error = %#v", err)
		}
	})
}
