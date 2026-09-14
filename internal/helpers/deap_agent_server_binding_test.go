// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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
	testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{CorpID: "corp", UserID: "supervisor", AccessToken: "supervisor-test-token"}, nil
	})
}

type employeeServerErrorCaller struct {
	digitalEmployeeProtocolCaller
	errors []error
}

func (c *employeeServerErrorCaller) CallToolWithToken(ctx context.Context, token, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	if len(c.errors) > 0 {
		c.tokenCalls = append(c.tokenCalls, deapAgentCall{productID: productID, toolName: toolName, args: args})
		c.tokens = append(c.tokens, token)
		err := c.errors[0]
		c.errors = c.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	return c.digitalEmployeeProtocolCaller.CallToolWithToken(ctx, token, productID, toolName, args)
}

func TestCrossPlatformCoverageEmployeeServerBindingUsesUnifiedTokenSnapshot(t *testing.T) {
	_, b := lifecycleFixture(t)
	called := 0
	testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
		called++
		return &auth.TokenData{CorpID: "corp", UserID: "supervisor", AccessToken: "refreshed-supervisor-token"}, nil
	})
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/rebind_local_agent": {`{"success":true,"data":"binding-new"}`},
	}}
	InitDepsForTest(t, caller)
	if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "rebind", b.AgentUUID), b, "rebind", "device-new"); err != nil {
		t.Fatal(err)
	}
	if called != 1 || len(caller.tokens) != 1 || caller.tokens[0] != "refreshed-supervisor-token" {
		t.Fatalf("snapshot calls=%d tokens=%v", called, caller.tokens)
	}
}

func TestCrossPlatformCoverageEmployeeServerBindingRefreshesRejectedTokenOnce(t *testing.T) {
	_, b := lifecycleFixture(t)
	refreshed := false
	testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) {
		token := "supervisor-test-token"
		if refreshed {
			token = "fresh-supervisor-token"
		}
		return &auth.TokenData{CorpID: "corp", UserID: "supervisor", AccessToken: token}, nil
	})
	testseam.Swap(t, &deapConnectForceRefreshSupervisorToken, func(_ context.Context, _ string, rejected string) (string, error) {
		if rejected != "supervisor-test-token" {
			t.Fatalf("rejected token mismatch")
		}
		refreshed = true
		return "fresh-supervisor-token", nil
	})
	caller := &employeeServerErrorCaller{
		digitalEmployeeProtocolCaller: digitalEmployeeProtocolCaller{responses: map[string][]string{
			"deap-dev/rebind_local_agent": {`{"success":true,"data":"binding-new"}`},
		}},
		errors: []error{&CLIError{Code: CodeAuthTokenExpired, Message: "token rejected"}},
	}
	InitDepsForTest(t, caller)
	id, err := mutateEmployeeServerBinding(lifecycleCmd(t, "rebind", b.AgentUUID), b, "rebind", "device-new")
	if err != nil || id != "binding-new" {
		t.Fatalf("binding = %q, %v", id, err)
	}
	if !refreshed || len(caller.tokens) != 2 || strings.Join(caller.tokens, ",") != "supervisor-test-token,fresh-supervisor-token" {
		t.Fatalf("refreshed=%t tokens=%v", refreshed, caller.tokens)
	}
}

func TestCrossPlatformCoverageEmployeeServerBindingRecordsGatewayRejection(t *testing.T) {
	_, b := lifecycleFixture(t)
	rejection := apperrors.NewAPI(
		"requested tool is unavailable",
		apperrors.WithReason("mcp_tool_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{TraceID: "trace-tool-not-found", ServerErrorCode: "TOOL_NOT_FOUND"}),
	)
	caller := &employeeServerErrorCaller{
		digitalEmployeeProtocolCaller: digitalEmployeeProtocolCaller{responses: map[string][]string{
			"deap-dev/rebind_local_agent": {`{"success":true,"data":"binding-new"}`},
		}},
		errors: []error{rejection},
	}
	InitDepsForTest(t, caller)
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	if _, err := mutateEmployeeServerBinding(cmd, b, "rebind", "device-new"); !errors.Is(err, rejection) {
		t.Fatalf("gateway rejection = %v", err)
	}
	var receipt employeeServerOperation
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Phase != "rejected" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if id, err := mutateEmployeeServerBinding(cmd, b, "rebind", "device-new"); err != nil || id != "binding-new" {
		t.Fatalf("corrected retry = %q, %v", id, err)
	}
	if len(caller.tokens) != 2 {
		t.Fatalf("calls=%d", len(caller.tokens))
	}
}

func TestCrossPlatformCoverageEmployeeServerBindingRefreshFailureIsRejected(t *testing.T) {
	_, b := lifecycleFixture(t)
	rejection := &CLIError{Code: CodeAuthTokenExpired, Message: "token rejected"}
	testseam.Swap(t, &deapConnectForceRefreshSupervisorToken, func(context.Context, string, string) (string, error) {
		return "", errors.New("refresh unavailable")
	})
	caller := &employeeServerErrorCaller{errors: []error{rejection}}
	InitDepsForTest(t, caller)
	if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "rebind", b.AgentUUID), b, "rebind", "device-new"); !errors.Is(err, rejection) {
		t.Fatalf("auth rejection = %v", err)
	}
	var receipt employeeServerOperation
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Phase != "rejected" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if len(caller.tokens) != 1 {
		t.Fatalf("calls=%d", len(caller.tokens))
	}
}

func TestCrossPlatformCoverageEmployeeServerBindingDoesNotRefreshPermissionRejection(t *testing.T) {
	_, b := lifecycleFixture(t)
	rejection := apperrors.NewAuth("permission denied", apperrors.WithReason("http_403"))
	testseam.Swap(t, &deapConnectForceRefreshSupervisorToken, func(context.Context, string, string) (string, error) {
		t.Fatal("permission rejection must not refresh the access token")
		return "", nil
	})
	caller := &employeeServerErrorCaller{errors: []error{rejection}}
	InitDepsForTest(t, caller)
	if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "rebind", b.AgentUUID), b, "rebind", "device-new"); !errors.Is(err, rejection) {
		t.Fatalf("permission rejection = %v", err)
	}
	var receipt employeeServerOperation
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Phase != "rejected" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestCrossPlatformCoverageEmployeeServerReceiptReplayAndIdentity(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/rebind_local_agent": {`{"success":true,"data":{"runtimeBindingId":"binding-new"}}`}}}
	InitDepsForTest(t, caller)
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	_ = cmd.Flags().Set("local-agent-name", "办公室 Agent")
	_ = cmd.Flags().Set("extensions", `{"note":"private-extension-value"}`)
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
	request := args
	if len(request) != 5 || request["agentUuid"] != b.AgentUUID || request["runtimeBindingId"] != "binding-old" || request["deviceId"] != "device-new" || request["localAgentName"] != "办公室 Agent" || request["extensions"] != `{"note":"private-extension-value"}` {
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
		{"invalid_argument", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_INVALID_ARGUMENT","errorMsg":"Invalid Local Agent binding parameters"}`, true},
		{"binding_conflict", `{"success":false,"data":null,"errorCode":"LOCAL_AGENT_BINDING_CONFLICT","errorMsg":"Binding changed or has active or recovering tasks","message":null}`, true},
		{"binding_forbidden", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_FORBIDDEN","errorMsg":"Local Agent binding access denied"}`, true},
		{"missing_success", `{"data":"id"}`, false},
		{"missing_id", `{"success":true,"data":null}`, false},
		{"nested_id", `{"success":true,"data":{"unrelated":{"runtimeBindingId":"id"}}}`, false},
		{"old_id", `{"success":true,"data":"binding-old"}`, false},
		{"old_object_id", `{"success":true,"data":{"runtimeBindingId":"binding-old"}}`, false},
		{"invalid_json", `private-error`, false},
		{"trailing_json", `{"success":true,"data":"id"}{}`, false},
		{"transport_lost", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{}}
			if tc.response != "" {
				caller.responses["deap-dev/rebind_local_agent"] = []string{tc.response, `{"success":true,"data":"recovered"}`}
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

func TestCrossPlatformCoverageEmployeeServerBindingResponseContract(t *testing.T) {
	for _, action := range []string{"bind", "rebind"} {
		for _, tc := range []struct {
			name, response string
			wantID         string
		}{
			{"data_object", `{"success":true,"data":{"runtimeBindingId":"binding-new"}}`, "binding-new"},
			{"documented_object", `{"success":true,"data":{"runtimeBindingId":"binding-new","runtimeId":"runtime-other","agentUuid":"employee-test","deviceId":"device-new","workspaceAlias":"default","localAgentName":"办公设备","extensions":"{}","status":"ACTIVE","updatedAt":"2026-09-14T17:00:00+08:00"},"errorCode":null,"errorMsg":null,"message":null}`, "binding-new"},
			{"matching_object_ids", `{"success":true,"runtimeBindingId":"binding-new","data":{"runtimeBindingId":"binding-new"}}`, "binding-new"},
			{"conflicting_object_ids", `{"success":true,"runtimeBindingId":"binding-other","data":{"runtimeBindingId":"binding-new"}}`, ""},
			{"empty_object", `{"success":true,"data":{}}`, ""},
			{"object_runtime_id_only", `{"success":true,"data":{"runtimeId":"runtime-other","status":"ACTIVE"}}`, ""},
			{"null_object_id", `{"success":true,"data":{"runtimeBindingId":null}}`, ""},
			{"empty_object_id", `{"success":true,"data":{"runtimeBindingId":""}}`, ""},
			{"invalid_object_id", `{"success":true,"data":{"runtimeBindingId":"\n"}}`, ""},
			{"non_string_object_id", `{"success":true,"data":{"runtimeBindingId":123}}`, ""},
			{"invalid_object_blocks_fallback", `{"success":true,"runtimeBindingId":"binding-new","data":{}}`, ""},
			{"empty_legacy_id_blocks_fallback", `{"success":true,"runtimeBindingId":"binding-new","data":""}`, ""},
			{"missing_data", `{"success":true}`, ""},
			{"null_data", `{"success":true,"data":null}`, ""},
			{"boolean_data", `{"success":true,"data":true}`, ""},
			{"array_data", `{"success":true,"data":[{"runtimeBindingId":"binding-new"}]}`, ""},
			{"active", `{"success":true,"status":"ACTIVE","runtimeBindingId":"binding-new","runtimeId":"runtime-other"}`, "binding-new"},
			{"legacy_null_data", `{"success":true,"runtimeBindingId":"binding-new","data":null}`, "binding-new"},
			{"legacy", `{"success":true,"data":"binding-new"}`, "binding-new"},
			{"matching_ids", `{"success":true,"runtimeBindingId":"binding-new","data":"binding-new"}`, "binding-new"},
			{"conflicting_ids", `{"success":true,"runtimeBindingId":"binding-new","data":"binding-other"}`, ""},
			{"runtime_id_only", `{"success":true,"status":"ACTIVE","runtimeId":"runtime-other"}`, ""},
			{"invalid_id", `{"success":true,"runtimeBindingId":"\n"}`, ""},
			{"non_string_id", `{"success":true,"runtimeBindingId":123}`, ""},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				_, b := lifecycleFixture(t)
				caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
					"deap-dev/" + action + "_local_agent": {tc.response},
				}}
				InitDepsForTest(t, caller)
				id, err := mutateEmployeeServerBinding(lifecycleCmd(t, action, b.AgentUUID), b, action, "device-new")
				if tc.wantID == "" {
					if err == nil || !strings.Contains(err.Error(), "server_binding_unknown") {
						t.Fatalf("ambiguous response accepted: id=%q err=%v", id, err)
					}
				} else if err != nil || id != tc.wantID {
					t.Fatalf("binding = %q, %v; want %q", id, err, tc.wantID)
				}
				var receipt employeeServerOperation
				raw, readErr := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
				if readErr != nil || json.Unmarshal(raw, &receipt) != nil {
					t.Fatalf("receipt unavailable: %v", readErr)
				}
				wantPhase := "confirmed"
				if tc.wantID == "" {
					wantPhase = "pending"
				}
				if receipt.Phase != wantPhase || receipt.RuntimeBindingID != tc.wantID || len(caller.tokenCalls) != 1 {
					t.Fatalf("receipt=%+v calls=%d", receipt, len(caller.tokenCalls))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageEmployeeServerUnbindResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name, response, phase string
	}{
		{"released", `{"success":true,"data":true,"message":"本次解绑成功。"}`, "confirmed"},
		{"already_released", `{"success":true,"data":true,"message":"该绑定此前已解除，本次未执行新的解绑操作。"}`, "confirmed"},
		{"no_message", `{"success":true,"data":true,"message":null}`, "confirmed"},
		{"forbidden", `{"success":false,"errorCode":"LOCAL_AGENT_BINDING_FORBIDDEN","errorMsg":"Local Agent binding access denied"}`, "rejected"},
		{"rejection_overrides_data", `{"success":false,"data":true,"message":"本次解绑成功。"}`, "rejected"},
		{"object_data", `{"success":true,"data":{},"message":"本次解绑成功。"}`, "pending"},
		{"false_data", `{"success":true,"data":false}`, "pending"},
		{"string_data", `{"success":true,"data":"true"}`, "pending"},
		{"null_data", `{"success":true,"data":null}`, "pending"},
		{"missing_data", `{"success":true}`, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/unbind_local_agent": {tc.response},
			}}
			InitDepsForTest(t, caller)
			id, err := mutateEmployeeServerBinding(lifecycleCmd(t, "unbind", b.AgentUUID), b, "unbind", "")
			wantID := ""
			if tc.phase == "confirmed" {
				wantID = b.RuntimeBindingID
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("ambiguous or rejected unbind accepted")
			}
			var receipt employeeServerOperation
			raw, readErr := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
			if readErr != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Phase != tc.phase || receipt.RuntimeBindingID != wantID || id != wantID || len(caller.tokenCalls) != 1 {
				t.Fatalf("id=%q receipt=%+v readErr=%v calls=%d", id, receipt, readErr, len(caller.tokenCalls))
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
	caller.responses["deap-dev/unbind_local_agent"] = []string{`{"success":true,"data":true}`}
	if id, err := mutateEmployeeServerBinding(cmd, b, "unbind", ""); err != nil || id != b.RuntimeBindingID {
		t.Fatalf("unbind retry %q %v", id, err)
	}
	for _, call := range caller.tokenCalls {
		request := call.args
		if len(request) != 2 || request["agentUuid"] != b.AgentUUID || request["runtimeBindingId"] != "binding-old" {
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
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/bind_local_agent": {`{"success":true,"data":"bound-id"}`}}}
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
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/bind_local_agent": {`{"success":true,"data":{"runtimeBindingId":"migrated-id"}}`}}}
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
	request := caller.tokenCalls[0].args
	if len(request) != 2 || request["agentUuid"] != b.AgentUUID || request["deviceId"] == "" {
		t.Fatalf("unexpected bind payload: %v", request)
	}
}

func TestCrossPlatformCoverageEmployeeServerBusyKeepsOldIDAndBlocksNewHost(t *testing.T) {
	_, b := lifecycleFixture(t)
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"deap-dev/rebind_local_agent": {`{"success":false}`, `{"success":true,"data":"new-id"}`}}}
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
	caller.responses["deap-dev/rebind_local_agent"] = []string{`{"success":true,"data":{"runtimeBindingId":"new-device-binding","runtimeId":"runtime-other"}}`}
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
		if call.toolName == "bind_local_agent" {
			t.Fatal("new device used bind instead of atomic rebind")
		}
	}
	request := caller.tokenCalls[len(caller.tokenCalls)-1].args
	if len(request) != 3 || request["agentUuid"] != "agent-1" || request["runtimeBindingId"] != "old-machine-binding" || request["deviceId"] != "new-machine" {
		t.Fatal("wrong expected binding")
	}
}

func TestCrossPlatformCoverageEmployeeConnectServerRejectionDoesNotStartAdapter(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
	caller.responses["deap-dev/bind_local_agent"] = []string{`{"success":false}`}
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

func TestCrossPlatformCoverageEmployeeServerPendingBlocksWorkerAndLease(t *testing.T) {
	_, b := lifecycleFixture(t)
	if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Key: "pending-request", Action: "bind", Phase: "pending"}); err != nil {
		t.Fatal(err)
	}
	auth.SetRuntimeProfile(b.DWSProfile)
	lease := newDeapConnectCommand()
	lease.SetContext(context.Background())
	for name, value := range map[string]string{"agent-uuid": b.AgentUUID, "channel": "dsh", "binding-revision": "7", "runtime-instance-id": "00000000-0000-4000-8000-000000000001"} {
		_ = lease.Flags().Set(name, value)
	}
	if err := runEmployeeLease(lease); err == nil || !strings.Contains(err.Error(), "server_binding_unknown") {
		t.Fatalf("lease accepted pending: %v", err)
	}
	b.Channel = "custom"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	cfg := digitalEmployeeAdapterConfig{Binding: b, SelfOpenDingTalkID: "employee-open-id"}
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDigitalEmployeeConfig(b.DWSProfile); err == nil {
		t.Fatal("worker accepted pending server operation")
	}
	if state := employeeLifecycleStatus(context.Background(), b); state["serverBindingState"] != "unknown" {
		t.Fatalf("state: %+v", state)
	}
}

func TestCrossPlatformCoverageEmployeeDeviceConcurrentCreation(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	var wg sync.WaitGroup
	ids := make([]string, 8)
	errs := make([]error, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) { defer wg.Done(); ids[i], errs[i] = employeeDeviceID(context.Background(), "") }(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] == "" || ids[i] != ids[0] {
			t.Fatalf("device generation race: %v", errs)
		}
	}
	b := digitalEmployeeBinding{DeviceID: "explicit-saved-device"}
	if id, err := employeeBindingDeviceID(context.Background(), b, ""); err != nil || id != b.DeviceID {
		t.Fatal("explicit device ID not reused")
	}
}

func TestCrossPlatformCoverageEmployeeBindingDryRunDoesNotRepairReceipts(t *testing.T) {
	_, b := lifecycleFixture(t)
	b.BindingState = "unbound"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	op := employeeServerOperation{Key: "completed", Action: "unbind", Phase: "confirmed", RuntimeBindingID: b.RuntimeBindingID}
	path := employeeServerOperationPath(b.DWSProfile)
	if err := writeEmployeeJSON(path, op); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, action := range []string{"unbind", "rebind"} {
		cmd := lifecycleCmd(t, action, b.AgentUUID)
		_ = cmd.Flags().Set("dry-run", "true")
		if action == "rebind" {
			_ = cmd.Flags().Set("channel", "dsh")
			_ = cmd.Flags().Set("device-id", "new-device")
		}
		if err := mutateEmployeeBinding(cmd, action); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("dry-run repaired receipt")
	}
	if _, err := os.Stat(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run persisted operation")
	}
}

func TestCrossPlatformCoverageEmployeeConnectReplaysReceiptAfterLocalCommitFailure(t *testing.T) {
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-open"}]}`)
	caller.responses["deap-dev/bind_local_agent"] = []string{`{"success":true,"data":{"runtimeBindingId":"binding-created"}}`}
	for key, values := range caller.responses {
		if key != "deap-dev/bind_local_agent" {
			caller.responses[key] = append(append([]string{}, values...), values...)
		}
	}
	InitDepsForTest(t, caller)
	setupSuccessfulConnectSeams(t)
	attempts := 0
	testseam.Swap(t, &deapConnectSaveBinding, func(dir string, b digitalEmployeeBinding) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("injected local commit failure")
		}
		return saveDigitalEmployeeBinding(dir, b)
	})
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{}, fmt.Errorf("host unavailable")
	})
	first := newConnectTestCommand(t, false)
	if err := first.RunE(first, nil); err == nil {
		t.Fatal("missing local failure")
	}
	cmd := newConnectTestCommand(t, false)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, call := range caller.tokenCalls {
		if call.toolName == "bind_local_agent" {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("recovery called bind %d times", calls)
	}
	b, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), "employee-corp:employee-user")
	if err != nil || b.RuntimeBindingID != "binding-created" {
		t.Fatalf("missing committed receipt: %+v %v", b, err)
	}
	var receipt employeeServerOperation
	raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Phase != "consumed" || receipt.RuntimeBindingID != b.RuntimeBindingID {
		t.Fatalf("receipt was not consumed: %+v %v", receipt, err)
	}
}
