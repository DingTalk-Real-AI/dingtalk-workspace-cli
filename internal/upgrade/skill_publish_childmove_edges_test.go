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

// childMoveDirEntry fakes one directory entry for seam-driven claim scans.
type childMoveDirEntry struct {
	name  string
	isDir bool
}

func (entry childMoveDirEntry) Name() string               { return entry.name }
func (entry childMoveDirEntry) IsDir() bool                { return entry.isDir }
func (entry childMoveDirEntry) Type() os.FileMode          { return entry.info().Mode().Type() }
func (entry childMoveDirEntry) Info() (os.FileInfo, error) { return entry.info(), nil }
func (entry childMoveDirEntry) info() os.FileInfo {
	if entry.isDir {
		return skillPathFakeInfo{mode: os.ModeDir | 0o755}
	}
	return skillPathFakeInfo{mode: 0o644}
}

// forceChildMove routes every directory publication through the child-move
// slow path: the atomic primitive reports the flag unsupported and the
// fast-path rename fails the way platforms that refuse renaming over any
// existing directory do.
func forceChildMove(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
	testseam.Swap(t, &skillPathRename, func(string, string) error { return os.ErrExist })
}

// TestCrossPlatformCoverageChildMoveEdges pins the error and dispatch
// branches of the child-move directory fallback on every platform: the
// platform coverage gates only execute TestCrossPlatformCoverage-named
// tests, so these branches must be driven through seams rather than the
// platform-specific filesystem behaviors that cover them incidentally
// elsewhere.
func TestCrossPlatformCoverageChildMoveEdges(t *testing.T) {

	t.Run("directory source stat failure surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		original := skillPathLstat
		calls := 0
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == source {
				calls++
				if calls > 1 {
					return nil, os.ErrPermission
				}
			}
			return original(path)
		})
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("directory stat failure = %v", err)
		}
	})

	t.Run("claim read failure surfaces and reports the cleanup check", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == destination {
				return nil, os.ErrPermission
			}
			return os.ReadDir(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "降级发布 Skill 目录失败") || !strings.Contains(err.Error(), "检查降级发布目标") {
			t.Fatalf("claim read failure = %v", err)
		}
	})

	t.Run("claim vanished during cleanup is tolerated", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		destinationReads := 0
		original := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == destination {
				destinationReads++
				if destinationReads == 1 {
					return nil, os.ErrPermission
				}
				return nil, os.ErrNotExist
			}
			return original(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "降级发布 Skill 目录失败") || strings.Contains(err.Error(), "检查降级发布目标") {
			t.Fatalf("vanished claim cleanup = %v", err)
		}
	})

	t.Run("file child link failure surfaces and the empty claim cleanup reports", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		testseam.Swap(t, &skillPathLink, func(string, string) error { return os.ErrPermission })
		testseam.Swap(t, &skillPathRemove, func(path string) error {
			if path == destination {
				return os.ErrPermission
			}
			return os.Remove(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "发布 Skill 文件失败") || !strings.Contains(err.Error(), "清理降级发布目标") {
			t.Fatalf("link failure = %v", err)
		}
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("file child collision reports ErrExist", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		testseam.Swap(t, &skillPathLink, func(string, string) error { return os.ErrExist })
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrExist) {
			t.Fatalf("file child collision = %v, want ErrExist", err)
		}
	})

	t.Run("later child failure rolls earlier children back", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		if err := os.WriteFile(filepath.Join(source, "second"), []byte("second"), 0o644); err != nil {
			t.Fatal(err)
		}
		forceChildMove(t)
		// The second child's link fails; the forced rename failure then also
		// breaks the rollback rename of the already relocated first child.
		testseam.Swap(t, &skillPathLink, func(source, _ string) error {
			if filepath.Base(source) == "second" {
				return os.ErrPermission
			}
			return os.Link(source, filepath.Join(destination, filepath.Base(source)))
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "迁移 Skill 目录项失败") {
			t.Fatalf("child failure rollback = %v", err)
		}
		// The forced rename failure also breaks the rollback restore, so the
		// relocated child stays at the destination — retained, never deleted.
		assertUpgradeSkillContent(t, destination, "payload")
	})

	t.Run("live claim read failure aborts with rollback", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		destinationReads := 0
		original := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == destination {
				destinationReads++
				if destinationReads == 2 {
					return nil, os.ErrPermission
				}
			}
			return original(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "确认降级发布目标") {
			t.Fatalf("live claim read failure = %v", err)
		}
	})

	t.Run("foreign claim entry aborts the move", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		destinationReads := 0
		original := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == destination {
				destinationReads++
				if destinationReads == 2 {
					return []os.DirEntry{
						childMoveDirEntry{name: "SKILL.md"},
						childMoveDirEntry{name: "foreign"},
					}, nil
				}
			}
			return original(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrExist) || !strings.Contains(err.Error(), "并发目录项") {
			t.Fatalf("foreign claim entry = %v", err)
		}
	})

	t.Run("claim mode restore failure aborts with rollback", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		testseam.Swap(t, &skillPathChmod, func(string, os.FileMode) error { return os.ErrPermission })
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "恢复已发布 Skill 目录权限") {
			t.Fatalf("mode restore failure = %v", err)
		}
	})

	t.Run("source shell removal failure surfaces with the destination retained", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		testseam.Swap(t, &skillPathRemove, func(path string) error {
			if path == source {
				return os.ErrPermission
			}
			return os.Remove(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "清理已迁移 Skill 源目录") {
			t.Fatalf("shell removal failure = %v", err)
		}
		assertUpgradeSkillContent(t, destination, "payload")
	})

	t.Run("nested directories and mixed children publish", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "nested", "inner.txt"), []byte("inner"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "link"), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
		forceChildMove(t)
		// The "link" child is presented to the dispatcher as a symlink; its
		// recreation is simulated through seams so the test needs no
		// symlink privileges (Windows runners refuse unprivileged symlinks).
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "link") {
				return skillPathFakeInfo{mode: os.ModeSymlink}, nil
			}
			return originalLstat(path)
		})
		testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "target", nil })
		testseam.Swap(t, &skillPathSymlink, func(string, string) error { return nil })
		claim, err := renameSkillPathNoReplace(source, destination)
		if err != nil {
			t.Fatal(err)
		}
		if claim == "" {
			t.Fatal("child-move must return the claim identity")
		}
		assertUpgradeSkillContent(t, destination, "payload")
		if data, readErr := os.ReadFile(filepath.Join(destination, "nested", "inner.txt")); readErr != nil || string(data) != "inner" {
			t.Fatalf("nested child = %q, %v", data, readErr)
		}
		if _, statErr := os.Lstat(filepath.Join(source, "link")); !os.IsNotExist(statErr) {
			t.Fatalf("simulated symlink child must have its source consumed, stat err=%v", statErr)
		}
		if _, statErr := os.Lstat(source); !os.IsNotExist(statErr) {
			t.Fatalf("source must be gone, stat err=%v", statErr)
		}
	})

	t.Run("special-type child is refused", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "SKILL.md") {
				return mockSpecialInfo{}, nil
			}
			return originalLstat(path)
		})
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, errNoReplaceRenameUnsupported) {
			t.Fatalf("special child = %v, want unsupported", err)
		}
	})

	t.Run("symlink child readlink failure surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "SKILL.md") {
				return skillPathFakeInfo{mode: os.ModeSymlink}, nil
			}
			return originalLstat(path)
		})
		testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "", os.ErrPermission })
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("symlink readlink failure = %v", err)
		}
	})

	t.Run("symlink child collision reports ErrExist", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "SKILL.md") {
				return skillPathFakeInfo{mode: os.ModeSymlink}, nil
			}
			return originalLstat(path)
		})
		testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "target", nil })
		testseam.Swap(t, &skillPathSymlink, func(string, string) error { return os.ErrExist })
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrExist) {
			t.Fatalf("symlink collision = %v, want ErrExist", err)
		}
	})

	t.Run("symlink child publish failure surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "SKILL.md") {
				return skillPathFakeInfo{mode: os.ModeSymlink}, nil
			}
			return originalLstat(path)
		})
		testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "target", nil })
		testseam.Swap(t, &skillPathSymlink, func(string, string) error { return os.ErrPermission })
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("symlink publish failure = %v", err)
		}
	})

	t.Run("child stat failure surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(source, "SKILL.md") {
				return nil, os.ErrPermission
			}
			return originalLstat(path)
		})
		if _, err := renameSkillPathNoReplace(source, destination); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("child stat failure = %v", err)
		}
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("source read failure aborts with the empty claim cleaned", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		forceChildMove(t)
		original := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == source {
				return nil, os.ErrPermission
			}
			return original(path)
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !strings.Contains(err.Error(), "读取源 Skill 目录失败") {
			t.Fatalf("source read failure = %v", err)
		}
		if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
			t.Fatalf("empty claim must be cleaned, stat err=%v", statErr)
		}
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("file identity degrades on unreadable and unrecognizable objects", func(t *testing.T) {
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return nil, os.ErrPermission })
		if id := skillPathFileIdentityImpl("unreadable"); id != "" {
			t.Fatalf("unreadable identity = %q, want empty", id)
		}
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return skillPathFakeInfo{mode: 0o644}, nil })
		if id := skillPathFileIdentityImpl("synthetic"); id != "" {
			t.Fatalf("unrecognizable identity = %q, want empty", id)
		}
	})

	t.Run("published content drift after the fast-path rename surfaces", func(t *testing.T) {
		dir := t.TempDir()
		staged := filepath.Join(dir, "staged")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(source, target string) (string, error) {
			if _, err := originalRename(source, target); err != nil {
				return "", err
			}
			// The published bytes drift after the rename while the object
			// keeps its identity, so only the fingerprint check can refuse.
			if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("drifted"), 0o644); err != nil {
				return "", err
			}
			return "", nil
		})
		if _, err := PublishSkillPathNoReplace(staged, destination); err == nil || !strings.Contains(err.Error(), "staging 内容已变化") {
			t.Fatalf("content drift = %v", err)
		}
	})
}
