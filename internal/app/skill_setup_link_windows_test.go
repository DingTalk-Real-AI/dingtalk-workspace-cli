//go:build windows

package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoverageSkillSetupWindowsJunctionReadable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "canonical", "dingtalk-chat")
	link := filepath.Join(t.TempDir(), "agent", "dingtalk-chat")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("chat"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := createSkillSetupDirLink(target, link); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(link)

	info, err := os.Stat(link)
	if err != nil || !info.IsDir() {
		t.Fatalf("junction stat = %#v, %v", info, err)
	}
	body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil || string(body) != "chat" {
		t.Fatalf("junction Skill read = %q, %v", body, err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatalf("junction removal changed canonical target: %v", err)
	}
}

func TestCrossPlatformCoverageSkillSetupWindowsJunctionHelpersAndEdges(t *testing.T) {
	// 1. skillSetupLinkTarget returns realTarget
	if got := skillSetupLinkTarget("real", "rel"); got != "real" {
		t.Fatalf("skillSetupLinkTarget = %q, want real", got)
	}

	// 2. junctionSubstituteName edge cases
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

	// 3. mountPointReparseBuffer NUL byte and oversized path error branches
	if _, err := mountPointReparseBuffer("bad\x00sub", "print"); err == nil {
		t.Fatal("expected error for NUL in substitute")
	}
	if _, err := mountPointReparseBuffer("sub", "bad\x00print"); err == nil {
		t.Fatal("expected error for NUL in printName")
	}
	longPath := strings.Repeat("a", 20000)
	if _, err := mountPointReparseBuffer(longPath, "print"); err == nil || !strings.Contains(err.Error(), "junction path too long") {
		t.Fatalf("expected path too long error for oversized substitute, got %v", err)
	}
	hugePath := strings.Repeat("b", 70000)
	if _, err := mountPointReparseBuffer(hugePath, "print"); err == nil || !strings.Contains(err.Error(), "junction path too long") {
		t.Fatalf("expected path too long error for >64k path without panic, got %v", err)
	}

	// 4. createSkillSetupDirLink error branches isolated via t.Run
	t.Run("empty_target", func(t *testing.T) {
		linkPath := filepath.Join(t.TempDir(), "link-empty-target")
		err := createSkillSetupDirLink("", linkPath)
		if err == nil || !strings.Contains(err.Error(), "junction target is empty") {
			t.Fatalf("expected empty target error, got %v", err)
		}
		if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("link path should not exist, got %v", err)
		}
	})

	t.Run("filepath_abs_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &filepathAbs, func(string) (string, error) {
			return "", errors.New("mock abs error")
		})
		err := createSkillSetupDirLink(tempDir, filepath.Join(tempDir, "link-abs-err"))
		if err == nil || !strings.Contains(err.Error(), "resolve junction target") {
			t.Fatalf("expected resolve junction target error, got %v", err)
		}
	})

	t.Run("mount_point_buffer_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &filepathAbs, func(s string) (string, error) { return s, nil })
		err := createSkillSetupDirLink("C:\\bad\x00target", filepath.Join(tempDir, "link-bad-target"))
		if err == nil || !strings.Contains(err.Error(), "encode junction substitute name") {
			t.Fatalf("expected encode junction substitute name error, got %v", err)
		}
	})

	t.Run("link_path_nul_error", func(t *testing.T) {
		tempDir := t.TempDir()
		err := createSkillSetupDirLink(tempDir, filepath.Join(tempDir, "link\x00bad"))
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
		if err := createSkillSetupDirLink(tempDir, existingFile); err == nil {
			t.Fatal("expected error when link path is an existing file")
		}
	})

	t.Run("create_file_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &windowsCreateFile, func(path *uint16, access uint32, shareMode uint32, sa *windows.SecurityAttributes, creationDisposition uint32, flagsAndAttributes uint32, templateFile windows.Handle) (windows.Handle, error) {
			return windows.InvalidHandle, errors.New("mock create file error")
		})
		linkPath := filepath.Join(tempDir, "link-create-fail")
		err := createSkillSetupDirLink(tempDir, linkPath)
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
		err := createSkillSetupDirLink(tempDir, linkPath)
		if err == nil || !strings.Contains(err.Error(), "set junction reparse point") {
			t.Fatalf("expected set junction reparse point error, got %v", err)
		}
		if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
			t.Fatalf("link path should be cleaned up on windowsDeviceIoControl error, got %v", err)
		}
	})

	t.Run("create_file_error_and_remove_placeholder_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &windowsCreateFile, func(path *uint16, access uint32, shareMode uint32, sa *windows.SecurityAttributes, creationDisposition uint32, flagsAndAttributes uint32, templateFile windows.Handle) (windows.Handle, error) {
			return windows.InvalidHandle, errors.New("mock create file error")
		})
		testseam.Swap(t, &windowsOsRemove, func(path string) error {
			return errors.New("mock remove placeholder error")
		})
		linkPath := filepath.Join(tempDir, "link-create-fail-remove-fail")
		err := createSkillSetupDirLink(tempDir, linkPath)
		if err == nil {
			t.Fatal("expected error when windowsCreateFile and windowsOsRemove fail")
		}
		if !strings.Contains(err.Error(), "open junction") || !strings.Contains(err.Error(), "清理 junction 占位目录失败") {
			t.Fatalf("expected joined error containing create and cleanup errors, got: %v", err)
		}
		var cleanupErr *skillSetupStagingCleanupError
		if !errors.As(err, &cleanupErr) {
			t.Fatalf("expected error to wrap *skillSetupStagingCleanupError, got: %T (%v)", err, err)
		}
	})

	t.Run("device_io_control_error_and_remove_placeholder_error", func(t *testing.T) {
		tempDir := t.TempDir()
		testseam.Swap(t, &windowsDeviceIoControl, func(handle windows.Handle, ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error {
			return errors.New("mock device io control error")
		})
		testseam.Swap(t, &windowsOsRemove, func(path string) error {
			return errors.New("mock remove placeholder error")
		})
		linkPath := filepath.Join(tempDir, "link-ioctl-fail-remove-fail")
		err := createSkillSetupDirLink(tempDir, linkPath)
		if err == nil {
			t.Fatal("expected error when windowsDeviceIoControl and windowsOsRemove fail")
		}
		if !strings.Contains(err.Error(), "set junction reparse point") || !strings.Contains(err.Error(), "清理 junction 占位目录失败") {
			t.Fatalf("expected joined error containing device io control and cleanup errors, got: %v", err)
		}
		var cleanupErr *skillSetupStagingCleanupError
		if !errors.As(err, &cleanupErr) {
			t.Fatalf("expected error to wrap *skillSetupStagingCleanupError, got: %T (%v)", err, err)
		}
	})
}
