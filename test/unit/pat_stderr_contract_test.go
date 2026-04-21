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

package unit_test

import (
	"encoding/json"
	"strings"
	"testing"

	errpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// D5 Smoke A — PAT stderr JSON is a single-line, json.Unmarshal-able
// payload for every code in the SSOT-frozen enum.
//
// This test is the CI-level guard for docs/pat/contract.md §2 ("stderr
// JSON MUST be single-line and directly unmarshal-able") and SSOT §3.3
// ("撞墙数据"). It sits in the public `test/unit` tree — deliberately
// separate from internal/errors/pat_test.go — so the contract stays
// defended even after internal refactors that might move the JSON
// assembly code to another package or collapse helpers into it.
//
// Wire path exercised: ClassifyPatAuthCheck covers BOTH error families
// the SSOT freezes — patNoPermissionCodes (PAT_NO_PERMISSION and the
// three risk-tier variants) and patAuthRequiredCodes
// (PAT_SCOPE_AUTH_REQUIRED, AGENT_CODE_NOT_EXISTS). Using this single
// entry point keeps the test format-agnostic to future code-churn in
// ClassifyMCPResponseText's branching while still covering the full
// SSOT code enum.
//
// What we assert for every case:
//  1. Non-nil *PATError is returned with ExitCode == 4.
//  2. RawJSON has NO '\n' characters (single-line invariant).
//  3. RawJSON is directly json.Unmarshal-able into a map.
//  4. Parsed body has success=false and code matching the fixture.
//  5. Required downstream data shape is present (requiredScopes for
//     PAT_*_NO_PERMISSION; missingScope for PAT_SCOPE_AUTH_REQUIRED).
func TestPATStderrJSON_SingleLineUnmarshalable(t *testing.T) {
	t.Parallel()

	type assertFn func(t *testing.T, parsed map[string]any)

	requiredScopesPresent := func(t *testing.T, parsed map[string]any) {
		t.Helper()
		data, ok := parsed["data"].(map[string]any)
		if !ok {
			t.Fatalf("parsed.data missing or wrong type: %v", parsed["data"])
		}
		scopes, ok := data["requiredScopes"].([]any)
		if !ok || len(scopes) == 0 {
			t.Fatalf("parsed.data.requiredScopes missing or empty: %v", data["requiredScopes"])
		}
		for i, s := range scopes {
			if _, ok := s.(string); !ok {
				t.Fatalf("parsed.data.requiredScopes[%d] is not a string: %v", i, s)
			}
		}
	}

	missingScopePresent := func(t *testing.T, parsed map[string]any) {
		t.Helper()
		data, ok := parsed["data"].(map[string]any)
		if !ok {
			t.Fatalf("parsed.data missing or wrong type: %v", parsed["data"])
		}
		scope, ok := data["missingScope"].(string)
		if !ok || scope == "" {
			t.Fatalf("parsed.data.missingScope missing or not a string: %v", data["missingScope"])
		}
	}

	agentCodeFieldPresent := func(t *testing.T, parsed map[string]any) {
		t.Helper()
		data, ok := parsed["data"].(map[string]any)
		if !ok {
			t.Fatalf("parsed.data missing or wrong type: %v", parsed["data"])
		}
		if _, ok := data["agentCode"].(string); !ok {
			t.Fatalf("parsed.data.agentCode missing or not a string: %v", data["agentCode"])
		}
	}

	cases := []struct {
		name    string
		code    string
		body    map[string]any
		extract assertFn
	}{
		{
			name: "PAT_NO_PERMISSION generic baseline",
			code: "PAT_NO_PERMISSION",
			body: map[string]any{
				"success": false,
				"code":    "PAT_NO_PERMISSION",
				"data": map[string]any{
					"requiredScopes": []any{"aitable.record:read"},
					"grantOptions":   []any{"session", "permanent"},
					"displayName":    "阅读多维表记录",
					"productName":    "AI 表格",
				},
			},
			extract: requiredScopesPresent,
		},
		{
			name: "PAT_LOW_RISK_NO_PERMISSION",
			code: "PAT_LOW_RISK_NO_PERMISSION",
			body: map[string]any{
				"success": false,
				"code":    "PAT_LOW_RISK_NO_PERMISSION",
				"data": map[string]any{
					"requiredScopes": []any{"aitable.record:read"},
					"grantOptions":   []any{"session", "permanent"},
					"displayName":    "阅读多维表记录",
					"productName":    "AI 表格",
				},
			},
			extract: requiredScopesPresent,
		},
		{
			name: "PAT_MEDIUM_RISK_NO_PERMISSION",
			code: "PAT_MEDIUM_RISK_NO_PERMISSION",
			body: map[string]any{
				"success": false,
				"code":    "PAT_MEDIUM_RISK_NO_PERMISSION",
				"data": map[string]any{
					"requiredScopes": []any{"chat.group:write"},
					"grantOptions":   []any{"session", "permanent"},
					"displayName":    "发送群消息",
					"productName":    "群聊",
				},
			},
			extract: requiredScopesPresent,
		},
		{
			name: "PAT_HIGH_RISK_NO_PERMISSION",
			code: "PAT_HIGH_RISK_NO_PERMISSION",
			body: map[string]any{
				"success": false,
				"code":    "PAT_HIGH_RISK_NO_PERMISSION",
				"data": map[string]any{
					"requiredScopes": []any{"finance.invoice:write"},
					"grantOptions":   []any{"once"},
					"displayName":    "开具发票",
					"productName":    "财务",
					"authRequestId":  "auth-req-xxxx",
				},
			},
			extract: requiredScopesPresent,
		},
		{
			name: "PAT_SCOPE_AUTH_REQUIRED",
			code: "PAT_SCOPE_AUTH_REQUIRED",
			body: map[string]any{
				"success": false,
				"code":    "PAT_SCOPE_AUTH_REQUIRED",
				"data": map[string]any{
					"missingScope": "Contact.User.Read",
					"hint":         "run `dws auth login --scope Contact.User.Read`",
				},
			},
			extract: missingScopePresent,
		},
		{
			name: "AGENT_CODE_NOT_EXISTS",
			code: "AGENT_CODE_NOT_EXISTS",
			body: map[string]any{
				"success": false,
				"code":    "AGENT_CODE_NOT_EXISTS",
				"data": map[string]any{
					"agentCode": "agt-unknown",
					"hint":      "请检查 DINGTALK_DWS_AGENTCODE / agent 注册",
				},
			},
			extract: agentCodeFieldPresent,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			patErr := errpkg.ClassifyPatAuthCheck(tc.body)
			if patErr == nil {
				t.Fatalf("ClassifyPatAuthCheck(%s) returned nil; expected *PATError", tc.code)
			}
			if patErr.ExitCode() != errpkg.ExitCodePermission {
				t.Fatalf("ExitCode() = %d, want %d (SSOT §2: PAT exit_code == 4)",
					patErr.ExitCode(), errpkg.ExitCodePermission)
			}

			raw := patErr.RawStderr()
			if raw == "" {
				t.Fatalf("RawStderr() is empty; expected single-line JSON")
			}
			if strings.ContainsAny(raw, "\n\r") {
				t.Fatalf("RawStderr() MUST be single-line (no \\n or \\r); got:\n%q", raw)
			}
			if strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t") {
				t.Fatalf("RawStderr() MUST not have leading whitespace; got: %q", raw)
			}

			var parsed map[string]any
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				t.Fatalf("json.Unmarshal(RawStderr()) error = %v; raw=%q", err, raw)
			}

			if success, ok := parsed["success"].(bool); !ok || success {
				t.Fatalf("parsed.success = %v, want false (SSOT §3.3)", parsed["success"])
			}
			gotCode, ok := parsed["code"].(string)
			if !ok {
				t.Fatalf("parsed.code missing or not a string: %v", parsed["code"])
			}
			if gotCode != tc.code {
				t.Fatalf("parsed.code = %q, want %q", gotCode, tc.code)
			}

			tc.extract(t, parsed)
		})
	}
}

// TestPATStderrJSON_CodeEnumFrozen pins the exact code enum that the
// SSOT / docs/pat/contract.md §2.3 freezes for PAT-family errors. If a
// future refactor accidentally renames a code (e.g. drops the risk-tier
// prefix) the classifier will stop producing a *PATError and this
// table-driven loop will fail with a clear message — preventing a
// silent break of the host integration contract.
func TestPATStderrJSON_CodeEnumFrozen(t *testing.T) {
	t.Parallel()

	frozen := []string{
		"PAT_NO_PERMISSION",
		"PAT_LOW_RISK_NO_PERMISSION",
		"PAT_MEDIUM_RISK_NO_PERMISSION",
		"PAT_HIGH_RISK_NO_PERMISSION",
		"PAT_SCOPE_AUTH_REQUIRED",
		"AGENT_CODE_NOT_EXISTS",
	}

	for _, code := range frozen {
		code := code
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			body := map[string]any{
				"success": false,
				"code":    code,
				"data":    map[string]any{"requiredScopes": []any{"stub.entity:read"}},
			}
			if patErr := errpkg.ClassifyPatAuthCheck(body); patErr == nil {
				t.Fatalf("ClassifyPatAuthCheck(%q) returned nil — code was removed from the SSOT-frozen enum", code)
			}
		})
	}
}
