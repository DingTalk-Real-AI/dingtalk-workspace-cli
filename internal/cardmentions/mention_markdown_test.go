package cardmentions

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMentionMarkdownExamplesAreNotNotifications(t *testing.T) {
	for _, example := range []string{
		"`<a atId=\"all\">everyone</a> <@123>`",
		"``<a atId=\"all\">`x`</a> <@123>``",
		"```html\n<a atId=\"all\">everyone</a> <@123>\n```",
		"~~~html\n<a atId=\"all\">everyone</a> <@123>\n~~~",
		"    <a atId=\"all\">everyone</a> <@123>\n",
		`\<a atId="all">everyone</a> \<@123>`,
	} {
		t.Run(example, func(t *testing.T) {
			messages := []string{markdownMessage(t, example)}
			usage := ExtractMentions(messages)
			if len(usage.AtTags)+len(usage.IMPlaceholders) != 0 {
				t.Fatalf("code treated as notification: %#v", usage)
			}
			if got := RewriteMentionAllTags(messages); got[0] != messages[0] {
				t.Fatalf("example rewritten: %s", got[0])
			}
			if got := MentionIMPlaceholderDiagnostics(example, "/content"); len(got) != 0 {
				t.Fatalf("example rejected: %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageMentionMarkdownMixedCodeAndRealTag(t *testing.T) {
	body := "`<a atId=\"all\">example</a>` and <a atId=\"all\">everyone</a>"
	if err := ValidateMentionConsistency([]string{markdownMessage(t, body)}, nil, true); err != nil {
		t.Fatal(err)
	}
	rewritten := RewriteMentionAllTags([]string{markdownMessage(t, body)})[0]
	if !strings.Contains(rewritten, `atId=\"all\">example`) || !strings.Contains(rewritten, `atId=\"10\">everyone`) {
		t.Fatalf("bad mixed rewrite: %s", rewritten)
	}
	if got := ExtractMentions([]string{markdownMessage(t, "plain")}); len(got.AtTags) != 0 {
		t.Fatal(got)
	}
}
