package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	setTestHome(t, t.TempDir())
	_, err := resolveSkillSetupSource(tmp, skillSetupModeMono)
	if err == nil {
		t.Fatalf("expected error when source missing")
	}
	if !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("expected 未找到 message, got %v", err)
	}
}

func TestResolveSkillSetupTargetsSingleAgent(t *testing.T) {
	home := t.TempDir()
	originalHome := skillSetupUserHomeDir
	skillSetupUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillSetupUserHomeDir = originalHome })
	got, err := resolveSkillSetupTargets("claude", skillSetupModeMono)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 dest, got %d", len(got))
	}
	want := filepath.Join(home, ".claude", "skills", "dws")
	if filepath.Clean(got[0]) != filepath.Clean(want) {
		t.Fatalf("expected %s, got %s", want, got[0])
	}
}

func TestResolveSkillSetupTargetsUnknown(t *testing.T) {
	if _, err := resolveSkillSetupTargets("nonsense", skillSetupModeMono); err == nil {
		t.Fatalf("expected error for unknown target")
	}
}

func TestResolveSkillSetupTargetsMultiOmitsDwsTail(t *testing.T) {
	home := t.TempDir()
	originalHome := skillSetupUserHomeDir
	skillSetupUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillSetupUserHomeDir = originalHome })
	got, err := resolveSkillSetupTargets("claude", skillSetupModeMulti)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 dest, got %d", len(got))
	}
	want := filepath.Join(home, ".claude", "skills")
	if filepath.Clean(got[0]) != filepath.Clean(want) {
		t.Fatalf("expected %s, got %s", want, got[0])
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
		skillBody := "---\nname: " + n + "\ndescription: test skill\n---\n\n# " + n + "\n"
		if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "references", n+".md"), []byte("ref "+n), 0o644); err != nil {
			t.Fatal(err)
		}
		if n == multiEventSkill {
			for _, ref := range eventMigrationRequiredReferences {
				if err := os.WriteFile(filepath.Join(sub, "references", ref), []byte("ref "+ref+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
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

func executeMultiSkillSetupTest(t *testing.T, src string, dests []string, args ...string) (string, string, error) {
	t.Helper()
	originalTargets := skillSetupResolveTargets
	skillSetupResolveTargets = func(string, string) ([]string, error) {
		return append([]string(nil), dests...), nil
	}
	t.Cleanup(func() { skillSetupResolveTargets = originalTargets })

	cmd := newSkillSetupCommand()
	cmd.Flags().Bool("dry-run", false, "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	baseArgs := []string{"--mode", "multi", "--source", src}
	cmd.SetArgs(append(baseArgs, args...))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeFoldedEventMisc(t *testing.T, agentHome string) {
	t.Helper()
	miscRoot := filepath.Join(agentHome, multiMiscSkill)
	if err := os.MkdirAll(filepath.Join(miscRoot, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(miscRoot, "SKILL.md"), []byte("personal IM route: dws event consume\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(miscRoot, "references", "event.md"), []byte("folded event docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeOldStandaloneEvent(t *testing.T, agentHome string) {
	t.Helper()
	eventRoot := filepath.Join(agentHome, multiEventSkill)
	if err := os.MkdirAll(eventRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eventRoot, "SKILL.md"), []byte("old standalone event\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertOldEventMiscPair(t *testing.T, agentHome string) {
	t.Helper()
	eventBody, err := os.ReadFile(filepath.Join(agentHome, multiEventSkill, "SKILL.md"))
	if err != nil || string(eventBody) != "old standalone event\n" {
		t.Fatalf("old standalone event was not restored: body=%q err=%v", eventBody, err)
	}
	miscBody, err := os.ReadFile(filepath.Join(agentHome, multiMiscSkill, "SKILL.md"))
	if err != nil || !strings.Contains(string(miscBody), "dws event") {
		t.Fatalf("folded misc was not restored: body=%q err=%v", miscBody, err)
	}
	if _, err := os.Stat(filepath.Join(agentHome, multiMiscSkill, "references", "event.md")); err != nil {
		t.Fatalf("folded Event reference was not restored: %v", err)
	}
}

func assertNoEventMigrationStages(t *testing.T, agentHome string) {
	t.Helper()
	entries, err := os.ReadDir(agentHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".dws-event-migration-") {
			t.Fatalf("unexpected leftover Event migration stage %s", filepath.Join(agentHome, entry.Name()))
		}
	}
}

func TestSkillSetupSelectiveEventMigratesOnlyFoldedTargets(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc",
	})
	foldedHome := filepath.Join(t.TempDir(), "folded", "skills")
	freshHome := filepath.Join(t.TempDir(), "fresh", "skills")
	writeFoldedEventMisc(t, foldedHome)
	if err := os.MkdirAll(filepath.Join(foldedHome, multiEventSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foldedHome, multiEventSkill, "SKILL.md"), []byte("old standalone event\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(foldedHome, "dingtalk-chat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foldedHome, "dingtalk-chat", "SKILL.md"), []byte("keep sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{freshHome, foldedHome}, "--skill", "event")
	if err != nil {
		t.Fatalf("selective event setup failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "迁移伴侣") || !strings.Contains(stdout, foldedHome) {
		t.Fatalf("confirmation output should expose folded misc migration: %s", stdout)
	}
	if !strings.Contains(stdout, "重新加载 Skills") {
		t.Fatalf("completion should tell the user to reload skills: %s", stdout)
	}

	for _, home := range []string{freshHome, foldedHome} {
		for _, name := range []string{multiSharedSkill, multiEventSkill} {
			if _, err := os.Stat(filepath.Join(home, name, "SKILL.md")); err != nil {
				t.Errorf("%s missing from %s: %v", name, home, err)
			}
		}
		if _, err := os.Stat(filepath.Join(home, "dingtalk-doc")); !os.IsNotExist(err) {
			t.Errorf("unselected doc appeared in %s: %v", home, err)
		}
	}
	if _, err := os.Stat(filepath.Join(freshHome, multiMiscSkill)); !os.IsNotExist(err) {
		t.Fatalf("fresh selective target must not receive misc, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(foldedHome, multiMiscSkill, "references", "event.md")); !os.IsNotExist(err) {
		t.Fatalf("folded event reference survived clean misc replacement, stat err=%v", err)
	}
	eventBody, err := os.ReadFile(filepath.Join(foldedHome, multiEventSkill, "SKILL.md"))
	if err != nil || strings.Contains(string(eventBody), "old standalone") {
		t.Fatalf("old standalone event was not replaced: body=%q err=%v", eventBody, err)
	}
	siblingBody, err := os.ReadFile(filepath.Join(foldedHome, "dingtalk-chat", "SKILL.md"))
	if err != nil || string(siblingBody) != "keep sibling\n" {
		t.Fatalf("unrelated sibling changed: body=%q err=%v", siblingBody, err)
	}

	// A second selective run sees the already-clean misc, does not plan another
	// migration, and leaves that unselected sibling in place.
	stdout, stderr, err = executeMultiSkillSetupTest(t, src, []string{freshHome, foldedHome}, "--skill", "event", "--yes")
	if err != nil {
		t.Fatalf("idempotent event setup failed: %v\nstderr=%s", err, stderr)
	}
	if strings.Contains(stdout, "迁移伴侣") {
		t.Fatalf("clean second run should not re-detect folded misc: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(foldedHome, multiMiscSkill, "SKILL.md")); err != nil {
		t.Fatalf("second selective run removed clean misc: %v", err)
	}
}

func TestSkillSetupEventMigrationDryRunAndExplicitExclude(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc",
	})

	t.Run("dry run reports companion without writes", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "skills")
		writeFoldedEventMisc(t, home)
		stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--dry-run", "--yes")
		if err != nil {
			t.Fatalf("dry run failed: %v\nstderr=%s", err, stderr)
		}
		if !strings.Contains(stdout, "DRY-RUN") || !strings.Contains(stdout, "迁移伴侣") {
			t.Fatalf("dry run did not expose migration: %s", stdout)
		}
		body, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
		if readErr != nil || !strings.Contains(string(body), "dws event") {
			t.Fatalf("dry run changed folded misc: body=%q err=%v", body, readErr)
		}
		if _, statErr := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(statErr) {
			t.Fatalf("dry run installed event, stat err=%v", statErr)
		}
	})

	t.Run("excluding required misc fails before writes", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "skills")
		writeFoldedEventMisc(t, home)
		_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--exclude", "misc", "--yes")
		if err == nil || !strings.Contains(err.Error(), "不能显式 --exclude misc") {
			t.Fatalf("expected clear migration exclusion error, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(statErr) {
			t.Fatalf("failed migration installed event, stat err=%v", statErr)
		}
		body, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
		if readErr != nil || !strings.Contains(string(body), "dws event") {
			t.Fatalf("failed migration changed misc: body=%q err=%v", body, readErr)
		}
	})
}

func TestSkillSetupEventMigrationRequiresCleanMiscInSource(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)

	_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "当前 multi 源缺少") {
		t.Fatalf("expected missing migration companion error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(statErr) {
		t.Fatalf("failed preflight installed event, stat err=%v", statErr)
	}
}

func TestSkillSetupEventMigrationAcceptsShippedMultiBundle(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Clean(filepath.Join(wd, "..", "..", "skills", "multi"))
	if err := validateEventMiscMigrationSource(src); err != nil {
		t.Fatalf("shipped multi bundle is not a valid Event migration source: %v", err)
	}
}

func TestSkillSetupEventMigrationRejectsInvalidSkillBundlesBeforeWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, src string)
	}{
		{
			name: "empty event root",
			mutate: func(t *testing.T, src string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(src, multiEventSkill, "SKILL.md"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong event name",
			mutate: func(t *testing.T, src string) {
				t.Helper()
				body := "---\nname: dingtalk-chat\ndescription: wrong skill\n---\n\n# Wrong\n"
				if err := os.WriteFile(filepath.Join(src, multiEventSkill, "SKILL.md"), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing event reference",
			mutate: func(t *testing.T, src string) {
				t.Helper()
				if err := os.Remove(filepath.Join(src, multiEventSkill, "references", "event-oa.md")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "empty event reference",
			mutate: func(t *testing.T, src string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(src, multiEventSkill, "references", "event-im.md"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong misc name",
			mutate: func(t *testing.T, src string) {
				t.Helper()
				body := "---\nname: dingtalk-event\ndescription: wrong skill\n---\n\n# Wrong\n"
				if err := os.WriteFile(filepath.Join(src, multiMiscSkill, "SKILL.md"), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
			test.mutate(t, src)
			home := filepath.Join(t.TempDir(), "skills")
			writeFoldedEventMisc(t, home)
			writeOldStandaloneEvent(t, home)

			stdout, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
			if err == nil || !strings.Contains(err.Error(), "迁移源无效") {
				t.Fatalf("invalid migration source was accepted: %v", err)
			}
			if strings.Contains(stdout, "Skill 安装完成") {
				t.Fatalf("invalid migration source reported success: %s", stdout)
			}
			assertOldEventMiscPair(t, home)
			if _, statErr := os.Stat(filepath.Join(home, multiSharedSkill)); !os.IsNotExist(statErr) {
				t.Fatalf("invalid source wrote shared skill: %v", statErr)
			}
		})
	}
}

func TestSkillSetupSelectiveEventPreservesFoldedMiscAfterPrimarySkip(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)

	originalInstallMulti := skillSetupInstallMulti
	t.Cleanup(func() { skillSetupInstallMulti = originalInstallMulti })
	calls := 0
	skillSetupInstallMulti = func(string, []string, []string, io.Writer, io.Writer) (int, int, error) {
		calls++
		if calls > 1 {
			t.Fatal("misc migration companion ran after a primary install skip")
		}
		return 1, 1, nil
	}

	stdout, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "已保留折叠版 Event/misc") {
		t.Fatalf("expected preserved-fallback error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("install calls = %d, want 1", calls)
	}
	if strings.Contains(stdout, "Skill 安装完成") {
		t.Fatalf("partial migration reported success: %s", stdout)
	}
	body, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(body), "dws event") {
		t.Fatalf("primary skip changed folded misc: body=%q err=%v", body, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(home, multiMiscSkill, "references", "event.md")); statErr != nil {
		t.Fatalf("primary skip removed folded Event reference: %v", statErr)
	}
}

func TestSkillSetupFreshTargetFailureDoesNotTouchFoldedPair(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	freshHome := filepath.Join(t.TempDir(), "fresh", "skills")
	foldedHome := filepath.Join(t.TempDir(), "folded", "skills")
	writeFoldedEventMisc(t, foldedHome)
	writeOldStandaloneEvent(t, foldedHome)

	originalInstallMulti := skillSetupInstallMulti
	t.Cleanup(func() { skillSetupInstallMulti = originalInstallMulti })
	calls := 0
	skillSetupInstallMulti = func(string, []string, []string, io.Writer, io.Writer) (int, int, error) {
		calls++
		if calls > 1 {
			t.Fatal("folded target prerequisites ran after fresh target failure")
		}
		return 1, 1, nil
	}

	stdout, _, err := executeMultiSkillSetupTest(
		t,
		src,
		[]string{foldedHome, freshHome},
		"--skill", "event",
		"--yes",
	)
	if err == nil || !strings.Contains(err.Error(), "已保留折叠版 Event/misc") {
		t.Fatalf("fresh target failure did not block migration: %v", err)
	}
	if strings.Contains(stdout, "Skill 安装完成") {
		t.Fatalf("partial mixed-target install reported success: %s", stdout)
	}
	assertOldEventMiscPair(t, foldedHome)
	assertNoEventMigrationStages(t, foldedHome)
}

func TestSkillSetupUnrelatedSelectiveInstallLeavesFoldedPairUntouched(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc",
	})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)
	writeOldStandaloneEvent(t, home)

	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "doc", "--yes")
	if err != nil {
		t.Fatalf("unrelated selective install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "Event Skill 迁移") || strings.Contains(stdout, "Event 原子迁移") {
		t.Fatalf("unrelated selective install planned Event migration: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(home, "dingtalk-doc", "SKILL.md")); err != nil {
		t.Fatalf("selected doc was not installed: %v", err)
	}
	assertOldEventMiscPair(t, home)
	assertNoEventMigrationStages(t, home)
}

func TestSkillSetupSelectiveEventAtomicStageFailurePreservesFoldedPair(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)

	originalCopyDir := skillSetupCopyDir
	t.Cleanup(func() { skillSetupCopyDir = originalCopyDir })
	skillSetupCopyDir = func(src, dest string) error {
		if strings.HasSuffix(dest, "new-misc") {
			return errors.New("injected stage failure")
		}
		return originalCopyDir(src, dest)
	}

	stdout, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "预备 dingtalk-misc 失败") {
		t.Fatalf("expected atomic staging error, got %v", err)
	}
	if strings.Contains(stdout, "Skill 安装完成") {
		t.Fatalf("partial atomic migration reported success: %s", stdout)
	}
	body, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(body), "dws event") {
		t.Fatalf("stage failure changed folded misc: body=%q err=%v", body, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(statErr) {
		t.Fatalf("stage failure installed standalone event, stat err=%v", statErr)
	}
}

func TestSkillSetupEventMigrationPreparationFailuresPreserveFoldedPair(t *testing.T) {
	t.Run("same-filesystem staging creation", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
		home := filepath.Join(t.TempDir(), "skills")
		writeFoldedEventMisc(t, home)
		writeOldStandaloneEvent(t, home)

		originalMkdirTemp := skillSetupMkdirTemp
		t.Cleanup(func() { skillSetupMkdirTemp = originalMkdirTemp })
		skillSetupMkdirTemp = func(dir, pattern string) (string, error) {
			if dir != home {
				t.Fatalf("staging dir = %s, want target filesystem root %s", dir, home)
			}
			return "", errors.New("injected mkdir-temp failure")
		}

		_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
		if err == nil || !strings.Contains(err.Error(), "injected mkdir-temp failure") {
			t.Fatalf("staging creation failure was not returned: %v", err)
		}
		assertOldEventMiscPair(t, home)
		assertNoEventMigrationStages(t, home)
	})

	t.Run("event staging copy", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
		home := filepath.Join(t.TempDir(), "skills")
		writeFoldedEventMisc(t, home)
		writeOldStandaloneEvent(t, home)

		originalCopyDir := skillSetupCopyDir
		t.Cleanup(func() { skillSetupCopyDir = originalCopyDir })
		skillSetupCopyDir = func(src, dest string) error {
			if strings.HasSuffix(dest, "new-event") {
				return errors.New("injected event copy failure")
			}
			return originalCopyDir(src, dest)
		}

		_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
		if err == nil || !strings.Contains(err.Error(), "injected event copy failure") {
			t.Fatalf("event staging failure was not returned: %v", err)
		}
		assertOldEventMiscPair(t, home)
		assertNoEventMigrationStages(t, home)
	})
}

func TestSkillSetupSelectiveEventRejectsCorruptStagedMisc(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)

	originalCopyDir := skillSetupCopyDir
	t.Cleanup(func() { skillSetupCopyDir = originalCopyDir })
	skillSetupCopyDir = func(src, dest string) error {
		if err := originalCopyDir(src, dest); err != nil {
			return err
		}
		if strings.HasSuffix(dest, "new-misc") {
			return os.WriteFile(filepath.Join(dest, "references", "event-partial.md"), []byte("corrupt\n"), 0o644)
		}
		return nil
	}

	_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "staging 验证失败") {
		t.Fatalf("corrupt staged misc was accepted: %v", err)
	}
	miscBody, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(miscBody), "dws event") {
		t.Fatalf("staging validation failure changed folded misc: body=%q err=%v", miscBody, readErr)
	}
	if _, err := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(err) {
		t.Fatalf("staging validation failure installed event: %v", err)
	}
	assertNoEventMigrationStages(t, home)
}

func TestSkillSetupSelectiveEventRejectsIncompleteStagedEvent(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)
	writeOldStandaloneEvent(t, home)

	originalCopyDir := skillSetupCopyDir
	t.Cleanup(func() { skillSetupCopyDir = originalCopyDir })
	skillSetupCopyDir = func(src, dest string) error {
		if err := originalCopyDir(src, dest); err != nil {
			return err
		}
		if strings.HasSuffix(dest, "new-event") {
			return os.Remove(filepath.Join(dest, "references", "event-oa.md"))
		}
		return nil
	}

	_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "staging 验证失败") {
		t.Fatalf("incomplete staged Event was accepted: %v", err)
	}
	assertOldEventMiscPair(t, home)
	assertNoEventMigrationStages(t, home)
}

func TestSkillSetupFullEventMigrationIsAtomicAndPreservesSiblings(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc",
	})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)
	writeOldStandaloneEvent(t, home)
	sibling := filepath.Join(home, "dingtalk-private-sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "SKILL.md"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{home}, "--yes")
	if err != nil {
		t.Fatalf("full setup migration failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Event 原子迁移") || !strings.Contains(stdout, "Skill 安装完成") {
		t.Fatalf("full setup did not report atomic migration success: %s", stdout)
	}
	for _, name := range []string{multiEventSkill, multiMiscSkill, multiSharedSkill, "dingtalk-doc"} {
		if _, err := os.Stat(filepath.Join(home, name, "SKILL.md")); err != nil {
			t.Fatalf("full setup missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, multiMiscSkill, "references", "event.md")); !os.IsNotExist(err) {
		t.Fatalf("full setup retained folded Event reference: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(sibling, "SKILL.md"))
	if err != nil || string(body) != "keep\n" {
		t.Fatalf("full setup changed unrelated sibling: body=%q err=%v", body, err)
	}
	assertNoEventMigrationStages(t, home)
}

func TestSkillSetupEventMigrationWithoutSharedStillCleansMonoLeftover(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)
	monoLeftover := filepath.Join(home, "dws")
	if err := os.MkdirAll(monoLeftover, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monoLeftover, "SKILL.md"), []byte("old mono\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err != nil {
		t.Fatalf("migration without shared failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(monoLeftover); !os.IsNotExist(err) {
		t.Fatalf("migration without prerequisites retained mono leftover: %v", err)
	}
	for _, name := range []string{multiEventSkill, multiMiscSkill} {
		if _, err := os.Stat(filepath.Join(home, name, "SKILL.md")); err != nil {
			t.Fatalf("migration without shared missing %s: %v", name, err)
		}
	}
	assertNoEventMigrationStages(t, home)
}

func TestSkillSetupFoldedEventMigrationSelectionPreflight(t *testing.T) {
	src := writeMultiSkillSource(t, []string{
		multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc",
	})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "misc only", args: []string{"--skill", "misc", "--yes"}, want: "不能只覆盖 dingtalk-misc"},
		{name: "explicitly excludes event", args: []string{"--exclude", "event", "--yes"}, want: "不能显式 --exclude event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "skills")
			writeFoldedEventMisc(t, home)
			_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("preflight error = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(home, multiSharedSkill)); !os.IsNotExist(err) {
				t.Fatalf("preflight failure wrote shared skill: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, multiEventSkill)); !os.IsNotExist(err) {
				t.Fatalf("preflight failure wrote event skill: %v", err)
			}
			miscBody, readErr := os.ReadFile(filepath.Join(home, multiMiscSkill, "SKILL.md"))
			if readErr != nil || !strings.Contains(string(miscBody), "dws event") {
				t.Fatalf("preflight failure changed folded misc: body=%q err=%v", miscBody, readErr)
			}
		})
	}
}

func TestSkillSetupEventMigrationRejectsEveryFoldedReferenceVariant(t *testing.T) {
	for _, filename := range []string{"event.md", "event-im.md", "event-oa.md", "EVENT-legacy.MD"} {
		t.Run(filename, func(t *testing.T) {
			src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
			if err := os.WriteFile(filepath.Join(src, multiMiscSkill, "references", filename), []byte("stale\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(t.TempDir(), "skills")
			writeFoldedEventMisc(t, home)

			_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
			if err == nil || !strings.Contains(err.Error(), "仍存在折叠 Event 参考页") {
				t.Fatalf("source with %s was accepted: %v", filename, err)
			}
			if _, err := os.Stat(filepath.Join(home, multiSharedSkill)); !os.IsNotExist(err) {
				t.Fatalf("invalid source wrote shared skill: %v", err)
			}
		})
	}
}

func TestSkillSetupEventMigrationRenameFailuresRollbackPair(t *testing.T) {
	for failAt := 1; failAt <= 4; failAt++ {
		t.Run(fmt.Sprintf("rename_%d", failAt), func(t *testing.T) {
			src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
			home := filepath.Join(t.TempDir(), "skills")
			writeFoldedEventMisc(t, home)
			writeOldStandaloneEvent(t, home)

			originalRename := skillSetupRename
			t.Cleanup(func() { skillSetupRename = originalRename })
			renameCalls := 0
			skillSetupRename = func(oldPath, newPath string) error {
				renameCalls++
				if renameCalls == failAt {
					return errors.New("injected rename failure")
				}
				return originalRename(oldPath, newPath)
			}

			stdout, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
			if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
				t.Fatalf("rename failure %d was not returned: %v", failAt, err)
			}
			if strings.Contains(stdout, "Skill 安装完成") {
				t.Fatalf("rename failure %d reported success: %s", failAt, stdout)
			}
			assertOldEventMiscPair(t, home)
			assertNoEventMigrationStages(t, home)
		})
	}
}

func TestSkillSetupEventMigrationFailureRollsBackEarlierTargets(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	root := t.TempDir()
	firstHome := filepath.Join(root, "a", "skills")
	secondHome := filepath.Join(root, "b", "skills")
	for _, home := range []string{firstHome, secondHome} {
		writeFoldedEventMisc(t, home)
		writeOldStandaloneEvent(t, home)
	}

	originalRename := skillSetupRename
	t.Cleanup(func() { skillSetupRename = originalRename })
	failed := false
	skillSetupRename = func(oldPath, newPath string) error {
		if !failed && oldPath == filepath.Join(secondHome, multiMiscSkill) {
			failed = true
			return errors.New("second target failure")
		}
		return originalRename(oldPath, newPath)
	}

	_, _, err := executeMultiSkillSetupTest(t, src, []string{secondHome, firstHome}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "second target failure") {
		t.Fatalf("second target failure was not returned: %v", err)
	}
	for _, home := range []string{firstHome, secondHome} {
		assertOldEventMiscPair(t, home)
		assertNoEventMigrationStages(t, home)
	}
}

func TestSkillSetupEventMigrationRollbackFailurePreservesRecoveryDirectory(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)
	writeOldStandaloneEvent(t, home)

	originalRename := skillSetupRename
	t.Cleanup(func() { skillSetupRename = originalRename })
	skillSetupRename = func(oldPath, newPath string) error {
		if strings.HasSuffix(oldPath, filepath.Join("new-misc")) && newPath == filepath.Join(home, multiMiscSkill) {
			return errors.New("commit failure")
		}
		if strings.HasSuffix(oldPath, filepath.Join("old-misc")) && newPath == filepath.Join(home, multiMiscSkill) {
			return errors.New("rollback restore failure")
		}
		return originalRename(oldPath, newPath)
	}

	_, stderr, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "回滚不完整") || !strings.Contains(err.Error(), "恢复目录") {
		t.Fatalf("rollback failure did not expose recovery directory: %v", err)
	}
	entries, readErr := os.ReadDir(home)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var recoveryRoot string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".dws-event-migration-") {
			recoveryRoot = filepath.Join(home, entry.Name())
			break
		}
	}
	if recoveryRoot == "" {
		t.Fatal("rollback failure deleted the only recovery directory")
	}
	if !strings.Contains(err.Error(), recoveryRoot) || !strings.Contains(stderr, recoveryRoot) {
		t.Fatalf("recovery directory was not reported: err=%v stderr=%s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(recoveryRoot, "old-misc", "SKILL.md")); err != nil {
		t.Fatalf("old folded misc backup is missing from recovery directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(recoveryRoot, "old-event", "SKILL.md")); err != nil {
		t.Fatalf("old standalone Event backup is missing from recovery directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, multiEventSkill, "SKILL.md")); err != nil {
		t.Fatalf("rollback failure removed the live standalone Event entry: %v", err)
	}
}

func TestSkillSetupEventMigrationRollbackFailureKeepsNewEventWithoutOldStandalone(t *testing.T) {
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
	home := filepath.Join(t.TempDir(), "skills")
	writeFoldedEventMisc(t, home)

	originalRename := skillSetupRename
	t.Cleanup(func() { skillSetupRename = originalRename })
	skillSetupRename = func(oldPath, newPath string) error {
		if strings.HasSuffix(oldPath, filepath.Join("new-misc")) && newPath == filepath.Join(home, multiMiscSkill) {
			return errors.New("commit failure")
		}
		if strings.HasSuffix(oldPath, filepath.Join("old-misc")) && newPath == filepath.Join(home, multiMiscSkill) {
			return errors.New("rollback restore failure")
		}
		return originalRename(oldPath, newPath)
	}

	_, _, err := executeMultiSkillSetupTest(t, src, []string{home}, "--skill", "event", "--yes")
	if err == nil || !strings.Contains(err.Error(), "回滚不完整") {
		t.Fatalf("rollback failure was not returned: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, multiEventSkill, "SKILL.md")); statErr != nil {
		t.Fatalf("rollback failure removed the only live Event entry: %v", statErr)
	}
	entries, readErr := os.ReadDir(home)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".dws-event-migration-") {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(home, entry.Name(), "old-misc", "SKILL.md")); statErr != nil {
			t.Fatalf("rollback failure lost the folded misc recovery copy: %v", statErr)
		}
		return
	}
	t.Fatal("rollback failure did not preserve a recovery directory")
}
