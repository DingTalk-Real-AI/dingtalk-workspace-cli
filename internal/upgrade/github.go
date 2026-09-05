// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package upgrade provides self-update functionality for the DWS CLI
// using GitHub Releases as the data source.
package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
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
		Name:         "DWS_UPGRADE_REPOSITORY",
		Category:     configmeta.CategoryNetwork,
		Description:  "覆盖升级使用的 GitHub 仓库 owner/repo (测试/灰度)",
		DefaultValue: defaultOwner + "/" + defaultRepo,
		Example:      "PeterGuy326/dingtalk-workspace-cli",
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
	gitHubAPIBase   = "https://api.github.com"
	defaultOwner    = "DingTalk-Real-AI"
	defaultRepo     = "dingtalk-workspace-cli"
	httpTimeout     = 30 * time.Second
	userAgent       = "DWS-CLI-Upgrade/1.0"
	skillsZipName   = "dws-skills.zip"
	checksumsName   = "checksums.txt"
	maxTagPeelDepth = 5
)

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
	URL  string `json:"url"`
}

type gitRef struct {
	Ref    string    `json:"ref"`
	Object gitObject `json:"object"`
}

type gitTag struct {
	Tag    string    `json:"tag"`
	SHA    string    `json:"sha"`
	Object gitObject `json:"object"`
}

// GitHubRelease represents a single release from the GitHub Releases API.
type GitHubRelease struct {
	TagName         string        `json:"tag_name"`
	TargetCommitish string        `json:"target_commitish"`
	Name            string        `json:"name"`
	Body            string        `json:"body"`
	Prerelease      bool          `json:"prerelease"`
	Draft           bool          `json:"draft"`
	PublishedAt     string        `json:"published_at"`
	Assets          []GitHubAsset `json:"assets"`
	HTMLURL         string        `json:"html_url"`
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
	Commit     string
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

// ReleaseTrack selects which release stream an upgrade operation should use.
type ReleaseTrack string

const (
	ReleaseTrackRelease ReleaseTrack = "release"
	ReleaseTrackBeta    ReleaseTrack = "beta"
	ReleaseTrackAll     ReleaseTrack = "all"
)

// Client communicates with the GitHub Releases API.
type Client struct {
	httpClient *http.Client
	owner      string
	repo       string
	baseURL    string // overridable for testing or mirrors
	configErr  error
}

// NewClient creates a GitHub release client with default settings.
func NewClient() *Client {
	baseURL := gitHubAPIBase
	if env := os.Getenv("DWS_UPGRADE_URL"); env != "" {
		baseURL = strings.TrimRight(env, "/")
	}
	owner, repo, configErr := repositoryFromEnv()
	return &Client{
		httpClient: &http.Client{Timeout: httpTimeout},
		owner:      owner,
		repo:       repo,
		baseURL:    baseURL,
		configErr:  configErr,
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
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, c.owner, c.repo)

	var gh GitHubRelease
	if err := c.getJSON(url, &gh); err != nil {
		return nil, fmt.Errorf("获取最新版本失败: %w", err)
	}

	return c.releaseToInfo(&gh)
}

// FetchLatestReleaseForTrack returns the latest release in the requested track.
func (c *Client) FetchLatestReleaseForTrack(track ReleaseTrack) (*ReleaseInfo, error) {
	switch track {
	case ReleaseTrackBeta:
		return c.FetchLatestPrerelease()
	case ReleaseTrackRelease, "":
		return c.FetchLatestStableRelease()
	default:
		return nil, fmt.Errorf("未知升级轨道: %s", track)
	}
}

// FetchLatestStableRelease returns the newest non-draft, non-prerelease release
// whose tag is a formal semantic version (vX.Y.Z).
func (c *Client) FetchLatestStableRelease() (*ReleaseInfo, error) {
	releases, err := c.fetchReleases()
	if err != nil {
		return nil, fmt.Errorf("获取正式 release 版本失败: %w", err)
	}
	for i := range releases {
		if !releaseMatchesTrack(releases[i], ReleaseTrackRelease) {
			continue
		}
		return c.releaseToInfo(&releases[i])
	}
	return nil, fmt.Errorf("未找到正式 release 版本（需要非 pre-release 且 tag 形如 vX.Y.Z）")
}

// FetchLatestPrerelease returns the newest non-draft prerelease.
func (c *Client) FetchLatestPrerelease() (*ReleaseInfo, error) {
	releases, err := c.fetchReleases()
	if err != nil {
		return nil, fmt.Errorf("获取 beta 版本失败: %w", err)
	}
	for i := range releases {
		if !releaseMatchesTrack(releases[i], ReleaseTrackBeta) {
			continue
		}
		return c.releaseToInfo(&releases[i])
	}
	return nil, fmt.Errorf("未找到 beta 版本（需要 GitHub pre-release 且 tag 形如 vX.Y.Z-*）")
}

// FetchReleaseByTag returns the release for a specific tag (e.g. "v1.0.5").
func (c *Client) FetchReleaseByTag(tag string) (*ReleaseInfo, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.baseURL, c.owner, c.repo, tag)

	var gh GitHubRelease
	if err := c.getJSON(url, &gh); err != nil {
		return nil, fmt.Errorf("获取版本 %s 失败: %w", tag, err)
	}
	if gh.TagName != tag {
		return nil, fmt.Errorf("release tag identity mismatch: requested %q, received %q", tag, gh.TagName)
	}
	return c.releaseToInfo(&gh)
}

// FetchAllReleases returns all non-draft releases, newest first.
func (c *Client) FetchAllReleases() ([]VersionEntry, error) {
	return c.FetchReleaseVersions(ReleaseTrackAll)
}

// FetchReleaseVersions returns non-draft releases matching the requested track,
// newest first. The GitHub API already returns releases newest first.
func (c *Client) FetchReleaseVersions(track ReleaseTrack) ([]VersionEntry, error) {
	ghReleases, err := c.fetchReleases()
	if err != nil {
		return nil, fmt.Errorf("获取版本列表失败: %w", err)
	}

	var versions []VersionEntry
	for _, gh := range ghReleases {
		if !releaseMatchesTrack(gh, track) {
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

func (c *Client) fetchReleases() ([]GitHubRelease, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", c.baseURL, c.owner, c.repo)

	var ghReleases []GitHubRelease
	if err := c.getJSON(url, &ghReleases); err != nil {
		return nil, err
	}
	return ghReleases, nil
}

func releaseMatchesTrack(gh GitHubRelease, track ReleaseTrack) bool {
	if gh.Draft {
		return false
	}
	switch track {
	case ReleaseTrackBeta:
		return gh.Prerelease && isPrereleaseVersionTag(gh.TagName)
	case ReleaseTrackRelease, "":
		return !gh.Prerelease && isStableVersionTag(gh.TagName)
	case ReleaseTrackAll:
		return true
	default:
		return false
	}
}

func (c *Client) validateConfig() error {
	if c.configErr != nil {
		return c.configErr
	}
	return nil
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

func repositoryFromEnv() (string, string, error) {
	raw := strings.TrimSpace(os.Getenv("DWS_UPGRADE_REPOSITORY"))
	if raw == "" {
		return defaultOwner, defaultRepo, nil
	}
	owner, repo, ok := parseGitHubRepository(raw)
	if !ok {
		return defaultOwner, defaultRepo, fmt.Errorf("DWS_UPGRADE_REPOSITORY 必须是 owner/repo 格式，当前值: %q", raw)
	}
	return owner, repo, nil
}

func parseGitHubRepository(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "https://github.com/")
	raw = strings.TrimPrefix(raw, "http://github.com/")
	raw = strings.TrimPrefix(raw, "github.com/")
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.Trim(raw, "/")

	parts := strings.Split(raw, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}

func (c *Client) releaseToInfo(gh *GitHubRelease) (*ReleaseInfo, error) {
	commit, err := c.resolveTagCommit(gh.TagName)
	if err != nil {
		return nil, fmt.Errorf("解析 release tag %q 的 commit 失败: %w", gh.TagName, err)
	}
	return &ReleaseInfo{
		Version:    stripV(gh.TagName),
		Commit:     commit,
		Date:       formatDate(gh.PublishedAt),
		Changelog:  gh.Body,
		Prerelease: gh.Prerelease,
		HTMLURL:    gh.HTMLURL,
		Assets:     gh.Assets,
	}, nil
}

func (c *Client) resolveTagCommit(tag string) (string, error) {
	if tag == "" || strings.ContainsAny(tag, "\x00\r\n") {
		return "", errors.New("invalid empty or control-character tag")
	}
	refURL := fmt.Sprintf("%s/repos/%s/%s/git/ref/tags/%s", c.baseURL, c.owner, c.repo, url.PathEscape(tag))
	var ref gitRef
	if err := c.getJSON(refURL, &ref); err != nil {
		return "", err
	}
	wantRef := "refs/tags/" + tag
	if ref.Ref != wantRef {
		return "", fmt.Errorf("tag ref identity mismatch: got %q, want %q", ref.Ref, wantRef)
	}
	object := ref.Object
	for depth := 0; depth <= maxTagPeelDepth; depth++ {
		if !validGitSHA(object.SHA) {
			return "", fmt.Errorf("invalid git object SHA %q", object.SHA)
		}
		switch object.Type {
		case "commit":
			return object.SHA, nil
		case "tag":
			if depth == maxTagPeelDepth {
				return "", fmt.Errorf("annotated tag nesting exceeds %d", maxTagPeelDepth)
			}
			var annotated gitTag
			tagURL := fmt.Sprintf("%s/repos/%s/%s/git/tags/%s", c.baseURL, c.owner, c.repo, object.SHA)
			if err := c.getJSON(tagURL, &annotated); err != nil {
				return "", err
			}
			if annotated.SHA != object.SHA {
				return "", fmt.Errorf("annotated tag identity mismatch: got SHA %q, want %q", annotated.SHA, object.SHA)
			}
			if depth == 0 && annotated.Tag != tag {
				return "", fmt.Errorf("annotated tag name mismatch: got %q, want %q", annotated.Tag, tag)
			}
			object = annotated.Object
		default:
			return "", fmt.Errorf("tag resolves to unsupported git object type %q", object.Type)
		}
	}
	return "", errors.New("unreachable tag peel state")
}

func validGitSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func stripV(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

func isVersionLikeTag(tag string) bool {
	tag = stripV(tag)
	core := tag
	if idx := strings.IndexByte(core, '-'); idx >= 0 {
		core = core[:idx]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

func isStableVersionTag(tag string) bool {
	return isVersionLikeTag(tag) && !strings.Contains(stripV(tag), "-")
}

func isPrereleaseVersionTag(tag string) bool {
	return isVersionLikeTag(tag) && strings.Contains(stripV(tag), "-")
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
