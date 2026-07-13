// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build darwin

package keychain

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateToFileDEKReencryptsSystemEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv(StorageDirEnv, root)
	t.Setenv(DisableKeychainEnv, "")
	stubMacOSSystemDEK(t, bytes.Repeat([]byte{0x42}, dekBytes), nil)

	service := "test-migrate-system-to-file"
	values := map[string]string{
		AccountToken:             "legacy-token",
		AccountToken + ":corp-a": "profile-token",
	}
	if err := Set(service, "appsecret_demo", "app-secret"); err != nil {
		t.Fatalf("Set(unrelated secret) error = %v", err)
	}
	unrelatedPath := filepath.Join(StorageDir(service), safeFileName("appsecret_demo"))
	unrelatedCiphertext := []byte("unreadable unrelated ciphertext")
	if err := os.WriteFile(unrelatedPath, unrelatedCiphertext, 0600); err != nil {
		t.Fatalf("WriteFile(unrelated secret) error = %v", err)
	}
	for account, value := range values {
		if err := Set(service, account, value); err != nil {
			t.Fatalf("Set(%q) error = %v", account, err)
		}
	}

	count, err := MigrateToFileDEK(service, true)
	if err != nil {
		t.Fatalf("MigrateToFileDEK(dry-run) error = %v", err)
	}
	if count != len(values) {
		t.Fatalf("dry-run count = %d, want %d", count, len(values))
	}
	dekPath := filepath.Join(StorageDir(service), "dek")
	if _, err := os.Stat(dekPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created file DEK; stat error = %v", err)
	}

	count, err = MigrateToFileDEK(service, false)
	if err != nil {
		t.Fatalf("MigrateToFileDEK() error = %v", err)
	}
	if count != len(values) {
		t.Fatalf("migration count = %d, want %d", count, len(values))
	}
	if info, err := os.Stat(dekPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file DEK stat = %v, %v; want mode 0600", info, err)
	}

	for _, disableKeychain := range []string{"1", ""} {
		t.Setenv(DisableKeychainEnv, disableKeychain)
		for account, want := range values {
			got, err := Get(service, account)
			if err != nil || got != want {
				t.Fatalf("Get(%q) with %s=%q = %q, %v; want %q", account, DisableKeychainEnv, disableKeychain, got, err, want)
			}
		}
	}
	if count, err := MigrateToFileDEK(service, true); err != nil || count != len(values) {
		t.Fatalf("repeat dry-run = %d, %v; want %d auth entries", count, err, len(values))
	}
	if got, err := os.ReadFile(unrelatedPath); err != nil || !bytes.Equal(got, unrelatedCiphertext) {
		t.Fatalf("unrelated secret after migration = %q, %v; want byte-for-byte preserved", got, err)
	}
}

func TestMigrateToFileDEKKeepsNewNonAuthSecretsOnSystemKeychain(t *testing.T) {
	root := t.TempDir()
	t.Setenv(StorageDirEnv, root)
	t.Setenv(DisableKeychainEnv, "")
	systemDEK := bytes.Repeat([]byte{0x52}, dekBytes)
	stubMacOSSystemDEK(t, systemDEK, nil)

	service := "test-migrate-secret-isolation"
	if err := Set(service, AccountToken, "system-token"); err != nil {
		t.Fatalf("Set(auth token) error = %v", err)
	}
	if _, err := MigrateToFileDEK(service, false); err != nil {
		t.Fatalf("MigrateToFileDEK() error = %v", err)
	}

	nonAuthEntries := map[string]string{
		"client-secret:demo": "client-secret",
		"app-token:demo":     "app-token",
		"appsecret:demo":     "stored-secret",
	}
	for account, value := range nonAuthEntries {
		if err := Set(service, account, value); err != nil {
			t.Fatalf("Set(%q) after migration error = %v", account, err)
		}
		ciphertext, err := os.ReadFile(filepath.Join(StorageDir(service), safeFileName(account)))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", account, err)
		}
		got, err := decryptData(ciphertext, systemDEK)
		if err != nil || got != value {
			t.Fatalf("system Keychain DEK decrypt %q = %q, %v; want %q", account, got, err, value)
		}
	}

	t.Setenv(DisableKeychainEnv, "1")
	if got, err := Get(service, AccountToken); err != nil || got != "system-token" {
		t.Fatalf("file-DEK Get(auth token) = %q, %v; want migrated token", got, err)
	}
	for account := range nonAuthEntries {
		if got, err := Get(service, account); !IsCiphertextKeyMismatch(err) {
			t.Fatalf("file-DEK Get(%q) = %q, %v; want ciphertext key mismatch", account, got, err)
		}
	}
}

func TestMigrateToFileDEKAbortsBeforeWritingWhenAnyEntryIsUnreadable(t *testing.T) {
	root := t.TempDir()
	t.Setenv(StorageDirEnv, root)
	t.Setenv(DisableKeychainEnv, "")
	stubMacOSSystemDEK(t, bytes.Repeat([]byte{0x24}, dekBytes), nil)

	service := "test-migrate-preflight"
	if err := Set(service, AccountToken, "preserve-me"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	goodPath := filepath.Join(StorageDir(service), safeFileName(AccountToken))
	goodBefore, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatalf("ReadFile(good entry) error = %v", err)
	}
	badPath := filepath.Join(StorageDir(service), safeFileName(AccountToken+":bad"))
	if err := os.WriteFile(badPath, []byte("corrupt ciphertext"), 0600); err != nil {
		t.Fatalf("WriteFile(bad entry) error = %v", err)
	}

	if _, err := MigrateToFileDEK(service, false); err == nil || !strings.Contains(err.Error(), "validate keychain entry") {
		t.Fatalf("MigrateToFileDEK() error = %v, want preflight failure", err)
	}
	goodAfter, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatalf("ReadFile(good entry after failure) error = %v", err)
	}
	if !bytes.Equal(goodAfter, goodBefore) {
		t.Fatal("failed migration modified a readable entry")
	}
	if _, err := os.Stat(filepath.Join(StorageDir(service), "dek")); !os.IsNotExist(err) {
		t.Fatalf("failed migration created file DEK; stat error = %v", err)
	}
}

func TestMigrateToFileDEKRequiresSystemKeychainMode(t *testing.T) {
	t.Setenv(StorageDirEnv, t.TempDir())
	t.Setenv(DisableKeychainEnv, "1")
	if _, err := MigrateToFileDEK("test-migrate-mode", true); err == nil || !strings.Contains(err.Error(), DisableKeychainEnv) {
		t.Fatalf("MigrateToFileDEK() error = %v, want mode guidance", err)
	}
}
