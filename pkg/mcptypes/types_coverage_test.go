package mcptypes

import (
	"encoding/json"
	"testing"
)

func TestCLIFlagOverrideUnmarshalJSONCoercesScalarDefaults(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing", raw: `{}`, want: ""},
		{name: "null", raw: `{"default":null}`, want: ""},
		{name: "object", raw: `{"default":{"nested":true}}`, want: ""},
		{name: "array", raw: `{"default":[1,2]}`, want: ""},
		{name: "string", raw: `{"default":"hello"}`, want: "hello"},
		{name: "escaped string", raw: `{"default":"line\nvalue"}`, want: "line\nvalue"},
		{name: "boolean", raw: `{"default":true}`, want: "true"},
		{name: "integer", raw: `{"default":42}`, want: "42"},
		{name: "number", raw: `{"default":-1.25}`, want: "-1.25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got CLIFlagOverride
			if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got.Default != tt.want {
				t.Fatalf("Default = %q, want %q", got.Default, tt.want)
			}
		})
	}

	var got CLIFlagOverride
	if err := json.Unmarshal([]byte(`{"alias":`), &got); err == nil {
		t.Fatal("json.Unmarshal() error = nil, want malformed JSON error")
	}
	if err := got.UnmarshalJSON([]byte(`{"alias":`)); err == nil {
		t.Fatal("CLIFlagOverride.UnmarshalJSON() error = nil, want malformed JSON error")
	}
	if err := json.Unmarshal([]byte(`{"unknownFlagField":true}`), &got); err == nil {
		t.Fatal("CLIFlagOverride.UnmarshalJSON() accepted an unknown field")
	}
	if got := coercePluginScalar(json.RawMessage(`"unterminated`)); got != "" {
		t.Fatalf("coercePluginScalar(invalid string) = %q, want empty", got)
	}
}

func TestOverlayFromJSONHandlesEmptyValidAndMalformedInput(t *testing.T) {
	if got := OverlayFromJSON(nil); got.ID != "" || got.Command != "" {
		t.Fatalf("OverlayFromJSON(nil) = %#v, want zero overlay", got)
	}

	valid := json.RawMessage(`{
		"id":"conference",
		"command":"meeting",
		"toolOverrides":{"create":{"flags":{"count":{"default":3}}}}
	}`)
	got := OverlayFromJSON(valid)
	if got.ID != "conference" || got.Command != "meeting" ||
		got.ToolOverrides["create"].Flags["count"].Default != "3" {
		t.Fatalf("OverlayFromJSON(valid) = %#v", got)
	}

	if got := OverlayFromJSON(json.RawMessage(`{`)); got.ID != "" || got.Command != "" {
		t.Fatalf("OverlayFromJSON(malformed) = %#v, want zero overlay", got)
	}
}
