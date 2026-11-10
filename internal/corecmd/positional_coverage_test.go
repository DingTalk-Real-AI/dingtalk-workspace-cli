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
	"bytes"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// positionalProbeContract 提供通过 validateContractDecl 的最小完整声明，
// 让构造与执行抵达位置参数框架的分支。
func positionalProbeContract(name string) ContractDecl {
	return ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "test", Name: name, CanonicalPath: "test." + name,
			CLIPath: "probe", PrimaryCLIPath: "probe",
		},
		Description: "positional probe",
		Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "probe"},
		Selection: contract.SelectionSpec{
			AgentSummary: "probe",
			UseWhen:      []string{"probe"},
			AvoidWhen:    []string{"probe"},
			Examples:     []string{"dws probe"},
		},
	}
}

// TestCrossPlatformCoveragePositionalDeclPanics 覆盖 validatePositionalDecls
// 的声明期拒绝分支（空 Name、非法 Kind、bind key 冲突、ConstParams 收录）。
func TestCrossPlatformCoveragePositionalDeclPanics(t *testing.T) {
	panics := []struct {
		name string
		spec Spec
		want string
	}{
		{
			name: "empty positional name",
			spec: Spec{Use: "bad", Short: "x",
				Positionals: []PositionalSpec{{Name: "  ", Kind: KindString}},
				Contract:    positionalProbeContract("bad_empty"),
				Invoke:      func(*Ctx, map[string]any) error { return nil }},
			want: "empty Name",
		},
		{
			name: "unsupported kind",
			spec: Spec{Use: "bad", Short: "x",
				Positionals: []PositionalSpec{{Name: "id", Kind: FlagKind(999)}},
				Contract:    positionalProbeContract("bad_kind"),
				Invoke:      func(*Ctx, map[string]any) error { return nil }},
			want: "only KindString/KindInt",
		},
		{
			name: "bind key conflicts with flag",
			spec: Spec{Use: "bad", Short: "x",
				Flags:       []FlagSpec{{Name: "app-id", Bind: "appId"}},
				Positionals: []PositionalSpec{{Name: "appId", Kind: KindInt, Bind: "appId"}},
				Contract:    positionalProbeContract("bad_conflict_flag"),
				Invoke:      func(*Ctx, map[string]any) error { return nil }},
			want: "conflicts with a flag/ConstParams key",
		},
		{
			name: "bind key conflicts with ConstParams",
			spec: Spec{Use: "bad", Short: "x",
				ConstParams: map[string]any{"appId": true},
				Positionals: []PositionalSpec{{Name: "appId", Kind: KindInt, Bind: "appId"}},
				Contract:    positionalProbeContract("bad_conflict_const"),
				Invoke:      func(*Ctx, map[string]any) error { return nil }},
			want: "conflicts with a flag/ConstParams key",
		},
	}
	for _, tc := range panics {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Fatalf("panic = %v, want containing %q", r, tc.want)
				}
			}()
			_ = New(tc.spec)
		})
	}
}

func TestCrossPlatformCoverageOptionalKindIntPositionalOmission(t *testing.T) {
	var got map[string]any
	newProbe := func() *cobra.Command {
		return newPositionalProbeLeaf(t,
			[]PositionalSpec{{Name: "app_id", Kind: KindInt, Bind: "appId"}},
			nil, func(args map[string]any) error { got = args; return nil })
	}
	if err := executePositionalProbe(t, newProbe()); err != nil {
		t.Fatalf("omitted optional int positional: %v", err)
	}
	if _, bound := got["appId"]; bound {
		t.Fatalf("omitted optional int positional must stay unbound: %#v", got)
	}
	if err := executePositionalProbe(t, newProbe(), "5005426001"); err != nil {
		t.Fatalf("provided optional int positional: %v", err)
	}
	if got["appId"] != int64(5005426001) {
		t.Fatalf("appId = %#v, want int64", got["appId"])
	}
	// 显式传空串与整体省略同义：可选 KindInt 按有效空值判定省略，保持不绑定。
	if err := executePositionalProbe(t, newProbe(), ""); err != nil {
		t.Fatalf("empty token optional int positional: %v", err)
	}
	if _, bound := got["appId"]; bound {
		t.Fatalf("empty token optional int positional must stay unbound: %#v", got)
	}
}

// newPositionalProbeLeaf 构造一个带位置参数的探针叶子；call 捕获 toolArgs。
func newPositionalProbeLeaf(t *testing.T, positionals []PositionalSpec, constParams map[string]any, call func(args map[string]any) error) *cobra.Command {
	t.Helper()
	cmd := New(Spec{
		Use:         "probe <note>",
		Short:       "probe",
		Positionals: positionals,
		ConstParams: constParams,
		Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:    positionalProbeContract("probe"),
		Invoke: func(c *Ctx, toolArgs map[string]any) error {
			if call != nil {
				return call(toolArgs)
			}
			return nil
		},
	})
	return cmd
}

func executePositionalProbe(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cmd)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"probe"}, args...))
	// 必须走 prepared 执行路径：裸 root.Execute 会跳过框架安装的
	// ValidationErrorFunc/FlagErrorFunc，位置参数 arity 与非法 flag 的
	// 校验分类（validation / 退出码 3）在测试中失真。
	return ExecuteForTest(root)
}

// TestCrossPlatformCoveragePositionalValueBinding 覆盖 buildPositionalArgs 的
// 取值分支（正整数校验、必填为空、可选为空跳过、Trim 后绑定、无 Bind 回退
// Name）以及 RunE/preflight 的绑定与错误返回路径。
func TestCrossPlatformCoveragePositionalValueBinding(t *testing.T) {
	t.Run("int positional binds int64", func(t *testing.T) {
		var got map[string]any
		cmd := newPositionalProbeLeaf(t,
			[]PositionalSpec{{Name: "app_id", Kind: KindInt, Required: true, Trim: true, Bind: "appId"}},
			nil, func(args map[string]any) error { got = args; return nil })
		if err := executePositionalProbe(t, cmd, "5005426001"); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got["appId"] != int64(5005426001) {
			t.Fatalf("appId = %#v", got["appId"])
		}
	})

	t.Run("int positional rejects non-positive", func(t *testing.T) {
		for _, bad := range []string{"0", "abc"} {
			// 每次执行构造新树：prepared 命令不可复用（Cobra flag 状态与
			// prepared 标注都按单次执行设计）。
			cmd := newPositionalProbeLeaf(t,
				[]PositionalSpec{{Name: "app_id", Kind: KindInt, Required: true, Bind: "appId"}},
				nil, nil)
			if err := executePositionalProbe(t, cmd, bad); err == nil || !strings.Contains(err.Error(), "必须是正整数") {
				t.Fatalf("args %q error = %v, want 正整数 validation", bad, err)
			}
		}
	})

	t.Run("required string rejects empty", func(t *testing.T) {
		cmd := newPositionalProbeLeaf(t,
			[]PositionalSpec{{Name: "mode", Kind: KindString, Required: true, Trim: true, Bind: "mode"}},
			nil, nil)
		if err := executePositionalProbe(t, cmd, ""); err == nil || !strings.Contains(err.Error(), "缺少必填参数 mode") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("omit trailing optional positional entirely", func(t *testing.T) {
		var got map[string]any
		cmd := newPositionalProbeLeaf(t,
			[]PositionalSpec{
				{Name: "mode", Kind: KindString, Required: true, Trim: true, Bind: "mode"},
				{Name: "note", Kind: KindString, Trim: true, Bind: "note"},
			},
			nil, func(args map[string]any) error { got = args; return nil })
		// 只传必填参数，尾部可选整体省略：Schema required=false 与 CLI arity 一致。
		if err := executePositionalProbe(t, cmd, "alpha"); err != nil {
			t.Fatalf("omit optional: %v", err)
		}
		if got["mode"] != "alpha" {
			t.Fatalf("mode = %#v", got["mode"])
		}
		if _, bound := got["note"]; bound {
			t.Fatalf("omitted optional positional must stay unbound: %#v", got)
		}
	})

	t.Run("missing required positional stays validation-classified", func(t *testing.T) {
		cmd := newPositionalProbeLeaf(t,
			[]PositionalSpec{{Name: "mode", Kind: KindString, Required: true, Trim: true, Bind: "mode"}},
			nil, nil)
		err := executePositionalProbe(t, cmd)
		if err == nil {
			t.Fatal("missing required positional must fail")
		}
		var appErr *apperrors.Error
		if !stderrors.As(err, &appErr) {
			t.Fatalf("error type = %T, want *errors.Error (validation classification)", err)
		}
	})

	t.Run("non-trailing optional positional rejected at declaration", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil || !strings.Contains(fmt.Sprint(r), "required positional cannot follow an optional one") {
				t.Fatalf("panic = %v, want trailing-optional rule", r)
			}
		}()
		_ = newPositionalProbeLeaf(t,
			[]PositionalSpec{
				{Name: "note", Kind: KindString, Bind: "note"},
				{Name: "mode", Kind: KindString, Required: true, Bind: "mode"},
			},
			nil, nil)
	})

	t.Run("optional empty skipped and trim binds without Bind", func(t *testing.T) {
		var got map[string]any
		// 每次执行构造新树：prepared 命令不可复用。
		newProbe := func() *cobra.Command {
			return newPositionalProbeLeaf(t,
				[]PositionalSpec{{Name: "note", Kind: KindString, Trim: true}}, // 无 Bind：key 回退 Name
				map[string]any{"probeConst": true}, func(args map[string]any) error { got = args; return nil })
		}
		if err := executePositionalProbe(t, newProbe(), ""); err != nil {
			t.Fatalf("optional empty: %v", err)
		}
		if _, bound := got["note"]; bound {
			t.Fatalf("empty optional positional must be skipped, got %#v", got)
		}
		if got["probeConst"] != true {
			t.Fatalf("ConstParams = %#v", got["probeConst"])
		}
		if err := executePositionalProbe(t, newProbe(), "  hello  "); err != nil {
			t.Fatalf("trimmed bind: %v", err)
		}
		if got["note"] != "hello" {
			t.Fatalf("note = %#v, want trimmed hello", got["note"])
		}
	})
}
