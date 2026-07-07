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

//go:build darwin

package keychain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keyringpkg "github.com/zalando/go-keyring"
)

func TestDefaultKeychainPathFromSecurityOutput(t *testing.T) {
	got := defaultKeychainPathFromSecurityOutput([]byte("\"/Users/me/Library/Keychains/login.keychain-db\"\n"))
	if got != "/Users/me/Library/Keychains/login.keychain-db" {
		t.Fatalf("path = %q", got)
	}
}

func TestCheckDefaultKeychainAvailableReportsMissingPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.keychain-db")
	prev := readDefaultKeychain
	readDefaultKeychain = func() ([]byte, error) {
		return []byte("\"" + missingPath + "\"\n"), nil
	}
	t.Cleanup(func() {
		readDefaultKeychain = prev
	})

	err := checkDefaultKeychainAvailable()
	if !IsUnavailable(err) {
		t.Fatalf("error = %v, want unavailable", err)
	}
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error = %v, want missing keychain path", err)
	}
}

func TestGetMissingAccountDoesNotReadMacOSDEK(t *testing.T) {
	t.Setenv(DisableKeychainEnv, "")

	keychainPath := filepath.Join(t.TempDir(), "login.keychain-db")
	if err := os.WriteFile(keychainPath, nil, 0600); err != nil {
		t.Fatalf("WriteFile(default keychain) error = %v", err)
	}

	prevReadDefault := readDefaultKeychain
	prevGet := keyringGet
	prevSet := keyringSet
	readDefaultKeychain = func() ([]byte, error) {
		return []byte("\"" + keychainPath + "\"\n"), nil
	}
	keyringGet = func(service, account string) (string, error) {
		t.Fatalf("keyring.Get(%q, %q) called for missing account", service, account)
		return "", nil
	}
	keyringSet = func(service, account, value string) error {
		t.Fatalf("keyring.Set(%q, %q) called for missing account", service, account)
		return nil
	}
	t.Cleanup(func() {
		readDefaultKeychain = prevReadDefault
		keyringGet = prevGet
		keyringSet = prevSet
	})

	got, err := Get("test-missing-account", "auth-token")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("Get() = %q, want empty string", got)
	}
}

func TestGetWithMissingMacOSDEKDoesNotCreateDEK(t *testing.T) {
	t.Setenv(DisableKeychainEnv, "")

	keychainPath := filepath.Join(t.TempDir(), "login.keychain-db")
	if err := os.WriteFile(keychainPath, nil, 0600); err != nil {
		t.Fatalf("WriteFile(default keychain) error = %v", err)
	}

	prevReadDefault := readDefaultKeychain
	prevGet := keyringGet
	prevSet := keyringSet
	readDefaultKeychain = func() ([]byte, error) {
		return []byte("\"" + keychainPath + "\"\n"), nil
	}
	keyringGet = func(service, account string) (string, error) {
		if account != "dek" {
			t.Fatalf("keyring.Get account = %q, want dek", account)
		}
		return "", keyringpkg.ErrNotFound
	}
	setCalls := 0
	keyringSet = func(service, account, value string) error {
		setCalls++
		return errors.New("keyring.Set should not be called by Get")
	}
	t.Cleanup(func() {
		readDefaultKeychain = prevReadDefault
		keyringGet = prevGet
		keyringSet = prevSet
	})

	service := "test-missing-dek"
	account := "auth-token"
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeFileName(account)), []byte("ciphertext"), 0600); err != nil {
		t.Fatalf("WriteFile(ciphertext) error = %v", err)
	}

	_, err := Get(service, account)
	if !IsDEKMissing(err) {
		t.Fatalf("Get() error = %v, want dek missing", err)
	}
	if setCalls != 0 {
		t.Fatalf("keyring.Set calls = %d, want 0", setCalls)
	}
}

// TestDisableKeychainFallback verifies that setting DWS_DISABLE_KEYCHAIN
// routes the DEK to a local file (same scheme as Linux) and the full
// Set/Get/Remove cycle works without touching the system Keychain.
// This is the support path for sandboxed runtimes such as Codex App.
func TestDisableKeychainFallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(StorageDirEnv, tmp)
	t.Setenv(DisableKeychainEnv, "1")

	service := "test-disable-keychain"
	account := "auth-token"
	payload := `{"access_token":"abc","refresh_token":"def"}`

	if err := Set(service, account, payload); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// File DEK must materialize on disk.
	dekPath := filepath.Join(tmp, service, "dek")
	info, err := os.Stat(dekPath)
	if err != nil {
		t.Fatalf("file DEK not created at %s: %v", dekPath, err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("DEK file perm = %o, want 0600", mode)
	}

	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != payload {
		t.Fatalf("Get() = %q, want %q", got, payload)
	}

	// A second Get must reuse the same DEK (no regeneration).
	dek1, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(dek) error = %v", err)
	}
	if _, err := Get(service, account); err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	dek2, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(dek) second error = %v", err)
	}
	if string(dek1) != string(dek2) {
		t.Fatal("DEK rotated between calls; want stable")
	}

	if err := Remove(service, account); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if Exists(service, account) {
		t.Fatal("Exists() = true after Remove(), want false")
	}
}

// TestDisableKeychainOverwrite verifies the fallback path supports
// overwriting an existing token entry.
func TestDisableKeychainOverwrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(StorageDirEnv, tmp)
	t.Setenv(DisableKeychainEnv, "1")

	service := "test-disable-keychain-overwrite"
	account := "auth-token"

	if err := Set(service, account, "initial"); err != nil {
		t.Fatalf("Set() initial error = %v", err)
	}
	if err := Set(service, account, "overwritten"); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}

	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "overwritten" {
		t.Fatalf("Get() = %q, want %q", got, "overwritten")
	}
}
