// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func npmSplitDownloadFixture(t *testing.T) (GitHubAsset, []byte) {
	t.Helper()
	const body = "release resource"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	entries := []npmPackageTestEntry{
		{name: "package/package.json", data: fmt.Sprintf(`{"name":%q,"version":"1.2.3"}`, npmPackageName+"-skills")},
		{name: "package/assets/" + skillsZipName, data: body},
		{name: "package/assets/checksums.txt", data: digest + "  " + skillsZipName + "\n"},
	}
	tarball := npmPackageTestGzip(t, npmPackageTestTar(t, entries))
	asset := GitHubAsset{Name: skillsZipName, Size: int64(len(body)), Digest: "sha256:" + digest,
		BrowserDownloadURL: "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v1.2.3/" + skillsZipName,
		NPM: &NPMPackage{Name: npmPackageName + "-skills", Version: "1.2.3", RegistryURL: publicNPMMirror,
			TarballURL: publicNPMMirror + "/package.tgz", Integrity: npmPackageTestSRI(tarball)}}
	return asset, tarball
}

func TestCrossPlatformCoverageNPMSplitAssetFallback(t *testing.T) {
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	for _, mode := range []string{"success", "main-failed", "split-failed", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			asset, tarball := npmSplitDownloadFixture(t)
			manifest := npmSplitManifest{}
			manifest.Name, manifest.Version = asset.NPM.Name, asset.NPM.Version
			manifest.Dist.Tarball, manifest.Dist.Integrity = asset.NPM.TarballURL, asset.NPM.Integrity
			manifest.Upgrade.Format, manifest.Upgrade.Edition, manifest.Upgrade.Asset = 1, "public", asset.Name
			manifest.Upgrade.SHA256, manifest.Upgrade.Size = ExtractDigestSHA256(asset.Digest), asset.Size
			if mode == "mismatch" {
				manifest.Upgrade.SHA256 = strings.Repeat("0", 64)
			}
			asset.NPM = nil
			githubRequests, tarballRequests := 0, 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status, body := 200, []byte("{}")
				if req.Header.Get("Authorization") != "" || req.Header.Get("Range") != "" {
					t.Fatal("fallback leaked credentials or a GitHub byte offset")
				}
				switch {
				case req.URL.Host == "github.com":
					githubRequests++
					status = 503
				case req.URL.String() == publicNPMMirror+"/"+npmPackageName+"/1.2.3":
					body, _ = json.Marshal(registryFixture("1.2.3"))
					if mode == "main-failed" {
						status = 503
					}
				case req.URL.String() == publicNPMMirror+"/"+npmPackageName+"-skills/1.2.3":
					body, _ = json.Marshal(manifest)
					if mode == "split-failed" {
						status = 503
					}
				case req.URL.String() == publicNPMMirror+"/package.tgz":
					tarballRequests++
					body = tarball
				default:
					t.Fatalf("unexpected fallback request: %s", req.URL)
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body))}, nil
			})))
			var events []DownloadEvent
			ctx := WithDownloadObserver(t.Context(), func(e DownloadEvent) {
				if e.Kind == "switch" {
					events = append(events, e)
				}
			})
			dest := filepath.Join(t.TempDir(), "asset")
			if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			progressCalls := 0
			err := DownloadPlatformAsset(ctx, asset, dest, func(float64, int64, int64) { progressCalls++ })
			if githubRequests != 3 || len(events) != 1 || events[0].Reason != "HTTP 503" || events[0].Source != "npmmirror" {
				t.Fatalf("fallback attempts/events = %d %+v", githubRequests, events)
			}
			data, readErr := os.ReadFile(dest)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if mode == "success" {
				if err != nil || string(data) != "release resource" || tarballRequests != 1 || progressCalls == 0 {
					t.Fatalf("fallback result = %q %v, downloads=%d progress=%d", data, err, tarballRequests, progressCalls)
				}
			} else if err == nil || string(data) != "original" || tarballRequests != 0 {
				t.Fatalf("invalid mirror replaced destination: %q %v, downloads=%d", data, err, tarballRequests)
			}
		})
	}
}

func TestCrossPlatformCoverageNPMSplitRejectsUnrelatedGitHubAsset(t *testing.T) {
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unrelated GitHub asset contacted public mirror")
		return nil, nil
	})))
	prefix := "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v"
	for _, address := range []string{"https://github.com/other/repository/asset", prefix + "1.2.3", prefix + "1.2.3/another.zip"} {
		if _, err := mirrorForGitHubAsset(t.Context(), GitHubAsset{Name: skillsZipName, BrowserDownloadURL: address}); err == nil {
			t.Fatalf("unrelated URL accepted: %s", address)
		}
	}
}

func TestCrossPlatformCoverageNPMSplitDownloadFailureBoundaries(t *testing.T) {
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	for _, mode := range []string{"integrity", "parent", "stage", "download", "open", "extract", "stat", "size", "cancel", "unknown-total"} {
		t.Run(mode, func(t *testing.T) {
			asset, tarball := npmSplitDownloadFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("split filesystem failure")
			dir := t.TempDir()
			dest := filepath.Join(dir, "asset")
			if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			original := dest
			switch mode {
			case "integrity":
				asset.NPM.Integrity = "invalid"
			case "parent":
				dest = filepath.Join(dest, "child")
			case "stage":
				testseam.Swap(t, &npmSplitMkdirTemp, func(string, string) (string, error) { return "", failure })
			case "open":
				testseam.Swap(t, &npmPackageOpen, func(string) (*os.File, error) { return nil, failure })
			case "extract":
				tarball = []byte("not a gzip archive")
				asset.NPM.Integrity = npmPackageTestSRI(tarball)
			case "stat":
				testseam.Swap(t, &npmSplitStat, func(string) (os.FileInfo, error) { return nil, failure })
			case "size":
				asset.Size++
			case "cancel":
				testseam.Swap(t, &npmSplitStat, func(name string) (os.FileInfo, error) {
					info, err := os.Stat(name)
					cancel()
					return info, err
				})
			}
			requests := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				status, length := 200, int64(len(tarball))
				if mode == "download" {
					status = 503
				}
				if mode == "unknown-total" {
					length = -1
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: length, Body: io.NopCloser(bytes.NewReader(tarball))}, nil
			})))
			progressCalls := 0
			err := downloadNPMSplitAsset(ctx, asset, dest, func(percent float64, n, total int64) {
				progressCalls++
				if mode == "unknown-total" && (percent != 0 || total != -1 || n <= 0) {
					t.Fatalf("unknown-size progress = %f %d/%d", percent, n, total)
				}
			})
			data, readErr := os.ReadFile(original)
			entries, listErr := os.ReadDir(dir)
			if readErr != nil || listErr != nil || len(entries) != 1 {
				t.Fatalf("split cleanup failed: %v %v %v", entries, readErr, listErr)
			}
			if mode == "unknown-total" {
				if err != nil || string(data) != "release resource" || progressCalls == 0 {
					t.Fatalf("unknown-size split download = %q %v progress=%d", data, err, progressCalls)
				}
			} else if err == nil || string(data) != "original" {
				t.Fatalf("failed split changed destination: %q %v", data, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled split error = %v", err)
			}
			if (mode == "stage" || mode == "open" || mode == "stat") && !errors.Is(err, failure) {
				t.Fatalf("filesystem cause lost: %v", err)
			}
			if mode == "download" && requests != 3 {
				t.Fatalf("download attempts = %d, want 3", requests)
			}
		})
	}
}
