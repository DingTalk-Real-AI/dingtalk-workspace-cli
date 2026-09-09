package schemaruntime

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"google.golang.org/protobuf/proto"
)

func TestCrossPlatformCoverageLocatorAndOverviewRemainingBranches(t *testing.T) {
	registry := allFieldsRegistry()
	registry.Products[0].Tools[0].Identity.Aliases = append(append([]string(nil), registry.Products[0].Tools[0].Identity.Aliases...), "   ")
	if _, err := BuildSchemaProductLocators(registry); err != nil {
		t.Fatalf("blank alias locator: %v", err)
	}

	first := allFieldsRegistry()
	first.Products[0].Tools[0].Identity.Aliases = append(append([]string(nil), first.Products[0].Tools[0].Identity.Aliases...), "beta")
	second := allFieldsRegistry().Products[0]
	second.ID = "beta"
	second.Name = "Beta"
	second.FieldProvenance = nil
	second.Selection = contract.SelectionSpec{}
	copied := second.Tools[0]
	copied.Identity.ProductID = "beta"
	copied.Identity.CanonicalPath = "beta.run"
	copied.Identity.Path = "beta.run"
	copied.Identity.CLIPath = "beta run"
	copied.Identity.PrimaryCLIPath = "beta run"
	copied.Identity.Aliases = nil
	copied.Identity.Group = ""
	second.Tools = []ToolSpec{copied}
	first.Products = append(first.Products, second)
	if _, err := BuildSchemaProductLocators(first); err == nil {
		t.Fatal("product ID collision accepted")
	}

	useWhen := allFieldsRegistry()
	useWhen.Products[0].Selection.AgentSummary = ""
	useWhen.Products[0].FieldProvenance = nil
	overview, err := BuildSchemaOverview(useWhen)
	if err != nil || overview.Products[0].SummaryKind != OverviewSummaryUseWhen {
		t.Fatalf("use_when overview = %#v, %v", overview, err)
	}

	described := allFieldsRegistry()
	described.Products[0].Selection.AgentSummary = ""
	described.Products[0].Selection.UseWhen = nil
	described.Products[0].FieldProvenance = nil
	payload, err := described.ToOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	if payload["products"].([]map[string]any)[0]["description"] != "Sample product" {
		t.Fatalf("description overview = %#v", payload["products"])
	}

	_, meta := buildFixtureCache(t, allFieldsRegistry())
	if _, ok := meta.commandMetaMapForProduct("sample"); !ok {
		t.Fatal("commandMetaMapForProduct missed sample")
	}
	if _, _, err := ProductShardBounds(ProductDescriptor{ProductID: "x", Offset: uint64(math.MaxInt64) + 1, Length: 1}, uint64(math.MaxInt64)+2); err == nil {
		t.Fatal("unrepresentable offset accepted")
	}
}

func TestCrossPlatformCoverageDecodeProductPayloadAndIndexMutations(t *testing.T) {
	built, meta := buildFixtureCache(t, allFieldsRegistry())
	desc := meta.ProductDescriptors[0]
	offset, length, err := ProductShardBounds(desc, uint64(len(built.ProductShards)))
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), built.ProductShards[int(offset):int(offset)+length]...)
	digestMismatch := append([]byte(nil), raw...)
	digestMismatch[0] ^= 1
	if _, err := DecodeSchemaProductCache(digestMismatch, desc, meta); err == nil {
		t.Fatal("product digest mismatch accepted")
	}

	var root schemacachepb.SchemaProductCache
	if err := proto.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	patch := func(edit func(*schemacachepb.SchemaProductCache)) {
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
		if _, err := DecodeSchemaProductCache(payload, updated, patched); err == nil {
			t.Fatal("mutated product accepted")
		}
	}
	patch(func(m *schemacachepb.SchemaProductCache) {
		m.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_UNSPECIFIED
	})
	patch(func(m *schemacachepb.SchemaProductCache) { m.Product = nil })
	patch(func(m *schemacachepb.SchemaProductCache) { m.Registry = nil })
	patch(func(m *schemacachepb.SchemaProductCache) {
		m.Registry.AgentMetadata = &schemacachepb.BytesValue{Value: []byte("{")}
	})
	patch(func(m *schemacachepb.SchemaProductCache) { m.Registry.Kind = "other" })
	patch(func(m *schemacachepb.SchemaProductCache) { m.Product.Id = "other" })

	if _, err := DecodeSchemaPayloadIndex(nil); err == nil {
		t.Fatal("empty payload index accepted")
	}
	indexRegion := append([]byte(nil), built.PayloadShards[:built.PayloadIndexLength]...)
	if _, err := DecodeSchemaPayloadIndex(indexRegion); err != nil {
		t.Fatalf("valid index: %v", err)
	}
	if _, err := DecodeSchemaPayloadIndex([]byte{0xff, 0xff, 0xff, 0xff, 0x01}); err == nil {
		t.Fatal("invalid index proto accepted")
	}
	var prefix [4]byte
	prefix[3] = 1
	if _, err := DecodeSchemaPayloadIndex(append(prefix[:], 0x01, 0x02)); err == nil {
		t.Fatal("trailing payload index bytes accepted")
	}

	payloadDesc := meta.PayloadDescriptors[0]
	fullPayload := extractProductPayload(t, built, payloadDesc)
	headerRegion := append([]byte(nil), fullPayload[:payloadDesc.HeaderLength]...)
	if _, err := DecodeSchemaCommandPayloadHeader(headerRegion, payloadDesc); err != nil {
		t.Fatalf("valid header: %v", err)
	}
	wrong := append([]byte(nil), headerRegion...)
	wrong[len(wrong)-1] ^= 1
	if _, err := DecodeSchemaCommandPayloadHeader(wrong, payloadDesc); err == nil {
		t.Fatal("header digest mismatch accepted")
	}
	header, _, err := splitCommandPayloadShard(headerRegion)
	if err != nil {
		t.Fatal(err)
	}
	var payloadRoot schemacachepb.SchemaCommandPayloadCache
	if err := proto.Unmarshal(header, &payloadRoot); err != nil {
		t.Fatal(err)
	}
	mutateHeader := func(edit func(*schemacachepb.SchemaCommandPayloadCache)) {
		t.Helper()
		cloned := proto.Clone(&payloadRoot).(*schemacachepb.SchemaCommandPayloadCache)
		edit(cloned)
		blob, err := proto.MarshalOptions{Deterministic: true}.Marshal(cloned)
		if err != nil {
			t.Fatal(err)
		}
		region, err := assembleCommandPayloadShard(blob, nil)
		if err != nil {
			t.Fatal(err)
		}
		updated := payloadDesc
		updated.HeaderLength = uint64(len(region))
		updated.HeaderSHA256 = sha256.Sum256(region)
		if _, err := DecodeSchemaCommandPayloadHeader(region, updated); err == nil {
			t.Fatal("mutated command payload header accepted")
		}
	}
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		m.DtoVersion = schemacachepb.DTOVersion_DTO_VERSION_UNSPECIFIED
	})
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) { m.ProductId = "other" })
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) { m.Entries = nil })
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		if len(m.Entries.Items) > 0 {
			m.Entries.Items[0].Identity = nil
		}
	})
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		if len(m.Entries.Items) > 0 && m.Entries.Items[0].Identity != nil {
			m.Entries.Items[0].Identity.CliPath = ""
		}
	})
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		if m.RenderedLeafIndex != nil && len(m.RenderedLeafIndex.Items) > 0 {
			m.RenderedLeafIndex.Items[0].Sha256 = []byte{1}
		}
	})
	mutateHeader(func(m *schemacachepb.SchemaCommandPayloadCache) {
		if m.RenderedLeafIndex != nil && len(m.RenderedLeafIndex.Items) > 0 {
			m.RenderedLeafIndex.Items[0].CanonicalPath = ""
		}
	})
}

func TestCrossPlatformCoverageProductProvenanceIndexFailure(t *testing.T) {
	registry := allFieldsRegistry()
	registry.Products[0].FieldProvenance["agent_summary"] = contract.FieldProvenance{
		Value: json.RawMessage(`"nope"`), Source: "s", Precedence: "1", Resolution: "x",
	}
	if _, err := registry.Index(); err == nil {
		t.Fatal("mismatched product provenance accepted")
	}
}
