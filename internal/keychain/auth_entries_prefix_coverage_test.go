// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin || linux

package keychain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoveragePrefixEntryRemovalFailureEdges(t *testing.T) {
	originalReadDir, originalInfo := authEntriesReadDir, authEntryInfo
	t.Cleanup(func() {
		authEntriesReadDir, authEntryInfo = originalReadDir, originalInfo
	})
	service := "prefix-edges-" + t.Name()
	t.Setenv(StorageDirEnv, t.TempDir())
	if err := platformRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); err != nil {
		t.Fatalf("missing storage = %v", err)
	}
	fail := errors.New("read failed")
	authEntriesReadDir = func(string) ([]os.DirEntry, error) { return nil, fail }
	if err := platformRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); !errors.Is(err, fail) {
		t.Fatalf("read failure = %v", err)
	}

	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	matching := safeFileName("appsecret:id")
	unmatched := safeFileName("auth-token")
	for _, name := range []string{matching, unmatched} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	authEntriesReadDir = os.ReadDir
	authEntryInfo = func(os.DirEntry) (os.FileInfo, error) { return nil, fail }
	if err := platformRemoveAccountEntriesWithPrefixes(service, []string{"", "appsecret:"}); !errors.Is(err, fail) {
		t.Fatalf("entry info failure = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, unmatched)); err != nil {
		t.Fatalf("unmatched entry was removed: %v", err)
	}

	authEntryInfo = originalInfo
	if err := os.Remove(filepath.Join(dir, matching)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, matching), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := platformRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); err == nil {
		t.Fatal("non-regular matching entry succeeded")
	}

	regularPath := filepath.Join(dir, "regular")
	if err := os.WriteFile(regularPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	regularInfo, err := os.Stat(regularPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, matching, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	authEntryInfo = func(os.DirEntry) (os.FileInfo, error) { return regularInfo, nil }
	if err := platformRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); err == nil {
		t.Fatal("remove failure was ignored")
	}
}
