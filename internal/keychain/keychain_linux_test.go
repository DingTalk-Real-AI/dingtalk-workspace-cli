// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

//go:build linux

package keychain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageLinuxOldCiphertextUsesMismatchClassification(t *testing.T) {
	t.Setenv(StorageDirEnv, t.TempDir())
	service := "linux-old-ciphertext-" + t.Name()

	if err := Set(service, "old-account", "old-secret"); err != nil {
		t.Fatalf("Set(old-account) error = %v", err)
	}
	if err := os.Remove(filepath.Join(StorageDir(service), "dek")); err != nil {
		t.Fatalf("remove old DEK: %v", err)
	}
	if err := Set(service, "new-account", "new-secret"); err != nil {
		t.Fatalf("Set(new-account) error = %v", err)
	}

	if _, err := Get(service, "old-account"); !IsCiphertextKeyMismatch(err) {
		t.Fatalf("Get(old-account) error = %v, want ciphertext key mismatch", err)
	}
}

func TestCrossPlatformCoverageLinuxAuthInventoryUsesMismatchClassification(t *testing.T) {
	t.Setenv(StorageDirEnv, t.TempDir())
	service := "linux-auth-inventory-" + t.Name()
	oldAccount := AccountToken + ":old-corp"

	if err := Set(service, oldAccount, "old-secret"); err != nil {
		t.Fatalf("Set(old account) error = %v", err)
	}
	if err := os.Remove(filepath.Join(StorageDir(service), "dek")); err != nil {
		t.Fatalf("remove old DEK: %v", err)
	}
	if err := Set(service, AccountToken, "new-secret"); err != nil {
		t.Fatalf("Set(new account) error = %v", err)
	}

	if err := ValidateAuthTokenEntries(service); !IsCiphertextKeyMismatch(err) {
		t.Fatalf("ValidateAuthTokenEntries() error = %v, want ciphertext key mismatch", err)
	}
}
