// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageUpgradeSourceCommand(t *testing.T) {
	upgrade.SwapPlatformGitHubTokenForTest(t, func(context.Context) string { return "" })
	testseam.Swap(t, &detectUpgradeInstallation, func() (upgradeInstallation, error) { return upgradeInstallation{}, nil })
	for _, source := range []string{"", "auto", "github", "npmmirror"} {
		t.Run("source="+source, func(t *testing.T) {
			for _, key := range []string{"DWS_UPGRADE_REGISTRY", "DWS_UPGRADE_URL", "DWS_UPGRADE_REPOSITORY"} {
				t.Setenv(key, "")
			}
			if source == "github" || source == "npmmirror" {
				t.Setenv("DWS_UPGRADE_REGISTRY", "https://ignored.example")
				t.Setenv("DWS_UPGRADE_URL", "https://ignored.example")
			}
			name := fmt.Sprintf("dws-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
			if runtime.GOOS == "windows" {
				name = fmt.Sprintf("dws-windows-%s.zip", runtime.GOARCH)
			}
			hosts := map[string]int{}
			testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(platformUpgradeTransport(func(req *http.Request) (*http.Response, error) {
				hosts[req.URL.Host]++
				var payload any
				switch {
				case req.URL.Path == "/dingtalk-workspace-cli/latest" || req.URL.Path == "/dingtalk-workspace-cli/9.9.9":
					payload = map[string]any{"name": "dingtalk-workspace-cli", "version": "9.9.9",
						"dist": map[string]string{
							"tarball":   "https://" + req.URL.Host + "/dingtalk-workspace-cli/-/dingtalk-workspace-cli-9.9.9.tgz",
							"integrity": "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64)),
						},
						"dwsUpgrade": map[string]any{"format": 1, "edition": "public", "assets": map[string]any{
							name: map[string]any{"size": 10, "sha256": strings.Repeat("a", 64)},
						}},
					}
				case strings.HasPrefix(req.URL.Path, "/repos/"):
					payload = map[string]any{"tag_name": "v9.9.9", "immutable": true, "assets": []map[string]any{{
						"name": name, "size": 10, "digest": "sha256:" + strings.Repeat("a", 64),
						"browser_download_url": "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v9.9.9/" + name,
					}}}
				case strings.HasSuffix(req.URL.Path, "/9.9.9"):
					payload = map[string]any{"name": strings.Split(req.URL.Path, "/")[1], "version": "9.9.9",
						"dist":       map[string]string{"tarball": "https://registry.npmmirror.com/platform.tgz", "integrity": "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))},
						"dwsUpgrade": map[string]any{"format": 1, "edition": "public", "asset": name, "size": 10, "sha256": strings.Repeat("a", 64)},
					}
				default:
					t.Fatalf("dry-run downloaded an archive or requested an unexpected path: %s", req.URL.Path)
				}
				data, _ := json.Marshal(payload)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data))}, nil
			})))
			cmd := newUpgradeCommand()
			cmd.Flags().Bool("dry-run", false, "preview")
			args := []string{"--dry-run", "--force", "--skip-skills"}
			if source != "" {
				args = append(args, "--source", source)
			}
			cmd.SetArgs(args)
			if err := corecmd.ExecuteForTest(cmd); err != nil {
				t.Fatal(err)
			}
			if source == "npmmirror" {
				if len(hosts) != 1 || hosts["registry.npmmirror.com"] == 0 {
					t.Fatalf("mirror routing: %v", hosts)
				}
			} else if len(hosts) != 2 || hosts["registry.npmjs.org"] == 0 || hosts["api.github.com"] == 0 {
				t.Fatalf("github routing: %v", hosts)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeSourceValidation(t *testing.T) {
	testseam.Swap(t, &http.DefaultTransport, http.RoundTripper(platformUpgradeTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid source options made a network request")
		return nil, nil
	})))
	for _, args := range [][]string{{"--source", "bad"}, {"--source", ""}, {"--rollback", "--source", "github"}} {
		cmd := newUpgradeCommand()
		cmd.SetArgs(args)
		if err := corecmd.ExecuteForTest(cmd); apperrors.ExitCode(err) != 3 {
			t.Fatalf("args=%v error=%v, want validation/3", args, err)
		}
	}
	if err := runUpgrade(t.Context(), upgradeOptions{source: "bad"}); apperrors.ExitCode(err) != 3 {
		t.Fatalf("invalid internal source error=%v, want validation/3", err)
	}
	cmd := newUpgradeCommand()
	complete, ok := cmd.GetFlagCompletionFunc("source")
	if !ok {
		t.Fatal("source completion is missing")
	}
	choices, directive := complete(cmd, nil, "")
	if strings.Join(choices, ",") != "auto,github,npmmirror" || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("source completion = %v, %v", choices, directive)
	}
	for _, manager := range []string{"npm", "pnpm", "brew"} {
		t.Run(manager, func(t *testing.T) {
			testseam.Swap(t, &detectUpgradeInstallation, func() (upgradeInstallation, error) { return upgradeInstallation{manager: manager}, nil })
			if err := runUpgrade(t.Context(), upgradeOptions{source: upgrade.SourceGitHub}); apperrors.ExitCode(err) != 3 {
				t.Fatalf("managed source error=%v, want validation/3", err)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeSourceProgressOutput(t *testing.T) {
	var out bytes.Buffer
	writeUpgradeDownloadEvent(&out, upgrade.DownloadEvent{Kind: "source", Source: "npmmirror", Resource: "完整 npm 包", Reason: "历史版本兼容"})
	writeUpgradeDownloadEvent(&out, upgrade.DownloadEvent{Kind: "switch", Previous: "github", Source: "npmmirror", Resource: "platform.tgz", Reason: "HTTP 503"})
	writeUpgradeDownloadEvent(&out, upgrade.DownloadEvent{Kind: "attempt", Source: "npmmirror", Resource: "platform.tgz", Attempt: 2, MaxAttempts: 3, Reason: "下载超时或停滞", RetryDelay: time.Second, ResumeBytes: 1 << 20})
	for _, want := range []string{"历史版本兼容", "github → npmmirror", "HTTP 503", "当前来源: npmmirror", "尝试 2/3", "重试 1/2", "等待 1s", "从 1.00 MiB 续传"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
}
