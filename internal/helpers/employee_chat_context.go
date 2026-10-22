// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// EmployeeChatContext 只携带非敏感绑定元数据，正文始终经 stdin 传入。
// 它是发送范围约束，不替代 chat 自身的用户确认。
type EmployeeChatContext struct {
	AgentUUID       string `json:"agentUuid"`
	Channel         string `json:"channel"`
	BindingRevision uint64 `json:"bindingRevision"`
}

// ValidateEmployeeChatContext 在通用 chat 写入前重新读取绑定，禁止过期实例和非主管目标。
func ValidateEmployeeChatContext(cmd *cobra.Command, operation, conversation, reference, recipient, key string) error {
	raw := devAppStringFlag(cmd, "employee-context")
	if raw == "" {
		return nil
	}
	var in EmployeeChatContext
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 4096 || decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validMachineString(in.AgentUUID) || !validMachineString(in.Channel) {
		return fmt.Errorf("invalid employee chat context")
	}
	bodyStdin, _ := cmd.Flags().GetBool("body-stdin")
	waitDelivery, _ := cmd.Flags().GetBool("wait-delivery")
	if !bodyStdin || !waitDelivery || !validMachineString(key) {
		return fmt.Errorf("employee chat requires --body-stdin, --wait-delivery and an idempotency key")
	}
	profile := strings.TrimSpace(auth.RuntimeProfile())
	if profile == "" {
		return fmt.Errorf("employee chat requires an explicit digital employee --profile")
	}
	b, err := deapChannelLoadBinding(deapConnectConfigDir(), profile)
	if err != nil || b.DWSProfile != profile || b.AgentUUID != in.AgentUUID || bindingChannel(b) != in.Channel || b.BindingRevision != in.BindingRevision || employeeBindingState(b) != "bound" || employeeDesiredState(b) != "running" {
		return fmt.Errorf("employee chat binding is no longer authorized")
	}
	switch operation {
	case "reply":
		if !validMachineString(conversation) || !validMachineString(reference) {
			return fmt.Errorf("employee reply requires an explicit conversation and reference message")
		}
	case "operator-private":
		if !validMachineString(recipient) || b.OperatorOpenDingTalkID != recipient {
			return fmt.Errorf("employee chat target does not match the operator fixed by connect")
		}
	default:
		return fmt.Errorf("unsupported employee chat operation")
	}
	return nil
}

// WriteChatDelivery 保持通用 chat 与兼容 Channel 的机器回执一致。
func WriteChatDelivery(cmd *cobra.Command, result map[string]any, conversation, key string) error {
	delivery, err := ResolveChatDelivery(cmd.Context(), result, conversation, key)
	if err != nil {
		return err
	}
	if output.UsesUnifiedResult(cmd) {
		return output.StoreResult(cmd.Context(), output.Success(delivery))
	}
	return output.WriteCommandPayload(cmd, delivery, output.FormatJSON)
}
