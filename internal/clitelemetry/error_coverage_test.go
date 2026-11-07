// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package clitelemetry

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
)

func TestCrossPlatformCoverageErrorSummarySanitizeAndConfig(t *testing.T) {
	if ErrorSummary(nil) != "" {
		t.Fatal("nil error summary")
	}
	if got := ErrorSummary(&apperrors.PATError{RawJSON: "{}"}); got != "permission error" {
		t.Fatalf("pat = %q", got)
	}
	if got := ErrorSummary(coverageRawStderr("raw")); got != "raw stderr error" {
		t.Fatalf("raw stderr = %q", got)
	}
	if got := ErrorSummary(errors.New("unknown command \"foo\"")); got != "unknown command" {
		t.Fatalf("unknown command = %q", got)
	}
	if got := ErrorSummary(errors.New("unknown flag: --weird-flag")); got != "unknown flag: --weird-flag" {
		t.Fatalf("unknown flag = %q", got)
	}
	display, summary := PanicMessages("boom")
	if !strings.Contains(display, "boom") || summary != "internal panic" {
		t.Fatalf("panic messages = %q %q", display, summary)
	}
	if TruncateText("abc", 0) != "" {
		t.Fatal("zero max")
	}
	if TruncateText("ab", 5) != "ab" {
		t.Fatal("short text")
	}
	if got := TruncateText("abcd", 3); got != "abc" {
		t.Fatalf("tiny max = %q", got)
	}
	if got := TruncateText("abcdefghij", 7); !strings.HasSuffix(got, "...") {
		t.Fatalf("ellipsis = %q", got)
	}

	sanitized := SanitizeErrorText(`Bearer abcdef Bearer tok --token hunter2 authorization: secretval https://example.test/path {"k":1} 'quoted' "also" ` + "`tick`" + ` ~/secret/file C:\Windows\Temp ../rel/path user@example.com +1 415 555 1212 abcdefghijklmnop12`)
	for _, leak := range []string{"hunter2", "secretval", "example.test", "quoted", "Windows", "user@example.com"} {
		if strings.Contains(sanitized, leak) {
			t.Fatalf("sanitize leaked %q in %q", leak, sanitized)
		}
	}
	if !strings.Contains(SanitizeErrorText(`token `+strings.Repeat("a", 20)), strings.Repeat("a", 20)) {
		t.Fatal("letters-only opaque token should stay")
	}
	if got := ErrorSummary(errors.New(`failed --access-token=xyz https://h/a`)); strings.Contains(got, "xyz") || strings.Contains(got, "https") {
		t.Fatalf("sanitized summary leaked: %q", got)
	}
	escaped := SanitizeErrorText(`prefix "inner\"still" tail`)
	if !strings.Contains(escaped, "<redacted>") {
		t.Fatalf("escaped quote = %q", escaped)
	}
	_ = redactTelemetryQuotedText(`ok 'a\'b' "c\"d" ` + "`e`")

	if IdentityFromProfile(nil) != (Identity{}) {
		t.Fatal("nil profile identity")
	}
	if got := IdentityFromProfile(&profilemetadata.ProfileMetadata{UserID: " u ", UserName: " n ", CorpID: " c "}); got != (Identity{UserID: "u", UserName: "n", CorpID: "c"}) {
		t.Fatalf("trimmed identity = %#v", got)
	}
	if (RenderedError{}).Error() != "" {
		t.Fatal("rendered error text")
	}

	cmd, errText := "calendar event create", "failed"
	cfg := Configuration("1.0.0", Identity{UserID: "u", UserName: "n", CorpID: "corp"}, &cmd, &errText)
	fields := cfg.ExtraFields()
	if fields["c9"] != cmd || fields["c10"] != "corp" || fields["c5"] != "failed" {
		t.Fatalf("extra fields = %#v", fields)
	}
	empty := ""
	cfg = Configuration("1.0.0", Identity{}, &cmd, &empty)
	fields = cfg.ExtraFields()
	if _, ok := fields["c10"]; ok {
		t.Fatal("empty corp emitted")
	}
	if _, ok := fields["c5"]; ok {
		t.Fatal("empty error emitted")
	}
	if got := SanitizeErrorText("id ABCDEFGH12345678 leftover"); !strings.Contains(got, "<id>") {
		t.Fatalf("mixed token = %q", got)
	}
	Run(cfg, func() error { return nil }, func(error) int { return 0 })
}

type coverageRawStderr string

func (e coverageRawStderr) Error() string     { return string(e) }
func (e coverageRawStderr) RawStderr() string { return string(e) }
