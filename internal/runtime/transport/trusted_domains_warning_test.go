package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWildcardTrustedDomainsWarnsOnlyOnce(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "*")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"result": map[string]any{
				"content": map[string]any{"ok": true},
			},
		})
	}))
	defer server.Close()

	var stderr bytes.Buffer
	client := NewClient(server.Client())
	client.AuthToken = "test-token"
	client.Stderr = &stderr

	for i := 0; i < 2; i++ {
		if _, err := client.CallTool(context.Background(), server.URL, "ping", map[string]any{}); err != nil {
			t.Fatalf("CallTool() error = %v", err)
		}
	}

	output := stderr.String()
	if count := strings.Count(output, "DWS_TRUSTED_DOMAINS=*"); count != 1 {
		t.Fatalf("warning count = %d, want 1\noutput:\n%s", count, output)
	}
}
