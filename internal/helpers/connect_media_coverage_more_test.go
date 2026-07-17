package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func mediaCoverageResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func mediaCoverageClient(transport roundTripFunc) *aiCardClient {
	client := newAICardClient("client", "secret", "")
	client.token = "token"
	client.tokenExp = time.Now().Add(time.Hour)
	client.httpClient = &http.Client{Transport: transport}
	return client
}

func TestCrossPlatformCoverageConnectMediaPureFunctionRemainingCoverage(t *testing.T) {
	info := parseFileInbound(map[string]interface{}{
		"filePath": " /tmp/file ", "fileType": " text/plain ",
		"dentryID": int64(1), "spaceID": int(2), "size": json.Number("3"),
	})
	if info.FilePath != "/tmp/file" || info.FileType != "text/plain" || info.DentryID != 1 || info.SpaceID != 2 || info.FileSize != 3 {
		t.Fatalf("parsed alternate fields = %#v", info)
	}
	for _, value := range []interface{}{json.Number("bad"), "bad", true} {
		if got := readInt64Field(map[string]interface{}{"value": value}, "missing", "value"); got != 0 {
			t.Errorf("readInt64Field(%v) = %d", value, got)
		}
	}
	if got := summarizeContent(make(chan int)); !strings.Contains(got, "unmarshalable") {
		t.Fatalf("unmarshalable summary = %q", got)
	}
	if got := summarizeContent(strings.Repeat("x", 500)); len(got) <= 400 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long summary length/suffix = %d/%q", len(got), got[len(got)-3:])
	}
	leaves := cardContentLeaves([]interface{}{1, map[string]interface{}{"value": "text"}})
	if len(leaves) != 1 || leaves[0] != "text" {
		t.Fatalf("card leaves = %#v", leaves)
	}
	for _, tc := range []struct{ rawURL, contentType, want string }{
		{"https://x/y", "image/gif", ".gif"},
		{"https://x/y", "image/webp", ".webp"},
		{"https://x/file.toolong", "", ".png"},
		{"%", "", ".png"},
	} {
		if got := mediaExt(tc.rawURL, tc.contentType); got != tc.want {
			t.Errorf("mediaExt(%q, %q) = %q, want %q", tc.rawURL, tc.contentType, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageDownloadMessageFileRemainingCoverage(t *testing.T) {
	withCardAPIBase(t, "https://api.test")
	boom := errors.New("media failure")
	originalMkdir := mediaMkdirAll
	originalCreate := mediaCreate
	originalCopy := mediaCopy
	originalRemove := mediaRemove
	t.Cleanup(func() {
		mediaMkdirAll = originalMkdir
		mediaCreate = originalCreate
		mediaCopy = originalCopy
		mediaRemove = originalRemove
	})

	client := mediaCoverageClient(func(*http.Request) (*http.Response, error) { return nil, boom })
	if _, err := client.downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("API error returned nil")
	}

	client = mediaCoverageClient(func(req *http.Request) (*http.Response, error) {
		return mediaCoverageResponse(200, `{"downloadUrl":":"}`, nil), nil
	})
	if _, err := client.downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("invalid download URL returned nil")
	}

	client = mediaCoverageClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.test" {
			return mediaCoverageResponse(200, `{"downloadUrl":"https://download.test/file"}`, nil), nil
		}
		return nil, boom
	})
	if _, err := client.downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("download transport error returned nil")
	}

	statusClient := func(status int) *aiCardClient {
		return mediaCoverageClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "api.test" {
				return mediaCoverageResponse(200, `{"downloadUrl":"https://download.test/file"}`, nil), nil
			}
			return mediaCoverageResponse(status, "body", map[string]string{"Content-Type": "image/png"}), nil
		})
	}
	if _, err := statusClient(500).downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("HTTP status error returned nil")
	}

	mediaMkdirAll = func(string, os.FileMode) error { return boom }
	if _, err := statusClient(200).downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("mkdir error returned nil")
	}
	mediaMkdirAll = originalMkdir
	mediaCreate = func(string) (*os.File, error) { return nil, boom }
	if _, err := statusClient(200).downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("create error returned nil")
	}
	mediaCreate = originalCreate
	mediaCopy = func(io.Writer, io.Reader) (int64, error) { return 0, boom }
	if _, err := statusClient(200).downloadMessageFile(context.Background(), "robot", "code"); err == nil {
		t.Fatal("copy error returned nil")
	}
	mediaCopy = originalCopy
}

func TestCrossPlatformCoverageDentryMediaRemainingCoverage(t *testing.T) {
	withCardAPIBase(t, "https://api.test")
	boom := errors.New("dentry failure")

	client := mediaCoverageClient(func(*http.Request) (*http.Response, error) { return nil, boom })
	if _, err := client.getUserUnionID(context.Background(), "user"); err == nil {
		t.Fatal("union API error returned nil")
	}
	client = mediaCoverageClient(func(*http.Request) (*http.Response, error) {
		return mediaCoverageResponse(200, `{}`, nil), nil
	})
	if _, err := client.getUserUnionID(context.Background(), "user"); err == nil {
		t.Fatal("missing union ID returned nil")
	}

	client = mediaCoverageClient(func(*http.Request) (*http.Response, error) { return nil, boom })
	if _, err := client.downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil {
		t.Fatal("download info API error returned nil")
	}
	client = mediaCoverageClient(func(*http.Request) (*http.Response, error) {
		return mediaCoverageResponse(200, `{}`, nil), nil
	})
	if _, err := client.downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil {
		t.Fatal("missing resource URL returned nil")
	}
	client = mediaCoverageClient(func(*http.Request) (*http.Response, error) {
		return mediaCoverageResponse(200, `{"resourceUrl":":"}`, nil), nil
	})
	if _, err := client.downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil {
		t.Fatal("invalid resource URL returned nil")
	}

	dentryClient := func(status int, downloadErr error) *aiCardClient {
		return mediaCoverageClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "api.test" {
				return mediaCoverageResponse(200, `{"resourceUrl":"https://download.test/file","headers":{"X-Test":"value"}}`, nil), nil
			}
			if downloadErr != nil {
				return nil, downloadErr
			}
			if req.Header.Get("X-Test") != "value" {
				t.Errorf("download header = %q", req.Header.Get("X-Test"))
			}
			return mediaCoverageResponse(status, "body", map[string]string{"Content-Type": "image/gif"}), nil
		})
	}
	if _, err := dentryClient(200, boom).downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil {
		t.Fatal("download transport error returned nil")
	}
	if _, err := dentryClient(500, nil).downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil {
		t.Fatal("download HTTP status returned nil")
	}
	path, err := dentryClient(200, nil).downloadDentryFile(context.Background(), 1, 2, "union", "")
	if err != nil {
		t.Fatalf("extension fallback success: %v", err)
	}
	_ = os.Remove(path)

	originalMkdir := mediaMkdirAll
	originalCreate := mediaCreate
	originalCopy := mediaCopy
	t.Cleanup(func() {
		mediaMkdirAll = originalMkdir
		mediaCreate = originalCreate
		mediaCopy = originalCopy
	})
	mediaMkdirAll = func(string, os.FileMode) error { return boom }
	if _, err := dentryClient(200, nil).downloadDentryFile(context.Background(), 1, 2, "union", ""); err == nil {
		t.Fatal("dentry mkdir error returned nil")
	}
	mediaMkdirAll = originalMkdir
	mediaCreate = func(string) (*os.File, error) { return nil, boom }
	if _, err := dentryClient(200, nil).downloadDentryFile(context.Background(), 1, 2, "union", "file.txt"); err == nil {
		t.Fatal("dentry create error returned nil")
	}
	mediaCreate = originalCreate
	mediaCopy = func(io.Writer, io.Reader) (int64, error) { return 0, boom }
	if _, err := dentryClient(200, nil).downloadDentryFile(context.Background(), 1, 2, "union", "file.txt"); err == nil {
		t.Fatal("dentry copy error returned nil")
	}
}
