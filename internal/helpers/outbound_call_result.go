// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/outboundcall"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

var outboundCallQuoteScopeValue = driveLatestScopeValue

func outboundCallResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if corecmd.BoolFlag(cmd, "dry-run") || deps.Caller.DryRun() {
		return output.Success(map[string]any{"tool": tool, "arguments": args, "executed": false}, output.WithDryRun()), nil
	}
	result, err := deps.Caller.CallTool(cmd.Context(), outboundCallServer, tool, args)
	if err != nil {
		if tool == outboundCallCreateTool && outboundCallSubmissionUncertain(err) {
			return outboundCallUnknown(), nil
		}
		var classified *apperrors.Error
		if errors.As(err, &classified) && (classified.Category != apperrors.CategoryAPI || (classified.ExecutionStarted != nil && !*classified.ExecutionStarted)) {
			return nil, err
		}
		return nil, WrapErrorWithOperation(err, tool)
	}
	var text string
	if result != nil {
		for _, block := range result.Content {
			if block.Type == "text" {
				text = block.Text
				break
			}
		}
	}
	receipt, err := outboundcall.ParseReceipt(text)
	if err != nil {
		if tool == outboundCallCreateTool {
			return outboundCallUnknown(), nil
		}
		return output.Failure(&output.ErrorInfo{Type: "api", Subtype: "invalid_response", Message: err.Error()}), nil
	}
	if receipt.Failed {
		message := receipt.Message
		if message == "" {
			message = "外呼接口明确返回失败"
		}
		return output.Failure(&output.ErrorInfo{Type: "api", Subtype: "upstream_failure", Message: message, UpstreamCode: receipt.Code, Hint: "检查服务端权限及任务状态后再决定后续操作。"}), nil
	}
	if tool == outboundCallCreateTool && receipt.TaskID != "" {
		taskIDArg, inline := outboundCallQuoteScopeValue(receipt.TaskID)
		if !inline {
			taskIDArg = "TASK_ID"
		}
		return bindRecoveryProfile(cmd, output.Pending(receipt.Data, &output.OperationInfo{
			ID: receipt.TaskID, State: "accepted",
			NextCommand: "dws outbound-call detail --task-id " + taskIDArg,
		}))
	}
	return output.Success(receipt.Data), nil
}

func outboundCallSubmissionUncertain(err error) bool {
	var classified *apperrors.Error
	if errors.As(err, &classified) {
		if classified.ExecutionStarted != nil && !*classified.ExecutionStarted {
			return false
		}
		if classified.Category == apperrors.CategoryAuth || classified.Category == apperrors.CategoryValidation {
			return false
		}
	}
	var callErr *transport.CallError
	if errors.As(err, &callErr) {
		switch callErr.Stage {
		case transport.CallStageRequest:
			return true
		case transport.CallStageHTTP:
			return callErr.HTTPStatus == http.StatusRequestTimeout || callErr.HTTPStatus >= http.StatusInternalServerError
		case transport.CallStageJSONRPC:
			// Protocol/parameter rejections precede tool execution. Internal and
			// server errors do not prove that a non-idempotent call was rejected.
			switch callErr.RPCCode {
			case -32700, -32600, -32601, -32602:
				return false
			default:
				return true
			}
		}
	}
	if classified != nil && classified.Operation == "tools/call" {
		// These transport-owned reasons are emitted only while reading or
		// decoding a response; their discovery category does not mean unsent.
		switch classified.Reason {
		case "tools_call_response_read_failed", "tools_call_response_too_large", "tools_call_invalid_response", "tools_call_empty_result", "tools_call_result_decode_failed":
			return true
		}
	}
	var networkError net.Error
	return (classified == nil || classified.Category == apperrors.CategoryAPI) &&
		(errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &networkError))
}

func outboundCallUnknown() output.CommandResult {
	return output.Failure(&output.ErrorInfo{
		Type: "api", Subtype: "submission_unknown", Message: "外呼创建调用未能确认提交结果",
		Hint: "不要自动重试创建，以免重复呼叫。请先核实任务记录；如已取得 taskId，使用 dws outbound-call detail 查询。",
	})
}
