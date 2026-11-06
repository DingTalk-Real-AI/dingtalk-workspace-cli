package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Sequence placement uses the same public flags as the legacy block route,
// including an explicitly supplied zero index. A write-shaped response without
// identities remains unknown even when the server reports success.
func TestCrossPlatformCoverageRolloutWriteSequencePlacementAndReceipt(t *testing.T) {
	t.Setenv("DINGTALK_AGENT", "Contract-Agent")
	for _, tc := range []struct {
		name string
		args []string
		tool string
		want map[string]any
	}{
		{
			"insert before reference at zero", []string{"block", "insert", "--node", "n", "--content-format", "markdown", "--content", "first\n\nsecond", "--ref-block", "anchor", "--where", "before", "--index", "0", "--suggestion"},
			"insert_document_block", map[string]any{"nodeId": "n", "format": "markdown", "markdown": "first\n\nsecond", "referenceBlockId": "anchor", "where": "before", "index": 0, "suggestion": true, "extraInfo": map[string]any{"agentName": "Contract-Agent"}},
		},
		{
			"replace one block with markdown sequence", []string{"block", "update", "--node", "n", "--block-id", "old", "--content-format", "markdown", "--content", "first\n\nsecond"},
			"update_document_block", map[string]any{"nodeId": "n", "blockId": "old", "format": "markdown", "markdown": "first\n\nsecond", "extraInfo": map[string]any{"agentName": "Contract-Agent"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, receipt := range []string{`{"newBlockIds":["new-first","new-second"]}`, `{"success":true,"blockId":"old"}`} {
				t.Run(receipt, func(t *testing.T) {
					caller := &docReadScopeCaller{text: receipt}
					err := executeDocSuggestionCommand(t, caller, tc.args...)
					if strings.Contains(receipt, "newBlockIds") {
						if err != nil {
							t.Fatal(err)
						}
					} else {
						var typed *apperrors.Error
						if !errors.As(err, &typed) || typed.Reason != "doc_block_sequence_receipt_unconfirmed" || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || typed.FailureStage != "response_validation" || typed.Details["nodeId"] != "n" || typed.Details["remoteEffect"] != "unknown" {
							t.Fatalf("unconfirmed identities must remain a post-write unknown: %#v", err)
						}
					}
					if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool || !reflect.DeepEqual(caller.calls[0].args, tc.want) {
						t.Fatalf("one atomic write with preserved placement required: %#v, want %s %#v", caller.calls, tc.tool, tc.want)
					}
				})
			}
		})
	}
}

// Force authorizes the guarded overwrite, not every subsequent append. Chunk
// order/content and Agent identity must survive rebuilding those append calls.
func TestCrossPlatformCoverageRolloutWriteForceChunkedOverwrite(t *testing.T) {
	agentOpenAPIViaDocProcess(t)
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "guarded", true: "forced"}[force], func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"nodeId":"n"}`}}}
			installScriptedCaller(t, caller)
			out, err := chunkedWrite(context.Background(), "update_document", map[string]any{
				"nodeId": "n", "mode": "overwrite", "force": force,
				"extraInfo": map[string]any{"agentName": "Frozen-Agent"},
			}, "aaa\n\nbbb\n\nccc", "doc update", 5)
			if err != nil || out.written != 3 || out.nodeID != "n" || caller.calls != 3 {
				t.Fatalf("chunk result=%#v calls=%d error=%v", out, caller.calls, err)
			}
			for i, content := range []string{"aaa", "bbb", "ccc"} {
				want := map[string]any{"nodeId": "n", "markdown": content, "mode": "append", "extraInfo": map[string]any{"agentName": "Frozen-Agent"}}
				if i == 0 {
					want["mode"], want["force"] = "overwrite", force
				}
				if caller.toolLog[i] != "update_document" || !reflect.DeepEqual(caller.argsLog[i], want) {
					t.Fatalf("chunk %d = %s %#v, want %#v", i, caller.toolLog[i], caller.argsLog[i], want)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageRolloutWriteOverwriteRefusalClassification(t *testing.T) {
	for _, code := range []string{"PENDING_SUGGESTIONS_EXIST", "PENDING_SUGGESTIONS_UNAVAILABLE"} {
		t.Run(code, func(t *testing.T) {
			body := map[string]any{"success": false, "errorCode": code, "errorMsg": "overwrite stopped"}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			caller := &docReadScopeCaller{text: string(raw)}
			installDocReadScopeCaller(t, caller)
			for _, classifyErr := range []error{
				ClassifyToolResultContent(body),
				callMCPToolOnServer("doc", "update_document", map[string]any{"nodeId": "n", "mode": "overwrite"}),
			} {
				var typed *apperrors.Error
				if !errors.As(classifyErr, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != code || typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.Retryable || !typed.RetryableSet || len(typed.Actions) == 0 {
					t.Fatalf("overwrite refusal lost pre-write/nonretryable facts: %#v", classifyErr)
				}
				if code == "PENDING_SUGGESTIONS_EXIST" && !strings.Contains(strings.Join(typed.Actions, " "), "--force") {
					t.Fatalf("pending suggestions lost explicit overwrite guidance: %#v", typed.Actions)
				}
			}
			if len(caller.calls) != 1 {
				t.Fatalf("refusal must not trigger retry/readback: %#v", caller.calls)
			}
		})
	}

	t.Run("gateway authentication still takes precedence", func(t *testing.T) {
		body := map[string]any{"success": false, "code": "USER_TOKEN_ILLEGAL", "errorCode": "PENDING_SUGGESTIONS_EXIST"}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		caller := &docReadScopeCaller{text: string(raw)}
		installDocReadScopeCaller(t, caller)
		for _, classifyErr := range []error{ClassifyToolResultContent(body), callMCPToolOnServer("doc", "update_document", map[string]any{"nodeId": "n"})} {
			var typed *CLIError
			if !errors.As(classifyErr, &typed) || typed.Code != CodeAuthTokenExpired {
				t.Fatalf("overwrite classifier replaced gateway authentication: %#v", classifyErr)
			}
		}
		if len(caller.calls) != 1 {
			t.Fatalf("authentication rejection retried: %#v", caller.calls)
		}
	})
}
