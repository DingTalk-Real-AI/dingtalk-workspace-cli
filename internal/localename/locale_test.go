package localename

import "testing"

func TestCrossPlatformCoverageResolve(t *testing.T) {
	if got := Resolve("zh-CN"); got != "zh" {
		t.Fatalf("zh = %q", got)
	}
	if got := Resolve(" EN "); got != "en" {
		t.Fatalf("en = %q", got)
	}
}
