package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type reportRegressionCall struct {
	product string
	tool    string
	args    map[string]any
}

type reportRegressionCaller struct {
	format    string
	dryRun    bool
	fields    string
	jq        string
	response  string
	responses map[string]string
	calls     []reportRegressionCall
}

func (c *reportRegressionCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, reportRegressionCall{product: productID, tool: toolName, args: copied})

	text := c.response
	if c.responses != nil {
		if v, ok := c.responses[productID+"/"+toolName]; ok {
			text = v
		}
	}
	if text == "" {
		text = `{"success":true}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (c *reportRegressionCaller) Format() string {
	if c.format != "" {
		return c.format
	}
	return "json"
}

func (c *reportRegressionCaller) DryRun() bool { return c.dryRun }
func (c *reportRegressionCaller) Fields() string {
	return c.fields
}
func (c *reportRegressionCaller) JQ() string {
	return c.jq
}

func setupReportRegressionDeps(t *testing.T, caller *reportRegressionCaller) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	oldDeps := deps
	InitDeps(caller)
	var out, errOut bytes.Buffer
	deps.Out.w = &out
	deps.Out.errW = &errOut
	t.Cleanup(func() { deps = oldDeps })
	return &out, &errOut
}

func executeReportRegressionCommand(t *testing.T, osArgs []string, cmd *cobra.Command, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{}, osArgs...)
	defer func() { os.Args = oldArgs }()

	var cobraOut bytes.Buffer
	cmd.SetOut(&cobraOut)
	cmd.SetErr(&cobraOut)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestAitableShareUpdateAcceptsSpaceSeparatedFalse(t *testing.T) {
	cases := []struct {
		name string
		args []string
		tool string
	}{
		{
			name: "chart",
			args: []string{"chart", "share", "update", "--base-id", "base1", "--dashboard-id", "dash1", "--chart-id", "chart1", "--enabled", "false"},
			tool: "update_chart_share",
		},
		{
			name: "dashboard",
			args: []string{"dashboard", "share", "update", "--base-id", "base1", "--dashboard-id", "dash1", "--enabled", "false"},
			tool: "update_dashboard_share",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &reportRegressionCaller{}
			setupReportRegressionDeps(t, caller)

			if err := executeReportRegressionCommand(t, append([]string{"dws", "aitable"}, tc.args...), newAitableCommand(), tc.args...); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("CallTool count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.tool != tc.tool {
				t.Fatalf("tool = %q, want %q", call.tool, tc.tool)
			}
			if got, ok := call.args["enabled"].(bool); !ok || got {
				t.Fatalf("enabled arg = %#v, want false", call.args["enabled"])
			}
		})
	}
}

func TestAitableChartUpdateAllowsLayoutOnly(t *testing.T) {
	caller := &reportRegressionCaller{}
	setupReportRegressionDeps(t, caller)

	args := []string{"chart", "update", "--base-id", "base1", "--dashboard-id", "dash1", "--chart-id", "chart1", "--layout", `{"x":0,"y":4,"w":12,"h":4}`}
	if err := executeReportRegressionCommand(t, append([]string{"dws", "aitable"}, args...), newAitableCommand(), args...); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(caller.calls))
	}
	if _, ok := caller.calls[0].args["config"]; ok {
		t.Fatalf("config arg should be omitted for layout-only update: %#v", caller.calls[0].args)
	}
	layout, ok := caller.calls[0].args["layout"].(map[string]any)
	if !ok {
		t.Fatalf("layout arg = %#v, want map", caller.calls[0].args["layout"])
	}
	if layout["w"] != float64(12) {
		t.Fatalf("layout.w = %#v, want 12", layout["w"])
	}
}

func TestAitableFormGetFiltersByViewID(t *testing.T) {
	caller := &reportRegressionCaller{responses: map[string]string{
		"aitable-helper/list_form_views": `{"data":{"formViews":[{"viewId":"view1","name":"one"},{"viewId":"view2","name":"two"}],"total":2}}`,
	}}
	out, _ := setupReportRegressionDeps(t, caller)

	args := []string{"form", "get", "--base-id", "base1", "--table-id", "tbl1", "--view-id", "view2"}
	if err := executeReportRegressionCommand(t, append([]string{"dws", "aitable"}, args...), newAitableCommand(), args...); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	data := got["data"].(map[string]any)
	views := data["formViews"].([]any)
	if len(views) != 1 {
		t.Fatalf("formViews length = %d, want 1: %#v", len(views), views)
	}
	if view := views[0].(map[string]any); view["viewId"] != "view2" {
		t.Fatalf("filtered view = %#v, want view2", view)
	}
	if data["total"] != float64(1) {
		t.Fatalf("total = %#v, want 1", data["total"])
	}
}

func TestAitableWidgetsExampleFixesMissingJSONCCommas(t *testing.T) {
	raw := "{\n  \"LINE\": {\n    \"config\": 1\n  }\n  \"BAR\": {\n    \"config\": 2\n  }\n}"
	fixed := fixJSONCMissingCommas(raw)
	if !strings.Contains(fixed, "},\n  \"BAR\"") {
		t.Fatalf("missing comma was not inserted:\n%s", fixed)
	}
	if !json.Valid([]byte(fixed)) {
		t.Fatalf("fixed JSON is invalid:\n%s", fixed)
	}
}

func TestAggregateClearIsRejectedLocally(t *testing.T) {
	if !aggregateBlockHasClear(map[string]any{"fld1": nil}) {
		t.Fatal("aggregateBlockHasClear should detect nil clear values")
	}
	if aggregateBlockHasClear(map[string]any{"fld1": "SUM"}) {
		t.Fatal("aggregateBlockHasClear should allow concrete aggregate actions")
	}
}

func TestChatConversationInfoUserUsesPeerUid(t *testing.T) {
	caller := &reportRegressionCaller{}
	setupReportRegressionDeps(t, caller)

	args := []string{"conversation-info", "--user", "011769261608"}
	if err := executeReportRegressionCommand(t, append([]string{"dws", "chat"}, args...), newChatCommand(), args...); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.product != "chat" || call.tool != "get_conversation_info" {
		t.Fatalf("call = %s/%s, want chat/get_conversation_info", call.product, call.tool)
	}
	if got := call.args["peerUid"]; got != "011769261608" {
		t.Fatalf("peerUid = %#v, want user id", got)
	}
	if _, ok := call.args["userId"]; ok {
		t.Fatalf("userId should not be sent: %#v", call.args)
	}
}

func TestChatWebhookErrorReturnsFailure(t *testing.T) {
	caller := &reportRegressionCaller{responses: map[string]string{
		"bot/send_message_by_custom_robot": `{"errcode":"300005","errmsg":"token is not exist","success":true}`,
	}}
	setupReportRegressionDeps(t, caller)

	args := []string{"message", "send-by-webhook", "--token", "bad", "--title", "t", "--text", "body"}
	err := executeReportRegressionCommand(t, append([]string{"dws", "chat"}, args...), newChatCommand(), args...)
	if err == nil {
		t.Fatal("Execute() error = nil, want webhook failure")
	}
	if !strings.Contains(err.Error(), "errcode=300005") {
		t.Fatalf("error = %q, want errcode detail", err.Error())
	}
	if len(caller.calls) != 1 || caller.calls[0].product != "bot" || caller.calls[0].tool != "send_message_by_custom_robot" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
}

func TestChatAuditAndConversationLimitValidateLocally(t *testing.T) {
	t.Run("audit unsupported status", func(t *testing.T) {
		caller := &reportRegressionCaller{}
		setupReportRegressionDeps(t, caller)
		args := []string{"group", "audit-join-validation", "--group", "cid", "--record-id", "123", "--applicant", "applicant", "--inviter", "inviter", "--status", "AuditRefuse"}
		err := executeReportRegressionCommand(t, append([]string{"dws", "chat"}, args...), newChatCommand(), args...)
		if err == nil || !strings.Contains(err.Error(), "AuditApprove or AuditDelete") {
			t.Fatalf("error = %v, want supported status validation", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("server should not be called: %#v", caller.calls)
		}
	})

	t.Run("limit greater than 100", func(t *testing.T) {
		caller := &reportRegressionCaller{}
		setupReportRegressionDeps(t, caller)
		args := []string{"list-all-conversations", "--limit", "101"}
		err := executeReportRegressionCommand(t, append([]string{"dws", "chat"}, args...), newChatCommand(), args...)
		if err == nil || !strings.Contains(err.Error(), "between 1 and 100") {
			t.Fatalf("error = %v, want limit validation", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("server should not be called: %#v", caller.calls)
		}
	})
}

func TestContactDeptPrimaryFlagsAreAccepted(t *testing.T) {
	cases := []struct {
		name string
		args []string
		tool string
	}{
		{name: "get-info", args: []string{"dept", "get-info", "--dept", "1"}, tool: "get_dept_info_by_dept_id"},
		{name: "list-children", args: []string{"dept", "list-children", "--dept", "1"}, tool: "get_sub_depts_by_dept_id"},
		{name: "list-members", args: []string{"dept", "list-members", "--depts", "1,2"}, tool: "get_dept_members_by_deptId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &reportRegressionCaller{}
			setupReportRegressionDeps(t, caller)
			if err := executeReportRegressionCommand(t, append([]string{"dws", "contact"}, tc.args...), newContactCommand(), tc.args...); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool {
				t.Fatalf("unexpected calls: %#v", caller.calls)
			}
		})
	}
}

func TestWikiNodeCreateHelpAndTypeValidation(t *testing.T) {
	t.Run("help omits asheet", func(t *testing.T) {
		caller := &reportRegressionCaller{}
		setupReportRegressionDeps(t, caller)
		cmd := newWikiCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"node", "create", "--help"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if strings.Contains(out.String(), "asheet") {
			t.Fatalf("node create help should not mention asheet:\n%s", out.String())
		}
	})

	t.Run("asheet rejected locally", func(t *testing.T) {
		caller := &reportRegressionCaller{}
		setupReportRegressionDeps(t, caller)
		args := []string{"node", "create", "--workspace", "ws1", "--name", "bad", "--type", "asheet"}
		err := executeReportRegressionCommand(t, append([]string{"dws", "wiki"}, args...), newWikiCommand(), args...)
		if err == nil || !strings.Contains(err.Error(), "supported values") {
			t.Fatalf("error = %v, want type validation", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("server should not be called: %#v", caller.calls)
		}
	})
}

func TestDevDocSearchRoutesToDevdocServer(t *testing.T) {
	caller := &reportRegressionCaller{}
	setupReportRegressionDeps(t, caller)

	args := []string{"doc", "search", "--query", "MCP"}
	if err := executeReportRegressionCommand(t, append([]string{"dws", "dev"}, args...), devHandler{}.Command(nil), args...); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(caller.calls))
	}
	if caller.calls[0].product != "devdoc" || caller.calls[0].tool != "search_open_platform_docs" {
		t.Fatalf("call = %s/%s, want devdoc/search_open_platform_docs", caller.calls[0].product, caller.calls[0].tool)
	}
}

func TestGlobalFiltersApplyToHelperToolOutput(t *testing.T) {
	caller := &reportRegressionCaller{
		format:   "table",
		fields:   "name",
		response: `{"name":"visible","secret":"hidden"}`,
	}
	out, _ := setupReportRegressionDeps(t, caller)

	if err := callMCPToolOnServer("chat", "example_tool", map[string]any{}); err != nil {
		t.Fatalf("callMCPToolOnServer() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "visible") {
		t.Fatalf("filtered output missing selected field:\n%s", got)
	}
	if strings.Contains(got, "hidden") || strings.Contains(got, "secret") {
		t.Fatalf("filtered output leaked unselected field:\n%s", got)
	}
}

func TestPrintMCPTextAppliesJQWithoutJSONFormat(t *testing.T) {
	caller := &reportRegressionCaller{format: "raw", jq: ".name"}
	out, _ := setupReportRegressionDeps(t, caller)

	if err := printMCPText(`{"name":"visible","secret":"hidden"}`); err != nil {
		t.Fatalf("printMCPText() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"visible"`) {
		t.Fatalf("jq output missing selected value:\n%s", got)
	}
	if strings.Contains(got, "hidden") {
		t.Fatalf("jq output leaked unselected value:\n%s", got)
	}
}

func TestSanitizeEmptyObjectListsRemovesGhostItems(t *testing.T) {
	payload := map[string]any{
		"rules": []any{
			map[string]any{"id": nil, "name": ""},
			map[string]any{"id": "rule1", "name": "real"},
		},
		"floatCharts": []any{
			map[string]any{"chart": map[string]any{"category": nil, "series": []any{map[string]any{"value": nil}}}},
		},
	}
	sanitizeEmptyObjectLists(payload, map[string]bool{"rules": true, "floatCharts": true})

	rules := payload["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules length = %d, want 1: %#v", len(rules), rules)
	}
	charts := payload["floatCharts"].([]any)
	if len(charts) != 0 {
		t.Fatalf("floatCharts length = %d, want 0: %#v", len(charts), charts)
	}
}
