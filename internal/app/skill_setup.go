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

func newSkillSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "安装 dws 自身 skill 到 Agent 目录",
		Long: `安装 dws 自身 skill 文档到 AI Agent 目录（如 ~/.claude/skills/、~/.cursor/skills/ 等）。

支持两种模式：
  mono                  单 skill（稳定 / 推荐）—— 总入口 SKILL.md + references/products/
  multi  🧪 EXPERIMENTAL  多 skill（试验版 / Preview）—— 按产品拆 N 个独立 skill
                         尚未达到 stable 标准，接口、命名与跨 skill 引用可能变动；
                         生产前请评估，问题请提 issue 反馈

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
  dws skill setup --mode multi -x live -x devdoc        # 装其余 18 个，剔除 2 个
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

	mode, err := resolveSkillSetupMode(mode, autoYes, out)
	if err != nil {
		return err
	}

	if mode == skillSetupModeMono && (len(includeRaw) > 0 || len(excludeRaw) > 0) {
		return fmt.Errorf("--skill / --exclude 仅在 --mode multi 下有效（mono 只有一个 skill，无需挑选）")
	}

	skillSrc, srcCleanup, err := resolveSkillSetupSourceOrEmbedded(source, mode)
	if err != nil {
		return err
	}
	defer srcCleanup()

	dests, err := resolveSkillSetupTargets(target, mode)
	if err != nil {
		return err
	}

	// multi 模式枚举 src 下的子 skill 名，供确认信息与安装步骤共用
	var multiSkillNames []string
	if mode == skillSetupModeMulti {
		allMultiSkillNames, listErr := listMultiSkillNames(skillSrc)
		if listErr != nil {
			return listErr
		}
		if len(allMultiSkillNames) == 0 {
			return fmt.Errorf("multi 模式下 %s 内未发现含 SKILL.md 的子目录", skillSrc)
		}
		filtered, filterErr := filterMultiSkillNames(allMultiSkillNames, includeRaw, excludeRaw)
		if filterErr != nil {
			return filterErr
		}
		// dws-shared carries the global rules every product skill declares as a
		// PREREQUISITE; it must ship even when --skill / --exclude narrows the set.
		multiSkillNames = ensureMandatorySharedSkill(filtered, allMultiSkillNames)
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
		}
		return nil
	}

	if !autoYes {
		ok, err := confirmSkillSetup(out, mode, skillSrc, dests, multiSkillNames)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "已取消。")
			return nil
		}
	} else if mode == skillSetupModeMulti {
		fmt.Fprintln(errOut, "🧪 multi 模式当前为 EXPERIMENTAL（试验版 / Preview）—— 接口与布局可能变动，稳定版请用 --mode mono")
	}

	var installed, skipped int
	switch mode {
	case skillSetupModeMono:
		installed, skipped, err = installSkillToHomes(skillSrc, dests, out, errOut)
	case skillSetupModeMulti:
		installed, skipped, err = installMultiSkillToHomes(skillSrc, multiSkillNames, dests, out, errOut)
	default:
		return fmt.Errorf("内部错误：未知 mode %q", mode)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\n✅ Skill 安装完成（mode=%s, installed=%d, skipped=%d）\n", mode, installed, skipped)
	return nil
}

// multiSkillPrefix is the canonical prefix for every per-product skill
// bundle in skills/multi/ (e.g. dingtalk-aitable, dingtalk-calendar).
const multiSkillPrefix = "dingtalk-"

// multiSharedSkill is the shared, non-product skill that every per-product
// skill declares as a PREREQUISITE. It must always be installed in multi mode
// regardless of --skill / --exclude, otherwise the product skills reference a
// dws-shared that was never installed.
const multiSharedSkill = "dws-shared"

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

// filterMultiSkillNames narrows `all` by include / exclude lists.
// Semantics mirror lark-cli's `npx skills add -s lark-calendar`:
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
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("无法读取 multi skill 源目录 %s: %w", src, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(src, e.Name(), "SKILL.md")); err == nil {
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

	if autoYes || !isInteractiveTerminal() {
		fmt.Fprintln(out, "未指定 --mode，非交互环境下默认使用 mono")
		return skillSetupModeMono, nil
	}

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 dws skill 安装模式").
				Description("mono = 单 skill 入口（稳定 / 推荐）\nmulti = 按产品拆分（🧪 EXPERIMENTAL / 试验版，未达 stable，接口可能变动）").
				Options(
					huh.NewOption("mono — 单 skill（稳定 / 推荐）", skillSetupModeMono),
					huh.NewOption("multi — 多 skill（🧪 EXPERIMENTAL · 试验版）", skillSetupModeMulti),
				).
				Value(&choice),
		),
	)
	if err := form.Run(); err != nil {
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
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(exeDir, "skills", subdir),
			filepath.Join(exeDir, "..", "skills", subdir),
			filepath.Join(exeDir, "..", "share", "skills", "dws"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(wd, "skills", subdir))
	}
	// User-level cache populated by install.sh / install.ps1 / npm install.js
	// from the dws-skills.zip release asset. Lets `dws skill setup` find a
	// source even when the user has no source checkout on disk.
	if home, err := os.UserHomeDir(); err == nil {
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
		fi, err := os.Stat(filepath.Join(path, "SKILL.md"))
		return err == nil && !fi.IsDir()
	case skillSetupModeMulti:
		entries, err := os.ReadDir(path)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.IsDir() {
				if _, err := os.Stat(filepath.Join(path, e.Name(), "SKILL.md")); err == nil {
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
	home, err := os.UserHomeDir()
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
			if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		out = append(out, agentHomeForMode(base, mode))
	}
	if len(out) == 0 {
		out = append(out, agentHomeForMode(filepath.Join(home, ".agents", "skills"), mode))
	}
	return out
}

func confirmSkillSetup(out io.Writer, mode, src string, dests []string, multiSkillNames []string) (bool, error) {
	if mode == skillSetupModeMulti {
		fmt.Fprintln(out, "\n🧪 ─────────────────────────────────────────────────────────────")
		fmt.Fprintln(out, "    multi 模式当前为 EXPERIMENTAL（试验版 / Preview）")
		fmt.Fprintln(out, "    · 20 个 dingtalk-* 子 skill 跑过 verifier，可用但未达 stable")
		fmt.Fprintln(out, "    · 跨 skill 引用、bundle 命名、目录布局后续可能调整")
		fmt.Fprintln(out, "    · 不建议在生产 / 共享环境直接落地；问题请提 issue 反馈")
		fmt.Fprintln(out, "    稳定版请用 --mode mono")
		fmt.Fprintln(out, "🧪 ─────────────────────────────────────────────────────────────")
	}
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

	if !isInteractiveTerminal() {
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
	if err := form.Run(); err != nil {
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
		entries, err := os.ReadDir(agentHome)
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
		if _, err := os.Stat(monoPath); err == nil {
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
		if err := os.RemoveAll(victim); err != nil {
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

		if err := os.RemoveAll(dest); err != nil {
			fmt.Fprintf(errOut, "  ✗ 清理失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintf(errOut, "  ✗ 父目录创建失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		if err := copyDir(src, dest); err != nil {
			fmt.Fprintf(errOut, "  ✗ 拷贝失败 %s: %v\n", dest, err)
			skipped++
			continue
		}
		fmt.Fprintf(out, "  ✓ %s\n", dest)
		installed++
	}
	return installed, skipped, nil
}

// installMultiSkillToHomes installs each subdir of src (dingtalk-*) into
// dest as a sibling skill directory. installed/skipped is counted per
// (agent-home × sub-skill) pair so the user sees granular progress.
func installMultiSkillToHomes(src string, skillNames []string, dests []string, out, errOut io.Writer) (installed, skipped int, err error) {
	sort.Strings(dests)
	for _, dest := range dests {
		// 互斥清理：装 multi 前先把 dest/dws/ 整个删除（mono 残留）
		cleanupMutualExclusion(dest, skillSetupModeMulti, out, errOut)

		if err := os.MkdirAll(dest, 0o755); err != nil {
			fmt.Fprintf(errOut, "  ✗ Agent 目录创建失败 %s: %v\n", dest, err)
			skipped += len(skillNames)
			continue
		}

		for _, name := range skillNames {
			subSrc := filepath.Join(src, name)
			subDest := filepath.Join(dest, name)
			if err := os.RemoveAll(subDest); err != nil {
				fmt.Fprintf(errOut, "  ✗ 清理失败 %s: %v\n", subDest, err)
				skipped++
				continue
			}
			if err := copyDir(subSrc, subDest); err != nil {
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
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// resolve symlink target and copy the underlying file
			resolved, err := os.Readlink(path)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode&os.ModePerm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
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
