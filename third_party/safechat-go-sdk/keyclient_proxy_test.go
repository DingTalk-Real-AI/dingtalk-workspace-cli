package safechat

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// http.ProxyFromEnvironment reads the environment once per process, so all
// settings are applied before the first lookup and one test covers them.
func TestNewKeyClientHonoursProxyEnvironment(t *testing.T) {
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "https_proxy", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
	t.Setenv("HTTPS_PROXY", "http://proxy.test:3128")
	t.Setenv("NO_PROXY", "bypass.example.com")

	kc := newKeyClient(Config{})
	tr, ok := kc.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", kc.httpClient.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("Transport.Proxy is nil: HTTP(S)_PROXY / NO_PROXY are ignored")
	}

	proxyFor := func(url string) string {
		t.Helper()
		got, err := tr.Proxy(httptest.NewRequest(http.MethodGet, url, nil))
		if err != nil {
			t.Fatalf("Proxy(%s) error: %v", url, err)
		}
		if got == nil {
			return ""
		}
		return got.String()
	}
	if got := proxyFor("https://keys.example.com/key"); got != "http://proxy.test:3128" {
		t.Errorf("HTTPS_PROXY not used: got %q", got)
	}
	if got := proxyFor("https://bypass.example.com/key"); got != "" {
		t.Errorf("NO_PROXY not honoured: got %q, want direct", got)
	}
}
