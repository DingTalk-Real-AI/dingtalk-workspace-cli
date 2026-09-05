package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/packagemanifest"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestResolveInstallationRejectsSpoofedIdentity(t *testing.T) {
	root := t.TempDir()
	packageRoot, identity := testPackage(t, filepath.Join(root, managedVersionsDir, "v1"), "v1.2.3", "commit-one", false)
	current := filepath.Join(root, managedCurrent)
	if err := os.Symlink(filepath.Join(managedVersionsDir, "v1"), current); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "bin", executableNameFor(runtime.GOOS, "dws"))
	if err := os.MkdirAll(filepath.Dir(public), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherRel, coreRel, _ := packagemanifest.Paths(identity.Target)
	if err := os.Symlink(filepath.Join("..", managedCurrent, filepath.FromSlash(launcherRel)), public); err != nil {
		t.Fatal(err)
	}
	launcherPath := filepath.Join(packageRoot, filepath.FromSlash(launcherRel))
	corePath := filepath.Join(packageRoot, filepath.FromSlash(coreRel))
	launcherPath, _ = filepath.EvalSymlinks(launcherPath)
	corePath, _ = filepath.EvalSymlinks(corePath)
	values := map[string]string{
		"DWS_INTERNAL_LAUNCHER_PATH": launcherPath,
		"DWS_INTERNAL_CORE_SHA256":   mustManifest(t, packageRoot).Core.SHA256,
		"DWS_INTERNAL_CORE_VERSION":  "v1.2.3",
	}
	testseam.Swap(t, &upgradeExecutable, func() (string, error) { return corePath, nil })
	testseam.Swap(t, &upgradeGetenv, func(key string) string { return values[key] })

	if _, err := resolveInstallationFor("1.2.3", "open", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	for name, mutate := range map[string]func(){
		"digest":  func() { values["DWS_INTERNAL_CORE_SHA256"] = strings.Repeat("0", 64) },
		"version": func() { values["DWS_INTERNAL_CORE_VERSION"] = "v9.9.9" },
		"partial": func() { values["DWS_INTERNAL_CORE_VERSION"] = "" },
	} {
		t.Run(name, func(t *testing.T) {
			original := map[string]string{}
			for key, value := range values {
				original[key] = value
			}
			mutate()
			if _, err := resolveInstallationFor("1.2.3", "open", runtime.GOOS, runtime.GOARCH); err == nil {
				t.Fatal("spoofed identity accepted")
			}
			values = original
		})
	}
}

func TestResolveInstallationDirectCoreAndLegacyFlat(t *testing.T) {
	values := map[string]string{}
	testseam.Swap(t, &upgradeGetenv, func(key string) string { return values[key] })
	testseam.Swap(t, &upgradeExecutable, func() (string, error) { return filepath.Join(t.TempDir(), "libexec", "dws-core"), nil })
	if _, err := resolveInstallationFor("1.0.0", "open", runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "canonical dws launcher") {
		t.Fatalf("direct core error = %v", err)
	}
	flat := filepath.Join(t.TempDir(), executableNameFor(runtime.GOOS, "dws"))
	testseam.Swap(t, &upgradeExecutable, func() (string, error) { return flat, nil })
	installation, err := resolveInstallationFor("1.0.0", "open", runtime.GOOS, runtime.GOARCH)
	if err != nil || !installation.LegacyFlat || installation.VersionsDir != filepath.Join(filepath.Dir(flat), ".dws", managedVersionsDir) {
		t.Fatalf("legacy flat resolution = %#v, %v", installation, err)
	}
}

func TestActivatePackageRestoresCurrentOnSmokeFailure(t *testing.T) {
	root := t.TempDir()
	oldRoot, oldIdentity := testPackage(t, filepath.Join(root, managedVersionsDir, "old"), "v1.0.0", "old-commit", false)
	newRoot, newIdentity := testPackage(t, filepath.Join(t.TempDir(), "new"), "v2.0.0", "new-commit", true)
	current := filepath.Join(root, managedCurrent)
	if err := os.Symlink(filepath.Join(managedVersionsDir, "old"), current); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "bin", executableNameFor(runtime.GOOS, "dws"))
	if err := os.MkdirAll(filepath.Dir(public), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherRel, coreRel, _ := packagemanifest.Paths(oldIdentity.Target)
	if err := os.Symlink(filepath.Join("..", managedCurrent, filepath.FromSlash(launcherRel)), public); err != nil {
		t.Fatal(err)
	}
	installation := Installation{
		PackageRoot: oldRoot, InstallRoot: root, VersionsDir: filepath.Join(root, managedVersionsDir),
		CurrentPath: current, PublicLauncher: public,
		RunningPath: filepath.Join(oldRoot, filepath.FromSlash(coreRel)), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Edition: "open",
	}
	if _, err := ActivatePackage(installation, newRoot, newIdentity); err == nil {
		t.Fatal("activation with failing launcher succeeded")
	}
	resolved, err := filepath.EvalSymlinks(current)
	wantRoot, _ := filepath.EvalSymlinks(oldRoot)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(wantRoot) {
		t.Fatalf("current = %q, %v; want %q", resolved, err, oldRoot)
	}
}

func TestActivatePackageReportsUncertainWhenRestoredLauncherFailsSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	oldRoot, oldIdentity := testPackage(t, filepath.Join(root, managedVersionsDir, "old"), "v1.0.0", "old-commit", true)
	newRoot, newIdentity := testPackage(t, filepath.Join(t.TempDir(), "new"), "v2.0.0", "new-commit", true)
	current := filepath.Join(root, managedCurrent)
	if err := os.Symlink(filepath.Join(managedVersionsDir, "old"), current); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "bin", "dws")
	if err := os.MkdirAll(filepath.Dir(public), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherRel, coreRel, _ := packagemanifest.Paths(oldIdentity.Target)
	if err := os.Symlink(filepath.Join("..", managedCurrent, filepath.FromSlash(launcherRel)), public); err != nil {
		t.Fatal(err)
	}
	installation := Installation{
		PackageRoot: oldRoot, VersionsDir: filepath.Join(root, managedVersionsDir), CurrentPath: current,
		PublicLauncher: public, RunningPath: filepath.Join(oldRoot, filepath.FromSlash(coreRel)),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, PreviousVersion: "v1.0.0",
	}
	if _, err := ActivatePackage(installation, newRoot, newIdentity); !errors.Is(err, ErrActivationStateUncertain) {
		t.Fatalf("error = %v, want state uncertain", err)
	}
}

func TestActivatePackageMigratesLegacyFlat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("running flat Windows migration intentionally requires installer assistance")
	}
	root := t.TempDir()
	flat := filepath.Join(root, "dws")
	if err := os.WriteFile(flat, []byte("#!/bin/sh\necho legacy\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newRoot, identity := testPackage(t, filepath.Join(t.TempDir(), "new"), "v2.0.0", "new-commit", false)
	installation := Installation{
		InstallRoot: filepath.Join(root, ".dws"), VersionsDir: filepath.Join(root, ".dws", managedVersionsDir), CurrentPath: filepath.Join(root, ".dws", managedCurrent),
		PublicLauncher: flat, RunningPath: flat, LegacyFlat: true, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Edition: "open",
	}
	previous, err := ActivatePackage(installation, newRoot, identity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previous, "legacy-flat-") {
		t.Fatalf("legacy backup = %q", previous)
	}
	if info, err := os.Lstat(flat); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("public launcher was not migrated to symlink: %v, %v", info, err)
	}
}

func TestPackageRollbackAtomicallySwitchesGeneration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only; Windows pointer replacement is cross-compiled")
	}
	root := t.TempDir()
	oldRoot, identity := testPackage(t, filepath.Join(root, managedVersionsDir, "old"), "v1.0.0", "old-commit", false)
	newRoot, _ := testPackage(t, filepath.Join(root, managedVersionsDir, "new"), "v2.0.0", "new-commit", false)
	current := filepath.Join(root, managedCurrent)
	if err := os.Symlink(filepath.Join(managedVersionsDir, "new"), current); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "bin", "dws")
	if err := os.MkdirAll(filepath.Dir(public), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherRel, coreRel, _ := packagemanifest.Paths(identity.Target)
	if err := os.Symlink(filepath.Join("..", managedCurrent, filepath.FromSlash(launcherRel)), public); err != nil {
		t.Fatal(err)
	}
	newManifest := mustManifest(t, newRoot)
	manager := NewRollbackManagerForInstallation(Installation{
		PackageRoot: newRoot, Manifest: newManifest, InstallRoot: root,
		VersionsDir: filepath.Join(root, managedVersionsDir), CurrentPath: current,
		PublicLauncher: public, RunningPath: filepath.Join(newRoot, filepath.FromSlash(coreRel)),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Edition: "open",
	})
	if err := manager.RollbackTo(BackupInfo{PackageRoot: oldRoot, Kind: "package", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(current)
	want, _ := filepath.EvalSymlinks(oldRoot)
	if err != nil || resolved != want {
		t.Fatalf("rollback current = %q, %v; want %q", resolved, err, want)
	}
}

func TestRollbackCleanupPreservesActiveAndRunningPackages(t *testing.T) {
	root := t.TempDir()
	versions := filepath.Join(root, managedVersionsDir)
	active := filepath.Join(versions, "active")
	running := filepath.Join(versions, "running")
	stale := filepath.Join(versions, "stale")
	for _, path := range []string{active, running, stale} {
		if err := os.MkdirAll(filepath.Join(path, "libexec"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	current := filepath.Join(root, managedCurrent)
	if err := os.Symlink(filepath.Join(managedVersionsDir, "active"), current); err != nil {
		t.Fatal(err)
	}
	manager := NewRollbackManagerForInstallation(Installation{
		PackageRoot: running, VersionsDir: versions, CurrentPath: current,
		RunningPath: filepath.Join(running, "libexec", "dws-core"), InstallRoot: root,
	})
	manager.backupDir = filepath.Join(root, "backups")
	for index, packageRoot := range []string{stale, active, running} {
		path := filepath.Join(manager.backupDir, string(rune('a'+index)))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		manager.saveBackupInfo(BackupInfo{Path: path, PackageRoot: packageRoot, Kind: "package", Version: "1", CreatedAt: time.Unix(int64(index+1), 0)})
	}
	if err := manager.Cleanup(1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale package retained: %v", err)
	}
	for _, path := range []string{active, running} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected package removed: %s: %v", path, err)
		}
	}
}

func TestWindowsRetentionParsesCurrentPointerAndProtectsAllLiveGenerations(t *testing.T) {
	root := t.TempDir()
	versions := filepath.Join(root, managedVersionsDir)
	active := filepath.Join(versions, "active")
	running := filepath.Join(versions, "running")
	rollbackTarget := filepath.Join(versions, "rollback-target")
	stale := filepath.Join(versions, "stale")
	for _, path := range []string{active, running, rollbackTarget, stale} {
		if err := os.MkdirAll(filepath.Join(path, "libexec"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(running, "libexec", "dws-core.exe"), []byte("running"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, managedWindowsCurrent)
	if err := os.WriteFile(current, []byte("active\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewRollbackManagerForInstallation(Installation{
		PackageRoot: rollbackTarget, VersionsDir: versions, CurrentPath: current, TextPointer: true,
		RunningPath: filepath.Join(running, "libexec", "dws-core.exe"), GOOS: "windows",
	})
	for _, path := range []string{active, running, rollbackTarget, stale} {
		manager.removePackageIfInactive(path)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale generation retained: %v", err)
	}
	for _, path := range []string{active, running, rollbackTarget} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("live generation removed: %s: %v", path, err)
		}
	}
	if err := os.WriteFile(current, []byte("../stale\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := activePackageRoot(*manager.installation); err == nil {
		t.Fatal("unsafe Windows pointer was accepted")
	}
}

func TestExtractAndVerifyPackageRejectsTraversalAndSymlink(t *testing.T) {
	identity := packagemanifest.Identity{
		Release: packagemanifest.Release{Version: "v1.2.3", Commit: "commit", Edition: "open"},
		Target:  packagemanifest.Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
	}
	for name, header := range map[string]*tar.Header{
		"traversal": {Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg},
		"symlink":   {Name: "dws-v1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + "/bin/dws", Linkname: "elsewhere", Typeflag: tar.TypeSymlink},
	} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "bad.tar.gz")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			writer := tar.NewWriter(gz)
			if err := writer.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if header.Size > 0 {
				_, _ = writer.Write([]byte("x"))
			}
			_ = writer.Close()
			_ = gz.Close()
			_ = file.Close()
			if _, _, err := ExtractAndVerifyPackage(archive, filepath.Join(t.TempDir(), "out"), identity); err == nil {
				t.Fatal("malicious archive accepted")
			}
		})
	}
}

func TestWindowsTargetManagedPaths(t *testing.T) {
	root := t.TempDir()
	packageRoot, identity := testPackageForTarget(t, filepath.Join(root, managedVersionsDir, "v1"), "v1.0.0", "commit", false, packagemanifest.Target{GOOS: "windows", GOARCH: "amd64"})
	launcherRel, coreRel, _ := packagemanifest.Paths(identity.Target)
	if filepath.Base(filepath.Join(packageRoot, filepath.FromSlash(launcherRel))) != "dws.exe" || filepath.Base(filepath.Join(packageRoot, filepath.FromSlash(coreRel))) != "dws-core.exe" {
		t.Fatal("Windows package paths lost executable suffix")
	}
	installation := Installation{InstallRoot: root, VersionsDir: filepath.Join(root, managedVersionsDir), GOOS: "windows"}
	if got := filepath.Join(installation.InstallRoot, "bin", executableNameFor(installation.GOOS, "dws")); filepath.Base(got) != "dws.exe" {
		t.Fatalf("Windows public path = %s", got)
	}
	pointer := filepath.Join(root, managedWindowsCurrent)
	if err := os.WriteFile(pointer, []byte("old\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceTextPointer(pointer, "new-generation", "windows"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(pointer); err != nil || string(data) != "new-generation\r\n" {
		t.Fatalf("Windows text pointer = %q, %v", data, err)
	}
}

func TestArchivePreservesFlatUpgraderWithoutInstallingStandaloneLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executables are POSIX scripts; Windows manifests are tested separately")
	}
	for _, tamper := range []bool{false, true} {
		t.Run(fmt.Sprintf("tamper=%v", tamper), func(t *testing.T) {
			stage := t.TempDir()
			wrapper := "dws-v1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH
			root, identity := testPackage(t, filepath.Join(stage, wrapper), "v1.2.3", strings.Repeat("a", 40), true)
			body, err := os.ReadFile(filepath.Join(root, "libexec", "dws-core"))
			if err != nil {
				t.Fatal(err)
			}
			if tamper {
				body = []byte("#!/bin/sh\nexit 99\n")
			}
			if err := os.WriteFile(filepath.Join(stage, "dws"), body, 0o755); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "package.tar.gz")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			tarWriter := tar.NewWriter(gz)
			err = filepath.Walk(stage, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || path == stage {
					return walkErr
				}
				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(stage, path)
				if err != nil {
					return err
				}
				header.Name = filepath.ToSlash(rel)
				if err := tarWriter.WriteHeader(header); err != nil || info.IsDir() {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				_, err = tarWriter.Write(data)
				return err
			})
			for _, closeErr := range []error{tarWriter.Close(), gz.Close(), file.Close()} {
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			extracted := t.TempDir()
			_, _, err = ExtractAndVerifyPackage(archive, extracted, identity)
			if tamper {
				if err == nil || !strings.Contains(err.Error(), "legacy migration entry") {
					t.Fatalf("tampered migration entry was not rejected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// Shipped FindBinaryInDir prefers <extract>/dws, then copies only
			// that file to the install path. No package siblings survive.
			flat := filepath.Join(t.TempDir(), "dws")
			if err := copyFile(filepath.Join(extracted, "dws"), flat, 0o755); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(flat, "--version").CombinedOutput()
			if err != nil || !strings.Contains(string(out), "v1.2.3") {
				t.Fatalf("flat upgrader installed an unusable executable: %v, %s", err, out)
			}
		})
	}
}

func TestCustomInstallationMetadataLocatesExactLauncher(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, managedVersionsDir, "v1", "bin", "dws")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(t.TempDir(), "custom-dws")
	if err := os.Symlink(launcher, public); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf("{\"format_version\":1,\"public_launcher\":%q,\"platform\":\"unix\"}\n", public)
	if err := os.WriteFile(filepath.Join(root, InstallationMetadataName), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := locatePublicLauncher(root, launcher, "linux")
	if err != nil || got != public {
		t.Fatalf("locatePublicLauncher = %q, %v; want %q", got, err, public)
	}

	legacyRedundant := fmt.Sprintf("{\"format_version\":1,\"public_launcher\":%q,\"platform\":\"unix\",\"cli_root\":%q}\n", public, root)
	if err := os.WriteFile(filepath.Join(root, InstallationMetadataName), []byte(legacyRedundant), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := locatePublicLauncher(root, launcher, "linux"); err == nil {
		t.Fatal("redundant cli_root metadata field was accepted")
	}
	wrongPlatform := strings.Replace(metadata, `"platform":"unix"`, `"platform":"windows"`, 1)
	if err := os.WriteFile(filepath.Join(root, InstallationMetadataName), []byte(wrongPlatform), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := locatePublicLauncher(root, launcher, "linux"); err == nil {
		t.Fatal("mismatched metadata platform was accepted")
	}
}

func TestCustomWindowsInstallationMetadataRequiresExactShim(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	public := filepath.Join(t.TempDir(), "acme-dws.cmd")
	shim := "@echo off\r\nsetlocal\r\nset \"DWS_ROOT=" + root + "\"\r\nset /p DWS_PACKAGE=<\"%DWS_ROOT%\\current.txt\"\r\n\"%DWS_ROOT%\\versions\\%DWS_PACKAGE%\\bin\\dws.exe\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	if err := os.WriteFile(public, []byte(shim), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf("{\"format_version\":1,\"public_launcher\":%q,\"platform\":\"windows\"}\n", public)
	if err := os.WriteFile(filepath.Join(root, InstallationMetadataName), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := locatePublicLauncher(root, "unused", "windows"); err != nil || got != public {
		t.Fatalf("custom Windows metadata = %q, %v", got, err)
	}
	if err := os.WriteFile(public, []byte(strings.Replace(shim, root, "%~dp0.dws", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := locatePublicLauncher(root, "unused", "windows"); err == nil {
		t.Fatal("default-root shim was accepted for a custom CLI root")
	}
}

func TestDefaultWindowsInstallationMetadataAcceptsRelativeRootShim(t *testing.T) {
	installDir := t.TempDir()
	root := filepath.Join(installDir, ".dws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(installDir, "dws.cmd")
	shim := "@echo off\r\nsetlocal\r\nset \"DWS_ROOT=%~dp0.dws\"\r\nset /p DWS_PACKAGE=<\"%DWS_ROOT%\\current.txt\"\r\n\"%DWS_ROOT%\\versions\\%DWS_PACKAGE%\\bin\\dws.exe\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	if err := os.WriteFile(public, []byte(shim), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf("{\"format_version\":1,\"public_launcher\":%q,\"platform\":\"windows\"}\n", public)
	if err := os.WriteFile(filepath.Join(root, InstallationMetadataName), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := locatePublicLauncher(root, "unused", "windows"); err != nil || got != public {
		t.Fatalf("default Windows metadata = %q, %v", got, err)
	}
}

func TestPackageManagerLayoutsAreRejected(t *testing.T) {
	for manager, path := range map[string]string{
		"npm":      filepath.Join(t.TempDir(), "lib", "node_modules", "dingtalk-workspace-cli", "libexec", "dws-core"),
		"homebrew": filepath.Join(t.TempDir(), "Cellar", "dingtalk-workspace-cli", "1.2.3", "libexec", "dws-core"),
	} {
		if got := packageManagerForPath(path); got != manager {
			t.Fatalf("packageManagerForPath(%q) = %q, want %q", path, got, manager)
		}
		if err := packageManagerUpgradeError(manager); err == nil || !strings.Contains(strings.ToLower(err.Error()), manager) {
			t.Fatalf("guidance for %s = %v", manager, err)
		}
	}
}

func TestStrictZipRejectsExcessiveEntryCount(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "many.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for index := 0; index <= maxArchiveEntries; index++ {
		if _, err := writer.Create(fmt.Sprintf("entry-%05d/", index)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractStrictZip(archive, t.TempDir()); err == nil {
		t.Fatal("archive entry-count limit was not enforced")
	}
}

func testPackage(t *testing.T, root, version, commit string, failLauncher bool) (string, packagemanifest.Identity) {
	t.Helper()
	return testPackageForTarget(t, root, version, commit, failLauncher, packagemanifest.Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
}

func testPackageForTarget(t *testing.T, root, version, commit string, failLauncher bool, target packagemanifest.Target) (string, packagemanifest.Identity) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "libexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherRel, coreRel, err := packagemanifest.Paths(target)
	if err != nil {
		t.Fatal(err)
	}
	launcherBody := "#!/bin/sh\necho 'dws version " + version + "'\n"
	if failLauncher {
		launcherBody = "#!/bin/sh\nexit 1\n"
	}
	coreBody := "#!/bin/sh\necho 'dws version " + version + "'\n"
	mode := os.FileMode(0o755)
	if target.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(launcherRel)), []byte(launcherBody), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(coreRel)), []byte(coreBody), mode); err != nil {
		t.Fatal(err)
	}
	identity := packagemanifest.Identity{Release: packagemanifest.Release{Version: version, Commit: commit, Edition: "open"}, Target: target}
	manifest, err := packagemanifest.Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := packagemanifest.WriteAtomic(root, manifest); err != nil {
		t.Fatal(err)
	}
	return root, identity
}

func mustManifest(t *testing.T, root string) packagemanifest.Manifest {
	t.Helper()
	manifest, err := decodeManifest(filepath.Join(root, packagemanifest.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
