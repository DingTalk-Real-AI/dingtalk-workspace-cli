package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillPathCrossDeviceMove(t *testing.T) {
	t.Run("directory file and lexical links", func(t *testing.T) {
		root := t.TempDir()
		t.Cleanup(func() { makeSkillPathTreeWritable(root) })
		src := filepath.Join(root, "external", "dingtalk-chat")
		dst := filepath.Join(root, "home", ".dws", "skill-backups", "stamp", "dingtalk-chat")
		if err := os.MkdirAll(filepath.Join(src, "references"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("chat\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "references", "guide.md"), []byte("guide\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		linksCreated := true
		if err := os.Symlink("SKILL.md", filepath.Join(src, "skill-link")); err != nil {
			if runtime.GOOS != "windows" {
				t.Fatal(err)
			}
			linksCreated = false
		}
		if linksCreated {
			if err := os.Symlink("missing.md", filepath.Join(src, "dangling-link")); err != nil {
				t.Fatal(err)
			}
		}

		forceCrossDeviceRename(t, src, dst)
		if err := moveSkillPathRecoverably(src, dst); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(src); !os.IsNotExist(err) {
			t.Fatalf("source must be removed: %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(dst, "SKILL.md")); err != nil || string(got) != "chat\n" {
			t.Fatalf("copied file = %q, %v", got, err)
		}
		if info, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
			t.Fatalf("copied file stat = %v, %v", info, err)
		} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
			t.Fatalf("copied file mode = %v; want 0640", info.Mode().Perm())
		}
		if linksCreated {
			for name, want := range map[string]string{"skill-link": "SKILL.md", "dangling-link": "missing.md"} {
				info, err := os.Lstat(filepath.Join(dst, name))
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("%s must remain a symlink: %v, %v", name, info, err)
				}
				if got, err := os.Readlink(filepath.Join(dst, name)); err != nil || got != want {
					t.Fatalf("%s target = %q, %v; want %q", name, got, err, want)
				}
			}
		}
		assertNoCrossDeviceStage(t, dst)
	})

	t.Run("regular file root", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "source-file")
		dst := filepath.Join(root, "backup", "source-file")
		if err := os.WriteFile(src, []byte("state"), 0o600); err != nil {
			t.Fatal(err)
		}
		forceCrossDeviceRename(t, src, dst)
		if err := moveSkillPathRecoverably(src, dst); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(dst); err != nil || string(got) != "state" {
			t.Fatalf("backup file = %q, %v", got, err)
		}
	})

	t.Run("read-only directory mode is restored after copying children", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX directory permission semantics are unavailable")
		}
		root := t.TempDir()
		t.Cleanup(func() { makeSkillPathTreeWritable(root) })
		src := filepath.Join(root, "external", "dingtalk-chat")
		dst := filepath.Join(root, "backup", "dingtalk-chat")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("read only\n"), 0o444); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(src, 0o555); err != nil {
			t.Fatal(err)
		}
		forceCrossDeviceRename(t, src, dst)
		if err := moveSkillPathRecoverably(src, dst); err != nil {
			t.Fatal(err)
		}
		if info, err := os.Stat(dst); err != nil || info.Mode().Perm() != 0o555 {
			t.Fatalf("copied directory mode = %v, %v", info, err)
		}
		if got, err := os.ReadFile(filepath.Join(dst, "SKILL.md")); err != nil || string(got) != "read only\n" {
			t.Fatalf("copied read-only file = %q, %v", got, err)
		}
	})

	t.Run("rollback uses the same cross-device contract", func(t *testing.T) {
		root := t.TempDir()
		backup := filepath.Join(root, "home", ".dws", "skill-backups", "stamp", "dingtalk-chat")
		original := filepath.Join(root, "external", "claude", "skills", "dingtalk-chat")
		if err := os.MkdirAll(backup, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backup, "SKILL.md"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		forceCrossDeviceRename(t, backup, original)
		if err := RestoreSkillPath(backup, original); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(original, "SKILL.md")); err != nil || string(got) != "old" {
			t.Fatalf("restored content = %q, %v", got, err)
		}
		if _, err := os.Lstat(backup); !os.IsNotExist(err) {
			t.Fatalf("backup must be consumed after verified restore: %v", err)
		}
	})
}

func TestCrossPlatformCoverageSkillPathCrossDeviceFailures(t *testing.T) {
	t.Run("non cross-device rename never copies", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		copied := false
		testseam.Swap(t, &skillPathRenameNoReplace, func(string, string) (string, error) { return "", os.ErrPermission })
		testseam.Swap(t, &skillPathCopy, func(string, string) error { copied = true; return nil })
		if err := moveSkillPathRecoverably(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
		if copied {
			t.Fatal("permission failure must not enter cross-device copy fallback")
		}
		assertSkillSourcePreserved(t, src)
	})

	for _, tc := range []struct {
		name   string
		copy   func(string, string) error
		verify func(string, string) error
	}{
		{name: "copy failure", copy: func(string, string) error { return errors.New("copy failed") }},
		{name: "verification failure", verify: func(string, string) error { return errors.New("verify failed") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, dst := makeSkillMoveFixture(t)
			forceCrossDeviceRename(t, src, dst)
			if tc.copy != nil {
				testseam.Swap(t, &skillPathCopy, tc.copy)
			}
			if tc.verify != nil {
				testseam.Swap(t, &skillPathVerify, tc.verify)
			}
			if err := moveSkillPathRecoverably(src, dst); err == nil {
				t.Fatal("expected failure")
			}
			assertSkillSourcePreserved(t, src)
			if _, err := os.Lstat(dst); !os.IsNotExist(err) {
				t.Fatalf("formal backup must not publish: %v", err)
			}
			assertNoCrossDeviceStage(t, dst)
		})
	}

	t.Run("staging publish failure", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(from, to string) (string, error) {
			if from == src && to == dst {
				return "", testCrossDeviceError()
			}
			if to == dst {
				return "", errors.New("publish failed")
			}
			return originalRename(from, to)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "publish failed") {
			t.Fatalf("error = %v", err)
		}
		assertSkillSourcePreserved(t, src)
		assertNoCrossDeviceStage(t, dst)
	})

	t.Run("published verification failure preserves both", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		calls := 0
		testseam.Swap(t, &skillPathVerify, func(string, string) error {
			calls++
			if calls == 2 {
				return errors.New("published verify failed")
			}
			return nil
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "published verify failed") {
			t.Fatalf("error = %v", err)
		}
		assertSkillSourcePreserved(t, src)
		if _, err := os.Lstat(dst); err != nil {
			t.Fatalf("verified backup candidate must remain: %v", err)
		}
		assertNoCrossDeviceStage(t, dst)
	})

	t.Run("source removal failure preserves both", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if path == src {
				return errors.New("remove failed")
			}
			return originalRemove(path)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "均保留") {
			t.Fatalf("error = %v", err)
		}
		assertSkillSourcePreserved(t, src)
		if _, err := os.Lstat(dst); err != nil {
			t.Fatalf("formal backup must remain: %v", err)
		}
	})

	t.Run("read-only source mode is restored after removal failure", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX directory permission semantics are unavailable")
		}
		src, dst := makeSkillMoveFixture(t)
		if err := os.Chmod(src, 0o555); err != nil {
			t.Fatal(err)
		}
		forceCrossDeviceRename(t, src, dst)
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if path == src {
				return errors.New("remove failed")
			}
			return originalRemove(path)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "均保留") {
			t.Fatalf("error = %v", err)
		}
		if info, err := os.Stat(src); err != nil || info.Mode().Perm() != 0o555 {
			t.Fatalf("source directory mode after failed removal = %v, %v", info, err)
		}
		if err := os.Chmod(src, 0o755); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("source permission preparation failure preserves both", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX directory permission semantics are unavailable")
		}
		src, dst := makeSkillMoveFixture(t)
		if err := os.Chmod(src, 0o555); err != nil {
			t.Fatal(err)
		}
		forceCrossDeviceRename(t, src, dst)
		originalChmod := skillPathChmod
		testseam.Swap(t, &skillPathChmod, func(path string, mode os.FileMode) error {
			if path == src {
				return errors.New("chmod failed")
			}
			return originalChmod(path, mode)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "chmod failed") {
			t.Fatalf("error = %v", err)
		}
		if err := originalChmod(src, 0o755); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lying remover is detected", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if path == src {
				return nil
			}
			return originalRemove(path)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "仍存在") {
			t.Fatalf("error = %v", err)
		}
		assertSkillSourcePreserved(t, src)
	})

	t.Run("same-filesystem fast path", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		if err := moveSkillPathRecoverably(src, dst); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(src); !os.IsNotExist(err) {
			t.Fatalf("source still exists: %v", err)
		}
	})

	t.Run("existing destination is never overwritten", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "目标已存在") {
			t.Fatalf("error = %v", err)
		}
		assertSkillSourcePreserved(t, src)
	})
}

func TestCrossPlatformCoverageVerifySkillPathCopyMismatches(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dst, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "类型") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("directory entries", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.Mkdir(src, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dst, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "one"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "数量") {
			t.Fatalf("error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dst, "two"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "目录项") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("file size and digest", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.WriteFile(src, []byte("same"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte("longer"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "大小") {
			t.Fatalf("error = %v", err)
		}
		if err := os.WriteFile(dst, []byte("diff"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "摘要") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("symlink target", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.Symlink("one", src); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("symlink creation unavailable")
			}
			t.Fatal(err)
		}
		if err := os.Symlink("two", dst); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "链接目标") {
			t.Fatalf("error = %v", err)
		}
	})
}

func forceCrossDeviceRename(t *testing.T, src, dst string) {
	t.Helper()
	originalRename := skillPathRenameNoReplace
	testseam.Swap(t, &skillPathRenameNoReplace, func(from, to string) (string, error) {
		if from == src && to == dst {
			return "", testCrossDeviceError()
		}
		return originalRename(from, to)
	})
}

func makeSkillMoveFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() { makeSkillPathTreeWritable(root) })
	src := filepath.Join(root, "external", "dingtalk-chat")
	dst := filepath.Join(root, "home", ".dws", "skill-backups", "stamp", "dingtalk-chat")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src, dst
}

func assertSkillSourcePreserved(t *testing.T, src string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(src, "SKILL.md")); err != nil || string(got) != "old" {
		t.Fatalf("source content = %q, %v", got, err)
	}
}

func assertNoCrossDeviceStage(t *testing.T, dst string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(dst))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	prefix := "." + filepath.Base(dst) + ".cross-device-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Fatalf("cross-device staging leaked: %s", entry.Name())
		}
	}
}
