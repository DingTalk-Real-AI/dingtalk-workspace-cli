// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package apiclient

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMultipartValidationAndFailures(t *testing.T) {
	ctx := context.Background()
	request := func() MultipartUploadRequest {
		return MultipartUploadRequest{Path: "/upload", FileName: "skill.zip", File: strings.NewReader("data")}
	}
	var absent *APIClient
	if _, err := absent.UploadMultipart(ctx, request()); err == nil {
		t.Fatal("nil client accepted")
	}
	client := NewClient("fixture-token", "https://api-deap.dingtalk.com")
	client.TargetValidator = nil
	if err := client.validateTarget("https://api-deap.dingtalk.com/upload"); err != nil {
		t.Fatal(err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("invalid request reached HTTP transport")
		return nil, errors.New("unexpected request")
	})
	if _, err := client.UploadMultipart(ctx, MultipartUploadRequest{}); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, path := range []string{"/%zz", "https://evil.example/upload"} {
		r := request()
		r.Path = path
		if _, err := client.UploadMultipart(ctx, r); err == nil {
			t.Fatal("invalid target accepted", path)
		}
	}
	client.TargetValidator = func(string) error { return errors.New("target rejected") }
	if _, err := client.UploadMultipart(ctx, request()); err == nil {
		t.Fatal("target rejection lost")
	}
	client.TargetValidator = func(string) error { return nil }
	client.DingTalkExt = "fixture-extension"
	t.Run("request construction", func(t *testing.T) {
		old := newHTTPRequest
		t.Cleanup(func() { newHTTPRequest = old })
		newHTTPRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
			return nil, errors.New("fixture request failure")
		}
		if _, err := client.UploadMultipart(ctx, request()); err == nil {
			t.Fatal("construction failure lost")
		}
	})
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("transport failure") })
	if _, err := client.UploadMultipart(ctx, request()); err == nil {
		t.Fatal("transport failure lost")
	}
	client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, r.Body)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(failingReader{err: errors.New("response failure")})}, nil
	})
	if _, err := client.UploadMultipart(ctx, request()); err == nil || !strings.Contains(err.Error(), "reading response") {
		t.Fatalf("error=%v", err)
	}
	r := request()
	r.File = failingReader{err: errors.New("source failure")}
	if _, err := client.UploadMultipart(ctx, r); err == nil || !strings.Contains(err.Error(), "streaming multipart") {
		t.Fatalf("error=%v", err)
	}
}

func TestCrossPlatformCoverageUploadMultipartRejectsCRLFInNames(t *testing.T) {
	// The streaming UploadMultipart path must reject CR/LF in field keys, the file
	// field name and the file name before creating the pipe/goroutine, matching the
	// non-streaming newMultipartBody guard. Otherwise user-controlled names could
	// inject extra MIME headers or fields into the multipart body.
	ctx := context.Background()
	client := NewClient("fixture-token", "https://api-deap.dingtalk.com")
	client.TargetValidator = func(string) error { return nil }
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("name carrying CR/LF reached the HTTP transport")
		return nil, errors.New("unexpected request")
	})
	for _, tc := range []struct {
		name string
		req  MultipartUploadRequest
	}{
		{"field key", MultipartUploadRequest{Path: "/upload", FileName: "skill.zip",
			File: strings.NewReader("data"), Fields: map[string]string{"bad\r\nX-Injected: 1": "v"}}},
		{"file field name", MultipartUploadRequest{Path: "/upload", FieldName: "file\r\nX-Injected: 1",
			FileName: "skill.zip", File: strings.NewReader("data")}},
		{"file name", MultipartUploadRequest{Path: "/upload", FileName: "skill.zip\r\nX-Injected: 1",
			File: strings.NewReader("data")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := client.UploadMultipart(ctx, tc.req); err == nil || !strings.Contains(err.Error(), "换行符") {
				t.Fatalf("CRLF-bearing name not rejected before side effects: err=%v", err)
			}
		})
	}
}

type multipartFailWriter struct{ writes, failAt int }

func (w *multipartFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func TestCrossPlatformCoverageMultipartFormFieldAndPartFailures(t *testing.T) {
	for _, failAt := range []int{1, 2, 3, 4, 5, 6} {
		writer := &multipartFailWriter{failAt: failAt}
		err := writeMultipartBody(multipart.NewWriter(writer), MultipartUploadRequest{Fields: map[string]string{"b": "two", "a": "one"}, FileName: "skill.zip", File: strings.NewReader("data")})
		if err == nil {
			t.Fatalf("write %d failure ignored (writes=%d)", failAt, writer.writes)
		}
	}
}
