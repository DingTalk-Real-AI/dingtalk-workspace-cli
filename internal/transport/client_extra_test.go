package transport

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestWithAuth_ClonesCorrectly(t *testing.T) {
	t.Parallel()
	original := NewClient(nil)
	original.AuthToken = "original-token"
	original.ExtraHeaders = map[string]string{"X-Old": "val"}
	original.TrustedDomains = []string{"*.example.com"}

	clone := original.WithAuth("new-token", map[string]string{"X-New": "val2"})

	if clone.AuthToken != "new-token" {
		t.Fatalf("clone token = %s, want new-token", clone.AuthToken)
	}
	if clone.ExtraHeaders["X-New"] != "val2" {
		t.Fatal("clone missing new header")
	}
	// Original unchanged
	if original.AuthToken != "original-token" {
		t.Fatal("original token was modified")
	}
	// Shared HTTP client
	if clone.HTTPClient != original.HTTPClient {
		t.Fatal("HTTP client should be shared")
	}
	// Trusted domains propagated
	if len(clone.TrustedDomains) != 1 || clone.TrustedDomains[0] != "*.example.com" {
		t.Fatal("trusted domains not propagated")
	}
}

func TestMatchDomain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host, pattern string
		want          bool
	}{
		{"api.dingtalk.com", "*.dingtalk.com", true},
		{"dingtalk.com", "*.dingtalk.com", true},
		{"api.dingtalk.com", "api.dingtalk.com", true},
		{"evil.com", "*.dingtalk.com", false},
		{"api.dingtalk.com", "", false},
		{"API.DINGTALK.COM", "*.dingtalk.com", true},
		{"localhost", "localhost", true},
		{"localhost", "*.localhost", true}, // matches host == pattern[2:]
	}
	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.pattern, func(t *testing.T) {
			if got := matchDomain(tt.host, tt.pattern); got != tt.want {
				t.Fatalf("matchDomain(%s, %s) = %v, want %v", tt.host, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"https://api.example.com/path", "https://api.example.com/path"},
		{"https://api.example.com/path?key=secret&token=abc", ""},
		{"not a url ://", "not a url ://"},
	}
	for _, tt := range tests {
		got := RedactURL(tt.input)
		if tt.want == "" {
			// Just check that secrets are redacted
			if strings.Contains(got, "secret") || strings.Contains(got, "abc") {
				t.Fatalf("RedactURL(%s) still contains secrets: %s", tt.input, got)
			}
		} else if got != tt.want {
			t.Fatalf("RedactURL(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeBearerToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"valid-token-123", "valid-token-123"},
		{" spaced ", "spaced"},
		{"", ""},
		{"has\x00null", ""},
		{"has\nnewline", ""},
		{"has\ttab", ""},
	}
	for _, tt := range tests {
		if got := sanitizeBearerToken(tt.input); got != tt.want {
			t.Fatalf("sanitizeBearerToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWarnWildcardDomains_OnlyOnce(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	c := NewClient(nil)
	c.Stderr = &buf

	c.warnWildcardDomains()
	c.warnWildcardDomains()
	c.warnWildcardDomains()

	lines := strings.Count(buf.String(), "[WARN]")
	if lines != 1 {
		t.Fatalf("expected 1 warning, got %d: %s", lines, buf.String())
	}
}

func TestTrustedDomainsList_FromField(t *testing.T) {
	t.Parallel()
	c := &Client{TrustedDomains: []string{"a.com", "b.com"}}
	got := c.trustedDomainsList()
	if len(got) != 2 || got[0] != "a.com" {
		t.Fatalf("expected field domains, got %v", got)
	}
}

func TestTrustedDomainsList_FromEnv(t *testing.T) {
	t.Setenv("DWS_TRUSTED_DOMAINS", "x.com,y.com")
	c := &Client{}
	got := c.trustedDomainsList()
	if len(got) != 2 || got[0] != "x.com" {
		t.Fatalf("expected env domains, got %v", got)
	}
}

func TestTrustedDomainsList_Default(t *testing.T) {
	t.Setenv("DWS_TRUSTED_DOMAINS", "")
	c := &Client{}
	got := c.trustedDomainsList()
	if len(got) == 0 {
		t.Fatal("expected default domains")
	}
}

func TestIsEndpointTrusted_WildcardTriggersWarning(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(nil)
	c.TrustedDomains = []string{"*"}
	c.Stderr = &buf
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "")

	result := c.isEndpointTrusted("https://evil.com/api")
	if !result {
		t.Fatal("wildcard should trust all HTTPS endpoints")
	}
	if !strings.Contains(buf.String(), "[WARN]") {
		t.Fatalf("expected warning, got: %s", buf.String())
	}
}

func TestIsEndpointTrusted_HTTPRejected(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "")
	c := NewClient(nil)
	c.TrustedDomains = []string{"*"}
	result := c.isEndpointTrusted("http://evil.com/api")
	if result {
		t.Fatal("plain HTTP to non-loopback should be rejected")
	}
}

func TestIsEndpointTrusted_HTTPLoopbackAllowed(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	c := NewClient(nil)
	c.TrustedDomains = []string{"*"}
	var buf bytes.Buffer
	c.Stderr = &buf
	result := c.isEndpointTrusted("http://127.0.0.1:8080/api")
	if !result {
		t.Fatal("HTTP loopback with env var should be allowed")
	}
}

func TestIsEndpointTrusted_DomainMatch(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "")
	c := NewClient(nil)
	c.TrustedDomains = []string{"*.dingtalk.com"}
	if !c.isEndpointTrusted("https://api.dingtalk.com/v1") {
		t.Fatal("should trust *.dingtalk.com")
	}
	if c.isEndpointTrusted("https://evil.com/v1") {
		t.Fatal("should not trust evil.com")
	}
}

func TestHttpStatusError(t *testing.T) {
	t.Parallel()
	codes := []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusInternalServerError,
	}
	for _, code := range codes {
		err := httpStatusError("tools/call", "https://api.example.com/mcp", code, nil, "", "")
		if err == nil {
			t.Fatalf("expected error for status %d", code)
		}
	}
}
