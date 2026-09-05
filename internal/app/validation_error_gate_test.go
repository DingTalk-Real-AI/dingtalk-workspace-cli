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

package app

import (
	stderrors "errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func requireFinalValidationError(t *testing.T, path string, err error) {
	t.Helper()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("%s returned naked %T error: %v", path, err, err)
	}
	if typed.Category != apperrors.CategoryValidation || typed.ExitCode() != apperrors.ExitCodeValidation {
		t.Fatalf("%s returned category %q exit %d", path, typed.Category, typed.ExitCode())
	}
}

func TestCrossPlatformCoverageTypedValidationErrorGateFinalCommandTree(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	tooManyArgs := make([]string, 256)
	nodes := 0
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		nodes++
		path := cmd.CommandPath()
		if cmd.Args != nil {
			for _, args := range [][]string{nil, tooManyArgs} {
				if err := cmd.Args(cmd, args); err != nil {
					requireFinalValidationError(t, path+" Args", err)
				}
			}
		}
		flagErr := cmd.FlagErrorFunc()(cmd, stderrors.New("synthetic flag parse error"))
		requireFinalValidationError(t, path+" FlagErrorFunc", flagErr)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	t.Logf("distribution validation coverage: %d nodes", nodes)
}

func TestCrossPlatformCoverageTypedValidationErrorGateRepresentativeCommands(t *testing.T) {
	for _, args := range [][]string{
		{"mcp", "published", "tools"},
		{"skill", "install"},
		{"skill", "get"},
		{"skill", "search", "--query", "value", "--unknown-validation-gate-flag"},
		{"oa", "approval", "list-by-admin"},
		{"oa", "approval", "list-by-admin", "--request", "{"},
		{"audit", "tail", "--lines", "0"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := NewSchemaSourceRootCommand()
			root.SetArgs(args)
			executed, err := root.ExecuteC()
			path := strings.Join(args, " ")
			if executed != nil {
				path = executed.CommandPath()
			}
			if err == nil {
				t.Fatalf("%s succeeded, want validation failure", path)
			}
			requireFinalValidationError(t, path, err)
		})
	}
}

func TestCrossPlatformCoverageTypedValidationErrorGateExtensions(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	businessCalls := 0
	edition.Override(&edition.Hooks{RegisterExtraCommands: func(root *cobra.Command, _ edition.ToolCaller) {
		leaf := &cobra.Command{Use: "validation-extension", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { businessCalls++; return nil }}
		leaf.Flags().String("required", "", "")
		_ = leaf.MarkFlagRequired("required")
		leaf.Flags().String("left", "", "")
		leaf.Flags().String("right", "", "")
		leaf.MarkFlagsOneRequired("left", "right")
		root.AddCommand(leaf)
	}})
	testseam.Swap(t, &rootLoadPlugins, func(root *cobra.Command, _ *pipeline.Engine, runner executor.Runner) []*cobra.Command {
		return buildPluginCommands([]mcptypes.ServerDescriptor{conferencePluginDescriptor()}, runner, root)
	})
	for _, args := range [][]string{
		{"validation-extension", "unexpected"},
		{"validation-extension"},
		{"validation-extension", "--required", "value"},
		{"validation-extension", "--unknown"},
		{"conference", "camera", "open", "--unknown"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(args)
			_, err := root.ExecuteC()
			requireFinalValidationError(t, strings.Join(args, " "), err)
			if businessCalls != 0 {
				t.Fatal("invalid extension args reached business execution")
			}
			if err := corecmd.PrepareCommandTree(root); err == nil {
				t.Fatal("runtime root was not already prepared")
			}
		})
	}
	t.Log("runtime validation coverage: edition positional/required/group/parser; plugin overlay parser")
}
