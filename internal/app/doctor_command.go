// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// checkStatus represents the outcome of a single doctor check.
type checkStatus string

const (
	statusPass checkStatus = "pass"
	statusWarn checkStatus = "warn"
	statusFail checkStatus = "fail"
)

// checkResult holds the outcome of a single doctor check.
type checkResult struct {
	Name    string      `json:"name"`
	Status  checkStatus `json:"status"`
	Message string      `json:"message"`
	Hint    string      `json:"hint,omitempty"`
	Detail  any         `json:"detail,omitempty"`
}

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "doctor",
		Short:             "环境健康检查",
		Long:              "一键检查登录态、网络连通性、缓存状态和版本更新，快速定位常见问题。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE:              runDoctor,
	}
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	cmd.Flags().Int("timeout", 10, "网络检查超时时间 (秒)")
	cmd.Flags().Bool("perf", false, "额外展示最近一次性能报告")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	timeout, _ := cmd.Flags().GetInt("timeout")
	if timeout <= 0 {
		timeout = 10
	}
	networkTimeout := time.Duration(timeout) * time.Second

	w := cmd.OutOrStdout()
	checks := make([]checkResult, 0, 4)

	authResult := doctorCheckAuth(cmd.Context(), w, jsonOut)
	checks = append(checks, authResult)

	networkResult := doctorCheckNetwork(cmd.Context(), w, jsonOut, networkTimeout)
	checks = append(checks, networkResult)

	cacheResult := doctorCheckCache(w, jsonOut)
	checks = append(checks, cacheResult)

	versionResult := doctorCheckVersion(w, jsonOut, networkTimeout)
	checks = append(checks, versionResult)

	showPerf, _ := cmd.Flags().GetBool("perf")
	if showPerf {
		perfResult := doctorCheckPerf(w, jsonOut)
		checks = append(checks, perfResult)
	}

	pass, warn, fail := countResults(checks)

	if jsonOut {
		result := map[string]any{
			"kind":   "doctor",
			"checks": checks,
			"summary": map[string]int{
				"pass": pass,
				"warn": warn,
				"fail": fail,
			},
		}
		if showPerf {
			if report, err := LoadLatestReport(); err == nil {
				result["perf_report"] = report
			}
		}
		return output.WriteJSON(w, result)
	}

	fmt.Fprintf(w, "\n诊断完成: %d 项通过, %d 项警告, %d 项失败\n", pass, warn, fail)
	if fail > 0 {
		return fmt.Errorf("诊断发现 %d 项失败", fail)
	}
	return nil
}

// ── Auth check ──────────────────────────────────────────────────────────

func doctorCheckAuth(ctx context.Context, w io.Writer, jsonOut bool) checkResult {
	if !jsonOut {
		fmt.Fprint(w, "检查登录状态...       ")
	}

	configDir := defaultConfigDir()
	provider := authpkg.NewOAuthProvider(configDir, nil)
	configureOAuthProviderCompatibility(provider, configDir)

	data, err := provider.Status()
	if err != nil || data == nil {
		r := checkResult{Name: "auth", Status: statusFail, Message: "未登录"}
		if !edition.Get().IsEmbedded {
			r.Hint = "运行 dws auth login 进行登录"
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	if data.IsAccessTokenValid() || data.IsRefreshTokenValid() {
		if !data.IsAccessTokenValid() {
			refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			_, refreshErr := provider.GetAccessToken(refreshCtx)
			cancel()
			if refreshErr != nil {
				r := checkResult{
					Name:    "auth",
					Status:  statusWarn,
					Message: "Refresh Token 有效, 但自动刷新 Access Token 失败",
					Hint:    "运行 dws auth login 重新登录",
				}
				if !jsonOut {
					printCheckResult(w, r)
				}
				return r
			}
		}

		r := checkResult{
			Name:    "auth",
			Status:  statusPass,
			Message: "已登录",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	r := checkResult{Name: "auth", Status: statusFail, Message: "登录已过期"}
	if !edition.Get().IsEmbedded {
		r.Hint = "运行 dws auth login 重新登录"
	}
	if !jsonOut {
		printCheckResult(w, r)
	}
	return r
}

// ── Network check ───────────────────────────────────────────────────────

func doctorCheckNetwork(ctx context.Context, w io.Writer, jsonOut bool, timeout time.Duration) checkResult {
	if !jsonOut {
		fmt.Fprint(w, "检查网络连通性...     ")
	}

	baseURL := cli.DefaultMarketBaseURL
	httpClient := &http.Client{Timeout: timeout}
	client := market.NewClient(baseURL, httpClient)

	start := time.Now()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := client.FetchServers(reqCtx, 1)
	latency := time.Since(start)

	if err != nil {
		r := checkResult{
			Name:    "network",
			Status:  statusFail,
			Message: fmt.Sprintf("mcp.dingtalk.com 不可达: %v", err),
			Hint:    "请检查网络连接或代理设置",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	r := checkResult{
		Name:    "network",
		Status:  statusPass,
		Message: fmt.Sprintf("mcp.dingtalk.com 可达 (延迟 %dms)", latency.Milliseconds()),
	}
	if !jsonOut {
		printCheckResult(w, r)
	}
	return r
}

// ── Cache check ─────────────────────────────────────────────────────────

func doctorCheckCache(w io.Writer, jsonOut bool) checkResult {
	if !jsonOut {
		fmt.Fprint(w, "检查缓存状态...       ")
	}

	store := cacheStoreFromEnv()
	files, _, err := cacheDirectoryStats(store.Root)
	if err != nil {
		r := checkResult{
			Name:    "cache",
			Status:  statusFail,
			Message: fmt.Sprintf("缓存目录不可读: %v", err),
			Hint:    "运行 dws cache clean 清理后重试",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	entries, _ := store.ListToolsCacheEntries(config.DefaultPartition)

	if files == 0 && len(entries) == 0 {
		r := checkResult{
			Name:    "cache",
			Status:  statusWarn,
			Message: "缓存为空 (首次使用)",
			Hint:    "运行任意 dws 命令后将自动建立缓存",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	staleCount := 0
	for _, e := range entries {
		if e.Freshness == cache.FreshnessStale {
			staleCount++
		}
	}

	if staleCount > 0 {
		r := checkResult{
			Name:    "cache",
			Status:  statusWarn,
			Message: fmt.Sprintf("%d 个文件, %d 个工具缓存, %d 个已过期", files, len(entries), staleCount),
			Hint:    "运行 dws cache refresh 刷新缓存",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	msg := fmt.Sprintf("%d 个文件, %d 个工具缓存", files, len(entries))
	if len(entries) > 0 {
		msg += ", 全部新鲜"
	}
	r := checkResult{
		Name:    "cache",
		Status:  statusPass,
		Message: msg,
	}
	if !jsonOut {
		printCheckResult(w, r)
	}
	return r
}

// ── Version check ───────────────────────────────────────────────────────

func doctorCheckVersion(w io.Writer, jsonOut bool, timeout time.Duration) checkResult {
	if !jsonOut {
		fmt.Fprint(w, "检查版本更新...       ")
	}

	currentVer := version

	client := upgrade.NewClient()
	latest, err := client.FetchLatestRelease()
	if err != nil {
		r := checkResult{
			Name:    "version",
			Status:  statusFail,
			Message: fmt.Sprintf("无法获取最新版本: %v", err),
			Hint:    "请检查网络连接",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	if upgrade.NeedsUpgrade(currentVer, latest.Version) {
		r := checkResult{
			Name:    "version",
			Status:  statusWarn,
			Message: fmt.Sprintf("有新版本 (当前 %s, 最新 v%s)", ensureV(currentVer), latest.Version),
			Hint:    "运行 dws upgrade 升级到最新版本",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	r := checkResult{
		Name:    "version",
		Status:  statusPass,
		Message: fmt.Sprintf("已是最新版本 %s", ensureV(currentVer)),
	}
	if !jsonOut {
		printCheckResult(w, r)
	}
	return r
}

// ── Output helpers ──────────────────────────────────────────────────────

func printCheckResult(w io.Writer, r checkResult) {
	icon := statusIcon(r.Status)
	fmt.Fprintf(w, "%s %s\n", icon, r.Message)
	if r.Hint != "" {
		fmt.Fprintf(w, "                      %s\n", r.Hint)
	}
}

func statusIcon(s checkStatus) string {
	switch s {
	case statusPass:
		return "✅"
	case statusWarn:
		return "⚠️"
	case statusFail:
		return "❌"
	default:
		return "?"
	}
}

func countResults(checks []checkResult) (pass, warn, fail int) {
	for _, c := range checks {
		switch c.Status {
		case statusPass:
			pass++
		case statusWarn:
			warn++
		case statusFail:
			fail++
		}
	}
	return
}

// ── Perf report check ──────────────────────────────────────────────────

func doctorCheckPerf(w io.Writer, jsonOut bool) checkResult {
	if !jsonOut {
		fmt.Fprint(w, "检查性能报告...       ")
	}

	report, err := LoadLatestReport()
	if err != nil {
		r := checkResult{
			Name:    "perf",
			Status:  statusWarn,
			Message: "未找到性能报告",
			Hint:    "设置 DWS_PERF_REPORT=auto 后运行任意命令生成报告",
		}
		if !jsonOut {
			printCheckResult(w, r)
		}
		return r
	}

	r := checkResult{
		Name:    "perf",
		Status:  statusPass,
		Message: fmt.Sprintf("报告可用 (%s, %s)", report.Command, report.Timestamp.Local().Format("2006-01-02 15:04")),
	}
	if !jsonOut {
		printCheckResult(w, r)
		printPerfReportSummary(w, report)
	}
	return r
}

func printPerfReportSummary(w io.Writer, report *PerfReport) {
	fmt.Fprintf(w, "\n最近一次性能报告 (%s, %s):\n",
		report.Command, report.Timestamp.Local().Format("2006-01-02 15:04"))

	for _, p := range report.Phases {
		marker := ""
		if p.Name == report.Slowest {
			marker = "  ← 最慢"
		}
		fmt.Fprintf(w, "  %-25s %dms%s\n", p.Name, p.DurationMs, marker)
	}
	fmt.Fprintf(w, "  %-25s ─────────\n", "─────────────────────────")
	fmt.Fprintf(w, "  %-25s %dms  (框架开销 %dms)\n", "总耗时", report.TotalMs, report.OverheadMs)
}
