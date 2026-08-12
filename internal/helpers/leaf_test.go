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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type deferConfirmTestCaller struct {
	calls int
}

func (c *deferConfirmTestCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}
func (*deferConfirmTestCaller) Format() string { return "json" }
func (*deferConfirmTestCaller) DryRun() bool   { return false }
func (*deferConfirmTestCaller) Fields() string { return "" }
func (*deferConfirmTestCaller) JQ() string     { return "" }

func leafTestSpec() LeafSpec {
	return LeafSpec{
		Use:   "send",
		Short: "发送",
		Tool:  "send_thing",
		Flags: []LeafFlag{
			{Name: "token", Usage: "令牌", Required: true, EnvVar: "DWS_LEAF_TEST_TOKEN", RequiredHint: "flag --token is required (or set DWS_LEAF_TEST_TOKEN)", Bind: "accessToken"},
			{Name: "users", Usage: "用户列表", Required: true, Bind: "userList", Transform: leafTestCSV},
			{Name: "content", Usage: "内容", Required: true},
			{Name: "type", Usage: "类型", Default: "app", Bind: "remindType"},
			{Name: "note", Usage: "备注", Aliases: []string{"remark"}, OmitEmpty: true, Bind: "noteText"},
			{Name: "cursor", Usage: "游标", Kind: LeafInt, Bind: "cursor"},
			{Name: "scope", Usage: "范围", ArgDefault: "ALL", Bind: "scope"},
		},
	}
}

// leafTestCSV 把逗号分隔字符串拆成 []string（测试专用，避免依赖产品文件的 helper）。
func leafTestCSV(raw string) (any, error) {
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

func TestNewLeafCommandRegistersFlags(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())

	f := cmd.Flags().Lookup("type")
	if f == nil || f.DefValue != "app" || f.Usage != "类型" {
		t.Fatalf("type flag = %+v, want default app with usage", f)
	}
	alias := cmd.Flags().Lookup("remark")
	if alias == nil || !alias.Hidden {
		t.Fatalf("alias remark = %+v, want registered and hidden", alias)
	}
	if cmd.Flags().Lookup("cursor") == nil {
		t.Fatal("cursor flag not registered")
	}
}

func TestNewLeafCommandMarksRequired(t *testing.T) {
	cmd := NewLeafCommand(LeafSpec{
		Use: "get", Tool: "get_thing",
		Flags: []LeafFlag{{Name: "id", Usage: "ID", Required: true, MarkRequired: true}},
	})
	f := cmd.Flags().Lookup("id")
	if f == nil || len(f.Annotations) == 0 {
		t.Fatalf("id flag = %+v, want MarkFlagRequired annotations", f)
	}
}

func TestLeafValidateRequiredPlainGroup(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	if err := cmd.Flags().Set("users", "u1"); err != nil {
		t.Fatal(err)
	}
	err := corecmd.ValidateRequired(cmd, leafTestSpec().Flags)
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content") {
		t.Fatalf("leafValidateRequired() = %v, want missing --content", err)
	}
}

func TestLeafValidateRequiredSatisfiedByAlias(t *testing.T) {
	// 声明的回退语义：只传兼容别名时，普通 Required 也视为已提供。
	spec := LeafSpec{
		Use: "send", Tool: "send_thing",
		Flags: []LeafFlag{
			{Name: "content", Usage: "内容", Required: true, Aliases: []string{"remark"}},
		},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("remark", "仅别名"); err != nil {
		t.Fatal(err)
	}
	if err := corecmd.ValidateRequired(cmd, spec.Flags); err != nil {
		t.Fatalf("leafValidateRequired() = %v, want nil (alias satisfies required)", err)
	}
}

func TestLeafValidateRequiredAliasAbsentStillFails(t *testing.T) {
	// 主 flag 与别名都缺失时，仍按主 flag 名报统一错误。
	spec := LeafSpec{
		Use: "send", Tool: "send_thing",
		Flags: []LeafFlag{
			{Name: "content", Usage: "内容", Required: true, Aliases: []string{"remark"}},
		},
	}
	cmd := NewLeafCommand(spec)
	err := corecmd.ValidateRequired(cmd, spec.Flags)
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content") {
		t.Fatalf("leafValidateRequired() = %v, want missing --content", err)
	}
}

func TestLeafValidateRequiredTrimWhitespaceOnlyFails(t *testing.T) {
	// Trim 声明下纯空白值在 required 校验中视为空。
	spec := LeafSpec{
		Use: "send", Tool: "send_thing",
		Flags: []LeafFlag{
			{Name: "content", Usage: "内容", Required: true, Trim: true},
		},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("content", "   "); err != nil {
		t.Fatal(err)
	}
	err := corecmd.ValidateRequired(cmd, spec.Flags)
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content") {
		t.Fatalf("leafValidateRequired() = %v, want whitespace-only treated as missing", err)
	}
}

func TestLeafValidateRequiredEnvFallback(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	err := corecmd.ValidateRequired(cmd, leafTestSpec().Flags)
	// 普通组（users/content）先报错，不触及 env 组。
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --users, --content") {
		t.Fatalf("leafValidateRequired() = %v, want plain group first", err)
	}
	// 普通组满足后，env 缺失时走 RequiredHint。
	if err := cmd.Flags().Set("users", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("content", "c"); err != nil {
		t.Fatal(err)
	}
	err = corecmd.ValidateRequired(cmd, leafTestSpec().Flags)
	if err == nil || !strings.Contains(err.Error(), "DWS_LEAF_TEST_TOKEN") {
		t.Fatalf("leafValidateRequired() = %v, want env hint", err)
	}
	// env 提供后通过。
	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	if err := corecmd.ValidateRequired(cmd, leafTestSpec().Flags); err != nil {
		t.Fatalf("leafValidateRequired() = %v, want nil", err)
	}
}

func TestLeafArgs(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	if err := cmd.Flags().Set("users", "u1, u2"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("content", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("remark", "via-alias"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("cursor", "10"); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, leafTestSpec().Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if args["accessToken"] != "tok" {
		t.Fatalf("accessToken = %v, want env fallback tok", args["accessToken"])
	}
	users, ok := args["userList"].([]string)
	if !ok || len(users) != 2 || users[0] != "u1" || users[1] != "u2" {
		t.Fatalf("userList = %v, want [u1 u2]", args["userList"])
	}
	if args["content"] != "hello" || args["remindType"] != "app" {
		t.Fatalf("content/remindType = %v/%v", args["content"], args["remindType"])
	}
	if args["noteText"] != "via-alias" {
		t.Fatalf("noteText = %v, want alias fallback", args["noteText"])
	}
	if args["cursor"] != 10 {
		t.Fatalf("cursor = %v (%T), want int(10)", args["cursor"], args["cursor"])
	}
	if args["scope"] != "ALL" {
		t.Fatalf("scope = %v, want ArgDefault ALL", args["scope"])
	}
}

func TestLeafArgsOmitsEmptyAndNonPositive(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	// Satisfy Required flags first: BuildArgs now rejects Required transforms that
	// collapse to empty (unset CSV), matching runtime behavior after ValidateRequired.
	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	if err := cmd.Flags().Set("users", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("content", "hello"); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, leafTestSpec().Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if _, present := args["noteText"]; present {
		t.Fatalf("noteText present = %v, want omitted when empty", args["noteText"])
	}
	if _, present := args["cursor"]; present {
		t.Fatalf("cursor present = %v, want omitted when zero", args["cursor"])
	}
	if v, present := args["accessToken"]; !present || v != "tok" {
		t.Fatalf("accessToken = %v/%v, want env tok", v, present)
	}
	// 未设置 OmitEmpty 的字符串即使为空也入参（复现手写 remindType 恒入参语义）。
	if v, present := args["remindType"]; !present || v != "app" {
		t.Fatalf("remindType = %v/%v, want registered default", v, present)
	}
}

func TestNewLeafCommandCustomRunE(t *testing.T) {
	// The custom RunE replaces dispatch, not the declared contract.
	called := false
	newSend := func() *cobra.Command {
		called = false
		spec := leafTestSpec()
		spec.RunE = func(cmd *cobra.Command, args []string) error {
			called = true
			return nil
		}
		return NewLeafCommand(spec)
	}

	unsatisfied := newSend()
	err := unsatisfied.RunE(unsatisfied, nil)
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s)") {
		t.Fatalf("custom RunE must not bypass declared required flags, got %v", err)
	}
	if called {
		t.Fatal("custom RunE ran despite an unsatisfied declaration")
	}

	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	satisfied := newSend()
	if err := satisfied.Flags().Set("users", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := satisfied.Flags().Set("content", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := satisfied.RunE(satisfied, nil); err != nil || !called {
		t.Fatalf("custom RunE not used: called=%v err=%v", called, err)
	}
}

func TestLeafValidateHookRunsAfterRequired(t *testing.T) {
	validated := false
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "start", Usage: "开始", Required: true}},
		Validate: func(cmd *cobra.Command, args []string) error {
			validated = true
			return fmt.Errorf("range invalid")
		},
	}
	cmd := NewLeafCommand(spec)
	// required 未满足时先报 required，不触发 Validate。
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --start") {
		t.Fatalf("RunE() = %v, want required error first", err)
	}
	if validated {
		t.Fatal("Validate ran before required check passed")
	}
	// required 满足后 Validate 拦截。
	if err := cmd.Flags().Set("start", "s"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil || err.Error() != "range invalid" {
		t.Fatalf("RunE() = %v, want Validate error", err)
	}
	if !validated {
		t.Fatal("Validate did not run")
	}
}

func TestLeafArgsTransformNilSkipsKey(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{
			{Name: "page", Usage: "页码", OmitEmpty: true, Bind: "pageNum", Transform: func(raw string) (any, error) {
				return nil, nil // 解析失败语义：跳过
			}},
		},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("page", "abc"); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if _, present := args["pageNum"]; present {
		t.Fatalf("pageNum present = %v, want skipped on (nil, nil) transform", args["pageNum"])
	}
}

func TestLeafArgsLeafIntOmitsZero(t *testing.T) {
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{
			{Name: "app-group-id", Usage: "分组", Kind: LeafInt, Bind: "appGroupId"},
			{Name: "develop-type", Usage: "类型", Kind: LeafInt, Bind: "developType"},
		},
	}
	cmd := NewLeafCommand(spec)
	// 默认 0：不入参。
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if _, present := args["appGroupId"]; present {
		t.Fatalf("appGroupId present = %v, want omitted when 0", args["appGroupId"])
	}
	// 非 0 入参，类型为 int（与手写 devAppPutInt 一致）。
	if err := cmd.Flags().Set("develop-type", "2"); err != nil {
		t.Fatal(err)
	}
	args, err = corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	v, ok := args["developType"].(int)
	if !ok || v != 2 {
		t.Fatalf("developType = %v (%T), want int(2)", args["developType"], args["developType"])
	}
}

func TestLeafCommandCallDispatch(t *testing.T) {
	// Call 非空时替代默认 callMCPTool，收到框架装配好的 toolArgs。
	var gotTool string
	var gotArgs map[string]any
	spec := LeafSpec{
		Use: "list", Tool: "list_thing",
		Flags: []LeafFlag{{Name: "name", Usage: "名称", Bind: "name"}},
		Call: func(cmd *cobra.Command, tool string, args map[string]any) error {
			gotTool, gotArgs = tool, args
			return nil
		},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("name", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if gotTool != "list_thing" {
		t.Fatalf("Call tool = %q, want list_thing", gotTool)
	}
	if gotArgs["name"] != "demo" {
		t.Fatalf("Call args = %v, want name=demo", gotArgs)
	}
}

func TestDeclareLeafMetadataInstallsConfirmSafetyForUserRequired(t *testing.T) {
	// With Validate: confirm runs at RunE entry (after PreRunE), so inner
	// must not execute without --yes.
	called := false
	cmd := &cobra.Command{
		Use: "delete",
		RunE: func(cmd *cobra.Command, args []string) error {
			called = true
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(*cobra.Command, []string) error { return nil },
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "test delete",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "test fixture for ConfirmSafety wrap",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "test delete",
				UseWhen:      []string{"test"},
				AvoidWhen:    []string{"never"},
				Examples:     []string{"dws delete --yes"},
			},
		},
	})
	if !HasContractConfirmSafety(cmd) || !HasContractValidate(cmd) {
		t.Fatal("expected contract ConfirmSafety + Validate annotations")
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("want confirmation_required without --yes")
	}
	if called {
		t.Fatal("inner RunE must not run before confirmation")
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	called = false
	if err := cmd.Execute(); err != nil {
		t.Fatalf("with --yes: %v", err)
	}
	if !called {
		t.Fatal("inner RunE must run after --yes")
	}
}

func TestDeclareLeafMetadataDefersConfirmUntilCallTool(t *testing.T) {
	// Without Validate: RunE-local checks run before ConfirmSafety; the gate
	// fires on the first MCP CallTool.
	testseam.Protect(t, &deps)
	concrete := &deferConfirmTestCaller{}
	InitDeps(concrete)
	deps.Out.w = io.Discard

	cmd := &cobra.Command{
		Use: "delete",
		RunE: func(c *cobra.Command, args []string) error {
			id, _ := c.Flags().GetString("id")
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("flag --id is required")
			}
			_, err := deps.Caller.CallTool(c.Context(), "test", "delete_thing", map[string]any{"id": id})
			return err
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("id", "", "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "test delete",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "test fixture for deferred ConfirmSafety",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "test delete",
				UseWhen:      []string{"test"},
				AvoidWhen:    []string{"never"},
				Examples:     []string{"dws delete --id x --yes"},
			},
		},
	})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "flag --id is required") {
		t.Fatalf("missing id: %v, want RunE validation before confirm", err)
	}
	if concrete.calls != 0 {
		t.Fatalf("CallTool calls = %d, want 0 when validation fails", concrete.calls)
	}

	if err := cmd.Flags().Set("id", "x"); err != nil {
		t.Fatal(err)
	}
	err = cmd.Execute()
	if err == nil || (!strings.Contains(err.Error(), "confirmation_required") && !strings.Contains(err.Error(), "需要用户确认")) {
		t.Fatalf("valid args without --yes: %v, want confirmation_required", err)
	}
	if concrete.calls != 0 {
		t.Fatalf("CallTool calls = %d, want 0 before confirmation", concrete.calls)
	}
}

func TestDeclareLeafMetadataValidateRunsBeforeConfirmSafety(t *testing.T) {
	// RFC §5.1 / §5.6: local validation must precede Risk confirmation.
	ran := false
	cmd := &cobra.Command{
		Use: "mutate",
		RunE: func(*cobra.Command, []string) error {
			ran = true
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("id", "", "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(c *cobra.Command, args []string) error {
			id, _ := c.Flags().GetString("id")
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("flag --id is required")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_thing",
				CanonicalPath:  "dev.create_thing",
				CLIPath:        "dev create",
				PrimaryCLIPath: "dev create",
			},

			Description: "test mutate",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "test fixture for Validate-before-ConfirmSafety",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "test mutate",
				UseWhen:      []string{"test"},
				AvoidWhen:    []string{"never"},
				Examples:     []string{"dws mutate --id x --yes"},
			},
		},
	})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "flag --id is required") {
		t.Fatalf("Execute() error = %v, want Validate failure before confirmation", err)
	}
	if strings.Contains(err.Error(), "confirmation_required") || strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("Validate must win over ConfirmSafety, got %v", err)
	}
	if ran {
		t.Fatal("RunE must not run when Validate fails")
	}
}

func TestLeafPostMountApplied(t *testing.T) {
	// PostMount 在 flag 注册后执行，用于设置 Args/annotation 等。
	spec := LeafSpec{
		Use: "get", Tool: "get_thing",
		Flags: []LeafFlag{{Name: "id", Usage: "ID"}},
		PostMount: func(cmd *cobra.Command) {
			cmd.DisableAutoGenTag = true
			if cmd.Annotations == nil {
				cmd.Annotations = map[string]string{}
			}
			cmd.Annotations["mcp-tool"] = "get_thing"
		},
	}
	cmd := NewLeafCommand(spec)
	if !cmd.DisableAutoGenTag {
		t.Fatal("PostMount did not set DisableAutoGenTag")
	}
	if cmd.Annotations["mcp-tool"] != "get_thing" {
		t.Fatalf("annotations = %v, want mcp-tool=get_thing", cmd.Annotations)
	}
	if cmd.Flags().Lookup("id") == nil {
		t.Fatal("flag id not registered before PostMount")
	}
}

func TestLeafArgsTrimsValue(t *testing.T) {
	spec := LeafSpec{
		Use: "get", Tool: "get_thing",
		Flags: []LeafFlag{
			{Name: "name", Usage: "名称", Bind: "appName", Trim: true},
			{Name: "note", Usage: "备注", Bind: "note"},
		},
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.Flags().Set("name", "  Demo  "); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("note", "  x  "); err != nil {
		t.Fatal(err)
	}
	args, err := corecmd.BuildArgs(cmd, spec.Flags)
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if args["appName"] != "Demo" {
		t.Fatalf("appName = %q, want trimmed \"Demo\"", args["appName"])
	}
	if args["note"] != "  x  " {
		t.Fatalf("note = %q, want untrimmed", args["note"])
	}
}
