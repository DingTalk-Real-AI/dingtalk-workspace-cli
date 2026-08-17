package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ExitSuccess    = 0
	ExitAPI        = 1
	ExitAuth       = 2
	ExitValidation = 3
	ExitPermission = 4
	ExitInternal   = 5
)

const (
	CodeAuthNotConfigured  = "AUTH_NOT_CONFIGURED"
	CodeAuthTokenExpired   = "AUTH_TOKEN_EXPIRED"
	CodeAuthPermission     = "AUTH_PERMISSION_DENIED"
	CodeNetworkTimeout     = "NETWORK_TIMEOUT"
	CodeNetworkUnreachable = "NETWORK_UNREACHABLE"
	CodeLockTimeout        = "LOCK_TIMEOUT"
	CodeResourceNotFound   = "RESOURCE_NOT_FOUND"
	CodeTableNotFound      = "TABLE_NOT_FOUND"
	CodeSheetNotFound      = "SHEET_NOT_FOUND"
	CodeFieldNotFound      = "FIELD_NOT_FOUND"
	CodeRecordNotFound     = "RECORD_NOT_FOUND"
	CodeInvalidJSON        = "INPUT_INVALID_JSON"
	CodeInvalidPath        = "INPUT_INVALID_PATH"
	CodeInputTooLarge      = "INPUT_TOO_LARGE"
	CodeMissingParam       = "INPUT_MISSING_PARAM"
	CodeInvalidParam       = "INPUT_INVALID_PARAM"
	CodeFileNotFound       = "INPUT_FILE_NOT_FOUND"
	CodeContentTruncated   = "CONTENT_TRUNCATED"
	CodeMCPServerError     = "MCP_SERVER_ERROR"
	CodeMCPToolError       = "MCP_TOOL_ERROR"
	CodeUnclassified       = "UNCLASSIFIED"
)

// CLIError is a user-friendly error with code, suggestion, and exit code.
// Supports traceability via the Operation field and error chain via Cause.
type CLIError struct {
	Code       string
	Message    string
	Suggestion string
	Operation  string // the operation that failed (for traceability)
	Cause      error
}

func (e *CLIError) Error() string {
	s := fmt.Sprintf("[%s] %s", e.Code, e.Message)
	if e.Operation != "" {
		s += fmt.Sprintf(" (operation: %s)", e.Operation)
	}
	if e.Suggestion != "" {
		s += fmt.Sprintf("\n  hint: %s", e.Suggestion)
	}
	return s
}

func (e *CLIError) Unwrap() error { return e.Cause }

func (e *CLIError) ExitCode() int {
	switch e.Code {
	case CodeAuthNotConfigured, CodeAuthTokenExpired:
		return ExitAuth
	case CodeAuthPermission:
		return ExitPermission
	case CodeMissingParam, CodeInvalidParam, CodeInvalidJSON, CodeInvalidPath, CodeInputTooLarge, CodeFileNotFound:
		return ExitValidation
	case CodeContentTruncated:
		return ExitAPI
	case CodeMCPServerError, CodeMCPToolError:
		return ExitAPI
	case CodeNetworkTimeout, CodeNetworkUnreachable:
		return ExitAPI
	case CodeResourceNotFound, CodeTableNotFound, CodeSheetNotFound, CodeFieldNotFound, CodeRecordNotFound:
		return ExitAPI
	case CodeLockTimeout, CodeUnclassified:
		return ExitInternal
	default:
		return ExitInternal
	}
}

// ToJSON returns a structured JSON representation for machine consumption.
func (e *CLIError) ToJSON() map[string]any {
	errMap := map[string]any{
		"code":      e.Code,
		"message":   e.Message,
		"exit_code": e.ExitCode(),
	}
	if e.Operation != "" {
		errMap["operation"] = e.Operation
	}
	if e.Suggestion != "" {
		errMap["suggestion"] = e.Suggestion
	}
	if e.Cause != nil {
		errMap["cause"] = e.Cause.Error()
	}
	return map[string]any{"error": errMap}
}

// PATError represents a PAT authorization failure that should be passed through
// to stderr as raw JSON without any CLI-layer wrapping.
type PATError struct {
	RawJSON string
}

func (e *PATError) Error() string { return e.RawJSON }

func (e *PATError) ExitCode() int { return ExitPermission }

func (e *PATError) RawStderr() string { return e.RawJSON }

// WrapError analyzes a raw error and wraps it with a friendly message.
// It classifies errors by pattern (network, auth, permission, resource,
// JSON parse, server error, file lock) and returns an appropriate CLIError.
func WrapError(err error) error {
	return WrapErrorWithOperation(err, "")
}

// WrapErrorWithOperation wraps an error with operation context for traceability.
func WrapErrorWithOperation(err error, operation string) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*CLIError); ok {
		return err
	}
	if _, ok := err.(*PATError); ok {
		return err
	}
	msg := err.Error()

	// File lock timeout (cross-process lock)
	if errContainsAny(msg, "timeout acquiring data lock", "timeout acquiring token lock") {
		return &CLIError{
			Code:       CodeLockTimeout,
			Message:    "Failed to acquire file lock",
			Suggestion: "Another dws process may be running. Wait for it to complete or check for stale lock files.",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Network: DNS resolution failure
	if errContainsAny(msg, "no such host", "dns") {
		return &CLIError{
			Code:       CodeNetworkUnreachable,
			Message:    "DNS resolution failed",
			Suggestion: "Check network connection and DNS settings",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Network: TLS handshake
	if errContainsAny(msg, "tls handshake", "certificate", "x509") {
		return &CLIError{
			Code:       CodeNetworkUnreachable,
			Message:    "TLS handshake failed",
			Suggestion: "Check system clock and CA certificates; corporate proxy may require custom CA",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Network: connection refused / unreachable
	if errContainsAny(msg, "connection refused", "dial tcp") {
		return &CLIError{
			Code:       CodeNetworkUnreachable,
			Message:    "Cannot connect to MCP server",
			Suggestion: "Check network connection or verify MCP server URL (dws auth status)",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Network: timeout
	if errContainsAny(msg, "timeout", "deadline exceeded", "context deadline") {
		return &CLIError{
			Code:       CodeNetworkTimeout,
			Message:    "Request timed out",
			Suggestion: "Retry later, or increase timeout with --timeout",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Token verification failure
	if errContainsAny(msg, "token验证失败", "Token验证失败", "USER_TOKEN_ILLEGAL") {
		return &CLIError{
			Code:       CodeAuthTokenExpired,
			Message:    "Token 已过期或验证失败",
			Suggestion: authExpiredSuggestion(),
			Operation:  operation,
			Cause:      err,
		}
	}

	// API key expired (e.g. apiKeyExpired from backend/gateway)
	if errContainsAny(msg, "apiKeyExpired", "api_key_expired", "ApiKeyExpired") {
		return &CLIError{
			Code:       CodeAuthTokenExpired,
			Message:    "API Key 已过期",
			Suggestion: authExpiredSuggestion(),
			Operation:  operation,
			Cause:      err,
		}
	}

	// Not logged in
	if errContainsAny(msg, "Missing service_id or access_key") {
		return &CLIError{
			Code:       CodeAuthNotConfigured,
			Message:    "当前未登录",
			Suggestion: notLoggedInSuggestion(),
			Operation:  operation,
			Cause:      err,
		}
	}

	// Permission denied
	if errContainsWord(msg, "403") || errContainsAny(msg, "forbidden", "permission") {
		suggestion := "Verify your account has permission for this resource"
		if errContainsAny(msg, "doc", "文档", "节点") {
			suggestion = "Contact document owner for access, or check if removed from collaborators"
		} else if errContainsAny(msg, "aitable", "table", "base", "record", "字段") {
			suggestion = "Run: dws aitable base list"
		} else if errContainsAny(msg, "chat", "群", "会话") {
			suggestion = "Verify you are a group member and the group still exists"
		}
		return &CLIError{
			Code:       CodeAuthPermission,
			Message:    "Permission denied",
			Suggestion: suggestion,
			Operation:  operation,
			Cause:      err,
		}
	}

	// 服务端已生成结构化错误信息（含操作索引、回滚告知等），
	// 直接透传，不进入下方的模式分类（避免用固定文案覆盖服务端原始信息）
	if operation == "sheet/batch_update" {
		return &CLIError{
			Code:      CodeMCPServerError,
			Message:   msg,
			Operation: operation,
			Cause:     err,
		}
	}

	// Resource not found
	if errContainsAny(msg, "not found", "不存在", "资源不存在") {
		suggestion := "Check if the resource exists or if your account has permission"
		code := CodeResourceNotFound
		if errContainsAny(msg, "table", "sheet", "dentry", "record", "base") {
			code = CodeTableNotFound
			suggestion = "Run: dws aitable doc search to verify BASE_ID, then sheet list for tables"
		}
		if errContainsAny(msg, "群不存在", "已解散", "被移出") {
			suggestion = "Group may have been disbanded or you were removed. Run: dws chat search"
		}
		if errContainsAny(msg, "文档", "doc", "节点") {
			suggestion = "Document may have been deleted or moved"
		}
		if errContainsAny(msg, "MCP不存在", "PARAM_ERROR") {
			return &CLIError{
				Code:       CodeMCPServerError,
				Message:    msg,
				Suggestion: "后端工具未注册或已下线；这不是参数格式问题。请升级到包含该工具注册的后端/静态端点版本，或改用当前可用替代命令。",
				Operation:  operation,
				Cause:      err,
			}
		}
		return &CLIError{
			Code:       code,
			Message:    "Requested resource not found",
			Suggestion: suggestion,
			Operation:  operation,
			Cause:      err,
		}
	}

	// JSON parse errors
	if errContainsAny(msg, "JSON 解析失败", "invalid character", "unexpected end of JSON input", "json:", "parsing tools/call response") {
		suggestion := "Check the JSON format of your input"
		if errContainsAny(msg, "add_base_record", "update_records") {
			suggestion = "Check --data JSON format, must be [{\"fields\":{...}}]"
		}
		if errContainsAny(msg, "fieldId", "字段") {
			suggestion += "; aitable record cells key must be fieldId, not field name. Run: dws aitable field get"
		}
		return &CLIError{
			Code:       CodeInvalidJSON,
			Message:    "Invalid JSON format",
			Suggestion: suggestion,
			Operation:  operation,
			Cause:      err,
		}
	}

	// Server errors (5xx)
	if errContainsAny(msg, "HTTP 5", "internal server error") || errContainsWord(msg, "500") || errContainsWord(msg, "502") || errContainsWord(msg, "503") {
		return &CLIError{
			Code:       CodeMCPServerError,
			Message:    "MCP server internal error",
			Suggestion: "Retry later. If persistent, contact admin",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Operation failed (catch-all for server-side "操作失败")
	if errContainsAny(msg, "操作失败") {
		suggestion := "Use --verbose for detailed logs"
		if errContainsAny(msg, "字段", "field", "类型") {
			suggestion = "Run: dws aitable field get to verify field type and fieldId"
		}
		return &CLIError{
			Code:       CodeMCPToolError,
			Message:    msg,
			Suggestion: suggestion,
			Operation:  operation,
			Cause:      err,
		}
	}

	// Generic tool/call failure
	if errContainsAny(msg, "tool", "调用失败") {
		return &CLIError{
			Code:       CodeMCPToolError,
			Message:    msg,
			Suggestion: "Use --verbose for detailed logs",
			Operation:  operation,
			Cause:      err,
		}
	}

	// Check for known business error text patterns
	if suggestion := suggestForBusinessErrorText(msg); suggestion != "" {
		return &CLIError{
			Code:       CodeMCPToolError,
			Message:    msg,
			Suggestion: suggestion,
			Operation:  operation,
			Cause:      err,
		}
	}

	return &CLIError{
		Code:       CodeUnclassified,
		Message:    msg,
		Suggestion: "Use --verbose for detailed error logs",
		Operation:  operation,
		Cause:      err,
	}
}

// IsAuthError returns true if the error is a CLIError with an auth-related code.
func IsAuthError(err error) bool {
	if cliErr, ok := err.(*CLIError); ok {
		return cliErr.Code == CodeAuthTokenExpired
	}
	return false
}

// suggestForBusinessErrorText returns a user-facing suggestion for known business
// error text patterns, or "" if no specific suggestion applies.
func suggestForBusinessErrorText(text string) string {
	switch {
	case strings.Contains(text, "搜索内容不能为空"):
		return "请提供非空搜索关键词: dws doc search --query \"关键词\"\n  若需浏览最近文档: dws doc list"
	case strings.Contains(text, "User has no permission to access this email"):
		return "请确认邮箱地址正确，查看可用邮箱: dws mail mailbox list"
	case strings.Contains(text, "频率超限") || strings.Contains(text, "rate limit"):
		return "API rate limit exceeded, wait a moment and retry"
	case strings.Contains(text, "未找到指定工具") || strings.Contains(text, "MCP不存在"):
		return "后端工具未注册或已下线；这不是参数格式问题。请升级到包含该工具注册的后端/静态端点版本，或改用当前可用替代命令。"
	case strings.Contains(text, "参数错误") || strings.Contains(text, "param error"):
		return "Check input parameters. Use --help for available flags"
	default:
		return ""
	}
}

// ClassifyToolResultContent checks a raw MCP tool result content map for
// DWS gateway auth errors and PAT permission error codes. This is used as the
// edition.Hooks.ClassifyToolResult callback so the framework's runner returns
// a typed error before its generic business-error classification.
//
// Check order matches ClassifyMCPResponseText: DWS gateway > PAT permission.
func ClassifyToolResultContent(content map[string]any) error {
	if _, ok := getDWSGatewayErrorCode(content); ok {
		raw, _ := json.Marshal(content)
		return &CLIError{
			Code:       CodeAuthTokenExpired,
			Message:    string(raw),
			Suggestion: authExpiredSuggestion(),
		}
	}
	for _, key := range []string{"code", "errorCode"} {
		if code, ok := content[key].(string); ok && patNoPermissionCodes[code] {
			return &PATError{RawJSON: cleanPATJSON(content, code)}
		}
	}
	return nil
}

// patNoPermissionCodes are PAT error codes that should be passed through.
var patNoPermissionCodes = map[string]bool{
	"PAT_NO_PERMISSION":             true,
	"PAT_LOW_RISK_NO_PERMISSION":    true,
	"PAT_MEDIUM_RISK_NO_PERMISSION": true,
	"PAT_HIGH_RISK_NO_PERMISSION":   true,
}

// ClassifyMCPResponseText classifies a text response returned by an MCP tool call.
// Returns a typed error for known gateway auth failures, PAT interceptions,
// and business-level errors embedded in HTTP-200 JSON bodies.
//
// Check order matters: DWS gateway > PAT permission > generic business error.
func ClassifyMCPResponseText(text string) error {
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return nil
	}

	if _, ok := getDWSGatewayErrorCode(body); ok {
		return &CLIError{
			Code:       CodeAuthTokenExpired,
			Message:    text,
			Suggestion: authExpiredSuggestion(),
		}
	}

	if isNotLoggedInError(body) {
		return &CLIError{
			Code:       CodeAuthNotConfigured,
			Message:    "当前未登录",
			Suggestion: notLoggedInSuggestion(),
		}
	}

	for _, key := range []string{"code", "errorCode"} {
		if code, ok := body[key].(string); ok && patNoPermissionCodes[code] {
			return &PATError{RawJSON: cleanPATJSON(body, code)}
		}
	}

	if isBusinessError(body) {
		return &CLIError{
			Code:       CodeMCPToolError,
			Message:    text,
			Suggestion: suggestForBusinessError(body),
		}
	}

	return nil
}

var patTopLevelStrip = map[string]bool{
	"success": true, "code": true, "errorCode": true, "error_code": true,
	"message": true, "error": true, "trace_id": true, "class": true,
}

func cleanPATJSON(body map[string]any, code string) string {
	out := map[string]any{
		"success": false,
		"code":    code,
	}
	if data, ok := body["data"]; ok {
		out["data"] = stripClassFields(data)
	} else {
		fallback := map[string]any{}
		for k, v := range body {
			if !patTopLevelStrip[k] {
				fallback[k] = v
			}
		}
		if len(fallback) > 0 {
			out["data"] = stripClassFields(fallback)
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"success":false,"code":"%s"}`, code)
	}
	return string(b)
}

func stripClassFields(v any) any {
	switch val := v.(type) {
	case map[string]any:
		clean := make(map[string]any, len(val))
		for k, item := range val {
			if k == "class" {
				continue
			}
			clean[k] = stripClassFields(item)
		}
		return clean
	case []any:
		clean := make([]any, len(val))
		for i, item := range val {
			clean[i] = stripClassFields(item)
		}
		return clean
	default:
		return v
	}
}

func errContainsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func errContainsWord(s string, substr string) bool {
	lower := strings.ToLower(s)
	sub := strings.ToLower(substr)
	for i := 0; i < len(lower); {
		idx := strings.Index(lower[i:], sub)
		if idx < 0 {
			return false
		}
		pos := i + idx
		leftOK := pos == 0 || !isAlnum(lower[pos-1])
		rightOK := pos+len(sub) >= len(lower) || !isAlnum(lower[pos+len(sub)])
		if leftOK && rightOK {
			return true
		}
		i = pos + 1
	}
	return false
}

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
