// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import "testing"

func TestCrossPlatformCoverageFormatKnownAndUnknownBuild(t *testing.T) {
	if got := Format("v1.2.3", "abc", "now"); got != "v1.2.3 (abc, now)" {
		t.Fatalf("known build = %q", got)
	}
	if got := Format("v1.2.3", "unknown", "unknown"); got != "v1.2.3" {
		t.Fatalf("unknown build = %q", got)
	}
	if got := Format("v0", "deadbeef", "unknown"); got != "v0 (deadbeef, unknown)" {
		t.Fatalf("partial unknown = %q", got)
	}
}
