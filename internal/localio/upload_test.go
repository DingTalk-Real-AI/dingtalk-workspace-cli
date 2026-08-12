// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePutFileRetriesAndUploadsExactBytesE2E(t *testing.T) {
	payload := []byte("minutes-e2e")
	replacement := []byte("replacement-data")
	path := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	replacementPath := filepath.Join(t.TempDir(), "replacement.wav")
	if err := os.WriteFile(replacementPath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	openCalls := 0
	testseam.Swap(t, &openUploadFile, func(candidate string) (*os.File, error) {
		openCalls++
		if openCalls > 1 {
			return os.Open(replacementPath)
		}
		return os.Open(candidate)
	})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != string(payload) {
			t.Errorf("body = %q", body)
		}
		if calls == 1 {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	result, err := putFileWithClient(context.Background(), server.URL, path, 100, server.Client(), func(string) error { return nil })
	if err != nil || calls != 2 || openCalls != 1 || result.Attempts != 2 || result.SizeBytes != int64(len(payload)) {
		t.Fatalf("upload = %#v calls=%d opens=%d err=%v", result, calls, openCalls, err)
	}
}

func TestCrossPlatformCoveragePutFileValidatesOpenedDescriptorE2E(t *testing.T) {
	base := t.TempDir()
	requestedPath := filepath.Join(base, "requested.wav")
	if err := os.WriteFile(requestedPath, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacementPath := filepath.Join(base, "replacement.wav")
	if err := os.WriteFile(replacementPath, []byte("replacement-exceeds-limit"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &openUploadFile, func(string) (*os.File, error) {
		return os.Open(replacementPath)
	})
	called := false
	client := &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", requestedPath, 2, client, func(string) error { return nil }); err == nil {
		t.Fatal("replacement descriptor bypassed size validation")
	}
	if called {
		t.Fatal("upload started before the opened descriptor was validated")
	}
}

type uploadRoundTripFunc func(*http.Request) (*http.Response, error)

func (f uploadRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type uploadCloseErrorBody struct{ io.Reader }

func (uploadCloseErrorBody) Close() error { return errors.New("close") }

func TestCrossPlatformCoveragePutFileFailureBranchesE2E(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "audio.wav")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	okClient := &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if _, err := PutFile(context.Background(), "not-a-valid-upload-url", path, 10); err == nil {
		t.Fatal("default upload URL validation accepted invalid URL")
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, okClient, nil); err == nil {
		t.Fatal("nil validator accepted")
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, okClient, func(string) error { return errors.New("invalid") }); err == nil {
		t.Fatal("validator error ignored")
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", filepath.Join(base, "missing"), 10, okClient, func(string) error { return nil }); err == nil {
		t.Fatal("stat error ignored")
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", base, 10, okClient, func(string) error { return nil }); err == nil {
		t.Fatal("directory accepted")
	}
	empty := filepath.Join(base, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", empty, 10, okClient, func(string) error { return nil }); err == nil {
		t.Fatal("empty file accepted")
	}
	if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 1, okClient, func(string) error { return nil }); err == nil {
		t.Fatal("oversize file accepted")
	}

	t.Run("wrapper", func(t *testing.T) {
		redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			http.Redirect(w, request, "http://example.invalid/redirected", http.StatusFound)
		}))
		defer redirect.Close()
		testseam.Swap(t, &newUploadHTTPClient, redirect.Client)
		testseam.Swap(t, &validateUploadURL, func(string) error { return nil })
		if _, err := PutFile(context.Background(), redirect.URL, path, 0); err == nil {
			t.Fatal("PutFile followed redirect")
		}
	})
	t.Run("open", func(t *testing.T) {
		testseam.Swap(t, &openUploadFile, func(string) (*os.File, error) { return nil, errors.New("open") })
		if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, okClient, func(string) error { return nil }); err == nil {
			t.Fatal("open error ignored")
		}
	})
	t.Run("opened file stat", func(t *testing.T) {
		testseam.Swap(t, &openUploadFile, func(candidate string) (*os.File, error) {
			file, err := os.Open(candidate)
			if err == nil {
				_ = file.Close()
			}
			return file, err
		})
		if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, okClient, func(string) error { return nil }); err == nil {
			t.Fatal("opened file stat error ignored")
		}
	})
	t.Run("request", func(t *testing.T) {
		testseam.Swap(t, &newUploadRequest, func(context.Context, string, string, io.Reader) (*http.Request, error) {
			return nil, errors.New("request")
		})
		if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, okClient, func(string) error { return nil }); err == nil {
			t.Fatal("request error ignored")
		}
	})

	for _, tc := range []struct {
		name   string
		client *http.Client
	}{
		{name: "client", client: &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("call") })}},
		{name: "server", client: &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("retry"))}, nil
		})}},
		{name: "close", client: &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: uploadCloseErrorBody{Reader: strings.NewReader("ok")}}, nil
		})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := putFileWithClient(context.Background(), "http://example.invalid", path, 10, tc.client, func(string) error { return nil }); err == nil {
				t.Fatalf("%s retries unexpectedly succeeded", tc.name)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	retryClient := &http.Client{Transport: uploadRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("retry"))}, nil
	})}
	if _, err := putFileWithClient(cancelled, "http://example.invalid", path, 10, retryClient, func(string) error { return nil }); err == nil {
		t.Fatal("cancelled retry succeeded")
	}
}

func TestCrossPlatformCoveragePutFileRejectsRedirectAndInvalidFileE2E(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	client := redirect.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if _, err := putFileWithClient(context.Background(), redirect.URL, path, 100, client, func(string) error { return nil }); err == nil {
		t.Fatal("redirect accepted")
	}
	if _, err := putFileWithClient(context.Background(), redirect.URL, filepath.Join(t.TempDir(), "missing"), 100, client, func(string) error { return nil }); err == nil {
		t.Fatal("missing file accepted")
	}
}
