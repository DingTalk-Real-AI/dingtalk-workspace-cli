// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

const platformArchiveMaxBytes int64 = 128 << 20

// ResolvePlatformRelease 将官方 npm 精确版本绑定到同 tag 的不可变 GitHub 资产。
// 返回 nil 表示来源不适用：显式 GitHub、自定义 Registry 和内部发行版保留原协议。
// 官方资产解析失败直接返回错误，不能悄悄降级成全平台 npm 包下载。
func ResolvePlatformRelease(ctx context.Context, release *ReleaseInfo, skipSkills bool) (*ReleaseInfo, error) {
	if release.NPM == nil || !publicUpgradeEdition(upgradeEditionName()) || strings.TrimRight(release.NPM.RegistryURL, "/") != defaultNPMRegistry {
		return nil, nil
	}
	pkg := release.NPM
	if pkg.Name != npmPackageName || pkg.Version != release.Version || !validNPMVersion(release.Version) {
		return nil, fmt.Errorf("npm 版本身份无效，拒绝解析平台资产")
	}
	expectedTarball := defaultNPMRegistry + "/" + npmPackageName + "/-/" + npmPackageName + "-" + release.Version + ".tgz"
	if pkg.TarballURL != expectedTarball {
		return nil, fmt.Errorf("npm 包来源与官方发布不一致，拒绝解析平台资产")
	}
	// 复用有界元数据读取；本机凭据仅通过专属 transport 附加到官方 GitHub API。
	client := &Client{httpClient: &http.Client{
		Timeout:   httpTimeout,
		Transport: githubMetadataTransport{base: http.DefaultTransport, token: platformGitHubToken(ctx)},
	}}
	return resolvePlatformRelease(ctx, client, release, skipSkills, runtime.GOOS, runtime.GOARCH)
}

func resolvePlatformRelease(ctx context.Context, client *Client, release *ReleaseInfo, skipSkills bool, goos, goarch string) (*ReleaseInfo, error) {
	tag := "v" + release.Version
	endpoint := gitHubAPIBase + "/repos/" + defaultOwner + "/" + defaultRepo + "/releases/tags/" + tag
	var gh struct {
		GitHubRelease
		Immutable bool `json:"immutable"`
	}
	if err := client.getBoundedJSON(ctx, endpoint, &gh, registryManifestLimit, "GitHub Release", "GitHub Release 元数据"); err != nil {
		return nil, fmt.Errorf("获取 %s 的平台发布资产失败: %w", tag, err)
	}
	if gh.TagName != tag || gh.Draft || !gh.Immutable || gh.Prerelease != release.Prerelease {
		return nil, fmt.Errorf("平台资产必须来自同版本、同轨道且已封存的公开 Release: %s", tag)
	}
	binaryName := npmPackageBinaryName(goos, goarch)
	names := []string{binaryName}
	if !skipSkills {
		names = append(names, skillsZipName)
	}
	resolved := *release
	resolved.Assets = make([]GitHubAsset, 0, len(names))
	for _, name := range names {
		var selected *GitHubAsset
		for i := range gh.Assets {
			if gh.Assets[i].Name == name {
				if selected != nil {
					return nil, fmt.Errorf("平台发布资产重复: %s", name)
				}
				selected = &gh.Assets[i]
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("%s 缺少平台发布资产: %s", tag, name)
		}
		expectedURL := "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/" + tag + "/" + name
		if selected.BrowserDownloadURL != expectedURL {
			return nil, fmt.Errorf("平台资产地址与目标版本不一致: %s", name)
		}
		if err := validatePlatformAsset(*selected); err != nil {
			return nil, err
		}
		resolved.Assets = append(resolved.Assets, *selected)
	}
	return &resolved, nil
}

func validatePlatformAsset(asset GitHubAsset) error {
	digest, err := hex.DecodeString(ExtractDigestSHA256(asset.Digest))
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("平台资产缺少有效 SHA256: %s", asset.Name)
	}
	if asset.Size <= 0 || asset.Size > platformArchiveMaxBytes || !validRegistryURL(asset.BrowserDownloadURL) {
		return fmt.Errorf("平台资产大小或地址无效: %s", asset.Name)
	}
	return nil
}

// GitHub 资产会重定向到带签名查询参数的 HTTPS CDN；允许查询参数，但禁止凭据和降级。
func platformAssetRedirect(req *http.Request, via []*http.Request) error {
	target := *req.URL
	target.RawQuery, target.ForceQuery = "", false
	if len(via) >= 10 || !validRegistryURL(target.String()) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
		return fmt.Errorf("平台资产重定向无效")
	}
	return nil
}

// DownloadPlatformAsset 下载单个资产；大小和摘要均符合清单后才允许解压或安装。
func DownloadPlatformAsset(ctx context.Context, asset GitHubAsset, dest string, progress func(float64, int64, int64)) error {
	if err := validatePlatformAsset(asset); err != nil {
		return err
	}
	cfg := DefaultDownloadConfig()
	cfg.MaxBytes = asset.Size
	cfg.CheckRedirect = platformAssetRedirect
	cfg.ProgressCallback = func(downloaded, _ int64) {
		if progress != nil {
			progress(float64(downloaded)/float64(asset.Size)*100, downloaded, asset.Size)
		}
	}
	size, err := DownloadWithConfig(ctx, asset.BrowserDownloadURL, dest, cfg)
	if err != nil {
		return err
	}
	if size != asset.Size {
		return fmt.Errorf("平台资产大小不匹配 (%s): 预期 %d，实际 %d", asset.Name, asset.Size, size)
	}
	return VerifySHA256(dest, ExtractDigestSHA256(asset.Digest))
}
