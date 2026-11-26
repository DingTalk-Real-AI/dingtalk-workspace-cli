// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func classifyAitableRecordTextForTest(serverID, toolName, raw string) error {
	_, err := parseMCPToolTextResult(serverID, toolName, &edition.ToolResult{
		Content: []edition.ContentBlock{{Type: "text", Text: raw}},
	}, nil)
	return err
}

func TestCrossPlatformCoverageAitableRecordRecoveryRequiresProductAndWriteContext(t *testing.T) {
	for _, code := range []string{"DUPLICATE_CLIENT_TOKEN", "UPDATE_RECORDS_RESULT_UNKNOWN", "CREATE_RECORDS_OUTCOME_UNKNOWN", "ATTACHMENT_PREPARATION_UNAVAILABLE", "ATTACHMENT_WRITE_UNCONFIRMED"} {
		body := map[string]any{"status": "error", "error": map[string]any{
			"code": code, "message": "original product diagnostic", "retryable": true,
			"details": map[string]any{"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true,
				"clientToken": "original-token", "reconcileTool": "get_record_write_result", "attachmentVerification": "unconfirmed"},
		}}
		wire, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		assertLegacy := func(t *testing.T, err error) {
			t.Helper()
			var legacy *CLIError
			var projected *apperrors.Error
			if !errors.As(err, &legacy) || errors.As(err, &projected) || legacy.Code != CodeMCPToolError ||
				legacy.ServerCode != code || !strings.Contains(legacy.Message, "original product diagnostic") {
				t.Fatalf("unrelated error acquired AI Table recovery semantics: %v", err)
			}
		}
		t.Run(code+"/no-context", func(t *testing.T) {
			assertLegacy(t, ClassifyMCPResponseText(string(wire)))
			if err := classifyAitableMutationError(body); err != nil {
				t.Fatal("context-free mutation classifier projected a record error")
			}
		})
		for _, server := range []string{"aitable", "aitable-helper", "drive", "doc", "chat", "aitable-other", ""} {
			for _, tool := range []string{"create_records", "update_records", "record_upsert", "query_records", "record_upsert_extra"} {
				allowed := (server == "aitable" || server == "aitable-helper") &&
					(tool == "create_records" || tool == "update_records" || tool == "record_upsert")
				for _, path := range []string{"text", "output"} {
					// Empty product tests exercise the context-free parser directly;
					// the output caller instead resolves an empty product from argv.
					if path == "output" && server == "" {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/%s/%s", code, server, tool, path), func(t *testing.T) {
						var err error
						if path == "text" {
							err = classifyAitableRecordTextForTest(server, tool, string(wire))
						} else {
							caller := &aitableAppCaller{response: string(wire)}
							InitDepsForTest(t, caller)
							err = callMCPToolInternalOptsContext(context.Background(), server, tool, nil, false)
							if len(caller.calls) != 1 {
								t.Fatal("classification replayed the original request")
							}
						}
						if !allowed {
							assertLegacy(t, err)
							if ClassifyAitableRecordMutationError(server, tool, body) != nil {
								t.Fatal("record classifier accepted an unrelated context")
							}
							return
						}
						var projected *apperrors.Error
						if !errors.As(err, &projected) || projected.Reason != code || projected.ExecutionStarted == nil ||
							!*projected.ExecutionStarted || !projected.RetryableSet || projected.Retryable {
							t.Fatalf("allowed record write lost safe recovery: %v", err)
						}
					})
				}
			}
		}
	}
}
