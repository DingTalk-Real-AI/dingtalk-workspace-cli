package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests exercise the installer main flow with an authentic, checksummed
// but unusable release archive. Network requests are served by local fixtures.
func TestInstallScriptMissingSkillsFailsWithoutSuccessBanner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"mono", "multi"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "scripts", "install.sh")
			mustWriteFile(t, script, data, 0o755)
			release := filepath.Join(root, "release")
			files := map[string]string{"README.md": "No usable skills\n"}
			if mode == "mono" {
				files["multi/dingtalk-chat/SKILL.md"] = "# Multi only\n"
			}
			writeZip(t, filepath.Join(release, "dws-skills.zip"), files)
			writeInstallerFixtureChecksums(t, release)
			stubs := filepath.Join(root, "stubs")
			writeFakeCurl(t, filepath.Join(stubs, "curl"))
			fixture := &installSourceFixture{root: root, stubRoot: stubs, fakeHome: filepath.Join(root, "home")}
			cmd := exec.Command("sh", script)
			cmd.Env = fixture.envWithSkillMode(mode,
				"DWS_VERSION=v0.0.0-test", "DWS_SKILLS_ONLY=1", "DWS_NO_SKILLS=0",
				"DWS_NO_FALLBACK=1", "DWS_GITEE_REPO=", "FAKE_ASSET_NAME=unused",
				"FAKE_RELEASE_DIR="+release,
			)
			output, runErr := cmd.CombinedOutput()
			if runErr == nil {
				t.Fatalf("unusable skills must fail:\n%s", output)
			}
			for _, want := range []string{"SHA256 checksum verified", "Skills were NOT installed."} {
				if !strings.Contains(string(output), want) {
					t.Fatalf("missing %q:\n%s", want, output)
				}
			}
			if strings.Contains(string(output), "Installation complete!") {
				t.Fatalf("failed installer printed success:\n%s", output)
			}
			if _, err := os.Stat(filepath.Join(fixture.fakeHome, ".agents", "skills", "dws")); !os.IsNotExist(err) {
				t.Fatalf("failed installer created a Skill directory: %v", err)
			}
		})
	}
}

func TestInstallPowerShellMissingSkillsFailsWithoutSuccessBanner(t *testing.T) {
	pwsh := lookPowerShellForScripts(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	// Preserve the actual main flow, redirect HOME references to the fixture,
	// and replace source discovery and HTTP transport with deterministic stubs.
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	stubs := `
function Resolve-SourceRoot { return $null }
function Resolve-Source {}
function Invoke-WebRequest {
    [CmdletBinding()]
    param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)
    $name = ([uri]$Uri).Segments[-1]
    if ($name -ne 'dws-skills.zip' -and $name -ne 'checksums.txt') {
        throw "Unexpected fixture URL: $Uri"
    }
    Copy-Item -LiteralPath (Join-Path $env:DWS_TEST_RELEASE $name) -Destination $OutFile
}
function Invoke-RestMethod { throw 'Network access is forbidden in this test' }
`
	for _, scenario := range []string{"mono", "multi", "local-missing"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			release := filepath.Join(root, "release")
			files := map[string]string{"README.md": "No usable skills\n"}
			if scenario == "mono" {
				files["multi/dingtalk-chat/SKILL.md"] = "# Multi only\n"
			}
			writeZip(t, filepath.Join(release, "dws-skills.zip"), files)
			writeInstallerFixtureChecksums(t, release)
			mode := scenario
			override := ""
			if scenario == "local-missing" {
				mode = "mono"
				override = "\nfunction Install-Skills { Install-SkillsLocal -Root $env:DWS_TEST_SOURCE }\n"
			}
			harness := filepath.Join(root, "harness.ps1")
			// Windows PowerShell 5.1 needs a UTF-8 BOM for the script's emoji.
			mustWriteFile(t, harness, []byte("\xef\xbb\xbf"+prefix+stubs+override+string(data[cut:])), 0o644)
			cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness)
			var env []string
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "DWS_") {
					env = append(env, kv)
				}
			}
			cmd.Env = append(env, "DWS_VERSION=v0.0.0-test", "DWS_SKILLS_ONLY=1", "DWS_SKILL_MODE="+mode,
				"DWS_TEST_HOME="+home, "DWS_TEST_RELEASE="+release, "DWS_TEST_SOURCE="+filepath.Join(root, "missing-source"))
			output, runErr := cmd.CombinedOutput()
			if runErr == nil || !strings.Contains(string(output), "Skills were NOT installed.") {
				t.Fatalf("missing skills must fail explicitly: %v\n%s", runErr, output)
			}
			if scenario != "local-missing" && !strings.Contains(string(output), "SHA256 checksum verified") {
				t.Fatalf("fixture did not reach the checked archive path:\n%s", output)
			}
			if strings.Contains(string(output), "Installation complete!") {
				t.Fatalf("failed installer printed success:\n%s", output)
			}
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws")); !os.IsNotExist(err) {
				t.Fatalf("failed installer created a Skill directory: %v", err)
			}
		})
	}
}
