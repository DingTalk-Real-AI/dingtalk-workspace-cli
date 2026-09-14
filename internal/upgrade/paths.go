// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
// The first entry (.agents/skills) is the canonical global Skill directory,
// following the universal .agents/skills convention.
// Agents that understand the universal .agents convention read it directly;
// detected non-universal Agents receive relative links to this canonical
// copy (with a direct-copy fallback when links are unavailable).
var knownSkillDirs = []string{
	".agents/skills",
	".config/agents/skills",
	".gemini/antigravity/skills",
	".gemini/antigravity-cli/skills",
	".deepagents/agent/skills",
	".firebender/skills",
	".copilot/skills",
	".config/opencode/skills",
	".aider-desk/skills",
	".astrbot/data/skills",
	".autohand/skills",
	".augment/skills",
	".bob/skills",
	".claude/skills",
	".openclaw/skills",
	".codeartsdoer/skills",
	".codebuddy/skills",
	".codemaker/skills",
	".codestudio/skills",
	".commandcode/skills",
	".continue/skills",
	".snowflake/cortex/skills",
	".config/crush/skills",
	".config/devin/skills",
	".factory/skills",
	".forge/skills",
	".config/goose/skills",
	".grok/skills",
	".hermes/skills",
	".inferencesh/skills",
	".jazz/skills",
	".junie/skills",
	".iflow/skills",
	".kilocode/skills",
	".config/kimchi/harness/skills",
	".kiro/skills",
	".kode/skills",
	".lingma/skills",
	".mcpjam/skills",
	".minimax/skills",
	".vibe/skills",
	".moxby/skills",
	".mux/skills",
	".openhands/skills",
	".ona/skills",
	".pi/agent/skills",
	".qoder/skills",
	".qoder-cn/skills",
	".qwen/skills",
	".reasonix/skills",
	".rovodev/skills",
	".roo/skills",
	".tabnine/agent/skills",
	".terramind/skills",
	".tinycloud/skills",
	".trae/skills",
	".trae-cn/skills",
	".codeium/windsurf/skills",
	".zcode/skills",
	".zencoder/skills",
	".neovate/skills",
	".pochi/skills",
	".adal/skills",
	// DWS-only target retained for product compatibility.
	".qoderwork/skills",
	// beta.6 compatibility roots: cleanup only, never publish new copies.
	".cursor/skills",
	".gemini/skills",
	".codex/skills",
	".github/skills",
	".windsurf/skills",
	".cline/skills",
	".amp/skills",
}

// universalSkillDirs are concrete Agent directories whose current clients
// natively scan ~/.agents/skills. Publishing another copy or link below these
// roots makes the same Skill appear twice. Keep this classification aligned
// with vercel-labs/skills' isUniversalAgent rule. Qoderwork is intentionally
// non-universal because it is a DWS-specific integration absent upstream.
var universalSkillDirs = map[string]bool{
	".config/agents/skills":          true,
	".gemini/antigravity/skills":     true,
	".gemini/antigravity-cli/skills": true,
	".codex/skills":                  true,
	".cursor/skills":                 true,
	".deepagents/agent/skills":       true,
	".firebender/skills":             true,
	".gemini/skills":                 true,
	".copilot/skills":                true,
	".config/opencode/skills":        true,
	// beta.6 compatibility roots are cleanup-only as well.
	".github/skills":   true,
	".windsurf/skills": true,
	".cline/skills":    true,
	".amp/skills":      true,
}

var (
	upgradeUserHomeDir                 = os.UserHomeDir
	upgradeExecutable                  = os.Executable
	upgradeEvalSymlinks                = filepath.EvalSymlinks
	upgradeCopyDir                     = copyDir
	upgradeEnsureDir                   = ensureDir
	upgradeRemoveAll                   = os.RemoveAll
	upgradeMkdirAll                    = os.MkdirAll
	upgradeMkdirTemp                   = os.MkdirTemp
	upgradeReadDir                     = os.ReadDir
	upgradeStat                        = os.Stat
	upgradeLstat                       = os.Lstat
	upgradeSymlink                     = os.Symlink
	upgradeRel                         = filepath.Rel
	upgradeGetenv                      = os.Getenv
	upgradeBuildProvenance             = skillprovenance.Build
	upgradeReadSkillState              = skillstate.Read
	upgradePublishSkillPath            = PublishSkillPathNoReplace
	upgradeRollbackPublishedSkillPaths = RollbackSkillPathPublications
	upgradeBackupStamp                 = func() string { return time.Now().UTC().Format("20060102-150405") }
	upgradeWriteSkillState             = skillstate.Write
	upgradeNow                         = time.Now
	upgradeFoldPathCase                = runtime.GOOS == "windows"
)

// skillBackupSubdir is the user-level directory where skill directories are
// preserved before a layout-changing install/upgrade removes them. Non-
// interactive flows (install scripts, npm postinstall, `dws upgrade`) cannot
// ask for confirmation, so deletions must stay reversible instead.
const skillBackupSubdir = ".dws/skill-backups"

// backupAndRemoveSkillDir moves dir into <homeDir>/.dws/skill-backups/
// <stamp>/<rel> instead of destroying it, and returns the backup path. It is
// fail-safe: a path that cannot be backed up is NOT removed and the
// error is returned so the caller can surface it (and never install the
// opposite layout next to it silently). Missing paths are no-ops. Directories,
// links (including dangling links), and ordinary files are all preserved:
// an unexpected file at a managed destination must never be overwritten.
func backupAndRemoveSkillDir(homeDir, dir string) (string, error) {
	_, err := upgradeLstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("检查技能目录失败 %s: %w", dir, err)
	}
	rel, relErr := filepath.Rel(homeDir, dir)
	if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(dir)
	}
	name := strings.NewReplacer(string(filepath.Separator), "-", "/", "-").Replace(rel)
	stamp := upgradeBackupStamp()
	if err := upgradeMkdirAll(filepath.Join(homeDir, skillBackupSubdir), dirPermShared); err != nil {
		return "", fmt.Errorf("创建备份根目录失败 %s: %w", filepath.Join(homeDir, skillBackupSubdir), err)
	}
	var backupRoot, target string
	for i := 0; ; i++ {
		candidate := stamp
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", stamp, i)
		}
		if i > 1000 {
			return "", fmt.Errorf("备份目录冲突无法解决: %s", filepath.Join(homeDir, skillBackupSubdir, candidate))
		}
		backupRoot = filepath.Join(homeDir, skillBackupSubdir, candidate)
		target = filepath.Join(backupRoot, name)
		if _, err := upgradeLstat(target); err == nil {
			continue // payload name already taken in this stamp root
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("检查备份目标失败 %s: %w", target, err)
		}
		if err := skillPathMkdir(backupRoot, dirPermShared); err == nil {
			// We created the root: stamp ownership before any payload moves
			// in, so an interrupted backup never leaves an unmarked
			// (never-prunable) stamp behind.
			if err := skillBackupWriteMarker(backupRoot); err != nil {
				// Bounded cleanup: the root is empty and ours alone.
				_ = skillPathRemove(backupRoot)
				return "", fmt.Errorf("写入备份所有权标记失败 %s: %w", backupRoot, err)
			}
			break
		} else if !os.IsExist(err) {
			return "", fmt.Errorf("创建备份目录失败 %s: %w", backupRoot, err)
		} else if skillBackupMarkerValid(backupRoot) {
			break // proven ours: same-stamp payloads from this run reuse it
		}
		// A stamp-shaped directory we cannot prove ours must never be
		// adopted — writing the marker or the payload into it would turn
		// its foreign contents into prunable DWS backups.
	}
	if err := moveSkillPathRecoverably(dir, target); err != nil {
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
	// SkillDirRetireWarning marks a universal Agent whose obsolete private copy
	// could not be retired. Nothing is installed below such a root, so the
	// leftover is reported without counting as an install failure.
	SkillDirRetireWarning
)

// SkillDirResult holds the per-directory install result.
type SkillDirResult struct {
	Dir    string         // destination directory (e.g. ~/.claude/skills/dws)
	Status SkillDirStatus // outcome
	Err    error          // non-nil when Status == SkillDirFailed or SkillDirRetireWarning
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

// RetireWarnings returns universal Agent roots that still hold an obsolete
// private copy because retiring it failed.
func (r *SkillUpgradeResult) RetireWarnings() []SkillDirResult {
	var out []SkillDirResult
	for _, d := range r.Results {
		if d.Status == SkillDirRetireWarning {
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
//   - ~/.agents/skills/ is always the canonical global store
//   - universal Agents (Codex, Cursor, Gemini, Cline, Amp, Copilot) read the
//     canonical store directly, so old agent-specific copies are retired
//   - detected non-universal Agents receive relative links to canonical;
//     filesystems that reject links receive a direct-copy fallback
//   - ~/.real/ and other blacklisted paths are NEVER touched
//   - canonical publication is mandatory and fails the upgrade loudly
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

// The ownership marker inside a stamp root. Every DWS surface writes the
// exact same bytes (install.sh, install-skills.sh, install-event.sh,
// install-devapp.sh, the PowerShell installers, and build/npm/install.js):
// a stamp-shaped directory name alone is not ownership proof, because users
// and unrelated tools can create 20260819-120000-shaped names. Counting or
// deleting a backup root requires the marker to verify, and adopting an
// existing root for a new backup requires it first.
const (
	skillBackupMarkerName    = ".dws-skill-backup"
	skillBackupMarkerContent = "dws skill backup v1\n"
)

var (
	skillBackupReadMarker = func(root string) (string, error) {
		data, err := os.ReadFile(filepath.Join(root, skillBackupMarkerName))
		return string(data), err
	}
	skillBackupWriteMarker = func(root string) error {
		return os.WriteFile(filepath.Join(root, skillBackupMarkerName), []byte(skillBackupMarkerContent), 0o644)
	}
)

// skillBackupMarkerValid reports whether root carries the ownership marker
// with exactly the expected bytes; a missing, unreadable, or differently
// worded marker means the directory is foreign data.
func skillBackupMarkerValid(root string) bool {
	body, err := skillBackupReadMarker(root)
	return err == nil && body == skillBackupMarkerContent
}

// skillBackupStampRegexp matches only directory names DWS itself writes:
// UTC YYYYmmdd-HHMMSS, with an optional -N collision suffix. Any other entry
// in the backup root is foreign (user data, unrelated tooling) and must be
// left untouched, so pruning is restricted to names it can prove DWS created.
var skillBackupStampRegexp = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}(-[0-9]+)?$`)

// isSkillBackupStamp reports whether name is a DWS-created backup stamp.
func isSkillBackupStamp(name string) bool {
	return skillBackupStampRegexp.MatchString(name)
}

// pruneSkillBackups removes the oldest backup directories when more than
// skillBackupKeep remain. Only directories whose names match the DWS backup
// stamp format AND whose ownership marker verifies are candidates; unknown
// or unmarked directories — including stamp roots created before the
// ownership marker existed — are foreign data and preserved. Best-effort:
// a removal failure never aborts, but pruning failures are reported so
// callers can warn the user.
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
		if !e.IsDir() || !isSkillBackupStamp(e.Name()) {
			continue
		}
		// A stamp-shaped name alone is not ownership proof: only roots
		// whose marker verifies are counted or deleted.
		if skillBackupMarkerValid(filepath.Join(root, e.Name())) {
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
func restoreSkillSet(published []SkillPathPublication, backups []backedUpSkillDir) error {
	restoreErr := upgradeRollbackPublishedSkillPaths(published)
	for i := len(backups) - 1; i >= 0; i-- {
		backup := backups[i]
		if err := upgradeMkdirAll(filepath.Dir(backup.original), dirPermShared); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("创建 Skill 恢复目录 %s: %w", filepath.Dir(backup.original), err))
			continue
		}
		if err := moveSkillPathRecoverably(backup.backup, backup.original); err != nil {
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
	published := make([]SkillPathPublication, 0, len(staged))
	for _, skill := range staged {
		publication, publishErr := upgradePublishSkillPath(skill.staged, skill.dest)
		if publishErr != nil {
			publishErr := fmt.Errorf("发布 Skill 失败 %s: %w", skill.dest, publishErr)
			if restoreErr := restoreSkillSet(published, backups); restoreErr != nil {
				return errors.Join(publishErr, fmt.Errorf("恢复原 Skill 集合失败: %w", restoreErr))
			}
			return publishErr
		}
		published = append(published, publication)
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

func isGenericSkillRoot(agentDir string) bool {
	return filepath.Clean(agentDir) == filepath.Clean(".agents/skills")
}

func isUniversalSkillRoot(agentDir string) bool {
	return universalSkillDirs[filepath.ToSlash(filepath.Clean(agentDir))]
}

type resolvedSkillRoot struct {
	base      string
	label     string
	universal bool
	canonical bool
}

func resolveOpenClawSkillBase(homeDir string) string {
	for _, name := range []string{".openclaw", ".clawdbot", ".moltbot"} {
		base := filepath.Join(homeDir, name)
		if info, err := upgradeStat(base); err == nil && info.IsDir() {
			return filepath.Join(base, "skills")
		}
	}
	return filepath.Join(homeDir, ".openclaw", "skills")
}

func configuredSkillRoots(homeDir string) []resolvedSkillRoot {
	var roots []resolvedSkillRoot
	seen := map[string]bool{}
	configHome := strings.TrimSpace(upgradeGetenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}
	add := func(base, label string, universal, canonical bool) {
		key := skillRootPathKey(base)
		if seen[key] {
			return
		}
		seen[key] = true
		roots = append(roots, resolvedSkillRoot{base: base, label: label, universal: universal, canonical: canonical})
	}
	for _, agentDir := range knownSkillDirs {
		base := filepath.Join(homeDir, agentDir)
		switch filepath.ToSlash(filepath.Clean(agentDir)) {
		case ".claude/skills":
			if custom := strings.TrimSpace(upgradeGetenv("CLAUDE_CONFIG_DIR")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".codex/skills":
			if custom := strings.TrimSpace(upgradeGetenv("CODEX_HOME")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".hermes/skills":
			if custom := strings.TrimSpace(upgradeGetenv("HERMES_HOME")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".autohand/skills":
			if custom := strings.TrimSpace(upgradeGetenv("AUTOHAND_HOME")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".grok/skills":
			if custom := strings.TrimSpace(upgradeGetenv("GROK_HOME")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".vibe/skills":
			if custom := strings.TrimSpace(upgradeGetenv("VIBE_HOME")); custom != "" {
				base = filepath.Join(custom, "skills")
			}
		case ".openclaw/skills":
			base = resolveOpenClawSkillBase(homeDir)
		case ".config/agents/skills":
			base = filepath.Join(configHome, "agents", "skills")
		case ".config/opencode/skills":
			base = filepath.Join(configHome, "opencode", "skills")
		case ".config/crush/skills":
			base = filepath.Join(configHome, "crush", "skills")
		case ".config/devin/skills":
			base = filepath.Join(configHome, "devin", "skills")
		case ".config/goose/skills":
			base = filepath.Join(configHome, "goose", "skills")
		case ".config/kimchi/harness/skills":
			base = filepath.Join(configHome, "kimchi", "harness", "skills")
		}
		add(base, agentDir, isUniversalSkillRoot(agentDir), isGenericSkillRoot(agentDir))
	}
	return roots
}

func skillRootDetectedBase(root resolvedSkillRoot) bool {
	detectedDir := filepath.Dir(root.base)
	switch filepath.ToSlash(filepath.Clean(root.label)) {
	case ".config/kimchi/harness/skills":
		detectedDir = filepath.Dir(filepath.Dir(root.base))
	case ".tabnine/agent/skills":
		detectedDir = filepath.Dir(filepath.Dir(root.base))
	case ".zcode/skills":
		// Application bundles are machine-scoped detection signals. Keep this
		// independent of HOME so upgrade matches npm, Shell, and PowerShell.
		if info, err := upgradeStat(filepath.Join(string(filepath.Separator), "Applications", "ZCode.app")); err == nil && info.IsDir() {
			return true
		}
	case ".minimax/skills":
		if info, err := upgradeStat(filepath.Join(string(filepath.Separator), "Applications", "MiniMax Code.app")); err == nil && info.IsDir() {
			return true
		}
	}
	info, err := upgradeStat(detectedDir)
	return err == nil && info.IsDir()
}

func samePhysicalSkillRoot(left, right string) bool {
	leftReal, leftErr := upgradeEvalSymlinks(left)
	rightReal, rightErr := upgradeEvalSymlinks(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return skillRootPathKey(leftReal) == skillRootPathKey(rightReal)
}

func skillRootPathKey(path string) string {
	clean := filepath.Clean(path)
	if upgradeFoldPathCase {
		clean = strings.ToLower(clean)
	}
	return clean
}

func retireManagedSkillRoot(homeDir, base string, managed map[string]bool) error {
	victims, err := managedMultiSkillVictims(base, managed)
	if err != nil {
		return err
	}
	victims = append(victims, filepath.Join(base, "dws"))
	if _, err := backupSkillSet(homeDir, victims); err != nil {
		return fmt.Errorf("迁移 Agent Skill 旧副本失败: %w", err)
	}
	return nil
}

func stageLinkedSkillSet(destBase, canonicalBase, prefix string, names []string) (stageRoot string, staged []stagedSkillDir, err error) {
	if err := upgradeMkdirAll(destBase, dirPermShared); err != nil {
		return "", nil, fmt.Errorf("创建 Agent Skill 目录失败 %s: %w", destBase, err)
	}
	stageRoot, err = upgradeMkdirTemp(destBase, prefix)
	if err != nil {
		return "", nil, fmt.Errorf("创建 Skill 链接 staging 失败 %s: %w", destBase, err)
	}
	defer func() {
		if err != nil {
			_ = upgradeRemoveAll(stageRoot)
		}
	}()
	realDestBase, realErr := upgradeEvalSymlinks(destBase)
	if realErr != nil {
		return stageRoot, nil, fmt.Errorf("解析 Agent Skill 物理目录失败 %s: %w", destBase, realErr)
	}
	for _, name := range names {
		target := filepath.Join(canonicalBase, name)
		realTarget, targetErr := upgradeEvalSymlinks(target)
		if targetErr != nil {
			return stageRoot, nil, fmt.Errorf("解析 canonical Skill 失败 %s: %w", target, targetErr)
		}
		relTarget, relErr := upgradeRel(realDestBase, realTarget)
		if relErr != nil {
			return stageRoot, nil, fmt.Errorf("计算 Skill 相对链接失败 %s: %w", target, relErr)
		}
		stagedPath := filepath.Join(stageRoot, name)
		if linkErr := upgradeSymlink(relTarget, stagedPath); linkErr != nil {
			return stageRoot, nil, fmt.Errorf("创建 Skill 链接失败 %s -> %s: %w", stagedPath, relTarget, linkErr)
		}
		staged = append(staged, stagedSkillDir{staged: stagedPath, dest: filepath.Join(destBase, name)})
	}
	return stageRoot, staged, nil
}

func publishLinkedUpgradeTarget(homeDir, destBase, canonicalBase string, names []string, victims []string) error {
	needed := make([]string, 0, len(names))
	correct := make(map[string]bool, len(names))
	for _, name := range names {
		dest := filepath.Join(destBase, name)
		canonical := filepath.Join(canonicalBase, name)
		if samePhysicalSkillRoot(dest, canonical) {
			correct[filepath.Clean(dest)] = true
			continue
		}
		needed = append(needed, name)
	}
	filteredVictims := make([]string, 0, len(victims))
	for _, victim := range victims {
		if !correct[filepath.Clean(victim)] {
			filteredVictims = append(filteredVictims, victim)
		}
	}
	stageRoot, staged, err := stageLinkedSkillSet(destBase, canonicalBase, ".dws-link-set-", needed)
	if err != nil {
		return err
	}
	defer func() { _ = upgradeRemoveAll(stageRoot) }()
	return publishStagedSkillSet(homeDir, staged, filteredVictims)
}

// upgradeMonoSkillLocations is the legacy mono behavior: one dws/ directory
// per agent home.
func upgradeMonoSkillLocations(homeDir, skillSrc string) (*SkillUpgradeResult, error) {
	result := &SkillUpgradeResult{}
	managedNames := readManagedSkillNames(homeDir)
	canonicalBase := filepath.Join(homeDir, ".agents", "skills")
	canonicalDest := filepath.Join(canonicalBase, "dws")
	if err := publishMonoUpgradeTarget(homeDir, canonicalBase, skillSrc, managedNames); err != nil {
		result.Results = append(result.Results, SkillDirResult{Dir: canonicalDest, Status: SkillDirFailed, Err: err})
		// Canonical is the mandatory store: universal agents read it directly, so a
		// failed canonical publish means nobody received the upgrade. Always fail
		// loud — including universal-only machines — instead of reporting success
		// with nothing installed.
		return result, fmt.Errorf("canonical Skill 安装失败: %w", err)
	}
	result.Results = append(result.Results, SkillDirResult{Dir: canonicalDest, Status: SkillDirOK})

	for _, root := range configuredSkillRoots(homeDir) {
		if root.canonical {
			continue
		}
		destBase := root.base
		destDir := filepath.Join(destBase, "dws")
		if isBlacklisted(root.label) {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirBlacklisted})
			continue
		}
		if !skillRootDetectedBase(root) {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirSkipped})
			continue
		}
		if samePhysicalSkillRoot(destBase, canonicalBase) {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirSkipped})
			continue
		}
		if root.universal {
			if err := retireManagedSkillRoot(homeDir, destBase, managedNames); err != nil {
				// Nothing is installed below a universal root, so a stale copy that
				// resists retirement is a warning, not an install failure.
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirRetireWarning, Err: err})
			} else {
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirSkipped})
			}
			continue
		}
		victims, victimErr := monoUpgradeVictims(destBase, destDir, managedNames)
		if victimErr != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirFailed, Err: victimErr})
			continue
		}
		if err := publishLinkedUpgradeTarget(homeDir, destBase, canonicalBase, []string{"dws"}, victims); err != nil {
			// An uncertain destination may belong to a concurrent writer.
			// The copy fallback below would back that object aside and
			// replace it — exactly what the sentinel forbids — so surface
			// the uncertain state instead of retrying over it.
			if errors.Is(err, ErrSkillPathPublicationUncertain) {
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirFailed, Err: err})
				continue
			}
			// Match the upstream installer: platforms/filesystems that reject
			// links receive a direct copy rather than losing Agent support.
			if copyErr := publishMonoUpgradeTarget(homeDir, destBase, skillSrc, managedNames); copyErr != nil {
				result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirFailed, Err: errors.Join(err, copyErr)})
				continue
			}
		}
		result.Results = append(result.Results, SkillDirResult{Dir: destDir, Status: SkillDirOK})
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
	canonicalBase := filepath.Join(homeDir, ".agents", "skills")
	if err := publishMultiUpgradeTarget(homeDir, canonicalBase, multiRoot, skills, skillSet, managedNames); err != nil {
		result.Results = append(result.Results, SkillDirResult{Dir: canonicalBase, Status: SkillDirFailed, Err: err})
		// Canonical is the mandatory store (see the mono branch): fail loud even
		// on universal-only machines instead of reporting success with nothing
		// installed.
		return result, fmt.Errorf("canonical Skill 安装失败: %w", err)
	}
	result.Results = append(result.Results, SkillDirResult{Dir: canonicalBase, Status: SkillDirOK})

	for _, root := range configuredSkillRoots(homeDir) {
		if root.canonical {
			continue
		}
		destBase := root.base
		if isBlacklisted(root.label) {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirBlacklisted})
			continue
		}
		if !skillRootDetectedBase(root) {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirSkipped})
			continue
		}
		if samePhysicalSkillRoot(destBase, canonicalBase) {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirSkipped})
			continue
		}
		if root.universal {
			if err := retireManagedSkillRoot(homeDir, destBase, managedNames); err != nil {
				// Nothing is installed below a universal root, so a stale copy that
				// resists retirement is a warning, not an install failure.
				result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirRetireWarning, Err: err})
			} else {
				result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirSkipped})
			}
			continue
		}
		victims, victimErr := multiUpgradeVictims(destBase, skillSet, skills, managedNames)
		if victimErr != nil {
			result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirFailed, Err: victimErr})
			continue
		}
		if err := publishLinkedUpgradeTarget(homeDir, destBase, canonicalBase, skills, victims); err != nil {
			// An uncertain destination may belong to a concurrent writer;
			// the copy fallback must not displace it (see the mono branch).
			if errors.Is(err, ErrSkillPathPublicationUncertain) {
				result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirFailed, Err: err})
				continue
			}
			if copyErr := publishMultiUpgradeTarget(homeDir, destBase, multiRoot, skills, skillSet, managedNames); copyErr != nil {
				result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirFailed, Err: errors.Join(err, copyErr)})
				continue
			}
		}
		result.Results = append(result.Results, SkillDirResult{Dir: destBase, Status: SkillDirOK})
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
		if (!e.IsDir() && e.Type()&os.ModeSymlink == 0) || !isManagedMultiSkillDir(filepath.Join(baseDir, e.Name()), managed...) {
			continue
		}
		victims = append(victims, filepath.Join(baseDir, e.Name()))
	}
	return victims, nil
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
		if (!e.IsDir() && e.Type()&os.ModeSymlink == 0) || skillSet[e.Name()] || !isManagedMultiSkillDir(filepath.Join(destBase, e.Name()), managed...) {
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
