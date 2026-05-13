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

import "testing"

// TestSuffixLooksLikeValue_UTF8FirstRune locks in the UTF-8 first-rune
// reading contract on SuffixLooksLikeValue. The function previously read
// suffix[0] (a single byte) which produced incorrect splits / matches for
// any multi-byte first rune — relevant because dws is a Chinese-language
// CLI and value text often starts with CJK characters.
func TestSuffixLooksLikeValue_UTF8FirstRune(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		suffix string
		typ    string
		format string
		enum   []string
		want   bool
	}{
		{
			// --name张三 must NOT be split: with no metadata the fallback
			// branch should treat a CJK letter as "not a value-looking
			// suffix" so cobra reports unknown flag instead of cutting
			// the user's typo into --name + 张三.
			name:   "plain string + CJK letter suffix refuses split",
			suffix: "张三",
			typ:    "string",
			want:   false,
		},
		{
			// CJK leading rune is fine as long as the email-format guard
			// ('@' anywhere in suffix) still passes.
			name:   "email format + CJK leading + @ allows split",
			suffix: "张三@example.com",
			typ:    "string",
			format: "email",
			want:   true,
		},
		{
			// uuid format: first rune must be hex. CJK starting char is
			// not hex so the suffix must be rejected.
			name:   "uuid format + CJK leading rejects split",
			suffix: "张abcd",
			typ:    "string",
			format: "uuid",
			want:   false,
		},
		{
			// Baseline: digit-led suffix on int still splits — guards
			// against accidental over-tightening of the fallback path.
			name:   "int + digit-led baseline still splits",
			suffix: "100",
			typ:    "int",
			want:   true,
		},
		{
			// Invalid UTF-8 (lone continuation byte 0x80) decodes as
			// utf8.RuneError; the RuneError guard makes the fallback
			// reject it instead of treating it as "non-letter, splittable".
			name:   "plain string + invalid UTF-8 leading byte refuses split",
			suffix: "\x80abc",
			typ:    "string",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SuffixLooksLikeValue(tc.suffix, tc.typ, tc.format, tc.enum)
			if got != tc.want {
				t.Errorf("SuffixLooksLikeValue(%q, %q, %q, %v) = %v, want %v",
					tc.suffix, tc.typ, tc.format, tc.enum, got, tc.want)
			}
		})
	}
}
