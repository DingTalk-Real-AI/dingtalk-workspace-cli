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

package corecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newInputCommand builds a Spec whose Invoke captures toolArgs, so tests can
// assert what the full pipeline (resolution → validation → BuildArgs) ships.
func newInputCommand(flags []FlagSpec, captured *map[string]any) *cobra.Command {
	return New(Spec{
		Use:   "t",
		Short: "t",
		Flags: flags,
		Invoke: func(c *Ctx, toolArgs map[string]any) error {
			*captured = toolArgs
			return nil
		},
	})
}

func runInputCommand(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func writeInputFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestCrossPlatformCoverageResolveInputFlagsFile(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile, InputStdin}},
	}, &got)
	path := writeInputFile(t, "# hello\nworld")

	if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "# hello\nworld" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

func TestCrossPlatformCoverageResolveInputFlagsStdin(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputStdin}},
	}, &got)
	cmd.SetIn(strings.NewReader("piped payload"))

	if err := runInputCommand(t, cmd, "--content", "-"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "piped payload" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

// The @@ escape keeps a literal leading @ inline and must not be treated as a
// source reference even when Input is declared.
func TestCrossPlatformCoverageResolveInputFlagsEscapedAtStaysInline(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile, InputStdin}},
	}, &got)

	if err := runInputCommand(t, cmd, "--content", "@@literal@x"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "@literal@x" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

func TestCrossPlatformCoverageResolveInputFlagsBOMStripped(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "\ufeff{\"a\":1}")

	if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "{\"a\":1}" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

// Required must be satisfied by the resolved payload, proving resolution runs
// before the required stage.
func TestCrossPlatformCoverageResolveInputFlagsSatisfiesRequired(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Required: true, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "payload")

	if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "payload" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

// Enum validation sees the resolved content, not the "@path" token.
func TestCrossPlatformCoverageResolveInputFlagsEnumValidatesResolvedContent(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "mode", Usage: "M", Bind: "mode", Enum: []string{"asc", "desc"}, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "sideways")

	err := runInputCommand(t, cmd, "--mode", "@"+path)
	if err == nil || !strings.Contains(err.Error(), "不合法") {
		t.Fatalf("expected enum rejection on resolved content, got %v", err)
	}
}

// A value passed through a declared alias is resolved exactly like the main
// name (fallback-chain parity).
func TestCrossPlatformCoverageResolveInputFlagsAliasResolved(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Aliases: []string{"body"}, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "via alias")

	if err := runInputCommand(t, cmd, "--body", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "via alias" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

// When the main flag shadows a changed alias (rawValue usability order), the
// shadowed alias must not be input-resolved: resolution targets exactly the
// name the fallback chain will read. The whitespace main value is usable for
// a non-Trim flag, and the alias path does not exist on purpose — a resolver
// that wrongly picked the alias would fail the read instead of shipping "   ".
func TestCrossPlatformCoverageResolveInputFlagsShadowedAliasNotResolved(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Aliases: []string{"body"}, Input: []string{InputFile}},
	}, &got)
	missing := filepath.Join(t.TempDir(), "missing.txt")

	if err := runInputCommand(t, cmd, "--content", "   ", "--body", "@"+missing); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "   " {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

func TestCrossPlatformCoverageResolveInputFlagsFileNotSupported(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputStdin}},
	}, &got)

	err := runInputCommand(t, cmd, "--content", "@/tmp/whatever.txt")
	if err == nil || !strings.Contains(err.Error(), "不支持文件输入") {
		t.Fatalf("expected file-input rejection, got %v", err)
	}
}

func TestCrossPlatformCoverageResolveInputFlagsStdinNotSupported(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}},
	}, &got)
	cmd.SetIn(strings.NewReader("x"))

	err := runInputCommand(t, cmd, "--content", "-")
	if err == nil || !strings.Contains(err.Error(), "不支持 stdin") {
		t.Fatalf("expected stdin rejection, got %v", err)
	}
}

func TestCrossPlatformCoverageResolveInputFlagsSingleStdinConsumer(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "first", Usage: "F", Bind: "first", Input: []string{InputStdin}},
		{Name: "second", Usage: "S", Bind: "second", Input: []string{InputStdin}},
	}, &got)
	cmd.SetIn(strings.NewReader("x"))

	err := runInputCommand(t, cmd, "--first", "-", "--second", "-")
	if err == nil || !strings.Contains(err.Error(), "只能被一个参数使用") {
		t.Fatalf("expected single-stdin rejection, got %v", err)
	}
}

// failingReader (corecmd_test.go) fails every read, making the stdin error
// branch reachable.
func TestCrossPlatformCoverageResolveInputFlagsStdinReadFailure(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputStdin}},
	}, &got)
	cmd.SetIn(failingReader{})

	err := runInputCommand(t, cmd, "--content", "-")
	if err == nil || !strings.Contains(err.Error(), "读取 stdin 失败") {
		t.Fatalf("expected stdin read failure, got %v", err)
	}
}

func TestCrossPlatformCoverageResolveInputFlagsFileNotFound(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile, InputStdin}},
	}, &got)

	err := runInputCommand(t, cmd, "--content", "@"+filepath.Join(t.TempDir(), "missing.txt"))
	if err == nil || !strings.Contains(err.Error(), "读取文件") {
		t.Fatalf("expected read failure, got %v", err)
	}
}

func TestCrossPlatformCoverageResolveInputFlagsEmptyPathAfterAt(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}},
	}, &got)

	err := runInputCommand(t, cmd, "--content", "@   ")
	if err == nil || !strings.Contains(err.Error(), "文件路径不能为空") {
		t.Fatalf("expected empty-path rejection, got %v", err)
	}
}

// A flag without Input keeps its literal value even when it looks like a
// source reference — resolution is strictly opt-in per declaration.
func TestCrossPlatformCoverageResolveInputFlagsNoInputSpecPassthrough(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "token", Usage: "T", Bind: "token"},
	}, &got)

	if err := runInputCommand(t, cmd, "--token", "@not-a-file"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["token"] != "@not-a-file" {
		t.Fatalf("toolArgs[token] = %q", got["token"])
	}
}

// Registration defaults and env fallback are never input-resolved: @file only
// applies to what the user literally typed on the command line.
func TestCrossPlatformCoverageResolveInputFlagsDefaultNotResolved(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Default: "@not-a-file", Input: []string{InputFile}},
	}, &got)

	if err := runInputCommand(t, cmd); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "@not-a-file" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

// Trim flags judge usability on the trimmed value (rawValue), so a leading
// whitespace before @ must still resolve instead of shipping as a literal.
func TestCrossPlatformCoverageResolveInputFlagsTrimmedLeadingSpace(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Trim: true, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "trimmed payload")

	if err := runInputCommand(t, cmd, "--content", " @"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "trimmed payload" {
		t.Fatalf("toolArgs[content] = %q", got["content"])
	}
}

func TestCrossPlatformCoverageValidateInputSpecsPanics(t *testing.T) {
	cases := []struct {
		name string
		flag FlagSpec
	}{
		{"non-string kind", FlagSpec{Name: "n", Kind: KindInt, Input: []string{InputFile}}},
		{"unknown source", FlagSpec{Name: "s", Input: []string{"url"}}},
		{"duplicate source", FlagSpec{Name: "s", Input: []string{InputFile, InputFile}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			New(Spec{
				Use:    "t",
				Short:  "t",
				Flags:  []FlagSpec{tc.flag},
				Invoke: func(c *Ctx, toolArgs map[string]any) error { return nil },
			})
		})
	}
}
