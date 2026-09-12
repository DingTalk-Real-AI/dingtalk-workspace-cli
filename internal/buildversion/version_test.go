// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestCrossPlatformCoverageExecutableMaterialFaultPaths(t *testing.T) {
	v, c, bt := CurrentStampForTest()
	if v == "" || c == "" || bt == "" {
		t.Fatalf("CurrentStampForTest empty: %q %q %q", v, c, bt)
	}

	t.Run("executable unavailable", func(t *testing.T) {
		testseam.Swap(t, &osExecutable, func() (string, error) {
			return "", errors.New("no executable")
		})
		got := computeExecutableMaterial()
		if string(got) != "exe-unavailable" {
			t.Fatalf("Executable error material = %q", got)
		}
	})

	t.Run("missing path falls through to fingerprintStatOnly", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing-binary")
		testseam.Swap(t, &osExecutable, func() (string, error) { return missing, nil })
		got := computeExecutableMaterial()
		want := fingerprintStatOnly(missing)
		if !bytes.Equal(got, want) {
			t.Fatal("missing-path material must equal fingerprintStatOnly")
		}
	})

	t.Run("stat fingerprint miss and hit", func(t *testing.T) {
		missing := fingerprintStatOnly(filepath.Join(t.TempDir(), "gone"))
		path := filepath.Join(t.TempDir(), "present")
		if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
			t.Fatal(err)
		}
		present := fingerprintStatOnly(path)
		if len(missing) != sha256.Size || len(present) != sha256.Size {
			t.Fatal("fingerprint length")
		}
		if bytes.Equal(missing, present) {
			t.Fatal("missing and present fingerprints must differ")
		}
	})

	t.Run("default path uses metadata fingerprint not full-file hash", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ok-binary")
		if err := os.WriteFile(path, []byte("ok-payload"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &osExecutable, func() (string, error) { return path, nil })
		got := computeExecutableMaterial()
		want := fingerprintStatOnly(path)
		if !bytes.Equal(got, want) {
			t.Fatal("unstamped default material must equal fingerprintStatOnly(path)")
		}
		// Also exercise the process-cached seam entrypoint once.
		testseam.Swap(t, &ExecutableMaterial, cachedExecutableMaterial)
		_ = cachedExecutableMaterial()
	})

	t.Run("size and mtime changes alter fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		aPath := filepath.Join(dir, "bin-a")
		bPath := filepath.Join(dir, "bin-b")
		if err := os.WriteFile(aPath, []byte("small"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bPath, []byte("much-larger-payload"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Ensure mtime differs even on coarse filesystems.
		past := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(aPath, past, past); err != nil {
			t.Fatal(err)
		}
		fa := fingerprintStatOnly(aPath)
		fb := fingerprintStatOnly(bPath)
		if bytes.Equal(fa, fb) {
			t.Fatal("different size/mtime/path binaries must fingerprint differently")
		}
		testseam.Swap(t, &osExecutable, func() (string, error) { return aPath, nil })
		gotA := computeExecutableMaterial()
		testseam.Swap(t, &osExecutable, func() (string, error) { return bPath, nil })
		gotB := computeExecutableMaterial()
		if bytes.Equal(gotA, gotB) {
			t.Fatal("computeExecutableMaterial must track metadata replace A→B")
		}
	})
}
