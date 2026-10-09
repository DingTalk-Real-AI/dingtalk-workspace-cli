// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cmdutil

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// MaxCommandSuggestions keeps typo recovery concise even for products with a
// large command surface.
const MaxCommandSuggestions = 3

type rankedCommandSuggestion struct {
	name        string
	explicit    bool
	prefix      bool
	distance    int
	lengthDelta int
}

// SuggestSubcommands returns at most MaxCommandSuggestions visible canonical
// child names, ranked by reviewed SuggestFor matches, prefix relevance, edit
// distance, length delta, and finally canonical name for deterministic ties.
// Aliases participate in scoring, but recovery always teaches the canonical
// command name.
func SuggestSubcommands(parent *cobra.Command, candidate string) []string {
	if parent == nil {
		return nil
	}
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "" {
		return []string{}
	}
	minimumDistance := parent.SuggestionsMinimumDistance
	if minimumDistance <= 0 {
		minimumDistance = 2
	}
	shortcutOnly := strings.HasPrefix(candidate, "+")
	ranked := make([]rankedCommandSuggestion, 0)
	for _, child := range parent.Commands() {
		if !child.IsAvailableCommand() || (shortcutOnly && !strings.HasPrefix(child.Name(), "+")) {
			continue
		}

		explicit := false
		for _, suggestion := range child.SuggestFor {
			if strings.EqualFold(candidate, suggestion) {
				explicit = true
				break
			}
		}

		identities := append([]string{child.Name()}, child.Aliases...)
		prefix := false
		bestDistance := -1
		bestLengthDelta := 0
		for _, identity := range identities {
			identity = strings.ToLower(strings.TrimSpace(identity))
			if identity == "" {
				continue
			}
			if strings.HasPrefix(identity, candidate) {
				prefix = true
			}
			distance := LevenshteinDist(candidate, identity)
			lengthDelta := len(identity) - len(candidate)
			if lengthDelta < 0 {
				lengthDelta = -lengthDelta
			}
			if bestDistance < 0 || distance < bestDistance || (distance == bestDistance && lengthDelta < bestLengthDelta) {
				bestDistance = distance
				bestLengthDelta = lengthDelta
			}
		}
		if !explicit && !prefix && (bestDistance < 0 || bestDistance > minimumDistance) {
			continue
		}
		ranked = append(ranked, rankedCommandSuggestion{
			name:        child.Name(),
			explicit:    explicit,
			prefix:      prefix,
			distance:    bestDistance,
			lengthDelta: bestLengthDelta,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		if left.explicit != right.explicit {
			return left.explicit
		}
		if left.prefix != right.prefix {
			return left.prefix
		}
		if left.distance != right.distance {
			return left.distance < right.distance
		}
		if left.lengthDelta != right.lengthDelta {
			return left.lengthDelta < right.lengthDelta
		}
		return left.name < right.name
	})
	if len(ranked) > MaxCommandSuggestions {
		ranked = ranked[:MaxCommandSuggestions]
	}

	suggestions := make([]string, 0, len(ranked))
	for _, suggestion := range ranked {
		suggestions = append(suggestions, suggestion.name)
	}
	return suggestions
}

// SuggestDescendantSubcommands returns bounded relative paths for available
// descendants whose canonical name or alias exactly matches candidate. It is
// the reviewed deep-recovery mode used by hierarchical products such as
// Sheet, where a model may omit an intermediate group (`sheet read` instead
// of `sheet range read`). Fuzzy ranking remains a sibling concern so a large
// subtree cannot drown the user in speculative paths.
func SuggestDescendantSubcommands(parent *cobra.Command, candidate string) []string {
	if parent == nil {
		return nil
	}
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "" {
		return []string{}
	}

	paths := make([]string, 0)
	var walk func(*cobra.Command, []string)
	walk = func(command *cobra.Command, prefix []string) {
		for _, child := range command.Commands() {
			if !child.IsAvailableCommand() {
				continue
			}
			path := append(append([]string(nil), prefix...), child.Name())
			matched := strings.EqualFold(strings.TrimSpace(child.Name()), candidate)
			if !matched {
				for _, alias := range child.Aliases {
					if strings.EqualFold(strings.TrimSpace(alias), candidate) {
						matched = true
						break
					}
				}
			}
			if matched {
				paths = append(paths, strings.Join(path, " "))
			}
			walk(child, path)
		}
	}
	walk(parent, nil)
	sort.Strings(paths)
	return normalizeCommandSuggestions(paths)
}

// FormatSubcommandSuggestionHint renders bounded suggestions and always keeps
// the full-list recovery action visible in the same hint.
func FormatSubcommandSuggestionHint(parent *cobra.Command, suggestions []string, fallback string) string {
	if len(suggestions) > MaxCommandSuggestions {
		suggestions = suggestions[:MaxCommandSuggestions]
	}
	paths := make([]string, 0, len(suggestions))
	parentPath := commandResolutionParentPath(parent)
	for _, suggestion := range suggestions {
		paths = append(paths, fmt.Sprintf("%q", parentPath+" "+suggestion))
	}
	var hint string
	switch len(paths) {
	case 0:
		return fallback
	case 1:
		hint = "Did you mean " + paths[0] + "?"
	default:
		hint = "Did you mean one of: " + strings.Join(paths, ", ") + "?"
	}
	if fallback != "" {
		hint += " (" + fallback + ")"
	}
	return hint
}
