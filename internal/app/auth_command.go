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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type authLoginConfig struct {
	Token        string
	Force        bool
	Device       bool
	Recommend    bool
	Yes          bool
	TargetCorpID string
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
		newAuthMigrateKeychainCommand(),
		newAuthExportCommand(),
		newAuthImportCommand(),
		newAuthExchangeCommand(patCaller),
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
  dws auth login              # 本机登录并新增/刷新一个组织 profile
  dws auth login --profile <corpId>  # 指定本次授权目标组织，不持久切换当前组织
  dws auth login --recommend  # 无交互批量授权服务端推荐权限
  dws auth login --device     # SSH 远程 / 无头环境登录 (设备流)
  dws auth login --force      # 兼容保留；login 默认已忽略缓存并进入授权流程
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
				if err := authSaveTokenData(configDir, tokenData); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to persist auth token: %v", err))
				}
			case cfg.Device:
				loginCtx, cancel := context.WithTimeout(cmd.Context(), config.DeviceFlowTimeout)
				defer cancel()

				provider := authpkg.NewDeviceFlowProvider(configDir, nil)
				provider.Output = cmd.ErrOrStderr()
				provider.NoBrowser, _ = cmd.Flags().GetBool("no-browser")
				provider.IdentityEnricher = func(ctx context.Context, data *authpkg.TokenData) error {
					return enrichAuthLoginProfileFromContact(ctx, configDir, patCaller, data)
				}
				tokenData, err = authDeviceLogin(provider, loginCtx)
				if err != nil {
					return apperrors.NewAuth(fmt.Sprintf("device authorization failed: %v", err))
				}
			default:
				loginCtx, cancel := context.WithTimeout(cmd.Context(), config.OAuthFlowTimeout)
				defer cancel()

				provider := authpkg.NewOAuthProvider(configDir, nil)
				provider.Output = cmd.ErrOrStderr()
				provider.NoBrowser, _ = cmd.Flags().GetBool("no-browser")
				provider.TargetCorpID = cfg.TargetCorpID
				provider.IdentityEnricher = func(ctx context.Context, data *authpkg.TokenData) error {
					return enrichAuthLoginProfileFromContact(ctx, configDir, patCaller, data)
				}
				configureOAuthProviderCompatibility(provider, configDir)
				tokenData, err = authOAuthLogin(provider, loginCtx, authLoginForcesAuthorization(cfg))
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
				restoreProfile := replaceRuntimeProfile(authpkg.TokenProfileSelector(tokenData))
				defer restoreProfile()
				recommendScopeMode := pat.LoginRecommendScopeRecommended
				var initialPlan *pat.LoginRecommendPlan
				if postLoginTUIMode {
					var planErr error
					initialPlan, planErr = authPlanLoginRecommend(cmd.Context(), patCaller)
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
					return authRunLoginRecommend(ctx, patCaller, cmd.ErrOrStderr(), opts)
				}
				err := run(cmd.Context())
				if patErr := apperrors.AsPatAuthCheckError(err); patErr != nil {
					return authRunDirectPATWait(
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
				return writeAuthLoginJSON(w, tokenData, authLoginForcesAuthorization(cfg))
			}

			// Default table output
			if err := runPostLoginAuthorization(); err != nil {
				return err
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, authLoginStatusLine("登录成功！"))
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
	cmd.Flags().Bool("force", false, "兼容保留；login 默认已忽略缓存并进入授权流程")
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
	authLoginGuideActionSelector     = selectAuthLoginGuideAction
	authLoginGuideActionApplier      = applyAuthLoginGuideAction
	authLoginManualCredentialsPrompt = promptAuthLoginManualCredentials
	loginRecommendScopeModeSelector  = selectLoginRecommendScopeMode
	loginRecommendProductSelector    = selectLoginRecommendProducts
	authLoginInteractiveTerminal     = isInteractiveTerminal
	migrateKeychainToFileDEK         = authpkg.MigrateKeychainToFileDEK
	authMigrateTarget                = func(cmd *cobra.Command) (string, error) { return cmd.Flags().GetString("to") }
	authRunForm                      = (*huh.Form).Run
	authSaveTokenData                = authpkg.SaveTokenData
	authSaveAppConfig                = authpkg.SaveAppConfig
	authDeviceLogin                  = func(provider *authpkg.DeviceFlowProvider, ctx context.Context) (*authpkg.TokenData, error) {
		return provider.Login(ctx)
	}
	authOAuthLogin = func(provider *authpkg.OAuthProvider, ctx context.Context, force bool) (*authpkg.TokenData, error) {
		return provider.Login(ctx, force)
	}
	authOAuthStatus      = func(provider *authpkg.OAuthProvider) (*authpkg.TokenData, error) { return provider.Status() }
	authOAuthAccessToken = func(provider *authpkg.OAuthProvider, ctx context.Context) (string, error) {
		return provider.GetAccessToken(ctx)
	}
	authOAuthExchange = func(provider *authpkg.OAuthProvider, ctx context.Context, code, uid string) (*authpkg.TokenData, error) {
		return provider.ExchangeAuthCode(ctx, code, uid)
	}
	authPlanLoginRecommend      = pat.PlanLoginRecommendAuthorization
	authRunLoginRecommend       = pat.RunLoginRecommendAuthorizationWithOptions
	authRunDirectPATWait        = runDirectPATAuthCheckWaitOnly
	authResolveProfile          = authpkg.ResolveProfile
	authResolveProfileDeletion  = authpkg.ResolveProfileDeletionScope
	authRevokeToken             = authpkg.RevokeTokenRemote
	authRevokeTokenForData      = authpkg.RevokeTokenRemoteForData
	authLoadTokenForProfile     = authpkg.LoadTokenDataForProfile
	authDeleteProfileToken      = authpkg.DeleteTokenDataForProfile
	authEnsureProfilesMigration = authpkg.EnsureProfilesMigration
	authLoadProfiles            = authpkg.LoadProfiles
	authDeleteAllTokenData      = authpkg.DeleteAllTokenData
	authDeleteTokenData         = authpkg.DeleteTokenData
	authMarkProfileStatus       = authpkg.MarkProfileStatus
	authPortableExportSupported = authpkg.PortableExportSupported
	authPortableSourceReady     = authpkg.PortableAuthSourceReady
	authPortableTargetPopulated = authpkg.PortableAuthTargetPopulated
	authExportPortableBundle    = authpkg.ExportPortableAuthBundle
	authImportPortableBundle    = authpkg.ImportPortableAuthBundle
	authAtomicWrite             = helpers.AtomicWrite
	authReadFile                = os.ReadFile
	authRemove                  = os.Remove
	authDeleteAppConfig         = authpkg.DeleteAppConfig
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
	if err := authRunForm(form); err != nil {
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
		clientID, clientSecret, err := authLoginManualCredentialsPrompt()
		if err != nil {
			return err
		}
		authpkg.SetClientID(clientID)
		authpkg.SetClientSecret(clientSecret)
		if err := authSaveAppConfig(configDir, &authpkg.AppConfig{
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
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("输入 AppKey").
				Value(&clientID).
				Validate(authLoginNonEmpty("AppKey")),
			huh.NewInput().
				Title("输入 AppSecret").
				EchoMode(huh.EchoModePassword).
				Value(&clientSecret).
				Validate(authLoginNonEmpty("AppSecret")),
		),
	).WithTheme(authLoginHuhTheme())
	if err := authRunForm(form); err != nil {
		return "", "", fmt.Errorf("应用凭证输入中止: %w", err)
	}
	return strings.TrimSpace(clientID), strings.TrimSpace(clientSecret), nil
}

func authLoginNonEmpty(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s 不能为空", label)
		}
		return nil
	}
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
	if err := authRunForm(form); err != nil {
		return "", fmt.Errorf("授权范围选择中止: %w", err)
	}
	return choice, nil
}

func newAuthLogoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "清除认证信息（默认退出所有组织）",
		Long: `清除本机钉钉登录态。

默认退出全部账号。--profile 传组织时退出该组织全部账号；传精确账号或本地 profile 名时只退出一个账号。`,
		Example: `  dws auth logout
  dws auth logout --profile <corpId>
  dws auth logout --profile <corpId>:<userId>
  dws auth logout --profile "钉钉"`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			profileSelector, err := cmd.Flags().GetString("profile")
			if err != nil {
				return apperrors.NewInternal("failed to read --profile")
			}
			revokeCtx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			if strings.TrimSpace(profileSelector) != "" {
				if err := logoutOneProfile(cmd, revokeCtx, configDir, profileSelector); err != nil {
					return err
				}
			} else {
				if err := logoutAllProfiles(cmd, revokeCtx, configDir); err != nil {
					return err
				}
			}
			ResetRuntimeTokenCache()
			clearCompatCache()
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "[OK] 已清除认证信息")
			if !edition.Get().IsEmbedded {
				fmt.Fprintln(w, "请运行 dws auth login --recommend 重新登录")
			}
			return nil
		},
	}
	cmd.Flags().String("profile", "", "指定组织或账号：corpId、corpName、corpId:userId、corpId:userName、corpName:userId、corpName:userName 或本地 profile 名")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看认证状态",
		Long: `查看当前或指定组织 profile 的认证状态。

指定 --profile 时只读取并刷新被选中的 token slot，不会修改 currentProfile。`,
		Example: `  dws auth status
  dws auth status --profile <corpId>
  dws auth status --profile <corpId>:<userId>
  dws auth status --profile "钉钉:孙博文"
  dws auth status --profile <corpId> --format json`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			profileSelector, err := cmd.Flags().GetString("profile")
			if err != nil {
				return apperrors.NewInternal("failed to read --profile")
			}
			profileSelector = strings.TrimSpace(profileSelector)
			if profileSelector != "" {
				selected, resolveErr := authpkg.ResolveProfile(configDir, profileSelector)
				if resolveErr != nil {
					return apperrors.NewValidation(resolveErr.Error())
				}
				if selected == nil {
					return apperrors.NewValidation(fmt.Sprintf("profile %q not found", profileSelector))
				}
				profileSelector = authpkg.ProfileSelector(*selected)
			}
			restoreProfile := pushRuntimeProfile(profileSelector)
			defer restoreProfile()

			authenticated := false
			refreshed := false
			var tokenData *authpkg.TokenData
			var statusErr error
			var refreshFailure error
			provider := authpkg.NewOAuthProvider(configDir, nil)
			configureOAuthProviderCompatibility(provider, configDir)
			if data, err := authOAuthStatus(provider); err == nil {
				tokenData = data
				if !data.IsAccessTokenValid() && data.IsRefreshTokenValid() {
					refreshCtx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
					_, refreshErr := authOAuthAccessToken(provider, refreshCtx)
					cancel()
					if refreshErr == nil {
						if updatedData, statusErr := authOAuthStatus(provider); statusErr == nil {
							tokenData = updatedData
							refreshed = true
						}
					} else if edition.Get().AutoPurgeToken {
						refreshFailure = refreshErr
						_ = authDeleteTokenData(configDir)
					} else if tokenData != nil {
						refreshFailure = refreshErr
						_ = authMarkProfileStatus(configDir, authpkg.TokenProfileSelector(tokenData), authpkg.ProfileStatusExpired)
					}
				}
				if refreshFailure == nil && authStatusAuthenticated(tokenData) {
					authenticated = true
				}
			} else {
				statusErr = err
			}
			diagnostic := authStatusDiagnosticFromError(statusErr)
			if refreshFailure != nil {
				diagnostic = authStatusRefreshDiagnostic(refreshFailure)
			}

			// Check if JSON output is requested
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return writeAuthStatusJSON(cmd.OutOrStdout(), authenticated, refreshed, tokenData, diagnostic)
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
					if tokenData.CorpName != "" {
						fmt.Fprintf(w, "%-16s%s\n", "企业:", tokenData.CorpName)
					}
					if tokenData.CorpID != "" {
						fmt.Fprintf(w, "%-16s%s\n", "企业 ID:", tokenData.CorpID)
					}
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
				if diagnostic != nil {
					fmt.Fprintf(w, "%-16s%s\n", "原因:", diagnostic.Message)
					fmt.Fprintf(w, "%-16s%s\n", "提示:", diagnostic.Hint)
				} else if !edition.Get().IsEmbedded {
					fmt.Fprintln(w, "运行 dws auth login --recommend 进行登录")
				}
			}
			return nil
		},
	}
	cmd.Flags().String("profile", "", "指定组织或账号：corpId、corpName、corpId:userId、corpId:userName、corpName:userId、corpName:userName 或本地 profile 名")
	return cmd
}

func newAuthMigrateKeychainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-keychain",
		Short: "将 macOS 系统 Keychain 登录态安全迁移到 file-DEK",
		Long: `将 dws-cli 的 legacy 与 profile 登录 token 统一重加密为 file-DEK，使 Codex 等沙箱进程与普通终端共享同一登录态。

迁移必须从仍可读取原登录态的系统 Keychain 模式运行。命令会先验证全部认证密文；任何认证条目不可解密时均不会写入。应用密钥等无关条目不在迁移范围内。
先用 --dry-run 预检，确认后加 --yes 执行。`,
		Example: `  env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run --format json
  env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --yes --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := authMigrateTarget(cmd)
			if err != nil {
				return apperrors.NewInternal("failed to read --to")
			}
			if strings.TrimSpace(target) != "file-dek" {
				return apperrors.NewValidation("--to 当前仅支持 file-dek")
			}
			if os.Getenv(keychain.DisableKeychainEnv) != "" {
				return apperrors.NewValidation(fmt.Sprintf(
					"迁移必须从系统 Keychain 模式运行；请使用 `env -u %s dws auth migrate-keychain --to file-dek ...`",
					keychain.DisableKeychainEnv,
				))
			}

			dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run")
			yes, _ := cmd.Root().PersistentFlags().GetBool("yes")
			if !dryRun && !yes {
				return apperrors.NewValidation("迁移会重加密全部本地登录 token；请先使用 --dry-run 预检，确认后加 --yes 执行")
			}
			count, err := migrateKeychainToFileDEK(defaultConfigDir(), dryRun)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("keychain migration failed: %v", err))
			}

			result := struct {
				Success bool   `json:"success"`
				DryRun  bool   `json:"dry_run"`
				Target  string `json:"target"`
				Entries int    `json:"entries"`
			}{Success: true, DryRun: dryRun, Target: "file-dek", Entries: count}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "预检通过：%d 个本地认证条目可迁移到 file-DEK\n", count)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "迁移完成：%d 个本地认证条目已统一使用 file-DEK\n", count)
			}
			return nil
		},
	}
	cmd.Flags().String("to", "file-dek", "目标密钥后端（当前仅支持 file-dek）")
	return cmd
}

func logoutOneProfile(_ *cobra.Command, ctx context.Context, configDir, selector string) error {
	selected, exact, err := authResolveProfileDeletion(configDir, selector)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if selected == nil {
		return apperrors.NewValidation(fmt.Sprintf("profile %q not found", selector))
	}
	stableSelector := selected.CorpID
	if exact {
		stableSelector = authpkg.ProfileSelector(*selected)
		if data, loadErr := authLoadTokenForProfile(configDir, stableSelector); loadErr == nil {
			_ = authRevokeTokenForData(ctx, data)
		}
	} else if cfg, loadErr := authLoadProfiles(configDir); loadErr == nil {
		for _, profile := range cfg.Profiles {
			if profile.CorpID != selected.CorpID {
				continue
			}
			if data, tokenErr := authLoadTokenForProfile(configDir, authpkg.ProfileSelector(profile)); tokenErr == nil {
				_ = authRevokeTokenForData(ctx, data)
			}
		}
	}
	if err := authDeleteProfileToken(configDir, stableSelector); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.NewValidation(err.Error())
		}
		return apperrors.NewInternal(fmt.Sprintf("failed to clear token data: %v", err))
	}
	return nil
}

func logoutAllProfiles(_ *cobra.Command, ctx context.Context, configDir string) error {
	if err := authEnsureProfilesMigration(configDir); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to migrate profiles: %v", err))
	}
	cfg, err := authLoadProfiles(configDir)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", err))
	}
	if cfg == nil || len(cfg.Profiles) == 0 {
		_ = authRevokeToken(ctx)
	} else {
		for _, profile := range cfg.Profiles {
			if data, tokenErr := authLoadTokenForProfile(configDir, authpkg.ProfileSelector(profile)); tokenErr == nil {
				_ = authRevokeTokenForData(ctx, data)
			}
		}
	}
	if err := authDeleteAllTokenData(configDir); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to clear token data: %v", err))
	}
	return nil
}

func pushRuntimeProfile(selector string) func() {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return func() {}
	}
	previous := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile(selector)
	return func() {
		authpkg.SetRuntimeProfile(previous)
	}
}

func replaceRuntimeProfile(selector string) func() {
	previous := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile(strings.TrimSpace(selector))
	return func() {
		authpkg.SetRuntimeProfile(previous)
	}
}

func newAuthExportCommand() *cobra.Command {
	return newAuthExportCommandWithSupport(authpkg.PortableExportSupportError)
}

func newAuthExportCommandWithSupport(supportError func() error) *cobra.Command {
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
			if err := supportError(); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			if !authPortableExportSupported() {
				return apperrors.NewValidation(fmt.Sprintf(
					"macOS 导出认证包需要 file-DEK 模式；请先设置 %s=1 并运行 dws auth status 验证，只有提示密钥不匹配且确认可丢弃旧登录态时，才执行 dws auth reset 后重新登录",
					keychain.DisableKeychainEnv,
				))
			}
			if !authPortableSourceReady() {
				return apperrors.NewValidation("尚未登录，请先运行 dws auth login --recommend")
			}

			var bundle bytes.Buffer
			if err := authExportPortableBundle(defaultConfigDir(), &bundle); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to export auth bundle: %v", err))
			}

			if asBase64 {
				payload := []byte(base64.StdEncoding.EncodeToString(bundle.Bytes()) + "\n")
				if output == "" {
					_, err := cmd.OutOrStdout().Write(payload)
					return err
				}
				if err := authAtomicWrite(output, payload, config.FilePerm); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to write auth bundle: %v", err))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[OK] 已导出认证包: %s\n", output)
				fmt.Fprintf(cmd.ErrOrStderr(), "认证包含敏感凭据，用完请删除: rm -P %s\n", output)
				return nil
			}

			if err := authAtomicWrite(output, bundle.Bytes(), config.FilePerm); err != nil {
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
	return newAuthImportCommandWithSupport(authpkg.PortableImportSupportError)
}

func newAuthImportCommandWithSupport(supportError func() error) *cobra.Command {
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
			if err := supportError(); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			configDir := defaultConfigDir()
			if !force && authPortableTargetPopulated(configDir) {
				return apperrors.NewValidation("检测到已有登录态，请使用 --force 确认覆盖")
			}

			payload, err := authReadFile(input)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to read auth bundle: %v", err))
			}
			if asBase64 {
				payload, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(payload)))
				if err != nil {
					return apperrors.NewValidation(fmt.Sprintf("invalid base64 auth bundle: %v", err))
				}
			}
			report, err := authImportPortableBundle(configDir, bytes.NewReader(payload))
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

func newAuthExchangeCommand(caller edition.ToolCaller) *cobra.Command {
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
			provider.IdentityEnricher = func(ctx context.Context, data *authpkg.TokenData) error {
				return enrichAuthLoginProfileFromContact(ctx, configDir, caller, data)
			}
			configureOAuthProviderCompatibility(provider, configDir)
			exchangeCtx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			tokenData, err := authOAuthExchange(provider, exchangeCtx, code, strings.TrimSpace(uid))
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
			if err := authDeleteAllTokenData(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to reset token data: %v", err))
			}
			_ = authRemove(filepath.Join(configDir, "mcp_url"))
			_ = authRemove(filepath.Join(configDir, "token"))
			_ = authDeleteAppConfig(configDir)
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
				Validate(authLoginProductsNonEmpty),
		),
	).WithTheme(authLoginHuhTheme())
	if err := authRunForm(form); err != nil {
		return nil, fmt.Errorf("授权业务域选择中止: %w", err)
	}
	return selected, nil
}

func authLoginProductsNonEmpty(values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("至少选择一个授权业务域")
	}
	return nil
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
	// Cache store removed; no-op in static endpoint mode.
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
	profileSelector := ""
	if cmd.Root() != nil {
		yes, _ = cmd.Root().PersistentFlags().GetBool("yes")
		profileSelector, _ = cmd.Root().PersistentFlags().GetString("profile")
	}
	targetCorpID, err := resolveAuthLoginTargetCorpID(defaultConfigDir(), profileSelector)
	if err != nil {
		return authLoginConfig{}, err
	}
	flow := "oauth"
	if strings.TrimSpace(token) != "" {
		flow = "token"
	} else if device {
		flow = "device"
	}
	logging.AuthDebug(
		"auth.login.request",
		"flow", flow,
		"profile_selector", strings.TrimSpace(profileSelector),
		"target_corp_id", targetCorpID,
		"recommend", recommend,
	)
	return authLoginConfig{
		Token:        strings.TrimSpace(token),
		Force:        force,
		Device:       device,
		Recommend:    recommend,
		Yes:          yes,
		TargetCorpID: targetCorpID,
	}, nil
}

func authLoginForcesAuthorization(_ authLoginConfig) bool {
	return true
}

func resolveAuthLoginTargetCorpID(configDir, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", nil
	}
	if profile, err := authResolveProfile(configDir, selector); err == nil && profile != nil {
		return strings.TrimSpace(profile.CorpID), nil
	}
	if strings.HasPrefix(selector, "ding") {
		return selector, nil
	}
	return "", apperrors.NewValidation(fmt.Sprintf("profile %q not found", selector))
}

type contactProfileIdentity struct {
	CorpID   string
	CorpName string
	UserID   string
	UserName string
}

type tokenOverrideToolCaller interface {
	CallToolWithToken(ctx context.Context, token, productID, toolName string, args map[string]any) (*edition.ToolResult, error)
}

func enrichAuthLoginProfileFromContact(ctx context.Context, _ string, caller edition.ToolCaller, data *authpkg.TokenData) error {
	if caller == nil || data == nil {
		return nil
	}
	corpID := strings.TrimSpace(data.CorpID)
	if corpID == "" {
		return nil
	}
	logging.AuthDebug(
		"auth.login.identity.lookup.start",
		"corp_id", corpID,
		"user_id", strings.TrimSpace(data.UserID),
		"user_name", strings.TrimSpace(data.UserName),
		"corp_name", strings.TrimSpace(data.CorpName),
	)
	if strings.TrimSpace(data.CorpName) != "" && strings.TrimSpace(data.UserID) != "" && strings.TrimSpace(data.UserName) != "" {
		logging.AuthDebug(
			"auth.login.identity.lookup.result",
			"source", "token_exchange",
			"corp_id", corpID,
			"user_id", strings.TrimSpace(data.UserID),
			"user_name", strings.TrimSpace(data.UserName),
			"corp_name", strings.TrimSpace(data.CorpName),
		)
		return nil
	}

	var (
		result *edition.ToolResult
		err    error
	)
	if tokenCaller, ok := caller.(tokenOverrideToolCaller); ok && strings.TrimSpace(data.AccessToken) != "" {
		result, err = tokenCaller.CallToolWithToken(ctx, data.AccessToken, "contact", "get_current_user_profile", nil)
	} else {
		if strings.TrimSpace(data.UserID) == "" {
			return fmt.Errorf("login identity lookup requires an in-memory token override")
		}
		return nil
	}
	if err != nil {
		logging.AuthDebug(
			"auth.login.identity.lookup.error",
			"corp_id", corpID,
			"existing_user_id", strings.TrimSpace(data.UserID),
			"error", err,
		)
		if strings.TrimSpace(data.UserID) != "" {
			return nil
		}
		return err
	}
	identity, ok := contactProfileIdentityFromToolResult(result)
	if !ok {
		logging.AuthDebug("auth.login.identity.lookup.empty", "corp_id", corpID)
		return nil
	}
	logging.AuthDebug(
		"auth.login.identity.lookup.result",
		"source", "contact.get_current_user_profile",
		"corp_id", strings.TrimSpace(identity.CorpID),
		"user_id", strings.TrimSpace(identity.UserID),
		"user_name", strings.TrimSpace(identity.UserName),
		"corp_name", strings.TrimSpace(identity.CorpName),
	)
	if identity.CorpID != "" && identity.CorpID != corpID {
		return fmt.Errorf("contact profile corpId %q does not match login corpId %q", identity.CorpID, corpID)
	}

	updated := *data
	if identity.CorpName != "" {
		updated.CorpName = identity.CorpName
	}
	if identity.UserID != "" {
		updated.UserID = identity.UserID
	}
	if identity.UserName != "" {
		updated.UserName = identity.UserName
	}
	if updated.CorpName == data.CorpName && updated.UserID == data.UserID && updated.UserName == data.UserName {
		logging.AuthDebug(
			"auth.login.identity.resolved",
			"corp_id", corpID,
			"user_id", strings.TrimSpace(data.UserID),
			"user_name", strings.TrimSpace(data.UserName),
			"changed", false,
		)
		return nil
	}
	*data = updated
	logging.AuthDebug(
		"auth.login.identity.resolved",
		"corp_id", strings.TrimSpace(data.CorpID),
		"user_id", strings.TrimSpace(data.UserID),
		"user_name", strings.TrimSpace(data.UserName),
		"changed", true,
	)
	return nil
}

func contactProfileIdentityFromToolResult(result *edition.ToolResult) (contactProfileIdentity, bool) {
	if result == nil {
		return contactProfileIdentity{}, false
	}
	for _, block := range result.Content {
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		if identity, ok := contactProfileIdentityFromJSON([]byte(block.Text)); ok {
			return identity, true
		}
	}
	return contactProfileIdentity{}, false
}

func contactProfileIdentityFromJSON(data []byte) (contactProfileIdentity, bool) {
	var payload struct {
		Result []struct {
			OrgEmployeeModel struct {
				CorpID      string `json:"corpId"`
				OrgName     string `json:"orgName"`
				UserID      string `json:"userId"`
				UserIDLower string `json:"userid"`
				OrgUserID   string `json:"orgUserId"`
				OrgUserName string `json:"orgUserName"`
				Name        string `json:"name"`
			} `json:"orgEmployeeModel"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return contactProfileIdentity{}, false
	}
	if len(payload.Result) == 0 {
		return contactProfileIdentity{}, false
	}
	org := payload.Result[0].OrgEmployeeModel
	identity := contactProfileIdentity{
		CorpID:   strings.TrimSpace(org.CorpID),
		CorpName: strings.TrimSpace(org.OrgName),
		UserID:   firstNonEmptyString(org.UserID, org.UserIDLower, org.OrgUserID),
		UserName: firstNonEmptyString(org.OrgUserName, org.Name),
	}
	return identity, identity.CorpID != "" || identity.CorpName != "" || identity.UserID != "" || identity.UserName != ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	Reason            string `json:"reason,omitempty"`
	Hint              string `json:"hint,omitempty"`
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

type authStatusDiagnostic struct {
	Reason  string
	Message string
	Hint    string
}

func authStatusDiagnosticFromError(err error) *authStatusDiagnostic {
	if err == nil {
		return nil
	}
	if keychain.IsCiphertextKeyMismatch(err) {
		return &authStatusDiagnostic{
			Reason:  "ciphertext_key_mismatch",
			Message: "本地登录态与可用登录密钥不匹配，已拒绝覆盖现有凭证",
			Hint:    "macOS 请先在系统 Keychain 模式运行 `env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run`，预检通过后加 --yes 迁移；只有密文损坏且确认无法恢复时才按 profile 退出或执行 auth reset。",
		}
	}
	if keychain.IsDEKMissing(err) {
		return &authStatusDiagnostic{
			Reason:  "dek_missing",
			Message: "本地登录密钥缺失，无法解密已保存的登录态",
			Hint:    "请先恢复或统一原登录密钥；确认旧登录态不可恢复后，执行 dws auth reset，再重新登录。",
		}
	}
	if !keychain.IsUnavailable(err) {
		return nil
	}
	return &authStatusDiagnostic{
		Reason:  "keychain_unavailable",
		Message: "无法读取 macOS Keychain 中的登录密钥，无法判断登录状态",
		Hint:    "检查 macOS 默认钥匙串是否存在且已解锁；修复后重试，或在测试环境设置 DWS_DISABLE_KEYCHAIN=1 后重新登录。",
	}
}

func authStatusRefreshDiagnostic(err error) *authStatusDiagnostic {
	if err == nil {
		return nil
	}
	return &authStatusDiagnostic{
		Reason:  "token_refresh_failed",
		Message: fmt.Sprintf("Token 刷新失败: %v", err),
		Hint:    "请重新运行 dws auth login 完成授权。",
	}
}

func writeAuthStatusJSON(w io.Writer, authenticated, refreshed bool, data *authpkg.TokenData, diagnostic *authStatusDiagnostic) error {
	resp := authStatusResponse{
		Success:       true,
		Authenticated: authenticated,
	}

	if !authenticated {
		if diagnostic != nil {
			resp.Message = diagnostic.Message
			resp.Reason = diagnostic.Reason
			resp.Hint = diagnostic.Hint
		} else {
			resp.Message = "未登录"
		}
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
