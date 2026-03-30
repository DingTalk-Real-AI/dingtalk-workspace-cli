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

// StickyHandler detects glued flag-value pairs in raw argv and splits
// them into separate tokens. For example, "--limit100" becomes
// "--limit", "100" when "limit" is a known flag name.
//
// The handler only operates on tokens that start with "--" and do not
// contain "=". It tries to match the longest known flag name prefix
// and, if the remaining suffix is non-empty, splits the token.
type StickyHandler struct{}

func (StickyHandler) Name() string          { return "sticky" }
func (StickyHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (StickyHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Args) == 0 || len(ctx.FlagSpecs) == 0 {
		return nil
	}

	known := buildFlagSet(ctx.FlagSpecs)
	result := make([]string, 0, len(ctx.Args))

	for _, arg := range ctx.Args {
		split, ok := trySplitSticky(arg, known)
		if ok {
			ctx.AddCorrection("sticky", pipeline.PreParse, split.flag, arg, split.flag+" "+split.value, "sticky")
			result = append(result, split.flag, split.value)
		} else {
			result = append(result, arg)
		}
	}

	ctx.Args = result
	return nil
}

type stickyPair struct {
	flag  string
	value string
}

// trySplitSticky checks if arg looks like a glued flag-value (e.g.
// "--limit100") and returns the split result. It requires:
//   - arg starts with "--"
//   - arg does not contain "=" (that is a valid Cobra syntax)
//   - the prefix matches a known flag name (directly or after
//     kebab-case normalisation)
//   - the remaining suffix is non-empty
//
// When multiple flag names match as prefixes, the longest one wins.
// The handler also tries kebab-case normalisation of each prefix so
// that camelCase+glued values like "--pageSize50" are correctly
// split to "--page-size", "50".
func trySplitSticky(arg string, known map[string]bool) (stickyPair, bool) {
	if !strings.HasPrefix(arg, "--") || strings.Contains(arg, "=") {
		return stickyPair{}, false
	}

	// Strip "--" prefix to work with the bare token.
	bare := arg[2:]
	if bare == "" {
		return stickyPair{}, false
	}

	// If the whole token is a known flag, it is not sticky — it is
	// a normal flag expecting a separate value token.
	if known[bare] || known[toKebabCase(bare)] {
		return stickyPair{}, false
	}

	// Try longest-prefix match: walk from len-1 down to 1, looking
	// for the longest known flag that is a prefix of bare. For each
	// candidate prefix, try both the raw form and kebab-case form.
	bestLen := 0
	bestFlag := ""
	for i := len(bare) - 1; i >= 1; i-- {
		prefix := bare[:i]

		matchedFlag := ""
		if known[prefix] {
			matchedFlag = prefix
		} else {
			kebab := toKebabCase(prefix)
			if kebab != "" && known[kebab] {
				matchedFlag = kebab
			}
		}

		if matchedFlag != "" && i > bestLen {
			bestLen = i
			bestFlag = matchedFlag
			break // longest first since we walk from the end
		}
	}
	if bestFlag == "" {
		return stickyPair{}, false
	}

	suffix := bare[bestLen:]
	if suffix == "" {
		return stickyPair{}, false
	}

	return stickyPair{
		flag:  "--" + bestFlag,
		value: suffix,
	}, true
}

// buildFlagSet creates a set of known flag names (without "--" prefix)
// from the context's FlagSpecs.
func buildFlagSet(specs []pipeline.FlagInfo) map[string]bool {
	m := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.Name != "" {
			m[spec.Name] = true
		}
	}
	return m
}
