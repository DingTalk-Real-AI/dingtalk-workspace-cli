// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDetectInstallFromPath(t *testing.T) {
	for _, test := range []struct {
		name      string
		path      string
		npm, pnpm bool
		manager   PackageManager
		available bool
	}{
		{name: "manual", path: "/usr/local/bin/dws", npm: true, pnpm: true, manager: PackageManagerManual},
		{name: "npm", path: "/usr/local/lib/node_modules/dingtalk-workspace-cli/vendor/dws", npm: true, manager: PackageManagerNPM, available: true},
		{name: "npm-missing", path: "/usr/local/lib/node_modules/dingtalk-workspace-cli/vendor/dws", manager: PackageManagerNPM},
		{name: "pnpm-virtual", path: "/x/node_modules/.pnpm/dingtalk-workspace-cli@1.0.0/node_modules/dingtalk-workspace-cli/vendor/dws", pnpm: true, manager: PackageManagerPNPM, available: true},
		{name: "pnpm-store-windows", path: `C:\Users\u\pnpm\store\v10\links\dingtalk-workspace-cli\node_modules\dingtalk-workspace-cli\vendor\dws.exe`, pnpm: true, manager: PackageManagerPNPM, available: true},
		{name: "npm-under-pnpm-named-dir", path: "/tmp/pnpm/example/node_modules/dingtalk-workspace-cli/vendor/dws", npm: true, manager: PackageManagerNPM, available: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := detectInstallFromPath(test.path, test.npm, test.pnpm)
			if got.Manager != test.manager || got.Available != test.available || got.ResolvedPath != test.path {
				t.Fatalf("detection = %#v", got)
			}
		})
	}
}

func TestRunPackageManagerInstallUsesExactVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	for _, name := range []string{"npm", "pnpm"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$DWS_TEST_ARGS\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DWS_TEST_ARGS", logPath)
	for _, test := range []struct {
		manager PackageManager
		want    string
	}{
		{PackageManagerNPM, "install -g dingtalk-workspace-cli@1.2.3-beta.4"},
		{PackageManagerPNPM, "add -g dingtalk-workspace-cli@1.2.3-beta.4"},
	} {
		result := RunPackageManagerInstall(context.Background(), test.manager, "v1.2.3-beta.4")
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		data, err := os.ReadFile(logPath)
		if err != nil || strings.TrimSpace(string(data)) != test.want {
			t.Fatalf("args = %q, %v", data, err)
		}
	}
	if result := RunPackageManagerInstall(context.Background(), PackageManagerManual, "1.0.0"); result.Err == nil {
		t.Fatal("manual manager unexpectedly installed")
	}
	if got := NPMPackageSpec(" v1.2.3-beta.4 "); got != "dingtalk-workspace-cli@1.2.3-beta.4" {
		t.Fatalf("package spec = %q", got)
	}
}

func TestVerifyInstalledBinaryRequiresExactVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dws")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'Version: v1.2.3-beta.4\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := VerifyInstalledBinary(context.Background(), "1.2.3-beta.4"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyInstalledBinary(context.Background(), "1.2.3-beta.5"); err == nil {
		t.Fatal("version mismatch accepted")
	}
	if got := versionFromOutput("Edition: open\nVersion: v2.0.0\n"); got != "2.0.0" {
		t.Fatalf("version = %q", got)
	}
}

func TestPreparePackageSelfReplaceWindowsRecovery(t *testing.T) {
	originalExecutable, originalEval := packageExecutable, packageEvalSymlinks
	originalGOOS := packageRuntimeGOOS
	t.Cleanup(func() {
		packageExecutable, packageEvalSymlinks = originalExecutable, originalEval
		packageRuntimeGOOS = originalGOOS
	})
	dir := t.TempDir()
	exe := filepath.Join(dir, "dws.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	packageRuntimeGOOS = "windows"
	packageExecutable = func() (string, error) { return exe, nil }
	packageEvalSymlinks = func(path string) (string, error) { return path, nil }
	restore, err := PreparePackageSelfReplace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	restore()
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "old" {
		t.Fatalf("restored = %q, %v", data, err)
	}
}

func TestDetectInstallMethodErrorsAndNoopPrepare(t *testing.T) {
	originalExecutable, originalEval := packageExecutable, packageEvalSymlinks
	originalGOOS := packageRuntimeGOOS
	t.Cleanup(func() {
		packageExecutable, packageEvalSymlinks = originalExecutable, originalEval
		packageRuntimeGOOS = originalGOOS
	})
	packageExecutable = func() (string, error) { return "", errors.New("boom") }
	if got := DetectInstallMethod(); got.Manager != PackageManagerManual {
		t.Fatalf("detection = %#v", got)
	}
	packageRuntimeGOOS = "linux"
	restore, err := PreparePackageSelfReplace()
	if err != nil {
		t.Fatal(err)
	}
	restore()
}

func TestPackageManagerFailureEdges(t *testing.T) {
	originalExecutable, originalEval := packageExecutable, packageEvalSymlinks
	originalLook, originalCommand := packageLookPath, packageCommand
	originalGOOS, originalRename := packageRuntimeGOOS, packageRename
	originalRemove, originalStat := packageRemove, packageStat
	originalInstallWait, originalVerifyWait := packageInstallWait, packageVerifyWait
	t.Cleanup(func() {
		packageExecutable, packageEvalSymlinks = originalExecutable, originalEval
		packageLookPath, packageCommand = originalLook, originalCommand
		packageRuntimeGOOS, packageRename = originalGOOS, originalRename
		packageRemove, packageStat = originalRemove, originalStat
		packageInstallWait, packageVerifyWait = originalInstallWait, originalVerifyWait
	})
	failure := errors.New("injected failure")

	packageExecutable = func() (string, error) { return "/node_modules/dingtalk-workspace-cli/vendor/dws", nil }
	packageEvalSymlinks = func(string) (string, error) { return "", failure }
	if got := DetectInstallMethod(); got.Manager != PackageManagerManual || got.ResolvedPath == "" {
		t.Fatalf("eval failure detection = %#v", got)
	}
	packageEvalSymlinks = func(path string) (string, error) { return path, nil }
	packageLookPath = func(name string) (string, error) { return "/" + name, nil }
	if got := DetectInstallMethod(); !got.CanAutoUpdate() || got.Manager != PackageManagerNPM {
		t.Fatalf("successful detection = %#v", got)
	}

	packageLookPath = func(string) (string, error) { return "", failure }
	if result := RunPackageManagerInstall(t.Context(), PackageManagerNPM, "1.0.0"); result.Err == nil {
		t.Fatal("missing npm accepted")
	}
	packageLookPath = func(string) (string, error) { return "/bin/sh", nil }
	packageInstallWait = time.Millisecond
	packageCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 1")
	}
	if result := RunPackageManagerInstall(t.Context(), PackageManagerNPM, "1.0.0"); result.Err == nil || !strings.Contains(result.Err.Error(), "超时") {
		t.Fatalf("install timeout = %v", result.Err)
	}

	packageRuntimeGOOS = "windows"
	packageExecutable = func() (string, error) { return "", failure }
	if restore, err := PreparePackageSelfReplace(); err != nil {
		t.Fatal(err)
	} else {
		restore()
	}
	packageExecutable = func() (string, error) { return "/dws.exe", nil }
	packageEvalSymlinks = func(string) (string, error) { return "", failure }
	if _, err := PreparePackageSelfReplace(); err != nil {
		t.Fatal(err)
	}
	packageEvalSymlinks = func(path string) (string, error) { return path, nil }
	packageRename = func(string, string) error { return failure }
	if _, err := PreparePackageSelfReplace(); err == nil {
		t.Fatal("rename failure ignored")
	}
	packageRename = func(string, string) error { return nil }
	restore, err := PreparePackageSelfReplace()
	if err != nil {
		t.Fatal(err)
	}
	packageStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	restore()

	packageLookPath = func(string) (string, error) { return "", failure }
	packageExecutable = func() (string, error) { return "", failure }
	if err := VerifyInstalledBinary(t.Context(), "1.0.0"); err == nil {
		t.Fatal("missing binary accepted")
	}
	packageLookPath = func(string) (string, error) { return "/bin/sh", nil }
	packageVerifyWait = time.Millisecond
	packageCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 1")
	}
	if err := VerifyInstalledBinary(t.Context(), "1.0.0"); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("verify timeout = %v", err)
	}
	packageVerifyWait = time.Second
	packageCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 1")
	}
	if err := VerifyInstalledBinary(t.Context(), "1.0.0"); err == nil || !strings.Contains(err.Error(), "无法执行") {
		t.Fatalf("verify execution failure = %v", err)
	}
	packageCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'Edition: open\\n'")
	}
	if err := VerifyInstalledBinary(t.Context(), "1.0.0"); err == nil || !strings.Contains(err.Error(), "未输出版本号") {
		t.Fatalf("missing version = %v", err)
	}
	if got := versionFromOutput("Edition: open\n"); got != "" {
		t.Fatalf("invalid version output = %q", got)
	}
}
