// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

const (
	npmRegistryBase = "https://registry.npmjs.org"
	npmPackageName  = "dingtalk-workspace-cli"
	updateCacheTTL  = 10 * time.Minute
	updateCacheFile = "update-state.json"
	registryTimeout = 15 * time.Second
	registryMaxBody = 256 << 10
)

type npmPackageMetadata struct {
	Version    string `json:"version"`
	Deprecated string `json:"deprecated"`
}

type updateCacheState struct {
	Channels map[string]updateCacheEntry `json:"channels"`
}

type updateCacheEntry struct {
	Version   string `json:"version"`
	CheckedAt int64  `json:"checked_at"`
}

func updateCachePath() string {
	homeDir, err := upgradeUserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, ".dws", "cache", updateCacheFile)
}

func (c *Client) fetchRegistryRelease(track ReleaseTrack, useCache bool) (*ReleaseInfo, error) {
	channel, err := registryChannel(track)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	if useCache {
		if entry, ok := c.cachedRegistryEntry(channel, now); ok {
			if validateRegistryVersion(entry.Version, track) == nil {
				return c.releaseInfoForRegistryVersion(entry.Version, track), nil
			}
		}
	}

	url := fmt.Sprintf("%s/%s/%s", strings.TrimRight(c.registryURL, "/"), npmPackageName, channel)
	var metadata npmPackageMetadata
	if err := c.getRegistryJSON(url, &metadata); err != nil {
		return nil, fmt.Errorf("获取 npm %s 版本失败: %w", channel, err)
	}
	metadata.Version = strings.TrimSpace(metadata.Version)
	if err := validateRegistryVersion(metadata.Version, track); err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.Deprecated) != "" {
		return nil, fmt.Errorf("npm %s 指向已弃用版本 %s", channel, metadata.Version)
	}

	c.saveRegistryEntry(channel, updateCacheEntry{
		Version: metadata.Version, CheckedAt: now.Unix(),
	})
	return c.releaseInfoForRegistryVersion(metadata.Version, track), nil
}

func registryChannel(track ReleaseTrack) (string, error) {
	switch track {
	case ReleaseTrackRelease, "":
		return "latest", nil
	case ReleaseTrackBeta:
		return "beta", nil
	default:
		return "", fmt.Errorf("未知升级轨道: %s", track)
	}
}

func validateRegistryVersion(version string, track ReleaseTrack) error {
	if version == "" {
		return fmt.Errorf("npm registry 返回空版本")
	}
	tag := "v" + strings.TrimPrefix(version, "v")
	switch track {
	case ReleaseTrackRelease, "":
		if !isStableVersionTag(tag) {
			return fmt.Errorf("npm latest 返回无效正式版本: %s", version)
		}
	case ReleaseTrackBeta:
		if !isPrereleaseVersionTag(tag) {
			return fmt.Errorf("npm beta 返回无效预发布版本: %s", version)
		}
	default:
		return fmt.Errorf("未知升级轨道: %s", track)
	}
	return nil
}

func (c *Client) getRegistryJSON(url string, target any) error {
	ctx, cancel := context.WithTimeout(context.Background(), registryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 npm registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("npm registry 返回 HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, registryMaxBody)).Decode(target); err != nil {
		return fmt.Errorf("解析 npm registry 响应失败: %w", err)
	}
	return nil
}

func (c *Client) cachedRegistryEntry(channel string, now time.Time) (updateCacheEntry, bool) {
	state := c.loadUpdateCache()
	entry, ok := state.Channels[channel]
	if !ok || entry.Version == "" || entry.CheckedAt <= 0 {
		return updateCacheEntry{}, false
	}
	checkedAt := time.Unix(entry.CheckedAt, 0)
	if now.Before(checkedAt) || now.Sub(checkedAt) >= updateCacheTTL {
		return updateCacheEntry{}, false
	}
	return entry, true
}

func (c *Client) saveRegistryEntry(channel string, entry updateCacheEntry) {
	if c.cachePath == "" {
		return
	}
	state := c.loadUpdateCache()
	state.Channels[channel] = entry
	data, _ := json.MarshalIndent(state, "", "  ")
	_ = helpers.AtomicWrite(c.cachePath, append(data, '\n'), 0o600)
}

func (c *Client) loadUpdateCache() updateCacheState {
	state := updateCacheState{Channels: make(map[string]updateCacheEntry)}
	if c.cachePath == "" {
		return state
	}
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Channels == nil {
		return updateCacheState{Channels: make(map[string]updateCacheEntry)}
	}
	return state
}

func (c *Client) releaseInfoForRegistryVersion(version string, track ReleaseTrack) *ReleaseInfo {
	version = strings.TrimPrefix(version, "v")
	tag := "v" + version
	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", c.owner, c.repo, tag)
	downloadBase := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s", c.owner, c.repo, tag)
	assetNames := []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
		"dws-windows-amd64.zip",
		"dws-windows-arm64.zip",
		skillsZipName,
		checksumsName,
	}
	assets := make([]GitHubAsset, 0, len(assetNames))
	for _, name := range assetNames {
		assets = append(assets, GitHubAsset{
			Name:               name,
			BrowserDownloadURL: downloadBase + "/" + name,
		})
	}
	return &ReleaseInfo{
		Version:    version,
		Prerelease: track == ReleaseTrackBeta,
		HTMLURL:    releaseURL,
		Assets:     assets,
	}
}
