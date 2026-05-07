package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var expectedPackagedSkillTargets = []string{
	".agents/skills/dws",
	".claude/skills/dws",
	".cursor/skills/dws",
	".gemini/skills/dws",
	".codex/skills/dws",
	".github/skills/dws",
	".windsurf/skills/dws",
	".augment/skills/dws",
	".cline/skills/dws",
	".amp/skills/dws",
	".kiro/skills/dws",
	".trae/skills/dws",
	".openclaw/skills/dws",
	".hermes/skills/dws",
}

// seedDistArtifacts creates fake goreleaser output archives (empty tar.gz/zip
// files) and a checksums.txt stub so that post-goreleaser.sh can run without
// an actual goreleaser build.
func seedDistArtifacts(t *testing.T, distDir string, targets []string) {
	t.Helper()
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", distDir, err)
	}

	for _, target := range targets {
		p := filepath.Join(distDir, target)
		if err := os.WriteFile(p, []byte("fake-archive"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", p, err)
		}
	}

	// Create empty checksums.txt (goreleaser creates this)
	checksums := filepath.Join(distDir, "checksums.txt")
	var lines []string
	for _, target := range targets {
		lines = append(lines, "deadbeef00000000000000000000000000000000000000000000000000000000  "+target)
	}
	if err := os.WriteFile(checksums, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", checksums, err)
	}
}

func TestPostGoreleaserBuildsExpectedArtifacts(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "post-goreleaser.sh"))
	if err != nil {
		t.Fatalf("Abs(post-goreleaser.sh) error = %v", err)
	}

	root := t.TempDir()
	distDir := filepath.Join(root, "dist")

	hostOS := runtime.GOOS
	hostArch := runtime.GOARCH
	archiveName := "dws-" + hostOS + "-" + hostArch + ".tar.gz"
	if hostOS == "windows" {
		archiveName = "dws-" + hostOS + "-" + hostArch + ".zip"
	}

	// Seed dist/ with fake goreleaser archives (simulate goreleaser output)
	seedDistArtifacts(t, distDir, []string{archiveName})

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v1.2.3",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("post-goreleaser.sh error = %v\noutput:\n%s", err, string(output))
	}

	for _, rel := range []string{
		"dws-skills.zip",
		"checksums.txt",
		filepath.Join("npm", "dingtalk-workspace-cli", "package.json"),
		filepath.Join("homebrew", "dingtalk-workspace-cli.rb"),
		filepath.Join("homebrew", "dingtalk-workspace-cli-local.rb"),
	} {
		full := filepath.Join(distDir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("Stat(%s) error = %v\noutput:\n%s", full, err, string(output))
		}
	}

	formulaPath := filepath.Join(distDir, "homebrew", "dingtalk-workspace-cli-local.rb")
	formulaData, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", formulaPath, err)
	}
	formulaText := string(formulaData)
	for _, want := range []string{
		"class DingtalkWorkspaceCliLocal < Formula",
		"resource \"skills\" do",
		"DingTalk Workspace CLI",
	} {
		if !strings.Contains(formulaText, want) {
			t.Fatalf("formula missing %q:\n%s", want, formulaText)
		}
	}

	releaseFormulaPath := filepath.Join(distDir, "homebrew", "dingtalk-workspace-cli.rb")
	releaseFormulaData, err := os.ReadFile(releaseFormulaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", releaseFormulaPath, err)
	}
	releaseFormulaText := string(releaseFormulaData)
	for _, want := range []string{
		"class DingtalkWorkspaceCli < Formula",
		"https://downloads.example.com/dws/releases/v1.2.3/" + archiveName,
		"https://downloads.example.com/dws/releases/v1.2.3/dws-skills.zip",
	} {
		if !strings.Contains(releaseFormulaText, want) {
			t.Fatalf("release formula missing %q:\n%s", want, releaseFormulaText)
		}
	}

	packageJSONPath := filepath.Join(distDir, "npm", "dingtalk-workspace-cli", "package.json")
	packageJSON, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", packageJSONPath, err)
	}
	for _, want := range []string{
		"\"name\": \"dingtalk-workspace-cli\"",
		"DingTalk Workspace CLI",
		"\"postinstall\": \"node install.js\"",
	} {
		if !strings.Contains(string(packageJSON), want) {
			t.Fatalf("package.json missing %q:\n%s", want, string(packageJSON))
		}
	}

	npmInstallPath := filepath.Join(distDir, "npm", "dingtalk-workspace-cli", "install.js")
	npmInstallData, err := os.ReadFile(npmInstallPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", npmInstallPath, err)
	}
	npmInstallText := string(npmInstallData)
	for _, target := range expectedPackagedSkillTargets {
		agentDir := strings.TrimSuffix(target, "/dws")
		if !strings.Contains(npmInstallText, agentDir) {
			t.Fatalf("npm install.js missing %q:\n%s", agentDir, npmInstallText)
		}
	}

	for _, target := range expectedPackagedSkillTargets {
		if !strings.Contains(releaseFormulaText, target) {
			t.Fatalf("release formula missing %q:\n%s", target, releaseFormulaText)
		}
	}

	// Verify checksums.txt was updated to include skills zip
	checksumsData, err := os.ReadFile(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("ReadFile(checksums.txt) error = %v", err)
	}
	if !strings.Contains(string(checksumsData), "dws-skills.zip") {
		t.Fatalf("checksums.txt missing dws-skills.zip entry:\n%s", string(checksumsData))
	}
}

func TestPostGoreleaserAllPlatformNpmAssets(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "post-goreleaser.sh"))
	if err != nil {
		t.Fatalf("Abs(post-goreleaser.sh) error = %v", err)
	}

	root := t.TempDir()
	distDir := filepath.Join(root, "dist")

	allArchives := []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
		"dws-windows-amd64.zip",
		"dws-windows-arm64.zip",
	}

	// Seed dist/ with all platform archives (simulate goreleaser --target all)
	seedDistArtifacts(t, distDir, allArchives)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v9.9.9",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("post-goreleaser.sh error = %v\noutput:\n%s", err, string(output))
	}

	for _, rel := range append(allArchives, "dws-skills.zip", "checksums.txt") {
		full := filepath.Join(distDir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("Stat(%s) error = %v\noutput:\n%s", full, err, string(output))
		}
	}

	packageAssetsDir := filepath.Join(distDir, "npm", "dingtalk-workspace-cli", "assets")
	for _, rel := range append(allArchives, "dws-skills.zip") {
		if _, err := os.Stat(filepath.Join(packageAssetsDir, rel)); err != nil {
			t.Fatalf("npm asset missing %q: %v", rel, err)
		}
	}
}

func TestPostGoreleaserUsesFlattenedSkillsSourceRoot(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "post-goreleaser.sh"))
	if err != nil {
		t.Fatalf("Abs(post-goreleaser.sh) error = %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}

	text := string(data)
	if !strings.Contains(text, `cd "$ROOT/skills"`) {
		t.Fatalf("post-goreleaser.sh missing flattened skills source root:\n%s", text)
	}
	if strings.Contains(text, `cd "$ROOT/skills/dws"`) {
		t.Fatalf("post-goreleaser.sh still references legacy nested skills root:\n%s", text)
	}
}
