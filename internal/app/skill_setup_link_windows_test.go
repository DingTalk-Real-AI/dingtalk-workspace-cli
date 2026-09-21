//go:build windows

package app

import (
	"errors"
	"os"
	"path/filepath"
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

	// 3. mountPointReparseBuffer NUL byte error branches
	if _, err := mountPointReparseBuffer("bad\x00sub", "print"); err == nil {
		t.Fatal("expected error for NUL in substitute")
	}
	if _, err := mountPointReparseBuffer("sub", "bad\x00print"); err == nil {
		t.Fatal("expected error for NUL in printName")
	}

	// 4. createSkillSetupDirLink error branches
	tempDir := t.TempDir()

	// 4a. empty target
	linkPath1 := filepath.Join(tempDir, "link-empty-target")
	if err := createSkillSetupDirLink("", linkPath1); err == nil {
		t.Fatal("expected error when target is empty")
	}
	if _, err := os.Stat(linkPath1); !os.IsNotExist(err) {
		t.Fatalf("link path should not exist, got %v", err)
	}

	// 4b. filepathAbs error
	testseam.Swap(t, &filepathAbs, func(string) (string, error) {
		return "", errors.New("abs error")
	})
	if err := createSkillSetupDirLink(tempDir, filepath.Join(tempDir, "link-abs-err")); err == nil {
		t.Fatal("expected error when filepathAbs fails")
	}

	// 4c. mountPointReparseBuffer error via target with NUL byte
	testseam.Swap(t, &filepathAbs, func(s string) (string, error) { return s, nil })
	if err := createSkillSetupDirLink("C:\\bad\x00target", filepath.Join(tempDir, "link-bad-target")); err == nil {
		t.Fatal("expected error when target has NUL byte")
	}

	// 4d. link path has NUL byte
	if err := createSkillSetupDirLink(tempDir, filepath.Join(tempDir, "link\x00bad")); err == nil {
		t.Fatal("expected error when link path has NUL byte")
	}

	// 4e. link cannot be created because a file exists at that path (os.Mkdir fails)
	existingFile := filepath.Join(tempDir, "existing-file")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := createSkillSetupDirLink(tempDir, existingFile); err == nil {
		t.Fatal("expected error when link path is an existing file")
	}

	// 4f. windowsCreateFile fails (exercises removeLink defer cleanup)
	testseam.Swap(t, &windowsCreateFile, func(path *uint16, access uint32, shareMode uint32, sa *windows.SecurityAttributes, creationDisposition uint32, flagsAndAttributes uint32, templateFile windows.Handle) (windows.Handle, error) {
		return windows.InvalidHandle, errors.New("mock create file error")
	})
	linkPathCreateFail := filepath.Join(tempDir, "link-create-fail")
	if err := createSkillSetupDirLink(tempDir, linkPathCreateFail); err == nil {
		t.Fatal("expected error when windowsCreateFile fails")
	}
	if _, err := os.Stat(linkPathCreateFail); !os.IsNotExist(err) {
		t.Fatalf("link path should be cleaned up on windowsCreateFile error, got %v", err)
	}

	// 4g. windowsDeviceIoControl fails (exercises removeLink defer cleanup)
	testseam.Swap(t, &windowsDeviceIoControl, func(handle windows.Handle, ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error {
		return errors.New("mock device io control error")
	})
	linkPathIoctlFail := filepath.Join(tempDir, "link-ioctl-fail")
	if err := createSkillSetupDirLink(tempDir, linkPathIoctlFail); err == nil {
		t.Fatal("expected error when windowsDeviceIoControl fails")
	}
	if _, err := os.Stat(linkPathIoctlFail); !os.IsNotExist(err) {
		t.Fatalf("link path should be cleaned up on windowsDeviceIoControl error, got %v", err)
	}
}
