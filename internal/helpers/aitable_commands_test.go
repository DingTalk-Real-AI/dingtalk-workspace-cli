package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
	"github.com/spf13/cobra"
)

type aitableCommandRunner struct {
	last executor.Invocation
}

func (r *aitableCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

type aitableRetryProbeRunner struct {
	calls  int
	result executor.Result
	err    error
}

func (r *aitableRetryProbeRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	r.result.Invocation = invocation
	return r.result, r.err
}

func newAitableRetryTestCommand() *cobra.Command {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	return cmd
}

func TestAitableBaseGetAcceptsCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		args   []string
		wantID string
	}{
		{name: "base", args: []string{"--base", "BASE_001"}, wantID: "BASE_001"},
		{name: "id", args: []string{"--id", "BASE_002"}, wantID: "BASE_002"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &aitableCommandRunner{}
			cmd := newAitableBaseGetCommand(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
			}
			if runner.last.Tool != "get_base" {
				t.Fatalf("tool = %q, want get_base", runner.last.Tool)
			}
			if got := runner.last.Params["baseId"]; got != tc.wantID {
				t.Fatalf("baseId = %#v, want %q", got, tc.wantID)
			}
		})
	}
}

func TestAitableBaseSearchWithoutQueryListsBases(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableBaseSearchCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--cursor", "CURSOR_001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "list_bases" {
		t.Fatalf("tool = %q, want list_bases", runner.last.Tool)
	}
	if got := runner.last.Params["cursor"]; got != "CURSOR_001" {
		t.Fatalf("cursor = %#v, want CURSOR_001", got)
	}
	if _, ok := runner.last.Params["query"]; ok {
		t.Fatalf("unexpected query param in %#v", runner.last.Params)
	}
}

func TestAitableBaseCopyBuildsWukongPayload(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableBaseCopyCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--target-folder-id", "FOLDER_001",
		"--only-struct",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "copy_base" {
		t.Fatalf("tool = %q, want copy_base", runner.last.Tool)
	}
	if got := runner.last.Params["baseId"]; got != "BASE_001" {
		t.Fatalf("baseId = %#v, want BASE_001", got)
	}
	if got := runner.last.Params["targetFolderId"]; got != "FOLDER_001" {
		t.Fatalf("targetFolderId = %#v, want FOLDER_001", got)
	}
	if got := runner.last.Params["onlyCopyMeta"]; got != true {
		t.Fatalf("onlyCopyMeta = %#v, want true", got)
	}
}

func TestAitableBusinessErrorDoesNotRetry(t *testing.T) {
	t.Parallel()

	runner := &aitableRetryProbeRunner{err: errors.New("invalid params")}
	cmd := newAitableRetryTestCommand()

	err := runAitableProductTool(cmd, runner, "aitable", "create_view", map[string]any{"baseId": "BASE"})
	if err == nil {
		t.Fatal("runAitableProductTool error = nil, want invalid params")
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestAitableSuccessResponseRetryableFieldDoesNotRetry(t *testing.T) {
	t.Parallel()

	runner := &aitableRetryProbeRunner{
		result: executor.Result{
			Response: map[string]any{
				"result": map[string]any{
					"retryable": true,
				},
			},
		},
	}
	cmd := newAitableRetryTestCommand()

	if err := runAitableProductTool(cmd, runner, "aitable", "query_records", map[string]any{"baseId": "BASE"}); err != nil {
		t.Fatalf("runAitableProductTool error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestAitableExportDataDryRunUsesWukongFormatFlags(t *testing.T) {
	t.Parallel()

	cmd := newAitableExportDataCommand(nil)
	cmd.Flags().Bool("dry-run", true, "dry run")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--scope", "table",
		"--table-id", "TBL_001",
		"--format", "excel",
		"--timeout-ms", "1000",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	var inv executor.Invocation
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil {
		t.Fatalf("output JSON parse error = %v\nstdout:\n%s", err, out.String())
	}
	if inv.Tool != "export_data" {
		t.Fatalf("tool = %q, want export_data", inv.Tool)
	}
	if got := inv.Params["format"]; got != "excel" {
		t.Fatalf("format param = %#v, want excel", got)
	}
	if got := inv.Params["tableId"]; got != "TBL_001" {
		t.Fatalf("tableId param = %#v, want TBL_001", got)
	}
	if got := inv.Params["timeoutMs"]; got != float64(1000) {
		t.Fatalf("timeoutMs param = %#v, want 1000", got)
	}
	if _, ok := inv.Params["exportFormat"]; ok {
		t.Fatalf("unexpected exportFormat param in %#v", inv.Params)
	}
	if _, ok := inv.Params["__async__"]; ok {
		t.Fatalf("unexpected __async__ param in %#v", inv.Params)
	}
	if _, ok := inv.Params["__output__"]; ok {
		t.Fatalf("unexpected __output__ param in %#v", inv.Params)
	}
}

func TestAitableExportDataRequiresFormatWhenCreatingTask(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableExportDataCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--scope", "table",
		"--table-id", "TBL_001",
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want missing --format validation")
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestParseAitableExportQueryResultUnwrapsResultPayload(t *testing.T) {
	t.Parallel()

	res := parseAitableExportQueryResult(map[string]any{
		"content": map[string]any{
			"success": true,
			"result": map[string]any{
				"status":      "SUCCESS",
				"downloadUrl": "https://example.com/export.xlsx",
				"fileName":    "export.xlsx",
			},
		},
	})

	if res.Status != asynctask.StatusSuccess {
		t.Fatalf("status = %s, want SUCCESS", res.Status)
	}
	if res.DownloadURL != "https://example.com/export.xlsx" {
		t.Fatalf("download url = %q", res.DownloadURL)
	}
	if got := firstAitableString(res.Raw, "fileName"); got != "export.xlsx" {
		t.Fatalf("fileName = %q, want export.xlsx", got)
	}
}

func TestParseAitableExportQueryResultRequiresDownloadURLForSuccess(t *testing.T) {
	t.Parallel()

	res := parseAitableExportQueryResult(map[string]any{
		"result": map[string]any{
			"status": "SUCCESS",
			"taskId": "TASK_001",
		},
	})

	if res.Status != asynctask.StatusProcessing {
		t.Fatalf("status = %s, want PROCESSING", res.Status)
	}
}

func TestAitableExportDataLiveCallsExportDataOnce(t *testing.T) {
	t.Parallel()

	runner := &aitableRetryProbeRunner{
		result: executor.Result{
			Response: map[string]any{
				"content": map[string]any{
					"result": map[string]any{
						"status": "PROCESSING",
						"taskId": "TASK_001",
					},
				},
			},
		},
	}
	cmd := newAitableExportDataCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--scope", "table",
		"--table-id", "TBL_001",
		"--format", "excel",
		"--timeout-ms", "1000",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if got := runner.result.Invocation.Params["format"]; got != "excel" {
		t.Fatalf("format param = %#v, want excel", got)
	}
	if got := runner.result.Invocation.Params["timeoutMs"]; got != 1000 {
		t.Fatalf("timeoutMs param = %#v, want 1000", got)
	}
	if got := runner.result.Invocation.Tool; got != "export_data" {
		t.Fatalf("tool = %q, want export_data", got)
	}
}

func TestAitableImportUploadBuildsWukongPayload(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableImportUploadCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--file-name", "data.csv",
		"--file-size", "12",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "prepare_import_upload" {
		t.Fatalf("tool = %q, want prepare_import_upload", runner.last.Tool)
	}
	if got := runner.last.Params["baseId"]; got != "BASE_001" {
		t.Fatalf("baseId = %#v, want BASE_001", got)
	}
	if got := runner.last.Params["fileName"]; got != "data.csv" {
		t.Fatalf("fileName = %#v, want data.csv", got)
	}
	if got := runner.last.Params["fileSize"]; got != int64(12) {
		t.Fatalf("fileSize = %#v, want int64(12)", got)
	}
}

func TestAitableImportDataDryRunAcceptsWukongTimeout(t *testing.T) {
	t.Parallel()

	cmd := newAitableImportDataCommand(nil)
	cmd.Flags().Bool("dry-run", true, "dry run")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--import-id", "IMPORT_001",
		"--timeout", "30",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	var inv executor.Invocation
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil {
		t.Fatalf("output JSON parse error = %v\nstdout:\n%s", err, out.String())
	}
	if inv.Tool != "import_data" {
		t.Fatalf("tool = %q, want import_data", inv.Tool)
	}
	if got := inv.Params["timeout"]; got != float64(30) {
		t.Fatalf("timeout param = %#v, want 30", got)
	}
	if _, ok := inv.Params["__async__"]; ok {
		t.Fatalf("unexpected __async__ param in %#v", inv.Params)
	}
}

func TestAitableImportDataDryRunPassesWukongImportOptions(t *testing.T) {
	t.Parallel()

	cmd := newAitableImportDataCommand(nil)
	cmd.Flags().Bool("dry-run", true, "dry run")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--import-id", "IMPORT_001",
		"--table-id", "TBL_001",
		"--field-mapping", `{"目标":"源"}`,
		"--header-row", "2",
		"--src-sheet-name", "Sheet1",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	var inv executor.Invocation
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil {
		t.Fatalf("output JSON parse error = %v\nstdout:\n%s", err, out.String())
	}
	if _, ok := inv.Params["timeout"]; ok {
		t.Fatalf("unexpected default timeout param in %#v", inv.Params)
	}
	if _, ok := inv.Params["__async__"]; ok {
		t.Fatalf("unexpected __async__ param in %#v", inv.Params)
	}
	if got := inv.Params["tableId"]; got != "TBL_001" {
		t.Fatalf("tableId param = %#v, want TBL_001", got)
	}
	if got := inv.Params["headerRow"]; got != float64(2) {
		t.Fatalf("headerRow param = %#v, want 2", got)
	}
	if got := inv.Params["srcSheetName"]; got != "Sheet1" {
		t.Fatalf("srcSheetName param = %#v, want Sheet1", got)
	}
	mapping, ok := inv.Params["fieldMapping"].(map[string]any)
	if !ok {
		t.Fatalf("fieldMapping = %#v, want object", inv.Params["fieldMapping"])
	}
	if got := mapping["目标"]; got != "源" {
		t.Fatalf("fieldMapping value = %#v, want 源", got)
	}
}

func TestAitableImportDataLiveCallsImportDataOnce(t *testing.T) {
	runner := &aitableRetryProbeRunner{
		result: executor.Result{
			Response: map[string]any{
				"status": "PROCESSING",
			},
		},
	}
	cmd := newAitableImportDataCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--import-id", "IMPORT_001",
		"--timeout", "30",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if got := runner.result.Invocation.Params["timeout"]; got != 30 {
		t.Fatalf("timeout param = %#v, want 30", got)
	}
	if got := runner.result.Invocation.Tool; got != "import_data" {
		t.Fatalf("tool = %q, want import_data", got)
	}
}

func TestAitableTopLevelAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		wantTool  string
		wantParam string
		wantValue any
	}{
		{
			name:      "search",
			args:      []string{"search", "--query", "测试"},
			wantTool:  "search_bases",
			wantParam: "query",
			wantValue: "测试",
		},
		{
			name:      "search keyword",
			args:      []string{"search", "--keyword", "测试"},
			wantTool:  "search_bases",
			wantParam: "query",
			wantValue: "测试",
		},
		{
			name:      "create",
			args:      []string{"create", "--name", "项目跟踪"},
			wantTool:  "create_base",
			wantParam: "baseName",
			wantValue: "项目跟踪",
		},
		{
			name:      "info",
			args:      []string{"info", "--base-id", "BASE_001"},
			wantTool:  "get_base",
			wantParam: "baseId",
			wantValue: "BASE_001",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &aitableCommandRunner{}
			cmd := aitableHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
			}
			if runner.last.Tool != tc.wantTool {
				t.Fatalf("tool = %q, want %q", runner.last.Tool, tc.wantTool)
			}
			if got := runner.last.Params[tc.wantParam]; got != tc.wantValue {
				t.Fatalf("%s = %#v, want %#v", tc.wantParam, got, tc.wantValue)
			}
		})
	}
}

func TestAitableListAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		wantTool  string
		wantParam string
		wantValue any
	}{
		{
			name:      "table list",
			args:      []string{"table", "list", "--base-id", "BASE_001", "--table-ids", "TABLE_001"},
			wantTool:  "get_tables",
			wantParam: "baseId",
			wantValue: "BASE_001",
		},
		{
			name:      "field list",
			args:      []string{"field", "list", "--base-id", "BASE_001", "--table-id", "TABLE_001", "--field-ids", "FIELD_001"},
			wantTool:  "get_fields",
			wantParam: "tableId",
			wantValue: "TABLE_001",
		},
		{
			name:      "record list",
			args:      []string{"record", "list", "--base-id", "BASE_001", "--table-id", "TABLE_001", "--page-size", "2"},
			wantTool:  "query_records",
			wantParam: "limit",
			wantValue: 2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &aitableCommandRunner{}
			cmd := aitableHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
			}
			if runner.last.Tool != tc.wantTool {
				t.Fatalf("tool = %q, want %q", runner.last.Tool, tc.wantTool)
			}
			if got := runner.last.Params[tc.wantParam]; got != tc.wantValue {
				t.Fatalf("%s = %#v, want %#v", tc.wantParam, got, tc.wantValue)
			}
		})
	}
}

func TestAitableFieldCreateAcceptsCompatibilityFlags(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableFieldCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--field-name", "AI 摘要",
		"--field-type", "text",
		"--ai-config", `{"outputType":"text","prompt":[{"type":"fieldRef","fieldId":"fld1"}]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "create_fields" {
		t.Fatalf("tool = %q, want create_fields", runner.last.Tool)
	}
	field := singleAitableRecord(t, runner.last.Params["fields"])
	if got := field["fieldName"]; got != "AI 摘要" {
		t.Fatalf("fieldName = %#v, want AI 摘要", got)
	}
	if got := field["type"]; got != "text" {
		t.Fatalf("type = %#v, want text", got)
	}
	aiConfig, ok := field["aiConfig"].(map[string]any)
	if !ok {
		t.Fatalf("aiConfig type = %T, want map[string]any", field["aiConfig"])
	}
	if got := aiConfig["outputType"]; got != "text" {
		t.Fatalf("aiConfig.outputType = %#v, want text", got)
	}
}

func TestAitableFieldCreateNormalizesFieldsJSON(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableFieldCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--fields", `[{"name":"授课老师","type":"Text"},{"fieldName":"执行人","type":"member"}]`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	fields, ok := runner.last.Params["fields"].([]any)
	if !ok {
		t.Fatalf("fields type = %T, want []any", runner.last.Params["fields"])
	}
	if len(fields) != 2 {
		t.Fatalf("fields len = %d, want 2", len(fields))
	}
	first, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatalf("field[0] type = %T, want map[string]any", fields[0])
	}
	if got := first["fieldName"]; got != "授课老师" {
		t.Fatalf("fieldName = %#v, want 授课老师", got)
	}
	if _, ok := first["name"]; ok {
		t.Fatalf("unexpected name key in normalized field: %#v", first)
	}
	if got := first["type"]; got != "text" {
		t.Fatalf("type = %#v, want text", got)
	}
	second, ok := fields[1].(map[string]any)
	if !ok {
		t.Fatalf("field[1] type = %T, want map[string]any", fields[1])
	}
	if got := second["type"]; got != "user" {
		t.Fatalf("member alias type = %#v, want user", got)
	}
}

func TestAitableTableCreateNormalizesWrappedFieldsJSON(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableTableCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--name", "课程列表",
		"--fields", `{"fields":[{"name":"课程名称","type":"TEXT"},{"name":"课时数","type":"Number"}]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	fields, ok := runner.last.Params["fields"].([]any)
	if !ok {
		t.Fatalf("fields type = %T, want []any", runner.last.Params["fields"])
	}
	first, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatalf("field[0] type = %T, want map[string]any", fields[0])
	}
	if got := first["fieldName"]; got != "课程名称" {
		t.Fatalf("fieldName = %#v, want 课程名称", got)
	}
	if got := first["type"]; got != "text" {
		t.Fatalf("type = %#v, want text", got)
	}
	second, ok := fields[1].(map[string]any)
	if !ok {
		t.Fatalf("field[1] type = %T, want map[string]any", fields[1])
	}
	if got := second["type"]; got != "number" {
		t.Fatalf("type = %#v, want number", got)
	}
}

func TestAitableTableCreateDefaultsFieldsToEmptyArray(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableTableCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--name", "空表",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	fields, ok := runner.last.Params["fields"].([]any)
	if !ok {
		t.Fatalf("fields type = %T, want []any", runner.last.Params["fields"])
	}
	if len(fields) != 0 {
		t.Fatalf("fields len = %d, want 0", len(fields))
	}
}

func TestAitableFieldUpdatePassesAIConfig(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableFieldUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--field-id", "FIELD_001",
		"--ai-config", `{"outputType":"text"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "update_field" {
		t.Fatalf("tool = %q, want update_field", runner.last.Tool)
	}
	aiConfig, ok := runner.last.Params["aiConfig"].(map[string]any)
	if !ok {
		t.Fatalf("aiConfig = %#v, want object", runner.last.Params["aiConfig"])
	}
	if got := aiConfig["outputType"]; got != "text" {
		t.Fatalf("outputType = %#v, want text", got)
	}
}

func TestAitableRecordCreateAcceptsBaseFieldsAndRecordsFile(t *testing.T) {
	t.Parallel()

	recordsFile := filepath.Join(t.TempDir(), "records.json")
	if err := os.WriteFile(recordsFile, []byte(`[{"cells":{"fldX":"from-file"}}]`), 0600); err != nil {
		t.Fatal(err)
	}

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base", "BASE_001",
		"--table-id", "TABLE_001",
		"--fields", `[{"cells":{"fldX":"from-fields"}}]`,
		"--records-file", recordsFile,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["baseId"]; got != "BASE_001" {
		t.Fatalf("baseId = %#v, want BASE_001", got)
	}
	record := singleAitableRecord(t, runner.last.Params["records"])
	cells, ok := record["cells"].(map[string]any)
	if !ok {
		t.Fatalf("cells type = %T, want map[string]any", record["cells"])
	}
	if got := cells["fldX"]; got != "from-file" {
		t.Fatalf("cells[fldX] = %#v, want from-file", got)
	}
}

func TestAitableRecordGetQueriesRecordIDs(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordGetCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--record-ids", "REC_001, REC_002",
		"--field-ids", "FIELD_001",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "query_records" {
		t.Fatalf("tool = %q, want query_records", runner.last.Tool)
	}
	if got := runner.last.Params["baseId"]; got != "BASE_001" {
		t.Fatalf("baseId = %#v, want BASE_001", got)
	}
	if got := runner.last.Params["tableId"]; got != "TABLE_001" {
		t.Fatalf("tableId = %#v, want TABLE_001", got)
	}
	recordIDs, ok := runner.last.Params["recordIds"].([]string)
	if !ok {
		t.Fatalf("recordIds type = %T, want []string", runner.last.Params["recordIds"])
	}
	if strings.Join(recordIDs, ",") != "REC_001,REC_002" {
		t.Fatalf("recordIds = %#v, want REC_001,REC_002", recordIDs)
	}
	fieldIDs, ok := runner.last.Params["fieldIds"].([]string)
	if !ok {
		t.Fatalf("fieldIds type = %T, want []string", runner.last.Params["fieldIds"])
	}
	if strings.Join(fieldIDs, ",") != "FIELD_001" {
		t.Fatalf("fieldIds = %#v, want FIELD_001", fieldIDs)
	}
}

func TestAitableRecordQueryAcceptsPageTokenAlias(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordQueryCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--page-token", "CURSOR_001",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if got := runner.last.Params["cursor"]; got != "CURSOR_001" {
		t.Fatalf("cursor = %#v, want CURSOR_001", got)
	}
}

func TestAitableRecordQueryNormalizesFiltersAndSortOrder(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordQueryCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--filters", `{"operator":"and","operands":[{"fieldId":"FIELD_001","operator":"eq","value":"张三"}]}`,
		"--sort", `[{"fieldId":"FIELD_001","order":"desc"}]`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	filters, ok := runner.last.Params["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters = %#v, want object", runner.last.Params["filters"])
	}
	operands, ok := filters["operands"].([]any)
	if !ok || len(operands) != 1 {
		t.Fatalf("filters.operands = %#v, want single operand", filters["operands"])
	}
	first, ok := operands[0].(map[string]any)
	if !ok {
		t.Fatalf("operand type = %T, want object", operands[0])
	}
	if _, ok := first["fieldId"]; ok {
		t.Fatalf("unexpected fieldId in normalized operand: %#v", first)
	}
	operandValues, ok := first["operands"].([]any)
	if !ok || len(operandValues) != 2 {
		t.Fatalf("operand values = %#v, want [field value]", first["operands"])
	}
	sortItems, ok := runner.last.Params["sort"].([]any)
	if !ok || len(sortItems) != 1 {
		t.Fatalf("sort = %#v, want single item", runner.last.Params["sort"])
	}
	sortFirst, ok := sortItems[0].(map[string]any)
	if !ok {
		t.Fatalf("sort item type = %T, want object", sortItems[0])
	}
	if got := sortFirst["direction"]; got != "desc" {
		t.Fatalf("direction = %#v, want desc", got)
	}
	if _, ok := sortFirst["order"]; ok {
		t.Fatalf("unexpected order key in sort: %#v", sortFirst)
	}
}

func TestAitableRecordGetRequiresRecordIDs(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordGetCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want missing --record-ids validation")
	}
	if !strings.Contains(err.Error(), "record-ids") || !strings.Contains(err.Error(), "required") {
		t.Fatalf("error = %q, want record-ids required", err.Error())
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestAitableRecordGetRejectsQueryFlags(t *testing.T) {
	t.Parallel()

	queryOnlyFlags := []string{"filters", "sort", "query", "cursor", "limit"}
	for _, flag := range queryOnlyFlags {
		flag := flag
		t.Run(flag, func(t *testing.T) {
			t.Parallel()

			runner := &aitableCommandRunner{}
			cmd := newAitableRecordGetCommand(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs([]string{
				"--base-id", "BASE_001",
				"--table-id", "TABLE_001",
				"--record-ids", "REC_001",
				"--" + flag, "x",
			})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() error = nil, want unknown flag --%s", flag)
			}
			if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--"+flag) {
				t.Fatalf("error = %q, want unknown flag --%s", err.Error(), flag)
			}
			if runner.last.Tool != "" {
				t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
			}
		})
	}
}

func TestAitableRecordUpdateAcceptsRecordsFile(t *testing.T) {
	t.Parallel()

	recordsFile := filepath.Join(t.TempDir(), "records.json")
	if err := os.WriteFile(recordsFile, []byte(`[{"recordId":"REC_FILE","cells":{"fldX":"from-file"}}]`), 0600); err != nil {
		t.Fatal(err)
	}

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--records-file", recordsFile,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	record := singleAitableUpdateRecord(t, runner.last.Params["records"])
	if got := record["recordId"]; got != "REC_FILE" {
		t.Fatalf("recordId = %#v, want REC_FILE", got)
	}
}

func TestAitableAttachmentUploadRequiresSize(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAITableAttachmentUploadCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--file-name", "test.png",
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want missing --size validation")
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestAitableChartCreateRequiresConfigAndLayout(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableChartCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--dashboard-id", "DASHBOARD_001",
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want missing config/layout validation")
	}
	if runner.last.Tool != "" {
		t.Fatalf("tool = %q, want no invocation", runner.last.Tool)
	}
}

func TestAitableViewCreatePassesWukongDescriptionAndSubtype(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableViewCreateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--view-type", "Grid",
		"--view-sub-type", "default",
		"--desc", `{"text":"描述"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "create_view" {
		t.Fatalf("tool = %q, want create_view", runner.last.Tool)
	}
	if got := runner.last.Params["viewSubType"]; got != "default" {
		t.Fatalf("viewSubType = %#v, want default", got)
	}
	desc, ok := runner.last.Params["viewDescription"].(map[string]any)
	if !ok {
		t.Fatalf("viewDescription = %#v, want object", runner.last.Params["viewDescription"])
	}
	if got := desc["text"]; got != "描述" {
		t.Fatalf("desc.text = %#v, want 描述", got)
	}
	if _, ok := runner.last.Params["desc"]; ok {
		t.Fatalf("unexpected desc param in %#v", runner.last.Params)
	}
}

func TestAitableViewUpdateUsesWukongToolParams(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableViewUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--view-id", "VIEW_001",
		"--name", "新视图",
		"--desc", `{"text":"描述"}`,
		"--config", `{"visibleFieldIds":["fld1","fld2"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "update_view" {
		t.Fatalf("tool = %q, want update_view", runner.last.Tool)
	}
	if got := runner.last.Params["newViewName"]; got != "新视图" {
		t.Fatalf("newViewName = %#v, want 新视图", got)
	}
	desc, ok := runner.last.Params["viewDescription"].(map[string]any)
	if !ok {
		t.Fatalf("viewDescription = %#v, want object", runner.last.Params["viewDescription"])
	}
	if got := desc["text"]; got != "描述" {
		t.Fatalf("desc.text = %#v, want 描述", got)
	}
	config, ok := runner.last.Params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want object", runner.last.Params["config"])
	}
	if _, ok := config["visibleFieldIds"]; !ok {
		t.Fatalf("config missing visibleFieldIds: %#v", config)
	}
}

func TestAitableRecordDeleteCallsDeleteOnlyOnce(t *testing.T) {
	t.Parallel()

	runner := &aitableRetryProbeRunner{}
	cmd := newAitableRecordDeleteCommand(runner)
	cmd.Flags().Bool("yes", false, "yes")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--record-ids", "REC_001",
		"--yes",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if got := runner.result.Invocation.Tool; got != "delete_records" {
		t.Fatalf("tool = %q, want delete_records", got)
	}
}

func TestAitableViewListUsesWukongGetViewsTool(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableViewListCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--view-ids", "VIEW_001,VIEW_002",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "get_views" {
		t.Fatalf("tool = %q, want get_views", runner.last.Tool)
	}
	viewIDs, ok := runner.last.Params["viewIds"].([]string)
	if !ok {
		t.Fatalf("viewIds = %#v, want []string", runner.last.Params["viewIds"])
	}
	if len(viewIDs) != 2 || viewIDs[0] != "VIEW_001" || viewIDs[1] != "VIEW_002" {
		t.Fatalf("viewIds = %#v, want [VIEW_001 VIEW_002]", viewIDs)
	}
}

func TestAitableDashboardAndChartHelpersUseWukongTools(t *testing.T) {
	t.Parallel()

	t.Run("dashboard create name shortcut", func(t *testing.T) {
		runner := &aitableCommandRunner{}
		cmd := newAitableDashboardCreateCommand(runner)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{
			"--base-id", "BASE_001",
			"--name", "销售看板",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
		}
		if runner.last.Tool != "create_dashboard" {
			t.Fatalf("tool = %q, want create_dashboard", runner.last.Tool)
		}
		config, ok := runner.last.Params["config"].(map[string]any)
		if !ok {
			t.Fatalf("config = %#v, want object", runner.last.Params["config"])
		}
		if got := config["dashboardName"]; got != "销售看板" {
			t.Fatalf("dashboardName = %#v, want 销售看板", got)
		}
	})

	t.Run("dashboard update name shortcut", func(t *testing.T) {
		runner := &aitableCommandRunner{}
		cmd := newAitableDashboardUpdateCommand(runner)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{
			"--base-id", "BASE_001",
			"--dashboard-id", "DASHBOARD_001",
			"--name", "新看板",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
		}
		if runner.last.Tool != "update_dashboard" {
			t.Fatalf("tool = %q, want update_dashboard", runner.last.Tool)
		}
		config, ok := runner.last.Params["config"].(map[string]any)
		if !ok {
			t.Fatalf("config = %#v, want object", runner.last.Params["config"])
		}
		if got := config["dashboardName"]; got != "新看板" {
			t.Fatalf("dashboardName = %#v, want 新看板", got)
		}
	})

	t.Run("dashboard share update", func(t *testing.T) {
		runner := &aitableCommandRunner{}
		cmd := newAitableDashboardShareUpdateCommand(runner)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{
			"--base-id", "BASE_001",
			"--dashboard-id", "DASHBOARD_001",
			"--enabled", "true",
			"--share-type", "PUBLIC",
			"--allow-back-to-doc", "true",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
		}
		if runner.last.Tool != "update_dashboard_share" {
			t.Fatalf("tool = %q, want update_dashboard_share", runner.last.Tool)
		}
		if got := runner.last.Params["shareType"]; got != "PUBLIC" {
			t.Fatalf("shareType = %#v, want PUBLIC", got)
		}
		if got := runner.last.Params["allowBackToDoc"]; got != true {
			t.Fatalf("allowBackToDoc = %#v, want true", got)
		}
	})

	t.Run("chart widgets example", func(t *testing.T) {
		runner := &aitableCommandRunner{}
		cmd := newAitableChartWidgetsExampleCommand(runner)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
		}
		if runner.last.Tool != "get_dashboard_widgets_example" {
			t.Fatalf("tool = %q, want get_dashboard_widgets_example", runner.last.Tool)
		}
	})
}

func TestAitableRecordUpdateAcceptsRecordIDAndCells(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--record-id", "REC_001",
		"--cells", `{"fldX":"值"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	record := singleAitableUpdateRecord(t, runner.last.Params["records"])
	if got := record["recordId"]; got != "REC_001" {
		t.Fatalf("recordId = %#v, want REC_001", got)
	}
	cells, ok := record["cells"].(map[string]any)
	if !ok {
		t.Fatalf("cells type = %T, want map[string]any", record["cells"])
	}
	if got := cells["fldX"]; got != "值" {
		t.Fatalf("cells[fldX] = %#v, want 值", got)
	}
}

func TestAitableRecordUpdateAcceptsDataCellsWithRecordID(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--record-id", "REC_002",
		"--data", `{"fldY":"新值"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	record := singleAitableUpdateRecord(t, runner.last.Params["records"])
	if got := record["recordId"]; got != "REC_002" {
		t.Fatalf("recordId = %#v, want REC_002", got)
	}
	cells, ok := record["cells"].(map[string]any)
	if !ok {
		t.Fatalf("cells type = %T, want map[string]any", record["cells"])
	}
	if got := cells["fldY"]; got != "新值" {
		t.Fatalf("cells[fldY] = %#v, want 新值", got)
	}
}

func TestAitableRecordUpdateAcceptsDataRecordsArray(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordUpdateCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--data", `[{"recordId":"REC_003","cells":{"fldZ":"done"}}]`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	record := singleAitableUpdateRecord(t, runner.last.Params["records"])
	if got := record["recordId"]; got != "REC_003" {
		t.Fatalf("recordId = %#v, want REC_003", got)
	}
}

func singleAitableUpdateRecord(t *testing.T, value any) map[string]any {
	t.Helper()

	return singleAitableRecord(t, value)
}

func singleAitableRecord(t *testing.T, value any) map[string]any {
	t.Helper()

	records, ok := value.([]any)
	if !ok {
		t.Fatalf("records type = %T, want []any", value)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record, ok := records[0].(map[string]any)
	if !ok {
		t.Fatalf("record type = %T, want map[string]any", records[0])
	}
	return record
}
