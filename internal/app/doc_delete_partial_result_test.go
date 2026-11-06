// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// Real transport decode -> runner business classification -> legacy error JSON.
// Removing the partial-error projection must lose these successful IDs again.
func TestCrossPlatformCoverageDocDeletePartialResultErrorJSON(t *testing.T) {
	fixture, err := os.ReadFile("testdata/doc-delete-partial-mcp-result.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"structured-and-text", "text-only", "mcp-is-error"} {
		t.Run(mode, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal(fixture, &raw); err != nil {
				t.Fatal(err)
			}
			reason := "business_error"
			wantMessage := "Some block deletion suggestions failed"
			if mode == "text-only" {
				delete(raw, "structuredContent")
			}
			if mode == "mcp-is-error" {
				raw["isError"] = true
				reason = "mcp_tool_error"
				wantMessage = raw["content"].([]any)[0].(map[string]any)["text"].(string)
			}
			err, calls := runDocDeletePartialFixture(t, "doc", "delete_document_block", raw)
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error=%T %v", err, err)
			}
			if apperrors.ExitCode(err) != 1 || typed.Category != apperrors.CategoryAPI || typed.Message != wantMessage || typed.Reason != reason || typed.ServerDiag.ServerErrorCode != "SOURCE_UNAVAILABLE" {
				t.Fatalf("failure contract changed: %#v", typed)
			}
			if calls != 1 {
				t.Fatalf("mutation calls=%d want1", calls)
			}
			var output bytes.Buffer
			if err := apperrors.PrintJSON(&output, err); err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"blockId": "partial-P,partial-Q", "deletedBlockIds": []any{"partial-P", "partial-Q"}, "notFoundBlockIds": []any{}, "logId": "trace-doc-delete-partial"}
			if !reflect.DeepEqual(wire.Error["details"], want) {
				t.Fatalf("partial details=%#v want=%#v", wire.Error["details"], want)
			}
			if wire.Error["code"] != float64(1) || wire.Error["reason"] != reason {
				t.Fatalf("public failure semantics=%#v", wire.Error)
			}
		})
	}
}

func TestCrossPlatformCoverageDocDeletePartialResultScope(t *testing.T) {
	for _, tc := range []struct {
		name, product, tool string
		payload             map[string]any
	}{
		{"other-product", "drive", "delete_document_block", map[string]any{"deletedBlockIds": []any{"P"}, "notFoundBlockIds": []any{}}},
		{"other-tool", "doc", "update_document_block", map[string]any{"deletedBlockIds": []any{"P"}, "notFoundBlockIds": []any{}}},
		{"scalar-error", "doc", "delete_document_block", map[string]any{"blockId": "P", "logId": "scalar-trace"}},
		{"malformed-partial-fields", "doc", "delete_document_block", map[string]any{"deletedBlockIds": []any{123}, "notFoundBlockIds": "missing", "blockId": "P"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.payload["success"] = false
			tc.payload["errorCode"] = "SOURCE_UNAVAILABLE"
			tc.payload["errorMsg"] = "Some block deletion suggestions failed"
			err, calls := runDocDeletePartialFixture(t, tc.product, tc.tool, map[string]any{"structuredContent": tc.payload, "content": []any{}})
			var typed *apperrors.Error
			if !errors.As(err, &typed) || len(typed.Details) != 0 || typed.Message != "Some block deletion suggestions failed" || typed.Reason != "business_error" || apperrors.ExitCode(err) != 1 || calls != 1 {
				t.Fatalf("unrelated/scalar failure changed: calls%d err%#v", calls, err)
			}
		})
	}
}

func TestCrossPlatformCoverageDocDeletePartialResultPreservesServerDiagnostics(t *testing.T) {
	retryable := false
	started := true
	unknown := false
	err := newServerFailureAPIError("Some block deletion suggestions failed", "business_error", "check results", "doc", "delete_document_block",
		apperrors.ServerDiagnostics{
			Reason: "partial_delete", Outcome: "partial_failure", NextCommand: "dws doc content get",
			ErrorCategory: "DOWNSTREAM", Unknown: &unknown,
			ServerRetryable: &retryable, ExecutionStarted: &started,
			Details: map[string]any{"checkpoint": "CP7", "error_category": "unselected"},
		},
		map[string]any{"deletedBlockIds": []any{"P"}, "notFoundBlockIds": []any{}, "blockId": "P,Q", "logId": "trace-partial"},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error=%T %v", err, err)
	}
	want := map[string]any{
		"checkpoint": "CP7", "outcome": "partial_failure", "nextCommand": "dws doc content get",
		"error_category": "DOWNSTREAM", "unknown": false,
		"deletedBlockIds": []string{"P"}, "notFoundBlockIds": []string{}, "blockId": "P,Q", "logId": "trace-partial",
	}
	if !reflect.DeepEqual(typed.Details, want) || typed.Reason != "partial_delete" || !typed.RetryableSet || typed.Retryable || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || apperrors.ExitCode(err) != 1 {
		t.Fatalf("merged failure contract lost details or semantics: %#v", typed)
	}
}

func TestCrossPlatformCoverageDocDeletePartialResultPreservesErrorCategory(t *testing.T) {
	err := newServerFailureAPIError("partial deletion", "business_error", "check results", "doc", "delete_document_block",
		apperrors.ServerDiagnostics{ErrorCategory: "DOWNSTREAM"},
		map[string]any{"deletedBlockIds": []any{"P"}},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Details["error_category"] != "DOWNSTREAM" || !reflect.DeepEqual(typed.Details["deletedBlockIds"], []string{"P"}) {
		t.Fatalf("error category or partial IDs lost: %#v", err)
	}
}

func TestCrossPlatformCoverageDocDeletePartialResultSuccessUnchanged(t *testing.T) {
	result := map[string]any{"structuredContent": map[string]any{"success": true, "deletedBlockIds": []any{"P"}, "notFoundBlockIds": []any{}}, "content": []any{}}
	err, calls := runDocDeletePartialFixture(t, "doc", "delete_document_block", result)
	if err != nil || calls != 1 {
		t.Fatalf("success changed: calls%d err%v", calls, err)
	}
}

func runDocDeletePartialFixture(t *testing.T, product, tool string, result map[string]any) (error, int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Method != "tools/call" || request.Params.Name != tool {
			t.Errorf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	client := transport.NewClient(server.Client())
	client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
	client.MaxRetries = 2
	runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "local-test-token"}}
	_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{CanonicalProduct: product, Tool: tool, Params: map[string]any{"nodeId": "isolated-test-node", "blockId": "partial-P,missing-block,partial-Q", "suggestion": true}})
	if client.MaxRetries != 2 {
		t.Fatal("base transport retry budget changed")
	}
	return err, calls
}
