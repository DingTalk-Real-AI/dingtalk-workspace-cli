// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package live defines strict task-oriented Shortcut candidates for DingTalk
// live broadcasts. The current list capability remains fail-closed because the
// downstream response can contradict its own completion and total fields.
package live

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

const liveListGapReason = "Exact bounded live probes returned total greater than zero together with hasFinish=true and an empty liveDetailModelList; the response cannot prove either a complete list or a legitimate empty result."

var ListMyLives = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "live", Command: "+list-my-lives", Product: "live",
	Description: "查看当前用户发起的直播列表与基础统计",
	Intent:      "需要查看本人发起的直播、状态或观看统计时使用；当前因下游列表数量与完成状态矛盾保持不可用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Description: "查看当前用户发起的直播列表与基础统计",
		Identity: contract.ToolIdentitySpec{
			ProductID: "live", Name: "shortcut_list_my_lives", CanonicalPath: "live.shortcut_list_my_lives",
			CLIPath: "live +list-my-lives", PrimaryCLIPath: "live +list-my-lives",
		},
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceUnavailable, Reason: liveListGapReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查看当前用户发起的直播列表与基础统计",
			UseWhen:      []string{"需要查看本人发起的直播、状态或观看统计时使用；当前因下游列表数量与完成状态矛盾保持不可用。"},
			AvoidWhen: []string{
				"需要创建、开播、结束、观看或控制直播时不要使用；当前 DWS 原子面没有这些能力",
				"下游列表总数、完成状态和项目数组未自洽前不要把空数组当作没有直播",
			},
			Examples: []string{"dws live +list-my-lives --format json"},
		},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			DataSchema: json.RawMessage(`{"type":"object","description":"严格校验且完整的当前用户直播列表","properties":{"count":{"type":"integer","description":"与下游 total 完全一致的直播数量"},"lives":{"type":"array","description":"带稳定直播身份的完整列表","items":{"type":"object","description":"带稳定直播身份的直播记录","additionalProperties":true}},"complete":{"type":"boolean","description":"下游明确报告且可由数量一致性证明的完结状态"}},"required":["count","lives","complete"],"additionalProperties":false}`),
		},
	},
	Execute: func(*shortcut.RuntimeContext) error {
		return responsecheck.Error("live/get_my_lives", "capability_unavailable", liveListGapReason)
	},
}

// projectLiveList is retained as the reviewed activation gate. It accepts only
// an explicit complete, unpaginated list whose total exactly equals its items.
func projectLiveList(data map[string]any) (map[string]any, error) {
	operation := "live/get_my_lives"
	envelope, err := responsecheck.RequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"errorCode", "errorMsg", "errorMessage", "error"} {
		if liveFailureValue(envelope[key]) {
			return nil, responsecheck.Error(operation, "conflicting_failure_evidence", "success=true 响应同时携带失败字段 "+key)
		}
	}
	result, err := responsecheck.RequireObjectResult(envelope, operation)
	if err != nil {
		return nil, err
	}
	total, ok := liveNonNegativeInt(result["total"])
	if !ok {
		return nil, responsecheck.Error(operation, "malformed_total", "响应 result.total 必须是非负整数")
	}
	complete, ok := result["hasFinish"].(bool)
	if !ok {
		return nil, responsecheck.Error(operation, "malformed_completion", "响应 result.hasFinish 必须是布尔值")
	}
	if !complete {
		return nil, responsecheck.Error(operation, "unpageable_incomplete_result", "响应未完结但接口没有 cursor 或下一页参数，不能声称列表成功")
	}
	items, err := responsecheck.RequireObjectCollection(envelope, operation, "result.liveDetailModelList")
	if err != nil {
		return nil, err
	}
	if total != len(items) {
		return nil, responsecheck.Error(operation, "inconsistent_total", fmt.Sprintf("响应 total=%d、项目数=%d 且 hasFinish=true，不能证明完整列表", total, len(items)))
	}
	for index, item := range items {
		if liveStableID(item) == "" {
			return nil, responsecheck.Error(operation, "missing_stable_identity", fmt.Sprintf("直播结果第 %d 项缺少稳定直播 ID", index))
		}
	}
	return map[string]any{"count": total, "lives": items, "complete": true}, nil
}

func liveFailureValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && trimmed != "0" && !strings.EqualFold(trimmed, "success")
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case bool:
		return typed
	case map[string]any:
		return len(typed) != 0
	case []any:
		return len(typed) != 0
	default:
		return true
	}
}

func liveNonNegativeInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed < 0 || typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case int:
		return typed, typed >= 0
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil && parsed >= 0
	default:
		return 0, false
	}
}

func liveStableID(item map[string]any) string {
	for _, key := range []string{"liveId", "liveUuid", "uuid", "id"} {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func init() {
	shortcut.Register(ListMyLives)
}
