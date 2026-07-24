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
	"regexp"
	"runtime"
	"sort"
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

func TestPackageManagerVersionVerificationReadsRawBinary(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "verify-package-managers.sh"))
	if err != nil {
		t.Fatalf("Abs(verify-package-managers.sh) error = %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	script := string(data)
	for _, binary := range []string{`"$vendor_bin"`, `"$prefix/bin/dws"`} {
		want := `LC_ALL=C grep -aFq "v$EXPECTED_VERSION" ` + binary
		if !strings.Contains(script, want) {
			t.Errorf("package-manager verifier is missing raw binary marker check %q", want)
		}
	}
	if strings.Contains(script, `strings "$vendor_bin"`) || strings.Contains(script, `strings "$prefix/bin/dws"`) {
		t.Fatal("package-manager verifier still requires the version marker to occupy a strings(1) line")
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

func releaseWorkflowRunScript(t *testing.T, workflow, stepName, nextStepName string) string {
	t.Helper()
	section := releaseWorkflowSection(
		t,
		workflow,
		"      - name: "+stepName+"\n",
		"\n      - name: "+nextStepName+"\n",
	)
	const runMarker = "        run: |\n"
	start := strings.Index(section, runMarker)
	if start == -1 {
		t.Fatalf("release workflow step %q is missing a run block", stepName)
	}

	lines := strings.Split(section[start+len(runMarker):], "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "          ") {
			t.Fatalf("release workflow step %q has an unexpected run indentation: %q", stepName, line)
		}
		lines[i] = strings.TrimPrefix(line, "          ")
	}
	return strings.Join(lines, "\n")
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
	if got := strings.Count(workflow, checksCall); got != 3 {
		t.Fatalf("release workflow Checks API call count = %d, want one tag check, one preflight check, and one Formula-parent check", got)
	}
	if got := strings.Count(workflow, paginatedCall); got != 3 {
		t.Fatalf("release workflow paginated Checks API call count = %d, want one tag check, one preflight check, and one Formula-parent check", got)
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

	tagChecks := releaseWorkflowSection(
		t,
		sections["tag"],
		"      - name: Require successful Code Admission contexts on the sealed commit\n",
		"\n      - name: Require delivered previous stable baseline\n",
	)
	tagGovernanceToken := releaseWorkflowSection(
		t,
		sections["tag"],
		"      - name: Require release governance token\n",
		"\n      - name: Require immutable releases governance\n",
	)
	tagImmutableGovernance := releaseWorkflowSection(
		t,
		sections["tag"],
		"      - name: Require immutable releases governance\n",
		"\n      - name: Require successful beta delivery before stable promotion\n",
	)
	for name, section := range map[string]string{
		"sealed Code Admission recheck":       tagChecks,
		"sealed governance token check":       tagGovernanceToken,
		"sealed immutable governance recheck": tagImmutableGovernance,
	} {
		if strings.Contains(section, "\n        if:") {
			t.Errorf("%s must run for push, recovery, and cloud create modes", name)
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
	immutableGovernance := strings.Index(preflight, "Require immutable releases governance")
	if admission == -1 || immutableGovernance == -1 || admission > immutableGovernance {
		t.Error("governance preflight must validate all exact Code Admission contexts before immutable-release governance")
	}
	for _, forbidden := range []string{
		"HOMEBREW_PR_TOKEN",
		"verify-homebrew-pr-token.sh",
		"DWS_TAP_PR_",
	} {
		if strings.Contains(preflight, forbidden) {
			t.Errorf("governance preflight must not retain obsolete Homebrew PR machinery %q", forbidden)
		}
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
		`release_flow + npm_repair + gitee_repair + oss_repair + governance + recovery`,
		`echo "mode=plan_release"`,
		`echo "mode=create_release"`,
		"name: Authorize repository release requester",
		`if: ${{ steps.mode.outputs.mode == 'plan_release' || steps.mode.outputs.mode == 'create_release' }}`,
		`REQUESTED_BY: ${{ github.actor }}`,
		`TRIGGERED_BY: ${{ github.triggering_actor }}`,
		"github.rest.repos.getCollaboratorPermissionLevel",
		`!["write", "admin"].includes(permission)`,
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
		`'refs/tags/v*' 'refs/tags/withdrawn/v*'`,
		"release ref manifest is empty after fetching allocated tags",
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
	if !strings.Contains(plan, "needs: [dispatch-contract]") {
		t.Error("cloud release planning must start after dispatch validation without serializing behind governance")
	}
	if strings.Contains(plan, "needs: [dispatch-contract, governance-preflight]") ||
		strings.Contains(plan, "needs.governance-preflight.result") {
		t.Error("read-only cloud planning must run in parallel with governance preflight")
	}
	if strings.Contains(plan, "refs/tags/v refs/tags/withdrawn/v") {
		t.Error("cloud release planning must use wildcard ref patterns that match the seal API prefixes")
	}
	workflowHeader := workflow[:strings.Index(workflow, "\njobs:\n")]
	if strings.Contains(workflowHeader, "\n  push:\n") {
		t.Error("new releases must not bypass cloud authorization through a tag-push trigger")
	}
	if regexp.MustCompile(`(?m)^      release_confirmation:$`).MatchString(workflowHeader) ||
		strings.Contains(workflow, "release_confirmation must be exactly: PUBLISH") {
		t.Error("cloud publication must not retain the redundant typed confirmation gate")
	}

	for _, required := range []string{
		"name: Seal cloud release tag",
		`name: ${{ inputs.release_channel == 'stable' && 'release-stable' || 'release-beta' }}`,
		"actions: read",
		"contents: write",
		"name: Verify release environment governance before sealing",
		`"GET /repos/{owner}/{repo}/environments/{environment_name}"`,
		`environment.can_admins_bypass !== false`,
		`branchPolicy?.protected_branches !== true`,
		`branchPolicy?.custom_branch_policies !== false`,
		`reviewerRules.length !== 0`,
		`reviewerRules[0].prevent_self_review !== true`,
		`permission.data.permission !== "admin"`,
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
	createRef := strings.Index(seal, "github.rest.git.createRef")
	environmentPolicy := strings.Index(seal, "Verify release environment governance before sealing")
	createTagStep := strings.Index(seal, "Create one immutable annotated release tag")
	if environmentPolicy == -1 || createTagStep == -1 || environmentPolicy > createTagStep {
		t.Fatal("release Environment policy must be verified before the first tag write")
	}
	visibilityRead := strings.LastIndex(seal, "github.rest.git.getRef")
	if createRef == -1 || visibilityRead == -1 || visibilityRead < createRef {
		t.Fatal("cloud release seal must confirm tag-ref visibility after creating the ref")
	}
	postCreate := seal[createRef:]
	for _, required := range []string{
		"error.status === 404",
		"error.status === 429",
		"error.status >= 500",
		"setTimeout",
		`.data.object.type !== "tag"`,
		"!== expectedTagObject",
		".data.tag !== version",
		".data.object.sha !== commit",
		".data.message !==",
	} {
		if !strings.Contains(seal, required) {
			t.Errorf("post-create tag verification is missing %q", required)
		}
	}
	if strings.Count(postCreate, "github.rest.git.getRef") < 2 {
		t.Error("post-create tag handling must both adopt an exact uncertain write and confirm final ref visibility")
	}
	if !regexp.MustCompile(`(?m)const \w*[Rr]etry\w*[Dd]elays\w* = \[(?:0,?\s*){1}[\d,\s]+\];`).MatchString(seal) {
		t.Error("post-create tag verification must use a bounded retry policy")
	}
	for _, required := range []string{
		"`${version} tag ref`",
		"() => github.rest.git.getRef",
	} {
		if !strings.Contains(postCreate, required) {
			t.Errorf("the bounded visibility retry must be used after createRef: missing %q", required)
		}
	}
	retryLoop := strings.LastIndex(seal[:createRef], "for (const delayMs of")
	retrySleep := strings.LastIndex(seal[:createRef], "await sleep(delayMs)")
	if retryLoop == -1 || retrySleep == -1 || retryLoop > retrySleep {
		t.Error("the bounded visibility retry must be used after createRef, not only declared")
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

func TestReleaseWorkflowParallelizesSealedValidationWithoutWeakeningPublication(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	releaseContract := releaseWorkflowSection(t, workflow, "  release-contract:\n", "\n  release:\n")
	build := releaseWorkflowSection(t, workflow, "  release:\n", "\n  verify-darwin-signatures:\n")
	validation := releaseWorkflowSection(t, workflow, "  release-validation:\n", "\n  release-plan:\n")
	publish := releaseWorkflowSection(t, workflow, "  publish-release:\n", "\n  publish-channels:\n")
	plan := releaseWorkflowSection(t, workflow, "  release-plan:\n", "\n  seal-release:\n")

	for _, required := range []string{
		"id: candidate",
		"previous_stable: ${{ steps.candidate.outputs.previous_stable }}",
		"previous_stable_commit: ${{ steps.candidate.outputs.previous_stable_commit }}",
		"from_beta_commit: ${{ steps.candidate.outputs.from_beta_commit }}",
		`printf '%s=%s\n' "$key" "$value" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(plan, required) {
			t.Errorf("cloud release plan does not export sealed contract evidence %q", required)
		}
	}

	for _, required := range []string{
		"channel: ${{ steps.metadata.outputs.channel }}",
		"previous_stable: ${{ steps.metadata.outputs.previous_stable }}",
		"name: Resolve validated release metadata",
		`if test "$DISPATCH_MODE" = create_release`,
		`test -n "$previous_stable_commit"`,
		`test -n "$from_beta_commit"`,
		`unsupported validated release channel`,
		`if: ${{ needs.dispatch-contract.outputs.mode != 'create_release' }}`,
	} {
		if !strings.Contains(releaseContract, required) {
			t.Errorf("release contract cloud fast path is missing %q", required)
		}
	}

	for _, required := range []string{
		"name: Validate sealed release (${{ matrix.check }})",
		"needs: [release-contract]",
		"fail-fast: false",
		"- automation",
		"- compatibility",
		"- e2e",
		"verify-github-tag-authority.sh",
		"go test -v -count=1 -timeout=5m ./test/scripts/... -run '^TestRelease'",
		"check-command-compatibility.sh",
		"test-multi-profile-e2e.sh",
	} {
		if !strings.Contains(validation, required) {
			t.Errorf("parallel release validation is missing %q", required)
		}
	}

	if strings.Contains(build, "test-multi-profile-e2e.sh") {
		t.Error("multi-profile E2E must not serialize GoReleaser")
	}
	goreleaser := strings.Index(build, "name: Build release artifacts without publishing")
	setupNode := strings.Index(build, "name: Setup Node.js")
	archiveTools := strings.Index(build, "name: Install archive tooling")
	if goreleaser == -1 || setupNode == -1 || archiveTools == -1 ||
		goreleaser > setupNode || goreleaser > archiveTools {
		t.Error("post-processing-only Node and archive tooling must be installed after GoReleaser")
	}

	for _, required := range []string{
		"needs: [release-contract, release-validation, release, verify-darwin-signatures]",
		"needs.release-validation.result == 'success'",
	} {
		if !strings.Contains(publish, required) {
			t.Errorf("immutable publication is not blocked by parallel validation %q", required)
		}
	}
}

func TestReleaseFingerprintRefPatternsMatchAllAllocatedTags(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	mustRun(t, repo, "git", "init", "-b", "main")
	mustRun(t, repo, "git", "config", "user.name", "Release Fingerprint Test")
	mustRun(t, repo, "git", "config", "user.email", "release-fingerprint@example.com")
	mustWriteFile(t, filepath.Join(repo, "tracked"), []byte("fixture\n"), 0o644)
	mustRun(t, repo, "git", "add", "tracked")
	mustRun(t, repo, "git", "commit", "-m", "fixture")

	allocatedTags := []string{
		"v1.0.52",
		"v1.0.53-beta.5",
		"withdrawn/v1.0.51",
	}
	for _, tag := range allocatedTags {
		mustRun(t, repo, "git", "tag", "-a", tag, "-m", "Release "+tag)
	}
	mustRun(t, repo, "git", "tag", "-a", "release/v1.0.52", "-m", "unrelated namespace")
	legacy := exec.Command(
		"git", "for-each-ref", "--format=%(refname)=%(objectname)",
		"refs/tags/v", "refs/tags/withdrawn/v",
	)
	legacy.Dir = repo
	legacyOutput, err := legacy.CombinedOutput()
	if err != nil {
		t.Fatalf("legacy git for-each-ref error = %v\noutput:\n%s", err, legacyOutput)
	}
	if strings.TrimSpace(string(legacyOutput)) != "" {
		t.Fatalf("legacy component patterns unexpectedly matched flat release refs:\n%s", legacyOutput)
	}

	cmd := exec.Command(
		"git", "for-each-ref", "--format=%(refname)=%(objectname)",
		"refs/tags/v*", "refs/tags/withdrawn/v*",
	)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git for-each-ref error = %v\noutput:\n%s", err, output)
	}
	got := strings.Split(strings.TrimSpace(string(output)), "\n")
	sort.Strings(got)

	want := make([]string, 0, len(allocatedTags))
	for _, tag := range allocatedTags {
		object := strings.TrimSpace(mustOutput(t, repo, "git", "rev-parse", "refs/tags/"+tag))
		want = append(want, "refs/tags/"+tag+"="+object)
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("release ref set differs from the seal API set\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	workflowDigest := sha256.Sum256([]byte(strings.Join(got, "\n") + "\n"))
	sealDigest := sha256.Sum256([]byte(strings.Join(want, "\n") + "\n"))
	if workflowDigest != sealDigest {
		t.Fatalf("release ref fingerprint differs from seal fingerprint: workflow=%x seal=%x", workflowDigest, sealDigest)
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

func TestReleaseWorkflowRequiresOSSOnlyWhenMirrorIsEnabled(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	releaseContract := releaseWorkflowSection(t, workflow, "  release-contract:\n", "\n  release:\n")
	targetAuthority := releaseWorkflowSection(
		t,
		releaseContract,
		"      - name: Resolve and verify exact release target\n",
		"\n      - name: Check out repository\n",
	)
	ossStep := releaseWorkflowSection(
		t,
		workflow,
		"      - name: Sync release artifacts to China OSS mirror\n",
		"\n  mirror-gitee-release:\n",
	)

	for _, required := range []string{
		`if: ${{ needs.release-contract.outputs.oss_mirror == 'enabled' }}`,
		`run: ./scripts/release/sync-to-oss.sh`,
		`DWS_REQUIRE_OSS: "1"`,
	} {
		if !strings.Contains(ossStep, required) {
			t.Errorf("opt-in OSS publication is missing %q", required)
		}
	}
	for _, required := range []string{
		`OSS_MIRROR: ${{ vars.ENABLE_OSS_MIRROR == 'true' && 'enabled' || 'deferred' }}`,
		`OSS-Mirror: ${ossMirror}`,
		`core.setOutput("oss_mirror", ossMirror);`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("immutable OSS release policy is missing %q", required)
		}
	}
	if strings.Contains(ossStep, "vars.ENABLE_OSS_MIRROR") {
		t.Error("channel publication must use the immutable tag policy, not the current repository variable")
	}
	for _, required := range []string{
		`const ossMirror = tagFields.has("OSS-Mirror")`,
		`? tagFields.get("OSS-Mirror")`,
		`: "enabled";`,
		`!["enabled", "deferred"].includes(ossMirror)`,
		`core.setOutput("oss_mirror", ossMirror);`,
	} {
		if !strings.Contains(targetAuthority, required) {
			t.Errorf("release target OSS policy authority is missing %q", required)
		}
	}
}

func TestReleaseWorkflowChannelRepairUsesSealedReleaseAuthority(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	dispatch := releaseWorkflowSection(t, workflow, "  dispatch-contract:\n", "\n  governance-preflight:\n")
	start := strings.Index(workflow, "  repair-channel:\n")
	if start == -1 {
		t.Fatal("release workflow is missing the channel repair job")
	}
	end := strings.Index(workflow[start:], "\n  release-plan:\n")
	if end == -1 {
		t.Fatal("release workflow channel repair job is missing its end marker")
	}
	repair := workflow[start : start+end]
	authority := releaseWorkflowSection(
		t,
		repair,
		"      - name: Verify immutable release authority\n",
		"\n      - name: Require sealed OSS policy for repair\n",
	)
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
		"Require sealed OSS policy for repair",
		"OSS repair is unavailable because this immutable release deferred the OSS channel.",
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
	for _, required := range []string{
		`const tagFields = new Map();`,
		`const ossMirror = tagFields.has("OSS-Mirror")`,
		`? tagFields.get("OSS-Mirror")`,
		`: "enabled";`,
		`!["enabled", "deferred"].includes(ossMirror)`,
		`core.setOutput("oss_mirror", ossMirror);`,
	} {
		if !strings.Contains(authority, required) {
			t.Errorf("channel repair tag policy authority is missing %q", required)
		}
	}
	npmRepair := releaseWorkflowSection(t, workflow, "  repair-npm:\n", "\n  release-delivery-gate:\n")
	if strings.Contains(npmRepair, `const ossMirror`) || strings.Contains(npmRepair, `core.setOutput("oss_mirror"`) {
		t.Error("npm repair must not parse or export the channel-only OSS policy")
	}
	if strings.Contains(repair, "ENABLE_OSS_MIRROR") {
		t.Error("OSS repair must use the immutable tag policy, not the current repository variable")
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
	for _, forbidden := range []string{
		"authorize-recovery:",
		"environment: release-recovery",
		"needs.authorize-recovery",
		"AUTHORIZE_RECOVERY_RESULT",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("machine-verified release recovery must not wait on manual approval %q", forbidden)
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

func TestReleaseWorkflowRerunAdoptsOnlyItsExactSeal(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	sealStart := strings.Index(workflow, "  seal-release:\n")
	if sealStart == -1 {
		t.Fatal("release workflow is missing the cloud seal job")
	}
	seal := workflow[sealStart:]

	for _, required := range []string{
		`const currentAttempt = Number(process.env.GITHUB_RUN_ATTEMPT)`,
		`Release-Run: ${context.runId}`,
		`Release-Run-Attempt: ${attempt}`,
		`Requested-By-ID: ${context.payload.sender?.id || ""}`,
		`Workflow-Commit: ${context.sha}`,
		`Allocation-Fingerprint: ${expectedFingerprint}`,
		`ref.ref === ` + "`refs/tags/${version}`",
		`existingRef = await github.rest.git.getRef`,
		`sealedAttempt > currentAttempt`,
		`.data.message !== messageForAttempt(sealedAttempt)`,
		`if (branchMoved)`,
		`if (!existingRef)`,
		`basehead: ` + "`${commit}...${currentBranchCommit}`",
		`["ahead", "identical"].includes(comparison.data.status)`,
		`if (existingRef)`,
		`continuing failed jobs without recovery approval`,
	} {
		if !strings.Contains(seal, required) {
			t.Errorf("same-run release rerun exact-seal contract is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"github.rest.git.updateRef",
		"github.rest.git.deleteRef",
		"environment: release-recovery",
		"needs.authorize-recovery",
	} {
		if strings.Contains(seal, forbidden) {
			t.Errorf("same-run exact seal reuse must not mutate the tag or require approval: found %q", forbidden)
		}
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
			condition: `if: ${{ !cancelled() && (github.event_name == 'push' || (needs.dispatch-contract.result == 'success' && needs.dispatch-contract.outputs.mode == 'recover_release') || (needs.dispatch-contract.result == 'success' && needs.dispatch-contract.outputs.mode == 'create_release' && needs.governance-preflight.result == 'success' && needs.release-plan.result == 'success' && needs.seal-release.result == 'success')) }}`,
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
			condition: `if: ${{ !cancelled() && needs.release-contract.result == 'success' && needs.release-validation.result == 'success' && needs.release.result == 'success' && needs.verify-darwin-signatures.result == 'success' }}`,
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
		"- governance-preflight",
		"- release-contract",
		"- release-validation",
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
		`RELEASE_VALIDATION_RESULT: ${{ needs.release-validation.result }}`,
		`RELEASE_PLAN_RESULT: ${{ needs.release-plan.result }}`,
		`SEAL_RELEASE_RESULT: ${{ needs.seal-release.result }}`,
		"require_publication",
		`require_result release-contract "$RELEASE_CONTRACT_RESULT" success`,
		`require_result release-validation "$RELEASE_VALIDATION_RESULT" success`,
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
	for _, forbidden := range []string{
		"- authorize-recovery",
		"AUTHORIZE_RECOVERY_RESULT",
		"needs.authorize-recovery",
		"require_result authorize-recovery",
	} {
		if strings.Contains(gate, forbidden) {
			t.Errorf("release delivery gate must not retain recovery approval dependency %q", forbidden)
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
		"Publish stable Homebrew formula",
		"Publish beta Homebrew formula",
		"DingTalk-Real-AI/dingtalk-workspace-cli.git",
		"secrets.GITHUB_TOKEN",
	} {
		if !strings.Contains(publishSection, required) {
			t.Errorf("post-verification publication stage is missing %q", required)
		}
	}
}

func TestReleaseWorkflowPublishesStableHomebrewFormulaDirectly(t *testing.T) {
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
	for _, forbidden := range []string{
		"Verify Homebrew PR automation permission",
		"verify-homebrew-pr-token.sh",
		"DWS_TAP_PR_REPOSITORY",
		"DWS_TAP_PR_BRANCH",
		"Open stable Homebrew formula PR",
		"Open beta Homebrew formula PR",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("release workflow must not retain Homebrew PR machinery %q", forbidden)
		}
	}

	start := strings.Index(workflow, "- name: Publish stable Homebrew formula")
	if start == -1 {
		t.Fatal("release workflow is missing direct stable Homebrew publication")
	}
	end := strings.Index(workflow[start:], "- name: Publish beta Homebrew formula")
	if end == -1 {
		t.Fatal("release workflow is missing direct beta Homebrew publication after the stable step")
	}
	section := workflow[start : start+end]
	for _, required := range []string{
		"github.repository_owner == 'DingTalk-Real-AI'",
		"needs.release-contract.outputs.channel == 'stable'",
		"scripts/release/publish-homebrew-formula.sh",
		"secrets.HOMEBREW_PR_TOKEN",
	} {
		if !strings.Contains(section, required) {
			t.Errorf("direct stable Homebrew publication is missing %q", required)
		}
	}
	stableNPM := strings.Index(workflow, "- name: Publish missing version to npm channel")
	if stableNPM == -1 || start > stableNPM {
		t.Fatal("Homebrew publication must run before npm so a failure is safely rerunnable")
	}
}

func TestReleaseWorkflowPublishesBetaHomebrewFormulaDirectly(t *testing.T) {
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

	start := strings.Index(workflow, "- name: Publish beta Homebrew formula")
	if start == -1 {
		t.Fatal("release workflow is missing direct beta Homebrew publication")
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
	} {
		if !strings.Contains(section, required) {
			t.Errorf("direct beta Homebrew publication is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"DWS_TAP_PR_REPOSITORY",
		"DWS_TAP_PR_BRANCH",
		"automation/homebrew-beta-",
	} {
		if strings.Contains(section, forbidden) {
			t.Errorf("direct beta Homebrew publication must not open a PR: found %q", forbidden)
		}
	}
}

func TestReleaseWorkflowSealsOnlyVerifiedFormulaCommitContexts(t *testing.T) {
	t.Parallel()
	workflow := readReleaseWorkflow(t)
	publishJob := releaseWorkflowSection(t, workflow, "  publish-release:\n", "\n  publish-channels:\n")
	if !strings.Contains(publishJob, "checks: write") {
		t.Fatal("Formula-only delivery must receive checks: write in the already write-capable publication job")
	}

	for _, required := range []string{
		"id: homebrew-stable",
		"id: homebrew-beta",
		`FORMULA_CHANGED: ${{ steps.homebrew-stable.outputs.formula_changed || steps.homebrew-beta.outputs.formula_changed }}`,
		`FORMULA_COMMIT: ${{ steps.homebrew-stable.outputs.published_commit || steps.homebrew-beta.outputs.published_commit }}`,
	} {
		if !strings.Contains(publishJob, required) {
			t.Errorf("Homebrew publisher output wiring is missing %q", required)
		}
	}

	seal := releaseWorkflowSection(
		t,
		publishJob,
		"      - name: Seal Formula-only Code Admission contexts\n",
		"\n      - name: Reverify exact immutable npm package\n",
	)
	for _, required := range []string{
		`const commit = process.env.FORMULA_COMMIT`,
		`commitResponse.data.sha === commit`,
		`commitResponse.data.parents.length === 1`,
		`commitResponse.data.commit.message === expectedMessage`,
		`commitResponse.data.author?.login === "github-actions[bot]"`,
		`commitResponse.data.committer?.login === "github-actions[bot]"`,
		`files.length === 1`,
		`files[0].filename === formulaPath`,
		`["added", "modified"].includes(files[0].status)`,
		`const parent = commitResponse.data.parents[0].sha`,
		`github.rest.checks.listForRef`,
		`ref: parent`,
		`run.head_sha !== parent`,
		`run.app?.slug !== "github-actions"`,
		`run.conclusion !== "success"`,
		`github.rest.repos.getContent`,
		`path: formulaPath`,
		`ref: commit`,
		`actualFormula.equals(expectedFormula)`,
		`github.rest.repos.compareCommitsWithBasehead`,
		`basehead: ` + "`${commit}...${branch.data.commit.sha}`",
		`["ahead", "identical"].includes(comparison.data.status)`,
		`github.rest.checks.create`,
		`head_sha: commit`,
		`status: "completed"`,
		`conclusion: "success"`,
	} {
		if !strings.Contains(seal, required) {
			t.Errorf("Formula-only Code Admission sealing is missing %q", required)
		}
	}
	for _, context := range expectedReleaseAdmissionContexts {
		if strings.Count(seal, fmt.Sprintf("%q", context)) != 1 {
			t.Errorf("Formula-only Code Admission must derive exactly one %q context from the reviewed list", context)
		}
	}
	if strings.Count(seal, "github.rest.checks.create") != 1 {
		t.Error("Formula-only checks must be created only by the single verified context loop")
	}
	for _, forbidden := range []string{
		"head_sha: context.sha",
		"head_sha: parent",
		"head_sha: branch.data.commit.sha",
	} {
		if strings.Contains(seal, forbidden) {
			t.Errorf("Formula-only Code Admission must not mark an unverified head green: found %q", forbidden)
		}
	}

	createCheck := strings.Index(seal, "github.rest.checks.create")
	for name, marker := range map[string]string{
		"single-parent Formula-only identity": "const exactFormulaCommit",
		"successful parent contexts":          "invalidParentContexts.length > 0",
		"byte-for-byte Formula content":       "actualFormula.equals(expectedFormula)",
		"default-branch containment":          `["ahead", "identical"].includes(comparison.data.status)`,
	} {
		index := strings.Index(seal, marker)
		if index == -1 || createCheck == -1 || index > createCheck {
			t.Errorf("%s must be verified before any success check is created", name)
		}
	}
}

func TestReleaseWorkflowWaitsForNPMDistTagPropagation(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	script := releaseWorkflowRunScript(
		t,
		workflow,
		"Verify npm channel delivery",
		"Sync release artifacts to China OSS mirror",
	)

	tests := []struct {
		name        string
		sequence    string
		wantSuccess bool
		wantCalls   int
		wantSleeps  int
		wantOutput  string
	}{
		{
			name:        "stale beta converges to target",
			sequence:    "1.0.53-beta.6\n1.0.53-beta.6\n1.0.53-beta.7\n",
			wantSuccess: true,
			wantCalls:   3,
			wantSleeps:  2,
		},
		{
			name:       "stale beta never converges",
			sequence:   "1.0.53-beta.6\n",
			wantCalls:  12,
			wantSleeps: 11,
			wantOutput: "still reports older v1.0.53-beta.6 after 12 attempts",
		},
		{
			name:       "permanent registry error fails immediately",
			sequence:   "__NPM_ERROR__\n",
			wantCalls:  1,
			wantSleeps: 0,
			wantOutput: "permanent npm registry error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			fakeBin := filepath.Join(root, "bin")
			if err := os.MkdirAll(fakeBin, 0o755); err != nil {
				t.Fatalf("MkdirAll(%s) error = %v", fakeBin, err)
			}
			sequencePath := filepath.Join(root, "sequence")
			statePath := filepath.Join(root, "state")
			npmLogPath := filepath.Join(root, "npm.log")
			sleepLogPath := filepath.Join(root, "sleep.log")
			mustWriteFile(t, sequencePath, []byte(test.sequence), 0o644)
			mustWriteFile(t, filepath.Join(fakeBin, "npm"), []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NPM_CALL_LOG"
test "$*" = "view dingtalk-workspace-cli dist-tags.beta --registry=https://registry.npmjs.org --prefer-online" || {
  echo "unexpected npm mutation: $*" >&2
  exit 97
}
call=0
if test -f "$NPM_STATE"; then call="$(cat "$NPM_STATE")"; fi
call=$((call + 1))
printf '%s\n' "$call" > "$NPM_STATE"
value="$(sed -n "${call}p" "$NPM_SEQUENCE")"
if test -z "$value"; then value="$(tail -n 1 "$NPM_SEQUENCE")"; fi
if test "$value" = "__NPM_ERROR__"; then
  echo "permanent npm registry error" >&2
  exit 42
fi
printf '%s\n' "$value"
`), 0o755)
			mustWriteFile(t, filepath.Join(fakeBin, "sleep"), []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$SLEEP_CALL_LOG"
`), 0o755)

			repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
			if err != nil {
				t.Fatalf("Abs(repository root) error = %v", err)
			}
			cmd := exec.Command("sh", "-c", script)
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"NPM_TAG=beta",
				"SEMVER=1.0.53-beta.7",
				"NPM_SEQUENCE="+sequencePath,
				"NPM_STATE="+statePath,
				"NPM_CALL_LOG="+npmLogPath,
				"SLEEP_CALL_LOG="+sleepLogPath,
			)
			output, runErr := cmd.CombinedOutput()
			if test.wantSuccess && runErr != nil {
				t.Fatalf("npm delivery verification error = %v\noutput:\n%s", runErr, output)
			}
			if !test.wantSuccess && runErr == nil {
				t.Fatalf("npm delivery verification unexpectedly succeeded\noutput:\n%s", output)
			}
			if test.wantOutput != "" && !strings.Contains(string(output), test.wantOutput) {
				t.Errorf("npm delivery verification output is missing %q:\n%s", test.wantOutput, output)
			}

			npmLog, err := os.ReadFile(npmLogPath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", npmLogPath, err)
			}
			npmCalls := strings.Split(strings.TrimSpace(string(npmLog)), "\n")
			if got := len(npmCalls); got != test.wantCalls {
				t.Errorf("npm view call count = %d, want %d; log:\n%s", got, test.wantCalls, npmLog)
			}
			if strings.Contains(string(npmLog), "dist-tag add") || strings.Contains(string(npmLog), "publish") {
				t.Errorf("delivery verification must remain read-only; log:\n%s", npmLog)
			}

			sleepCalls := 0
			if sleepLog, err := os.ReadFile(sleepLogPath); err == nil {
				sleepCalls = len(strings.Fields(string(sleepLog)))
			} else if !os.IsNotExist(err) {
				t.Fatalf("ReadFile(%s) error = %v", sleepLogPath, err)
			}
			if sleepCalls != test.wantSleeps {
				t.Errorf("sleep call count = %d, want %d", sleepCalls, test.wantSleeps)
			}
		})
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
