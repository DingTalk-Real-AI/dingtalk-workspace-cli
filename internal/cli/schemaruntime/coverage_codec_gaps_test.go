package schemaruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestCrossPlatformCoverageCommandMetaMapAndLocatorCollisions(t *testing.T) {
	_, meta := buildFixtureCache(t, allFieldsRegistry())
	meta.commandEntryShards[0].Entries = []byte("not-a-proto")
	if _, ok := meta.commandMetaMapForProduct("sample"); ok {
		t.Fatal("garbage shard produced a command meta map")
	}

	alpha := allFieldsRegistry()
	alpha.Products[0].ID = "alpha"
	alpha.Products[0].Tools[0].Identity.ProductID = "alpha"
	alpha.Products[0].Tools[0].Identity.CanonicalPath = "alpha.run"
	alpha.Products[0].Tools[0].Identity.CLIPath = "beta run"
	alpha.Products[0].Tools[0].Identity.PrimaryCLIPath = "beta run"
	alpha.Products[0].Tools[0].Identity.Group = "beta"
	alpha.Products[0].Tools[0].Identity.Aliases = nil
	alpha.Products[0].Tools[1].Identity.ProductID = "alpha"
	alpha.Products[0].Tools[1].Identity.CanonicalPath = "alpha.zzz"
	alpha.Products[0].Tools[1].Identity.Path = "alpha.zzz"
	alpha.Products[0].Tools[1].Identity.CLIPath = "alpha zzz"
	alpha.Products[0].Tools[1].Identity.PrimaryCLIPath = "alpha zzz"
	alpha.Products[0].FieldProvenance = nil
	beta := allFieldsRegistry().Products[0]
	beta.ID = "beta"
	beta.Name = "Beta"
	beta.FieldProvenance = nil
	beta.Selection = contract.SelectionSpec{}
	copied := beta.Tools[0]
	copied.Identity.ProductID = "beta"
	copied.Identity.Name = "other"
	copied.Identity.CLIName = "other"
	copied.Identity.CanonicalPath = "beta.other"
	copied.Identity.Path = "beta.other"
	copied.Identity.CLIPath = "beta other"
	copied.Identity.PrimaryCLIPath = "beta other"
	copied.Identity.Aliases = nil
	copied.Identity.Group = ""
	copied.FieldProvenance = nil
	zzz := beta.Tools[1]
	zzz.Identity.ProductID = "beta"
	zzz.Identity.CanonicalPath = "beta.zzz"
	zzz.Identity.Path = "beta.zzz"
	zzz.Identity.CLIPath = "beta zzz"
	zzz.Identity.PrimaryCLIPath = "beta zzz"
	beta.Tools = []ToolSpec{copied, zzz}
	alpha.Products = append(alpha.Products, beta)
	if _, err := alpha.Index(); err != nil {
		t.Fatalf("colliding locator registry must still Index: %v", err)
	}
	if _, err := BuildSchemaProductLocators(alpha); err == nil {
		t.Fatal("product ID collision via path prefix accepted")
	}
	if _, err := buildSchemaProductLocatorsUnchecked(alpha); err == nil {
		t.Fatal("unchecked product ID collision accepted")
	}

	dot := allFieldsRegistry()
	dot.Products[0].Tools[0].Identity.Aliases = append(append([]string(nil), dot.Products[0].Tools[0].Identity.Aliases...), "other.group extra")
	other := allFieldsRegistry().Products[0]
	other.ID = "other"
	other.Name = "Other"
	other.FieldProvenance = nil
	other.Selection = contract.SelectionSpec{}
	ot := other.Tools[0]
	ot.Identity.ProductID = "other"
	ot.Identity.Name = "group"
	ot.Identity.CLIName = "group"
	ot.Identity.CanonicalPath = "other.group"
	ot.Identity.Path = "other.group"
	ot.Identity.CLIPath = "other group"
	ot.Identity.PrimaryCLIPath = "other group"
	ot.Identity.Aliases = nil
	ot.Identity.Group = "other group"
	ot.FieldProvenance = nil
	oz := other.Tools[1]
	oz.Identity.ProductID = "other"
	oz.Identity.Name = "zzz"
	oz.Identity.CanonicalPath = "other.zzz"
	oz.Identity.Path = "other.zzz"
	oz.Identity.CLIPath = "other zzz"
	oz.Identity.PrimaryCLIPath = "other zzz"
	other.Tools = []ToolSpec{ot, oz}
	dot.Products = append(dot.Products, other)
	if _, err := buildSchemaProductLocatorsUnchecked(dot); err == nil {
		t.Fatal("dot-prefix locator collision accepted")
	}
}

func TestCrossPlatformCoverageBuildSchemaCacheProjectionDrift(t *testing.T) {
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
	rendered := fixtureRenderedLeaves(lookup)
	drifted := make(map[string]CommandMeta, len(lookup))
	for key, value := range lookup {
		drifted[key] = value
	}
	primary := drifted["sample group run"]
	primary.Identity.Title = "drifted"
	drifted["sample group run"] = primary
	if _, err := BuildSchemaCache(registry, drifted, overview, locators, fixtureHashes(), rendered); err == nil {
		t.Fatal("lookup drift accepted")
	}
	wrongOverview := overview
	wrongOverview.Kind = "other"
	if _, err := BuildSchemaCache(registry, lookup, wrongOverview, locators, fixtureHashes(), rendered); err == nil {
		t.Fatal("overview drift accepted")
	}
	wrongLocators := make(map[string]string, len(locators))
	for key, value := range locators {
		wrongLocators[key] = value
	}
	wrongLocators["sample group run"] = "other"
	if _, err := BuildSchemaCache(registry, lookup, overview, wrongLocators, fixtureHashes(), rendered); err == nil {
		t.Fatal("locator drift accepted")
	}

	badProduct := allFieldsRegistry()
	badProduct.Products[0].Selection.ExampleDispositions = []contract.ExampleDisposition{{Mode: "not-a-mode", ReasonCode: contract.ExampleDispositionReasonLocalState, Reason: "x", Reviewed: true}}
	badLookup := BuildCommandMetaLookup(badProduct)
	badOverview, err := BuildSchemaOverview(badProduct)
	if err != nil {
		t.Fatal(err)
	}
	badLocators, err := BuildSchemaProductLocators(badProduct)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSchemaCache(badProduct, badLookup, badOverview, badLocators, fixtureHashes(), fixtureRenderedLeaves(badLookup)); err == nil {
		t.Fatal("unsupported disposition converted")
	}

	colliding := allFieldsRegistry()
	colliding.Products[0].Tools[0].Identity.CLIPath = "beta run"
	colliding.Products[0].Tools[0].Identity.PrimaryCLIPath = "beta run"
	colliding.Products[0].Tools[0].Identity.Group = "beta"
	colliding.Products[0].Tools[0].Identity.Aliases = nil
	second := allFieldsRegistry().Products[0]
	second.ID = "beta"
	second.FieldProvenance = nil
	second.Selection = contract.SelectionSpec{}
	st := second.Tools[0]
	st.Identity.ProductID = "beta"
	st.Identity.Name = "other"
	st.Identity.CLIName = "other"
	st.Identity.CanonicalPath = "beta.other"
	st.Identity.Path = "beta.other"
	st.Identity.CLIPath = "beta other"
	st.Identity.PrimaryCLIPath = "beta other"
	st.Identity.Aliases = nil
	st.FieldProvenance = nil
	sz := second.Tools[1]
	sz.Identity.ProductID = "beta"
	sz.Identity.CanonicalPath = "beta.zzz"
	sz.Identity.Path = "beta.zzz"
	sz.Identity.CLIPath = "beta zzz"
	sz.Identity.PrimaryCLIPath = "beta zzz"
	second.Tools = []ToolSpec{st, sz}
	colliding.Products = append(colliding.Products, second)
	collidingLookup := BuildCommandMetaLookup(colliding)
	collidingOverview, err := BuildSchemaOverview(colliding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSchemaCache(colliding, collidingLookup, collidingOverview, map[string]string{"alpha": "alpha"}, fixtureHashes(), fixtureRenderedLeaves(collidingLookup)); err == nil {
		t.Fatal("locator collision during cache build accepted")
	}

	orig := marshalSchemaCacheMessage
	testseam.Swap(t, &marshalSchemaCacheMessage, func(message proto.Message) ([]byte, error) {
		switch message.(type) {
		case *schemacachepb.SchemaCommandPayloadCache:
			return nil, nil
		default:
			return orig(message)
		}
	})
	if _, err := BuildSchemaCache(registry, lookup, overview, locators, fixtureHashes(), rendered); err == nil {
		t.Fatal("empty command payload header accepted")
	}
	testseam.Swap(t, &marshalSchemaCacheMessage, func(message proto.Message) ([]byte, error) {
		switch message.(type) {
		case *schemacachepb.SchemaCommandPayloadCache:
			return make([]byte, MaxSchemaProductBytes), nil
		default:
			return orig(message)
		}
	})
	if _, err := BuildSchemaCache(registry, lookup, overview, locators, fixtureHashes(), rendered); err == nil {
		t.Fatal("oversized command payload shard accepted")
	}
	testseam.Swap(t, &marshalSchemaCacheMessage, func(message proto.Message) ([]byte, error) {
		switch message.(type) {
		case *schemacachepb.SchemaPayloadIndex:
			return nil, nil
		default:
			return orig(message)
		}
	})
	if _, err := BuildSchemaCache(registry, lookup, overview, locators, fixtureHashes(), rendered); err == nil {
		t.Fatal("empty payload index accepted")
	}

	many := allFieldsRegistry()
	for i := 0; i < 8; i++ {
		p := allFieldsRegistry().Products[0]
		id := "p" + string(rune('a'+i))
		p.ID = id
		p.Name = id
		p.FieldProvenance = nil
		p.Selection = contract.SelectionSpec{}
		for j := range p.Tools {
			p.Tools[j].Identity.ProductID = id
			p.Tools[j].Identity.SourceProductID = ""
			p.Tools[j].Identity.CanonicalPath = id + "." + p.Tools[j].Identity.Name
			p.Tools[j].Identity.Path = p.Tools[j].Identity.CanonicalPath
			p.Tools[j].Identity.CLIPath = id + " " + p.Tools[j].Identity.Name
			p.Tools[j].Identity.PrimaryCLIPath = p.Tools[j].Identity.CLIPath
			p.Tools[j].Identity.Aliases = nil
			p.Tools[j].Identity.Group = ""
			p.Tools[j].FieldProvenance = nil
		}
		many.Products = append(many.Products, p)
	}
	manyLookup := BuildCommandMetaLookup(many)
	manyOverview, err := BuildSchemaOverview(many)
	if err != nil {
		t.Fatal(err)
	}
	manyLocators, err := BuildSchemaProductLocators(many)
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &marshalSchemaCacheMessage, func(message proto.Message) ([]byte, error) {
		if _, ok := message.(*schemacachepb.SchemaProductCache); ok {
			return make([]byte, MaxSchemaProductBytes), nil
		}
		return orig(message)
	})
	if _, err := BuildSchemaCache(many, manyLookup, manyOverview, manyLocators, fixtureHashes(), fixtureRenderedLeaves(manyLookup)); err == nil {
		t.Fatal("oversized registry shard data accepted")
	}
}

func TestCrossPlatformCoverageDecodeProductAndIndexMutationsRemaining(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	desc := meta.ProductDescriptors[0]
	offset, length, err := ProductShardBounds(desc, uint64(len(built.ProductShards)))
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), built.ProductShards[int(offset):int(offset)+length]...)
	garbage := []byte{0xff, 0xff, 0x01}
	garbageDesc := desc
	garbageDesc.Length = uint64(len(garbage))
	garbageDesc.SHA256 = sha256.Sum256(garbage)
	garbageMeta := meta
	garbageMeta.ProductDescriptors = append([]ProductDescriptor(nil), meta.ProductDescriptors...)
	garbageMeta.ProductDescriptors[0] = garbageDesc
	if _, err := DecodeSchemaProductCache(garbage, garbageDesc, garbageMeta); err == nil {
		t.Fatal("invalid product protobuf accepted")
	}

	var root schemacachepb.SchemaProductCache
	if err := proto.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	patchProduct := func(edit func(*schemacachepb.SchemaProductCache), also func(*DecodedSchemaMeta)) {
		t.Helper()
		cloned := proto.Clone(&root).(*schemacachepb.SchemaProductCache)
		edit(cloned)
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		updated := desc
		updated.Length = uint64(len(payload))
		updated.SHA256 = sha256.Sum256(payload)
		patched := meta
		patched.ProductDescriptors = append([]ProductDescriptor(nil), meta.ProductDescriptors...)
		patched.ProductDescriptors[0] = updated
		if also != nil {
			also(&patched)
		}
		if _, err := DecodeSchemaProductCache(payload, updated, patched); err == nil {
			t.Fatal("mutated product accepted")
		}
	}
	patchProduct(func(m *schemacachepb.SchemaProductCache) {
		m.DtoVersion = schemacachepb.DTOVersion(99)
	}, nil)
	patchProduct(func(m *schemacachepb.SchemaProductCache) {
		m.Product.Tools.Items[0].Identity.CliPath = "drifted path"
	}, nil)
	patchProduct(func(m *schemacachepb.SchemaProductCache) {}, func(meta *DecodedSchemaMeta) {
		meta.commandEntryShards[0].Entries = []byte("nope")
	})
	patchProduct(func(m *schemacachepb.SchemaProductCache) {}, func(meta *DecodedSchemaMeta) {
		for path := range meta.LocatorProductByPath {
			meta.LocatorProductByPath[path] = "other"
		}
		meta.locatorCountByProduct["sample"] = 0
	})
	patchProduct(func(m *schemacachepb.SchemaProductCache) {}, func(meta *DecodedSchemaMeta) {
		meta.Overview.Products[0].ToolCount = 0
	})

	indexRegion := append([]byte(nil), built.PayloadShards[:built.PayloadIndexLength]...)
	header, _, err := splitCommandPayloadShard(indexRegion)
	if err != nil {
		t.Fatal(err)
	}
	var indexRoot schemacachepb.SchemaPayloadIndex
	if err := proto.Unmarshal(header, &indexRoot); err != nil {
		t.Fatal(err)
	}
	patchIndex := func(edit func(*schemacachepb.SchemaPayloadIndex)) {
		t.Helper()
		cloned := proto.Clone(&indexRoot).(*schemacachepb.SchemaPayloadIndex)
		edit(cloned)
		blob, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		region, err := assembleCommandPayloadShard(blob, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeSchemaPayloadIndex(region); err == nil {
			t.Fatal("mutated payload index accepted")
		}
	}
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) { m.DtoVersion = schemacachepb.DTOVersion(99) })
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) {
		m.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_UNSPECIFIED
	})
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) { m.Locators = nil })
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) {
		m.Locators.Items = append(m.Locators.Items, m.Locators.Items...)
	})
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) {
		if len(m.Locators.Items) > 1 {
			m.Locators.Items[0].LookupPath, m.Locators.Items[1].LookupPath = m.Locators.Items[1].LookupPath, m.Locators.Items[0].LookupPath
		}
	})
	patchIndex(func(m *schemacachepb.SchemaPayloadIndex) {
		m.Products.Items[0].Sha256 = []byte{1}
	})
	if _, err := DecodeSchemaPayloadIndex([]byte{0x00, 0x00, 0x00, 0x03, 0xff, 0xff, 0x01}); err == nil {
		t.Fatal("invalid index protobuf accepted")
	}

	payloadDesc := meta.PayloadDescriptors[0]
	if err := AuthenticateCommandPayloadDescriptor(meta, payloadDesc); err != nil {
		t.Fatal(err)
	}
	fullPayload := extractProductPayload(t, built, payloadDesc)
	if _, err := DecodeSchemaCommandPayloadCache(fullPayload, payloadDesc, meta); err != nil {
		t.Fatalf("valid full payload: %v", err)
	}

	headerRegion := append([]byte(nil), fullPayload[:payloadDesc.HeaderLength]...)
	inconsistent := append([]byte(nil), headerRegion...)
	binary.BigEndian.PutUint32(inconsistent[:4], uint32(len(inconsistent)-10))
	inconsistentDesc := payloadDesc
	inconsistentDesc.HeaderLength = uint64(len(inconsistent))
	inconsistentDesc.HeaderSHA256 = sha256.Sum256(inconsistent)
	if _, err := DecodeSchemaCommandPayloadHeader(inconsistent, inconsistentDesc); err == nil {
		t.Fatal("inconsistent header prefix accepted")
	}

	wrongHeaderLen := payloadDesc
	wrongHeaderLen.HeaderLength = payloadDesc.Length
	wrongHeaderLen.HeaderSHA256 = payloadDesc.SHA256
	if _, err := DecodeSchemaCommandPayloadCache(fullPayload, wrongHeaderLen, meta); err == nil {
		t.Fatal("header length disagreement accepted")
	}

	splitHeader, blobs, err := splitCommandPayloadShard(fullPayload)
	if err != nil {
		t.Fatal(err)
	}
	badRange := append([]byte(nil), fullPayload...)
	copy(badRange[4+len(splitHeader):], make([]byte, len(blobs)))
	badDesc := payloadDesc
	badDesc.SHA256 = sha256.Sum256(badRange)
	badMeta := meta
	badMeta.PayloadDescriptors = append([]CommandPayloadDescriptor(nil), meta.PayloadDescriptors...)
	badMeta.PayloadDescriptors[0] = badDesc
	if _, err := DecodeSchemaCommandPayloadCache(badRange, badDesc, badMeta); err == nil {
		t.Fatal("blob digest mismatch accepted")
	}

	var payloadRoot schemacachepb.SchemaCommandPayloadCache
	if err := proto.Unmarshal(splitHeader, &payloadRoot); err != nil {
		t.Fatal(err)
	}
	mutateFull := func(edit func(*schemacachepb.SchemaCommandPayloadCache), blobs []byte) {
		t.Helper()
		cloned := proto.Clone(&payloadRoot).(*schemacachepb.SchemaCommandPayloadCache)
		edit(cloned)
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		assembled, err := assembleCommandPayloadShard(encoded, blobs)
		if err != nil {
			t.Fatal(err)
		}
		updated := payloadDesc
		updated.Length = uint64(len(assembled))
		updated.SHA256 = sha256.Sum256(assembled)
		h, _, _ := splitCommandPayloadShard(assembled)
		updated.HeaderLength = uint64(4 + len(h))
		updated.HeaderSHA256 = sha256.Sum256(assembled[:updated.HeaderLength])
		updatedMeta := meta
		updatedMeta.PayloadDescriptors = append([]CommandPayloadDescriptor(nil), meta.PayloadDescriptors...)
		updatedMeta.PayloadDescriptors[0] = updated
		if _, err := DecodeSchemaCommandPayloadCache(assembled, updated, updatedMeta); err == nil {
			t.Fatal("mutated full payload accepted")
		}
	}
	mutateFull(func(m *schemacachepb.SchemaCommandPayloadCache) {
		m.ProtoReflect().SetUnknown([]byte{0xc8, 0x3e, 0x00})
	}, blobs)
	mutateFull(func(m *schemacachepb.SchemaCommandPayloadCache) {
		m.Entries.Items[0].Identity.CliPath = ""
	}, blobs)
	notJSON := []byte("x")
	mutateFull(func(m *schemacachepb.SchemaCommandPayloadCache) {
		sum := sha256.Sum256(notJSON)
		m.RenderedLeafIndex.Items[0].Length = uint64(len(notJSON))
		m.RenderedLeafIndex.Items[0].Offset = 0
		m.RenderedLeafIndex.Items[0].Sha256 = append([]byte(nil), sum[:]...)
	}, notJSON)

	hugeShards := make([]byte, MaxSchemaShardData+1)
	if _, err := DecodeSchemaProductFromShards(hugeShards, DecodedSchemaMeta{RegistryDataLength: uint64(len(hugeShards))}, "sample"); err == nil {
		t.Fatal("oversized shard data accepted")
	}
	if _, _, err := DecodeAllSchemaProducts(built.ProductShards, func() DecodedSchemaMeta {
		cloned := meta
		cloned.ProductDescriptors = append([]ProductDescriptor(nil), meta.ProductDescriptors...)
		cloned.ProductDescriptors[0].Length = 0
		return cloned
	}()); err == nil {
		t.Fatal("zero-length product in DecodeAll accepted")
	}

	var metaRoot schemacachepb.SchemaMetaCache
	if err := proto.Unmarshal(built.Meta, &metaRoot); err != nil {
		t.Fatal(err)
	}
	mutateMeta := func(edit func(*schemacachepb.SchemaMetaCache)) {
		t.Helper()
		cloned := proto.Clone(&metaRoot).(*schemacachepb.SchemaMetaCache)
		edit(cloned)
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeSchemaMetaCache(payload); err == nil {
			t.Fatal("mutated meta accepted")
		}
	}
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		items := make([]*schemacachepb.CommandMetaEntryShard, maxSchemaProducts+1)
		for i := range items {
			items[i] = &schemacachepb.CommandMetaEntryShard{ProductId: "p" + strings.Repeat("x", i%8), Entries: []byte{1}, EntryCount: 1}
		}
		m.CommandEntryShards.Items = items
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.Overview.Registry.Kind = "other"
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.Overview.Products.Items = append(m.Overview.Products.Items, proto.Clone(m.Overview.Products.Items[0]).(*schemacachepb.OverviewProduct))
		m.Overview.Products.Items[1].Id = "zzz"
		m.Overview.Products.Items[0].ToolCount = math.MaxUint64
		m.Overview.Products.Items[1].ToolCount = 1
		m.Overview.ToolCount = math.MaxUint64
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.Overview.Products.Items = nil
		m.Locators.Items = nil
		m.CommandEntryShards.Items = nil
		m.RegistryDataLength = 10
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.ProductDescriptors.Items[0].ProductId = "aaa"
		m.Overview.Products.Items[0].Id = "aaa"
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.ProductDescriptors.Items[0].Offset = 1
	})
	mutateMeta(func(m *schemacachepb.SchemaMetaCache) {
		m.ProductDescriptors.Items[0].Length = 1
	})
}

func TestCrossPlatformCoverageRejectUnknownFieldsAndEnumsRemaining(t *testing.T) {
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.SchemaProductCache{DtoVersion: 99}); err == nil {
		t.Fatal("unknown product DTO accepted")
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.LocatorEntryList{Items: []*schemacachepb.LocatorEntry{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.ProductDescriptorList{Items: []*schemacachepb.ProductDescriptor{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.StringListList{Items: []*schemacachepb.StringList{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.PositionalList{Items: []*schemacachepb.Positional{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.ExampleDispositionList{Items: []*schemacachepb.ExampleDisposition{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.ExampleDisposition{Mode: 99}); err == nil {
		t.Fatal("unknown disposition mode accepted")
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.ExampleDisposition{ReasonCode: 99}); err == nil {
		t.Fatal("unknown disposition reason accepted")
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.CandidateList{Items: []*schemacachepb.FieldCandidate{nil}}); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(&schemacachepb.Constraints{MutuallyExclusive: &schemacachepb.StringListList{Items: []*schemacachepb.StringList{{}}}}); err != nil {
		t.Fatal(err)
	}

	unknown := &schemacachepb.SchemaCommandPayloadCache{}
	unknown.ProtoReflect().SetUnknown([]byte{0xc8, 0x3e, 0x00})
	if err := rejectUnknownFieldsAndEnumsReflect(unknown.ProtoReflect()); err == nil {
		t.Fatal("unknown reflect fields accepted")
	}
	mapped, err := structpb.NewStruct(map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknownFieldsAndEnums(mapped); err == nil {
		t.Fatal("protobuf map encoding accepted")
	}
	outcomes := &schemacachepb.ResultOutcomeList{Items: []schemacachepb.ResultOutcome{99}}
	if err := rejectUnknownFieldsAndEnumsReflect(outcomes.ProtoReflect()); err == nil {
		t.Fatal("unknown repeated enum accepted")
	}
	overview := &schemacachepb.OverviewProduct{SummaryKind: 99}
	if err := rejectUnknownFieldsAndEnumsReflect(overview.ProtoReflect()); err == nil {
		t.Fatal("unknown scalar enum accepted")
	}
	nested := &schemacachepb.SchemaCommandPayloadCache{Entries: &schemacachepb.CommandPayloadEntryList{Items: []*schemacachepb.CommandPayloadEntry{{}}}}
	nested.Entries.Items[0].ProtoReflect().SetUnknown([]byte{0xc8, 0x3e, 0x00})
	if err := rejectUnknownFieldsAndEnumsReflect(nested.ProtoReflect()); err == nil {
		t.Fatal("nested unknown fields accepted")
	}

	if err := validateProductProto(&schemacachepb.ProductSpec{}); err == nil {
		t.Fatal("empty product proto accepted")
	}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", FieldProvenance: &schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{{Key: ""}}}}); err == nil {
		t.Fatal("invalid product provenance accepted")
	}
	if err := validateProductProto(&schemacachepb.ProductSpec{
		Id: "p", Selection: &schemacachepb.Selection{ExampleDispositions: &schemacachepb.ExampleDispositionList{Items: []*schemacachepb.ExampleDisposition{{}}}},
	}); err == nil {
		t.Fatal("unspecified selection enums accepted")
	}
	tools := make([]*schemacachepb.ToolSpec, maxSchemaTools+1)
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: tools}}); err == nil {
		t.Fatal("oversized tool list accepted")
	}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{{}}}}); err == nil {
		t.Fatal("incomplete tool accepted")
	}
	tool := func() *schemacachepb.ToolSpec {
		return &schemacachepb.ToolSpec{
			Identity: &schemacachepb.ToolIdentity{CanonicalPath: "p.run"}, Constraints: &schemacachepb.Constraints{},
			Safety: &schemacachepb.Safety{}, Interface: &schemacachepb.Interface{}, Selection: &schemacachepb.Selection{},
		}
	}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{tool(), tool()}}}); err == nil {
		t.Fatal("unsorted tools accepted")
	}
	params := make([]*schemacachepb.ParameterSpec, maxSchemaParameters+1)
	tooMany := tool()
	tooMany.Parameters = &schemacachepb.ParameterList{Items: params}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{tooMany}}}); err == nil {
		t.Fatal("oversized parameter list accepted")
	}
	nilParam := tool()
	nilParam.Parameters = &schemacachepb.ParameterList{Items: []*schemacachepb.ParameterSpec{nil}}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{nilParam}}}); err == nil {
		t.Fatal("nil parameter accepted")
	}
	provTool := tool()
	provTool.Parameters = &schemacachepb.ParameterList{Items: []*schemacachepb.ParameterSpec{{Name: "x", FieldProvenance: &schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{{Key: ""}}}}}}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{provTool}}}); err == nil {
		t.Fatal("parameter provenance accepted")
	}
	resultTool := tool()
	resultTool.Result = &schemacachepb.Result{}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{resultTool}}}); err == nil {
		t.Fatal("incomplete result accepted")
	}
	selTool := tool()
	selTool.Selection = &schemacachepb.Selection{ExampleDispositions: &schemacachepb.ExampleDispositionList{Items: []*schemacachepb.ExampleDisposition{{}}}}
	if err := validateProductProto(&schemacachepb.ProductSpec{Id: "p", Selection: &schemacachepb.Selection{}, Tools: &schemacachepb.ToolList{Items: []*schemacachepb.ToolSpec{selTool}}}); err == nil {
		t.Fatal("unspecified tool selection accepted")
	}
	if err := validateProvenanceProto(&schemacachepb.ProvenanceList{Items: make([]*schemacachepb.ProvenanceEntry, maxSchemaProvenance+1)}, "p"); err == nil {
		t.Fatal("oversized provenance accepted")
	}
	if err := validateProvenanceProto(&schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{nil}}, "p"); err == nil {
		t.Fatal("nil provenance entry accepted")
	}
	if err := validateProvenanceProto(&schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{{
		Key: "k", Value: &schemacachepb.FieldProvenance{Candidates: &schemacachepb.CandidateList{Items: make([]*schemacachepb.FieldCandidate, maxSchemaCandidates+1)}},
	}}}, "p"); err == nil {
		t.Fatal("oversized candidates accepted")
	}
	if err := validateProvenanceProto(&schemacachepb.ProvenanceList{Items: []*schemacachepb.ProvenanceEntry{{
		Key: "k", Value: &schemacachepb.FieldProvenance{Candidates: &schemacachepb.CandidateList{Items: []*schemacachepb.FieldCandidate{nil}}},
	}}}, "p"); err == nil {
		t.Fatal("nil candidate accepted")
	}
}

func TestCrossPlatformCoverageModelPaginationAndProvenanceRemaining(t *testing.T) {
	tool := allFieldsRegistry().Products[0].Tools[0]
	tool.Pagination = &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "missing"}
	if err := tool.Validate(); err == nil {
		t.Fatal("missing pagination cursor accepted")
	}
	selected := true
	tool = allFieldsRegistry().Products[0].Tools[0]
	tool.FieldProvenance["title"] = contract.FieldProvenance{
		Value: json.RawMessage(`"Run sample"`), Source: "contract_final", Precedence: "100", Resolution: "selected",
		Candidates: []contract.FieldCandidateProvenance{{
			Value: json.RawMessage(`"Run sample"`), Source: "contract_final", Precedence: "100", Selected: &selected,
		}},
		OverriddenCandidates: []contract.FieldCandidateProvenance{{Value: json.RawMessage(`"Run sample"`), Selected: &selected}},
	}
	if err := tool.Validate(); err == nil {
		t.Fatal("selected overridden provenance accepted")
	}

	registry := allFieldsRegistry()
	registry.Products[0].FieldProvenance["unknown"] = contract.FieldProvenance{Value: json.RawMessage(`{`)}
	if _, err := registry.ToPayload(); err == nil {
		t.Fatal("invalid extra product provenance payload accepted")
	}
}
