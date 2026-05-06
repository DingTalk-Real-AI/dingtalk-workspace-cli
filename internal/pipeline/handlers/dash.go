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

// DashHandler corrects single-dash flags to double-dash when the bare
// token is longer than one character. This handles the common mistake
// where users or AI models write "-name" instead of "--name".
//
// POSIX combined short flags like "-vf" (equivalent to -v -f) are
// detected and left intact: each character of the bare token is checked
// against the registered shorthand flag set. If all characters are valid
// shorthands, the token is a combined short flag and must not be touched.
//
// The handler runs before AliasHandler in the PreParse phase so that
// the corrected "--xxx" token can undergo further normalisation
// (camelCase → kebab-case, fuzzy matching, etc.).
type DashHandler struct{}

func (DashHandler) Name() string          { return "dash" }
func (DashHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (DashHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Args) == 0 {
		return nil
	}

	shorthands := buildShorthandSet(ctx.FlagSpecs)
	result := make([]string, 0, len(ctx.Args))

	for _, arg := range ctx.Args {
		rewritten, ok := tryFixSingleDash(arg, shorthands)
		if ok {
			ctx.AddCorrection("dash", pipeline.PreParse, rewritten, arg, rewritten, "dash")
			result = append(result, rewritten)
		} else {
			result = append(result, arg)
		}
	}

	ctx.Args = result
	return nil
}

// buildShorthandSet collects all single-character short flag names from
// the flag specs into a set for O(1) lookup.
func buildShorthandSet(specs []pipeline.FlagInfo) map[rune]bool {
	s := make(map[rune]bool, len(specs))
	for _, spec := range specs {
		for _, r := range spec.Shorthand {
			s[r] = true
		}
	}
	return s
}

// isCombinedShortFlags returns true when every character of bare is a
// registered shorthand flag and the token is longer than one character.
func isCombinedShortFlags(bare string, shorthands map[rune]bool) bool {
	if len(bare) <= 1 {
		return false
	}
	for _, r := range bare {
		if !shorthands[r] {
			return false
		}
	}
	return true
}

// tryFixSingleDash checks whether arg is a single-dash token that should
// be converted to double-dash. It handles both bare flags and "-flag=value"
// syntax.
//
// Rules:
//   - Must start with exactly one "-" (not "--").
//   - Bare token must be longer than 1 character (protects -h, -v, -f, -y).
//   - Bare token must not start with a digit (protects -50, -3.14).
//   - Bare token must not be a POSIX combined short flag (-vf).
//   - Handles "-flag=value" syntax.
func tryFixSingleDash(arg string, shorthands map[rune]bool) (string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", false
	}
	if strings.HasPrefix(arg, "--") {
		return "", false
	}

	bare := arg[1:]

	var suffix string
	if idx := strings.IndexByte(bare, '='); idx >= 0 {
		suffix = bare[idx:]
		bare = bare[:idx]
	}

	if bare == "" {
		return "", false
	}

	// Single-character short flags (-h, -v, -f, -y) are legitimate.
	// Numeric bare tokens (-10, -3.14) are negative values, not flags.
	// POSIX combined short flags (-vf = -v -f) are left intact.
	if len(bare) <= 1 || (bare[0] >= '0' && bare[0] <= '9') {
		return "", false
	}
	if isCombinedShortFlags(bare, shorthands) {
		return "", false
	}

	return "--" + bare + suffix, true
}
