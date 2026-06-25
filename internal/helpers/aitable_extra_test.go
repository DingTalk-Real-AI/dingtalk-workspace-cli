package helpers

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func executeAitableExtraCommand(t *testing.T, cmd *cobra.Command, args ...string) {
	t.Helper()

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
}

func TestAitableRecordHistoryListRoutesToHelper(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordHistoryListCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--record-id", "REC_001",
		"--offset", "10",
		"--limit", "30",
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "query_record_history" {
		t.Fatalf("Tool = %q, want query_record_history", got)
	}
	if got := runner.last.Params["recordId"]; got != "REC_001" {
		t.Fatalf("recordId = %#v, want REC_001", got)
	}
	if got := runner.last.Params["offset"]; got != 10 {
		t.Fatalf("offset = %#v, want 10", got)
	}
	if got := runner.last.Params["limit"]; got != 30 {
		t.Fatalf("limit = %#v, want 30", got)
	}
}

func TestAitableRecordUpsertAcceptsFieldsAlias(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordUpsertCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--fields", `[{"recordId":"REC_001","cells":{"fld":"updated"}},{"cells":{"fld":"new"}}]`,
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "record_upsert" {
		t.Fatalf("Tool = %q, want record_upsert", got)
	}
	records, ok := runner.last.Params["records"].([]any)
	if !ok {
		t.Fatalf("records type = %T, want []any", runner.last.Params["records"])
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
}

func TestAitableViewExtraCommandsRouteToExpectedTools(t *testing.T) {
	t.Parallel()

	t.Run("lock unlock", func(t *testing.T) {
		t.Parallel()
		runner := &aitableCommandRunner{}
		cmd := newAitableViewLockCommand(runner)
		executeAitableExtraCommand(t, cmd,
			"--base-id", "BASE_001",
			"--table-id", "TABLE_001",
			"--view-id", "VIEW_001",
			"--off",
		)
		if got := runner.last.CanonicalProduct; got != "aitable-helper" {
			t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
		}
		if got := runner.last.Tool; got != "lock_or_unlock_view" {
			t.Fatalf("Tool = %q, want lock_or_unlock_view", got)
		}
		if got := runner.last.Params["action"]; got != "unlock" {
			t.Fatalf("action = %#v, want unlock", got)
		}
	})

	t.Run("fill color rule", func(t *testing.T) {
		t.Parallel()
		runner := &aitableCommandRunner{}
		cmd := newAitableViewUpdateFillColorRuleCommand(runner)
		executeAitableExtraCommand(t, cmd,
			"--base-id", "BASE_001",
			"--table-id", "TABLE_001",
			"--view-id", "VIEW_001",
			"--json", `[]`,
		)
		if got := runner.last.CanonicalProduct; got != "aitable" {
			t.Fatalf("CanonicalProduct = %q, want aitable", got)
		}
		if got := runner.last.Tool; got != "set_view_fill_color_rule" {
			t.Fatalf("Tool = %q, want set_view_fill_color_rule", got)
		}
		if formats, ok := runner.last.Params["conditionalFormats"].([]any); !ok || len(formats) != 0 {
			t.Fatalf("conditionalFormats = %#v, want empty []any", runner.last.Params["conditionalFormats"])
		}
	})
}

func TestAitableWorkflowListRoutesToHelper(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableWorkflowListCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--limit", "50",
		"--offset", "100",
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "list_workflows" {
		t.Fatalf("Tool = %q, want list_workflows", got)
	}
	if got := runner.last.Params["limit"]; got != 50 {
		t.Fatalf("limit = %#v, want 50", got)
	}
	if got := runner.last.Params["offset"]; got != 100 {
		t.Fatalf("offset = %#v, want 100", got)
	}
}

func TestAitableAdvpermRoleCreateParsesSubRoles(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableAdvpermRoleCreateCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--name", "市场可读",
		"--sub-roles", `[{"targetId":"TABLE_001","targetType":"sheet","authLevel":"read"}]`,
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "create_role" {
		t.Fatalf("Tool = %q, want create_role", got)
	}
	subRoles, ok := runner.last.Params["subRoles"].([]any)
	if !ok || len(subRoles) != 1 {
		t.Fatalf("subRoles = %#v, want single-item []any", runner.last.Params["subRoles"])
	}
}

func TestAitableSectionMoveNodeAllowsRootParent(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableSectionMoveNodeCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--node-id", "NODE_001",
		"--new-parent-section-id", "",
		"--target-index", "0",
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "move_nsheet_node" {
		t.Fatalf("Tool = %q, want move_nsheet_node", got)
	}
	if got := runner.last.Params["newParentSectionId"]; got != "" {
		t.Fatalf("newParentSectionId = %#v, want empty string", got)
	}
	if got := runner.last.Params["targetIndex"]; got != 0 {
		t.Fatalf("targetIndex = %#v, want 0", got)
	}
}
