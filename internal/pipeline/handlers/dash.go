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
// Safety: all short flags in the project are single-character (-h, -v,
// -f, -y), and all business parameters use --xxx long flags. Correcting
// "-xxx" (length > 1) to "--xxx" is therefore always safe: either the
// resulting "--xxx" is a known flag and parsing proceeds normally, or it
// is still unknown and Cobra will report it as such.
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
	known := buildFlagSet(ctx.FlagSpecs)

	for _, arg := range ctx.Args {
		rewritten, ok := tryFixSingleDash(arg, known)
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
// be converted to double-dash. It only converts when the resulting "--xxx"
// is a known long flag, which prevents breaking POSIX combined short
// flags like "-vf" (which Cobra natively parses as -v -f).
//
// Rules:
//   - Must start with exactly one "-" (not "--").
//   - Bare token must be longer than 1 character (protects -h, -v, etc.).
//   - Bare token must not start with a digit (protects -50, -3.14).
//   - The kebab-case form of bare must be a known flag name.
//   - Handles "-flag=value" syntax.
func tryFixSingleDash(arg string, known map[string]bool) (string, bool) {
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

	// Single-character short flags (-h, -v, -f, -y) are legitimate; do not touch.
	// Numeric bare tokens (-10, -3.14) are negative values, not flags.
	if len(bare) <= 1 || (bare[0] >= '0' && bare[0] <= '9') {
		return "", false
	}

	// Only convert if the kebab-case form is a known flag.
	// This prevents breaking POSIX combined short flags (-vf → should
	// stay as Cobra-native combined short flags, not become --vf).
	candidate := toKebabCase(bare)
	if !known[candidate] && !known[bare] {
		return "", false
	}

	flagName := candidate
	if known[bare] {
		flagName = bare
	}

	return "--" + flagName + suffix, true
}
