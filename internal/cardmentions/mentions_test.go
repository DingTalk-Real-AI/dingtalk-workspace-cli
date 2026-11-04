// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package cardmentions

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func markdownMessage(t *testing.T, content string) string {
	t.Helper()
	return fmt.Sprintf(`{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"m","component":"Markdown","content":%s}]}}`,
		fmt.Sprintf("%q", content))
}

func TestCrossPlatformCoverageExtractMentionsMarkdownLiterals(t *testing.T) {
	messages := []string{
		markdownMessage(t, `hi <a atId="DAAA1">nick</a> and <a atId="all">everyone</a> <a atId="DAAA1">again</a>`),
		markdownMessage(t, `second <a atId="DAAA2">other</a> <@123456>`),
	}
	usage := ExtractMentions(messages)
	if got, want := usage.AtIDs(), []string{"DAAA1", "DAAA2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AtIDs() = %#v, want %#v", got, want)
	}
	if !usage.HasAtAll() {
		t.Fatal("HasAtAll() = false, want true")
	}
	if len(usage.AtTags) != 4 {
		t.Fatalf("len(AtTags) = %d, want 4", len(usage.AtTags))
	}
	if usage.AtTags[0].Pointer != "/0/updateComponents/components/0/content" {
		t.Fatalf("pointer = %q", usage.AtTags[0].Pointer)
	}
	if len(usage.IMPlaceholders) != 1 || usage.IMPlaceholders[0].Token != "<@123456>" {
		t.Fatalf("IMPlaceholders = %#v", usage.IMPlaceholders)
	}
}

func TestExtractMentionsSkipsNonLiteralAndNonMarkdown(t *testing.T) {
	messages := []string{
		`{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"t","component":"Text","text":"<a atId=\"DAAA1\">x</a>"}]}}`,
		`{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"m","component":"Markdown","content":{"binding":"/content"}}]}}`,
		`not-json`,
	}
	usage := ExtractMentions(messages)
	if len(usage.AtTags) != 0 || len(usage.IMPlaceholders) != 0 {
		t.Fatalf("usage = %#v, want empty", usage)
	}
	if usage.HasAtAll() {
		t.Fatal("HasAtAll() = true, want false")
	}
}

func TestCrossPlatformCoverageValidateMentionConsistency(t *testing.T) {
	for _, tc := range []struct {
		name     string
		messages []string
		declared []string
		atAll    bool
		wantErr  string
	}{
		{
			name:     "no mentions at all is fine",
			messages: []string{markdownMessage(t, "plain")},
			wantErr:  "",
		},
		{
			name:     "declared and used",
			messages: []string{markdownMessage(t, `hi <a atId="DAAA1">nick</a>`)},
			declared: []string{"DAAA1"},
			wantErr:  "",
		},
		{
			name:     "declared but unused",
			messages: []string{markdownMessage(t, "plain")},
			declared: []string{"DAAA1"},
			wantErr:  "未使用: DAAA1",
		},
		{
			name:     "used but undeclared",
			messages: []string{markdownMessage(t, `hi <a atId="DAAA1">nick</a>`)},
			wantErr:  "未声明: DAAA1",
		},
		{
			name:     "at-all with all tag",
			messages: []string{markdownMessage(t, `<a atId="all">everyone</a>`)},
			atAll:    true,
			wantErr:  "",
		},
		{
			name:     "at-all without all tag",
			messages: []string{markdownMessage(t, "plain")},
			atAll:    true,
			wantErr:  "--at-all 需要卡片 Markdown 中同时包含 <a atId=\"all\">everyone</a>",
		},
		{
			name:     "all tag without at-all",
			messages: []string{markdownMessage(t, `<a atId="all">everyone</a>`)},
			wantErr:  "卡片 Markdown 中的 <a atId=\"all\"> 必须同时指定 --at-all",
		},
		{
			name:     "all never needs declaration alongside member",
			messages: []string{markdownMessage(t, `<a atId="DAAA1">n</a> <a atId="all">everyone</a>`)},
			declared: []string{"DAAA1"},
			atAll:    true,
			wantErr:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMentionConsistency(tc.messages, tc.declared, tc.atAll)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageMentionConsistencyMalformedJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared []string
		atAll    bool
	}{
		{"member", []string{"DAAA1"}, false},
		{"everyone", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMentionConsistency([]string{"not-json"}, tc.declared, tc.atAll)
			if err == nil || !strings.Contains(err.Error(), "消息[0] JSON 无效") {
				t.Fatalf("error = %v, want malformed message diagnostic", err)
			}
		})
	}
	if err := ValidateMentionConsistency([]string{"not-json"}, nil, false); err != nil {
		t.Fatalf("without explicit mentions, preserve existing parsing behavior: %v", err)
	}
}

func TestMentionDeclarationDiagnostics(t *testing.T) {
	t.Run("consistent produces nothing", func(t *testing.T) {
		got := MentionDeclarationDiagnostics(
			[]string{markdownMessage(t, `hi <a atId="DAAA1">nick</a> <a atId="all">everyone</a>`)},
			[]string{"DAAA1"},
		)
		if len(got) != 0 {
			t.Fatalf("diagnostics = %#v, want empty", got)
		}
	})

	t.Run("undeclared occurrences reported per tag with pointer", func(t *testing.T) {
		got := MentionDeclarationDiagnostics(
			[]string{markdownMessage(t, `<a atId="DAAA1">a</a> <a atId="DAAA1">b</a>`)},
			nil,
		)
		if len(got) != 2 {
			t.Fatalf("len(diagnostics) = %d, want 2", len(got))
		}
		for _, d := range got {
			if d.Code != "mention.undeclared" || d.Severity != "error" || d.Pointer == "" {
				t.Fatalf("diagnostic = %#v", d)
			}
			if !strings.Contains(d.Message, `"DAAA1"`) {
				t.Fatalf("message = %q", d.Message)
			}
		}
	})

	t.Run("declared unused reported once", func(t *testing.T) {
		got := MentionDeclarationDiagnostics(
			[]string{markdownMessage(t, "plain")},
			[]string{"DAAA1", "DAAA2"},
		)
		if len(got) != 2 {
			t.Fatalf("len(diagnostics) = %d, want 2", len(got))
		}
		if got[0].Code != "mention.declared_unused" || !strings.Contains(got[0].Message, `"DAAA1"`) {
			t.Fatalf("diagnostic = %#v", got[0])
		}
	})
}

func TestMentionIMPlaceholderDiagnostics(t *testing.T) {
	if got := MentionIMPlaceholderDiagnostics("plain text", "/0/content"); got != nil {
		t.Fatalf("diagnostics = %#v, want nil", got)
	}
	got := MentionIMPlaceholderDiagnostics(`<@123456> and <@abcdef>`, "/0/content")
	if len(got) != 1 {
		t.Fatalf("len(diagnostics) = %d, want 1", len(got))
	}
	d := got[0]
	if d.Code != "mention.im_placeholder" || d.Severity != "error" || d.Pointer != "/0/content" {
		t.Fatalf("diagnostic = %#v", d)
	}
	if !strings.Contains(d.Message, "<@123456>") || !strings.Contains(d.Message, "<@abcdef>") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestCrossPlatformCoverageRewriteMentionAllTags(t *testing.T) {
	t.Run("markdown literal all tags become the wire alias", func(t *testing.T) {
		memberAndAll := markdownMessage(t, `hi <a atId="DAAA1">nick</a> and <a atId="all">everyone</a> <a atId = "all" >again</a>`)
		out := RewriteMentionAllTags([]string{memberAndAll})
		if len(out) != 1 {
			t.Fatalf("out = %#v", out)
		}
		usage := ExtractMentions(out)
		if usage.HasAtAll() {
			t.Fatalf("all tag survived rewrite: %s", out[0])
		}
		if got := usage.AtIDs(); !reflect.DeepEqual(got, []string{"DAAA1", "10"}) {
			t.Fatalf("AtIDs after rewrite = %#v", got)
		}
		if strings.Count(out[0], `<a atId=\"10\">`) != 2 {
			t.Fatalf("wire alias count wrong: %s", out[0])
		}
		if !strings.Contains(out[0], `<a atId=\"DAAA1\">nick</a>`) {
			t.Fatalf("member tag corrupted: %s", out[0])
		}
	})

	t.Run("non-markdown and bound content stay untouched", func(t *testing.T) {
		text := `{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"t","component":"Text","text":"<a atId=\"all\">x</a>"},{"id":"m","component":"Markdown","content":{"binding":"/c"}}]}}`
		out := RewriteMentionAllTags([]string{text})
		if out[0] != text {
			t.Fatalf("non-markdown/bound content rewritten: %s", out[0])
		}
	})

	t.Run("messages without all tags keep their bytes", func(t *testing.T) {
		plain := markdownMessage(t, "plain")
		out := RewriteMentionAllTags([]string{plain, "not-json"})
		if out[0] != plain || out[1] != "not-json" {
			t.Fatalf("out = %#v", out)
		}
	})

	t.Run("rewrite is idempotent", func(t *testing.T) {
		msg := markdownMessage(t, `<a atId="DAAA1">nick</a> <a atId="all">everyone</a>`)
		once := RewriteMentionAllTags([]string{msg})
		twice := RewriteMentionAllTags(once)
		if !reflect.DeepEqual(once, twice) {
			t.Fatalf("second rewrite changed bytes:\n%s\n%s", once[0], twice[0])
		}
	})
}
