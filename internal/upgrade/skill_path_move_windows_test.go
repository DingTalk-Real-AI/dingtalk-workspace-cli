//go:build windows

package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/windows"
)

func testCrossDeviceError() error {
	return &os.LinkError{Op: "rename", Old: "src", New: "dst", Err: windows.ERROR_NOT_SAME_DEVICE}
}

func testNoReplaceUnsupportedErrors() []error {
	return []error{errNoReplaceRenameUnsupported}
}

func TestCrossPlatformCoverageWindowsCrossDeviceError(t *testing.T) {
	if !isCrossDeviceError(testCrossDeviceError()) {
		t.Fatal("ERROR_NOT_SAME_DEVICE must enter the cross-device fallback")
	}
}

func TestCrossPlatformCoverageWindowsCrossDeviceMoveJunction(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "canonical", "dingtalk-chat")
	src := filepath.Join(tempDir, "agent", "dingtalk-chat")
	dst := filepath.Join(tempDir, "backup", "dingtalk-chat")

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("chat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createSkillPathDirJunction(target, src); err != nil {
		t.Fatal(err)
	}

	forceCrossDeviceRename(t, src, dst)
	if err := moveSkillPathRecoverably(src, dst); err != nil {
		t.Fatalf("moveSkillPathRecoverably junction cross-device = %v", err)
	}

	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("source junction must be removed: %v", err)
	}
	dstInfo, err := os.Lstat(dst)
	if err != nil || !isSkillPathLink(dstInfo.Mode()) {
		t.Fatalf("destination must be a link/junction: mode=%v, err=%v", dstInfo.Mode(), err)
	}
	readTarget, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink(dst) error = %v", err)
	}
	cleanTarget, _ := filepath.Abs(target)
	if filepath.Clean(readTarget) != filepath.Clean(cleanTarget) {
		t.Fatalf("readlink(dst) = %q, want %q", readTarget, cleanTarget)
	}
	content, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil || string(content) != "chat\n" {
		t.Fatalf("read SKILL.md through junction = %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatalf("canonical target must remain intact: %v", err)
	}

	// Test RestoreSkillPath (rollback)
	forceCrossDeviceRename(t, dst, src)
	if err := RestoreSkillPath(dst, src); err != nil {
		t.Fatalf("RestoreSkillPath junction = %v", err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("backup junction must be removed: %v", err)
	}
	srcInfo, err := os.Lstat(src)
	if err != nil || !isSkillPathLink(srcInfo.Mode()) {
		t.Fatalf("restored source must be a link/junction: mode=%v, err=%v", srcInfo.Mode(), err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatalf("canonical target must still remain intact: %v", err)
	}
}

func TestCrossPlatformCoverageWindowsJunctionHelpersAndEdges(t *testing.T) {
	// 1. junctionSubstituteName edge cases
	for _, tc := range []struct {
		input string
		want  string
	}{
		{`\\?\UNC\server\share\dir`, `\??\UNC\server\share\dir\`},
		{`\\?\C:\dir`, `\??\C:\dir\`},
		{`\\server\share\dir`, `\??\UNC\server\share\dir\`},
		{`C:\dir\`, `\??\C:\dir\`},
		{`C:\dir`, `\??\C:\dir\`},
	} {
		got := junctionSubstituteName(tc.input)
		if got != tc.want {
			t.Fatalf("junctionSubstituteName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	// 2. mountPointReparseBuffer NUL byte error branches
	if _, err := mountPointReparseBuffer("bad\x00sub", "print"); err == nil {
		t.Fatal("expected error for NUL in substitute")
	}
	if _, err := mountPointReparseBuffer("sub", "bad\x00print"); err == nil {
		t.Fatal("expected error for NUL in printName")
	}

	// 3. createSkillPathDirJunction error branches isolated via t.Run
	t.Run("empty_target", func(t *testing.T) {
		linkPath := filepath.Join(t.TempDir(), "link-empty-target")
		err := createSkillPathDirJunction("", linkPath)
		if err == nil || !strings.Contains(err.Error(), "junction target is empty") {
			t.Fatalf("expected empty target error, got %v", err)
		}
		if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("link path should not exist, got %v", err)
		}
	})

	t.Run("filepath_abs_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &skillPathAbs, func(string) (string, error) {
			return "", errors.New("mock abs error")
		})
		err := createSkillPathDirJunction(tempDir, filepath.Join(tempDir, "link-abs-err"))
		if err == nil || !strings.Contains(err.Error(), "resolve junction target") {
			t.Fatalf("expected resolve junction target error, got %v", err)
		}
	})

	t.Run("mount_point_buffer_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &skillPathAbs, func(s string) (string, error) { return s, nil })
		err := createSkillPathDirJunction("C:\\bad\x00target", filepath.Join(tempDir, "link-bad-target"))
		if err == nil || !strings.Contains(err.Error(), "encode junction substitute name") {
			t.Fatalf("expected encode junction substitute name error, got %v", err)
		}
	})

	t.Run("link_path_nul_error", func(t *testing.T) {
		tempDir := t.TempDir()
		err := createSkillPathDirJunction(tempDir, filepath.Join(tempDir, "link\x00bad"))
		if err == nil || !strings.Contains(err.Error(), "encode junction path") {
			t.Fatalf("expected encode junction path error, got %v", err)
		}
	})

	t.Run("mkdir_existing_file_error", func(t *testing.T) {
		tempDir := t.TempDir()
		existingFile := filepath.Join(tempDir, "existing-file")
		if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := createSkillPathDirJunction(tempDir, existingFile); err == nil {
			t.Fatal("expected error when link path is an existing file")
		}
	})

	t.Run("create_file_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &windowsCreateFile, func(path *uint16, access uint32, shareMode uint32, sa *windows.SecurityAttributes, creationDisposition uint32, flagsAndAttributes uint32, templateFile windows.Handle) (windows.Handle, error) {
			return windows.InvalidHandle, errors.New("mock create file error")
		})
		linkPath := filepath.Join(tempDir, "link-create-fail")
		err := createSkillPathDirJunction(tempDir, linkPath)
		if err == nil || !strings.Contains(err.Error(), "open junction") {
			t.Fatalf("expected open junction error, got %v", err)
		}
		if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("link path should be cleaned up on windowsCreateFile error, got %v", err)
		}
	})

	t.Run("device_io_control_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &windowsDeviceIoControl, func(handle windows.Handle, ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error {
			return errors.New("mock device io control error")
		})
		linkPath := filepath.Join(tempDir, "link-ioctl-fail")
		err := createSkillPathDirJunction(tempDir, linkPath)
		if err == nil || !strings.Contains(err.Error(), "set junction reparse point") {
			t.Fatalf("expected set junction reparse point error, got %v", err)
		}
		if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("link path should be cleaned up on windowsDeviceIoControl error, got %v", err)
		}
	})

	t.Run("copy_skill_path_link_symlink_branch", func(t *testing.T) {
		tempDir := t.TempDir()
		symlinkCalled := false
		testseam.Swap(t, &skillPathSymlink, func(target, link string) error {
			symlinkCalled = true
			return nil
		})
		if err := copySkillPathLink("target", filepath.Join(tempDir, "symlink"), os.ModeSymlink); err != nil {
			t.Fatalf("copySkillPathLink symlink = %v", err)
		}
		if !symlinkCalled {
			t.Fatal("expected skillPathSymlink to be called for ModeSymlink")
		}
	})
}
