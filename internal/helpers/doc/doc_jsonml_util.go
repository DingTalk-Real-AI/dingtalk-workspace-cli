package doc

// deepCloneAny clones JSON-shaped values (map[string]any, []any, primitives).
// Used by test helpers to compare before/after without shared references.
func deepCloneAny(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCloneAny(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = deepCloneAny(val)
		}
		return out
	default:
		return v
	}
}
