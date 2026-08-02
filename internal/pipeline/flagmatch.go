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

package pipeline

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
)

const maxFlagEditDistance = 2

// NormalizeFlagToken folds a long flag's morphological spelling and returns a
// canonical token only when the folded name is a real flag. Both the normal
// PreParse handler chain and Cobra command traversal use this primitive so a
// spelling accepted after a command is also recognised before it.
func NormalizeFlagToken(argument string, known map[string]bool) (string, bool) {
	if !strings.HasPrefix(argument, "--") {
		return "", false
	}

	bare := argument[2:]
	if bare == "" {
		return "", false
	}

	var suffix string
	if index := strings.IndexByte(bare, '='); index >= 0 {
		suffix = bare[index:]
		bare = bare[:index]
	}
	if known[bare] {
		return "", false
	}

	normalized := cmdutil.Morph(bare)
	if normalized == bare || !known[normalized] {
		return "", false
	}
	return "--" + normalized + suffix, true
}

// StickyFlagPair is the canonical flag and value resolved from one glued
// long-flag token. Inline is required for boolean flags because pflag treats a
// bare boolean as true without consuming the following argv token.
type StickyFlagPair struct {
	Flag   string
	Value  string
	Inline bool
}

// SplitStickyFlag splits a safely recognisable glued flag/value token. The
// suffix must satisfy the real flag's type/format/enum contract, preventing a
// typo from being reinterpreted as data.
func SplitStickyFlag(argument string, specByName map[string]FlagInfo) (StickyFlagPair, bool) {
	if !strings.HasPrefix(argument, "--") || strings.Contains(argument, "=") {
		return StickyFlagPair{}, false
	}

	bare := argument[2:]
	if bare == "" {
		return StickyFlagPair{}, false
	}
	if _, ok := specByName[bare]; ok {
		return StickyFlagPair{}, false
	}
	if _, ok := specByName[cmdutil.Morph(bare)]; ok {
		return StickyFlagPair{}, false
	}

	for index := len(bare) - 1; index >= 1; index-- {
		prefix := bare[:index]
		matchedFlag := ""
		if _, ok := specByName[prefix]; ok {
			matchedFlag = prefix
		} else if normalized := cmdutil.Morph(prefix); normalized != "" {
			if _, ok := specByName[normalized]; ok {
				matchedFlag = normalized
			}
		}
		if matchedFlag == "" {
			continue
		}

		suffix := bare[index:]
		spec := specByName[matchedFlag]
		if !cmdutil.SuffixLooksLikeValue(suffix, spec.Type, spec.Format, spec.Enum) {
			return StickyFlagPair{}, false
		}
		inline := false
		if spec.Type == "bool" || spec.Type == "boolean" {
			suffix, _ = cmdutil.NormalizeBoolLiteral(suffix)
			inline = true
		}
		return StickyFlagPair{Flag: "--" + matchedFlag, Value: suffix, Inline: inline}, true
	}
	return StickyFlagPair{}, false
}

// FuzzyMatchFlag returns the unique closest real long flag within the
// conservative edit-distance threshold used by ParamNameHandler.
func FuzzyMatchFlag(argument string, known map[string]bool, candidates []string) (string, bool) {
	if !strings.HasPrefix(argument, "--") {
		return "", false
	}

	bare := argument[2:]
	if bare == "" {
		return "", false
	}

	var suffix string
	if index := strings.IndexByte(bare, '='); index >= 0 {
		suffix = bare[index:]
		bare = bare[:index]
	}
	if known[bare] {
		return "", false
	}

	threshold := maxFlagEditDistance
	if len(bare) <= 3 {
		threshold = 1
	}
	bestDistance := threshold + 1
	bestMatch := ""
	ambiguous := false
	for _, candidate := range candidates {
		distance := cmdutil.LevenshteinDist(bare, candidate)
		if distance < bestDistance {
			bestDistance = distance
			bestMatch = candidate
			ambiguous = false
		} else if distance == bestDistance && candidate != bestMatch {
			ambiguous = true
		}
	}
	if bestDistance > threshold || ambiguous || bestMatch == "" {
		return "", false
	}
	return "--" + bestMatch + suffix, true
}
