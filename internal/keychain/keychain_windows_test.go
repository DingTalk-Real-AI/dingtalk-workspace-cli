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
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestCrossPlatformCoverageRegistryPathForServiceHonorsTestNamespace(t *testing.T) {
	t.Setenv(StorageDirEnv, "")
	t.Setenv(TestNamespaceEnv, "")
	defaultPath := registryPathForService("service")
	if defaultPath != regRootPath+`\service` {
		t.Fatalf("default registry path = %q, want historical path %q", defaultPath, regRootPath+`\service`)
	}

	t.Setenv(TestNamespaceEnv, t.TempDir())
	firstPath := registryPathForService("service")

	t.Setenv(TestNamespaceEnv, t.TempDir())
	secondPath := registryPathForService("service")

	if firstPath == defaultPath || secondPath == defaultPath {
		t.Fatalf("isolated registry paths = %q, %q; want paths distinct from %q", firstPath, secondPath, defaultPath)
	}
	if firstPath == secondPath {
		t.Fatalf("isolated registry paths collide: %q", firstPath)
	}
	if !strings.HasPrefix(firstPath, defaultPath+`\test-`) {
		t.Fatalf("isolated registry path = %q, want prefix %q", firstPath, defaultPath+`\test-`)
	}
}

func TestCrossPlatformCoverageWindowsRegistryPathHonorsStorageDir(t *testing.T) {
	t.Setenv(TestNamespaceEnv, "")
	t.Setenv(StorageDirEnv, "")
	defaultPath := registryPathForService(Service)

	firstDir, secondDir := t.TempDir(), t.TempDir()
	t.Setenv(StorageDirEnv, firstDir)
	firstPath := registryPathForService(Service)
	t.Setenv(StorageDirEnv, secondDir)
	secondPath := registryPathForService(Service)
	if firstPath == secondPath {
		t.Fatalf("different storage directories share registry path %q", firstPath)
	}
	for _, path := range []string{firstPath, secondPath} {
		if !strings.HasPrefix(path, defaultPath+`\dir-`) {
			t.Fatalf("registry path = %q, want isolated child of %q", path, defaultPath)
		}
	}

	t.Chdir(firstDir)
	for _, variant := range []string{
		firstDir,
		strings.ToUpper(firstDir),
		strings.ReplaceAll(firstDir, `\`, "/"),
		firstDir + `\.\child\..`,
		firstDir + `\`,
		".",
	} {
		t.Setenv(StorageDirEnv, variant)
		if got := registryPathForService(Service); got != firstPath {
			t.Errorf("storage directory %q resolved to %q, want %q", variant, got, firstPath)
		}
	}
}

func TestCrossPlatformCoverageWindowsRegistryNamespacesCompose(t *testing.T) {
	root := t.TempDir()
	t.Setenv(StorageDirEnv, "")
	t.Setenv(TestNamespaceEnv, root)
	testPath := registryPathForService(Service)

	t.Setenv(TestNamespaceEnv, "")
	t.Setenv(StorageDirEnv, root)
	storagePath := registryPathForService(Service)
	if storagePath == testPath {
		t.Fatalf("test and storage namespaces collide at %q", storagePath)
	}

	t.Setenv(TestNamespaceEnv, root)
	combinedPath := registryPathForService(Service)
	if !strings.HasPrefix(combinedPath, testPath+`\dir-`) {
		t.Fatalf("combined path = %q, want storage child of %q", combinedPath, testPath)
	}
	t.Setenv(StorageDirEnv, t.TempDir())
	if got := registryPathForService(Service); got == combinedPath {
		t.Fatalf("test namespace masked storage directory change at %q", got)
	}
	t.Setenv(StorageDirEnv, root)
	t.Setenv(TestNamespaceEnv, t.TempDir())
	if got := registryPathForService(Service); got == combinedPath {
		t.Fatalf("storage directory masked test namespace change at %q", got)
	}
}

func TestCrossPlatformCoverageWindowsRegistryStorageDirIsolatesCredentials(t *testing.T) {
	t.Setenv(TestNamespaceEnv, t.TempDir())
	t.Setenv(StorageDirEnv, "")
	testKeyPath := registryPathForService(Service)
	t.Cleanup(func() {
		if err := registry.DeleteKey(registry.CURRENT_USER, testKeyPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("cleanup test registry namespace: %v", err)
		}
	})
	dirs := []string{t.TempDir(), t.TempDir()}
	accounts := []string{
		AccountToken,
		AccountToken + ":corp-a",
		AccountToken + ":id:0123456789abcdef",
		"appsecret:client-a",
		"client-secret:client-a",
		"app-token:client-a",
	}
	values := []string{"fixture-a", "fixture-b"}
	for i, dir := range dirs {
		t.Setenv(StorageDirEnv, dir)
		keyPath := registryPathForService(Service)
		t.Cleanup(func() {
			if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
				t.Errorf("cleanup registry key: %v", err)
			}
		})
		for _, account := range accounts {
			if got, err := Get(Service, account); err != nil || got != "" {
				t.Fatalf("new directory account %q = %q, %v; want missing", account, got, err)
			}
			if err := Set(Service, account, values[i]); err != nil {
				t.Fatalf("Set(%q): %v", account, err)
			}
		}
	}

	for i, dir := range dirs {
		t.Setenv(StorageDirEnv, dir)
		for _, account := range accounts {
			if got, err := Get(Service, account); err != nil || got != values[i] {
				t.Fatalf("directory %d account %q = %q, %v; want %q", i, account, got, err, values[i])
			}
		}
	}

	t.Setenv(StorageDirEnv, dirs[0])
	if err := Set(Service, AccountToken, "fixture-updated"); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(Service, AccountToken); err != nil || got != "fixture-updated" {
		t.Fatalf("overwrite = %q, %v; want updated fixture", got, err)
	}
	if err := Remove(Service, accounts[2]); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(Service, accounts[2]); err != nil || got != "" {
		t.Fatalf("removed identity token = %q, %v; want missing", got, err)
	}
	if err := RemoveAccountEntriesWithPrefixes(Service, "appsecret:", "client-secret:", "app-token:"); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(Service, AccountToken); err != nil || got != "fixture-updated" {
		t.Fatalf("token after application cleanup = %q, %v; want preserved", got, err)
	}
	if err := RemoveAuthTokenEntries(Service); err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if got, err := Get(Service, account); err != nil || got != "" {
			t.Fatalf("cleaned account %q = %q, %v; want missing", account, got, err)
		}
	}

	writeRawRegistryString(t, Service, AccountToken+":orphan", "%%%invalid-ciphertext%%")
	if err := ValidateAuthTokenEntries(Service); err == nil {
		t.Fatal("corrupt token inventory passed validation")
	}
	t.Setenv(StorageDirEnv, dirs[1])
	if err := ValidateAuthTokenEntries(Service); err != nil {
		t.Fatalf("other directory corruption affected validation: %v", err)
	}
	for _, account := range accounts {
		if got, err := Get(Service, account); err != nil || got != values[1] {
			t.Fatalf("other directory account %q = %q, %v; want preserved", account, got, err)
		}
	}
}

func TestDeleteRegistryValuePropagatesFailure(t *testing.T) {
	originalDelete := registryDeleteValue
	failure := errors.New("delete failed")
	registryDeleteValue = func(registry.Key, string) error { return failure }
	t.Cleanup(func() { registryDeleteValue = originalDelete })

	if err := deleteRegistryValue(0, "auth-token"); !errors.Is(err, failure) {
		t.Fatalf("deleteRegistryValue() error = %v, want %v", err, failure)
	}
}

func TestRegistryRemoveAuthTokenEntriesPropagatesOpenFailure(t *testing.T) {
	originalOpen := registryOpenDeleteKey
	failure := windows.ERROR_ACCESS_DENIED
	registryOpenDeleteKey = func(string, uint32) (registry.Key, error) {
		return 0, failure
	}
	t.Cleanup(func() { registryOpenDeleteKey = originalOpen })

	if err := registryRemoveAuthTokenEntries("service"); !errors.Is(err, failure) {
		t.Fatalf("registryRemoveAuthTokenEntries() error = %v, want %v", err, failure)
	}
}

func TestCrossPlatformCoverageWindowsPrefixCleanupFailureEdges(t *testing.T) {
	originalOpen := registryOpenDeleteKey
	t.Cleanup(func() { registryOpenDeleteKey = originalOpen })
	failure := windows.ERROR_ACCESS_DENIED
	registryOpenDeleteKey = func(string, uint32) (registry.Key, error) { return 0, failure }
	if err := registryRemoveAccountEntriesWithPrefixes("service", []string{"appsecret:"}); !errors.Is(err, failure) {
		t.Fatalf("prefix cleanup open failure = %v", err)
	}
	registryOpenDeleteKey = originalOpen
	if err := registryRemoveAccountEntriesWithPrefixes("missing-"+t.Name(), []string{"appsecret:"}); err != nil {
		t.Fatalf("missing prefix registry = %v", err)
	}

	service := "prefix-failures-" + t.Name()
	writeRawRegistryString(t, service, "appsecret:id", "value")
	writeRawRegistryNamedString(t, service, "%%%invalid-account%%%", "value")
	t.Cleanup(func() { _ = Remove(service, "appsecret:id") })
	keyPath := registryPathForService(service)

	registryOpenDeleteKey = func(string, uint32) (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	}
	if err := registryRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); err == nil {
		t.Fatal("prefix cleanup list failure succeeded")
	}

	registryOpenDeleteKey = func(string, uint32) (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	}
	if err := registryRemoveAccountEntriesWithPrefixes(service, []string{"", "appsecret:"}); err == nil {
		t.Fatal("prefix cleanup delete failure succeeded")
	}
}

func TestCrossPlatformCoverageWindowsPrefixCleanupSkipsMalformedAccountNames(t *testing.T) {
	t.Setenv(TestNamespaceEnv, t.TempDir())
	service := "prefix-malformed-" + t.Name()
	keyPath := registryPathForService(service)
	const malformedName = "%%%invalid-account%%%"
	const matchingAccount = "appsecret:client"
	const unrelatedAccount = "auth-token:corp"

	writeRawRegistryNamedString(t, service, malformedName, "malformed")
	writeRawRegistryString(t, service, matchingAccount, "matching")
	writeRawRegistryString(t, service, unrelatedAccount, "unrelated")
	t.Cleanup(func() {
		k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return
		}
		if names, readErr := k.ReadValueNames(-1); readErr == nil {
			for _, name := range names {
				_ = k.DeleteValue(name)
			}
		}
		_ = k.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
	})

	if err := registryRemoveAccountEntriesWithPrefixes(service, []string{"appsecret:"}); err != nil {
		t.Fatalf("prefix cleanup = %v", err)
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open cleaned registry = %v", err)
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		t.Fatalf("list cleaned registry = %v", err)
	}
	present := make(map[string]bool, len(names))
	for _, name := range names {
		present[name] = true
	}
	if present[valueNameForAccount(matchingAccount)] {
		t.Fatal("matching account was not removed")
	}
	if !present[malformedName] {
		t.Fatal("malformed account name was not skipped")
	}
	if !present[valueNameForAccount(unrelatedAccount)] {
		t.Fatal("unrelated account was removed")
	}
}
