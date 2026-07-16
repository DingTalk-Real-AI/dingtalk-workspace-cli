// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package upgrade

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type upgradeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f upgradeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type upgradeFailWriter struct{}

func (upgradeFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCrossPlatformCoverageDownloaderEdgeCases(t *testing.T) {
	badParent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadWithConfig(t.Context(), "http://example.test", filepath.Join(badParent, "out"), DefaultDownloadConfig()); err == nil {
		t.Fatal("download mkdir error was ignored")
	}
	if _, err := doDownload(t.Context(), ":", filepath.Join(t.TempDir(), "out"), DefaultDownloadConfig()); err == nil {
		t.Fatal("invalid download URL accepted")
	}

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(w, "data")
	}))
	defer okServer.Close()
	cfg := DefaultDownloadConfig()
	cfg.MaxRetries = 0
	if n, err := DownloadWithConfig(t.Context(), okServer.URL, filepath.Join(t.TempDir(), "out"), cfg); err != nil || n != 4 {
		t.Fatalf("download with normalized retries = %d, %v", n, err)
	}
	if _, err := doDownload(t.Context(), okServer.URL, t.TempDir(), DefaultDownloadConfig()); err == nil {
		t.Fatal("download to directory accepted")
	}
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	defer statusServer.Close()
	if _, err := doDownload(t.Context(), statusServer.URL, filepath.Join(t.TempDir(), "out"), DefaultDownloadConfig()); err == nil {
		t.Fatal("bad HTTP status accepted")
	}

	var retries atomic.Int32
	retryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if retries.Add(1) == 1 {
			w.Header().Set("Content-Length", "10")
			_, _ = io.WriteString(w, "x")
			return
		}
		_, _ = io.WriteString(w, "done")
	}))
	defer retryServer.Close()
	cfg = DefaultDownloadConfig()
	cfg.MaxRetries = 2
	if _, err := DownloadWithConfig(t.Context(), retryServer.URL, filepath.Join(t.TempDir(), "out"), cfg); err != nil {
		t.Fatalf("retry download: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "x")
		cancel()
	}))
	defer cancelServer.Close()
	cfg = DefaultDownloadConfig()
	cfg.MaxRetries = 2
	if _, err := DownloadWithConfig(ctx, cancelServer.URL, filepath.Join(t.TempDir(), "out"), cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retry = %v", err)
	}

	callbacks := 0
	cfg = DefaultDownloadConfig()
	cfg.ProgressInterval = -time.Second
	cfg.ProgressCallback = func(downloaded, total int64) { callbacks++ }
	if _, err := DownloadWithConfig(t.Context(), okServer.URL, filepath.Join(t.TempDir(), "progress"), cfg); err != nil || callbacks < 2 {
		t.Fatalf("progress callbacks = %d, %v", callbacks, err)
	}
	var shown bool
	if _, err := DownloadWithProgress(t.Context(), okServer.URL, filepath.Join(t.TempDir(), "show"), func(percent float64, downloaded, total int64) {
		shown = percent == 100 && downloaded == total
	}); err != nil || !shown {
		t.Fatalf("progress display = %v, %v", shown, err)
	}
	if _, err := DownloadWithProgress(t.Context(), okServer.URL, filepath.Join(t.TempDir(), "nil-show"), nil); err != nil {
		t.Fatal(err)
	}

	pw := &progressWriter{writer: upgradeFailWriter{}, callback: func(int64, int64) {}, interval: -time.Second}
	if _, err := pw.Write([]byte("x")); err == nil {
		t.Fatal("progress writer ignored underlying error")
	}
	if !isRetriable(&os.PathError{Op: "read", Path: "x", Err: syscall.ETIMEDOUT}) || isRetriable(errors.New("permanent")) {
		t.Fatal("retry classification failed")
	}
}

func TestCrossPlatformCoverageGitHubClientEdgeCases(t *testing.T) {
	t.Setenv("DWS_UPGRADE_URL", "http://mirror.test/")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "owner/repo")
	client := NewClient()
	if client.baseURL != "http://mirror.test" || client.owner != "owner" {
		t.Fatalf("environment client = %#v", client)
	}
	if _, err := client.FetchLatestReleaseForTrack("unknown"); err == nil {
		t.Fatal("unknown release track accepted")
	}
	if _, err := FindBinaryAsset(nil); err == nil {
		t.Fatal("missing host binary asset accepted")
	}
	if got := binaryNameFor("windows"); got != "dws.exe" || binaryNameFor("linux") != "dws" {
		t.Fatalf("binary names = %q / %q", got, binaryNameFor("linux"))
	}
	for tag := range map[string]bool{"1.2": true, "1..3": true, "1.x.3": true, "1.2.3.4": true} {
		if isVersionLikeTag(tag) {
			t.Fatalf("invalid version-like tag %q accepted", tag)
		}
	}
	if truncateBytes([]byte("abcd"), 2) != "ab..." || truncateBytes([]byte("ab"), 2) != "ab" {
		t.Fatal("byte truncation failed")
	}

	releases := `[
		{"tag_name":"v1.2.3","prerelease":false,"draft":false},
		{"tag_name":"v1.3.0-beta.1","prerelease":true,"draft":false},
		{"tag_name":"nightly","prerelease":true,"draft":false},
		{"tag_name":"v9.0.0","prerelease":false,"draft":true}
	]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_, _ = io.WriteString(w, `{"tag_name":"v2.0.0"}`)
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			_, _ = io.WriteString(w, `{"tag_name":"v1.0.0"}`)
		default:
			_, _ = io.WriteString(w, releases)
		}
	}))
	defer server.Close()
	client = NewClientWithBaseURL(server.URL)
	if _, err := client.FetchLatestReleaseForTrack(ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchLatestReleaseForTrack(ReleaseTrackBeta); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchLatestReleaseForTrack(""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchReleaseByTag("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if versions, err := client.FetchReleaseVersions("unknown"); err != nil || len(versions) != 0 {
		t.Fatalf("unknown track versions = %#v, %v", versions, err)
	}

	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	defer emptyServer.Close()
	client = NewClientWithBaseURL(emptyServer.URL)
	if _, err := client.FetchLatestStableRelease(); err == nil {
		t.Fatal("missing stable release accepted")
	}
	if _, err := client.FetchLatestPrerelease(); err == nil {
		t.Fatal("missing beta release accepted")
	}

	t.Setenv("DWS_UPGRADE_REPOSITORY", "invalid")
	invalidConfig := NewClient()
	if _, err := invalidConfig.FetchLatestStableRelease(); err == nil {
		t.Fatal("invalid repository config accepted for stable fetch")
	}
	if _, err := invalidConfig.FetchLatestPrerelease(); err == nil {
		t.Fatal("invalid repository config accepted for beta fetch")
	}
	if _, err := invalidConfig.FetchReleaseVersions(ReleaseTrackAll); err == nil {
		t.Fatal("invalid repository config accepted for versions")
	}
	if _, err := invalidConfig.FetchReleaseByTag("v1"); err == nil {
		t.Fatal("invalid repository config accepted for tag")
	}
}

func TestCrossPlatformCoverageGitHubGetJSONMatrix(t *testing.T) {
	client := NewClientWithBaseURL("")
	if err := client.getJSON(":", &map[string]any{}); err == nil {
		t.Fatal("invalid request URL accepted")
	}
	client.httpClient = &http.Client{Transport: upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	if err := client.getJSON("http://example.test", &map[string]any{}); err == nil {
		t.Fatal("transport error ignored")
	}
	for name, handler := range map[string]http.HandlerFunc{
		"rate": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "limited", http.StatusForbidden)
		},
		"not-found": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "missing", http.StatusNotFound) },
		"server": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, strings.Repeat("x", 250), http.StatusInternalServerError)
		},
		"invalid-json": func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{`) },
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			c := NewClientWithBaseURL(server.URL)
			if err := c.getJSON(server.URL, &map[string]any{}); err == nil {
				t.Fatal("getJSON error response accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradePathsAndSkillsEdges(t *testing.T) {
	originalHome := upgradeUserHomeDir
	originalDirs := append([]string(nil), knownSkillDirs...)
	t.Cleanup(func() {
		upgradeUserHomeDir = originalHome
		knownSkillDirs = originalDirs
	})
	failure := errors.New("home failed")
	upgradeUserHomeDir = func() (string, error) { return "", failure }
	if _, err := UpgradeSkillLocations(t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("skill home error = %v", err)
	}
	if err := EnsureUpgradeDirectories(); !errors.Is(err, failure) {
		t.Fatalf("directory home error = %v", err)
	}

	home := t.TempDir()
	upgradeUserHomeDir = func() (string, error) { return home, nil }
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".present"), 0o755); err != nil {
		t.Fatal(err)
	}
	knownSkillDirs = []string{".agents/skills", ".real/skills", ".missing/skills", ".present/skills"}
	result, err := UpgradeSkillLocations(source)
	if err != nil || len(result.Succeeded()) != 2 || len(result.Failed()) != 0 {
		t.Fatalf("skill upgrade = %#v, %v", result, err)
	}
	knownSkillDirs = []string{".real/skills"}
	result, err = UpgradeSkillLocations(source)
	if err != nil || len(result.Succeeded()) != 1 {
		t.Fatalf("skill fallback = %#v, %v", result, err)
	}
	if _, err := UpgradeSkillLocations(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("failed skill fallback accepted")
	}
	if err := EnsureUpgradeDirectories(); err != nil {
		t.Fatal(err)
	}
	if got := DownloadCacheDir(); !strings.HasPrefix(got, home) {
		t.Fatalf("download cache dir = %q", got)
	}

	modeDir := filepath.Join(t.TempDir(), "mode")
	if err := os.Mkdir(modeDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(modeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(filepath.Join(blocker, "child"), 0o700); err == nil {
		t.Fatal("ensureDir stat error ignored")
	}
}

func TestCrossPlatformCoverageExecutablePathEdges(t *testing.T) {
	originalExecutable := upgradeExecutable
	originalEval := upgradeEvalSymlinks
	t.Cleanup(func() {
		upgradeExecutable = originalExecutable
		upgradeEvalSymlinks = originalEval
	})
	failure := errors.New("injected failure")
	upgradeExecutable = func() (string, error) { return "", failure }
	if _, err := CurrentBinaryPath(); !errors.Is(err, failure) {
		t.Fatalf("executable error = %v", err)
	}
	upgradeExecutable = func() (string, error) { return "/tmp/exe", nil }
	upgradeEvalSymlinks = func(string) (string, error) { return "", failure }
	if _, err := CurrentBinaryPath(); !errors.Is(err, failure) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestCrossPlatformCoverageReplacerAndZipEdges(t *testing.T) {
	originalExecutable := upgradeExecutable
	originalEval := upgradeEvalSymlinks
	originalRename := upgradeRename
	originalChmod := upgradeChmod
	originalCopy := upgradeCopyFile
	originalSync := upgradeSyncParentDir
	t.Cleanup(func() {
		upgradeExecutable = originalExecutable
		upgradeEvalSymlinks = originalEval
		upgradeRename = originalRename
		upgradeChmod = originalChmod
		upgradeCopyFile = originalCopy
		upgradeSyncParentDir = originalSync
	})
	failure := errors.New("injected failure")
	current := filepath.Join(t.TempDir(), "dws")
	if err := os.WriteFile(current, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeExecutable = func() (string, error) { return current, nil }
	upgradeEvalSymlinks = func(path string) (string, error) { return path, nil }
	newBinary := filepath.Join(t.TempDir(), "new")
	if err := os.WriteFile(newBinary, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceSelf(newBinary); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(current); string(data) != "new" {
		t.Fatalf("replaced binary = %q", data)
	}
	upgradeExecutable = func() (string, error) { return "", failure }
	if err := ReplaceSelf("x"); err == nil {
		t.Fatal("executable failure ignored")
	}
	upgradeExecutable = func() (string, error) { return current, nil }
	upgradeEvalSymlinks = func(string) (string, error) { return "", failure }
	if err := ReplaceSelf("x"); err == nil {
		t.Fatal("symlink failure ignored")
	}
	upgradeEvalSymlinks = func(path string) (string, error) { return path, nil }
	upgradeChmod = func(string, os.FileMode) error { return failure }
	if err := ReplaceSelf("x"); err == nil {
		t.Fatal("chmod failure ignored")
	}
	upgradeChmod = originalChmod
	upgradeRename = func(string, string) error { return failure }
	upgradeCopyFile = func(string, string, os.FileMode) error { return failure }
	if err := replaceExeFile("src", "dst"); !errors.Is(err, failure) {
		t.Fatalf("copy fallback error = %v", err)
	}

	upgradeRename = originalRename
	upgradeCopyFile = originalCopy
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := windowsReplace(src, dst); err != nil {
		t.Fatalf("windows replacement success path: %v", err)
	}
	upgradeRename = func(string, string) error { return failure }
	if err := windowsReplace("src", "dst"); err == nil {
		t.Fatal("windows move failure ignored")
	}

	upgradeExecutable = func() (string, error) { return "", failure }
	CleanupStaleFiles()
	upgradeExecutable = func() (string, error) { return current, nil }
	CleanupStaleFiles()

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	var zipData bytes.Buffer
	zw := zip.NewWriter(&zipData)
	_, _ = zw.Create("dir/")
	f, _ := zw.Create("dir/file.txt")
	_, _ = f.Write([]byte("ok"))
	evil, _ := zw.Create("../evil")
	_, _ = evil.Write([]byte("bad"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zipPath, zipData.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := ExtractZip(zipPath, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "dir", "file.txt")); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(filepath.Join(t.TempDir(), "missing"), target); err == nil {
		t.Fatal("missing zip accepted")
	}
	if err := copyFile("missing", filepath.Join(t.TempDir(), "x"), 0o600); err == nil {
		t.Fatal("missing copy source accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(file, t.TempDir(), 0o600); err == nil {
		t.Fatal("directory copy destination accepted")
	}
	syncParentDir(filepath.Join(t.TempDir(), "file"))
	syncParentDir("/definitely/missing/file")
}

func TestCrossPlatformCoverageRollbackLifecycleEdges(t *testing.T) {
	originalExecutable := upgradeExecutable
	originalEval := upgradeEvalSymlinks
	originalHome := upgradeUserHomeDir
	originalReadDir := rollbackReadDir
	t.Cleanup(func() {
		upgradeExecutable = originalExecutable
		upgradeEvalSymlinks = originalEval
		upgradeUserHomeDir = originalHome
		rollbackReadDir = originalReadDir
	})
	failure := errors.New("injected failure")
	upgradeUserHomeDir = func() (string, error) { return "", failure }
	if got := NewRollbackManager(); !strings.HasPrefix(got.backupDir, ".dws") {
		t.Fatalf("fallback rollback dir = %q", got.backupDir)
	}

	current := filepath.Join(t.TempDir(), "dws")
	if err := os.WriteFile(current, []byte("current"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeExecutable = func() (string, error) { return current, nil }
	upgradeEvalSymlinks = func(path string) (string, error) { return path, nil }
	manager := NewRollbackManagerWithDir(t.TempDir())
	backupPath, err := manager.Backup("1.2.3")
	if err != nil || backupPath == "" {
		t.Fatalf("backup = %q, %v", backupPath, err)
	}
	backups, err := manager.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %#v, %v", backups, err)
	}
	if err := os.WriteFile(current, []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := manager.Rollback(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(current); string(data) != "current" {
		t.Fatalf("rolled-back binary = %q", data)
	}
	if err := manager.RollbackTo(BackupInfo{Path: t.TempDir()}); err == nil {
		t.Fatal("missing backup binary accepted")
	}
	empty := NewRollbackManagerWithDir(filepath.Join(t.TempDir(), "missing"))
	if err := empty.Rollback(); err == nil {
		t.Fatal("empty rollback accepted")
	}

	for i := 0; i < 3; i++ {
		name := "v0.0." + string(rune('1'+i)) + "-20260101-00000" + string(rune('1'+i))
		if err := os.MkdirAll(filepath.Join(manager.backupDir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Cleanup(1); err != nil {
		t.Fatal(err)
	}
	if err := manager.Cleanup(0); err != nil {
		t.Fatal(err)
	}
	rollbackReadDir = func(string) ([]os.DirEntry, error) { return nil, failure }
	if _, err := manager.ListBackups(); !errors.Is(err, failure) {
		t.Fatalf("backup read error = %v", err)
	}
	rollbackReadDir = originalReadDir
	syncFileData(current)
	syncFileData("/missing/file")
}

func TestCrossPlatformCoverageVersionPrereleaseEdges(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"", "alpha", 1}, {"alpha", "", -1},
		{"alpha", "alpha.1", -1}, {"alpha.1", "alpha", 1},
		{"1", "2", -1}, {"2", "1", 1}, {"1", "1", 0},
		{"1", "alpha", -1}, {"alpha", "1", 1},
		{"alpha", "beta", -1}, {"beta", "alpha", 1}, {"alpha", "alpha", 0},
	}
	for _, tc := range cases {
		if got := comparePrerelease(tc.a, tc.b); got != tc.want {
			t.Fatalf("comparePrerelease(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if _, ok := parseNumericIdentifier(""); ok {
		t.Fatal("empty numeric identifier accepted")
	}
}

func TestCrossPlatformCoverageDownloaderRetryControlEdges(t *testing.T) {
	originalAfter := downloadAfter
	t.Cleanup(func() { downloadAfter = originalAfter })

	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "x")
	}))
	defer truncated.Close()
	cfg := DefaultDownloadConfig()
	cfg.MaxRetries = 1
	if _, err := DownloadWithConfig(t.Context(), truncated.URL, filepath.Join(t.TempDir(), "out"), cfg); err == nil || !strings.Contains(err.Error(), "重试 1 次") {
		t.Fatalf("retry exhaustion = %v", err)
	}

	ready := make(chan time.Time)
	close(ready)
	downloadAfter = func(time.Duration) <-chan time.Time { return ready }
	cfg.MaxRetries = 7
	if _, err := DownloadWithConfig(t.Context(), truncated.URL, filepath.Join(t.TempDir(), "out"), cfg); err == nil {
		t.Fatal("retry exhaustion with capped backoff succeeded")
	}

	ctx, cancel := context.WithCancel(t.Context())
	downloadAfter = func(time.Duration) <-chan time.Time {
		cancel()
		return make(chan time.Time)
	}
	cfg.MaxRetries = 2
	if _, err := DownloadWithConfig(ctx, truncated.URL, filepath.Join(t.TempDir(), "out"), cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("backoff cancellation = %v", err)
	}
}

func TestCrossPlatformCoverageGitHubFetchReleasesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	if _, err := NewClientWithBaseURL(server.URL).FetchAllReleases(); err == nil {
		t.Fatal("release list HTTP error ignored")
	}
}

func TestCrossPlatformCoverageUpgradePathInjectedFailures(t *testing.T) {
	originalHome := upgradeUserHomeDir
	originalDirs := append([]string(nil), knownSkillDirs...)
	originalCopy := upgradeCopyDir
	originalEnsure := upgradeEnsureDir
	t.Cleanup(func() {
		upgradeUserHomeDir = originalHome
		knownSkillDirs = originalDirs
		upgradeCopyDir = originalCopy
		upgradeEnsureDir = originalEnsure
	})
	home := t.TempDir()
	upgradeUserHomeDir = func() (string, error) { return home, nil }
	knownSkillDirs = []string{".agents/skills"}
	calls := 0
	upgradeCopyDir = func(src, dst string) error {
		calls++
		if calls == 1 {
			return errors.New("primary failed")
		}
		return originalCopy(src, dst)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := UpgradeSkillLocations(source)
	if err != nil || len(result.Succeeded()) != 1 || len(result.Results) != 1 {
		t.Fatalf("replaced primary fallback = %#v, %v", result, err)
	}
	failure := errors.New("ensure failed")
	upgradeEnsureDir = func(string, os.FileMode) error { return failure }
	if err := EnsureUpgradeDirectories(); !errors.Is(err, failure) {
		t.Fatalf("ensure upgrade directory error = %v", err)
	}
}

func TestCrossPlatformCoverageReplacerInjectedIOEdges(t *testing.T) {
	originalExecutable := upgradeExecutable
	originalEval := upgradeEvalSymlinks
	originalRename := upgradeRename
	originalChmod := upgradeChmod
	originalCopy := upgradeCopyFile
	originalSyncParent := upgradeSyncParentDir
	originalOpenEntry := upgradeOpenZipEntry
	originalOpenFile := upgradeOpenFile
	originalCopyIO := upgradeIOCopy
	originalSync := upgradeFileSync
	t.Cleanup(func() {
		upgradeExecutable = originalExecutable
		upgradeEvalSymlinks = originalEval
		upgradeRename = originalRename
		upgradeChmod = originalChmod
		upgradeCopyFile = originalCopy
		upgradeSyncParentDir = originalSyncParent
		upgradeOpenZipEntry = originalOpenEntry
		upgradeOpenFile = originalOpenFile
		upgradeIOCopy = originalCopyIO
		upgradeFileSync = originalSync
	})
	failure := errors.New("injected failure")
	current := filepath.Join(t.TempDir(), "current")
	newBinary := filepath.Join(t.TempDir(), "new")
	for _, path := range []string{current, newBinary} {
		if err := os.WriteFile(path, []byte("x"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	upgradeExecutable = func() (string, error) { return current, nil }
	upgradeEvalSymlinks = func(path string) (string, error) { return path, nil }
	upgradeRename = func(string, string) error { return failure }
	upgradeCopyFile = func(string, string, os.FileMode) error { return failure }
	if err := ReplaceSelf(newBinary); !errors.Is(err, failure) {
		t.Fatalf("ReplaceSelf replacement error = %v", err)
	}
	if err := replaceExeFileFor("src", "dst", "linux"); !errors.Is(err, failure) {
		t.Fatalf("non-Windows copy fallback error = %v", err)
	}

	if err := replaceExeFileFor("src", "dst", "windows"); err == nil {
		t.Fatal("windows replace path unexpectedly succeeded")
	}
	renameCalls := 0
	upgradeRename = func(src, dst string) error {
		renameCalls++
		if renameCalls == 1 {
			return originalRename(src, dst)
		}
		return failure
	}
	upgradeCopyFile = originalCopy
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := windowsReplace(src, dst); err != nil {
		t.Fatalf("windows copy fallback: %v", err)
	}
	renameCalls = 0
	upgradeCopyFile = func(string, string, os.FileMode) error { return failure }
	src = filepath.Join(t.TempDir(), "src")
	dst = filepath.Join(t.TempDir(), "dst")
	_ = os.WriteFile(src, []byte("new"), 0o600)
	_ = os.WriteFile(dst, []byte("old"), 0o600)
	if err := windowsReplace(src, dst); err == nil {
		t.Fatal("windows copy fallback failure ignored")
	}

	var zipData bytes.Buffer
	zw := zip.NewWriter(&zipData)
	entryWriter, _ := zw.Create("file")
	_, _ = entryWriter.Write([]byte("x"))
	_ = zw.Close()
	zipPath := filepath.Join(t.TempDir(), "one.zip")
	_ = os.WriteFile(zipPath, zipData.Bytes(), 0o600)
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entry := reader.File[0]
	upgradeOpenZipEntry = func(*zip.File) (io.ReadCloser, error) { return nil, failure }
	if err := extractZipEntry(entry, filepath.Join(t.TempDir(), "out")); !errors.Is(err, failure) {
		t.Fatalf("zip entry open error = %v", err)
	}
	upgradeOpenZipEntry = originalOpenEntry
	upgradeOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, failure }
	if err := extractZipEntry(entry, filepath.Join(t.TempDir(), "out")); !errors.Is(err, failure) {
		t.Fatalf("zip destination open error = %v", err)
	}
	upgradeOpenFile = originalOpenFile
	upgradeIOCopy = func(io.Writer, io.Reader) (int64, error) { return 0, failure }
	if err := extractZipEntry(entry, filepath.Join(t.TempDir(), "out")); !errors.Is(err, failure) {
		t.Fatalf("zip copy error = %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	if err := copyFile(file, filepath.Join(t.TempDir(), "out"), 0o600); !errors.Is(err, failure) {
		t.Fatalf("file copy error = %v", err)
	}
	upgradeIOCopy = originalCopyIO
	upgradeFileSync = func(*os.File) error { return failure }
	if err := copyFile(file, filepath.Join(t.TempDir(), "out"), 0o600); !errors.Is(err, failure) {
		t.Fatalf("file sync error = %v", err)
	}

	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, "file"), 0o700); err != nil {
		t.Fatal(err)
	}
	upgradeFileSync = originalSync
	if err := ExtractZip(zipPath, target); err == nil {
		t.Fatal("zip extraction destination error ignored")
	}
}

func TestCrossPlatformCoverageRollbackInjectedFailures(t *testing.T) {
	originalExecutable := upgradeExecutable
	originalEval := upgradeEvalSymlinks
	originalStat := rollbackStat
	originalMkdir := rollbackMkdirAll
	originalCopy := rollbackCopyFile
	originalReadDir := rollbackReadDir
	originalRemove := rollbackRemoveAll
	originalMarshal := rollbackMarshalIndent
	originalWrite := rollbackWriteFile
	originalRead := rollbackReadFile
	originalReplace := rollbackReplaceFile
	t.Cleanup(func() {
		upgradeExecutable = originalExecutable
		upgradeEvalSymlinks = originalEval
		rollbackStat = originalStat
		rollbackMkdirAll = originalMkdir
		rollbackCopyFile = originalCopy
		rollbackReadDir = originalReadDir
		rollbackRemoveAll = originalRemove
		rollbackMarshalIndent = originalMarshal
		rollbackWriteFile = originalWrite
		rollbackReadFile = originalRead
		rollbackReplaceFile = originalReplace
	})
	failure := errors.New("injected failure")
	manager := NewRollbackManagerWithDir(t.TempDir())
	upgradeExecutable = func() (string, error) { return "", failure }
	if _, err := manager.Backup("1"); err == nil {
		t.Fatal("backup executable error ignored")
	}
	if err := manager.RollbackTo(BackupInfo{}); err == nil {
		t.Fatal("rollback executable error ignored")
	}
	upgradeExecutable = func() (string, error) { return "/current", nil }
	upgradeEvalSymlinks = func(string) (string, error) { return "", failure }
	if _, err := manager.Backup("1"); err == nil {
		t.Fatal("backup symlink error ignored")
	}
	if err := manager.RollbackTo(BackupInfo{}); err == nil {
		t.Fatal("rollback symlink error ignored")
	}
	upgradeEvalSymlinks = func(path string) (string, error) { return path, nil }
	rollbackStat = func(string) (os.FileInfo, error) { return nil, failure }
	if _, err := manager.Backup("1"); err == nil {
		t.Fatal("backup stat error ignored")
	}

	current := filepath.Join(t.TempDir(), "current")
	backup := filepath.Join(t.TempDir(), "backup")
	_ = os.WriteFile(current, []byte("old"), 0o700)
	_ = os.WriteFile(backup, []byte("new"), 0o700)
	upgradeExecutable = func() (string, error) { return current, nil }
	rollbackStat = originalStat
	for failAt := 1; failAt <= 3; failAt++ {
		calls := 0
		rollbackMkdirAll = func(path string, mode os.FileMode) error {
			calls++
			if calls == failAt {
				return failure
			}
			return originalMkdir(path, mode)
		}
		if _, err := NewRollbackManagerWithDir(filepath.Join(t.TempDir(), "backups")).Backup("1"); err == nil {
			t.Fatalf("backup mkdir stage %d error ignored", failAt)
		}
	}
	rollbackMkdirAll = originalMkdir
	rollbackCopyFile = func(string, string, os.FileMode) error { return failure }
	if _, err := manager.Backup("1"); err == nil {
		t.Fatal("backup copy error ignored")
	}
	if err := manager.RollbackTo(BackupInfo{BinaryPath: backup}); err == nil {
		t.Fatal("rollback preparation copy error ignored")
	}
	rollbackCopyFile = originalCopy
	rollbackReplaceFile = func(string, string) error { return failure }
	if err := manager.RollbackTo(BackupInfo{BinaryPath: backup}); err == nil {
		t.Fatal("rollback replacement error ignored")
	}

	rollbackReadDir = func(string) ([]os.DirEntry, error) { return nil, failure }
	if err := manager.Rollback(); err == nil {
		t.Fatal("rollback list error ignored")
	}
	if err := manager.Cleanup(1); err == nil {
		t.Fatal("cleanup list error ignored")
	}
	rollbackReadDir = originalReadDir
	rollbackRemoveAll = func(string) error { return failure }
	_ = rollbackRemoveAll
	rollbackMarshalIndent = func(any, string, string) ([]byte, error) { return nil, failure }
	manager.saveBackupInfo(BackupInfo{Path: t.TempDir()})
	rollbackMarshalIndent = originalMarshal
	rollbackWriteFile = func(string, []byte, os.FileMode) error { return failure }
	manager.saveBackupInfo(BackupInfo{Path: t.TempDir()})
	rollbackWriteFile = originalWrite
	rollbackReadFile = func(string) ([]byte, error) { return nil, failure }
	if _, err := manager.loadBackupInfo(t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("backup info read error = %v", err)
	}
}

func TestCrossPlatformCoverageCopyDirAndVerifyErrors(t *testing.T) {
	if err := copyDir("missing", t.TempDir()); err == nil {
		t.Fatal("missing source directory accepted")
	}
	sourceFile := filepath.Join(t.TempDir(), "source")
	_ = os.WriteFile(sourceFile, []byte("x"), 0o600)
	if err := copyDir(sourceFile, t.TempDir()); err == nil {
		t.Fatal("file source directory accepted")
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	if err := copyDir(t.TempDir(), filepath.Join(blocker, "child")); err == nil {
		t.Fatal("copy directory mkdir error ignored")
	}
	source := t.TempDir()
	if err := os.Symlink(filepath.Join(source, "missing"), filepath.Join(source, "dangling")); err == nil {
		if err := copyDir(source, t.TempDir()); err == nil {
			t.Fatal("dangling source entry accepted")
		}
	}
	if err := VerifySHA256("missing", "deadbeef"); err == nil {
		t.Fatal("missing checksum file accepted")
	}
	if _, err := ComputeSHA256("missing"); err == nil {
		t.Fatal("missing SHA file accepted")
	}
}

func TestCrossPlatformCoverageCopyDirAndBackupEntryInjectedEdges(t *testing.T) {
	originalRollbackInfo := rollbackEntryInfo
	originalStat := upgradeDirStat
	originalMkdir := upgradeDirMkdirAll
	originalReadDir := upgradeDirReadDir
	originalInfo := upgradeDirEntryInfo
	originalOpen := upgradeDirOpen
	originalOpenFile := upgradeDirOpenFile
	originalCopy := upgradeDirCopy
	t.Cleanup(func() {
		rollbackEntryInfo = originalRollbackInfo
		upgradeDirStat = originalStat
		upgradeDirMkdirAll = originalMkdir
		upgradeDirReadDir = originalReadDir
		upgradeDirEntryInfo = originalInfo
		upgradeDirOpen = originalOpen
		upgradeDirOpenFile = originalOpenFile
		upgradeDirCopy = originalCopy
	})
	failure := errors.New("injected failure")

	backupDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(backupDir, "v1-20260101"), 0o700); err != nil {
		t.Fatal(err)
	}
	rollbackEntryInfo = func(os.DirEntry) (os.FileInfo, error) { return nil, failure }
	backups, err := NewRollbackManagerWithDir(backupDir).ListBackups()
	if err != nil || len(backups) != 0 {
		t.Fatalf("failed backup entry info = %#v, %v", backups, err)
	}

	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	upgradeDirReadDir = func(path string) ([]os.DirEntry, error) {
		if strings.HasSuffix(path, "sub") {
			return nil, failure
		}
		return originalReadDir(path)
	}
	if err := copyDir(source, t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("recursive copy error = %v", err)
	}
	upgradeDirReadDir = originalReadDir
	upgradeDirEntryInfo = func(os.DirEntry) (os.FileInfo, error) { return nil, failure }
	if err := copyDir(source, t.TempDir()); err != nil {
		t.Fatalf("entry info errors should be skipped: %v", err)
	}
	upgradeDirEntryInfo = originalInfo
	upgradeDirOpen = func(string) (*os.File, error) { return nil, failure }
	if err := copyDir(source, t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("source open error = %v", err)
	}
	upgradeDirOpen = originalOpen
	upgradeDirOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, failure }
	if err := copyDir(source, t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("destination open error = %v", err)
	}
	upgradeDirOpenFile = originalOpenFile
	upgradeDirCopy = func(io.Writer, io.Reader) (int64, error) { return 0, failure }
	if err := copyDir(source, t.TempDir()); !errors.Is(err, failure) {
		t.Fatalf("directory file copy error = %v", err)
	}
}
