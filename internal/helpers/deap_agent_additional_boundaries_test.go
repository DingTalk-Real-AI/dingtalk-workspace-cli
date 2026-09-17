// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeAdditionalValidation(t *testing.T) {
	for _, item := range []struct {
		update bool
		flag   string
		limit  int
	}{{false, "name", 30}, {false, "description", 300}, {false, "employee-no", 64}, {true, "name", 30}, {true, "description", 300}} {
		t.Run(item.flag+map[bool]string{true: "-update", false: "-create"}[item.update], func(t *testing.T) {
			newDeapAgentTestTree(t, false)
			cmd := newDeapAgentCreateCommand()
			if item.update {
				cmd = newDeapAgentSaveDraftCommand()
			}
			cmd.SetContext(context.Background())
			_ = cmd.Flags().Set("name", "fixture")
			_ = cmd.Flags().Set("description", "fixture")
			if cmd.Flags().Lookup("agent-uuid") != nil {
				_ = cmd.Flags().Set("agent-uuid", "agent")
			}
			_ = cmd.Flags().Set(item.flag, strings.Repeat("界", item.limit+1))
			if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "最多允许") {
				t.Fatalf("oversized field not rejected by size validation: %v", err)
			}
		})
	}
	for _, scenario := range []string{"daemon", "permission", "workdir"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := newConnectTestCommandWithMode(t, false, "custom")
			_ = cmd.Flags().Set("agent-cmd", "fixture")
			switch scenario {
			case "daemon":
				_ = cmd.Flags().Set("daemon", "true")
				testseam.Swap(t, &daemonDetachEnabled, false)
			case "permission":
				_ = cmd.Flags().Set("agent-permission-mode", "invalid")
			case "workdir":
				_ = cmd.Flags().Set("agent-workdir", t.TempDir())
				testseam.Swap(t, &employeeAbsPath, func(string) (string, error) { return "", errors.New("unavailable") })
			}
			if err := validateDigitalEmployeeAdapter(cmd); err == nil {
				t.Fatal("invalid adapter options accepted")
			}
		})
	}
	t.Setenv("DWS_AGENT_CMD", "")
	if _, err := digitalEmployeeNewForwarder(context.Background(), digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{Channel: "custom"}, Options: connectAgentOptions{Command: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"missing", "custom"} {
		if _, err := newLocalAgentForwarder(channel, "scope", connectAgentOptions{}); err == nil {
			t.Fatal("invalid forwarder accepted")
		}
	}
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("missing") })
	if _, err := employeeMachineCall(context.Background(), "corp:user", nil); err == nil {
		t.Fatal("missing executable accepted")
	}
	if got := deapAgentSkillStageFromResponse([]byte("no stage"), "fallback"); got != "fallback" {
		t.Fatal(got)
	}
	newDeapAgentTestTree(t, false)
	deps.Caller.(*deapAgentCaller).err = errors.New("unavailable")
	if _, err := (deapAgentOpenAPISkillUploader{}).temporaryCredential(context.Background(), "agent"); err == nil {
		t.Fatal("credential request failure ignored")
	}
}

func TestCrossPlatformCoverageEmployeeEntryAndReleaseLocks(t *testing.T) {
	for _, mode := range []string{"local-worker", "local-supervise", "local-lease", "invalid-channel", "local-preview"} {
		t.Run(mode, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			cmd := newConnectTestCommandWithMode(t, mode == "local-preview", "custom")
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			if mode == "invalid-channel" {
				_ = cmd.Flags().Set("channel", "missing")
			} else if mode != "local-preview" {
				_ = cmd.Flags().Set(mode, "true")
			}
			if mode == "local-worker" || mode == "local-supervise" {
				auth.SetRuntimeProfile(cfg.Binding.DWSProfile)
			}
			err := runDeapConnect(cmd, nil)
			if mode == "local-preview" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid saved entry accepted")
			}
		})
	}
	for _, scenario := range []string{"stop-activation", "release-worker", "lease-worker", "unbind-operation"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			kind := "worker"
			if scenario == "stop-activation" {
				kind = "activation"
				b.Channel = "custom"
			}
			if scenario == "unbind-operation" {
				kind = "operation"
			}
			lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), kind))
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Release()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			switch scenario {
			case "stop-activation":
				err = stopEmployeeRuntime(ctx, b)
			case "release-worker":
				err = confirmEmployeeRuntimeReleased(ctx, b, "")
			case "unbind-operation":
				cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
				cmd.SetContext(ctx)
				err = runEmployeeUnbind(cmd)
			case "lease-worker":
				auth.SetRuntimeProfile(b.DWSProfile)
				cmd := newDeapConnectCommand()
				cmd.SetContext(ctx)
				_ = cmd.Flags().Set("binding-revision", "7")
				_ = cmd.Flags().Set("runtime-instance-id", "11111111-1111-4111-8111-111111111111")
				err = runEmployeeLease(cmd)
			}
			if err == nil {
				t.Fatal("held lock accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeLookupAndTransportBoundaries(t *testing.T) {
	t.Run("missing-binding", func(t *testing.T) {
		employeeBoundaryConfig(t)
		cmd := lifecycleCmd(t, "unbind", "absent")
		if err := runEmployeeUnbind(cmd); err == nil {
			t.Fatal("missing binding accepted")
		}
	})
	t.Run("broken-directory", func(t *testing.T) {
		cfg := employeeBoundaryConfig(t)
		testseam.Swap(t, &employeeStat, func(string) (os.FileInfo, error) { return nil, errors.New("store unavailable") })
		cmd := lifecycleCmd(t, "unbind", cfg.Binding.AgentUUID)
		if err := runEmployeeUnbind(cmd); err == nil {
			t.Fatal("broken store accepted")
		}
		if err := runDigitalEmployeeLifecycle(cmd, "stop"); err == nil {
			t.Fatal("broken store accepted")
		}
	})
	if status := observeEmployeeDSHTransport(digitalEmployeeBinding{DWSProfile: "invalid"}); status.State != "unknown" {
		t.Fatal(status)
	}
	testseam.Swap(t, &employeeEventBuses, func(string, string) ([]busctl.BusEntry, error) { return []busctl.BusEntry{{}}, nil })
	if status := observeEmployeeDSHTransport(digitalEmployeeBinding{DWSProfile: "corp:user"}); status.State != "unknown" {
		t.Fatal(status)
	}
}

func TestCrossPlatformCoverageEmployeeLoginExchangeIdentity(t *testing.T) {
	for _, failExchange := range []bool{false, true} {
		t.Run(map[bool]string{true: "identity", false: "authorization"}[failExchange], func(t *testing.T) {
			setupSuccessfulConnectSeams(t)
			caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[]}`)
			caller.responses["deap-dev/get_digital_employee_detail"] = caller.responses["deap-dev/get_digital_employee_detail"][1:]
			InitDepsForTest(t, caller)
			if failExchange {
				testseam.Swap(t, &deapConnectManagedExchange, func(context.Context, string, auth.ManagedExchangeRequest) (*auth.TokenData, error) {
					return &auth.TokenData{CorpID: "wrong", UserID: "wrong"}, nil
				})
			} else {
				delete(caller.responses, "deap-dev/get_dws_auth_code")
			}
			cmd := newDeapAgentLoginCommand()
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			_ = cmd.Flags().Set("agent-uuid", "agent-1")
			if err := runDeapAgentLogin(cmd, nil); err == nil {
				t.Fatal("incomplete login accepted")
			}
		})
	}
}
