// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNPMSplitMetadataIdentity(t *testing.T) {
	for _, mode := range []string{"ok", "missing", "wrong-name", "wrong-version", "private", "deprecated", "wrong-asset", "bad-digest"} {
		t.Run(mode, func(t *testing.T) {
			version, name := "1.2.3", skillsZipName
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/"+npmPackageName+"-skills/"+version {
					t.Errorf("wrong exact package URL: %s", r.URL.Path)
				}
				if mode == "missing" {
					http.NotFound(w, r)
					return
				}
				m := npmSplitManifest{}
				m.Name, m.Version = npmPackageName+"-skills", version
				m.Dist.Tarball, m.Dist.Integrity = "https://registry.npmmirror.com/package.tgz", npmPackageTestSRI([]byte("fixture"))
				m.Upgrade.Format, m.Upgrade.Edition, m.Upgrade.Asset = 1, "public", name
				m.Upgrade.SHA256, m.Upgrade.Size = strings.Repeat("a", 64), 10
				switch mode {
				case "wrong-name":
					m.Name = "another-package"
				case "wrong-version":
					m.Version = "1.2.4"
				case "private":
					m.Upgrade.Edition = "private"
				case "deprecated":
					m.Deprecated = "withdrawn"
				case "wrong-asset":
					m.Upgrade.Asset = "other.zip"
				case "bad-digest":
					m.Upgrade.SHA256 = "bad"
				}
				_ = json.NewEncoder(w).Encode(m)
			}))
			defer server.Close()
			client := &Client{registryURL: server.URL + "/" + npmPackageName, httpClient: server.Client()}
			asset, err := resolveNPMSplitAsset(t.Context(), client, version, name)
			if (err == nil) != (mode == "ok") {
				t.Fatalf("mode=%s asset=%+v err=%v", mode, asset, err)
			}
			if mode == "ok" && (asset.NPM == nil || asset.NPM.Version != version || asset.NPM.Name != npmPackageName+"-skills") {
				t.Fatal("package identity was not preserved")
			}
		})
	}
}

func TestCrossPlatformCoverageNPMSplitDownloadAndIntegrity(t *testing.T) {
	for _, name := range []string{npmPackageBinaryName(runtime.GOOS, runtime.GOARCH), skillsZipName} {
		t.Run(name, func(t *testing.T) {
			suffix, err := npmSplitSuffix(name)
			if err != nil {
				t.Fatal(err)
			}
			packageName := npmPackageName + "-" + suffix
			body := "release resource"
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
			entries := []npmPackageTestEntry{
				{name: "package/package.json", data: fmt.Sprintf(`{"name":%q,"version":"1.2.3"}`, packageName)},
				{name: "package/assets/" + name, data: body},
				{name: "package/assets/checksums.txt", data: digest + "  " + name + "\n"},
			}
			tarball := npmPackageTestGzip(t, npmPackageTestTar(t, entries))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tarball) }))
			defer server.Close()
			asset := GitHubAsset{Name: name, Digest: "sha256:" + digest, Size: int64(len(body)),
				NPM: &NPMPackage{Name: packageName, Version: "1.2.3", RegistryURL: publicNPMMirror, TarballURL: server.URL, Integrity: npmPackageTestSRI(tarball)}}
			dest := filepath.Join(t.TempDir(), "download")
			if err := DownloadPlatformAsset(t.Context(), asset, dest, nil); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != body {
				t.Fatalf("asset content: %q %v", got, err)
			}
			asset.NPM.Integrity = npmPackageTestSRI([]byte("different"))
			if err := DownloadPlatformAsset(t.Context(), asset, dest, nil); err == nil {
				t.Fatal("bad outer integrity accepted")
			}
			got, _ = os.ReadFile(dest)
			if string(got) != body {
				t.Fatal("failed download replaced the destination")
			}
			asset.NPM.Integrity = npmPackageTestSRI(tarball)
			asset.Digest = "sha256:" + strings.Repeat("0", 64)
			if err := DownloadPlatformAsset(t.Context(), asset, dest, nil); err == nil {
				t.Fatal("bad inner identity accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageNPMSplitPlatformNames(t *testing.T) {
	for _, tc := range []struct{ asset, suffix string }{
		{"dws-darwin-arm64.tar.gz", "darwin-arm64"}, {"dws-darwin-amd64.tar.gz", "darwin-x64"},
		{"dws-linux-arm64.tar.gz", "linux-arm64"}, {"dws-linux-amd64.tar.gz", "linux-x64"},
		{"dws-windows-arm64.zip", "win32-arm64"}, {"dws-windows-amd64.zip", "win32-x64"},
		{skillsZipName, "skills"},
	} {
		got, err := npmSplitSuffix(tc.asset)
		if err != nil || got != tc.suffix {
			t.Fatalf("%s: %q %v", tc.asset, got, err)
		}
	}
	if _, err := npmSplitSuffix("../invalid"); err == nil {
		t.Fatal("invalid asset accepted")
	}
}

func TestCrossPlatformCoverageNPMSplitNetworkFallback(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			SwapPlatformGitHubTokenForTest(t, func(context.Context) string { return "fixture-token" })
			testseam.Swap(t, &upgradeEditionName, func() string { return "open" })
			mirrorCalls := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status, body := 200, []byte("{}")
				if req.URL.Host == "api.github.com" {
					status = 503
				} else {
					mirrorCalls++
					if req.URL.Host != "registry.npmmirror.com" || req.Header.Get("Authorization") != "" {
						t.Fatal("mirror destination or credential isolation failed")
					}
					if req.URL.Path == "/"+npmPackageName+"/1.2.3" {
						body, _ = json.Marshal(registryFixture("1.2.3"))
					} else if missing {
						status = 404
					} else {
						name := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
						if strings.Contains(req.URL.Path, "-skills/") {
							name = skillsZipName
						}
						suffix, _ := npmSplitSuffix(name)
						m := npmSplitManifest{}
						m.Name, m.Version = npmPackageName+"-"+suffix, "1.2.3"
						m.Dist.Tarball, m.Dist.Integrity = "https://registry.npmmirror.com/package.tgz", npmPackageTestSRI([]byte("fixture"))
						m.Upgrade.Format, m.Upgrade.Edition, m.Upgrade.Asset = 1, "public", name
						m.Upgrade.SHA256, m.Upgrade.Size = strings.Repeat("a", 64), 10
						body, _ = json.Marshal(m)
					}
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})))
			release := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
			var switches []DownloadEvent
			ctx := WithDownloadObserver(t.Context(), func(event DownloadEvent) {
				if event.Kind == "switch" {
					switches = append(switches, event)
				}
			})
			resolved, err := ResolvePlatformRelease(ctx, release, false)
			if len(switches) != 1 || switches[0].Previous != "github" || switches[0].Source != "npmmirror" || switches[0].Reason != "HTTP 503" {
				t.Fatalf("missing source/reason event: %+v", switches)
			}
			if missing {
				if err == nil || resolved != nil {
					t.Fatal("missing splits silently selected whole npm package")
				}
			} else if err != nil || resolved == nil || len(resolved.Assets) != 2 || resolved.Assets[0].NPM == nil {
				t.Fatalf("platform fallback failed: %+v %v", resolved, err)
			}
			if mirrorCalls < 2 {
				t.Fatal("fallback did not verify the main and split versions")
			}
		})
	}
}
