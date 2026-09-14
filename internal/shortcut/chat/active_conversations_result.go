// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func activeConversationsResultSchema() json.RawMessage {
	partial := map[string]any{
		"type":        "object",
		"description": "部分读取成功：一个已验证页面的聚合批次和一个失败页面；不是会话级成功失败计数",
		"properties": map[string]any{
			"total": map[string]any{
				"type": "integer", "const": 2,
				"description": "两个处理单元：已验证页面聚合批次和失败页面；不是页数或会话数",
			},
			"succeeded": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 1,
				"description": "已成功读取并完整校验的所有页面合并为一个去重聚合批次",
				"items":       activeConversationAggregateSchema(true),
			},
			"failed": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 1,
				"description": "终止本次查询的失败页面和原始错误诊断",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type": "string", "pattern": "^page:[1-9][0-9]*$",
							"description": "失败页标识，格式为 page:N；N 是本次查询中从 1 开始的页序号",
						},
						"error": map[string]any{
							"type": "object", "description": "原始错误类型、上游诊断、重试信息及恢复所需的失败页上下文",
							"properties": map[string]any{
								"type": map[string]any{
									"type": "string", "enum": []string{"api", "auth", "validation", "permission", "discovery", "internal"},
									"description": "稳定错误分类；权限或认证错误保留自身类别",
								},
								"message": map[string]any{"type": "string", "description": "原始错误说明"},
								"hint":    map[string]any{"type": "string", "description": "先处理失败原因，再使用原查询参数和失败游标继续读取的建议"},
								"details": map[string]any{
									"type": "object", "description": "保留原始错误详情，并补充失败页序号和输入游标",
									"properties": map[string]any{
										"failedPage":   map[string]any{"type": "integer", "minimum": 1, "description": "本次查询中失败页的序号，从 1 开始"},
										"failedCursor": map[string]any{"type": "string", "minLength": 1, "description": "失败页面请求使用的输入游标，和 meta.pagination.next_token 相同"},
									},
									"required": []string{"failedPage", "failedCursor"}, "additionalProperties": true,
								},
							},
							"required": []string{"type", "message", "hint", "details"}, "additionalProperties": true,
						},
					},
					"required": []string{"id", "error"}, "additionalProperties": false,
				},
			},
			"unknown": map[string]any{
				"type": "array", "maxItems": 0,
				"description": "本命令只读取数据，没有写入终态不明的条目，始终为空数组",
				"items":       map[string]any{"type": "object"},
			},
		},
		"required": []string{"total", "succeeded", "failed", "unknown"}, "additionalProperties": false,
	}
	// This declaration contains only fixed JSON-safe maps, slices and scalar
	// values. NormalizeResultSpec still validates the complete result contract.
	schema, _ := json.Marshal(map[string]any{
		"type":        "object",
		"description": "正常返回时间窗内的去重会话；后续页面失败时保留已验证页面的聚合批次及失败信息",
		"oneOf":       []any{activeConversationAggregateSchema(false), partial},
	})
	return schema
}

func activeConversationAggregateSchema(partial bool) map[string]any {
	properties := map[string]any{
		"start":            map[string]any{"type": "string", "format": "date-time", "description": "整秒精度的有效查询开始时间（包含）；省略 --start 时从有效 end 往前推 24 小时；续页时显式复用该值"},
		"end":              map[string]any{"type": "string", "format": "date-time", "description": "整秒精度的固定查询结束时间（不包含）；默认将本次执行开始时间向下取整到当前秒；续页时保持不变"},
		"rangeSemantics":   map[string]any{"type": "string", "enum": []string{"[start,end)"}, "description": "查询时间范围为左闭右开区间"},
		"count":            map[string]any{"type": "integer", "minimum": 0, "description": "按 openConversationId 去重后的会话数量"},
		"complete":         map[string]any{"type": "boolean", "description": "是否从首页开始并已观察到服务端分页耗尽；--cursor 续页和部分失败批次始终为 false；不保证服务端索引无延迟或一致性快照"},
		"pagesFetched":     map[string]any{"type": "integer", "minimum": 1, "description": "本次成功读取且完整校验的消息分页数量，不包含失败页面或此前执行的页面"},
		"pageSize":         map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "本次底层每页消息数量；续页时使用同一 --limit"},
		"unknownTypeCount": map[string]any{"type": "integer", "minimum": 0, "description": "下层未返回 singleChat 因而类型未知的会话数量"},
		"conversations": map[string]any{
			"type": "array", "description": "按时间窗内最新消息时间倒序排列的去重会话",
			"items": map[string]any{
				"type": "object", "description": "一个在时间窗内有消息的会话",
				"properties": map[string]any{
					"conversationId":    map[string]any{"type": "string", "minLength": 1, "description": "稳定 openConversationId"},
					"name":              map[string]any{"type": "string", "description": "下层返回的会话显示名称；未知时为空字符串"},
					"nameKnown":         map[string]any{"type": "boolean", "description": "下层是否返回了非空会话名称"},
					"type":              map[string]any{"type": "string", "enum": []string{"direct", "group", "unknown"}, "description": "规范化会话类型；缺失 singleChat 时为 unknown"},
					"latestMessageTime": map[string]any{"type": "string", "format": "date-time", "description": "本批次已读取消息中，该会话在查询时间窗内的最大创建时间；保留消息时间的原始精度"},
				},
				"required": []string{"conversationId", "name", "nameKnown", "type", "latestMessageTime"}, "additionalProperties": false,
			},
		},
	}
	required := []string{"start", "end", "rangeSemantics", "count", "complete", "pagesFetched", "pageSize", "unknownTypeCount", "conversations"}
	if partial {
		properties["id"] = map[string]any{"type": "string", "const": "completed-pages", "description": "已完整校验页面的聚合批次标识"}
		properties["complete"].(map[string]any)["const"] = false
		required = append(required, "id")
	}
	return map[string]any{
		"type": "object", "description": "指定时间窗内已完整校验页面的去重会话及完整性证据",
		"properties": properties, "required": required, "additionalProperties": false,
	}
}

// activeConversationPageErrorInfo preserves the original error classification
// and diagnostics. A safe-to-repeat read does not make every cause retryable.
func activeConversationPageErrorInfo(err error, failedPage int, failedCursor string) *output.ErrorInfo {
	info := &output.ErrorInfo{Type: "internal", Message: err.Error(), Operation: activeConversationsOperation}
	switch apperrors.ExitCode(err) {
	case apperrors.ExitCodeAPI:
		info.Type = "api"
	case apperrors.ExitCodeAuth:
		info.Type = "auth"
	case apperrors.ExitCodeValidation:
		info.Type = "validation"
	case apperrors.ExitCodePermission:
		info.Type = "permission"
	case apperrors.ExitCodeDiscovery:
		info.Type = "discovery"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		info.Subtype = "deadline_exceeded"
	} else if errors.Is(err, context.Canceled) {
		info.Subtype = "canceled"
	}
	var cliErr *helpers.CLIError
	if errors.As(err, &cliErr) && cliErr != nil {
		info.UpstreamCode = cliErr.Code
		info.Hint = cliErr.Suggestion
		if cliErr.Operation != "" {
			info.Operation = cliErr.Operation
		}
	}
	details := make(map[string]any)
	var callErr *transport.CallError
	if errors.As(err, &callErr) && callErr != nil {
		info.HTTPStatus = callErr.HTTPStatus
		info.RPCCode = callErr.RPCCode
		info.Stage = string(callErr.Stage)
		info.RequestID = callErr.RequestID
		info.TraceID = callErr.TraceID
		if info.RequestID == "" {
			info.RequestID = callErr.TraceID
		}
		if callErr.RetryAfter != "" {
			details["retryAfter"] = callErr.RetryAfter
			if seconds, parseErr := strconv.ParseInt(strings.TrimSpace(callErr.RetryAfter), 10, 64); parseErr == nil && seconds >= 0 {
				info.RetryAfterSeconds = &seconds
			}
		}
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) && typed != nil {
		if typed.Reason != "" {
			info.Subtype = typed.Reason
		}
		if typed.Hint != "" {
			info.Hint = typed.Hint
		}
		info.Actions = apperrors.RecoveryActions(err)
		info.Retryable = typed.RetryableSet && typed.Retryable
		if typed.RetryAfterSeconds != nil {
			seconds := *typed.RetryAfterSeconds
			info.RetryAfterSeconds = &seconds
		}
		if typed.RPCCode != 0 {
			info.RPCCode = typed.RPCCode
		}
		if typed.ServerDiag.TraceID != "" {
			info.TraceID = typed.ServerDiag.TraceID
		}
		if typed.Operation != "" {
			info.Operation = typed.Operation
		}
		info.ServerKey = typed.ServerKey
		info.Origin = typed.Origin
		if typed.FailureStage != "" {
			info.Stage = typed.FailureStage
		}
		if typed.ExecutionStarted != nil {
			started := *typed.ExecutionStarted
			info.ExecutionStarted = &started
		}
		if typed.NextRetryAt != nil {
			info.NextRetryAt = typed.NextRetryAt.UTC().Format(time.RFC3339)
		}
		info.AvailableFlags = append([]string(nil), typed.AvailableFlags...)
		info.SnapshotPath = typed.Snapshot
		for key, value := range typed.Details {
			details[key] = value
		}
		if len(typed.RPCData) > 0 {
			var rpcData any
			if json.Unmarshal(typed.RPCData, &rpcData) == nil {
				info.RPCData = rpcData
			}
		}
		info.TechnicalDetail = typed.ServerDiag.TechnicalDetail
		info.FriendlyHint, info.ActionURL = apperrors.ServerGuidance(typed.ServerDiag)
		if typed.Cause != nil {
			info.Cause = typed.Cause.Error()
		}
		if typed.ServerDiag.ServerErrorCode != "" {
			info.UpstreamCode = typed.ServerDiag.ServerErrorCode
		}
	}
	var patErr *apperrors.PATError
	if errors.As(err, &patErr) && patErr != nil {
		info.Type = "permission"
		var permission map[string]any
		if json.Unmarshal([]byte(patErr.RawJSON), &permission) == nil && permission != nil {
			details["permission"] = permission
			for _, key := range []string{"code", "errorCode", "error_code"} {
				if code, ok := permission[key].(string); ok && code != "" {
					info.UpstreamCode = code
					break
				}
			}
		}
	}
	details["failedPage"] = failedPage
	details["failedCursor"] = failedCursor
	info.Details = details
	const resumeHint = "已保留此前完整校验页面的聚合结果；先处理本次失败原因，再保持同一 profile、--start、--end 和 --limit（结果 pageSize），使用 meta.pagination.next_token 继续查询，并按 conversationId 合并各批次。"
	if info.Hint = strings.TrimSpace(info.Hint); info.Hint == "" {
		info.Hint = resumeHint
	} else {
		info.Hint += "；" + resumeHint
	}
	return info
}
