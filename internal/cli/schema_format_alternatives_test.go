package cli

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
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
