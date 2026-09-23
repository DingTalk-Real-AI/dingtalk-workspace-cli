// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDeapAgentPublishRejectsUnverifiableDraft(t *testing.T) {
	for _, response := range []string{
		`{"success":true,"data":null}`,
		`{"success":true,"data":"invalid"}`,
		`{"success":true,"data":{"agentUuid":"another-agent","type":"local_agent"}}`,
		`{"success":true,"data":{"agentUuid":"agent-1","updateUserId":12}}`,
		`{"data":{"agentUuid":"agent-1","type":"local_agent"}}`,
	} {
		t.Run(response, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail":   {response},
				"deap-dev/update_digital_employee_draft": {`{"success":true,"data":{"agentUuid":"agent-1"}}`},
				"deap-dev/publish_digital_employee":      {`{"success":true}`},
			}}
			InitDepsForTest(t, caller)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			if err := corecmd.ExecuteForTest(root); err == nil {
				t.Fatal("unverifiable draft accepted")
			}
			if len(caller.calls) != 1 {
				t.Fatalf("unverifiable draft caused writes: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentPublishPreviewAndConfirmationHaveNoCalls(t *testing.T) {
	for _, dry := range []bool{false, true} {
		t.Run(fmt.Sprint(dry), func(t *testing.T) {
			caller, out := newDeapAgentTestTree(t, dry)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.PersistentFlags().Bool("dry-run", false, "preview")
			argv := []string{"manage", "publish", "--agent-uuid", "agent-1"}
			if dry {
				argv = append(argv, "--dry-run")
			}
			root.SetArgs(argv)
			err := corecmd.ExecuteForTest(root)
			if dry {
				if err != nil {
					t.Fatal(err)
				}
				var plan struct {
					Steps    []string `json:"steps"`
					DryRun   bool     `json:"dry_run"`
					Executed *bool    `json:"executed"`
				}
				if err := json.Unmarshal(out.Bytes(), &plan); err != nil || len(plan.Steps) != 3 || !plan.DryRun || plan.Executed == nil || *plan.Executed {
					t.Fatalf("invalid publish plan: %s", out.String())
				}
			} else if err == nil {
				t.Fatal("publishing without confirmation accepted")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("preview/unconfirmed publish made calls: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentSaveDraftRejectsExplicitBlankFields(t *testing.T) {
	for _, field := range []string{"name", "description", "dept-id", "prompt", "avatar-url", "supervisor-user-id", "type", "response-mode"} {
		for _, value := range []string{"", " \t "} {
			for _, dry := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%q/dry=%t", field, value, dry), func(t *testing.T) {
					caller, _ := newDeapAgentTestTree(t, dry)
					root := deapHandler{}.Command(&captureRunner{})
					root.PersistentFlags().Bool("yes", false, "confirmation")
					root.PersistentFlags().Bool("dry-run", false, "preview")
					argv := []string{"manage", "save-draft", "--agent-uuid", "agent-1", "--" + field, value, "--yes"}
					if dry {
						argv = append(argv, "--dry-run")
					}
					root.SetArgs(argv)
					err := corecmd.ExecuteForTest(root)
					if err == nil || !strings.Contains(err.Error(), "--"+field) {
						t.Fatalf("explicit blank must be rejected with the flag name: %v", err)
					}
					if len(caller.calls) != 0 {
						t.Fatalf("invalid request made calls: %#v", caller.calls)
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageDeapAgentPublishChecksEditorWithoutWritingDraft(t *testing.T) {
	for _, tc := range []struct {
		name, detail, operator, wantError string
		resolveOperator                   bool
	}{
		{"same_editor", `{"agentUuid":"agent-1","type":"local_agent","updateUserId":"operator-1"}`, "operator-1", "", true},
		{"other_editor", `{"agentUuid":"agent-1","type":"local_agent","prompt":"custom","updateUserId":"other-operator"}`, "operator-1", "草稿最近更新人为 other-operator，当前操作人为 operator-1", true},
		{"operator_unavailable", `{"agentUuid":"agent-1","updateUserId":"operator-1"}`, "", "无法确认当前操作人身份", true},
		{"operator_missing_user_id", `{"agentUuid":"agent-1","updateUserId":"operator-1"}`, "  ", "当前操作人 Profile 缺少 userId", true},
		{"legacy_missing_editor", `{"agentUuid":"agent-1","type":"local_agent"}`, "", "", false},
		{"legacy_null_editor", `{"agentUuid":"agent-1","updateUserId":null}`, "", "", false},
		{"legacy_blank_editor", `{"agentUuid":"agent-1","updateUserId":"  "}`, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail": {`{"success":true,"data":` + tc.detail + `}`},
				"deap-dev/publish_digital_employee":    {`{"success":true,"data":{"agentUuid":"agent-1","publishSuccess":true}}`},
			}}
			InitDepsForTest(t, caller)
			resolved := false
			testseam.Swap(t, &deapAgentPublishOperatorUserID, func(context.Context) (string, error) {
				resolved = true
				if tc.operator == "" {
					return "", errors.New("profile unavailable")
				}
				return tc.operator, nil
			})
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			err := corecmd.ExecuteForTest(root)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("publish error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if resolved != tc.resolveOperator {
				t.Fatalf("operator resolved = %t, want %t", resolved, tc.resolveOperator)
			}
			wantCalls := 2
			if tc.wantError != "" {
				wantCalls = 1
			}
			if len(caller.calls) != wantCalls {
				t.Fatalf("calls=%#v, want %d", caller.calls, wantCalls)
			}
			if caller.calls[0].toolName != deapAgentDetailTool || caller.calls[0].args["snapshot"] != "draft" {
				t.Fatalf("must read draft first: %#v", caller.calls)
			}
			if tc.wantError == "" && caller.calls[1].toolName != deapAgentPublishTool {
				t.Fatalf("publish must follow identity check without a draft write: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentPublishResolvesOperatorFromSupervisorProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		setupServerBindingSupervisor(t)
		operator, err := deapAgentPublishOperatorUserID(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if operator != "supervisor" {
			t.Fatalf("operator = %q, want supervisor", operator)
		}
	})

	t.Run("profile_error", func(t *testing.T) {
		setupServerBindingSupervisor(t)
		testseam.Swap(t, &deapConnectLoadSupervisorToken, func(context.Context, string) (*auth.TokenData, error) {
			return nil, errors.New("profile unavailable")
		})
		if _, err := deapAgentPublishOperatorUserID(context.Background()); err == nil || !strings.Contains(err.Error(), "profile unavailable") {
			t.Fatalf("operator profile error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageDeapAgentPublishStopsWhenDraftReadFails(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/get_digital_employee_detail": {`{"success":false,"errorCode":"FORBIDDEN","errorMsg":"read rejected"}`},
	}}
	InitDepsForTest(t, caller)
	root := deapHandler{}.Command(&captureRunner{})
	root.PersistentFlags().Bool("yes", false, "confirmation")
	root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
	if err := corecmd.ExecuteForTest(root); err == nil || !strings.Contains(err.Error(), "read rejected") {
		t.Fatalf("upstream failure must stop publishing: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != deapAgentDetailTool {
		t.Fatalf("published after draft read failure: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDeapAgentPublishPreservesFailureWithoutDefaulting(t *testing.T) {
	for _, agentType := range []string{"local_agent", "open_code"} {
		t.Run(agentType, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail": {fmt.Sprintf(`{"success":true,"data":{"agentUuid":"agent-1","type":%q,"prompt":"existing persona"}}`, agentType)},
				"deap-dev/publish_digital_employee":    {`{"success":false,"errorCode":"INVALID_PARAM","errorMsg":"publish rejected"}`},
			}}
			InitDepsForTest(t, caller)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			err := corecmd.ExecuteForTest(root)
			var apiErr *CLIError
			if !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "publish rejected") {
				t.Fatalf("publish failure must retain its API error: %v", err)
			}
			if apiErr.Code != CodeMCPToolError || !strings.Contains(apiErr.Message, "INVALID_PARAM") {
				t.Fatalf("upstream failure classification changed: %+v", apiErr)
			}
			if strings.Contains(err.Error(), "默认人设已保存") {
				t.Fatalf("failure incorrectly reports a draft modification: %v", err)
			}
			if len(caller.calls) != 2 || caller.calls[0].toolName != deapAgentDetailTool || caller.calls[1].toolName != deapAgentPublishTool {
				t.Fatalf("an existing prompt must not be saved or retried after publish fails: %#v", caller.calls)
			}
		})
	}
}
