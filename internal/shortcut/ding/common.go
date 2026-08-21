// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package ding

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	dingCompositeReason          = "Reviewed DING Shortcut composite: the executable CLI owns strict business-success validation, exact collection paths, stable DING identity checks, truthful pagination, and unified output projection."
	dingCompatibilityWriteReason = "Historical DING CLI write compatibility: the command remains executable behind user confirmation, but it is excluded from Agent public discovery because receiver identity and recall terminal-state verification are incomplete."
	dingWriteUnavailableReason   = "DING mutation is unavailable to Agents until the downstream exposes stable receiver identities and a queryable recall terminal state; isolated self-fixtures already prove stable DING receipts but cannot prove those two facts."
)

type dingPageEvidence struct {
	HasMore bool
	Next    string
}

func dingReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func dingWriteSafety(destructive bool) contract.SafetySpec {
	effect, risk := "write", "medium"
	if destructive {
		effect, risk = "destructive", "high"
	}
	return contract.SafetySpec{Effect: effect, Risk: risk, Confirmation: "user_required", Idempotency: "unknown"}
}

func dingListResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"严格验证的 DING 消息页","properties":{"count":{"type":"integer","description":"当前页有效 DING 消息数量"},"messages":{"type":"array","description":"严格验证并投影后的 DING 消息","items":{"type":"object","description":"一条具有稳定 openDingId 的 DING 消息","properties":{"openDingId":{"type":"string","description":"稳定 DING 消息 ID"},"content":{"type":"string","description":"DING 消息内容"},"sourceMessageId":{"type":"string","description":"作为 DING 来源的聊天消息 ID"},"sendTime":{"type":"string","description":"服务端返回的发送时间"},"senderNick":{"type":"string","description":"发送人显示名"}},"required":["openDingId"],"additionalProperties":false}},"complete":{"type":"boolean","description":"当前页是否具有服务端分页终态证据"}},"required":["count","messages","complete"],"additionalProperties":false}`),
		SensitivePaths: []string{"messages.content", "messages.senderNick"},
	}
}

func dingReceiverResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"指定 DING 的严格接收状态","properties":{"count":{"type":"integer","description":"经验证的接收状态数量"},"receivers":{"type":"array","description":"身份匹配的 DING 接收状态","items":{"type":"object","description":"一名接收人的 DING 确认状态","properties":{"openDingId":{"type":"string","description":"与请求一致的稳定 DING ID"},"confirmedStatus":{"type":"integer","description":"服务端 DING 确认状态码"},"receiverNick":{"type":"string","description":"接收人显示名"}},"required":["openDingId","confirmedStatus","receiverNick"],"additionalProperties":false}}},"required":["count","receivers"],"additionalProperties":false}`),
		SensitivePaths: []string{"receivers.receiverNick"},
	}
}

func dingPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func dingContract(command, description, intent string, available bool, result *contract.ResultSpec, pagination *contract.PaginationSpec, params []contract.ParamDecl, examples ...string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "ding " + command
	availability, reason := contract.InterfaceAvailable, dingCompositeReason
	if !available {
		availability, reason = contract.InterfaceUnavailable, dingWriteUnavailableReason
	}
	return corecmd.ContractDecl{
		Description: description,
		Result:      result,
		Pagination:  pagination,
		Parameters:  params,
		Identity: contract.ToolIdentitySpec{
			ProductID: "ding", Name: name, CanonicalPath: "ding." + name,
			CLIPath: cliPath, PrimaryCLIPath: cliPath,
		},
		Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: availability, Reason: reason},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen:    []string{"普通聊天消息使用 chat；写能力在缺少可逆真实 fixture、精确读回或清理证据时不要执行"},
			Examples:     examples,
		},
	}
}

func dingResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func dingRequireResult(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, dingResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明 DING 调用成功或结果确实为空")
	}
	success, present := data["success"]
	if !present {
		return nil, dingResponseError(operation, "missing_success", "DING 响应缺少 success 业务状态")
	}
	succeeded, ok := success.(bool)
	if !ok {
		return nil, dingResponseError(operation, "invalid_success_type", fmt.Sprintf("DING 响应 success 应为布尔值，实际为 %T", success))
	}
	if !succeeded {
		message := "DING 服务明确返回失败"
		for _, key := range []string{"errorMsg", "errorMessage", "message"} {
			if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
				message = strings.TrimSpace(value)
				break
			}
		}
		return nil, dingResponseError(operation, "remote_failure", message)
	}
	result, ok := data["result"].(map[string]any)
	if !ok || result == nil {
		return nil, dingResponseError(operation, "missing_result", "DING 成功响应缺少 result 对象")
	}
	return result, nil
}

func dingRequireObjectCollection(result map[string]any, operation, field string) ([]map[string]any, error) {
	raw, present := result[field]
	if !present {
		return nil, dingResponseError(operation, "missing_collection", fmt.Sprintf("DING 成功响应缺少 result.%s 数组", field))
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, dingResponseError(operation, "malformed_collection", fmt.Sprintf("DING result.%s 应为数组，实际为 %T", field, raw))
	}
	items := make([]map[string]any, 0, len(values))
	for index, value := range values {
		item, ok := value.(map[string]any)
		if !ok || len(item) == 0 {
			return nil, dingResponseError(operation, "malformed_item", fmt.Sprintf("DING result.%s[%d] 不是非空对象", field, index))
		}
		items = append(items, item)
	}
	return items, nil
}

func dingRequiredString(item map[string]any, operation, field string, index int) (string, error) {
	value, ok := item[field].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", dingResponseError(operation, "missing_item_identity", fmt.Sprintf("DING 结果第 %d 项缺少非空 %s", index, field))
	}
	return value, nil
}

func dingOptionalString(item map[string]any, field string) (string, error) {
	value, present := item[field]
	if !present || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("字段 %s 应为字符串，实际为 %T", field, value)
	}
	return strings.TrimSpace(text), nil
}

func dingInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > math.MaxInt64 || typed < math.MinInt64 {
			return 0, false
		}
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func dingProjectMessages(data map[string]any, operation string) ([]map[string]any, dingPageEvidence, error) {
	result, err := dingRequireResult(data, operation)
	if err != nil {
		return nil, dingPageEvidence{}, err
	}
	items, err := dingRequireObjectCollection(result, operation, "dingMessages")
	if err != nil {
		return nil, dingPageEvidence{}, err
	}
	messages := make([]map[string]any, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		id, identityErr := dingRequiredString(item, operation, "openDingId", index)
		if identityErr != nil {
			return nil, dingPageEvidence{}, identityErr
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, dingPageEvidence{}, dingResponseError(operation, "duplicate_item_identity", "当前 DING 页包含重复 openDingId")
		}
		seen[id] = struct{}{}
		projected := map[string]any{"openDingId": id}
		for source, target := range map[string]string{
			"dingContent": "content", "dingSourceOpenMessageId": "sourceMessageId",
			"sendTime": "sendTime", "senderNick": "senderNick",
		} {
			value, valueErr := dingOptionalString(item, source)
			if valueErr != nil {
				return nil, dingPageEvidence{}, dingResponseError(operation, "malformed_item", fmt.Sprintf("DING 结果第 %d 项%s", index, valueErr.Error()))
			}
			if value != "" {
				projected[target] = value
			}
		}
		messages = append(messages, projected)
	}
	hasMoreValue, present := result["hasMore"]
	if !present {
		return nil, dingPageEvidence{}, dingResponseError(operation, "missing_pagination", "DING 列表缺少 result.hasMore，不能宣称当前页完整")
	}
	hasMore, ok := hasMoreValue.(bool)
	if !ok {
		return nil, dingPageEvidence{}, dingResponseError(operation, "malformed_pagination", "DING result.hasMore 应为布尔值")
	}
	page := dingPageEvidence{HasMore: hasMore}
	if !hasMore {
		if next, exists := result["nextCursor"]; exists && next != nil {
			if numeric, valid := dingInteger(next); !valid || numeric != 0 {
				return nil, dingPageEvidence{}, dingResponseError(operation, "conflicting_pagination", "hasMore=false 但 nextCursor 仍表示后续页")
			}
		}
		return messages, page, nil
	}
	if len(messages) == 0 {
		return nil, dingPageEvidence{}, dingResponseError(operation, "empty_page_with_continuation", "DING 空页仍声明 hasMore=true，无法证明游标可安全推进")
	}
	next, valid := dingInteger(result["nextCursor"])
	if !valid || next <= 0 {
		return nil, dingPageEvidence{}, dingResponseError(operation, "missing_next_cursor", "hasMore=true 时必须返回正整数 nextCursor")
	}
	page.Next = strconv.FormatInt(next, 10)
	return messages, page, nil
}

func dingProjectReceivers(data map[string]any, operation, expectedID string) ([]map[string]any, error) {
	result, err := dingRequireResult(data, operation)
	if err != nil {
		return nil, err
	}
	items, err := dingRequireObjectCollection(result, operation, "receivers")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, dingResponseError(operation, "empty_receiver_status", "DING 接收状态显式为空，无法证明目标 DING 的接收状态")
	}
	receivers := make([]map[string]any, 0, len(items))
	for index, item := range items {
		id, identityErr := dingRequiredString(item, operation, "openDingId", index)
		if identityErr != nil {
			return nil, identityErr
		}
		if id != expectedID {
			return nil, dingResponseError(operation, "identity_mismatch", "DING 接收状态的 openDingId 与请求目标不一致")
		}
		status, ok := dingInteger(item["confirmedStatus"])
		if !ok {
			return nil, dingResponseError(operation, "malformed_item", fmt.Sprintf("DING 接收状态第 %d 项 confirmedStatus 不是整数", index))
		}
		nick, nickErr := dingRequiredString(item, operation, "receiverNick", index)
		if nickErr != nil {
			return nil, nickErr
		}
		receivers = append(receivers, map[string]any{"openDingId": id, "confirmedStatus": status, "receiverNick": nick})
	}
	return receivers, nil
}

func outputDingPage(rt *shortcut.RuntimeContext, messages []map[string]any, page dingPageEvidence) error {
	payload := map[string]any{"count": len(messages), "messages": messages, "complete": !page.HasMore}
	if !output.UsesUnifiedResult(rt.Command()) {
		if page.HasMore {
			payload["nextCursor"] = page.Next
		}
		return rt.Output(payload)
	}
	pagination, err := output.NewPagination(!page.HasMore, page.Next)
	if err != nil {
		return dingResponseError("im/list_ding_messages", "invalid_pagination", err.Error())
	}
	meta := &output.Meta{Count: output.NewCount(len(messages)), Pagination: pagination}
	return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
}

func dingUnavailable(operation string) error {
	return apperrors.NewDiscovery("该 DING 写 Shortcut 虽有稳定写回执，但下游没有稳定接收人身份和可查询撤回终态，当前不可执行",
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("shortcut_registry"),
		apperrors.WithFailureStage("capability_gate"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithReason("write_fixture_unavailable"),
	)
}
