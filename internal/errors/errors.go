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

package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

var marshalErrorJSON = jsonutil.MarshalIndent

// Category represents a stable error class with a documented exit code.
type Category string

const (
	CategoryAPI        Category = "api"
	CategoryAuth       Category = "auth"
	CategoryValidation Category = "validation"
	CategoryDiscovery  Category = "discovery"
	CategoryInternal   Category = "internal"

	// CategoryPartial is retained for source compatibility, but an error cannot
	// reconstruct the per-item data required by a partial result. It therefore
	// fails closed as internal; callers must use output.Partial for exit code 7.
	CategoryPartial Category = "partial_failure"
)

// 退出码表（规划 v1.2 OQ-1 定案；契约规范 §4；轮10裁决⑬——保留现行码表，
// 仅新增 partial_failure 专用码，不做 wire 破坏性重排）：
//
//	0  success / pending（异步受理不是失败）
//	1  api            （CategoryAPI）
//	2  auth           （CategoryAuth）
//	3  validation     （CategoryValidation；confirmation_required 子类共享此码，
//	                   以 reason/subtype 区分，AC-13）
//	4  PAT            （PATError 专属，见 pat.go ExitCodePermission；Category 不占用）
//	5  internal       （CategoryInternal 与兜底：非结构化错误、panic 收敛均归 5）
//	6  discovery      （CategoryDiscovery）
//	7  partial_failure（部分成功专用码，见 ExitCodePartial）
//
// ExitCodePartial is the partial-result exit code shared with internal/output.
// It is not returned for CategoryPartial errors because they lack the typed
// succeeded/failed/unknown payload required for an honest partial result.
const ExitCodePartial = 7

// 类别专属退出码常量（B171/B172，权威 = 规划 v1.2 OQ-1 定案，契约规范 §4）。
// ExitCode() 的 switch 用内联字面量，本组常量由 exitcodes.go 的
// exitCodeByCategory 映射表引用，值与内联字面量一一对应（同源不双轨）。
// 修改任一值必须先同步 ExitCode() 的 switch 分支与 internal/output 侧码表。
const (
	ExitCodeAPI        = 1
	ExitCodeAuth       = 2
	ExitCodeValidation = 3
	ExitCodeDiscovery  = 6
	ExitCodeInternal   = 5
)

// Error is the structured repository-local error model for the Go rewrite.
type Error struct {
	Category          Category
	Message           string
	Operation         string
	ServerKey         string
	Origin            string
	FailureStage      string
	ExecutionStarted  *bool
	Retryable         bool
	RetryableSet      bool
	RetryAfterSeconds *int64
	NextRetryAt       *time.Time
	Reason            string
	Hint              string
	Actions           []string
	AvailableFlags    []string
	Snapshot          string
	Details           map[string]any
	RPCCode           int               `json:"rpc_code,omitempty"`
	RPCData           json.RawMessage   `json:"rpc_data,omitempty"`
	ServerDiag        ServerDiagnostics `json:"-"`
	Cause             error             `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As chains.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Option mutates a structured error before it is returned.
type Option func(*Error)

// ExitCode returns the documented process exit code for the error category.
// exit=4 is reserved exclusively for PATError (see internal/errors/pat.go
// ExitCodePermission and the exit-code table in docs/reference.md);
// Discovery therefore uses 6 so hosts can tell "catalog lookup broke"
// apart from "PAT permission insufficient".
//
// confirmation_required 是 validation 的子类而非独立类别（B171，AC-13，
// 规划 v1.2 OQ-1 定案）：门禁拦截错误挂 CategoryValidation 并以
// reason=confirmation_required 区分，与 validation 共享 rc=3。信封侧
// internal/output exitCodeForErrorInfo 的「subtype 优先于 type、
// confirmation_required 恒 3」规则与本表同源（轮10裁决⑬；远期独立码
// 保留于规划 OQ-9，落地前不得双轨）。
func (e *Error) ExitCode() int {
	switch e.Category {
	case CategoryAPI:
		return 1
	case CategoryAuth:
		return 2
	case CategoryValidation:
		return 3
	case CategoryDiscovery:
		return 6
	case CategoryPartial:
		// An error has no per-item succeeded/failed/unknown data and therefore
		// cannot truthfully represent partial_failure. Fail closed as internal.
		return ExitCodeInternal
	default:
		return 5
	}
}

// WithOperation records the operation that failed.
func WithOperation(operation string) Option {
	return func(err *Error) {
		err.Operation = operation
	}
}

// IsConfirmationRequired reports whether err (or any wrapped cause) is a
// typed framework confirmation-gate failure carrying reason
// confirmation_required. Downstream classifiers must pass such errors through
// verbatim: the "re-run with --yes" semantics can only be carried by the
// machine-readable reason, while message-text classification actively
// misroutes them (a command path containing "permission" would be reported as
// an auth failure, and any other wording degrades to an unclassified error).
func IsConfirmationRequired(err error) bool {
	if err == nil {
		return false
	}
	var typed *Error
	if stderrors.As(err, &typed) {
		return strings.TrimSpace(typed.Reason) == "confirmation_required"
	}
	return false
}

// WithServerKey records the server identifier associated with the failure.
func WithServerKey(serverKey string) Option {
	return func(err *Error) {
		err.ServerKey = serverKey
	}
}

// WithOrigin records the component that produced the failure, such as the
// client, MCP gateway, or DingTalk API. It is independent from Category,
// which remains the stable exit-code contract.
func WithOrigin(origin string) Option {
	return func(err *Error) {
		err.Origin = strings.TrimSpace(origin)
	}
}

// WithFailureStage records the execution stage at which the failure occurred.
func WithFailureStage(stage string) Option {
	return func(err *Error) {
		err.FailureStage = strings.TrimSpace(stage)
	}
}

// WithExecutionStarted records whether the downstream business operation was
// known to have started. Unknown state must be represented by omitting this
// option, which is important for safe retry decisions on write operations.
func WithExecutionStarted(started bool) Option {
	return func(err *Error) {
		value := started
		err.ExecutionStarted = &value
	}
}

// WithRetryable marks whether the error can be retried safely.
func WithRetryable(retryable bool) Option {
	return func(err *Error) {
		err.Retryable = retryable
		err.RetryableSet = true
	}
}

// WithRetryAfterSeconds records the server-recommended delay before a retry.
// A zero delay is meaningful and is therefore preserved; negative values are
// ignored as invalid server guidance.
//
// 本通道只存原值、不钳制（B195/B199，AC-24）：服务端给多少存多少，wire 上
// retry_after_seconds 原样透传。transport 侧的 RetryMaxDelay 钳制只作用于
// 重试延迟选择（retryDelayForAttempt），不得回写或截断本字段（B196 草案：
// 钳制上限可配置化后仍须保持「钳制延迟、不钳制透传」双通道分离）。
func WithRetryAfterSeconds(seconds int64) Option {
	return func(err *Error) {
		if seconds < 0 {
			return
		}
		value := seconds
		err.RetryAfterSeconds = &value
	}
}

// WithNextRetryAt records the absolute time at which a retry may be attempted.
func WithNextRetryAt(next time.Time) Option {
	return func(err *Error) {
		if next.IsZero() {
			return
		}
		value := next.UTC()
		err.NextRetryAt = &value
	}
}

// WithReason records a stable machine-readable failure reason.
func WithReason(reason string) Option {
	return func(err *Error) {
		err.Reason = reason
	}
}

// WithHint records a short recovery hint for humans and agents.
func WithHint(hint string) Option {
	return func(err *Error) {
		err.Hint = hint
	}
}

// WithActions records suggested next actions for recovery.
func WithActions(actions ...string) Option {
	return func(err *Error) {
		out := make([]string, 0, len(actions))
		for _, action := range actions {
			if action == "" {
				continue
			}
			out = append(out, action)
		}
		if len(out) > 0 {
			err.Actions = out
		}
	}
}

// WithAvailableFlags records visible local flag names for agent recovery.
func WithAvailableFlags(names ...string) Option {
	return func(err *Error) {
		if len(names) == 0 {
			return
		}
		err.AvailableFlags = append([]string{}, names...)
	}
}

// WithSnapshot records the recovery snapshot path associated with the failure.
func WithSnapshot(path string) Option {
	return func(err *Error) {
		err.Snapshot = path
	}
}

// WithDetails records an additive machine-readable payload for errors whose
// recovery needs typed context, such as ambiguous target-resolution
// candidates. Callers must keep credentials and other secrets out of details.
func WithDetails(details map[string]any) Option {
	return func(err *Error) {
		if len(details) == 0 {
			return
		}
		err.Details = make(map[string]any, len(details))
		for key, value := range details {
			err.Details[key] = value
		}
	}
}

// WithRPCCode records the original JSON-RPC error code.
func WithRPCCode(code int) Option {
	return func(err *Error) {
		err.RPCCode = code
	}
}

// WithRPCData records the original JSON-RPC error data payload.
func WithRPCData(data json.RawMessage) Option {
	if len(bytes.TrimSpace(data)) == 0 {
		return func(*Error) {} // no-op, consistent with other Options
	}
	return func(err *Error) {
		err.RPCData = data
	}
}

// WithCause wraps the original error so it can be retrieved via errors.Unwrap.
func WithCause(err error) Option {
	return func(e *Error) {
		e.Cause = err
	}
}

func newError(category Category, message string, opts ...Option) error {
	err := &Error{
		Category: category,
		Message:  message,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(err)
		}
	}
	return err
}

// NewAPI returns an API-category error.
func NewAPI(message string, opts ...Option) error {
	return newError(CategoryAPI, message, opts...)
}

// NewAuth returns an auth-category error.
func NewAuth(message string, opts ...Option) error {
	return newError(CategoryAuth, message, opts...)
}

// NewValidation returns a validation-category error.
func NewValidation(message string, opts ...Option) error {
	return newError(CategoryValidation, message, opts...)
}

// NewDiscovery returns a discovery-category error.
func NewDiscovery(message string, opts ...Option) error {
	return newError(CategoryDiscovery, message, opts...)
}

// NewInternal returns an internal-category error.
func NewInternal(message string, opts ...Option) error {
	return newError(CategoryInternal, message, opts...)
}

// ExitCoder is implemented by errors that provide their own exit code.
// Edition-specific error types (e.g. PATError, CLIError) implement this
// so the framework can resolve exit codes without importing edition packages.
type ExitCoder interface {
	ExitCode() int
}

// RawStderrError is implemented by errors that must output raw content
// directly to stderr, bypassing all CLI formatting (e.g. "Error:" prefix).
// PAT authorization errors use this to pass JSON through to the desktop runtime.
type RawStderrError interface {
	error
	RawStderr() string
}

// ExitCode maps any error to a stable exit code.
func ExitCode(err error) int {
	var typed *Error
	if stderrors.As(err, &typed) {
		return typed.ExitCode()
	}
	var ec ExitCoder
	if stderrors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 5
}

// PrintJSON writes the legacy machine-readable JSON error object.
//
// This wire predates the unified result framework and is intentionally kept
// byte-compatible for commands whose rollout is legacy_only or dual_validate.
// Unified commands publish outcome/type/subtype through internal/output only.
func PrintJSON(w io.Writer, err error) error {
	errorPayload := map[string]any{
		"code":     ExitCode(err),
		"category": category(err),
		"message":  err.Error(),
	}
	var typed *Error
	if stderrors.As(err, &typed) {
		if typed.Reason != "" {
			errorPayload["reason"] = typed.Reason
		}
		if typed.Operation != "" {
			errorPayload["operation"] = typed.Operation
		}
		if typed.ServerKey != "" {
			errorPayload["server_key"] = typed.ServerKey
		}
		if typed.Origin != "" {
			errorPayload["origin"] = typed.Origin
		}
		if typed.FailureStage != "" {
			errorPayload["stage"] = typed.FailureStage
		}
		if typed.ExecutionStarted != nil {
			errorPayload["execution_started"] = *typed.ExecutionStarted
		}
		if typed.RetryableSet {
			errorPayload["retryable"] = typed.Retryable
		}
		if typed.RetryAfterSeconds != nil {
			errorPayload["retry_after_seconds"] = *typed.RetryAfterSeconds
		}
		if typed.NextRetryAt != nil {
			errorPayload["next_retry_at"] = typed.NextRetryAt.UTC().Format(time.RFC3339)
		}
		if typed.Hint != "" {
			errorPayload["hint"] = typed.Hint
		}
		if actions := RecoveryActions(err); len(actions) > 0 {
			errorPayload["actions"] = actions
		}
		if len(typed.AvailableFlags) > 0 {
			errorPayload["available_flags"] = typed.AvailableFlags
		}
		if typed.Snapshot != "" {
			errorPayload["snapshot_path"] = typed.Snapshot
		}
		if len(typed.Details) > 0 {
			errorPayload["details"] = typed.Details
		}
		if typed.RPCCode != 0 {
			errorPayload["rpc_code"] = typed.RPCCode
		}
		if len(typed.RPCData) > 0 {
			var parsed any
			if json.Unmarshal(typed.RPCData, &parsed) == nil {
				errorPayload["rpc_data"] = parsed
			}
		}
		if !typed.ServerDiag.IsEmpty() {
			if typed.ServerDiag.TraceID != "" {
				errorPayload["trace_id"] = typed.ServerDiag.TraceID
			}
			if typed.ServerDiag.ServerErrorCode != "" {
				errorPayload["server_error_code"] = typed.ServerDiag.ServerErrorCode
			}
			if typed.ServerDiag.TechnicalDetail != "" {
				errorPayload["technical_detail"] = typed.ServerDiag.TechnicalDetail
			}
			friendlyHint, actionURL := serverGuidance(typed.ServerDiag)
			if friendlyHint != "" {
				errorPayload["friendly_hint"] = friendlyHint
			}
			if actionURL != "" {
				errorPayload["action_url"] = actionURL
			}
		}
		if typed.Cause != nil {
			errorPayload["cause"] = typed.Cause.Error()
		}
	}
	payload := map[string]any{"error": errorPayload}

	data, marshalErr := marshalErrorJSON(payload, "", "  ")
	if marshalErr != nil {
		_, writeErr := fmt.Fprintln(w, `{"error":{"code":5,"category":"internal","message":"failed to encode error output"}}`)
		return writeErr
	}

	_, writeErr := fmt.Fprintln(w, string(data))
	return writeErr
}

// Verbosity controls how much detail PrintHuman includes.
type Verbosity int

const (
	// VerbosityNormal shows essential info: error, hint, actions, trace_id, server_code.
	VerbosityNormal Verbosity = 0
	// VerbosityVerbose adds technical_detail, snapshot, execution context.
	VerbosityVerbose Verbosity = 1
	// VerbosityDebug adds all internal diagnostics (category, operation, reason, rpc_code).
	VerbosityDebug Verbosity = 2
)

// PrintHuman writes a concise human-readable error rendering at normal verbosity.
func PrintHuman(w io.Writer, err error) error {
	return PrintHumanAt(w, err, VerbosityNormal)
}

// PrintHumanAt writes a human-readable error rendering at the given verbosity level.
func PrintHumanAt(w io.Writer, err error, v Verbosity) error {
	if err == nil {
		return nil
	}

	var typed *Error
	if !stderrors.As(err, &typed) {
		_, writeErr := fmt.Fprintf(w, "%s %s\n", tui.StateMark("error"), tui.Danger("Error: "+err.Error()))
		return writeErr
	}

	// Line 1: Error summary
	lines := []string{
		fmt.Sprintf("%s %s", tui.StateMark("error"), tui.Danger(fmt.Sprintf("Error: [%s] %s", strings.ToUpper(string(typed.Category)), typed.Message))),
	}

	// Always shown: hint, actions, retryable
	if typed.Hint != "" {
		lines = append(lines, tui.Cyan(fmt.Sprintf("Hint: %s", typed.Hint)))
	}

	if friendlyHint, actionURL := serverGuidance(typed.ServerDiag); friendlyHint != "" || actionURL != "" {
		if friendlyHint != "" {
			lines = append(lines, tui.Cyan("Hint: "+friendlyHint))
		}
		if actionURL != "" {
			lines = append(lines, tui.White("Action: 处理入口: "+actionURL))
		}
	}

	if actions := HumanRecoveryActions(err); len(actions) > 0 {
		for _, action := range actions {
			if strings.TrimSpace(action) == "" {
				continue
			}
			lines = append(lines, tui.White(fmt.Sprintf("Action: %s", action)))
		}
	}
	if line := formatAvailableFlagsHumanLine(typed.AvailableFlags); line != "" {
		lines = append(lines, tui.Dim(line))
	}
	if typed.RetryableSet {
		lines = append(lines, tui.Warning(fmt.Sprintf("Retryable: %t", typed.Retryable)))
	}
	if typed.RetryAfterSeconds != nil {
		lines = append(lines, tui.Warning(fmt.Sprintf("Retry After: %ds", *typed.RetryAfterSeconds)))
	}
	if typed.NextRetryAt != nil {
		lines = append(lines, tui.Warning("Next Retry At: "+typed.NextRetryAt.UTC().Format(time.RFC3339)))
	}

	// Always shown when present: Trace ID, Server Code
	if typed.ServerDiag.TraceID != "" {
		lines = append(lines, tui.Dim(fmt.Sprintf("Trace ID: %s", typed.ServerDiag.TraceID)))
	}
	if typed.ServerDiag.ServerErrorCode != "" {
		lines = append(lines, tui.Dim(fmt.Sprintf("Server Code: %s", typed.ServerDiag.ServerErrorCode)))
	}

	// Verbose+: technical detail, snapshot, reason, server key
	if v >= VerbosityVerbose {
		if typed.ServerDiag.TechnicalDetail != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Detail: %s", typed.ServerDiag.TechnicalDetail)))
		}
		if typed.Reason != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Reason: %s", typed.Reason)))
		}
		if typed.ServerKey != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Server: %s", typed.ServerKey)))
		}
		if typed.Origin != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Origin: %s", typed.Origin)))
		}
		if typed.FailureStage != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Stage: %s", typed.FailureStage)))
		}
		if typed.ExecutionStarted != nil {
			lines = append(lines, tui.Dim(fmt.Sprintf("Execution Started: %t", *typed.ExecutionStarted)))
		}
		if typed.Snapshot != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Snapshot: %s", typed.Snapshot)))
		}
		if typed.Cause != nil {
			lines = append(lines, tui.Dim(fmt.Sprintf("Cause: %s", typed.Cause.Error())))
		}
	}

	// Debug: all internal diagnostics
	if v >= VerbosityDebug {
		if typed.Operation != "" {
			lines = append(lines, tui.Dim(fmt.Sprintf("Operation: %s", typed.Operation)))
		}
		if typed.RPCCode != 0 {
			lines = append(lines, tui.Dim(fmt.Sprintf("RPC Code: %d", typed.RPCCode)))
		}
		if len(typed.RPCData) > 0 {
			lines = append(lines, tui.Dim(fmt.Sprintf("RPC Data: %s", string(typed.RPCData))))
		}
	}

	_, writeErr := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return writeErr
}

func serverGuidance(diag ServerDiagnostics) (string, string) {
	friendlyHint := strings.TrimSpace(diag.FriendlyHint)
	actionURL := safeServerActionURL(diag.ActionURL)
	if friendlyHint == "" || actionURL == "" {
		switch diag.ServerErrorCode {
		case "TOKEN_VERIFIED_FAILED", "CLI_ORG_NOT_AUTHORIZED":
			if friendlyHint == "" {
				friendlyHint = "该组织尚未开启 CLI 数据访问权限，请联系组织主管理员开启。"
			}
			if actionURL == "" {
				actionURL = config.GetDeveloperSettingsURL()
			}
		}
	}
	return friendlyHint, actionURL
}

// ServerGuidance exposes the same recovery projection to repository-local
// adapters so legacy JSON and unified-result errors stay semantically aligned.
func ServerGuidance(diag ServerDiagnostics) (string, string) {
	return serverGuidance(diag)
}

func safeServerActionURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func category(err error) string {
	var typed *Error
	if stderrors.As(err, &typed) {
		if typed.Category == CategoryPartial {
			return string(CategoryInternal)
		}
		return string(typed.Category)
	}
	return string(CategoryInternal)
}

const availableFlagsHumanMaxRunes = 200

func formatAvailableFlagsHumanLine(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	b := strings.Builder{}
	b.WriteString("Flags: ")
	written := 0
	for i, name := range flags {
		if i > 0 {
			if written+2 > availableFlagsHumanMaxRunes {
				b.WriteString("...")
				return b.String()
			}
			b.WriteString(", ")
			written += 2
		}
		if written+len(name) > availableFlagsHumanMaxRunes {
			b.WriteString("...")
			return b.String()
		}
		b.WriteString(name)
		written += len(name)
	}
	return b.String()
}
