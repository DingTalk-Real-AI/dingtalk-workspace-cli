// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// Feed the employee caller's arguments through the atomic command's real parser
// and parameter construction. A string-only argv assertion misses default drift.
func assertEmployeeForwardRequest(t *testing.T, want bool) *int {
	t.Helper()
	calls := new(int)
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		*calls++
		if args[0] != "send-a2ui-card" {
			t.Fatalf("unexpected operation: %v", args)
		}
		content, err := encodeEmployeeCardMessages(messages)
		if err != nil {
			t.Fatal(err)
		}
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"bizId":"fixture-only"}}`}}}
		cliArgs := append([]string{"message"}, args...)
		cliArgs = append(cliArgs, "--content", string(content))
		if err := runNativeCardUpdate(t, caller, cliArgs...); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.server != "im" || caller.tool != "create_and_send_a2ui_card" || caller.args["supportForward"] != want {
			t.Fatalf("actual MCP request supportForward=%v want=%v; tool=%s calls=%d", caller.args["supportForward"], want, caller.tool, caller.calls)
		}
		return nil, errors.New("stop after synthetic create request; no remote write")
	})
	return calls
}

func TestCrossPlatformCoverageEmployeeForwardContract(t *testing.T) {
	for _, group := range []bool{false, true} {
		for _, forward := range []bool{false, true} {
			t.Run(fmt.Sprintf("reply/group=%t/forward=%t", group, forward), func(t *testing.T) {
				r, event := employeeLedgerFixture(t)
				event.SenderID = "DAAAAAAAAAAAiE"
				r.cfg.Options.AllowedUsers = []string{event.SenderID}
				r.cfg.ReplyCardForward = forward
				if group {
					event.Type = "user_im_message_receive_group_all"
					r.cfg.Options.AllowedGroups = []string{event.ConversationID}
				}
				calls := assertEmployeeForwardRequest(t, forward)
				fwd := &employeeProgressFixture{run: func(func(agentProgress)) (string, error) {
					t.Fatal("model execution must not follow a failed synthetic create")
					return "", nil
				}}
				_ = r.processCard(event, &employeeTaskRecord{}, fwd)
				if *calls != 1 {
					t.Fatalf("create calls=%d want=1", *calls)
				}
			})
		}
	}
	t.Run("approval", func(t *testing.T) {
		r, _ := employeeLedgerFixture(t)
		r.cfg.Binding.OperatorOpenDingTalkID = "DAAAAAAAAAAAiE"
		calls := assertEmployeeForwardRequest(t, false)
		if r.requestApproval(context.Background(), "item/commandExecution/requestApproval", json.RawMessage(`{"command":"echo fixture"}`)) {
			t.Fatal("synthetic create failure must not approve an operation")
		}
		if *calls != 1 {
			t.Fatalf("create calls=%d want=1", *calls)
		}
	})
}
