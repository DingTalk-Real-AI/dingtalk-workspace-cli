// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageCompactRemainingShapes(t *testing.T) {
	if Compact(nil) != nil {
		t.Fatal("nil compact")
	}
	payload := map[string]any{
		"kind":        "schema",
		"dropped":     "x",
		"product":     map[string]any{"id": "p", "dropped": 1},
		"products":    []map[string]any{{"id": "a"}},
		"tools":       []any{map[string]any{"id": "t"}, "plain"},
		"parameters":  map[string]any{"flag": map[string]any{"type": "string", "secret": "no"}, "raw": 3},
		"description": "d",
	}
	got := Compact(payload)
	if _, ok := got["dropped"]; ok {
		t.Fatal("dropped key survived")
	}
	product, _ := got["product"].(map[string]any)
	if product["id"] != "p" || product["dropped"] != nil {
		t.Fatalf("nested product = %#v", product)
	}
	if Compact(map[string]any{"product": "not-map"})["product"] != "not-map" {
		t.Fatal("non-map product")
	}
	if CompactCollection("x") != "x" {
		t.Fatal("collection default")
	}
	if CompactCollection([]map[string]any{{"id": "a"}}) == nil {
		t.Fatal("map collection")
	}
	if CompactParameters("plain") != "plain" {
		t.Fatal("plain parameters")
	}
	if CompactValue(map[string]any{"type": "string", "secret": 1}).(map[string]any)["secret"] != nil {
		t.Fatal("param-shaped value")
	}
	if CompactValue(map[string]any{"required": true, "secret": 1}).(map[string]any)["secret"] != nil {
		t.Fatal("required-shaped value")
	}
	if CompactValue(map[string]any{"kind": "schema", "dropped": 1}).(map[string]any)["dropped"] != nil {
		t.Fatal("nested compact value")
	}
	if CompactValue([]map[string]any{{"id": "a"}}) == nil {
		t.Fatal("value map slice")
	}
	if CompactValue([]any{1, map[string]any{"id": "a"}}) == nil {
		t.Fatal("value any slice")
	}
	if CompactValue(9) != 9 {
		t.Fatal("scalar value")
	}
	if CompactParameter(map[string]any{"type": "string", "extra": 1})["extra"] != nil {
		t.Fatal("param extra")
	}

	tool := allFieldsRegistry().Products[0].Tools[0]
	if _, ok := tool.provenanceValue("missing"); ok {
		t.Fatal("unknown tool field")
	}
	for _, field := range []string{
		"description", "metadata_source", "dry_run", "effect", "effect_source", "risk",
		"confirmation", "idempotency", "interface_ref", "interface_mode", "availability",
		"interface_reason", "agent_summary", "use_when", "avoid_when", "prerequisites",
		"tips", "workflow_refs", "examples", "reviewed",
	} {
		if _, ok := tool.provenanceValue(field); !ok {
			t.Fatalf("tool field %s", field)
		}
	}
	param := tool.Parameters[0]
	if _, ok := param.provenanceValue("missing"); ok {
		t.Fatal("unknown param field")
	}
	for _, field := range []string{
		"name", "type", "description", "property", "required", "cli_required",
		"required_when", "default", "interface_default", "example", "anyOf",
		"format", "enum", "interface_description", "interface_type",
	} {
		if _, ok := param.provenanceValue(field); !ok {
			t.Fatalf("param field %s", field)
		}
	}
	full, err := tool.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if full["title"] != "Run sample" || full["parameters"] == nil {
		t.Fatalf("payload = %#v", full)
	}
	summary, err := tool.ToSummaryPayload()
	if err != nil || summary["parameters"] != nil {
		t.Fatalf("summary = %#v %v", summary, err)
	}
	if _, err := (ParameterSpec{Name: "bad", Type: "int", AnyOf: []contract.FormatAlternative{{Format: "uri"}}}).ToPayload(); err == nil {
		t.Fatal("anyOf type accepted")
	}
	if _, err := (ParameterSpec{Name: "bad", Type: "string", Format: "uri", AnyOf: []contract.FormatAlternative{{Format: "uri"}}}).ToPayload(); err == nil {
		t.Fatal("anyOf format accepted")
	}
	if _, err := (ParameterSpec{Name: "bad", Default: json.RawMessage("not-json")}).ToPayload(); err == nil {
		t.Fatal("invalid default accepted")
	}

	emptyCLI := tool
	emptyCLI.Identity.CLIPath = ""
	if len(BuildCommandMetaLookup(SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{emptyCLI}}}})) != 0 {
		t.Fatal("empty cli path entered lookup")
	}
	if !(CommandSafety{Effect: "read"}).ShouldRender() || (CommandSafety{}).ShouldRender() {
		t.Fatal("should render")
	}

	reg := allFieldsRegistry()
	index, err := reg.Index()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderQuery(reg, index, "sample missing"); err == nil {
		t.Fatal("unknown path")
	}
	if _, err := RenderQueryWithProjectors(reg, index, "sample", QueryProjectors{
		ProductSummary: func(ProductSpec) (map[string]any, error) { return nil, errQuery },
	}); err == nil {
		t.Fatal("forced product summary succeeded")
	}
	if _, err := RenderQueryWithProjectors(reg, index, "sample group", QueryProjectors{
		ToolSummary: func(ToolSpec) (map[string]any, error) { return nil, errQuery },
	}); err == nil {
		t.Fatal("forced tool summary succeeded")
	}
	if !ToolUnderGroup(tool, "sample group") {
		t.Fatal("under group")
	}
	if ToolUnderGroup(tool, "other") {
		t.Fatal("not under group")
	}
	aliased := AliasView(tool, "sample legacy run")
	if !aliased.Identity.IsAlias || aliased.Identity.CLIPath != "sample legacy run" {
		t.Fatalf("alias view = %#v", aliased.Identity)
	}
	if AliasView(tool, "sample group run").Identity.IsAlias {
		t.Fatal("primary marked alias")
	}
	if AliasView(tool, "").Identity.IsAlias {
		t.Fatal("empty alias")
	}
	StampTrustedHashes(full, TrustedHashes{CatalogHash: "c", SurfaceHash: "s"})
	if full["catalog_hash"] != "c" || full["surface_hash"] != "s" {
		t.Fatalf("hashes = %#v", full)
	}
	StampTrustedHashes(full, TrustedHashes{CatalogHash: "c"})
	if sourceOrDefault("  ") != runtimeAssembledSource || sourceOrDefault("live") != "live" {
		t.Fatal("source default")
	}
	if quote(`a"b`) != `"a\"b"` {
		t.Fatal("quote")
	}
	if !reflect.DeepEqual(SplitPathTokens(" a./b "), []string{"a", "b"}) {
		t.Fatal("split")
	}
	if DefaultString("", "x") != "x" || StableUniqueStrings([]string{"b", "a", "b"})[0] != "b" {
		t.Fatal("helpers")
	}
	if NormalizeCLIPath("  dws   sample\trun  ") != "sample run" {
		t.Fatal("normalize fields")
	}
	if NormalizeCLIPath("dws") != "" || NormalizeCLIPath("dws sample run") != "sample run" {
		t.Fatal("normalize dws prefix")
	}
	if NormalizeCLIPath("样 本") != "样 本" && NormalizeQueryCLIPath("dws/sample/run") != "sample run" {
		t.Fatal("normalize unicode/query")
	}
	if NormalizeQueryCLIPath("dws sample run") != "sample run" {
		t.Fatal("query dws prefix")
	}

	built, meta := buildFixtureCache(t, allFieldsRegistry())
	desc := meta.PayloadDescriptors[0]
	shard := extractProductPayload(t, built, desc)
	decoded, err := DecodeSchemaCommandPayloadCache(shard, desc, meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded.RenderedLeaf("sample.run"); !ok {
		t.Fatal("canonical rendered leaf missing")
	}
	if _, ok := decoded.RenderedLeaf("missing.run"); ok {
		t.Fatal("unknown rendered leaf resolved")
	}
}

var errQuery = json.Unmarshal([]byte("x"), new(int))
