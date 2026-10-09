// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	CheckStatusUpToDate        = "up_to_date"
	CheckStatusUpdateAvailable = "update_available"
	CheckStatusUnknown         = "unknown"
	CheckStatusSkipped         = "skipped"
	versionCheckTTL            = 24 * time.Hour
	versionCheckBudget         = time.Second
	versionCheckFile           = "version-check.json"
)

// CheckOptions 控制版本检查；空 CacheDir 禁用持久缓存，空 Track 按当前版本选择轨道。
// ReadOnly 优先于 Force，只读取有效缓存，不发网络请求或写入文件。
type CheckOptions struct {
	CacheDir string
	Edition  string
	Track    ReleaseTrack
	ReadOnly bool
	Force    bool
}

// CheckResult 是检查事实，不改变业务命令的成功状态或退出码。
// unknown 表示无法确认是否最新；Cached=true 表示使用了仍在有效期内的检查结果。
type CheckResult struct {
	Current   string       `json:"current"`
	Latest    string       `json:"latest,omitempty"`
	Status    string       `json:"status"`
	CheckedAt string       `json:"checked_at,omitempty"`
	Cached    bool         `json:"cached,omitempty"`
	Track     ReleaseTrack `json:"-"`
}

// UpgradeCommand 返回与检查轨道一致的升级动作，本方法不会执行升级。
func (r CheckResult) UpgradeCommand() string {
	if r.Track == ReleaseTrackBeta {
		return "dws upgrade --beta"
	}
	return "dws upgrade"
}

type versionCheckKey struct {
	Source     string       `json:"source"`
	Repository string       `json:"repository"`
	Edition    string       `json:"edition"`
	Track      ReleaseTrack `json:"track"`
	Current    string       `json:"current"`
}

type versionCheckCache struct {
	Key       versionCheckKey `json:"key"`
	Latest    string          `json:"latest"`
	CheckedAt time.Time       `json:"checked_at"`
}

// CheckVersion 在一秒网络预算内检查发行版本。检查与缓存错误均返回 unknown，
// 不输出诊断、不安装升级，也不把陈旧缓存当作当前发行状态。
func CheckVersion(ctx context.Context, current string, opts CheckOptions) CheckResult {
	result := CheckResult{Current: current, Status: CheckStatusSkipped}
	normalized, currentTrack, ok := checkedReleaseVersion(current)
	if !ok {
		return result
	}
	track := opts.Track
	if track == "" {
		track = currentTrack
	}
	if track != ReleaseTrackRelease && track != ReleaseTrackBeta {
		return result
	}
	result.Track = track
	result.Status = CheckStatusUnknown
	client := NewVersionClient()
	client.anonymous = true
	if client.validateConfig() != nil || (!publicUpgradeEdition(opts.Edition) && !hasExplicitUpgradeSource()) {
		return result
	}
	source, repository := client.baseURL, client.owner+"/"+client.repo
	if client.registryURL != "" {
		source, repository = client.registryURL, npmPackageName
	}
	if !validCheckSource(source) {
		return result
	}
	key := versionCheckKey{
		Source: source, Repository: repository,
		Edition: opts.Edition, Track: track, Current: normalized,
	}
	if opts.ReadOnly || !opts.Force {
		if cached, ok := readVersionCheckCache(opts.CacheDir, key, time.Now()); ok {
			return checkedVersionResult(result, cached.Latest, cached.CheckedAt, true)
		}
	}
	if opts.ReadOnly {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, versionCheckBudget)
	defer cancel()
	client.httpClient.Timeout = versionCheckBudget
	latest, ok := client.latestCheckVersion(ctx, track)
	if !ok {
		return result
	}
	checkedAt := time.Now().UTC()
	writeVersionCheckCache(opts.CacheDir, versionCheckCache{Key: key, Latest: latest, CheckedAt: checkedAt})
	return checkedVersionResult(result, latest, checkedAt, false)
}

func (c *Client) latestCheckVersion(ctx context.Context, track ReleaseTrack) (string, bool) {
	if c.registryURL != "" {
		release, err := c.FetchLatestReleaseForTrackContext(ctx, track)
		if err != nil {
			return "", false
		}
		latest, actualTrack, ok := checkedReleaseVersion(release.Version)
		return latest, ok && actualTrack == track
	}
	// 兼容自定义 GitHub 源中的历史标签，同时只检查已知的 stable/beta 轨道。
	releases, err := c.fetchReleasesContext(ctx)
	if err != nil {
		return "", false
	}
	for _, release := range releases {
		latest, actualTrack, valid := checkedReleaseVersion(release.TagName)
		release.TagName = latest
		if valid && actualTrack == track && releaseMatchesTrack(release, track) {
			return latest, true
		}
	}
	return "", false
}

func checkedVersionResult(result CheckResult, latest string, checkedAt time.Time, cached bool) CheckResult {
	current, _, _ := checkedReleaseVersion(result.Current)
	result.Latest = latest
	result.CheckedAt = checkedAt.Format(time.RFC3339Nano)
	result.Cached = cached
	result.Status = CheckStatusUpToDate
	if NeedsUpgrade(current, latest) {
		result.Status = CheckStatusUpdateAvailable
	}
	return result
}

func readVersionCheckCache(dir string, key versionCheckKey, now time.Time) (versionCheckCache, bool) {
	var cached versionCheckCache
	if dir == "" {
		return cached, false
	}
	data, err := os.ReadFile(filepath.Join(dir, versionCheckFile))
	if err != nil || json.Unmarshal(data, &cached) != nil || cached.Key != key {
		return cached, false
	}
	latest, track, valid := checkedReleaseVersion(cached.Latest)
	age := now.Sub(cached.CheckedAt)
	if !valid || track != key.Track || age < 0 || age >= versionCheckTTL {
		return cached, false
	}
	cached.Latest = latest
	return cached, true
}

var createVersionCheckTemp = os.CreateTemp

func writeVersionCheckCache(dir string, cached versionCheckCache) {
	if dir == "" || os.MkdirAll(dir, 0700) != nil {
		return
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	file, err := createVersionCheckTemp(dir, ".version-check-*.json")
	if err != nil {
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(name, filepath.Join(dir, versionCheckFile))
	}
}

func validCheckSource(source string) bool {
	u, err := url.Parse(source)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
