// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingBody) Close() error             { return nil }

type coverageTempFile struct {
	file     *os.File
	writeErr error
	syncErr  error
	closeErr error
	onClose  func()
}

func (f *coverageTempFile) Write(value []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.file.Write(value)
}
func (f *coverageTempFile) Name() string { return f.file.Name() }
func (f *coverageTempFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.file.Sync()
}
func (f *coverageTempFile) Close() error {
	err := f.file.Close()
	if f.onClose != nil {
		f.onClose()
		f.onClose = nil
	}
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func TestCrossPlatformCoverageDownloadURLAndPublicIPPolicy(t *testing.T) {
	valid := []string{
		"https://alidocs.dingtalk.com/file.docx",
		"https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md",
	}
	for _, raw := range valid {
		if _, err := ValidateDownloadURL(raw); err != nil {
			t.Errorf("ValidateDownloadURL(%q): %v", raw, err)
		}
	}
	invalid := []string{
		"http://alidocs.dingtalk.com/file.docx",
		"https://127.0.0.1/file.docx",
		"https://evil.example/file.docx",
		"https://oss-cn-hangzhou-internal.aliyuncs.com/file.docx",
		"https://user@alidocs.dingtalk.com/file.docx",
		"https://alidocs.dingtalk.com:8443/file.docx",
	}
	for _, raw := range invalid {
		if _, err := ValidateDownloadURL(raw); err == nil {
			t.Errorf("ValidateDownloadURL(%q) unexpectedly succeeded", raw)
		}
	}

	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "2001:db8::1"} {
		if publicIP(net.ParseIP(raw)) {
			t.Errorf("publicIP(%s) = true", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(raw)) {
			t.Errorf("publicIP(%s) = false", raw)
		}
	}
}

func TestCrossPlatformCoverageOutputPathPolicy(t *testing.T) {
	for _, output := range []string{"", "../escape", "nested/../../escape", "/tmp/absolute", `C:\\absolute\\file`} {
		if err := ValidateOutput(output); err == nil {
			t.Errorf("ValidateOutput(%q) unexpectedly succeeded", output)
		}
	}

	base := t.TempDir()
	destination, rel, err := ResolveOutputPath(base, "nested/file.md", "https://alidocs.dingtalk.com/file.md", "", false)
	if err != nil {
		t.Fatal(err)
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	if rel != filepath.Join("nested", "file.md") || filepath.Dir(destination) != filepath.Join(realBase, "nested") {
		t.Fatalf("destination=%q rel=%q", destination, rel)
	}
	if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveOutputPath(base, "nested/file.md", "https://alidocs.dingtalk.com/file.md", "", false); err == nil || !strings.Contains(err.Error(), "LOCAL_FILE_EXISTS") {
		t.Fatalf("no-clobber error = %v", err)
	}

	outside := t.TempDir()
	link := filepath.Join(base, "outside-link")
	if err := os.Symlink(outside, link); err == nil {
		if _, _, err := ResolveOutputPath(base, "outside-link/file", "https://alidocs.dingtalk.com/file", "", false); err == nil || !strings.Contains(err.Error(), "LOCAL_PATH_UNSAFE") {
			t.Fatalf("symlink escape error = %v", err)
		}
	}

	if got := SafeFilename("../evil", "https://alidocs.dingtalk.com/"); got != "evil" {
		t.Errorf("SafeFilename traversal basename = %q", got)
	}
	for _, name := range []string{"CON", "bad?.txt", " trailing.txt"} {
		if got := SafeFilename(name, "https://alidocs.dingtalk.com/"); got != "download" {
			t.Errorf("SafeFilename(%q) = %q", name, got)
		}
	}
}

func TestCrossPlatformCoverageDownloadAtomicNoClobberAndOverwrite(t *testing.T) {
	base := t.TempDir()
	payload := "first payload"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "alidocs.oss-cn-zhangjiakou.aliyuncs.com" {
			return nil, errors.New("unexpected host")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
	})}
	result, err := Download(context.Background(), "https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md", DownloadOptions{
		BaseDir: base, Output: "nested/result.md", Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RelativePath != "nested/result.md" || result.SizeBytes != int64(len(payload)) {
		t.Fatalf("result = %#v", result)
	}
	got, err := os.ReadFile(result.AbsolutePath)
	if err != nil || string(got) != payload {
		t.Fatalf("published content = %q, err=%v", got, err)
	}
	if _, err := Download(context.Background(), "https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md", DownloadOptions{
		BaseDir: base, Output: "nested/result.md", Client: client,
	}); err == nil || !strings.Contains(err.Error(), "LOCAL_FILE_EXISTS") {
		t.Fatalf("second download error = %v", err)
	}

	payload = "replacement"
	replaced, err := Download(context.Background(), "https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md", DownloadOptions{
		BaseDir: base, Output: "nested/result.md", Overwrite: true, Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(replaced.AbsolutePath)
	if err != nil || string(got) != payload {
		t.Fatalf("replacement content = %q, err=%v", got, err)
	}
}

func TestCrossPlatformCoverageDownloadFailureBoundaries(t *testing.T) {
	base := t.TempDir()
	validURL := "https://download.dingtalk.com/file.bin"
	if _, err := Download(context.Background(), "bad", DownloadOptions{BaseDir: base, Output: "x"}); err == nil {
		t.Fatal("invalid URL download succeeded")
	}
	if _, err := Download(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "../x"}); err == nil {
		t.Fatal("unsafe output download succeeded")
	}

	clientError := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("transport") })}
	if _, err := Download(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "transport.bin", Client: clientError}); err == nil {
		t.Fatal("transport error was ignored")
	}
	statusClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-test") != "ok" || req.Header.Get("") != "" {
			t.Errorf("headers = %#v", req.Header)
		}
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("backend")), Header: make(http.Header)}, nil
	})}
	if _, err := Download(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "status.bin", Client: statusClient, Headers: map[string]string{"x-test": "ok", " ": "ignored"}}); err == nil {
		t.Fatal("HTTP status error was ignored")
	}
	bodyErrorClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}, Header: make(http.Header)}, nil
	})}
	if _, err := Download(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "copy.bin", Client: bodyErrorClient}); err == nil {
		t.Fatal("body read error was ignored")
	}

	okClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload")), Header: make(http.Header)}, nil
	})}
	for _, tc := range []struct {
		name     string
		makeTemp func(string, string) (downloadTempFile, error)
	}{
		{"create", func(string, string) (downloadTempFile, error) { return nil, errors.New("create") }},
		{"sync", func(dir, pattern string) (downloadTempFile, error) {
			file, err := os.CreateTemp(dir, pattern)
			return &coverageTempFile{file: file, syncErr: errors.New("sync")}, err
		}},
		{"close", func(dir, pattern string) (downloadTempFile, error) {
			file, err := os.CreateTemp(dir, pattern)
			return &coverageTempFile{file: file, closeErr: errors.New("close")}, err
		}},
		{"publish-race", func(dir, pattern string) (downloadTempFile, error) {
			file, err := os.CreateTemp(dir, pattern)
			return &coverageTempFile{file: file, onClose: func() { _ = os.WriteFile(filepath.Join(base, "publish-race.bin"), []byte("race"), 0o600) }}, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &createDownloadTemp, tc.makeTemp)
			if _, err := Download(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: tc.name + ".bin", Client: okClient}); err == nil {
				t.Fatalf("%s failure was ignored", tc.name)
			}
		})
	}
}

func TestCrossPlatformCoverageSecureHTTPClientAndFilesystemEdges(t *testing.T) {
	client := secureHTTPClient()
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "https://download.dingtalk.com/x")}, make([]*http.Request, 5)); err == nil {
		t.Fatal("redirect limit accepted")
	}
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "https://evil.example/x")}, nil); err == nil {
		t.Fatal("unsafe redirect accepted")
	}
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "https://download.dingtalk.com/x")}, nil); err != nil {
		t.Fatal(err)
	}
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "bad-address"); err == nil {
		t.Fatal("bad address dial succeeded")
	}

	t.Run("lookup error", func(t *testing.T) {
		testseam.Swap(t, &lookupDownloadIPs, func(context.Context, string) ([]net.IPAddr, error) { return nil, errors.New("lookup") })
		if _, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err == nil {
			t.Fatal("lookup error ignored")
		}
	})
	t.Run("private answer", func(t *testing.T) {
		testseam.Swap(t, &lookupDownloadIPs, func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		})
		if _, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err == nil {
			t.Fatal("private DNS answer accepted")
		}
	})
	t.Run("public dial fallback and success", func(t *testing.T) {
		testseam.Swap(t, &lookupDownloadIPs, func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("1.1.1.1")}}, nil
		})
		left, right := net.Pipe()
		t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
		calls := 0
		testseam.Swap(t, &dialDownloadIP, func(context.Context, string, string) (net.Conn, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("first")
			}
			return left, nil
		})
		if conn, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err != nil {
			t.Fatal(err)
		} else {
			_ = conn.Close()
		}
	})
	t.Run("all public dials fail", func(t *testing.T) {
		testseam.Swap(t, &lookupDownloadIPs, func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		})
		testseam.Swap(t, &dialDownloadIP, func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("dial") })
		if _, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err == nil {
			t.Fatal("dial failure ignored")
		}
	})

	base := t.TempDir()
	if _, _, err := ResolveOutputPath("", "default-base.tmp", "https://download.dingtalk.com/x", "", false); err != nil {
		t.Fatal(err)
	} else {
		_ = os.Remove("default-base.tmp")
	}
	if _, _, err := ResolveOutputPath(filepath.Join(base, "missing"), "x", "https://download.dingtalk.com/x", "", false); err == nil {
		t.Fatal("missing base succeeded")
	}
	dir := filepath.Join(base, "directory")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{".", "directory/", "directory"} {
		if _, _, err := ResolveOutputPath(base, output, "https://download.dingtalk.com/path/name.txt", "preferred.txt", false); err != nil {
			t.Errorf("directory output %q: %v", output, err)
		}
	}
	if _, _, err := ResolveOutputPath(base, "directory", "https://download.dingtalk.com/x", "", false); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(base, "target-dir")
	_ = os.Mkdir(targetDir, 0o700)
	if _, _, err := ResolveOutputPath(base, "target-dir", "https://download.dingtalk.com/x", "x", true); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(base, "overwrite.txt")
	_ = os.WriteFile(targetFile, []byte("x"), 0o600)
	if _, _, err := ResolveOutputPath(base, "overwrite.txt", "https://download.dingtalk.com/x", "", true); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "target-link")
	if err := os.Symlink(targetFile, link); err == nil {
		if _, _, err := ResolveOutputPath(base, "target-link", "https://download.dingtalk.com/x", "", true); err == nil {
			t.Fatal("symlink destination accepted")
		}
	}
	fileParent := filepath.Join(base, "file-parent")
	_ = os.WriteFile(fileParent, []byte("x"), 0o600)
	if _, _, err := ResolveOutputPath(base, "file-parent/child", "https://download.dingtalk.com/x", "", false); err == nil {
		t.Fatal("file parent accepted")
	}
	if err := ensureSafeParent(base, filepath.Dir(base)); err == nil {
		t.Fatal("escaping parent accepted")
	}
	if err := ensureSafeParent(base, base); err != nil {
		t.Fatal(err)
	}

	source, err := os.CreateTemp(base, "source-*")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = source.WriteString("x")
	_ = source.Close()
	destination := filepath.Join(base, "publish.txt")
	_ = os.WriteFile(destination, []byte("old"), 0o600)
	if err := publishTempFile(source.Name(), destination, false); err == nil {
		t.Fatal("publish existing destination succeeded")
	}
	if err := publishTempFile(filepath.Join(base, "missing-source"), filepath.Join(base, "new.txt"), false); err == nil {
		t.Fatal("publish missing source succeeded")
	}
	symlinkDestination := filepath.Join(base, "publish-link")
	if err := os.Symlink(destination, symlinkDestination); err == nil {
		if err := publishTempFile(filepath.Join(base, "missing-source"), symlinkDestination, true); err == nil {
			t.Fatal("overwrite symlink succeeded")
		}
	}
	if err := publishTempFile(filepath.Join(base, "missing-source"), filepath.Join(base, "replace.txt"), true); err == nil {
		t.Fatal("replace missing source succeeded")
	}

	for _, name := range []string{"", ".", "..", "name.", "name ", "bad\x00", "AUX", "COM1", "LPT9"} {
		_ = sanitizeFilename(name)
	}
	_ = SafeFilename("", "https://download.dingtalk.com/path/fallback.txt")
	_ = SafeFilename("", "https://download.dingtalk.com/%zz")
	_ = SafeFilename("", "://bad")
	_ = publicIP(net.IP{1, 2, 3})
}

func TestCrossPlatformCoverageFilesystemInjectedFailures(t *testing.T) {
	base := t.TempDir()
	validURL := "https://download.dingtalk.com/x"
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Download(cancelled, validURL, DownloadOptions{BaseDir: base, Output: "default-client.bin"}); err == nil {
		t.Fatal("cancelled default client download succeeded")
	}

	t.Run("getwd", func(t *testing.T) {
		testseam.Swap(t, &localGetwd, func() (string, error) { return "", errors.New("getwd") })
		_, _, _ = ResolveOutputPath("", "x", validURL, "", false)
	})
	t.Run("abs", func(t *testing.T) {
		testseam.Swap(t, &localAbs, func(string) (string, error) { return "", errors.New("abs") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("eval base", func(t *testing.T) {
		testseam.Swap(t, &localEvalSymlinks, func(string) (string, error) { return "", errors.New("eval") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("eval parent", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &localEvalSymlinks, func(value string) (string, error) {
			calls++
			if calls == 2 {
				return "", errors.New("eval parent")
			}
			return value, nil
		})
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("parent rel", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &localRel, func(base, target string) (string, error) {
			calls++
			if calls == 2 {
				return "", errors.New("parent rel")
			}
			return filepath.Rel(base, target)
		})
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("destination directory", func(t *testing.T) {
		dirInfo, err := os.Stat(base)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &localLstat, func(string) (os.FileInfo, error) { return dirInfo, nil })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("destination lstat", func(t *testing.T) {
		testseam.Swap(t, &localLstat, func(string) (os.FileInfo, error) { return nil, errors.New("lstat") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("final rel", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &localRel, func(base, target string) (string, error) {
			calls++
			if calls == 3 {
				return "", errors.New("final rel")
			}
			return filepath.Rel(base, target)
		})
		_, _, _ = ResolveOutputPath(base, "x", validURL, "", false)
	})
	t.Run("mkdir", func(t *testing.T) {
		testseam.Swap(t, &localLstat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
		testseam.Swap(t, &localMkdir, func(string, os.FileMode) error { return errors.New("mkdir") })
		_ = ensureSafeParent(base, filepath.Join(base, "new"))
	})
	t.Run("lstat after mkdir", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &localLstat, func(string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, os.ErrNotExist
			}
			return nil, errors.New("after mkdir")
		})
		testseam.Swap(t, &localMkdir, func(string, os.FileMode) error { return nil })
		_ = ensureSafeParent(base, filepath.Join(base, "new"))
	})
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
