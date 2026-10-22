package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEmployeeChatContext(t *testing.T) {
	for _, scenario := range []string{"reply", "operator", "no-context", "bad-json", "unknown-field", "extra-json", "oversize", "missing-id", "missing-stdin", "missing-wait", "missing-key", "missing-profile", "load-error", "wrong-profile", "wrong-agent", "wrong-channel", "stale", "unbound", "stopped", "wrong-operator", "missing-message", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			old := auth.RuntimeProfile()
			auth.SetRuntimeProfile("corp:employee")
			t.Cleanup(func() { auth.SetRuntimeProfile(old) })
			b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "employee", DWSProfile: "corp:employee", OperatorOpenDingTalkID: "operator", Channel: "dsh", BindingRevision: 7, BindingState: "bound", DesiredState: "running"}
			var loadErr error
			raw := `{"agentUuid":"employee","channel":"dsh","bindingRevision":7}`
			op, key, ref, target := "reply", "key", "message", "operator"
			body, wait := true, true
			switch scenario {
			case "operator":
				op = "operator-private"
			case "no-context":
				raw = ""
			case "bad-json":
				raw = "{"
			case "unknown-field":
				raw = `{"agentUuid":"employee","channel":"dsh","bindingRevision":7,"extra":true}`
			case "extra-json":
				raw += `{}`
			case "oversize":
				raw = strings.Repeat("x", 4097)
			case "missing-id":
				raw = `{"channel":"dsh"}`
			case "missing-stdin":
				body = false
			case "missing-wait":
				wait = false
			case "missing-key":
				key = ""
			case "missing-profile":
				auth.SetRuntimeProfile("")
			case "load-error":
				loadErr = errors.New("fixture")
			case "wrong-profile":
				b.DWSProfile = "other"
			case "wrong-agent":
				b.AgentUUID = "other"
			case "wrong-channel":
				b.Channel = "codex"
			case "stale":
				b.BindingRevision++
			case "unbound":
				b.BindingState = "unbound"
			case "stopped":
				b.DesiredState = "stopped"
			case "wrong-operator":
				op = "operator-private"
				target = "allowed-user"
			case "missing-message":
				ref = ""
			case "unsupported":
				op = "other"
			}
			testseam.Swap(t, &deapChannelLoadBinding, func(string, string) (digitalEmployeeBinding, error) { return b, loadErr })
			cmd := &cobra.Command{}
			cmd.Flags().String("employee-context", raw, "")
			cmd.Flags().Bool("body-stdin", body, "")
			cmd.Flags().Bool("wait-delivery", wait, "")
			err := ValidateEmployeeChatContext(cmd, op, "conversation", ref, target, key)
			wantOK := scenario == "reply" || scenario == "operator" || scenario == "no-context"
			if (err == nil) != wantOK {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageChatDeliveryUnifiedReceipt(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"im/query_message_send_status": {`{"openMessageId":"sent","openConvThreadId":"conversation","sendStatus":"SUCCESS"}`}}}
	InitDepsForTest(t, caller)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := WriteChatDelivery(cmd, map[string]any{"openTaskId": "task"}, "", "key"); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result["openMessageId"] != "sent" || result["idempotencyKey"] != "key" {
		t.Fatalf("%s err=%v", out.String(), err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "query_message_send_status" {
		t.Fatal(caller.calls)
	}
	if err := WriteChatDelivery(cmd, map[string]any{}, "", "key"); err == nil {
		t.Fatal("missing receipt accepted")
	}
}
