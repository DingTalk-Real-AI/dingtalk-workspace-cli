// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDeapAgentCreatePrompt(t *testing.T) {
	for _, agentType := range []string{"open_code", "local_agent"} {
		for _, supplied := range []bool{false, true} {
			for _, avatar := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/supplied=%t/avatar=%t", agentType, supplied, avatar), func(t *testing.T) {
					caller, out := newDeapAgentTestTree(t, false)
					caller.resultText = `{"success":true,"data":{"agentUuid":"created-1"}}`
					root := deapHandler{}.Command(&captureRunner{})
					var warnings bytes.Buffer
					deps.Out.errW = &warnings
					root.SetErr(&warnings)
					argv := []string{"manage", "create", "--name", "helper", "--description", "assist", "--type", agentType}
					if supplied {
						argv = append(argv, "--prompt", "  Custom persona 中文  ")
					}
					uploader := &deapAgentAvatarUploaderStub{fileURL: "https://oss.example/avatar.png"}
					testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(uploader))
					if avatar {
						argv = append(argv, "--avatar-url", writeRelativeImage(t, "avatar.png"))
					}
					root.SetArgs(argv)
					if err := corecmd.ExecuteForTest(root); err != nil {
						t.Fatal(err)
					}
					wantPrompt := supplied || agentType == "local_agent"
					wantCalls := 1
					if wantPrompt || avatar {
						wantCalls = 2
					}
					if len(caller.calls) != wantCalls {
						t.Fatalf("calls=%#v", caller.calls)
					}
					if caller.calls[0].toolName != deapAgentCreateTool {
						t.Fatal("must create first")
					}
					if _, ok := caller.calls[0].args["prompt"]; ok {
						t.Fatal("create API does not support prompt; must save after creation")
					}
					if wantCalls == 2 {
						patch := caller.calls[1]
						if patch.toolName != deapAgentSaveDraftTool || patch.args["agentUuid"] != "created-1" {
							t.Fatalf("wrong patch: %#v", patch)
						}
						prompt, _ := patch.args["prompt"].(string)
						if supplied && prompt != "Custom persona 中文" {
							t.Fatalf("custom prompt lost: %q", prompt)
						}
						if wantPrompt && strings.TrimSpace(prompt) == "" {
							t.Fatal("missing prompt")
						}
						if !wantPrompt && prompt != "" {
							t.Fatal("must not default open_code prompt")
						}
						if avatar && patch.args["avatarUrl"] != "https://oss.example/avatar.png" {
							t.Fatal("avatar lost")
						}
					}
					if strings.Contains(warnings.String(), "--prompt") == wantPrompt {
						t.Fatalf("unexpected warning: %q", warnings.String())
					}
					if !json.Valid(out.Bytes()) {
						t.Fatalf("stdout must contain one JSON result: %s", out.String())
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageDeapAgentCreatePromptPreview(t *testing.T) {
	for _, avatar := range []bool{false, true} {
		t.Run(fmt.Sprint(avatar), func(t *testing.T) {
			caller, out := newDeapAgentTestTree(t, true)
			root := deapHandler{}.Command(&captureRunner{})
			argv := []string{"manage", "create", "--name", "helper", "--description", "assist", "--type", "local_agent"}
			if avatar {
				argv = append(argv, "--avatar-url", writeRelativeImage(t, "avatar.png"))
			}
			root.SetArgs(argv)
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			var plan struct {
				DryRun        bool           `json:"dryRun"`
				AuditedDryRun bool           `json:"dry_run"`
				DraftPatch    map[string]any `json:"draftPatch"`
			}
			if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
				t.Fatal(err)
			}
			if !plan.DryRun || !plan.AuditedDryRun || strings.TrimSpace(fmt.Sprint(plan.DraftPatch["prompt"])) == "" || plan.DraftPatch["prompt"] == nil {
				t.Fatalf("missing default prompt in preview: %s", out.String())
			}
			if len(caller.calls) != 0 {
				t.Fatal("preview made remote calls")
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentCreatePromptValidation(t *testing.T) {
	for _, value := range []string{"", " \t ", strings.Repeat("中", 5001)} {
		caller, _ := newDeapAgentTestTree(t, false)
		root := deapHandler{}.Command(&captureRunner{})
		root.SetArgs([]string{"manage", "create", "--name", "helper", "--description", "assist", "--type", "open_code", "--prompt", value})
		err := corecmd.ExecuteForTest(root)
		if err == nil || !strings.Contains(err.Error(), "--prompt") || strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("expected prompt validation: %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatal("invalid input created a draft")
		}
	}
}

func TestCrossPlatformCoverageDeapAgentCreatePromptPartialFailure(t *testing.T) {
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{
		"deap-dev/create_digital_employee":       {`{"success":true,"data":{"agentUuid":"created-1"}}`},
		"deap-dev/update_digital_employee_draft": {`{"success":false,"errorCode":"INVALID_PARAM","errorMsg":"save rejected"}`},
	}}
	InitDepsForTest(t, caller)
	root := deapHandler{}.Command(&captureRunner{})
	root.SetArgs([]string{"manage", "create", "--name", "helper", "--description", "assist", "--type", "open_code", "--prompt", "Custom persona"})
	err := corecmd.ExecuteForTest(root)
	if err == nil || !strings.Contains(err.Error(), "created-1") || !strings.Contains(err.Error(), "save-draft") {
		t.Fatalf("must preserve recoverable draft identity: %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("must not retry creation: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDeapAgentCreatePromptUnicodeLimit(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	caller.resultText = `{"success":true,"data":{"agentUuid":"created-1"}}`
	root := deapHandler{}.Command(&captureRunner{})
	prompt := strings.Repeat("中", 5000)
	root.SetArgs([]string{"manage", "create", "--name", "helper", "--description", "assist", "--type", "open_code", "--prompt", prompt})
	if err := corecmd.ExecuteForTest(root); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[1].args["prompt"] != prompt {
		t.Fatal("valid Unicode prompt was truncated or rejected")
	}
}
