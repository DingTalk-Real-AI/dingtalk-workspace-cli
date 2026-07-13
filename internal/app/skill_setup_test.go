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

func TestIsCharDeviceRejectsNilAndRegularFiles(t *testing.T) {
	if isCharDevice(nil) {
		t.Fatal("nil file must not be treated as interactive")
	}

	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if isCharDevice(file) {
		t.Fatal("regular files must not be treated as interactive terminals")
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
	// Isolate HOME so the ~/.dws/skills/<mode>/ fallback (added by the release
	// pipeline cache work) does not pick up real cached content on the
	// developer machine.
	t.Setenv("HOME", t.TempDir())
	_, err := resolveSkillSetupSource(tmp, skillSetupModeMono)
	if err == nil {
		t.Fatalf("expected error when source missing")
	}
	if !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("expected 未找到 message, got %v", err)
	}
}

func TestResolveSkillSetupTargetsSingleAgent(t *testing.T) {
	got, err := resolveSkillSetupTargets("claude", skillSetupModeMono)
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
	if _, err := resolveSkillSetupTargets("nonsense", skillSetupModeMono); err == nil {
		t.Fatalf("expected error for unknown target")
	}
}

func TestResolveSkillSetupTargetsMultiOmitsDwsTail(t *testing.T) {
	got, err := resolveSkillSetupTargets("claude", skillSetupModeMulti)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 dest, got %d", len(got))
	}
	if strings.HasSuffix(got[0], "/dws") {
		t.Fatalf("multi target must not end with /dws, got %s", got[0])
	}
	if !strings.HasSuffix(got[0], ".claude/skills") {
		t.Fatalf("expected suffix .claude/skills, got %s", got[0])
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

// writeMultiSkillSource builds a fake skills/multi/ layout containing N
// dingtalk-* subdirs, each with a SKILL.md and one references/<name>.md
// file. Returns the absolute skill source root.
func writeMultiSkillSource(t *testing.T, names []string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		sub := filepath.Join(root, n)
		if err := os.MkdirAll(filepath.Join(sub, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte("# "+n), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "references", n+".md"), []byte("ref "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestInstallMultiSkillToHomes(t *testing.T) {
	names := []string{"dingtalk-aitable", "dingtalk-calendar", "dingtalk-doc"}
	src := writeMultiSkillSource(t, names)

	got, err := listMultiSkillNames(src)
	if err != nil {
		t.Fatalf("listMultiSkillNames err: %v", err)
	}
	if len(got) != len(names) {
		t.Fatalf("expected %d skills, got %d (%v)", len(names), len(got), got)
	}

	dst1 := filepath.Join(t.TempDir(), ".claude", "skills")
	dst2 := filepath.Join(t.TempDir(), ".cursor", "skills")

	var stdout, stderr bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(src, got, []string{dst1, dst2}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("installMultiSkillToHomes err: %v", err)
	}
	if installed != len(names)*2 || skipped != 0 {
		t.Fatalf("expected installed=%d skipped=0, got %d/%d (stderr=%q)", len(names)*2, installed, skipped, stderr.String())
	}
	for _, d := range []string{dst1, dst2} {
		for _, n := range names {
			sub := filepath.Join(d, n)
			if _, err := os.Stat(filepath.Join(sub, "SKILL.md")); err != nil {
				t.Fatalf("missing %s/SKILL.md: %v", sub, err)
			}
			if _, err := os.Stat(filepath.Join(sub, "references", n+".md")); err != nil {
				t.Fatalf("missing %s/references/%s.md: %v", sub, n, err)
			}
		}
		// dws/ should NOT exist (multi mode is pure siblings)
		if _, err := os.Stat(filepath.Join(d, "dws")); err == nil {
			t.Fatalf("unexpected dws/ subdir in multi-mode install at %s", d)
		}
	}
}

func TestSkillSetupMutualExclusion(t *testing.T) {
	names := []string{"dingtalk-aitable", "dingtalk-calendar"}
	src := writeMultiSkillSource(t, names)

	// Simulate a pre-existing mono install under <agent-home>/dws/
	agentHome := filepath.Join(t.TempDir(), ".claude", "skills")
	monoLeftover := filepath.Join(agentHome, "dws")
	if err := os.MkdirAll(filepath.Join(monoLeftover, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monoLeftover, "SKILL.md"), []byte("old mono"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sanity: leftover exists before
	if _, err := os.Stat(monoLeftover); err != nil {
		t.Fatalf("setup: mono leftover should exist before, err=%v", err)
	}

	// Confirm mutualExclusionVictims sees the leftover
	victims := mutualExclusionVictims(agentHome, skillSetupModeMulti)
	if len(victims) != 1 || victims[0] != monoLeftover {
		t.Fatalf("expected victims=[%s], got %v", monoLeftover, victims)
	}

	var stdout, stderr bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(src, names, []string{agentHome}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("install err: %v (stderr=%s)", err, stderr.String())
	}
	if installed != len(names) || skipped != 0 {
		t.Fatalf("expected installed=%d skipped=0, got %d/%d", len(names), installed, skipped)
	}

	// mono leftover should be gone
	if _, err := os.Stat(monoLeftover); !os.IsNotExist(err) {
		t.Fatalf("expected mono leftover removed, stat err=%v", err)
	}
	// multi skills should be in place
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(agentHome, n, "SKILL.md")); err != nil {
			t.Fatalf("missing %s/%s/SKILL.md: %v", agentHome, n, err)
		}
	}
	// the cleanup line should appear in stdout (best-effort observability)
	if !strings.Contains(stdout.String(), "已清理对面模式残留") {
		t.Fatalf("expected cleanup log line, got stdout=%q", stdout.String())
	}

	// Now test the reverse: pre-existing multi → installing mono cleans dingtalk-*
	monoSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(monoSrc, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	monoDest := filepath.Join(agentHome, "dws")
	stdout.Reset()
	stderr.Reset()
	installed2, skipped2, err := installSkillToHomes(monoSrc, []string{monoDest}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("mono install err: %v", err)
	}
	if installed2 != 1 || skipped2 != 0 {
		t.Fatalf("expected mono installed=1 skipped=0, got %d/%d", installed2, skipped2)
	}
	// All dingtalk-* siblings should be gone after mono install
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(agentHome, n)); !os.IsNotExist(err) {
			t.Fatalf("expected %s removed by mutual exclusion, stat err=%v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(monoDest, "SKILL.md")); err != nil {
		t.Fatalf("mono SKILL.md missing: %v", err)
	}
	if !strings.Contains(stdout.String(), "已清理对面模式残留") {
		t.Fatalf("expected cleanup log line on mono install, got stdout=%q", stdout.String())
	}
}

// TestSkillSourceCandidatesIncludesUserCache verifies that the user-level
// cache populated by install.sh / install.ps1 / npm install.js is part of the
// fallback candidate list, so `dws skill setup` can find a source on a fresh
// machine without --source.
func TestSkillSourceCandidatesIncludesUserCache(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error = %v", err)
	}

	for _, subdir := range []string{"mono", "multi"} {
		got := skillSourceCandidates("", subdir)
		want := filepath.Join(home, ".dws", "skills", subdir)
		found := false
		for _, c := range got {
			if c == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("skillSourceCandidates(%q) missing %q; got %v", subdir, want, got)
		}
	}
}

// TestResolveSkillSetupSourceFallsBackToUserCache verifies that when no
// --source / DWS_SKILL_SOURCE / source checkout is available, the resolver
func TestNormalizeMultiSkillName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"aitable", "dingtalk-aitable"},
		{"dingtalk-aitable", "dingtalk-aitable"},
		{"  Calendar  ", "dingtalk-calendar"},
		{"DINGTALK-DOC", "dingtalk-doc"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := normalizeMultiSkillName(c.in); got != c.want {
			t.Errorf("normalizeMultiSkillName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFilterMultiSkillNames(t *testing.T) {
	all := []string{"dingtalk-aitable", "dingtalk-calendar", "dingtalk-doc", "dingtalk-live"}

	t.Run("no filter returns all", func(t *testing.T) {
		got, err := filterMultiSkillNames(all, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(all) {
			t.Fatalf("expected %d, got %v", len(all), got)
		}
	})

	t.Run("include short names", func(t *testing.T) {
		got, err := filterMultiSkillNames(all, []string{"aitable", "calendar"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != "dingtalk-aitable,dingtalk-calendar" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("include full names", func(t *testing.T) {
		got, err := filterMultiSkillNames(all, []string{"dingtalk-doc"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "dingtalk-doc" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("include dedups", func(t *testing.T) {
		got, err := filterMultiSkillNames(all, []string{"aitable", "dingtalk-aitable", "AITABLE"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "dingtalk-aitable" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("include unknown errors with available list", func(t *testing.T) {
		_, err := filterMultiSkillNames(all, []string{"aitable", "bogus"}, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "bogus") {
			t.Errorf("error should mention bad name, got: %s", msg)
		}
		if !strings.Contains(msg, "dingtalk-calendar") {
			t.Errorf("error should list available names, got: %s", msg)
		}
	})

	t.Run("exclude short names", func(t *testing.T) {
		got, err := filterMultiSkillNames(all, nil, []string{"live", "doc"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != "dingtalk-aitable,dingtalk-calendar" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("exclude unknown errors", func(t *testing.T) {
		_, err := filterMultiSkillNames(all, nil, []string{"bogus"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("exclude all errors", func(t *testing.T) {
		_, err := filterMultiSkillNames(all, nil, []string{"aitable", "calendar", "doc", "live"})
		if err == nil {
			t.Fatal("expected error when exclude drops everything")
		}
		if !strings.Contains(err.Error(), "全部") {
			t.Errorf("expected 全部 in error, got: %s", err.Error())
		}
	})

	t.Run("include + exclude mutually exclusive", func(t *testing.T) {
		_, err := filterMultiSkillNames(all, []string{"aitable"}, []string{"doc"})
		if err == nil {
			t.Fatal("expected error when both given")
		}
	})
}

// TestSkillSetupMultiAdditivePreservesSiblings verifies the key UX promise of
// `dws skill setup --mode multi -s aitable`: installing a subset must NOT
// touch already-installed dingtalk-* siblings (additive semantics).
func TestSkillSetupMultiAdditivePreservesSiblings(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		"dingtalk-aitable", "dingtalk-calendar", "dingtalk-doc",
	})
	agentHome := filepath.Join(t.TempDir(), ".claude", "skills")

	// Pretend the user already installed two dingtalk-* skills earlier.
	preExisting := []string{"dingtalk-chat", "dingtalk-todo"}
	for _, n := range preExisting {
		dir := filepath.Join(agentHome, n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("OLD "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// User now runs `... --mode multi -s aitable -s calendar`.
	filtered, err := filterMultiSkillNames(
		[]string{"dingtalk-aitable", "dingtalk-calendar", "dingtalk-doc"},
		[]string{"aitable", "calendar"},
		nil,
	)
	if err != nil {
		t.Fatalf("filter err: %v", err)
	}

	var stdout, stderr bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(src, filtered, []string{agentHome}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("install err: %v (stderr=%s)", err, stderr.String())
	}
	if installed != 2 || skipped != 0 {
		t.Fatalf("expected installed=2 skipped=0, got %d/%d", installed, skipped)
	}

	// Asked-for skills should be in place.
	for _, n := range []string{"dingtalk-aitable", "dingtalk-calendar"} {
		if _, err := os.Stat(filepath.Join(agentHome, n, "SKILL.md")); err != nil {
			t.Errorf("missing newly-installed %s: %v", n, err)
		}
	}
	// Unselected source skill must NOT be installed.
	if _, err := os.Stat(filepath.Join(agentHome, "dingtalk-doc")); !os.IsNotExist(err) {
		t.Errorf("dingtalk-doc was not requested but appeared (stat err=%v)", err)
	}
	// Pre-existing sibling skills must be UNTOUCHED — additive semantics.
	for _, n := range preExisting {
		body, err := os.ReadFile(filepath.Join(agentHome, n, "SKILL.md"))
		if err != nil {
			t.Errorf("pre-existing %s was wiped (err=%v)", n, err)
			continue
		}
		if !strings.HasPrefix(string(body), "OLD ") {
			t.Errorf("pre-existing %s content changed: got %q", n, string(body))
		}
	}
}

// TestRunSkillSetupRejectsSkillFlagInMonoMode verifies that the new
// -s/--skill and -x/--exclude flags are gated on --mode multi.
func TestRunSkillSetupRejectsSkillFlagInMonoMode(t *testing.T) {
	cmd := newSkillSetupCommand()
	cmd.SetArgs([]string{"--mode", "mono", "--yes", "--skill", "aitable"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --skill in mono mode")
	}
	if !strings.Contains(err.Error(), "multi") {
		t.Fatalf("error should mention multi gating, got: %v", err)
	}
}

func TestResolveSkillSetupSourceMultiFinds(t *testing.T) {
	tmp := t.TempDir()
	multiDir := filepath.Join(tmp, "skills", "multi")
	for _, n := range []string{"dingtalk-aitable", "dingtalk-doc"} {
		if err := os.MkdirAll(filepath.Join(multiDir, n), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(multiDir, n, "SKILL.md"), []byte("# "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := resolveSkillSetupSource(tmp, skillSetupModeMulti)
	if err != nil {
		t.Fatalf("expected to find multi source, got err=%v", err)
	}
	if got != multiDir {
		t.Fatalf("expected %s, got %s", multiDir, got)
	}
}
