// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemacachepb

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestCrossPlatformCoverageGeneratedSchemaCacheAccessors(t *testing.T) {
	for _, enum := range []interface {
		String() string
		Number() protoreflect.EnumNumber
	}{
		DTOVersion_DTO_VERSION_V5, DTOVersion(99),
		OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_USE_WHEN, OverviewSummaryKind(99),
		ResultOutcome_RESULT_OUTCOME_SUCCESS, ResultOutcome(99),
		ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT_ONLY, ExampleDispositionMode(99),
		ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_LOCAL_STATE, ExampleDispositionReasonCode(99),
	} {
		_ = enum.String()
		_ = enum.Number()
	}
	_ = DTOVersion_DTO_VERSION_V5.Enum()
	_ = DTOVersion_DTO_VERSION_V5.Descriptor()
	_ = DTOVersion_DTO_VERSION_V5.Type()
	_, _ = DTOVersion_DTO_VERSION_V5.EnumDescriptor()
	_ = OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_AGENT_SUMMARY.Enum()
	_ = OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_DESCRIPTION.Descriptor()
	_ = OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_UNSPECIFIED.Type()
	_, _ = OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_UNSPECIFIED.EnumDescriptor()
	_ = ResultOutcome_RESULT_OUTCOME_PENDING.Enum()
	_ = ResultOutcome_RESULT_OUTCOME_FAILURE.Descriptor()
	_ = ResultOutcome_RESULT_OUTCOME_PARTIAL_FAILURE.Type()
	_, _ = ResultOutcome_RESULT_OUTCOME_UNSPECIFIED.EnumDescriptor()
	_ = ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_DRY_RUN.Enum()
	_ = ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT.Descriptor()
	_ = ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_UNSPECIFIED.Type()
	_, _ = ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_UNSPECIFIED.EnumDescriptor()
	_ = ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_STATEFUL_PREFLIGHT.Enum()
	_ = ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_UNSPECIFIED.Descriptor()
	_ = ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_UNSPECIFIED.Type()
	_, _ = ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_UNSPECIFIED.EnumDescriptor()
	_ = File_schema_cache_proto.Path()
	_ = file_schema_cache_proto_rawDescGZIP()

	for _, msg := range []proto.Message{
		&StringList{Items: []string{"a"}},
		&BytesValue{Value: []byte("v")},
		&BoolValue{Value: true},
		&IntValue{Value: 7},
		&StringListList{Items: []*StringList{{Items: []string{"a"}}}},
		&SchemaMetaCache{DtoVersion: DTOVersion_DTO_VERSION_V5, Registry: &RegistryFields{Kind: "schema"}, Overview: &SchemaOverviewCache{}, Locators: &LocatorEntryList{}, ProductDescriptors: &ProductDescriptorList{}, RegistryDataLength: 1, RegistryDataSha256: []byte("r"), SourceSha256: []byte("s"), SurfaceSha256: []byte("t"), CommandPayloadDescriptors: &CommandPayloadDescriptorList{}, PayloadDataLength: 1, PayloadSha256: []byte("p"), CommandEntryShards: &CommandMetaEntryShardList{}},
		&SchemaProductCache{DtoVersion: DTOVersion_DTO_VERSION_V5, Registry: &RegistryFields{Source: "runtime"}, Product: &ProductSpec{Id: "p"}},
		&RegistryFields{Kind: "schema", Level: "catalog", Source: "runtime", AgentMetadata: &BytesValue{Value: []byte("null")}},
		&CommandMetaEntryList{Items: []*CommandMetaEntry{{LookupPath: "p run"}}},
		&CommandMetaEntryShard{ProductId: "p", Entries: []byte("e"), EntryCount: 1},
		&CommandMetaEntryShardList{Items: []*CommandMetaEntryShard{{ProductId: "p"}}},
		&CommandSelectionPayload{AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Prerequisites: []string{"p"}, Tips: []string{"t"}, Examples: []string{"e"}, ListsPresent: 31},
		&CommandMetaEntry{LookupPath: "p run", CliPath: "p run", Canonical: "p.run", ProductId: "p", Title: "T", Aliases: []string{"alias"}, ListsPresent: 1},
		&CommandPayloadEntry{LookupPath: "p run", Effect: "read", Risk: "low", Confirmation: "none", Idempotency: "yes", Selection: &CommandSelectionPayload{AgentSummary: "s"}, Identity: &CommandMetaEntry{CliPath: "p run"}},
		&CommandPayloadEntryList{Items: []*CommandPayloadEntry{{LookupPath: "p run"}}},
		&SchemaPayloadIndex{DtoVersion: DTOVersion_DTO_VERSION_V5, Locators: &LocatorEntryList{}, Products: &CommandPayloadDescriptorList{}},
		&RenderedSchemaLeafRef{CanonicalPath: "p.run", Offset: 1, Length: 2, Sha256: []byte("h")},
		&RenderedSchemaLeafRefList{Items: []*RenderedSchemaLeafRef{{CanonicalPath: "p.run"}}},
		&SchemaCommandPayloadCache{DtoVersion: DTOVersion_DTO_VERSION_V5, ProductId: "p", Entries: &CommandPayloadEntryList{}, RenderedLeafIndex: &RenderedSchemaLeafRefList{}},
		&CommandPayloadDescriptor{ProductId: "p", Offset: 1, Length: 2, Sha256: []byte("h"), HeaderLength: 3, HeaderSha256: []byte("i")},
		&CommandPayloadDescriptorList{Items: []*CommandPayloadDescriptor{{ProductId: "p"}}},
		&SchemaOverviewCache{Products: &OverviewProductList{}},
		&OverviewProductList{Items: []*OverviewProduct{{Id: "p"}}},
		&OverviewProduct{Id: "p", ToolCount: 1, SchemaPath: "p", SummaryKind: OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_AGENT_SUMMARY, Summary: "s"},
		&LocatorEntryList{Items: []*LocatorEntry{{LookupPath: "p.run"}}},
		&LocatorEntry{LookupPath: "p.run", ProductId: "p"},
		&ProductDescriptorList{Items: []*ProductDescriptor{{ProductId: "p"}}},
		&ProductDescriptor{ProductId: "p", Offset: 1, Length: 2, Sha256: []byte("h")},
		&ProductSpec{Id: "p", Name: "P", Description: "d", Runtime: true, Tools: &ToolList{}, Selection: &Selection{}, FieldProvenance: &ProvenanceList{}},
		&ToolList{Items: []*ToolSpec{{Title: "T"}}},
		&ToolSpec{Identity: &ToolIdentity{Name: "run"}, Display: "D", Title: "T", Description: "d", MetadataSource: "c", Parameters: &ParameterList{}, Constraints: &Constraints{}, Positionals: &PositionalList{}, DryRun: &DryRun{}, Result: &Result{}, Pagination: &Pagination{}, Safety: &Safety{}, Interface: &Interface{}, Selection: &Selection{}, FieldProvenance: &ProvenanceList{}},
		&ParameterList{Items: []*ParameterSpec{{Name: "n"}}},
		&ParameterSpec{Name: "n", Type: "string", Description: "d", Property: "p", Required: true, CliRequired: true, RequiredWhen: "always", DefaultValue: &BytesValue{Value: []byte("null")}, InterfaceDefault: &BytesValue{Value: []byte("null")}, Example: &BytesValue{Value: []byte("null")}, Format: "token", Enum: &StringList{Items: []string{"a"}}, InterfaceDescription: "id", InterfaceType: "string", FieldProvenance: &ProvenanceList{}},
		&ToolIdentity{ProductId: "p", SourceProductId: "s", Name: "run", CliName: "run", CanonicalPath: "p.run", Path: "p.run", CliPath: "p run", PrimaryCliPath: "p run", Group: "g", Aliases: &StringList{Items: []string{"a"}}, IsAlias: false, Source: "runtime"},
		&Constraints{MutuallyExclusive: &StringListList{Items: []*StringList{{Items: []string{"a", "b"}}}}, RequireOneOf: &StringListList{}, RequireTogether: &StringListList{}},
		&PositionalList{Items: []*Positional{{Name: "p"}}},
		&Positional{Name: "p", Type: "string", Description: "d", Required: true, Variadic: true, Index: 1},
		&DryRun{PreviewKind: "request", RemoteReads: true},
		&Result{Outcomes: &ResultOutcomeList{Items: []ResultOutcome{ResultOutcome_RESULT_OUTCOME_SUCCESS}}, DataSchema: &BytesValue{Value: []byte(`{"type":"object"}`)}, SensitivePaths: &StringList{Items: []string{"secret"}}},
		&ResultOutcomeList{Items: []ResultOutcome{ResultOutcome_RESULT_OUTCOME_FAILURE}},
		&Pagination{Kind: "cursor", CursorParameter: "cursor", MetaPath: "meta.pagination", EndpointExhaustedPath: "meta.pagination.endpoint_exhausted", NextTokenPath: "meta.pagination.next_token"},
		&Safety{Effect: "read", EffectSource: "declared", Risk: "low", Confirmation: "none", Idempotency: "yes"},
		&Interface{Mode: "mcp", Availability: "available", Reason: "rpc", Ref: &InterfaceRef{ProductId: "p", RpcName: "Run"}},
		&InterfaceRef{ProductId: "p", RpcName: "Run"},
		&Selection{AgentSummary: "s", AgentSummarySource: "c", UseWhen: &StringList{Items: []string{"u"}}, AvoidWhen: &StringList{}, Prerequisites: &StringList{}, Tips: &StringList{}, WorkflowRefs: &StringList{Items: []string{"w"}}, Examples: &StringList{}, ExampleDispositions: &ExampleDispositionList{}, Reviewed: &BoolValue{Value: true}, SourceRefs: &StringList{Items: []string{"r"}}, MetadataSource: "c"},
		&ExampleDispositionList{Items: []*ExampleDisposition{{Reason: "r"}}},
		&ExampleDisposition{Index: &IntValue{Value: 0}, Mode: ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT_ONLY, ReasonCode: ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_LOCAL_STATE, Reason: "r", Reviewed: true},
		&ProvenanceList{Items: []*ProvenanceEntry{{Key: "title"}}},
		&ProvenanceEntry{Key: "title", Value: &FieldProvenance{Source: "c"}},
		&FieldProvenance{Value: &BytesValue{Value: []byte(`"t"`)}, Source: "c", SourceRef: "r", Precedence: "100", Resolution: "selected", ReviewReason: "ok", Candidates: &CandidateList{}, OverriddenCandidates: &CandidateList{}},
		&CandidateList{Items: []*FieldCandidate{{Source: "c"}}},
		&FieldCandidate{Value: &BytesValue{Value: []byte(`"t"`)}, Source: "c", SourceRef: "r", Precedence: "100", ReviewReason: "ok", Selected: &BoolValue{Value: true}},
	} {
		exerciseGeneratedMessage(t, msg)
	}
}

func exerciseGeneratedMessage(t *testing.T, msg proto.Message) {
	t.Helper()
	typ := reflect.TypeOf(msg)
	if typ.Kind() != reflect.Pointer {
		t.Fatalf("want pointer message, got %s", typ)
	}
	callZeroArgMethods(reflect.Zero(typ))
	filled := proto.Clone(msg)
	if _, err := proto.Marshal(filled); err != nil {
		t.Fatalf("marshal %T: %v", msg, err)
	}
	_ = filled.ProtoReflect()
	callZeroArgMethods(reflect.ValueOf(filled))
	proto.Reset(filled)
}

func callZeroArgMethods(rv reflect.Value) {
	if !rv.IsValid() {
		return
	}
	skipNilUnsafe := rv.Kind() == reflect.Pointer && rv.IsNil()
	for i := 0; i < rv.NumMethod(); i++ {
		method := rv.Method(i)
		name := rv.Type().Method(i).Name
		if method.Type().NumIn() != 0 {
			continue
		}
		if skipNilUnsafe && name == "Reset" {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			method.Call(nil)
		}()
	}
}
