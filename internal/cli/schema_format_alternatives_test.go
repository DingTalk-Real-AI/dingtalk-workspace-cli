package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSchemaFormatAlternatives(t *testing.T) {
	cmd := &cobra.Command{Use: "sample"}
	cmd.Flags().String("time", "", "时间 ISO-8601")
	decl := contract.ParamDecl{Name: "time", Property: "startDateTime", AnyOf: []contract.FormatAlternative{{Format: "date-time"}, {Format: "date"}}}
	if err := contractfinal.ApplyParamDecls(cmd, []contract.ParamDecl{decl}); err != nil {
		t.Fatal(err)
	}
	params, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{})
	if err != nil || len(params) != 1 {
		t.Fatalf("parameters: %v, %v", params, err)
	}
	p := params[0]
	if p.Format != "" || len(p.AnyOf) != 2 || p.AnyOf[0].Format != "date" {
		t.Fatalf("format union: %+v", p)
	}
	payload, err := p.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["format"]; ok {
		t.Fatal("union still constrained by format")
	}
	if p.FieldProvenance["anyOf"].Source != "native_annotation" {
		t.Fatalf("missing provenance: %+v", p.FieldProvenance)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire schemaParamWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire.AnyOf, p.AnyOf) {
		t.Fatalf("wire lost anyOf: %s", raw)
	}
	cloned := p.normalized()
	cloned.AnyOf[0].Format = "uri"
	if p.AnyOf[0].Format != "date" {
		t.Fatal("normalization aliases format alternatives")
	}
}

func TestCrossPlatformCoverageCatalogFormatAlternatives(t *testing.T) {
	valid := []any{map[string]any{"format": "date"}, map[string]any{"format": "date-time"}}
	for _, tc := range []struct {
		name      string
		value     any
		typ       string
		topFormat bool
		wantError bool
	}{
		{"valid", valid, "string", false, false},
		{"nonstring", valid, "integer", false, true},
		{"top format", valid, "string", true, true},
		{"null", nil, "string", false, true},
		{"object", map[string]any{"format": "date"}, "string", false, true},
		{"empty", []any{}, "string", false, true},
		{"single", valid[:1], "string", false, true},
		{"empty branch", []any{valid[0], map[string]any{}}, "string", false, true},
		{"unknown branch field", []any{valid[0], map[string]any{"format": "date-time", "pattern": ".*"}}, "string", false, true},
		{"duplicate", []any{valid[0], valid[0]}, "string", false, true},
		{"whitespace", []any{valid[0], map[string]any{"format": " date-time"}}, "string", false, true},
		{"nonstring format", []any{valid[0], map[string]any{"format": true}}, "string", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := validCatalogToolEntry()
			param := entry["parameters"].(map[string]any)["base-id"].(map[string]any)
			param["anyOf"] = tc.value
			param["type"] = tc.typ
			if tc.topFormat {
				param["format"] = "date-time"
			}
			err := ValidateCatalogStructure(catalogPayload(t, entry))
			if (err != nil) != tc.wantError {
				t.Fatalf("validation error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaFormatAlternativesRejectInvalidAnnotations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		format string
		want   string
	}{
		{"missing value", nil, "", "invalid or conflicting"},
		{"multiple values", []string{"[]", "[]"}, "", "invalid or conflicting"},
		{"explicit format", []string{`[{"format":"date"},{"format":"date-time"}]`}, "date-time", "invalid or conflicting"},
		{"invalid json", []string{"["}, "", "anyOf:"},
		{"unknown branch field", []string{`[{"pattern":".*"}]`}, "", "unknown field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "sample"}
			cmd.Flags().String("time", "", "time")
			cmd.Flags().Lookup("time").Annotations = map[string][]string{runtimeannotate.AnnotationFlagAnyOf: tc.values}
			if tc.format != "" {
				runtimeannotate.AnnotateRuntimeFlagFormat(cmd, "time", tc.format)
			}
			_, err := runtimeCommandParameterSpecs(cmd, "sample.run", RuntimeSchemaConstraints{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaFormatAlternativesPayloadAndProvenance(t *testing.T) {
	union := []contract.FormatAlternative{{Format: "date"}, {Format: "date-time"}}
	for _, p := range []ParameterSpec{{Name: "time", Type: "integer", AnyOf: union}, {Name: "time", Type: "string", Format: "date-time", AnyOf: union}} {
		if _, err := p.ToPayload(); err == nil || !strings.Contains(err.Error(), "no top-level format") {
			t.Fatalf("invalid combination accepted: %v", err)
		}
	}
	p := ParameterSpec{Name: "time", Type: "string", Property: "startDateTime", AnyOf: union}
	value, ok := p.provenanceValue("anyOf")
	if !ok || !reflect.DeepEqual(value, union) {
		t.Fatalf("provenance value: %v, %v", value, ok)
	}
	p.FieldProvenance = schemaDeliveryTestProvenance(map[string]any{
		"type": "string", "description": "", "property": "startDateTime", "required": false, "required_when": "", "anyOf": union,
	})
	registry := schemaDeliveryTestRegistry(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run", Parameters: []ParameterSpec{p}})
	if err := validateFinalSchemaProvenanceCoverage(registry); err != nil {
		t.Fatalf("valid provenance rejected: %v", err)
	}
	delete(registry.Products[0].Tools[0].Parameters[0].FieldProvenance, "anyOf")
	if err := validateFinalSchemaProvenanceCoverage(registry); err == nil || !strings.Contains(err.Error(), "has no provenance for anyOf") {
		t.Fatalf("missing provenance accepted: %v", err)
	}
}
