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
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
)

func TestCrossPlatformCoverageWindowsStorageDirectoryRegistryIsolation(t *testing.T) {
	t.Setenv(TestNamespaceEnv, "")
	service := "storage-isolation-" + uuid.NewString()
	directories := []string{"", t.TempDir(), t.TempDir()}
	accounts := []string{AccountToken, AccountToken + ":corp:user", "appsecret:app", "client-secret:app", "app-token:app"}
	paths := make([]string, 0, len(directories))
	t.Cleanup(func() {
		// Delete captured test-only paths directly, independent of whichever
		// namespace the last assertion selected. Never touch a real service.
		for i := len(paths) - 1; i >= 0; i-- {
			if err := registry.DeleteKey(registry.CURRENT_USER, paths[i]); err != nil && !errors.Is(err, registry.ErrNotExist) {
				t.Errorf("clean test registry key: %v", err)
			}
		}
	})

	for i, directory := range directories {
		t.Setenv(StorageDirEnv, directory)
		selected := registryPathForService(service)
		for _, previous := range paths {
			if selected == previous {
				t.Fatal("different storage directories selected the same registry key")
			}
		}
		if i == 0 && selected != regRootPath+`\`+service {
			t.Fatal("unset storage directory changed the historical registry key")
		}
		for _, account := range accounts {
			if value, err := Get(service, account); err != nil || value != "" {
				t.Fatalf("fresh namespace inherited account %q: %v", account, err)
			}
		}
		paths = append(paths, selected)
		for _, account := range accounts {
			if err := Set(service, account, "fixture:"+directory+":"+account); err != nil {
				t.Fatal(err)
			}
		}
	}

	// A spelling change must still find the same Windows directory scope.
	t.Setenv(StorageDirEnv, strings.ToUpper(directories[1])+`\child\..\`)
	if registryPathForService(service) != paths[1] {
		t.Fatal("equivalent Windows path selected a different namespace")
	}
	if got := StorageDir(service); got != filepath.Join(strings.ToUpper(directories[1])+`\child\..\`, service) {
		t.Fatalf("storage location = %q", got)
	}
	for _, account := range accounts {
		value, err := Get(service, account)
		if err != nil || value != "fixture:"+directories[1]+":"+account {
			t.Fatalf("equivalent directory lost account %q: %v", account, err)
		}
	}
	// A broken token in B must not block A's login preflight.
	t.Setenv(StorageDirEnv, directories[2])
	writeRawRegistryString(t, service, AccountToken, "invalid-base64!")
	if err := ValidateAuthTokenEntries(service); err == nil {
		t.Fatal("corrupt token was not detected in its own namespace")
	}
	t.Setenv(StorageDirEnv, directories[1])
	if err := ValidateAuthTokenEntries(service); err != nil {
		t.Fatalf("another namespace blocked token validation: %v", err)
	}
	if err := Remove(service, "appsecret:app"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAuthTokenEntries(service); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAccountEntriesWithPrefixes(service, "appsecret:", "client-secret:", "app-token:"); err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if value, err := Get(service, account); err != nil || value != "" {
			t.Fatalf("namespace cleanup retained account %q: %v", account, err)
		}
	}
	for _, i := range []int{0, 2} {
		t.Setenv(StorageDirEnv, directories[i])
		for _, account := range accounts {
			if i == 2 && account == AccountToken {
				if _, err := Get(service, account); err == nil {
					t.Fatal("cleanup in A removed the corrupt token in B")
				}
				continue
			}
			value, err := Get(service, account)
			if err != nil || value != "fixture:"+directories[i]+":"+account {
				t.Fatalf("cleanup in A changed another namespace account %q: %v", account, err)
			}
		}
	}
	// Cleaning the historical default key must not recurse into namespaces.
	t.Setenv(StorageDirEnv, "")
	if err := RemoveAuthTokenEntries(service); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAccountEntriesWithPrefixes(service, "appsecret:", "client-secret:", "app-token:"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(StorageDirEnv, directories[2])
	if value, err := Get(service, "appsecret:app"); err != nil || value != "fixture:"+directories[2]+":appsecret:app" {
		t.Fatalf("default cleanup changed a directory namespace: %v", err)
	}
}

func TestCrossPlatformCoverageWindowsStorageDirectoryProcessReuse(t *testing.T) {
	const helperService = "DWS_TEST_REGISTRY_REUSE_SERVICE"
	const helperDirectory = "DWS_TEST_REGISTRY_REUSE_DIRECTORY"
	const helperOtherDirectory = "DWS_TEST_REGISTRY_REUSE_OTHER_DIRECTORY"
	const account = "auth-token:process-reuse"
	const fixture = "process-reuse-fixture"
	if service := os.Getenv(helperService); service != "" {
		// TestMain has its own safe defaults. The helper explicitly selects
		// the parent's public directory override to model a new CLI invocation.
		t.Setenv(TestNamespaceEnv, "")
		t.Setenv(StorageDirEnv, os.Getenv(helperDirectory))
		if value, err := Get(service, account); err != nil || value != fixture {
			t.Fatalf("new process did not reuse its directory credential: %v", err)
		}
		t.Setenv(StorageDirEnv, os.Getenv(helperOtherDirectory))
		if value, err := Get(service, account); err != nil || value != "" {
			t.Fatalf("new process read another directory's credential: %v", err)
		}
		return
	}

	t.Setenv(TestNamespaceEnv, "")
	directory, otherDirectory := t.TempDir(), t.TempDir()
	t.Setenv(StorageDirEnv, directory)
	service := "process-isolation-" + uuid.NewString()
	keyPath := registryPathForService(service)
	t.Cleanup(func() {
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("clean process test registry key: %v", err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, regRootPath+`\`+service); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("clean process test service key: %v", err)
		}
	})
	if err := Set(service, account, fixture); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperService, service)
	t.Setenv(helperDirectory, directory)
	t.Setenv(helperOtherDirectory, otherDirectory)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrossPlatformCoverageWindowsStorageDirectoryProcessReuse$", "-test.count=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("credential reader subprocess failed: %v\n%s", err, output)
	}
}
