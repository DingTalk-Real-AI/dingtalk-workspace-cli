package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCrossPlatformCoverageDefaultFileHTTPTransfersCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == http.StatusOK && r.Header.Get("X-Test") != "yes" && r.Method == http.MethodPut {
			t.Error("upload header missing")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "response")
	}))
	defer server.Close()

	if err := defaultHTTPPutFile(context.Background(), server.URL, map[string]string{"X-Test": "yes"}, file, 7); err != nil {
		t.Fatal(err)
	}
	status = http.StatusBadRequest
	if err := defaultHTTPPutFile(context.Background(), server.URL, nil, file, 7); err == nil {
		t.Fatal("failed upload succeeded")
	}
	if err := defaultHTTPPutFile(context.Background(), server.URL, nil, filepath.Join(t.TempDir(), "missing"), 0); err == nil {
		t.Fatal("missing upload file succeeded")
	}
	if err := defaultHTTPPutFile(context.Background(), ":", nil, file, 7); err == nil {
		t.Fatal("invalid upload URL succeeded")
	}

	status = http.StatusOK
	destination := filepath.Join(t.TempDir(), "download.txt")
	if err := defaultHTTPGetFile(context.Background(), server.URL, map[string]string{"X-Test": "yes"}, destination); err != nil {
		t.Fatal(err)
	}
	status = http.StatusNotFound
	if err := defaultHTTPGetFile(context.Background(), server.URL, nil, destination); err == nil {
		t.Fatal("failed download succeeded")
	}
	if err := defaultHTTPGetFile(context.Background(), ":", nil, destination); err == nil {
		t.Fatal("invalid download URL succeeded")
	}
	status = http.StatusOK
	if err := defaultHTTPGetFile(context.Background(), server.URL, nil, filepath.Join(t.TempDir(), "missing", "file")); err == nil {
		t.Fatal("uncreatable download path succeeded")
	}

	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
	SetHTTPPutFile(nil)
	SetHTTPGetFile(func(context.Context, string, map[string]string, string) error { return nil })
	SetHTTPGetFile(nil)
}

func TestCrossPlatformCoverageMailHTTPTransfersCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "attachment.bin")
	if err := os.WriteFile(file, []byte("mail"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "mail-body")
	}))
	defer server.Close()

	if err := httpPutMailAttachment(context.Background(), "PERSONAL", server.URL, file, 4); err != nil {
		t.Fatal(err)
	}
	status = http.StatusBadRequest
	if err := httpPutMailAttachment(context.Background(), "ENTERPRISE", server.URL, file, 4); err == nil {
		t.Fatal("failed mail upload succeeded")
	}
	if err := httpPutMailAttachment(context.Background(), "PERSONAL", server.URL, "missing", 0); err == nil {
		t.Fatal("missing mail upload file succeeded")
	}
	if err := httpPutMailAttachment(context.Background(), "PERSONAL", ":", file, 4); err == nil {
		t.Fatal("invalid mail upload URL succeeded")
	}

	status = http.StatusOK
	destination := filepath.Join(t.TempDir(), "mail.bin")
	if err := httpGetMailAttachment(context.Background(), "PERSONAL", server.URL, destination); err != nil {
		t.Fatal(err)
	}
	status = http.StatusBadRequest
	if err := httpGetMailAttachment(context.Background(), "ENTERPRISE", server.URL, destination); err == nil {
		t.Fatal("failed mail download succeeded")
	}
	if err := httpGetMailAttachment(context.Background(), "PERSONAL", ":", destination); err == nil {
		t.Fatal("invalid mail download URL succeeded")
	}
	status = http.StatusOK
	if err := httpGetMailAttachment(context.Background(), "PERSONAL", server.URL, filepath.Join(t.TempDir(), "missing", "file")); err == nil {
		t.Fatal("uncreatable mail path succeeded")
	}
}

func TestCrossPlatformCoverageChatMediaHTTPAndDocVersionsCoverage(t *testing.T) {
	previousTransport := http.DefaultTransport
	mode := "success"
	var requestQuery url.Values
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if mode == "transport-error" {
			return nil, errors.New("transport")
		}
		if req.Body != nil {
			if _, err := io.ReadAll(req.Body); err != nil {
				return nil, err
			}
		}
		requestQuery = req.URL.Query()
		status := http.StatusOK
		body := `{"access_token":"token","errcode":0}`
		if strings.Contains(req.URL.Path, "media/upload") {
			body = `{"media_id":"media-id","errcode":0}`
		}
		switch mode {
		case "http-error":
			status = http.StatusInternalServerError
			body = `failure`
		case "invalid-json":
			body = `{`
		case "api-error":
			body = `{"errcode":1,"errmsg":"failed"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")
	if _, err := mediaResolveAppToken(context.Background()); err == nil {
		t.Fatal("missing media credentials succeeded")
	}
	t.Setenv("DWS_CLIENT_ID", "client")
	t.Setenv("DWS_CLIENT_SECRET", "secret")
	for _, current := range []string{"transport-error", "http-error", "invalid-json", "api-error", "success"} {
		mode = current
		_, _ = mediaResolveAppToken(context.Background())
	}
	t.Setenv("DWS_CLIENT_ID", "client&scope=chat")
	t.Setenv("DWS_CLIENT_SECRET", "secret=with?reserved")
	mode = "success"
	if _, err := mediaResolveAppToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requestQuery.Get("appkey") != "client&scope=chat" || requestQuery.Get("appsecret") != "secret=with?reserved" {
		t.Fatalf("token query was not encoded: %v", requestQuery)
	}
	file := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(file, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, current := range []string{"transport-error", "http-error", "invalid-json", "api-error", "success"} {
		mode = current
		_, _ = mediaUploadFile(context.Background(), "token", file, "image")
	}
	mode = "success"
	if _, err := mediaUploadFile(context.Background(), "token&scope=chat", file, "image&type=file"); err != nil {
		t.Fatal(err)
	}
	if requestQuery.Get("access_token") != "token&scope=chat" || requestQuery.Get("type") != "image&type=file" {
		t.Fatalf("upload query was not encoded: %v", requestQuery)
	}
	mode = "success"
	_, _ = mediaUploadFile(context.Background(), "token", filepath.Join(t.TempDir(), "missing"), "image")

	requestFailure := func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("request")
	}
	if _, err := mediaResolveAppTokenWithRequest(context.Background(), requestFailure); err == nil {
		t.Fatal("token request construction failure returned nil")
	}
	if _, err := mediaUploadFileWithRequest(context.Background(), "token", file, "image", requestFailure); err == nil {
		t.Fatal("upload request construction failure returned nil")
	}

	for _, value := range []any{float64(3), float64(3.5), "3", "bad", jsonNumber("3"), jsonNumber("bad"), true} {
		_ = docVersionNumberMatches(value, 3)
	}
	for _, payload := range []any{
		map[string]any{"hasMore": false, "nextCursor": "ignored"},
		map[string]any{"nextToken": "next"},
		map[string]any{"result": map[string]any{"cursor": "cursor"}},
		[]any{map[string]any{"nextCursor": "array"}},
		"bad",
	} {
		_ = docVersionNextCursor(payload)
		_ = docVersionPayloadContains(payload, 3)
	}
}

func jsonNumber(value string) json.Number { return json.Number(value) }

func TestCrossPlatformCoverageRunDocReadJsonMLCoverage(t *testing.T) {
	previous := deps
	t.Cleanup(func() { deps = previous })
	caller := &helpersCoreCaller{format: "json"}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	for _, text := range []string{
		`{`,
		`{}`,
		`{"jsonml":"{\"type\":\"doc\"}","revision":2}`,
		`{"jsonml":"{\"type\":\"doc\"}","revision":"3"}`,
		`{"jsonml":"{\"type\":\"doc\"}","revision":"bad"}`,
	} {
		caller.result = &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
		_ = runDocReadJsonML(nil, "node", "")
	}
	caller.result = &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"jsonml":"{\"type\":\"doc\"}"}`}}}
	_ = runDocReadJsonML(nil, "node", filepath.Join(t.TempDir(), "out.json"))
	_ = runDocReadJsonML(nil, "node", filepath.Join(t.TempDir(), "missing", "out.json"))
}
