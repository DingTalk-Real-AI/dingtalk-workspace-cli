package helpers

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func executeDocSuggestionCommand(t *testing.T, caller *docReadScopeCaller, args ...string) error {
	t.Helper()
	installDocReadScopeCaller(t, caller)
	root := newDocCommand()
	root.PersistentFlags().Bool("yes", false, "")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return corecmd.ExecuteForTest(root)
}

func TestCrossPlatformCoverageDocSuggestionRejectsRemovedAlias(t *testing.T) {
	for _, args := range [][]string{
		{"read", "--node", "n"},
		{"read", "--node", "n", "--content-format", "jsonml"},
		{"block", "insert", "--node", "n", "--content", "body"},
		{"block", "update", "--node", "n", "--block-id", "b", "--content", "new"},
		{"block", "delete", "--node", "n", "--block-id", "b", "--yes"},
	} {
		for _, flag := range []string{"--sg", "--sg=false"} {
			t.Run(strings.Join(args, " ")+flag, func(t *testing.T) {
				caller := &docReadScopeCaller{text: `{"applied":false,"suggestionId":"pending"}`}
				argv := append(append([]string{}, args...), flag)
				err := executeDocSuggestionCommand(t, caller, argv...)
				var classified *apperrors.Error
				if !errors.As(err, &classified) || classified.Category != apperrors.CategoryValidation || apperrors.ExitCode(err) != 3 {
					t.Errorf("error = %#v, want typed validation with exit code 3", err)
				}
				if err == nil || !strings.Contains(err.Error(), "unknown flag") || len(caller.calls) != 0 {
					t.Fatalf("error=%v calls=%#v, want unknown flag before any RPC", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionHelpHidesRolloutSelector(t *testing.T) {
	for _, path := range [][]string{{"read"}, {"block", "insert"}, {"block", "update"}, {"block", "delete"}} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			caller := &docReadScopeCaller{}
			installDocReadScopeCaller(t, caller)
			root := newDocCommand()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs(append(append([]string{}, path...), "--help"))
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "--suggestion") || strings.Contains(output.String(), "--sg") || strings.Contains(output.String(), "建议模式") || len(caller.calls) != 0 {
				t.Fatalf("help must hide rollout selector without RPC: calls=%#v output=%s", caller.calls, output.String())
			}
		})
	}
}

// These tests catch a missing selector, a changed legacy payload, or an extra
// preview/readback call while exercising the real command and content builders.
func TestCrossPlatformCoverageDocSuggestionRequests(t *testing.T) {
	t.Setenv("DINGTALK_AGENT", "")
	for _, tc := range []struct {
		name string
		args []string
		tool string
		want map[string]any
	}{
		{"read markdown", []string{"read", "--node", "n"}, "get_document_content", map[string]any{"nodeId": "n"}},
		{"read jsonml", []string{"read", "--node", "n", "--content-format", "jsonml"}, "get_document_content", map[string]any{"nodeId": "n", "format": "jsonml"}},
		{"read scope", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "outline", "--password", "pw", "--version", "0"}, "get_document_content", map[string]any{"nodeId": "n", "format": "jsonml", "scope": "outline", "password": "pw", "historyVersion": 0}},
		{"insert element", []string{"block", "insert", "--node", "n", "--content", "body", "--ref-block", "b", "--where", "before"}, "insert_document_block", map[string]any{"nodeId": "n", "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": "body"}}, "referenceBlockId": "b", "where": "before"}},
		{"insert jsonml", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--element", `["p",{},"body"]`, "--index", "0"}, "insert_document_block", map[string]any{"nodeId": "n", "jsonml": `["p",{},"body"]`, "format": "jsonml", "index": 0}},
		{"update element", []string{"block", "update", "--node", "n", "--block-id", "b", "--element", `{"blockType":"paragraph","paragraph":{"text":"new"}}`}, "update_document_block", map[string]any{"nodeId": "n", "blockId": "b", "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": "new"}}}},
		{"update jsonml", []string{"block", "update", "--node", "n", "--block-id", "b", "--content-format", "jsonml", "--element", `["p",{},"new"]`}, "update_document_block", map[string]any{"nodeId": "n", "blockId": "b", "jsonml": `["p",{},"new"]`, "format": "jsonml"}},
		{"delete multiple", []string{"block", "delete", "--node", "n", "--block-id", " a,b,a ", "--yes"}, "delete_document_block", map[string]any{"nodeId": "n", "blockId": "a,b"}},
	} {
		for _, flag := range []string{"", "--suggestion", "--suggestion=true", "--suggestion=false"} {
			t.Run(tc.name+flag, func(t *testing.T) {
				caller := &docReadScopeCaller{text: `{"jsonml":"[\"fragment\",{},[\"p\",{},\"preview\"]]","markdown":"preview","suggestionId":"pending-1"}`}
				argv := append([]string{}, tc.args...)
				if flag != "" {
					argv = append(argv, flag)
				}
				if err := executeDocSuggestionCommand(t, caller, argv...); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 1 {
					t.Fatalf("calls = %#v, want exactly one RPC", caller.calls)
				}
				call := caller.calls[0]
				want := map[string]any{}
				for key, value := range tc.want {
					want[key] = value
				}
				if tc.tool != "get_document_content" {
					want["extraInfo"] = map[string]any{"agentName": "unknown"}
				}
				if flag == "--suggestion" || flag == "--suggestion=true" {
					want["suggestion"] = true
				}
				if call.tool != tc.tool || !reflect.DeepEqual(call.args, want) {
					t.Fatalf("request = %s %#v, want %s %#v", call.tool, call.args, tc.tool, want)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionExcludedWrites(t *testing.T) {
	for _, path := range [][]string{{"create"}, {"update"}, {"update", "overwrite"}, {"block", "list"}} {
		for _, flag := range []string{"--suggestion", "--sg"} {
			t.Run(strings.Join(path, " ")+flag, func(t *testing.T) {
				caller := &docReadScopeCaller{}
				err := executeDocSuggestionCommand(t, caller, append(append([]string{}, path...), flag)...)
				if err == nil || !strings.Contains(err.Error(), "unknown flag") {
					t.Fatalf("error = %v, want unknown flag", err)
				}
				if len(caller.calls) != 0 {
					t.Fatalf("excluded command called RPC: %#v", caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocSuggestionPendingReceipt(t *testing.T) {
	caller := &docReadScopeCaller{text: `{"suggestionId":"pending-1","applied":false}`}
	output := installDocReadScopeCaller(t, caller)
	root := newDocCommand()
	root.SetArgs([]string{"block", "update", "--node", "n", "--block-id", "b", "--content", "new", "--suggestion"})
	if err := corecmd.ExecuteForTest(root); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || !strings.Contains(output.String(), "pending-1") || strings.Contains(output.String(), `"verified": true`) {
		t.Fatalf("pending receipt must survive without body readback: calls=%#v output=%s", caller.calls, output.String())
	}
}

func TestCrossPlatformCoverageDocSuggestionErrorDoesNotRetryOrdinaryWrite(t *testing.T) {
	caller := &docReadScopeCaller{err: errors.New("suggestion branch rejected")}
	err := executeDocSuggestionCommand(t, caller, "block", "insert", "--node", "n", "--content", "body", "--suggestion")
	if err == nil || !strings.Contains(err.Error(), "suggestion branch rejected") || len(caller.calls) != 1 {
		t.Fatalf("error=%v calls=%#v; want branch error and one RPC", err, caller.calls)
	}
}
