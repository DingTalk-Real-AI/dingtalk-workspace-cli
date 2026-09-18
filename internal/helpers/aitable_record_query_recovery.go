// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	// nonResumableCursorReason 标记服务端不可恢复的 error-v1: 保护游标，供服务端错误码
	// 路径与成功响应体夹带保护游标的路径共用同一结构化分类。
	nonResumableCursorReason = "pagination_non_resumable_error"
	nonResumableCursorHint   = "服务端返回了不可恢复的错误游标（error-v1: 保护游标）；丢弃全部累计结果和旧游标，立即停止分页，禁止将该游标回传。核对查询条件后可尝试不传 --cursor 从第一页重新查询。"
)

// RecordQueryRecoveryError 仅为记录查询的已知游标错误补充恢复策略，不重试请求或重放写流程。
// 网络、权限与未知错误保持原样；判断依据为结构化错误码，不能匹配报错文案猜测原因。
func RecordQueryRecoveryError(err error) error {
	if recovery := recordQueryCursorError(err); recovery != nil {
		return recovery
	}
	return err
}

// recordQueryCursorError 兼容统一诊断、旧 CLIError 和数据编排路径保留的原始 MCP 错误 JSON。
func recordQueryCursorError(err error) *apperrors.Error {
	if err == nil {
		return nil
	}
	diag := aitableServerDiag(err)
	code := diag.ServerErrorCode
	switch code {
	case "INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE", "CURSOR_OFFSET_LIMIT", "NON_RESUMABLE_ERROR_CURSOR":
	default:
		return nil
	}

	reason := "pagination_restart_required"
	hint := "丢弃本轮和此前保存的累计结果及旧游标；核对查询条件后，不传 --cursor 从第一页重新查询。若持续失败，先等待服务版本/数据稳定；不要重跑含写入步骤的整条命令。"
	details := map[string]any{"discard_previous_results": true, "restart_from_first_page": true}
	switch code {
	case "NON_RESUMABLE_ERROR_CURSOR":
		reason = nonResumableCursorReason
		hint = nonResumableCursorHint
		details = map[string]any{"discard_previous_results": true, "restart_from_first_page": false, "stop_pagination": true}
	case "CURSOR_SNAPSHOT_UNAVAILABLE":
		reason = "pagination_snapshot_unavailable"
		hint = "服务端缺少排序分页所需版本信息；丢弃累计结果和旧游标，待服务修复后从第一页查询。不要原样重试，也不要重跑含写入步骤的整条命令。"
	case "CURSOR_OFFSET_LIMIT":
		// offset 超限不能靠不传 cursor 从头重查解决：新查询会再次抵达同一上限。
		// 必须先收窄 filters（或改用 recordIds/分段条件）缩小结果集。
		reason = "pagination_offset_limit_exceeded"
		hint = "排序游标 offset 已达上限（100000）；丢弃累计结果与旧游标，先收窄 --filters（或改用 --record-ids / 分段条件）缩小结果集后再从第一页查询。不要重跑含写入步骤的整条命令。"
		details = map[string]any{"discard_previous_results": true, "restart_from_first_page": false, "narrow_filters_required": true}
	}
	// 保留诊断 trace，但以本地安全策略为准，不接受上游错误地允许重试失效游标。
	recovery := apperrors.NewAPI("record pagination cannot continue: "+code,
		apperrors.WithOperation("aitable.query_records"),
		apperrors.WithServerKey("aitable"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithServerDiag(diag),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
		apperrors.WithHint(hint),
		apperrors.WithDetails(details),
		apperrors.WithCause(err),
	)
	return recovery.(*apperrors.Error)
}

// aitableServerDiag 从统一诊断、旧 CLIError 或保留的原始 MCP 错误 JSON 中提取服务端诊断，
// 仅用于按结构化错误码分支，不做任何重试或写重放。
func aitableServerDiag(err error) apperrors.ServerDiagnostics {
	var diag apperrors.ServerDiagnostics
	if err == nil {
		return diag
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		diag = typed.ServerDiag
	}
	var legacy *CLIError
	if diag.ServerErrorCode == "" && errors.As(err, &legacy) {
		diag.ServerErrorCode = legacy.ServerCode
	}
	for cause := err; diag.ServerErrorCode == "" && cause != nil; cause = errors.Unwrap(cause) {
		// CLIError.Error() 包含展示前缀，只有 Message 才保留原始业务 JSON。
		raw := cause.Error()
		if cli, ok := cause.(*CLIError); ok {
			raw = cli.Message
		}
		var body map[string]any
		if json.Unmarshal([]byte(raw), &body) == nil && isBusinessError(body) {
			diag.ServerErrorCode = businessErrorCode(body)
		}
	}
	return diag
}

// NonResumableCursorResponseError 处理成功响应体中夹带的 error-v1: 保护游标。
// 服务端可能返回成功却把不可恢复游标放进 nextCursor/cursor，表示本轮分页结果不可续读，
// 语义与服务端 NON_RESUMABLE_ERROR_CURSOR 错误码一致：必须丢弃全部累计结果、立即停止分页、
// 禁止回传该游标。executionStarted 供调用方区分只读分页（true）与写前唯一键预检（false）——
// 预检失败必须声明未开始写入，避免调用方误判可能已产生副作用。
func NonResumableCursorResponseError(guardCursor string, executionStarted bool) error {
	return apperrors.NewAPI("record pagination cannot continue: NON_RESUMABLE_ERROR_CURSOR",
		apperrors.WithOperation("aitable.query_records"),
		apperrors.WithServerKey("aitable"),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(executionStarted),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NON_RESUMABLE_ERROR_CURSOR"}),
		apperrors.WithRetryable(false),
		apperrors.WithReason(nonResumableCursorReason),
		apperrors.WithHint(nonResumableCursorHint),
		apperrors.WithDetails(map[string]any{
			"discard_previous_results": true,
			"restart_from_first_page":  false,
			"stop_pagination":          true,
			"guard_cursor":             guardCursor,
		}),
	)
}
