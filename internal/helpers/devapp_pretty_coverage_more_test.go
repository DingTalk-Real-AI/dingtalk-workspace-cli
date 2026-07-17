package helpers

import (
	"encoding/json"
	"testing"
)

func TestCrossPlatformCoverageDevAppAsIntRemainingTypes(t *testing.T) {
	for _, value := range []any{int(1), int64(1), float64(1), json.Number("1"), "1"} {
		if got, ok := devAppAsInt(value); !ok || got != 1 {
			t.Fatalf("devAppAsInt(%T(%v))=%d,%v", value, value, got, ok)
		}
	}
	for _, value := range []any{json.Number("bad"), "bad", true} {
		if got, ok := devAppAsInt(value); ok || got != 0 {
			t.Fatalf("invalid devAppAsInt(%T(%v))=%d,%v", value, value, got, ok)
		}
	}
}
