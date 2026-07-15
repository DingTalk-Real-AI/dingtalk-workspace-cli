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

func postGoreleaserEnv(t *testing.T, distDir, releaseBaseURL string) []string {
	t.Helper()

	binDir := t.TempDir()
	fakeCodesign := filepath.Join(binDir, "codesign")
	if err := os.WriteFile(fakeCodesign, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake codesign) error = %v", err)
	}

	return append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DWS_PACKAGE_VERSION=v0.0.0",
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
	cmd.Env = postGoreleaserEnv(t, distDir, "https://downloads.example.com/dws/releases/v1.2.3")
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
		`version "0.0.0"`,
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

	// Verify checksums.txt was updated to include skills zip
	checksumsData, err := os.ReadFile(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("ReadFile(checksums.txt) error = %v", err)
	}
	if !strings.Contains(string(checksumsData), "dws-skills.zip") {
		t.Fatalf("checksums.txt missing dws-skills.zip entry:\n%s", string(checksumsData))
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
	env := postGoreleaserEnv(t, distDir, "https://downloads.example.com/dws/releases/v1.2.3-beta.4")
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
	cmd.Env = postGoreleaserEnv(t, distDir, "https://downloads.example.com/dws/releases/v9.9.9")
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
	cmd.Env = postGoreleaserEnv(t, distDir, "https://downloads.example.com/dws/releases/v0.0.0")
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

	postProcess := strings.Index(workflow, "./scripts/release/post-goreleaser.sh")
	upload := strings.Index(workflow, "Upload finalized signed assets to release")
	if postProcess == -1 || upload == -1 || upload < postProcess {
		t.Fatalf("finalized asset upload must run after post-goreleaser.sh")
	}

	if !strings.Contains(workflow[upload:], "./scripts/release/finalize-github-release.sh") {
		t.Fatal("release workflow must delegate atomic finalization to finalize-github-release.sh")
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

	for _, required := range []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"checksums.txt",
		"dws-skills.zip",
		"gh release upload",
		"gh release view",
		"--clobber",
		"release asset digest mismatch",
	} {
		if !strings.Contains(finalize, required) {
			t.Errorf("finalized asset upload is missing %q", required)
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
	goReleaser := strings.Index(workflow, "Run GoReleaser")
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

	upload := strings.Index(workflow, "Upload finalized signed assets to release")
	verifyJob := strings.Index(workflow, "verify-darwin-signatures:")
	publishJob := strings.Index(workflow, "publish-release:")
	if upload == -1 || verifyJob == -1 || publishJob == -1 || !(upload < verifyJob && verifyJob < publishJob) {
		t.Fatal("finalized Draft assets must be uploaded, Apple-verified, and only then published")
	}

	codesign := strings.Index(workflow[verifyJob:publishJob], "codesign --verify --strict --verbose=4")
	publish := strings.Index(workflow[publishJob:], `gh release edit "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" --draft=false`)
	if codesign == -1 || publish == -1 {
		t.Fatal("macOS codesign verification and explicit Draft publication are required")
	}

	buildSection := workflow[upload:verifyJob]
	for _, required := range []string{
		`DWS_PUBLISH_RELEASE: "false"`,
		"actions/upload-artifact@v4",
		"finalized-release-dist",
	} {
		if !strings.Contains(buildSection, required) {
			t.Errorf("Draft build stage is missing %q", required)
		}
	}

	verifySection := workflow[verifyJob:publishJob]
	for _, required := range []string{
		"runs-on: macos-latest",
		"gh release download",
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
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
		"Publish verified Draft release",
		"Publish stable to npm",
		"Publish prerelease to npm beta",
		"Open stable Homebrew formula PR",
		"Open beta Homebrew formula PR",
		"DingTalk-Real-AI/dingtalk-workspace-cli.git",
		"secrets.GITHUB_TOKEN",
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
		"Check Homebrew PR automation token",
		"secrets.HOMEBREW_PR_TOKEN",
		"HOMEBREW_PR_TOKEN is required to open Formula PRs from official releases",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing Homebrew PR token preflight %q", required)
		}
	}

	start := strings.Index(workflow, "- name: Open stable Homebrew formula PR")
	if start == -1 {
		t.Fatal("release workflow is missing the stable Homebrew PR step")
	}
	end := strings.Index(workflow[start:], "- name: Mirror release to Gitee")
	if end == -1 {
		t.Fatal("release workflow is missing the post-Homebrew Gitee step")
	}
	section := workflow[start : start+end]
	for _, required := range []string{
		"github.repository_owner == 'DingTalk-Real-AI'",
		"!contains(github.ref_name, '-')",
		"./scripts/release/publish-homebrew-formula.sh",
		"secrets.HOMEBREW_PR_TOKEN",
		"DWS_TAP_PR_REPOSITORY",
		"automation/homebrew-${{ github.ref_name }}",
	} {
		if !strings.Contains(section, required) {
			t.Errorf("Homebrew publication step is missing %q", required)
		}
	}
	if strings.Contains(section, "secrets.GITHUB_TOKEN") {
		t.Error("Homebrew Formula PRs must use the dedicated token so their CI is triggered")
	}
	stableNPM := strings.Index(workflow, "- name: Publish stable to npm")
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
	end := strings.Index(workflow[start:], "- name: Sync release to China OSS mirror")
	if end == -1 {
		t.Fatal("release workflow is missing the post-Homebrew OSS step")
	}
	section := workflow[start : start+end]
	for _, required := range []string{
		"github.repository_owner == 'DingTalk-Real-AI'",
		"contains(github.ref_name, '-')",
		"dist/homebrew/dingtalk-workspace-cli-beta.rb",
		"Formula/dingtalk-workspace-cli-beta.rb",
		"secrets.HOMEBREW_PR_TOKEN",
		"automation/homebrew-beta-${{ github.ref_name }}",
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
