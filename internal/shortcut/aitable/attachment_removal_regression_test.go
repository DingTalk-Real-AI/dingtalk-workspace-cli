// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAttachmentRemovalUsesResourceIDs(t *testing.T) {
	for _, scenario := range []string{"success", "same count wrong remainder", "removed ID renamed", "input rejection", "dry run", "no match"} {
		t.Run(scenario, func(t *testing.T) {
			before := []any{
				map[string]any{"resourceId": "remove", "filename": "remove.pdf"},
				map[string]any{"resourceId": "keep", "filename": "keep.pdf", "url": "https://temporary"},
			}
			after := []any{map[string]any{"resourceId": "keep", "filename": "keep.pdf", "url": "https://refreshed"}}
			write := `{"status":"success","data":{"removedCount":1}}`
			switch scenario {
			case "same count wrong remainder":
				after = []any{map[string]any{"resourceId": "other", "filename": "keep.pdf"}}
			case "removed ID renamed":
				after = []any{map[string]any{"resourceId": "remove", "filename": "renamed.pdf"}}
			case "input rejection":
				write = `{"status":"error","error":{"type":"INPUT_ERROR","code":"BAD_FIELD","retryable":false}}`
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: attachmentRecordJSON(t, "field", before)},
				{text: write},
				{text: attachmentRecordJSON(t, "field", after)},
			}}
			name := "remove.pdf"
			if scenario == "no match" {
				name = "missing.pdf"
			}
			args := []string{"--base-id", "base", "--table-id", "table", "--record-id", "record", "--field-id", "field", "--remove-name", name, "--yes"}
			if scenario == "dry run" {
				caller.dryRun = true
				args = append(args, "--dry-run")
			}
			out, err := runAITableCompositeCLI(t, caller, "+attachment-remove", args...)
			if scenario == "dry run" || scenario == "no match" {
				if err != nil || len(caller.calls) != 1 {
					t.Fatalf("unexpected write: %s %v %v", out, err, caller.calls)
				}
				return
			}
			if len(caller.calls) < 2 || caller.calls[1].tool != "remove_attachments" || !reflect.DeepEqual(caller.calls[1].args, map[string]any{
				"baseId": "base", "tableId": "table", "recordId": "record", "fieldId": "field", "resourceIds": []string{"remove"},
			}) {
				t.Fatalf("wrong removal request: %#v", caller.calls)
			}
			if scenario == "success" {
				if err != nil || !strings.Contains(out, `"status": "verified"`) || len(caller.calls) != 3 {
					t.Fatalf("failed: %s %v", out, err)
				}
			} else if err == nil {
				t.Fatalf("must not claim success: %s", out)
			}
			if scenario == "input rejection" && len(caller.calls) != 2 {
				t.Fatalf("input rejection must not be recovered by read-back: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAttachmentRemovalRejectsAmbiguousIdentity(t *testing.T) {
	_, err := planAttachmentRemoval([]map[string]any{
		{"resourceId": "shared", "filename": "remove.pdf"},
		{"resourceId": "shared", "filename": "keep.pdf"},
	}, "remove.pdf", false)
	if err == nil {
		t.Fatal("must not delete a retained attachment sharing the same resource ID")
	}
}

func TestCrossPlatformCoverageAttachmentRemovalRequiresRetainedIdentity(t *testing.T) {
	plan := attachmentRemovalPlan{remaining: []map[string]any{{"fileToken": "keep-token", "filename": "keep.pdf"}}}
	for _, actual := range [][]map[string]any{
		{{"fileToken": "other-token", "filename": "keep.pdf"}},
		{{"filename": "keep.pdf"}},
	} {
		if err := verifyAttachmentRemoval(actual, plan, "remove.pdf"); err == nil {
			t.Fatalf("count/name alone must not prove retained identity: %#v", actual)
		}
	}
	if err := verifyAttachmentRemoval([]map[string]any{{"fileToken": "keep-token", "filename": "keep.pdf"}}, plan, "remove.pdf"); err != nil {
		t.Fatal(err)
	}
}
