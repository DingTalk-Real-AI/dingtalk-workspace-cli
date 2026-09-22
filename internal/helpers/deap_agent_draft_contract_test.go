// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func TestCrossPlatformCoverageDeapAgentPublishRejectsUnverifiableDraft(t *testing.T) {
	for _, response := range []string{
		`{"success":true,"data":null}`,
		`{"success":true,"data":"invalid"}`,
		`{"success":true,"data":{"agentUuid":"another-agent","type":"local_agent"}}`,
		`{"success":true,"data":{"agentUuid":"agent-1","type":"local_agent","prompt":12}}`,
		`{"data":{"agentUuid":"agent-1","type":"local_agent"}}`,
		`{"success":true,"data":{"agentUuid":"agent-1"}}`,
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

func TestCrossPlatformCoverageDeapAgentPublishDefaultsOnlyMissingLocalPrompt(t *testing.T) {
	for _, tc := range []struct {
		name, detail string
		wantSave     bool
	}{
		{"local_missing", `{"agentUuid":"agent-1","type":"local_agent"}`, true},
		{"local_blank", `{"agentUuid":"agent-1","type":"local_agent","prompt":"  "}`, true},
		{"local_custom", `{"agentUuid":"agent-1","type":"local_agent","prompt":"custom persona"}`, false},
		{"legacy_custom", `{"agentUuid":"agent-1","mainProgramType":"local_agent","promptConfig":{"prompt":"custom persona"}}`, false},
		{"legacy_type", `{"agentUuid":"agent-1","digitalTagEmployeeProfile":{"type":"local_agent"}}`, true},
		{"open_code", `{"agentUuid":"agent-1","type":"open_code"}`, false},
		{"legacy_local", `{"agentUuid":"agent-1","digitalTagEmployeeProfile":{"mainProgramType":"local_agent"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail":   {`{"success":true,"data":` + tc.detail + `}`},
				"deap-dev/update_digital_employee_draft": {`{"success":true,"data":{"agentUuid":"agent-1"}}`},
				"deap-dev/publish_digital_employee":      {`{"success":true,"data":{"agentUuid":"agent-1","publishSuccess":true}}`},
			}}
			InitDepsForTest(t, caller)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			wantCalls := 2
			if tc.wantSave {
				wantCalls++
			}
			if len(caller.calls) != wantCalls {
				t.Fatalf("calls=%#v, want %d", caller.calls, wantCalls)
			}
			if caller.calls[0].toolName != deapAgentDetailTool || caller.calls[0].args["snapshot"] != "draft" {
				t.Fatalf("must read draft first: %#v", caller.calls)
			}
			if tc.wantSave {
				call := caller.calls[1]
				prompt, _ := call.args["prompt"].(string)
				if call.toolName != deapAgentSaveDraftTool || call.args["agentUuid"] != "agent-1" || strings.TrimSpace(prompt) == "" || len(call.args) != 2 {
					t.Fatalf("default must only patch prompt: %#v", call)
				}
			}
			if caller.calls[wantCalls-1].toolName != deapAgentPublishTool {
				t.Fatalf("publish must follow initialization: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentPublishStopsWhenDraftReadOrDefaultSaveFails(t *testing.T) {
	for _, stage := range []string{"read", "save"} {
		t.Run(stage, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail":   {`{"success":true,"data":{"agentUuid":"agent-1","type":"local_agent"}}`},
				"deap-dev/update_digital_employee_draft": {`{"success":false,"errorCode":"INVALID_PARAM","errorMsg":"save rejected"}`},
			}}
			if stage == "read" {
				caller.responses["deap-dev/get_digital_employee_detail"] = []string{`{"success":false,"errorCode":"FORBIDDEN","errorMsg":"read rejected"}`}
			}
			InitDepsForTest(t, caller)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			if err := corecmd.ExecuteForTest(root); err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("upstream failure must stop publishing: %v", err)
			}
			for _, call := range caller.calls {
				if call.toolName == deapAgentPublishTool {
					t.Fatal("published after precondition failure")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentPublishReportsPartialState(t *testing.T) {
	for _, tc := range []struct {
		name, save, publish, message string
		wantCalls                    int
	}{
		{"unknown_save", `{}`, `{"success":true}`, "保存结果无法确认", 2},
		{"publish_rejected", `{"success":true}`, `{"success":false,"errorCode":"INVALID_PARAM","errorMsg":"publish rejected"}`, "默认人设已保存，但发布失败", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
				"deap-dev/get_digital_employee_detail":   {`{"success":true,"result":{"agentUuid":"agent-1","type":"local_agent"}}`},
				"deap-dev/update_digital_employee_draft": {tc.save},
				"deap-dev/publish_digital_employee":      {tc.publish},
			}}
			InitDepsForTest(t, caller)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs([]string{"manage", "publish", "--agent-uuid", "agent-1", "--yes"})
			if err := corecmd.ExecuteForTest(root); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("partial state must be explicit: %v", err)
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("unexpected operations: %#v", caller.calls)
			}
		})
	}
}
