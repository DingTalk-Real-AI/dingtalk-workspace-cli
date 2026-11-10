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
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageRegistryNamespaceKeepsDefaultAndTestPaths(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	if got := registryNamespacePath(base, "", " \t"); got != base {
		t.Fatalf("default registry path = %q, want %q", got, base)
	}

	// Pin the pre-existing test namespace layout independently of the
	// implementation, so upgrades cannot strand test credentials.
	testDir := t.TempDir()
	absolute, err := filepath.Abs(filepath.Clean(testDir))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(absolute)))
	testPath := fmt.Sprintf(`%s\test-%x`, base, sum[:16])
	if got := registryNamespacePath(base, "", " "+testDir+" "); got != testPath {
		t.Fatalf("test registry path = %q, want %q", got, testPath)
	}

	// A storage directory nests below the test namespace instead of escaping it.
	nested := regexp.MustCompile(`^` + regexp.QuoteMeta(testPath) + `\\dir-[0-9a-f]{32}$`)
	if got := registryNamespacePath(base, t.TempDir(), testDir); !nested.MatchString(got) {
		t.Fatalf("combined registry path = %q, want storage child of %q", got, testPath)
	}
}

func TestCrossPlatformCoverageRegistryNamespaceSeparatesStorageDirectories(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	firstDir, secondDir := t.TempDir(), t.TempDir()
	first := registryNamespacePath(base, firstDir, "")
	second := registryNamespacePath(base, secondDir, "")
	if first == second || first == base || second == base {
		t.Fatalf("storage directories share registry paths: %q, %q", first, second)
	}
	for _, got := range []string{first, second} {
		if !regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\\dir-[0-9a-f]{32}$`).MatchString(got) {
			t.Fatalf("registry path must contain only a directory digest below the service: %q", got)
		}
	}
	if got := registryNamespacePath(base, firstDir, firstDir); got == first {
		t.Fatal("production and test namespace domains overlap")
	}
	if got := registryNamespacePath(base+"-other", firstDir, ""); got == first {
		t.Fatal("different services share a registry path")
	}
	if got := registryNamespacePath(base, " \t", ""); got == base {
		t.Fatal("whitespace-only storage directory collapsed into the shared default key")
	}
}

func TestCrossPlatformCoverageRegistryNamespaceComposesTestAndStorageScopes(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	root := t.TempDir()
	testOnly := registryNamespacePath(base, "", root)
	storageOnly := registryNamespacePath(base, root, "")
	combined := registryNamespacePath(base, root, root)
	if testOnly == storageOnly || combined == testOnly || combined == storageOnly {
		t.Fatalf("namespaces collide: test %q, storage %q, combined %q", testOnly, storageOnly, combined)
	}
	if !strings.HasPrefix(combined, testOnly+`\dir-`) {
		t.Fatalf("combined path = %q, want storage child of %q", combined, testOnly)
	}
	if got := registryNamespacePath(base, t.TempDir(), root); got == combined {
		t.Fatalf("test namespace masked storage directory change at %q", got)
	}
	if got := registryNamespacePath(base, root, t.TempDir()); got == combined {
		t.Fatalf("storage directory masked test namespace change at %q", got)
	}
}

func TestCrossPlatformCoverageRegistryNamespaceNormalizesDirectory(t *testing.T) {
	const base = `Software\DwsCli\keychain\dws-cli`
	dir := t.TempDir()
	want := registryNamespacePath(base, dir, "")
	for _, spelling := range []string{
		dir + string(filepath.Separator),
		dir + string(filepath.Separator) + "child" + string(filepath.Separator) + "..",
		strings.ToUpper(dir),
	} {
		if got := registryNamespacePath(base, spelling, ""); got != want {
			t.Fatalf("equivalent directory %q selected %q, want %q", spelling, got, want)
		}
	}
	t.Chdir(dir)
	if got := registryNamespacePath(base, ".", ""); got != want {
		t.Fatalf("relative directory selected %q, want %q", got, want)
	}
}
