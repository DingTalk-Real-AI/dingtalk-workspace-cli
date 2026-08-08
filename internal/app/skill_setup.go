package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// skillSetupAgentHomes is the ordered list of agent home subdirectories
// where dws skills get installed. Mirrors install.sh / install.ps1 /
// build/npm/install.js so that `dws skill setup` and the install scripts
// agree on the install footprint.
var skillSetupAgentHomes = []string{
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

const (
	skillSetupModeMono  = "mono"
	skillSetupModeMulti = "multi"
)

var (
	skillSetupResolveMode    = resolveSkillSetupMode
	skillSetupResolveSource  = resolveSkillSetupSourceOrEmbedded
	skillSetupResolveTargets = resolveSkillSetupTargets
	skillSetupListMulti      = listMultiSkillNames
	skillSetupFilterMulti    = filterMultiSkillNames
	skillSetupConfirm        = confirmSkillSetup
	skillSetupInstallMono    = installSkillToHomes
	skillSetupInstallMulti   = installMultiSkillToHomes
	skillSetupCopyDir        = copyDir
	skillSetupMkdirTemp      = os.MkdirTemp
	skillSetupRename         = os.Rename
	skillSetupRunForm        = (*huh.Form).Run
	skillSetupInteractive    = isInteractiveTerminal
	skillSetupReadDir        = os.ReadDir
	skillSetupStat           = os.Stat
	skillSetupExecutable     = os.Executable
	skillSetupGetwd          = os.Getwd
	skillSetupUserHomeDir    = os.UserHomeDir
	skillSetupRemoveAll      = os.RemoveAll
	skillSetupMkdirAll       = os.MkdirAll
	skillSetupWalk           = filepath.Walk
	skillSetupRel            = filepath.Rel
	skillSetupReadlink       = os.Readlink
	skillSetupOpen           = os.Open
	skillSetupOpenFile       = os.OpenFile
	skillSetupCopy           = io.Copy
)

func newSkillSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "安装 dws 自身 skill 到 Agent 目录",
		Long: `安装 dws 自身 skill 文档到 AI Agent 目录（如 ~/.claude/skills/、~/.cursor/skills/ 等）。

支持两种模式：
  mono   单 skill（稳定 / 推荐）—— 总入口 SKILL.md + references/products/
  multi  多 skill—— 按产品拆 N 个独立 skill

multi 模式支持按产品挑选：
  -s/--skill   只装指定子 skill（可重复，短名 aitable 或全名 dingtalk-aitable 均可）
  -x/--exclude 从全装里剔除指定子 skill（可重复，与 --skill 互斥）
  未列出的已有 dingtalk-* skill 会保留（additive 叠加语义）

不带 --mode 时进入交互式询问；不带 --target 时铺到所有检测到的 Agent 目录。
skill 源默认取二进制内嵌的版本（升级二进制即升级 skill）；--source / DWS_SKILL_SOURCE 可显式覆盖。`,
		Example: `  dws skill setup                                       # 交互式
  dws skill setup --mode mono --yes                     # 非交互装 mono
  dws skill setup --mode multi --target claude          # multi 全装到 ~/.claude/skills/
  dws skill setup --mode multi -s aitable -s calendar   # 只装 aitable + calendar
  dws skill setup --mode multi -x live -x devdoc        # 安装除 live、devdoc 外的其余 skill
  dws skill setup --source /path/to/repo                # 显式指定 skill 源`,
		DisableAutoGenTag: true,
		RunE:              runSkillSetup,
	}
	cmd.Flags().String("mode", "", "skill 模式：mono | multi（不指定则交互询问）")
	cmd.Flags().String("target", "all", "目标 Agent：all | "+supportedTargets())
	cmd.Flags().String("source", "", "skill 源目录（默认使用二进制内嵌的 skill 源，与当前版本一致）")
	cmd.Flags().Bool("yes", false, "跳过所有确认提示")
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
	var multiSkillNames []string
	var foldedEventMiscTargets []string
	var migrateEventMiscTargets []string
	var installsEventMiscCompanion bool
	if mode == skillSetupModeMulti {
		allMultiSkillNames, listErr := skillSetupListMulti(skillSrc)
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

	// --dry-run：仅预览将安装的内容与目标目录，不写入任何文件、不弹确认。
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		fmt.Fprintf(out, "[DRY-RUN] 预览（不写入任何文件）：mode=%s，来源 %s\n", mode, skillSrc)
		fmt.Fprintln(out, "将安装到：")
		for _, d := range dests {
			fmt.Fprintf(out, "  - %s\n", d)
		}
		if mode == skillSetupModeMulti && len(multiSkillNames) > 0 {
			fmt.Fprintf(out, "子 skill：%s\n", strings.Join(multiSkillNames, ", "))
			printEventMiscMigrationPreview(out, migrateEventMiscTargets, installsEventMiscCompanion)
		}
		return nil
	}

	if !autoYes {
		if mode == skillSetupModeMulti {
			printEventMiscMigrationPreview(out, migrateEventMiscTargets, installsEventMiscCompanion)
		}
		ok, err := skillSetupConfirm(out, mode, skillSrc, dests, multiSkillNames)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "已取消。")
			return nil
		}
	}

	var installed, skipped int
	switch mode {
	case skillSetupModeMono:
		installed, skipped, err = skillSetupInstallMono(skillSrc, dests, out, errOut)
	case skillSetupModeMulti:
		installed, skipped, err = installMultiSkillsWithEventMigration(
			skillSrc,
			multiSkillNames,
			dests,
			migrateEventMiscTargets,
			out,
			errOut,
		)
	default:
		return fmt.Errorf("内部错误：未知 mode %q", mode)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\n✅ Skill 安装完成（mode=%s, installed=%d, skipped=%d）\n", mode, installed, skipped)
	fmt.Fprintln(out, "ℹ️  若 Agent 会话已打开，请重启 Agent 或重新加载 Skills 后再验证路由。")
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
		fmt.Fprintf(out, "  - %s\n", target)
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
// The caller is responsible for additive installation: install only the
// returned names, leaving any other already-installed dingtalk-* siblings
// untouched (handled by installMultiSkillToHomes which does not enumerate
// the destination).
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
		fmt.Fprintln(out, "未指定 --mode，非交互环境下默认使用 mono")
		return skillSetupModeMono, nil
	}

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 dws skill 安装模式").
				Description("mono = 单 skill 入口（稳定 / 推荐）\nmulti = 按产品拆分的独立 skill").
				Options(
					huh.NewOption("mono — 单 skill（稳定 / 推荐）", skillSetupModeMono),
					huh.NewOption("multi — 多 skill（按产品拆分）", skillSetupModeMulti),
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
// given mode. PR 1 supports only mono; multi is reserved for a later PR
// and currently returns an error before reaching this function.
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
// If target == "all", returns every agent home whose parent directory exists.
// Otherwise returns the single matching home (whether or not it currently exists).
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
	if target == "" || target == "all" {
		return detectExistingAgentHomes(home, mode), nil
	}

	rel, ok := agentSkillPaths[target]
	if !ok {
		return nil, fmt.Errorf("不支持的 --target 值: %s（可选 all, %s）", target, supportedTargets())
	}
	return []string{agentHomeForMode(filepath.Join(home, rel), mode)}, nil
}

// agentHomeForMode appends the mode-specific tail segment to an agent home base.
func agentHomeForMode(base, mode string) string {
	if mode == skillSetupModeMulti {
		return base
	}
	return filepath.Join(base, "dws")
}

func detectExistingAgentHomes(home, mode string) []string {
	var out []string
	for i, rel := range skillSetupAgentHomes {
		base := filepath.Join(home, rel)
		parent := filepath.Dir(base)
		if i > 0 {
			if _, err := skillSetupStat(parent); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		out = append(out, agentHomeForMode(base, mode))
	}
	return out
}

func confirmSkillSetup(out io.Writer, mode, src string, dests []string, multiSkillNames []string) (bool, error) {
	fmt.Fprintf(out, "\n📦 将安装 skill：\n  mode: %s\n  source: %s\n", mode, src)
	if mode == skillSetupModeMulti {
		fmt.Fprintf(out, "  将装 %d 个独立 skill（按子目录平铺到 <agent-home>/<skill-name>/）：\n", len(multiSkillNames))
		for _, n := range multiSkillNames {
			fmt.Fprintf(out, "    · %s\n", n)
		}
	}
	fmt.Fprintln(out, "  destinations:")
	for _, d := range dests {
		fmt.Fprintf(out, "    - %s\n", d)
	}
	// 列出互斥清理：装 mode 前要把对面 mode 的残留删掉
	fmt.Fprintln(out, "  互斥清理（确认后才执行）：")
	for _, d := range dests {
		for _, victim := range mutualExclusionVictims(d, mode) {
			fmt.Fprintf(out, "    × 将删除 %s\n", victim)
		}
	}

	if !skillSetupInteractive() {
		return true, nil
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
//   - mono dest is <agent-home>/dws  → multi 残留是 <agent-home>/dingtalk-*
//   - multi dest is <agent-home>     → mono 残留是 <agent-home>/dws
func mutualExclusionVictims(dest, mode string) []string {
	switch mode {
	case skillSetupModeMono:
		// dest = <agent-home>/dws → agent-home = parent
		agentHome := filepath.Dir(dest)
		entries, err := skillSetupReadDir(agentHome)
		if err != nil {
			return nil
		}
		var victims []string
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "dingtalk-") {
				victims = append(victims, filepath.Join(agentHome, e.Name()))
			}
		}
		sort.Strings(victims)
		return victims
	case skillSetupModeMulti:
		// dest = <agent-home> → mono 残留是 dest/dws
		monoPath := filepath.Join(dest, "dws")
		if _, err := skillSetupStat(monoPath); err == nil {
			return []string{monoPath}
		}
		return nil
	}
	return nil
}

// cleanupMutualExclusion best-effort removes the opposite-mode leftovers.
// Failures emit a warning to errOut but never abort the install.
func cleanupMutualExclusion(dest, mode string, out, errOut io.Writer) {
	for _, victim := range mutualExclusionVictims(dest, mode) {
		if err := skillSetupRemoveAll(victim); err != nil {
			fmt.Fprintf(errOut, "  ⚠️  互斥清理失败（继续安装） %s: %v\n", victim, err)
			continue
		}
		fmt.Fprintf(out, "  × 已清理对面模式残留 %s\n", victim)
	}
}

func installSkillToHomes(src string, dests []string, out, errOut io.Writer) (installed, skipped int, err error) {
	sort.Strings(dests)
	for _, dest := range dests {
		// 先做互斥清理：装 mono 前先把同级 dingtalk-* 子目录全部干掉
		cleanupMutualExclusion(dest, skillSetupModeMono, out, errOut)

		if err := skillSetupRemoveAll(dest); err != nil {
			fmt.Fprintf(errOut, "  ✗ 清理失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		if err := skillSetupMkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintf(errOut, "  ✗ 父目录创建失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		if err := skillSetupCopyDir(src, dest); err != nil {
			fmt.Fprintf(errOut, "  ✗ 拷贝失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		fmt.Fprintf(out, "  ✓ %s\n", dest)
		installed++
	}
	return installed, skipped, nil
}

func installMultiSkillsWithEventMigration(
	src string,
	skillNames []string,
	dests []string,
	migrationTargets []string,
	out, errOut io.Writer,
) (installed, skipped int, err error) {
	if len(migrationTargets) == 0 {
		return skillSetupInstallMulti(src, skillNames, dests, out, errOut)
	}

	migrationSet := make(map[string]struct{}, len(migrationTargets))
	for _, dest := range migrationTargets {
		migrationSet[dest] = struct{}{}
	}
	var ordinaryTargets []string
	for _, dest := range dests {
		if _, migrates := migrationSet[dest]; !migrates {
			ordinaryTargets = append(ordinaryTargets, dest)
		}
	}

	if len(ordinaryTargets) > 0 {
		var n, nSkipped int
		n, nSkipped, err = skillSetupInstallMulti(src, skillNames, ordinaryTargets, out, errOut)
		installed += n
		skipped += nSkipped
		if err != nil {
			return installed, skipped, err
		}
		if nSkipped > 0 {
			return installed, skipped, fmt.Errorf("multi Skill 安装不完整（skipped=%d）；已保留折叠版 Event/misc，未执行迁移", nSkipped)
		}
	}

	// The folded pair is excluded from the ordinary best-effort installer. All
	// other selected skills (especially dingtalk-shared) must succeed before the
	// old Event route is touched.
	for _, dest := range migrationTargets {
		cleanupMutualExclusion(dest, skillSetupModeMulti, out, errOut)
	}
	var prerequisiteNames []string
	for _, name := range skillNames {
		if name != multiEventSkill && name != multiMiscSkill {
			prerequisiteNames = append(prerequisiteNames, name)
		}
	}
	if len(prerequisiteNames) > 0 {
		var n, nSkipped int
		n, nSkipped, err = skillSetupInstallMulti(src, prerequisiteNames, migrationTargets, out, errOut)
		installed += n
		skipped += nSkipped
		if err != nil {
			return installed, skipped, err
		}
		if nSkipped > 0 {
			return installed, skipped, fmt.Errorf("event Skill 迁移前置安装不完整（skipped=%d）；已保留折叠版 Event/misc", nSkipped)
		}
	}

	migrated, migrationErr := migrateEventMiscAtomically(src, migrationTargets, out, errOut)
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
func installMultiSkillToHomes(src string, skillNames []string, dests []string, out, errOut io.Writer) (installed, skipped int, err error) {
	sort.Strings(dests)
	for _, dest := range dests {
		// 互斥清理：装 multi 前先把 dest/dws/ 整个删除（mono 残留）
		cleanupMutualExclusion(dest, skillSetupModeMulti, out, errOut)

		if err := skillSetupMkdirAll(dest, 0o755); err != nil {
			fmt.Fprintf(errOut, "  ✗ Agent 目录创建失败 %s: %v\n", dest, err)
			skipped += len(skillNames)
			continue
		}

		for _, name := range skillNames {
			subSrc := filepath.Join(src, name)
			subDest := filepath.Join(dest, name)
			if err := skillSetupRemoveAll(subDest); err != nil {
				fmt.Fprintf(errOut, "  ✗ 清理失败 %s: %v\n", subDest, err)
				skipped++
				continue
			}
			if err := skillSetupCopyDir(subSrc, subDest); err != nil {
				fmt.Fprintf(errOut, "  ✗ 拷贝失败 %s: %v\n", subDest, err)
				skipped++
				continue
			}
			fmt.Fprintf(out, "  ✓ %s\n", subDest)
			installed++
		}
	}
	return installed, skipped, nil
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
