// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// NPMArtifacts 由发布打包步骤从实际资产生成，随精确版本的 Registry 清单交付。
type NPMArtifacts struct {
	Version int                    `json:"version"`
	Files   map[string]NPMArtifact `json:"files"`
}

// NPMArtifact 声明一个已发布文件的下载地址、摘要和字节数。
type NPMArtifact struct {
	URLs   []string `json:"urls"`
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
}

func validateNPMArtifacts(artifacts *NPMArtifacts) error {
	if artifacts == nil {
		return nil
	}
	if artifacts.Version != 1 || len(artifacts.Files) == 0 || len(artifacts.Files) > 16 {
		return fmt.Errorf("npm 按平台下载清单版本或文件数量无效")
	}
	allowed := map[string]bool{skillsZipName: true}
	for _, platform := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64", "openharmony-arm64"} {
		ext := ".tar.gz"
		if strings.HasPrefix(platform, "windows-") {
			ext = ".zip"
		}
		allowed["dws-"+platform+ext] = true
	}
	for name, artifact := range artifacts.Files {
		digest, err := hex.DecodeString(artifact.SHA256)
		if !allowed[name] || err != nil || len(digest) != 32 || artifact.Size <= 0 || artifact.Size > npmPackageMaxEntry || len(artifact.URLs) == 0 || len(artifact.URLs) > 3 {
			return fmt.Errorf("npm 按平台下载清单文件 %s 无效", name)
		}
		for _, address := range artifact.URLs {
			if !validRegistryURL(address) {
				return fmt.Errorf("npm 按平台下载地址无效")
			}
		}
	}
	return nil
}

// 仅允许默认公开源的规范包使用下载镜像；镜像不提供版本或完整性权威。
// 自定义 Registry 与非规范 tarball 保持来源隔离，不继承全局镜像配置。
func npmTarballURLs(pkg NPMPackage) ([]string, error) {
	canonical := defaultNPMRegistry + "/" + npmPackageName + "/-/" + npmPackageName + "-" + pkg.Version + ".tgz"
	if pkg.RegistryURL != defaultNPMRegistry || pkg.TarballURL != canonical {
		return []string{pkg.TarballURL}, nil
	}
	mirror := os.Getenv("DWS_UPGRADE_NPM_MIRROR")
	if mirror == "" {
		mirror = "https://registry.npmmirror.com"
	}
	if !validRegistryURL(mirror) {
		return nil, fmt.Errorf("DWS_UPGRADE_NPM_MIRROR 必须是无凭据、查询参数或片段的 HTTPS Registry 根地址")
	}
	address := strings.TrimRight(mirror, "/") + "/" + npmPackageName + "/-/" + npmPackageName + "-" + pkg.Version + ".tgz"
	if address == canonical {
		return []string{canonical}, nil
	}
	return []string{address, canonical}, nil
}

func downloadCanFallback(err error) bool {
	var typed *apperrors.Error
	return errors.As(err, &typed) && (typed.Reason == "download_timeout" || typed.Reason == "download_network_error" || typed.Reason == "download_http_status")
}

func downloadFromSources(ctx context.Context, urls []string, dest string, cfg *DownloadConfig) (int64, error) {
	var previousErr error
	for i, address := range urls {
		attemptConfig := *cfg
		if cfg.EventCallback != nil {
			attemptConfig.EventCallback = func(event DownloadEvent) {
				if event.Attempt == 1 {
					event.Err = previousErr
				}
				cfg.EventCallback(event)
			}
		}
		if i < len(urls)-1 {
			if attemptConfig.Timeout <= 0 || attemptConfig.Timeout > 2*time.Minute {
				attemptConfig.Timeout = 2 * time.Minute
			}
		}
		sourceCtx := ctx
		cancel := func() {}
		if attemptConfig.Timeout > 0 {
			sourceCtx, cancel = context.WithTimeout(ctx, attemptConfig.Timeout)
		}
		n, err := DownloadWithConfig(sourceCtx, address, dest, &attemptConfig)
		if sourceCtx.Err() != nil && ctx.Err() == nil {
			err = downloadNetworkError(sourceCtx.Err())
		}
		cancel()
		if err == nil || ctx.Err() != nil || !downloadCanFallback(err) || i == len(urls)-1 {
			return n, err
		}
		previousErr = err
	}
	return 0, fmt.Errorf("升级文件没有下载来源")
}

func prepareNPMArtifacts(ctx context.Context, pkg NPMPackage, destDir string, skipSkills bool, progress func(float64, int64, int64), observer func(DownloadEvent)) (result PreparedNPMPackage, err error) {
	if err := validateNPMArtifacts(pkg.Artifacts); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(destDir, "npm-artifacts-")
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(stage)
		}
	}()
	names := []string{npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)}
	if !skipSkills {
		names = append(names, skillsZipName)
	}
	for _, name := range names {
		artifact, ok := pkg.Artifacts.Files[name]
		if !ok {
			return result, fmt.Errorf("npm 按平台下载清单缺少 %s", name)
		}
		cfg := DefaultDownloadConfig()
		// 按平台源有界等待，网络失败才回退完整 npm 包；持续传输也有总预算。
		cfg.Timeout = 2 * time.Minute
		cfg.MaxBytes = artifact.Size
		cfg.CheckRedirect = npmPackageRedirect
		cfg.EventCallback = observer
		cfg.ProgressCallback = func(downloaded, _ int64) {
			if progress != nil {
				progress(float64(downloaded)/float64(artifact.Size)*100, downloaded, artifact.Size)
			}
		}
		path := filepath.Join(stage, name)
		n, downloadErr := downloadFromSources(ctx, artifact.URLs, path, cfg)
		if downloadErr != nil {
			return result, downloadErr
		}
		if n != artifact.Size {
			return result, fmt.Errorf("升级文件 %s 大小与发布清单不一致", name)
		}
		if verifyErr := VerifySHA256(path, artifact.SHA256); verifyErr != nil {
			return result, verifyErr
		}
		result.ChecksumsContent += strings.ToLower(artifact.SHA256) + "  " + name + "\n"
		if name == skillsZipName {
			result.SkillsArchivePath = path
		} else {
			result.BinaryArchivePath = path
		}
	}
	return result, nil
}
