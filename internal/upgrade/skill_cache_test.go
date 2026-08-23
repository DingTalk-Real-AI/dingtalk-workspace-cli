// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func writeSkillCacheFixture(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSkillCacheFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read cache fixture: %v", err)
	}
	return string(data)
}

// TestCrossPlatformCoverageRefreshSkillCacheTransaction pins the cache refresh
// transaction on every platform: copying is staged before the old cache moves,
// and a publish failure restores the prior usable cache.
func TestCrossPlatformCoverageRefreshSkillCacheTransaction(t *testing.T) {
	newSource := func(t *testing.T) string {
		t.Helper()
		src := t.TempDir()
		writeSkillCacheFixture(t, src, "new")
		return src
	}
	cachePath := func(home string) string {
		return filepath.Join(home, ".dws", "skills", "multi")
	}
	assertOldCache := func(t *testing.T, home string) {
		t.Helper()
		if got := readSkillCacheFixture(t, cachePath(home)); got != "old" {
			t.Fatalf("cache content = %q, want old", got)
		}
	}

	t.Run("replace existing cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		if err := refreshSkillCache(home, "multi", newSource(t)); err != nil {
			t.Fatalf("refreshSkillCache() error = %v", err)
		}
		if got := readSkillCacheFixture(t, cachePath(home)); got != "new" {
			t.Fatalf("cache content = %q, want new", got)
		}
		entries, err := os.ReadDir(filepath.Dir(cachePath(home)))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "multi" {
			t.Fatalf("cache parent entries = %v, want only multi", entries)
		}
	})

	t.Run("publish new cache", func(t *testing.T) {
		home := t.TempDir()
		if err := refreshSkillCache(home, "multi", newSource(t)); err != nil {
			t.Fatalf("refreshSkillCache() error = %v", err)
		}
		if got := readSkillCacheFixture(t, cachePath(home)); got != "new" {
			t.Fatalf("cache content = %q, want new", got)
		}
	})

	t.Run("parent creation failure", func(t *testing.T) {
		home := t.TempDir()
		failure := errors.New("mkdir denied")
		testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
	})

	t.Run("staging creation failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("temp denied")
		testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return "", failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("copy failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("copy denied")
		testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("cache stat failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("stat denied")
		testseam.Swap(t, &upgradeStat, func(string) (os.FileInfo, error) { return nil, failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("new cache publish failure cleans staging", func(t *testing.T) {
		home := t.TempDir()
		failure := errors.New("rename denied")
		testseam.Swap(t, &upgradeRename, func(string, string) error { return failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		entries, err := os.ReadDir(filepath.Dir(cachePath(home)))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("cache parent entries = %v, want empty", entries)
		}
	})

	t.Run("rollback staging creation failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("rollback temp denied")
		original := upgradeMkdirTemp
		calls := 0
		testseam.Swap(t, &upgradeMkdirTemp, func(dir, pattern string) (string, error) {
			calls++
			if calls == 2 {
				return "", failure
			}
			return original(dir, pattern)
		})
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("rollback preparation failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("remove denied")
		testseam.Swap(t, &upgradeRemoveAll, func(string) error { return failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("old cache move failure preserves old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("old move denied")
		testseam.Swap(t, &upgradeRename, func(string, string) error { return failure })
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("publish failure restores old cache", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		failure := errors.New("publish denied")
		original := upgradeRename
		calls := 0
		testseam.Swap(t, &upgradeRename, func(src, dst string) error {
			calls++
			if calls == 2 {
				return failure
			}
			return original(src, dst)
		})
		if err := refreshSkillCache(home, "multi", newSource(t)); !errors.Is(err, failure) {
			t.Fatalf("refreshSkillCache() error = %v, want %v", err, failure)
		}
		assertOldCache(t, home)
	})

	t.Run("restore failure reports both errors and retains backup", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		publishFailure := errors.New("publish denied")
		restoreFailure := errors.New("restore denied")
		original := upgradeRename
		calls := 0
		testseam.Swap(t, &upgradeRename, func(src, dst string) error {
			calls++
			switch calls {
			case 2:
				return publishFailure
			case 3:
				return restoreFailure
			default:
				return original(src, dst)
			}
		})
		err := refreshSkillCache(home, "multi", newSource(t))
		if !errors.Is(err, publishFailure) || !strings.Contains(err.Error(), restoreFailure.Error()) {
			t.Fatalf("refreshSkillCache() error = %v, want publish and restore errors", err)
		}
		matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(cachePath(home)), ".multi.old-*", "SKILL.md"))
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("rollback cache matches = %v, err = %v", matches, globErr)
		}
		if got := readSkillCacheFixture(t, filepath.Dir(matches[0])); got != "old" {
			t.Fatalf("rollback cache content = %q, want old", got)
		}
	})

	t.Run("post publish cleanup is best effort", func(t *testing.T) {
		home := t.TempDir()
		writeSkillCacheFixture(t, cachePath(home), "old")
		original := upgradeRemoveAll
		calls := 0
		testseam.Swap(t, &upgradeRemoveAll, func(path string) error {
			calls++
			if calls == 2 {
				return errors.New("cleanup denied")
			}
			return original(path)
		})
		if err := refreshSkillCache(home, "multi", newSource(t)); err != nil {
			t.Fatalf("refreshSkillCache() error = %v", err)
		}
		if got := readSkillCacheFixture(t, cachePath(home)); got != "new" {
			t.Fatalf("cache content = %q, want new", got)
		}
	})
}
