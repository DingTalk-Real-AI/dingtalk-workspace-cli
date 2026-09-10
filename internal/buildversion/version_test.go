// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import (
	"crypto/sha256"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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

func TestCrossPlatformCoverageUnstampedDigestFoldsExecutableMaterial(t *testing.T) {
	oldV, oldC, oldT := stampVersion, stampCommit, stampBuildTime
	t.Cleanup(func() { stampVersion, stampCommit, stampBuildTime = oldV, oldC, oldT })
	stampVersion, stampCommit, stampBuildTime = "dev", "unknown", "unknown"

	testseam.Swap(t, &ExecutableMaterial, func() []byte { return []byte("exe-material-A") })
	a := Digest()
	testseam.Swap(t, &ExecutableMaterial, func() []byte { return []byte("exe-material-B") })
	b := Digest()
	if a == b {
		t.Fatal("unstamped Digest must change when executable material changes")
	}
	if a == ([sha256.Size]byte{}) || b == ([sha256.Size]byte{}) {
		t.Fatal("Digest must be non-zero")
	}
}

func TestCrossPlatformCoverageStampedDigestIgnoresExecutableMaterial(t *testing.T) {
	oldV, oldC, oldT := stampVersion, stampCommit, stampBuildTime
	t.Cleanup(func() { stampVersion, stampCommit, stampBuildTime = oldV, oldC, oldT })
	Set("1.2.3", "deadbeef", "2026-01-02T03:04:05Z")

	testseam.Swap(t, &ExecutableMaterial, func() []byte { return []byte("exe-material-A") })
	a := Digest()
	testseam.Swap(t, &ExecutableMaterial, func() []byte { return []byte("exe-material-B") })
	b := Digest()
	if a != b {
		t.Fatal("stamped Digest must stay stamp-only and ignore executable material")
	}
}
