// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
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
	Path       string    `json:"path"`
	BinaryPath string    `json:"binaryPath"`
	Version    string    `json:"version"`
	CreatedAt  time.Time `json:"createdAt"`
	Size       int64     `json:"size"`
}

// RollbackManager manages backup and rollback operations.
type RollbackManager struct {
	backupDir  string
	maxBackups int
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

// NewRollbackManagerWithDir creates a rollback manager with a custom directory.
func NewRollbackManagerWithDir(backupDir string) *RollbackManager {
	return &RollbackManager{
		backupDir:  backupDir,
		maxBackups: defaultMaxBackups,
	}
}

// Backup creates a backup of the currently running binary.
// Returns the backup directory path.
func (r *RollbackManager) Backup(currentVersion string) (string, error) {
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

	timestamp := time.Now().Format("20060102-150405")
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
	}
	r.saveBackupInfo(backupInfo)

	return backupSetPath, nil
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
		rollbackRemoveAll(backups[i].Path)
	}
	return nil
}

func (r *RollbackManager) saveBackupInfo(info BackupInfo) {
	infoPath := filepath.Join(info.Path, "info.json")
	data, err := rollbackMarshalIndent(info, "", "  ")
	if err != nil {
		return
	}
	rollbackWriteFile(infoPath, data, filePermConfig)
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
