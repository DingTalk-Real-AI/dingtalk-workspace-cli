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
)

var expectedHomeSkillTargets = []string{
	".agents/skills/dws",
	".cursor/skills/dws",
}

func TestInstallScriptSourceModeInstallsBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	installDir := filepath.Join(root, "bin")

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("Abs(install.sh) error = %v", err)
	}

	// Stub make: when invoked as "make -C <dir> build", create a fake dws binary
	// in the project root (the -C target directory).
	stubRoot := filepath.Join(root, "stubs")
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
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
	// Also need a stub go so need_cmd check passes
	mustWriteFile(t, filepath.Join(stubRoot, "go"), []byte("#!/bin/sh\ntrue\n"), 0o755)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"DWS_INSTALL_DIR="+installDir,
		"DWS_INSTALL_NAME=dws-test",
	)
	output, err := cmd.CombinedOutput()

	// Clean up the fake binary created in the real repo root
	_ = os.Remove(filepath.Join(repoRoot, "dws"))

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	got := string(output)
	for _, want := range []string{
		"Installing dws from source checkout",
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

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("Abs(install.sh) error = %v", err)
	}

	stubRoot := filepath.Join(root, "stubs")
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
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

	// Gate for index>0 agent dirs (matches build/npm/install.js): parent must exist.
	if err := os.MkdirAll(filepath.Join(fakeHome, ".cursor"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.cursor) error = %v", err)
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"DWS_INSTALL_DIR="+installDir,
	)
	output, err := cmd.CombinedOutput()

	// Clean up the fake binary created in the real repo root
	_ = os.Remove(filepath.Join(repoRoot, "dws"))

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	for _, rel := range expectedHomeSkillTargets {
		skillPath := filepath.Join(fakeHome, filepath.FromSlash(rel), "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("Stat(%s) error = %v\noutput:\n%s", skillPath, err, string(output))
		}
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
		"multi/dingtalk-event/SKILL.md": "event skill user_im_message_receive_o2o\n",
		"mono/SKILL.md":                 "mono skill user_im_message_receive_o2o\n",
		"SKILL.md":                      "legacy mono root\n",
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
	got := string(output)
	for _, want := range []string{
		"Version: v1.0.51",
		"Skill dingtalk-event",
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

	for _, rel := range []string{
		".agents/skills/dingtalk-event/SKILL.md",
		".codex/skills/dingtalk-event/SKILL.md",
		".config/opencode/skills/dingtalk-event/SKILL.md",
		".agents/skills/dws/SKILL.md",
		".codex/skills/dws/SKILL.md",
		".dws/skills/multi/dingtalk-event/SKILL.md",
		".dws/skills/mono/SKILL.md",
	} {
		p := filepath.Join(fakeHome, filepath.FromSlash(rel))
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v\noutput:\n%s", p, err, got)
		}
		if !strings.Contains(string(data), "user_im_message_receive_o2o") {
			t.Fatalf("%s does not contain event skill marker: %q", p, string(data))
		}
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
		"multi/dingtalk-event/SKILL.md": "event skill user_im_message_receive_o2o\n",
		"mono/SKILL.md":                 "mono skill user_im_message_receive_o2o\n",
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
	if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-event", "SKILL.md")); err != nil {
		t.Fatalf("skills-only should install event skill: %v\noutput:\n%s", err, string(output))
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
	if _, err := os.Stat(filepath.Join(fakeHome, ".agents", "skills", "dingtalk-event")); !os.IsNotExist(err) {
		t.Fatalf("DWS_NO_SKILLS=1 should not install event skill, stat err=%v\noutput:\n%s", err, string(output))
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
			relPath: filepath.Join("..", "..", "scripts", "policy", "check-command-surface.sh"),
			want:    `go build -ldflags="-s -w" -o "$BIN_PATH" ./cmd`,
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

	root := t.TempDir()
	fakeHome := filepath.Join(root, "home")
	installDir := filepath.Join(root, "bin")

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("Abs(install.sh) error = %v", err)
	}

	stubRoot := filepath.Join(root, "stubs")
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
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

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		"PATH="+stubRoot+":"+os.Getenv("PATH"),
		"DWS_INSTALL_DIR="+installDir,
	)
	output, err := cmd.CombinedOutput()

	// Clean up the fake binary created in the real repo root
	_ = os.Remove(filepath.Join(repoRoot, "dws"))

	if err != nil {
		t.Fatalf("install.sh error = %v\noutput:\n%s", err, string(output))
	}

	// Verify multi cache was populated. We expect dingtalk-* subdirs.
	multiCache := filepath.Join(fakeHome, ".dws", "skills", "multi")
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
	monoCacheSkill := filepath.Join(fakeHome, ".dws", "skills", "mono", "SKILL.md")
	if _, err := os.Stat(monoCacheSkill); err != nil {
		t.Fatalf("missing mono cache SKILL.md at %s: %v", monoCacheSkill, err)
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
