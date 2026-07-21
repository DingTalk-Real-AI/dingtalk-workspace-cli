package helpers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageIsBusinessErrorRecognizesRealErrorEnvelopes(t *testing.T) {
	cases := []map[string]any{
		{"status": "error", "success": true, "error": map[string]any{"code": "INVALID_BASE_ID"}},
		{"status": " error ", "success": true},
		{"success": true, "errorCode": "1001"},
		{"success": true, "error": []any{"failed"}},
		{"success": true, "error": true},
		{"success": false},
	}
	for _, tc := range cases {
		if !isBusinessError(tc) {
			t.Fatalf("isBusinessError(%v) = false, want true", tc)
		}
	}
}

func TestCrossPlatformCoverageErrorCodeValueShapes(t *testing.T) {
	for _, value := range []any{float64(1), int(1), int64(1), json.Number("1")} {
		if !isErrorCodeValue(value) {
			t.Fatalf("isErrorCodeValue(%T(%v)) = false, want true", value, value)
		}
	}
	if isErrorCodeValue(" ") {
		t.Fatal("blank error code should not be classified as an error")
	}
}

func TestCrossPlatformCoverageIsBusinessErrorAllowsSuccessEnvelope(t *testing.T) {
	body := map[string]any{
		"success":   true,
		"errorCode": nil,
		"errorMsg":  nil,
		"result":    map[string]any{"ok": true},
	}
	if isBusinessError(body) {
		t.Fatalf("isBusinessError(%v) = true, want false", body)
	}
}

func TestCrossPlatformCoverageIsBusinessErrorAllowsCodeZeroSuccessEnvelope(t *testing.T) {
	body := map[string]any{
		"success": true,
		"code":    "0",
		"message": "success",
		"result":  []any{},
	}
	if isBusinessError(body) {
		t.Fatalf("isBusinessError(%v) = true, want false", body)
	}
}

func TestPermissionBusinessErrorSuggestsApplyWorkflow(t *testing.T) {
	hint := suggestForBusinessError(map[string]any{
		"success":   false,
		"errorCode": "NO_PERMISSION",
		"errorMsg":  "没有访问权限",
	})
	for _, expected := range []string{"drive permission apply-info", "drive permission apply"} {
		if !strings.Contains(hint, expected) {
			t.Fatalf("hint = %q, want %q", hint, expected)
		}
	}
}

func TestClassifyToolResultContentRecognizesPermissionCode(t *testing.T) {
	err := ClassifyToolResultContent(map[string]any{
		"success":           false,
		"server_error_code": "NO_PERMISSION",
		"message":           "access denied",
	})
	cliErr, ok := err.(*CLIError)
	if !ok || cliErr.Code != CodeAuthPermission {
		t.Fatalf("error = %#v, want permission CLIError", err)
	}
	if !strings.Contains(cliErr.Suggestion, "permission apply-info") {
		t.Fatalf("suggestion = %q", cliErr.Suggestion)
	}
}

func TestDriveNotFoundErrorAlsoSuggestsPermissionWorkflow(t *testing.T) {
	err := WrapErrorWithOperation(errors.New("resource not found"), "drive/get_dentry")
	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("error = %#v, want CLIError", err)
	}
	if !strings.Contains(cliErr.Suggestion, "permission apply-info") {
		t.Fatalf("suggestion = %q", cliErr.Suggestion)
	}
}
