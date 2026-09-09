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
)

// The html domain shares the textfile create engine with markdown. These tests
// pin the html-specific deltas: the .html/.htm extension contract, the
// text/html MIME type on the Drive upload, the HTML operation prose, and the
// html leaf declaration surface. Engine-wide branches that markdown already
// covers through the same shared functions stay covered there.

func TestCrossPlatformCoverageHTMLCreateRoutesThroughSharedEngine(t *testing.T) {
	t.Run("content defaults to doc upload", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"doc-key"}`},
				{text: `{"created":true}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, _ int64) error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if string(data) != "<h1>hello</h1>" {
				t.Fatalf("uploaded body = %q", string(data))
			}
			return nil
		}

		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--name", "index.html", "--content", "<h1>hello</h1>")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 ||
			caller.calls[0].server != "doc" || caller.calls[0].tool != "get_file_upload_info" ||
			caller.calls[1].server != "doc" || caller.calls[1].tool != "commit_uploaded_file" {
			t.Fatalf("calls = %#v", caller.calls)
		}
		if caller.calls[0].args["name"] != "index.html" {
			t.Fatalf("upload name = %#v", caller.calls[0].args)
		}
	})

	t.Run("html file with space id uses drive with html mime", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"created":true}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "index.html", "<h1>from file</h1>")
		var uploaded string
		httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, _ int64) error {
			data, err := os.ReadFile(path)
			uploaded = string(data)
			return err
		}

		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--space-id", "space-1", "--file", path)
		if err != nil {
			t.Fatal(err)
		}
		if uploaded != "<h1>from file</h1>" {
			t.Fatalf("uploaded body = %q", uploaded)
		}
		if len(caller.calls) != 2 ||
			caller.calls[0].server != "drive" || caller.calls[0].tool != "get_upload_info" ||
			caller.calls[1].server != "drive" || caller.calls[1].tool != "commit_upload" {
			t.Fatalf("calls = %#v", caller.calls)
		}
		if caller.calls[0].args["mimeType"] != "text/html" {
			t.Fatalf("drive upload mimeType = %#v, want text/html", caller.calls[0].args["mimeType"])
		}
		if caller.calls[0].args["spaceId"] != "space-1" || caller.calls[0].args["fileName"] != "index.html" {
			t.Fatalf("drive upload args = %#v", caller.calls[0].args)
		}
	})

	t.Run("htm file is accepted as a second extension", func(t *testing.T) {
		caller := &markdownDriveCaller{
			format: "json",
			steps: []markdownDriveStep{
				{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"created":true}`},
			},
		}
		installMarkdownDriveDeps(t, caller)
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		path := writeMarkdownDriveFixture(t, "page.htm", "<p>legacy</p>")

		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--space-id", "space-1", "--file", path)
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 2 || caller.calls[0].tool != "get_upload_info" {
			t.Fatalf("htm upload calls = %#v", caller.calls)
		}
		if caller.calls[0].args["fileName"] != "page.htm" {
			t.Fatalf("htm fileName = %#v", caller.calls[0].args["fileName"])
		}
	})
}

func TestCrossPlatformCoverageHTMLCreateDryRunPlansStayOffline(t *testing.T) {
	t.Run("json plan keeps folder resolution offline", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--name", "index.html", "--content", "<h1>hello</h1>", "--folder", "unknown-offline")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dry run made MCP calls: %#v", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("dry-run output is not JSON: %v\n%s", err, stdout.String())
		}
		if payload["dry_run"] != true || payload["executed"] != false ||
			payload["file_name"] != "index.html" || payload["folder_id"] != "unknown-offline" ||
			payload["operation"] != "create" {
			t.Fatalf("dry-run payload = %#v", payload)
		}
	})

	t.Run("human plan names the HTML operation", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "raw", dryRun: true}
		stdout, _ := installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--name", "index.html", "--content", "<h1>hello</h1>", "--space-id", "space-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dry run made MCP calls: %#v", caller.calls)
		}
		if text := stdout.String(); !strings.Contains(text, "创建 HTML 文件") || !strings.Contains(text, "index.html") {
			t.Fatalf("human dry-run output = %q", text)
		}
	})
}

func TestCrossPlatformCoverageHTMLCreateValidationRejectsBadInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "create requires source",
			args: []string{"html", "create", "--name", "a.html"},
			want: "必须指定其一",
		},
		{
			name: "create rejects both sources",
			args: []string{"html", "create", "--name", "a.html", "--content", "x", "--file", "x.html"},
			want: "互斥",
		},
		{
			name: "create content requires name",
			args: []string{"html", "create", "--content", "x"},
			want: "必须指定 --name",
		},
		{
			name: "create route flags are exclusive",
			args: []string{"html", "create", "--name", "a.html", "--content", "x", "--space-id", "s", "--workspace", "w"},
			want: "--space-id 与 --workspace 互斥",
		},
		{
			name: "create name must end with html extension",
			args: []string{"html", "create", "--name", "a.md", "--content", "x"},
			want: "--name 必须以 .html/.htm 结尾",
		},
		{
			name: "create rejects missing file",
			args: []string{"html", "create", "--file", filepath.Join(t.TempDir(), "missing.html")},
			want: "无法读取文件",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json"}
			installMarkdownDriveDeps(t, caller)
			err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("validation failure made calls: %#v", caller.calls)
			}
		})
	}

	t.Run("create rejects non-html file", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		path := writeMarkdownDriveFixture(t, "source.md", "# text")
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--file", path)
		if err == nil || !strings.Contains(err.Error(), "必须以 .html/.htm 结尾") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("create rejects directory source", func(t *testing.T) {
		caller := &markdownDriveCaller{format: "json"}
		installMarkdownDriveDeps(t, caller)
		err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
			"html", "create", "--file", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "是目录而非文件") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageHTMLCreateFolderProbeFailureIsActionable(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{err: errors.New("not found in drive")},
			{err: errors.New("not found in doc")},
		},
	}
	installMarkdownDriveDeps(t, caller)
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("folder probe failure attempted upload")
		return nil
	}

	err := executeMarkdownDriveCommand(t, newHTMLCommand(), nil,
		"html", "create", "--name", "index.html", "--content", "<h1>x</h1>", "--folder", "missing-folder")
	if err == nil || !strings.Contains(err.Error(), "无法根据 --folder missing-folder 自动识别 HTML 创建目标域") {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("probe failure calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageHTMLCreateDelegationTargetCarriesHTMLMIME(t *testing.T) {
	server, tool, args := textFileCreateDelegationTarget(htmlTextFileSpec, "index.html", 12, "", "space-1", "")
	if server != "drive" || tool != "get_upload_info" {
		t.Fatalf("space route = %s.%s", server, tool)
	}
	if args["mimeType"] != "text/html" || args["fileName"] != "index.html" ||
		args["spaceId"] != "space-1" || args["fileSize"] != float64(12) {
		t.Fatalf("space route args = %#v", args)
	}
	if _, ok := args["parentId"]; ok {
		t.Fatalf("space route without folder must not set parentId: %#v", args)
	}

	server, tool, args = textFileCreateDelegationTarget(htmlTextFileSpec, "index.html", 12, "folder-1", "space-1", "")
	if server != "drive" || tool != "get_upload_info" || args["parentId"] != "folder-1" || args["mimeType"] != "text/html" {
		t.Fatalf("space+folder route = %s.%s %#v", server, tool, args)
	}

	server, tool, args = textFileCreateDelegationTarget(htmlTextFileSpec, "index.html", 12, "folder-1", "", "")
	if server != "drive" || tool != "get_file_info" || args["fileId"] != "folder-1" {
		t.Fatalf("folder probe route = %s.%s %#v", server, tool, args)
	}

	server, tool, args = textFileCreateDelegationTarget(htmlTextFileSpec, "index.html", 12, "", "", "workspace-1")
	if server != "doc" || tool != "get_file_upload_info" || args["workspaceId"] != "workspace-1" {
		t.Fatalf("workspace route = %s.%s %#v", server, tool, args)
	}

	server, tool, args = textFileCreateDelegationTarget(htmlTextFileSpec, "index.html", 12, "", "", "")
	if server != "doc" || tool != "get_file_upload_info" || len(args) != 0 {
		t.Fatalf("default route = %s.%s %#v", server, tool, args)
	}
}

func TestCrossPlatformCoverageHTMLPublishesTypedConstraints(t *testing.T) {
	root := newHTMLCommand()
	if lookup := root.PersistentFlags().Lookup(FlagPrincipalUserID); lookup == nil {
		t.Fatal("html root does not declare delegation auth flag")
	}
	leaf, remaining, err := root.Find([]string{"create"})
	if err != nil || leaf == nil || len(remaining) != 0 {
		t.Fatalf("html create leaf not found: leaf=%v remaining=%v err=%v", leaf, remaining, err)
	}

	raw := ""
	if leaf.Annotations != nil {
		raw = leaf.Annotations["dws.schema.constraints"]
	}
	if raw == "" {
		t.Fatal("html create has no typed constraints annotation")
	}
	var parsed struct {
		MutuallyExclusive [][]string `json:"mutually_exclusive"`
		RequireOneOf      [][]string `json:"require_one_of"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("html create constraints: %v", err)
	}
	hasGroup := func(groups [][]string, names ...string) bool {
		for _, group := range groups {
			if len(group) == len(names) {
				match := true
				for i, name := range names {
					if group[i] != name {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
		return false
	}
	if !hasGroup(parsed.MutuallyExclusive, "content", "file") ||
		!hasGroup(parsed.MutuallyExclusive, "space-id", "workspace") ||
		!hasGroup(parsed.RequireOneOf, "content", "file") {
		t.Fatalf("html create constraints = %#v", parsed)
	}
}
