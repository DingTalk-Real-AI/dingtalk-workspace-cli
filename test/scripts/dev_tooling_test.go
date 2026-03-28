package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakefileIncludesHookSetupAndBuildAllTargets(t *testing.T) {
	t.Parallel()

	makefilePath, err := filepath.Abs(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("Abs(Makefile) error = %v", err)
	}

	data, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", makefilePath, err)
	}

	text := string(data)
	for _, want := range []string{
		".PHONY: all help build rebuild test lint fmt policy package release publish-homebrew-formula build-all setup-hooks",
		"all: setup-hooks fmt lint build test rebuild",
		"build-all:",
		"setup-hooks:",
		"git config core.hooksPath scripts/hooks",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing %q:\n%s", want, text)
		}
	}
}

func TestToolingScriptsExistAndParse(t *testing.T) {
	t.Parallel()

	buildAllPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "build-all.sh"))
	if err != nil {
		t.Fatalf("Abs(build-all.sh) error = %v", err)
	}
	hookPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "hooks", "pre-commit"))
	if err != nil {
		t.Fatalf("Abs(pre-commit) error = %v", err)
	}

	for _, full := range []string{buildAllPath, hookPath} {
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("Stat(%s) error = %v", full, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s is not executable", full)
		}
	}

	if output, err := exec.Command("bash", "-n", buildAllPath).CombinedOutput(); err != nil {
		t.Fatalf("bash -n build-all.sh error = %v\noutput:\n%s", err, string(output))
	}
	if output, err := exec.Command("sh", "-n", hookPath).CombinedOutput(); err != nil {
		t.Fatalf("sh -n pre-commit error = %v\noutput:\n%s", err, string(output))
	}
}

func TestCommandSurfaceCheckDoesNotReferenceDWSPIN(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "policy", "check-command-surface.sh"))
	if err != nil {
		t.Fatalf("Abs(check-command-surface.sh) error = %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}

	text := string(data)
	for _, forbidden := range []string{"DWS_PIN=", "DWS_SURFACE_CHECK_PIN", "PIN="} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("check-command-surface.sh still references %q:\n%s", forbidden, text)
		}
	}
}

func TestRunMCPProbeScriptSupportsTruthSourceBinary(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "run-mcp-probe.sh"))
	if err != nil {
		t.Fatalf("Abs(run-mcp-probe.sh) error = %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}

	text := string(data)
	for _, want := range []string{
		"--truth-dws PATH",
		"TRUTH_DWS_BINARY",
		"ARGS+=(\"--truth-dws\" \"$TRUTH_DWS_BINARY\")",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("run-mcp-probe.sh missing %q:\n%s", want, text)
		}
	}
}

func TestRunMCPProbeScriptSupportsServersJSONDiscoveryOverride(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "run-mcp-probe.sh"))
	if err != nil {
		t.Fatalf("Abs(run-mcp-probe.sh) error = %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}

	text := string(data)
	for _, want := range []string{
		"--servers-json PATH",
		"SERVERS_JSON_PATH",
		"DWS_DISCOVERY_BASE_URL",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("run-mcp-probe.sh missing %q:\n%s", want, text)
		}
	}
}
