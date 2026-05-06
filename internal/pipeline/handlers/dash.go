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
// The corrected "--xxx" then either matches a known flag (normal parsing)
// or reaches Cobra as "unknown flag: --xxx", which triggers
// flagErrorWithSuggestions to display a friendly error with the command's
// Example text — far more readable than Cobra's default "unknown
// shorthand flag: 'n' in -name" message.
//
// Trade-off: POSIX combined short flags like "-vf" are also converted
// to "--vf", losing the multi-flag combination. In practice this is
// acceptable because:
//   - Combined short flags are rare in this project's CLI usage.
//   - The resulting "unknown flag: --vf" is still a clear error.
//   - Single-character short flags (-h, -v, -f, -y) are protected by
//     the len(bare) <= 1 check.
//   - Negative numbers (-50, -3.14) are protected by the digit check.
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

	result := make([]string, 0, len(ctx.Args))

	for _, arg := range ctx.Args {
		rewritten, ok := tryFixSingleDash(arg)
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

// tryFixSingleDash checks whether arg is a single-dash token that should
// be converted to double-dash. It handles both bare flags and "-flag=value"
// syntax.
//
// Rules:
//   - Must start with exactly one "-" (not "--").
//   - Bare token must be longer than 1 character (protects -h, -v, -f, -y).
//   - Bare token must not start with a digit (protects -50, -3.14).
//   - Handles "-flag=value" syntax.
func tryFixSingleDash(arg string) (string, bool) {
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
	if len(bare) <= 1 || (bare[0] >= '0' && bare[0] <= '9') {
		return "", false
	}

	return "--" + bare + suffix, true
}
