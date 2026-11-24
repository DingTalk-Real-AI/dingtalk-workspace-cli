// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"encoding/json"
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
