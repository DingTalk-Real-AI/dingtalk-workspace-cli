package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

func TestCrossPlatformCoverageSkillSetupCanonicalTargetsAndAgentCapabilities(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })
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
	var out, errOut bytes.Buffer
	installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
	if err != nil || skipped != 0 || installed != 6 { // canonical + two linked Agents, two Skills each
		t.Fatalf("execute = installed %d skipped %d err %v; stdout:\n%s\nstderr:\n%s", installed, skipped, err, out.String(), errOut.String())
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
			if err != nil {
				t.Fatalf("link %s lstat failed: %v", link, err)
			}
			if runtime.GOOS == "windows" {
				if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular == 0 {
					t.Fatalf("windows link %s mode = %v, want junction (irregular, non-symlink)", link, info.Mode())
				}
				if _, err := os.Readlink(link); err != nil {
					t.Fatalf("windows junction %s readlink error = %v", link, err)
				}
				statInfo, err := os.Stat(link)
				if err != nil || !statInfo.IsDir() {
					t.Fatalf("windows junction %s stat = %#v, %v", link, statInfo, err)
				}
				body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
				if err != nil || string(body) != name {
					t.Fatalf("windows junction %s SKILL.md = %q, %v", link, body, err)
				}
			} else if info.Mode()&os.ModeSymlink == 0 {
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
		link := filepath.Join(home, agent, "skills", "dingtalk-chat")
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("idempotent setup %s lstat failed: %v", agent, err)
		}
		if runtime.GOOS == "windows" {
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular == 0 {
				t.Fatalf("idempotent setup windows link %s mode = %v, want junction", agent, info.Mode())
			}
			if _, err := os.Readlink(link); err != nil {
				t.Fatalf("idempotent setup windows junction %s readlink error = %v", agent, err)
			}
			statInfo, err := os.Stat(link)
			if err != nil || !statInfo.IsDir() {
				t.Fatalf("idempotent setup windows junction %s stat = %#v, %v", link, statInfo, err)
			}
			body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
			if err != nil || string(body) != "dingtalk-chat" {
				t.Fatalf("idempotent setup windows junction %s SKILL.md = %q, %v", link, body, err)
			}
		} else if info.Mode()&os.ModeSymlink == 0 {
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

func TestCrossPlatformCoverageSkillSetupLinkValidationFallback(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         string
		failStat     bool
		notDir       bool
		skillStatErr bool
		skillIsDir   bool
		failOpen     bool
		failTemp     bool
		failSecond   bool
	}{
		{name: "mono stat error", mode: skillSetupModeMono, failStat: true},
		{name: "stat error", failStat: true},
		{name: "link not a directory", notDir: true},
		{name: "skill.md stat error", skillStatErr: true},
		{name: "skill.md is directory", skillIsDir: true},
		{name: "skill entry read", failOpen: true},
		{name: "staging temp error", failTemp: true},
		{name: "staging cleanup second link error", failSecond: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			canonical := filepath.Join(home, ".agents", "skills")
			claude := filepath.Join(home, ".claude", "skills")
			src := t.TempDir()
			for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
				skillSrc := filepath.Join(src, name)
				if err := os.MkdirAll(skillSrc, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte(name), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("mono"), 0o644); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
			testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

			var stagedPath string
			calls := 0
			testseam.Swap(t, &skillSetupSymlink, func(target, link string) error {
				calls++
				if tc.failSecond && calls == 2 {
					return errors.New("second link failed")
				}
				stagedPath = link
				// Real link so the staging identity lstat observes an entry; the
				// target may not resolve, which the mocked stat checks cover.
				return os.Symlink(target, link)
			})
			if tc.failTemp {
				testseam.Swap(t, &skillSetupPublishTemp, func(dir, pattern string) (string, error) {
					if strings.Contains(pattern, "staging-") {
						return "", errors.New("temp error")
					}
					return os.MkdirTemp(dir, pattern)
				})
			}
			originalStat := skillSetupStat
			testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
				if tc.failStat && stagedPath != "" && path == stagedPath {
					return nil, errors.New("EPERM")
				}
				if tc.notDir && stagedPath != "" && path == stagedPath {
					return skillSetupFileInfo{name: filepath.Base(path), mode: 0o644}, nil
				}
				if stagedPath != "" && path == stagedPath {
					return skillSetupFileInfo{name: filepath.Base(path), mode: os.ModeDir}, nil
				}
				if tc.skillStatErr && stagedPath != "" && path == filepath.Join(stagedPath, "SKILL.md") {
					return nil, os.ErrNotExist
				}
				if tc.skillIsDir && stagedPath != "" && path == filepath.Join(stagedPath, "SKILL.md") {
					return skillSetupFileInfo{name: "SKILL.md", mode: os.ModeDir}, nil
				}
				if stagedPath != "" && path == filepath.Join(stagedPath, "SKILL.md") {
					return skillSetupFileInfo{name: "SKILL.md", mode: 0o644}, nil
				}
				return originalStat(path)
			})
			if tc.failOpen {
				originalOpen := skillSetupOpen
				testseam.Swap(t, &skillSetupOpen, func(path string) (*os.File, error) {
					if stagedPath != "" && path == filepath.Join(stagedPath, "SKILL.md") {
						return nil, errors.New("EPERM")
					}
					return originalOpen(path)
				})
			} else {
				originalOpen := skillSetupOpen
				testseam.Swap(t, &skillSetupOpen, func(path string) (*os.File, error) {
					if stagedPath != "" && path == filepath.Join(stagedPath, "SKILL.md") {
						return os.Open(filepath.Join(src, "dingtalk-chat", "SKILL.md"))
					}
					return originalOpen(path)
				})
			}

			mode := skillSetupModeMulti
			if tc.mode != "" {
				mode = tc.mode
			}
			canonicalDest := canonical
			if mode == skillSetupModeMono {
				canonicalDest = filepath.Join(canonical, "dws")
			}
			destClaude := claude
			if mode == skillSetupModeMono {
				destClaude = filepath.Join(claude, "dws")
			}
			plan, err := buildSkillSetupPlan(mode, src, []string{canonicalDest, destClaude}, []string{"dingtalk-chat", "dingtalk-shared"}, false)
			if err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
			expectedInstalled := 4
			if mode == skillSetupModeMono {
				expectedInstalled = 2
			}
			if err != nil || installed != expectedInstalled || skipped != 0 {
				t.Fatalf("execute = installed %d skipped %d err %v", installed, skipped, err)
			}
			if !strings.Contains(errOut.String(), "自动改用兼容安装") {
				t.Fatalf("fallback message missing: %s", errOut.String())
			}
			skillFile := filepath.Join(claude, "dingtalk-chat", "SKILL.md")
			expectedContent := "dingtalk-chat"
			if mode == skillSetupModeMono {
				skillFile = filepath.Join(claude, "dws", "SKILL.md")
				expectedContent = "mono"
			}
			body, err := os.ReadFile(skillFile)
			if err != nil || string(body) != expectedContent {
				t.Fatalf("fallback Skill unreadable: %q, %v", body, err)
			}
		})
	}
}

func TestCrossPlatformCoverageSkillSetupStagingCleanupFailureBlocksFallback(t *testing.T) {
	// Also exercise skillSetupStagingCleanupError Unwrap
	cleanupErr := &skillSetupStagingCleanupError{Path: "test", Err: errors.New("underlying")}
	if !errors.Is(cleanupErr, cleanupErr.Err) {
		t.Fatal("expected skillSetupStagingCleanupError to unwrap underlying error")
	}

	t.Run("cleanup failure blocks fallback", func(t *testing.T) {
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
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		var stagedPath string
		testseam.Swap(t, &skillSetupSymlink, func(target, link string) error {
			stagedPath = link
			return os.Symlink(target, link)
		})
		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			if stagedPath != "" && path == stagedPath {
				return nil, errors.New("EPERM")
			}
			return os.Stat(path)
		})
		origRemoveAll := skillSetupRemoveAll
		testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
			if stagedPath != "" && path == stagedPath {
				return errors.New("mock cleanup staged link failure")
			}
			return origRemoveAll(path)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil {
			t.Fatalf("unexpected fatal plan error: %v", err)
		}
		if installed != 1 || skipped != 1 {
			t.Fatalf("execute = installed %d, skipped %d; want 1, 1; stdout:\n%s\nstderr:\n%s", installed, skipped, out.String(), errOut.String())
		}
		if strings.Contains(errOut.String(), "自动改用兼容安装") {
			t.Fatalf("cleanup failure must NOT enter compatibility fallback, got: %s", errOut.String())
		}
		if !strings.Contains(errOut.String(), "清理 Skill staging 失败") || !strings.Contains(errOut.String(), "mock cleanup staged link failure") {
			t.Fatalf("cleanup error must be visible in output, got: %s", errOut.String())
		}
		if _, err := os.Lstat(filepath.Join(claude, "dingtalk-chat")); !os.IsNotExist(err) {
			t.Fatalf("claude destination must not exist after failed staging cleanup: %v", err)
		}
	})

	t.Run("staged items cleanup error in stage defer", func(t *testing.T) {
		home := t.TempDir()
		canonical := filepath.Join(home, ".agents", "skills")
		claude := filepath.Join(home, ".claude", "skills")
		src := t.TempDir()
		for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
			s := filepath.Join(src, name)
			if err := os.MkdirAll(s, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s, "SKILL.md"), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		linkCalls := 0
		firstStaged := ""
		testseam.Swap(t, &skillSetupSymlink, func(target, link string) error {
			linkCalls++
			if linkCalls == 1 {
				firstStaged = link
				return os.Symlink(target, link)
			}
			return errors.New("second link failure")
		})
		origRemoveAll := skillSetupRemoveAll
		testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
			if firstStaged != "" && path == firstStaged {
				return errors.New("mock cleanup error for first staged item")
			}
			return origRemoveAll(path)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat", "dingtalk-shared"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		_, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil {
			t.Fatalf("unexpected fatal plan error: %v", err)
		}
		if skipped != 2 {
			t.Fatalf("skipped = %d, want 2", skipped)
		}
		if !strings.Contains(errOut.String(), "mock cleanup error for first staged item") {
			t.Fatalf("staged item cleanup error must be in errOut, got: %s", errOut.String())
		}
	})

	t.Run("staging placeholder prep error blocks fallback", func(t *testing.T) {
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
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		origRemoveAll := skillSetupRemoveAll
		testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
			if strings.Contains(path, "staging-") {
				return errors.New("mock prep error")
			}
			return origRemoveAll(path)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil {
			t.Fatalf("unexpected fatal plan error: %v", err)
		}
		if installed != 1 || skipped != 1 {
			t.Fatalf("execute = installed %d, skipped %d; want 1, 1", installed, skipped)
		}
		if strings.Contains(errOut.String(), "自动改用兼容安装") {
			t.Fatalf("prep failure must NOT enter compatibility fallback, got: %s", errOut.String())
		}
		if !strings.Contains(errOut.String(), "准备 Skill staging 路径失败") || !strings.Contains(errOut.String(), "mock prep error") {
			t.Fatalf("prep error must be visible in output, got: %s", errOut.String())
		}
	})

	t.Run("un-published staged item cleanup failure on publish error", func(t *testing.T) {
		home := t.TempDir()
		canonical := filepath.Join(home, ".agents", "skills")
		claude := filepath.Join(home, ".claude", "skills")
		src := t.TempDir()
		for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
			s := filepath.Join(src, name)
			if err := os.MkdirAll(s, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s, "SKILL.md"), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		publishStarted := false
		origPublish := skillSetupPublishPath
		testseam.Swap(t, &skillSetupPublishPath, func(staged, dest string) (upgrade.SkillPathPublication, error) {
			if strings.Contains(dest, ".claude") {
				publishStarted = true
				if strings.Contains(dest, "dingtalk-chat") {
					return upgrade.SkillPathPublication{}, errors.New("mock publish error")
				}
			}
			return origPublish(staged, dest)
		})
		origRemoveAll := skillSetupRemoveAll
		testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
			if publishStarted && strings.Contains(path, "dingtalk-shared.staging-") {
				return errors.New("mock un-published cleanup error")
			}
			return origRemoveAll(path)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat", "dingtalk-shared"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		_, _, err = executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil {
			t.Fatalf("unexpected plan fatal error: %v", err)
		}
		if !strings.Contains(errOut.String(), "mock un-published cleanup error") {
			t.Fatalf("un-published cleanup error must be in errOut, got: %s", errOut.String())
		}
	})

	t.Run("successful publish does not delete staged paths", func(t *testing.T) {
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
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		publishDone := false
		origPublish := skillSetupPublishPath
		testseam.Swap(t, &skillSetupPublishPath, func(staged, dest string) (upgrade.SkillPathPublication, error) {
			res, err := origPublish(staged, dest)
			if strings.Contains(dest, ".claude") {
				publishDone = true
			}
			return res, err
		})

		var postPublishRemovals []string
		origRemoveAll := skillSetupRemoveAll
		testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
			if publishDone {
				postPublishRemovals = append(postPublishRemovals, path)
			}
			return origRemoveAll(path)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil || installed != 2 || skipped != 0 {
			t.Fatalf("execute = installed %d, skipped %d, err %v", installed, skipped, err)
		}
		for _, call := range postPublishRemovals {
			if strings.Contains(call, "dingtalk-chat.staging-") {
				t.Fatalf("successfully published staged path must NOT be removed via RemoveAll: %s", call)
			}
		}
	})

	t.Run("staging cleanup identity mismatch refuses removal", func(t *testing.T) {
		item := skillSetupStagedDir{
			staged:   filepath.Join(t.TempDir(), "dummy"),
			identity: &skillSetupFileInfo{name: "dummy"},
			fileID:   "id1",
		}
		if err := os.WriteFile(item.staged, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillSetupIdentityProven, func(_, _ os.FileInfo, _, _ string) bool {
			return false
		})
		err := cleanSkillSetupStagedItem(item)
		if err == nil || !strings.Contains(err.Error(), "staging 对象身份已变化") {
			t.Fatalf("expected identity mismatch error, got: %v", err)
		}
		if _, err := os.Stat(item.staged); os.IsNotExist(err) {
			t.Fatal("file must not be deleted on identity mismatch")
		}
	})

	t.Run("upgrades legacy canonical adapter", func(t *testing.T) {
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
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })

		canonicalSkill := filepath.Join(canonical, "dingtalk-chat")
		if err := os.MkdirAll(canonicalSkill, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(canonicalSkill, "SKILL.md"), []byte("chat"), 0o644); err != nil {
			t.Fatal(err)
		}
		claudeSkill := filepath.Join(claude, "dingtalk-chat")
		if err := os.MkdirAll(claude, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(canonicalSkill, claudeSkill); err != nil {
			t.Fatal(err)
		}

		upgraded := false
		origAdapter := skillSetupCurrentCanonicalAdapter
		testseam.Swap(t, &skillSetupCurrentCanonicalAdapter, func(path, target string) bool {
			if path == claudeSkill && !upgraded {
				return false
			}
			return origAdapter(path, target)
		})

		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-chat"}, false)
		if err != nil {
			t.Fatal(err)
		}
		foundClaude := false
		for _, target := range plan.Targets {
			if target.Destination == claude {
				foundClaude = true
				if len(target.Backups) != 1 || target.Backups[0].Path != claudeSkill {
					t.Fatalf("claude backups = %#v, want replacement of %s", target.Backups, claudeSkill)
				}
			}
		}
		if !foundClaude {
			t.Fatal("claude target not found in plan")
		}

		upgraded = true
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil || installed != 2 || skipped != 0 {
			t.Fatalf("execute = installed %d, skipped %d, err %v", installed, skipped, err)
		}
	})

	t.Run("cleanSkillSetupStagedItem edge cases", func(t *testing.T) {
		// 1. Empty staged path
		if err := cleanSkillSetupStagedItem(skillSetupStagedDir{}); err != nil {
			t.Fatalf("cleanSkillSetupStagedItem empty = %v", err)
		}

		// 2. lstat error (non-NotExist)
		testseam.Swap(t, &skillSetupLstat, func(string) (os.FileInfo, error) {
			return nil, errors.New("mock lstat error")
		})
		item := skillSetupStagedDir{
			staged:   "some/path",
			identity: &skillSetupFileInfo{name: "some"},
		}
		if err := cleanSkillSetupStagedItem(item); err == nil || !strings.Contains(err.Error(), "mock lstat error") {
			t.Fatalf("expected lstat error, got: %v", err)
		}

		// 3. Missing creation identity refuses removal
		if err := cleanSkillSetupStagedItem(skillSetupStagedDir{staged: item.staged}); err == nil || !strings.Contains(err.Error(), "缺少创建身份") {
			t.Fatalf("expected missing identity error, got: %v", err)
		}

		// 4. Vanished staged path is already clean
		testseam.Swap(t, &skillSetupLstat, func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		})
		if err := cleanSkillSetupStagedItem(item); err != nil {
			t.Fatalf("vanished staged path = %v", err)
		}
	})

	t.Run("cleanSkillSetupStagedRoot ownership branches", func(t *testing.T) {
		// 1. Empty root path
		if err := cleanSkillSetupStagedRoot(skillSetupStagedRoot{}); err != nil {
			t.Fatalf("cleanSkillSetupStagedRoot empty = %v", err)
		}

		liveDir := filepath.Join(t.TempDir(), "stage-root")
		if err := os.MkdirAll(liveDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// 2. Missing creation identity refuses removal
		if err := cleanSkillSetupStagedRoot(skillSetupStagedRoot{path: liveDir}); err == nil || !strings.Contains(err.Error(), "缺少创建身份") {
			t.Fatalf("expected missing identity error, got: %v", err)
		}
		if _, err := os.Stat(liveDir); err != nil {
			t.Fatalf("root must survive refused cleanup: %v", err)
		}

		// 3. Identity mismatch refuses removal
		root := skillSetupStagedRoot{path: liveDir, identity: &skillSetupFileInfo{name: "stage-root"}, fileID: "id1"}
		testseam.Swap(t, &skillSetupIdentityProven, func(_, _ os.FileInfo, _, _ string) bool {
			return false
		})
		if err := cleanSkillSetupStagedRoot(root); err == nil || !strings.Contains(err.Error(), "身份已变化") {
			t.Fatalf("expected identity mismatch error, got: %v", err)
		}
		if _, err := os.Stat(liveDir); err != nil {
			t.Fatalf("root must survive refused cleanup: %v", err)
		}

		// 4. Proven identity removes the root
		testseam.Swap(t, &skillSetupIdentityProven, func(_, _ os.FileInfo, _, _ string) bool {
			return true
		})
		if err := cleanSkillSetupStagedRoot(root); err != nil {
			t.Fatalf("cleanSkillSetupStagedRoot proven = %v", err)
		}
		if _, err := os.Stat(liveDir); !os.IsNotExist(err) {
			t.Fatalf("proven root must be removed: %v", err)
		}
	})

	t.Run("staging root identity read failure", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		dest := t.TempDir()
		originalLstat := skillSetupLstat
		testseam.Swap(t, &skillSetupLstat, func(path string) (os.FileInfo, error) {
			if strings.HasPrefix(filepath.Base(path), ".dws-setup-set-") {
				return nil, errors.New("mock root lstat error")
			}
			return originalLstat(path)
		})
		_, _, err := stageSkillSetupTarget(
			&skillSetupPlan{Mode: skillSetupModeMulti, Source: src, MultiSkillNames: []string{"dingtalk-a"}},
			skillSetupTargetPlan{Destination: dest},
		)
		if err == nil || !strings.Contains(err.Error(), "读取 Skill staging 身份失败") {
			t.Fatalf("expected staging root identity error, got: %v", err)
		}
		entries, readErr := os.ReadDir(dest)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".dws-setup-set-") {
				t.Fatalf("staging root must be cleaned after identity read failure: %s", entry.Name())
			}
		}
	})

	t.Run("staging parent mkdir failure", func(t *testing.T) {
		dest := t.TempDir()
		failure := errors.New("mock parent mkdir error")
		testseam.Swap(t, &skillSetupMkdirAll, func(string, os.FileMode) error { return failure })
		_, _, err := stageSkillSetupTarget(
			&skillSetupPlan{Mode: skillSetupModeMulti, Source: t.TempDir(), MultiSkillNames: []string{"dingtalk-a"}},
			skillSetupTargetPlan{Destination: dest},
		)
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "创建 Skill 目标父目录失败") {
			t.Fatalf("expected parent mkdir error, got: %v", err)
		}
	})

	t.Run("staging root temp failure", func(t *testing.T) {
		dest := t.TempDir()
		failure := errors.New("mock root temp error")
		testseam.Swap(t, &skillSetupPublishTemp, func(dir, pattern string) (string, error) {
			if strings.Contains(pattern, ".dws-setup-set-") {
				return "", failure
			}
			return os.MkdirTemp(dir, pattern)
		})
		_, _, err := stageSkillSetupTarget(
			&skillSetupPlan{Mode: skillSetupModeMulti, Source: t.TempDir(), MultiSkillNames: []string{"dingtalk-a"}},
			skillSetupTargetPlan{Destination: dest},
		)
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "创建 Skill staging 失败") {
			t.Fatalf("expected root temp error, got: %v", err)
		}
	})

	t.Run("copy staging identity read failure skips target", func(t *testing.T) {
		home := t.TempDir()
		canonical := filepath.Join(home, ".agents", "skills")
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })
		originalLstat := skillSetupLstat
		testseam.Swap(t, &skillSetupLstat, func(path string) (os.FileInfo, error) {
			if filepath.Base(filepath.Dir(path)) != path && strings.HasPrefix(filepath.Base(filepath.Dir(path)), ".dws-setup-set-") && filepath.Base(path) == "dingtalk-a" {
				return nil, errors.New("mock staged lstat error")
			}
			return originalLstat(path)
		})
		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical}, []string{"dingtalk-a"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil || installed != 0 || skipped != 1 {
			t.Fatalf("execute = installed %d, skipped %d, err %v", installed, skipped, err)
		}
		if !strings.Contains(errOut.String(), "读取 Skill staging 身份失败") {
			t.Fatalf("expected staged identity error in output, got: %s", errOut.String())
		}
		if _, err := os.Stat(filepath.Join(canonical, "dingtalk-a", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("target must not be installed after staged identity failure: %v", err)
		}
	})

	t.Run("link staging identity read failure falls back to copy", func(t *testing.T) {
		home := t.TempDir()
		canonical := filepath.Join(home, ".agents", "skills")
		claude := filepath.Join(home, ".claude", "skills")
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		testseam.Swap(t, &skillSetupGetenv, func(string) string { return "" })
		originalLstat := skillSetupLstat
		testseam.Swap(t, &skillSetupLstat, func(path string) (os.FileInfo, error) {
			if strings.Contains(filepath.Base(path), ".staging-") {
				return nil, errors.New("mock link lstat error")
			}
			return originalLstat(path)
		})
		plan, err := buildSkillSetupPlan(skillSetupModeMulti, src, []string{canonical, claude}, []string{"dingtalk-a"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
		if err != nil || installed != 2 || skipped != 0 {
			t.Fatalf("execute = installed %d, skipped %d, err %v; stdout:\n%s\nstderr:\n%s", installed, skipped, err, out.String(), errOut.String())
		}
		if !strings.Contains(errOut.String(), "自动改用兼容安装") {
			t.Fatalf("expected fallback message, got: %s", errOut.String())
		}
		body, err := os.ReadFile(filepath.Join(claude, "dingtalk-a", "SKILL.md"))
		if err != nil {
			t.Fatalf("fallback copy unreadable: %v", err)
		}
		_ = body
	})

	t.Run("current canonical adapter rejects non-symlink", func(t *testing.T) {
		tempDir := t.TempDir()
		canonical := filepath.Join(tempDir, "canonical")
		if err := os.MkdirAll(canonical, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("chat"), 0o644); err != nil {
			t.Fatal(err)
		}
		nonSymlink := filepath.Join(tempDir, "non-symlink")
		if err := os.MkdirAll(nonSymlink, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nonSymlink, "SKILL.md"), []byte("chat"), 0o644); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillSetupEvalSymlinks, func(string) (string, error) {
			return canonical, nil
		})
		if isSkillSetupCurrentCanonicalAdapter(nonSymlink, canonical) {
			t.Fatal("non-symlink must not be current canonical adapter")
		}
	})
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
