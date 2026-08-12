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

package runtimeannotate

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

// RuntimeSchemaConstraints describes cross-parameter rules that cannot be
// represented by an individual parameter's required bit.
type RuntimeSchemaConstraints struct {
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`
	RequireOneOf      [][]string `json:"require_one_of,omitempty"`
	RequireTogether   [][]string `json:"require_together,omitempty"`
}

// AnnotateRuntimeConstraints records command-level parameter relationships.
func AnnotateRuntimeConstraints(cmd *cobra.Command, constraints RuntimeSchemaConstraints) {
	if cmd == nil {
		return
	}
	constraints = NormalizeConstraints(constraints)
	if ConstraintsEmpty(constraints) {
		return
	}
	if existing := CommandConstraints(cmd); !ConstraintsEmpty(existing) {
		constraints.MutuallyExclusive = append(existing.MutuallyExclusive, constraints.MutuallyExclusive...)
		constraints.RequireOneOf = append(existing.RequireOneOf, constraints.RequireOneOf...)
		constraints.RequireTogether = append(existing.RequireTogether, constraints.RequireTogether...)
		constraints = NormalizeConstraints(constraints)
	}
	encoded, _ := json.Marshal(constraints)
	SetCommandAnnotation(cmd, AnnotationConstraints, string(encoded))
}

// CommandConstraints reads the annotated constraint payload (empty on miss/error).
func CommandConstraints(cmd *cobra.Command) RuntimeSchemaConstraints {
	if cmd == nil || cmd.Annotations == nil {
		return RuntimeSchemaConstraints{}
	}
	raw := strings.TrimSpace(cmd.Annotations[AnnotationConstraints])
	if raw == "" {
		return RuntimeSchemaConstraints{}
	}
	var constraints RuntimeSchemaConstraints
	if json.Unmarshal([]byte(raw), &constraints) != nil {
		return RuntimeSchemaConstraints{}
	}
	return NormalizeConstraints(constraints)
}

// NormalizeConstraints trims, deduplicates, and drops undersized groups.
func NormalizeConstraints(constraints RuntimeSchemaConstraints) RuntimeSchemaConstraints {
	constraints.MutuallyExclusive = normalizeGroups(constraints.MutuallyExclusive, 2)
	constraints.RequireOneOf = normalizeGroups(constraints.RequireOneOf, 1)
	constraints.RequireTogether = normalizeGroups(constraints.RequireTogether, 2)
	return constraints
}

// ConstraintsEmpty reports whether no constraint groups remain.
func ConstraintsEmpty(constraints RuntimeSchemaConstraints) bool {
	return len(constraints.MutuallyExclusive) == 0 &&
		len(constraints.RequireOneOf) == 0 &&
		len(constraints.RequireTogether) == 0
}

func normalizeGroups(groups [][]string, minimum int) [][]string {
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
