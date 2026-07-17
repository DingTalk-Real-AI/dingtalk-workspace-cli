package helpers

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
)

func geminiResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCrossPlatformCoverageGeminiAttachmentAndUploadFailureEdges(t *testing.T) {
	f := &geminiAPIForwarder{model: "model", apiKey: "key", baseURL: "https://example.test/v1beta", timeout: time.Second, httpClient: http.DefaultClient}
	parts, err := f.partsWithAttachments(context.Background(), "text", []connectMediaAttachment{{LocalPath: ""}})
	if err != nil || len(parts) != 1 {
		t.Fatalf("parts(empty path) = %#v, %v", parts, err)
	}
	if _, err := f.partsWithAttachments(context.Background(), "text", []connectMediaAttachment{{LocalPath: filepath.Join(t.TempDir(), "missing")}}); err == nil {
		t.Fatal("parts(missing path) error = nil")
	}
	if _, err := f.uploadFile(context.Background(), filepath.Join(t.TempDir(), "missing"), "text/plain", ""); err == nil {
		t.Fatal("uploadFile(missing) error = nil")
	}
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid := *f
	invalid.baseURL = "%"
	if _, err := invalid.uploadFile(context.Background(), path, "text/plain", ""); err == nil {
		t.Fatal("uploadFile(invalid endpoint) error = nil")
	}

	tests := []struct {
		name      string
		transport roundTripFunc
		want      string
	}{
		{"start transport", func(*http.Request) (*http.Response, error) { return nil, errors.New("start") }, "start"},
		{"start status", func(*http.Request) (*http.Response, error) { return geminiResponse(500, "bad", nil), nil }, "启动上传"},
		{"missing upload URL", func(*http.Request) (*http.Response, error) { return geminiResponse(200, "{}", nil), nil }, "X-Goog-Upload-URL"},
		{"invalid upload URL", func(*http.Request) (*http.Response, error) {
			return geminiResponse(200, "{}", map[string]string{"X-Goog-Upload-URL": ":"}), nil
		}, "missing protocol scheme"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *f
			candidate.httpClient = &http.Client{Transport: test.transport}
			if _, err := candidate.uploadFile(context.Background(), path, "text/plain", ""); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("uploadFile() error = %v, want %q", err, test.want)
			}
		})
	}

	sequenceTest := func(t *testing.T, second *http.Response, secondErr error, want string) {
		t.Helper()
		calls := 0
		candidate := *f
		candidate.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return geminiResponse(200, "{}", map[string]string{"X-Goog-Upload-URL": "https://upload.test/session"}), nil
			}
			return second, secondErr
		})}
		_, err := candidate.uploadFile(context.Background(), path, "text/plain", "")
		if want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("uploadFile() error = %v, want %q", err, want)
		}
	}
	t.Run("upload transport", func(t *testing.T) { sequenceTest(t, nil, errors.New("upload"), "upload") })
	t.Run("upload status", func(t *testing.T) { sequenceTest(t, geminiResponse(500, "bad", nil), nil, "上传 HTTP") })
	t.Run("invalid result", func(t *testing.T) { sequenceTest(t, geminiResponse(200, "bad", nil), nil, "invalid") })
	t.Run("missing uri", func(t *testing.T) {
		sequenceTest(t, geminiResponse(200, `{"file":{"state":"ACTIVE"}}`, nil), nil, "file.uri")
	})
	t.Run("mime fallback", func(t *testing.T) {
		sequenceTest(t, geminiResponse(200, `{"file":{"uri":"uri","state":"ACTIVE"}}`, nil), nil, "")
	})

	removable := filepath.Join(t.TempDir(), "remove-after-start")
	if err := os.WriteFile(removable, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	removeCandidate := *f
	removeCandidate.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		_ = os.Remove(removable)
		return geminiResponse(200, "{}", map[string]string{"X-Goog-Upload-URL": "https://upload.test/session"}), nil
	})}
	if _, err := removeCandidate.uploadFile(context.Background(), removable, "text/plain", ""); err == nil {
		t.Fatal("uploadFile(open after removal) error = nil")
	}
}

func TestCrossPlatformCoverageGeminiUploadedFilePollingEdges(t *testing.T) {
	originalInterval := geminiFilePollInterval
	t.Cleanup(func() { geminiFilePollInterval = originalInterval })
	f := &geminiAPIForwarder{apiKey: "key", baseURL: "https://example.test/v1beta", httpClient: http.DefaultClient}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	geminiFilePollInterval = time.Hour
	if _, err := f.waitForUploadedFile(ctx, geminiUploadedFile{State: "PROCESSING"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForUploadedFile(cancelled) = %v", err)
	}
	geminiFilePollInterval = 0

	tests := []struct {
		name      string
		baseURL   string
		response  *http.Response
		err       error
		wantError string
	}{
		{"bad endpoint", "%", nil, nil, "BASE_URL"},
		{"transport", "https://example.test/v1beta", nil, errors.New("poll"), "poll"},
		{"status", "https://example.test/v1beta", geminiResponse(500, "bad", nil), nil, "查询 HTTP"},
		{"invalid json", "https://example.test/v1beta", geminiResponse(200, "bad", nil), nil, "invalid"},
		{"failed", "https://example.test/v1beta", geminiResponse(200, `{"name":"files/1","state":"FAILED"}`, nil), nil, "处理附件失败"},
		{"active", "https://example.test/v1beta", geminiResponse(200, `{"name":"files/1","state":"ACTIVE","uri":"uri"}`, nil), nil, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *f
			candidate.baseURL = test.baseURL
			candidate.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return test.response, test.err })}
			_, err := candidate.waitForUploadedFile(context.Background(), geminiUploadedFile{Name: "/files/1", State: "PROCESSING"})
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("waitForUploadedFile() error = %v, want %q", err, test.wantError)
			}
		})
	}
	if endpoint, err := (&geminiAPIForwarder{baseURL: "https://example.test/custom"}).filesEndpoint(true); err != nil || !strings.Contains(endpoint, "/custom/upload/v1beta/files") {
		t.Fatalf("filesEndpoint(custom upload) = %q, %v", endpoint, err)
	}
	if endpoint, err := (&geminiAPIForwarder{baseURL: "https://example.test/v1beta/files"}).filesEndpoint(false); err != nil || strings.Count(endpoint, "/files") != 1 {
		t.Fatalf("filesEndpoint(files) = %q, %v", endpoint, err)
	}
}
