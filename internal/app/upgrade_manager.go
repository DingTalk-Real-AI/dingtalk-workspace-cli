// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

const upgradeNPMPackageName = "dingtalk-workspace-cli"

// manager 为空代表独立二进制。无法证明归属的托管路径必须报错，不能降级为原生覆盖。
type upgradeInstallation struct {
	manager    string
	root       string
	packageDir string
	executable string
	global     bool
	formula    string
	dependency string
}

var (
	detectUpgradeInstallation = currentUpgradeInstallation
	upgradeExecutable         = os.Executable
	upgradeEvalSymlinks       = filepath.EvalSymlinks
	upgradeManagerReadFile    = os.ReadFile
	upgradeManagedOutput      = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }
)

func currentUpgradeInstallation() (upgradeInstallation, error) {
	executable, err := upgradeExecutable()
	if err != nil {
		return upgradeInstallation{}, fmt.Errorf("无法识别当前安装路径: %w", err)
	}
	executable, err = upgradeEvalSymlinks(executable)
	if err != nil {
		return upgradeInstallation{}, fmt.Errorf("无法解析当前安装路径: %w", err)
	}
	return inspectUpgradeInstallation(executable)
}

func inspectUpgradeInstallation(executable string) (upgradeInstallation, error) {
	install := upgradeInstallation{executable: executable}
	// Homebrew 的入口常为 opt/bin 下的链接，必须先解析到实际 Cellar keg。
	for dir := filepath.Dir(executable); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(filepath.Dir(filepath.Dir(dir))) != "Cellar" {
			continue
		}
		formula := filepath.Base(filepath.Dir(dir))
		if formula != upgradeNPMPackageName && formula != upgradeNPMPackageName+"-beta" {
			return install, fmt.Errorf("当前二进制由 Homebrew formula %s 管理，请通过原 formula 升级", formula)
		}
		var receipt struct {
			Source struct {
				Tap string `json:"tap"`
			} `json:"source"`
		}
		data, err := upgradeManagerReadFile(filepath.Join(dir, "INSTALL_RECEIPT.json"))
		if err != nil || json.Unmarshal(data, &receipt) != nil || !validUpgradeTap(receipt.Source.Tap) {
			return install, fmt.Errorf("无法核实 Homebrew 安装收据，请通过 brew 升级 %s", formula)
		}
		install.manager, install.root = "brew", dir
		install.formula = receipt.Source.Tap + "/" + formula
		return install, nil
	}
	packageDir := filepath.Dir(filepath.Dir(executable))
	if filepath.Base(filepath.Dir(executable)) != "vendor" || filepath.Base(packageDir) != upgradeNPMPackageName {
		return install, nil
	}
	var manifest struct {
		Name string            `json:"name"`
		Bin  map[string]string `json:"bin"`
	}
	data, err := upgradeManagerReadFile(filepath.Join(packageDir, "package.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.Name != upgradeNPMPackageName || strings.TrimPrefix(manifest.Bin["dws"], "./") != "bin/dws.js" {
		return install, fmt.Errorf("无法核实 npm 安装清单，请使用原包管理器重新安装 %s", upgradeNPMPackageName)
	}
	install.packageDir = packageDir
	modules := filepath.Dir(packageDir)
	if filepath.Base(modules) != "node_modules" {
		return install, fmt.Errorf("npm 包不在标准 node_modules 路径，请使用原包管理器升级")
	}
	root := filepath.Dir(modules)
	install.manager = "npm"
	if filepath.Base(filepath.Dir(root)) == ".pnpm" {
		install.manager = "pnpm"
		modules = filepath.Dir(filepath.Dir(root))
		root = filepath.Dir(modules)
	} else if data, err := upgradeManagerReadFile(filepath.Join(modules, ".modules.yaml")); err == nil && strings.Contains(string(data), "packageManager: pnpm@") {
		install.manager = "pnpm"
	}
	install.root = root
	if install.manager == "npm" && filepath.Base(root) == "lib" {
		install.root, install.global = filepath.Dir(root), true
		return install, nil
	}
	if install.manager == "pnpm" && filepath.Base(filepath.Dir(root)) == "global" {
		install.global = true
		return install, nil
	}
	if install.manager == "pnpm" {
		for dir := root; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if filepath.Base(dir) == "global" {
				return install, fmt.Errorf("当前 pnpm 全局存储布局无法安全定位，请退出 dws 后在原 pnpm 环境执行 pnpm add --global dingtalk-workspace-cli@latest")
			}
		}
	}
	// 项目局部安装必须是该项目的直接依赖，不能把传递依赖误加到另一项目。
	var project struct {
		PackageManager       string            `json:"packageManager"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	data, err = upgradeManagerReadFile(filepath.Join(root, "package.json"))
	if err == nil && json.Unmarshal(data, &project) == nil {
		if strings.HasPrefix(project.PackageManager, "pnpm@") {
			install.manager = "pnpm"
		}
		if project.PackageManager != "" && !strings.HasPrefix(project.PackageManager, install.manager+"@") {
			return install, fmt.Errorf("当前项目由 %s 管理，请通过该管理器升级", project.PackageManager)
		}
		if install.manager == "npm" && project.PackageManager == "" {
			for _, lock := range []string{"yarn.lock", "bun.lock", "bun.lockb", "pnpm-lock.yaml"} {
				if _, err := upgradeManagerReadFile(filepath.Join(root, lock)); err == nil {
					return install, fmt.Errorf("项目存在 %s，无法确认由 npm 管理，请通过原项目包管理器升级", lock)
				}
			}
			if _, err := upgradeManagerReadFile(filepath.Join(root, "package-lock.json")); err != nil {
				return install, fmt.Errorf("当前项目未声明 npm 且没有 package-lock.json，请在原项目中使用原包管理器升级")
			}
		}
		switch {
		case project.OptionalDependencies[upgradeNPMPackageName] != "":
			install.dependency = "optional"
		case project.DevDependencies[upgradeNPMPackageName] != "":
			install.dependency = "dev"
		case project.Dependencies[upgradeNPMPackageName] != "":
			install.dependency = "prod"
		default:
			return install, fmt.Errorf("当前 DWS 不是项目的直接依赖，请通过原项目依赖管理器升级")
		}
		return install, nil
	}
	// Windows npm 全局目录没有 lib 层；同目录必须存在 npm 创建的命令 shim。
	if upgradeRuntimeGOOS == "windows" {
		if _, err := upgradeManagerReadFile(filepath.Join(root, "dws.cmd")); err == nil {
			install.global = true
			return install, nil
		}
	}
	return install, fmt.Errorf("无法确认 npm/pnpm 的安装范围，请通过原安装目录的包管理器升级")
}

func validUpgradeTap(tap string) bool {
	parts := strings.Split(tap, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, "-") {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
				return false
			}
		}
	}
	return true
}

func managedUpgradeArgs(install upgradeInstallation, release *upgrade.ReleaseInfo, opts upgradeOptions) ([]string, error) {
	if opts.skipSkills {
		return nil, fmt.Errorf("%s 安装器不支持 --skip-skills；请移除此选项，技能包将由原包管理器维护", install.manager)
	}
	if install.manager == "brew" {
		if release.NPM == nil || strings.TrimRight(release.NPM.RegistryURL, "/") != "https://registry.npmjs.org" {
			return nil, fmt.Errorf("Homebrew 只能从原 formula 更新，不能应用自定义发布源；请核对来源后使用 brew upgrade %s", install.formula)
		}
		beta := strings.HasSuffix(install.formula, "-beta")
		if beta != release.Prerelease {
			return nil, fmt.Errorf("当前 Homebrew formula %s 与目标版本轨道不一致；请通过 Homebrew 显式切换 formula", install.formula)
		}
		action := "upgrade"
		if opts.force {
			action = "reinstall"
		}
		return []string{action, install.formula}, nil
	}
	if release.NPM == nil || release.NPM.Name != upgradeNPMPackageName || release.NPM.RegistryURL == "" || release.NPM.Version != release.Version {
		return nil, fmt.Errorf("npm/pnpm 安装不能使用其他发布源自更新；请通过原包管理器升级")
	}
	pkg := upgradeNPMPackageName + "@" + release.NPM.Version
	if install.manager == "npm" {
		args := []string{"install", "--prefix", install.root, "--registry", release.NPM.RegistryURL}
		if install.global {
			args = append(args, "--global")
		}
		if opts.force {
			args = append(args, "--force")
		}
		return append(args, pkg), nil
	}
	args := []string{"add", "--registry", release.NPM.RegistryURL}
	if opts.force {
		args = append(args, "--force")
	}
	if install.global {
		args = append(args, "--global")
	} else {
		args = append(args, "--dir", install.root)
		if install.dependency == "dev" {
			args = append(args, "--save-dev")
		}
		if install.dependency == "optional" {
			args = append(args, "--save-optional")
		}
	}
	return append(args, pkg), nil
}

func upgradeChildCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	// 验证/安装器子进程不得再次触发版本请求。
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if !strings.EqualFold(key, "DWS_NO_UPDATE_CHECK") && !strings.EqualFold(key, "HOMEBREW_NO_AUTO_UPDATE") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "DWS_NO_UPDATE_CHECK=1", "HOMEBREW_NO_AUTO_UPDATE=1")
	return cmd
}

func runManagedUpgrade(ctx context.Context, install upgradeInstallation, release *upgrade.ReleaseInfo, opts upgradeOptions) error {
	args, err := managedUpgradeArgs(install, release, opts)
	if err != nil {
		return err
	}
	fmt.Printf("  安装方式: %s\n  将执行: %s\n", install.manager, displayUpgradeCommand(install.manager, args))
	if install.manager == "brew" {
		fmt.Println("  升级前将执行 brew update，核对 formula 目标版本。")
	}
	rebuild := opts.force && install.manager == "npm" && strings.TrimPrefix(version, "v") == strings.TrimPrefix(release.Version, "v")
	if rebuild {
		fmt.Println("  同版本重装还将执行 npm rebuild，重新运行安装脚本。")
	}
	if opts.dryRun {
		fmt.Println("  [dry-run] 仅预览；实际升级前将核实包管理器的安装目录和目标版本。")
		return nil
	}
	// Windows 的 .cmd shim 需要 shell 解释；避免不安全的命令串及覆盖正在运行的 exe。
	if upgradeRuntimeGOOS == "windows" {
		return fmt.Errorf("Windows 包管理器安装请在当前 dws 进程退出后，在 PowerShell 中执行上述原包管理器命令升级")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	manager, err := upgradeLookPath(install.manager)
	if err != nil {
		return fmt.Errorf("找不到原包管理器 %s，请恢复该管理器后重试: %w", install.manager, err)
	}
	if !opts.yes {
		fmt.Print("是否通过原包管理器升级? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("已取消")
			return nil
		}
	}
	fmt.Printf("  核对 %s 安装位置和目标版本...\n", install.manager)
	if err := verifyManagedUpgrade(ctx, manager, install, release); err != nil {
		return err
	}
	fmt.Printf("  正在通过 %s 升级，请等待包管理器完成...\n", install.manager)
	cmd := upgradeChildCommand(ctx, manager, args...)
	cmd.Dir = install.root
	if _, err := upgradeManagedOutput(cmd); err != nil {
		return fmt.Errorf("%s 升级失败（未执行原生二进制替换）；请在原安装目录用上述命令检查: %w", install.manager, err)
	}
	if rebuild {
		args := []string{"rebuild", "--prefix", install.root}
		if install.global {
			args = append(args, "--global")
		}
		cmd := upgradeChildCommand(ctx, manager, append(args, upgradeNPMPackageName)...)
		cmd.Dir = install.root
		if _, err := upgradeManagedOutput(cmd); err != nil {
			return fmt.Errorf("npm 包已安装，但重装脚本失败，请在原安装目录执行 npm rebuild: %w", err)
		}
	}
	// pnpm 会切换实体包目录，brew 会切换 keg；通过原管理器再次定位而非执行旧 inode。
	if err := verifyManagedInstalled(ctx, manager, install, release.Version); err != nil {
		return err
	}
	invalidateSchemaCacheAfterUpgrade()
	fmt.Printf("  ✔ 已通过 %s 升级到 %s\n", install.manager, ensureV(release.Version))
	return nil
}

// 仅用于可复制的提示；实际执行始终使用 exec.Cmd 的参数数组。
func displayUpgradeCommand(manager string, args []string) string {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, manager)
	for _, arg := range args {
		if upgradeRuntimeGOOS == "windows" {
			arg = strings.ReplaceAll(arg, "'", "''")
		} else {
			arg = strings.ReplaceAll(arg, "'", "'\"'\"'")
		}
		quoted = append(quoted, "'"+arg+"'")
	}
	return strings.Join(quoted, " ")
}

func verifyManagedUpgrade(ctx context.Context, manager string, install upgradeInstallation, release *upgrade.ReleaseInfo) error {
	if install.manager == "brew" {
		out, err := upgradeManagedOutput(upgradeChildCommand(ctx, manager, "--cellar", install.formula))
		cellar, resolveErr := upgradeEvalSymlinks(strings.TrimSpace(string(out)))
		if err != nil || resolveErr != nil || cellar != filepath.Dir(install.root) {
			return fmt.Errorf("当前 brew 与运行中的 DWS 不属于同一 Cellar，请通过原 Homebrew 安装升级")
		}
		fmt.Println("  更新 Homebrew formula 索引...")
		if _, err := upgradeManagedOutput(upgradeChildCommand(ctx, manager, "update")); err != nil {
			return fmt.Errorf("更新 Homebrew formula 索引失败，未执行升级: %w", err)
		}
		out, err = upgradeManagedOutput(upgradeChildCommand(ctx, manager, "info", "--json=v2", install.formula))
		var info struct {
			Formulae []struct {
				FullName string `json:"full_name"`
				Versions struct {
					Stable string `json:"stable"`
				} `json:"versions"`
			} `json:"formulae"`
		}
		if err != nil || json.Unmarshal(out, &info) != nil || len(info.Formulae) != 1 || info.Formulae[0].FullName != install.formula || strings.TrimPrefix(info.Formulae[0].Versions.Stable, "v") != strings.TrimPrefix(release.Version, "v") {
			return fmt.Errorf("Homebrew formula %s 尚不能安装目标版本 %s；请先 brew update 并核对 formula，未执行升级", install.formula, release.Version)
		}
		return nil
	}
	if install.manager == "pnpm" {
		args := []string{"root", "--dir", install.root}
		if install.global {
			args = []string{"root", "--global"}
		}
		cmd := upgradeChildCommand(ctx, manager, args...)
		cmd.Dir = install.root
		out, err := upgradeManagedOutput(cmd)
		actual, resolveErr := upgradeEvalSymlinks(strings.TrimSpace(string(out)))
		expected, expectedErr := upgradeEvalSymlinks(filepath.Join(install.root, "node_modules"))
		if err != nil || resolveErr != nil || expectedErr != nil || actual != expected {
			return fmt.Errorf("pnpm 当前安装目录与运行中的 DWS 不一致，请通过原 pnpm 环境升级")
		}
	}
	return nil
}

func verifyManagedInstalled(ctx context.Context, manager string, install upgradeInstallation, target string) error {
	var manifestPath string
	if install.manager == "brew" {
		out, err := upgradeManagedOutput(upgradeChildCommand(ctx, manager, "--prefix", install.formula))
		if err != nil {
			return fmt.Errorf("Homebrew 已执行，但无法定位升级结果: %w", err)
		}
		out, err = upgradeTryExecVersion(filepath.Join(strings.TrimSpace(string(out)), "bin", "dws"))
		if err != nil || !upgradeOutputHasVersion(string(out), target) {
			return fmt.Errorf("Homebrew 已执行，但当前版本未确认为 %s，请检查原 formula", target)
		}
		return nil
	}
	if install.manager == "npm" && install.global {
		manifestPath = filepath.Join(install.packageDir, "package.json")
	} else {
		manifestPath = filepath.Join(install.root, "node_modules", upgradeNPMPackageName, "package.json")
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	data, err := upgradeManagerReadFile(manifestPath)
	if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.Name != upgradeNPMPackageName || strings.TrimPrefix(manifest.Version, "v") != strings.TrimPrefix(target, "v") {
		return fmt.Errorf("%s 已执行，但安装清单版本未确认为 %s，请检查原安装目录", install.manager, target)
	}
	out, err := upgradeTryExecVersion(filepath.Join(filepath.Dir(manifestPath), "vendor", "dws"))
	if err != nil || !upgradeOutputHasVersion(string(out), target) {
		return fmt.Errorf("%s 已执行，但二进制版本未确认为 %s，请检查安装脚本输出", install.manager, target)
	}
	return nil
}

func upgradeOutputHasVersion(out, expected string) bool {
	for _, word := range strings.Fields(out) {
		if strings.TrimPrefix(word, "v") == strings.TrimPrefix(expected, "v") {
			return true
		}
	}
	return false
}
