// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AliasKind records how a reviewed alias path is represented by Cobra. A
// Cobra alias resolves to the primary command pointer; a compatibility leaf is
// a separately registered (usually hidden) runnable command. Keeping the two
// forms explicit prevents downstream code from assuming that every alias has
// its own Cobra leaf.
type AliasKind string

const (
	AliasKindCobraAlias                       AliasKind = "cobra_alias"
	AliasKindCompatibilityLeaf                AliasKind = "compatibility_leaf"
	runtimeCompatibilityEquivalenceAnnotation           = "dws.schema.compatibility_equivalence"
)

// RuntimeCompatibilityEquivalence is an implementation-side review record
// for two separately registered Cobra leaves that intentionally share one
// Schema identity despite using different execution handlers. Registry alias
// review alone is not enough: different handlers may inject fixed arguments or
// route to different business operations while exposing identical flags.
type RuntimeCompatibilityEquivalence struct {
	ID       string `json:"id"`
	Reason   string `json:"reason"`
	Reviewed bool   `json:"reviewed"`
}

var (
	bindValidateParameterBindings  = ValidateEmbeddedSchemaParameterBindings
	loadCompatibilityFlagContracts = effectiveCompatibilityFlagContracts
	compatibilityParameterBindings = runtimeSchemaParameterBindingData
)

// AnnotateRuntimeCompatibilityEquivalence records the same typed review on
// both sides of one independently executable compatibility pair. Invalid
// review data panics during command construction so production cannot silently
// accept an unreviewed handler mismatch.
func AnnotateRuntimeCompatibilityEquivalence(primary, compatibility *cobra.Command, review RuntimeCompatibilityEquivalence) {
	if primary == nil || compatibility == nil {
		panic("runtime compatibility equivalence requires two commands")
	}
	review.ID = strings.TrimSpace(review.ID)
	review.Reason = strings.TrimSpace(review.Reason)
	if review.ID == "" || review.Reason == "" || !review.Reviewed {
		panic("runtime compatibility equivalence requires a reviewed id and reason")
	}
	encoded, _ := json.Marshal(review)
	commands := []*cobra.Command{primary, compatibility}
	for _, command := range commands {
		if existing, exists := command.Annotations[runtimeCompatibilityEquivalenceAnnotation]; exists && existing != string(encoded) {
			panic(fmt.Sprintf("runtime compatibility equivalence for %q conflicts with an existing reviewed marker", command.CommandPath()))
		}
	}
	for _, command := range commands {
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		command.Annotations[runtimeCompatibilityEquivalenceAnnotation] = string(encoded)
	}
}

// BoundAlias is one reviewed alias path resolved against the live Cobra tree.
type BoundAlias struct {
	Path         string
	Command      *cobra.Command
	Kind         AliasKind
	Source       string
	ReviewReason string
}

// exactCobraPathMatch is the result of resolving one reviewed CLI path. The
// resolver records whether any path segment used Cobra's Aliases metadata so
// callers can distinguish a same-command Cobra alias from a separately
// registered compatibility leaf.
type exactCobraPathMatch struct {
	Command   *cobra.Command
	UsedAlias bool
}

// BoundCommandSpec joins one stable registry identity to its executable
// primary leaf and all executable alias paths.
type BoundCommandSpec struct {
	CommandSpec
	PrimaryCommand *cobra.Command
	AliasCommands  []BoundAlias
}

// BoundCommandRegistry is the only command input the Schema assembler should
// consume. Every entry has passed exact path, executable-leaf, collision, and
// native-annotation consistency checks.
type BoundCommandRegistry struct {
	Commands    []BoundCommandSpec
	ByCanonical map[string]BoundCommandSpec
	ByCLIPath   map[string]BoundCommandSpec
}

// BindEffectiveCommandRegistry resolves every registry path against the live
// Cobra tree. Registry entries may target hidden compatibility leaves when
// they are explicitly reviewed, but every target must be an actual runnable
// leaf. Native annotations are optional implementation evidence; when present
// they must exactly agree with the registry identity.
func BindEffectiveCommandRegistry(root *cobra.Command, effective EffectiveCommandRegistry) (BoundCommandRegistry, error) {
	if root == nil {
		return BoundCommandRegistry{}, fmt.Errorf("bind effective Schema command registry: root is nil")
	}
	if err := bindValidateParameterBindings(); err != nil {
		return BoundCommandRegistry{}, fmt.Errorf("validate reviewed Schema parameter bindings: %w", err)
	}
	validated, err := newEffectiveCommandRegistry(effective.Commands)
	if err != nil {
		return BoundCommandRegistry{}, fmt.Errorf("bind effective Schema command registry: %w", err)
	}

	bound := BoundCommandRegistry{
		Commands:    make([]BoundCommandSpec, 0, len(validated.Commands)),
		ByCanonical: make(map[string]BoundCommandSpec, len(validated.Commands)),
		ByCLIPath:   make(map[string]BoundCommandSpec, len(validated.ByCLIPath)),
	}
	for _, spec := range validated.Commands {
		primary, err := bindCommandRegistryPath(root, spec, spec.PrimaryCLIPath)
		if err != nil {
			return BoundCommandRegistry{}, err
		}
		item := BoundCommandSpec{
			CommandSpec:    cloneCommandSpec(spec),
			PrimaryCommand: primary,
			AliasCommands:  make([]BoundAlias, 0, len(spec.Aliases)),
		}
		for _, alias := range spec.Aliases {
			boundAlias, err := bindCommandRegistryAlias(root, spec, primary, alias)
			if err != nil {
				return BoundCommandRegistry{}, err
			}
			item.AliasCommands = append(item.AliasCommands, boundAlias)
		}
		bound.Commands = append(bound.Commands, item)
	}
	sort.Slice(bound.Commands, func(i, j int) bool {
		return bound.Commands[i].CanonicalPath < bound.Commands[j].CanonicalPath
	})
	for _, item := range bound.Commands {
		bound.ByCanonical[item.CanonicalPath] = item
		bound.ByCLIPath[item.PrimaryCLIPath] = item
		for _, alias := range item.Aliases {
			bound.ByCLIPath[alias] = item
		}
	}
	return bound, nil
}

func bindCommandRegistryAlias(root *cobra.Command, spec CommandSpec, primary *cobra.Command, rawPath string) (BoundAlias, error) {
	path := normalizeSchemaCLIPath(rawPath)
	match, err := resolveExactCobraPath(root, path)
	if err != nil {
		return BoundAlias{}, fmt.Errorf("schema command registry %s alias %q: %w", spec.CanonicalPath, path, err)
	}
	if match.Command == nil {
		return BoundAlias{}, fmt.Errorf("schema command registry %s has stale alias path %q: command does not exist", spec.CanonicalPath, path)
	}
	if !runnableSchemaLeaf(match.Command) {
		return BoundAlias{}, fmt.Errorf("schema command registry %s alias %q is not a runnable Cobra leaf", spec.CanonicalPath, path)
	}

	// A Cobra alias is the same runnable command reached through one or more
	// Aliases segments. A compatibility leaf is a separately registered exact
	// Name path. A Cobra alias that reaches another command pointer is neither
	// representation and fails closed.
	kind := AliasKindCompatibilityLeaf
	if match.UsedAlias {
		if match.Command != primary {
			return BoundAlias{}, fmt.Errorf("schema command registry %s Cobra alias %q resolves to a different command than primary path %q", spec.CanonicalPath, path, spec.PrimaryCLIPath)
		}
		kind = AliasKindCobraAlias
	} else if match.Command == primary {
		return BoundAlias{}, fmt.Errorf("schema command registry %s alias %q duplicates primary path %q", spec.CanonicalPath, path, spec.PrimaryCLIPath)
	}
	if err := validateCommandRegistryAnnotation(match.Command, path, spec); err != nil {
		return BoundAlias{}, err
	}
	if kind == AliasKindCompatibilityLeaf {
		if err := validateCompatibilityLeafContract(spec, primary, match.Command, path); err != nil {
			return BoundAlias{}, err
		}
	}
	return BoundAlias{
		Path:         path,
		Command:      match.Command,
		Kind:         kind,
		Source:       spec.Source,
		ReviewReason: spec.ReviewReason,
	}, nil
}

// compatibilityFlagContract is the executable and Schema-relevant portion of
// one effective Cobra flag. A separately registered compatibility leaf is a
// reviewed alias only when it accepts the same effective flag surface as its
// primary command. Presentation on the command itself may differ (for example,
// a deprecation notice), but flag parsing and parameter facts may not.
type compatibilityFlagContract struct {
	Type                string
	Default             string
	NoOptDefault        string
	Shorthand           string
	Hidden              bool
	Deprecated          string
	ShorthandDeprecated string
	Origin              string
	Annotations         map[string][]string
	Required            compatibilityFlagRequiredContract
}

type compatibilityFlagRequiredContract struct {
	CobraRequired        bool
	UsageRequired        bool
	UsageDefault         bool
	NativeRequired       string
	MetadataRequired     string
	NativeRequiredWhen   string
	MetadataRequiredWhen string
}

// validateCompatibilityLeafContract proves that an independently executable
// compatibility leaf has the same invocation contract as the reviewed primary
// leaf. Unlike a Cobra alias, it is a different command pointer and can drift
// even while both paths still resolve to the same canonical identity.
func validateCompatibilityLeafContract(spec CommandSpec, primary, alias *cobra.Command, aliasPath string) error {
	problems := make([]string, 0)
	problems = append(problems, compatibilityHandlerContractProblems(primary, alias)...)
	flagProblems, err := compatibilityFlagContractProblems(spec.CanonicalPath, primary, alias)
	if err != nil {
		return fmt.Errorf("validate reviewed Schema parameter bindings for compatibility leaf %q: %w", aliasPath, err)
	}
	problems = append(problems, flagProblems...)
	problems = append(problems, compatibilityArgsContractProblems(primary, alias)...)

	primaryConstraints, err := strictCompatibilityConstraints(primary, spec.CanonicalPath)
	if err != nil {
		problems = append(problems, "primary runtime constraints are invalid: "+err.Error())
	}
	aliasConstraints, aliasErr := strictCompatibilityConstraints(alias, spec.CanonicalPath)
	if aliasErr != nil {
		problems = append(problems, "compatibility runtime constraints are invalid: "+aliasErr.Error())
	}
	if err == nil && aliasErr == nil && !reflect.DeepEqual(primaryConstraints, aliasConstraints) {
		problems = append(problems, fmt.Sprintf("runtime constraints differ: primary=%s compatibility=%s",
			compatibilityJSON(primaryConstraints), compatibilityJSON(aliasConstraints)))
	}

	primaryPositionals, err := strictCompatibilityPositionals(primary)
	if err != nil {
		problems = append(problems, "primary runtime positionals are invalid: "+err.Error())
	}
	aliasPositionals, aliasErr := strictCompatibilityPositionals(alias)
	if aliasErr != nil {
		problems = append(problems, "compatibility runtime positionals are invalid: "+aliasErr.Error())
	}
	if err == nil && aliasErr == nil && !reflect.DeepEqual(primaryPositionals, aliasPositionals) {
		problems = append(problems, fmt.Sprintf("runtime positionals differ: primary=%s compatibility=%s",
			compatibilityJSON(primaryPositionals), compatibilityJSON(aliasPositionals)))
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf(
		"schema command registry %s compatibility leaf alias %q is not executable-contract equivalent to primary path %q:\n - %s",
		spec.CanonicalPath,
		aliasPath,
		spec.PrimaryCLIPath,
		strings.Join(problems, "\n - "),
	)
}

func compatibilityHandlerContractProblems(primary, alias *cobra.Command) []string {
	if primary == nil || alias == nil {
		return []string{"execution handlers cannot be compared for a nil command"}
	}

	differences := make([]string, 0)
	for _, handler := range []struct {
		name    string
		primary any
		alias   any
	}{
		{name: "PersistentPreRun", primary: primary.PersistentPreRun, alias: alias.PersistentPreRun},
		{name: "PersistentPreRunE", primary: primary.PersistentPreRunE, alias: alias.PersistentPreRunE},
		{name: "PreRun", primary: primary.PreRun, alias: alias.PreRun},
		{name: "PreRunE", primary: primary.PreRunE, alias: alias.PreRunE},
		{name: "Run", primary: primary.Run, alias: alias.Run},
		{name: "RunE", primary: primary.RunE, alias: alias.RunE},
		{name: "PostRun", primary: primary.PostRun, alias: alias.PostRun},
		{name: "PostRunE", primary: primary.PostRunE, alias: alias.PostRunE},
		{name: "PersistentPostRun", primary: primary.PersistentPostRun, alias: alias.PersistentPostRun},
		{name: "PersistentPostRunE", primary: primary.PersistentPostRunE, alias: alias.PersistentPostRunE},
	} {
		if compatibilityHandlerPointer(handler.primary) != compatibilityHandlerPointer(handler.alias) {
			differences = append(differences, handler.name)
		}
	}

	primaryReview, primaryPresent, primaryErr := runtimeCompatibilityEquivalence(primary)
	aliasReview, aliasPresent, aliasErr := runtimeCompatibilityEquivalence(alias)
	problems := make([]string, 0)
	if primaryErr != nil {
		problems = append(problems, "primary compatibility equivalence is invalid: "+primaryErr.Error())
	}
	if aliasErr != nil {
		problems = append(problems, "compatibility equivalence is invalid: "+aliasErr.Error())
	}
	reviewMatches := primaryPresent && aliasPresent && primaryErr == nil && aliasErr == nil && reflect.DeepEqual(primaryReview, aliasReview)
	if !primaryPresent && !aliasPresent {
		problems = append(problems, "independent compatibility leaves require the same reviewed typed compatibility equivalence on both commands")
	} else if primaryPresent != aliasPresent {
		problems = append(problems, "reviewed compatibility equivalence must be present on both commands")
	} else if primaryPresent && aliasPresent && primaryErr == nil && aliasErr == nil && !reflect.DeepEqual(primaryReview, aliasReview) {
		problems = append(problems, fmt.Sprintf("reviewed compatibility equivalence differs: primary=%s compatibility=%s",
			compatibilityJSON(primaryReview), compatibilityJSON(aliasReview)))
	}
	if len(differences) > 0 && !reviewMatches {
		problems = append(problems, fmt.Sprintf("execution handler implementation differs for %s; add the same reviewed typed compatibility equivalence to both commands or model them as distinct canonical tools", strings.Join(differences, ", ")))
	}
	return problems
}

func compatibilityHandlerPointer(handler any) uintptr {
	if handler == nil {
		return 0
	}
	value := reflect.ValueOf(handler)
	if value.Kind() != reflect.Func || value.IsNil() {
		return 0
	}
	return value.Pointer()
}

func runtimeCompatibilityEquivalence(command *cobra.Command) (RuntimeCompatibilityEquivalence, bool, error) {
	if command == nil || command.Annotations == nil {
		return RuntimeCompatibilityEquivalence{}, false, nil
	}
	raw, present := command.Annotations[runtimeCompatibilityEquivalenceAnnotation]
	if !present {
		return RuntimeCompatibilityEquivalence{}, false, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RuntimeCompatibilityEquivalence{}, true, fmt.Errorf("annotation is empty")
	}
	var review RuntimeCompatibilityEquivalence
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&review); err != nil {
		return RuntimeCompatibilityEquivalence{}, true, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return RuntimeCompatibilityEquivalence{}, true, err
	}
	if review.ID == "" || review.ID != strings.TrimSpace(review.ID) || review.Reason == "" || review.Reason != strings.TrimSpace(review.Reason) || !review.Reviewed {
		return RuntimeCompatibilityEquivalence{}, true, fmt.Errorf("annotation requires an exact reviewed id and reason")
	}
	return review, true, nil
}

func compatibilityFlagContractProblems(canonicalPath string, primary, alias *cobra.Command) ([]string, error) {
	primaryFlags, err := loadCompatibilityFlagContracts(primary, canonicalPath)
	if err != nil {
		return nil, err
	}
	aliasFlags, err := loadCompatibilityFlagContracts(alias, canonicalPath)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(primaryFlags)+len(aliasFlags))
	seen := map[string]bool{}
	for name := range primaryFlags {
		seen[name] = true
		names = append(names, name)
	}
	for name := range aliasFlags {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	problems := make([]string, 0)
	for _, name := range names {
		primaryFlag, primaryOK := primaryFlags[name]
		aliasFlag, aliasOK := aliasFlags[name]
		switch {
		case !primaryOK:
			problems = append(problems, fmt.Sprintf("flag --%s exists only on compatibility leaf", name))
			continue
		case !aliasOK:
			problems = append(problems, fmt.Sprintf("flag --%s is missing from compatibility leaf", name))
			continue
		}
		if primaryFlag.Type != aliasFlag.Type {
			problems = append(problems, fmt.Sprintf("flag --%s type differs: primary=%q compatibility=%q", name, primaryFlag.Type, aliasFlag.Type))
		}
		if primaryFlag.Default != aliasFlag.Default {
			problems = append(problems, fmt.Sprintf("flag --%s default differs: primary=%q compatibility=%q", name, primaryFlag.Default, aliasFlag.Default))
		}
		if primaryFlag.NoOptDefault != aliasFlag.NoOptDefault {
			problems = append(problems, fmt.Sprintf("flag --%s no-option default differs: primary=%q compatibility=%q", name, primaryFlag.NoOptDefault, aliasFlag.NoOptDefault))
		}
		if primaryFlag.Shorthand != aliasFlag.Shorthand {
			problems = append(problems, fmt.Sprintf("flag --%s shorthand differs: primary=%q compatibility=%q", name, primaryFlag.Shorthand, aliasFlag.Shorthand))
		}
		if primaryFlag.Hidden != aliasFlag.Hidden {
			problems = append(problems, fmt.Sprintf("flag --%s hidden state differs: primary=%t compatibility=%t", name, primaryFlag.Hidden, aliasFlag.Hidden))
		}
		if primaryFlag.Deprecated != aliasFlag.Deprecated {
			problems = append(problems, fmt.Sprintf("flag --%s deprecation behavior differs: primary=%q compatibility=%q", name, primaryFlag.Deprecated, aliasFlag.Deprecated))
		}
		if primaryFlag.ShorthandDeprecated != aliasFlag.ShorthandDeprecated {
			problems = append(problems, fmt.Sprintf("flag --%s shorthand deprecation differs: primary=%q compatibility=%q", name, primaryFlag.ShorthandDeprecated, aliasFlag.ShorthandDeprecated))
		}
		if primaryFlag.Origin != aliasFlag.Origin {
			problems = append(problems, fmt.Sprintf("flag --%s local/persistent/inherited behavior differs: primary=%s compatibility=%s", name, primaryFlag.Origin, aliasFlag.Origin))
		}
		if !reflect.DeepEqual(primaryFlag.Required, aliasFlag.Required) {
			problems = append(problems, fmt.Sprintf("flag --%s required/required_when facts differ: primary=%s compatibility=%s",
				name, compatibilityJSON(primaryFlag.Required), compatibilityJSON(aliasFlag.Required)))
		}
		if !reflect.DeepEqual(primaryFlag.Annotations, aliasFlag.Annotations) {
			problems = append(problems, fmt.Sprintf("flag --%s annotations differ: primary=%s compatibility=%s",
				name, compatibilityJSON(primaryFlag.Annotations), compatibilityJSON(aliasFlag.Annotations)))
		}
	}
	return problems, nil
}

func effectiveCompatibilityFlagContracts(command *cobra.Command, canonicalPath string) (map[string]compatibilityFlagContract, error) {
	contracts := map[string]compatibilityFlagContract{}
	if command == nil {
		return contracts, nil
	}
	canonicalPath = strings.TrimSpace(canonicalPath)
	snapshot, err := compatibilityParameterBindings()
	if err != nil {
		return nil, err
	}
	bindings := snapshot.Bindings[canonicalPath]
	metadata := runtimeSchemaParameterMetadataByCanonical[canonicalPath]
	// Ask Cobra to materialize inherited flags, then visit every contributing
	// set explicitly. In particular, a newly-added persistent flag on a leaf is
	// not guaranteed to have been merged into Flags() yet.
	command.InheritedFlags()
	visit := func(flag *pflag.Flag) {
		// Cobra materializes --help lazily on only the command being executed.
		// It is framework scaffolding, not part of a leaf's executable/Schema
		// parameter contract, and must not make compatibility equivalence depend
		// on which alias happened to run before binding.
		if flag == nil || flag.Name == "help" {
			return
		}
		flagType := ""
		if flag.Value != nil {
			flagType = flag.Value.Type()
		}
		annotations := effectiveCompatibilityFlagAnnotations(flag, bindings, metadata)
		contracts[flag.Name] = compatibilityFlagContract{
			Type:                flagType,
			Default:             flag.DefValue,
			NoOptDefault:        flag.NoOptDefVal,
			Shorthand:           flag.Shorthand,
			Hidden:              flag.Hidden,
			Deprecated:          flag.Deprecated,
			ShorthandDeprecated: flag.ShorthandDeprecated,
			Origin:              compatibilityFlagOrigin(command, flag.Name),
			Annotations:         annotations,
			Required: compatibilityFlagRequiredContract{
				CobraRequired:        runtimeFlagCobraHardRequired(flag),
				UsageRequired:        usageImpliesRequired(flag.Usage),
				UsageDefault:         usageImpliesDefault(flag.Usage),
				NativeRequired:       firstCompatibilityAnnotation(annotations, runtimeSchemaFlagRequiredAnnotation),
				MetadataRequired:     firstCompatibilityAnnotation(annotations, runtimeSchemaFlagMetadataRequiredAnnotation),
				NativeRequiredWhen:   firstCompatibilityAnnotation(annotations, runtimeSchemaFlagRequiredWhenAnnotation),
				MetadataRequiredWhen: firstCompatibilityAnnotation(annotations, runtimeSchemaFlagMetadataRequiredWhenAnnotation),
			},
		}
	}
	command.Flags().VisitAll(visit)
	command.PersistentFlags().VisitAll(visit)
	command.InheritedFlags().VisitAll(visit)
	return contracts, nil
}

// effectiveCompatibilityFlagAnnotations overlays canonical, code-owned
// parameter facts without mutating either command. Schema collection applies
// the same facts to the primary leaf later; modelling them here keeps repeated
// binding order-independent while still exposing path-specific drift.
func effectiveCompatibilityFlagAnnotations(flag *pflag.Flag, bindings map[string]string, metadata RuntimeSchemaParameterMetadata) map[string][]string {
	annotations := cloneCompatibilityFlagAnnotations(flag.Annotations)
	// Reviewed Manual Schema hints are canonical ToolSpec projection inputs,
	// not executable facts owned by each Cobra path. They are intentionally
	// attached to and resolved from the primary command once; a compatibility
	// leaf remains a navigation view of that same ToolSpec. Keep every other
	// native/typed annotation in the equivalence check so real command drift
	// still fails closed.
	delete(annotations, runtimeSchemaManualParameterAnnotation)
	delete(annotations, runtimeSchemaManualReasonAnnotation)
	if len(annotations) == 0 {
		annotations = nil
	}
	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			if annotations == nil {
				annotations = map[string][]string{}
			}
			annotations[key] = []string{value}
		}
	}
	set(runtimeSchemaFlagBindingPropertyAnnotation, bindings[flag.Name])
	for _, name := range metadata.Required {
		if strings.TrimSpace(name) == flag.Name {
			set(runtimeSchemaFlagMetadataRequiredAnnotation, "true")
			break
		}
	}
	set(runtimeSchemaFlagMetadataRequiredWhenAnnotation, metadata.RequiredWhen[flag.Name])
	set(runtimeSchemaFlagMetadataFormatAnnotation, metadata.Formats[flag.Name])
	set(runtimeSchemaFlagMetadataExampleAnnotation, metadata.Examples[flag.Name])
	if values := metadata.Enums[flag.Name]; len(values) > 0 {
		if annotations == nil {
			annotations = map[string][]string{}
		}
		annotations[runtimeSchemaFlagMetadataEnumAnnotation] = append([]string(nil), values...)
	}
	return annotations
}

func firstCompatibilityAnnotation(annotations map[string][]string, key string) string {
	for _, value := range annotations[key] {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func compatibilityFlagOrigin(command *cobra.Command, name string) string {
	if command.PersistentFlags().Lookup(name) != nil {
		return "local_persistent"
	}
	if command.LocalNonPersistentFlags().Lookup(name) != nil {
		return "local"
	}
	if command.InheritedFlags().Lookup(name) != nil {
		return "inherited_persistent"
	}
	return "effective_unknown"
}

func cloneCompatibilityFlagAnnotations(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(source))
	for key, values := range source {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func compatibilityArgsContractProblems(primary, alias *cobra.Command) []string {
	problems := make([]string, 0)
	if primary == nil || alias == nil {
		return append(problems, "Args contract cannot be compared for a nil command")
	}
	if primary.DisableFlagParsing != alias.DisableFlagParsing {
		problems = append(problems, fmt.Sprintf("DisableFlagParsing differs: primary=%t compatibility=%t", primary.DisableFlagParsing, alias.DisableFlagParsing))
	}
	if primary.TraverseChildren != alias.TraverseChildren {
		problems = append(problems, fmt.Sprintf("TraverseChildren differs: primary=%t compatibility=%t", primary.TraverseChildren, alias.TraverseChildren))
	}
	if !reflect.DeepEqual(primary.FParseErrWhitelist, alias.FParseErrWhitelist) {
		problems = append(problems, fmt.Sprintf("flag parse error allowlist differs: primary=%+v compatibility=%+v", primary.FParseErrWhitelist, alias.FParseErrWhitelist))
	}
	primarySyntax := compatibilityUseArgsSyntax(primary)
	aliasSyntax := compatibilityUseArgsSyntax(alias)
	if primarySyntax != aliasSyntax {
		problems = append(problems, fmt.Sprintf("positional Use contract differs: primary=%q compatibility=%q", primarySyntax, aliasSyntax))
	}
	if !reflect.DeepEqual(primary.ValidArgs, alias.ValidArgs) {
		problems = append(problems, fmt.Sprintf("ValidArgs differ: primary=%q compatibility=%q", primary.ValidArgs, alias.ValidArgs))
	}
	if !reflect.DeepEqual(primary.ArgAliases, alias.ArgAliases) {
		problems = append(problems, fmt.Sprintf("ArgAliases differ: primary=%q compatibility=%q", primary.ArgAliases, alias.ArgAliases))
	}
	if (primary.Args == nil) != (alias.Args == nil) {
		problems = append(problems, fmt.Sprintf("Args validator presence differs: primary=%t compatibility=%t", primary.Args != nil, alias.Args != nil))
		return problems
	}
	if primary.Args == nil {
		return problems
	}
	if reflect.ValueOf(primary.Args).Pointer() != reflect.ValueOf(alias.Args).Pointer() {
		problems = append(problems, "Args validator implementation differs; reuse the primary validator for a reviewed compatibility leaf")
		return problems
	}
	for _, probe := range compatibilityArgsProbes(primary, alias) {
		primaryResult := runCompatibilityArgsValidator(primary, probe)
		aliasResult := runCompatibilityArgsValidator(alias, probe)
		if primaryResult != aliasResult {
			problems = append(problems, fmt.Sprintf("Args behavior differs for %d positional argument(s): primary=%s compatibility=%s", len(probe), primaryResult, aliasResult))
			break
		}
	}
	return problems
}

func compatibilityUseArgsSyntax(command *cobra.Command) string {
	parts := strings.Fields(command.Use)
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], " ")
}

func compatibilityArgsProbes(primary, alias *cobra.Command) [][]string {
	// Count validators are by far the most common Cobra Args contract. Probe a
	// deliberately broad range so separately-created Exact/Range/Maximum
	// closures with the same code pointer cannot hide different captured bounds.
	probes := make([][]string, 0, 260)
	for count := 0; count <= 256; count++ {
		args := make([]string, count)
		for index := range args {
			args[index] = fmt.Sprintf("arg-%d", index)
		}
		probes = append(probes, args)
	}
	for _, command := range []*cobra.Command{primary, alias} {
		for _, value := range command.ValidArgs {
			probes = append(probes, []string{string(value)})
		}
		for _, value := range command.ArgAliases {
			probes = append(probes, []string{value})
		}
	}
	probes = append(probes, []string{""}, []string{"unknown-value"})
	return probes
}

func runCompatibilityArgsValidator(command *cobra.Command, args []string) (result string) {
	result = "accepted"
	defer func() {
		if recovered := recover(); recovered != nil {
			result = fmt.Sprintf("panic(%v)", recovered)
		}
	}()
	if err := command.Args(command, append([]string(nil), args...)); err != nil {
		return "rejected"
	}
	return result
}

func strictCompatibilityConstraints(command *cobra.Command, canonicalPath string) (RuntimeSchemaConstraints, error) {
	var constraints RuntimeSchemaConstraints
	if command != nil && command.Annotations != nil {
		raw := strings.TrimSpace(command.Annotations[runtimeSchemaRulesAnnotation])
		if raw != "" {
			if err := decodeStrictSchemaJSON([]byte(raw), &constraints); err != nil {
				return RuntimeSchemaConstraints{}, err
			}
		}
	}
	// RegisterRuntimeSchemaConstraints is canonical code-owned input. Collection
	// attaches it to the primary leaf, so compare the same effective union on
	// both executable paths without changing the live Cobra tree here.
	registered := runtimeSchemaConstraintsByCanonical[strings.TrimSpace(canonicalPath)]
	constraints.MutuallyExclusive = append(constraints.MutuallyExclusive, registered.MutuallyExclusive...)
	constraints.RequireOneOf = append(constraints.RequireOneOf, registered.RequireOneOf...)
	constraints.RequireTogether = append(constraints.RequireTogether, registered.RequireTogether...)
	constraints = normalizeRuntimeSchemaConstraints(constraints)
	constraints.MutuallyExclusive = canonicalCompatibilityGroups(constraints.MutuallyExclusive)
	constraints.RequireOneOf = canonicalCompatibilityGroups(constraints.RequireOneOf)
	constraints.RequireTogether = canonicalCompatibilityGroups(constraints.RequireTogether)
	return constraints, nil
}

func canonicalCompatibilityGroups(groups [][]string) [][]string {
	canonical := make([][]string, 0, len(groups))
	for _, group := range groups {
		group = append([]string(nil), group...)
		sort.Strings(group)
		canonical = append(canonical, group)
	}
	sort.Slice(canonical, func(i, j int) bool {
		return strings.Join(canonical[i], "\x00") < strings.Join(canonical[j], "\x00")
	})
	return canonical
}

func strictCompatibilityPositionals(command *cobra.Command) ([]RuntimeSchemaPositional, error) {
	if command == nil || command.Annotations == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(command.Annotations[runtimeSchemaArgsAnnotation])
	if raw == "" {
		return nil, nil
	}
	var positionals []RuntimeSchemaPositional
	if err := decodeStrictSchemaJSON([]byte(raw), &positionals); err != nil {
		return nil, err
	}
	sort.Slice(positionals, func(i, j int) bool {
		if positionals[i].Index != positionals[j].Index {
			return positionals[i].Index < positionals[j].Index
		}
		return positionals[i].Name < positionals[j].Name
	})
	return positionals, nil
}

func compatibilityJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%+v", value)
	}
	return string(encoded)
}

// resolveExactCobraPath resolves only exact command names and exact Cobra
// aliases. At every level a real command Name takes precedence over Aliases;
// multiple matching Names or Aliases fail closed. It intentionally does not
// use cobra.Command.Find because Find may apply prefix matching and
// suggestions, neither of which is a stable registry binding contract.
func resolveExactCobraPath(root *cobra.Command, rawPath string) (exactCobraPathMatch, error) {
	if root == nil {
		return exactCobraPathMatch{}, nil
	}
	parts := strings.Fields(strings.TrimSpace(rawPath))
	if len(parts) > 0 && parts[0] == root.Name() {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return exactCobraPathMatch{}, nil
	}

	current := root
	usedAlias := false
	for _, part := range parts {
		var exactMatches []*cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == part {
				exactMatches = appendDistinctCobraCommand(exactMatches, child)
			}
		}
		if len(exactMatches) > 1 {
			return exactCobraPathMatch{}, fmt.Errorf("cobra command segment %q is ambiguous", part)
		}
		if len(exactMatches) == 1 {
			current = exactMatches[0]
			continue
		}

		var aliasMatches []*cobra.Command
		for _, child := range current.Commands() {
			for _, alias := range child.Aliases {
				if alias != part {
					continue
				}
				aliasMatches = appendDistinctCobraCommand(aliasMatches, child)
			}
		}
		if len(aliasMatches) > 1 {
			return exactCobraPathMatch{}, fmt.Errorf("cobra alias segment %q is ambiguous", part)
		}
		if len(aliasMatches) == 0 {
			return exactCobraPathMatch{}, nil
		}
		usedAlias = true
		current = aliasMatches[0]
	}
	return exactCobraPathMatch{Command: current, UsedAlias: usedAlias}, nil
}

func appendDistinctCobraCommand(commands []*cobra.Command, command *cobra.Command) []*cobra.Command {
	for _, existing := range commands {
		if existing == command {
			return commands
		}
	}
	return append(commands, command)
}

func bindCommandRegistryPath(root *cobra.Command, spec CommandSpec, path string) (*cobra.Command, error) {
	path = normalizeSchemaCLIPath(path)
	match, err := resolveExactCobraPath(root, path)
	if err != nil {
		return nil, fmt.Errorf("schema command registry %s primary path %q: %w", spec.CanonicalPath, path, err)
	}
	if match.Command == nil {
		return nil, fmt.Errorf("schema command registry %s has stale cli path %q: command does not exist", spec.CanonicalPath, path)
	}
	if match.UsedAlias {
		return nil, fmt.Errorf("schema command registry %s primary path %q must use real Cobra command names, not Aliases", spec.CanonicalPath, path)
	}
	if !runnableSchemaLeaf(match.Command) {
		return nil, fmt.Errorf("schema command registry %s path %q is not a runnable Cobra leaf", spec.CanonicalPath, path)
	}
	if err := validateCommandRegistryAnnotation(match.Command, path, spec); err != nil {
		return nil, err
	}
	return match.Command, nil
}

func runnableSchemaLeaf(command *cobra.Command) bool {
	return command != nil && command.Runnable() && !command.HasSubCommands()
}

func validateCommandRegistryAnnotation(command *cobra.Command, path string, spec CommandSpec) error {
	nativeProduct, nativeTool, _ := runtimeSchemaAnnotations(command)
	if nativeProduct != "" || nativeTool != "" {
		if nativeProduct == "" || nativeTool == "" {
			return fmt.Errorf("schema command registry %s path %q has incomplete native annotation %q.%q", spec.CanonicalPath, path, nativeProduct, nativeTool)
		}
		nativeCanonical := nativeProduct + "." + nativeTool
		if nativeCanonical != spec.CanonicalPath {
			return fmt.Errorf("schema command registry %s path %q conflicts with native annotation %s", spec.CanonicalPath, path, nativeCanonical)
		}
	}
	if manualProduct, manualTool, _, ok := runtimeManualSchemaIdentity(command); ok {
		manualCanonical := strings.TrimSpace(manualProduct + "." + manualTool)
		if manualCanonical != spec.CanonicalPath {
			return fmt.Errorf("schema command registry %s path %q conflicts with reviewed manual identity %s", spec.CanonicalPath, path, manualCanonical)
		}
	}
	return nil
}
