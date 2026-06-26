package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
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

type aitableSequencedRunner struct {
	calls     []executor.Invocation
	responses []map[string]any
}

func (r *aitableSequencedRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, invocation)
	var response map[string]any
	if idx := len(r.calls) - 1; idx >= 0 && idx < len(r.responses) {
		response = r.responses[idx]
	}
	return executor.Result{Invocation: invocation, Response: response}, nil
}

func TestAitableFieldSearchOptionsRoutesToAitable(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableFieldSearchOptionsCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--field-id", "FIELD_001",
		"--keyword", "已",
		"--limit", "10",
	)

	if got := runner.last.CanonicalProduct; got != "aitable" {
		t.Fatalf("CanonicalProduct = %q, want aitable", got)
	}
	if got := runner.last.Tool; got != "search_field_options" {
		t.Fatalf("Tool = %q, want search_field_options", got)
	}
	if got := runner.last.Params["keyword"]; got != "已" {
		t.Fatalf("keyword = %#v, want 已", got)
	}
	if got := runner.last.Params["limit"]; got != 10 {
		t.Fatalf("limit = %#v, want 10", got)
	}
}

func TestAitableViewGetFieldWidthsProjectsCustomWidthMap(t *testing.T) {
	t.Parallel()

	runner := &aitableSequencedRunner{responses: []map[string]any{{
		"content": map[string]any{
			"status": "success",
			"data": map[string]any{"views": []any{map[string]any{
				"viewId":   "VIEW_001",
				"viewType": "Grid",
				"custom": map[string]any{
					"widthMap": map[string]any{"FIELD_001": 240},
				},
			}}},
		},
	}}}
	cmd := newAitableViewGetFieldWidthsCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--view-id", "VIEW_001",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output JSON parse error = %v\nstdout:\n%s", err, out.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", payload["data"])
	}
	if got := data["FIELD_001"]; got != float64(240) {
		t.Fatalf("FIELD_001 width = %#v, want 240", got)
	}
}

func TestAitableViewUpdateCardDispatchesGalleryConfig(t *testing.T) {
	t.Parallel()

	runner := &aitableSequencedRunner{responses: []map[string]any{
		{"content": map[string]any{
			"status": "success",
			"data": map[string]any{"views": []any{map[string]any{
				"viewId":   "VIEW_001",
				"viewType": "Gallery",
			}}},
		}},
		{"content": map[string]any{"status": "success"}},
	}}
	cmd := newAitableViewUpdateCardCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--view-id", "VIEW_001",
		"--cover-mode", "custom",
		"--cover-field-id", "FIELD_001",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.calls))
	}
	update := runner.calls[1]
	if update.Tool != "update_view" {
		t.Fatalf("second tool = %q, want update_view", update.Tool)
	}
	config, ok := update.Params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want object", update.Params["config"])
	}
	card, ok := config["galleryCard"].(map[string]any)
	if !ok {
		t.Fatalf("galleryCard = %#v, want object", config["galleryCard"])
	}
	if got := card["coverMode"]; got != "custom" {
		t.Fatalf("coverMode = %#v, want custom", got)
	}
}

func TestAitableViewUpdateConfigRoutedKeyHintsSubcommand(t *testing.T) {
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
		"--config", `{"frozenColCount":2}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "frozen-cols") {
		t.Fatalf("stderr missing frozen-cols hint:\n%s", errOut.String())
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

func TestAitableRecordQueryEmptyRoutesToHelper(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordQueryEmptyCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--limit", "50",
		"--cursor", "CUR_001",
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "query_empty_records" {
		t.Fatalf("Tool = %q, want query_empty_records", got)
	}
	if got := runner.last.Params["limit"]; got != 50 {
		t.Fatalf("limit = %#v, want 50", got)
	}
	if got := runner.last.Params["cursor"]; got != "CUR_001" {
		t.Fatalf("cursor = %#v, want CUR_001", got)
	}
}

func TestAitableRecordQueryEmptyRejectsOutOfRangeLimit(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableRecordQueryEmptyCommand(runner)
	cmd.SetArgs([]string{
		"--base-id", "BASE_001",
		"--table-id", "TABLE_001",
		"--limit", "200",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for --limit 200, got nil")
	}
	if runner.last.Tool != "" {
		t.Fatalf("runner should not be called on invalid limit, got tool %q", runner.last.Tool)
	}
}

func TestAitableDashboardArrangeRoutesToHelper(t *testing.T) {
	t.Parallel()

	runner := &aitableCommandRunner{}
	cmd := newAitableDashboardArrangeCommand(runner)
	executeAitableExtraCommand(t, cmd,
		"--base-id", "BASE_001",
		"--dashboard-id", "DASH_001",
	)

	if got := runner.last.CanonicalProduct; got != "aitable-helper" {
		t.Fatalf("CanonicalProduct = %q, want aitable-helper", got)
	}
	if got := runner.last.Tool; got != "align_dashboard" {
		t.Fatalf("Tool = %q, want align_dashboard", got)
	}
	if got := runner.last.Params["dashboardId"]; got != "DASH_001" {
		t.Fatalf("dashboardId = %#v, want DASH_001", got)
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
