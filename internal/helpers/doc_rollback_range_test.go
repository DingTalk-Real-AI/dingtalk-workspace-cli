package helpers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditPublicCarriers(t *testing.T) {
	for _, target := range [][]string{{"--block-id", "old"}, {"--start-block-id", "a", "--end-block-id", "b"}} {
		for _, input := range [][]string{{"--content-format", "markdown", "--content", "one\n\ntwo"}, {"--content-format", "jsonml", "--jsonml-nodes", `[["p",{},"one"],["p",{},"two"]]`}} {
			caller := &docReadScopeCaller{text: `{"newBlockIds":["new-a","new-b"]}`}
			args := append([]string{"block", "update", "--node", "n"}, target...)
			args = append(args, input...)
			args = append(args, "--suggestion")
			if err := executeDocSuggestionCommand(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("atomic request count: %#v", caller.calls)
			}
		}
	}
	for _, args := range [][]string{{"block", "update", "--node", "n", "--start-block-id", "a", "--content", "x"}, {"block", "update", "--node", "n", "--start-block-id", "a", "--end-block-id", "b", "--block-id", "c", "--content", "x"}} {
		caller := &docReadScopeCaller{}
		if err := executeDocSuggestionCommand(t, caller, args...); err == nil || len(caller.calls) != 0 {
			t.Fatal("invalid target sent", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditPendingReasons(t *testing.T) {
	for _, code := range []string{"PENDING_SUGGESTIONS_EXIST", "PENDING_SUGGESTIONS_UNAVAILABLE"} {
		err := ClassifyMCPResponseText(`{"success":false,"errorCode":"` + code + `","errorMsg":"stopped"}`)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != code || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
			t.Fatalf("lost pending guard code: %#v", err)
		}
	}
}

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditSequenceUUIDRepair(t *testing.T) {
	input := `[["p",{"uuid":"same"},"one"],["p",{"uuid":"same"},"two"]]`
	caller := &docReadScopeCaller{text: `{"newBlockIds":["same","new"]}`}
	args := []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", input, "--suggestion"}
	if err := executeDocSuggestionCommand(t, caller, args...); err == nil || len(caller.calls) != 0 {
		t.Fatalf("duplicate not rejected: %v", err)
	}
	caller = &docReadScopeCaller{text: `{"newBlockIds":["same","new"]}`}
	if err := executeDocSuggestionCommand(t, caller, append(args, "--fix-jsonml")...); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditForceOnBothWholeFormats(t *testing.T) {
	for _, format := range []string{"markdown", "jsonml"} {
		for _, force := range []bool{false, true} {
			content := "new"
			if format == "jsonml" {
				content = `["root",{},["p",{},"new"]]`
			}
			args := []string{"update", "--node", "n", "--mode", "overwrite", "--content-format", format, "--content", content, "--yes"}
			if force {
				args = append(args, "--force")
			}
			caller := &docReadScopeCaller{text: `{"success":true,"nodeId":"n","mode":"overwrite"}`}
			if err := executeDocSuggestionCommand(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != "update_document" || caller.calls[0].args["force"] != force {
				t.Fatalf("force/yes carrier drift: %#v", caller.calls)
			}
		}
	}
}

func TestCrossPlatformCoverageDiffblockOverwriteStandardErrorsStillRejectFailedResponses(t *testing.T) {
	for _, code := range []string{"PENDING_SUGGESTIONS_EXIST", "PENDING_SUGGESTIONS_UNAVAILABLE", "VERSION_CONFLICT", "PERMISSION_DENIED"} {
		t.Run(code, func(t *testing.T) {
			caller := &docReadScopeCaller{text: `{"success":false,"errorCode":"` + code + `","errorMsg":"stopped"}`}
			err := executeDocSuggestionCommand(t, caller, "update", "--node", "n", "--mode", "overwrite", "--content", "new", "--yes", "--force")
			if err == nil || !strings.Contains(err.Error(), code) || len(caller.calls) != 1 {
				t.Fatalf("standard failure must remain a single failed request: %v %#v", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDiffblockReviewSequenceReceiptRequired(t *testing.T) {
	for _, text := range []string{"", `{}`, `{"success":true}`, `{"newBlockIds":[]}`, `{"newBlockIds":[""]}`, `{"newBlockIds":["a","a"]}`, `{"newBlockIds":[4]}`, `{"newBlockIds":"a"}`} {
		for _, suggestion := range []bool{false, true} {
			for _, operation := range []string{"insert", "update"} {
				for _, input := range [][]string{{"--content-format", "markdown", "--content", "x"}, {"--content-format", "jsonml", "--jsonml-nodes", `[["p",{},"x"]]`}} {
					args := append([]string{"block", operation, "--node", "n"}, input...)
					if operation == "update" {
						args = append(args, "--start-block-id", "a", "--end-block-id", "b")
					}
					if suggestion {
						args = append(args, "--suggestion")
					}
					caller := &docReadScopeCaller{text: text}
					err := executeDocSuggestionCommand(t, caller, args...)
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "doc_block_sequence_receipt_unconfirmed" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || len(caller.calls) != 1 {
						t.Fatalf("receipt %q must be post-write unknown, no retry: %v %#v", text, err, caller.calls)
					}
				}
			}
		}
	}
}

func TestCrossPlatformCoverageDiffblockReviewSequenceReceiptAndLegacyCompatibility(t *testing.T) {
	for _, text := range []string{`{"newBlockIds":["z","a"]}`, `{"success":true,"result":{"newBlockIds":["z","a"]}}`} {
		for _, suggestion := range []bool{false, true} {
			caller := &docReadScopeCaller{text: text}
			args := []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", `[["p",{},"x"]]`}
			if suggestion {
				args = append(args, "--suggestion")
			}
			if err := executeDocSuggestionCommand(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatal(caller.calls)
			}
		}
	}
	// The old single-node carrier does not acquire the new receipt requirement.
	caller := &docReadScopeCaller{text: `{"blockId":"legacy"}`}
	if err := executeDocSuggestionCommand(t, caller, "block", "insert", "--node", "n", "--content-format", "jsonml", "--element", `["p",{},"x"]`); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDiffblockUnifiedJSONMLSequenceWire(t *testing.T) {
	for _, operation := range []string{"insert", "update"} {
		for _, content := range []string{`[["p",{},["span",{"data-type":"text"},"one"]]]`, `[["p",{},["span",{"data-type":"text"},"one"]],["p",{},["span",{"data-type":"text"},"two"]]]`} {
			for _, suggestion := range []bool{false, true} {
				caller := &docReadScopeCaller{text: `{"newBlockIds":["new-a","new-b"]}`}
				args := []string{"block", operation, "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", content}
				if operation == "update" {
					args = append(args, "--block-id", "old")
				}
				if suggestion {
					args = append(args, "--suggestion")
				}
				if err := executeDocSuggestionCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 1 {
					t.Fatalf("want one atomic request, got %#v", caller.calls)
				}
				payload := caller.calls[0].args
				if _, removed := payload["jsonmlNodes"]; removed {
					t.Fatalf("removed external field sent: %#v", payload)
				}
				var actual, expected []any
				wire, ok := payload["jsonml"].(string)
				if !ok || json.Unmarshal([]byte(wire), &actual) != nil || json.Unmarshal([]byte(content), &expected) != nil || len(actual) != len(expected) {
					t.Fatalf("sequence must remain JSON string under jsonml: %#v", payload)
				}
				if payload["format"] != "jsonml" || suggestion && payload["suggestion"] != true {
					t.Fatalf("format/suggestion lost: %#v", payload)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageDiffblockUnifiedJSONMLSequenceStillRequiresReceipt(t *testing.T) {
	for _, content := range []string{`[["p",{},["span",{"data-type":"text"},"one"]]]`, `[["p",{},["span",{"data-type":"text"},"one"]],["p",{},["span",{"data-type":"text"},"two"]]]`} {
		caller := &docReadScopeCaller{text: `{"blockId":"legacy-only"}`}
		err := executeDocSuggestionCommand(t, caller, "block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", content)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "doc_block_sequence_receipt_unconfirmed" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || len(caller.calls) != 1 {
			t.Fatalf("len1/multi sequence must fail closed after one call: %v %#v", err, caller.calls)
		}
	}
}
