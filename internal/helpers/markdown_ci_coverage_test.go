// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type markdownCIFailingReader struct{}

func (markdownCIFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func startMarkdownCITempFileRemover(t *testing.T, root, dirPrefix, fileName string) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			entries, err := os.ReadDir(root)
			if err != nil {
				result <- err
				return
			}
			for _, entry := range entries {
				if !entry.IsDir() || !strings.HasPrefix(entry.Name(), dirPrefix) {
					continue
				}
				target := filepath.Join(root, entry.Name(), fileName)
				if err := os.Remove(target); err == nil {
					result <- nil
					return
				} else if !os.IsNotExist(err) {
					result <- err
					return
				}
			}
			runtime.Gosched()
		}
		result <- fmt.Errorf("timed out waiting for %s/%s", dirPrefix, fileName)
	}()
	return result
}

func waitMarkdownCIRemover(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for temporary-file remover")
	}
}

func TestMarkdownCICoverageFetchAndOutputPaths(t *testing.T) {
	t.Run("missing node reaches runtime validation", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		if err := runMarkdownFetch(newMarkdownFetchCmd(), nil); err == nil || !strings.Contains(err.Error(), "--node") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("output resolution failure propagates", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "body")
		parentFile := writeMarkdownDriveFixture(t, "parent", "not a directory")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--space-id", "space-1",
			"--output", filepath.Join(parentFile, "child.md"))
		if err == nil || !strings.Contains(err.Error(), "检查输出路径") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("write failure propagates", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "body")
		output := filepath.Join(t.TempDir(), "missing", "child.md")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--space-id", "space-1", "--output", output)
		if err == nil || !strings.Contains(err.Error(), "保存到") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("raw saved output reports destination", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		_, stderr := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "body")
		output := filepath.Join(t.TempDir(), "saved.md")
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--space-id", "space-1", "--output", output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr.String(), "已保存到 "+output) {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("safe path fallbacks and lstat failure", func(t *testing.T) {
		dir := t.TempDir()
		got, err := resolveMarkdownOutputPath(dir, ".")
		if err != nil || got != filepath.Join(dir, "download.md") {
			t.Fatalf("path = %q, error = %v", got, err)
		}
		tooLong := strings.Repeat("x", 300) + ".md"
		if _, err := resolveMarkdownOutputPath(dir, tooLong); err == nil || !strings.Contains(err.Error(), "检查输出文件") {
			t.Fatalf("error = %v", err)
		}
		if got := resolveDownloadFilename(`{`, "https://download.test/fallback.md?token=redacted"); got != "fallback.md" {
			t.Fatalf("fallback name = %q", got)
		}
	})

	t.Run("directory traversal defense rejects unsanitized names", func(t *testing.T) {
		if _, err := resolveMarkdownDirectoryOutputPath(t.TempDir(), "../escape.md"); err == nil ||
			!strings.Contains(err.Error(), "越过输出目录") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMarkdownCICoverageCreateEdges(t *testing.T) {
	t.Run("directory source", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--file", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "是目录而非文件") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("content source failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--name", "a.md", "--content", "@"+filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "从文件") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary directory failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--name", "a.md", "--content", "body")
		if err == nil || !strings.Contains(err.Error(), "创建临时目录失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file write failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		name := strings.Repeat("x", 300) + ".md"
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--name", name, "--content", "body")
		if err == nil || !strings.Contains(err.Error(), "写入临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file disappears before stat", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		tempRoot := t.TempDir()
		t.Setenv("TMPDIR", tempRoot)
		removed := startMarkdownCITempFileRemover(t, tempRoot, "dws-markdown-create-", "a.md")
		content := strings.Repeat("x", 16<<20)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--name", "a.md", "--content", content)
		waitMarkdownCIRemover(t, removed)
		if err == nil || !strings.Contains(err.Error(), "读取上传文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("caller dry run after stat", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "a.md", "body")
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--file", path, "--space-id", "space-1"); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["operation"] != "create" {
			t.Fatalf("payload = %#v, error = %v, output = %q", payload, err, stdout.String())
		}
	})

	t.Run("stdin read failure", func(t *testing.T) {
		cmd := &cobra.Command{Use: "source"}
		cmd.SetIn(markdownCIFailingReader{})
		if _, err := resolveMarkdownContentSource(cmd, "-"); err == nil || !strings.Contains(err.Error(), "stdin") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMarkdownCICoverageOverwriteEdges(t *testing.T) {
	t.Run("content source failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", "a.md", "--content", "@"+filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "从文件") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("content mode remote name failure", func(t *testing.T) {
		boom := errors.New("metadata failed")
		caller := &markdownDriveCaller{format: "json", steps: []markdownDriveStep{{err: boom}}}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1", "--content", "body")
		if err == nil || !strings.Contains(err.Error(), "自动获取文件名失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary directory failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", "a.md", "--content", "body")
		if err == nil || !strings.Contains(err.Error(), "创建临时目录失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file write failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		name := strings.Repeat("x", 300) + ".md"
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", name, "--content", "body")
		if err == nil || !strings.Contains(err.Error(), "写入临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file disappears before stat", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		tempRoot := t.TempDir()
		t.Setenv("TMPDIR", tempRoot)
		removed := startMarkdownCITempFileRemover(t, tempRoot, "dws-markdown-overwrite-", "a.md")
		content := strings.Repeat("x", 16<<20)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", "a.md", "--content", content)
		waitMarkdownCIRemover(t, removed)
		if err == nil || !strings.Contains(err.Error(), "读取上传文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("file mode remote name failure", func(t *testing.T) {
		boom := errors.New("metadata failed")
		caller := &markdownDriveCaller{format: "json", steps: []markdownDriveStep{{err: boom}}}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "source.md", "body")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1", "--file", path)
		if err == nil || !strings.Contains(err.Error(), "自动获取文件名失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("local preview read failure", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "json"})
		path := writeMarkdownDriveFixture(t, "source.md", "body")
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", "source.md", "--file", path, "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "读取新内容失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("confirmation cancellation", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{format: "raw"})
		path := writeMarkdownDriveFixture(t, "source.md", "body")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), strings.NewReader("no\n"),
			"markdown", "overwrite", "--node", "node-1", "--space-id", "space-1",
			"--name", "source.md", "--file", path)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMarkdownCICoveragePatchEdges(t *testing.T) {
	t.Run("global caller dry run", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--pattern", "old", "--content", "new"); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["operation"] != "patch" {
			t.Fatalf("payload = %#v, error = %v", payload, err)
		}
	})

	t.Run("raw zero match", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "alpha beta")
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "missing", "--content", "new", "--yes"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "未找到匹配内容") || !strings.Contains(stdout.String(), "匹配数") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("empty replacement cannot clear the whole file", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "entire file")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "entire file", "--content", "", "--yes")
		if err == nil || !strings.Contains(err.Error(), "替换后内容为空") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("empty-result guard made write calls: %#v", caller.calls)
		}
	})

	t.Run("remote name failure", func(t *testing.T) {
		boom := errors.New("metadata failed")
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{err: boom},
			},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", "new", "--yes")
		if err == nil || !strings.Contains(err.Error(), "自动获取文件名失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary directory failure", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{text: `{"fileName":"current.md"}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		missingTemp := filepath.Join(t.TempDir(), "missing")
		previousTemp, hadPreviousTemp := os.LookupEnv("TMPDIR")
		t.Cleanup(func() {
			if hadPreviousTemp {
				_ = os.Setenv("TMPDIR", previousTemp)
			} else {
				_ = os.Unsetenv("TMPDIR")
			}
		})
		httpGetFile = func(_ context.Context, _ string, _ map[string]string, destPath string) error {
			if err := os.WriteFile(destPath, []byte("old value"), 0o600); err != nil {
				return err
			}
			return os.Setenv("TMPDIR", missingTemp)
		}
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", "new", "--yes")
		if err == nil || !strings.Contains(err.Error(), "创建临时目录失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file write failure", func(t *testing.T) {
		longName := strings.Repeat("x", 300) + ".md"
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{text: fmt.Sprintf(`{"fileName":%q}`, longName)},
			},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", "new", "--yes")
		if err == nil || !strings.Contains(err.Error(), "写入临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temporary file disappears before stat", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{text: `{"fileName":"current.md"}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old")
		tempRoot := t.TempDir()
		t.Setenv("TMPDIR", tempRoot)
		removed := startMarkdownCITempFileRemover(t, tempRoot, "dws-markdown-patch-", "current.md")
		replacement := strings.Repeat("x", 16<<20)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", replacement, "--yes")
		waitMarkdownCIRemover(t, removed)
		if err == nil || !strings.Contains(err.Error(), "读取临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("drive upload error propagates", func(t *testing.T) {
		boom := errors.New("upload info failed")
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{text: `{"fileName":"current.md"}`},
				{err: boom},
			},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", "new", "--yes")
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("raw drive success reports result", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
				{text: `{"fileName":"current.md"}`},
				{text: `{"uploadId":"upload-1","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"updated":true}`},
			},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--space-id", "space-1",
			"--pattern", "old", "--content", "new", "--yes"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "内容已更新") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})
}

func TestMarkdownCICoverageGlobalDryRunAndNames(t *testing.T) {
	t.Run("caller enables helper dry run", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{dryRun: true})
		if !markdownGlobalDryRun(&cobra.Command{Use: "standalone"}) {
			t.Fatal("caller dry run was not observed")
		}
	})

	t.Run("nil command", func(t *testing.T) {
		previous := deps
		deps = nil
		t.Cleanup(func() { deps = previous })
		if markdownGlobalDryRun(nil) {
			t.Fatal("nil command unexpectedly enabled dry run")
		}
	})

	t.Run("root without flag", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{})
		if markdownGlobalDryRun(&cobra.Command{Use: "standalone"}) {
			t.Fatal("standalone command unexpectedly enabled dry run")
		}
	})

	t.Run("short command path", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{})
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", false, "")
		if markdownGlobalDryRun(root) {
			t.Fatal("root command unexpectedly enabled dry run")
		}
	})

	t.Run("subcommand without preceding global flag", func(t *testing.T) {
		installMarkdownDriveDeps(t, &markdownDriveCaller{})
		previousArgs := os.Args
		os.Args = []string{"dws", "markdown", "patch"}
		t.Cleanup(func() { os.Args = previousArgs })
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", false, "")
		markdown := &cobra.Command{Use: "markdown"}
		patch := &cobra.Command{Use: "patch"}
		root.AddCommand(markdown)
		markdown.AddCommand(patch)
		if markdownGlobalDryRun(patch) {
			t.Fatal("local command unexpectedly enabled global dry run")
		}
	})

	for _, test := range []struct {
		name string
		step markdownDriveStep
		want string
	}{
		{name: "metadata error", step: markdownDriveStep{err: errors.New("boom")}, want: "自动获取文件名失败"},
		{name: "empty name", step: markdownDriveStep{text: `{}`}, want: "无法自动获取原文件名"},
		{name: "wrong extension", step: markdownDriveStep{text: `{"fileName":"notes.txt"}`}, want: "不是 .md 文件"},
	} {
		t.Run(test.name, func(t *testing.T) {
			installMarkdownDriveDeps(t, &markdownDriveCaller{steps: []markdownDriveStep{test.step}})
			if _, err := markdownRemoteNameWithContext(context.Background(), "node-1", false); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMarkdownCICoverageDiffOutputModes(t *testing.T) {
	t.Run("raw overwrite preview", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "before")
		if err := previewMarkdownOverwriteDiff(context.Background(), "node-1", "space-1", false, "after"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "markdown overwrite") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("JSON patch preview", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		if err := printMarkdownPatchDiff("node-1", "before", "after", 1); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["operation"] != "patch" {
			t.Fatalf("payload = %#v, error = %v", payload, err)
		}
	})
}

var _ io.Reader = markdownCIFailingReader{}
