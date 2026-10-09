// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type npmPackageTestEntry struct {
	name string
	data string
	kind byte
	size int64
	pax  map[string]string
}

func npmPackageTestEntries(goos, goarch string) []npmPackageTestEntry {
	binaryName := npmPackageBinaryName(goos, goarch)
	return []npmPackageTestEntry{
		{name: "package/", kind: tar.TypeDir},
		{name: "package/package.json", data: `{"name":"dingtalk-workspace-cli","version":"1.2.3"}`},
		{name: "package/assets/" + binaryName, data: "binary archive fixture"},
		{name: "package/assets/dws-skills.zip", data: "skills archive fixture"},
		{name: "package/assets/checksums.txt", data: fmt.Sprintf("%x  %s\n%x  dws-skills.zip\n", sha256.Sum256([]byte("binary archive fixture")), binaryName, sha256.Sum256([]byte("skills archive fixture")))},
		{name: "package/install.js", data: "throw new Error('must never execute')"},
		{name: "package/assets/other-platform.tar.gz", data: "ignored"},
	}
}

func npmPackageTestTar(t *testing.T, entries []npmPackageTestEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.data))
		if entry.size != 0 {
			size = entry.size
		}
		if err := w.WriteHeader(&tar.Header{Name: entry.name, Mode: 0600, Typeflag: kind, Size: size, Linkname: "outside", PAXRecords: entry.pax}); err != nil {
			t.Fatal(err)
		}
		if entry.size != 0 {
			// 只保留超限或截断成员的 header，不分配其声明的巨大内容。
			return b.Bytes()
		}
		if _, err := io.WriteString(w, entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func npmPackageTestGzip(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func npmPackageTestIdentity() NPMPackage {
	return NPMPackage{Name: "dingtalk-workspace-cli", Version: "1.2.3"}
}

func npmPackageTestSRI(data []byte) string {
	digest := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(digest[:])
}

func TestCrossPlatformCoverageNPMPackagePrepareWithoutNode(t *testing.T) {
	t.Setenv("PATH", "")
	entries := npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)
	body := npmPackageTestGzip(t, npmPackageTestTar(t, entries))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	pkg := npmPackageTestIdentity()
	pkg.TarballURL = server.URL
	pkg.Integrity = npmPackageTestSRI(body)
	for _, skip := range []bool{false, true} {
		dest := t.TempDir()
		progress := 0
		got, err := PrepareNPMPackage(t.Context(), pkg, dest, skip, func(percent float64, downloaded, total int64) {
			progress++
			if percent != 100 || downloaded != int64(len(body)) || total != downloaded {
				t.Fatalf("unexpected progress: %v %v %v", percent, downloaded, total)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		binary, err := os.ReadFile(got.BinaryArchivePath)
		if err != nil || string(binary) != entries[2].data || got.ChecksumsContent != entries[4].data || progress == 0 {
			t.Fatalf("prepared=%+v err=%v", got, err)
		}
		if (got.SkillsArchivePath == "") != skip {
			t.Fatalf("skills path=%q skip=%v", got.SkillsArchivePath, skip)
		}
		children, err := os.ReadDir(filepath.Dir(got.BinaryArchivePath))
		if err != nil || len(children) != 3-boolToIntNPMPackage(skip) {
			t.Fatalf("unexpected extracted files: %v %v", children, err)
		}
	}
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

func boolToIntNPMPackage(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestCrossPlatformCoverageNPMPackagePrepareFailures(t *testing.T) {
	good := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
	for _, tc := range []struct {
		name     string
		edit     func(*NPMPackage)
		body     []byte
		code     int
		failOpen bool
	}{
		{name: "missing integrity", edit: func(p *NPMPackage) { p.Integrity = "" }},
		{name: "unsafe url", edit: func(p *NPMPackage) { p.TarballURL = "http://example.com/pkg" }},
		{name: "http error", code: 404},
		{name: "integrity mismatch", edit: func(p *NPMPackage) { p.Integrity = npmPackageTestSRI([]byte("other")) }},
		{name: "invalid archive", body: []byte("not gzip")},
		{name: "open failure", failOpen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if body == nil {
				body = good
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.code != 0 {
					w.WriteHeader(tc.code)
					return
				}
				w.Header().Set("Content-Length", fmt.Sprint(len(body)))
				_, _ = w.Write(body)
			}))
			defer server.Close()
			pkg := npmPackageTestIdentity()
			pkg.TarballURL, pkg.Integrity = server.URL, npmPackageTestSRI(body)
			if tc.edit != nil {
				tc.edit(&pkg)
			}
			dir := t.TempDir()
			if tc.failOpen {
				testseam.Swap(t, &npmPackageOpen, func(string) (*os.File, error) {
					return nil, os.ErrPermission
				})
			}
			if _, err := PrepareNPMPackage(t.Context(), pkg, dir, false, nil); err == nil {
				t.Fatal("expected error")
			}
			children, _ := os.ReadDir(dir)
			if len(children) != 0 {
				t.Fatalf("failed prepare leaked files: %v", children)
			}
		})
	}
	pkg := npmPackageTestIdentity()
	pkg.TarballURL, pkg.Integrity = "https://example.com/pkg", npmPackageTestSRI(good)
	if _, err := PrepareNPMPackage(t.Context(), pkg, filepath.Join(t.TempDir(), "absent"), false, nil); err == nil {
		t.Fatal("missing dest accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PrepareNPMPackage(ctx, pkg, t.TempDir(), false, nil); err == nil {
		t.Fatal("canceled download accepted")
	}
}

func TestCrossPlatformCoverageNPMPackageIntegrity(t *testing.T) {
	data := []byte("test package")
	sha := sha256.Sum256(data)
	weak := "sha256-" + base64.StdEncoding.EncodeToString(sha[:])
	strong := npmPackageTestSRI(data)
	for _, value := range []string{weak, strong, weak + " " + strong, strong + " " + weak, npmPackageTestSRI([]byte("different")) + " " + strong} {
		i, err := parseNPMPackageIntegrity(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := i.verify(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "sha1-abcd", "sha512-?", "sha256-YQ==", weak + " " + npmPackageTestSRI([]byte("different"))} {
		i, err := parseNPMPackageIntegrity(value)
		if err == nil {
			err = i.verify(bytes.NewReader(data))
		}
		if err == nil {
			t.Fatalf("invalid SRI accepted: %q", value)
		}
	}
	i, _ := parseNPMPackageIntegrity(strong)
	if err := i.verify(npmPackageErrorReader{}); err == nil {
		t.Fatal("reader error ignored")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (npmPackageContextReader{ctx, bytes.NewReader(data)}).Read(make([]byte, 1)); err != context.Canceled {
		t.Fatal(err)
	}
}

type npmPackageErrorReader struct{}

func (npmPackageErrorReader) Read([]byte) (int, error) { return 0, fmt.Errorf("injected read failure") }

func TestCrossPlatformCoverageNPMPackageArchiveFailures(t *testing.T) {
	base := npmPackageTestEntries("linux", "amd64")
	for _, tc := range []struct {
		name string
		edit func([]npmPackageTestEntry) []npmPackageTestEntry
	}{
		{"traversal", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: "package/../outside"})
		}},
		{"absolute", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: "/outside"})
		}},
		{"backslash", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: `package\outside`})
		}},
		{"symlink", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: "package/link", kind: tar.TypeSymlink})
		}},
		{"hardlink", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: "package/link", kind: tar.TypeLink})
		}},
		{"duplicate", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es, es[2]) }},
		{"duplicate ignored member", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es, es[5]) }},
		{"entry too big", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			return append(es, npmPackageTestEntry{name: "package/huge", size: npmPackageMaxEntry + 1})
		}},
		{"metadata too big", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			es[1].size = npmPackageMaxMetadata + 1
			return es[:2]
		}},
		{"metadata truncated", func(es []npmPackageTestEntry) []npmPackageTestEntry { es[1].size = 100; return es[:2] }},
		{"asset truncated", func(es []npmPackageTestEntry) []npmPackageTestEntry { es[2].size = 100; return es[:3] }},
		{"missing manifest", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es[:1], es[2:]...) }},
		{"bad manifest", func(es []npmPackageTestEntry) []npmPackageTestEntry { es[1].data = "{"; return es }},
		{"wrong name", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			es[1].data = `{"name":"other","version":"1.2.3"}`
			return es
		}},
		{"wrong version", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			es[1].data = `{"name":"dingtalk-workspace-cli","version":"1.2.4"}`
			return es
		}},
		{"missing checksum", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es[:4], es[5:]...) }},
		{"missing binary", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es[:2], es[3:]...) }},
		{"missing skills", func(es []npmPackageTestEntry) []npmPackageTestEntry { return append(es[:3], es[4:]...) }},
		{"tampered binary", func(es []npmPackageTestEntry) []npmPackageTestEntry { es[2].data += "tampered"; return es }},
		{"tampered skills", func(es []npmPackageTestEntry) []npmPackageTestEntry { es[3].data += "tampered"; return es }},
		{"too many members", func(es []npmPackageTestEntry) []npmPackageTestEntry {
			for i := 0; i < npmPackageMaxEntries; i++ {
				es = append(es, npmPackageTestEntry{name: fmt.Sprintf("package/extra-%d", i)})
			}
			return es
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := tc.edit(append([]npmPackageTestEntry{}, base...))
			body := npmPackageTestGzip(t, npmPackageTestTar(t, es))
			if _, err := extractNPMPackage(t.Context(), bytes.NewReader(body), npmPackageTestIdentity(), t.TempDir(), false, "linux", "amd64"); err == nil {
				t.Fatal("unsafe package accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageNPMPackageArchiveBoundsAndTrailer(t *testing.T) {
	validTar := npmPackageTestTar(t, npmPackageTestEntries("windows", "arm64"))
	good := npmPackageTestGzip(t, validTar)
	if got, err := extractNPMPackage(t.Context(), bytes.NewReader(good), npmPackageTestIdentity(), t.TempDir(), false, "windows", "arm64"); err != nil || !strings.HasSuffix(got.BinaryArchivePath, ".zip") {
		t.Fatalf("windows asset: %+v %v", got, err)
	}
	for _, data := range [][]byte{
		good[:len(good)-4],
		npmPackageTestGzip(t, []byte("bad tar")),
		npmPackageTestGzip(t, append(append([]byte{}, validTar...), []byte("extra archive")...)),
	} {
		if _, err := extractNPMPackage(t.Context(), bytes.NewReader(data), npmPackageTestIdentity(), t.TempDir(), false, "windows", "arm64"); err == nil {
			t.Fatal("truncated or appended archive accepted")
		}
	}
	var bomb bytes.Buffer
	_, _ = bomb.Write(good)
	// 复用小 gzip 成员构造真实的 512 MiB 多成员压缩炸弹，避免测试反复压缩大块数据。
	chunk := npmPackageTestGzip(t, make([]byte, 1<<20))
	for i := 0; i < npmPackageMaxExpanded/(1<<20); i++ {
		_, _ = bomb.Write(chunk)
	}
	if _, err := extractNPMPackage(t.Context(), bytes.NewReader(bomb.Bytes()), npmPackageTestIdentity(), t.TempDir(), false, "windows", "arm64"); err == nil || !strings.Contains(err.Error(), "展开大小") {
		t.Fatalf("expanded bound: %v", err)
	}
	if n, err := (npmPackagePaddingWriter{}).Write([]byte{0, 0}); n != 2 || err != nil {
		t.Fatalf("padding: %d %v", n, err)
	}
}

func TestCrossPlatformCoverageNPMPackageSparseAndCRC(t *testing.T) {
	entries := npmPackageTestEntries("linux", "amd64")
	entries[2].pax = map[string]string{"ABC.sparse.major": "2"}
	tarBytes := npmPackageTestTar(t, entries)
	// tar.Writer 不写 GNU sparse 字段；等长替换 PAX 内容构造真实输入，header 校验和不变。
	malicious := bytes.ReplaceAll(tarBytes, []byte("ABC.sparse.major"), []byte("GNU.sparse.major"))
	for _, raw := range [][]byte{tarBytes, malicious} {
		_, err := extractNPMPackage(t.Context(), bytes.NewReader(npmPackageTestGzip(t, raw)), npmPackageTestIdentity(), t.TempDir(), false, "linux", "amd64")
		if (err != nil) != bytes.Equal(raw, malicious) {
			t.Fatalf("sparse rejection: %v", err)
		}
	}
	body := npmPackageTestGzip(t, tarBytes)
	body[len(body)-8] ^= 1
	if _, err := extractNPMPackage(t.Context(), bytes.NewReader(body), npmPackageTestIdentity(), t.TempDir(), false, "linux", "amd64"); err == nil {
		t.Fatal("invalid gzip CRC accepted")
	}
}

func TestCrossPlatformCoverageNPMPackageChecksumsAndFileErrors(t *testing.T) {
	valid := fmt.Sprintf("%x  archive.tar.gz", sha256.Sum256([]byte("test")))
	for _, value := range []string{"", "bad", "short  name", strings.Repeat("g", 64) + "  name", valid + "\n" + valid, strings.Repeat("a", 64) + "  ../outside", strings.Repeat("a", 64) + `  a\b`} {
		if _, err := strictNPMPackageChecksums(value); err == nil {
			t.Fatalf("invalid checksums accepted: %q", value)
		}
	}
	if got, err := strictNPMPackageChecksums("\n" + strings.ToUpper(valid[:64]) + valid[64:] + "\n"); err != nil || got["archive.tar.gz"] != valid[:64] {
		t.Fatalf("checksum normalization: %v %v", got, err)
	}
	if _, err := writeNPMPackageMember(t.TempDir(), strings.NewReader("test")); err == nil {
		t.Fatal("file write over directory accepted")
	}
	for _, w := range []*npmPackageTestCloser{{writeErr: true}, {closeErr: true}} {
		if _, err := copyNPMPackageMember(w, strings.NewReader("test")); err == nil {
			t.Fatal("writer failure ignored")
		}
		if !w.closed {
			t.Fatal("writer not closed")
		}
	}
	if _, err := copyNPMPackageMember(&npmPackageTestCloser{}, npmPackageErrorReader{}); err == nil {
		t.Fatal("reader failure ignored")
	}
}

type npmPackageTestCloser struct{ writeErr, closeErr, closed bool }

func (w *npmPackageTestCloser) Write(p []byte) (int, error) {
	if w.writeErr {
		return 0, fmt.Errorf("write failure")
	}
	return len(p), nil
}
func (w *npmPackageTestCloser) Close() error {
	w.closed = true
	if w.closeErr {
		return fmt.Errorf("close failure")
	}
	return nil
}

func TestCrossPlatformCoverageNPMPackageRedirectAndDownloadLimit(t *testing.T) {
	request := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: u}
	}
	for _, tc := range []struct {
		target string
		via    []*http.Request
		fail   bool
	}{
		{"https://registry.npmjs.org/pkg", nil, false},
		{"http://127.0.0.1/pkg", nil, false},
		{"http://127.0.0.1/pkg", []*http.Request{request("https://registry.npmjs.org/pkg")}, true},
		{"http://external.example/pkg", nil, true},
		{"https://registry.npmjs.org/pkg", make([]*http.Request, 10), true},
	} {
		if err := npmPackageRedirect(request(tc.target), tc.via); (err != nil) != tc.fail {
			t.Fatalf("redirect %q: %v", tc.target, err)
		}
	}
	for _, unknown := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unknown {
				w.(http.Flusher).Flush()
			} else {
				w.Header().Set("Content-Length", "9")
			}
			_, _ = io.WriteString(w, "123456789")
		}))
		cfg := DefaultDownloadConfig()
		cfg.MaxBytes = 4
		cfg.MaxRetries = 1
		if _, err := DownloadWithConfig(t.Context(), server.URL, filepath.Join(t.TempDir(), "download"), cfg); err == nil || !strings.Contains(err.Error(), "大小上限") {
			t.Fatalf("download bound: %v", err)
		}
		server.Close()
	}
}
