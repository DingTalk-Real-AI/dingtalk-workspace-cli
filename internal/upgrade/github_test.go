// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestRelease(t *testing.T) {
	release := GitHubRelease{
		TagName:     "v1.0.6",
		Name:        "v1.0.6",
		Body:        "Bug fixes and improvements",
		Prerelease:  false,
		Draft:       false,
		PublishedAt: "2026-04-01T09:29:05Z",
		Assets: []GitHubAsset{
			{Name: "dws-darwin-arm64.tar.gz", Size: 3942308, BrowserDownloadURL: "https://example.com/dws-darwin-arm64.tar.gz"},
			{Name: "dws-skills.zip", Size: 50000, BrowserDownloadURL: "https://example.com/dws-skills.zip"},
			{Name: "checksums.txt", Size: 534, BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	info, err := client.FetchLatestRelease()
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}

	if info.Version != "1.0.6" {
		t.Errorf("Version = %q, want %q", info.Version, "1.0.6")
	}
	if info.Date != "2026-04-01" {
		t.Errorf("Date = %q, want %q", info.Date, "2026-04-01")
	}
	if len(info.Assets) != 3 {
		t.Errorf("len(Assets) = %d, want 3", len(info.Assets))
	}
}

func TestFetchReleaseByTag(t *testing.T) {
	release := GitHubRelease{
		TagName:     "v1.0.5",
		PublishedAt: "2026-03-25T10:00:00Z",
		Assets: []GitHubAsset{
			{Name: "dws-darwin-arm64.tar.gz", Size: 3800000, BrowserDownloadURL: "https://example.com/dws-darwin-arm64.tar.gz"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases/tags/v1.0.5"
		if r.URL.Path != expected {
			t.Errorf("path = %q, want %q", r.URL.Path, expected)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)

	// Without "v" prefix
	info, err := client.FetchReleaseByTag("1.0.5")
	if err != nil {
		t.Fatalf("FetchReleaseByTag() error = %v", err)
	}
	if info.Version != "1.0.5" {
		t.Errorf("Version = %q, want %q", info.Version, "1.0.5")
	}
}

func TestFetchAllReleases(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v1.0.6", PublishedAt: "2026-04-01T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.6-beta", PublishedAt: "2026-03-28T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.5", PublishedAt: "2026-03-25T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.4-draft", PublishedAt: "2026-03-20T09:00:00Z", Draft: true, Prerelease: false},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	versions, err := client.FetchAllReleases()
	if err != nil {
		t.Fatalf("FetchAllReleases() error = %v", err)
	}

	// Draft should be filtered out
	if len(versions) != 3 {
		t.Fatalf("len(versions) = %d, want 3", len(versions))
	}
	if versions[0].Version != "1.0.6" {
		t.Errorf("versions[0].Version = %q, want %q", versions[0].Version, "1.0.6")
	}
	if versions[1].Prerelease != true {
		t.Errorf("versions[1].Prerelease = false, want true")
	}
}

func TestFetchReleaseVersions_ByTrack(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "delivery/remove-discovery-yolo-latest", PublishedAt: "2026-04-04T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.7-beta.2", PublishedAt: "2026-04-03T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "feat/remove-discovery-v1", PublishedAt: "2026-04-02T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.6", PublishedAt: "2026-04-01T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.7-beta.1", PublishedAt: "2026-03-28T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.5", PublishedAt: "2026-03-25T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.8-beta-draft", PublishedAt: "2026-04-04T09:00:00Z", Draft: true, Prerelease: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)

	stable, err := client.FetchReleaseVersions(ReleaseTrackRelease)
	if err != nil {
		t.Fatalf("FetchReleaseVersions(release) error = %v", err)
	}
	if len(stable) != 2 || stable[0].Version != "1.0.6" || stable[1].Version != "1.0.5" {
		t.Fatalf("stable versions = %#v, want 1.0.6 and 1.0.5", stable)
	}

	beta, err := client.FetchReleaseVersions(ReleaseTrackBeta)
	if err != nil {
		t.Fatalf("FetchReleaseVersions(beta) error = %v", err)
	}
	if len(beta) != 2 || beta[0].Version != "1.0.7-beta.2" || beta[1].Version != "1.0.7-beta.1" {
		t.Fatalf("beta versions = %#v, want beta.2 and beta.1", beta)
	}
}

func TestFetchLatestStableRelease(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "feat/remove-discovery-v1", PublishedAt: "2026-04-02T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.7-beta.1", PublishedAt: "2026-04-01T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.6", PublishedAt: "2026-03-30T09:00:00Z", Draft: false, Prerelease: false},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases"
		if r.URL.Path != expected {
			t.Errorf("path = %q, want %q", r.URL.Path, expected)
		}
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	info, err := client.FetchLatestStableRelease()
	if err != nil {
		t.Fatalf("FetchLatestStableRelease() error = %v", err)
	}
	if info.Version != "1.0.6" {
		t.Errorf("Version = %q, want 1.0.6", info.Version)
	}
	if info.Prerelease {
		t.Error("Prerelease = true, want false")
	}
}

func TestFetchLatestPrerelease(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "delivery/remove-discovery-yolo-latest", PublishedAt: "2026-04-02T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.6", PublishedAt: "2026-04-01T09:00:00Z", Draft: false, Prerelease: false},
		{TagName: "v1.0.7-beta.2", PublishedAt: "2026-03-30T09:00:00Z", Draft: false, Prerelease: true},
		{TagName: "v1.0.7-beta.1", PublishedAt: "2026-03-28T09:00:00Z", Draft: false, Prerelease: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases"
		if r.URL.Path != expected {
			t.Errorf("path = %q, want %q", r.URL.Path, expected)
		}
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	info, err := client.FetchLatestPrerelease()
	if err != nil {
		t.Fatalf("FetchLatestPrerelease() error = %v", err)
	}
	if info.Version != "1.0.7-beta.2" {
		t.Errorf("Version = %q, want 1.0.7-beta.2", info.Version)
	}
	if !info.Prerelease {
		t.Error("Prerelease = false, want true")
	}
}

func TestFetchLatestPrerelease_NotFound(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v1.0.6", Draft: false, Prerelease: false},
		{TagName: "v1.0.7-beta.1", Draft: true, Prerelease: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	if _, err := client.FetchLatestPrerelease(); err == nil {
		t.Fatal("FetchLatestPrerelease() error = nil, want not found")
	}
}

func TestFindBinaryAsset(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "checksums.txt", Size: 534},
		{Name: "dws-darwin-amd64.tar.gz", Size: 4276643},
		{Name: "dws-darwin-arm64.tar.gz", Size: 3942308},
		{Name: "dws-linux-amd64.tar.gz", Size: 4394696},
		{Name: "dws-windows-amd64.zip", Size: 4500000},
		{Name: "dws-skills.zip", Size: 50000},
	}

	tests := []struct {
		goos, goarch string
		wantName     string
		wantErr      bool
	}{
		{"darwin", "arm64", "dws-darwin-arm64.tar.gz", false},
		{"darwin", "amd64", "dws-darwin-amd64.tar.gz", false},
		{"linux", "amd64", "dws-linux-amd64.tar.gz", false},
		{"windows", "amd64", "dws-windows-amd64.zip", false},
		{"freebsd", "amd64", "", true},
	}

	for _, tt := range tests {
		asset, err := FindBinaryAssetFor(assets, tt.goos, tt.goarch)
		if tt.wantErr {
			if err == nil {
				t.Errorf("FindBinaryAssetFor(%s, %s) expected error", tt.goos, tt.goarch)
			}
			continue
		}
		if err != nil {
			t.Errorf("FindBinaryAssetFor(%s, %s) error = %v", tt.goos, tt.goarch, err)
			continue
		}
		if asset.Name != tt.wantName {
			t.Errorf("FindBinaryAssetFor(%s, %s).Name = %q, want %q", tt.goos, tt.goarch, asset.Name, tt.wantName)
		}
	}
}

func TestFindSkillsAsset(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "dws-darwin-arm64.tar.gz"},
		{Name: "dws-skills.zip", Size: 50000},
		{Name: "checksums.txt"},
	}

	skill := FindSkillsAsset(assets)
	if skill == nil {
		t.Fatal("FindSkillsAsset() returned nil")
	}
	if skill.Name != "dws-skills.zip" {
		t.Errorf("Name = %q, want %q", skill.Name, "dws-skills.zip")
	}

	// No skills asset
	noSkills := []GitHubAsset{{Name: "dws-darwin-arm64.tar.gz"}}
	if got := FindSkillsAsset(noSkills); got != nil {
		t.Errorf("FindSkillsAsset() = %v, want nil", got)
	}
}

func TestFindChecksumsAsset(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "dws-darwin-arm64.tar.gz"},
		{Name: "checksums.txt", Size: 534},
	}

	checksums := FindChecksumsAsset(assets)
	if checksums == nil {
		t.Fatal("FindChecksumsAsset() returned nil")
	}
	if checksums.Name != "checksums.txt" {
		t.Errorf("Name = %q, want %q", checksums.Name, "checksums.txt")
	}
}

func TestExtractDigestSHA256(t *testing.T) {
	tests := []struct {
		digest string
		want   string
	}{
		{"sha256:abcdef1234567890", "abcdef1234567890"},
		{"sha256:", ""},
		{"md5:abcdef", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ExtractDigestSHA256(tt.digest)
		if got != tt.want {
			t.Errorf("ExtractDigestSHA256(%q) = %q, want %q", tt.digest, got, tt.want)
		}
	}
}

func TestRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchLatestRelease()
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if got := err.Error(); !contains(got, "频率超限") {
		t.Errorf("error = %q, want to contain rate limit message", got)
	}
}

func TestFetchLatestRelease_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchLatestRelease()
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !contains(err.Error(), "404") {
		t.Errorf("error = %q, want to contain '404'", err.Error())
	}
}

func TestFetchReleaseByTag_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchReleaseByTag("v1.0.0")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchLatestRelease_FieldMapping(t *testing.T) {
	release := GitHubRelease{
		TagName:     "v2.1.0",
		Body:        "## Changelog\n* abc1234 - add new feature\n* def5678 Merge branch 'main'",
		Prerelease:  true,
		PublishedAt: "2026-06-15T14:30:00Z",
		HTMLURL:     "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/tag/v2.1.0",
		Assets:      []GitHubAsset{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	info, err := client.FetchLatestRelease()
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if info.Version != "2.1.0" {
		t.Errorf("Version = %q, want %q", info.Version, "2.1.0")
	}
	if info.Date != "2026-06-15" {
		t.Errorf("Date = %q, want %q", info.Date, "2026-06-15")
	}
	if !info.Prerelease {
		t.Error("Prerelease = false, want true")
	}
	if info.HTMLURL != release.HTMLURL {
		t.Errorf("HTMLURL = %q, want %q", info.HTMLURL, release.HTMLURL)
	}
	if info.Changelog == "" {
		t.Error("Changelog should not be empty")
	}
}

func TestFetchAllReleases_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]GitHubRelease{})
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	versions, err := client.FetchAllReleases()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("len = %d, want 0", len(versions))
	}
}

func TestFetchAllReleases_AllDrafts(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v1.0.0", Draft: true},
		{TagName: "v0.9.0", Draft: true},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	versions, err := client.FetchAllReleases()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("all-drafts: len = %d, want 0", len(versions))
	}
}

func TestFetchReleaseByTag_WithVPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases/tags/v1.0.5"
		if r.URL.Path != expected {
			t.Errorf("path = %q, want %q", r.URL.Path, expected)
		}
		json.NewEncoder(w).Encode(GitHubRelease{TagName: "v1.0.5", PublishedAt: "2026-01-01T00:00:00Z"})
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	info, err := client.FetchReleaseByTag("v1.0.5") // already has "v"
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if info.Version != "1.0.5" {
		t.Errorf("Version = %q, want %q", info.Version, "1.0.5")
	}
}

func TestFindChecksumsAsset_NotFound(t *testing.T) {
	assets := []GitHubAsset{{Name: "dws-darwin-arm64.tar.gz"}}
	if got := FindChecksumsAsset(assets); got != nil {
		t.Errorf("FindChecksumsAsset() = %v, want nil", got)
	}
}

func TestFindBinaryAssetFor_EmptyAssets(t *testing.T) {
	_, err := FindBinaryAssetFor(nil, "darwin", "arm64")
	if err == nil {
		t.Error("expected error for empty assets")
	}
}

func TestStripV(t *testing.T) {
	tests := []struct{ in, want string }{
		{"v1.0.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"v", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripV(tt.in)
		if got != tt.want {
			t.Errorf("stripV(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-04-01T09:29:05Z", "2026-04-01"},
		{"invalid-date", "invalid-date"},
		{"", ""},
	}
	for _, tt := range tests {
		got := formatDate(tt.in)
		if got != tt.want {
			t.Errorf("formatDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		body   string
		maxLen int
		want   string
	}{
		{"short", 100, "short"},
		{"line1\nline2", 100, "line1"},
		{"a very long string here", 10, "a very ..."},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncateBody(tt.body, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateBody(%q, %d) = %q, want %q", tt.body, tt.maxLen, got, tt.want)
		}
	}
}

func TestGithubToken(t *testing.T) {
	// Clean env first
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if got := githubToken(); got != "" {
		t.Errorf("expected empty token, got %q", got)
	}

	t.Setenv("GITHUB_TOKEN", "gh-tok-123")
	if got := githubToken(); got != "gh-tok-123" {
		t.Errorf("expected gh-tok-123, got %q", got)
	}

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-alt-456")
	if got := githubToken(); got != "gh-alt-456" {
		t.Errorf("expected gh-alt-456, got %q", got)
	}
}

func TestRepositoryFromEnv(t *testing.T) {
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	owner, repo, err := repositoryFromEnv()
	if err != nil {
		t.Fatalf("repositoryFromEnv default error = %v", err)
	}
	if owner != defaultOwner || repo != defaultRepo {
		t.Fatalf("repositoryFromEnv default = %s/%s, want %s/%s", owner, repo, defaultOwner, defaultRepo)
	}

	t.Setenv("DWS_UPGRADE_REPOSITORY", "PeterGuy326/dingtalk-workspace-cli")
	owner, repo, err = repositoryFromEnv()
	if err != nil {
		t.Fatalf("repositoryFromEnv owner/repo error = %v", err)
	}
	if owner != "PeterGuy326" || repo != "dingtalk-workspace-cli" {
		t.Fatalf("repositoryFromEnv owner/repo = %s/%s", owner, repo)
	}

	t.Setenv("DWS_UPGRADE_REPOSITORY", "https://github.com/PeterGuy326/dingtalk-workspace-cli.git")
	owner, repo, err = repositoryFromEnv()
	if err != nil {
		t.Fatalf("repositoryFromEnv URL error = %v", err)
	}
	if owner != "PeterGuy326" || repo != "dingtalk-workspace-cli" {
		t.Fatalf("repositoryFromEnv URL = %s/%s", owner, repo)
	}
}

func TestRepositoryFromEnv_Invalid(t *testing.T) {
	t.Setenv("DWS_UPGRADE_REPOSITORY", "not-a-repository")
	client := NewClient()
	if _, err := client.FetchLatestRelease(); err == nil {
		t.Fatal("FetchLatestRelease with invalid DWS_UPGRADE_REPOSITORY error = nil")
	}
}

func TestIsVersionLikeTag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"v1.0.7", true},
		{"1.0.7", true},
		{"v1.0.7-beta.1", true},
		{"v1.0.50-yolo-preview", true},
		{"delivery/remove-discovery-yolo-latest", false},
		{"feat/remove-discovery-v1", false},
		{"v1.0", false},
	}
	for _, tt := range tests {
		if got := isVersionLikeTag(tt.tag); got != tt.want {
			t.Errorf("isVersionLikeTag(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}

	if !isStableVersionTag("v1.0.7") {
		t.Error("isStableVersionTag(v1.0.7) = false, want true")
	}
	if isStableVersionTag("v1.0.7-beta.1") {
		t.Error("isStableVersionTag(v1.0.7-beta.1) = true, want false")
	}
	if !isPrereleaseVersionTag("v1.0.7-beta.1") {
		t.Error("isPrereleaseVersionTag(v1.0.7-beta.1) = false, want true")
	}
	if isPrereleaseVersionTag("v1.0.7") {
		t.Error("isPrereleaseVersionTag(v1.0.7) = true, want false")
	}
}

func TestGetJSON_SetsHeaders(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token-abc")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != userAgent {
			t.Errorf("User-Agent = %q, want %q", ua, userAgent)
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want application/vnd.github+json", accept)
		}
		if auth := r.Header.Get("Authorization"); auth != "token test-token-abc" {
			t.Errorf("Authorization = %q, want %q", auth, "token test-token-abc")
		}
		json.NewEncoder(w).Encode(GitHubRelease{TagName: "v1.0.0", PublishedAt: "2026-01-01T00:00:00Z"})
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchLatestRelease()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
