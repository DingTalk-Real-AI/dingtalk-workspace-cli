package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
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

func mustWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
