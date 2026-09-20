// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type publishedQueryErrorCaller struct {
	digitalEmployeeProtocolCaller
	queryError error
}

func (c *publishedQueryErrorCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	if tool == deapAgentDetailTool && args["type"] == "published" {
		c.calls = append(c.calls, deapAgentCall{productID: product, toolName: tool, args: args})
		return nil, c.queryError
	}
	return c.digitalEmployeeProtocolCaller.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageDigitalEmployeePreservesPublishedQueryErrors(t *testing.T) {
	for _, route := range []string{"connect", "login"} {
		t.Run(route, func(t *testing.T) {
			for _, tc := range []struct{ name, code, message string }{
				{"network_timeout", CodeNetworkTimeout, "published detail request timed out"},
				{"network_unreachable", CodeNetworkUnreachable, "published detail connection refused"},
				{"permission_denied", CodeAuthPermission, "published detail permission denied"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					source := &CLIError{Code: tc.code, Message: tc.message, Suggestion: "原始恢复建议", Operation: deapAgentDetailTool}
					caller := &publishedQueryErrorCaller{digitalEmployeeProtocolCaller: digitalEmployeeProtocolCaller{responses: map[string][]string{
						"deap-dev/get_digital_employee_detail": {`{"success":true,"data":{"digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}}`},
					}}, queryError: source}
					InitDepsForTest(t, caller)
					setupConnectSupervisorSeams(t)
					testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
						t.Fatal("查询失败不得换票")
						return nil, nil
					})
					testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error { t.Fatal("查询失败不得写绑定"); return nil })
					testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
						t.Fatal("查询失败不得启动宿主")
						return "", nil
					})
					cmd := newConnectTestCommand(t, false)
					expectedCalls := 2
					if route == "login" {
						cmd = newManageLoginTestCommand(t, false)
						expectedCalls = 1
					}
					err := cmd.RunE(cmd, nil)
					var got *CLIError
					if !errors.Is(err, source) || !errors.As(err, &got) || got.Code != tc.code || got.Suggestion != source.Suggestion {
						t.Fatalf("原始错误分类或恢复建议丢失: %v", err)
					}
					if strings.Contains(err.Error(), "尚未发布") || !strings.Contains(err.Error(), tc.message) {
						t.Fatalf("错误原因被覆盖: %v", err)
					}
					if len(caller.calls) != expectedCalls || len(caller.tokenCalls) != 0 {
						t.Fatalf("查询失败后仍执行后续调用: %#v", caller.calls)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageDigitalEmployeePublishedResponseBoundaries(t *testing.T) {
	for _, route := range []string{"connect", "login"} {
		for _, tc := range []struct {
			name, response, want string
			unpublished          bool
		}{
			{"empty_data", `{"success":true,"data":null}`, "尚未发布", true},
			{"empty_object", `{"success":true,"data":{}}`, "尚未发布", true},
			{"empty_result", `{"success":true,"result":null}`, "尚未发布", true},
			{"permission", `{"success":false,"errorCode":"FORBIDDEN","errorMsg":"permission denied"}`, "permission denied", false},
			{"invalid_json", `{invalid`, "invalid MCP JSON response", false},
			{"invalid_data", `{"success":true,"data":"invalid"}`, "响应格式无效", false},
			{"missing_success", `{"data":null}`, "缺少成功状态", false},
			{"missing_data", `{"success":true}`, "缺少登录所需的内部身份信息", false},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				responses := []string{tc.response}
				if route == "connect" {
					responses = append([]string{`{"success":true,"data":{"mainProgramType":"local_agent"}}`}, responses...)
				}
				caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/get_digital_employee_detail": responses}}
				InitDepsForTest(t, caller)
				setupConnectSupervisorSeams(t)
				testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
					t.Fatal("无发布身份不得换票")
					return nil, nil
				})
				cmd := newConnectTestCommand(t, false)
				if route == "login" {
					cmd = newManageLoginTestCommand(t, false)
				}
				err := cmd.RunE(cmd, nil)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("错误 = %v, 应包含 %q", err, tc.want)
				}
				if strings.Contains(err.Error(), "尚未发布") != tc.unpublished {
					t.Fatalf("发布状态误判: %v", err)
				}
				if len(caller.calls) != len(responses) || len(caller.tokenCalls) != 0 {
					t.Fatal("异常响应后仍发起后续调用")
				}
			})
		}
	}
}
