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

func TestCrossPlatformCoverageEmployeeSupervisorFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"cancelled", "state", "retry-corrupt", "retry-read", "terminal", "retry-wait", "failure-cleanup", "worker-start", "retry-write"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, filepath.Join(dir, "missing-executable"))
			})
			switch scenario {
			case "cancelled":
				cancel()
			case "state":
				if err := os.MkdirAll(filepath.Join(dir, "runtime.log"), 0700); err != nil {
					t.Fatal(err)
				}
			case "retry-corrupt":
				if err := AtomicWriteJSON(filepath.Join(dir, "retry.json"), []byte("{")); err != nil {
					t.Fatal(err)
				}
			case "retry-read", "retry-write":
				if scenario == "retry-read" {
					if err := os.MkdirAll(filepath.Join(dir, "retry.json"), 0700); err != nil {
						t.Fatal(err)
					}
				} else {
					testseam.Swap(t, &atomicRename, func(src, dst string) error {
						if filepath.Base(dst) == "retry.json" {
							return errors.New("retry write unavailable")
						}
						return os.Rename(src, dst)
					})
				}
			case "terminal":
				if err := writeEmployeeJSON(filepath.Join(dir, "retry.json"), employeeRetryState{Terminal: true}); err != nil {
					t.Fatal(err)
				}
			case "retry-wait":
				if err := writeEmployeeJSON(filepath.Join(dir, "retry.json"), employeeRetryState{NextAttempt: time.Now().Add(time.Hour)}); err != nil {
					t.Fatal(err)
				}
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 100*time.Millisecond)
				defer stop()
			case "failure-cleanup":
				if err := os.MkdirAll(filepath.Join(dir, "failure.json"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "failure.json", "keep"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := superviseDigitalEmployee(ctx, cfg)
			wantError := scenario != "terminal" && scenario != "retry-wait" && scenario != "worker-start" && scenario != "cancelled"
			if scenario == "cancelled" {
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
			if (err != nil) != wantError {
				t.Fatalf("error=%v", err)
			}
			if scenario == "terminal" || scenario == "worker-start" {
				state, err := readDigitalEmployeeState(dir)
				if err != nil || state.Status != "blocked" {
					t.Fatalf("state=%+v err=%v", state, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeLeaseRejectsUnconfirmedOwnership(t *testing.T) {
	for _, scenario := range []string{"identity", "cancelled", "guard", "binding", "server-receipt", "guard-write", "ready-write", "unconfirmed-release"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			auth.SetRuntimeProfile(b.DWSProfile)
			cmd := newDeapConnectCommand()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd.SetContext(ctx)
			cmd.SetIn(strings.NewReader("not-released\n"))
			cmd.SetErr(io.Discard)
			for key, value := range map[string]string{"agent-uuid": b.AgentUUID, "channel": "dsh", "binding-revision": "7", "runtime-instance-id": "00000000-0000-4000-8000-000000000001"} {
				if err := cmd.Flags().Set(key, value); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "identity":
				_ = cmd.Flags().Set("binding-revision", "invalid")
			case "cancelled":
				cancel()
			case "guard":
				if err := writeEmployeeJSON(employeeLeaseGuardPath(b.DWSProfile), employeeLeaseGuard{}); err != nil {
					t.Fatal(err)
				}
			case "binding":
				_ = cmd.Flags().Set("agent-uuid", "different")
			case "server-receipt":
				if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "pending"}); err != nil {
					t.Fatal(err)
				}
			case "guard-write":
				testseam.Swap(t, &atomicRename, func(src, dst string) error {
					if filepath.Base(dst) == "lease-guard.json" {
						return errors.New("disk unavailable")
					}
					return os.Rename(src, dst)
				})
			case "ready-write":
				cmd.SetErr(employeeBoundaryFailWriter{})
			}
			if err := runEmployeeLease(cmd); err == nil {
				t.Fatal("unconfirmed lease released successfully")
			}
		})
	}
}

type employeeBoundaryFailWriter struct{}

func (employeeBoundaryFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("writer unavailable")
}

func TestCrossPlatformCoverageEmployeeFindBindingFailsClosed(t *testing.T) {
	for _, scenario := range []string{"missing", "directory-unreadable", "invalid-json", "invalid-binding", "wrong-path", "different-agent"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
			root := filepath.Join(dir, "digital-employees")
			b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:employee", OperatorOpenDingTalkID: "owner", Channel: "custom"}
			switch scenario {
			case "missing":
			case "directory-unreadable":
				if err := os.WriteFile(root, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid-json":
				if err := AtomicWriteJSON(filepath.Join(root, "bad.json"), []byte("{")); err != nil {
					t.Fatal(err)
				}
			case "invalid-binding":
				if err := writeEmployeeJSON(filepath.Join(root, "bad.json"), b); err != nil {
					t.Fatal(err)
				}
			case "wrong-path", "different-agent":
				if err := saveDigitalEmployeeBinding(dir, b); err != nil {
					t.Fatal(err)
				}
				if scenario == "wrong-path" {
					if err := writeEmployeeJSON(filepath.Join(root, "bad.json"), b); err != nil {
						t.Fatal(err)
					}
				}
			}
			items, err := employeeFindBindings("other-agent")
			if scenario == "wrong-path" || scenario == "invalid-binding" {
				items, err = employeeFindBindings("")
			}
			wantError := scenario != "missing" && scenario != "different-agent"
			if (err != nil) != wantError || (!wantError && len(items) != 0) {
				t.Fatalf("items=%v err=%v", items, err)
			}
		})
	}
}
