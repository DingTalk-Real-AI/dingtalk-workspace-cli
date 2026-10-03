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

package aitableprotocol

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageSQLSheetKeyPolicy(t *testing.T) {
	for _, product := range []string{"aitable", "other"} {
		for _, tool := range []string{"create_sql_sheet_api_key", "list_sql_sheet_api_keys", "revoke_sql_sheet_api_key", "create_base"} {
			want := product == "aitable" && tool != "create_base"
			if IsSQLSheetKeyOperation(product, tool) != want {
				t.Fatal("incorrect credential policy")
			}
		}
	}
}

// The protocol policy preserves actionable codes without forwarding remote messages.
func TestCrossPlatformCoverageSQLSheetKeyErrors(t *testing.T) {
	for _, code := range []string{"403", "404", "600", "INVALID_PARAM", "INVALID_SQL_SHEET_API_KEY_REQUEST", "SQL_SHEET_API_KEY_ALREADY_EXISTS", "RATE_LIMIT_EXCEEDED", "INTERNAL_ERROR", "INVALID_SQL_SHEET_API_KEY_RESPONSE", "SQL_SHEET_API_KEY_OPERATION_FAILED", "UNKNOWN"} {
		t.Run(code, func(t *testing.T) {
			body := map[string]any{"status": "error", "error": map[string]any{"code": code, "message": "TEST_SECRET"}}
			err := SQLSheetKeyResponseError("create_sql_sheet_api_key", body, false)
			var typed *apperrors.Error
			want := code
			if code == "UNKNOWN" {
				want = "SQL_SHEET_API_KEY_OPERATION_FAILED"
			}
			if !errors.As(err, &typed) || typed.Reason != want || typed.Retryable || strings.Contains(err.Error(), "TEST_SECRET") {
				t.Fatalf("incorrect safe error: %v", err)
			}
		})
	}
	if err := SQLSheetKeyResponseError("list_sql_sheet_api_keys", map[string]any{"status": "success"}, false); err != nil {
		t.Fatal(err)
	}
	if err := SQLSheetKeyResponseError("list_sql_sheet_api_keys", map[string]any{"status": "success"}, true); err == nil {
		t.Fatal("MCP isError was ignored")
	}
	for _, code := range []string{"not_authenticated", "sql_sheet_api_key_single_profile_required", "403"} {
		err := SQLSheetKeyCallError("list_sql_sheet_api_keys", apperrors.NewAPI("TEST_SECRET", apperrors.WithReason(code)))
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != code || strings.Contains(err.Error(), "TEST_SECRET") {
			t.Fatalf("caller error lost code: %v", err)
		}
	}
	err := SQLSheetKeyCallError("create_sql_sheet_api_key", errors.New("TEST_SECRET"))
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable || typed.ExecutionStarted != nil || strings.Contains(err.Error(), "TEST_SECRET") {
		t.Fatalf("uncertain call: %v", err)
	}
	err = SQLSheetKeyCallError("create_sql_sheet_api_key", apperrors.NewValidation("TEST_SECRET", apperrors.WithOrigin("client"), apperrors.WithExecutionStarted(false)))
	if !errors.As(err, &typed) || typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.FailureStage != "invocation_setup" {
		t.Fatalf("setup failure: %v", err)
	}
}

// Only an absent, null or zero-member error can accompany business success.
func TestCrossPlatformCoverageSQLSheetKeyResponseErrorShapes(t *testing.T) {
	for _, tool := range []string{"create_sql_sheet_api_key", "list_sql_sheet_api_keys", "revoke_sql_sheet_api_key"} {
		for _, tc := range []struct {
			name        string
			detail      any
			wantSuccess bool
		}{
			{"missing", nil, true},
			{"null", nil, true},
			{"empty object", map[string]any{}, true},
			{"empty code", map[string]any{"code": ""}, false},
			{"empty message", map[string]any{"message": ""}, false},
			{"retryable false", map[string]any{"retryable": false}, false},
			{"unknown null", map[string]any{"unknown": nil}, false},
			{"array", []any{}, false},
			{"string", "", false},
			{"boolean", false, false},
			{"number", 0, false},
		} {
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				body := map[string]any{"status": "success"}
				if tc.name != "missing" {
					body["error"] = tc.detail
				}
				if err := SQLSheetKeyResponseError(tool, body, false); (err == nil) != tc.wantSuccess {
					t.Fatalf("success=%t, error=%v", tc.wantSuccess, err)
				}
				if err := SQLSheetKeyResponseError(tool, body, true); err == nil {
					t.Fatal("MCP error accepted")
				}
				body["status"] = "error"
				if err := SQLSheetKeyResponseError(tool, body, false); err == nil {
					t.Fatal("business error accepted")
				}
			})
		}
	}
}
