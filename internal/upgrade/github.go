// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package upgrade provides self-update functionality for the DWS CLI
// using GitHub Releases as the data source.
package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_UPGRADE_URL",
		Category:     configmeta.CategoryNetwork,
		Description:  "覆盖 GitHub API 地址 (镜像/测试)",
		DefaultValue: "https://api.github.com",
		Example:      "https://mirror.example.com/api",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "GITHUB_TOKEN",
		Category:    configmeta.CategoryExternal,
		Description: "GitHub API Token (提升 API 限额)",
		Sensitive:   true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "GH_TOKEN",
		Category:    configmeta.CategoryExternal,
		Description: "GitHub API Token 备选 (GITHUB_TOKEN 为空时使用)",
		Sensitive:   true,
	})
}

const (
	gitHubAPIBase = "https://api.github.com"
	defaultOwner  = "DingTalk-Real-AI"
	defaultRepo   = "dingtalk-workspace-cli"
	httpTimeout   = 30 * time.Second
	userAgent     = "DWS-CLI-Upgrade/1.0"
	skillsZipName = "dws-skills.zip"
	checksumsName = "checksums.txt"
)

// GitHubRelease represents a single release from the GitHub Releases API.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Prerelease  bool          `json:"prerelease"`
	Draft       bool          `json:"draft"`
	PublishedAt string        `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
	HTMLURL     string        `json:"html_url"`
}

// GitHubAsset represents a release asset (downloadable file).
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// ReleaseInfo is the simplified view of a release used throughout the upgrade flow.
type ReleaseInfo struct {
	Version    string
	Date       string
	Changelog  string
	Prerelease bool
	HTMLURL    string
	Assets     []GitHubAsset
}

// VersionEntry represents a single version in the version list.
type VersionEntry struct {
	Version    string
	Date       string
	Changelog  string
	Prerelease bool
}

// Client communicates with the GitHub Releases API.
type Client struct {
	httpClient *http.Client
	owner      string
	repo       string
	baseURL    string // overridable for testing or mirrors
}

// NewClient creates a GitHub release client with default settings.
func NewClient() *Client {
	baseURL := gitHubAPIBase
	if env := os.Getenv("DWS_UPGRADE_URL"); env != "" {
		baseURL = strings.TrimRight(env, "/")
	}
	return &Client{
		httpClient: &http.Client{Timeout: httpTimeout},
		owner:      defaultOwner,
		repo:       defaultRepo,
		baseURL:    baseURL,
	}
}

// NewClientWithBaseURL creates a client with a custom base URL (for testing).
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: httpTimeout},
		owner:      defaultOwner,
		repo:       defaultRepo,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// FetchLatestRelease returns the latest non-draft release.
func (c *Client) FetchLatestRelease() (*ReleaseInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, c.owner, c.repo)

	var gh GitHubRelease
	if err := c.getJSON(url, &gh); err != nil {
		return nil, fmt.Errorf("获取最新版本失败: %w", err)
	}

	return ghReleaseToInfo(&gh), nil
}

// FetchReleaseByTag returns the release for a specific tag (e.g. "v1.0.5").
func (c *Client) FetchReleaseByTag(tag string) (*ReleaseInfo, error) {
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.baseURL, c.owner, c.repo, tag)

	var gh GitHubRelease
	if err := c.getJSON(url, &gh); err != nil {
		return nil, fmt.Errorf("获取版本 %s 失败: %w", tag, err)
	}

	return ghReleaseToInfo(&gh), nil
}

// FetchAllReleases returns all non-draft releases, newest first.
func (c *Client) FetchAllReleases() ([]VersionEntry, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", c.baseURL, c.owner, c.repo)

	var ghReleases []GitHubRelease
	if err := c.getJSON(url, &ghReleases); err != nil {
		return nil, fmt.Errorf("获取版本列表失败: %w", err)
	}

	var versions []VersionEntry
	for _, gh := range ghReleases {
		if gh.Draft {
			continue
		}
		versions = append(versions, VersionEntry{
			Version:    stripV(gh.TagName),
			Date:       formatDate(gh.PublishedAt),
			Changelog:  gh.Body,
			Prerelease: gh.Prerelease,
		})
	}
	return versions, nil
}

// FindBinaryAsset locates the platform-specific binary archive from the release assets.
// Pattern: dws-{os}-{arch}.tar.gz (or .zip for windows).
func FindBinaryAsset(assets []GitHubAsset) (*GitHubAsset, error) {
	return FindBinaryAssetFor(assets, runtime.GOOS, runtime.GOARCH)
}

// FindBinaryAssetFor locates the binary archive for a specific platform.
func FindBinaryAssetFor(assets []GitHubAsset, goos, goarch string) (*GitHubAsset, error) {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	target := fmt.Sprintf("dws-%s-%s%s", goos, goarch, ext)

	for i := range assets {
		if assets[i].Name == target {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("当前平台 %s/%s 没有可用的预编译二进制 (需要 %s)", goos, goarch, target)
}

// FindSkillsAsset locates the dws-skills.zip asset.
func FindSkillsAsset(assets []GitHubAsset) *GitHubAsset {
	for i := range assets {
		if assets[i].Name == skillsZipName {
			return &assets[i]
		}
	}
	return nil
}

// FindChecksumsAsset locates the checksums.txt asset.
func FindChecksumsAsset(assets []GitHubAsset) *GitHubAsset {
	for i := range assets {
		if assets[i].Name == checksumsName {
			return &assets[i]
		}
	}
	return nil
}

// ExtractDigestSHA256 extracts the hex hash from a GitHub asset digest field.
// GitHub format: "sha256:abcdef1234..."
func ExtractDigestSHA256(digest string) string {
	if strings.HasPrefix(digest, "sha256:") {
		return digest[7:]
	}
	return ""
}

// getJSON performs a GET request and decodes the JSON response.
func (c *Client) getJSON(url string, target interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接到 GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
			return fmt.Errorf("GitHub API 请求频率超限。设置 GITHUB_TOKEN 或 GH_TOKEN 环境变量可提升限额")
		}
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("未找到 (404)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API 返回 HTTP %d: %s", resp.StatusCode, truncateBytes(body, 200))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}

	return nil
}

// githubToken returns a GitHub token from environment if available.
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return ""
}

func ghReleaseToInfo(gh *GitHubRelease) *ReleaseInfo {
	return &ReleaseInfo{
		Version:    stripV(gh.TagName),
		Date:       formatDate(gh.PublishedAt),
		Changelog:  gh.Body,
		Prerelease: gh.Prerelease,
		HTMLURL:    gh.HTMLURL,
		Assets:     gh.Assets,
	}
}

func stripV(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

func formatDate(published string) string {
	t, err := time.Parse(time.RFC3339, published)
	if err != nil {
		return published
	}
	return t.Format("2006-01-02")
}

func truncateBody(body string, maxLen int) string {
	body = strings.TrimSpace(body)
	lines := strings.SplitN(body, "\n", 2)
	first := lines[0]
	if len(first) > maxLen {
		return first[:maxLen-3] + "..."
	}
	return first
}

func truncateBytes(b []byte, max int) string {
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
