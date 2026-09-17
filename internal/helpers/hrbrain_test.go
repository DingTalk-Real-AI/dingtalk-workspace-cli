package helpers

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// executeHrbrainWriteCommand runs a write command tree that requires
// confirmation. It registers the root-level persistent --yes / --dry-run flags
// (injected by the app root in production) and feeds a scripted stdin so the
// deferred ConfirmSafety gate behaves deterministically (empty input → EOF →
// confirmation_required unless --yes is passed).
func executeHrbrainWriteCommand(t *testing.T, root *cobra.Command, input string, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"dws", "hrbrain"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	root.PersistentFlags().Bool("dry-run", false, "preview only")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(strings.NewReader(input))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{}, args...))
	return root.Execute()
}

// executeHrbrainCommand runs the given hrbrain command tree with args, discarding
// output, and returns the error (if any) from RunE.
func executeHrbrainCommand(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"dws", "hrbrain"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	// SetArgs must receive a non-nil slice: when args is nil (zero variadic
	// call), cobra falls back to os.Args[1:] internally, which would wrongly
	// feed the literal "hrbrain" token back in as a bogus positional arg.
	root.SetArgs(append([]string{}, args...))
	return root.Execute()
}

func TestHrbrainTalentPoolList(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "list",
	); err != nil {
		t.Fatalf("list without optional flags: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "list",
		"--page", "1", "--page-size", "20",
		"--keyword", "储备干部", "--pool-type", "TYPE", "--creator", "USER_ID",
		"--labels", "a,b,c",
	); err != nil {
		t.Fatalf("list with optional flags: %v", err)
	}
}

func TestHrbrainTalentPoolDetail(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "detail", "--pool-code", "POOL_CODE",
	); err != nil {
		t.Fatalf("detail with pool-code: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "detail",
	); err == nil {
		t.Fatal("detail without pool-code should error")
	}
}

func TestHrbrainTalentPoolEmployees(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "employees",
		"--pool-code", "POOL_CODE", "--page", "1", "--page-size", "20",
	); err != nil {
		t.Fatalf("employees with pool-code: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "employees",
	); err == nil {
		t.Fatal("employees without pool-code should error")
	}
}

func TestHrbrainTalentPoolSave(t *testing.T) {
	// Missing required --pool-name errors before any MCP call.
	missingNameCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, missingNameCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "save",
	); err == nil {
		t.Fatal("save without pool-name should error")
	}
	if missingNameCaller.calls != 0 {
		t.Fatalf("save without pool-name must not call MCP, got %d call(s)", missingNameCaller.calls)
	}

	// Invalid --rule-json (not valid JSON) is rejected before dispatch.
	invalidRuleCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, invalidRuleCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "save", "--pool-name", "储备干部池", "--rule-json", "not-json",
	); err == nil {
		t.Fatal("save with invalid rule-json should error")
	}
	if invalidRuleCaller.calls != 0 {
		t.Fatalf("save with invalid rule-json must not call MCP, got %d call(s)", invalidRuleCaller.calls)
	}

	// A bare JSON scalar is valid JSON but not an object, so --rule-json must
	// still reject it (json.Valid cannot distinguish object from non-object).
	nonObjectRuleCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, nonObjectRuleCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "save", "--pool-name", "储备干部池", "--rule-json", "111",
	); err == nil {
		t.Fatal("save with non-object rule-json should error")
	}
	if nonObjectRuleCaller.calls != 0 {
		t.Fatalf("save with non-object rule-json must not call MCP, got %d call(s)", nonObjectRuleCaller.calls)
	}

	// Invalid --pool-tags JSON is rejected before dispatch.
	invalidTagsCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, invalidTagsCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "save", "--pool-name", "储备干部池", "--pool-tags", "not-json",
	); err == nil {
		t.Fatal("save with invalid pool-tags should error")
	}
	if invalidTagsCaller.calls != 0 {
		t.Fatalf("save with invalid pool-tags must not call MCP, got %d call(s)", invalidTagsCaller.calls)
	}

	// An empty --pool-tags array is syntactically valid but carries no tag, so
	// it must be rejected before dispatch.
	emptyTagsCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, emptyTagsCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "save", "--pool-name", "储备干部池", "--pool-tags", "[]",
	); err == nil {
		t.Fatal("save with empty pool-tags array should error")
	}
	if emptyTagsCaller.calls != 0 {
		t.Fatalf("save with empty pool-tags must not call MCP, got %d call(s)", emptyTagsCaller.calls)
	}

	// user_required write without --yes must fail closed with
	// confirmation_required and never dispatch the MCP call.
	confirmCaller := &scriptedToolCaller{}
	installScriptedCaller(t, confirmCaller)
	if err := executeHrbrainWriteCommand(t, newHrbrainCommand(), "",
		"talent-pool", "save", "--pool-name", "储备干部池",
	); err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("save without --yes error = %v, want confirmation_required", err)
	}
	if confirmCaller.calls != 0 {
		t.Fatalf("save without --yes must not call MCP, got %d call(s)", confirmCaller.calls)
	}

	// Happy path with --yes dispatches create_or_update_pool carrying every
	// provided field (rule-json passed through as the raw string, pool-tags as
	// a parsed array).
	okCaller := &scriptedToolCaller{}
	installScriptedCaller(t, okCaller)
	if err := executeHrbrainWriteCommand(t, newHrbrainCommand(), "",
		"talent-pool", "save",
		"--pool-code", "POOL_CODE",
		"--pool-name", "储备干部池",
		"--pool-desc", "描述",
		"--rule-json", `{"auto":true}`,
		"--pool-tags", `[{"label":"共享人才池","setting":{"color":"#fff","backgroundColor":"#000"}}]`,
		"--yes",
	); err != nil {
		t.Fatalf("save happy path: %v", err)
	}
	if okCaller.calls != 1 {
		t.Fatalf("save happy path calls = %d, want 1", okCaller.calls)
	}
	if okCaller.tool != "create_or_update_pool" {
		t.Fatalf("save happy path tool = %q, want create_or_update_pool", okCaller.tool)
	}
	if okCaller.args["poolName"] != "储备干部池" || okCaller.args["poolCode"] != "POOL_CODE" ||
		okCaller.args["poolDesc"] != "描述" || okCaller.args["ruleJson"] != `{"auto":true}` {
		t.Fatalf("save happy path args = %#v", okCaller.args)
	}
	if tags, ok := okCaller.args["poolTags"].([]any); !ok || len(tags) != 1 {
		t.Fatalf("save happy path poolTags = %#v, want 1-element array", okCaller.args["poolTags"])
	}
}

func TestHrbrainTalentPoolMoveMembers(t *testing.T) {
	// Each required flag missing errors before any MCP call.
	missingCases := []struct {
		name string
		args []string
	}{
		{"missing pool-code", []string{"talent-pool", "move-members", "--opt-type", "ENTERING", "--staff-ids", "W1"}},
		{"missing opt-type", []string{"talent-pool", "move-members", "--pool-code", "P", "--staff-ids", "W1"}},
		{"missing staff-ids", []string{"talent-pool", "move-members", "--pool-code", "P", "--opt-type", "ENTERING"}},
	}
	for _, tc := range missingCases {
		caller := &scriptedToolCaller{dry: true}
		installScriptedCaller(t, caller)
		if err := executeHrbrainCommand(t, newHrbrainCommand(), tc.args...); err == nil {
			t.Fatalf("move-members %s should error", tc.name)
		}
		if caller.calls != 0 {
			t.Fatalf("move-members %s must not call MCP, got %d call(s)", tc.name, caller.calls)
		}
	}

	// An unsupported --opt-type value is rejected before dispatch.
	invalidOptCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, invalidOptCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "move-members", "--pool-code", "P", "--opt-type", "MOVE", "--staff-ids", "W1",
	); err == nil {
		t.Fatal("move-members with invalid opt-type should error")
	}
	if invalidOptCaller.calls != 0 {
		t.Fatalf("move-members invalid opt-type must not call MCP, got %d call(s)", invalidOptCaller.calls)
	}

	// A --staff-ids value that resolves to no work numbers is rejected.
	emptyStaffCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, emptyStaffCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"talent-pool", "move-members", "--pool-code", "P", "--opt-type", "ENTERING", "--staff-ids", " , ",
	); err == nil {
		t.Fatal("move-members with blank staff-ids should error")
	}
	if emptyStaffCaller.calls != 0 {
		t.Fatalf("move-members blank staff-ids must not call MCP, got %d call(s)", emptyStaffCaller.calls)
	}

	// user_required write without --yes fails closed and never dispatches.
	confirmCaller := &scriptedToolCaller{}
	installScriptedCaller(t, confirmCaller)
	if err := executeHrbrainWriteCommand(t, newHrbrainCommand(), "",
		"talent-pool", "move-members", "--pool-code", "P", "--opt-type", "ENTERING", "--staff-ids", "W1",
	); err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("move-members without --yes error = %v, want confirmation_required", err)
	}
	if confirmCaller.calls != 0 {
		t.Fatalf("move-members without --yes must not call MCP, got %d call(s)", confirmCaller.calls)
	}

	// Happy path with --yes dispatches entering_or_leaving_pool with the parsed
	// staff list and optional remark.
	okCaller := &scriptedToolCaller{}
	installScriptedCaller(t, okCaller)
	if err := executeHrbrainWriteCommand(t, newHrbrainCommand(), "",
		"talent-pool", "move-members",
		"--pool-code", "POOL_CODE",
		"--opt-type", "LEAVING",
		"--staff-ids", "W1,W2",
		"--remark", "转岗",
		"--yes",
	); err != nil {
		t.Fatalf("move-members happy path: %v", err)
	}
	if okCaller.calls != 1 || okCaller.tool != "entering_or_leaving_pool" {
		t.Fatalf("move-members happy path calls=%d tool=%q", okCaller.calls, okCaller.tool)
	}
	if okCaller.args["poolCode"] != "POOL_CODE" || okCaller.args["optType"] != "LEAVING" ||
		okCaller.args["remark"] != "转岗" {
		t.Fatalf("move-members happy path args = %#v", okCaller.args)
	}
	if staff, ok := okCaller.args["staffIds"].([]string); !ok || !reflect.DeepEqual(staff, []string{"W1", "W2"}) {
		t.Fatalf("move-members happy path staffIds = %#v, want [W1 W2]", okCaller.args["staffIds"])
	}
}

func TestHrbrainProfileMetadata(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "metadata", "--work-no", "WORK_NO",
	); err != nil {
		t.Fatalf("metadata with work-no: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "metadata",
	); err == nil {
		t.Fatal("metadata without work-no should error")
	}
}

func TestHrbrainProfileQuery(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "query",
		"--work-no", "WORK_NO",
		"--data-queries", `[{"modelCode":"basic","fields":["name","dept"]}]`,
	); err != nil {
		t.Fatalf("query with valid data-queries: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "query",
		"--work-no", "WORK_NO",
		"--data-queries", `not-json`,
	); err == nil {
		t.Fatal("query with invalid data-queries JSON should error")
	}

	// An empty JSON array is syntactically valid but queries nothing, so it
	// must be rejected before dispatch.
	emptyDataQueriesCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, emptyDataQueriesCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "query",
		"--work-no", "WORK_NO",
		"--data-queries", `[]`,
	); err == nil {
		t.Fatal("query with empty data-queries array should error")
	}
	if emptyDataQueriesCaller.calls != 0 {
		t.Fatalf("query with empty data-queries must not call MCP, got %d call(s)", emptyDataQueriesCaller.calls)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "query", "--work-no", "WORK_NO",
	); err == nil {
		t.Fatal("query without data-queries should error")
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "query", "--data-queries", `[]`,
	); err == nil {
		t.Fatal("query without work-no should error")
	}
}

func TestHrbrainProfileLabels(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "labels", "--staff-ids", "WORK_NO1,WORK_NO2", "--all-label",
	); err != nil {
		t.Fatalf("labels with staff-ids and all-label: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "labels", "--staff-ids", "WORK_NO1",
	); err != nil {
		t.Fatalf("labels without all-label: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "labels",
	); err == nil {
		t.Fatal("labels without staff-ids should error")
	}
}

func TestHrbrainProfileCareer(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "career", "--work-no", "WORK_NO",
	); err != nil {
		t.Fatalf("career with work-no: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "career",
	); err == nil {
		t.Fatal("career without work-no should error")
	}
}

func TestHrbrainProfilePerformance(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "performance", "--work-no", "WORK_NO",
	); err != nil {
		t.Fatalf("performance with work-no: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"profile", "performance",
	); err == nil {
		t.Fatal("performance without work-no should error")
	}
}

func TestHrbrainSearchEmployees(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees",
	); err != nil {
		t.Fatalf("search employees without optional flags: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees",
		"--keyword", "张三", "--dept-name", "技术部", "--position-name", "工程师",
		"--job-level", "P7", "--pool-code", "POOL_CODE",
		"--page", "1", "--page-size", "20",
	); err != nil {
		t.Fatalf("search employees with all optional flags: %v", err)
	}
}

func TestHrbrainSearchEmployeesStructured(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured",
		"--origin-json", `{"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}`,
		"--fields", `[{"label":"姓名","value":"name"}]`,
		"--order-by", "name,dept",
		"--page", "1", "--page-size", "20",
	); err != nil {
		t.Fatalf("search employees-structured with valid args: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured",
		"--origin-json", `{}`,
		"--fields", `not-json`,
	); err == nil {
		t.Fatal("search employees-structured with invalid fields JSON should error")
	}

	invalidOriginJSONCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, invalidOriginJSONCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured",
		"--origin-json", `not-json`,
		"--fields", `[]`,
	); err == nil {
		t.Fatal("search employees-structured with invalid origin-json JSON should error")
	}
	if invalidOriginJSONCaller.calls != 0 {
		t.Fatalf("search employees-structured with invalid origin-json must not call MCP, got %d call(s)", invalidOriginJSONCaller.calls)
	}

	// A bare JSON scalar (e.g. a number) is valid JSON per json.Valid, but it
	// is not a JSON object, so it must still be rejected before dispatch.
	nonObjectOriginJSONCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, nonObjectOriginJSONCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured",
		"--origin-json", `111`,
		"--fields", `[]`,
	); err == nil {
		t.Fatal("search employees-structured with non-object origin-json JSON should error")
	}
	if nonObjectOriginJSONCaller.calls != 0 {
		t.Fatalf("search employees-structured with non-object origin-json must not call MCP, got %d call(s)", nonObjectOriginJSONCaller.calls)
	}

	// An empty --fields array is syntactically valid but selects no columns,
	// so it must be rejected before dispatch.
	emptyFieldsCaller := &scriptedToolCaller{dry: true}
	installScriptedCaller(t, emptyFieldsCaller)
	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured",
		"--origin-json", `{"rules":[],"combinator":"and"}`,
		"--fields", `[]`,
	); err == nil {
		t.Fatal("search employees-structured with empty fields array should error")
	}
	if emptyFieldsCaller.calls != 0 {
		t.Fatalf("search employees-structured with empty fields must not call MCP, got %d call(s)", emptyFieldsCaller.calls)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured", "--fields", `[]`,
	); err == nil {
		t.Fatal("search employees-structured without origin-json should error")
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "employees-structured", "--origin-json", `{}`,
	); err == nil {
		t.Fatal("search employees-structured without fields should error")
	}
}

func TestHrbrainSearchFields(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	if err := executeHrbrainCommand(t, newHrbrainCommand(),
		"search", "fields",
	); err != nil {
		t.Fatalf("search fields: %v", err)
	}
}

func TestHrbrainGroupCommandsWiring(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})

	root := newHrbrainCommand()
	if err := executeHrbrainCommand(t, root); err != nil {
		t.Fatalf("hrbrain root with no args should show help: %v", err)
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(), "talent-pool"); err != nil {
		t.Fatalf("talent-pool group with no args should show help: %v", err)
	}
	if err := executeHrbrainCommand(t, newHrbrainCommand(), "talent-pool", "bogus"); err == nil {
		t.Fatal("talent-pool group with unknown subcommand should error")
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(), "profile"); err != nil {
		t.Fatalf("profile group with no args should show help: %v", err)
	}
	if err := executeHrbrainCommand(t, newHrbrainCommand(), "profile", "bogus"); err == nil {
		t.Fatal("profile group with unknown subcommand should error")
	}

	if err := executeHrbrainCommand(t, newHrbrainCommand(), "search"); err != nil {
		t.Fatalf("search group with no args should show help: %v", err)
	}
	if err := executeHrbrainCommand(t, newHrbrainCommand(), "search", "bogus"); err == nil {
		t.Fatal("search group with unknown subcommand should error")
	}
}
