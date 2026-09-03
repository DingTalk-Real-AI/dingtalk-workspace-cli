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

func seedUpgradeSkill(t *testing.T, dir, content string, managed bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = managed
}

func assertUpgradeSkillContent(t *testing.T, dir, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", dir, got, want)
	}
}

func assertNoUpgradeStaging(t *testing.T, base string) {
	t.Helper()
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".dws-upgrade-") {
			t.Fatalf("staging directory leaked after failed transaction: %s", filepath.Join(base, entry.Name()))
		}
	}
}

func seedMultiUpgradeTarget(t *testing.T) (home, base, multiRoot string, skills []string, skillSet map[string]bool) {
	t.Helper()
	home = t.TempDir()
	base = filepath.Join(home, ".agents", "skills")
	seedUpgradeSkill(t, filepath.Join(base, "dws"), "old mono", false)
	seedUpgradeSkill(t, filepath.Join(base, "dingtalk-a"), "old a", true)
	seedUpgradeSkill(t, filepath.Join(base, "dingtalk-b"), "old b", true)
	seedUpgradeSkill(t, filepath.Join(base, "dingtalk-stale"), "old stale", true)
	multiRoot = writeMultiBundle(t, t.TempDir(), "dingtalk-a", "dingtalk-b")
	skills = []string{"dingtalk-a", "dingtalk-b"}
	skillSet = map[string]bool{"dingtalk-a": true, "dingtalk-b": true}
	useUpgradeManagedNames(t, "dingtalk-a", "dingtalk-b", "dingtalk-stale")
	return home, base, multiRoot, skills, skillSet
}

func assertOriginalMultiUpgradeTarget(t *testing.T, base string) {
	t.Helper()
	assertUpgradeSkillContent(t, filepath.Join(base, "dws"), "old mono")
	assertUpgradeSkillContent(t, filepath.Join(base, "dingtalk-a"), "old a")
	assertUpgradeSkillContent(t, filepath.Join(base, "dingtalk-b"), "old b")
	assertUpgradeSkillContent(t, filepath.Join(base, "dingtalk-stale"), "old stale")
	assertNoUpgradeStaging(t, base)
}

func TestCrossPlatformCoverageMultiUpgradeTransactionPreservesOldSet(t *testing.T) {
	failure := errors.New("injected transaction failure")

	t.Run("copy failure before backup", func(t *testing.T) {
		home, base, multiRoot, skills, skillSet := seedMultiUpgradeTarget(t)
		originalCopy := upgradeCopyDir
		testseam.Swap(t, &upgradeCopyDir, func(src, dest string) error {
			if filepath.Base(src) == "dingtalk-b" {
				return failure
			}
			return originalCopy(src, dest)
		})
		if err := publishMultiUpgradeTarget(home, base, multiRoot, skills, skillSet); !errors.Is(err, failure) {
			t.Fatalf("copy failure = %v, want injected failure", err)
		}
		assertOriginalMultiUpgradeTarget(t, base)
	})

	t.Run("backup failure restores earlier victims", func(t *testing.T) {
		home, base, multiRoot, skills, skillSet := seedMultiUpgradeTarget(t)
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(src, dest string) (string, error) {
			if src == filepath.Join(base, "dingtalk-b") {
				return "", failure
			}
			return originalRename(src, dest)
		})
		if err := publishMultiUpgradeTarget(home, base, multiRoot, skills, skillSet); !errors.Is(err, failure) {
			t.Fatalf("backup failure = %v, want injected failure", err)
		}
		assertOriginalMultiUpgradeTarget(t, base)
	})

	t.Run("publish failure rolls back complete set", func(t *testing.T) {
		home, base, multiRoot, skills, skillSet := seedMultiUpgradeTarget(t)
		originalPublish := upgradePublishSkillPath
		failed := false
		testseam.Swap(t, &upgradePublishSkillPath, func(src, dest string) (SkillPathPublication, error) {
			if !failed && strings.Contains(src, ".dws-upgrade-multi-") && filepath.Base(dest) == "dingtalk-b" {
				failed = true
				return SkillPathPublication{}, failure
			}
			return originalPublish(src, dest)
		})
		if err := publishMultiUpgradeTarget(home, base, multiRoot, skills, skillSet); !errors.Is(err, failure) {
			t.Fatalf("publish failure = %v, want injected failure", err)
		}
		assertOriginalMultiUpgradeTarget(t, base)
	})
}

func TestCrossPlatformCoverageMonoUpgradeTransactionPreservesOldSet(t *testing.T) {
	failure := errors.New("injected mono transaction failure")
	seed := func(t *testing.T) (home, base, monoRoot string) {
		t.Helper()
		home = t.TempDir()
		base = filepath.Join(home, ".agents", "skills")
		seedUpgradeSkill(t, filepath.Join(base, "dws"), "old mono", false)
		seedUpgradeSkill(t, filepath.Join(base, "dingtalk-a"), "old multi", true)
		useUpgradeManagedNames(t, "dingtalk-a")
		monoRoot = t.TempDir()
		if err := os.WriteFile(filepath.Join(monoRoot, "SKILL.md"), []byte("new mono"), 0o644); err != nil {
			t.Fatal(err)
		}
		return home, base, monoRoot
	}
	assertOld := func(t *testing.T, base string) {
		t.Helper()
		assertUpgradeSkillContent(t, filepath.Join(base, "dws"), "old mono")
		assertUpgradeSkillContent(t, filepath.Join(base, "dingtalk-a"), "old multi")
		assertNoUpgradeStaging(t, base)
	}

	t.Run("copy failure before backup", func(t *testing.T) {
		home, base, monoRoot := seed(t)
		testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return failure })
		if err := publishMonoUpgradeTarget(home, base, monoRoot); !errors.Is(err, failure) {
			t.Fatalf("copy failure = %v, want injected failure", err)
		}
		assertOld(t, base)
	})

	t.Run("publish failure rolls back mono and multi", func(t *testing.T) {
		home, base, monoRoot := seed(t)
		originalPublish := upgradePublishSkillPath
		failed := false
		testseam.Swap(t, &upgradePublishSkillPath, func(src, dest string) (SkillPathPublication, error) {
			if !failed && strings.Contains(src, ".dws-upgrade-mono-") && filepath.Base(dest) == "dws" {
				failed = true
				return SkillPathPublication{}, failure
			}
			return originalPublish(src, dest)
		})
		if err := publishMonoUpgradeTarget(home, base, monoRoot); !errors.Is(err, failure) {
			t.Fatalf("publish failure = %v, want injected failure", err)
		}
		assertOld(t, base)
	})
}

func TestCrossPlatformCoverageSkillPublishTransactionFailureEdges(t *testing.T) {
	failure := errors.New("injected failure")
	restoreFailure := errors.New("injected restore failure")

	t.Run("stage setup and cleanup failures", func(t *testing.T) {
		destBase := filepath.Join(t.TempDir(), "skills")
		testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return failure })
		if _, _, err := stageSkillSet(destBase, ".stage-", nil); !errors.Is(err, failure) {
			t.Fatalf("stage parent failure = %v", err)
		}
	})

	t.Run("stage temp failure", func(t *testing.T) {
		testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return "", failure })
		if _, _, err := stageSkillSet(t.TempDir(), ".stage-", nil); !errors.Is(err, failure) {
			t.Fatalf("stage temp failure = %v", err)
		}
	})

	t.Run("stage cleanup failure joins copy error", func(t *testing.T) {
		destBase := t.TempDir()
		src := t.TempDir()
		testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return failure })
		testseam.Swap(t, &upgradeRemoveAll, func(string) error { return restoreFailure })
		_, _, err := stageSkillSet(destBase, ".stage-", []skillStageSpec{{src: src, dest: filepath.Join(destBase, "dws")}})
		if !errors.Is(err, failure) || !errors.Is(err, restoreFailure) {
			t.Fatalf("joined stage cleanup error = %v", err)
		}
	})

	t.Run("duplicate victims are removed once", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "dws")
		got := uniqueSkillDirs([]string{path, path})
		if len(got) != 1 || got[0] != path {
			t.Fatalf("uniqueSkillDirs = %v", got)
		}
	})

	t.Run("restore reports removal and directory failures", func(t *testing.T) {
		testseam.Swap(t, &upgradeRollbackPublishedSkillPaths, func([]SkillPathPublication) error { return failure })
		testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return restoreFailure })
		err := restoreSkillSet(
			[]SkillPathPublication{{Destination: filepath.Join(t.TempDir(), "published")}},
			[]backedUpSkillDir{{original: filepath.Join(t.TempDir(), "old"), backup: filepath.Join(t.TempDir(), "backup")}},
		)
		if !errors.Is(err, failure) || !errors.Is(err, restoreFailure) {
			t.Fatalf("restore removal/directory error = %v", err)
		}
	})

	t.Run("restore reports rename failure", func(t *testing.T) {
		testseam.Swap(t, &skillPathRenameNoReplace, func(string, string) (string, error) { return "", restoreFailure })
		err := restoreSkillSet(nil, []backedUpSkillDir{{original: filepath.Join(t.TempDir(), "old"), backup: filepath.Join(t.TempDir(), "backup")}})
		if !errors.Is(err, restoreFailure) {
			t.Fatalf("restore rename error = %v", err)
		}
	})

	t.Run("backup failure joins restore failure", func(t *testing.T) {
		home := t.TempDir()
		first := filepath.Join(home, "first")
		second := filepath.Join(home, "second")
		seedUpgradeSkill(t, first, "first", false)
		seedUpgradeSkill(t, second, "second", false)
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(src, dest string) (string, error) {
			switch {
			case src == second:
				return "", failure
			case strings.Contains(filepath.ToSlash(src), skillBackupSubdir):
				return "", restoreFailure
			default:
				return originalRename(src, dest)
			}
		})
		_, err := backupSkillSet(home, []string{first, second})
		if !errors.Is(err, failure) || !errors.Is(err, restoreFailure) {
			t.Fatalf("backup/restore error = %v", err)
		}
	})

	t.Run("publish failure joins restore failure", func(t *testing.T) {
		home := t.TempDir()
		old := filepath.Join(home, "skills", "dws")
		staged := filepath.Join(home, "skills", ".stage", "dws")
		seedUpgradeSkill(t, old, "old", false)
		seedUpgradeSkill(t, staged, "new", false)
		testseam.Swap(t, &upgradePublishSkillPath, func(src, dest string) (SkillPathPublication, error) {
			return SkillPathPublication{}, failure
		})
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(src, dest string) (string, error) {
			if strings.Contains(filepath.ToSlash(src), skillBackupSubdir) {
				return "", restoreFailure
			}
			return originalRename(src, dest)
		})
		err := publishStagedSkillSet(home, []stagedSkillDir{{staged: staged, dest: old}}, []string{old})
		if !errors.Is(err, failure) || !errors.Is(err, restoreFailure) {
			t.Fatalf("publish/restore error = %v", err)
		}
	})

	t.Run("victim scans fail closed", func(t *testing.T) {
		testseam.Swap(t, &upgradeReadDir, func(string) ([]os.DirEntry, error) { return nil, failure })
		base := t.TempDir()
		if _, err := monoUpgradeVictims(base, filepath.Join(base, "dws")); !errors.Is(err, failure) {
			t.Fatalf("mono victim scan error = %v", err)
		}
		if _, err := multiUpgradeVictims(base, map[string]bool{}, []string{"dingtalk-a"}); !errors.Is(err, failure) {
			t.Fatalf("multi victim scan error = %v", err)
		}
		if err := publishMultiUpgradeTarget(base, base, base, []string{"dingtalk-a"}, map[string]bool{}); !errors.Is(err, failure) {
			t.Fatalf("multi target victim scan error = %v", err)
		}
	})
}
