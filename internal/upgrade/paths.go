// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// Permission constants following Unix best practices.
const (
	dirPermSecure  os.FileMode = 0o700
	dirPermShared  os.FileMode = 0o755
	filePermBinary os.FileMode = 0o755
	filePermConfig os.FileMode = 0o644
)

// knownSkillDirs lists well-known Agent skill directories (relative to $HOME).
// Used as a fallback when active discovery misses entries.
// Kept in sync with build/npm/install.js AGENT_DIRS.
// The first entry (.agents/skills) is always updated; subsequent entries are
// only updated when their parent directory already exists.
var knownSkillDirs = []string{
	".agents/skills",
	".claude/skills",
	".cursor/skills",
	".gemini/skills",
	".codex/skills",
	".github/skills",
	".windsurf/skills",
	".augment/skills",
	".cline/skills",
	".amp/skills",
	".kiro/skills",
	".trae/skills",
	".openclaw/skills",
}

// skillDirBlacklist contains parent directories whose skills are managed by
// external mechanisms (e.g. IDE extensions) and must NOT be touched by upgrade.
var skillDirBlacklist = []string{
	".real",
}

// DiscoverSkillDirs scans homeDir for agent skill directories by looking for
// any first-level subdirectory that contains a "skills" subdirectory.
// Returns relative paths (e.g. ".claude/skills") suitable for skill installation.
//
// This enables automatic coverage of any new AI agent without requiring
// code changes — any directory matching ~/xxx/skills/ is detected.
func DiscoverSkillDirs(homeDir string) []string {
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return nil
	}

	var discovered []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Build the relative path: .claude/skills, .hermes/skills, etc.
		relPath := filepath.ToSlash(filepath.Join(name, "skills"))
		// Check if the "skills" subdirectory exists
		skillsDir := filepath.Join(homeDir, name, "skills")
		if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
			if !isBlacklisted(relPath) {
				discovered = append(discovered, relPath)
			}
		}
	}

	return discovered
}

// mergeSkillDirs combines discovered and known directories, deduplicating
// while preserving order: primary (.agents/skills) always first, then
// discovered dirs, then any known dirs not already included.
func mergeSkillDirs(discovered, known []string) []string {
	seen := make(map[string]bool)
	var result []string

	// Ensure .agents/skills is always first
	primary := ".agents/skills"
	result = append(result, primary)
	seen[primary] = true

	// Add discovered dirs (skip primary, already added)
	for _, d := range discovered {
		if !seen[d] {
			result = append(result, d)
			seen[d] = true
		}
	}

	// Add known dirs not yet included (fallback for undiscovered agents)
	for _, d := range known {
		if !seen[d] {
			result = append(result, d)
			seen[d] = true
		}
	}

	// Sort everything after the primary entry
	if len(result) > 1 {
		sort.Strings(result[1:])
	}

	return result
}

// removeAllSafe removes a directory safely. If the path is a symlink,
// only the symlink itself is removed (not the target), preventing
// accidental destruction of linked source content.
func removeAllSafe(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// If it's a symlink, remove just the link (not the target)
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

// SkillDirStatus describes the installation outcome for a single skill directory.
type SkillDirStatus int

const (
	SkillDirOK          SkillDirStatus = iota // successfully installed
	SkillDirSkipped                           // agent not detected, directory skipped
	SkillDirBlacklisted                       // blacklisted, never touched
	SkillDirFailed                            // installation attempted but failed
)

// SkillDirResult holds the per-directory install result.
type SkillDirResult struct {
	Dir    string         // destination directory (e.g. ~/.claude/skills/dws)
	Status SkillDirStatus // outcome
	Err    error          // non-nil when Status == SkillDirFailed
}

// SkillUpgradeResult aggregates the outcome of an UpgradeSkillLocations call.
type SkillUpgradeResult struct {
	Results []SkillDirResult
}

// Succeeded returns directories that were successfully updated.
func (r *SkillUpgradeResult) Succeeded() []SkillDirResult {
	var out []SkillDirResult
	for _, d := range r.Results {
		if d.Status == SkillDirOK {
			out = append(out, d)
		}
	}
	return out
}

// Failed returns directories where installation was attempted but failed.
func (r *SkillUpgradeResult) Failed() []SkillDirResult {
	var out []SkillDirResult
	for _, d := range r.Results {
		if d.Status == SkillDirFailed {
			out = append(out, d)
		}
	}
	return out
}

// UpgradeSkillLocations installs skills from extractedDir into all locations
// where they are currently installed or expected.
//
// Strategy:
//   - First, scan $HOME for any */skills/ directories (active discovery)
//   - Merge discovered dirs with knownSkillDirs (fallback) for full coverage
//   - ~/.agents/skills/dws/ is ALWAYS updated (primary install location)
//   - Other agent dirs are updated only when the parent directory exists
//   - Blacklisted paths (e.g. ~/.real/) are NEVER touched
//   - Symlink destinations are handled safely (link removed, not target)
//   - If no location was updated at all, fall back to ~/.agents/skills/dws/
func UpgradeSkillLocations(extractedDir string) (*SkillUpgradeResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &SkillUpgradeResult{}

	// Phase 1: Active discovery — scan $HOME for any */skills/ directories
	discovered := DiscoverSkillDirs(homeDir)

	// Phase 2: Merge with known list for full coverage
	allDirs := mergeSkillDirs(discovered, knownSkillDirs)

	for i, agentDir := range allDirs {
		destDir := filepath.Join(homeDir, agentDir, "dws")

		if isBlacklisted(agentDir) {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirBlacklisted})
			continue
		}

		// Primary location (.agents/skills) is always updated;
		// others require the parent directory to exist
		if i > 0 {
			parentGate := filepath.Dir(filepath.Join(homeDir, agentDir))
			if _, err := os.Stat(parentGate); os.IsNotExist(err) {
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirSkipped})
				continue
			}
		}

		removeAllSafe(destDir)
		if err := copyDir(extractedDir, destDir); err != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirFailed, Err: err})
			continue
		}
		result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirOK})
	}

	// Fallback: if nothing succeeded, force the primary location
	if len(result.Succeeded()) == 0 {
		dest := filepath.Join(homeDir, ".agents", "skills", "dws")
		os.MkdirAll(filepath.Dir(dest), dirPermShared)
		if err := copyDir(extractedDir, dest); err != nil {
			return result, fmt.Errorf("所有技能目录安装失败，回退到主目录也失败: %w", err)
		}
		// Replace the earlier failed entry for this dir (if any) or append a new one
		replaced := false
		for idx, r := range result.Results {
			if r.Dir == dest {
				result.Results[idx] = SkillDirResult{Dir: dest, Status: SkillDirOK}
				replaced = true
				break
			}
		}
		if !replaced {
			result.Results = append(result.Results, SkillDirResult{Dir: dest, Status: SkillDirOK})
		}
	}

	return result, nil
}

// LocateSkillMD finds the directory containing SKILL.md in an extracted zip.
// It handles both flat layouts (SKILL.md at root) and nested layouts (dws/SKILL.md).
func LocateSkillMD(extractDir string) string {
	// Check nested: {extractDir}/dws/SKILL.md
	nested := filepath.Join(extractDir, "dws", "SKILL.md")
	if _, err := os.Stat(nested); err == nil {
		return filepath.Join(extractDir, "dws")
	}

	// Check flat: {extractDir}/SKILL.md
	flat := filepath.Join(extractDir, "SKILL.md")
	if _, err := os.Stat(flat); err == nil {
		return extractDir
	}

	return ""
}

// EnsureUpgradeDirectories creates the directories needed for upgrade operations.
func EnsureUpgradeDirectories() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{filepath.Join(homeDir, ".dws"), dirPermSecure},
		{filepath.Join(homeDir, ".dws", "data"), dirPermSecure},
		{filepath.Join(homeDir, ".dws", "data", "backups"), dirPermSecure},
		{filepath.Join(homeDir, ".dws", "cache"), dirPermSecure},
		{filepath.Join(homeDir, ".dws", "cache", "downloads"), dirPermSecure},
	}

	for _, d := range dirs {
		if err := ensureDir(d.path, d.perm); err != nil {
			return err
		}
	}
	return nil
}

// DownloadCacheDir returns the path for temporary downloads during upgrade.
func DownloadCacheDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".dws", "cache", "downloads")
}

// CurrentBinaryPath returns the resolved path of the currently running binary.
func CurrentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// BinaryName returns the platform-specific binary name.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "dws.exe"
	}
	return "dws"
}

func isBlacklisted(agentDir string) bool {
	for _, bl := range skillDirBlacklist {
		// agentDir is like ".real/skills" — check if it starts with a blacklisted prefix
		if len(agentDir) >= len(bl) && agentDir[:len(bl)] == bl {
			next := len(bl)
			if next == len(agentDir) || agentDir[next] == '/' {
				return true
			}
		}
	}
	return false
}

func ensureDir(path string, perm os.FileMode) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, perm)
	}
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != perm {
		if info.Mode().Perm()&^perm != 0 {
			return os.Chmod(path, perm)
		}
	}
	return nil
}
