// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeTransportDisconnectAndRecovery(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	phase := filepath.Join(dir, "phase")
	t.Setenv("DWS_EMPLOYEE_EVENT_FIXTURE", "transport-lifecycle")
	t.Setenv("DWS_EMPLOYEE_PHASE_FILE", phase)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeSubprocessFixture$", "--", "employee-consume-fixture")
	})
	testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
		return &employeeTestForwarder{}, nil
	})
	cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "fixture-agent", DWSProfile: "fixture-corp:fixture-user", Channel: "custom", OperatorOpenDingTalkID: "fixture-owner"}}
	if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runEmployeeWorker(ctx, cfg, 0); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("worker 未退出")
		}
	})
	for _, step := range []struct {
		trigger, source, status string
		ready                   bool
	}{
		{"", "connecting", "starting", false}, {"connect", "connected", "running", true}, {"disconnect", "reconnecting", "running", false}, {"recover", "idle", "running", true},
	} {
		if step.trigger != "" {
			if err := os.WriteFile(phase, []byte(step.trigger), 0600); err != nil {
				t.Fatal(err)
			}
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			s, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile))
			if err == nil && s.SourceState == step.source && s.Status == step.status {
				if s.TransportReady != step.ready || !s.ExecutorReady {
					t.Fatalf("传输变化误改 Agent 状态: %+v", s)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("未观测到 %s: %+v err=%v", step.source, s, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDSHTransportIdentityIsolation(t *testing.T) {
	b := digitalEmployeeBinding{DWSProfile: "fixture-corp:fixture-user"}
	identity := personal.Identity{CorpID: "fixture-corp", UserID: "fixture-user", ClientID: "fixture-app", SourceID: "open"}
	hash := dwsevent.IdentityHash(identity.Key())
	for _, scenario := range []string{"ready", "no_bus", "another_employee", "ambiguous", "wrong_pid", "wrong_source", "no_single_chat", "legacy_bus", "list_error", "rpc_error"} {
		t.Run(scenario, func(t *testing.T) {
			entry := busctl.BusEntry{WorkDir: t.TempDir(), State: busctl.BusStateRunning, SourceKind: dwsevent.SourceKindPersonalStream, IdentityHash: hash, HolderPID: 42, Meta: &bus.Meta{ClientID: identity.ClientID, SourceID: "open"}}
			entries := []busctl.BusEntry{entry}
			response := transport.StatusResp{Bus: transport.StatusBus{PID: 42, IdentityHash: hash, SourceID: "open"}, SourceState: transport.StatusSource{Observed: true, State: "connected"}, Consumers: []transport.StatusConsumer{{EventTypes: []string{personal.EventAllSingleChat}}}}
			switch scenario {
			case "no_bus":
				entries = nil
			case "another_employee":
				entries[0].IdentityHash = "another-identity"
			case "ambiguous":
				entries = append(entries, entry)
			case "wrong_pid":
				response.Bus.PID = 43
			case "wrong_source":
				response.Bus.SourceID = "another-source"
			case "no_single_chat":
				response.Consumers[0].EventTypes = []string{"user_card_action_triggered"}
			case "legacy_bus":
				response.SourceState.Observed = false
			}
			testseam.Swap(t, &employeeEventBuses, func(string, string) ([]busctl.BusEntry, error) {
				if scenario == "list_error" {
					return nil, errors.New("fixture")
				}
				return entries, nil
			})
			testseam.Swap(t, &employeeEventStatus, func(string) (*transport.StatusResp, error) {
				if scenario == "rpc_error" {
					return nil, errors.New("fixture")
				}
				return &response, nil
			})
			got := observeEmployeeDSHTransport(b)
			if got.Observed != (scenario == "ready") {
				t.Fatalf("借用了其他来源的就绪状态: %+v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFourAdapterReadiness(t *testing.T) {
	for _, channel := range []string{"qoder", "codex", "custom", "dsh"} {
		t.Run(channel, func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
			b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "fixture-agent", DWSProfile: "fixture-corp:fixture-user", Channel: channel}
			testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
				return employeeDSHState{RuntimeState: "running", TransportReady: true, ExecutorReady: true}, nil
			})
			for _, tc := range []struct {
				state           string
				observed, ready bool
			}{
				{"connected", false, false}, {"connecting", true, false}, {"connected", true, true}, {"reconnecting", true, false}, {"idle", true, true}, {"stopped", true, false},
			} {
				t.Run(tc.state+"_"+map[bool]string{true: "observed", false: "legacy"}[tc.observed], func(t *testing.T) {
					testseam.Swap(t, &employeeDSHTransportStatus, func(digitalEmployeeBinding) transport.StatusSource {
						return transport.StatusSource{State: tc.state, Observed: tc.observed}
					})
					if err := persistEmployeeState(digitalEmployeeRuntimeDir(b.DWSProfile), digitalEmployeeRunState{RunID: "fixture", Status: "running", PID: os.Getpid(), AgentUUID: b.AgentUUID, Profile: b.DWSProfile, Channel: channel, TransportReady: tc.ready, ExecutorReady: true, SourceState: tc.state}); err != nil {
						t.Fatal(err)
					}
					got := employeeLifecycleStatus(t.Context(), b)
					if got["transportReady"] != tc.ready || got["executorReady"] != true {
						t.Fatalf("就绪状态失真: %+v", got)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeIPCReadyIsNotTransportReady(t *testing.T) {
	for _, mode := range []string{"ipc-only", "legacy-ready"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
			testseam.Swap(t, &digitalEmployeeReadyTimeout, 100*time.Millisecond)
			t.Setenv("DWS_EMPLOYEE_EVENT_FIXTURE", mode)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeSubprocessFixture$", "--", "employee-consume-fixture")
			})
			testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
				return &employeeTestForwarder{}, nil
			})
			cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "fixture-agent", DWSProfile: "fixture-corp:fixture-user", Channel: "custom", OperatorOpenDingTalkID: "fixture-owner"}}
			if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
				t.Fatal(err)
			}
			err := runEmployeeWorker(t.Context(), cfg, 0)
			expected := "ready_timeout"
			if mode == "legacy-ready" {
				expected = "event_bus_upgrade_required"
			}
			if err == nil || !strings.Contains(err.Error(), expected) {
				t.Fatalf("仅 IPC 就绪不能启动员工: %v", err)
			}
			s, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile))
			if err != nil || s.TransportReady || s.ExecutorReady || s.Status != "blocked" {
				t.Fatalf("退出后仍报告就绪: %+v err=%v", s, err)
			}
		})
	}
}
