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

//go:build (darwin || linux || windows) && (amd64 || arm64)

package schemacache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCrossPlatformCoverageUserOnlyBypassesSharedOverrides pins the repair
// fallback's open mode: WithUserOnly must ignore DWS_SCHEMA_CACHE_DIR and
// resolve the edition tree under the per-user base only.
func TestCrossPlatformCoverageUserOnlyBypassesSharedOverrides(t *testing.T) {
	sharedBase := privateTestBase(t)
	t.Setenv("DWS_SCHEMA_CACHE_DIR", sharedBase)

	userBase := privateTestBase(t)
	UseUserCacheDirForTest(t, userBase)

	cache, err := Open("open", WithUserOnly())
	if err != nil {
		t.Fatalf("user-only open: %v", err)
	}
	defer func() { _ = cache.Close() }()
	if !strings.HasPrefix(cache.Directory(), filepath.Join(userBase, "dws")) {
		t.Fatalf("user-only directory %q escaped base %q", cache.Directory(), userBase)
	}
	if _, err := os.Stat(filepath.Join(cache.Directory(), "meta.cache")); !os.IsNotExist(err) {
		t.Fatalf("user-only open must not fabricate artifacts: %v", err)
	}
}

// TestCrossPlatformCoverageUserOnlySurfacesBaseErrors covers both
// user-cache-unavailable legs of the user-only open path.
func TestCrossPlatformCoverageUserOnlySurfacesBaseErrors(t *testing.T) {
	t.Run("base lookup fails", func(t *testing.T) {
		t.Setenv("DWS_SCHEMA_CACHE_DIR", privateTestBase(t))
		UseUserCacheDirErrorForTest(t)
		if _, err := Open("open", WithUserOnly()); !errors.Is(err, ErrDisabled) {
			t.Fatalf("user-only lookup error = %v, want ErrDisabled", err)
		}
	})
	t.Run("base is not a directory", func(t *testing.T) {
		t.Setenv("DWS_SCHEMA_CACHE_DIR", privateTestBase(t))
		baseFile := filepath.Join(privateTestBase(t), "not-a-dir")
		if err := os.WriteFile(baseFile, []byte("file"), 0o644); err != nil {
			t.Fatal(err)
		}
		UseUserCacheDirForTest(t, baseFile)
		if _, err := Open("open", WithUserOnly()); err == nil {
			t.Fatal("user-only open on a file base must fail")
		}
	})
}
