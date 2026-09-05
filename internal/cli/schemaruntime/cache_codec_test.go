// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"google.golang.org/protobuf/proto"
)

func TestSchemaCacheAllFieldsExactRoundTrip(t *testing.T) {
	registry := allFieldsRegistry()
	built, meta := buildFixtureCache(t, registry)
	decoded, index, err := DecodeAllSchemaProducts(built.ProductShards, meta)
	if err != nil {
		t.Fatalf("DecodeAllSchemaProducts() error = %v", err)
	}
	if !reflect.DeepEqual(registry, decoded) {
		t.Fatalf("Registry round trip mismatch\nwant: %#v\n got: %#v", registry, decoded)
	}
	if got := len(index.CanonicalPaths()); got != 2 {
		t.Fatalf("decoded canonical paths = %d, want 2", got)
	}
	wantPayload, err := registry.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	gotPayload, err := decoded.ToPayload()
	if err != nil || !reflect.DeepEqual(wantPayload, gotPayload) {
		t.Fatalf("public wire mismatch: %v", err)
	}
	wantOverview, err := registry.ToOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	gotOverview, err := meta.Overview.ToPayload()
	if err != nil || !reflect.DeepEqual(wantOverview, gotOverview) {
		t.Fatalf("overview wire mismatch: %v\nwant: %#v\n got: %#v", err, wantOverview, gotOverview)
	}
	if !reflect.DeepEqual(BuildCommandMetaLookup(registry), meta.CommandMetaByPath) {
		t.Fatal("CommandMeta lookup round trip mismatch")
	}
	if got := decoded.Products[0].Tools[0].Parameters[0]; got.Default != nil || got.InterfaceDefault == nil || len(got.InterfaceDefault) != 0 || string(got.Example) != "null" {
		t.Fatalf("RawMessage presence changed: %#v", got)
	}
	selection := decoded.Products[0].Tools[0].Selection
	if selection.Reviewed == nil || *selection.Reviewed || selection.Prerequisites != nil || selection.Tips == nil || len(selection.Tips) != 0 {
		t.Fatalf("pointer/list presence changed: %#v", selection)
	}
	candidate := decoded.Products[0].Tools[0].FieldProvenance["title"].Candidates[1]
	if candidate.Selected == nil || *candidate.Selected {
		t.Fatalf("*bool false presence changed: %#v", candidate.Selected)
	}
	disposition := decoded.Products[0].Tools[0].Selection.ExampleDispositions[0]
	if disposition.Index == nil || *disposition.Index != 0 {
		t.Fatalf("*int zero presence changed: %#v", disposition.Index)
	}
	if decoded.Products[0].Tools[1].FieldProvenance == nil || len(decoded.Products[0].Tools[1].FieldProvenance) != 0 {
		t.Fatalf("empty map presence changed: %#v", decoded.Products[0].Tools[1].FieldProvenance)
	}
}

func TestSchemaCacheDeterministicAndDeepCopies(t *testing.T) {
	registry := allFieldsRegistry()
	lookup := BuildCommandMetaLookup(registry)
	overview, err := BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	hashes := fixtureHashes()
	first, err := BuildSchemaCache(registry, lookup, overview, locators, hashes)
	if err != nil {
		t.Fatal(err)
	}
	reversedLookup := make(map[string]CommandMeta, len(lookup))
	keys := sortedMapKeys(lookup)
	for i := len(keys) - 1; i >= 0; i-- {
		reversedLookup[keys[i]] = lookup[keys[i]]
	}
	reversedLocators := make(map[string]string, len(locators))
	locatorKeys := sortedMapKeys(locators)
	for i := len(locatorKeys) - 1; i >= 0; i-- {
		reversedLocators[locatorKeys[i]] = locators[locatorKeys[i]]
	}
	second, err := BuildSchemaCache(registry, reversedLookup, overview, reversedLocators, hashes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Meta, second.Meta) || !bytes.Equal(first.ProductShards, second.ProductShards) {
		t.Fatal("deterministic build changed with map insertion order")
	}
	meta, err := DecodeSchemaMetaCache(first.Meta)
	if err != nil {
		t.Fatal(err)
	}
	product, err := DecodeSchemaProductFromShards(first.ProductShards, meta, "sample")
	if err != nil {
		t.Fatal(err)
	}
	for i := range first.Meta {
		first.Meta[i] = 0
	}
	for i := range first.ProductShards {
		first.ProductShards[i] = 0
	}
	registry.AgentMetadata[0] = 'x'
	registry.Products[0].Tools[0].Selection.UseWhen[0] = "mutated"
	if string(meta.AgentMetadata) != "null" || product.Registry.Products[0].Tools[0].Selection.UseWhen[0] != "use sample" {
		t.Fatal("decoded runtime values alias source or encoded buffers")
	}
}

func TestSchemaCacheStableProductSort(t *testing.T) {
	product := func(id string) ProductSpec {
		return ProductSpec{ID: id, Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{
			ProductID: id, Name: "run", CanonicalPath: id + ".run", Path: id + ".run", CLIPath: id + " run", PrimaryCLIPath: id + " run",
		}}}}
	}
	left := SchemaRegistry{Kind: "schema", Level: "catalog", Products: []ProductSpec{product("b"), product("a")}}
	right := SchemaRegistry{Kind: "schema", Level: "catalog", Products: []ProductSpec{product("a"), product("b")}}
	leftBuilt, leftMeta := buildFixtureCache(t, left)
	rightBuilt, _ := buildFixtureCache(t, right)
	if !bytes.Equal(leftBuilt.Meta, rightBuilt.Meta) || !bytes.Equal(leftBuilt.ProductShards, rightBuilt.ProductShards) {
		t.Fatal("product input order changed deterministic artifacts")
	}
	if got := []string{leftMeta.ProductDescriptors[0].ProductID, leftMeta.ProductDescriptors[1].ProductID}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("descriptor order = %v", got)
	}
}

func TestSchemaCacheBuildRejectsProjectionDrift(t *testing.T) {
	registry := allFieldsRegistry()
	lookup := BuildCommandMetaLookup(registry)
	overview, err := BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("lookup", func(t *testing.T) {
		bad := make(map[string]CommandMeta, len(lookup))
		for key, value := range lookup {
			bad[key] = value
		}
		delete(bad, "sample zzz")
		if _, err := BuildSchemaCache(registry, bad, overview, locators, fixtureHashes()); err == nil {
			t.Fatal("drifted lookup unexpectedly succeeded")
		}
	})
	t.Run("overview", func(t *testing.T) {
		bad := overview
		bad.ToolCount++
		if _, err := BuildSchemaCache(registry, lookup, bad, locators, fixtureHashes()); err == nil {
			t.Fatal("drifted overview unexpectedly succeeded")
		}
	})
	t.Run("locators", func(t *testing.T) {
		bad := make(map[string]string, len(locators))
		for key, value := range locators {
			bad[key] = value
		}
		delete(bad, "sample.zzz")
		if _, err := BuildSchemaCache(registry, lookup, overview, bad, fixtureHashes()); err == nil {
			t.Fatal("drifted locator unexpectedly succeeded")
		}
	})
}

func TestSchemaCacheRejectsMalformedMeta(t *testing.T) {
	built, _ := buildFixtureCache(t, allFieldsRegistry())
	mutate := func(t *testing.T, edit func(*schemacachepb.SchemaMetaCache)) []byte {
		t.Helper()
		var message schemacachepb.SchemaMetaCache
		if err := proto.Unmarshal(built.Meta, &message); err != nil {
			t.Fatal(err)
		}
		edit(&message)
		payload, err := MarshalSchemaCacheDeterministic(&message)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	tests := map[string][]byte{
		"unknown-root-field": append(append([]byte(nil), built.Meta...), 0xf8, 0x07, 0x01),
		"unknown-enum": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.DtoVersion = schemacachepb.DTOVersion(99)
		}),
		"wrong-version": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_UNSPECIFIED
		}),
		"retired-dto-version": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_V1
		}),
		"retired-nested-meta-field": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.CommandEntries.Items[0].ProtoReflect().SetUnknown([]byte{0x12, 0x00})
		}),
		"unknown-list-presence-bit": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.CommandEntries.Items[0].ListsPresent |= 1 << 6
		}),
		"nonempty-list-without-presence": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.CommandEntries.Items[0].Tips = []string{"tip"}
			message.CommandEntries.Items[0].ListsPresent &^= 1 << 4
		}),
		"duplicate-command-key": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.CommandEntries.Items = append(message.CommandEntries.Items, proto.Clone(message.CommandEntries.Items[0]).(*schemacachepb.CommandMetaEntry))
		}),
		"unsorted-locator": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			if len(message.Locators.Items) > 1 {
				message.Locators.Items[0], message.Locators.Items[1] = message.Locators.Items[1], message.Locators.Items[0]
			}
		}),
		"descriptor-gap": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.ProductDescriptors.Items[0].Offset = 1
		}),
		"duplicate-descriptor": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.ProductDescriptors.Items = append(message.ProductDescriptors.Items, proto.Clone(message.ProductDescriptors.Items[0]).(*schemacachepb.ProductDescriptor))
		}),
		"descriptor-bad-hash": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.ProductDescriptors.Items[0].Sha256 = []byte{1}
		}),
		"aggregate-too-large": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.RegistryDataLength = MaxSchemaShardData + 1
		}),
		"overview-unknown-summary": mutate(t, func(message *schemacachepb.SchemaMetaCache) {
			message.Overview.Products.Items[0].SummaryKind = schemacachepb.OverviewSummaryKind(99)
		}),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSchemaMetaCache(payload); err == nil {
				t.Fatal("DecodeSchemaMetaCache() unexpectedly succeeded")
			}
		})
	}
	if _, err := DecodeSchemaMetaCache(nil); err == nil {
		t.Fatal("empty Meta unexpectedly succeeded")
	}
}

func TestSchemaCacheRejectsMalformedProductBeforeRuntimeUse(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	descriptor := meta.ProductDescriptors[0]
	if _, err := DecodeSchemaProductCache(built.ProductShards[:len(built.ProductShards)-1], descriptor, meta); err == nil {
		t.Fatal("truncated shard unexpectedly succeeded")
	}
	corrupt := append([]byte(nil), built.ProductShards...)
	corrupt[len(corrupt)/2] ^= 1
	if _, err := DecodeSchemaProductCache(corrupt, descriptor, meta); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("corrupt shard error = %v", err)
	}

	mutate := func(t *testing.T, edit func(*schemacachepb.SchemaProductCache)) ([]byte, ProductDescriptor, DecodedSchemaMeta) {
		t.Helper()
		var message schemacachepb.SchemaProductCache
		if err := proto.Unmarshal(built.ProductShards, &message); err != nil {
			t.Fatal(err)
		}
		edit(&message)
		payload, err := MarshalSchemaCacheDeterministic(&message)
		if err != nil {
			t.Fatal(err)
		}
		updated := descriptor
		updated.Length = uint64(len(payload))
		updated.SHA256 = sha256.Sum256(payload)
		updatedMeta := meta
		updatedMeta.ProductDescriptors = append([]ProductDescriptor(nil), meta.ProductDescriptors...)
		updatedMeta.ProductDescriptors[0] = updated
		return payload, updated, updatedMeta
	}
	tests := map[string]func(*schemacachepb.SchemaProductCache){
		"nested-unknown-field": func(message *schemacachepb.SchemaProductCache) {
			message.Product.Tools.Items[0].Identity.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
		},
		"unknown-result-enum": func(message *schemacachepb.SchemaProductCache) {
			message.Product.Tools.Items[0].Result.Outcomes.Items[0] = schemacachepb.ResultOutcome(99)
		},
		"unspecified-result-enum": func(message *schemacachepb.SchemaProductCache) {
			message.Product.Tools.Items[0].Result.Outcomes.Items[0] = schemacachepb.ResultOutcome_RESULT_OUTCOME_UNSPECIFIED
		},
		"duplicate-parameter": func(message *schemacachepb.SchemaProductCache) {
			message.Product.Tools.Items[0].Parameters.Items = append(message.Product.Tools.Items[0].Parameters.Items, proto.Clone(message.Product.Tools.Items[0].Parameters.Items[0]).(*schemacachepb.ParameterSpec))
		},
		"unsorted-provenance": func(message *schemacachepb.SchemaProductCache) {
			items := message.Product.Tools.Items[0].FieldProvenance.Items
			items = append(items, proto.Clone(items[0]).(*schemacachepb.ProvenanceEntry))
			items[1].Key = "aaa"
			message.Product.Tools.Items[0].FieldProvenance.Items = items
		},
		"invalid-raw-json": func(message *schemacachepb.SchemaProductCache) {
			message.Product.Tools.Items[0].Parameters.Items[0].Example.Value = []byte("{")
		},
	}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			payload, updatedDescriptor, updatedMeta := mutate(t, edit)
			if _, err := DecodeSchemaProductCache(payload, updatedDescriptor, updatedMeta); err == nil {
				t.Fatal("DecodeSchemaProductCache() unexpectedly succeeded")
			}
		})
	}

	unauthenticated := descriptor
	unauthenticated.Offset++
	if _, err := DecodeSchemaProductCache(built.ProductShards, unauthenticated, meta); err == nil {
		t.Fatal("unauthenticated descriptor unexpectedly succeeded")
	}
	if _, _, err := ProductShardBounds(ProductDescriptor{ProductID: "x", Offset: math.MaxUint64, Length: 2}, 10); err == nil {
		t.Fatal("overflowing bounds unexpectedly succeeded")
	}
}

func buildFixtureCache(t *testing.T, registry SchemaRegistry) (BuiltSchemaCache, DecodedSchemaMeta) {
	t.Helper()
	overview, err := BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	built, err := BuildSchemaCache(registry, BuildCommandMetaLookup(registry), overview, locators, fixtureHashes())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := DecodeSchemaMetaCache(built.Meta)
	if err != nil {
		t.Fatal(err)
	}
	return built, meta
}

func fixtureHashes() CacheHashes {
	return CacheHashes{SourceSHA256: sha256.Sum256([]byte("source")), SurfaceSHA256: sha256.Sum256([]byte("surface"))}
}

func allFieldsRegistry() SchemaRegistry {
	selected, notSelected, reviewed, index := true, false, false, 0
	provenance := func(value json.RawMessage) contract.FieldProvenance {
		return contract.FieldProvenance{
			Value: value, Source: "contract_final", SourceRef: "source-ref", Precedence: "100", Resolution: "selected", ReviewReason: "reviewed",
			Candidates: []contract.FieldCandidateProvenance{
				{Value: cloneBytes(value), Source: "contract_final", SourceRef: "source-ref", Precedence: "100", ReviewReason: "winner", Selected: &selected},
				{Value: cloneBytes(value), Source: "other", SourceRef: "other-ref", Precedence: "10", ReviewReason: "loser", Selected: &notSelected},
			},
			OverriddenCandidates: []contract.FieldCandidateProvenance{{Value: cloneBytes(value), Source: "old", SourceRef: "old-ref", Precedence: "1", ReviewReason: "overridden", Selected: &notSelected}},
		}
	}
	tool := ToolSpec{
		Identity: contract.ToolIdentitySpec{
			ProductID: "sample", SourceProductID: "source", Name: "run", CLIName: "run", CanonicalPath: "sample.run", Path: "source.run",
			CLIPath: "sample group run", PrimaryCLIPath: "sample group run", Group: "sample group", Aliases: []string{"sample legacy run"}, IsAlias: false, Source: "runtime",
		},
		Display: "Sample Run", Title: "Run sample", Description: "Runs the sample", MetadataSource: "contract_final",
		Parameters: []ParameterSpec{{
			Name: "cursor", Type: "string", Description: "Cursor", Property: "request.cursor", Required: true, CLIRequired: true, RequiredWhen: "always",
			Default: nil, InterfaceDefault: json.RawMessage{}, Example: json.RawMessage("null"), Format: "token", Enum: []string{},
			InterfaceDescription: "RPC cursor", InterfaceType: "string", FieldProvenance: map[string]contract.FieldProvenance{"required": provenance(json.RawMessage("true"))},
		}},
		Constraints: contract.RuntimeSchemaConstraints{
			MutuallyExclusive: [][]string{{"cursor", "position"}}, RequireOneOf: [][]string{{"cursor"}}, RequireTogether: [][]string{},
		},
		Positionals: []contract.RuntimeSchemaPositional{{Name: "position", Type: "string", Description: "Position", Required: false, Variadic: true, Index: 0}},
		DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: true},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			DataSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"ID"}}}`), SensitivePaths: []string{"credential.secret"},
		},
		Pagination: &contract.PaginationSpec{
			Kind: contract.PaginationKindCursor, CursorParameter: "cursor", MetaPath: contract.PaginationMetaPath,
			EndpointExhaustedPath: contract.PaginationExhaustedPath, NextTokenPath: contract.PaginationNextTokenPath,
		},
		Safety:    contract.SafetySpec{Effect: "write", EffectSource: "declared", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Interface: contract.InterfaceSpec{Ref: &contract.InterfaceRefSpec{ProductID: "sample-rpc", RPCName: "Run"}, Mode: contract.InterfaceModeMCP, Availability: contract.InterfaceAvailable, Reason: "RPC implementation"},
		Selection: contract.SelectionSpec{
			AgentSummary: "Run a sample", AgentSummarySource: "contract", UseWhen: []string{"use sample"}, AvoidWhen: []string{},
			Prerequisites: nil, Tips: []string{}, WorkflowRefs: []string{"workflow"}, Examples: []string{"dws sample group run --cursor c"},
			ExampleDispositions: []contract.ExampleDisposition{{Index: &index, Mode: contract.ExampleDispositionModeContractOnly, ReasonCode: contract.ExampleDispositionReasonLocalState, Reason: "requires local state", Reviewed: true}},
			Reviewed:            &reviewed, SourceRefs: []string{"ref"}, MetadataSource: "contract_final",
		},
		FieldProvenance: map[string]contract.FieldProvenance{"title": provenance(json.RawMessage(`"Run sample"`))},
	}
	noop := ToolSpec{
		Identity: contract.ToolIdentitySpec{
			ProductID: "sample", Name: "zzz", CLIName: "zzz", CanonicalPath: "sample.zzz", Path: "sample.zzz",
			CLIPath: "sample zzz", PrimaryCLIPath: "sample zzz", Source: "runtime",
		},
		FieldProvenance: map[string]contract.FieldProvenance{},
	}
	return SchemaRegistry{
		Kind: "schema", Level: "catalog", Source: "runtime-assembled", AgentMetadata: json.RawMessage("null"),
		Products: []ProductSpec{{
			ID: "sample", Name: "Sample", Description: "Sample product", Runtime: true, Tools: []ToolSpec{tool, noop},
			Selection:       contract.SelectionSpec{AgentSummary: "Sample operations", UseWhen: []string{"sample work"}, AvoidWhen: []string{}},
			FieldProvenance: map[string]contract.FieldProvenance{"agent_summary": provenance(json.RawMessage(`"Sample operations"`))},
		}},
	}
}
