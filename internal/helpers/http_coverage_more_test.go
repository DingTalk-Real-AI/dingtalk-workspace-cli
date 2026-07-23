package helpers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestCrossPlatformCoverageDocVersionsCoverage(t *testing.T) {
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
