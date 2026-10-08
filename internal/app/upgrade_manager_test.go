// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

const managedPackageManifest = `{"name":"dingtalk-workspace-cli","bin":{"dws":"./bin/dws.js"},"version":"9.9.9"}`

func TestCrossPlatformCoverageUpgradeVersionSubprocessFailure(t *testing.T) {
	if _, err := tryExecVersion(filepath.Join(t.TempDir(), "missing-dws")); err == nil {
		t.Fatal("新二进制无法启动时应返回验证错误")
	}
}

func TestCrossPlatformCoverageUpgradeInstallation(t *testing.T) {
	for _, tc := range []struct {
		name, binary, manager, root, goos, dependency string
		global, fail                                  bool
		files                                         map[string]string
	}{
		{name: "standalone", binary: "/usr/local/bin/dws"},
		{name: "npm-global", binary: "/prefix/lib/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "npm", root: "/prefix", global: true},
		{name: "manifest-missing", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{}},
		{name: "manifest-invalid", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{"/x/node_modules/dingtalk-workspace-cli/package.json": "{}"}},
		{name: "nonstandard", binary: "/x/dingtalk-workspace-cli/vendor/dws", fail: true},
		{name: "npm-local", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "npm", root: "/x", dependency: "prod", files: map[string]string{"/x/package.json": `{"dependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "npm-dev", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "npm", root: "/x", dependency: "dev", files: map[string]string{"/x/package.json": `{"devDependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "pnpm-optional", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "pnpm", root: "/x", dependency: "optional", files: map[string]string{"/x/package.json": `{"packageManager":"pnpm@10","optionalDependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "pnpm-yaml", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "pnpm", root: "/x", dependency: "prod", files: map[string]string{"/x/node_modules/.modules.yaml": "packageManager: pnpm@10", "/x/package.json": `{"dependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "pnpm-store", binary: "/p/global/5/node_modules/.pnpm/dingtalk-workspace-cli@1/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "pnpm", root: "/p/global/5", global: true},
		{name: "pnpm-unknown-global", binary: "/p/global/v11/hash/node_modules/.pnpm/dingtalk-workspace-cli@1/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true},
		{name: "pnpm-local-store", binary: "/x/node_modules/.pnpm/dingtalk-workspace-cli@1/node_modules/dingtalk-workspace-cli/vendor/dws", manager: "pnpm", root: "/x", dependency: "prod", files: map[string]string{"/x/package.json": `{"dependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "competing-lock", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{"/x/package.json": `{}`, "/x/yarn.lock": "lock", "/x/package-lock.json": "lock"}},
		{name: "yarn", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{"/x/package.json": `{"packageManager":"yarn@4"}`}},
		{name: "transitive", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{"/x/package.json": "{}"}},
		{name: "no-manager-proof", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true, files: map[string]string{"/x/package.json": `{"dependencies":{"dingtalk-workspace-cli":"1"}}`}},
		{name: "ambiguous", binary: "/x/node_modules/dingtalk-workspace-cli/vendor/dws", fail: true},
		{name: "windows-global", binary: "/win/node_modules/dingtalk-workspace-cli/vendor/dws.exe", manager: "npm", root: "/win", goos: "windows", global: true, files: map[string]string{"/win/dws.cmd": "npm shim"}},
		{name: "windows-ambiguous", binary: "/win/node_modules/dingtalk-workspace-cli/vendor/dws.exe", goos: "windows", fail: true},
		{name: "brew", binary: "/brew/Cellar/dingtalk-workspace-cli/1/libexec/dws", manager: "brew", root: "/brew/Cellar/dingtalk-workspace-cli/1", files: map[string]string{"/brew/Cellar/dingtalk-workspace-cli/1/INSTALL_RECEIPT.json": `{"source":{"tap":"open-dingtalk/tap"}}`}},
		{name: "brew-beta", binary: "/brew/Cellar/dingtalk-workspace-cli-beta/1/libexec/dws", manager: "brew", root: "/brew/Cellar/dingtalk-workspace-cli-beta/1", files: map[string]string{"/brew/Cellar/dingtalk-workspace-cli-beta/1/INSTALL_RECEIPT.json": `{"source":{"tap":"open-dingtalk/tap"}}`}},
		{name: "brew-unproven", binary: "/brew/Cellar/dingtalk-workspace-cli/1/libexec/dws", fail: true},
		{name: "brew-other", binary: "/brew/Cellar/other/1/bin/dws", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			if tc.name == "npm-local" || tc.name == "npm-dev" || tc.name == "transitive" {
				files["/x/package-lock.json"] = "{}"
			}
			if tc.name != "manifest-missing" {
				files[filepath.Join(filepath.Dir(filepath.Dir(tc.binary)), "package.json")] = managedPackageManifest
			}
			for k, v := range tc.files {
				files[k] = v
			}
			testseam.Swap(t, &upgradeManagerReadFile, func(path string) ([]byte, error) {
				if data, ok := files[path]; ok {
					return []byte(data), nil
				}
				return nil, os.ErrNotExist
			})
			goos := tc.goos
			if goos == "" {
				goos = "linux"
			}
			testseam.Swap(t, &upgradeRuntimeGOOS, goos)
			got, err := inspectUpgradeInstallation(tc.binary)
			if (err != nil) != tc.fail {
				t.Fatalf("install=%+v err=%v", got, err)
			}
			if !tc.fail && (got.manager != tc.manager || got.root != tc.root || got.global != tc.global || got.dependency != tc.dependency) {
				t.Fatalf("install=%+v", got)
			}
		})
	}
	for _, tap := range []string{"", "a", "/b", "-a/b", "a/b?"} {
		if validUpgradeTap(tap) {
			t.Fatalf("accepted %q", tap)
		}
	}
	if !validUpgradeTap("A_b.c/t-ap") {
		t.Fatal("valid tap")
	}
	for _, stage := range []string{"executable", "symlink", "success"} {
		t.Run(stage, func(t *testing.T) {
			testseam.Swap(t, &upgradeExecutable, func() (string, error) {
				if stage == "executable" {
					return "", os.ErrPermission
				}
				return "/usr/local/bin/dws", nil
			})
			testseam.Swap(t, &upgradeEvalSymlinks, func(p string) (string, error) {
				if stage == "symlink" {
					return "", os.ErrNotExist
				}
				return p, nil
			})
			_, err := currentUpgradeInstallation()
			if (err != nil) != (stage != "success") {
				t.Fatal(err)
			}
		})
	}
}

func testManagedRelease() *upgrade.ReleaseInfo {
	return &upgrade.ReleaseInfo{Version: "9.9.9", NPM: &upgrade.NPMPackage{Name: upgradeNPMPackageName, Version: "9.9.9", RegistryURL: "https://registry.example/npm"}}
}

func TestCrossPlatformCoverageManagedUpgradeArgs(t *testing.T) {
	for _, tc := range []struct {
		manager, dependency                     string
		global, force, beta, skip, custom, fail bool
	}{
		{manager: "npm", global: true}, {manager: "npm"}, {manager: "pnpm", global: true}, {manager: "pnpm", dependency: "dev", force: true}, {manager: "pnpm", dependency: "optional"}, {manager: "pnpm", dependency: "prod"},
		{manager: "brew"}, {manager: "brew", force: true}, {manager: "brew", custom: true, fail: true}, {manager: "brew", beta: true, fail: true}, {manager: "npm", skip: true, fail: true}, {manager: "npm", custom: true, fail: true},
	} {
		t.Run(tc.manager+tc.dependency+string(rune('0'+boolInt(tc.force)+boolInt(tc.skip)*2+boolInt(tc.custom)*4+boolInt(tc.beta)*8+boolInt(tc.global)*16)), func(t *testing.T) {
			r := testManagedRelease()
			if tc.manager == "brew" {
				r.NPM.RegistryURL = "https://registry.npmjs.org"
			}
			if tc.custom {
				r.NPM = nil
			}
			if tc.beta {
				r.Prerelease = true
			}
			a, err := managedUpgradeArgs(upgradeInstallation{manager: tc.manager, root: "/install", formula: "org/tap/dingtalk-workspace-cli", global: tc.global, dependency: tc.dependency}, r, upgradeOptions{skipSkills: tc.skip, force: tc.force})
			if (err != nil) != tc.fail {
				t.Fatalf("%v %v", a, err)
			}
			if !tc.fail && tc.manager != "brew" && !strings.Contains(strings.Join(a, " "), "--registry https://registry.example/npm") {
				t.Fatal(a)
			}
		})
	}
	if upgradeOutputHasVersion("11.0.63", "1.0.63") || !upgradeOutputHasVersion("Version: v1.0.63\nBuild: x", "1.0.63") {
		t.Fatal("version token comparison")
	}
	t.Setenv("DWS_NO_UPDATE_CHECK", "0")
	t.Setenv("HOMEBREW_NO_AUTO_UPDATE", "0")
	cmd := upgradeChildCommand(context.Background(), "example", "version")
	if strings.Count(strings.Join(cmd.Env, "\n"), "DWS_NO_UPDATE_CHECK=") != 1 || !strings.Contains(strings.Join(cmd.Env, "\n"), "DWS_NO_UPDATE_CHECK=1") {
		t.Fatal("child version checks not disabled")
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestCrossPlatformCoverageManagedUpgradeVerification(t *testing.T) {
	for _, stage := range []string{"brew-cellar-error", "brew-update-error", "brew-info-error", "brew-version-mismatch", "brew-ok", "pnpm-error", "pnpm-mismatch", "pnpm-ok", "pnpm-global", "npm"} {
		t.Run(stage, func(t *testing.T) {
			manager := strings.Split(stage, "-")[0]
			install := upgradeInstallation{manager: manager, root: "/brew/Cellar/dingtalk-workspace-cli/1", formula: "org/tap/dingtalk-workspace-cli", global: stage == "pnpm-global"}
			testseam.Swap(t, &upgradeEvalSymlinks, func(p string) (string, error) { return p, nil })
			testseam.Swap(t, &upgradeManagedOutput, func(cmd *exec.Cmd) ([]byte, error) {
				if stage == "brew-cellar-error" || stage == "pnpm-error" {
					return nil, os.ErrPermission
				}
				if cmd.Args[1] == "--cellar" {
					return []byte(filepath.Dir(install.root)), nil
				}
				if cmd.Args[1] == "update" && stage != "brew-update-error" {
					return nil, nil
				}
				if stage == "brew-info-error" || stage == "brew-update-error" {
					return nil, os.ErrPermission
				}
				if stage == "brew-version-mismatch" {
					return []byte(`{"formulae":[{"full_name":"org/tap/dingtalk-workspace-cli","versions":{"stable":"8.0.0"}}]}`), nil
				}
				if manager == "brew" {
					return []byte(`{"formulae":[{"full_name":"org/tap/dingtalk-workspace-cli","versions":{"stable":"9.9.9"}}]}`), nil
				}
				if stage == "pnpm-mismatch" {
					return []byte("/other/node_modules"), nil
				}
				return []byte(filepath.Join(install.root, "node_modules")), nil
			})
			err := verifyManagedUpgrade(context.Background(), "/bin/manager", install, testManagedRelease())
			fail := strings.Contains(stage, "error") || strings.Contains(stage, "mismatch")
			if (err != nil) != fail {
				t.Fatal(err)
			}
		})
	}
	for _, stage := range []string{"brew-prefix-error", "brew-wrong-version", "brew-ok", "npm-global", "pnpm-local", "manifest-error", "binary-error"} {
		t.Run(stage, func(t *testing.T) {
			manager := "npm"
			if strings.HasPrefix(stage, "brew") {
				manager = "brew"
			}
			if stage == "pnpm-local" {
				manager = "pnpm"
			}
			install := upgradeInstallation{manager: manager, root: "/install", packageDir: "/install/lib/node_modules/dingtalk-workspace-cli", global: stage == "npm-global"}
			testseam.Swap(t, &upgradeManagedOutput, func(*exec.Cmd) ([]byte, error) {
				if stage == "brew-prefix-error" {
					return nil, os.ErrPermission
				}
				return []byte("/brew/opt/dws"), nil
			})
			testseam.Swap(t, &upgradeTryExecVersion, func(string) ([]byte, error) {
				if stage == "binary-error" {
					return nil, os.ErrPermission
				}
				if stage == "brew-wrong-version" {
					return []byte("19.9.9"), nil
				}
				return []byte("Version: v9.9.9"), nil
			})
			testseam.Swap(t, &upgradeManagerReadFile, func(path string) ([]byte, error) {
				if stage == "manifest-error" {
					return nil, os.ErrPermission
				}
				want := filepath.Join(install.root, "node_modules", upgradeNPMPackageName, "package.json")
				if install.global {
					want = filepath.Join(install.packageDir, "package.json")
				}
				if path != want {
					t.Fatalf("wrong install: %s", path)
				}
				return []byte(managedPackageManifest), nil
			})
			err := verifyManagedInstalled(context.Background(), "/bin/manager", install, "9.9.9")
			fail := strings.Contains(stage, "error") || strings.Contains(stage, "wrong")
			if (err != nil) != fail {
				t.Fatal(err)
			}
		})
	}
}

func TestCrossPlatformCoverageManagedUpgradeExecution(t *testing.T) {
	for _, stage := range []string{"skip", "preview", "brew-preview", "windows", "missing", "preflight", "cancel", "confirm", "execute-error", "postflight", "success", "rebuild", "rebuild-error"} {
		t.Run(stage, func(t *testing.T) {
			install := upgradeInstallation{manager: "npm", root: "/install", packageDir: "/install/lib/node_modules/dingtalk-workspace-cli", global: true}
			testseam.Swap(t, &version, "9.9.9")
			if stage == "preflight" {
				install.manager = "pnpm"
			}
			if stage == "brew-preview" {
				install.manager = "brew"
				install.formula = "org/tap/dingtalk-workspace-cli"
			}
			opts := upgradeOptions{yes: true, dryRun: stage == "preview" || stage == "brew-preview", skipSkills: stage == "skip", force: strings.HasPrefix(stage, "rebuild")}
			testseam.Swap(t, &upgradeRuntimeGOOS, "linux")
			if stage == "windows" {
				testseam.Swap(t, &upgradeRuntimeGOOS, "windows")
			}
			testseam.Swap(t, &upgradeLookPath, func(string) (string, error) {
				if stage == "missing" {
					return "", os.ErrNotExist
				}
				return "/bin/manager", nil
			})
			calls, invalidated := 0, 0
			testseam.Swap(t, &upgradeManagedOutput, func(cmd *exec.Cmd) ([]byte, error) {
				calls++
				if stage == "execute-error" || stage == "preflight" || stage == "rebuild-error" && cmd.Args[1] == "rebuild" {
					return nil, os.ErrPermission
				}
				return nil, nil
			})
			testseam.Swap(t, &upgradeTryExecVersion, func(string) ([]byte, error) { return []byte("Version: 9.9.9"), nil })
			testseam.Swap(t, &upgradeManagerReadFile, func(string) ([]byte, error) {
				if stage == "postflight" {
					return nil, os.ErrPermission
				}
				return []byte(managedPackageManifest), nil
			})
			testseam.Swap(t, &invalidateSchemaCacheAfterUpgrade, func() { invalidated++ })
			if stage == "cancel" || stage == "confirm" {
				opts.yes = false
				answer := "n\n"
				if stage == "confirm" {
					answer = "y\n"
				}
				upgradeTestStdin(t, answer)
			}
			release := testManagedRelease()
			if install.manager == "brew" {
				release.NPM.RegistryURL = "https://registry.npmjs.org"
			}
			err := runManagedUpgrade(context.Background(), install, release, opts)
			fail := stage == "skip" || stage == "windows" || stage == "missing" || stage == "preflight" || stage == "execute-error" || stage == "postflight" || stage == "rebuild-error"
			if (err != nil) != fail {
				t.Fatal(err)
			}
			if (stage == "preview" || stage == "brew-preview") && calls != 0 {
				t.Fatal("dry-run executed manager")
			}
			if (stage == "success" || stage == "confirm" || stage == "rebuild") && invalidated != 1 {
				t.Fatal("missing successful invalidation")
			}
		})
	}
}

func upgradeTestStdin(t *testing.T, value string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	testseam.Swap(t, &os.Stdin, f)
}

func TestCrossPlatformCoverageUpgradeManagedEntrypoints(t *testing.T) {
	for _, stage := range []string{"detect-error", "managed-preview", "native-preview", "rollback-managed", "rollback-error", "rollback-preview"} {
		t.Run(stage, func(t *testing.T) {
			install := upgradeInstallation{}
			if stage == "managed-preview" || stage == "rollback-managed" {
				install.manager = "npm"
				install.root = "/install"
			}
			testseam.Swap(t, &detectUpgradeInstallation, func() (upgradeInstallation, error) {
				if stage == "detect-error" || stage == "rollback-error" {
					return install, os.ErrPermission
				}
				return install, nil
			})
			r := testManagedRelease()
			r.Assets = []upgrade.GitHubAsset{{Name: "dws-darwin-arm64.tar.gz"}}
			testseam.Swap(t, &newUpgradeReleaseClient, func() upgradeReleaseClient { return &fakeUpgradeClient{latest: r} })
			testseam.Swap(t, &findUpgradeBinary, func([]upgrade.GitHubAsset) (*upgrade.GitHubAsset, error) { return &r.Assets[0], nil })
			testseam.Swap(t, &ensureUpgradeDirs, func() error { t.Fatal("preview wrote directories"); return nil })
			testseam.Swap(t, &cleanupUpgradeStale, func() { t.Fatal("preview cleaned directories") })
			testseam.Swap(t, &upgradeManagedOutput, func(*exec.Cmd) ([]byte, error) { t.Fatal("preview ran manager"); return nil, nil })
			testseam.Swap(t, &newUpgradeRollback, func() upgradeRollbackManager {
				return &fakeUpgradeRollback{backups: []upgrade.BackupInfo{{Version: "1.0.0"}}}
			})
			var err error
			if strings.HasPrefix(stage, "rollback") {
				err = runUpgradeRollbackPreview(true, true)
			} else {
				err = runUpgrade(context.Background(), upgradeOptions{yes: true, dryRun: true, force: true})
			}
			fail := stage == "detect-error" || stage == "rollback-error" || stage == "rollback-managed"
			if (err != nil) != fail {
				t.Fatal(err)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeNativeNPM(t *testing.T) {
	for _, stage := range []string{"prepare-error", "version-error", "wrong-version", "success", "skip-skills"} {
		t.Run(stage, func(t *testing.T) {
			r := testManagedRelease()
			binary := upgrade.GitHubAsset{Name: "dws.zip"}
			skills := upgrade.GitHubAsset{Name: "dws-skills.zip"}
			r.Assets = []upgrade.GitHubAsset{binary, skills}
			testseam.Swap(t, &detectUpgradeInstallation, func() (upgradeInstallation, error) { return upgradeInstallation{}, nil })
			testseam.Swap(t, &newUpgradeReleaseClient, func() upgradeReleaseClient { return &fakeUpgradeClient{latest: r} })
			testseam.Swap(t, &newUpgradeRollback, func() upgradeRollbackManager { return &fakeUpgradeRollback{} })
			testseam.Swap(t, &ensureUpgradeDirs, func() error { return nil })
			testseam.Swap(t, &cleanupUpgradeStale, func() {})
			testseam.Swap(t, &findUpgradeBinary, func([]upgrade.GitHubAsset) (*upgrade.GitHubAsset, error) { return &binary, nil })
			testseam.Swap(t, &findUpgradeSkills, func([]upgrade.GitHubAsset) *upgrade.GitHubAsset { return &skills })
			testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return "/controlled/cache", nil })
			testseam.Swap(t, &upgradeRemoveAll, func(string) error { return nil })
			testseam.Swap(t, &downloadUpgradeFile, func(string, string) (int64, error) { t.Fatal("npm path downloaded github asset"); return 0, nil })
			testseam.Swap(t, &prepareUpgradeNPMPackage, func(_ context.Context, p upgrade.NPMPackage, _ string, skip bool, progress func(float64, int64, int64)) (upgrade.PreparedNPMPackage, error) {
				progress(50, 1024, 2048)
				if p.Name != upgradeNPMPackageName || skip != (stage == "skip-skills") {
					t.Fatalf("prepare args: %+v %v", p, skip)
				}
				if stage == "prepare-error" {
					return upgrade.PreparedNPMPackage{}, errors.New("integrity mismatch")
				}
				return upgrade.PreparedNPMPackage{BinaryArchivePath: "/controlled/binary.zip", SkillsArchivePath: "/controlled/skills.zip", ChecksumsContent: "verified checksums"}, nil
			})
			testseam.Swap(t, &verifyUpgradeFile, func(_, _, _, _, checksums string) error {
				if checksums != "verified checksums" {
					t.Fatal(checksums)
				}
				return nil
			})
			testseam.Swap(t, &extractUpgradeZip, func(string, string) error { return nil })
			testseam.Swap(t, &findExtractedBinary, func(string) string { return "/controlled/dws" })
			testseam.Swap(t, &validateUpgradeBinary, func(string, string) error { return nil })
			testseam.Swap(t, &upgradeTryExecVersion, func(string) ([]byte, error) {
				if stage == "version-error" {
					return nil, os.ErrPermission
				}
				if stage == "wrong-version" {
					return []byte("19.9.9"), nil
				}
				return []byte("Version: 9.9.9"), nil
			})
			testseam.Swap(t, &upgradeMkdirAll, func(string, os.FileMode) error { return nil })
			testseam.Swap(t, &locateUpgradeSkill, func(string) string { return "/controlled/skills" })
			replaced := 0
			testseam.Swap(t, &replaceUpgradeSelf, func(string) error { replaced++; return nil })
			testseam.Swap(t, &invalidateSchemaCacheAfterUpgrade, func() {})
			testseam.Swap(t, &installUpgradeSkills, func(string, upgrade.SkillUpgradeOptions) (*upgrade.SkillUpgradeResult, error) {
				if stage == "skip-skills" {
					t.Fatal("skip-skills installed skills")
				}
				return &upgrade.SkillUpgradeResult{}, nil
			})
			err := runUpgrade(context.Background(), upgradeOptions{yes: true, force: true, skipSkills: stage == "skip-skills"})
			fail := stage == "prepare-error" || stage == "version-error" || stage == "wrong-version"
			if (err != nil) != fail || replaced != boolInt(!fail) {
				t.Fatalf("err=%v replaced=%d", err, replaced)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeManagerDefaultRunner(t *testing.T) {
	// 启动失败也必须通过真实默认执行器返回，不能回落到原生替换。
	if _, err := upgradeManagedOutput(upgradeChildCommand(context.Background(), filepath.Join(t.TempDir(), "missing-manager"))); err == nil {
		t.Fatal("missing manager unexpectedly ran")
	}
}

func TestCrossPlatformCoverageUpgradeManagerDisplay(t *testing.T) {
	for _, tc := range []struct{ goos, want string }{
		{"windows", `npm '--prefix' 'C:\Program Files\Raph''s'`},
		{"linux", `npm '--prefix' 'C:\Program Files\Raph'"'"'s'`},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			testseam.Swap(t, &upgradeRuntimeGOOS, tc.goos)
			if got := displayUpgradeCommand("npm", []string{"--prefix", `C:\Program Files\Raph's`}); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
