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

package cmdpolicy

import (
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

// nodeKind classifies a Cobra node by what executing it does. Whether a node
// has RunE cannot decide this: navigation groups carry a framework RunE too.
type nodeKind int

const (
	// kindNavigation never runs business logic: a navigation-only group, or a
	// node that is not runnable. It is not evaluated.
	kindNavigation nodeKind = iota
	// kindLeaf is a runnable node without children.
	kindLeaf
	// kindHybrid is a declared hybrid group: its own business operation is
	// evaluated independently of its children.
	kindHybrid
	// kindUndeclaredGroup is a runnable node with children and no group
	// policy. It is evaluated like a hybrid but owns no recovery contract.
	kindUndeclaredGroup
)

func classify(cmd *cobra.Command) nodeKind {
	if !cmd.Runnable() {
		return kindNavigation
	}
	if !cmd.HasSubCommands() {
		return kindLeaf
	}
	policy, declared, err := corecmd.GroupPolicyFor(cmd)
	if err != nil || !declared {
		return kindUndeclaredGroup
	}
	if policy.Mode == corecmd.GroupHybrid {
		return kindHybrid
	}
	return kindNavigation
}

// entry is one executable Cobra node.
type entry struct {
	cmd      *cobra.Command
	path     string
	kind     nodeKind
	identity *contract.ToolIdentitySpec
	safety   safetyFacts
}

// identityIndex groups executable entries by the command identity they reach.
//
// A primary leaf declares ContractFinal.Identity, including the alias paths
// that reach the same operation. An independently mounted compatibility node
// (for example "report create" for "report entry submit") carries no Identity
// of its own; it is attributed to the primary that names it, and shares that
// primary's Safety and verdict. Attribution that cannot be established
// reliably is a conflict and fails closed.
type identityIndex struct {
	entries []*entry
	// owner maps an entry path to the primary path of its identity.
	owner map[string]string
	// subjects is keyed by primary path.
	subjects map[string]*subject
	// canonical maps a canonical dot path to its primary path.
	canonical map[string]string
	conflicts map[string]string
}

func buildIdentityIndex(root *cobra.Command) *identityIndex {
	index := &identityIndex{
		owner:     map[string]string{},
		subjects:  map[string]*subject{},
		canonical: map[string]string{},
		conflicts: map[string]string{},
	}
	byPath := map[string]*entry{}
	walk(root, func(cmd *cobra.Command) {
		kind := classify(cmd)
		if kind == kindNavigation {
			return
		}
		item := &entry{cmd: cmd, path: CommandPath(cmd), kind: kind, safety: commandSafety(cmd)}
		if final, ok := contractfinal.RuntimeContractFinal(cmd); ok && final.Identity != nil {
			item.identity = final.Identity
		}
		index.entries = append(index.entries, item)
		byPath[item.path] = item
	})

	// Pass 1: declared identities claim their primary path and their aliases.
	claim := func(path, primary string) {
		if path == "" {
			return
		}
		if existing, taken := index.owner[path]; taken && existing != primary {
			index.conflicts[path] = "claimed by both " + existing + " and " + primary
			return
		}
		index.owner[path] = primary
	}
	for _, item := range index.entries {
		if item.identity == nil {
			continue
		}
		primary := primaryPath(*item.identity)
		if primary == "" {
			primary = item.path
		}
		claim(item.path, primary)
		for _, alias := range item.identity.Aliases {
			claim(DeclaredPath(alias), primary)
		}
		if canonical := strings.TrimSpace(item.identity.CanonicalPath); canonical != "" {
			if existing, taken := index.canonical[canonical]; taken && existing != primary {
				index.conflicts[item.path] = "canonical path " + canonical + " is declared for both " + existing + " and " + primary
			} else {
				index.canonical[canonical] = primary
			}
		}
	}

	// Pass 2: every entry joins the subject of its owner; unclaimed entries
	// are their own identity. Each entry also contributes every spelling Cobra
	// accepts for it: its own native aliases and those of its ancestor groups
	// ("log entry submit" for "report entry submit"). They are executable
	// entry paths of the same identity, so a Deny written against any of them
	// applies to all.
	for _, item := range index.entries {
		primary, owned := index.owner[item.path]
		if !owned {
			primary = item.path
			index.owner[item.path] = primary
		}
		subj := index.subjects[primary]
		if subj == nil {
			subj = &subject{primary: primary}
			index.subjects[primary] = subj
		}
		subj.entries = append(subj.entries, item.path)
		for _, spelling := range nativeAliasPaths(item.cmd) {
			if _, live := byPath[spelling]; live {
				// A real node owns this path; Cobra dispatches to it, not to
				// the alias, so it is not a spelling of this entry.
				continue
			}
			if existing, taken := index.owner[spelling]; taken && existing != primary {
				index.conflicts[item.path] = "alias path " + spelling + " is also claimed by " + existing
				continue
			}
			index.owner[spelling] = primary
			subj.entries = append(subj.entries, spelling)
		}
	}
	// The identity's Safety is the primary node's. When the primary path has
	// no live node, fall back to the declaring entries, which must agree.
	for primary, subj := range index.subjects {
		sort.Strings(subj.entries)
		if item, ok := byPath[primary]; ok {
			subj.safety = item.safety
			continue
		}
		first := true
		for _, path := range subj.entries {
			item := byPath[path]
			if item == nil || item.identity == nil {
				continue
			}
			if first {
				subj.safety, first = item.safety, false
			} else if item.safety != subj.safety {
				index.conflicts[path] = "entries of " + primary + " declare different Safety"
			}
		}
	}
	return index
}

// primaryPath mirrors the Schema collector: PrimaryCLIPath wins, CLIPath fills
// when it is empty. The app-level test pins both implementations together.
func primaryPath(identity contract.ToolIdentitySpec) string {
	primary := strings.TrimSpace(identity.PrimaryCLIPath)
	if primary == "" {
		primary = strings.TrimSpace(identity.CLIPath)
	}
	return DeclaredPath(primary)
}

// nativeAliasPaths returns every alternative policy path Cobra resolves to cmd
// through native command aliases at any level, excluding the primary path.
func nativeAliasPaths(cmd *cobra.Command) []string {
	var levels [][]string
	hasAlias := false
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		names := append([]string{c.Name()}, c.Aliases...)
		if len(names) > 1 {
			hasAlias = true
		}
		levels = append([][]string{names}, levels...)
	}
	if !hasAlias {
		return nil
	}
	paths := []string{""}
	for _, names := range levels {
		next := make([]string, 0, len(paths)*len(names))
		for _, prefix := range paths {
			for _, name := range names {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				if prefix == "" {
					next = append(next, name)
				} else {
					next = append(next, prefix+"/"+name)
				}
			}
		}
		paths = next
	}
	primary := CommandPath(cmd)
	out := paths[:0]
	for _, path := range paths {
		if path != primary {
			out = append(out, path)
		}
	}
	return out
}

func walk(cmd *cobra.Command, visit func(*cobra.Command)) {
	if cmd == nil {
		return
	}
	if cmd.HasParent() {
		visit(cmd)
	}
	for _, child := range cmd.Commands() {
		walk(child, visit)
	}
}
