// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageWikiNodeWriteTargets(t *testing.T) {
	for _, command := range []string{"+node-create", "+node-copy"} {
		for _, tc := range []struct {
			name, body, reason string
			folder             bool
		}{
			{"wrong workspace", `{"nodeId":"created","workspaceId":"other","name":"Doc","extension":"adoc"}`, "workspace_readback_mismatch", false},
			{"missing workspace", `{"success":true,"nodeId":"created","name":"Doc","extension":"adoc"}`, "workspace_readback_mismatch", false},
			{"wrong folder", `{"nodeId":"created","workspaceId":"w","folderId":"other","name":"Doc","extension":"adoc"}`, "folder_readback_mismatch", true},
			{"missing folder", `{"nodeId":"created","workspaceId":"w","name":"Doc","extension":"adoc"}`, "folder_readback_mismatch", true},
			{"wrong node", `{"nodeId":"other","workspaceId":"w","name":"Doc","extension":"adoc"}`, "readback_id_mismatch", false},
			{"missing node", `{"success":true,"workspaceId":"w","name":"Doc","extension":"adoc"}`, "readback_id_mismatch", false},
			{"nested failure", `{"success":true,"result":{"success":false,"nodeId":"created","workspaceId":"w","name":"Doc","extension":"adoc"}}`, "remote_failure", false},
			{"default root", `{"nodeId":"created","workspaceId":"w","folderId":"real-root","name":"Doc","extension":"adoc"}`, "", false},
			{"explicit folder", `{"nodeId":"created","workspaceId":"w","folderId":"f","name":"Doc","extension":"adoc"}`, "", true},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				tool, args := "create_file", []string{command, "--workspace", "w", "--name", "Doc"}
				if command == "+node-copy" {
					tool, args = "copy_document", []string{command, "--workspace", "w", "--node", "source", "--yes"}
				}
				if tc.folder {
					args = append(args, "--folder", "f")
				}
				readbacks := []string{tc.body}
				if command == "+node-copy" {
					readbacks = append([]string{`{"nodeId":"source","workspaceId":"source-w","name":"Source","extension":"adoc"}`}, readbacks...)
				}
				caller := &wikiCoverageCaller{responses: map[string][]string{
					"doc/" + tool:           {`{"success":true,"nodeId":"created","token":"WRITE_SECRET_CANARY"}`},
					"doc/get_document_info": readbacks,
				}}
				out, err := runWikiCoverageCLI(t, caller, args...)
				if command == "+node-copy" {
					if len(caller.calls) != 3 || caller.calls[0].tool != "get_document_info" || caller.calls[1].tool != tool || caller.calls[2].tool != "get_document_info" {
						t.Fatalf("expected source read, one write, and copy read, calls=%#v", caller.calls)
					}
				} else if len(caller.calls) != 2 || caller.calls[0].tool != tool || caller.calls[1].tool != "get_document_info" {
					t.Fatalf("expected one write and one read, calls=%#v", caller.calls)
				}
				if tc.reason == "" {
					if err != nil || out["success"] != true || out["nodeId"] != "created" {
						t.Fatalf("valid write out=%#v err=%v", out, err)
					}
					return
				}
				var typed *apperrors.Error
				if out != nil || !errors.As(err, &typed) || typed.Reason != tc.reason || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || !typed.RetryableSet {
					t.Fatalf("out=%#v err=%#v", out, err)
				}
				receipt, _ := typed.Details["writeReceipt"].(map[string]any)
				if receipt["returnedNodeId"] != "created" || receipt["expectedWorkspaceId"] != "w" || receipt["newResourceConfirmed"] != false {
					t.Fatalf("receipt=%#v", receipt)
				}
				var encoded bytes.Buffer
				if err := apperrors.PrintJSON(&encoded, typed); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(encoded.String(), "WRITE_SECRET_CANARY") {
					t.Fatal("unreviewed write payload leaked into error receipt")
				}
			})
		}
	}
}

func TestCrossPlatformCoverageWikiCopyDistinctIDAndInputBoundary(t *testing.T) {
	caller := &wikiCoverageCaller{responses: map[string][]string{
		"doc/get_document_info": {`{"nodeId":"source","workspaceId":"source-w","name":"Source","extension":"adoc"}`},
		"doc/copy_document":     {`{"success":true,"nodeId":"source"}`},
	}}
	_, err := runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--node", "source", "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "copy_id_not_new" || len(caller.calls) != 2 {
		t.Fatalf("same-ID copy err=%#v calls=%#v", err, caller.calls)
	}
	receipt, _ := typed.Details["writeReceipt"].(map[string]any)
	if receipt["returnedNodeId"] != "source" || receipt["sourceNodeId"] != "source" || receipt["newResourceConfirmed"] != false {
		t.Fatalf("source became an owned copy: %#v", receipt)
	}
	for _, ref := range []string{"https://alidocs.dingtalk.com/i/nodes/source", "http://alidocs.dingtalk.com/i/nodes/source"} {
		caller := &wikiCoverageCaller{}
		_, err := runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--node", ref, "--yes")
		if !errors.As(err, &typed) || typed.Reason != "stable_node_id_required" || len(caller.calls) != 0 {
			t.Fatalf("URL err=%#v calls=%#v", err, caller.calls)
		}
	}
	rejected := errors.New("API rejected opaque source fixture")
	caller = &wikiCoverageCaller{
		responses: map[string][]string{"doc/get_document_info": {`{"nodeId":"opaque/id?part","workspaceId":"source-w","name":"Source","extension":"adoc"}`}},
		errors:    map[string][]error{"doc/copy_document": {rejected}},
	}
	_, err = runWikiCoverageCLI(t, caller, "+node-copy", "--workspace", "w", "--node", "opaque/id?part", "--yes")
	if !errors.Is(err, rejected) || len(caller.calls) != 2 || caller.calls[1].args["nodeId"] != "opaque/id?part" {
		t.Fatalf("opaque ID validation moved out of API: err=%v calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageWikiWriteReadbackErrorIdentity(t *testing.T) {
	typedCause := apperrors.NewValidation("readback invalid", apperrors.WithReason("readback_parameter"), apperrors.WithRPCCode(40017), apperrors.WithDetails(map[string]any{"existing": "detail"})).(*apperrors.Error)
	raw := &apperrors.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	if got := wikiNodeWriteError(nil, "created", "", raw); got != raw {
		t.Fatal("host-owned raw error must remain untouched")
	}
	for _, cause := range []error{typedCause, &helpers.CLIError{Code: helpers.CodeAuthTokenExpired, Message: "expired"}} {
		caller := &wikiCoverageCaller{responses: map[string][]string{"doc/create_file": {`{"success":true,"nodeId":"created"}`}}, errors: map[string][]error{"doc/get_document_info": {cause}}}
		_, err := runWikiCoverageCLI(t, caller, "+node-create", "--workspace", "w", "--name", "Doc")
		if !errors.Is(err, cause) || apperrors.ExitCode(err) != apperrors.ExitCode(cause) || len(caller.calls) != 2 {
			t.Fatalf("error identity/exit changed: err=%#v cause=%#v calls=%#v", err, cause, caller.calls)
		}
		var typed *apperrors.Error
		if errors.As(err, &typed) {
			receipt, _ := typed.Details["writeReceipt"].(map[string]any)
			if typed == typedCause || typed.Category != typedCause.Category || typed.RPCCode != typedCause.RPCCode || typed.Details["existing"] != "detail" || receipt["returnedNodeId"] != "created" {
				t.Fatalf("typed category/receipt lost: %#v", typed)
			}
		} else if !strings.Contains(err.Error(), `"returnedNodeId":"created"`) {
			t.Fatalf("legacy error lost received ID: %v", err)
		}
	}
	if typedCause.ExecutionStarted != nil || len(typedCause.Details) != 1 {
		t.Fatalf("original typed cause mutated: %#v", typedCause)
	}
}
