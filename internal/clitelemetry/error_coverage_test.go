package clitelemetry

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
)

func TestCrossPlatformCoverageErrorSummaryAndSanitize(t *testing.T) {
	if ErrorSummary(nil) != "" {
		t.Fatal("nil error")
	}
	if ErrorSummary(&apperrors.PATError{}) != "permission error" {
		t.Fatal("PATError")
	}
	if ErrorSummary(rawStderrStub{}) != "raw stderr error" {
		t.Fatal("RawStderrError")
	}
	if ErrorSummary(errors.New("unknown command foo")) != "unknown command" {
		t.Fatal("unknown command")
	}
	if got := ErrorSummary(errors.New("unknown flag: --weird-flag")); got != "unknown flag: --weird-flag" {
		t.Fatalf("unknown flag = %q", got)
	}
	got := ErrorSummary(errors.New(`bearer abcdef Authorization: secret --token xyz https://example.com {"a":1} '/tmp/x' C:\Windows\a ./rel/path user@ex.com +1 555 123 4567 abcdefghijklmnop12`))
	if got == "" || strings.Contains(got, "https://") || strings.Contains(got, "bearer ") {
		t.Fatalf("sanitize = %q", got)
	}
	display, summary := PanicMessages("boom")
	if display == "" || summary != "internal panic" {
		t.Fatalf("panic = %q %q", display, summary)
	}
	if TruncateText("abc", 0) != "" || TruncateText("abc", 3) != "abc" || TruncateText("abcd", 3) != "abc" {
		t.Fatal("truncate short")
	}
	if !strings.HasSuffix(TruncateText(strings.Repeat("x", 10), 6), "...") {
		t.Fatal("truncate ellipsis")
	}
	_ = redactTelemetryQuotedText(`ok 'a\'b' "c\"d" ` + "`e`")
}

func TestCrossPlatformCoverageConfigurationExtraFields(t *testing.T) {
	if IdentityFromProfile(nil) != (Identity{}) {
		t.Fatal("nil profile identity")
	}
	cmd := "schema list"
	errMsg := "boom"
	cfg := Configuration("1.0", Identity{UserID: "u", UserName: "n", CorpID: "c"}, &cmd, &errMsg)
	fields := cfg.ExtraFields()
	if fields["c9"] != cmd || fields["c10"] != "c" || fields["c5"] != errMsg {
		t.Fatalf("fields = %#v", fields)
	}
	empty := ""
	cfg = Configuration("1.0", Identity{}, &cmd, &empty)
	fields = cfg.ExtraFields()
	if _, ok := fields["c10"]; ok || fields["c5"] != "" && fields["c5"] == "x" {
		t.Fatalf("empty fields = %#v", fields)
	}
	_ = RenderedError{}.Error()
	_ = profilemetadata.ProfileMetadata{}
}

type rawStderrStub struct{}

func (rawStderrStub) Error() string     { return "raw" }
func (rawStderrStub) RawStderr() string { return "raw" }
