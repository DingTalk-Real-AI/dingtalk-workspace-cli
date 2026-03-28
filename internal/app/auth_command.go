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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/config"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/spf13/cobra"
)

type authLoginConfig struct {
	Token  string
	Force  bool
	Device bool
}

func buildAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "auth",
		Short:             "认证管理",
		Long:              "管理钉钉 CLI 的认证凭证。支持 OAuth 扫码登录和 Device Flow。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAuthLoginCommand())
	cmd.AddCommand(
		newAuthLogoutCommand(),
		newAuthStatusCommand(),
		newAuthExchangeCommand(),
		newAuthResetCommand(),
	)
	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "登录钉钉（自动刷新 token，必要时扫码）",
		Long: `登录钉钉并获取认证凭证。

支持的登录方式:
  - OAuth 设备流 (默认): 通过钉钉扫码授权登录
  - 直接提供 Token: 通过 --token 参数传入已有 token

不支持的登录方式:
  - 邮箱/密码登录
  - 手机号/验证码登录
  - 应用凭证 (AppKey/AppSecret) 直接登录

示例:
  dws auth login              # 扫码登录
  dws auth login --force      # 强制重新登录 (忽略缓存 token)
  dws auth login --token xxx  # 使用指定 token`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveAuthLoginConfig(cmd)
			if err != nil {
				return err
			}
			configDir := defaultConfigDir()
			var tokenData *authpkg.TokenData

			switch {
			case strings.TrimSpace(cfg.Token) != "":
				tokenData = &authpkg.TokenData{
					AccessToken: cfg.Token,
					ExpiresAt:   time.Now().Add(config.ManualTokenExpiry),
				}
				if err := authpkg.SaveTokenData(configDir, tokenData); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to persist auth token: %v", err))
				}
			case cfg.Device:
				loginCtx, cancel := context.WithTimeout(cmd.Context(), config.DeviceFlowTimeout)
				defer cancel()

				provider := authpkg.NewDeviceFlowProvider(configDir, nil)
				provider.Output = cmd.ErrOrStderr()
				tokenData, err = provider.Login(loginCtx)
				if err != nil {
					return apperrors.NewAuth(fmt.Sprintf("device authorization failed: %v", err))
				}
			default:
				loginCtx, cancel := context.WithTimeout(cmd.Context(), config.OAuthFlowTimeout)
				defer cancel()

				provider := authpkg.NewOAuthProvider(configDir, nil)
				provider.Output = cmd.ErrOrStderr()
				configureOAuthProviderCompatibility(provider, configDir)
				tokenData, err = provider.Login(loginCtx, cfg.Force)
				if err != nil {
					return apperrors.NewAuth(fmt.Sprintf("dingtalk login failed: %v", err))
				}
			}

			clearCompatCache()

			w := cmd.OutOrStdout()

			// Check if JSON output is requested
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return writeAuthLoginJSON(w, tokenData, cfg.Force)
			}

			// Default table output
			fmt.Fprintln(w)
			if !cfg.Device && tokenData != nil && tokenData.IsAccessTokenValid() && !cfg.Force {
				fmt.Fprintf(w, "[OK] Token 有效，无需重新登录\n")
			} else {
				fmt.Fprintf(w, "[OK] 登录成功！\n")
			}
			if tokenData != nil {
				if tokenData.CorpName != "" {
					fmt.Fprintf(w, "%-16s%s\n", "企业:", tokenData.CorpName)
				}
				if tokenData.CorpID != "" {
					fmt.Fprintf(w, "%-16s%s\n", "企业 ID:", tokenData.CorpID)
				}
				if tokenData.UserName != "" {
					fmt.Fprintf(w, "%-16s%s\n", "用户:", tokenData.UserName)
				}
				if expiry := authLoginDisplayExpiry(tokenData); expiry != "" {
					fmt.Fprintf(w, "%-16s%s\n", "有效期:", expiry)
				}
			}
			fmt.Fprintf(w, "Token 将自动刷新，无需重复登录\n")
			return nil
		},
	}
	cmd.Flags().String("token", "", "Access token")
	cmd.Flags().Bool("device", false, "Use device authorization flow")
	cmd.Flags().Bool("force", false, "Force interactive login (ignore cached token)")
	// Hidden compatibility flags
	cmd.Flags().String("redirect-url", "", "Loopback redirect URL")
	cmd.Flags().String("scopes", "", "Space-separated DingTalk OAuth scopes")
	cmd.Flags().String("authorize-url", "", "Override DingTalk authorization URL")
	cmd.Flags().String("token-url", "", "Override DingTalk token exchange URL")
	cmd.Flags().String("refresh-url", "", "Override DingTalk refresh token URL")
	cmd.Flags().Int("login-timeout", 0, "Login timeout seconds")
	cmd.Flags().Bool("no-browser", false, "Suppress browser launch")
	_ = cmd.Flags().MarkHidden("redirect-url")
	_ = cmd.Flags().MarkHidden("scopes")
	_ = cmd.Flags().MarkHidden("authorize-url")
	_ = cmd.Flags().MarkHidden("token-url")
	_ = cmd.Flags().MarkHidden("refresh-url")
	_ = cmd.Flags().MarkHidden("login-timeout")
	_ = cmd.Flags().MarkHidden("no-browser")
	return cmd
}

func newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "logout",
		Short:             "清除认证信息",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			revokeCtx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			_ = authpkg.RevokeTokenRemote(revokeCtx)

			if err := authpkg.DeleteTokenData(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to clear token data: %v", err))
			}
			_ = os.Remove(filepath.Join(configDir, "mcp_url"))
			_ = os.Remove(filepath.Join(configDir, "token"))
			clearCompatCache()
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 已清除所有认证信息")
			fmt.Fprintln(w, "请运行 dws auth login 重新登录")
			return nil
		},
	}
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "status",
		Short:             "查看认证状态",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()

			authenticated := false
			refreshed := false
			var tokenData *authpkg.TokenData
			provider := authpkg.NewOAuthProvider(configDir, nil)
			configureOAuthProviderCompatibility(provider, configDir)
			if data, err := provider.Status(); err == nil {
				tokenData = data
				if !data.IsAccessTokenValid() && data.IsRefreshTokenValid() {
					refreshCtx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
					_, refreshErr := provider.GetAccessToken(refreshCtx)
					cancel()
					if refreshErr == nil {
						if updatedData, statusErr := provider.Status(); statusErr == nil {
							tokenData = updatedData
							refreshed = true
						}
					}
				}
				if authStatusAuthenticated(tokenData) {
					authenticated = true
				}
			}

			// Check if JSON output is requested
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return writeAuthStatusJSON(cmd.OutOrStdout(), authenticated, refreshed, tokenData)
			}

			// Default table output
			w := cmd.OutOrStdout()
			if authenticated {
				if refreshed {
					fmt.Fprintf(w, "%-16s%s\n", "状态:", "已登录 ✅")
					fmt.Fprintln(w, "Token 已自动刷新")
				} else {
					fmt.Fprintf(w, "%-16s%s\n", "状态:", "已登录 ✅")
				}
				if updatedAt := authStatusUpdatedAt(tokenData); updatedAt != "" {
					fmt.Fprintf(w, "%-16s%s\n", "有效期:", updatedAt)
				}
			} else {
				fmt.Fprintf(w, "%-16s%s\n", "状态:", "未登录")
				fmt.Fprintln(w, "运行 dws auth login 进行登录")
			}
			return nil
		},
	}
}

func newAuthExchangeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "exchange",
		Short:             "Exchange an authorization code for credentials",
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			code, err := cmd.Flags().GetString("code")
			if err != nil {
				return apperrors.NewInternal("failed to read --code")
			}
			code = strings.TrimSpace(code)
			if code == "" {
				return apperrors.NewValidation("--code is required")
			}
			uid, err := cmd.Flags().GetString("uid")
			if err != nil {
				return apperrors.NewInternal("failed to read --uid")
			}

			configDir := defaultConfigDir()
			provider := authpkg.NewOAuthProvider(configDir, nil)
			configureOAuthProviderCompatibility(provider, configDir)
			exchangeCtx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			tokenData, err := provider.ExchangeAuthCode(exchangeCtx, code, strings.TrimSpace(uid))
			if err != nil {
				return apperrors.NewAuth(fmt.Sprintf("failed to exchange authorization code: %v", err))
			}
			clearCompatCache()

			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 授权码兑换成功！")
			if strings.TrimSpace(uid) != "" {
				fmt.Fprintf(w, "%-16s%s\n", "用户:", strings.TrimSpace(uid))
			}
			if strings.TrimSpace(tokenData.CorpID) != "" {
				fmt.Fprintf(w, "%-16s%s\n", "企业 ID:", tokenData.CorpID)
			}
			if !tokenData.ExpiresAt.IsZero() {
				fmt.Fprintf(w, "%-16s%s\n", "有效期:", authLoginFormatExpiry(tokenData.ExpiresAt))
			}
			return nil
		},
	}
	cmd.Flags().String("code", "", "Authorization code")
	cmd.Flags().String("uid", "", "Optional user identifier for compatibility")
	cmd.Flags().String("client-id", "", "Compatibility flag")
	cmd.Flags().String("authorize-url", "", "Compatibility flag")
	cmd.Flags().String("token-url", "", "Compatibility flag")
	cmd.Flags().String("refresh-url", "", "Compatibility flag")
	cmd.Flags().String("redirect-url", "", "Compatibility flag")
	cmd.Flags().String("scopes", "", "Compatibility flag")
	return cmd
}

func newAuthResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "reset",
		Short:             "重置认证信息（清除本地 Token，触发重新授权）",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			if err := authpkg.DeleteTokenData(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to reset token data: %v", err))
			}
			_ = os.Remove(filepath.Join(configDir, "mcp_url"))
			_ = os.Remove(filepath.Join(configDir, "token"))
			clearCompatCache()
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 认证信息已重置")
			fmt.Fprintln(w, "请运行 dws auth login 重新登录")
			return nil
		},
	}
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func authLoginFormatExpiry(t time.Time) string {
	remaining := time.Until(t)
	if remaining <= 0 {
		return "已过期"
	}
	if remaining > 24*time.Hour {
		return fmt.Sprintf("%.0f 天后", remaining.Hours()/24)
	}
	return fmt.Sprintf("%.0f 小时后", remaining.Hours())
}

// authLoginDisplayExpiry 返回用于显示的有效期（优先显示 refresh token 有效期）
func authLoginDisplayExpiry(data *authpkg.TokenData) string {
	if data == nil {
		return ""
	}
	// 优先使用 refresh token 有效期（更长，对用户更有意义）
	if data.IsRefreshTokenValid() {
		return authLoginFormatExpiry(data.RefreshExpAt)
	}
	// 回退到 access token 有效期
	if !data.ExpiresAt.IsZero() {
		return authLoginFormatExpiry(data.ExpiresAt)
	}
	return ""
}

func clearCompatCache() {
	store := cacheStoreFromEnv()
	if store != nil {
		_ = os.RemoveAll(store.Root)
	}
}

func resolveAuthLoginConfig(cmd *cobra.Command) (authLoginConfig, error) {
	token, err := cmd.Flags().GetString("token")
	if err != nil {
		return authLoginConfig{}, apperrors.NewInternal("failed to read --token")
	}
	device, err := cmd.Flags().GetBool("device")
	if err != nil {
		return authLoginConfig{}, apperrors.NewInternal("failed to read --device")
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return authLoginConfig{}, apperrors.NewInternal("failed to read --force")
	}
	return authLoginConfig{
		Token:  strings.TrimSpace(token),
		Force:  force,
		Device: device,
	}, nil
}

func authStatusAuthenticated(data *authpkg.TokenData) bool {
	if data == nil {
		return false
	}
	return data.IsAccessTokenValid() || data.IsRefreshTokenValid()
}

func authStatusUpdatedAt(data *authpkg.TokenData) string {
	if data == nil {
		return ""
	}
	if data.IsAccessTokenValid() {
		return timeOrEmpty(data.ExpiresAt)
	}
	if data.IsRefreshTokenValid() {
		return timeOrEmpty(data.RefreshExpAt)
	}
	return ""
}

// authStatusResponse is the JSON response for auth status command.
type authStatusResponse struct {
	Success           bool   `json:"success"`
	Authenticated     bool   `json:"authenticated"`
	Message           string `json:"message,omitempty"`
	Refreshed         bool   `json:"refreshed,omitempty"`
	TokenValid        bool   `json:"token_valid,omitempty"`
	RefreshTokenValid bool   `json:"refresh_token_valid,omitempty"`
	ExpiresAt         string `json:"expires_at,omitempty"`
	RefreshExpiresAt  string `json:"refresh_expires_at,omitempty"`
	CorpID            string `json:"corp_id,omitempty"`
	CorpName          string `json:"corp_name,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	UserName          string `json:"user_name,omitempty"`
}

func writeAuthStatusJSON(w io.Writer, authenticated, refreshed bool, data *authpkg.TokenData) error {
	resp := authStatusResponse{
		Success:       true,
		Authenticated: authenticated,
	}

	if !authenticated {
		resp.Message = "未登录"
	} else if data != nil {
		resp.Refreshed = refreshed
		resp.TokenValid = data.IsAccessTokenValid()
		resp.RefreshTokenValid = data.IsRefreshTokenValid()
		if !data.ExpiresAt.IsZero() {
			resp.ExpiresAt = data.ExpiresAt.Format(time.RFC3339Nano)
		}
		if !data.RefreshExpAt.IsZero() {
			resp.RefreshExpiresAt = data.RefreshExpAt.Format(time.RFC3339Nano)
		}
		resp.CorpID = data.CorpID
		resp.CorpName = data.CorpName
		resp.UserID = data.UserID
		resp.UserName = data.UserName
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// authLoginResponse is the JSON response for auth login command.
type authLoginResponse struct {
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	TokenValid        bool   `json:"token_valid,omitempty"`
	RefreshTokenValid bool   `json:"refresh_token_valid,omitempty"`
	ExpiresAt         string `json:"expires_at,omitempty"`
	RefreshExpiresAt  string `json:"refresh_expires_at,omitempty"`
	CorpID            string `json:"corp_id,omitempty"`
	CorpName          string `json:"corp_name,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	UserName          string `json:"user_name,omitempty"`
}

func writeAuthLoginJSON(w io.Writer, data *authpkg.TokenData, forced bool) error {
	resp := authLoginResponse{
		Success: true,
		Message: "登录成功",
	}

	if data != nil {
		if data.IsAccessTokenValid() && !forced {
			resp.Message = "Token 有效，无需重新登录"
		}
		resp.TokenValid = data.IsAccessTokenValid()
		resp.RefreshTokenValid = data.IsRefreshTokenValid()
		if !data.ExpiresAt.IsZero() {
			resp.ExpiresAt = data.ExpiresAt.Format(time.RFC3339Nano)
		}
		if !data.RefreshExpAt.IsZero() {
			resp.RefreshExpiresAt = data.RefreshExpAt.Format(time.RFC3339Nano)
		}
		resp.CorpID = data.CorpID
		resp.CorpName = data.CorpName
		resp.UserID = data.UserID
		resp.UserName = data.UserName
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
