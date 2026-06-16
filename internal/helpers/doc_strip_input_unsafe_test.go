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

package helpers

import "testing"

// TestStripDocInputUnsafe verifies that stripDocInputUnsafe removes exactly the
// characters the server-side RejectControlChars validator rejects (C0 controls
// except tab/newline, DEL, and the dangerous-Unicode set), while leaving all
// legitimate text untouched. Offending codepoints use explicit \u / \x escapes
// so they are unambiguous in source.
func TestStripDocInputUnsafe(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "preserves plain text",
			in:   "正常的文档内容 with ASCII",
			want: "正常的文档内容 with ASCII",
		},
		{
			name: "keeps tab and newline",
			in:   "标题\n\t正文",
			want: "标题\n\t正文",
		},
		{
			name: "drops C0 controls (null, SOH, CR) and DEL",
			in:   "正文\x00内容\x01段落\x0d结尾\x7f",
			want: "正文内容段落结尾",
		},
		{
			name: "drops zero-width space/non-joiner/joiner",
			in:   "正文\u200b内容\u200c段落\u200d结尾",
			want: "正文内容段落结尾",
		},
		{
			name: "drops bidi overrides and isolates",
			in:   "Bidi\u202a测试\u202e结束\u2066左\u2069右",
			want: "Bidi测试结束左右",
		},
		{
			name: "drops line and paragraph separators",
			in:   "\u2028\u2029行段",
			want: "行段",
		},
		{
			name: "drops BOM / ZWNBSP",
			in:   "BOM\ufeff尾",
			want: "BOM尾",
		},
		{
			name: "drops mixed control and dangerous unicode",
			in:   "混合\x00测试\u200b结尾\x7f",
			want: "混合测试结尾",
		},
		{
			name: "empty string stays empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripDocInputUnsafe(tc.in); got != tc.want {
				t.Fatalf("stripDocInputUnsafe(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
