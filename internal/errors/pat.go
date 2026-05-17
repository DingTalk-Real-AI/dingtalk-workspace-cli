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
	"net/url"
	"strings"
	"sync"
)

// hostControlProvider returns the host-owned clawType for the current
// process, or empty string when CLI is in default (CLI-owned) mode.
// Injected lazily via SetHostControlProvider to avoid an
// internal/errors → internal/auth import cycle.
//
// Access is serialized by hostControlMu so that tests can swap the provider
// without triggering the race detector against parallel classifier callers.
var (
	hostControlMu       sync.RWMutex
	hostControlProvider func() string
	patBrowserMu        sync.RWMutex
	patBrowserProvider  func() bool
)

// SetHostControlProvider wires up the classifier's hostControl injection.
// It MUST be called once during CLI bootstrap (e.g. from internal/app
// init()) so that the first cleanPATJSON call observes a valid provider.
// Passing nil disables injection (useful for isolated tests).
func SetHostControlProvider(fn func() string) {
	hostControlMu.Lock()
	defer hostControlMu.Unlock()
	hostControlProvider = fn
}

// SetPATOpenBrowserProvider wires the PAT JSON serializer to the current
// browser policy. Passing nil restores the open-source fallback (true).
func SetPATOpenBrowserProvider(fn func() bool) {
	patBrowserMu.Lock()
	defer patBrowserMu.Unlock()
	patBrowserProvider = fn
}

// PATOpenBrowserValue returns the effective browser-open recommendation to
// embed in PAT JSON payloads. The open-source fallback is true to preserve
// historical behavior when no provider is wired.
func PATOpenBrowserValue() bool {
	patBrowserMu.RLock()
	provider := patBrowserProvider
	patBrowserMu.RUnlock()
	if provider == nil {
		return true
	}
	return provider()
}

// HostControlBlock returns the canonical hostControl map injected into
// PAT stderr JSON when the CLI is operating in host-owned mode, or nil
// when it is not. The returned map is safe for the caller to mutate
// because a new map is constructed on each call.
//
// callbackOwner is kept as a legacy compatibility key for hosts that adopted
// it before the contract converged on the hostControl single injection point.
func HostControlBlock() map[string]any {
	hostControlMu.RLock()
	provider := hostControlProvider
	hostControlMu.RUnlock()
	if provider == nil {
		return nil
	}
	claw := provider()
	if claw == "" {
		return nil
	}
	return map[string]any{
		"clawType":      claw,
		"callbackOwner": "host",
		"mode":          "host",
		"pollingOwner":  "host",
		"retryOwner":    "host",
	}
}

// ExitCodePermission is the process exit code for PAT authorisation failures.
const ExitCodePermission = 4

// PATError represents a PAT (Personal Action Token) authorization failure
// that should be passed through to stderr as raw JSON without any CLI-layer
// wrapping. The host application parses the JSON to display its own
// authorization UI. The wire schema is fixed: a single-line, directly
// json.Unmarshal-able payload of the form
// {"success":false,"code":<frozen enum>,"data":{...}}.
//
// When the payload includes data.uri, that URL is the authoritative
// server-provided authorization link. Hosts must treat it as opaque and open
// it verbatim instead of parsing and reconstructing it locally, because
// required parameters may live in query, encoded hash, or fragment sections.
// New hosts may prefer data.authorizationUrl when present; it keeps data.uri
// untouched while adding a copy/open-safe URL for DingTalk hash-route variants.
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
// flow (e.g. the server auto-created a CLI app and returned auth details,
// or the caller's OAuth token lacks a scope that must be re-acquired via
// `dws auth login --scope <missing>`).
//
// Keep keys in alphabetical order so diffs are stable. Both codes below are
// part of the frozen PAT-family selector and MUST be surfaced as *PATError
// (exit=4) so hosts can act on them:
//   - AGENT_CODE_NOT_EXISTS: data.agentCode tells the host which agent
//     registration is missing.
//   - PAT_SCOPE_AUTH_REQUIRED: data.missingScope tells the host which
//     OAuth scope to re-acquire via
//     `dws auth login --scope <data.missingScope>`.
var patAuthRequiredCodes = map[string]bool{
	"AGENT_CODE_NOT_EXISTS":   true,
	"PAT_SCOPE_AUTH_REQUIRED": true,
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

// errCodeKeys is the canonical priority order in which we look up
// upstream error code fields. Servers historically rotated between camel
// and snake case; we accept all three and pick the first that resolves to
// a recognised value.
var errCodeKeys = []string{"code", "errorCode", "error_code"}

// lookupCodeIn returns the first value in body[errCodeKeys] that is a
// non-empty string AND is a member of accept. Used by the PAT and DWS
// gateway classifiers, which differ only in their accept-set.
func lookupCodeIn(body map[string]any, accept map[string]bool) (string, bool) {
	for _, key := range errCodeKeys {
		if code, ok := body[key].(string); ok && accept[code] {
			return code, true
		}
	}
	return "", false
}

// getPATErrorCode extracts any PAT-intercept code from a map. PAT
// intercepts include both permission denials and auth-required selectors:
// callers on the text/tool-result path must preserve both families as
// *PATError so exit=4 + raw stderr JSON survives all the way to the host/CLI.
func getPATErrorCode(body map[string]any) (string, bool) {
	if code, ok := lookupCodeIn(body, patNoPermissionCodes); ok {
		return code, true
	}
	return lookupCodeIn(body, patAuthRequiredCodes)
}

// ---- DWS gateway auth errors (shared between PAT & general auth) ----------

// dwsGatewayErrors is the set of DWS gateway-level auth error codes.
var dwsGatewayErrors = map[string]bool{
	"DWS_SERVICE_UNAUTHORIZED": true,
	"DWS_AUTH_SERVICE_FAILED":  true,
}

// getDWSGatewayErrorCode extracts a DWS gateway error code from errBody.
func getDWSGatewayErrorCode(errBody map[string]any) (string, bool) {
	return lookupCodeIn(errBody, dwsGatewayErrors)
}

// isNotLoggedInError checks if the error body indicates missing authentication.
func isNotLoggedInError(body map[string]any) bool {
	for _, key := range []string{"error", "message", "errorMsg"} {
		errMsg, ok := body[key].(string)
		if !ok {
			continue
		}
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
	if code, ok := getPATErrorCode(content); ok {
		return &PATError{RawJSON: cleanPATJSON(content, code)}
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

	if code, ok := getPATErrorCode(body); ok {
		return &PATError{RawJSON: cleanPATJSON(body, code)}
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

// ApplyHostMutations writes the two stderr-JSON fields the host integration
// contract requires onto out["data"]:
//   - data.authorizationUrl / data.userCode: derived from data.uri when the
//     service returned a Device Code Flow PAT link.
//   - data.hostControl: present iff the CLI is in host-owned mode (i.e.
//     HostControlBlock returns non-nil); legacy data.callbacks is stripped
//     in the same pass so passive classifier and active retry paths stay
//     byte-for-byte aligned.
//   - data.openBrowser: always present; reflects the user's PAT browser
//     policy.
//
// Centralizing the two writes here is the single-injection invariant —
// any caller that produces a PAT-shaped stderr payload (cleanPATJSON,
// active-retry enrichers, scope-required builders) MUST go through this
// function instead of writing the fields directly. out["data"] is
// promoted to map[string]any if missing or of the wrong type.
func ApplyHostMutations(out map[string]any) {
	data, ok := out["data"].(map[string]any)
	if !ok || data == nil {
		data = map[string]any{}
		out["data"] = data
	}
	ApplyPATAuthorizationFields(data)
	if block := HostControlBlock(); block != nil {
		delete(data, "callbacks")
		data["hostControl"] = block
	}
	data["openBrowser"] = PATOpenBrowserValue()
}

// ApplyPATAuthorizationFields enriches PAT data with fields that host-owned
// clients need for Device Code Flow without forcing them to parse the browser
// URL themselves. data.uri is intentionally preserved verbatim by callers.
func ApplyPATAuthorizationFields(data map[string]any) {
	rawURI, ok := data["uri"].(string)
	if !ok || strings.TrimSpace(rawURI) == "" {
		return
	}

	data["authorizationUrl"] = PATAuthorizationURL(rawURI)
	routeQuery := PATAuthorizationRouteValues(rawURI)
	if routeQuery.Get("flowId") != "" && stringField(data, "flowId") == "" {
		data["flowId"] = routeQuery.Get("flowId")
	}
	if routeQuery.Get("userCode") != "" && stringField(data, "userCode") == "" {
		data["userCode"] = routeQuery.Get("userCode")
	}
}

func stringField(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

// PATAuthorizationURL returns the best URL for hosts to open or show to users.
// It keeps non-PAT URLs unchanged. For DingTalk's /fe/old Device Code Flow
// route, it returns a canonical hash-query URL and strips the duplicate decoded
// fragment so hosts do not lose userCode when launching through shell-like
// openers that split on a naked '&' in the fragment.
func PATAuthorizationURL(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return ""
	}

	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURI
	}
	if !strings.HasSuffix(parsed.Path, "/fe/old") {
		return rawURI
	}

	routeQuery := patAuthorizationRouteQuery(parsed)
	if routeQuery.Get("flowId") == "" || routeQuery.Get("userCode") == "" {
		return rawURI
	}

	route := "/personalAuthorization?" + routeQuery.Encode()

	next := *parsed
	query := next.Query()
	query.Set("hash", "#"+route)
	next.RawQuery = query.Encode()
	next.Fragment = ""
	next.RawFragment = ""
	return next.String()
}

// PATAuthorizationRouteValues extracts the flow route parameters from supported
// PAT Device Code Flow URLs.
func PATAuthorizationRouteValues(rawURI string) url.Values {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return nil
	}
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return nil
	}
	return patAuthorizationRouteQuery(parsed)
}

func patAuthorizationRouteQuery(parsed *url.URL) url.Values {
	candidates := []string{
		parsed.Fragment,
		parsed.RawFragment,
		parsed.Query().Get("hash"),
	}
	for _, candidate := range candidates {
		if values := parsePersonalAuthorizationRouteQuery(candidate); values.Get("flowId") != "" && values.Get("userCode") != "" {
			return values
		}
		if decoded, err := url.QueryUnescape(candidate); err == nil && decoded != candidate {
			if values := parsePersonalAuthorizationRouteQuery(decoded); values.Get("flowId") != "" && values.Get("userCode") != "" {
				return values
			}
		}
	}
	return nil
}

func parsePersonalAuthorizationRouteQuery(route string) url.Values {
	route = strings.TrimSpace(route)
	route = strings.TrimPrefix(route, "#")
	idx := strings.Index(route, "personalAuthorization?")
	if idx < 0 {
		return nil
	}
	rawQuery := route[idx+len("personalAuthorization?"):]
	if cut := strings.IndexAny(rawQuery, "?#"); cut >= 0 {
		rawQuery = rawQuery[:cut]
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil
	}
	return values
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
	ApplyHostMutations(out)

	// stderr JSON MUST be a single-line, directly json.Unmarshal-able
	// payload — pretty-printing would break naïve host parsers that read
	// stderr line-by-line and fail on leading whitespace.
	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"code":"%s"}`, code)
	}
	return string(b)
}

// ---- Runner adapter functions ------------------------------------------------
// These match the function signatures referenced by runner.go's PAT check
// framework (ClassifyPatAuthCheck / AsPatAuthCheckError).

// ClassifyPatAuthCheck is the open-source fallback that checks a tool-call
// Content map for PAT permission codes and auth-required codes. Returns a
// non-nil *PATError when the content carries a recognised PAT/auth error.
func ClassifyPatAuthCheck(content map[string]any) *PATError {
	if code, ok := getPATErrorCode(content); ok {
		return &PATError{RawJSON: cleanPATJSON(content, code)}
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
