// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDownloadTemporaryFileFailures(t *testing.T) {
	for _, mode := range []string{"create", "close", "cancel-before-attempt", "cancel-before-publish"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("temporary file failure")
			requests := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 4, Body: io.NopCloser(strings.NewReader("data"))}, nil
			})))
			testseam.Swap(t, &downloadCreateTemp, func(dir, pattern string) (*os.File, error) {
				if mode == "create" {
					return nil, failure
				}
				f, err := os.CreateTemp(dir, pattern)
				if err == nil && mode == "close" {
					_ = f.Close()
				}
				if mode == "cancel-before-attempt" {
					cancel()
				}
				return f, err
			})
			dir := t.TempDir()
			dest := filepath.Join(dir, "asset")
			if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := &DownloadConfig{MaxRetries: 3, ProgressInterval: time.Hour}
			if mode == "cancel-before-publish" {
				cfg.ProgressCallback = func(int64, int64) { cancel() }
			}
			_, err := DownloadWithConfig(ctx, "https://example.test/asset", dest, cfg)
			if strings.HasPrefix(mode, "cancel-") {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			} else {
				var fileErr *downloadFileError
				if !errors.As(err, &fileErr) || isRetriable(err) || err.Error() == "" {
					t.Fatalf("filesystem error classification: %v", err)
				}
				if mode == "create" && !errors.Is(err, failure) {
					t.Fatal("filesystem cause was not preserved")
				}
			}
			wantRequests := 0
			if mode == "cancel-before-publish" {
				wantRequests = 1
			}
			if requests != wantRequests {
				t.Fatalf("requests = %d, want %d", requests, wantRequests)
			}
			data, readErr := os.ReadFile(dest)
			entries, listErr := os.ReadDir(dir)
			if readErr != nil || listErr != nil || string(data) != "original" || len(entries) != 1 {
				t.Fatalf("failure changed destination or leaked a partial file: %q %v %v %v", data, entries, readErr, listErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadFileIOFailures(t *testing.T) {
	for _, mode := range []string{"resume-stat", "resume-size", "write", "close", "short-body"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "partial")
			if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			state := &downloadState{}
			resume := strings.HasPrefix(mode, "resume-")
			if resume {
				state.offset, state.total, state.etag = 2, 4, `"original"`
			}
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 4, Body: io.NopCloser(strings.NewReader("data"))}
				if resume {
					resp.StatusCode, resp.ContentLength = 206, 2
					resp.Header.Set("ETag", `"original"`)
					resp.Header.Set("Content-Range", "bytes 2-3/4")
					resp.Body = io.NopCloser(strings.NewReader("ta"))
				} else if mode == "short-body" {
					resp.ContentLength = 10
				}
				return resp, nil
			})))
			var opened *os.File
			testseam.Swap(t, &downloadOpenFile, func(name string, flag int, perm os.FileMode) (*os.File, error) {
				if mode == "write" {
					flag = os.O_RDONLY
				}
				f, err := os.OpenFile(name, flag, perm)
				opened = f
				if err == nil && mode == "resume-stat" {
					_ = f.Close()
				}
				return f, err
			})
			cfg := &DownloadConfig{}
			if mode == "close" {
				cfg.ProgressInterval = -time.Second
				cfg.ProgressCallback = func(int64, int64) { _ = opened.Close() }
			}
			_, err := downloadAttempt(t.Context(), "https://example.test/asset", path, cfg, state)
			if mode == "short-body" {
				if !errors.Is(err, io.ErrUnexpectedEOF) || !isRetriable(err) {
					t.Fatalf("short response not retryable: %v", err)
				}
			} else {
				var fileErr *downloadFileError
				if !errors.As(err, &fileErr) || isRetriable(err) {
					t.Fatalf("disk failure should stop retries: %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadRedirectAndEncoding(t *testing.T) {
	for _, mode := range []string{"allowed", "credentials", "downgrade", "loop", "encoding"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 4, Body: io.NopCloser(strings.NewReader("data"))}
				if mode == "encoding" {
					resp.Header.Set("Content-Encoding", "gzip")
				} else if requests == 1 || mode == "loop" {
					resp.StatusCode = http.StatusFound
					target := "https://cdn.example.test/asset"
					if mode == "credentials" {
						target = "https://user:secret@cdn.example.test/asset"
					} else if mode == "downgrade" {
						target = "http://cdn.example.test/asset"
					}
					resp.Header.Set("Location", target)
				}
				return resp, nil
			})))
			dest := filepath.Join(t.TempDir(), "asset")
			_, err := DownloadWithConfig(t.Context(), "https://example.test/asset", dest, &DownloadConfig{MaxRetries: 1})
			if mode == "allowed" {
				data, readErr := os.ReadFile(dest)
				if err != nil || readErr != nil || string(data) != "data" || requests != 2 {
					t.Fatalf("safe redirect failed: %q %v %v, requests=%d", data, err, readErr, requests)
				}
			} else if err == nil {
				t.Fatal("unsafe or encoded response was accepted")
			} else if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("rejected response published a destination")
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadProtocolBounds(t *testing.T) {
	for _, value := range []string{"", "items 1-2/3", "bytes 1-2", "bytes 1/3", "bytes a-2/3", "bytes 2-1/3", "bytes 1-3/3"} {
		if _, _, _, ok := parseDownloadRange(value); ok {
			t.Fatalf("invalid range accepted: %q", value)
		}
	}
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"-1", 0}, {"12", 12 * time.Second}, {"100", 30 * time.Second}, {"invalid", 0},
		{time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat), 30 * time.Second},
		{time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat), 0}} {
		if got := retryAfterDuration(tc.value); got != tc.want {
			t.Fatalf("Retry-After %q = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageSHA256ReadFailure(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "unreadable-")
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	testseam.Swap(t, &verifyOpen, func(string) (*os.File, error) { return file, nil })
	if _, err := ComputeSHA256(file.Name()); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("stream read failure lost: %v", err)
	}
}
