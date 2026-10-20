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
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type geminiMCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"params"`
}

func geminiTestFunction(w http.ResponseWriter, name string, args map[string]any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": geminiContent{Role: "model", Parts: []geminiPart{{FunctionCall: &geminiFunctionCall{ID: "call-1", Name: name, Args: args}, ThoughtSignature: "signature"}}}}}})
}

func geminiSearchForwarder(t *testing.T, model http.HandlerFunc, opts connectAgentOptions) *geminiAPIForwarder {
	t.Helper()
	clearChannelEnv(t)
	t.Setenv("PARALLEL_API_KEY", "")
	t.Setenv("PARALLEL_API_TOKEN", "")
	t.Setenv("DWS_TRUSTED_DOMAINS", "")
	t.Setenv("GEMINI_API_KEY", "model-key")
	ts := httptest.NewServer(model)
	t.Cleanup(ts.Close)
	t.Setenv("GEMINI_API_BASE_URL", ts.URL)
	fwd, err := forwarderForChannel("gemini", "", opts)
	if err != nil {
		t.Fatal(err)
	}
	return fwd.(*geminiAPIForwarder)
}

func geminiMCPFixture(t *testing.T, f *geminiAPIForwarder, onCall func(context.Context, string, map[string]any) (map[string]any, error)) {
	t.Helper()
	if f.search == nil {
		t.Fatal("Parallel must be the unconfigured default")
	}
	f.search.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != parallelSearchEndpoint {
			t.Errorf("destination = %s", r.URL)
		}
		if r.UserAgent() != geminiSearchUserAgent {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		for _, key := range []string{"Authorization", "x-user-access-token", "x-goog-api-key"} {
			if r.Header.Get(key) != "" {
				t.Errorf("credential leaked: %s", key)
			}
		}
		if r.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Error("missing MCP accept types")
		}
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
			result = map[string]any{"tools": []any{map[string]any{"name": "web_search", "inputSchema": map[string]any{}}, map[string]any{"name": "web_fetch", "inputSchema": map[string]any{}}}}
		case "tools/call":
			value, err := onCall(r.Context(), req.Params.Name, req.Params.Arguments)
			if err != nil {
				return nil, err
			}
			result = value
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

func TestCrossPlatformCoverageGeminiParallelCaller(t *testing.T) {
	var modelCalls int
	source := "https://example.org/report"
	f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		if r.Header.Get("x-goog-api-key") != "model-key" {
			t.Error("Gemini key changed")
		}
		var req geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Tools) != 1 || len(req.Tools[0].FunctionDeclarations) != 2 {
			t.Error("missing search tools")
		}
		if modelCalls == 1 {
			geminiTestFunction(w, "web_search", map[string]any{"objective": "latest report", "search_queries": []string{"latest report"}, "session_id": "model-must-not-own-id"})
			return
		}
		prior := req.Contents[len(req.Contents)-2].Parts[0]
		if prior.ThoughtSignature != "signature" || prior.FunctionCall.ID != "call-1" {
			t.Error("lost Gemini tool-call signature or identity")
		}
		result := req.Contents[len(req.Contents)-1].Parts[0].FunctionResponse
		raw, _ := json.Marshal(result.Response)
		if !strings.Contains(string(raw), source) || result.ID != "call-1" {
			t.Errorf("model did not consume source result: %s", raw)
		}
		if modelCalls == 2 {
			geminiTestFunction(w, "web_fetch", map[string]any{"urls": []string{source}})
			return
		}
		if !strings.Contains(string(raw), "page evidence") {
			t.Error("missing fetched evidence")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": geminiContent{Parts: []geminiPart{{Text: "private thought", Thought: true}, {Text: "Report: page evidence [source](" + source + ")"}}}}}})
	}, connectAgentOptions{Yolo: true, Memory: true, Model: "gemini-test"})
	var sessionID string
	calls := 0
	geminiMCPFixture(t, f, func(_ context.Context, name string, args map[string]any) (map[string]any, error) {
		calls++
		id, _ := args["session_id"].(string)
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("not an opaque UUID: %q", id)
		}
		if sessionID != "" && sessionID != id {
			t.Error("search/fetch identity changed")
		}
		sessionID = id
		evidence := "search evidence"
		if name == "web_fetch" {
			evidence = "page evidence"
		}
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": evidence}}, "structuredContent": map[string]any{"url": source, "excerpt": evidence}}, nil
	})
	reply, err := f.forward(context.Background(), "chat-a", "Find the latest report")
	if err != nil || reply != "Report: page evidence [source]("+source+")" || modelCalls != 3 || calls != 2 {
		t.Fatalf("reply=%q err=%v model=%d tools=%d", reply, err, modelCalls, calls)
	}
}

func TestCrossPlatformCoverageGeminiParallelConversations(t *testing.T) {
	f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Contents) == 1 {
			geminiTestFunction(w, "web_search", map[string]any{"objective": req.Contents[0].Parts[0].Text, "search_queries": []string{"test query"}})
			return
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
	}, connectAgentOptions{Yolo: true, Memory: true})
	var mu sync.Mutex
	ids := map[string][]string{}
	geminiMCPFixture(t, f, func(_ context.Context, _ string, args map[string]any) (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		key := args["objective"].(string)
		ids[key] = append(ids[key], args["session_id"].(string))
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": "evidence"}}}, nil
	})
	run := func(conv string) {
		if _, err := f.forward(context.Background(), conv, conv); err != nil {
			t.Error(err)
		}
	}
	var wg sync.WaitGroup
	for _, conv := range []string{"a", "b"} {
		wg.Add(1)
		go func(c string) { defer wg.Done(); run(c) }(conv)
	}
	wg.Wait()
	run("a")
	run("b")
	if ids["a"][0] != ids["a"][1] || ids["b"][0] != ids["b"][1] || ids["a"][0] == ids["b"][0] {
		t.Fatalf("conversation identity crossed: %v", ids)
	}
	f.resetSession("a")
	run("a")
	if ids["a"][2] == ids["a"][0] {
		t.Fatal("/new reused search identity")
	}
	stateless, err := forwarderForChannel("gemini", "", connectAgentOptions{Yolo: true, Memory: false})
	if err != nil {
		t.Fatal(err)
	}
	next := stateless.(*geminiAPIForwarder)
	next.search.HTTPClient.Transport = f.search.HTTPClient.Transport
	f = next
	run("a")
	run("a")
	if ids["a"][3] == ids["a"][4] {
		t.Fatal("standalone runs shared an identity")
	}
}

func TestCrossPlatformCoverageGeminiParallelSettings(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		yolo          bool
		want          bool
	}{{"default", "", true, true}, {"saved", "parallel", true, true}, {"off", "off", true, false}, {"ask", "parallel", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			clearChannelEnv(t)
			t.Setenv("GEMINI_API_KEY", "key")
			t.Setenv("DWS_GEMINI_SEARCH", tc.setting)
			fwd, err := forwarderForChannel("gemini", "", connectAgentOptions{Yolo: tc.yolo, Model: "saved-model"})
			if err != nil {
				t.Fatal(err)
			}
			f := fwd.(*geminiAPIForwarder)
			if (f.search != nil) != tc.want || f.model != "saved-model" {
				t.Fatalf("setting was not preserved: %#v", f)
			}
		})
	}
	clearChannelEnv(t)
	t.Setenv("GEMINI_API_KEY", "key")
	t.Setenv("DWS_GEMINI_SEARCH", "invalid")
	if _, err := forwarderForChannel("gemini", "", connectAgentOptions{Yolo: true}); err == nil {
		t.Fatal("invalid provider silently accepted")
	}
	// Explicit custom commands keep their existing resolver and ignore Gemini settings.
	t.Setenv("DWS_AGENT_CMD", "echo custom")
	if f, err := forwarderForChannel("gemini", "", connectAgentOptions{}); err != nil {
		t.Fatal(err)
	} else if _, ok := f.(*geminiAPIForwarder); ok {
		t.Fatal("custom command overridden")
	}
}

func TestCrossPlatformCoverageGeminiParallelFailureAndBounds(t *testing.T) {
	for _, mode := range []string{"rpc-error", "quota", "malformed", "timeout", "deadline", "cancel", "disabled", "untrusted", "bad-url", "limit", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			modelCalls, toolCalls := 0, 0
			var observedIDs []string
			f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
				modelCalls++
				var req geminiGenerateContentRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if mode == "disabled" && len(req.Tools) != 0 {
					t.Error("tools advertised despite disabled search")
				}
				if modelCalls == 1 || mode == "limit" || (mode == "quota" && modelCalls == 2) {
					name := "web_search"
					args := map[string]any{"objective": "test", "search_queries": []string{"test query"}}
					if mode == "bad-url" {
						name = "web_fetch"
						args = map[string]any{"urls": []string{"file:///etc/passwd"}}
					}
					if mode == "unknown" {
						name = "execute_shell"
					}
					geminiTestFunction(w, name, args)
					return
				}
				response := req.Contents[len(req.Contents)-1].Parts[0].FunctionResponse
				if response.Response["error"] == nil {
					t.Errorf("tool failure was hidden: %#v", response)
				}
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"Search unavailable"}]}}]}`)
			}, connectAgentOptions{Yolo: mode != "disabled", Memory: true})
			if mode == "untrusted" {
				t.Setenv("DWS_TRUSTED_DOMAINS", "*.dingtalk.com")
				rebuilt, err := forwarderForChannel("gemini", "", connectAgentOptions{Yolo: true, Memory: true})
				if err != nil {
					t.Fatal(err)
				}
				f = rebuilt.(*geminiAPIForwarder)
			}
			if mode == "deadline" {
				f.timeout = 10 * time.Millisecond
			}
			if f.search != nil {
				geminiMCPFixture(t, f, func(ctx context.Context, _ string, args map[string]any) (map[string]any, error) {
					toolCalls++
					observedIDs = append(observedIDs, args["session_id"].(string))
					if mode == "deadline" {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					if mode == "cancel" {
						cancel()
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "private provider error"}}}, nil
				})
				original := f.search.HTTPClient.Transport
				if mode == "quota" || mode == "malformed" || mode == "timeout" {
					f.search.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						var req geminiMCPRequest
						raw, _ := io.ReadAll(r.Body)
						_ = json.Unmarshal(raw, &req)
						r.Body = io.NopCloser(strings.NewReader(string(raw)))
						if req.Method == "tools/call" {
							toolCalls++
							observedIDs = append(observedIDs, req.Params.Arguments["session_id"].(string))
							if mode == "timeout" {
								return nil, context.DeadlineExceeded
							}
							if mode == "quota" {
								return geminiResponse(429, `{"error":"free quota exceeded"}`, nil), nil
							}
							return geminiResponse(200, "invalid-json", nil), nil
						}
						return original.RoundTrip(r)
					})
				}
			}
			reply, err := f.forward(ctx, "same-chat", "test")
			switch mode {
			case "disabled", "limit":
				if err == nil {
					t.Fatalf("expected bounded error, got %q", reply)
				}
			case "deadline":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("deadline lost: %v", err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel lost: %v", err)
				}
			default:
				if err != nil || reply != "Search unavailable" {
					t.Fatalf("reply=%q err=%v", reply, err)
				}
			}
			if (mode == "disabled" || mode == "untrusted" || mode == "bad-url" || mode == "unknown") && toolCalls != 0 {
				t.Fatal("forbidden call executed")
			}
			if mode == "limit" && toolCalls != 6 {
				t.Fatalf("limit made %d tool calls", toolCalls)
			}
			if mode == "quota" && (toolCalls != 2 || observedIDs[0] != observedIDs[1]) {
				t.Fatalf("retry changed session: %v", observedIDs)
			}
		})
	}
}

// This is an actual Gemini-forwarder/MCP test with a scripted model, not live
// model inference or DingTalk delivery. It is opt-in to keep CI offline.
func TestGeminiParallelLive(t *testing.T) {
	if os.Getenv("DWS_PARALLEL_LIVE") != "1" {
		t.Skip("set DWS_PARALLEL_LIVE=1 for anonymous MCP validation")
	}
	var resultText string
	var quotaObserved atomic.Bool
	f := geminiSearchForwarder(t, func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch len(req.Contents) {
		case 1:
			geminiTestFunction(w, "web_search", map[string]any{"objective": "Find the official Go programming language website", "search_queries": []string{"Go programming language official website"}})
		case 3:
			result := req.Contents[2].Parts[0].FunctionResponse.Response
			if result["error"] != nil {
				if !quotaObserved.Load() {
					t.Error("live search failed without an observed explicit anonymous quota response")
				}
				resultText = "quota"
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"Search unavailable"}]}}]}`)
				return
			}
			geminiTestFunction(w, "web_fetch", map[string]any{"urls": []string{"https://go.dev/"}})
		default:
			raw, _ := json.Marshal(req.Contents[len(req.Contents)-1].Parts[0].FunctionResponse.Response)
			resultText = string(raw)
			_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": geminiContent{Parts: []geminiPart{{Text: resultText}}}}}})
		}
	}, connectAgentOptions{Yolo: true, Memory: true})
	original := f.search.HTTPClient.Transport
	f.search.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != parallelSearchEndpoint || r.UserAgent() != geminiSearchUserAgent || r.Header.Get("Authorization") != "" {
			t.Error("incorrect live destination, identity or auth")
		}
		reqRaw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(reqRaw)))
		var req geminiMCPRequest
		_ = json.Unmarshal(reqRaw, &req)
		resp, err := original.RoundTrip(r)
		if err != nil {
			t.Error(err)
			return nil, err
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(strings.NewReader(string(raw)))
		t.Logf("%s %s status=%d UTC=%s Retry-After=%q response=%s", req.Method, req.Params.Name, resp.StatusCode, time.Now().UTC().Format(time.RFC3339), resp.Header.Get("Retry-After"), string(raw))
		if err != nil {
			t.Error(err)
		}
		lower := strings.ToLower(string(raw))
		quota := (strings.Contains(lower, "quota") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit") || strings.Contains(lower, "usage limit")) &&
			!strings.Contains(lower, "unauthorized") && !strings.Contains(lower, "authentication") && !strings.Contains(lower, "invalid api key")
		var envelope struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Error   json.RawMessage `json:"error"`
			Result  struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		parseErr := json.Unmarshal(raw, &envelope)
		if quota && (resp.StatusCode == 429 || (resp.StatusCode == 200 && parseErr == nil && envelope.Result.IsError)) {
			quotaObserved.Store(true)
		} else if resp.StatusCode >= 400 {
			t.Errorf("unexpected live HTTP %d", resp.StatusCode)
		} else if req.Method != "notifications/initialized" && (parseErr != nil || envelope.JSONRPC != "2.0" || envelope.ID != req.ID || len(envelope.Error) > 0 || envelope.Result.IsError) {
			t.Error("invalid live MCP response")
		}

		return resp, nil
	})
	reply, err := f.forward(context.Background(), "live-test", "Find official Go sources")
	if err != nil {
		t.Fatal(err)
	}
	if quotaObserved.Load() {
		t.Log("Explicit anonymous MCP quota observed; deterministic caller coverage is required separately")
	}
	if resultText != "quota" && (!strings.Contains(reply, "go.dev") || !strings.Contains(reply, "Go")) {
		t.Fatalf("no useful consumed output: %s", reply)
	}
}
