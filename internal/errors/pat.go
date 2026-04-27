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
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
)

// ExitCodePermission is the process exit code for PAT authorisation failures.
const ExitCodePermission = 4

// PATError represents a PAT (Personal Action Token) authorization failure
// that should be passed through to stderr as raw JSON without any CLI-layer
// wrapping. The host application (e.g. RewindDesktop) parses the JSON to
// display its own authorisation UI.
//
// When the payload includes data.uri, that URL is the authoritative
// server-provided authorization link. Hosts must treat it as opaque and open
// it verbatim instead of parsing and reconstructing it locally, because
// required parameters may live in query, encoded hash, or fragment sections.
type PATError struct {
	RawJSON string
}

func (e *PATError) Error() string { return e.RawJSON }

// ExitCode returns the documented exit code for PAT permission errors (4).
func (e *PATError) ExitCode() int { return ExitCodePermission }

// RawStderr returns the raw JSON to be written directly to stderr.
func (e *PATError) RawStderr() string { return e.RawJSON }

// patNoPermissionCodes are PAT error codes that should be passed through
// as transparent PATError without CLI-level wrapping.
var patNoPermissionCodes = map[string]bool{
	"PAT_NO_PERMISSION":             true,
	"PAT_LOW_RISK_NO_PERMISSION":    true,
	"PAT_MEDIUM_RISK_NO_PERMISSION": true,
	"PAT_HIGH_RISK_NO_PERMISSION":   true,
}

// patAuthRequiredCodes are error codes that trigger the PAT authorization
// flow (e.g. the server auto-created a CLI app and returned auth details).
var patAuthRequiredCodes = map[string]bool{
	"AGENT_CODE_NOT_EXISTS": true,
}

// IsPATError reports whether err is a *PATError.
func IsPATError(err error) bool {
	_, ok := err.(*PATError)
	return ok
}

// IsPATNoPermissionCode reports whether code is a known PAT permission error code.
func IsPATNoPermissionCode(code string) bool {
	return patNoPermissionCodes[code]
}

// ---- DWS gateway auth errors (shared between PAT & general auth) ----------

// dwsGatewayErrors is the set of DWS gateway-level auth error codes.
var dwsGatewayErrors = map[string]bool{
	"DWS_SERVICE_UNAUTHORIZED": true,
	"DWS_AUTH_SERVICE_FAILED":  true,
}

// getDWSGatewayErrorCode extracts a DWS gateway error code from errBody
// (supports both errorCode and error_code field names).
func getDWSGatewayErrorCode(errBody map[string]any) (string, bool) {
	for _, key := range []string{"errorCode", "error_code"} {
		if code, ok := errBody[key].(string); ok && dwsGatewayErrors[code] {
			return code, true
		}
	}
	return "", false
}

// isNotLoggedInError checks if the error body indicates missing authentication.
func isNotLoggedInError(body map[string]any) bool {
	if errMsg, ok := body["error"].(string); ok {
		if strings.Contains(errMsg, "Missing service_id or access_key") {
			return true
		}
	}
	return false
}

// isBusinessError checks if a parsed JSON body represents a business-level error.
func isBusinessError(body map[string]any) bool {
	if _, ok := body["error"].(string); ok {
		return true
	}
	if v, ok := body["success"].(bool); ok && !v {
		return true
	}
	if v, ok := body["success"].(string); ok && strings.EqualFold(v, "false") {
		return true
	}
	return false
}

// ---- Classification functions -----------------------------------------------

// ClassifyToolResultContent checks a raw MCP tool result content map for
// DWS gateway auth errors and PAT permission error codes.  This is intended
// for use as the edition.Hooks.ClassifyToolResult callback so the framework's
// runner returns a typed error before its generic business-error classification.
//
// Check order: DWS gateway auth > PAT permission.
func ClassifyToolResultContent(content map[string]any) error {
	if _, ok := getDWSGatewayErrorCode(content); ok {
		raw, _ := json.Marshal(content)
		return NewAuth(string(raw),
			WithReason("gateway_auth_expired"),
			WithHint(authExpiredHint()),
		)
	}
	for _, key := range []string{"code", "errorCode"} {
		if code, ok := content[key].(string); ok && patNoPermissionCodes[code] {
			return &PATError{RawJSON: cleanPATJSON(content, code)}
		}
	}
	return nil
}

// ClassifyMCPResponseText classifies a text response returned by an MCP tool call.
// Returns a typed error for known gateway auth failures, PAT interceptions,
// and business-level errors embedded in HTTP-200 JSON bodies.
//
// Check order: DWS gateway > PAT permission > generic business error.
func ClassifyMCPResponseText(text string) error {
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return nil
	}

	if _, ok := getDWSGatewayErrorCode(body); ok {
		return NewAuth(text,
			WithReason("gateway_auth_expired"),
			WithHint(authExpiredHint()),
		)
	}

	if isNotLoggedInError(body) {
		return NewAuth("当前未登录",
			WithReason("not_configured"),
			WithHint(notLoggedInHint()),
			WithActions("dws auth login"),
		)
	}

	for _, key := range []string{"code", "errorCode"} {
		if code, ok := body[key].(string); ok && patNoPermissionCodes[code] {
			return &PATError{RawJSON: cleanPATJSON(body, code)}
		}
	}

	if isBusinessError(body) {
		return NewAPI(text,
			WithReason("business_error"),
			WithHint(suggestForBusinessErrorText(body)),
		)
	}

	return nil
}

// ---- Hints -----------------------------------------------------------------

func authExpiredHint() string {
	return "Re-authenticate: dws auth login"
}

func notLoggedInHint() string {
	return "请先登录：dws auth login"
}

func suggestForBusinessErrorText(body map[string]any) string {
	msg := ""
	if v, ok := body["errorMsg"].(string); ok {
		msg = v
	} else if v, ok := body["message"].(string); ok {
		msg = v
	} else if v, ok := body["error"].(string); ok {
		msg = v
	}
	switch {
	case strings.Contains(msg, "搜索内容不能为空"):
		return "请提供非空搜索关键词: dws doc search --query \"关键词\""
	case strings.Contains(msg, "User has no permission to access this email"):
		return "请确认邮箱地址正确，查看可用邮箱: dws mail mailbox list"
	case strings.Contains(msg, "频率超限") || strings.Contains(msg, "rate limit"):
		return "API rate limit exceeded, wait a moment and retry"
	case strings.Contains(msg, "参数错误") || strings.Contains(msg, "param error"):
		return "Check input parameters. Use --help for available flags"
	default:
		return "MCP tool returned a business error; check parameters and refer to skill documentation."
	}
}

// ---- PAT JSON helpers ------------------------------------------------------

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
		// Keep data.uri exactly as returned by the service. Host consumers open
		// that link directly, so local normalization would risk dropping
		// parameters embedded in query/hash/fragment sections.
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

// ---- Runner adapter functions ------------------------------------------------
// These match the function signatures referenced by runner.go's PAT check
// framework (ClassifyPatAuthCheck / AsPatAuthCheckError).

// ClassifyPatAuthCheck is the open-source fallback that checks a tool-call
// Content map for PAT permission codes and auth-required codes.  Returns a
// non-nil *PATError when the content carries a recognised PAT/auth error.
func ClassifyPatAuthCheck(content map[string]any) *PATError {
	for _, key := range []string{"code", "errorCode"} {
		if code, ok := content[key].(string); ok {
			if patNoPermissionCodes[code] || patAuthRequiredCodes[code] {
				return &PATError{RawJSON: cleanPATJSON(content, code)}
			}
		}
	}
	return nil
}

// AsPatAuthCheckError extracts a *PATError from an error chain.
func AsPatAuthCheckError(err error) *PATError {
	var patErr *PATError
	if stderrors.As(err, &patErr) {
		return patErr
	}
	return nil
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
