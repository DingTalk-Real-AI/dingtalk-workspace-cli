package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
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
