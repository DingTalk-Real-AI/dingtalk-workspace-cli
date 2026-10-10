package chat

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageChatDeliveryBodyAndReceipt(t *testing.T) {
	for _, leaf := range []string{"+messages-reply", "+messages-send"} {
		for _, scenario := range []string{"success", "literal", "empty", "oversize", "wrong-sentinel", "no-body", "wrong-mode", "dry-run", "no-confirmation", "body-alias"} {
			t.Run(leaf+"/"+scenario, func(t *testing.T) {
				fake := &larkAlignmentCaller{responses: map[string]string{
					"im/list_messages_by_ids":      `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"D-sender"}]}`,
					"chat/send_personal_message":   `{"openTaskId":"task"}`,
					"im/query_message_send_status": `{"openMessageId":"sent","openConversationId":"cid","sendStatus":"SUCCESS"}`,
				}}
				helpers.InitDepsForTest(t, fake)
				root := newPlatformCoverageRoot()
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&bytes.Buffer{})
				body := "正文含引号 \" 和换行\n第二行"
				field := "--content"
				args := []string{"chat", leaf, "--group", "cid", "--idempotency-key", "key", "--wait-delivery"}
				if leaf == "+messages-reply" {
					args = append(args, "--message-id", "msg")
					if scenario == "body-alias" {
						field = "--markdown"
					}
				} else {
					field = "--text"
				}
				value := "-"
				if scenario == "empty" {
					body = " "
				}
				if scenario == "oversize" {
					body = strings.Repeat("x", (256<<10)+1)
				}
				if scenario == "wrong-sentinel" {
					value = "body"
				}
				if scenario != "no-body" {
					args = append(args, field, value)
				}
				if scenario != "literal" {
					args = append(args, "--body-stdin")
				}
				if scenario != "no-confirmation" {
					args = append(args, "--yes")
				}
				if scenario == "wrong-mode" {
					if leaf == "+messages-reply" {
						args = append(args, "--as", "bot", "--robot-code", "code")
					} else {
						args = append(args, "--as", "webhook", "--webhook-token", "fixture")
					}
				}
				if scenario == "dry-run" {
					args = append(args, "--dry-run")
					fake.dryRun = true
				}
				root.SetIn(strings.NewReader(body))
				root.SetArgs(args)
				err := corecmd.ExecuteForTest(root)
				ok := scenario == "success" || scenario == "literal" || scenario == "dry-run" || scenario == "body-alias"
				if (err == nil) != ok {
					t.Fatalf("err=%v output=%s", err, out.String())
				}
				writes := 0
				for _, call := range fake.calls {
					if call.tool == "send_personal_message" {
						writes++
						var c map[string]string
						if e := json.Unmarshal([]byte(fmt.Sprint(call.args["content"])), &c); e != nil {
							t.Fatal(e)
						}
						expected := body
						if scenario == "literal" {
							expected = "-"
						}
						actual := c["content"]
						if actual != expected {
							t.Fatalf("body=%q expected=%q", actual, expected)
						}
					}
				}
				if scenario == "success" || scenario == "literal" || scenario == "body-alias" {
					if writes != 1 || !strings.Contains(out.String(), `"delivered"`) {
						t.Fatalf("writes=%d output=%s", writes, out.String())
					}
				} else if writes != 0 {
					t.Fatal("unexpected write")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageChatEmployeeBindingAtPublicLeaf(t *testing.T) {
	for _, scenario := range []string{"reply", "operator", "stale", "wrong-operator", "missing-context-fields"} {
		t.Run(scenario, func(t *testing.T) {
			profile := "corp:employee"
			old := auth.RuntimeProfile()
			auth.SetRuntimeProfile(profile)
			t.Cleanup(func() { auth.SetRuntimeProfile(old) })
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			if err := os.MkdirAll(filepath.Join(dir, "digital-employees"), 0700); err != nil {
				t.Fatal(err)
			}
			revision := 7
			if scenario == "stale" {
				revision = 8
			}
			binding := map[string]any{"schemaVersion": 1, "agentUuid": "employee", "dwsProfile": profile, "operatorOpenDingTalkId": fixtureCurrentDOpenID, "channel": "dsh", "bindingRevision": revision, "bindingState": "bound", "desiredState": "running"}
			raw, _ := json.Marshal(binding)
			sum := sha256.Sum256([]byte(profile))
			if err := os.WriteFile(filepath.Join(dir, "digital-employees", fmt.Sprintf("%x.json", sum)), raw, 0600); err != nil {
				t.Fatal(err)
			}
			fake := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"D-sender"}]}`, "chat/send_personal_message": `{"openMessageId":"sent","openConversationId":"cid","sendStatus":"SUCCESS"}`}}
			helpers.InitDepsForTest(t, fake)
			root := newPlatformCoverageRoot()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetIn(strings.NewReader("正文"))
			metadata := `{"agentUuid":"employee","channel":"dsh","bindingRevision":7}`
			if scenario == "missing-context-fields" {
				metadata = `{}`
			}
			args := []string{"chat", "+messages-reply", "--group", "cid", "--message-id", "msg", "--content", "-"}
			if scenario == "operator" || scenario == "wrong-operator" {
				target := fixtureCurrentDOpenID
				if scenario == "wrong-operator" {
					target = "D-invalid-user"
				}
				args = []string{"chat", "+messages-send", "--open-dingtalk-id", target, "--markdown", "-"}
			}
			args = append(args, "--body-stdin", "--wait-delivery", "--employee-context", metadata, "--idempotency-key", "key", "--yes")
			root.SetArgs(args)
			err := corecmd.ExecuteForTest(root)
			if (err == nil) != (scenario == "reply" || scenario == "operator") {
				t.Fatal(err)
			}
			if err != nil && len(fake.calls) != 0 {
				t.Fatal("binding failure reached MCP", fake.calls)
			}
		})
	}
}
