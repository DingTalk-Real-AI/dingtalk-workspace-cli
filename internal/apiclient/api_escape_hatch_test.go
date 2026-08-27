// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func TestCrossPlatformCoverageLegacyCompatibilityGoldenRequests(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			client := NewClient("app-token", "")
			client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != method {
					t.Fatalf("method = %s, want %s", req.Method, method)
				}
				if got := req.URL.String(); got != "https://api.dingtalk.com/v1.0/test?page=2" {
					t.Fatalf("URL = %s", got)
				}
				if got := req.Header.Get(AuthHeader); got != "app-token" {
					t.Fatalf("auth header = %q", got)
				}
				if got := req.Header.Get("User-Agent"); got != "dws-cli/raw-api" {
					t.Fatalf("User-Agent = %q", got)
				}
				return jsonHTTPResponse(`{"value":"unchanged"}`), nil
			})
			var data any
			if method != http.MethodGet {
				data = map[string]any{"name": "value"}
			}
			resp, err := client.Do(context.Background(), RawAPIRequest{
				Method: method,
				Path:   "/v1.0/test",
				Params: map[string]any{"page": 2},
				Data:   data,
			})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := HandleResponse(resp, ResponseOptions{Format: output.FormatJSON, Out: &out, ErrOut: io.Discard}); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["value"] != "unchanged" || got["ok"] != nil {
				t.Fatalf("successful output changed envelope: %s (%v)", out.String(), err)
			}
		})
	}
}

func TestCrossPlatformCoverageLegacyCompatibilityGoldenOAPITokenInjection(t *testing.T) {
	client := NewClient("legacy-app-token", "")
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() != "oapi.dingtalk.com" || req.URL.Query().Get(LegacyAuthParam) != "legacy-app-token" {
			t.Fatalf("legacy URL/token = %s", req.URL.Redacted())
		}
		if req.Header.Get(AuthHeader) != "" {
			t.Fatalf("legacy request leaked new auth header")
		}
		return jsonHTTPResponse(`{"errcode":0,"errmsg":"ok"}`), nil
	})
	resp, err := client.Do(context.Background(), RawAPIRequest{
		Method: http.MethodPost,
		Path:   "https://oapi.dingtalk.com/topapi/v2/user/get",
		Data:   map[string]any{"userid": "u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.BodyReader.Close()
}

func TestCrossPlatformCoverageTargetValidationAndLegacyDetectionAreExact(t *testing.T) {
	allowed := []string{
		"https://api.dingtalk.com/v1.0/test",
		"https://api.dingtalk.com:443/v1.0/test",
		"https://oapi.dingtalk.com/topapi/test",
	}
	for _, target := range allowed {
		if err := ValidateTargetHost(target); err != nil {
			t.Errorf("ValidateTargetHost(%q): %v", target, err)
		}
	}
	blocked := []string{
		"http://api.dingtalk.com/v1.0/test",
		"https://api.dingtalk.com:8443/v1.0/test",
		"https://user:secret@api.dingtalk.com/v1.0/test",
		"https://api.dingtalk.com/v1.0/test#fragment",
		"https://api.dingtalk.com.evil.test/v1.0/test",
	}
	for _, target := range blocked {
		if err := ValidateTargetHost(target); err == nil {
			t.Errorf("ValidateTargetHost(%q) unexpectedly succeeded", target)
		}
	}
	if IsLegacyAPI("https://evil.test/oapi.dingtalk.com/topapi/test") {
		t.Fatal("legacy detection must use parsed host, not substring")
	}
}

func TestCrossPlatformCoverageRedirectPolicyRejectsCredentialLeaks(t *testing.T) {
	original, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/v1.0/start", nil)
	for _, target := range []string{
		"https://oapi.dingtalk.com/topapi/target",
		"http://api.dingtalk.com/v1.0/target",
		"https://api.dingtalk.com:8443/v1.0/target",
	} {
		next, _ := http.NewRequest(http.MethodGet, target, nil)
		if err := ValidateRedirect(next, []*http.Request{original}); err == nil {
			t.Errorf("redirect to %s should fail", target)
		}
	}
	same, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/v1.0/next", nil)
	if err := ValidateRedirect(same, []*http.Request{original}); err != nil {
		t.Fatalf("same-origin redirect failed: %v", err)
	}
}

func TestCrossPlatformCoverageClientDoesNotFollowCrossOriginRedirectWithToken(t *testing.T) {
	client := NewClient("sensitive-app-token", "")
	calls := 0
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			t.Fatalf("cross-origin redirect was followed with headers %v", req.Header)
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://oapi.dingtalk.com/topapi/target"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
			Request:    req,
		}, nil
	})
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: http.MethodGet, Path: "/v1.0/start"}); err == nil {
		t.Fatal("cross-origin redirect should fail")
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
}

func TestCrossPlatformCoverageJSONAtFileAndMultipartStreaming(t *testing.T) {
	dir := t.TempDir()
	paramsPath := filepath.Join(dir, "params.json")
	if err := os.WriteFile(paramsPath, []byte(`{"cursor":"c1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	params, err := ParseJSONMap("@"+paramsPath, "--params", nil)
	if err != nil || params["cursor"] != "c1" {
		t.Fatalf("@file params = %#v, %v", params, err)
	}

	client := NewClient("app-token", "")
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := req.FormValue("count"); got != "2" {
			t.Fatalf("multipart field count = %q", got)
		}
		file, header, err := req.FormFile("media")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if header.Filename != "demo.txt" || string(content) != "streamed-content" {
			t.Fatalf("multipart file = %q %q", header.Filename, content)
		}
		return jsonHTTPResponse(`{"uploaded":true}`), nil
	})
	resp, err := client.Do(context.Background(), RawAPIRequest{
		Method: http.MethodPost,
		Path:   "/v1.0/files/upload",
		Data:   map[string]any{"count": 2},
		File: &FileUpload{
			FieldName: "media",
			FileName:  "demo.txt",
			Reader:    strings.NewReader("streamed-content"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.BodyReader.Close()
}

func TestCrossPlatformCoverageMultipartRejectsHeaderInjectionNames(t *testing.T) {
	if _, err := ParseFileSpec("bad\nfield=demo.txt"); err == nil {
		t.Fatal("multipart field newline should fail")
	}
	defaultField, err := ParseFileSpec("./a=b.png")
	if err != nil || defaultField.FieldName != "file" || defaultField.Path != "./a=b.png" {
		t.Fatalf("path containing equals = %#v, %v", defaultField, err)
	}
	explicitField, err := ParseFileSpec("media=./a=b.png")
	if err != nil || explicitField.FieldName != "media" || explicitField.Path != "./a=b.png" {
		t.Fatalf("explicit field with equals path = %#v, %v", explicitField, err)
	}
	if _, _, err := newMultipartBody(
		&FileUpload{FieldName: "file", FileName: "demo.txt", Reader: strings.NewReader("x")},
		strings.NewReader("x"),
		map[string]any{"bad\r\nheader": "value"},
	); err == nil {
		t.Fatal("multipart form field header injection should fail")
	}
}

func TestCrossPlatformCoverageResponseLimitsStreamingDownloadAndSafeFilename(t *testing.T) {
	large := bytes.Repeat([]byte("x"), config.MaxResponseBodySize+1)
	err := HandleResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		BodyReader: io.NopCloser(bytes.NewReader(large)),
	}, ResponseOptions{Format: output.FormatJSON, Out: io.Discard, ErrOut: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "安全上限") {
		t.Fatalf("oversized JSON error = %v", err)
	}

	header := http.Header{"Content-Disposition": []string{`attachment; filename="../../unsafe.bin"`}}
	if got := inferFilename(header); got != "unsafe.bin" {
		t.Fatalf("safe inferred filename = %q", got)
	}
	path := filepath.Join(t.TempDir(), "download.bin")
	if err := os.WriteFile(path, []byte("old-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	err = HandleResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
		BodyReader: io.NopCloser(strings.NewReader("binary-stream")),
	}, ResponseOptions{OutputPath: path, Out: io.Discard, ErrOut: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "binary-stream" || !strings.Contains(stderr.String(), "13 字节") {
		t.Fatalf("download = %q, stderr=%q", data, stderr.String())
	}
}

func TestCrossPlatformCoverageErrorsAndCamelCasePagination(t *testing.T) {
	err := HandleResponse(&RawAPIResponse{
		StatusCode: 400,
		Header: http.Header{
			"Content-Type":     []string{"application/json"},
			"X-Acs-Request-Id": []string{"req-123"},
		},
		Body: []byte(`{"code":"InvalidParameter","message":"bad value"}`),
	}, ResponseOptions{Format: output.FormatJSON, Out: io.Discard, ErrOut: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "InvalidParameter") || !strings.Contains(err.Error(), "req-123") {
		t.Fatalf("structured error = %v", err)
	}
	var successOut bytes.Buffer
	err = HandleResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"code":"A100","name":"dept-a"}`),
	}, ResponseOptions{Format: output.FormatJSON, Out: &successOut, ErrOut: io.Discard})
	if err != nil || !strings.Contains(successOut.String(), `"code": "A100"`) {
		t.Fatalf("successful payload with business code = %q, %v", successOut.String(), err)
	}
	var redirectOut bytes.Buffer
	err = HandleResponse(&RawAPIResponse{
		StatusCode: http.StatusNotModified,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"name":"cached-dept"}`),
	}, ResponseOptions{Format: output.FormatJSON, Out: &redirectOut, ErrOut: io.Discard})
	if err != nil || !strings.Contains(redirectOut.String(), `"name": "cached-dept"`) {
		t.Fatalf("generic 3xx payload compatibility = %q, %v", redirectOut.String(), err)
	}

	header := http.Header{"Content-Type": []string{"application/json"}}
	_, more, token, err := parsePaginatedResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     header,
		Body:       []byte(`{"result":{"hasMore":true,"nextCursor":"cursor-2","items":[]}}`),
	})
	if err != nil || !more || token != "cursor-2" {
		t.Fatalf("camel pagination = %v %q %v", more, token, err)
	}
	_, _, _, err = parsePaginatedResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     header,
		Body:       []byte(`{"hasMore":true}`),
	})
	if err == nil {
		t.Fatal("ambiguous continuation must fail closed")
	}
	_, _, _, err = parsePaginatedResponse(&RawAPIResponse{
		StatusCode: 200,
		Header:     header,
		Body:       []byte(`{"hasMore":true,"next_token":"same","nextToken":"same"}`),
	})
	if err == nil {
		t.Fatal("continuation request-key ambiguity must fail closed even when values match")
	}
}

func TestCrossPlatformCoverageSameHTTPSOrigin(t *testing.T) {
	a, _ := url.Parse("https://api.dingtalk.com/v1.0/a")
	b, _ := url.Parse("https://API.DINGTALK.COM:443/v1.0/b")
	if !sameHTTPSOrigin(a, b) {
		t.Fatal("default port and explicit 443 should be same origin")
	}
}
