// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package keychain

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageRegistryNamespaceKeepsDefaultAndTestPaths(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	if got := registryNamespacePath(base, "", " \t"); got != base {
		t.Fatalf("default registry path = %q, want %q", got, base)
	}

	// Pin the pre-existing test namespace layout independently of the new
	// storage-directory branch, so upgrades cannot strand test credentials.
	testDir := t.TempDir()
	absolute, err := filepath.Abs(filepath.Clean(testDir))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(absolute)))
	want := fmt.Sprintf(`%s\test-%x`, base, sum[:16])
	for _, storageDir := range []string{"", t.TempDir()} {
		if got := registryNamespacePath(base, storageDir, " "+testDir+" "); got != want {
			t.Fatalf("test registry path = %q, want %q", got, want)
		}
	}
}

func TestCrossPlatformCoverageRegistryNamespaceSeparatesStorageDirectories(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	firstDir, secondDir := t.TempDir(), t.TempDir()
	first := registryNamespacePath(base, firstDir, "")
	second := registryNamespacePath(base, secondDir, "")
	if first == second || first == base || second == base {
		t.Fatalf("storage directories share registry paths: %q, %q", first, second)
	}
	for _, got := range []string{first, second} {
		if !regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\\dir-[0-9a-f]{32}$`).MatchString(got) {
			t.Fatalf("registry path must contain only a directory digest below the service: %q", got)
		}
	}
	if got := registryNamespacePath(base, firstDir, firstDir); got == first {
		t.Fatal("production and test namespace domains overlap")
	}
	if got := registryNamespacePath(base+"-other", firstDir, ""); got == first {
		t.Fatal("different services share a registry path")
	}
}

func TestCrossPlatformCoverageRegistryNamespaceNormalizesDirectory(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	dir := t.TempDir()
	want := registryNamespacePath(base, dir, "")
	for _, spelling := range []string{
		dir + string(filepath.Separator),
		dir + string(filepath.Separator) + "child" + string(filepath.Separator) + "..",
		strings.ToUpper(dir),
	} {
		if got := registryNamespacePath(base, spelling, ""); got != want {
			t.Fatalf("equivalent directory %q selected %q, want %q", spelling, got, want)
		}
	}
	t.Chdir(dir)
	if got := registryNamespacePath(base, ".", ""); got != want {
		t.Fatalf("relative directory selected %q, want %q", got, want)
	}
}
