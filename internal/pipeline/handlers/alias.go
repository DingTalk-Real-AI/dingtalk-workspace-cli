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
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// AliasHandler normalises flag names in raw argv so that common
// model-generated variants resolve to the canonical kebab-case name.
//
// Supported normalisations:
//   - camelCase  → kebab-case  (--userId     → --user-id)
//   - snake_case → kebab-case  (--user_name  → --user-name)
//   - UPPER-CASE → lower-case  (--USER-ID    → --user-id)
//   - PascalCase → kebab-case  (--UserName   → --user-name)
//
// The handler only rewrites tokens that start with "--" and whose
// normalised form matches a known flag name. Unknown flags are left
// untouched so that Cobra can report them as errors.
type AliasHandler struct{}

func (AliasHandler) Name() string          { return "alias" }
func (AliasHandler) Phase() pipeline.Phase { return pipeline.PreParse }

func (AliasHandler) Handle(ctx *pipeline.Context) error {
	if len(ctx.Args) == 0 || len(ctx.FlagSpecs) == 0 {
		return nil
	}

	known := buildFlagSet(ctx.FlagSpecs)
	result := make([]string, 0, len(ctx.Args))

	for i, arg := range ctx.Args {
		rewritten, ok := tryNormaliseFlag(arg, known)
		if ok {
			ctx.AddCorrection("alias", pipeline.PreParse, rewritten, arg, rewritten, "alias")
			result = append(result, rewritten)
		} else {
			result = append(result, ctx.Args[i])
		}
	}

	ctx.Args = result
	return nil
}

// tryNormaliseFlag checks whether arg is a "--flag" token that can be
// normalised to a known flag name. It handles both bare flags and
// "--flag=value" syntax.
func tryNormaliseFlag(arg string, known map[string]bool) (string, bool) {
	if !strings.HasPrefix(arg, "--") {
		return "", false
	}

	bare := arg[2:]
	if bare == "" {
		return "", false
	}

	// Handle --flag=value syntax: split, normalise the key, reassemble.
	var suffix string
	if idx := strings.IndexByte(bare, '='); idx >= 0 {
		suffix = bare[idx:] // includes "="
		bare = bare[:idx]
	}

	// Already a known flag in its current form — no change needed.
	if known[bare] {
		return "", false
	}

	normalised := toKebabCase(bare)
	if normalised == bare {
		return "", false
	}
	if !known[normalised] {
		return "", false
	}

	return "--" + normalised + suffix, true
}

// toKebabCase converts a string from camelCase, PascalCase, or
// snake_case to kebab-case. Examples:
//
//	"userId"    → "user-id"
//	"UserName"  → "user-name"
//	"user_name" → "user-name"
//	"USER_ID"   → "user-id"
//	"pageSize"  → "page-size"
func toKebabCase(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s) + 4) // small extra for hyphens

	runes := []rune(s)
	for i, r := range runes {
		if r == '_' || r == ' ' {
			if b.Len() > 0 {
				b.WriteByte('-')
			}
			continue
		}

		if unicode.IsUpper(r) {
			// Insert hyphen before an uppercase letter when:
			// 1. Not at start, AND
			// 2. Previous char was lowercase, OR
			// 3. Next char is lowercase (handles "userID" → "user-id"
			//    at the boundary between "I" and "D" in "ID" we don't
			//    split, but "IDs" → we split before "s" which is
			//    handled by the lowercase check at the next iteration).
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) {
					b.WriteByte('-')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					b.WriteByte('-')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}

	return strings.Trim(b.String(), "-")
}
