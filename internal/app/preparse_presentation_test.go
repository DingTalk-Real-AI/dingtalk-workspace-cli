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
	"slices"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestCrossPlatformCoverageLeadingPersistentFlagVariantsReachTheRealCommand(t *testing.T) {
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

func TestCrossPlatformCoverageFuzzyRootBooleanBetweenGroupAndLeafKeepsLeafPreParse(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	args := []string{
		"aisearch", "--query", "Alice", "--yess", "enterprise",
		"--queries", "fixture", "--content-types", "document", "--time_range", "本周", "--help",
	}
	ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if err != nil {
		t.Fatalf("RunPreParseArgs(%v) error = %v", args, err)
	}
	if ctx == nil || ctx.Command != "dws aisearch enterprise" ||
		!slices.Contains(ctx.Args, "--yes") || !slices.Contains(ctx.Args, "--types") || !slices.Contains(ctx.Args, "--time-range") {
		t.Fatalf("group-middle fuzzy flag skipped leaf PreParse: context=%#v", ctx)
	}
	if err := root.Execute(); err != nil {
		t.Fatalf("corrected group-middle persistent flag failed: %v", err)
	}
}

func TestCrossPlatformCoverageProtectedFlagChildNameValueStaysOnOwningCommand(t *testing.T) {
	for _, args := range [][]string{
		{"aisearch", "--types", "enterprise"},
		{"aisearch", "--types", "false", "enterprise"},
		{"aisearch", "--types=false", "enterprise"},
	} {
		root := NewSchemaSourceRootCommand()
		ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
		if err != nil {
			t.Fatalf("RunPreParseArgs(%v) error = %v", args, err)
		}
		if ctx == nil || ctx.Command != "dws aisearch" || !ctx.IsFlagProtected("types") || !slices.Equal(ctx.Args, args) {
			t.Fatalf("protected child-name value selected wrong command: args=%v context=%#v", args, ctx)
		}
	}
}

func TestCrossPlatformCoveragePreParseConflictHonorsErrorPresentationFlags(t *testing.T) {
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

func TestCrossPlatformCoverageCommandResolutionPrecedesFlagErrorsOnProductionTree(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantReason  string
		wantCommand string
		wantHint    string
	}{
		{
			name:        "unknown shortcut",
			args:        []string{"chat", "+chat-mesages", "--keyword", "x", "--format", "json"},
			wantReason:  "unknown_shortcut",
			wantCommand: "dws chat",
		},
		{
			name:        "unknown dev app subcommand",
			args:        []string{"dev", "app", "search", "--keyword", "x", "--format", "json"},
			wantReason:  "unknown_subcommand",
			wantCommand: "dws dev app",
		},
		{
			name:        "unknown aisearch subcommand before protected flag",
			args:        []string{"aisearch", "--query", "Alice", "enterprize", "--types", "enterprise", "--format", "json"},
			wantReason:  "unknown_subcommand",
			wantCommand: "dws aisearch",
			wantHint:    "dws aisearch enterprise",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := NewSchemaSourceRootCommand()
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), test.args)
			if ctx == nil || ctx.Command != test.wantCommand {
				t.Fatalf("Context = %#v, want command %q", ctx, test.wantCommand)
			}
			err = newPreParseValidationError(err)
			var structured *apperrors.Error
			if !stderrors.As(err, &structured) {
				t.Fatalf("error = %T %v", err, err)
			}
			if structured.Reason != test.wantReason || structured.ExitCode() != 3 {
				t.Fatalf("structured error = %#v", structured)
			}
			if test.wantHint != "" && !strings.Contains(structured.Hint, test.wantHint) {
				t.Fatalf("hint = %q, want %q", structured.Hint, test.wantHint)
			}
			if len(structured.AvailableFlags) != 0 || strings.Contains(structured.Message, "unknown flag") {
				t.Fatalf("command error leaked flag classification: %#v", structured)
			}
		})
	}
}

func TestCrossPlatformCoverageResolvedShortcutStillUsesExactLeafFlagError(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	args := []string{"chat", "+chat-messages", "--keyword", "x", "--format", "json"}
	if ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
		t.Fatalf("valid shortcut path failed PreParse: %#v, %v", ctx, err)
	}
	root.SetArgs(args)
	err := root.Execute()
	var structured *apperrors.Error
	if !stderrors.As(err, &structured) {
		t.Fatalf("error = %T %v", err, err)
	}
	if structured.Reason != "unknown_flag" || !strings.Contains(structured.Message, "dws chat +chat-messages --help") {
		t.Fatalf("resolved shortcut flag error = %#v", structured)
	}
}
