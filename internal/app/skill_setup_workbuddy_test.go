package app

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageWorkBuddyCustomRootWithUniversalSuffix(t *testing.T) {
	for _, suffix := range []string{".cursor", ".codex", ".config/opencode", ".agents"} {
		for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti} {
			for _, target := range []string{"workbuddy", "all"} {
				t.Run(suffix+"/"+mode+"/"+target, func(t *testing.T) {
					home := t.TempDir()
					config := filepath.Join(t.TempDir(), "profile", filepath.FromSlash(suffix))
					if err := os.MkdirAll(config, 0o755); err != nil {
						t.Fatal(err)
					}
					testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
					testseam.Swap(t, &skillSetupGetenv, func(key string) string {
						if key == "WORKBUDDY_CONFIG_DIR" {
							return config
						}
						return ""
					})
					testseam.Swap(t, &skillSetupAgentHomes, []string{".agents/skills", ".workbuddy/skills"})
					dests, err := resolveSkillSetupTargets(target, mode)
					if err != nil {
						t.Fatal(err)
					}
					src := t.TempDir()
					var names []string
					if mode == skillSetupModeMulti {
						names = []string{"dingtalk-chat"}
						if err := os.MkdirAll(filepath.Join(src, names[0]), 0o755); err != nil {
							t.Fatal(err)
						}
					}
					leaf := ""
					if len(names) > 0 {
						leaf = names[0]
					}
					if err := os.WriteFile(filepath.Join(src, leaf, "SKILL.md"), []byte("new skill"), 0o644); err != nil {
						t.Fatal(err)
					}
					dest := agentHomeForMode(filepath.Join(config, "skills"), mode)
					if err := os.MkdirAll(filepath.Join(dest, leaf), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dest, leaf, "SKILL.md"), []byte("old skill"), 0o644); err != nil {
						t.Fatal(err)
					}
					plan, err := buildSkillSetupPlan(mode, src, dests, names, false)
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, entry := range plan.Targets {
						if sameSkillSetupPath(entry.Destination, dest) {
							found = entry.LinkCanonical && !entry.CleanupOnly
						}
					}
					if !found {
						t.Fatalf("WorkBuddy must receive an adapter, plan = %#v", plan.Targets)
					}
					if _, _, err := executeSkillSetupPlan(plan, io.Discard, io.Discard); err != nil {
						t.Fatal(err)
					}
					if body, err := os.ReadFile(filepath.Join(dest, leaf, "SKILL.md")); err != nil || string(body) != "new skill" {
						t.Fatalf("WorkBuddy Skill was retired instead of installed: %q, %v", body, err)
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageWorkBuddySkillTargetsWithoutConnector(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "default"
		if custom {
			name = "custom"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, ".workbuddy")
			override := " \t"
			if custom {
				config = filepath.Join(t.TempDir(), "WorkBuddy config")
				override = " " + config + " "
			}
			testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
			testseam.Swap(t, &skillUserHomeDir, func() (string, error) { return home, nil })
			testseam.Swap(t, &skillSetupGetenv, func(key string) string {
				if key == "WORKBUDDY_CONFIG_DIR" {
					return override
				}
				return ""
			})
			testseam.Swap(t, &skillSetupAgentHomes, []string{".agents/skills", ".workbuddy/skills"})
			base := filepath.Join(config, "skills")
			if got, err := resolveSkillTargetPath("WorkBuddy"); err != nil || got != base {
				t.Fatalf("marketplace target = %q, %v; want %q", got, err, base)
			}
			if isUniversalSkillSetupBase(base) {
				t.Fatal("WorkBuddy must receive a client adapter")
			}
			for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti} {
				want := agentHomeForMode(base, mode)
				// Explicit targeting must work before the client config exists.
				dests, err := resolveSkillSetupTargets("workbuddy", mode)
				if err != nil || len(dests) != 2 || dests[1] != want {
					t.Fatalf("explicit %s targets = %v, %v; want canonical + %s", mode, dests, err, want)
				}
				if containsSkillName(detectExistingAgentHomes(home, mode), want) {
					t.Fatalf("absent WorkBuddy detected for %s", mode)
				}
			}
			// A plain config root is enough; no connector or plugin is installed.
			if err := os.MkdirAll(config, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti} {
				want := agentHomeForMode(base, mode)
				if !containsSkillName(detectExistingAgentHomes(home, mode), want) {
					t.Fatalf("existing WorkBuddy not detected for %s: %s", mode, want)
				}
			}
		})
	}
}
