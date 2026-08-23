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
	"errors"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// command is a shared base package: it must be fully covered by its own direct
// tests rather than relying on cross-package coverpkg from its consumers, so the
// per-package coverage CI job (which runs without -coverpkg) sees it covered.

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "t"}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

func testReadSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func testWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func testDestructiveSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

// ── flag registration ──────────────────────────────────────────────

func TestCrossPlatformCoverageRegisterFlagsAllKinds(t *testing.T) {
	cmd := newTestCommand()
	RegisterFlags(cmd, []FlagSpec{
		{Name: "s", Shorthand: "s", Usage: "S", Default: "d"},
		{Name: "i", Shorthand: "i", Usage: "I", Kind: KindInt, Aliases: []string{"i-alias"}},
		{Name: "b", Shorthand: "b", Usage: "B", Kind: KindBool},
		{Name: "sl", Shorthand: "l", Usage: "SL", Kind: KindStringSlice, Default: "a,b", Aliases: []string{"sl-alias"}},
		{Name: "req", Usage: "R", MarkRequired: true},
		{Name: "hidden", Usage: "H", Hidden: true},
	})

	if f := cmd.Flags().Lookup("s"); f == nil || f.DefValue != "d" || f.Usage != "S" {
		t.Fatalf("string flag = %#v", f)
	}
	for shorthand, name := range map[string]string{"s": "s", "i": "i", "b": "b", "l": "sl"} {
		if flag := cmd.Flags().ShorthandLookup(shorthand); flag == nil || flag.Name != name {
			t.Fatalf("shorthand -%s = %#v, want --%s", shorthand, flag, name)
		}
	}
	for name, wantType := range map[string]string{"i": "int", "b": "bool", "sl": "stringSlice"} {
		f := cmd.Flags().Lookup(name)
		if f == nil || f.Value.Type() != wantType {
			t.Fatalf("flag %q type = %v, want %s", name, f, wantType)
		}
	}
	// Aliases are registered with the main kind and hidden.
	for alias, canonical := range map[string]string{"i-alias": "i", "sl-alias": "sl"} {
		f := cmd.Flags().Lookup(alias)
		if f == nil || !f.Hidden {
			t.Fatalf("alias %q = %#v, want registered+hidden", alias, f)
		}
		if got := f.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != canonical {
			t.Fatalf("alias %q annotation = %#v, want alias_of %q", alias, got, canonical)
		}
		if got := f.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
			t.Fatalf("alias %q origin = %#v, want corecmd FlagSpec marker", alias, got)
		}
	}
	if cmd.Flags().Lookup("i-alias").Value.Type() != "int" {
		t.Fatal("int alias must be registered as int")
	}
	if got, _ := cmd.Flags().GetStringSlice("sl"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("string-slice default = %#v, want [a b]", got)
	}
	if hidden := cmd.Flags().Lookup("hidden"); hidden == nil || !hidden.Hidden {
		t.Fatalf("real hidden flag = %#v", hidden)
	}
	if ann := cmd.Flags().Lookup("req").Annotations[cobra.BashCompOneRequiredFlag]; len(ann) == 0 {
		t.Fatal("MarkRequired did not reach cobra")
	}
}

func TestCrossPlatformCoverageAnnotateFlagAliasIgnoresMissingInputs(t *testing.T) {
	AnnotateFlagAlias(nil, "alias", "canonical")

	cmd := newTestCommand()
	AnnotateFlagAlias(cmd, "missing", "canonical")
	if flag := cmd.Flags().Lookup("missing"); flag != nil {
		t.Fatalf("unexpected missing flag registered: %#v", flag)
	}
}

// ── effective value fallback chain ─────────────────────────────────

func TestCrossPlatformCoverageEffectiveValueFallbackChain(t *testing.T) {
	spec := FlagSpec{Name: "v", Usage: "V", Default: "reg-default", Aliases: []string{"v-alias"}, EnvVar: "DWS_CMDCORE_V"}

	// registration default is the chain tail
	cmd := newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})
	if got := EffectiveValue(cmd, spec); got != "reg-default" {
		t.Fatalf("default tail = %q", got)
	}

	// env beats the registration default
	t.Setenv("DWS_CMDCORE_V", "from-env")
	if got := EffectiveValue(cmd, spec); got != "from-env" {
		t.Fatalf("env = %q", got)
	}

	// explicit alias beats env
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})
	_ = cmd.Flags().Set("v-alias", "from-alias")
	if got := EffectiveValue(cmd, spec); got != "from-alias" {
		t.Fatalf("alias = %q", got)
	}

	// explicit main flag wins
	_ = cmd.Flags().Set("v", "explicit")
	if got := EffectiveValue(cmd, spec); got != "explicit" {
		t.Fatalf("explicit = %q", got)
	}
}

func TestCrossPlatformCoverageEffectiveValueTrimAndEmptySkips(t *testing.T) {
	// Trim makes whitespace-only fall through to the next fallback level.
	trimmed := FlagSpec{Name: "v", Usage: "V", Trim: true, Aliases: []string{"v-alias"}, Default: "tail"}
	cmd := newTestCommand()
	RegisterFlags(cmd, []FlagSpec{trimmed})
	_ = cmd.Flags().Set("v", "   ")
	_ = cmd.Flags().Set("v-alias", "  kept  ")
	if got := EffectiveValue(cmd, trimmed); got != "kept" {
		t.Fatalf("trim fallthrough = %q, want trimmed alias", got)
	}

	// Empty explicit value also falls through (usable() rejects "").
	plain := FlagSpec{Name: "p", Usage: "P", Default: "tail"}
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{plain})
	_ = cmd.Flags().Set("p", "")
	if got := EffectiveValue(cmd, plain); got != "tail" {
		t.Fatalf("empty explicit = %q, want tail", got)
	}

	// Empty alias is skipped too.
	aliased := FlagSpec{Name: "a", Usage: "A", Aliases: []string{"a-alias"}, Default: "tail"}
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{aliased})
	_ = cmd.Flags().Set("a-alias", "")
	if got := EffectiveValue(cmd, aliased); got != "tail" {
		t.Fatalf("empty alias = %q, want tail", got)
	}

	// Empty env is skipped.
	env := FlagSpec{Name: "e", Usage: "E", EnvVar: "DWS_CMDCORE_EMPTY", Default: "tail"}
	t.Setenv("DWS_CMDCORE_EMPTY", "")
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{env})
	if got := EffectiveValue(cmd, env); got != "tail" {
		t.Fatalf("empty env = %q, want tail", got)
	}
}

func TestCrossPlatformCoverageIntegerValue(t *testing.T) {
	spec := FlagSpec{Name: "n", Usage: "N", Kind: KindInt, EnvVar: "DWS_CMDCORE_N"}
	cmd := newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})

	// unset int formats to "0" → treated as empty → 0, no error
	if v, err := integerValue(cmd, spec); v != 0 || err != nil {
		t.Fatalf("unset int = %d, %v", v, err)
	}
	_ = cmd.Flags().Set("n", "7")
	if v, err := integerValue(cmd, spec); v != 7 || err != nil {
		t.Fatalf("explicit int = %d, %v", v, err)
	}

	// unparseable env value must error rather than silently drop
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})
	t.Setenv("DWS_CMDCORE_N", "abc")
	if _, err := integerValue(cmd, spec); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("unparseable env err = %v", err)
	}
}

func TestCrossPlatformCoverageSliceValue(t *testing.T) {
	spec := FlagSpec{Name: "ids", Usage: "IDS", Kind: KindStringSlice, Aliases: []string{"ids-alias"}}
	cmd := newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})

	if got := sliceValue(cmd, spec); got != nil {
		t.Fatalf("unset slice = %#v, want nil", got)
	}
	_ = cmd.Flags().Set("ids", " a , , b ")
	if got := sliceValue(cmd, spec); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("trimmed slice = %#v", got)
	}

	// all-empty elements count as not provided
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{spec})
	_ = cmd.Flags().Set("ids", " , ")
	if got := sliceValue(cmd, spec); got != nil {
		t.Fatalf("blank slice = %#v, want nil", got)
	}
	// alias supplies the value when the main flag is blank
	_ = cmd.Flags().Set("ids-alias", "x")
	if got := sliceValue(cmd, spec); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("alias slice = %#v", got)
	}
}

func TestCrossPlatformCoverageHasEffectiveValueAllKinds(t *testing.T) {
	cmd := newTestCommand()
	flags := []FlagSpec{
		{Name: "s", Usage: "S"},
		{Name: "i", Usage: "I", Kind: KindInt},
		{Name: "b", Usage: "B", Kind: KindBool},
		{Name: "sl", Usage: "SL", Kind: KindStringSlice},
		{Name: "bad", Usage: "BAD", Kind: KindInt, EnvVar: "DWS_CMDCORE_BAD"},
	}
	RegisterFlags(cmd, flags)

	for _, f := range flags[:4] {
		if hasEffectiveValue(cmd, f) {
			t.Fatalf("unset %q reported as provided", f.Name)
		}
	}
	_ = cmd.Flags().Set("s", "v")
	_ = cmd.Flags().Set("i", "3")
	_ = cmd.Flags().Set("b", "false") // bool: Changed counts even for false
	_ = cmd.Flags().Set("sl", "a")
	for _, f := range flags[:4] {
		if !hasEffectiveValue(cmd, f) {
			t.Fatalf("set %q reported as missing", f.Name)
		}
	}
	// explicit int 0 is NOT provided (non-zero only)
	zero := newTestCommand()
	RegisterFlags(zero, flags)
	_ = zero.Flags().Set("i", "0")
	if hasEffectiveValue(zero, flags[1]) {
		t.Fatal("explicit int 0 must count as missing")
	}
	// unparseable int counts as provided so BuildArgs reports the precise error
	t.Setenv("DWS_CMDCORE_BAD", "nope")
	if !hasEffectiveValue(cmd, flags[4]) {
		t.Fatal("unparseable int must count as provided")
	}
}

// ── required validation ────────────────────────────────────────────

func TestCrossPlatformCoverageValidateRequired(t *testing.T) {
	plain := []FlagSpec{{Name: "content", Usage: "C", Required: true}}
	cmd := newTestCommand()
	RegisterFlags(cmd, plain)
	if err := ValidateRequired(cmd, plain); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("missing plain required err = %v", err)
	}
	_ = cmd.Flags().Set("content", "x")
	if err := ValidateRequired(cmd, plain); err != nil {
		t.Fatalf("satisfied plain required err = %v", err)
	}

	// alias satisfies required
	aliased := []FlagSpec{{Name: "node", Usage: "N", Required: true, Aliases: []string{"id"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, aliased)
	_ = cmd.Flags().Set("id", "n1")
	if err := ValidateRequired(cmd, aliased); err != nil {
		t.Fatalf("alias-satisfied required err = %v", err)
	}

	// env-backed required with explicit hint
	hinted := []FlagSpec{{Name: "token", Usage: "T", Required: true, EnvVar: "DWS_CMDCORE_TOKEN", RequiredHint: "flag --token is required (or set DWS_CMDCORE_TOKEN)"}}
	cmd = newTestCommand()
	RegisterFlags(cmd, hinted)
	if err := ValidateRequired(cmd, hinted); err == nil || !strings.Contains(err.Error(), "DWS_CMDCORE_TOKEN") {
		t.Fatalf("hint err = %v", err)
	}
	t.Setenv("DWS_CMDCORE_TOKEN", "t")
	if err := ValidateRequired(cmd, hinted); err != nil {
		t.Fatalf("env-satisfied required err = %v", err)
	}

	// env-backed required WITHOUT hint falls back to the default wording
	noHint := []FlagSpec{{Name: "key", Usage: "K", Required: true, EnvVar: "DWS_CMDCORE_ABSENT"}}
	cmd = newTestCommand()
	RegisterFlags(cmd, noHint)
	if err := ValidateRequired(cmd, noHint); err == nil || !strings.Contains(err.Error(), "flag --key is required") {
		t.Fatalf("default hint err = %v", err)
	}

	// non-required flags are ignored
	optional := []FlagSpec{{Name: "opt", Usage: "O"}}
	cmd = newTestCommand()
	RegisterFlags(cmd, optional)
	if err := ValidateRequired(cmd, optional); err != nil {
		t.Fatalf("optional err = %v", err)
	}

	// Shortcut compatibility requires the token itself even when a default is
	// registered, and preserves the authored error.
	changed := []FlagSpec{{
		Name: "explicit", Usage: "E", Default: "fallback", Required: true,
		ValidationMode: ValidationShortcut, RequiredError: "缺少必填参数 --explicit：显式参数",
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, changed)
	if err := ValidateRequired(cmd, changed); err == nil ||
		err.Error() != "缺少必填参数 --explicit：显式参数" {
		t.Fatalf("ValidationShortcut missing error = %v", err)
	}
	_ = cmd.Flags().Set("explicit", "fallback")
	if err := ValidateRequired(cmd, changed); err != nil {
		t.Fatalf("ValidationShortcut explicit value failed: %v", err)
	}
}

func TestCrossPlatformCoverageValidateEnums(t *testing.T) {
	flags := []FlagSpec{
		{Name: "mode", Usage: "M", Enum: []string{"a", "b"}},
		{Name: "items", Usage: "I", Kind: KindStringSlice, Enum: []string{"x", "y"}},
	}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("mode", "bad")
	if err := ValidateEnums(cmd, flags); err == nil ||
		!strings.Contains(err.Error(), `参数 --mode 取值 "bad" 不合法`) {
		t.Fatalf("invalid scalar enum error = %v", err)
	}
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("mode", "a")
	_ = cmd.Flags().Set("items", "x,bad")
	if err := ValidateEnums(cmd, flags); err == nil ||
		!strings.Contains(err.Error(), `参数 --items 取值 "bad" 不合法`) {
		t.Fatalf("invalid slice enum error = %v", err)
	}

	// ValidationShortcut enums are enforced by ValidateRequired, not ValidateEnums.
	shortcut := []FlagSpec{{Name: "mode", Usage: "M", ValidationMode: ValidationShortcut, Enum: []string{"a", "b"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, shortcut)
	_ = cmd.Flags().Set("mode", "bad")
	if err := ValidateEnums(cmd, shortcut); err != nil {
		t.Fatalf("ValidateEnums must skip ValidationShortcut, got %v", err)
	}
}

// ── toolArgs assembly ──────────────────────────────────────────────

func TestCrossPlatformCoverageBuildArgsAllKinds(t *testing.T) {
	flags := []FlagSpec{
		{Name: "name", Usage: "N", Bind: "userName"},
		{Name: "plain", Usage: "P"},
		{Name: "n", Usage: "NUM", Kind: KindInt, Bind: "count"},
		{Name: "b", Usage: "B", Kind: KindBool, Bind: "flagOn"},
		{Name: "ids", Usage: "IDS", Kind: KindStringSlice, Bind: "idList"},
		{Name: "scope", Usage: "S", ArgDefault: "ALL"},
		{Name: "note", Usage: "NOTE", OmitEmpty: true},
	}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("name", "alice")
	_ = cmd.Flags().Set("n", "5")
	_ = cmd.Flags().Set("b", "false")
	_ = cmd.Flags().Set("ids", " a , b ")

	args, err := BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["userName"] != "alice" || args["count"] != 5 || args["flagOn"] != false {
		t.Fatalf("args = %#v", args)
	}
	if !reflect.DeepEqual(args["idList"], []string{"a", "b"}) {
		t.Fatalf("idList = %#v", args["idList"])
	}
	if args["scope"] != "ALL" {
		t.Fatalf("ArgDefault not applied: %#v", args["scope"])
	}
	if _, ok := args["note"]; ok {
		t.Fatalf("OmitEmpty flag leaked: %#v", args)
	}
	// bind defaults to the flag name; empty non-OmitEmpty string still enters
	if v, ok := args["plain"]; !ok || v != "" {
		t.Fatalf("plain = %#v (want present empty string)", args["plain"])
	}
	// unset int / unset bool / unset slice stay out
	for _, key := range []string{"count2", "flagOff", "idNone"} {
		if _, ok := args[key]; ok {
			t.Fatalf("unexpected key %q", key)
		}
	}
}

func TestCrossPlatformCoverageBuildArgsIntZeroBoolUnsetSliceEmpty(t *testing.T) {
	flags := []FlagSpec{
		{Name: "n", Usage: "N", Kind: KindInt},
		{Name: "b", Usage: "B", Kind: KindBool},
		{Name: "sl", Usage: "SL", Kind: KindStringSlice},
	}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("n", "0")  // non-zero only → skipped
	_ = cmd.Flags().Set("sl", " ") // blank elements → skipped
	args, err := BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want empty", args)
	}
}

func TestCrossPlatformCoverageBuildArgsTransformAndErrors(t *testing.T) {
	// Transform result is bound
	ok := []FlagSpec{{Name: "csv", Usage: "C", Bind: "list", Transform: func(raw string) (any, error) {
		return strings.Split(raw, ","), nil
	}}}
	cmd := newTestCommand()
	RegisterFlags(cmd, ok)
	_ = cmd.Flags().Set("csv", "a,b")
	args, err := BuildArgs(cmd, ok)
	if err != nil || !reflect.DeepEqual(args["list"], []string{"a", "b"}) {
		t.Fatalf("transform args = %#v err = %v", args, err)
	}

	// Transform returning (nil, nil) skips the key
	skip := []FlagSpec{{Name: "x", Usage: "X", Transform: func(string) (any, error) { return nil, nil }}}
	cmd = newTestCommand()
	RegisterFlags(cmd, skip)
	_ = cmd.Flags().Set("x", "v")
	args, err = BuildArgs(cmd, skip)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := args["x"]; present {
		t.Fatalf("nil transform should skip key: %#v", args)
	}

	// Required + transform-to-empty must fail (separator-only lists)
	requiredEmpty := []FlagSpec{{
		Name: "codes", Usage: "C", Required: true, RequiredHint: "--codes 为必填",
		Transform: func(string) (any, error) { return nil, nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, requiredEmpty)
	_ = cmd.Flags().Set("codes", ",")
	if _, err := BuildArgs(cmd, requiredEmpty); err == nil || !strings.Contains(err.Error(), "--codes 为必填") {
		t.Fatalf("required empty transform err = %v", err)
	}
	requiredEmptySlice := []FlagSpec{{
		Name: "codes", Usage: "C", Required: true,
		Transform: func(string) (any, error) { return []string{}, nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, requiredEmptySlice)
	_ = cmd.Flags().Set("codes", ";")
	if _, err := BuildArgs(cmd, requiredEmptySlice); err == nil || !strings.Contains(err.Error(), "必填参数 --codes 不能为空") {
		t.Fatalf("required empty-slice transform err = %v", err)
	}
	requiredEmptyString := []FlagSpec{{
		Name: "codes", Usage: "C", Required: true,
		Transform: func(string) (any, error) { return "  ", nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, requiredEmptyString)
	_ = cmd.Flags().Set("codes", ",")
	if _, err := BuildArgs(cmd, requiredEmptyString); err == nil || !strings.Contains(err.Error(), "必填参数 --codes 不能为空") {
		t.Fatalf("required empty-string transform err = %v", err)
	}
	requiredEmptyAnySlice := []FlagSpec{{
		Name: "codes", Usage: "C", Required: true,
		Transform: func(string) (any, error) { return []any{}, nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, requiredEmptyAnySlice)
	_ = cmd.Flags().Set("codes", ",")
	if _, err := BuildArgs(cmd, requiredEmptyAnySlice); err == nil || !strings.Contains(err.Error(), "必填参数 --codes 不能为空") {
		t.Fatalf("required empty []any transform err = %v", err)
	}
	requiredErrorHint := []FlagSpec{{
		Name: "codes", Usage: "C", Required: true, RequiredError: "codes missing after transform",
		Transform: func(string) (any, error) { return nil, nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, requiredErrorHint)
	_ = cmd.Flags().Set("codes", ",")
	if _, err := BuildArgs(cmd, requiredErrorHint); err == nil || !strings.Contains(err.Error(), "codes missing after transform") {
		t.Fatalf("required RequiredError transform err = %v", err)
	}
	// Non-empty non-list transform result is kept (covers emptyTransformResult default).
	keepScalar := []FlagSpec{{
		Name: "n", Usage: "N", Required: true, Bind: "count",
		Transform: func(string) (any, error) { return 3, nil },
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, keepScalar)
	_ = cmd.Flags().Set("n", "x")
	args, err = BuildArgs(cmd, keepScalar)
	if err != nil || args["count"] != 3 {
		t.Fatalf("scalar transform args = %#v err = %v", args, err)
	}

	// Transform error propagates
	boom := errors.New("transform boom")
	bad := []FlagSpec{{Name: "y", Usage: "Y", Transform: func(string) (any, error) { return nil, boom }}}
	cmd = newTestCommand()
	RegisterFlags(cmd, bad)
	_ = cmd.Flags().Set("y", "v")
	if _, err := BuildArgs(cmd, bad); !errors.Is(err, boom) {
		t.Fatalf("transform error = %v", err)
	}

	// integer parse error propagates from BuildArgs
	badInt := []FlagSpec{{Name: "n", Usage: "N", Kind: KindInt, EnvVar: "DWS_CMDCORE_BADINT"}}
	t.Setenv("DWS_CMDCORE_BADINT", "zzz")
	cmd = newTestCommand()
	RegisterFlags(cmd, badInt)
	if _, err := BuildArgs(cmd, badInt); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("bad int err = %v", err)
	}
}

// ── constraints ────────────────────────────────────────────────────

func TestCrossPlatformCoverageValidateConstraintDeclsPanics(t *testing.T) {
	flags := []FlagSpec{{Name: "a", Usage: "A"}, {Name: "b", Usage: "B"}}
	// valid declaration does not panic
	ValidateConstraintDecls("ok", flags, []Constraint{{Kind: ExactlyOne, Flags: []string{"a", "b"}}})
	ValidateConstraintDecls("custom", flags, []Constraint{{
		Kind: Custom, Flags: []string{"a"}, Description: "由 Validate 执行",
	}})

	mustPanic := func(name string, constraints []Constraint, needle string) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected panic", name)
			}
			if msg, _ := r.(string); !strings.Contains(msg, needle) {
				t.Fatalf("%s: panic = %v, want %q", name, r, needle)
			}
		}()
		ValidateConstraintDecls("leafX", flags, constraints)
	}
	mustPanic("unknown kind", []Constraint{{Kind: "bogus", Flags: []string{"a", "b"}}}, "unknown constraint kind")
	mustPanic("too few flags", []Constraint{{Kind: AtLeastOne, Flags: []string{"a"}}}, "needs at least two flags")
	mustPanic("custom no flags", []Constraint{{Kind: Custom, Description: "x"}}, "needs at least one flag")
	mustPanic("custom no description", []Constraint{{Kind: Custom, Flags: []string{"a"}}}, "requires a description")
	mustPanic("undeclared", []Constraint{{Kind: AtLeastOne, Flags: []string{"a", "zzz"}}}, "references undeclared flag")
}

func TestCrossPlatformCoverageConstraintProvided(t *testing.T) {
	flags := []FlagSpec{
		{Name: "s", Usage: "S", Default: "reg", Aliases: []string{"s-alias"}, EnvVar: "DWS_CMDCORE_CP"},
		{Name: "b", Usage: "B", Kind: KindBool},
		{Name: "sl", Usage: "SL", Kind: KindStringSlice, Aliases: []string{"sl-alias"}},
	}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)

	// registration default must NOT count as provided
	if constraintProvided(cmd, flags[0]) {
		t.Fatal("registration default must not count as provided")
	}
	if constraintProvided(cmd, flags[1]) || constraintProvided(cmd, flags[2]) {
		t.Fatal("unset bool/slice must not count as provided")
	}

	// explicit main flag / alias / env each count
	_ = cmd.Flags().Set("s", "v")
	if !constraintProvided(cmd, flags[0]) {
		t.Fatal("explicit main flag must count")
	}
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("s-alias", "v")
	if !constraintProvided(cmd, flags[0]) {
		t.Fatal("explicit alias must count")
	}
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	t.Setenv("DWS_CMDCORE_CP", "v")
	if !constraintProvided(cmd, flags[0]) {
		t.Fatal("env must count")
	}

	// whitespace-only explicit values do not count
	blank := FlagSpec{Name: "w", Usage: "W", Aliases: []string{"w-alias"}}
	cmd = newTestCommand()
	RegisterFlags(cmd, []FlagSpec{blank})
	_ = cmd.Flags().Set("w", "  ")
	_ = cmd.Flags().Set("w-alias", "  ")
	if constraintProvided(cmd, blank) {
		t.Fatal("whitespace-only must not count")
	}

	// bool counts on Changed, slice counts on non-empty element (main or alias)
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("b", "false")
	if !constraintProvided(cmd, flags[1]) {
		t.Fatal("explicit bool false must count")
	}
	_ = cmd.Flags().Set("sl", " ")
	if constraintProvided(cmd, flags[2]) {
		t.Fatal("blank slice must not count")
	}
	_ = cmd.Flags().Set("sl-alias", "x")
	if !constraintProvided(cmd, flags[2]) {
		t.Fatal("alias slice must count")
	}
	// a non-blank main slice flag counts on its own (no alias involved)
	mainSlice := newTestCommand()
	RegisterFlags(mainSlice, flags)
	_ = mainSlice.Flags().Set("sl", "kept")
	if !constraintProvided(mainSlice, flags[2]) {
		t.Fatal("non-blank main slice must count")
	}
}

func TestCrossPlatformCoverageValidateConstraints(t *testing.T) {
	flags := []FlagSpec{{Name: "a", Usage: "A"}, {Name: "b", Usage: "B"}}
	build := func(set ...string) *cobra.Command {
		cmd := newTestCommand()
		RegisterFlags(cmd, flags)
		for _, name := range set {
			_ = cmd.Flags().Set(name, "v")
		}
		return cmd
	}

	atLeast := []Constraint{{Kind: AtLeastOne, Flags: []string{"a", "b"}}}
	if err := ValidateConstraints(build(), flags, atLeast); err == nil || !strings.Contains(err.Error(), "请至少指定 --a、--b 之一") {
		t.Fatalf("at_least_one(0) err = %v", err)
	}
	if err := ValidateConstraints(build("a"), flags, atLeast); err != nil {
		t.Fatalf("at_least_one(1) err = %v", err)
	}

	exactly := []Constraint{{Kind: ExactlyOne, Flags: []string{"a", "b"}}}
	if err := ValidateConstraints(build(), flags, exactly); err == nil || !strings.Contains(err.Error(), "请指定 --a、--b 之一") {
		t.Fatalf("exactly_one(0) err = %v", err)
	}
	if err := ValidateConstraints(build("a"), flags, exactly); err != nil {
		t.Fatalf("exactly_one(1) err = %v", err)
	}
	if err := ValidateConstraints(build("a", "b"), flags, exactly); err == nil ||
		!strings.Contains(err.Error(), "参数 --a、--b 只能指定其一（当前指定了 --a、--b）") {
		t.Fatalf("exactly_one(2) err = %v", err)
	}

	exclusive := []Constraint{{Kind: MutuallyExclusive, Flags: []string{"a", "b"}}}
	if err := ValidateConstraints(build(), flags, exclusive); err != nil {
		t.Fatalf("mutually_exclusive(0) err = %v", err)
	}
	if err := ValidateConstraints(build("a"), flags, exclusive); err != nil {
		t.Fatalf("mutually_exclusive(1) err = %v", err)
	}
	if err := ValidateConstraints(build("a", "b"), flags, exclusive); err == nil ||
		!strings.Contains(err.Error(), "参数 --a、--b 互斥，只能指定其一（当前指定了 --a、--b）") {
		t.Fatalf("mutually_exclusive(2) err = %v", err)
	}

	// no constraints → nil
	if err := ValidateConstraints(build(), flags, nil); err != nil {
		t.Fatalf("no constraints err = %v", err)
	}

	// Custom constraints are declaration/help-only; Validate owns the rule.
	custom := []Constraint{{Kind: Custom, Flags: []string{"a"}, Description: "由 Validate 执行"}}
	if err := ValidateConstraints(build("a"), flags, custom); err != nil {
		t.Fatalf("custom constraint must be a no-op in ValidateConstraints, got %v", err)
	}
}

// ── safety confirmation ────────────────────────────────────────────

func TestCrossPlatformCoverageConfirmSafety(t *testing.T) {
	newSafetyCmd := func(stdin string) *cobra.Command {
		cmd := newTestCommand()
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.PersistentFlags().Bool("dry-run", false, "")
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetErr(&strings.Builder{})
		return cmd
	}

	// Confirmation is the only field that drives the gate.
	if err := ConfirmSafety(newSafetyCmd(""), testReadSafety()); err != nil {
		t.Fatal("not_required safety must pass")
	}
	if err := ConfirmSafety(newSafetyCmd(""), contract.SafetySpec{Effect: "destructive", Risk: "high"}); err != nil {
		t.Fatal("effect/risk must not imply confirmation")
	}
	// --yes and --dry-run bypass the prompt
	yes := newSafetyCmd("")
	_ = yes.PersistentFlags().Set("yes", "true")
	if err := ConfirmSafety(yes, testDestructiveSafety()); err != nil {
		t.Fatal("--yes must bypass")
	}
	dry := newSafetyCmd("")
	_ = dry.PersistentFlags().Set("dry-run", "true")
	if err := ConfirmSafety(dry, testWriteSafety()); err != nil {
		t.Fatal("--dry-run must bypass")
	}
	// interactive accept / decline (SetIn buffers count as readable answers)
	for _, answer := range []string{"yes\n", "y\n", "YES\n"} {
		if err := ConfirmSafety(newSafetyCmd(answer), testWriteSafety()); err != nil {
			t.Fatalf("answer %q must confirm, got %v", answer, err)
		}
	}
	for _, answer := range []string{"no\n", "\n", "maybe\n"} {
		err := ConfirmSafety(newSafetyCmd(answer), testDestructiveSafety())
		if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
			t.Fatalf("answer %q must decline with cancel, got %v", answer, err)
		}
	}
	// EOF / closed stdin is ConfirmUnavailable, not decline
	err := ConfirmSafety(newSafetyCmd(""), testWriteSafety())
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("EOF must be confirmation_required, got %#v", err)
	}
}

func TestCrossPlatformCoverageConfirmSafetySuppressesPromptOffTerminal(t *testing.T) {
	// Non-terminal stdin (buffer/pipe/EOF): the interactive prompt line must
	// not pollute stderr — the structured error carries the semantics. Piped
	// answers still confirm for general ConfirmSafety (Sheet uses an outer
	// --yes-only gate instead).
	var stderr strings.Builder
	cmd := newTestCommand()
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&stderr)
	if err := ConfirmSafety(cmd, testWriteSafety()); err == nil {
		t.Fatal("EOF must fail closed")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("non-terminal stdin must not print a prompt, got %q", got)
	}

	var answered strings.Builder
	piped := newTestCommand()
	piped.PersistentFlags().Bool("yes", false, "")
	piped.SetIn(strings.NewReader("yes\n"))
	piped.SetErr(&answered)
	if err := ConfirmSafety(piped, testWriteSafety()); err != nil {
		t.Fatalf("piped answer must confirm, got %v", err)
	}
	if got := answered.String(); got != "" {
		t.Fatalf("piped answer path must also skip the prompt, got %q", got)
	}
}

func TestCrossPlatformCoverageBoolFlag(t *testing.T) {
	if BoolFlag(nil, "yes") {
		t.Fatal("nil command must report false")
	}
	// missing flag → false
	if BoolFlag(newTestCommand(), "yes") {
		t.Fatal("missing flag must report false")
	}
	// local flag
	local := newTestCommand()
	local.Flags().Bool("yes", true, "")
	if !BoolFlag(local, "yes") {
		t.Fatal("local flag must be read")
	}
	// root persistent flag read from a child (inherited path)
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("yes", true, "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if !BoolFlag(child, "yes") {
		t.Fatal("root persistent flag must be visible to the child")
	}
	// A leaf-local false must not shadow a root persistent true: BoolFlag and
	// confirmationBypass must agree, or confirmation is bypassed as a dry run
	// while Ctx.DryRun() reports false.
	shadowRoot := &cobra.Command{Use: "root"}
	shadowRoot.PersistentFlags().Bool("dry-run", true, "")
	shadowLeaf := &cobra.Command{Use: "leaf"}
	shadowLeaf.Flags().Bool("dry-run", false, "")
	shadowRoot.AddCommand(shadowLeaf)
	if !BoolFlag(shadowLeaf, "dry-run") {
		t.Fatal("leaf-local default must not shadow root persistent dry-run")
	}
	if !confirmationBypass(shadowLeaf) {
		t.Fatal("confirmationBypass disagrees with BoolFlag")
	}
}

// ── schema projection + help ───────────────────────────────────────

func TestCrossPlatformCoverageAnnotateConstraints(t *testing.T) {
	cmd := newTestCommand()
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		cmd.Flags().String(name, "", "")
	}
	AnnotateConstraints(cmd, []Constraint{
		{Kind: AtLeastOne, Flags: []string{"a", "b"}},
		{Kind: ExactlyOne, Flags: []string{"c", "d"}},
		{Kind: MutuallyExclusive, Flags: []string{"e", "f"}},
	})
	encoded := cmd.Annotations["dws.schema.constraints"]
	if !strings.Contains(encoded, `"require_one_of"`) || !strings.Contains(encoded, `"mutually_exclusive"`) {
		t.Fatalf("annotation = %s", encoded)
	}
	// exactly_one projects into BOTH require_one_of and mutually_exclusive
	for _, want := range []string{`["a","b"]`, `["c","d"]`, `["e","f"]`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("annotation missing %s: %s", want, encoded)
		}
	}

	// no constraints → no annotation written
	bare := newTestCommand()
	AnnotateConstraints(bare, nil)
	if bare.Annotations["dws.schema.constraints"] != "" {
		t.Fatalf("unexpected annotation %q", bare.Annotations["dws.schema.constraints"])
	}

	// Single *visible* flag with a hidden sibling must NOT collapse to
	// unconditionally required: runtime ValidateConstraints still accepts the
	// hidden member, so Schema must keep the full declared group.
	single := newTestCommand()
	single.Flags().String("only", "", "")
	single.Flags().String("hidden", "", "")
	_ = single.Flags().MarkHidden("hidden")
	AnnotateConstraints(single, []Constraint{
		{Kind: AtLeastOne, Flags: []string{"only", "hidden"}},
		{Kind: ExactlyOne, Flags: []string{"only", "hidden"}},
	})
	if got := single.Flags().Lookup("only").Annotations[runtimeannotate.AnnotationFlagRequired]; len(got) > 0 {
		t.Fatalf("visible flag must not be marked required when hidden sibling exists: %#v", got)
	}
	encodedSingle := single.Annotations["dws.schema.constraints"]
	if !strings.Contains(encodedSingle, `["only","hidden"]`) {
		t.Fatalf("hidden-sibling group must project full require_one_of, got %s", encodedSingle)
	}

	// A true single-flag group (no hidden siblings) may still collapse to required.
	solo := newTestCommand()
	solo.Flags().String("solo", "", "")
	AnnotateConstraints(solo, []Constraint{
		{Kind: AtLeastOne, Flags: []string{"solo"}},
	})
	got := solo.Flags().Lookup("solo").Annotations[runtimeannotate.AnnotationFlagRequired]
	if len(got) != 1 || got[0] != "true" {
		t.Fatalf("solo-flag required annotation = %#v", solo.Flags().Lookup("solo").Annotations)
	}

	// ExactlyOne with a sole published member also collapses to required.
	exactSolo := newTestCommand()
	exactSolo.Flags().String("pick", "", "")
	AnnotateConstraints(exactSolo, []Constraint{
		{Kind: ExactlyOne, Flags: []string{"pick"}},
	})
	if got := exactSolo.Flags().Lookup("pick").Annotations[runtimeannotate.AnnotationFlagRequired]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("exactly-one solo required annotation = %#v", exactSolo.Flags().Lookup("pick").Annotations)
	}
}

func TestAnnotateConstraintsHiddenSiblingRuntimeHomology(t *testing.T) {
	flags := []FlagSpec{
		{Name: "only", Usage: "visible"},
		{Name: "hidden", Usage: "hidden sibling", Hidden: true},
	}
	constraints := []Constraint{{Kind: AtLeastOne, Flags: []string{"only", "hidden"}}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	AnnotateConstraints(cmd, constraints)
	if got := cmd.Flags().Lookup("only").Annotations[runtimeannotate.AnnotationFlagRequired]; len(got) > 0 {
		t.Fatalf("Schema must not mark visible flag required: %#v", got)
	}
	_ = cmd.Flags().Set("hidden", "v")
	if err := ValidateConstraints(cmd, flags, constraints); err != nil {
		t.Fatalf("runtime must still accept hidden sibling alone: %v", err)
	}
}

func TestCrossPlatformCoverageConstraintHelp(t *testing.T) {
	if got := ConstraintHelp(nil); got != "" {
		t.Fatalf("empty constraints help = %q", got)
	}
	help := ConstraintHelp([]Constraint{
		{Kind: AtLeastOne, Flags: []string{"a", "b"}},
		{Kind: ExactlyOne, Flags: []string{"c", "d"}},
		{Kind: MutuallyExclusive, Flags: []string{"e", "f"}},
		{Kind: AtLeastOne, Flags: []string{"g", "h"}, Description: "自定义文案"},
	})
	for _, want := range []string{
		"参数约束：",
		"--a、--b 至少指定一个",
		"--c、--d 必须且只能指定一个",
		"--e、--f 互斥，最多指定一个",
		"  - 自定义文案",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

// ── unified builder ────────────────────────────────────────────────

func TestCrossPlatformCoverageNewCommandOrchestration(t *testing.T) {
	var order []string
	var gotArgs map[string]any
	postMounted := false

	cmd := New(Spec{
		Use:     "route",
		Short:   "S",
		Long:    "L",
		Example: "E",
		Flags: []FlagSpec{
			{Name: "a", Usage: "A", Bind: "aKey"},
			{Name: "b", Usage: "B"},
		},
		Constraints: []Constraint{{Kind: MutuallyExclusive, Flags: []string{"a", "b"}}},
		Validate: func(*cobra.Command, []string) error {
			order = append(order, "validate")
			return nil
		},
		PostMount: func(c *cobra.Command) {
			postMounted = true
			if c.RunE != nil {
				t.Error("PostMount must run before RunE is assigned")
			}
		},
		Invoke: func(_ *Ctx, toolArgs map[string]any) error {
			order = append(order, "dispatch")
			gotArgs = toolArgs
			return nil
		},
	})

	if !postMounted {
		t.Fatal("PostMount not invoked")
	}
	if cmd.Use != "route" || cmd.Short != "S" || cmd.Example != "E" {
		t.Fatalf("identity = %#v", cmd)
	}
	// constraint help appended to Long, schema annotation projected
	if !strings.Contains(cmd.Long, "参数约束：") || !strings.HasPrefix(cmd.Long, "L") {
		t.Fatalf("Long = %q", cmd.Long)
	}
	if cmd.Annotations["dws.schema.constraints"] == "" {
		t.Fatal("schema constraints not projected")
	}
	if cmd.Annotations["dws.schema.contract"] != "command" {
		t.Fatalf("contract embed marker = %q", cmd.Annotations["dws.schema.contract"])
	}
	if got := cmd.Flags().Lookup("a").Annotations["dws.schema.property"]; len(got) == 0 || got[0] != "aKey" {
		t.Fatalf("flag a property annotation = %#v, want aKey", cmd.Flags().Lookup("a").Annotations["dws.schema.property"])
	}

	cmd.SetArgs([]string{"--a", "v"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"validate", "dispatch"}) {
		t.Fatalf("order = %v, want validate before dispatch", order)
	}
	if gotArgs["aKey"] != "v" {
		t.Fatalf("dispatch args = %#v", gotArgs)
	}
}

func TestCrossPlatformCoverageNewCommandRunEEscapeHatch(t *testing.T) {
	// The escape hatch replaces the dispatch body, not the declared contract:
	// a declared Required flag is still enforced and RunE is never entered.
	ran := false
	newEscape := func() *cobra.Command {
		ran = false
		cmd := New(Spec{
			Use:   "escape",
			Flags: []FlagSpec{{Name: "x", Usage: "X", Required: true}},
			RunE: func(*cobra.Command, []string) error {
				ran = true
				return nil
			},
		})
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return cmd
	}

	missing := newEscape()
	missing.SetArgs(nil)
	if err := missing.Execute(); err == nil {
		t.Fatal("escape hatch must still enforce a declared Required flag")
	}
	if ran {
		t.Fatal("escape hatch RunE ran despite a failed declared check")
	}

	satisfied := newEscape()
	satisfied.SetArgs([]string{"--x", "value"})
	if err := satisfied.Execute(); err != nil {
		t.Fatalf("escape hatch must run once the declaration is satisfied: %v", err)
	}
	if !ran {
		t.Fatal("escape hatch did not run")
	}
}

func TestCrossPlatformCoverageNewCommandStopsOnFailures(t *testing.T) {
	// required failure stops before Validate/Dispatch
	validated := false
	dispatched := false
	spec := Spec{
		Use:   "gate",
		Flags: []FlagSpec{{Name: "need", Usage: "N", Required: true}, {Name: "other", Usage: "O"}},
		Constraints: []Constraint{
			{Kind: MutuallyExclusive, Flags: []string{"need", "other"}},
		},
		Validate: func(*cobra.Command, []string) error {
			validated = true
			return nil
		},
		Invoke: func(*Ctx, map[string]any) error {
			dispatched = true
			return nil
		},
	}
	cmd := New(spec)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "need") {
		t.Fatalf("required err = %v", err)
	}
	if validated || dispatched {
		t.Fatal("required failure must stop the pipeline")
	}

	// constraint failure stops before Validate/Dispatch
	validated, dispatched = false, false
	cmd = New(spec)
	cmd.SetArgs([]string{"--need", "a", "--other", "b"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("constraint err = %v", err)
	}
	if validated || dispatched {
		t.Fatal("constraint failure must stop before Validate")
	}

	// Validate hook failure stops before Dispatch
	boom := errors.New("validate boom")
	dispatched = false
	cmd = New(Spec{
		Use:      "hook",
		Flags:    []FlagSpec{{Name: "x", Usage: "X"}},
		Validate: func(*cobra.Command, []string) error { return boom },
		Invoke: func(*Ctx, map[string]any) error {
			dispatched = true
			return nil
		},
	})
	cmd.SetArgs([]string{"--x", "v"})
	if err := cmd.Execute(); !errors.Is(err, boom) {
		t.Fatalf("validate hook err = %v", err)
	}
	if dispatched {
		t.Fatal("Validate failure must stop before Dispatch")
	}

	// BuildArgs failure stops before Dispatch
	dispatched = false
	cmd = New(Spec{
		Use:   "args",
		Flags: []FlagSpec{{Name: "y", Usage: "Y", Transform: func(string) (any, error) { return nil, boom }}},
		Invoke: func(*Ctx, map[string]any) error {
			dispatched = true
			return nil
		},
	})
	cmd.SetArgs([]string{"--y", "v"})
	if err := cmd.Execute(); !errors.Is(err, boom) {
		t.Fatalf("BuildArgs err = %v", err)
	}
	if dispatched {
		t.Fatal("BuildArgs failure must stop before Dispatch")
	}
}

func TestCrossPlatformCoverageNewCommandDeclineCancels(t *testing.T) {
	dispatched := false
	cmd := New(Spec{
		Use:    "risky",
		Safety: testDestructiveSafety(),
		Flags:  []FlagSpec{{Name: "x", Usage: "X"}},
		Invoke: func(*Ctx, map[string]any) error {
			dispatched = true
			return nil
		},
	})
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--x", "v"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("decline err = %v", err)
	}
	if dispatched {
		t.Fatal("declined command must not dispatch")
	}
}

func TestCrossPlatformCoverageNewCommandRequiresExactlyOneDispatcher(t *testing.T) {
	mustPanic := func(name string, spec Spec, needle string) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected panic", name)
			}
			if msg, _ := r.(string); !strings.Contains(msg, needle) {
				t.Fatalf("%s: panic = %v, want %q", name, r, needle)
			}
		}()
		New(spec)
	}
	flags := []FlagSpec{{Name: "x", Usage: "X"}}

	// Zero dispatchers: a spec with no runnable body must never reach run time,
	// where it would have prompted for confirmation and then exited 0 doing nothing.
	mustPanic("no dispatcher", Spec{Use: "bare", Safety: testDestructiveSafety(), Flags: flags},
		"must declare exactly one of RunE/Invoke/Orchestrate, got 0")

	// Two competing dispatchers are equally a programming error.
	mustPanic("two dispatchers", Spec{
		Use:         "both",
		Flags:       flags,
		Invoke:      func(*Ctx, map[string]any) error { return nil },
		Orchestrate: func(*Ctx) error { return nil },
	}, "got 2")
	mustPanic("runE plus invoke", Spec{
		Use:    "both2",
		Flags:  flags,
		RunE:   func(*cobra.Command, []string) error { return nil },
		Invoke: func(*Ctx, map[string]any) error { return nil },
	}, "got 2")

	// ConfirmFirst without a user_required confirmation orders a gate that
	// does not exist.
	mustPanic("confirmFirst without confirmation", Spec{
		Use:          "guarded",
		Flags:        flags,
		ConfirmFirst: true,
		Invoke:       func(*Ctx, map[string]any) error { return nil },
	}, "Safety.Confirmation is not user_required")
}

func TestCrossPlatformCoverageNewCommandOrchestrateDispatch(t *testing.T) {
	var seen struct {
		str   string
		n     int
		flag  bool
		slice []string
		args  []string
		dry   bool
		yes   bool
		chg   bool
	}
	t.Setenv("DWS_CMDCORE_ORCH_ENV", "from-env")
	t.Setenv("DWS_CMDCORE_ORCH_BADINT", "not-a-number")
	cmd := New(Spec{
		Use: "orch",
		Flags: []FlagSpec{
			{Name: "name", Usage: "N", Aliases: []string{"n-alias"}},
			{Name: "env", Usage: "E", EnvVar: "DWS_CMDCORE_ORCH_ENV"},
			{Name: "count", Usage: "C", Kind: KindInt},
			{Name: "bad", Usage: "B", Kind: KindInt, EnvVar: "DWS_CMDCORE_ORCH_BADINT"},
			{Name: "on", Usage: "O", Kind: KindBool},
			{Name: "ids", Usage: "I", Kind: KindStringSlice},
		},
		Orchestrate: func(c *Ctx) error {
			// Ctx accessors must honor the declared alias/env fallback chain.
			seen.str = c.Str("name")
			seen.n = c.Int("count")
			seen.flag = c.Bool("on")
			seen.slice = c.StrSlice("ids")
			seen.args = c.Args()
			seen.dry = c.DryRun()
			seen.yes = c.Yes()
			seen.chg = c.Changed("count")
			if got := c.Str("env"); got != "from-env" {
				t.Errorf("Ctx.Str(env) = %q, want env fallback", got)
			}
			if c.Command() == nil || c.Command().Use != "orch" {
				t.Errorf("Ctx.Command() = %v", c.Command())
			}
			// Undeclared names resolve to zero values rather than panicking.
			if c.Str("nope") != "" || c.Int("nope") != 0 || c.StrSlice("nope") != nil {
				t.Error("undeclared flag must yield zero values")
			}
			// An unparseable effective value degrades to 0 instead of erroring.
			if got := c.Int("bad"); got != 0 {
				t.Errorf("Ctx.Int(bad) = %d, want 0", got)
			}
			return nil
		},
	})
	cmd.Args = cobra.ArbitraryArgs
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--n-alias", "via-alias", "--count", "4", "--on", "--ids", " a , b ", "--yes", "pos1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if seen.str != "via-alias" {
		t.Fatalf("Ctx.Str(name) = %q, want alias value", seen.str)
	}
	if seen.n != 4 || !seen.flag || !seen.chg {
		t.Fatalf("typed accessors = %+v", seen)
	}
	if !reflect.DeepEqual(seen.slice, []string{"a", "b"}) {
		t.Fatalf("Ctx.StrSlice = %#v", seen.slice)
	}
	if !reflect.DeepEqual(seen.args, []string{"pos1"}) {
		t.Fatalf("Ctx.Args = %#v", seen.args)
	}
	if seen.dry || !seen.yes {
		t.Fatalf("global flags: dry=%v yes=%v", seen.dry, seen.yes)
	}
}

func TestCrossPlatformCoverageNewCommandOrchestrateHonorsConfirmation(t *testing.T) {
	ran := false
	build := func() *cobra.Command {
		cmd := New(Spec{
			Use:         "risky-orch",
			Safety:      testDestructiveSafety(),
			Flags:       []FlagSpec{{Name: "x", Usage: "X"}},
			Orchestrate: func(*Ctx) error { ran = true; return nil },
		})
		cmd.PersistentFlags().Bool("dry-run", false, "")
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetErr(&strings.Builder{})
		return cmd
	}
	// Declining must abort before the orchestration body runs.
	declined := build()
	declined.SetIn(strings.NewReader("no\n"))
	declined.SetArgs([]string{"--x", "v"})
	if err := declined.Execute(); err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined orchestrate err = %v", err)
	}
	if ran {
		t.Fatal("declined orchestrate must not run the body")
	}
	// --yes bypasses the prompt and runs it.
	confirmed := build()
	confirmed.SetArgs([]string{"--x", "v", "--yes"})
	if err := confirmed.Execute(); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("confirmed orchestrate did not run")
	}
}

func TestNewCommandEmbedsContractFlagsWithoutLegacyRiskAnnotation(t *testing.T) {
	cmd := New(Spec{
		Use:    "wipe",
		Safety: testDestructiveSafety(),
		Flags: []FlagSpec{
			{Name: "id", Usage: "ID", Required: true, Bind: "versionId", Kind: KindString},
			{Name: "count", Usage: "N", Kind: KindInt, Default: "1"},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	if _, ok := cmd.Annotations["dws.schema.risk"]; ok {
		t.Fatal("command must not emit the legacy dws.schema.risk annotation")
	}
	id := cmd.Flags().Lookup("id")
	if id.Annotations["dws.schema.required"][0] != "true" {
		t.Fatalf("id required annotation = %#v", id.Annotations["dws.schema.required"])
	}
	if id.Annotations["dws.schema.type"][0] != "string" {
		t.Fatalf("id type = %#v", id.Annotations["dws.schema.type"])
	}
	if cmd.Flags().Lookup("count").Annotations["dws.schema.type"][0] != "integer" {
		t.Fatalf("count type = %#v", cmd.Flags().Lookup("count").Annotations["dws.schema.type"])
	}
	// Empty Safety must not stamp the removed Risk annotation.
	plain := New(Spec{
		Use:    "list",
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	if _, ok := plain.Annotations["dws.schema.risk"]; ok {
		t.Fatal("empty Safety must not embed dws.schema.risk")
	}
	if plain.Annotations["dws.schema.contract"] != "command" {
		t.Fatal("contract marker still required when Safety empty")
	}
}

func TestCrossPlatformCoverageRegisterFlagTypedDefaults(t *testing.T) {
	cmd := newTestCommand()
	RegisterFlag(cmd, KindInt, "page-size", "20", "page")
	RegisterFlag(cmd, KindBool, "flag", "true", "bool")
	RegisterFlag(cmd, KindBool, "off", "false", "bool off")
	if def, _ := cmd.Flags().GetInt("page-size"); def != 20 {
		t.Fatalf("int default = %d, want 20", def)
	}
	if def, _ := cmd.Flags().GetBool("flag"); !def {
		t.Fatal("bool default = false, want true")
	}
	if def, _ := cmd.Flags().GetBool("off"); def {
		t.Fatal("bool default = true, want false")
	}
	if got := cmd.Flags().Lookup("page-size").DefValue; got != "20" {
		t.Fatalf("page-size DefValue = %q, want 20", got)
	}
}

func TestCrossPlatformCoverageRegisterFlagMalformedDefaultPanics(t *testing.T) {
	mustPanic := func(name string, kind FlagKind, def, needle string) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected panic", name)
			}
			if msg, _ := r.(string); !strings.Contains(msg, needle) {
				t.Fatalf("%s: panic = %v, want %q", name, r, needle)
			}
		}()
		RegisterFlag(newTestCommand(), kind, "x", def, "usage")
	}
	mustPanic("bad int", KindInt, "abc", "invalid KindInt Default")
	mustPanic("bool TRUE", KindBool, "TRUE", "invalid KindBool Default")
	mustPanic("bool 1", KindBool, "1", "invalid KindBool Default")
	mustPanic("bool yes", KindBool, "yes", "invalid KindBool Default")
}

func TestCrossPlatformCoverageBuildArgsIntArgDefaultFloor(t *testing.T) {
	flags := []FlagSpec{{
		Name: "page-size", Usage: "P", Kind: KindInt,
		Default: "20", ArgDefault: "20", Bind: "pageSize",
	}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)

	args, err := BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["pageSize"] != 20 {
		t.Fatalf("unset pageSize = %#v, want 20", args["pageSize"])
	}

	_ = cmd.Flags().Set("page-size", "0")
	args, err = BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["pageSize"] != 20 {
		t.Fatalf("zero pageSize = %#v, want floor 20", args["pageSize"])
	}
}

func TestCrossPlatformCoverageNewCommandFreezesAndMergesConstParams(t *testing.T) {
	var got map[string]any
	declared := map[string]any{
		"precheckOnly":      false,
		"convThreadEnabled": true,
	}
	cmd := New(Spec{
		Use:         "pub",
		Flags:       []FlagSpec{{Name: "id", Usage: "ID", Bind: "versionId", Trim: true}},
		ConstParams: declared,
		Invoke: func(_ *Ctx, toolArgs map[string]any) error {
			got = toolArgs
			return nil
		},
	})
	declared["precheckOnly"] = true
	declared["forgedAfterNew"] = true
	_ = cmd.Flags().Set("id", "V1")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if got["versionId"] != "V1" {
		t.Fatalf("versionId = %#v", got["versionId"])
	}
	if got["precheckOnly"] != false {
		t.Fatalf("precheckOnly = %#v, want false", got["precheckOnly"])
	}
	if got["convThreadEnabled"] != true {
		t.Fatalf("convThreadEnabled = %#v, want true", got["convThreadEnabled"])
	}
	if _, exists := got["forgedAfterNew"]; exists {
		t.Fatalf("caller mutation leaked into dispatch args: %#v", got)
	}
	evidence := InterfaceBoolConstParams(cmd)
	if !reflect.DeepEqual(evidence, map[string]bool{"convThreadEnabled": true, "precheckOnly": false}) {
		t.Fatalf("bool ConstParams evidence = %#v", evidence)
	}
	if got["precheckOnly"] != evidence["precheckOnly"] {
		t.Fatalf("dispatch/evidence drift: args=%#v evidence=%#v", got, evidence)
	}
}

func TestCrossPlatformCoverageNewCommandDoesNotProjectMixedConstParamsEvidence(t *testing.T) {
	var got map[string]any
	cmd := New(Spec{
		Use: "mixed",
		ConstParams: map[string]any{
			"convThreadEnabled": true,
			"retryLimit":        3,
		},
		Invoke: func(_ *Ctx, toolArgs map[string]any) error {
			got = toolArgs
			return nil
		},
	})

	if evidence := InterfaceBoolConstParams(cmd); evidence != nil {
		t.Fatalf("mixed ConstParams evidence = %#v; want missing evidence", evidence)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if got["convThreadEnabled"] != true || got["retryLimit"] != 3 {
		t.Fatalf("mixed ConstParams dispatch args = %#v", got)
	}
}

func TestCrossPlatformCoverageNewCommandRejectsInvalidConstParamsDispatch(t *testing.T) {
	mustPanic := func(name string, spec Spec, needle string) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected panic", name)
			}
			if msg, _ := r.(string); !strings.Contains(msg, needle) {
				t.Fatalf("%s: panic=%v, want %q", name, r, needle)
			}
		}()
		New(spec)
	}

	mustPanic("RunE", Spec{
		Use:         "run-e",
		ConstParams: map[string]any{"fixed": true},
		RunE:        func(*cobra.Command, []string) error { return nil },
	}, "ConstParams require Invoke or ResultInvoke")
	mustPanic("Orchestrate", Spec{
		Use:         "orchestrate",
		ConstParams: map[string]any{"fixed": true},
		Orchestrate: func(*Ctx) error { return nil },
	}, "ConstParams require Invoke or ResultInvoke")
	mustPanic("explicit bind conflict", Spec{
		Use:         "bind-conflict",
		Flags:       []FlagSpec{{Name: "thread", Usage: "T", Bind: "convThreadEnabled"}},
		ConstParams: map[string]any{"convThreadEnabled": true},
		Invoke:      func(*Ctx, map[string]any) error { return nil },
	}, "conflicts with flag --thread")
	mustPanic("default bind conflict", Spec{
		Use:         "default-bind-conflict",
		Flags:       []FlagSpec{{Name: "fixed-value", Usage: "F"}},
		ConstParams: map[string]any{"fixedValue": true},
		Invoke:      func(*Ctx, map[string]any) error { return nil },
	}, "conflicts with flag --fixed-value")

	result := New(Spec{
		Use:           "result",
		OutputRollout: output.RolloutUnifiedActive,
		ConstParams:   map[string]any{"fixed": true},
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Success(nil), nil
		},
	})
	if evidence := InterfaceBoolConstParams(result); !evidence["fixed"] {
		t.Fatalf("ResultInvoke ConstParams evidence = %#v", evidence)
	}
}

func TestCrossPlatformCoverageNewCommandConfirmFirstAnnotationAndOrder(t *testing.T) {
	ran := false
	build := func(postMount func(*cobra.Command)) *cobra.Command {
		cmd := New(Spec{
			Use:          "guard-first",
			Safety:       testWriteSafety(),
			ConfirmFirst: true,
			Flags:        []FlagSpec{{Name: "x", Usage: "X", Required: true}},
			PostMount:    postMount,
			Invoke: func(*Ctx, map[string]any) error {
				ran = true
				return nil
			},
		})
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.PersistentFlags().Bool("dry-run", false, "")
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetErr(&strings.Builder{})
		return cmd
	}

	// The declared marker is stamped even when a PostMount hook dropped the
	// annotations map (defensive re-init path).
	cleared := build(func(c *cobra.Command) { c.Annotations = nil })
	if cleared.Annotations[ConfirmFirstAnnotation] != "true" {
		t.Fatalf("annotations = %#v, want declared ConfirmFirst marker", cleared.Annotations)
	}
	if !HasDeclaredConfirmFirst(cleared) {
		t.Fatal("HasDeclaredConfirmFirst must report a declared guard-first command")
	}
	if HasDeclaredConfirmFirst(nil) || HasDeclaredConfirmFirst(newTestCommand()) {
		t.Fatal("HasDeclaredConfirmFirst must be false for nil/undeclared commands")
	}

	// Guard-first confirms BEFORE required validation: declining cancels even
	// though the required --x is missing.
	ran = false
	declined := build(nil)
	declined.SetIn(strings.NewReader("no\n"))
	declined.SetArgs(nil)
	if err := declined.Execute(); err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined guard-first err = %v", err)
	}
	if ran {
		t.Fatal("declined guard-first must not dispatch")
	}

	// After confirming, the declared Required check still gates dispatch.
	ran = false
	confirmed := build(nil)
	confirmed.SetIn(strings.NewReader("yes\n"))
	confirmed.SetArgs(nil)
	if err := confirmed.Execute(); err == nil || !strings.Contains(err.Error(), "x") {
		t.Fatalf("confirmed guard-first required err = %v", err)
	}
	if ran {
		t.Fatal("guard-first must not dispatch with the required flag missing")
	}

	// A satisfied declaration dispatches exactly once, without a second prompt.
	ran = false
	satisfied := build(nil)
	satisfied.SetIn(strings.NewReader("yes\n"))
	satisfied.SetArgs([]string{"--x", "v"})
	if err := satisfied.Execute(); err != nil {
		t.Fatalf("satisfied guard-first err = %v", err)
	}
	if !ran {
		t.Fatal("confirmed guard-first did not dispatch")
	}
}

func TestCrossPlatformCoverageNewCommandRunEHonorsConfirmation(t *testing.T) {
	// The escape hatch keeps the declared confirmation gate: an unanswerable
	// prompt fails closed before the RunE body runs.
	ran := false
	build := func() *cobra.Command {
		cmd := New(Spec{
			Use:    "guarded-rune",
			Safety: testWriteSafety(),
			Flags:  []FlagSpec{{Name: "x", Usage: "X"}},
			RunE: func(*cobra.Command, []string) error {
				ran = true
				return nil
			},
		})
		cmd.PersistentFlags().Bool("yes", false, "")
		cmd.PersistentFlags().Bool("dry-run", false, "")
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetErr(&strings.Builder{})
		return cmd
	}

	blocked := build()
	blocked.SetIn(strings.NewReader(""))
	blocked.SetArgs(nil)
	err := blocked.Execute()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("RunE confirmation err = %#v, want confirmation_required", err)
	}
	if ran {
		t.Fatal("RunE body must not run when confirmation fails")
	}

	confirmed := build()
	confirmed.SetArgs([]string{"--yes"})
	if err := confirmed.Execute(); err != nil {
		t.Fatalf("confirmed RunE err = %v", err)
	}
	if !ran {
		t.Fatal("confirmed RunE did not run")
	}
}

func TestCrossPlatformCoverageNewCommandPreflightEnumGate(t *testing.T) {
	dispatched := false
	cmd := New(Spec{
		Use:   "enum-gate",
		Flags: []FlagSpec{{Name: "mode", Usage: "M", Enum: []string{"a", "b"}}},
		Invoke: func(*Ctx, map[string]any) error {
			dispatched = true
			return nil
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--mode", "zzz"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), `参数 --mode 取值 "zzz" 不合法`) {
		t.Fatalf("preflight enum err = %v", err)
	}
	if dispatched {
		t.Fatal("enum failure must stop before dispatch")
	}
}

func TestCrossPlatformCoverageValidateRequiredShortcutMessages(t *testing.T) {
	// Without an authored RequiredError the unified default wording applies.
	plain := []FlagSpec{{Name: "target", Usage: "T", Required: true, ValidationMode: ValidationShortcut}}
	cmd := newTestCommand()
	RegisterFlags(cmd, plain)
	if err := ValidateRequired(cmd, plain); err == nil || err.Error() != "缺少必填参数 --target" {
		t.Fatalf("shortcut default message = %v", err)
	}

	// A provided-but-blank list still fails with the emptiness wording.
	slice := []FlagSpec{{
		Name: "ids", Usage: "I", Kind: KindStringSlice, Required: true,
		ValidationMode: ValidationShortcut, RequiredError: "缺少必填参数 --ids：列表",
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, slice)
	_ = cmd.Flags().Set("ids", " , ")
	if err := ValidateRequired(cmd, slice); err == nil || !strings.Contains(err.Error(), "必填参数 --ids 不能为空") {
		t.Fatalf("shortcut blank slice message = %v", err)
	}

	// A provided-but-blank string fails with the same emptiness wording.
	blank := []FlagSpec{{Name: "name", Usage: "N", Required: true, ValidationMode: ValidationShortcut}}
	cmd = newTestCommand()
	RegisterFlags(cmd, blank)
	_ = cmd.Flags().Set("name", "   ")
	if err := ValidateRequired(cmd, blank); err == nil || !strings.Contains(err.Error(), "必填参数 --name 不能为空") {
		t.Fatalf("shortcut blank string message = %v", err)
	}

	// Shortcut-mode enum violations surface through ValidateRequired too.
	enum := []FlagSpec{{Name: "mode", Usage: "M", ValidationMode: ValidationShortcut, Enum: []string{"a", "b"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, enum)
	_ = cmd.Flags().Set("mode", "z")
	if err := ValidateRequired(cmd, enum); err == nil || !strings.Contains(err.Error(), `参数 --mode 取值 "z" 不合法`) {
		t.Fatalf("shortcut enum message = %v", err)
	}
}

func TestCrossPlatformCoverageValidateAliasAwareShortcutAndEnum(t *testing.T) {
	// D2: a declared alias satisfies Shortcut Required.
	aliased := []FlagSpec{{
		Name: "target", Usage: "T", Required: true,
		ValidationMode: ValidationShortcut, Aliases: []string{"t-alias"},
	}}
	cmd := newTestCommand()
	RegisterFlags(cmd, aliased)
	_ = cmd.Flags().Set("t-alias", "hit")
	if err := ValidateRequired(cmd, aliased); err != nil {
		t.Fatalf("alias-satisfied shortcut required err = %v", err)
	}

	// D2: a registration default alone is not an explicit token, so Shortcut
	// Required still reports the missing parameter.
	defaulted := []FlagSpec{{
		Name: "target", Usage: "T", Required: true, Default: "reg-default",
		ValidationMode: ValidationShortcut, Aliases: []string{"t-alias"},
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, defaulted)
	if err := ValidateRequired(cmd, defaulted); err == nil || err.Error() != "缺少必填参数 --target" {
		t.Fatalf("default-only shortcut required err = %v", err)
	}

	// D1: an enum violation arriving through the alias is validated, not
	// skipped, and the message names the main flag.
	scalar := []FlagSpec{{Name: "mode", Usage: "M", Enum: []string{"a", "b"}, Aliases: []string{"m-alias"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, scalar)
	_ = cmd.Flags().Set("m-alias", "z")
	if err := ValidateEnums(cmd, scalar); err == nil || !strings.Contains(err.Error(), `参数 --mode 取值 "z" 不合法`) {
		t.Fatalf("alias enum violation = %v", err)
	}

	// D1: a valid alias value passes.
	cmd = newTestCommand()
	RegisterFlags(cmd, scalar)
	_ = cmd.Flags().Set("m-alias", "a")
	if err := ValidateEnums(cmd, scalar); err != nil {
		t.Fatalf("valid alias enum err = %v", err)
	}

	// D1: slice flags honor the alias too.
	sliced := []FlagSpec{{Name: "ids", Usage: "I", Kind: KindStringSlice, Enum: []string{"x", "y"}, Aliases: []string{"ids-alias"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, sliced)
	_ = cmd.Flags().Set("ids-alias", "x,z")
	if err := ValidateEnums(cmd, sliced); err == nil || !strings.Contains(err.Error(), `参数 --ids 取值 "z" 不合法`) {
		t.Fatalf("alias slice enum violation = %v", err)
	}

	// D1: an unprovided flag with a registered default stays unvalidated.
	untouched := []FlagSpec{{Name: "mode", Usage: "M", Enum: []string{"a", "b"}, Default: "z", Aliases: []string{"m-alias"}}}
	cmd = newTestCommand()
	RegisterFlags(cmd, untouched)
	if err := ValidateEnums(cmd, untouched); err != nil {
		t.Fatalf("default-only enum must not validate, err = %v", err)
	}
}

func TestCrossPlatformCoverageBuildArgsHonorsBoolAlias(t *testing.T) {
	// C1: a KindBool alias is honored wherever bool presence is checked —
	// BuildArgs transmits the alias value, hasEffectiveValue reports it as
	// provided, and constraintProvided counts it.
	flags := []FlagSpec{{Name: "force", Usage: "F", Kind: KindBool, Bind: "force", Aliases: []string{"force-alias"}}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("force-alias", "true")
	args, err := BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["force"] != true {
		t.Fatalf("bool alias must enter toolArgs as true, got %#v", args)
	}
	if !hasEffectiveValue(cmd, flags[0]) {
		t.Fatal("bool alias must count as an effective value")
	}
	if !constraintProvided(cmd, flags[0]) {
		t.Fatal("bool alias must count as constraint-provided")
	}

	// Explicit false via the alias is still transmitted (Changed semantics).
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("force-alias", "false")
	args, err = BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := args["force"]; !ok || v != false {
		t.Fatalf("explicit false via bool alias must be sent, got %#v", args)
	}

	// The main flag keeps precedence when both names are changed.
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("force", "false")
	_ = cmd.Flags().Set("force-alias", "true")
	args, err = BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["force"] != false {
		t.Fatalf("main bool flag must win over the alias, got %#v", args)
	}

	// An untouched bool flag stays out of toolArgs.
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	args, err = BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := args["force"]; ok {
		t.Fatalf("untouched bool must not enter toolArgs: %#v", args)
	}
}

func TestCrossPlatformCoverageValidateEnumEnvValue(t *testing.T) {
	// C3: an env-sourced enum value is validated even though env never counts
	// as provided.
	flags := []FlagSpec{{Name: "mode", Usage: "M", Enum: []string{"a", "b"}, EnvVar: "DWS_CMDCORE_ENUM_MODE"}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "zzz")
	if err := ValidateEnums(cmd, flags); err == nil || !strings.Contains(err.Error(), `参数 --mode 取值 "zzz" 不合法`) {
		t.Fatalf("env enum violation must be rejected, got %v", err)
	}

	// A valid env value passes.
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "a")
	if err := ValidateEnums(cmd, flags); err != nil {
		t.Fatalf("valid env enum value err = %v", err)
	}

	// Env still never counts as provided: Shortcut Required keeps demanding
	// the explicit token.
	shortcut := []FlagSpec{{
		Name: "mode", Usage: "M", Required: true, ValidationMode: ValidationShortcut,
		Enum: []string{"a", "b"}, EnvVar: "DWS_CMDCORE_ENUM_MODE",
	}}
	cmd = newTestCommand()
	RegisterFlags(cmd, shortcut)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "a")
	if err := ValidateRequired(cmd, shortcut); err == nil || err.Error() != "缺少必填参数 --mode" {
		t.Fatalf("env must not count as provided, got %v", err)
	}

	// An explicit token wins over the env fallback: an invalid env value does
	// not surface while the explicit value is valid.
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "zzz")
	_ = cmd.Flags().Set("mode", "b")
	if err := ValidateEnums(cmd, flags); err != nil {
		t.Fatalf("explicit token must shadow the invalid env value, got %v", err)
	}

	// Empty env stays unvalidated.
	cmd = newTestCommand()
	RegisterFlags(cmd, flags)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "")
	if err := ValidateEnums(cmd, flags); err != nil {
		t.Fatalf("empty env must not validate, got %v", err)
	}

	// Slice flags never consume env (explicit tokens only), so an out-of-enum
	// env value must not be rejected for them.
	sliceFlags := []FlagSpec{{Name: "mode", Usage: "M", Kind: KindStringSlice, Enum: []string{"a", "b"}, EnvVar: "DWS_CMDCORE_ENUM_MODE"}}
	cmd = newTestCommand()
	RegisterFlags(cmd, sliceFlags)
	t.Setenv("DWS_CMDCORE_ENUM_MODE", "zzz")
	if err := ValidateEnums(cmd, sliceFlags); err != nil {
		t.Fatalf("slice flags must not validate env values, got %v", err)
	}
}

func TestRegisterFlagsMarkRequiredWithAliasesPanics(t *testing.T) {
	// C2: MarkRequired + Aliases is rejected at build time — cobra
	// MarkFlagRequired only knows the main name.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for MarkRequired combined with Aliases")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "MarkRequired cannot be combined with Aliases") {
			t.Fatalf("panic = %v", r)
		}
	}()
	RegisterFlags(newTestCommand(), []FlagSpec{{
		Name: "req", Usage: "R", MarkRequired: true, Aliases: []string{"req-alias"},
	}})
}

func TestCrossPlatformCoverageRegisterFlagsMarkRequiredWithAliasesPanics(t *testing.T) {
	TestRegisterFlagsMarkRequiredWithAliasesPanics(t)
}

func TestCrossPlatformCoverageBuildArgsInvalidArgDefault(t *testing.T) {
	flags := []FlagSpec{{Name: "page-size", Usage: "P", Kind: KindInt, ArgDefault: "not-a-number"}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	if _, err := BuildArgs(cmd, flags); err == nil || !strings.Contains(err.Error(), `invalid ArgDefault "not-a-number"`) {
		t.Fatalf("invalid ArgDefault err = %v", err)
	}
}

func TestCrossPlatformCoverageBindKeyKebabToCamel(t *testing.T) {
	// Declared Bind always wins.
	if got := bindKey(FlagSpec{Name: "disable-ssl-verify", Bind: "disableSSLVerify"}); got != "disableSSLVerify" {
		t.Fatalf("explicit bind = %q", got)
	}
	// Single-word names pass through unchanged.
	if got := bindKey(FlagSpec{Name: "plain"}); got != "plain" {
		t.Fatalf("single-word bind = %q", got)
	}
	// Kebab names camelize mechanically; empty segments are skipped.
	if got := bindKey(FlagSpec{Name: "page-size"}); got != "pageSize" {
		t.Fatalf("kebab bind = %q", got)
	}
	if got := bindKey(FlagSpec{Name: "a--b-c"}); got != "aBC" {
		t.Fatalf("empty-segment bind = %q", got)
	}

	// The derived key is what BuildArgs actually transmits.
	flags := []FlagSpec{{Name: "page-size", Usage: "P", Kind: KindInt}}
	cmd := newTestCommand()
	RegisterFlags(cmd, flags)
	_ = cmd.Flags().Set("page-size", "5")
	args, err := BuildArgs(cmd, flags)
	if err != nil {
		t.Fatal(err)
	}
	if args["pageSize"] != 5 {
		t.Fatalf("camelized args = %#v, want pageSize=5", args)
	}
}

// failingReader returns a non-EOF error so the confirmation read path must
// fail closed instead of treating the answer as a decline.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stdin read failed") }

func TestCrossPlatformCoverageConfirmSafetyReadErrorFailsClosed(t *testing.T) {
	cmd := newTestCommand()
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.SetIn(failingReader{})
	cmd.SetErr(&strings.Builder{})
	err := ConfirmSafety(cmd, testWriteSafety())
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("non-EOF stdin error must be confirmation_required, got %#v", err)
	}
}

func TestCrossPlatformCoverageConfirmSafetyTerminalPromptsBeforeReading(t *testing.T) {
	master, slave, ok := openTestPTY(t)
	if !ok {
		t.Skipf("no native pty on %s: the interactive prompt branch is terminal-only (unix)", runtime.GOOS)
	}
	defer master.Close()
	defer slave.Close()

	// Queue the answer on the master side before ConfirmSafety reads so the
	// canonical-mode read can never block the test.
	if _, err := master.WriteString("yes\n"); err != nil {
		t.Fatalf("write pty answer: %v", err)
	}
	var stderr strings.Builder
	cmd := newTestCommand()
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.SetIn(slave)
	cmd.SetErr(&stderr)
	if err := ConfirmSafety(cmd, testWriteSafety()); err != nil {
		t.Fatalf("terminal yes answer must confirm, got %v", err)
	}
	if !strings.Contains(stderr.String(), "确认继续") {
		t.Fatalf("terminal stdin must print the interactive prompt, got %q", stderr.String())
	}
}

func TestCrossPlatformCoverageConfirmSafetyTerminalProbeHook(t *testing.T) {
	testseam.Swap(t, &stdinIsTerminalFn, func(io.Reader) bool { return true })

	var stderr strings.Builder
	cmd := newTestCommand()
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetErr(&stderr)
	if err := ConfirmSafety(cmd, testWriteSafety()); err != nil {
		t.Fatalf("hooked terminal yes answer must confirm, got %v", err)
	}
	if !strings.Contains(stderr.String(), "确认继续") {
		t.Fatalf("hooked terminal probe must print prompt, got %q", stderr.String())
	}
	// Exercise the real probe's non-file and file paths.
	if stdinIsTerminal(strings.NewReader("x")) {
		t.Fatal("non-file reader must not be a terminal")
	}
	regular, err := os.CreateTemp(t.TempDir(), "notty")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if stdinIsTerminal(regular) {
		t.Fatal("regular file must not be a terminal")
	}
}

func TestCrossPlatformCoverageBoolFlagGettersNilCommand(t *testing.T) {
	if got := boolFlagGetters(nil); got != nil {
		t.Fatalf("boolFlagGetters(nil) = %#v, want nil", got)
	}
}

func TestCrossPlatformCoverageEmbedContractCobraProjection(t *testing.T) {
	cmd := New(Spec{
		Use:                 "shortcut-leaf",
		Short:               "short",
		ParameterProjection: ProjectCobraParameters,
		Flags: []FlagSpec{
			{
				Name: "mode", Usage: "M", Required: true,
				Enum: []string{"a", "b"}, RequiredWhen: "when identity=user",
			},
			{Name: "internal", Usage: "I", Hidden: true, Required: true},
		},
		Safety: testWriteSafety(),
		Contract: ContractDecl{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "desc",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "op"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "s", UseWhen: []string{"u"},
				AvoidWhen: []string{"a"}, Examples: []string{"dws shortcut-leaf"},
			},
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})

	mode := cmd.Flags().Lookup("mode")
	if got := mode.Annotations["dws.schema.required"]; len(got) == 0 || got[0] != "true" {
		t.Fatalf("cobra projection must annotate Required, got %#v", mode.Annotations)
	}
	if got := mode.Annotations["x-cli-enum"]; len(got) != 2 {
		t.Fatalf("cobra projection must annotate Enum, got %#v", got)
	}
	if got := mode.Annotations["dws.schema.required_when"]; len(got) == 0 || got[0] != "when identity=user" {
		t.Fatalf("cobra projection must annotate RequiredWhen, got %#v", got)
	}
	// Hidden flags stay Cobra-only facts: no projection annotations.
	if got := cmd.Flags().Lookup("internal").Annotations["dws.schema.required"]; len(got) != 0 {
		t.Fatalf("hidden flag must not be projected, got %#v", got)
	}
	// The authored ContractDecl still lands as the typed ContractFinal.
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Description != "desc" {
		t.Fatalf("cobra projection must still embed ContractDecl, final=%#v ok=%v", final, ok)
	}
}

func TestCrossPlatformCoverageAttachContractNilAndEmptyGuards(t *testing.T) {
	// Both guards are no-ops: nil command, and a command with no authored decl.
	AttachContract(nil, testWriteSafety(), ContractDecl{Description: "d"}, "s", "l")
	cmd := newTestCommand()
	AttachContract(cmd, testWriteSafety(), ContractDecl{}, "s", "l")
	if contractfinal.HasRuntimeContractFinal(cmd) {
		t.Fatal("empty ContractDecl must not register a ContractFinal")
	}
}

func TestCrossPlatformCoverageAttachContractOverwritesLegacySelectionSources(t *testing.T) {
	cmd := newTestCommand()
	AttachContract(cmd, testWriteSafety(), ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			CLIPath: "dev create", PrimaryCLIPath: "dev create",
		},
		Description: "Declared description for attach coverage",
		Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "op"}},
		Parameters:  []contract.ParamDecl{{Name: "mode", Property: "mode", InterfaceType: "string"}},
		Selection: contract.SelectionSpec{
			AgentSummary:       "Declared summary",
			UseWhen:            []string{"use declared"},
			AvoidWhen:          []string{"avoid declared"},
			Examples:           []string{"dws t"},
			AgentSummarySource: "schema_hints/legacy",
			SourceRefs:         []string{"schema_hints/selection/sample.json"},
			MetadataSource:     "schema_hints",
		},
	}, "short", "long")
	payload, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || payload.Selection == nil {
		t.Fatal("expected ContractFinal selection payload")
	}
	if len(payload.Parameters) != 1 || payload.Parameters[0].Name != "mode" {
		t.Fatalf("Parameters copy = %#v", payload.Parameters)
	}
	sel := payload.Selection
	if sel.AgentSummarySource != "corecmd.ContractDecl" ||
		len(sel.SourceRefs) != 1 || sel.SourceRefs[0] != "corecmd.ContractDecl" ||
		sel.MetadataSource != "corecmd.contract" || sel.Reviewed != nil {
		t.Fatalf("selection sources = %#v", sel)
	}
}

func TestCrossPlatformCoverageAttachContractIdentityCLIPrimaryFillAndAliases(t *testing.T) {
	baseSel := contract.SelectionSpec{
		AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Examples: []string{"dws t"},
	}
	baseIface := &contract.InterfaceSpec{
		Mode: "mcp", Availability: "available",
		Ref: &contract.InterfaceRefSpec{ProductID: "dev", RPCName: "op"},
	}

	primaryOnly := newTestCommand()
	AttachContract(primaryOnly, testWriteSafety(), ContractDecl{
		Description: "desc",
		Interface:   baseIface,
		Selection:   baseSel,
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			PrimaryCLIPath: "dev create",
			Aliases:        []string{"dev alt"},
		},
	}, "short", "long")
	got, ok := contractfinal.RuntimeContractFinal(primaryOnly)
	if !ok || got.Identity == nil {
		t.Fatal("expected Identity after PrimaryCLIPath-only attach")
	}
	if got.Identity.CLIPath != "dev create" || got.Identity.PrimaryCLIPath != "dev create" {
		t.Fatalf("CLI/Primary mutual fill = %#v", got.Identity)
	}
	if len(got.Identity.Aliases) != 1 || got.Identity.Aliases[0] != "dev alt" {
		t.Fatalf("Aliases copy = %#v", got.Identity.Aliases)
	}

	cliOnly := newTestCommand()
	AttachContract(cliOnly, testWriteSafety(), ContractDecl{
		Description: "desc",
		Interface:   baseIface,
		Selection:   baseSel,
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			CLIPath: "dev create",
		},
	}, "short", "long")
	got, ok = contractfinal.RuntimeContractFinal(cliOnly)
	if !ok || got.Identity == nil {
		t.Fatal("expected Identity after CLIPath-only attach")
	}
	if got.Identity.CLIPath != "dev create" || got.Identity.PrimaryCLIPath != "dev create" {
		t.Fatalf("Primary fill from CLIPath = %#v", got.Identity)
	}
}

func TestCrossPlatformCoverageEffectiveSafetySpecZeroValueDefaultsToRead(t *testing.T) {
	got := effectiveSafetySpec(contract.SafetySpec{})
	want := contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
	if got != want {
		t.Fatalf("effectiveSafetySpec(zero) = %+v, want %+v", got, want)
	}
	partial := effectiveSafetySpec(contract.SafetySpec{Effect: " write ", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"})
	if partial.Effect != "write" || partial.Risk != "high" {
		t.Fatalf("effectiveSafetySpec(trim) = %+v", partial)
	}
}

func TestCrossPlatformCoverageEmbedContractSkipsBlankAndHiddenFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "skip-blank-hidden"}
	cmd.Flags().String("visible", "", "visible flag")
	cmd.Flags().String("ghost", "", "hidden flag")
	embedContractIntoSchema(cmd, Spec{
		Use: "skip-blank-hidden",
		Flags: []FlagSpec{
			{Name: "  ", Usage: "blank name is skipped"},
			{Name: "ghost", Usage: "hidden is skipped", Hidden: true},
			{Name: "visible", Usage: "annotated", Required: true},
		},
	})
	anns := cmd.Annotations
	if anns == nil {
		t.Fatal("expected runtime contract annotations")
	}
	for key := range anns {
		if strings.Contains(key, "ghost") {
			t.Fatalf("hidden flag must not be annotated: %s", key)
		}
	}
}
