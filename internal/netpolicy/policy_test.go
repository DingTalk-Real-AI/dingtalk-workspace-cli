// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package netpolicy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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

func TestCrossPlatformCoverageRoutingPrecedenceAndSnapshot(t *testing.T) {
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

func TestCrossPlatformCoverageBypassRules(t *testing.T) {
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

func TestCrossPlatformCoverageInvalidConfigurationFailsClosedAndRedacts(t *testing.T) {
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
func TestCrossPlatformCoverageCONNECTTLSAndNoDirectFallback(t *testing.T) {
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

func TestCrossPlatformCoverageStoreAndCAValidation(t *testing.T) {
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

func TestCrossPlatformCoverageRequestErrorsRedactSignedURLs(t *testing.T) {
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

func TestCrossPlatformCoverageEnvironmentRoutingEdges(t *testing.T) {
	for _, entry := range []struct{ key, value string }{
		{"DWS_PROXY", "auto"},
		{"DWS_NO_PROXY", "legacy.example"},
		{"DWS_CA_BUNDLE", ""},
		{"HTTP_PROXY", "proxy.example:3128"},
		{"HTTPS_PROXY", ""},
		{"https_proxy", ""},
		{"REQUEST_METHOD", "GET"},
	} {
		t.Setenv(entry.key, entry.value)
	}
	if got := SnapshotEnvironment()["HTTP_PROXY"]; got != "proxy.example:3128" {
		t.Fatalf("environment proxy = %q", got)
	}
	plain, _ := http.NewRequest(http.MethodGet, "http://external.example", nil)
	if _, err := EnvironmentProxy(plain); err == nil || !strings.Contains(err.Error(), "CGI") {
		t.Fatalf("CGI HTTP proxy = %v", err)
	}
	p, err := Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Explain(plain.URL); err == nil {
		t.Fatal("explain accepted HTTP_PROXY in a CGI process")
	}
	secure, _ := url.Parse("https://external.example")
	if got, err := p.Explain(secure); err != nil || got.Reason != "no proxy configured for target scheme" {
		t.Fatalf("HTTPS without HTTPS_PROXY = %#v, %v", got, err)
	}
	if _, err := p.Proxy(&http.Request{}); err == nil {
		t.Fatal("nil URL accepted")
	}
	if _, err := p.Explain(&url.URL{Scheme: "ftp", Host: "external.example"}); err == nil {
		t.Fatal("unsupported scheme accepted")
	}
	direct := mustPolicy(t, Settings{Proxy: str("direct")}, Overrides{}, nil)
	if got, err := direct.Explain(secure); err != nil || got.Reason != "direct mode" {
		t.Fatalf("direct mode = %#v, %v", got, err)
	}
	t.Setenv("HTTP_PROXY", "http://proxy.example/path")
	if _, err := Do(nil, plain); err == nil || !strings.Contains(err.Error(), "invalid HTTP_PROXY") {
		t.Fatalf("invalid HTTP_PROXY on standalone request = %v", err)
	}
	if _, err := EnvironmentProxy(plain); err == nil {
		t.Fatal("invalid environment silently accepted")
	}
	t.Setenv("DWS_PROXY", "direct")
	if proxy, err := EnvironmentProxy(plain); err != nil || proxy != nil {
		t.Fatalf("standalone direct proxy = %v, %v", proxy, err)
	}
	for _, bad := range []string{"host:0", "host:65536"} {
		if _, err := Resolve(Settings{NoProxy: &bad}, Overrides{}, nil); err == nil {
			t.Fatalf("accepted invalid bypass port %q", bad)
		}
	}
}

func TestCrossPlatformCoverageCorporateCAAndDefaultClient(t *testing.T) {
	if _, err := Resolve(Settings{CABundle: str("relative.pem")}, Overrides{}, nil); err == nil {
		t.Fatal("accepted relative CA bundle")
	}
	p := mustPolicy(t, Settings{Proxy: str("direct")}, Overrides{}, nil)
	p.roots = x509.NewCertPool()
	base := &http.Transport{TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
	got := p.Transport(base)
	if got.TLSClientConfig == nil || got.TLSClientConfig.RootCAs == nil || base.TLSClientConfig != nil {
		t.Fatal("TLS roots were not installed on the clone")
	}
	if p.Transport(base) != got {
		t.Fatal("invocation did not reuse its transport")
	}
	if b := bind(p, nil); b.Transport == nil {
		t.Fatal("default client transport missing")
	}
	tuned := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if transport := p.Transport(tuned); transport.TLSClientConfig.MinVersion != tls.VersionTLS12 || tuned.TLSClientConfig.RootCAs != nil {
		t.Fatal("CA override lost or mutated caller TLS settings")
	}
	ca := filepath.Join(t.TempDir(), "roots.pem")
	if err := os.WriteFile(ca, []byte("certificate fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("system trust unavailable")
	testseam.Swap(t, &systemCertPool, func() (*x509.CertPool, error) { return nil, fail })
	if _, err := Resolve(Settings{}, Overrides{}, Environment{"DWS_CA_BUNDLE": ca}); err == nil || !strings.Contains(err.Error(), "system CA roots") {
		t.Fatalf("missing system roots error = %v", err)
	}

	client := &http.Client{Transport: failingTransport{err: fail}}
	malformed := &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "["}}
	_, err := Do(client, malformed.WithContext(WithPolicy(context.Background(), p)))
	var targetError *url.Error
	if !errors.As(err, &targetError) || targetError.URL != "<redacted target>" || !errors.Is(err, fail) {
		t.Fatalf("malformed URL was not redacted: %v", err)
	}
}

func TestCrossPlatformCoverageNetworkStoreFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("loaded a directory as a configuration file")
	}
	if err := Save(dir, File{Version: 1}); err == nil {
		t.Fatal("saved over a directory")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	f := File{Version: 1, Profiles: map[string]Settings{"corp:user": {CABundle: str("")}}}
	if err := Save(dir, f); err != nil {
		t.Fatal(err)
	}
	if got := f.Settings("corp:user").CABundle; got == nil || *got != "" {
		t.Fatal("explicit CA override lost")
	}
	f.Default.Proxy = str("ftp://proxy:21")
	if err := Save(dir, f); err == nil {
		t.Fatal("saved invalid default proxy")
	}
	f.Default.Proxy = nil
	f.Profiles["corp:user"] = Settings{CABundle: str("relative.pem")}
	if err := Save(dir, f); err == nil {
		t.Fatal("saved invalid profile CA")
	}
	fail := errors.New("storage unavailable")
	t.Run("read failure", func(t *testing.T) {
		testseam.Swap(t, &readNetworkFile, func(string) ([]byte, error) { return nil, fail })
		if _, err := Load(dir); err == nil {
			t.Fatal("configuration read failure ignored")
		}
	})
	t.Run("serialization failure", func(t *testing.T) {
		testseam.Swap(t, &marshalNetworkFile, func(any, string, string) ([]byte, error) { return nil, fail })
		if err := Save(dir, File{Version: 1}); !errors.Is(err, fail) {
			t.Fatalf("serialization failure = %v", err)
		}
		if stored, err := Load(dir); err != nil || len(stored.Profiles) != 1 {
			t.Fatalf("serialization failure changed saved settings: %v, %v", stored, err)
		}
	})
}
