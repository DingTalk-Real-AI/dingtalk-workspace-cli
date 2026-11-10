package doc

import (
	"errors"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"os"
	"testing"
)

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditAtomicRange(t *testing.T) {
	for _, format := range []string{"markdown", "jsonml"} {
		content := "first\n\nsecond"
		if format == "jsonml" {
			content = `[["p",{},"first"],["p",{},"second"]]`
		}
		caller := &docCoverageCaller{responses: map[string][]map[string]any{
			"update_document_block": {{"newBlockIds": []any{"new-1", "new-2"}}},
		}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_replace", "--start-block-id", "a", "--end-block-id", "b", "--doc-format", format, "--content", content, "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(caller.history) != 1 {
			t.Fatalf("range must make one atomic server request: %#v", caller.history)
		}
		call := caller.history[0]
		if _, removed := call.params["jsonmlNodes"]; removed {
			t.Fatalf("removed external field sent: %#v", call.params)
		}
		if call.tool != "update_document_block" || call.params["startBlockId"] != "a" || call.params["endBlockId"] != "b" || call.params["blockId"] != nil {
			t.Fatalf("wrong range: %#v", call)
		}
		field := "markdown"
		if format == "jsonml" {
			field = "jsonml"
		}
		if call.params[field] != content {
			t.Fatalf("lost sequence: %#v", call.params)
		}
	}
}

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditForceDistinctFromYes(t *testing.T) {
	for _, force := range []bool{false, true} {
		args := []string{"--node", "n", "--command", "overwrite", "--content", "new", "--yes"}
		if force {
			args = append(args, "--force")
		}
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"get_document_content": {{"markdown": "new"}}, "update_document": {{"success": true, "nodeId": "n", "mode": "overwrite"}}}}
		if err := runDocCoverage(t, Update, caller, args...); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, call := range caller.history {
			if call.tool == "update_document" {
				found = true
				if call.params["force"] != force {
					t.Fatalf("yes must not imply force: %#v", call.params)
				}
			}
		}
		if !found {
			t.Fatal("missing write")
		}
	}
}

func TestCrossPlatformCoverageDiffblockOverwriteStandardSuccessContinuesToReadback(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{"update_document": {{"success": true}}, "get_document_content": {{"markdown": "new"}}}}
	err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "overwrite", "--content", "new", "--yes")
	if err != nil || len(caller.history) != 2 || caller.history[0].tool != "update_document" || caller.history[1].tool != "get_document_content" {
		t.Fatalf("standard success must verify once without another write: %v %#v", err, caller.history)
	}
}

func TestCrossPlatformCoverageDiffblockReviewSequenceReceiptAndPrettyInputs(t *testing.T) {
	for _, suggestion := range []bool{false, true} {
		for _, target := range [][]string{{"--block-id", "a"}, {"--start-block-id", "a", "--end-block-id", "b"}} {
			for _, ids := range []map[string]any{{}, {"success": true}, {"newBlockIds": []any{}}, {"newBlockIds": []any{"a", "a"}}, {"newBlockIds": []any{" "}}, {"newBlockIds": []any{"actual-1", "actual-2"}}} {
				caller := &docCoverageCaller{responses: map[string][]map[string]any{"update_document_block": {ids}}}
				args := append([]string{"--node", "n", "--command", "block_replace", "--content", "x", "--yes"}, target...)
				if suggestion {
					args = append(args, "--suggestion")
				}
				err := runDocCoverage(t, Update, caller, args...)
				valid := len(ids) == 1 && lenIDSequence(ids) == 2 && ids["newBlockIds"].([]any)[0] == "actual-1"
				if valid {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable {
						t.Fatalf("invalid receipt succeeded: %#v %v", ids, err)
					}
				}
				if caller.calls != 1 {
					t.Fatalf("must call once: %#v", caller.history)
				}
			}
			for _, content := range []string{`[["p",{},"one"],["p",{},"two"]]`, "[\n [\"p\",{},\"one\"],\n [\"p\",{},\"two\"]\n]"} {
				caller := &docCoverageCaller{responses: map[string][]map[string]any{"update_document_block": {{"newBlockIds": []any{"new1", "new2"}}}}}
				args := append([]string{"--node", "n", "--command", "block_replace", "--doc-format", "jsonml", "--content", content, "--yes"}, target...)
				if suggestion {
					args = append(args, "--suggestion")
				}
				if err := runDocCoverage(t, Update, caller, args...); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.history[0].params["jsonml"] != `[["p",{},"one"],["p",{},"two"]]` {
					t.Fatalf("pretty/compact must preserve sequence: %#v", caller.history)
				}
			}
		}
	}
}
func lenIDSequence(value map[string]any) int { ids, _ := value["newBlockIds"].([]any); return len(ids) }

func TestCrossPlatformCoverageDiffblockReviewPrettySequenceIsShapeBased(t *testing.T) {
	t.Chdir(t.TempDir())
	pretty := "[\n [\"p\",{},\"one\"],\n [\"p\",{},\"two\"]\n]"
	if err := os.WriteFile("nodes.json", []byte(pretty), 0600); err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range []bool{false, true} {
		for _, operation := range []struct {
			command string
			target  []string
		}{
			{"block_replace", []string{"--block-id", "a"}},
			{"block_replace", []string{"--start-block-id", "a", "--end-block-id", "b"}},
			{"block_insert_before", []string{"--before-block-id", "a"}},
			{"block_insert_after", []string{"--after-block-id", "a"}},
		} {
			for _, content := range []string{`[["p",{},"one"],["p",{},"two"]]`, pretty, "@nodes.json"} {
				tool := "update_document_block"
				if operation.command != "block_replace" {
					tool = "insert_document_block"
				}
				caller := &docCoverageCaller{responses: map[string][]map[string]any{tool: {{"newBlockIds": []any{"new1", "new2"}}}}}
				args := append([]string{"--node", "n", "--command", operation.command, "--doc-format", "jsonml", "--content", content, "--yes"}, operation.target...)
				if suggestion {
					args = append(args, "--suggestion")
				}
				if err := runDocCoverage(t, Update, caller, args...); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.history[0].params["jsonml"] != `[["p",{},"one"],["p",{},"two"]]` {
					t.Fatalf("pretty shape changed: %#v", caller.history)
				}
			}
		}
	}
	// A pretty single node remains a single input, wrapped only by the selected range route.
	for _, content := range []string{`["p",{},"one"]`, "[\n \"p\", {}, \"one\"\n]"} {
		caller := &docCoverageCaller{responses: map[string][]map[string]any{"update_document_block": {{"newBlockIds": []any{"new1"}}}}}
		if err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_replace", "--start-block-id", "a", "--end-block-id", "b", "--doc-format", "jsonml", "--content", content, "--yes"); err != nil {
			t.Fatal(err)
		}
		if caller.history[0].params["jsonml"] != `[["p",{},"one"]]` {
			t.Fatal(caller.history)
		}
	}
}

func TestCrossPlatformCoverageDiffblockReviewSequenceInsertReceiptIsUnknown(t *testing.T) {
	for _, format := range []string{"markdown", "jsonml"} {
		for _, suggestion := range []bool{false, true} {
			content := "x"
			if format == "jsonml" {
				content = `[["p",{},"x"]]`
			}
			caller := &docCoverageCaller{responses: map[string][]map[string]any{"insert_document_block": {{"success": true}}}}
			args := []string{"--node", "n", "--command", "block_insert_after", "--after-block-id", "a", "--doc-format", format, "--content", content, "--yes"}
			if suggestion {
				args = append(args, "--suggestion")
			}
			err := runDocCoverage(t, Update, caller, args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "doc_block_sequence_receipt_unconfirmed" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || caller.calls != 1 {
				t.Fatalf("missing insert ids became success: %v %#v", err, caller.history)
			}
		}
	}
}
