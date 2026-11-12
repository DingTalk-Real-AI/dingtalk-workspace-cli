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

package publishedmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

type operationFixture struct {
	calls []string
	ids   []string
	fail  bool
}

func (f *operationFixture) Tools(context.Context, string) (transport.ToolsListResult, error) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"requestId": map[string]any{"type": "string"}, "operationId": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}}}
	return transport.ToolsListResult{Tools: []transport.ToolDescriptor{{Name: "write", InputSchema: schema}, {Name: "operation_get", InputSchema: schema}}}, nil
}
func (f *operationFixture) InvokeValidated(_ context.Context, _, tool string, args map[string]any) (ValidatedInvocationResult, error) {
	f.calls = append(f.calls, tool)
	id, _ := args["requestId"].(string)
	f.ids = append(f.ids, id)
	if f.fail {
		return ValidatedInvocationResult{}, errors.New("lost response")
	}
	return ValidatedInvocationResult{Result: transport.ToolCallResult{Content: map[string]any{"success": true, "executionState": "applied", "resultJson": `{"applied":[{}]}`}}}, nil
}
func TestOperationRestartNeverResubmitsWrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "operation.json")
	f := &operationFixture{fail: true}
	args := map[string]any{"value": "one"}
	if _, err := InvokeOperation(context.Background(), f, "https://example.invalid", "service", "write", args, file); err == nil {
		t.Fatal("lost response hidden")
	}
	firstID := f.ids[0]
	if firstID == "" {
		t.Fatal("no host request ID")
	}
	f.fail = false
	if _, err := InvokeOperation(context.Background(), f, "https://example.invalid", "service", "write", args, file); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[1] != "operation_get" || f.ids[1] != firstID {
		t.Fatal("restart repeated write or changed identity")
	}
	if _, err := InvokeOperation(context.Background(), f, "https://example.invalid", "service", "write", map[string]any{"value": "two"}, file); err == nil {
		t.Fatal("different parameters reused")
	}
	if len(f.calls) != 2 {
		t.Fatal("conflict sent a call")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 1024 {
		t.Fatal("journal contains business payload")
	}
}
func TestOperationCorruptJournalNeverExecutes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "operation.json")
	if err := os.WriteFile(file, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &operationFixture{}
	if _, err := InvokeOperation(context.Background(), f, "https://example.invalid", "service", "write", map[string]any{}, file); err == nil {
		t.Fatal("corrupt journal accepted")
	}
	if len(f.calls) != 0 {
		t.Fatal("write resent")
	}
}

func TestCrossPlatformCoverageOperationHTTPPollPreservesCompletePartialResult(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	writes, lookups := 0, 0
	id := ""
	full := `{"text":"` + strings.Repeat("完整结果", 100) + `"}`
	fixture := &operationFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("missing caller auth")
		}
		var result any
		if rpc.Method == "tools/list" {
			list, _ := fixture.Tools(r.Context(), "")
			result = list
		} else {
			args := rpc.Params.Arguments
			if rpc.Params.Name == "write" {
				writes++
				id, _ = args["requestId"].(string)
				result = map[string]any{"content": map[string]any{"success": true, "executionState": "pending", "operationId": "3a696319-e8c4-4e17-9932-f8619b91e148"}}
			} else {
				lookups++
				if args["requestId"] != id {
					t.Error("lookup identity changed")
				}
				result = map[string]any{"content": map[string]any{"success": true, "executionState": "partial", "resultJson": full, "operationId": "3a696319-e8c4-4e17-9932-f8619b91e148"}}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	}))
	defer server.Close()
	base := transport.NewClient(server.Client())
	base.TrustedDomains = []string{"127.0.0.1"}
	client := New(base, "synthetic", nil)
	out, err := InvokeOperation(t.Context(), client, server.URL, "synthetic-service", "write", map[string]any{"value": "test"}, filepath.Join(t.TempDir(), "op.json"))
	if err != nil {
		t.Fatal(err)
	}
	if writes != 1 || lookups != 1 || out.Result.Content["executionState"] != "partial" || out.Result.Content["resultJson"] != full {
		t.Fatal("poll resubmitted or lost business output")
	}
}

// Expiry must return the pending operation, not send another business call.
type pendingOperationFixture struct{ operationFixture }

func (f *pendingOperationFixture) InvokeValidated(ctx context.Context, endpoint, tool string, args map[string]any) (ValidatedInvocationResult, error) {
	out, err := f.operationFixture.InvokeValidated(ctx, endpoint, tool, args)
	out.Result.Content = map[string]any{"success": true, "executionState": "pending", "operationId": "3a696319-e8c4-4e17-9932-f8619b91e148"}
	return out, err
}
func TestOperationDeadlinePreservesPendingWithoutResubmitting(t *testing.T) {
	f := &pendingOperationFixture{}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	out, err := InvokeOperation(ctx, f, "unused", "service", "write", map[string]any{}, filepath.Join(t.TempDir(), "op.json"))
	if err != nil || out.Result.Content["executionState"] != "pending" || len(f.calls) != 1 || f.calls[0] != "write" {
		t.Fatal("deadline lost pending or replayed write", err)
	}
}
