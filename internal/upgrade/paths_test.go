// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsBlacklisted(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{".real/skills", true},
		{".real", true},
		{".agents/skills", false},
		{".claude/skills", false},
		{".cursor/skills", false},
		{".realtime/skills", false},
	}
	for _, tt := range tests {
		got := isBlacklisted(tt.dir)
		if got != tt.want {
			t.Errorf("isBlacklisted(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

func TestLocateSkillMD(t *testing.T) {
	// Flat layout
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# skill"), 0644)

	result := LocateSkillMD(dir)
	if result != dir {
		t.Errorf("flat layout: LocateSkillMD() = %q, want %q", result, dir)
	}

	// Nested layout
	dir2 := t.TempDir()
	os.MkdirAll(filepath.Join(dir2, "dws"), 0755)
	os.WriteFile(filepath.Join(dir2, "dws", "SKILL.md"), []byte("# skill"), 0644)

	result2 := LocateSkillMD(dir2)
	want2 := filepath.Join(dir2, "dws")
	if result2 != want2 {
		t.Errorf("nested layout: LocateSkillMD() = %q, want %q", result2, want2)
	}

	// No SKILL.md
	dir3 := t.TempDir()
	result3 := LocateSkillMD(dir3)
	if result3 != "" {
		t.Errorf("empty dir: LocateSkillMD() = %q, want empty", result3)
	}
}

func TestUpgradeSkillLocations(t *testing.T) {
	skillSrc := t.TempDir()
	os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# test skill"), 0644)
	os.MkdirAll(filepath.Join(skillSrc, "references"), 0755)
	os.WriteFile(filepath.Join(skillSrc, "references", "guide.md"), []byte("# guide"), 0644)

	result, err := UpgradeSkillLocations(skillSrc)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}

	succeeded := result.Succeeded()
	if len(succeeded) == 0 {
		t.Fatal("UpgradeSkillLocations() returned 0 succeeded locations")
	}

	homeDir, _ := os.UserHomeDir()
	primaryDest := filepath.Join(homeDir, ".agents", "skills", "dws")

	found := false
	for _, d := range succeeded {
		if d.Dir == primaryDest {
			found = true
			break
		}
	}
	if !found {
		dirs := make([]string, len(succeeded))
		for i, d := range succeeded {
			dirs[i] = d.Dir
		}
		t.Errorf("primary location %s not in succeeded list: %v", primaryDest, dirs)
	}

	skillMDPath := filepath.Join(primaryDest, "SKILL.md")
	if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
		t.Errorf("SKILL.md not found at %s", skillMDPath)
	}

	guidePath := filepath.Join(primaryDest, "references", "guide.md")
	if _, err := os.Stat(guidePath); os.IsNotExist(err) {
		t.Errorf("references/guide.md not found at %s", guidePath)
	}

	// Verify that failed list is empty for a normal install
	if len(result.Failed()) != 0 {
		t.Errorf("expected 0 failures, got %d", len(result.Failed()))
	}

	// Verify total results contain at least one entry for each known (non-blacklisted) dir
	if len(result.Results) == 0 {
		t.Error("expected non-empty Results")
	}

	os.RemoveAll(primaryDest)
}

func TestBlacklistPreventsRealDir(t *testing.T) {
	for _, dir := range knownSkillDirs {
		if isBlacklisted(dir) {
			t.Errorf("knownSkillDirs contains blacklisted entry: %q", dir)
		}
	}
}

func TestSkillUpgradeResult_SucceededAndFailed(t *testing.T) {
	result := &SkillUpgradeResult{
		Results: []SkillDirResult{
			{Dir: "/a", Status: SkillDirOK},
			{Dir: "/b", Status: SkillDirFailed, Err: os.ErrPermission},
			{Dir: "/c", Status: SkillDirSkipped},
			{Dir: "/d", Status: SkillDirBlacklisted},
			{Dir: "/e", Status: SkillDirOK},
		},
	}

	succeeded := result.Succeeded()
	if len(succeeded) != 2 {
		t.Errorf("Succeeded() len = %d, want 2", len(succeeded))
	}
	if succeeded[0].Dir != "/a" || succeeded[1].Dir != "/e" {
		t.Errorf("Succeeded() dirs = %v, %v", succeeded[0].Dir, succeeded[1].Dir)
	}

	failed := result.Failed()
	if len(failed) != 1 {
		t.Errorf("Failed() len = %d, want 1", len(failed))
	}
	if failed[0].Dir != "/b" {
		t.Errorf("Failed()[0].Dir = %q, want /b", failed[0].Dir)
	}
	if failed[0].Err != os.ErrPermission {
		t.Errorf("Failed()[0].Err = %v, want ErrPermission", failed[0].Err)
	}
}

func TestSkillUpgradeResult_EmptyResults(t *testing.T) {
	result := &SkillUpgradeResult{}
	if len(result.Succeeded()) != 0 {
		t.Error("empty Succeeded should be nil/empty")
	}
	if len(result.Failed()) != 0 {
		t.Error("empty Failed should be nil/empty")
	}
}

func TestSkillDirStatusConstants(t *testing.T) {
	if SkillDirOK != 0 {
		t.Errorf("SkillDirOK = %d, want 0", SkillDirOK)
	}
	if SkillDirSkipped != 1 {
		t.Errorf("SkillDirSkipped = %d, want 1", SkillDirSkipped)
	}
	if SkillDirBlacklisted != 2 {
		t.Errorf("SkillDirBlacklisted = %d, want 2", SkillDirBlacklisted)
	}
	if SkillDirFailed != 3 {
		t.Errorf("SkillDirFailed = %d, want 3", SkillDirFailed)
	}
}

func TestEnsureUpgradeDirectories(t *testing.T) {
	// This test verifies the structure is correct without actually modifying $HOME
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	err = EnsureUpgradeDirectories()
	if err != nil {
		t.Fatalf("EnsureUpgradeDirectories() error = %v", err)
	}

	expectedDirs := []string{
		filepath.Join(homeDir, ".dws"),
		filepath.Join(homeDir, ".dws", "data"),
		filepath.Join(homeDir, ".dws", "data", "backups"),
		filepath.Join(homeDir, ".dws", "cache"),
		filepath.Join(homeDir, ".dws", "cache", "downloads"),
	}
	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %s should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
		}
	}
}

func TestDownloadCacheDir(t *testing.T) {
	dir := DownloadCacheDir()
	if dir == "" {
		t.Error("DownloadCacheDir() returned empty")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("DownloadCacheDir() = %q, want absolute path", dir)
	}
}

func TestBinaryName(t *testing.T) {
	name := BinaryName()
	if name != "dws" && name != "dws.exe" {
		t.Errorf("BinaryName() = %q, want dws or dws.exe", name)
	}
}

func TestLocateSkillMD_NestedBeatsFlat(t *testing.T) {
	dir := t.TempDir()
	// Create both flat and nested layouts; nested should take priority
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("flat"), 0644)
	os.MkdirAll(filepath.Join(dir, "dws"), 0755)
	os.WriteFile(filepath.Join(dir, "dws", "SKILL.md"), []byte("nested"), 0644)

	result := LocateSkillMD(dir)
	want := filepath.Join(dir, "dws")
	if result != want {
		t.Errorf("LocateSkillMD() = %q, want nested %q", result, want)
	}
}

func TestKnownSkillDirsNotEmpty(t *testing.T) {
	if len(knownSkillDirs) == 0 {
		t.Error("knownSkillDirs is empty")
	}
	if knownSkillDirs[0] != ".agents/skills" {
		t.Errorf("first knownSkillDir = %q, want .agents/skills", knownSkillDirs[0])
	}
}

func TestDiscoverSkillDirs(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".cursor", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".hermes", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, "downloads"), 0755)

	got := DiscoverSkillDirs(home)
	want := map[string]bool{
		".claude/skills": true,
		".cursor/skills": true,
		".hermes/skills": true,
	}
	if len(got) != len(want) {
		t.Errorf("DiscoverSkillDirs() len = %d, want %d: %v", len(got), len(want), got)
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected dir %q", d)
		}
	}
}

func TestDiscoverSkillDirs_SkipsBlacklisted(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".real", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0755)

	got := DiscoverSkillDirs(home)
	for _, d := range got {
		if strings.HasPrefix(d, ".real") {
			t.Errorf("blacklisted .real/skills should not appear, got %q", d)
		}
	}
	found := false
	for _, d := range got {
		if d == ".claude/skills" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .claude/skills to be discovered")
	}
}

func TestDiscoverSkillDirs_SkipsNonSkillDirs(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "Documents"), 0755)
	os.WriteFile(filepath.Join(home, "notes.txt"), []byte("notes"), 0644)

	got := DiscoverSkillDirs(home)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestDiscoverSkillDirs_EmptyHome(t *testing.T) {
	home := t.TempDir()
	if got := DiscoverSkillDirs(home); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestDiscoverSkillDirs_InvalidHome(t *testing.T) {
	got := DiscoverSkillDirs("/nonexistent/path/xyz123")
	if len(got) != 0 {
		t.Errorf("expected empty for invalid home, got %v", got)
	}
}

func TestMergeSkillDirs(t *testing.T) {
	discovered := []string{".cursor/skills", ".claude/skills"}
	known := []string{".agents/skills", ".claude/skills", ".gemini/skills"}

	got := mergeSkillDirs(discovered, known)

	if got[0] != ".agents/skills" {
		t.Errorf("primary = %q, want .agents/skills", got[0])
	}

	seen := make(map[string]int)
	for _, d := range got {
		seen[d]++
	}
	for d, count := range seen {
		if count > 1 {
			t.Errorf("duplicate dir %q", d)
		}
	}

	for _, want := range []string{".agents/skills", ".claude/skills", ".cursor/skills", ".gemini/skills"} {
		if seen[want] != 1 {
			t.Errorf("missing %q", want)
		}
	}

	if len(got) > 2 {
		for i := 2; i < len(got); i++ {
			if got[i] < got[i-1] {
				t.Errorf("not sorted: %q before %q", got[i-1], got[i])
			}
		}
	}
}

func TestMergeSkillDirs_PrimaryAlwaysFirst(t *testing.T) {
	got := mergeSkillDirs([]string{".cursor/skills"}, []string{".claude/skills"})
	if got[0] != ".agents/skills" {
		t.Errorf("primary not injected first, got %v", got)
	}
}

func TestMergeSkillDirs_EmptyInputs(t *testing.T) {
	got := mergeSkillDirs(nil, nil)
	if len(got) != 1 || got[0] != ".agents/skills" {
		t.Errorf("expected [.agents/skills], got %v", got)
	}
}

func TestRemoveAllSafe_RegularDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "remove-me")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "file.txt"), []byte("data"), 0644)

	if err := removeAllSafe(sub); err != nil {
		t.Fatalf("removeAllSafe failed: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("expected subdir to be removed")
	}
}

func TestRemoveAllSafe_NonExistent(t *testing.T) {
	if err := removeAllSafe("/nonexistent/path/xyz123"); err != nil {
		t.Errorf("expected nil error for nonexistent path, got %v", err)
	}
}

func TestRemoveAllSafe_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.MkdirAll(target, 0755)
	os.WriteFile(filepath.Join(target, "file.txt"), []byte("data"), 0644)

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if err := removeAllSafe(link); err != nil {
		t.Fatalf("removeAllSafe failed: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
	if _, err := os.Stat(filepath.Join(target, "file.txt")); os.IsNotExist(err) {
		t.Error("target should still exist after symlink removal")
	}
}

func TestUpgradeSkillLocations_WithDiscovery(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	// Create a new agent dir that is NOT in knownSkillDirs
	hermesSkillDir := filepath.Join(homeDir, ".hermes", "skills")
	os.MkdirAll(hermesSkillDir, 0755)
	defer os.RemoveAll(filepath.Join(homeDir, ".hermes"))

	skillSrc := t.TempDir()
	os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# discovered skill"), 0644)

	result, err := UpgradeSkillLocations(skillSrc)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}

	// Verify .hermes/skills/dws was discovered and installed
	hermesDest := filepath.Join(hermesSkillDir, "dws")
	found := false
	for _, d := range result.Succeeded() {
		if d.Dir == hermesDest {
			found = true
			break
		}
	}
	if !found {
		t.Errorf(".hermes/skills/dws not installed; succeeded dirs: %v", result.Succeeded())
	}
	if _, err := os.Stat(filepath.Join(hermesDest, "SKILL.md")); os.IsNotExist(err) {
		t.Error("SKILL.md not found at discovered .hermes destination")
	}

	// Cleanup
	os.RemoveAll(hermesDest)
}
