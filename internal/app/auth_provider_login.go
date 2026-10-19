// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License")

package app

import (
	"context"
	"errors"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var authProviderLogin = loginWithProviderLogin

func loginWithProviderLogin(ctx context.Context, configDir, provider, baseURL string, caller edition.ToolCaller, credentials *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
	authorizer, err := resolveOIDCAuthorizer(provider)
	if err != nil {
		return nil, err
	}
	login := authpkg.NewOIDCLoginProvider(provider, baseURL, authorizer)
	login.ExchangeCredentials = credentials
	login.IdentityEnricher = func(ctx context.Context, data *authpkg.TokenData) error {
		return enrichAuthLoginProfileFromContact(ctx, configDir, caller, data)
	}
	return login.LoginAndSave(ctx, configDir)
}

// isAuthProviderLoginCommand identifies the provider-login leaf by tree
// position. Provider selection is a positional argument, so no flag half-state
// has to be tracked on early exits.
func isAuthProviderLoginCommand(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Name() == "provider-login" && cmd.Parent() != nil && cmd.Parent().Name() == "auth"
}

// Application credentials belong to one invocation and are never written into
// the global application. Cobra may reuse the same root after any error or
// help exit, so clear the pair the provider exchange consumed on every exit
// path.
func resetAuthProviderLoginInvocationFlags(cmd *cobra.Command) {
	if !isAuthProviderLoginCommand(cmd) || cmd.Root() == nil {
		return
	}
	for _, name := range []string{"client-id", "client-secret"} {
		if flag := cmd.Root().PersistentFlags().Lookup(name); flag != nil {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		}
	}
}

func newAuthProviderLoginCommand(patCaller edition.ToolCaller) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider-login <provider>",
		Short: "通过接入方（Provider）单点登录钉钉",
		Long: `通过接入方（Provider）单点登录（SSO）钉钉并获取认证凭证。

<provider> 为登录提供方名称（如 ant）。DWS 服务端检查接入方配置并托管授权
事务，CLI 只创建事务、把授权请求转交给接入方并兑换 DWS 登录态，不接触接入方
密码，接入方 SDK 会话也不写入本地。

参数:
  --client-id/--client-secret  可选的自定义 DWS 应用凭据，必须成对非空，
                               随登录安全保存以便刷新，不改变全局应用配置；不传则由服务端托管
  --mcp-url                    覆盖本次登录的 MCP 端点
  --login-timeout              登录超时秒数（默认 12 分钟）

示例:
  dws auth provider-login ant
  dws auth provider-login ant --client-id <dws-app-id> --client-secret <dws-app-secret>`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return apperrors.NewValidation("provider-login requires exactly one non-empty provider argument")
			}
			return runAuthProviderLogin(cmd, patCaller, args[0])
		},
	}
	cmd.Flags().String("mcp-url", "", "Override MCP base URL for this login")
	cmd.Flags().Int("login-timeout", 0, "Login timeout seconds")
	_ = cmd.Flags().MarkHidden("login-timeout")
	return cmd
}

func runAuthProviderLogin(cmd *cobra.Command, caller edition.ToolCaller, provider string) error {
	defer resetAuthProviderLoginInvocationFlags(cmd)
	var credentials *authpkg.OIDCExchangeCredentials
	id, secret := cmd.Flag("client-id"), cmd.Flag("client-secret")
	idSet, secretSet := id != nil && id.Changed, secret != nil && secret.Changed
	if idSet != secretSet {
		return apperrors.NewValidation("--client-id and --client-secret must be provided together")
	}
	if idSet {
		credentials = &authpkg.OIDCExchangeCredentials{ClientID: id.Value.String(), ClientSecret: secret.Value.String()}
		if err := credentials.Validate(); err != nil {
			return err
		}
	}
	seconds, _ := cmd.Flags().GetInt("login-timeout")
	if seconds < 0 || int64(seconds) > int64((292*365*24*time.Hour)/time.Second) {
		return apperrors.NewValidation("--login-timeout must be a non-negative number of seconds within the duration limit")
	}
	if seconds == 0 {
		seconds = 12 * 60
	}
	base := authpkg.GetMCPBaseURL()
	persistence := authLoginMCPUseDefault
	if cmd.Flags().Changed("mcp-url") {
		value, _ := cmd.Flags().GetString("mcp-url")
		if strings.TrimSpace(value) == "" {
			return apperrors.NewValidation("--mcp-url cannot be empty")
		}
		// Validate the original input before normalization; credentials/query must
		// never be silently stripped from a provider endpoint configuration.
		candidate := strings.TrimSpace(value)
		if !strings.Contains(candidate, "://") {
			candidate = "https://" + candidate
		}
		if err := authpkg.NewOIDCLoginProvider(provider, candidate, nil).Validate(); err != nil {
			return err
		}
		base = strings.TrimRight(candidate, "/")
		persistence = authLoginMCPUseExplicitOverride
	}
	if err := authpkg.NewOIDCLoginProvider(provider, base, nil).Validate(); err != nil {
		return err
	}
	dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run")
	if dryRun {
		return writeAuthExchangeResult(cmd, map[string]any{"status": "planned", "provider": provider, "steps": []string{"create_oidc_transaction", "sdk_login", "exchange_same_code_state", "persist_exact_profile"}})
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(seconds)*time.Second)
	defer cancel()
	configDir := defaultConfigDir()
	restoreMCPBaseURL := authpkg.PushMCPBaseURLOverride(base)
	defer restoreMCPBaseURL()
	data, err := authProviderLogin(ctx, configDir, provider, base, caller, credentials)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		if !apperrors.DeclaresClassification(err) && errors.Is(err, context.DeadlineExceeded) {
			return helpers.WrapErrorWithOperation(context.DeadlineExceeded, "auth provider-login "+provider)
		}
		return err
	}
	if data == nil || data.AccessToken == "" {
		return apperrors.NewInternal("Provider login returned no token")
	}
	if err := persistAuthLoginMCPBaseURL(configDir, base, persistence); err != nil {
		return apperrors.NewInternal("login profile saved but MCP endpoint could not be persisted")
	}
	ResetRuntimeTokenCache()
	clearCompatCache()
	w := cmd.OutOrStdout()
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		return writeAuthLoginJSON(w, data, true)
	}
	authLoginPrintHumanResult(w, data)
	return nil
}
