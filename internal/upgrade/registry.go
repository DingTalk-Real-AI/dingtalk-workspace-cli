// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	defaultNPMRegistry     = "https://registry.npmjs.org"
	npmPackageName         = "dingtalk-workspace-cli"
	registryManifestLimit  = 1 << 20
	registryPackumentLimit = 16 << 20
)

// NPMPackage 是经元数据验证的包来源；下载方须验证 Integrity 和归档内的 name/version。
// Assets 仅声明包内标准文件名，不能将 tarball URL 当作单个二进制的下载地址。
type NPMPackage struct {
	RegistryURL string
	Name        string
	Version     string
	TarballURL  string
	Integrity   string
}

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_UPGRADE_REGISTRY",
		Category:     configmeta.CategoryNetwork,
		Description:  "覆盖 npm Registry 根地址（显式 GitHub 升级配置优先）",
		DefaultValue: defaultNPMRegistry,
		Example:      "https://registry.npmmirror.com",
	})
}

var upgradeEditionName = func() string { return edition.Get().Name }

// NewVersionClient 选择版本元数据来源：默认 npm，显式 GitHub 配置优先。
// 独立入口允许只读版本检查先采用 Registry，而不改变尚未支持 npm 包的安装链路。
func NewVersionClient() *Client {
	if hasExplicitGitHubSource() {
		return NewClient()
	}
	return newRegistryClient()
}

func hasExplicitGitHubSource() bool {
	return os.Getenv("DWS_UPGRADE_URL") != "" || os.Getenv("DWS_UPGRADE_REPOSITORY") != ""
}

func hasExplicitUpgradeSource() bool {
	return hasExplicitGitHubSource() || os.Getenv("DWS_UPGRADE_REGISTRY") != ""
}

func publicUpgradeEdition(name string) bool { return name == "" || name == "open" }

func newRegistryClient() *Client {
	registry := defaultNPMRegistry
	if value := os.Getenv("DWS_UPGRADE_REGISTRY"); value != "" {
		registry = strings.TrimRight(value, "/")
	}
	client := &Client{httpClient: &http.Client{Timeout: httpTimeout}, registryURL: registry + "/" + npmPackageName}
	if !publicUpgradeEdition(upgradeEditionName()) && !hasExplicitUpgradeSource() {
		client.configErr = fmt.Errorf("当前发行版未配置升级源，拒绝安装公开 npm 包；请配置该发行版的升级源")
	} else if !validRegistryURL(registry) {
		client.configErr = fmt.Errorf("DWS_UPGRADE_REGISTRY 必须是无凭据、查询参数或片段的 HTTPS Registry 根地址")
	}
	return client
}

// Registry 和 tarball 都只允许 HTTPS；loopback HTTP 专供本机镜像与测试。
func validRegistryURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(raw, "\\ \t\r\n") {
		return false
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return false
		}
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

type npmManifest struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Deprecated string `json:"deprecated"`
	Dist       struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

type npmPackument struct {
	Name     string                 `json:"name"`
	Versions map[string]npmManifest `json:"versions"`
	Time     map[string]string      `json:"time"`
}

func (c *Client) fetchRegistryRelease(ctx context.Context, selector string, track ReleaseTrack) (*ReleaseInfo, error) {
	var manifest npmManifest
	if err := c.getRegistryJSON(ctx, c.registryURL+"/"+url.PathEscape(selector), &manifest, registryManifestLimit); err != nil {
		return nil, fmt.Errorf("获取 npm 版本 %s 失败: %w", selector, err)
	}
	if err := validateNPMManifest(manifest, track); err != nil {
		return nil, err
	}
	if selector != "latest" && selector != "beta" && manifest.Version != selector {
		return nil, fmt.Errorf("npm 返回版本与指定版本不一致")
	}
	return npmManifestToRelease(manifest, strings.TrimSuffix(c.registryURL, "/"+npmPackageName)), nil
}

func validateNPMManifest(manifest npmManifest, track ReleaseTrack) error {
	if manifest.Name != npmPackageName || !validNPMVersion(manifest.Version) {
		return fmt.Errorf("npm 包名或版本无效")
	}
	if manifest.Deprecated != "" {
		return fmt.Errorf("npm 版本 %s 已被弃用", manifest.Version)
	}
	if !npmVersionMatchesTrack(manifest.Version, track) {
		return fmt.Errorf("npm 版本 %s 不属于 %s 轨道", manifest.Version, track)
	}
	if !validRegistryURL(manifest.Dist.Tarball) {
		return fmt.Errorf("npm tarball 地址无效")
	}
	if !validNPMIntegrity(manifest.Dist.Integrity) {
		return fmt.Errorf("npm 包缺少有效的 SHA-512/SHA-256 integrity")
	}
	return nil
}

func validNPMIntegrity(integrity string) bool {
	items := strings.Fields(integrity)
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		algorithm, encoded, found := strings.Cut(item, "-")
		size := 0
		switch algorithm {
		case "sha512":
			size = 64
		case "sha256":
			size = 32
		default:
			return false
		}
		digest, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if !found || err != nil || len(digest) != size {
			return false
		}
	}
	return true
}

func npmManifestToRelease(manifest npmManifest, registry string) *ReleaseInfo {
	// npm 发布工作流将这些归档打入同一包；下载方验证实际存在后才可安装。
	assets := []GitHubAsset{{Name: skillsZipName}, {Name: checksumsName}}
	for _, platform := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"} {
		ext := ".tar.gz"
		if strings.HasPrefix(platform, "windows-") {
			ext = ".zip"
		}
		assets = append(assets, GitHubAsset{Name: "dws-" + platform + ext})
	}
	core, _, _ := strings.Cut(manifest.Version, "+")
	return &ReleaseInfo{
		Version:    manifest.Version,
		Prerelease: strings.Contains(core, "-"),
		Assets:     assets,
		NPM:        &NPMPackage{RegistryURL: registry, Name: manifest.Name, Version: manifest.Version, TarballURL: manifest.Dist.Tarball, Integrity: manifest.Dist.Integrity},
	}
}

func (c *Client) fetchRegistryVersions(ctx context.Context, track ReleaseTrack) ([]VersionEntry, error) {
	var pack npmPackument
	if err := c.getRegistryJSON(ctx, c.registryURL, &pack, registryPackumentLimit); err != nil {
		return nil, err
	}
	if pack.Name != npmPackageName {
		return nil, fmt.Errorf("npm 版本列表包名不匹配")
	}
	versions := make([]VersionEntry, 0, len(pack.Versions))
	for version, manifest := range pack.Versions {
		if manifest.Version != version || validateNPMManifest(manifest, track) != nil {
			continue
		}
		core, _, _ := strings.Cut(version, "+")
		versions = append(versions, VersionEntry{Version: version, Date: formatDate(pack.Time[version]), Prerelease: strings.Contains(core, "-")})
	}
	sort.Slice(versions, func(i, j int) bool {
		a, _, _ := strings.Cut(versions[i].Version, "+")
		b, _, _ := strings.Cut(versions[j].Version, "+")
		if compared := CompareVersions(a, b); compared != 0 {
			return compared > 0
		}
		return versions[i].Version > versions[j].Version
	})
	return versions, nil
}

func (c *Client) getRegistryJSON(ctx context.Context, endpoint string, target any, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	// Registry 从不继承 GitHub Token；每次重定向也执行同一 URL 安全约束。
	client := *c.httpClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !validRegistryURL(req.URL.String()) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
			return fmt.Errorf("npm Registry 重定向无效")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 npm Registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("npm Registry 返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("读取 npm 元数据失败: %w", err)
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("npm 元数据超过大小限制")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 npm 元数据失败: %w", err)
	}
	return nil
}

var npmVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func validNPMVersion(version string) bool {
	match := npmVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return false
	}
	for _, number := range match[1:4] {
		if _, err := strconv.Atoi(number); err != nil {
			return false
		}
	}
	for _, identifier := range strings.Split(match[4], ".") {
		if len(identifier) > 1 && identifier[0] == '0' && strings.Trim(identifier, "0123456789") == "" {
			return false
		}
	}
	return true
}

func npmVersionMatchesTrack(version string, track ReleaseTrack) bool {
	if track == ReleaseTrackAll {
		return true
	}
	_, actual, ok := checkedReleaseVersion(version)
	if track == "" {
		track = ReleaseTrackRelease
	}
	return ok && actual == track
}

// 仅比较已知发行轨道；build metadata 校验后移除，避免旧比较器将其解释为 patch。
var checkedVersionPattern = regexp.MustCompile(`^v?((0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))(-beta(?:\.(0|[1-9][0-9]*))?)?(\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func checkedReleaseVersion(version string) (string, ReleaseTrack, bool) {
	match := checkedVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return "", "", false
	}
	for _, number := range []string{match[2], match[3], match[4], match[6]} {
		if number == "" {
			continue
		}
		if _, err := strconv.Atoi(number); err != nil {
			return "", "", false
		}
	}
	track := ReleaseTrackRelease
	if match[5] != "" {
		track = ReleaseTrackBeta
	}
	return match[1] + match[5], track, true
}
