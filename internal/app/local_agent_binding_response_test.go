// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// 服务端业务对象经两种 MCP 封装抵达 helper 时必须保持字段层级和错误语义。
func TestCrossPlatformCoverageEmployeeBindingMCPResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name, action, response string
	}{
		{"bind_success", "bind", `{"success":true,"data":{"runtimeBindingId":"binding-a","runtimeId":"runtime-a","status":"ACTIVE"}}`},
		{"bind_invalid", "bind", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_INVALID_ARGUMENT","errorMsg":"Invalid Local Agent binding parameters"}`},
		{"rebind_success", "rebind", `{"success":true,"data":{"runtimeBindingId":"binding-b","runtimeId":"runtime-b","status":"ACTIVE"}}`},
		{"rebind_conflict", "rebind", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_CONFLICT","errorMsg":"Binding changed or has active or recovering tasks"}`},
		{"unbind_success", "unbind", `{"success":true,"data":true,"message":"本次解绑成功。"}`},
		{"unbind_already_released", "unbind", `{"success":true,"data":true,"message":"该绑定此前已解除，本次未执行新的解绑操作。"}`},
		{"unbind_forbidden", "unbind", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_FORBIDDEN","errorMsg":"Local Agent binding access denied"}`},
	} {
		for _, shape := range []string{"text", "structured"} {
			t.Run(tc.name+"/"+shape, func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				t.Setenv("DINGTALK_DEAP_DEV_MCP_URL", "https://binding.example.test")
				var business map[string]any
				if err := json.Unmarshal([]byte(tc.response), &business); err != nil {
					t.Fatal(err)
				}
				calls := 0
				client := transport.NewClient(&http.Client{Transport: employeeBindingRoundTripper(func(request *http.Request) (*http.Response, error) {
					calls++
					var rpc map[string]any
					if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
						return nil, err
					}
					result := map[string]any{"content": []any{map[string]any{"type": "text", "text": tc.response}}}
					if shape == "structured" {
						result["structuredContent"] = business
					}
					payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": rpc["id"], "result": result})
					if err != nil {
						return nil, err
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
				})})
				runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
				testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
					return nil
				})
				caller := &toolCallerAdapter{runner: runner, flags: &GlobalFlags{}}
				result, err := caller.CallToolWithToken(context.Background(), "supervisor-test-token", "deap-dev", tc.action+"_local_agent", map[string]any{"agentUuid": "employee-test"})
				if calls != 1 {
					t.Fatalf("calls=%d, want 1", calls)
				}
				if business["success"] == false {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "business_error" || typed.ServerDiag.ServerErrorCode != business["errorCode"] || !strings.Contains(err.Error(), business["errorMsg"].(string)) {
						t.Fatalf("business error lost: %v", err)
					}
					return
				}
				if err != nil || result == nil || len(result.Content) != 1 {
					t.Fatalf("adapter result=%v err=%v", result, err)
				}
				var got map[string]any
				if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil || !reflect.DeepEqual(got, business) {
					t.Fatalf("business object changed: got=%v err=%v", got, err)
				}
			})
		}
	}
}
