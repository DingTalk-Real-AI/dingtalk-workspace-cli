package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/google/uuid"
)

// Duplicate input identities must fail before the single write RPC. UPDATE's
// addressed root is reserved too: the producer will assign --block-id to it.
func TestCrossPlatformCoverageDocSuggestionJSONMLUUIDRejectsDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name, action, input string
	}{
		{"insert nested duplicate", "insert", `["container",{"uuid":"same","subType":"normal"},["p",{"uuid":"same"},"body"]]`},
		{"insert sibling duplicate", "insert", `["container",{"uuid":"root","subType":"normal"},["p",{"uuid":"same"}],["p",{"uuid":"same"}]]`},
		{"update root duplicate", "update", `["p",{"uuid":"target"},["span",{"uuid":"target"},"body"]]`},
		{"update supplied root duplicate", "update", `["p",{"uuid":"supplied"},["span",{"uuid":"supplied"},"body"]]`},
		{"update effective root alias", "update", `["p",{"uuid":"supplied"},["span",{"uuid":"target"},"body"]]`},
		{"update absent root alias", "update", `["p",{},["span",{"uuid":"target"},"body"]]`},
	} {
		for _, selector := range []string{"--suggestion", "--suggestion=true"} {
			t.Run(tc.name+selector, func(t *testing.T) {
				caller := &docReadScopeCaller{text: `{"applied":false,"suggestionId":"pending"}`}
				args := []string{"block", tc.action, "--node", "n", "--content-format", "jsonml", "--element", tc.input, selector}
				if tc.action == "update" {
					args = append(args, "--block-id", "target")
				}
				err := executeDocSuggestionCommand(t, caller, args...)
				var classified *apperrors.Error
				if !errors.As(err, &classified) || classified.Category != apperrors.CategoryValidation || apperrors.ExitCode(err) != 3 {
					t.Fatalf("error = %#v, want typed validation with exit code 3", err)
				}
				if !strings.Contains(err.Error(), "UUID") || !strings.Contains(err.Error(), "--fix-jsonml") || len(caller.calls) != 0 {
					t.Fatalf("error=%v calls=%#v, want actionable rejection before any RPC", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionJSONMLUUIDRepairsOnlyNodeAttrs(t *testing.T) {
	input := `["container",{"uuid":"same","subType":"normal","metadata":{"uuid":"same","node":["p",{"uuid":"same"}]}},["p",{"uuid":"same","list":{"listId":"same"}},["span",{"uuid":"same"},"body"]],["p",{"uuid":"unique"},"tail"]]`
	caller := &docReadScopeCaller{text: `{"applied":false,"suggestionId":"pending"}`}
	stdout := installDocReadScopeCaller(t, caller)
	root := newDocCommand()
	root.SetArgs([]string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--element", input, "--suggestion", "--fix-jsonml"})
	var runErr error
	stderr := captureDocFragmentStderr(t, func() { runErr = corecmd.ExecuteForTest(root) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	node := suggestionUUIDRequestNode(t, caller, "insert_document_block")
	attrs := node[1].(map[string]any)
	child := node[2].([]any)
	span := child[2].([]any)
	childID := child[1].(map[string]any)["uuid"].(string)
	spanID := span[1].(map[string]any)["uuid"].(string)
	for _, id := range []string{childID, spanID} {
		if _, err := uuid.Parse(id); err != nil || id == "same" || id == "unique" {
			t.Fatalf("repaired UUID = %q, want a fresh UUID", id)
		}
	}
	if childID == spanID {
		t.Fatalf("repaired nodes still alias: %q", childID)
	}
	// Replacing the two generated identities restores the exact input. UUIDs in
	// metadata, node-looking metadata arrays and ordinary attributes stay intact.
	child[1].(map[string]any)["uuid"] = "same"
	span[1].(map[string]any)["uuid"] = "same"
	var want []any
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatal(err)
	}
	if attrs["uuid"] != "same" || !reflect.DeepEqual(node, want) {
		t.Fatalf("repair changed nonduplicate content: %#v", node)
	}
	if !strings.Contains(stderr, "[WARN]") || !strings.Contains(stderr, "UUID") || !strings.Contains(stderr, "2") {
		t.Fatalf("repair warning missing: %q", stderr)
	}
	if !json.Valid(stdout.Bytes()) || !strings.Contains(stdout.String(), "pending") || strings.Contains(stdout.String(), "[WARN]") {
		t.Fatalf("machine receipt contaminated or lost: %q", stdout.String())
	}
}

func TestCrossPlatformCoverageDocSuggestionJSONMLUUIDUpdatePreservesAddressedRoot(t *testing.T) {
	for _, rootAttrs := range []string{`{"uuid":"target"}`, `{"uuid":"supplied"}`, `{}`} {
		t.Run(rootAttrs, func(t *testing.T) {
			input := `["p",` + rootAttrs + `,["span",{"uuid":"target"},"body"],["span",{"uuid":"other"},"tail"]]`
			caller := &docReadScopeCaller{text: `{"applied":false,"suggestionId":"pending"}`}
			var runErr error
			stderr := captureDocFragmentStderr(t, func() {
				runErr = executeDocSuggestionCommand(t, caller, "block", "update", "--node", "n", "--block-id", "target", "--content-format", "jsonml", "--element", input, "--suggestion", "--fix-jsonml")
			})
			if runErr != nil {
				t.Fatal(runErr)
			}
			node := suggestionUUIDRequestNode(t, caller, "update_document_block")
			var wantAttrs map[string]any
			if err := json.Unmarshal([]byte(rootAttrs), &wantAttrs); err != nil {
				t.Fatal(err)
			}
			childID := node[2].([]any)[1].(map[string]any)["uuid"].(string)
			if _, err := uuid.Parse(childID); err != nil || childID == "target" || !reflect.DeepEqual(node[1], wantAttrs) || caller.calls[0].args["blockId"] != "target" {
				t.Fatalf("UPDATE lost addressed identity or kept child alias: %#v", caller.calls[0].args)
			}
			if node[3].([]any)[1].(map[string]any)["uuid"] != "other" || !strings.Contains(stderr, "[WARN]") {
				t.Fatalf("unique child changed or no repair warning: %#v / %q", node, stderr)
			}
		})
	}
}

func TestCrossPlatformCoverageDocSuggestionJSONMLUUIDGuardPreservesLegacyAndUniqueInputs(t *testing.T) {
	duplicate := `["p",{"uuid":"same"},["span",{"uuid":"same"},"body"]]`
	unique := `["p",{"uuid":"root","metadata":{"uuid":"root"}},["span",{"uuid":"child"},"body"],["span",{},"tail"]]`
	for _, tc := range []struct {
		name, input string
		flags       []string
	}{
		{"ordinary strict", duplicate, nil},
		{"ordinary fix", duplicate, []string{"--fix-jsonml"}},
		{"suggestion false", duplicate, []string{"--suggestion=false", "--fix-jsonml"}},
		{"suggestion false strict", duplicate, []string{"--suggestion=false"}},
		{"unique suggestion", unique, []string{"--suggestion"}},
		{"unique suggestion fix", unique, []string{"--suggestion", "--fix-jsonml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docReadScopeCaller{text: `{"applied":false,"suggestionId":"pending"}`}
			args := []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--element", tc.input}
			if err := executeDocSuggestionCommand(t, caller, append(args, tc.flags...)...); err != nil {
				t.Fatal(err)
			}
			node := suggestionUUIDRequestNode(t, caller, "insert_document_block")
			var want []any
			if err := json.Unmarshal([]byte(tc.input), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(node, want) {
				t.Fatalf("input changed: %#v, want %#v", node, want)
			}
		})
	}
}

func suggestionUUIDRequestNode(t *testing.T, caller *docReadScopeCaller, tool string) []any {
	t.Helper()
	if len(caller.calls) != 1 || caller.calls[0].tool != tool {
		t.Fatalf("calls = %#v, want one %s RPC without hidden reads", caller.calls, tool)
	}
	var node []any
	if err := json.Unmarshal([]byte(caller.calls[0].args["jsonml"].(string)), &node); err != nil {
		t.Fatal(err)
	}
	return node
}
