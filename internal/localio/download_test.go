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

func TestCrossPlatformCoverageDownloadURLPolicy(t *testing.T) {
	valid := []string{
		"https://alidocs.dingtalk.com/file.docx",
		"https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md",
		// 域名白名单、IP 直连拦截与拨号层公网 IP 校验均已移除：
		// 任意 HTTPS 主机（含 IP 字面量）在 URL 校验层放行，对齐
		// GUI 客户端（无客户端侧 SSRF 拦截）。
		"https://ddoss.ijingbo.chambroad.com/file.doc",
		// 专属部署存储域可服务在非默认端口；端口不是信任信号。
		"https://ddoss.ijingbo.chambroad.com:8443/file.doc",
		"https://evil.example/file.docx",
		"https://oss-cn-hangzhou-internal.aliyuncs.com/file.docx",
		"https://127.0.0.1/file.docx",
		"https://[::1]/file.docx",
	}
	for _, raw := range valid {
		if _, err := ValidateDownloadURL(raw); err != nil {
			t.Errorf("ValidateDownloadURL(%q): %v", raw, err)
		}
	}
	invalid := []string{
		"http://alidocs.dingtalk.com/file.docx",
		"https://user@alidocs.dingtalk.com/file.docx",
		"http://alidocs.dingtalk.com:8443/file.docx",
		"https://alidocs.dingtalk.com:NOTAPORT/file.docx",
	}
	for _, raw := range invalid {
		if _, err := ValidateDownloadURL(raw); err == nil {
			t.Errorf("ValidateDownloadURL(%q) unexpectedly succeeded", raw)
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
	destination, rel, err := ResolveOutputPath(base, "nested/file.md", "https://alidocs.dingtalk.com/file.md", "")
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
	if _, _, err := ResolveOutputPath(base, "nested/file.md", "https://alidocs.dingtalk.com/file.md", ""); err == nil || !strings.Contains(err.Error(), "LOCAL_FILE_EXISTS") {
		t.Fatalf("no-clobber error = %v", err)
	}

	outside := t.TempDir()
	link := filepath.Join(base, "outside-link")
	if err := os.Symlink(outside, link); err == nil {
		if _, _, err := ResolveOutputPath(base, "outside-link/file", "https://alidocs.dingtalk.com/file", ""); err == nil || !strings.Contains(err.Error(), "LOCAL_PATH_UNSAFE") {
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

func TestCrossPlatformCoverageDownloadAtomicNoClobber(t *testing.T) {
	base := t.TempDir()
	payload := "first payload"
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host != "alidocs.oss-cn-zhangjiakou.aliyuncs.com" {
			return nil, errors.New("unexpected host")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
	})}
	result, err := downloadWithClient(context.Background(), "https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md", DownloadOptions{
		BaseDir: base, Output: "nested/result.md",
	}, client)
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
	if _, err := downloadWithClient(context.Background(), "https://alidocs.oss-cn-zhangjiakou.aliyuncs.com/res/file.md", DownloadOptions{
		BaseDir: base, Output: "nested/result.md",
	}, client); err == nil || !strings.Contains(err.Error(), "LOCAL_FILE_EXISTS") {
		t.Fatalf("second download error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("existing destination performed %d network requests, want 1 total", requests)
	}

	got, err = os.ReadFile(result.AbsolutePath)
	if err != nil || string(got) != payload {
		t.Fatalf("no-clobber content = %q, err=%v", got, err)
	}
}

func TestCrossPlatformCoverageDownloadSizeLimitCleansPartialFiles(t *testing.T) {
	base := t.TempDir()
	for _, tc := range []struct {
		name          string
		contentLength int64
	}{
		{name: "declared", contentLength: 6},
		{name: "streamed", contentLength: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Body:          io.NopCloser(strings.NewReader("123456")),
					Header:        make(http.Header),
					ContentLength: tc.contentLength,
				}, nil
			})}
			output := tc.name + ".bin"
			if _, err := downloadWithClientLimit(context.Background(), "https://download.dingtalk.com/file.bin", DownloadOptions{
				BaseDir: base, Output: output,
			}, client, 5); err == nil || !strings.Contains(err.Error(), "LOCAL_DOWNLOAD_TOO_LARGE") {
				t.Fatalf("oversized download error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(base, output)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("oversized destination exists: %v", err)
			}
			entries, err := os.ReadDir(base)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".dws-download-") {
					t.Fatalf("oversized download left temp file %q", entry.Name())
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadRejectsParentReplacementDuringNetwork(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "nested")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(base, "original-parent")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if err := os.Rename(parent, original); err != nil {
			t.Skipf("platform cannot replace an open directory: %v", err)
		}
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload")), Header: make(http.Header)}, nil
	})}
	if _, err := downloadWithClient(context.Background(), "https://download.dingtalk.com/file.bin", DownloadOptions{
		BaseDir: base, Output: "nested/result.bin",
	}, client); err == nil || !strings.Contains(err.Error(), "LOCAL_PATH_CHANGED") {
		t.Fatalf("parent replacement error = %v", err)
	}
	for _, candidate := range []string{filepath.Join(parent, "result.bin"), filepath.Join(original, "result.bin")} {
		if _, err := os.Stat(candidate); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("parent replacement wrote %q: %v", candidate, err)
		}
	}
}

func TestCrossPlatformCoverageDownloadRejectsParentReplacementBeforePublish(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "nested")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(base, "original-parent")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload")), Header: make(http.Header)}, nil
	})}
	testseam.Swap(t, &createDownloadTemp, func(root *os.Root) (downloadTempFile, string, error) {
		created, name, err := createDownloadTempInRoot(root)
		if err != nil {
			return nil, "", err
		}
		return &coverageTempFile{file: created.(*os.File), onClose: func() {
			if renameErr := os.Rename(parent, original); renameErr != nil {
				t.Skipf("platform cannot replace an open directory: %v", renameErr)
			}
			if mkdirErr := os.Mkdir(parent, 0o700); mkdirErr != nil {
				t.Fatal(mkdirErr)
			}
		}}, name, nil
	})
	if _, err := downloadWithClient(context.Background(), "https://download.dingtalk.com/file.bin", DownloadOptions{
		BaseDir: base, Output: "nested/result.bin",
	}, client); err == nil || !strings.Contains(err.Error(), "LOCAL_PATH_CHANGED") {
		t.Fatalf("parent replacement error = %v", err)
	}
	for _, candidate := range []string{filepath.Join(parent, "result.bin"), filepath.Join(original, "result.bin")} {
		if _, err := os.Stat(candidate); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("parent replacement wrote %q: %v", candidate, err)
		}
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
	statusClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-test") != "ok" || req.Header.Get("") != "" {
			t.Errorf("headers = %#v", req.Header)
		}
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("backend")), Header: make(http.Header)}, nil
	})}
	bodyErrorClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}, Header: make(http.Header)}, nil
	})}
	if _, err := downloadWithClient(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "transport.bin"}, clientError); err == nil {
		t.Fatal("transport error was ignored")
	}
	if _, err := downloadWithClient(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "status.bin", Headers: map[string]string{"x-test": "ok", " ": "ignored"}}, statusClient); err == nil {
		t.Fatal("HTTP status error was ignored")
	}
	if _, err := downloadWithClient(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: "copy.bin"}, bodyErrorClient); err == nil {
		t.Fatal("body read error was ignored")
	}

	okClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload")), Header: make(http.Header)}, nil
	})}
	for _, tc := range []struct {
		name     string
		makeTemp func(*os.Root) (downloadTempFile, string, error)
	}{
		{"create", func(*os.Root) (downloadTempFile, string, error) { return nil, "", errors.New("create") }},
		{"sync", func(root *os.Root) (downloadTempFile, string, error) {
			created, name, err := createDownloadTempInRoot(root)
			if err != nil {
				return nil, "", err
			}
			return &coverageTempFile{file: created.(*os.File), syncErr: errors.New("sync")}, name, nil
		}},
		{"close", func(root *os.Root) (downloadTempFile, string, error) {
			created, name, err := createDownloadTempInRoot(root)
			if err != nil {
				return nil, "", err
			}
			return &coverageTempFile{file: created.(*os.File), closeErr: errors.New("close")}, name, nil
		}},
		{"publish-race", func(root *os.Root) (downloadTempFile, string, error) {
			created, name, err := createDownloadTempInRoot(root)
			if err != nil {
				return nil, "", err
			}
			return &coverageTempFile{file: created.(*os.File), onClose: func() {
				file, createErr := root.OpenFile("publish-race.bin", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if createErr == nil {
					_ = file.Close()
				}
			}}, name, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &createDownloadTemp, tc.makeTemp)
			if _, err := downloadWithClient(context.Background(), validURL, DownloadOptions{BaseDir: base, Output: tc.name + ".bin"}, okClient); err == nil {
				t.Fatalf("%s failure was ignored", tc.name)
			}
		})
	}
}

func TestCrossPlatformCoverageSecureHTTPClientAndFilesystemEdges(t *testing.T) {
	client := secureHTTPClient()
	transport := client.Transport.(*http.Transport)
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "https://download.dingtalk.com/x")}, make([]*http.Request, 5)); err == nil {
		t.Fatal("redirect limit accepted")
	}
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "http://evil.example/x")}, nil); err == nil {
		t.Fatal("unsafe redirect accepted")
	}
	if err := client.CheckRedirect(&http.Request{URL: mustURL(t, "https://download.dingtalk.com/x")}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.DialContext(context.Background(), "tcp", "bad-address"); err == nil {
		t.Fatal("bad address dial succeeded")
	}

	t.Run("dial seam failure propagates", func(t *testing.T) {
		testseam.Swap(t, &secureDialContext, func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("dial") })
		if _, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err == nil {
			t.Fatal("dial failure ignored")
		}
	})
	t.Run("dial seam success", func(t *testing.T) {
		left, right := net.Pipe()
		t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
		testseam.Swap(t, &secureDialContext, func(context.Context, string, string) (net.Conn, error) { return left, nil })
		if conn, err := transport.DialContext(context.Background(), "tcp", "download.dingtalk.com:443"); err != nil {
			t.Fatal(err)
		} else {
			_ = conn.Close()
		}
	})

	base := t.TempDir()
	if _, _, err := ResolveOutputPath("", "default-base.tmp", "https://download.dingtalk.com/x", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveOutputPath(filepath.Join(base, "missing"), "x", "https://download.dingtalk.com/x", ""); err == nil {
		t.Fatal("missing base succeeded")
	}
	dir := filepath.Join(base, "directory")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{".", "directory/", "directory"} {
		if _, _, err := ResolveOutputPath(base, output, "https://download.dingtalk.com/path/name.txt", "preferred.txt"); err != nil {
			t.Errorf("directory output %q: %v", output, err)
		}
	}
	if _, _, err := ResolveOutputPath(base, "directory", "https://download.dingtalk.com/x", ""); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(base, "target-dir")
	_ = os.Mkdir(targetDir, 0o700)
	if _, _, err := ResolveOutputPath(base, "target-dir", "https://download.dingtalk.com/x", "x"); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(base, "existing.txt")
	_ = os.WriteFile(targetFile, []byte("x"), 0o600)
	if _, _, err := ResolveOutputPath(base, "existing.txt", "https://download.dingtalk.com/x", ""); err == nil || !strings.Contains(err.Error(), "LOCAL_FILE_EXISTS") {
		t.Fatalf("existing destination error = %v", err)
	}
	link := filepath.Join(base, "target-link")
	if err := os.Symlink(targetFile, link); err == nil {
		if _, _, err := ResolveOutputPath(base, "target-link", "https://download.dingtalk.com/x", ""); err == nil {
			t.Fatal("symlink destination accepted")
		}
	}
	fileParent := filepath.Join(base, "file-parent")
	_ = os.WriteFile(fileParent, []byte("x"), 0o600)
	if _, _, err := ResolveOutputPath(base, "file-parent/child", "https://download.dingtalk.com/x", ""); err == nil {
		t.Fatal("file parent accepted")
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := ensureSafeParent(root, "../escape"); err == nil {
		t.Fatal("escaping parent accepted")
	}
	if err := ensureSafeParent(root, "."); err != nil {
		t.Fatal(err)
	}

	source, err := root.OpenFile("source.tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = source.WriteString("x")
	_ = source.Close()
	destination := filepath.Join(base, "publish.txt")
	_ = os.WriteFile(destination, []byte("old"), 0o600)
	if err := publishTempFile(root, "source.tmp", "publish.txt"); err == nil {
		t.Fatal("publish existing destination succeeded")
	}
	if err := publishTempFile(root, "missing-source", "new.txt"); err == nil {
		t.Fatal("publish missing source succeeded")
	}
	symlinkDestination := filepath.Join(base, "publish-link")
	if err := os.Symlink(destination, symlinkDestination); err == nil {
		if err := publishTempFile(root, "source.tmp", "publish-link"); err == nil {
			t.Fatal("publish to symlink succeeded")
		}
	}
	closedRoot, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	_ = closedRoot.Close()
	if _, _, err := createDownloadTempInRoot(closedRoot); err == nil {
		t.Fatal("temp creation in closed root succeeded")
	}

	for _, name := range []string{"", ".", "..", "name.", "name ", "bad\x00", "AUX", "COM1", "LPT9"} {
		_ = sanitizeFilename(name)
	}
	_ = SafeFilename("", "https://download.dingtalk.com/path/fallback.txt")
	_ = SafeFilename("", "https://download.dingtalk.com/%zz")
	_ = SafeFilename("", "://bad")
}

func TestCrossPlatformCoverageSecureHTTPClientDisablesEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	transport := SecureHTTPClient().Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Fatal("secure download client accepted an environment proxy")
	}
}

func TestCrossPlatformCoverageSetSecureDownloadDialTargetForTest(t *testing.T) {
	prevDial := secureDialContext
	t.Cleanup(func() { secureDialContext = prevDial })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
		close(accepted)
	}()

	SetSecureDownloadDialTargetForTest(listener.Addr().String())
	conn, err := secureDialContext(context.Background(), "tcp", "download.dingtalk.com:443")
	if err != nil {
		t.Fatalf("overridden dial did not reach the fixture listener: %v", err)
	}
	_ = conn.Close()
	<-accepted
}

func TestCrossPlatformCoverageSecureHTTPClientStripsCrossOriginHeaders(t *testing.T) {
	client := secureHTTPClient()
	original := &http.Request{
		URL: mustURL(t, "https://download.dingtalk.com/source"),
		Header: http.Header{
			"X-Oss-Security-Token": []string{"credential-a"},
			"X-Download-Auth":      []string{"credential-b"},
		},
	}

	sameOrigin := &http.Request{
		URL:    mustURL(t, "https://DOWNLOAD.dingtalk.com.:443/next"),
		Header: original.Header.Clone(),
	}
	if err := client.CheckRedirect(sameOrigin, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	if sameOrigin.Header.Get("X-Oss-Security-Token") == "" {
		t.Fatal("same-origin redirect unexpectedly stripped request headers")
	}

	sameHostNonDefaultPort := &http.Request{
		URL:    mustURL(t, "https://download.dingtalk.com:8443/next"),
		Header: original.Header.Clone(),
	}
	if err := client.CheckRedirect(sameHostNonDefaultPort, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	if len(sameHostNonDefaultPort.Header) != 0 {
		t.Fatal("port change is a cross-origin redirect; headers must be stripped")
	}

	crossOrigin := &http.Request{
		URL:    mustURL(t, "https://attacker-bucket.oss-cn-hangzhou.aliyuncs.com/next"),
		Header: original.Header.Clone(),
	}
	if err := client.CheckRedirect(crossOrigin, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	if len(crossOrigin.Header) != 0 {
		t.Fatalf("cross-origin redirect retained %d request headers", len(crossOrigin.Header))
	}

	multiHop := &http.Request{
		URL:    mustURL(t, "https://attacker-bucket.oss-cn-hangzhou.aliyuncs.com/final"),
		Header: original.Header.Clone(),
	}
	if err := client.CheckRedirect(multiHop, []*http.Request{original, crossOrigin}); err != nil {
		t.Fatal(err)
	}
	if len(multiHop.Header) != 0 {
		t.Fatalf("later cross-origin redirect restored %d initial request headers", len(multiHop.Header))
	}
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
		_, _, _ = ResolveOutputPath("", "x", validURL, "")
	})
	t.Run("abs", func(t *testing.T) {
		testseam.Swap(t, &localAbs, func(string) (string, error) { return "", errors.New("abs") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "")
	})
	t.Run("eval base", func(t *testing.T) {
		testseam.Swap(t, &localEvalSymlinks, func(string) (string, error) { return "", errors.New("eval") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "")
	})
	t.Run("open base", func(t *testing.T) {
		testseam.Swap(t, &openDownloadRoot, func(string) (*os.Root, error) { return nil, errors.New("open root") })
		_, _, _ = ResolveOutputPath(base, "x", validURL, "")
	})
	t.Run("mkdir", func(t *testing.T) {
		testseam.Swap(t, &downloadRootMkdir, func(*os.Root, string, os.FileMode) error { return errors.New("mkdir") })
		_, _, _ = ResolveOutputPath(base, "new/target", validURL, "")
	})
	t.Run("lstat after mkdir", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &downloadRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "new-after" {
				calls++
				if calls > 1 {
					return nil, errors.New("after mkdir")
				}
			}
			return root.Lstat(name)
		})
		_, _, _ = ResolveOutputPath(base, "new-after/target", validURL, "")
	})
	t.Run("open parent", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "open-parent"), 0o700); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &openDownloadParent, func(*os.Root, string) (*os.Root, error) { return nil, errors.New("open parent") })
		_, _, _ = ResolveOutputPath(base, "open-parent/target", validURL, "")
	})
	t.Run("parent stat", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "parent-stat"), 0o700); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &downloadRootStat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "." {
				return nil, errors.New("parent stat")
			}
			return root.Stat(name)
		})
		_, _, _ = ResolveOutputPath(base, "parent-stat/target", validURL, "")
	})
	t.Run("parent identity", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "parent-identity"), 0o700); err != nil {
			t.Fatal(err)
		}
		otherInfo, err := os.Stat(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &downloadRootStat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "parent-identity" {
				return otherInfo, nil
			}
			return root.Stat(name)
		})
		_, _, _ = ResolveOutputPath(base, "parent-identity/target", validURL, "")
	})
	t.Run("destination directory", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "destination-directory"), 0o700); err != nil {
			t.Fatal(err)
		}
		dirInfo, err := os.Stat(base)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &downloadRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "target" {
				return dirInfo, nil
			}
			return root.Lstat(name)
		})
		_, _, _ = ResolveOutputPath(base, "destination-directory/target", validURL, "")
	})
	t.Run("destination symlink", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "destination-symlink"), 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(base, "coverage-link")
		if err := os.Symlink(filepath.Join(base, "destination-symlink"), link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		linkInfo, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &downloadRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "target" {
				return linkInfo, nil
			}
			return root.Lstat(name)
		})
		_, _, _ = ResolveOutputPath(base, "destination-symlink/target", validURL, "")
	})
	t.Run("destination lstat", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "destination-lstat"), 0o700); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &downloadRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "target" {
				return nil, errors.New("destination lstat")
			}
			return root.Lstat(name)
		})
		_, _, _ = ResolveOutputPath(base, "destination-lstat/target", validURL, "")
	})
	t.Run("unsafe parent type", func(t *testing.T) {
		filePath := filepath.Join(base, "unsafe-parent")
		if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(base)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := ensureSafeParent(root, "unsafe-parent"); err == nil {
			t.Fatal("regular file accepted as output parent")
		}
	})
	t.Run("publish remove", func(t *testing.T) {
		root, err := os.OpenRoot(base)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		file, err := root.OpenFile("remove-source", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		testseam.Swap(t, &downloadRootRemove, func(*os.Root, string) error { return errors.New("remove") })
		if err := publishTempFile(root, "remove-source", "remove-destination"); err == nil {
			t.Fatal("publish remove error ignored")
		}
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
