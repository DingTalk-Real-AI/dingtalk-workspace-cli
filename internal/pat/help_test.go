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
		"DINGTALK_AGENT",
		"claw-type",
		"business-agent-name",
		"为空或为 default",
		"claw-type != default",
		"PAT 返回 JSON",
		"由宿主处理全部 UI / 交互 / 回调节奏 / 重试逻辑",
		"DWS_CHANNEL",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("pat help missing %q\n%s", want, help)
		}
	}
	for _, bad := range []string{"host-control", "rewind-desktop", "dws-wukong", "wukong"} {
		if strings.Contains(help, bad) {
			t.Fatalf("pat help should not mention %q\n%s", bad, help)
		}
	}
}

func TestPatCallbackHelpMentionsStableCommandsAndDingTalkAgentFlow(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, nil)

	cmd, _, err := root.Find([]string{"pat", "callback"})
	if err != nil {
		t.Fatalf("Find(pat callback) error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"DINGTALK_AGENT",
		"claw-type",
		"business-agent-name",
		"claw-type != default",
		"PAT 返回 JSON",
		"由宿主处理全部 UI / 交互 / 回调节奏 / 重试逻辑",
		"list-super-admins",
		"send-apply",
		"poll-flow",
		"DWS_CHANNEL",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("callback help missing %q\n%s", want, help)
		}
	}
	for _, bad := range []string{"host-control", "rewind-desktop", "dws-wukong", "wukong"} {
		if strings.Contains(help, bad) {
			t.Fatalf("callback help should not mention %q\n%s", bad, help)
		}
	}
}
