//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"
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
	if _, err := junctionSubstituteName(""); err == nil {
		t.Fatal("expected error for empty target")
	}
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
		got, err := junctionSubstituteName(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("junctionSubstituteName(%q) = (%q, %v), want %q", tc.input, got, err, tc.want)
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

	// 4a. link cannot be created because a file exists at that path
	existingFile := filepath.Join(tempDir, "existing-file")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := createSkillSetupDirLink(tempDir, existingFile); err == nil {
		t.Fatal("expected error when link path is an existing file")
	}

	// 4b. link path has NUL byte (exercises linkPath encode error and removeLink defer)
	if err := createSkillSetupDirLink(tempDir, filepath.Join(tempDir, "link\x00bad")); err == nil {
		t.Fatal("expected error when link path has NUL byte")
	}

	// 4c. empty target (exercises junctionSubstituteName error and removeLink defer)
	linkPath1 := filepath.Join(tempDir, "link-empty-target")
	if err := createSkillSetupDirLink("", linkPath1); err == nil {
		t.Fatal("expected error when target is empty")
	}
	if _, err := os.Stat(linkPath1); !os.IsNotExist(err) {
		t.Fatalf("link path should be cleaned up on error, got %v", err)
	}

	// 4d. target has NUL byte (exercises mountPointReparseBuffer error and removeLink defer)
	linkPath2 := filepath.Join(tempDir, "link-bad-target")
	if err := createSkillSetupDirLink("C:\\bad\x00target", linkPath2); err == nil {
		t.Fatal("expected error when target has NUL byte")
	}
	if _, err := os.Stat(linkPath2); !os.IsNotExist(err) {
		t.Fatalf("link path should be cleaned up on error, got %v", err)
	}

	// 4e. target is a file instead of directory (DeviceIoControl fails, exercises removeLink defer)
	targetFile := filepath.Join(tempDir, "target-is-file")
	if err := os.WriteFile(targetFile, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath3 := filepath.Join(tempDir, "link-to-file")
	if err := createSkillSetupDirLink(targetFile, linkPath3); err == nil {
		t.Fatal("expected error when junction target is a file")
	}
	if _, err := os.Stat(linkPath3); !os.IsNotExist(err) {
		t.Fatalf("link path should be cleaned up on error, got %v", err)
	}
}
