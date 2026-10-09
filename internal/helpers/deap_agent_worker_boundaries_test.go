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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeBoundaryConfig(t *testing.T) digitalEmployeeAdapterConfig {
	t.Helper()
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	cfg := digitalEmployeeAdapterConfig{SelfOpenDingTalkID: "self", Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:employee", OperatorOpenDingTalkID: "owner", Channel: "custom"}}
	if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCrossPlatformCoverageEmployeeWorkerFailsClosedBeforeSubscription(t *testing.T) {
	for _, tc := range []struct{ mode, code string }{
		{"guard", "previous_host_release_unconfirmed"},
		{"binding", "binding_not_authorized"},
		{"state", "state_unavailable"},
		{"adapter", "agent_unavailable"},
		{"stdin", "consumer_unavailable"},
		{"stdout", "consumer_unavailable"},
		{"stderr", "consumer_unavailable"},
		{"start", "consumer_unavailable"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
				if tc.mode == "adapter" {
					return nil, errors.New("private adapter failure")
				}
				return &employeeTestForwarder{}, nil
			})
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, filepath.Join(t.TempDir(), "missing-executable"))
				switch tc.mode {
				case "stdin":
					cmd.Stdin = strings.NewReader("")
				case "stdout":
					cmd.Stdout = io.Discard
				case "stderr":
					cmd.Stderr = io.Discard
				}
				return cmd
			})
			switch tc.mode {
			case "guard":
				if err := writeEmployeeJSON(employeeLeaseGuardPath(cfg.Binding.DWSProfile), employeeLeaseGuard{}); err != nil {
					t.Fatal(err)
				}
			case "binding":
				cfg.Binding.AgentUUID = "different"
			case "state":
				if err := os.MkdirAll(filepath.Join(dir, "runtime.log"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := runEmployeeWorker(context.Background(), cfg, 0)
			if err == nil || err.Error() != tc.code {
				t.Fatalf("mode=%s error=%v want=%s", tc.mode, err, tc.code)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeSavedAndForegroundFailWithoutReauthorization(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	previous := auth.RuntimeProfile()
	auth.SetRuntimeProfile(cfg.Binding.DWSProfile)
	t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("agent-uuid", cfg.Binding.AgentUUID)
	_ = cmd.Flags().Set("channel", cfg.Binding.Channel)
	if err := runDigitalEmployeeSaved(cmd); err == nil {
		t.Fatal("missing adapter config accepted")
	}
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json"), cfg); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("agent-uuid", "other")
	if err := runDigitalEmployeeSaved(cmd); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("error=%v", err)
	}
	_ = cmd.Flags().Set("agent-uuid", cfg.Binding.AgentUUID)
	testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
		return nil, errors.New("private initialization error")
	})
	if err := runDigitalEmployeeSaved(cmd); err == nil || err.Error() != "agent_unavailable" {
		t.Fatalf("saved worker=%v", err)
	}
	if err := runDigitalEmployeeForeground(cmd, cfg); err == nil || err.Error() != "agent_unavailable" {
		t.Fatalf("foreground worker=%v", err)
	}
}

func TestCrossPlatformCoverageEmployeeLocalAdapterPreservesLiveProcessAndWriteFailure(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	cmd := newDeapConnectCommand()
	cmd.SetContext(context.Background())
	if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), digitalEmployeeRunState{PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if err := (digitalEmployeeLocalAdapter{}).Connect(cmd, cfg); err == nil || !strings.Contains(err.Error(), "已运行") {
		t.Fatalf("live worker overwritten: %v", err)
	}
	if err := writeEmployeeJSON(filepath.Join(dir, "state.json"), digitalEmployeeRunState{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "retry.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := (digitalEmployeeLocalAdapter{}).Connect(cmd, cfg); err == nil {
		t.Fatal("retry persistence failure ignored")
	}
}
