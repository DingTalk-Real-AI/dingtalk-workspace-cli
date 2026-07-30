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
	"bytes"
	stderrors "errors"
	"io"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestLeadingPersistentFlagVariantsReachTheRealCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "camel case", args: []string{"--dryRun", "chat", "bot", "find", "--help"}},
		{name: "fuzzy boolean", args: []string{"--dry-rnu", "chat", "bot", "find", "--help"}},
		{name: "fuzzy value", args: []string{"--profle", "corp:user", "chat", "bot", "find", "--help"}},
		{name: "sticky value", args: []string{"--timeout30", "chat", "bot", "find", "--help"}},
		{name: "sticky boolean value", args: []string{"--verbosefalse", "chat", "bot", "find", "--help"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := NewSchemaSourceRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), test.args)
			if err != nil {
				t.Fatalf("RunPreParseArgs(%v) error = %v", test.args, err)
			}
			if ctx == nil || ctx.Command != "dws chat bot find" || len(ctx.Corrections) == 0 {
				t.Fatalf("RunPreParseArgs(%v) context = %#v", test.args, ctx)
			}
			if err := root.Execute(); err != nil {
				t.Fatalf("corrected leading persistent flag failed: %v", err)
			}
		})
	}
}

func TestPreParseConflictHonorsErrorPresentationFlags(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	args := []string{
		"chat", "message", "send",
		"--user-id", "123", "--user", "456", "--text", "hi",
		"--format", "table", "--debug",
	}
	_, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if err == nil {
		t.Fatal("alias/canonical conflict unexpectedly succeeded")
	}
	if wantsJSONErrors(root) {
		t.Fatal("--format table was not applied before rendering the PreParse error")
	}
	if got := resolveVerbosity(root); got != apperrors.VerbosityDebug {
		t.Fatalf("PreParse error verbosity = %v, want debug", got)
	}

	err = newPreParseValidationError(err)
	var structured *apperrors.Error
	if !stderrors.As(err, &structured) {
		t.Fatalf("PreParse validation error = %T, want *errors.Error", err)
	}
	if strings.Contains(structured.Message, "pipeline") || strings.Contains(structured.Message, "semantic-alias") ||
		strings.Contains(structured.Cause.Error(), "pipeline") || strings.Contains(structured.Cause.Error(), "semantic-alias") {
		t.Fatalf("internal pipeline identity leaked to user error: message=%q cause=%q", structured.Message, structured.Cause)
	}
	var conflict *pipeline.FlagConflictError
	if !stderrors.As(err, &conflict) {
		t.Fatalf("PreParse validation error lost FlagConflictError: %v", err)
	}
	var output bytes.Buffer
	if printErr := printExecutionError(root, &output, &output, err); printErr != nil {
		t.Fatalf("printExecutionError() error = %v", printErr)
	}
	rendered := output.String()
	if strings.HasPrefix(strings.TrimSpace(rendered), "{") {
		t.Fatalf("--format table rendered JSON:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Reason: parameter_conflict") || !strings.Contains(rendered, "Cause:") {
		t.Fatalf("--debug details missing from early error:\n%s", rendered)
	}
}
