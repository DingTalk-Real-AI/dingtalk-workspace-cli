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
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// leafDispatchCaller 是 leaf 默认派发路径的 fake ToolCaller。
type leafDispatchCaller struct {
	dryRun    bool
	productID string
	toolName  string
}

func (c *leafDispatchCaller) CallTool(_ context.Context, productID, toolName string, _ map[string]any) (*edition.ToolResult, error) {
	c.productID = productID
	c.toolName = toolName
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (c *leafDispatchCaller) Format() string { return "json" }
func (c *leafDispatchCaller) DryRun() bool   { return c.dryRun }
func (c *leafDispatchCaller) Fields() string { return "" }
func (c *leafDispatchCaller) JQ() string     { return "" }

func withLeafDispatchCaller(t *testing.T, dryRun bool) *leafDispatchCaller {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	caller := &leafDispatchCaller{dryRun: dryRun}
	InitDeps(caller)
	deps.Out.w = io.Discard
	return caller
}

// TestLeafCommandDefaultDispatch 覆盖默认 callMCPTool 派发（无 Call/Server）。
// DryRun caller 在路由前早返回，验证 RunE 走到默认派发语句且无错误。
func TestLeafCommandDefaultDispatch(t *testing.T) {
	withLeafDispatchCaller(t, true)
	cmd := NewLeafCommand(LeafSpec{
		Use: "get", Tool: "get_thing",
		Flags: []LeafFlag{{Name: "id", Usage: "ID", Bind: "id"}},
	})
	if err := cmd.Flags().Set("id", "x"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() = %v, want nil via dry-run dispatch", err)
	}
}

// TestLeafCommandServerDispatch 覆盖显式 Server 路由分支。
func TestLeafCommandServerDispatch(t *testing.T) {
	caller := withLeafDispatchCaller(t, false)
	cmd := NewLeafCommand(LeafSpec{
		Use: "get", Server: "doc", Tool: "get_document",
		Flags: []LeafFlag{{Name: "id", Usage: "ID", Bind: "docId"}},
	})
	if err := cmd.Flags().Set("id", "d1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() = %v", err)
	}
	if caller.productID != "doc" || caller.toolName != "get_document" {
		t.Fatalf("dispatched to %s/%s, want doc/get_document", caller.productID, caller.toolName)
	}
}

// TestLeafCommandTransformErrorAborts 覆盖 RunE 中 leafArgs 错误传播。
func TestLeafCommandTransformErrorAborts(t *testing.T) {
	withLeafDispatchCaller(t, true)
	cmd := NewLeafCommand(LeafSpec{
		Use: "get", Tool: "get_thing",
		Flags: []LeafFlag{{
			Name: "when", Usage: "时间", Bind: "when",
			Transform: func(string) (any, error) { return nil, errors.New("bad time format") },
		}},
	})
	if err := cmd.Flags().Set("when", "not-a-time"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "bad time format") {
		t.Fatalf("RunE() = %v, want transform error", err)
	}
}

// TestLeafValidateRequiredEnvDefaultHint 覆盖 EnvVar Required 未配置
// RequiredHint 时的默认报错文案。
func TestLeafValidateRequiredEnvDefaultHint(t *testing.T) {
	spec := LeafSpec{
		Use: "send", Tool: "send_thing",
		Flags: []LeafFlag{{Name: "token", Usage: "令牌", Required: true, EnvVar: "DWS_LEAF_TEST_HINTLESS"}},
	}
	cmd := NewLeafCommand(spec)
	err := corecmd.ValidateRequired(cmd, spec.Flags)
	if err == nil || err.Error() != "flag --token is required" {
		t.Fatalf("leafValidateRequired() = %v, want default hint", err)
	}
}

// ── CR 探针回归：A1/A2/A3 ──────────────────────────────────────────────

// TestLeafIntRequiredAcceptsExplicitValue（A1 探针）：LeafInt + Required
// 传了值必须通过校验，不得报「missing required flag(s)」。
func TestLeafIntRequiredAcceptsExplicitValue(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "n", Usage: "数量", Kind: LeafInt, Required: true, Bind: "count"}},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("n", "5"); err != nil {
		t.Fatal(err)
	}
	if err := corecmd.ValidateRequired(cmd, spec.Flags); err != nil {
		t.Fatalf("leafValidateRequired() = %v, want nil for --n 5", err)
	}
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatal(err)
	}
	if got := args["count"]; got != 5 {
		t.Fatalf("count = %v (%T), want int 5", got, got)
	}
	// 未提供值时仍按主 flag 名报缺失。
	missing := NewLeafCommand(spec)
	if err := corecmd.ValidateRequired(missing, spec.Flags); err == nil || !strings.Contains(err.Error(), "missing required flag(s): --n") {
		t.Fatalf("leafValidateRequired() = %v, want missing --n", err)
	}
}

// TestLeafIntAliasAndEnvFallback（A2 探针）：整型 flag 的别名与 env 回退
// 必须生效，不得静默丢值。
func TestLeafIntAliasAndEnvFallback(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "page-size", Usage: "分页大小", Kind: LeafInt, Aliases: []string{"limit"}, EnvVar: "DWS_LEAF_TEST_PAGE_SIZE", Bind: "pageSize"}},
	}
	// 别名提供值。
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("limit", "7"); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatal(err)
	}
	if got := args["pageSize"]; got != 7 {
		t.Fatalf("pageSize = %v (%T), want int 7 from alias", got, got)
	}
	// env 提供值。
	t.Setenv("DWS_LEAF_TEST_PAGE_SIZE", "9")
	cmd = NewLeafCommand(spec)
	args, err = corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatal(err)
	}
	if got := args["pageSize"]; got != 9 {
		t.Fatalf("pageSize = %v (%T), want int 9 from env", got, got)
	}
	// env 值不可解析必须报错而非静默丢弃。
	t.Setenv("DWS_LEAF_TEST_PAGE_SIZE", "not-a-number")
	cmd = NewLeafCommand(spec)
	if _, err := corecmd.BuildArgs(cmd, spec.Flags); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("leafArgs() = %v, want parse error for garbage env", err)
	}
}

// TestLeafIntAliasRegisteredTyped：整型 flag 的别名按 Kind 注册且回退生效。
func TestLeafIntAliasRegisteredTyped(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "cursor", Usage: "游标", Kind: LeafInt, Aliases: []string{"offset"}, Bind: "cursor"}},
	}
	cmd := NewLeafCommand(spec)
	if f := cmd.Flags().Lookup("offset"); f == nil || f.Value.Type() != "int" {
		t.Fatalf("alias offset = %+v, want registered as int", f)
	}
	if err := cmd.Flags().Set("offset", "11"); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatal(err)
	}
	if got := args["cursor"]; got != 11 {
		t.Fatalf("cursor = %v (%T), want int 11 from alias", got, got)
	}
}

// TestLeafDefaultDoesNotShadowFallback（A3 探针）：注册默认值不得遮蔽
// 别名与环境变量，只能作为链尾兜底。
func TestLeafDefaultDoesNotShadowFallback(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "type", Usage: "类型", Default: "ALL", Aliases: []string{"kind"}, EnvVar: "DWS_LEAF_TEST_TYPE", Bind: "type"}},
	}
	// env 覆盖注册默认值。
	t.Setenv("DWS_LEAF_TEST_TYPE", "from-env")
	cmd := NewLeafCommand(spec)
	if got := corecmd.EffectiveValue(cmd, spec.Flags[0]); got != "from-env" {
		t.Fatalf("effective = %q, want env to beat registered default", got)
	}
	// 别名覆盖 env 与默认值。
	if err := cmd.Flags().Set("kind", "from-alias"); err != nil {
		t.Fatal(err)
	}
	if got := corecmd.EffectiveValue(cmd, spec.Flags[0]); got != "from-alias" {
		t.Fatalf("effective = %q, want alias to beat env", got)
	}
	// 显式主 flag 最高优先。
	if err := cmd.Flags().Set("type", "explicit"); err != nil {
		t.Fatal(err)
	}
	if got := corecmd.EffectiveValue(cmd, spec.Flags[0]); got != "explicit" {
		t.Fatalf("effective = %q, want explicit primary to win", got)
	}
	// 全部缺席时回落注册默认值。
	bare := NewLeafCommand(spec)
	t.Setenv("DWS_LEAF_TEST_TYPE", "")
	if got := corecmd.EffectiveValue(bare, spec.Flags[0]); got != "ALL" {
		t.Fatalf("effective = %q, want registered default as tail fallback", got)
	}
}

// TestLeafIntRequiredExplicitZeroReportsMissing：Required 判定与 leafArgs 入参
// 判定必须一致——LeafInt 显式 0 不会入参，因此 Required 校验也须报缺失，
// 不允许「校验通过但入参缺席」的裂缝。
func TestLeafIntRequiredExplicitZeroReportsMissing(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "n", Usage: "数量", Kind: LeafInt, Required: true, Bind: "count"}},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("n", "0"); err != nil {
		t.Fatal(err)
	}
	if err := corecmd.ValidateRequired(cmd, spec.Flags); err == nil || !strings.Contains(err.Error(), "missing required flag(s): --n") {
		t.Fatalf("leafValidateRequired() = %v, want missing --n for explicit 0", err)
	}

	// 整型解析失败视为「已提供」：Required 校验放行，让 leafArgs 报出更
	// 精确的 invalid integer 错误。
	specEnv := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "n", Usage: "数量", Kind: LeafInt, Required: true, EnvVar: "DWS_LEAF_TEST_REQ_GARBAGE"}},
	}
	t.Setenv("DWS_LEAF_TEST_REQ_GARBAGE", "not-a-number")
	cmdEnv := NewLeafCommand(specEnv)
	if err := corecmd.ValidateRequired(cmdEnv, specEnv.Flags); err != nil {
		t.Fatalf("leafValidateRequired() = %v, want nil for unparsable env (leafArgs reports it)", err)
	}
	if _, err := corecmd.BuildArgs(cmdEnv, specEnv.Flags); err == nil || !strings.Contains(err.Error(), "invalid integer value") {
		t.Fatalf("leafArgs() = %v, want invalid integer error", err)
	}
}

// TestLeafTrimWhitespaceFallsThroughChain：Trim 开启时纯空白候选值与空串
// 同样落入下一级回退，不得在链中「命中后被 trim 成空」。
func TestLeafTrimWhitespaceFallsThroughChain(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "type", Usage: "类型", Trim: true, Default: "ALL", Aliases: []string{"kind"}, EnvVar: "DWS_LEAF_TEST_TRIM_TYPE"}},
	}
	// 主 flag 纯空白 → 回退 env。
	t.Setenv("DWS_LEAF_TEST_TRIM_TYPE", "from-env")
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("type", "   "); err != nil {
		t.Fatal(err)
	}
	if got := corecmd.EffectiveValue(cmd, spec.Flags[0]); got != "from-env" {
		t.Fatalf("effective = %q, want whitespace primary to fall through to env", got)
	}
	// 别名纯空白也回退；env 纯空白最终落到注册默认值。
	t.Setenv("DWS_LEAF_TEST_TRIM_TYPE", "  ")
	if err := cmd.Flags().Set("kind", "\t"); err != nil {
		t.Fatal(err)
	}
	if got := corecmd.EffectiveValue(cmd, spec.Flags[0]); got != "ALL" {
		t.Fatalf("effective = %q, want whitespace chain to land on default", got)
	}
}
