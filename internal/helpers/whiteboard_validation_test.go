package helpers

import (
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWhiteboardInvalidSourceStopsEveryNativeWrite(t *testing.T) {
	for _, operation := range []string{"create-with-content", "update"} {
		t.Run(operation, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json"}
			installWhiteboardTestCaller(t, caller)
			path := writeWhiteboardFixture(t, `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"shape","x":"private-invalid-value"}]}}`)
			cmd := newWhiteboardCommand()
			cmd.PersistentFlags().Bool("yes", false, "")
			args := []string{operation, "--source", path, "--yes"}
			if operation == "create-with-content" {
				args = append(args, "--name", "Board", "--request-id", "invalid-1")
			} else {
				args = append(args, "--node", "doc", "--part-id", "part")
			}
			cmd.SetArgs(args)
			err := corecmd.ExecuteForTest(cmd)
			if err == nil || apperrors.ExitCode(err) != 3 || !strings.Contains(err.Error(), "/source/nodes/0/x") {
				t.Fatalf("want actionable validation/3, got %v", err)
			}
			if len(caller.calls) != 0 || strings.Contains(err.Error(), "private-invalid-value") {
				t.Fatalf("invalid source leaked or reached MCP: err=%v calls=%#v", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardRecoveryKeepsServerCode(t *testing.T) {
	for _, tc := range []struct{ code, hint string }{
		{"SCHEMA_INVALID", "不要原样重试"},
		{"OVERWRITE_UNSAFE", "不要原样重试"},
		{"REVISION_MISMATCH", "不要只替换 expectedRevision"},
		{"STATE_TIMEOUT", "不要自动重发"},
		{"COMMIT_FAILED", "不要自动重发"},
		{"CREATE_RESULT_UNKNOWN", "不要自动重发"},
		{"CHECKPOINT_INVALID", "不要改用 overwrite"},
		{"CHECKPOINT_DECODE_FAILED", "不要改用 overwrite"},
		{"CHECKPOINT_HYDRATION_FAILED", "不要改用 overwrite"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			text := `{"success":false,"errorCode":"` + tc.code + `","errorMsg":"service failure"}`
			caller := &whiteboardTestCaller{response: func(whiteboardTestCall, int) string { return text }}
			installWhiteboardTestCaller(t, caller)
			cmd := newWhiteboardCommand()
			cmd.SetArgs([]string{"query", "--node", "doc", "--part-id", "part"})
			err := corecmd.ExecuteForTest(cmd)
			var failure *CLIError
			if !errors.As(err, &failure) || failure.ServerCode != tc.code || !strings.Contains(failure.Suggestion, tc.hint) {
				t.Fatalf("lost recovery facts: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("failure triggered extra calls: %#v", caller.calls)
			}
		})
	}
}
