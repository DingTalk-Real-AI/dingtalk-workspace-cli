// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageEmployeeDeviceIdentityFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"explicit", "invalid", "lock", "read", "write"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
			deviceDir := filepath.Join(dir, "digital-employee-device")
			switch scenario {
			case "explicit":
				id, err := employeeDeviceID(context.Background(), "explicit-device")
				if err != nil || id != "explicit-device" {
					t.Fatalf("id=%q err=%v", id, err)
				}
				if _, err := os.Stat(deviceDir); !os.IsNotExist(err) {
					t.Fatal("explicit identity persisted unexpectedly")
				}
				return
			case "invalid":
				if _, err := employeeDeviceID(context.Background(), "bad\nidentity"); err == nil {
					t.Fatal("accepted invalid device")
				}
				return
			case "lock":
				if err := os.WriteFile(deviceDir, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			case "read":
				if err := os.MkdirAll(filepath.Join(deviceDir, "identity.json"), 0700); err != nil {
					t.Fatal(err)
				}
			case "write":
				testseam.Swap(t, &employeeServerWrite, func(string, any) error { return errors.New("write unavailable") })
			}
			if _, err := employeeDeviceID(context.Background(), ""); err == nil {
				t.Fatal("failure produced a device identity")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeBindingPreflightRejectsInvalidRequests(t *testing.T) {
	for _, scenario := range []string{"profile-load", "employee-profile", "ordinary-caller", "action", "missing-binding", "missing-device", "receipt-read", "receipt-corrupt", "receipt-phase", "write"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeProtocolCaller{}
			InitDepsForTest(t, caller)
			action, device := "bind", "device"
			path := employeeServerOperationPath(b.DWSProfile)
			switch scenario {
			case "profile-load":
				testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) { return nil, errors.New("unavailable") })
			case "employee-profile":
				b.DWSProfile = "corp:supervisor"
			case "ordinary-caller":
				InitDepsForTest(t, &deapAgentCaller{})
			case "action":
				action = "replace"
			case "missing-binding":
				action, b.RuntimeBindingID = "unbind", ""
			case "missing-device":
				device = ""
			case "receipt-read":
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "receipt-corrupt", "receipt-phase":
				raw := []byte("{")
				if scenario == "receipt-phase" {
					raw = []byte(`{"key":"key","phase":"invalid"}`)
				}
				if err := AtomicWriteJSON(path, raw); err != nil {
					t.Fatal(err)
				}
			case "write":
				testseam.Swap(t, &employeeServerWrite, func(string, any) error { return errors.New("disk unavailable") })
			}
			if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "bind", b.AgentUUID), b, action, device); err == nil {
				t.Fatal("invalid binding request accepted")
			}
			if len(caller.tokenCalls) != 0 {
				t.Fatal("invalid request reached server")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeBindingResultAndReceiptFailures(t *testing.T) {
	for _, scenario := range []string{"non-text", "trailing-json", "write-rejection", "write-gateway-rejection", "write-confirmation"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			caller := &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":"bound"}`}}}
			switch scenario {
			case "non-text":
				caller.blocks = []edition.ContentBlock{{Type: "image"}, {Type: "text", Text: " "}}
			case "trailing-json":
				caller.blocks[0].Text += `{}`
			case "write-rejection":
				caller.blocks[0].Text = `{"success":false}`
			}
			InitDepsForTest(t, caller)
			if scenario == "write-gateway-rejection" {
				InitDepsForTest(t, &employeeServerErrorCaller{errors: []error{&CLIError{Code: CodeAuthPermission, Message: "denied"}}})
			}
			if strings.HasPrefix(scenario, "write-") {
				writes := 0
				testseam.Swap(t, &employeeServerWrite, func(path string, value any) error {
					writes++
					if writes == 2 {
						return errors.New("receipt unavailable")
					}
					return writeEmployeeJSON(path, value)
				})
			}
			if _, err := mutateEmployeeServerBinding(lifecycleCmd(t, "bind", b.AgentUUID), b, "bind", "device"); err == nil {
				t.Fatal("ambiguous binding result accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeServerReceiptRecoveryBoundaries(t *testing.T) {
	for _, scenario := range []string{"missing", "directory", "corrupt", "rejected", "consumed", "pending", "confirmed-unbind"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			path := employeeServerOperationPath(b.DWSProfile)
			switch scenario {
			case "directory":
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := AtomicWriteJSON(path, []byte("{")); err != nil {
					t.Fatal(err)
				}
			case "missing":
			default:
				op := employeeServerOperation{Phase: scenario, RuntimeBindingID: b.RuntimeBindingID}
				if scenario == "confirmed-unbind" {
					op.Phase, op.Action, b.BindingState = "confirmed", "unbind", "unbound"
				}
				if err := writeEmployeeJSON(path, op); err != nil {
					t.Fatal(err)
				}
			}
			wantError := scenario == "directory" || scenario == "corrupt" || scenario == "pending"
			if err := checkEmployeeServerOperation(b); (err != nil) != wantError {
				t.Fatalf("check=%v", err)
			}
			if err := consumeEmployeeServerOperation(b.DWSProfile); (err != nil) != wantError {
				t.Fatalf("consume=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeServerErrorClassification(t *testing.T) {
	for _, code := range []string{CodeAuthPermission, CodeMCPToolError, CodeResourceNotFound, CodeTableNotFound, CodeSheetNotFound, CodeFieldNotFound, CodeRecordNotFound} {
		if !employeeServerDefinitiveRejection(&CLIError{Code: code}) {
			t.Fatalf("not definitive: %s", code)
		}
	}
	for _, reason := range []string{"business_error", "mcp_tool_error", "invalid_request"} {
		if !employeeServerDefinitiveRejection(apperrors.NewAPI("denied", apperrors.WithReason(reason))) {
			t.Fatalf("not definitive: %s", reason)
		}
	}
	for _, reason := range []string{"access_token_rejected", "gateway_auth_expired", "http_401"} {
		err := apperrors.NewAuth("denied", apperrors.WithReason(reason))
		if !employeeServerAccessTokenRejected(err) || !employeeServerDefinitiveRejection(err) {
			t.Fatalf("not auth rejection: %s", reason)
		}
	}
	if employeeServerDefinitiveRejection(errors.New("transport uncertain")) || employeeServerDefinitiveRejection(apperrors.NewAPI("unknown")) {
		t.Fatal("uncertain error treated as definitive")
	}
}

func TestCrossPlatformCoverageEmployeeIdentityLookupFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"empty-token", "ordinary-caller", "transport", "non-text", "invalid-json", "rejected", "keyed-result"} {
		t.Run(scenario, func(t *testing.T) {
			caller := &digitalEmployeeIdentityBlocksCaller{blocks: []edition.ContentBlock{{Type: "text", Text: `{"target":{"openDingTalkId":"exact"}}`}}}
			InitDepsForTest(t, caller)
			token := "fixture-token"
			switch scenario {
			case "empty-token":
				token = ""
			case "ordinary-caller":
				InitDepsForTest(t, &deapAgentCaller{})
			case "transport":
				InitDepsForTest(t, &digitalEmployeeProtocolCaller{})
			case "non-text":
				caller.blocks = []edition.ContentBlock{{Type: "image"}, {Type: "text", Text: " "}}
			case "invalid-json":
				caller.blocks[0].Text = "{"
			case "rejected":
				caller.blocks[0].Text = `{"success":false}`
			}
			id, err := resolveExactOperatorOpenDingTalkID(context.Background(), token, "target")
			if scenario == "keyed-result" {
				if err != nil || id != "exact" {
					t.Fatalf("id=%s err=%v", id, err)
				}
			} else if err == nil {
				t.Fatal("invalid lookup accepted")
			}
			if _, err := resolveDigitalEmployeeManagedIdentity(context.Background(), token, "corp"); err == nil {
				t.Fatal("unverified managed identity accepted")
			}
		})
	}
}
