// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeConnectRemovedBindingCommands(t *testing.T) {
	for _, action := range []string{"bind", "rebind"} {
		t.Run(action, func(t *testing.T) {
			cmd := newDeapConnectCommand()
			for _, child := range cmd.Commands() {
				if child.Name() == action {
					t.Fatalf("connect %s 仍然注册", action)
				}
			}
		})
	}
}

// 用真实本地绑定文件验证替代流程；MCP 和宿主均隔离为测试替身。
func TestCrossPlatformCoverageEmployeeUnbindThenConnect(t *testing.T) {
	for _, tc := range []struct {
		name, device            string
		rejectUnbind, failStart bool
	}{
		{name: "same_device"}, {name: "new_device", device: "new-device"},
		{name: "unbind_rejected", rejectUnbind: true}, {name: "restart_after_registration_failure", failStart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSuccessfulConnectSeams(t)
			testseam.Swap(t, &deapConnectSaveBinding, saveDigitalEmployeeBinding)
			b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: "employee-corp:employee-user", Channel: "custom", OperatorOpenDingTalkID: "operator-open", BindingRevision: 7, BindingState: "bound", DesiredState: "running", DeviceID: "old-device", RuntimeBindingID: "old-binding"}
			device, err := employeeDeviceID(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			b.DeviceID = device
			if err := updateEmployeeBinding(b); err != nil {
				t.Fatal(err)
			}
			caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
			response := `{"success":true,"data":true}`
			if tc.rejectUnbind {
				response = `{"success":false}`
			}
			caller.responses["deap-dev/unbind_local_agent"] = []string{response}
			InitDepsForTest(t, caller)
			registered := false
			testseam.Swap(t, &deapConnectRegisterDSH, func(_ context.Context, payload map[string]any) (string, error) {
				if payload["bindingRevision"] != uint64(9) {
					t.Fatalf("错误的新绑定代数: %v", payload["bindingRevision"])
				}
				if tc.failStart {
					return "", fmt.Errorf("injected registration failure")
				}
				registered = true
				return "created", nil
			})
			testseam.Swap(t, &employeeDSHControl, func(_ context.Context, _ digitalEmployeeBinding, action string) (employeeDSHState, error) {
				if action == "stop" {
					return employeeDSHState{Released: true, RuntimeState: "stopped"}, nil
				}
				if action == "start" && !registered {
					t.Fatal("DSH 必须先注册再启动")
				}
				return employeeDSHState{RuntimeState: "running", TransportReady: true, ExecutorReady: true}, nil
			})
			err = runEmployeeUnbind(lifecycleCmd(t, "unbind", b.AgentUUID))
			if tc.rejectUnbind {
				if err == nil {
					t.Fatal("解绑拒绝被忽略")
				}
				cmd := newConnectTestCommand(t, false)
				if err := cmd.RunE(cmd, nil); err == nil {
					t.Fatal("解绑失败后仍允许切换 Agent")
				}
				if len(caller.tokenCalls) != 1 || registered {
					t.Fatal("解绑失败后发起了新绑定或启动")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cmd := newConnectTestCommand(t, false)
			if tc.device != "" {
				if err := cmd.Flags().Set("device-id", tc.device); err != nil {
					t.Fatal(err)
				}
			}
			err = cmd.RunE(cmd, nil)
			if tc.failStart {
				if err == nil || !strings.Contains(err.Error(), "connect restart") {
					t.Fatalf("缺少启动恢复指引: %v", err)
				}
				tc.failStart = false
				restart := newDigitalEmployeeRestartCommand()
				restart.SetContext(context.Background())
				restart.SetOut(io.Discard)
				if err := restart.Flags().Set("agent-uuid", b.AgentUUID); err != nil {
					t.Fatal(err)
				}
				if err := runDigitalEmployeeLifecycle(restart, "restart"); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			current, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
			if err != nil || current.RuntimeBindingID != "binding-created" || current.BindingRevision != 9 || current.BindingState != "bound" || current.Channel != "dsh" || !registered {
				t.Fatalf("新绑定未生效: %+v err=%v", current, err)
			}
			if tc.device == "" && current.DeviceID != b.DeviceID {
				t.Fatal("同设备连接未复用稳定设备标识")
			}
			if tc.device != "" && current.DeviceID != tc.device {
				t.Fatal("未使用指定的新设备")
			}
			var calls []string
			for _, call := range caller.tokenCalls {
				if strings.HasSuffix(call.toolName, "_local_agent") {
					calls = append(calls, call.toolName)
					if call.toolName == "unbind_local_agent" && call.args["runtimeBindingId"] != "old-binding" {
						t.Fatal("解绑未使用旧绑定 ID")
					}
					if call.toolName == "bind_local_agent" {
						if _, ok := call.args["runtimeBindingId"]; ok {
							t.Fatal("新绑定仍携带旧 ID")
						}
					}
				}
			}
			if strings.Join(calls, ",") != "unbind_local_agent,bind_local_agent" {
				t.Fatalf("绑定顺序错误或重复请求: %v", calls)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeConnectMigratesLegacyBinding(t *testing.T) {
	setupSuccessfulConnectSeams(t)
	testseam.Swap(t, &deapConnectSaveBinding, saveDigitalEmployeeBinding)
	b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: "employee-corp:employee-user", Channel: "dsh", OperatorOpenDingTalkID: "operator-open", BindingRevision: 7, BindingState: "bound", DesiredState: "stopped"}
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
	InitDepsForTest(t, caller)
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{Released: true, RuntimeState: "stopped"}, nil
	})
	cmd := newConnectTestCommand(t, true)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(caller.tokenCalls) != 0 {
		t.Fatal("预览调用了服务端")
	}
	current, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current != b {
		t.Fatal("预览改写了旧绑定")
	}
	cmd = newConnectTestCommand(t, false)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	current, err = loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current.RuntimeBindingID != "binding-created" || current.DeviceID == "" || current.BindingRevision != 7 {
		t.Fatalf("旧连接补登记失败: %+v %v", current, err)
	}
}
