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
	".qoder/skills/dws",
	".qoderwork/skills/dws",
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
		if strings.HasPrefix(target, "dws-darwin-") && strings.HasSuffix(target, ".tar.gz") {
			stage := t.TempDir()
			mustWriteFile(t, filepath.Join(stage, "dws"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
			mustRun(t, stage, "tar", "-czf", p, "dws")
			continue
		}
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

func fakeCodesignPath(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	mustWriteFile(t, filepath.Join(binDir, "codesign"), []byte("#!/bin/sh\nset -eu\nexit 0\n"), 0o755)
	return binDir + string(os.PathListSeparator) + os.Getenv("PATH")
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
		"DWS_PACKAGE_VERSION=v1.2.3",
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v1.2.3",
		"PATH="+fakeCodesignPath(t),
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
		"version \"1.2.3\"",
		"skill_home_override = \"\"",
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
		"version \"1.2.3\"",
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

	// Re-running post packaging must replace, not duplicate, the skills checksum.
	cmd = exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_PACKAGE_VERSION=v1.2.3",
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v1.2.3",
		"PATH="+fakeCodesignPath(t),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second post-goreleaser.sh error = %v\noutput:\n%s", err, output)
	}

	// Verify checksums.txt includes exactly one skills zip entry.
	checksumsData, err := os.ReadFile(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("ReadFile(checksums.txt) error = %v", err)
	}
	if count := strings.Count(string(checksumsData), "dws-skills.zip"); count != 1 {
		t.Fatalf("checksums.txt dws-skills.zip count = %d, want 1:\n%s", count, checksumsData)
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
		"DWS_PACKAGE_VERSION=v9.9.9",
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v9.9.9",
		"PATH="+fakeCodesignPath(t),
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
	for _, rel := range append(allArchives, "dws-skills.zip", "checksums.txt") {
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
	// The new layout copies skills/mono/ to staging root + staging/mono/, so we
	// no longer have a single `cd "$ROOT/skills/mono"`. Instead verify the
	// staging-based create_skills_zip references both source trees explicitly.
	for _, want := range []string{
		`cp -R "$ROOT/skills/mono/." "$staging/"`,
		`cp -R "$ROOT/skills/mono/." "$staging/mono/"`,
		`cp -R "$ROOT/skills/multi/." "$staging/multi/"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("post-goreleaser.sh missing skills layout line %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `cd "$ROOT/skills/dws"`) {
		t.Fatalf("post-goreleaser.sh still references legacy nested skills root:\n%s", text)
	}
}

// TestPostGoreleaserSkillsZipLayout exercises create_skills_zip end-to-end:
// runs post-goreleaser.sh against a tempdir, unzips dws-skills.zip, and
// verifies that the new zip layout contains (a) mono content at the root for
// backward compatibility, (b) an explicit mono/ subtree, and (c) a multi/
// subtree carrying per-product skills.
func TestPostGoreleaserSkillsZipLayout(t *testing.T) {
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
	seedDistArtifacts(t, distDir, []string{archiveName})

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_PACKAGE_VERSION=v0.0.0-test",
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL=https://downloads.example.com/dws/releases/v0.0.0",
		"PATH="+fakeCodesignPath(t),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("post-goreleaser.sh error = %v\noutput:\n%s", err, string(output))
	}

	skillsZip := filepath.Join(distDir, "dws-skills.zip")
	extractDir := filepath.Join(root, "skills-extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v", extractDir, err)
	}
	if out, err := exec.Command("unzip", "-q", skillsZip, "-d", extractDir).CombinedOutput(); err != nil {
		t.Fatalf("unzip dws-skills.zip error = %v: %s", err, string(out))
	}

	// Backward compat: zip root must carry mono content.
	for _, rel := range []string{"SKILL.md", "references", "scripts"} {
		if _, err := os.Stat(filepath.Join(extractDir, rel)); err != nil {
			t.Fatalf("zip root missing %s (backward compat broken): %v", rel, err)
		}
	}
	// Explicit mono/ subdir.
	if _, err := os.Stat(filepath.Join(extractDir, "mono", "SKILL.md")); err != nil {
		t.Fatalf("zip missing mono/SKILL.md: %v", err)
	}
	// multi/ subtree with at least one per-product skill.
	multiEntries, err := os.ReadDir(filepath.Join(extractDir, "multi"))
	if err != nil {
		t.Fatalf("ReadDir multi/ error = %v", err)
	}
	if len(multiEntries) == 0 {
		t.Fatalf("multi/ is empty; expected per-product skills")
	}
	foundDingtalk := false
	for _, e := range multiEntries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "dingtalk-") {
			foundDingtalk = true
			skillFile := filepath.Join(extractDir, "multi", e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				t.Fatalf("missing %s: %v", skillFile, err)
			}
			break
		}
	}
	if !foundDingtalk {
		t.Fatalf("multi/ does not contain any dingtalk-* skill: %v", multiEntries)
	}
}
