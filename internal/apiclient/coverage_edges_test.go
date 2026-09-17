package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type closeTrackingReader struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

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
	out.Reset()
	if err := PrintDryRun(&out, RawAPIRequest{
		Method: "post",
		Path:   "/v1.0/media/upload",
		Data:   map[string]any{"type": "image"},
		File:   &FileUpload{FieldName: "media", Path: "./demo.png"},
	}, DefaultBaseURL, ""); err != nil {
		t.Fatalf("PrintDryRun multipart preview: %v", err)
	}
	preview := out.String()
	if !strings.Contains(preview, "multipart/form-data") || !strings.Contains(preview, "Form fields:") || strings.Contains(preview, "Body:") {
		t.Fatalf("multipart dry-run preview = %q", preview)
	}
	out.Reset()
	if err := PrintDryRun(&out, RawAPIRequest{
		Method: "post", Path: "/v1.0/media/upload",
		DataSource: "@fields.json", File: &FileUpload{FieldName: "media", Path: "-"},
	}, DefaultBaseURL, ""); err != nil || !strings.Contains(out.String(), "<stdin> (not opened)") || !strings.Contains(out.String(), "@fields.json (not opened)") {
		t.Fatalf("deferred multipart preview = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := PrintDryRun(&out, RawAPIRequest{Method: "post", Path: "/x", DataSource: "-"}, DefaultBaseURL, ""); err != nil || !strings.Contains(out.String(), "<stdin> (not read)") {
		t.Fatalf("deferred JSON preview = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := PrintDryRun(&out, RawAPIRequest{Method: "get", Path: "/x", ParamsSource: "@params.json"}, DefaultBaseURL, ""); err != nil || !strings.Contains(out.String(), "@params.json (not opened)") {
		t.Fatalf("deferred params preview = %q, %v", out.String(), err)
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
	if _, err := ParseJSONMap("-", "--params", nil); err == nil {
		t.Fatal("nil stdin should fail")
	}
	for _, input := range []string{"@", "@bad\npath", "@bad\u200bpath", "@/definitely/missing/dws.json"} {
		if _, err := ParseJSONMap(input, "--params", nil); err == nil {
			t.Errorf("ParseJSONMap(%q) should fail", input)
		}
	}
	largePath := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(largePath, bytes.Repeat([]byte("x"), config.MaxResponseBodySize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseJSONMap("@"+largePath, "--params", nil); err == nil || !strings.Contains(err.Error(), "安全上限") {
		t.Fatalf("oversized input error = %v", err)
	}
	for _, spec := range []string{"=path", "field=", "bad\nfield=path", "field=bad\tpath", "bad\u200bfield=path", "field=bad\u200bpath"} {
		if _, err := ParseFileSpec(spec); err == nil {
			t.Errorf("ParseFileSpec(%q) should fail", spec)
		}
	}
	if upload, err := ParseFileSpec(""); err != nil || upload != nil {
		t.Fatalf("empty file spec = %#v, %v", upload, err)
	}
	if !isExplicitFileField("") || isExplicitFileField("./path") {
		t.Fatal("explicit file field detection changed")
	}
	if !IsDeferredInput("-") || !IsDeferredInput("@file") || IsDeferredInput("inline") {
		t.Fatal("deferred input detection changed")
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
	if err := HandleResponse(nil, opts); err == nil {
		t.Fatal("nil response should fail")
	}
	if got := (*ResponseError)(nil).Error(); got != "API response error" || (*ResponseError)(nil).Unwrap() != nil {
		t.Fatalf("nil ResponseError behavior = %q", got)
	}
	if newResponseError(nil) != nil {
		t.Fatal("nil response error should remain nil")
	}
	readErr := errors.New("response read failed")
	if err := HandleResponse(&RawAPIResponse{StatusCode: 500, Header: textHeader, BodyReader: io.NopCloser(failingReader{err: readErr})}, opts); !errors.Is(err, readErr) {
		t.Fatalf("plain response read error = %v", err)
	}
	tracking := &closeTrackingReader{Reader: strings.NewReader(`{"ok":true}`)}
	if err := HandleResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, BodyReader: tracking}, opts); err != nil || !tracking.closed {
		t.Fatalf("JSON response close = %v, closed=%v", err, tracking.closed)
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
		`{"code":"BadRequest"}`,
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
	if err := checkDingTalkErrorWithRequestID(map[string]any{"code": "BadRequest"}, 400, ""); err == nil || !strings.Contains(err.Error(), "unknown error") {
		t.Fatalf("missing business error message = %v", err)
	}
	if err := checkDingTalkErrorWithRequestID(map[string]any{"code": "RedirectWithoutLocation"}, 302, "redirect-id"); err == nil || !strings.Contains(err.Error(), "redirect-id") {
		t.Fatalf("redirect response error = %v", err)
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
	inferred := filepath.Join(dir, "inferred.bin")
	header := http.Header{"Content-Type": []string{"application/octet-stream"}, "Content-Disposition": []string{`attachment; filename="inferred.bin"`}}
	if got := inferFilename(header); got != "inferred.bin" {
		t.Fatalf("inferred filename = %q", got)
	}
	opts.OutputPath = inferred
	if err := handleBinaryResponse(&RawAPIResponse{Header: header, Body: []byte("bytes")}, opts); err != nil {
		t.Fatalf("inferred binary save: %v", err)
	}
	if !strings.Contains(errOut.String(), "已保存") {
		t.Fatalf("binary status = %q", errOut.String())
	}

	originalCreateTemp, originalSync, originalClose := responseCreateTemp, responseFileSync, responseFileClose
	originalChmod, originalReplace := responseChmod, responseReplace
	t.Cleanup(func() {
		responseCreateTemp, responseFileSync, responseFileClose = originalCreateTemp, originalSync, originalClose
		responseChmod, responseReplace = originalChmod, originalReplace
	})
	for _, tc := range []struct {
		name string
		set  func(error)
		want string
	}{
		{"create", func(want error) { responseCreateTemp = func(string, string) (*os.File, error) { return nil, want } }, "创建临时"},
		{"sync", func(want error) { responseFileSync = func(*os.File) error { return want } }, "同步下载"},
		{"close", func(want error) { responseFileClose = func(*os.File) error { return want } }, "关闭下载"},
		{"chmod", func(want error) { responseChmod = func(string, os.FileMode) error { return want } }, "设置下载"},
		{"replace", func(want error) { responseReplace = func(string, string) error { return want } }, "原子替换"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseCreateTemp, responseFileSync, responseFileClose = originalCreateTemp, originalSync, originalClose
			responseChmod, responseReplace = originalChmod, originalReplace
			wantErr := errors.New(tc.name + " failed")
			tc.set(wantErr)
			err := handleBinaryResponse(&RawAPIResponse{Header: textHeader, Body: []byte("x")}, ResponseOptions{OutputPath: filepath.Join(t.TempDir(), "out.bin"), Out: io.Discard, ErrOut: io.Discard})
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s failure = %v", tc.name, err)
			}
		})
	}
	responseCreateTemp, responseFileSync, responseFileClose = originalCreateTemp, originalSync, originalClose
	responseChmod, responseReplace = originalChmod, originalReplace

	for _, filename := range []string{"", ".", "..", "/", "bad\x00name"} {
		header := http.Header{"Content-Disposition": []string{`attachment; filename="` + filename + `"`}}
		if inferFilename(header) != "" {
			t.Errorf("unsafe inferred filename %q was accepted", filename)
		}
	}
	if _, err := readBoundedResponse(&RawAPIResponse{BodyReader: io.NopCloser(failingReader{err: readErr})}); !errors.Is(err, readErr) {
		t.Fatalf("bounded read error = %v", err)
	}

	for _, ct := range []string{" application/json; charset=utf-8 ", "text/json", "application/problem+json", "text/plain"} {
		_ = isJSONContentType(ct)
	}
	for _, value := range []any{float64(1), 2, int64(3), json.Number("4"), json.Number("bad"), "5"} {
		_ = toFloat64(value)
	}
	for _, value := range []any{float64(0), float64(200), json.Number("0"), json.Number("200"), "ok", "success", 0, 200, int64(0), int64(200), struct{}{}} {
		_, _ = errorCode(value)
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
	if _, _, _, err := parsePaginatedResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, BodyReader: io.NopCloser(failingReader{err: errors.New("read failed")})}); err == nil {
		t.Fatal("pagination read failure should fail")
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
	if _, _, _, err := parsePaginatedResponse(&RawAPIResponse{StatusCode: 200, Header: jsonHeader, Body: []byte(`{"has_more":true,"hasMore":false}`)}); err == nil {
		t.Fatal("conflicting hasMore values should fail")
	}
	if got := paginationValue(json.Number("42")); got != "42" {
		t.Fatalf("json.Number pagination value = %q", got)
	}
	for _, tc := range []struct {
		params string
		data   string
		file   *FileUpload
		fail   bool
	}{
		{"-", "", nil, false},
		{"", "-", nil, false},
		{"", "", &FileUpload{Path: "-"}, false},
		{"-", "-", nil, true},
		{"-", "", &FileUpload{Path: "-"}, true},
	} {
		err := ValidateInputStdinExclusion(tc.params, tc.data, tc.file)
		if (err != nil) != tc.fail {
			t.Errorf("stdin exclusion %#v = %v", tc, err)
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
	var completionLogs bytes.Buffer
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"hasMore":false,"items":[]}`), nil
	})
	if pages, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{LogWriter: &completionLogs}); err != nil || len(pages) != 1 || !strings.Contains(completionLogs.String(), "数据获取完成") {
		t.Fatalf("pagination completion pages=%d logs=%q err=%v", len(pages), completionLogs.String(), err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"hasMore":true}`), nil
	})
	if _, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{}); err == nil || !strings.Contains(err.Error(), "hasMore=true") {
		t.Fatalf("ambiguous pagination continuation = %v", err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"hasMore":true,"nextToken":"next"}`), nil
	})
	if _, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "POST", Path: "/x", Data: "not-object"}, PaginationOptions{}); err == nil || !strings.Contains(err.Error(), "非 object") {
		t.Fatalf("pagination injection error = %v", err)
	}
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
	if pages, err := client.PaginateAll(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"}, PaginationOptions{PageDelay: 1, LogWriter: &logs}); err == nil || len(pages) != 1 || !strings.Contains(err.Error(), "第 2 页解析失败") {
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
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "/x", File: &FileUpload{Reader: strings.NewReader("x")}}); err == nil {
		t.Fatal("GET multipart should fail")
	}
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "POST", Path: "/x", File: &FileUpload{Path: "/definitely/missing/dws-upload"}}); err == nil {
		t.Fatal("missing upload path should fail")
	}
	if _, _, err := newMultipartBody(nil, strings.NewReader("x"), nil); err == nil {
		t.Fatal("nil multipart upload should fail")
	}
	if _, _, err := newMultipartBody(&FileUpload{}, strings.NewReader("x"), []string{"not-object"}); err == nil {
		t.Fatal("non-object multipart data should fail")
	}
	if _, _, err := newMultipartBody(&FileUpload{FieldName: "bad\nfield"}, strings.NewReader("x"), nil); err == nil {
		t.Fatal("multipart field newline should fail")
	}
	if _, _, err := newMultipartBody(&FileUpload{FieldName: "file", FileName: "bad\nname"}, strings.NewReader("x"), nil); err == nil {
		t.Fatal("multipart filename newline should fail")
	}
	if _, err := multipartFieldValue(make(chan int)); err == nil {
		t.Fatal("unmarshalable multipart field should fail")
	}
	if got, err := multipartFieldValue("text"); err != nil || got != "text" {
		t.Fatalf("string multipart field = %q, %v", got, err)
	}
	uploadPath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return jsonHTTPResponse(`{"ok":true}`), nil
	})
	resp, err := client.Do(context.Background(), RawAPIRequest{Method: "POST", Path: "/x", File: &FileUpload{Path: uploadPath}})
	if err != nil {
		t.Fatalf("multipart path upload = %v", err)
	}
	_ = resp.BodyReader.Close()
	if _, err := client.Do(context.Background(), RawAPIRequest{Method: "POST", Path: "/x", File: &FileUpload{Reader: strings.NewReader("x")}, Data: map[string]any{"bad": make(chan int)}}); err == nil {
		t.Fatal("unmarshalable multipart field should reach transport error")
	}
	for _, req := range []RawAPIRequest{
		{Method: "POST", Path: "/x", File: &FileUpload{Reader: strings.NewReader("x")}, Data: []string{"not-object"}},
		{Method: "POST", Path: "/x", File: &FileUpload{FieldName: "bad\nfield", Reader: strings.NewReader("x")}},
	} {
		if _, err := client.Do(context.Background(), req); err == nil {
			t.Fatalf("invalid multipart request %#v should fail", req)
		}
	}
	originalWriteField, originalCreateFormFile := multipartWriteField, multipartCreateFormFile
	t.Cleanup(func() {
		multipartWriteField, multipartCreateFormFile = originalWriteField, originalCreateFormFile
	})
	wantMultipartErr := errors.New("multipart hook failed")
	for _, hook := range []string{"field", "file"} {
		multipartWriteField, multipartCreateFormFile = originalWriteField, originalCreateFormFile
		if hook == "field" {
			multipartWriteField = func(*multipart.Writer, string, string) error { return wantMultipartErr }
		} else {
			multipartCreateFormFile = func(*multipart.Writer, string, string) (io.Writer, error) { return nil, wantMultipartErr }
		}
		body, _, err := newMultipartBody(&FileUpload{FieldName: "file", FileName: "x.txt"}, strings.NewReader("x"), map[string]any{"field": "value"})
		if err != nil {
			t.Fatalf("create hooked multipart body: %v", err)
		}
		_, err = io.ReadAll(body)
		if !errors.Is(err, wantMultipartErr) {
			t.Fatalf("multipart %s hook error = %v", hook, err)
		}
	}
	multipartWriteField, multipartCreateFormFile = originalWriteField, originalCreateFormFile
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
	if _, err := client.buildURL("/x#fragment", nil); err == nil {
		t.Fatal("URL fragment should fail")
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
	resp, err = client.Do(context.Background(), RawAPIRequest{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("streaming response headers = %v", err)
	}
	if err := HandleResponse(resp, ResponseOptions{OutputPath: filepath.Join(t.TempDir(), "out.bin"), Out: io.Discard, ErrOut: io.Discard}); !errors.Is(err, wantErr) {
		t.Fatalf("response stream error = %v", err)
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
	origin, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/x", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://api.dingtalk.com/y", nil)
	if err := ValidateRedirect(next, nil); err != nil {
		t.Fatalf("initial redirect validation = %v", err)
	}
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = origin
	}
	if ValidateRedirect(next, via) == nil {
		t.Fatal("redirect limit should fail")
	}
}
