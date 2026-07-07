package helpers

import "testing"

func TestDocVersionNextCursorUnwrapsEnvelope(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"hasMore":    true,
			"nextCursor": "page-2",
			"versions": []any{
				map[string]any{"version": float64(1)},
			},
		},
	}

	if got := docVersionNextCursor(payload); got != "page-2" {
		t.Fatalf("docVersionNextCursor() = %q, want page-2", got)
	}
}

func TestDocVersionNextCursorHonorsNestedHasMoreFalse(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"hasMore":    false,
			"nextCursor": "stale",
		},
	}

	if got := docVersionNextCursor(payload); got != "" {
		t.Fatalf("docVersionNextCursor() = %q, want empty", got)
	}
}
