// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
)

// Permission constants following Unix best practices.
const (
	dirPermSecure  os.FileMode = 0o700
	dirPermShared  os.FileMode = 0o755
	filePermBinary os.FileMode = 0o755
	filePermConfig os.FileMode = 0o644
)

// knownSkillDirs lists all known Agent skill directories (relative to $HOME).
// Kept in sync with:
//   - build/npm/install.js                                  AGENT_DIRS
//   - scripts/install.sh                                    for-in list
//   - scripts/install.ps1                                   $AgentDirs
//   - scripts/install-skills.sh                             for-in list
//   - build/homebrew.rb.tmpl                                targets
//   - test/scripts/package_script_test.go                   expectedPackagedSkillTargets
//   - scripts/release/verify-package-managers.sh            HOME_AGENT_PARENTS / HOME_SKILL_TARGETS
//
// The first entry (.agents/skills) is a generic fallback. It is used only
// when no concrete Agent home is detected; otherwise publishing there would
// duplicate every Skill for Agents (including Codex) that scan both roots.
// Subsequent entries are updated when their parent directory already exists.
var knownSkillDirs = []string{
	".agents/skills",
	".claude/skills",
	".cursor/skills",
	".qoder/skills",
	".qoderwork/skills",
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
	".hermes/skills",
}

var (
	upgradeUserHomeDir     = os.UserHomeDir
	upgradeExecutable      = os.Executable
	upgradeEvalSymlinks    = filepath.EvalSymlinks
	upgradeCopyDir         = copyDir
	upgradeEnsureDir       = ensureDir
	upgradeRemoveAll       = os.RemoveAll
	upgradeMkdirAll        = os.MkdirAll
	upgradeMkdirTemp       = os.MkdirTemp
	upgradeReadDir         = os.ReadDir
	upgradeStat            = os.Stat
	upgradeBuildProvenance = skillprovenance.Build
	upgradeReadSkillState  = skillstate.Read
	upgradeBackupStamp     = func() string { return time.Now().UTC().Format("20060102-150405") }
	upgradeWriteSkillState = skillstate.Write
	upgradeNow             = time.Now
)

// skillBackupSubdir is the user-level directory where skill directories are
// preserved before a layout-changing install/upgrade removes them. Non-
// interactive flows (install scripts, npm postinstall, `dws upgrade`) cannot
// ask for confirmation, so deletions must stay reversible instead.
const skillBackupSubdir = ".dws/skill-backups"

// backupAndRemoveSkillDir moves dir into <homeDir>/.dws/skill-backups/
// <stamp>/<rel> instead of destroying it, and returns the backup path. It is
// fail-safe: a directory that cannot be backed up is NOT removed and the
// error is returned so the caller can surface it (and never install the
// opposite layout next to it silently). Missing paths and regular files are
// no-ops ("", nil).
func backupAndRemoveSkillDir(homeDir, dir string) (string, error) {
	info, err := upgradeStat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("检查技能目录失败 %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", nil
	}
	rel, relErr := filepath.Rel(homeDir, dir)
	if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(dir)
	}
	name := strings.NewReplacer(string(filepath.Separator), "-", "/", "-").Replace(rel)
	stamp := upgradeBackupStamp()
	backupRoot := filepath.Join(homeDir, skillBackupSubdir, stamp)
	target := filepath.Join(backupRoot, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		backupRoot = filepath.Join(homeDir, skillBackupSubdir, fmt.Sprintf("%s-%d", stamp, i))
		target = filepath.Join(backupRoot, name)
		if i > 1000 {
			return "", fmt.Errorf("备份目录冲突无法解决: %s", target)
		}
	}
	if err := upgradeMkdirAll(backupRoot, dirPermShared); err != nil {
		return "", fmt.Errorf("创建备份目录失败 %s: %w", backupRoot, err)
	}
	if err := upgradeRename(dir, target); err != nil {
		return "", fmt.Errorf("备份技能目录失败 %s: %w", dir, err)
	}
	// Keep the backup directory bounded; a prune failure must not fail the
	// install (the backup itself succeeded).
	_ = pruneSkillBackups(homeDir)
	return target, nil
}

// BackupAndRemoveSkillDir is the exported wrapper over
// backupAndRemoveSkillDir for callers outside the upgrade package (the
// skill-setup channel in internal/app).
func BackupAndRemoveSkillDir(homeDir, dir string) (string, error) {
	return backupAndRemoveSkillDir(homeDir, dir)
}

// skillDirBlacklist contains parent directories whose skills are managed by
// external mechanisms (e.g. IDE extensions) and must NOT be touched by upgrade.
var skillDirBlacklist = []string{
	".real",
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

// UpgradeSkillLocations refreshes skills from extractedDir into agent homes.
// extractedDir may be a multi-skill bundle root (subdirectories each containing
// SKILL.md) or a legacy mono root (SKILL.md at its top level). Callers that
// resolve a release zip usually pass LocateSkillsRoot's result (multi/
// preferred when present).
//
// Package-driven layout:
//   - release zip has multi/ → install and overwrite the complete official
//     bundle. Locally deleted bundled skills are restored on the next upgrade;
//     local absence is never treated as a persistent exclusion.
//   - dingtalk-shared is mandatory whenever it exists in the bundle.
//   - legacy zip with no multi tree → mono refresh path (unchanged fallback)
//
// Fresh install defaults to multi with opt-in mono via the interactive
// `dws skill setup --mode mono` flow.
//
// Strategy (matches npm install.js installSkillsToHomes):
//   - Concrete agent dirs (claude, cursor, codex, ...) are updated when their
//     parent directory exists (e.g. ~/.codex/ exists => user has Codex)
//   - ~/.agents/skills/ is used only when no concrete Agent is detected
//   - ~/.real/ and other blacklisted paths are NEVER touched
//   - If no location was updated at all, fall back to ~/.agents/skills/
//
// Opposite-mode leftovers are backed up to ~/.dws/skill-backups/ and then
// removed so mono and multi never co-exist after an upgrade; a leftover that
// cannot be backed up is never removed and fails that home. Same-name bundle
// skills are refreshed in place (verified DWS-managed overwrite). Caches
// under ~/.dws/skills/{multi,mono} are refreshed best-effort.
func UpgradeSkillLocations(extractedDir string) (*SkillUpgradeResult, error) {
	return UpgradeSkillLocationsWithOptions(extractedDir, SkillUpgradeOptions{})
}

type SkillUpgradeOptions struct {
	Version string
}

func UpgradeSkillLocationsWithOptions(extractedDir string, opts SkillUpgradeOptions) (*SkillUpgradeResult, error) {
	homeDir, err := upgradeUserHomeDir()
	if err != nil {
		return nil, err
	}

	multiRoot, skills := resolveMultiBundle(extractedDir)
	if len(skills) > 0 {
		official := append([]string(nil), skills...)
		managedSkills, provenanceErr := buildUpgradeProvenanceRecords(multiRoot, official, opts.Version)
		if provenanceErr != nil {
			return nil, fmt.Errorf("生成统一 Skill provenance 失败: %w", provenanceErr)
		}
		result, installErr := upgradeMultiSkillLocations(homeDir, multiRoot, official)
		if installErr != nil {
			return result, installErr
		}
		if len(result.Failed()) == 0 && len(result.Succeeded()) > 0 {
			state := skillstate.State{
				Version:        opts.Version,
				OfficialSkills: official,
				UpdatedSkills:  official,
				ManagedSkills:  managedSkills,
				UpdatedAt:      upgradeNow().UTC().Format(time.RFC3339),
			}
			if writeErr := upgradeWriteSkillState(homeDir, state); writeErr != nil {
				return result, fmt.Errorf("skill 已同步但状态未写入: %w", writeErr)
			}
		}
		return result, nil
	}
	monoSrc := resolveMonoSkillSrc(extractedDir)
	if monoSrc != "" {
		return upgradeMonoSkillLocations(homeDir, monoSrc)
	}
	return nil, fmt.Errorf("升级包中找不到可安装的 skill 源")
}

func buildUpgradeProvenanceRecords(root string, names []string, version string) ([]skillprovenance.Record, error) {
	records := make([]skillprovenance.Record, 0, len(names))
	for _, name := range names {
		record, err := upgradeBuildProvenance(name, filepath.Join(root, name), version, skillprovenance.SourceUpgrade)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// skillBackupKeep limits ~/.dws/skill-backups/ growth: only the newest
// backups are kept.
const skillBackupKeep = 5

// pruneSkillBackups removes the oldest backup directories when more than
// skillBackupKeep remain. Best-effort: a removal failure never aborts, but
// pruning failures are reported so callers can warn the user.
func pruneSkillBackups(homeDir string) error {
	root := filepath.Join(homeDir, skillBackupSubdir)
	entries, err := upgradeReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var firstErr error
	for len(names) > skillBackupKeep {
		old := filepath.Join(root, names[0])
		names = names[1:]
		if err := upgradeRemoveAll(old); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// resolveMultiBundle returns the multi skill root and its skill names when
// extractedDir is itself a bundle, or when a multi/ child of the parent
// extract root carries one (LocateSkillsRoot already prefers that child).
func resolveMultiBundle(extractedDir string) (string, []string) {
	if skills := bundleSkillNames(extractedDir); len(skills) > 0 {
		return extractedDir, skills
	}
	child := filepath.Join(extractedDir, "multi")
	if skills := bundleSkillNames(child); len(skills) > 0 {
		return child, skills
	}
	return "", nil
}

// resolveMonoSkillSrc finds a mono skill tree for the legacy mono-only
// package fallback: the path itself, a sibling mono/ next to a multi root,
// or the extract-root SKILL.md copy that release zips still ship.
func resolveMonoSkillSrc(extractedDir string) string {
	if skillTreeHasRoot(extractedDir) {
		return extractedDir
	}
	sibling := filepath.Join(filepath.Dir(extractedDir), "mono")
	if skillTreeHasRoot(sibling) {
		return sibling
	}
	parent := filepath.Dir(extractedDir)
	if skillTreeHasRoot(parent) {
		return parent
	}
	child := filepath.Join(extractedDir, "mono")
	if skillTreeHasRoot(child) {
		return child
	}
	return ""
}

type skillStageSpec struct {
	src  string
	dest string
}

type stagedSkillDir struct {
	staged string
	dest   string
}

type backedUpSkillDir struct {
	original string
	backup   string
}

// stageSkillSet builds a complete replacement next to its final destinations.
// Nothing Agent-visible is changed until every copy succeeds.
func stageSkillSet(destBase, prefix string, specs []skillStageSpec) (stageRoot string, staged []stagedSkillDir, err error) {
	if err := upgradeMkdirAll(destBase, dirPermShared); err != nil {
		return "", nil, fmt.Errorf("创建 Skill 目标目录失败 %s: %w", destBase, err)
	}
	stageRoot, err = upgradeMkdirTemp(destBase, prefix)
	if err != nil {
		return "", nil, fmt.Errorf("创建 Skill staging 失败 %s: %w", destBase, err)
	}
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := upgradeRemoveAll(stageRoot); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理 Skill staging 失败 %s: %w", stageRoot, cleanupErr))
		}
	}()

	staged = make([]stagedSkillDir, 0, len(specs))
	for _, spec := range specs {
		stageDir := filepath.Join(stageRoot, filepath.Base(spec.dest))
		if err := upgradeCopyDir(spec.src, stageDir); err != nil {
			return stageRoot, nil, fmt.Errorf("拷贝 Skill staging 失败 %s: %w", stageDir, err)
		}
		staged = append(staged, stagedSkillDir{staged: stageDir, dest: spec.dest})
	}
	return stageRoot, staged, nil
}

func uniqueSkillDirs(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		unique = append(unique, path)
	}
	return unique
}

// restoreSkillSet removes any newly published directories, then restores all
// original directories in reverse backup order.
func restoreSkillSet(published []string, backups []backedUpSkillDir) error {
	var restoreErr error
	for i := len(published) - 1; i >= 0; i-- {
		if err := upgradeRemoveAll(published[i]); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("移除失败发布目录 %s: %w", published[i], err))
		}
	}
	for i := len(backups) - 1; i >= 0; i-- {
		backup := backups[i]
		if err := upgradeMkdirAll(filepath.Dir(backup.original), dirPermShared); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("创建 Skill 恢复目录 %s: %w", filepath.Dir(backup.original), err))
			continue
		}
		if err := upgradeRename(backup.backup, backup.original); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复原 Skill 失败 %s: %w", backup.original, err))
		}
	}
	return restoreErr
}

// backupSkillSet moves every victim aside as one logical operation. If a
// later backup fails, earlier moves are restored before the error is returned.
func backupSkillSet(homeDir string, victims []string) ([]backedUpSkillDir, error) {
	backups := make([]backedUpSkillDir, 0, len(victims))
	for _, victim := range uniqueSkillDirs(victims) {
		backup, err := backupAndRemoveSkillDir(homeDir, victim)
		if err != nil {
			if restoreErr := restoreSkillSet(nil, backups); restoreErr != nil {
				return nil, errors.Join(err, fmt.Errorf("恢复已备份 Skill 失败: %w", restoreErr))
			}
			return nil, err
		}
		if backup != "" {
			backups = append(backups, backedUpSkillDir{original: victim, backup: backup})
		}
	}
	return backups, nil
}

// publishStagedSkillSet switches a fully staged set into place. Any publish
// failure removes the partial new set and restores every original directory.
func publishStagedSkillSet(homeDir string, staged []stagedSkillDir, victims []string) error {
	backups, err := backupSkillSet(homeDir, victims)
	if err != nil {
		return err
	}
	published := make([]string, 0, len(staged))
	for _, skill := range staged {
		if err := upgradeRename(skill.staged, skill.dest); err != nil {
			publishErr := fmt.Errorf("发布 Skill 失败 %s: %w", skill.dest, err)
			if restoreErr := restoreSkillSet(published, backups); restoreErr != nil {
				return errors.Join(publishErr, fmt.Errorf("恢复原 Skill 集合失败: %w", restoreErr))
			}
			return publishErr
		}
		published = append(published, skill.dest)
	}
	return nil
}

func monoUpgradeVictims(baseDir, destDir string, managed ...map[string]bool) ([]string, error) {
	victims, err := managedMultiSkillVictims(baseDir, managed...)
	if err != nil {
		return nil, err
	}
	return append(victims, destDir), nil
}

func multiUpgradeVictims(destBase string, skillSet map[string]bool, skills []string, managed ...map[string]bool) ([]string, error) {
	victims, err := oppositeModeSkillVictims(destBase, skillSet, managed...)
	if err != nil {
		return nil, err
	}
	for _, name := range skills {
		victims = append(victims, filepath.Join(destBase, name))
	}
	return victims, nil
}

func publishMonoUpgradeTarget(homeDir, destBase, skillSrc string, managed ...map[string]bool) error {
	destDir := filepath.Join(destBase, "dws")
	victims, err := monoUpgradeVictims(destBase, destDir, managed...)
	if err != nil {
		return err
	}
	stageRoot, staged, err := stageSkillSet(destBase, ".dws-upgrade-mono-", []skillStageSpec{{src: skillSrc, dest: destDir}})
	if err != nil {
		return err
	}
	defer func() { _ = upgradeRemoveAll(stageRoot) }()
	return publishStagedSkillSet(homeDir, staged, victims)
}

func publishMultiUpgradeTarget(homeDir, destBase, multiRoot string, skills []string, skillSet map[string]bool, managed ...map[string]bool) error {
	victims, err := multiUpgradeVictims(destBase, skillSet, skills, managed...)
	if err != nil {
		return err
	}
	specs := make([]skillStageSpec, 0, len(skills))
	for _, name := range skills {
		specs = append(specs, skillStageSpec{
			src:  filepath.Join(multiRoot, name),
			dest: filepath.Join(destBase, name),
		})
	}
	stageRoot, staged, err := stageSkillSet(destBase, ".dws-upgrade-multi-", specs)
	if err != nil {
		return err
	}
	defer func() { _ = upgradeRemoveAll(stageRoot) }()
	return publishStagedSkillSet(homeDir, staged, victims)
}

func hasDetectedSpecificSkillRoot(homeDir string) bool {
	for _, agentDir := range knownSkillDirs {
		if isGenericSkillRoot(agentDir) || isBlacklisted(agentDir) {
			continue
		}
		parentGate := filepath.Dir(filepath.Join(homeDir, agentDir))
		if info, err := upgradeStat(parentGate); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func isGenericSkillRoot(agentDir string) bool {
	return filepath.Clean(agentDir) == filepath.Clean(".agents/skills")
}

func retireGenericSkillRoot(homeDir string, managed map[string]bool) error {
	base := filepath.Join(homeDir, ".agents", "skills")
	victims, err := managedMultiSkillVictims(base, managed)
	if err != nil {
		return err
	}
	victims = append(victims, filepath.Join(base, "dws"))
	if _, err := backupSkillSet(homeDir, victims); err != nil {
		return fmt.Errorf("迁移通用 Skill 根目录失败: %w", err)
	}
	return nil
}

// upgradeMonoSkillLocations is the legacy mono behavior: one dws/ directory
// per agent home.
func upgradeMonoSkillLocations(homeDir, skillSrc string) (*SkillUpgradeResult, error) {
	result := &SkillUpgradeResult{}
	managedNames := readManagedSkillNames(homeDir)
	hasSpecificRoot := hasDetectedSpecificSkillRoot(homeDir)

	for _, agentDir := range knownSkillDirs {
		destDir := filepath.Join(homeDir, agentDir, "dws")

		if isBlacklisted(agentDir) {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirBlacklisted})
			continue
		}
		if isGenericSkillRoot(agentDir) && hasSpecificRoot {
			continue
		}

		if !isGenericSkillRoot(agentDir) {
			parentGate := filepath.Dir(filepath.Join(homeDir, agentDir))
			if _, err := upgradeStat(parentGate); os.IsNotExist(err) {
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirSkipped})
				continue
			}
		}

		if err := publishMonoUpgradeTarget(homeDir, filepath.Join(homeDir, agentDir), skillSrc, managedNames); err != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirFailed, Err: err})
			continue
		}
		result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirOK})
	}
	if hasSpecificRoot && len(result.Succeeded()) > 0 {
		genericBase := filepath.Join(homeDir, ".agents", "skills")
		if err := retireGenericSkillRoot(homeDir, managedNames); err != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: genericBase, Status: SkillDirFailed, Err: err})
		} else {
			result.Results = append(result.Results, SkillDirResult{Dir: genericBase, Status: SkillDirSkipped})
		}
	}

	// Fallback: if nothing succeeded, force the primary location. The multi
	// leftovers under the primary base are the usual reason the primary
	// install failed, so clean them first — failing loud like the multi
	// fallback — instead of letting mono and multi co-exist marked OK.
	if len(result.Succeeded()) == 0 && !hasSpecificRoot {
		destBase := filepath.Join(homeDir, ".agents", "skills")
		dest := filepath.Join(destBase, "dws")
		if err := publishMonoUpgradeTarget(homeDir, destBase, skillSrc, managedNames); err != nil {
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

	// Best-effort: refresh the user-level mono cache so that
	// `dws skill setup --mode mono` fallbacks stay on the upgraded version
	// (symmetric with the multi cache refresh in upgradeMultiSkillLocations).
	_ = refreshSkillCache(homeDir, "mono", skillSrc)

	return result, nil
}

// upgradeMultiSkillLocations installs every skill of the multi bundle into
// each agent home as sibling directories and backs up + removes the
// opposite-mode leftovers. A home is marked failed (and multi is NOT
// installed into it) when leftover backup/removal fails, so mono and multi
// never co-exist.
func upgradeMultiSkillLocations(homeDir, multiRoot string, skills []string) (*SkillUpgradeResult, error) {
	skillSet := make(map[string]bool, len(skills))
	for _, s := range skills {
		skillSet[s] = true
	}

	result := &SkillUpgradeResult{}
	managedNames := readManagedSkillNames(homeDir)
	hasSpecificRoot := hasDetectedSpecificSkillRoot(homeDir)

	for _, agentDir := range knownSkillDirs {
		destBase := filepath.Join(homeDir, agentDir)

		if isBlacklisted(agentDir) {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirBlacklisted})
			continue
		}
		if isGenericSkillRoot(agentDir) && hasSpecificRoot {
			continue
		}

		if !isGenericSkillRoot(agentDir) {
			parentGate := filepath.Dir(destBase)
			if _, err := upgradeStat(parentGate); os.IsNotExist(err) {
				result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirSkipped})
				continue
			}
		}

		if err := publishMultiUpgradeTarget(homeDir, destBase, multiRoot, skills, skillSet, managedNames); err != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirFailed, Err: err})
			continue
		}
		result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirOK})
	}
	if hasSpecificRoot && len(result.Succeeded()) > 0 {
		genericBase := filepath.Join(homeDir, ".agents", "skills")
		if err := retireGenericSkillRoot(homeDir, managedNames); err != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: genericBase, Status: SkillDirFailed, Err: err})
		} else {
			result.Results = append(result.Results, SkillDirResult{Dir: genericBase, Status: SkillDirSkipped})
		}
	}

	// Fallback: if nothing succeeded, force the primary location
	if len(result.Succeeded()) == 0 && !hasSpecificRoot {
		destBase := filepath.Join(homeDir, ".agents", "skills")
		if err := publishMultiUpgradeTarget(homeDir, destBase, multiRoot, skills, skillSet, managedNames); err != nil {
			return result, fmt.Errorf("所有技能目录安装失败，回退到主目录也失败: %w", err)
		}
		// Replace the earlier failed entry for this dir (if any) or append a new one
		replaced := false
		for idx, r := range result.Results {
			if r.Dir == destBase {
				result.Results[idx] = SkillDirResult{Dir: destBase, Status: SkillDirOK}
				replaced = true
				break
			}
		}
		if !replaced {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirOK})
		}
	}

	// Best-effort: refresh the user-level caches so that `dws skill setup`
	// fallbacks stay on the upgraded version. The release zip ships both
	// trees, so when the sibling mono/ tree is present the mono cache is
	// refreshed as well.
	_ = refreshSkillCache(homeDir, "multi", multiRoot)
	if monoSrc := filepath.Join(filepath.Dir(multiRoot), "mono"); skillTreeHasRoot(monoSrc) {
		_ = refreshSkillCache(homeDir, "mono", monoSrc)
	}

	return result, nil
}

// refreshSkillCache mirrors src into ~/.dws/skills/<name>/ through a staged
// sibling directory. The existing cache is moved aside only after the staged
// copy is complete, and is restored if publishing the new cache fails.
func refreshSkillCache(homeDir, name, src string) error {
	cacheDir := filepath.Join(homeDir, ".dws", "skills", name)
	cacheParent := filepath.Dir(cacheDir)
	if err := upgradeMkdirAll(cacheParent, dirPermShared); err != nil {
		return fmt.Errorf("创建 Skill 缓存目录失败 %s: %w", cacheParent, err)
	}

	stagedDir, err := upgradeMkdirTemp(cacheParent, "."+name+".tmp-")
	if err != nil {
		return fmt.Errorf("创建 Skill 缓存临时目录失败 %s: %w", cacheParent, err)
	}
	stagedPublished := false
	defer func() {
		if !stagedPublished {
			_ = upgradeRemoveAll(stagedDir)
		}
	}()
	if err := upgradeCopyDir(src, stagedDir); err != nil {
		return fmt.Errorf("暂存 Skill 缓存失败 %s: %w", stagedDir, err)
	}

	if _, err := upgradeStat(cacheDir); os.IsNotExist(err) {
		if err := upgradeRename(stagedDir, cacheDir); err != nil {
			return fmt.Errorf("发布 Skill 缓存失败 %s: %w", cacheDir, err)
		}
		stagedPublished = true
		return nil
	} else if err != nil {
		return fmt.Errorf("检查 Skill 缓存失败 %s: %w", cacheDir, err)
	}

	rollbackDir, err := upgradeMkdirTemp(cacheParent, "."+name+".old-")
	if err != nil {
		return fmt.Errorf("创建 Skill 缓存回滚目录失败 %s: %w", cacheParent, err)
	}
	if err := upgradeRemoveAll(rollbackDir); err != nil {
		return fmt.Errorf("准备 Skill 缓存回滚目录失败 %s: %w", rollbackDir, err)
	}
	if err := upgradeRename(cacheDir, rollbackDir); err != nil {
		return fmt.Errorf("暂存原 Skill 缓存失败 %s: %w", cacheDir, err)
	}
	if err := upgradeRename(stagedDir, cacheDir); err != nil {
		if restoreErr := upgradeRename(rollbackDir, cacheDir); restoreErr != nil {
			return fmt.Errorf("发布 Skill 缓存失败 %s: %w（恢复原缓存也失败: %v）", cacheDir, err, restoreErr)
		}
		return fmt.Errorf("发布 Skill 缓存失败 %s: %w", cacheDir, err)
	}
	stagedPublished = true
	_ = upgradeRemoveAll(rollbackDir)
	return nil
}

// skillTreeHasRoot reports whether dir carries a top-level SKILL.md.
func skillTreeHasRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil && !info.IsDir()
}

// cleanupMultiLeftovers backs up + removes every proven DWS-managed
// multi-mode skill directory inside one agent home before mono is installed.
// A missing base directory simply means no leftovers; any other read failure
// is reported so mono never silently co-exists with multi. Removal is
// reversible: each leftover is preserved under ~/.dws/skill-backups/ and a
// backup failure aborts the removal for that home.
func cleanupMultiLeftovers(homeDir, baseDir string) error {
	victims, err := managedMultiSkillVictims(baseDir, readManagedSkillNames(homeDir))
	if err != nil {
		return err
	}
	if _, err := backupSkillSet(homeDir, victims); err != nil {
		return fmt.Errorf("备份并清理 multi 残留失败: %w", err)
	}
	return nil
}

func managedMultiSkillVictims(baseDir string, managed ...map[string]bool) ([]string, error) {
	entries, err := upgradeReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取技能目录失败 %s: %w", baseDir, err)
	}
	victims := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() || !isManagedMultiSkillDir(filepath.Join(baseDir, e.Name()), managed...) {
			continue
		}
		victims = append(victims, filepath.Join(baseDir, e.Name()))
	}
	return victims, nil
}

// cleanupOppositeModeLeftovers backs up + removes, inside one agent home, the
// legacy mono directory (dws/) and every proven DWS-managed multi skill
// directory that is not part of the new bundle. A dingtalk-* prefix alone is
// not proof of ownership because market/user skills may use the same prefix.
// Removal is reversible: each
// directory is preserved under ~/.dws/skill-backups/ and a backup failure
// aborts the removal for that home.
func cleanupOppositeModeLeftovers(homeDir, destBase string, skillSet map[string]bool) error {
	victims, err := oppositeModeSkillVictims(destBase, skillSet, readManagedSkillNames(homeDir))
	if err != nil {
		return err
	}
	if _, err := backupSkillSet(homeDir, victims); err != nil {
		return fmt.Errorf("备份并清理对面模式残留失败: %w", err)
	}
	return nil
}

func oppositeModeSkillVictims(destBase string, skillSet map[string]bool, managed ...map[string]bool) ([]string, error) {
	victims := []string{filepath.Join(destBase, "dws")}
	entries, err := upgradeReadDir(destBase)
	if err != nil {
		if os.IsNotExist(err) {
			return victims, nil
		}
		return nil, fmt.Errorf("读取技能目录失败 %s: %w", destBase, err)
	}
	for _, e := range entries {
		if !e.IsDir() || skillSet[e.Name()] || !isManagedMultiSkillDir(filepath.Join(destBase, e.Name()), managed...) {
			continue
		}
		victims = append(victims, filepath.Join(destBase, e.Name()))
	}
	return victims, nil
}

// isManagedMultiSkillDir accepts only centralized metadata or the frozen exact
// official-name migration list. Files inside Skill directories are ignored.
func isManagedMultiSkillDir(dir string, managed ...map[string]bool) bool {
	if skillstate.IsLegacyOfficialSkillName(filepath.Base(dir)) {
		return true
	}
	return len(managed) > 0 && managed[0][filepath.Base(dir)]
}

func readManagedSkillNames(homeDir string) map[string]bool {
	state, readable, err := upgradeReadSkillState(homeDir)
	if err != nil || !readable {
		return map[string]bool{}
	}
	return skillstate.ManagedSkillNames(state)
}

// bundleSkillNames returns the sorted names of subdirectories of dir that
// contain a SKILL.md. It returns nil when dir itself carries a top-level
// SKILL.md (mono layout) so callers can distinguish the two layouts.
func bundleSkillNames(dir string) []string {
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// LocateSkillsRoot resolves the skill root inside an extracted dws-skills.zip,
// preferring the multi bundle ({extractDir}/multi) over the legacy mono
// layouts handled by LocateSkillMD.
func LocateSkillsRoot(extractDir string) string {
	multiRoot := filepath.Join(extractDir, "multi")
	if skills := bundleSkillNames(multiRoot); len(skills) > 0 {
		return multiRoot
	}
	return LocateSkillMD(extractDir)
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
	homeDir, err := upgradeUserHomeDir()
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
		if err := upgradeEnsureDir(d.path, d.perm); err != nil {
			return err
		}
	}
	return nil
}

// DownloadCacheDir returns the path for temporary downloads during upgrade.
func DownloadCacheDir() string {
	homeDir, _ := upgradeUserHomeDir()
	return filepath.Join(homeDir, ".dws", "cache", "downloads")
}

// CurrentBinaryPath returns the resolved path of the currently running binary.
func CurrentBinaryPath() (string, error) {
	exe, err := upgradeExecutable()
	if err != nil {
		return "", err
	}
	return upgradeEvalSymlinks(exe)
}

// BinaryName returns the platform-specific binary name.
func BinaryName() string {
	return binaryNameFor(runtime.GOOS)
}

func binaryNameFor(goos string) string {
	if goos == "windows" {
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
