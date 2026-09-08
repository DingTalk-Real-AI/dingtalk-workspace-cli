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

package aitable

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type platformCoverageCaller struct {
	called  bool
	product string
	tool    string
	args    map[string]any
}

func (f *platformCoverageCaller) reset() {
	f.called, f.product, f.tool, f.args = false, "", "", nil
}

func (f *platformCoverageCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.called, f.product, f.tool, f.args = true, product, tool, args
	return &edition.ToolResult{
		Content: []edition.ContentBlock{{Type: "text", Text: `{"result":[]}`}},
	}, nil
}

func (f *platformCoverageCaller) Format() string { return "json" }
func (f *platformCoverageCaller) DryRun() bool   { return false }
func (f *platformCoverageCaller) Fields() string { return "" }
func (f *platformCoverageCaller) JQ() string     { return "" }

func newPlatformCoverageRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	return root
}

func TestCrossPlatformCoverageImportUploadRequiresPositiveFileSize(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)

	tests := []struct {
		name     string
		fileSize string
		wantErr  bool
	}{
		{name: "missing", wantErr: true},
		{name: "zero", fileSize: "0", wantErr: true},
		{name: "negative", fileSize: "-1", wantErr: true},
		{name: "positive", fileSize: "204800"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake.reset()
			root := newPlatformCoverageRoot()
			args := []string{
				"aitable", "+import-upload",
				"--base-id", "base-smoke",
				"--file-name", "data.xlsx",
				"--yes",
			}
			if test.fileSize != "" {
				args = append(args, "--file-size", test.fileSize)
			}
			root.SetArgs(args)
			err := root.Execute()

			if test.wantErr {
				if err == nil {
					t.Fatal("expected --file-size validation error")
				}
				if !strings.Contains(err.Error(), "--file-size") {
					t.Fatalf("error = %q, want --file-size validation", err)
				}
				if fake.called {
					t.Fatalf("invalid file size called %s/%s with %#v", fake.product, fake.tool, fake.args)
				}
				return
			}

			if err != nil {
				t.Fatalf("positive file size returned error: %v", err)
			}
			if !fake.called || fake.product != "aitable" || fake.tool != "prepare_import_upload" {
				t.Fatalf("tool call = called:%v %s/%s, want aitable/prepare_import_upload", fake.called, fake.product, fake.tool)
			}
			if got := fake.args["fileSize"]; got != 204800 {
				t.Fatalf("fileSize = %#v, want 204800", got)
			}
		})
	}
}

func TestCrossPlatformCoverageShareFormShortcutMatchesPublishedSchema(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)

	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"aitable", "+form-share-update",
		"--base-id", "base-smoke", "--table-id", "table-smoke", "--view-id", "view-smoke",
		"--enabled", "false", "--auth-type-code=2", "--auth-data=u1,u2",
		"--submit-times-limit=0", "--submit-times-user-limit=3",
		"--form-start-time=1788307200000", "--form-end-time=1788393600000",
		"--form-name", "活动报名", "--form-desc", "请填写", "--anonymous-submit", "true",
		"--load-last-submit", "false", "--reply-notice", "true", "--share-uid-list=u1,u2", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !fake.called || fake.product != "aitable" || fake.tool != "update_share_form" {
		t.Fatalf("tool call = called:%v %s/%s, want aitable/update_share_form", fake.called, fake.product, fake.tool)
	}
	expected := map[string]any{
		"enabled": false, "authTypeCode": 2, "authData": "u1,u2",
		"submitTimesLimit": 0, "submitTimesUserLimit": 3,
		"formStartTime": 1788307200000, "formEndTime": 1788393600000,
		"formName": "活动报名", "formDesc": "请填写", "anonymousSubmit": true,
		"loadLastSubmit": false, "replyNotice": true, "shareUidList": "u1,u2",
	}
	for key, want := range expected {
		if fake.args[key] != want {
			t.Fatalf("share form arg %s = %#v, want %#v; all args = %#v", key, fake.args[key], want, fake.args)
		}
	}

	fake.reset()
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{
		"aitable", "+form-share-update",
		"--base-id", "base-smoke", "--table-id", "table-smoke", "--view-id", "view-smoke",
	})
	if err := root.Execute(); err == nil || fake.called {
		t.Fatalf("missing partial update must fail before MCP call: err=%v called=%v", err, fake.called)
	}
}

func TestCrossPlatformCoverageShareFormShortcutExplicitEmptyUpdate(t *testing.T) {
	for flag, property := range map[string]string{"form-desc": "formDesc", "auth-data": "authData", "share-uid-list": "shareUidList"} {
		t.Run(flag, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDepsForTest(t, fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"aitable", "+form-share-update", "--base-id=b", "--table-id=t", "--view-id=v", "--" + flag + "=", "--yes"})
			if err := root.Execute(); err != nil || !fake.called || fake.args[property] != "" || len(fake.args) != 4 {
				t.Fatalf("err=%v called=%v args=%#v", err, fake.called, fake.args)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableShortcutDeleteConfirmationMatchesSnapshot(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	tests := []struct {
		tool string
		args []string
	}{
		{tool: "delete_base", args: []string{"aitable", "+base-delete", "--base-id=b", "--yes"}},
		{tool: "delete_table", args: []string{"aitable", "+table-delete", "--base-id=b", "--table-id=t", "--yes"}},
		{tool: "delete_field", args: []string{"aitable", "+field-delete", "--base-id=b", "--table-id=t", "--field-id=f", "--yes"}},
		{tool: "delete_view", args: []string{"aitable", "+view-delete", "--base-id=b", "--table-id=t", "--view-id=v", "--yes"}},
		{tool: "delete_dashboard", args: []string{"aitable", "+dashboard-delete", "--base-id=b", "--dashboard-id=d", "--yes"}},
		{tool: "delete_chart", args: []string{"aitable", "+chart-delete", "--base-id=b", "--dashboard-id=d", "--chart-id=c", "--yes"}},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			fake.reset()
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !fake.called || fake.product != "aitable" || fake.tool != tc.tool {
				t.Fatalf("call = called:%v %s/%s %#v", fake.called, fake.product, fake.tool, fake.args)
			}
			if confirm, ok := fake.args["confirm"].(bool); !ok || !confirm {
				t.Fatalf("confirm = %#v", fake.args["confirm"])
			}
		})
	}
}

func TestCrossPlatformCoverageAitableShortcutUpdatesDoNotSendDeleteConfirmation(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{tool: "update_table", args: []string{"aitable", "+table-update", "--base-id=b", "--table-id=t", "--name=n", "--yes"}},
		{tool: "update_chart", args: []string{"aitable", "+chart-update", "--base-id=b", "--dashboard-id=d", "--chart-id=c", `--config={"chartName":"n"}`, "--yes"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			fake.reset()
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !fake.called || fake.tool != tc.tool {
				t.Fatalf("call = called:%v %s/%s %#v", fake.called, fake.product, fake.tool, fake.args)
			}
			if _, exists := fake.args["confirm"]; exists {
				t.Fatalf("update must not send delete-only confirm: %#v", fake.args)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableShortcutFieldNameFailsBeforeMCP(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"aitable", "+field-update", "--base-id=b", "--table-id=t", "--field-id=f",
		"--name=" + strings.Repeat("😀", 76),
	})
	if err := root.Execute(); err == nil {
		t.Fatal("expected UTF-16 field-name validation error")
	}
	if fake.called {
		t.Fatalf("invalid field name reached MCP: %s/%s %#v", fake.product, fake.tool, fake.args)
	}
}
