// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package cli_test

import (
	"os"
	"sync"
	"testing"
)

// blockSchemaCachePublication makes the cache directory unwritable so a
// live-assembly Publish cannot replace artifacts. Unix uses chmod 0500.
func blockSchemaCachePublication(t *testing.T, cacheDirectory string) func() {
	t.Helper()
	if err := os.Chmod(cacheDirectory, 0o500); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	unblock := func() {
		once.Do(func() {
			if err := os.Chmod(cacheDirectory, 0o700); err != nil {
				t.Errorf("restore cache directory mode: %v", err)
			}
		})
	}
	t.Cleanup(unblock)
	return unblock
}
