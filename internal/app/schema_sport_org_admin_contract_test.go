// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestSportOrgAdminLeafSchemaPublishesTriStateResult 验证 sport.is_org_admin
// 的最终 embedded Schema 投影：full 与 compact 都必须发布布尔三态 result（true=
// 管理员 / false=确认非管理员 / success=false 时报错），无业务入参，且两个投影
// 的 Result 完全一致。runtime delivery 见 TestSportOrgAdminRuntimeDelivery。
func TestSportOrgAdminLeafSchemaPublishesTriStateResult(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "sport org admin")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "sport org admin", "--compact")
	for name, leaf := range map[string]map[string]any{"full": full, "compact": compact} {
		if len(leaf) == 0 {
			t.Fatalf("%s 查询返回空 Schema 投影", name)
		}
		description, _ := leaf["description"].(string)
		if !strings.Contains(description, "管理员") {
			t.Errorf("%s 投影缺少管理员判定语义：%q", name, description)
		}
		parameters, _ := leaf["parameters"].(map[string]any)
		if len(parameters) != 0 {
			t.Errorf("%s 无业务入参工具投影出参数：%#v", name, parameters)
		}
		result, ok := leaf["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s 缺少 result 投影", name)
		}
		schema, _ := result["data_schema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		boolResult, _ := properties["result"].(map[string]any)
		if boolResult["type"] != "boolean" {
			t.Errorf("%s result 应为布尔：%#v", name, boolResult)
		}
		if text, _ := boolResult["description"].(string); !strings.Contains(text, "管理员") || !strings.Contains(text, "success=false") {
			t.Errorf("%s result 描述缺少三态语义：%q", name, text)
		}
		if _, exists := properties["success"]; !exists {
			t.Errorf("%s 缺少 success 字段投影", name)
		}
	}
	if !reflect.DeepEqual(full["result"], compact["result"]) {
		t.Errorf("compact 必须保留与 full 相同的 Result：\nfull=%#v\ncompact=%#v", full["result"], compact["result"])
	}
	if full["description"] != compact["description"] || full["cli_path"] != compact["cli_path"] {
		t.Errorf("full 与 compact 投影不一致：\nfull=%#v\ncompact=%#v", full["description"], compact["description"])
	}
}

// TestSportOrgAdminRuntimeDelivery 经 NewRootCommand → runtimeRunner → HTTP →
// ToolCaller adapter → helper 的真实生产链路执行 sport org admin，并用最终
// embedded Schema 发布的 data_schema 校验实际交付的 data。helper 同包测试的 fake
// ToolCaller 会绕过 runner 的业务错误拦截与 success 补写，不能代替本用例。
func TestSportOrgAdminRuntimeDelivery(t *testing.T) {
	validators := map[string]*jsonschema.Schema{}
	for _, compact := range []bool{false, true} {
		query := []string{"--cli-path", "sport org admin"}
		name := "full"
		if compact {
			query, name = append(query, "--compact"), "compact"
		}
		result := executeShortcutSchemaQuery(t, query...)["result"].(map[string]any)
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		resource := "https://dws.test/sport-org-admin-" + name + ".json"
		if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile(resource)
		if err != nil {
			t.Fatal(err)
		}
		validators[name] = schema
	}

	for _, tc := range []struct {
		name    string
		text    string
		want    any
		wantErr []string
	}{
		{name: "管理员", text: `{"success":true,"result":true,"errorCode":null,"errorMsg":null}`, want: true},
		{name: "确认非管理员", text: `{"success":true,"result":false,"errorCode":null,"errorMsg":null}`, want: false},
		{name: "组织服务不可用", text: `{"success":false,"result":null,"errorCode":"QW_ADMIN_CHECK_FAILED","errorMsg":"组织管理员服务查询不可用"}`, wantErr: []string{"组织管理员服务查询不可用", "不能当作非管理员", "请稍后重试"}},
		{name: "result 非布尔", text: `{"success":true,"result":"yes"}`, wantErr: []string{"缺少布尔 result"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				calls.Add(1)
				if request.Method != "tools/call" || request.Params.Name != "is_org_admin" || len(request.Params.Arguments) != 0 {
					t.Errorf("应以空参数调用 is_org_admin：%#v", request)
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID,
					"result": map[string]any{"content": []map[string]any{{"type": "text", "text": tc.text}}},
				}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Cleanup(CloseFileLogger)
			t.Setenv("DO_NOT_TRACK", "1")
			t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
			t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
			t.Setenv("DINGTALK_SPORT_MCP_URL", server.URL)
			args := []string{"--token", "synthetic-sport-token", "sport", "org", "admin", "--format", "json"}
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			// Runtime initialization can recreate the formatter; capture the process
			// stream used before and after setup.
			capture, err := os.CreateTemp(t.TempDir(), "sport-org-admin-stdout-*")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = capture.Close() })
			testseam.Swap(t, &os.Stdout, capture)
			helpers.InitDepsForTest(t, helpers.GetCaller())
			root := NewRootCommand()
			var stderr bytes.Buffer
			root.SetOut(capture)
			root.SetErr(&stderr)
			root.SetArgs(args)
			_, err = root.ExecuteC()
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls=%d，应且仅应调用一次 is_org_admin", calls.Load())
			}
			if tc.wantErr != nil {
				var failure *apperrors.Error
				if !errors.As(err, &failure) || failure.Category != apperrors.CategoryAPI || apperrors.ExitCode(err) != 1 {
					t.Fatalf("应返回 API 错误而不是判定结果：%v", err)
				}
				text := failure.Message + "\n" + failure.Hint
				for _, want := range tc.wantErr {
					if !strings.Contains(text, want) {
						t.Fatalf("错误缺少 %q：message=%q hint=%q", want, failure.Message, failure.Hint)
					}
				}
				if tc.name == "组织服务不可用" && failure.ServerDiag.ServerErrorCode != "QW_ADMIN_CHECK_FAILED" {
					t.Fatalf("应保留服务端错误码：%#v", failure.ServerDiag)
				}
				return
			}
			if err != nil {
				t.Fatalf("合法判定不应报错：%v; stderr=%s", err, &stderr)
			}
			stdout, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout, &envelope); err != nil {
				t.Fatalf("解析统一结果失败：%v\n%s", err, stdout)
			}
			data, _ := envelope["data"].(map[string]any)
			if envelope["ok"] != true || envelope["outcome"] != "success" || data["success"] != true || data["result"] != tc.want {
				t.Fatalf("三态判定交付错误：%s", stdout)
			}
			for name, schema := range validators {
				if err := schema.Validate(envelope["data"]); err != nil {
					t.Fatalf("%s data_schema 不接受实际交付的 data：%v\n%s", name, err, stdout)
				}
			}
		})
	}
}
