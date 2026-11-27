// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDownloadResumeAndIdentity(t *testing.T) {
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	for _, mode := range []string{"resume", "ignored-range", "changed-etag", "precondition", "unsatisfiable", "no-etag"} {
		t.Run(mode, func(t *testing.T) {
			body := "0123456789abcdefghijklmnopqrstuvwxyz"
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Accept-Encoding") != "identity" {
					t.Error("representation encoding not pinned")
				}
				w.Header().Set("ETag", `"original"`)
				if calls == 1 {
					if r.Header.Get("Range") != "" {
						t.Error("new download must not resume an existing destination")
					}
					if mode == "no-etag" {
						w.Header().Del("ETag")
					}
					w.Header().Set("Content-Length", fmt.Sprint(len(body)))
					_, _ = w.Write([]byte(body[:10]))
					return
				}
				if calls == 2 && mode != "no-etag" {
					if r.Header.Get("Range") != "bytes=10-" || r.Header.Get("If-Match") != `"original"` {
						t.Errorf("resume headers: %v", r.Header)
					}
					switch mode {
					case "precondition":
						w.WriteHeader(412)
						return
					case "unsatisfiable":
						w.WriteHeader(416)
						return
					case "ignored-range":
						_, _ = w.Write([]byte(body))
						return
					case "changed-etag":
						w.Header().Set("ETag", `"changed"`)
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes 10-%d/%d", len(body)-1, len(body)))
					w.Header().Set("Content-Length", fmt.Sprint(len(body)-10))
					w.WriteHeader(206)
					_, _ = w.Write([]byte(body[10:]))
					return
				}
				if r.Header.Get("Range") != "" {
					t.Error("full restart still has a Range")
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			dest := filepath.Join(t.TempDir(), "asset")
			if err := os.WriteFile(dest, []byte("unrelated old destination"), 0600); err != nil {
				t.Fatal(err)
			}
			var final, total int64
			var events []DownloadEvent
			ctx := WithDownloadObserver(t.Context(), func(e DownloadEvent) { events = append(events, e) })
			cfg := &DownloadConfig{MaxRetries: 3, Timeout: time.Second, ProgressCallback: func(n, size int64) { final, total = n, size }}
			n, err := DownloadWithConfig(ctx, server.URL, dest, cfg)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != body || n != int64(len(body)) || final != n || total != n {
				t.Fatalf("content/size/progress mismatch: %q %d %d/%d", got, n, final, total)
			}
			if len(events) != calls || len(events) < 2 {
				t.Fatalf("attempt events=%v requests=%d", events, calls)
			}
			for i, event := range events {
				if event.Kind != "attempt" || event.Attempt != i+1 || event.MaxAttempts != 3 || event.Source == "" || (i > 0 && (event.Reason == "" || event.RetryDelay <= 0)) {
					t.Fatalf("incomplete attempt event: %+v", event)
				}
			}
			wantResume := int64(10)
			if mode == "no-etag" {
				wantResume = 0
			}
			if events[1].ResumeBytes != wantResume {
				t.Fatalf("reported resume offset=%d, want %d", events[1].ResumeBytes, wantResume)
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadFailurePreservesDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
	}))
	defer server.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "asset")
	if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := DownloadWithConfig(t.Context(), server.URL, dest, &DownloadConfig{MaxRetries: 1, Timeout: time.Second})
	if err == nil {
		t.Fatal("truncated download succeeded")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "original" {
		t.Fatal("failed download replaced destination")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("partial file leaked")
	}
}

func TestCrossPlatformCoverageDownloadRejectsOversizeAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 32))) }))
	defer server.Close()
	if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "asset"), &DownloadConfig{MaxRetries: 1, MaxBytes: 8}); err == nil {
		t.Fatal("oversize accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := DownloadWithConfig(ctx, server.URL, filepath.Join(t.TempDir(), "asset"), nil); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestCrossPlatformCoverageDownloadBodyStall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := &DownloadConfig{MaxRetries: 1, Timeout: time.Second, StallTimeout: 20 * time.Millisecond}
	_, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "asset"), cfg)
	if err == nil || !strings.Contains(err.Error(), "停滞") {
		t.Fatalf("body stall not classified: %v", err)
	}
}
