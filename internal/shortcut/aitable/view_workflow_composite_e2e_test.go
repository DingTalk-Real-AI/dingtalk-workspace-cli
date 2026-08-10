// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const workflowDSLFixture = `{"version":"workflow-dsl/v1","name":"提醒","nodes":[]}`

func TestCrossPlatformCoverageViewPresetCreateUpdateAndVerificationE2E(t *testing.T) {
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

func TestCrossPlatformCoverageViewPresetAmbiguousOrMismatchedIsNotSuccessE2E(t *testing.T) {
	t.Run("duplicate name stops before write", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"views":[{"viewId":"v1","viewName":"X"},{"viewId":"v2","viewName":"X"}]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
			"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
		if err == nil || out != "" || len(caller.calls) != 1 {
			t.Fatalf("ambiguous view = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("read-back config mismatch is unknown", func(t *testing.T) {
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
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_unknown" || typed.Retryable {
			t.Fatalf("mismatched view error = %#v", err)
		}
	})
}

func TestCrossPlatformCoverageWorkflowDeployCreateEnableAndVerifyE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"valid":true,"flowId":"w1","issues":[]}}`},
		{text: `{"data":{"flowId":"w1","flowSchema":{"name":"提醒"}}}`},
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

func TestCrossPlatformCoverageWorkflowDeployUpdateAndInvalidResponsesE2E(t *testing.T) {
	t.Run("update preflights and reads back", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"flowId":"w1","flowSchema":{}}`},
			{text: `{"valid":true,"flowId":"w1"}`},
			{text: `{"flowId":"w1","flowSchema":{"name":"提醒"}}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--workflow-id", "w1", "--dsl", workflowDSLFixture, "--yes")
		if err != nil || !strings.Contains(out, `"action": "update"`) || len(caller.calls) != 3 || caller.calls[1].tool != "update_workflow" {
			t.Fatalf("workflow update = output:%q err:%v calls:%#v", out, err, caller.calls)
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
			{text: `{"valid":true,"flowId":"w1"}`}, {text: `{"flowId":"w1"}`},
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
