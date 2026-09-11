//go:build windows && (amd64 || arm64)

package schemacache

import (
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
