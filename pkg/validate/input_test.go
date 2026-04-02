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

func TestRejectControlChars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"clean ASCII", "hello world", false},
		{"tab allowed", "col1\tcol2", false},
		{"newline allowed", "line1\nline2", false},
		{"Chinese characters", "你好世界", false},
		{"null byte", "hello\x00world", true},
		{"bell character", "hello\x07world", true},
		{"escape character", "hello\x1bworld", true},
		{"carriage return", "hello\rworld", true},
		{"DEL character", "hello\x7fworld", true},
		{"zero-width space", "hello\u200Bworld", true},
		{"zero-width joiner", "hello\u200Dworld", true},
		{"BOM", "hello\uFEFFworld", true},
		{"Bidi LRO", "hello\u202Dworld", true},
		{"Bidi RLO", "hello\u202Eworld", true},
		{"Bidi LRI", "hello\u2066world", true},
		{"Bidi PDI", "hello\u2069world", true},
		{"line separator", "hello\u2028world", true},
		{"paragraph separator", "hello\u2029world", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RejectControlChars(tt.value, "--test")
			if (err != nil) != tt.wantErr {
				t.Errorf("RejectControlChars(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestRejectCRLF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"clean", "hello world", false},
		{"with CR", "hello\rworld", true},
		{"with LF", "hello\nworld", true},
		{"with CRLF", "hello\r\nworld", true},
		{"tab OK", "hello\tworld", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RejectCRLF(tt.value, "header")
			if (err != nil) != tt.wantErr {
				t.Errorf("RejectCRLF(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestStripQueryFragment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"/api/v1/users", "/api/v1/users"},
		{"/api/v1/users?evil=true", "/api/v1/users"},
		{"/api/v1/users#section", "/api/v1/users"},
		{"/api?a=1#b", "/api"},
		{"", ""},
		{"no-special-chars", "no-special-chars"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := StripQueryFragment(tt.input)
			if got != tt.want {
				t.Errorf("StripQueryFragment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDangerousUnicode(t *testing.T) {
	t.Parallel()

	dangerous := []rune{
		0x200B, 0x200C, 0x200D, // zero-width
		0xFEFF,         // BOM
		0x202A, 0x202E, // Bidi
		0x2028, 0x2029, // separators
		0x2066, 0x2069, // Bidi isolates
	}
	for _, r := range dangerous {
		if !isDangerousUnicode(r) {
			t.Errorf("isDangerousUnicode(%U) = false, want true", r)
		}
	}

	safe := []rune{'A', '中', '0', ' ', '\t', '\n', 0x2000, 0x206F}
	for _, r := range safe {
		if isDangerousUnicode(r) {
			t.Errorf("isDangerousUnicode(%U) = true, want false", r)
		}
	}
}
