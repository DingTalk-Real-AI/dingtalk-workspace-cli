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

package handlers

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// ParamNameHandler performs fuzzy correction on flag names that are
// not recognised after alias normalisation. It uses Levenshtein edit
// distance to find the closest known flag name, with a conservative
// threshold to avoid false positives.
//
// Correction rules:
//   - Edit distance must be ≤ maxEditDistance (default 2).
//   - The match must be unambiguous (exactly one candidate within threshold).
//   - Very short flag names (≤ 3 chars) use a tighter threshold of 1.
//
// This handler should run after AliasHandler in the PreParse phase
// so that obvious normalisation (camelCase → kebab-case) is already
// done and fuzzy matching only handles genuine near-misses.
type ParamNameHandler struct{}

const maxEditDistance = 2

func (ParamNameHandler) Name() string          { return "paramname" }
func (ParamNameHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (ParamNameHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Args) == 0 || len(ctx.FlagSpecs) == 0 {
		return nil
	}

	known := buildFlagSet(ctx.FlagSpecs)
	names := make([]string, 0, len(ctx.FlagSpecs))
	for _, spec := range ctx.FlagSpecs {
		if spec.Name != "" {
			names = append(names, spec.Name)
		}
	}

	result := make([]string, 0, len(ctx.Args))
	for _, arg := range ctx.Args {
		rewritten, ok := tryFuzzyMatch(arg, known, names)
		if ok {
			ctx.AddCorrection("paramname", pipeline.PreParse, rewritten, arg, rewritten, "fuzzy")
			result = append(result, rewritten)
		} else {
			result = append(result, arg)
		}
	}

	ctx.Args = result
	return nil
}

// tryFuzzyMatch attempts to correct an unrecognised "--flag" token by
// finding the closest known flag name within the edit distance threshold.
func tryFuzzyMatch(arg string, known map[string]bool, candidates []string) (string, bool) {
	if !strings.HasPrefix(arg, "--") {
		return "", false
	}

	bare := arg[2:]
	if bare == "" {
		return "", false
	}

	// Handle --flag=value syntax.
	var suffix string
	if idx := strings.IndexByte(bare, '='); idx >= 0 {
		suffix = bare[idx:]
		bare = bare[:idx]
	}

	// Already known — nothing to fix.
	if known[bare] {
		return "", false
	}

	threshold := maxEditDistance
	if len(bare) <= 3 {
		threshold = 1
	}

	bestDist := threshold + 1
	bestMatch := ""
	ambiguous := false

	for _, candidate := range candidates {
		dist := levenshtein(bare, candidate)
		if dist < bestDist {
			bestDist = dist
			bestMatch = candidate
			ambiguous = false
		} else if dist == bestDist && candidate != bestMatch {
			ambiguous = true
		}
	}

	if bestDist > threshold || ambiguous || bestMatch == "" {
		return "", false
	}

	return "--" + bestMatch + suffix, true
}

// levenshtein computes the edit distance between two strings using
// the standard dynamic programming approach with O(min(m,n)) space.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}

	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Ensure a is the shorter string for O(min) space.
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}

	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := range prev {
		prev[i] = i
	}

	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[i] = min(
				prev[i]+1,      // deletion
				curr[i-1]+1,    // insertion
				prev[i-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[la]
}
