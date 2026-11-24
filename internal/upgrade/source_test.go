// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageExplicitSourceOverridesEnvironment(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	t.Setenv("DWS_UPGRADE_REGISTRY", "https://custom.example/npm")
	t.Setenv("DWS_UPGRADE_URL", "https://custom.example/github")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "private/repo")
	for _, source := range []DownloadSource{SourceAuto, SourceGitHub, SourceNPMMirror} {
		client := NewClientForSource(source)
		if err := client.validateConfig(); err != nil {
			t.Fatal(err)
		}
		switch source {
		case SourceAuto:
			if client.baseURL != "https://custom.example/github" || client.owner != "private" || client.repo != "repo" || client.registryURL != "" {
				t.Fatal("auto lost the existing source environment configuration")
			}
		case SourceGitHub:
			if client.registryURL != defaultNPMRegistry+"/"+npmPackageName {
				t.Fatal("github bypassed the official main-package authority")
			}
		case SourceNPMMirror:
			if client.registryURL != publicNPMMirror+"/"+npmPackageName {
				t.Fatal("npmmirror did not override source environment variables")
			}
		}
	}
	if os.Getenv("DWS_UPGRADE_REGISTRY") != "https://custom.example/npm" {
		t.Fatal("source selection mutated the process environment")
	}
	testseam.Swap(t, &upgradeEditionName, func() string { return "private" })
	for _, source := range []DownloadSource{SourceGitHub, SourceNPMMirror} {
		if NewClientForSource(source).validateConfig() == nil {
			t.Fatal("explicit public source replaced a private edition")
		}
	}
}

func TestCrossPlatformCoveragePinnedSourceDoesNotFallback(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	SwapPlatformGitHubTokenForTest(t, func(context.Context) string { return "" })
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	for _, phase := range []string{"metadata", "asset"} {
		t.Run(phase, func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host != "github.com" && req.URL.Host != "api.github.com" {
					t.Fatalf("pinned github contacted %s", req.URL.Host)
				}
				return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
			})))
			ctx := WithDownloadObserver(t.Context(), func(e DownloadEvent) {
				if e.Kind == "switch" {
					t.Fatal("pinned source emitted a fallback event")
				}
			})
			var err error
			if phase == "metadata" {
				release := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
				_, err = ResolvePlatformReleaseForSource(ctx, release, true, SourceGitHub)
			} else {
				asset := GitHubAsset{Name: skillsZipName, Size: 10, Digest: "sha256:" + strings.Repeat("a", 64),
					BrowserDownloadURL: "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v1.2.3/" + skillsZipName}
				err = DownloadPlatformAssetForSource(ctx, asset, filepath.Join(t.TempDir(), "asset"), nil, SourceGitHub)
			}
			wantCalls := 1
			if phase == "asset" {
				wantCalls = 3
			}
			if err == nil || calls != wantCalls {
				t.Fatalf("err=%v requests=%d, want %d failed same-source requests", err, calls, wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoveragePinnedMirrorRequiresSplits(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "registry.npmmirror.com" {
			t.Fatalf("pinned mirror contacted %s", req.URL.Host)
		}
		status, body := 404, []byte("{}")
		if req.URL.Path == "/"+npmPackageName+"/1.2.3" {
			status = 200
			body, _ = json.Marshal(registryFixture("1.2.3"))
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})))
	release := npmManifestToRelease(registryFixture("1.2.3"), publicNPMMirror)
	if resolved, err := ResolvePlatformReleaseForSource(t.Context(), release, true, SourceNPMMirror); err == nil || resolved != nil {
		t.Fatalf("pinned mirror silently selected a whole package: %+v %v", resolved, err)
	}
	if resolved, err := ResolvePlatformRelease(t.Context(), release, true); err != nil || resolved != nil {
		t.Fatalf("auto lost historical mirror compatibility: %+v %v", resolved, err)
	}
}

func TestCrossPlatformCoverageSourceValidationBoundaries(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid source or identity made a network request")
		return nil, nil
	})))
	if DownloadSource("").normalized() != SourceAuto || NewClientForSource("invalid").validateConfig() == nil {
		t.Fatal("source default or validation was lost")
	}
	for _, tc := range []struct {
		source  DownloadSource
		release *ReleaseInfo
	}{
		{"invalid", nil}, {SourceGitHub, nil}, {SourceNPMMirror, &ReleaseInfo{}},
		{SourceGitHub, npmManifestToRelease(registryFixture("1.2.3"), publicNPMMirror)},
		{SourceNPMMirror, npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)},
	} {
		if _, err := ResolvePlatformReleaseForSource(t.Context(), tc.release, true, tc.source); err == nil {
			t.Fatalf("source/release mismatch accepted: %s %+v", tc.source, tc.release)
		}
	}
	asset := GitHubAsset{Name: skillsZipName, Size: 4, Digest: "sha256:" + strings.Repeat("a", 64), BrowserDownloadURL: "https://github.com/asset"}
	for _, source := range []DownloadSource{"invalid", SourceNPMMirror} {
		if err := DownloadPlatformAssetForSource(t.Context(), asset, filepath.Join(t.TempDir(), "asset"), nil, source); err == nil {
			t.Fatalf("invalid source %s accepted", source)
		}
	}
	asset.NPM = &NPMPackage{}
	if err := DownloadPlatformAssetForSource(t.Context(), asset, filepath.Join(t.TempDir(), "asset"), nil, SourceGitHub); err == nil {
		t.Fatal("github accepted an npm asset")
	}
	if err := validatePlatformAsset(asset); err == nil {
		t.Fatal("invalid npm package identity accepted")
	}
	if _, _, err := mirrorSplitClient(t.Context(), "1.2.3-private.1"); err == nil {
		t.Fatal("private release selected public split packages")
	}
	if _, err := resolveMirrorSplitRelease(t.Context(), &ReleaseInfo{Version: "invalid"}, true); err == nil {
		t.Fatal("invalid mirror version accepted")
	}
	if _, err := resolveNPMSplitAsset(t.Context(), nil, "1.2.3", "../asset"); err == nil {
		t.Fatal("invalid split filename accepted")
	}
}

func TestCrossPlatformCoverageSourceEventLabels(t *testing.T) {
	for raw, want := range map[string]string{
		"https://registry.npmjs.org/package":             "npmjs",
		"https://npmmirror.com/package":                  "npmmirror",
		"https://registry.npmmirror.com/package":         "npmmirror",
		"https://cdn.example.test/package?secret=hidden": "cdn.example.test",
		"https://%": "已配置来源", "relative": "已配置来源",
	} {
		if got := downloadSourceName(raw); got != want {
			t.Fatalf("source label for %q = %q, want %q", raw, got, want)
		}
	}
	if got := downloadFailureReason(context.DeadlineExceeded); got != "下载超时或停滞" {
		t.Fatalf("timeout reason = %q", got)
	}
	if got := downloadFailureReason(errors.New("https://user:secret@example.test")); strings.Contains(got, "secret") {
		t.Fatal("raw transport error leaked into progress")
	}
}

func TestCrossPlatformCoverageGitHubRateLimitFallbackClassification(t *testing.T) {
	for _, remaining := range []string{"0", "1", ""} {
		t.Run("remaining="+remaining, func(t *testing.T) {
			client := &Client{httpClient: &http.Client{Transport: upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"X-Ratelimit-Remaining": []string{remaining}}, Body: io.NopCloser(strings.NewReader("forbidden"))}, nil
			})}}
			var payload any
			err := client.getBoundedJSON(t.Context(), "https://api.github.com/release", &payload, 1024, "GitHub Release", "release")
			var status *downloadHTTPError
			if !errors.As(err, &status) || canFallbackPlatform(err) != (remaining == "0") {
				t.Fatalf("GitHub 403 classification = %v", err)
			}
		})
	}
}
