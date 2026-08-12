// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func runAITableCompositeCLI(t *testing.T, caller *upsertByKeyCaller, command string, args ...string) (string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"aitable", command}, args...))
	err := root.Execute()
	return stdout.String(), err
}

func TestCrossPlatformCoverageBaseSchemaSnapshotE2E(t *testing.T) {
	t.Run("one table with legal empty fields and views", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"data":{"baseId":"base","tables":[{"tableId":"t1","name":"任务"}]}}`},
			{text: `{"data":{"tables":[{"tableId":"t1","name":"任务"}]}}`},
			{text: `{"data":{"fields":[]}}`},
			{text: `{"data":{"views":[]}}`},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+base-schema-snapshot", "--base-id", "base")
		if err != nil {
			t.Fatalf("snapshot error = %v", err)
		}
		for _, want := range []string{`"status": "success"`, `"tableCount": 1`, `"fields": []`, `"views": []`} {
			if !strings.Contains(out, want) {
				t.Fatalf("snapshot output missing %s: %s", want, out)
			}
		}
	})

	t.Run("empty tables is explicit success", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"baseId":"base","tables":[]}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+base-schema-snapshot", "--base-id", "base")
		if err != nil || !strings.Contains(out, `"tableCount": 0`) || len(caller.calls) != 1 {
			t.Fatalf("empty snapshot = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("missing tables contract fails", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"baseId":"base","data":{}}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+base-schema-snapshot", "--base-id", "base")
		if err == nil || out != "" {
			t.Fatalf("missing tables = output:%q err:%v", out, err)
		}
	})
}

func bootstrapFields(count int) []any {
	fields := make([]any, 0, count)
	for index := 0; index < count; index++ {
		fields = append(fields, map[string]any{"fieldName": fmt.Sprintf("F%02d", index), "type": "text"})
	}
	return fields
}

func marshalBootstrapTables(t *testing.T, fields []any) string {
	t.Helper()
	if fields == nil {
		fields = []any{}
	}
	raw, err := json.Marshal([]any{map[string]any{"name": "任务", "fields": fields}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func fieldReadBackJSON(t *testing.T, fields []any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCrossPlatformCoverageBaseBootstrapCreatesChunksAndVerifiesE2E(t *testing.T) {
	fields := bootstrapFields(16)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"baseId":"b-new"}}`},
		{text: `{"baseId":"b-new","tables":[]}`},
		{text: `{"data":{"tableId":"t-new"}}`},
		{text: `{"createdFields":[{"fieldId":"f16"}]}`},
		{text: `{"tables":[{"tableId":"t-new","name":"任务"}]}`},
		{text: fieldReadBackJSON(t, fields)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "项目", "--tables", marshalBootstrapTables(t, fields), "--yes")
	if err != nil {
		t.Fatalf("bootstrap error = %v", err)
	}
	for _, want := range []string{`"baseId": "b-new"`, `"tableId": "t-new"`, `"status": "verified"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("bootstrap output missing %s: %s", want, out)
		}
	}
	if len(caller.calls) != 6 || caller.calls[0].tool != "create_base" || caller.calls[2].tool != "create_table" || caller.calls[3].tool != "create_fields" {
		t.Fatalf("bootstrap calls = %#v", caller.calls)
	}
	if caller.calls[0].args["baseName"] != "项目" {
		t.Fatalf("create_base args = %#v", caller.calls[0].args)
	}
	createBaseCalls := 0
	for _, call := range caller.calls {
		if call.tool == "create_base" {
			createBaseCalls++
		}
	}
	if createBaseCalls != 1 {
		t.Fatalf("create_base calls = %d, want exactly 1; calls = %#v", createBaseCalls, caller.calls)
	}
	if got := len(caller.calls[2].args["fields"].([]any)); got != 15 {
		t.Fatalf("create_table fields = %d, want 15", got)
	}
	if got := len(caller.calls[3].args["fields"].([]any)); got != 1 {
		t.Fatalf("create_fields fields = %d, want 1", got)
	}
}

func TestCrossPlatformCoverageBaseBootstrapRequiresConfirmationBeforeMCP(t *testing.T) {
	caller := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "项目", "--tables", marshalBootstrapTables(t, nil))
	if out != "" {
		t.Fatalf("unconfirmed bootstrap output = %q, want empty", out)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
		t.Fatalf("unconfirmed bootstrap error = %#v, want confirmation_required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unconfirmed bootstrap reached MCP: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageBaseBootstrapUnknownAndDryRunE2E(t *testing.T) {
	t.Run("empty create response is unknown and not retry-safe", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: ""}}}
		out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "项目", "--tables", marshalBootstrapTables(t, nil), "--yes")
		if err == nil || out != "" {
			t.Fatalf("unknown base create = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "aitable_composite_unknown" || typed.Retryable {
			t.Fatalf("unknown base error = %#v", err)
		}
	})

	t.Run("dry run validates and makes no calls", func(t *testing.T) {
		caller := &upsertByKeyCaller{dryRun: true}
		out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap", "--name", "项目", "--tables", marshalBootstrapTables(t, nil), "--dry-run")
		if err != nil || len(caller.calls) != 0 || !strings.Contains(out, `"status": "planned"`) {
			t.Fatalf("bootstrap dry run = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageBaseBootstrapInputValidation(t *testing.T) {
	tooMany := make([]any, 101)
	for index := range tooMany {
		tooMany[index] = map[string]any{"name": fmt.Sprintf("T%d", index)}
	}
	tooManyJSON, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"invalid JSON":       `{`,
		"not an array":       `{}`,
		"empty array":        `[]`,
		"too many tables":    string(tooManyJSON),
		"non-object table":   `[1]`,
		"empty name":         `[{"name":" "}]`,
		"duplicate name":     `[{"name":"T"},{"name":"T"}]`,
		"fields not array":   `[{"name":"T","fields":{}}]`,
		"field not object":   `[{"name":"T","fields":[1]}]`,
		"field missing type": `[{"name":"T","fields":[{"fieldName":"F"}]}]`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if tables, err := parseBootstrapTables(raw); err == nil || tables != nil {
				t.Fatalf("parseBootstrapTables(%s) = %#v, %v", raw, tables, err)
			}
		})
	}
}

func TestCrossPlatformCoverageBaseSnapshotFailureStagesE2E(t *testing.T) {
	base := `{"baseId":"base","tables":[{"tableId":"t1","name":"T"}]}`
	cases := []struct {
		name  string
		steps []upsertByKeyStep
	}{
		{name: "get base error", steps: []upsertByKeyStep{{err: errors.New("get base failed")}}},
		{name: "get base wrong id", steps: []upsertByKeyStep{{text: `{"baseId":"other","tables":[]}`}}},
		{name: "table missing id", steps: []upsertByKeyStep{{text: `{"baseId":"base","tables":[{"name":"T"}]}`}}},
		{name: "get tables error", steps: []upsertByKeyStep{{text: base}, {err: errors.New("get tables failed")}}},
		{name: "get tables wrong id", steps: []upsertByKeyStep{{text: base}, {text: `{"tables":[{"tableId":"other"}]}`}}},
		{name: "get fields error", steps: []upsertByKeyStep{{text: base}, {text: `{"tables":[{"tableId":"t1"}]}`}, {err: errors.New("get fields failed")}}},
		{name: "get fields missing collection", steps: []upsertByKeyStep{{text: base}, {text: `{"tables":[{"tableId":"t1"}]}`}, {text: `{}`}}},
		{name: "get views error", steps: []upsertByKeyStep{{text: base}, {text: `{"tables":[{"tableId":"t1"}]}`}, {text: `{"fields":[]}`}, {err: errors.New("get views failed")}}},
		{name: "get views missing collection", steps: []upsertByKeyStep{{text: base}, {text: `{"tables":[{"tableId":"t1"}]}`}, {text: `{"fields":[]}`}, {text: `{}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+base-schema-snapshot", "--base-id", "base")
			if err == nil || out != "" {
				t.Fatalf("snapshot failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageBaseBootstrapExecuteRejectsInvalidTablesE2E(t *testing.T) {
	out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+base-bootstrap", "--name", "Project", "--tables", `{`, "--yes")
	if err == nil || out != "" {
		t.Fatalf("invalid bootstrap tables = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageBaseBootstrapFailureStagesE2E(t *testing.T) {
	tables := marshalBootstrapTables(t, nil)
	cases := []struct {
		name      string
		steps     []upsertByKeyStep
		extra     []string
		withField bool
	}{
		{name: "missing base id", steps: []upsertByKeyStep{{text: `{}`}}},
		{name: "base verification error", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {err: errors.New("verify base failed")}}},
		{name: "base verification wrong id", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"other"}`}}},
		{name: "create table error", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {err: errors.New("create table failed")}}},
		{name: "create table missing id", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{}`}}},
		{name: "verify table error", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{"tableId":"t"}`}, {err: errors.New("verify table failed")}}},
		{name: "verify table wrong id", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{"tableId":"t"}`}, {text: `{"tables":[]}`}}},
		{name: "verify fields error", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{"tableId":"t"}`}, {text: `{"tables":[{"tableId":"t"}]}`}, {err: errors.New("verify fields failed")}}},
		{name: "verify fields mismatch", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{"tableId":"t"}`}, {text: `{"tables":[{"tableId":"t"}]}`}, {text: `{"fields":[]}`}}, extra: []string{"--folder-id", "folder", "--template-id", "template"}, withField: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputTables := tables
			if tc.withField {
				inputTables = marshalBootstrapTables(t, bootstrapFields(1))
			}
			args := []string{"--name", "Project", "--tables", inputTables}
			args = append(args, tc.extra...)
			args = append(args, "--yes")
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: tc.steps}, "+base-bootstrap", args...)
			if err == nil || out != "" {
				t.Fatalf("bootstrap failure = output:%q err:%v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageBaseBootstrapRecoversFieldCallErrorE2E(t *testing.T) {
	fields := bootstrapFields(16)
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"baseId":"b"}`},
		{text: `{"baseId":"b"}`},
		{text: `{"tableId":"t"}`},
		{err: errors.New("create fields reply failed")},
		{text: `{"tables":[{"tableId":"t"}]}`},
		{text: fieldReadBackJSON(t, fields)},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap",
		"--name", "Project", "--tables", marshalBootstrapTables(t, fields), "--folder-id", "folder", "--template-id", "template", "--yes")
	if err != nil || !strings.Contains(out, `"status": "success"`) || !strings.Contains(out, "create_fields offset") {
		t.Fatalf("field recovery = output:%q err:%v", out, err)
	}
	if caller.calls[0].args["folderId"] != "folder" || caller.calls[0].args["templateId"] != "template" {
		t.Fatalf("create base args = %#v", caller.calls[0].args)
	}
}

func TestCrossPlatformCoverageBaseCompositeShapeHelpers(t *testing.T) {
	if _, ok := findNamedObjectList(map[string]any{"tables": "bad"}, "tables"); ok {
		t.Fatal("non-array named child must fail")
	}
	if _, ok := findNamedObjectList(map[string]any{"tables": []any{"bad"}}, "tables"); ok {
		t.Fatal("non-object list item must fail")
	}
	if containsAllFieldNames([]map[string]any{{"fieldName": "A"}}, []any{map[string]any{"fieldName": "B"}}) {
		t.Fatal("missing field name must fail")
	}
	if got := findStringByKeys(map[string]any{"items": []any{map[string]any{"nested": " value "}}}, "nested"); got != "value" {
		t.Fatalf("nested array string = %q", got)
	}
}
