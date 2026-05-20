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
  mono   单 skill（推荐）—— 总入口 SKILL.md + references/products/
  multi  多 skill（实验中）—— 按产品拆 N 个独立 skill

不带 --mode 时进入交互式询问；不带 --target 时铺到所有检测到的 Agent 目录。`,
		Example: `  dws skill setup                                # 交互式
  dws skill setup --mode mono --yes              # 非交互装 mono
  dws skill setup --mode multi --target claude   # 只装到 ~/.claude/skills/
  dws skill setup --source /path/to/repo         # 显式指定 skill 源`,
		DisableAutoGenTag: true,
		RunE:              runSkillSetup,
	}
	cmd.Flags().String("mode", "", "skill 模式：mono | multi（不指定则交互询问）")
	cmd.Flags().String("target", "all", "目标 Agent：all | "+supportedTargets())
	cmd.Flags().String("source", "", "skill 源目录（默认自动查找二进制旁边或当前目录）")
	cmd.Flags().Bool("yes", false, "跳过所有确认提示")
	return cmd
}

func runSkillSetup(cmd *cobra.Command, _ []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	target, _ := cmd.Flags().GetString("target")
	source, _ := cmd.Flags().GetString("source")
	autoYes, _ := cmd.Flags().GetBool("yes")

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	mode, err := resolveSkillSetupMode(mode, autoYes, out)
	if err != nil {
		return err
	}

	if mode == skillSetupModeMulti {
		return fmt.Errorf("--mode multi 当前尚未启用（multi skill 内容将在后续 PR 中提供），请先使用 --mode mono")
	}

	skillSrc, err := resolveSkillSetupSource(source, mode)
	if err != nil {
		return err
	}

	dests, err := resolveSkillSetupTargets(target)
	if err != nil {
		return err
	}

	if !autoYes {
		ok, err := confirmSkillSetup(out, mode, skillSrc, dests)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "已取消。")
			return nil
		}
	}

	installed, skipped, err := installSkillToHomes(skillSrc, dests, out, errOut)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\n✅ Skill 安装完成（mode=%s, installed=%d, skipped=%d）\n", mode, installed, skipped)
	return nil
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
				Description("mono = 单 skill 入口（推荐）；multi = 按产品拆分（实验中）").
				Options(
					huh.NewOption("mono — 单 skill（推荐）", skillSetupModeMono),
					huh.NewOption("multi — 多 skill（实验中）", skillSetupModeMulti),
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

	candidates := skillSourceCandidates(explicit, subdir)
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
func resolveSkillSetupTargets(target string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("无法解析用户 HOME: %w", err)
	}

	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" || target == "all" {
		return detectExistingAgentHomes(home), nil
	}

	rel, ok := agentSkillPaths[target]
	if !ok {
		return nil, fmt.Errorf("不支持的 --target 值: %s（可选 all, %s）", target, supportedTargets())
	}
	return []string{filepath.Join(home, rel, "dws")}, nil
}

func detectExistingAgentHomes(home string) []string {
	var out []string
	for i, rel := range skillSetupAgentHomes {
		base := filepath.Join(home, rel)
		parent := filepath.Dir(base)
		if i > 0 {
			if _, err := os.Stat(parent); errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		out = append(out, filepath.Join(base, "dws"))
	}
	if len(out) == 0 {
		out = append(out, filepath.Join(home, ".agents", "skills", "dws"))
	}
	return out
}

func confirmSkillSetup(out io.Writer, mode, src string, dests []string) (bool, error) {
	fmt.Fprintf(out, "\n📦 将安装 skill：\n  mode: %s\n  source: %s\n  destinations:\n", mode, src)
	for _, d := range dests {
		fmt.Fprintf(out, "    - %s\n", d)
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

func installSkillToHomes(src string, dests []string, out, errOut io.Writer) (installed, skipped int, err error) {
	sort.Strings(dests)
	for _, dest := range dests {
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
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
