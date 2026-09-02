// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// The html fetch/overwrite/patch leaves run through the shared textfile engine.
// These tests pin the html-specific deltas: the .html/.htm extension contract,
// the text/html MIME on Drive uploads, the HTML operation prose and diff echo
// lines, the download.html fallback name, and the html leaf declaration
// surface. Engine-wide branches that markdown already covers through the same
// shared functions stay covered there.

func TestCrossPlatformCoverageHTMLFetchAutoRoutesAndKeepsJSONPure(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{err: errors.New("not found in drive")},
			{text: `{"name":"doc","extension":"html"}`},
			{text: `{"resourceUrl":"https://download.test/internal.file","fileName":"../../evil.html"}`},
		},
	}
	stdout, stderr := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "external html")
	outputDir := t.TempDir()
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "fetch", "--node", "node-1", "--output", outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 ||
		caller.calls[0].server != "drive" ||
		caller.calls[1].server != "doc" ||
		caller.calls[2].server != "doc" {
		t.Fatalf("auto-route calls = %#v", caller.calls)
	}
	savedPath := filepath.Join(outputDir, "evil.html")
	if data, err := os.ReadFile(savedPath); err != nil || string(data) != "external html" {
		t.Fatalf("safe output file: data=%q err=%v", data, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("fetch stdout is not pure JSON: %v\n%s", err, stdout.String())
	}
	if payload["content"] != "external html" || payload["saved_to"] != savedPath || payload["source"] != "doc" {
		t.Fatalf("fetch payload = %#v", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON fetch unexpectedly wrote warnings: %q", stderr.String())
	}
}

func TestCrossPlatformCoverageHTMLFetchDryRunAndRawOutput(t *testing.T) {
	t.Run("dry run avoids network", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "fetch", "--node", "node-1", "--output", "out.html")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dry-run fetch calls = %#v", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["operation"] != "fetch" {
			t.Fatalf("dry-run payload: err=%v output=%q", err, stdout.String())
		}
	})

	t.Run("human dry run names the HTML operation", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "raw", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "fetch", "--node", "node-1", "--space-id", "space-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dry-run fetch calls = %#v", caller.calls)
		}
		if text := stdout.String(); !strings.Contains(text, "获取 HTML 内容") {
			t.Fatalf("human dry-run output = %q", text)
		}
	})

	t.Run("raw output keeps warning on stderr", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		stdout, stderr := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "untrusted html")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "fetch", "--node", "node-1", "--space-id", "space-1")
		if err != nil {
			t.Fatal(err)
		}
		if stdout.String() != "untrusted html\n" {
			t.Fatalf("raw stdout = %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "不可信数据") {
			t.Fatalf("missing out-of-band warning: %q", stderr.String())
		}
	})

	t.Run("route flags remain exclusive", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "fetch", "--node", "node-1", "--space-id", "space-1", "--workspace", "workspace-1")
		if err == nil || !strings.Contains(err.Error(), "互斥") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing node reaches runtime validation", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil, "html", "fetch")
		if err == nil || !strings.Contains(err.Error(), "--node") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageHTMLFetchOutputPathFallback(t *testing.T) {
	got, err := resolveTextOutputPath(t.TempDir(), ".", htmlTextFileSpec)
	if err != nil || got == "" {
		t.Fatalf("path = %q, error = %v", got, err)
	}
	if filepath.Base(got) != "download.html" {
		t.Fatalf("fallback name = %q, want download.html", filepath.Base(got))
	}

	names := map[string]bool{
		"index.html": true,
		"INDEX.HTML": true,
		"page.htm":   true,
		"PAGE.Htm":   true,
		"page.md":    false,
		"page.txt":   false,
		"page":       false,
	}
	for name, want := range names {
		if got := htmlTextFileSpec.hasExtension(name); got != want {
			t.Errorf("hasExtension(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCrossPlatformCoverageHTMLFetchRejectsSymlinkOutput(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "page.html")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if _, err := resolveTextOutputPath(link, "remote.html", htmlTextFileSpec); err == nil ||
		!strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("resolveTextOutputPath error = %v, want symlink rejection", err)
	}

	caller := &markdownDriveCaller{
		format: "json",
		steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
	}
	installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "untrusted html")
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "fetch", "--node", "file-1", "--space-id", "space-1", "--output", link)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("fetch through symlink output error = %v, want symlink rejection", err)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != "original" {
		t.Fatalf("symlink target was overwritten: data=%q err=%v", data, readErr)
	}
}

func TestCrossPlatformCoverageHTMLOverwriteWrites(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"uploadId":"upload-1","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
			{text: `{"updated":true}`},
		},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	var uploaded string
	httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, _ int64) error {
		data, err := os.ReadFile(path)
		uploaded = string(data)
		return err
	}
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "overwrite", "--node", "file-1", "--content", "<h1>changed</h1>",
		"--name", "index.html", "--space-id", "space-1", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if uploaded != "<h1>changed</h1>" {
		t.Fatalf("uploaded content = %q", uploaded)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	for _, call := range caller.calls {
		if call.server != "drive" || call.args["overwriteFileId"] != "file-1" {
			t.Fatalf("overwrite call = %#v", call)
		}
	}
	if caller.calls[0].args["mimeType"] != "text/html" || caller.calls[0].args["fileName"] != "index.html" {
		t.Fatalf("drive overwrite upload args = %#v", caller.calls[0].args)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["updated"] != true {
		t.Fatalf("overwrite output is not pure server JSON: err=%v output=%q", err, stdout.String())
	}
}

func TestCrossPlatformCoverageHTMLOverwriteValidation(t *testing.T) {
	t.Run("file mode rejects non-html file", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "source.md", "# not html")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--space-id", "space-1", "--file", path, "--yes")
		if err == nil || !strings.Contains(err.Error(), "必须以 .html/.htm 结尾") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("validation failure made calls: %#v", caller.calls)
		}
	})

	t.Run("file mode rejects missing file", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--space-id", "space-1",
			"--file", filepath.Join(t.TempDir(), "missing.html"), "--yes")
		if err == nil || !strings.Contains(err.Error(), "无法读取文件") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("validation failure made calls: %#v", caller.calls)
		}
	})

	t.Run("content mode rejects non-html name", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--space-id", "space-1",
			"--content", "body", "--name", "a.md", "--yes")
		if err == nil || !strings.Contains(err.Error(), "--name 必须以 .html/.htm 结尾") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("validation failure made calls: %#v", caller.calls)
		}
	})

	t.Run("remote name must carry the html extension", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"fileName":"current.md"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--space-id", "space-1",
			"--content", "body", "--yes")
		if err == nil || !strings.Contains(err.Error(), "远程文件不是 .html/.htm 文件，当前文件名: current.md") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "get_file_info" {
			t.Fatalf("remote-name probe calls = %#v", caller.calls)
		}
	})

	t.Run("missing node reaches runtime validation", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--content", "body", "--name", "index.html")
		if err == nil || !strings.Contains(err.Error(), "--node") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageHTMLOverwritePreviews(t *testing.T) {
	t.Run("local dry run downloads and renders json diff", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old body")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--content", "<h1>new</h1>",
			"--name", "index.html", "--space-id", "space-1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "download_file" {
			t.Fatalf("local preview calls = %#v", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("local preview is not JSON: %v\n%s", err, stdout.String())
		}
		if payload["before"] != "old body" || payload["after"] != "<h1>new</h1>" ||
			payload["operation"] != "overwrite" || payload["dry_run"] != true || payload["executed"] != false {
			t.Fatalf("diff payload = %#v", payload)
		}
	})

	t.Run("local dry run renders human diff with html echo line", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old body")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "overwrite", "--node", "file-1", "--content", "<h1>new</h1>",
			"--name", "index.html", "--space-id", "space-1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		text := stdout.String()
		if !strings.Contains(text, "[dry-run] dws html overwrite --node file-1") ||
			!strings.Contains(text, "No write performed") {
			t.Fatalf("human diff = %q", text)
		}
	})

	t.Run("global dry run stays offline", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "raw"}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownGlobalDryRun(t, newHTMLCommand(),
			"html", "overwrite", "--node", "node-1", "--content", "new", "--name", "index.html")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("global preview made calls: %#v", caller.calls)
		}
		if text := stdout.String(); !strings.Contains(text, "覆盖更新 HTML 文件") {
			t.Fatalf("global preview output = %q", text)
		}
	})
}

func TestCrossPlatformCoverageHTMLOverwriteConfirmationGate(t *testing.T) {
	caller := &markdownDriveCaller{format: "json"}
	installMarkdownDriveDeps(t, caller)
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("declined overwrite attempted upload")
		return nil
	}
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), strings.NewReader("no\n"),
		"html", "overwrite", "--node", "file-1", "--space-id", "space-1",
		"--content", "body", "--name", "index.html")
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("error = %v, want 用户取消了操作", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("cancelled overwrite calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageHTMLPatchWrites(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"resourceUrl":"https://download.test/current.html","fileName":"internal.file"}`},
			{text: `{"name":"remote","extension":"html"}`},
			{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"key-1"}`},
			{text: `{"patched":true}`},
		},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "v1 v2")
	var uploaded string
	httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, _ int64) error {
		data, err := os.ReadFile(path)
		uploaded = string(data)
		return err
	}
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "patch", "--node", "node-1", "--pattern", `v\d`, "--content", "$1",
		"--regex", "--workspace", "workspace-1", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if uploaded != "$1 $1" {
		t.Fatalf("regex replacement expanded capture syntax: %q", uploaded)
	}
	if len(caller.calls) != 4 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[0].tool != "download_file" ||
		caller.calls[1].tool != "get_document_info" ||
		caller.calls[2].tool != "get_file_upload_info" ||
		caller.calls[3].tool != "commit_uploaded_file" {
		t.Fatalf("patch sequence = %#v", caller.calls)
	}
	if caller.calls[2].args["overwriteNodeId"] != "node-1" ||
		caller.calls[2].args["name"] != "remote.html" {
		t.Fatalf("patch upload args = %#v", caller.calls[2].args)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["patched"] != true {
		t.Fatalf("patch output is not pure server JSON: err=%v output=%q", err, stdout.String())
	}
}

func TestCrossPlatformCoverageHTMLPatchZeroMatchNeverUploads(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "alpha beta")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("zero-match patch attempted an upload")
		return nil
	}
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "patch", "--node", "file-1", "--pattern", "missing", "--content", "new",
		"--space-id", "space-1", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "download_file" {
		t.Fatalf("zero-match calls = %#v", caller.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("zero-match output is not JSON: %v\n%s", err, stdout.String())
	}
	if payload["changed"] != false || payload["match_count"] != float64(0) {
		t.Fatalf("zero-match payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageHTMLPatchEdges(t *testing.T) {
	t.Run("empty replacement aborts before upload", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old")
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
			t.Fatal("empty-replacement patch attempted an upload")
			return nil
		}
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--pattern", "old", "--content", "",
			"--space-id", "space-1", "--yes")
		if err == nil || !strings.Contains(err.Error(), "替换后内容为空，已中止操作") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid regex reports compile failure", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "body")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--pattern", "[", "--content", "new",
			"--regex", "--space-id", "space-1", "--yes")
		if err == nil || !strings.Contains(err.Error(), "正则表达式编译失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("required flags fail before routing", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--content", "new", "--space-id", "space-1")
		if err == nil || !strings.Contains(err.Error(), "--node、--pattern 与 --content 均为必填") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("validation failure made calls: %#v", caller.calls)
		}
	})

	t.Run("remote name must carry the html extension", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`},
				{text: `{"fileName":"current.md"}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old text")
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
			t.Fatal("non-html remote-name patch attempted an upload")
			return nil
		}
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--pattern", "old", "--content", "new",
			"--space-id", "space-1", "--yes")
		if err == nil || !strings.Contains(err.Error(), "远程文件不是 .html/.htm 文件，当前文件名: current.md") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 2 ||
			caller.calls[0].tool != "download_file" || caller.calls[1].tool != "get_file_info" {
			t.Fatalf("remote-name probe calls = %#v", caller.calls)
		}
	})

	t.Run("local dry run renders patch diff", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--pattern", "old", "--content", "new",
			"--space-id", "space-1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		text := stdout.String()
		if !strings.Contains(text, "[dry-run] dws html patch --node file-1") ||
			!strings.Contains(text, "匹配数: 1") || !strings.Contains(text, "No write performed") {
			t.Fatalf("human patch diff = %q", text)
		}
	})

	t.Run("local dry run keeps json diff payload", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "patch", "--node", "file-1", "--pattern", "old", "--content", "new",
			"--space-id", "space-1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("patch dry-run output is not JSON: %v\n%s", err, stdout.String())
		}
		if payload["before"] != "old value" || payload["after"] != "new value" ||
			payload["match_count"] != float64(1) || payload["operation"] != "patch" ||
			payload["dry_run"] != true || payload["executed"] != false {
			t.Fatalf("patch dry-run payload = %#v", payload)
		}
	})
}

func TestCrossPlatformCoverageHTMLPatchCancellationStopsBeforeUploadMetadata(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.html","fileName":"current.html"}`}},
	}
	installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "old value")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("cancelled patch attempted upload")
		return nil
	}
	err := executeMarkdownDriveCommand(t, newHTMLCommand(), strings.NewReader("no\n"),
		"html", "patch", "--node", "file-1", "--pattern", "old", "--content", "new",
		"--space-id", "space-1")
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("error = %v, want 用户取消了操作", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("cancelled patch calls = %#v, want none before ConfirmSafety decline", caller.calls)
	}
}

func TestCrossPlatformCoverageHTMLFileOpsPublishTypedConstraints(t *testing.T) {
	findLeaf := func(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
		t.Helper()
		leaf, remaining, err := root.Find(path)
		if err != nil || leaf == nil || len(remaining) != 0 {
			t.Fatalf("find %v: leaf=%v remaining=%v err=%v", path, leaf, remaining, err)
		}
		return leaf
	}
	hasGroup := func(groups [][]string, names ...string) bool {
		want := append([]string(nil), names...)
		for _, group := range groups {
			matched := true
			if len(group) != len(want) {
				continue
			}
			for i := range want {
				if group[i] != want[i] {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
		return false
	}
	constraints := func(t *testing.T, command *cobra.Command) cli.RuntimeSchemaConstraints {
		t.Helper()
		var parsed cli.RuntimeSchemaConstraints
		raw := ""
		if command.Annotations != nil {
			raw = command.Annotations["dws.schema.constraints"]
		}
		if raw == "" {
			t.Fatalf("%s has no typed constraints annotation", command.CommandPath())
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Fatalf("%s constraints: %v", command.CommandPath(), err)
		}
		return parsed
	}

	html := newHTMLCommand()
	fetch := constraints(t, findLeaf(t, html, "fetch"))
	if !hasGroup(fetch.MutuallyExclusive, "space-id", "workspace") {
		t.Fatalf("html fetch constraints = %#v", fetch)
	}
	overwrite := constraints(t, findLeaf(t, html, "overwrite"))
	if !hasGroup(overwrite.MutuallyExclusive, "content", "file") ||
		!hasGroup(overwrite.MutuallyExclusive, "space-id", "workspace") ||
		!hasGroup(overwrite.RequireOneOf, "content", "file") {
		t.Fatalf("html overwrite constraints = %#v", overwrite)
	}
	patch := constraints(t, findLeaf(t, html, "patch"))
	if !hasGroup(patch.MutuallyExclusive, "space-id", "workspace") {
		t.Fatalf("html patch constraints = %#v", patch)
	}
}
