package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

type platformUpgradeTransport func(*http.Request) (*http.Response, error)

func (f platformUpgradeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// 从真实 Registry 查询、资产解析和 HTTP 下载进入升级流程；仅隔离本机安装和二进制执行。
func TestCrossPlatformCoverageUpgradePlatformDownloads(t *testing.T) {
	upgrade.SwapPlatformGitHubTokenForTest(t, func(context.Context) string { return "fixture-local-token" })
	for _, mode := range []string{"success", "skip-skills", "dry-run", "managed-preview", "tampered", "wrong-version", "missing-release", "missing-skills"} {
		t.Run(mode, func(t *testing.T) {
			for _, key := range []string{"DWS_UPGRADE_REGISTRY", "DWS_UPGRADE_URL", "DWS_UPGRADE_REPOSITORY", "GITHUB_TOKEN", "GH_TOKEN"} {
				t.Setenv(key, "")
			}
			root := t.TempDir()
			binaryName := fmt.Sprintf("dws-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
			binaryEntry := "dws"
			if runtime.GOOS == "windows" {
				binaryName = fmt.Sprintf("dws-windows-%s.zip", runtime.GOARCH)
				binaryEntry = "dws.exe"
			}
			binary := platformUpgradeArchive(t, strings.HasSuffix(binaryName, ".zip"), binaryEntry)
			skills := platformUpgradeArchive(t, true, "multi/dingtalk-shared/SKILL.md")
			const releasePath = "/repos/DingTalk-Real-AI/dingtalk-workspace-cli/releases/tags/v9.9.9"
			const assetPath = "/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v9.9.9/"
			assets := []map[string]any{}
			for name, data := range map[string][]byte{binaryName: binary, "dws-skills.zip": skills} {
				if mode == "missing-skills" && name == "dws-skills.zip" {
					continue
				}
				assets = append(assets, map[string]any{"name": name, "size": len(data), "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "browser_download_url": "https://github.com" + assetPath + name})
			}
			var mu sync.Mutex
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				wantAuth := ""
				if r.URL.Path == releasePath {
					wantAuth = "Bearer fixture-local-token"
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("凭据必须仅用于官方 Release 元数据，不能进入 Registry 或资产请求")
				}
				switch r.URL.Path {
				case "/dingtalk-workspace-cli/latest":
					_ = json.NewEncoder(w).Encode(map[string]any{"name": "dingtalk-workspace-cli", "version": "9.9.9", "dist": map[string]string{"tarball": "https://registry.npmjs.org/dingtalk-workspace-cli/-/dingtalk-workspace-cli-9.9.9.tgz", "integrity": "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))}})
				case releasePath:
					if mode == "missing-release" {
						http.NotFound(w, r)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.9.9", "immutable": true, "assets": assets})
				case assetPath + binaryName:
					data := bytes.Clone(binary)
					if mode == "tampered" {
						data[0] ^= 1
					}
					_, _ = w.Write(data)
				case assetPath + "dws-skills.zip":
					_, _ = w.Write(skills)
				default:
					t.Errorf("不应下载 npm 全包、其他平台或移动版本地址: %s", r.URL.Path)
					http.Error(w, "unexpected download", http.StatusTeapot)
				}
			}))
			t.Cleanup(server.Close)
			endpoint, _ := url.Parse(server.URL)
			transport := http.DefaultTransport
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(platformUpgradeTransport(func(r *http.Request) (*http.Response, error) {
				clone := r.Clone(r.Context())
				clone.URL.Scheme, clone.URL.Host = endpoint.Scheme, endpoint.Host
				return transport.RoundTrip(clone)
			})))
			testseam.Swap(t, &detectUpgradeInstallation, func() (upgradeInstallation, error) {
				if mode == "managed-preview" {
					return upgradeInstallation{manager: "npm", root: root, global: true}, nil
				}
				return upgradeInstallation{}, nil
			})
			testseam.Swap(t, &newUpgradeRollback, func() upgradeRollbackManager { return &fakeUpgradeRollback{} })
			testseam.Swap(t, &ensureUpgradeDirs, func() error {
				if mode == "dry-run" {
					t.Fatal("预览创建安装目录")
				}
				return nil
			})
			testseam.Swap(t, &cleanupUpgradeStale, func() {})
			testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return os.MkdirTemp(root, "upgrade-") })
			testseam.Swap(t, &upgradeTryExecVersion, func(string) ([]byte, error) {
				if mode == "wrong-version" {
					return []byte("Version: 19.9.9"), nil
				}
				return []byte("Version: 9.9.9"), nil
			})
			replaced, installed := false, false
			testseam.Swap(t, &replaceUpgradeSelf, func(string) error { replaced = true; return nil })
			testseam.Swap(t, &invalidateSchemaCacheAfterUpgrade, func() {})
			testseam.Swap(t, &locateUpgradeSkill, func(dir string) string { return filepath.Join(dir, "multi") })
			testseam.Swap(t, &installUpgradeSkills, func(string, upgrade.SkillUpgradeOptions) (*upgrade.SkillUpgradeResult, error) {
				installed = true
				return &upgrade.SkillUpgradeResult{}, nil
			})
			preview := mode == "dry-run" || mode == "managed-preview"
			err := runUpgrade(context.Background(), upgradeOptions{yes: true, force: true, skipSkills: mode == "skip-skills", dryRun: preview})
			wantSuccess := mode == "success" || mode == "skip-skills" || preview
			if (err == nil) != wantSuccess || replaced != (wantSuccess && !preview) || installed != (mode == "success") {
				t.Fatalf("err=%v replaced=%v installed=%v", err, replaced, installed)
			}
			want := []string{"/dingtalk-workspace-cli/latest", releasePath}
			if mode == "managed-preview" {
				want = want[:1]
			}
			if !preview && mode != "missing-release" && mode != "missing-skills" {
				want = append(want, assetPath+binaryName)
				if mode != "skip-skills" && mode != "tampered" {
					want = append(want, assetPath+"dws-skills.zip")
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("requests=%v, want=%v", paths, want)
			}
		})
	}
}

func platformUpgradeArchive(t *testing.T, useZip bool, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	data := []byte("fixture")
	if useZip {
		w := zip.NewWriter(&buf)
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&buf)
		w := tar.NewWriter(gz)
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}
