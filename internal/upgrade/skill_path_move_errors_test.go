package upgrade

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillPathMoveSystemErrors(t *testing.T) {
	t.Run("destination probe", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		original := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == dst {
				return nil, os.ErrPermission
			}
			return original(path)
		})
		if err := moveSkillPathRecoverably(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("destination parent", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		testseam.Swap(t, &skillPathMkdirAll, func(string, os.FileMode) error { return os.ErrPermission })
		if err := moveSkillPathRecoverably(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("staging creation", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		testseam.Swap(t, &skillPathMkdirTemp, func(string, string) (string, error) { return "", os.ErrPermission })
		if err := moveSkillPathRecoverably(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("failed staging cleanup", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		testseam.Swap(t, &skillPathCopy, func(string, string) error { return errors.New("copy failed") })
		testseam.Swap(t, &skillPathRemoveAll, func(string) error { return errors.New("cleanup failed") })
		err := moveSkillPathRecoverably(src, dst)
		if err == nil || !strings.Contains(err.Error(), "copy failed") || !strings.Contains(err.Error(), "cleanup failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing copied staging", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		testseam.Swap(t, &skillPathCopy, func(string, string) error { return nil })
		testseam.Swap(t, &skillPathVerify, func(string, string) error { return nil })
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "检查跨设备") {
			t.Fatalf("error = %v", err)
		}
	})

	for _, tc := range []struct {
		name string
		path func(src, dst string) string
		want string
	}{
		{name: "prepare staging mode", path: func(_, dst string) string { return filepath.Join(filepath.Dir(dst), "payload") }, want: "准备跨设备"},
		{name: "restore published mode", path: func(_, dst string) string { return dst }, want: "恢复已发布"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, dst := makeSkillMoveFixture(t)
			forceCrossDeviceRename(t, src, dst)
			original := skillPathChmod
			testseam.Swap(t, &skillPathChmod, func(path string, mode os.FileMode) error {
				if tc.name == "prepare staging mode" && strings.HasSuffix(path, string(filepath.Separator)+"payload") {
					return errors.New("chmod failed")
				}
				if tc.name == "restore published mode" && path == dst {
					return errors.New("chmod failed")
				}
				return original(path, mode)
			})
			if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	t.Run("published staging cleanup", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		original := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if strings.Contains(filepath.Base(path), ".cross-device-") {
				return errors.New("cleanup failed")
			}
			return original(path)
		})
		if err := moveSkillPathRecoverably(src, dst); err == nil || !strings.Contains(err.Error(), "清理已发布") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("source deletion cannot be confirmed", func(t *testing.T) {
		src, dst := makeSkillMoveFixture(t)
		forceCrossDeviceRename(t, src, dst)
		removalAttempted := false
		originalLstat := skillPathLstat
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if path == src {
				removalAttempted = true
				return nil
			}
			return originalRemove(path)
		})
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if path == src && removalAttempted {
				return nil, os.ErrPermission
			}
			return originalLstat(path)
		})
		if err := moveSkillPathRecoverably(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSkillPathPermissionRestorationErrors(t *testing.T) {
	t.Run("source preparation error", func(t *testing.T) {
		testseam.Swap(t, &skillPathWalkDir, func(string, fs.WalkDirFunc) error {
			return errors.New("prepare failed")
		})
		if err := removePublishedSkillSource("ignored"); err == nil || !strings.Contains(err.Error(), "prepare failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("walk error", func(t *testing.T) {
		testseam.Swap(t, &skillPathWalkDir, func(root string, fn fs.WalkDirFunc) error {
			return fn(root, nil, errors.New("walk failed"))
		})
		if _, err := prepareSkillPathTreeRemoval("ignored"); err == nil || !strings.Contains(err.Error(), "walk failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("writable mode preparation", func(t *testing.T) {
		entry := skillPathModeDirEntry{mode: os.ModeDir | 0o500}
		testseam.Swap(t, &skillPathWalkDir, func(root string, fn fs.WalkDirFunc) error {
			return fn(root, entry, nil)
		})
		var chmodMode os.FileMode
		testseam.Swap(t, &skillPathChmod, func(_ string, mode os.FileMode) error {
			chmodMode = mode
			return nil
		})
		modes, err := prepareSkillPathTreeRemoval("source")
		if err != nil {
			t.Fatal(err)
		}
		if chmodMode != 0o700 || len(modes) != 1 || modes[0].path != "source" || modes[0].mode != 0o500 {
			t.Fatalf("prepared mode = %v, records = %#v", chmodMode, modes)
		}
	})

	t.Run("writable mode preparation chmod error", func(t *testing.T) {
		entry := skillPathModeDirEntry{mode: os.ModeDir | 0o500}
		testseam.Swap(t, &skillPathWalkDir, func(root string, fn fs.WalkDirFunc) error {
			return fn(root, entry, nil)
		})
		testseam.Swap(t, &skillPathChmod, func(string, os.FileMode) error {
			return errors.New("chmod failed")
		})
		if _, err := prepareSkillPathTreeRemoval("source"); err == nil || !strings.Contains(err.Error(), "chmod failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("entry info error", func(t *testing.T) {
		testseam.Swap(t, &skillPathWalkDir, func(root string, fn fs.WalkDirFunc) error {
			return fn(root, skillPathErrorDirEntry{}, nil)
		})
		if _, err := prepareSkillPathTreeRemoval("ignored"); err == nil || !strings.Contains(err.Error(), "info failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing restored directory", func(t *testing.T) {
		if err := restoreSkillPathDirModes([]skillPathDirMode{{path: filepath.Join(t.TempDir(), "missing"), mode: 0o555}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("restore probe error", func(t *testing.T) {
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return nil, os.ErrPermission })
		if err := restoreSkillPathDirModes([]skillPathDirMode{{path: "blocked", mode: 0o555}}); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("restore chmod error", func(t *testing.T) {
		path := t.TempDir()
		testseam.Swap(t, &skillPathChmod, func(string, os.FileMode) error { return os.ErrPermission })
		if err := restoreSkillPathDirModes([]skillPathDirMode{{path: path, mode: 0o555}}); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSkillPathLexicalCopyErrors(t *testing.T) {
	t.Run("source lstat", func(t *testing.T) {
		if err := copySkillPathLexically(filepath.Join(t.TempDir(), "missing"), "unused"); !os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("destination mkdir", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.Mkdir(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte("occupied"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := copySkillPathLexically(src, dst); err == nil {
			t.Fatal("expected mkdir failure")
		}
	})

	t.Run("source readdir", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		if err := os.Mkdir(src, 0o755); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathReadDir, func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission })
		if err := copySkillPathLexically(src, filepath.Join(root, "dst")); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("nested copy", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.Mkdir(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "child"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		original := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == "child" {
				return nil, os.ErrPermission
			}
			return original(path)
		})
		if err := copySkillPathLexically(src, dst); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("readlink", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "link")
		if err := os.Symlink("target", src); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "", os.ErrPermission })
		if err := copySkillPathLexically(src, filepath.Join(root, "dst")); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("special path", func(t *testing.T) {
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) {
			return skillPathFakeInfo{mode: os.ModeNamedPipe}, nil
		})
		if err := copySkillPathLexically("special", "dst"); err == nil || !strings.Contains(err.Error(), "特殊") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("regular open", func(t *testing.T) {
		if err := copyRegularSkillFile(filepath.Join(t.TempDir(), "missing"), "dst", 0o600); !os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("regular destination open", func(t *testing.T) {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte("occupied"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := copyRegularSkillFile(src, dst, 0o600); err == nil {
			t.Fatal("expected exclusive create failure")
		}
	})

	for _, tc := range []struct {
		name string
		swap func(*testing.T)
	}{
		{name: "copy", swap: func(t *testing.T) {
			testseam.Swap(t, &skillPathCopyBytes, func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy failed") })
		}},
		{name: "sync", swap: func(t *testing.T) {
			testseam.Swap(t, &skillPathSync, func(*os.File) error { return errors.New("sync failed") })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
			if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			tc.swap(t)
			if err := copyRegularSkillFile(src, dst, 0o600); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageSkillPathVerificationErrors(t *testing.T) {
	root := t.TempDir()

	t.Run("source and destination lstat", func(t *testing.T) {
		if err := verifySkillPathCopy(filepath.Join(root, "missing-src"), "dst"); !os.IsNotExist(err) {
			t.Fatalf("source error = %v", err)
		}
		src := filepath.Join(root, "source")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, filepath.Join(root, "missing-dst")); !os.IsNotExist(err) {
			t.Fatalf("destination error = %v", err)
		}
	})

	t.Run("directory and file modes", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			src  os.FileMode
			dst  os.FileMode
		}{
			{name: "directory", src: os.ModeDir | 0o700, dst: os.ModeDir | 0o755},
			{name: "file", src: 0o600, dst: 0o644},
		} {
			t.Run(tc.name, func(t *testing.T) {
				src, dst := "src", "dst"
				testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
					if path == src {
						return skillPathFakeInfo{mode: tc.src}, nil
					}
					return skillPathFakeInfo{mode: tc.dst}, nil
				})
				if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "权限") {
					t.Fatalf("error = %v", err)
				}
			})
		}
	})

	t.Run("directory read errors", func(t *testing.T) {
		for _, failDest := range []bool{false, true} {
			t.Run(map[bool]string{false: "source", true: "destination"}[failDest], func(t *testing.T) {
				base := t.TempDir()
				src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
				if err := os.Mkdir(src, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dst, 0o700); err != nil {
					t.Fatal(err)
				}
				original := skillPathReadDir
				testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
					if (path == dst) == failDest {
						return nil, os.ErrPermission
					}
					return original(path)
				})
				if err := verifySkillPathCopy(src, dst); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("error = %v", err)
				}
			})
		}
	})

	t.Run("nested mismatch", func(t *testing.T) {
		base := t.TempDir()
		src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
		if err := os.Mkdir(src, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dst, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "child"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dst, "child"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifySkillPathCopy(src, dst); err == nil || !strings.Contains(err.Error(), "类型") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("symlink reads", func(t *testing.T) {
		base := t.TempDir()
		src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
		if err := os.Symlink("one", src); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := os.Symlink("one", dst); err != nil {
			t.Fatal(err)
		}
		for _, failDest := range []bool{false, true} {
			t.Run(map[bool]string{false: "source", true: "destination"}[failDest], func(t *testing.T) {
				original := skillPathReadlink
				testseam.Swap(t, &skillPathReadlink, func(path string) (string, error) {
					if (path == dst) == failDest {
						return "", os.ErrPermission
					}
					return original(path)
				})
				if err := verifySkillPathCopy(src, dst); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("error = %v", err)
				}
			})
		}
	})

	t.Run("file digest opens", func(t *testing.T) {
		base := t.TempDir()
		src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, failDest := range []bool{false, true} {
			t.Run(map[bool]string{false: "source", true: "destination"}[failDest], func(t *testing.T) {
				original := skillPathOpen
				testseam.Swap(t, &skillPathOpen, func(path string) (*os.File, error) {
					if (path == dst) == failDest {
						return nil, os.ErrPermission
					}
					return original(path)
				})
				if err := verifySkillPathCopy(src, dst); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("error = %v", err)
				}
			})
		}
	})

	t.Run("special path", func(t *testing.T) {
		fake := skillPathFakeInfo{mode: os.ModeNamedPipe}
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return fake, nil })
		if err := verifySkillPathCopy("src", "dst"); err == nil || !strings.Contains(err.Error(), "特殊") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("entry names", func(t *testing.T) {
		if _, err := skillPathEntryNames(filepath.Join(root, "missing-dir")); !os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("digest open and read", func(t *testing.T) {
		if _, err := skillPathFileDigest(filepath.Join(root, "missing-file")); !os.IsNotExist(err) {
			t.Fatalf("open error = %v", err)
		}
		file := filepath.Join(root, "digest-file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathCopyBytes, func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("read failed") })
		if _, err := skillPathFileDigest(file); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("read error = %v", err)
		}
	})
}

type skillPathFakeInfo struct {
	mode os.FileMode
}

func (skillPathFakeInfo) Name() string           { return "fake" }
func (skillPathFakeInfo) Size() int64            { return 0 }
func (info skillPathFakeInfo) Mode() os.FileMode { return info.mode }
func (skillPathFakeInfo) ModTime() time.Time     { return time.Time{} }
func (info skillPathFakeInfo) IsDir() bool       { return info.mode.IsDir() }
func (skillPathFakeInfo) Sys() any               { return nil }

type skillPathErrorDirEntry struct{}

func (skillPathErrorDirEntry) Name() string               { return "broken" }
func (skillPathErrorDirEntry) IsDir() bool                { return true }
func (skillPathErrorDirEntry) Type() os.FileMode          { return os.ModeDir }
func (skillPathErrorDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("info failed") }

type skillPathModeDirEntry struct {
	mode os.FileMode
}

func (skillPathModeDirEntry) Name() string            { return "directory" }
func (skillPathModeDirEntry) IsDir() bool             { return true }
func (entry skillPathModeDirEntry) Type() os.FileMode { return entry.mode.Type() }
func (entry skillPathModeDirEntry) Info() (os.FileInfo, error) {
	return skillPathFakeInfo{mode: entry.mode}, nil
}
