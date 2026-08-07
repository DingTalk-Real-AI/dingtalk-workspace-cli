// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

type runtimeSchemaMetadataSources struct {
	// Agent remains only for historical test seams that still construct this
	// struct; production assembly does not overlay Agent or MCP pin onto
	// parameters or tool text.
	Agent agentMetadata
}

var (
	resolveEffectiveCommandRegistry  = BuildEffectiveCommandRegistry
	resolveBoundCommandRegistry      = BindEffectiveCommandRegistry
	resolveAssembleSchemaRegistry    = AssembleSchemaRegistryFromBound
	resolveValidateParameterDelivery = ValidateSchemaParameterBindingDelivery
	assembleValidateBindings         = ValidateSchemaParameterBindings
	assembleCollectEntries           = collectRuntimeSchemaEntriesFromBound
	assembleRuntimeToolSpec          = runtimeToolSpecFromMetadata
	assembleTypedRegistry            = SchemaRegistryFromRuntime
	assembleMarshalRaw               = marshalSchemaRaw
	resolveRuntimeParameters         = runtimeCommandParameterSpecs
	finalSchemaAgentMetadata         = runtimeAgentMetadata
)

// ResolvedSchemaBuild is the single source-to-delivery hand-off used by the
// Catalog generator. Effective, Bound, and Registry are three views of one
// resolution pass: reviewed identity, executable Cobra binding, and the final
// typed Agent contract. Downstream gates and serializers must consume this
// value instead of rebuilding any view from the command tree.
//
// The command root is intentionally private. It lets delivery completeness
// inspect the same executable tree without allowing callers to construct a
// seemingly resolved build by assembling the exported fields themselves.
type ResolvedSchemaBuild struct {
	effective EffectiveCommandRegistry
	bound     BoundCommandRegistry
	registry  SchemaRegistry
	root      *cobra.Command
}

// RegistryHash returns the semantic identity/navigation hash attached to this
// resolved build. It is an envelope value, not a second registry input.
func (resolved ResolvedSchemaBuild) RegistryHash() string {
	return resolved.effective.SourceHash()
}

// CommandCount reports the reviewed effective command count for generator
// diagnostics without exposing a mutable registry view.
func (resolved ResolvedSchemaBuild) CommandCount() int {
	return len(resolved.effective.Commands)
}

func pinnedRuntimeSchemaMetadataSources() runtimeSchemaMetadataSources {
	// Production pin and Agent inject are both retired; assembly is Contract /
	// ParamDecl / Cobra only.
	return runtimeSchemaMetadataSources{}
}

// ResolveSchemaBuild is the only assembly path from executable Cobra commands
// and reviewed metadata into the typed Agent contract. It resolves identity
// once, binds Cobra once, and assembles one SchemaRegistry from ContractFinal /
// contract.ProductDecl leaf declarations. Catalog gates and serialization consume the
// returned value directly; they never re-read overlays or merge sources.
func ResolveSchemaBuild(root *cobra.Command) (ResolvedSchemaBuild, error) {
	if root == nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("resolve Schema build: root is nil")
	}
	effective, err := resolveEffectiveCommandRegistry(root)
	if err != nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("build effective Schema command registry: %w", err)
	}
	bound, err := resolveBoundCommandRegistry(root, effective)
	if err != nil {
		return ResolvedSchemaBuild{}, fmt.Errorf("bind effective Schema command registry: %w", err)
	}
	registry, err := resolveAssembleSchemaRegistry(bound)
	if err != nil {
		return ResolvedSchemaBuild{}, err
	}
	return ResolvedSchemaBuild{
		effective: effective,
		bound:     bound,
		registry:  registry,
		root:      root,
	}, nil
}

// AssembleSchemaRegistry is a test/homology gate entry that only needs the
// typed registry. Catalog production must use ResolveSchemaBuild so the
// bound/effective views remain attached to the exact same resolution pass.
func AssembleSchemaRegistry(root *cobra.Command) (SchemaRegistry, error) {
	resolved, err := ResolveSchemaBuild(root)
	if err != nil {
		return SchemaRegistry{}, err
	}
	if err := resolveValidateParameterDelivery(resolved.bound, resolved.registry); err != nil {
		return SchemaRegistry{}, fmt.Errorf("validate final Schema parameter binding delivery: %w", err)
	}
	return resolved.registry, nil
}

// AssembleSchemaRegistryFromBound resolves non-identity sources into the
// single typed ToolSpec model. Command discovery is intentionally impossible
// below this boundary: callers must first provide a fail-closed bound registry.
func AssembleSchemaRegistryFromBound(bound BoundCommandRegistry) (SchemaRegistry, error) {
	if err := assembleValidateBindings(); err != nil {
		return SchemaRegistry{}, fmt.Errorf("validate reviewed Schema parameter bindings: %w", err)
	}
	return assembleSchemaRegistryFromBound(bound, pinnedRuntimeSchemaMetadataSources())
}

// assembleSchemaRegistryFromBound resolves every entry through the
// ContractFinal / ProductDecl production path. Missing declarations fail
// closed; retired skill/MCP-pin/agent-inject overlays are never reopened.
func assembleSchemaRegistryFromBound(bound BoundCommandRegistry, metadata runtimeSchemaMetadataSources) (SchemaRegistry, error) {
	entries, err := assembleCollectEntries(bound)
	if err != nil {
		return SchemaRegistry{}, err
	}
	byProduct := make(map[string]*ProductSpec)
	for _, entry := range entries {
		tool, err := assembleRuntimeToolSpec(entry, metadata)
		if err != nil {
			return SchemaRegistry{}, err
		}
		product := byProduct[entry.ProductID]
		if product == nil {
			selection, provenance, err := assembleProductSelection(entry)
			if err != nil {
				return SchemaRegistry{}, err
			}
			product = &ProductSpec{
				ID:              entry.ProductID,
				Name:            entry.ProductName,
				Description:     entry.ProductName,
				Runtime:         true,
				Selection:       selection,
				FieldProvenance: provenance,
			}
			byProduct[entry.ProductID] = product
		}
		product.Tools = append(product.Tools, tool)
	}

	productIDs := make([]string, 0, len(byProduct))
	for productID := range byProduct {
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	products := make([]ProductSpec, 0, len(productIDs))
	for _, productID := range productIDs {
		products = append(products, *byProduct[productID])
	}
	registry, err := assembleTypedRegistry("runtime-command", products)
	if err != nil {
		return SchemaRegistry{}, fmt.Errorf("build typed Schema registry: %w", err)
	}
	// Derive Agent metadata summary from the assembled ContractFinal /
	// ProductDecl surface so runtime delivery and cmd_schema_catalog dumps
	// share one Catalog blob (inject remains validation-only for the dump).
	registry.AgentMetadata, err = assembleMarshalRaw(agentMetadataSummaryFromProducts(products))
	if err != nil {
		return SchemaRegistry{}, fmt.Errorf("encode Agent metadata summary: %w", err)
	}
	return registry, nil
}

func runtimeToolSpecFromMetadata(entry runtimeSchemaEntry, metadata runtimeSchemaMetadataSources) (ToolSpec, error) {
	if final, ok := contractfinal.RuntimeContractFinal(entry.Command); ok {
		return runtimeToolSpecFromContractFinal(entry, final, metadata)
	}
	canonicalPath := entry.ProductID + "." + entry.ToolName
	return ToolSpec{}, fmt.Errorf("assemble Schema tool %s: missing RuntimeContractFinal (legacy skill/MCP/agent-metadata assembly is retired)", canonicalPath)
}

// assembleProductSelection requires ProductDecl: the legacy agent-product
// JSON overlay is retired, so a missing declaration fails closed.
func assembleProductSelection(entry runtimeSchemaEntry) (contract.SelectionSpec, map[string]contract.FieldProvenance, error) {
	if decl, ok := contract.LookupProductDecl(entry.ProductID); ok {
		selection, provenance := contract.ProductSelectionFromDecl(decl)
		return selection, provenance, nil
	}
	return contract.SelectionSpec{}, nil, fmt.Errorf("assemble Schema product %q: missing ProductDecl (legacy agent-metadata product selection is retired)", entry.ProductID)
}

// runtimeToolSpecFromContractFinal pass-throughs Contract-authored Schema fields.
// Declared values are the final data source; hints/registry text does not merge.
// MCP pin is retired: interface_type / interface_* facts come from ParamDecl /
// native annotations only.
func runtimeToolSpecFromContractFinal(entry runtimeSchemaEntry, final contract.ContractFinalPayload, metadata runtimeSchemaMetadataSources) (ToolSpec, error) {
	_ = metadata // reserved for historical assemble seams; no overlay sources remain
	canonicalPath := entry.ProductID + "." + entry.ToolName
	constraints := runtimeCommandConstraints(entry.Command)
	// Apply parameter declarations from the contract.ContractFinalPayload before the
	// resolver reads them. The decls were put there by AttachContract at
	// DeclareLeafMetadata time; now that all flags exist on the fully-built
	// command tree, they can be emitted as dws.schema.* annotations.
	if err := ApplyParamDecls(entry.Command, final.Parameters); err != nil {
		return ToolSpec{}, fmt.Errorf("apply Contract Schema ParamDecls for %s: %w", canonicalPath, err)
	}
	parameters, err := resolveRuntimeParameters(entry.Command, canonicalPath, constraints)
	if err != nil {
		return ToolSpec{}, fmt.Errorf("resolve Contract Schema parameters for %s: %w", canonicalPath, err)
	}

	identity := contract.ToolIdentitySpec{
		ProductID:       entry.ProductID,
		SourceProductID: strings.TrimSpace(entry.SourceProductID),
		Name:            entry.ToolName,
		CLIName:         entry.CLIName,
		CanonicalPath:   canonicalPath,
		Path:            canonicalPath,
		CLIPath:         entry.CLIPath,
		PrimaryCLIPath:  entry.PrimaryCLIPath,
		Group:           entry.Group,
		Aliases:         append([]string(nil), entry.Aliases...),
		IsAlias:         false,
		Source:          entry.Source,
	}
	if final.Identity != nil {
		if err := validateContractFinalIdentity(entry, *final.Identity, canonicalPath); err != nil {
			return ToolSpec{}, err
		}
		id := *final.Identity
		if id.ProductID != "" {
			identity.ProductID = id.ProductID
		}
		if id.SourceProductID != "" {
			identity.SourceProductID = id.SourceProductID
		}
		if id.Name != "" {
			identity.Name = id.Name
		}
		if id.CLIName != "" {
			identity.CLIName = id.CLIName
		}
		if id.CanonicalPath != "" {
			identity.CanonicalPath = id.CanonicalPath
			identity.Path = id.CanonicalPath
		}
		if id.CLIPath != "" {
			identity.CLIPath = id.CLIPath
		}
		if id.PrimaryCLIPath != "" {
			identity.PrimaryCLIPath = id.PrimaryCLIPath
		}
		if id.Group != "" {
			identity.Group = id.Group
		}
		if len(id.Aliases) > 0 {
			identity.Aliases = append([]string(nil), id.Aliases...)
		}
		if id.Source != "" {
			identity.Source = id.Source
		}
	}
	if identity.SourceProductID == identity.ProductID {
		identity.SourceProductID = ""
	}

	// Text delivery: Cobra Long wins over declared Contract.Description when
	// present (declared Description is mandatory and often a one-line restatement).
	// Declared Title wins over Short. Provenance must name the real winner.
	title, titleProv := contractFinalTextProvenance(
		strings.TrimSpace(final.Title),
		strings.TrimSpace(entry.Command.Short),
		false, // prefer declared
	)
	description, descriptionProv := contractFinalTextProvenance(
		strings.TrimSpace(final.Description),
		strings.TrimSpace(entry.Command.Long),
		true, // prefer cobra help
	)

	safety := contract.SafetySpec{}
	if final.Safety != nil {
		safety = *final.Safety
	} else if risk, ok := RuntimeContractRisk(entry.Command); ok {
		safety = applyContractRiskToSafety(safety, risk)
	} else if gate, ok := RuntimeContractGate(entry.Command); ok {
		safety = applyContractGateToSafety(safety, gate)
	}

	positionals := final.Positionals
	if len(positionals) == 0 {
		positionals = runtimeCommandPositionals(entry.Command)
	}

	var interfaceSpec contract.InterfaceSpec
	if final.Interface != nil {
		interfaceSpec = *final.Interface
	}
	var selection contract.SelectionSpec
	if final.Selection != nil {
		if final.Selection.Reviewed != nil {
			return ToolSpec{}, fmt.Errorf("contract final selection for %s must not carry reviewed field: declaration is the final source", canonicalPath)
		}
		selection = *final.Selection
	}
	// Declared selection provenance metadata: the declaration lives in reviewed
	// source code, so the catalog keeps the same uniform agent_* shape across
	// all tools. Reviewed is assembly-derived (declarations are code-reviewed
	// by construction), never author-provided.
	if strings.TrimSpace(selection.AgentSummarySource) == "" && strings.TrimSpace(selection.AgentSummary) != "" {
		selection.AgentSummarySource = "corecmd.ContractDecl"
	}
	if selection.SourceRefs == nil {
		selection.SourceRefs = []string{"corecmd.ContractDecl"}
	}
	if strings.TrimSpace(selection.MetadataSource) == "" {
		selection.MetadataSource = "corecmd.contract"
	}
	if selection.Reviewed == nil {
		reviewed := true
		selection.Reviewed = &reviewed
	}
	// Example dispositions control only the policy gate's execution eligibility.
	// They remain on ContractFinal for BuildAgentExampleExecutionPlan and are not
	// part of the public ToolSpec / Schema wire contract.
	selection.ExampleDispositions = nil

	provenance := contractFinalProvenance(identity, title, description, titleProv, descriptionProv, safety, interfaceSpec, selection, final.DryRun)

	return ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity:        identity,
		Display:         entry.ProductName,
		Title:           title,
		Description:     description,
		MetadataSource:  "corecmd.contract",
		Parameters:      parameters,
		Constraints:     constraints,
		Positionals:     positionals,
		DryRun:          final.DryRun,
		Result:          final.Result,
		Safety:          safety,
		Interface:       interfaceSpec,
		Selection:       selection,
		FieldProvenance: provenance,
	})
}

// contractFinalTextProvenance picks delivered title/description text and the
// provenance that matches the real winner. preferCobra=true implements the
// Long-over-declared-Description rule; preferCobra=false keeps declared Title
// over Short.
func contractFinalTextProvenance(declared, cobra string, preferCobra bool) (string, contract.FieldProvenance) {
	decl := strings.TrimSpace(declared)
	help := strings.TrimSpace(cobra)
	switch {
	case preferCobra && help != "":
		return help, resolvedFieldProvenance(
			help, "cobra_help", "cobra_help", "cobra_help",
			"cobra_help_preferred", "Cobra Long preferred over ContractDecl text",
		)
	case decl != "":
		return decl, resolvedFieldProvenance(
			decl, "corecmd.contract", "corecmd.ContractDecl", "contract_final",
			"contract_pass_through", "Contract final Schema pass-through",
		)
	case help != "":
		return help, resolvedFieldProvenance(
			help, "cobra_help", "cobra_help", "cobra_help",
			"cobra_help_fallback", "Cobra Long fallback when ContractDecl text is empty",
		)
	default:
		// Keep a winner even when both sides are empty so the final-delivery
		// provenance gate (winner value == delivered value) still holds.
		return "", resolvedFieldProvenance(
			"", "corecmd.contract", "corecmd.ContractDecl", "contract_final",
			"contract_pass_through", "Contract final Schema pass-through",
		)
	}
}

// contractFinalProvenance records one pass-through winner per delivered field.
// The final Schema provenance gate requires a winner for every required tool
// field (safety/interface/agent_summary unconditionally, selection slices and
// dry_run when present), so declared leaves must emit the full set, not only
// the fields they happened to author. Title/description provenance must match
// the real text winner (cobra_help vs contract_final).
func contractFinalProvenance(identity contract.ToolIdentitySpec, title, description string, titleProv, descriptionProv contract.FieldProvenance, safety contract.SafetySpec, iface contract.InterfaceSpec, selection contract.SelectionSpec, dryRun *contract.DryRunSpec) map[string]contract.FieldProvenance {
	prov := func(value any, sourceRef string) contract.FieldProvenance {
		return resolvedFieldProvenance(
			value,
			"corecmd.contract",
			sourceRef,
			"contract_final",
			"contract_pass_through",
			"Contract final Schema pass-through",
		)
	}
	out := map[string]contract.FieldProvenance{
		"canonical_path":  prov(identity.CanonicalPath, "corecmd.ContractDecl"),
		"title":           titleProv,
		"description":     descriptionProv,
		"metadata_source": prov("corecmd.contract", "corecmd.ContractDecl"),
		"effect":          prov(safety.Effect, "corecmd.ContractDecl"),
		"risk":            prov(safety.Risk, "corecmd.ContractDecl"),
		"confirmation":    prov(safety.Confirmation, "corecmd.ContractDecl"),
		"idempotency":     prov(safety.Idempotency, "corecmd.ContractDecl"),
		"interface_mode":  prov(iface.Mode, "corecmd.ContractDecl"),
		"availability":    prov(iface.Availability, "corecmd.ContractDecl"),
		"agent_summary":   prov(selection.AgentSummary, "corecmd.ContractDecl"),
	}
	var ref any
	if iface.Ref != nil {
		ref = *iface.Ref
	}
	out["interface_ref"] = prov(ref, "corecmd.ContractDecl")
	if strings.TrimSpace(iface.Reason) != "" ||
		strings.TrimSpace(iface.Mode) == contract.InterfaceModeComposite ||
		strings.TrimSpace(iface.Availability) == contract.InterfaceUnavailable {
		out["interface_reason"] = prov(iface.Reason, "corecmd.ContractDecl")
	}
	for field, values := range map[string][]string{
		"use_when":      selection.UseWhen,
		"avoid_when":    selection.AvoidWhen,
		"prerequisites": selection.Prerequisites,
		"tips":          selection.Tips,
		"workflow_refs": selection.WorkflowRefs,
		"examples":      selection.Examples,
	} {
		if values != nil {
			out[field] = prov(values, "corecmd.ContractDecl")
		}
	}
	if selection.Reviewed != nil {
		out["reviewed"] = prov(*selection.Reviewed, "corecmd.ContractDecl")
	}
	if dryRun != nil {
		out["dry_run"] = prov(*dryRun, "corecmd.ContractDecl")
	}
	return out
}

// validateContractFinalIdentity guards the pass-through contract: on a bound
// (managed) leaf, a declared identity must agree with the bound tree entry.
// Otherwise the Schema catalog would publish an identity the registry never
// indexed. Registry-owned keys are compared exactly (including empty aliases);
// optional derived fields (cli_name/group/source) still fail only when
// non-empty and disagreeing.
func validateContractFinalIdentity(entry runtimeSchemaEntry, id contract.ToolIdentitySpec, canonicalPath string) error {
	mismatches := make([]string, 0, 10)
	checkExact := func(field, declared, bound string) {
		declared = strings.TrimSpace(declared)
		bound = strings.TrimSpace(bound)
		if declared != bound {
			mismatches = append(mismatches, fmt.Sprintf("%s: declared %q, bound %q", field, declared, bound))
		}
	}
	checkOptional := func(field, declared, bound string) {
		declared = strings.TrimSpace(declared)
		if declared != "" && declared != strings.TrimSpace(bound) {
			mismatches = append(mismatches, fmt.Sprintf("%s: declared %q, bound %q", field, declared, bound))
		}
	}
	checkExact("product_id", id.ProductID, entry.ProductID)
	checkExact("name", id.Name, entry.ToolName)
	checkExact("canonical_path", id.CanonicalPath, canonicalPath)
	cliPath := strings.TrimSpace(id.CLIPath)
	primary := strings.TrimSpace(id.PrimaryCLIPath)
	if cliPath == "" {
		cliPath = primary
	}
	if primary == "" {
		primary = cliPath
	}
	checkExact("cli_path", cliPath, entry.CLIPath)
	checkExact("primary_cli_path", primary, entry.PrimaryCLIPath)
	// Empty / product-equal source_product_id are equivalent (registry decode
	// defaults omitted source to product_id; Contract may omit it).
	checkExact(
		"source_product_id",
		normalizeIdentitySourceProduct(id.SourceProductID, entry.ProductID),
		normalizeIdentitySourceProduct(entry.SourceProductID, entry.ProductID),
	)
	checkOptional("cli_name", id.CLIName, entry.CLIName)
	checkOptional("group", id.Group, entry.Group)
	checkOptional("source", id.Source, entry.Source)
	if !stringSlicesEqualAsSet(id.Aliases, entry.Aliases) {
		mismatches = append(mismatches, fmt.Sprintf("aliases: declared %v, bound %v", id.Aliases, entry.Aliases))
	}
	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		return fmt.Errorf("contract final identity mismatch for %s: %s", canonicalPath, strings.Join(mismatches, "; "))
	}
	return nil
}

func marshalSchemaRaw(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func schemaToolForResolvedPath(tool ToolSpec, raw string) ToolSpec {
	normalized := normalizeSchemaQueryCLIPath(raw)
	if normalized == "" || normalized == tool.Identity.CLIPath || normalized == tool.Identity.PrimaryCLIPath {
		return tool
	}
	for _, alias := range tool.Identity.Aliases {
		if normalizeSchemaCLIPath(alias) == normalized {
			tool.Identity.CLIPath = normalizeSchemaCLIPath(alias)
			tool.Identity.IsAlias = true
			return tool
		}
	}
	return tool
}

func schemaToolUnderGroup(tool ToolSpec, group string) bool {
	prefix := normalizeSchemaCLIPath(group) + " "
	paths := append([]string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}, tool.Identity.Aliases...)
	for _, path := range paths {
		if strings.HasPrefix(normalizeSchemaCLIPath(path), prefix) {
			return true
		}
	}
	return false
}

// validateSchemaRegistryAgainstCommandRegistry compares identities as exact
// sets, not counts. A stale/missing primary path or alias must fail generation
// even when another executable path happens to preserve the canonical count.
func validateSchemaRegistryAgainstCommandRegistry(registry SchemaRegistry, commandRegistry EffectiveCommandRegistry) error {
	index, err := registry.Index()
	if err != nil {
		return err
	}
	publicCommands := make(map[string]CommandSpec)
	for canonical, command := range commandRegistry.ByCanonical {
		if command.Visibility == SchemaVisibilityPublic {
			publicCommands[canonical] = command
		}
	}
	if got, want := len(index.CanonicalPaths()), len(publicCommands); got != want {
		return fmt.Errorf("typed Schema registry contains %d canonical tools, reviewed CommandRegistry contains %d", got, want)
	}
	canonicals := make([]string, 0, len(publicCommands))
	for canonical := range publicCommands {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	for _, canonical := range canonicals {
		expected := publicCommands[canonical]
		tool, ok := index.Resolve(canonical)
		if !ok {
			return fmt.Errorf("reviewed CommandRegistry canonical %s is missing from typed Schema registry", canonical)
		}
		if actual := normalizeSchemaCLIPath(tool.Identity.PrimaryCLIPath); actual != expected.PrimaryCLIPath {
			return fmt.Errorf("schema tool %s primary path %q disagrees with reviewed CommandRegistry %q", canonical, actual, expected.PrimaryCLIPath)
		}
		actualSourceProduct := strings.TrimSpace(tool.Identity.SourceProductID)
		if actualSourceProduct == "" {
			actualSourceProduct = tool.Identity.ProductID
		}
		expectedSourceProduct := strings.TrimSpace(expected.SourceProductID)
		if expectedSourceProduct == "" {
			expectedSourceProduct = tool.Identity.ProductID
		}
		if actualSourceProduct != expectedSourceProduct {
			return fmt.Errorf("schema tool %s source product %q disagrees with reviewed CommandRegistry %q", canonical, actualSourceProduct, expectedSourceProduct)
		}
		if actualSource := strings.TrimSpace(tool.Identity.Source); actualSource != strings.TrimSpace(expected.Source) {
			return fmt.Errorf("schema tool %s identity source %q disagrees with effective CommandRegistry %q", canonical, actualSource, expected.Source)
		}
		actualAliases := sortedUniqueStrings(tool.Identity.Aliases)
		expectedAliases := sortedUniqueStrings(expected.Aliases)
		if strings.Join(actualAliases, "\x00") != strings.Join(expectedAliases, "\x00") {
			return fmt.Errorf("schema tool %s aliases %q disagree with reviewed CommandRegistry %q", canonical, actualAliases, expectedAliases)
		}
	}
	return nil
}

// validateSchemaRegistryAgentMetadata compares exact canonical sets after
// resolving generated metadata keys through the same SchemaIndex. Counts alone
// cannot detect one missing tool being masked by one duplicate alias.
//
// When Agent metadata is not injected (shipped runtime / live assembly without
// the Catalog generator), the retired embed is empty. In that mode validate that
// every assembled tool already carries selection prose from ContractFinal
// instead of reopening schema_agent_metadata/.
func validateSchemaRegistryAgentMetadata(registry SchemaRegistry) error {
	index, err := registry.Index()
	if err != nil {
		return err
	}
	metadata := finalSchemaAgentMetadata()
	if len(metadata.Tools) == 0 {
		var problems []string
		for _, canonical := range index.CanonicalPaths() {
			// CanonicalPaths is derived from the same index Resolve reads, so a
			// miss is impossible for a consistent SchemaIndex.
			tool, _ := index.Resolve(canonical)
			if strings.TrimSpace(tool.Selection.AgentSummary) == "" {
				problems = append(problems, fmt.Sprintf("final Schema tool %s has no agent_summary without injected Agent metadata", canonical))
			}
		}
		if len(problems) > 0 {
			sort.Strings(problems)
			return fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return nil
	}
	resolved := make(map[string]string, len(metadata.Tools))
	var problems []string
	keys := make([]string, 0, len(metadata.Tools))
	for key := range metadata.Tools {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		tool, ok := index.Resolve(key)
		if !ok {
			problems = append(problems, fmt.Sprintf("Agent metadata key %q does not resolve in final Schema registry", key))
			continue
		}
		canonical := tool.Identity.CanonicalPath
		if previous := resolved[canonical]; previous != "" && previous != key {
			problems = append(problems, fmt.Sprintf("Agent metadata keys %q and %q both resolve to %s", previous, key, canonical))
			continue
		}
		resolved[canonical] = key
	}
	for _, canonical := range index.CanonicalPaths() {
		if resolved[canonical] == "" {
			problems = append(problems, fmt.Sprintf("final Schema tool %s has no generated Agent metadata", canonical))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// validateFinalSchemaProvenanceCoverage ensures every field resolved from
// competing contract sources carries a mechanically checkable winner. The
// per-field equality invariant itself is enforced by ToolSpec.Validate.
func validateFinalSchemaProvenanceCoverage(registry SchemaRegistry) error {
	var problems []string
	require := func(owner, field string, provenance map[string]contract.FieldProvenance) {
		if _, ok := provenance[field]; !ok {
			problems = append(problems, fmt.Sprintf("Schema %s has no provenance for %s", owner, field))
		}
	}
	for _, product := range registry.Products {
		if strings.TrimSpace(product.Selection.AgentSummary) != "" {
			require("product "+product.ID, "agent_summary", product.FieldProvenance)
		}
		if product.Selection.UseWhen != nil {
			require("product "+product.ID, "use_when", product.FieldProvenance)
		}
		if product.Selection.AvoidWhen != nil {
			require("product "+product.ID, "avoid_when", product.FieldProvenance)
		}
		for _, tool := range product.Tools {
			canonical := tool.Identity.CanonicalPath
			if err := tool.Validate(); err != nil {
				problems = append(problems, err.Error())
				continue
			}
			for _, field := range requiredToolProvenanceFields {
				require("tool "+canonical, field, tool.FieldProvenance)
			}
			if tool.DryRun != nil {
				require("tool "+canonical, "dry_run", tool.FieldProvenance)
			}
			// interface_reason is part of the final interface contract only when
			// the disposition requires or actually delivers a reason. An MCP or
			// local available command with no reason has no resolver winner to
			// invent; unavailable/composite commands fail closed without one.
			if strings.TrimSpace(tool.Interface.Reason) != "" ||
				strings.TrimSpace(tool.Interface.Mode) == contract.InterfaceModeComposite ||
				strings.TrimSpace(tool.Interface.Availability) == contract.InterfaceUnavailable {
				require("tool "+canonical, "interface_reason", tool.FieldProvenance)
			}
			selectionValues := map[string][]string{
				"use_when":      tool.Selection.UseWhen,
				"avoid_when":    tool.Selection.AvoidWhen,
				"prerequisites": tool.Selection.Prerequisites,
				"tips":          tool.Selection.Tips,
				"workflow_refs": tool.Selection.WorkflowRefs,
				"examples":      tool.Selection.Examples,
			}
			for _, field := range conditionalSelectionProvenanceFields {
				// A non-nil empty slice is an authored [] winner, not absence.
				if selectionValues[field] != nil {
					require("tool "+canonical, field, tool.FieldProvenance)
				}
			}
			if tool.Selection.Reviewed != nil {
				require("tool "+canonical, "reviewed", tool.FieldProvenance)
			}
			for field, value := range map[string]string{
				"title":           tool.Title,
				"description":     tool.Description,
				"metadata_source": tool.MetadataSource,
			} {
				if strings.TrimSpace(value) != "" {
					require("tool "+canonical, field, tool.FieldProvenance)
				}
			}
			for _, parameter := range tool.Parameters {
				owner := "tool " + canonical + " parameter " + parameter.Name
				for _, field := range requiredParameterProvenanceFields {
					require(owner, field, parameter.FieldProvenance)
				}
				if parameter.CLIRequired {
					require(owner, "cli_required", parameter.FieldProvenance)
				}
				if parameter.InterfaceType != "" {
					require(owner, "interface_type", parameter.FieldProvenance)
				}
				if parameter.Format != "" {
					require(owner, "format", parameter.FieldProvenance)
				}
				if len(parameter.Example) > 0 {
					require(owner, "example", parameter.FieldProvenance)
				}
				if len(parameter.Enum) > 0 {
					require(owner, "enum", parameter.FieldProvenance)
				}
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
