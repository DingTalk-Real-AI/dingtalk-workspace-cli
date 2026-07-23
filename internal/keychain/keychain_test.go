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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dws-keychain-test-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv(StorageDirEnv, dir)
	_ = os.Setenv(TestNamespaceEnv, dir)
	_ = os.Setenv(DisableKeychainEnv, "1")
	code := m.Run()
	if err := RemoveAuthTokenEntries(Service); err != nil {
		fmt.Fprintf(os.Stderr, "internal/keychain test cleanup: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestKeychainBasicOperations(t *testing.T) {
	t.Parallel()

	service := "test-service-" + t.Name()
	account := "test-account"
	testData := `{"access_token":"test123","refresh_token":"refresh456"}`

	// Clean up after test
	defer func() {
		_ = Remove(service, account)
	}()

	// Test Set
	if err := Set(service, account, testData); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Test Exists
	if !Exists(service, account) {
		t.Fatal("Exists() = false, want true")
	}

	// Test Get
	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != testData {
		t.Fatalf("Get() = %q, want %q", got, testData)
	}

	// Test Remove
	if err := Remove(service, account); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Verify removal
	if Exists(service, account) {
		t.Fatal("Exists() after Remove() = true, want false")
	}
}

func TestKeychainNonExistentAccount(t *testing.T) {
	t.Parallel()

	service := "test-service-nonexistent"
	account := "nonexistent-account"

	// Get should return empty string for non-existent
	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("Get() = %q, want empty string", got)
	}

	// Exists should return false
	if Exists(service, account) {
		t.Fatal("Exists() = true for non-existent, want false")
	}

	// Remove should not error for non-existent
	if err := Remove(service, account); err != nil {
		t.Fatalf("Remove() error = %v for non-existent", err)
	}
}

func TestGetNonExistentAccountDoesNotCreateFileDEK(t *testing.T) {
	service := "test-service-readonly-" + t.Name()
	account := "nonexistent-account"
	dekPath := filepath.Join(StorageDir(service), "dek")

	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("Get() = %q, want empty string", got)
	}
	if _, err := os.Stat(dekPath); !os.IsNotExist(err) {
		t.Fatalf("Get() created DEK at %s; stat error = %v", dekPath, err)
	}
}

func TestKeychainOverwrite(t *testing.T) {
	t.Parallel()

	service := "test-service-" + t.Name()
	account := "test-account"

	defer func() {
		_ = Remove(service, account)
	}()

	// Set initial value
	if err := Set(service, account, "initial"); err != nil {
		t.Fatalf("Set() initial error = %v", err)
	}

	// Overwrite
	if err := Set(service, account, "overwritten"); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}

	// Verify overwrite
	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "overwritten" {
		t.Fatalf("Get() = %q, want %q", got, "overwritten")
	}
}

func TestRemoveAuthTokenEntriesPreservesOtherAccounts(t *testing.T) {
	service := "test-service-" + t.Name()
	authAccounts := []string{
		AccountToken,
		AccountToken + ":corp-a",
		AccountToken + ":id:0123456789abcdef",
	}
	for _, account := range authAccounts {
		if err := Set(service, account, "secret"); err != nil {
			t.Fatalf("Set(%q) error = %v", account, err)
		}
	}
	const unrelated = "app-secret:client-a"
	if err := Set(service, unrelated, "preserve"); err != nil {
		t.Fatalf("Set(unrelated) error = %v", err)
	}

	if err := RemoveAuthTokenEntries(service); err != nil {
		t.Fatalf("RemoveAuthTokenEntries() error = %v", err)
	}
	for _, account := range authAccounts {
		if Exists(service, account) {
			t.Fatalf("auth account %q still exists", account)
		}
	}
	if got, err := Get(service, unrelated); err != nil || got != "preserve" {
		t.Fatalf("unrelated account = %q, %v; want preserved", got, err)
	}
}

func TestMigrationNoLegacyData(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	// Migration should be a no-op when no legacy data exists
	result := MigrateFromLegacy(configDir)

	if result.Migrated {
		t.Fatal("Migrated = true when no legacy data, want false")
	}
	if result.NeedRelogin {
		t.Fatal("NeedRelogin = true when no legacy data, want false")
	}
	if result.Error != nil {
		t.Fatalf("Error = %v when no legacy data, want nil", result.Error)
	}
}

func TestHasLegacyData(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	// Should be false when no .data file
	if HasLegacyData(configDir) {
		t.Fatal("HasLegacyData() = true for empty dir, want false")
	}

	// Create a fake .data file
	legacyPath := filepath.Join(configDir, ".data")
	if err := os.WriteFile(legacyPath, []byte("fake"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Should be true now
	if !HasLegacyData(configDir) {
		t.Fatal("HasLegacyData() = false after creating .data, want true")
	}
}
