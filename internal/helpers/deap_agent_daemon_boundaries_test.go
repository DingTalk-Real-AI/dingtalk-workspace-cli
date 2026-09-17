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
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeDaemonStartupFailures(t *testing.T) {
	for _, scenario := range []string{"unsupported", "executable", "stage", "log", "start", "exited", "cancelled", "timeout", "blocked"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			cmd := newDeapConnectCommand()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			testseam.Swap(t, &daemonDetachEnabled, scenario != "unsupported")
			var child *exec.Cmd
			if scenario == "executable" {
				testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("executable unavailable") })
			}
			if scenario == "stage" {
				testseam.Swap(t, &daemonCreateTemp, func(string, string) (*os.File, error) { return nil, errors.New("stage unavailable") })
			}
			if scenario == "log" {
				if err := os.MkdirAll(filepath.Join(dir, "runtime.log"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			testseam.Swap(t, &employeeExecCommand, func(_ context.Context, bin string, _ ...string) *exec.Cmd {
				child = exec.CommandContext(ctx, bin, "-test.run=^TestEmployeeDaemonBoundaryFixture$", "--", "employee-daemon-boundary")
				child.Env = []string{"DWS_DAEMON_BOUNDARY=" + scenario, "DWS_DAEMON_BOUNDARY_DIR=" + dir}
				if scenario == "start" {
					child.Dir = filepath.Join(dir, "nonexistent")
				}
				return child
			})
			if scenario == "timeout" {
				testseam.Swap(t, &digitalEmployeeReadyTimeout, -15*time.Second)
			}
			if scenario == "cancelled" {
				cancel()
			}
			err := startDigitalEmployeeDaemon(cmd, cfg)
			cancel()
			if child != nil && child.Process != nil {
				// startDigitalEmployeeDaemon owns Wait; wait for its goroutine before
				// temporary paths or package seams are restored.
				deadline := time.Now().Add(5 * time.Second)
				for processAlive(child.Process.Pid) && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if processAlive(child.Process.Pid) {
					t.Fatal("fixture did not exit")
				}
			}
			if err == nil {
				t.Fatal("startup failure reported success")
			}
		})
	}
}

func TestEmployeeDaemonBoundaryFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-daemon-boundary" {
		return
	}
	switch os.Getenv("DWS_DAEMON_BOUNDARY") {
	case "exited":
		os.Exit(1)
	case "blocked":
		if err := writeEmployeeJSON(filepath.Join(os.Getenv("DWS_DAEMON_BOUNDARY_DIR"), "state.json"), digitalEmployeeRunState{SupervisorPID: os.Getpid(), Status: "blocked", Code: "fixture_blocked"}); err != nil {
			os.Exit(2)
		}
	}
	time.Sleep(10 * time.Second)
}

func TestCrossPlatformCoverageEmployeeLifecycleRestartFailures(t *testing.T) {
	for _, scenario := range []string{"missing", "cancelled", "unbound", "binding-write", "stop", "pending", "consume", "unbinding", "running-write", "register", "start", "not-ready", "ready", "missing-adapter"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			cmd := newDigitalEmployeeRestartCommand()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			_ = cmd.Flags().Set("agent-uuid", b.AgentUUID)
			switch scenario {
			case "missing":
				_ = cmd.Flags().Set("agent-uuid", "missing")
			case "cancelled":
				lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(lock.Release)
				cancel()
			case "unbound":
				b.BindingState = "unbound"
			case "unbinding":
				b.BindingState = "unbinding"
			case "pending":
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "pending"}); err != nil {
					t.Fatal(err)
				}
			case "consume":
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "confirmed"}); err != nil {
					t.Fatal(err)
				}
			case "missing-adapter":
				b.Channel = "custom"
			}
			if err := updateEmployeeBinding(b); err != nil {
				t.Fatal(err)
			}
			n := 0
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				if dst == digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile) {
					n++
				}
				if (scenario == "binding-write" && n == 1) || (scenario == "running-write" && n == 2) || (scenario == "consume" && dst == employeeServerOperationPath(b.DWSProfile)) {
					return errors.New("write unavailable")
				}
				return os.Rename(src, dst)
			})
			testseam.Swap(t, &deapConnectRegisterDSH, func(context.Context, map[string]any) (string, error) {
				if scenario == "register" {
					return "", errors.New("register unavailable")
				}
				return "created", nil
			})
			testseam.Swap(t, &employeeDSHControl, func(_ context.Context, _ digitalEmployeeBinding, action string) (employeeDSHState, error) {
				if (scenario == "stop" && action == "stop") || (scenario == "start" && action == "start") {
					return employeeDSHState{}, errors.New("host unavailable")
				}
				state := "stopped"
				if action == "start" {
					state = "running"
				}
				return employeeDSHState{Released: true, RuntimeState: state, TransportReady: scenario != "not-ready", ExecutorReady: true}, nil
			})
			err := runDigitalEmployeeLifecycle(cmd, "restart")
			if (err == nil) != (scenario == "ready") {
				t.Fatalf("restart=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeExecutableAndSupervisorErrors(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("executable unavailable") })
	if _, err := employeeCommand(context.Background(), cfg.Binding.DWSProfile); err == nil {
		t.Fatal("executable failure ignored")
	}
	if err := superviseDigitalEmployee(context.Background(), cfg); err == nil {
		t.Fatal("supervisor command failure ignored")
	}
	previous := auth.RuntimeProfile()
	auth.SetRuntimeProfile(cfg.Binding.DWSProfile)
	t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json"), cfg); err != nil {
		t.Fatal(err)
	}
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("agent-uuid", cfg.Binding.AgentUUID)
	_ = cmd.Flags().Set("channel", cfg.Binding.Channel)
	_ = cmd.Flags().Set("local-supervise", "true")
	if err := runDigitalEmployeeSaved(cmd); err == nil {
		t.Fatal("saved supervisor failure ignored")
	}
}
