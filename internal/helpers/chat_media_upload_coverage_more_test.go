package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatMediaUploadCommandRemainingCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(file, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := executeFilterCoverage(t, newChatMediaUploadCommand(), "--file", t.TempDir()); err == nil {
		t.Fatal("directory upload returned nil")
	}

	origResolve, origUpload := chatMediaResolveAppToken, chatMediaUploadFile
	t.Cleanup(func() {
		chatMediaResolveAppToken = origResolve
		chatMediaUploadFile = origUpload
	})
	chatMediaResolveAppToken = func(context.Context) (string, error) { return "", errors.New("token") }
	if err := executeFilterCoverage(t, newChatMediaUploadCommand(), "--file", file); err == nil {
		t.Fatal("token failure returned nil")
	}
	chatMediaResolveAppToken = func(context.Context) (string, error) { return "token", nil }
	chatMediaUploadFile = func(context.Context, string, string, string) (string, error) { return "", errors.New("upload") }
	if err := executeFilterCoverage(t, newChatMediaUploadCommand(), "--file", file); err == nil {
		t.Fatal("upload failure returned nil")
	}
	var mediaType string
	chatMediaUploadFile = func(_ context.Context, _, _, typ string) (string, error) {
		mediaType = typ
		return "media-id", nil
	}
	cmd := newChatMediaUploadCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--file", file, "--type", " "})
	if err := cmd.Execute(); err != nil || mediaType != "image" || !strings.Contains(out.String(), "media-id") {
		t.Fatalf("success type=%q output=%q err=%v", mediaType, out.String(), err)
	}
}

func TestCrossPlatformCoverageMediaUploadMultipartWriterRemainingFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(file, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	origTransport := http.DefaultTransport
	origCreate, origOpen, origCopy := mediaCreateFormFile, mediaOpenFile, mediaCopyFile
	t.Cleanup(func() {
		http.DefaultTransport = origTransport
		mediaCreateFormFile, mediaOpenFile, mediaCopyFile = origCreate, origOpen, origCopy
	})
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"media_id":"id"}`)), Request: req}, nil
	})

	mediaCreateFormFile = func(*multipart.Writer, string, string) (io.Writer, error) { return nil, errors.New("part") }
	if _, err := mediaUploadFile(context.Background(), "token", file, "image"); err == nil {
		t.Fatal("form part failure returned nil")
	}
	mediaCreateFormFile = origCreate
	mediaOpenFile = func(string) (*os.File, error) { return nil, errors.New("open") }
	if _, err := mediaUploadFile(context.Background(), "token", file, "image"); err == nil {
		t.Fatal("open failure returned nil")
	}
	mediaOpenFile = origOpen
	mediaCopyFile = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
	if _, err := mediaUploadFile(context.Background(), "token", file, "image"); err == nil {
		t.Fatal("copy failure returned nil")
	}
}
