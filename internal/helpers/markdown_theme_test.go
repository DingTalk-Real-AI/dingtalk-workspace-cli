// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMarkdownThemeIDsAndStrictFlagValidation(t *testing.T) {
	want := []string{"default", "songyan", "taiying", "sujian", "juxia", "qingya"}
	if got := markdownThemeIDValues(); !reflect.DeepEqual(got, want) {
		t.Fatalf("theme IDs = %#v, want %#v", got, want)
	}

	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "theme"}
		cmd.Flags().String("theme", "", markdownThemeHelp())
		return cmd
	}

	omitted, err := markdownThemeFromCommand(newCommand())
	if err != nil || omitted.enabled || omitted.id != "" {
		t.Fatalf("omitted theme = %#v, %v", omitted, err)
	}
	for _, themeID := range want {
		t.Run(themeID, func(t *testing.T) {
			cmd := newCommand()
			if err := cmd.Flags().Set("theme", themeID); err != nil {
				t.Fatal(err)
			}
			got, err := markdownThemeFromCommand(cmd)
			if err != nil || !got.enabled || got.id != themeID {
				t.Fatalf("theme = %#v, %v", got, err)
			}
		})
	}
	for _, invalid := range []string{"", "Default", " qingya", "qingya ", "dark"} {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			cmd := newCommand()
			if err := cmd.Flags().Set("theme", invalid); err != nil {
				t.Fatal(err)
			}
			if _, err := markdownThemeFromCommand(cmd); err == nil || !strings.Contains(err.Error(), "不合法") {
				t.Fatalf("theme %q error = %v", invalid, err)
			}
		})
	}
}

func TestApplyMarkdownThemePreservesCompatibleFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		themeID string
		want    string
	}{
		{
			name:    "explicit default creates product envelope",
			source:  "# 标题\n正文",
			themeID: "default",
			want:    "---\nx-we-markdown-theme: default\n---\n# 标题\n正文",
		},
		{
			name:    "BOM CRLF metadata comments order and body stay byte exact",
			source:  "\ufeff---\r\ntitle: \"示例\"\r\n# keep\r\nx-we-markdown-theme: songyan\r\ntags: [a, b]\r\n---\r\n\r\n正文",
			themeID: "juxia",
			want:    "\ufeff---\r\ntitle: \"示例\"\r\n# keep\r\nx-we-markdown-theme: juxia\r\ntags: [a, b]\r\n---\r\n\r\n正文",
		},
		{
			name:    "insert before closing fence",
			source:  "---\ntheme: dark\ntitle: 示例\n---\n正文",
			themeID: "sujian",
			want:    "---\ntheme: dark\ntitle: 示例\nx-we-markdown-theme: sujian\n---\n正文",
		},
		{
			name:    "quoted duplicate fields normalize at first position",
			source:  "---\n'x-we-markdown-theme': qingya\ntitle: 示例\n\"x-we-markdown-theme\" : 'juxia' # old\n---\n正文",
			themeID: "taiying",
			want:    "---\nx-we-markdown-theme: taiying\ntitle: 示例\n---\n正文",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := applyMarkdownTheme([]byte(test.source), test.themeID)
			if string(got) != test.want {
				t.Fatalf("themed Markdown:\n got %q\nwant %q", string(got), test.want)
			}
			if again := applyMarkdownTheme(got, test.themeID); !bytes.Equal(again, got) {
				t.Fatalf("theme application is not idempotent:\nfirst  %q\nsecond %q", string(got), string(again))
			}
		})
	}
}

func TestApplyMarkdownThemeWrapsInvalidFrontmatterWithoutRepair(t *testing.T) {
	overLineLimit := "---\ntitle: example\n" + strings.Repeat("# keep\n", markdownFrontmatterMaxLines-1) + "---\nVISIBLE"
	overByteLimit := "---\ntitle: " + strings.Repeat("a", markdownFrontmatterMaxBytes) + "\n---\nVISIBLE"
	tests := []struct {
		name   string
		source string
	}{
		{name: "unclosed", source: "---\ntitle: 未闭合\n正文"},
		{name: "empty payload", source: "---\n---\n正文"},
		{name: "plain body between fences", source: "---\n普通正文\n---\n后文"},
		{name: "list only", source: "---\n- title: hidden\n---\nVISIBLE"},
		{name: "over 256 scanned lines", source: overLineLimit},
		{name: "over 64 KiB", source: overByteLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := "---\nx-we-markdown-theme: qingya\n---\n" + test.source
			got := string(applyMarkdownTheme([]byte(test.source), "qingya"))
			if got != want {
				t.Fatalf("invalid envelope was repaired instead of wrapped:\n got prefix %q\nwant prefix %q", prefixForFailure(got), prefixForFailure(want))
			}
		})
	}
}

func TestApplyMarkdownThemeHonorsBoundedEnvelopeBoundary(t *testing.T) {
	// Opening is not part of the budget. The payload's mapping line, comment
	// lines and closing fence together may occupy exactly 256 scanned lines.
	valid := "---\ntitle: example\n" + strings.Repeat("# keep\n", markdownFrontmatterMaxLines-2) + "---\nVISIBLE"
	if envelope := parseMarkdownFrontmatterEnvelope([]byte(valid)); envelope == nil {
		t.Fatal("frontmatter at the 256-line boundary was rejected")
	}
	want := strings.Replace(valid, "---\nVISIBLE", "x-we-markdown-theme: songyan\n---\nVISIBLE", 1)
	if got := string(applyMarkdownTheme([]byte(valid), "songyan")); got != want {
		t.Fatalf("boundary envelope update mismatch: got suffix %q want suffix %q", suffixForFailure(got), suffixForFailure(want))
	}
}

func TestApplyMarkdownThemeKeepsBOMOnIndependentEnvelope(t *testing.T) {
	source := "\ufeff---\r\n---\r\n正文"
	want := "\ufeff---\r\nx-we-markdown-theme: default\r\n---\r\n---\r\n---\r\n正文"
	if got := string(applyMarkdownTheme([]byte(source), "default")); got != want {
		t.Fatalf("BOM/CRLF wrapped content = %q, want %q", got, want)
	}
}

func TestMarkdownThemeWhitespaceMatchesECMAScriptBaseline(t *testing.T) {
	for _, test := range []struct {
		name string
		line string
		want bool
	}{
		{name: "BOM is ECMAScript whitespace at line start", line: "\ufefftitle: value", want: false},
		{name: "BOM is ECMAScript whitespace after colon", line: "title:\ufeffvalue", want: true},
		{name: "NEL is not ECMAScript whitespace at line start", line: "\u0085title: value", want: true},
		{name: "NEL is not ECMAScript whitespace after colon", line: "title:\u0085value", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := markdownLineHasTopLevelMapping([]byte(test.line)); got != test.want {
				t.Fatalf("top-level mapping classification = %v, want %v for %q", got, test.want, test.line)
			}
		})
	}

	if !isMarkdownThemeFieldLine([]byte(markdownThemeFrontmatterField + "\ufeff: qingya")) {
		t.Fatal("theme field did not accept ECMAScript whitespace before colon")
	}
	if isMarkdownThemeFieldLine([]byte(markdownThemeFrontmatterField + "\u0085: qingya")) {
		t.Fatal("theme field incorrectly accepted U+0085 before colon")
	}
}

func prefixForFailure(value string) string {
	if len(value) <= 160 {
		return value
	}
	return fmt.Sprintf("%s... (%d bytes)", value[:160], len(value))
}

func suffixForFailure(value string) string {
	if len(value) <= 160 {
		return value
	}
	return fmt.Sprintf("...%s (%d bytes)", value[len(value)-160:], len(value))
}

func TestMarkdownThemeRejectsWrongFlagType(t *testing.T) {
	cmd := &cobra.Command{Use: "theme"}
	cmd.Flags().Bool("theme", false, "wrong declaration")
	if err := cmd.Flags().Set("theme", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := markdownThemeFromCommand(cmd); err == nil || !strings.Contains(err.Error(), "读取 --theme 失败") {
		t.Fatalf("wrong flag type error = %v", err)
	}
}

func TestMarkdownBoundedLineRejectsInvalidBounds(t *testing.T) {
	for _, test := range []struct{ start, budget int }{{0, 0}, {0, -1}, {-1, 3}, {4, 3}} {
		if _, ok := readMarkdownBoundedLine([]byte("abc"), test.start, test.budget); ok {
			t.Fatalf("invalid bounds accepted: %#v", test)
		}
	}
	if markdownRuneIsSpace(nil) {
		t.Fatal("empty input is not whitespace")
	}
	line, ok := readMarkdownBoundedLine([]byte("abc"), 0, 3)
	if !ok || string(line.content) != "abc" || line.end != 3 || len(line.ending) != 0 {
		t.Fatalf("unterminated final line = %#v, %v", line, ok)
	}
}

func TestMarkdownThemeFieldNormalizesUnterminatedPayload(t *testing.T) {
	for _, test := range []struct{ name, payload, want string }{
		{"insert", "title: example", "title: example\nx-we-markdown-theme: qingya\n"},
		{"replace", "x-we-markdown-theme: default", "x-we-markdown-theme: qingya\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := string(updateMarkdownThemeField([]byte(test.payload), []byte("\n"), "qingya")); got != test.want {
				t.Fatalf("normalized payload = %q, want %q", got, test.want)
			}
		})
	}
}
