// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Invalid optional-mode inputs must fail before confirmation or a remote write.
func TestCrossPlatformCoverageRolloutShortcutInputGuardsBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "force cannot turn append into overwrite",
			args: []string{"--command", "append", "--content", "body", "--force"},
			want: []string{"--force", "overwrite"},
		},
		{
			name: "JSONML errors use shortcut flag names",
			args: []string{"--command", "block_insert_after", "--after-block-id", "anchor", "--doc-format", "jsonml", "--content", `{}`},
			want: []string{"--doc-format", "--content", "JSON"},
		},
		{
			name: "suggested sequence rejects duplicate identities",
			args: []string{"--command", "block_replace", "--block-id", "old", "--doc-format", "jsonml", "--content", `[["p",{"uuid":"same"},["span",{},"one"]],["p",{"uuid":"same"},["span",{},"two"]]]`, "--suggestion"},
			want: []string{"UUID", "same"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
			args := append([]string{"--node", "n", "--yes"}, tc.args...)
			err := runDocCoverage(t, Update, caller, args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 || caller.calls != 0 {
				t.Fatalf("invalid input must stop before RPC: err=%v calls=%#v", err, caller.history)
			}
			for _, fragment := range tc.want {
				if !strings.Contains(typed.Message, fragment) {
					t.Errorf("validation diagnostic %q lacks %q", typed.Message, fragment)
				}
			}
			if strings.Contains(typed.Message, "--content-format") || strings.Contains(typed.Message, "--element") {
				t.Errorf("helper-only flag names escaped shortcut validation: %q", typed.Message)
			}
		})
	}
}

// A single JSONML node in suggestion mode uses the sequence carrier and keeps
// the actual server identity, instead of claiming an ordinary readback proved it.
func TestCrossPlatformCoverageRolloutShortcutSuggestionSingleJSONMLReceipt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		target  []string
		tool    string
		where   string
	}{
		{"insert after anchor", "block_insert_after", []string{"--after-block-id", "anchor"}, "insert_document_block", "after"},
		{"replace target", "block_replace", []string{"--block-id", "old"}, "update_document_block", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				tc.tool: {{"newBlockIds": []any{"actual-server-id"}, "marker": "pending-receipt"}},
			}}
			args := append([]string{"--node", "n", "--command", tc.command, "--doc-format", "jsonml", "--content", `["p",{"uuid":"input-id"},["span",{},"proposed"]]`, "--suggestion", "--yes"}, tc.target...)
			envelope := runDocCoverageEnvelope(t, Update, caller, args...)
			if caller.calls != 1 || caller.history[0].tool != tc.tool {
				t.Fatalf("suggestion must issue one write without readback: %#v", caller.history)
			}
			params := caller.history[0].params
			if params["nodeId"] != "n" || params["suggestion"] != true || params["format"] != "jsonml" || params["jsonml"] != `[["p",{"uuid":"input-id"},["span",{},"proposed"]]]` || params["element"] != nil {
				t.Fatalf("single-node sequence payload drifted: %#v", params)
			}
			if tc.where == "after" {
				if params["referenceBlockId"] != "anchor" || params["where"] != "after" || params["blockId"] != nil {
					t.Errorf("insert target changed: %#v", params)
				}
			} else if params["blockId"] != "old" || params["referenceBlockId"] != nil {
				t.Errorf("replace target changed: %#v", params)
			}
			data, _ := envelope["data"].(map[string]any)
			if data["marker"] != "pending-receipt" || !reflect.DeepEqual(data["newBlockIds"], []any{"actual-server-id"}) {
				t.Fatalf("server sequence receipt was lost: %#v", envelope)
			}
		})
	}
}

func TestCrossPlatformCoverageRolloutShortcutSuggestionHeadingWriteContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		target  string
		where   string
		fail    bool
	}{
		{"before success", "block_insert_before", "--before-block-id", "before", false},
		{"after success", "block_insert_after", "--after-block-id", "after", false},
		{"failure remains unknown without replay", "block_insert_after", "--after-block-id", "after", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"insert_document_block": {{"success": true, "blockId": "pending-heading", "marker": "heading-receipt"}},
			}}
			if tc.fail {
				caller.failAt = 1
			}
			args := []string{"--node", "n", "--command", tc.command, tc.target, "anchor", "--content", "Review title", "--heading-level", "2", "--suggestion", "--yes"}
			if tc.fail {
				err := runDocCoverage(t, Update, caller, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "doc_write_commit_unknown" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable || typed.Details["status"] != "unknown" {
					t.Fatalf("failed heading write lost unknown/no-retry state: %v", err)
				}
			} else {
				envelope := runDocCoverageEnvelope(t, Update, caller, args...)
				data, _ := envelope["data"].(map[string]any)
				if data["marker"] != "heading-receipt" || data["blockId"] != "pending-heading" {
					t.Fatalf("ordinary heading receipt rejected or replaced: %#v", envelope)
				}
			}
			if caller.calls != 1 || caller.history[0].tool != "insert_document_block" {
				t.Fatalf("heading suggestion must not read back or replay: %#v", caller.history)
			}
			params := caller.history[0].params
			wantElement := map[string]any{"blockType": "heading", "heading": map[string]any{"text": "Review title", "level": "2"}}
			if params["suggestion"] != true || params["nodeId"] != "n" || params["referenceBlockId"] != "anchor" || params["where"] != tc.where || !reflect.DeepEqual(params["element"], wantElement) || params["markdown"] != nil || params["jsonml"] != nil {
				t.Fatalf("heading intent or wire level changed: %#v", params)
			}
		})
	}
}

// Pending-suggestion guards are a definite server refusal before overwrite,
// unlike ambiguous transport failures after a write request.
func TestCrossPlatformCoverageRolloutShortcutPendingOverwriteStopsBeforeReadback(t *testing.T) {
	for _, tc := range []struct {
		code   string
		action string
	}{
		{"PENDING_SUGGESTIONS_EXIST", "--force"},
		{"PENDING_SUGGESTIONS_UNAVAILABLE", "覆盖尚未开始"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			caller := &docCoverageCaller{responses: map[string][]map[string]any{
				"update_document": {{"success": false, "errorCode": tc.code, "errorMsg": "overwrite stopped by pending guard"}},
			}}
			err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", "replacement", "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != tc.code || typed.ExecutionStarted == nil || *typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable || typed.Details["status"] != "failed" || typed.Details["state"] != "failed" {
				t.Fatalf("pending guard must remain definite/no-retry refusal: %v", err)
			}
			if caller.calls != 1 || caller.history[0].tool != "update_document" || caller.history[0].params["force"] != false {
				t.Fatalf("--yes must not force, retry or read back after refusal: %#v", caller.history)
			}
			if !strings.Contains(strings.Join(typed.Actions, " "), tc.action) {
				t.Errorf("pending guard lost recovery guidance: %#v", typed.Actions)
			}
		})
	}
}
