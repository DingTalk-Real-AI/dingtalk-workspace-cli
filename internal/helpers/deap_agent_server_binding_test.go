// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func setupServerBindingSupervisor(t *testing.T) {
	t.Helper()
	auth.SetRuntimeProfile("")
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) {
		return &auth.ProfilesConfig{CurrentProfile: "corp:supervisor"}, nil
	})
	testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) {
		return &auth.TokenData{CorpID: "corp", UserID: "supervisor", AccessToken: "supervisor-test-token"}, nil
	})
}

func TestCrossPlatformCoverageEmployeeServerReceiptReplayAndIdentity(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/de_local_agent_rebind": {`{"success":true,"data":"binding-new"}`}}}
	InitDepsForTest(t, caller)
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	_ = cmd.Flags().Set("local-agent-name", "办公室 Agent")
	_ = cmd.Flags().Set("extensions", "private-extension-value")
	for i := 0; i < 2; i++ {
		id, err := mutateEmployeeServerBinding(cmd, b, "rebind", "device-new")
		if err != nil || id != "binding-new" {
			t.Fatalf("receipt: %q %v", id, err)
		}
	}
	if len(caller.tokenCalls) != 1 || len(caller.calls) != 0 || caller.tokens[0] != "supervisor-test-token" {
		t.Fatalf("unexpected calls: %v", caller.tokenCalls)
	}
	args := caller.tokenCalls[0].args
	request, ok := args["RebindLocalAgentRequest"].(map[string]any)
	if !ok || len(args) != 1 || request["agentUuid"] != b.AgentUUID || request["runtimeBindingId"] != "binding-old" || request["deviceId"] != "device-new" || request["localAgentName"] != "办公室 Agent" || request["extensions"] != "private-extension-value" {
		t.Fatalf("payload: %+v", args)
	}
	for _, key := range []string{"identity", "userId", "orgId", "corpId"} {
		if _, ok := request[key]; ok {
			t.Fatalf("identity leaked: %s", key)
		}
	}
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if err != nil || strings.Contains(string(raw), "private-extension-value") || strings.Contains(string(raw), "supervisor-test-token") {
		t.Fatalf("unsafe receipt: %v", err)
	}
	if _, err := mutateEmployeeServerBinding(cmd, b, "rebind", "different-device"); err == nil {
		t.Fatal("changed request bypassed recovery")
	}
	if err := checkEmployeeServerOperation(b); err == nil {
		t.Fatal("old binding may not start after successful rebind")
	}
	b.RuntimeBindingID = "binding-new"
	if err := checkEmployeeServerOperation(b); err != nil {
		t.Fatal(err)
	}
	if err := checkEmployeeServerOperation(b); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeServerFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		rejected       bool
	}{
		{"busy", `{"success":false,"errorMsg":"private-error","errorCode":"BUSY"}`, true},
		{"missing_success", `{"data":"id"}`, false},
		{"missing_id", `{"success":true,"data":null}`, false},
		{"nested_id", `{"success":true,"data":{"unrelated":{"runtimeBindingId":"id"}}}`, false},
		{"old_id", `{"success":true,"data":"binding-old"}`, false},
		{"invalid_json", `private-error`, false},
		{"trailing_json", `{"success":true,"data":"id"}{}`, false},
		{"transport_lost", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
			if tc.response != "" {
				caller.responses["deap-dev/de_local_agent_rebind"] = []string{tc.response, `{"success":true,"data":"recovered"}`}
			}
			InitDepsForTest(t, caller)
			cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
			_, err := mutateEmployeeServerBinding(cmd, b, "rebind", "new-device")
			if err == nil || strings.Contains(err.Error(), "private-error") {
				t.Fatalf("failure: %v", err)
			}
			_, again := mutateEmployeeServerBinding(cmd, b, "rebind", "new-device")
			if tc.rejected {
				if again != nil || len(caller.tokenCalls) != 2 {
					t.Fatalf("rejected retry: %v", again)
				}
			} else {
				if again == nil || len(caller.tokenCalls) != 1 {
					t.Fatal("uncertain rebind was retried")
				}
			}
			current, e := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
			if e != nil || current.RuntimeBindingID != b.RuntimeBindingID {
				t.Fatal("old ID lost")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeServerUnbindRetriesExactID(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
	if _, err := mutateEmployeeServerBinding(cmd, b, "unbind", ""); err == nil {
		t.Fatal("missing failure")
	}
	caller.responses["deap-dev/de_local_agent_unbind"] = []string{`{"success":true,"data":true}`}
	if id, err := mutateEmployeeServerBinding(cmd, b, "unbind", ""); err != nil || id != b.RuntimeBindingID {
		t.Fatalf("unbind retry %q %v", id, err)
	}
	for _, call := range caller.tokenCalls {
		request := call.args["UnbindLocalAgentRequest"].(map[string]any)
		if len(request) != 2 || request["runtimeBindingId"] != "binding-old" {
			t.Fatalf("unsafe unbind: %v", request)
		}
	}
	b.BindingState = "unbound"
	if err := checkEmployeeServerOperation(b); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeDeviceIdentityStableAndPrivate(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	first, err := employeeDeviceID(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := employeeDeviceID(context.Background(), "")
	if err != nil || first == "" || first != second {
		t.Fatal("unstable device ID")
	}
	if id, err := employeeDeviceID(context.Background(), "explicit-device"); err != nil || id != "explicit-device" {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "digital-employee-device", "identity.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("device identity not private")
	}
	if err := writeEmployeeJSON(path, map[string]string{"deviceId": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := employeeDeviceID(context.Background(), ""); err == nil {
		t.Fatal("corrupt ID silently replaced")
	}
}

func TestCrossPlatformCoverageEmployeeServerDurableBeforeNetworkAndBeforeCommit(t *testing.T) {
	for _, phase := range []string{"pending", "confirmed"} {
		t.Run(phase, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/de_local_agent_bind": {`{"success":true,"data":"bound-id"}`}}}
			InitDepsForTest(t, caller)
			testseam.Swap(t, &employeeServerWrite, func(path string, value any) error {
				if op, ok := value.(employeeServerOperation); ok && op.Phase == phase {
					return fmt.Errorf("disk unavailable")
				}
				return writeEmployeeJSON(path, value)
			})
			if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "rebind", b.AgentUUID), b, "bind", b.DeviceID); err == nil {
				t.Fatal("missing disk error")
			}
			want := 0
			if phase == "confirmed" {
				want = 1
			}
			if len(caller.tokenCalls) != want {
				t.Fatalf("network count: %d", len(caller.tokenCalls))
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeServerBindMigrationAndDryRun(t *testing.T) {
	_, b := lifecycleFixture(t)
	b.RuntimeBindingID, b.DeviceID = "", ""
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/de_local_agent_bind": {`{"success":true,"data":"migrated-id"}`}}}
	InitDepsForTest(t, caller)
	cmd := newEmployeeServerBindCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("dry-run", true, "")
	cmd.Flags().Bool("yes", true, "test confirmation")
	_ = cmd.Flags().Set("agent-uuid", b.AgentUUID)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(caller.tokenCalls) != 0 {
		t.Fatal("dry-run called server")
	}
	cmd = newEmployeeServerBindCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("yes", true, "test confirmation")
	_ = cmd.Flags().Set("agent-uuid", b.AgentUUID)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	current, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current.RuntimeBindingID != "migrated-id" || current.DeviceID == "" || current.BindingRevision != b.BindingRevision {
		t.Fatalf("migration: %+v %v", current, err)
	}
	request := caller.tokenCalls[0].args["BindLocalAgentRequest"].(map[string]any)
	if len(request) != 2 {
		t.Fatalf("unexpected bind payload: %v", request)
	}
}

func TestCrossPlatformCoverageEmployeeServerBusyKeepsOldIDAndBlocksNewHost(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/de_local_agent_rebind": {`{"success":false}`, `{"success":true,"data":"new-id"}`}}}
	InitDepsForTest(t, caller)
	started := false
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) { started = true; return "created", nil })
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{Prepared: true, Released: true, RuntimeState: "stopped"}, nil
	})
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	_ = cmd.Flags().Set("channel", "dsh")
	_ = cmd.Flags().Set("device-id", "new-device")
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("busy must fail")
	}
	current, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current.RuntimeBindingID != b.RuntimeBindingID || current.BindingState != "rebinding" || current.DesiredState != "stopped" || started {
		t.Fatalf("unsafe busy state: %+v", current)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	current, err = loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current.RuntimeBindingID != "new-id" || current.DeviceID != "new-device" || !started {
		t.Fatalf("new binding: %+v", current)
	}
	raw, _ := os.ReadFile(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"))
	var cfg digitalEmployeeAdapterConfig
	if json.Unmarshal(raw, &cfg) != nil || cfg.Binding.RuntimeBindingID != "new-id" {
		t.Fatal("adapter used old server ID")
	}
}

func TestCrossPlatformCoverageEmployeeServerNewDeviceRebindUsesOldID(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
	caller.responses["deap-dev/de_local_agent_rebind"] = []string{`{"success":true,"data":"new-device-binding"}`}
	InitDepsForTest(t, caller)
	setupSuccessfulConnectSeams(t)
	var saved digitalEmployeeBinding
	testseam.Swap(t, &deapConnectSaveBinding, func(_ string, b digitalEmployeeBinding) error { saved = b; return nil })
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{}, fmt.Errorf("no host")
	})
	cmd := lifecycleCmd(t, "rebind", "agent-1")
	_ = cmd.Flags().Set("channel", "dsh")
	_ = cmd.Flags().Set("runtime-binding-id", "old-machine-binding")
	_ = cmd.Flags().Set("device-id", "new-machine")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if saved.RuntimeBindingID != "new-device-binding" || saved.DeviceID != "new-machine" {
		t.Fatalf("wrong new device: %+v", saved)
	}
	for _, call := range caller.tokenCalls {
		if call.toolName == "de_local_agent_bind" {
			t.Fatal("new device used bind instead of atomic rebind")
		}
	}
	request := caller.tokenCalls[len(caller.tokenCalls)-1].args["RebindLocalAgentRequest"].(map[string]any)
	if request["runtimeBindingId"] != "old-machine-binding" {
		t.Fatal("wrong expected binding")
	}
}

func TestCrossPlatformCoverageEmployeeConnectServerRejectionDoesNotStartAdapter(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
	caller.responses["deap-dev/de_local_agent_bind"] = []string{`{"success":false}`}
	InitDepsForTest(t, caller)
	setupSuccessfulConnectSeams(t)
	testseam.Swap(t, &deapConnectSaveBinding, func(string, digitalEmployeeBinding) error { t.Fatal("rejected bind persisted as bound"); return nil })
	testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
		t.Fatal("rejected bind started DSH")
		return "", nil
	})
	cmd := newConnectTestCommand(t, false)
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "server_binding_rejected") {
		t.Fatalf("rejection: %v", err)
	}
}

func TestCrossPlatformCoverageEmployeeUnbindOldIDCannotTargetSuccessor(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
	InitDepsForTest(t, caller)
	cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
	_ = cmd.Flags().Set("runtime-binding-id", "different-id")
	if err := mutateEmployeeBinding(cmd, "unbind"); err == nil {
		t.Fatal("foreign ID accepted")
	}
	if len(caller.tokenCalls) != 0 {
		t.Fatal("foreign ID reached server")
	}
	b.BindingState = "unbound"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	if err := mutateEmployeeBinding(lifecycleCmd(t, "unbind", b.AgentUUID), "unbind"); err != nil {
		t.Fatal(err)
	}
	if len(caller.tokenCalls) != 0 {
		t.Fatal("completed unbind targeted successor")
	}
}
