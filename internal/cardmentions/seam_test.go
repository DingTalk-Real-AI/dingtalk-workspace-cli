package cardmentions

import (
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMentionInputBoundaries(t *testing.T) {
	for _, raw := range [][]byte{[]byte("not-json"), []byte("{} {}"), {0xff}} {
		if _, err := decode(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if _, err := decode([]byte("\ufeff{}")); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{null:1}`, `{"x"`, `{"x":`, `["x",`, `"x" "trailing"`} {
		if got := rewriteMentionJSON(raw, map[string]string{"/x": "after", "/0": "after", "": "after"}); got != raw {
			t.Fatalf("partially rewrote invalid JSON %s", got)
		}
	}
	t.Setenv("DWS_A2UI_MENTIONS_DISABLED", "1")
	if err := ValidateMentionConsistency(nil, []string{"D1"}, false); err == nil {
		t.Fatal("disabled notification allowed")
	}
	t.Setenv("DWS_A2UI_MENTIONS_DISABLED", "")
}

func TestCrossPlatformCoverageMentionBindingSurfacesArraysAndDiagnostics(t *testing.T) {
	messages := []string{
		`{"createSurface":{"surfaceId":"s","dataModel":{"items":["<a atId=\"D1\">member</a>"]}}}`,
		`{"updateComponents":{"surfaceId":"s","components":[{"component":"Markdown","content":{"path":"/items/0"}}]}}`,
		`{"createSurface":{"surfaceId":"other","dataModel":{"items":["plain"]}}}`,
	}
	if got := ExtractMentions(messages).AtIDs(); !reflect.DeepEqual(got, []string{"D1"}) {
		t.Fatal(got)
	}
	messages = append(messages, `{"updateDataModel":{"surfaceId":"s","path":"/","value":{"items":["<@123>"]}}}`)
	if got := PreflightDiagnostics(messages, nil); len(got) != 1 || got[0].Code != "mention.im_placeholder" {
		t.Fatal(got)
	}
	if got := MentionDeclarationDiagnostics(messages, []string{"D1"}); len(got) != 0 {
		t.Fatal(got)
	}
	if got := PreflightDiagnostics([]string{markdownMessage(t, `<a atId="D1">member</a>`)}, []string{"D1"}); len(got) != 0 {
		t.Fatal(got)
	}
	// Map traversal and diagnostic order remain stable across invocations.
	nested := `{"z":{"component":"Markdown","content":"<a atId=\"Z\">z</a>"},"a":{"component":"Markdown","content":"<a atId=\"A\">a</a>"}}`
	for i := 0; i < 20; i++ {
		if got := strings.Join(ExtractMentions([]string{`{"updateComponents":` + nested + `}`}).AtIDs(), ","); got != "A,Z" {
			t.Fatal(got)
		}
	}
}
