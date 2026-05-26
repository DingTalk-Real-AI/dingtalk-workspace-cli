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

package helpers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type docCommandRunner struct {
	calls int
	last  executor.Invocation
}

func (r *docCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	r.last = invocation
	return executor.Result{Invocation: invocation}, nil
}

func executeDocCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func newDocTestRoot(runner executor.Runner) *cobra.Command {
	cmd := docHandler{}.Command(runner)
	cmd.PersistentFlags().Bool("dry-run", false, "dry run")
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	return cmd
}

func TestDocPermissionListLimitAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "limit", args: []string{"--node", "NODE_001", "--limit", "50"}, want: 50},
		{name: "max results", args: []string{"--node", "NODE_001", "--max-results", "40"}, want: 40},
		{name: "page size", args: []string{"--node", "NODE_001", "--page-size", "10"}, want: 10},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocPermissionListCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "list_permission" {
				t.Fatalf("tool = %q, want list_permission", runner.last.Tool)
			}
			if got := runner.last.Params["maxResults"]; got != tc.want {
				t.Fatalf("maxResults = %#v, want %d", got, tc.want)
			}
		})
	}
}

func TestDocPermissionListMaxresultsRejected(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocPermissionListCommand(runner)
	_, _, err := executeDocCommand(t, cmd, "--node", "NODE_001", "--maxresults", "10")
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag: --maxresults") {
		t.Fatalf("error = %q, want unknown flag for --maxresults", err.Error())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestDocUpdateOverwriteRequiresYes(t *testing.T) {
	t.Parallel()

	runner := &docCommandRunner{}
	cmd := newDocTestRoot(runner)
	_, _, err := executeDocCommand(t, cmd,
		"update",
		"--node", "NODE_001",
		"--content", "# overwrite probe",
		"--mode", "overwrite",
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want --yes validation failure")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %q, want --yes hint", err.Error())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestDocUpdateOverwriteAllowsYesAndDryRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		extraArgs  []string
		wantDryRun bool
	}{
		{name: "yes", extraArgs: []string{"--yes"}},
		{name: "dry run", extraArgs: []string{"--dry-run"}, wantDryRun: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocTestRoot(runner)
			args := []string{
				"update",
				"--node", "NODE_001",
				"--content", "# overwrite probe",
				"--mode", "overwrite",
			}
			args = append(args, tc.extraArgs...)
			_, errOut, err := executeDocCommand(t, cmd, args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", runner.calls)
			}
			if runner.last.DryRun != tc.wantDryRun {
				t.Fatalf("DryRun = %v, want %v", runner.last.DryRun, tc.wantDryRun)
			}
		})
	}
}

func TestDocListAcceptsFolderCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "node", args: []string{"--node", "FOLDER_001"}},
		{name: "file id", args: []string{"--file-id", "FOLDER_001"}},
		{name: "nodee typo", args: []string{"--nodee", "FOLDER_001"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocListCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "list_nodes" {
				t.Fatalf("tool = %q, want list_nodes", runner.last.Tool)
			}
			if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
				t.Fatalf("folderId = %#v, want FOLDER_001", got)
			}
		})
	}
}

func TestDocCreateAcceptsParentFolderAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "parent folder id",
			args: []string{"--name", "doc", "--parent-folder-id", "FOLDER_001", "--content", "hello"},
		},
		{
			name: "parent folder",
			args: []string{"--name", "doc", "--parent-folder", "FOLDER_001", "--content", "hello"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &docCommandRunner{}
			cmd := newDocCreateCommand(runner)
			_, errOut, err := executeDocCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
			}
			if runner.last.Tool != "create_document" {
				t.Fatalf("tool = %q, want create_document", runner.last.Tool)
			}
			if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
				t.Fatalf("folderId = %#v, want FOLDER_001", got)
			}
		})
	}
}

func TestDocCreateAcceptsContentPathAlias(t *testing.T) {
	t.Parallel()

	contentPath := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(contentPath, []byte("# from file"), 0600); err != nil {
		t.Fatal(err)
	}

	runner := &docCommandRunner{}
	cmd := newDocCreateCommand(runner)
	_, errOut, err := executeDocCommand(t, cmd, "--name", "doc", "--content-path", contentPath)
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut)
	}
	if runner.last.Tool != "create_document" {
		t.Fatalf("tool = %q, want create_document", runner.last.Tool)
	}
	if got := runner.last.Params["markdown"]; got != "# from file" {
		t.Fatalf("markdown = %#v, want # from file", got)
	}
}
