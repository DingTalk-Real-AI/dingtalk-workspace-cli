package pat

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPatHelpMentionsDingTalkAgentHostFlow(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, nil)

	cmd, _, err := root.Find([]string{"pat"})
	if err != nil {
		t.Fatalf("Find(pat) error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"DINGTALK_DWS_AGENTCODE",
		"DINGTALK_AGENT",
		"claw-type",
		"x-dingtalk-agent",
		"openClaw",
		"host-owned",
		"stderr JSON",
		"exit=4",
		"由宿主处理全部 UI / 交互 / 回调节奏 / 重试逻辑",
		"DWS_CHANNEL",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("pat help missing %q\n%s", want, help)
		}
	}
	for _, bad := range []string{"host-control", "rewind-desktop", "dws-wukong", "wukong", "callback"} {
		if strings.Contains(help, bad) {
			t.Fatalf("pat help should not mention %q\n%s", bad, help)
		}
	}
}

func TestPatCallbackCommandIsNotRegistered(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, nil)

	cmd, _, err := root.Find([]string{"pat"})
	if err != nil {
		t.Fatalf("Find(pat) error = %v", err)
	}

	for _, sub := range cmd.Commands() {
		if sub.Name() == "callback" {
			t.Fatalf("pat subcommands unexpectedly include %q", sub.Name())
		}
	}
}
