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
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// fakeDevAppRunner 捕获 runDevAppTool 派发的 invocation，用于断言 LeafSpec.Call
// 装配出的 toolArgs 与手写版逐字等价。这是 catalog drift 不覆盖的运行时维度。
type fakeDevAppRunner struct {
	got executor.Invocation
}

func (f *fakeDevAppRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	f.got = inv
	// 空 Response：normalizeDevAppServiceResult 无 "content" 时早返回，安全。
	return executor.Result{Invocation: inv, Response: map[string]any{}}, nil
}

// TestDevAppCredentialsGetLeafDispatchesTrimmedArgs 验证迁移到 LeafSpec 后：
// toolArgs 键/值/trim 与手写版等价，且 PostMount 设上了 schema 注解与 NoArgs。
func TestDevAppCredentialsGetLeafDispatchesTrimmedArgs(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppCredentialsGetCommand(r)
	// 含首尾空白：验证 LeafFlag.Trim 等价手写 devAppStringFlag 的 TrimSpace。
	if err := cmd.Flags().Set("unified-app-id", "  APP-123  "); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if r.got.Tool != devAppCredentialsGetTool {
		t.Fatalf("tool = %q, want %q", r.got.Tool, devAppCredentialsGetTool)
	}
	if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-123" {
		t.Fatalf("unifiedAppId = %q, want trimmed \"APP-123\"", v)
	}
	if cmd.Annotations["mcp-tool"] != devAppCredentialsGetTool {
		t.Fatalf("mcp-tool annotation = %q (PostMount 未生效)", cmd.Annotations["mcp-tool"])
	}
	if cmd.Args == nil {
		t.Fatal("Args == nil, want NoArgs via PostMount")
	}
}

// TestDevAppLifecycleLeafWriteGuardAndArgs 验证写守卫拦截/放行 + toolArgs 装配。
func TestDevAppLifecycleLeafWriteGuardAndArgs(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppLifecycleCommand(r, "enable", "启用应用", devAppEnableTool)
	if err := cmd.Flags().Set("unified-app-id", "APP-9"); err != nil {
		t.Fatal(err)
	}
	// 无 --yes / --dry-run：写守卫必须拦下（devAppRequireWriteGuard）。
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("RunE() without --yes: want write-guard error, got nil")
	}
	// devAppYes 在 cmd.Flags 里找 "yes"；注册并置位，守卫放行。
	cmd.Flags().Bool("yes", false, "")
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() with --yes: error = %v", err)
	}
	if r.got.Tool != devAppEnableTool {
		t.Fatalf("tool = %q, want %q", r.got.Tool, devAppEnableTool)
	}
	if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-9" {
		t.Fatalf("unifiedAppId = %q, want \"APP-9\"", v)
	}
}

// TestDevAppCredentialsGetMissingRequired 验证自定义必填报错（apperrors）逐字保留。
func TestDevAppCredentialsGetMissingRequired(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppCredentialsGetCommand(r)
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("want required error for missing --unified-app-id")
	}
	if !strings.Contains(err.Error(), "--unified-app-id 为必填") {
		t.Fatalf("err = %v, want 含 \"--unified-app-id 为必填\"", err)
	}
}

// TestDevAppVersionLeafs 验证 version get/status/check-approval 的 toolArgs
// 装配：双必填 trim + check-approval 经 Call 注入 precheckOnly=true 常量。
func TestDevAppVersionLeafs(t *testing.T) {
	cases := []struct {
		name         string
		build        func(*fakeDevAppRunner) *cobra.Command
		tool         string
		precheckOnly any
	}{
		{"get", func(r *fakeDevAppRunner) *cobra.Command { return newDevAppVersionGetCommand(r) }, devAppVersionDetailTool, nil},
		{"status", func(r *fakeDevAppRunner) *cobra.Command { return newDevAppVersionStatusCommand(r) }, devAppVersionStatusTool, nil},
		{"check-approval", func(r *fakeDevAppRunner) *cobra.Command { return newDevAppVersionCheckApprovalCommand(r) }, devAppVersionPublishTool, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeDevAppRunner{}
			cmd := c.build(r)
			if err := cmd.Flags().Set("unified-app-id", "  APP-1  "); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Flags().Set("version-id", " v9 "); err != nil {
				t.Fatal(err)
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if r.got.Tool != c.tool {
				t.Fatalf("tool = %q, want %q", r.got.Tool, c.tool)
			}
			if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-1" {
				t.Fatalf("unifiedAppId = %q, want trimmed \"APP-1\"", v)
			}
			if v, _ := r.got.Params["versionId"].(string); v != "v9" {
				t.Fatalf("versionId = %q, want trimmed \"v9\"", v)
			}
			if c.precheckOnly != nil {
				if v, _ := r.got.Params["precheckOnly"].(bool); v != c.precheckOnly {
					t.Fatalf("precheckOnly = %v, want %v", r.got.Params["precheckOnly"], c.precheckOnly)
				}
			} else if _, present := r.got.Params["precheckOnly"]; present {
				t.Fatalf("precheckOnly present = %v, want absent", r.got.Params["precheckOnly"])
			}
		})
	}
}

// TestDevAppMemberListLeaf 验证 member list 的 unified-id 装配。
func TestDevAppMemberListLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppMemberListCommand(r)
	if err := cmd.Flags().Set("unified-app-id", "APP-7"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if r.got.Tool != devAppMemberListTool {
		t.Fatalf("tool = %q, want %q", r.got.Tool, devAppMemberListTool)
	}
	if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-7" {
		t.Fatalf("unifiedAppId = %q, want \"APP-7\"", v)
	}
}

// TestDevAppPermissionListLeaf 验证 ToUpper transform + auth-status 默认 + 命令别名。
func TestDevAppPermissionListLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppPermissionListCommand(r)
	// 命令别名 "search" 经 PostMount 设回。
	if !containsString(cmd.Aliases, "search") {
		t.Fatalf("cmd.Aliases = %v, want contain search", cmd.Aliases)
	}
	_ = cmd.Flags().Set("unified-app-id", "APP-P")
	_ = cmd.Flags().Set("scope-type", "app")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	// auth-status 默认 ALL → ToUpper ALL。
	if v, _ := r.got.Params["authStatus"].(string); v != "ALL" {
		t.Fatalf("authStatus = %v, want ALL (default ToUpper)", r.got.Params["authStatus"])
	}
	// scope-type 小写 app → ToUpper APP。
	if v, _ := r.got.Params["scopeType"].(string); v != "APP" {
		t.Fatalf("scopeType = %v, want APP (ToUpper)", r.got.Params["scopeType"])
	}
	if _, present := r.got.Params["pageSize"]; !present {
		t.Fatal("pageSize missing, want cursor injection")
	}
}

// TestDevAppVersionPublishLeaf 验证 precheckOnly=false 常量 + confirmed-sensitive Changed() 语义。
func TestDevAppVersionPublishLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppVersionPublishCommand(r)
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")
	_ = cmd.Flags().Set("unified-app-id", "APP-V")
	_ = cmd.Flags().Set("version-id", "v1")
	// confirmed-sensitive 未显式设：不入参。
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["precheckOnly"].(bool); v != false {
		t.Fatalf("precheckOnly = %v, want false", r.got.Params["precheckOnly"])
	}
	if _, present := r.got.Params["confirmedSensitive"]; present {
		t.Fatalf("confirmedSensitive present = %v, want omitted (not Changed)", r.got.Params["confirmedSensitive"])
	}
	// 显式设 confirmed-sensitive=true：入参。
	r2 := &fakeDevAppRunner{}
	cmd2 := newDevAppVersionPublishCommand(r2)
	cmd2.Flags().Bool("yes", false, "")
	_ = cmd2.Flags().Set("yes", "true")
	_ = cmd2.Flags().Set("unified-app-id", "APP-V")
	_ = cmd2.Flags().Set("version-id", "v1")
	_ = cmd2.Flags().Set("confirmed-sensitive", "true")
	_ = cmd2.RunE(cmd2, nil)
	if v, _ := r2.got.Params["confirmedSensitive"].(bool); v != true {
		t.Fatalf("confirmedSensitive = %v, want true", r2.got.Params["confirmedSensitive"])
	}
}

// TestDevAppGetLeafDualKey 验证二选一定位键。
func TestDevAppGetLeafDualKey(t *testing.T) {
	// 都不传：报错。
	r := &fakeDevAppRunner{}
	cmd := newDevAppGetCommand(r)
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "请传入 --unified-app-id 或 --app-key") {
		t.Fatalf("err = %v, want 二选一报错", err)
	}
	// 只 app-key：params 含 appKey，无 unifiedAppId。
	_ = cmd.Flags().Set("app-key", "dingABC")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["appKey"].(string); v != "dingABC" {
		t.Fatalf("appKey = %v, want dingABC", r.got.Params["appKey"])
	}
	if _, present := r.got.Params["unifiedAppId"]; present {
		t.Fatalf("unifiedAppId present, want omitted")
	}
}

// TestDevAppListLeaf 是 showcase：退役 devAppPutString/devAppPutInt/devAppFlagOrFallback。
// 验证 name/keyword 别名回退、LeafInt(!=0 入参)、cursor 注入、OmitEmpty。
func TestDevAppListLeaf(t *testing.T) {
	// keyword 别名回退：只设 keyword，name 应取 keyword 值。
	r := &fakeDevAppRunner{}
	cmd := newDevAppListCommand(r)
	_ = cmd.Flags().Set("keyword", "DemoApp")
	_ = cmd.Flags().Set("app-group-id", "5")
	_ = cmd.Flags().Set("develop-type", "0") // 0：LeafInt 不入参
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["name"].(string); v != "DemoApp" {
		t.Fatalf("name = %q, want keyword-fallback \"DemoApp\"", v)
	}
	if v, _ := r.got.Params["appGroupId"].(int); v != 5 {
		t.Fatalf("appGroupId = %v, want int(5)", r.got.Params["appGroupId"])
	}
	if _, present := r.got.Params["developType"]; present {
		t.Fatalf("developType present = %v, want omitted (LeafInt 0)", r.got.Params["developType"])
	}
	if _, present := r.got.Params["pageSize"]; !present {
		t.Fatal("pageSize missing, want cursor injection")
	}
	if _, present := r.got.Params["creator"]; present {
		t.Fatalf("creator present, want omitted (OmitEmpty)")
	}
	// name 优先于 keyword。
	r2 := &fakeDevAppRunner{}
	cmd2 := newDevAppListCommand(r2)
	_ = cmd2.Flags().Set("name", "Primary")
	_ = cmd2.Flags().Set("keyword", "Fallback")
	_ = cmd2.RunE(cmd2, nil)
	if v, _ := r2.got.Params["name"].(string); v != "Primary" {
		t.Fatalf("name = %q, want Primary (name beats keyword)", v)
	}
}

// TestDevAppEventListLeaf 验证 cursor/pageSize 经 devAppApplyCursorParams 注入。
func TestDevAppEventListLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppEventListCommand(r)
	_ = cmd.Flags().Set("unified-app-id", "APP-E")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-E" {
		t.Fatalf("unifiedAppId = %q, want APP-E", v)
	}
	// page-size 默认 20，经 devAppApplyCursorParams 注入为 float64(20)。
	if _, present := r.got.Params["pageSize"]; !present {
		t.Fatal("pageSize missing, want injected by devAppApplyCursorParams")
	}
	// cursor 默认空：不入参。
	if _, present := r.got.Params["cursor"]; present {
		t.Fatalf("cursor present = %v, want omitted when empty", r.got.Params["cursor"])
	}
	// keyword 未设：OmitEmpty 省略。
	if _, present := r.got.Params["keyword"]; present {
		t.Fatalf("keyword present, want omitted (OmitEmpty)")
	}
}

// TestDevAppUpdateLeaf 验证「至少一项」校验 + OmitEmpty 装配。
func TestDevAppUpdateLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppUpdateCommand(r)
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")
	_ = cmd.Flags().Set("unified-app-id", "APP-U")
	// 全空：至少一项拦。
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "至少提供一项待更新字段") {
		t.Fatalf("err = %v, want 至少提供一项待更新字段", err)
	}
	_ = cmd.Flags().Set("desc", " 新描述 ")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["desc"].(string); v != "新描述" {
		t.Fatalf("desc = %q, want trimmed", v)
	}
	if _, present := r.got.Params["name"]; present {
		t.Fatalf("name present, want omitted (OmitEmpty)")
	}
}

// TestDevAppMemberAddLeaf 验证 userIds []string 注入 + memberType 装配 + 写守卫/必填顺序。
func TestDevAppMemberAddLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppMemberAddCommand(r)
	_ = cmd.Flags().Set("unified-app-id", "APP-M")
	// 无 --yes：写守卫拦。
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("want write-guard error without --yes")
	}
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")
	// 缺 user-ids/member-type：必填拦。
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "--user-ids 为必填") {
		t.Fatalf("err = %v, want --user-ids 为必填", err)
	}
	_ = cmd.Flags().Set("user-ids", "u1, u2")
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "--member-type 为必填") {
		t.Fatalf("err = %v, want --member-type 为必填", err)
	}
	// 齐全：userIds []string + memberType 装配。
	_ = cmd.Flags().Set("member-type", "DEVELOPER")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	users, ok := r.got.Params["userIds"].([]string)
	if !ok || len(users) != 2 || users[0] != "u1" || users[1] != "u2" {
		t.Fatalf("userIds = %v, want [u1 u2]", r.got.Params["userIds"])
	}
	if v, _ := r.got.Params["memberType"].(string); v != "DEVELOPER" {
		t.Fatalf("memberType = %v, want DEVELOPER", r.got.Params["memberType"])
	}
}

// TestDevAppSecurityConfigLeaf 验证 3 个列表 flag 的 Call 注入 + 「至少一项」校验。
func TestDevAppSecurityConfigLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppSecurityConfigCommand(r)
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")
	_ = cmd.Flags().Set("unified-app-id", "APP-S")
	// 全空：至少一项拦。
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "至少提供一项安全配置") {
		t.Fatalf("err = %v, want 至少提供一项安全配置", err)
	}
	// 设 ip-whitelist：注入 []string，其余省略。
	_ = cmd.Flags().Set("ip-whitelist", "10.0.0.1; 10.0.0.2")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	ips, ok := r.got.Params["ipWhitelist"].([]string)
	if !ok || len(ips) != 2 || ips[0] != "10.0.0.1" || ips[1] != "10.0.0.2" {
		t.Fatalf("ipWhitelist = %v, want [10.0.0.1 10.0.0.2]", r.got.Params["ipWhitelist"])
	}
	if _, present := r.got.Params["redirectUrls"]; present {
		t.Fatalf("redirectUrls present, want omitted")
	}
}

// TestDevAppPermissionAddLeaf 验证 scope-values 经 Call 注入为 []string + 写守卫/必填。
func TestDevAppPermissionAddLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppPermissionAddCommand(r)
	// scope-values 在 PostMount 注册。
	if err := cmd.Flags().Set("unified-app-id", "APP-3"); err != nil {
		t.Fatal(err)
	}
	// 无 --yes 且无 scope-values：写守卫先拦。
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("want write-guard error without --yes")
	}
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")
	// 有 --yes 但无 scope-values：必填校验拦。
	if err := cmd.RunE(cmd, nil); err == nil ||
		!strings.Contains(err.Error(), "--scope-values 为必填") {
		t.Fatalf("err = %v, want --scope-values 为必填", err)
	}
	// 齐全：scopeValues 注入为 []string。
	_ = cmd.Flags().Set("scope-values", "Contact.User.mobile, qyapi_robot_sendmsg")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	scopes, ok := r.got.Params["scopeValues"].([]string)
	if !ok || len(scopes) != 2 || scopes[0] != "Contact.User.mobile" || scopes[1] != "qyapi_robot_sendmsg" {
		t.Fatalf("scopeValues = %v, want [Contact.User.mobile qyapi_robot_sendmsg]", r.got.Params["scopeValues"])
	}
	if v, _ := r.got.Params["unifiedAppId"].(string); v != "APP-3" {
		t.Fatalf("unifiedAppId = %q, want APP-3", v)
	}
}

// TestDevAppWebappConfigLeaf 验证 OmitEmpty 装配 + 「至少一项」跨 flag 校验。
func TestDevAppWebappConfigLeaf(t *testing.T) {
	r := &fakeDevAppRunner{}
	cmd := newDevAppWebappConfigCommand(r)
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")

	// 全空：写守卫放行后，「至少一项」必须拦。
	if err := cmd.Flags().Set("unified-app-id", "APP-1"); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "至少提供一项网页应用配置") {
		t.Fatalf("err = %v, want 至少提供一项网页应用配置", err)
	}

	// 只设一项：该项入参，其余 OmitEmpty 省略。
	if err := cmd.Flags().Set("homepage-url", " https://a.example.invalid "); err != nil {
		t.Fatal(err)
	}
	r = &fakeDevAppRunner{}
	cmd2 := newDevAppWebappConfigCommand(r)
	cmd2.Flags().Bool("yes", false, "")
	_ = cmd2.Flags().Set("yes", "true")
	_ = cmd2.Flags().Set("unified-app-id", "APP-1")
	_ = cmd2.Flags().Set("homepage-url", " https://a.example.invalid ")
	if err := cmd2.RunE(cmd2, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if v, _ := r.got.Params["homepageUrl"].(string); v != "https://a.example.invalid" {
		t.Fatalf("homepageUrl = %q, want trimmed", v)
	}
	if _, present := r.got.Params["h5PageType"]; present {
		t.Fatalf("h5PageType present = %v, want omitted (OmitEmpty)", r.got.Params["h5PageType"])
	}
	if _, present := r.got.Params["ompUrl"]; present {
		t.Fatalf("ompUrl present = %v, want omitted (OmitEmpty)", r.got.Params["ompUrl"])
	}
}

// TestDevAppLeafToolArgsTable 表驱动补齐剩余迁移命令的 toolArgs 断言。
// 覆盖此前无断言的:robot enable/disable、permission remove、event sub/unsub、
// member remove、version create、app create、robot config-get、lifecycle disable。
func TestDevAppLeafToolArgsTable(t *testing.T) {
	cases := []struct {
		name       string
		build      func(executor.Runner) *cobra.Command
		flags      map[string]string
		needsYes   bool
		wantTool   string
		wantParam  map[string]any
		wantAbsent []string // 应缺席的 params 键（OmitEmpty 校验）
	}{
		{"robot enable", newDevAppRobotEnableCommand, map[string]string{"unified-app-id": "APP-R"}, true, devAppRobotEnableTool, map[string]any{"unifiedAppId": "APP-R"}, nil},
		{"robot disable", newDevAppRobotOfflineCommand, map[string]string{"unified-app-id": "APP-R"}, true, devAppRobotOfflineTool, map[string]any{"unifiedAppId": "APP-R"}, nil},
		{"permission remove", newDevAppPermissionRemoveCommand, map[string]string{"unified-app-id": "APP-P", "scope-values": "a.b, c.d"}, true, devAppPermissionRmTool, map[string]any{"unifiedAppId": "APP-P", "scopeValues": []string{"a.b", "c.d"}}, nil},
		{"event subscribe", newDevAppEventSubscribeCommand, map[string]string{"unified-app-id": "APP-E", "event-codes": "x, y"}, true, devAppEventSubscribeTool, map[string]any{"unifiedAppId": "APP-E", "eventCodes": []string{"x", "y"}}, nil},
		{"event unsubscribe", newDevAppEventUnsubscribeCommand, map[string]string{"unified-app-id": "APP-E", "event-codes": "x, y"}, true, devAppEventUnsubscribeTool, map[string]any{"unifiedAppId": "APP-E", "eventCodes": []string{"x", "y"}}, nil},
		{"member remove", newDevAppMemberRemoveCommand, map[string]string{"unified-app-id": "APP-M", "user-ids": "u1,u2", "member-type": "DEVELOPER"}, true, devAppMemberRemoveTool, map[string]any{"unifiedAppId": "APP-M", "userIds": []string{"u1", "u2"}, "memberType": "DEVELOPER"}, nil},
		{"version create", newDevAppVersionCreateCommand, map[string]string{"unified-app-id": "APP-V", "desc": "新增机器人"}, true, devAppVersionCreateTool, map[string]any{"unifiedAppId": "APP-V", "desc": "新增机器人"}, []string{"version"}},
		{"app create", newDevAppCreateCommand, map[string]string{"name": "DemoApp"}, true, devAppCreateTool, map[string]any{"name": "DemoApp"}, []string{"desc", "iconMediaId"}},
		{"robot config get", newDevAppRobotConfigGetCommand, map[string]string{"unified-app-id": "APP-C"}, false, devAppRobotConfigGetTool, map[string]any{"unifiedAppId": "APP-C"}, nil},
		{"lifecycle disable", func(r executor.Runner) *cobra.Command {
			return newDevAppLifecycleCommand(r, "disable", "停用应用", devAppDisableTool)
		}, map[string]string{"unified-app-id": "APP-D"}, true, devAppDisableTool, map[string]any{"unifiedAppId": "APP-D"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeDevAppRunner{}
			cmd := c.build(r)
			if c.needsYes {
				cmd.Flags().Bool("yes", false, "")
				_ = cmd.Flags().Set("yes", "true")
			}
			for k, v := range c.flags {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatalf("set %s: %v", k, err)
				}
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if r.got.Tool != c.wantTool {
				t.Fatalf("tool = %q, want %q", r.got.Tool, c.wantTool)
			}
			for k, want := range c.wantParam {
				if got := r.got.Params[k]; !reflect.DeepEqual(got, want) {
					t.Fatalf("param %s = %#v (%T), want %#v (%T)", k, got, got, want, want)
				}
			}
			for _, k := range c.wantAbsent {
				if _, present := r.got.Params[k]; present {
					t.Fatalf("param %s present = %#v, want absent (OmitEmpty)", k, r.got.Params[k])
				}
			}
		})
	}
}
