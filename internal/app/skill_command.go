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
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_SKILL_API_HOST",
		Category:     configmeta.CategoryNetwork,
		Description:  "覆盖 Skill API 地址",
		DefaultValue: "https://pre-mcp.dingtalk.com",
		Example:      "https://custom-mcp.example.com",
	})
}

const (
	// legacySkillAPIHost is the legacy skill market host used by the old cli.
	legacySkillAPIHost = "https://pre-mcp.dingtalk.com"
	// skillDownloadEndpoint is the API endpoint for downloading skills.
	skillDownloadEndpoint = "https://aihub.dingtalk.com/cli/download"
	// skillDownloadTimeout is the timeout for skill download operations.
	skillDownloadTimeout = 5 * time.Minute
)

// downloadSkillResponse represents the API response for skill download.
type downloadSkillResponse struct {
	Success   bool                 `json:"success"`
	ErrorCode string               `json:"errorCode,omitempty"`
	ErrorMsg  string               `json:"errorMsg,omitempty"`
	Result    *downloadSkillResult `json:"result,omitempty"`
}

// downloadSkillResult contains the download URL and file name.
type downloadSkillResult struct {
	DownloadURL string `json:"downloadUrl"`
	FileName    string `json:"fileName"`
}

// findSkillsResponse represents the legacy skill search API response.
type findSkillsResponse struct {
	Success   bool          `json:"success"`
	ErrorCode string        `json:"errorCode,omitempty"`
	ErrorMsg  string        `json:"errorMsg,omitempty"`
	Result    []CliSkillDTO `json:"result,omitempty"`
}

// CliSkillDTO mirrors the old cli response payload for `skill search`.
type CliSkillDTO struct {
	SkillID string `json:"skillId"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Icon    string `json:"icon"`
}

// agentSkillPaths maps target names to their relative skill installation paths.
// These paths are relative to the user's home directory.
var agentSkillPaths = map[string]string{
	"qoder":    ".qoder/skills",
	"claude":   ".claude/skills",
	"cursor":   ".cursor/skills",
	"codex":    ".codex/skills",
	"opencode": filepath.Join(".config", "opencode", "skills"),
}

// supportedTargets returns a comma-separated list of supported targets.
func supportedTargets() string {
	targets := make([]string, 0, len(agentSkillPaths)+1)
	for target := range agentSkillPaths {
		targets = append(targets, target)
	}
	targets = append(targets, ".")
	return strings.Join(targets, ", ")
}

func buildSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "skill",
		Short:             "技能管理",
		Long:              "管理钉钉技能市场的技能。支持搜索、下载与安装到指定 Agent 目录。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newSkillInstallCommand(),
		newSkillGetCommand(),
		newSkillSearchCommand(),
		newSkillFindHintCommand(),
		newSkillAddHintCommand(),
	)
	return cmd
}

func newSkillGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "获取技能压缩文件",
		Long:              "从服务端下载技能包到本地临时目录。命令执行成功后会输出临时目录路径，供调用方使用。",
		Example:           "  dws skill get --skill-id <skillId>",
		DisableAutoGenTag: true,
		RunE:              runSkillGet,
	}
	cmd.Flags().String("skill-id", "", "技能 ID（必填）")
	_ = cmd.MarkFlagRequired("skill-id")
	return cmd
}

func newSkillSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "从钉钉技能市场搜索技能",
		Long:              "从钉钉技能市场搜索技能，根据关键词返回匹配的技能列表。",
		Example:           "  dws skill search --query 关键词",
		DisableAutoGenTag: true,
		RunE:              runSkillFind,
	}
	cmd.Flags().String("query", "", "搜索关键词（必填）")
	_ = cmd.MarkFlagRequired("query")
	cmd.Flags().String("scopes", "", "查询范围，空格分隔。备选值：DingtalkMarket（钉钉市场）、OrgInternal（企业内部）。为空默认查市场技能")
	return cmd
}

func newSkillFindHintCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "find",
		Short:             "兼容旧用法，提示使用 skill search",
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "use: dws skill search --query <关键词>")
			return nil
		},
	}
}

func newSkillInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <skillId> <target>",
		Short: "下载并安装技能到指定目录",
		Long: fmt.Sprintf(`从钉钉技能市场下载技能并安装到指定 Agent 目录。

参数:
  skillId   技能 ID（必填），可从钉钉技能市场获取
  target    安装目标（必填），支持: %s

安装路径:
  qoder    -> ~/.qoder/skills/
  claude   -> ~/.claude/skills/
  cursor   -> ~/.cursor/skills/
  codex    -> ~/.codex/skills/
  opencode -> ~/.config/opencode/skills/
  .        -> 当前目录

示例:
  dws skill install skill-123 qoder     # 安装到 ~/.qoder/skills/
  dws skill install skill-123 claude    # 安装到 ~/.claude/skills/
  dws skill install skill-123 .         # 安装到当前目录`, supportedTargets()),
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE:              runSkillAdd,
	}

	return cmd
}

func newSkillAddHintCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "add",
		Short:             "兼容旧用法，提示使用 skill install",
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "use: dws skill install <skillId> <target>")
			return nil
		},
	}
}

func runSkillGet(cmd *cobra.Command, args []string) error {
	skillID, _ := cmd.Flags().GetString("skill-id")
	accessToken, err := loadSkillAccessToken()
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/cli/install?skillId=%s", skillAPIHost(), url.QueryEscape(strings.TrimSpace(skillID)))
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "⬇️  下载技能包...")

	tmpDir, err := downloadSkillToTmpDir(cmd.Context(), apiURL, accessToken)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), tmpDir)
	return nil
}

func runSkillFind(cmd *cobra.Command, args []string) error {
	keyword, _ := cmd.Flags().GetString("query")
	scopes, _ := cmd.Flags().GetString("scopes")
	accessToken, err := loadSkillAccessToken()
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/cli/find-skills?keyword=%s", skillAPIHost(), url.QueryEscape(strings.TrimSpace(keyword)))
	if scopes != "" {
		apiURL += "&scopes=" + url.QueryEscape(scopes)
	}
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, apiURL, nil)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create request: %v", err))
	}
	req.Header.Set("x-user-access-token", accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return apperrors.NewAPI(fmt.Sprintf("failed to search skills: %v", err), apperrors.WithRetryable(true))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return parseLegacySkillAPIError(resp)
	}

	var result findSkillsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return apperrors.NewAPI(fmt.Sprintf("failed to parse search response: %v", err))
	}
	if !result.Success {
		errMsg := strings.TrimSpace(result.ErrorMsg)
		if errMsg == "" {
			errMsg = strings.TrimSpace(result.ErrorCode)
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return apperrors.NewAPI(fmt.Sprintf("failed to search skills: %s", errMsg))
	}

	if len(result.Result) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "未找到匹配的技能")
		return nil
	}

	for _, skill := range result.Result {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "SkillID: %s\n", skill.SkillID)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", skill.Name)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Desc: %s\n", skill.Desc)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "---")
	}
	return nil
}

func runSkillAdd(cmd *cobra.Command, args []string) error {
	skillID := strings.TrimSpace(args[0])
	target := strings.TrimSpace(args[1])

	if skillID == "" {
		return apperrors.NewValidation("skillId is required")
	}

	// Resolve target path
	destPath, err := resolveSkillTargetPath(target)
	if err != nil {
		return apperrors.NewValidation(fmt.Sprintf("invalid target '%s': %v. Supported targets: %s", target, err, supportedTargets()))
	}

	accessToken, err := loadSkillAccessToken()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), skillDownloadTimeout)
	defer cancel()

	w := cmd.OutOrStdout()

	// Step 1: Get download URL from API
	fmt.Fprintf(w, "正在获取技能信息...\n")
	downloadResp, err := fetchSkillDownloadInfo(ctx, accessToken, skillID)
	if err != nil {
		return err
	}

	if !downloadResp.Success {
		errMsg := downloadResp.ErrorMsg
		if errMsg == "" {
			errMsg = downloadResp.ErrorCode
		}
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return apperrors.NewAPI(fmt.Sprintf("failed to get skill download info: %s", errMsg),
			apperrors.WithReason(downloadResp.ErrorCode))
	}

	if downloadResp.Result == nil || downloadResp.Result.DownloadURL == "" {
		return apperrors.NewAPI("skill download URL not found in response")
	}

	// Step 2: Download the skill zip file
	fmt.Fprintf(w, "正在下载技能...\n")
	tempZipPath, err := downloadSkillFile(ctx, downloadResp.Result.DownloadURL, downloadResp.Result.FileName)
	if err != nil {
		return err
	}
	defer cleanupTempFile(tempZipPath)

	// Step 3: Extract zip to destination
	fmt.Fprintf(w, "正在解压到 %s...\n", destPath)
	if err := extractSkillZip(tempZipPath, destPath); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n[OK] 技能安装成功！\n")
	fmt.Fprintf(w, "安装路径: %s\n", destPath)

	return nil
}

func loadSkillAccessToken() (string, error) {
	configDir := defaultConfigDir()
	tokenData, err := authpkg.LoadTokenData(configDir)
	if err != nil || tokenData == nil || !tokenData.IsAccessTokenValid() {
		return "", skillAuthError()
	}
	return tokenData.AccessToken, nil
}

func skillAuthError() error {
	if edition.Get().IsEmbedded {
		return apperrors.NewAuth("认证信息已失效",
			apperrors.WithReason("not_authenticated"),
			apperrors.WithHint("请先完成钉钉账号登录后重试"))
	}
	return apperrors.NewAuth("not logged in or token expired. Please run 'dws auth login' first",
		apperrors.WithHint("请先执行 'dws auth login' 登录"),
		apperrors.WithActions("dws auth login"))
}

func skillAPIHost() string {
	if override := strings.TrimSpace(os.Getenv("DWS_SKILL_API_HOST")); override != "" {
		return strings.TrimRight(override, "/")
	}
	return legacySkillAPIHost
}

// resolveSkillTargetPath resolves the target argument to an absolute path.
func resolveSkillTargetPath(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	// Special case: current directory
	if target == "." {
		return os.Getwd()
	}

	// Look up predefined agent paths
	relPath, ok := agentSkillPaths[strings.ToLower(target)]
	if !ok {
		return "", fmt.Errorf("unsupported target")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, relPath), nil
}

// fetchSkillDownloadInfo calls the download API to get the skill download URL.
func fetchSkillDownloadInfo(ctx context.Context, accessToken, skillID string) (*downloadSkillResponse, error) {
	url := fmt.Sprintf("%s?skillId=%s", skillDownloadEndpoint, skillID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("failed to create request: %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-user-access-token", accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apperrors.NewAPI(fmt.Sprintf("failed to call download API: %v", err),
			apperrors.WithRetryable(true))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, skillAuthError()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, apperrors.NewAPI(fmt.Sprintf("download API returned HTTP %d", resp.StatusCode),
			apperrors.WithRetryable(resp.StatusCode >= 500))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, apperrors.NewAPI(fmt.Sprintf("failed to read response: %v", err))
	}

	var result downloadSkillResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, apperrors.NewAPI(fmt.Sprintf("failed to parse response: %v", err))
	}

	return &result, nil
}

func downloadSkillToTmpDir(ctx context.Context, apiURL, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to create request: %v", err))
	}
	req.Header.Set("x-user-access-token", accessToken)

	client := &http.Client{Timeout: skillDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", apperrors.NewAPI(fmt.Sprintf("failed to download skill package: %v", err), apperrors.WithRetryable(true))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", parseLegacySkillAPIError(resp)
	}

	tmpDir, err := os.MkdirTemp("", "dws-skill-*")
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to create temp dir: %v", err))
	}

	filename := filenameFromDisposition(resp.Header.Get("Content-Disposition"))
	destPath := filepath.Join(tmpDir, filename)
	file, err := os.Create(destPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", apperrors.NewInternal(fmt.Sprintf("failed to create temp file: %v", err))
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		os.RemoveAll(tmpDir)
		return "", apperrors.NewAPI(fmt.Sprintf("failed to save downloaded file: %v", err))
	}
	return tmpDir, nil
}

func filenameFromDisposition(cd string) string {
	if cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if name := strings.TrimSpace(params["filename"]); name != "" {
				return name
			}
		}
	}
	return "skill.zip"
}

func parseLegacySkillAPIError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return skillAuthError()
	case http.StatusBadRequest:
		return apperrors.NewValidation("request parameters are invalid")
	case http.StatusNotFound:
		return apperrors.NewValidation("skill does not exist or corresponding file was not found")
	default:
		return apperrors.NewAPI(fmt.Sprintf("skill API returned HTTP %d", resp.StatusCode),
			apperrors.WithRetryable(resp.StatusCode >= 500))
	}
}

// downloadSkillFile downloads the skill zip file to a temporary location.
func downloadSkillFile(ctx context.Context, downloadURL, fileName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to create download request: %v", err))
	}

	client := &http.Client{Timeout: skillDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", apperrors.NewAPI(fmt.Sprintf("failed to download skill: %v", err),
			apperrors.WithRetryable(true))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperrors.NewAPI(fmt.Sprintf("download returned HTTP %d", resp.StatusCode),
			apperrors.WithRetryable(resp.StatusCode >= 500))
	}

	// Create temp file
	if fileName == "" {
		fileName = "skill.zip"
	}
	tempFile, err := os.CreateTemp("", "dws-skill-*.zip")
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to create temp file: %v", err))
	}
	tempPath := tempFile.Name()

	// Copy response body to temp file
	_, err = io.Copy(tempFile, resp.Body)
	closeErr := tempFile.Close()
	if err != nil {
		os.Remove(tempPath)
		return "", apperrors.NewAPI(fmt.Sprintf("failed to save downloaded file: %v", err))
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return "", apperrors.NewInternal(fmt.Sprintf("failed to close temp file: %v", closeErr))
	}

	return tempPath, nil
}

// extractSkillZip extracts a zip file to the destination directory.
func extractSkillZip(zipPath, destDir string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create destination directory: %v", err))
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to open zip file: %v", err))
	}
	defer reader.Close()

	for _, file := range reader.File {
		if err := extractZipFile(file, destDir); err != nil {
			return err
		}
	}

	return nil
}

// extractZipFile extracts a single file from the zip archive.
func extractZipFile(file *zip.File, destDir string) error {
	// Sanitize file path to prevent zip slip attacks
	filePath := filepath.Join(destDir, file.Name)
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(destDir)+string(os.PathSeparator)) {
		return apperrors.NewValidation(fmt.Sprintf("invalid file path in zip: %s", file.Name))
	}

	if file.FileInfo().IsDir() {
		// Use 0755 to ensure we have write permission for creating files inside
		return os.MkdirAll(filePath, 0755)
	}

	// Ensure parent directory exists with write permission
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create directory: %v", err))
	}

	// Extract file
	srcFile, err := file.Open()
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to open file in zip: %v", err))
	}
	defer srcFile.Close()

	// Use file mode from zip but ensure at least 0644 for files
	fileMode := file.Mode()
	if fileMode&0600 == 0 {
		fileMode = 0644
	}
	destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create file: %v", err))
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to extract file: %v", err))
	}

	return nil
}

// cleanupTempFile removes a temporary file, ignoring errors.
func cleanupTempFile(path string) {
	if path != "" {
		os.Remove(path)
	}
}
