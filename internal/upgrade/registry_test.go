// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func registryFixture(version string) npmManifest {
	m := npmManifest{Name: npmPackageName, Version: version}
	m.Dist.Tarball = "https://registry.npmjs.org/" + npmPackageName + "/-/" + npmPackageName + "-" + version + ".tgz"
	m.Dist.Integrity = "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))
	return m
}

func registryTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	t.Setenv("DWS_UPGRADE_REGISTRY", srv.URL)
	return NewVersionClient(), srv
}

func TestCrossPlatformCoverageRegistrySourceSelection(t *testing.T) {
	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	t.Setenv("DWS_UPGRADE_REGISTRY", "")
	client := NewVersionClient()
	if client.configErr != nil || client.registryURL != defaultNPMRegistry+"/"+npmPackageName || client.baseURL != "" || client.httpClient.Timeout != httpTimeout {
		t.Fatalf("default source: %#v", client)
	}
	if err := NewClient().validateConfig(); err != nil {
		t.Fatalf("default upgrade configuration rejected: %v", err)
	}
	t.Run("private edition requires explicit source", func(t *testing.T) {
		testseam.Swap(t, &upgradeEditionName, func() string { return "internal" })
		if _, err := NewVersionClient().FetchLatestRelease(); err == nil {
			t.Fatal("private edition accepted public default")
		}
		t.Setenv("DWS_UPGRADE_REGISTRY", "https://mirror.example.com/npm/")
		client := NewVersionClient()
		if client.configErr != nil || client.registryURL != "https://mirror.example.com/npm/"+npmPackageName {
			t.Fatalf("explicit source: %#v", client)
		}
	})
	t.Setenv("DWS_UPGRADE_REGISTRY", "https://mirror.example.com")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "owner/repo")
	client = NewVersionClient()
	if client.registryURL != "" || client.owner != "owner" || client.repo != "repo" || client.baseURL != gitHubAPIBase {
		t.Fatalf("explicit repository lost: %#v", client)
	}
	t.Setenv("DWS_UPGRADE_URL", "https://github-mirror.example/api/")
	if got := NewVersionClient(); got.baseURL != "https://github-mirror.example/api" || got.registryURL != "" {
		t.Fatalf("explicit API lost: %#v", got)
	}
	if got := NewClientWithBaseURL("https://legacy.example/"); got.baseURL != "https://legacy.example" || got.registryURL != "" {
		t.Fatalf("legacy constructor changed: %#v", got)
	}
}

func TestCrossPlatformCoverageRegistryQueriesAndHonestAssets(t *testing.T) {
	var paths []string
	client, srv := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != userAgent || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected registry headers: %v", r.Header)
		}
		version := "1.2.3"
		if strings.HasSuffix(r.URL.Path, "/beta") {
			version = "1.3.0-beta.2"
		}
		if strings.HasSuffix(r.URL.Path, "/1.1.0-rc.1+build.7") {
			version = "1.1.0-rc.1+build.7"
		}
		_ = json.NewEncoder(w).Encode(registryFixture(version))
	})
	t.Setenv("GITHUB_TOKEN", "fixture-token")
	t.Setenv("GH_TOKEN", "fixture-fallback")
	for _, fetch := range []func() (*ReleaseInfo, error){client.FetchLatestRelease, client.FetchLatestStableRelease, func() (*ReleaseInfo, error) { return client.FetchLatestReleaseForTrack("") }} {
		release, err := fetch()
		if err != nil || release.Version != "1.2.3" || release.Prerelease || release.NPM == nil || release.NPM.RegistryURL != srv.URL || release.NPM.Name != npmPackageName || release.NPM.Version != release.Version {
			t.Fatalf("stable = %#v, %v", release, err)
		}
		if len(release.Assets) != 8 || FindSkillsAsset(release.Assets) == nil || FindChecksumsAsset(release.Assets) == nil {
			t.Fatal("missing package assets")
		}
		for _, asset := range release.Assets {
			if asset.BrowserDownloadURL != "" || asset.Digest != "" || asset.Size != 0 {
				t.Fatalf("fabricated asset metadata: %#v", asset)
			}
		}
		for _, platform := range [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
			if _, err := FindBinaryAssetFor(release.Assets, platform[0], platform[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
	release, err := client.FetchLatestPrerelease()
	if err != nil || release.Version != "1.3.0-beta.2" || !release.Prerelease {
		t.Fatalf("beta = %#v, %v", release, err)
	}
	release, err = client.FetchReleaseByTag("v1.1.0-rc.1+build.7")
	if err != nil || release.Version != "1.1.0-rc.1+build.7" || !release.Prerelease {
		t.Fatalf("exact = %#v, %v", release, err)
	}
	if _, err := client.FetchLatestReleaseForTrack(ReleaseTrackAll); err == nil {
		t.Fatal("all accepted for latest")
	}
	if _, err := client.FetchReleaseByTag("../latest"); err == nil {
		t.Fatal("unsafe selector accepted")
	}
	if _, err := client.FetchReleaseByTag("latest"); err == nil {
		t.Fatal("moving dist-tag accepted as exact version")
	}
	want := []string{"/" + npmPackageName + "/latest", "/" + npmPackageName + "/latest", "/" + npmPackageName + "/latest", "/" + npmPackageName + "/beta", "/" + npmPackageName + "/1.1.0-rc.1+build.7"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("queries = %v, want %v", paths, want)
	}
}

func TestCrossPlatformCoverageRegistryVersionList(t *testing.T) {
	pack := npmPackument{Name: npmPackageName, Versions: map[string]npmManifest{}, Time: map[string]string{"1.2.3": "2026-10-01T12:00:00Z"}}
	for _, version := range []string{"1.2.3", "1.2.2", "1.3.0-beta.2", "1.3.0-beta.10", "1.3.0-rc.1", "1.2.3+build.1", "1.2.3+build.2"} {
		pack.Versions[version] = registryFixture(version)
	}
	deprecated := registryFixture("9.0.0")
	deprecated.Deprecated = "withdrawn"
	pack.Versions[deprecated.Version] = deprecated
	pack.Versions["9.1.0"] = registryFixture("9.2.0")
	pack.Versions["invalid"] = registryFixture("invalid")
	client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+npmPackageName {
			t.Errorf("unexpected list path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(pack)
	})
	for _, tc := range []struct {
		track ReleaseTrack
		want  []string
	}{
		{ReleaseTrackRelease, []string{"1.2.3+build.2", "1.2.3+build.1", "1.2.3", "1.2.2"}},
		{"", []string{"1.2.3+build.2", "1.2.3+build.1", "1.2.3", "1.2.2"}},
		{ReleaseTrackBeta, []string{"1.3.0-beta.10", "1.3.0-beta.2"}},
		{ReleaseTrackAll, []string{"1.3.0-rc.1", "1.3.0-beta.10", "1.3.0-beta.2", "1.2.3+build.2", "1.2.3+build.1", "1.2.3", "1.2.2"}},
	} {
		entries, err := client.FetchReleaseVersions(tc.track)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, entry := range entries {
			got = append(got, entry.Version)
			if entry.Version == "1.2.3" && entry.Date != "2026-10-01" {
				t.Fatal(entry)
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s got %v want %v", tc.track, got, tc.want)
		}
	}
	if entries, err := client.FetchAllReleases(); err != nil || len(entries) != 7 {
		t.Fatalf("all entries=%v err=%v", entries, err)
	}
	pack.Name = "different-package"
	if _, err := client.FetchAllReleases(); err == nil {
		t.Fatal("wrong package list accepted")
	}
}

func TestCrossPlatformCoverageRegistryRejectsUntrustedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*npmManifest)
	}{
		{"name", func(m *npmManifest) { m.Name = "other" }},
		{"version", func(m *npmManifest) { m.Version = "01.2.3" }},
		{"deprecated", func(m *npmManifest) { m.Deprecated = "withdrawn" }},
		{"track", func(m *npmManifest) { m.Version = "1.2.3-beta.1" }},
		{"tarball", func(m *npmManifest) { m.Dist.Tarball = "http://cdn.example/package.tgz" }},
		{"integrity", func(m *npmManifest) { m.Dist.Integrity = "sha512-YQ==" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := registryFixture("1.2.3")
			tc.mutate(&m)
			client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(m) })
			if _, err := client.FetchLatestRelease(); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(registryFixture("1.2.3")) })
	if _, err := client.FetchLatestPrerelease(); err == nil {
		t.Fatal("beta tag pointing to stable accepted")
	}
	if _, err := client.FetchReleaseByTag("1.0.0"); err == nil {
		t.Fatal("exact version mismatch accepted")
	}
	for _, raw := range []string{"", "sha1-YQ==", "sha512", "sha512-%%%", "sha256-YQ==", "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64)) + " invalid"} {
		if validNPMIntegrity(raw) {
			t.Errorf("invalid SRI accepted: %q", raw)
		}
	}
	valid256 := "sha256-" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	if !validNPMIntegrity(valid256) || !validNPMIntegrity(valid256+" "+registryFixture("1.0.0").Dist.Integrity) {
		t.Fatal("supported SRI rejected")
	}
	for _, version := range []string{"1.2.3", "1.2.3-beta.2", "1.2.3-rc.1+build.1", "1.2.3-0", "1.2.3-01a"} {
		if !validNPMVersion(version) {
			t.Errorf("valid semver rejected: %s", version)
		}
	}
	for _, version := range []string{"v1.2.3", "1.2.3-01", "1.2.3-rc..1", "1.2.3+", "999999999999999999999999.0.0"} {
		if validNPMVersion(version) {
			t.Errorf("invalid semver accepted: %s", version)
		}
	}
}

func TestCrossPlatformCoverageRegistryURLAndFailures(t *testing.T) {
	for _, raw := range []string{"https://registry.example/npm", "http://localhost:1234", "http://127.0.0.1:1234", "http://[::1]:1234"} {
		if !validRegistryURL(raw) {
			t.Errorf("valid URL rejected: %s", raw)
		}
	}
	for _, raw := range []string{"", "https://", "http://example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com?token=x", "https://example.com?", "https://example.com#x", "https://example.com/a/../b", "https://example.com/./b", "https://example.com/a%2fb", "https://example.com/a\\b", " https://example.com", "https://example.com:bad"} {
		if validRegistryURL(raw) {
			t.Errorf("invalid URL accepted: %s", raw)
		}
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DWS_UPGRADE_URL", "")
			t.Setenv("DWS_UPGRADE_REPOSITORY", "")
			t.Setenv("DWS_UPGRADE_REGISTRY", raw)
			if raw != "" {
				if _, err := NewVersionClient().FetchLatestRelease(); err == nil {
					t.Fatal("invalid configured URL accepted")
				}
			}
		})
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing", "", 404}, {"failed", "private upstream error", 500}, {"json", "{", 200}, {"oversize", strings.Repeat(" ", registryManifestLimit+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			if _, err := client.FetchLatestRelease(); err == nil {
				t.Fatal("failed response accepted")
			}
			if _, err := client.FetchAllReleases(); err == nil && tc.name != "oversize" {
				t.Fatal("failed list response accepted")
			}
		})
	}
	client, srv := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.FetchLatestReleaseForTrackContext(ctx, ReleaseTrackRelease); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
	srv.Close()
	if _, err := client.FetchLatestRelease(); err == nil {
		t.Fatal("unreachable registry accepted")
	}
}

func TestCrossPlatformCoverageRegistryRedirectBoundary(t *testing.T) {
	client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/latest") {
			http.Redirect(w, r, "/manifest", http.StatusFound)
			return
		}
		_ = json.NewEncoder(w).Encode(registryFixture("1.2.3"))
	})
	if _, err := client.FetchLatestRelease(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"http://example.com/manifest", "https://user:pass@example.com/manifest", "/loop"} {
		t.Run(target, func(t *testing.T) {
			client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) })
			if _, err := client.FetchLatestRelease(); err == nil {
				t.Fatal("unsafe/loop redirect accepted")
			}
		})
	}
	// HTTPS 即使重定向到本机 HTTP 也不可降级。
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/manifest", http.StatusFound)
	}))
	defer srv.Close()
	client = &Client{registryURL: srv.URL + "/" + npmPackageName, httpClient: srv.Client()}
	if _, err := client.FetchLatestRelease(); err == nil || !strings.Contains(err.Error(), "重定向无效") {
		t.Fatalf("HTTPS downgrade not blocked: %v", err)
	}
}

func TestCrossPlatformCoverageRegistryRequestAndReadFailures(t *testing.T) {
	client, _ := registryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "{")
	})
	if _, err := client.FetchLatestRelease(); err == nil || !strings.Contains(err.Error(), "读取 npm 元数据失败") {
		t.Fatalf("truncated response not rejected: %v", err)
	}
	var target npmManifest
	if err := client.getRegistryJSON(context.Background(), "://invalid", &target, registryManifestLimit); err == nil {
		t.Fatal("malformed request accepted")
	}
	// 显式 GitHub 配置继续走旧 API，同时透传请求错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer srv.Close()
	t.Setenv("DWS_UPGRADE_URL", srv.URL)
	client = NewVersionClient()
	for _, fetch := range []func() (*ReleaseInfo, error){client.FetchLatestRelease, client.FetchLatestStableRelease, func() (*ReleaseInfo, error) { return client.FetchReleaseByTag("1.2.3") }} {
		if _, err := fetch(); err == nil {
			t.Fatal("GitHub failure disappeared")
		}
	}
}

func TestCrossPlatformCoverageRegistryReleaseTrackSemantics(t *testing.T) {
	for _, tc := range []struct {
		input   string
		version string
		track   ReleaseTrack
	}{
		{"v1.2.3+build.4", "1.2.3", ReleaseTrackRelease},
		{"1.2.3-beta", "1.2.3-beta", ReleaseTrackBeta},
		{"1.2.3-beta.0+build.4", "1.2.3-beta.0", ReleaseTrackBeta},
	} {
		version, track, ok := checkedReleaseVersion(tc.input)
		if !ok || version != tc.version || track != tc.track {
			t.Fatalf("%q: %q %q %v", tc.input, version, track, ok)
		}
	}
	for _, version := range []string{"dev", "1.2.3-rc.1", "1.2.3-beta.01", "1.2.3+", "9999999999999999999999.0.0", "1.2.3-beta.9999999999999999999999"} {
		if _, _, ok := checkedReleaseVersion(version); ok {
			t.Errorf("invalid or unsupported track accepted: %q", version)
		}
	}
}
