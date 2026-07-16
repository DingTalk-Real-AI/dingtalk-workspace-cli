// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	upgradeRename        = os.Rename
	upgradeChmod         = os.Chmod
	upgradeCopyFile      = copyFile
	upgradeSyncParentDir = syncParentDir
	upgradeOpenZipEntry  = func(f *zip.File) (io.ReadCloser, error) { return f.Open() }
	upgradeOpenFile      = os.OpenFile
	upgradeIOCopy        = io.Copy
	upgradeFileSync      = func(f *os.File) error { return f.Sync() }
)

// ReplaceSelf atomically replaces the currently running binary with newBinaryPath.
//
// On Unix (macOS / Linux):
//  1. Try atomic os.Rename (same filesystem, instant swap)
//  2. Fallback to copy if cross-device
//
// On Windows the running .exe is locked by the OS, so direct overwrite fails.
// Strategy: rename running exe → .old, then rename/copy new binary in, then
// clean up .old (best-effort, may be cleaned on next run).
func ReplaceSelf(newBinaryPath string) error {
	currentExe, err := upgradeExecutable()
	if err != nil {
		return fmt.Errorf("无法获取当前二进制路径: %w", err)
	}
	currentExe, err = upgradeEvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("无法解析符号链接: %w", err)
	}

	if err := upgradeChmod(newBinaryPath, filePermBinary); err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}

	if err := replaceExeFile(newBinaryPath, currentExe); err != nil {
		return err
	}

	upgradeSyncParentDir(currentExe)
	return nil
}

// replaceExeFile replaces dst with src, handling Windows file-lock semantics.
func replaceExeFile(src, dst string) error {
	return replaceExeFileFor(src, dst, runtime.GOOS)
}

func replaceExeFileFor(src, dst, goos string) error {
	// Fast path: atomic rename (works on Unix same-filesystem, or Windows when dst is unlocked)
	if err := upgradeRename(src, dst); err == nil {
		return nil
	}

	if goos == "windows" {
		return windowsReplace(src, dst)
	}

	// Unix cross-device fallback
	return upgradeCopyFile(src, dst, filePermBinary)
}

// windowsReplace handles the Windows-specific case where the running exe is locked.
// Windows allows renaming a running executable but not overwriting it.
func windowsReplace(src, dst string) error {
	oldPath := dst + ".old"

	// Clean up leftover .old from a previous upgrade
	os.Remove(oldPath)

	// Move the running (locked) binary out of the way
	if err := upgradeRename(dst, oldPath); err != nil {
		return fmt.Errorf("无法移动正在运行的二进制文件: %w", err)
	}

	// Place the new binary at the target path
	if err := upgradeRename(src, dst); err != nil {
		// Cross-device fallback
		if cpErr := upgradeCopyFile(src, dst, filePermBinary); cpErr != nil {
			// Attempt to restore the original
			upgradeRename(oldPath, dst)
			return fmt.Errorf("替换失败: %w", cpErr)
		}
	}

	// Best-effort cleanup; the .old file may still be locked and will be
	// removed on the next upgrade or reboot.
	os.Remove(oldPath)
	return nil
}

// CleanupStaleFiles removes leftover .old and .rollback-tmp files from
// previous upgrades (relevant on Windows where locked files cannot be
// deleted immediately).
func CleanupStaleFiles() {
	exe, err := CurrentBinaryPath()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
	os.Remove(exe + ".rollback-tmp")
}

// ExtractZip unzips zipPath contents into targetDir.
// Contains zip-slip protection against path traversal attacks.
func ExtractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	targetDir = filepath.Clean(targetDir)
	for _, f := range r.File {
		destPath := filepath.Join(targetDir, f.Name)
		rel, err := filepath.Rel(targetDir, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue // zip-slip guard
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, dirPermShared)
			continue
		}
		if err := extractZipEntry(f, destPath); err != nil {
			return err
		}
	}
	return nil
}

// FindBinaryInDir recursively finds the dws binary in an extracted directory.
func FindBinaryInDir(dir string) string {
	candidates := []string{
		filepath.Join(dir, "dws"),
		filepath.Join(dir, "dws.exe"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "dws" || name == "dws.exe" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func extractZipEntry(f *zip.File, destPath string) error {
	os.MkdirAll(filepath.Dir(destPath), dirPermShared)

	rc, err := upgradeOpenZipEntry(f)
	if err != nil {
		return fmt.Errorf("读取 zip 条目失败: %w", err)
	}
	defer rc.Close()

	out, err := upgradeOpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermConfig)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer out.Close()

	_, err = upgradeIOCopy(out, rc)
	return err
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := upgradeOpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = upgradeIOCopy(out, in); err != nil {
		return err
	}
	return upgradeFileSync(out)
}

func syncParentDir(path string) {
	dir := filepath.Dir(path)
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	f.Sync()
	f.Close()
}
