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
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func executeLiteapp(t *testing.T, runner executor.Runner, args ...string) (string, error) {
	t.Helper()
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"dev", "liteapp"}, args...))
	// 走 prepared 执行路径（corecmd.ExecuteForTest）：与 app 装配一致地安装
	// ValidationErrorFunc/FlagErrorFunc，校验分类与退出码在测试中不失真。
	err := corecmd.ExecuteForTest(root)
	return out.String(), err
}

func TestCrossPlatformCoverageLiteappCommandTree(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	liteapp, _, err := dev.Find([]string{"liteapp"})
	if err != nil {
		t.Fatalf("missing liteapp subtree: %v", err)
	}
	for _, name := range []string{"create", "update", "delete", "list", "detail", "credential"} {
		if _, _, err := liteapp.Find([]string{name}); err != nil {
			t.Fatalf("missing liteapp command %q: %v", name, err)
		}
	}
}

func TestCrossPlatformCoverageLiteappDeclaresUnifiedRolloutAndTerminalOutcomes(t *testing.T) {
	group := newLiteappGroup(&captureRunner{})
	for _, name := range []string{"create", "update", "delete", "list", "detail", "credential"} {
		cmd, _, err := group.Find([]string{name})
		if err != nil {
			t.Fatalf("missing liteapp command %q: %v", name, err)
		}
		if got := output.CommandRollout(cmd); got != output.RolloutUnifiedActive {
			t.Fatalf("%s rollout = %q, want %q", name, got, output.RolloutUnifiedActive)
		}
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "create", args: []string{"create", "--name", "周报助手", "--homepage-url", "https://example.com", "--yes"}},
		{name: "update", args: []string{"update", "uni-5005426001", "--desc", "新描述", "--yes"}},
		{name: "delete", args: []string{"delete", "uni-5005426001", "--yes"}},
		{name: "list", args: []string{"list"}},
		{name: "detail", args: []string{"detail", "uni-5005426001"}},
		{name: "credential", args: []string{"credential", "uni-5005426001"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/success", func(t *testing.T) {
			runner := &devAppResponseRunner{response: map[string]any{
				"content": map[string]any{
					"success": true,
					"result":  map[string]any{"unifiedAppId": "uni-5005426001"},
				},
			}}
			out, errOut, store, err := runDevAppFamilyProdAlignedWithStore(t, runner, append([]string{"dev", "liteapp"}, tc.args...)...)
			if err != nil {
				t.Fatalf("success execution error = %v\nstdout=%s\nstderr=%s", err, out, errOut)
			}
			if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
				t.Fatalf("success stored exit code=(%d,%v), want (0,true)", code, emitted)
			}
			var envelope struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("success output is not JSON: %v\n%s", err, out)
			}
			if envelope.Outcome != "success" {
				t.Fatalf("success outcome = %q, want success\n%s", envelope.Outcome, out)
			}
		})

		t.Run(tc.name+"/failure", func(t *testing.T) {
			runner := &devAppResponseRunner{response: map[string]any{
				"content": map[string]any{
					"success":   false,
					"errorCode": "E_TEST_FAILURE",
					"errorMsg":  "test failure",
				},
			}}
			out, errOut, store, err := runDevAppFamilyProdAlignedWithStore(t, runner, append([]string{"dev", "liteapp"}, tc.args...)...)
			if err != nil {
				t.Fatalf("failure execution error = %v\nstdout=%s\nstderr=%s", err, out, errOut)
			}
			if code, emitted := output.StoredExitCode(store); !emitted || code != 1 {
				t.Fatalf("failure stored exit code=(%d,%v), want (1,true)", code, emitted)
			}
			var envelope struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("failure output is not JSON: %v\n%s", err, out)
			}
			if envelope.Outcome != "failure" {
				t.Fatalf("failure outcome = %q, want failure\n%s", envelope.Outcome, out)
			}
		})
	}
}

// requestId is optional, so an unconditional idempotent Safety contract would
// license unsafe replays (duplicate app creation) whenever the key is absent.
// The final schema projection must therefore say non_idempotent; conditional
// idempotency stays documented on the --request-id flag and in the Skill.
func TestCrossPlatformCoverageLiteappCreateSafetyDeclaresNonIdempotent(t *testing.T) {
	dev := devHandler{}.Command(&captureRunner{})
	create, _, err := dev.Find([]string{"liteapp", "create"})
	if err != nil {
		t.Fatalf("missing liteapp create: %v", err)
	}
	payload, ok := contractfinal.RuntimeContractFinal(create)
	if !ok {
		t.Fatal("liteapp create has no ContractFinal")
	}
	if got := payload.Safety.Idempotency; got != "non_idempotent" {
		t.Fatalf("liteapp create safety.idempotency = %q, want non_idempotent", got)
	}
	if create.Flags().Lookup("request-id") == nil {
		t.Fatal("liteapp create lost the conditional --request-id flag")
	}
}

func TestCrossPlatformCoverageLiteappDispatchesThroughDevAppProduct(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantParams map[string]any
	}{
		{
			name:     "create",
			args:     []string{"create", "--name", "周报助手", "--homepage-url", "https://example.com", "--pc-url", "https://example.com/pc", "--desc", "描述", "--request-id", "req-1", "--yes"},
			wantTool: liteappCreateTool,
			wantParams: map[string]any{
				"name":        "周报助手",
				"homepageUrl": "https://example.com",
				"pcUrl":       "https://example.com/pc",
				"desc":        "描述",
				"requestId":   "req-1",
			},
		},
		{
			name:     "update mixed",
			args:     []string{"update", "uni-5005426001", "--name", "新名称", "--redirect-uris", "https://a.example.com/cb, https://b.example.com/cb", "--yes"},
			wantTool: liteappUpdateTool,
			wantParams: map[string]any{
				"unifiedAppId": "uni-5005426001",
				"name":         "新名称",
				"redirectUris": []string{"https://a.example.com/cb", "https://b.example.com/cb"},
			},
		},
		{
			name:     "delete",
			args:     []string{"delete", "uni-5005426001", "--yes"},
			wantTool: liteappDeleteTool,
			wantParams: map[string]any{
				"unifiedAppId": "uni-5005426001",
			},
		},
		{
			name:     "list paging",
			args:     []string{"list", "--size", "10", "--offset", "20"},
			wantTool: liteappListTool,
			wantParams: map[string]any{
				"pageSize": 10,
				"offset":   20,
			},
		},
		{
			name:     "detail",
			args:     []string{"detail", "uni-5005426001"},
			wantTool: liteappDetailTool,
			wantParams: map[string]any{
				"unifiedAppId": "uni-5005426001",
			},
		},
		{
			name:     "credential",
			args:     []string{"credential", "uni-5005426001"},
			wantTool: liteappCredentialTool,
			wantParams: map[string]any{
				"unifiedAppId": "uni-5005426001",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			if _, err := executeLiteapp(t, runner, tc.args...); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := runner.last.CanonicalProduct; got != devAppProduct {
				t.Fatalf("CanonicalProduct = %q, want %q (liteapp 与 devapp 共用同一调用机制)", got, devAppProduct)
			}
			if runner.last.Tool != tc.wantTool {
				t.Fatalf("Tool = %q, want %q", runner.last.Tool, tc.wantTool)
			}
			if !reflect.DeepEqual(runner.last.Params, tc.wantParams) {
				t.Fatalf("Params = %#v, want %#v", runner.last.Params, tc.wantParams)
			}
		})
	}
}

func TestCrossPlatformCoverageLiteappWriteCommandsRequireYes(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--name", "周报助手", "--homepage-url", "https://example.com"},
		{"update", "uni-5005426001", "--desc", "新描述"},
		{"delete", "uni-5005426001"},
	} {
		runner := &captureRunner{}
		_, err := executeLiteapp(t, runner, args...)
		if err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("args=%v error = %v, want confirmation gate", args, err)
		}
		if runner.last.Tool != "" {
			t.Fatalf("args=%v must not dispatch before confirmation, got %q", args, runner.last.Tool)
		}
	}
}

func TestCrossPlatformCoverageLiteappDryRunPropagatesToInvocation(t *testing.T) {
	runner := &captureRunner{}
	if _, err := executeLiteapp(t, runner, "create", "--name", "周报助手", "--homepage-url", "https://example.com", "--dry-run"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !runner.last.DryRun {
		t.Fatalf("DryRun = false, want true propagation to runner barrier")
	}
}

func TestCrossPlatformCoverageLiteappUpdateValidationFailClosed(t *testing.T) {
	for _, args := range [][]string{
		{"update", "uni-5005426001", "--name", "   ", "--yes"},
		{"update", "uni-5005426001", "--desc", "", "--yes"},
		{"update", "uni-5005426001", "--redirect-uris", "   ", "--yes"},
		{"update", "uni-5005426001", "--redirect-uris", "https://a.example.com/cb,", "--yes"},
		{"update", "uni-5005426001", "--yes"},
		// 空白字段与有效字段同传：越过至少一个字段约束，命中 Validate 的空串拒绝。
		{"update", "uni-5005426001", "--name", "   ", "--desc", "新描述", "--yes"},
		{"update", "uni-5005426001", "--redirect-uris", "   ", "--desc", "新描述", "--yes"},
		{"update", "uni-5005426001", "--redirect-uris", "https://a.example.com/cb,", "--desc", "新描述", "--yes"},
	} {
		runner := &captureRunner{}
		_, err := executeLiteapp(t, runner, args...)
		if err == nil {
			t.Fatalf("args=%v must fail validation", args)
		}
		if runner.last.Tool != "" {
			t.Fatalf("args=%v must not dispatch on validation failure, got %q", args, runner.last.Tool)
		}
	}
}

// TestCrossPlatformCoverageLiteappValidateGetStringError 直接驱动
// validateLiteappUpdate 的 GetString 错误分支（注册类型与读取类型不一致的
// 防御路径，正常 CLI 解析不可达）。
func TestCrossPlatformCoverageLiteappValidateGetStringError(t *testing.T) {
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Int("redirect-uris", 0, "wrong typed flag")
	cmd.Flags().Set("redirect-uris", "42")
	err := validateLiteappUpdate(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--redirect-uris 参数无效") {
		t.Fatalf("error = %v, want GetString failure", err)
	}
}

func TestCrossPlatformCoverageLiteappListPageBoundsValidation(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--size", "0"},
		{"list", "--size", "51"},
		{"list", "--offset", "-1"},
	} {
		runner := &captureRunner{}
		_, err := executeLiteapp(t, runner, args...)
		if err == nil || !strings.Contains(err.Error(), "--size") && !strings.Contains(err.Error(), "--offset") {
			t.Fatalf("args=%v error = %v, want page bounds validation", args, err)
		}
		if runner.last.Tool != "" {
			t.Fatalf("args=%v must not dispatch on validation failure, got %q", args, runner.last.Tool)
		}
	}
}

func TestCrossPlatformCoverageLiteappRejectsMCPIDFlag(t *testing.T) {
	runner := &captureRunner{}
	_, err := executeLiteapp(t, runner, "list", "--mcp-id", "9999")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v, want unknown flag for --mcp-id", err)
	}
	if runner.last.Tool != "" {
		t.Fatalf("must not dispatch with --mcp-id, got %q", runner.last.Tool)
	}
}

func TestCrossPlatformCoverageLiteappServiceResultUnwrappedIntoEnvelope(t *testing.T) {
	runner := &devAppResponseRunner{response: map[string]any{
		"content": map[string]any{
			"success": true,
			// 与真实服务端 list_lite_apps 返回一致：列表键为 apps（非 items），
			// 条目字段为 appId/appName（ResultSpec 与联调实测口径）。
			"result": map[string]any{
				"total": 1,
				"apps":  []any{map[string]any{"appId": 5005426001, "appName": "周报助手"}},
			},
		},
	}}
	out, err := executeLiteapp(t, runner, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if envelope["ok"] != true {
		t.Fatalf("envelope ok = %#v, want true", envelope["ok"])
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope data missing: %#v", envelope)
	}
	if data["total"] != float64(1) {
		t.Fatalf("data.total = %#v, want 1 (ServiceResult envelope must be unwrapped)", data["total"])
	}
	apps, ok := data["apps"].([]any)
	if !ok || len(apps) != 1 {
		t.Fatalf("data.apps = %#v, want 1 entry keyed apps (真实服务端返回 apps)", data["apps"])
	}
	first, ok := apps[0].(map[string]any)
	if !ok || first["appId"] != float64(5005426001) || first["appName"] != "周报助手" {
		t.Fatalf("data.apps[0] = %#v", apps[0])
	}
	if _, wrapped := data["content"]; wrapped {
		t.Fatalf("data must not keep the ServiceResult content wrapper: %#v", data)
	}
}

// TestCrossPlatformCoverageLiteappCreateCredentialDegradation 覆盖 create 的
// 凭证生成降级路径：appKey/secret/secretMask 为 null 且带 warning 的成功响应
// 必须原样进入 envelope（ResultSpec 已按可空声明）。
func TestCrossPlatformCoverageLiteappCreateCredentialDegradation(t *testing.T) {
	runner := &devAppResponseRunner{response: map[string]any{
		"content": map[string]any{
			"success": true,
			"result": map[string]any{
				"appId":        5005426001,
				"appKey":       nil,
				"secret":       nil,
				"secretMask":   nil,
				"unifiedAppId": "uni-1",
				"status":       "PUBLISHED",
				"requestId":    "req-1",
				"warning":      "OAuth 凭证生成降级",
			},
		},
	}}
	out, err := executeLiteapp(t, runner,
		"create", "--name", "周报助手", "--homepage-url", "https://example.com", "--yes")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if envelope["ok"] != true {
		t.Fatalf("degraded create must remain ok=true: %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope data missing: %#v", envelope)
	}
	if v, present := data["appKey"]; !present || v != nil {
		t.Fatalf("appKey = %#v (present=%v), want null", v, present)
	}
	if v, present := data["secret"]; !present || v != nil {
		t.Fatalf("secret = %#v (present=%v), want null", v, present)
	}
	if v, present := data["secretMask"]; !present || v != nil {
		t.Fatalf("secretMask = %#v (present=%v), want null", v, present)
	}
	if data["warning"] != "OAuth 凭证生成降级" {
		t.Fatalf("warning = %#v", data["warning"])
	}
}

func TestCrossPlatformCoverageLiteappServiceFailureBecomesEnvelopeError(t *testing.T) {
	runner := &devAppResponseRunner{response: map[string]any{
		"content": map[string]any{
			"success":   false,
			"errorCode": "E_USER_QUOTA_EXCEEDED",
			"errorMsg":  "quota exceeded",
		},
	}}
	out, err := executeLiteapp(t, runner, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if envelope["ok"] != false {
		t.Fatalf("envelope ok = %#v, want false for success=false payload", envelope["ok"])
	}
	errorObj, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope error missing: %#v", envelope)
	}
	if errorObj["upstream_code"] != "E_USER_QUOTA_EXCEEDED" && errorObj["upstreamCode"] != "E_USER_QUOTA_EXCEEDED" {
		t.Fatalf("error = %#v, want upstream errorCode preserved", errorObj)
	}
}

func TestCrossPlatformCoverageLiteappContractDetailsAndDeleteNormalization(t *testing.T) {
	group := newLiteappGroup(&captureRunner{})
	for _, tc := range []struct {
		name       string
		wantEffect string
		wantFields []string
	}{
		{name: "create", wantEffect: "write", wantFields: []string{"appName", "homepageUrl", "pcUrl", "desc", "iconUrl", "orgId", "unifiedAppId", "status", "requestId", "warning"}},
		{name: "update", wantEffect: "write", wantFields: []string{"unifiedAppId"}},
		{name: "list", wantEffect: "read", wantFields: []string{"unifiedAppId"}},
		{name: "detail", wantEffect: "read", wantFields: []string{"unifiedAppId"}},
		{name: "delete", wantEffect: "destructive", wantFields: []string{"timeToDel"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := group.Find([]string{tc.name})
			if err != nil {
				t.Fatal(err)
			}
			final, ok := contractfinal.RuntimeContractFinal(cmd)
			if !ok || final.Result == nil {
				t.Fatal("missing reviewed result contract")
			}
			if final.Safety.Effect != tc.wantEffect {
				t.Fatalf("effect = %q, want %q", final.Safety.Effect, tc.wantEffect)
			}
			var schema map[string]any
			if err := json.Unmarshal(final.Result.DataSchema, &schema); err != nil {
				t.Fatal(err)
			}
			properties, _ := schema["properties"].(map[string]any)
			resultProperties := properties
			if tc.name == "list" {
				apps, _ := properties["apps"].(map[string]any)
				items, _ := apps["items"].(map[string]any)
				resultProperties, _ = items["properties"].(map[string]any)
			}
			for _, field := range tc.wantFields {
				if _, ok := resultProperties[field]; !ok {
					t.Errorf("missing result field %q", field)
				}
			}
			for _, field := range []string{"update", "list", "detail"} {
				if tc.name != field {
					continue
				}
				unified := resultProperties["unifiedAppId"]
				unifiedProperty, _ := unified.(map[string]any)
				if got := unifiedProperty["type"]; !reflect.DeepEqual(got, []any{"string", "null"}) {
					t.Fatalf("unifiedAppId type = %#v, want nullable string", got)
				}
			}
		})
	}

	for _, timestamp := range []any{int64(1700000000000), float64(1700000000001), json.Number("1700000000002")} {
		result := normalizeDevAppToolResult(liteappDeleteTool, executor.Result{Response: map[string]any{"content": timestamp}})
		content, ok := result.Response["content"].(map[string]any)
		if !ok || content["timeToDel"] != timestamp {
			t.Fatalf("timestamp %#v normalized to %#v", timestamp, result.Response["content"])
		}
	}
	original := map[string]any{"timeToDel": int64(7)}
	result := normalizeDevAppToolResult(liteappDeleteTool, executor.Result{Response: map[string]any{"content": original}})
	if got := result.Response["content"]; !reflect.DeepEqual(got, original) {
		t.Fatalf("existing map changed: %#v", got)
	}
}

func TestCrossPlatformCoverageLiteappCreateUnpublishedWarningAndNullableFields(t *testing.T) {
	group := newLiteappGroup(&captureRunner{})
	cmd, _, err := group.Find([]string{"create"})
	if err != nil {
		t.Fatal(err)
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Result == nil {
		t.Fatal("missing create result contract")
	}
	if final.Safety.Idempotency != "non_idempotent" {
		t.Fatalf("create idempotency = %q, want non_idempotent", final.Safety.Idempotency)
	}
	var schema map[string]any
	if err := json.Unmarshal(final.Result.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, field := range []string{"appKey", "secret", "secretMask", "unifiedAppId", "status", "requestId", "warning"} {
		property, _ := properties[field].(map[string]any)
		if got := property["type"]; got == nil {
			t.Fatalf("field %s missing type", field)
		}
	}
	if !strings.Contains(final.Description, "UNPUBLISHED") || !strings.Contains(final.Description, "request-id") || !strings.Contains(final.Description, "warning") {
		t.Fatalf("create description misses lifecycle semantics: %q", final.Description)
	}
	if !strings.Contains(final.Selection.AgentSummary, "UNPUBLISHED") || !strings.Contains(final.Selection.AgentSummary, "warning") {
		t.Fatalf("create selection misses lifecycle semantics: %q", final.Selection.AgentSummary)
	}
}

func TestCrossPlatformCoverageLiteappLeafResultContractsAreBusinessDataOnly(t *testing.T) {
	group := newLiteappGroup(&captureRunner{})
	for _, tc := range []struct {
		name       string
		wantDryRun bool
		wantSecret string
	}{
		{name: "create", wantDryRun: true, wantSecret: "secret"},
		{name: "update", wantDryRun: true},
		{name: "delete", wantDryRun: true},
		{name: "list"},
		{name: "detail", wantSecret: "secretMask"},
		{name: "credential", wantSecret: "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := group.Find([]string{tc.name})
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Annotations == nil {
				t.Fatal("missing contract annotations")
			}
			final, ok := contractfinal.RuntimeContractFinal(cmd)
			if !ok || final.Result == nil {
				t.Fatal("missing reviewed result contract")
			}
			if tc.wantDryRun && final.DryRun == nil {
				t.Fatal("write leaf must publish dry-run capability")
			}
			if tc.wantSecret != "" {
				found := false
				for _, path := range final.Result.SensitivePaths {
					if path == tc.wantSecret {
						found = true
					}
				}
				if !found {
					t.Fatalf("contract missing sensitive path %q", tc.wantSecret)
				}
			}
			if strings.Contains(string(final.Result.DataSchema), `"ok"`) || strings.Contains(string(final.Result.DataSchema), `"meta"`) {
				t.Fatal("result schema must not describe runtime wrapper fields")
			}
		})
	}
}

// TestCrossPlatformCoverageLiteappPositionalFramework 覆盖位置参数框架在
// corecmd 的声明期 panic 分支与取值校验分支（changed-code coverage gate）。
func TestCrossPlatformCoverageLiteappPositionalFramework(t *testing.T) {
	// probeContract 提供通过框架 Contract 校验的最小完整声明，
	// 让构造抵达 validatePositionalDecls 的 panic 分支。
	probeContract := func(name string) LeafContract {
		return LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "dev", Name: name, CanonicalPath: "dev." + name,
				CLIPath: "probe", PrimaryCLIPath: "probe",
			},
			Description: "probe",
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "probe",
				UseWhen:      []string{"probe"},
				AvoidWhen:    []string{"probe"},
				Examples:     []string{"dws probe"},
			},
		}
	}

	// 声明期 panic：空 Name / 非法 Kind / bind key 与 flag 冲突。
	panics := []struct {
		name string
		spec LeafSpec
		want string
	}{
		{
			name: "empty positional name",
			spec: LeafSpec{Use: "bad", Short: "x", Tool: "t",
				Positionals: []LeafPositional{{Name: "  ", Kind: LeafString}},
				Contract:    probeContract("bad_empty")},
			want: "empty Name",
		},
		{
			name: "unsupported kind",
			spec: LeafSpec{Use: "bad", Short: "x", Tool: "t",
				Positionals: []LeafPositional{{Name: "id", Kind: LeafFlagKind(999)}},
				Contract:    probeContract("bad_kind")},
			want: "only KindString/KindInt",
		},
		{
			name: "bind key conflict",
			spec: LeafSpec{Use: "bad", Short: "x", Tool: "t",
				Flags:       []LeafFlag{{Name: "app-id", Bind: "appId"}},
				Positionals: []LeafPositional{{Name: "appId", Kind: LeafInt, Bind: "appId"}},
				Contract:    probeContract("bad_conflict")},
			want: "conflicts with a flag",
		},
	}
	for _, tc := range panics {
		t.Run("panic/"+tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Fatalf("panic = %v, want containing %q", r, tc.want)
				}
			}()
			_ = NewLeafCommand(tc.spec)
		})
	}

	// DeclareLeafMetadata（metadata-only 模式）拒绝 Positionals。
	func() {
		defer func() {
			r := recover()
			if r == nil || !strings.Contains(fmt.Sprint(r), "Positionals must be empty") {
				t.Fatalf("DeclareLeafMetadata panic = %v, want Positionals rejection", r)
			}
		}()
		cmd := &cobra.Command{Use: "meta"}
		DeclareLeafMetadata(cmd, LeafSpec{
			Positionals: []LeafPositional{{Name: "id"}},
			Contract:    LeafContract{Description: "x"},
		})
	}()

	// 取值校验：合成叶子覆盖 KindString 必填为空 / 可选为空（continue）/ Trim 绑定。
	leaf := NewLeafCommand(LeafSpec{
		Use:   "probe <note>",
		Short: "probe",
		Tool:  "probe_tool",
		Positionals: []LeafPositional{
			{Name: "mode", Kind: LeafString, Required: true, Trim: true, Bind: "mode"},
			{Name: "note", Kind: LeafString, Trim: true, Bind: "note"},
		},
		Contract: probeContract("probe_one"),
		Call: func(cmd *cobra.Command, tool string, args map[string]any) error {
			return nil
		},
	})
	probeRoot := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	probeRoot.PersistentFlags().Bool("dry-run", false, "")
	probeRoot.AddCommand(leaf)

	probeRoot.SetArgs([]string{"probe", "", "  hello  "})
	if err := corecmd.ExecuteForTest(probeRoot); err == nil || !strings.Contains(err.Error(), "缺少必填参数 mode") {
		t.Fatalf("required-empty error = %v", err)
	}

	var seen map[string]any
	leaf2 := NewLeafCommand(LeafSpec{
		Use:         "probe <note>",
		Short:       "probe",
		Tool:        "probe_tool",
		Positionals: []LeafPositional{{Name: "note", Kind: LeafString, Trim: true}}, // 无 Bind：bind key 回退 Name（positionalBindKey）
		ConstParams: map[string]any{"probeConst": true},                             // 覆盖声明期 ConstParams 收录分支
		Contract:    probeContract("probe_two"),
		Call: func(cmd *cobra.Command, tool string, args map[string]any) error {
			seen = args
			return nil
		},
	})
	probeRoot2 := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	probeRoot2.PersistentFlags().Bool("dry-run", false, "")
	probeRoot2.AddCommand(leaf2)
	var out2 bytes.Buffer
	probeRoot2.SetOut(&out2)
	probeRoot2.SetErr(&out2)
	probeRoot2.SetArgs([]string{"probe", ""})
	if err := corecmd.ExecuteForTest(probeRoot2); err != nil {
		t.Fatalf("optional empty positional: %v", err)
	}
	if _, bound := seen["note"]; bound {
		t.Fatalf("empty optional positional must be skipped, got %#v", seen)
	}
	probeRoot2.SetArgs([]string{"probe", "  hi  "})
	if err := corecmd.ExecuteForTest(probeRoot2); err != nil {
		t.Fatalf("trimmed bind: %v", err)
	}
	if seen["note"] != "hi" {
		t.Fatalf("note = %#v, want trimmed hi", seen["note"])
	}
	if seen["probeConst"] != true {
		t.Fatalf("probeConst = %#v, want ConstParams merged", seen["probeConst"])
	}
}
