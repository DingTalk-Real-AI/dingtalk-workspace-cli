// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	var diag apperrors.ServerDiagnostics
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
	code := diag.ServerErrorCode
	switch code {
	case "INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE":
	default:
		return nil
	}

	reason := "pagination_restart_required"
	hint := "丢弃本轮和此前保存的累计结果及旧游标；核对查询条件后，不传 --cursor 从第一页重新查询。若持续失败，先等待服务版本/数据稳定；不要重跑含写入步骤的整条命令。"
	if code == "CURSOR_SNAPSHOT_UNAVAILABLE" {
		reason = "pagination_snapshot_unavailable"
		hint = "服务端缺少排序分页所需版本信息；丢弃累计结果和旧游标，待服务修复后从第一页查询。不要原样重试，也不要重跑含写入步骤的整条命令。"
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
		apperrors.WithDetails(map[string]any{"discard_previous_results": true, "restart_from_first_page": true}),
		apperrors.WithCause(err),
	)
	return recovery.(*apperrors.Error)
}
