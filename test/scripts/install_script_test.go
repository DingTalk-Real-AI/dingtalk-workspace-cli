package scripts_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
)

func assertSkillProvenance(t *testing.T, home, skillDir, name, source string) {
	t.Helper()
	state, readable, err := skillstate.Read(home)
	if err != nil || !readable {
		t.Fatalf("read unified Skill state: %#v, %v, %v", state, readable, err)
	}
	var provenance skillprovenance.Record
	for _, record := range state.ManagedSkills {
		if record.Name == name {
			provenance = record
			break
		}
	}
	if provenance.Name == "" || provenance.Source != source || provenance.Version == "" {
		t.Fatalf("Skill provenance %s = %#v", name, provenance)
	}
	digest, err := skillprovenance.DigestDir(skillDir)
	if err != nil {
		t.Fatalf("digest Skill directory for %s: %v", name, err)
	}
	if provenance.Digest != digest {
		t.Fatalf("Skill provenance digest %s = %q, want %q", name, provenance.Digest, digest)
	}
}

type installSourceFixture struct {
	root       string
	scriptPath string
	stubRoot   string
	fakeHome   string
}

func newInstallSourceFixture(t *testing.T) *installSourceFixture {
	t.Helper()

	repoScript, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("Abs(install.sh) error = %v", err)
	}
	scriptData, err := os.ReadFile(repoScript)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", repoScript, err)
	}

	root := t.TempDir()
	scriptPath := filepath.Join(root, "scripts", "install.sh")
	mustWriteFile(t, scriptPath, scriptData, 0o755)
	// resolve_source_root requires both go.mod and cmd/. The source installer
	// tests need only that layout plus small representative skill trees.
	mustWriteFile(t, filepath.Join(root, "go.mod"), []byte("module example.com/dws-install-test\n"), 0o644)
	mustWriteFile(t, filepath.Join(root, "cmd", ".keep"), nil, 0o644)
	mustWriteFile(t, filepath.Join(root, "skills", "mono", "SKILL.md"), []byte("# Test skill\n"), 0o644)
	mustWriteFile(t, filepath.Join(root, "skills", "multi", "dingtalk-test", "SKILL.md"), []byte("# Test split skill\n"), 0o644)
	mustWriteFile(t, filepath.Join(root, "skills", "multi", "dws-shared", "SKILL.md"), []byte("# Test shared skill\n"), 0o644)

	stubRoot := filepath.Join(root, "stubs")
	makeStub := `#!/bin/sh
set -eu
dir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -C) dir="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ -n "$dir" ] && printf 'fake-binary\n' > "$dir/dws"
`
	mustWriteFile(t, filepath.Join(stubRoot, "make"), []byte(makeStub), 0o755)
	mustWriteFile(t, filepath.Join(stubRoot, "go"), []byte("#!/bin/sh\ntrue\n"), 0o755)

	return &installSourceFixture{
		root:       root,
		scriptPath: scriptPath,
		stubRoot:   stubRoot,
		fakeHome:   filepath.Join(root, "home"),
	}
}

func (f *installSourceFixture) env(extra ...string) []string {
	return f.envWithSkillMode("mono", extra...)
}

// envWithSkillMode builds the fixture environment. An empty mode omits
// DWS_SKILL_MODE entirely so the installer exercises its own default (multi)
// resolution; any inherited DWS_SKILL_MODE is filtered out either way.
func (f *installSourceFixture) envWithSkillMode(mode string, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DWS_SKILL_MODE=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"HOME="+f.fakeHome,
		"PATH="+f.stubRoot+":"+os.Getenv("PATH"),
		"DWS_VERSION=latest",
		"DWS_SKILLS_ONLY=0",
	)
	if mode != "" {
		env = append(env, "DWS_SKILL_MODE="+mode)
	}
	return append(env, extra...)
}

func TestInstallScriptSourceModeInstallsBinary(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")

	cmd := exec.Command("sh", fixture.scriptPath)
	cmd.Env = fixture.env(
		"DWS_INSTALL_DIR="+installDir,
		"DWS_INSTALL_NAME=dws-test",
		"DWS_NO_SKILLS=1",
	)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	got := string(output)
	for _, want := range []string{
		"Installing dws from source checkout: " + fixture.root,
		"Install dir: " + installDir,
		"Binary installed:",
		installDir + "/dws-test",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install output missing %q:\n%s", want, got)
		}
	}

	binaryPath := filepath.Join(installDir, "dws-test")
	binaryData, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", binaryPath, err)
	}
	if string(binaryData) != "fake-binary\n" {
		t.Fatalf("installed binary content = %q, want fake-binary", string(binaryData))
	}
}

func TestInstallScriptSourceModeInstallsSkillsIntoAgentsDir(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")

	// Gate for index>0 agent dirs (matches build/npm/install.js): parent must exist.
	if err := os.MkdirAll(filepath.Join(fixture.fakeHome, ".cursor"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.cursor) error = %v", err)
	}

	cmd := exec.Command("sh", fixture.scriptPath)
	cmd.Env = fixture.env(
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	skillPath := filepath.Join(fixture.fakeHome, ".agents", "skills", "dws", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("Stat(%s) error = %v\noutput:\n%s", skillPath, err, string(output))
	}
	duplicatePath := filepath.Join(fixture.fakeHome, ".cursor", "skills", "dws")
	if _, err := os.Lstat(duplicatePath); !os.IsNotExist(err) {
		t.Fatalf("universal Cursor root must not duplicate canonical Skill: Lstat(%s) = %v\noutput:\n%s", duplicatePath, err, string(output))
	}
}

func TestInstallPowerShellScriptInstallsToAgentsDir(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatalf("Abs(install.ps1) error = %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}

	text := string(data)
	if !strings.Contains(text, ".agents\\skills") {
		t.Fatalf("install.ps1 missing .agents\\skills")
	}
	if !strings.Contains(text, ".cursor\\skills") {
		t.Fatalf("install.ps1 missing .cursor\\skills (AGENT_DIRS must match build/npm/install.js)")
	}
}

func TestInstallDevappScriptsReportSkillRollbackFailures(t *testing.T) {
	t.Parallel()

	checks := []struct {
		path      string
		required  string
		forbidden string
	}{
		{filepath.Join("..", "..", "scripts", "install-devapp.sh"), "Skill rollback failed; backup retained at", "[ -z \"$backup\" ] || mv \"$backup\" \"$dest\" 2>/dev/null || true"},
		{filepath.Join("..", "..", "scripts", "install-devapp.ps1"), "Die \"Skill install failed:", "(skill install skipped:"},
		{filepath.Join("..", "..", "scripts", "install-event.sh"), "Skill rollback failed; backup retained at", "[ -z \"$backup\" ] || mv \"$backup\" \"$dest\" 2>/dev/null || true"},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, err)
		}
		text := string(data)
		if !strings.Contains(text, check.required) {
			t.Errorf("%s missing explicit failure contract %q", check.path, check.required)
		}
		if strings.Contains(text, check.forbidden) {
			t.Errorf("%s still contains silent failure path %q", check.path, check.forbidden)
		}
	}
}

func TestInstallScriptsRemoveJunctionStagesLexically(t *testing.T) {
	t.Parallel()

	installPs1 := filepath.Join("..", "..", "scripts", "install.ps1")
	data, err := os.ReadFile(installPs1)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", installPs1, err)
	}
	text := string(data)
	for _, want := range []string{
		"function Remove-LinkStageRoot",
		"Remove-LinkStageRoot -StageRoot $stageRoot",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("install.ps1 missing junction-safe removal contract %q", want)
		}
	}
	// The rollback of published junction paths must go through the lexical
	// remover: Windows PowerShell 5.1 follows reparse points during
	// Remove-Item -Recurse and could delete the canonical store's contents.
	// Assert on the Restore-MultiSkillSet section so identity-anchor
	// refactors keep the contract without breaking on variable naming.
	begin := strings.Index(text, "function Restore-MultiSkillSet")
	if begin < 0 {
		t.Fatal("install.ps1 missing Restore-MultiSkillSet")
	}
	end := strings.Index(text[begin+1:], "\nfunction ")
	if end < 0 {
		t.Fatal("install.ps1 Restore-MultiSkillSet section not terminated")
	}
	restore := text[begin : begin+1+end]
	if !strings.Contains(restore, "Remove-SkillPathLexically") {
		t.Errorf("install.ps1 Restore-MultiSkillSet must remove published paths lexically")
	}
	if strings.Contains(restore, "Remove-Item") && strings.Contains(restore, "-Recurse") {
		t.Errorf("install.ps1 Restore-MultiSkillSet still contains a recursive removal:\n%s", restore)
	}

	devappPs1 := filepath.Join("..", "..", "scripts", "install-devapp.ps1")
	devappData, err := os.ReadFile(devappPs1)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", devappPs1, err)
	}
	for _, want := range []string{
		"function Remove-DevLinkStageRoot",
		"Remove-DevLinkStageRoot $stageRoot",
	} {
		if !strings.Contains(string(devappData), want) {
			t.Errorf("install-devapp.ps1 missing junction-safe removal contract %q", want)
		}
	}

	// The backup prune must also be reparse-safe: a stamp tree can hold
	// junction/symlink entries at any depth (victims are collected before
	// the physical-equality filter), and Windows PowerShell 5.1 follows
	// reparse points during Remove-Item -Recurse.
	for scriptName, pruneFn := range map[string]string{
		"install.ps1":        "Remove-OldSkillBackups",
		"install-devapp.ps1": "Remove-OldDevSkillBackups",
	} {
		scriptData, err := os.ReadFile(filepath.Join("..", "..", "scripts", scriptName))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptName, err)
		}
		body := extractPowerShellFunction(t, string(scriptData), pruneFn)
		if strings.Contains(body, "-Recurse") {
			t.Errorf("%s %s must delegate to child-first lexical removal, not Remove-Item -Recurse:\n%s", scriptName, pruneFn, body)
		}
	}
}

// extractPowerShellFunction returns a `function Name { ... }` body from a
// PowerShell script, terminated by the next top-level function declaration.
func extractPowerShellFunction(t *testing.T, text, name string) string {
	t.Helper()
	begin := strings.Index(text, "function "+name)
	if begin < 0 {
		t.Fatalf("function %s not found", name)
	}
	end := strings.Index(text[begin+1:], "\nfunction ")
	if end < 0 {
		end = len(text) - begin - 1
	}
	return text[begin : begin+1+end]
}

func TestInstallEventScriptDegradesFailedAgentLoudly(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer test is for unix-like hosts")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("unsupported test os %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("unsupported test arch %s", runtime.GOARCH)
	}

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")

	assetName := "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", releaseDir, err)
	}
	writeTarGz(t, filepath.Join(releaseDir, assetName), map[string]string{
		"dws": "fake-event-binary\n",
	})
	writeZip(t, filepath.Join(releaseDir, "dws-skills.zip"), map[string]string{
		"multi/dingtalk-event/SKILL.md":  "event skill user_im_message_receive_o2o\n",
		"multi/dingtalk-shared/SKILL.md": "shared prerequisite\n",
		"multi/dingtalk-misc/SKILL.md":   "clean misc oa routing\n",
		"mono/SKILL.md":                  "mono skill user_im_message_receive_o2o\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))

	// A regular file where an agent skill base belongs makes that one agent
	// target uninstallable, while later agents (.zcode) stay reachable.
	mustWriteFile(t, filepath.Join(fakeHome, ".aider-desk", "skills"), []byte("not a directory\n"), 0o644)
	mustWriteFile(t, filepath.Join(fakeHome, ".zcode", "keep.txt"), []byte("zcode home\n"), 0o644)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_INSTALL_DIR="+installDir,
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME="+assetName,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install-event.sh should fail loudly when an agent target fails:\n%s", string(output))
	}
	got := string(output)
	for _, want := range []string{
		"Agent 目标安装失败，已跳过",
		"dingtalk-event 分发到 Agent 目录失败",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install-event output missing degrade report %q:\n%s", want, got)
		}
	}
	// The failed agent must not abort the loop: later agents still get links.
	if _, err := os.Stat(filepath.Join(fakeHome, ".zcode", "skills", "dingtalk-event", "SKILL.md")); err != nil {
		t.Fatalf("later agent link missing after per-agent degrade: %v\noutput:\n%s", err, got)
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-event", "SKILL.md")); err != nil {
		t.Fatalf("canonical Skill missing: %v", err)
	}
}

func TestInstallPowerShellJunctionsResolvePhysicalRoot(t *testing.T) {
	// Prefer the advertised Windows PowerShell 5.1 irm|iex surface on Windows;
	// pwsh 7 can mask junction removal and resolution differences.
	powerShellNames := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		powerShellNames = []string{"powershell", "pwsh"}
	}
	pwsh := ""
	for _, name := range powerShellNames {
		if candidate, lookupErr := exec.LookPath(name); lookupErr == nil {
			pwsh = candidate
			break
		}
	}
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	// Probe Test-SamePhysicalSkillRoot with both link flavors: junctions need
	// no privilege on Windows but silently no-op on non-Windows pwsh builds;
	// symlinks need no privilege on Unix but may fail without Developer Mode
	// on Windows. Exit 4 marks "no link flavor testable on this host".
	probe := prefix + `
$canonical = Join-Path $env:DWS_TEST_HOME '.agents\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $canonical -Force | Out-Null
$other = Join-Path $env:DWS_TEST_HOME '.agents\skills\dingtalk-shared'
New-Item -ItemType Directory -Path $other -Force | Out-Null
$linkParent = Join-Path $env:DWS_TEST_HOME '.zcode\skills'
New-Item -ItemType Directory -Path $linkParent -Force | Out-Null
$asserted = 0
$junction = Join-Path $linkParent 'dingtalk-chat'
New-Item -ItemType Junction -Path $junction -Target $canonical -ErrorAction SilentlyContinue | Out-Null
if (Test-Path -LiteralPath $junction) {
    $asserted = 1
    if (!(Test-SamePhysicalSkillRoot -Left $junction -Right $canonical)) { exit 3 }
    $junctionChain = Join-Path $linkParent 'dingtalk-chat-chain'
    New-Item -ItemType Junction -Path $junctionChain -Target $junction -ErrorAction Stop | Out-Null
    if (!(Test-SamePhysicalSkillRoot -Left $junctionChain -Right $canonical)) { exit 5 }
}
$symlink = Join-Path $linkParent 'dingtalk-shared'
New-Item -ItemType SymbolicLink -Path $symlink -Target $other -ErrorAction SilentlyContinue | Out-Null
if (Test-Path -LiteralPath $symlink) {
    $asserted = 1
    if (!(Test-SamePhysicalSkillRoot -Left $symlink -Right $other)) { exit 3 }
    $symlinkChain = Join-Path $linkParent 'dingtalk-shared-chain'
    New-Item -ItemType SymbolicLink -Path $symlinkChain -Target $symlink -ErrorAction Stop | Out-Null
    if (!(Test-SamePhysicalSkillRoot -Left $symlinkChain -Right $other)) { exit 5 }
}
if ($asserted -eq 0) { exit 4 }
if (Test-SamePhysicalSkillRoot -Left $canonical -Right $other) { exit 3 }
if (Test-Path -LiteralPath $junction) { exit 0 }
exit 4
`
	probePath := filepath.Join(t.TempDir(), "junction-probe.ps1")
	mustWriteFile(t, probePath, []byte(probe), 0o644)
	home := t.TempDir()
	probeCmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", probePath)
	probeCmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	probeOutput, probeErr := probeCmd.CombinedOutput()
	if probeErr != nil {
		exitCode := 0
		if exitErr, ok := probeErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if exitCode != 4 {
			t.Fatalf("link must resolve to its physical target (lexical Resolve-Path regression):\n%s", string(probeOutput))
		}
		return // junctions unavailable on this host (non-Windows pwsh): probed via symlink only
	}

	// A rerun over an already-linked agent base must be idempotent: the
	// junction is recognized as published and never re-backed-up.
	install := prefix + `
if (!(Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME)) { exit 2 }
exit 0
`
	installPath := filepath.Join(t.TempDir(), "junction-idempotent.ps1")
	mustWriteFile(t, installPath, []byte(install), 0o644)
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(multi, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".zcode", "v2", "config.json"), []byte("{}\n"), 0o644)

	runInstall := func() string {
		cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", installPath)
		cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home, "DWS_TEST_MULTI="+multi)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("PowerShell junction idempotency harness failed: %v\n%s", err, string(output))
		}
		return string(output)
	}
	countBackups := func() int {
		entries, err := os.ReadDir(filepath.Join(home, ".dws", "skill-backups"))
		if err != nil {
			return 0
		}
		return len(entries)
	}
	runInstall()
	first := countBackups()
	runInstall()
	if second := countBackups(); second != first {
		t.Fatalf("rerun re-backed-up already-published junctions (backup churn): first=%d second=%d", first, second)
	}
	linkData, err := os.ReadFile(filepath.Join(home, ".zcode", "skills", "dingtalk-chat", "SKILL.md"))
	if err != nil || string(linkData) != "new chat\n" {
		t.Fatalf("linked ZCode Skill content mismatch: %v %q", err, string(linkData))
	}
}

func TestInstallPowerShellPublicationRacePreservesConcurrentDirectory(t *testing.T) {
	powerShellNames := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		powerShellNames = []string{"powershell", "pwsh"}
	}
	pwsh := ""
	for _, name := range powerShellNames {
		if candidate, err := exec.LookPath(name); err == nil {
			pwsh = candidate
			break
		}
	}
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$root = $env:DWS_TEST_HOME
$stage = Join-Path $root 'stage-payload'
$dest = Join-Path $root 'agent\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $stage -Force | Out-Null
New-Item -ItemType Directory -Path $dest -Force | Out-Null
Set-Content -LiteralPath (Join-Path $dest 'user-data.txt') -Value 'keep'
try {
    Move-SkillPath -Source $stage -Destination $dest
    exit 11
} catch {}
if (!(Test-Path -LiteralPath $stage -PathType Container)) { exit 12 }
if (Test-Path -LiteralPath (Join-Path $dest 'stage-payload')) { exit 13 }
if (!(Test-Path -LiteralPath (Join-Path $dest 'user-data.txt') -PathType Leaf)) { exit 14 }

# Simulate a later failure after publication, with another process replacing
# our link before rollback. Identity-aware rollback must retain its directory.
$canonical = Join-Path $root '.agents\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $canonical -Force | Out-Null
$publishedPath = Join-Path $root 'published-link'
$linkType = if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) { 'Junction' } else { 'SymbolicLink' }
New-Item -ItemType $linkType -Path $publishedPath -Target $canonical -ErrorAction Stop | Out-Null
$record = New-PublishedSkillLinkRecord -Path $publishedPath
Remove-SkillPathLexically -Path $publishedPath
New-Item -ItemType Directory -Path $publishedPath -Force | Out-Null
Set-Content -LiteralPath (Join-Path $publishedPath 'concurrent-user-data.txt') -Value 'keep'
$restored = Restore-MultiSkillSet -Published @($record) -Backups @()
if ($restored) { exit 15 }
if (!(Test-Path -LiteralPath (Join-Path $publishedPath 'concurrent-user-data.txt') -PathType Leaf)) { exit 16 }

# Copied mono/multi publications use the same identity-protected rollback.
$copySource = Join-Path $root 'copy-source'
$copyPath = Join-Path $root 'published-copy'
New-Item -ItemType Directory -Path $copySource -Force | Out-Null
Set-Content -LiteralPath (Join-Path $copySource 'SKILL.md') -Value 'transaction copy'
Copy-SkillPathLexically -Source $copySource -Destination $copyPath
$copyRecord = New-PublishedSkillCopyRecord -Path $copyPath -Source $copySource
Remove-SkillPathLexically -Path $copyPath
New-Item -ItemType Directory -Path $copyPath -Force | Out-Null
Set-Content -LiteralPath (Join-Path $copyPath 'concurrent-copy-data.txt') -Value 'keep'
$copyRestored = Restore-MultiSkillSet -Published @($copyRecord) -Backups @()
if ($copyRestored) { exit 17 }
if (!(Test-Path -LiteralPath (Join-Path $copyPath 'concurrent-copy-data.txt') -PathType Leaf)) { exit 18 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "publication-race.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+t.TempDir())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell publication race must retain concurrent directory: %v\n%s", err, string(output))
	}
}

// The ownership-marker contract (skillBackupMarkerFile / skillBackupMarkerBody,
// declared in skill_backup_prune_test.go) is shared by the Go core
// (internal/upgrade/paths.go), install.sh, both PowerShell installers, and
// build/npm/install.js: every stamp root DWS creates carries the marker, and
// pruning only deletes stamp-named directories whose marker verifies.

// seedSkillBackupOwnershipFixture fills ~/.dws/skill-backups with five
// verified stamps (marker bytes exactly as the Go core writes them, proving
// cross-surface acceptance) plus three foreign stamp-shaped siblings: two
// without a marker and one with a wrong marker body.
func seedSkillBackupOwnershipFixture(t *testing.T, home string) {
	t.Helper()
	backupRoot := filepath.Join(home, ".dws", "skill-backups")
	for _, stamp := range []string{
		"20260101-000001", "20260101-000002", "20260101-000003",
		"20260101-000004", "20260101-000005",
	} {
		mustWriteFile(t, filepath.Join(backupRoot, stamp, "placeholder", "SKILL.md"), []byte(stamp+"\n"), 0o644)
		mustWriteFile(t, filepath.Join(backupRoot, stamp, skillBackupMarkerFile), []byte(skillBackupMarkerBody), 0o644)
	}
	for _, stamp := range []string{"20260102-000001", "20260102-000002", "20260101-000000"} {
		mustWriteFile(t, filepath.Join(backupRoot, stamp, "user-data.txt"), []byte("foreign:"+stamp+"\n"), 0o644)
	}
	mustWriteFile(t, filepath.Join(backupRoot, "20260101-000000", skillBackupMarkerFile), []byte("dws skill backup v2\n"), 0o644)
}

// assertSkillBackupOwnershipResult checks the post-prune contract shared by
// both PowerShell installers: the four oldest verified stamps are gone
// (three by the final prune, one by the backup's internal prune), exactly
// five verified stamps survive, every foreign stamp-shaped sibling is intact,
// and the current-run stamp carries the exact marker bytes next to the moved
// payload.
func assertSkillBackupOwnershipResult(t *testing.T, home string) {
	t.Helper()
	backupRoot := filepath.Join(home, ".dws", "skill-backups")
	for _, pruned := range []string{"20260101-000001", "20260101-000002", "20260101-000003", "20260101-000004"} {
		if _, err := os.Lstat(filepath.Join(backupRoot, pruned)); !os.IsNotExist(err) {
			t.Fatalf("verified stamp %s must be pruned, err = %v", pruned, err)
		}
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", backupRoot, err)
	}
	surviving := map[string]bool{}
	runStamps := []string{}
	for _, entry := range entries {
		surviving[entry.Name()] = true
		if entry.Name() >= "20260801-000000" {
			runStamps = append(runStamps, entry.Name())
		}
	}
	for _, want := range []string{
		"20260101-000005",
		"20260101-000000", "20260102-000001", "20260102-000002",
		"20260103-000001", "20260103-000002", "20260103-000003",
	} {
		if !surviving[want] {
			t.Fatalf("stamp %s must survive pruning, surviving = %v", want, surviving)
		}
	}
	if len(runStamps) != 1 || len(entries) != 8 {
		t.Fatalf("expected the five newest verified stamps, three foreign stamps, and one run stamp, got %d entries (run stamps %v)", len(entries), runStamps)
	}
	runRoot := filepath.Join(backupRoot, runStamps[0])
	marker, err := os.ReadFile(filepath.Join(runRoot, skillBackupMarkerFile))
	if err != nil || string(marker) != skillBackupMarkerBody {
		t.Fatalf("run stamp marker = %q, %v; want exactly %q", string(marker), err, skillBackupMarkerBody)
	}
	if backup := findSkillBackupContent(home, "old copy\n"); backup == "" {
		t.Fatalf("backed-up Skill content missing from run stamp %s", runRoot)
	}
	for _, stamp := range []string{"20260101-000000", "20260102-000001", "20260102-000002"} {
		data, err := os.ReadFile(filepath.Join(backupRoot, stamp, "user-data.txt"))
		if err != nil || string(data) != "foreign:"+stamp+"\n" {
			t.Fatalf("foreign stamp %s must be intact, got %q, %v", stamp, string(data), err)
		}
	}
}

func TestInstallPowerShellPruneVerifiesBackupOwnershipMarker(t *testing.T) {
	powerShellNames := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		powerShellNames = []string{"powershell", "pwsh"}
	}
	pwsh := ""
	for _, name := range powerShellNames {
		if candidate, lookupErr := exec.LookPath(name); lookupErr == nil {
			pwsh = candidate
			break
		}
	}
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$backupRoot = Join-Path $env:DWS_TEST_HOME '.dws\skill-backups'
# Five verified stamps plus three foreign stamp-shaped siblings: the foreign
# entries must not count against the keep limit, so nothing is pruned here.
Remove-OldSkillBackups
foreach ($stamp in @('20260101-000001','20260101-000002','20260101-000003','20260101-000004','20260101-000005','20260101-000000','20260102-000001','20260102-000002')) {
    if (!(Test-Path -LiteralPath (Join-Path $backupRoot $stamp) -PathType Container)) { exit 10 }
}
# A real backup both proves the writer stamps the exact marker bytes and adds
# a current-run root that pruning must keep.
$victim = Join-Path $env:DWS_TEST_HOME '.zcode\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $victim -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $victim 'SKILL.md'), "old copy` + "`n" + `")
if (!(Backup-SkillDir -Dir $victim)) { exit 11 }
if (Test-Path -LiteralPath $victim) { exit 12 }
foreach ($stamp in @('20260103-000001','20260103-000002','20260103-000003')) {
    $stampRoot = Join-Path $backupRoot $stamp
    New-Item -ItemType Directory -Path (Join-Path $stampRoot 'placeholder') -Force | Out-Null
    Write-SkillBackupMarker -Root $stampRoot
}
Remove-OldSkillBackups
exit 0
`
	home := filepath.Join(t.TempDir(), "home")
	seedSkillBackupOwnershipFixture(t, home)
	harnessPath := filepath.Join(t.TempDir(), "skill-backup-marker.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell backup marker harness failed: %v\n%s", err, string(output))
	}
	assertSkillBackupOwnershipResult(t, home)
}

func TestInstallDevappPowerShellPruneVerifiesBackupOwnershipMarker(t *testing.T) {
	pwsh := ""
	if candidate, err := exec.LookPath("pwsh"); err == nil {
		pwsh = candidate
	} else if runtime.GOOS == "windows" {
		if candidate, err := exec.LookPath("powershell"); err == nil {
			pwsh = candidate
		}
	}
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$backupRoot = Join-Path $env:DWS_TEST_HOME '.dws\skill-backups'
# Five verified stamps plus three foreign stamp-shaped siblings: the foreign
# entries must not count against the keep limit, so nothing is pruned here.
Remove-OldDevSkillBackups
foreach ($stamp in @('20260101-000001','20260101-000002','20260101-000003','20260101-000004','20260101-000005','20260101-000000','20260102-000001','20260102-000002')) {
    if (!(Test-PathLexically (Join-Path $backupRoot $stamp))) { exit 10 }
}
$victim = Join-Path $env:DWS_TEST_HOME '.zcode\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $victim -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $victim 'SKILL.md'), "old copy` + "`n" + `")
Backup-DevSkill $victim
if (Test-PathLexically $victim) { exit 12 }
foreach ($stamp in @('20260103-000001','20260103-000002','20260103-000003')) {
    $stampRoot = Join-Path $backupRoot $stamp
    New-Item -ItemType Directory -Path (Join-Path $stampRoot 'placeholder') -Force | Out-Null
    Write-DevSkillBackupMarker $stampRoot
}
Remove-OldDevSkillBackups
exit 0
`
	home := filepath.Join(t.TempDir(), "home")
	seedSkillBackupOwnershipFixture(t, home)
	harnessPath := filepath.Join(t.TempDir(), "devapp-skill-backup-marker.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell devapp backup marker harness failed: %v\n%s", err, string(output))
	}
	assertSkillBackupOwnershipResult(t, home)
}

// lookPowerShellForScripts prefers Windows PowerShell 5.1 (the advertised
// irm|iex surface) on Windows and falls back to pwsh everywhere.
func lookPowerShellForScripts(t *testing.T) string {
	t.Helper()
	powerShellNames := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		powerShellNames = []string{"powershell", "pwsh"}
	}
	for _, name := range powerShellNames {
		if candidate, err := exec.LookPath(name); err == nil {
			return candidate
		}
	}
	t.Skip("PowerShell is not available")
	return ""
}

// TestInstallPowerShellPruneRemovesLinkChildrenWithoutFollowingTargets pins
// the Blocker 2 fix: pruning a stamp root that contains a junction/symlink
// child must delete the link itself while the link target's contents survive.
// Windows PowerShell 5.1 follows reparse points during Remove-Item -Recurse,
// so the prune removes children lexically, recursing real directories only.
func TestInstallPowerShellPruneRemovesLinkChildrenWithoutFollowingTargets(t *testing.T) {
	pwsh := lookPowerShellForScripts(t)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	// Junctions need no privilege on Windows but silently no-op on non-Windows
	// pwsh builds; symlinks need no privilege on Unix. Exit 4 marks "no link
	// flavor testable on this host".
	harness := prefix + `
$backupRoot = Join-Path $env:DWS_TEST_HOME '.dws\skill-backups'
$linkType = if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) { 'Junction' } else { 'SymbolicLink' }
$precious = Join-Path $env:DWS_TEST_HOME 'precious-store'
New-Item -ItemType Directory -Path $precious -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $precious 'target-data.txt'), 'must survive')
# The oldest of six verified stamps is prunable and holds a link child.
$oldest = Join-Path $backupRoot '20200101-000001'
New-Item -ItemType Directory -Path (Join-Path $oldest 'payload') -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $oldest 'payload\SKILL.md'), 'old')
Write-SkillBackupMarker -Root $oldest
$link = Join-Path $oldest 'linked-skill'
New-Item -ItemType $linkType -Path $link -Target $precious -ErrorAction SilentlyContinue | Out-Null
if (!(Test-Path -LiteralPath $link)) { exit 4 }
foreach ($stamp in @('20200101-000002','20200101-000003','20200101-000004','20200101-000005','20200101-000006')) {
    $stampRoot = Join-Path $backupRoot $stamp
    New-Item -ItemType Directory -Path (Join-Path $stampRoot 'payload') -Force | Out-Null
    Write-SkillBackupMarker -Root $stampRoot
}
Remove-OldSkillBackups
if (Test-Path -LiteralPath $oldest) { exit 5 }
if (!(Test-Path -LiteralPath (Join-Path $precious 'target-data.txt') -PathType Leaf)) { exit 6 }
foreach ($stamp in @('20200101-000002','20200101-000003','20200101-000004','20200101-000005','20200101-000006')) {
    if (!(Test-Path -LiteralPath (Join-Path $backupRoot $stamp) -PathType Container)) { exit 7 }
}
exit 0
`
	home := filepath.Join(t.TempDir(), "home")
	harnessPath := filepath.Join(t.TempDir(), "skill-backup-link-prune.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 4 {
			t.Skipf("no junction/symlink flavor can be created on this host:\n%s", string(output))
		}
		t.Fatalf("PowerShell link-aware prune harness failed: %v\n%s", err, string(output))
	}
	if got, err := os.ReadFile(filepath.Join(home, "precious-store", "target-data.txt")); err != nil || string(got) != "must survive" {
		t.Fatalf("link target content = %q, %v; want it to survive pruning", got, err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".dws", "skill-backups"))
	if err != nil {
		t.Fatal(err)
	}
	var surviving []string
	for _, entry := range entries {
		surviving = append(surviving, entry.Name())
	}
	want := []string{"20200101-000002", "20200101-000003", "20200101-000004", "20200101-000005", "20200101-000006"}
	if len(surviving) != len(want) {
		t.Fatalf("after pruning the link-bearing stamp, surviving stamps = %v, want %v", surviving, want)
	}
	for _, stamp := range want {
		if _, err := os.Stat(filepath.Join(home, ".dws", "skill-backups", stamp)); err != nil {
			t.Fatalf("stamp %s must survive pruning: %v", stamp, err)
		}
	}
}

// TestInstallDevappPowerShellPruneRemovesLinkChildrenWithoutFollowingTargets
// is the install-devapp.ps1 twin of the link-aware prune contract above.
func TestInstallDevappPowerShellPruneRemovesLinkChildrenWithoutFollowingTargets(t *testing.T) {
	pwsh := lookPowerShellForScripts(t)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$backupRoot = Join-Path $env:DWS_TEST_HOME '.dws\skill-backups'
$linkType = if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) { 'Junction' } else { 'SymbolicLink' }
$precious = Join-Path $env:DWS_TEST_HOME 'precious-store'
New-Item -ItemType Directory -Path $precious -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $precious 'target-data.txt'), 'must survive')
$oldest = Join-Path $backupRoot '20200101-000001'
New-Item -ItemType Directory -Path (Join-Path $oldest 'payload') -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $oldest 'payload\SKILL.md'), 'old')
Write-DevSkillBackupMarker $oldest
$link = Join-Path $oldest 'linked-skill'
New-Item -ItemType $linkType -Path $link -Target $precious -ErrorAction SilentlyContinue | Out-Null
if (!(Test-PathLexically $link)) { exit 4 }
foreach ($stamp in @('20200101-000002','20200101-000003','20200101-000004','20200101-000005','20200101-000006')) {
    $stampRoot = Join-Path $backupRoot $stamp
    New-Item -ItemType Directory -Path (Join-Path $stampRoot 'payload') -Force | Out-Null
    Write-DevSkillBackupMarker $stampRoot
}
Remove-OldDevSkillBackups
if (Test-PathLexically $oldest) { exit 5 }
if (!(Test-PathLexically (Join-Path $precious 'target-data.txt'))) { exit 6 }
foreach ($stamp in @('20200101-000002','20200101-000003','20200101-000004','20200101-000005','20200101-000006')) {
    if (!(Test-PathLexically (Join-Path $backupRoot $stamp))) { exit 7 }
}
exit 0
`
	home := filepath.Join(t.TempDir(), "home")
	harnessPath := filepath.Join(t.TempDir(), "devapp-skill-backup-link-prune.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 4 {
			t.Skipf("no junction/symlink flavor can be created on this host:\n%s", string(output))
		}
		t.Fatalf("PowerShell devapp link-aware prune harness failed: %v\n%s", err, string(output))
	}
	if got, err := os.ReadFile(filepath.Join(home, "precious-store", "target-data.txt")); err != nil || string(got) != "must survive" {
		t.Fatalf("link target content = %q, %v; want it to survive pruning", got, err)
	}
	for _, stamp := range []string{"20200101-000002", "20200101-000003", "20200101-000004", "20200101-000005", "20200101-000006"} {
		if _, err := os.Stat(filepath.Join(home, ".dws", "skill-backups", stamp)); err != nil {
			t.Fatalf("stamp %s must survive pruning: %v", stamp, err)
		}
	}
}

// TestInstallPowerShellBackupNeverClaimsUnverifiedStampRoot pins the
// collision-loop fix: a stamp root that exists without the verified
// ownership marker is foreign data, so Backup-SkillDir bumps to a -N root
// instead of writing its marker (and later prune eligibility) into it.
// Foreign roots are seeded for the current second and the next nine so the
// stamp Backup-SkillDir picks is always a pre-existing unverified root.
func TestInstallPowerShellBackupNeverClaimsUnverifiedStampRoot(t *testing.T) {
	pwsh := lookPowerShellForScripts(t)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$backupRoot = Join-Path $env:DWS_TEST_HOME '.dws\skill-backups'
New-Item -ItemType Directory -Path $backupRoot -Force | Out-Null
$now = [DateTime]::UtcNow
$seeded = @()
for ($i = 0; $i -lt 10; $i++) {
    $stamp = $now.AddSeconds($i).ToString('yyyyMMdd-HHmmss')
    $seeded += $stamp
    $foreign = Join-Path $backupRoot $stamp
    New-Item -ItemType Directory -Path $foreign -Force | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $foreign 'user-data.txt'), 'foreign')
}
$victim = Join-Path $env:DWS_TEST_HOME '.zcode\skills\dingtalk-chat'
New-Item -ItemType Directory -Path $victim -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $victim 'SKILL.md'), 'old copy')
$backupPath = ''
if (!(Backup-SkillDir -Dir $victim -BackupPath ([ref]$backupPath))) { exit 2 }
if (!$backupPath) { exit 3 }
foreach ($stamp in $seeded) {
    $foreign = Join-Path $backupRoot $stamp
    if (Test-Path -LiteralPath (Join-Path $foreign '.dws-skill-backup')) { exit 4 }
    $children = @(Get-ChildItem -LiteralPath $foreign -Force | ForEach-Object { $_.Name })
    if ($children.Count -ne 1 -or $children[0] -ne 'user-data.txt') { exit 5 }
}
# Every plain stamp root was foreign, so the payload must sit in a -N root.
if ($backupPath -notmatch '-[0-9]+[\\/]\.zcode-skills-dingtalk-chat$') { exit 6 }
if (!(Test-Path -LiteralPath (Join-Path $backupPath 'SKILL.md') -PathType Leaf)) { exit 7 }
exit 0
`
	home := filepath.Join(t.TempDir(), "home")
	harnessPath := filepath.Join(t.TempDir(), "skill-backup-collision.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell backup collision harness failed: %v\n%s", err, string(output))
	}
	// The foreign same-second roots stay untouched: no marker, no payload.
	entries, err := os.ReadDir(filepath.Join(home, ".dws", "skill-backups"))
	if err != nil {
		t.Fatal(err)
	}
	foreignIntact, bumped := 0, ""
	for _, entry := range entries {
		children, err := os.ReadDir(filepath.Join(home, ".dws", "skill-backups", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		hasPayload, hasMarker := false, false
		for _, child := range children {
			if child.Name() == ".dws-skill-backup" {
				hasMarker = true
			}
			if child.Name() == ".zcode-skills-dingtalk-chat" {
				hasPayload = true
			}
		}
		if hasPayload {
			bumped = entry.Name()
			if !hasMarker {
				t.Fatalf("backup stamp root %s must carry the ownership marker", bumped)
			}
			continue
		}
		if hasMarker {
			t.Fatalf("foreign stamp root %s must never be marked DWS-owned", entry.Name())
		}
		if len(children) != 1 || children[0].Name() != "user-data.txt" {
			t.Fatalf("foreign stamp root %s must stay untouched, children = %v", entry.Name(), children)
		}
		foreignIntact++
	}
	if foreignIntact != 10 || bumped == "" || !strings.Contains(bumped, "-") {
		t.Fatalf("want 10 untouched foreign roots plus one -N backup root, got %d intact and backup root %q", foreignIntact, bumped)
	}
}

// TestInstallDevappPowerShellCrossDeviceRetractVerifiesPublication pins the
// path-blind retract fix: after a cross-device publish, a post-occupy
// verification failure must retract the destination only through
// Remove-VerifiedDevSkillPublication, so a concurrently replaced destination
// is restored instead of deleted.
func TestInstallDevappPowerShellCrossDeviceRetractVerifiesPublication(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "bundle", "dingtalk-misc")
	destination := filepath.Join(root, "external", "dingtalk-misc")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new dev skill\n"), 0o640)

	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
# The first move throws the cross-device error, so the recoverable path
# stages and publishes a copy; right after that publish another process
# replaces the destination, and post-occupy verification fails.
$script:ReplacedPublished = $false
function Move-DevSkillPath([string]$Source, [string]$Destination) {
    if ($Source -eq $env:DWS_TEST_SOURCE -and $Destination -eq $env:DWS_TEST_DESTINATION) {
        throw [System.IO.IOException]::new("injected cross-device rename", -2147024879)
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
    if (!$script:ReplacedPublished -and $Destination -eq $env:DWS_TEST_DESTINATION) {
        $script:ReplacedPublished = $true
        Microsoft.PowerShell.Management\Move-Item -LiteralPath $Destination (Join-Path $env:DWS_TEST_HOME 'replaced-published') -ErrorAction Stop
        New-Item -ItemType Directory -Path $Destination -Force -ErrorAction Stop | Out-Null
        Set-Content -LiteralPath (Join-Path $Destination 'concurrent-user-data.txt') -Value 'keep'
    }
}
try {
    Move-DevSkillPathRecoverably $env:DWS_TEST_SOURCE $env:DWS_TEST_DESTINATION
    exit 2
} catch {
    if ($_.Exception.Message -notmatch 'state uncertain') { Write-Error $_; exit 3 }
}
if (!(Test-PathLexically (Join-Path $env:DWS_TEST_DESTINATION 'concurrent-user-data.txt'))) { exit 4 }
if (!(Test-PathLexically (Join-Path $env:DWS_TEST_SOURCE 'SKILL.md'))) { exit 5 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-devapp-retract-verify-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_SOURCE="+source,
		"DWS_TEST_DESTINATION="+destination,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell devapp verified retract must retain the concurrent replacement: %v\n%s", err, string(output))
	}
	if got, err := os.ReadFile(filepath.Join(destination, "concurrent-user-data.txt")); err != nil || strings.TrimSpace(string(got)) != "keep" {
		t.Fatalf("concurrent replacement data = %q, %v; want it retained", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(source, "SKILL.md")); err != nil || string(got) != "new dev skill\n" {
		t.Fatalf("source must be retained after the failed cross-device move, got %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, "replaced-published")); err != nil {
		t.Fatalf("the replaced original copy must stay inspectable, not be deleted: %v", err)
	}
	for _, pattern := range []string{".dws-dev-rollback-*", ".dingtalk-misc.cross-device-*"} {
		if matches, err := filepath.Glob(filepath.Join(root, "external", pattern)); err != nil || len(matches) != 0 {
			t.Fatalf("devapp rollback leftovers %s = %v, %v", pattern, matches, err)
		}
	}
}

func TestInstallScriptLinkPublicationRacePreservesConcurrentObjects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.sh main section not found")
	}
	harness := string(data[:cut]) + `
ROOT="$DWS_TEST_HOME/root"
BASE="$ROOT/agent/skills"
CANONICAL="$ROOT/.agents/skills"
BUNDLE="$DWS_TEST_HOME/bundle"
mkdir -p "$CANONICAL/dingtalk-chat" "$BASE" "$BUNDLE/dingtalk-chat" || exit 9
printf 'canonical\n' > "$CANONICAL/dingtalk-chat/SKILL.md" || exit 9
printf 'bundled\n' > "$BUNDLE/dingtalk-chat/SKILL.md" || exit 9

race_case="$1"
dest="$BASE/dingtalk-chat"

# Inject the concurrent writer at the exact instant before publication: the
# publish loop reads every staged link first, so the override occupies the
# destination right before the real creation attempt. The override runs in a
# command-substitution subshell, so only its on-disk effects are asserted.
readlink() {
  case "$1" in
    *.dws-link-set.*dingtalk-chat)
      case "$race_case" in
        file) printf 'must survive\n' > "$dest" ;;
        dir)
          mkdir -p "$dest" || exit 9
          printf 'must survive\n' > "$dest/concurrent-user-data.txt" || exit 9
          ;;
      esac
      ;;
  esac
  command readlink "$1"
}

if link_canonical_skills_to_base "$ROOT" "$BASE" multi "$BUNDLE"; then
  printf 'UNEXPECTED-SUCCESS\n'
  exit 1
fi
case "$race_case" in
  file)
    [ -f "$dest" ] || { printf 'MISSING-FILE\n'; exit 3; }
    [ "$(cat "$dest")" = "must survive" ] || { printf 'FILE-CONTENT\n'; exit 4; }
    ;;
  dir)
    [ -d "$dest" ] || { printf 'MISSING-DIR\n'; exit 3; }
    [ "$(cat "$dest/concurrent-user-data.txt")" = "must survive" ] || { printf 'DIR-CONTENT\n'; exit 4; }
    if [ -n "$(ls -A "$dest" | grep -v '^concurrent-user-data\.txt$')" ]; then
      printf 'NESTED-LEFTOVER\n'
      exit 5
    fi
    ;;
esac
case "$(ls -A "$BASE")" in
  *".dws-link-set."*) printf 'STAGE-LEAK\n'; exit 6 ;;
esac
exit 0
`
	for _, raceCase := range []string{"file", "dir"} {
		harnessPath := filepath.Join(t.TempDir(), "link-publication-race.sh")
		mustWriteFile(t, harnessPath, []byte(harness), 0o755)
		cmd := exec.Command("sh", harnessPath, raceCase)
		cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+t.TempDir())
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install.sh link publication race (%s) must preserve the concurrent object: %v\n%s", raceCase, err, string(output))
		}
	}
}

func TestInstallEventScriptLinkPublicationRacePreservesConcurrentObjects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "\nmain\n")
	if cut < 0 {
		t.Fatal("install-event.sh main call not found")
	}
	harness := string(data[:cut]) + `
ROOT="$DWS_TEST_HOME"
CANONICAL="$ROOT/.agents/skills/dingtalk-chat"
SRC="$ROOT/src/dingtalk-chat"
DEST="$ROOT/agent/skills/dingtalk-chat"
mkdir -p "$CANONICAL" "$SRC" || exit 9
printf 'canonical\n' > "$CANONICAL/SKILL.md" || exit 9
printf 'src\n' > "$SRC/SKILL.md" || exit 9

# Inject the concurrent writer at the exact instant before publication:
# backup_skill_dir is the last call before the link is created at DEST. The
# override runs in a command-substitution subshell, so only its on-disk
# effects are asserted.
backup_skill_dir() {
  case "$RACE_CASE" in
    file) printf 'must survive\n' > "$DEST" || exit 9 ;;
    dir)
      mkdir -p "$DEST" || exit 9
      printf 'must survive\n' > "$DEST/concurrent-user-data.txt" || exit 9
      ;;
  esac
  printf '\n'
}

if link_or_copy_skill "$CANONICAL" "$SRC" "$DEST"; then
  printf 'UNEXPECTED-SUCCESS\n'
  exit 1
fi
case "$RACE_CASE" in
  file)
    [ -f "$DEST" ] || { printf 'MISSING-FILE\n'; exit 3; }
    [ "$(cat "$DEST")" = "must survive" ] || { printf 'FILE-CONTENT\n'; exit 4; }
    ;;
  dir)
    [ -d "$DEST" ] || { printf 'MISSING-DIR\n'; exit 3; }
    [ "$(cat "$DEST/concurrent-user-data.txt")" = "must survive" ] || { printf 'DIR-CONTENT\n'; exit 4; }
    if [ -n "$(ls -A "$DEST" | grep -v '^concurrent-user-data\.txt$')" ]; then
      printf 'NESTED-LEFTOVER\n'
      exit 5
    fi
    ;;
esac
exit 0
`
	for _, raceCase := range []string{"file", "dir"} {
		harnessPath := filepath.Join(t.TempDir(), "event-link-publication-race.sh")
		mustWriteFile(t, harnessPath, []byte(harness), 0o755)
		cmd := exec.Command("sh", harnessPath)
		cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+t.TempDir(), "EVENT_VERSION=1.0.0-test", "RACE_CASE="+raceCase)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install-event.sh link publication race (%s) must preserve the concurrent object: %v\n%s", raceCase, err, string(output))
		}
	}
}

func TestInstallerStandaloneCopyPublicationRacePreservesConcurrentDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}

	for _, scriptName := range []string{"install-event.sh", "install-devapp.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			src := filepath.Join(t.TempDir(), "src-skill")
			dest := filepath.Join(home, "agent", "skills", "dingtalk-misc")
			mustWriteFile(t, filepath.Join(src, "SKILL.md"), []byte("new skill\n"), 0o644)
			mustWriteFile(t, filepath.Join(dest, "SKILL.md"), []byte("old skill\n"), 0o644)

			harness := `. "$DWS_TEST_LIBRARY"
mkdir() {
  if [ "$1" = "$DWS_TEST_DEST" ]; then
    command mkdir -p "$1"
    printf '%s\n' must-survive > "$1/concurrent-user-data.txt"
    return 1
  fi
  command mkdir "$@"
}
if copy_tree "$DWS_TEST_SRC" "$DWS_TEST_DEST"; then
  exit 2
fi
[ -f "$DWS_TEST_DEST/concurrent-user-data.txt" ] || exit 3
[ "$(cat "$DWS_TEST_DEST/concurrent-user-data.txt")" = "must-survive" ] || exit 4
if [ -n "$(ls -A "$DWS_TEST_DEST" | grep -v '^concurrent-user-data\.txt$')" ]; then
  exit 5
fi
[ -n "$(find "$HOME/.dws/skill-backups" -name SKILL.md -print 2>/dev/null)" ] || exit 6
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_SRC="+src,
				"DWS_TEST_DEST="+dest,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s copy publication race must preserve the concurrent object: %v\n%s", scriptName, err, output)
			}
			got, err := os.ReadFile(filepath.Join(dest, "concurrent-user-data.txt"))
			if err != nil || strings.TrimSpace(string(got)) != "must-survive" {
				t.Fatalf("%s concurrent dest = %q, %v", scriptName, got, err)
			}
			if backup := findSkillBackupContent(home, "old skill\n"); backup == "" {
				t.Fatalf("%s original backup must be retained after refused publish", scriptName)
			}
		})
	}
}

func TestInstallScriptsUseGitHubReleaseSkillsAsset(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		filepath.Join("..", "..", "scripts", "install.sh"),
		filepath.Join("..", "..", "scripts", "install-event.sh"),
		filepath.Join("..", "..", "scripts", "install-devapp.sh"),
		filepath.Join("..", "..", "scripts", "install.ps1"),
		filepath.Join("..", "..", "scripts", "install-devapp.ps1"),
		filepath.Join("..", "..", "scripts", "install-skills.sh"),
	} {
		scriptPath, err := filepath.Abs(rel)
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", rel, err)
		}

		data, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
		}

		text := string(data)
		if !strings.Contains(text, "releases/download") || !strings.Contains(text, "dws-skills.zip") {
			t.Fatalf("%s should download dws-skills.zip from GitHub Releases", scriptPath)
		}
		if !strings.Contains(text, "checksums.txt") || !strings.Contains(strings.ToLower(text), "sha256") {
			t.Fatalf("%s should verify dws-skills.zip against release checksums", scriptPath)
		}
		if strings.Contains(text, "archive/refs/heads/main.tar.gz") || strings.Contains(text, "archive/refs/tags/") {
			t.Fatalf("%s should not download skills from repository archive refs", scriptPath)
		}
	}
}

func TestInstallerStandaloneCopyChildRacePreservesConcurrentObjects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}

	// sameName: a concurrently created same-name empty child directory must
	// refuse the publish (a POSIX mv would have replaced it wholesale).
	// foreignName: a different-named entry landing inside the claimed dest
	// mid-publish aborts the transaction with the destination retained.
	for _, scriptName := range []string{"install-event.sh", "install-devapp.sh"} {
		for _, raceCase := range []string{"same-name", "foreign-name"} {
			scriptName, raceCase := scriptName, raceCase
			t.Run(scriptName+"/"+raceCase, func(t *testing.T) {
				t.Parallel()
				scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(scriptPath)
				if err != nil {
					t.Fatal(err)
				}
				cut := strings.LastIndex(string(data), "\nmain\n")
				if cut < 0 {
					t.Fatalf("%s final main invocation not found", scriptName)
				}
				library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
				mustWriteFile(t, library, data[:cut], 0o755)

				home := t.TempDir()
				src := filepath.Join(t.TempDir(), "src-skill")
				dest := filepath.Join(home, "agent", "skills", "dingtalk-misc")
				mustWriteFile(t, filepath.Join(src, "SKILL.md"), []byte("new skill\n"), 0o644)
				if err := os.MkdirAll(filepath.Join(src, "aaa-subdir"), 0o755); err != nil {
					t.Fatal(err)
				}
				mustWriteFile(t, filepath.Join(src, "aaa-subdir", "inner.txt"), []byte("inner\n"), 0o644)
				mustWriteFile(t, filepath.Join(dest, "SKILL.md"), []byte("old skill\n"), 0o644)

				harness := `. "$DWS_TEST_LIBRARY"
case "$RACE_CASE" in
  same-name)
    mkdir() {
      if [ "$1" = "$DWS_TEST_DEST/aaa-subdir" ]; then
        # The concurrent writer takes the child path as an empty directory
        # between the top-level claim and the child publish. POSIX mv would
        # replace it; the no-clobber child primitive must refuse instead.
        command mkdir -p "$1"
        printf '%s\n' must-survive > "$1/concurrent-user-data.txt"
        return 1
      fi
      command mkdir "$@"
    }
    ;;
  foreign-name)
    ln() {
      if [ "$2" = "$DWS_TEST_DEST/SKILL.md" ]; then
        command ln "$@"
        # A different-named foreign entry lands inside the claim while the
        # remaining staged children are still publishing.
        printf '%s\n' must-survive > "$DWS_TEST_DEST/zzz-foreign"
        return 0
      fi
      command ln "$@"
    }
    ;;
esac
if copy_tree "$DWS_TEST_SRC" "$DWS_TEST_DEST"; then
  exit 2
fi
case "$RACE_CASE" in
  same-name)
    [ -f "$DWS_TEST_DEST/aaa-subdir/concurrent-user-data.txt" ] || exit 3
    [ "$(cat "$DWS_TEST_DEST/aaa-subdir/concurrent-user-data.txt")" = "must-survive" ] || exit 4
    [ ! -e "$DWS_TEST_DEST/aaa-subdir/inner.txt" ] || exit 5
    [ ! -e "$DWS_TEST_DEST/SKILL.md" ] || exit 6
    ;;
  foreign-name)
    [ -f "$DWS_TEST_DEST/zzz-foreign" ] || exit 3
    [ "$(cat "$DWS_TEST_DEST/zzz-foreign")" = "must-survive" ] || exit 4
    # The transaction's own children are retracted; only the foreign entry
    # remains in the retained destination.
    if [ -n "$(ls -A "$DWS_TEST_DEST" | grep -v '^zzz-foreign$')" ]; then
      ls -A "$DWS_TEST_DEST" >&2
      exit 5
    fi
    ;;
esac
exit 0
`
				harnessPath := filepath.Join(t.TempDir(), "copy-child-race.sh")
				mustWriteFile(t, harnessPath, []byte(harness), 0o755)
				cmd := exec.Command("sh", harnessPath)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"DWS_TEST_LIBRARY="+library,
					"DWS_TEST_SRC="+src,
					"DWS_TEST_DEST="+dest,
					"RACE_CASE="+raceCase,
				)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s copy child race (%s) must preserve the concurrent object: %v\n%s", scriptName, raceCase, err, string(output))
				}
				if backup := findSkillBackupContent(home, "old skill\n"); backup == "" {
					t.Fatalf("%s original backup must be retained after refused publish (%s)", scriptName, raceCase)
				}
			})
		}
	}
}

func TestInstallEventScriptStaticExpectations(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	text := string(data)

	for _, want := range []string{
		"DingTalk-Real-AI/dingtalk-workspace-cli",
		"releases/latest",
		"EVENT_VERSION",
		"DWS_SKILLS_ONLY",
		"dingtalk-event",
		"dingtalk-shared",
		"dingtalk-misc",
		"user_im_message_receive_o2o",
		".config/opencode/skills",
		"$HOME/.dws/skills/multi/$EVENT_SKILL_NAME",
		"$HOME/.dws/skills/mono",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("install-event.sh missing %q", want)
		}
	}
	for _, avoid := range []string{
		"releases?per_page=30",
		"select(.tag_name",
		"dingtalk-dev",
		"client-secret",
		"--as app",
	} {
		if strings.Contains(text, avoid) {
			t.Fatalf("install-event.sh should not expose old app/dev install content %q", avoid)
		}
	}
}

func TestInstallEventScriptInstallsBinaryAndEventSkills(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer test is for unix-like hosts")
	}

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")

	assetName := "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("unsupported test arch %s", runtime.GOARCH)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("unsupported test os %s", runtime.GOOS)
	}

	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", releaseDir, err)
	}
	writeTarGz(t, filepath.Join(releaseDir, assetName), map[string]string{
		"dws": "fake-event-binary\n",
	})
	writeZip(t, filepath.Join(releaseDir, "dws-skills.zip"), map[string]string{
		"multi/dingtalk-event/SKILL.md":  "event skill user_im_message_receive_o2o\n",
		"multi/dingtalk-shared/SKILL.md": "shared prerequisite\n",
		"multi/dingtalk-misc/SKILL.md":   "clean misc oa routing\n",
		"mono/SKILL.md":                  "mono skill user_im_message_receive_o2o\n",
		"SKILL.md":                       "legacy mono root\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))

	for _, dir := range []string{
		filepath.Join(fakeHome, ".codex"),
		filepath.Join(fakeHome, ".config", "opencode"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	// Simulate both upgrade shapes: an old standalone Event skill and the
	// short-lived folded Event-in-misc layout. An unrelated sibling must stay.
	mustWriteFile(t, filepath.Join(fakeHome, ".agents", "skills", "dingtalk-event", "SKILL.md"), []byte("old standalone event\n"), 0o644)
	mustWriteFile(t, filepath.Join(fakeHome, ".agents", "skills", "dingtalk-misc", "SKILL.md"), []byte("old misc dws event\n"), 0o644)
	mustWriteFile(t, filepath.Join(fakeHome, ".agents", "skills", "dingtalk-misc", "references", "event.md"), []byte("folded event docs\n"), 0o644)
	mustWriteFile(t, filepath.Join(fakeHome, ".agents", "skills", "dingtalk-chat", "SKILL.md"), []byte("keep sibling\n"), 0o644)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_INSTALL_DIR="+installDir,
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME="+assetName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-event.sh error = %v\noutput:\n%s", err, string(output))
	}
	repeat := exec.Command("sh", scriptPath)
	repeat.Env = cmd.Env
	if repeatOutput, repeatErr := repeat.CombinedOutput(); repeatErr != nil {
		t.Fatalf("second install-event.sh run should be idempotent: %v\noutput:\n%s", repeatErr, string(repeatOutput))
	}
	got := string(output)
	for _, want := range []string{
		"Version: v1.0.51",
		"Skill dingtalk-event",
		"Skill dingtalk-shared",
		"Skill dingtalk-misc",
		"Skill dws",
		"dws event consume user_im_message_receive_o2o",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install-event output missing %q:\n%s", want, got)
		}
	}

	binaryData, err := os.ReadFile(filepath.Join(installDir, "dws"))
	if err != nil {
		t.Fatalf("ReadFile(installed dws) error = %v", err)
	}
	if string(binaryData) != "fake-event-binary\n" {
		t.Fatalf("installed binary content = %q", string(binaryData))
	}

	expectedSkills := map[string]string{
		".agents/skills/dingtalk-event/SKILL.md":     "user_im_message_receive_o2o",
		".agents/skills/dingtalk-shared/SKILL.md":    "shared prerequisite",
		".agents/skills/dingtalk-misc/SKILL.md":      "clean misc oa routing",
		".agents/skills/dws/SKILL.md":                "user_im_message_receive_o2o",
		".dws/skills/multi/dingtalk-event/SKILL.md":  "user_im_message_receive_o2o",
		".dws/skills/multi/dingtalk-shared/SKILL.md": "shared prerequisite",
		".dws/skills/multi/dingtalk-misc/SKILL.md":   "clean misc oa routing",
		".dws/skills/mono/SKILL.md":                  "user_im_message_receive_o2o",
	}
	for _, duplicate := range []string{
		filepath.Join(fakeHome, ".codex", "skills", "dingtalk-event"),
		filepath.Join(fakeHome, ".config", "opencode", "skills", "dingtalk-event"),
		filepath.Join(fakeHome, ".codex", "skills", "dws"),
	} {
		if _, err := os.Lstat(duplicate); !os.IsNotExist(err) {
			t.Fatalf("universal Agent duplicate remains at %s: %v", duplicate, err)
		}
	}
	for rel, marker := range expectedSkills {
		p := filepath.Join(fakeHome, filepath.FromSlash(rel))
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v\noutput:\n%s", p, err, got)
		}
		if !strings.Contains(string(data), marker) {
			t.Fatalf("%s does not contain %q: %q", p, marker, string(data))
		}
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-misc", "references", "event.md")); !os.IsNotExist(err) {
		t.Fatalf("folded misc event reference should be removed, stat err=%v", err)
	}
	sibling, err := os.ReadFile(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-chat", "SKILL.md"))
	if err != nil || string(sibling) != "keep sibling\n" {
		t.Fatalf("unrelated sibling changed: data=%q err=%v", sibling, err)
	}
	// The replaced standalone Event skill must sit in a marker-stamped backup
	// root: that marker is what lets a later prune count and recycle it. The
	// event installer archives under the flattened source path, so locate the
	// backup by content and the stamp root two levels above it.
	if got := findSkillBackupContent(fakeHome, "old standalone event\n"); got == "" {
		t.Fatalf("replaced standalone Event skill should be backed up under .dws/skill-backups")
	} else {
		assertSkillBackupMarker(t, filepath.Dir(filepath.Dir(got)))
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-dev")); !os.IsNotExist(err) {
		t.Fatalf("dingtalk-dev should not be installed by install-event.sh, stat err=%v", err)
	}
}

func TestInstallEventScriptSkillsOnlySkipsBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")

	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", releaseDir, err)
	}
	writeZip(t, filepath.Join(releaseDir, "dws-skills.zip"), map[string]string{
		"multi/dingtalk-event/SKILL.md":  "event skill user_im_message_receive_o2o\n",
		"multi/dingtalk-shared/SKILL.md": "shared prerequisite\n",
		"multi/dingtalk-misc/SKILL.md":   "clean misc oa routing\n",
		"mono/SKILL.md":                  "mono skill user_im_message_receive_o2o\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_SKILLS_ONLY=1",
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME=unused",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-event.sh skills-only error = %v\noutput:\n%s", err, string(output))
	}
	if _, err := os.Stat(filepath.Join(installDir, "dws")); !os.IsNotExist(err) {
		t.Fatalf("DWS_SKILLS_ONLY=1 should not install binary, stat err=%v\noutput:\n%s", err, string(output))
	}
	for _, name := range []string{"dingtalk-event", "dingtalk-shared", "dingtalk-misc", "dws"} {
		if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", name, "SKILL.md")); err != nil {
			t.Fatalf("skills-only should install %s: %v\noutput:\n%s", name, err, string(output))
		}
	}
}

func TestInstallEventScriptPreflightFailureDoesNotChangeSkills(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")

	// Deliberately omit dingtalk-misc. The installer must resolve all members
	// from this one bundle before replacing any installed or cached directory.
	writeZip(t, filepath.Join(releaseDir, "dws-skills.zip"), map[string]string{
		"multi/dingtalk-event/SKILL.md":  "new event\n",
		"multi/dingtalk-shared/SKILL.md": "new shared\n",
		"mono/SKILL.md":                  "new mono\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))

	existing := filepath.Join(fakeHome, ".agents", "skills", "dingtalk-event", "SKILL.md")
	mustWriteFile(t, existing, []byte("old event stays\n"), 0o644)

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_SKILLS_ONLY=1",
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME=unused",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install-event.sh should fail when bundle misses misc:\n%s", string(output))
	}
	if !strings.Contains(string(output), "dingtalk-misc not found") {
		t.Fatalf("preflight error should name missing misc:\n%s", string(output))
	}
	data, readErr := os.ReadFile(existing)
	if readErr != nil || string(data) != "old event stays\n" {
		t.Fatalf("preflight failure changed installed event: data=%q err=%v", data, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".dws", "skills")); !os.IsNotExist(statErr) {
		t.Fatalf("preflight failure should not populate cache, stat err=%v", statErr)
	}
}

func TestInstallEventChecksumMismatchPreservesExistingSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer test is for unix-like hosts")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")
	writeZip(t, filepath.Join(releaseDir, "dws-skills.zip"), map[string]string{
		"multi/dingtalk-event/SKILL.md":  "new event\n",
		"multi/dingtalk-shared/SKILL.md": "new shared\n",
		"multi/dingtalk-misc/SKILL.md":   "new misc\n",
		"mono/SKILL.md":                  "new mono\n",
	})
	mustWriteFile(t, filepath.Join(releaseDir, "checksums.txt"), []byte(strings.Repeat("0", 64)+"  dws-skills.zip\n"), 0o644)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))
	existing := filepath.Join(home, ".agents", "skills", "dingtalk-event", "SKILL.md")
	mustWriteFile(t, existing, []byte("old event stays\n"), 0o644)
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_SKILLS_ONLY=1",
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME=unused",
	)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "checksum mismatch") {
		t.Fatalf("checksum mismatch result = %v\n%s", runErr, output)
	}
	if got, err := os.ReadFile(existing); err != nil || string(got) != "old event stays\n" {
		t.Fatalf("existing Skill after checksum mismatch = %q, %v", got, err)
	}
}

func TestInstallEventScriptNoSkillsOnlyInstallsBinary(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer test is for unix-like hosts")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("unsupported test arch %s", runtime.GOARCH)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("unsupported test os %s", runtime.GOOS)
	}

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")
	assetName := "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"

	writeTarGz(t, filepath.Join(releaseDir, assetName), map[string]string{
		"dws": "fake-event-binary\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))
	for _, name := range []string{"dingtalk-event", "dingtalk-shared", "dingtalk-misc"} {
		mustWriteFile(t, filepath.Join(fakeHome, ".agents", "skills", name, "SKILL.md"), []byte("keep "+name+"\n"), 0o644)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"EVENT_VERSION=v1.0.51",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=1",
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME="+assetName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-event.sh no-skills error = %v\noutput:\n%s", err, string(output))
	}
	if _, err := os.Stat(filepath.Join(installDir, "dws")); err != nil {
		t.Fatalf("DWS_NO_SKILLS=1 should install binary: %v\noutput:\n%s", err, string(output))
	}
	for _, name := range []string{"dingtalk-event", "dingtalk-shared", "dingtalk-misc"} {
		data, err := os.ReadFile(filepath.Join(fakeHome, ".agents", "skills", name, "SKILL.md"))
		if err != nil || string(data) != "keep "+name+"\n" {
			t.Fatalf("DWS_NO_SKILLS=1 changed %s: data=%q err=%v\noutput:\n%s", name, data, err, string(output))
		}
	}
}

func TestInstallEventScriptDefaultsToLatestStableRelease(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell installer test is for unix-like hosts")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("unsupported test arch %s", runtime.GOARCH)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("unsupported test os %s", runtime.GOOS)
	}

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")
	releaseDir := filepath.Join(root, "release")
	stubRoot := filepath.Join(root, "stubs")
	assetName := "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"

	writeTarGz(t, filepath.Join(releaseDir, assetName), map[string]string{
		"dws": "fake-event-binary\n",
	})
	writeInstallerFixtureChecksums(t, releaseDir)
	writeFakeCurl(t, filepath.Join(stubRoot, "curl"))
	writeFakeGH(t, filepath.Join(stubRoot, "gh"), "v1.0.51")

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-event.sh"))
	if err != nil {
		t.Fatalf("Abs(install-event.sh) error = %v", err)
	}
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=1",
		"FAKE_RELEASE_DIR="+releaseDir,
		"FAKE_ASSET_NAME="+assetName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-event.sh latest release error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "Version: v1.0.51") {
		t.Fatalf("install-event.sh did not resolve the latest stable version:\n%s", string(output))
	}
}

func TestInstallScriptsUseFlattenedSkillsSourceRoot(t *testing.T) {
	t.Parallel()

	checks := []struct {
		relPath string
		want    string
		avoid   string
	}{
		{
			relPath: filepath.Join("..", "..", "scripts", "install.sh"),
			want:    `skill_src="${root}/skills/mono"`,
			avoid:   `skill_src="${root}/skills/${SKILL_NAME}"`,
		},
		{
			relPath: filepath.Join("..", "..", "scripts", "install.ps1"),
			want:    `$skillSrc = Join-Path (Join-Path $Root "skills") "mono"`,
			avoid:   `$skillSrc = Join-Path $Root "skills\$SkillName"`,
		},
	}

	for _, tc := range checks {
		scriptPath, err := filepath.Abs(tc.relPath)
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", tc.relPath, err)
		}

		data, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
		}

		text := string(data)
		if !strings.Contains(text, tc.want) {
			t.Fatalf("%s missing flattened skills root %q", scriptPath, tc.want)
		}
		if strings.Contains(text, tc.avoid) {
			t.Fatalf("%s still references legacy nested skills root %q", scriptPath, tc.avoid)
		}
	}
}

func TestInstallScriptsExposeSkillModeSelection(t *testing.T) {
	t.Parallel()

	shPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("Abs(install.sh) error = %v", err)
	}
	shData, err := os.ReadFile(shPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", shPath, err)
	}
	shText := string(shData)

	// install.sh must honor DWS_SKILL_MODE, expose mono/multi, and check TTY via [ -t 0 ].
	for _, want := range []string{
		"DWS_SKILL_MODE",
		"mono",
		"multi",
		"[ -t 0 ]",
		"dws skill setup --mode multi",
	} {
		if !strings.Contains(shText, want) {
			t.Fatalf("install.sh missing %q (needed for skill mode selection)", want)
		}
	}

	ps1Path, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatalf("Abs(install.ps1) error = %v", err)
	}
	ps1Data, err := os.ReadFile(ps1Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", ps1Path, err)
	}
	ps1Text := string(ps1Data)

	for _, want := range []string{
		"DWS_SKILL_MODE",
		"mono",
		"multi",
		"IsInputRedirected",
		"dws skill setup --mode multi",
	} {
		if !strings.Contains(ps1Text, want) {
			t.Fatalf("install.ps1 missing %q (needed for skill mode selection)", want)
		}
	}
}

func TestBuildEntrypointsUseStripLdflags(t *testing.T) {
	t.Parallel()

	checks := []struct {
		relPath string
		want    string
	}{
		{
			relPath: filepath.Join("..", "..", "scripts", "install.ps1"),
			want:    `go build -ldflags="-s -w" -o $tmpBin "$Root/cmd"`,
		},
		{
			relPath: filepath.Join("..", "..", "scripts", "dev", "build.sh"),
			want:    `-ldflags="-s -w"`,
		},
	}

	for _, tc := range checks {
		scriptPath, err := filepath.Abs(tc.relPath)
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", tc.relPath, err)
		}

		data, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
		}

		if !strings.Contains(string(data), tc.want) {
			t.Fatalf("%s missing stripped ldflags build invocation %q", scriptPath, tc.want)
		}
	}
}

func TestPolicyBinaryChecksReuseBuiltDWS(t *testing.T) {
	t.Parallel()

	for _, relPath := range []string{
		filepath.Join("..", "..", "scripts", "policy", "check-command-surface.sh"),
		filepath.Join("..", "..", "scripts", "policy", "check-schema-binary.sh"),
	} {
		scriptPath, err := filepath.Abs(relPath)
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", relPath, err)
		}
		data, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
		}
		text := string(data)
		if !strings.Contains(text, `BIN="${DWS_BIN:-$ROOT/dws}"`) {
			t.Fatalf("%s does not reuse DWS_BIN", scriptPath)
		}
		if strings.Contains(text, `go build -o "$tmp/dws" ./cmd`) ||
			strings.Contains(text, `go build -ldflags="-s -w"`) {
			t.Fatalf("%s unexpectedly rebuilds dws", scriptPath)
		}
	}
}

// TestInstallScriptsCacheMultiSkills verifies install.sh / install.ps1 /
// install-skills.sh / build/npm/install.js all carry the wiring that caches
// the multi/ tree to ~/.dws/skills/multi/ during install. This is what lets
// `dws skill setup --mode multi` find a source on a fresh machine.
func TestInstallScriptsCacheMultiSkills(t *testing.T) {
	t.Parallel()

	checks := []struct {
		relPath string
		wants   []string
	}{
		{
			relPath: filepath.Join("..", "..", "scripts", "install.sh"),
			wants: []string{
				"cache_multi_skills",
				"${HOME}/.dws/skills/multi",
				"cache_mono_skills",
			},
		},
		{
			relPath: filepath.Join("..", "..", "scripts", "install.ps1"),
			wants: []string{
				"Cache-MultiSkills",
				".dws\\skills\\multi",
				"Cache-MonoSkills",
			},
		},
		{
			relPath: filepath.Join("..", "..", "scripts", "install-skills.sh"),
			wants: []string{
				"${DWS_CACHE_ROOT}/skills/multi",
				"${DWS_CACHE_ROOT}/skills/mono",
			},
		},
		{
			relPath: filepath.Join("..", "..", "build", "npm", "install.js"),
			wants: []string{
				"cacheUserSkills",
				".dws",
				"\"multi\"",
				"\"mono\"",
			},
		},
	}

	for _, tc := range checks {
		scriptPath, err := filepath.Abs(tc.relPath)
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", tc.relPath, err)
		}
		data, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
		}
		text := string(data)
		for _, want := range tc.wants {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q (needed for multi-skill caching)", scriptPath, want)
			}
		}
	}
}

// TestInstallScriptCachesMultiEndToEnd runs install.sh in source-checkout mode
// with a fake HOME, then verifies that ~/.dws/skills/multi/ ends up populated
// with the per-product skills from skills/multi/.
func TestInstallScriptCachesMultiEndToEnd(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")

	cmd := exec.Command("sh", fixture.scriptPath)
	cmd.Env = fixture.env(
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	// Verify multi cache was populated. We expect dingtalk-* subdirs.
	multiCache := filepath.Join(fixture.fakeHome, ".dws", "skills", "multi")
	entries, err := os.ReadDir(multiCache)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v\noutput:\n%s", multiCache, err, string(output))
	}
	foundDingtalk := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "dingtalk-") {
			foundDingtalk++
		}
	}
	if foundDingtalk == 0 {
		t.Fatalf("no dingtalk-* entries under %s: %v\noutput:\n%s", multiCache, entries, string(output))
	}

	// And verify mono cache.
	monoCacheSkill := filepath.Join(fixture.fakeHome, ".dws", "skills", "mono", "SKILL.md")
	if _, err := os.Stat(monoCacheSkill); err != nil {
		t.Fatalf("missing mono cache SKILL.md at %s: %v", monoCacheSkill, err)
	}
}

// seedAgentHome pre-creates <fakeHome>/.agents/skills/<name>/SKILL.md with the
// given content, simulating a pre-existing skill installation.
func seedAgentHome(t *testing.T, fakeHome, name, content string) string {
	t.Helper()
	p := filepath.Join(fakeHome, ".agents", "skills", name, "SKILL.md")
	mustWriteFile(t, p, []byte(content), 0o644)
	return p
}

func runInstallScript(t *testing.T, scriptPath string, env []string) string {
	t.Helper()
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}
	return string(output)
}

// TestInstallScriptSourceModeDefaultMultiInstall exercises the default (no
// DWS_SKILL_MODE, non-TTY) path: multi must win, the mono leftover (dws/) and
// a stale dingtalk-* skill absent from the bundle must be removed, and
// dws-shared must land alongside the product skill.
func TestInstallScriptSourceModeDefaultMultiInstall(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")

	seedAgentHome(t, fixture.fakeHome, "dws", "old mono\n")
	seedAgentHome(t, fixture.fakeHome, "dingtalk-stale", "stale\n")
	if err := skillstate.Write(fixture.fakeHome, skillstate.State{ManagedSkills: []skillprovenance.Record{{Name: "dingtalk-stale"}}}); err != nil {
		t.Fatal(err)
	}
	seedAgentHome(t, fixture.fakeHome, "dingtalk-custom", "market skill\n")
	seedAgentHome(t, fixture.fakeHome, "other-skill", "not dws\n")

	output := runInstallScript(t, fixture.scriptPath, fixture.envWithSkillMode("",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	))

	if !strings.Contains(output, "Installing agent skills (multi) from local source") {
		t.Fatalf("expected multi install branch, output:\n%s", output)
	}

	base := filepath.Join(fixture.fakeHome, ".agents", "skills")
	for _, name := range []string{"dingtalk-test", "dws-shared"} {
		if _, err := os.Stat(filepath.Join(base, name, "SKILL.md")); err != nil {
			t.Errorf("multi skill %q not installed: %v\noutput:\n%s", name, err, output)
		}
	}
	for _, gone := range []string{"dws", "dingtalk-stale"} {
		if _, err := os.Stat(filepath.Join(base, gone)); !os.IsNotExist(err) {
			t.Errorf("%q should be removed by the multi install, stat err=%v\noutput:\n%s", gone, err, output)
		}
	}
	// Removals must be reversible: the displaced dirs land under
	// ~/.dws/skill-backups/ instead of being destroyed, each inside a stamp
	// root stamped with the ownership marker.
	if got := findSkillBackup(fixture.fakeHome, "dws", "old mono\n"); got == "" {
		t.Errorf("old mono dws/ should be backed up under .dws/skill-backups, output:\n%s", output)
	} else {
		assertSkillBackupMarker(t, filepath.Dir(got))
	}
	if got := findSkillBackup(fixture.fakeHome, "dingtalk-stale", "stale\n"); got == "" {
		t.Errorf("stale dingtalk-* should be backed up under .dws/skill-backups, output:\n%s", output)
	} else {
		assertSkillBackupMarker(t, filepath.Dir(got))
	}
	if _, err := os.Stat(filepath.Join(base, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS skill must be preserved: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(base, "dingtalk-custom", "SKILL.md")); err != nil || string(data) != "market skill\n" {
		t.Errorf("unregistered market/user dingtalk-* skill must be preserved: data=%q err=%v", string(data), err)
	}
	assertSkillProvenance(t, fixture.fakeHome, filepath.Join(base, "dingtalk-test"), "dingtalk-test", "install.sh")
}

// TestInstallScriptSourceModeEmptyMultiFallsBackToMono pins the empty-bundle
// guard: a multi/ tree without any */SKILL.md must fall back to the mono
// branch (with a warning) instead of wiping the user's existing skills and
// installing nothing. The outcome must equal a normal mono install: dws/ is
// replaced with the new mono content and dingtalk-* leftovers are removed by
// mono mutual exclusion — the failure mode being guarded against is the
// empty-state wipe.
func TestInstallScriptSourceModeEmptyMultiFallsBackToMono(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")

	// Corrupt the multi tree: subdirs exist but no SKILL.md anywhere.
	if err := os.Remove(filepath.Join(fixture.root, "skills", "multi", "dingtalk-test", "SKILL.md")); err != nil {
		t.Fatalf("Remove(dingtalk-test/SKILL.md) error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(fixture.root, "skills", "multi", "dws-shared")); err != nil {
		t.Fatalf("RemoveAll(dws-shared) error = %v", err)
	}

	seedAgentHome(t, fixture.fakeHome, "dws", "old mono\n")
	seedAgentHome(t, fixture.fakeHome, "dingtalk-aitable", "pre-state official\n")
	seedAgentHome(t, fixture.fakeHome, "dingtalk-keep", "keep\n")
	seedAgentHome(t, fixture.fakeHome, "other-skill", "not dws\n")

	output := runInstallScript(t, fixture.scriptPath, fixture.envWithSkillMode("multi",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	))

	if !strings.Contains(output, "falling back to mono") {
		t.Fatalf("expected mono fallback warning, output:\n%s", output)
	}

	base := filepath.Join(fixture.fakeHome, ".agents", "skills")
	data, err := os.ReadFile(filepath.Join(base, "dws", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "# Test skill") {
		t.Fatalf("mono dws/ not (re)installed from skills/mono (data=%q, err=%v) — empty multi must not wipe skills\noutput:\n%s", string(data), err, output)
	}
	// An unregistered dingtalk-* directory has unknown ownership and must survive.
	if _, err := os.Stat(filepath.Join(base, "dingtalk-keep", "SKILL.md")); err != nil {
		t.Errorf("unregistered dingtalk-keep should survive mono fallback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-aitable")); !os.IsNotExist(err) {
		t.Errorf("pre-state official dingtalk-aitable should be migrated during mono fallback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-test")); !os.IsNotExist(err) {
		t.Errorf("dingtalk-test must not be installed from the empty multi tree, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS skill must be preserved: %v", err)
	}
}

// TestInstallScriptSourceModeMonoMultiMonoExclusion runs mono → multi → mono
// against the same fake HOME and asserts mutual exclusion in both directions,
// including the dws-shared shared bundle.
func TestInstallScriptSourceModeMonoMultiMonoExclusion(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")
	base := filepath.Join(fixture.fakeHome, ".agents", "skills")

	run := func(mode string) string {
		t.Helper()
		return runInstallScript(t, fixture.scriptPath, fixture.envWithSkillMode(mode,
			"DWS_INSTALL_DIR="+installDir,
			"DWS_NO_SKILLS=0",
		))
	}
	assertPresent := func(rel string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(base, rel, "SKILL.md")); err != nil {
			t.Errorf("%s/SKILL.md should exist: %v", rel, err)
		}
	}
	assertAbsent := func(rel string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(base, rel)); !os.IsNotExist(err) {
			t.Errorf("%s should be absent, stat err=%v", rel, err)
		}
	}

	run("mono")
	assertPresent("dws")
	assertAbsent("dingtalk-test")
	assertAbsent("dws-shared")

	out := run("multi")
	if !strings.Contains(out, "Installing agent skills (multi)") {
		t.Fatalf("expected multi branch, output:\n%s", out)
	}
	assertAbsent("dws")
	assertPresent("dingtalk-test")
	assertPresent("dws-shared")

	run("mono")
	assertPresent("dws")
	assertAbsent("dingtalk-test")
	assertAbsent("dws-shared")

	// Every mutual-exclusion removal along the mono→multi→mono chain must
	// surface as a backup under ~/.dws/skill-backups/ with the old content
	// intact, never as a silent hard delete.
	if got := findSkillBackup(fixture.fakeHome, "dws", "# Test skill\n"); got == "" {
		t.Errorf("mono dws/ replaced by the multi run should be backed up")
	}
	if got := findSkillBackup(fixture.fakeHome, "dingtalk-test", "# Test split skill\n"); got == "" {
		t.Errorf("dingtalk-test replaced by the final mono run should be backed up")
	}
	if got := findSkillBackup(fixture.fakeHome, "dws-shared", "# Test shared skill\n"); got == "" {
		t.Errorf("dws-shared replaced by the final mono run should be backed up")
	}
}

// findSkillBackup searches fakeHome/.dws/skill-backups recursively for a
// directory named name whose SKILL.md equals content, returning its path
// ("" when absent). The layout is <skill-backups>/<stamp>/<name> with
// optional -N collision suffixes on the stamp.
func findSkillBackupContent(fakeHome, content string) string {
	root := filepath.Join(fakeHome, ".dws", "skill-backups")
	var found string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || found != "" || info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil && string(data) == content {
			found = p
		}
		return nil
	})
	return found
}

func findSkillBackup(fakeHome, name, content string) string {
	root := filepath.Join(fakeHome, ".dws", "skill-backups")
	var found string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || found != "" || !info.IsDir() || info.Name() != name {
			return nil
		}
		if data, err := os.ReadFile(filepath.Join(p, "SKILL.md")); err == nil && string(data) == content {
			found = p
		}
		return nil
	})
	return found
}

// assertSkillBackupMarker pins the ownership contract shared with the Go core:
// every stamp root a shell installer creates carries .dws-skill-backup with
// exactly "dws skill backup v1\n", because pruning only ever removes
// stamp-shaped roots with those exact bytes.
func assertSkillBackupMarker(t *testing.T, stampRoot string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stampRoot, skillBackupMarkerFile))
	if err != nil {
		t.Fatalf("backup ownership marker missing in %s: %v", stampRoot, err)
	}
	if string(data) != skillBackupMarkerBody {
		t.Fatalf("backup ownership marker in %s = %q, want %q", stampRoot, data, skillBackupMarkerBody)
	}
}

// TestInstallScriptPrunePreservesUnmarkedStampDirs runs the real install.sh
// (whose main ends with prune_skill_backups) against a HOME whose backup root
// already holds a full marked history plus one unmarked stamp-shaped
// directory. The end-of-run prune must remove the marked excess while the
// unmarked directory — foreign data without the ownership marker — is neither
// deleted nor counted toward the keep limit, and the stamp this run creates
// must carry the exact marker bytes.
func TestInstallScriptPrunePreservesUnmarkedStampDirs(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")
	backupRoot := filepath.Join(fixture.fakeHome, ".dws", "skill-backups")

	// Oldest on purpose: without the marker check this directory would be the
	// first pruning candidate and would inflate the excess count.
	unmarked := filepath.Join(backupRoot, "20200101-000000")
	mustWriteFile(t, filepath.Join(unmarked, "user-data.txt"), []byte("foreign\n"), 0o644)
	for i := 1; i <= 6; i++ {
		mustWriteFile(t, filepath.Join(backupRoot, oldStampName(i), "skill", "SKILL.md"), []byte("old batch\n"), 0o644)
		mustWriteFile(t, filepath.Join(backupRoot, oldStampName(i), skillBackupMarkerFile), []byte(skillBackupMarkerBody), 0o644)
	}
	seedAgentHome(t, fixture.fakeHome, "dws", "old mono\n")

	runInstallScript(t, fixture.scriptPath, fixture.envWithSkillMode("mono",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	))

	// Six marked batches plus this run's one exceed the keep limit of five,
	// so exactly the two oldest marked batches are excess and must be gone —
	// proving the prune actually ran.
	for _, gone := range []string{oldStampName(1), oldStampName(2)} {
		if _, err := os.Stat(filepath.Join(backupRoot, gone)); !os.IsNotExist(err) {
			t.Fatalf("marked excess stamp %s should have been pruned, stat err=%v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(backupRoot, oldStampName(3))); err != nil {
		t.Fatalf("marked keep-range stamp should survive the prune: %v", err)
	}
	// The unmarked directory is never counted and never removed, even though
	// it sorts before every pruned batch.
	if data, err := os.ReadFile(filepath.Join(unmarked, "user-data.txt")); err != nil || string(data) != "foreign\n" {
		t.Fatalf("unmarked stamp-shaped directory must be preserved untouched: data=%q err=%v", data, err)
	}
	got := findSkillBackup(fixture.fakeHome, "dws", "old mono\n")
	if got == "" {
		t.Fatal("this run's dws backup is missing after the prune")
	}
	assertSkillBackupMarker(t, filepath.Dir(got))
}

// TestInstallScriptBackupFailureKeepsOriginalDir pins the fail-safe contract:
// when the backup destination cannot be created (skill-backups pre-created as
// a regular file), the installer must keep every pre-existing skill directory
// untouched and skip the affected Agent target — a backup failure must never
// degrade into a hard delete or leave a mixed mono + multi layout.
func TestInstallScriptBackupFailureKeepsOriginalDir(t *testing.T) {
	t.Parallel()

	fixture := newInstallSourceFixture(t)
	installDir := filepath.Join(fixture.root, "bin")
	base := filepath.Join(fixture.fakeHome, ".agents", "skills")

	// Poison the backup root: mkdir -p <file>/<stamp> cannot succeed.
	mustWriteFile(t, filepath.Join(fixture.fakeHome, ".dws", "skill-backups"), []byte("not a directory\n"), 0o644)
	seedAgentHome(t, fixture.fakeHome, "dws", "old mono\n")

	monoCmd := exec.Command("sh", fixture.scriptPath)
	monoCmd.Env = fixture.envWithSkillMode("mono",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	)
	monoOutput, monoErr := monoCmd.CombinedOutput()
	if monoErr == nil {
		t.Fatalf("mono install unexpectedly succeeded after backup failure:\n%s", monoOutput)
	}
	out := string(monoOutput)
	if !strings.Contains(out, "保留原目录") {
		t.Fatalf("expected backup-failure warning in mono output:\n%s", out)
	}
	data, err := os.ReadFile(filepath.Join(base, "dws", "SKILL.md"))
	if err != nil || string(data) != "old mono\n" {
		t.Fatalf("mono run must keep the original dws/ untouched on backup failure (data=%q, err=%v)\noutput:\n%s", string(data), err, out)
	}

	cmd := exec.Command("sh", fixture.scriptPath)
	cmd.Env = fixture.envWithSkillMode("multi",
		"DWS_INSTALL_DIR="+installDir,
		"DWS_NO_SKILLS=0",
	)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("multi install unexpectedly succeeded after backup failure:\n%s", output)
	}
	out = string(output)
	if !strings.Contains(out, "保留原目录") {
		t.Fatalf("expected backup-failure warning in multi output:\n%s", out)
	}
	data, err = os.ReadFile(filepath.Join(base, "dws", "SKILL.md"))
	if err != nil || string(data) != "old mono\n" {
		t.Fatalf("multi run must keep the mono leftover dws/ untouched on backup failure (data=%q, err=%v)\noutput:\n%s", string(data), err, out)
	}
	for _, name := range []string{"dingtalk-test", "dws-shared"} {
		if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Fatalf("multi run must not install %s after cleanup backup failure, stat err=%v\noutput:\n%s", name, err, out)
		}
	}
}

func TestInstallSkillsShellBackupFailureWritesNoMultiSkills(t *testing.T) {
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-skills.sh"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "\nmain\n")
	if cut < 0 {
		t.Fatal("install-skills.sh final main invocation not found")
	}
	library := filepath.Join(t.TempDir(), "install-skills-lib.sh")
	mustWriteFile(t, library, data[:cut], 0o755)

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "project")
	base := filepath.Join(root, ".agents", "skills")
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(base, "dws", "SKILL.md"), []byte("old mono\n"), 0o644)
	mustWriteFile(t, filepath.Join(multi, "dingtalk-test", "SKILL.md"), []byte("new multi\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".dws", "skill-backups"), []byte("not a directory\n"), 0o644)

	harness := `. "$DWS_TEST_LIBRARY"
install_multi_skills_to_root "$DWS_TEST_MULTI" "$DWS_TEST_ROOT"
`
	cmd := exec.Command("sh", "-c", harness)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"DWS_TEST_LIBRARY="+library,
		"DWS_TEST_MULTI="+multi,
		"DWS_TEST_ROOT="+root,
	)
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("install-skills harness unexpectedly succeeded after backup failure:\n%s", output)
	}
	if !strings.Contains(string(output), "所有检测到的 Agent 目标均失败") {
		t.Fatalf("install-skills aggregate failure missing:\n%s", output)
	}
	if data, err := os.ReadFile(filepath.Join(base, "dws", "SKILL.md")); err != nil || string(data) != "old mono\n" {
		t.Fatalf("mono changed after backup failure (data=%q, err=%v)", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-test")); !os.IsNotExist(err) {
		t.Fatalf("multi installed after backup failure, stat err=%v", err)
	}
}

func TestInstallSkillsShellMvCrossFilesystemBackupAndRestore(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real cross-filesystem mv fixture uses Linux /dev/shm")
	}
	sharedMemoryRoot, err := os.MkdirTemp("/dev/shm", "dws-shell-exdev-")
	if err != nil {
		t.Skipf("/dev/shm is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sharedMemoryRoot) })

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-skills.sh"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "\nmain\n")
	if cut < 0 {
		t.Fatal("install-skills.sh final main invocation not found")
	}
	library := filepath.Join(t.TempDir(), "install-skills-lib.sh")
	mustWriteFile(t, library, data[:cut], 0o755)

	source := filepath.Join(t.TempDir(), "dingtalk-chat")
	mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("old skill\n"), 0o640)
	if err := os.Symlink("SKILL.md", filepath.Join(source, "skill-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.md", filepath.Join(source, "dangling-link")); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(sharedMemoryRoot, "rename-probe")
	if err := os.Rename(source, probe); err == nil {
		if restoreErr := os.Rename(probe, source); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		t.Skip("/dev/shm and the test temp directory share a filesystem")
	} else if !strings.Contains(strings.ToLower(err.Error()), "cross-device") {
		t.Skipf("could not establish a cross-filesystem fixture: %v", err)
	}

	home := filepath.Join(sharedMemoryRoot, "home")
	manifest := filepath.Join(sharedMemoryRoot, "backups.manifest")
	harness := `. "$DWS_TEST_LIBRARY"
backup_and_remove_skill_dir "$DWS_TEST_SOURCE"
backup=$DWS_LAST_SKILL_BACKUP
[ -n "$backup" ] && [ -d "$backup" ] && [ ! -e "$DWS_TEST_SOURCE" ]
printf '%s\n%s\n' "$DWS_TEST_SOURCE" "$backup" > "$DWS_TEST_MANIFEST"
restore_multi_skill_set /dev/null "$DWS_TEST_MANIFEST"
`
	cmd := exec.Command("sh", "-c", harness)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"DWS_TEST_LIBRARY="+library,
		"DWS_TEST_SOURCE="+source,
		"DWS_TEST_MANIFEST="+manifest,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-filesystem shell backup/restore failed: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(source, "SKILL.md")); err != nil || string(got) != "old skill\n" {
		t.Fatalf("restored shell content = %q, %v", got, err)
	}
	for name, want := range map[string]string{"skill-link": "SKILL.md", "dangling-link": "missing.md"} {
		info, err := os.Lstat(filepath.Join(source, name))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("shell %s must remain a symlink: %v, %v", name, info, err)
		}
		if got, err := os.Readlink(filepath.Join(source, name)); err != nil || got != want {
			t.Fatalf("shell %s target = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestInstallSkillsShellPreservesUnregisteredDingtalkSkill(t *testing.T) {
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-skills.sh"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "\nmain\n")
	if cut < 0 {
		t.Fatal("install-skills.sh final main invocation not found")
	}
	library := filepath.Join(t.TempDir(), "install-skills-lib.sh")
	mustWriteFile(t, library, data[:cut], 0o755)

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "project")
	base := filepath.Join(root, ".agents", "skills")
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(base, "dingtalk-custom", "SKILL.md"), []byte("market skill\n"), 0o644)
	mustWriteFile(t, filepath.Join(base, "dingtalk-aitable", "SKILL.md"), []byte("pre-state official\n"), 0o644)
	mustWriteFile(t, filepath.Join(base, "dingtalk-retired", "SKILL.md"), []byte("retired\n"), 0o644)
	if err := skillstate.Write(home, skillstate.State{ManagedSkills: []skillprovenance.Record{{Name: "dingtalk-retired"}}}); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(multi, "dingtalk-test", "SKILL.md"), []byte("new multi\n"), 0o644)

	harness := `. "$DWS_TEST_LIBRARY"
install_multi_skills_to_root "$DWS_TEST_MULTI" "$DWS_TEST_ROOT"
`
	cmd := exec.Command("sh", "-c", harness)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"DWS_TEST_LIBRARY="+library,
		"DWS_TEST_MULTI="+multi,
		"DWS_TEST_ROOT="+root,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install-skills harness failed: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(base, "dingtalk-custom", "SKILL.md")); err != nil || string(got) != "market skill\n" {
		t.Fatalf("unregistered market/user dingtalk-* Skill changed: data=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-retired")); !os.IsNotExist(err) {
		t.Fatalf("centrally managed retired DWS Skill must be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-aitable")); !os.IsNotExist(err) {
		t.Fatalf("pre-state official DWS Skill must be removed: %v", err)
	}
	assertSkillProvenance(t, home, filepath.Join(base, "dingtalk-test"), "dingtalk-test", "install-skills.sh")
}

func TestInstallerShellUsesCanonicalRootWithoutCodexDuplicate(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		t.Run(scriptName, func(t *testing.T) {
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			source := filepath.Join(t.TempDir(), "multi")
			mustWriteFile(t, filepath.Join(home, ".codex", "config.toml"), []byte("model=test\n"), 0o644)
			mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"), []byte("old nested\n"), 0o644)
			mustWriteFile(t, filepath.Join(source, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)

			installCall := `install_multi_skills_to_homes "$DWS_TEST_SOURCE"`
			if scriptName == "install-skills.sh" {
				installCall = `install_multi_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
			}
			cmd := exec.Command("sh", "-c", `. "$DWS_TEST_LIBRARY"
`+installCall+`
`)
			cmd.Env = append(os.Environ(), "HOME="+home, "DWS_TEST_LIBRARY="+library, "DWS_TEST_SOURCE="+source)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s Codex-root harness failed: %v\n%s", scriptName, err, output)
			}
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
				t.Fatalf("%s canonical Skill missing: %v", scriptName, err)
			}
			for _, duplicate := range []string{
				filepath.Join(home, ".codex", "skills", "dingtalk-chat"),
				filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"),
			} {
				if _, err := os.Stat(duplicate); !os.IsNotExist(err) {
					t.Fatalf("%s generic duplicate remains at %s: %v", scriptName, duplicate, err)
				}
			}
		})
	}
}

func TestInstallerShellLinksZCodeRootToCanonical(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		t.Run(scriptName, func(t *testing.T) {
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			source := filepath.Join(t.TempDir(), "multi")
			mustWriteFile(t, filepath.Join(home, ".zcode", "v2", "config.json"), []byte("{}\n"), 0o644)
			mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"), []byte("old nested\n"), 0o644)
			mustWriteFile(t, filepath.Join(source, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)

			installCall := `install_multi_skills_to_homes "$DWS_TEST_SOURCE"`
			if scriptName == "install-skills.sh" {
				installCall = `install_multi_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
			}
			cmd := exec.Command("sh", "-c", `. "$DWS_TEST_LIBRARY"
`+installCall+`
`)
			cmd.Env = append(os.Environ(), "HOME="+home, "DWS_TEST_LIBRARY="+library, "DWS_TEST_SOURCE="+source)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s ZCode-root harness failed: %v\n%s", scriptName, err, output)
			}
			if _, err := os.Stat(filepath.Join(home, ".zcode", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
				t.Fatalf("%s linked ZCode Skill missing: %v", scriptName, err)
			}
			if info, err := os.Lstat(filepath.Join(home, ".zcode", "skills", "dingtalk-chat")); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s ZCode target is not a canonical link: %#v, %v", scriptName, info, err)
			}
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
				t.Fatalf("%s canonical Skill missing: %v", scriptName, err)
			}
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws")); !os.IsNotExist(err) {
				t.Fatalf("%s generic duplicate remains: %v", scriptName, err)
			}
		})
	}
}

func TestInstallerShellLinkPublicationRacePreservesConcurrentDirectories(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			base := filepath.Join(home, ".zcode", "skills")
			first := filepath.Join(base, "dingtalk-first")
			second := filepath.Join(base, "dingtalk-second")
			mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dingtalk-first", "SKILL.md"), []byte("first\n"), 0o644)
			mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dingtalk-second", "SKILL.md"), []byte("second\n"), 0o644)
			bundle := filepath.Join(t.TempDir(), "multi")
			mustWriteFile(t, filepath.Join(bundle, "dingtalk-first", "SKILL.md"), []byte("first\n"), 0o644)
			mustWriteFile(t, filepath.Join(bundle, "dingtalk-second", "SKILL.md"), []byte("second\n"), 0o644)

			// Publication creates the link directly at the destination with
			// `ln -s` (symlink(2) refuses an occupied path atomically; BusyBox
			// has no `ln -P`), and multi mode requires the bundle source so
			// that only bundle skills are linked. Inject the race on that
			// creation.
			harness := `. "$DWS_TEST_LIBRARY"
ln() {
  if [ "$#" -eq 3 ] && [ "$1" = "-s" ] && [ "$3" = "$DWS_TEST_SECOND" ]; then
    rm -f "$DWS_TEST_FIRST"
    mkdir -p "$DWS_TEST_FIRST" "$DWS_TEST_SECOND"
    printf '%s\n' first-user-data > "$DWS_TEST_FIRST/user.txt"
    printf '%s\n' second-user-data > "$DWS_TEST_SECOND/user.txt"
  fi
  command ln "$@"
}
if link_canonical_skills_to_base "$HOME" "$DWS_TEST_BASE" multi "$DWS_TEST_BUNDLE"; then
  exit 2
fi
identity_dest="$DWS_TEST_BASE/identity-link"
identity_anchor="$HOME/identity-anchor"
identity_manifest="$HOME/identity.manifest"
command ln -s ../same-target "$identity_anchor"
command ln -P "$identity_anchor" "$identity_dest"
identity_inode="$(skill_link_inode "$identity_anchor")"
printf '%s\n%s\n%s\n' "$identity_dest" ../same-target "$identity_inode" > "$identity_manifest"
rm -f "$identity_dest"
command ln -s ../same-target "$identity_dest"
if restore_linked_skill_set "$identity_manifest" /dev/null; then
  exit 3
fi
rm -f "$identity_anchor"
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_BASE="+base,
				"DWS_TEST_BUNDLE="+bundle,
				"DWS_TEST_FIRST="+first,
				"DWS_TEST_SECOND="+second,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s race harness failed: %v\n%s", scriptName, err, output)
			}
			if !strings.Contains(string(output), "跳过回滚已被并发修改的 Skill 路径: "+first) {
				t.Fatalf("%s did not report identity-protected rollback:\n%s", scriptName, output)
			}
			identityDest := filepath.Join(base, "identity-link")
			if target, err := os.Readlink(identityDest); err != nil || target != "../same-target" {
				t.Fatalf("%s replacement link was removed: target=%q, err=%v", scriptName, target, err)
			}
			for path, want := range map[string]string{
				filepath.Join(first, "user.txt"):  "first-user-data\n",
				filepath.Join(second, "user.txt"): "second-user-data\n",
			} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("%s concurrent data at %s = %q, err=%v", scriptName, path, got, err)
				}
			}
			for _, dir := range []string{first, second} {
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatalf("%s read concurrent directory %s: %v", scriptName, dir, err)
				}
				if len(entries) != 1 || entries[0].Name() != "user.txt" {
					t.Fatalf("%s transaction artifacts remain in %s: %v", scriptName, dir, entries)
				}
			}
			if matches, err := filepath.Glob(filepath.Join(base, ".dws-link-set.*")); err != nil || len(matches) != 0 {
				t.Fatalf("%s staging leftovers = %v, err=%v", scriptName, matches, err)
			}
		})
	}
}

func TestInstallerShellCopyPublicationRacePreservesConcurrentDirectories(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			base := filepath.Join(home, ".zcode", "skills")
			first := filepath.Join(base, "dingtalk-first")
			second := filepath.Join(base, "dingtalk-second")
			mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
			mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)
			src := filepath.Join(t.TempDir(), "multi")
			mustWriteFile(t, filepath.Join(src, "dingtalk-first", "SKILL.md"), []byte("new first\n"), 0o644)
			mustWriteFile(t, filepath.Join(src, "dingtalk-second", "SKILL.md"), []byte("new second\n"), 0o644)

			// Inject the race inside the second publication's child move: the
			// first skill has already been published and recorded, so this
			// simulates a concurrent writer replacing it and the transaction
			// failing afterwards — rollback must verify identity and retain
			// the foreign object instead of blind-deleting the path.
			harness := `. "$DWS_TEST_LIBRARY"
mv() {
  case "$1" in
    */.dws-multi-set.*)
      if [ "$2" = "$DWS_TEST_SECOND/" ]; then
        rm -rf "$DWS_TEST_FIRST"
        mkdir -p "$DWS_TEST_FIRST"
        printf '%s\n' first-user-data > "$DWS_TEST_FIRST/concurrent-user-data.txt"
        return 1
      fi
      ;;
  esac
  command mv "$@"
}
if _install_multi_to_base "$DWS_TEST_SRC" "$DWS_TEST_BASE" "$HOME" ".zcode/skills"; then
  exit 2
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_BASE="+base,
				"DWS_TEST_SRC="+src,
				"DWS_TEST_FIRST="+first,
				"DWS_TEST_SECOND="+second,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s copy race harness failed: %v\n%s", scriptName, err, output)
			}
			if !strings.Contains(string(output), "跳过回滚已被并发修改的 Skill 路径: "+first) {
				t.Fatalf("%s did not skip identity-mismatched rollback:\n%s", scriptName, output)
			}
			got, err := os.ReadFile(filepath.Join(first, "concurrent-user-data.txt"))
			if err != nil || string(got) != "first-user-data\n" {
				t.Fatalf("%s concurrent directory was deleted or modified: %q, %v", scriptName, string(got), err)
			}
			entries, err := os.ReadDir(first)
			if err != nil || len(entries) != 1 || entries[0].Name() != "concurrent-user-data.txt" {
				t.Fatalf("%s foreign directory contents changed: %v, %v", scriptName, entries, err)
			}
			if restored, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(restored) != "old second\n" {
				t.Fatalf("%s second skill was not restored from backup: %q, %v", scriptName, string(restored), err)
			}
			if matches, err := filepath.Glob(filepath.Join(base, ".dws-multi-set.*")); err != nil || len(matches) != 0 {
				t.Fatalf("%s staging leftovers = %v, err=%v", scriptName, matches, err)
			}
		})
	}
}

// TestInstallerShellRollbackPreservesInPlaceEditedSkill pins the P1 contract:
// a published Skill directory whose file contents were edited in place (same
// directory inode, same child names) must not be deleted by the rollback of a
// later failed publication. The recorded identity carries a recursive content
// digest, so the edit makes the dest provably not-this-transaction's object.
func TestInstallerShellRollbackPreservesInPlaceEditedSkill(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			base := filepath.Join(home, ".zcode", "skills")
			first := filepath.Join(base, "dingtalk-first")
			second := filepath.Join(base, "dingtalk-second")
			mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
			mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)
			src := filepath.Join(t.TempDir(), "multi")
			mustWriteFile(t, filepath.Join(src, "dingtalk-first", "SKILL.md"), []byte("new first\n"), 0o644)
			mustWriteFile(t, filepath.Join(src, "dingtalk-first", "references", "guide.md"), []byte("guide\n"), 0o644)
			mustWriteFile(t, filepath.Join(src, "dingtalk-second", "SKILL.md"), []byte("new second\n"), 0o644)

			// Inside the second publication's child move — after the first
			// skill has been published and its identity recorded — edit a file
			// inside the first destination in place (append keeps the inode,
			// directory inode and child names are untouched), then fail the
			// transaction. Rollback must detect the content drift through the
			// recorded digest and leave the user's edit alone.
			harness := `. "$DWS_TEST_LIBRARY"
mv() {
  case "$1" in
    */.dws-multi-set.*)
      if [ "$2" = "$DWS_TEST_SECOND/" ]; then
        printf 'user-edit\n' >> "$DWS_TEST_FIRST/references/guide.md"
        return 1
      fi
      ;;
  esac
  command mv "$@"
}
if _install_multi_to_base "$DWS_TEST_SRC" "$DWS_TEST_BASE" "$HOME" ".zcode/skills"; then
  exit 2
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_BASE="+base,
				"DWS_TEST_SRC="+src,
				"DWS_TEST_FIRST="+first,
				"DWS_TEST_SECOND="+second,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s in-place edit harness failed: %v\n%s", scriptName, err, output)
			}
			if !strings.Contains(string(output), "跳过回滚已被并发修改的 Skill 路径: "+first) {
				t.Fatalf("%s did not skip in-place-edited rollback:\n%s", scriptName, output)
			}
			got, err := os.ReadFile(filepath.Join(first, "references", "guide.md"))
			if err != nil || string(got) != "guide\nuser-edit\n" {
				t.Fatalf("%s in-place user edit was lost: %q, %v", scriptName, string(got), err)
			}
			if restored, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(restored) != "old second\n" {
				t.Fatalf("%s second skill was not restored from backup: %q, %v", scriptName, string(restored), err)
			}
			if matches, err := filepath.Glob(filepath.Join(base, ".dws-multi-set.*")); err != nil || len(matches) != 0 {
				t.Fatalf("%s staging leftovers = %v, err=%v", scriptName, matches, err)
			}
		})
	}
}

// TestInstallerShellBackupMarkerWriteFailurePreservesSameStampSibling pins
// the review blocker on the shared stamp root: the backup collision loop
// tests the payload path, so two backups in the same stamp second land in
// ONE stamp root. When the second call fails to write the ownership marker,
// the old `rm -rf` cleanup destroyed the first, already completed sibling
// payload whose original directory had already been moved away. The cleanup
// must be non-recursive and may only drop a stamp root the failing call
// itself created (marker file plus empty root; rmdir refuses non-empty).
func TestInstallerShellBackupMarkerWriteFailurePreservesSameStampSibling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}
	// Both scenarios rely on permission-bit enforcement (unwritable marker
	// file, unwritable fresh root), which root bypasses entirely.
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores permission bits, the marker write would succeed")
	}
	for _, scriptName := range []string{"install.sh", "install-skills.sh", "install-event.sh", "install-devapp.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			backupFn := "backup_and_remove_skill_dir"
			emitsMarkerWarning := true
			if scriptName == "install-event.sh" || scriptName == "install-devapp.sh" {
				backupFn = "backup_skill_dir"
				emitsMarkerWarning = false
			}
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			// Pin the stamp: a stub `date` always prints the same UTC second,
			// so consecutive backups deterministically share one stamp root.
			const stamp = "20260818-120000"
			stubDir := filepath.Join(t.TempDir(), "bin")
			mustWriteFile(t, filepath.Join(stubDir, "date"), []byte("#!/bin/sh\nprintf '"+stamp+"\\n'\n"), 0o755)

			t.Run("same-stamp sibling payload survives marker write failure", func(t *testing.T) {
				home := t.TempDir()
				root := filepath.Join(home, ".dws", "skill-backups", stamp)
				victimA := filepath.Join(home, ".agents", "skills", "skill-a")
				victimB := filepath.Join(home, ".agents", "skills", "skill-b")
				mustWriteFile(t, filepath.Join(victimA, "SKILL.md"), []byte("payload A\n"), 0o644)
				mustWriteFile(t, filepath.Join(victimB, "SKILL.md"), []byte("victim B\n"), 0o644)

				// The first backup succeeds and moves victim A into the stamp
				// root; the marker is then made unwritable so the second,
				// same-second backup fails exactly at the marker write.
				harness := `. "$DWS_TEST_LIBRARY"
if ` + backupFn + ` "$DWS_TEST_VICTIM_A"; then :; else exit 10; fi
chmod 000 "$DWS_TEST_ROOT/` + skillBackupMarkerFile + `"
if ` + backupFn + ` "$DWS_TEST_VICTIM_B"; then
  exit 11
fi
[ -d "$DWS_TEST_ROOT" ] || exit 12
[ -f "$DWS_TEST_VICTIM_B/SKILL.md" ] || exit 13
`
				cmd := exec.Command("sh", "-c", harness)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"PATH="+stubDir+":"+os.Getenv("PATH"),
					"DWS_TEST_LIBRARY="+library,
					"DWS_TEST_ROOT="+root,
					"DWS_TEST_VICTIM_A="+victimA,
					"DWS_TEST_VICTIM_B="+victimB,
				)
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s sibling marker-failure harness failed: %v\n%s", scriptName, err, output)
				}
				if emitsMarkerWarning && !strings.Contains(string(output), "无法写入备份所有权标记") {
					t.Fatalf("%s did not report the marker write failure:\n%s", scriptName, output)
				}
				if _, err := os.Lstat(victimA); !os.IsNotExist(err) {
					t.Fatalf("%s first victim must have been moved into the stamp root: %v", scriptName, err)
				}
				entries, err := os.ReadDir(root)
				if err != nil {
					t.Fatalf("%s stamp root must survive the failed marker write: %v", scriptName, err)
				}
				var payloadDirs []string
				for _, entry := range entries {
					if entry.Name() == skillBackupMarkerFile {
						continue
					}
					if !entry.IsDir() {
						t.Fatalf("%s stamp root carries an unexpected non-marker entry %s", scriptName, entry.Name())
					}
					payloadDirs = append(payloadDirs, entry.Name())
				}
				if len(payloadDirs) != 1 {
					t.Fatalf("%s stamp root must retain exactly the first payload, got %v", scriptName, payloadDirs)
				}
				got, err := os.ReadFile(filepath.Join(root, payloadDirs[0], "SKILL.md"))
				if err != nil || string(got) != "payload A\n" {
					t.Fatalf("%s completed sibling payload was destroyed: %q, %v", scriptName, got, err)
				}
				if got, err := os.ReadFile(filepath.Join(victimB, "SKILL.md")); err != nil || string(got) != "victim B\n" {
					t.Fatalf("%s second victim must stay at its original path: %q, %v", scriptName, got, err)
				}
			})

			t.Run("fresh stamp root is dropped non-recursively", func(t *testing.T) {
				home := t.TempDir()
				root := filepath.Join(home, ".dws", "skill-backups", stamp)
				victim := filepath.Join(home, ".agents", "skills", "skill-fresh")
				mustWriteFile(t, filepath.Join(victim, "SKILL.md"), []byte("fresh victim\n"), 0o644)

				// The failing call itself created the root, so the cleanup
				// must drop it again: mkdir is intercepted to hand back a
				// non-writable root, failing the marker write while the root
				// stays removable through its writable parent directory.
				harness := `. "$DWS_TEST_LIBRARY"
mkdir() {
  command mkdir "$@"
  _mk_rc=$?
  if [ "${1:-}" = "-p" ] && [ "${2:-}" = "$DWS_TEST_ROOT" ]; then
    chmod 0555 "$DWS_TEST_ROOT"
  fi
  return "$_mk_rc"
}
if ` + backupFn + ` "$DWS_TEST_VICTIM"; then
  exit 11
fi
[ ! -e "$DWS_TEST_ROOT" ] || exit 12
[ -f "$DWS_TEST_VICTIM/SKILL.md" ] || exit 13
`
				cmd := exec.Command("sh", "-c", harness)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"PATH="+stubDir+":"+os.Getenv("PATH"),
					"DWS_TEST_LIBRARY="+library,
					"DWS_TEST_ROOT="+root,
					"DWS_TEST_VICTIM="+victim,
				)
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s fresh marker-failure harness failed: %v\n%s", scriptName, err, output)
				}
				if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Fatalf("%s fresh stamp root must be removed again: %v", scriptName, err)
				}
				if got, err := os.ReadFile(filepath.Join(victim, "SKILL.md")); err != nil || string(got) != "fresh victim\n" {
					t.Fatalf("%s victim must stay untouched at its original path: %q, %v", scriptName, got, err)
				}
			})
		})
	}
}

func TestInstallerShellUsesUpstreamXDGAndCustomAgentRoots(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		t.Run(scriptName, func(t *testing.T) {
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			tmp := t.TempDir()
			xdg := filepath.Join(tmp, "xdg config")
			autohand := filepath.Join(tmp, "autohand home")
			codex := filepath.Join(tmp, "codex home")
			source := filepath.Join(tmp, "multi")
			mustWriteFile(t, filepath.Join(source, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)
			mustWriteFile(t, filepath.Join(xdg, "goose", "config.yaml"), []byte("enabled: true\n"), 0o644)
			mustWriteFile(t, filepath.Join(xdg, "agents", "skills", "dingtalk-chat", "SKILL.md"), []byte("old amp copy\n"), 0o644)
			mustWriteFile(t, filepath.Join(autohand, "config.json"), []byte("{}\n"), 0o644)
			mustWriteFile(t, filepath.Join(codex, "config.toml"), []byte("model=test\n"), 0o644)
			mustWriteFile(t, filepath.Join(codex, "skills", "dingtalk-chat", "SKILL.md"), []byte("old codex copy\n"), 0o644)
			mustWriteFile(t, filepath.Join(home, ".qoderwork", "config.json"), []byte("{}\n"), 0o644)
			mustWriteFile(t, filepath.Join(home, ".amp", "skills", "dingtalk-chat", "SKILL.md"), []byte("old DWS path\n"), 0o644)

			installCall := `install_multi_skills_to_homes "$DWS_TEST_SOURCE"`
			if scriptName == "install-skills.sh" {
				installCall = `install_multi_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
			}
			cmd := exec.Command("sh", "-c", `. "$DWS_TEST_LIBRARY"
`+installCall+`
`)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_SOURCE="+source,
				"XDG_CONFIG_HOME="+xdg,
				"AUTOHAND_HOME="+autohand,
				"CODEX_HOME="+codex,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s upstream-root harness failed: %v\n%s", scriptName, err, output)
			}

			for _, linked := range []string{
				filepath.Join(xdg, "goose", "skills", "dingtalk-chat"),
				filepath.Join(autohand, "skills", "dingtalk-chat"),
				filepath.Join(home, ".qoderwork", "skills", "dingtalk-chat"),
			} {
				info, err := os.Lstat(linked)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("%s is not a canonical symlink: %#v, %v", linked, info, err)
				}
				target, err := os.Readlink(linked)
				if err != nil || filepath.IsAbs(target) {
					t.Fatalf("%s link must be relative: target=%q err=%v", linked, target, err)
				}
			}
			for _, retired := range []string{
				filepath.Join(xdg, "agents", "skills", "dingtalk-chat"),
				filepath.Join(codex, "skills", "dingtalk-chat"),
				filepath.Join(home, ".amp", "skills", "dingtalk-chat"),
			} {
				if _, err := os.Lstat(retired); !os.IsNotExist(err) {
					t.Fatalf("%s universal/legacy duplicate remains: %v", retired, err)
				}
			}
		})
	}
}

func TestInstallerShellPinsCompleteUpstreamAgentRegistry(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		t.Run(scriptName, func(t *testing.T) {
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)
			cmd := exec.Command("sh", "-c", `. "$DWS_TEST_LIBRARY"
upstream_agent_registry
`)
			cmd.Env = append(os.Environ(), "DWS_TEST_LIBRARY="+library)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s registry failed: %v\n%s", scriptName, err, output)
			}
			lines := strings.Fields(strings.TrimSpace(string(output)))
			if len(lines) != 76 {
				t.Fatalf("%s registry has %d Agents, want 76", scriptName, len(lines))
			}
			ids := map[string]bool{}
			roots := map[string]bool{}
			universal, nonUniversal, noGlobal, canonicalDirect := 0, 0, 0, 0
			for _, line := range lines {
				parts := strings.Split(line, "|")
				if len(parts) != 3 || ids[parts[0]] {
					t.Fatalf("%s invalid or duplicate registry record %q", scriptName, line)
				}
				ids[parts[0]] = true
				switch parts[1] {
				case "U":
					universal++
				case "N":
					nonUniversal++
				default:
					t.Fatalf("%s invalid classification in %q", scriptName, line)
				}
				switch parts[2] {
				case "-":
					noGlobal++
				case ".agents/skills":
					canonicalDirect++
				default:
					roots[parts[2]] = true
				}
			}
			if universal != 19 || nonUniversal != 57 || noGlobal != 2 || canonicalDirect != 6 || len(roots) != 65 {
				t.Fatalf("%s registry classification U=%d N=%d no-global=%d canonical=%d roots=%d", scriptName, universal, nonUniversal, noGlobal, canonicalDirect, len(roots))
			}
		})
	}
}

func TestInstallPowerShellUsesCanonicalRootWithoutCodexDuplicate(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
if (!(Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME)) { exit 2 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-codex-root.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

	home := t.TempDir()
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(home, ".codex", "config.toml"), []byte("model=test\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"), []byte("old nested\n"), 0o644)
	mustWriteFile(t, filepath.Join(multi, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home, "DWS_TEST_MULTI="+multi)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell Codex-root harness failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("PowerShell canonical Skill missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", "dingtalk-chat")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell Codex duplicate remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell generic duplicate remains: %v", err)
	}
}

func TestInstallPowerShellLinksZCodeRootToCanonical(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
if (!(Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME)) { exit 2 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-zcode-root.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

	home := t.TempDir()
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(home, ".zcode", "v2", "config.json"), []byte("{}\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"), []byte("old nested\n"), 0o644)
	mustWriteFile(t, filepath.Join(multi, "dingtalk-chat", "SKILL.md"), []byte("new chat\n"), 0o644)

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home, "DWS_TEST_MULTI="+multi)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell ZCode-root harness failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(home, ".zcode", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("PowerShell linked ZCode Skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("PowerShell canonical Skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell generic duplicate remains: %v", err)
	}
}

func TestInstallPowerShellBackupFailureWritesNoMultiSkills(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
$ok = Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME
if ($ok) { exit 2 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

	home := t.TempDir()
	base := filepath.Join(home, ".agents", "skills")
	multi := filepath.Join(t.TempDir(), "multi")
	mustWriteFile(t, filepath.Join(base, "dws", "SKILL.md"), []byte("old mono\n"), 0o644)
	mustWriteFile(t, filepath.Join(multi, "dingtalk-test", "SKILL.md"), []byte("new multi\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".dws", "skill-backups"), []byte("not a directory\n"), 0o644)

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_MULTI="+multi,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell harness failed: %v\n%s", err, output)
	}
	if data, err := os.ReadFile(filepath.Join(base, "dws", "SKILL.md")); err != nil || string(data) != "old mono\n" {
		t.Fatalf("mono changed after PowerShell backup failure (data=%q, err=%v)", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-test")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell installed multi after backup failure, stat err=%v", err)
	}
}

func TestInstallPowerShellMultiMonoSwitchEndToEnd(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
if (!(Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME)) { exit 2 }
if (!(Install-SkillsToHomes -SkillSrc $env:DWS_TEST_MONO -Root $env:DWS_TEST_HOME)) { exit 3 }
if (!(Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_MULTI -Root $env:DWS_TEST_HOME)) { exit 4 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-switch-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

	home := t.TempDir()
	base := filepath.Join(home, ".agents", "skills")
	multi := filepath.Join(t.TempDir(), "multi")
	mono := filepath.Join(t.TempDir(), "mono")
	mustWriteFile(t, filepath.Join(multi, "dingtalk-test", "SKILL.md"), []byte("new multi\n"), 0o644)
	mustWriteFile(t, filepath.Join(multi, "dingtalk-shared", "SKILL.md"), []byte("shared\n"), 0o644)
	mustWriteFile(t, filepath.Join(mono, "SKILL.md"), []byte("new mono\n"), 0o644)
	mustWriteFile(t, filepath.Join(base, "user-owned", "SKILL.md"), []byte("keep\n"), 0o644)
	mustWriteFile(t, filepath.Join(base, "dingtalk-custom", "SKILL.md"), []byte("market skill\n"), 0o644)
	mustWriteFile(t, filepath.Join(base, "dingtalk-aitable", "SKILL.md"), []byte("pre-state official\n"), 0o644)

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_MULTI="+multi,
		"DWS_TEST_MONO="+mono,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell multi -> mono -> multi harness failed: %v\n%s", err, output)
	}
	for _, name := range []string{"dingtalk-test", "dingtalk-shared"} {
		if _, err := os.Stat(filepath.Join(base, name, "SKILL.md")); err != nil {
			t.Fatalf("PowerShell final multi layout missing %s: %v\n%s", name, err, output)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "dws")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell final multi layout retained mono dws/: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(base, "user-owned", "SKILL.md")); err != nil || string(got) != "keep\n" {
		t.Fatalf("PowerShell switch changed non-DWS Skill: data=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(base, "dingtalk-custom", "SKILL.md")); err != nil || string(got) != "market skill\n" {
		t.Fatalf("PowerShell switch changed unregistered market/user dingtalk-* Skill: data=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-aitable")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell switch retained pre-state official Skill: %v", err)
	}
	assertSkillProvenance(t, home, filepath.Join(base, "dingtalk-test"), "dingtalk-test", "install.ps1")
	if matches, err := filepath.Glob(filepath.Join(home, ".dws", "skill-backups", "*", "*")); err != nil || len(matches) == 0 {
		t.Fatalf("PowerShell switch created no recoverable backups: matches=%v err=%v\n%s", matches, err, output)
	}
}

func TestInstallerShellCacheCopyFailurePreservesOldCache(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			source := filepath.Join(t.TempDir(), "multi")
			cache := filepath.Join(home, ".dws", "skills", "multi")
			mustWriteFile(t, filepath.Join(source, "dingtalk-new", "SKILL.md"), []byte("new cache\n"), 0o644)
			mustWriteFile(t, filepath.Join(cache, "dingtalk-old", "SKILL.md"), []byte("old cache\n"), 0o644)

			harness := `. "$DWS_TEST_LIBRARY"
cp() { return 1; }
if publish_skill_cache "$DWS_TEST_SOURCE" "$DWS_TEST_CACHE"; then
  exit 2
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_SOURCE="+source,
				"DWS_TEST_CACHE="+cache,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s cache harness failed: %v\n%s", scriptName, err, output)
			}
			if got, err := os.ReadFile(filepath.Join(cache, "dingtalk-old", "SKILL.md")); err != nil || string(got) != "old cache\n" {
				t.Fatalf("%s old cache = %q, err = %v", scriptName, got, err)
			}
			if _, err := os.Stat(filepath.Join(cache, "dingtalk-new")); !os.IsNotExist(err) {
				t.Fatalf("%s published new cache after copy failure: %v", scriptName, err)
			}
			if matches, err := filepath.Glob(filepath.Join(filepath.Dir(cache), ".multi.tmp.*")); err != nil || len(matches) != 0 {
				t.Fatalf("%s staging leftovers = %v, err = %v", scriptName, matches, err)
			}
		})
	}
}

func TestInstallerShellManagedSkillNamesAreLiteral(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			statePath := filepath.Join(home, ".dws", "skills-state.json")
			mustWriteFile(t, statePath, []byte(`{
  "managed_skills": [
    {"name":"dingtalk-user[1]"},
    {"name": "dingtalk-user+2"}
  ]
}
`), 0o600)
			base := filepath.Join(home, ".agents", "skills")
			unregistered := filepath.Join(base, "dingtalk-.*")
			compact := filepath.Join(base, "dingtalk-user[1]")
			spaced := filepath.Join(base, "dingtalk-user+2")
			for _, dir := range []string{unregistered, compact, spaced} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			harness := `. "$DWS_TEST_LIBRARY"
if is_managed_multi_skill_dir "$DWS_TEST_UNREGISTERED"; then
  exit 2
fi
is_managed_multi_skill_dir "$DWS_TEST_COMPACT" || exit 3
is_managed_multi_skill_dir "$DWS_TEST_SPACED" || exit 4
printf '{"other":[{"name":"dingtalk-user[1]"}]}\n' > "$DWS_TEST_STATE"
if is_managed_multi_skill_dir "$DWS_TEST_COMPACT"; then
  exit 5
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_STATE="+statePath,
				"DWS_TEST_UNREGISTERED="+unregistered,
				"DWS_TEST_COMPACT="+compact,
				"DWS_TEST_SPACED="+spaced,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s literal ownership harness failed: %v\n%s", scriptName, err, output)
			}
		})
	}
}

func TestInstallerShellMultiCopyFailureReturnsFailure(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			source := filepath.Join(t.TempDir(), "multi")
			base := filepath.Join(home, ".agents", "skills")
			mustWriteFile(t, filepath.Join(source, "dingtalk-test", "SKILL.md"), []byte("new skill\n"), 0o644)

			installCall := `install_multi_skills_to_homes "$DWS_TEST_SOURCE"`
			if scriptName == "install-skills.sh" {
				installCall = `install_multi_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
			}
			harness := `. "$DWS_TEST_LIBRARY"
cp() { return 1; }
if ` + installCall + `; then
  exit 2
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_SOURCE="+source,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s copy-failure harness failed: %v\n%s", scriptName, err, output)
			}
			if strings.Contains(string(output), "✅ Skills") {
				t.Fatalf("%s reported success after copy failure:\n%s", scriptName, output)
			}
			if !strings.Contains(string(output), "所有检测到的 Agent 目标均失败") {
				t.Fatalf("%s did not report aggregate install failure:\n%s", scriptName, output)
			}
			if _, err := os.Stat(filepath.Join(base, "dingtalk-test", "SKILL.md")); !os.IsNotExist(err) {
				t.Fatalf("%s left a completed Skill after copy failure: %v", scriptName, err)
			}
		})
	}
}

func TestInstallerShellMultiTransactionFailuresRestoreOldSet(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		for _, failureKind := range []string{"backup", "publish"} {
			failureKind := failureKind
			t.Run(scriptName+"/"+failureKind, func(t *testing.T) {
				t.Parallel()
				scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(scriptPath)
				if err != nil {
					t.Fatal(err)
				}
				cut := strings.LastIndex(string(data), "\nmain\n")
				if cut < 0 {
					t.Fatalf("%s final main invocation not found", scriptName)
				}
				library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
				mustWriteFile(t, library, data[:cut], 0o755)

				home := t.TempDir()
				source := filepath.Join(t.TempDir(), "multi")
				base := filepath.Join(home, ".agents", "skills")
				first := filepath.Join(base, "dingtalk-first")
				second := filepath.Join(base, "dingtalk-second")
				mustWriteFile(t, filepath.Join(source, "dingtalk-first", "SKILL.md"), []byte("new first\n"), 0o644)
				mustWriteFile(t, filepath.Join(source, "dingtalk-second", "SKILL.md"), []byte("new second\n"), 0o644)
				mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
				mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)

				installCall := `install_multi_skills_to_homes "$DWS_TEST_SOURCE"`
				if scriptName == "install-skills.sh" {
					installCall = `install_multi_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
				}
				harness := `. "$DWS_TEST_LIBRARY"
mv() {
  if [ "$DWS_TEST_FAILURE_KIND" = backup ] && [ "${1%/}" = "$DWS_TEST_SECOND" ]; then
    return 1
  fi
  if [ "$DWS_TEST_FAILURE_KIND" = publish ]; then
    case "$1" in
      */.dws-multi-set.*/dingtalk-second/*) return 1 ;;
    esac
  fi
  command mv "$@"
}
if ` + installCall + `; then
  exit 2
fi
`
				cmd := exec.Command("sh", "-c", harness)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"DWS_TEST_LIBRARY="+library,
					"DWS_TEST_SOURCE="+source,
					"DWS_TEST_SECOND="+second,
					"DWS_TEST_FAILURE_KIND="+failureKind,
				)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s %s-failure harness failed: %v\n%s", scriptName, failureKind, err, output)
				}
				if got, err := os.ReadFile(filepath.Join(first, "SKILL.md")); err != nil || string(got) != "old first\n" {
					t.Fatalf("%s first Skill after %s failure = %q, err=%v", scriptName, failureKind, got, err)
				}
				if got, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(got) != "old second\n" {
					t.Fatalf("%s second Skill after %s failure = %q, err=%v", scriptName, failureKind, got, err)
				}
				if matches, err := filepath.Glob(filepath.Join(base, ".dws-multi-set.*")); err != nil || len(matches) != 0 {
					t.Fatalf("%s staging leftovers after %s failure = %v, err=%v", scriptName, failureKind, matches, err)
				}
			})
		}
	}
}

func TestInstallerShellMonoCopyFailureReturnsFailure(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			source := filepath.Join(t.TempDir(), "mono")
			base := filepath.Join(home, ".agents", "skills")
			mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new mono\n"), 0o644)

			installCall := `install_skills_to_homes "$DWS_TEST_SOURCE"`
			if scriptName == "install-skills.sh" {
				installCall = `install_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
			}
			harness := `. "$DWS_TEST_LIBRARY"
cp() { return 1; }
if ` + installCall + `; then
  exit 2
fi
`
			cmd := exec.Command("sh", "-c", harness)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DWS_TEST_LIBRARY="+library,
				"DWS_TEST_SOURCE="+source,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s mono copy-failure harness failed: %v\n%s", scriptName, err, output)
			}
			if strings.Contains(string(output), "✅ Skills") {
				t.Fatalf("%s reported mono success after copy failure:\n%s", scriptName, output)
			}
			if !strings.Contains(string(output), "未安装任何 mono Skill") {
				t.Fatalf("%s did not report aggregate mono install failure:\n%s", scriptName, output)
			}
			if _, err := os.Stat(filepath.Join(base, "dws", "SKILL.md")); !os.IsNotExist(err) {
				t.Fatalf("%s left a completed mono Skill after copy failure: %v", scriptName, err)
			}
		})
	}
}

func TestInstallerShellMonoTransactionFailuresRestoreOldSet(t *testing.T) {
	for _, scriptName := range []string{"install.sh", "install-skills.sh"} {
		scriptName := scriptName
		for _, failureKind := range []string{"backup", "publish"} {
			failureKind := failureKind
			t.Run(scriptName+"/"+failureKind, func(t *testing.T) {
				t.Parallel()
				scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(scriptPath)
				if err != nil {
					t.Fatal(err)
				}
				cut := strings.LastIndex(string(data), "\nmain\n")
				if cut < 0 {
					t.Fatalf("%s final main invocation not found", scriptName)
				}
				library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
				mustWriteFile(t, library, data[:cut], 0o755)

				home := t.TempDir()
				source := filepath.Join(t.TempDir(), "mono")
				base := filepath.Join(home, ".agents", "skills")
				first := filepath.Join(base, "dingtalk-aitable")
				second := filepath.Join(base, "dingtalk-calendar")
				mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new mono\n"), 0o644)
				mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
				mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)

				installCall := `install_skills_to_homes "$DWS_TEST_SOURCE"`
				if scriptName == "install-skills.sh" {
					installCall = `install_skills_to_root "$DWS_TEST_SOURCE" "$HOME"`
				}
				harness := `. "$DWS_TEST_LIBRARY"
mv() {
  if [ "$DWS_TEST_FAILURE_KIND" = backup ] && [ "${1%/}" = "$DWS_TEST_SECOND" ]; then
    return 1
  fi
  if [ "$DWS_TEST_FAILURE_KIND" = publish ]; then
    case "$1" in
      */.dws-mono-set.*/dws/*) return 1 ;;
    esac
  fi
  command mv "$@"
}
if ` + installCall + `; then
  exit 2
fi
`
				cmd := exec.Command("sh", "-c", harness)
				cmd.Env = append(os.Environ(),
					"HOME="+home,
					"DWS_TEST_LIBRARY="+library,
					"DWS_TEST_SOURCE="+source,
					"DWS_TEST_SECOND="+second,
					"DWS_TEST_FAILURE_KIND="+failureKind,
				)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s mono %s-failure harness failed: %v\n%s", scriptName, failureKind, err, output)
				}
				if got, err := os.ReadFile(filepath.Join(first, "SKILL.md")); err != nil || string(got) != "old first\n" {
					t.Fatalf("%s first multi Skill after mono %s failure = %q, err=%v", scriptName, failureKind, got, err)
				}
				if got, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(got) != "old second\n" {
					t.Fatalf("%s second multi Skill after mono %s failure = %q, err=%v", scriptName, failureKind, got, err)
				}
				if _, err := os.Stat(filepath.Join(base, "dws")); !os.IsNotExist(err) {
					t.Fatalf("%s exposed mono dws after %s failure: %v", scriptName, failureKind, err)
				}
				if matches, err := filepath.Glob(filepath.Join(base, ".dws-mono-set.*")); err != nil || len(matches) != 0 {
					t.Fatalf("%s mono staging leftovers after %s failure = %v, err=%v", scriptName, failureKind, matches, err)
				}
			})
		}
	}
}

func TestInstallPowerShellMultiTransactionFailuresRestoreOldSet(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}

	for _, failureKind := range []string{"backup", "publish"} {
		failureKind := failureKind
		t.Run(failureKind, func(t *testing.T) {
			home := t.TempDir()
			base := filepath.Join(home, ".agents", "skills")
			source := filepath.Join(t.TempDir(), "multi")
			first := filepath.Join(base, "dingtalk-first")
			second := filepath.Join(base, "dingtalk-second")
			mustWriteFile(t, filepath.Join(source, "dingtalk-first", "SKILL.md"), []byte("new first\n"), 0o644)
			mustWriteFile(t, filepath.Join(source, "dingtalk-second", "SKILL.md"), []byte("new second\n"), 0o644)
			mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
			mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)

			prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
			prefix += `
function Move-SkillPath {
    param([string]$Source, [string]$Destination)
    if ($env:DWS_TEST_FAILURE_KIND -eq "backup" -and $Source -eq $env:DWS_TEST_SECOND) {
        throw "injected second backup failure"
    }
    if ($env:DWS_TEST_FAILURE_KIND -eq "publish" -and
        $Source -match "[\\/].dws-multi-set-[^\\/]+[\\/]dingtalk-second$") {
        throw "injected second publish failure"
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
}
$ok = Install-MultiSkillsToHomes -MultiSrc $env:DWS_TEST_SOURCE -Root $env:DWS_TEST_HOME
if ($ok) { exit 2 }
exit 0
`
			harnessPath := filepath.Join(t.TempDir(), "install-transaction-harness.ps1")
			mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

			cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
			cmd.Env = append(os.Environ(),
				"DWS_TEST_HOME="+home,
				"DWS_TEST_SOURCE="+source,
				"DWS_TEST_SECOND="+second,
				"DWS_TEST_FAILURE_KIND="+failureKind,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("PowerShell %s-failure harness failed: %v\n%s", failureKind, err, output)
			}
			if got, err := os.ReadFile(filepath.Join(first, "SKILL.md")); err != nil || string(got) != "old first\n" {
				t.Fatalf("PowerShell first Skill after %s failure = %q, err=%v", failureKind, got, err)
			}
			if got, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(got) != "old second\n" {
				t.Fatalf("PowerShell second Skill after %s failure = %q, err=%v", failureKind, got, err)
			}
			if matches, err := filepath.Glob(filepath.Join(base, ".dws-multi-set-*")); err != nil || len(matches) != 0 {
				t.Fatalf("PowerShell staging leftovers after %s failure = %v, err=%v", failureKind, matches, err)
			}
		})
	}
}

func TestInstallPowerShellCrossDeviceMoveContract(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")

	for _, phase := range []string{"success", "copy", "verify", "remove", "permission", "dest-verify"} {
		phase := phase
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			source := filepath.Join(root, "external", "skill")
			destination := filepath.Join(home, ".dws", "skill-backups", "stamp", "skill")
			restored := filepath.Join(root, "external", "restored-skill")
			mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("old skill\n"), 0o640)
			linksCreated := false
			if phase == "success" {
				if err := os.Symlink("SKILL.md", filepath.Join(source, "skill-link")); err == nil {
					if err := os.Symlink("missing.md", filepath.Join(source, "dangling-link")); err != nil {
						t.Fatal(err)
					}
					linksCreated = true
				} else if runtime.GOOS != "windows" {
					t.Fatal(err)
				}
			}

			harness := prefix + `
$script:OriginalCopySkillPathLexically = ${function:Copy-SkillPathLexically}
$script:OriginalAssertSkillPathCopy = ${function:Assert-SkillPathCopy}
$script:OriginalRemoveSkillPathLexically = ${function:Remove-SkillPathLexically}
function New-CrossDeviceError { return [System.IO.IOException]::new("injected cross-device rename", -2147024879) }
function Move-SkillPath {
    param([string]$Source, [string]$Destination)
    if ($env:DWS_TEST_PHASE -eq "permission" -and $Source -eq $env:DWS_TEST_SOURCE) {
        throw [System.UnauthorizedAccessException]::new("permission denied")
    }
    if (($Source -eq $env:DWS_TEST_SOURCE -and $Destination -eq $env:DWS_TEST_DESTINATION) -or
        ($Source -eq $env:DWS_TEST_DESTINATION -and $Destination -eq $env:DWS_TEST_RESTORED)) {
        throw (New-CrossDeviceError)
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
}
function Copy-SkillPathLexically {
    param([string]$Source, [string]$Destination)
    if ($env:DWS_TEST_PHASE -eq "copy") { throw "copy failed" }
    if ($env:DWS_TEST_PHASE -eq "permission") {
        [System.IO.File]::WriteAllText($env:DWS_TEST_COPY_MARKER, "called")
    }
    & $script:OriginalCopySkillPathLexically -Source $Source -Destination $Destination
}
function Assert-SkillPathCopy {
    param([string]$Source, [string]$Destination)
    if ($env:DWS_TEST_PHASE -eq "verify") { throw "verify failed" }
    if ($env:DWS_TEST_PHASE -eq "dest-verify" -and $Destination -eq $env:DWS_TEST_DESTINATION) { throw "dest-verify failed" }
    & $script:OriginalAssertSkillPathCopy -Source $Source -Destination $Destination
}
function Remove-SkillPathLexically {
    param([string]$Path)
    if ($env:DWS_TEST_PHASE -eq "remove" -and $Path -eq $env:DWS_TEST_SOURCE) { throw "remove failed" }
    & $script:OriginalRemoveSkillPathLexically -Path $Path
}
try {
    Move-SkillPathRecoverably -Source $env:DWS_TEST_SOURCE -Destination $env:DWS_TEST_DESTINATION
    if ($env:DWS_TEST_PHASE -ne "success") { exit 2 }
    Move-SkillPathRecoverably -Source $env:DWS_TEST_DESTINATION -Destination $env:DWS_TEST_RESTORED
    exit 0
} catch {
    if ($env:DWS_TEST_PHASE -eq "success") { Write-Error $_; exit 3 }
    if ($env:DWS_TEST_PHASE -eq "copy" -and $_ -notmatch "copy failed") { Write-Error $_; exit 4 }
    if ($env:DWS_TEST_PHASE -eq "verify" -and $_ -notmatch "verify failed") { Write-Error $_; exit 5 }
    if ($env:DWS_TEST_PHASE -eq "remove" -and $_ -notmatch "均保留") { Write-Error $_; exit 6 }
    if ($env:DWS_TEST_PHASE -eq "permission" -and $_ -notmatch "permission denied") { Write-Error $_; exit 7 }
    if ($env:DWS_TEST_PHASE -eq "dest-verify" -and $_ -notmatch "目标已撤回|dest-verify failed|状态不确定") { Write-Error $_; exit 8 }
    exit 0
}
`
			harnessPath := filepath.Join(t.TempDir(), "install-cross-device-harness.ps1")
			mustWriteFile(t, harnessPath, []byte(harness), 0o644)
			copyMarker := filepath.Join(root, "copy-called")
			cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
			cmd.Env = append(os.Environ(),
				"DWS_TEST_HOME="+home,
				"DWS_TEST_SOURCE="+source,
				"DWS_TEST_DESTINATION="+destination,
				"DWS_TEST_RESTORED="+restored,
				"DWS_TEST_COPY_MARKER="+copyMarker,
				"DWS_TEST_PHASE="+phase,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("PowerShell %s EXDEV harness failed: %v\n%s", phase, err, output)
			}

			if phase == "success" {
				if got, err := os.ReadFile(filepath.Join(restored, "SKILL.md")); err != nil || string(got) != "old skill\n" {
					t.Fatalf("restored content = %q, %v", got, err)
				}
				if _, err := os.Lstat(source); !os.IsNotExist(err) {
					t.Fatalf("source must be removed: %v", err)
				}
				if _, err := os.Lstat(destination); !os.IsNotExist(err) {
					t.Fatalf("backup must be consumed by rollback: %v", err)
				}
				if linksCreated {
					for name, want := range map[string]string{"skill-link": "SKILL.md", "dangling-link": "missing.md"} {
						info, err := os.Lstat(filepath.Join(restored, name))
						if err != nil || info.Mode()&os.ModeSymlink == 0 {
							t.Fatalf("%s must remain a symlink: %v, %v", name, info, err)
						}
						if got, err := os.Readlink(filepath.Join(restored, name)); err != nil || got != want {
							t.Fatalf("%s target = %q, %v; want %q", name, got, err, want)
						}
					}
				}
				return
			}

			if got, err := os.ReadFile(filepath.Join(source, "SKILL.md")); err != nil || string(got) != "old skill\n" {
				t.Fatalf("source after %s failure = %q, %v", phase, got, err)
			}
			if phase == "remove" {
				if got, err := os.ReadFile(filepath.Join(destination, "SKILL.md")); err != nil || string(got) != "old skill\n" {
					t.Fatalf("published backup after remove failure = %q, %v", got, err)
				}
			} else if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination after %s failure = %v", phase, err)
			}
			if phase == "permission" {
				if _, err := os.Stat(copyMarker); !os.IsNotExist(err) {
					t.Fatalf("permission error must not trigger copy fallback: %v", err)
				}
			}
			if matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".skill.cross-device-*")); err != nil || len(matches) != 0 {
				t.Fatalf("PowerShell %s staging leftovers = %v, %v", phase, matches, err)
			}
		})
	}
}

func TestInstallDevappPowerShellCrossDeviceBackup(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "external", "dingtalk-misc")
	mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("old dev skill\n"), 0o640)
	linksCreated := false
	if err := os.Symlink("SKILL.md", filepath.Join(source, "skill-link")); err == nil {
		if err := os.Symlink("missing.md", filepath.Join(source, "dangling-link")); err != nil {
			t.Fatal(err)
		}
		linksCreated = true
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
function Move-DevSkillPath([string]$Source, [string]$Destination) {
    if ($Source -eq $env:DWS_TEST_SOURCE) {
        throw [System.IO.IOException]::new("injected cross-device rename", -2147024879)
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
}
Backup-DevSkill $env:DWS_TEST_SOURCE
if (Test-PathLexically $env:DWS_TEST_SOURCE) { exit 2 }
`
	harnessPath := filepath.Join(t.TempDir(), "install-devapp-cross-device-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(), "DWS_TEST_HOME="+home, "DWS_TEST_SOURCE="+source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell devapp EXDEV backup failed: %v\n%s", err, output)
	}
	backup := findSkillBackup(home, "dingtalk-misc", "old dev skill\n")
	if backup == "" {
		t.Fatal("PowerShell devapp backup missing")
	}
	if linksCreated {
		for name, want := range map[string]string{"skill-link": "SKILL.md", "dangling-link": "missing.md"} {
			info, err := os.Lstat(filepath.Join(backup, name))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("devapp %s must remain a symlink: %v, %v", name, info, err)
			}
			if got, err := os.Readlink(filepath.Join(backup, name)); err != nil || got != want {
				t.Fatalf("devapp %s target = %q, %v; want %q", name, got, err, want)
			}
		}
	}
}

func TestInstallDevappPowerShellPublishFailureRestoresAcrossDevice(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "bundle", "dingtalk-misc")
	destination := filepath.Join(root, "external", "dingtalk-misc")
	mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new dev skill\n"), 0o640)
	mustWriteFile(t, filepath.Join(destination, "SKILL.md"), []byte("old dev skill\n"), 0o644)
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
function Move-DevSkillPath([string]$Source, [string]$Destination) {
    if ($Source -match "[\\/].dws-dev-copy-[^\\/]+[\\/]payload$") { throw "injected publish failure" }
    if ($Source -match "[\\/]skill-backups[\\/]" -and $Destination -eq $env:DWS_TEST_DESTINATION) {
        throw [System.IO.IOException]::new("injected cross-device rollback", -2147024879)
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
}
try {
    Publish-DevSkillCopy $env:DWS_TEST_SOURCE $env:DWS_TEST_DESTINATION
    exit 2
} catch {
    if ($_ -notmatch "injected publish failure") { Write-Error $_; exit 3 }
}
`
	harnessPath := filepath.Join(t.TempDir(), "install-devapp-publish-rollback-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_SOURCE="+source,
		"DWS_TEST_DESTINATION="+destination,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell devapp publish rollback failed: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(destination, "SKILL.md")); err != nil || string(got) != "old dev skill\n" {
		t.Fatalf("restored devapp content = %q, %v", got, err)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".dws-dev-copy-*")); err != nil || len(matches) != 0 {
		t.Fatalf("devapp publish staging leftovers = %v, %v", matches, err)
	}
}

func TestInstallDevappPowerShellPublicationRacePreservesConcurrentDirectory(t *testing.T) {
	powerShellNames := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		powerShellNames = []string{"powershell", "pwsh"}
	}
	pwsh := ""
	for _, name := range powerShellNames {
		if candidate, err := exec.LookPath(name); err == nil {
			pwsh = candidate
			break
		}
	}
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-devapp.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(string(data), "# Read the releases list")
	if cut < 0 {
		t.Fatal("install-devapp.ps1 release section not found")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "bundle", "dingtalk-misc")
	destination := filepath.Join(root, "external", "dingtalk-misc")
	mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new dev skill\n"), 0o640)
	mustWriteFile(t, filepath.Join(destination, "SKILL.md"), []byte("old dev skill\n"), 0o644)

	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	harness := prefix + `
$script:OriginalBackupDevSkill = ${function:Backup-DevSkill}
function Backup-DevSkill([string]$path, [ref]$BackupPath) {
    & $script:OriginalBackupDevSkill $path $BackupPath
    # Simulate another installer recreating the destination after the old
    # content was backed up but before this transaction publishes its stage.
    New-Item -ItemType Directory -Path $path -Force -ErrorAction Stop | Out-Null
    Set-Content -LiteralPath (Join-Path $path 'concurrent-user-data.txt') -Value 'keep'
}
try {
    Publish-DevSkillCopy $env:DWS_TEST_SOURCE $env:DWS_TEST_DESTINATION
    exit 2
} catch {
    if (!$_.Exception.Data['DWSRollbackFailed']) { Write-Error $_; exit 3 }
}
if (!(Test-Path -LiteralPath (Join-Path $env:DWS_TEST_DESTINATION 'concurrent-user-data.txt') -PathType Leaf)) { exit 4 }
if (Test-Path -LiteralPath (Join-Path $env:DWS_TEST_DESTINATION 'payload')) { exit 5 }
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-devapp-publication-race.ps1")
	mustWriteFile(t, harnessPath, []byte(harness), 0o644)
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_SOURCE="+source,
		"DWS_TEST_DESTINATION="+destination,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell devapp publication race must retain concurrent directory: %v\n%s", err, output)
	}
	if got, err := os.ReadFile(filepath.Join(destination, "concurrent-user-data.txt")); err != nil || strings.TrimSpace(string(got)) != "keep" {
		t.Fatalf("concurrent devapp data = %q, %v", got, err)
	}
	if backup := findSkillBackup(home, "dingtalk-misc", "old dev skill\n"); backup == "" {
		t.Fatal("original devapp backup must be retained after publication race")
	}
}

func TestInstallPowerShellMonoTransactionFailuresRestoreOldSet(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}

	for _, failureKind := range []string{"backup", "publish"} {
		failureKind := failureKind
		t.Run(failureKind, func(t *testing.T) {
			home := t.TempDir()
			base := filepath.Join(home, ".agents", "skills")
			source := filepath.Join(t.TempDir(), "mono")
			first := filepath.Join(base, "dingtalk-aitable")
			second := filepath.Join(base, "dingtalk-calendar")
			mustWriteFile(t, filepath.Join(source, "SKILL.md"), []byte("new mono\n"), 0o644)
			mustWriteFile(t, filepath.Join(first, "SKILL.md"), []byte("old first\n"), 0o644)
			mustWriteFile(t, filepath.Join(second, "SKILL.md"), []byte("old second\n"), 0o644)

			prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
			prefix += `
function Move-SkillPath {
    param([string]$Source, [string]$Destination)
    if ($env:DWS_TEST_FAILURE_KIND -eq "backup" -and $Source -eq $env:DWS_TEST_SECOND) {
        throw "injected second backup failure"
    }
    if ($env:DWS_TEST_FAILURE_KIND -eq "publish" -and
        $Source -match "[\\/].dws-mono-set-[^\\/]+[\\/]dws$") {
        throw "injected mono publish failure"
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
}
$ok = Install-SkillsToHomes -SkillSrc $env:DWS_TEST_SOURCE -Root $env:DWS_TEST_HOME
if ($ok) { exit 2 }
exit 0
`
			harnessPath := filepath.Join(t.TempDir(), "install-mono-transaction-harness.ps1")
			mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

			cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
			cmd.Env = append(os.Environ(),
				"DWS_TEST_HOME="+home,
				"DWS_TEST_SOURCE="+source,
				"DWS_TEST_SECOND="+second,
				"DWS_TEST_FAILURE_KIND="+failureKind,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("PowerShell mono %s-failure harness failed: %v\n%s", failureKind, err, output)
			}
			if got, err := os.ReadFile(filepath.Join(first, "SKILL.md")); err != nil || string(got) != "old first\n" {
				t.Fatalf("PowerShell first multi Skill after mono %s failure = %q, err=%v", failureKind, got, err)
			}
			if got, err := os.ReadFile(filepath.Join(second, "SKILL.md")); err != nil || string(got) != "old second\n" {
				t.Fatalf("PowerShell second multi Skill after mono %s failure = %q, err=%v", failureKind, got, err)
			}
			if _, err := os.Stat(filepath.Join(base, "dws")); !os.IsNotExist(err) {
				t.Fatalf("PowerShell exposed mono dws after %s failure: %v", failureKind, err)
			}
			if matches, err := filepath.Glob(filepath.Join(base, ".dws-mono-set-*")); err != nil || len(matches) != 0 {
				t.Fatalf("PowerShell mono staging leftovers after %s failure = %v, err=%v", failureKind, matches, err)
			}
		})
	}
}

func TestInstallPowerShellCacheCopyFailurePreservesOldCache(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell")
		}
		if err != nil {
			t.Skip("PowerShell is not available")
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndex(string(data), "# ── Main")
	if cut < 0 {
		t.Fatal("install.ps1 main section not found")
	}
	prefix := strings.ReplaceAll(string(data[:cut]), "$HOME", "$env:DWS_TEST_HOME")
	prefix += `
function Copy-DirRecursive { throw "injected cache copy failure" }
Cache-MultiSkills -Source $env:DWS_TEST_SOURCE
exit 0
`
	harnessPath := filepath.Join(t.TempDir(), "install-cache-harness.ps1")
	mustWriteFile(t, harnessPath, []byte(prefix), 0o644)

	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "multi")
	cache := filepath.Join(home, ".dws", "skills", "multi")
	mustWriteFile(t, filepath.Join(source, "dingtalk-new", "SKILL.md"), []byte("new cache\n"), 0o644)
	mustWriteFile(t, filepath.Join(cache, "dingtalk-old", "SKILL.md"), []byte("old cache\n"), 0o644)

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath)
	cmd.Env = append(os.Environ(),
		"DWS_TEST_HOME="+home,
		"DWS_TEST_SOURCE="+source,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell cache harness failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "未覆盖原缓存") {
		t.Fatalf("PowerShell cache failure warning missing:\n%s", output)
	}
	if got, err := os.ReadFile(filepath.Join(cache, "dingtalk-old", "SKILL.md")); err != nil || string(got) != "old cache\n" {
		t.Fatalf("PowerShell old cache = %q, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(cache, "dingtalk-new")); !os.IsNotExist(err) {
		t.Fatalf("PowerShell published new cache after copy failure: %v", err)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(cache), ".multi.tmp-*")); err != nil || len(matches) != 0 {
		t.Fatalf("PowerShell staging leftovers = %v, err = %v", matches, err)
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", path, err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s) error = %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(%s) error = %v", path, err)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", path, err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip Create(%s) error = %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write(%s) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(%s) error = %v", path, err)
	}
}

func writeFakeCurl(t *testing.T, path string) {
	t.Helper()
	const script = `#!/bin/sh
set -eu
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
[ -n "$out" ] || { echo "fake curl: missing -o" >&2; exit 1; }
case "$url" in
  *"/${FAKE_ASSET_NAME}") cp "$FAKE_RELEASE_DIR/$FAKE_ASSET_NAME" "$out" ;;
  *"/dws-skills.zip") cp "$FAKE_RELEASE_DIR/dws-skills.zip" "$out" ;;
  *"/checksums.txt") cp "$FAKE_RELEASE_DIR/checksums.txt" "$out" ;;
  *) echo "fake curl: unexpected URL $url" >&2; exit 1 ;;
esac
`
	mustWriteFile(t, path, []byte(script), 0o755)
}

func writeInstallerFixtureChecksums(t *testing.T, releaseDir string) {
	t.Helper()
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", releaseDir, err)
	}
	var lines []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(releaseDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}
		digest := sha256.Sum256(data)
		lines = append(lines, hex.EncodeToString(digest[:])+"  "+entry.Name())
	}
	mustWriteFile(t, filepath.Join(releaseDir, "checksums.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func writeFakeGH(t *testing.T, path, version string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' '" + version + "'\n"
	mustWriteFile(t, path, []byte(script), 0o755)
}

func mustWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
