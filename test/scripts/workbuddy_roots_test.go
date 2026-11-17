package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerWorkBuddyShellRootsNormalizeWhitespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell resolvers")
	}
	for _, name := range []string{"install.sh", "install-skills.sh", "install-event.sh", "install-devapp.sh", "release/verify-package-managers.sh"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			custom := filepath.Join(t.TempDir(), "WorkBuddy config")
			function := "resolve_agent_skill_base"
			call := `resolve_agent_skill_base "$HOME" .workbuddy/skills`
			if name == "install-event.sh" || name == "install-devapp.sh" {
				call = `resolve_agent_skill_base .workbuddy/skills`
			} else if name == "release/verify-package-managers.sh" {
				function = "resolve_agent_base"
				call = `resolve_agent_base "$HOME" dws-workbuddy .workbuddy/skills`
			}
			body := extractShellFunction(t, filepath.Join("..", "..", "scripts", filepath.FromSlash(name)), function)
			for _, raw := range []string{"", " \t\r\n", " \t\n" + custom + "\r\n "} {
				want := filepath.Join(home, ".workbuddy", "skills")
				if strings.TrimSpace(raw) != "" {
					want = filepath.Join(custom, "skills")
				}
				cmd := exec.Command("sh", "-eu", "-c", body+"\n"+call)
				cmd.Env = append(os.Environ(), "HOME="+home, "WORKBUDDY_CONFIG_DIR="+raw)
				out, err := cmd.CombinedOutput()
				if err != nil || strings.TrimSpace(string(out)) != want {
					t.Fatalf("root for %q = %q, %v; want %q", raw, out, err, want)
				}
			}
		})
	}
}

func TestInstallPowerShellWorkBuddyRootsNormalizeWhitespace(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil && runtime.GOOS == "windows" {
		pwsh, err = exec.LookPath("powershell")
	}
	if err != nil {
		t.Skip("PowerShell is not available")
	}
	for _, name := range []string{"install.ps1", "install-devapp.ps1"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			custom := filepath.Join(t.TempDir(), "WorkBuddy config")
			text := readTestFile(t, filepath.Join("..", "..", "scripts", name))
			function := "Resolve-AgentSkillBase"
			call := `Resolve-AgentSkillBase -Root $env:DWS_TEST_HOME -Agent ([PSCustomObject]@{Id="dws-workbuddy"; Dir=".workbuddy/skills"})`
			if name == "install-devapp.ps1" {
				function = "Resolve-AgentBase"
				call = `Resolve-AgentBase "dws-workbuddy|0|.workbuddy/skills"`
			}
			body := strings.ReplaceAll(extractPowerShellFunction(t, text, function), "$HOME", "$env:DWS_TEST_HOME")
			harness := filepath.Join(t.TempDir(), "workbuddy-root.ps1")
			mustWriteFile(t, harness, []byte(body+"\n"+call), 0o644)
			for _, raw := range []string{"", " \t\r\n", " \t\n" + custom + "\r\n "} {
				want := filepath.Join(home, ".workbuddy", "skills")
				if strings.TrimSpace(raw) != "" {
					want = filepath.Join(custom, "skills")
				}
				cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness)
				cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home, "WORKBUDDY_CONFIG_DIR="+raw)
				out, err := cmd.CombinedOutput()
				if err != nil || filepath.Clean(strings.TrimSpace(string(out))) != want {
					t.Fatalf("root for %q = %q, %v; want %q", raw, out, err, want)
				}
			}
		})
	}
}
