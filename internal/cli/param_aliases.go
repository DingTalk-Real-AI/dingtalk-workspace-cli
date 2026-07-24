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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:generate go run ../generator/cmd_param_aliases -root ../.. -output param_aliases_generated.go

// ParamAliasEntry is the reduced parameter-alias table for one runnable Cobra
// leaf. It is the typed value the build-time generator serializes into
// param_aliases_generated.go and that the runtime normalizer (P2) consumes.
//
// Aliases maps an already-morphed emitted name to the command's canonical real
// flag; the runtime looks up Morph(emitted) here to resolve a synonym. Blocked
// lists morphed names that must never be reduced (they route to did-you-mean),
// and Ambiguous lists morphed names that a reviewed co-occurrence guard leaves
// unresolved on purpose.
type ParamAliasEntry struct {
	CLIPath   string            `json:"cli_path"`
	Aliases   map[string]string `json:"aliases,omitempty"`
	Blocked   []string          `json:"blocked,omitempty"`
	Ambiguous []string          `json:"ambiguous,omitempty"`
}

// ReduceParamAliases resolves the reviewed concept dictionary against every
// runnable leaf's real flags and returns the per-command alias table. It is the
// single source of the reduction algorithm, shared by the generator and tests
// so the build-time (intersection) and generated views can never disagree.
//
// The reduction is deliberately mechanical (no NLU): for each concept it morphs
// the concept members (plus any command-bound generic flag) and intersects them
// with the command's morphed real flags. An intersection of exactly one real
// flag yields aliases onto it; two or more real flags is a co-occurrence that
// must be an explicitly reviewed `ambiguous` entry or generation fails. Command
// scoped aliases override, blocks are removed and recorded, and every override
// path and target is validated against the live tree.
func ReduceParamAliases(root *cobra.Command) ([]ParamAliasEntry, error) {
	concepts, err := LoadParamConcepts()
	if err != nil {
		return nil, fmt.Errorf("load reviewed parameter concepts: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("parameter alias source root is nil")
	}

	overrideByPath := make(map[string]CommandOverride, len(concepts.Overrides))
	for _, ov := range concepts.Overrides {
		overrideByPath[ov.CommandPath] = ov
	}
	usedOverride := make(map[string]bool, len(overrideByPath))
	conceptsByPath := make(map[string][]Concept)
	usedConceptScope := make(map[string]bool)
	for _, concept := range concepts.Concepts {
		for _, path := range concept.Commands {
			conceptsByPath[path] = append(conceptsByPath[path], concept)
		}
	}

	var problems []string
	var entries []ParamAliasEntry

	walkRunnableParamCommands(root, func(leaf *cobra.Command) {
		path := normalizeSchemaCLIPath(leaf.CommandPath())
		realByMorph := realFlagsByMorph(leaf)
		ov, hasOverride := overrideByPath[path]
		if hasOverride {
			usedOverride[path] = true
		}
		scopedConcepts := conceptsByPath[path]
		for _, concept := range scopedConcepts {
			usedConceptScope[concept.ID+"\x00"+path] = true
			if !conceptHasRealFlag(concept, ov, realByMorph) {
				problems = append(problems, fmt.Sprintf("concept %q reviewed command %q has no matching real flag or reviewed bind", concept.ID, path))
			}
		}

		entry, entryProblems := reduceLeafParamAliases(path, realByMorph, scopedConcepts, ov)
		problems = append(problems, entryProblems...)
		if entry != nil {
			entries = append(entries, *entry)
		}
	})

	for path := range overrideByPath {
		if !usedOverride[path] {
			problems = append(problems, fmt.Sprintf("command_override %q does not match any runnable Cobra leaf", path))
		}
	}
	for _, concept := range concepts.Concepts {
		for _, path := range concept.Commands {
			if !usedConceptScope[concept.ID+"\x00"+path] {
				problems = append(problems, fmt.Sprintf("concept %q command scope %q does not match any runnable Cobra command", concept.ID, path))
			}
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("parameter alias reduction failed:\n  - %s", strings.Join(problems, "\n  - "))
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].CLIPath < entries[j].CLIPath })
	return entries, nil
}

// walkRunnableParamCommands invokes fn for every runnable command in the tree,
// including runnable parents such as `chat group members` that expose their own
// flags while also owning subcommands. Parameter aliasing applies to any command
// that accepts flags, which is broader than the schema's leaf-only traversal.
func walkRunnableParamCommands(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil {
		return
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() {
			fn(cmd)
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == "help" {
				continue
			}
			if !sub.IsAvailableCommand() && !hasRuntimeSchemaCommand(sub) {
				continue
			}
			walk(sub)
		}
	}
	walk(root)
}

// realFlag is one of a leaf's real flags, remembering whether it is hidden so
// the reduction can treat a hidden legacy alias flag (for example a hand-written
// --base living next to the visible --base-id) as an absorbable synonym rather
// than a genuine co-occurrence.
type realFlag struct {
	name   string
	hidden bool
}

// realFlagsByMorph maps each of a leaf's real flags (local + inherited) by its
// morphed name to the real flags that share that morph.
func realFlagsByMorph(leaf *cobra.Command) map[string][]realFlag {
	byMorph := make(map[string][]realFlag)
	visitManualAgentCommandFlags(leaf, func(flag *pflag.Flag) {
		if flag == nil || flag.Name == "help" {
			return
		}
		key := cmdutil.Morph(flag.Name)
		byMorph[key] = appendRealFlag(byMorph[key], realFlag{name: flag.Name, hidden: flag.Hidden})
	})
	return byMorph
}

// reduceLeafParamAliases computes one leaf's alias entry and returns any
// contract problems. A nil entry means the leaf produced no aliases, blocks, or
// ambiguous guards.
func reduceLeafParamAliases(path string, realByMorph map[string][]realFlag, concepts []Concept, ov CommandOverride) (*ParamAliasEntry, []string) {
	var problems []string
	aliasMap := make(map[string]string)
	blockedSet := make(map[string]bool)
	excludedSet := make(map[string]bool)
	pendingReview := ov.Confirm || ov.Investigate

	for boundFlag, conceptID := range ov.Bind {
		if _, ok := realByMorph[cmdutil.Morph(boundFlag)]; !ok {
			problems = append(problems, fmt.Sprintf("command_override %q binds %q to concept %q but %q is not a real flag", path, boundFlag, conceptID, boundFlag))
		}
	}

	// (a) Concept auto-reduction. The caller has already admitted only the
	// concepts whose reviewed command scope contains this exact leaf.
	for _, concept := range concepts {
		eff := make(map[string]bool, len(concept.Members)+2)
		for _, member := range concept.Members {
			eff[cmdutil.Morph(member)] = true
		}
		for boundFlag, conceptID := range ov.Bind {
			if conceptID == concept.ID {
				if pendingReview {
					for _, member := range concept.Members {
						morphed := cmdutil.Morph(member)
						if _, isReal := realByMorph[morphed]; !isReal {
							blockedSet[morphed] = true
						}
					}
				} else {
					eff[cmdutil.Morph(boundFlag)] = true
				}
			}
		}

		// Gather the concept's candidate real flags on this command, then
		// choose a canonical. A single visible real flag wins and absorbs the
		// rest (including hidden legacy alias flags). Two or more visible real
		// flags is a genuine co-occurrence that must be reviewed.
		var candidates []realFlag
		for key := range eff {
			candidates = append(candidates, realByMorph[key]...)
		}
		if len(candidates) == 0 {
			continue
		}
		visible := distinctRealNames(candidates, true)
		var canon string
		switch len(visible) {
		case 1:
			canon = visible[0]
		case 0:
			names := distinctRealNames(candidates, false)
			if len(names) != 1 {
				continue
			}
			canon = names[0]
		default:
			// Genuine co-occurrence: this concept intersects two or more
			// visible real flags, so it cannot be auto-reduced. Require that
			// every emittable synonym of THIS concept (a concept member that is
			// not itself a real flag on the command) is acknowledged in the
			// reviewed ambiguous whitelist. Checking only that the command has
			// some ambiguous entry would let one concept's whitelist silently
			// vouch for a different concept's unreviewed co-occurrence.
			ambiguousSet := make(map[string]bool, len(ov.Ambiguous))
			for _, a := range ov.Ambiguous {
				ambiguousSet[cmdutil.Morph(a)] = true
			}
			var unreviewed []string
			for m := range eff {
				if _, isReal := realByMorph[m]; isReal {
					continue
				}
				if !ambiguousSet[m] {
					unreviewed = append(unreviewed, m)
				}
			}
			if len(unreviewed) > 0 {
				sort.Strings(visible)
				sort.Strings(unreviewed)
				problems = append(problems, fmt.Sprintf("command %q concept %q intersects visible real flags %s; unreviewed emittable members %s must be listed in the ambiguous whitelist", path, concept.ID, strings.Join(visible, ","), strings.Join(unreviewed, ",")))
			}
			continue
		}
		for m := range eff {
			if _, isReal := realByMorph[m]; isReal {
				continue
			}
			if prev, ok := aliasMap[m]; ok && prev != canon {
				problems = append(problems, fmt.Sprintf("command %q emitted %q reduces to both %q and %q", path, m, prev, canon))
				continue
			}
			aliasMap[m] = canon
		}
		// Excludes are not passive prose: once this concept is active on a
		// reviewed command, a non-real excluded spelling is protected from
		// downstream fuzzy correction. A real flag is left alone because it
		// already has an independently valid command-local meaning.
		for _, exclude := range concept.Excludes {
			morphed := cmdutil.Morph(exclude)
			if _, isReal := realByMorph[morphed]; !isReal {
				excludedSet[morphed] = true
			}
		}
	}
	for excluded := range excludedSet {
		if _, isAlias := aliasMap[excluded]; !isAlias {
			blockedSet[excluded] = true
		}
	}

	// (b) Command scoped aliases override concept reductions.
	for emitted, target := range ov.ScopedAliases {
		morphedEmitted := cmdutil.Morph(emitted)
		reals, ok := realByMorph[cmdutil.Morph(target)]
		if !ok {
			problems = append(problems, fmt.Sprintf("command_override %q scoped alias %q->%q targets %q which is not a real flag", path, emitted, target, target))
			continue
		}
		if _, sourceIsReal := realByMorph[morphedEmitted]; sourceIsReal {
			problems = append(problems, fmt.Sprintf("command_override %q scoped alias source %q is already a real flag; keep its native compatibility path or remove it before enabling semantic rewrite", path, emitted))
			continue
		}
		if pendingReview {
			delete(aliasMap, morphedEmitted)
			blockedSet[morphedEmitted] = true
			continue
		}
		delete(blockedSet, morphedEmitted)
		aliasMap[morphedEmitted] = canonicalRealName(reals)
	}

	// (c) Blocks are removed from the alias map and recorded for did-you-mean.
	for _, b := range ov.Block {
		mb := cmdutil.Morph(b)
		if _, isReal := realByMorph[mb]; isReal {
			problems = append(problems, fmt.Sprintf("command_override %q blocks %q but it is already a real flag; blocking must not disable a canonical/native parameter", path, b))
			continue
		}
		delete(aliasMap, mb)
		blockedSet[mb] = true
	}
	ambiguous := make([]string, 0, len(ov.Ambiguous))
	for _, a := range ov.Ambiguous {
		ma := cmdutil.Morph(a)
		if _, isReal := realByMorph[ma]; isReal {
			problems = append(problems, fmt.Sprintf("command_override %q marks %q ambiguous but it is already a real flag", path, a))
			continue
		}
		if canon, ok := aliasMap[ma]; ok {
			problems = append(problems, fmt.Sprintf("command %q name %q is both auto-reduced to %q and marked ambiguous; a name cannot be aliased and ambiguous at once", path, a, canon))
		}
		delete(aliasMap, ma)
		delete(blockedSet, ma)
		ambiguous = append(ambiguous, ma)
	}
	blocked := make([]string, 0, len(blockedSet))
	for b := range blockedSet {
		blocked = append(blocked, b)
	}

	// A name cannot be both auto-reduced and ambiguous: the alias table says
	// "silently rewrite it" while the ambiguous list says "stop and ask". This
	// usually means a reviewer hand-listed a name in the ambiguous whitelist
	// that the concept can in fact resolve to a single real flag.
	for _, a := range ambiguous {
		if canon, ok := aliasMap[a]; ok {
			problems = append(problems, fmt.Sprintf("command %q name %q is both auto-reduced to %q and marked ambiguous; a name cannot be aliased and ambiguous at once", path, a, canon))
		}
	}

	if len(aliasMap) == 0 && len(blocked) == 0 && len(ambiguous) == 0 {
		return nil, problems
	}
	return &ParamAliasEntry{
		CLIPath:   path,
		Aliases:   aliasMap,
		Blocked:   sortedUnique(blocked),
		Ambiguous: sortedUnique(ambiguous),
	}, problems
}

func conceptHasRealFlag(concept Concept, ov CommandOverride, realByMorph map[string][]realFlag) bool {
	for _, member := range concept.Members {
		if _, ok := realByMorph[cmdutil.Morph(member)]; ok {
			return true
		}
	}
	for boundFlag, conceptID := range ov.Bind {
		if conceptID == concept.ID {
			if _, ok := realByMorph[cmdutil.Morph(boundFlag)]; ok {
				return true
			}
		}
	}
	return false
}

func appendRealFlag(list []realFlag, value realFlag) []realFlag {
	for i, existing := range list {
		if existing.name == value.name {
			// Prefer the visible record if any registration is visible.
			if existing.hidden && !value.hidden {
				list[i] = value
			}
			return list
		}
	}
	list = append(list, value)
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	return list
}

// distinctRealNames returns the sorted unique flag names among candidates,
// optionally restricted to visible (non-hidden) flags.
func distinctRealNames(candidates []realFlag, visibleOnly bool) []string {
	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if visibleOnly && c.hidden {
			continue
		}
		if seen[c.name] {
			continue
		}
		seen[c.name] = true
		out = append(out, c.name)
	}
	sort.Strings(out)
	return out
}

// canonicalRealName chooses the canonical target among real flags sharing a
// morph key, preferring a visible flag over a hidden legacy alias.
func canonicalRealName(reals []realFlag) string {
	for _, r := range reals {
		if !r.hidden {
			return r.name
		}
	}
	if len(reals) > 0 {
		return reals[0].name
	}
	return ""
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
