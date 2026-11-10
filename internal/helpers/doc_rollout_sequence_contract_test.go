package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// These inputs must fail before a write: mixing carriers must not silently
// discard content, and malformed sequences must retain the validation exit.
func TestCrossPlatformCoverageRolloutSequencePublicInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		topic string
	}{
		{"markdown with element", []string{"block", "insert", "--node", "n", "--content-format", "markdown", "--content", "body", "--element", `["p",{}]`}, "混用"},
		{"markdown with nodes", []string{"block", "insert", "--node", "n", "--content-format", "markdown", "--content", "body", "--jsonml-nodes", `[["p",{}]]`}, "混用"},
		{"whitespace-only markdown", []string{"block", "insert", "--node", "n", "--content-format", "markdown", "--content", " \n\t"}, "不能为空"},
		{"two JSONML carriers", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--element", `["p",{}]`, "--jsonml-nodes", `[["p",{}]]`}, "互斥"},
		{"malformed sequence", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", `[`}, "非空"},
		{"empty sequence", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", `[]`}, "非空"},
		{"object sequence", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", `{}`}, "非空"},
		{"invalid sequence node", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--jsonml-nodes", `[["p",{"jc":1}]]`}, "JSONML"},
		{"range requires sequence format", []string{"block", "update", "--node", "n", "--start-block-id", "a", "--end-block-id", "b", "--content-format", "element", "--element", `{"blockType":"paragraph"}`}, "markdown/jsonml"},
		{"range missing JSONML node", []string{"block", "update", "--node", "n", "--start-block-id", "a", "--end-block-id", "b", "--content-format", "jsonml"}, "--element"},
		{"range malformed JSONML node", []string{"block", "update", "--node", "n", "--start-block-id", "a", "--end-block-id", "b", "--content-format", "jsonml", "--element", `{`}, "JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &docReadScopeCaller{}
			err := executeDocSuggestionCommand(t, caller, tc.args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || apperrors.ExitCode(err) != 3 {
				t.Fatalf("error = %#v, want typed validation with exit code 3", err)
			}
			if !strings.Contains(err.Error(), tc.topic) || len(caller.calls) != 0 {
				t.Fatalf("error=%v calls=%#v; want input rejection before any RPC", err, caller.calls)
			}
		})
	}
}

// A range may replace its addressed blocks with one JSONML node. It still
// uses the atomic range carrier and requires the server's new identities.
func TestCrossPlatformCoverageRolloutSequenceRangeAcceptsOneElement(t *testing.T) {
	t.Setenv("DINGTALK_AGENT", "")
	input := `["p",{},["span",{"data-type":"text"},"replacement"]]`
	caller := &docReadScopeCaller{text: `{"newBlockIds":["new"]}`}
	if err := executeDocSuggestionCommand(t, caller, "block", "update", "--node", "n", "--start-block-id", "a", "--end-block-id", "b", "--content-format", "jsonml", "--element", input); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"nodeId": "n", "startBlockId": "a", "endBlockId": "b",
		"format": "jsonml", "jsonml": input,
		"extraInfo": map[string]any{"agentName": "unknown"},
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "update_document_block" || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls=%#v, want one atomic range write with %#v", caller.calls, want)
	}
}

// The decoded adapter also accepts values constructed in Go. It must return
// serialization failures without publishing a partial sequence to its caller.
func TestCrossPlatformCoverageRolloutSequenceDecodedSerializationFailure(t *testing.T) {
	cmd := &cobra.Command{Use: "sequence"}
	got, err := prepareDecodedDocJSONMLNodes(cmd, []any{func() {}})
	var unsupported *json.UnsupportedTypeError
	if got != "" || !errors.As(err, &unsupported) || unsupported.Type.Kind() != reflect.Func {
		t.Fatalf("result=%q error=%#v, want original unsupported-value error and no payload", got, err)
	}
}

func TestCrossPlatformCoverageRolloutSequenceUUIDGuardOnlyChecksBlockWrites(t *testing.T) {
	block := &cobra.Command{Use: "block"}
	list := &cobra.Command{Use: "list"}
	list.Flags().Bool("suggestion", true, "")
	block.AddCommand(list)
	input := `["p",{"uuid":"same"},["span",{"uuid":"same"},"body"]]`
	var node, want []any
	if err := json.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatal(err)
	}
	if err := prepareSuggestionNodeUUIDs(list, node, true); err != nil || !reflect.DeepEqual(node, want) {
		t.Fatalf("non-write UUID input was changed or rejected: %#v, %v", node, err)
	}
}

// Structural validation belongs to the JSONML pipeline. This identity walker
// must tolerate non-node arrays in a decoded tree without treating them as
// UUID-bearing nodes, mutating them, or panicking on an empty array.
func TestCrossPlatformCoverageRolloutSequenceUUIDWalkerPreservesNonNodes(t *testing.T) {
	input := `["p",{"uuid":"root"},[],[null,{"uuid":"root"}],[1,{"uuid":"root"}],["span",{"uuid":"child"},"body"]]`
	var node, want []any
	if err := json.Unmarshal([]byte(input), &node); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatal(err)
	}
	if err := prepareInputNodeUUIDs(node, false, ""); err != nil || !reflect.DeepEqual(node, want) {
		t.Fatalf("non-node arrays were changed or rejected: %#v, %v", node, err)
	}
}
