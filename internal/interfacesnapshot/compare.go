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

package interfacesnapshot

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Report combines comparisons against independently supplied compatibility
// references, normally the PR merge-base and the latest stable GA release.
type Report struct {
	Compatible  bool         `json:"compatible"`
	Comparisons []Comparison `json:"comparisons"`
}

// Comparison is the result for one reference snapshot.
type Comparison struct {
	Reference  string   `json:"reference"`
	Compatible bool     `json:"compatible"`
	Blocking   []Change `json:"blocking"`
	Additions  []Change `json:"additions"`
}

// Change describes one compatibility decision. Before and After are concise,
// human-readable values intended for CI annotations.
type Change struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Flag   string `json:"flag,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// CompareAll compares current against every named reference in one pass.
// Additions are reported but remain compatible.
func CompareAll(current Snapshot, references map[string]Snapshot) Report {
	labels := make([]string, 0, len(references))
	for label := range references {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	report := Report{Compatible: true, Comparisons: []Comparison{}}
	for _, label := range labels {
		comparison := Compare(current, references[label], label)
		report.Comparisons = append(report.Comparisons, comparison)
		if !comparison.Compatible {
			report.Compatible = false
		}
	}
	return report
}

// Compare enforces the deliberately small admission policy:
//   - every previously accepted command path must still resolve to a runnable,
//     visible-compatible target (a rename may preserve the old path as an
//     alias),
//   - flags accepted at each command path may not disappear, change type, or
//     become required; an existing path may not gain a new required flag,
//   - new commands and flags are allowed.
//
// Comparing the effective local + inherited set is intentional. It catches a
// persistent flag whose scope is accidentally narrowed to its declaring
// command, while allowing a local flag to move to an ancestor without breaking
// the old invocation path.
func Compare(current, baseline Snapshot, reference string) Comparison {
	result := Comparison{
		Reference:  reference,
		Compatible: true,
		Blocking:   []Change{},
		Additions:  []Change{},
	}

	if !reflect.DeepEqual(current.Rules, baseline.Rules) {
		result.Blocking = append(result.Blocking, Change{
			Kind:   "snapshot_rules_changed",
			Path:   "dws",
			Before: fmt.Sprintf("%v", baseline.Rules),
			After:  fmt.Sprintf("%v", current.Rules),
		})
	}
	result.Blocking = append(result.Blocking, aliasCollisionChanges(current)...)

	currentCommands := commandIndex(current)
	baselineCommands := commandIndex(baseline)
	currentAccepted := acceptedPathIndex(current)
	baselineAccepted := acceptedPathIndex(baseline)

	removedPaths := make([]string, 0)
	for path := range baselineAccepted {
		if _, ok := currentAccepted[path]; !ok {
			removedPaths = append(removedPaths, path)
		}
	}
	for _, path := range minimalPaths(removedPaths) {
		kind := "command_alias_removed"
		if _, canonical := baselineCommands[path]; canonical {
			kind = "command_removed"
		}
		result.Blocking = append(result.Blocking, Change{Kind: kind, Path: path})
	}

	mappedCurrent := make(map[string]bool)
	for _, acceptedPath := range sortedAcceptedPaths(baselineAccepted) {
		oldPath := baselineAccepted[acceptedPath]
		oldCommand := baselineCommands[oldPath]
		newPath, ok := currentAccepted[acceptedPath]
		if !ok {
			continue
		}
		newCommand, ok := currentCommands[newPath]
		if !ok {
			continue
		}
		mappedCurrent[newPath] = true
		compareCommandContract(&result, acceptedPath, oldCommand, newCommand)
		compareEffectiveFlags(&result, acceptedPath, oldCommand, newCommand)
	}

	for _, path := range sortedCommandPaths(current.Commands) {
		if mappedCurrent[path] {
			continue
		}
		if _, existed := baselineCommands[path]; !existed {
			result.Additions = append(result.Additions, Change{Kind: "command_added", Path: path})
		}
	}

	sortChanges(result.Blocking)
	sortChanges(result.Additions)
	result.Blocking = dedupeChanges(result.Blocking)
	result.Additions = dedupeChanges(result.Additions)
	result.Compatible = len(result.Blocking) == 0
	return result
}

func compareCommandContract(result *Comparison, acceptedPath string, oldCommand, newCommand Command) {
	if oldCommand.Runnable && !newCommand.Runnable {
		result.Blocking = append(result.Blocking, Change{
			Kind:   "command_became_non_runnable",
			Path:   acceptedPath,
			Before: "runnable",
			After:  "non-runnable",
		})
	}
	if !oldCommand.Hidden && newCommand.Hidden {
		result.Blocking = append(result.Blocking, Change{
			Kind:   "command_became_hidden",
			Path:   acceptedPath,
			Before: "visible",
			After:  "hidden",
		})
	}
}

func compareEffectiveFlags(result *Comparison, acceptedPath string, oldCommand, newCommand Command) {
	oldFlags := effectiveFlagIndex(oldCommand)
	newFlags := effectiveFlagIndex(newCommand)

	for _, name := range sortedFlagNames(oldFlags) {
		oldFlag := oldFlags[name]
		newFlag, ok := newFlags[name]
		if !ok {
			result.Blocking = append(result.Blocking, Change{
				Kind: "flag_removed",
				Path: acceptedPath,
				Flag: name,
			})
			continue
		}
		if oldFlag.Type != newFlag.Type {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "flag_type_changed",
				Path:   acceptedPath,
				Flag:   name,
				Before: oldFlag.Type,
				After:  newFlag.Type,
			})
		}
		if !oldFlag.Required && newFlag.Required {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "flag_became_required",
				Path:   acceptedPath,
				Flag:   name,
				Before: "optional",
				After:  "required",
			})
		}
		if oldFlag.Shorthand != "" && newFlag.Shorthand != oldFlag.Shorthand {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "flag_shorthand_changed",
				Path:   acceptedPath,
				Flag:   name,
				Before: oldFlag.Shorthand,
				After:  newFlag.Shorthand,
			})
		}
		if oldFlag.NoOpt != "" && newFlag.NoOpt != oldFlag.NoOpt {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "flag_no_opt_changed",
				Path:   acceptedPath,
				Flag:   name,
				Before: oldFlag.NoOpt,
				After:  newFlag.NoOpt,
			})
		}
		if !oldFlag.Hidden && newFlag.Hidden {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "flag_became_hidden",
				Path:   acceptedPath,
				Flag:   name,
				Before: "visible",
				After:  "hidden",
			})
		}
	}

	for _, name := range sortedFlagNames(newFlags) {
		newFlag := newFlags[name]
		if _, existed := oldFlags[name]; existed {
			continue
		}
		if newFlag.Required {
			result.Blocking = append(result.Blocking, Change{
				Kind:   "required_flag_added",
				Path:   acceptedPath,
				Flag:   name,
				Before: "absent",
				After:  "required",
			})
		}
	}

	// Report optional additions once where they are declared. Required
	// additions were handled against the effective set above because they can
	// break every descendant invocation.
	newLocalFlags := flagIndex(newCommand.LocalFlags)
	for _, name := range sortedFlagNames(newLocalFlags) {
		newFlag := newLocalFlags[name]
		if _, existed := oldFlags[name]; existed || newFlag.Required {
			continue
		}
		result.Additions = append(result.Additions, Change{Kind: "flag_added", Path: newCommand.Path, Flag: name})
	}
}

func commandIndex(snapshot Snapshot) map[string]Command {
	out := make(map[string]Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		out[command.Path] = command
	}
	return out
}

func aliasCollisionChanges(snapshot Snapshot) []Change {
	acceptedSiblings := make(map[string]string)
	changes := []Change{}
	for _, command := range snapshot.Commands {
		parent, name := splitParent(command.Path)
		for _, acceptedName := range append([]string{name}, command.Aliases...) {
			acceptedName = strings.TrimSpace(acceptedName)
			if acceptedName == "" {
				continue
			}
			key := parent + "\x00" + acceptedName
			if previous, exists := acceptedSiblings[key]; exists && previous != command.Path {
				changes = append(changes, Change{
					Kind:   "command_alias_collision",
					Path:   strings.TrimSpace(parent + " " + acceptedName),
					Before: previous,
					After:  command.Path,
				})
				continue
			}
			acceptedSiblings[key] = command.Path
		}
	}
	return changes
}

// acceptedPathIndex expands aliases at every path segment. For example, if
// "dws chat" has alias "im", then "dws im message send" remains accepted for
// every descendant even though descendants only store their canonical paths.
func acceptedPathIndex(snapshot Snapshot) map[string]string {
	commands := append([]Command(nil), snapshot.Commands...)
	sort.Slice(commands, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(commands[i].Path), pathDepth(commands[j].Path)
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return commands[i].Path < commands[j].Path
	})

	variants := make(map[string][]string, len(commands))
	accepted := make(map[string]string)
	// Canonical paths always win over an alias collision.
	for _, command := range commands {
		accepted[command.Path] = command.Path
	}

	for _, command := range commands {
		parent, name := splitParent(command.Path)
		names := compactSorted(append([]string{name}, command.Aliases...))
		parentVariants := variants[parent]
		if parent == "" {
			parentVariants = []string{""}
		} else if len(parentVariants) == 0 {
			parentVariants = []string{parent}
		}

		commandVariants := make([]string, 0, len(parentVariants)*len(names))
		for _, parentVariant := range parentVariants {
			for _, candidateName := range names {
				path := strings.TrimSpace(parentVariant + " " + candidateName)
				commandVariants = append(commandVariants, path)
				if _, canonical := accepted[path]; !canonical {
					accepted[path] = command.Path
				}
			}
		}
		variants[command.Path] = compactSorted(commandVariants)
	}
	return accepted
}

func flagIndex(flags []Flag) map[string]Flag {
	out := make(map[string]Flag, len(flags))
	for _, flag := range flags {
		out[flag.Name] = flag
	}
	return out
}

func effectiveFlagIndex(command Command) map[string]Flag {
	out := flagIndex(command.InheritedFlags)
	for _, flag := range command.LocalFlags {
		// A local flag is the callable definition when it shadows an inherited
		// flag with the same name.
		out[flag.Name] = flag
	}
	return out
}

func sortedCommandPaths(commands []Command) []string {
	paths := make([]string, 0, len(commands))
	for _, command := range commands {
		paths = append(paths, command.Path)
	}
	sort.Strings(paths)
	return paths
}

func sortedAcceptedPaths(accepted map[string]string) []string {
	paths := make([]string, 0, len(accepted))
	for path := range accepted {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sortedFlagNames(flags map[string]Flag) []string {
	names := make([]string, 0, len(flags))
	for name := range flags {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func minimalPaths(paths []string) []string {
	sort.Slice(paths, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(paths[i]), pathDepth(paths[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return paths[i] < paths[j]
	})
	minimal := make([]string, 0, len(paths))
	for _, path := range paths {
		covered := false
		for _, parent := range minimal {
			if strings.HasPrefix(path, parent+" ") {
				covered = true
				break
			}
		}
		if !covered {
			minimal = append(minimal, path)
		}
	}
	return minimal
}

func splitParent(path string) (string, string) {
	index := strings.LastIndexByte(path, ' ')
	if index < 0 {
		return "", path
	}
	return path[:index], path[index+1:]
}

func pathDepth(path string) int {
	return len(strings.Fields(path))
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool {
		left := changes[i].Path + "\x00" + changes[i].Flag + "\x00" + changes[i].Kind
		right := changes[j].Path + "\x00" + changes[j].Flag + "\x00" + changes[j].Kind
		return left < right
	})
}

func dedupeChanges(changes []Change) []Change {
	if len(changes) < 2 {
		return changes
	}
	out := changes[:1]
	for _, change := range changes[1:] {
		if change == out[len(out)-1] {
			continue
		}
		out = append(out, change)
	}
	return out
}
