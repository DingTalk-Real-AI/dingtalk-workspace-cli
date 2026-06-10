package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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
			// 群聊对齐 wukong：tool=send_personal_message，会话键=openConversationId
			name:      "group",
			args:      []string{"--group", "cid-xyz", "--title", "t", "--text", "hello"},
			wantTool:  "send_personal_message",
			wantKey:   "openConversationId",
			wantValue: "cid-xyz",
		},
		{
			name:      "user-direct",
			args:      []string{"--user", "034766", "--title", "t", "--text", "hi"},
			wantTool:  "send_direct_message_as_user",
			wantKey:   "receiverUserId",
			wantValue: "034766",
		},
		{
			// openDingTalkId 单聊也走 send_personal_message（content 携带正文）
			name:      "open-dingtalk-id-direct",
			args:      []string{"--open-dingtalk-id", "OP123", "--title", "t", "--text", "hi"},
			wantTool:  "send_personal_message",
			wantKey:   "receiverOpenDingTalkId",
			wantValue: "OP123",
		},
		{
			// 群聊正文打包进 content JSON（键序按 encoding/json 字典序：text 在 title 前）
			name:      "positional-text",
			args:      []string{"--group", "cid-xyz", "--title", "t", "hello from positional"},
			wantTool:  "send_personal_message",
			wantKey:   "content",
			wantValue: `{"text":"hello from positional","title":"t"}`,
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
		// 注：--title 不再强制必填——缺省时由 deriveTitleFromText 从正文自动派生
		// (对齐 wukong)，故原 *-without-title 的"必须报错"用例已随实现移除。
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

// TestChatMessageSendForwardsAtMentions guards that group @-mentions survive the
// destination-based routing. After aligning `send` with wukong, group messages go
// through the send_personal_message tool and the @ surface is --at-all (→ atAll)
// and --at-open-dingtalk-ids (→ atOpenDingTalkIds, openDingTalkId-based). The
// pre-wukong envelope flags (--at-users / --at-mobiles) no longer exist on `send`;
// regressing them would resurface `unknown flag: --at-...` (issue #177).
func TestChatMessageSendForwardsAtMentions(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantParams map[string]any
	}{
		{
			name: "group-with-at-open-dingtalk-ids",
			args: []string{
				"--group", "cid-xyz",
				"--title", "拉群通知",
				"--text", "<@op-1> <@op-2> 请关注",
				"--at-open-dingtalk-ids", "op-1,op-2",
			},
			wantParams: map[string]any{
				"openConversationId": "cid-xyz",
				"atOpenDingTalkIds":  []string{"op-1", "op-2"},
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
				"openConversationId": "cid-xyz",
				"atAll":              true,
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
			if got := runner.last.Tool; got != "send_personal_message" {
				t.Fatalf("Tool = %q, want send_personal_message", got)
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

// TestChatMessageSendContentNotHTMLEscaped guards the @-mention rendering fix:
// the send_personal_message content must keep literal <@openDingTalkId> / <@all>
// tokens. If json.Marshal's default HTML escaping is reintroduced, the tokens
// become <@...> and the DingTalk client renders them as plain text
// instead of a real @-mention.
func TestChatMessageSendContentNotHTMLEscaped(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // literal token that must survive in content
	}{
		{
			name: "group-at-all",
			args: []string{"--group", "cid-xyz", "--title", "t", "--text", "<@all> hi", "--at-all"},
			want: "<@all>",
		},
		{
			name: "group-at-open-dingtalk-id",
			args: []string{"--group", "cid-xyz", "--title", "t", "--text", "<@op-1> hi", "--at-open-dingtalk-ids", "op-1"},
			want: "<@op-1>",
		},
		{
			name: "direct-open-dingtalk-id",
			args: []string{"--open-dingtalk-id", "OP123", "--title", "t", "--text", "<@OP123> hi"},
			want: "<@OP123>",
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
			content, _ := runner.last.Params["content"].(string)
			if !strings.Contains(content, tc.want) {
				t.Fatalf("content %q missing literal %q (HTML-escaped?)", content, tc.want)
			}
			if strings.Contains(content, "\\u003c") || strings.Contains(content, "\\u003e") {
				t.Fatalf("content %q is HTML-escaped; @-mention will not render", content)
			}
		})
	}
}

// TestChatMessageSendRejectsAtMentionsOutsideGroup ensures we do not silently
// drop user intent when --at-* is combined with --user / --open-dingtalk-id
// (single-chat tools have no @-mention semantics, so the flag would never
// take effect — fail loudly instead of swallowing).
func TestChatMessageSendRejectsAtMentionsOutsideGroup(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "user-with-at-open-dingtalk-ids",
			args: []string{"--user", "034766", "--text", "hi", "--at-open-dingtalk-ids", "op-1"},
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

// TestChatMessageSendCarriesClawType guards the "Send from AI" indicator:
// every user-identity send path must attach the edition claw identity as the
// clawType tool argument so the IM server renders the AI-sent label on the
// delivered message. The open-source build pins it to edition.DefaultOSSClawType
// ("openClaw"); dropping the parameter (or hardcoding another edition's value,
// as the reply command once did with "wukong") mislabels or unlabels messages.
func TestChatMessageSendCarriesClawType(t *testing.T) {
	cases := []struct {
		name string
		make func(runner executor.Runner) *cobra.Command
		args []string
	}{
		{
			name: "group-markdown",
			make: newChatMessageSendCommand,
			args: []string{"--group", "cid-xyz", "--title", "t", "--text", "hello"},
		},
		{
			name: "user-direct",
			make: newChatMessageSendCommand,
			args: []string{"--user", "034766", "--title", "t", "--text", "hi"},
		},
		{
			name: "open-dingtalk-id-direct",
			make: newChatMessageSendCommand,
			args: []string{"--open-dingtalk-id", "OP123", "--title", "t", "--text", "hi"},
		},
		{
			name: "group-rich-media-image",
			make: newChatMessageSendCommand,
			args: []string{"--group", "cid-xyz", "--msg-type", "image", "--media-id", "media-1"},
		},
		{
			name: "reply",
			make: newChatMessageReplyCommand,
			args: []string{
				"--conversation-id", "cid-xyz",
				"--ref-msg-id", "msg-1",
				"--ref-sender", "op-1",
				"--text", "got it",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			cmd := tc.make(runner)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			got, ok := runner.last.Params["clawType"]
			if !ok {
				t.Fatalf("Params missing clawType; got %#v", runner.last.Params)
			}
			if got != edition.DefaultOSSClawType {
				t.Fatalf("clawType = %#v, want %q", got, edition.DefaultOSSClawType)
			}
		})
	}
}

// Robot sends are rendered as bot messages already; they must NOT carry the
// user-identity clawType argument.
func TestChatMessageSendByBotOmitsClawType(t *testing.T) {
	runner := &captureRunner{}
	cmd := newChatMessageSendByBotCommand(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--group", "cid-xyz", "--robot-code", "robot-001", "--title", "t", "--text", "x"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, ok := runner.last.Params["clawType"]; ok {
		t.Fatalf("bot send must not carry clawType; got %#v", runner.last.Params)
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
	case []string:
		// splitCSVStrings 产出 []string（如 atOpenDingTalkIds），用例期望值也写成 []string
		bv, ok := b.([]string)
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
