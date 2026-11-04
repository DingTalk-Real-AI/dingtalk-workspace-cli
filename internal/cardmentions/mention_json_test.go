package cardmentions

import "testing"

func TestCrossPlatformCoverageMentionJSONPreservesWireBytes(t *testing.T) {
	source := `{ "version":"v1.0", "x":[null,true,12,{"content" : "before"}],"untouched":"\u4f60" }`
	want := `{ "version":"v1.0", "x":[null,true,12,{"content" : "<a atId=\"10\">everyone</a>"}],"untouched":"\u4f60" }`
	if got := rewriteMentionJSON(source, map[string]string{"/x/3/content": `<a atId="10">everyone</a>`}); got != want {
		t.Fatalf("got %s; want %s", got, want)
	}
	if got := rewriteMentionJSON(source, map[string]string{"/x/3/content": "before"}); got != source {
		t.Fatal(got)
	}
	if got := rewriteMentionJSON(source, nil); got != source {
		t.Fatal(got)
	}
	if got := rewriteMentionJSON("invalid", map[string]string{"": "x"}); got != "invalid" {
		t.Fatal(got)
	}
}

func TestCrossPlatformCoverageMentionJSONPreservesLargeNumbersAndBOM(t *testing.T) {
	source := "\ufeff" + `{"n":1e1000,"content":"before"}`
	want := "\ufeff" + `{"n":1e1000,"content":"after"}`
	if got := rewriteMentionJSON(source, map[string]string{"/content": "after"}); got != want {
		t.Fatalf("got %s", got)
	}
}
