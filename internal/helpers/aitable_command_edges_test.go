package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableCommandCoverageCaller struct {
	viewType string
	err      error
	response map[string]string
}

type aitableCommandContextKey struct{}

type aitableCommandContextCaller struct {
	value any
}

func (c *aitableCommandContextCaller) CallTool(ctx context.Context, _, _ string, _ map[string]any) (*edition.ToolResult, error) {
	c.value = ctx.Value(aitableCommandContextKey{})
	return nil, context.Canceled
}

func (*aitableCommandContextCaller) Format() string { return "json" }
func (*aitableCommandContextCaller) DryRun() bool   { return false }
func (*aitableCommandContextCaller) Fields() string { return "" }
func (*aitableCommandContextCaller) JQ() string     { return "" }

func (c *aitableCommandCoverageCaller) CallTool(_ context.Context, _, tool string, _ map[string]any) (*edition.ToolResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	if response, ok := c.response[tool]; ok {
		return textToolResult(response), nil
	}
	viewType := c.viewType
	if viewType == "" {
		viewType = "Grid"
	}
	var response string
	switch tool {
	case "get_views":
		response = fmt.Sprintf(`{"data":{"views":[{"viewId":"view","viewType":%q,"kanbanCard":{},"galleryCard":{},"ganttTimebar":{},"aggregate":{},"filter":[],"sort":[],"group":[],"visibleFieldIds":[],"custom":{"widthMap":{}}}]}}`, viewType)
	case "list_form_views":
		response = `{"data":[{"viewId":"view","title":"Form"}]}`
	case "query_records":
		response = `{"data":{"records":[],"hasMore":false,"nextCursor":""}}`
	default:
		response = `{"success":true,"data":{}}`
	}
	return textToolResult(response), nil
}

func (*aitableCommandCoverageCaller) Format() string { return "json" }
func (*aitableCommandCoverageCaller) DryRun() bool   { return false }
func (*aitableCommandCoverageCaller) Fields() string { return "" }
func (*aitableCommandCoverageCaller) JQ() string     { return "" }

func runAitableCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func TestCrossPlatformCoverageAitableRetryWrappersExhaustAndRecover(t *testing.T) {
	oldDeps, oldSleep := deps, helperSleep
	t.Cleanup(func() { deps, helperSleep = oldDeps, oldSleep })
	helperSleep = func(time.Duration) {}
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Time{}
		return ready
	})

	retryable := fmt.Errorf("timeout: retryable: true")
	caller := &aitableTestCaller{errors: []error{retryable, retryable, retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableTool("get_base", nil); err == nil {
		t.Fatal("exhausted aitable retries returned nil")
	}

	caller = &aitableTestCaller{errors: []error{retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableHelperTool("list_workflows", nil); err != nil {
		t.Fatalf("helper retry did not recover: %v", err)
	}

	caller = &aitableTestCaller{errors: []error{retryable, retryable, retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableHelperTool("list_workflows", nil); err == nil {
		t.Fatal("exhausted helper retries returned nil")
	}

	for _, tc := range []struct {
		name   string
		tool   string
		helper bool
	}{
		{name: "main write", tool: "create_records"},
		{name: "helper write", tool: "record_upsert", helper: true},
	} {
		t.Run(tc.name+" is never replayed", func(t *testing.T) {
			writeCaller := &aitableTestCaller{errors: []error{retryable}}
			installAitableDeps(t, writeCaller)
			var err error
			if tc.helper {
				err = callAitableHelperTool(tc.tool, nil)
			} else {
				err = callAitableTool(tc.tool, nil)
			}
			if err == nil {
				t.Fatal("write timeout should remain unknown to the caller")
			}
			if got := len(writeCaller.calls); got != 1 {
				t.Fatalf("write call count = %d, want 1", got)
			}
		})
	}

	caller = &aitableTestCaller{}
	installAitableDeps(t, caller)
	if err := callAitableToolContext(nil, "get_base", nil); err != nil {
		t.Fatalf("nil context was not normalized: %v", err)
	}

	caller = &aitableTestCaller{errors: []error{retryable}}
	installAitableDeps(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	backoffPending := make(chan time.Time)
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		cancel()
		return backoffPending
	})
	if err := callAitableToolContext(ctx, "get_base", nil); err != context.Canceled {
		t.Fatalf("cancel during retry backoff = %v, want %v", err, context.Canceled)
	}
}

func TestCrossPlatformCoverageAitableFieldListPreservesCommandContext(t *testing.T) {
	old := deps
	t.Cleanup(func() { deps = old })
	caller := &aitableCommandContextCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard

	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"field", "list", "--base-id=b", "--table-id=t"})
	ctx := context.WithValue(context.Background(), aitableCommandContextKey{}, "field-list-context")
	if err := root.ExecuteContext(ctx); err == nil {
		t.Fatal("field list context probe unexpectedly succeeded")
	}
	if caller.value != "field-list-context" {
		t.Fatalf("field list caller context value = %#v", caller.value)
	}
}

func TestCrossPlatformCoverageAitableCommandValidationEdges(t *testing.T) {
	oldDeps, oldArgs, oldStdin, oldSleep := deps, os.Args, os.Stdin, helperSleep
	t.Cleanup(func() {
		deps, os.Args, os.Stdin, helperSleep = oldDeps, oldArgs, oldStdin, oldSleep
	})
	helperSleep = func(time.Duration) {}
	os.Args = []string{"dws", "aitable", "--yes"}
	caller := &aitableCommandCoverageCaller{}

	manyIDs := make([]string, 101)
	for i := range manyIDs {
		manyIDs[i] = fmt.Sprintf("r%d", i)
	}
	manyRecords := "[" + strings.TrimSuffix(strings.Repeat(`{"cells":{}},`, 101), ",") + "]"

	scenarios := []struct {
		name string
		args []string
	}{
		{"primary doc missing base", []string{"base", "get-primary-doc-id", "--table-id=t", "--record-id=r"}},
		{"table create invalid fields", []string{"table", "create", "--base-id=b", "--name=n", "--fields={"}},
		{"table create missing base", []string{"table", "create", "--name=n", `--fields=[{"fieldName":"N","type":"text"}]`}},
		{"field create invalid fields", []string{"field", "create", "--base-id=b", "--table-id=t", "--fields={"}},
		{"field create invalid configured options", []string{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=singleSelect", "--config={}", "--options={"}},
		{"field create configured options", []string{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=singleSelect", "--config={}", `--options=[{"name":"A"}]`}},
		{"field create scalar config", []string{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=text", "--config=[]"}},
		{"field create invalid options", []string{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=singleSelect", "--options={"}},
		{"field create options and ai", []string{"field", "create", "--base-id=b", "--table-id=t", "--name=n", "--type=singleSelect", `--options=[{"name":"A"}]`, `--ai-config={"enabled":true}`}},
		{"field update no changes", []string{"field", "update", "--base-id=b", "--table-id=t", "--field-id=f"}},
		{"query explicitly empty table", []string{"record", "query", "--base-id=b", "--table-id="}},
		{"query rich options", []string{"record", "query", "--base-id=b", "--table-id=t", "--view-id=v", "--record-ids=r1,r2", "--field-ids=f1,f2", `--filters={"operator":"and","operands":[{"operator":"eq","operands":["f","v"]}]}`, `--sort=[{"fieldId":"f","order":"desc"},{"fieldId":"g","order":"asc","direction":"desc"}]`, "--keyword=k", "--page-size=2", "--cursor=c"}},
		{"query primary options", []string{"record", "query", "--base-id=b", "--table-id=t", "--query=k", "--limit=2"}},
		{"query invalid sort", []string{"record", "query", "--base-id=b", "--table-id=t", "--sort={"}},
		{"query all default page limit", []string{"record", "query", "--base-id=b", "--table-id=t", "--all"}},
		{"query all unlimited", []string{"record", "query", "--base-id=b", "--table-id=t", "--all", "--page-limit=0"}},
		{"create invalid cells", []string{"record", "create", "--base-id=b", "--table-id=t", "--cells={"}},
		{"create cells shortcut", []string{"record", "create", "--base-id=b", "--table-id=t", `--cells={"f":"v"}`}},
		{"create invalid records", []string{"record", "create", "--base-id=b", "--table-id=t", "--records={"}},
		{"update half shortcut", []string{"record", "update", "--base-id=b", "--table-id=t", "--record-id=r"}},
		{"update invalid shortcut cells", []string{"record", "update", "--base-id=b", "--table-id=t", "--record-id=r", "--cells={"}},
		{"update shortcut missing base", []string{"record", "update", "--table-id=t", "--record-id=r", `--cells={"f":"v"}`}},
		{"update invalid records", []string{"record", "update", "--base-id=b", "--table-id=t", "--records={"}},
		{"update records", []string{"record", "update", "--base-id=b", "--table-id=t", `--records=[{"recordId":"r","cells":{}}]`}},
		{"batch empty ids", []string{"record", "batch-update", "--base-id=b", "--table-id=t", "--record-ids=,", `--cells={"f":"v"}`}},
		{"batch too many ids", []string{"record", "batch-update", "--base-id=b", "--table-id=t", "--record-ids=" + strings.Join(manyIDs, ","), `--cells={"f":"v"}`}},
		{"batch invalid cells", []string{"record", "batch-update", "--base-id=b", "--table-id=t", "--record-ids=r", "--cells={"}},
		{"batch empty cells", []string{"record", "batch-update", "--base-id=b", "--table-id=t", "--record-ids=r", "--cells={}"}},
		{"batch missing base", []string{"record", "batch-update", "--table-id=t", "--record-ids=r", `--cells={"f":"v"}`}},
		{"query empty invalid limit", []string{"record", "query-empty", "--base-id=b", "--table-id=t", "--limit=0"}},
		{"history invalid offset", []string{"record", "history-list", "--base-id=b", "--table-id=t", "--record-id=r", "--offset=-1"}},
		{"history invalid limit", []string{"record", "history-list", "--base-id=b", "--table-id=t", "--record-id=r", "--limit=0"}},
		{"share empty ids", []string{"record", "share-url", "--base-id=b", "--table-id=t", "--record-ids=,"}},
		{"share too many ids", []string{"record", "share-url", "--base-id=b", "--table-id=t", "--record-ids=" + strings.Join(manyIDs[:21], ",")}},
		{"upsert invalid records", []string{"record", "upsert", "--base-id=b", "--table-id=t", "--records={"}},
		{"upsert empty records", []string{"record", "upsert", "--base-id=b", "--table-id=t", "--records=[]"}},
		{"upsert too many records", []string{"record", "upsert", "--base-id=b", "--table-id=t", "--records=" + manyRecords}},
		{"primary doc create missing base", []string{"record", "primary-doc-create", "--table-id=t", "--field-id=f", "--record-id=r"}},
		{"attachment all options", []string{"attachment", "upload", "--base-id=b", "--file-name=x", "--size=1", "--mime-type=text/plain"}},
		{"form update no changes", []string{"form", "update", "--base-id=b", "--table-id=t", "--view-id=v"}},
		{"form field update missing base", []string{"form", "field", "update", "--table-id=t", "--view-id=v", "--field-id=f"}},
		{"form field hide missing base", []string{"form", "field", "hide", "--table-id=t", "--view-id=v", "--field-id=f", "--hidden=true"}},
		{"form share update missing base", []string{"form", "share", "update", "--table-id=t", "--view-id=v", "--enabled=true"}},
		{"dashboard create empty", []string{"dashboard", "create", "--base-id=b"}},
		{"dashboard create invalid config scalar", []string{"dashboard", "create", "--base-id=b", "--config=[]"}},
		{"dashboard create config", []string{"dashboard", "create", "--base-id=b", `--config={"name":"n"}`}},
		{"dashboard update config", []string{"dashboard", "update", "--base-id=b", "--dashboard-id=d", `--config={"name":"n"}`}},
		{"dashboard share invalid bool", []string{"dashboard", "share", "update", "--base-id=b", "--dashboard-id=d", "--enabled=invalid"}},
		{"dashboard share missing base", []string{"dashboard", "share", "update", "--dashboard-id=d", "--enabled=true"}},
		{"dashboard share all options", []string{"dashboard", "share", "update", "--base-id=b", "--dashboard-id=d", "--enabled=true", "--share-type=PUBLIC", "--allow-back-to-doc"}},
		{"chart create missing base", []string{"chart", "create", "--dashboard-id=d", `--config={"name":"n"}`, `--layout={"x":0}`}},
		{"chart update missing base", []string{"chart", "update", "--dashboard-id=d", "--chart-id=c", `--config={"name":"n"}`}},
		{"chart share invalid bool", []string{"chart", "share", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--enabled=invalid"}},
		{"chart share missing base", []string{"chart", "share", "update", "--dashboard-id=d", "--chart-id=c", "--enabled=true"}},
		{"chart share all options", []string{"chart", "share", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--enabled=true", "--share-type=ORG", "--allow-back-to-doc"}},
		{"workflow invalid limit", []string{"workflow", "list", "--base-id=b", "--limit=0"}},
		{"workflow invalid offset", []string{"workflow", "list", "--base-id=b", "--offset=-1"}},
		{"export create task", []string{"export", "data", "--base-id=b", "--scope=all", "--format=excel", "--table-id=t", "--view-id=v", "--timeout-ms=1"}},
		{"role create invalid json", []string{"advperm", "role-create", "--base-id=b", "--name=n", "--sub-roles={"}},
		{"role create non-array", []string{"advperm", "role-create", "--base-id=b", "--name=n", "--sub-roles={}"}},
		{"role create array", []string{"advperm", "role-create", "--base-id=b", "--name=n", "--sub-roles=[]"}},
		{"role update invalid json", []string{"advperm", "role-update", "--base-id=b", "--role-id=r", "--sub-roles={"}},
		{"role update non-array", []string{"advperm", "role-update", "--base-id=b", "--role-id=r", "--sub-roles={}"}},
		{"role update array", []string{"advperm", "role-update", "--base-id=b", "--role-id=r", "--sub-roles=[]"}},
		{"import rich options", []string{"import", "data", "--import-id=i", "--table-id=t", "--timeout=1", "--header-row=2", "--src-sheet-name=Sheet1", `--field-mapping={"A":"f"}`}},
		{"import invalid mapping", []string{"import", "data", "--import-id=i", "--field-mapping={"}},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if err := runAitableCoverageCommand(t, caller, scenario.args...); err != nil {
				t.Logf("command returned: %v", err)
			}
		})
	}

	formGet := []string{"form", "get", "--base-id=b", "--table-id=t", "--view-id=view"}
	_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{err: fmt.Errorf("transport")}, formGet...)
	_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{response: map[string]string{"list_form_views": "{"}}, formGet...)
	_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{response: map[string]string{"list_form_views": `{}`}}, formGet...)
	_ = runAitableCoverageCommand(t, caller, formGet...)
}

func TestCrossPlatformCoverageAitableFormBooleanFlagsStayTyped(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		tool     string
		argName  string
		wantBool bool
	}{
		{name: "field required", args: []string{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--required=false"}, tool: "update_form_field", argName: "required", wantBool: false},
		{name: "field hidden", args: []string{"form", "field", "hide", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--hidden=true"}, tool: "update_form_field_hidden", argName: "hidden", wantBool: true},
		{name: "share enabled", args: []string{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--enabled=false"}, tool: "update_share_form", argName: "enabled", wantBool: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool {
				t.Fatalf("calls = %#v", caller.calls)
			}
			got, ok := caller.calls[0].args[tc.argName].(bool)
			if !ok || got != tc.wantBool {
				t.Fatalf("%s = %#v (%T), want bool %v", tc.argName, caller.calls[0].args[tc.argName], caller.calls[0].args[tc.argName], tc.wantBool)
			}
		})
	}

	for _, args := range [][]string{
		{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--required=1"},
		{"form", "field", "hide", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--hidden=invalid"},
		{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--enabled=yes"},
	} {
		caller := &aitableTestCaller{}
		if err := runAitableCoverageCommand(t, caller, args...); err == nil || len(caller.calls) != 0 {
			t.Fatalf("invalid boolean must fail before MCP call: err=%v calls=%#v", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableChartConfigStaysTyped(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{
		`{"status":"success","data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`,
		`{"status":"success","data":{"chartId":"chart"}}`,
	}}
	config := `{"name":"趋势","chartType":"LINE","sheet":"tbl","smooth":true,"digits":2}`
	layout := `{"x":0,"y":0,"w":12,"h":4}`
	if err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d", "--config="+config, "--layout="+layout); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "get_dashboard" || caller.calls[1].tool != "create_chart" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	got := caller.calls[1].args["config"].(map[string]any)
	if _, ok := got["smooth"].(bool); !ok {
		t.Fatalf("smooth type = %T", got["smooth"])
	}
	if _, ok := got["digits"].(float64); !ok {
		t.Fatalf("digits type = %T", got["digits"])
	}

	for _, args := range [][]string{
		{"chart", "create", "--base-id=b", "--dashboard-id=d", "--config={}", "--layout=" + layout},
		{"chart", "create", "--base-id=b", "--dashboard-id=d", "--config=[]", "--layout=" + layout},
		{"chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--config=" + config, "--layout=[]"},
	} {
		invalidCaller := &aitableTestCaller{}
		err := runAitableCoverageCommand(t, invalidCaller, args...)
		var structured *apperrors.Error
		if err == nil || !errors.As(err, &structured) || structured.Category != apperrors.CategoryValidation || len(invalidCaller.calls) != 0 {
			t.Fatalf("invalid chart JSON must fail before MCP call: err=%v calls=%#v", err, invalidCaller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableChartLayoutPreflight(t *testing.T) {
	config := `{"name":"趋势","chartType":"LINE","sheet":"tbl"}`
	dashboard := func(version string, verified bool) string {
		return fmt.Sprintf(`{"status":"success","data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":%s,"schemaVersionTypeVerified":%t}}}`, version, verified)
	}

	t.Run("numeric version two accepts 48 columns", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{dashboard("2", true), `{"status":"success"}`}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":48,"h":12}`)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 || caller.calls[0].tool != "get_dashboard" || caller.calls[1].tool != "create_chart" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("string version two stays on 12 columns", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{dashboard(`"2"`, true)}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":13,"h":4}`)
		if err == nil || !strings.Contains(err.Error(), "totalColumns=12") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
			t.Fatalf("write must not run: %#v", caller.calls)
		}
	})

	t.Run("unverified metadata fails closed", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{dashboard("2", false)}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":12,"h":4}`)
		if err == nil || !strings.Contains(err.Error(), "schemaVersionTypeVerified") || len(caller.calls) != 1 {
			t.Fatalf("error = %v calls = %#v", err, caller.calls)
		}
	})

	t.Run("confirmed app mode forces 48 without persisting context", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{`{"status":"success","data":{"baseId":"b","dashboardId":"d"}}`, `{"status":"success"}`}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":48,"h":12}`, "--is-app-mode=true")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 || caller.calls[1].tool != "create_chart" {
			t.Fatalf("calls = %#v", caller.calls)
		}
		if _, exists := caller.calls[1].args["isAppMode"]; exists {
			t.Fatalf("read-only context leaked into create_chart: %#v", caller.calls[1].args)
		}
	})

	t.Run("target identity mismatch fails before write", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{
			`{"status":"success","data":{"baseId":"b","dashboardId":"other"}}`,
		}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":48,"h":12}`, "--is-app-mode=true")
		var structured *apperrors.Error
		if err == nil || !errors.As(err, &structured) || structured.Category != apperrors.CategoryAPI ||
			structured.FailureStage != "response_validation" || len(caller.calls) != 1 {
			t.Fatalf("identity mismatch must be an API response error before write: err=%v calls=%#v", err, caller.calls)
		}
	})

	t.Run("transient dashboard read retries before one write", func(t *testing.T) {
		testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
			ready := make(chan time.Time, 1)
			ready <- time.Time{}
			return ready
		})
		caller := &aitableTestCaller{
			errors:    []error{fmt.Errorf("timeout: retryable: true")},
			responses: []string{"", dashboard("2", true), `{"status":"success"}`},
		}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":48,"h":12}`)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 3 || caller.calls[0].tool != "get_dashboard" ||
			caller.calls[1].tool != "get_dashboard" || caller.calls[2].tool != "create_chart" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("dry run reads protocol but does not write", func(t *testing.T) {
		caller := &aitableTestCaller{
			dryRun:    true,
			responses: []string{dashboard("2", true)},
		}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":48,"h":12}`, "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "get_dashboard" {
			t.Fatalf("dry-run calls = %#v", caller.calls)
		}
	})

	t.Run("non-root layout stops before write", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{dashboard("2", true)}}
		err := runAitableCoverageCommand(t, caller, "chart", "create", "--base-id=b", "--dashboard-id=d",
			"--config="+config, `--layout={"x":0,"y":0,"w":12,"h":4,"parentId":"tab-1"}`)
		if err == nil || !strings.Contains(err.Error(), "独立容器坐标系") || len(caller.calls) != 1 {
			t.Fatalf("error = %v calls = %#v", err, caller.calls)
		}
	})

	t.Run("config-only update avoids dashboard read", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{`{"status":"success"}`}}
		err := runAitableCoverageCommand(t, caller, "chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--config="+config)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "update_chart" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("layout update reads before writing", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{dashboard("1", true), `{"status":"success"}`}}
		err := runAitableCoverageCommand(t, caller, "chart", "update", "--base-id=b", "--dashboard-id=d", "--chart-id=c",
			"--config="+config, `--layout={"x":6,"y":0,"w":6,"h":4}`)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 || caller.calls[0].tool != "get_dashboard" || caller.calls[1].tool != "update_chart" {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageAitableQueryValidationIsStructured(t *testing.T) {
	caller := &aitableTestCaller{}
	err := runAitableCoverageCommand(t, caller,
		"record", "query", "--base-id=b", "--table-id=t",
		`--filters={"operator":"and","operands":[{"operator":"or","operands":"bad"}]}`,
	)
	var structured *apperrors.Error
	if err == nil || !errors.As(err, &structured) || structured.Category != apperrors.CategoryValidation || len(caller.calls) != 0 {
		t.Fatalf("invalid query filters must be structured validation before MCP: err=%v calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageAitableViewCommandEdges(t *testing.T) {
	oldDeps, oldArgs, oldSleep := deps, os.Args, helperSleep
	t.Cleanup(func() { deps, os.Args, helperSleep = oldDeps, oldArgs, oldSleep })
	helperSleep = func(time.Duration) {}
	os.Args = []string{"dws", "aitable", "--yes"}

	type scenario struct {
		name     string
		viewType string
		args     []string
	}
	base := []string{"--base-id=b", "--table-id=t", "--view-id=view"}
	with := func(prefix []string, flags ...string) []string {
		out := append([]string(nil), prefix...)
		out = append(out, base...)
		return append(out, flags...)
	}
	scenarios := []scenario{
		{"get card unsupported", "Grid", with([]string{"view", "get", "card"})},
		{"get card", "Kanban", with([]string{"view", "get", "card"})},
		{"get timebar wrong type", "Grid", with([]string{"view", "get", "timebar"})},
		{"get timebar", "Gantt", with([]string{"view", "get", "timebar"})},
		{"get filter", "Grid", with([]string{"view", "get", "filter"})},
		{"update invalid config", "Grid", with([]string{"view", "update"}, `--config={"filter":1}`)},
		{"update normalized config", "Grid", with([]string{"view", "update"}, `--config={"filter":{"operator":"eq","operands":["f","v"]}}`)},
		{"card conflict", "Kanban", with([]string{"view", "update", "card"}, "--no-cover", "--cover-field-id=f")},
		{"card unsupported", "Grid", with([]string{"view", "update", "card"}, "--no-cover")},
		{"card no cover", "Kanban", with([]string{"view", "update", "card"}, "--no-cover", "--hidden-field-title", "--display-field-name")},
		{"card cover", "Gallery", with([]string{"view", "update", "card"}, "--cover-field-id=f", "--cover-mode=auto")},
		{"card invalid json", "Kanban", with([]string{"view", "update", "card"}, "--json=[]")},
		{"timebar wrong type", "Grid", with([]string{"view", "update", "timebar"}, "--start-field=f")},
		{"timebar invalid colors", "Gantt", with([]string{"view", "update", "timebar"}, "--color-configs={}")},
		{"timebar colors", "Gantt", with([]string{"view", "update", "timebar"}, "--color-configs=[]", "--official-holiday")},
		{"timebar invalid json", "Gantt", with([]string{"view", "update", "timebar"}, "--json=[]")},
		{"aggregate half pair", "Grid", with([]string{"view", "update", "aggregate"}, "--field-id=f")},
		{"aggregate typed clear", "Grid", with([]string{"view", "update", "aggregate"}, "--field-id=f", "--action=SUM", "--clear-field-id=x,y")},
		{"aggregate invalid json", "Grid", with([]string{"view", "update", "aggregate"}, "--json=[]")},
		{"width half pair", "Grid", with([]string{"view", "update", "field-widths"}, "--field-id=f")},
		{"width typed", "Grid", with([]string{"view", "update", "field-widths"}, "--field-id=f", "--width=120")},
		{"width invalid json", "Grid", with([]string{"view", "update", "field-widths"}, "--json=[]")},
		{"visible non-array", "Grid", with([]string{"view", "update", "visible-fields"}, "--json={}")},
		{"visible mixed array", "Grid", with([]string{"view", "update", "visible-fields"}, `--json=["f",1]`)},
		{"visible empty", "Grid", with([]string{"view", "update", "visible-fields"})},
		{"visible both", "Grid", with([]string{"view", "update", "visible-fields"}, "--field-ids=x", `--json=["f","g"]`)},
		{"filter missing json", "Grid", with([]string{"view", "update", "filter"})},
		{"filter invalid json", "Grid", with([]string{"view", "update", "filter"}, "--json={")},
		{"filter invalid shape", "Grid", with([]string{"view", "update", "filter"}, "--json=1")},
		{"filter valid object", "Grid", with([]string{"view", "update", "filter"}, `--json={"operator":"eq","operands":["f","v"]}`)},
		{"sort valid", "Grid", with([]string{"view", "update", "sort"}, `--json=[{"fieldId":"f","direction":"asc"}]`)},
		{"group valid", "Grid", with([]string{"view", "update", "group"}, `--json=[{"fieldId":"f","direction":"asc"}]`)},
		{"name missing base", "Grid", []string{"view", "update", "name", "--table-id=t", "--view-id=view", "--name=n"}},
		{"frozen negative", "Grid", with([]string{"view", "update", "frozen-cols"}, "--count=-1")},
		{"frozen missing base", "Grid", []string{"view", "update", "frozen-cols", "--table-id=t", "--view-id=view", "--count=1"}},
		{"row height invalid", "Grid", with([]string{"view", "update", "row-height"}, "--cell-height=0")},
		{"row height missing base", "Grid", []string{"view", "update", "row-height", "--table-id=t", "--view-id=view", "--cell-height=32"}},
		{"fill missing json", "Grid", with([]string{"view", "update", "fill-color-rule"})},
		{"fill invalid json", "Grid", with([]string{"view", "update", "fill-color-rule"}, "--json={")},
		{"fill non-array", "Grid", with([]string{"view", "update", "fill-color-rule"}, "--json={}")},
		{"fill valid", "Grid", with([]string{"view", "update", "fill-color-rule"}, "--json=[]")},
		{"fill missing base", "Grid", []string{"view", "update", "fill-color-rule", "--table-id=t", "--view-id=view", "--json=[]"}},
	}
	for _, item := range scenarios {
		t.Run(item.name, func(t *testing.T) {
			if err := runAitableCoverageCommand(t, &aitableCommandCoverageCaller{viewType: item.viewType}, item.args...); err != nil {
				t.Logf("command returned: %v", err)
			}
		})
	}

	// Exercise the shared view preflight's base-ID and transport-error exits.
	_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{}, "view", "update", "visible-fields", "--table-id=t", "--view-id=view", "--field-ids=f")
	_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{err: fmt.Errorf("transport")}, with([]string{"view", "update", "card"}, "--no-cover")...)
}

func TestCrossPlatformCoverageAitableDeleteCancellationEdges(t *testing.T) {
	oldDeps, oldArgs, oldStdin := deps, os.Args, os.Stdin
	t.Cleanup(func() { deps, os.Args, os.Stdin = oldDeps, oldArgs, oldStdin })
	os.Args = []string{"dws", "aitable"}
	input, err := os.CreateTemp(t.TempDir(), "answers")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if _, err := input.WriteString(strings.Repeat("no\n", 20)); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	os.Stdin = input

	commands := [][]string{
		{"base", "delete", "--base-id=b"},
		{"table", "delete", "--base-id=b", "--table-id=t"},
		{"field", "delete", "--base-id=b", "--table-id=t", "--field-id=f"},
		{"record", "delete", "--base-id=b", "--table-id=t", "--record-ids=r"},
		{"view", "delete", "--base-id=b", "--table-id=t", "--view-id=v"},
		{"form", "delete", "--base-id=b", "--table-id=t", "--view-id=v"},
		{"workflow", "disable", "--base-id=b", "--workflow-id=w"},
		{"dashboard", "delete", "--base-id=b", "--dashboard-id=d"},
		{"chart", "delete", "--base-id=b", "--dashboard-id=d", "--chart-id=c"},
		{"advperm", "disable", "--base-id=b"},
		{"advperm", "role-delete", "--base-id=b", "--role-id=r"},
	}
	for _, args := range commands {
		_ = runAitableCoverageCommand(t, &aitableCommandCoverageCaller{}, args...)
	}
}

func TestCrossPlatformCoverageAitableViewFilterValidationAndReadBack(t *testing.T) {
	testseam.Swap(t, &aitableViewFilterReadbackSleep, func(time.Duration) {})
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dws", "aitable"}
	filter := `[{"operator":"eq","operands":["fldA","x"]},{"operator":"any_of","operands":["fldMulti","A"]}]`
	fields := `{"data":{"fields":[{"fieldId":"fldA","type":"text"},{"fieldId":"fldB","type":"text"},{"fieldId":"fldMulti","type":"multipleSelect"},{"fieldId":"fldDate","type":"date"}]}}`
	readBack := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":[{"operator":"eq","operands":["fldA","x"]},{"operator":"any_of","operands":["fldMulti","A"]}]}]}}`

	t.Run("flat leaf filters are written then exactly verified", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{fields, `{"success":true}`, readBack}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+filter)
		if err != nil || len(caller.calls) != 3 || caller.calls[0].tool != "get_fields" || caller.calls[1].tool != "update_view" || caller.calls[2].tool != "get_views" {
			t.Fatalf("verified view filter = err:%v calls:%#v", err, caller.calls)
		}
		config, _ := caller.calls[1].args["config"].(map[string]any)
		if _, ok := config["filter"].([]any); !ok {
			t.Fatalf("update_view filter encoding = %#v", caller.calls[1].args)
		}
	})

	t.Run("explicit logical root is preserved and verified", func(t *testing.T) {
		input := `[{"operator":"or","operands":[{"operator":"eq","operands":["fldA","x"]},{"operator":"eq","operands":["fldB","y"]}]}]`
		readBack := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"or","operands":[{"operator":"eq","operands":["fldA","x"]},{"operator":"eq","operands":["fldB","y"]}]}}]}}`
		caller := &aitableTestCaller{responses: []string{fields, `{"success":true}`, readBack}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err != nil || len(caller.calls) != 3 || caller.calls[1].tool != "update_view" || caller.calls[2].tool != "get_views" {
			t.Fatalf("logical filter root = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("group open conversation id uses normalized update response for verification", func(t *testing.T) {
		groupFields := `{"data":{"fields":[{"fieldId":"fldGroup","type":"group"}]}}`
		input := `[{"operator":"eq","operands":["fldGroup",{"openConversationId":"open-cid-1"}]}]`
		updateResponse := `{"status":"success","data":{"viewId":"view","filter":{"operator":"and","operands":[{"operator":"eq","operands":["fldGroup","cid-1"]}]}}}`
		readBack := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"and","operands":[{"operator":"eq","operands":["fldGroup","cid-1"]}]}}]}}`
		caller := &aitableTestCaller{responses: []string{groupFields, updateResponse, readBack}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err != nil || len(caller.calls) != 3 {
			t.Fatalf("group filter = err:%v calls:%#v", err, caller.calls)
		}
		config := caller.calls[1].args["config"].(map[string]any)
		leaf := config["filter"].([]any)[0].(map[string]any)
		if got := leaf["operands"].([]any)[1]; !reflect.DeepEqual(got, map[string]any{"openConversationId": "open-cid-1"}) {
			t.Fatalf("DWS converted group identifier before MCP: %#v", got)
		}
	})

	t.Run("date Scheme preserves operator-specific JSON scalar types", func(t *testing.T) {
		input := `[{"operator":"and","operands":[{"operator":"date_eq","operands":["fldDate",{"type":"relative","period":"month","offset":-1}]},{"operator":"from_now","operands":["fldDate",{"type":"relative","period":"day","offset":"-30"}]},{"operator":"date_eq","operands":["fldDate",{"type":"exact","timestamp":1786896000000}]}]}]`
		readBack := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"and","operands":[{"operator":"date_eq","operands":["fldDate",{"type":"relative","period":"month","offset":-1}]},{"operator":"from_now","operands":["fldDate",{"type":"relative","period":"day","offset":"-30"}]},{"operator":"date_eq","operands":["fldDate",{"type":"exact","timestamp":1786896000000}]}]}}]}}`
		caller := &aitableTestCaller{responses: []string{fields, `{"success":true}`, readBack}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err != nil || len(caller.calls) != 3 {
			t.Fatalf("date Scheme = err:%v calls:%#v", err, caller.calls)
		}
		config := caller.calls[1].args["config"].(map[string]any)
		root := config["filter"].([]any)[0].(map[string]any)
		conditions := root["operands"].([]any)
		relative := conditions[0].(map[string]any)["operands"].([]any)[1].(map[string]any)
		fromNow := conditions[1].(map[string]any)["operands"].([]any)[1].(map[string]any)
		exact := conditions[2].(map[string]any)["operands"].([]any)[1].(map[string]any)
		if _, ok := relative["offset"].(float64); !ok {
			t.Fatalf("date_eq relative offset type = %T, want JSON number", relative["offset"])
		}
		if _, ok := fromNow["offset"].(string); !ok {
			t.Fatalf("from_now offset type = %T, want JSON string", fromNow["offset"])
		}
		if _, ok := exact["timestamp"].(float64); !ok {
			t.Fatalf("date_eq exact timestamp type = %T, want JSON number", exact["timestamp"])
		}
	})

	t.Run("unknown field is rejected before write", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{fields}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", `--json=[{"operator":"eq","operands":["missing","x"]}]`)
		if err == nil || !strings.Contains(err.Error(), "unknown fieldId") || len(caller.calls) != 1 {
			t.Fatalf("unknown filter field = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("eventual persisted and wrapper is retried then verified", func(t *testing.T) {
		input := `[{"operator":"any_of","operands":["fldMulti","A"]}]`
		stale := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"and","operands":[]}}]}}`
		terminal := `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"and","operands":[{"operator":"any_of","operands":["fldMulti","A"]}]}}]}}`
		caller := &aitableTestCaller{responses: []string{fields, `{"success":true}`, stale, terminal}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err != nil || len(caller.calls) != 4 || caller.calls[2].tool != "get_views" || caller.calls[3].tool != "get_views" {
			t.Fatalf("eventual wrapped filter = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("multi-select operator rejects text field", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{fields}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", `--json=[{"operator":"any_of","operands":["fldA",["A"]]}]`)
		if err == nil || !strings.Contains(err.Error(), "requires a multipleSelect field") || len(caller.calls) != 1 {
			t.Fatalf("wrong filter field type = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("multi-select array fails closed before write", func(t *testing.T) {
		input := `[{"operator":"any_of","operands":["fldMulti",["A","B"]]}]`
		caller := &aitableTestCaller{responses: []string{fields}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err == nil || !strings.Contains(err.Error(), "persisted view protocol") || len(caller.calls) != 1 || caller.calls[0].tool != "get_fields" {
			t.Fatalf("multi-select array fail-closed = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("multi-select invalid array value fails before write", func(t *testing.T) {
		input := `[{"operator":"any_of","operands":["fldMulti",["A",""]]}]`
		caller := &aitableTestCaller{responses: []string{fields}}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+input)
		if err == nil || !strings.Contains(err.Error(), "non-empty option-name") || len(caller.calls) != 1 {
			t.Fatalf("invalid multi-select array = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("date and system-time fields require date operators", func(t *testing.T) {
		fieldTypes := map[string]string{"date": "date", "created": "createdTime", "modified": "lastModifiedTime"}
		for fieldID := range fieldTypes {
			valid := []any{map[string]any{"operator": "date_eq", "operands": []any{fieldID, map[string]any{"type": "exact", "timestamp": int64(1786982400000)}}}}
			if err := validateAitableViewFilter(valid, fieldTypes); err != nil {
				t.Fatalf("date_eq for %s: %v", fieldID, err)
			}
			invalid := []any{map[string]any{"operator": "eq", "operands": []any{fieldID, "2026-08-18"}}}
			if err := validateAitableViewFilter(invalid, fieldTypes); err == nil || !strings.Contains(err.Error(), "invalid for") {
				t.Fatalf("eq for %s = %v, want date-operator error", fieldID, err)
			}
		}
	})

	t.Run("dry-run validates but performs no write or read-back", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{fields}, dryRun: true}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+filter, "--dry-run")
		if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_fields" {
			t.Fatalf("view filter dry-run = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("read-back mismatch is not success", func(t *testing.T) {
		responses := []string{fields, `{"success":true}`}
		for range aitableViewFilterReadbackAttempts {
			responses = append(responses, `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":[]}]}}`)
		}
		caller := &aitableTestCaller{responses: responses}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+filter)
		if err == nil || !strings.Contains(err.Error(), "read-back mismatch") || len(caller.calls) != 2+aitableViewFilterReadbackAttempts {
			t.Fatalf("mismatched filter readback = err:%v calls:%#v", err, caller.calls)
		}
		requireViewFilterVerificationUnknown(t, err)
	})

	t.Run("person internal key is reported as executed verification unknown", func(t *testing.T) {
		personFields := `{"data":{"fields":[{"fieldId":"fldOwner","type":"user"}]}}`
		personFilter := `[{"operator":"eq","operands":["fldOwner",{"userId":"staff1","corpId":"ding1"}]}]`
		responses := []string{personFields, `{"success":true}`}
		for range aitableViewFilterReadbackAttempts {
			responses = append(responses, `{"data":{"views":[{"viewId":"view","viewType":"Grid","filter":{"operator":"and","operands":[{"operator":"eq","operands":["fldOwner","12345"]}]}}]}}`)
		}
		caller := &aitableTestCaller{responses: responses}
		err := runAitableCoverageCommand(t, caller, "view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json="+personFilter)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "view_filter_verification_unknown" {
			t.Fatalf("person readback error = %#v", err)
		}
		if typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || len(caller.calls) != 2+aitableViewFilterReadbackAttempts {
			t.Fatalf("person readback metadata = %#v, calls=%d", typed, len(caller.calls))
		}
		if typed.Details["status"] != "unknown" || typed.Details["executed"] != true || typed.Details["verified"] != false {
			t.Fatalf("person readback details = %#v", typed.Details)
		}
		if strings.Contains(fmt.Sprintf("%#v", typed.Details), "12345") {
			t.Fatalf("internal person key leaked in error details: %#v", typed.Details)
		}
	})
}

func TestCrossPlatformCoverageAitableViewFilterFailureAndShapeEdges(t *testing.T) {
	testseam.Swap(t, &aitableViewFilterReadbackSleep, func(time.Duration) {})
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "aitable"}
	fields := `{"data":{"fields":[{"fieldId":"fldA","type":"text"}]}}`
	filter := `[{"operator":"eq","operands":["fldA","x"]}]`
	args := []string{"view", "update", "filter", "--base-id=b", "--table-id=t", "--view-id=view", "--json=" + filter}

	t.Run("update transport error", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{fields}, errors: []error{nil, context.Canceled}}
		if err := runAitableCoverageCommand(t, caller, args...); err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) || len(caller.calls) != 2 {
			t.Fatalf("update error = %v, calls=%#v", err, caller.calls)
		}
	})

	t.Run("readback transport errors exhaust", func(t *testing.T) {
		errs := []error{nil, nil}
		for range aitableViewFilterReadbackAttempts {
			errs = append(errs, context.DeadlineExceeded)
		}
		caller := &aitableTestCaller{responses: []string{fields, `{"success":true}`}, errors: errs}
		err := runAitableCoverageCommand(t, caller, args...)
		if err == nil || len(caller.calls) != 2+aitableViewFilterReadbackAttempts {
			t.Fatalf("readback errors = %v, calls=%d", err, len(caller.calls))
		}
		requireViewFilterVerificationUnknown(t, err)
	})

	t.Run("readback wrong identity exhausts", func(t *testing.T) {
		responses := []string{fields, `{"success":true}`}
		for range aitableViewFilterReadbackAttempts {
			responses = append(responses, `{"data":{"views":[{"viewId":"other","filter":[]}]}}`)
		}
		caller := &aitableTestCaller{responses: responses}
		err := runAitableCoverageCommand(t, caller, args...)
		if err == nil || !strings.Contains(err.Error(), "returned viewId") {
			t.Fatalf("wrong readback identity = %v", err)
		}
		requireViewFilterVerificationUnknown(t, err)
	})

	loadCases := []struct {
		name      string
		response  string
		callErr   error
		wantError string
	}{
		{name: "transport", callErr: context.Canceled, wantError: "context canceled"},
		{name: "invalid json", response: `{`, wantError: "not valid JSON"},
		{name: "missing collection", response: `{}`, wantError: "missing the fields collection"},
		{name: "missing identity", response: `{"fields":[{"type":"text"}]}`, wantError: "missing fieldId or type"},
	}
	for _, tc := range loadCases {
		t.Run("load fields "+tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{responses: []string{tc.response}, errors: []error{tc.callErr}}
			installAitableDeps(t, caller)
			if _, err := loadAitableFieldTypes(context.Background(), "b", "t"); err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("load fields error = %v, want %q", err, tc.wantError)
			}
		})
	}
	t.Run("load legacy field keys", func(t *testing.T) {
		caller := &aitableTestCaller{responses: []string{`{"fieldList":[{"id":"legacy","fieldType":"text"}]}`}}
		installAitableDeps(t, caller)
		got, err := loadAitableFieldTypes(context.Background(), "b", "t")
		if err != nil || got["legacy"] != "text" {
			t.Fatalf("legacy field keys = %#v, %v", got, err)
		}
	})

	if _, ok := findAitableObjectList(map[string]any{"fields": "bad"}, "fields"); ok {
		t.Fatal("scalar fields collection must fail")
	}
	if _, ok := findAitableObjectList(map[string]any{"fields": []any{"bad"}}, "fields"); ok {
		t.Fatal("scalar field item must fail")
	}
	if got, ok := findAitableObjectList([]any{"skip", map[string]any{"nested": map[string]any{"fields": []any{map[string]any{"fieldId": "f"}}}}}, "fields"); !ok || len(got) != 1 {
		t.Fatalf("recursive fields = %#v, %v", got, ok)
	}
	if got, ok := findAitableObjectList(map[string]any{
		"fieldList": []any{map[string]any{"fieldId": "legacy"}},
		"fields":    []any{map[string]any{"fieldId": "canonical"}},
	}, "fields", "fieldList"); !ok || len(got) != 1 || got[0]["fieldId"] != "canonical" {
		t.Fatalf("declared collection priority = %#v, %v", got, ok)
	}

	invalidFilters := []struct {
		filter []any
		want   string
	}{
		{filter: []any{"bad"}, want: "must be an object"},
		{filter: []any{map[string]any{"operator": "bogus", "operands": []any{}}}, want: "unsupported operator"},
		{filter: []any{map[string]any{"operator": "eq", "operands": "bad"}}, want: "requires an operands array"},
		{filter: []any{map[string]any{"operator": "and", "operands": []any{}}}, want: "at least one condition"},
		{filter: []any{map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "or", "operands": []any{map[string]any{"operator": "eq", "operands": []any{"f", "x"}}}}}}}, want: "nested boolean"},
		{filter: []any{map[string]any{"operator": "or", "operands": []any{map[string]any{"operator": "eq", "operands": []any{"f", "x"}}}}, map[string]any{"operator": "eq", "operands": []any{"f", "y"}}}, want: "single top-level"},
		{filter: []any{map[string]any{"operator": "exist", "operands": []any{"f", "extra"}}}, want: "requires 1 operands"},
		{filter: []any{map[string]any{"operator": "eq", "operands": []any{1, "x"}}}, want: "requires a fieldId"},
		{filter: []any{map[string]any{"operator": "any_of", "operands": []any{"multi", 1}}}, want: "one option-name string"},
		{filter: []any{map[string]any{"operator": "date_eq", "operands": []any{"date", "2026-08-18"}}}, want: "structured date Scheme"},
		{filter: []any{map[string]any{"operator": "date_eq", "operands": []any{"date", map[string]any{"type": "relative", "period": "month", "offset": "-1"}}}}, want: "JSON integer number"},
		{filter: []any{map[string]any{"operator": "date_eq", "operands": []any{"date", map[string]any{"type": "exact", "timestamp": "1786982400000"}}}}, want: "Unix-millisecond JSON integer"},
		{filter: []any{map[string]any{"operator": "from_now", "operands": []any{"date", map[string]any{"type": "relative", "period": "day", "offset": -30}}}}, want: "JSON string"},
		{filter: []any{map[string]any{"operator": "from_now", "operands": []any{"date", map[string]any{"type": "relative", "period": "month", "offset": "-1"}}}}, want: "period must be day"},
		{filter: []any{map[string]any{"operator": "date_eq", "operands": []any{"f", map[string]any{"type": "relative", "period": "day", "offset": 0}}}}, want: "requires a date"},
	}
	for _, tc := range invalidFilters {
		if err := validateAitableViewFilter(tc.filter, map[string]string{"f": "text", "multi": "multipleSelect", "date": "date"}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("validate filter %#v = %v, want %q", tc.filter, err, tc.want)
		}
	}
	if err := validateAitableMultiSelectOptionNames(nil); err == nil {
		t.Fatal("empty any_of array must fail")
	}
	if err := validateAitableMultiSelectOptionNames([]any{"ok", 1}); err == nil {
		t.Fatal("non-string any_of option must fail")
	}
	if err := validateAitableMultiSelectOptionNames([]any{"first", " second "}); err != nil {
		t.Fatalf("valid any_of option names = %v", err)
	}
	if got := compactJSON(make(chan int)); !strings.HasPrefix(got, "(chan int)") {
		t.Fatalf("compactJSON fallback = %q", got)
	}
	if persistedViewFilterMatches("bad", nil) || persistedViewFilterMatches(map[string]any{"operator": "or"}, nil) {
		t.Fatal("invalid persisted wrapper must not match")
	}
	explicit := []any{map[string]any{"operator": "or", "operands": []any{map[string]any{"operator": "eq", "operands": []any{"f", "x"}}}}}
	if !persistedViewFilterMatches(explicit[0], explicit) {
		t.Fatal("explicit persisted logical root should match its singleton-array request")
	}
}

func requireViewFilterVerificationUnknown(t *testing.T, err error) {
	t.Helper()
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "view_filter_verification_unknown" || typed.Retryable {
		t.Fatalf("view filter verification error = %#v", err)
	}
	if typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Details["status"] != "unknown" || typed.Details["verified"] != false {
		t.Fatalf("view filter verification metadata = %#v", typed)
	}
}

func TestCrossPlatformCoverageAitableSnapshotThinCommands(t *testing.T) {
	testseam.Swap(t, &deps, nil)
	const token = "123e4567-e89b-42d3-a456-426614174000"
	tests := []struct {
		name  string
		tool  string
		args  []string
		check func(*testing.T, map[string]any)
	}{
		{
			name: "run ai field", tool: "run_ai_field",
			args: []string{"field", "run-ai", "--base-id=b", "--table-id=t", "--field-ids=f1,f2", "--record-ids=r1,r2"},
			check: func(t *testing.T, args map[string]any) {
				if got := args["fieldIds"].([]string); len(got) != 2 || got[1] != "f2" {
					t.Fatalf("fieldIds = %#v", got)
				}
			},
		},
		{
			name: "query record ids", tool: "query_record_ids",
			args: []string{"record", "ids", "--base-id=b", "--table-id=t", "--limit=25", "--cursor=next"},
			check: func(t *testing.T, args map[string]any) {
				if args["limit"] != 25 || args["cursor"] != "next" {
					t.Fatalf("paging args = %#v", args)
				}
			},
		},
		{
			name: "create sub records", tool: "create_sub_records",
			args: []string{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=parent", `--records=[{"cells":{"f":"v"}}]`, "--view-id=v", "--client-token=" + token},
			check: func(t *testing.T, args map[string]any) {
				if args["parentRecordId"] != "parent" || args["viewId"] != "v" || args["clientToken"] != token {
					t.Fatalf("create-sub args = %#v", args)
				}
			},
		},
		{
			name: "submit form", tool: "submit_form",
			args: []string{"form", "submit", "--base-id=b", "--table-id=t", "--view-id=v", `--value={"f":"v"}`},
			check: func(t *testing.T, args map[string]any) {
				if value, ok := args["value"].(string); !ok || value != `{"f":"v"}` {
					t.Fatalf("form value = %#v", args["value"])
				}
			},
		},
		{
			name: "remove attachments", tool: "remove_attachments",
			args: []string{"attachment", "remove", "--base-id=b", "--table-id=t", "--record-id=r", "--field-id=f", "--resource-ids=res1,res2", "--yes"},
			check: func(t *testing.T, args map[string]any) {
				if _, exists := args["confirm"]; exists {
					t.Fatalf("remove_attachments snapshot has no confirm property: %#v", args)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, tc.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v", caller.calls)
			}
			call := caller.calls[0]
			if call.server == "aitable-helper" || call.tool != tc.tool {
				t.Fatalf("call = %#v, want public aitable/%s", call, tc.tool)
			}
			tc.check(t, call.args)
		})
	}
}

func TestCrossPlatformCoverageAitableSnapshotThinCommandsFailClosed(t *testing.T) {
	testseam.Swap(t, &deps, nil)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "invalid create token", args: []string{"record", "create", "--base-id=b", "--table-id=t", `--records=[{"cells":{}}]`, "--client-token=not-a-uuid"}},
		{name: "table field name exceeds utf16 limit", args: []string{"table", "create", "--base-id=b", "--name=t", `--fields=[{"fieldName":"` + strings.Repeat("😀", 76) + `","type":"text"}]`}},
		{name: "field update rejects blank name", args: []string{"field", "update", "--base-id=b", "--table-id=t", "--field-id=f", "--name= "}},
		{name: "invalid sub record cells", args: []string{"record", "create-sub", "--base-id=b", "--table-id=t", "--parent-record-id=p", `--records=[{}]`}},
		{name: "attachment scope required", args: []string{"attachment", "remove", "--base-id=b", "--table-id=t", "--record-id=r", "--yes"}},
		{name: "form value object required", args: []string{"form", "submit", "--base-id=b", "--table-id=t", "--view-id=v", `--value=[]`}},
		{name: "record ids max limit", args: []string{"record", "ids", "--base-id=b", "--table-id=t", "--limit=101"}},
		{name: "nested view filter", args: []string{"view", "create", "--base-id=b", "--table-id=t", "--view-type=Grid", `--config={"filter":{"operator":"and","operands":[{"operator":"or","operands":[{"operator":"eq","operands":["f","x"]}]}]}}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, tc.args...); err == nil {
				t.Fatal("expected validation error")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableAtomicDeletesPassServiceConfirmation(t *testing.T) {
	testseam.Swap(t, &deps, nil)
	tests := []struct {
		tool string
		args []string
	}{
		{tool: "delete_base", args: []string{"base", "delete", "--base-id=b", "--yes"}},
		{tool: "delete_table", args: []string{"table", "delete", "--base-id=b", "--table-id=t", "--yes"}},
		{tool: "delete_field", args: []string{"field", "delete", "--base-id=b", "--table-id=t", "--field-id=f", "--yes"}},
		{tool: "delete_records", args: []string{"record", "delete", "--base-id=b", "--table-id=t", "--record-ids=r", "--yes"}},
		{tool: "delete_view", args: []string{"view", "delete", "--base-id=b", "--table-id=t", "--view-id=v", "--yes"}},
		{tool: "delete_dashboard", args: []string{"dashboard", "delete", "--base-id=b", "--dashboard-id=d", "--yes"}},
		{tool: "delete_chart", args: []string{"chart", "delete", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--yes"}},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, tc.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool {
				t.Fatalf("calls = %#v", caller.calls)
			}
			if confirm, ok := caller.calls[0].args["confirm"].(bool); !ok || !confirm {
				t.Fatalf("%s confirm = %#v", tc.tool, caller.calls[0].args["confirm"])
			}
		})
	}
}

func TestCrossPlatformCoverageAitableHistoricalHelperRouting(t *testing.T) {
	caller := &aitableTestCaller{}
	installAitableDeps(t, caller)
	if err := callAitableHelperTool("list_form_views", map[string]any{"baseId": "b", "tableId": "t"}); err != nil {
		t.Fatal(err)
	}
	if err := callAitableHelperTool("get_cell_doc", map[string]any{"baseId": "b", "tableId": "t", "recordId": "r"}); err != nil {
		t.Fatal(err)
	}
	if err := callAitableHelperTool("create_cell_doc", map[string]any{"baseId": "b", "tableId": "t", "fieldId": "f", "recordId": "r"}); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 || caller.calls[0].server != "aitable" || caller.calls[1].server != "aitable" || caller.calls[2].server != "aitable" {
		t.Fatalf("routing calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAitableStrictJSONFailsBeforeMCP(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "record sort", args: []string{"record", "query", "--base-id=b", "--table-id=t", "--sort={"}},
		{name: "record sort null", args: []string{"record", "query", "--base-id=b", "--table-id=t", "--sort=null"}},
		{name: "field config", args: []string{"field", "update", "--base-id=b", "--table-id=t", "--field-id=f", "--config=[]"}},
		{name: "field ai config", args: []string{"field", "update", "--base-id=b", "--table-id=t", "--field-id=f", "--ai-config={"}},
		{name: "view description", args: []string{"view", "create", "--base-id=b", "--table-id=t", "--view-type=Grid", "--desc=[]"}},
		{name: "dashboard config", args: []string{"dashboard", "update", "--base-id=b", "--dashboard-id=d", "--config=[]"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, tc.args...); err == nil {
				t.Fatal("invalid JSON unexpectedly succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid JSON made MCP calls: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitablePublicFormAndViewTools(t *testing.T) {
	t.Run("create form uses dedicated public tool", func(t *testing.T) {
		caller := &aitableTestCaller{}
		err := runAitableCoverageCommand(t, caller,
			"form", "create", "--base-id=b", "--table-id=t", "--name=问卷",
			"--description=说明", "--allow-many-times=false", "--submit-message=已提交")
		if err != nil {
			t.Fatalf("form create: %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "create_form_view" {
			t.Fatalf("form create calls = %#v", caller.calls)
		}
		args := caller.calls[0].args
		if args["name"] != "问卷" || args["description"] != "说明" || args["allowManyTimes"] != false || args["submitMessage"] != "已提交" {
			t.Fatalf("form create args = %#v", args)
		}
		if _, exists := args["viewType"]; exists {
			t.Fatalf("form create leaked create_view args = %#v", args)
		}
	})

	t.Run("hidden fields uses public projection", func(t *testing.T) {
		caller := &aitableTestCaller{}
		if err := runAitableCoverageCommand(t, caller, "view", "get", "hidden-fields", "--base-id=b", "--table-id=t", "--view-id=v"); err != nil {
			t.Fatalf("hidden fields: %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "get_hidden_fields_of_view" || caller.calls[0].args["viewId"] != "v" {
			t.Fatalf("hidden fields calls = %#v", caller.calls)
		}
	})

	t.Run("primary doc create uses public tool and optional parameters", func(t *testing.T) {
		caller := &aitableTestCaller{}
		if err := runAitableCoverageCommand(t, caller, "record", "primary-doc-create", "--base-id=b", "--table-id=t", "--field-id=f", "--record-id=r", "--doc-name=文档", "--template-doc-id=tpl"); err != nil {
			t.Fatalf("primary doc create: %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "create_cell_doc" || caller.calls[0].args["docName"] != "文档" || caller.calls[0].args["templateDocId"] != "tpl" {
			t.Fatalf("primary doc create calls = %#v", caller.calls)
		}
	})

	t.Run("notification channels may use downstream defaults", func(t *testing.T) {
		caller := &aitableTestCaller{}
		if err := runAitableCoverageCommand(t, caller, "form", "share", "notify", "--base-id=b", "--table-id=t", "--view-id=v", "--recipients=u", "--yes"); err != nil {
			t.Fatalf("form notify with default channels: %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "notify_share_form_recipients" {
			t.Fatalf("form notify calls = %#v", caller.calls)
		}
		for _, key := range []string{"enableSendChat", "enableSendCardByDingDoc", "enableSendTodoTask", "enableSendWorkNotice"} {
			if _, exists := caller.calls[0].args[key]; exists {
				t.Fatalf("default channel %s should be omitted: %#v", key, caller.calls[0].args)
			}
		}
	})
}

func TestCrossPlatformCoverageAitableDestructiveAndNotificationGates(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args []string
	}{
		{name: "section delete", tool: "delete_section", args: []string{"section", "delete", "--base-id=b", "--section-id=s"}},
		{name: "form question delete", tool: "delete_field", args: []string{"form", "questions", "delete", "--base-id=b", "--table-id=t", "--field-id=f"}},
		{name: "form notification", tool: "notify_share_form_recipients", args: []string{"form", "share", "notify", "--base-id=b", "--table-id=t", "--view-id=v", "--recipients=u1,u1,u2", "--send-chat=true"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withoutYes := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, withoutYes, tc.args...); err == nil {
				t.Fatal("command without --yes unexpectedly succeeded")
			}
			if len(withoutYes.calls) != 0 {
				t.Fatalf("command without --yes made calls: %#v", withoutYes.calls)
			}

			withYes := &aitableTestCaller{}
			args := append(append([]string(nil), tc.args...), "--yes")
			if err := runAitableCoverageCommand(t, withYes, args...); err != nil {
				t.Fatalf("command with --yes: %v", err)
			}
			if len(withYes.calls) != 1 || withYes.calls[0].tool != tc.tool {
				t.Fatalf("command calls = %#v", withYes.calls)
			}
			if tc.tool == "notify_share_form_recipients" {
				recipients, ok := withYes.calls[0].args["recipients"].([]string)
				if !ok || len(recipients) != 2 || recipients[0] != "u1" || recipients[1] != "u2" {
					t.Fatalf("deduplicated recipients = %#v", withYes.calls[0].args["recipients"])
				}
			}
		})
	}
}
