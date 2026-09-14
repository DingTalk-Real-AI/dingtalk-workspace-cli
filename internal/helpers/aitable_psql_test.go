package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func runPsqlCLI(t *testing.T, caller *recordQueryE2ECaller, args ...string) (string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	InitDeps(caller)
	out := &bytes.Buffer{}
	cmd := newAitablePsqlCommand()
	cmd.PersistentFlags().String("format", "json", "test output format")
	cmd.PersistentFlags().String("jq", "", "test jq expression")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	ctx, _ := output.WithResultStore(context.Background())
	executed, returnValue := cmd.ExecuteContextC(ctx)
	if returnValue == nil {
		_, _, returnValue = output.EmitStoredResult(executed)
	}
	return out.String(), returnValue
}

func TestAitablePsqlExplicitJSONUsesUnifiedEnvelope(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":[{"tableId":"tbl1","tableName":"项目表"}]}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-l", "--format", "json")
	if err != nil {
		t.Fatalf("psql json failed: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("psql json output is invalid: %v\n%s", err, out)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	data, ok := envelope["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("unexpected envelope data: %#v", envelope["data"])
	}
}

func TestAitablePsqlDeclaresUnifiedResultContract(t *testing.T) {
	cmd := newAitablePsqlCommand()
	if output.CommandRollout(cmd) != output.RolloutUnifiedActive {
		t.Fatalf("rollout = %q", output.CommandRollout(cmd))
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Result == nil || len(final.Result.DataSchema) == 0 {
		t.Fatalf("missing reviewed result contract: %#v", final)
	}
	if final.DryRun == nil || final.DryRun.PreviewKind != "request" || final.DryRun.RemoteReads {
		t.Fatalf("dry-run contract = %#v", final.DryRun)
	}
}

func TestAitablePsqlJQEvaluatesUnifiedEnvelope(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":{"columns":[{"columnName":"number"},{"columnName":"flag"},{"columnName":"empty"}],"rows":[[1,true,null]],"rowCount":1,"truncated":false}}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-c", "SELECT 1", "--jq", ".data.rows")
	if err != nil {
		t.Fatalf("psql jq failed: %v", err)
	}
	var rows []any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("psql jq output is invalid: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected jq rows: %#v", rows)
	}
}

func TestAitablePsqlStructuredOutputRejectsMalformedBusinessData(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":{"columns":[]}}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-c", "SELECT 1", "--format", "json")
	if err == nil || !strings.Contains(err.Error(), "missing rows") {
		t.Fatalf("error = %v, output = %q", err, out)
	}
	if out != "" {
		t.Fatalf("malformed result leaked output: %q", out)
	}
}

func TestAitablePsqlListTables(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":[{"tableId":"tbl1","tableName":"项目表","description":"项目进度管理"}]}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-l")
	if err != nil {
		t.Fatalf("psql list failed: %v", err)
	}
	if !strings.Contains(out, "项目表") || !strings.Contains(out, "tbl1") ||
		!strings.Contains(out, "Description") || !strings.Contains(out, "项目进度管理") {
		t.Fatalf("unexpected output: %s", out)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "otable_pg_list_tables" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
}

func TestAitablePsqlDescribeAllProperties(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":{"tableId":"tbl1","tableName":"项目表","description":"项目进度管理","columns":[{"columnName":"人员字段1_id","pgType":"text[]","fieldId":"fld1","attribute":"id","description":"项目负责人"}]}}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-t", "tbl1", "--all-properties")
	if err != nil {
		t.Fatalf("psql describe failed: %v", err)
	}
	if !strings.Contains(out, "人员字段1_id") || !strings.Contains(out, "项目进度管理") ||
		!strings.Contains(out, "项目负责人") || caller.calls[0].args["allProperties"] != true {
		t.Fatalf("unexpected output/call: %s %#v", out, caller.calls)
	}
}

func TestAitablePsqlExecuteExpanded(t *testing.T) {
	caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
		`{"status":"success","data":{"columns":[{"columnName":"人员字段1_id","pgType":"text[]"}],"rows":[[["user_001","user_002"]]],"rowCount":1,"truncated":false}}`)}}}
	out, err := runPsqlCLI(t, caller, "-d", "base1", "-x", "-c", "SELECT 人员字段1_id FROM 项目表")
	if err != nil {
		t.Fatalf("psql execute failed: %v", err)
	}
	if !strings.Contains(out, "-[ RECORD 1 ]-") || !strings.Contains(out, `["user_001","user_002"]`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if caller.calls[0].tool != "otable_pg_execute" || caller.calls[0].args["limit"] != 200 {
		t.Fatalf("unexpected call: %#v", caller.calls[0])
	}
	if _, exists := caller.calls[0].args["tableId"]; exists {
		t.Fatalf("execute should not send tableId: %#v", caller.calls[0])
	}
}

func TestAitablePsqlWarnsWhenResultIsTruncated(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		t.Run(map[bool]string{false: "table", true: "expanded"}[expanded], func(t *testing.T) {
			caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(
				`{"status":"success","data":{"columns":[{"columnName":"名称","pgType":"text"}],"rows":[["记录1"]],"rowCount":1,"truncated":true}}`)}}}
			args := []string{"-d", "base1", "-c", "SELECT 名称 FROM 项目表"}
			if expanded {
				args = append(args, "-x")
			}
			out, err := runPsqlCLI(t, caller, args...)
			if err != nil {
				t.Fatalf("psql execute failed: %v", err)
			}
			if !strings.Contains(out, "Warning: result truncated") || !strings.Contains(out, "increase --limit") {
				t.Fatalf("missing truncation warning: %s", out)
			}
		})
	}
}

func TestAitablePsqlRejectsAmbiguousMode(t *testing.T) {
	out, err := runPsqlCLI(t, &recordQueryE2ECaller{}, "-d", "base1", "-l", "-t", "tbl1")
	if err == nil || !strings.Contains(err.Error(), "exactly one mode") {
		t.Fatalf("error = %v, output = %s", err, out)
	}
}

type psqlFailingWriter struct{}

func (psqlFailingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestAitablePsqlValidationErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing database", args: []string{"-l"}, want: "missing required flag"},
		{name: "missing mode", args: []string{"-d", "base1"}, want: "exactly one mode"},
		{name: "invalid limit", args: []string{"-d", "base1", "-l", "--limit", "0"}, want: "--limit must be between"},
		{name: "invalid timeout", args: []string{"-d", "base1", "-l", "--timeout", "61"}, want: "--timeout must be between"},
		{name: "list plus command", args: []string{"-d", "base1", "-l", "-c", "SELECT 1"}, want: "exactly one mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := runPsqlCLI(t, &recordQueryE2ECaller{}, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCallAitablePsqlToolValidatesMCPResponses(t *testing.T) {
	for _, test := range []struct {
		name string
		step recordQueryE2EStep
		want string
	}{
		{name: "caller error", step: recordQueryE2EStep{err: errors.New("transport failed")}, want: "transport failed"},
		{name: "nil result", step: recordQueryE2EStep{}, want: "nil result"},
		{name: "no text", step: recordQueryE2EStep{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "ignored"}}}}, want: "no text content"},
		{name: "invalid json", step: recordQueryE2EStep{result: textToolResult("{")}, want: "invalid JSON"},
		{name: "multiple json values", step: recordQueryE2EStep{result: textToolResult(`{"data":{}} {"data":{}}`)}, want: "multiple JSON values"},
		{name: "generic mcp error", step: recordQueryE2EStep{result: textToolResult(`{"status":"error"}`)}, want: "MCP tool returned an error"},
		{name: "detailed mcp error", step: recordQueryE2EStep{result: textToolResult(`{"status":"ERROR","error":{"message":"denied"}}`)}, want: "denied"},
		{name: "missing data", step: recordQueryE2EStep{result: textToolResult(`{"status":"success"}`)}, want: "returned no data"},
	} {
		t.Run(test.name, func(t *testing.T) {
			testseam.Protect(t, &deps)
			InitDeps(&recordQueryE2ECaller{steps: []recordQueryE2EStep{test.step}})
			_, err := callAitablePsqlTool(context.Background(), "demo", map[string]any{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAitablePsqlRenderValidationAndValues(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*bytes.Buffer) error
		want string
	}{
		{name: "tables root", run: func(out *bytes.Buffer) error { return renderPgTables(out, map[string]any{}) }, want: "must be an array"},
		{name: "tables item", run: func(out *bytes.Buffer) error { return renderPgTables(out, []any{"bad"}) }, want: "table 0 must be an object"},
		{name: "tables identity", run: func(out *bytes.Buffer) error { return renderPgTables(out, []any{map[string]any{}}) }, want: "tableName and tableId"},
		{name: "schema root", run: func(out *bytes.Buffer) error { return renderPgSchema(out, []any{}) }, want: "must be an object"},
		{name: "schema identity", run: func(out *bytes.Buffer) error { return renderPgSchema(out, map[string]any{}) }, want: "missing tableId or tableName"},
		{name: "schema columns", run: func(out *bytes.Buffer) error {
			return renderPgSchema(out, map[string]any{"tableId": "t", "tableName": "n"})
		}, want: "missing columns"},
		{name: "schema item", run: func(out *bytes.Buffer) error {
			return renderPgSchema(out, map[string]any{"tableId": "t", "tableName": "n", "columns": []any{"bad"}})
		}, want: "column 0 must be an object"},
		{name: "query root", run: func(out *bytes.Buffer) error { return renderPgQuery(out, []any{}, false) }, want: "must be an object"},
		{name: "query columns", run: func(out *bytes.Buffer) error { return renderPgQuery(out, map[string]any{}, false) }, want: "missing columns"},
		{name: "query column", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{"bad"}}, false)
		}, want: "query column 0 must be an object"},
		{name: "query column name", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{map[string]any{}}}, false)
		}, want: "non-empty columnName"},
		{name: "query rows", run: func(out *bytes.Buffer) error { return renderPgQuery(out, map[string]any{"columns": []any{}}, false) }, want: "missing rows"},
		{name: "query row", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{}, "rows": []any{"bad"}}, false)
		}, want: "query row 0 must be an array"},
		{name: "query row width", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{map[string]any{"columnName": "a"}}, "rows": []any{[]any{}}}, false)
		}, want: "values for 1 columns"},
		{name: "query row count", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{}, "rows": []any{}}, false)
		}, want: "rowCount must be an integer"},
		{name: "query truncated", run: func(out *bytes.Buffer) error {
			return renderPgQuery(out, map[string]any{"columns": []any{}, "rows": []any{}, "rowCount": json.Number("0")}, false)
		}, want: "truncated must be a boolean"},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			if err := test.run(out); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	if err := renderPgQuery(psqlFailingWriter{}, map[string]any{"columns": []any{}, "rows": []any{}, "rowCount": json.Number("0"), "truncated": false}, false); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("renderPgQuery write error = %v", err)
	}
	if err := printPgTable(psqlFailingWriter{}, []string{"header"}, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("printPgTable write error = %v", err)
	}
	for _, test := range []struct {
		value any
		want  string
	}{
		{value: nil, want: ""},
		{value: "text", want: "text"},
		{value: 7, want: "7"},
		{value: []any{"a"}, want: `["a"]`},
		{value: map[string]any{"a": "b"}, want: `{"a":"b"}`},
	} {
		if got := pgValue(test.value); got != test.want {
			t.Errorf("pgValue(%#v) = %q, want %q", test.value, got, test.want)
		}
	}
}
