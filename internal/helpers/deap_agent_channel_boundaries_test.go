// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageEmployeeChannelFailureBoundaries(t *testing.T) {
	for _, operation := range []string{"reply", "operator"} {
		for _, scenario := range []string{"malformed", "invalid-payload", "binding-mismatch", "lookup-failed", "sender-missing", "send-failed", "receipt-failed", "success-alias"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				installEmployeeReplyBinding(t)
				caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
					"im/list_messages_by_ids":    {`{"sender_open_dingtalk_id":"sender"}`},
					"chat/send_personal_message": {`{"openMessageId":"delivered","status":"success"}`},
				}}
				InitDepsForTest(t, caller)
				cmd := newDeapChannelReplyCommand()
				var input any = digitalEmployeeReplyInput{SchemaVersion: 1, ProtocolVersion: 1, AgentUUID: "agent-1", EventID: "event", ConversationID: "conversation", ReferenceMessageID: "original", Text: "private body", IdempotencyKey: "unique"}
				if operation == "operator" {
					cmd = newDeapChannelOperatorPrivateCommand()
					input = digitalEmployeeOperatorInput{SchemaVersion: 1, ProtocolVersion: 1, AgentUUID: "agent-1", OperatorOpenDingTalkID: "operator-open", Text: "private body", IdempotencyKey: "unique"}
				}
				cmd.SetContext(context.Background())
				cmd.SetOut(&bytes.Buffer{})
				_ = cmd.Flags().Set("channel", "dsh")
				_ = cmd.Flags().Set("stdin", "true")
				raw, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "malformed":
					raw = []byte("{")
				case "invalid-payload":
					raw = []byte(`{}`)
				case "binding-mismatch":
					auth.SetRuntimeProfile("")
				case "lookup-failed":
					if operation == "reply" {
						delete(caller.responses, "im/list_messages_by_ids")
					} else {
						raw = bytes.ReplaceAll(raw, []byte("operator-open"), []byte("another-operator"))
					}
				case "sender-missing":
					if operation == "reply" {
						caller.responses["im/list_messages_by_ids"] = []string{`{}`}
					} else {
						raw = bytes.ReplaceAll(raw, []byte("operator-open"), []byte("another-operator"))
					}
				case "send-failed":
					delete(caller.responses, "chat/send_personal_message")
				case "receipt-failed":
					caller.responses["chat/send_personal_message"] = []string{`{}`}
				}
				cmd.SetIn(bytes.NewReader(raw))
				err = cmd.RunE(cmd, nil)
				if (err == nil) != (scenario == "success-alias") {
					t.Fatalf("err=%v", err)
				}
				if err != nil && strings.Contains(err.Error(), "private body") {
					t.Fatal("failure exposed message body")
				}
			})
		}
	}
}

type employeeOrdinaryBlocksCaller struct {
	digitalEmployeeProtocolCaller
	blocks []edition.ContentBlock
}

func (c *employeeOrdinaryBlocksCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{Content: c.blocks}, nil
}

func TestCrossPlatformCoverageEmployeePrivateResultValidation(t *testing.T) {
	for _, blocks := range [][]edition.ContentBlock{
		{{Type: "image"}, {Type: "text", Text: " "}}, {{Type: "text", Text: "{"}}, {{Type: "text", Text: `{"success":false}`}},
	} {
		InitDepsForTest(t, &employeeOrdinaryBlocksCaller{blocks: blocks})
		if _, err := callPrivateMCPJSON(context.Background(), "test", "fixture", nil); err == nil {
			t.Fatal("untrusted result accepted")
		}
	}
	cmd := newDeapChannelReplyCommand()
	cmd.SetIn(employeeReadFailure{})
	if err := decodeBoundedDigitalEmployeeStdin(cmd, new(digitalEmployeeReplyInput)); err == nil {
		t.Fatal("read failure accepted")
	}
	if _, err := resolveDigitalEmployeeDelivery(nil, map[string]any{"openMessageId": "message"}, "", "key"); err != nil {
		t.Fatal(err)
	}
}

type employeeReadFailure struct{}

func (employeeReadFailure) Read([]byte) (int, error) { return 0, errors.New("stdin unavailable") }

func TestCrossPlatformCoverageEmployeeSupervisorSnapshotFailures(t *testing.T) {
	for _, scenario := range []string{"no-profile", "load-token", "missing-identity", "refresh", "refresh-empty", "refresh-other"} {
		t.Run(scenario, func(t *testing.T) {
			setupServerBindingSupervisor(t)
			switch scenario {
			case "no-profile":
				testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) { return &auth.ProfilesConfig{}, nil })
			case "load-token":
				testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) { return nil, errors.New("cannot read token") })
			case "missing-identity":
				testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) { return &auth.TokenData{}, nil })
			default:
				testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
					if scenario == "refresh" {
						return nil, errors.New("refresh failed")
					}
					if scenario == "refresh-empty" {
						return nil, nil
					}
					return &auth.TokenData{CorpID: "other", UserID: "other", AccessToken: "fixture-token"}, nil
				})
			}
			if _, _, err := currentSupervisorProfile(context.Background(), t.TempDir()); err == nil {
				t.Fatal("unverified supervisor snapshot accepted")
			}
		})
	}
}
