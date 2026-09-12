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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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

// A payload loaded from a source is authoritative: an empty file must reach
// toolArgs verbatim rather than being replaced by the declared Default, because
// the user pointed at that file explicitly (e.g. clearing a field).
func TestCrossPlatformCoverageResolveInputFlagsEmptyPayloadIsAuthoritative(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Default: "FALLBACK", Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "")

	if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "" {
		t.Fatalf("toolArgs[content] = %q, want the empty payload", got["content"])
	}
}

// Same authority through a declared alias.
func TestCrossPlatformCoverageResolveInputFlagsEmptyPayloadViaAlias(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Default: "FALLBACK",
			Aliases: []string{"body"}, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "")

	if err := runInputCommand(t, cmd, "--body", "@"+path); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "" {
		t.Fatalf("toolArgs[content] = %q, want the empty payload", got["content"])
	}
}

// The resolved-source bit is true only for genuinely source-loaded values, so a
// domain guard can skip shape heuristics without re-rejecting correct @file use.
func TestCrossPlatformCoverageInputResolvedFromSourceReportsProvenance(t *testing.T) {
	path := writeInputFile(t, "payload")
	cases := []struct {
		name     string
		value    string
		resolved bool
	}{
		{"file", "@" + path, true},
		{"inline", "plain", false},
		{"escaped", "@@plain", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen, seenViaCtx bool
			cmd := New(Spec{
				Use: "t", Short: "t",
				Flags: []FlagSpec{{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}}},
				Invoke: func(c *Ctx, toolArgs map[string]any) error {
					seen = InputResolvedFromSource(c.Command(), "content")
					seenViaCtx = c.InputResolvedFromSource("content")
					return nil
				},
			})
			if err := runInputCommand(t, cmd, "--content", tc.value); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if seen != tc.resolved || seenViaCtx != tc.resolved {
				t.Fatalf("resolved = %v (ctx %v), want %v", seen, seenViaCtx, tc.resolved)
			}
		})
	}
	if InputResolvedFromSource(nil, "content") {
		t.Fatal("nil command must not report a resolved flag")
	}
}

// A command object can be executed more than once (a test or wrapper reusing
// it), so a marker from an earlier @file call must not make a later inline
// value authoritative.
func TestCrossPlatformCoverageInputResolvedMarkerIsPerInvocation(t *testing.T) {
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", Default: "FALLBACK",
			Aliases: []string{"body"}, Input: []string{InputFile}},
	}, &got)
	path := writeInputFile(t, "")

	if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if got["content"] != "" {
		t.Fatalf("first invocation toolArgs[content] = %q", got["content"])
	}

	// Reuse the same command with an inline empty value: no source was read, so
	// the declared Default applies again.
	if err := runInputCommand(t, cmd, "--content", ""); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if got["content"] != "FALLBACK" {
		t.Fatalf("second invocation toolArgs[content] = %q, want the Default", got["content"])
	}
	if InputResolvedFromSource(cmd, "content") {
		t.Fatal("marker leaked into the second invocation")
	}
}

// The authoritative rule must hold at constraint provision too: pointing at a
// file is supplying the flag, so at_least_one is satisfied even by an empty
// payload, while an inline empty value still fails it.
func TestCrossPlatformCoverageInputResolvedSatisfiesConstraint(t *testing.T) {
	newCmd := func() *cobra.Command {
		return New(Spec{
			Use: "t", Short: "t",
			Flags: []FlagSpec{
				{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}},
				{Name: "url", Usage: "U", Bind: "url"},
			},
			Constraints: []Constraint{{Kind: AtLeastOne, Flags: []string{"content", "url"}}},
			Invoke:      func(c *Ctx, toolArgs map[string]any) error { return nil },
		})
	}
	path := writeInputFile(t, "")

	if err := runInputCommand(t, newCmd(), "--content", "@"+path); err != nil {
		t.Fatalf("empty payload must satisfy at_least_one, got %v", err)
	}
	err := runInputCommand(t, newCmd(), "--content", "")
	if err == nil || !strings.Contains(err.Error(), "请至少指定") {
		t.Fatalf("inline empty must still fail at_least_one, got %v", err)
	}
}

// ArgDefault must not re-substitute an authoritative payload one stage after
// rawValue returned it verbatim.
func TestCrossPlatformCoverageInputResolvedBeatsArgDefault(t *testing.T) {
	path := writeInputFile(t, "")
	cases := []struct{ name, token string }{
		{"main", "--content"},
		{"alias", "--body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			cmd := newInputCommand([]FlagSpec{
				{Name: "content", Usage: "C", Bind: "content", ArgDefault: "ARGDEF",
					Aliases: []string{"body"}, Input: []string{InputFile}},
			}, &got)

			if err := runInputCommand(t, cmd, tc.token, "@"+path); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if got["content"] != "" {
				t.Fatalf("toolArgs[content] = %q, want the empty payload", got["content"])
			}
		})
	}

	// Without a source, ArgDefault still applies.
	var got map[string]any
	cmd := newInputCommand([]FlagSpec{
		{Name: "content", Usage: "C", Bind: "content", ArgDefault: "ARGDEF", Input: []string{InputFile}},
	}, &got)
	if err := runInputCommand(t, cmd, "--content", ""); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got["content"] != "ARGDEF" {
		t.Fatalf("toolArgs[content] = %q, want ArgDefault", got["content"])
	}
}

// The corecmd path delivers a payload byte-exact when no Trim is declared, and
// a declared Transform collapses an empty payload so its key is skipped — the
// documented author-declared exception to "an empty payload reaches the args".
func TestCrossPlatformCoverageInputPayloadBoundaries(t *testing.T) {
	t.Run("verbatim", func(t *testing.T) {
		var got map[string]any
		cmd := newInputCommand([]FlagSpec{
			{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile}},
		}, &got)
		path := writeInputFile(t, "  body  \n")

		if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got["content"] != "  body  \n" {
			t.Fatalf("toolArgs[content] = %q, want the payload byte-exact", got["content"])
		}
	})

	t.Run("transform drops empty payload", func(t *testing.T) {
		var got map[string]any
		cmd := newInputCommand([]FlagSpec{
			{Name: "content", Usage: "C", Bind: "content", Input: []string{InputFile},
				Transform: func(raw string) (any, error) { return raw, nil }},
		}, &got)
		path := writeInputFile(t, "")

		if err := runInputCommand(t, cmd, "--content", "@"+path); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if _, present := got["content"]; present {
			t.Fatalf("toolArgs kept a collapsed Transform result: %#v", got)
		}
	})
}

// A user_required write declaring InputStdin must fail closed without --yes:
// resolution consumed stdin, so the confirmation prompt sees EOF. Pinned
// because this is a safety guarantee, not merely a convenience.
func TestCrossPlatformCoverageInputStdinWriteFailsClosedWithoutYes(t *testing.T) {
	newWriteCmd := func(executed *bool, got *map[string]any) *cobra.Command {
		cmd := New(Spec{
			Use: "t", Short: "t",
			Safety: contract.SafetySpec{Effect: "write", Risk: "medium",
				Confirmation: "user_required", Idempotency: "unknown"},
			Flags: []FlagSpec{{Name: "content", Usage: "C", Bind: "content", Input: []string{InputStdin}}},
			Invoke: func(c *Ctx, toolArgs map[string]any) error {
				*executed = true
				*got = toolArgs
				return nil
			},
		})
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.PersistentFlags().Bool("dry-run", false, "")
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetIn(strings.NewReader("payload body"))
		return cmd
	}

	var executed bool
	var got map[string]any
	err := runInputCommand(t, newWriteCmd(&executed, &got), "--content", "-")
	if executed {
		t.Fatal("write executed without confirmation")
	}
	if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("expected confirmation_required, got %v", err)
	}

	executed, got = false, nil
	if err := runInputCommand(t, newWriteCmd(&executed, &got), "--content", "-", "--yes"); err != nil {
		t.Fatalf("execute with --yes: %v", err)
	}
	if !executed || got["content"] != "payload body" {
		t.Fatalf("executed=%v toolArgs=%#v", executed, got)
	}
}

// ConfirmFirst confirms before resolution, so the prompt would read the piped
// payload as its answer and a payload starting with "yes" would authorize the
// operation. The combination must be unrepresentable, not merely discouraged.
func TestCrossPlatformCoverageConfirmFirstWithInputStdinPanics(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		if msg, ok := recovered.(string); !ok || !strings.Contains(msg, "ConfirmFirst cannot be combined") {
			t.Fatalf("unexpected panic %v", recovered)
		}
	}()
	New(Spec{
		Use: "t", Short: "t",
		Safety: contract.SafetySpec{Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown"},
		ConfirmFirst: true,
		Flags:        []FlagSpec{{Name: "content", Usage: "C", Bind: "content", Input: []string{InputStdin}}},
		Invoke:       func(c *Ctx, toolArgs map[string]any) error { return nil },
	})
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
