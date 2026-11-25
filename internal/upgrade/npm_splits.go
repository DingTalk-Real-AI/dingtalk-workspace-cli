// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const publicNPMMirror = "https://registry.npmmirror.com"

var publicSplitVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-beta\.[1-9][0-9]*)?$`)
var errNPMSplitUnavailable = errors.New("目标版本尚无 npm 平台分包")

var (
	npmSplitMkdirTemp = os.MkdirTemp
	npmSplitStat      = os.Stat
)

type npmSplitManifest struct {
	npmManifest
	Upgrade struct {
		Format  int    `json:"format"`
		Edition string `json:"edition"`
		Asset   string `json:"asset"`
		SHA256  string `json:"sha256"`
		Size    int64  `json:"size"`
	} `json:"dwsUpgrade"`
}

func npmSplitSuffix(asset string) (string, error) {
	if asset == skillsZipName {
		return "skills", nil
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			if asset != npmPackageBinaryName(goos, goarch) {
				continue
			}
			osName, arch := goos, goarch
			if osName == "windows" {
				osName = "win32"
			}
			if arch == "amd64" {
				arch = "x64"
			}
			return osName + "-" + arch, nil
		}
	}
	return "", fmt.Errorf("npm 分包不支持资源 %s", asset)
}

// The main package remains the release/withdrawal authority. Split packages
// never discover a newer version or substitute another version during failover.
func mirrorSplitClient(ctx context.Context, version string) (*Client, *ReleaseInfo, error) {
	if !publicUpgradeEdition(upgradeEditionName()) || !publicSplitVersion.MatchString(version) {
		return nil, nil, fmt.Errorf("npm 平台分包仅支持公共正式版和 beta 版")
	}
	client := &Client{registryURL: publicNPMMirror + "/" + npmPackageName, httpClient: &http.Client{Timeout: 8 * time.Second}}
	release, err := client.fetchRegistryRelease(ctx, version, ReleaseTrackAll)
	return client, release, err
}

func resolveMirrorSplitRelease(ctx context.Context, release *ReleaseInfo, skipSkills bool) (*ReleaseInfo, error) {
	client, _, err := mirrorSplitClient(ctx, release.Version)
	if err != nil {
		return nil, err
	}
	names := []string{npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)}
	if !skipSkills {
		names = append(names, skillsZipName)
	}
	result := *release
	result.Assets = nil
	for _, name := range names {
		asset, err := resolveNPMSplitAsset(ctx, client, release.Version, name)
		if err != nil {
			return nil, err
		}
		anchor := release.NPM.Upgrade
		expected, ok := anchor.Assets[name]
		if anchor.Format != 1 || anchor.Edition != "public" || !ok ||
			asset.Size != expected.Size || !strings.EqualFold(asset.Digest, "sha256:"+expected.SHA256) {
			return nil, fmt.Errorf("npm 分包缺少官方主包校验依据或与其不一致: %s", name)
		}
		result.Assets = append(result.Assets, asset)
	}
	return &result, nil
}

func resolveNPMSplitAsset(ctx context.Context, client *Client, version, name string) (GitHubAsset, error) {
	suffix, err := npmSplitSuffix(name)
	if err != nil {
		return GitHubAsset{}, err
	}
	packageName := npmPackageName + "-" + suffix
	var manifest npmSplitManifest
	if err := client.getRegistryJSON(ctx, client.registryURL+"-"+suffix+"/"+version, &manifest, registryManifestLimit); err != nil {
		var status *downloadHTTPError
		if errors.As(err, &status) && status.status == http.StatusNotFound {
			return GitHubAsset{}, errNPMSplitUnavailable
		}
		return GitHubAsset{}, err
	}
	if manifest.Name != packageName || manifest.Version != version || manifest.Deprecated != "" ||
		!validRegistryURL(manifest.Dist.Tarball) || !validNPMIntegrity(manifest.Dist.Integrity) ||
		manifest.Upgrade.Format != 1 || manifest.Upgrade.Edition != "public" || manifest.Upgrade.Asset != name {
		return GitHubAsset{}, fmt.Errorf("npm 分包身份或资源声明无效: %s", packageName)
	}
	asset := GitHubAsset{Name: name, Size: manifest.Upgrade.Size, Digest: "sha256:" + manifest.Upgrade.SHA256,
		NPM: &NPMPackage{RegistryURL: publicNPMMirror, Name: packageName, Version: version,
			TarballURL: manifest.Dist.Tarball, Integrity: manifest.Dist.Integrity}}
	if err := validatePlatformAsset(asset); err != nil {
		return GitHubAsset{}, err
	}
	return asset, nil
}

func canFallbackPlatform(err error) bool {
	return isRetriable(err) && !errors.Is(err, errRestartDownload)
}

// Only canonical public GitHub assets can use the public mirror. Explicit
// custom sources must never be replaced with another repository's package.
func mirrorForGitHubAsset(ctx context.Context, asset GitHubAsset) (GitHubAsset, error) {
	prefix := "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v"
	if !strings.HasPrefix(asset.BrowserDownloadURL, prefix) {
		return GitHubAsset{}, fmt.Errorf("非官方平台资产不能切换公共镜像")
	}
	version, name, ok := strings.Cut(strings.TrimPrefix(asset.BrowserDownloadURL, prefix), "/")
	if !ok || name != asset.Name {
		return GitHubAsset{}, fmt.Errorf("平台资产地址与文件名不一致")
	}
	client, _, err := mirrorSplitClient(ctx, version)
	if err != nil {
		return GitHubAsset{}, err
	}
	mirrored, err := resolveNPMSplitAsset(ctx, client, version, name)
	if err != nil {
		return GitHubAsset{}, err
	}
	if mirrored.Size != asset.Size || !strings.EqualFold(mirrored.Digest, asset.Digest) {
		return GitHubAsset{}, fmt.Errorf("npm 分包与 GitHub 平台资源不一致")
	}
	return mirrored, nil
}

// The outer npm tarball and inner release archive have distinct identities and
// partial files; a GitHub byte offset is never applied to npm package bytes.
func downloadNPMSplitAsset(ctx context.Context, asset GitHubAsset, dest string, progress func(float64, int64, int64)) error {
	pkg := *asset.NPM
	integrity, err := parseNPMPackageIntegrity(pkg.Integrity)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	stage, err := npmSplitMkdirTemp(filepath.Dir(dest), ".npm-split-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "package.tgz")
	cfg := DefaultDownloadConfig()
	cfg.MaxBytes, cfg.CheckRedirect = npmPackageMaxDownload, npmPackageRedirect
	cfg.ProgressCallback = func(n, total int64) {
		if progress != nil {
			percent := float64(0)
			if total > 0 {
				percent = float64(n) * 100 / float64(total)
			}
			progress(percent, n, total)
		}
	}
	size, err := DownloadWithConfig(ctx, pkg.TarballURL, archive, cfg)
	if err != nil {
		return err
	}
	f, err := npmPackageOpen(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := integrity.verify(npmPackageContextReader{ctx, io.NewSectionReader(f, 0, size)}); err != nil {
		return err
	}
	prepared, err := extractNPMPackageAssets(ctx, io.NewSectionReader(f, 0, size), pkg, stage,
		map[string]bool{asset.Name: true}, binaryNameForSplit(asset.Name))
	if err != nil {
		return err
	}
	file := prepared.BinaryArchivePath
	if asset.Name == skillsZipName {
		file = prepared.SkillsArchivePath
	}
	info, err := npmSplitStat(file)
	if err != nil {
		return err
	}
	if info.Size() != asset.Size {
		return fmt.Errorf("npm 分包资源大小不一致")
	}
	if err := VerifySHA256(file, ExtractDigestSHA256(asset.Digest)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(file, dest)
}

func binaryNameForSplit(asset string) string {
	if asset == skillsZipName {
		return npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
	}
	return asset
}
