package sender

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestSend_HostOnlyEndpoint 验证 endpoint 不带 scheme 时自动拼 https://。
func TestSend_HostOnlyEndpoint(t *testing.T) {
	// 这里只能验证函数不会因 scheme 缺失而崩溃，无法真实发到 https。
	// 实际的 endpoint 拼接逻辑在其它测试用真实 server 间接覆盖。
	err := Send("invalid.host.that.never.resolves", "pid=test", "")
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}

// TestSend_HTTPEndpoint 验证带 http:// 的 endpoint 不被强转 https。
func TestSend_HTTPEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// httptest 起的是 http://，如果代码强转 https 这里会失败
	if err := Send(server.URL, "pid=test", ""); err != nil {
		t.Fatalf("Send failed for http:// endpoint: %v", err)
	}
}

// TestSend_BodyShape 验证请求体结构是 {"gokey": ..., "gmkey": "EXP"}，
// 且 gokey 已经过 EncodeURIComponent。
func TestSend_BodyShape(t *testing.T) {
	var receivedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rawGokey := "pid=test&msg=hello world"
	if err := Send(server.URL, rawGokey, ""); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if got := receivedBody["gmkey"]; got != "EXP" {
		t.Errorf("gmkey = %q, want %q", got, "EXP")
	}

	gokey := receivedBody["gokey"]
	if gokey == "" {
		t.Fatal("gokey should not be empty")
	}
	// 经过一次 EncodeURIComponent 后，原始空格应被编为 %20
	if !strings.Contains(gokey, "%20") {
		t.Errorf("gokey should be URL-encoded, got %q", gokey)
	}
	// EncodeURIComponent 后的字符串再做一次 URL decode，应能还原成原始 gokey
	if decoded, err := url.QueryUnescape(gokey); err == nil && decoded != rawGokey {
		t.Errorf("decoded gokey = %q, want %q", decoded, rawGokey)
	}
}

// TestSend_Non2xxReturnsError 验证非 2xx 状态码返回带 HTTP {code} 的 error。
func TestSend_Non2xxReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := Send(server.URL, "pid=test", "")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %v, want contains 'HTTP 500'", err)
	}
}

// TestSend_NoRetry 是核心保证：验证 Send 只发一次请求，retry 已被删除。
//
// 如果未来有人不慎把 retry 循环加回来，这个测试会立刻失败。
func TestSend_NoRetry(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_ = Send(server.URL, "pid=test", "")

	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Errorf("Send made %d HTTP calls, want exactly 1 (retry must be removed)", got)
	}
}

// TestSend_TrailingSlash 验证 endpoint 末尾斜杠不会变成双斜杠。
func TestSend_TrailingSlash(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := Send(server.URL+"/", "pid=test", ""); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if requestPath != "/aes.1.1" {
		t.Errorf("request path = %q, want %q", requestPath, "/aes.1.1")
	}
}

func TestBuildURL_AppendsAESPath(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{"gm.mmstat.com", "https://gm.mmstat.com/aes.1.1"},
		{"sg.mmstat.com/alicloud", "https://sg.mmstat.com/alicloud/aes.1.1"},
		{"https://sg.mmstat.com", "https://sg.mmstat.com/aes.1.1"},
		{"https://sg.mmstat.com/aes.1.1", "https://sg.mmstat.com/aes.1.1"},
		{"https://sg.mmstat.com/aes.1.1/", "https://sg.mmstat.com/aes.1.1"},
	}

	for _, tt := range tests {
		if got := buildURL(tt.endpoint); got != tt.want {
			t.Errorf("buildURL(%q) = %q, want %q", tt.endpoint, got, tt.want)
		}
	}
}

// TestSend_TimeoutIs5s 验证 httpClient.Timeout 是 5 秒。
//
// 上报不能长时间阻塞业务流程，超时不能太长。
func TestSend_TimeoutIs5s(t *testing.T) {
	if got := httpClient.Timeout; got != 5*time.Second {
		t.Errorf("httpClient.Timeout = %v, want %v", got, 5*time.Second)
	}
}
