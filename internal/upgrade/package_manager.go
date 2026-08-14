// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	PackageManagerManual PackageManager = "manual"
	PackageManagerNPM    PackageManager = "npm"
	PackageManagerPNPM   PackageManager = "pnpm"

	packageInstallTimeout = 10 * time.Minute
	packageVerifyTimeout  = 10 * time.Second
)

// PackageManager identifies the owner of the running installation.
type PackageManager string

// InstallDetection describes how the current dws binary is installed.
type InstallDetection struct {
	Manager      PackageManager
	ResolvedPath string
	Available    bool
}

// CanAutoUpdate reports whether the owning package manager can update dws.
func (d InstallDetection) CanAutoUpdate() bool {
	return d.Manager != PackageManagerManual && d.Available
}

// PackageInstallResult captures package-manager output without sending it to
// the terminal until the caller knows whether installation succeeded.
type PackageInstallResult struct {
	Stdout string
	Stderr string
	Err    error
}

// NPMPackageSpec returns the exact package selector used for an update.
func NPMPackageSpec(version string) string {
	return npmPackageName + "@" + strings.TrimPrefix(strings.TrimSpace(version), "v")
}

var (
	packageExecutable   = upgradeExecutable
	packageEvalSymlinks = upgradeEvalSymlinks
	packageLookPath     = exec.LookPath
	packageCommand      = exec.CommandContext
	packageRuntimeGOOS  = runtime.GOOS
	packageRename       = os.Rename
	packageRemove       = os.Remove
	packageStat         = os.Stat
	packageInstallWait  = packageInstallTimeout
	packageVerifyWait   = packageVerifyTimeout
)

// DetectInstallMethod mirrors lark-cli's npm/pnpm ownership detection while
// keeping direct release downloads available for manual installations.
func DetectInstallMethod() InstallDetection {
	exe, err := packageExecutable()
	if err != nil {
		return InstallDetection{Manager: PackageManagerManual}
	}
	resolved, err := packageEvalSymlinks(exe)
	if err != nil {
		return InstallDetection{Manager: PackageManagerManual, ResolvedPath: exe}
	}
	_, npmErr := packageLookPath("npm")
	_, pnpmErr := packageLookPath("pnpm")
	return detectInstallFromPath(resolved, npmErr == nil, pnpmErr == nil)
}

func detectInstallFromPath(resolved string, npmAvailable, pnpmAvailable bool) InstallDetection {
	manager := PackageManagerManual
	available := false
	if pathHasSegment(resolved, "node_modules") {
		if containsPNPMMarker(resolved) {
			manager = PackageManagerPNPM
			available = pnpmAvailable
		} else {
			manager = PackageManagerNPM
			available = npmAvailable
		}
	}
	return InstallDetection{Manager: manager, ResolvedPath: resolved, Available: available}
}

func pathHasSegment(path, target string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		if part == target {
			return true
		}
	}
	return false
}

func containsPNPMMarker(path string) bool {
	parts := strings.Split(strings.ReplaceAll(path, `\`, "/"), "/")
	for i, part := range parts {
		if part == ".pnpm" || part == "pnpm" && i+1 < len(parts) && parts[i+1] == "store" {
			return true
		}
	}
	return false
}

// RunPackageManagerInstall installs the exact release version through the
// package manager that owns the current installation.
func RunPackageManagerInstall(ctx context.Context, manager PackageManager, version string) PackageInstallResult {
	if manager != PackageManagerNPM && manager != PackageManagerPNPM {
		return PackageInstallResult{Err: fmt.Errorf("不支持的包管理器: %s", manager)}
	}
	name := string(manager)
	path, err := packageLookPath(name)
	if err != nil {
		return PackageInstallResult{Err: fmt.Errorf("%s 不在 PATH 中: %w", name, err)}
	}
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	args := []string{"install", "-g", NPMPackageSpec(version)}
	if manager == PackageManagerPNPM {
		args = []string{"add", "-g", NPMPackageSpec(version)}
	}
	installCtx, cancel := context.WithTimeout(ctx, packageInstallWait)
	defer cancel()
	cmd := packageCommand(installCtx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if installCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%s 安装超时 (%s)", name, packageInstallWait)
	}
	return PackageInstallResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

// PreparePackageSelfReplace mirrors lark-cli's Windows recovery contract. On
// Unix the running inode can be replaced and the returned restore is a no-op.
func PreparePackageSelfReplace() (restore func(), err error) {
	noop := func() {}
	if packageRuntimeGOOS != "windows" {
		return noop, nil
	}
	exe, err := packageExecutable()
	if err != nil {
		return noop, nil
	}
	exe, err = packageEvalSymlinks(exe)
	if err != nil {
		return noop, nil
	}
	oldPath := exe + ".old"
	_ = packageRemove(oldPath)
	if err := packageRename(exe, oldPath); err != nil {
		return noop, fmt.Errorf("准备 Windows 包升级失败: %w", err)
	}
	return func() {
		if _, statErr := packageStat(oldPath); statErr != nil {
			return
		}
		_ = packageRemove(exe)
		_ = packageRename(oldPath, exe)
	}, nil
}

// VerifyInstalledBinary runs the PATH-visible dws and requires an exact
// version match after a package-manager update.
func VerifyInstalledBinary(ctx context.Context, expectedVersion string) error {
	exe, err := packageLookPath("dws")
	if err != nil {
		exe, err = packageExecutable()
		if err != nil {
			return fmt.Errorf("无法定位升级后的 dws: %w", err)
		}
	}
	verifyCtx, cancel := context.WithTimeout(ctx, packageVerifyWait)
	defer cancel()
	out, err := packageCommand(verifyCtx, exe, "version").CombinedOutput()
	if verifyCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("升级后版本校验超时 (%s)", packageVerifyWait)
	}
	if err != nil {
		return fmt.Errorf("升级后的 dws 无法执行: %w", err)
	}
	actual := versionFromOutput(string(out))
	expected := strings.TrimPrefix(strings.TrimSpace(expectedVersion), "v")
	if actual == "" {
		return fmt.Errorf("升级后的 dws 未输出版本号")
	}
	if actual != expected {
		return fmt.Errorf("升级后版本不匹配: 期望 %s，实际 %s", expected, actual)
	}
	return nil
}

func versionFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "version") {
			return strings.TrimPrefix(strings.TrimSpace(value), "v")
		}
	}
	return ""
}
