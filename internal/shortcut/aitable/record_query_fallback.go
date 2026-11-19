// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// A timeout fallback gets one normal transport-timeout budget for the entire
// small-page attempt, not a fresh unbounded budget for every page. An existing
// parent deadline must leave this full budget; we do not guess query latency.
const recordQueryTimeoutFallbackBudget = config.HTTPTimeout

// recordQueryLargePageTimeout identifies a transport timeout at a page larger
// than 20. The underlying error keeps its classification and diagnostic cause.
type recordQueryLargePageTimeout struct{ err error }

func (e *recordQueryLargePageTimeout) Error() string { return e.err.Error() }
func (e *recordQueryLargePageTimeout) Unwrap() error { return e.err }

// recordQueryRPCTimeout accepts only structured tools/call transport timeouts.
// Bare deadlines, server business errors and message-only hints are insufficient.
func recordQueryRPCTimeout(err error) bool {
	var classified *apperrors.Error
	if errors.Is(err, context.Canceled) || !errors.As(err, &classified) ||
		classified.Operation != "tools/call" || classified.ServerDiag.ServerErrorCode != "" {
		return false
	}
	var timeout net.Error
	if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &timeout) && timeout.Timeout()) {
		return false // Transport classification alone must not turn an arbitrary server error into a retry.
	}
	if classified.Category == apperrors.CategoryDiscovery && classified.Reason == "tools_call_response_read_failed" {
		return true
	}
	return classified.Category == apperrors.CategoryAPI && classified.FailureStage == "request" &&
		(classified.Reason == "request_timeout" || classified.Reason == "http_client_timeout")
}

// recordQueryFallbackPolicy makes at most one retry eligible. The wrappers call
// the small-page loop directly, so its failure cannot recursively retry. User
// cancellation and an exhausted/insufficient parent budget always stop the read.
func recordQueryFallbackPolicy(rt *shortcut.RuntimeContext, err error) (recordQueryReadPolicy, context.CancelFunc, bool) {
	policy := recordQueryReadPolicy{pageSize: recordQueryServicePageSize, scan: true}
	ctx := rt.Command().Context()
	if !recordQueryNeedsSmallPage(err) || ctx.Err() != nil {
		return policy, nil, false
	}
	var timeout *recordQueryLargePageTimeout
	if !errors.As(err, &timeout) {
		return policy, func() {}, true // Preserve existing size/legacy-shape recovery.
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < recordQueryTimeoutFallbackBudget {
		return policy, nil, false
	}
	bounded, cancel := context.WithTimeout(ctx, recordQueryTimeoutFallbackBudget)
	policy.ctx = bounded
	return policy, cancel, true
}

// callPage preserves the normal runtime for all initial and exact-ID reads.
// Only timeout recovery supplies a bounded read-only context. It uses the same
// helper/auth/error path, without temporarily changing the command's context.
func (policy recordQueryReadPolicy) callPage(rt *shortcut.RuntimeContext, params map[string]any) (map[string]any, error) {
	var data map[string]any
	var err error
	if policy.ctx == nil {
		data, err = rt.CallMCPData(serverMain, "query_records", params)
	} else {
		if err := policy.ctx.Err(); err != nil {
			// Context errors at this boundary need an API classification; a bare
			// deadline/cancellation would otherwise surface as an internal failure.
			return nil, apperrors.NewAPI("记录查询回退已停止；请丢弃本轮累计结果",
				apperrors.WithOperation("aitable.query_records"),
				apperrors.WithOrigin("client"),
				apperrors.WithFailureStage("pagination"),
				apperrors.WithExecutionStarted(true),
				apperrors.WithReason("record_query_fallback_stopped"),
				apperrors.WithRetryable(false),
				apperrors.WithHint("查询已取消或回退预算已耗尽。需要重新读取时，仅从第一页重新执行只读查询；不要重跑含写入的流程。"),
				apperrors.WithDetails(map[string]any{"discard_previous_results": true, "restart_from_first_page": true}),
				apperrors.WithCause(err))
		}
		var text string
		text, err = helpers.CallMCPReadToolTextOnServerContext(policy.ctx, serverMain, "query_records", params)
		if err == nil && strings.TrimSpace(text) != "" {
			if decodeErr := json.Unmarshal([]byte(text), &data); decodeErr != nil {
				err = apperrors.NewInternal(fmt.Sprintf("解析 query_records 返回失败: %v", decodeErr))
			}
		}
	}
	if size, _ := params["limit"].(int); policy.scan && size > recordQueryServicePageSize && recordQueryRPCTimeout(err) {
		return nil, &recordQueryLargePageTimeout{err: err}
	}
	return data, err
}
