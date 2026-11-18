// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package netpolicy

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func str(v string) *string { return &v }
func mustPolicy(t *testing.T, s Settings, o Overrides, e Environment) *Policy {
	t.Helper()
	p, err := Resolve(s, o, e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.CloseIdleConnections)
	return p
}

func TestRoutingPrecedenceAndSnapshot(t *testing.T) {
	e := Environment{"HTTP_PROXY": "http://upper:80", "http_proxy": "http://lower:80", "HTTPS_PROXY": "http://secure:80", "DWS_NO_PROXY": "", "NO_PROXY": "ignored.example"}
	p := mustPolicy(t, Settings{Proxy: str("direct"), NoProxy: str("also-ignored.example")}, Overrides{Proxy: str("auto")}, e)
	e["HTTP_PROXY"] = "http://changed:80"
	for _, tc := range []struct{ url, want string }{
		{"http://example.com", "http://upper:80"}, {"https://example.com", "http://secure:80"},
		{"http://ignored.example", "http://upper:80"}, {"http://localhost:8080", ""}, {"http://127.0.0.2", ""}, {"https://[::1]", ""},
	} {
		u, _ := url.Parse(tc.url)
		got, err := p.Proxy(&http.Request{URL: u})
		if err != nil {
			t.Fatal(err)
		}
		if redactedURL(got) != tc.want {
			t.Errorf("%s = %s, want %s", tc.url, redactedURL(got), tc.want)
		}
	}
	for _, tc := range []struct {
		s            Settings
		o            Overrides
		e            Environment
		mode, source string
	}{
		{Settings{}, Overrides{}, Environment{}, "auto", "default"},
		{Settings{Proxy: str("direct")}, Overrides{}, Environment{}, "direct", "profile"},
		{Settings{Proxy: str("direct")}, Overrides{}, Environment{"DWS_PROXY": "http://proxy:80"}, "proxy", "DWS_PROXY"},
		{Settings{}, Overrides{Proxy: str("direct")}, Environment{"DWS_PROXY": "http://proxy:80"}, "direct", "flag"},
	} {
		p := mustPolicy(t, tc.s, tc.o, tc.e)
		if p.mode != tc.mode || p.source != tc.source {
			t.Errorf("mode/source = %s/%s", p.mode, p.source)
		}
	}
}

func TestBypassRules(t *testing.T) {
	p := mustPolicy(t, Settings{Proxy: str("http://proxy:80"), NoProxy: str("corp.example,.sub.example,port.example:443,10.0.0.0/8,192.168.1.2,[2001:db8::1]:443")}, Overrides{}, Environment{})
	for _, tc := range []struct {
		url    string
		direct bool
	}{
		{"https://corp.example", true}, {"https://a.corp.example", true}, {"https://evilcorp.example", false},
		{"https://sub.example", false}, {"https://a.sub.example", true}, {"https://port.example", true}, {"http://port.example", false},
		{"https://10.1.2.3", true}, {"https://192.168.1.2", true}, {"https://[2001:db8::1]", true}, {"http://[2001:db8::1]", false},
		// Matching a CIDR does not resolve this hostname to an intranet address.
		{"https://intranet.invalid", false},
	} {
		u, _ := url.Parse(tc.url)
		got, _, err := p.route(u)
		if err != nil || (got == nil) != tc.direct {
			t.Errorf("route %s = %v, %v", tc.url, got, err)
		}
	}
	p = mustPolicy(t, Settings{Proxy: str("http://proxy:80")}, Overrides{NoProxy: str("*")}, Environment{})
	u, _ := url.Parse("https://anywhere.example")
	got, _, _ := p.route(u)
	if got != nil {
		t.Fatal("NO_PROXY * ignored")
	}
}

func TestInvalidConfigurationFailsClosedAndRedacts(t *testing.T) {
	for _, raw := range []string{"", "ftp://proxy:21", "http://secret:password@proxy:bad", "http://proxy:65536", "http://proxy:0", "http://proxy/path", "http://proxy?password=secret", "http://", "://secret"} {
		p, err := Resolve(Settings{}, Overrides{Proxy: &raw}, Environment{})
		if err == nil || p != nil {
			t.Errorf("accepted invalid proxy")
		}
		if err != nil && (strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password")) {
			t.Fatal("error leaked URL")
		}
	}
	if _, err := Resolve(Settings{}, Overrides{}, Environment{"HTTPS_PROXY": "http://proxy/bad"}); err == nil {
		t.Fatal("invalid env proxy silently became direct")
	}
	p := mustPolicy(t, Settings{}, Overrides{}, Environment{"HTTPS_PROXY": "http://user:secret@proxy:80", "ALL_PROXY": "socks5://ignored:1"})
	u, _ := url.Parse("https://user:password@example.com/private?token=secret")
	e, err := p.Explain(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(e), "secret") || strings.Contains(fmt.Sprint(e), "private") || strings.Contains(fmt.Sprint(e), "user:") {
		t.Fatal("explanation leaked credentials or path")
	}
	if !e.ALLProxyIgnored {
		t.Fatal("missing ALL_PROXY diagnostic")
	}
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		mustPolicy(t, Settings{Proxy: str(scheme + "://proxy:1234")}, Overrides{}, Environment{})
	}
	if _, err := Resolve(Settings{NoProxy: str("http://secret")}, Overrides{}, Environment{}); err == nil {
		t.Fatal("accepted malformed bypass")
	}
	if _, err := Resolve(Settings{NoProxy: str("*invalid")}, Overrides{}, Environment{}); err == nil {
		t.Fatal("accepted malformed wildcard bypass")
	}
}

// Local CONNECT fixture forwards to a private TLS origin. No public network or
// real credentials are used. The origin hostname is only resolvable by the proxy.
func TestCONNECTTLSAndNoDirectFallback(t *testing.T) {
	var originHost atomic.Value
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHost.Store(r.Host)
		io.WriteString(w, "resource-bytes")
	}))
	defer origin.Close()
	var connects atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("method=%s", r.Method)
			w.WriteHeader(400)
			return
		}
		connects.Add(1)
		up, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			up.Close()
			return
		}
		defer client.Close()
		defer up.Close()
		buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buffer.Flush()
		done := make(chan struct{})
		go func() { io.Copy(up, buffer); up.Close(); close(done) }()
		io.Copy(client, up)
		client.Close()
		<-done
	}))
	defer proxy.Close()
	cert := origin.Certificate()
	host := cert.DNSNames[0]
	if err := cert.VerifyHostname(host); err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Settings{Proxy: str(proxy.URL), CABundle: &ca}, Overrides{}, Environment{})
	base := http.DefaultTransport.(*http.Transport).Clone()
	originalTLS := base.TLSClientConfig
	proxyURL, _ := url.Parse(proxy.URL)
	var direct atomic.Int32
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != proxyURL.Host {
			direct.Add(1)
			return nil, fmt.Errorf("direct forbidden")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	client := &http.Client{Transport: base}
	req, _ := http.NewRequestWithContext(WithPolicy(context.Background(), p), "GET", "https://"+host+"/resource", nil)
	resp, err := Do(client, req)
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	p.CloseIdleConnections()
	if string(bytes) != "resource-bytes" || connects.Load() != 1 || direct.Load() != 0 || originHost.Load() != host {
		t.Fatalf("body=%s connects=%d direct=%d host=%s", bytes, connects.Load(), direct.Load(), originHost.Load())
	}
	if base.Proxy == nil || base.TLSClientConfig != originalTLS {
		t.Fatal("caller transport mutated")
	}
	// The same untrusted origin must fail with a normal certificate-verifying
	// client when no corporate CA was explicitly supplied.
	untrusted := mustPolicy(t, Settings{Proxy: str(proxy.URL)}, Overrides{}, Environment{})
	badReq := req.WithContext(WithPolicy(context.Background(), untrusted))
	if r, err := Do(client, badReq); err == nil {
		r.Body.Close()
		t.Fatal("TLS validation disabled")
	}
	untrusted.CloseIdleConnections()
	// Shutdown the proxy: a selected proxy failure must not dial the origin.
	proxy.Close()
	if r, err := Do(client, req); err == nil {
		r.Body.Close()
		t.Fatal("closed proxy accepted")
	}
	if direct.Load() != 0 {
		t.Fatal("proxy error fell back to direct")
	}
}

func TestStoreAndCAValidation(t *testing.T) {
	dir := t.TempDir()
	f, err := Load(dir)
	if err != nil || f.Version != 1 {
		t.Fatal(f, err)
	}
	f.Default = Settings{Proxy: str("auto"), NoProxy: str("default.example")}
	f.Profiles = map[string]Settings{"corp:user": {Proxy: str("direct"), NoProxy: str("")}}
	if err := Save(dir, f); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *loaded.Settings("corp:user").NoProxy != "" || *loaded.Settings("other").Proxy != "auto" {
		t.Fatal("profile isolation/empty override failed")
	}
	loaded.Default.Proxy = str("http://user:secret@proxy:80")
	if err := Save(dir, loaded); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("persisted credentials or leaked password", err)
	}
	still, err := Load(dir)
	if err != nil || *still.Default.Proxy != "auto" {
		t.Fatal("failed save changed existing configuration")
	}
	if _, err := Resolve(Settings{CABundle: str(filepath.Join(dir, "missing"))}, Overrides{}, Environment{}); err == nil {
		t.Fatal("missing CA ignored")
	}
	bad := filepath.Join(dir, "bad.pem")
	os.WriteFile(bad, []byte("not a certificate"), 0600)
	if _, err := Resolve(Settings{CABundle: &bad}, Overrides{}, Environment{}); err == nil {
		t.Fatal("invalid CA ignored")
	}
	if err := os.WriteFile(filepath.Join(dir, "network.json"), []byte(`{"version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("future config version accepted")
	}
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestRequestErrorsRedactSignedURLs(t *testing.T) {
	sentinel := errors.New("connection refused")
	client := &http.Client{Transport: failingTransport{err: sentinel}}
	p := mustPolicy(t, Settings{Proxy: str("direct")}, Overrides{}, Environment{})
	req, _ := http.NewRequestWithContext(WithPolicy(context.Background(), p), "GET", "https://user:password@storage.example/private?signature=secret", nil)
	_, err := Do(client, req)
	if !errors.Is(err, sentinel) {
		t.Fatal("error identity lost", err)
	}
	for _, secret := range []string{"user", "password", "private", "signature", "secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("request URL leaked", err)
		}
	}
}
