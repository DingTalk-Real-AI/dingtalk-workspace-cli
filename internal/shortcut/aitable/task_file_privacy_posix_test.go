// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package aitable

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func aitablePrivateTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func aitableTaskFilePrivacyError(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("task file is not private: mode=%v", info.Mode())
	}
	return nil
}

func TestCrossPlatformCoverageTaskFilePrivacyRejectsPublicAccess(t *testing.T) {
	path := filepath.Join(aitablePrivateTempDir(t), "task.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := aitableTaskFilePrivacyError(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := aitableTaskFilePrivacyError(path); err == nil {
		t.Fatal("publicly readable task file accepted")
	}
}
