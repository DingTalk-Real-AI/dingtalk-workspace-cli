// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

// TestCrossPlatformCoverageUpgradeSkillLocationsMonoSeedMigratesToMulti is the fake-HOME E2E for
// the 2026-08-05 owner decision: upgrade is not disk-sticky. Seeding a mono
// layout then calling UpgradeSkillLocations with a multi bundle must install
// product skills, remove dws/, and leave non-DWS dirs alone.
func TestCrossPlatformCoverageUpgradeSkillLocationsMonoSeedMigratesToMulti(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	upgrade.SwapUserHomeDirForTest(t, func() (string, error) { return home, nil })

	agentsBase := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(agentsBase, "dws"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsBase, "dws", "SKILL.md"), []byte("old mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentsBase, "other-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsBase, "other-skill", "SKILL.md"), []byte("not dws"), 0o644); err != nil {
		t.Fatal(err)
	}

	extract := t.TempDir()
	multiRoot := filepath.Join(extract, "multi")
	for _, name := range []string{"dingtalk-chat", "dws-shared"} {
		dir := filepath.Join(multiRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := upgrade.UpgradeSkillLocations(multiRoot)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 0 {
		t.Fatalf("expected 0 failures, got %v", failed)
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "dws")); !os.IsNotExist(err) {
		t.Fatalf("mono leftover dws/ must be gone, stat err=%v", err)
	}
	for _, name := range []string{"dingtalk-chat", "dws-shared"} {
		if _, err := os.Stat(filepath.Join(agentsBase, name, "SKILL.md")); err != nil {
			t.Errorf("multi skill missing: %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS dir should be preserved: %v", err)
	}
}

// TestUpgradeSkillLocationsRealBundleMigratesMono runs the upgrade path against
// the repository's actual multi bundle (skills/multi, post-#887 layout with
// dingtalk-shared). Disk is seeded with a mono install plus the leftovers the
// rename and stale releases leave behind: a pre-rename dws-shared directory
// and a dingtalk-* skill no longer in the bundle. All three must be gone after
// the upgrade, every bundle skill installed, non-DWS dirs untouched, and the
// ~/.dws/skills/multi cache refreshed.
func TestCrossPlatformCoverageUpgradeSkillLocationsRealBundleMigratesMono(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	upgrade.SwapUserHomeDirForTest(t, func() (string, error) { return home, nil })

	bundle := filepath.Join("..", "..", "skills", "multi")
	entries, err := os.ReadDir(bundle)
	if err != nil {
		t.Fatalf("real multi bundle missing: %v", err)
	}
	var bundleNames []string
	for _, e := range entries {
		if e.IsDir() {
			bundleNames = append(bundleNames, e.Name())
		}
	}
	if len(bundleNames) == 0 {
		t.Fatal("real multi bundle is empty")
	}

	agentsBase := filepath.Join(home, ".agents", "skills")
	// Mono install plus pre-rename shared and a stale product skill.
	for _, name := range []string{"dws", "dws-shared", "dingtalk-stale", "other-skill"} {
		dir := filepath.Join(agentsBase, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := skillstate.Write(home, skillstate.State{ManagedSkills: []skillprovenance.Record{{Name: "dingtalk-stale"}}}); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(agentsBase, "dingtalk-custom")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "SKILL.md"), []byte("market skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := upgrade.UpgradeSkillLocations(bundle)
	if err != nil {
		t.Fatalf("UpgradeSkillLocations() error = %v", err)
	}
	if failed := result.Failed(); len(failed) != 0 {
		t.Fatalf("expected 0 failures, got %v", failed)
	}

	for _, stale := range []string{"dws", "dws-shared", "dingtalk-stale"} {
		if _, err := os.Stat(filepath.Join(agentsBase, stale)); !os.IsNotExist(err) {
			t.Errorf("leftover %q must be removed by the upgrade, stat err=%v", stale, err)
		}
	}
	for _, name := range bundleNames {
		if _, err := os.Stat(filepath.Join(agentsBase, name, "SKILL.md")); err != nil {
			t.Errorf("bundle skill %q missing after upgrade: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(agentsBase, "other-skill", "SKILL.md")); err != nil {
		t.Errorf("non-DWS dir should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(custom, "SKILL.md")); err != nil {
		t.Errorf("unregistered market/user dingtalk-* dir should be preserved: %v", err)
	}
	// Shared skill must come from the renamed bundle dir, never the legacy name.
	if _, err := os.Stat(filepath.Join(agentsBase, "dingtalk-shared", "SKILL.md")); err != nil {
		t.Errorf("dingtalk-shared missing: %v", err)
	}
	// Cache refresh so `dws skill setup` fallbacks stay on the upgraded version.
	if _, err := os.Stat(filepath.Join(home, ".dws", "skills", "multi", "SKILL.md")); err != nil {
		// multi cache mirrors the bundle root, which has no top-level SKILL.md;
		// check a bundle skill inside the cache instead.
		if _, err2 := os.Stat(filepath.Join(home, ".dws", "skills", "multi", bundleNames[0], "SKILL.md")); err2 != nil {
			t.Errorf("multi cache not refreshed: %v / %v", err, err2)
		}
	}
}
