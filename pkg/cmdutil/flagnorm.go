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

package cmdutil

import "strings"

// Morph normalizes a flag spelling to a table-free canonical form so that
// morphological variants collapse to one key: case is folded, '_', '.' and
// spaces are treated as '-', and camelCase boundaries become '-'. It is the
// single shared primitive used both at build time (when the parameter-alias
// generator intersects concept members with a command's real flags) and at
// runtime (when pflag's SetNormalizeFunc resolves an emitted name). Keeping one
// implementation is a hard invariant: if the two diverged, a name the generator
// believed reducible could fail to resolve at runtime, which is contract drift.
//
// Morph is idempotent and stable on already-canonical names: Morph("limit") ==
// "limit" and Morph("page-size") == "page-size", so applying it to a command's
// real flags never changes their identity.
func Morph(name string) string {
	s := camelToKebab(name)
	s = strings.NewReplacer("_", "-", ".", "-", " ", "-").Replace(s)
	s = strings.ToLower(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// camelToKebab inserts a '-' at camelCase boundaries. A boundary is an
// uppercase rune that follows a lowercase letter or digit (fooBar -> foo-Bar),
// or an uppercase rune that starts a new word inside an acronym run
// (APIKey -> API-Key). Existing separators are left untouched; ToLower runs
// afterwards in Morph.
func camelToKebab(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes) + 4)
	for i, r := range runes {
		if i > 0 && isUpper(r) {
			prev := runes[i-1]
			switch {
			case isLower(prev) || isDigit(prev):
				b.WriteByte('-')
			case isUpper(prev) && i+1 < len(runes) && isLower(runes[i+1]):
				b.WriteByte('-')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }
