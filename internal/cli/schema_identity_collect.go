// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

// Command identity collection — the single identity source.
//
// CollectIdentitySpecs walks the live Cobra leaves carrying
// ContractFinal.Identity and produces the CommandSpec set consumed by
// BuildEffectiveCommandRegistry. The reviewed schema_command_registry was
// retired together with this switchover; the standing gate in internal/app
// asserts the collector is a valid, stable single source.

// IdentityCollectionReport summarises a collection walk.
type IdentityCollectionReport struct {
	Leaves          int      // runnable leaves walked (hidden included)
	WithIdentity    int      // primary leaves carrying a ContractFinal.Identity
	HiddenPrimaries int      // collected primaries that are Hidden deprecated/migration shims
	Excluded        int      // leaves matched by reviewed exclusions
	NoIdentity      []string // leaves without Identity (alias / compatibility / deprecated; informational)
	MissingPrimary  []string // canonicals with no collected primary (populated by spec-set comparisons)
}

// BuildEffectiveFromSpecs wraps the shared indexing so collected specs are
// normalised by the exact same rules used for effective registry assembly.
func BuildEffectiveFromSpecs(specs []CommandSpec) (EffectiveCommandRegistry, error) {
	return newEffectiveCommandRegistry(specs)
}

// primaryCLIPathFromIdentity resolves the primary leaf path declared by an
// identity: PrimaryCLIPath wins, CLIPath fills when primary is empty.
func primaryCLIPathFromIdentity(id contract.ToolIdentitySpec) string {
	primary := strings.TrimSpace(id.PrimaryCLIPath)
	if primary == "" {
		primary = strings.TrimSpace(id.CLIPath)
	}
	return normalizeSchemaCLIPath(primary)
}

// walkIdentityLeaves visits every runnable leaf under cmd, INCLUDING Hidden
// ones. The production bind path (bindCommandRegistryPath →
// resolveExactCobraPath) resolves identity entries through raw Commands() with
// no availability filter, so hidden deprecated shims that still carry
// ContractFinal.Identity are part of the current effective surface. The
// collector must match that reachability; filtering on IsAvailableCommand (as
// the public-surface completeness walk does) would silently drop them.
func walkIdentityLeaves(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd == nil {
		return
	}
	if cmd.Runnable() && !cmd.HasSubCommands() {
		fn(cmd)
		return
	}
	for _, child := range cmd.Commands() {
		if child.Name() == "help" {
			continue
		}
		walkIdentityLeaves(child, fn)
	}
}

// CollectIdentitySpecs walks the runnable leaves under root and synthesises a
// CommandSpec from each leaf's ContractFinal.Identity (public visibility;
// SourceProductID defaulting is handled by the shared indexer). It is the
// single identity source consumed by BuildEffectiveCommandRegistry.
func CollectIdentitySpecs(root *cobra.Command) ([]CommandSpec, IdentityCollectionReport, error) {
	report := IdentityCollectionReport{}
	if root == nil {
		return nil, report, fmt.Errorf("collect identity specs: root is nil")
	}
	exclusions, err := ReviewedRuntimeSchemaExclusions()
	if err != nil {
		return nil, report, err
	}
	excluded := make(map[string]bool, len(exclusions))
	for _, exclusion := range exclusions {
		path := normalizeSchemaCLIPath(exclusion.CLIPath)
		if path != "" {
			excluded[path] = true
		}
	}

	specs := []CommandSpec{}
	walkIdentityLeaves(root, func(leaf *cobra.Command) {
		report.Leaves++
		path := normalizeSchemaCLIPath(strings.Join(commandPathParts(leaf), " "))
		if path != "" && excluded[path] {
			report.Excluded++
			return
		}
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Identity == nil {
			// Alias / compatibility / deprecated leaves carry no Identity of
			// their own; they resolve to a primary. Informational only.
			report.NoIdentity = append(report.NoIdentity, path)
			return
		}
		report.WithIdentity++
		if leaf.Hidden {
			report.HiddenPrimaries++
		}
		id := *final.Identity
		specs = append(specs, CommandSpec{
			CanonicalPath:   strings.TrimSpace(id.CanonicalPath),
			SourceProductID: strings.TrimSpace(id.SourceProductID),
			PrimaryCLIPath:  primaryCLIPathFromIdentity(id),
			Aliases:         append([]string(nil), id.Aliases...),
			Visibility:      SchemaVisibilityPublic,
			Source:          CommandSourceContractIdentity,
		})
	})
	sort.Slice(specs, func(i, j int) bool { return specs[i].CanonicalPath < specs[j].CanonicalPath })
	sort.Strings(report.NoIdentity)
	if err := validateCollectedUniqueness(specs); err != nil {
		return nil, report, err
	}
	return specs, report, nil
}

// validateCollectedUniqueness fails closed on identity collisions among the
// collected primaries: duplicate canonical paths, duplicate primary CLI paths,
// or an alias that collides with any primary path or another alias. These are
// authoring drift the collector must surface before the specs are trusted.
func validateCollectedUniqueness(specs []CommandSpec) error {
	canonicals := make(map[string]bool, len(specs))
	primaryPaths := make(map[string]bool, len(specs))
	aliasOwners := make(map[string]string)
	for _, spec := range specs {
		canonical := strings.TrimSpace(spec.CanonicalPath)
		if canonicals[canonical] {
			return fmt.Errorf("collect identity specs: duplicate canonical path %q", canonical)
		}
		canonicals[canonical] = true
		primary := normalizeSchemaCLIPath(spec.PrimaryCLIPath)
		if primary != "" {
			if primaryPaths[primary] {
				return fmt.Errorf("collect identity specs: duplicate primary CLI path %q", primary)
			}
			primaryPaths[primary] = true
		}
		for _, rawAlias := range spec.Aliases {
			alias := normalizeSchemaCLIPath(rawAlias)
			if alias == "" {
				continue
			}
			if primaryPaths[alias] {
				return fmt.Errorf("collect identity specs: alias %q collides with a primary CLI path", alias)
			}
			if owner, taken := aliasOwners[alias]; taken {
				return fmt.Errorf("collect identity specs: alias %q declared by both %q and %q", alias, owner, canonical)
			}
			aliasOwners[alias] = canonical
		}
	}
	for alias, owner := range aliasOwners {
		if primaryPaths[alias] {
			return fmt.Errorf("collect identity specs: alias %q (from %q) collides with a primary CLI path", alias, owner)
		}
	}
	return nil
}

// DiagnoseMissingPrimaries resolves each spec that has no collected primary
// and reports why: the primary CLI path may not exist as a Cobra leaf, the
// leaf may lack ContractFinal, or its declared canonical may drift from the
// expected one.
func DiagnoseMissingPrimaries(root *cobra.Command, missing []CommandSpec) []string {
	var diagnostics []string
	for _, spec := range missing {
		path := normalizeSchemaCLIPath(spec.PrimaryCLIPath)
		match, err := resolveExactCobraPath(root, path)
		if err != nil || match.Command == nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s: primary path %q has no Cobra leaf (%v)", spec.CanonicalPath, path, err))
			continue
		}
		final, ok := contractfinal.RuntimeContractFinal(match.Command)
		if !ok || final.Identity == nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s: leaf %q has no ContractFinal.Identity", spec.CanonicalPath, path))
			continue
		}
		declared := strings.TrimSpace(final.Identity.CanonicalPath)
		diagnostics = append(diagnostics, fmt.Sprintf("%s: leaf %q declares canonical %q (reviewed %q)", spec.CanonicalPath, path, declared, spec.CanonicalPath))
	}
	sort.Strings(diagnostics)
	return diagnostics
}

// CompareCommandSpecEquivalence returns a human-readable, deterministic diff
// between two CommandSpec sets keyed by canonical path. An empty slice means
// the two sets agree on every compared field.
func CompareCommandSpecEquivalence(collected, reviewed []CommandSpec) []string {
	byCanonical := func(specs []CommandSpec) map[string]CommandSpec {
		m := make(map[string]CommandSpec, len(specs))
		for _, spec := range specs {
			m[strings.TrimSpace(spec.CanonicalPath)] = spec
		}
		return m
	}
	collectedMap := byCanonical(collected)
	reviewedMap := byCanonical(reviewed)

	var problems []string
	for canonical := range reviewedMap {
		if _, present := collectedMap[canonical]; !present {
			problems = append(problems, fmt.Sprintf("missing from collected: %s", canonical))
		}
	}
	for canonical := range collectedMap {
		if _, present := reviewedMap[canonical]; !present {
			problems = append(problems, fmt.Sprintf("extra in collected (not reviewed): %s", canonical))
		}
	}
	for canonical, got := range collectedMap {
		want, present := reviewedMap[canonical]
		if !present {
			continue
		}
		if got.PrimaryCLIPath != want.PrimaryCLIPath {
			problems = append(problems, fmt.Sprintf("%s primary_cli_path: collected %q, reviewed %q", canonical, got.PrimaryCLIPath, want.PrimaryCLIPath))
		}
		if got.SourceProductID != want.SourceProductID {
			problems = append(problems, fmt.Sprintf("%s source_product_id: collected %q, reviewed %q", canonical, got.SourceProductID, want.SourceProductID))
		}
		if got.Source != want.Source {
			problems = append(problems, fmt.Sprintf("%s source: collected %q, reviewed %q", canonical, got.Source, want.Source))
		}
		if got.Visibility != want.Visibility {
			problems = append(problems, fmt.Sprintf("%s visibility: collected %q, reviewed %q", canonical, got.Visibility, want.Visibility))
		}
		if !stringSlicesEqualAsSet(got.Aliases, want.Aliases) {
			problems = append(problems, fmt.Sprintf("%s aliases: collected %v, reviewed %v", canonical, got.Aliases, want.Aliases))
		}
	}
	sort.Strings(problems)
	return problems
}
