package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
)

type uploadFileRunner struct {
	last   executor.Invocation
	result executor.Result
}

func (r *uploadFileRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.last = invocation
	return r.result, nil
}

func TestAITableUploadFileUnwrapsRuntimeContent(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "report.txt")
	wantBody := []byte("hello from upload-file")
	if err := os.WriteFile(filePath, wantBody, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var gotBody []byte
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", req.Method)
		}
		gotContentType = req.Header.Get("Content-Type")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := &uploadFileRunner{
		result: executor.Result{
			Response: map[string]any{
				"content": map[string]any{
					"data": map[string]any{
						"uploadUrl": server.URL,
						"fileToken": "ft_test_123",
					},
				},
			},
		},
	}

	cmd := newAITableUploadFileCommand(runner)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--base-id", "BASE_001", "--file", filePath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}

	if runner.last.Tool != "prepare_attachment_upload" {
		t.Fatalf("tool = %q, want prepare_attachment_upload", runner.last.Tool)
	}
	if got := runner.last.Params["fileName"]; got != "report.txt" {
		t.Fatalf("fileName = %#v, want report.txt", got)
	}
	if string(gotBody) != string(wantBody) {
		t.Fatalf("uploaded body = %q, want %q", string(gotBody), string(wantBody))
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/plain; charset=utf-8", gotContentType)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if got := payload["fileToken"]; got != "ft_test_123" {
		t.Fatalf("fileToken = %#v, want ft_test_123", got)
	}
}
