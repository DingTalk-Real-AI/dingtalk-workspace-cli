package helpers

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

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
