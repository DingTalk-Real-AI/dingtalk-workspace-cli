package helpers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type aitableCommandRunner struct {
	last executor.Invocation
}

func (r *aitableCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
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
