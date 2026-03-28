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

package errors

import "testing"

func TestResourceName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "search_open_platform_docs"},
		{name: "valid-cjk", input: "审批查询"},
		{name: "leading-digit", input: "1tool", wantErr: true},
		{name: "shell-char", input: "tool;rm", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ResourceName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("ResourceName(%q) error = nil, want failure", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ResourceName(%q) error = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "relative", input: "skills/generated"},
		{name: "absolute", input: "/tmp/dws/export.json"},
		{name: "traversal", input: "../secret", wantErr: true},
		{name: "shell", input: "out;rm -rf /", wantErr: true},
		{name: "null-byte", input: "bad\x00path", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := SafePath(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("SafePath(%q) error = nil, want failure", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("SafePath(%q) error = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestSafeLocalFlagPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		flagName string
		value    string
		want     string
		wantErr  bool
	}{
		// URL pass-through
		{name: "http-url", flagName: "--source", value: "http://example.com/file", want: "http://example.com/file"},
		{name: "https-url", flagName: "--source", value: "https://api.example.com/data", want: "https://api.example.com/data"},
		// Empty pass-through
		{name: "empty", flagName: "--file", value: "", want: ""},
		// Valid relative paths (returns original relative path)
		{name: "relative-file", flagName: "--file", value: "data.json", want: "data.json"},
		{name: "relative-nested", flagName: "--file", value: "dir/file.txt", want: "dir/file.txt"},
		// Invalid paths
		{name: "absolute", flagName: "--file", value: "/etc/passwd", wantErr: true},
		{name: "traversal", flagName: "--file", value: "../secret", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SafeLocalFlagPath(tc.flagName, tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SafeLocalFlagPath(%q, %q) error = nil, want failure", tc.flagName, tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("SafeLocalFlagPath(%q, %q) error = %v, want nil", tc.flagName, tc.value, err)
			}
			if got != tc.want {
				t.Fatalf("SafeLocalFlagPath(%q, %q) = %q, want %q", tc.flagName, tc.value, got, tc.want)
			}
		})
	}
}

func TestRejectCRLF(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "clean", value: "normal text"},
		{name: "with-space", value: "text with spaces"},
		{name: "with-CR", value: "text\rwith CR", wantErr: true},
		{name: "with-LF", value: "text\nwith LF", wantErr: true},
		{name: "with-CRLF", value: "text\r\nwith CRLF", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RejectCRLF(tc.value, "--header")
			if tc.wantErr && err == nil {
				t.Fatalf("RejectCRLF(%q) error = nil, want failure", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("RejectCRLF(%q) error = %v, want nil", tc.value, err)
			}
		})
	}
}

func TestStripQueryFragment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "no-query", input: "/api/v1/users", want: "/api/v1/users"},
		{name: "with-query", input: "/api/v1/users?page=1", want: "/api/v1/users"},
		{name: "with-fragment", input: "/docs#section", want: "/docs"},
		{name: "query-and-fragment", input: "/api?a=1#sec", want: "/api"},
		{name: "fragment-before-query", input: "/path#frag?notquery", want: "/path"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StripQueryFragment(tc.input)
			if got != tc.want {
				t.Fatalf("StripQueryFragment(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRejectControlChars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// ── Normal text: allowed ──
		{name: "plain-text", input: "hello world"},
		{name: "with-tab", input: "hello\tworld"},
		{name: "with-newline", input: "hello\nworld"},
		{name: "unicode-text", input: "你好世界"},
		{name: "mixed-unicode", input: "Hello 世界 123"},
		{name: "emoji", input: "test 😀 emoji"},

		// ── C0 control characters (except tab/newline): rejected ──
		{name: "null-byte", input: "bad\x00path", wantErr: true},
		{name: "bell", input: "alert\x07here", wantErr: true},
		{name: "backspace", input: "back\x08space", wantErr: true},
		{name: "form-feed", input: "form\x0cfeed", wantErr: true},
		{name: "carriage-return", input: "cr\rhere", wantErr: true},
		{name: "escape", input: "esc\x1bhere", wantErr: true},
		{name: "delete", input: "del\x7fete", wantErr: true},

		// ── Dangerous Unicode: rejected ──
		{name: "zero-width-space", input: "foo\u200Bbar", wantErr: true},
		{name: "zero-width-non-joiner", input: "foo\u200Cbar", wantErr: true},
		{name: "zero-width-joiner", input: "foo\u200Dbar", wantErr: true},
		{name: "bom", input: "\uFEFFstart", wantErr: true},
		{name: "bidi-lre", input: "foo\u202Abar", wantErr: true},
		{name: "bidi-rle", input: "foo\u202Bbar", wantErr: true},
		{name: "bidi-pdf", input: "foo\u202Cbar", wantErr: true},
		{name: "bidi-lro", input: "foo\u202Dbar", wantErr: true},
		{name: "bidi-rlo", input: "foo\u202Ebar", wantErr: true},
		{name: "line-separator", input: "foo\u2028bar", wantErr: true},
		{name: "paragraph-separator", input: "foo\u2029bar", wantErr: true},
		{name: "bidi-lri", input: "foo\u2066bar", wantErr: true},
		{name: "bidi-rli", input: "foo\u2067bar", wantErr: true},
		{name: "bidi-fsi", input: "foo\u2068bar", wantErr: true},
		{name: "bidi-pdi", input: "foo\u2069bar", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RejectControlChars(tc.input, "--test")
			if tc.wantErr && err == nil {
				t.Fatalf("RejectControlChars(%q) error = nil, want failure", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("RejectControlChars(%q) error = %v, want nil", tc.input, err)
			}
		})
	}
}
