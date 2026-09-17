// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeServerReceiptDurability(t *testing.T) {
	for _, scenario := range []string{"caller", "confirmed-id", "rejection-write", "refresh-identity"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			cmd := lifecycleCmd(t, "bind", b.AgentUUID)
			rejection := &CLIError{Code: CodeAuthTokenExpired, Message: "expired"}
			caller := &employeeServerErrorCaller{errors: []error{rejection}}
			InitDepsForTest(t, caller)
			switch scenario {
			case "caller":
				testseam.Swap(t, &deps, nil)
			case "confirmed-id":
				// A lost response leaves an exact request key; a damaged confirmed receipt
				// may not fabricate a binding ID or send the non-idempotent request again.
				caller.errors = []error{errors.New("lost response")}
				_, _ = mutateEmployeeServerBinding(cmd, b, "bind", "device")
				raw, err := os.ReadFile(employeeServerOperationPath(b.DWSProfile))
				if err != nil {
					t.Fatal(err)
				}
				var receipt employeeServerOperation
				if err = json.Unmarshal(raw, &receipt); err != nil {
					t.Fatal(err)
				}
				receipt.Phase = "confirmed"
				receipt.RuntimeBindingID = ""
				if err = writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), receipt); err != nil {
					t.Fatal(err)
				}
			case "rejection-write":
				testseam.Swap(t, &deapConnectForceRefreshSupervisorToken, func(context.Context, string, string) (string, error) { return "", errors.New("refresh failed") })
				testseam.Swap(t, &employeeServerWrite, func(path string, value any) error {
					if op, ok := value.(employeeServerOperation); ok && op.Phase == "rejected" {
						return errors.New("receipt not durable")
					}
					return writeEmployeeJSON(path, value)
				})
			case "refresh-identity":
				testseam.Swap(t, &deapConnectForceRefreshSupervisorToken, func(context.Context, string, string) (string, error) { return "fresh-other-profile", nil })
			}
			if _, err := mutateEmployeeServerBinding(cmd, b, "bind", "device"); err == nil {
				t.Fatal("invalid receipt accepted")
			}
			if scenario == "confirmed-id" || scenario == "refresh-identity" {
				if len(caller.tokens) != 1 {
					t.Fatalf("unsafe replay: %d", len(caller.tokens))
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeBindingViewAndUnbindResume(t *testing.T) {
	for _, scenario := range []string{"binding-valid", "binding-stale", "binding-reload", "unbound-consume", "resume-operation", "stop-consume"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			if strings.HasPrefix(scenario, "binding-") {
				auth.SetRuntimeProfile(b.DWSProfile)
				cmd := newEmployeeBindingCommand()
				cmd.SetContext(context.Background())
				cmd.SetOut(io.Discard)
				_ = cmd.Flags().Set("channel", "dsh")
				_ = cmd.Flags().Set("stdin", "true")
				raw := `{"agentUuid":"employee-test","bindingRevision":7}`
				if scenario == "binding-stale" {
					raw = `{"agentUuid":"employee-test","bindingRevision":6}`
				}
				calls := 0
				testseam.Swap(t, &deapChannelLoadBinding, func(string, string) (digitalEmployeeBinding, error) {
					calls++
					if scenario == "binding-reload" && calls > 2 {
						return digitalEmployeeBinding{}, errors.New("changed")
					}
					return b, nil
				})
				cmd.SetIn(strings.NewReader(raw))
				err := cmd.RunE(cmd, nil)
				if (err == nil) != (scenario == "binding-valid") {
					t.Fatalf("binding view: %v", err)
				}
				return
			}
			if scenario == "unbound-consume" {
				b.BindingState = "unbound"
				if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), b); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "resume-operation" {
				if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation.json"), employeeBindingOperation{ID: "prior-operation", Action: "unbind", FromRevision: b.BindingRevision}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Key: "key", Phase: "confirmed", RuntimeBindingID: b.RuntimeBindingID}); err != nil {
					t.Fatal(err)
				}
				testseam.Swap(t, &employeeServerWrite, func(string, any) error { return errors.New("receipt unavailable") })
			}
			testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
				return employeeDSHState{Released: true, RuntimeState: "stopped"}, nil
			})
			cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
			var err error
			if scenario == "stop-consume" {
				err = runDigitalEmployeeLifecycle(cmd, "restart")
			} else {
				err = runEmployeeUnbind(cmd)
			}
			if scenario == "resume-operation" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("receipt persistence failure accepted")
			}
		})
	}
}
