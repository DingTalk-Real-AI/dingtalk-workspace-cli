//go:build !goexperiment.jsonv2

package doc

// DiagnoseJSONError is a no-op fallback for builds without goexperiment.jsonv2.
// Returns empty string (no enhanced diagnostics available).
func DiagnoseJSONError(src []byte) string {
	return ""
}
