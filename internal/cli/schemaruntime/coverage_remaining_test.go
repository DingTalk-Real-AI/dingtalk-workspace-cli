package schemaruntime

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
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

func TestCrossPlatformCoverageCompactProvenanceQueryAndIndexGaps(t *testing.T) {
	if Compact(nil) != nil {
		t.Fatal("nil compact")
	}
	payload := Compact(map[string]any{
		"kind": "schema", "ignored": 1,
		"product":  map[string]any{"kind": "schema", "id": "p"},
		"products": []map[string]any{{"kind": "schema", "id": "a"}},
		"tools":    []any{map[string]any{"id": "t"}, "plain"},
		"parameters": map[string]any{
			"ok":   map[string]any{"type": "string", "required": true, "secret": 1},
			"raw":  "leaf",
			"nest": map[string]any{"type": "object"},
		},
	})
	if payload["kind"] != "schema" || payload["ignored"] != nil {
		t.Fatalf("compact payload = %#v", payload)
	}
	if Compact(map[string]any{"product": "not-map"})["product"] != "not-map" {
		t.Fatal("non-map product")
	}
	_ = CompactCollection("x")
	_ = CompactCollection([]map[string]any{{"kind": "schema"}})
	_ = CompactCollection([]any{map[string]any{"kind": "schema"}, 1})
	_ = CompactParameters("x")
	_ = CompactParameters(map[string]any{"n": 1})
	_ = CompactValue(map[string]any{"description": "x"})
	_ = CompactValue([]map[string]any{{"kind": "schema"}})
	_ = CompactValue([]any{1, map[string]any{"type": "string"}})
	_ = CompactValue(3)
	_ = CompactParameter(map[string]any{"type": "string", "extra": true})

	tool := allFieldsRegistry().Products[0].Tools[0]
	for _, field := range []string{
		"description", "metadata_source", "dry_run", "effect", "effect_source", "risk", "confirmation", "idempotency",
		"interface_ref", "interface_mode", "availability", "interface_reason", "agent_summary", "use_when", "avoid_when",
		"prerequisites", "tips", "workflow_refs", "examples", "reviewed", "missing",
	} {
		ToolProvenanceValue(tool, field)
	}
	param := tool.Parameters[0]
	for _, field := range []string{
		"name", "type", "description", "property", "required", "cli_required", "required_when", "default",
		"interface_default", "example", "anyOf", "format", "enum", "interface_description", "interface_type", "missing",
	} {
		ParameterProvenanceValue(param, field)
	}
	ProductProvenanceValue(allFieldsRegistry().Products[0], "agent_summary")
	ProductProvenanceValue(allFieldsRegistry().Products[0], "use_when")
	ProductProvenanceValue(allFieldsRegistry().Products[0], "avoid_when")
	ProductProvenanceValue(allFieldsRegistry().Products[0], "missing")

	emptyID := allFieldsRegistry()
	emptyID.Products[0].ID = ""
	if _, err := emptyID.Index(); err == nil {
		t.Fatal("empty product id")
	}
	dup := allFieldsRegistry()
	dup.Products = append(dup.Products, dup.Products[0])
	if _, err := dup.Index(); err == nil {
		t.Fatal("duplicate product")
	}
	mismatch := allFieldsRegistry()
	mismatch.Products[0].Tools[0].Identity.ProductID = "other"
	if _, err := mismatch.Index(); err == nil {
		t.Fatal("product mismatch")
	}
	dupCanon := allFieldsRegistry()
	dupCanon.Products[0].Tools[1].Identity = dupCanon.Products[0].Tools[0].Identity
	dupCanon.Products[0].Tools[1].Identity.Name = "run"
	dupCanon.Products[0].Tools[1].Identity.CanonicalPath = "sample.run"
	if _, err := dupCanon.Index(); err == nil {
		t.Fatal("duplicate canonical")
	}
	cliConflict := allFieldsRegistry()
	cliConflict.Products[0].Tools[1].Identity.CLIPath = cliConflict.Products[0].Tools[0].Identity.CLIPath
	cliConflict.Products[0].Tools[1].Identity.PrimaryCLIPath = cliConflict.Products[0].Tools[0].Identity.PrimaryCLIPath
	if _, err := cliConflict.Index(); err == nil {
		t.Fatal("cli path conflict")
	}
	aliasCanon := allFieldsRegistry()
	aliasCanon.Products[0].Tools[0].Identity.IsAlias = true
	if err := validateCanonicalToolIdentity(aliasCanon.Products[0].Tools[0]); err == nil {
		t.Fatal("alias canonical accepted")
	}
	cliDrift := allFieldsRegistry()
	cliDrift.Products[0].Tools[0].Identity.CLIPath = "other path"
	if err := validateCanonicalToolIdentity(cliDrift.Products[0].Tools[0]); err == nil {
		t.Fatal("cli drift accepted")
	}

	bad := allFieldsRegistry().Products[0].Tools[0]
	bad.Identity.ProductID = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty product_id")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Identity.Name = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty name")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Identity.CanonicalPath = "nope"
	if err := bad.Validate(); err == nil {
		t.Fatal("canonical mismatch")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Identity.CLIPath = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty cli")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Parameters = append(append([]ParameterSpec(nil), bad.Parameters...), ParameterSpec{})
	if err := bad.Validate(); err == nil {
		t.Fatal("empty parameter")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Parameters = append(append([]ParameterSpec(nil), bad.Parameters...), bad.Parameters[0])
	if err := bad.Validate(); err == nil {
		t.Fatal("duplicate parameter")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Parameters[0].Default = json.RawMessage(`{`)
	if err := bad.Validate(); err == nil {
		t.Fatal("bad default json")
	}
	bad = allFieldsRegistry().Products[0].Tools[0]
	bad.Interface.Ref = &contract.InterfaceRefSpec{}
	if err := bad.Validate(); err == nil {
		t.Fatal("incomplete interface_ref")
	}

	if _, err := ToSnapshotPayloadWithProjectors(emptyID, SnapshotProjectors{}); err == nil {
		t.Fatal("snapshot index failure")
	}
	fail := errors.New("proj")
	if _, err := ToSnapshotPayloadWithProjectors(allFieldsRegistry(), SnapshotProjectors{ToolSummary: func(ToolSpec) (map[string]any, error) { return nil, fail }}); err == nil {
		t.Fatal("summary projector")
	}
	if _, err := ToSnapshotPayloadWithProjectors(allFieldsRegistry(), SnapshotProjectors{ToolPayload: func(ToolSpec) (map[string]any, error) { return nil, fail }}); err == nil {
		t.Fatal("tool projector")
	}
	if _, err := ToSnapshotPayloadWithProjectors(allFieldsRegistry(), SnapshotProjectors{ProductSummary: func(ProductSpec) (map[string]any, error) { return nil, fail }}); err == nil {
		t.Fatal("product projector")
	}

	p := ParameterSpec{Name: "x", Type: "string", Description: "d", CLIRequired: true, RequiredWhen: "always", Default: json.RawMessage(`{`)}
	if _, err := p.ToPayload(); err == nil {
		t.Fatal("param default json")
	}
	p = ParameterSpec{Name: "x", Type: "string", Description: "d", InterfaceDefault: json.RawMessage(`{`)}
	if _, err := p.ToPayload(); err == nil {
		t.Fatal("param interface default json")
	}
	p = ParameterSpec{Name: "x", Type: "string", Description: "d", Example: json.RawMessage(`{`)}
	if _, err := p.ToPayload(); err == nil {
		t.Fatal("param example json")
	}
	p = ParameterSpec{Name: "x", Type: "int", Description: "d", AnyOf: []contract.FormatAlternative{{Format: "email"}}}
	if _, err := p.ToPayload(); err == nil {
		t.Fatal("anyOf type")
	}
	p = ParameterSpec{Name: "x", Type: "string", Description: "d", AnyOf: []contract.FormatAlternative{{Format: "email"}}, Format: "uuid"}
	if _, err := p.ToPayload(); err == nil {
		t.Fatal("anyOf format")
	}
	p = ParameterSpec{Name: "x", Type: "string", Description: "d", AnyOf: []contract.FormatAlternative{{Format: "email"}}, Enum: []string{"a"}, FieldProvenance: map[string]contract.FieldProvenance{"name": {}}}
	if _, err := p.ToPayload(); err != nil {
		t.Fatal(err)
	}

	_ = DefaultString("", "fb")
	_ = DefaultString("x", "fb")
	if _, err := RawJSONValue(json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid raw json")
	}
	if _, err := RawJSONValue(json.RawMessage(`1`)); err != nil {
		t.Fatal(err)
	}
	if _, err := TypedJSONValue(map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	_ = StableUniqueStrings([]string{"b", "a", "b"})
	_ = SortedUniqueStrings([]string{"b", "a"})
	_ = CloneOptionalStrings(nil)
	_ = CloneOptionalStrings([]string{"a"})
	_ = ValidateFinalFieldProvenance("o", "f", contract.FieldProvenance{Value: json.RawMessage(`1`)}, 1)
	_ = EqualJSONValues([]byte("1"), []byte("1"))

	decoded := DecodedCommandPayloads{LeafIndex: []RenderedLeafRef{{CanonicalPath: "a"}, {CanonicalPath: "c"}}}
	if _, ok := decoded.RenderedLeaf("missing"); ok {
		t.Fatal("missing leaf")
	}
	if _, ok := decoded.RenderedLeaf("c"); !ok {
		t.Fatal("leaf c")
	}

	reg := allFieldsRegistry()
	index, err := reg.Index()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderQueryWithProjectors(reg, index, "sample group", QueryProjectors{}); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderQueryWithProjectors(reg, index, "sample group", QueryProjectors{ToolSummary: func(ToolSpec) (map[string]any, error) { return nil, fail }}); err == nil {
		t.Fatal("group projector")
	}
	view := AliasView(reg.Products[0].Tools[0], "sample legacy run")
	if !view.Identity.IsAlias {
		t.Fatal("alias view")
	}
	_ = AliasView(reg.Products[0].Tools[0], "")
	_ = AliasView(reg.Products[0].Tools[0], "sample group run")
	_ = ToolUnderGroup(reg.Products[0].Tools[0], "sample group")
	_ = ToolUnderGroup(reg.Products[0].Tools[1], "sample group")
	hashed := map[string]any{}
	StampTrustedHashes(hashed, TrustedHashes{CatalogHash: "c", SurfaceHash: "s"})
	if hashed["surface_hash"] != "s" {
		t.Fatalf("hashes = %#v", hashed)
	}
	_ = sourceOrDefault("")
	_ = sourceOrDefault(" custom ")
	_ = UnknownPathError{Path: `say "hi"`}.Error()

	norm := allFieldsRegistry().Products[0].Tools[1]
	norm.Identity.CanonicalPath = ""
	norm.Identity.ProductID = "sample"
	norm.Identity.Name = "zzz"
	_ = norm.normalized()
}
