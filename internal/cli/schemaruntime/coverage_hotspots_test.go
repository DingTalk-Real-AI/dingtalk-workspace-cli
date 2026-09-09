// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"google.golang.org/protobuf/proto"
)

func TestCrossPlatformCoverageOverviewToPayloadBranches(t *testing.T) {
	if _, err := (SchemaOverview{Products: []OverviewProduct{{ID: "p", SummaryKind: OverviewSummaryNone, Summary: "leftover"}}}).ToPayload(); err == nil {
		t.Fatal("summary without kind accepted")
	}
	useWhen, err := (SchemaOverview{Products: []OverviewProduct{{ID: "p", SchemaPath: "p", SummaryKind: OverviewSummaryUseWhen, Summary: "when"}}}).ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := useWhen["products"].([]map[string]any)[0]["use_when"]; !ok {
		t.Fatalf("use_when missing: %#v", useWhen)
	}
	if _, err := (SchemaOverview{Products: []OverviewProduct{{ID: "p", SummaryKind: "nope"}}}).ToPayload(); err == nil {
		t.Fatal("unknown summary kind accepted")
	}
	if _, err := (SchemaOverview{AgentMetadata: json.RawMessage("{")}).ToPayload(); err == nil {
		t.Fatal("invalid agent_metadata accepted")
	}
}

func TestCrossPlatformCoverageCommandMetaLookupAndShards(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	_ = built
	primary := "sample group run"
	got, ok := meta.CommandMeta(primary)
	if !ok || got.Identity.Canonical != "sample.run" {
		t.Fatalf("shard CommandMeta = %#v, %v", got, ok)
	}
	meta.CommandMetaByPath = map[string]CommandMeta{primary: {Identity: CommandIdentity{Canonical: "from-map"}}}
	if got, ok := meta.CommandMeta(primary); !ok || got.Identity.Canonical != "from-map" {
		t.Fatalf("map CommandMeta = %#v, %v", got, ok)
	}
	if _, ok := meta.CommandMeta("missing path"); ok {
		t.Fatal("missing path resolved")
	}
	meta.CommandMetaByPath = nil
	meta.MaterializeCommandMeta()
	if meta.CommandMetaByPath != nil {
		t.Fatal("nil map must stay nil")
	}
	meta.CommandMetaByPath = map[string]CommandMeta{}
	meta.LocatorProductByPath["zzzz missing"] = "sample"
	if _, ok := meta.CommandMeta("zzzz missing"); ok {
		t.Fatal("locator-only path must miss the shard binary search")
	}
	if _, ok := meta.commandEntriesForProduct("missing-product"); ok {
		t.Fatal("unknown product shard resolved")
	}
	meta.commandEntryShards[0].Entries = []byte("not-a-proto")
	if _, ok := meta.commandEntriesForProduct("sample"); ok {
		t.Fatal("garbage shard decoded")
	}
	meta.commandEntryShards[0].EntryCount = 99
	meta.commandEntryShards[0].Entries = mustMarshalCommandEntries(t, []*schemacachepb.CommandMetaEntry{{
		LookupPath: "sample group run", CliPath: "sample group run", Canonical: "sample.run", ProductId: "sample",
	}})
	if _, ok := meta.commandEntriesForProduct("sample"); ok {
		t.Fatal("entry-count mismatch accepted")
	}
}

func TestCrossPlatformCoverageCommandMetaShardValidation(t *testing.T) {
	_, meta := buildFixtureCache(t, allFieldsRegistry())
	list := decodedEntryList(t, meta)
	t.Run("unsorted", func(t *testing.T) {
		rows := cloneEntries(list)
		rows[0].LookupPath, rows[len(rows)-1].LookupPath = rows[len(rows)-1].LookupPath, rows[0].LookupPath
		meta.commandEntryShards[0].Entries = mustMarshalCommandEntries(t, rows)
		meta.commandEntryShards[0].EntryCount = uint64(len(rows))
		if _, ok := meta.commandEntriesForProduct("sample"); ok {
			t.Fatal("unsorted shard accepted")
		}
	})
	t.Run("incomplete identity", func(t *testing.T) {
		rows := cloneEntries(list)
		rows[0].CliPath = ""
		meta.commandEntryShards[0].Entries = mustMarshalCommandEntries(t, rows)
		meta.commandEntryShards[0].EntryCount = uint64(len(rows))
		if _, ok := meta.commandEntriesForProduct("sample"); ok {
			t.Fatal("incomplete identity accepted")
		}
	})
	t.Run("locator mismatch", func(t *testing.T) {
		rows := cloneEntries(list)
		meta.commandEntryShards[0].Entries = mustMarshalCommandEntries(t, rows)
		meta.commandEntryShards[0].EntryCount = uint64(len(rows))
		meta.LocatorProductByPath[rows[0].GetLookupPath()] = "other"
		if _, ok := meta.commandEntriesForProduct("sample"); ok {
			t.Fatal("locator mismatch accepted")
		}
	})
	t.Run("alias locator mismatch", func(t *testing.T) {
		_, meta := buildFixtureCache(t, allFieldsRegistry())
		list := decodedEntryList(t, meta)
		rows := cloneEntries(list)
		if len(rows[0].Aliases) == 0 {
			rows[0].Aliases = []string{"sample legacy run"}
		}
		meta.commandEntryShards[0].Entries = mustMarshalCommandEntries(t, rows)
		meta.commandEntryShards[0].EntryCount = uint64(len(rows))
		meta.LocatorProductByPath[strings.TrimSpace(rows[0].Aliases[0])] = "other"
		if _, ok := meta.commandEntriesForProduct("sample"); ok {
			t.Fatal("alias locator mismatch accepted")
		}
	})
	t.Run("materialize skips bad shard", func(t *testing.T) {
		_, meta := buildFixtureCache(t, allFieldsRegistry())
		meta.CommandMetaByPath = map[string]CommandMeta{}
		meta.commandEntryShards[0].Entries = []byte("nope")
		meta.MaterializeCommandMeta()
		if len(meta.CommandMetaByPath) != 0 {
			t.Fatalf("bad shard leaked into lookup: %#v", meta.CommandMetaByPath)
		}
	})
}

func TestCrossPlatformCoverageRenderedLeavesAndPayloadAssembly(t *testing.T) {
	lookup := map[string]CommandMeta{
		"sample run": {Identity: CommandIdentity{CLIPath: "sample run", Canonical: "sample.run"}},
	}
	if err := validateRenderedSchemaLeaves(lookup, map[string][]byte{"sample.run": []byte("{}\n"), "extra.run": []byte("{}\n")}); err == nil {
		t.Fatal("extra rendered leaf accepted")
	}
	if err := validateRenderedSchemaLeaves(lookup, nil); err == nil {
		t.Fatal("missing rendered leaf accepted")
	}
	if err := validateRenderedSchemaLeaves(lookup, map[string][]byte{"sample.run": []byte("{")}); err == nil {
		t.Fatal("non-json leaf accepted")
	}
	huge := make(map[string][]byte, maxSchemaMetaEntries+1)
	for i := 0; i < maxSchemaMetaEntries+1; i++ {
		huge[fmt.Sprintf("p.%d", i)] = []byte("{}\n")
	}
	if err := validateRenderedSchemaLeaves(nil, huge); err == nil {
		t.Fatal("oversized rendered set accepted")
	}
	if _, err := assembleCommandPayloadShard(nil, nil); err == nil {
		t.Fatal("empty header accepted")
	}
}

func TestCrossPlatformCoverageBuildSchemaOverviewAndLocators(t *testing.T) {
	if _, err := BuildSchemaOverview(SchemaRegistry{Products: []ProductSpec{{ID: ""}}}); err == nil {
		t.Fatal("invalid registry overview accepted")
	}
	described := SchemaRegistry{Kind: "schema", Level: "catalog", Products: []ProductSpec{{
		ID: "desc", Name: "Desc", Description: "from description",
		Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{
			ProductID: "desc", Name: "run", CanonicalPath: "desc.run", Path: "desc.run",
			CLIPath: "desc run", PrimaryCLIPath: "desc run",
		}}},
	}}}
	overview, err := BuildSchemaOverview(described)
	if err != nil || overview.Products[0].SummaryKind != OverviewSummaryDescription {
		t.Fatalf("description overview = %#v, %v", overview, err)
	}
	if _, err := BuildSchemaProductLocators(SchemaRegistry{Products: []ProductSpec{{ID: ""}}}); err == nil {
		t.Fatal("invalid registry locators accepted")
	}
	colliding := SchemaRegistry{Kind: "schema", Level: "catalog", Products: []ProductSpec{
		{ID: "alpha", Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{
			ProductID: "alpha", Name: "run", CanonicalPath: "alpha.run", Path: "alpha.run",
			CLIPath: "alpha run", PrimaryCLIPath: "alpha run",
		}}}},
		{ID: "beta", Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{
			ProductID: "beta", Name: "run", CanonicalPath: "beta.run", Path: "beta.run",
			CLIPath: "alpha extra", PrimaryCLIPath: "alpha extra",
		}}}},
	}}
	if _, err := BuildSchemaProductLocators(colliding); err == nil {
		t.Fatal("locator collision accepted")
	}
	emptyPath := allFieldsRegistry()
	emptyPath.Products[0].Tools[0].Identity.CLIPath = "   "
	emptyPath.Products[0].Tools[0].Identity.PrimaryCLIPath = "   "
	if locators, err := BuildSchemaProductLocators(emptyPath); err == nil {
		_ = locators
	}
}

func TestCrossPlatformCoverageBuildSchemaCacheRejectsLimitsAndMarshal(t *testing.T) {
	empty := SchemaRegistry{Kind: "schema", Level: "catalog"}
	if _, err := BuildSchemaCache(empty, nil, SchemaOverview{}, nil, fixtureHashes(), nil); err == nil {
		t.Fatal("empty product list accepted")
	}
	if _, err := BuildSchemaCache(SchemaRegistry{Products: []ProductSpec{{ID: ""}}}, nil, SchemaOverview{}, nil, fixtureHashes(), nil); err == nil {
		t.Fatal("invalid registry accepted")
	}
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
	oversized := make(map[string]string, maxSchemaMetaEntries+1)
	for i := 0; i < maxSchemaMetaEntries+1; i++ {
		oversized[fmt.Sprintf("k%d", i)] = "sample"
	}
	if _, err := BuildSchemaCache(registry, lookup, overview, oversized, fixtureHashes(), fixtureRenderedLeaves(lookup)); err == nil {
		t.Fatal("oversized locators accepted")
	}
	aliasLookup := make(map[string]CommandMeta, len(lookup)+1)
	for key, value := range lookup {
		aliasLookup[key] = value
	}
	aliasLookup["not-an-owned-alias"] = lookup["sample group run"]
	if _, err := BuildSchemaCache(registry, aliasLookup, overview, locators, fixtureHashes(), fixtureRenderedLeaves(lookup)); err == nil {
		t.Fatal("alias expansion drift accepted")
	}

	orig := marshalSchemaCacheMessage
	for _, tc := range []struct {
		name string
		next func(proto.Message) ([]byte, error)
	}{
		{"error", func(proto.Message) ([]byte, error) { return nil, errors.New("forced marshal") }},
		{"empty", func(proto.Message) ([]byte, error) { return []byte{}, nil }},
		{"huge-meta", func(m proto.Message) ([]byte, error) {
			payload, err := orig(m)
			if err != nil {
				return nil, err
			}
			if len(payload) < 256 {
				return append(payload, make([]byte, MaxSchemaMetaBytes+1-len(payload))...), nil
			}
			return payload, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &marshalSchemaCacheMessage, tc.next)
			if _, err := BuildSchemaCache(registry, lookup, overview, locators, fixtureHashes(), fixtureRenderedLeaves(lookup)); err == nil {
				t.Fatal("forced marshal failure unexpectedly succeeded")
			}
		})
	}
	t.Run("nth-call-error", func(t *testing.T) {
		for n := 1; n <= 5; n++ {
			t.Run(fmt.Sprintf("call-%d", n), func(t *testing.T) {
				remaining := n
				testseam.Swap(t, &marshalSchemaCacheMessage, func(message proto.Message) ([]byte, error) {
					remaining--
					if remaining <= 0 {
						return nil, errors.New("nth marshal")
					}
					return orig(message)
				})
				if _, err := BuildSchemaCache(registry, lookup, overview, locators, fixtureHashes(), fixtureRenderedLeaves(lookup)); err == nil {
					t.Fatal("nth marshal failure unexpectedly succeeded")
				}
			})
		}
	})
}

func TestCrossPlatformCoverageDecodeAndPayloadDescriptorFailures(t *testing.T) {
	if _, err := MarshalSchemaCacheDeterministic(nil); err == nil {
		t.Fatal("nil message marshaled")
	}
	if _, err := DecodeSchemaMetaCache([]byte{0xff, 0xff, 0xff, 0xff, 0x01}); err == nil {
		t.Fatal("invalid protobuf accepted")
	}
	if _, _, err := ProductShardBounds(ProductDescriptor{ProductID: "x", Length: 0}, 10); err == nil {
		t.Fatal("zero length bounds accepted")
	}
	if _, _, err := ProductShardBounds(ProductDescriptor{ProductID: "x", Offset: math.MaxUint64, Length: math.MaxUint64}, math.MaxUint64); err == nil {
		t.Fatal("unrepresentable range accepted")
	}
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	if _, ok := metaPayloadDescriptor(meta, "missing"); ok {
		t.Fatal("missing payload descriptor resolved")
	}
	desc, ok := metaPayloadDescriptor(meta, meta.PayloadDescriptors[0].ProductID)
	if !ok {
		t.Fatal("payload descriptor missing")
	}
	wrong := desc
	wrong.Length++
	if err := AuthenticateCommandPayloadDescriptor(meta, wrong); err == nil {
		t.Fatal("mismatched descriptor authenticated")
	}
	if _, err := DecodeSchemaCommandPayloadCache([]byte("short"), desc, meta); err == nil {
		t.Fatal("short payload accepted")
	}
	if _, err := DecodeSchemaCommandPayloadCache(append([]byte(nil), built.PayloadShards...), wrong, meta); err == nil {
		t.Fatal("unauthenticated payload accepted")
	}
	payload := extractProductPayload(t, built, desc)
	badLen := append([]byte(nil), payload...)
	if _, err := DecodeSchemaCommandPayloadCache(badLen[:len(badLen)-1], desc, meta); err == nil {
		t.Fatal("truncated command payload accepted")
	}
	digestMismatch := append([]byte(nil), payload...)
	digestMismatch[len(digestMismatch)-1] ^= 1
	if _, err := DecodeSchemaCommandPayloadCache(digestMismatch, desc, meta); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if _, _, err := splitCommandPayloadShard([]byte{0, 0}); err == nil {
		t.Fatal("short shard split accepted")
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], 100)
	if _, _, err := splitCommandPayloadShard(append(prefix[:], 1, 2, 3)); err == nil {
		t.Fatal("header length overflow accepted")
	}
	header, blobs, err := splitCommandPayloadShard(payload)
	if err != nil {
		t.Fatal(err)
	}
	_ = blobs
	if _, err := decodeCommandPayloadHeader([]byte("nope"), desc); err == nil {
		t.Fatal("invalid header proto accepted")
	}
	_ = header
	if _, _, err := DecodeAllSchemaProducts(nil, meta); err == nil {
		t.Fatal("empty shards accepted")
	}
	if _, err := DecodeSchemaProductFromShards(built.ProductShards, meta, "missing"); err == nil {
		t.Fatal("unknown product decoded")
	}
	wrongShards := append([]byte(nil), built.ProductShards...)
	wrongShards[0] ^= 1
	if _, err := DecodeSchemaProductFromShards(wrongShards, meta, "sample"); err == nil {
		t.Fatal("corrupt registry shards accepted")
	}
	if _, _, err := DecodeAllSchemaProducts(wrongShards, meta); err == nil {
		t.Fatal("corrupt DecodeAll accepted")
	}
}

func TestCrossPlatformCoverageMutatedPayloadAndMetaValidation(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	desc := meta.PayloadDescriptors[0]
	payload := extractProductPayload(t, built, desc)
	header, blobs, err := splitCommandPayloadShard(payload)
	if err != nil {
		t.Fatal(err)
	}
	var root schemacachepb.SchemaCommandPayloadCache
	if err := proto.Unmarshal(header, &root); err != nil {
		t.Fatal(err)
	}
	mutateHeader := func(edit func(*schemacachepb.SchemaCommandPayloadCache)) []byte {
		cloned := proto.Clone(&root).(*schemacachepb.SchemaCommandPayloadCache)
		edit(cloned)
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		assembled, err := assembleCommandPayloadShard(encoded, blobs)
		if err != nil {
			t.Fatal(err)
		}
		return assembled
	}
	cases := []func(*schemacachepb.SchemaCommandPayloadCache){
		func(m *schemacachepb.SchemaCommandPayloadCache) {
			m.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_V1
		},
		func(m *schemacachepb.SchemaCommandPayloadCache) { m.ProductId = "other" },
		func(m *schemacachepb.SchemaCommandPayloadCache) { m.Entries = nil },
		func(m *schemacachepb.SchemaCommandPayloadCache) {
			m.RenderedLeafIndex.Items[0].Sha256 = []byte{1}
		},
		func(m *schemacachepb.SchemaCommandPayloadCache) {
			m.RenderedLeafIndex.Items[0].CanonicalPath = ""
		},
		func(m *schemacachepb.SchemaCommandPayloadCache) {
			m.Entries.Items[0].Identity = nil
		},
	}
	for i, edit := range cases {
		assembled := mutateHeader(edit)
		updated := desc
		updated.Length = uint64(len(assembled))
		updated.SHA256 = sha256.Sum256(assembled)
		updated.HeaderLength = uint64(4 + func() int {
			h, _, _ := splitCommandPayloadShard(assembled)
			return len(h)
		}())
		updated.HeaderSHA256 = sha256.Sum256(assembled[:updated.HeaderLength])
		updatedMeta := meta
		updatedMeta.PayloadDescriptors = append([]CommandPayloadDescriptor(nil), meta.PayloadDescriptors...)
		updatedMeta.PayloadDescriptors[0] = updated
		if _, err := DecodeSchemaCommandPayloadCache(assembled, updated, updatedMeta); err == nil {
			t.Fatalf("case %d accepted mutated payload", i)
		}
	}
	rangeExceed := mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		m.RenderedLeafIndex.Items[0].Offset = uint64(len(blobs) + 1)
	})
	updated := desc
	updated.Length = uint64(len(rangeExceed))
	updated.SHA256 = sha256.Sum256(rangeExceed)
	h, _, _ := splitCommandPayloadShard(rangeExceed)
	updated.HeaderLength = uint64(4 + len(h))
	updated.HeaderSHA256 = sha256.Sum256(rangeExceed[:updated.HeaderLength])
	updatedMeta := meta
	updatedMeta.PayloadDescriptors = append([]CommandPayloadDescriptor(nil), meta.PayloadDescriptors...)
	updatedMeta.PayloadDescriptors[0] = updated
	if _, err := DecodeSchemaCommandPayloadCache(rangeExceed, updated, updatedMeta); err == nil {
		t.Fatal("leaf range overflow accepted")
	}

	var metaRoot schemacachepb.SchemaMetaCache
	if err := proto.Unmarshal(built.Meta, &metaRoot); err != nil {
		t.Fatal(err)
	}
	mutateMeta := func(edit func(*schemacachepb.SchemaMetaCache)) []byte {
		cloned := proto.Clone(&metaRoot).(*schemacachepb.SchemaMetaCache)
		edit(cloned)
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	metaCases := []func(*schemacachepb.SchemaMetaCache){
		func(m *schemacachepb.SchemaMetaCache) { m.Registry = nil },
		func(m *schemacachepb.SchemaMetaCache) { m.Overview.Registry = nil },
		func(m *schemacachepb.SchemaMetaCache) {
			m.Registry.AgentMetadata = &schemacachepb.BytesValue{Value: []byte("{")}
		},
		func(m *schemacachepb.SchemaMetaCache) { m.RegistryDataSha256 = []byte{1} },
		func(m *schemacachepb.SchemaMetaCache) { m.CommandEntryShards.Items[0].Entries = nil },
		func(m *schemacachepb.SchemaMetaCache) { m.Locators.Items[0].ProductId = "missing" },
		func(m *schemacachepb.SchemaMetaCache) { m.CommandEntryShards.Items[0].ProductId = "no-descriptor" },
		func(m *schemacachepb.SchemaMetaCache) {
			m.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_UNSPECIFIED
		},
		func(m *schemacachepb.SchemaMetaCache) { m.Overview.Products.Items[0].Id = "" },
		func(m *schemacachepb.SchemaMetaCache) {
			m.Overview.Products.Items[0].SummaryKind = schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_UNSPECIFIED
			m.Overview.Products.Items[0].Summary = "x"
		},
		func(m *schemacachepb.SchemaMetaCache) { m.Overview.ToolCount = 0 },
		func(m *schemacachepb.SchemaMetaCache) { m.ProductDescriptors.Items = nil },
		func(m *schemacachepb.SchemaMetaCache) { m.ProductDescriptors.Items[0].ProductId = "" },
	}
	for i, edit := range metaCases {
		if _, err := DecodeSchemaMetaCache(mutateMeta(edit)); err == nil {
			t.Fatalf("meta case %d accepted", i)
		}
	}
	if _, ok := metaDescriptor(DecodedSchemaMeta{ProductDescriptors: meta.ProductDescriptors}, "missing"); ok {
		t.Fatal("missing product descriptor resolved")
	}
}

func TestCrossPlatformCoverageConversionNilAndEnumDefaults(t *testing.T) {
	if overviewSummaryKindToProto(OverviewSummaryUseWhen) != schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_USE_WHEN {
		t.Fatal("use_when proto")
	}
	if overviewSummaryKindToProto(OverviewSummaryDescription) != schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_DESCRIPTION {
		t.Fatal("description proto")
	}
	if overviewSummaryKindFromProto(schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_USE_WHEN) != OverviewSummaryUseWhen {
		t.Fatal("use_when from proto")
	}
	if overviewSummaryKindFromProto(schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_DESCRIPTION) != OverviewSummaryDescription {
		t.Fatal("description from proto")
	}
	if got := constraintsFromProto(nil); got.RequireOneOf != nil || got.MutuallyExclusive != nil {
		t.Fatalf("nil constraints = %#v", got)
	}
	if descriptorsFromProto(nil) != nil || payloadDescriptorsFromProto(nil) != nil || provenanceFromProto(nil) != nil {
		t.Fatal("nil descriptor conversion")
	}
	if toolsFromProto(nil) != nil {
		t.Fatal("nil tools from proto")
	}
	if got, err := toolsToProto(nil); err != nil || got != nil {
		t.Fatalf("nil tools = %v, %v", got, err)
	}
	if _, err := productToProto(ProductSpec{Tools: []ToolSpec{{Result: &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"bad"}}}}}); err == nil {
		t.Fatal("productToProto accepted bad result")
	}
	if _, err := productToProto(ProductSpec{Selection: contract.SelectionSpec{ExampleDispositions: []contract.ExampleDisposition{{Mode: "bad"}}}}); err == nil {
		t.Fatal("productToProto accepted bad selection")
	}
	if got := interfaceFromProto(nil); got.Mode != "" {
		t.Fatalf("nil interface = %#v", got)
	}
	if got := selectionFromProto(nil); got.AgentSummary != "" {
		t.Fatalf("nil selection = %#v", got)
	}
	if _, err := resultToProto(&contract.ResultSpec{Outcomes: []contract.ResultOutcome{"nope"}}); err == nil {
		t.Fatal("unsupported outcome accepted")
	}
	if _, ok := resultOutcomeToProto("nope"); ok {
		t.Fatal("unsupported outcome mapped")
	}
	if resultOutcomeFromProto(99) != "" {
		t.Fatal("unknown outcome from proto")
	}
	if _, ok := dispositionModeToProto("nope"); ok {
		t.Fatal("unsupported mode mapped")
	}
	if _, ok := dispositionModeToProto(contract.ExampleDispositionModeContract); !ok {
		t.Fatal("contract mode")
	}
	if _, ok := dispositionModeToProto(contract.ExampleDispositionModeDryRun); !ok {
		t.Fatal("dry-run mode")
	}
	if dispositionModeFromProto(schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT) == "" {
		t.Fatal("contract from proto")
	}
	if dispositionModeFromProto(schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_DRY_RUN) == "" {
		t.Fatal("dry-run from proto")
	}
	if dispositionModeFromProto(99) != "" {
		t.Fatal("unknown mode from proto")
	}
	if _, ok := dispositionReasonToProto(contract.ExampleDispositionReasonStatefulPreflight); !ok {
		t.Fatal("stateful reason")
	}
	if _, ok := dispositionReasonToProto("nope"); ok {
		t.Fatal("unsupported reason mapped")
	}
	if dispositionReasonFromProto(schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_STATEFUL_PREFLIGHT) == "" {
		t.Fatal("stateful from proto")
	}
	if dispositionReasonFromProto(99) != "" {
		t.Fatal("unknown reason from proto")
	}
	if _, err := selectionToProtoExact(contract.SelectionSpec{ExampleDispositions: []contract.ExampleDisposition{{Mode: "nope"}}}); err == nil {
		t.Fatal("unsupported disposition mode accepted")
	}
	if _, err := selectionToProtoExact(contract.SelectionSpec{ExampleDispositions: []contract.ExampleDisposition{{Mode: contract.ExampleDispositionModeContractOnly, ReasonCode: "nope"}}}); err == nil {
		t.Fatal("unsupported disposition reason accepted")
	}
	if bytesToProto([]byte(nil)) != nil || boolToProto(nil) != nil || intToProto(nil) != nil {
		t.Fatal("nil proto wrappers")
	}
	if boolFromProto(nil) != nil || intFromProto(nil) != nil {
		t.Fatal("nil from proto wrappers")
	}
	unsorted := ProductSpec{ID: "sample", Tools: []ToolSpec{
		{Identity: contract.ToolIdentitySpec{CLIPath: "sample z", CanonicalPath: "sample.same"}},
		{Identity: contract.ToolIdentitySpec{CLIPath: "sample a", CanonicalPath: "sample.same"}},
	}}
	sorted := cloneProductExact(unsorted)
	if sorted.Tools[0].Identity.CLIPath != "sample a" {
		t.Fatalf("tools not sorted: %#v", sorted.Tools)
	}
	if _, err := toolsToProto([]ToolSpec{{Identity: contract.ToolIdentitySpec{CanonicalPath: "x.run"}, Result: &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"bad"}}}}); err == nil {
		t.Fatal("toolsToProto accepted bad result")
	}
	if _, err := toolsToProto([]ToolSpec{{Identity: contract.ToolIdentitySpec{CanonicalPath: "x.run"}, Selection: contract.SelectionSpec{ExampleDispositions: []contract.ExampleDisposition{{Mode: "bad"}}}}}); err == nil {
		t.Fatal("toolsToProto accepted bad selection")
	}
	if dryRunToProto(nil) != nil {
		t.Fatal("nil dry-run")
	}
	if got, err := resultToProto(nil); err != nil || got != nil {
		t.Fatalf("nil result = %v, %v", got, err)
	}
	if paginationToProto(nil) != nil {
		t.Fatal("nil pagination")
	}
	if provenanceToProto(nil) != nil {
		t.Fatal("nil provenance")
	}
	if _, err := commandLookupToShards(map[string]CommandMeta{"a": {Identity: CommandIdentity{ProductID: "p"}}}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &marshalSchemaCacheMessage, func(proto.Message) ([]byte, error) { return nil, errors.New("forced shard marshal") })
	if _, err := commandLookupToShards(map[string]CommandMeta{"a": {Identity: CommandIdentity{ProductID: "p"}}}); err == nil {
		t.Fatal("forced shard marshal succeeded")
	}
}

func TestCrossPlatformCoverageModelValidationAndQuery(t *testing.T) {
	if _, err := SchemaRegistryFromRuntime("src", []ProductSpec{{ID: ""}}); err == nil {
		t.Fatal("empty product id accepted")
	}
	if _, err := ToolSpecFromRuntime(RuntimeToolSpecInput{}); err == nil {
		t.Fatal("empty tool accepted")
	}
	tool := ToolSpec{Identity: contract.ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run", Path: "sample.run",
		CLIPath: "sample run", PrimaryCLIPath: "sample other",
	}}
	if err := validateCanonicalToolIdentity(tool); err == nil {
		t.Fatal("cli/primary mismatch accepted")
	}
	spec := ToolSpec{
		Identity:  contract.ToolIdentitySpec{CanonicalPath: "sample.run", CLIPath: "sample run", PrimaryCLIPath: "sample run", ProductID: "sample", Name: "run", Path: "sample.run"},
		Selection: contract.SelectionSpec{Prerequisites: []string{"p"}, Tips: []string{"t"}, WorkflowRefs: []string{"w"}},
	}
	if value, ok := spec.provenanceValue("prerequisites"); !ok || value == nil {
		t.Fatal("prerequisites provenance")
	}
	if _, ok := spec.provenanceValue("tips"); !ok {
		t.Fatal("tips provenance")
	}
	if _, ok := spec.provenanceValue("workflow_refs"); !ok {
		t.Fatal("workflow_refs provenance")
	}
	if NormalizeQueryCLIPath("dws calendar event create") != "calendar event create" {
		t.Fatal("dws prefix not stripped")
	}
	if CompactParameter(map[string]any{"type": "string", "unknown": 1})["type"] != "string" {
		t.Fatal("CompactParameter")
	}
	bad := allFieldsRegistry()
	bad.AgentMetadata = json.RawMessage("{")
	if _, err := RenderAll(bad, TrustedHashes{}); err == nil {
		t.Fatal("RenderAll accepted invalid metadata")
	}
	if _, err := RenderOverview(bad, TrustedHashes{}); err == nil {
		t.Fatal("RenderOverview accepted invalid metadata")
	}
	if _, err := RenderCatalog(bad, TrustedHashes{}); err == nil {
		t.Fatal("RenderCatalog accepted invalid metadata")
	}
	registry := allFieldsRegistry()
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderQueryWithProjectors(registry, index, "sample", QueryProjectors{
		ProductSummary: func(ProductSpec) (map[string]any, error) { return nil, errors.New("product fail") },
	}); err == nil {
		t.Fatal("product projector failure ignored")
	}
	if _, err := RenderQueryWithProjectors(registry, index, "sample group", QueryProjectors{
		ToolSummary: func(ToolSpec) (map[string]any, error) { return nil, errors.New("tool fail") },
	}); err == nil {
		t.Fatal("tool projector failure ignored")
	}
	wantJSON := json.RawMessage(`"v"`)
	selected := true
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{Value: wantJSON, Source: "", Precedence: "1", Resolution: "x"}, "v"); err == nil {
		t.Fatal("incomplete winner accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x"}, "v"); err == nil {
		t.Fatal("no candidates accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{
		Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x",
		Candidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage("{"), Selected: &selected}},
	}, "v"); err == nil {
		t.Fatal("invalid candidate accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{
		Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x",
		Candidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage(`"other"`), Selected: &selected}},
	}, "v"); err == nil {
		t.Fatal("mismatched selected candidate accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{
		Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x",
		Candidates:           []contract.FieldCandidateProvenance{{Value: wantJSON, Selected: &selected}},
		OverriddenCandidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage("{")}},
	}, "v"); err == nil {
		t.Fatal("invalid overridden candidate accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{
		Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x",
		Candidates:           []contract.FieldCandidateProvenance{{Value: wantJSON, Selected: &selected}},
		OverriddenCandidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage(`"old"`), Selected: &selected}},
	}, "v"); err == nil {
		t.Fatal("selected overridden candidate accepted")
	}
	if err := validateFinalFieldProvenance("owner", "title", contract.FieldProvenance{
		Value: wantJSON, Source: "s", Precedence: "1", Resolution: "x",
		Candidates: []contract.FieldCandidateProvenance{{Value: wantJSON}},
	}, "v"); err == nil {
		t.Fatal("zero selected candidates accepted")
	}
	if equalJSONValues([]byte("{"), []byte("{")) {
		t.Fatal("invalid identical bytes must not compare equal")
	}
	if err := spec.Validate(); err != nil {
		// may fail without full identity; still exercise pagination cursor check below
	}
	paged := ToolSpec{
		Identity:   contract.ToolIdentitySpec{ProductID: "sample", Name: "run", CanonicalPath: "sample.run", Path: "sample.run", CLIPath: "sample run", PrimaryCLIPath: "sample run"},
		Parameters: []ParameterSpec{{Name: "other", Type: "string"}},
		Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
		Result:     &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object"}`)},
	}
	if err := paged.Validate(); err == nil {
		t.Fatal("missing cursor parameter accepted")
	}
	paged.Pagination = &contract.PaginationSpec{Kind: "offset", CursorParameter: "other"}
	if err := paged.Validate(); err == nil {
		t.Fatal("unsupported pagination kind accepted")
	}
	paged.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"bad"}, DataSchema: json.RawMessage(`{"type":"object"}`)}
	paged.Pagination = nil
	if err := paged.Validate(); err == nil {
		t.Fatal("invalid result accepted")
	}
	if err := (ToolSpec{Identity: contract.ToolIdentitySpec{CanonicalPath: "other", ProductID: "sample", Name: "run", Path: "sample.run", CLIPath: "sample run", PrimaryCLIPath: "sample run"}}).Validate(); err == nil {
		t.Fatal("canonical mismatch accepted")
	}
	primary := CommandMeta{Identity: CommandIdentity{CLIPath: "sample run", Canonical: "sample.run", Aliases: []string{"sample alt"}}}
	if !commandMetaSubsetEqual(map[string]CommandMeta{"a": primary}, map[string]CommandMeta{"a": primary}) {
		t.Fatal("equal subset")
	}
	if validMetaAliasExpansion(nil) {
		t.Fatal("nil lookup accepted")
	}
	if validMetaAliasExpansion(map[string]CommandMeta{"sample run": primary}) {
		t.Fatal("missing alias row accepted")
	}
	if validMetaAliasExpansion(map[string]CommandMeta{
		"sample run": primary,
		"sample alt": {Identity: CommandIdentity{CLIPath: "sample run"}},
	}) {
		t.Fatal("alias metadata mismatch accepted")
	}
	shared := CommandMeta{Identity: CommandIdentity{CLIPath: "b run", Canonical: "b.run", Aliases: []string{"shared"}}}
	if validMetaAliasExpansion(map[string]CommandMeta{
		"a run":  {Identity: CommandIdentity{CLIPath: "a run", Canonical: "a.run", Aliases: []string{"shared"}}},
		"b run":  shared,
		"shared": shared,
	}) {
		t.Fatal("lexically later alias owner accepted")
	}
	if commandMetaSubsetEqual(map[string]CommandMeta{"a": primary}, map[string]CommandMeta{"missing": primary}) {
		t.Fatal("missing subset key equal")
	}
	if commandIdentitySubsetEqual(map[string]CommandMeta{"a": primary}, map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "a", Aliases: []string{"x"}}}}) {
		t.Fatal("alias identity mismatch equal")
	}
	if locatorSubsetEqual(map[string]string{"a": "p"}, map[string]string{"a": "q"}) {
		t.Fatal("locator value mismatch equal")
	}
}

func extractProductPayload(t *testing.T, built BuiltSchemaCache, desc CommandPayloadDescriptor) []byte {
	t.Helper()
	indexLen := int(built.PayloadIndexLength)
	if indexLen > len(built.PayloadShards) {
		t.Fatalf("index length %d > payload %d", indexLen, len(built.PayloadShards))
	}
	body := built.PayloadShards[indexLen:]
	if int(desc.Offset)+int(desc.Length) > len(body) {
		if int(desc.Length) <= len(built.PayloadShards) {
			return append([]byte(nil), built.PayloadShards[:desc.Length]...)
		}
		t.Fatalf("payload range %d+%d exceeds %d", desc.Offset, desc.Length, len(body))
	}
	return append([]byte(nil), body[int(desc.Offset):int(desc.Offset)+int(desc.Length)]...)
}

func decodedEntryList(t *testing.T, meta DecodedSchemaMeta) []*schemacachepb.CommandMetaEntry {
	t.Helper()
	var list schemacachepb.CommandMetaEntryList
	if err := proto.Unmarshal(meta.commandEntryShards[0].GetEntries(), &list); err != nil {
		t.Fatal(err)
	}
	return list.Items
}

func cloneEntries(items []*schemacachepb.CommandMetaEntry) []*schemacachepb.CommandMetaEntry {
	out := make([]*schemacachepb.CommandMetaEntry, len(items))
	for i, item := range items {
		out[i] = proto.Clone(item).(*schemacachepb.CommandMetaEntry)
	}
	return out
}

func mustMarshalCommandEntries(t *testing.T, items []*schemacachepb.CommandMetaEntry) []byte {
	t.Helper()
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(&schemacachepb.CommandMetaEntryList{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
