package buildversion

import "testing"

func TestCrossPlatformCoverageFormat(t *testing.T) {
	if got := Format("1.0", "unknown", "unknown"); got != "1.0" {
		t.Fatalf("plain version = %q", got)
	}
	if got := Format("1.0", "abc", "now"); got != "1.0 (abc, now)" {
		t.Fatalf("detailed version = %q", got)
	}
	if got := Format("1.0", "abc", "unknown"); got != "1.0 (abc, unknown)" {
		t.Fatalf("commit-only version = %q", got)
	}
}
