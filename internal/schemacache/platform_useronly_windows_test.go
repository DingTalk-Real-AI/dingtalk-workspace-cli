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

//go:build windows && (amd64 || arm64)

package schemacache

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCrossPlatformCoverageUserOnlyOpenSkipsSharedBases covers the Windows
// per-user WithUserOnly open path: shared overrides are bypassed, the
// default-counters leg of Open runs, and the user-cache-dir seam helpers are
// exercisable from this package's own (instrumented) tests.
func TestCrossPlatformCoverageUserOnlyOpenSkipsSharedBases(t *testing.T) {
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", "")

	userBase := privateTestBase(t)
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
