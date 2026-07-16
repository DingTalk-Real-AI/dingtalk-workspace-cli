package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCrossPlatformCoverageDryRunAndParseCoverageEdges(t *testing.T) {
	for _, tc := range []struct {
		base string
		path string
	}{
		{DefaultBaseURL, "/v1.0/test"},
		{LegacyBaseURL, "/topapi/test"},
	} {
		var out bytes.Buffer
		err := PrintDryRun(&out, RawAPIRequest{
			Method: "post", Path: tc.path,
			Params: map[string]any{"page": 1}, Data: map[string]any{"name": "value"},
		}, tc.base, "token-value")
		if err != nil || !strings.Contains(out.String(), "Dry Run") || !strings.Contains(out.String(), "toke****") {
			t.Fatalf("PrintDryRun(%s) = %q, %v", tc.base, out.String(), err)
		}
	}
	var out bytes.Buffer
	if err := PrintDryRun(&out, RawAPIRequest{Method: "get", Path: "/x", Params: map[string]any{"bad": make(chan int)}, Data: make(chan int)}, DefaultBaseURL, "tiny"); err != nil {
		t.Fatalf("PrintDryRun unsupported preview: %v", err)
	}

	wantErr := errors.New("read failed")
	if _, err := ParseJSONMap("-", "--params", failingReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("ParseJSONMap read error = %v", err)
	}
	if got, err := ParseJSONMap("-", "--params", strings.NewReader(" \n")); err != nil || got != nil {
		t.Fatalf("ParseJSONMap empty stdin = %#v, %v", got, err)
	}
	if _, err := ParseOptionalBody("POST", "-", failingReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("ParseOptionalBody read error = %v", err)
	}
	if got, err := ParseOptionalBody("POST", "-", strings.NewReader(" \n")); err != nil || got != nil {
		t.Fatalf("ParseOptionalBody empty stdin = %#v, %v", got, err)
	}
	if _, err := ParseOptionalBody("POST", "{", strings.NewReader("")); err == nil {
		t.Fatal("invalid optional body should fail")
	}
}

func TestCrossPlatformCoverageResponseHandlingCoverageEdges(t *testing.T) {
	jsonHeader := http.Header{"Content-Type": []string{"application/json"}}
	textHeader := http.Header{"Content-Type": []string{"text/plain"}}
	var out, errOut bytes.Buffer
	opts := ResponseOptions{Format: output.FormatJSON, Out: &out, ErrOut: &errOut}

	if err := HandleResponse(&RawAPIResponse{StatusCode: 500, Header: textHeader, Body: []byte(" failed ")}, opts); err == nil {
		t.Fatal("plain HTTP error should fail")
	}
	for _, body := range [][]byte{nil, []byte("{")} {
		if err := HandleResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, Body: body}, opts); err == nil {
			t.Errorf("invalid JSON body %q should fail", body)
		}
	}
	out.Reset()
	if err := HandleResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, Body: []byte(`{"ok":true}`)}, opts); err != nil || !strings.Contains(out.String(), "ok") {
		t.Fatalf("successful JSON response = %q, %v", out.String(), err)
	}
	for _, payload := range []string{
		`{"errcode":1}`,
		`{"message":"message failure"}`,
		`{"error":"error failure"}`,
		`{}`,
	} {
		status := 200
		if !strings.Contains(payload, "errcode") {
			status = 500
		}
		if err := HandleResponse(&RawAPIResponse{StatusCode: status, Header: jsonHeader, Body: []byte(payload)}, opts); err == nil {
			t.Errorf("business/HTTP payload %s should fail", payload)
		}
	}
	if err := checkDingTalkError([]any{1}, 200); err != nil || checkDingTalkError(map[string]any{"errcode": 0}, 200) != nil {
		t.Fatal("successful DingTalk response classified as error")
	}

	if err := HandleResponse(&RawAPIResponse{StatusCode: 200, Header: textHeader, Body: []byte("binary")}, opts); err == nil {
		t.Fatal("binary response without filename should fail")
	}
	invalidCD := http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{`attachment; filename="unterminated`}}
	if inferFilename(invalidCD) != "" {
		t.Fatal("invalid content disposition should not infer filename")
	}
	if inferFilename(http.Header{}) != "" {
		t.Fatal("missing content disposition should not infer filename")
	}

	dir := t.TempDir()
	blockedParent := filepath.Join(dir, "file")
	if err := os.WriteFile(blockedParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.OutputPath = filepath.Join(blockedParent, "child.bin")
	if err := handleBinaryResponse(&RawAPIResponse{Header: textHeader, Body: []byte("x")}, opts); err == nil {
		t.Fatal("binary mkdir failure should fail")
	}
	opts.OutputPath = dir
	if err := handleBinaryResponse(&RawAPIResponse{Header: textHeader, Body: []byte("x")}, opts); err == nil {
		t.Fatal("binary write to directory should fail")
	}
	opts.OutputPath = ""
	inferred := filepath.Join(dir, "inferred.bin")
	header := http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{`attachment; filename="` + inferred + `"`}}
	if err := handleBinaryResponse(&RawAPIResponse{Header: header, Body: []byte("bytes")}, opts); err != nil {
		t.Fatalf("inferred binary save: %v", err)
	}
	if !strings.Contains(errOut.String(), "已保存") {
		t.Fatalf("binary status = %q", errOut.String())
	}

	for _, ct := range []string{" application/json; charset=utf-8 ", "text/json", "application/problem+json", "text/plain"} {
		_ = isJSONContentType(ct)
	}
	for _, value := range []any{float64(1), 2, int64(3), json.Number("4"), json.Number("bad"), "5"} {
		_ = toFloat64(value)
	}
}

func TestCrossPlatformCoveragePaginationParsingAndInjectionEdges(t *testing.T) {
	jsonHeader := http.Header{"Content-Type": []string{"application/json"}}
	for _, resp := range []*RawAPIResponse{
		{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: []byte("x")},
		{StatusCode: 200, Header: jsonHeader},
		{StatusCode: 200, Header: jsonHeader, Body: []byte("{")},
		{StatusCode: 500, Header: jsonHeader, Body: []byte(`{"message":"bad"}`)},
	} {
		if _, _, _, err := parsePaginatedResponse(resp); err == nil {
			t.Errorf("parsePaginatedResponse(%#v) should fail", resp)
		}
	}
	responses := []struct {
		body  string
		more  bool
		token string
	}{
		{`{"result":{"has_more":true,"next_cursor":12}}`, true, "12"},
		{`{"has_more":true,"next_cursor":13}`, true, "13"},
		{`{"next_token":"next"}`, true, "next"},
		{`{"result":[],"has_more":false}`, false, ""},
	}
	for _, tc := range responses {
		_, more, token, err := parsePaginatedResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, Body: []byte(tc.body)})
		if err != nil || more != tc.more || token != tc.token {
			t.Errorf("pagination %s = %v, %q, %v", tc.body, more, token, err)
		}
	}

	getCases := []RawAPIRequest{
		{Method: "GET"},
		{Method: "GET", Params: map[string]any{"cursor": "old"}},
		{Method: "GET", Params: map[string]any{"next_token": "old"}},
		{Method: "POST", Data: map[string]any{"cursor": "old"}},
		{Method: "PUT", Data: map[string]any{}},
		{Method: "POST", Data: "not-a-map"},
	}
	for _, req := range getCases {
		_ = injectPageToken(req, "new")
	}
	logf(nil, "ignored")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCrossPlatformCoveragePaginationControlFlowEdges(t *testing.T) {
	wantErr := errors.New("transport failed")
	client := NewClient("token", DefaultBaseURL)
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader("bad"))}, nil
	})
	if _, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{}); err == nil {
		t.Fatal("first page parse error should fail")
	}
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr })
	if _, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{}); !errors.Is(err, wantErr) {
		t.Fatalf("first page transport error = %v", err)
	}

	calls := 0
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonHTTPResponse(`{"next_token":"next"}`), nil
		}
		return nil, wantErr
	})
	if pages, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{PageDelay: 1}); err == nil || len(pages) != 1 {
		t.Fatalf("later transport error pages=%d err=%v", len(pages), err)
	}

	calls = 0
	var logs bytes.Buffer
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonHTTPResponse(`{"next_token":"next"}`), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader("bad"))}, nil
	})
	if pages, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{PageDelay: 1, LogWriter: &logs}); err != nil || len(pages) != 1 || !strings.Contains(logs.String(), "解析失败") {
		t.Fatalf("later parse failure pages=%d logs=%q err=%v", len(pages), logs.String(), err)
	}

	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"next_token":"next"}`), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if pages, err := client.PaginateAll(ctx, RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{PageDelay: 10}); !errors.Is(err, context.Canceled) || len(pages) != 1 {
		t.Fatalf("pagination cancellation pages=%d err=%v", len(pages), err)
	}
	logs.Reset()
	if pages, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{PageLimit: 1, PageDelay: 1, LogWriter: &logs}); err != nil || len(pages) != 1 || !strings.Contains(logs.String(), "安全上限") {
		t.Fatalf("pagination safety cap pages=%d logs=%q err=%v", len(pages), logs.String(), err)
	}
}

func TestCrossPlatformCoverageClientAndValidationFailureEdges(t *testing.T) {
	client := NewClient("token", DefaultBaseURL)
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "https://api.dingtalk.com/%zz"}); err == nil {
		t.Fatal("Do with invalid URL should fail")
	}
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "https://example.test/x"}); err == nil {
		t.Fatal("Do to untrusted host should fail")
	}
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "POST", Path: "/x", Data: make(chan int)}); err == nil {
		t.Fatal("unmarshalable request body should fail")
	}
	if _, err := client.buildURL("https://api.dingtalk.com/%zz", nil); err == nil {
		t.Fatal("invalid URL should fail")
	}
	oldNewRequest := newHTTPRequest
	t.Cleanup(func() { newHTTPRequest = oldNewRequest })
	wantCreateErr := errors.New("request creation failed")
	newHTTPRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) { return nil, wantCreateErr }
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}); !errors.Is(err, wantCreateErr) {
		t.Fatalf("request creation error = %v", err)
	}
	newHTTPRequest = oldNewRequest
	wantErr := errors.New("request failed")
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr })
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}); !errors.Is(err, wantErr) {
		t.Fatalf("HTTP transport error = %v", err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(failingReader{err: wantErr})}, nil
	})
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}); !errors.Is(err, wantErr) {
		t.Fatalf("response read error = %v", err)
	}

	if ValidateTargetHost("http://%zz") == nil {
		t.Fatal("invalid target URL should fail")
	}
	for _, r := range []rune{0x200B, 0xFEFF, 0x202A, 0x2028, 0x2066, 0x061C, 0xFDD0} {
		if !isDangerousUnicode(r) || ValidateUserInput("x"+string(r), "field") == nil {
			t.Errorf("dangerous rune %U was accepted", r)
		}
	}
	if isDangerousUnicode('中') || ValidateUserInput("safe\t\n中文", "field") != nil {
		t.Fatal("safe Unicode/input was rejected")
	}
}
