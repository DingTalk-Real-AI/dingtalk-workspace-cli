// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type markdownDriveCall struct {
	server string
	tool   string
	args   map[string]any
}

type markdownDriveStep struct {
	text string
	err  error
}

type markdownDriveCaller struct {
	steps  []markdownDriveStep
	calls  []markdownDriveCall
	format string
	dryRun bool
}

func (c *markdownDriveCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, markdownDriveCall{server: server, tool: tool, args: copied})
	index := len(c.calls) - 1
	if index >= len(c.steps) {
		return &edition.ToolResult{}, nil
	}
	step := c.steps[index]
	if step.err != nil {
		return nil, step.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: step.text}}}, nil
}

func (c *markdownDriveCaller) Format() string { return c.format }
func (c *markdownDriveCaller) DryRun() bool   { return c.dryRun }
func (*markdownDriveCaller) Fields() string   { return "" }
func (*markdownDriveCaller) JQ() string       { return "" }

func installMarkdownDriveDeps(t *testing.T, caller *markdownDriveCaller) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	previousDeps := deps
	previousPut := httpPutFile
	previousGet := httpGetFile
	InitDeps(caller)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps.Out = &Formatter{w: stdout, errW: stderr}
	t.Cleanup(func() {
		deps = previousDeps
		httpPutFile = previousPut
		httpGetFile = previousGet
	})
	return stdout, stderr
}

func executeMarkdownDriveCommand(t *testing.T, product *cobra.Command, input io.Reader, args ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "preview only")
	root.PersistentFlags().BoolP("yes", "y", false, "skip confirmation")
	root.AddCommand(product)
	root.SetArgs(args)
	if input == nil {
		input = strings.NewReader("")
	}
	root.SetIn(input)
	root.SetOut(io.Discard)
	if deps != nil && deps.Out != nil {
		root.SetErr(deps.Out.errW)
	} else {
		root.SetErr(io.Discard)
	}
	return root.Execute()
}

func executeMarkdownGlobalDryRun(t *testing.T, product *cobra.Command, args ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "preview only")
	root.PersistentFlags().BoolP("yes", "y", false, "skip confirmation")
	if err := root.PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	root.AddCommand(product)
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	if deps != nil && deps.Out != nil {
		root.SetErr(deps.Out.errW)
	}
	return root.Execute()
}

func writeMarkdownDriveFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func installMarkdownHTTPGet(t *testing.T, content string) {
	t.Helper()
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destPath string) error {
		return os.WriteFile(destPath, []byte(content), 0o600)
	}
}

func TestDriveUploadOverwriteRoutesAndConfirms(t *testing.T) {
	t.Run("node and folder are mutually exclusive", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		err := executeMarkdownDriveCommand(t, newDriveCommand(), nil,
			"drive", "upload", "--file", path, "--node", "file-1", "--folder", "folder-1", "--yes")
		if err == nil || !strings.Contains(err.Error(), "--node 与 --folder 互斥") {
			t.Fatalf("expected mutual-exclusion error, got %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("mutual-exclusion failure made MCP calls: %#v", caller.calls)
		}
	})

	t.Run("space and workspace are mutually exclusive", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		err := executeMarkdownDriveCommand(t, newDriveCommand(), nil,
			"drive", "upload", "--file", path, "--space-id", "space-1", "--workspace", "workspace-1")
		if err == nil || !strings.Contains(err.Error(), "--space-id 与 --workspace 互斥") {
			t.Fatalf("expected target-domain conflict, got %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("target-domain conflict made MCP calls: %#v", caller.calls)
		}
	})

	t.Run("overwrite dry run is a no-network JSON plan", func(t *testing.T) {
		for _, route := range []struct {
			name string
			args []string
		}{
			{name: "drive", args: []string{"--space-id", "space-1"}},
			{name: "doc", args: []string{"--workspace", "workspace-1"}},
		} {
			t.Run(route.name, func(t *testing.T) {
				caller := &markdownDriveCaller{format: "json", dryRun: true}
				stdout, _ := installMarkdownDriveDeps(t, caller)
				path := writeMarkdownDriveFixture(t, "payload.md", "body")
				args := []string{"drive", "upload", "--file", path, "--node", "file-1"}
				args = append(args, route.args...)
				if err := executeMarkdownDriveCommand(t, newDriveCommand(), nil, args...); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 0 {
					t.Fatalf("dry run made MCP calls: %#v", caller.calls)
				}
				var payload map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
					t.Fatalf("dry-run output is not JSON: %q: %v", stdout.String(), err)
				}
				if payload["dry_run"] != true || payload["executed"] != false ||
					payload["preview_kind"] != "plan" || payload["node_id"] != "file-1" ||
					payload["source"] != route.name {
					t.Fatalf("dry-run payload = %#v", payload)
				}
			})
		}
	})

	t.Run("overwrite dry run is human readable", func(t *testing.T) {
		for _, route := range []struct {
			name       string
			routeArgs  []string
			wantTarget string
		}{
			{name: "drive", routeArgs: []string{"--space-id", "space-1"}, wantTarget: "覆盖目标"},
			{name: "doc", routeArgs: []string{"--workspace", "workspace-1"}, wantTarget: "覆盖目标"},
		} {
			t.Run(route.name, func(t *testing.T) {
				caller := &markdownDriveCaller{format: "raw", dryRun: true}
				stdout, _ := installMarkdownDriveDeps(t, caller)
				path := writeMarkdownDriveFixture(t, "payload.md", "body")
				args := []string{"drive", "upload", "--file", path, "--node", "file-1"}
				args = append(args, route.routeArgs...)
				if err := executeMarkdownDriveCommand(t, newDriveCommand(), nil, args...); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 0 {
					t.Fatalf("human dry run made MCP calls: %#v", caller.calls)
				}
				if text := stdout.String(); !strings.Contains(text, route.wantTarget) || !strings.Contains(text, "file-1") {
					t.Fatalf("human dry-run output = %q", text)
				}
			})
		}
	})

	t.Run("drive overwrite uses explicit route and overwrite id", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"uploadId":"upload-1","resourceUrls":[{"url":"https://upload.test/drive","headers":{"x-test":"yes"}}]}`},
				{text: `{"ok":true}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		var uploaded string
		httpPutFile = func(_ context.Context, resourceURL string, headers map[string]string, filePath string, size int64) error {
			data, err := os.ReadFile(filePath)
			uploaded = string(data)
			if err != nil {
				return err
			}
			if resourceURL != "https://upload.test/drive" || headers["x-test"] != "yes" || size != 4 {
				t.Fatalf("unexpected PUT request: url=%q headers=%v size=%d", resourceURL, headers, size)
			}
			return nil
		}
		err := executeMarkdownDriveCommand(t, newDriveCommand(), nil,
			"drive", "upload", "--file", path, "--node", "file-1", "--space-id", "space-1",
			"--mime-type", "text/markdown", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if uploaded != "body" {
			t.Fatalf("uploaded content = %q", uploaded)
		}
		if len(caller.calls) != 2 {
			t.Fatalf("calls = %#v", caller.calls)
		}
		for _, call := range caller.calls {
			if call.server != "drive" {
				t.Fatalf("server = %q, want drive", call.server)
			}
			if call.args["overwriteFileId"] != "file-1" {
				t.Fatalf("%s overwriteFileId = %#v", call.tool, call.args["overwriteFileId"])
			}
			if _, exists := call.args["parentId"]; exists {
				t.Fatalf("%s unexpectedly received parentId: %#v", call.tool, call.args)
			}
		}
		if caller.calls[0].tool != "get_upload_info" || caller.calls[1].tool != "commit_upload" {
			t.Fatalf("unexpected upload sequence: %#v", caller.calls)
		}
	})

	t.Run("document overwrite uses standard dangerous confirmation", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"key-1"}`},
				{text: `{"ok":true}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		err := executeMarkdownDriveCommand(t, newDriveCommand(), nil,
			"drive", "upload", "--file", path, "--file-name", "renamed", "--node", "node-1",
			"--workspace", "workspace-1", "--convert", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 {
			t.Fatalf("calls = %#v", caller.calls)
		}
		wantStep1 := map[string]any{"workspaceId": "workspace-1", "overwriteNodeId": "node-1", "name": "renamed.md"}
		if !reflect.DeepEqual(caller.calls[0].args, wantStep1) {
			t.Fatalf("step1 args = %#v, want %#v", caller.calls[0].args, wantStep1)
		}
		if caller.calls[1].args["overwriteNodeId"] != "node-1" ||
			caller.calls[1].args["convertToOnlineDoc"] != true {
			t.Fatalf("commit args = %#v", caller.calls[1].args)
		}
	})

	t.Run("negative confirmation prevents all writes", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		_, stderr := installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		err := executeMarkdownDriveCommand(t, newDriveCommand(), strings.NewReader("no\n"),
			"drive", "upload", "--file", path, "--node", "file-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("cancelled overwrite made calls: %#v", caller.calls)
		}
		if text := stderr.String(); !strings.Contains(text, "overwrite drive file") || strings.Contains(strings.ToLower(text), "delete") {
			t.Fatalf("unexpected confirmation text: %q", text)
		}
	})

	t.Run("negative document confirmation prevents all writes", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		_, stderr := installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "payload.md", "body")
		err := executeMarkdownDriveCommand(t, newDriveCommand(), strings.NewReader("no\n"),
			"drive", "upload", "--file", path, "--node", "node-1", "--workspace", "workspace-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("cancelled document overwrite made calls: %#v", caller.calls)
		}
		if text := stderr.String(); !strings.Contains(text, "overwrite document-space file") {
			t.Fatalf("unexpected confirmation text: %q", text)
		}
	})
}

func TestMarkdownGlobalAndLocalDryRunAreDistinct(t *testing.T) {
	t.Run("global dry run reads root persistent flag without os args", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownGlobalDryRun(t, newMarkdownCommand(), "markdown", "overwrite")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("global preview made calls: %#v", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("global preview is not pure JSON: %v\n%s", err, stdout.String())
		}
		if payload["dry_run"] != true || payload["executed"] != false {
			t.Fatalf("global preview payload = %#v", payload)
		}
	})

	t.Run("real argv global dry run survives leaf flag shadowing", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		previousArgs := os.Args
		os.Args = []string{
			"dws", "--dry-run", "markdown", "overwrite",
			"--node", "node-1", "--content", "new", "--name", "README.md",
		}
		t.Cleanup(func() { os.Args = previousArgs })
		if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"--dry-run", "markdown", "overwrite",
			"--node", "node-1", "--content", "new", "--name", "README.md",
		); err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("global dry run made MCP calls: %#v", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("global dry-run output is not JSON: %q: %v", stdout.String(), err)
		}
		if payload["dry_run"] != true || payload["preview_kind"] != "plan" {
			t.Fatalf("global dry-run payload = %#v", payload)
		}
	})

	t.Run("local dry run downloads and renders diff", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
			},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old\n")
		path := writeMarkdownDriveFixture(t, "incoming.md", "new\n")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "file-1", "--file", path, "--name", "current.md",
			"--space-id", "space-1", "--dry-run")
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
		if payload["before"] != "old\n" || payload["after"] != "new\n" || payload["operation"] != "overwrite" {
			t.Fatalf("diff payload = %#v", payload)
		}
	})
}

func TestMarkdownCreateDriveAndDocRouting(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		steps      []markdownDriveStep
		wantServer string
		wantTool   string
		wantBody   string
	}{
		{
			name: "content defaults to doc",
			args: []string{"markdown", "create", "--name", "README.md", "--content", "# hello"},
			steps: []markdownDriveStep{
				{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"doc-key"}`},
				{text: `{"created":true}`},
			},
			wantServer: "doc",
			wantTool:   "get_file_upload_info",
			wantBody:   "# hello",
		},
		{
			name: "file with space id uses drive",
			args: []string{"markdown", "create", "--space-id", "space-1"},
			steps: []markdownDriveStep{
				{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"created":true}`},
			},
			wantServer: "drive",
			wantTool:   "get_upload_info",
			wantBody:   "from file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json", steps: test.steps}
			installMarkdownDriveDeps(t, caller)
			if test.wantServer == "drive" {
				path := writeMarkdownDriveFixture(t, "source.md", test.wantBody)
				test.args = append(test.args, "--file", path)
			}
			var uploaded string
			httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, _ int64) error {
				data, err := os.ReadFile(path)
				uploaded = string(data)
				return err
			}
			if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, test.args...); err != nil {
				t.Fatal(err)
			}
			if uploaded != test.wantBody {
				t.Fatalf("uploaded body = %q, want %q", uploaded, test.wantBody)
			}
			if len(caller.calls) != 2 || caller.calls[0].server != test.wantServer || caller.calls[0].tool != test.wantTool {
				t.Fatalf("calls = %#v", caller.calls)
			}
		})
	}
}

func TestMarkdownFetchAutoRoutesAndKeepsJSONPure(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{err: errors.New("not found in drive")},
			{text: `{"name":"doc","extension":"md"}`},
			{text: `{"resourceUrl":"https://download.test/internal.file","fileName":"../../evil.md"}`},
		},
	}
	stdout, stderr := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "external markdown")
	outputDir := t.TempDir()
	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
		"markdown", "fetch", "--node", "node-1", "--output", outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 ||
		caller.calls[0].server != "drive" ||
		caller.calls[1].server != "doc" ||
		caller.calls[2].server != "doc" {
		t.Fatalf("auto-route calls = %#v", caller.calls)
	}
	savedPath := filepath.Join(outputDir, "evil.md")
	if data, err := os.ReadFile(savedPath); err != nil || string(data) != "external markdown" {
		t.Fatalf("safe output file: data=%q err=%v", data, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("fetch stdout is not pure JSON: %v\n%s", err, stdout.String())
	}
	if payload["content"] != "external markdown" || payload["saved_to"] != savedPath || payload["source"] != "doc" {
		t.Fatalf("fetch payload = %#v", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON fetch unexpectedly wrote warnings: %q", stderr.String())
	}
}

func TestMarkdownFetchDryRunAndRawOutput(t *testing.T) {
	t.Run("dry run avoids network", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--output", "out.md")
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

	t.Run("raw output keeps warning on stderr", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		stdout, stderr := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "untrusted body")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--space-id", "space-1")
		if err != nil {
			t.Fatal(err)
		}
		if stdout.String() != "untrusted body\n" {
			t.Fatalf("raw stdout = %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "不可信数据") {
			t.Fatalf("missing out-of-band warning: %q", stderr.String())
		}
	})

	t.Run("route flags remain exclusive", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "fetch", "--node", "node-1", "--space-id", "space-1", "--workspace", "workspace-1")
		if err == nil || !strings.Contains(err.Error(), "互斥") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMarkdownOutputPathRejectsRemoteSymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeMarkdownDriveFixture(t, "target.md", "keep")
	link := filepath.Join(dir, "remote.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := resolveMarkdownOutputPath(dir, "../../remote.md"); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("symlink target changed: data=%q err=%v", data, err)
	}
}

func TestMarkdownOverwriteAndPatchWrites(t *testing.T) {
	t.Run("overwrite uses drive overwrite upload", func(t *testing.T) {
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
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "file-1", "--content", "# changed",
			"--name", "README.md", "--space-id", "space-1", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if uploaded != "# changed" {
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
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["updated"] != true {
			t.Fatalf("overwrite output is not pure server JSON: err=%v output=%q", err, stdout.String())
		}
	})

	t.Run("regex patch uses doc route and literal replacement", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"resourceUrl":"https://download.test/current.md","fileName":"internal.file"}`},
				{text: `{"name":"remote","extension":"md"}`},
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
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "node-1", "--pattern", `v\d`, "--content", "$1",
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
			caller.calls[2].args["name"] != "remote.md" {
			t.Fatalf("patch upload args = %#v", caller.calls[2].args)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload["patched"] != true {
			t.Fatalf("patch output is not pure server JSON: err=%v output=%q", err, stdout.String())
		}
	})
}

func TestMarkdownPatchZeroMatchNeverUploads(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
		},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "alpha beta")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("zero-match patch attempted an upload")
		return nil
	}
	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
		"markdown", "patch", "--node", "file-1", "--pattern", "missing", "--content", "new",
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

func TestMarkdownPatchCancellationStopsBeforeUploadMetadata(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "raw",
		steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
	}
	_, stderr := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "old value")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("cancelled patch attempted upload")
		return nil
	}
	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), strings.NewReader("no\n"),
		"markdown", "patch", "--node", "file-1", "--pattern", "old", "--content", "new",
		"--space-id", "space-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "download_file" {
		t.Fatalf("cancelled patch calls = %#v", caller.calls)
	}
	if text := stderr.String(); !strings.Contains(text, "patch Markdown file") || strings.Contains(strings.ToLower(text), "delete") {
		t.Fatalf("confirmation text = %q", text)
	}
}

func TestMarkdownHelpersCoverSafeNamesAndErrorRouting(t *testing.T) {
	names := map[string]string{
		`../../escape.md`:       "escape.md",
		`..\..\windows.md`:      "windows.md",
		".":                     "unnamed",
		"\x00":                  "unnamed",
		"safe\x00-name.md":      "safe-name.md",
		"/absolute/nested/a.md": "a.md",
	}
	for input, want := range names {
		if got := sanitizeFileName(input); got != want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", input, got, want)
		}
	}
	if got := parseRemoteFileName(`{"result":{"fileName":"../../drive.md"}}`); got != "drive.md" {
		t.Fatalf("drive remote name = %q", got)
	}
	if got := parseRemoteFileName(`{"name":"doc","extension":"md"}`); got != "doc.md" {
		t.Fatalf("doc remote name = %q", got)
	}
	if got := parseRemoteFileName(`{"name":"doc.md","extension":"md"}`); got != "doc.md" {
		t.Fatalf("doc remote name with extension = %q", got)
	}
	if got := parseRemoteFileName(`{`); got != "" {
		t.Fatalf("invalid remote metadata name = %q, want empty", got)
	}
	if got := parseRemoteFileName(`{"extension":"md"}`); got != "" {
		t.Fatalf("remote metadata without name = %q, want empty", got)
	}
	if !isTimeoutCLIError(&CLIError{Code: CodeNetworkTimeout}) ||
		!isTimeoutCLIError(errors.New("request timeout")) ||
		!isPermissionCLIError(&CLIError{Code: CodeAuthPermission}) ||
		!isPermissionCLIError(&PATError{RawJSON: `{}`}) {
		t.Fatal("typed routing errors were not classified")
	}
	if isTimeoutCLIError(nil) || isPermissionCLIError(nil) || isPermissionCLIError(errors.New("ordinary failure")) {
		t.Fatal("ordinary errors were misclassified")
	}
}

func TestMarkdownDownloadHelpersCoverFailuresAndFallbackNames(t *testing.T) {
	downloadPayload := `{"downloadUrl":"https://download.test/content","fileName":"\u0000"}`
	boom := errors.New("download failed")

	t.Run("drive MCP error", func(t *testing.T) {
		caller := &markdownDriveCaller{steps: []markdownDriveStep{{err: boom}}}
		installMarkdownDriveDeps(t, caller)
		if _, _, err := downloadFromDrive(context.Background(), "file-1", "space-1"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("doc MCP error", func(t *testing.T) {
		caller := &markdownDriveCaller{steps: []markdownDriveStep{{err: boom}}}
		installMarkdownDriveDeps(t, caller)
		if _, _, err := downloadFromDoc(context.Background(), "node-1"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("drive fallback name and successful read", func(t *testing.T) {
		caller := &markdownDriveCaller{steps: []markdownDriveStep{{text: downloadPayload}}}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "drive body")
		content, name, err := downloadFromDrive(context.Background(), "file-1", "space-1")
		if err != nil {
			t.Fatal(err)
		}
		if content != "drive body" || name != "download.md" {
			t.Fatalf("download = (%q, %q), want (%q, %q)", content, name, "drive body", "download.md")
		}
	})

	t.Run("doc fallback name and successful read", func(t *testing.T) {
		caller := &markdownDriveCaller{steps: []markdownDriveStep{{text: downloadPayload}}}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "doc body")
		content, name, err := downloadFromDoc(context.Background(), "node-1")
		if err != nil {
			t.Fatal(err)
		}
		if content != "doc body" || name != "download.md" {
			t.Fatalf("download = (%q, %q), want (%q, %q)", content, name, "doc body", "download.md")
		}
	})

	for _, domain := range []struct {
		name string
		run  func() error
	}{
		{
			name: "drive",
			run: func() error {
				_, _, err := downloadFromDrive(context.Background(), "file-1", "")
				return err
			},
		},
		{
			name: "doc",
			run: func() error {
				_, _, err := downloadFromDoc(context.Background(), "node-1")
				return err
			},
		},
	} {
		t.Run(domain.name+" temp directory error", func(t *testing.T) {
			caller := &markdownDriveCaller{steps: []markdownDriveStep{{text: downloadPayload}}}
			installMarkdownDriveDeps(t, caller)
			t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
			err := domain.run()
			if err == nil || !strings.Contains(err.Error(), "创建临时目录失败") {
				t.Fatalf("error = %v, want temporary-directory failure", err)
			}
		})

		t.Run(domain.name+" HTTP error", func(t *testing.T) {
			caller := &markdownDriveCaller{steps: []markdownDriveStep{{text: downloadPayload}}}
			installMarkdownDriveDeps(t, caller)
			httpGetFile = func(context.Context, string, map[string]string, string) error { return boom }
			if err := domain.run(); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want %v", err, boom)
			}
		})

		t.Run(domain.name+" read error", func(t *testing.T) {
			caller := &markdownDriveCaller{steps: []markdownDriveStep{{text: downloadPayload}}}
			installMarkdownDriveDeps(t, caller)
			httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
			err := domain.run()
			if err == nil || !strings.Contains(err.Error(), "读取下载内容失败") {
				t.Fatalf("error = %v, want read failure", err)
			}
		})
	}
}

func TestMarkdownFetchRemoteFileNamePropagatesLookupError(t *testing.T) {
	boom := errors.New("metadata failed")
	caller := &markdownDriveCaller{steps: []markdownDriveStep{{err: boom}}}
	installMarkdownDriveDeps(t, caller)
	if _, err := fetchRemoteFileName(context.Background(), "node-1", true); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestMarkdownAndDrivePublishTypedConstraints(t *testing.T) {
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
			if reflect.DeepEqual(group, want) {
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

	driveUpload := constraints(t, findLeaf(t, newDriveCommand(), "upload"))
	if !hasGroup(driveUpload.MutuallyExclusive, "node", "folder") ||
		!hasGroup(driveUpload.MutuallyExclusive, "space-id", "workspace") {
		t.Fatalf("drive upload constraints = %#v", driveUpload)
	}

	markdown := newMarkdownCommand()
	fetch := constraints(t, findLeaf(t, markdown, "fetch"))
	if !hasGroup(fetch.MutuallyExclusive, "space-id", "workspace") {
		t.Fatalf("markdown fetch constraints = %#v", fetch)
	}
	create := constraints(t, findLeaf(t, markdown, "create"))
	if !hasGroup(create.MutuallyExclusive, "content", "file") ||
		!hasGroup(create.MutuallyExclusive, "space-id", "workspace") ||
		!hasGroup(create.RequireOneOf, "content", "file") {
		t.Fatalf("markdown create constraints = %#v", create)
	}
	overwrite := constraints(t, findLeaf(t, markdown, "overwrite"))
	if !hasGroup(overwrite.MutuallyExclusive, "content", "file") ||
		!hasGroup(overwrite.MutuallyExclusive, "space-id", "workspace") ||
		!hasGroup(overwrite.RequireOneOf, "content", "file") {
		t.Fatalf("markdown overwrite constraints = %#v", overwrite)
	}
	patch := constraints(t, findLeaf(t, markdown, "patch"))
	if !hasGroup(patch.MutuallyExclusive, "space-id", "workspace") {
		t.Fatalf("markdown patch constraints = %#v", patch)
	}
}

func TestMarkdownContentSourcesAndHumanDiffs(t *testing.T) {
	caller := &markdownDriveCaller{format: "raw"}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	cmd := &cobra.Command{Use: "source"}
	cmd.SetIn(strings.NewReader("from stdin"))

	if got, err := resolveMarkdownContentSource(cmd, "-"); err != nil || got != "from stdin" {
		t.Fatalf("stdin source = %q, %v", got, err)
	}
	path := writeMarkdownDriveFixture(t, "source.md", "from file")
	if got, err := resolveMarkdownContentSource(cmd, "@"+path); err != nil || got != "from file" {
		t.Fatalf("file source = %q, %v", got, err)
	}
	if got, err := resolveMarkdownContentSource(cmd, "literal"); err != nil || got != "literal" {
		t.Fatalf("literal source = %q, %v", got, err)
	}
	if _, err := resolveMarkdownContentSource(cmd, "@"); err == nil {
		t.Fatal("empty @file source unexpectedly succeeded")
	}
	if _, err := resolveMarkdownContentSource(cmd, "@"+filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("missing @file source unexpectedly succeeded")
	}

	longBefore := strings.Repeat("old\n", 25)
	longAfter := strings.Repeat("new\n", 25)
	overwriteDiff := renderMarkdownOverwriteDiff("node-1", longBefore, longAfter)
	if !strings.Contains(overwriteDiff, "... (") || !strings.Contains(overwriteDiff, "No write performed") {
		t.Fatalf("overwrite diff did not truncate safely:\n%s", overwriteDiff)
	}
	if err := printMarkdownPatchDiff("node-1", "old", "new", 1); err != nil {
		t.Fatal(err)
	}
	if text := stdout.String(); !strings.Contains(text, "markdown patch") || !strings.Contains(text, "- old") || !strings.Contains(text, "+ new") {
		t.Fatalf("human patch diff = %q", text)
	}
	stdout.Reset()
	if err := printMarkdownDryRun(map[string]any{"operation": "fetch"}, "获取 Markdown 内容", "node-1"); err != nil {
		t.Fatal(err)
	}
	if text := stdout.String(); !strings.Contains(text, "获取 Markdown 内容") || !strings.Contains(text, "node-1") {
		t.Fatalf("human dry-run output = %q", text)
	}
	if markdownRouteName(false) != "drive" || markdownRouteName(true) != "doc" {
		t.Fatal("route names are incorrect")
	}
}

func TestMarkdownValidationRejectsAmbiguousOrUnsafeInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "create requires source",
			args: []string{"markdown", "create", "--name", "a.md"},
			want: "必须指定其一",
		},
		{
			name: "create rejects both sources",
			args: []string{"markdown", "create", "--name", "a.md", "--content", "x", "--file", "x.md"},
			want: "互斥",
		},
		{
			name: "create content requires name",
			args: []string{"markdown", "create", "--content", "x"},
			want: "必须指定 --name",
		},
		{
			name: "create route flags are exclusive",
			args: []string{"markdown", "create", "--name", "a.md", "--content", "x", "--space-id", "s", "--workspace", "w"},
			want: "--space-id 与 --workspace 互斥",
		},
		{
			name: "overwrite requires node",
			args: []string{"markdown", "overwrite", "--content", "x", "--name", "a.md"},
			want: "flag --node is required",
		},
		{
			name: "overwrite requires source",
			args: []string{"markdown", "overwrite", "--node", "n", "--space-id", "s"},
			want: "必须指定其一",
		},
		{
			name: "overwrite rejects both sources",
			args: []string{"markdown", "overwrite", "--node", "n", "--content", "x", "--file", "x.md", "--space-id", "s"},
			want: "互斥",
		},
		{
			name: "overwrite route flags are exclusive",
			args: []string{"markdown", "overwrite", "--node", "n", "--content", "x", "--name", "a.md", "--space-id", "s", "--workspace", "w"},
			want: "--space-id 与 --workspace 互斥",
		},
		{
			name: "patch requires all values",
			args: []string{"markdown", "patch", "--node", "n", "--pattern", "x"},
			want: "均为必填",
		},
		{
			name: "patch route flags are exclusive",
			args: []string{"markdown", "patch", "--node", "n", "--pattern", "x", "--content", "y", "--space-id", "s", "--workspace", "w"},
			want: "--space-id 与 --workspace 互斥",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json"}
			installMarkdownDriveDeps(t, caller)
			err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("validation failure made calls: %#v", caller.calls)
			}
		})
	}

	t.Run("create rejects non-markdown file", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "source.txt", "text")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "create", "--file", path)
		if err == nil || !strings.Contains(err.Error(), "必须以 .md 结尾") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("overwrite rejects directory source", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "overwrite", "--node", "n", "--file", t.TempDir(), "--space-id", "s")
		if err == nil || !strings.Contains(err.Error(), "是目录而非文件") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMarkdownPatchPreviewAndRegexErrors(t *testing.T) {
	t.Run("human local preview does not upload", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "raw",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
			t.Fatal("local patch preview attempted upload")
			return nil
		}
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "n", "--pattern", "old", "--content", "new",
			"--space-id", "s", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %#v", caller.calls)
		}
		if text := stdout.String(); !strings.Contains(text, "- old value") || !strings.Contains(text, "+ new value") {
			t.Fatalf("preview output = %q", text)
		}
	})

	t.Run("invalid regex stops after fetch", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`}},
		}
		installMarkdownDriveDeps(t, caller)
		installMarkdownHTTPGet(t, "old value")
		err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
			"markdown", "patch", "--node", "n", "--pattern", "[", "--content", "new",
			"--space-id", "s", "--regex", "--yes")
		if err == nil || !strings.Contains(err.Error(), "正则表达式编译失败") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("invalid regex made extra calls: %#v", caller.calls)
		}
	})
}

func TestMarkdownOverwritePreservesRemoteName(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"name":"remote","extension":"md"}`},
			{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"key-1"}`},
			{text: `{"updated":true}`},
		},
	}
	installMarkdownDriveDeps(t, caller)
	path := writeMarkdownDriveFixture(t, "local-name.md", "updated")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
		"markdown", "overwrite", "--node", "node-1", "--file", path,
		"--workspace", "workspace-1", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 || caller.calls[0].tool != "get_document_info" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if caller.calls[1].args["name"] != "remote.md" || caller.calls[2].args["name"] != "remote.md" {
		t.Fatalf("remote name was not preserved: %#v %#v", caller.calls[1].args, caller.calls[2].args)
	}
}

func TestMarkdownDomainResolutionErrorsAreActionable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "超时"},
		{name: "permission", err: &CLIError{Code: CodeAuthPermission, Message: "denied"}, want: "无权限"},
		{name: "not found", err: errors.New("missing"), want: "均未找到"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{
				format: "json",
				steps:  []markdownDriveStep{{err: test.err}, {err: test.err}},
			}
			installMarkdownDriveDeps(t, caller)
			_, err := resolveFileDomain(context.Background(), "node-1")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if len(caller.calls) != 2 {
				t.Fatalf("route probes = %#v", caller.calls)
			}
		})
	}

	t.Run("drive success wins without doc probe", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps:  []markdownDriveStep{{text: `{"result":{"fileName":"a.md"}}`}},
		}
		installMarkdownDriveDeps(t, caller)
		domain, err := resolveFileDomain(context.Background(), "node-1")
		if err != nil || domain != "drive" || len(caller.calls) != 1 {
			t.Fatalf("domain=%q err=%v calls=%#v", domain, err, caller.calls)
		}
	})
}
