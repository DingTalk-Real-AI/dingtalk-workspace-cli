// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
)

func TestCrossPlatformCoverageDelegatorRedirectOriginBoundary(t *testing.T) {
	headers := map[string]string{
		requestmeta.DelegatorUserIDHeader: "user", requestmeta.DelegatorCorpIDHeader: "corp",
		requestmeta.DelegatorOpenDingtalkIDHeader: "open", requestmeta.DelegatorUIDHeader: "reserved",
	}
	var originURL string
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name := range headers {
			if r.Header.Get(name) != "" {
				t.Errorf("cross-origin leaked %s", name)
			}
		}
		http.Redirect(w, r, originURL+"/back", http.StatusTemporaryRedirect)
	}))
	defer away.Close()
	seen := map[string]int{}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		for name, value := range headers {
			want := value
			if r.URL.Path == "/back" {
				want = ""
			}
			if r.Header.Get(name) != want {
				t.Errorf("%s %s: got %q want %q", r.URL.Path, name, r.Header.Get(name), want)
			}
		}
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/same", http.StatusTemporaryRedirect)
		case "/same":
			http.Redirect(w, r, away.URL, http.StatusTemporaryRedirect)
		case "/back":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"content":{"success":true}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer origin.Close()
	originURL = origin.URL
	client := NewClient(origin.Client()).WithAuth("", headers)
	if _, err := client.CallTool(context.Background(), origin.URL+"/start", "fixture", nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/start", "/same", "/back"} {
		if seen[path] != 1 {
			t.Fatalf("%s visits=%d", path, seen[path])
		}
	}
	for name, value := range headers {
		if client.ExtraHeaders[name] != value {
			t.Fatal("redirect mutated original headers")
		}
	}
}

func TestCrossPlatformCoverageDelegatorTransportRetrySnapshot(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get(requestmeta.DelegatorOpenDingtalkIDHeader) != "original" {
			t.Error("retry lost original identity")
		}
		if calls == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"content":{"success":true}}}`)
	}))
	defer server.Close()
	headers := map[string]string{requestmeta.DelegatorOpenDingtalkIDHeader: "original"}
	client := NewClient(server.Client()).WithAuth("", headers)
	headers[requestmeta.DelegatorOpenDingtalkIDHeader] = "later"
	client.MaxRetries = 1
	client.RetryDelay = time.Millisecond
	client.RetryMaxDelay = time.Millisecond
	if _, err := client.CallTool(context.Background(), server.URL, "fixture", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
