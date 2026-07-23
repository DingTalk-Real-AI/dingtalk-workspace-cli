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
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
			{Name: "cursor", Usage: "游标", Kind: LeafInt64, Bind: "cursor"},
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
	err := leafValidateRequired(cmd, leafTestSpec())
	if err == nil || !strings.Contains(err.Error(), "missing required flag(s): --content") {
		t.Fatalf("leafValidateRequired() = %v, want missing --content", err)
	}
}

func TestLeafValidateRequiredEnvFallback(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	err := leafValidateRequired(cmd, leafTestSpec())
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
	err = leafValidateRequired(cmd, leafTestSpec())
	if err == nil || !strings.Contains(err.Error(), "DWS_LEAF_TEST_TOKEN") {
		t.Fatalf("leafValidateRequired() = %v, want env hint", err)
	}
	// env 提供后通过。
	t.Setenv("DWS_LEAF_TEST_TOKEN", "tok")
	if err := leafValidateRequired(cmd, leafTestSpec()); err != nil {
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
	args, err := leafArgs(cmd, leafTestSpec())
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
	if args["cursor"] != int64(10) {
		t.Fatalf("cursor = %v (%T), want int64(10)", args["cursor"], args["cursor"])
	}
	if args["scope"] != "ALL" {
		t.Fatalf("scope = %v, want ArgDefault ALL", args["scope"])
	}
}

func TestLeafArgsOmitsEmptyAndNonPositive(t *testing.T) {
	cmd := NewLeafCommand(leafTestSpec())
	args, err := leafArgs(cmd, leafTestSpec())
	if err != nil {
		t.Fatalf("leafArgs() error = %v", err)
	}
	if _, present := args["noteText"]; present {
		t.Fatalf("noteText present = %v, want omitted when empty", args["noteText"])
	}
	if _, present := args["cursor"]; present {
		t.Fatalf("cursor present = %v, want omitted when <= 0", args["cursor"])
	}
	// 未配置 OmitEmpty 的 flag 即使为空也入参（复现手写语义；Required 校验在
	// leafArgs 之前执行，保证真实路径不会发出空值）。
	if v, present := args["accessToken"]; !present || v != "" {
		t.Fatalf("accessToken = %v/%v, want present-but-empty without OmitEmpty", v, present)
	}
	// 未设置 OmitEmpty 的字符串即使为空也入参（复现手写 remindType 恒入参语义）。
	if v, present := args["remindType"]; !present || v != "app" {
		t.Fatalf("remindType = %v/%v, want registered default", v, present)
	}
}

func TestNewLeafCommandCustomRunE(t *testing.T) {
	called := false
	spec := leafTestSpec()
	spec.RunE = func(cmd *cobra.Command, args []string) error {
		called = true
		return nil
	}
	cmd := NewLeafCommand(spec)
	if err := cmd.RunE(cmd, nil); err != nil || !called {
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
	args, err := leafArgs(cmd, spec)
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
	args, err := leafArgs(cmd, spec)
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
	args, err = leafArgs(cmd, spec)
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
	args, err := leafArgs(cmd, spec)
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
