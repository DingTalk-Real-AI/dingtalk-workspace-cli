// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newRegistryTestClient(server *httptest.Server, cachePath string, now func() time.Time) *Client {
	return &Client{
		httpClient:  server.Client(),
		owner:       defaultOwner,
		repo:        defaultRepo,
		baseURL:     gitHubAPIBase,
		registryURL: server.URL,
		cachePath:   cachePath,
		now:         now,
	}
}

func TestCrossPlatformCoverageNewClientUsesRegistryOnlyForOfficialDefaults(t *testing.T) {
	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	client := NewClient()
	if client.registryURL != npmRegistryBase {
		t.Fatalf("default registry URL = %q", client.registryURL)
	}

	t.Setenv("DWS_UPGRADE_URL", "https://mirror.example.com/api")
	client = NewClient()
	if client.registryURL != "" {
		t.Fatalf("custom API unexpectedly uses npm registry: %q", client.registryURL)
	}

	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "owner/repo")
	client = NewClient()
	if client.registryURL != "" {
		t.Fatalf("custom repository unexpectedly uses npm registry: %q", client.registryURL)
	}
}

func TestCrossPlatformCoverageRegistryReleaseUsesTenMinuteCache(t *testing.T) {
	var requests atomic.Int32
	version := "1.0.58-beta.6"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/dingtalk-workspace-cli/beta" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("registry request leaked Authorization header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: version})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRegistryTestClient(server, filepath.Join(t.TempDir(), "update-state.json"), func() time.Time { return now })

	first, err := client.FetchLatestReleaseForTrack(ReleaseTrackBeta)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != version || !first.Prerelease {
		t.Fatalf("first release = %#v", first)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests after first fetch = %d", requests.Load())
	}

	version = "1.0.58-beta.7"
	now = now.Add(9*time.Minute + 59*time.Second)
	cached, err := client.FetchLatestReleaseForTrack(ReleaseTrackBeta)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Version != "1.0.58-beta.6" || requests.Load() != 1 {
		t.Fatalf("cached release = %q, requests = %d", cached.Version, requests.Load())
	}

	now = now.Add(time.Second)
	refreshed, err := client.FetchLatestReleaseForTrack(ReleaseTrackBeta)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Version != version || requests.Load() != 2 {
		t.Fatalf("refreshed release = %q, requests = %d", refreshed.Version, requests.Load())
	}
}

func TestCrossPlatformCoverageRegistryFreshReleaseBypassesCache(t *testing.T) {
	var requests atomic.Int32
	version := "1.0.58"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: version})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRegistryTestClient(server, filepath.Join(t.TempDir(), "update-state.json"), func() time.Time { return now })
	if _, err := client.FetchLatestReleaseForTrack(ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}
	version = "1.0.59"
	fresh, err := client.FetchLatestReleaseForTrackFresh(ReleaseTrackRelease)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Version != version || requests.Load() != 2 {
		t.Fatalf("fresh release = %q, requests = %d", fresh.Version, requests.Load())
	}
}

func TestCrossPlatformCoverageFreshReleaseTrackBranches(t *testing.T) {
	betaRegistry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.59-beta.1"})
	}))
	defer betaRegistry.Close()
	registryClient := newRegistryTestClient(betaRegistry, "", time.Now)
	if release, err := registryClient.FetchLatestReleaseForTrackFresh(ReleaseTrackBeta); err != nil || release.Version != "1.0.59-beta.1" {
		t.Fatalf("fresh registry beta = %#v, %v", release, err)
	}

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]GitHubRelease{
			{TagName: "v1.0.59-beta.2", Prerelease: true},
			{TagName: "v1.0.58", Prerelease: false},
		})
	}))
	defer githubServer.Close()
	githubClient := NewClientWithBaseURL(githubServer.URL)
	if release, err := githubClient.FetchLatestReleaseForTrackFresh(ReleaseTrackBeta); err != nil || release.Version != "1.0.59-beta.2" {
		t.Fatalf("fresh GitHub beta = %#v, %v", release, err)
	}
	if release, err := githubClient.FetchLatestReleaseForTrackFresh(ReleaseTrackRelease); err != nil || release.Version != "1.0.58" {
		t.Fatalf("fresh GitHub stable = %#v, %v", release, err)
	}

	badClient := NewClientWithBaseURL(githubServer.URL)
	badClient.configErr = errors.New("bad config")
	for _, track := range []ReleaseTrack{ReleaseTrackRelease, ReleaseTrackBeta} {
		if _, err := badClient.FetchLatestReleaseForTrackFresh(track); err == nil {
			t.Fatalf("config error ignored for %s", track)
		}
	}
	if _, err := githubClient.FetchLatestReleaseForTrackFresh(ReleaseTrackAll); err == nil {
		t.Fatal("unknown fresh track accepted")
	}
}

func TestCrossPlatformCoverageRegistryHelperFailureEdges(t *testing.T) {
	originalHome := upgradeUserHomeDir
	t.Cleanup(func() { upgradeUserHomeDir = originalHome })
	upgradeUserHomeDir = func() (string, error) { return "", errors.New("home") }
	if got := updateCachePath(); got != "" {
		t.Fatalf("cache path = %q", got)
	}
	if _, err := registryChannel(ReleaseTrackAll); err == nil {
		t.Fatal("unknown registry channel accepted")
	}
	if err := validateRegistryVersion("1.0.0", ReleaseTrackAll); err == nil {
		t.Fatal("unknown validation track accepted")
	}
	client := &Client{registryURL: "%", now: time.Now, httpClient: http.DefaultClient}
	if _, err := client.fetchRegistryRelease(ReleaseTrackRelease, false); err == nil {
		t.Fatal("invalid registry URL accepted")
	}
	client.registryURL = "https://registry.invalid"
	client.httpClient = &http.Client{Transport: downloadRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network")
	})}
	if _, err := client.fetchRegistryRelease(ReleaseTrackRelease, false); err == nil || !strings.Contains(err.Error(), "无法连接") {
		t.Fatalf("registry network error = %v", err)
	}
	if _, err := client.fetchRegistryRelease(ReleaseTrackAll, false); err == nil {
		t.Fatal("unknown fetch track accepted")
	}
}

func TestCrossPlatformCoverageRegistryReleaseBuildsVerifiedAssetURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dingtalk-workspace-cli/latest" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.58"})
	}))
	defer server.Close()

	client := newRegistryTestClient(server, "", time.Now)
	release, err := client.FetchLatestRelease()
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.0.58" || release.Prerelease {
		t.Fatalf("release = %#v", release)
	}
	if release.HTMLURL != "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/tag/v1.0.58" {
		t.Fatalf("release URL = %q", release.HTMLURL)
	}
	binary, err := FindBinaryAssetFor(release.Assets, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/releases/download/v1.0.58/dws-darwin-arm64.tar.gz"; !strings.HasSuffix(binary.BrowserDownloadURL, want) {
		t.Fatalf("binary URL = %q", binary.BrowserDownloadURL)
	}
	if FindSkillsAsset(release.Assets) == nil || FindChecksumsAsset(release.Assets) == nil {
		t.Fatal("registry release is missing skills or checksums asset")
	}
}

func TestCrossPlatformCoverageRegistryCacheKeepsChannelsSeparate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dingtalk-workspace-cli/latest":
			_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.58"})
		case "/dingtalk-workspace-cli/beta":
			_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.59-beta.1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "update-state.json")
	client := newRegistryTestClient(server, cachePath, time.Now)
	stable, err := client.FetchLatestStableRelease()
	if err != nil {
		t.Fatal(err)
	}
	beta, err := client.FetchLatestPrerelease()
	if err != nil {
		t.Fatal(err)
	}
	if stable.Version != "1.0.58" || beta.Version != "1.0.59-beta.1" {
		t.Fatalf("stable/beta = %q/%q", stable.Version, beta.Version)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var state updateCacheState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Channels) != 2 {
		t.Fatalf("cached channels = %#v", state.Channels)
	}
}

func TestCrossPlatformCoverageRegistryReleaseRejectsBadMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		track  ReleaseTrack
		want   string
	}{
		{name: "http", status: http.StatusBadGateway, track: ReleaseTrackRelease, want: "HTTP 502"},
		{name: "invalid-json", status: http.StatusOK, body: `{`, track: ReleaseTrackRelease, want: "解析 npm registry"},
		{name: "empty", status: http.StatusOK, body: `{}`, track: ReleaseTrackRelease, want: "空版本"},
		{name: "stable-on-beta", status: http.StatusOK, body: `{"version":"1.0.58"}`, track: ReleaseTrackBeta, want: "无效预发布版本"},
		{name: "beta-on-latest", status: http.StatusOK, body: `{"version":"1.0.59-beta.1"}`, track: ReleaseTrackRelease, want: "无效正式版本"},
		{name: "deprecated", status: http.StatusOK, body: `{"version":"1.0.58","deprecated":"withdrawn"}`, track: ReleaseTrackRelease, want: "已弃用版本"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := test.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := newRegistryTestClient(server, "", time.Now)
			_, err := client.FetchLatestReleaseForTrack(test.track)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRegistryCacheCorruptionAndFutureTimestampAreIgnored(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.58"})
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "update-state.json")
	if err := os.WriteFile(cachePath, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRegistryTestClient(server, cachePath, func() time.Time { return now })
	if _, err := client.FetchLatestStableRelease(); err != nil {
		t.Fatal(err)
	}

	future := updateCacheState{Channels: map[string]updateCacheEntry{
		"latest": {Version: "9.9.9", CheckedAt: now.Add(time.Minute).Unix()},
	}}
	data, _ := json.Marshal(future)
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchLatestStableRelease(); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestCrossPlatformCoverageRegistryCacheWrongTrackIsIgnored(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.59-beta.2"})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "update-state.json")
	state := updateCacheState{Channels: map[string]updateCacheEntry{
		"beta": {Version: "1.0.58", CheckedAt: now.Unix()},
	}}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	client := newRegistryTestClient(server, cachePath, func() time.Time { return now })
	release, err := client.FetchLatestPrerelease()
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.0.59-beta.2" || requests.Load() != 1 {
		t.Fatalf("release = %q, requests = %d", release.Version, requests.Load())
	}
}
