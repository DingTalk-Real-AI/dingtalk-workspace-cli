// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeSupervisorFinalFailures(t *testing.T) {
	for _, scenario := range []string{"saved-lock", "retry-state", "worker-receipt"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if scenario == "saved-lock" {
				previous := auth.RuntimeProfile()
				auth.SetRuntimeProfile(cfg.Binding.DWSProfile)
				t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
				if err := writeEmployeeJSON(filepath.Join(dir, "adapter.json"), cfg); err != nil {
					t.Fatal(err)
				}
				lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(dir, "activation"))
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Release()
				limited, stop := context.WithTimeout(ctx, 20*time.Millisecond)
				defer stop()
				cmd := newDeapConnectCommand()
				cmd.SetContext(limited)
				_ = cmd.Flags().Set("agent-uuid", cfg.Binding.AgentUUID)
				_ = cmd.Flags().Set("channel", cfg.Binding.Channel)
				if err := runDigitalEmployeeSaved(cmd); err == nil {
					t.Fatal("occupied activation ignored")
				}
				raw, err := os.ReadFile(filepath.Join(dir, "failure.json"))
				if err != nil || !strings.Contains(string(raw), "worker_failed") {
					t.Fatalf("failure classification %s %v", raw, err)
				}
				return
			}
			if scenario == "retry-state" {
				if err := writeEmployeeJSON(filepath.Join(dir, "retry.json"), employeeRetryState{NextAttempt: time.Now().Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
				writes := 0
				testseam.Swap(t, &atomicRename, func(src, dst string) error {
					if filepath.Base(dst) == "state.json" {
						writes++
						if writes == 2 {
							return errors.New("state unavailable")
						}
					}
					return os.Rename(src, dst)
				})
				if err := superviseDigitalEmployee(ctx, cfg); err == nil {
					t.Fatal("retry state persistence ignored")
				}
				return
			}
			t.Setenv("DWS_RECEIPT_BOUNDARY_DIR", dir)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeSupervisorReceiptFixture$", "--", "employee-receipt-boundary")
			})
			if err := superviseDigitalEmployee(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			state, err := readDigitalEmployeeState(dir)
			if err != nil || state.Status != "blocked" {
				t.Fatalf("terminal worker retried: %+v %v", state, err)
			}
		})
	}
}

func TestEmployeeSupervisorReceiptFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-receipt-boundary" {
		return
	}
	if err := writeEmployeeJSON(filepath.Join(os.Getenv("DWS_RECEIPT_BOUNDARY_DIR"), "failure.json"), employeeTerminal("fixture_terminal")); err != nil {
		os.Exit(2)
	}
	os.Exit(1)
}

func TestCrossPlatformCoverageEmployeeDefaultCredentialBoundaries(t *testing.T) {
	dir := t.TempDir()
	old := auth.RuntimeProfile()
	auth.SetRuntimeProfile("missing-corp:missing-user")
	t.Cleanup(func() { auth.SetRuntimeProfile(old) })
	t.Setenv("DWS_ACCESS_TOKEN", "")
	t.Setenv("DWS_TOKEN", "")
	t.Setenv("DWS_CONFIG_DIR", dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := deapConnectLoadSupervisorToken(ctx, dir); err == nil {
		t.Fatal("missing supervisor credential accepted")
	}
	if _, err := deapConnectForceRefreshSupervisorToken(ctx, dir, "rejected-fixture-token"); err == nil {
		t.Fatal("missing refresh credential accepted")
	}
	r, e := employeeLedgerFixture(t)
	e.MessageID = ""
	if r.accept(e) {
		t.Fatal("message without ID accepted")
	}
}

func TestCrossPlatformCoverageEmployeeLocalDaemonDispatch(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("daemon", "true")
	testseam.Swap(t, &daemonDetachEnabled, false)
	if err := (digitalEmployeeLocalAdapter{}).Connect(cmd, cfg); err == nil {
		t.Fatal("unsupported daemon accepted")
	}
}

func TestCrossPlatformCoverageEmployeeOperatorRejectsProfileSwitch(t *testing.T) {
	installEmployeeReplyBinding(t)
	old := deapChannelLoadBinding
	calls := 0
	testseam.Swap(t, &deapChannelLoadBinding, func(dir, profile string) (digitalEmployeeBinding, error) {
		b, err := old(dir, profile)
		calls++
		if calls == 2 {
			auth.SetRuntimeProfile("")
		}
		return b, err
	})
	cmd := newDeapChannelOperatorPrivateCommand()
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("channel", "dsh")
	_ = cmd.Flags().Set("stdin", "true")
	cmd.SetIn(strings.NewReader(`{"schemaVersion":1,"protocolVersion":1,"agentUuid":"agent-1","operatorOpenDingTalkId":"operator-open","text":"body","idempotencyKey":"fixture"}`))
	if err := runDeapChannelOperatorPrivate(cmd, nil); err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeStopTimeoutRetainsBinding(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), digitalEmployeeRunState{PID: os.Getpid(), RunID: "fixture", Profile: cfg.Binding.DWSProfile, AgentUUID: cfg.Binding.AgentUUID}); err != nil {
		t.Fatal(err)
	}
	if err := stopEmployeeRuntime(context.Background(), cfg.Binding); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatal(err)
	}
	if _, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding.DWSProfile); err != nil {
		t.Fatal("stop timeout removed binding", err)
	}
}

func TestCrossPlatformCoverageNamedRegistrationRejectsMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mismatched public command registration accepted")
		}
	}()
	buildCommands([]registeredFactory{{name: "different", factory: func() Handler { return deapHandler{} }}}, &captureRunner{})
}
