package chat

import (
	"context"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOptimizationResourceFaultsNeverPublishPartialBytes(t *testing.T) {
	for _, name := range []string{"invalid-url", "nil-context", "missing-dir", "cancel-network", "network", "cancel-delay", "headers", "weak-etag", "too-many-parts", "bad-range", "short-body", "content-length", "not-partial", "no-attempt", "copy", "sync", "close", "publish", "exists", "overwrite-publish"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "file")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requestContext context.Context = ctx
			url := "https://files.example.com/object"
			headers := map[string]string{}
			retries, delay := 0, 0
			overwrite := false
			calls := 0
			switch name {
			case "invalid-url":
				url = "bad://object"
			case "nil-context":
				requestContext = nil
			case "missing-dir":
				dest = filepath.Join(dir, "missing", "file")
			case "cancel-network":
				cancel()
			case "cancel-delay":
				retries = 1
				delay = 1000
			case "headers":
				headers["X-Fixture"] = "value"
			case "no-attempt":
				retries = -1
			case "exists":
				if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			case "overwrite-publish":
				overwrite = true
			}
			if name == "copy" {
				testseam.Swap(t, &resourceCopy, func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("disk full") })
			}
			if name == "sync" {
				testseam.Swap(t, &resourceTempSync, func(*os.File) error { return errors.New("sync failed") })
			}
			if name == "close" {
				testseam.Swap(t, &resourceTempClose, func(*os.File) error { return errors.New("close failed") })
			}
			if name == "publish" {
				testseam.Swap(t, &resourceLink, func(string, string) error { return errors.New("link failed") })
			}
			if name == "overwrite-publish" {
				testseam.Swap(t, &resourceRename, func(string, string) error { return errors.New("rename failed") })
			}
			client := &http.Client{Transport: optimizationRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				status := 206
				body := "data"
				h := http.Header{}
				h.Set("Content-Range", "bytes 0-3/4")
				h.Set("ETag", `"v1"`)
				length := int64(4)
				switch name {
				case "network", "cancel-network":
					return nil, errors.New("connection reset")
				case "cancel-delay":
					cancel()
					status = 503
				case "headers":
					if req.Header.Get("X-Fixture") != "value" {
						t.Fatal("scoped header lost")
					}
				case "weak-etag":
					if calls == 1 {
						h.Set("ETag", `W/"v1"`)
					} else {
						status = 200
						if req.Header.Get("Range") != "" {
							t.Fatal("full fallback kept Range")
						}
					}
				case "too-many-parts":
					h.Set("Content-Range", "bytes 0-3/50000")
				case "bad-range":
					h.Set("Content-Range", "invalid")
				case "short-body":
					body = "da"
					length = 2
				case "content-length":
					length = 3
				case "not-partial":
					status = 404
				}
				return &http.Response{StatusCode: status, Header: h, ContentLength: length, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			n, err := downloadWithRecovery(requestContext, client, url, headers, dest, overwrite, 4, retries, delay, func() (string, map[string]string, error) { return url, headers, nil })
			success := name == "headers" || name == "weak-etag"
			if (err == nil) != success {
				t.Fatalf("%s result=%d err=%v calls=%d", name, n, err, calls)
			}
			data, readErr := os.ReadFile(dest)
			if success {
				if readErr != nil || string(data) != "data" {
					t.Fatalf("wrong bytes %q %v", data, readErr)
				}
			} else if name == "exists" {
				if string(data) != "original" {
					t.Fatal("existing file damaged")
				}
			} else if !os.IsNotExist(readErr) {
				t.Fatal("failure published partial file", readErr)
			}
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				if strings.Contains(f.Name(), ".range-") {
					t.Fatal("temporary file leaked")
				}
			}
		})
	}
}
