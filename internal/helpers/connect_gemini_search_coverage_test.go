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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/uuid"
)

// Rejected arguments never reach the anonymous MCP endpoint, so every
// validation branch has to be proven on the pure argument builder.
func TestCrossPlatformCoverageGeminiSearchArgumentValidation(t *testing.T) {
	longObjective := strings.Repeat("网", 201)
	manyQueries := make([]any, 11)
	for i := range manyQueries {
		manyQueries[i] = "query"
	}
	for _, test := range []struct {
		name      string
		call      geminiFunctionCall
		wantError string
		wantKeys  []string
	}{
		{name: "unsupported tool", call: geminiFunctionCall{Name: "execute_shell"}, wantError: "unsupported web tool"},
		{
			name:      "search objective missing",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"search_queries": []any{"query"}}},
			wantError: "objective must be a nonempty string",
		},
		{
			name:      "search objective blank",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "   ", "search_queries": []any{"query"}}},
			wantError: "objective must be a nonempty string",
		},
		{
			name:      "search objective not a string",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": 7, "search_queries": []any{"query"}}},
			wantError: "objective must be a nonempty string",
		},
		{
			name:      "search queries missing",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "objective"}},
			wantError: "search_queries must be a nonempty string array (at most 10)",
		},
		{
			name:      "search queries empty",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "objective", "search_queries": []any{}}},
			wantError: "search_queries must be a nonempty string array (at most 10)",
		},
		{
			name:      "search queries above bound",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "objective", "search_queries": manyQueries}},
			wantError: "search_queries must be a nonempty string array (at most 10)",
		},
		{
			name:      "search query blank",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "objective", "search_queries": []any{" "}}},
			wantError: "search_queries contains an empty or non-string value",
		},
		{
			name:      "search query not a string",
			call:      geminiFunctionCall{Name: "web_search", Args: map[string]any{"objective": "objective", "search_queries": []any{42}}},
			wantError: "search_queries contains an empty or non-string value",
		},
		{
			name:      "fetch urls missing",
			call:      geminiFunctionCall{Name: "web_fetch", Args: map[string]any{}},
			wantError: "urls must be a nonempty string array (at most 20)",
		},
		{
			name:      "fetch url with credentials",
			call:      geminiFunctionCall{Name: "web_fetch", Args: map[string]any{"urls": []any{"https://user:secret@example.org/page"}}},
			wantError: "urls must contain HTTP/HTTPS URLs without embedded credentials",
		},
		{
			name:      "fetch objective not a string",
			call:      geminiFunctionCall{Name: "web_fetch", Args: map[string]any{"objective": 7, "urls": []any{"https://example.org/page"}}},
			wantError: "fetch objective must be a string of at most 200 characters",
		},
		{
			name:      "fetch objective above rune bound",
			call:      geminiFunctionCall{Name: "web_fetch", Args: map[string]any{"objective": longObjective, "urls": []any{"https://example.org/page"}}},
			wantError: "fetch objective must be a string of at most 200 characters",
		},
		{
			name: "fetch queries blank",
			call: geminiFunctionCall{Name: "web_fetch", Args: map[string]any{
				"urls": []any{"https://example.org/page"}, "search_queries": []any{""},
			}},
			wantError: "search_queries contains an empty or non-string value",
		},
		{
			name: "fetch accepts optional members",
			call: geminiFunctionCall{Name: "web_fetch", Args: map[string]any{
				"objective":      strings.Repeat("网", 200),
				"urls":           []any{"https://example.org/page", "http://example.org/other"},
				"search_queries": []any{"query"},
			}},
			wantKeys: []string{"objective", "search_queries", "session_id", "urls"},
		},
		{
			name: "search drops fetch-only members",
			call: geminiFunctionCall{Name: "web_search", Args: map[string]any{
				"objective": "objective", "search_queries": []any{"query"}, "urls": []any{"https://example.org/page"},
			}},
			wantKeys: []string{"objective", "search_queries", "session_id"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			args, err := geminiSearchArguments(&test.call, "session-1")
			if test.wantError != "" {
				if args != nil || err == nil || err.Error() != test.wantError {
					t.Fatalf("geminiSearchArguments() = %v, %v, want error %q", args, err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(args) != len(test.wantKeys) || args["session_id"] != "session-1" {
				t.Fatalf("geminiSearchArguments() = %v, want keys %v", args, test.wantKeys)
			}
			for _, key := range test.wantKeys {
				if _, ok := args[key]; !ok {
					t.Fatalf("geminiSearchArguments() dropped %q: %v", key, args)
				}
			}
		})
	}
}

// geminiMCPScript answers MCP discovery with an explicit tool list and every
// tools/call with one fixed result object, so discovery and result-shape
// branches can be exercised independently.
func geminiMCPScript(t *testing.T, f *geminiAPIForwarder, tools []string, callResult string) {
	t.Helper()
	f.search.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req geminiMCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "notifications/initialized":
			return geminiResponse(202, "", nil), nil
		case "tools/list":
			descriptors := make([]any, 0, len(tools))
			for _, name := range tools {
				descriptors = append(descriptors, map[string]any{"name": name, "inputSchema": map[string]any{}})
			}
			result = map[string]any{"tools": descriptors}
		case "tools/call":
			return geminiResponse(200, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, callResult), nil), nil
		default:
			return nil, fmt.Errorf("unexpected MCP method %s", req.Method)
		}
		raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		if err != nil {
			return nil, err
		}
		return geminiResponse(200, string(raw), nil), nil
	})
}

func TestCrossPlatformCoverageGeminiSearchDiscoveryAndResultShapes(t *testing.T) {
	for _, test := range []struct {
		name       string
		tools      []string
		callResult string
		want       string
	}{
		{
			name:       "incomplete tool set",
			tools:      []string{"web_search"},
			callResult: `{"content":[{"type":"text","text":"evidence"}]}`,
			want:       `"error":"Web search is unavailable.`,
		},
		{
			name:       "object content without structured content",
			tools:      []string{"web_search", "web_fetch"},
			callResult: `{"content":{"excerpt":"object evidence"}}`,
			want:       `"result":{"excerpt":"object evidence"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			var observed string
			f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
				modelCalls++
				var req geminiGenerateContentRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if modelCalls == 1 {
					geminiTestFunction(w, "web_search", map[string]any{"objective": "test", "search_queries": []string{"test query"}})
					return
				}
				raw, _ := json.Marshal(req.Contents[len(req.Contents)-1].Parts[0].FunctionResponse.Response)
				observed = string(raw)
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"done"}]}}]}`)
			}, connectAgentOptions{Yolo: true})
			geminiMCPScript(t, f, test.tools, test.callResult)
			reply, err := f.forward(context.Background(), "chat", "test")
			if err != nil || reply != "done" || modelCalls != 2 {
				t.Fatalf("reply=%q err=%v model=%d", reply, err, modelCalls)
			}
			if !strings.Contains(observed, test.want) {
				t.Fatalf("tool response = %s, want %q", observed, test.want)
			}
		})
	}
}

// Search is stateless without memory, so each standalone forward still carries
// one opaque identity instead of leaking the conversation key.
func TestCrossPlatformCoverageGeminiSearchStatelessIdentity(t *testing.T) {
	f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Contents) == 1 {
			geminiTestFunction(w, "web_search", map[string]any{"objective": "test", "search_queries": []string{"test query"}})
			return
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"done"}]}}]}`)
	}, connectAgentOptions{Yolo: true})
	if f.sessions != nil {
		t.Fatal("stateless forwarder kept conversation sessions")
	}
	var ids []string
	geminiMCPFixture(t, f, func(_ context.Context, _ string, args map[string]any) (map[string]any, error) {
		id := args["session_id"].(string)
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("not an opaque UUID: %q", id)
		}
		ids = append(ids, id)
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": "evidence"}}}, nil
	})
	// resetSession must stay a no-op rather than panic without a session store.
	f.resetSession("chat")
	for range 2 {
		if _, err := f.forward(context.Background(), "chat", "test"); err != nil {
			t.Fatal(err)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] || strings.Contains(ids[0], "chat") {
		t.Fatalf("stateless identities = %v", ids)
	}
}

func TestCrossPlatformCoverageGeminiGenerateContentTransportLimits(t *testing.T) {
	f := &geminiAPIForwarder{model: "model", apiKey: "key", baseURL: "https://example.test/v1beta", timeout: time.Second, httpClient: http.DefaultClient}
	endpoint, err := f.generateContentEndpoint()
	if err != nil {
		t.Fatal(err)
	}
	body := geminiGenerateContentRequest{Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: "text"}}}}}

	// DEL is rejected by the URL parser, so the request is never constructed.
	if _, err := f.generateContent(context.Background(), "https://example.test/\x7f", body); err == nil {
		t.Fatal("generateContent(invalid endpoint) error = nil")
	}

	for _, test := range []struct {
		name      string
		transport roundTripFunc
		want      string
	}{
		{
			name: "body read failure",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(iotest.ErrReader(errors.New("stream aborted")))}, nil
			},
			want: "stream aborted",
		},
		{
			name: "oversized body",
			transport: func(*http.Request) (*http.Response, error) {
				return geminiResponse(200, strings.Repeat("a", 4*1024*1024+1), nil), nil
			},
			want: "gemini API response exceeds 4 MiB",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := *f
			candidate.httpClient = &http.Client{Transport: test.transport}
			if _, err := candidate.generateContent(context.Background(), endpoint, body); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("generateContent() error = %v, want %q", err, test.want)
			}
		})
	}
}
