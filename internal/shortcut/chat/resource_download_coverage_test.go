// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func resetResourceDownloadHooks(t *testing.T) {
	t.Helper()
	getwd := resourceGetwd
	abs := resourceAbs
	eval := resourceEvalSymlinks
	stat := resourceStat
	lstat := resourceLstat
	rel := resourceRel
	mkdir := resourceMkdir
	createTemp := resourceCreateTemp
	copyFn := resourceCopy
	syncFn := resourceTempSync
	closeFn := resourceTempClose
	renameFn := resourceRename
	linkFn := resourceLink
	downloadFn := resourceDownload
	t.Cleanup(func() {
		resourceGetwd = getwd
		resourceAbs = abs
		resourceEvalSymlinks = eval
		resourceStat = stat
		resourceLstat = lstat
		resourceRel = rel
		resourceMkdir = mkdir
		resourceCreateTemp = createTemp
		resourceCopy = copyFn
		resourceTempSync = syncFn
		resourceTempClose = closeFn
		resourceRename = renameFn
		resourceLink = linkFn
		resourceDownload = downloadFn
	})
}

func TestCrossPlatformCoverageResourceDownloadCommandOutcomes(t *testing.T) {
	baseArgs := []string{
		"chat", "+messages-resource-download",
		"--resource-id", "@image",
		"--message-id", "msg",
		"--open-conversation-id", "cid",
		"--yes",
	}
	t.Run("dry run", func(t *testing.T) {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append(append([]string{}, baseArgs...), "--dry-run"))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("lower error", func(t *testing.T) {
		helpers.InitDeps(&larkAlignmentCaller{failProductTool: "im/get_resource_download_url"})
		root := newPlatformCoverageRoot()
		root.SetArgs(baseArgs)
		if err := root.Execute(); err == nil {
			t.Fatal("lower error was swallowed")
		}
	})
	t.Run("invalid download info", func(t *testing.T) {
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/get_resource_download_url": `{"result":{}}`,
		}})
		root := newPlatformCoverageRoot()
		root.SetArgs(baseArgs)
		if err := root.Execute(); err == nil {
			t.Fatal("missing URL was accepted")
		}
	})
	t.Run("getwd error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceGetwd = func() (string, error) { return "", errors.New("getwd") }
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/file"}}`,
		}})
		root := newPlatformCoverageRoot()
		root.SetArgs(baseArgs)
		if err := root.Execute(); err == nil {
			t.Fatal("getwd error was swallowed")
		}
	})
	t.Run("path error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceAbs = func(string) (string, error) { return "", errors.New("path") }
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/file"}}`,
		}})
		root := newPlatformCoverageRoot()
		root.SetArgs(baseArgs)
		if err := root.Execute(); err == nil {
			t.Fatal("path error was swallowed")
		}
	})
	for _, tc := range []struct {
		name      string
		download  func(context.Context, *http.Client, string, map[string]string, string, bool) (int64, error)
		wantError bool
	}{
		{
			name: "download error",
			download: func(context.Context, *http.Client, string, map[string]string, string, bool) (int64, error) {
				return 0, errors.New("download")
			},
			wantError: true,
		},
		{
			name: "success",
			download: func(context.Context, *http.Client, string, map[string]string, string, bool) (int64, error) {
				return 7, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetResourceDownloadHooks(t)
			resourceDownload = tc.download
			helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
				"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/file"}}`,
			}})
			root := newPlatformCoverageRoot()
			root.SetArgs(append(append([]string{}, baseArgs...), "--output", filepath.Join(t.TempDir(), "file")))
			// The absolute path is intentionally rejected by the public command,
			// so use a relative path while running from the repository cwd.
			root.SetArgs(append(append([]string{}, baseArgs...), "--output", "coverage-resource.bin", "--overwrite"))
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestCrossPlatformCoverageResourceDownloadValidationAndInfo(t *testing.T) {
	for _, output := range []string{"", "/absolute", "../escape"} {
		if err := validateResourceDownloadOutput(output); err == nil {
			t.Errorf("unsafe output %q accepted", output)
		}
	}
	for _, data := range []map[string]any{
		{},
		{"resourceUrl": []any{42, ""}},
		{"resourceUrl": "://bad"},
		{"resourceUrl": "http://example.test/file"},
	} {
		if _, _, err := resourceDownloadInfo(data); err == nil {
			t.Errorf("invalid resource info accepted: %#v", data)
		}
	}
	resourceURL, headers, err := resourceDownloadInfo(map[string]any{
		"resourceUrl": "https://download.dingtalk.com/file",
		"headers": map[string]any{
			"":        "ignored",
			"X-Count": 3,
			"X-Test":  "ok",
		},
	})
	if err != nil || resourceURL == "" || len(headers) != 1 || headers["X-Test"] != "ok" {
		t.Fatalf("resource info = %q %#v %v", resourceURL, headers, err)
	}
	for host, want := range map[string]bool{
		"ALIYUNCS.COM.":                                false,
		"bucket.aliyuncs.com":                          false,
		"bucket.oss-cn-hangzhou.aliyuncs.com":          true,
		"bucket.oss-internal.aliyuncs.com":             false,
		"bucket.oss-cn-hangzhou-internal.aliyuncs.com": false,
		"example.com":                                  false,
	} {
		if got := isAliyunOSSHost(host); got != want {
			t.Errorf("isAliyunOSSHost(%q) = %v, want %v", host, got, want)
		}
	}
	for host, want := range map[string]bool{
		"DINGTALK.COM.":                       true,
		"download.dingtalk.com":               true,
		"bucket.oss-cn-hangzhou.aliyuncs.com": true,
		"aliyuncs.com.evil.test":              false,
		"evildingtalk.com":                    false,
		"download.example.invalid":            false,
	} {
		if got := isResourceDownloadAllowedHost(host); got != want {
			t.Errorf("isResourceDownloadAllowedHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestCrossPlatformCoverageResourceDownloadPathErrors(t *testing.T) {
	t.Run("absolute resolution", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceAbs = func(string) (string, error) { return "", errors.New("abs") }
		if _, _, err := resolveResourceDownloadPath(".", "file", "https://example.test/file", false); err == nil {
			t.Fatal("absolute-path error was swallowed")
		}
	})
	t.Run("base symlink resolution", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceEvalSymlinks = func(string) (string, error) { return "", errors.New("eval") }
		if _, _, err := resolveResourceDownloadPath(t.TempDir(), "file", "https://example.test/file", false); err == nil {
			t.Fatal("base eval error was swallowed")
		}
	})
	t.Run("parent symlink resolution", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		calls := 0
		resourceEvalSymlinks = func(path string) (string, error) {
			calls++
			if calls == 2 {
				return "", errors.New("parent eval")
			}
			return filepath.EvalSymlinks(path)
		}
		if _, _, err := resolveResourceDownloadPath(t.TempDir(), "file", "https://example.test/file", false); err == nil {
			t.Fatal("parent eval error was swallowed")
		}
	})
	t.Run("parent escape", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		calls := 0
		resourceRel = func(base, target string) (string, error) {
			calls++
			if calls == 2 {
				return "../outside", nil
			}
			return filepath.Rel(base, target)
		}
		if _, _, err := resolveResourceDownloadPath(t.TempDir(), "file", "https://example.test/file", false); err == nil {
			t.Fatal("parent escape was accepted")
		}
	})
	t.Run("target inspection", func(t *testing.T) {
		base := t.TempDir()
		existing := filepath.Join(base, "existing")
		if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := resolveResourceDownloadPath(base, "existing", "https://example.test/file", false); err == nil {
			t.Fatal("existing file was accepted without overwrite")
		}
		if _, _, err := resolveResourceDownloadPath(base, "existing", "https://example.test/file", true); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(base, "dir")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := resolveResourceDownloadPath(base, filepath.Join("dir", "child")+"/", "https://example.test/file", false); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(existing, link); err == nil {
			if _, _, resolveErr := resolveResourceDownloadPath(base, "link", "https://example.test/file", false); resolveErr == nil {
				t.Fatal("symlink target was accepted")
			}
		}
	})
	t.Run("lstat error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceLstat = func(string) (os.FileInfo, error) { return nil, errors.New("lstat") }
		if _, _, err := resolveResourceDownloadPath(t.TempDir(), "file", "https://example.test/file", false); err == nil {
			t.Fatal("lstat error was swallowed")
		}
	})
	t.Run("target becomes directory", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		base := t.TempDir()
		resourceStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		resourceLstat = func(string) (os.FileInfo, error) {
			return os.Stat(base)
		}
		if _, _, err := resolveResourceDownloadPath(base, "file", "https://example.test/file", false); err == nil {
			t.Fatal("directory target was accepted")
		}
	})
	t.Run("final relative error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		calls := 0
		resourceRel = func(base, target string) (string, error) {
			calls++
			if calls == 3 {
				return "", errors.New("rel")
			}
			return filepath.Rel(base, target)
		}
		if _, _, err := resolveResourceDownloadPath(t.TempDir(), "file", "https://example.test/file", false); err == nil {
			t.Fatal("final relative error was swallowed")
		}
	})
}

func TestCrossPlatformCoverageEnsureResourceDownloadParentErrors(t *testing.T) {
	base := t.TempDir()
	if err := ensureResourceDownloadParent(base, filepath.Join(base, "..", "outside")); err == nil {
		t.Fatal("outside parent was accepted")
	}
	if err := ensureResourceDownloadParent(base, base); err != nil {
		t.Fatal(err)
	}
	t.Run("mkdir and recheck error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceMkdir = func(string, os.FileMode) error { return errors.New("mkdir") }
		resourceLstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		if err := ensureResourceDownloadParent(base, filepath.Join(base, "new")); err == nil {
			t.Fatal("mkdir error was swallowed")
		}
	})
	t.Run("concurrent creator", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		target := filepath.Join(base, "concurrent")
		resourceMkdir = func(path string, mode os.FileMode) error {
			if err := os.Mkdir(path, mode); err != nil {
				t.Fatal(err)
			}
			return errors.New("lost race")
		}
		if err := ensureResourceDownloadParent(base, target); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("inspection error", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceLstat = func(string) (os.FileInfo, error) { return nil, errors.New("inspect") }
		if err := ensureResourceDownloadParent(base, filepath.Join(base, "inspect")); err == nil {
			t.Fatal("inspection error was swallowed")
		}
	})
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureResourceDownloadParent(base, file); err == nil {
		t.Fatal("file parent was accepted")
	}
	link := filepath.Join(base, "link-parent")
	if err := os.Symlink(base, link); err == nil {
		if err := ensureResourceDownloadParent(base, link); err == nil {
			t.Fatal("symlink parent was accepted")
		}
	}
}

func TestCrossPlatformCoverageResourceDownloadFilenameFallbacks(t *testing.T) {
	for _, resourceURL := range []string{"://bad", "https://example.test/", "https://example.test/%zz"} {
		if got := resourceDownloadFilename(resourceURL); got != "download" {
			t.Errorf("resourceDownloadFilename(%q) = %q", resourceURL, got)
		}
	}
	if got := resourceDownloadFilename("https://example.test/a%20b.txt"); got != "a b.txt" {
		t.Fatalf("decoded filename = %q", got)
	}
	for _, unsafeName := range []string{
		"..",
		"CON",
		"nul.txt",
		"COM1.log",
		"trailing.",
		"trailing ",
		"line\nbreak.txt",
		`bad:name.txt`,
	} {
		got := resourceDownloadFilename(
			"https://download.dingtalk.com/fallback.bin",
			unsafeName,
		)
		if got != "fallback.bin" {
			t.Errorf("unsafe preferred name %q produced %q", unsafeName, got)
		}
	}
}

type resourceRoundTripper func(*http.Request) (*http.Response, error)

func (f resourceRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func resourceResponseClient(status int, body string, length int64) *http.Client {
	return &http.Client{Transport: resourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    status,
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: length,
			Header:        make(http.Header),
		}, nil
	})}
}

func TestCrossPlatformCoverageDownloadResourceHTTPFailures(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "resource")
	if _, err := downloadResourceAtomically(context.Background(), nil, ":", nil, dest, false); err == nil {
		t.Fatal("invalid request URL was accepted")
	}
	errorClient := &http.Client{Transport: resourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	if _, err := downloadResourceAtomically(context.Background(), errorClient, "https://download.dingtalk.com/file", nil, dest, false); err == nil {
		t.Fatal("transport error was swallowed")
	}
	if _, err := downloadResourceAtomically(context.Background(), resourceResponseClient(500, "", 0), "https://download.dingtalk.com/file", nil, dest, false); err == nil {
		t.Fatal("HTTP 500 was accepted")
	}
	if _, err := downloadResourceAtomically(context.Background(), resourceResponseClient(200, "x", 2), "https://download.dingtalk.com/file", nil, dest, false); err == nil {
		t.Fatal("content-length mismatch was accepted")
	}
	nilClientDest := filepath.Join(t.TempDir(), "nil-client")
	if _, err := downloadResourceAtomically(
		context.Background(), nil, "https://evil.example/file",
		map[string]string{"X-Test": "ok"}, nilClientDest, true,
	); err == nil {
		t.Fatal("nil-client path accepted an untrusted URL")
	}
}

func TestCrossPlatformCoverageDownloadResourceRedirectGuards(t *testing.T) {
	httpRedirect := &http.Client{Transport: resourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{"Location": []string{"http://evil.example/file"}},
		}, nil
	})}
	if _, err := downloadResourceAtomically(
		context.Background(), httpRedirect, "https://download.dingtalk.com/start", nil,
		filepath.Join(t.TempDir(), "http-redirect"), false,
	); err == nil {
		t.Fatal("HTTP redirect was accepted")
	}

	loop := &http.Client{Transport: resourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{"Location": []string{"https://download.dingtalk.com/loop"}},
		}, nil
	})}
	if _, err := downloadResourceAtomically(
		context.Background(), loop, "https://download.dingtalk.com/loop", nil,
		filepath.Join(t.TempDir(), "loop"), false,
	); err == nil {
		t.Fatal("redirect loop was accepted")
	}

	original := *loop
	original.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("original redirect policy")
	}
	if _, err := downloadResourceAtomically(
		context.Background(), &original, "https://download.dingtalk.com/loop", nil,
		filepath.Join(t.TempDir(), "original"), false,
	); err == nil {
		t.Fatal("original redirect rejection was swallowed")
	}

	redirectCount := 0
	crossHost := &http.Client{Transport: resourceRoundTripper(func(request *http.Request) (*http.Response, error) {
		redirectCount++
		switch redirectCount {
		case 1:
			if request.Header.Get("X-Resource-Token") != "secret" {
				t.Fatal("initial signed header missing")
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     http.Header{"Location": []string{"https://bucket.oss-cn-hangzhou.aliyuncs.com/intermediate"}},
			}, nil
		case 2:
			if request.Header.Get("X-Resource-Token") != "" {
				t.Fatal("server-supplied header leaked on first cross-host hop")
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     http.Header{"Location": []string{"https://bucket.oss-cn-hangzhou.aliyuncs.com/final"}},
			}, nil
		default:
			if request.Header.Get("X-Resource-Token") != "" {
				t.Fatal("server-supplied header was restored on later same-host hop")
			}
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
			Header:        make(http.Header),
		}, nil
	})}
	if _, err := downloadResourceAtomically(
		context.Background(), crossHost, "https://download.dingtalk.com/start",
		map[string]string{"X-Resource-Token": "secret"},
		filepath.Join(t.TempDir(), "cross-host"), false,
	); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDownloadResourceFileFailures(t *testing.T) {
	client := resourceResponseClient(http.StatusOK, "body", 4)
	run := func(t *testing.T, overwrite bool) error {
		t.Helper()
		_, err := downloadResourceAtomically(
			context.Background(), client, "https://download.dingtalk.com/file", nil,
			filepath.Join(t.TempDir(), "resource"), overwrite)
		return err
	}
	t.Run("create temp", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("create") }
		if err := run(t, false); err == nil {
			t.Fatal("create-temp error was swallowed")
		}
	})
	t.Run("copy", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
		if err := run(t, false); err == nil {
			t.Fatal("copy error was swallowed")
		}
	})
	t.Run("sync", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceTempSync = func(*os.File) error { return errors.New("sync") }
		if err := run(t, false); err == nil {
			t.Fatal("sync error was swallowed")
		}
	})
	t.Run("close", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		calls := 0
		resourceTempClose = func(file *os.File) error {
			calls++
			if calls == 1 {
				return errors.New("close")
			}
			return file.Close()
		}
		if err := run(t, false); err == nil {
			t.Fatal("close error was swallowed")
		}
	})
	t.Run("rename", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceRename = func(string, string) error { return errors.New("rename") }
		if err := run(t, true); err == nil {
			t.Fatal("rename error was swallowed")
		}
	})
	t.Run("link exists", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceLink = func(string, string) error { return os.ErrExist }
		if err := run(t, false); err == nil {
			t.Fatal("link-exists error was swallowed")
		}
	})
	t.Run("link other", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceLink = func(string, string) error { return errors.New("link") }
		if err := run(t, false); err == nil {
			t.Fatal("link error was swallowed")
		}
	})
}

func TestCrossPlatformCoverageDownloadResourceCopySuccessWithBuffer(t *testing.T) {
	resetResourceDownloadHooks(t)
	var copied bytes.Buffer
	resourceCopy = func(writer io.Writer, reader io.Reader) (int64, error) {
		body, err := io.ReadAll(reader)
		if err != nil {
			return 0, err
		}
		copied.Write(body)
		written, writeErr := writer.Write(body)
		return int64(written), writeErr
	}
	dest := filepath.Join(t.TempDir(), "resource")
	if _, err := downloadResourceAtomically(
		context.Background(), resourceResponseClient(200, "ok", 2),
		"https://download.dingtalk.com/file", nil, dest, true,
	); err != nil {
		t.Fatal(err)
	}
	if copied.String() != "ok" {
		t.Fatalf("copied = %q", copied.String())
	}
}
