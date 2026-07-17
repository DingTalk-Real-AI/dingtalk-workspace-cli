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
