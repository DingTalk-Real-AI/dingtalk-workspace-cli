package helpers

import "testing"

func TestCrossPlatformCoverageIsBusinessErrorRecognizesRealErrorEnvelopes(t *testing.T) {
	cases := []map[string]any{
		{"status": "error", "success": true, "error": map[string]any{"code": "INVALID_BASE_ID"}},
		{"success": true, "errorCode": "1001"},
		{"success": true, "error": []any{"failed"}},
		{"success": false},
	}
	for _, tc := range cases {
		if !isBusinessError(tc) {
			t.Fatalf("isBusinessError(%v) = false, want true", tc)
		}
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
