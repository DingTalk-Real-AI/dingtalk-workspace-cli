package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type mockSpecialInfo struct{}

func (mockSpecialInfo) Name() string       { return "source" }
func (mockSpecialInfo) Size() int64        { return 0 }
func (mockSpecialInfo) Mode() os.FileMode  { return os.ModeNamedPipe }
func (mockSpecialInfo) ModTime() time.Time { return time.Time{} }
func (mockSpecialInfo) IsDir() bool        { return false }
func (mockSpecialInfo) Sys() interface{}   { return nil }

// TestCrossPlatformCoverageNoReplaceRenameFallback pins the degradation path for
// filesystems that reject RENAME_NOREPLACE / RENAME_EXCL (NFS, FUSE, overlayfs).
// Those installs must still publish, and must still refuse to clobber an
// occupied destination. The fallback uses atomic no-clobber primitives
// (os.Mkdir for directories, os.Link for files) instead of a TOCTOU-prone
// check-then-rename.
func TestCrossPlatformCoverageNoReplaceRenameFallback(t *testing.T) {
	unsupported := testNoReplaceUnsupportedErrors()

	t.Run("publishes a file when the filesystem rejects the flag", func(t *testing.T) {
		for _, unsupportedErr := range unsupported {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			destination := filepath.Join(dir, "destination")
			if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return unsupportedErr })
			if _, err := renameSkillPathNoReplace(source, destination); err != nil {
				t.Fatalf("fallback must publish for %v, got %v", unsupportedErr, err)
			}
			if data, err := os.ReadFile(destination); err != nil || string(data) != "payload" {
				t.Fatalf("destination = %q, %v", data, err)
			}
			if _, err := os.Lstat(source); !os.IsNotExist(err) {
				t.Fatalf("source must be gone, stat err=%v", err)
			}
		}
	})

	t.Run("publishes a directory when the filesystem rejects the flag", func(t *testing.T) {
		for _, unsupportedErr := range unsupported {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			destination := filepath.Join(dir, "destination")
			seedUpgradeSkill(t, source, "payload", false)
			testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return unsupportedErr })
			if _, err := renameSkillPathNoReplace(source, destination); err != nil {
				t.Fatalf("fallback must publish for %v, got %v", unsupportedErr, err)
			}
			assertUpgradeSkillContent(t, destination, "payload")
			if _, err := os.Lstat(source); !os.IsNotExist(err) {
				t.Fatalf("source must be gone, stat err=%v", err)
			}
		}
	})

	t.Run("still refuses an occupied destination for files", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		for _, path := range []string{source, destination} {
			if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("occupied destination must report ErrExist, got %v", err)
		}
		if data, readErr := os.ReadFile(destination); readErr != nil || string(data) != "destination" {
			t.Fatalf("destination must be untouched, got %q, %v", data, readErr)
		}
		if _, statErr := os.Lstat(source); statErr != nil {
			t.Fatalf("source must be preserved: %v", statErr)
		}
	})

	t.Run("file link failure with non-EEXIST error surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathLink, func(string, string) error { return os.ErrPermission })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("link error must surface, got %v", err)
		}
		if _, statErr := os.Lstat(source); statErr != nil {
			t.Fatalf("source must be preserved: %v", statErr)
		}
	})

	t.Run("refuses an occupied directory destination via mkdir EEXIST", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		seedUpgradeSkill(t, destination, "existing", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("occupied destination must report ErrExist, got %v", err)
		}
		assertUpgradeSkillContent(t, destination, "existing")
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("concurrent entry in the claimed directory aborts with the destination retained", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathRename, func(_, dst string) error {
			// A concurrent writer lands inside the empty claim before the
			// slow path can take it; the fast-path rename then fails the
			// way it would against a populated target.
			if err := os.WriteFile(filepath.Join(dst, "foreign"), []byte("foreign"), 0o644); err != nil {
				return err
			}
			return os.ErrExist
		})
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrExist) || !strings.Contains(err.Error(), "并发写入") {
			t.Fatalf("concurrent claim entry must abort with ErrExist, got %v", err)
		}
		if data, readErr := os.ReadFile(filepath.Join(destination, "foreign")); readErr != nil || string(data) != "foreign" {
			t.Fatalf("foreign entry must be retained, got %q, %v", data, readErr)
		}
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("a genuine error is not retried", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		calls := 0
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error {
			calls++
			return os.ErrPermission
		})
		if _, err := renameSkillPathNoReplace(source, filepath.Join(dir, "destination")); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("permission error must surface, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("atomic attempts = %d, want 1", calls)
		}
	})

	t.Run("source stat failure surfaces without rename", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(dir, "destination")
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return nil, os.ErrPermission })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("stat error must surface, got %v", err)
		}
		if _, statErr := os.Lstat(source); statErr != nil {
			t.Fatalf("source must be preserved: %v", statErr)
		}
	})

	t.Run("cross-device errors still reach the copy fallback", func(t *testing.T) {
		crossDevice := testCrossDeviceError()
		if isNoReplaceRenameUnsupported(crossDevice) {
			t.Fatal("cross-device error must not be treated as an unsupported flag")
		}
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		if err := os.MkdirAll(source, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(dir, "backup", "destination")
		original := skillPathRenameNoReplaceAtomic
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(from, to string) error {
			if from == source {
				return crossDevice
			}
			return original(from, to)
		})
		if err := moveSkillPathRecoverably(source, destination); err != nil {
			t.Fatalf("cross-device move = %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(destination, "SKILL.md")); err != nil || string(data) != "payload" {
			t.Fatalf("cross-device destination = %q, %v", data, err)
		}
	})

	t.Run("mkdir failure with non-EEXIST error surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathMkdir, func(string, os.FileMode) error { return os.ErrPermission })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("mkdir error must surface, got %v", err)
		}
		assertUpgradeSkillContent(t, source, "payload")
	})

	t.Run("remove failure after rename failure surfaces", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathRename, func(string, string) error { return errors.New("rename failed") })
		removeErr := errors.New("simulated remove failure")
		testseam.Swap(t, &skillPathRemove, func(string) error { return removeErr })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, removeErr) {
			t.Fatalf("remove error must surface, got %v", err)
		}
		if _, statErr := os.Lstat(destination); statErr != nil {
			t.Fatalf("destination must still exist after remove failure: %v", statErr)
		}
		assertUpgradeSkillContent(t, source, "payload")
		_ = os.RemoveAll(destination)
	})

	t.Run("fast-path rename failure alone publishes through child-move", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		// macOS refuses to rename over any directory, empty or not. The
		// claim's verified emptiness — not the rename error — decides
		// whether the child-move slow path may proceed.
		testseam.Swap(t, &skillPathRename, func(string, string) error { return os.ErrExist })
		if _, err := renameSkillPathNoReplace(source, destination); err != nil {
			t.Fatalf("child-move must publish despite rename failure, got %v", err)
		}
		assertUpgradeSkillContent(t, destination, "payload")
		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			t.Fatalf("source must be gone, stat err=%v", err)
		}
	})

	t.Run("directory publish when first rename replaces empty dir", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		originalRename := skillPathRename
		testseam.Swap(t, &skillPathRename, func(src, dst string) error {
			_ = os.Remove(dst)
			return originalRename(src, dst)
		})
		if _, err := renameSkillPathNoReplace(source, destination); err != nil {
			t.Fatalf("publish must succeed when rename replaces empty dir, got %v", err)
		}
		assertUpgradeSkillContent(t, destination, "payload")
	})

	t.Run("non-regular non-directory source fails safely", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) { return mockSpecialInfo{}, nil })
		_, err := renameSkillPathNoReplace(source, destination)
		if !errors.Is(err, errNoReplaceRenameUnsupported) {
			t.Fatalf("unsupported error must surface for special files, got %v", err)
		}
	})
}
