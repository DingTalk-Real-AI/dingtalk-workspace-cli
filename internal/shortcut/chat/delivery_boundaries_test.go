package chat

import (
	"bytes"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

type deliveryReadFailure struct{}

func (deliveryReadFailure) Read([]byte) (int, error) { return 0, errors.New("fixture read failure") }

func TestCrossPlatformCoverageChatDeliveryInputFailures(t *testing.T) {
	for _, scenario := range []string{"duplicate-body", "read-failure", "ambiguous-operator"} {
		t.Run(scenario, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDepsForTest(t, fake)
			root := newPlatformCoverageRoot()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			args := []string{"chat", "+messages-send", "--text", "-", "--body-stdin", "--yes"}
			root.SetIn(strings.NewReader("body"))
			switch scenario {
			case "duplicate-body":
				args = append(args, "--markdown", "-")
			case "read-failure":
				root.SetIn(deliveryReadFailure{})
			case "ambiguous-operator":
				args = append(args, "--group", "cid", "--employee-context", "{}", "--wait-delivery")
			}
			root.SetArgs(args)
			if err := corecmd.ExecuteForTest(root); err == nil {
				t.Fatal("非法输入被接受")
			}
			if len(fake.calls) != 0 {
				t.Fatalf("非法输入触达 MCP: %v", fake.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatDeliveryRevalidatesBeforeWrite(t *testing.T) {
	for _, leaf := range []string{"+messages-reply", "+messages-send"} {
		t.Run(leaf, func(t *testing.T) {
			fake := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"D-sender"}]}`}}
			helpers.InitDepsForTest(t, fake)
			root := newPlatformCoverageRoot()
			cmd, _, err := root.Find([]string{"chat", leaf})
			if err != nil {
				t.Fatal(err)
			}
			cmd.Flags().AddFlagSet(root.PersistentFlags())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			flags := map[string]string{"group": "cid", "yes": "true"}
			sc := MessagesSend
			if leaf == "+messages-reply" {
				flags["message-id"] = "msg"
				flags["content"] = "body"
				sc = MessagesReply
			} else {
				flags["text"] = "body"
			}
			for k, v := range flags {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			rt := shortcut.RuntimeContextForTest(cmd, sc)
			if err := sc.Validate(rt); err != nil {
				t.Fatal(err)
			}
			// 模拟校验后绑定上下文变化，执行阶段必须在真实写入前再次拒绝。
			if err := cmd.Flags().Set("employee-context", "{"); err != nil {
				t.Fatal(err)
			}
			if err := sc.Execute(rt); err == nil {
				t.Fatal("执行阶段未重验绑定")
			}
			for _, call := range fake.calls {
				if call.tool == "send_personal_message" {
					t.Fatal("失效绑定仍发送了消息")
				}
			}
		})
	}
}

// Cobra 的互斥检查先于 Validate；这里在真实 Shortcut 的校验 seam 固定第二道正文防线。
func TestCrossPlatformCoverageChatDeliveryRejectsTwoSelectedBodies(t *testing.T) {
	cmd := corecmd.New(shortcut.FromShortcut(MessagesSend))
	for k, v := range map[string]string{"text": "-", "markdown": "-", "body-stdin": "true"} {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	err := MessagesSend.Validate(shortcut.RuntimeContextForTest(cmd, MessagesSend))
	if err == nil || !strings.Contains(err.Error(), "exactly one body flag") {
		t.Fatalf("重复正文未被拒绝: %v", err)
	}
}
