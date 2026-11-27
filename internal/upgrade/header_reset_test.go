// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageHTTP2ResetBeforeHeaders(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	name := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
	body := "official platform resource"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	suffix, err := npmSplitSuffix(name)
	if err != nil {
		t.Fatal(err)
	}
	tarball := npmPackageTestGzip(t, npmPackageTestTar(t, []npmPackageTestEntry{
		{name: "package/package.json", data: fmt.Sprintf(`{"name":%q,"version":"1.2.3"}`, npmPackageName+"-"+suffix)},
		{name: "package/assets/" + name, data: body},
		{name: "package/assets/checksums.txt", data: digest + "  " + name + "\n"},
	}))
	main := registryFixture("1.2.3")
	main.Upgrade = &npmUpgradeManifest{Format: 1, Edition: "public", Assets: map[string]npmUpgradeDigest{name: {SHA256: digest, Size: int64(len(body))}}}
	mainJSON, _ := json.Marshal(main)
	split := npmSplitManifest{}
	split.Name, split.Version = npmPackageName+"-"+suffix, "1.2.3"
	split.Dist.Tarball, split.Dist.Integrity = publicNPMMirror+"/package.tgz", npmPackageTestSRI(tarball)
	split.Upgrade.Format, split.Upgrade.Edition, split.Upgrade.Asset = 1, "public", name
	split.Upgrade.SHA256, split.Upgrade.Size = digest, int64(len(body))
	splitJSON, _ := json.Marshal(split)
	for _, phase := range []string{"metadata", "archive"} {
		for _, source := range []DownloadSource{SourceAuto, SourceGitHub} {
			t.Run(phase+"/"+string(source), func(t *testing.T) {
				var githubCalls atomic.Int32
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.ProtoMajor != 2 {
						t.Error("fixture requires HTTP/2")
					}
					githubCalls.Add(1)
					// No Write/WriteHeader/Flush: Client.Do itself must fail.
					panic(http.ErrAbortHandler)
				}))
				server.EnableHTTP2 = true
				server.StartTLS()
				defer server.Close()
				endpoint, _ := url.Parse(server.URL)
				mirrorCalls, switches := 0, 0
				testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == "api.github.com" || req.URL.Host == "github.com" {
						clone := req.Clone(req.Context())
						clone.URL.Scheme, clone.URL.Host = endpoint.Scheme, endpoint.Host
						return server.Client().Transport.RoundTrip(clone)
					}
					if req.URL.Host != "registry.npmmirror.com" {
						t.Fatalf("unexpected source: %s", req.URL)
					}
					mirrorCalls++
					payload := splitJSON
					if req.URL.Path == "/"+npmPackageName+"/1.2.3" {
						payload = mainJSON
					} else if req.URL.Path == "/package.tgz" {
						payload = tarball
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: int64(len(payload)), Body: io.NopCloser(strings.NewReader(string(payload)))}, nil
				})))
				ctx := WithDownloadObserver(t.Context(), func(event DownloadEvent) {
					if event.Kind == "switch" && event.Previous == "github" && event.Source == "npmmirror" && event.Reason != "" {
						switches++
					}
				})
				var resultErr error
				wantGithub, wantMirror := int32(1), 2
				if phase == "metadata" {
					resolved, err := ResolvePlatformReleaseForSource(ctx, npmManifestToRelease(main, defaultNPMRegistry), true, source)
					resultErr = err
					if source == SourceAuto && (resolved == nil || len(resolved.Assets) != 1 || resolved.Assets[0].NPM == nil) {
						t.Fatalf("auto did not select the mirror: %+v %v", resolved, err)
					}
				} else {
					wantGithub, wantMirror = 3, 3
					asset := GitHubAsset{Name: name, Size: int64(len(body)), Digest: "sha256:" + digest,
						BrowserDownloadURL: "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v1.2.3/" + name}
					dest := filepath.Join(t.TempDir(), "asset")
					resultErr = DownloadPlatformAssetForSource(ctx, asset, dest, nil, source)
					if source == SourceAuto {
						got, err := os.ReadFile(dest)
						if err != nil || string(got) != body {
							t.Fatalf("mirror bytes: %q %v", got, err)
						}
					} else if _, err := os.Stat(dest); !os.IsNotExist(err) {
						t.Fatalf("failed pinned download published a file: %v", err)
					}
				}
				wantSwitch := 1
				if source == SourceGitHub {
					wantMirror, wantSwitch = 0, 0
				}
				if (resultErr == nil) != (source == SourceAuto) || githubCalls.Load() != wantGithub || mirrorCalls != wantMirror || switches != wantSwitch {
					t.Fatalf("err=%v github=%d/%d mirror=%d/%d switches=%d/%d", resultErr, githubCalls.Load(), wantGithub, mirrorCalls, wantMirror, switches, wantSwitch)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageHTTP2ResetClassification(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"stream error: stream ID 1; INTERNAL_ERROR; received from peer", true},
		{"stream error: stream ID 3; REFUSED_STREAM; received from peer", true},
		{"stream error: stream ID 5; CANCEL; received from peer", true},
		{"stream error: stream ID 1; PROTOCOL_ERROR; received from peer", false},
		{"stream error: stream ID 0; INTERNAL_ERROR; received from peer", false},
		{"stream error: stream ID invalid; INTERNAL_ERROR; received from peer", false},
		{"stream error: stream ID 1; INTERNAL_ERROR; locally generated", false},
		{"stream error: stream ID 1; INTERNAL_ERROR", false},
		{"certificate signed by unknown authority", false},
		{"拒绝不安全的下载重定向", false},
	} {
		err := &url.Error{Op: "Get", URL: "https://github.com/asset", Err: errors.New(tc.message)}
		if got := isRetriable(err); got != tc.want {
			t.Errorf("%s: retriable=%t want=%t", tc.message, got, tc.want)
		}
		if isRetriable(errors.New(tc.message)) {
			t.Errorf("error outside Client.Do was treated as HTTP/2 reset: %s", tc.message)
		}
	}
}
