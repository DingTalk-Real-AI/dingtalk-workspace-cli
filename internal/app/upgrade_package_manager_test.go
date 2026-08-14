// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	upgradepkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

type packageRollbackFake struct {
	backupErr   error
	listErr     error
	rollbackErr error
	cleanupErr  error
	backups     []upgradepkg.BackupInfo
	rolledBack  bool
	cleaned     bool
}

func (r *packageRollbackFake) Backup(string) (string, error) { return "backup", r.backupErr }
func (r *packageRollbackFake) ListBackups() ([]upgradepkg.BackupInfo, error) {
	return r.backups, r.listErr
}
func (r *packageRollbackFake) RollbackTo(upgradepkg.BackupInfo) error {
	r.rolledBack = true
	return r.rollbackErr
}
func (r *packageRollbackFake) Cleanup(int) error { r.cleaned = true; return r.cleanupErr }

func TestPackageUpgradeAllowed(t *testing.T) {
	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	detection := upgradepkg.InstallDetection{Manager: upgradepkg.PackageManagerNPM, Available: true}
	if !packageUpgradeAllowed(upgradeOptions{}, detection) {
		t.Fatal("npm-owned default upgrade should use npm")
	}
	for _, opts := range []upgradeOptions{{targetVersion: "v1.0.0"}, {skipSkills: true}} {
		if packageUpgradeAllowed(opts, detection) {
			t.Fatalf("narrow upgrade %#v unexpectedly used npm", opts)
		}
	}
	t.Setenv("DWS_UPGRADE_URL", "https://mirror.example.com")
	if packageUpgradeAllowed(upgradeOptions{}, detection) {
		t.Fatal("custom release source unexpectedly used npm")
	}
}

func TestRunPackageManagedUpgradeSuccessAndRecovery(t *testing.T) {
	originalRollback := newUpgradeRollback
	originalRun := runUpgradePackageInstall
	originalPrepare := prepareUpgradePackageReplace
	originalVerify := verifyUpgradeInstalledBinary
	t.Cleanup(func() {
		newUpgradeRollback = originalRollback
		runUpgradePackageInstall = originalRun
		prepareUpgradePackageReplace = originalPrepare
		verifyUpgradeInstalledBinary = originalVerify
	})
	detection := upgradepkg.InstallDetection{Manager: upgradepkg.PackageManagerNPM, Available: true}
	failure := errors.New("failure")

	for _, stage := range []string{"success", "backup", "prepare", "install", "install-long", "verify", "verify-restore", "restore", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			rb := &packageRollbackFake{backups: []upgradepkg.BackupInfo{{Version: "1.0.0"}}}
			if stage == "backup" {
				rb.backupErr = failure
			}
			if stage == "restore" || stage == "verify-restore" {
				rb.rollbackErr = failure
			}
			if stage == "cleanup" {
				rb.cleanupErr = failure
			}
			newUpgradeRollback = func() upgradeRollbackManager { return rb }
			restored := false
			prepareUpgradePackageReplace = func() (func(), error) {
				if stage == "prepare" {
					return func() {}, failure
				}
				return func() { restored = true }, nil
			}
			runUpgradePackageInstall = func(context.Context, upgradepkg.PackageManager, string) upgradepkg.PackageInstallResult {
				if stage == "install" || stage == "restore" || stage == "install-long" {
					detail := "npm detail"
					if stage == "install-long" {
						detail = strings.Repeat("x", 2100)
					}
					return upgradepkg.PackageInstallResult{Stderr: detail, Err: failure}
				}
				return upgradepkg.PackageInstallResult{}
			}
			verifyUpgradeInstalledBinary = func(context.Context, string) error {
				if stage == "verify" || stage == "verify-restore" {
					return failure
				}
				return nil
			}
			err := runPackageManagedUpgrade(t.Context(), "v1.0.0", "1.0.1", detection)
			if stage == "success" || stage == "cleanup" {
				if err != nil || !rb.cleaned || rb.rolledBack || restored {
					t.Fatalf("success = err:%v clean:%v rollback:%v restore:%v", err, rb.cleaned, rb.rolledBack, restored)
				}
				return
			}
			if err == nil {
				t.Fatalf("stage %s unexpectedly succeeded", stage)
			}
			if stage == "install" && (!restored || !rb.rolledBack || !strings.Contains(err.Error(), "npm detail")) {
				t.Fatalf("install recovery = err:%v rollback:%v restore:%v", err, rb.rolledBack, restored)
			}
			if stage == "verify" && (!restored || !rb.rolledBack) {
				t.Fatalf("verify recovery = rollback:%v restore:%v", rb.rolledBack, restored)
			}
		})
	}
}

func TestRunUpgradeUsesPackageManagerForPreviewAndInstall(t *testing.T) {
	oldClient, oldRollback := newUpgradeReleaseClient, newUpgradeRollback
	oldEnsure, oldCleanup, oldNeeds := ensureUpgradeDirs, cleanupUpgradeStale, upgradeNeedsUpgrade
	oldDetect, oldRun := detectUpgradeInstall, runUpgradePackageInstall
	oldPrepare, oldVerify := prepareUpgradePackageReplace, verifyUpgradeInstalledBinary
	oldVersion := version
	t.Cleanup(func() {
		newUpgradeReleaseClient, newUpgradeRollback = oldClient, oldRollback
		ensureUpgradeDirs, cleanupUpgradeStale, upgradeNeedsUpgrade = oldEnsure, oldCleanup, oldNeeds
		detectUpgradeInstall, runUpgradePackageInstall = oldDetect, oldRun
		prepareUpgradePackageReplace, verifyUpgradeInstalledBinary = oldPrepare, oldVerify
		version = oldVersion
	})
	version = "1.0.0"
	newUpgradeReleaseClient = func() upgradeReleaseClient {
		return &fakeUpgradeClient{latest: &upgradepkg.ReleaseInfo{Version: "1.0.1"}}
	}
	ensureUpgradeDirs = func() error { return nil }
	cleanupUpgradeStale = func() {}
	upgradeNeedsUpgrade = func(string, string) bool { return true }
	detectUpgradeInstall = func() upgradepkg.InstallDetection {
		return upgradepkg.InstallDetection{Manager: upgradepkg.PackageManagerNPM, Available: true}
	}
	newUpgradeRollback = func() upgradeRollbackManager { return &packageRollbackFake{} }
	prepareUpgradePackageReplace = func() (func(), error) { return func() {}, nil }
	verifyUpgradeInstalledBinary = func(context.Context, string) error { return nil }
	installCalls := 0
	runUpgradePackageInstall = func(context.Context, upgradepkg.PackageManager, string) upgradepkg.PackageInstallResult {
		installCalls++
		return upgradepkg.PackageInstallResult{}
	}
	if err := runUpgrade(t.Context(), upgradeOptions{force: true, yes: true, dryRun: true}); err != nil {
		t.Fatal(err)
	}
	if installCalls != 0 {
		t.Fatal("package dry-run installed")
	}
	if err := runUpgrade(t.Context(), upgradeOptions{force: true, yes: true}); err != nil {
		t.Fatal(err)
	}
	if installCalls != 1 {
		t.Fatalf("package install calls = %d", installCalls)
	}
}

func TestRestoreUpgradeBackupEdges(t *testing.T) {
	if err := restoreUpgradeBackup(&packageRollbackFake{}, ""); err == nil {
		t.Fatal("empty backup path accepted")
	}
	rb := &packageRollbackFake{}
	if err := restoreUpgradeBackup(rb, "/backup"); err != nil || !rb.rolledBack {
		t.Fatalf("restore = %v, rolledBack=%v", err, rb.rolledBack)
	}
	failure := errors.New("rollback")
	if err := rollbackDirectUpgrade(&packageRollbackFake{rollbackErr: failure}, "/backup", errors.New("apply")); err == nil || !strings.Contains(err.Error(), "自动恢复也失败") {
		t.Fatalf("rollback failure = %v", err)
	}
	if err := rollbackDirectUpgrade(&packageRollbackFake{}, "/backup", errors.New("apply")); err == nil || !strings.Contains(err.Error(), "已自动恢复") {
		t.Fatalf("rollback success = %v", err)
	}
}
