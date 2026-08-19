// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageSkillBackupOwnership pins the backup ownership
// contract: a stamp-shaped directory name alone proves nothing, so a backup
// root this transaction writes into is either created fresh (mkdir claim,
// marker stamped before any payload moves in) or an existing root whose
// marker already verifies. A same-stamp foreign directory is never adopted —
// writing the marker or the payload into it would turn its contents into
// prunable DWS backups.
func TestCrossPlatformCoverageSkillBackupOwnership(t *testing.T) {

	t.Run("unproven stamp root is never adopted", func(t *testing.T) {
		home := t.TempDir()
		testseam.Swap(t, &upgradeBackupStamp, func() string { return "20260101-120000" })
		foreign := filepath.Join(home, skillBackupSubdir, "20260101-120000")
		if err := os.MkdirAll(foreign, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(foreign, "user-data.txt"), []byte("must survive"), 0o644); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(home, "skills", "dws")
		seedUpgradeSkill(t, victim, "payload", false)
		backup, err := backupAndRemoveSkillDir(home, victim)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, skillBackupSubdir, "20260101-120000-1", "skills-dws"); backup != want {
			t.Fatalf("backup = %s, want suffixed %s", backup, want)
		}
		assertUpgradeSkillContent(t, backup, "payload")
		// The foreign root keeps its data and never receives our marker.
		if data, readErr := os.ReadFile(filepath.Join(foreign, "user-data.txt")); readErr != nil || string(data) != "must survive" {
			t.Fatalf("foreign data = %q, %v", data, readErr)
		}
		if _, statErr := os.Lstat(filepath.Join(foreign, skillBackupMarkerName)); !os.IsNotExist(statErr) {
			t.Fatalf("foreign root must not receive the ownership marker, stat err=%v", statErr)
		}
		if !skillBackupMarkerValid(filepath.Dir(backup)) {
			t.Fatal("the freshly claimed root must carry the ownership marker")
		}
	})

	t.Run("same-stamp payloads reuse the proven root", func(t *testing.T) {
		home := t.TempDir()
		testseam.Swap(t, &upgradeBackupStamp, func() string { return "20260101-120000" })
		first := filepath.Join(home, "skills", "dws")
		second := filepath.Join(home, "agents", "claude", "dws")
		seedUpgradeSkill(t, first, "first", false)
		seedUpgradeSkill(t, second, "second", false)
		backupA, err := backupAndRemoveSkillDir(home, first)
		if err != nil {
			t.Fatal(err)
		}
		backupB, err := backupAndRemoveSkillDir(home, second)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(backupA) != filepath.Dir(backupB) {
			t.Fatalf("same-stamp backups split roots: %s vs %s", backupA, backupB)
		}
		root := filepath.Dir(backupA)
		if !skillBackupMarkerValid(root) {
			t.Fatal("reused root must keep a verifying marker")
		}
		assertUpgradeSkillContent(t, backupA, "first")
		assertUpgradeSkillContent(t, backupB, "second")
	})

	t.Run("marker write failure refuses the backup and cleans the empty root", func(t *testing.T) {
		home := t.TempDir()
		victim := filepath.Join(home, "skills", "dws")
		seedUpgradeSkill(t, victim, "payload", false)
		testseam.Swap(t, &skillBackupWriteMarker, func(string) error { return errors.New("marker denied") })
		_, err := backupAndRemoveSkillDir(home, victim)
		if err == nil || !strings.Contains(err.Error(), "备份所有权标记失败") {
			t.Fatalf("marker write failure = %v", err)
		}
		assertUpgradeSkillContent(t, victim, "payload")
		entries, readErr := os.ReadDir(filepath.Join(home, skillBackupSubdir))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("empty unowned stamp root must be cleaned, left %v", entries)
		}
	})

	t.Run("backup root creation failure surfaces", func(t *testing.T) {
		home := t.TempDir()
		victim := filepath.Join(home, "skills", "dws")
		seedUpgradeSkill(t, victim, "payload", false)
		testseam.Swap(t, &skillPathMkdir, func(string, os.FileMode) error { return os.ErrPermission })
		_, err := backupAndRemoveSkillDir(home, victim)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("root creation failure = %v", err)
		}
		assertUpgradeSkillContent(t, victim, "payload")
	})
}
