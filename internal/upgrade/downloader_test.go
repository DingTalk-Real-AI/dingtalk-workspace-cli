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
	"sync/atomic"
	"testing"
	"time"
)

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

	// Transient HTTP 500 responses consume the bounded attempt budget.
	got := atomic.LoadInt32(&attempts)
	if got != 2 {
		t.Errorf("attempts = %d, want 2", got)
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
