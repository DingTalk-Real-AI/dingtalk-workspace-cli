// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// devevent_errors.go 是 `dws dev event` 的 runner 错误诊断出口。
//
// 背景：runner 在服务端业务失败（信封 success=false）时返回结构化错误
// （internal/app newServerFailureAPIError → apperrors *Error，信封事实挂在
// ServerDiag：server_error_code/technical_detail/trace_id），但 Error() 只回
// Message。当信封事实不在 detectBusinessError 可见的层级时，Message 退化为
// 不透明的 "business error: success=false"，仅消费错误字符串的调用方（日志、
// %v、上层 echo）完全看不到 errorCode/errorMsg/technical_detail。
//
// 本文件的契约（与 AGENTS.md 分类边界规则一致）：
//
//   - 取消/截止错误、以及链上无可提取信封事实的错误：原样透传，不按错误
//     文案猜测类别；
//   - 已分类错误（apperrors *Error）：类别/reason/hint/退出码语义原样保留，
//     仅当 ServerDiag 携带 message 未自述的 errorCode/technical_detail 时，
//     重建同一类别的结构化错误，把 errorCode 补进 message 与 Details，
//     原错误经 WithCause 挂链，errors.Is/As 语义不变；
//   - 未分类的 transport.CallError：提取 trace/rpc/stage 事实重建为 API 类别
//     （与 helpers.WrapErrorWithOperation 对网络类失败回 ExitAPI 的既有约定
//     一致），避免退化成 internal/退出码 5。
//
// 409 CONFIG_REVISION_CONFLICT 走 runDevEventTool 的在带结果投影，不经本文件。
func devEventRunnerError(err error) error {
	if err == nil {
		return nil
	}
	// 取消/截止错误必须原样透传：它们的身份只能由既有分类边界改写。
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var typed *apperrors.Error
	var callErr *transport.CallError
	switch {
	case stderrors.As(err, &typed) && typed != nil:
		return devEventDiagnoseTypedError(err, typed)
	case stderrors.As(err, &callErr) && callErr != nil:
		return devEventDiagnoseCallError(err, callErr)
	default:
		// 纯文本错误且链上没有结构化信封：无事实可提取，原样透传。
		return err
	}
}

// devEventDiagnoseTypedError 处理已分类错误：仅在 ServerDiag 携带 message 未
// 自述的 errorCode/technical_detail 时重建，类别、reason、hint、退出码与
// ServerDiag 原样保留，原错误整体挂 Cause 链。
func devEventDiagnoseTypedError(err error, typed *apperrors.Error) error {
	code := strings.TrimSpace(typed.ServerDiag.ServerErrorCode)
	detail := strings.TrimSpace(typed.ServerDiag.TechnicalDetail)
	if code == "" && detail == "" {
		return err
	}
	if code != "" && devEventMessageMentionsCode(err.Error(), code) {
		// message 已自述 errorCode（如 "business error: code X"），
		// ServerDiag 又会由输出层投影，重建只会有噪声。
		return err
	}
	details := make(map[string]any, len(typed.Details)+3)
	for key, value := range typed.Details {
		details[key] = value
	}
	if code != "" {
		details["errorCode"] = code
	}
	if detail != "" {
		details["technicalDetail"] = detail
	}
	if trace := strings.TrimSpace(typed.ServerDiag.TraceID); trace != "" {
		details["traceId"] = trace
	}
	message := err.Error()
	if code != "" {
		message = fmt.Sprintf("%s (errorCode=%s)", strings.TrimRight(message, " "), code)
	}
	opts := []apperrors.Option{
		apperrors.WithOperation(typed.Operation),
		apperrors.WithServerKey(typed.ServerKey),
		apperrors.WithOrigin(typed.Origin),
		apperrors.WithFailureStage(typed.FailureStage),
		apperrors.WithReason(typed.Reason),
		apperrors.WithHint(typed.Hint),
		apperrors.WithActions(typed.Actions...),
		apperrors.WithDetails(details),
		apperrors.WithCause(err),
		apperrors.WithServerDiag(typed.ServerDiag),
	}
	if typed.RetryableSet {
		opts = append(opts, apperrors.WithRetryable(typed.Retryable))
	}
	if typed.RetryAfterSeconds != nil {
		opts = append(opts, apperrors.WithRetryAfterSeconds(*typed.RetryAfterSeconds))
	}
	if typed.NextRetryAt != nil {
		opts = append(opts, apperrors.WithNextRetryAt(*typed.NextRetryAt))
	}
	if typed.ExecutionStarted != nil {
		opts = append(opts, apperrors.WithExecutionStarted(*typed.ExecutionStarted))
	}
	if typed.RPCCode != 0 {
		opts = append(opts, apperrors.WithRPCCode(typed.RPCCode))
	}
	if len(typed.AvailableFlags) > 0 {
		opts = append(opts, apperrors.WithAvailableFlags(typed.AvailableFlags...))
	}
	if typed.Snapshot != "" {
		opts = append(opts, apperrors.WithSnapshot(typed.Snapshot))
	}
	if len(typed.RPCData) > 0 {
		opts = append(opts, apperrors.WithRPCData(typed.RPCData))
	}
	return devEventCategoryError(typed.Category, message, opts)
}

// devEventDiagnoseCallError 处理未分类的 transport.CallError：HTTP/JSON-RPC
// 传输失败当前会退化成 internal/退出码 5，这里按既有约定归一为 API 类别并
// 透传 trace/rpc/stage 事实；CallError 本体留在 Cause 链上，输出层既有的
// CallError 投影不受影响。
func devEventDiagnoseCallError(err error, callErr *transport.CallError) error {
	diag := apperrors.ServerDiagnostics{}
	if trace := strings.TrimSpace(callErr.TraceID); trace != "" {
		diag.TraceID = trace
	} else if requestID := strings.TrimSpace(callErr.RequestID); requestID != "" {
		diag.TraceID = requestID
	}
	opts := []apperrors.Option{
		apperrors.WithServerKey(devEventProduct),
		apperrors.WithFailureStage(strings.TrimSpace(string(callErr.Stage))),
		apperrors.WithCause(err),
	}
	if !diag.IsEmpty() {
		opts = append(opts, apperrors.WithServerDiag(diag))
	}
	if callErr.RPCCode != 0 {
		opts = append(opts, apperrors.WithRPCCode(callErr.RPCCode))
	}
	return apperrors.NewAPI(err.Error(), opts...)
}

// devEventCategoryError 按原错误的类别重建，类别决定退出码，不做任何推导。
func devEventCategoryError(category apperrors.Category, message string, opts []apperrors.Option) error {
	switch category {
	case apperrors.CategoryAuth:
		return apperrors.NewAuth(message, opts...)
	case apperrors.CategoryValidation:
		return apperrors.NewValidation(message, opts...)
	case apperrors.CategoryDiscovery:
		return apperrors.NewDiscovery(message, opts...)
	case apperrors.CategoryInternal:
		return apperrors.NewInternal(message, opts...)
	default:
		return apperrors.NewAPI(message, opts...)
	}
}

// devEventMessageMentionsCode 判断 message 是否已自述该业务码。保守策略：
// 出现疑似提及（大小写不敏感）就放弃重建，透传永远安全。
func devEventMessageMentionsCode(message, code string) bool {
	return strings.Contains(strings.ToLower(message), strings.ToLower(code))
}
