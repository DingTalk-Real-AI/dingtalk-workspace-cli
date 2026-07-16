package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageThemePresentationHelpers(t *testing.T) {
	for name, render := range map[string]func(string) string{
		"brand": Brand, "blue": Blue, "cyan": Cyan, "white": White, "gray": Gray,
		"success": Success, "warning": Warning, "danger": Danger, "bold": Bold, "dim": Dim,
	} {
		if got := render("value"); !strings.Contains(got, "value") {
			t.Errorf("%s renderer = %q", name, got)
		}
	}
	if !strings.Contains(Header(" title ", " "), "title") || !strings.Contains(Header("title", " subtitle "), "subtitle") {
		t.Fatal("header rendering changed")
	}
	for _, got := range []string{Section("section"), Key("key"), Bullet(), Arrow(), Border(), SoftBorder()} {
		if got == "" {
			t.Fatal("presentation helper returned empty text")
		}
	}
	for _, kind := range []string{"ok", "success", "pass", "warn", "warning", "retryable", "error", "danger", "failed", "fail", "other"} {
		if StateMark(kind) == "" {
			t.Errorf("StateMark(%q) returned empty", kind)
		}
	}
	if PlainRuneWidth(Rule(0)) != MinPanelWidth || PlainRuneWidth(Rule(MaxPanelWidth+1)) != MaxPanelWidth {
		t.Fatal("rule width clamping changed")
	}
	if got := PadRightANSI("long", 2); got != "long" {
		t.Fatalf("negative padding = %q", got)
	}
	for _, r := range []rune{'\u200d', '\ufe00', '\U000e0100', '\u0301', '\u0488'} {
		if !isZeroWidthRune(r) {
			t.Errorf("%U should be zero width", r)
		}
	}
	if got := PlainRuneWidth("a\u200db"); got != 2 {
		t.Fatalf("zero-width rune count = %d", got)
	}
	for _, r := range []rune{'\u1100', '\u2329', '\u2e80', '\uac00', '\uf900', '\ufe10', '\ufe30', '\uff00', '\uffe0', '\U0001f1e6', '\U0001f300'} {
		if !isWideRune(r) {
			t.Errorf("%U should be wide", r)
		}
	}
}

func TestCrossPlatformCoveragePanelSuccessAndWriteFailures(t *testing.T) {
	var out bytes.Buffer
	if err := Panel(&out, "Title", []string{"line", strings.Repeat("wide", MaxPanelWidth)}); err != nil {
		t.Fatalf("Panel(): %v", err)
	}
	if !strings.Contains(out.String(), "Title") || !strings.Contains(out.String(), "line") {
		t.Fatalf("panel output = %q", out.String())
	}
	if err := Panel(&out, "", nil); err != nil {
		t.Fatalf("empty Panel(): %v", err)
	}
	for failAt := 1; failAt <= 5; failAt++ {
		writer := &failNthWriter{failAt: failAt}
		if err := Panel(writer, "Title", []string{"line"}); !errors.Is(err, errPanelWrite) {
			t.Errorf("Panel failAt=%d error = %v", failAt, err)
		}
	}
}

var errPanelWrite = errors.New("panel write failure")

type failNthWriter struct {
	writes int
	failAt int
}

func (w *failNthWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, errPanelWrite
	}
	return len(p), nil
}
