// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package localio

import (
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/netpolicy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// The origin is deliberately unreachable. Only the local proxy may be dialled;
// counting its CONNECT proves real routing rather than a non-nil Proxy field.
func TestCrossPlatformCoverageDownloadUsesEnvironmentProxy(t *testing.T) {
	var connects atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connects.Add(1)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	testseam.Swap(t, &secureDialContext, func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != u.Host {
			return nil, fmt.Errorf("direct origin dial forbidden")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	})
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	client := SecureHTTPClient()
	client.Timeout = time.Second
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://resource.invalid/file", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected proxy's CONNECT refusal")
	}
	if connects.Load() != 1 {
		t.Fatalf("proxy CONNECT count = %d, want 1 (direct dialing is forbidden)", connects.Load())
	}
}

func TestCrossPlatformCoverageDownloadThroughCONNECTAndNoProxy(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Resource-Key") != "test-key" {
			t.Error("resource header lost")
		}
		io.WriteString(w, "downloaded-through-selected-route")
	}))
	defer origin.Close()
	var connects atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			t.Error("expected CONNECT")
			w.WriteHeader(400)
			return
		}
		connects.Add(1)
		up, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			up.Close()
			return
		}
		defer conn.Close()
		defer up.Close()
		rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		rw.Flush()
		done := make(chan struct{})
		go func() { io.Copy(up, rw); up.Close(); close(done) }()
		io.Copy(conn, up)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	host := origin.Certificate().DNSNames[0]
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: origin.Certificate().Raw}), 0600)
	var direct atomic.Int32
	testseam.Swap(t, &secureDialContext, func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == proxyURL.Host {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		if address == net.JoinHostPort(host, "443") {
			direct.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, origin.Listener.Addr().String())
		}
		return nil, fmt.Errorf("unexpected dial forbidden")
	})
	proxyValue := proxy.URL
	for i, bypass := range []string{"", host} {
		p, err := netpolicy.Resolve(netpolicy.Settings{Proxy: &proxyValue, NoProxy: &bypass, CABundle: &ca}, netpolicy.Overrides{}, netpolicy.Environment{})
		if err != nil {
			t.Fatal(err)
		}
		base := t.TempDir()
		result, err := Download(netpolicy.WithPolicy(context.Background(), p), "https://"+host+"/resource?signature=test", DownloadOptions{BaseDir: base, Output: "resource.bin", Headers: map[string]string{"X-Resource-Key": "test-key"}})
		p.CloseIdleConnections()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(result.AbsolutePath)
		if err != nil || string(data) != "downloaded-through-selected-route" || result.SHA256 == "" {
			t.Fatalf("download result=%+v error=%v", result, err)
		}
		if connects.Load() != 1 || direct.Load() != int32(i) {
			t.Fatalf("connects=%d direct=%d", connects.Load(), direct.Load())
		}
	}
}
