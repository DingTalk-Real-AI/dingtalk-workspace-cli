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

package validate

import (
	"testing"
)

func TestSanitizeForTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean text", "hello world", "hello world"},
		{"preserves newline", "line1\nline2", "line1\nline2"},
		{"preserves tab", "col1\tcol2", "col1\tcol2"},
		{"strips null byte", "hello\x00world", "helloworld"},
		{"strips bell", "hello\x07world", "helloworld"},
		{"strips CR", "hello\rworld", "helloworld"},
		{"strips DEL", "hello\x7fworld", "helloworld"},
		{
			"strips ANSI color",
			"hello \x1b[31mred\x1b[0m world",
			"hello red world",
		},
		{
			"strips ANSI cursor move",
			"\x1b[2Jhello",
			"hello",
		},
		{
			"strips ANSI private CSI",
			"\x1b[?25lhidden cursor",
			"hidden cursor",
		},
		{
			"strips OSC title change",
			"\x1b]0;evil title\x07hello",
			"hello",
		},
		{
			"strips zero-width space",
			"admin\u200Buser",
			"adminuser",
		},
		{
			"strips Bidi override",
			"file\u202Etxt.exe",
			"filetxt.exe",
		},
		{
			"strips BOM",
			"\uFEFFhello",
			"hello",
		},
		{"empty string", "", ""},
		{"Chinese text preserved", "你好世界", "你好世界"},
		{
			"mixed attack",
			"\x1b[31m\x00evil\u200B\x07payload\x1b[0m",
			"evilpayload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeForTerminal(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeForTerminal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
