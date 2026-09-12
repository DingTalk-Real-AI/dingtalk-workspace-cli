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

//go:build (darwin || linux) && (amd64 || arm64)

package schemacache

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCrossPlatformCoverageUserOnlyOpenSkipsSharedBases covers the per-user
// WithUserOnly open path end to end: shared overrides must be bypassed, the
// default-counters leg of Open must run, and the two user-cache-dir seam
// helpers must be exercisable from this package's own (instrumented) tests.
func TestCrossPlatformCoverageUserOnlyOpenSkipsSharedBases(t *testing.T) {
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Cache bases must sit behind symlink-free ancestry (the walk opens each
	// component with O_NOFOLLOW and t.TempDir() on macOS sits behind /var).
	userBase, err := os.MkdirTemp(home, ".dws-useronly-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(userBase) })
	UseUserCacheDirForTest(t, userBase)

	digest, err := EditionSHA256("useronly")
	if err != nil {
		t.Fatal(err)
	}
	cache, err := Open("useronly", WithUserOnly())
	if err != nil {
		t.Fatalf("user-only open: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	want := filepath.Join(userBase, "dws", "schema", hex.EncodeToString(digest[:]), "v1")
	if got := cache.Directory(); got != want {
		t.Fatalf("user-only directory = %q want %q", got, want)
	}
	if _, statErr := os.Stat(cache.Directory()); statErr != nil {
		t.Fatalf("user-only cache directory missing: %v", statErr)
	}

	UseUserCacheDirErrorForTest(t)
	if _, err := Open("useronly", WithUserOnly()); err == nil {
		t.Fatal("user cache dir error must fail the user-only open")
	}

	notADir := filepath.Join(userBase, "not-a-dir")
	if err := os.WriteFile(notADir, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	UseUserCacheDirForTest(t, notADir)
	if _, err := Open("useronly", WithUserOnly()); err == nil || !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("non-directory user base must fail unsafe: %v", err)
	}
}
