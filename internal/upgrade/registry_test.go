// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"encoding/json"
	"fmt"
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
		httpClient:   server.Client(),
		owner:        defaultOwner,
		repo:         defaultRepo,
		baseURL:      gitHubAPIBase,
		registryURL:  server.URL,
		assetBaseURL: server.URL + "/releases/download",
		cachePath:    cachePath,
		now:          now,
	}
}

func serveRegistryTestChecksums(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasSuffix(r.URL.Path, "/checksums.txt") {
		return false
	}
	hash := strings.Repeat("a", 64)
	for _, name := range registryDigestAssetNames() {
		_, _ = fmt.Fprintf(w, "%s  %s\n", hash, name)
	}
	return true
}

func TestNewClientUsesRegistryOnlyForOfficialDefaults(t *testing.T) {
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

func TestRegistryReleaseUsesTenMinuteCache(t *testing.T) {
	var requests atomic.Int32
	version := "1.0.58-beta.6"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRegistryTestChecksums(w, r) {
			return
		}
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

func TestRegistryReleaseBuildsVerifiedAssetURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRegistryTestChecksums(w, r) {
			return
		}
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
	if binary.Digest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("binary digest = %q", binary.Digest)
	}
	if FindSkillsAsset(release.Assets) == nil || FindChecksumsAsset(release.Assets) == nil {
		t.Fatal("registry release is missing skills or checksums asset")
	}
}

func TestRegistryCacheKeepsChannelsSeparate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRegistryTestChecksums(w, r) {
			return
		}
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

func TestRegistryReleaseRequiresValidChecksums(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "missing", status: http.StatusNotFound, want: "HTTP 404"},
		{name: "incomplete", status: http.StatusOK, body: strings.Repeat("a", 64) + "  dws-darwin-arm64.tar.gz\n", want: "缺少有效"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(test.body))
					return
				}
				_ = json.NewEncoder(w).Encode(npmPackageMetadata{Version: "1.0.58"})
			}))
			defer server.Close()
			client := newRegistryTestClient(server, "", time.Now)
			_, err := client.FetchLatestStableRelease()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRegistryReleaseRejectsBadMetadata(t *testing.T) {
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

func TestRegistryCacheCorruptionAndFutureTimestampAreIgnored(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRegistryTestChecksums(w, r) {
			return
		}
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

func TestRegistryCacheWrongTrackIsIgnored(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRegistryTestChecksums(w, r) {
			return
		}
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
