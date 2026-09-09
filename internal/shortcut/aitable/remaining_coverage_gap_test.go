// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageAITableProjectionShapeFailures(t *testing.T) {
	if _, err := resolveNamedList("op", nil, []string{"data"}, []string{"items"}); err == nil {
		t.Fatal("nil list response must fail")
	}
	if _, err := resolveNamedList("op", map[string]any{"data": "bad"}, []string{"data"}, []string{"items"}); err == nil {
		t.Fatal("non-object envelope must fail")
	}
	if _, err := resolveNamedList("op", map[string]any{"data": map[string]any{"other": []any{}}}, []string{"data"}, []string{"items"}); err == nil {
		t.Fatal("unrecognized envelope must fail")
	}
	for name, project := range map[string]func() error{
		"base": func() error {
			_, err := baseListProject("list_bases", map[string]any{"bases": []any{"bad"}})
			return err
		},
		"template": func() error {
			_, err := templateSearchProject(map[string]any{"templates": []any{"bad"}})
			return err
		},
		"view": func() error {
			_, err := viewGetProject(map[string]any{"views": []any{"bad"}})
			return err
		},
		"form": func() error {
			_, err := formListProject(map[string]any{"views": []any{"bad"}})
			return err
		},
		"workflow": func() error {
			_, err := workflowListProject(map[string]any{"workflows": []any{"bad"}})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := project(); err == nil {
				t.Fatal("non-object projection item must fail")
			}
		})
	}
}

func TestCrossPlatformCoverageAITableProjectionValidAndMissingIdentityShapes(t *testing.T) {
	bases, err := baseListProject("list_bases", map[string]any{"bases": []any{map[string]any{"baseId": "b", "baseName": "Base"}}})
	if err != nil || len(bases) != 1 {
		t.Fatalf("base projection = %#v, %v", bases, err)
	}
	if _, err := templateSearchProject(map[string]any{"templates": []any{map[string]any{"name": "missing"}}}); err == nil {
		t.Fatal("template missing ID succeeded")
	}
	templates, err := templateSearchProject(map[string]any{"templates": []any{map[string]any{"templateId": "t"}}})
	if err != nil || len(templates) != 1 {
		t.Fatalf("template projection = %#v, %v", templates, err)
	}
	views, err := viewGetProject(map[string]any{"views": []any{map[string]any{"viewId": "v"}}})
	if err != nil || len(views) != 1 {
		t.Fatalf("view projection = %#v, %v", views, err)
	}
	forms, err := formListProject(map[string]any{"views": []any{map[string]any{"viewId": "f"}}})
	if err != nil || len(forms) != 1 {
		t.Fatalf("form projection = %#v, %v", forms, err)
	}
	workflows, err := workflowListProject(map[string]any{"workflows": []any{map[string]any{"workflowId": "w"}}})
	if err != nil || len(workflows) != 1 {
		t.Fatalf("workflow projection = %#v, %v", workflows, err)
	}
	workflows, err = workflowListProject(map[string]any{"list": []any{map[string]any{"flowId": "flow", "name": "real service shape"}}})
	if err != nil || len(workflows) != 1 || workflows[0]["workflowId"] != "flow" {
		t.Fatalf("workflow flowId projection = %#v, %v", workflows, err)
	}
}

func TestCrossPlatformCoverageCompositeAndReviewedContractFallbacks(t *testing.T) {
	err := compositeError(newCompositeResult("test"), errors.New("cause"), true)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("composite default status = %v", err)
	}
	unreviewed := shortcut.Shortcut{Command: "+not-reviewed"}
	if got := withReviewedAITableShortcutContracts(unreviewed); len(got) != 1 || !got[0].Contract.Empty() {
		t.Fatalf("unreviewed shortcut changed: %#v", got)
	}
	value := shortcut.Shortcut{
		Command:     "+base-list",
		Description: "fallback description",
		Tips:        []string{"one", "two", "three"},
	}
	contract := reviewedAITableShortcutContract(value)
	if contract.Description != "fallback description" || len(contract.Selection.Examples) != 2 {
		t.Fatalf("reviewed contract fallback = %#v", contract)
	}
}

func TestCrossPlatformCoverageURLResolveAllVerificationKindsE2E(t *testing.T) {
	cases := []struct {
		name string
		url  string
		tool string
		text string
	}{
		{name: "record", url: "https://alidocs.dingtalk.com/i/nodes/base?iframeQuery=sheetId%3Dtable%26recordId%3Drecord", tool: "query_records", text: `{"records":[{"record_id":"record"}]}`},
		{name: "view", url: "https://alidocs.dingtalk.com/i/nodes/base?iframeQuery=sheetId%3Dtable%26viewId%3Dview", tool: "get_views", text: `{"views":[{"view_id":"view"}]}`},
		{name: "base by id", url: "https://alidocs.dingtalk.com/i/nodes/base", tool: "get_base", text: `{"base_id":"base"}`},
		{name: "base structural fallback", url: "https://alidocs.dingtalk.com/i/nodes/base", tool: "get_base", text: `{"data":{"tables":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(tc.text)}}
			out, err := runURLResolveCLI(t, caller, "--url", tc.url, "--verify")
			if err != nil || !strings.Contains(out, `"verified": true`) || caller.calls[0].tool != tc.tool {
				t.Fatalf("verification = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}

	t.Run("tool error", func(t *testing.T) {
		caller := &urlResolveE2ECaller{errors: []error{errors.New("verification failed")}}
		out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/base", "--verify")
		if err == nil || out != "" {
			t.Fatalf("tool error = output:%q err:%v", out, err)
		}
	})

	t.Run("base unknown response", func(t *testing.T) {
		caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(`{"success":true}`)}}
		out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/base", "--verify")
		if err == nil || out != "" {
			t.Fatalf("unknown base = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageURLResolveRecursiveShapeHelpers(t *testing.T) {
	value := map[string]any{"data": []any{map[string]any{"id": "wanted"}}}
	if !responseContainsID(value, "wanted", "id") || responseContainsID(value, "other", "id") {
		t.Fatal("responseContainsID recursion mismatch")
	}
	if !responseHasAnyKey(map[string]any{"data": []any{map[string]any{"tables": []any{}}}}, "tables") {
		t.Fatal("responseHasAnyKey recursion mismatch")
	}
	if responseHasAnyKey([]any{"value"}, "tables") {
		t.Fatal("unrelated key must not match")
	}
}

func TestCrossPlatformCoverageViewPresetValidationAndDryRunE2E(t *testing.T) {
	cases := []struct {
		name   string
		config string
		steps  []upsertByKeyStep
	}{
		{name: "invalid config", config: `{`},
		{name: "empty config", config: `{}`},
		{name: "preflight error", config: `{"f":1}`, steps: []upsertByKeyStep{{err: errors.New("views failed")}}},
		{name: "preflight missing list", config: `{"f":1}`, steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "matched missing id", config: `{"f":1}`, steps: []upsertByKeyStep{{text: `{"views":[{"viewName":"X","viewType":"Grid"}]}`}}},
		{name: "matched type conflict", config: `{"f":1}`, steps: []upsertByKeyStep{{text: `{"views":[{"viewId":"v","viewName":"X","viewType":"Kanban"}]}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+view-preset-apply",
				"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", tc.config, "--yes")
			if err == nil || out != "" {
				t.Fatalf("validation = output:%q err:%v", out, err)
			}
		})
	}

	caller := &upsertByKeyCaller{dryRun: true, steps: []upsertByKeyStep{{text: `{"views":[]}`}}}
	out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
		"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"f":1}`, "--dry-run", "--yes")
	if err != nil || !strings.Contains(out, `"status": "planned"`) {
		t.Fatalf("dry run = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageViewPresetReadBackFailuresAndUpdateE2E(t *testing.T) {
	disableViewPresetSleep(t)
	preflight := `{"views":[{"viewId":"v","viewName":"X","viewType":"Grid","config":{"f":0}}]}`
	cases := []struct {
		name     string
		write    upsertByKeyStep
		readBack upsertByKeyStep
	}{
		{name: "read error", write: upsertByKeyStep{text: `{"updated":1}`}, readBack: upsertByKeyStep{err: errors.New("read failed")}},
		{name: "missing list", write: upsertByKeyStep{text: `{"updated":1}`}, readBack: upsertByKeyStep{text: `{}`}},
		{name: "missing exact name", write: upsertByKeyStep{text: `{"updated":1}`}, readBack: upsertByKeyStep{text: `{"views":[]}`}},
		{name: "identity mismatch", write: upsertByKeyStep{text: `{"updated":1}`}, readBack: upsertByKeyStep{text: `{"views":[{"viewId":"other","viewName":"X","viewType":"Grid","config":{"f":1}}]}`}},
		{name: "dual failure", write: upsertByKeyStep{err: errors.New("write failed")}, readBack: upsertByKeyStep{err: errors.New("read failed")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: preflight}, tc.write, tc.readBack}}
			out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
				"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"f":1}`, "--yes")
			if err == nil || out != "" {
				t.Fatalf("read-back failure = output:%q err:%v", out, err)
			}
		})
	}

	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: preflight}, {text: `{"updated":1}`}, {text: `{"views":[{"viewId":"v","viewName":"X","viewType":"Grid","config":{"f":1}}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
		"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"f":1}`, "--yes")
	if err != nil || !strings.Contains(out, `"action": "update"`) {
		t.Fatalf("update success = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageViewPresetPureMatching(t *testing.T) {
	if presetViewMatches(map[string]any{"viewType": "Kanban", "config": map[string]any{}}, "Grid", map[string]any{}) {
		t.Fatal("type mismatch must fail")
	}
	if mapContains(map[string]any{}, map[string]any{"missing": 1}) {
		t.Fatal("missing key must fail")
	}
	if mapContains(map[string]any{"nested": 1}, map[string]any{"nested": map[string]any{"x": 1}}) {
		t.Fatal("nested type mismatch must fail")
	}
	if mapContains(map[string]any{"nested": map[string]any{}}, map[string]any{"nested": map[string]any{"x": 1}}) {
		t.Fatal("nested value mismatch must fail")
	}
	if !mapContains(map[string]any{"nested": map[string]any{"x": 1, "extra": true}}, map[string]any{"nested": map[string]any{"x": 1}}) {
		t.Fatal("nested subset must match")
	}
	if mapContains(map[string]any{"x": 1}, map[string]any{"x": 2}) {
		t.Fatal("scalar mismatch must fail")
	}
}

func TestCrossPlatformCoverageViewPresetProjectionShapeEdges(t *testing.T) {
	if !presetViewMatches(map[string]any{"viewType": "Grid", "f": 1}, "Grid", map[string]any{"f": 1}) {
		t.Fatal("top-level projected config must match")
	}
	invalid := []map[string]any{
		{"columns": "bad", "custom": map[string]any{}},
		{"columns": []any{"f"}, "custom": map[string]any{"hiddenFields": map[string]any{"f": "bad"}}},
		{"columns": []any{1}, "custom": map[string]any{"hiddenFields": map[string]any{"1": false}}},
		{"columns": []any{"f"}, "custom": map[string]any{"hiddenFields": map[string]any{}}},
		{"columns": []any{"f"}, "custom": map[string]any{"hiddenFields": []any{}}},
		{"columns": []any{1}, "custom": map[string]any{"hiddenFields": []any{false}}},
		{"columns": []any{"f"}, "custom": map[string]any{"hiddenFields": []any{"bad"}}},
	}
	for _, view := range invalid {
		if got, ok := projectedVisibleFieldIDs(view); ok || got != nil {
			t.Errorf("invalid projection %#v = %#v, %v", view, got, ok)
		}
	}
}

func TestCrossPlatformCoverageViewPresetReadBackDuplicateStopsE2E(t *testing.T) {
	disableViewPresetSleep(t)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"views":[]}`},
		{text: `{"viewId":"v1"}`},
		{text: `{"views":[{"viewId":"v1","viewName":"X"},{"viewId":"v2","viewName":"X"}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply",
		"--base-id", "base", "--table-id", "table", "--name", "X", "--view-type", "Grid", "--config", `{"f":1}`, "--yes")
	if err == nil || out != "" || len(caller.calls) != 3 {
		t.Fatalf("duplicate readback = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageWorkflowDeployValidationAndFailureStagesE2E(t *testing.T) {
	cases := []struct {
		name  string
		dsl   string
		extra []string
		steps []upsertByKeyStep
	}{
		{name: "invalid DSL", dsl: `{`},
		{name: "wrong DSL version", dsl: `{"version":"v0"}`},
		{name: "update preflight error", dsl: workflowDSLFixture, extra: []string{"--workflow-id", "w"}, steps: []upsertByKeyStep{{err: errors.New("preflight failed")}}},
		{name: "update target not found", dsl: workflowDSLFixture, extra: []string{"--workflow-id", "w"}, steps: []upsertByKeyStep{{text: `{"flowId":"other","flowSchema":{}}`}}},
		{name: "publish missing ID", dsl: workflowDSLFixture, steps: []upsertByKeyStep{{text: `{"valid":true}`}}},
		{name: "publish ID wrong type", dsl: workflowDSLFixture, steps: []upsertByKeyStep{{text: `{"valid":true,"flowId":1}`}}},
		{name: "read-back error", dsl: workflowDSLFixture, steps: []upsertByKeyStep{{text: `{"valid":true,"flowId":"w"}`}, {err: errors.New("read failed")}}},
		{name: "read-back wrong ID", dsl: workflowDSLFixture, steps: []upsertByKeyStep{{text: `{"valid":true,"flowId":"w"}`}, {text: `{"flowId":"other","flowSchema":{}}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--base-id", "base", "--dsl", tc.dsl}
			args = append(args, tc.extra...)
			args = append(args, "--yes")
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+workflow-deploy", args...)
			if err == nil || out != "" {
				t.Fatalf("workflow failure = output:%q err:%v", out, err)
			}
			if tc.name == "update target not found" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.ExitCode() != apperrors.ExitCodeValidation ||
					typed.Reason != "target_not_found" || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
					t.Fatalf("update preflight error contract = %#v", err)
				}
			}
		})
	}

	caller := &upsertByKeyCaller{dryRun: true}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--dry-run", "--yes")
	if err != nil || !strings.Contains(out, `"status": "planned"`) || !strings.Contains(out, "enable and verify") {
		t.Fatalf("workflow dry run = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageWorkflowEnableErrorCanStillBeVerifiedE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"valid":true,"flowId":"w"}`}, {text: `{"flowSchema":{}}`},
		{err: errors.New("enable reply failed")}, {text: `{"list":[{"flowId":"w","enabled":true}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
	if err != nil || !strings.Contains(out, "enable response error") {
		t.Fatalf("enable recovery = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageWorkflowListFailureShapesE2E(t *testing.T) {
	for name, step := range map[string]upsertByKeyStep{
		"list error":   {err: errors.New("list failed")},
		"missing list": {text: `{}`},
		"not present":  {text: `{"list":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: `{"valid":true,"flowId":"w"}`}, {text: `{"flowSchema":{}}`}, {text: `{"enabled":true}`}, step,
			}}
			out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
			if err == nil || out != "" {
				t.Fatalf("list failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageWorkflowListPaginationAndBoundE2E(t *testing.T) {
	page := make([]any, 100)
	for index := range page {
		page[index] = map[string]any{"flowId": fmt.Sprintf("other-%d", index)}
	}
	caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, _ map[string]any) (string, error) {
		switch index {
		case 0:
			return `{"valid":true,"flowId":"w"}`, nil
		case 1:
			return `{"flowSchema":{}}`, nil
		case 2:
			return `{"enabled":true}`, nil
		case 3:
			return mustJSONText(t, map[string]any{"list": page}), nil
		default:
			return `{"list":[{"flowId":"w","status":"ACTIVE"}]}`, nil
		}
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
	if err != nil || !strings.Contains(out, `"running": true`) || len(caller.calls) != 5 {
		t.Fatalf("paginated list = output:%q err:%v calls:%d", out, err, len(caller.calls))
	}

	fullPage := mustJSONText(t, map[string]any{"list": page})
	caller = &upsertByKeyCaller{callFn: func(index int, _, _ string, _ map[string]any) (string, error) {
		if index == 0 {
			return `{"valid":true,"flowId":"w"}`, nil
		}
		if index == 1 {
			return `{"flowSchema":{}}`, nil
		}
		if index == 2 {
			return `{"enabled":true}`, nil
		}
		return fullPage, nil
	}}
	out, err = runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "base", "--dsl", workflowDSLFixture, "--enable", "--yes")
	if err == nil || out != "" || len(caller.calls) != 103 {
		t.Fatalf("workflow list bound = output:%q err:%v calls:%d", out, err, len(caller.calls))
	}
}

func TestCrossPlatformCoverageWorkflowPureResponseHelpers(t *testing.T) {
	if _, _, _, err := workflowPublishResult(nil); err == nil {
		t.Fatal("nil publish result must fail")
	}
	detail, err := workflowDetail(map[string]any{"data": map[string]any{"name": "提醒", "flowSchema": map[string]any{}}}, "w")
	if err != nil || detail["name"] != "提醒" {
		t.Fatalf("workflow detail without echoed ID = %#v, %v", detail, err)
	}
	idOnly, err := workflowDetail(map[string]any{"data": map[string]any{"flowId": "w", "name": "提醒"}}, "w")
	if err != nil || idOnly["flowId"] != "w" {
		t.Fatalf("workflow detail with echoed ID only = %#v, %v", idOnly, err)
	}
	nameNormalized, err := workflowDetail(map[string]any{"flowId": "w", "flowSchema": map[string]any{}, "name": "提醒(1)"}, "w")
	if err != nil || nameNormalized["name"] != "提醒(1)" {
		t.Fatalf("workflow detail with normalized name = %#v, %v", nameNormalized, err)
	}
	if _, err := workflowDetail(map[string]any{"flowId": "other", "flowSchema": map[string]any{}}, "w"); err == nil {
		t.Fatal("workflow detail with conflicting ID must fail")
	}
	if _, err := workflowDetail(map[string]any{"flowId": 1, "flowSchema": map[string]any{}}, "w"); err == nil {
		t.Fatal("workflow detail with non-string ID must fail")
	}
	if _, err := workflowDetail(map[string]any{"flowSchema": "bad"}, "w"); err == nil {
		t.Fatal("workflow detail with malformed flowSchema must fail")
	}
	if _, err := workflowDetail(map[string]any{"name": "提醒"}, "w"); err == nil {
		t.Fatal("workflow detail without ID or flowSchema must fail")
	}
	if value, found := findBoolByKeys(map[string]any{"data": []any{map[string]any{"enabled": true}}}, "enabled"); !found || !value {
		t.Fatal("nested bool not found")
	}
	if _, found := findBoolByKeys([]any{"value"}, "enabled"); found {
		t.Fatal("unrelated bool unexpectedly found")
	}
	for _, workflow := range []map[string]any{
		{"status": "enabled"}, {"state": "active"}, {"isEnabled": true},
	} {
		if running, known := workflowRunningState(workflow); !running || !known {
			t.Fatalf("workflow should be running: %#v (running:%v known:%v)", workflow, running, known)
		}
	}
	if running, known := workflowRunningState(map[string]any{"status": "STOP"}); running || !known {
		t.Fatalf("stopped workflow state = running:%v known:%v", running, known)
	}
	if running, known := workflowRunningState(map[string]any{"name": "unknown"}); running || known {
		t.Fatalf("unknown workflow state = running:%v known:%v", running, known)
	}
}
