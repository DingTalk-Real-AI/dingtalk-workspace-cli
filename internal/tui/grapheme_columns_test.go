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

package tui

import "testing"

func TestCrossPlatformCoverageGraphemeColumns(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       int
	}{
		{"family", "👨‍👩‍👧‍👦", 2}, {"flag", "🇨🇳", 2}, {"skin-tone", "👍🏽", 2},
		{"ascii", "abc", 3}, {"cjk", "钉钉", 4}, {"combining", "e\u0301", 1},
		{"ansi", "\x1b[34m🇨🇳\x1b[0m", 2}, {"unsupported-escape", "\x1b[2Jhidden", 0},
		{"controls", "\r\n\t", 3}, {"zero-width", "\u0301", 0}, {"empty", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlainRuneWidth(tc.text); got != tc.want {
				t.Fatalf("PlainRuneWidth(%q)=%d want%d", tc.text, got, tc.want)
			}
		})
	}
}
