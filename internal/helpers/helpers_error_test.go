package helpers

import (
	"encoding/json"
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
