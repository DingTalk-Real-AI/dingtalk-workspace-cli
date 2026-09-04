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
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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

func TestTypedValidationErrorGateFinalCommandTree(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := cmd.CommandPath()
		if cmd.Args != nil {
			for _, args := range [][]string{nil, make([]string, 256)} {
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
}

func TestTypedValidationErrorGateRepresentativeCommands(t *testing.T) {
	for _, args := range [][]string{
		{"mcp", "published", "tools"},
		{"skill", "install"},
		{"skill", "get"},
		{"skill", "search", "--query", "value", "--unknown-validation-gate-flag"},
		{"oa", "approval", "list-by-admin"},
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
