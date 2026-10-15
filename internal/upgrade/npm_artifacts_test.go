// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNPMPlatformArtifactsAndFallback(t *testing.T) {
	for _, scenario := range []string{"platform", "skip-skills", "network-fallback", "bad-hash", "bad-size", "missing-platform", "invalid-manifest"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "platform" {
				// 全包镜像不参与按平台下载，也不改变平台清单的来源。
				t.Setenv("DWS_UPGRADE_NPM_MIRROR", "unused-invalid-mirror")
			}
			binary := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
			requests := []string{}
			legacy := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				if r.Header.Get("Authorization") != "" {
					t.Error("升级下载携带了凭据")
				}
				if r.URL.Path == "/package.tgz" {
					_, _ = w.Write(legacy)
					return
				}
				if scenario == "network-fallback" {
					http.NotFound(w, r)
					return
				}
				_, _ = io.WriteString(w, "artifact")
			}))
			defer srv.Close()
			pkg := npmPackageTestIdentity()
			pkg.TarballURL, pkg.Integrity = srv.URL+"/package.tgz", npmPackageTestSRI(legacy)
			pkg.Artifacts = &NPMArtifacts{Version: 1, Files: map[string]NPMArtifact{}}
			for _, name := range []string{binary, skillsZipName} {
				pkg.Artifacts.Files[name] = NPMArtifact{URLs: []string{srv.URL + "/" + name}, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("artifact"))), Size: 8}
			}
			switch scenario {
			case "bad-hash":
				a := pkg.Artifacts.Files[binary]
				a.SHA256 = strings.Repeat("0", 64)
				pkg.Artifacts.Files[binary] = a
			case "bad-size":
				a := pkg.Artifacts.Files[binary]
				a.Size = 9
				pkg.Artifacts.Files[binary] = a
			case "missing-platform":
				delete(pkg.Artifacts.Files, binary)
			case "invalid-manifest":
				pkg.Artifacts.Version = 2
			}
			dest := t.TempDir()
			progressCount := 0
			result, err := PrepareNPMPackage(t.Context(), pkg, dest, scenario == "skip-skills", func(percent float64, downloaded, total int64) {
				progressCount++
				if percent < 0 || percent > 100 || downloaded < 0 || total < downloaded {
					t.Errorf("非法下载进度: %v %d/%d", percent, downloaded, total)
				}
			})
			fail := strings.HasPrefix(scenario, "bad-") || scenario == "missing-platform" || scenario == "invalid-manifest"
			if (err != nil) != fail {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if fail {
				for _, p := range requests {
					if p == "/package.tgz" {
						t.Fatal("校验失败不允许回退完整包")
					}
				}
				children, _ := os.ReadDir(dest)
				if len(children) != 0 {
					t.Fatalf("失败未清理下载文件: %v", children)
				}
				return
			}
			if progressCount == 0 {
				t.Fatal("没有报告下载进度")
			}
			if scenario == "network-fallback" {
				if !reflect.DeepEqual(requests, []string{"/" + binary, "/package.tgz"}) {
					t.Fatalf("回退请求=%v", requests)
				}
			} else {
				expected := []string{"/" + binary}
				if scenario != "skip-skills" {
					expected = append(expected, "/"+skillsZipName)
				}
				if !reflect.DeepEqual(requests, expected) {
					t.Fatalf("下载了其他平台或完整包: %v", requests)
				}
				body, readErr := os.ReadFile(result.BinaryArchivePath)
				if readErr != nil || string(body) != "artifact" {
					t.Fatalf("body=%q err=%v", body, readErr)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageNPMSourceBudgetFallsBackWithoutCancellingCaller(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer second.Close()
	cfg := DefaultDownloadConfig()
	cfg.Timeout = 50 * time.Millisecond
	n, err := downloadFromSources(t.Context(), []string{first.URL, second.URL}, filepath.Join(t.TempDir(), "out"), cfg)
	if err != nil || n != 2 || t.Context().Err() != nil {
		t.Fatalf("单源超时没有回退或取消了外层请求: n=%d err=%v", n, err)
	}
}

func TestCrossPlatformCoverageNPMMirrorPreservesOriginalIntegrity(t *testing.T) {
	for _, scenario := range []string{"mirror-success", "mirror-unavailable", "mirror-integrity-failure"} {
		t.Run(scenario, func(t *testing.T) {
			body := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
			pkg := npmPackageTestIdentity()
			pkg.RegistryURL = defaultNPMRegistry
			pkg.TarballURL = defaultNPMRegistry + "/" + npmPackageName + "/-/" + npmPackageName + "-" + pkg.Version + ".tgz"
			pkg.Integrity = npmPackageTestSRI(body)
			var hosts []string
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				hosts = append(hosts, r.URL.Hostname())
				data, status := body, http.StatusOK
				if len(hosts) == 1 {
					if scenario == "mirror-unavailable" {
						status = http.StatusNotFound
					}
					if scenario == "mirror-integrity-failure" {
						data = []byte("corrupt mirror")
					}
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
			})))
			var events []DownloadEvent
			_, err := PrepareNPMPackageWithEvents(t.Context(), pkg, t.TempDir(), false, nil, func(e DownloadEvent) { events = append(events, e) })
			if (err != nil) != (scenario == "mirror-integrity-failure") {
				t.Fatalf("err=%v", err)
			}
			expected := []string{"registry.npmmirror.com"}
			if scenario == "mirror-unavailable" {
				expected = append(expected, "registry.npmjs.org")
				if len(events) != 2 || events[1].Err == nil {
					t.Fatalf("没有显示换源原因: %v", events)
				}
			}
			if !reflect.DeepEqual(hosts, expected) {
				t.Fatalf("来源或完整性边界错误: %v", hosts)
			}
		})
	}
}

func TestCrossPlatformCoverageNPMArtifactManifestValidation(t *testing.T) {
	for _, edit := range []func(*NPMArtifacts){
		func(m *NPMArtifacts) { m.Version = 2 },
		func(m *NPMArtifacts) { m.Files = nil },
		func(m *NPMArtifacts) { m.Files["../dws"] = m.Files[skillsZipName] },
		func(m *NPMArtifacts) { a := m.Files[skillsZipName]; a.SHA256 = "bad"; m.Files[skillsZipName] = a },
		func(m *NPMArtifacts) { a := m.Files[skillsZipName]; a.Size = 0; m.Files[skillsZipName] = a },
		func(m *NPMArtifacts) { a := m.Files[skillsZipName]; a.URLs = nil; m.Files[skillsZipName] = a },
		func(m *NPMArtifacts) {
			a := m.Files[skillsZipName]
			a.URLs = []string{"http://remote.test/file"}
			m.Files[skillsZipName] = a
		},
	} {
		manifest := registryFixture("1.2.3")
		manifest.Artifacts = &NPMArtifacts{Version: 1, Files: map[string]NPMArtifact{skillsZipName: {URLs: []string{"https://example.test/skills"}, SHA256: strings.Repeat("0", 64), Size: 1}}}
		edit(manifest.Artifacts)
		if err := validateNPMManifest(manifest, ReleaseTrackRelease); err == nil {
			t.Fatal("无效平台清单被接受")
		}
	}
	manifest := registryFixture("1.2.3")
	manifest.Artifacts = &NPMArtifacts{Version: 1, Files: map[string]NPMArtifact{skillsZipName: {URLs: []string{"https://example.test/skills"}, SHA256: strings.Repeat("0", 64), Size: 1}}}
	if got := npmManifestToRelease(manifest, defaultNPMRegistry); got.NPM.Artifacts != manifest.Artifacts {
		t.Fatal("Registry 丢失按平台下载清单")
	}
}

func TestCrossPlatformCoverageNPMSourcesRespectExplicitRegistry(t *testing.T) {
	manifest := registryFixture("1.2.3")
	pkg := *npmManifestToRelease(manifest, defaultNPMRegistry).NPM
	if urls, err := npmTarballURLs(pkg); err != nil || len(urls) != 2 || !strings.HasPrefix(urls[0], "https://registry.npmmirror.com/") || urls[1] != pkg.TarballURL {
		t.Fatalf("默认源=%v", urls)
	}
	pkg.RegistryURL = "https://private.test/npm"
	if urls, err := npmTarballURLs(pkg); err != nil || !reflect.DeepEqual(urls, []string{pkg.TarballURL}) {
		t.Fatalf("自定义 Registry 被切换来源: %v", urls)
	}
	pkg.RegistryURL = defaultNPMRegistry
	pkg.TarballURL = "https://other.test/package.tgz"
	if urls, err := npmTarballURLs(pkg); err != nil || len(urls) != 1 {
		t.Fatalf("非规范 URL 被改写: %v", urls)
	}

	var received int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received++; http.NotFound(w, r) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received++; _, _ = io.WriteString(w, "valid") }))
	defer second.Close()
	n, err := downloadFromSources(t.Context(), []string{first.URL, second.URL}, filepath.Join(t.TempDir(), "out"), DefaultDownloadConfig())
	if err != nil || n != 5 || received != 2 {
		t.Fatalf("网络失败未回退: n=%d requests=%d err=%v", n, received, err)
	}
	if _, err := downloadFromSources(t.Context(), nil, filepath.Join(t.TempDir(), "out"), DefaultDownloadConfig()); err == nil {
		t.Fatal("空来源没有返回错误")
	}
	pkg.Artifacts = &NPMArtifacts{Version: 1, Files: map[string]NPMArtifact{skillsZipName: {URLs: []string{second.URL}, SHA256: strings.Repeat("0", 64), Size: 1}}}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareNPMArtifacts(t.Context(), pkg, file, false, nil, nil); err == nil {
		t.Fatal("目录创建失败被忽略")
	}
}

func TestCrossPlatformCoverageNPMCustomDownloadMirror(t *testing.T) {
	for _, scenario := range []string{"success", "unavailable", "integrity-failure", "official-only", "private-registry", "noncanonical-tarball", "invalid-mirror"} {
		t.Run(scenario, func(t *testing.T) {
			mirror := "https://mirror.example.test/repository/npm/"
			if scenario == "official-only" {
				mirror = defaultNPMRegistry
			}
			if scenario == "invalid-mirror" {
				mirror = "http://mirror.example.test"
			}
			t.Setenv("DWS_UPGRADE_NPM_MIRROR", mirror)
			body := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
			pkg := *npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry).NPM
			pkg.Integrity = npmPackageTestSRI(body)
			if scenario == "private-registry" {
				pkg.RegistryURL = "https://private.example.test/npm"
				pkg.TarballURL = "https://private.example.test/archive.tgz"
			}
			if scenario == "noncanonical-tarball" {
				pkg.TarballURL = "https://custom.example.test/archive.tgz"
			}
			var requests []string
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.URL.String())
				if r.Header.Get("Authorization") != "" {
					t.Error("下载镜像请求携带了凭据")
				}
				data, status := body, http.StatusOK
				if r.URL.Host == "mirror.example.test" {
					if scenario == "unavailable" {
						status = http.StatusNotFound
					}
					if scenario == "integrity-failure" {
						data = []byte("tampered package")
					}
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
			})))
			var events []DownloadEvent
			_, err := PrepareNPMPackageWithEvents(t.Context(), pkg, t.TempDir(), false, nil, func(e DownloadEvent) { events = append(events, e) })
			wantError := scenario == "integrity-failure" || scenario == "invalid-mirror"
			if (err != nil) != wantError {
				t.Fatalf("err=%v, wantError=%v", err, wantError)
			}
			want := []string{"https://mirror.example.test/repository/npm/dingtalk-workspace-cli/-/dingtalk-workspace-cli-1.2.3.tgz"}
			switch scenario {
			case "unavailable":
				want = append(want, pkg.TarballURL)
				if len(events) != 2 || events[1].Err == nil {
					t.Fatalf("没有报告回退原因: %v", events)
				}
			case "official-only", "private-registry", "noncanonical-tarball":
				want = []string{pkg.TarballURL}
			case "invalid-mirror":
				want = nil
				if err == nil || !strings.Contains(err.Error(), "DWS_UPGRADE_NPM_MIRROR") {
					t.Fatalf("没有指出无效配置项: %v", err)
				}
			}
			if !reflect.DeepEqual(requests, want) {
				t.Fatalf("下载来源=%v, want=%v", requests, want)
			}
		})
	}
}

func TestCrossPlatformCoverageNPMMirrorKeepsOfficialVersionAuthority(t *testing.T) {
	t.Setenv("DWS_UPGRADE_URL", "")
	t.Setenv("DWS_UPGRADE_REPOSITORY", "")
	t.Setenv("DWS_UPGRADE_REGISTRY", "")
	t.Setenv("DWS_UPGRADE_NPM_MIRROR", "https://download.example.test")
	body := npmPackageTestGzip(t, npmPackageTestTar(t, npmPackageTestEntries(runtime.GOOS, runtime.GOARCH)))
	manifest := registryFixture("1.2.3")
	manifest.Dist.Integrity = npmPackageTestSRI(body)
	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var requests []string
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		data := body
		if r.URL.Path == "/dingtalk-workspace-cli/latest" {
			data = metadata
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
	})))
	release, err := NewVersionClient().FetchLatestRelease()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareNPMPackage(t.Context(), *release.NPM, t.TempDir(), false, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		defaultNPMRegistry + "/dingtalk-workspace-cli/latest",
		"https://download.example.test/dingtalk-workspace-cli/-/dingtalk-workspace-cli-1.2.3.tgz",
	}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("版本查询或下载来源错误: %v", requests)
	}
}

func TestCrossPlatformCoverageNPMMirrorURLValidation(t *testing.T) {
	pkg := *npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry).NPM
	for _, raw := range []string{
		"http://remote.test", "https://user:secret@example.test", "https://example.test?key=secret",
		"https://example.test#fragment", "https://example.test/../npm", "https://example.test/%2e%2e/npm",
		"https://example.test\\npm", " https://example.test", "not-a-url",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DWS_UPGRADE_NPM_MIRROR", raw)
			if urls, err := npmTarballURLs(pkg); err == nil || urls != nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("非法配置未拒绝或错误包含凭据: urls=%v err=%v", urls, err)
			}
		})
	}
	t.Setenv("DWS_UPGRADE_NPM_MIRROR", "http://127.0.0.1:12345/npm/")
	urls, err := npmTarballURLs(pkg)
	if err != nil || len(urls) != 2 || urls[0] != "http://127.0.0.1:12345/npm/dingtalk-workspace-cli/-/dingtalk-workspace-cli-1.2.3.tgz" {
		t.Fatalf("本机测试镜像不支持: urls=%v err=%v", urls, err)
	}
}
