// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type employeeReceiptCaller struct {
	*digitalEmployeeProtocolCaller
	receiptErrors []error
}

func TestCrossPlatformCoverageEmployeeReceiptRetryBoundaryAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, server, tool string
		err                error
		wantRetry          bool
	}{
		{"exact receipt", "im", "query_message_send_status", receiptNotVisibleError(), true},
		{"wrapped receipt", "im", "query_message_send_status", fmt.Errorf("private-wrapper: %w", receiptNotVisibleError()), true},
		{"structured detail", "im", "query_message_send_status", apperrors.NewAPI("private-diagnostics", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR", TechnicalDetail: "消息不存在"})), true},
		{"conflicting detail", "im", "query_message_send_status", apperrors.NewAPI("消息不存在", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR", TechnicalDetail: "openTaskId 参数错误"})), false},
		{"send must not retry", "chat", "send_personal_message", receiptNotVisibleError(), false},
		{"lookup must not retry", "im", "list_messages_by_ids", receiptNotVisibleError(), false},
		{"auth must not retry", "im", "query_message_send_status", apperrors.NewInternal("secret", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "TOKEN_VERIFIED_FAILED", TechnicalDetail: "消息不存在"})), false},
		{"different parameter error", "im", "query_message_send_status", apperrors.NewInternal("secret", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR", TechnicalDetail: "openTaskId 参数错误"})), false},
		{"unstructured lookalike", "im", "query_message_send_status", errors.New("PARAM_ERROR 消息不存在 secret"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_DUMP_RAW", "1")
			caller := &receiptErrorOnlyCaller{err: tc.err}
			InitDepsForTest(t, caller)
			_, err := callPrivateMCPJSON(context.Background(), tc.server, tc.tool, map[string]any{"openTaskId": "task-1"})
			if err == nil || errors.Is(err, errEmployeeReceiptNotVisible) != tc.wantRetry {
				t.Fatalf("classification = %v", err)
			}
			for _, raw := range []string{"secret", "private-wrapper", "private-diagnostics", "消息不存在"} {
				if strings.Contains(err.Error(), raw) {
					t.Fatal("raw error leaked")
				}
			}
			var typed *apperrors.Error
			if errors.As(err, &typed) {
				t.Fatal("raw diagnostics retained through error chain")
			}
		})
	}
}

type receiptErrorOnlyCaller struct {
	digitalEmployeeProtocolCaller
	err error
}

func (c *receiptErrorOnlyCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return nil, c.err
}

func TestCrossPlatformCoverageEmployeeReceiptNotVisibleExhaustionIsUnknownAndNeverResends(t *testing.T) {
	caller := &employeeReceiptCaller{digitalEmployeeProtocolCaller: &digitalEmployeeProtocolCaller{}}
	for i := 0; i < digitalEmployeeReceiptAttempts; i++ {
		caller.receiptErrors = append(caller.receiptErrors, receiptNotVisibleError())
	}
	InitDepsForTest(t, caller)
	waits := 0
	testseam.Swap(t, &deapChannelReceiptWait, func(context.Context, time.Duration) error { waits++; return nil })
	_, err := resolveDigitalEmployeeDelivery(context.Background(), map[string]any{"openTaskId": "task-1"}, "conversation-1", "idem-1")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "delivery_unknown" || typed.Retryable || !typed.RetryableSet {
		t.Fatalf("exhaustion = %v", err)
	}
	if len(caller.calls) != digitalEmployeeReceiptAttempts || waits != digitalEmployeeReceiptAttempts-1 {
		t.Fatalf("calls=%d waits=%d", len(caller.calls), waits)
	}
	for _, call := range caller.calls {
		if call.toolName != "query_message_send_status" {
			t.Fatal("receipt polling resent a message")
		}
	}
}

func TestCrossPlatformCoverageEmployeeReceiptPermanentFailureStopsPolling(t *testing.T) {
	caller := &employeeReceiptCaller{digitalEmployeeProtocolCaller: &digitalEmployeeProtocolCaller{}, receiptErrors: []error{errors.New("private-error")}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &deapChannelReceiptWait, func(context.Context, time.Duration) error { t.Fatal("permanent failure retried"); return nil })
	_, err := resolveDigitalEmployeeDelivery(context.Background(), map[string]any{"openTaskId": "task-1"}, "conversation-1", "idem-1")
	if err == nil || len(caller.calls) != 1 || strings.Contains(err.Error(), "private-error") {
		t.Fatalf("failure=%v calls=%d", err, len(caller.calls))
	}
}

func TestCrossPlatformCoverageEmployeeReceiptCancellationStopsQueries(t *testing.T) {
	for _, beforeFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(beforeFirst), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			caller := &employeeReceiptCaller{digitalEmployeeProtocolCaller: &digitalEmployeeProtocolCaller{}, receiptErrors: []error{receiptNotVisibleError()}}
			InitDepsForTest(t, caller)
			if beforeFirst {
				cancel()
			}
			testseam.Swap(t, &deapChannelReceiptWait, func(ctx context.Context, _ time.Duration) error {
				cancel()
				return waitForDigitalEmployeeReceipt(ctx, time.Hour)
			})
			_, err := resolveDigitalEmployeeDelivery(ctx, map[string]any{"openTaskId": "task-1"}, "conversation-1", "idem-1")
			want := 1
			if beforeFirst {
				want = 0
			}
			if !errors.Is(err, context.Canceled) || len(caller.calls) != want {
				t.Fatalf("cancellation=%v calls=%d", err, len(caller.calls))
			}
		})
	}
}

func (c *employeeReceiptCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	if product == "im" && tool == "query_message_send_status" && len(c.receiptErrors) > 0 {
		err := c.receiptErrors[0]
		c.receiptErrors = c.receiptErrors[1:]
		if err != nil {
			c.calls = append(c.calls, deapAgentCall{productID: product, toolName: tool, args: args})
			return nil, err
		}
	}
	return c.digitalEmployeeProtocolCaller.CallTool(ctx, product, tool, args)
}

func receiptNotVisibleError() error {
	// 真实业务错误由 errorMsg 转为 Message；服务端未给 technical_detail。
	return apperrors.NewAPI("消息不存在", apperrors.WithServerDiag(apperrors.ServerDiagnostics{
		ServerErrorCode: "PARAM_ERROR",
	}))
}

func TestCrossPlatformCoverageDingTalkTagChannelReplyRetriesNotVisibleReceiptWithoutResending(t *testing.T) {
	installEmployeeReplyBinding(t)
	t.Setenv("DWS_DUMP_RAW", "1")
	caller := &employeeReceiptCaller{
		digitalEmployeeProtocolCaller: &digitalEmployeeProtocolCaller{responses: map[string][]string{
			"im/list_messages_by_ids":      {`{"result":[{"openMessageId":"message-1","senderOpenDingTalkId":"operator-open"}]}`},
			"chat/send_personal_message":   {`{"result":{"openTaskId":"task-1"}}`},
			"im/query_message_send_status": {`{"result":{"openMessageId":"reply-1","sendStatus":"SUCCESS"}}`},
		}},
		receiptErrors: []error{receiptNotVisibleError()},
	}
	InitDepsForTest(t, caller)
	waits := 0
	testseam.Swap(t, &deapChannelReceiptWait, func(context.Context, time.Duration) error { waits++; return nil })
	leaf, _, err := deapHandler{}.Command(&captureRunner{}).Find([]string{"channel", "reply"})
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"channel": "dsh", "stdin": "true"} {
		if err := leaf.Flags().Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	leaf.SetIn(strings.NewReader(`{"schemaVersion":1,"protocolVersion":1,"agentUuid":"agent-1","eventId":"event-1","sessionId":"session-1","conversationId":"conversation-1","referenceMessageId":"message-1","text":"private-message-body","idempotencyKey":"idem-1"}`))
	var out, stderr bytes.Buffer
	leaf.SetOut(&out)
	leaf.SetErr(&stderr)
	if err := leaf.RunE(leaf, nil); err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data["openMessageId"] != "reply-1" || envelope.Data["deliveryStatus"] != "delivered" {
		t.Fatalf("unexpected result: %v", envelope)
	}
	sends, queries := 0, 0
	for _, call := range caller.calls {
		if call.toolName == "send_personal_message" {
			sends++
		}
		if call.toolName == "query_message_send_status" {
			queries++
			if call.args["openTaskId"] != "task-1" {
				t.Fatal("receipt query changed task")
			}
		}
	}
	if sends != 1 || queries != 2 || waits != 1 {
		t.Fatalf("sends=%d queries=%d waits=%d", sends, queries, waits)
	}
	for _, secret := range []string{"private-message-body", "private server diagnostics"} {
		if strings.Contains(out.String()+stderr.String(), secret) {
			t.Fatal("private response leaked")
		}
	}
}
