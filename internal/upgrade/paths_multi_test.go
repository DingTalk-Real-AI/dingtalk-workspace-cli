// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// withFakeHome swaps upgradeUserHomeDir to a temp dir for the test duration.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home, nil })
	return home
}

func useUpgradeManagedNames(t *testing.T, names ...string) {
	t.Helper()
	records := make([]skillprovenance.Record, 0, len(names))
	for _, name := range names {
		records = append(records, skillprovenance.Record{Name: name})
	}
	testseam.Swap(t, &upgradeReadSkillState, func(string) (*skillstate.State, bool, error) {
		return &skillstate.State{ManagedSkills: records}, true, nil
	})
}

// writeMultiBundle creates a multi-skill bundle root with the given skills.
func writeMultiBundle(t *testing.T, root string, skills ...string) string {
	t.Helper()
	multi := filepath.Join(root, "multi")
	for _, name := range skills {
		dir := filepath.Join(multi, name)
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "references", "guide.md"), []byte("guide "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return multi
}

func TestCrossPlatformCoverageLocateSkillsRootPrefersMulti(t *testing.T) {
	extract := t.TempDir()
	// Legacy mono copy at zip root plus the multi bundle: multi must win.
	os.WriteFile(filepath.Join(extract, "SKILL.md"), []byte("# mono"), 0o644)
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat", "dws-shared")

	got := LocateSkillsRoot(extract)
	if got != multiRoot {
		t.Errorf("LocateSkillsRoot() = %q, want multi root %q", got, multiRoot)
	}
}

func TestCrossPlatformCoverageLocateSkillsRootFallsBackToMono(t *testing.T) {
	extract := t.TempDir()
	os.WriteFile(filepath.Join(extract, "SKILL.md"), []byte("# mono"), 0o644)

	got := LocateSkillsRoot(extract)
	if got != extract {
		t.Errorf("LocateSkillsRoot() = %q, want mono flat root %q", got, extract)
	}
}

func TestCrossPlatformCoverageSwapUserHomeDirForTest(t *testing.T) {
	want := t.TempDir()
	SwapUserHomeDirForTest(t, func() (string, error) { return want, nil })

	got, err := upgradeUserHomeDir()
	if err != nil {
		t.Fatalf("upgradeUserHomeDir() error = %v", err)
	}
	if got != want {
		t.Fatalf("upgradeUserHomeDir() = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoverageBundleSkillNamesLayouts(t *testing.T) {
	// Mono layout (top-level SKILL.md) is not a bundle.
	mono := t.TempDir()
	os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644)
	os.MkdirAll(filepath.Join(mono, "references"), 0o755)
	if got := bundleSkillNames(mono); got != nil {
		t.Errorf("mono layout: bundleSkillNames() = %v, want nil", got)
	}

	// Multi bundle: sorted subdir names containing SKILL.md.
	extract := t.TempDir()
	multi := writeMultiBundle(t, extract, "dingtalk-wiki", "dingtalk-chat", "dws-shared")
	want := []string{"dingtalk-chat", "dingtalk-wiki", "dws-shared"}
	if got := bundleSkillNames(multi); !reflect.DeepEqual(got, want) {
		t.Errorf("multi layout: bundleSkillNames() = %v, want %v", got, want)
	}

	// Regular files inside the bundle root are ignored.
	if err := os.WriteFile(filepath.Join(multi, "README.md"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := bundleSkillNames(multi); !reflect.DeepEqual(got, want) {
		t.Errorf("bundle with file entry: bundleSkillNames() = %v, want %v", got, want)
	}

	// Missing directory.
	if got := bundleSkillNames(filepath.Join(extract, "nope")); got != nil {
		t.Errorf("missing dir: bundleSkillNames() = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageUpgradeSkillLocationsMulti(t *testing.T) {
	home := withFakeHome(t)

	// A concrete Agent root wins over the generic .agents fallback; .claude
	// installs and .cursor is skipped.
	agentsBase := filepath.Join(home, ".agents", "skills")
	claudeBase := filepath.Join(home, ".claude", "skills")
	for _, base := range []string{agentsBase, claudeBase} {
		if err := os.MkdirAll(base, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Pre-existing state in both homes: mono leftover, a stale dingtalk skill
	// no longer in the bundle, and a non-DWS dir that must survive.
	for _, base := range []string{agentsBase, claudeBase} {
		os.MkdirAll(filepath.Join(base, "dws"), 0o755)
		os.WriteFile(filepath.Join(base, "dws", "SKILL.md"), []byte("old mono"), 0o644)
		os.MkdirAll(filepath.Join(base, "dingtalk-old"), 0o755)
		os.WriteFile(filepath.Join(base, "dingtalk-old", "SKILL.md"), []byte("stale"), 0o644)
		os.MkdirAll(filepath.Join(base, "other-skill"), 0o755)
		os.WriteFile(filepath.Join(base, "other-skill", "SKILL.md"), []byte("not dws"), 0o644)
	}
	useUpgradeManagedNames(t, "dingtalk-old")

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat", "dws-shared")
	// The release zip ships both trees; the mono sibling must refresh the
	// mono cache too, so --mode mono fallbacks stay on the upgraded version.
	if err := os.MkdirAll(filepath.Join(extract, "mono"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extract, "mono", "SKILL.md"), []byte("# mono sibling"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 0 {
		t.Fatalf("expected 0 failures, got %v", failed)
	}

	for _, base := range []string{claudeBase} {
		if _, err := os.Stat(filepath.Join(base, "dws")); !os.IsNotExist(err) {
			t.Errorf("mono leftover still present: %s", filepath.Join(base, "dws"))
		}
		if _, err := os.Stat(filepath.Join(base, "dingtalk-old")); !os.IsNotExist(err) {
			t.Errorf("stale skill still present: %s", filepath.Join(base, "dingtalk-old"))
		}
		if _, err := os.Stat(filepath.Join(base, "other-skill", "SKILL.md")); err != nil {
			t.Errorf("non-DWS dir should be preserved: %v", err)
		}
		for _, name := range []string{"dingtalk-chat", "dws-shared"} {
			if _, err := os.Stat(filepath.Join(base, name, "SKILL.md")); err != nil {
				t.Errorf("installed skill missing: %s/%s: %v", base, name, err)
			}
			if _, err := os.Stat(filepath.Join(base, name, "references", "guide.md")); err != nil {
				t.Errorf("installed skill references missing: %s/%s: %v", base, name, err)
			}
		}
	}

	// .cursor has no parent dir: must be skipped and untouched.
	cursorBase := filepath.Join(home, ".cursor", "skills")
	if _, err := os.Stat(cursorBase); !os.IsNotExist(err) {
		t.Errorf(".cursor should not be created, stat err = %v", err)
	}

	// Succeeded entries report the agent home base in multi mode.
	succeeded := result.Succeeded()
	if len(succeeded) != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 (%v)", len(succeeded), result.Results)
	}
	wantDirs := map[string]bool{claudeBase: true}
	for _, d := range succeeded {
		if !wantDirs[d.Dir] {
			t.Errorf("unexpected succeeded dir %q", d.Dir)
		}
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Fatalf("generic mono duplicate still visible: %v", err)
	}

	// Multi cache refreshed under the fake home.
	if _, err := os.Stat(filepath.Join(home, ".dws", "skills", "multi", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Errorf("multi cache not refreshed: %v", err)
	}
	// Mono sibling tree refreshed the mono cache too.
	if _, err := os.Stat(filepath.Join(home, ".dws", "skills", "mono", "SKILL.md")); err != nil {
		t.Errorf("mono cache not refreshed from sibling mono tree: %v", err)
	}
}

func TestCrossPlatformCoverageUpgradeUsesAgentSpecificRootWithoutGenericDuplicate(t *testing.T) {
	home := withFakeHome(t)
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".codex/skills"})

	genericBase := filepath.Join(home, ".agents", "skills")
	codexBase := filepath.Join(home, ".codex", "skills")
	for _, base := range []string{genericBase, codexBase} {
		if err := os.MkdirAll(base, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	legacyNested := filepath.Join(genericBase, "dws", "multi", "dingtalk-chat")
	if err := os.MkdirAll(legacyNested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyNested, "SKILL.md"), []byte("old nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	multiRoot := writeMultiBundle(t, t.TempDir(), "dingtalk-chat", "dingtalk-shared")
	result, err := UpgradeSkillLocationsWithOptions(multiRoot, SkillUpgradeOptions{Version: "1.2.3"})
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("UpgradeSkillLocationsWithOptions() = %#v, %v", result, err)
	}

	want := filepath.Join(codexBase, "dingtalk-chat", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("canonical Codex Skill missing at %s: %v", want, err)
	}
	for _, duplicate := range []string{
		filepath.Join(genericBase, "dingtalk-chat", "SKILL.md"),
		filepath.Join(genericBase, "dws", "multi", "dingtalk-chat", "SKILL.md"),
	} {
		if _, err := os.Stat(duplicate); !os.IsNotExist(err) {
			t.Fatalf("generic duplicate still visible at %s: %v", duplicate, err)
		}
	}
}

func TestCrossPlatformCoverageUpgradeSkillLocationsMultiFallbackPrimary(t *testing.T) {
	home := withFakeHome(t)
	// No agent parent dirs at all: only .agents (index 0) is attempted and the
	// primary fallback must also land there.
	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat")

	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("expected 0 failures, got %v", result.Failed())
	}
	primary := filepath.Join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")
	if _, err := os.Stat(primary); err != nil {
		t.Errorf("primary fallback missing: %v", err)
	}
}

func TestCrossPlatformCoverageUpgradeSkillLocationsMonoOnlyPackageStillWorks(t *testing.T) {
	home := withFakeHome(t)
	mono := t.TempDir()
	os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644)

	// Legacy mono-only package (no multi/): fall back to mono refresh even
	// when the disk already has dws/.
	agentsBase := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(filepath.Join(agentsBase, "dws"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "dws", "SKILL.md"), []byte("old mono"), 0o644)
	os.MkdirAll(filepath.Join(agentsBase, "other-skill"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "other-skill", "SKILL.md"), []byte("not dws"), 0o644)

	result, err := UpgradeSkillLocations(mono)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("expected 0 failures, got %v", result.Failed())
	}
	dest := filepath.Join(home, ".agents", "skills", "dws", "SKILL.md")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("mono install missing: %v", err)
	}
	if string(data) != "# mono" {
		t.Errorf("mono content = %q, want refreshed package", data)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS dir should be preserved: %v", err)
	}

	// Mono install refreshes the mono cache so --mode mono fallbacks stay on
	// the upgraded version.
	if _, err := os.Stat(filepath.Join(home, ".dws", "skills", "mono", "SKILL.md")); err != nil {
		t.Errorf("mono cache not refreshed: %v", err)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsMonoDiskMigratesToMulti pins the 2026-08-05
// decision: upgrade does NOT stick to disk. A mono-only home is one-shot
// migrated to multi when the release zip has multi/ (LocateSkillsRoot input).
func TestCrossPlatformCoverageUpgradeSkillLocationsMonoDiskMigratesToMulti(t *testing.T) {
	home := withFakeHome(t)
	agentsBase := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(filepath.Join(agentsBase, "dws"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "dws", "SKILL.md"), []byte("old mono"), 0o644)
	os.MkdirAll(filepath.Join(agentsBase, "other-skill"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "other-skill", "SKILL.md"), []byte("not dws"), 0o644)

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat", "dws-shared")
	if err := os.MkdirAll(filepath.Join(extract, "mono"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extract, "mono", "SKILL.md"), []byte("# mono sibling"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("expected 0 failures, got %v", result.Failed())
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Fatalf("mono leftover dws/ must be removed after multi upgrade, stat err=%v", err)
	}
	for _, name := range []string{"dingtalk-chat", "dws-shared"} {
		if _, err := os.Stat(filepath.Join(agentsBase, name, "SKILL.md")); err != nil {
			t.Errorf("multi skill missing after mono→multi migration: %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS dir should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".dws", "skills", "multi", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Errorf("multi cache not refreshed: %v", err)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsEmptyDiskInstallsMulti pins fresh/empty homes:
// with a multi package, upgrade installs multi (install default) and never
// writes dws/.
func TestCrossPlatformCoverageUpgradeSkillLocationsEmptyDiskInstallsMulti(t *testing.T) {
	home := withFakeHome(t)
	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat", "dws-shared")

	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("expected 0 failures, got %v", result.Failed())
	}
	agentsBase := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"dingtalk-chat", "dws-shared"} {
		if _, err := os.Stat(filepath.Join(agentsBase, name, "SKILL.md")); err != nil {
			t.Errorf("empty-disk multi install missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Errorf("empty-disk multi upgrade must not create dws/, stat err=%v", err)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsMultiDiskRefreshes pins an already-multi home:
// product skills are refreshed, stale dingtalk-* removed, non-DWS kept,
// and dws/ stays absent.
func TestCrossPlatformCoverageUpgradeSkillLocationsMultiDiskRefreshes(t *testing.T) {
	home := withFakeHome(t)
	agentsBase := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(filepath.Join(agentsBase, "dingtalk-chat"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "dingtalk-chat", "SKILL.md"), []byte("OLD chat"), 0o644)
	os.MkdirAll(filepath.Join(agentsBase, "dingtalk-stale"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "dingtalk-stale", "SKILL.md"), []byte("stale"), 0o644)
	useUpgradeManagedNames(t, "dingtalk-stale")
	os.MkdirAll(filepath.Join(agentsBase, "dingtalk-custom"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "dingtalk-custom", "SKILL.md"), []byte("market skill"), 0o644)
	os.MkdirAll(filepath.Join(agentsBase, "other-skill"), 0o755)
	os.WriteFile(filepath.Join(agentsBase, "other-skill", "SKILL.md"), []byte("not dws"), 0o644)

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat", "dws-shared")

	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("expected 0 failures, got %v", result.Failed())
	}
	data, err := os.ReadFile(filepath.Join(agentsBase, "dingtalk-chat", "SKILL.md"))
	if err != nil {
		t.Fatalf("refreshed chat missing: %v", err)
	}
	if string(data) != "# dingtalk-chat" {
		t.Errorf("chat content = %q, want refreshed package", data)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws-shared", "SKILL.md")); err != nil {
		t.Errorf("dws-shared missing after multi refresh: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dingtalk-stale")); !os.IsNotExist(err) {
		t.Errorf("stale multi skill should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Errorf("multi refresh must not create dws/, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS dir should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dingtalk-custom", "SKILL.md")); err != nil {
		t.Errorf("unregistered market/user dingtalk-* dir must be preserved: %v", err)
	}
}

func TestUpgradeMonoCleansPreStateOfficialAndPreservesCustom(t *testing.T) {
	home := withFakeHome(t)
	base := filepath.Join(home, ".agents", "skills")
	legacyOfficial := filepath.Join(base, "dingtalk-aitable")
	custom := filepath.Join(base, "dingtalk-custom")
	for _, dir := range []string{legacyOfficial, custom} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(filepath.Base(dir)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mono := t.TempDir()
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := UpgradeSkillLocations(mono)
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("mono upgrade = %#v, %v", result, err)
	}
	if _, err := os.Stat(legacyOfficial); !os.IsNotExist(err) {
		t.Fatalf("pre-state official Skill survived mono switch: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(custom, "SKILL.md")); err != nil || string(got) != "dingtalk-custom" {
		t.Fatalf("custom same-prefix Skill changed: data=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(base, "dws", "SKILL.md")); err != nil {
		t.Fatalf("mono Skill missing: %v", err)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsMonoFallbackAfterCopyFailure pins the mono primary
// fallback: when the main-loop copy into ~/.agents/skills/dws fails, the
// fallback retries the primary location and reports success (legacy
// mono-only package path).
func TestCrossPlatformCoverageUpgradeSkillLocationsMonoFallbackAfterCopyFailure(t *testing.T) {
	home := withFakeHome(t)
	agentsBase := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(agentsBase, "dws"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsBase, "dws", "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	origCopy := upgradeCopyDir
	copyAttempts := 0
	testseam.Swap(t, &upgradeCopyDir, func(src, dst string) error {
		copyAttempts++
		if copyAttempts == 1 {
			return errors.New("injected copy failure")
		}
		return origCopy(src, dst)
	})

	mono := t.TempDir()
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := UpgradeSkillLocations(mono)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 0 {
		t.Fatalf("fallback should replace the failed entry with OK, got failed=%v", failed)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 (%v)", got, result.Results)
	}
	data, err := os.ReadFile(filepath.Join(agentsBase, "dws", "SKILL.md"))
	if err != nil {
		t.Fatalf("mono fallback install missing: %v", err)
	}
	if string(data) != "# mono" {
		t.Errorf("mono content = %q, want refreshed package", data)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsMonoReadDirErrorFailsHome pins the F4 fix: a base
// directory that exists but cannot be read must mark the home failed instead
// of silently installing mono alongside the multi leftovers.
func TestCrossPlatformCoverageUpgradeSkillLocationsMonoReadDirErrorFailsHome(t *testing.T) {
	home := withFakeHome(t)

	// Inject a read failure for .agents/skills on every platform. .claude
	// installs fine so the primary
	// fallback never runs and the per-home failure is observable as-is.
	agentsBase := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(agentsBase, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	origReadDir := upgradeReadDir
	testseam.Swap(t, &upgradeReadDir, func(path string) ([]os.DirEntry, error) {
		if path == agentsBase {
			return nil, errors.New("injected read failure")
		}
		return origReadDir(path)
	})

	mono := t.TempDir()
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := UpgradeSkillLocations(mono)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v (fallback should not run: .claude succeeded)", err)
	}
	failed := result.Failed()
	if len(failed) != 1 {
		t.Fatalf("Failed() len = %d, want 1 (%v)", len(failed), result.Results)
	}
	wantDir := agentsBase
	if failed[0].Dir != wantDir || failed[0].Err == nil {
		t.Fatalf("failed entry = %#v, want dir %q with non-nil err", failed[0], wantDir)
	}
	if !strings.Contains(failed[0].Err.Error(), "读取技能目录失败") {
		t.Fatalf("failed err should mention the read failure, got %v", failed[0].Err)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 (.claude)", got)
	}
	// Mono must NOT have been laid down next to the unreadable multi state.
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Errorf("mono must not be installed into the unreadable home, stat err=%v", err)
	}
}

// TestCrossPlatformCoverageUpgradeSkillLocationsMultiFallbackCleanupFailure pins the fail-loud
// semantics of the multi fallback: when leftover cleanup fails even at the
// primary location, UpgradeSkillLocations returns an error instead of
// installing multi next to the stale skills.
func TestCrossPlatformCoverageUpgradeSkillLocationsMultiFallbackCleanupFailure(t *testing.T) {
	home := withFakeHome(t)
	agentsBase := filepath.Join(home, ".agents", "skills")
	staleDir := filepath.Join(agentsBase, "dingtalk-stale")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	useUpgradeManagedNames(t, filepath.Base(staleDir))

	origRename := upgradeRename
	testseam.Swap(t, &upgradeRename, func(src, dst string) error {
		if strings.Contains(src, "dingtalk-stale") {
			return errors.New("injected backup failure")
		}
		return origRename(src, dst)
	})

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat")

	result, err := UpgradeSkillLocations(multiRoot)
	if err == nil {
		t.Fatal("multi fallback cleanup failure must return an error")
	}
	if !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("error should mention the fallback cleanup failure, got %v", err)
	}
	if failed := result.Failed(); len(failed) != 1 {
		t.Fatalf("Failed() len = %d, want 1 (%v)", len(failed), result.Results)
	}
	// The stale skill was never removed and no partial multi install landed.
	if _, err := os.Stat(staleDir); err != nil {
		t.Errorf("stale dir should survive the failed cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dingtalk-chat")); !os.IsNotExist(err) {
		t.Errorf("multi skill must not be installed when cleanup failed, stat err=%v", err)
	}
}

// TestCrossPlatformCoverageBackupAndRemoveSkillDirEdges pins the fail-safe
// contract of the backup helper: non-dir paths are no-ops, a colliding backup
// target gets a numbered stamp, and any failure (mkdir / rename / unresolvable
// collision) leaves the original directory untouched with an error.
func TestCrossPlatformCoverageBackupAndRemoveSkillDirEdges(t *testing.T) {
	home := t.TempDir()

	// Stat failure other than not-exist must surface without attempting a
	// backup. Inject it through the path seam so this branch is portable to
	// Windows, where chmod-based permission failures are not reliable.
	statFailurePath := filepath.Join(home, "stat-failure")
	origStat := upgradeStat
	testseam.Swap(t, &upgradeStat, func(path string) (os.FileInfo, error) {
		if path == statFailurePath {
			return nil, errors.New("injected stat failure")
		}
		return origStat(path)
	})
	if got, err := backupAndRemoveSkillDir(home, statFailurePath); got != "" || err == nil || !strings.Contains(err.Error(), "检查技能目录失败") {
		t.Fatalf("stat failure = (%q, %v), want wrapped error", got, err)
	}

	// Regular file: no-op, no backup.
	filePath := filepath.Join(home, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := backupAndRemoveSkillDir(home, filePath); got != "" || err != nil {
		t.Fatalf("regular file = (%q, %v), want no-op", got, err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("regular file must survive: %v", err)
	}

	// A skill directory outside home falls back to its basename instead of
	// allowing a ../ segment into the backup target. Exercise the exported
	// wrapper used by internal/app at the same time.
	outsideVictim := t.TempDir()
	outsideName := filepath.Base(outsideVictim)
	got, err := BackupAndRemoveSkillDir(home, outsideVictim)
	if err != nil {
		t.Fatalf("outside-home backup error = %v", err)
	}
	if filepath.Base(got) != outsideName {
		t.Fatalf("outside-home backup path = %q, want basename %q", got, outsideName)
	}
	if _, err := os.Stat(outsideVictim); !os.IsNotExist(err) {
		t.Fatalf("outside-home victim must be gone after backup, stat err=%v", err)
	}

	testseam.Swap(t, &upgradeBackupStamp, func() string { return "20260810-000000" })

	// Collision: stamp already taken → numbered stamp wins.
	victim := filepath.Join(home, ".agents", "skills", "dws")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(home, skillBackupSubdir, "20260810-000000", ".agents-skills-dws")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = backupAndRemoveSkillDir(home, victim)
	if err != nil {
		t.Fatalf("backup with collision error = %v", err)
	}
	want := filepath.Join(home, skillBackupSubdir, "20260810-000000-1", ".agents-skills-dws")
	if got != want {
		t.Fatalf("backup path = %q, want numbered %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(want, "SKILL.md")); err != nil {
		t.Fatalf("backup content missing: %v", err)
	}
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Fatalf("victim must be gone after backup, stat err=%v", err)
	}

	// Unresolvable collision (>1000 numbered stamps taken) fails and keeps dir.
	victim2 := filepath.Join(home, "victim2")
	if err := os.MkdirAll(victim2, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 1000; i++ {
		stamp := "20260810-000000"
		if i > 0 {
			stamp = fmt.Sprintf("20260810-000000-%d", i)
		}
		if err := os.MkdirAll(filepath.Join(home, skillBackupSubdir, stamp, "victim2"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := backupAndRemoveSkillDir(home, victim2); err == nil || !strings.Contains(err.Error(), "备份目录冲突无法解决") {
		t.Fatalf("collision limit error = %v, want unresolvable", err)
	}
	if _, err := os.Stat(victim2); err != nil {
		t.Fatalf("victim2 must survive unresolvable collision: %v", err)
	}

	// MkdirAll failure: no removal.
	victim3 := filepath.Join(home, "victim3")
	if err := os.MkdirAll(victim3, 0o755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return errors.New("mkdir denied") })
	if _, err := backupAndRemoveSkillDir(home, victim3); err == nil || !strings.Contains(err.Error(), "创建备份目录失败") {
		t.Fatalf("mkdir error = %v", err)
	}
	if _, err := os.Stat(victim3); err != nil {
		t.Fatalf("victim3 must survive mkdir failure: %v", err)
	}
	testseam.Swap(t, &upgradeMkdirAll, os.MkdirAll)

	// Rename failure: no removal.
	victim4 := filepath.Join(home, "victim4")
	if err := os.MkdirAll(victim4, 0o755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &upgradeRename, func(string, string) error { return errors.New("rename denied") })
	if _, err := backupAndRemoveSkillDir(home, victim4); err == nil || !strings.Contains(err.Error(), "备份技能目录失败") {
		t.Fatalf("rename error = %v", err)
	}
	if _, err := os.Stat(victim4); err != nil {
		t.Fatalf("victim4 must survive rename failure: %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)
}

// TestCrossPlatformCoveragePruneSkillBackupsEdges pins the backup retention:
// oldest stamps beyond skillBackupKeep are pruned and a prune failure is
// reported without aborting callers.
func TestCrossPlatformCoveragePruneSkillBackupsEdges(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, skillBackupSubdir)
	for i := 0; i < skillBackupKeep+2; i++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("20260810-00000%d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneSkillBackups(home); err != nil {
		t.Fatalf("pruneSkillBackups() error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != skillBackupKeep {
		t.Fatalf("pruned backups = %d, want %d", len(entries), skillBackupKeep)
	}
	for _, wantGone := range []string{"20260810-000000", "20260810-000001"} {
		if _, err := os.Stat(filepath.Join(root, wantGone)); !os.IsNotExist(err) {
			t.Errorf("oldest backup %s must be pruned, stat err=%v", wantGone, err)
		}
	}

	// Read failure (non-ENOENT) surfaces.
	testseam.Swap(t, &upgradeReadDir, func(string) ([]os.DirEntry, error) { return nil, errors.New("read denied") })
	if err := pruneSkillBackups(home); err == nil {
		t.Fatal("read failure must surface")
	}
	// ENOENT is a no-op.
	testseam.Swap(t, &upgradeReadDir, func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist })
	if err := pruneSkillBackups(home); err != nil {
		t.Fatalf("missing backup root must be a no-op, got %v", err)
	}
	testseam.Swap(t, &upgradeReadDir, os.ReadDir)

	// Removal failure is reported as the first error. Seed more than
	// skillBackupKeep dirs again so the prune loop actually runs.
	for i := 0; i < skillBackupKeep+2; i++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("20260811-00000%d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	testseam.Swap(t, &upgradeRemoveAll, func(string) error { return errors.New("remove denied") })
	if err := pruneSkillBackups(home); err == nil {
		t.Fatal("removal failure must surface")
	}
	testseam.Swap(t, &upgradeRemoveAll, os.RemoveAll)
}

// TestCrossPlatformCoverageResolveSkillSrcLayouts pins every fallback branch of
// the mono-source resolver and the multi-bundle child lookup.
func TestCrossPlatformCoverageResolveSkillSrcLayouts(t *testing.T) {
	// (a) extractedDir itself is the mono tree.
	direct := t.TempDir()
	os.WriteFile(filepath.Join(direct, "SKILL.md"), []byte("# m"), 0o644)
	if got := resolveMonoSkillSrc(direct); got != direct {
		t.Errorf("direct root = %q", got)
	}

	// (b) sibling mono/ next to a multi/ root.
	pack := t.TempDir()
	multiSibling := filepath.Join(pack, "multi")
	os.MkdirAll(multiSibling, 0o755)
	monoSibling := filepath.Join(pack, "mono")
	os.MkdirAll(monoSibling, 0o755)
	os.WriteFile(filepath.Join(monoSibling, "SKILL.md"), []byte("# m"), 0o644)
	if got := resolveMonoSkillSrc(multiSibling); got != monoSibling {
		t.Errorf("sibling mono = %q, want %q", got, monoSibling)
	}

	// (c) extract root SKILL.md fallback.
	rootOnly := t.TempDir()
	os.WriteFile(filepath.Join(rootOnly, "SKILL.md"), []byte("# m"), 0o644)
	child := filepath.Join(rootOnly, "extract-child")
	os.MkdirAll(child, 0o755)
	if got := resolveMonoSkillSrc(child); got != rootOnly {
		t.Errorf("parent root = %q, want %q", got, rootOnly)
	}

	// (d) child mono/ inside the extracted dir.
	childMonoParent := t.TempDir()
	childMono := filepath.Join(childMonoParent, "mono")
	os.MkdirAll(childMono, 0o755)
	os.WriteFile(filepath.Join(childMono, "SKILL.md"), []byte("# m"), 0o644)
	if got := resolveMonoSkillSrc(childMonoParent); got != childMono {
		t.Errorf("child mono = %q, want %q", got, childMono)
	}

	// (e) nothing found.
	if got := resolveMonoSkillSrc(t.TempDir()); got != "" {
		t.Errorf("empty package = %q, want empty", got)
	}

	// resolveMultiBundle: the extracted dir itself is the bundle.
	directBundle := writeMultiBundle(t, pack, "dingtalk-chat")
	if root, skills := resolveMultiBundle(directBundle); root != directBundle || len(skills) != 1 {
		t.Errorf("resolveMultiBundle(direct) = %q %v", root, skills)
	}
	// ... or only its multi/ child is.
	bundleRoot := t.TempDir()
	childBundle := writeMultiBundle(t, bundleRoot, "dingtalk-chat")
	if root, skills := resolveMultiBundle(bundleRoot); root != childBundle || len(skills) != 1 {
		t.Errorf("resolveMultiBundle(child) = %q %v, want %q with 1 skill", root, skills, childBundle)
	}
	if root, skills := resolveMultiBundle(t.TempDir()); root != "" || skills != nil {
		t.Errorf("resolveMultiBundle(empty) = %q %v", root, skills)
	}
}

// TestCrossPlatformCoverageMonoUpgradeBackupAndFallbackEdges pins the mono
// path's fail-loud backup semantics and every primary-fallback outcome.
func TestCrossPlatformCoverageMonoUpgradeBackupAndFallbackEdges(t *testing.T) {
	originalDirs := append([]string(nil), knownSkillDirs...)
	t.Cleanup(func() { knownSkillDirs = originalDirs })

	mono := t.TempDir()
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Per-home backup failure marks that home failed; another home still wins.
	home := withFakeHome(t)
	knownSkillDirs = []string{".agents/skills", ".claude/skills"}
	os.MkdirAll(filepath.Join(home, ".agents", "skills", "dws"), 0o755)
	os.WriteFile(filepath.Join(home, ".agents", "skills", "dws", "SKILL.md"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	testseam.Swap(t, &upgradeRename, func(src, dst string) error {
		if strings.Contains(src, ".agents") {
			return errors.New("backup denied")
		}
		return os.Rename(src, dst)
	})
	result, err := UpgradeSkillLocations(mono)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 1 || failed[0].Err == nil {
		t.Fatalf("failed = %v, want exactly 1 backup-failed home", failed)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws", "SKILL.md")); err != nil {
		t.Fatalf("failed home must keep its original dws/: %v", err)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 (.claude)", got)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Fallback cleanup failure: only a blacklisted home in the main loop, then
	// the primary cleanup hits a stale dir whose backup fails.
	home2 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home2, nil })
	knownSkillDirs = []string{".real/skills"}
	stale := filepath.Join(home2, ".agents", "skills", "dingtalk-stale")
	os.MkdirAll(stale, 0o755)
	useUpgradeManagedNames(t, filepath.Base(stale))
	testseam.Swap(t, &upgradeRename, func(string, string) error { return errors.New("backup denied") })
	if _, err := UpgradeSkillLocations(mono); err == nil || !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("fallback cleanup error = %v", err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("stale dir must survive failed cleanup: %v", err)
	}

	// Fallback backup failure of the primary dws/ itself.
	home3 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home3, nil })
	os.MkdirAll(filepath.Join(home3, ".agents", "skills", "dws"), 0o755)
	if _, err := UpgradeSkillLocations(mono); err == nil || !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("fallback backup error = %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Fallback copy failure: everything fails loud.
	home4 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home4, nil })
	testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return errors.New("copy denied") })
	if _, err := UpgradeSkillLocations(mono); err == nil || !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("fallback copy error = %v", err)
	}
	testseam.Swap(t, &upgradeCopyDir, copyDir)

	// Fallback append (no prior entry for the primary dir): a blacklisted-only
	// main loop leaves no primary entry, so the fallback appends a fresh OK.
	home5 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home5, nil })
	result, err = UpgradeSkillLocations(mono)
	if err != nil {
		t.Fatalf("fallback append error = %v", err)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 appended primary", got)
	}
	if _, err := os.Stat(filepath.Join(home5, ".agents", "skills", "dws", "SKILL.md")); err != nil {
		t.Fatalf("fallback append install missing: %v", err)
	}
}

// TestCrossPlatformCoverageMultiUpgradeBackupAndFallbackEdges pins the multi
// path's blacklisted branch, per-skill backup/copy failures, and the primary
// fallback outcomes (cleanup OK + backup/copy failure, fresh append).
func TestCrossPlatformCoverageMultiUpgradeBackupAndFallbackEdges(t *testing.T) {
	originalDirs := append([]string(nil), knownSkillDirs...)
	t.Cleanup(func() { knownSkillDirs = originalDirs })

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat")

	// Blacklisted entry is reported, non-blacklisted installs.
	home := withFakeHome(t)
	knownSkillDirs = []string{".real/skills", ".agents/skills"}
	os.MkdirAll(filepath.Join(home, ".agents"), 0o755)
	result, err := UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	var blacklisted int
	for _, r := range result.Results {
		if r.Status == SkillDirBlacklisted {
			blacklisted++
		}
	}
	if blacklisted != 1 {
		t.Fatalf("blacklisted entries = %d, want 1 (%v)", blacklisted, result.Results)
	}
	if _, err := os.Stat(filepath.Join(home, ".real")); !os.IsNotExist(err) {
		t.Fatalf("blacklisted home must never be touched, stat err=%v", err)
	}

	// A detected concrete Agent root wins over .agents. A failure while retiring
	// the old generic copy is surfaced after the concrete root succeeds.
	home2 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home2, nil })
	knownSkillDirs = []string{".agents/skills", ".claude/skills"}
	os.MkdirAll(filepath.Join(home2, ".agents", "skills", "dingtalk-chat"), 0o755)
	os.MkdirAll(filepath.Join(home2, ".claude"), 0o755)
	testseam.Swap(t, &upgradeRename, func(src, dst string) error {
		if strings.Contains(src, ".agents") {
			return errors.New("backup denied")
		}
		return os.Rename(src, dst)
	})
	result, err = UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 1 || failed[0].Err == nil {
		t.Fatalf("failed = %v, want 1 backup-failed home", failed)
	}
	if _, err := os.Stat(filepath.Join(home2, ".agents", "skills", "dingtalk-chat")); err != nil {
		t.Fatalf("failed home must keep its original skill: %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Per-skill copy failure on the concrete home fails that home; with no
	// successful concrete target, the generic copy is left untouched.
	home3 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home3, nil })
	os.MkdirAll(filepath.Join(home3, ".claude"), 0o755)
	testseam.Swap(t, &upgradeCopyDir, func(src, dst string) error {
		if strings.Contains(dst, ".claude") {
			return errors.New("copy denied")
		}
		return copyDir(src, dst)
	})
	result, err = UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 1 || failed[0].Err == nil {
		t.Fatalf("failed = %v, want 1 copy-failed home", failed)
	}
	testseam.Swap(t, &upgradeCopyDir, copyDir)

	// Fallback backup failure: the same-name bundle skill refresh is what
	// fails (cleanup succeeds, the per-skill backup does not).
	home4 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home4, nil })
	knownSkillDirs = []string{".real/skills"}
	os.MkdirAll(filepath.Join(home4, ".agents", "skills", "dingtalk-chat"), 0o755)
	testseam.Swap(t, &upgradeRename, func(src, dst string) error {
		if strings.Contains(src, "dingtalk-chat") {
			return errors.New("backup denied")
		}
		return os.Rename(src, dst)
	})
	if _, err := UpgradeSkillLocations(multiRoot); err == nil || !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("fallback backup error = %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Fallback copy failure.
	home5 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home5, nil })
	testseam.Swap(t, &upgradeCopyDir, func(string, string) error { return errors.New("copy denied") })
	if _, err := UpgradeSkillLocations(multiRoot); err == nil || !strings.Contains(err.Error(), "回退到主目录也失败") {
		t.Fatalf("fallback copy error = %v", err)
	}
	testseam.Swap(t, &upgradeCopyDir, copyDir)

	// Fallback append: blacklisted-only main loop → fresh OK entry appended.
	home6 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home6, nil })
	result, err = UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("fallback append error = %v", err)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 appended primary (%v)", got, result.Results)
	}
	if _, err := os.Stat(filepath.Join(home6, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("fallback append install missing: %v", err)
	}

	// Fallback replace: the main loop fails the primary home on a transient
	// copy error, the fallback retry succeeds and replaces the failed entry.
	home7 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home7, nil })
	knownSkillDirs = []string{".agents/skills"}
	origCopy := upgradeCopyDir
	attempts := 0
	testseam.Swap(t, &upgradeCopyDir, func(src, dst string) error {
		attempts++
		if attempts == 1 {
			return errors.New("transient copy failure")
		}
		return origCopy(src, dst)
	})
	result, err = UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("fallback replace error = %v", err)
	}
	if got := len(result.Succeeded()); got != 1 {
		t.Fatalf("Succeeded() len = %d, want 1 replaced primary (%v)", got, result.Results)
	}
	if failed := result.Failed(); len(failed) != 0 {
		t.Fatalf("failed entry must be replaced by OK, got %v", failed)
	}
	if _, err := os.Stat(filepath.Join(home7, ".agents", "skills", "dingtalk-chat", "SKILL.md")); err != nil {
		t.Fatalf("fallback replace install missing: %v", err)
	}
}

// TestCrossPlatformCoverageCleanupLeftoversEdges pins the cleanup helpers:
// read failures surface, backup failures abort, and the opposite-mode cleanup
// preserves bundle skills while removing the mono leftover.
func TestCrossPlatformCoverageCleanupLeftoversEdges(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".agents", "skills")

	// Read failure (non-ENOENT) surfaces from both cleanups.
	testseam.Swap(t, &upgradeReadDir, func(string) ([]os.DirEntry, error) { return nil, errors.New("read denied") })
	if err := cleanupMultiLeftovers(home, base); err == nil || !strings.Contains(err.Error(), "读取技能目录失败") {
		t.Fatalf("cleanupMultiLeftovers read error = %v", err)
	}
	if err := cleanupOppositeModeLeftovers(home, base, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "读取技能目录失败") {
		t.Fatalf("cleanupOppositeModeLeftovers read error = %v", err)
	}
	testseam.Swap(t, &upgradeReadDir, os.ReadDir)

	// Backup failure of a multi leftover aborts cleanupMultiLeftovers.
	os.MkdirAll(filepath.Join(base, "dingtalk-stale"), 0o755)
	useUpgradeManagedNames(t, "dingtalk-stale")
	testseam.Swap(t, &upgradeRename, func(string, string) error { return errors.New("backup denied") })
	if err := cleanupMultiLeftovers(home, base); err == nil || !strings.Contains(err.Error(), "备份并清理 multi 残留失败") {
		t.Fatalf("cleanupMultiLeftovers backup error = %v", err)
	}

	// Backup failure of the mono leftover aborts the opposite-mode cleanup.
	os.MkdirAll(filepath.Join(base, "dws"), 0o755)
	if err := cleanupOppositeModeLeftovers(home, base, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "备份并清理对面模式残留失败") {
		t.Fatalf("opposite cleanup mono backup error = %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Backup failure of a stale (non-mono) skill aborts with its own message.
	testseam.Swap(t, &upgradeRename, func(src, dst string) error {
		if strings.Contains(src, "dingtalk-stale") {
			return errors.New("backup denied")
		}
		return os.Rename(src, dst)
	})
	if err := cleanupOppositeModeLeftovers(home, base, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "备份并清理对面模式残留失败") {
		t.Fatalf("opposite cleanup stale backup error = %v", err)
	}
	testseam.Swap(t, &upgradeRename, os.Rename)

	// Success matrix: mono leftover + stale skill removed into backups, bundle
	// skill and regular file preserved.
	os.MkdirAll(filepath.Join(base, "dws"), 0o755)
	skillSet := map[string]bool{"dingtalk-keep": true}
	os.MkdirAll(filepath.Join(base, "dingtalk-keep"), 0o755)
	os.WriteFile(filepath.Join(base, "regular-file"), []byte("x"), 0o644)
	if err := cleanupOppositeModeLeftovers(home, base, skillSet); err != nil {
		t.Fatalf("cleanupOppositeModeLeftovers() error = %v", err)
	}
	for _, gone := range []string{"dws", "dingtalk-stale"} {
		if _, err := os.Stat(filepath.Join(base, gone)); !os.IsNotExist(err) {
			t.Errorf("%s must be backed up and removed, stat err=%v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "dingtalk-keep")); err != nil {
		t.Errorf("bundle skill must be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "regular-file")); err != nil {
		t.Errorf("regular file must be preserved: %v", err)
	}
	backupRoot := filepath.Join(home, skillBackupSubdir)
	if entries, err := os.ReadDir(backupRoot); err != nil || len(entries) == 0 {
		t.Errorf("removed dirs must land under %s: %v", backupRoot, err)
	}

	// cleanupMultiLeftovers on a missing base is a no-op.
	if err := cleanupMultiLeftovers(home, filepath.Join(home, "missing")); err != nil {
		t.Fatalf("missing base must be a no-op, got %v", err)
	}
}

func TestCrossPlatformCoverageUnifiedSkillOwnershipAndProvenanceFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dingtalk-custom")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if isManagedMultiSkillDir(dir) {
		t.Fatal("an unregistered dingtalk-* directory must not be treated as DWS-owned")
	}
	if !isManagedMultiSkillDir(dir, map[string]bool{"dingtalk-custom": true}) {
		t.Fatal("unified metadata must prove ownership")
	}
	if !isManagedMultiSkillDir(filepath.Join(t.TempDir(), "dws-shared")) {
		t.Fatal("the exact legacy dws-shared name must remain managed")
	}
	if !isManagedMultiSkillDir(filepath.Join(t.TempDir(), "dingtalk-aitable")) {
		t.Fatal("an exact pre-state official name must remain managed")
	}

	home := withFakeHome(t)
	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat")
	testseam.Swap(t, &upgradeBuildProvenance, func(string, string, string, string) (skillprovenance.Record, error) {
		return skillprovenance.Record{}, errors.New("digest denied")
	})
	result, err := UpgradeSkillLocations(multiRoot)
	if err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("provenance failure must fail the upgrade, got %v", err)
	}
	if result != nil {
		t.Fatalf("provenance failure must happen before install: %#v", result)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".dws", "skills-state.json")); !os.IsNotExist(statErr) {
		t.Fatalf("failed managed install must not publish state, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".agents", "skills", "dingtalk-chat")); !os.IsNotExist(statErr) {
		t.Fatalf("provenance failure published a Skill, stat err=%v", statErr)
	}
}
