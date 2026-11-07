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
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePlatformSourceBoundary(t *testing.T) {
	SwapPlatformGitHubTokenForTest(t, func(context.Context) string { return "" })
	for _, mode := range []string{"official", "github", "custom-registry", "private-edition", "wrong-name", "wrong-version", "invalid-version", "custom-tarball"} {
		t.Run(mode, func(t *testing.T) {
			r := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
			wantError := false
			switch mode {
			case "official":
				wantError = true
			case "github":
				r.NPM = nil
			case "custom-registry":
				r.NPM.RegistryURL = "https://private.example/npm"
			case "private-edition":
				testseam.Swap(t, &upgradeEditionName, func() string { return "internal" })
			case "wrong-name":
				r.NPM.Name = "other"
				wantError = true
			case "wrong-version":
				r.NPM.Version = "1.2.4"
				wantError = true
			case "invalid-version":
				r.NPM.Version = "../latest"
				r.Version = r.NPM.Version
				wantError = true
			case "custom-tarball":
				r.NPM.TarballURL = "https://private.example/package.tgz"
				wantError = true
			}
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(*http.Request) (*http.Response, error) {
				if mode == "official" {
					return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
				}
				t.Fatal("来源隔离失败：不应请求官方 GitHub")
				return nil, nil
			})))
			resolved, err := ResolvePlatformRelease(context.Background(), r, false)
			if (err != nil) != wantError || resolved != nil {
				t.Fatalf("resolved=%v err=%v", resolved, err)
			}
		})
	}
}

func TestCrossPlatformCoveragePlatformReleaseResolution(t *testing.T) {
	for _, mode := range []string{"success", "skip-skills", "beta", "wrong-tag", "draft", "mutable", "wrong-track", "missing-binary", "missing-skills", "duplicate", "wrong-url", "no-digest", "invalid-digest", "wrong-size", "http-error", "bad-json", "oversized-json"} {
		t.Run(mode, func(t *testing.T) {
			r := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
			if mode == "beta" {
				r = npmManifestToRelease(registryFixture("1.2.4-beta.1"), defaultNPMRegistry)
			}
			tag := "v" + r.Version
			asset := func(name string) GitHubAsset {
				return GitHubAsset{Name: name, Size: 7, Digest: "sha256:" + strings.Repeat("a", 64), BrowserDownloadURL: "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/" + tag + "/" + name}
			}
			gh := struct {
				GitHubRelease
				Immutable bool `json:"immutable"`
			}{GitHubRelease: GitHubRelease{TagName: tag, Prerelease: r.Prerelease, Assets: []GitHubAsset{asset("dws-windows-arm64.zip"), asset(skillsZipName), asset("dws-linux-amd64.tar.gz")}}, Immutable: true}
			switch mode {
			case "skip-skills", "missing-skills":
				gh.Assets = gh.Assets[:1]
			case "wrong-tag":
				gh.TagName = "v9.9.9"
			case "draft":
				gh.Draft = true
			case "mutable":
				gh.Immutable = false
			case "wrong-track":
				gh.Prerelease = true
			case "missing-binary":
				gh.Assets = gh.Assets[1:]
			case "duplicate":
				gh.Assets = append(gh.Assets, gh.Assets[0])
			case "wrong-url":
				gh.Assets[0].BrowserDownloadURL = "https://github.com/other/repo/releases/download/" + tag + "/dws-windows-arm64.zip"
			case "no-digest":
				gh.Assets[0].Digest = ""
			case "invalid-digest":
				gh.Assets[0].Digest = "sha256:" + strings.Repeat("z", 64)
			case "wrong-size":
				gh.Assets[0].Size = 0
			}
			client := &Client{httpClient: &http.Client{Transport: upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != gitHubAPIBase+"/repos/"+defaultOwner+"/"+defaultRepo+"/releases/tags/"+tag || req.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				data, err := json.Marshal(gh)
				if err != nil {
					t.Fatal(err)
				}
				status := http.StatusOK
				if mode == "http-error" {
					status = http.StatusNotFound
				}
				if mode == "bad-json" {
					data = []byte("{")
				}
				if mode == "oversized-json" {
					data = []byte(strings.Repeat(" ", registryManifestLimit+1))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
			})}}
			original := append([]GitHubAsset(nil), r.Assets...)
			resolved, err := resolvePlatformRelease(context.Background(), client, r, mode == "skip-skills", "windows", "arm64")
			wantOK := mode == "success" || mode == "skip-skills" || mode == "beta"
			if (err == nil) != wantOK {
				t.Fatalf("resolved=%v err=%v", resolved, err)
			}
			if !reflect.DeepEqual(original, r.Assets) {
				t.Fatal("修改了版本查询结果")
			}
			if wantOK {
				wantLen := 2
				if mode == "skip-skills" {
					wantLen = 1
				}
				if resolved.NPM != r.NPM || resolved.Version != r.Version || len(resolved.Assets) != wantLen || resolved.Assets[0].Name != "dws-windows-arm64.zip" {
					t.Fatalf("resolved=%+v", resolved)
				}
			}
		})
	}
}

func TestCrossPlatformCoveragePlatformAssetDownload(t *testing.T) {
	for _, mode := range []string{"success", "without-progress", "short", "oversized", "tampered", "invalid-digest", "invalid-url", "size-limit", "write-failed", "cancelled", "http-error", "unsafe-redirect"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("platform archive")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("资产请求携带凭据")
				}
				body := data
				switch mode {
				case "http-error":
					http.NotFound(w, r)
					return
				case "unsafe-redirect":
					http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
					return
				case "short":
					body = data[:len(data)-1]
				case "oversized":
					body = append(append([]byte{}, data...), '!')
				case "tampered":
					body = []byte(strings.Repeat("!", len(data)))
				}
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)
			asset := GitHubAsset{Name: "dws-linux-arm64.tar.gz", BrowserDownloadURL: server.URL, Size: int64(len(data)), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(data))}
			switch mode {
			case "invalid-digest":
				asset.Digest = "sha256:bad"
			case "invalid-url":
				asset.BrowserDownloadURL = "http://public.example/archive"
			case "size-limit":
				asset.Size = platformArchiveMaxBytes + 1
			}
			dest := filepath.Join(t.TempDir(), "archive")
			if mode == "write-failed" {
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			var last int64
			progress := func(percent float64, downloaded, total int64) {
				last = downloaded
				if total != asset.Size {
					t.Errorf("total=%d", total)
				}
			}
			if mode == "without-progress" {
				progress = nil
			}
			err := DownloadPlatformAsset(ctx, asset, dest, progress)
			wantOK := mode == "success" || mode == "without-progress"
			if (err == nil) != wantOK {
				t.Fatal(err)
			}
			if mode == "success" && last != asset.Size {
				t.Fatalf("progress=%d", last)
			}
		})
	}
}

func TestCrossPlatformCoveragePlatformAssetRedirect(t *testing.T) {
	origin, _ := http.NewRequest(http.MethodGet, "https://github.com/owner/repo/releases/download/v1.2.3/archive", nil)
	for _, tc := range []struct {
		name, target string
		hops         int
		wantError    bool
	}{
		{"signed-cdn", "https://release-assets.githubusercontent.com/archive?sig=fixture&expires=123", 1, false},
		{"downgrade", "http://127.0.0.1/archive", 1, true},
		{"credentials", "https://user:secret@example.com/archive?sig=fixture", 1, true},
		{"fragment", "https://example.com/archive#fragment", 1, true},
		{"loop", "https://example.com/archive", 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			via := make([]*http.Request, tc.hops)
			for i := range via {
				via[i] = origin
			}
			if err := platformAssetRedirect(req, via); (err != nil) != tc.wantError {
				t.Fatal(err)
			}
		})
	}
}
