package helpers

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableCommandCoverageCaller struct {
	viewType string
	err      error
	response map[string]string
}

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

	retryable := fmt.Errorf("timeout: retryable: true")
	caller := &aitableTestCaller{errors: []error{retryable, retryable, retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableTool("retry", nil); err == nil {
		t.Fatal("exhausted aitable retries returned nil")
	}

	caller = &aitableTestCaller{errors: []error{retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableHelperTool("retry", nil); err != nil {
		t.Fatalf("helper retry did not recover: %v", err)
	}

	caller = &aitableTestCaller{errors: []error{retryable, retryable, retryable, retryable}}
	installAitableDeps(t, caller)
	if err := callAitableHelperTool("retry", nil); err == nil {
		t.Fatal("exhausted helper retries returned nil")
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
		{"update normalized config", "Grid", with([]string{"view", "update"}, `--config={"filter":{"operator":"and","operands":[]}}`)},
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
		{"filter valid object", "Grid", with([]string{"view", "update", "filter"}, `--json={"operator":"and","operands":[]}`)},
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
