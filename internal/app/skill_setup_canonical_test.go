package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillSetupCanonicalTargetsAndAgentCapabilities(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupAgentHomes, []string{
		".agents/skills", ".codex/skills", ".claude/skills", ".openclaw/skills",
	})
	for _, parent := range []string{".codex", ".claude", ".openclaw"} {
		if err := os.MkdirAll(filepath.Join(home, parent), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dests, err := resolveSkillSetupTargets("all", skillSetupModeMulti)
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(home, ".agents", "skills")
	if len(dests) != 4 || dests[0] != canonical {
		t.Fatalf("targets = %v", dests)
	}

	src := t.TempDir()
	for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
		dir := filepath.Join(src, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldCodex := filepath.Join(home, ".codex", "skills", "dingtalk-chat")
	if err := os.MkdirAll(oldCodex, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldCodex, "SKILL.md"), []byte("beta.6"), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSkills, "dingtalk-chat"), []byte("unexpected file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(claudeSkills, "dingtalk-shared")); err != nil {
		t.Fatal(err)
	}

	plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, dests, []string{"dingtalk-chat", "dingtalk-shared"}, false)
	if err != nil {
		t.Fatal(err)
	}
	installed, skipped, err := executeSkillSetupPlan(plan, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil || skipped != 0 || installed != 6 { // canonical + two linked Agents, two Skills each
		t.Fatalf("execute = installed %d skipped %d err %v", installed, skipped, err)
	}
	if _, err := os.Lstat(oldCodex); !os.IsNotExist(err) {
		t.Fatalf("Codex duplicate remains: %v", err)
	}
	for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
		if _, err := os.Stat(filepath.Join(canonical, name, "SKILL.md")); err != nil {
			t.Fatalf("canonical %s missing: %v", name, err)
		}
		for _, agent := range []string{".claude", ".openclaw"} {
			link := filepath.Join(home, agent, "skills", name)
			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("link %s = %#v, %v", link, info, err)
			}
		}
	}
	// Re-running setup must recognize the existing links as already correct;
	// canonical refreshes in place without turning links into copied trees.
	plan, err = buildSkillSetupPlan(skillSetupModeMulti, src, dests, []string{"dingtalk-chat", "dingtalk-shared"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeSkillSetupPlan(plan, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{".claude", ".openclaw"} {
		info, err := os.Lstat(filepath.Join(home, agent, "skills", "dingtalk-chat"))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("idempotent setup replaced %s link: %#v, %v", agent, info, err)
		}
	}
}

func TestCrossPlatformCoverageSkillSetupDetectsShallowAndApplicationAgents(t *testing.T) {
	// Keep the destination HOME synthetic: app-bundle detection is deliberately
	// machine-scoped and must not depend on the selected installation HOME.
	home := t.TempDir()
	testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })
	for _, dir := range []string{filepath.Join(home, ".config", "kimchi"), filepath.Join(home, ".tabnine")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(home, "app-sentinel")
	if err := os.MkdirAll(sentinel, 0o755); err != nil {
		t.Fatal(err)
	}
	appInfo, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	zcodeApp := filepath.Join(string(filepath.Separator), "Applications", "ZCode.app")
	minimaxApp := filepath.Join(string(filepath.Separator), "Applications", "MiniMax Code.app")
	originalStat := skillSetupStat
	testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
		if path == zcodeApp || path == minimaxApp {
			return appInfo, nil
		}
		return originalStat(path)
	})
	dests := detectExistingAgentHomes(home, skillSetupModeMulti)
	for _, target := range []string{
		filepath.Join(home, ".config", "kimchi", "harness", "skills"),
		filepath.Join(home, ".tabnine", "agent", "skills"),
		filepath.Join(home, ".zcode", "skills"),
		filepath.Join(home, ".minimax", "skills"),
	} {
		if !containsSkillName(dests, target) {
			t.Errorf("detected targets %v missing %s", dests, target)
		}
	}
}

func TestCrossPlatformCoverageSkillSetupCustomRootsAliasesAndUniversalTargets(t *testing.T) {
	home := t.TempDir()
	customClaude := filepath.Join(t.TempDir(), "claude")
	customCodex := filepath.Join(t.TempDir(), "codex")
	customHermes := filepath.Join(t.TempDir(), "hermes")
	for _, root := range []string{customClaude, customCodex, customHermes, filepath.Join(home, ".moltbot"), filepath.Join(home, ".copilot"), filepath.Join(home, ".config", "opencode"), filepath.Join(home, ".config", "agents"), filepath.Join(home, ".codeium", "windsurf")} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupGetenv, func(name string) string {
		switch name {
		case "CLAUDE_CONFIG_DIR":
			return customClaude
		case "CODEX_HOME":
			return customCodex
		case "HERMES_HOME":
			return customHermes
		default:
			return ""
		}
	})

	dests, err := resolveSkillSetupTargets("all", skillSetupModeMulti)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(customClaude, "skills"),
		filepath.Join(customCodex, "skills"),
		filepath.Join(customHermes, "skills"),
		filepath.Join(home, ".moltbot", "skills"),
		filepath.Join(home, ".copilot", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".config", "agents", "skills"),
		filepath.Join(home, ".codeium", "windsurf", "skills"),
	} {
		found := false
		for _, got := range dests {
			if sameSkillSetupPath(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("resolved targets %v missing %s", dests, want)
		}
	}
	if !isUniversalSkillSetupBase(filepath.Join(customCodex, "skills")) || !isUniversalSkillSetupBase(filepath.Join(home, ".config", "opencode", "skills")) || !isUniversalSkillSetupBase(filepath.Join(home, ".config", "agents", "skills")) {
		t.Fatal("custom Codex, OpenCode, and Amp must be universal cleanup-only targets")
	}
}

func TestCrossPlatformCoverageSkillSetupCanonicalFailureStopsDependentTargets(t *testing.T) {
	home := t.TempDir()
	canonical := filepath.Join(home, ".agents", "skills")
	claude := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	oldClaude := filepath.Join(claude, "dingtalk-chat")
	if err := os.MkdirAll(oldClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldClaude, "SKILL.md"), []byte("old remains"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "dingtalk-chat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dingtalk-chat", "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat"}, true)
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { return errors.New("canonical copy denied") })
	if _, _, err := executeSkillSetupPlan(plan, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("canonical failure = %v", err)
	}
	body, readErr := os.ReadFile(filepath.Join(oldClaude, "SKILL.md"))
	if readErr != nil || string(body) != "old remains" {
		t.Fatalf("dependent Claude target changed: %q, %v", body, readErr)
	}
}

func TestCrossPlatformCoverageSkillSetupCanonicalCopyFallbackMessage(t *testing.T) {
	home := t.TempDir()
	canonical := filepath.Join(home, ".agents", "skills")
	claude := filepath.Join(home, ".claude", "skills")
	src := t.TempDir()
	skillSrc := filepath.Join(src, "dingtalk-chat")
	if err := os.MkdirAll(skillSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("chat"), 0o644); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupSymlink, func(string, string) error { return errors.New("links unavailable") })

	plan, err := buildSkillSetupPlan(
		skillSetupModeMulti,
		src,
		[]string{canonical, claude},
		[]string{"dingtalk-chat"},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
	if err != nil || installed != 2 || skipped != 0 {
		t.Fatalf("execute = installed %d skipped %d err %v", installed, skipped, err)
	}
	if !strings.Contains(errOut.String(), "自动改用兼容安装") {
		t.Fatalf("human-readable fallback message missing: %s", errOut.String())
	}
	info, err := os.Lstat(filepath.Join(claude, "dingtalk-chat"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy fallback = %#v, %v", info, err)
	}
}

func TestCrossPlatformCoverageUpstreamAgentEnumerationAndEffectiveRoots(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

	expected := map[string]string{
		"aider-desk": ".aider-desk/skills", "amp": ".config/agents/skills",
		"antigravity": ".gemini/antigravity/skills", "antigravity-cli": ".gemini/antigravity-cli/skills",
		"astrbot": ".astrbot/data/skills", "autohand-code": ".autohand/skills",
		"augment": ".augment/skills", "bob": ".bob/skills", "claude-code": ".claude/skills",
		"openclaw": ".openclaw/skills", "cline": ".agents/skills", "codearts-agent": ".codeartsdoer/skills",
		"codebuddy": ".codebuddy/skills", "codemaker": ".codemaker/skills", "codestudio": ".codestudio/skills",
		"codex": ".codex/skills", "command-code": ".commandcode/skills", "continue": ".continue/skills",
		"cortex": ".snowflake/cortex/skills", "crush": ".config/crush/skills", "cursor": ".cursor/skills",
		"deepagents": ".deepagents/agent/skills", "devin": ".config/devin/skills", "dexto": ".agents/skills",
		"droid": ".factory/skills", "firebender": ".firebender/skills", "forgecode": ".forge/skills",
		"gemini-cli": ".gemini/skills", "github-copilot": ".copilot/skills", "goose": ".config/goose/skills",
		"grok": ".grok/skills", "hermes-agent": ".hermes/skills", "inference-sh": ".inferencesh/skills",
		"jazz": ".jazz/skills", "junie": ".junie/skills", "iflow-cli": ".iflow/skills",
		"kilo": ".kilocode/skills", "kimchi": ".config/kimchi/harness/skills", "kimi-code-cli": ".agents/skills",
		"kiro-cli": ".kiro/skills", "kode": ".kode/skills", "lingma": ".lingma/skills", "loaf": ".agents/skills",
		"mcpjam": ".mcpjam/skills", "minimax-code": ".minimax/skills", "mistral-vibe": ".vibe/skills",
		"moxby": ".moxby/skills", "mux": ".mux/skills", "opencode": ".config/opencode/skills",
		"openhands": ".openhands/skills", "ona": ".ona/skills", "pi": ".pi/agent/skills",
		"qoder": ".qoder/skills", "qoder-cn": ".qoder-cn/skills", "qwen-code": ".qwen/skills",
		"replit": ".config/agents/skills", "reasonix": ".reasonix/skills", "rovodev": ".rovodev/skills",
		"roo": ".roo/skills", "tabnine-cli": ".tabnine/agent/skills", "terramind": ".terramind/skills",
		"tinycloud": ".tinycloud/skills", "trae": ".trae/skills", "trae-cn": ".trae-cn/skills",
		"universal": ".config/agents/skills", "warp": ".agents/skills", "windsurf": ".codeium/windsurf/skills",
		"zed": ".agents/skills", "zcode": ".zcode/skills", "zencoder": ".zencoder/skills",
		"zenflow": ".zencoder/skills", "neovate": ".neovate/skills", "pochi": ".pochi/skills", "adal": ".adal/skills",
	}
	if got := len(expected) + len(unsupportedGlobalAgentTargets); got != 76 {
		t.Fatalf("upstream agent enumeration = %d, want 76", got)
	}
	for target, rel := range expected {
		mapped, ok := agentSkillPaths[target]
		if !ok || filepath.Clean(mapped) != filepath.Clean(rel) {
			t.Errorf("agent %s map = %q, want %q", target, mapped, rel)
		}
		if got := resolveSkillSetupBase(home, target); !sameSkillSetupPath(got, filepath.Join(home, filepath.FromSlash(rel))) {
			t.Errorf("agent %s effective root = %q, want %q", target, got, filepath.Join(home, rel))
		}
	}
	for _, target := range []string{"eve", "promptscript"} {
		if _, err := resolveSkillSetupTargets(target, skillSetupModeMulti); err == nil {
			t.Errorf("%s unexpectedly resolved a global setup root", target)
		}
		if _, err := resolveSkillTargetPath(target); err == nil {
			t.Errorf("%s unexpectedly resolved a marketplace install root", target)
		}
	}
	if got := supportedTargets(); !strings.Contains(got, "eve") || !strings.Contains(got, "promptscript") {
		t.Fatalf("supported targets omit no-global upstream agents: %s", got)
	}

	custom := map[string]string{
		"AUTOHAND_HOME": filepath.Join(home, "autohand-home"), "CLAUDE_CONFIG_DIR": filepath.Join(home, "claude-home"),
		"CODEX_HOME": filepath.Join(home, "codex-home"), "GROK_HOME": filepath.Join(home, "grok-home"),
		"HERMES_HOME": filepath.Join(home, "hermes-home"), "VIBE_HOME": filepath.Join(home, "vibe-home"),
		"XDG_CONFIG_HOME": filepath.Join(home, "xdg"),
	}
	testseam.Swap(t, &skillSetupGetenv, func(name string) string { return custom[name] })
	customCases := map[string]string{
		"autohand-code": filepath.Join(custom["AUTOHAND_HOME"], "skills"),
		"claude-code":   filepath.Join(custom["CLAUDE_CONFIG_DIR"], "skills"),
		"codex":         filepath.Join(custom["CODEX_HOME"], "skills"),
		"grok":          filepath.Join(custom["GROK_HOME"], "skills"),
		"hermes-agent":  filepath.Join(custom["HERMES_HOME"], "skills"),
		"mistral-vibe":  filepath.Join(custom["VIBE_HOME"], "skills"),
		"amp":           filepath.Join(custom["XDG_CONFIG_HOME"], "agents", "skills"),
		"replit":        filepath.Join(custom["XDG_CONFIG_HOME"], "agents", "skills"),
		"universal":     filepath.Join(custom["XDG_CONFIG_HOME"], "agents", "skills"),
		"crush":         filepath.Join(custom["XDG_CONFIG_HOME"], "crush", "skills"),
		"devin":         filepath.Join(custom["XDG_CONFIG_HOME"], "devin", "skills"),
		"goose":         filepath.Join(custom["XDG_CONFIG_HOME"], "goose", "skills"),
		"kimchi":        filepath.Join(custom["XDG_CONFIG_HOME"], "kimchi", "harness", "skills"),
		"opencode":      filepath.Join(custom["XDG_CONFIG_HOME"], "opencode", "skills"),
	}
	for target, want := range customCases {
		if got := resolveSkillSetupBase(home, target); !sameSkillSetupPath(got, want) {
			t.Errorf("custom %s root = %q, want %q", target, got, want)
		}
	}
	for _, target := range []string{"codex", "amp", "opencode"} {
		if !isUniversalSkillSetupBase(resolveSkillSetupBase(home, target)) {
			t.Errorf("custom %s root not classified universal", target)
		}
	}
}

func TestCrossPlatformCoverageOpenClawAliasPriority(t *testing.T) {
	for _, tc := range []struct {
		name string
		dirs []string
		want string
	}{
		{name: "default", want: ".openclaw"},
		{name: "moltbot", dirs: []string{".moltbot"}, want: ".moltbot"},
		{name: "clawdbot-before-moltbot", dirs: []string{".moltbot", ".clawdbot"}, want: ".clawdbot"},
		{name: "openclaw-first", dirs: []string{".moltbot", ".clawdbot", ".openclaw"}, want: ".openclaw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := resolveOpenClawSetupBase(home); got != filepath.Join(home, tc.want, "skills") {
				t.Fatalf("OpenClaw root = %q", got)
			}
		})
	}
}

func TestCrossPlatformCoverageSkillSetupWindowsPathNormalization(t *testing.T) {
	testseam.Swap(t, &skillSetupFoldPathCase, true)
	if !sameSkillSetupPath(filepath.Join("Root", "Skills"), filepath.Join("root", "skills")) {
		t.Fatal("case-insensitive platform path normalization failed")
	}
}
