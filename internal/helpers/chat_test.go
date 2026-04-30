package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type captureRunner struct {
	last executor.Invocation
}

func (r *captureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func TestChatMessageSendByBotIgnoresLegacyRealBuildModeEnv(t *testing.T) {
	t.Setenv("DWS_"+"BUILD_MODE", "real")

	runner := &captureRunner{}
	cmd := newChatMessageSendByBotCommand(runner)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--users", "user-001",
		"--robot-code", "robot-001",
		"--title", "Greeting",
		"--text", "hello",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if got := runner.last.Tool; got != "batch_send_robot_msg_to_users" {
		t.Fatalf("tool = %q, want batch_send_robot_msg_to_users", got)
	}
	if got := runner.last.Params["robotCode"]; got != "robot-001" {
		t.Fatalf("robotCode = %#v, want robot-001", got)
	}
	if got := runner.last.CanonicalProduct; got != "bot" {
		t.Fatalf("CanonicalProduct = %q, want bot", got)
	}
}

func TestChatMessageSendRoutesByDestination(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantTool  string
		wantKey   string
		wantValue string
	}{
		{
			name:      "group",
			args:      []string{"--group", "cid-xyz", "--text", "hello"},
			wantTool:  "send_message_as_user",
			wantKey:   "openConversation_id",
			wantValue: "cid-xyz",
		},
		{
			name:      "user-direct",
			args:      []string{"--user", "034766", "--text", "hi"},
			wantTool:  "send_direct_message_as_user",
			wantKey:   "receiverUserId",
			wantValue: "034766",
		},
		{
			name:      "open-dingtalk-id-direct",
			args:      []string{"--open-dingtalk-id", "OP123", "--text", "hi"},
			wantTool:  "send_direct_message_as_user",
			wantKey:   "receiverOpenDingTalkId",
			wantValue: "OP123",
		},
		{
			name:      "positional-text",
			args:      []string{"--group", "cid-xyz", "hello from positional"},
			wantTool:  "send_message_as_user",
			wantKey:   "text",
			wantValue: "hello from positional",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := newChatMessageSendCommand(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
			if got := runner.last.CanonicalProduct; got != "chat" {
				t.Fatalf("CanonicalProduct = %q, want chat", got)
			}
			if got, ok := runner.last.Params[tc.wantKey]; !ok || got != tc.wantValue {
				t.Fatalf("Params[%q] = %#v, want %q", tc.wantKey, got, tc.wantValue)
			}
		})
	}
}

func TestChatMessageSendRejectsInvalidDestination(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no-destination",
			args:    []string{"--text", "hi"},
			wantErr: "one of --group, --user, or --open-dingtalk-id is required",
		},
		{
			name:    "group-and-user",
			args:    []string{"--group", "cid-x", "--user", "034766", "--text", "hi"},
			wantErr: "--group, --user, and --open-dingtalk-id are mutually exclusive",
		},
		{
			name:    "empty-text",
			args:    []string{"--group", "cid-x"},
			wantErr: "--text (or positional argument) is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := newChatMessageSendCommand(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil; output: %s", out.String())
			}
			if got := err.Error(); !strings.Contains(got, tc.wantErr) {
				t.Fatalf("error = %q, want to contain %q", got, tc.wantErr)
			}
		})
	}
}

// TestChatMessageSendForwardsAtMentions guards the regression introduced
// alongside the destination-based routing in PR #170: the hardcoded helper
// declared --group / --user / --open-dingtalk-id / --text / --title but
// dropped the v1.0.15 envelope's --at-users / --at-all / --at-mobiles flags,
// so `dws chat message send --group ... --at-users ...` failed with
// `unknown flag: --at-users` (issue #177).
func TestChatMessageSendForwardsAtMentions(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantParams map[string]any
	}{
		{
			name: "group-with-at-users",
			args: []string{
				"--group", "cid-xyz",
				"--title", "拉群通知",
				"--text", "<@uid-1> <@uid-2> 请关注",
				"--at-users", "uid-1,uid-2",
			},
			wantParams: map[string]any{
				"openConversation_id": "cid-xyz",
				"title":               "拉群通知",
				"text":                "<@uid-1> <@uid-2> 请关注",
				"atUserIds":           []any{"uid-1", "uid-2"},
				"msgType":             "actionCard",
			},
		},
		{
			name: "group-with-at-all",
			args: []string{
				"--group", "cid-xyz",
				"--title", "全员通知",
				"--text", "<@all> 请关注",
				"--at-all",
			},
			wantParams: map[string]any{
				"openConversation_id": "cid-xyz",
				"title":               "全员通知",
				"text":                "<@all> 请关注",
				"isAtAll":             true,
				"msgType":             "actionCard",
			},
		},
		{
			name: "group-with-at-mobiles",
			args: []string{
				"--group", "cid-xyz",
				"--title", "提醒",
				"--text", "请 13800000000 确认",
				"--at-mobiles", "13800000000,13900000000",
			},
			wantParams: map[string]any{
				"openConversation_id": "cid-xyz",
				"title":               "提醒",
				"text":                "请 13800000000 确认",
				"atMobiles":           []any{"13800000000", "13900000000"},
				"msgType":             "actionCard",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := newChatMessageSendCommand(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.Tool; got != "send_message_as_user" {
				t.Fatalf("Tool = %q, want send_message_as_user", got)
			}
			for key, want := range tc.wantParams {
				got, ok := runner.last.Params[key]
				if !ok {
					t.Fatalf("Params missing %q; got %#v", key, runner.last.Params)
				}
				if !equalAny(got, want) {
					t.Fatalf("Params[%q] = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}

// TestChatMessageSendRejectsAtMentionsOutsideGroup ensures we do not silently
// drop user intent when --at-* is combined with --user / --open-dingtalk-id
// (single-chat tools have no @-mention semantics, so the flag would never
// take effect — fail loudly instead of swallowing).
// TestChatMessageSend_NoTitle_NoMsgType verifies that when --title is
// omitted, msgType is NOT set at all (the group-chat API requires title
// and does not support msgType "text").
func TestChatMessageSend_NoTitle_NoMsgType(t *testing.T) {
	runner := &captureRunner{}
	cmd := newChatMessageSendCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--group", "cid-xyz", "--text", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, hasMsgType := runner.last.Params["msgType"]; hasMsgType {
		t.Errorf("msgType should not be set when --title is empty, got %v", runner.last.Params["msgType"])
	}
	if _, hasTitle := runner.last.Params["title"]; hasTitle {
		t.Errorf("title should not be set when --title is empty")
	}
}

// TestChatMessageSend_WithTitle_SetsActionCard verifies that providing --title
// without --msg-type automatically sets msgType to "actionCard".
func TestChatMessageSend_WithTitle_SetsActionCard(t *testing.T) {
	runner := &captureRunner{}
	cmd := newChatMessageSendCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--group", "cid-xyz", "--title", "通知", "--text", "请查收"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runner.last.Params["msgType"]; got != "actionCard" {
		t.Errorf("msgType = %v, want actionCard when --title is provided", got)
	}
	if got := runner.last.Params["title"]; got != "通知" {
		t.Errorf("title = %v, want \"通知\"", got)
	}
}

// TestChatMessageSend_ExplicitMsgType_Overrides verifies that --msg-type takes
// precedence over the title-based inference.
func TestChatMessageSend_ExplicitMsgType_Overrides(t *testing.T) {
	runner := &captureRunner{}
	cmd := newChatMessageSendCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--group", "cid-xyz", "--text", "hello", "--msg-type", "markdown"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runner.last.Params["msgType"]; got != "markdown" {
		t.Errorf("msgType = %v, want markdown", got)
	}
}

// TestChatMessageSend_ExplicitMsgTypeWithTitle verifies that --msg-type +
// --title passes both through without overriding msgType to actionCard.
func TestChatMessageSend_ExplicitMsgTypeWithTitle(t *testing.T) {
	runner := &captureRunner{}
	cmd := newChatMessageSendCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--group", "cid-xyz", "--title", "标题", "--text", "正文", "--msg-type", "markdown"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := runner.last.Params["msgType"]; got != "markdown" {
		t.Errorf("msgType = %v, want markdown (explicit override)", got)
	}
	if got := runner.last.Params["title"]; got != "标题" {
		t.Errorf("title = %v, want \"标题\"", got)
	}
}

func TestChatMessageSendRejectsAtMentionsOutsideGroup(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "user-with-at-users",
			args: []string{"--user", "034766", "--text", "hi", "--at-users", "uid-1"},
		},
		{
			name: "open-dingtalk-id-with-at-all",
			args: []string{"--open-dingtalk-id", "OP123", "--text", "hi", "--at-all"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := newChatMessageSendCommand(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil; output: %s", out.String())
			}
			if !strings.Contains(err.Error(), "only apply when --group is set") {
				t.Fatalf("error = %q, want '...only apply when --group is set'", err.Error())
			}
		})
	}
}

func equalAny(a, b any) bool {
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// TestChatGroupMembersListSubcommand pins the explicit `list` subcommand
// added for issue #164: previously the bare `chat group members --id` was
// the list path, but it shape-mismatched the dynamic envelope's `members`
// leaf and got eaten by the merge layer. Now `dws chat group members list
// --id <cid>` is a proper leaf siblings of add/remove/add-bot.
func TestChatGroupMembersListSubcommand(t *testing.T) {
	runner := &captureRunner{}
	groupCmd := newChatGroupCommand(runner)
	var members *cobra.Command
	for _, sub := range groupCmd.Commands() {
		if sub.Name() == "members" {
			members = sub
			break
		}
	}
	if members == nil {
		t.Fatalf("members subcommand missing under chat group")
	}

	want := map[string]bool{"list": false, "add": false, "remove": false, "add-bot": false}
	for _, leaf := range members.Commands() {
		if _, ok := want[leaf.Name()]; ok {
			want[leaf.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected `chat group members %s` subcommand, missing", name)
		}
	}

	if members.Flags().Lookup("id") != nil {
		t.Errorf("members container should not declare --id (moved to `list` subcommand to avoid shape-mismatch with dynamic envelope)")
	}

	var listCmd *cobra.Command
	for _, leaf := range members.Commands() {
		if leaf.Name() == "list" {
			listCmd = leaf
			break
		}
	}
	if listCmd == nil {
		t.Fatalf("`list` subcommand not found")
	}
	if listCmd.Flags().Lookup("id") == nil {
		t.Errorf("`list` subcommand must declare --id")
	}
	if listCmd.Flags().Lookup("cursor") == nil {
		t.Errorf("`list` subcommand must declare --cursor")
	}

	// Drive execution via the group root so cobra resolves the subcommand
	// path properly (calling Execute() on a child directly would re-enter
	// the root help branch).
	var out bytes.Buffer
	groupCmd.SetOut(&out)
	groupCmd.SetErr(&out)
	groupCmd.SetArgs([]string{"members", "list", "--id", "cid-xyz"})
	if err := groupCmd.Execute(); err != nil {
		t.Fatalf("members list Execute error = %v\noutput: %s", err, out.String())
	}
	if got := runner.last.Tool; got != "get_group_members" {
		t.Fatalf("Tool = %q, want get_group_members", got)
	}
	if got := runner.last.Params["openconversation_id"]; got != "cid-xyz" {
		t.Fatalf("openconversation_id = %#v, want cid-xyz", got)
	}
}

func TestChatMessageSendByBotRoutesToBotProduct(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantTool string
	}{
		{
			name: "single-chat",
			args: []string{
				"--users", "user-001",
				"--robot-code", "robot-001",
				"--title", "t",
				"--text", "x",
			},
			wantTool: "batch_send_robot_msg_to_users",
		},
		{
			name: "group-chat",
			args: []string{
				"--group", "cid-xyz",
				"--robot-code", "robot-001",
				"--title", "t",
				"--text", "x",
			},
			wantTool: "send_robot_group_message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := newChatMessageSendByBotCommand(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if got := runner.last.CanonicalProduct; got != "bot" {
				t.Fatalf("CanonicalProduct = %q, want bot", got)
			}
			if got := runner.last.Tool; got != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", got, tc.wantTool)
			}
		})
	}
}
