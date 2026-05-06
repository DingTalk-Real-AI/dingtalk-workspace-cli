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

// DashHandler corrects "-xxx" to "--xxx" when the user or AI omits a dash.
// POSIX combined short flags ("-vf") are detected via registered shorthand
// flags and left intact. Runs before AliasHandler in PreParse.
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

// buildShorthandSet collects registered shorthand chars from FlagSpecs.
func buildShorthandSet(specs []pipeline.FlagInfo) map[rune]bool {
	s := make(map[rune]bool, len(specs))
	for _, spec := range specs {
		for _, r := range spec.Shorthand {
			s[r] = true
		}
	}
	return s
}

// isCombinedShortFlags checks if bare is a POSIX combo like "-vf" where
// every character is a registered shorthand flag.
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

// tryFixSingleDash converts a single-dash token to double-dash.
// It skips single-character short flags, negative numbers, and
// POSIX combined short flags.
func tryFixSingleDash(arg string, shorthands map[rune]bool) (string, bool) {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return "", false
	}
	bare := arg[1:]
	var suffix string
	if idx := strings.IndexByte(bare, '='); idx >= 0 {
		suffix = bare[idx:]
		bare = bare[:idx]
	}
	if bare == "" || len(bare) <= 1 || (bare[0] >= '0' && bare[0] <= '9') {
		return "", false
	}
	if isCombinedShortFlags(bare, shorthands) {
		return "", false
	}
	return "--" + bare + suffix, true
}
