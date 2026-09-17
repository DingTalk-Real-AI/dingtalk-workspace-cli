package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageCanonicalSkillLayoutMigratesUniversalCopiesAndLinksOtherAgents(t *testing.T) {
	home := withFakeHome(t)
	testseam.Swap(t, &knownSkillDirs, []string{
		".agents/skills", ".codex/skills", ".claude/skills", ".openclaw/skills",
	})

	for _, parent := range []string{".codex", ".claude", ".openclaw"} {
		if err := os.MkdirAll(filepath.Join(home, parent), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// beta.6 wrote a physical copy into Codex. The canonical migration must
	// remove it without treating a matching user/market name as DWS-owned.
	oldCodex := filepath.Join(home, ".codex", "skills", "dingtalk-chat")
	if err := os.MkdirAll(oldCodex, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldCodex, "SKILL.md"), []byte("beta.6"), 0o644); err != nil {
		t.Fatal(err)
	}

	multiRoot := writeMultiBundle(t, t.TempDir(), "dingtalk-chat", "dingtalk-shared")
	result, err := UpgradeSkillLocationsWithOptions(multiRoot, SkillUpgradeOptions{Version: "1.0.58-beta.6"})
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("upgrade = %#v, %v", result, err)
	}

	canonical := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
		if _, err := os.Stat(filepath.Join(canonical, name, "SKILL.md")); err != nil {
			t.Fatalf("canonical %s missing: %v", name, err)
		}
		if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", name)); !os.IsNotExist(err) {
			t.Fatalf("universal Codex duplicate remains for %s: %v", name, err)
		}
		for _, agent := range []string{".claude", ".openclaw"} {
			link := filepath.Join(home, agent, "skills", name)
			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s link = %#v, %v", link, info, err)
			}
			if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
				t.Fatalf("%s does not resolve canonical content: %v", link, err)
			}
		}
	}
	result, err = UpgradeSkillLocationsWithOptions(multiRoot, SkillUpgradeOptions{Version: "1.0.58-beta.6"})
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("idempotent upgrade = %#v, %v", result, err)
	}
	for _, agent := range []string{".claude", ".openclaw"} {
		info, err := os.Lstat(filepath.Join(home, agent, "skills", "dingtalk-chat"))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("idempotent upgrade replaced %s link: %#v, %v", agent, info, err)
		}
	}
}

func TestCrossPlatformCoverageCanonicalCustomRootsSymlinkedParentsAndLexicalVictims(t *testing.T) {
	home := withFakeHome(t)
	customCodex := filepath.Join(t.TempDir(), "codex")
	customHermes := filepath.Join(t.TempDir(), "hermes")
	externalClaude := filepath.Join(t.TempDir(), "claude")
	for _, root := range []string{customCodex, customHermes, externalClaude, filepath.Join(home, ".clawdbot"), filepath.Join(home, ".codeium", "windsurf")} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(externalClaude, filepath.Join(home, ".claude")); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &upgradeGetenv, func(name string) string {
		switch name {
		case "CODEX_HOME":
			return customCodex
		case "HERMES_HOME":
			return customHermes
		default:
			return ""
		}
	})

	oldCodex := filepath.Join(customCodex, "skills", "dingtalk-chat")
	if err := os.MkdirAll(oldCodex, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldCodex, "SKILL.md"), []byte("beta.6"), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeBase := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeBase, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeBase, "dingtalk-chat"), []byte("unexpected file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(claudeBase, "dingtalk-shared")); err != nil {
		t.Fatal(err)
	}

	multiRoot := writeMultiBundle(t, t.TempDir(), "dingtalk-chat", "dingtalk-shared")
	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("upgrade = %#v, %v", result, err)
	}
	if _, err := os.Lstat(oldCodex); !os.IsNotExist(err) {
		t.Fatalf("custom CODEX_HOME duplicate remains: %v", err)
	}
	for _, base := range []string{
		claudeBase,
		filepath.Join(customHermes, "skills"),
		filepath.Join(home, ".clawdbot", "skills"),
		filepath.Join(home, ".codeium", "windsurf", "skills"),
	} {
		for _, name := range []string{"dingtalk-chat", "dingtalk-shared"} {
			path := filepath.Join(base, name)
			info, statErr := os.Lstat(path)
			if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("expected canonical link %s: %#v, %v", path, info, statErr)
			}
			body, readErr := os.ReadFile(filepath.Join(path, "SKILL.md"))
			if readErr != nil || !strings.Contains(string(body), name) {
				t.Fatalf("link %s does not resolve: %q, %v", path, body, readErr)
			}
		}
	}
}

func TestCrossPlatformCoverageCanonicalSkillLinksFallBackToCopies(t *testing.T) {
	home := withFakeHome(t)
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &upgradeSymlink, func(string, string) error { return errors.New("links unavailable") })

	multiRoot := writeMultiBundle(t, t.TempDir(), "dingtalk-chat")
	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("upgrade = %#v, %v", result, err)
	}
	dest := filepath.Join(home, ".claude", "skills", "dingtalk-chat")
	info, err := os.Lstat(dest)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy fallback = %#v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Fatalf("copy fallback content missing: %v", err)
	}
}

func TestCrossPlatformCoverageConfiguredRootsDetectShallowAndApplicationAgents(t *testing.T) {
	// Keep the destination HOME synthetic: app-bundle detection is deliberately
	// machine-scoped and must not depend on the selected installation HOME.
	home := t.TempDir()
	testseam.Swap(t, &upgradeGetenv, func(string) string { return "" })
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
	originalStat := upgradeStat
	testseam.Swap(t, &upgradeStat, func(path string) (os.FileInfo, error) {
		if path == zcodeApp || path == minimaxApp {
			return appInfo, nil
		}
		return originalStat(path)
	})
	want := map[string]bool{
		".config/kimchi/harness/skills": false,
		".tabnine/agent/skills":         false,
		".zcode/skills":                 false,
		".minimax/skills":               false,
	}
	for _, root := range configuredSkillRoots(home) {
		label := filepath.ToSlash(root.label)
		if _, ok := want[label]; ok {
			want[label] = skillRootDetectedBase(root)
		}
	}
	for label, detected := range want {
		if !detected {
			t.Errorf("root %s was not detected", label)
		}
	}
}

func TestCrossPlatformCoverageConfiguredAgentRootsMatchUpstreamAndCustomHomes(t *testing.T) {
	home := t.TempDir()
	custom := map[string]string{
		"AUTOHAND_HOME":     filepath.Join(home, "custom-autohand"),
		"CLAUDE_CONFIG_DIR": filepath.Join(home, "custom-claude"),
		"CODEX_HOME":        filepath.Join(home, "custom-codex"),
		"GROK_HOME":         filepath.Join(home, "custom-grok"),
		"HERMES_HOME":       filepath.Join(home, "custom-hermes"),
		"VIBE_HOME":         filepath.Join(home, "custom-vibe"),
		"XDG_CONFIG_HOME":   filepath.Join(home, "custom-xdg"),
	}
	testseam.Swap(t, &upgradeGetenv, func(name string) string { return custom[name] })
	roots := configuredSkillRoots(home)
	if len(roots) != len(knownSkillDirs) {
		t.Fatalf("configured unique roots = %d, want %d", len(roots), len(knownSkillDirs))
	}
	byLabel := make(map[string]resolvedSkillRoot, len(roots))
	for _, root := range roots {
		if _, duplicate := byLabel[root.label]; duplicate {
			t.Fatalf("duplicate configured label %s", root.label)
		}
		byLabel[root.label] = root
	}
	wants := map[string]string{
		".autohand/skills":              filepath.Join(custom["AUTOHAND_HOME"], "skills"),
		".claude/skills":                filepath.Join(custom["CLAUDE_CONFIG_DIR"], "skills"),
		".codex/skills":                 filepath.Join(custom["CODEX_HOME"], "skills"),
		".grok/skills":                  filepath.Join(custom["GROK_HOME"], "skills"),
		".hermes/skills":                filepath.Join(custom["HERMES_HOME"], "skills"),
		".vibe/skills":                  filepath.Join(custom["VIBE_HOME"], "skills"),
		".config/agents/skills":         filepath.Join(custom["XDG_CONFIG_HOME"], "agents", "skills"),
		".config/crush/skills":          filepath.Join(custom["XDG_CONFIG_HOME"], "crush", "skills"),
		".config/devin/skills":          filepath.Join(custom["XDG_CONFIG_HOME"], "devin", "skills"),
		".config/goose/skills":          filepath.Join(custom["XDG_CONFIG_HOME"], "goose", "skills"),
		".config/kimchi/harness/skills": filepath.Join(custom["XDG_CONFIG_HOME"], "kimchi", "harness", "skills"),
		".config/opencode/skills":       filepath.Join(custom["XDG_CONFIG_HOME"], "opencode", "skills"),
	}
	for label, want := range wants {
		if got := byLabel[label].base; filepath.Clean(got) != filepath.Clean(want) {
			t.Errorf("configured %s = %q, want %q", label, got, want)
		}
	}
	for _, label := range []string{
		".config/agents/skills", ".gemini/antigravity/skills", ".gemini/antigravity-cli/skills",
		".codex/skills", ".cursor/skills", ".deepagents/agent/skills", ".firebender/skills",
		".gemini/skills", ".copilot/skills", ".config/opencode/skills",
	} {
		if !byLabel[label].universal {
			t.Errorf("%s not classified universal", label)
		}
	}
	if !byLabel[".agents/skills"].canonical || byLabel[".claude/skills"].universal {
		t.Fatal("canonical/non-universal classification drift")
	}
}

func TestCrossPlatformCoverageUpgradeCanonicalFailureFailsFastUnconditionally(t *testing.T) {
	for _, mode := range []string{"mono", "multi"} {
		t.Run(mode, func(t *testing.T) {
			withFakeHome(t)
			// Canonical-only configuration (no non-universal dependents): a failed
			// canonical publish must still fail the whole upgrade. Universal agents
			// read canonical directly, so silently succeeding here would leave the
			// machine without any usable Skill while reporting success.
			testseam.Swap(t, &knownSkillDirs, []string{".agents/skills"})
			testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return errors.New("canonical denied") })

			src := t.TempDir()
			if mode == "mono" {
				if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("mono"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				src = writeMultiBundle(t, src, "dingtalk-chat")
			}
			result, err := UpgradeSkillLocations(src)
			if err == nil || !strings.Contains(err.Error(), "canonical Skill 安装失败") {
				t.Fatalf("canonical publish failure must propagate an error, got (%v)", err)
			}
			if result == nil || len(result.Failed()) != 1 || len(result.Succeeded()) != 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeOpenClawAliasPriority(t *testing.T) {
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
			if got := resolveOpenClawSkillBase(home); got != filepath.Join(home, tc.want, "skills") {
				t.Fatalf("OpenClaw root = %q", got)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeLinkStagingFailureEdges(t *testing.T) {
	home := t.TempDir()
	canonical := filepath.Join(home, ".agents", "skills")
	dest := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(canonical, "dingtalk-chat"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("mkdir", func(t *testing.T) {
		testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return errors.New("mkdir denied") })
		if _, _, err := stageLinkedSkillSet(dest, canonical, ".stage-", []string{"dingtalk-chat"}); err == nil || !strings.Contains(err.Error(), "创建 Agent Skill 目录") {
			t.Fatalf("mkdir failure = %v", err)
		}
	})
	t.Run("temp", func(t *testing.T) {
		testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return "", errors.New("temp denied") })
		if _, _, err := stageLinkedSkillSet(dest, canonical, ".stage-", []string{"dingtalk-chat"}); err == nil || !strings.Contains(err.Error(), "链接 staging") {
			t.Fatalf("temp failure = %v", err)
		}
	})
	t.Run("physical-parent", func(t *testing.T) {
		testseam.Swap(t, &upgradeEvalSymlinks, func(path string) (string, error) {
			if path == dest {
				return "", errors.New("physical denied")
			}
			return filepath.EvalSymlinks(path)
		})
		if _, _, err := stageLinkedSkillSet(dest, canonical, ".stage-", []string{"dingtalk-chat"}); err == nil || !strings.Contains(err.Error(), "物理目录") {
			t.Fatalf("physical failure = %v", err)
		}
	})
	t.Run("canonical-target", func(t *testing.T) {
		target := filepath.Join(canonical, "dingtalk-chat")
		testseam.Swap(t, &upgradeEvalSymlinks, func(path string) (string, error) {
			if path == target {
				return "", errors.New("canonical denied")
			}
			return filepath.EvalSymlinks(path)
		})
		if _, _, err := stageLinkedSkillSet(dest, canonical, ".stage-", []string{"dingtalk-chat"}); err == nil || !strings.Contains(err.Error(), "解析 canonical") {
			t.Fatalf("canonical failure = %v", err)
		}
	})
	t.Run("relative-path", func(t *testing.T) {
		testseam.Swap(t, &upgradeRel, func(string, string) (string, error) { return "", errors.New("relative denied") })
		if _, _, err := stageLinkedSkillSet(dest, canonical, ".stage-", []string{"dingtalk-chat"}); err == nil || !strings.Contains(err.Error(), "相对链接") {
			t.Fatalf("relative failure = %v", err)
		}
	})
}

func TestCrossPlatformCoverageUpgradeAgentBranchFailures(t *testing.T) {
	for _, mode := range []string{"mono", "multi"} {
		t.Run(mode, func(t *testing.T) {
			makeSource := func(t *testing.T) string {
				t.Helper()
				src := t.TempDir()
				if mode == "mono" {
					if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("mono"), 0o644); err != nil {
						t.Fatal(err)
					}
					return src
				}
				return writeMultiBundle(t, src, "dingtalk-chat")
			}

			t.Run("undetected-and-physical-alias", func(t *testing.T) {
				home := withFakeHome(t)
				testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills", ".qoder/skills"})
				if err := os.MkdirAll(filepath.Join(home, ".qoder"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".qoder", "skills")); err != nil {
					t.Fatal(err)
				}
				result, err := UpgradeSkillLocations(makeSource(t))
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Results) < 3 || result.Results[1].Status != SkillDirSkipped || result.Results[2].Status != SkillDirSkipped {
					t.Fatalf("skip branches = %#v", result.Results)
				}
			})

			t.Run("universal-retire-failure", func(t *testing.T) {
				home := withFakeHome(t)
				testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".cursor/skills"})
				victimBase := filepath.Join(home, ".cursor", "skills")
				victim := filepath.Join(victimBase, "dws")
				if mode == "multi" {
					victim = filepath.Join(victimBase, "dingtalk-chat")
				}
				if err := os.MkdirAll(victim, 0o755); err != nil {
					t.Fatal(err)
				}
				if mode == "multi" {
					useUpgradeManagedNames(t, "dingtalk-chat")
				}
				origRename := skillPathRenameNoReplace
				testseam.Swap(t, &skillPathRenameNoReplace, func(src, dst string) (string, error) {
					if src == victim {
						return "", errors.New("retire denied")
					}
					return origRename(src, dst)
				})
				result, err := UpgradeSkillLocations(makeSource(t))
				if err != nil {
					t.Fatalf("retire failure must not fail the upgrade: %v", err)
				}
				if len(result.Failed()) != 0 {
					t.Fatalf("retire failure must not count as an install failure: %#v", result.Failed())
				}
				warnings := result.RetireWarnings()
				if len(warnings) != 1 || warnings[0].Err == nil {
					t.Fatalf("retire warnings = %#v", warnings)
				}
				if len(result.Succeeded()) == 0 {
					t.Fatalf("canonical must still be published: %#v", result)
				}
				if _, statErr := os.Lstat(victim); statErr != nil {
					t.Fatalf("unretired copy must be preserved: %v", statErr)
				}
			})

			t.Run("universal-retire-success", func(t *testing.T) {
				home := withFakeHome(t)
				testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".cursor/skills"})
				victimBase := filepath.Join(home, ".cursor", "skills")
				victim := filepath.Join(victimBase, "dws")
				if mode == "multi" {
					victim = filepath.Join(victimBase, "dingtalk-chat")
					useUpgradeManagedNames(t, "dingtalk-chat")
				}
				if err := os.MkdirAll(victim, 0o755); err != nil {
					t.Fatal(err)
				}
				result, err := UpgradeSkillLocations(makeSource(t))
				if err != nil || len(result.Failed()) != 0 {
					t.Fatalf("retire success = %#v, %v", result, err)
				}
				if _, err := os.Lstat(victim); !os.IsNotExist(err) {
					t.Fatalf("universal victim remains: %v", err)
				}
			})

			t.Run("victim-scan-and-fallback-copy-failures", func(t *testing.T) {
				t.Run("victim-scan", func(t *testing.T) {
					home := withFakeHome(t)
					testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
					base := filepath.Join(home, ".claude", "skills")
					if err := os.MkdirAll(base, 0o755); err != nil {
						t.Fatal(err)
					}
					origReadDir := upgradeReadDir
					testseam.Swap(t, &upgradeReadDir, func(path string) ([]os.DirEntry, error) {
						if path == base {
							return nil, errors.New("scan denied")
						}
						return origReadDir(path)
					})
					result, err := UpgradeSkillLocations(makeSource(t))
					if err != nil || len(result.Failed()) != 1 {
						t.Fatalf("victim scan = %#v, %v", result, err)
					}
				})

				t.Run("fallback-copy", func(t *testing.T) {
					home := withFakeHome(t)
					testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
					base := filepath.Join(home, ".claude", "skills")
					if err := os.MkdirAll(base, 0o755); err != nil {
						t.Fatal(err)
					}
					testseam.Swap(t, &upgradeSymlink, func(string, string) error { return errors.New("link denied") })
					origCopy := upgradeCopyDir
					testseam.Swap(t, &upgradeCopyDir, func(src, dst string) error {
						if strings.HasPrefix(filepath.Clean(dst), filepath.Clean(base)+string(filepath.Separator)) {
							return errors.New("copy denied")
						}
						return origCopy(src, dst)
					})
					result, err := UpgradeSkillLocations(makeSource(t))
					if err != nil || len(result.Failed()) != 1 {
						t.Fatalf("fallback copy = %#v, %v", result, err)
					}
				})
			})
		})
	}
}

func TestCrossPlatformCoverageUpgradeWindowsPathNormalizationAndRootDedup(t *testing.T) {
	testseam.Swap(t, &upgradeFoldPathCase, true)
	if got := skillRootPathKey(filepath.Join("Root", "Skills")); got != strings.ToLower(got) {
		t.Fatalf("case-folded path key = %q", got)
	}
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills", ".claude/skills"})
	if roots := configuredSkillRoots(t.TempDir()); len(roots) != 2 {
		t.Fatalf("deduplicated roots = %#v", roots)
	}
}
