// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/netpolicy"
)

func TestResourceDownloadsUseInvocationProxy(t *testing.T) {
	var connects, direct atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			t.Error("expected CONNECT")
		}
		connects.Add(1)
		w.WriteHeader(502)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	value := proxy.URL
	p, err := netpolicy.Resolve(netpolicy.Settings{Proxy: &value}, netpolicy.Overrides{}, netpolicy.Environment{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseIdleConnections()
	client := localio.SecureHTTPClient()
	client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != proxyURL.Host {
			direct.Add(1)
			return nil, fmt.Errorf("origin direct forbidden")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	ctx := netpolicy.WithPolicy(context.Background(), p)
	for i, partSize := range []int64{0, 1024} {
		_, err := downloadWithRecovery(ctx, client, "https://unreachable.invalid/resource", nil, filepath.Join(t.TempDir(), "resource"), false, partSize, 0, 0, nil)
		if err == nil || connects.Load() != int32(i+1) || direct.Load() != 0 {
			t.Fatalf("partSize=%d CONNECT=%d direct=%d err=%v", partSize, connects.Load(), direct.Load(), err)
		}
	}
}
