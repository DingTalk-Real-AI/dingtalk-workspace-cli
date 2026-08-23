package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type packagedAgent struct {
	universal bool
	path      string
}

var upstreamAgentIDs = strings.Fields(`
aider-desk amp antigravity antigravity-cli astrbot autohand-code augment bob
claude-code openclaw cline codearts-agent codebuddy codemaker codestudio codex
command-code continue cortex crush cursor deepagents devin dexto droid eve
firebender forgecode gemini-cli github-copilot goose grok hermes-agent inference-sh
jazz junie iflow-cli kilo kimchi kimi-code-cli kiro-cli kode lingma loaf mcpjam
minimax-code mistral-vibe moxby mux opencode openhands ona pi qoder qoder-cn
qwen-code replit reasonix rovodev roo tabnine-cli terramind tinycloud trae trae-cn
warp windsurf zed zcode zencoder zenflow neovate pochi promptscript adal universal
`)

func TestAgentRegistriesMatchUpstream76(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	installPS := readTestFile(t, filepath.Join(root, "scripts", "install.ps1"))
	devappPS := readTestFile(t, filepath.Join(root, "scripts", "install-devapp.ps1"))
	verifier := readTestFile(t, filepath.Join(root, "scripts", "release", "verify-package-managers.sh"))

	wantIDs := append([]string(nil), upstreamAgentIDs...)
	sort.Strings(wantIDs)
	registries := map[string]map[string]packagedAgent{
		"install.ps1":        parseInstallPowerShellRegistry(t, installPS),
		"install-devapp.ps1": parsePipeRegistry(t, between(t, devappPS, "$AgentRegistryRows = @(", ")\n$LegacyAgentCleanupRows")),
		"verifier":           parsePipeRegistry(t, between(t, verifier, "UPSTREAM_AGENT_REGISTRY='", "'\n\n# DWS-only")),
	}

	for name, registry := range registries {
		gotIDs := make([]string, 0, len(registry))
		universal := 0
		for id, agent := range registry {
			gotIDs = append(gotIDs, id)
			if agent.universal {
				universal++
			}
		}
		sort.Strings(gotIDs)
		if strings.Join(gotIDs, "\n") != strings.Join(wantIDs, "\n") {
			t.Fatalf("%s agent IDs differ from upstream 76\ngot:  %v\nwant: %v", name, gotIDs, wantIDs)
		}
		if universal != 19 || len(registry)-universal != 57 {
			t.Fatalf("%s classification = %d universal/%d non-universal, want 19/57", name, universal, len(registry)-universal)
		}
	}

	canonical := registries["install.ps1"]
	for name, got := range registries {
		for id, want := range canonical {
			if got[id] != want {
				t.Errorf("%s[%s] = %+v, want %+v", name, id, got[id], want)
			}
		}
	}

	for id, wantPath := range map[string]string{
		"amp":            ".config/agents/skills",
		"github-copilot": ".copilot/skills",
		"opencode":       ".config/opencode/skills",
		"windsurf":       ".codeium/windsurf/skills",
		"cline":          ".agents/skills",
		"eve":            "-",
		"promptscript":   "-",
	} {
		if got := canonical[id].path; got != wantPath {
			t.Errorf("upstream path for %s = %q, want %q", id, got, wantPath)
		}
	}
}

func TestDWSCompatibilityAgentTargets(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	installPS := readTestFile(t, filepath.Join(root, "scripts", "install.ps1"))
	legacyBlock := between(t, installPS, "$LegacyAgentCleanupTargets = @(", ")\n\n# Kept as")
	if !strings.Contains(legacyBlock, `Id = "dws-qoderwork"; Universal = $false`) {
		t.Error("DWS Qoderwork compatibility target must remain non-universal")
	}
	for _, id := range []string{
		"dws-legacy-github", "dws-legacy-amp", "dws-legacy-cline", "dws-legacy-windsurf",
	} {
		if !strings.Contains(legacyBlock, `Id = "`+id+`"; Universal = $true`) {
			t.Errorf("legacy cleanup registry missing cleanup-only target %s", id)
		}
	}
	for _, id := range upstreamAgentIDs {
		if strings.HasPrefix(id, "dws-") {
			t.Fatalf("DWS compatibility target %s leaked into upstream registry", id)
		}
	}
}

func TestInstallPowerShellUsesAbsoluteJunctionTargetsAndCopyFallback(t *testing.T) {
	t.Parallel()
	installPS := readTestFile(t, filepath.Join("..", "..", "scripts", "install.ps1"))
	for _, want := range []string{
		`$absoluteTarget = [System.IO.Path]::GetFullPath((Join-Path $canonical $name))`,
		`New-Item -ItemType Junction -Path (Join-Path $stageRoot $name) -Target $absoluteTarget`,
		`Install-MultiToBase -MultiSrc $MultiSrc -BaseDir $baseDir`,
		`Install-MonoToBase -SkillSrc $SkillSrc -BaseDir $baseDir`,
	} {
		if !strings.Contains(installPS, want) {
			t.Errorf("install.ps1 missing Windows junction/fallback contract %q", want)
		}
	}
}

func TestAgentInstallersCarryUpstreamShallowAndApplicationDetection(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"build/npm/install.js":       {"agentTargetDetected", `case "kimchi"`, `case "tabnine-cli"`, "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install.sh":         {"agent_skill_base_detected", ".config/kimchi/harness/skills", ".tabnine/agent/skills", "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install-skills.sh":  {"agent_skill_base_detected", ".config/kimchi/harness/skills", ".tabnine/agent/skills", "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install-event.sh":   {"agent_skill_base_detected", ".config/kimchi/harness/skills", ".tabnine/agent/skills", "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install-devapp.sh":  {"agent_skill_base_detected", ".config/kimchi/harness/skills", ".tabnine/agent/skills", "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install.ps1":        {"Test-AgentSkillBaseDetected", `"kimchi"`, `"tabnine-cli"`, "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
		"scripts/install-devapp.ps1": {"Test-AgentBaseDetected", `"kimchi"`, `"tabnine-cli"`, "/Applications/ZCode.app", "/Applications/MiniMax Code.app"},
	}
	for rel, wants := range checks {
		body := readTestFile(t, filepath.Join("..", "..", filepath.FromSlash(rel)))
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing detection contract %q", rel, want)
			}
		}
	}
}

func TestInstallPowerShellResolvesCustomAndLegacyAgentHomes(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}

	root := filepath.Join("..", "..")
	installPS := readTestFile(t, filepath.Join(root, "scripts", "install.ps1"))
	cut := strings.LastIndex(installPS, "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "custom roots")
	xdg := filepath.Join(custom, "xdg")
	clawdbot := filepath.Join(home, ".clawdbot")
	if err := os.MkdirAll(clawdbot, 0o755); err != nil {
		t.Fatal(err)
	}

	prefix := strings.ReplaceAll(installPS[:cut], "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
$ids = @("amp", "autohand-code", "claude-code", "codex", "grok", "hermes-agent", "mistral-vibe", "openclaw", "goose", "kimchi", "opencode")
foreach ($id in $ids) {
    $agent = $AgentRegistry | Where-Object { $_.Id -eq $id } | Select-Object -First 1
    Write-Output ($id + "|" + (Resolve-AgentSkillBase -Root $env:DWS_TEST_HOME -Agent $agent))
}
exit 0
`
	harness := filepath.Join(t.TempDir(), "resolve-agent-roots.ps1")
	mustWriteFile(t, harness, []byte(prefix), 0o644)

	envRoots := map[string]string{
		"AUTOHAND_HOME":     filepath.Join(custom, "autohand"),
		"CLAUDE_CONFIG_DIR": filepath.Join(custom, "claude"),
		"CODEX_HOME":        filepath.Join(custom, "codex"),
		"GROK_HOME":         filepath.Join(custom, "grok"),
		"HERMES_HOME":       filepath.Join(custom, "hermes"),
		"VIBE_HOME":         filepath.Join(custom, "vibe"),
		"XDG_CONFIG_HOME":   xdg,
	}
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	for key, value := range envRoots {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell root resolver failed: %v\n%s", err, output)
	}
	want := map[string]string{
		"amp":           filepath.Join(xdg, "agents", "skills"),
		"autohand-code": filepath.Join(envRoots["AUTOHAND_HOME"], "skills"),
		"claude-code":   filepath.Join(envRoots["CLAUDE_CONFIG_DIR"], "skills"),
		"codex":         filepath.Join(envRoots["CODEX_HOME"], "skills"),
		"grok":          filepath.Join(envRoots["GROK_HOME"], "skills"),
		"hermes-agent":  filepath.Join(envRoots["HERMES_HOME"], "skills"),
		"mistral-vibe":  filepath.Join(envRoots["VIBE_HOME"], "skills"),
		"openclaw":      filepath.Join(clawdbot, "skills"),
		"goose":         filepath.Join(xdg, "goose", "skills"),
		"kimchi":        filepath.Join(xdg, "kimchi", "harness", "skills"),
		"opencode":      filepath.Join(xdg, "opencode", "skills"),
	}
	got := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) == 2 {
			got[parts[0]] = filepath.Clean(parts[1])
		}
	}
	for id, path := range want {
		if got[id] != filepath.Clean(path) {
			t.Errorf("resolved %s = %q, want %q\noutput:\n%s", id, got[id], path, output)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

func between(t *testing.T, text, start, end string) string {
	t.Helper()
	i := strings.Index(text, start)
	if i < 0 {
		t.Fatalf("start marker %q missing", start)
	}
	i += len(start)
	j := strings.Index(text[i:], end)
	if j < 0 {
		t.Fatalf("end marker %q missing", end)
	}
	return text[i : i+j]
}

func parseInstallPowerShellRegistry(t *testing.T, text string) map[string]packagedAgent {
	t.Helper()
	block := between(t, text, "$AgentRegistry = @(", ")\n\n# DWS compatibility targets")
	re := regexp.MustCompile(`Id = "([^"]+)"; Universal = \$(true|false); Dir = (?:"([^"]+)"|\$null)`)
	result := make(map[string]packagedAgent)
	for _, match := range re.FindAllStringSubmatch(block, -1) {
		path := strings.ReplaceAll(match[3], `\`, "/")
		if path == "" {
			path = "-"
		}
		result[match[1]] = packagedAgent{universal: match[2] == "true", path: path}
	}
	return result
}

func parsePipeRegistry(t *testing.T, block string) map[string]packagedAgent {
	t.Helper()
	re := regexp.MustCompile(`([a-z0-9-]+)\|([01])\|([.a-z0-9_\\/-]+)`)
	result := make(map[string]packagedAgent)
	for _, match := range re.FindAllStringSubmatch(block, -1) {
		path := strings.ReplaceAll(match[3], `\`, "/")
		if _, exists := result[match[1]]; exists {
			t.Fatalf("duplicate agent %q in pipe registry", match[1])
		}
		result[match[1]] = packagedAgent{universal: match[2] == "1", path: path}
	}
	return result
}
