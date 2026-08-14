// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type downloadRoundTripFunc func(*http.Request) (*http.Response, error)

func (f downloadRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDownload_Success(t *testing.T) {
	body := "hello world binary content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write([]byte(body))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "downloaded")
	n, err := Download(server.URL+"/file.tar.gz", dest)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("Download() = %d bytes, want %d", n, len(body))
	}

	got, _ := os.ReadFile(dest)
	if string(got) != body {
		t.Errorf("file content = %q, want %q", string(got), body)
	}
}

func TestDownload_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "notfound")
	_, err := Download(server.URL+"/missing", dest)
	if err == nil {
		t.Fatal("Download() expected error for 404")
	}
}

func TestDownload_HTTP500_RetriesAndFails(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "err")
	cfg := &DownloadConfig{
		MaxRetries: 2,
		Timeout:    5 * time.Second,
	}
	_, err := DownloadWithConfig(context.Background(), server.URL+"/fail", dest, cfg)
	if err == nil {
		t.Fatal("expected error after retries")
	}

	// Transient 5xx responses are retried, matching the lark-cli download path.
	got := atomic.LoadInt32(&attempts)
	if got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestDownloadConfigAndResponseBoundsEdges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "nil-config"), nil); err != nil {
		t.Fatal(err)
	}

	responseClient := func(body string, length int64) *http.Client {
		return &http.Client{Transport: downloadRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader(body)),
				ContentLength: length,
				Header:        make(http.Header),
			}, nil
		})}
	}
	for _, test := range []struct {
		name, body string
		length     int64
		max        int64
		want       string
	}{
		{name: "stream-too-large", body: "1234", length: -1, max: 3, want: "超过上限"},
		{name: "claimed-length-incomplete", body: "12", length: 3, max: 10, want: "不完整"},
		{name: "empty", body: "", length: -1, max: 10, want: "为空"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultDownloadConfig()
			cfg.MaxRetries = 1
			cfg.MaxBytes = test.max
			cfg.HTTPClient = responseClient(test.body, test.length)
			_, err := DownloadWithConfig(t.Context(), "https://example.com/asset", filepath.Join(t.TempDir(), "asset"), cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	cfg := DefaultDownloadConfig()
	cfg.MaxBytes = 0
	if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "default-max"), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadFileOperationFailures(t *testing.T) {
	originalCreate, originalChmod := downloadCreateTemp, downloadChmod
	originalSync, originalClose, originalRename := downloadSync, downloadClose, downloadRename
	t.Cleanup(func() {
		downloadCreateTemp, downloadChmod = originalCreate, originalChmod
		downloadSync, downloadClose, downloadRename = originalSync, originalClose, originalRename
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()
	failure := errors.New("injected file failure")

	for _, stage := range []string{"create", "chmod", "sync", "close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			downloadCreateTemp, downloadChmod = originalCreate, originalChmod
			downloadSync, downloadClose, downloadRename = originalSync, originalClose, originalRename
			switch stage {
			case "create":
				downloadCreateTemp = func(string, string) (*os.File, error) { return nil, failure }
			case "chmod":
				downloadChmod = func(*os.File, os.FileMode) error { return failure }
			case "sync":
				downloadSync = func(*os.File) error { return failure }
			case "close":
				downloadClose = func(*os.File) error { return failure }
			case "rename":
				downloadRename = func(string, string) error { return failure }
			}
			cfg := DefaultDownloadConfig()
			cfg.MaxRetries = 1
			if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "asset"), cfg); err == nil {
				t.Fatalf("%s failure ignored", stage)
			}
		})
	}
}

func TestRetryDelayAndRetryAfterEdges(t *testing.T) {
	if got := retryDelay(&downloadHTTPError{retryAfter: time.Minute}, 1); got != 30*time.Second {
		t.Fatalf("retry delay cap = %s", got)
	}
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("0", now); got != 0 {
		t.Fatalf("zero retry-after = %s", got)
	}
	if got := parseRetryAfter("nonsense", now); got != 0 {
		t.Fatalf("invalid retry-after = %s", got)
	}
	future := now.Add(5 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(future, now); got != 5*time.Second {
		t.Fatalf("date retry-after = %s", got)
	}
	if got := parseRetryAfter(now.Add(-time.Second).Format(http.TimeFormat), now); got != 0 {
		t.Fatalf("past retry-after = %s", got)
	}
}

func TestDownloadIsAtomicAndRejectsIncompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write([]byte("partial"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(dest, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultDownloadConfig()
	cfg.MaxRetries = 1
	if _, err := DownloadWithConfig(t.Context(), server.URL, dest, cfg); err == nil {
		t.Fatal("incomplete response unexpectedly succeeded")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "previous" {
		t.Fatalf("existing destination changed: %q, %v", data, err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".asset.*.part"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("partial files = %v, %v", matches, err)
	}
}

func TestDownloadRejectsHTTPSDowngradeAndRedirectLoops(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unsafe"))
	}))
	defer httpServer.Close()

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/downgrade" {
			http.Redirect(w, r, httpServer.URL, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer tlsServer.Close()

	cfg := DefaultDownloadConfig()
	cfg.MaxRetries = 1
	cfg.HTTPClient = tlsServer.Client()
	for _, path := range []string{"/downgrade", "/loop"} {
		if _, err := DownloadWithConfig(t.Context(), tlsServer.URL+path, filepath.Join(t.TempDir(), "out"), cfg); err == nil {
			t.Fatalf("redirect %s unexpectedly succeeded", path)
		}
	}
}

func TestDownloadRetriesRateLimitAndHonorsBounds(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	originalAfter := downloadAfter
	delays := make(chan time.Duration, 1)
	done := make(chan time.Time)
	close(done)
	downloadAfter = func(delay time.Duration) <-chan time.Time {
		delays <- delay
		return done
	}
	t.Cleanup(func() { downloadAfter = originalAfter })
	cfg := DefaultDownloadConfig()
	cfg.MaxBytes = 2
	if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "out"), cfg); err != nil {
		t.Fatal(err)
	}
	if delay := <-delays; delay != time.Second {
		t.Fatalf("retry delay = %v", delay)
	}

	tooLarge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "3")
		_, _ = w.Write([]byte("abc"))
	}))
	defer tooLarge.Close()
	cfg.MaxRetries = 1
	if _, err := DownloadWithConfig(t.Context(), tooLarge.URL, filepath.Join(t.TempDir(), "large"), cfg); err == nil {
		t.Fatal("oversized response unexpectedly succeeded")
	}
}

func TestDownload_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("late"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	dest := filepath.Join(t.TempDir(), "cancelled")
	_, err := DownloadWithProgress(ctx, server.URL+"/slow", dest, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDownloadWithProgress_Callback(t *testing.T) {
	body := make([]byte, 1024)
	for i := range body {
		body[i] = 'A'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer server.Close()

	var called int32
	dest := filepath.Join(t.TempDir(), "progress")
	n, err := DownloadWithProgress(context.Background(), server.URL+"/file", dest,
		func(percent float64, downloaded, total int64) {
			atomic.AddInt32(&called, 1)
		})
	if err != nil {
		t.Fatalf("DownloadWithProgress() error = %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("bytes = %d, want %d", n, len(body))
	}
	if atomic.LoadInt32(&called) == 0 {
		t.Error("progress callback was never called")
	}
}

func TestDownload_CreatesParentDirectories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "a", "b", "c", "file")
	_, err := Download(server.URL+"/file", dest)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestDefaultDownloadConfig(t *testing.T) {
	cfg := DefaultDownloadConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.Timeout != 10*time.Minute {
		t.Errorf("Timeout = %v, want 10m", cfg.Timeout)
	}
	if cfg.ProgressInterval != 200*time.Millisecond {
		t.Errorf("ProgressInterval = %v, want 200ms", cfg.ProgressInterval)
	}
}

func TestIsRetriable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("connection reset by peer"), true},
		{fmt.Errorf("Connection Refused"), true},
		{fmt.Errorf("request timeout exceeded"), true},
		{fmt.Errorf("temporary failure in name resolution"), true},
		{fmt.Errorf("unexpected EOF"), true},
		{fmt.Errorf("permission denied"), false},
		{fmt.Errorf("file not found"), false},
	}
	for _, tt := range tests {
		got := isRetriable(tt.err)
		if got != tt.want {
			t.Errorf("isRetriable(%q) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
