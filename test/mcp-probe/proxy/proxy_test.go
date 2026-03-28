package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyInterceptsToolCallsWithoutForwarding(t *testing.T) {
	t.Parallel()

	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      99,
			"result": map[string]any{
				"content": map[string]any{"forwarded": true},
			},
		})
	}))
	defer target.Close()

	p, err := New(target.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close()

	body := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"delete_records","arguments":{"baseId":"base-1"}}}`)
	resp, err := http.Post(p.URL(), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if forwarded != 0 {
		t.Fatalf("forwarded calls = %d, want 0", forwarded)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v; body=%s", err, string(data))
	}
	if payload["id"] != float64(3) {
		t.Fatalf("response id = %#v, want 3", payload["id"])
	}

	calls := p.DrainCalls()
	if len(calls) != 1 {
		t.Fatalf("DrainCalls() len = %d, want 1", len(calls))
	}
	if calls[0].ToolName != "delete_records" {
		t.Fatalf("ToolName = %q, want delete_records", calls[0].ToolName)
	}
	if calls[0].Arguments["baseId"] != "base-1" {
		t.Fatalf("Arguments = %#v, want baseId preserved", calls[0].Arguments)
	}
}
