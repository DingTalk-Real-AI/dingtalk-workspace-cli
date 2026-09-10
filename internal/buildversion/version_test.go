// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import (
	"crypto/sha256"
	"testing"
)

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

func TestCrossPlatformCoverageBinaryDigestChangesWithStamp(t *testing.T) {
	oldV, oldC, oldT := stampVersion, stampCommit, stampBuildTime
	t.Cleanup(func() { stampVersion, stampCommit, stampBuildTime = oldV, oldC, oldT })

	Set("1.0.0", "aaa", "t0")
	a := Digest()
	Set("1.0.1", "bbb", "t1")
	b := Digest()
	if a == b {
		t.Fatal("Digest must change when binary stamp changes")
	}
	if a == ([sha256.Size]byte{}) || b == ([sha256.Size]byte{}) {
		t.Fatal("Digest must be non-zero")
	}
	Set("", "", "") // empty inputs keep prior
	if Digest() != b {
		t.Fatal("empty Set must leave stamp unchanged")
	}
}
