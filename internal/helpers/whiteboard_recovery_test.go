package helpers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWhiteboardRecoveryDoesNotLeakAcrossProducts(t *testing.T) {
	for _, code := range []string{"SCHEMA_INVALID", "OVERWRITE_UNSAFE", "REVISION_MISMATCH", "STATE_TIMEOUT", "COMMIT_FAILED", "CREATE_RESULT_UNKNOWN", "CHECKPOINT_INVALID", "CHECKPOINT_DECODE_FAILED", "CHECKPOINT_HYDRATION_FAILED"} {
		for _, product := range []string{"sheet", "doc", "drive", "aitable", "chat"} {
			for _, guided := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/guided=%t", product, code, guided), func(t *testing.T) {
					body := `{"success":false,"errorCode":"` + code + `","errorMsg":"service failure"`
					if guided {
						body += `,"meta":{"suggestions":[{"reason":"server-specific-recovery"}]}`
					}
					body += `}`
					caller := &whiteboardTestCaller{response: func(whiteboardTestCall, int) string { return body }}
					installWhiteboardTestCaller(t, caller)
					// Exercise the real shared MCP parser, not only the suggestion helper.
					_, err := CallMCPToolTextOnServer(product, "get_item", nil)
					var failure *CLIError
					if !errors.As(err, &failure) || failure.ServerCode != code || failure.Code != CodeMCPToolError || failure.Message != body || failure.ExitCode() != ExitAPI {
						t.Fatalf("classification or original body changed: %#v", err)
					}
					want := ""
					if guided {
						want = "server-specific-recovery"
					}
					if failure.Suggestion != want || len(caller.calls) != 1 {
						t.Fatalf("cross-product guidance: %q, calls=%#v", failure.Suggestion, caller.calls)
					}
					if strings.Contains(failure.Error(), "OpenNodes") || strings.Contains(failure.Error(), "白板") {
						t.Fatal("whiteboard hint leaked into another product")
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageWhiteboardRecoveryPreservesErrorContract(t *testing.T) {
	original := &CLIError{Code: CodeMCPToolError, ServerCode: "SCHEMA_INVALID", Message: "original", Operation: "whiteboard/update", Details: map[string]any{"logId": "trace"}, Cause: context.Canceled}
	before := *original
	projected := WithWhiteboardRecovery(original).(*CLIError)
	if projected == original || !reflect.DeepEqual(*original, before) || !errors.Is(projected, original) || !errors.Is(projected, context.Canceled) {
		t.Fatal("mutated original or lost error chain")
	}
	if projected.Code != original.Code || projected.ServerCode != original.ServerCode || projected.Message != original.Message || projected.Operation != original.Operation || !reflect.DeepEqual(projected.Details, original.Details) || projected.ExitCode() != original.ExitCode() || !strings.Contains(projected.Suggestion, "OpenNodes") {
		t.Fatalf("error contract changed: %#v", projected)
	}
	var typedNil *CLIError
	for _, err := range []error{nil, typedNil, context.Canceled, context.DeadlineExceeded,
		&CLIError{Code: CodeAuthPermission, ServerCode: "SCHEMA_INVALID"},
		&CLIError{Code: CodeMCPToolError, ServerCode: "NEW_CODE"},
		&CLIError{Code: CodeMCPToolError, ServerCode: "SCHEMA_INVALID", Suggestion: "server hint"},
		apperrors.NewValidation("validation"),
		fmt.Errorf("outer context: %w", original),
	} {
		if got := WithWhiteboardRecovery(err); got != err {
			t.Fatalf("unexpected replacement: %v", err)
		}
	}
}
