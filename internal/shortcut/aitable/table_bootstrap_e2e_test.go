// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageTableBootstrapPublishesResultContract(t *testing.T) {
	result, err := contract.NormalizeResultSpec(TableBootstrap.Contract.Result, "aitable.shortcut_table_bootstrap")
	if err != nil {
		t.Fatalf("normalize table bootstrap Result: %v", err)
	}
	wantOutcomes := []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
	if result == nil || !reflect.DeepEqual(result.Outcomes, wantOutcomes) {
		t.Fatalf("table bootstrap outcomes = %#v, want %#v", result, wantOutcomes)
	}
	var schema map[string]any
	if err := json.Unmarshal(result.DataSchema, &schema); err != nil {
		t.Fatalf("decode table bootstrap data_schema: %v", err)
	}
	properties, _ := schema["properties"].(map[string]any)
	status, _ := properties["status"].(map[string]any)
	if got, want := mustJSON(t, status["enum"]), `["success","planned","partial_success","unknown"]`; got != want {
		t.Fatalf("table bootstrap status enum = %s, want %s", got, want)
	}
	for _, property := range []string{"contractVersion", "operation", "executed", "retryable", "plan", "completedSteps", "verification", "checkpoint", "knownSideEffects", "result"} {
		if properties[property] == nil {
			t.Errorf("table bootstrap data_schema is missing %q", property)
		}
	}
}

func TestCrossPlatformCoverageTableBootstrapCreatesChunksAndVerifies(t *testing.T) {
	fields := bootstrapFields(16)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"tableId":"table-new"}}`},
		{text: `{"data":{"createdFields":[{"fieldId":"f16"}]}}`},
		{text: `{"data":{"tables":[{"tableId":"table-new","tableName":"任务"}]}}`},
		{text: fieldReadBackJSON(t, fields)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", mustJSON(t, fields), "--yes")
	if err != nil {
		t.Fatalf("table bootstrap error = %v", err)
	}
	for _, want := range []string{`"operation": "table_bootstrap"`, `"tableId": "table-new"`, `"status": "verified"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("table bootstrap output missing %s: %s", want, out)
		}
	}
	if got := len(caller.calls); got != 4 {
		t.Fatalf("table bootstrap calls = %d, want 4: %#v", got, caller.calls)
	}
	wantFirstArgs := map[string]any{
		"baseId":    "base",
		"tableName": "任务",
		"fields":    fields[:15],
	}
	if caller.calls[0].product != serverMain || caller.calls[0].tool != "create_table" || mustJSON(t, caller.calls[0].args) != mustJSON(t, wantFirstArgs) {
		t.Fatalf("table bootstrap first call = %#v, want product:%q tool:create_table args:%s", caller.calls[0], serverMain, mustJSON(t, wantFirstArgs))
	}
	if caller.calls[0].tool != "create_table" || caller.calls[1].tool != "create_fields" || caller.calls[2].tool != "get_tables" || caller.calls[3].tool != "get_fields" {
		t.Fatalf("table bootstrap call order = %#v", caller.calls)
	}
	if got := len(caller.calls[0].args["fields"].([]any)); got != 15 {
		t.Fatalf("initial fields = %d, want 15", got)
	}
	if got := len(caller.calls[1].args["fields"].([]any)); got != 1 {
		t.Fatalf("remaining fields = %d, want 1", got)
	}
}

func TestCrossPlatformCoverageTableBootstrapRequiresConfirmationBeforeMCP(t *testing.T) {
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", mustJSON(t, bootstrapFields(1)))
	if out != "" {
		t.Fatalf("unconfirmed table bootstrap output = %q, want empty", out)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
		t.Fatalf("unconfirmed table bootstrap error = %#v, want confirmation_required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unconfirmed table bootstrap calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageTableBootstrapValidationPublishesRecovery(t *testing.T) {
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", `{}`, "--yes")
	if out != "" || err == nil || len(caller.calls) != 0 {
		t.Fatalf("validation = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(typed.Actions) != 1 || len(typed.AvailableFlags) != 3 {
		t.Fatalf("typed validation recovery = %#v", err)
	}
}

func TestCrossPlatformCoverageTableBootstrapInputValidation(t *testing.T) {
	tooMany := bootstrapFields(101)
	cases := map[string]string{
		"invalid JSON":       `{`,
		"not an array":       `{}`,
		"too many fields":    mustJSON(t, tooMany),
		"non-object field":   `[1]`,
		"field missing type": `[{"fieldName":"标题"}]`,
		"duplicate name":     `[{"fieldName":"标题","type":"text"},{"fieldName":" 标题 ","type":"number"}]`,
		"config not object":  `[{"fieldName":"标题","type":"text","config":[]}]`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			fields, err := parseBootstrapFields(raw)
			if err == nil || fields != nil {
				t.Fatalf("parseBootstrapFields(%q) = %#v, %v", raw, fields, err)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || len(typed.Actions) != 1 || len(typed.AvailableFlags) != 3 {
				t.Fatalf("typed validation recovery = %#v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageTableBootstrapRejectsDuplicateFieldsBeforeMCP(t *testing.T) {
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务",
		"--fields", `[{"fieldName":"标题","type":"text"},{"fieldName":"标题","type":"number"}]`, "--yes")
	if out != "" || err == nil || len(caller.calls) != 0 {
		t.Fatalf("duplicate field validation = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageTableBootstrapVerifiesTypeAndDeclaredConfig(t *testing.T) {
	fields := []any{map[string]any{
		"fieldName": "状态",
		"type":      "singleSelect",
		"config": map[string]any{
			"options": []any{map[string]any{"name": "待办"}},
		},
	}}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"tableId":"table-new"}`},
		{text: `{"tables":[{"tableId":"table-new"}]}`},
		{text: `{"fields":[{"fieldId":"field-1","fieldName":"状态","fieldType":"singleSelect","config":{"options":[{"name":"待办","optionId":"option-1"}],"extra":true}}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", mustJSON(t, fields), "--yes")
	if err != nil || !strings.Contains(out, `"status": "verified"`) {
		t.Fatalf("typed config verification = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageTableBootstrapDryRun(t *testing.T) {
	caller := &upsertByKeyCaller{dryRun: true}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", `[]`, "--dry-run")
	if err != nil || len(caller.calls) != 0 || !strings.Contains(out, `"status": "planned"`) {
		t.Fatalf("table bootstrap dry run = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageTableBootstrapFailureStages(t *testing.T) {
	oneField := bootstrapFields(1)
	cases := []struct {
		name   string
		fields []any
		steps  []upsertByKeyStep
	}{
		{name: "create table error", fields: oneField, steps: []upsertByKeyStep{{err: errors.New("create table failed")}}},
		{name: "create table missing id", fields: oneField, steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "get tables error", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {err: errors.New("get tables failed")}}},
		{name: "get tables wrong id", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"other"}]}`}}},
		{name: "get fields error", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {err: errors.New("get fields failed")}}},
		{name: "get fields missing collection", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {text: `{}`}}},
		{name: "get fields mismatch", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {text: `{"fields":[]}`}}},
		{name: "get fields type mismatch", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {text: `{"fields":[{"fieldName":"F00","fieldType":"number"}]}`}}},
		{name: "get fields config mismatch", fields: []any{map[string]any{"fieldName": "状态", "type": "singleSelect", "config": map[string]any{"multiple": false}}}, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {text: `{"fields":[{"fieldName":"状态","fieldType":"singleSelect","config":{"multiple":true}}]}`}}},
		{name: "get fields duplicate name", fields: oneField, steps: []upsertByKeyStep{{text: `{"tableId":"table-new"}`}, {text: `{"tables":[{"tableId":"table-new"}]}`}, {text: `{"fields":[{"fieldName":"F00","fieldType":"text"},{"fieldName":"F00","fieldType":"text"}]}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: tc.steps}
			out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
				"--base-id", "base", "--name", "任务", "--fields", mustJSON(t, tc.fields), "--yes")
			if out != "" || err == nil {
				t.Fatalf("table bootstrap failure = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable {
				t.Fatalf("typed table bootstrap failure = %#v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageTableBootstrapRecoversFieldCallError(t *testing.T) {
	fields := bootstrapFields(16)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"tableId":"table-new"}`},
		{err: errors.New("create fields reply failed")},
		{text: `{"tables":[{"tableId":"table-new"}]}`},
		{text: fieldReadBackJSON(t, fields)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
		"--base-id", "base", "--name", "任务", "--fields", mustJSON(t, fields), "--yes")
	if err != nil || !strings.Contains(out, `"status": "success"`) || !strings.Contains(out, "create_fields offset") {
		t.Fatalf("field recovery = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageCompositeRecoveryFlags(t *testing.T) {
	cases := map[string][]string{
		"base_bootstrap":                {"name", "folder-id", "template-id", "tables"},
		"table_bootstrap":               {"base-id", "name", "fields"},
		"base_schema_snapshot_unmapped": nil,
	}
	for operation, want := range cases {
		got := compositeRecoveryFlags(operation)
		if mustJSON(t, got) != mustJSON(t, want) {
			t.Fatalf("compositeRecoveryFlags(%q) = %#v, want %#v", operation, got, want)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
