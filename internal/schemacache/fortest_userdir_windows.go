//go:build windows && (amd64 || arm64)

package schemacache

import (
	"fmt"
	"path/filepath"
	"testing"
)

// UseUserCacheDirForTest points the per-user cache base at dir for the test.
// Production must not call this; the ForTest suffix is the boundary.
func UseUserCacheDirForTest(t *testing.T, dir string) {
	t.Helper()
	previous := userCacheDir
	userCacheDir = func() (string, error) { return filepath.Clean(dir), nil }
	t.Cleanup(func() { userCacheDir = previous })
}

// UseUserCacheDirErrorForTest makes the per-user cache base lookup fail for
// the test (covers the fallback's user-cache-unavailable error leg).
func UseUserCacheDirErrorForTest(t *testing.T) {
	t.Helper()
	previous := userCacheDir
	userCacheDir = func() (string, error) { return "", errUserCacheDirForTest }
	t.Cleanup(func() { userCacheDir = previous })
}

var errUserCacheDirForTest = fmt.Errorf("user cache dir unavailable (test)")
