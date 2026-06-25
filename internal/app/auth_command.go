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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type authLoginConfig struct {
	Token     string
	Force     bool
	Device    bool
	Recommend bool
	Yes       bool
}

type authLoginGuideAction string

const (
	authLoginGuideDirectCLI         authLoginGuideAction = "direct_cli"
	authLoginGuideConfigureAgentApp authLoginGuideAction = "configure_agent_app"
	authLoginGuideManualCredentials authLoginGuideAction = "manual_credentials"
)

var (
	authLoginBrandBlue = lipgloss.AdaptiveColor{Light: "#1677FF", Dark: "#69B1FF"}
	authLoginInk       = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#EAF2FF"}
	authLoginMuted     = lipgloss.AdaptiveColor{Light: "#667085", Dark: "#8A96A8"}
	authLoginLine      = lipgloss.AdaptiveColor{Light: "#D6E4FF", Dark: "#2F3B52"}
	authLoginDanger    = lipgloss.AdaptiveColor{Light: "#D92D20", Dark: "#FF6B6B"}
)

func buildAuthCommand(patCaller edition.ToolCaller) *cobra.Command {
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

	if !edition.Get().HideAuthLogin {
		cmd.AddCommand(newAuthLoginCommand(patCaller))
	}
	cmd.AddCommand(
		newAuthLogoutCommand(),
		newAuthStatusCommand(),
		newAuthExportCommand(),
		newAuthImportCommand(),
		newAuthExchangeCommand(),
		newAuthResetCommand(),
	)
	return cmd
}

func newAuthLoginCommand(patCaller edition.ToolCaller) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "登录钉钉（自动刷新 token，必要时扫码）",
		Long: `登录钉钉并获取认证凭证。

支持的登录方式:
  - OAuth Loopback 流 (默认): 本机自动起 127.0.0.1 监听接收回调，浏览器授权后自动完成
  - OAuth 设备流 (--device): 显示 user_code + 短 URL，适合 SSH 远程 / 容器 / 无头环境
  - 直接提供 Token (--token): 跳过授权，使用已有 token

不支持的登录方式:
  - 邮箱/密码登录
  - 手机号/验证码登录
  - 应用凭证 (AppKey/AppSecret) 直接登录

注意: SSH 远程或无头环境（无本地浏览器可访问远端的 127.0.0.1）请使用 --device，
      否则 OAuth 回调会跳到本机不可达的 127.0.0.1 链接，授权完成后无法回写 token。

示例:
  dws auth login              # 本机登录后选择推荐/全部权限与授权业务域
  dws auth login --recommend  # 无交互批量授权服务端推荐权限
  dws auth login --device     # SSH 远程 / 无头环境登录 (设备流)
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
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			postLoginTUIMode := !cfg.Yes && authLoginShouldUsePostLoginTUIMode(cmd, format, cfg.Recommend)
			recommendAuthMode := cfg.Recommend || postLoginTUIMode
			humanAuthMode := !cfg.Yes && authLoginShouldUseHumanAuthorizationMode(cmd, format, recommendAuthMode)

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
				provider.NoBrowser, _ = cmd.Flags().GetBool("no-browser")
				tokenData, err = provider.Login(loginCtx)
				if err != nil {
					return apperrors.NewAuth(fmt.Sprintf("device authorization failed: %v", err))
				}
			default:
				loginCtx, cancel := context.WithTimeout(cmd.Context(), config.OAuthFlowTimeout)
				defer cancel()

				provider := authpkg.NewOAuthProvider(configDir, nil)
				provider.Output = cmd.ErrOrStderr()
				provider.NoBrowser, _ = cmd.Flags().GetBool("no-browser")
				configureOAuthProviderCompatibility(provider, configDir)
				tokenData, err = provider.Login(loginCtx, cfg.Force)
				if err != nil {
					return apperrors.NewAuth(fmt.Sprintf("dingtalk login failed: %v", err))
				}
			}

			ResetRuntimeTokenCache()
			clearCompatCache()

			w := cmd.OutOrStdout()
			runPostLoginAuthorization := func() error {
				if !recommendAuthMode {
					return nil
				}
				recommendScopeMode := pat.LoginRecommendScopeRecommended
				var initialPlan *pat.LoginRecommendPlan
				if postLoginTUIMode {
					var planErr error
					initialPlan, planErr = pat.PlanLoginRecommendAuthorization(cmd.Context(), patCaller)
					if planErr != nil {
						return planErr
					}
					if authLoginRecommendPlanSkipsInteractiveAuthorization(initialPlan) {
						fmt.Fprintln(cmd.ErrOrStderr(), "推荐权限已全部授权或没有可授权项")
						return nil
					}
					var err error
					recommendScopeMode, err = loginRecommendScopeModeSelector()
					if err != nil {
						return err
					}
				}
				opts := pat.LoginRecommendOptions{Confirmed: cfg.Yes, ScopeMode: recommendScopeMode, InitialPlan: initialPlan}
				if postLoginTUIMode {
					opts.ProductSelector = func(products []pat.LoginRecommendProduct) ([]string, error) {
						return loginRecommendProductSelector(products)
					}
				}
				retryFormat := format
				if humanAuthMode {
					retryFormat = "table"
				}
				run := func(ctx context.Context) error {
					return pat.RunLoginRecommendAuthorizationWithOptions(ctx, patCaller, cmd.ErrOrStderr(), opts)
				}
				err := run(cmd.Context())
				if patErr := apperrors.AsPatAuthCheckError(err); patErr != nil {
					return runDirectPATAuthCheckWaitOnly(
						cmd.Context(),
						&GlobalFlags{Format: retryFormat},
						patErr,
						cmd.ErrOrStderr(),
					)
				}
				return err
			}

			// Check if JSON output is requested
			if strings.EqualFold(strings.TrimSpace(format), "json") && !humanAuthMode {
				if err := runPostLoginAuthorization(); err != nil {
					return err
				}
				return writeAuthLoginJSON(w, tokenData, cfg.Force)
			}

			// Default table output
			if err := runPostLoginAuthorization(); err != nil {
				return err
			}
			fmt.Fprintln(w)
			if !cfg.Device && tokenData != nil && tokenData.IsAccessTokenValid() && !cfg.Force {
				fmt.Fprintln(w, authLoginStatusLine("Token 有效，无需重新登录"))
			} else {
				fmt.Fprintln(w, authLoginStatusLine("登录成功！"))
			}
			if tokenData != nil {
				if tokenData.CorpName != "" {
					fmt.Fprintln(w, authLoginInfoLine("企业", tokenData.CorpName))
				}
				if tokenData.CorpID != "" {
					fmt.Fprintln(w, authLoginInfoLine("企业 ID", tokenData.CorpID))
				}
				if tokenData.UserName != "" {
					fmt.Fprintln(w, authLoginInfoLine("用户", tokenData.UserName))
				}
				if expiry := authLoginDisplayExpiry(tokenData); expiry != "" {
					fmt.Fprintln(w, authLoginInfoLine("有效期", expiry))
				}
			}
			fmt.Fprintln(w, authLoginMutedStyle().Render("Token 将自动刷新，无需重复登录"))
			return nil
		},
	}
	cmd.Flags().String("token", "", "Access token")
	cmd.Flags().Bool("device", false, "Use device authorization flow")
	cmd.Flags().Bool("force", false, "Force interactive login (ignore cached token)")
	cmd.Flags().Bool("recommend", false, "登录成功后无交互批量授权服务端推荐权限")
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
	return cmd
}

var (
	authLoginGuideActionSelector    = selectAuthLoginGuideAction
	authLoginGuideActionApplier     = applyAuthLoginGuideAction
	loginRecommendScopeModeSelector = selectLoginRecommendScopeMode
	loginRecommendProductSelector   = selectLoginRecommendProducts
	authLoginInteractiveTerminal    = isInteractiveTerminal
)

func selectAuthLoginGuideAction() (authLoginGuideAction, error) {
	choice := authLoginGuideDirectCLI
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[authLoginGuideAction]().
				Title("选择操作").
				Options(
					huh.NewOption("直接使用CLI", authLoginGuideDirectCLI),
					huh.NewOption("一键配置智能体应用", authLoginGuideConfigureAgentApp),
					huh.NewOption("手动输入应用凭证", authLoginGuideManualCredentials),
				).
				Value(&choice),
		),
	).WithTheme(authLoginHuhTheme())
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("使用引导选择中止: %w", err)
	}
	return choice, nil
}

func applyAuthLoginGuideAction(cmd *cobra.Command, configDir string, action authLoginGuideAction) error {
	switch action {
	case authLoginGuideDirectCLI:
		return nil
	case authLoginGuideConfigureAgentApp:
		fmt.Fprintln(cmd.ErrOrStderr(), "一键配置智能体应用暂未开放，已继续使用 CLI 登录")
		return nil
	case authLoginGuideManualCredentials:
		clientID, clientSecret, err := promptAuthLoginManualCredentials()
		if err != nil {
			return err
		}
		authpkg.SetClientID(clientID)
		authpkg.SetClientSecret(clientSecret)
		if err := authpkg.SaveAppConfig(configDir, &authpkg.AppConfig{
			ClientID:     clientID,
			ClientSecret: authpkg.PlainSecret(clientSecret),
		}); err != nil {
			return apperrors.NewInternal(fmt.Sprintf("failed to persist app credentials: %v", err))
		}
		return nil
	default:
		return fmt.Errorf("未知操作: %s", action)
	}
}

func promptAuthLoginManualCredentials() (string, string, error) {
	var clientID, clientSecret string
	nonEmpty := func(label string) func(string) error {
		return func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s 不能为空", label)
			}
			return nil
		}
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("输入 AppKey").
				Value(&clientID).
				Validate(nonEmpty("AppKey")),
			huh.NewInput().
				Title("输入 AppSecret").
				EchoMode(huh.EchoModePassword).
				Value(&clientSecret).
				Validate(nonEmpty("AppSecret")),
		),
	).WithTheme(authLoginHuhTheme())
	if err := form.Run(); err != nil {
		return "", "", fmt.Errorf("应用凭证输入中止: %w", err)
	}
	return strings.TrimSpace(clientID), strings.TrimSpace(clientSecret), nil
}

func selectLoginRecommendScopeMode() (pat.LoginRecommendScopeMode, error) {
	choice := pat.LoginRecommendScopeRecommended
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[pat.LoginRecommendScopeMode]().
				Title("选择授权范围").
				Description("空格选择 回车确认").
				Options(
					huh.NewOption("推荐授权", pat.LoginRecommendScopeRecommended),
					huh.NewOption("全部授权", pat.LoginRecommendScopeAll),
				).
				Value(&choice),
		),
	).WithTheme(authLoginHuhTheme())
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("授权范围选择中止: %w", err)
	}
	return choice, nil
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

			// Load token data to get associated clientId before deletion
			var storedClientID string
			if tokenData, err := authpkg.LoadTokenData(configDir); err == nil && tokenData != nil {
				storedClientID = tokenData.ClientID
			}

			if err := authpkg.DeleteTokenData(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to clear token data: %v", err))
			}
			// Clean up associated client secret and app token from keychain
			if storedClientID != "" {
				_ = authpkg.DeleteClientSecret(storedClientID)
				_ = authpkg.DeleteAppTokenData(storedClientID)
			}
			// Also try cleaning app token using appKey from app config
			if appKey, _ := authpkg.ResolveAppCredentials(configDir); appKey != "" && appKey != storedClientID {
				_ = authpkg.DeleteAppTokenData(appKey)
			}
			// Clean up app credentials (app.json + keychain secret)
			_ = authpkg.DeleteAppConfig(configDir)
			_ = os.Remove(filepath.Join(configDir, "mcp_url"))
			_ = os.Remove(filepath.Join(configDir, "token"))
			_ = os.Remove(filepath.Join(configDir, "token.json"))
			ResetRuntimeTokenCache()
			clearCompatCache()
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 已清除所有认证信息")
			if !edition.Get().IsEmbedded {
				fmt.Fprintln(w, "请运行 dws auth login --recommend 重新登录")
			}
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
					} else if edition.Get().AutoPurgeToken {
						_ = authpkg.DeleteTokenData(configDir)
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
				if tokenData != nil {
					if tokenData.IsRefreshTokenValid() {
						fmt.Fprintf(w, "%-16s%s\n", "Refresh Token:", "有效 ✅")
					} else {
						fmt.Fprintf(w, "%-16s%s\n", "Refresh Token:", "缺失或已过期 ⚠️")
					}
				}
				if updatedAt := authStatusUpdatedAt(tokenData); updatedAt != "" {
					fmt.Fprintf(w, "%-16s%s\n", "有效期:", updatedAt)
				}
			} else {
				fmt.Fprintf(w, "%-16s%s\n", "状态:", "未登录")
				if !edition.Get().IsEmbedded {
					fmt.Fprintln(w, "运行 dws auth login --recommend 进行登录")
				}
			}
			return nil
		},
	}
}

func newAuthExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "导出可迁移认证包",
		Long: `导出包含 refresh token 与解密材料的认证包，便于在另一台 Linux 沙箱中导入。

包内包含 ~/.local/share/dws-cli 加密 keychain 与 ~/.dws 必要配置，不含 token 明文。

示例:
  dws auth export -o dws-auth.tar.gz
  dws auth export --base64 > dws-auth.b64`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := cmd.Flags().GetString("output")
			if err != nil {
				return apperrors.NewInternal("failed to read --output")
			}
			asBase64, err := cmd.Flags().GetBool("base64")
			if err != nil {
				return apperrors.NewInternal("failed to read --base64")
			}
			output = strings.TrimSpace(output)
			if !asBase64 && output == "" {
				return apperrors.NewValidation("--output is required unless --base64 is used")
			}
			if !authpkg.PortableExportSupported() {
				return apperrors.NewValidation(fmt.Sprintf(
					"macOS 默认将 DEK 存在系统 Keychain，导出的包无法在其它机器解密；请设置 %s=1 后重新登录再导出",
					keychain.DisableKeychainEnv,
				))
			}
			if !authpkg.PortableAuthSourceReady() {
				return apperrors.NewValidation("尚未登录，请先运行 dws auth login --recommend")
			}

			var bundle bytes.Buffer
			if err := authpkg.ExportPortableAuthBundle(defaultConfigDir(), &bundle); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to export auth bundle: %v", err))
			}

			if asBase64 {
				payload := []byte(base64.StdEncoding.EncodeToString(bundle.Bytes()) + "\n")
				if output == "" {
					_, err := cmd.OutOrStdout().Write(payload)
					return err
				}
				if err := helpers.AtomicWrite(output, payload, config.FilePerm); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to write auth bundle: %v", err))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[OK] 已导出认证包: %s\n", output)
				fmt.Fprintf(cmd.ErrOrStderr(), "认证包含敏感凭据，用完请删除: rm -P %s\n", output)
				return nil
			}

			if err := helpers.AtomicWrite(output, bundle.Bytes(), config.FilePerm); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to write auth bundle: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[OK] 已导出认证包: %s\n", output)
			fmt.Fprintf(cmd.ErrOrStderr(), "认证包含敏感凭据，用完请删除: rm -P %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "认证包输出路径")
	cmd.Flags().Bool("base64", false, "将认证包编码为 base64，便于复制粘贴")
	return cmd
}

func newAuthImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "导入可迁移认证包",
		Long: `从 dws auth export 生成的 tar.gz 或 base64 文件恢复认证。

导入后请运行 dws auth status 确认 refresh token 仍有效。

示例:
  dws auth import -i dws-auth.tar.gz
  dws auth import -i dws-auth.b64 --base64`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := cmd.Flags().GetString("input")
			if err != nil {
				return apperrors.NewInternal("failed to read --input")
			}
			input = strings.TrimSpace(input)
			if input == "" {
				return apperrors.NewValidation("--input is required")
			}
			asBase64, err := cmd.Flags().GetBool("base64")
			if err != nil {
				return apperrors.NewInternal("failed to read --base64")
			}
			force, err := cmd.Flags().GetBool("force")
			if err != nil {
				return apperrors.NewInternal("failed to read --force")
			}

			configDir := defaultConfigDir()
			if !force && authpkg.PortableAuthTargetPopulated(configDir) {
				return apperrors.NewValidation("检测到已有登录态，请使用 --force 确认覆盖")
			}

			payload, err := os.ReadFile(input)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to read auth bundle: %v", err))
			}
			if asBase64 {
				payload, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
				if err != nil {
					return apperrors.NewValidation(fmt.Sprintf("invalid base64 auth bundle: %v", err))
				}
			}
			report, err := authpkg.ImportPortableAuthBundle(configDir, bytes.NewReader(payload))
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to import auth bundle: %v", err))
			}
			if report.OSMismatch {
				fmt.Fprintf(cmd.ErrOrStderr(), "警告: 认证包来自 %s，当前系统为 %s，请确认解密材料兼容\n", report.BundleOS, runtime.GOOS)
			}
			ResetRuntimeTokenCache()
			clearCompatCache()
			fmt.Fprintln(cmd.OutOrStdout(), "[OK] 已导入认证包")
			fmt.Fprintln(cmd.OutOrStdout(), "请运行 dws auth status 验证登录状态")
			return nil
		},
	}
	cmd.Flags().StringP("input", "i", "", "认证包输入路径")
	cmd.Flags().Bool("base64", false, "输入为 base64 编码的认证包")
	cmd.Flags().Bool("force", false, "覆盖已有登录态")
	return cmd
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
			ResetRuntimeTokenCache()
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
			ResetRuntimeTokenCache()
			clearCompatCache()
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 认证信息已重置")
			if !edition.Get().IsEmbedded {
				fmt.Fprintln(w, "请运行 dws auth login --recommend 重新登录")
			}
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

func selectLoginRecommendProducts(products []pat.LoginRecommendProduct) ([]string, error) {
	if len(products) == 0 {
		return nil, nil
	}
	selected := make([]string, 0, len(products))
	options := make([]huh.Option[string], 0, len(products))
	for _, product := range products {
		code := strings.TrimSpace(product.ProductCode)
		if code == "" {
			continue
		}
		selected = append(selected, code)
		options = append(options, huh.NewOption(loginRecommendProductLabel(product), code).Selected(true))
	}
	if len(options) == 0 {
		return nil, nil
	}
	height := len(options)
	if height > 15 {
		height = 15
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("选择要授权的业务域").
				Description("空格选择 回车确认").
				Options(options...).
				Height(height).
				Value(&selected).
				Validate(func(values []string) error {
					if len(values) == 0 {
						return fmt.Errorf("至少选择一个授权业务域")
					}
					return nil
				}),
		),
	).WithTheme(authLoginHuhTheme())
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("授权业务域选择中止: %w", err)
	}
	return selected, nil
}

func authLoginHuhTheme() *huh.Theme {
	t := huh.ThemeBase()

	t.Form.Base = lipgloss.NewStyle().Foreground(authLoginInk)
	t.FieldSeparator = lipgloss.NewStyle().SetString("\n")

	t.Focused.Base = t.Focused.Base.BorderForeground(authLoginBrandBlue)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = lipgloss.NewStyle().Foreground(authLoginBrandBlue).Bold(true)
	t.Focused.NoteTitle = t.Focused.Title.MarginBottom(1)
	t.Focused.Description = authLoginMutedStyle()
	t.Focused.ErrorIndicator = lipgloss.NewStyle().SetString(" *").Foreground(authLoginDanger)
	t.Focused.ErrorMessage = lipgloss.NewStyle().SetString(" *").Foreground(authLoginDanger)
	t.Focused.SelectSelector = lipgloss.NewStyle().SetString("› ").Foreground(authLoginBrandBlue).Bold(true)
	t.Focused.MultiSelectSelector = t.Focused.SelectSelector
	t.Focused.Option = lipgloss.NewStyle().Foreground(authLoginInk)
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(authLoginBrandBlue).Bold(true)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().SetString("● ").Foreground(authLoginBrandBlue)
	t.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(authLoginInk)
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString("○ ").Foreground(authLoginMuted)
	t.Focused.NextIndicator = lipgloss.NewStyle().SetString("→").Foreground(authLoginBrandBlue)
	t.Focused.PrevIndicator = lipgloss.NewStyle().SetString("←").Foreground(authLoginMuted)
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0B1220"}).
		Background(authLoginBrandBlue).
		Padding(0, 2).
		Bold(true)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(authLoginInk).
		Background(authLoginLine).
		Padding(0, 2)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(authLoginBrandBlue)
	t.Focused.TextInput.CursorText = lipgloss.NewStyle().Foreground(authLoginInk)
	t.Focused.TextInput.Placeholder = authLoginMutedStyle()
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(authLoginBrandBlue)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(authLoginInk)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder()).BorderForeground(authLoginLine)
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = lipgloss.NewStyle().Foreground(authLoginInk)
	t.Blurred.NoteTitle = t.Blurred.Title.MarginBottom(1)
	t.Blurred.Description = authLoginMutedStyle()
	t.Blurred.SelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.MultiSelectSelector = t.Blurred.SelectSelector
	t.Blurred.SelectedOption = lipgloss.NewStyle().Foreground(authLoginInk)
	t.Blurred.SelectedPrefix = lipgloss.NewStyle().SetString("● ").Foreground(authLoginBrandBlue)
	t.Blurred.UnselectedOption = lipgloss.NewStyle().Foreground(authLoginMuted)
	t.Blurred.UnselectedPrefix = lipgloss.NewStyle().SetString("○ ").Foreground(authLoginMuted)
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()
	t.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(authLoginMuted)
	t.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(authLoginInk)

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	t.Help.ShortKey = authLoginMutedStyle()
	t.Help.ShortDesc = authLoginMutedStyle()
	t.Help.ShortSeparator = authLoginMutedStyle()
	t.Help.FullKey = authLoginMutedStyle()
	t.Help.FullDesc = authLoginMutedStyle()
	t.Help.FullSeparator = authLoginMutedStyle()
	t.Help.Ellipsis = authLoginMutedStyle()

	return t
}

func authLoginStatusLine(message string) string {
	return fmt.Sprintf("%s %s",
		lipgloss.NewStyle().Foreground(authLoginBrandBlue).Bold(true).Render("[OK]"),
		lipgloss.NewStyle().Foreground(authLoginInk).Bold(true).Render(message),
	)
}

func authLoginInfoLine(key, value string) string {
	label := authLoginMutedStyle().Width(14).Render(key + ":")
	return fmt.Sprintf("%s %s", label, value)
}

func authLoginMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(authLoginMuted)
}

func authLoginShouldShowPostLoginTUI(cmd *cobra.Command, format string, recommend bool) bool {
	return authLoginShouldUsePostLoginTUIModeForTerminal(cmd, format, recommend, authLoginInteractiveTerminal())
}

func authLoginShouldShowPostLoginTUIForTerminal(cmd *cobra.Command, format string, recommend bool, interactive bool) bool {
	return authLoginShouldUsePostLoginTUIModeForTerminal(cmd, format, recommend, interactive)
}

func authLoginShouldUsePostLoginTUIMode(cmd *cobra.Command, format string, recommend bool) bool {
	return authLoginShouldUsePostLoginTUIModeForTerminal(cmd, format, recommend, authLoginInteractiveTerminal())
}

func authLoginShouldUsePostLoginTUIModeForTerminal(cmd *cobra.Command, format string, recommend bool, interactive bool) bool {
	if recommend || !interactive {
		return false
	}
	return authLoginAllowsInteractiveDefault(cmd, format)
}

func authLoginShouldUseHumanAuthorizationMode(cmd *cobra.Command, format string, hasAuthorizationFlow bool) bool {
	return authLoginShouldUseHumanAuthorizationModeForTerminal(cmd, format, hasAuthorizationFlow, authLoginInteractiveTerminal())
}

func authLoginShouldUseHumanAuthorizationModeForTerminal(cmd *cobra.Command, format string, hasAuthorizationFlow bool, interactive bool) bool {
	if !hasAuthorizationFlow || !interactive {
		return false
	}
	return authLoginAllowsInteractiveDefault(cmd, format)
}

func authLoginRecommendPlanSkipsInteractiveAuthorization(plan *pat.LoginRecommendPlan) bool {
	if plan == nil {
		return false
	}
	return plan.AllGranted || len(plan.Scopes) == 0
}

func authLoginAllowsInteractiveDefault(cmd *cobra.Command, format string) bool {
	if cmd == nil || cmd.Root() == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(format), "json") {
		return true
	}
	flags := cmd.Root().PersistentFlags()
	return !flags.Changed("format")
}

func loginRecommendProductLabel(product pat.LoginRecommendProduct) string {
	name := strings.TrimSpace(product.ProductName)
	if name == "" || name == product.ProductCode {
		name = product.ProductCode
	}
	summary := strings.TrimSpace(product.Summary)
	if summary != "" {
		summary = " - " + clipRunes(summary, 42)
	}
	return fmt.Sprintf("%-10s %s%s", product.ProductCode, name, summary)
}

func clipRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
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
	recommend, err := cmd.Flags().GetBool("recommend")
	if err != nil {
		return authLoginConfig{}, apperrors.NewInternal("failed to read --recommend")
	}
	yes := false
	if cmd.Root() != nil {
		yes, _ = cmd.Root().PersistentFlags().GetBool("yes")
	}
	return authLoginConfig{
		Token:     strings.TrimSpace(token),
		Force:     force,
		Device:    device,
		Recommend: recommend,
		Yes:       yes,
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
