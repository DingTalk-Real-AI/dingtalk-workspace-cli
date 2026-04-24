package helpers

import (
	"bytes"
	"context"
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
