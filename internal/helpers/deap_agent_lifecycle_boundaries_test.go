// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeStopFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"dsh-error", "dsh-unreleased", "missing", "corrupt", "identity", "dead", "unknown-instance", "stop-write", "cancelled-wait"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			b := cfg.Binding
			dir := digitalEmployeeRuntimeDir(b.DWSProfile)
			state := digitalEmployeeRunState{PID: os.Getpid(), Profile: b.DWSProfile, AgentUUID: b.AgentUUID, RunID: "test-run", Status: "running"}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			switch scenario {
			case "dsh-error", "dsh-unreleased":
				b.Channel = "dsh"
				testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
					if scenario == "dsh-error" {
						return employeeDSHState{}, errors.New("host unavailable")
					}
					return employeeDSHState{RuntimeState: "running"}, nil
				})
			case "identity":
				state.AgentUUID = "different"
			case "dead":
				state.PID = 0
			case "unknown-instance":
				state.RunID = ""
			case "stop-write":
				testseam.Swap(t, &atomicRename, func(src, dst string) error {
					if filepath.Base(dst) == "stop.json" {
						return errors.New("stop unavailable")
					}
					return os.Rename(src, dst)
				})
			}
			if scenario != "missing" {
				if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), state); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "corrupt" {
				if err := AtomicWriteJSON(filepath.Join(dir, "state.json"), []byte("{")); err != nil {
					t.Fatal(err)
				}
			}
			err := stopEmployeeRuntime(ctx, b)
			wantError := scenario != "missing" && scenario != "dead"
			if (err != nil) != wantError {
				t.Fatalf("stop=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeLifecycleStatusPreservesFailureEvidence(t *testing.T) {
	for _, scenario := range []string{"unbound", "receipt-directory", "receipt-confirmed", "runtime-corrupt", "runtime-upgrade", "operation-failed"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			b := cfg.Binding
			b.RuntimeBindingID = "binding"
			dir := digitalEmployeeRuntimeDir(b.DWSProfile)
			switch scenario {
			case "unbound":
				b.BindingState = "unbound"
			case "receipt-directory":
				if err := os.MkdirAll(employeeServerOperationPath(b.DWSProfile), 0700); err != nil {
					t.Fatal(err)
				}
			case "receipt-confirmed":
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "confirmed"}); err != nil {
					t.Fatal(err)
				}
			case "runtime-corrupt":
				if err := AtomicWriteJSON(filepath.Join(dir, "state.json"), []byte("{")); err != nil {
					t.Fatal(err)
				}
			case "runtime-upgrade":
				if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), digitalEmployeeRunState{Status: "blocked", AgentUUID: b.AgentUUID, Profile: b.DWSProfile, Channel: b.Channel, Code: "event_bus_upgrade_required"}); err != nil {
					t.Fatal(err)
				}
			case "operation-failed":
				if err := writeEmployeeJSON(filepath.Join(dir, "operation.json"), employeeBindingOperation{ID: "operation", ReasonCode: "incomplete"}); err != nil {
					t.Fatal(err)
				}
			}
			got := employeeLifecycleStatus(context.Background(), b)
			key, want := "serverBindingState", "unbound"
			switch scenario {
			case "receipt-directory":
				want = "unknown"
			case "receipt-confirmed":
				want = "commit_pending"
			case "runtime-corrupt":
				key, want = "reasonCode", "runtime_state_invalid"
			case "runtime-upgrade":
				key, want = "reasonCode", "event_bus_upgrade_required"
			case "operation-failed":
				key, want = "reasonCode", "incomplete"
			}
			if got[key] != want {
				t.Fatalf("status=%v want %s=%s", got, key, want)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeUnbindPersistenceBoundaries(t *testing.T) {
	for _, scenario := range []string{"missing-binding-id", "wrong-explicit-id", "operation-write", "binding-write", "stop-error", "release-error", "release-unconfirmed", "backup-write", "commit-write", "consume-write", "unbound-pending"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
			testseam.Swap(t, &employeeDSHControl, func(_ context.Context, _ digitalEmployeeBinding, action string) (employeeDSHState, error) {
				if (scenario == "stop-error" && action == "stop") || (scenario == "release-error" && action == "release") {
					return employeeDSHState{}, errors.New("host unavailable")
				}
				return employeeDSHState{Released: scenario != "release-unconfirmed" || action != "release", RuntimeState: "stopped"}, nil
			})
			switch scenario {
			case "missing-binding-id":
				b.RuntimeBindingID = ""
			case "wrong-explicit-id":
				_ = cmd.Flags().Set("runtime-binding-id", "another-binding")
			case "unbound-pending":
				b.BindingState = "unbound"
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "pending"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := updateEmployeeBinding(b); err != nil {
				t.Fatal(err)
			}
			bindingWrites, receiptWrites := 0, 0
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				name := filepath.Base(dst)
				if dst == digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile) {
					bindingWrites++
				}
				if dst == employeeServerOperationPath(b.DWSProfile) {
					receiptWrites++
				}
				fail := (scenario == "operation-write" && name == "operation.json") || (scenario == "binding-write" && bindingWrites == 1 && dst == digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile)) || (scenario == "backup-write" && name == "binding-7.json") || (scenario == "commit-write" && bindingWrites == 2 && dst == digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile)) || (scenario == "consume-write" && receiptWrites == 3 && dst == employeeServerOperationPath(b.DWSProfile))
				if fail {
					return errors.New("injected persistence failure")
				}
				return os.Rename(src, dst)
			})
			if err := runEmployeeUnbind(cmd); err == nil {
				t.Fatal("incomplete unbind reported success")
			}
			persisted, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "consume-write" && scenario != "unbound-pending" && persisted.BindingState == "unbound" {
				t.Fatal("unconfirmed unbind discarded old binding")
			}
		})
	}
}
