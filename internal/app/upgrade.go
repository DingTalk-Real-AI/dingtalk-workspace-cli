// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var (
	ugBold    = tui.Bold
	ugGreen   = tui.Success
	ugYellow  = tui.Warning
	ugRed     = tui.Danger
	ugCyan    = tui.Cyan
	ugDim     = tui.Dim
	ugBoldGrn = tui.Success
)

type upgradeReleaseClient interface {
	FetchLatestReleaseForTrack(upgrade.ReleaseTrack) (*upgrade.ReleaseInfo, error)
	FetchReleaseByTag(string) (*upgrade.ReleaseInfo, error)
	FetchReleaseVersions(upgrade.ReleaseTrack) ([]upgrade.VersionEntry, error)
}

type upgradeRollbackManager interface {
	ListBackups() ([]upgrade.BackupInfo, error)
	RollbackTo(upgrade.BackupInfo) error
	Backup(string) (string, error)
	Cleanup(int) error
}

var (
	newUpgradeReleaseClient = func() upgradeReleaseClient { return upgrade.NewClient() }
	newUpgradeRollback      = func() upgradeRollbackManager { return upgrade.NewRollbackManager() }
	ensureUpgradeDirs       = upgrade.EnsureUpgradeDirectories
	cleanupUpgradeStale     = upgrade.CleanupStaleFiles
	upgradeNeedsUpgrade     = upgrade.NeedsUpgrade
	findUpgradeBinary       = upgrade.FindBinaryAsset
	findUpgradeSkills       = upgrade.FindSkillsAsset
	findUpgradeChecksums    = upgrade.FindChecksumsAsset
	downloadUpgradeFile     = upgrade.Download
	downloadUpgradeProgress = upgrade.DownloadWithProgress
	extractUpgradeZip       = upgrade.ExtractZip
	findExtractedBinary     = upgrade.FindBinaryInDir
	locateUpgradeSkill      = upgrade.LocateSkillMD
	replaceUpgradeSelf      = upgrade.ReplaceSelf
	installUpgradeSkills    = upgrade.UpgradeSkillLocations
	upgradeMkdirTemp        = os.MkdirTemp
	upgradeRemoveAll        = os.RemoveAll
	upgradeReadFile         = os.ReadFile
	upgradeMkdirAll         = os.MkdirAll
	verifyUpgradeFile       = strictVerifyFile
	extractUpgradeTarGz     = extractTarGz
	validateUpgradeBinary   = validateNewBinary
	upgradeStat             = os.Stat
	upgradeChmod            = os.Chmod
	upgradeTryExecVersion   = tryExecVersion
	upgradeRepairDarwin     = repairDarwinBinary
	upgradeRuntimeGOOS      = runtime.GOOS
	upgradeLookPath         = exec.LookPath
	upgradeCommandOutput    = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
	upgradeUserHomeDir = os.UserHomeDir
)

const defaultListLimit = 10

func newUpgradeCommand() *cobra.Command {
	var (
		flagCheck      bool
		flagList       bool
		flagVersion    string
		flagRollback   bool
		flagForce      bool
		flagSkipSkills bool
		flagAll        bool
		flagBeta       bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "升级 DWS CLI 到最新版本",
		Long: `检查并升级 DWS CLI 到最新版本。

自动下载匹配当前平台的二进制文件和技能包，通过 SHA256 校验后原子替换。
升级前会自动备份当前版本，可通过 --rollback 回滚。`,
		Example: `  dws upgrade                    # 交互式升级到最新版本
  dws upgrade --check            # 仅检查是否有新版本
  dws upgrade --list             # 列出最近版本
  dws upgrade --list --all       # 列出所选轨道的全部版本
  dws upgrade --beta             # 升级到最新 beta 预发布版本
  dws upgrade --version v1.0.7   # 升级到指定正式版本
  dws upgrade --version v1.0.8-beta.1  # 升级到指定 beta 版本
  dws upgrade --rollback         # 回滚到上一版本
  dws upgrade --dry-run          # 仅预览升级步骤，不实际执行
  dws upgrade -y                 # 跳过确认直接升级`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if h := edition.Get(); h != nil && h.IsEmbedded {
				name := h.Name
				if name == "" {
					name = "embedded"
				}
				return fmt.Errorf("当前运行在嵌入模式（%s），dws upgrade 已禁用；请通过宿主完成升级", name)
			}

			yes, _ := cmd.Flags().GetBool("yes")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			format := resolveUpgradeFormat(cmd)
			track := upgradeTrack(flagBeta)
			if flagBeta && flagVersion != "" {
				return fmt.Errorf("--beta 与 --version 不能同时使用；安装指定 beta 版本请直接使用 --version vX.Y.Z-beta.N")
			}

			if flagList {
				limit := defaultListLimit
				if flagAll {
					limit = 0
				}
				return runUpgradeList(cmd, format, limit, track)
			}
			if flagRollback {
				return runUpgradeRollback(yes)
			}
			if flagCheck {
				return runUpgradeCheck(cmd, format, track)
			}
			return runUpgrade(cmd.Context(), upgradeOptions{
				targetVersion: flagVersion,
				force:         flagForce,
				skipSkills:    flagSkipSkills,
				yes:           yes,
				dryRun:        dryRun,
				track:         track,
			})
		},
	}

	cmd.Flags().BoolVar(&flagCheck, "check", false, "仅检查是否有新版本")
	cmd.Flags().BoolVar(&flagList, "list", false, "列出正式 release 版本（配合 --beta 查看 beta）")
	cmd.Flags().BoolVar(&flagAll, "all", false, "与 --list 搭配，显示所选轨道的全部版本")
	cmd.Flags().StringVar(&flagVersion, "version", "", "升级到指定版本")
	cmd.Flags().BoolVar(&flagBeta, "beta", false, "使用最新 beta 预发布版本（默认使用正式 release）")
	cmd.Flags().BoolVar(&flagRollback, "rollback", false, "回滚到上一版本")
	cmd.Flags().BoolVar(&flagForce, "force", false, "强制重新安装当前版本")
	cmd.Flags().BoolVar(&flagSkipSkills, "skip-skills", false, "跳过技能包更新")

	return cmd
}

type upgradeOptions struct {
	targetVersion string
	force         bool
	skipSkills    bool
	yes           bool
	dryRun        bool
	track         upgrade.ReleaseTrack
}

// --- dws upgrade --check ---

func runUpgradeCheck(cmd *cobra.Command, format string, track upgrade.ReleaseTrack) error {
	client := newUpgradeReleaseClient()

	if format != "json" {
		fmt.Printf("  %s\n", ugDim(fmt.Sprintf("检查更新%s...", upgradeTrackSuffix(track))))
	}

	latest, err := client.FetchLatestReleaseForTrack(track)
	if err != nil {
		return fmt.Errorf("检查更新失败: %w", err)
	}

	currentVer := version
	needsUpgrade := upgradeNeedsUpgrade(currentVer, latest.Version)

	if format == "json" {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"current_version": ensureV(currentVer),
			"latest_version":  "v" + latest.Version,
			"needs_upgrade":   needsUpgrade,
			"track":           string(track),
			"release_date":    latest.Date,
			"prerelease":      latest.Prerelease,
			"changelog":       parseChangelogEntries(latest.Changelog, 10),
			"release_url":     latest.HTMLURL,
		})
	}

	if !needsUpgrade {
		fmt.Printf("\n  %s 已是最新版本 %s\n", ugBoldGrn("✔"), ugBold(ensureV(currentVer)))
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s  %s %s %s\n", ugBold("新版本可用:"), ugDim(ensureV(currentVer)), ugBold("→"), ugBoldGrn("v"+latest.Version))
	if latest.Date != "" {
		fmt.Printf("  %s  %s\n", ugBold("发布日期:  "), latest.Date)
	}
	if latest.Prerelease {
		fmt.Printf("  %s  %s\n", ugBold("轨道:      "), ugYellow("beta / pre-release"))
	}
	if entries := parseChangelogEntries(latest.Changelog, 5); len(entries) > 0 {
		fmt.Printf("  %s\n", ugBold("更新内容:"))
		for _, e := range entries {
			fmt.Printf("    %s %s\n", ugGreen("•"), e)
		}
	}
	fmt.Println()
	fmt.Printf("  %s\n", ugDim(upgradeHintForTrack(track)))
	return nil
}

// --- dws upgrade --list ---

// runUpgradeList displays available versions. When limit > 0, only the most
// recent `limit` versions are shown; pass 0 to show all (--all flag).
func runUpgradeList(cmd *cobra.Command, format string, limit int, track upgrade.ReleaseTrack) error {
	client := newUpgradeReleaseClient()

	if format != "json" {
		fmt.Printf("  %s\n", ugDim(fmt.Sprintf("获取版本列表%s...", upgradeTrackSuffix(track))))
	}

	versions, err := client.FetchReleaseVersions(track)
	if err != nil {
		return fmt.Errorf("获取版本列表失败: %w", err)
	}

	totalCount := len(versions)
	truncated := false
	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
		truncated = true
	}

	currentVer := strings.TrimPrefix(version, "v")

	if format == "json" {
		items := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			items = append(items, map[string]any{
				"version":    "v" + v.Version,
				"date":       v.Date,
				"prerelease": v.Prerelease,
				"installed":  v.Version == currentVer,
				"changelog":  parseChangelogEntries(v.Changelog, 10),
			})
		}
		result := map[string]any{
			"current_version": ensureV(version),
			"versions":        items,
			"track":           string(track),
			"total":           totalCount,
		}
		if truncated {
			result["truncated"] = true
			result["shown"] = limit
		}
		return writeJSON(cmd.OutOrStdout(), result)
	}

	if totalCount == 0 {
		fmt.Printf("  %s\n", ugYellow(fmt.Sprintf("未找到任何%s", upgradeTrackVersionName(track))))
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s\n", ugBold(fmt.Sprintf("%-12s %-12s %-12s %s", "VERSION", "DATE", "TYPE", "CHANGELOG")))
	fmt.Printf("  %s\n", ugDim(strings.Repeat("─", 70)))

	for _, v := range versions {
		releaseType := ugGreen("stable")
		if v.Prerelease {
			releaseType = ugYellow("beta")
		}
		versionStr := fmt.Sprintf("v%-11s", v.Version)
		marker := ""
		if v.Version == currentVer {
			versionStr = ugBoldGrn(versionStr)
			marker = ugCyan(" ← 已安装")
		}
		changelog := ugDim(truncateChangelogForList(v.Changelog, 40))
		fmt.Printf("  %s %-12s %-23s %s%s\n", versionStr, v.Date, releaseType, changelog, marker)
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", ugBold("当前版本:"), ugBoldGrn(ensureV(version)))
	if truncated {
		fmt.Printf("  %s\n", ugDim(fmt.Sprintf("显示最近 %d 个版本（共 %d 个），使用 --list --all 查看全部", limit, totalCount)))
	}
	fmt.Printf("  %s\n", ugDim("提示: 使用 dws upgrade --beta 安装最新 beta；使用 --version v1.0.7 安装指定版本"))
	return nil
}

// --- dws upgrade --rollback ---

func runUpgradeRollback(yes bool) error {
	rm := newUpgradeRollback()

	backups, err := rm.ListBackups()
	if err != nil {
		return fmt.Errorf("获取备份列表失败: %w", err)
	}
	if len(backups) == 0 {
		return fmt.Errorf("没有可用的备份，无法回滚")
	}

	target := backups[0]
	targetVer := ensureV(target.Version)
	currentVer := ensureV(version)

	fmt.Println()
	fmt.Printf("  当前版本:  %s\n", ugBold(currentVer))
	fmt.Printf("  回滚目标:  %s  %s\n", ugCyan(targetVer), ugDim("("+target.CreatedAt.Format("2006-01-02 15:04")+")"))

	if !yes {
		fmt.Println()
		fmt.Printf("是否回滚到 %s? [y/N] ", ugBold(targetVer))
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("已取消")
			return nil
		}
	}

	fmt.Print("  回滚中...")
	if err := rm.RollbackTo(target); err != nil {
		return fmt.Errorf("\n回滚失败: %w", err)
	}
	fmt.Printf(" %s\n", ugGreen("✓"))

	fmt.Println()
	fmt.Printf("  %s\n", ugDim("──────────────────────────────────────"))
	fmt.Printf("  %s 已回滚  %s %s %s\n", ugBoldGrn("✔"), ugDim(currentVer), ugBold("→"), ugBoldGrn(targetVer))
	fmt.Printf("  %s\n", ugDim("──────────────────────────────────────"))
	fmt.Println()
	fmt.Printf("  %s\n", ugDim("运行 dws version 验证当前版本"))
	return nil
}

// --- dws upgrade (full) ---
//
// The upgrade flow is split into two phases for atomicity:
//   Phase 1 (Prepare): download, verify, extract — all in a temp directory, zero side effects.
//   Phase 2 (Apply):   replace binary + install skills — only runs if Phase 1 fully succeeds.
// If anything fails in Phase 1, no files on disk are modified.

// writeDryRunPlan renders the steps that `dws upgrade` would perform, without
// touching the filesystem. Kept side-effect-free and writer-injectable so the
// --dry-run contract can be asserted in tests.
func writeDryRunPlan(w io.Writer, currentVer, binaryAssetName string, hasSkills bool) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s 预览模式，不会下载或修改任何文件\n", ugBold("[dry-run]"))
	fmt.Fprintf(w, "  将执行以下操作:\n")
	fmt.Fprintf(w, "    [1/5] 备份当前版本 %s\n", ugDim(ensureV(currentVer)))
	fmt.Fprintf(w, "    [2/5] 下载 %s\n", ugCyan(binaryAssetName))
	if hasSkills {
		fmt.Fprintf(w, "          下载 %s\n", ugCyan("dws-skills.zip"))
	}
	fmt.Fprintf(w, "    [3/5] 校验 SHA256\n")
	fmt.Fprintf(w, "    [4/5] 解压并验证\n")
	replaceStep := "替换二进制"
	if hasSkills {
		replaceStep += " 并安装技能包"
	}
	fmt.Fprintf(w, "    [5/5] %s\n", replaceStep)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", ugDim("移除 --dry-run 以实际执行升级"))
}

func runUpgrade(ctx context.Context, opts upgradeOptions) error {
	fmt.Printf("  %s\n", ugDim(fmt.Sprintf("检查更新%s...", upgradeTrackSuffix(opts.track))))

	if err := ensureUpgradeDirs(); err != nil {
		return fmt.Errorf("初始化目录结构失败: %w", err)
	}

	cleanupUpgradeStale()

	client := newUpgradeReleaseClient()
	var release *upgrade.ReleaseInfo
	var err error

	if opts.targetVersion != "" {
		fmt.Printf("  指定版本: %s\n", ugCyan(ensureV(opts.targetVersion)))
		release, err = client.FetchReleaseByTag(opts.targetVersion)
		if err != nil {
			return fmt.Errorf("获取版本 %s 信息失败: %w", opts.targetVersion, err)
		}
	} else {
		release, err = client.FetchLatestReleaseForTrack(opts.track)
		if err != nil {
			return fmt.Errorf("检查更新失败: %w", err)
		}
	}

	currentVer := version
	if !opts.force && !upgradeNeedsUpgrade(currentVer, release.Version) {
		fmt.Printf("\n  %s 已是最新版本 %s\n", ugBoldGrn("✔"), ugBold(ensureV(currentVer)))
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s  %s %s %s\n", ugBold("新版本可用:"), ugDim(ensureV(currentVer)), ugBold("→"), ugBoldGrn("v"+release.Version))
	if release.Date != "" {
		fmt.Printf("  %s  %s\n", ugBold("发布日期:  "), release.Date)
	}
	if release.Prerelease {
		fmt.Printf("  %s  %s\n", ugBold("轨道:      "), ugYellow("beta / pre-release"))
	}

	// --dry-run: preview only. Resolve the platform asset so a missing build is
	// still reported, then describe the steps that *would* run and return before
	// any side effect (no backup, no download, no replace). Matches the global
	// flag's contract: "预览操作内容，不实际执行".
	if opts.dryRun {
		binaryAsset, err := findUpgradeBinary(release.Assets)
		if err != nil {
			return err
		}
		hasSkills := findUpgradeSkills(release.Assets) != nil && !opts.skipSkills
		writeDryRunPlan(os.Stdout, currentVer, binaryAsset.Name, hasSkills)
		return nil
	}

	if !opts.yes {
		fmt.Println()
		fmt.Printf("是否升级? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("已取消")
			return nil
		}
	}

	binaryAsset, err := findUpgradeBinary(release.Assets)
	if err != nil {
		return err
	}

	tmpDir, err := upgradeMkdirTemp(upgrade.DownloadCacheDir(), "upgrade-*")
	if err != nil {
		tmpDir, err = upgradeMkdirTemp("", "dws-upgrade-*")
		if err != nil {
			return fmt.Errorf("创建临时目录失败: %w", err)
		}
	}
	defer upgradeRemoveAll(tmpDir)

	hasSkills := findUpgradeSkills(release.Assets) != nil && !opts.skipSkills
	// Steps: 1.备份  2.下载  3.校验  4.解压验证  5.替换+安装
	const totalSteps = 5
	stepFmt := func(n int) string { return ugBold(fmt.Sprintf("[%d/%d]", n, totalSteps)) }

	// ========================================================================
	// Phase 1: Prepare (download + verify + extract — no side effects)
	// ========================================================================

	fmt.Println()

	// --- Step 1: Backup ---
	fmt.Printf("  %s 备份当前版本...", stepFmt(1))
	rm := newUpgradeRollback()
	_, backupErr := rm.Backup(strings.TrimPrefix(currentVer, "v"))
	if backupErr != nil {
		fmt.Printf(" %s %v\n", ugYellow("⚠"), backupErr)
	} else {
		fmt.Printf(" %s\n", ugGreen("✓"))
	}

	// Fetch checksums.txt (needed for strict verification of both binary and skills)
	var checksumsContent string
	checksumsAsset := findUpgradeChecksums(release.Assets)
	if checksumsAsset != nil {
		checksumsPath := filepath.Join(tmpDir, "checksums.txt")
		if _, dlErr := downloadUpgradeFile(checksumsAsset.BrowserDownloadURL, checksumsPath); dlErr == nil {
			if data, readErr := upgradeReadFile(checksumsPath); readErr == nil {
				checksumsContent = string(data)
			}
		}
	}

	// --- Step 2: Download (binary + skills together) ---
	sl := stepFmt(2)
	progressPrefix := fmt.Sprintf("  %s 下载 %s", sl, ugCyan(binaryAsset.Name))
	fmt.Print(progressPrefix)
	start := time.Now()
	binaryArchivePath := filepath.Join(tmpDir, binaryAsset.Name)
	n, err := downloadUpgradeProgress(ctx, binaryAsset.BrowserDownloadURL, binaryArchivePath,
		func(percent float64, downloaded, total int64) {
			bar := progressBar(percent)
			fmt.Printf("\r  %s 下载 %s [%s] %5.1f%%", sl, ugCyan(binaryAsset.Name), ugCyan(bar), percent)
		})
	if err != nil {
		fmt.Println()
		return fmt.Errorf("下载二进制失败: %w", err)
	}
	elapsed := time.Since(start)
	clearLine := strings.Repeat(" ", 100)

	var skillsZipPath string
	if hasSkills {
		skillsAsset := findUpgradeSkills(release.Assets)
		fmt.Printf("\r%s\r%s %s %s\n", clearLine, progressPrefix, ugGreen("✓"), ugDim(fmt.Sprintf("(%.1fMB, %.1fs)", float64(n)/1024/1024, elapsed.Seconds())))

		fmt.Printf("        下载 %s...", ugCyan("dws-skills.zip"))
		skillsZipPath = filepath.Join(tmpDir, "dws-skills.zip")
		if _, dlErr := downloadUpgradeFile(skillsAsset.BrowserDownloadURL, skillsZipPath); dlErr != nil {
			fmt.Printf(" %s\n", ugRed("✗"))
			return fmt.Errorf("技能包下载失败: %w", dlErr)
		}
		fmt.Printf(" %s\n", ugGreen("✓"))
	} else {
		fmt.Printf("\r%s\r%s %s %s\n", clearLine, progressPrefix, ugGreen("✓"), ugDim(fmt.Sprintf("(%.1fMB, %.1fs)", float64(n)/1024/1024, elapsed.Seconds())))
	}

	// --- Step 3: Verify SHA256 (binary + skills together) ---
	if err := verifyUpgradeFile(stepFmt(3), binaryArchivePath, binaryAsset.Name, binaryAsset.Digest, checksumsContent); err != nil {
		return err
	}
	if hasSkills {
		skillsAsset := findUpgradeSkills(release.Assets)
		if err := verifyUpgradeFile("     ", skillsZipPath, "dws-skills.zip", skillsAsset.Digest, checksumsContent); err != nil {
			return err
		}
	}

	// --- Step 4: Extract + validate ---
	fmt.Printf("  %s 解压并验证...", stepFmt(4))
	extractDir := filepath.Join(tmpDir, "extracted")
	if strings.HasSuffix(binaryAsset.Name, ".zip") {
		if err := extractUpgradeZip(binaryArchivePath, extractDir); err != nil {
			fmt.Println()
			return fmt.Errorf("解压失败: %w", err)
		}
	} else {
		if err := extractUpgradeTarGz(binaryArchivePath, extractDir); err != nil {
			fmt.Println()
			return fmt.Errorf("解压失败: %w", err)
		}
	}
	binaryPath := findExtractedBinary(extractDir)
	if binaryPath == "" {
		fmt.Println()
		return fmt.Errorf("在解压目录中未找到 dws 二进制文件")
	}
	if err := validateUpgradeBinary(binaryPath, release.Version); err != nil {
		fmt.Println()
		return fmt.Errorf("验证失败: %w", err)
	}

	var skillSrc string
	if hasSkills {
		skillsExtractDir := filepath.Join(tmpDir, "skills-extracted")
		_ = upgradeMkdirAll(skillsExtractDir, 0755)
		if err := extractUpgradeZip(skillsZipPath, skillsExtractDir); err != nil {
			fmt.Println()
			return fmt.Errorf("技能包解压失败 (文件可能损坏，请检查网络后重试): %w", err)
		}
		skillSrc = locateUpgradeSkill(skillsExtractDir)
		if skillSrc == "" {
			fmt.Println()
			return fmt.Errorf("技能包结构异常 (未找到 SKILL.md)，请反馈到 GitHub Issues")
		}
	}
	fmt.Printf(" %s\n", ugGreen("✓"))

	// ========================================================================
	// Phase 2: Apply (all preparations succeeded — now do the actual changes)
	// ========================================================================

	// --- Step 5: Replace binary + install skills ---
	fmt.Printf("  %s 替换并安装...", stepFmt(5))
	if err := replaceUpgradeSelf(binaryPath); err != nil {
		fmt.Printf(" %s\n", ugRed("✗"))
		return fmt.Errorf("替换二进制失败: %w", err)
	}

	if hasSkills {
		result, installErr := installUpgradeSkills(skillSrc)
		if installErr != nil {
			fmt.Printf(" %s\n", ugRed("✗"))
			return fmt.Errorf("技能包安装失败: %w", installErr)
		}
		failed := result.Failed()
		if len(failed) > 0 {
			fmt.Printf(" %s\n", ugRed("✗"))
			for _, d := range failed {
				fmt.Printf("       %s %s %s\n", ugRed("✗"), shortenHome(d.Dir), ugDim(d.Err.Error()))
			}
			return fmt.Errorf("技能包安装到 %d 个目录失败，请检查权限后手动重试: dws upgrade --force", len(failed))
		}
		succeeded := result.Succeeded()
		fmt.Printf(" %s\n", ugGreen("✓"))
		fmt.Printf("       %s %s\n", ugGreen("✓"), ugDim("二进制已替换"))
		fmt.Printf("       %s %s\n", ugGreen("✓"), ugDim(fmt.Sprintf("技能包已安装 (%d 个位置)", len(succeeded))))
		for _, d := range succeeded {
			fmt.Printf("         %s %s\n", ugDim("→"), ugCyan(shortenHome(d.Dir)))
		}
	} else {
		fmt.Printf(" %s\n", ugGreen("✓"))
	}

	// Discovery cache purge removed — static endpoint mode has no discovery cache.

	// Cleanup old backups
	rm.Cleanup(5)

	// Summary
	fmt.Println()
	fmt.Printf("  %s\n", ugDim("──────────────────────────────────────"))
	fmt.Printf("  %s 升级完成  %s %s %s\n", ugBoldGrn("✔"), ugDim(ensureV(currentVer)), ugBold("→"), ugBoldGrn("v"+release.Version))
	fmt.Printf("  %s\n", ugDim("──────────────────────────────────────"))
	fmt.Println()
	fmt.Printf("  %s\n", ugDim("运行 dws version 验证当前版本"))
	fmt.Printf("  %s\n", ugDim("如遇问题，运行 dws upgrade --rollback 回滚"))

	return nil
}

// strictVerifyFile performs SHA256 verification with strict semantics:
//   - If checksum info is available and matches → ✓
//   - If checksum info is available but MISMATCHES → error (abort upgrade)
//   - If no checksum info at all → skip (no data to compare against)
func strictVerifyFile(label, filePath, fileName, assetDigest, checksumsContent string) error {
	fmt.Printf("  %s 校验 %s...", label, fileName)

	// Source 1: checksums.txt
	if checksumsContent != "" {
		checksums := upgrade.ParseChecksumFile(checksumsContent)
		if expectedHash, ok := checksums[fileName]; ok {
			if err := upgrade.VerifySHA256(filePath, expectedHash); err != nil {
				fmt.Printf(" %s\n", ugRed("✗"))
				return fmt.Errorf("SHA256 校验失败 (%s): %w\n       文件可能被篡改或下载不完整，请重试升级", fileName, err)
			}
			fmt.Printf(" %s\n", ugGreen("✓"))
			return nil
		}
	}

	// Source 2: GitHub asset digest
	if digest := upgrade.ExtractDigestSHA256(assetDigest); digest != "" {
		if err := upgrade.VerifySHA256(filePath, digest); err != nil {
			fmt.Printf(" %s\n", ugRed("✗"))
			return fmt.Errorf("SHA256 校验失败 (%s): %w\n       文件可能被篡改或下载不完整，请重试升级", fileName, err)
		}
		fmt.Printf(" %s\n", ugGreen("✓"))
		return nil
	}

	// No checksum info available at all
	fmt.Printf(" %s\n", ugDim("- 跳过 (无可用校验信息)"))
	return nil
}

// validateNewBinary checks the downloaded binary is valid.
func validateNewBinary(binaryPath, expectedVersion string) error {
	info, err := upgradeStat(binaryPath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("文件为空")
	}
	if err := upgradeChmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("设置执行权限失败: %w", err)
	}

	out, err := upgradeTryExecVersion(binaryPath)
	if err != nil {
		// Apple Silicon kills unsigned arm64 binaries with SIGKILL via amfid.
		// Repair the binary in-place (ad-hoc codesign + drop quarantine) and retry once.
		if upgradeRuntimeGOOS == "darwin" && isLikelyAMFIKill(err) {
			if repairErr := upgradeRepairDarwin(binaryPath); repairErr == nil {
				out, err = upgradeTryExecVersion(binaryPath)
			}
		}
		if err != nil {
			return fmt.Errorf("二进制无法执行: %w", err)
		}
	}

	if !strings.Contains(string(out), expectedVersion) {
		// Not fatal, version format might differ
		fmt.Printf("\n  注意: 版本输出中未包含 %s", expectedVersion)
	}
	return nil
}

func tryExecVersion(binaryPath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, binaryPath, "version").CombinedOutput()
}

// isLikelyAMFIKill returns true when err looks like macOS amfid SIGKILL'ing an
// unsigned binary. Go reports this as "signal: killed".
func isLikelyAMFIKill(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "signal: killed") || strings.Contains(msg, "signal: kill")
}

// repairDarwinBinary applies an ad-hoc codesign and clears the quarantine xattr.
// Used as a self-heal step when an unsigned binary is killed by amfid on Apple Silicon.
func repairDarwinBinary(binaryPath string) error {
	// Best-effort: strip quarantine. Failure is fine (attribute often absent).
	_, _ = upgradeCommandOutput("xattr", "-d", "com.apple.quarantine", binaryPath)

	if _, err := upgradeLookPath("codesign"); err != nil {
		return fmt.Errorf("codesign 不可用: %w", err)
	}
	out, err := upgradeCommandOutput("codesign", "--force", "--sign", "-", binaryPath)
	if err != nil {
		return fmt.Errorf("codesign 失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// extractTarGz extracts a .tar.gz file using the system tar command.
func extractTarGz(archivePath, destDir string) error {
	_ = upgradeMkdirAll(destDir, 0755)
	if out, err := upgradeCommandOutput("tar", "xzf", archivePath, "-C", destDir); err != nil {
		return fmt.Errorf("tar 解压失败: %v: %s", err, string(out))
	}
	return nil
}

func progressBar(percent float64) string {
	width := 20
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// parseChangelogEntries extracts human-readable commit messages from a
// GitHub Release body. The body typically looks like:
//
//	## Changelog
//	* abcdef1234 - some commit message
//	* 0123456789 Merge branch 'main' into main
//
// We strip the hash prefix and skip noisy entries (Merge branch, Merge pull request).
func parseChangelogEntries(body string, maxEntries int) []string {
	var entries []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "- ")

		msg := stripCommitHash(line)
		if msg == "" {
			continue
		}
		if isNoiseCommit(msg) {
			continue
		}
		entries = append(entries, msg)
		if maxEntries > 0 && len(entries) >= maxEntries {
			break
		}
	}
	return entries
}

// truncateChangelog returns a short one-line summary for the --check output.
func truncateChangelog(body string) string {
	entries := parseChangelogEntries(body, 3)
	if len(entries) == 0 {
		return ""
	}
	return strings.Join(entries, "; ")
}

// truncateChangelogForList returns a compact summary for the --list table.
func truncateChangelogForList(body string, maxLen int) string {
	entries := parseChangelogEntries(body, 2)
	if len(entries) == 0 {
		return "-"
	}
	summary := strings.Join(entries, "; ")
	if len(summary) > maxLen {
		return summary[:maxLen-3] + "..."
	}
	return summary
}

// stripCommitHash removes a leading Git commit hash (7-40 hex chars)
// and optional separator (" - ", " ") from a line.
func stripCommitHash(line string) string {
	if len(line) < 8 {
		return line
	}
	// Check if line starts with hex chars (commit hash)
	hashEnd := 0
	for hashEnd < len(line) && hashEnd < 40 {
		c := line[hashEnd]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			hashEnd++
		} else {
			break
		}
	}
	if hashEnd < 7 {
		return line
	}
	rest := line[hashEnd:]
	rest = strings.TrimPrefix(rest, " - ")
	rest = strings.TrimLeft(rest, " ")
	return rest
}

func isNoiseCommit(msg string) bool {
	lower := strings.ToLower(msg)
	noisePatterns := []string{
		"merge branch",
		"merge pull request",
		"merge remote-tracking",
	}
	for _, p := range noisePatterns {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// ensureV ensures a version string has a "v" prefix for display consistency.
// Non-semver values like "dev" or "unknown" are returned as-is.
func ensureV(ver string) string {
	if ver == "" {
		return "v0.0.0"
	}
	if strings.HasPrefix(ver, "v") {
		return ver
	}
	// Only add "v" prefix for semver-like strings (starts with digit)
	if len(ver) > 0 && ver[0] >= '0' && ver[0] <= '9' {
		return "v" + ver
	}
	return ver
}

func upgradeTrack(beta bool) upgrade.ReleaseTrack {
	if beta {
		return upgrade.ReleaseTrackBeta
	}
	return upgrade.ReleaseTrackRelease
}

func upgradeTrackSuffix(track upgrade.ReleaseTrack) string {
	if track == upgrade.ReleaseTrackBeta {
		return " (beta)"
	}
	return ""
}

func upgradeTrackVersionName(track upgrade.ReleaseTrack) string {
	if track == upgrade.ReleaseTrackBeta {
		return "beta 版本"
	}
	return "正式 release 版本"
}

func upgradeHintForTrack(track upgrade.ReleaseTrack) string {
	if track == upgrade.ReleaseTrackBeta {
		return "运行 dws upgrade --beta 进行升级"
	}
	return "运行 dws upgrade 进行升级"
}

// resolveUpgradeFormat returns "json" only when the user explicitly passes -f json.
// Unlike other commands, upgrade defaults to table (human-friendly) output.
func resolveUpgradeFormat(cmd *cobra.Command) string {
	pf := cmd.Root().PersistentFlags()
	if pf.Changed("format") {
		if f, err := pf.GetString("format"); err == nil {
			return strings.ToLower(strings.TrimSpace(f))
		}
	}
	return "table"
}

func writeJSON(w interface{ Write([]byte) (int, error) }, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func shortenHome(path string) string {
	homeDir, err := upgradeUserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}
