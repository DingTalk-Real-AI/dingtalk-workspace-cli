package doc

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestUpdateBlockDeleteRejectsInvalidBlockID(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", ",", "--yes")
	if err == nil {
		t.Fatal("comma-only --block-id must be rejected before the delete runs")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "block_id_empty" {
		t.Fatalf("must fail with block_id_empty, got %#v", err)
	}
}

func TestUpdateBlockDeleteBatchDeletesAndVerifies(t *testing.T) {
	remainingBlocks := func(ids ...string) map[string]any {
		blocks := make([]any, 0, len(ids))
		for _, id := range ids {
			blocks = append(blocks, map[string]any{
				"element": map[string]any{
					"id":        id,
					"paragraph": map[string]any{"text": id},
				},
			})
		}
		return map[string]any{"blocks": blocks}
	}

	t.Run("success", func(t *testing.T) {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{
			"list_document_blocks": {remainingBlocks()},
		}}
		if err := runDocCoverage(t, Update, caller,
			"--node", "n", "--command", "block_delete", "--block-id", "a,b", "--yes"); err != nil {
			t.Fatalf("batch block delete must succeed: %v", err)
		}
		var deleteCalls int
		for _, call := range caller.history {
			if call.tool != "delete_document_block" {
				continue
			}
			deleteCalls++
			if call.params["nodeId"] != "n" {
				t.Fatalf("unexpected nodeId: %v", call.params["nodeId"])
			}
			if call.params["blockId"] != "a,b" {
				t.Fatalf("unexpected blockId payload: %v", call.params["blockId"])
			}
		}
		if deleteCalls != 1 {
			t.Fatalf("expected exactly one delete_document_block call, got %d", deleteCalls)
		}
	})

	t.Run("verification failure when block remains", func(t *testing.T) {
		attempts := len(docVerifyDelays) + 1
		responses := make([]map[string]any, attempts)
		for i := range responses {
			responses[i] = remainingBlocks("a")
		}
		caller := &docCoverageCaller{responses: map[string][]map[string]any{
			"list_document_blocks": responses,
		}}
		err := runDocCoverage(t, Update, caller,
			"--node", "n", "--command", "block_delete", "--block-id", "a,b", "--yes")
		if err == nil {
			t.Fatal("command must fail when a target block remains after delete")
		}
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) || appErr.Reason != "doc_write_verification_failed" {
			t.Fatalf("must fail with doc_write_verification_failed, got %#v", err)
		}
		var deleteCalls int
		for _, call := range caller.history {
			if call.tool == "delete_document_block" {
				deleteCalls++
			}
		}
		if deleteCalls != 1 {
			t.Fatalf("expected exactly one delete_document_block call, got %d", deleteCalls)
		}
	})
}
