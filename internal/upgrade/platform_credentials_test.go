package upgrade

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePlatformExplicitCredential(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fixture-primary-token")
	t.Setenv("GH_TOKEN", "fixture-secondary-token")
	r := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer fixture-primary-token" {
			t.Error("官方 Release 查询没有使用显式配置的优先凭据")
		}
		body := platformCredentialFixture(t)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})))
	if _, err := ResolvePlatformRelease(context.Background(), r, true); err != nil {
		t.Fatal(err)
	}
}

func platformCredentialFixture(t *testing.T) []byte {
	t.Helper()
	name := npmPackageBinaryName(runtime.GOOS, runtime.GOARCH)
	body, err := json.Marshal(map[string]any{"tag_name": "v1.2.3", "immutable": true, "assets": []GitHubAsset{{Name: name, Size: 7, Digest: "sha256:" + strings.Repeat("a", 64), BrowserDownloadURL: "https://github.com/" + defaultOwner + "/" + defaultRepo + "/releases/download/v1.2.3/" + name}}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestCrossPlatformCoveragePlatformCredentialSelection(t *testing.T) {
	for _, tc := range []struct {
		name, primary, secondary, process, want string
		ghCalls                                 int
	}{
		{"primary", "fixture-primary", "fixture-secondary", "unused", "fixture-primary", 0},
		{"secondary", "", "fixture-secondary", "unused", "fixture-secondary", 0},
		{"local-gh", "", "", "success", "fixture-local", 1},
		{"missing-gh", "", "", "missing", "", 1},
		{"not-logged-in", "", "", "failure", "", 1},
		{"empty", "", "", "empty", "", 1},
		{"timeout", "", "", "hang", "", 1},
		{"invalid-local", "", "", "invalid", "", 1},
		{"oversized-local", "", "", "oversized", "", 1},
		{"invalid-env", "fixture\r\nInjected: value", "", "unused", "", 0},
		{"non-ascii-env", "凭据", "", "unused", "", 0},
		{"oversized-env", strings.Repeat("x", githubCredentialMaxBytes+1), "", "unused", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tc.primary)
			t.Setenv("GH_TOKEN", tc.secondary)
			t.Setenv("GH_HOST", "other.example")
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			testseam.Swap(t, &githubCredentialCommand, func(ctx context.Context, name string, args ...string) *exec.Cmd {
				calls++
				if name != "gh" || !reflect.DeepEqual(args, []string{"auth", "token", "--hostname", "github.com"}) {
					t.Fatal("凭据查询必须绑定 github.com")
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > githubCredentialTimeout {
					t.Fatal("凭据读取缺少有限截止时间")
				}
				if tc.process == "missing" {
					return exec.CommandContext(ctx, filepath.Join(t.TempDir(), "missing-gh"))
				}
				return exec.CommandContext(ctx, executable, "-test.run=^TestCrossPlatformCoverageGitHubCredentialProcess$", "--", "dws-gh-fixture-"+tc.process)
			})
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				expected := ""
				if tc.want != "" {
					expected = "Bearer " + tc.want
				}
				if req.Header.Get("Authorization") != expected {
					t.Error("凭据选择或匿名回退不符合预期")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(platformCredentialFixture(t))))}, nil
			})))
			release := npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry)
			if _, err := ResolvePlatformRelease(context.Background(), release, true); err != nil {
				t.Fatal(err)
			}
			if calls != tc.ghCalls {
				t.Fatalf("gh 调用次数=%d，预期=%d", calls, tc.ghCalls)
			}
		})
	}
}

// 子进程只输出固定测试值；父进程使用真实 os/exec 验证超时和输出隔离。
func TestCrossPlatformCoverageGitHubCredentialProcess(t *testing.T) {
	mode := os.Args[len(os.Args)-1]
	if !strings.HasPrefix(mode, "dws-gh-fixture-") {
		return
	}
	switch strings.TrimPrefix(mode, "dws-gh-fixture-") {
	case "success":
		_, _ = io.WriteString(os.Stdout, "  fixture-local\n")
	case "empty":
	case "failure":
		_, _ = io.WriteString(os.Stdout, "fixture-must-not-escape")
		_, _ = io.WriteString(os.Stderr, "fixture-must-not-escape")
		os.Exit(1)
	case "hang":
		time.Sleep(time.Minute)
	case "invalid":
		_, _ = io.WriteString(os.Stdout, "fixture\ninvalid")
	case "oversized":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", githubCredentialMaxBytes+1))
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func TestCrossPlatformCoverageGitHubCredentialCancellation(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	testseam.Swap(t, &githubCredentialCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "unused-gh")
	})
	if localGitHubToken(ctx) != "" {
		t.Fatal("取消后不应返回凭据")
	}
	var output githubCredentialBuffer
	if n, err := output.Write([]byte(strings.Repeat("x", githubCredentialMaxBytes))); err != nil || n != githubCredentialMaxBytes {
		t.Fatal("合法大小输出被拒绝")
	}
	if _, err := output.Write([]byte("x")); err == nil {
		t.Fatal("未限制累计输出")
	}
}

func TestCrossPlatformCoverageGitHubMetadataCredentialBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, target, token string
		authenticated       bool
	}{
		{"official", "https://api.github.com/repos/test/test", "fixture-token", true},
		{"anonymous", "https://api.github.com/repos/test/test", "", false},
		{"http", "http://api.github.com/repos/test/test", "fixture-token", false},
		{"subdomain", "https://sub.api.github.com/repos/test/test", "fixture-token", false},
		{"other-port", "https://api.github.com:8443/repos/test/test", "fixture-token", false},
		{"userinfo", "https://user@api.github.com/repos/test/test", "fixture-token", false},
		{"registry", "https://registry.npmjs.org/pkg", "fixture-token", false},
		{"asset", "https://github.com/test/test/releases/download/v1/a.zip", "fixture-token", false},
		{"cdn", "https://release-assets.githubusercontent.com/archive", "fixture-token", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "inherited-fixture")
			tr := githubMetadataTransport{token: tc.token, base: upgradeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				want := ""
				if tc.authenticated {
					want = "Bearer " + tc.token
				}
				if r.Header.Get("Authorization") != want {
					t.Fatal("请求的凭据隔离失败")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if req.Header.Get("Authorization") != "inherited-fixture" {
				t.Fatal("认证 transport 修改了原始请求")
			}
		})
	}
}

func TestCrossPlatformCoveragePlatformCredentialRedirectAndFailure(t *testing.T) {
	for _, mode := range []string{"cross-origin", "subdomain", "same-origin", "unauthorized"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "fixture-sensitive-token")
			calls := 0
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(upgradeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if req.Header.Get("Authorization") != "Bearer fixture-sensitive-token" {
						t.Error("首个 API 请求未认证")
					}
					if mode == "unauthorized" {
						return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture-sensitive-token"))}, nil
					}
					target := "https://other.example/metadata"
					if mode == "subdomain" {
						target = "https://sub.api.github.com/metadata"
					}
					if mode == "same-origin" {
						target = "https://api.github.com/metadata"
					}
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{target}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				want := ""
				if mode == "same-origin" {
					want = "Bearer fixture-sensitive-token"
				}
				if req.Header.Get("Authorization") != want {
					t.Error("重定向认证边界错误")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(platformCredentialFixture(t))))}, nil
			})))
			_, err := ResolvePlatformRelease(context.Background(), npmManifestToRelease(registryFixture("1.2.3"), defaultNPMRegistry), true)
			if mode == "unauthorized" {
				if err == nil || calls != 1 || strings.Contains(err.Error(), "fixture-sensitive-token") {
					t.Fatal("认证拒绝必须保留错误、不泄露凭据、不自动匿名重试")
				}
			} else if err != nil || calls != 2 {
				t.Fatalf("重定向失败: calls=%d err=%v", calls, err)
			}
		})
	}
}
