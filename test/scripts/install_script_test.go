package scripts_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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

	skillPath := filepath.Join(fixture.fakeHome, ".cursor", "skills", "dws", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("Stat(%s) error = %v\noutput:\n%s", skillPath, err, string(output))
	}
	genericPath := filepath.Join(fixture.fakeHome, ".agents", "skills", "dws")
	if _, err := os.Stat(genericPath); !os.IsNotExist(err) {
		t.Fatalf("generic Skill root must not duplicate detected Cursor: Stat(%s) = %v\noutput:\n%s", genericPath, err, string(output))
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

func TestInstallScriptsUseGitHubReleaseSkillsAsset(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		filepath.Join("..", "..", "scripts", "install.sh"),
		filepath.Join("..", "..", "scripts", "install-event.sh"),
		filepath.Join("..", "..", "scripts", "install.ps1"),
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
		if strings.Contains(text, "archive/refs/heads/main.tar.gz") || strings.Contains(text, "archive/refs/tags/") {
			t.Fatalf("%s should not download skills from repository archive refs", scriptPath)
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
		".agents/skills/dingtalk-event/SKILL.md":          "user_im_message_receive_o2o",
		".codex/skills/dingtalk-event/SKILL.md":           "user_im_message_receive_o2o",
		".config/opencode/skills/dingtalk-event/SKILL.md": "user_im_message_receive_o2o",
		".agents/skills/dingtalk-shared/SKILL.md":         "shared prerequisite",
		".agents/skills/dingtalk-misc/SKILL.md":           "clean misc oa routing",
		".agents/skills/dws/SKILL.md":                     "user_im_message_receive_o2o",
		".codex/skills/dws/SKILL.md":                      "user_im_message_receive_o2o",
		".dws/skills/multi/dingtalk-event/SKILL.md":       "user_im_message_receive_o2o",
		".dws/skills/multi/dingtalk-shared/SKILL.md":      "shared prerequisite",
		".dws/skills/multi/dingtalk-misc/SKILL.md":        "clean misc oa routing",
		".dws/skills/mono/SKILL.md":                       "user_im_message_receive_o2o",
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
	// ~/.dws/skill-backups/ instead of being destroyed.
	if findSkillBackup(fixture.fakeHome, "dws", "old mono\n") == "" {
		t.Errorf("old mono dws/ should be backed up under .dws/skill-backups, output:\n%s", output)
	}
	if findSkillBackup(fixture.fakeHome, "dingtalk-stale", "stale\n") == "" {
		t.Errorf("stale dingtalk-* should be backed up under .dws/skill-backups, output:\n%s", output)
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

func TestInstallerShellPrefersCodexRootWithoutGenericDuplicate(t *testing.T) {
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
			if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
				t.Fatalf("%s canonical Codex Skill missing: %v", scriptName, err)
			}
			for _, duplicate := range []string{
				filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md"),
				filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"),
			} {
				if _, err := os.Stat(duplicate); !os.IsNotExist(err) {
					t.Fatalf("%s generic duplicate remains at %s: %v", scriptName, duplicate, err)
				}
			}
		})
	}
}

func TestInstallPowerShellPrefersCodexRootWithoutGenericDuplicate(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("PowerShell canonical Codex Skill missing: %v", err)
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
      */.dws-multi-set.*/dingtalk-second) return 1 ;;
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
      */.dws-mono-set.*/dws) return 1 ;;
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
  *) echo "fake curl: unexpected URL $url" >&2; exit 1 ;;
esac
`
	mustWriteFile(t, path, []byte(script), 0o755)
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
