// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func registryFieldsToProto(in SchemaRegistry) *schemacachepb.RegistryFields {
	return &schemacachepb.RegistryFields{Kind: in.Kind, Level: in.Level, Source: in.Source, AgentMetadata: bytesToProto(in.AgentMetadata)}
}

func registryFromProductProto(in *schemacachepb.SchemaProductCache) SchemaRegistry {
	fields := in.GetRegistry()
	return SchemaRegistry{
		Kind: fields.GetKind(), Level: fields.GetLevel(), Source: fields.GetSource(),
		AgentMetadata: bytesFromProto(fields.GetAgentMetadata()),
		Products:      []ProductSpec{productFromProto(in.GetProduct())},
	}
}

func commandLookupToProto(in map[string]CommandMeta) *schemacachepb.CommandMetaEntryList {
	keys := sortedMapKeys(in)
	out := &schemacachepb.CommandMetaEntryList{Items: make([]*schemacachepb.CommandMetaEntry, len(keys))}
	for i, key := range keys {
		out.Items[i] = commandMetaToProto(in[key])
		out.Items[i].LookupPath = key
	}
	return out
}

const commandMetaListCount = 6

func commandMetaToProto(in CommandMeta) *schemacachepb.CommandMetaEntry {
	out := &schemacachepb.CommandMetaEntry{
		CliPath: in.Identity.CLIPath, Canonical: in.Identity.Canonical, ProductId: in.Identity.ProductID, Title: in.Identity.Title,
		Effect: in.Safety.Effect, Risk: in.Safety.Risk, Confirmation: in.Safety.Confirmation, Idempotency: in.Safety.Idempotency,
		AgentSummary: in.Selection.AgentSummary,
		Aliases:      slices.Clone(in.Identity.Aliases), UseWhen: slices.Clone(in.Selection.UseWhen), AvoidWhen: slices.Clone(in.Selection.AvoidWhen),
		Prerequisites: slices.Clone(in.Selection.Prerequisites), Tips: slices.Clone(in.Selection.Tips), Examples: slices.Clone(in.Selection.Examples),
	}
	for bit, list := range commandMetaProtoLists(out) {
		if list != nil {
			out.ListsPresent |= 1 << bit
		}
	}
	return out
}

func commandMetaProtoLists(in *schemacachepb.CommandMetaEntry) [commandMetaListCount][]string {
	return [commandMetaListCount][]string{in.Aliases, in.UseWhen, in.AvoidWhen, in.Prerequisites, in.Tips, in.Examples}
}

func validateCommandMetaListPresence(in *schemacachepb.CommandMetaEntry) error {
	if in.ListsPresent & ^uint32((1<<commandMetaListCount)-1) != 0 {
		return fmt.Errorf("unknown metadata list presence bits")
	}
	for bit, list := range commandMetaProtoLists(in) {
		if len(list) != 0 && in.ListsPresent&(1<<bit) == 0 {
			return fmt.Errorf("nonempty metadata list %d has no presence bit", bit)
		}
	}
	return nil
}

// Copy into runtime-owned slices; no generated pointer or backing slice escapes.
// An explicit set bit restores present-empty even when protobuf omits its values.
func commandMetaFromProto(in *schemacachepb.CommandMetaEntry) CommandMeta {
	var lists [commandMetaListCount][]string
	for bit, values := range commandMetaProtoLists(in) {
		if in.ListsPresent&(1<<bit) != 0 {
			lists[bit] = make([]string, len(values))
			copy(lists[bit], values)
		}
	}
	return CommandMeta{
		Identity:  CommandIdentity{CLIPath: in.CliPath, Canonical: in.Canonical, Aliases: lists[0], ProductID: in.ProductId, Title: in.Title},
		Safety:    CommandSafety{Effect: in.Effect, Risk: in.Risk, Confirmation: in.Confirmation, Idempotency: in.Idempotency},
		Selection: CommandSelection{AgentSummary: in.AgentSummary, UseWhen: lists[1], AvoidWhen: lists[2], Prerequisites: lists[3], Tips: lists[4], Examples: lists[5]},
	}
}

func overviewToProto(in SchemaOverview) *schemacachepb.SchemaOverviewCache {
	products := &schemacachepb.OverviewProductList{Items: make([]*schemacachepb.OverviewProduct, len(in.Products))}
	for i, product := range in.Products {
		products.Items[i] = &schemacachepb.OverviewProduct{
			Id: product.ID, ToolCount: product.ToolCount, SchemaPath: product.SchemaPath,
			SummaryKind: overviewSummaryKindToProto(product.SummaryKind), Summary: product.Summary,
		}
	}
	return &schemacachepb.SchemaOverviewCache{
		Registry: &schemacachepb.RegistryFields{Kind: in.Kind, Level: in.Level, Source: in.Source, AgentMetadata: bytesToProto(in.AgentMetadata)},
		Products: products, ToolCount: in.ToolCount,
	}
}

func overviewFromProto(in *schemacachepb.SchemaOverviewCache) SchemaOverview {
	fields := in.GetRegistry()
	out := SchemaOverview{
		Kind: fields.GetKind(), Level: fields.GetLevel(), Source: fields.GetSource(), AgentMetadata: bytesFromProto(fields.GetAgentMetadata()),
		ToolCount: in.GetToolCount(),
	}
	if in.GetProducts() != nil {
		out.Products = make([]OverviewProduct, len(in.Products.Items))
		for i, product := range in.Products.Items {
			out.Products[i] = OverviewProduct{
				ID: product.GetId(), ToolCount: product.GetToolCount(), SchemaPath: product.GetSchemaPath(),
				SummaryKind: overviewSummaryKindFromProto(product.GetSummaryKind()), Summary: product.GetSummary(),
			}
		}
	}
	return out
}

func overviewSummaryKindToProto(in OverviewSummaryKind) schemacachepb.OverviewSummaryKind {
	switch in {
	case OverviewSummaryAgentSummary:
		return schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_AGENT_SUMMARY
	case OverviewSummaryUseWhen:
		return schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_USE_WHEN
	case OverviewSummaryDescription:
		return schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_DESCRIPTION
	default:
		return schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_UNSPECIFIED
	}
}

func overviewSummaryKindFromProto(in schemacachepb.OverviewSummaryKind) OverviewSummaryKind {
	switch in {
	case schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_AGENT_SUMMARY:
		return OverviewSummaryAgentSummary
	case schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_USE_WHEN:
		return OverviewSummaryUseWhen
	case schemacachepb.OverviewSummaryKind_OVERVIEW_SUMMARY_KIND_DESCRIPTION:
		return OverviewSummaryDescription
	default:
		return OverviewSummaryNone
	}
}

func locatorsToProto(in map[string]string) *schemacachepb.LocatorEntryList {
	keys := sortedMapKeys(in)
	out := &schemacachepb.LocatorEntryList{Items: make([]*schemacachepb.LocatorEntry, len(keys))}
	for i, key := range keys {
		out.Items[i] = &schemacachepb.LocatorEntry{LookupPath: key, ProductId: in[key]}
	}
	return out
}

func descriptorsToProto(in []ProductDescriptor) *schemacachepb.ProductDescriptorList {
	out := &schemacachepb.ProductDescriptorList{Items: make([]*schemacachepb.ProductDescriptor, len(in))}
	for i, descriptor := range in {
		out.Items[i] = &schemacachepb.ProductDescriptor{
			ProductId: descriptor.ProductID, Offset: descriptor.Offset, Length: descriptor.Length, Sha256: cloneBytes(descriptor.SHA256[:]),
		}
	}
	return out
}

func descriptorsFromProto(in *schemacachepb.ProductDescriptorList) []ProductDescriptor {
	if in == nil {
		return nil
	}
	out := make([]ProductDescriptor, len(in.Items))
	for i, descriptor := range in.Items {
		out[i] = ProductDescriptor{ProductID: descriptor.GetProductId(), Offset: descriptor.GetOffset(), Length: descriptor.GetLength()}
		copy(out[i].SHA256[:], descriptor.GetSha256())
	}
	return out
}

func productToProto(in ProductSpec) (*schemacachepb.ProductSpec, error) {
	tools, err := toolsToProto(in.Tools)
	if err != nil {
		return nil, err
	}
	selection, err := selectionToProtoExact(in.Selection)
	if err != nil {
		return nil, err
	}
	return &schemacachepb.ProductSpec{
		Id: in.ID, Name: in.Name, Description: in.Description, Runtime: in.Runtime, Tools: tools,
		Selection: selection, FieldProvenance: provenanceToProto(in.FieldProvenance),
	}, nil
}

func productFromProto(in *schemacachepb.ProductSpec) ProductSpec {
	return ProductSpec{
		ID: in.GetId(), Name: in.GetName(), Description: in.GetDescription(), Runtime: in.GetRuntime(),
		Tools: toolsFromProto(in.GetTools()), Selection: selectionFromProto(in.GetSelection()), FieldProvenance: provenanceFromProto(in.GetFieldProvenance()),
	}
}

func toolsToProto(in []ToolSpec) (*schemacachepb.ToolList, error) {
	if in == nil {
		return nil, nil
	}
	out := &schemacachepb.ToolList{Items: make([]*schemacachepb.ToolSpec, len(in))}
	for i, tool := range in {
		result, err := resultToProto(tool.Result)
		if err != nil {
			return nil, fmt.Errorf("tool %q result: %w", tool.Identity.CanonicalPath, err)
		}
		selection, err := selectionToProtoExact(tool.Selection)
		if err != nil {
			return nil, fmt.Errorf("tool %q selection: %w", tool.Identity.CanonicalPath, err)
		}
		out.Items[i] = &schemacachepb.ToolSpec{
			Identity: toolIdentityToProto(tool.Identity), Display: tool.Display, Title: tool.Title, Description: tool.Description,
			MetadataSource: tool.MetadataSource, Parameters: parametersToProto(tool.Parameters), Constraints: constraintsToProto(tool.Constraints),
			Positionals: positionalsToProto(tool.Positionals), DryRun: dryRunToProto(tool.DryRun), Result: result,
			Pagination: paginationToProto(tool.Pagination), Safety: safetyToProto(tool.Safety), Interface: interfaceToProto(tool.Interface),
			Selection: selection, FieldProvenance: provenanceToProto(tool.FieldProvenance),
		}
	}
	return out, nil
}

func toolsFromProto(in *schemacachepb.ToolList) []ToolSpec {
	if in == nil {
		return nil
	}
	out := make([]ToolSpec, len(in.Items))
	for i, tool := range in.Items {
		out[i] = ToolSpec{
			Identity: toolIdentityFromProto(tool.GetIdentity()), Display: tool.GetDisplay(), Title: tool.GetTitle(), Description: tool.GetDescription(),
			MetadataSource: tool.GetMetadataSource(), Parameters: parametersFromProto(tool.GetParameters()), Constraints: constraintsFromProto(tool.GetConstraints()),
			Positionals: positionalsFromProto(tool.GetPositionals()), DryRun: dryRunFromProto(tool.GetDryRun()), Result: resultFromProto(tool.GetResult()),
			Pagination: paginationFromProto(tool.GetPagination()), Safety: safetyFromProto(tool.GetSafety()), Interface: interfaceFromProto(tool.GetInterface()),
			Selection: selectionFromProto(tool.GetSelection()), FieldProvenance: provenanceFromProto(tool.GetFieldProvenance()),
		}
	}
	return out
}

func parametersToProto(in []ParameterSpec) *schemacachepb.ParameterList {
	if in == nil {
		return nil
	}
	out := &schemacachepb.ParameterList{Items: make([]*schemacachepb.ParameterSpec, len(in))}
	for i, parameter := range in {
		out.Items[i] = &schemacachepb.ParameterSpec{
			Name: parameter.Name, Type: parameter.Type, Description: parameter.Description, Property: parameter.Property,
			Required: parameter.Required, CliRequired: parameter.CLIRequired, RequiredWhen: parameter.RequiredWhen,
			DefaultValue: bytesToProto(parameter.Default), InterfaceDefault: bytesToProto(parameter.InterfaceDefault), Example: bytesToProto(parameter.Example),
			Format: parameter.Format, Enum: stringsToProto(parameter.Enum), InterfaceDescription: parameter.InterfaceDescription,
			InterfaceType: parameter.InterfaceType, FieldProvenance: provenanceToProto(parameter.FieldProvenance),
		}
	}
	return out
}

func parametersFromProto(in *schemacachepb.ParameterList) []ParameterSpec {
	if in == nil {
		return nil
	}
	out := make([]ParameterSpec, len(in.Items))
	for i, parameter := range in.Items {
		out[i] = ParameterSpec{
			Name: parameter.GetName(), Type: parameter.GetType(), Description: parameter.GetDescription(), Property: parameter.GetProperty(),
			Required: parameter.GetRequired(), CLIRequired: parameter.GetCliRequired(), RequiredWhen: parameter.GetRequiredWhen(),
			Default: bytesFromProto(parameter.GetDefaultValue()), InterfaceDefault: bytesFromProto(parameter.GetInterfaceDefault()), Example: bytesFromProto(parameter.GetExample()),
			Format: parameter.GetFormat(), Enum: stringsFromProto(parameter.GetEnum()), InterfaceDescription: parameter.GetInterfaceDescription(),
			InterfaceType: parameter.GetInterfaceType(), FieldProvenance: provenanceFromProto(parameter.GetFieldProvenance()),
		}
	}
	return out
}

func toolIdentityToProto(in contract.ToolIdentitySpec) *schemacachepb.ToolIdentity {
	return &schemacachepb.ToolIdentity{
		ProductId: in.ProductID, SourceProductId: in.SourceProductID, Name: in.Name, CliName: in.CLIName,
		CanonicalPath: in.CanonicalPath, Path: in.Path, CliPath: in.CLIPath, PrimaryCliPath: in.PrimaryCLIPath,
		Group: in.Group, Aliases: stringsToProto(in.Aliases), IsAlias: in.IsAlias, Source: in.Source,
	}
}

func toolIdentityFromProto(in *schemacachepb.ToolIdentity) contract.ToolIdentitySpec {
	return contract.ToolIdentitySpec{
		ProductID: in.GetProductId(), SourceProductID: in.GetSourceProductId(), Name: in.GetName(), CLIName: in.GetCliName(),
		CanonicalPath: in.GetCanonicalPath(), Path: in.GetPath(), CLIPath: in.GetCliPath(), PrimaryCLIPath: in.GetPrimaryCliPath(),
		Group: in.GetGroup(), Aliases: stringsFromProto(in.GetAliases()), IsAlias: in.GetIsAlias(), Source: in.GetSource(),
	}
}

func constraintsToProto(in contract.RuntimeSchemaConstraints) *schemacachepb.Constraints {
	return &schemacachepb.Constraints{
		MutuallyExclusive: stringListsToProto(in.MutuallyExclusive), RequireOneOf: stringListsToProto(in.RequireOneOf), RequireTogether: stringListsToProto(in.RequireTogether),
	}
}

func constraintsFromProto(in *schemacachepb.Constraints) contract.RuntimeSchemaConstraints {
	if in == nil {
		return contract.RuntimeSchemaConstraints{}
	}
	return contract.RuntimeSchemaConstraints{
		MutuallyExclusive: stringListsFromProto(in.GetMutuallyExclusive()), RequireOneOf: stringListsFromProto(in.GetRequireOneOf()), RequireTogether: stringListsFromProto(in.GetRequireTogether()),
	}
}

func positionalsToProto(in []contract.RuntimeSchemaPositional) *schemacachepb.PositionalList {
	if in == nil {
		return nil
	}
	out := &schemacachepb.PositionalList{Items: make([]*schemacachepb.Positional, len(in))}
	for i, value := range in {
		out.Items[i] = &schemacachepb.Positional{Name: value.Name, Type: value.Type, Description: value.Description, Required: value.Required, Variadic: value.Variadic, Index: int64(value.Index)}
	}
	return out
}

func positionalsFromProto(in *schemacachepb.PositionalList) []contract.RuntimeSchemaPositional {
	if in == nil {
		return nil
	}
	out := make([]contract.RuntimeSchemaPositional, len(in.Items))
	for i, value := range in.Items {
		out[i] = contract.RuntimeSchemaPositional{Name: value.GetName(), Type: value.GetType(), Description: value.GetDescription(), Required: value.GetRequired(), Variadic: value.GetVariadic(), Index: int(value.GetIndex())}
	}
	return out
}

func dryRunToProto(in *contract.DryRunSpec) *schemacachepb.DryRun {
	if in == nil {
		return nil
	}
	return &schemacachepb.DryRun{PreviewKind: in.PreviewKind, RemoteReads: in.RemoteReads}
}

func dryRunFromProto(in *schemacachepb.DryRun) *contract.DryRunSpec {
	if in == nil {
		return nil
	}
	return &contract.DryRunSpec{PreviewKind: in.GetPreviewKind(), RemoteReads: in.GetRemoteReads()}
}

func resultToProto(in *contract.ResultSpec) (*schemacachepb.Result, error) {
	if in == nil {
		return nil, nil
	}
	outcomes := &schemacachepb.ResultOutcomeList{Items: make([]schemacachepb.ResultOutcome, len(in.Outcomes))}
	for i, outcome := range in.Outcomes {
		var ok bool
		outcomes.Items[i], ok = resultOutcomeToProto(outcome)
		if !ok {
			return nil, fmt.Errorf("unsupported outcome %q", outcome)
		}
	}
	return &schemacachepb.Result{Outcomes: outcomes, DataSchema: bytesToProto(in.DataSchema), SensitivePaths: stringsToProto(in.SensitivePaths)}, nil
}

func resultFromProto(in *schemacachepb.Result) *contract.ResultSpec {
	if in == nil {
		return nil
	}
	out := &contract.ResultSpec{DataSchema: bytesFromProto(in.GetDataSchema()), SensitivePaths: stringsFromProto(in.GetSensitivePaths())}
	if in.GetOutcomes() != nil {
		out.Outcomes = make([]contract.ResultOutcome, len(in.Outcomes.Items))
		for i, outcome := range in.Outcomes.Items {
			out.Outcomes[i] = resultOutcomeFromProto(outcome)
		}
	}
	return out
}

func resultOutcomeToProto(in contract.ResultOutcome) (schemacachepb.ResultOutcome, bool) {
	switch in {
	case contract.ResultOutcomeSuccess:
		return schemacachepb.ResultOutcome_RESULT_OUTCOME_SUCCESS, true
	case contract.ResultOutcomePending:
		return schemacachepb.ResultOutcome_RESULT_OUTCOME_PENDING, true
	case contract.ResultOutcomePartialFailure:
		return schemacachepb.ResultOutcome_RESULT_OUTCOME_PARTIAL_FAILURE, true
	case contract.ResultOutcomeFailure:
		return schemacachepb.ResultOutcome_RESULT_OUTCOME_FAILURE, true
	default:
		return schemacachepb.ResultOutcome_RESULT_OUTCOME_UNSPECIFIED, false
	}
}

func resultOutcomeFromProto(in schemacachepb.ResultOutcome) contract.ResultOutcome {
	switch in {
	case schemacachepb.ResultOutcome_RESULT_OUTCOME_SUCCESS:
		return contract.ResultOutcomeSuccess
	case schemacachepb.ResultOutcome_RESULT_OUTCOME_PENDING:
		return contract.ResultOutcomePending
	case schemacachepb.ResultOutcome_RESULT_OUTCOME_PARTIAL_FAILURE:
		return contract.ResultOutcomePartialFailure
	case schemacachepb.ResultOutcome_RESULT_OUTCOME_FAILURE:
		return contract.ResultOutcomeFailure
	default:
		return ""
	}
}

func paginationToProto(in *contract.PaginationSpec) *schemacachepb.Pagination {
	if in == nil {
		return nil
	}
	return &schemacachepb.Pagination{Kind: in.Kind, CursorParameter: in.CursorParameter, MetaPath: in.MetaPath, EndpointExhaustedPath: in.EndpointExhaustedPath, NextTokenPath: in.NextTokenPath}
}

func paginationFromProto(in *schemacachepb.Pagination) *contract.PaginationSpec {
	if in == nil {
		return nil
	}
	return &contract.PaginationSpec{Kind: in.GetKind(), CursorParameter: in.GetCursorParameter(), MetaPath: in.GetMetaPath(), EndpointExhaustedPath: in.GetEndpointExhaustedPath(), NextTokenPath: in.GetNextTokenPath()}
}

func safetyToProto(in contract.SafetySpec) *schemacachepb.Safety {
	return &schemacachepb.Safety{Effect: in.Effect, EffectSource: in.EffectSource, Risk: in.Risk, Confirmation: in.Confirmation, Idempotency: in.Idempotency}
}

func safetyFromProto(in *schemacachepb.Safety) contract.SafetySpec {
	return contract.SafetySpec{Effect: in.GetEffect(), EffectSource: in.GetEffectSource(), Risk: in.GetRisk(), Confirmation: in.GetConfirmation(), Idempotency: in.GetIdempotency()}
}

func interfaceToProto(in contract.InterfaceSpec) *schemacachepb.Interface {
	out := &schemacachepb.Interface{Mode: in.Mode, Availability: in.Availability, Reason: in.Reason}
	if in.Ref != nil {
		out.Ref = &schemacachepb.InterfaceRef{ProductId: in.Ref.ProductID, RpcName: in.Ref.RPCName}
	}
	return out
}

func interfaceFromProto(in *schemacachepb.Interface) contract.InterfaceSpec {
	if in == nil {
		return contract.InterfaceSpec{}
	}
	out := contract.InterfaceSpec{Mode: in.GetMode(), Availability: in.GetAvailability(), Reason: in.GetReason()}
	if in.GetRef() != nil {
		out.Ref = &contract.InterfaceRefSpec{ProductID: in.Ref.GetProductId(), RPCName: in.Ref.GetRpcName()}
	}
	return out
}

func selectionToProtoExact(in contract.SelectionSpec) (*schemacachepb.Selection, error) {
	dispositions, err := dispositionsToProto(in.ExampleDispositions)
	if err != nil {
		return nil, err
	}
	return &schemacachepb.Selection{
		AgentSummary: in.AgentSummary, AgentSummarySource: in.AgentSummarySource, UseWhen: stringsToProto(in.UseWhen), AvoidWhen: stringsToProto(in.AvoidWhen),
		Prerequisites: stringsToProto(in.Prerequisites), Tips: stringsToProto(in.Tips), WorkflowRefs: stringsToProto(in.WorkflowRefs), Examples: stringsToProto(in.Examples),
		ExampleDispositions: dispositions, Reviewed: boolToProto(in.Reviewed), SourceRefs: stringsToProto(in.SourceRefs), MetadataSource: in.MetadataSource,
	}, nil
}

func selectionFromProto(in *schemacachepb.Selection) contract.SelectionSpec {
	if in == nil {
		return contract.SelectionSpec{}
	}
	return contract.SelectionSpec{
		AgentSummary: in.GetAgentSummary(), AgentSummarySource: in.GetAgentSummarySource(), UseWhen: stringsFromProto(in.GetUseWhen()), AvoidWhen: stringsFromProto(in.GetAvoidWhen()),
		Prerequisites: stringsFromProto(in.GetPrerequisites()), Tips: stringsFromProto(in.GetTips()), WorkflowRefs: stringsFromProto(in.GetWorkflowRefs()), Examples: stringsFromProto(in.GetExamples()),
		ExampleDispositions: dispositionsFromProto(in.GetExampleDispositions()), Reviewed: boolFromProto(in.GetReviewed()), SourceRefs: stringsFromProto(in.GetSourceRefs()), MetadataSource: in.GetMetadataSource(),
	}
}

func dispositionsToProto(in []contract.ExampleDisposition) (*schemacachepb.ExampleDispositionList, error) {
	if in == nil {
		return nil, nil
	}
	out := &schemacachepb.ExampleDispositionList{Items: make([]*schemacachepb.ExampleDisposition, len(in))}
	for i, value := range in {
		mode, ok := dispositionModeToProto(value.Mode)
		if !ok {
			return nil, fmt.Errorf("unsupported disposition mode %q", value.Mode)
		}
		reason, ok := dispositionReasonToProto(value.ReasonCode)
		if !ok {
			return nil, fmt.Errorf("unsupported disposition reason code %q", value.ReasonCode)
		}
		out.Items[i] = &schemacachepb.ExampleDisposition{Index: intToProto(value.Index), Mode: mode, ReasonCode: reason, Reason: value.Reason, Reviewed: value.Reviewed}
	}
	return out, nil
}

func dispositionsFromProto(in *schemacachepb.ExampleDispositionList) []contract.ExampleDisposition {
	if in == nil {
		return nil
	}
	out := make([]contract.ExampleDisposition, len(in.Items))
	for i, value := range in.Items {
		out[i] = contract.ExampleDisposition{
			Index: intFromProto(value.GetIndex()), Mode: dispositionModeFromProto(value.GetMode()), ReasonCode: dispositionReasonFromProto(value.GetReasonCode()),
			Reason: value.GetReason(), Reviewed: value.GetReviewed(),
		}
	}
	return out
}

func dispositionModeToProto(in contract.ExampleDispositionMode) (schemacachepb.ExampleDispositionMode, bool) {
	switch in {
	case contract.ExampleDispositionModeContract:
		return schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT, true
	case contract.ExampleDispositionModeDryRun:
		return schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_DRY_RUN, true
	case contract.ExampleDispositionModeContractOnly:
		return schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT_ONLY, true
	default:
		return schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_UNSPECIFIED, false
	}
}

func dispositionModeFromProto(in schemacachepb.ExampleDispositionMode) contract.ExampleDispositionMode {
	switch in {
	case schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT:
		return contract.ExampleDispositionModeContract
	case schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_DRY_RUN:
		return contract.ExampleDispositionModeDryRun
	case schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_CONTRACT_ONLY:
		return contract.ExampleDispositionModeContractOnly
	default:
		return ""
	}
}

func dispositionReasonToProto(in contract.ExampleDispositionReasonCode) (schemacachepb.ExampleDispositionReasonCode, bool) {
	switch in {
	case contract.ExampleDispositionReasonLocalState:
		return schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_LOCAL_STATE, true
	case contract.ExampleDispositionReasonStatefulPreflight:
		return schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_STATEFUL_PREFLIGHT, true
	default:
		return schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_UNSPECIFIED, false
	}
}

func dispositionReasonFromProto(in schemacachepb.ExampleDispositionReasonCode) contract.ExampleDispositionReasonCode {
	switch in {
	case schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_LOCAL_STATE:
		return contract.ExampleDispositionReasonLocalState
	case schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_STATEFUL_PREFLIGHT:
		return contract.ExampleDispositionReasonStatefulPreflight
	default:
		return ""
	}
}

func provenanceToProto(in map[string]contract.FieldProvenance) *schemacachepb.ProvenanceList {
	if in == nil {
		return nil
	}
	keys := sortedMapKeys(in)
	out := &schemacachepb.ProvenanceList{Items: make([]*schemacachepb.ProvenanceEntry, len(keys))}
	for i, key := range keys {
		value := in[key]
		out.Items[i] = &schemacachepb.ProvenanceEntry{Key: key, Value: &schemacachepb.FieldProvenance{
			Value: bytesToProto(value.Value), Source: value.Source, SourceRef: value.SourceRef, Precedence: value.Precedence,
			Resolution: value.Resolution, ReviewReason: value.ReviewReason, Candidates: candidatesToProto(value.Candidates), OverriddenCandidates: candidatesToProto(value.OverriddenCandidates),
		}}
	}
	return out
}

func provenanceFromProto(in *schemacachepb.ProvenanceList) map[string]contract.FieldProvenance {
	if in == nil {
		return nil
	}
	out := make(map[string]contract.FieldProvenance, len(in.Items))
	for _, entry := range in.Items {
		value := entry.GetValue()
		out[entry.GetKey()] = contract.FieldProvenance{
			Value: bytesFromProto(value.GetValue()), Source: value.GetSource(), SourceRef: value.GetSourceRef(), Precedence: value.GetPrecedence(),
			Resolution: value.GetResolution(), ReviewReason: value.GetReviewReason(), Candidates: candidatesFromProto(value.GetCandidates()), OverriddenCandidates: candidatesFromProto(value.GetOverriddenCandidates()),
		}
	}
	return out
}

func candidatesToProto(in []contract.FieldCandidateProvenance) *schemacachepb.CandidateList {
	if in == nil {
		return nil
	}
	out := &schemacachepb.CandidateList{Items: make([]*schemacachepb.FieldCandidate, len(in))}
	for i, value := range in {
		out.Items[i] = &schemacachepb.FieldCandidate{
			Value: bytesToProto(value.Value), Source: value.Source, SourceRef: value.SourceRef, Precedence: value.Precedence, ReviewReason: value.ReviewReason, Selected: boolToProto(value.Selected),
		}
	}
	return out
}

func candidatesFromProto(in *schemacachepb.CandidateList) []contract.FieldCandidateProvenance {
	if in == nil {
		return nil
	}
	out := make([]contract.FieldCandidateProvenance, len(in.Items))
	for i, value := range in.Items {
		out[i] = contract.FieldCandidateProvenance{
			Value: bytesFromProto(value.GetValue()), Source: value.GetSource(), SourceRef: value.GetSourceRef(), Precedence: value.GetPrecedence(), ReviewReason: value.GetReviewReason(), Selected: boolFromProto(value.GetSelected()),
		}
	}
	return out
}

func stringsToProto(in []string) *schemacachepb.StringList {
	if in == nil {
		return nil
	}
	return &schemacachepb.StringList{Items: cloneSlice(in)}
}

func stringsFromProto(in *schemacachepb.StringList) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in.Items))
	copy(out, in.Items)
	return out
}

func stringListsToProto(in [][]string) *schemacachepb.StringListList {
	if in == nil {
		return nil
	}
	out := &schemacachepb.StringListList{Items: make([]*schemacachepb.StringList, len(in))}
	for i, item := range in {
		out.Items[i] = stringsToProto(item)
	}
	return out
}

func stringListsFromProto(in *schemacachepb.StringListList) [][]string {
	if in == nil {
		return nil
	}
	out := make([][]string, len(in.Items))
	for i, item := range in.Items {
		out[i] = stringsFromProto(item)
	}
	return out
}

func bytesToProto[T ~[]byte](in T) *schemacachepb.BytesValue {
	if in == nil {
		return nil
	}
	return &schemacachepb.BytesValue{Value: cloneBytes(in)}
}

func bytesFromProto(in *schemacachepb.BytesValue) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in.Value))
	copy(out, in.Value)
	return out
}

func boolToProto(in *bool) *schemacachepb.BoolValue {
	if in == nil {
		return nil
	}
	return &schemacachepb.BoolValue{Value: *in}
}

func boolFromProto(in *schemacachepb.BoolValue) *bool {
	if in == nil {
		return nil
	}
	value := in.GetValue()
	return &value
}

func intToProto(in *int) *schemacachepb.IntValue {
	if in == nil {
		return nil
	}
	return &schemacachepb.IntValue{Value: int64(*in)}
}

func intFromProto(in *schemacachepb.IntValue) *int {
	if in == nil {
		return nil
	}
	value := int(in.GetValue())
	return &value
}

func sortedMapKeys[V any](in map[string]V) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneProductExact(in ProductSpec) ProductSpec {
	out := in
	out.Tools = cloneSlice(in.Tools)
	for i := range out.Tools {
		out.Tools[i].Parameters = cloneSlice(out.Tools[i].Parameters)
		sort.SliceStable(out.Tools[i].Parameters, func(a, b int) bool { return out.Tools[i].Parameters[a].Name < out.Tools[i].Parameters[b].Name })
	}
	sort.SliceStable(out.Tools, func(i, j int) bool {
		if out.Tools[i].Identity.CanonicalPath != out.Tools[j].Identity.CanonicalPath {
			return out.Tools[i].Identity.CanonicalPath < out.Tools[j].Identity.CanonicalPath
		}
		return out.Tools[i].Identity.CLIPath < out.Tools[j].Identity.CLIPath
	})
	return out
}
