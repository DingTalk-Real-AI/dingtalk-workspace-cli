// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const platformArchiveMaxBytes int64 = 128 << 20

// ResolvePlatformRelease 将官方 npm 精确版本绑定到同 tag 的不可变 GitHub 资产。
// 返回 nil 表示来源不适用：显式 GitHub、自定义 Registry 和内部发行版保留原协议。
// 网络错误可切换同版本 npm 平台分包，不隐式改为下载全平台 npm 包。
func ResolvePlatformRelease(ctx context.Context, release *ReleaseInfo, skipSkills bool) (*ReleaseInfo, error) {
	return ResolvePlatformReleaseForSource(ctx, release, skipSkills, SourceAuto)
}

// ResolvePlatformReleaseForSource pins explicit sources; only auto may switch.
func ResolvePlatformReleaseForSource(ctx context.Context, release *ReleaseInfo, skipSkills bool, source DownloadSource) (*ReleaseInfo, error) {
	source = source.normalized()
	if _, err := ParseDownloadSource(string(source)); err != nil {
		return nil, err
	}
	if source != SourceAuto && (release == nil || release.NPM == nil || !publicUpgradeEdition(upgradeEditionName())) {
		return nil, fmt.Errorf("--source %s 需要公共主包的有效版本信息", source)
	}
	if release == nil || release.NPM == nil || !publicUpgradeEdition(upgradeEditionName()) {
		return nil, nil
	}
	pkg := release.NPM
	registry := strings.TrimRight(pkg.RegistryURL, "/")
	if source == SourceGitHub && registry != defaultNPMRegistry || source == SourceNPMMirror && registry != publicNPMMirror {
		return nil, fmt.Errorf("主包 Registry 与 --source %s 不一致", source)
	}
	if registry != defaultNPMRegistry && registry != publicNPMMirror {
		return nil, nil
	}
	if pkg.Name != npmPackageName || pkg.Version != release.Version || !validNPMVersion(release.Version) {
		return nil, fmt.Errorf("npm 版本身份无效，拒绝解析平台资产")
	}
	if registry == publicNPMMirror {
		// Legacy main packages authenticate the complete tarball with their SRI.
		// Later split backfills must not change that version's installation path.
		if source == SourceAuto && pkg.Upgrade == nil {
			reportDownload(ctx, DownloadEvent{Kind: "source", Source: "npmmirror", Resource: "完整 npm 包", Reason: "该历史主包未声明平台资源，使用整包兼容路径"})
			return nil, nil
		}
		reportDownload(ctx, DownloadEvent{Kind: "source", Source: "npmmirror", Resource: "平台资源元数据"})
		resolved, err := resolveMirrorSplitRelease(ctx, release, skipSkills)
		if source == SourceAuto && errors.Is(err, errNPMSplitUnavailable) {
			reportDownload(ctx, DownloadEvent{Kind: "source", Source: "npmmirror", Resource: "完整 npm 包", Reason: "该历史版本没有平台分包，使用整包兼容路径"})
			return nil, nil
		}
		return resolved, err
	}
	expectedTarball := defaultNPMRegistry + "/" + npmPackageName + "/-/" + npmPackageName + "-" + release.Version + ".tgz"
	if pkg.TarballURL != expectedTarball {
		return nil, fmt.Errorf("npm 包来源与官方发布不一致，拒绝解析平台资产")
	}
	// 复用有界元数据读取；本机凭据仅通过专属 transport 附加到官方 GitHub API。
	client := &Client{httpClient: &http.Client{
		Timeout:   8 * time.Second,
		Transport: githubMetadataTransport{base: http.DefaultTransport, token: platformGitHubToken(ctx)},
	}}
	reportDownload(ctx, DownloadEvent{Kind: "source", Source: "github", Resource: "平台资源元数据"})
	resolved, err := resolvePlatformRelease(ctx, client, release, skipSkills, runtime.GOOS, runtime.GOARCH)
	if err == nil || source != SourceAuto || ctx.Err() != nil || !canFallbackPlatform(err) {
		return resolved, err
	}
	reportDownload(ctx, DownloadEvent{Kind: "switch", Previous: "github", Source: "npmmirror", Resource: "平台资源元数据", Reason: downloadFailureReason(err)})
	mirrored, mirrorErr := resolveMirrorSplitRelease(ctx, release, skipSkills)
	if mirrorErr != nil {
		return nil, errors.Join(err, fmt.Errorf("npmmirror 同版本平台分包不可用: %w", mirrorErr))
	}
	return mirrored, nil
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
	assetURL := asset.BrowserDownloadURL
	if asset.NPM != nil {
		suffix, err := npmSplitSuffix(asset.Name)
		pkg := asset.NPM
		if err != nil || pkg.Name != npmPackageName+"-"+suffix || !publicSplitVersion.MatchString(pkg.Version) ||
			pkg.RegistryURL != publicNPMMirror || !validNPMIntegrity(pkg.Integrity) {
			return fmt.Errorf("npm 平台分包身份无效")
		}
		assetURL = pkg.TarballURL
	}
	if asset.Size <= 0 || asset.Size > platformArchiveMaxBytes || !validRegistryURL(assetURL) {
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
	return DownloadPlatformAssetForSource(ctx, asset, dest, progress, SourceAuto)
}

// DownloadPlatformAssetForSource never falls back when a source is pinned.
func DownloadPlatformAssetForSource(ctx context.Context, asset GitHubAsset, dest string, progress func(float64, int64, int64), source DownloadSource) error {
	source = source.normalized()
	if _, err := ParseDownloadSource(string(source)); err != nil {
		return err
	}
	if source == SourceGitHub && asset.NPM != nil || source == SourceNPMMirror && asset.NPM == nil {
		return fmt.Errorf("平台资源与 --source %s 不一致", source)
	}
	if err := validatePlatformAsset(asset); err != nil {
		return err
	}
	if asset.NPM != nil {
		return downloadNPMSplitAsset(ctx, asset, dest, progress)
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
		if source == SourceAuto && ctx.Err() == nil && canFallbackPlatform(err) && publicUpgradeEdition(upgradeEditionName()) {
			reportDownload(ctx, DownloadEvent{Kind: "switch", Previous: "github", Source: "npmmirror", Resource: asset.Name, Reason: downloadFailureReason(err)})
			mirrored, mirrorErr := mirrorForGitHubAsset(ctx, asset)
			if mirrorErr == nil {
				return downloadNPMSplitAsset(ctx, mirrored, dest, progress)
			}
			return errors.Join(err, fmt.Errorf("npmmirror 同版本资源不可用: %w", mirrorErr))
		}
		return err
	}
	if size != asset.Size {
		return fmt.Errorf("平台资产大小不匹配 (%s): 预期 %d，实际 %d", asset.Name, asset.Size, size)
	}
	return VerifySHA256(dest, ExtractDigestSHA256(asset.Digest))
}
