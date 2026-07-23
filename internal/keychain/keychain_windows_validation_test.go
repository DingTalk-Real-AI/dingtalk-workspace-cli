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

//go:build windows

package keychain

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func writeRawRegistryString(t *testing.T, service, account, value string) {
	t.Helper()
	writeRawRegistryNamedString(t, service, valueNameForAccount(account), value)
}

func writeRawRegistryNamedString(t *testing.T, service, name, value string) {
	t.Helper()
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		registryPathForService(service),
		registry.SET_VALUE,
	)
	if err != nil {
		t.Fatalf("registry.CreateKey() error = %v", err)
	}
	defer k.Close()
	if err := k.SetStringValue(name, value); err != nil {
		t.Fatalf("SetStringValue(%q) error = %v", name, err)
	}
}

func writeRawRegistryDWORD(t *testing.T, service, account string, value uint32) {
	t.Helper()
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		registryPathForService(service),
		registry.SET_VALUE,
	)
	if err != nil {
		t.Fatalf("registry.CreateKey() error = %v", err)
	}
	defer k.Close()
	if err := k.SetDWordValue(valueNameForAccount(account), value); err != nil {
		t.Fatalf("SetDWordValue(%q) error = %v", account, err)
	}
}

func TestCrossPlatformCoverageWindowsRegistryDistinguishesMissingFromUnreadable(t *testing.T) {
	service := "test-service-" + t.Name()
	account := AccountToken + ":legacy-corp"
	t.Cleanup(func() { _ = Remove(service, account) })

	got, err := Get(service, account)
	if err != nil || got != "" {
		t.Fatalf("missing registry account = %q, %v; want empty and nil", got, err)
	}

	writeRawRegistryString(t, service, account, "")
	if _, err := Get(service, account); err == nil || !strings.Contains(err.Error(), "empty DPAPI ciphertext") {
		t.Fatalf("empty registry value error = %v", err)
	}
	missingAccount := AccountToken + ":missing-value"
	if got, err := Get(service, missingAccount); err != nil || got != "" {
		t.Fatalf("missing value in existing registry key = %q, %v; want empty and nil", got, err)
	}

	writeRawRegistryString(t, service, account, "%%%not-base64%%")
	if _, err := Get(service, account); err == nil || !strings.Contains(err.Error(), "decode registry account") {
		t.Fatalf("invalid base64 error = %v", err)
	}

	writeRawRegistryString(t, service, account, base64.StdEncoding.EncodeToString([]byte("not a DPAPI blob")))
	if _, err := Get(service, account); err == nil || !strings.Contains(err.Error(), "dpapi unprotect") {
		t.Fatalf("invalid DPAPI blob error = %v", err)
	}

	writeRawRegistryDWORD(t, service, account, 1)
	if _, err := Get(service, account); err == nil || !strings.Contains(err.Error(), "registry read account") {
		t.Fatalf("unexpected registry value type error = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsAuthTokenInventoryValidatesOrphanSlots(t *testing.T) {
	service := "test-service-" + t.Name()
	const unrelated = "app-secret:client"
	orphan := AccountToken + ":orphan-corp"
	t.Cleanup(func() {
		_ = Remove(service, unrelated)
		_ = Remove(service, orphan)
	})

	if err := ValidateAuthTokenEntries(service); err != nil {
		t.Fatalf("missing registry key validation error = %v", err)
	}

	// Corruption in unrelated keychain accounts is outside the auth inventory.
	writeRawRegistryString(t, service, unrelated, "%%%not-base64%%")
	writeRawRegistryNamedString(t, service, "%%%not-an-account-name%%", "%%%not-base64%%")
	if err := ValidateAuthTokenEntries(service); err != nil {
		t.Fatalf("unrelated registry value validation error = %v", err)
	}

	// The orphan is deliberately not represented in profiles.json. Inventory
	// discovery must still find it by its historical auth-token account prefix.
	writeRawRegistryString(t, service, orphan, "%%%not-base64%%")
	if err := ValidateAuthTokenEntries(service); err == nil || !strings.Contains(err.Error(), orphan) {
		t.Fatalf("orphan auth token validation error = %v", err)
	}

	if err := Set(service, orphan, "legacy token"); err != nil {
		t.Fatalf("Set(orphan) error = %v", err)
	}
	if err := ValidateAuthTokenEntries(service); err != nil {
		t.Fatalf("valid orphan auth token validation error = %v", err)
	}
	got, err := Get(service, orphan)
	if err != nil || got != "legacy token" {
		t.Fatalf("round trip = %q, %v; want legacy token", got, err)
	}
}

func TestCrossPlatformCoverageWindowsRegistryReadFailuresFailClosed(t *testing.T) {
	t.Run("open failure", func(t *testing.T) {
		originalOpen := registryOpenReadKey
		failure := windows.ERROR_ACCESS_DENIED
		registryOpenReadKey = func(registry.Key, string, uint32) (registry.Key, error) {
			return 0, failure
		}
		t.Cleanup(func() { registryOpenReadKey = originalOpen })

		if _, err := Get("unreadable-service", AccountToken); !errors.Is(err, failure) {
			t.Fatalf("Get() error = %v, want %v", err, failure)
		}
		if err := ValidateAuthTokenEntries("unreadable-service"); !errors.Is(err, failure) {
			t.Fatalf("ValidateAuthTokenEntries() error = %v, want %v", err, failure)
		}
	})

	t.Run("inventory enumeration failure", func(t *testing.T) {
		service := "test-service-" + t.Name()
		account := AccountToken + ":enumeration-failure"
		writeRawRegistryString(t, service, account, "fixture")
		t.Cleanup(func() { _ = Remove(service, account) })

		originalReadNames := registryReadValueNames
		failure := windows.ERROR_ACCESS_DENIED
		registryReadValueNames = func(registry.Key, int) ([]string, error) {
			return nil, failure
		}
		t.Cleanup(func() { registryReadValueNames = originalReadNames })

		if err := ValidateAuthTokenEntries(service); !errors.Is(err, failure) {
			t.Fatalf("ValidateAuthTokenEntries() error = %v, want %v", err, failure)
		}
	})

	t.Run("inventory value disappears", func(t *testing.T) {
		service := "test-service-" + t.Name()
		account := AccountToken + ":disappearing"
		unrelated := "app-secret:keeps-key-present"
		writeRawRegistryString(t, service, unrelated, "fixture")
		t.Cleanup(func() { _ = Remove(service, unrelated) })

		originalReadNames := registryReadValueNames
		registryReadValueNames = func(registry.Key, int) ([]string, error) {
			// Model the real race after enumeration: the account name was
			// observed, but GetStringValue no longer finds its value.
			return []string{valueNameForAccount(account)}, nil
		}
		t.Cleanup(func() { registryReadValueNames = originalReadNames })

		err := ValidateAuthTokenEntries(service)
		if err == nil || !strings.Contains(err.Error(), "value disappeared during validation") {
			t.Fatalf("ValidateAuthTokenEntries() error = %v, want disappearance error", err)
		}
	})
}
