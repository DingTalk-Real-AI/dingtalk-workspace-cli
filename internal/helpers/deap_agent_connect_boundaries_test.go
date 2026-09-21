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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEmployeeConnectCommitBoundaries(t *testing.T) {
	for _, scenario := range []string{"draft", "cancelled-lock", "existing-live", "existing-dsh", "missing-self", "options", "device", "adapter-write", "consume-write", "resume-unbound", "local-start", "device-mismatch", "adapter-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			setupSuccessfulConnectSeams(t)
			caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"owner"}]}`)
			InitDepsForTest(t, caller)
			channel := "dsh"
			if scenario == "existing-live" || scenario == "missing-self" || scenario == "options" || scenario == "local-start" || scenario == "device-mismatch" {
				channel = "custom"
			}
			cmd := newConnectTestCommandWithMode(t, false, channel)
			cmd.SetOut(io.Discard)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd.SetContext(ctx)
			_ = cmd.Flags().Set("agent-cmd", "fixture")
			b := digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent-1", DWSProfile: "employee-corp:employee-user", OperatorOpenDingTalkID: "owner", Channel: channel, BindingRevision: 1, BindingState: "bound", DesiredState: "running"}
			if scenario != "missing-self" {
				caller.responses["deap-dev/get_digital_employee_detail"][1] = strings.Replace(caller.responses["deap-dev/get_digital_employee_detail"][1], `"userId":"employee-user"`, `"userId":"employee-user","openDingTalkId":"self"`, 1)
			}
			if channel == "dsh" {
				_ = cmd.Flags().Set("agent-cmd", "")
				cmd.Flags().Lookup("agent-cmd").Changed = false
			}
			switch scenario {
			case "draft":
				delete(caller.responses, "deap-dev/get_digital_employee_detail")
			case "cancelled-lock":
				lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "operation"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(lock.Release)
				cancel()
			case "existing-live":
				if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "state.json"), digitalEmployeeRunState{PID: os.Getpid()}); err != nil {
					t.Fatal(err)
				}
			case "existing-dsh", "resume-unbound":
				if scenario == "resume-unbound" {
					b.BindingState = "unbound"
					b.RuntimeBindingID = "old"
					b.DeviceID = "old"
				}
				if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), b); err != nil {
					t.Fatal(err)
				}
			case "options":
				_ = cmd.Flags().Set("allowed-users", "unknown-user")
			case "device":
				_ = cmd.Flags().Set("device-id", "invalid\nidentity")
			case "adapter-unavailable":
				testseam.Swap(t, &digitalEmployeeResolveAdapter, func(string) (digitalEmployeeAdapter, error) { return nil, errors.New("adapter unavailable") })
			case "device-mismatch":
				b.DeviceID = "11111111-1111-4111-8111-111111111111"
				if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), b); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Flags().Set("device-id", "22222222-2222-4222-8222-222222222222")
			case "local-start":
				testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
					return nil, errors.New("agent unavailable")
				})
			}
			testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
				return employeeDSHState{}, errors.New("host unavailable")
			})
			if scenario == "adapter-write" || scenario == "consume-write" {
				n := 0
				testseam.Swap(t, &atomicRename, func(src, dst string) error {
					if dst == employeeServerOperationPath(b.DWSProfile) {
						n++
					}
					if (scenario == "adapter-write" && filepath.Base(dst) == "adapter.json") || (scenario == "consume-write" && n == 3 && dst == employeeServerOperationPath(b.DWSProfile)) {
						return errors.New("storage unavailable")
					}
					return os.Rename(src, dst)
				})
			}
			// Invoke the orchestration directly; CLI flag validation is tested separately.
			err := runDeapConnect(cmd, nil)
			if err == nil && scenario != "resume-unbound" {
				t.Fatal("incomplete connect reported success")
			}
			if scenario == "resume-unbound" {
				raw, readErr := os.ReadFile(filepath.Join(digitalEmployeeRuntimeDir(b.DWSProfile), "adapter.json"))
				if readErr != nil || !strings.Contains(string(raw), `"bindingRevision":2`) {
					t.Fatalf("resume did not advance revision: %s %v %v", raw, readErr, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeMissingDependenciesAndInputLimits(t *testing.T) {
	newDeapAgentTestTree(t, false)
	testseam.Swap(t, &deps, nil)
	for _, cmd := range []*cobra.Command{newDeapAgentLoginCommand(), newDeapConnectCommand()} {
		cmd.SetContext(context.Background())
		_ = cmd.Flags().Set("agent-uuid", "agent")
		if cmd.Flags().Lookup("channel") != nil {
			_ = cmd.Flags().Set("channel", "dsh")
		}
		if err := cmd.RunE(cmd, nil); err == nil {
			t.Fatal("missing caller accepted")
		}
	}
	if _, err := callPrivateMCPJSON(context.Background(), "server", "tool", nil); err == nil {
		t.Fatal("private call without caller accepted")
	}
	cmd := newDeapChannelReplyCommand()
	cmd.SetIn(strings.NewReader(`{} {}`))
	if err := decodeBoundedDigitalEmployeeStdin(cmd, new(digitalEmployeeReplyInput)); err == nil {
		t.Fatal("multiple payloads accepted")
	}
}
