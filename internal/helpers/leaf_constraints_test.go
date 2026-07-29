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
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func leafConstraintTestSpec(captured *map[string]any) LeafSpec {
	return LeafSpec{
		Use:   "route",
		Short: "路由",
		Tool:  "route_thing",
		Flags: []LeafFlag{
			{Name: "group", Usage: "群 ID", Aliases: []string{"chat"}},
			{Name: "user", Usage: "用户 ID", EnvVar: "DWS_LEAF_CONSTRAINT_TEST_USER"},
			{Name: "channel", Usage: "频道", Default: "main"},
			{Name: "verbose", Usage: "详细输出", Kind: LeafBool, Bind: "verboseOutput"},
			{Name: "tags", Usage: "标签列表", Kind: LeafStringSlice, Aliases: []string{"labels"}, Bind: "tagList"},
		},
		Constraints: []LeafConstraint{
			{Kind: LeafExactlyOne, Flags: []string{"group", "user"}},
			{Kind: LeafMutuallyExclusive, Flags: []string{"verbose", "tags"}},
		},
		Call: func(_ *cobra.Command, _ string, args map[string]any) error {
			*captured = args
			return nil
		},
	}
}

func leafConstraintExecute(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	var captured map[string]any
	cmd := NewLeafCommand(leafConstraintTestSpec(&captured))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return captured, err
}

func TestCrossPlatformCoverageLeafConstraintExactlyOne(t *testing.T) {
	if _, err := leafConstraintExecute(t); err == nil ||
		!strings.Contains(err.Error(), "请指定 --group、--user 之一") {
		t.Fatalf("zero provided err = %v", err)
	}
	if _, err := leafConstraintExecute(t, "--group", "g1", "--user", "u1"); err == nil ||
		!strings.Contains(err.Error(), "参数 --group、--user 只能指定其一（当前指定了 --group、--user）") {
		t.Fatalf("both provided err = %v", err)
	}
	if _, err := leafConstraintExecute(t, "--group", "g1"); err != nil {
		t.Fatalf("exactly one err = %v", err)
	}
}

func TestCrossPlatformCoverageLeafConstraintAliasAndEnvCountAsProvided(t *testing.T) {
	// 隐藏别名 --chat 命中 group：满足 exactly_one。
	if _, err := leafConstraintExecute(t, "--chat", "g1"); err != nil {
		t.Fatalf("alias provided err = %v", err)
	}
	// 环境变量命中 user：同样满足。
	t.Setenv("DWS_LEAF_CONSTRAINT_TEST_USER", "u-env")
	if _, err := leafConstraintExecute(t); err != nil {
		t.Fatalf("env provided err = %v", err)
	}
	// env 提供 user + 显式 group：应触发 exactly_one 冲突。
	if _, err := leafConstraintExecute(t, "--group", "g1"); err == nil ||
		!strings.Contains(err.Error(), "只能指定其一") {
		t.Fatalf("env + explicit err = %v", err)
	}
}

func TestCrossPlatformCoverageLeafConstraintDefaultNotCounted(t *testing.T) {
	// channel 有注册默认值 "main"，但未显式提供时不得参与约束判定：
	// 用仅含 at_least_one(channel, group) 的 spec 验证默认值不算提供。
	var captured map[string]any
	spec := LeafSpec{
		Use:  "pick",
		Tool: "pick_thing",
		Flags: []LeafFlag{
			{Name: "channel", Usage: "频道", Default: "main"},
			{Name: "group", Usage: "群 ID"},
		},
		Constraints: []LeafConstraint{
			{Kind: LeafAtLeastOne, Flags: []string{"channel", "group"}},
		},
		Call: func(_ *cobra.Command, _ string, args map[string]any) error {
			captured = args
			return nil
		},
	}
	cmd := NewLeafCommand(spec)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "请至少指定 --channel、--group 之一") {
		t.Fatalf("default counted as provided: err = %v, captured = %#v", err, captured)
	}
}

func TestCrossPlatformCoverageLeafConstraintMutuallyExclusive(t *testing.T) {
	if _, err := leafConstraintExecute(t, "--group", "g1", "--verbose", "--tags", "a,b"); err == nil ||
		!strings.Contains(err.Error(), "参数 --verbose、--tags 互斥，只能指定其一（当前指定了 --verbose、--tags）") {
		t.Fatalf("mutually exclusive err = %v", err)
	}
	// 各自单独提供均合法。
	if _, err := leafConstraintExecute(t, "--group", "g1", "--verbose"); err != nil {
		t.Fatalf("verbose alone err = %v", err)
	}
	if _, err := leafConstraintExecute(t, "--group", "g1", "--tags", "a"); err != nil {
		t.Fatalf("tags alone err = %v", err)
	}
	// 空白元素的列表视为未提供，不触发互斥。
	if _, err := leafConstraintExecute(t, "--group", "g1", "--verbose", "--tags", " "); err != nil {
		t.Fatalf("blank slice counted as provided: err = %v", err)
	}
}

func TestCrossPlatformCoverageLeafBoolAndSliceArgs(t *testing.T) {
	captured, err := leafConstraintExecute(t, "--group", "g1", "--verbose=false")
	if err != nil {
		t.Fatal(err)
	}
	if captured["verboseOutput"] != false {
		t.Fatalf("explicit false not delivered: %#v", captured)
	}

	captured, err = leafConstraintExecute(t, "--group", "g1", "--tags", "a, ,b")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured["tagList"], []string{"a", "b"}) {
		t.Fatalf("tagList = %#v, want trimmed non-empty elements", captured["tagList"])
	}
	if _, ok := captured["verboseOutput"]; ok {
		t.Fatalf("unchanged bool leaked into args: %#v", captured)
	}

	// 别名 --labels 提供列表同样入参。
	captured, err = leafConstraintExecute(t, "--group", "g1", "--labels", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured["tagList"], []string{"x"}) {
		t.Fatalf("alias slice = %#v", captured["tagList"])
	}
}

func TestCrossPlatformCoverageLeafBoolSliceRequired(t *testing.T) {
	var captured map[string]any
	spec := LeafSpec{
		Use:  "need",
		Tool: "need_thing",
		Flags: []LeafFlag{
			{Name: "confirm", Usage: "确认", Kind: LeafBool, Required: true},
			{Name: "ids", Usage: "ID 列表", Kind: LeafStringSlice, Required: true},
		},
		Call: func(_ *cobra.Command, _ string, args map[string]any) error {
			captured = args
			return nil
		},
	}
	run := func(args ...string) error {
		cmd := NewLeafCommand(spec)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("--ids", "a"); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("missing bool err = %v", err)
	}
	if err := run("--confirm", "--ids", " "); err == nil || !strings.Contains(err.Error(), "ids") {
		t.Fatalf("blank slice err = %v", err)
	}
	if err := run("--confirm=false", "--ids", "a"); err != nil {
		t.Fatalf("explicit false should satisfy required: %v", err)
	}
	if captured["confirm"] != false || !reflect.DeepEqual(captured["ids"], []string{"a"}) {
		t.Fatalf("captured = %#v", captured)
	}
}

func TestCrossPlatformCoverageLeafConstraintSchemaAndHelp(t *testing.T) {
	var captured map[string]any
	cmd := NewLeafCommand(leafConstraintTestSpec(&captured))
	encoded := cmd.Annotations["dws.schema.constraints"]
	if !strings.Contains(encoded, `"require_one_of":[["group","user"]]`) {
		t.Fatalf("schema constraints missing require_one_of: %s", encoded)
	}
	if !strings.Contains(encoded, `["group","user"]`) || !strings.Contains(encoded, `["verbose","tags"]`) {
		t.Fatalf("schema constraints incomplete: %s", encoded)
	}
	if !strings.Contains(cmd.Long, "参数约束：") ||
		!strings.Contains(cmd.Long, "--group、--user 必须且只能指定一个") ||
		!strings.Contains(cmd.Long, "--verbose、--tags 互斥，最多指定一个") {
		t.Fatalf("constraint help missing: %q", cmd.Long)
	}

	withDesc := LeafSpec{
		Use:  "desc",
		Tool: "desc_thing",
		Flags: []LeafFlag{
			{Name: "a", Usage: "A"},
			{Name: "b", Usage: "B"},
		},
		Constraints: []LeafConstraint{
			{Kind: LeafAtLeastOne, Flags: []string{"a", "b"}, Description: "自定义文案"},
		},
	}
	if long := NewLeafCommand(withDesc).Long; !strings.Contains(long, "  - 自定义文案") {
		t.Fatalf("custom description missing: %q", long)
	}
	atLeast := LeafSpec{
		Use:  "least",
		Tool: "least_thing",
		Flags: []LeafFlag{
			{Name: "a", Usage: "A"},
			{Name: "b", Usage: "B"},
		},
		Constraints: []LeafConstraint{{Kind: LeafAtLeastOne, Flags: []string{"a", "b"}}},
	}
	if long := NewLeafCommand(atLeast).Long; !strings.Contains(long, "--a、--b 至少指定一个") {
		t.Fatalf("at_least_one help missing: %q", long)
	}
}

func TestCrossPlatformCoverageLeafConstraintDeclPanics(t *testing.T) {
	mustPanic := func(name string, spec LeafSpec, needle string) {
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
		NewLeafCommand(spec)
	}
	base := []LeafFlag{{Name: "a", Usage: "A"}, {Name: "b", Usage: "B"}}
	mustPanic("unknown kind", LeafSpec{
		Use: "x", Tool: "t", Flags: base,
		Constraints: []LeafConstraint{{Kind: "bogus", Flags: []string{"a", "b"}}},
	}, "unknown constraint kind")
	mustPanic("too few flags", LeafSpec{
		Use: "x", Tool: "t", Flags: base,
		Constraints: []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"a"}}},
	}, "needs at least two flags")
	mustPanic("undeclared flag", LeafSpec{
		Use: "x", Tool: "t", Flags: base,
		Constraints: []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"a", "missing"}}},
	}, "references undeclared flag")
}

func TestCrossPlatformCoverageLeafConstraintRunsBeforeValidateHook(t *testing.T) {
	hookRan := false
	spec := LeafSpec{
		Use:  "order",
		Tool: "order_thing",
		Flags: []LeafFlag{
			{Name: "a", Usage: "A"},
			{Name: "b", Usage: "B"},
		},
		Constraints: []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"a", "b"}}},
		Validate: func(*cobra.Command, []string) error {
			hookRan = true
			return nil
		},
		Call: func(*cobra.Command, string, map[string]any) error { return nil },
	}
	cmd := NewLeafCommand(spec)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Fatal("constraint should fail before Validate")
	}
	if hookRan {
		t.Fatal("Validate hook ran despite constraint failure")
	}
}

// TestCrossPlatformCoverageLeafSpecCorePaths exercises the LeafSpec paths that
// the framework shares between constrained and unconstrained leaves
// (MarkRequired, RunE escape hatch, Server routing, LeafInt assembly, Transform
// and EnvVar-backed required hints). leaf_test.go asserts the same behaviour but
// its names sit outside the platform coverage runner's filter.
func TestCrossPlatformCoverageLeafSpecCorePaths(t *testing.T) {
	markRequired := NewLeafCommand(LeafSpec{
		Use:   "mark",
		Tool:  "mark_thing",
		Flags: []LeafFlag{{Name: "id", Usage: "ID", MarkRequired: true}},
	})
	if ann := markRequired.Flags().Lookup("id").Annotations[cobra.BashCompOneRequiredFlag]; len(ann) == 0 {
		t.Fatal("MarkRequired did not reach cobra")
	}

	escaped := false
	escape := NewLeafCommand(LeafSpec{
		Use:  "escape",
		Tool: "escape_thing",
		RunE: func(*cobra.Command, []string) error {
			escaped = true
			return nil
		},
	})
	escape.SetArgs(nil)
	if err := escape.Execute(); err != nil || !escaped {
		t.Fatalf("RunE escape hatch not used: err = %v", err)
	}

	caller := &guardedMutationCaller{}
	serverRouted := func() error {
		previous := deps
		t.Cleanup(func() { deps = previous })
		InitDeps(caller)
		deps.Out.w = io.Discard
		cmd := NewLeafCommand(LeafSpec{
			Use:    "route-server",
			Tool:   "route_tool",
			Server: "im",
			Flags: []LeafFlag{
				{Name: "count", Usage: "数量", Kind: LeafInt, Bind: "count"},
				{Name: "scope", Usage: "范围", ArgDefault: "ALL", Bind: "scope"},
				{Name: "note", Usage: "备注", OmitEmpty: true},
			},
		})
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs([]string{"--count", "3"})
		return cmd.Execute()
	}
	if err := serverRouted(); err != nil {
		t.Fatalf("server routing err = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].productID != "im" || caller.calls[0].toolName != "route_tool" {
		t.Fatalf("server routing call = %#v", caller.calls)
	}
	if caller.calls[0].args["count"] != 3 || caller.calls[0].args["scope"] != "ALL" {
		t.Fatalf("server routing args = %#v", caller.calls[0].args)
	}
	if _, ok := caller.calls[0].args["note"]; ok {
		t.Fatalf("OmitEmpty flag leaked: %#v", caller.calls[0].args)
	}

	intSpec := LeafSpec{
		Use:   "count",
		Tool:  "count_thing",
		Flags: []LeafFlag{{Name: "n", Usage: "N", Kind: LeafInt, EnvVar: "DWS_LEAF_CORE_TEST_N"}},
		Call:  func(*cobra.Command, string, map[string]any) error { return nil },
	}
	t.Setenv("DWS_LEAF_CORE_TEST_N", "not-a-number")
	badInt := NewLeafCommand(intSpec)
	badInt.SilenceErrors = true
	badInt.SilenceUsage = true
	badInt.SetArgs(nil)
	if err := badInt.Execute(); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("invalid env integer err = %v", err)
	}

	transformFail := NewLeafCommand(LeafSpec{
		Use:  "transform",
		Tool: "transform_thing",
		Flags: []LeafFlag{{Name: "raw", Usage: "RAW", Transform: func(string) (any, error) {
			return nil, errTransformTest
		}}},
		Call: func(*cobra.Command, string, map[string]any) error { return nil },
	})
	transformFail.SilenceErrors = true
	transformFail.SilenceUsage = true
	transformFail.SetArgs([]string{"--raw", "x"})
	if err := transformFail.Execute(); err == nil || !strings.Contains(err.Error(), "transform failed") {
		t.Fatalf("transform error not surfaced: %v", err)
	}

	hinted := NewLeafCommand(LeafSpec{
		Use:  "hint",
		Tool: "hint_thing",
		Flags: []LeafFlag{{
			Name: "token", Usage: "令牌", Required: true,
			EnvVar: "DWS_LEAF_CORE_TEST_TOKEN", RequiredHint: "flag --token is required (or set DWS_LEAF_CORE_TEST_TOKEN)",
		}},
		Call: func(*cobra.Command, string, map[string]any) error { return nil },
	})
	hinted.SilenceErrors = true
	hinted.SilenceUsage = true
	hinted.SetArgs(nil)
	if err := hinted.Execute(); err == nil || !strings.Contains(err.Error(), "DWS_LEAF_CORE_TEST_TOKEN") {
		t.Fatalf("required hint err = %v", err)
	}

	// 默认 product 路由（无 Server）+ LeafInt Required（走 leafHasEffectiveValue
	// 的 LeafInt 分支）+ Transform 返回 (nil,nil) 跳过键。
	defaultRouted := func() error {
		previous := deps
		t.Cleanup(func() { deps = previous })
		oldArgs := os.Args
		os.Args = []string{"dws", "chat"}
		t.Cleanup(func() { os.Args = oldArgs })
		InitDeps(caller)
		deps.Out.w = io.Discard
		caller.calls = nil
		cmd := NewLeafCommand(LeafSpec{
			Use:  "default-route",
			Tool: "im_default_tool",
			Flags: []LeafFlag{
				{Name: "n", Usage: "N", Kind: LeafInt, Required: true, Bind: "n"},
				{Name: "opt", Usage: "OPT", Transform: func(string) (any, error) { return nil, nil }},
			},
		})
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs([]string{"--n", "5", "--opt", "ignored"})
		return cmd.Execute()
	}
	if err := defaultRouted(); err != nil {
		t.Fatalf("default route err = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("default route call = %#v", caller.calls)
	}
	if caller.calls[0].args["n"] != 5 {
		t.Fatalf("int required arg = %#v", caller.calls[0].args)
	}
	if _, ok := caller.calls[0].args["opt"]; ok {
		t.Fatalf("Transform nil should skip key: %#v", caller.calls[0].args)
	}
	// LeafInt Required 缺失（值为 0）应报缺参。
	missingInt := NewLeafCommand(LeafSpec{
		Use:   "need-int",
		Tool:  "need_int_tool",
		Flags: []LeafFlag{{Name: "n", Usage: "N", Kind: LeafInt, Required: true}},
		Call:  func(*cobra.Command, string, map[string]any) error { return nil },
	})
	missingInt.SilenceErrors = true
	missingInt.SilenceUsage = true
	missingInt.SetArgs(nil)
	if err := missingInt.Execute(); err == nil || !strings.Contains(err.Error(), "n") {
		t.Fatalf("missing int required err = %v", err)
	}
	// 带 EnvVar 但 RequiredHint 为空：走默认 'flag --x is required' 文案。
	envDefaultHint := NewLeafCommand(LeafSpec{
		Use:   "env-default-hint",
		Tool:  "env_hint_tool",
		Flags: []LeafFlag{{Name: "key", Usage: "KEY", Required: true, EnvVar: "DWS_LEAF_CORE_TEST_MISSING"}},
		Call:  func(*cobra.Command, string, map[string]any) error { return nil },
	})
	envDefaultHint.SilenceErrors = true
	envDefaultHint.SilenceUsage = true
	envDefaultHint.SetArgs(nil)
	if err := envDefaultHint.Execute(); err == nil || !strings.Contains(err.Error(), "flag --key is required") {
		t.Fatalf("env default hint err = %v", err)
	}
}

var errTransformTest = errors.New("transform failed")

func leafRiskSpec(risk LeafRisk, called *bool) LeafSpec {
	return LeafSpec{
		Use:   "danger",
		Short: "危险",
		Tool:  "danger_thing",
		Risk:  risk,
		Flags: []LeafFlag{{Name: "id", Usage: "ID"}},
		Call: func(*cobra.Command, string, map[string]any) error {
			*called = true
			return nil
		},
	}
}

func leafRiskRun(t *testing.T, risk LeafRisk, stdin string, args ...string) (bool, error) {
	t.Helper()
	called := false
	cmd := NewLeafCommand(leafRiskSpec(risk, &called))
	// 注入根级 --yes/--dry-run 持久 flag，模拟真实根命令。
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(args)
	return called, cmd.Execute()
}

func TestCrossPlatformCoverageLeafRiskReadNeverPrompts(t *testing.T) {
	// 只读：无 stdin 也直接派发。
	if called, err := leafRiskRun(t, LeafRiskRead, "", "--id", "x"); err != nil || !called {
		t.Fatalf("read risk err = %v called = %v", err, called)
	}
	// 空 Risk 等价只读。
	if called, err := leafRiskRun(t, "", "", "--id", "x"); err != nil || !called {
		t.Fatalf("empty risk err = %v called = %v", err, called)
	}
}

func TestCrossPlatformCoverageLeafRiskWriteConfirmation(t *testing.T) {
	// 拒绝：不派发，返回取消错误。
	called, err := leafRiskRun(t, LeafRiskWrite, "no\n", "--id", "x")
	if called {
		t.Fatal("declined write should not dispatch")
	}
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined write err = %v", err)
	}
	// 同意：派发。
	if called, err := leafRiskRun(t, LeafRiskWrite, "yes\n", "--id", "x"); err != nil || !called {
		t.Fatalf("confirmed write err = %v called = %v", err, called)
	}
	// 高危同样走确认链。
	if called, err := leafRiskRun(t, LeafRiskHighWrite, "y\n", "--id", "x"); err != nil || !called {
		t.Fatalf("confirmed high-write err = %v called = %v", err, called)
	}
}

func TestCrossPlatformCoverageLeafRiskYesAndDryRunBypass(t *testing.T) {
	// --yes 跳过提示直接派发（无 stdin）。
	if called, err := leafRiskRun(t, LeafRiskHighWrite, "", "--id", "x", "--yes"); err != nil || !called {
		t.Fatalf("--yes bypass err = %v called = %v", err, called)
	}
	// --dry-run 跳过提示直接派发（无 stdin）。
	if called, err := leafRiskRun(t, LeafRiskWrite, "", "--id", "x", "--dry-run"); err != nil || !called {
		t.Fatalf("--dry-run bypass err = %v called = %v", err, called)
	}
}

func TestCrossPlatformCoverageLeafYesFlagAndIntEdges(t *testing.T) {
	if leafYesFlag(nil) {
		t.Fatal("nil cmd should not report --yes")
	}

	// Required LeafInt 的 env 值不可解析：leafHasEffectiveValue 视为已提供
	// （err→true 分支），required 通过后由 leafArgs 报 invalid integer。
	t.Setenv("DWS_LEAF_INT_EDGE", "not-an-int")
	badInt := NewLeafCommand(LeafSpec{
		Use:   "int-edge",
		Tool:  "int_edge_tool",
		Flags: []LeafFlag{{Name: "n", Usage: "N", Kind: LeafInt, Required: true, EnvVar: "DWS_LEAF_INT_EDGE"}},
		Call:  func(*cobra.Command, string, map[string]any) error { return nil },
	})
	badInt.SilenceErrors = true
	badInt.SilenceUsage = true
	badInt.SetArgs(nil)
	if err := badInt.Execute(); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("unparseable int env err = %v", err)
	}

	// 写风险但全局无 --yes flag 注册：leafYesFlag 各 getter 均报错走兜底 false，
	// 于是进入提示；stdin "no" 取消。
	called := false
	noYes := NewLeafCommand(leafRiskSpec(LeafRiskWrite, &called))
	noYes.SilenceErrors = true
	noYes.SilenceUsage = true
	noYes.SetIn(strings.NewReader("no\n"))
	noYes.SetArgs([]string{"--id", "x"})
	if err := noYes.Execute(); err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("no-yes-flag cancel err = %v", err)
	}
	if called {
		t.Fatal("declined write dispatched despite no --yes flag")
	}
}
