// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package contract

import "strings"

// RuntimeSchemaConstraints describes cross-parameter rules that cannot be
// represented by an individual parameter's required bit.
type RuntimeSchemaConstraints struct {
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`
	RequireOneOf      [][]string `json:"require_one_of,omitempty"`
	RequireTogether   [][]string `json:"require_together,omitempty"`
}

// NormalizeRuntimeSchemaConstraints trims, deduplicates, and drops undersized groups.
func NormalizeRuntimeSchemaConstraints(constraints RuntimeSchemaConstraints) RuntimeSchemaConstraints {
	constraints.MutuallyExclusive = normalizeConstraintGroups(constraints.MutuallyExclusive, 2)
	constraints.RequireOneOf = normalizeConstraintGroups(constraints.RequireOneOf, 1)
	constraints.RequireTogether = normalizeConstraintGroups(constraints.RequireTogether, 2)
	return constraints
}

// RuntimeSchemaConstraintsEmpty reports whether no constraint groups remain.
func RuntimeSchemaConstraintsEmpty(constraints RuntimeSchemaConstraints) bool {
	return len(constraints.MutuallyExclusive) == 0 &&
		len(constraints.RequireOneOf) == 0 &&
		len(constraints.RequireTogether) == 0
}

func normalizeConstraintGroups(groups [][]string, minimum int) [][]string {
	out := make([][]string, 0, len(groups))
	seenGroups := map[string]bool{}
	for _, group := range groups {
		clean := make([]string, 0, len(group))
		seenNames := map[string]bool{}
		for _, name := range group {
			name = strings.TrimSpace(name)
			if name == "" || seenNames[name] {
				continue
			}
			seenNames[name] = true
			clean = append(clean, name)
		}
		if len(clean) < minimum {
			continue
		}
		key := strings.Join(clean, "\x00")
		if seenGroups[key] {
			continue
		}
		seenGroups[key] = true
		out = append(out, clean)
	}
	return out
}
