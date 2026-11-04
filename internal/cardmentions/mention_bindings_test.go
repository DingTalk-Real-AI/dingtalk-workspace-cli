package cardmentions

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMentionBoundMarkdown(t *testing.T) {
	initial := `{"createSurface":{"surfaceId":"s","dataModel":{"answer":{"text":"<a atId=\"DAAA1\">member</a> <a atId=\"all\">everyone</a>"}}}}`
	component := `{"updateComponents":{"surfaceId":"s","components":[{"id":"m","component":"Markdown","content":{"path":"/answer/text"}}]}}`
	usage := ExtractMentions([]string{initial, component})
	if !reflect.DeepEqual(usage.AtIDs(), []string{"DAAA1"}) || !usage.HasAtAll() {
		t.Fatalf("bound mentions missing: %#v", usage)
	}
	if err := ValidateMentionConsistency([]string{initial, component}, []string{"DAAA1"}, true); err != nil {
		t.Fatal(err)
	}
	rewritten := RewriteMentionAllTags([]string{initial, component})
	expected := strings.Replace(initial, `atId=\"all\"`, `atId=\"10\"`, 1)
	if rewritten[0] != expected || rewritten[1] != component {
		t.Fatalf("binding source not surgically normalized: %#v", rewritten)
	}

	delta := `{"updateDataModel":{"surfaceId":"s","path":"/answer","value":{"text":"plain"}}}`
	if got := ExtractMentions([]string{initial, component, delta}); len(got.AtTags) != 0 {
		t.Fatal(got)
	}
	// Placeholder creation can notify declared members before bound text arrives.
	if err := ValidateMentionConsistency([]string{component}, []string{"DAAA1"}, true); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageMentionStreamingUpdateAndStopSwitch(t *testing.T) {
	delta := `{"updateDataModel":{"surfaceId":"s","path":"/answer","value":{"text":"<a atId=\"all\">everyone</a>"}}}`
	component := `{"updateComponents":{"surfaceId":"s","components":[{"id":"m","component":"Markdown","content":{"path":"/answer/text"}}]}}`
	got := RewriteMentionAllTags([]string{component, delta})
	if got[0] != component || !strings.Contains(got[1], `atId=\"10\"`) {
		t.Fatalf("bound delta missing normalization: %#v", got)
	}
	// A value-only delta has no component type: do not rewrite arbitrary data.
	if got := RewriteMentionAllTags([]string{delta}); got[0] != delta {
		t.Fatal(got)
	}
	t.Setenv("DWS_A2UI_MENTIONS_DISABLED", "true")
	if err := ValidateMentionConsistency([]string{component, delta}, nil, true); err == nil {
		t.Fatal("switch allowed notification")
	}
	if got := RewriteMentionAllTags([]string{component, delta}); !reflect.DeepEqual(got, []string{component, delta}) {
		t.Fatal(got)
	}
	t.Setenv("DWS_A2UI_MENTIONS_DISABLED", "false")
	if err := ValidateMentionConsistency([]string{component, delta}, nil, true); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageMentionBindingRequiresAValidPath(t *testing.T) {
	for _, path := range []string{"", "relative", "/bad~", "/bad~2"} {
		body := fmt.Sprintf(`{"updateComponents":{"surfaceId":"s","components":[{"component":"Markdown","content":{"path":%q}}]}}`, path)
		if err := ValidateMentionConsistency([]string{body}, []string{"DAAA1"}, true); err == nil {
			t.Fatalf("invalid binding %q bypassed validation", path)
		}
	}
	for _, path := range []string{"/", "/a~0b~1c"} {
		if !validMentionPath(path) {
			t.Fatal(path)
		}
	}
}
