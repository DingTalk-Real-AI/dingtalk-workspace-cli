package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// skillSetupAgentHomes is the ordered list of agent home subdirectories
// where dws skills get installed. Mirrors install.sh / install.ps1 /
// build/npm/install.js so that `dws skill setup` and the install scripts
// agree on the install footprint.
var skillSetupAgentHomes = []string{
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
	".qoderwork/skills",
	// beta.6 compatibility roots: cleanup only.
	".cursor/skills",
	".gemini/skills",
	".codex/skills",
	".github/skills",
	".windsurf/skills",
	".cline/skills",
	".amp/skills",
}

const (
	skillSetupModeMono  = "mono"
	skillSetupModeMulti = "multi"
)

var (
	skillSetupResolveMode     = resolveSkillSetupMode
	skillSetupResolveSource   = resolveSkillSetupSourceOrEmbedded
	skillSetupResolveTargets  = resolveSkillSetupTargets
	skillSetupListMulti       = listMultiSkillNames
	skillSetupFilterMulti     = filterMultiSkillNames
	skillSetupBuildPlan       = buildSkillSetupPlan
	skillSetupConfirmPlan     = confirmSkillSetupPlan
	skillSetupExecutePlan     = executeSkillSetupPlan
	skillSetupCopyDir         = copyDir
	skillSetupInstallMulti    = installMultiSkillToHomes
	skillSetupPublishTemp     = os.MkdirTemp
	skillSetupPublishRename   = os.Rename
	skillSetupMkdirTemp       = os.MkdirTemp
	skillSetupRename          = os.Rename
	skillSetupRunForm         = (*huh.Form).Run
	skillSetupInteractive     = isInteractiveTerminal
	skillSetupReadDir         = os.ReadDir
	skillSetupStat            = os.Stat
	skillSetupLstat           = os.Lstat
	skillSetupGetenv          = os.Getenv
	skillSetupSymlink         = os.Symlink
	skillSetupExecutable      = os.Executable
	skillSetupGetwd           = os.Getwd
	skillSetupUserHomeDir     = os.UserHomeDir
	skillSetupRemoveAll       = os.RemoveAll
	skillSetupBackupAndRemove = upgrade.BackupAndRemoveSkillDir
	skillSetupRestoreBackup   = upgrade.RestoreSkillPath
	skillSetupMkdirAll        = os.MkdirAll
	skillSetupWalk            = filepath.Walk
	skillSetupRel             = filepath.Rel
	skillSetupReadlink        = os.Readlink
	skillSetupEvalSymlinks    = filepath.EvalSymlinks
	skillSetupOpen            = os.Open
	skillSetupOpenFile        = os.OpenFile
	skillSetupWriteFile       = os.WriteFile
	skillSetupBuildProvenance = skillprovenance.Build
	skillSetupCopy            = io.Copy
	skillSetupReadState       = skillstate.Read
	skillSetupWriteState      = skillstate.Write
	skillSetupRemoveState     = skillstate.Remove
	skillSetupPublishPath     = upgrade.PublishSkillPathNoReplace
	skillSetupRollbackPaths   = upgrade.RollbackSkillPathPublications
	skillSetupNow             = time.Now
	skillSetupFoldPathCase    = runtime.GOOS == "windows"
)

type skillSetupBackup struct {
	Path   string
	Reason string
}

type skillSetupTargetPlan struct {
	Destination   string
	CanonicalBase string
	Backups       []skillSetupBackup
	CleanupOnly   bool
	LinkCanonical bool
}

type skillSetupPlan struct {
	Mode                       string
	Source                     string
	MultiSkillNames            []string
	Filtered                   bool
	Targets                    []skillSetupTargetPlan
	EventMiscMigrationTargets  []string
	InstallsEventMiscCompanion bool
}

type skillSetupStagedDir struct {
	staged string
	dest   string
}

type skillSetupBackedUpDir struct {
	original string
	backup   string
}

const (
	skillSetupBackupMutual  = "opposite layout"
	skillSetupBackupStale   = "stale official Skill"
	skillSetupBackupReplace = "same-name Skill"
)

func newSkillSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "安装 dws 自身 skill 到 Agent 目录",
		Long: `安装 dws 自身 skill 文档到 AI Agent 目录（如 ~/.claude/skills/、~/.cursor/skills/ 等）。

支持两种模式：
  multi                 多 skill（默认）—— 按产品拆 N 个独立 skill（dingtalk-*）
  mono                  单 skill（legacy）—— 总入口 SKILL.md + references/products/

multi 模式支持按产品挑选：
  -s/--skill   只装指定子 skill（可重复，短名 aitable 或全名 dingtalk-aitable 均可）
  -x/--exclude 从全装里剔除指定子 skill（可重复，与 --skill 互斥）
  用 -s/-x 挑选时未列出的已有 dingtalk-* skill 会保留（additive 叠加语义）；
  不带过滤条件的全量安装只清理统一状态中登记，或属于状态上线前精确官方
  名称集合且不在 bundle 内的过期 Skill。
  setup 成功后记录本次官方清单；后续每次 dws upgrade 都按新版本官方清单
  全量覆盖预制 skill，因此本地删除或 setup 时排除的预制 skill 会在升级时恢复。
清理与备份（本命令可能移除的目录）：
  · 安装任一模式前会清理对面模式残留：装 mono 移除统一状态中登记，或属于
    状态上线前精确官方名称集合的 multi Skill，
    装 multi 移除 <agent-home>/dws/；全量 multi 安装还会移除不在 bundle 内的
    过期受管 Skill。仅有 dingtalk-* 前缀的市场/用户 Skill 不会被清理。
  · 被移除的目录与同名旧 skill 会先备份到 ~/.dws/skill-backups/<时间戳>/；
    备份失败时保留原目录并跳过该目标，绝不静默删除。
  · 所有将被移除的目录都会在确认前逐条列出。

不带 --mode 时进入交互式询问；Skill 统一安装到 ~/.agents/skills。
DWS 会自动适配本机上检测到的 Agent；共享安装方式不可用时会自动改用兼容安装，
无需用户手动处理，也不会让同一个 Skill 重复出现。
skill 源默认取二进制内嵌的版本（升级二进制即升级 skill）；--source / DWS_SKILL_SOURCE 可显式覆盖。`,
		Example: `  dws skill setup --mode multi --target claude --dry-run
  dws skill setup --mode multi --target claude`,
		DisableAutoGenTag: true,
		RunE:              runSkillSetup,
	}
	cmd.Flags().String("mode", "", "skill 模式：mono | multi（不指定则交互询问）")
	cmd.Flags().String("target", "all", "目标 Agent：all | "+supportedTargets())
	cmd.Flags().String("source", "", "skill 源目录（默认使用二进制内嵌的 skill 源，与当前版本一致）")
	cmd.Flags().Bool("yes", false, "跳过确认提示（仅供脚本使用；删除操作仍会先备份到 ~/.dws/skill-backups/）")
	cmd.Flags().StringSliceP("skill", "s", nil, "multi 模式：仅安装指定子 skill（可重复，接受短名 aitable 或全名 dingtalk-aitable）")
	cmd.Flags().StringSliceP("exclude", "x", nil, "multi 模式：从全装中剔除指定子 skill（可重复，与 --skill 互斥）")
	return cmd
}

func runSkillSetup(cmd *cobra.Command, _ []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	target, _ := cmd.Flags().GetString("target")
	source, _ := cmd.Flags().GetString("source")
	autoYes, _ := cmd.Flags().GetBool("yes")
	includeRaw, _ := cmd.Flags().GetStringSlice("skill")
	excludeRaw, _ := cmd.Flags().GetStringSlice("exclude")

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	mode, err := skillSetupResolveMode(mode, autoYes, out)
	if err != nil {
		return err
	}

	if mode == skillSetupModeMono && (len(includeRaw) > 0 || len(excludeRaw) > 0) {
		return fmt.Errorf("--skill / --exclude 仅在 --mode multi 下有效（mono 只有一个 skill，无需挑选）")
	}

	skillSrc, srcCleanup, err := skillSetupResolveSource(source, mode)
	if err != nil {
		return err
	}
	defer srcCleanup()

	dests, err := skillSetupResolveTargets(target, mode)
	if err != nil {
		return err
	}

	// multi 模式枚举 src 下的子 skill 名，供确认信息与安装步骤共用
	var multiSkillNames, allMultiSkillNames []string
	var foldedEventMiscTargets []string
	var migrateEventMiscTargets []string
	var installsEventMiscCompanion bool
	if mode == skillSetupModeMulti {
		var listErr error
		allMultiSkillNames, listErr = skillSetupListMulti(skillSrc)
		if listErr != nil {
			return listErr
		}
		if len(allMultiSkillNames) == 0 {
			return fmt.Errorf("multi 模式下 %s 内未发现含 SKILL.md 的子目录", skillSrc)
		}
		filtered, filterErr := skillSetupFilterMulti(allMultiSkillNames, includeRaw, excludeRaw)
		if filterErr != nil {
			return filterErr
		}
		// dingtalk-shared carries the global rules every product skill declares as a
		// PREREQUISITE; it must ship even when --skill / --exclude narrows the set.
		multiSkillNames = ensureMandatorySharedSkill(filtered, allMultiSkillNames)

		foldedEventMiscTargets = findFoldedEventMiscTargets(dests)
		if len(foldedEventMiscTargets) > 0 {
			hasEvent := containsSkillName(multiSkillNames, multiEventSkill)
			hasMisc := containsSkillName(multiSkillNames, multiMiscSkill)
			switch {
			case normalizedSkillListContains(excludeRaw, multiEventSkill):
				return fmt.Errorf("检测到已有 dingtalk-misc 仍承载个人 Event 路由；不能显式 --exclude event，请先完成 dingtalk-event 迁移")
			case hasMisc && !hasEvent:
				return fmt.Errorf("检测到已有 dingtalk-misc 仍承载个人 Event 路由；不能只覆盖 dingtalk-misc，必须同时迁移 dingtalk-event")
			case hasEvent:
				if normalizedSkillListContains(excludeRaw, multiMiscSkill) {
					return fmt.Errorf("检测到已有 dingtalk-misc 仍承载个人 Event 路由；本次安装 dingtalk-event 必须同时迁移 dingtalk-misc，不能显式 --exclude misc")
				}
				if !containsSkillName(allMultiSkillNames, multiMiscSkill) {
					return fmt.Errorf("检测到已有 dingtalk-misc 仍承载个人 Event 路由，但当前 multi 源缺少迁移所需的 %s", multiMiscSkill)
				}
				if err := validateEventMiscMigrationSource(skillSrc); err != nil {
					return err
				}
				migrateEventMiscTargets = append(migrateEventMiscTargets, foldedEventMiscTargets...)
				installsEventMiscCompanion = !hasMisc
			}
		}
	}
	// filtered 决定 multi 安装的清理语义：带 -s/--skill 或 -x/--exclude
	// 时保持 additive（不动未列出的 sibling）；全量安装与 install.sh /
	// install.js 对齐，清掉不在 bundle 内且有明确 DWS 所有权记录的过期 Skill。
	filtered := len(includeRaw) > 0 || len(excludeRaw) > 0
	plan, err := skillSetupBuildPlan(mode, skillSrc, dests, multiSkillNames, filtered)
	if err != nil {
		return fmt.Errorf("无法完整计算 Skill 安装计划: %w", err)
	}
	configureEventMiscMigrationPlan(plan, migrateEventMiscTargets, installsEventMiscCompanion)

	// --dry-run 与交互确认消费同一份计划，所以展示的备份路径
	// 与执行阶段传给 BackupAndRemove 的路径完全一致。
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		fmt.Fprintf(out, "[DRY-RUN] 预览（不写入任何文件）：mode=%s\n", plan.Mode)
		renderSkillSetupPlan(out, plan)
		return nil
	}

	if !autoYes {
		ok, err := skillSetupConfirmPlan(out, plan)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "已取消。")
			return nil
		}
	}

	var managedSkills []skillprovenance.Record
	if mode == skillSetupModeMulti {
		updatedSkillNames := append([]string(nil), multiSkillNames...)
		if installsEventMiscCompanion && !containsSkillName(updatedSkillNames, multiMiscSkill) {
			updatedSkillNames = append(updatedSkillNames, multiMiscSkill)
			sort.Strings(updatedSkillNames)
		}
		managedSkills, err = buildSkillProvenanceRecords(plan.Source, updatedSkillNames, RawVersion(), skillprovenance.SourceSkillSetup)
		if err != nil {
			return fmt.Errorf("生成统一 Skill provenance 失败: %w", err)
		}
		if filtered {
			home, homeErr := skillSetupUserHomeDir()
			if homeErr != nil {
				return fmt.Errorf("无法解析 HOME 以读取 Skill 统一状态: %w", homeErr)
			}
			previous, readable, readErr := skillSetupReadState(home)
			if readErr != nil {
				return fmt.Errorf("读取 Skill 统一状态失败: %w", readErr)
			}
			if readable {
				managedSkills = skillprovenance.Merge(previous.ManagedSkills, managedSkills)
			}
		}
	}

	var installed, skipped int
	if mode == skillSetupModeMulti && len(migrateEventMiscTargets) > 0 {
		installed, skipped, err = installMultiSkillsWithEventMigration(
			skillSrc,
			multiSkillNames,
			dests,
			migrateEventMiscTargets,
			filtered,
			out,
			errOut,
		)
	} else {
		installed, skipped, err = skillSetupExecutePlan(plan, out, errOut)
	}
	if err != nil {
		return err
	}
	if mode == skillSetupModeMulti && len(migrateEventMiscTargets) > 0 {
		retiredNames := append([]string(nil), multiSkillNames...)
		if installsEventMiscCompanion && !containsSkillName(retiredNames, multiMiscSkill) {
			retiredNames = append(retiredNames, multiMiscSkill)
		}
		if retireErr := retireMigratedUniversalSkills(migrateEventMiscTargets, retiredNames, out); retireErr != nil {
			// Retiring an obsolete universal copy installs nothing; report it and
			// keep the successful installation rather than failing the run.
			fmt.Fprintf(errOut, "  ⚠️  %v\n", retireErr)
		}
	}
	if skipped > 0 {
		return fmt.Errorf(
			"Skill 安装不完整（mode=%s, installed=%d, skipped=%d）；修复失败原因后请重试 setup，或运行普通 upgrade 全量刷新预制 Skill",
			mode,
			installed,
			skipped,
		)
	}
	if installed > 0 {
		home, homeErr := skillSetupUserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("skill 已安装，但无法解析 HOME 以保存更新状态: %w", homeErr)
		}
		if mode == skillSetupModeMulti {
			updatedSkillNames := append([]string(nil), multiSkillNames...)
			if installsEventMiscCompanion && !containsSkillName(updatedSkillNames, multiMiscSkill) {
				updatedSkillNames = append(updatedSkillNames, multiMiscSkill)
				sort.Strings(updatedSkillNames)
			}
			state := skillstate.State{
				Version:        RawVersion(),
				OfficialSkills: allMultiSkillNames,
				UpdatedSkills:  updatedSkillNames,
				ManagedSkills:  managedSkills,
				UpdatedAt:      skillSetupNow().UTC().Format(time.RFC3339),
			}
			if stateErr := skillSetupWriteState(home, state); stateErr != nil {
				return fmt.Errorf("skill 已安装，但保存官方 Skill 信息快照失败: %w", stateErr)
			}
		} else if stateErr := skillSetupRemoveState(home); stateErr != nil {
			return fmt.Errorf("mono Skill 已安装，但清理 multi 更新状态失败: %w", stateErr)
		}
	}
	fmt.Fprintf(out, "\n✅ Skill 安装完成（mode=%s, installed=%d, skipped=%d）\n", mode, installed, skipped)
	fmt.Fprintln(out, "   统一安装位置：~/.agents/skills")
	fmt.Fprintln(out, "   已自动适配本机上检测到的 Agent")
	fmt.Fprintln(out, "ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效。")
	return nil
}

// multiSkillPrefix is the canonical prefix for every per-product skill
// bundle in skills/multi/ (e.g. dingtalk-aitable, dingtalk-calendar).
const multiSkillPrefix = "dingtalk-"

// multiSharedSkill is the shared, non-product skill that every per-product
// skill declares as a PREREQUISITE. It must always be installed in multi mode
// regardless of --skill / --exclude, otherwise the product skills reference a
// dingtalk-shared that was never installed.
const multiSharedSkill = "dingtalk-shared"

// legacySharedSkill is the pre-rename directory name of the shared skill.
// Installations created before the dws-shared -> dingtalk-shared rename keep a
// dws-shared directory on disk; cleanup paths must still recognize it so a
// full install / mode switch removes it instead of leaving an orphaned,
// unreferenced skill next to the new dingtalk-shared.
const legacySharedSkill = "dws-shared"

// isManagedDWSMultiSkillDir reports whether dir is named by unified metadata
// or by the frozen pre-state official migration list.
func isManagedDWSMultiSkillDir(dir string, managed ...map[string]bool) bool {
	if skillstate.IsLegacyOfficialSkillName(filepath.Base(dir)) {
		return true
	}
	return len(managed) > 0 && managed[0][filepath.Base(dir)]
}

func buildSkillProvenanceRecords(root string, names []string, version, source string) ([]skillprovenance.Record, error) {
	records := make([]skillprovenance.Record, 0, len(names))
	for _, name := range names {
		record, err := skillSetupBuildProvenance(name, filepath.Join(root, name), version, source)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// publishDWSManagedSkillDir stages a complete Skill before exposing it.
func publishDWSManagedSkillDir(src, dest string) (err error) {
	parent := filepath.Dir(dest)
	stage, err := skillSetupPublishTemp(parent, "."+filepath.Base(dest)+".tmp-")
	if err != nil {
		return fmt.Errorf("创建 Skill staging 失败 %s: %w", parent, err)
	}
	published := false
	defer func() {
		if published {
			return
		}
		if cleanupErr := skillSetupRemoveAll(stage); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理 Skill staging 失败 %s: %w", stage, cleanupErr))
		}
	}()
	if err := skillSetupCopyDir(src, stage); err != nil {
		return fmt.Errorf("拷贝 Skill staging 失败 %s: %w", stage, err)
	}
	if err := skillSetupPublishRename(stage, dest); err != nil {
		return fmt.Errorf("发布 Skill 失败 %s: %w", dest, err)
	}
	published = true
	return nil
}

// legacyMultiSharedSkill is the retired name shipped by older multi-skill
// bundles. Once the replacement has been installed successfully, remove this
// exact directory so Agent discovery cannot load both routing contracts.
const legacyMultiSharedSkill = legacySharedSkill

const (
	multiEventSkill = "dingtalk-event"
	multiMiscSkill  = "dingtalk-misc"
)

var eventMigrationRequiredReferences = []string{
	"event-im.md",
	"event-im-keys.md",
	"event-im-lifecycle.md",
	"event-im-operations.md",
	"event-im-output.md",
	"event-oa.md",
}

func containsSkillName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func normalizedSkillListContains(raw []string, want string) bool {
	for _, name := range raw {
		if normalizeMultiSkillName(name) == want {
			return true
		}
	}
	return false
}

// findFoldedEventMiscTargets identifies the short-lived multi-skill layout in
// which personal Event routing lived inside dingtalk-misc. Both markers are
// required so an unrelated misc install is never treated as a migration target.
func findFoldedEventMiscTargets(dests []string) []string {
	var targets []string
	for _, dest := range dests {
		miscRoot := filepath.Join(dest, multiMiscSkill)
		skillBody, err := os.ReadFile(filepath.Join(miscRoot, "SKILL.md"))
		if err != nil || !containsPersonalEventRoute(skillBody) {
			continue
		}
		eventRef, err := skillSetupStat(filepath.Join(miscRoot, "references", "event.md"))
		if err != nil || eventRef.IsDir() {
			continue
		}
		targets = append(targets, dest)
	}
	sort.Strings(targets)
	return targets
}

func containsPersonalEventRoute(skillBody []byte) bool {
	body := strings.ToLower(string(skillBody))
	for _, marker := range []string{
		"dws event",
		"个人 event",
		"个人 im 事件",
		"个人 im/oa",
		"personal event",
	} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

func printEventMiscMigrationPreview(out io.Writer, targets []string, installsCompanion bool) {
	if len(targets) == 0 {
		return
	}
	action := "将原子切换 dingtalk-event 与本次已选择的干净 dingtalk-misc"
	if installsCompanion {
		action = "将原子切换 dingtalk-event，并额外安装干净的 dingtalk-misc 作为迁移伴侣（仅限以下目标）"
	}
	fmt.Fprintf(out, "Event Skill 迁移：%s：\n", action)
	for _, target := range targets {
		fmt.Fprintf(out, "  × %s\n", filepath.Join(target, multiEventSkill))
		fmt.Fprintf(out, "  × %s\n", filepath.Join(target, multiMiscSkill))
	}
}

func validateEventMiscMigrationSource(src string) error {
	if err := validateEventMigrationSkillRoot(filepath.Join(src, multiEventSkill)); err != nil {
		return fmt.Errorf("event Skill 迁移源无效: %w", err)
	}
	if err := validateMigrationSkillRoot(filepath.Join(src, multiMiscSkill), multiMiscSkill, nil); err != nil {
		return fmt.Errorf("event Skill 迁移源无效: %w", err)
	}

	if err := validateCleanEventMiscRoot(filepath.Join(src, multiMiscSkill)); err != nil {
		return fmt.Errorf("event Skill 迁移源无效: %w", err)
	}
	return nil
}

func validateEventMigrationSkillRoot(root string) error {
	required := make([]string, 0, len(eventMigrationRequiredReferences))
	for _, name := range eventMigrationRequiredReferences {
		required = append(required, filepath.Join("references", name))
	}
	return validateMigrationSkillRoot(root, multiEventSkill, required)
}

func validateMigrationSkillRoot(root, expectedName string, requiredFiles []string) error {
	skillPath := filepath.Join(root, "SKILL.md")
	skillBody, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("无法读取 %s: %w", skillPath, err)
	}
	name, err := parseMigrationSkillFrontmatter(skillBody)
	if err != nil {
		return fmt.Errorf("%s 无效: %w", skillPath, err)
	}
	if name != expectedName {
		return fmt.Errorf("%s 的 name=%q，期望 %q", skillPath, name, expectedName)
	}
	for _, rel := range requiredFiles {
		path := filepath.Join(root, rel)
		info, statErr := skillSetupStat(path)
		if statErr != nil || info.IsDir() {
			if statErr == nil {
				statErr = errors.New("is a directory")
			}
			return fmt.Errorf("缺少有效文件 %s: %w", path, statErr)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("无法读取 %s: %w", path, readErr)
		}
		if strings.TrimSpace(string(body)) == "" {
			return fmt.Errorf("文件为空 %s", path)
		}
	}
	return nil
}

func parseMigrationSkillFrontmatter(body []byte) (string, error) {
	normalized := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", errors.New("缺少 YAML frontmatter")
	}
	name := ""
	description := ""
	closingLine := -1
	for i := 1; i < len(lines); i++ {
		rawLine := lines[i]
		line := strings.TrimSpace(rawLine)
		if line == "---" {
			closingLine = i
			break
		}
		// Only inspect top-level frontmatter keys. Nested metadata may legally
		// contain its own `name` without changing the Skill identity.
		if strings.TrimLeft(rawLine, " \t") != rawLine {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		switch strings.TrimSpace(key) {
		case "name":
			if name != "" {
				return "", errors.New("frontmatter 含重复 name")
			}
			name = value
		case "description":
			description = value
		}
	}
	if closingLine < 0 {
		return "", errors.New("YAML frontmatter 未闭合")
	}
	if name == "" {
		return "", errors.New("frontmatter 缺少 name")
	}
	if description == "" {
		return "", errors.New("frontmatter 缺少 description")
	}
	if strings.TrimSpace(strings.Join(lines[closingLine+1:], "\n")) == "" {
		return "", errors.New("SKILL.md 正文为空")
	}
	return name, nil
}

func validateCleanEventMiscRoot(miscRoot string) error {
	miscSkillPath := filepath.Join(miscRoot, "SKILL.md")
	miscBody, err := os.ReadFile(miscSkillPath)
	if err != nil {
		return fmt.Errorf("无法读取 %s: %w", miscSkillPath, err)
	}
	if containsPersonalEventRoute(miscBody) {
		return fmt.Errorf("%s 仍包含个人 Event 路由", miscSkillPath)
	}
	refsRoot := filepath.Join(miscRoot, "references")
	entries, err := skillSetupReadDir(refsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法检查 %s: %w", refsRoot, err)
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() && strings.HasPrefix(name, "event") && strings.HasSuffix(name, ".md") {
			return fmt.Errorf("%s 仍存在折叠 Event 参考页", filepath.Join(refsRoot, entry.Name()))
		}
	}
	return nil
}

// ensureMandatorySharedSkill guarantees the shared dependency skill is included
// whenever it exists in the source, even if --skill / --exclude narrowed it out.
func ensureMandatorySharedSkill(selected, all []string) []string {
	hasShared := false
	for _, n := range all {
		if n == multiSharedSkill {
			hasShared = true
			break
		}
	}
	if !hasShared {
		return selected
	}
	for _, n := range selected {
		if n == multiSharedSkill {
			return selected
		}
	}
	return append([]string{multiSharedSkill}, selected...)
}

// normalizeMultiSkillName accepts either the short form (aitable) or the
// full form (dingtalk-aitable) and returns the canonical full form.
// Empty input returns "". Comparison is case-insensitive.
func normalizeMultiSkillName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	if strings.HasPrefix(n, multiSkillPrefix) {
		return n
	}
	return multiSkillPrefix + n
}

// filterMultiSkillNames narrows `all` by include / exclude lists:
//
//   - include + exclude are mutually exclusive (both → error)
//   - names accept short or full form; normalized before matching
//   - unknown names → error, with the available list inlined for discovery
//   - both lists empty → return `all` (install everything)
//   - exclude that drops every name → error (avoid silent no-op install)
//
// The caller threads whether a filter was used into installMultiSkillToHomes:
// filtered installs stay additive; a full unfiltered install also removes
// stale, proven DWS-managed directories that are no longer in the bundle.
func filterMultiSkillNames(all, include, exclude []string) ([]string, error) {
	if len(include) > 0 && len(exclude) > 0 {
		return nil, fmt.Errorf("--skill 与 --exclude 不能同时使用")
	}

	available := make(map[string]struct{}, len(all))
	for _, n := range all {
		available[n] = struct{}{}
	}

	validate := func(raw []string, flagName string) ([]string, error) {
		var normalized []string
		var unknown []string
		seen := make(map[string]bool)
		for _, r := range raw {
			n := normalizeMultiSkillName(r)
			if n == "" {
				continue
			}
			if _, ok := available[n]; !ok {
				unknown = append(unknown, r)
				continue
			}
			if !seen[n] {
				seen[n] = true
				normalized = append(normalized, n)
			}
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("%s 中的以下名称在 multi 源中找不到：%s\n可用列表（共 %d 个）：%s",
				flagName, strings.Join(unknown, ", "), len(all), strings.Join(all, ", "))
		}
		return normalized, nil
	}

	if len(include) > 0 {
		names, err := validate(include, "--skill")
		if err != nil {
			return nil, err
		}
		sort.Strings(names)
		return names, nil
	}
	if len(exclude) > 0 {
		excluded, err := validate(exclude, "--exclude")
		if err != nil {
			return nil, err
		}
		excludedSet := make(map[string]bool, len(excluded))
		for _, n := range excluded {
			excludedSet[n] = true
		}
		var out []string
		for _, n := range all {
			if !excludedSet[n] {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("--exclude 把全部 %d 个子 skill 都剔除了，没有可装的", len(all))
		}
		return out, nil
	}
	return all, nil
}

// listMultiSkillNames returns sorted names of subdirectories under src that
// contain a SKILL.md file (i.e. valid multi-mode skill bundles).
func listMultiSkillNames(src string) ([]string, error) {
	entries, err := skillSetupReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("无法读取 multi skill 源目录 %s: %w", src, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := skillSetupStat(filepath.Join(src, e.Name(), "SKILL.md")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// resolveSkillSetupMode resolves the mode either from the flag or via an
// interactive prompt. If no TTY is available and no mode was given, returns
// an error rather than silently picking a default.
func resolveSkillSetupMode(mode string, autoYes bool, out io.Writer) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case skillSetupModeMono, skillSetupModeMulti:
		return mode, nil
	case "":
		// fall through to interactive prompt
	default:
		return "", fmt.Errorf("不支持的 --mode 值: %s（可选 mono / multi）", mode)
	}

	if autoYes || !skillSetupInteractive() {
		fmt.Fprintln(out, "未指定 --mode，非交互环境下默认使用 multi")
		return skillSetupModeMulti, nil
	}

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 dws skill 安装模式").
				Description("multi = 按产品拆分（默认）\nmono = 单 skill 入口（legacy）").
				Options(
					huh.NewOption("multi — 多 skill（默认）", skillSetupModeMulti),
					huh.NewOption("mono — 单 skill（legacy）", skillSetupModeMono),
				).
				Value(&choice),
		),
	)
	if err := skillSetupRunForm(form); err != nil {
		return "", fmt.Errorf("交互式选择中止: %w", err)
	}
	return choice, nil
}

// resolveSkillSetupSource finds the local skill source directory for the
// given mode ("mono" or "multi"). Explicit overrides (--source flag or
// DWS_SKILL_SOURCE) win and never fall back to another source; without an
// override the ordered candidate list (binary-adjacent, working directory,
// ~/.dws/skills user cache) is probed for a valid skill root of that mode.
func resolveSkillSetupSource(explicit, mode string) (string, error) {
	subdir := mode // "mono" or "multi"

	// An explicit override (--source flag or DWS_SKILL_SOURCE) wins, and an
	// override that does not contain a skill root is an error — never a
	// silent fallback to another source the user did not ask for.
	var overrides []string
	if explicit != "" {
		overrides = append(overrides, explicit, filepath.Join(explicit, "skills", subdir))
	}
	if env := strings.TrimSpace(os.Getenv("DWS_SKILL_SOURCE")); env != "" {
		overrides = append(overrides, env, filepath.Join(env, "skills", subdir))
	}
	if len(overrides) > 0 {
		for _, c := range overrides {
			if isSkillSourceRoot(c, mode) {
				return c, nil
			}
		}
		hint := strings.Join(overrides, "\n  - ")
		return "", fmt.Errorf("未找到 %s 模式的 skill 源目录（--source / DWS_SKILL_SOURCE 显式指定时不回退到内嵌源），已尝试：\n  - %s", mode, hint)
	}

	// No explicit override: legacy fallback only — embedded materialization
	// is handled by resolveSkillSetupSourceOrEmbedded (skill_setup_embed.go),
	// the wrapper that callers use. This branch is reachable only when the
	// wrapper passes through with an empty explicit/env (legacy direct call).
	candidates := skillSourceCandidates("", subdir)
	for _, c := range candidates {
		if isSkillSourceRoot(c, mode) {
			return c, nil
		}
	}

	hint := strings.Join(candidates, "\n  - ")
	return "", fmt.Errorf("未找到 %s 模式的 skill 源目录，已尝试：\n  - %s\n\n请用 --source 显式指定包含 skills/%s 的仓库根目录", mode, hint, mode)
}

// skillSourceCandidates returns the ordered list of paths to probe for a
// skill source root, given an optional explicit override and the mode
// subdir (mono or multi).
func skillSourceCandidates(explicit, subdir string) []string {
	var roots []string
	if explicit != "" {
		// allow either repo root or already-resolved skills/<mode> dir
		roots = append(roots, explicit, filepath.Join(explicit, "skills", subdir))
	}
	if env := strings.TrimSpace(os.Getenv("DWS_SKILL_SOURCE")); env != "" {
		roots = append(roots, env, filepath.Join(env, "skills", subdir))
	}
	if exe, err := skillSetupExecutable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(exeDir, "skills", subdir),
			filepath.Join(exeDir, "..", "skills", subdir),
			filepath.Join(exeDir, "..", "share", "skills", "dws"),
		)
	}
	if wd, err := skillSetupGetwd(); err == nil {
		roots = append(roots, filepath.Join(wd, "skills", subdir))
	}
	// User-level cache populated by install.sh / install.ps1 / npm install.js
	// from the dws-skills.zip release asset. Lets `dws skill setup` find a
	// source even when the user has no source checkout on disk.
	if home, err := skillSetupUserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".dws", "skills", subdir))
	}
	return roots
}

func isSkillSourceRoot(path, mode string) bool {
	if path == "" {
		return false
	}
	switch mode {
	case skillSetupModeMono:
		fi, err := skillSetupStat(filepath.Join(path, "SKILL.md"))
		return err == nil && !fi.IsDir()
	case skillSetupModeMulti:
		entries, err := skillSetupReadDir(path)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.IsDir() {
				if _, err := skillSetupStat(filepath.Join(path, e.Name(), "SKILL.md")); err == nil {
					return true
				}
			}
		}
		return false
	}
	return false
}

// resolveSkillSetupTargets returns the list of absolute Agent home destinations.
// The canonical ~/.agents/skills destination is always first. If target ==
// "all", detected concrete Agent roots follow it. A specific target follows
// canonical as well so unknown/future Agents retain the universal copy.
//
// 末段约定：
//   - mono  → <agent-home>/dws   （单 skill，整个 src 拷成一个 dws 目录）
//   - multi → <agent-home>       （安装时把 src 下每个子目录拷成兄弟 skill）
func resolveSkillSetupTargets(target, mode string) ([]string, error) {
	home, err := skillSetupUserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("无法解析用户 HOME: %w", err)
	}

	target = strings.ToLower(strings.TrimSpace(target))
	canonical := agentHomeForMode(filepath.Join(home, skillSetupAgentHomes[0]), mode)
	if target == "" || target == "all" {
		return detectExistingAgentHomes(home, mode), nil
	}

	if reason, unsupported := unsupportedGlobalAgentTargets[target]; unsupported {
		return nil, errors.New(reason)
	}
	_, ok := agentSkillPaths[target]
	if !ok {
		return nil, fmt.Errorf("不支持的 --target 值: %s（可选 all, %s）", target, supportedTargets())
	}
	dest := agentHomeForMode(resolveSkillSetupBase(home, target), mode)
	if sameSkillSetupPath(dest, canonical) {
		return []string{canonical}, nil
	}
	return []string{canonical, dest}, nil
}

func resolveOpenClawSetupBase(home string) string {
	for _, name := range []string{".openclaw", ".clawdbot", ".moltbot"} {
		base := filepath.Join(home, name)
		if info, err := skillSetupStat(base); err == nil && info.IsDir() {
			return filepath.Join(base, "skills")
		}
	}
	return filepath.Join(home, ".openclaw", "skills")
}

func resolveSkillSetupBase(home, target string) string {
	switch target {
	case "claude", "claude-code":
		if custom := strings.TrimSpace(skillSetupGetenv("CLAUDE_CONFIG_DIR")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "codex":
		if custom := strings.TrimSpace(skillSetupGetenv("CODEX_HOME")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "hermes", "hermes-agent":
		if custom := strings.TrimSpace(skillSetupGetenv("HERMES_HOME")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "autohand-code":
		if custom := strings.TrimSpace(skillSetupGetenv("AUTOHAND_HOME")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "grok":
		if custom := strings.TrimSpace(skillSetupGetenv("GROK_HOME")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "mistral-vibe":
		if custom := strings.TrimSpace(skillSetupGetenv("VIBE_HOME")); custom != "" {
			return filepath.Join(custom, "skills")
		}
	case "openclaw":
		return resolveOpenClawSetupBase(home)
	case "opencode", "amp", "replit", "universal", "crush", "devin", "goose", "kimchi":
		configHome := strings.TrimSpace(skillSetupGetenv("XDG_CONFIG_HOME"))
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		switch target {
		case "opencode":
			return filepath.Join(configHome, "opencode", "skills")
		case "amp", "replit", "universal":
			return filepath.Join(configHome, "agents", "skills")
		case "crush":
			return filepath.Join(configHome, "crush", "skills")
		case "devin":
			return filepath.Join(configHome, "devin", "skills")
		case "goose":
			return filepath.Join(configHome, "goose", "skills")
		case "kimchi":
			return filepath.Join(configHome, "kimchi", "harness", "skills")
		}
	case "github", "github-copilot":
		return filepath.Join(home, ".copilot", "skills")
	case "windsurf":
		return filepath.Join(home, ".codeium", "windsurf", "skills")
	}
	return filepath.Join(home, agentSkillPaths[target])
}

// agentHomeForMode appends the mode-specific tail segment to an agent home base.
func agentHomeForMode(base, mode string) string {
	if mode == skillSetupModeMulti {
		return base
	}
	return filepath.Join(base, "dws")
}

func detectExistingAgentHomes(home, mode string) []string {
	canonical := agentHomeForMode(filepath.Join(home, skillSetupAgentHomes[0]), mode)
	dests := []string{canonical}
	canonicalKey := skillSetupPathKey(canonical)
	seen := map[string]bool{canonicalKey: true}
	addDetected := func(rel, base string) {
		detectedDir := filepath.Dir(base)
		switch filepath.ToSlash(filepath.Clean(rel)) {
		case ".config/kimchi/harness/skills", ".tabnine/agent/skills":
			detectedDir = filepath.Dir(filepath.Dir(base))
		case ".zcode/skills":
			// Application bundles are machine-scoped detection signals. Keep this
			// independent of HOME so setup matches npm, Shell, and PowerShell.
			if info, err := skillSetupStat(filepath.Join(string(filepath.Separator), "Applications", "ZCode.app")); err == nil && info.IsDir() {
				detectedDir = ""
			}
		case ".minimax/skills":
			if info, err := skillSetupStat(filepath.Join(string(filepath.Separator), "Applications", "MiniMax Code.app")); err == nil && info.IsDir() {
				detectedDir = ""
			}
		}
		if detectedDir != "" {
			if info, err := skillSetupStat(detectedDir); err != nil || !info.IsDir() {
				return
			}
		}
		dest := agentHomeForMode(base, mode)
		key := skillSetupPathKey(dest)
		if !seen[key] {
			seen[key] = true
			dests = append(dests, dest)
		}
	}
	for i, rel := range skillSetupAgentHomes {
		if i == 0 {
			continue
		}
		base := filepath.Join(home, rel)
		switch filepath.ToSlash(filepath.Clean(rel)) {
		case ".claude/skills":
			base = resolveSkillSetupBase(home, "claude-code")
		case ".codex/skills":
			base = resolveSkillSetupBase(home, "codex")
		case ".hermes/skills":
			base = resolveSkillSetupBase(home, "hermes-agent")
		case ".autohand/skills":
			base = resolveSkillSetupBase(home, "autohand-code")
		case ".grok/skills":
			base = resolveSkillSetupBase(home, "grok")
		case ".vibe/skills":
			base = resolveSkillSetupBase(home, "mistral-vibe")
		case ".openclaw/skills":
			base = resolveSkillSetupBase(home, "openclaw")
		case ".config/opencode/skills":
			base = resolveSkillSetupBase(home, "opencode")
		case ".config/agents/skills":
			base = resolveSkillSetupBase(home, "amp")
		case ".config/crush/skills":
			base = resolveSkillSetupBase(home, "crush")
		case ".config/devin/skills":
			base = resolveSkillSetupBase(home, "devin")
		case ".config/goose/skills":
			base = resolveSkillSetupBase(home, "goose")
		case ".config/kimchi/harness/skills":
			base = resolveSkillSetupBase(home, "kimchi")
		}
		addDetected(rel, base)
	}
	for _, target := range []string{"github-copilot", "windsurf"} {
		addDetected(agentSkillPaths[target], resolveSkillSetupBase(home, target))
	}
	return dests
}

func skillSetupBaseForMode(dest, mode string) string {
	if mode == skillSetupModeMono {
		return filepath.Dir(dest)
	}
	return dest
}

func isUniversalSkillSetupBase(base string) bool {
	cleanBase := filepath.Clean(base)
	if custom := strings.TrimSpace(skillSetupGetenv("CODEX_HOME")); custom != "" && sameSkillSetupPath(cleanBase, filepath.Join(custom, "skills")) {
		return true
	}
	if custom := strings.TrimSpace(skillSetupGetenv("XDG_CONFIG_HOME")); custom != "" {
		if sameSkillSetupPath(cleanBase, filepath.Join(custom, "agents", "skills")) ||
			sameSkillSetupPath(cleanBase, filepath.Join(custom, "opencode", "skills")) {
			return true
		}
	}
	base = filepath.ToSlash(cleanBase)
	for rel := range map[string]bool{
		".config/agents/skills": true, ".gemini/antigravity/skills": true,
		".gemini/antigravity-cli/skills": true, ".codex/skills": true,
		".cursor/skills": true, ".deepagents/agent/skills": true,
		".firebender/skills": true, ".gemini/skills": true,
		".copilot/skills": true, ".config/opencode/skills": true,
		// beta.6 compatibility roots are cleanup-only.
		".github/skills": true, ".windsurf/skills": true,
		".cline/skills": true, ".amp/skills": true,
	} {
		if strings.HasSuffix(base, "/"+rel) {
			return true
		}
	}
	return false
}

func sameSkillSetupPath(left, right string) bool {
	return skillSetupPathKey(left) == skillSetupPathKey(right)
}

func skillSetupPathKey(path string) string {
	clean := filepath.Clean(path)
	if skillSetupFoldPathCase {
		clean = strings.ToLower(clean)
	}
	return clean
}

func canonicalSkillSetupBase(dests []string, mode string) string {
	for _, dest := range dests {
		base := filepath.ToSlash(filepath.Clean(skillSetupBaseForMode(dest, mode)))
		if strings.HasSuffix(base, "/.agents/skills") {
			return skillSetupBaseForMode(dest, mode)
		}
	}
	return ""
}

func samePhysicalSkillSetupPath(left, right string) bool {
	leftReal, leftErr := skillSetupEvalSymlinks(left)
	rightReal, rightErr := skillSetupEvalSymlinks(right)
	return leftErr == nil && rightErr == nil && sameSkillSetupPath(leftReal, rightReal)
}

func buildSkillSetupPlan(mode, src string, dests, multiSkillNames []string, filtered bool) (*skillSetupPlan, error) {
	if mode != skillSetupModeMono && mode != skillSetupModeMulti {
		return nil, fmt.Errorf("内部错误：未知 mode %q", mode)
	}
	plan := &skillSetupPlan{
		Mode:            mode,
		Source:          src,
		MultiSkillNames: append([]string(nil), multiSkillNames...),
		Filtered:        filtered,
	}
	sort.Strings(plan.MultiSkillNames)
	sortedDests := append([]string(nil), dests...)
	sort.Slice(sortedDests, func(i, j int) bool {
		leftCanonical := strings.HasSuffix(filepath.ToSlash(filepath.Clean(skillSetupBaseForMode(sortedDests[i], mode))), "/.agents/skills")
		rightCanonical := strings.HasSuffix(filepath.ToSlash(filepath.Clean(skillSetupBaseForMode(sortedDests[j], mode))), "/.agents/skills")
		if leftCanonical != rightCanonical {
			return leftCanonical
		}
		return sortedDests[i] < sortedDests[j]
	})
	managedNames := currentManagedSkillNames()
	canonicalBase := canonicalSkillSetupBase(sortedDests, mode)
	for _, dest := range sortedDests {
		base := skillSetupBaseForMode(dest, mode)
		target := skillSetupTargetPlan{Destination: dest, CanonicalBase: canonicalBase}
		if canonicalBase != "" && filepath.Clean(base) != filepath.Clean(canonicalBase) {
			if isUniversalSkillSetupBase(base) {
				target.CleanupOnly = true
			} else {
				target.LinkCanonical = true
			}
		}
		seen := map[string]bool{}
		add := func(path, reason string) {
			if seen[path] {
				return
			}
			seen[path] = true
			target.Backups = append(target.Backups, skillSetupBackup{Path: path, Reason: reason})
		}
		mutual, err := mutualExclusionVictims(dest, mode, managedNames)
		if err != nil {
			return nil, err
		}
		for _, path := range mutual {
			add(path, skillSetupBackupMutual)
		}
		if mode == skillSetupModeMulti && !filtered {
			stale, staleErr := staleMultiSkillVictimsWithError(dest, multiSkillNames, managedNames)
			if staleErr != nil {
				return nil, staleErr
			}
			for _, path := range stale {
				add(path, skillSetupBackupStale)
			}
		}
		if mode == skillSetupModeMulti && filtered && containsSkillName(plan.MultiSkillNames, multiSharedSkill) {
			legacyPath := filepath.Join(dest, legacySharedSkill)
			info, statErr := skillSetupStat(legacyPath)
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return nil, fmt.Errorf("检查已退役 Skill 失败 %s: %w", legacyPath, statErr)
			}
			if statErr == nil && info.IsDir() {
				add(legacyPath, skillSetupBackupStale)
			}
		}
		var replacements []string
		if mode == skillSetupModeMono {
			replacements = []string{dest}
		} else {
			for _, name := range plan.MultiSkillNames {
				replacements = append(replacements, filepath.Join(dest, name))
			}
		}
		for _, path := range replacements {
			if target.LinkCanonical && samePhysicalSkillSetupPath(path, filepath.Join(target.CanonicalBase, filepath.Base(path))) {
				continue
			}
			_, statErr := skillSetupLstat(path)
			if statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("检查将被替换的 Skill 失败 %s: %w", path, statErr)
			}
			add(path, skillSetupBackupReplace)
		}
		sort.Slice(target.Backups, func(i, j int) bool { return target.Backups[i].Path < target.Backups[j].Path })
		if !target.CleanupOnly || len(target.Backups) > 0 {
			plan.Targets = append(plan.Targets, target)
		}
	}
	return plan, nil
}

func currentManagedSkillNames() map[string]bool {
	home, err := skillSetupUserHomeDir()
	if err != nil {
		return map[string]bool{}
	}
	state, readable, err := skillSetupReadState(home)
	if err != nil || !readable {
		return map[string]bool{}
	}
	return skillstate.ManagedSkillNames(state)
}

func configureEventMiscMigrationPlan(plan *skillSetupPlan, targets []string, installsCompanion bool) {
	plan.EventMiscMigrationTargets = append([]string(nil), targets...)
	sort.Strings(plan.EventMiscMigrationTargets)
	plan.InstallsEventMiscCompanion = installsCompanion
	if len(targets) == 0 {
		return
	}
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[target] = true
	}
	for i := range plan.Targets {
		if !targetSet[plan.Targets[i].Destination] {
			continue
		}
		filtered := plan.Targets[i].Backups[:0]
		for _, backup := range plan.Targets[i].Backups {
			base := filepath.Base(backup.Path)
			if base == multiEventSkill || base == multiMiscSkill {
				continue
			}
			filtered = append(filtered, backup)
		}
		plan.Targets[i].Backups = filtered
	}
}

func retireMigratedUniversalSkills(targets, names []string, out io.Writer) error {
	home, err := skillSetupUserHomeDir()
	if err != nil {
		return fmt.Errorf("无法解析 HOME 以退役 universal Agent 旧副本: %w", err)
	}
	seen := map[string]bool{}
	var victims []skillSetupBackup
	for _, target := range targets {
		if !isUniversalSkillSetupBase(target) {
			continue
		}
		for _, name := range names {
			path := filepath.Join(target, name)
			if seen[path] {
				continue
			}
			seen[path] = true
			victims = append(victims, skillSetupBackup{Path: path, Reason: skillSetupBackupReplace})
		}
	}
	sort.Slice(victims, func(i, j int) bool { return victims[i].Path < victims[j].Path })
	if _, err := backupSkillSetupTarget(home, victims, out); err != nil {
		return fmt.Errorf("退役 universal Agent Event/misc 旧副本失败，已回滚: %w", err)
	}
	return nil
}

func renderSkillSetupPlan(out io.Writer, plan *skillSetupPlan) {
	fmt.Fprintf(out, "📦 将安装 skill：\n  mode: %s\n  source: %s\n", plan.Mode, plan.Source)
	if plan.Mode == skillSetupModeMulti {
		fmt.Fprintf(out, "  将装 %d 个独立 skill（按子目录平铺到 <agent-home>/<skill-name>/）：\n", len(plan.MultiSkillNames))
		for _, name := range plan.MultiSkillNames {
			fmt.Fprintf(out, "    · %s\n", name)
		}
	}
	fmt.Fprintln(out, "  安装与适配位置:")
	for _, target := range plan.Targets {
		if target.CleanupOnly {
			fmt.Fprintf(out, "    - %s（移除该 Agent 中的旧版 DWS Skills，改用统一安装位置）\n", target.Destination)
		} else if target.LinkCanonical {
			fmt.Fprintf(out, "    - %s（自动配置此 Agent 使用统一安装位置）\n", target.Destination)
		} else {
			fmt.Fprintf(out, "    - %s（统一安装位置）\n", target.Destination)
		}
	}
	fmt.Fprintln(out, "  将备份并移除（先保存到 ~/.dws/skill-backups/）：")
	count := 0
	for _, target := range plan.Targets {
		for _, backup := range target.Backups {
			switch backup.Reason {
			case skillSetupBackupMutual:
				fmt.Fprintf(out, "    × 将备份并移除对面模式残留 %s\n", backup.Path)
			case skillSetupBackupStale:
				fmt.Fprintf(out, "    × 将备份并移除过期 skill %s\n", backup.Path)
			default:
				fmt.Fprintf(out, "    × 将备份并移除同名 Skill %s\n", backup.Path)
			}
			count++
		}
	}
	if count == 0 {
		fmt.Fprintln(out, "    (无)")
	}
	if len(plan.EventMiscMigrationTargets) > 0 {
		fmt.Fprint(out, "  ")
		printEventMiscMigrationPreview(out, plan.EventMiscMigrationTargets, plan.InstallsEventMiscCompanion)
	}
}

func confirmSkillSetup(out io.Writer, mode, src string, dests []string, multiSkillNames []string, filtered bool) (bool, error) {
	plan, err := buildSkillSetupPlan(mode, src, dests, multiSkillNames, filtered)
	if err != nil {
		return false, err
	}
	return confirmSkillSetupPlan(out, plan)
}

func confirmSkillSetupPlan(out io.Writer, plan *skillSetupPlan) (bool, error) {
	fmt.Fprintln(out)
	renderSkillSetupPlan(out, plan)

	if !skillSetupInteractive() {
		return false, fmt.Errorf("非交互环境无法确认目录迁移；请先用 --dry-run 预览，确认后显式传入 --yes")
	}

	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("确认安装？").
				Affirmative("继续").
				Negative("取消").
				Value(&confirm),
		),
	)
	if err := skillSetupRunForm(form); err != nil {
		return false, fmt.Errorf("确认中止: %w", err)
	}
	return confirm, nil
}

// mutualExclusionVictims returns the paths that should be removed before
// installing into dest under the given mode, to prevent leftover files from
// the opposite mode from co-existing.
//
//   - mono dest is <agent-home>/dws  → multi 残留是统一状态中登记的兄弟目录
//   - multi dest is <agent-home>     → mono 残留是 <agent-home>/dws
//
// A scan failure (e.g. unreadable agent home) is returned as a non-nil error
// so callers can surface a warning instead of silently skipping cleanup.
func mutualExclusionVictims(dest, mode string, managed ...map[string]bool) ([]string, error) {
	switch mode {
	case skillSetupModeMono:
		// dest = <agent-home>/dws → agent-home = parent
		agentHome := filepath.Dir(dest)
		entries, err := skillSetupReadDir(agentHome)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, fmt.Errorf("扫描 multi 残留失败 %s: %w", agentHome, err)
		}
		var victims []string
		for _, e := range entries {
			path := filepath.Join(agentHome, e.Name())
			if e.IsDir() && isManagedDWSMultiSkillDir(path, managed...) {
				victims = append(victims, path)
			}
		}
		sort.Strings(victims)
		return victims, nil
	case skillSetupModeMulti:
		// dest = <agent-home> → mono 残留是 dest/dws
		monoPath := filepath.Join(dest, "dws")
		if _, err := skillSetupStat(monoPath); err == nil {
			return []string{monoPath}, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("检查 mono 残留失败 %s: %w", monoPath, err)
		}
		return nil, nil
	}
	return nil, nil
}

// cleanupMutualExclusion backs up + removes the opposite-mode
// leftovers. Removals are reversible: each victim is moved to
// ~/.dws/skill-backups/<stamp>/ first (skillSetupBackupAndRemove), and a
// victim whose backup fails is left in place with a warning instead of being
// destroyed. A failure is returned so the caller skips the complete Agent
// target and never installs both layouts together.
func cleanupMutualExclusion(dest, mode string, out, errOut io.Writer) error {
	victims, scanErr := mutualExclusionVictims(dest, mode, currentManagedSkillNames())
	if scanErr != nil {
		fmt.Fprintf(errOut, "  ⚠️  互斥清理扫描失败（跳过整个 Agent 目标） %s: %v\n", dest, scanErr)
		return scanErr
	}
	if len(victims) == 0 {
		return nil
	}
	home, homeErr := skillSetupUserHomeDir()
	if homeErr != nil {
		for _, victim := range victims {
			fmt.Fprintf(errOut, "  ⚠️  无法解析 HOME，跳过删除（保留） %s: %v\n", victim, homeErr)
		}
		return homeErr
	}
	for _, victim := range victims {
		backup, err := skillSetupBackupAndRemove(home, victim)
		if err != nil {
			fmt.Fprintf(errOut, "  ⚠️  互斥清理失败（保留原目录，跳过整个 Agent 目标） %s: %v\n", victim, err)
			return err
		}
		fmt.Fprintf(out, "  × 已备份并清理对面模式残留 %s → %s\n", victim, backup)
	}
	return nil
}

func cleanupLegacyMultiSharedSkill(dest string, out, errOut io.Writer) {
	legacyPath := filepath.Join(dest, legacyMultiSharedSkill)
	if _, err := skillSetupStat(legacyPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(errOut, "  ⚠️  无法检查已退役 Skill 残留 %s: %v\n", legacyPath, err)
		}
		return
	}
	if err := skillSetupRemoveAll(legacyPath); err != nil {
		fmt.Fprintf(errOut, "  ⚠️  已退役 Skill 清理失败（已安装 %s） %s: %v\n", multiSharedSkill, legacyPath, err)
		return
	}
	fmt.Fprintf(out, "  × 已清理已退役 Skill 残留 %s\n", legacyPath)
}

func installSkillToHomes(src string, dests []string, out, errOut io.Writer) (installed, skipped int, err error) {
	plan, planErr := buildSkillSetupPlan(skillSetupModeMono, src, dests, nil, false)
	if planErr != nil {
		return 0, len(dests), planErr
	}
	return executeSkillSetupPlan(plan, out, errOut)
}

func installMultiSkillsWithEventMigration(
	src string,
	skillNames []string,
	dests []string,
	migrationTargets []string,
	filtered bool,
	out, errOut io.Writer,
) (installed, skipped int, err error) {
	if len(migrationTargets) == 0 {
		return skillSetupInstallMulti(src, skillNames, dests, out, errOut, filtered)
	}

	migrationSet := make(map[string]struct{}, len(migrationTargets))
	physicalMigrationTargets := make([]string, 0, len(migrationTargets))
	for _, dest := range migrationTargets {
		migrationSet[dest] = struct{}{}
		if !isUniversalSkillSetupBase(dest) {
			physicalMigrationTargets = append(physicalMigrationTargets, dest)
		}
	}
	var ordinaryTargets []string
	for _, dest := range dests {
		if _, migrates := migrationSet[dest]; !migrates {
			ordinaryTargets = append(ordinaryTargets, dest)
		}
	}

	var canonicalTargets, otherOrdinaryTargets []string
	for _, dest := range ordinaryTargets {
		base := filepath.ToSlash(filepath.Clean(skillSetupBaseForMode(dest, skillSetupModeMulti)))
		if strings.HasSuffix(base, "/.agents/skills") {
			canonicalTargets = append(canonicalTargets, dest)
		} else {
			otherOrdinaryTargets = append(otherOrdinaryTargets, dest)
		}
	}
	installOrdinary := func(names, targets []string) error {
		if len(targets) == 0 {
			return nil
		}
		var n, nSkipped int
		n, nSkipped, err = skillSetupInstallMulti(src, names, targets, out, errOut, filtered)
		installed += n
		skipped += nSkipped
		if err != nil {
			return err
		}
		if nSkipped > 0 {
			return fmt.Errorf("multi Skill 安装不完整（skipped=%d）；已保留折叠版 Event/misc，未执行迁移", nSkipped)
		}
		return nil
	}
	canonicalNames := append([]string(nil), skillNames...)
	if !containsSkillName(canonicalNames, multiMiscSkill) {
		canonicalNames = append(canonicalNames, multiMiscSkill)
		sort.Strings(canonicalNames)
	}
	if err := installOrdinary(canonicalNames, canonicalTargets); err != nil {
		return installed, skipped, err
	}
	if err := installOrdinary(skillNames, otherOrdinaryTargets); err != nil {
		return installed, skipped, err
	}

	// The folded pair is excluded from the ordinary best-effort installer. All
	// other selected skills (especially dingtalk-shared) must succeed before the
	// old Event route is touched.
	for _, dest := range physicalMigrationTargets {
		if cleanupErr := cleanupMutualExclusion(dest, skillSetupModeMulti, out, errOut); cleanupErr != nil {
			return installed, skipped + len(skillNames), cleanupErr
		}
	}
	var prerequisiteNames []string
	for _, name := range skillNames {
		if name != multiEventSkill && name != multiMiscSkill {
			prerequisiteNames = append(prerequisiteNames, name)
		}
	}
	if len(prerequisiteNames) > 0 && len(physicalMigrationTargets) > 0 {
		var n, nSkipped int
		n, nSkipped, err = skillSetupInstallMulti(src, prerequisiteNames, physicalMigrationTargets, out, errOut, true)
		installed += n
		skipped += nSkipped
		if err != nil {
			return installed, skipped, err
		}
		if nSkipped > 0 {
			return installed, skipped, fmt.Errorf("event Skill 迁移前置安装不完整（skipped=%d）；已保留折叠版 Event/misc", nSkipped)
		}
	}

	migrated, migrationErr := migrateEventMiscAtomically(src, physicalMigrationTargets, out, errOut)
	installed += migrated
	if migrationErr != nil {
		return installed, skipped, migrationErr
	}
	return installed, skipped, nil
}

type eventMiscMigration struct {
	dest string

	stageRoot   string
	stagedEvent string
	stagedMisc  string
	backupEvent string
	backupMisc  string

	eventPath string
	miscPath  string

	eventBackedUp   bool
	miscBackedUp    bool
	newEventEnabled bool
	newMiscEnabled  bool
}

func prepareEventMiscMigration(src, dest string) (*eventMiscMigration, error) {
	stageRoot, err := skillSetupMkdirTemp(dest, ".dws-event-migration-")
	if err != nil {
		return nil, fmt.Errorf("无法在目标文件系统创建 Event Skill 迁移 staging %s: %w", dest, err)
	}
	migration := &eventMiscMigration{
		dest:        dest,
		stageRoot:   stageRoot,
		stagedEvent: filepath.Join(stageRoot, "new-event"),
		stagedMisc:  filepath.Join(stageRoot, "new-misc"),
		backupEvent: filepath.Join(stageRoot, "old-event"),
		backupMisc:  filepath.Join(stageRoot, "old-misc"),
		eventPath:   filepath.Join(dest, multiEventSkill),
		miscPath:    filepath.Join(dest, multiMiscSkill),
	}
	cleanupOnError := func(cause error) (*eventMiscMigration, error) {
		if cleanupErr := skillSetupRemoveAll(stageRoot); cleanupErr != nil {
			cause = errors.Join(cause, fmt.Errorf("清理 staging %s 失败: %w", stageRoot, cleanupErr))
		}
		return nil, cause
	}

	if err := skillSetupCopyDir(filepath.Join(src, multiEventSkill), migration.stagedEvent); err != nil {
		return cleanupOnError(fmt.Errorf("预备 dingtalk-event 失败 %s: %w", dest, err))
	}
	if err := skillSetupCopyDir(filepath.Join(src, multiMiscSkill), migration.stagedMisc); err != nil {
		return cleanupOnError(fmt.Errorf("预备 dingtalk-misc 失败 %s: %w", dest, err))
	}
	if err := validateEventMigrationSkillRoot(migration.stagedEvent); err != nil {
		return cleanupOnError(fmt.Errorf("迁移 staging 验证失败 %s: %w", migration.stagedEvent, err))
	}
	if err := validateMigrationSkillRoot(migration.stagedMisc, multiMiscSkill, nil); err != nil {
		return cleanupOnError(fmt.Errorf("迁移 staging 验证失败 %s: %w", migration.stagedMisc, err))
	}
	if err := validateCleanEventMiscRoot(migration.stagedMisc); err != nil {
		return cleanupOnError(fmt.Errorf("迁移 staging 验证失败 %s: %w", migration.stagedMisc, err))
	}
	return migration, nil
}

func migrateEventMiscAtomically(src string, dests []string, out, errOut io.Writer) (int, error) {
	sortedDests := append([]string(nil), dests...)
	sort.Strings(sortedDests)
	migrations := make([]*eventMiscMigration, 0, len(sortedDests))

	// Stage every target before switching any target. This prevents a source or
	// copy failure on a later Agent home from leaving earlier homes upgraded.
	for _, dest := range sortedDests {
		migration, err := prepareEventMiscMigration(src, dest)
		if err != nil {
			if cleanupErr := cleanupEventMiscStages(migrations, false, errOut); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
			return 0, err
		}
		migrations = append(migrations, migration)
	}

	committed := make([]*eventMiscMigration, 0, len(migrations))
	for _, migration := range migrations {
		if err := commitEventMiscMigration(migration); err != nil {
			rollbackErr := rollbackEventMiscMigrations(committed)
			if rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("已切换目标回滚失败: %w", rollbackErr))
			}
			var recoveryRoots []string
			for _, candidate := range migrations {
				if eventMiscMigrationNeedsRecovery(candidate) {
					recoveryRoots = append(recoveryRoots, candidate.stageRoot)
				}
			}
			if len(recoveryRoots) > 0 {
				err = errors.Join(err, fmt.Errorf("回滚不完整，已保留恢复目录（请勿删除）: %s", strings.Join(recoveryRoots, ", ")))
			}
			if cleanupErr := cleanupEventMiscStages(migrations, true, errOut); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
			return 0, err
		}
		committed = append(committed, migration)
	}

	for _, migration := range migrations {
		fmt.Fprintf(out, "  ✓ %s\n", migration.eventPath)
		fmt.Fprintf(out, "  ✓ %s（Event 原子迁移）\n", migration.miscPath)
	}
	if cleanupErr := cleanupEventMiscStages(migrations, false, errOut); cleanupErr != nil {
		fmt.Fprintf(errOut, "  ⚠️  Event Skill 迁移已完成，但 staging 清理不完整: %v\n", cleanupErr)
	}
	return len(migrations) * 2, nil
}

func cleanupEventMiscStages(migrations []*eventMiscMigration, preserveRecovery bool, errOut io.Writer) error {
	var cleanupErr error
	for _, migration := range migrations {
		if preserveRecovery && eventMiscMigrationNeedsRecovery(migration) {
			fmt.Fprintf(errOut, "  ⚠️  已保留 Event Skill 恢复目录 %s\n", migration.stageRoot)
			continue
		}
		if err := skillSetupRemoveAll(migration.stageRoot); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("清理 Event Skill staging %s 失败: %w", migration.stageRoot, err))
		}
	}
	return cleanupErr
}

func eventMiscMigrationNeedsRecovery(migration *eventMiscMigration) bool {
	return migration.eventBackedUp || migration.miscBackedUp || migration.newEventEnabled || migration.newMiscEnabled
}

func commitEventMiscMigration(migration *eventMiscMigration) error {
	eventExists, err := skillSetupPathExists(migration.eventPath)
	if err != nil {
		return fmt.Errorf("无法检查旧 dingtalk-event %s: %w", migration.dest, err)
	}
	miscExists, err := skillSetupPathExists(migration.miscPath)
	if err != nil {
		return fmt.Errorf("无法检查旧 dingtalk-misc %s: %w", migration.dest, err)
	}
	if !miscExists {
		return fmt.Errorf("event Skill 迁移中止：折叠版 dingtalk-misc 已不存在 %s", migration.dest)
	}

	rollbackFailure := func(cause error) error {
		if rollbackErr := rollbackEventMiscMigration(migration); rollbackErr != nil {
			return errors.Join(cause, fmt.Errorf("回滚 Event/misc 失败 %s: %w", migration.dest, rollbackErr))
		}
		return cause
	}
	if eventExists {
		if err := skillSetupRename(migration.eventPath, migration.backupEvent); err != nil {
			return fmt.Errorf("备份旧 dingtalk-event 失败 %s: %w", migration.dest, err)
		}
		migration.eventBackedUp = true
	}
	if err := skillSetupRename(migration.stagedEvent, migration.eventPath); err != nil {
		return rollbackFailure(fmt.Errorf("切换 dingtalk-event 失败 %s: %w", migration.dest, err))
	}
	migration.newEventEnabled = true
	if err := skillSetupRename(migration.miscPath, migration.backupMisc); err != nil {
		return rollbackFailure(fmt.Errorf("备份旧 dingtalk-misc 失败 %s: %w", migration.dest, err))
	}
	migration.miscBackedUp = true
	if err := skillSetupRename(migration.stagedMisc, migration.miscPath); err != nil {
		return rollbackFailure(fmt.Errorf("切换 dingtalk-misc 失败 %s: %w", migration.dest, err))
	}
	migration.newMiscEnabled = true
	return nil
}

func rollbackEventMiscMigrations(migrations []*eventMiscMigration) error {
	var rollbackErr error
	for i := len(migrations) - 1; i >= 0; i-- {
		if err := rollbackEventMiscMigration(migrations[i]); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func rollbackEventMiscMigration(migration *eventMiscMigration) error {
	move := func(enabled *bool, from, to, label string) error {
		if !*enabled {
			return nil
		}
		if err := skillSetupRename(from, to); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		*enabled = false
		return nil
	}

	// Stop at the first rollback failure. In particular, do not remove the
	// already-working standalone Event while the folded misc route has not been
	// restored: even an incomplete rollback must leave at least one Event entry
	// point live and preserve the remaining assets in staging for recovery.
	steps := []struct {
		enabled *bool
		from    string
		to      string
		label   string
	}{
		{&migration.newMiscEnabled, migration.miscPath, migration.stagedMisc, "移出新 dingtalk-misc"},
		{&migration.miscBackedUp, migration.backupMisc, migration.miscPath, "恢复旧 dingtalk-misc"},
		{&migration.newEventEnabled, migration.eventPath, migration.stagedEvent, "移出新 dingtalk-event"},
		{&migration.eventBackedUp, migration.backupEvent, migration.eventPath, "恢复旧 dingtalk-event"},
	}
	for _, step := range steps {
		if err := move(step.enabled, step.from, step.to, step.label); err != nil {
			return err
		}
	}
	return nil
}

func skillSetupPathExists(path string) (bool, error) {
	_, err := skillSetupStat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// installMultiSkillToHomes installs each subdir of src (dingtalk-*) into
// dest as a sibling skill directory. installed/skipped is counted per
// (agent-home × sub-skill) pair so the user sees granular progress.
//
// filtered mirrors whether runSkillSetup saw -s/--skill or -x/--exclude:
// a filtered install stays additive and never touches siblings outside the
// requested set; a full (unfiltered) install additionally removes stale,
// proven DWS-managed directories that are no longer in the bundle,
// matching install.sh / install.ps1 / install.js / upgrade paths.
func installMultiSkillToHomes(src string, skillNames []string, dests []string, out, errOut io.Writer, filtered bool) (installed, skipped int, err error) {
	plan, planErr := buildSkillSetupPlan(skillSetupModeMulti, src, dests, skillNames, filtered)
	if planErr != nil {
		return 0, len(skillNames) * len(dests), planErr
	}
	return executeSkillSetupPlan(plan, out, errOut)
}

// stageSkillSetupTarget builds the complete replacement set next to its final
// destination before any Agent-visible directory is moved.
func stageSkillSetupTarget(plan *skillSetupPlan, target skillSetupTargetPlan) (stageRoot string, staged []skillSetupStagedDir, err error) {
	stageParent := target.Destination
	if plan.Mode == skillSetupModeMono {
		stageParent = filepath.Dir(target.Destination)
	}
	if err := skillSetupMkdirAll(stageParent, 0o755); err != nil {
		return "", nil, fmt.Errorf("创建 Skill 目标父目录失败 %s: %w", stageParent, err)
	}
	stageRoot, err = skillSetupPublishTemp(stageParent, ".dws-setup-set-")
	if err != nil {
		return "", nil, fmt.Errorf("创建 Skill staging 失败 %s: %w", stageParent, err)
	}
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := skillSetupRemoveAll(stageRoot); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理 Skill staging 失败 %s: %w", stageRoot, cleanupErr))
		}
	}()
	realStageParent, realParentErr := skillSetupEvalSymlinks(stageParent)
	if realParentErr != nil {
		return stageRoot, nil, fmt.Errorf("解析 Agent Skill 物理目录失败 %s: %w", stageParent, realParentErr)
	}

	stageOne := func(src, dest string) error {
		stagedDir := filepath.Join(stageRoot, filepath.Base(dest))
		if target.LinkCanonical {
			canonicalTarget := filepath.Join(target.CanonicalBase, filepath.Base(dest))
			if samePhysicalSkillSetupPath(dest, canonicalTarget) {
				return nil
			}
			realCanonicalTarget, realTargetErr := skillSetupEvalSymlinks(canonicalTarget)
			if realTargetErr != nil {
				return fmt.Errorf("解析 canonical Skill 失败 %s: %w", canonicalTarget, realTargetErr)
			}
			relTarget, relErr := skillSetupRel(realStageParent, realCanonicalTarget)
			if relErr != nil {
				return fmt.Errorf("计算 Skill 相对链接失败 %s: %w", canonicalTarget, relErr)
			}
			if linkErr := skillSetupSymlink(relTarget, stagedDir); linkErr != nil {
				return fmt.Errorf("创建 Skill 链接失败 %s -> %s: %w", stagedDir, relTarget, linkErr)
			}
			staged = append(staged, skillSetupStagedDir{staged: stagedDir, dest: dest})
			return nil
		}
		if err := skillSetupMkdirAll(stagedDir, 0o755); err != nil {
			return fmt.Errorf("创建 Skill staging 目录失败 %s: %w", stagedDir, err)
		}
		if err := skillSetupCopyDir(src, stagedDir); err != nil {
			return fmt.Errorf("拷贝 Skill staging 失败 %s: %w", stagedDir, err)
		}
		staged = append(staged, skillSetupStagedDir{staged: stagedDir, dest: dest})
		return nil
	}

	if plan.Mode == skillSetupModeMono {
		if err := stageOne(plan.Source, target.Destination); err != nil {
			return stageRoot, nil, err
		}
		return stageRoot, staged, nil
	}
	for _, name := range plan.MultiSkillNames {
		if err := stageOne(filepath.Join(plan.Source, name), filepath.Join(target.Destination, name)); err != nil {
			return stageRoot, nil, err
		}
	}
	return stageRoot, staged, nil
}

// restoreSkillSetupTarget removes a partially published replacement and
// restores every original directory from its exact backup path.
func restoreSkillSetupTarget(published []upgrade.SkillPathPublication, backups []skillSetupBackedUpDir) error {
	restoreErr := skillSetupRollbackPaths(published)
	for i := len(backups) - 1; i >= 0; i-- {
		item := backups[i]
		if _, err := skillSetupLstat(item.original); err == nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复目标仍存在 %s；备份保留于 %s", item.original, item.backup))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("检查 Skill 恢复目标失败 %s: %w；备份保留于 %s", item.original, err, item.backup))
			continue
		}
		if err := skillSetupMkdirAll(filepath.Dir(item.original), 0o755); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("创建 Skill 恢复目录失败 %s: %w；备份保留于 %s", filepath.Dir(item.original), err, item.backup))
			continue
		}
		if err := skillSetupRestoreBackup(item.backup, item.original); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复原 Skill 失败 %s: %w；备份保留于 %s", item.original, err, item.backup))
		}
	}
	return restoreErr
}

func backupSkillSetupTarget(home string, planned []skillSetupBackup, out io.Writer) ([]skillSetupBackedUpDir, error) {
	backups := make([]skillSetupBackedUpDir, 0, len(planned))
	for _, item := range planned {
		backup, err := skillSetupBackupAndRemove(home, item.Path)
		if err != nil {
			if restoreErr := restoreSkillSetupTarget(nil, backups); restoreErr != nil {
				return nil, errors.Join(err, fmt.Errorf("恢复已备份 Skill 失败: %w", restoreErr))
			}
			return nil, err
		}
		if backup != "" {
			backups = append(backups, skillSetupBackedUpDir{original: item.Path, backup: backup})
		}
		switch item.Reason {
		case skillSetupBackupMutual:
			fmt.Fprintf(out, "  × 已备份并清理对面模式残留 %s → %s\n", item.Path, backup)
		case skillSetupBackupStale:
			fmt.Fprintf(out, "  × 已备份并清理过期 skill %s → %s\n", item.Path, backup)
		default:
			fmt.Fprintf(out, "  × 已备份并移除同名 Skill %s → %s\n", item.Path, backup)
		}
	}
	return backups, nil
}

func publishSkillSetupTarget(staged []skillSetupStagedDir, backups []skillSetupBackedUpDir) error {
	published := make([]upgrade.SkillPathPublication, 0, len(staged))
	for _, item := range staged {
		publication, err := skillSetupPublishPath(item.staged, item.dest)
		if err != nil {
			publishErr := fmt.Errorf("发布 Skill 失败 %s: %w", item.dest, err)
			if restoreErr := restoreSkillSetupTarget(published, backups); restoreErr != nil {
				return errors.Join(publishErr, fmt.Errorf("Skill setup 回滚不完整: %w", restoreErr))
			}
			return publishErr
		}
		published = append(published, publication)
	}
	return nil
}

func executeSkillSetupPlan(plan *skillSetupPlan, out, errOut io.Writer) (installed, skipped int, err error) {
	home, homeErr := skillSetupUserHomeDir()
	hasCanonicalDependents := false
	for _, candidate := range plan.Targets {
		if candidate.CanonicalBase != "" && !sameSkillSetupPath(skillSetupBaseForMode(candidate.Destination, plan.Mode), candidate.CanonicalBase) {
			hasCanonicalDependents = true
			break
		}
	}
	for _, target := range plan.Targets {
		perTarget := 1
		if plan.Mode == skillSetupModeMulti {
			perTarget = len(plan.MultiSkillNames)
		}
		isCanonical := target.CanonicalBase != "" && sameSkillSetupPath(skillSetupBaseForMode(target.Destination, plan.Mode), target.CanonicalBase)
		if target.CleanupOnly {
			// Nothing is installed below a universal root, so a stale copy that
			// resists retirement must not count as a skipped install: any skipped
			// count fails the whole setup, even when every real target succeeded.
			if homeErr != nil {
				fmt.Fprintf(errOut, "  ⚠️  无法解析 HOME，保留 universal Agent 旧副本 %s: %v\n", target.Destination, homeErr)
				continue
			}
			if _, cleanupErr := backupSkillSetupTarget(home, target.Backups, out); cleanupErr != nil {
				fmt.Fprintf(errOut, "  ⚠️  universal Agent 旧副本迁移失败，已回滚，可手动删除 %s: %v\n", target.Destination, cleanupErr)
			}
			continue
		}
		if len(target.Backups) > 0 && homeErr != nil {
			if isCanonical && hasCanonicalDependents {
				return installed, skipped + perTarget, fmt.Errorf("无法解析 HOME，canonical Skill 刷新中止 %s: %w", target.Destination, homeErr)
			}
			if plan.Mode == skillSetupModeMono {
				fmt.Fprintf(errOut, "  ✗ 无法解析 HOME，跳过刷新（保留原目录） %s: %v\n", target.Destination, homeErr)
			} else {
				fmt.Fprintf(errOut, "  ✗ 无法解析 HOME，跳过整个 Agent 目标 %s: %v\n", target.Destination, homeErr)
			}
			skipped += perTarget
			continue
		}

		stageRoot, staged, stageErr := stageSkillSetupTarget(plan, target)
		if stageErr != nil && target.LinkCanonical {
			fmt.Fprintf(errOut, "  ℹ️  %s 无法使用共享安装方式，正在自动改用兼容安装\n", target.Destination)
			fallback := target
			fallback.LinkCanonical = false
			stageRoot, staged, stageErr = stageSkillSetupTarget(plan, fallback)
		}
		if stageErr != nil {
			if isCanonical && hasCanonicalDependents {
				return installed, skipped + perTarget, fmt.Errorf("canonical Skill staging 失败 %s: %w", target.Destination, stageErr)
			}
			fmt.Fprintf(errOut, "  ✗ Skill staging 失败，保留原集合 %s: %v\n", target.Destination, stageErr)
			skipped += perTarget
			continue
		}
		backups, backupErr := backupSkillSetupTarget(home, target.Backups, out)
		if backupErr != nil {
			if cleanupErr := skillSetupRemoveAll(stageRoot); cleanupErr != nil {
				backupErr = errors.Join(backupErr, fmt.Errorf("清理 Skill staging 失败 %s: %w", stageRoot, cleanupErr))
			}
			if isCanonical && hasCanonicalDependents {
				return installed, skipped + perTarget, fmt.Errorf("canonical Skill 备份失败，已执行回滚 %s: %w", target.Destination, backupErr)
			}
			fmt.Fprintf(errOut, "  ✗ Skill 备份失败，已执行回滚，跳过整个 Agent 目标 %s: %v\n", target.Destination, backupErr)
			skipped += perTarget
			continue
		}
		publishErr := publishSkillSetupTarget(staged, backups)
		cleanupErr := skillSetupRemoveAll(stageRoot)
		if publishErr != nil {
			if cleanupErr != nil {
				publishErr = errors.Join(publishErr, fmt.Errorf("清理 Skill staging 失败 %s: %w", stageRoot, cleanupErr))
			}
			if isCanonical && hasCanonicalDependents {
				if errors.Is(publishErr, upgrade.ErrSkillPathPublicationUncertain) {
					return installed, skipped + perTarget, fmt.Errorf("canonical Skill 发布状态不确定，目标可能属于并发写入并已保留 %s: %w", target.Destination, publishErr)
				}
				return installed, skipped + perTarget, fmt.Errorf("canonical Skill 发布失败，已执行回滚 %s: %w", target.Destination, publishErr)
			}
			if errors.Is(publishErr, upgrade.ErrSkillPathPublicationUncertain) {
				// The destination may belong to a concurrent writer and was
				// deliberately retained; the rollback refuses to displace it.
				fmt.Fprintf(errOut, "  ✗ Skill 发布状态不确定，目标可能属于并发写入并已保留 %s: %v\n", target.Destination, publishErr)
			} else {
				fmt.Fprintf(errOut, "  ✗ Skill 发布失败，已执行回滚，跳过整个 Agent 目标 %s: %v\n", target.Destination, publishErr)
			}
			skipped += perTarget
			continue
		}
		if cleanupErr != nil {
			fmt.Fprintf(errOut, "  ⚠️  Skill staging 清理失败 %s: %v\n", stageRoot, cleanupErr)
		}
		for _, item := range staged {
			fmt.Fprintf(out, "  ✓ %s\n", item.dest)
		}
		installed += perTarget
	}
	return installed, skipped, nil
}

// staleMultiSkillVictims lists proven DWS-managed directories under dest that
// a full install would delete because they are not part of the bundle.
func staleMultiSkillVictims(dest string, keep []string, managed ...map[string]bool) []string {
	if len(managed) == 0 {
		managed = []map[string]bool{currentManagedSkillNames()}
	}
	victims, _ := staleMultiSkillVictimsWithError(dest, keep, managed...)
	return victims
}

func staleMultiSkillVictimsWithError(dest string, keep []string, managed ...map[string]bool) ([]string, error) {
	entries, err := skillSetupReadDir(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("扫描过期 Skill 失败 %s: %w", dest, err)
	}
	keepSet := make(map[string]bool, len(keep))
	for _, n := range keep {
		keepSet[n] = true
	}
	var victims []string
	for _, e := range entries {
		if (!e.IsDir() && e.Type()&os.ModeSymlink == 0) || keepSet[e.Name()] {
			continue
		}
		if !isManagedDWSMultiSkillDir(filepath.Join(dest, e.Name()), managed...) {
			continue
		}
		victims = append(victims, filepath.Join(dest, e.Name()))
	}
	sort.Strings(victims)
	return victims, nil
}

// removeStaleMultiSkills backs up + removes proven DWS-managed directories
// under dest that are not part of the current bundle. Removal is
// reversible: each stale directory is moved to ~/.dws/skill-backups/<stamp>/
// first, and a backup failure keeps the directory in place with a warning.
// A scan or backup failure is returned so callers do not write a new bundle
// into a partially reconciled Agent target.
func removeStaleMultiSkills(dest string, keep []string, out, errOut io.Writer) error {
	managedNames := currentManagedSkillNames()
	entries, err := skillSetupReadDir(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		fmt.Fprintf(errOut, "  ⚠️  过期 skill 扫描失败（跳过整个 Agent 目标） %s: %v\n", dest, err)
		return err
	}
	keepSet := make(map[string]bool, len(keep))
	for _, n := range keep {
		keepSet[n] = true
	}
	var stales []string
	for _, e := range entries {
		if !e.IsDir() || keepSet[e.Name()] {
			continue
		}
		if !isManagedDWSMultiSkillDir(filepath.Join(dest, e.Name()), managedNames) {
			continue
		}
		stales = append(stales, filepath.Join(dest, e.Name()))
	}
	if len(stales) == 0 {
		return nil
	}
	home, homeErr := skillSetupUserHomeDir()
	if homeErr != nil {
		for _, stale := range stales {
			fmt.Fprintf(errOut, "  ⚠️  无法解析 HOME，跳过删除（保留） %s: %v\n", stale, homeErr)
		}
		return homeErr
	}
	for _, stale := range stales {
		backup, err := skillSetupBackupAndRemove(home, stale)
		if err != nil {
			fmt.Fprintf(errOut, "  ⚠️  过期 skill 清理失败（保留原目录，跳过整个 Agent 目标） %s: %v\n", stale, err)
			return err
		}
		fmt.Fprintf(out, "  × 已备份并清理过期 skill %s → %s\n", stale, backup)
	}
	return nil
}

func copyDir(src, dst string) error {
	return skillSetupWalk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := skillSetupRel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return skillSetupMkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// resolve symlink target and copy the underlying file
			resolved, err := skillSetupReadlink(path)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			return copyFileContent(resolved, target, info.Mode())
		}
		return copyFileContent(path, target, info.Mode())
	})
}

func copyFileContent(src, dst string, mode os.FileMode) error {
	if err := skillSetupMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := skillSetupOpen(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := skillSetupOpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode&os.ModePerm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = skillSetupCopy(out, in)
	return err
}

func isInteractiveTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout) && isCharDevice(os.Stderr)
}

func isCharDevice(file *os.File) bool {
	if file == nil {
		return false
	}
	fi, err := file.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
