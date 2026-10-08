// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func versionCheckServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("DWS_UPGRADE_URL", srv.URL)
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	return srv
}

func TestCrossPlatformCoverageVersionCheckTracksAndSemver(t *testing.T) {
	var calls atomic.Int32
	versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/repos/"+defaultOwner+"/"+defaultRepo+"/releases" || r.URL.RawQuery != "per_page=100" {
			t.Errorf("unexpected release source: %s", r.URL)
		}
		_, _ = w.Write([]byte(`[
		 {"tag_name":"v9.0.0","draft":true},
		 {"tag_name":"v8.0.0-rc.1","prerelease":true},
		 {"tag_name":"v7.0.0-beta.1","prerelease":false},
		 {"tag_name":"v6.0.0","prerelease":true},
		 {"tag_name":"v1.2.4-beta.10+build.1","prerelease":true},
		 {"tag_name":"v1.2.3+build.2","prerelease":false}
		]`))
	})
	for _, tc := range []struct {
		current                 string
		track                   ReleaseTrack
		status, latest, command string
	}{
		{"v1.2.2", "", CheckStatusUpdateAvailable, "1.2.3", "dws upgrade"},
		{"1.2.3+build.1", "", CheckStatusUpToDate, "1.2.3", "dws upgrade"},
		{"1.2.4", "", CheckStatusUpToDate, "1.2.3", "dws upgrade"},
		{"1.2.4-beta.2", "", CheckStatusUpdateAvailable, "1.2.4-beta.10", "dws upgrade --beta"},
		{"v1.2.4-beta.10+build.2", "", CheckStatusUpToDate, "1.2.4-beta.10", "dws upgrade --beta"},
		{"1.2.3", ReleaseTrackBeta, CheckStatusUpdateAvailable, "1.2.4-beta.10", "dws upgrade --beta"},
		{"1.2.3-beta.1", ReleaseTrackRelease, CheckStatusUpdateAvailable, "1.2.3", "dws upgrade"},
	} {
		t.Run(tc.current+string(tc.track), func(t *testing.T) {
			got := CheckVersion(context.Background(), tc.current, CheckOptions{Track: tc.track})
			if got.Current != tc.current || got.Status != tc.status || got.Latest != tc.latest || got.UpgradeCommand() != tc.command || got.CheckedAt == "" || got.Cached {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
	before := calls.Load()
	for _, version := range []string{"", "dev", "DEV", "unknown", "1.2", "1.2.-3", "01.2.3", "1.02.3", "1.2.03", "v1.2.3-1-g1234567", "1.2.3-dirty", "1.2.3-rc.1", "1.2.3-qwenwork.1", "1.2.3-beta.01", "1.2.3-beta.1.dirty", "1.2.3+", "1.2.3+a..b", " 1.2.3", "999999999999999999999999.0.0", "1.2.3-beta.9999999999999999999999"} {
		got := CheckVersion(context.Background(), version, CheckOptions{})
		if got.Status != CheckStatusSkipped || got.Latest != "" || got.CheckedAt != "" {
			t.Errorf("invalid version %q: %#v", version, got)
		}
	}
	if got := CheckVersion(context.Background(), "1.2.3", CheckOptions{Track: ReleaseTrackAll}); got.Status != CheckStatusSkipped {
		t.Fatal(got)
	}
	if calls.Load() != before {
		t.Fatal("skipped versions performed network requests")
	}
}

func TestCrossPlatformCoverageVersionCheckCacheIsolation(t *testing.T) {
	var calls atomic.Int32
	srv := versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.0"},{"tag_name":"v2.1.0-beta.1","prerelease":true}]`))
	})
	dir := t.TempDir()
	opts := CheckOptions{CacheDir: dir, Edition: "public"}
	check := func(current string, wantCalls int32, wantCached bool) {
		t.Helper()
		got := CheckVersion(context.Background(), current, opts)
		if got.Status != CheckStatusUpdateAvailable || got.Cached != wantCached || calls.Load() != wantCalls {
			t.Fatalf("result=%#v calls=%d want=%d", got, calls.Load(), wantCalls)
		}
	}
	check("1.0.0", 1, false)
	check("v1.0.0+local", 1, true)
	check("1.0.1", 2, false)
	opts.Edition = "another"
	check("1.0.1", 3, false)
	t.Setenv("DWS_UPGRADE_REPOSITORY", "another/repo")
	check("1.0.1", 4, false)
	t.Setenv("DWS_UPGRADE_URL", srv.URL+"/mirror")
	check("1.0.1", 5, false)
	opts.Track = ReleaseTrackBeta
	check("1.0.1", 6, false)
	opts.Force = true
	check("1.0.1", 7, false)
	opts.ReadOnly = true
	check("1.0.1", 7, true)
}

func TestCrossPlatformCoverageVersionCheckReadOnlyAndStaleCache(t *testing.T) {
	var calls atomic.Int32
	srv := versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	dir := filepath.Join(t.TempDir(), "absent")
	opts := CheckOptions{CacheDir: dir, ReadOnly: true, Force: true}
	got := CheckVersion(context.Background(), "1.0.0", opts)
	if got.Status != CheckStatusUnknown || calls.Load() != 0 {
		t.Fatalf("read-only missing cache: %#v", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read-only created cache directory: %v", err)
	}
	key := versionCheckKey{Source: srv.URL, Repository: defaultOwner + "/" + defaultRepo, Track: ReleaseTrackRelease, Current: "1.0.0"}
	for _, tc := range []struct {
		name   string
		delta  time.Duration
		latest string
	}{
		{"expired_equal", -25 * time.Hour, "1.0.0"},
		{"expired_newer", -25 * time.Hour, "2.0.0"},
		{"future", time.Hour, "1.0.0"},
		{"bad_version", -time.Hour, "broken"},
		{"wrong_track", -time.Hour, "2.0.0-beta.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeVersionCheckCache(dir, versionCheckCache{Key: key, Latest: tc.latest, CheckedAt: time.Now().Add(tc.delta)})
			path := filepath.Join(dir, versionCheckFile)
			before, _ := os.ReadFile(path)
			got := CheckVersion(context.Background(), "1.0.0", opts)
			after, _ := os.ReadFile(path)
			if got.Status != CheckStatusUnknown || got.Latest != "" || got.CheckedAt != "" || got.Cached || calls.Load() != 0 || string(before) != string(after) {
				t.Fatalf("stale readonly result=%#v", got)
			}
		})
	}
	writeVersionCheckCache(dir, versionCheckCache{Key: key, Latest: "1.0.0", CheckedAt: time.Now().Add(-time.Hour)})
	opts.ReadOnly = false
	if got := CheckVersion(context.Background(), "1.0.0", opts); got.Status != CheckStatusUnknown || got.Cached {
		t.Fatalf("forced failed check reused cache: %#v", got)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

func TestCrossPlatformCoverageVersionCheckFailuresAreUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"http_error", "", http.StatusInternalServerError},
		{"bad_json", "not-json", http.StatusOK},
		{"no_releases", "[]", http.StatusOK},
		{"no_matching_track", `[{"tag_name":"v2.0.0-beta.1","prerelease":true}]`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			got := CheckVersion(context.Background(), "1.0.0", CheckOptions{CacheDir: t.TempDir()})
			if got.Status != CheckStatusUnknown || got.Latest != "" {
				t.Fatalf("failure result: %#v", got)
			}
		})
	}
	t.Run("invalid_source", func(t *testing.T) {
		for _, source := range []string{"bad url", "file:///tmp/version", "https://user:secret@example.com", "https://example.com?query=1", "https://example.com#part"} {
			t.Setenv("DWS_UPGRADE_URL", source)
			if got := CheckVersion(context.Background(), "1.0.0", CheckOptions{}); got.Status != CheckStatusUnknown {
				t.Fatalf("invalid source: %#v", got)
			}
		}
	})
	t.Run("invalid_repository", func(t *testing.T) {
		t.Setenv("DWS_UPGRADE_REPOSITORY", "invalid")
		if got := CheckVersion(context.Background(), "1.0.0", CheckOptions{}); got.Status != CheckStatusUnknown {
			t.Fatal(got)
		}
	})
	t.Run("unwritable_cache", func(t *testing.T) {
		versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[{"tag_name":"v2.0.0"}]`)) })
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, []byte("retain"), 0600); err != nil {
			t.Fatal(err)
		}
		got := CheckVersion(context.Background(), "1.0.0", CheckOptions{CacheDir: path})
		if got.Status != CheckStatusUpdateAvailable {
			t.Fatal(got)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "retain" {
			t.Fatal("cache failure overwrote existing file")
		}
	})
	t.Run("corrupt_cache", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, versionCheckFile), []byte("bad-json"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := CheckVersion(context.Background(), "1.0.0", CheckOptions{CacheDir: dir, ReadOnly: true}); got.Status != CheckStatusUnknown {
			t.Fatal(got)
		}
	})
}

func TestCrossPlatformCoverageVersionCheckNetworkDeadline(t *testing.T) {
	var calls atomic.Int32
	versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() })
	start := time.Now()
	got := CheckVersion(context.Background(), "1.0.0", CheckOptions{})
	if got.Status != CheckStatusUnknown || calls.Load() != 1 {
		t.Fatalf("timeout result: %#v, requests=%d", got, calls.Load())
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("one-second budget elapsed %s", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = CheckVersion(ctx, "1.0.0", CheckOptions{})
	if got.Status != CheckStatusUnknown || calls.Load() != 1 {
		t.Fatalf("cancelled check sent request: %#v calls=%d", got, calls.Load())
	}
}

func TestCrossPlatformCoverageVersionCheckWireShape(t *testing.T) {
	data, err := json.Marshal(CheckResult{Current: "dev", Status: CheckStatusSkipped, Track: ReleaseTrackBeta})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"current":"dev","status":"skipped"}` {
		t.Fatalf("unexpected optional fields: %s", data)
	}
	if !strings.Contains((CheckResult{Track: ReleaseTrackBeta}).UpgradeCommand(), "--beta") {
		t.Fatal("beta command lost track")
	}
}

func TestCrossPlatformCoverageVersionCheckNeverSendsGitHubCredentials(t *testing.T) {
	headers := make(chan string, 4)
	var calls atomic.Int32
	versionCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		headers <- r.Header.Get("Authorization")
		if strings.HasSuffix(r.URL.Path, "/latest") {
			_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
			return
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.0"}]`))
	})
	for _, tc := range []struct{ github, gh string }{
		{"fake-github-token", "fake-gh-token"},
		{"", "fake-gh-token"},
	} {
		t.Setenv("GITHUB_TOKEN", tc.github)
		t.Setenv("GH_TOKEN", tc.gh)
		got := CheckVersion(context.Background(), "1.0.0", CheckOptions{})
		authorization := <-headers
		if got.Status != CheckStatusUpdateAvailable || authorization != "" {
			t.Fatalf("automatic check sent authorization or failed: status=%s header_present=%t", got.Status, authorization != "")
		}
		// 显式升级仍使用原有 token 优先级，不能让检查修复改变升级行为。
		if _, err := NewClient().FetchLatestRelease(); err != nil {
			t.Fatal(err)
		}
		authorization = <-headers
		want := tc.github
		if want == "" {
			want = tc.gh
		}
		if authorization != "token "+want {
			t.Fatal("explicit upgrade lost configured authorization")
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("requests=%d, want 4", calls.Load())
	}
}

func TestCrossPlatformCoverageVersionCheckCachePublicationFailures(t *testing.T) {
	cached := versionCheckCache{Latest: "2.0.0", CheckedAt: time.Now()}
	t.Run("marshal_failure", func(t *testing.T) {
		dir := t.TempDir()
		invalid := cached
		invalid.CheckedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		writeVersionCheckCache(dir, invalid)
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Fatalf("invalid cache was published: entries=%v err=%v", entries, err)
		}
	})
	for _, mode := range []string{"create_failure", "write_failure"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, versionCheckFile)
			if err := os.WriteFile(path, []byte("retain-existing-cache"), 0600); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &createVersionCheckTemp, func(dir, pattern string) (*os.File, error) {
				if mode == "create_failure" {
					return nil, errors.New("disk unavailable")
				}
				file, err := os.CreateTemp(dir, pattern)
				if err == nil {
					_ = file.Close()
				}
				return file, err
			})
			writeVersionCheckCache(dir, cached)
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "retain-existing-cache" {
				t.Fatalf("failed publication damaged existing cache: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed publication left staging files: entries=%v err=%v", entries, err)
			}
		})
	}
}
