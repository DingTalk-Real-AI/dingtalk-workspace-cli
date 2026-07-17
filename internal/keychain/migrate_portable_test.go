// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keychain

import (
	"errors"
	"os"
	"testing"
)

func TestCrossPlatformCoverageMigrateFromLegacyPortableEdges(t *testing.T) {
	injectedErr := errors.New("injected migration failure")
	origMAC := migrateGetMACAddress
	origDecrypt := migrateDecrypt
	origExists := migrateExists
	origSet := migrateSet
	origRead := keychainReadFile
	origStat := keychainStat
	origRename := keychainRename
	origRemove := keychainRemove
	t.Cleanup(func() {
		migrateGetMACAddress = origMAC
		migrateDecrypt = origDecrypt
		migrateExists = origExists
		migrateSet = origSet
		keychainReadFile = origRead
		keychainStat = origStat
		keychainRename = origRename
		keychainRemove = origRemove
	})

	migrateExists = func(string, string) bool { return true }
	if got := MigrateFromLegacy(t.TempDir()); got.Migrated || got.Error != nil {
		t.Fatalf("existing keychain migration = %#v, want no-op", got)
	}

	migrateExists = func(string, string) bool { return false }
	keychainStat = func(string) (os.FileInfo, error) { return nil, nil }
	migrateGetMACAddress = func() (string, error) { return "", injectedErr }
	got := MigrateFromLegacy(t.TempDir())
	if !got.NeedRelogin || !errors.Is(got.Error, injectedErr) || got.FromPath == "" {
		t.Fatalf("MAC failure migration = %#v, want relogin error with source path", got)
	}

	migrateGetMACAddress = func() (string, error) { return "test-mac", nil }
	keychainReadFile = func(string) ([]byte, error) { return nil, injectedErr }
	if _, err := loadLegacyData(t.TempDir()); !errors.Is(err, injectedErr) {
		t.Fatalf("loadLegacyData() read error = %v, want injected error", err)
	}

	keychainReadFile = func(string) ([]byte, error) { return []byte("ciphertext"), nil }
	migrateDecrypt = func([]byte, []byte) ([]byte, error) { return nil, injectedErr }
	if _, err := loadLegacyData(t.TempDir()); !errors.Is(err, injectedErr) {
		t.Fatalf("loadLegacyData() decrypt error = %v, want injected error", err)
	}

	migrateDecrypt = func([]byte, []byte) ([]byte, error) { return []byte("not-json"), nil }
	if _, err := loadLegacyData(t.TempDir()); err == nil {
		t.Fatal("loadLegacyData() error = nil, want invalid JSON error")
	}

	migrateDecrypt = func([]byte, []byte) ([]byte, error) {
		return []byte(`{"access_token":"legacy-token"}`), nil
	}
	legacyData, err := loadLegacyData(t.TempDir())
	if err != nil || legacyData["access_token"] != "legacy-token" {
		t.Fatalf("loadLegacyData() = %#v, %v; want decoded legacy token", legacyData, err)
	}

	migrateSet = func(string, string, string) error { return injectedErr }
	got = MigrateFromLegacy(t.TempDir())
	if !errors.Is(got.Error, injectedErr) || got.Migrated || got.NeedRelogin {
		t.Fatalf("keychain set failure migration = %#v", got)
	}

	migrateSet = func(string, string, string) error { return nil }
	keychainRename = func(string, string) error { return nil }
	got = MigrateFromLegacy(t.TempDir())
	if got.Error != nil || !got.Migrated || got.BackupPath == "" {
		t.Fatalf("successful migration = %#v", got)
	}

	removedLegacy := false
	keychainRename = func(string, string) error { return injectedErr }
	keychainRemove = func(string) error {
		removedLegacy = true
		return nil
	}
	got = MigrateFromLegacy(t.TempDir())
	if got.Error != nil || !got.Migrated || got.BackupPath != "" || !removedLegacy {
		t.Fatalf("rename fallback migration = %#v, removedLegacy=%v", got, removedLegacy)
	}

	keychainRemove = func(string) error { return os.ErrNotExist }
	if err := CleanupLegacyBackup(t.TempDir()); err != nil {
		t.Fatalf("CleanupLegacyBackup() missing backup error = %v", err)
	}
	keychainRemove = func(string) error { return injectedErr }
	if err := CleanupLegacyBackup(t.TempDir()); !errors.Is(err, injectedErr) {
		t.Fatalf("CleanupLegacyBackup() error = %v, want injected error", err)
	}
	keychainRemove = func(string) error { return nil }
	if err := CleanupLegacyBackup(t.TempDir()); err != nil {
		t.Fatalf("CleanupLegacyBackup() error = %v", err)
	}
}
