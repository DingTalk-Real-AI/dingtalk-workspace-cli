package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Unicode White_Space is the shared Go / npm / PowerShell / Shell contract.
const workbuddyWhitespace = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

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
			script := filepath.Join("..", "..", "scripts", filepath.FromSlash(name))
			body := extractShellFunction(t, script, "trim_workbuddy_config_dir") + "\n" + extractShellFunction(t, script, function)
			for _, raw := range []string{"", workbuddyWhitespace, workbuddyWhitespace + custom + workbuddyWhitespace, "\u00a0" + custom + "\u00a0"} {
				want := filepath.Join(home, ".workbuddy", "skills")
				if strings.TrimSpace(raw) != "" {
					want = filepath.Join(custom, "skills")
				}
				cmd := exec.Command("sh", "-eu", "-c", body+"\n"+call)
				cmd.Env = append(os.Environ(), "HOME="+home, "WORKBUDDY_CONFIG_DIR="+raw, "LC_ALL=C")
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
			for _, raw := range []string{"", workbuddyWhitespace, workbuddyWhitespace + custom + workbuddyWhitespace, "\u00a0" + custom + "\u00a0"} {
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
