// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type employeeOrdinaryBlocksCaller struct {
	digitalEmployeeProtocolCaller
	blocks []edition.ContentBlock
}

func (c *employeeOrdinaryBlocksCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{Content: c.blocks}, nil
}

func TestCrossPlatformCoverageEmployeePrivateResultValidation(t *testing.T) {
	for _, blocks := range [][]edition.ContentBlock{
		{{Type: "image"}, {Type: "text", Text: " "}}, {{Type: "text", Text: "{"}}, {{Type: "text", Text: `{"success":false}`}},
	} {
		InitDepsForTest(t, &employeeOrdinaryBlocksCaller{blocks: blocks})
		if _, err := callPrivateMCPJSON(context.Background(), "test", "fixture", nil); err == nil {
			t.Fatal("untrusted result accepted")
		}
	}
	cmd := newEmployeeBindingCommand()
	cmd.SetIn(employeeReadFailure{})
	if err := decodeBoundedDigitalEmployeeStdin(cmd, new(struct {
		AgentUUID string `json:"agentUuid"`
	})); err == nil {
		t.Fatal("read failure accepted")
	}
	if _, err := resolveDigitalEmployeeDelivery(nil, map[string]any{"openMessageId": "message"}, "", "key"); err != nil {
		t.Fatal(err)
	}
}

type employeeReadFailure struct{}

func (employeeReadFailure) Read([]byte) (int, error) { return 0, errors.New("stdin unavailable") }

func TestCrossPlatformCoverageEmployeeSupervisorSnapshotFailures(t *testing.T) {
	for _, scenario := range []string{"no-profile", "load-token", "missing-identity", "refresh", "refresh-empty", "refresh-other"} {
		t.Run(scenario, func(t *testing.T) {
			setupServerBindingSupervisor(t)
			switch scenario {
			case "no-profile":
				testseam.Swap(t, &deapConnectLoadProfiles, func(string) (*auth.ProfilesConfig, error) { return &auth.ProfilesConfig{}, nil })
			case "load-token":
				testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) { return nil, errors.New("cannot read token") })
			case "missing-identity":
				testseam.Swap(t, &deapConnectLoadToken, func(string, string) (*auth.TokenData, error) { return &auth.TokenData{}, nil })
			default:
				testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
					if scenario == "refresh" {
						return nil, errors.New("refresh failed")
					}
					if scenario == "refresh-empty" {
						return nil, nil
					}
					return &auth.TokenData{CorpID: "other", UserID: "other", AccessToken: "fixture-token"}, nil
				})
			}
			if _, _, err := currentSupervisorProfile(context.Background(), t.TempDir()); err == nil {
				t.Fatal("unverified supervisor snapshot accepted")
			}
		})
	}
}
