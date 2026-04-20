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

package pat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

const (
	callbackRequestTimeout = 30 * time.Second
	callbackExitCode       = 1
)

type callbackEnvelope struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    any    `json:"data,omitempty"`
}

type callbackError struct {
	exitCode int
	payload  callbackEnvelope
}

type callbackSharedOptions struct {
	AccessToken   string
	ConfigDir     string
	AuthRequestID string
}

func (e *callbackError) Error() string {
	return callbackJSON(e.payload)
}

func (e *callbackError) ExitCode() int {
	if e == nil || e.exitCode == 0 {
		return callbackExitCode
	}
	return e.exitCode
}

func (e *callbackError) RawStderr() string {
	return callbackJSON(e.payload)
}

var callbackCmd = &cobra.Command{
	Use:   "callback",
	Short: "PAT 宿主 / Agent 回调接口",
	Long: `面向宿主 / Agent 的 PAT 机器接口。

这些命令复用 CLI 已有的 PAT/CLI 授权接口，供宿主在拿到 PAT JSON
事件后继续完成“查主管理员 / 发送申请 / 轮询流程”等动作，而不是直接
调用 DingTalk API。

宿主接管模式由 CLAW_TYPE 主导，支持值：
  host-control
  rewind-desktop
  dws-wukong
  wukong

DWS_CHANNEL 只保留为上游 channelCode；历史上的
  DWS_CHANNEL='...;host-control'
仅作为兼容路径保留在文档中。`,
	Example: `  dws pat callback list-super-admins --auth-request-id req-001
  dws pat callback send-apply --admin-staff-id manager123 --auth-request-id req-001
  dws pat callback poll-flow --flow-id flow-001 --auth-request-id req-001

  # 显式指定 access token / config-dir（宿主通常二选一）
  dws pat callback list-super-admins --access-token <token>
  dws pat callback poll-flow --config-dir ~/.dws --flow-id flow-001`,
	RunE: cmdutil.GroupRunE,
}

var listSuperAdminsCmd = &cobra.Command{
	Use:   "list-super-admins",
	Short: "列出组织主管理员",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := loadCallbackSharedOptions(cmd)
		if caller != nil && caller.DryRun() {
			return writeCallbackEnvelope(cmd, callbackEnvelope{
				Success: true,
				Code:    "PAT_CALLBACK_LIST_SUPER_ADMINS_DRY_RUN",
				Data: map[string]any{
					"authRequestId": opts.AuthRequestID,
					"dryRun":        true,
				},
			})
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), callbackRequestTimeout)
		defer cancel()

		accessToken, err := resolveCallbackAccessToken(ctx, opts.ConfigDir, opts.AccessToken)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_AUTH_REQUIRED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"hint":          "provide --access-token or run dws auth login first",
				"reason":        err.Error(),
			})
		}

		resp, err := listSuperAdmins(ctx, opts.ConfigDir, accessToken)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_LIST_SUPER_ADMINS_FAILED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"reason":        err.Error(),
			})
		}
		if !resp.Success {
			return newCallbackError("PAT_CALLBACK_LIST_SUPER_ADMINS_FAILED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"errorCode":     resp.ErrorCode,
				"errorMsg":      resp.ErrorMsg,
			})
		}

		return writeCallbackEnvelope(cmd, callbackEnvelope{
			Success: true,
			Code:    "PAT_CALLBACK_LIST_SUPER_ADMINS",
			Data: map[string]any{
				"authRequestId": opts.AuthRequestID,
				"superAdmins":   resp.Result,
				"count":         len(resp.Result),
			},
		})
	},
}

var sendApplyCmd = &cobra.Command{
	Use:   "send-apply",
	Short: "向主管理员发送 CLI 开通申请",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := loadCallbackSharedOptions(cmd)
		adminStaffID, _ := cmd.Flags().GetString("admin-staff-id")
		if strings.TrimSpace(adminStaffID) == "" {
			return fmt.Errorf("flag --admin-staff-id is required")
		}
		if caller != nil && caller.DryRun() {
			return writeCallbackEnvelope(cmd, callbackEnvelope{
				Success: true,
				Code:    "PAT_CALLBACK_SEND_APPLY_DRY_RUN",
				Data: map[string]any{
					"authRequestId": opts.AuthRequestID,
					"adminStaffId":  adminStaffID,
					"dryRun":        true,
				},
			})
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), callbackRequestTimeout)
		defer cancel()

		accessToken, err := resolveCallbackAccessToken(ctx, opts.ConfigDir, opts.AccessToken)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_AUTH_REQUIRED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"adminStaffId":  adminStaffID,
				"hint":          "provide --access-token or run dws auth login first",
				"reason":        err.Error(),
			})
		}

		resp, err := sendApply(ctx, opts.ConfigDir, accessToken, adminStaffID)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_SEND_APPLY_FAILED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"adminStaffId":  adminStaffID,
				"reason":        err.Error(),
			})
		}
		if !resp.Success || !resp.Result {
			return newCallbackError("PAT_CALLBACK_SEND_APPLY_FAILED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"adminStaffId":  adminStaffID,
				"errorCode":     resp.ErrorCode,
				"errorMsg":      resp.ErrorMsg,
			})
		}

		return writeCallbackEnvelope(cmd, callbackEnvelope{
			Success: true,
			Code:    "PAT_CALLBACK_SEND_APPLY",
			Data: map[string]any{
				"authRequestId": opts.AuthRequestID,
				"adminStaffId":  adminStaffID,
				"applied":       true,
			},
		})
	},
}

var pollFlowCmd = &cobra.Command{
	Use:   "poll-flow",
	Short: "轮询一次 PAT 流程状态",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := loadCallbackSharedOptions(cmd)
		flowID, _ := cmd.Flags().GetString("flow-id")
		if strings.TrimSpace(flowID) == "" {
			return fmt.Errorf("flag --flow-id is required")
		}
		if caller != nil && caller.DryRun() {
			return writeCallbackEnvelope(cmd, callbackEnvelope{
				Success: true,
				Code:    "PAT_CALLBACK_POLL_FLOW_DRY_RUN",
				Data: map[string]any{
					"authRequestId": opts.AuthRequestID,
					"flowId":        flowID,
					"dryRun":        true,
				},
			})
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), callbackRequestTimeout)
		defer cancel()

		accessToken, err := resolveOptionalCallbackAccessToken(ctx, opts.ConfigDir, opts.AccessToken)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_AUTH_REQUIRED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"flowId":        flowID,
				"hint":          "provide --access-token or run dws auth login first",
				"reason":        err.Error(),
			})
		}

		resp, err := pollFlow(ctx, opts.ConfigDir, accessToken, flowID)
		if err != nil {
			return newCallbackError("PAT_CALLBACK_POLL_FLOW_FAILED", map[string]any{
				"authRequestId": opts.AuthRequestID,
				"flowId":        flowID,
				"reason":        err.Error(),
			})
		}

		status := authpkg.ParseDeviceFlowStatus(resp.Data.Status, resp.Success)
		authCode := strings.TrimSpace(resp.Data.AuthCode)
		terminal := status == authpkg.StatusApproved || status == authpkg.StatusRejected || status == authpkg.StatusExpired || status == authpkg.StatusCancelled
		tokenUpdated := false
		if status == authpkg.StatusApproved && authCode != "" {
			if clientID := strings.TrimSpace(authpkg.ClientID()); clientID != "" && !authpkg.HasValidClientSecret() {
				authpkg.SetClientIDFromMCP(clientID)
			}
			tokenData, err := authpkg.ExchangeCodeForToken(ctx, opts.ConfigDir, authCode)
			if err != nil {
				return newCallbackError("PAT_CALLBACK_EXCHANGE_CODE_FAILED", map[string]any{
					"authRequestId": opts.AuthRequestID,
					"flowId":        flowID,
					"status":        status,
					"reason":        err.Error(),
				})
			}
			if err := authpkg.SaveTokenData(opts.ConfigDir, tokenData); err != nil {
				return newCallbackError("PAT_CALLBACK_SAVE_TOKEN_FAILED", map[string]any{
					"authRequestId": opts.AuthRequestID,
					"flowId":        flowID,
					"status":        status,
					"reason":        err.Error(),
				})
			}
			tokenUpdated = true
		}

		return writeCallbackEnvelope(cmd, callbackEnvelope{
			Success: true,
			Code:    "PAT_CALLBACK_POLL_FLOW",
			Data: map[string]any{
				"authRequestId":  opts.AuthRequestID,
				"flowId":         firstNonEmpty(resp.Data.FlowID, flowID),
				"status":         status,
				"authCode":       authCode,
				"approved":       status == authpkg.StatusApproved,
				"terminal":       terminal,
				"tokenUpdated":   tokenUpdated,
				"retrySuggested": status == authpkg.StatusApproved && tokenUpdated,
			},
		})
	},
}

func init() {
	addCallbackSharedFlags(listSuperAdminsCmd)
	addCallbackSharedFlags(sendApplyCmd)
	addCallbackSharedFlags(pollFlowCmd)
	sendApplyCmd.Flags().String("admin-staff-id", "", "主管理员 staffId（必填）")
	_ = sendApplyCmd.MarkFlagRequired("admin-staff-id")
	pollFlowCmd.Flags().String("flow-id", "", "PAT flowId（必填）")
	_ = pollFlowCmd.MarkFlagRequired("flow-id")
	callbackCmd.AddCommand(listSuperAdminsCmd)
	callbackCmd.AddCommand(sendApplyCmd)
	callbackCmd.AddCommand(pollFlowCmd)
}

func addCallbackSharedFlags(cmd *cobra.Command) {
	cmd.Flags().String("access-token", "", "显式传入的用户 access token（优先级高于本地登录态）")
	cmd.Flags().String("config-dir", "", "DWS 配置目录；默认读取 DWS_CONFIG_DIR 或 ~/.dws")
	cmd.Flags().String("auth-request-id", "", "宿主透传的 authRequestId，用于结果关联")
}

func loadCallbackSharedOptions(cmd *cobra.Command) callbackSharedOptions {
	accessToken, _ := cmd.Flags().GetString("access-token")
	configDir, _ := cmd.Flags().GetString("config-dir")
	authRequestID, _ := cmd.Flags().GetString("auth-request-id")
	return callbackSharedOptions{
		AccessToken:   strings.TrimSpace(accessToken),
		ConfigDir:     normalizeConfigDir(configDir),
		AuthRequestID: strings.TrimSpace(authRequestID),
	}
}

func normalizeConfigDir(configDir string) string {
	if strings.TrimSpace(configDir) != "" {
		return configDir
	}
	return config.DefaultConfigDir()
}

func writeCallbackEnvelope(cmd *cobra.Command, payload callbackEnvelope) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func newCallbackError(code string, data any) error {
	return &callbackError{
		exitCode: callbackExitCode,
		payload: callbackEnvelope{
			Success: false,
			Code:    code,
			Data:    data,
		},
	}
}

func callbackJSON(payload callbackEnvelope) string {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return `{"success":false,"code":"PAT_CALLBACK_INTERNAL_ERROR"}`
	}
	return string(body)
}

func resolveOptionalCallbackAccessToken(ctx context.Context, configDir, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit, nil
	}
	token, err := resolveCallbackAccessToken(ctx, configDir, "")
	if err == nil {
		return token, nil
	}
	if isNoCredentialsError(err) {
		return "", nil
	}
	return "", err
}

func resolveCallbackAccessToken(ctx context.Context, configDir, explicit string) (string, error) {
	if token := strings.TrimSpace(explicit); token != "" {
		return token, nil
	}

	configDir = normalizeConfigDir(configDir)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := authpkg.NewOAuthProvider(configDir, logger)
	token, err := provider.GetAccessToken(ctx)
	if err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), nil
	}

	manager := authpkg.NewManager(configDir, nil)
	legacyToken, _, legacyErr := manager.GetToken()
	if legacyErr == nil && strings.TrimSpace(legacyToken) != "" {
		return strings.TrimSpace(legacyToken), nil
	}

	if err != nil {
		return "", err
	}
	if legacyErr != nil {
		return "", legacyErr
	}
	return "", errors.New("no credentials found")
}

func isNoCredentialsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no credentials found") ||
		strings.Contains(msg, "run: dws auth login") ||
		strings.Contains(msg, "not logged in") ||
		strings.Contains(msg, "未登录") ||
		strings.Contains(msg, "未找到认证信息")
}

func listSuperAdmins(ctx context.Context, configDir, accessToken string) (*authpkg.SuperAdminResponse, error) {
	var resp authpkg.SuperAdminResponse
	if err := doCallbackRequest(ctx, configDir, accessToken, authpkg.SuperAdminPath, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func sendApply(ctx context.Context, configDir, accessToken, adminStaffID string) (*authpkg.SendApplyResponse, error) {
	query := url.Values{}
	query.Set("adminStaffId", adminStaffID)

	var resp authpkg.SendApplyResponse
	if err := doCallbackRequest(ctx, configDir, accessToken, authpkg.SendCliAuthApplyPath, query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func pollFlow(ctx context.Context, configDir, accessToken, flowID string) (*authpkg.DevicePollResponse, error) {
	query := url.Values{}
	query.Set("flowId", flowID)

	var resp authpkg.DevicePollResponse
	if err := doCallbackRequest(ctx, configDir, accessToken, authpkg.DevicePollPath, query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func doCallbackRequest(ctx context.Context, configDir, accessToken, endpointPath string, query url.Values, dest any) error {
	baseURL := callbackMCPBaseURL(configDir)
	requestURL := strings.TrimRight(baseURL, "/") + endpointPath
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	client := &http.Client{
		Timeout: callbackRequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("x-user-access-token", token)
	}
	authpkg.ApplyChannelHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		return fmt.Errorf("upstream redirected request, likely missing or expired login state")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func callbackMCPBaseURL(configDir string) string {
	mcpURLPath := filepath.Join(normalizeConfigDir(configDir), "mcp_url")
	if data, err := os.ReadFile(mcpURLPath); err == nil {
		if mcpURL := strings.TrimSpace(string(data)); mcpURL != "" {
			return mcpURL
		}
	}
	return authpkg.DefaultMCPBaseURL
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
