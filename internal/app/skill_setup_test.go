package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillSetupCommandRegistered(t *testing.T) {
	root := buildSkillCommand()
	var found bool
	for _, sub := range root.Commands() {
		if sub.Name() == "setup" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dws skill setup not registered as subcommand")
	}
}

func TestResolveSkillSetupModeFlagDirect(t *testing.T) {
	got, err := resolveSkillSetupMode("mono", true, &bytes.Buffer{})
	if err != nil || got != skillSetupModeMono {
		t.Fatalf("expected mono no-error, got %q err=%v", got, err)
	}
	got, err = resolveSkillSetupMode("MULTI", true, &bytes.Buffer{})
	if err != nil || got != skillSetupModeMulti {
		t.Fatalf("expected multi case-insensitive, got %q err=%v", got, err)
	}
	if _, err = resolveSkillSetupMode("hybrid", true, &bytes.Buffer{}); err == nil {
		t.Fatalf("expected error on invalid mode")
	}
}

func TestResolveSkillSetupModeNonInteractiveDefaultsMono(t *testing.T) {
	var buf bytes.Buffer
	got, err := resolveSkillSetupMode("", true, &buf)
	if err != nil || got != skillSetupModeMono {
		t.Fatalf("non-interactive empty mode should default to mono, got %q err=%v", got, err)
	}
	if !strings.Contains(buf.String(), "mono") {
		t.Fatalf("expected output to mention mono fallback, got %q", buf.String())
	}
}

func TestResolveSkillSetupSourceFindsMonoRoot(t *testing.T) {
	tmp := t.TempDir()
	monoDir := filepath.Join(tmp, "skills", "mono")
	if err := os.MkdirAll(monoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monoDir, "SKILL.md"), []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSkillSetupSource(tmp, skillSetupModeMono)
	if err != nil {
		t.Fatalf("expected to find mono source, got err=%v", err)
	}
	if got != monoDir {
		t.Fatalf("expected %s, got %s", monoDir, got)
	}
}

func TestResolveSkillSetupSourceErrorWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DWS_SKILL_SOURCE", "")
	_, err := resolveSkillSetupSource(tmp, skillSetupModeMono)
	if err == nil {
		t.Fatalf("expected error when source missing")
	}
	if !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("expected 未找到 message, got %v", err)
	}
}

func TestResolveSkillSetupTargetsSingleAgent(t *testing.T) {
	got, err := resolveSkillSetupTargets("claude")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 dest, got %d", len(got))
	}
	if !strings.Contains(got[0], ".claude/skills/dws") {
		t.Fatalf("expected .claude/skills/dws path, got %s", got[0])
	}
}

func TestResolveSkillSetupTargetsUnknown(t *testing.T) {
	if _, err := resolveSkillSetupTargets("nonsense"); err == nil {
		t.Fatalf("expected error for unknown target")
	}
}

func TestInstallSkillToHomesEndToEnd(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "references", "x.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst1 := filepath.Join(t.TempDir(), "a", "dws")
	dst2 := filepath.Join(t.TempDir(), "b", "dws")

	var stdout, stderr bytes.Buffer
	installed, skipped, err := installSkillToHomes(src, []string{dst1, dst2}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("install err: %v", err)
	}
	if installed != 2 || skipped != 0 {
		t.Fatalf("expected installed=2 skipped=0, got %d/%d", installed, skipped)
	}
	for _, d := range []string{dst1, dst2} {
		if _, err := os.Stat(filepath.Join(d, "SKILL.md")); err != nil {
			t.Fatalf("missing SKILL.md in %s: %v", d, err)
		}
		if _, err := os.Stat(filepath.Join(d, "references", "x.md")); err != nil {
			t.Fatalf("missing references/x.md in %s: %v", d, err)
		}
	}
}
