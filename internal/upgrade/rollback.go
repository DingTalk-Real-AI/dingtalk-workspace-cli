// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/packagemanifest"
)

const defaultMaxBackups = 5

var (
	rollbackStat          = os.Stat
	rollbackMkdirAll      = os.MkdirAll
	rollbackCopyFile      = copyFile
	rollbackReadDir       = os.ReadDir
	rollbackRemoveAll     = os.RemoveAll
	rollbackMarshalIndent = json.MarshalIndent
	rollbackWriteFile     = os.WriteFile
	rollbackReadFile      = os.ReadFile
	rollbackReplaceFile   = replaceExeFile
	rollbackEntryInfo     = func(entry os.DirEntry) (os.FileInfo, error) { return entry.Info() }
	upgradeDirStat        = os.Stat
	upgradeDirMkdirAll    = os.MkdirAll
	upgradeDirReadDir     = os.ReadDir
	upgradeDirEntryInfo   = func(entry os.DirEntry) (os.FileInfo, error) { return entry.Info() }
	upgradeDirOpen        = os.Open
	upgradeDirOpenFile    = os.OpenFile
	upgradeDirCopy        = io.Copy
)

// BackupInfo contains information about a single backup.
type BackupInfo struct {
	Path        string    `json:"path"`
	BinaryPath  string    `json:"binaryPath"`
	PackageRoot string    `json:"packageRoot,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	Size        int64     `json:"size"`
}

// RollbackManager manages backup and rollback operations.
type RollbackManager struct {
	backupDir    string
	maxBackups   int
	installation *Installation
}

// NewRollbackManager creates a rollback manager using the standard backup directory.
func NewRollbackManager() *RollbackManager {
	homeDir, err := upgradeUserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return &RollbackManager{
		backupDir:  filepath.Join(homeDir, ".dws", "data", "backups"),
		maxBackups: defaultMaxBackups,
	}
}

// NewRollbackManagerForInstallation binds rollback operations to the same
// trusted installation identity used by activation.
func NewRollbackManagerForInstallation(installation Installation) *RollbackManager {
	manager := NewRollbackManager()
	manager.installation = &installation
	manager.backupDir = filepath.Join(installation.InstallRoot, ".dws-backups")
	return manager
}

// NewRollbackManagerWithDir creates a rollback manager with a custom directory.
func NewRollbackManagerWithDir(backupDir string) *RollbackManager {
	return &RollbackManager{
		backupDir:  backupDir,
		maxBackups: defaultMaxBackups,
	}
}

// Backup records a complete immutable package generation. Legacy flat
// installations are copied only as one-time migration rollback material.
func (r *RollbackManager) Backup(currentVersion string) (string, error) {
	if r.installation != nil && !r.installation.LegacyFlat {
		if _, err := packagemanifest.VerifyTree(r.installation.PackageRoot, packagemanifest.Identity{
			Release: r.installation.Manifest.Release,
			Target:  r.installation.Manifest.Target,
		}); err != nil {
			return "", fmt.Errorf("验证当前 package 失败: %w", err)
		}
		return r.recordBackup(BackupInfo{
			PackageRoot: r.installation.PackageRoot,
			Kind:        "package", Version: strings.TrimPrefix(r.installation.Manifest.Release.Version, "v"),
			CreatedAt: time.Now(),
		})
	}
	currentExe, err := upgradeExecutable()
	if err != nil {
		return "", fmt.Errorf("无法获取当前二进制路径: %w", err)
	}
	currentExe, err = upgradeEvalSymlinks(currentExe)
	if err != nil {
		return "", fmt.Errorf("无法解析符号链接: %w", err)
	}

	info, err := rollbackStat(currentExe)
	if err != nil {
		return "", fmt.Errorf("无法读取当前二进制信息: %w", err)
	}

	if err := rollbackMkdirAll(r.backupDir, dirPermSecure); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102-150405.000000000")
	backupSetName := fmt.Sprintf("v%s-%s", currentVersion, timestamp)
	backupSetPath := filepath.Join(r.backupDir, backupSetName)

	if err := rollbackMkdirAll(backupSetPath, dirPermSecure); err != nil {
		return "", fmt.Errorf("创建备份集目录失败: %w", err)
	}

	binaryBackupDir := filepath.Join(backupSetPath, "binary")
	if err := rollbackMkdirAll(binaryBackupDir, dirPermSecure); err != nil {
		return "", fmt.Errorf("创建二进制备份目录失败: %w", err)
	}

	binaryBackupPath := filepath.Join(binaryBackupDir, filepath.Base(currentExe))
	if err := rollbackCopyFile(currentExe, binaryBackupPath, info.Mode()); err != nil {
		return "", fmt.Errorf("备份二进制失败: %w", err)
	}

	backupInfo := BackupInfo{
		Path:       backupSetPath,
		BinaryPath: binaryBackupPath,
		Version:    currentVersion,
		CreatedAt:  time.Now(),
		Size:       info.Size(),
		Kind:       "legacy-flat",
	}
	if err := r.saveBackupInfoStrict(backupInfo); err != nil {
		return "", fmt.Errorf("写入备份 metadata 失败: %w", err)
	}

	return backupSetPath, nil
}

func (r *RollbackManager) recordBackup(info BackupInfo) (string, error) {
	if err := rollbackMkdirAll(r.backupDir, dirPermSecure); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	path := filepath.Join(r.backupDir, "v"+info.Version+"-"+stamp)
	if err := rollbackMkdirAll(path, dirPermSecure); err != nil {
		return "", err
	}
	info.Path = path
	if err := r.saveBackupInfoStrict(info); err != nil {
		return "", fmt.Errorf("写入备份 metadata 失败: %w", err)
	}
	return path, nil
}

// Rollback restores the most recent backup.
func (r *RollbackManager) Rollback() error {
	backups, err := r.ListBackups()
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("没有可用的备份")
	}
	return r.RollbackTo(backups[0])
}

// RollbackTo restores a specific backup.
// Uses replaceExeFile to handle Windows file-lock semantics correctly.
func (r *RollbackManager) RollbackTo(backup BackupInfo) error {
	if backup.PackageRoot != "" || backup.Kind == "package" {
		if r.installation == nil || r.installation.LegacyFlat {
			return errors.New("package rollback requires a trusted canonical launcher invocation")
		}
		manifest, err := decodeManifest(filepath.Join(backup.PackageRoot, packagemanifest.ManifestName))
		if err != nil {
			return fmt.Errorf("读取回滚 package manifest 失败: %w", err)
		}
		expected := packagemanifest.Identity{Release: manifest.Release, Target: r.installation.Manifest.Target}
		if manifest.Release.Edition != r.installation.Edition {
			return fmt.Errorf("回滚 package edition 不匹配: %s", manifest.Release.Edition)
		}
		if _, err := packagemanifest.VerifyTree(backup.PackageRoot, expected); err != nil {
			return fmt.Errorf("验证回滚 package 失败: %w", err)
		}
		previous := r.installation.PackageRoot
		if err := switchPointer(r.installation.CurrentPath, backup.PackageRoot, r.installation.GOOS, r.installation.TextPointer); err != nil {
			return fmt.Errorf("切换回滚 package 失败: %w", err)
		}
		if err := smokeLauncher(r.installation.PublicLauncher, manifest.Release.Version); err != nil {
			smokeErr := fmt.Errorf("回滚 smoke 失败: %w", err)
			if restoreErr := RestoreActivation(*r.installation, previous); restoreErr != nil {
				return activationStateUncertain(smokeErr, restoreErr)
			}
			return fmt.Errorf("回滚 smoke 失败，已恢复当前版本: %w", err)
		}
		return nil
	}
	if backup.Kind == "legacy-flat" && r.installation != nil && !r.installation.LegacyFlat {
		info, err := os.Lstat(backup.BinaryPath)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("legacy backup is not a regular non-symlink file")
			}
			return fmt.Errorf("验证 legacy flat 回滚文件失败: %w", err)
		}
		temp := filepath.Join(filepath.Dir(r.installation.PublicLauncher), ".dws-legacy-rollback-tmp")
		_ = os.Remove(temp)
		if err := rollbackCopyFile(backup.BinaryPath, temp, info.Mode()); err != nil {
			return fmt.Errorf("准备 legacy flat 回滚失败: %w", err)
		}
		if err := replacePointerPath(temp, r.installation.PublicLauncher); err != nil {
			_ = os.Remove(temp)
			return fmt.Errorf("发布 legacy flat 回滚失败: %w", err)
		}
		if err := smokeLauncher(r.installation.PublicLauncher, canonicalVersion(backup.Version)); err != nil {
			launcherRel, _, _ := packagemanifest.Paths(r.installation.Manifest.Target)
			restoreErr := replaceSymlink(r.installation.PublicLauncher, filepath.Join(r.installation.CurrentPath, filepath.FromSlash(launcherRel)))
			if restoreErr == nil {
				restoreErr = smokeLauncher(r.installation.PublicLauncher, r.installation.Manifest.Release.Version)
			}
			if restoreErr != nil {
				return activationStateUncertain(fmt.Errorf("legacy flat 回滚 smoke 失败: %w", err), restoreErr)
			}
			return fmt.Errorf("legacy flat 回滚 smoke 失败，已恢复 canonical launcher: %w", err)
		}
		return nil
	}
	currentExe, err := upgradeExecutable()
	if err != nil {
		return fmt.Errorf("无法获取当前二进制路径: %w", err)
	}
	currentExe, err = upgradeEvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("无法解析符号链接: %w", err)
	}

	binaryBackupPath := backup.BinaryPath
	if binaryBackupPath == "" {
		binaryBackupPath = filepath.Join(backup.Path, "binary", BinaryName())
	}

	if _, err := rollbackStat(binaryBackupPath); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在: %s", binaryBackupPath)
	}

	// Copy backup to a temp file first so replaceExeFile can use rename
	tmpPath := currentExe + ".rollback-tmp"
	if err := rollbackCopyFile(binaryBackupPath, tmpPath, filePermBinary); err != nil {
		return fmt.Errorf("准备回滚文件失败: %w", err)
	}

	if err := rollbackReplaceFile(tmpPath, currentExe); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("恢复二进制失败: %w", err)
	}

	syncFileData(currentExe)
	return nil
}

// ListBackups returns all available backups, newest first.
func (r *RollbackManager) ListBackups() ([]BackupInfo, error) {
	entries, err := rollbackReadDir(r.backupDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		backupPath := filepath.Join(r.backupDir, entry.Name())
		info, err := r.loadBackupInfo(backupPath)
		if err != nil {
			fi, statErr := rollbackEntryInfo(entry)
			if statErr != nil {
				continue
			}
			info = BackupInfo{
				Path:      backupPath,
				Version:   parseVersionFromBackupName(entry.Name()),
				CreatedAt: fi.ModTime(),
			}
		}
		info.Path = backupPath
		backups = append(backups, info)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// Cleanup removes old backups, keeping only the most recent N.
func (r *RollbackManager) Cleanup(keep int) error {
	backups, err := r.ListBackups()
	if err != nil {
		return err
	}
	if keep <= 0 {
		keep = r.maxBackups
	}
	for i := keep; i < len(backups); i++ {
		if backups[i].PackageRoot != "" {
			r.removePackageIfInactive(backups[i].PackageRoot)
		}
		rollbackRemoveAll(backups[i].Path)
	}
	return nil
}

func (r *RollbackManager) removePackageIfInactive(packageRoot string) {
	if r.installation == nil {
		return
	}
	root, err := canonicalExistingPath(packageRoot)
	if err != nil || filepath.Dir(root) != filepath.Clean(r.installation.VersionsDir) {
		versions, versionsErr := canonicalExistingPath(r.installation.VersionsDir)
		if err != nil || versionsErr != nil || filepath.Dir(root) != versions {
			return
		}
	}
	protected := []string{r.installation.PackageRoot}
	if current, err := activePackageRoot(*r.installation); err == nil {
		protected = append(protected, current)
	}
	if running, err := filepath.EvalSymlinks(r.installation.RunningPath); err == nil {
		protected = append(protected, filepath.Dir(filepath.Dir(running)))
	}
	for _, path := range protected {
		absolute, _ := canonicalExistingPath(path)
		if absolute == root {
			return
		}
	}
	_ = rollbackRemoveAll(root)
}

func activePackageRoot(installation Installation) (string, error) {
	if !installation.TextPointer {
		return canonicalExistingPath(installation.CurrentPath)
	}
	info, err := os.Lstat(installation.CurrentPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1024 {
		return "", errors.New("invalid Windows current.txt")
	}
	body, err := os.ReadFile(installation.CurrentPath)
	if err != nil {
		return "", err
	}
	name, err := parseWindowsPointer(body)
	if err != nil {
		return "", err
	}
	root := filepath.Join(installation.VersionsDir, name)
	canonical, err := canonicalExistingPath(root)
	if err != nil {
		return "", err
	}
	versions, err := canonicalExistingPath(installation.VersionsDir)
	if err != nil || filepath.Dir(canonical) != versions {
		return "", errors.New("Windows current.txt escapes versions directory")
	}
	return canonical, nil
}

func canonicalExistingPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func (r *RollbackManager) saveBackupInfo(info BackupInfo) {
	_ = r.saveBackupInfoStrict(info)
}

func (r *RollbackManager) saveBackupInfoStrict(info BackupInfo) error {
	infoPath := filepath.Join(info.Path, "info.json")
	data, err := rollbackMarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return rollbackWriteFile(infoPath, data, filePermConfig)
}

func (r *RollbackManager) loadBackupInfo(backupSetPath string) (BackupInfo, error) {
	infoPath := filepath.Join(backupSetPath, "info.json")
	data, err := rollbackReadFile(infoPath)
	if err != nil {
		return BackupInfo{}, err
	}
	var info BackupInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return BackupInfo{}, err
	}
	return info, nil
}

// parseVersionFromBackupName extracts version from "v0.2.7-20260314-100523".
func parseVersionFromBackupName(name string) string {
	if len(name) > 1 && name[0] == 'v' {
		// Find first '-' after version digits
		for i := 1; i < len(name); i++ {
			if name[i] == '-' {
				// Check if next char is a digit (timestamp), meaning this is the separator
				if i+1 < len(name) && name[i+1] >= '0' && name[i+1] <= '9' {
					return name[1:i]
				}
			}
		}
		return name[1:]
	}
	return "unknown"
}

func syncFileData(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	f.Sync()
	f.Close()
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	srcInfo, err := upgradeDirStat(src)
	if err != nil {
		return err
	}
	if err := upgradeDirMkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := upgradeDirReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			info, err := upgradeDirEntryInfo(entry)
			if err != nil {
				continue
			}
			srcFile, err := upgradeDirOpen(srcPath)
			if err != nil {
				return err
			}
			dstFile, err := upgradeDirOpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
			if err != nil {
				srcFile.Close()
				return err
			}
			_, err = upgradeDirCopy(dstFile, srcFile)
			srcFile.Close()
			dstFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
