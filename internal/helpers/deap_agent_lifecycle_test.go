// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package helpers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmployeeLeaseHoldsTheSameProfileLockUntilStdinCloses(t *testing.T) {
	_, b := lifecycleFixture(t)
	auth.SetRuntimeProfile(b.DWSProfile)
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("agent-uuid", b.AgentUUID)
	_ = cmd.Flags().Set("channel", "dsh")
	_ = cmd.Flags().Set("binding-revision", "7")
	_ = cmd.Flags().Set("runtime-instance-id", "00000000-0000-4000-8000-000000000001")
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	cmd.SetIn(input)
	readyOut, readyIn := io.Pipe()
	defer readyOut.Close()
	defer readyIn.Close()
	cmd.SetErr(readyIn)
	done := make(chan error, 1)
	go func() { done <- runEmployeeLease(cmd) }()
	buf := make([]byte, len("[employee] leased\n"))
	if _, err := io.ReadFull(readyOut, buf); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if lock, err := auth.AcquireDualLock(ctx, filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "worker")); err == nil {
		lock.Release()
		t.Fatal("DSH lease did not exclude a worker")
	}
	_, _ = io.WriteString(writer, "released\n")
	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := confirmEmployeeRuntimeReleased(context.Background(), b, ""); err != nil {
		t.Fatal(err)
	}
}

func lifecycleFixture(t *testing.T) (string, digitalEmployeeBinding) {
	t.Helper()
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "employee-test", DWSProfile: "corp:employee", Channel: "dsh", OperatorOpenDingTalkID: "operator", BindingRevision: 7, BindingState: "bound", DesiredState: "running"}
	if err := saveDigitalEmployeeBinding(dir, b); err != nil {
		t.Fatal(err)
	}
	return dir, b
}

func TestEmployeeLostLeaseIsQuarantinedUntilExactHostRelease(t *testing.T) {
	_, b := lifecycleFixture(t)
	guard := employeeLeaseGuard{AgentUUID: b.AgentUUID, Revision: b.BindingRevision, InstanceID: "old-host-instance"}
	if err := writeEmployeeJSON(employeeLeaseGuardPath(b.DWSProfile), guard); err != nil {
		t.Fatal(err)
	}
	if err := checkEmployeeLeaseGuard(b.DWSProfile); err == nil {
		t.Fatal("crashed lease must not allow another worker")
	}
	for _, instance := range []string{"", "another-host"} {
		if err := confirmEmployeeRuntimeReleased(context.Background(), b, instance); err == nil {
			t.Fatal("foreign or empty runtime confirmation cleared quarantine")
		}
	}
	if err := confirmEmployeeRuntimeReleased(context.Background(), b, guard.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := checkEmployeeLeaseGuard(b.DWSProfile); err != nil {
		t.Fatal(err)
	}
}

func TestEmployeeRebindToDSHAndRestartRepairsFailedRegistration(t *testing.T) {
	_, b := lifecycleFixture(t)
	b.Channel = "custom"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	registered := false
	failRegistration := true
	testseam.Swap(t, &deapConnectRegisterDSH, func(_ context.Context, payload map[string]any) (string, error) {
		if payload["bindingRevision"] != uint64(8) {
			t.Fatal("registration must use new revision")
		}
		if failRegistration {
			return "", fmt.Errorf("injected registration failure")
		}
		registered = true
		return "created", nil
	})
	testseam.Swap(t, &employeeDSHControl, func(_ context.Context, got digitalEmployeeBinding, action string) (employeeDSHState, error) {
		if action == "prepare" {
			return employeeDSHState{Prepared: true}, nil
		}
		if action == "start" && !registered {
			t.Fatal("restart must register before start")
		}
		if action == "stop" {
			return employeeDSHState{Released: true, RuntimeState: "stopped"}, nil
		}
		return employeeDSHState{RuntimeState: "running", TransportReady: true, ExecutorReady: true}, nil
	})
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	_ = cmd.Flags().Set("channel", "dsh")
	if err := mutateEmployeeBinding(cmd, "rebind"); err == nil {
		t.Fatal("expected registration failure")
	}
	current, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
	if err != nil || current.Channel != "dsh" || current.BindingRevision != 8 || employeeBindingState(current) != "bound" {
		t.Fatalf("committed binding lost: %+v %v", current, err)
	}
	failRegistration = false
	restart := newDigitalEmployeeRestartCommand()
	restart.SetContext(context.Background())
	restart.SetOut(io.Discard)
	_ = restart.Flags().Set("agent-uuid", b.AgentUUID)
	if err := runDigitalEmployeeLifecycle(restart, "restart"); err != nil {
		t.Fatal(err)
	}
	if !registered {
		t.Fatal("registration not recovered")
	}
}
func lifecycleCmd(t *testing.T, action, id string) *cobra.Command {
	t.Helper()
	cmd := newEmployeeUnbindCommand()
	if action == "rebind" {
		cmd = newEmployeeRebindCommand()
	}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.Flags().Bool("dry-run", false, "")
	if err := cmd.Flags().Set("agent-uuid", id); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func TestEmployeeUnbindRequiresReleaseAndKeepsProfile(t *testing.T) {
	dir, b := lifecycleFixture(t)
	tokenFile := filepath.Join(dir, "profile-kept.json")
	if err := os.WriteFile(tokenFile, []byte("test-private-profile"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	testseam.Swap(t, &employeeDSHControl, func(_ context.Context, got digitalEmployeeBinding, action string) (employeeDSHState, error) {
		calls = append(calls, action)
		onDisk, err := loadDigitalEmployeeBinding(dir, b.DWSProfile)
		if err != nil || onDisk.DesiredState != "stopped" {
			t.Fatal("must persist stop intent before IPC")
		}
		if got.BindingRevision != 7 {
			t.Fatal("old runtime must be released using old revision")
		}
		return employeeDSHState{Released: true, RuntimeState: "stopped"}, nil
	})
	if err := mutateEmployeeBinding(lifecycleCmd(t, "unbind", b.AgentUUID), "unbind"); err != nil {
		t.Fatal(err)
	}
	current, err := loadDigitalEmployeeBinding(dir, b.DWSProfile)
	if err != nil {
		t.Fatal(err)
	}
	if current.BindingState != "unbound" || current.BindingRevision != 8 || strings.Join(calls, ",") != "stop,release" {
		t.Fatalf("binding=%+v calls=%v", current, calls)
	}
	if data, err := os.ReadFile(tokenFile); err != nil || string(data) != "test-private-profile" {
		t.Fatal("profile changed")
	}
	calls = nil
	if err := mutateEmployeeBinding(lifecycleCmd(t, "unbind", b.AgentUUID), "unbind"); err != nil || len(calls) != 0 {
		t.Fatal("unbind must be idempotent", err)
	}
}

func TestEmployeeUnbindUnknownRuntimePreservesOldBinding(t *testing.T) {
	dir, b := lifecycleFixture(t)
	testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
		return employeeDSHState{}, fmt.Errorf("host lost")
	})
	if err := mutateEmployeeBinding(lifecycleCmd(t, "unbind", b.AgentUUID), "unbind"); err == nil {
		t.Fatal("unknown release accepted")
	}
	current, _ := loadDigitalEmployeeBinding(dir, b.DWSProfile)
	if current.BindingRevision != b.BindingRevision || current.Channel != "dsh" || current.BindingState != "unbinding" {
		t.Fatalf("unsafe overwrite: %+v", current)
	}
	status := employeeLifecycleStatus(context.Background(), current)
	if status["runtimeState"] != "unknown" || status["operationId"] == nil {
		t.Fatal(status)
	}
}

func TestEmployeeRebindPreflightFailsWithoutStoppingOld(t *testing.T) {
	dir, b := lifecycleFixture(t)
	b.Channel = "custom"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &employeeDSHControl, func(_ context.Context, _ digitalEmployeeBinding, action string) (employeeDSHState, error) {
		if action != "prepare" {
			t.Fatal("stopped before preflight")
		}
		return employeeDSHState{}, fmt.Errorf("no host")
	})
	cmd := lifecycleCmd(t, "rebind", b.AgentUUID)
	_ = cmd.Flags().Set("channel", "dsh")
	if err := mutateEmployeeBinding(cmd, "rebind"); err == nil {
		t.Fatal("preflight passed")
	}
	current, _ := loadDigitalEmployeeBinding(dir, b.DWSProfile)
	if current != b {
		t.Fatal("preflight changed binding")
	}
}

func TestEmployeeBindingRevisionFencesOldReplies(t *testing.T) {
	_, b := lifecycleFixture(t)
	auth.SetRuntimeProfile(b.DWSProfile)
	t.Cleanup(func() { auth.SetRuntimeProfile("") })
	cmd := newDeapChannelReplyCommand()
	_ = cmd.Flags().Set("channel", "dsh")
	if err := validateEmployeeMachineRevision(cmd, b.AgentUUID, b.BindingRevision); err != nil {
		t.Fatal(err)
	}
	if err := validateEmployeeMachineRevision(cmd, b.AgentUUID, b.BindingRevision-1); err == nil {
		t.Fatal("old revision accepted")
	}
	b.BindingState = "unbound"
	if err := updateEmployeeBinding(b); err != nil {
		t.Fatal(err)
	}
	if err := validateEmployeeMachineRevision(cmd, b.AgentUUID, b.BindingRevision); err == nil {
		t.Fatal("unbound reply accepted")
	}
}
