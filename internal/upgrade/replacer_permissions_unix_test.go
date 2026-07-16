// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

//go:build !windows

package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageCopyFileExecutablePermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("copy me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o755 != 0o755 {
		t.Errorf("dst perm = %o, want 0755", info.Mode().Perm())
	}
}
