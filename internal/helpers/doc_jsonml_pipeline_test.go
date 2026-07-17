package helpers

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func jsonMLTestCommand(t *testing.T, fix bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "jsonml-test"}
	addJsonMLFlags(cmd)
	addJsonMLFlags(cmd)
	if err := cmd.Flags().Set("fix-jsonml", map[bool]string{true: "true", false: "false"}[fix]); err != nil {
		t.Fatalf("set fix-jsonml: %v", err)
	}
	return cmd
}

func TestCrossPlatformCoverageJSONMLInputSanitizingAndShapeCoercion(t *testing.T) {
	for _, r := range []rune{0x200B, 0x200C, 0x200D, 0xFEFF, 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2028, 0x2029, 0x2066, 0x2067, 0x2068, 0x2069} {
		if !isInputDangerousUnicode(r) {
			t.Errorf("rune %U should be dangerous", r)
		}
	}
	for _, r := range []rune{'a', '\t', '\n', 0x2065, 0x206A} {
		if isInputDangerousUnicode(r) {
			t.Errorf("rune %U should be safe", r)
		}
	}
	if got := stripInputUnsafeChars("a\x00\t\n\x7fb\u200bc"); got != "a\t\nbc" {
		t.Fatalf("stripInputUnsafeChars() = %q", got)
	}

	bodyCases := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"{", "{"},
		{`{"jsonml":[]}`, `{"jsonml":[]}`},
		{`"text"`, `"text"`},
	}
	for _, tc := range bodyCases {
		got, notes, err := coerceJsonMLBodyShape(tc.raw)
		if err != nil || got != tc.want || len(notes) != 0 {
			t.Errorf("coerceJsonMLBodyShape(%q) = %q, %v, %v", tc.raw, got, notes, err)
		}
	}
	if got, _, err := coerceJsonMLBodyShape(`["root",{}]`); err != nil || got != `{"jsonml":["root",{}]}` {
		t.Fatalf("bare body coercion = %q, %v", got, err)
	}

	nodePassthrough := []string{"", "{", `["p",{}]`, `"text"`, `{}`, `{"other":[]}`, `{"jsonml":"bad"}`}
	for _, raw := range nodePassthrough {
		got, notes, err := coerceJsonMLNodeShape(raw)
		if err != nil || got != raw || len(notes) != 0 {
			t.Errorf("coerceJsonMLNodeShape(%q) = %q, %v, %v", raw, got, notes, err)
		}
	}
	if _, _, err := coerceJsonMLNodeShape(`{"jsonml":[]}`); err == nil {
		t.Fatal("empty wrapper should fail")
	}
	if _, _, err := coerceJsonMLNodeShape(`{"jsonml":[["p",{}],["p",{}]]}`); err == nil {
		t.Fatal("multi-node wrapper should fail")
	}
	got, notes, err := coerceJsonMLNodeShape(`{"jsonml":[["p",{}]]}`)
	if err != nil || got != `["p",{}]` || len(notes) != 1 {
		t.Fatalf("single-node wrapper = %q, %v, %v", got, notes, err)
	}
	emitFixNotes(nil)
	emitFixNotes(notes)
}

func TestCrossPlatformCoveragePrepareJSONMLBody(t *testing.T) {
	strict := jsonMLTestCommand(t, false)
	valid := `{"jsonml":["root",{},["p",{},["span",{},"ok"]]]}`
	if got, err := prepareJsonMLBody(strict, valid); err != nil || !strings.Contains(got, `"root"`) {
		t.Fatalf("valid wrapper = %q, %v", got, err)
	}
	if got, err := prepareJsonMLBody(strict, `["root",{},["p",{},["span",{},"ok"]]]`); err != nil || !strings.Contains(got, `"root"`) {
		t.Fatalf("valid bare body = %q, %v", got, err)
	}
	for _, raw := range []string{
		"{",
		`{}`,
		`{"jsonml":"bad"}`,
		`{"jsonml":[]}`,
		`{"jsonml":["p",{}]}`,
	} {
		if _, err := prepareJsonMLBody(strict, raw); err == nil {
			t.Errorf("prepareJsonMLBody(%q) should fail", raw)
		}
	}

	fix := jsonMLTestCommand(t, true)
	broken := `{"jsonml":["root",{},["p",{},["span",{},"ok"]]]`
	if got, err := prepareJsonMLBody(fix, broken); err != nil || !strings.Contains(got, `"root"`) {
		t.Fatalf("repaired body = %q, %v", got, err)
	}
	if got, err := prepareJsonMLBody(strict, `{"jsonml":["root",{},["unknown",{}]]}`); err != nil || !strings.Contains(got, "unknown") {
		t.Fatalf("warning-only body = %q, %v", got, err)
	}
}

func TestCrossPlatformCoveragePrepareJSONMLNode(t *testing.T) {
	strict := jsonMLTestCommand(t, false)
	if _, err := prepareJsonMLNode(strict, ""); err == nil {
		t.Fatal("empty node should fail")
	}
	for _, raw := range []string{
		`["p",{},["span",{},"ok"]]`,
		`{"jsonml":[["p",{},["span",{},"ok"]]]}`,
	} {
		if got, err := prepareJsonMLNode(strict, raw); err != nil || !strings.Contains(got, `"p"`) {
			t.Errorf("prepareJsonMLNode(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"{",
		`"text"`,
		`{"jsonml":[]}`,
		`{"jsonml":[["p",{}],["p",{}]]}`,
	} {
		if _, err := prepareJsonMLNode(strict, raw); err == nil {
			t.Errorf("prepareJsonMLNode(%q) should fail", raw)
		}
	}

	fix := jsonMLTestCommand(t, true)
	if got, err := prepareJsonMLNode(fix, `["p",{},["span",{},"ok"]`); err != nil || !strings.Contains(got, `"p"`) {
		t.Fatalf("repaired node = %q, %v", got, err)
	}
	if got, err := prepareJsonMLNode(strict, `["unknown",{}]`); err != nil || !strings.Contains(got, "unknown") {
		t.Fatalf("warning-only node = %q, %v", got, err)
	}
}
