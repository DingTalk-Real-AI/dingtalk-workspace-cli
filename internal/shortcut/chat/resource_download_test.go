// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chat

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageCanonicalMessageResourceType(t *testing.T) {
	for input, want := range map[string]string{
		"mediaId":  "mediaId",
		"MEDIAID":  "mediaId",
		" fileid ": "fileId",
		"FileId":   "fileId",
	} {
		got, ok := canonicalMessageResourceType(input)
		if !ok || got != want {
			t.Errorf("canonicalMessageResourceType(%q) = %q, %v; want %q, true",
				input, got, ok, want)
		}
	}
	if got, ok := canonicalMessageResourceType("attachment"); ok || got != "attachment" {
		t.Errorf("unsupported resource type = %q, %v", got, ok)
	}
	if _, err := resolveMessageResourceDownloadData(
		nil, "attachment", "resource", "message", "conversation",
	); err == nil || !strings.Contains(err.Error(), "不支持的消息资源类型") {
		t.Fatalf("unsupported resolver type error = %v", err)
	}
}

func TestCrossPlatformCoverageResourceDownloadInfo(t *testing.T) {
	url, headers, err := resourceDownloadInfo(map[string]any{
		"result": map[string]any{
			"resourceUrl": []any{"https://download.dingtalk.com/path/image.png"},
			"headers": map[string]any{
				"X-Test": "value",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://download.dingtalk.com/path/image.png" {
		t.Fatalf("url = %q", url)
	}
	if headers["X-Test"] != "value" {
		t.Fatalf("headers = %#v", headers)
	}
	upgraded, _, err := resourceDownloadInfo(map[string]any{
		"result": map[string]any{
			"downloadUrl": "http://bucket.oss-cn-hangzhou.aliyuncs.com/file?signature=x",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if upgraded != "https://bucket.oss-cn-hangzhou.aliyuncs.com/file?signature=x" {
		t.Fatalf("upgraded URL = %q", upgraded)
	}
	if _, _, err := resourceDownloadInfo(map[string]any{
		"result": map[string]any{"resourceUrl": "http://download.example.test/a"},
	}); err == nil {
		t.Fatal("plain HTTP URL unexpectedly accepted")
	}
	for _, resourceURL := range []string{
		"https://user:secret@download.dingtalk.com/file",
		"http://download.dingtalk.com:8443/file",
	} {
		if _, _, err := resourceDownloadInfo(
			map[string]any{"resourceUrl": resourceURL},
		); err == nil {
			t.Fatalf("untrusted URL %q unexpectedly accepted", resourceURL)
		}
	}
	// The static host allowlist and the IP-literal refusal are both retired:
	// dedicated-deployment download hosts and IP literals pass the same
	// host-agnostic HTTPS policy as DingTalk/OSS hosts, mirroring the GUI
	// client which applies no client-side SSRF interception.
	// Non-default HTTPS ports are accepted too: dedicated storage domains
	// legitimately serve on them.
	for _, resourceURL := range []string{
		"https://download.dingtalk.com/file",
		"https://ddoss.tenant.example.com/file",
		"https://ddoss.tenant.example.com:8443/file",
		"https://127.0.0.1/file",
	} {
		if _, _, err := resourceDownloadInfo(
			map[string]any{"resourceUrl": resourceURL},
		); err != nil {
			t.Fatalf("HTTPS domain %q unexpectedly rejected: %v", resourceURL, err)
		}
	}
}

func TestCrossPlatformCoverageResolveResourceDownloadPath(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	absolute, relative, err := resolveResourceDownloadPath(
		base,
		"downloads/",
		"https://download.example.test/path/photo.png?token=redacted",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(absolute) != "photo.png" || filepath.ToSlash(relative) != "downloads/photo.png" {
		t.Fatalf("path = %q / %q", absolute, relative)
	}

	for _, unsafe := range []string{"../escape", filepath.Join("a", "..", "..", "escape")} {
		if _, _, err := resolveResourceDownloadPath(
			base, unsafe, "https://download.example.test/file", false,
		); err == nil {
			t.Fatalf("unsafe output %q unexpectedly accepted", unsafe)
		}
	}
}

func TestCrossPlatformCoverageValidateResourceDownloadOutputUsesOwningFlagName(t *testing.T) {
	err := validateResourceDownloadOutputFlag("../escape", "--output-dir")
	if err == nil || !strings.Contains(err.Error(), "--output-dir") {
		t.Fatalf("error = %v, want --output-dir", err)
	}
}

func TestCrossPlatformCoverageValidateResourceDownloadOutputRejectsPortableAbsolutePaths(t *testing.T) {
	for _, output := range []string{
		"/absolute", `\\absolute`, `C:\\absolute`, "C:/absolute",
		"C:relative", "c:relative",
	} {
		if err := validateResourceDownloadOutput(output); err == nil {
			t.Errorf("portable absolute output %q unexpectedly accepted", output)
		}
	}
	for _, output := range []string{"../escape", `..\\escape`} {
		if err := validateResourceDownloadOutput(output); err == nil {
			t.Errorf("portable parent escape %q unexpectedly accepted", output)
		}
	}
}

func TestCrossPlatformCoverageResolveResourceDownloadPathRejectsSymlinkParentBeforeCreatingOutside(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(base, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, _, err := resolveResourceDownloadPath(
		base,
		filepath.Join("linked", "new-child", "resource.bin"),
		"https://download.example.test/resource.bin",
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("symlink-parent error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "new-child")); !os.IsNotExist(statErr) {
		t.Fatalf("outside directory was created before rejection: %v", statErr)
	}
}

func TestCrossPlatformCoverageDownloadResourceAtomically(t *testing.T) {
	const body = "verified download bytes"
	client := &http.Client{Transport: resourceRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://download.dingtalk.com/resource.bin" {
			t.Fatalf("request URL = %q", request.URL)
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
			Header:        make(http.Header),
		}, nil
	})}
	dest := filepath.Join(t.TempDir(), "resource.bin")

	size, err := downloadResourceAtomically(
		context.Background(), client, "https://download.dingtalk.com/resource.bin", nil, dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", size, len(body))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("body = %q", got)
	}
	if _, err := downloadResourceAtomically(
		context.Background(), client, "https://download.dingtalk.com/resource.bin", nil, dest, false,
	); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("no-clobber error = %v", err)
	}
}
