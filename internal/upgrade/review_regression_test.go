// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
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

func TestCrossPlatformCoverageNPMSplitMainAuthority(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	name := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
	for _, mode := range []string{"valid", "absent", "missing", "format", "edition", "missing-asset", "wrong-size", "wrong-sha"} {
		t.Run(mode, func(t *testing.T) {
			main := registryFixture("1.2.3")
			main.Upgrade = &npmUpgradeManifest{Format: 1, Edition: "public", Assets: map[string]npmUpgradeDigest{name: {SHA256: strings.Repeat("a", 64), Size: 10}}}
			switch mode {
			case "absent":
				main.Upgrade = nil
			case "missing":
				main.Upgrade = &npmUpgradeManifest{}
			case "format":
				main.Upgrade.Format = 2
			case "edition":
				main.Upgrade.Edition = "private"
			case "missing-asset":
				delete(main.Upgrade.Assets, name)
			case "wrong-size":
				main.Upgrade.Assets[name] = npmUpgradeDigest{SHA256: strings.Repeat("a", 64), Size: 11}
			case "wrong-sha":
				main.Upgrade.Assets[name] = npmUpgradeDigest{SHA256: strings.Repeat("b", 64), Size: 10}
			}
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status, body := 200, []byte("{}")
				if req.URL.Host == "api.github.com" {
					status = 503
				} else if req.URL.Path == "/"+npmPackageName+"/1.2.3" {
					// The mirror's main metadata must not replace the npmjs anchor.
					mirrored := registryFixture("1.2.3")
					mirrored.Upgrade = &npmUpgradeManifest{Format: 1, Edition: "public", Assets: map[string]npmUpgradeDigest{name: {SHA256: strings.Repeat("a", 64), Size: 10}}}
					body, _ = json.Marshal(mirrored)
				} else {
					suffix, _ := npmSplitSuffix(name)
					m := npmSplitManifest{}
					m.Name, m.Version = npmPackageName+"-"+suffix, "1.2.3"
					m.Dist.Tarball, m.Dist.Integrity = publicNPMMirror+"/package.tgz", npmPackageTestSRI([]byte("self-consistent but unrelated"))
					m.Upgrade.Format, m.Upgrade.Edition, m.Upgrade.Asset = 1, "public", name
					m.Upgrade.SHA256, m.Upgrade.Size = strings.Repeat("a", 64), 10
					body, _ = json.Marshal(m)
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})))
			for _, source := range []DownloadSource{SourceAuto, SourceNPMMirror} {
				registry := defaultNPMRegistry
				if source == SourceNPMMirror {
					registry = publicNPMMirror
				}
				resolved, err := ResolvePlatformReleaseForSource(t.Context(), npmManifestToRelease(main, registry), true, source)
				if mode == "absent" && source == SourceNPMMirror {
					if err != nil || resolved != nil {
						t.Fatalf("legacy mirror must retain whole-package path: %+v %v", resolved, err)
					}
				} else if mode == "valid" {
					if err != nil || resolved == nil || len(resolved.Assets) != 1 {
						t.Fatalf("valid authority: %v %+v", err, resolved)
					}
				} else if err == nil || resolved != nil || !strings.Contains(err.Error(), "官方主包") {
					t.Fatalf("untrusted split authorized (%s %s): %+v %v", source, mode, resolved, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageGitHubMetadataStreamReset(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Error("metadata fixture must use HTTP/2")
		}
		calls.Add(1)
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3",`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := &Client{httpClient: server.Client()}
	var metadata map[string]any
	err := client.getBoundedJSON(t.Context(), server.URL, &metadata, registryManifestLimit, "GitHub Release", "metadata")
	if err == nil || !canFallbackPlatform(err) {
		t.Fatalf("metadata stream reset must permit fallback: %v", err)
	}
	endpoint, _ := url.Parse(server.URL)
	name := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
	main := registryFixture("1.2.3")
	main.Upgrade = &npmUpgradeManifest{Format: 1, Edition: "public", Assets: map[string]npmUpgradeDigest{name: {SHA256: strings.Repeat("a", 64), Size: 10}}}
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			clone := req.Clone(req.Context())
			clone.URL.Scheme, clone.URL.Host = endpoint.Scheme, endpoint.Host
			return server.Client().Transport.RoundTrip(clone)
		}
		var body []byte
		if req.URL.Path == "/"+npmPackageName+"/1.2.3" {
			body, _ = json.Marshal(main)
		} else {
			suffix, _ := npmSplitSuffix(name)
			m := npmSplitManifest{}
			m.Name, m.Version = npmPackageName+"-"+suffix, "1.2.3"
			m.Dist.Tarball, m.Dist.Integrity = publicNPMMirror+"/package.tgz", npmPackageTestSRI([]byte("fixture"))
			m.Upgrade.Format, m.Upgrade.Edition, m.Upgrade.Asset = 1, "public", name
			m.Upgrade.SHA256, m.Upgrade.Size = strings.Repeat("a", 64), 10
			body, _ = json.Marshal(m)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})))
	got, err := ResolvePlatformReleaseForSource(t.Context(), npmManifestToRelease(main, defaultNPMRegistry), true, SourceAuto)
	if err != nil || got == nil || len(got.Assets) != 1 || got.Assets[0].NPM == nil || calls.Load() != 2 {
		t.Fatalf("auto did not switch after metadata reset: %+v err=%v calls=%d", got, err, calls.Load())
	}
}

func TestCrossPlatformCoverageNPMSplitLegacyMainKeepsWholePackage(t *testing.T) {
	testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
	archive := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
	for _, splitStatus := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(splitStatus), func(t *testing.T) {
			var splitCalls, packageCalls int
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status, body := 200, archive
				if !strings.HasSuffix(req.URL.Path, ".tgz") {
					splitCalls++
					status, body = splitStatus, []byte(`{}`)
				} else {
					packageCalls++
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})))
			main := registryFixture("1.2.3")
			main.Upgrade = nil
			main.Dist.Tarball, main.Dist.Integrity = publicNPMMirror+"/package.tgz", npmPackageTestSRI(archive)
			release := npmManifestToRelease(main, publicNPMMirror)
			resolved, err := ResolvePlatformReleaseForSource(t.Context(), release, true, SourceAuto)
			if err != nil || resolved != nil || splitCalls != 0 {
				t.Fatalf("legacy main queried splits: %+v %v calls=%d", resolved, err, splitCalls)
			}
			prepared, err := PrepareNPMPackage(t.Context(), *release.NPM, t.TempDir(), true, nil)
			if err != nil || prepared.BinaryArchivePath == "" || packageCalls != 1 {
				t.Fatalf("legacy whole-package preparation failed: %+v %v", prepared, err)
			}
			release.NPM.Integrity = npmPackageTestSRI([]byte("different official bytes"))
			if _, err := PrepareNPMPackage(t.Context(), *release.NPM, t.TempDir(), true, nil); err == nil {
				t.Fatal("legacy fallback bypassed main package SRI")
			}
		})
	}
}

func TestCrossPlatformCoverageDownloadHTTP2StreamReset(t *testing.T) {
	ready := make(chan time.Time)
	close(ready)
	testseam.Swap(t, &downloadAfter, func(time.Duration) <-chan time.Time { return ready })
	for _, alwaysFail := range []bool{false, true} {
		t.Run(fmt.Sprint(alwaysFail), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 {
					t.Error("HTTP/2 was not negotiated")
				}
				call := calls.Add(1)
				w.Header().Set("ETag", `"original"`)
				w.Header().Set("Content-Length", "10")
				if call == 1 || alwaysFail {
					_, _ = w.Write([]byte("01234"))
					w.(http.Flusher).Flush()
					panic(http.ErrAbortHandler)
				}
				if r.Header.Get("Range") != "bytes=5-" || r.Header.Get("If-Match") != `"original"` {
					t.Errorf("resume headers: %v", r.Header)
				}
				w.Header().Set("Content-Length", "5")
				w.Header().Set("Content-Range", "bytes 5-9/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("56789"))
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			testseam.Swap(t, &http.DefaultTransport, server.Client().Transport)
			dest := filepath.Join(t.TempDir(), "asset")
			cfg := DefaultDownloadConfig()
			_, err := DownloadWithConfig(t.Context(), server.URL, dest, cfg)
			if alwaysFail {
				if err == nil || calls.Load() != 3 || !canFallbackPlatform(err) {
					t.Fatalf("exhausted stream reset: calls=%d err=%v", calls.Load(), err)
				}
				if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
					t.Fatal("partial download published")
				}
			} else {
				got, readErr := os.ReadFile(dest)
				if err != nil || readErr != nil || string(got) != "0123456789" || calls.Load() != 2 {
					t.Fatalf("resume: calls=%d content=%q err=%v read=%v", calls.Load(), got, err, readErr)
				}
			}
		})
	}
	if canFallbackPlatform(&downloadReadError{context.Canceled}) || canFallbackPlatform(&downloadFileError{errors.New("stream error")}) {
		t.Fatal("local failure or cancellation became retriable")
	}
}

func TestCrossPlatformCoverageGitHubSecondaryRateLimit(t *testing.T) {
	for _, retry := range []string{"30", "", "invalid", "-1"} {
		for _, source := range []string{"GitHub Release", "npm Registry"} {
			client := &Client{httpClient: &http.Client{Transport: upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"X-Ratelimit-Remaining": {"300"}, "Retry-After": {retry}}, Body: io.NopCloser(strings.NewReader("secondary rate limit"))}, nil
			})}}
			var payload any
			err := client.getBoundedJSON(t.Context(), "https://api.github.com/release", &payload, 1024, source, "release")
			if canFallbackPlatform(err) != (retry == "30" && source == "GitHub Release") {
				t.Fatalf("classification source=%s retry=%q err=%v", source, retry, err)
			}
		}
	}
}
