// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package safechat

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type callbackTransport func(*http.Request) (*http.Response, error)

func (f callbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestKeyClientEnvironmentAndHostTransport(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("NO_PROXY", "keys.internal")
	kc := newKeyClient(Config{})
	transport := kc.httpClient.Transport.(*http.Transport)
	req, _ := http.NewRequest("POST", "https://key.example/keys", nil)
	p, err := transport.Proxy(req)
	if err != nil || p == nil || p.Host != "127.0.0.1:7890" {
		t.Fatalf("proxy=%v err=%v", p, err)
	}
	req.URL, _ = url.Parse("https://keys.internal/keys")
	if p, err := transport.Proxy(req); err != nil || p != nil {
		t.Fatal("NO_PROXY ignored", p, err)
	}
	called := false
	kc = newKeyClient(Config{HTTPTransport: func(base *http.Transport) http.RoundTripper {
		if base.MaxIdleConns != 10 || !base.DisableCompression {
			t.Fatal("SDK tuning lost")
		}
		return callbackTransport(func(req *http.Request) (*http.Response, error) {
			called = true
			if req.Method != "POST" || req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("key request changed")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("key-response")), Header: http.Header{}}, nil
		})
	}})
	if got, err := kc.doKeyRequest("https://unreachable.invalid/keys", "param", "test-code"); err != nil || got != "key-response" || !called {
		t.Fatalf("response=%s called=%v err=%v", got, called, err)
	}
}

// A concrete proxy receives the SDK request without resolving the fake origin.
func TestKeyClientActualHTTPProxy(t *testing.T) {
	called := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Host != "unreachable.invalid" {
			t.Error("origin URL lost")
		}
		io.WriteString(w, "proxy-key-response")
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	kc := newKeyClient(Config{HTTPTransport: func(base *http.Transport) http.RoundTripper { base.Proxy = http.ProxyURL(u); return base }})
	got, err := kc.doKeyRequest("http://unreachable.invalid/key", "x", "code")
	if err != nil || got != "proxy-key-response" || !called {
		t.Fatal(got, err, called)
	}
}
