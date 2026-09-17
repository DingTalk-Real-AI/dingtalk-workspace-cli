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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeDaemonStartupFailures(t *testing.T) {
	for _, scenario := range []string{"unsupported", "executable", "stage", "log", "start", "exited", "cancelled", "timeout", "blocked", "running", "cancel-after-start"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			cmd := newDeapConnectCommand()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			testseam.Swap(t, &daemonDetachEnabled, scenario != "unsupported")
			if runtime.GOOS == "windows" {
				// Exercise orchestration below the unsupported-platform guard.
				// Windows exec resolves an extensionless Path through .exe; only
				// the test fixture gets this alias, not the public daemon command.
				testseam.Swap(t, &daemonRename, func(src, dst string) error {
					if err := os.Rename(src, dst); err != nil {
						return err
					}
					return os.Link(dst, dst+".exe")
				})
			}
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
			if scenario == "cancel-after-start" {
				done := make(chan struct{})
				go func() {
					defer close(done)
					for ctx.Err() == nil {
						if _, err := os.Stat(filepath.Join(dir, "fixture-started")); err == nil {
							cancel()
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
				}()
				defer func() { cancel(); <-done }()
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
			if (err == nil) != (scenario == "running") {
				t.Fatalf("startup %s: %v", scenario, err)
			}
			want := map[string]string{
				"unsupported": "不支持后台运行", "executable": "executable unavailable",
				"stage": "stage unavailable", "exited": "后台进程退出",
				"timeout": "尚未 ready", "blocked": "fixture_blocked",
			}[scenario]
			if want != "" && !strings.Contains(err.Error(), want) {
				t.Fatalf("startup %s: got %v, want %s", scenario, err, want)
			}
			switch scenario {
			case "exited", "timeout", "blocked", "running", "cancel-after-start":
				if child == nil || child.Process == nil {
					t.Fatalf("startup %s never reached the running subprocess: %v", scenario, err)
				}
			}
			if scenario == "cancel-after-start" && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "后台进程退出") {
				t.Fatalf("cancelled running subprocess: %v", err)
			}
		})
	}
}

func TestEmployeeDaemonBoundaryFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-daemon-boundary" {
		return
	}
	switch os.Getenv("DWS_DAEMON_BOUNDARY") {
	case "cancel-after-start":
		if err := os.WriteFile(filepath.Join(os.Getenv("DWS_DAEMON_BOUNDARY_DIR"), "fixture-started"), []byte("ready"), 0600); err != nil {
			os.Exit(2)
		}
	case "exited":
		os.Exit(1)
	case "blocked", "running":
		if err := writeEmployeeJSON(filepath.Join(os.Getenv("DWS_DAEMON_BOUNDARY_DIR"), "state.json"), digitalEmployeeRunState{SupervisorPID: os.Getpid(), Status: os.Getenv("DWS_DAEMON_BOUNDARY"), Code: "fixture_blocked"}); err != nil {
			os.Exit(2)
		}
	}
	time.Sleep(10 * time.Second)
}

func TestCrossPlatformCoverageEmployeeSupervisorWorkerExit(t *testing.T) {
	for _, scenario := range []string{"success", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			t.Setenv("DWS_WORKER_EXIT_SCENARIO", scenario)
			t.Setenv("DWS_WORKER_EXIT_DIR", dir)
			var child *exec.Cmd
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, bin string, _ ...string) *exec.Cmd {
				child = exec.CommandContext(ctx, bin, "-test.run=^TestEmployeeSupervisorExitFixture$", "--", "employee-worker-exit")
				return child
			})
			watchDone := make(chan struct{})
			go func() {
				defer close(watchDone)
				for scenario == "cancelled" && ctx.Err() == nil {
					if _, err := os.Stat(filepath.Join(dir, "worker-started")); err == nil {
						cancel()
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
			}()
			defer func() { cancel(); <-watchDone }()
			if err := superviseDigitalEmployee(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			if child == nil || child.ProcessState == nil || processAlive(child.Process.Pid) {
				t.Fatal("supervisor did not start and reap its worker")
			}
			if _, err := os.Stat(filepath.Join(dir, "worker-started")); err != nil {
				t.Fatalf("worker never reached fixture: %v", err)
			}
			if scenario == "success" && (!child.ProcessState.Success() || ctx.Err() != nil) {
				t.Fatalf("successful worker exit: %v, %v", child.ProcessState, ctx.Err())
			}
			if scenario == "cancelled" {
				state, err := readDigitalEmployeeState(dir)
				if !errors.Is(ctx.Err(), context.Canceled) || err != nil || state.Status != "stopped" {
					t.Fatalf("cancelled worker state: %+v, %v, %v", state, err, ctx.Err())
				}
			}
		})
	}
}

func TestEmployeeSupervisorExitFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-worker-exit" {
		return
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DWS_WORKER_EXIT_DIR"), "worker-started"), []byte("ready"), 0600); err != nil {
		os.Exit(2)
	}
	if os.Getenv("DWS_WORKER_EXIT_SCENARIO") == "cancelled" {
		// Cancellation is handled by the supervisor, including its bounded
		// WaitDelay fallback on platforms without SIGTERM support.
		time.Sleep(time.Minute)
	}
}

func TestCrossPlatformCoverageEmployeeLifecycleRestartFailures(t *testing.T) {
	for _, scenario := range []string{"missing", "cancelled", "unbound", "binding-write", "stop", "pending", "consume", "unbinding", "running-write", "register", "start", "not-ready", "ready", "missing-adapter", "local-adapter"} {
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
			case "missing-adapter", "local-adapter":
				b.Channel = "custom"
			}
			if scenario == "local-adapter" {
				cfg := digitalEmployeeAdapterConfig{Binding: b, SelfOpenDingTalkID: "employee-open-id"}
				if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"), cfg); err != nil {
					t.Fatal(err)
				}
				testseam.Swap(t, &daemonDetachEnabled, false)
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
			if scenario == "local-adapter" && !strings.Contains(err.Error(), "不支持后台运行") {
				t.Fatalf("local restart did not reach daemon dispatch: %v", err)
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
