package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

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
