// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"aitable", command}, args...))
	executed, err := root.ExecuteC()
	if err == nil && output.UsesUnifiedResult(executed) {
		if _, _, emitErr := output.EmitStoredResult(executed); emitErr != nil {
			return stdout.String(), emitErr
		}
	}
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
		"duplicate field":    `[{"name":"T","fields":[{"fieldName":"F","type":"text"},{"fieldName":" F ","type":"number"}]}]`,
		"field config array": `[{"name":"T","fields":[{"fieldName":"F","type":"text","config":[]}]}]`,
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
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(typed.Actions) != 1 || len(typed.AvailableFlags) != 4 {
		t.Fatalf("invalid bootstrap recovery = %#v", err)
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
		{name: "verify fields missing collection", steps: []upsertByKeyStep{{text: `{"baseId":"b"}`}, {text: `{"baseId":"b"}`}, {text: `{"tableId":"t"}`}, {text: `{"tables":[{"tableId":"t"}]}`}, {text: `{}`}}},
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

func TestCrossPlatformCoverageBaseBootstrapFailurePublishesExactRecovery(t *testing.T) {
	out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{}`}}},
		"+base-bootstrap", "--name", "Project", "--tables", marshalBootstrapTables(t, nil), "--yes")
	if out != "" || err == nil {
		t.Fatalf("bootstrap recovery = output:%q err:%v", out, err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(typed.Actions) != 1 || len(typed.AvailableFlags) != 4 {
		t.Fatalf("bootstrap typed recovery = %#v", err)
	}
	if typed.Actions[0] != `dws aitable +base-search --query Project --format json` {
		t.Fatalf("bootstrap next command = %#v", typed.Actions)
	}
}

func TestCrossPlatformCoverageAITableRecoveryCommandsQuoteUntrustedValues(t *testing.T) {
	t.Run("base name", func(t *testing.T) {
		name := `项目 $(touch /tmp/pwn) 'Q'`
		out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{}`}}},
			"+base-bootstrap", "--name", name, "--tables", marshalBootstrapTables(t, nil), "--yes")
		if out != "" || err == nil {
			t.Fatalf("hostile base name recovery = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || len(typed.Actions) != 1 {
			t.Fatalf("hostile base name error = %#v", err)
		}
		want := `dws aitable +base-search --query '项目 $(touch /tmp/pwn) '\''Q'\''' --format json`
		if runtime.GOOS == "windows" {
			want = `dws aitable +base-search --query REPLACE_QUERY --format json`
		}
		if typed.Actions[0] != want {
			t.Fatalf("base name recovery = %q, want %q", typed.Actions[0], want)
		}
	})

	t.Run("base id", func(t *testing.T) {
		baseID := `base;printf hacked`
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: mustJSON(t, map[string]any{"baseId": baseID})},
			{err: errors.New("verify base failed")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+base-bootstrap",
			"--name", "Project", "--tables", marshalBootstrapTables(t, nil), "--yes")
		if out != "" || err == nil {
			t.Fatalf("hostile base id recovery = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || len(typed.Actions) != 1 {
			t.Fatalf("hostile base id error = %#v", err)
		}
		want := `dws aitable +base-get --base-id 'base;printf hacked' --format json`
		if runtime.GOOS == "windows" {
			want = `dws aitable +base-get --base-id REPLACE_BASE_ID --format json`
		}
		if typed.Actions[0] != want {
			t.Fatalf("base id recovery = %q, want %q", typed.Actions[0], want)
		}
	})

	t.Run("base and table ids", func(t *testing.T) {
		baseID := `base id;exit 1`
		tableID := "table`uname`'x"
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: mustJSON(t, map[string]any{"tableId": tableID})},
			{err: errors.New("verify table failed")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+table-bootstrap",
			"--base-id", baseID, "--name", "任务", "--fields", `[]`, "--yes")
		if out != "" || err == nil {
			t.Fatalf("hostile table ids recovery = output:%q err:%v", out, err)
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || len(typed.Actions) != 1 {
			t.Fatalf("hostile table ids error = %#v", err)
		}
		want := `dws aitable +table-get --base-id 'base id;exit 1' --table-id 'table` + "`uname`" + `'\''x' --format json`
		if runtime.GOOS == "windows" {
			want = `dws aitable +table-get --base-id REPLACE_BASE_ID --table-id REPLACE_TABLE_ID --format json`
		}
		if typed.Actions[0] != want {
			t.Fatalf("table ids recovery = %q, want %q", typed.Actions[0], want)
		}
	})
}

func TestCrossPlatformCoverageAITableRecoveryCommandsUseWindowsPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "query ampersand",
			argv: []string{"dws", "aitable", "+base-search", "--query", "x&calc", "--format", "json"},
			want: "dws aitable +base-search --query REPLACE_QUERY --format json",
		},
		{
			name: "base id pipe",
			argv: []string{"dws", "aitable", "+base-get", "--base-id", "base|whoami", "--format", "json"},
			want: "dws aitable +base-get --base-id REPLACE_BASE_ID --format json",
		},
		{
			name: "table id variable expansion",
			argv: []string{"dws", "aitable", "+table-get", "--base-id", "base", "--table-id", "%PATH%", "--format", "json"},
			want: "dws aitable +table-get --base-id base --table-id REPLACE_TABLE_ID --format json",
		},
		{
			name: "portable values stay inline",
			argv: []string{"dws", "aitable", "+table-get", "--base-id", "base-1", "--table-id", "table_2", "--format", "json"},
			want: "dws aitable +table-get --base-id base-1 --table-id table_2 --format json",
		},
		{
			name: "unknown argument fallback",
			argv: []string{"dws", "aitable", "unsafe value"},
			want: "dws aitable REPLACE_VALUE",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := aitableRecoveryCommandForPlatform("windows", tc.argv...)
			if got != tc.want {
				t.Fatalf("windows recovery = %q, want %q", got, tc.want)
			}
			for _, hostile := range []string{"&", "|", "%PATH%"} {
				if strings.Contains(got, hostile) {
					t.Fatalf("windows recovery contains hostile token %q: %s", hostile, got)
				}
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
	if err := verifyDeclaredFieldStructures([]map[string]any{{"fieldName": "A", "fieldType": "text"}}, []any{map[string]any{"fieldName": "B", "type": "text"}}); err == nil {
		t.Fatal("missing field name must fail")
	}
	if err := verifyDeclaredFieldStructures(nil, []any{"bad"}); err == nil {
		t.Fatal("non-object declaration must fail")
	}
	if declaredValueMatches("bad", map[string]any{"x": 1}) {
		t.Fatal("object declaration must not match a scalar")
	}
	if declaredValueMatches("bad", []any{}) || declaredValueMatches([]any{}, []any{1}) {
		t.Fatal("array declaration must require an array of equal length")
	}
	if declaredValueMatches([]any{1}, []any{2}) {
		t.Fatal("array declaration must compare each item")
	}
	if got := findStringByKeys(map[string]any{"items": []any{map[string]any{"nested": " value "}}}, "nested"); got != "value" {
		t.Fatalf("nested array string = %q", got)
	}
}
