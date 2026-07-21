package scripts_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
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

var expectedReleaseAdmissionContexts = []string{
	"Lint",
	"Test",
	"Coverage",
	"Policy",
	"Edition",
	"Interface Integrity",
	"AI Behavior",
	"CLI Smoke",
	"Mock MCP",
}

func seedDistArchive(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", path, err)
	}
	defer file.Close()

	content := []byte("#!/bin/sh\nexit 0\n")
	switch {
	case strings.HasSuffix(path, ".tar.gz"):
		gzipWriter := gzip.NewWriter(file)
		tarWriter := tar.NewWriter(gzipWriter)
		if err := tarWriter.WriteHeader(&tar.Header{Name: "dws", Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatalf("WriteHeader(%s) error = %v", path, err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("Write(%s) error = %v", path, err)
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatalf("Close tar(%s) error = %v", path, err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatalf("Close gzip(%s) error = %v", path, err)
		}
	case strings.HasSuffix(path, ".zip"):
		zipWriter := zip.NewWriter(file)
		header := &zip.FileHeader{Name: "dws.exe", Method: zip.Store}
		header.SetMode(0o755)
		entry, err := zipWriter.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader(%s) error = %v", path, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("Write(%s) error = %v", path, err)
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatalf("Close zip(%s) error = %v", path, err)
		}
	default:
		t.Fatalf("unsupported archive path %s", path)
	}
}

// seedDistArtifacts creates minimal goreleaser output archives and a
// checksums.txt stub so post-goreleaser.sh can run without a real build.
// Every archive is valid so the packaging tests exercise extraction for all
// platforms; Darwin archives are additionally processed by the signing path.
func seedDistArtifacts(t *testing.T, distDir string, targets []string) {
	t.Helper()
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", distDir, err)
	}

	for _, target := range targets {
		p := filepath.Join(distDir, target)
		seedDistArchive(t, p)
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

func postGoreleaserEnv(t *testing.T, distDir, version, releaseBaseURL string) []string {
	t.Helper()

	binDir := t.TempDir()
	fakeCodesign := filepath.Join(binDir, "codesign")
	if err := os.WriteFile(fakeCodesign, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake codesign) error = %v", err)
	}

	return append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DWS_PACKAGE_VERSION="+version,
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_RELEASE_BASE_URL="+releaseBaseURL,
	)
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

	// Seed every archive referenced by the public multi-platform Homebrew formula.
	// The local verification formula still selects the current host archive.
	targets := []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
	}
	foundHost := false
	for _, target := range targets {
		if target == archiveName {
			foundHost = true
			break
		}
	}
	if !foundHost {
		targets = append(targets, archiveName)
	}
	seedDistArtifacts(t, distDir, targets)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = postGoreleaserEnv(t, distDir, "v1.2.3", "https://downloads.example.com/dws/releases/v1.2.3")
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
		"Install locally built DingTalk workspace CLI artifacts for verification",
		"Agent Skills are bundled in #{pkgshare}/skills/dws",
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
		`desc "Automate DingTalk workspace tasks from the terminal"`,
		`version "1.2.3"`,
		"on_macos do",
		"on_linux do",
		"https://downloads.example.com/dws/releases/v1.2.3/dws-darwin-amd64.tar.gz",
		"https://downloads.example.com/dws/releases/v1.2.3/dws-darwin-arm64.tar.gz",
		"https://downloads.example.com/dws/releases/v1.2.3/dws-linux-amd64.tar.gz",
		"https://downloads.example.com/dws/releases/v1.2.3/dws-linux-arm64.tar.gz",
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

	for _, want := range []string{"Agent Skills are bundled", "dws skill setup"} {
		if !strings.Contains(releaseFormulaText, want) {
			t.Fatalf("release formula missing caveat %q:\n%s", want, releaseFormulaText)
		}
	}
	if strings.Contains(releaseFormulaText, "Dir.home") {
		t.Fatalf("release formula must not mutate the user's home directory:\n%s", releaseFormulaText)
	}
	for _, forbidden := range []string{`require "fileutils"`, "FileUtils.", "__DESCRIPTION__"} {
		if strings.Contains(releaseFormulaText, forbidden) {
			t.Fatalf("release formula contains forbidden text %q:\n%s", forbidden, releaseFormulaText)
		}
	}

	// Re-running post packaging must replace, not duplicate, the skills checksum.
	cmd = exec.Command("sh", scriptPath)
	cmd.Env = postGoreleaserEnv(t, distDir, "v1.2.3", "https://downloads.example.com/dws/releases/v1.2.3")
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

func TestCheckedInHomebrewFormulaIsStableAndSideEffectFree(t *testing.T) {
	t.Parallel()

	formulaPath := filepath.Join("..", "..", "Formula", "dingtalk-workspace-cli.rb")
	data, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", formulaPath, err)
	}
	formula := string(data)
	versionPrefix := `version "`
	versionStart := strings.Index(formula, versionPrefix)
	if versionStart == -1 {
		t.Fatal("checked-in Homebrew formula has no explicit version")
	}
	versionStart += len(versionPrefix)
	versionEnd := strings.Index(formula[versionStart:], `"`)
	if versionEnd == -1 {
		t.Fatal("checked-in Homebrew formula has an invalid version declaration")
	}
	version := formula[versionStart : versionStart+versionEnd]
	if strings.Contains(version, "-") {
		t.Fatalf("checked-in Homebrew formula must be stable, got version %q", version)
	}
	releaseBase := "releases/download/v" + version + "/"
	for _, required := range []string{
		releaseBase + "dws-darwin-amd64.tar.gz",
		releaseBase + "dws-darwin-arm64.tar.gz",
		releaseBase + "dws-linux-amd64.tar.gz",
		releaseBase + "dws-linux-arm64.tar.gz",
		releaseBase + "dws-skills.zip",
		"dws skill setup",
	} {
		if !strings.Contains(formula, required) {
			t.Errorf("checked-in Homebrew formula is missing %q", required)
		}
	}
	for _, forbidden := range []string{"-beta.", "Dir.home", "def post_install", `require "fileutils"`, "FileUtils."} {
		if strings.Contains(formula, forbidden) {
			t.Errorf("checked-in Homebrew formula contains forbidden text %q", forbidden)
		}
	}
}

func TestCheckedInHomebrewBetaFormulaIsSeparateAndKegOnly(t *testing.T) {
	t.Parallel()

	formulaPath := filepath.Join("..", "..", "Formula", "dingtalk-workspace-cli-beta.rb")
	data, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", formulaPath, err)
	}
	formula := string(data)
	versionPrefix := `version "`
	versionStart := strings.Index(formula, versionPrefix)
	if versionStart == -1 {
		t.Fatal("checked-in Homebrew beta formula is missing a version declaration")
	}
	versionStart += len(versionPrefix)
	versionEnd := strings.Index(formula[versionStart:], `"`)
	if versionEnd == -1 {
		t.Fatal("checked-in Homebrew beta formula has an invalid version declaration")
	}
	version := formula[versionStart : versionStart+versionEnd]
	if !strings.Contains(version, "-") {
		t.Fatalf("checked-in Homebrew beta formula must be a prerelease, got version %q", version)
	}
	releaseBase := "releases/download/v" + version + "/"
	for _, required := range []string{
		"class DingtalkWorkspaceCliBeta < Formula",
		`desc "Automate DingTalk workspace tasks from the terminal (beta channel)"`,
		`keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"`,
		releaseBase + "dws-darwin-amd64.tar.gz",
		releaseBase + "dws-darwin-arm64.tar.gz",
		releaseBase + "dws-linux-amd64.tar.gz",
		releaseBase + "dws-linux-arm64.tar.gz",
		releaseBase + "dws-skills.zip",
		"This beta is keg-only",
	} {
		if !strings.Contains(formula, required) {
			t.Errorf("checked-in Homebrew beta formula is missing %q", required)
		}
	}
	for _, forbidden := range []string{"Dir.home", "def post_install", `require "fileutils"`, "FileUtils."} {
		if strings.Contains(formula, forbidden) {
			t.Errorf("checked-in Homebrew beta formula contains forbidden text %q", forbidden)
		}
	}
}

func TestPostGoreleaserBuildsVersionedBetaFormula(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "post-goreleaser.sh"))
	if err != nil {
		t.Fatalf("Abs(post-goreleaser.sh) error = %v", err)
	}
	distDir := filepath.Join(t.TempDir(), "dist")
	seedDistArtifacts(t, distDir, []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
	})
	env := postGoreleaserEnv(t, distDir, "v1.2.3-beta.4", "https://downloads.example.com/dws/releases/v1.2.3-beta.4")
	for i, value := range env {
		if strings.HasPrefix(value, "DWS_PACKAGE_VERSION=") {
			env[i] = "DWS_PACKAGE_VERSION=v1.2.3-beta.4"
		}
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("post-goreleaser.sh error = %v\noutput:\n%s", err, output)
	}

	formulaPath := filepath.Join(distDir, "homebrew", "dingtalk-workspace-cli-beta.rb")
	data, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", formulaPath, err)
	}
	formula := string(data)
	for _, required := range []string{
		"class DingtalkWorkspaceCliBeta < Formula",
		`desc "Automate DingTalk workspace tasks from the terminal (beta channel)"`,
		`version "1.2.3-beta.4"`,
		`keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"`,
		"This beta is keg-only",
	} {
		if !strings.Contains(formula, required) {
			t.Errorf("generated beta formula is missing %q", required)
		}
	}
	if strings.Contains(formula, "__") {
		t.Fatalf("generated beta formula contains an unresolved placeholder:\n%s", formula)
	}
	for _, forbidden := range []string{`require "fileutils"`, "FileUtils."} {
		if strings.Contains(formula, forbidden) {
			t.Fatalf("generated beta formula contains forbidden text %q:\n%s", forbidden, formula)
		}
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
	cmd.Env = postGoreleaserEnv(t, distDir, "v9.9.9", "https://downloads.example.com/dws/releases/v9.9.9")
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
	targets := []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
	}
	foundHost := false
	for _, target := range targets {
		if target == archiveName {
			foundHost = true
			break
		}
	}
	if !foundHost {
		targets = append(targets, archiveName)
	}
	seedDistArtifacts(t, distDir, targets)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = postGoreleaserEnv(t, distDir, "v0.0.0-test", "https://downloads.example.com/dws/releases/v0.0.0")
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
	// Schema hints are shared build-only inputs, not mono Skill content. They
	// must not leak into either backward-compatible copy of the mono bundle.
	for _, rel := range []string{
		"schema-hints",
		filepath.Join("mono", "schema-hints"),
		filepath.Join("multi", "schema-hints"),
	} {
		if _, err := os.Stat(filepath.Join(extractDir, rel)); err == nil {
			t.Fatalf("zip unexpectedly contains build-only %s", rel)
		} else if !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) error = %v", rel, err)
		}
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

func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	return string(data)
}

func releaseWorkflowSection(t *testing.T, workflow, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(workflow, startMarker)
	if start == -1 {
		t.Fatalf("release workflow is missing section marker %q", startMarker)
	}
	end := strings.Index(workflow[start+len(startMarker):], endMarker)
	if end == -1 {
		t.Fatalf("release workflow section %q is missing end marker %q", startMarker, endMarker)
	}
	return workflow[start : start+len(startMarker)+end]
}

func TestReleaseWorkflowUsesDedicatedGovernanceIdentity(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)

	const (
		checksCall    = "github.rest.checks.listForRef"
		paginatedCall = "github.paginate(github.rest.checks.listForRef"
		immutableCall = `"GET /repos/{owner}/{repo}/immutable-releases"`
		governanceID  = `github-token: ${{ secrets.RELEASE_GOVERNANCE_TOKEN }}`
	)
	if got := strings.Count(workflow, checksCall); got != 2 {
		t.Fatalf("release workflow Checks API call count = %d, want one tag check and one preflight check", got)
	}
	if got := strings.Count(workflow, paginatedCall); got != 2 {
		t.Fatalf("release workflow paginated Checks API call count = %d, want one tag check and one preflight check", got)
	}
	if got := strings.Count(workflow, immutableCall); got != 2 {
		t.Fatalf("release workflow immutable governance call count = %d, want one tag check and one preflight check", got)
	}
	if got := strings.Count(workflow, governanceID); got != 2 {
		t.Fatalf("release workflow dedicated governance identity count = %d, want one per immutable check", got)
	}

	sections := map[string]string{
		"preflight": releaseWorkflowSection(t, workflow, "  governance-preflight:\n", "\n  release-contract:\n"),
		"tag":       releaseWorkflowSection(t, workflow, "  release-contract:\n", "\n  release:\n"),
	}
	for name, section := range sections {
		for _, required := range []string{
			"checks: read",
			checksCall,
			paginatedCall,
			"run.head_sha !== sha",
			"const missing = requiredContexts.filter",
			"const nonSuccess = requiredContexts.flatMap",
			"missing:",
			"non-success:",
			immutableCall,
			governanceID,
			"RELEASE_GOVERNANCE_TOKEN with repository Administration read permission is required",
		} {
			if !strings.Contains(section, required) {
				t.Errorf("%s governance path is missing %q", name, required)
			}
		}
		for _, context := range expectedReleaseAdmissionContexts {
			if !strings.Contains(section, fmt.Sprintf("%q", context)) {
				t.Errorf("%s governance path is missing exact Code Admission context %q", name, context)
			}
		}
		if strings.Contains(section, "check_name:") {
			t.Errorf("%s governance path must fetch all check runs in one exact-SHA query", name)
		}
		if strings.Contains(section, "contents: write") {
			t.Errorf("%s governance path must not grant contents write permission", name)
		}
		if strings.Contains(section, `github-token: ${{ secrets.GITHUB_TOKEN }}`) {
			t.Errorf("%s immutable governance path must not fall back to GITHUB_TOKEN", name)
		}
	}
	if strings.Contains(workflow, "CI"+" Gate") {
		t.Error("release workflow must not retain the retired aggregate gate name")
	}
}

func TestReleaseScriptRequiresExactCodeAdmissionContexts(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "release.sh"))
	if err != nil {
		t.Fatalf("Abs(release.sh) error = %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	script := string(data)

	const checksQuery = `commits/$sealed_commit/check-runs?filter=latest&per_page=100`
	if got := strings.Count(script, checksQuery); got != 1 {
		t.Fatalf("release script exact-SHA Checks API query count = %d, want 1", got)
	}
	for _, required := range []string{
		`group_by(.name) | map(max_by(.id))`,
		`missing_contexts=""`,
		`non_success_contexts=""`,
		`"$context_state" != "success"`,
		"Code Admission contexts are not all successful for sealed commit",
		"missing: %s; non-success: %s",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("release script Code Admission gate is missing %q", required)
		}
	}
	for _, context := range expectedReleaseAdmissionContexts {
		if !strings.Contains(script, "\n"+context+"\n") {
			t.Errorf("release script is missing exact Code Admission context %q", context)
		}
	}
	if strings.Contains(script, "check_name=") {
		t.Error("release script must fetch all check runs in one exact-SHA query")
	}
	if strings.Contains(script, "CI"+" Gate") {
		t.Error("release script must not retain the retired aggregate gate name")
	}
}

func TestReleaseWorkflowGovernancePreflightCannotPublish(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	preflight := releaseWorkflowSection(t, workflow, "  governance-preflight:\n", "\n  release-contract:\n")

	for _, required := range []string{
		"governance_preflight_commit:",
		"governance_preflight_nonce:",
		`format('Release governance preflight {0}', inputs.governance_preflight_nonce)`,
		"name: Release governance preflight",
		"name: Check out trusted preflight tooling",
		"github.event_name == 'workflow_dispatch'",
		"EXPECTED_REPOSITORY: DingTalk-Real-AI/dingtalk-workspace-cli",
		`DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}`,
		`test "$PREFLIGHT_COMMIT" = "$GITHUB_SHA"`,
		`ref: ${{ needs.dispatch-contract.outputs.mode == 'create_release' && github.sha || inputs.governance_preflight_commit }}`,
		"persist-credentials: false",
		"governance preflight cannot be combined with npm repair",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("governance preflight contract is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"goreleaser",
		"gh release",
		"npm publish",
		"sync-to-oss",
		"sync-to-gitee",
	} {
		if strings.Contains(preflight, forbidden) {
			t.Errorf("governance preflight must not contain publishing behavior %q", forbidden)
		}
	}
	admission := strings.Index(preflight, "Require successful Code Admission contexts on the preflight commit")
	homebrewCanary := strings.Index(preflight, "Verify Homebrew PR automation permission")
	if admission == -1 || homebrewCanary == -1 || admission > homebrewCanary {
		t.Error("governance preflight must validate all exact Code Admission contexts before exposing Homebrew credentials")
	}

	mirror := releaseWorkflowSection(t, workflow, "  mirror-gitee-release:\n", "\n  repair-npm:\n")
	if !strings.Contains(mirror, "needs: [release-contract, release, publish-channels]") {
		t.Error("Gitee publication must remain downstream of the verified release target and channel jobs")
	}
	repair := workflow[strings.Index(workflow, "  repair-npm:\n"):]
	for _, required := range []string{
		"needs: dispatch-contract",
		"needs.dispatch-contract.outputs.mode == 'repair_npm'",
		`ref: ` + "`tags/withdrawn/${version}`",
		"was withdrawn and cannot be repaired",
		"verify-release-workflow-delivery.sh --npm-repair",
	} {
		if !strings.Contains(repair, required) {
			t.Errorf("npm repair dispatch contract is missing %q", required)
		}
	}
}

func TestReleaseWorkflowPlansAndSealsCurrentMainInTheCloud(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	planStart := strings.Index(workflow, "  release-plan:\n")
	sealStart := strings.Index(workflow, "  seal-release:\n")
	if planStart == -1 || sealStart == -1 || planStart >= sealStart {
		t.Fatal("cloud release plan and seal jobs are missing or out of order")
	}
	plan := workflow[planStart:sealStart]
	seal := workflow[sealStart:]

	for _, required := range []string{
		"release_operation:",
		"- none",
		"- plan",
		"- publish",
		"release_channel:",
		"release_bump:",
		"release_confirmation:",
		`release_flow + npm_repair + gitee_repair + oss_repair + governance + recovery`,
		`echo "mode=plan_release"`,
		`echo "mode=create_release"`,
		`release_confirmation must be exactly: PUBLISH $RELEASE_CHANNEL`,
		`needs.dispatch-contract.outputs.mode == 'plan_release'`,
		`needs.governance-preflight.result == 'success'`,
		"actions: read",
		"contents: read",
		`github.event.repository.default_branch`,
		`GITHUB_REPOSITORY" = "$EXPECTED_REPOSITORY`,
		`GITHUB_REF" = "refs/heads/$DEFAULT_BRANCH`,
		`ref: ${{ github.sha }}`,
		"persist-credentials: false",
		`refs/remotes/origin/main)" = "$GITHUB_SHA`,
		"next-release-version.sh",
		"refs/tags/withdrawn/v",
		"refs_fingerprint",
		"Validate the candidate release contract before sealing",
		"release-contract.sh",
		"Require delivered previous stable baseline before sealing",
		"Require delivered beta before sealing stable",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("cloud release contract is missing %q", required)
		}
	}
	if strings.Contains(plan, "contents: write") {
		t.Error("cloud release planning must remain read-only")
	}

	for _, required := range []string{
		"name: Seal cloud release tag",
		"contents: write",
		"name: Create one immutable annotated release tag",
		`branch.data.commit.sha !== commit`,
		`actualFingerprint !== expectedFingerprint`,
		`github.rest.git.createTag`,
		`github.rest.git.createRef`,
		`ref: ` + "`refs/tags/${version}`",
		"`Release-Run: ${context.runId}`",
		"`Requested-By: ${context.actor}`",
		"`Requested-By-ID: ${context.payload.sender?.id || \"\"}`",
		"`Sealed-Commit: ${commit}`",
		"`Workflow-Commit: ${context.sha}`",
		"`Allocation-Fingerprint: ${expectedFingerprint}`",
	} {
		if !strings.Contains(seal, required) {
			t.Errorf("cloud release seal is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"actions/checkout",
		"github.rest.git.updateRef",
		"github.rest.git.deleteRef",
		"git push",
		"--force",
	} {
		if strings.Contains(seal, forbidden) {
			t.Errorf("write-capable cloud seal must not contain %q", forbidden)
		}
	}
}

func TestReleaseWorkflowAcceptsGuardedLocalTagMetadata(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)

	for _, required := range []string{
		`const cloudOnlyKeys = [`,
		`const cloudKeys = ["Channel", ...cloudOnlyKeys];`,
		`const hasAnyCloudMetadata = cloudOnlyKeys.some((key) => tagFields.has(key));`,
		`const isCloudSeal = cloudKeys.every((key) => tagFields.has(key));`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("local tag metadata compatibility is missing %q", required)
		}
	}
	if strings.Contains(workflow, `const hasAnyCloudMetadata = cloudKeys.some((key) => tagFields.has(key));`) {
		t.Error("Channel-only guarded local tags must not be classified as partial cloud seals")
	}
}

func TestReleaseWorkflowChannelRepairUsesSealedReleaseAuthority(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	dispatch := releaseWorkflowSection(t, workflow, "  dispatch-contract:\n", "\n  authorize-recovery:\n")
	start := strings.Index(workflow, "  repair-channel:\n")
	if start == -1 {
		t.Fatal("release workflow is missing the channel repair job")
	}
	end := strings.Index(workflow[start:], "\n  release-plan:\n")
	if end == -1 {
		t.Fatal("release workflow channel repair job is missing its end marker")
	}
	repair := workflow[start : start+end]
	tagAuthority := releaseWorkflowSection(
		t,
		repair,
		"      - name: Fetch and verify sealed release tag\n",
		"\n      - name: Require successful Release workflow delivery\n",
	)

	for _, required := range []string{
		"repair_gitee_version:",
		`format('Release Gitee repair {0}', inputs.repair_gitee_version)`,
		`REPAIR_GITEE_VERSION: ${{ inputs.repair_gitee_version }}`,
		"repair_oss_version:",
		`format('Release OSS repair {0}', inputs.repair_oss_version)`,
		`REPAIR_OSS_VERSION: ${{ inputs.repair_oss_version }}`,
		"gitee_repair=0",
		"oss_repair=0",
		`test -z "$REPAIR_GITEE_VERSION" || gitee_repair=1`,
		`test -z "$REPAIR_OSS_VERSION" || oss_repair=1`,
		"release_flow + npm_repair + gitee_repair + oss_repair + governance + recovery",
		`echo "mode=repair_gitee" >> "$GITHUB_OUTPUT"`,
		`echo "mode=repair_oss" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(dispatch, required) && !strings.Contains(workflow, required) {
			t.Errorf("channel repair dispatch contract is missing %q", required)
		}
	}

	for _, required := range []string{
		"needs: dispatch-contract",
		`if: ${{ !cancelled() && needs.dispatch-contract.result == 'success' && (needs.dispatch-contract.outputs.mode == 'repair_gitee' || needs.dispatch-contract.outputs.mode == 'repair_oss') && github.ref == format('refs/heads/{0}', github.event.repository.default_branch) && github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli' }}`,
		`github.ref == format('refs/heads/{0}', github.event.repository.default_branch)`,
		`github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli'`,
		"actions: read",
		"contents: read",
		`ref: ${{ github.sha }}`,
		"path: tooling",
		"persist-credentials: false",
		"release_is_stable_version",
		"release_is_prerelease_version",
		`ref: ` + "`tags/withdrawn/${version}`",
		"was withdrawn and cannot be repaired",
		`ref: ` + "`tags/${version}`",
		`["ahead", "identical"].includes(comparison.data.status)`,
		"!release.data.immutable",
		`release.data.prerelease !== expectedPrerelease`,
		"assetNames.length !== expectedAssets.size",
		"new Set(assetNames).size !== expectedAssets.size",
		`core.setOutput("tag_object", ref.data.object.sha)`,
		`ref: ${{ steps.authority.outputs.commit_sha }}`,
		"path: release-source",
		"verify-github-tag-authority.sh",
		`GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`,
		"verify-release-workflow-delivery.sh",
		`REPAIR_MODE: ${{ needs.dispatch-contract.outputs.mode }}`,
		"repair_gitee) target=gitee",
		"repair_oss) target=oss",
		`--channel-repair "$target"`,
		`DWS_RELEASE_GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`,
		"verify-release-artifacts.sh",
		`--repo "$GITHUB_REPOSITORY"`,
		`if: ${{ needs.dispatch-contract.outputs.mode == 'repair_gitee' }}`,
		`DWS_REQUIRE_GITEE: "1"`,
		`if: ${{ needs.dispatch-contract.outputs.mode == 'repair_oss' }}`,
		`DWS_REQUIRE_OSS: "1"`,
		`OSS_ACCESS_KEY_ID: ${{ secrets.OSS_ACCESS_KEY_ID }}`,
		`OSS_ACCESS_KEY_SECRET: ${{ secrets.OSS_ACCESS_KEY_SECRET }}`,
		`OSS_ENDPOINT: ${{ secrets.OSS_ENDPOINT }}`,
		`OSS_BUCKET: ${{ secrets.OSS_BUCKET }}`,
		`OSS_PREFIX: ${{ secrets.OSS_PREFIX }}`,
		"working-directory: release-source",
		"working-directory: tooling\n        run: |\n          " +
			`"$GITHUB_WORKSPACE/tooling/scripts/release/sync-to-gitee.sh"`,
		`"$GITHUB_WORKSPACE/tooling/scripts/release/sync-to-oss.sh"`,
	} {
		if !strings.Contains(repair, required) {
			t.Errorf("channel repair authority is missing %q", required)
		}
	}
	for _, asset := range []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
		"dws-windows-amd64.zip",
		"dws-windows-arm64.zip",
		"dws-skills.zip",
		"checksums.txt",
	} {
		if strings.Count(repair, `"`+asset+`"`) != 1 {
			t.Errorf("channel repair must require exactly one %s asset declaration", asset)
		}
	}
	if strings.Contains(repair, "contents: write") {
		t.Error("channel repair must not grant contents write permission")
	}
	for _, forbidden := range []string{
		"RELEASE_GOVERNANCE_TOKEN",
		"HOMEBREW_PR_TOKEN",
		"NPM_TOKEN",
	} {
		if strings.Contains(repair, forbidden) {
			t.Errorf("channel repair must not expose unrelated credential %s", forbidden)
		}
	}
	if strings.Contains(repair, "ref: ${{ github.event.repository.default_branch }}") {
		t.Error("channel repair must not check out a floating default branch")
	}
	if !strings.Contains(tagAuthority, `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`) {
		t.Error("remote annotated-tag authority step must receive the read-only GitHub token")
	}
	if got := strings.Count(repair, "working-directory: tooling"); got < 4 {
		t.Errorf("channel repair trusted tooling working-directory count = %d, want at least 4", got)
	}
}

func TestReleaseWorkflowRecoveryReusesGuardedJobs(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)

	for _, required := range []string{
		"recover_release_version:",
		"recover_release_tag_object:",
		"recover_release_commit:",
		"recover_failed_run_id:",
		"recover_failed_run_attempt:",
		"recover_release_nonce:",
		"recover_release_confirmation:",
		`format('Release recovery {0} at {1} {2}', inputs.recover_release_version, inputs.recover_release_commit, inputs.recover_release_nonce)`,
		"workflow_dispatch must select exactly one release mode",
		"release recovery confirmation must equal the exact version",
		"recover_release_nonce must be bound to the release commit",
		"environment: release-recovery",
		"prevent_self_review !== true",
		"protected_branches !== true",
		"can_admins_bypass !== false",
		`run.path !== ".github/workflows/release.yml"`,
		`const expectedEvent = failedByCloud ? "workflow_dispatch" : "push"`,
		`run.event !== expectedEvent`,
		`tagFields.get("Release-Run") !== failedRunId`,
		`"GET /repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}"`,
		"attempt_number: Number(failedRunAttempt)",
		"run.run_attempt !== Number(failedRunAttempt)",
		`["failure", "cancelled", "timed_out", "startup_failure", "stale"].includes(run.conclusion)`,
		`run.head_branch !== expectedBranch`,
		`run.head_sha !== commit`,
		`tagObject !== expectedTagObject`,
		`["ahead", "identical"].includes(comparison.data.status)`,
		"already has a public release; use a channel repair instead",
		"Bind recovery publication to this workflow run",
		"dws-release-recovery run=%s tag-object=%s commit=%s",
		"Public release is not bound to this exact recovery run.",
		"Public recovery asset differs from this run's sealed artifact",
		`const sha = process.env.RELEASE_COMMIT`,
		`ref: sha`,
		`path: tmp/trusted-release-tooling`,
		`ref: ${{ github.sha }}`,
		"verify-release-workflow-delivery.sh",
		"Require a clean sealed source before GoReleaser",
		`git status --porcelain --untracked-files=all`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release recovery contract is missing %q", required)
		}
	}

	sections := map[string]string{
		"release":          releaseWorkflowSection(t, workflow, "  release:\n", "\n  verify-darwin-signatures:\n"),
		"publish-release":  releaseWorkflowSection(t, workflow, "  publish-release:\n", "\n  publish-channels:\n"),
		"publish-channels": releaseWorkflowSection(t, workflow, "  publish-channels:\n", "\n  mirror-gitee-release:\n"),
	}
	for name, section := range sections {
		for _, required := range []string{
			"needs.release-contract.outputs.release_version",
			"ref: ${{ needs.release-contract.outputs.release_commit }}",
			"persist-credentials: false",
			`tmp/trusted-release-tooling/scripts/release/verify-github-tag-authority.sh`,
		} {
			if !strings.Contains(section, required) {
				t.Errorf("%s does not consume the verified recovery target %q", name, required)
			}
		}
		if strings.Contains(section, "github.event_name == 'workflow_dispatch'") {
			t.Errorf("%s must not fork into a recovery-specific publisher", name)
		}
	}
	if strings.Count(workflow, "name: Build signed release artifacts") != 1 ||
		strings.Count(workflow, "name: Verify Apple Developer ID signatures") != 1 ||
		strings.Count(workflow, "name: Publish immutable GitHub Release") != 1 ||
		strings.Count(workflow, "name: Publish npm and mirrors") != 1 {
		t.Fatal("normal and recovery publication must share one build/sign/publish job graph")
	}
}

func TestReleaseWorkflowDraftLifecycleUsesOneReleaseID(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	publishJob := releaseWorkflowSection(t, workflow, "  publish-release:\n", "\n  publish-channels:\n")
	publishStep := releaseWorkflowSection(
		t,
		publishJob,
		"      - name: Publish or reuse immutable GitHub Release\n",
		"\n      - name: Require immutable published GitHub Release\n",
	)

	for _, required := range []string{
		"id: publish",
		"--json databaseId",
		`"repos/$GITHUB_REPOSITORY/releases/$release_id"`,
		`uploaded_release_id="$(`,
		`test "$uploaded_release_id" = "$release_id"`,
		"Draft GitHub Release ID $release_id targets",
		"Draft GitHub Release notes differ from the sealed CHANGELOG.",
		"Draft GitHub Release is not bound to this exact recovery run.",
		`DWS_GITHUB_RELEASE_ID="$release_id"`,
		`tmp/trusted-release-tooling/scripts/release/verify-github-release-assets.sh`,
		`tmp/trusted-release-tooling/scripts/release/download-github-release-assets.sh`,
		`cmp -s "$local_asset" "$remote_asset"`,
		"-F draft=false",
		`echo "release_id=$release_id" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(publishStep, required) {
			t.Errorf("Draft release lifecycle is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`gh release download "$RELEASE_VERSION"`,
		`gh release edit "$RELEASE_VERSION" --draft=false`,
		`"repos/$GITHUB_REPOSITORY/releases/tags/$RELEASE_VERSION"`,
	} {
		if strings.Contains(publishStep, forbidden) {
			t.Errorf("Draft release lifecycle must not switch back from the locked release ID via %q", forbidden)
		}
	}

	tagGuard := strings.Index(publishStep, "Draft GitHub Release ID $release_id targets")
	draftPatch := strings.Index(publishStep, "-F draft=true")
	bodyVerify := strings.Index(publishStep, "Draft GitHub Release notes differ from the sealed CHANGELOG.")
	markerVerify := strings.Index(publishStep, "Draft GitHub Release is not bound to this exact recovery run.")
	upload := strings.Index(publishStep, `gh release upload "$RELEASE_VERSION"`)
	idRecheck := strings.Index(publishStep, `test "$uploaded_release_id" = "$release_id"`)
	verify := strings.Index(publishStep, "tmp/trusted-release-tooling/scripts/release/verify-github-release-assets.sh")
	download := strings.LastIndex(publishStep, "tmp/trusted-release-tooling/scripts/release/download-github-release-assets.sh")
	byteCompare := strings.Index(publishStep, `cmp -s "$local_asset" "$remote_asset"`)
	publish := strings.Index(publishStep, "-F draft=false")
	if tagGuard == -1 || draftPatch == -1 || bodyVerify == -1 || markerVerify == -1 ||
		upload == -1 || idRecheck == -1 || verify == -1 || download == -1 || byteCompare == -1 || publish == -1 ||
		!(tagGuard < draftPatch && draftPatch < bodyVerify && bodyVerify < markerVerify && markerVerify < upload &&
			upload < idRecheck && idRecheck < verify && verify < download && download < byteCompare && byteCompare < publish) {
		t.Fatal("Draft must retain one release ID through upload, exact verification, download, byte comparison, and publication")
	}

	terminalStep := publishJob[strings.Index(publishJob, "      - name: Require immutable published GitHub Release\n"):]
	for _, required := range []string{
		`RELEASE_ID: ${{ steps.publish.outputs.release_id }}`,
		`RELEASE_CHANNEL: ${{ needs.release-contract.outputs.channel }}`,
		`"repos/$GITHUB_REPOSITORY/releases/$RELEASE_ID"`,
		`DWS_GITHUB_RELEASE_ID="$RELEASE_ID"`,
		`tmp/trusted-release-tooling/scripts/release/verify-github-release-assets.sh`,
		`tmp/trusted-release-tooling/scripts/release/download-github-release-assets.sh`,
		`[.tag_name, .draft, .prerelease, .immutable] | @tsv`,
		`printf '%s\tfalse\t%s\ttrue' "$RELEASE_VERSION" "$expected_prerelease"`,
		"Immutable GitHub Release notes differ from the sealed CHANGELOG.",
		"Immutable GitHub Release is not bound to this exact recovery run.",
		`DWS_PACKAGE_DIST_DIR="$immutable_dir"`,
		`cmp -s "$sealed_asset" "$immutable_asset"`,
	} {
		if !strings.Contains(terminalStep, required) {
			t.Errorf("terminal immutable release gate is missing %q", required)
		}
	}
	immutableState := strings.Index(terminalStep, `[.tag_name, .draft, .prerelease, .immutable] | @tsv`)
	immutableBody := strings.Index(terminalStep, "Immutable GitHub Release notes differ from the sealed CHANGELOG.")
	immutableDownload := strings.Index(terminalStep, "tmp/trusted-release-tooling/scripts/release/download-github-release-assets.sh")
	immutableVerify := strings.Index(terminalStep, `DWS_PACKAGE_DIST_DIR="$immutable_dir"`)
	immutableCompare := strings.Index(terminalStep, `cmp -s "$sealed_asset" "$immutable_asset"`)
	if immutableState == -1 || immutableBody == -1 || immutableDownload == -1 || immutableVerify == -1 || immutableCompare == -1 ||
		!(immutableState < immutableBody && immutableBody < immutableDownload && immutableDownload < immutableVerify && immutableVerify < immutableCompare) {
		t.Fatal("terminal gate must reverify immutable notes and exact sealed bytes on the locked release ID")
	}
}

func TestRecoverReleaseBindsOneFailedRunAttempt(t *testing.T) {
	t.Parallel()
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "recover-release.sh"))
	if err != nil {
		t.Fatalf("Abs(recover-release.sh) error = %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	script := string(data)

	for _, required := range []string{
		"--failed-attempt <attempt>",
		"--failed-attempt requires --failed-run",
		"find_latest_failed_attempt",
		`while [ "$find_attempt" -ge 1 ]`,
		"actions/runs/$find_run_id/attempts/$find_attempt",
		`select(.head_sha == \"$commit\" and .head_branch == \"$VERSION\")`,
		"Release run %s has no failed attempt",
		`[.id, .run_attempt, .repository.full_name, .path, .event, .status, .conclusion, .head_branch, .head_sha, .actor.login, .actor.id] | @tsv`,
		`Release-Run`,
		`Release-Run-Attempt`,
		`expected_attempt_event="workflow_dispatch"`,
		`is not bound by the cloud seal`,
		"actions/runs/%s/attempts/%s",
		`-f "recover_failed_run_attempt=$FAILED_RUN_ATTEMPT"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("release recovery script is missing attempt binding %q", required)
		}
	}
}

func TestReleaseWorkflowPublicationBypassesSkippedDispatchButStopsOnCancellation(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)

	tests := []struct {
		name      string
		start     string
		end       string
		condition string
	}{
		{
			name:      "release contract",
			start:     "  release-contract:\n",
			end:       "\n  release:\n",
			condition: `if: ${{ !cancelled() && (github.event_name == 'push' || (needs.dispatch-contract.result == 'success' && needs.dispatch-contract.outputs.mode == 'recover_release' && needs.authorize-recovery.result == 'success') || (needs.dispatch-contract.result == 'success' && needs.dispatch-contract.outputs.mode == 'create_release' && needs.governance-preflight.result == 'success' && needs.release-plan.result == 'success' && needs.seal-release.result == 'success')) }}`,
		},
		{
			name:      "build",
			start:     "  release:\n",
			end:       "\n  verify-darwin-signatures:\n",
			condition: `if: ${{ !cancelled() && needs.release-contract.result == 'success' }}`,
		},
		{
			name:      "Darwin verification",
			start:     "  verify-darwin-signatures:\n",
			end:       "\n  publish-release:\n",
			condition: `if: ${{ !cancelled() && needs.release-contract.result == 'success' && needs.release.result == 'success' }}`,
		},
		{
			name:      "GitHub publication",
			start:     "  publish-release:\n",
			end:       "\n  publish-channels:\n",
			condition: `if: ${{ !cancelled() && needs.release-contract.result == 'success' && needs.release.result == 'success' && needs.verify-darwin-signatures.result == 'success' }}`,
		},
		{
			name:      "channel publication",
			start:     "  publish-channels:\n",
			end:       "\n  mirror-gitee-release:\n",
			condition: `if: ${{ !cancelled() && needs.release-contract.result == 'success' && needs.publish-release.result == 'success' }}`,
		},
		{
			name:      "Gitee mirror",
			start:     "  mirror-gitee-release:\n",
			end:       "\n  repair-npm:\n",
			condition: `if: ${{ !cancelled() && vars.ENABLE_GITEE_UPLOAD_FALLBACK == 'true' && needs.release-contract.result == 'success' && needs.release.result == 'success' && needs.publish-channels.result == 'success' }}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			section := releaseWorkflowSection(t, workflow, test.start, test.end)
			if !strings.Contains(section, test.condition) {
				t.Errorf("%s must override skipped dispatch ancestors while preserving cancellation and dependency gates", test.name)
			}
		})
	}
}

func TestReleaseWorkflowDeliveryGateFailsClosed(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	gate := releaseWorkflowSection(t, workflow, "  release-delivery-gate:\n", "\n  repair-channel:\n")

	for _, required := range []string{
		"name: Release delivery gate",
		`if: ${{ !cancelled() }}`,
		"- dispatch-contract",
		"- authorize-recovery",
		"- governance-preflight",
		"- release-contract",
		"- release",
		"- verify-darwin-signatures",
		"- publish-release",
		"- publish-channels",
		"- mirror-gitee-release",
		"- repair-npm",
		"- repair-channel",
		"- release-plan",
		"- seal-release",
		`REPAIR_CHANNEL_RESULT: ${{ needs.repair-channel.result }}`,
		`RELEASE_PLAN_RESULT: ${{ needs.release-plan.result }}`,
		`SEAL_RELEASE_RESULT: ${{ needs.seal-release.result }}`,
		"require_publication",
		`require_result release-contract "$RELEASE_CONTRACT_RESULT" success`,
		`require_result release "$RELEASE_RESULT" success`,
		`require_result verify-darwin-signatures "$DARWIN_SIGNATURE_RESULT" success`,
		`require_result publish-release "$PUBLISH_RELEASE_RESULT" success`,
		`require_result publish-channels "$PUBLISH_CHANNELS_RESULT" success`,
		"workflow_dispatch:recover_release",
		"workflow_dispatch:create_release",
		"workflow_dispatch:plan_release",
		"workflow_dispatch:governance_preflight",
		"workflow_dispatch:repair_npm",
		"workflow_dispatch:repair_gitee",
		"workflow_dispatch:repair_oss",
		`require_result repair-channel "$REPAIR_CHANNEL_RESULT" success`,
		`require_result repair-channel "$REPAIR_CHANNEL_RESULT" skipped`,
		"unsupported release mode",
	} {
		if !strings.Contains(gate, required) {
			t.Errorf("release delivery gate is missing %q", required)
		}
	}
}

func TestReleaseWorkflowUploadsPostProcessedDarwinAssets(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)

	build := strings.Index(workflow, "Build release artifacts without publishing")
	postProcess := strings.Index(workflow, "./scripts/release/post-goreleaser.sh")
	preserve := strings.Index(workflow, "Preserve finalized distribution files")
	verifyJob := strings.Index(workflow, "verify-darwin-signatures:")
	publishJob := strings.Index(workflow, "publish-release:")
	if build == -1 || postProcess == -1 || preserve == -1 || verifyJob == -1 || publishJob == -1 ||
		!(build < postProcess && postProcess < preserve && preserve < verifyJob && verifyJob < publishJob) {
		t.Fatalf("post-processed assets must be preserved, Apple-verified, and only then published")
	}

	buildSection := workflow[build:verifyJob]
	for _, required := range []string{
		"--skip=publish",
		"actions/upload-artifact@v4",
		"finalized-release-dist",
	} {
		if !strings.Contains(buildSection, required) {
			t.Errorf("signed build stage is missing %q", required)
		}
	}

	publishSection := workflow[publishJob:]
	for _, required := range []string{
		"actions/download-artifact@v4",
		"dist/dws-*.tar.gz",
		"dist/dws-windows-*.zip",
		"checksums.txt",
		"dws-skills.zip",
		"gh release upload",
		"--clobber",
		"verify-release-artifacts.sh",
	} {
		if !strings.Contains(publishSection, required) {
			t.Errorf("immutable publication stage is missing %q", required)
		}
	}
}

func TestReleaseWorkflowConfiguresDeveloperIDSigning(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)

	prepare := strings.Index(workflow, "Prepare Apple Developer ID certificate")
	goReleaser := strings.Index(workflow, "Build release artifacts without publishing")
	postProcess := strings.Index(workflow, "./scripts/release/post-goreleaser.sh")
	cleanup := strings.Index(workflow, "Remove Apple Developer ID certificate")
	if prepare == -1 || goReleaser == -1 || postProcess == -1 || cleanup == -1 ||
		prepare > goReleaser || goReleaser > postProcess || cleanup < postProcess {
		t.Fatalf("Developer ID material must be validated before GoReleaser and removed after post-processing")
	}

	for _, required := range []string{
		`RCS_VERSION="0.29.0"`,
		"secrets.APPLE_CERTIFICATE_P12_BASE64",
		"secrets.APPLE_CERTIFICATE_PASSWORD",
		"base64 --decode",
		"openssl pkcs12 -legacy",
		"DWS_APPLE_CERTIFICATE_P12",
		"DWS_APPLE_CERTIFICATE_PASSWORD_FILE",
		"DWS_REQUIRE_DEVELOPER_ID_SIGNING",
		`GITHUB_REPOSITORY_OWNER" = "DingTalk-Real-AI`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing Developer ID configuration %q", required)
		}
	}
}

func TestPostGoreleaserSupportsDeveloperIDSigning(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "post-goreleaser.sh"))
	if err != nil {
		t.Fatalf("Abs(post-goreleaser.sh) error = %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	script := string(data)

	for _, required := range []string{
		`APPLE_CERTIFICATE_P12="${DWS_APPLE_CERTIFICATE_P12:-}"`,
		`APPLE_CERTIFICATE_PASSWORD_FILE="${DWS_APPLE_CERTIFICATE_PASSWORD_FILE:-}"`,
		`REQUIRE_DEVELOPER_ID_SIGNING="${DWS_REQUIRE_DEVELOPER_ID_SIGNING:-false}"`,
		`--p12-file "$APPLE_CERTIFICATE_P12"`,
		`--p12-password-file "$APPLE_CERTIFICATE_PASSWORD_FILE"`,
		"--for-notarization",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("post-goreleaser.sh is missing Developer ID signing behavior %q", required)
		}
	}
	if strings.Contains(script, `rcodesign verify "$bin"`) {
		t.Fatal("rcodesign verify must not be treated as authoritative Apple signature validation")
	}
}

func TestReleaseWorkflowVerifiesRcodesignArchiveChecksum(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)

	hash := strings.Index(workflow, `RCS_ARCHIVE_SHA256="dbe85cedd8ee4217b64e9a0e4c2aef92ab8bcaaa41f20bde99781ff02e600002"`)
	checksum := strings.Index(workflow, "sha256sum --check --strict")
	extract := strings.Index(workflow, "tar -xzf /tmp/rcodesign.tar.gz")
	execute := strings.Index(workflow, "rcodesign --version")
	if hash == -1 || checksum == -1 || extract == -1 || execute == -1 ||
		!(hash < checksum && checksum < extract && extract < execute) {
		t.Fatal("rcodesign archive must match the pinned SHA-256 before extraction or execution")
	}
}

func TestReleaseWorkflowUsesAppleCodesignBeforePublication(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)

	preserve := strings.Index(workflow, "Preserve finalized distribution files")
	verifyJob := strings.Index(workflow, "verify-darwin-signatures:")
	publishJob := strings.Index(workflow, "publish-release:")
	if preserve == -1 || verifyJob == -1 || publishJob == -1 || !(preserve < verifyJob && verifyJob < publishJob) {
		t.Fatal("finalized artifacts must be preserved, Apple-verified, and only then published")
	}

	codesign := strings.Index(workflow[verifyJob:publishJob], "codesign --verify --strict --verbose=4")
	publish := strings.Index(workflow[publishJob:], "-F draft=false")
	if codesign == -1 || publish == -1 {
		t.Fatal("macOS codesign verification and explicit Draft publication are required")
	}

	buildSection := workflow[preserve:verifyJob]
	for _, required := range []string{
		"actions/upload-artifact@v4",
		"finalized-release-dist",
	} {
		if !strings.Contains(buildSection, required) {
			t.Errorf("signed build stage is missing %q", required)
		}
	}

	verifySection := workflow[verifyJob:publishJob]
	for _, required := range []string{
		"runs-on: macos-latest",
		"actions/download-artifact@v4",
		"finalized-release-dist",
		`dws-darwin-${arch}.tar.gz`,
		"codesign --verify --strict --verbose=4",
	} {
		if !strings.Contains(verifySection, required) {
			t.Errorf("Apple verification stage is missing %q", required)
		}
	}

	publishSection := workflow[publishJob:]
	for _, required := range []string{
		"verify-darwin-signatures",
		"actions/download-artifact@v4",
		"Publish or reuse immutable GitHub Release",
		"gh release upload",
		"Publish missing version to npm channel",
		"Open stable Homebrew formula PR",
		"Open beta Homebrew formula PR",
		"DingTalk-Real-AI/dingtalk-workspace-cli.git",
		"secrets.HOMEBREW_PR_TOKEN",
	} {
		if !strings.Contains(publishSection, required) {
			t.Errorf("post-verification publication stage is missing %q", required)
		}
	}
}

func TestReleaseWorkflowOpensHomebrewPROnlyForOfficialStableTags(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)
	if strings.Contains(workflow, "pull-requests: write") {
		t.Fatal("the built-in GITHUB_TOKEN must not receive pull-request write permission")
	}
	for _, required := range []string{
		"Verify Homebrew PR automation permission",
		"secrets.HOMEBREW_PR_TOKEN",
		"verify-homebrew-pr-token.sh",
		"--canary",
		"HOMEBREW_PR_TOKEN and RELEASE_GOVERNANCE_TOKEN must use separate least-privilege identities",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing Homebrew PR token preflight %q", required)
		}
	}
	if got := strings.Count(workflow, "Verify Homebrew PR automation permission"); got != 2 {
		t.Errorf("Homebrew PR permission preflight count = %d, want one default-branch preflight and one tag contract", got)
	}
	tagContract := releaseWorkflowSection(t, workflow, "  release-contract:\n", "\n  release:\n")
	tagAdmission := strings.Index(tagContract, "Require successful Code Admission contexts on the sealed commit")
	tagHomebrew := strings.Index(tagContract, "Verify Homebrew PR automation permission")
	if tagAdmission == -1 || tagHomebrew == -1 || tagAdmission > tagHomebrew {
		t.Error("tag contract must validate all exact Code Admission contexts before exposing Homebrew credentials")
	}

	start := strings.Index(workflow, "- name: Open stable Homebrew formula PR")
	if start == -1 {
		t.Fatal("release workflow is missing the stable Homebrew PR step")
	}
	end := strings.Index(workflow[start:], "- name: Open beta Homebrew formula PR")
	if end == -1 {
		t.Fatal("release workflow is missing the beta Homebrew PR step after the stable step")
	}
	section := workflow[start : start+end]
	for _, required := range []string{
		"github.repository_owner == 'DingTalk-Real-AI'",
		"needs.release-contract.outputs.channel == 'stable'",
		"./scripts/release/publish-homebrew-formula.sh",
		"secrets.HOMEBREW_PR_TOKEN",
		"DWS_TAP_PR_REPOSITORY",
		"automation/homebrew-${{ needs.release-contract.outputs.release_version }}",
	} {
		if !strings.Contains(section, required) {
			t.Errorf("Homebrew publication step is missing %q", required)
		}
	}
	if strings.Contains(section, "secrets.GITHUB_TOKEN") {
		t.Error("Homebrew Formula PRs must use the dedicated token so their CI is triggered")
	}
	stableNPM := strings.Index(workflow, "- name: Publish missing version to npm channel")
	if stableNPM == -1 || start > stableNPM {
		t.Fatal("Homebrew PR creation must run before npm so a failure is safely rerunnable")
	}
}

func TestReleaseWorkflowOpensVersionedHomebrewPRForBetaTags(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("Abs(release.yml) error = %v", err)
	}
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(data)

	start := strings.Index(workflow, "- name: Open beta Homebrew formula PR")
	if start == -1 {
		t.Fatal("release workflow is missing the beta Homebrew PR step")
	}
	end := strings.Index(workflow[start:], "- name: Reverify exact immutable npm package")
	if end == -1 {
		t.Fatal("release workflow is missing the post-Homebrew npm verification step")
	}
	section := workflow[start : start+end]
	for _, required := range []string{
		"github.repository_owner == 'DingTalk-Real-AI'",
		"needs.release-contract.outputs.channel == 'prerelease'",
		"dist/homebrew/dingtalk-workspace-cli-beta.rb",
		"Formula/dingtalk-workspace-cli-beta.rb",
		"secrets.HOMEBREW_PR_TOKEN",
		"automation/homebrew-beta-${{ needs.release-contract.outputs.release_version }}",
	} {
		if !strings.Contains(section, required) {
			t.Errorf("beta Homebrew PR step is missing %q", required)
		}
	}
	if strings.Contains(section, "secrets.GITHUB_TOKEN") {
		t.Error("Homebrew beta Formula PRs must use the dedicated token so their CI is triggered")
	}
}

func TestReleaseStaysDraftUntilFinalizedAssetDigestsMatch(t *testing.T) {
	t.Parallel()

	goreleaserPath, err := filepath.Abs(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("Abs(.goreleaser.yaml) error = %v", err)
	}
	goreleaserData, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", goreleaserPath, err)
	}
	if !strings.Contains(string(goreleaserData), "draft: true") {
		t.Fatal("GoReleaser must keep the release as Draft during post-processing")
	}

	finalizePath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "finalize-github-release.sh"))
	if err != nil {
		t.Fatalf("Abs(finalize-github-release.sh) error = %v", err)
	}
	finalizeData, err := os.ReadFile(finalizePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", finalizePath, err)
	}
	finalize := string(finalizeData)

	upload := strings.Index(finalize, "gh release upload")
	digestFailure := strings.Index(finalize, "release asset digest mismatch")
	publish := strings.Index(finalize, "gh release edit")
	if upload == -1 || digestFailure == -1 || publish == -1 || !(upload < digestFailure && digestFailure < publish) {
		t.Fatal("Draft publication must happen after finalized asset upload and digest verification")
	}
}

func TestFinalizeGitHubReleaseDoesNotPublishAfterUploadFailure(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "finalize-github-release.sh"))
	if err != nil {
		t.Fatalf("Abs(finalize-github-release.sh) error = %v", err)
	}

	root := t.TempDir()
	distDir := filepath.Join(root, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", distDir, err)
	}
	for _, name := range []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"checksums.txt",
		"dws-skills.zip",
	} {
		if err := os.WriteFile(filepath.Join(distDir, name), []byte("finalized"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", binDir, err)
	}
	logPath := filepath.Join(root, "gh.log")
	fakeGH := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH_LOG"
if [ "$1" = "release" ] && [ "$2" = "upload" ]; then
  exit 42
fi
if [ "$1" = "release" ] && [ "$2" = "edit" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(fakeGH), 0o755); err != nil {
		t.Fatalf("WriteFile(fake gh) error = %v", err)
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH_LOG="+logPath,
		"GITHUB_REF_NAME=v-test",
		"GITHUB_REPOSITORY=example/dws",
		"DWS_PACKAGE_DIST_DIR="+distDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("finalize-github-release.sh unexpectedly succeeded after upload failure:\n%s", output)
	}

	logData, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%s) error = %v", logPath, readErr)
	}
	logText := string(logData)
	if !strings.Contains(logText, "release upload") {
		t.Fatalf("fake gh did not observe release upload:\n%s", logText)
	}
	if strings.Contains(logText, "release edit") {
		t.Fatalf("Draft release was published after upload failure:\n%s", logText)
	}
}

func TestFinalizeGitHubReleaseCanVerifyWithoutPublishing(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "finalize-github-release.sh"))
	if err != nil {
		t.Fatalf("Abs(finalize-github-release.sh) error = %v", err)
	}

	root := t.TempDir()
	distDir := filepath.Join(root, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", distDir, err)
	}
	assetContent := []byte("finalized")
	for _, name := range []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"checksums.txt",
		"dws-skills.zip",
	} {
		if err := os.WriteFile(filepath.Join(distDir, name), assetContent, 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", binDir, err)
	}
	logPath := filepath.Join(root, "gh.log")
	fakeGH := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH_LOG"
if [ "$1" = "release" ] && [ "$2" = "upload" ]; then
  exit 0
fi
if [ "$1" = "release" ] && [ "$2" = "view" ]; then
  printf '%s\n' "$FAKE_REMOTE_DIGEST"
  exit 0
fi
if [ "$1" = "release" ] && [ "$2" = "edit" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(fakeGH), 0o755); err != nil {
		t.Fatalf("WriteFile(fake gh) error = %v", err)
	}

	digest := sha256.Sum256(assetContent)
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH_LOG="+logPath,
		"FAKE_REMOTE_DIGEST="+fmt.Sprintf("sha256:%x", digest),
		"GITHUB_REF_NAME=v-test",
		"GITHUB_REPOSITORY=example/dws",
		"DWS_PACKAGE_DIST_DIR="+distDir,
		"DWS_PUBLISH_RELEASE=false",
		"DWS_RELEASE_DIGEST_ATTEMPTS=1",
		"DWS_RELEASE_DIGEST_RETRY_DELAY=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("finalize-github-release.sh error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(string(output), "keeping release v-test as Draft") {
		t.Fatalf("finalizer did not report preserved Draft:\n%s", output)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", logPath, err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "release upload") || !strings.Contains(logText, "release view") {
		t.Fatalf("finalizer did not upload and verify assets:\n%s", logText)
	}
	if strings.Contains(logText, "release edit") {
		t.Fatalf("finalizer published a release configured to remain Draft:\n%s", logText)
	}
}
