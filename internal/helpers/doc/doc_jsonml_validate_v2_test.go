package doc

import (
	"strings"
	"testing"
)

func floatPtr(value float64) *float64 { return &value }

func TestCrossPlatformCoverageSchemaLoadingAndLookup(t *testing.T) {
	if !schemaV2.IsKnownTag("root") || schemaV2.IsKnownTag("definitely-unknown") {
		t.Fatal("schema known-tag lookup is incorrect")
	}
	root := schemaV2.TagSchemaFor("root")
	if root == nil || !root.IsAllowedChild("p") || root.IsAllowedChild("span") {
		t.Fatal("schema child lookup is incorrect")
	}
	if schemaV2.TagSchemaFor("definitely-unknown") != nil {
		t.Fatal("unknown tag returned a schema")
	}
	loaded := mustLoadSchemaV2([]byte(`{"tags":{"x":{"allowed_children":["#text"],"attrs":{}}}}`))
	if !loaded.IsKnownTag("x") || !loaded.TagSchemaFor("x").IsAllowedChild("#text") {
		t.Fatalf("mustLoadSchemaV2() = %#v", loaded)
	}
	for _, raw := range [][]byte{[]byte(`{`), []byte(`{"tags":{}}`)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("mustLoadSchemaV2(%q) did not panic", raw)
				}
			}()
			_ = mustLoadSchemaV2(raw)
		}()
	}
}

func TestCrossPlatformCoverageValidateJSONMLSourceAndShapes(t *testing.T) {
	if result := ValidateJsonMLBodyV2(nil); result.HasErrors() || result.Summary() != "" {
		t.Fatalf("empty body result = %#v", result)
	}
	if result := ValidateJsonMLSource(nil); result.HasErrors() {
		t.Fatalf("empty source result = %#v", result)
	}
	if result := ValidateJsonMLSource([]byte(`{`)); !result.HasErrors() {
		t.Fatal("invalid JSON source produced no error")
	}
	if result := ValidateJsonMLSource([]byte(`{"value":1}`)); !result.HasErrors() {
		t.Fatal("non-JSONML object produced no error")
	}
	if result := ValidateJsonMLSource([]byte(`{"jsonml":"bad"}`)); !result.HasErrors() {
		t.Fatal("invalid JSONML wrapper produced no error")
	}
	if result := ValidateJsonMLSource([]byte(`{"jsonml":["root",{},["p",{},["span",{},"ok"]]]}`)); result.HasErrors() {
		t.Fatalf("valid wrapped JSONML errors = %v", result.Errors)
	}
	if result := ValidateJsonMLSource([]byte(`["root",{},["p",{},["span",{},"ok"]]]`)); result.HasErrors() {
		t.Fatalf("valid JSONML errors = %v", result.Errors)
	}

	tests := []any{
		"not-an-array",
		[]any{},
		[]any{1},
		[]any{"unknown", map[string]any{}},
		[]any{"root", map[string]any{}, "bare text", nil, 3},
		[]any{"root", map[string]any{}, []any{"span", map[string]any{}, "bad child"}},
		[]any{"p", map[string]any{"unknown": true, "jc": 1}, []any{"span", map[string]any{}, "ok"}},
	}
	for _, node := range tests {
		result := ValidateJsonMLNodeV2(node)
		if len(result.Errors)+len(result.Warnings) == 0 {
			t.Errorf("ValidateJsonMLNodeV2(%#v) returned no diagnostics", node)
		}
	}
	result := ValidateJsonMLBodyV2([]any{[]any{"p", map[string]any{}}, "bad", []any{1}})
	if len(result.Errors)+len(result.Warnings) == 0 {
		t.Fatal("array-of-blocks validation returned no diagnostics")
	}
	if DiagnoseJSONError([]byte(`{`)) != "" {
		t.Fatal("fallback DiagnoseJSONError() should be empty")
	}
}

func TestCrossPlatformCoverageTypeValidationCoversEverySchemaType(t *testing.T) {
	result := &JsonMLValidationResult{}
	cases := []struct {
		value any
		spec  TypeSpec
	}{
		{"anything", TypeSpec{Type: "any"}},
		{"ok", TypeSpec{Type: "string"}},
		{1, TypeSpec{Type: "string"}},
		{"bad", TypeSpec{Type: "number"}},
		{float64(-1), TypeSpec{Type: "number", Min: floatPtr(0)}},
		{float64(11), TypeSpec{Type: "number", Max: floatPtr(10)}},
		{float64(5), TypeSpec{Type: "number", Min: floatPtr(0), Max: floatPtr(10)}},
		{true, TypeSpec{Type: "boolean"}},
		{"bad", TypeSpec{Type: "boolean"}},
		{[]any{}, TypeSpec{Type: "array"}},
		{"bad", TypeSpec{Type: "array"}},
		{"bad", TypeSpec{Type: "object"}},
		{map[string]any{"known": "ok", "extra": true}, TypeSpec{Type: "object", Fields: map[string]TypeSpec{"known": {Type: "string"}}}},
		{1, TypeSpec{Type: "enum", Values: []string{"one"}}},
		{"one", TypeSpec{Type: "enum", Values: []string{"one"}}},
		{"two", TypeSpec{Type: "enum", Values: []string{"one"}}},
		{"ok", TypeSpec{Type: "union", Types: []string{"string"}}},
		{true, TypeSpec{Type: "union", Types: []string{"string"}, WarnTypes: []string{"boolean"}}},
		{map[string]any{}, TypeSpec{Type: "union", Types: []string{"string"}, WarnTypes: []string{"number"}}},
	}
	for index, tc := range cases {
		checkTypeV2(tc.value, &tc.spec, "$.value", result)
		_ = index
	}
	if len(result.Errors) == 0 || len(result.Warnings) == 0 || !strings.Contains(result.Summary(), "JSONML") {
		t.Fatalf("type validation result = %#v", result)
	}

	for _, value := range []any{float64(1), int(1), int64(1), "bad"} {
		_, _ = toFloat64(value)
	}
	for _, tc := range []struct {
		value any
		types []string
	}{
		{"x", []string{"string"}}, {1, []string{"number"}}, {true, []string{"boolean"}},
		{[]any{}, []string{"array"}}, {map[string]any{}, []string{"object"}}, {nil, []string{"null"}},
		{struct{}{}, []string{"any"}}, {struct{}{}, []string{"string", "unknown"}},
	} {
		_ = matchesUnion(tc.value, tc.types)
	}
}

func TestCrossPlatformCoverageDiagnosticsFormattingAndAggregation(t *testing.T) {
	diagnostics := DiagnosticList{
		{Range: Range{Start: Position{Line: 2, Col: 3}}, Severity: SeverityError, Code: "E", Message: "bad", Suggestion: "fix"},
		{Severity: SeverityWarning, Code: "W", Message: "warn", Source: SourceJSONMLSchema},
		{Severity: SeverityInformation, Code: "I", Message: "info"},
		{Severity: SeverityHint, Code: "H", Message: "hint"},
	}
	if !diagnostics.HasErrors() || diagnostics.ErrorCount() != 1 || diagnostics.WarningCount() != 1 {
		t.Fatalf("diagnostic counts = %#v", diagnostics)
	}
	if len(diagnostics.Filter(SourceJSONMLSchema)) != 1 || !strings.Contains(diagnostics.Summary(), "fix") {
		t.Fatalf("diagnostic output = %q", diagnostics.Summary())
	}
	legacy := diagnostics.ToLegacyResult()
	if len(legacy.Errors) != 1 || len(legacy.Warnings) != 3 || !legacy.HasErrors() {
		t.Fatalf("legacy result = %#v", legacy)
	}
	if DiagnosticList(nil).Summary() != "" || (&JsonMLValidationResult{}).Summary() != "" {
		t.Fatal("empty diagnostics produced output")
	}
	for severity := SeverityError; severity <= SeverityHint; severity++ {
		_ = severity.String()
	}
	if Severity(99).String() != "severity(99)" {
		t.Fatal("unknown severity formatting changed")
	}

	warnings := DiagnosticList{{Severity: SeverityWarning, Message: "warn"}}
	if warnings.HasErrors() || !strings.Contains(warnings.Summary(), "警告") {
		t.Fatalf("warning summary = %q", warnings.Summary())
	}
	result := &JsonMLValidationResult{}
	result.addError("$", "bad.", "")
	result.addWarn("$", "warn", "try this")
	if !strings.Contains(result.Summary(), "try this") {
		t.Fatalf("legacy summary = %q", result.Summary())
	}
}
