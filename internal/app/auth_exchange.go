// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var authExternalExchange = authpkg.ExchangeExternalAuthCode

func newAuthExchangeCommand(caller edition.ToolCaller) *cobra.Command {
	cmd := &cobra.Command{
		Use: "exchange", Short: "使用外部授权码登录并保存完整身份",
		Long: `使用在其他设备或服务中取得的授权码登录，无需本机已有主管身份。
--client-id default 选择当前服务环境的官方默认授权应用；具体 ID 必须与 code 所属应用一致。
省略 client-id 时沿用已配置应用，无配置时使用官方默认应用。已有匹配的 ClientID/ClientSecret
配置继续使用普通 OAuth；其他应用使用服务端托管凭证。不会跨应用或路由试兑一次性 code。
必须通过新 Token 在线核验 corpId/userId 才保存 Profile。首次登录设为默认；已有默认身份时保留。
已持有 agentUuid、需主管取码时用 dingtalk-tag manage login；接入本地 DSH 时用 dingtalk-tag connect。
--dry-run 仅检查参数并输出步骤，不读取 stdin、不联网、不写入凭证。`,
		Example: "  dws auth exchange --code-stdin --client-id default --format json\n  dws auth exchange --code-stdin --client-id <dwsClientId> --format json",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra 保留 flag 值；授权码及应用参数只能属于本次调用，失败也必须清理。
			defer resetAuthExchangeInvocationFlags(cmd)
			code, err := cmd.Flags().GetString("code")
			if err != nil {
				return apperrors.NewInternal("failed to read --code")
			}
			stdin, err := cmd.Flags().GetBool("code-stdin")
			if err != nil {
				return apperrors.NewInternal("failed to read --code-stdin")
			}
			code = strings.TrimSpace(code)
			if stdin && cmd.Flags().Changed("code") {
				return apperrors.NewValidation("--code and --code-stdin are mutually exclusive")
			}
			if !stdin && code == "" {
				return apperrors.NewValidation("--code or --code-stdin is required")
			}
			uid, _ := cmd.Flags().GetString("uid")
			expectedUID, _ := cmd.Flags().GetString("expected-user-id")
			expectedCorp, _ := cmd.Flags().GetString("expected-corp-id")
			uid, expectedUID = strings.TrimSpace(uid), strings.TrimSpace(expectedUID)
			if uid != "" && expectedUID != "" && uid != expectedUID {
				return apperrors.NewValidation("--uid and --expected-user-id must agree")
			}
			if expectedUID == "" {
				expectedUID = uid
			}
			clientID, err := authExchangeClientIDFlag(cmd)
			if err != nil {
				return err
			}
			secret := ""
			if flag := cmd.Root().PersistentFlags().Lookup("client-secret"); flag != nil && flag.Changed {
				secret = flag.Value.String()
			}
			if clientID == "default" && secret != "" {
				return apperrors.NewValidation("--client-id default cannot be combined with --client-secret")
			}
			dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run")
			if dryRun {
				return writeAuthExchangeResult(cmd, map[string]any{"status": "planned", "steps": []string{"resolve_application", "exchange_code", "verify_online_identity", "persist_exact_profile"}})
			}
			if stdin {
				// Bound input, and never include the supplied code in errors/output.
				const limit = 64 * 1024
				input, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), limit+1))
				if err != nil || len(input) > limit {
					return apperrors.NewValidation("cannot read authorization code from stdin (maximum 64 KiB)")
				}
				code = strings.TrimSpace(string(input))
				if code == "" {
					return apperrors.NewValidation("authorization code on stdin is empty")
				}
			}
			configDir := defaultConfigDir()
			exchangeCtx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			data, err := authExternalExchange(exchangeCtx, configDir, authpkg.ExternalExchangeRequest{
				ClientID: clientID, ClientSecret: secret, AuthCode: code,
				ExpectedUserID: expectedUID, ExpectedCorpID: strings.TrimSpace(expectedCorp),
				ResolveIdentity: func(ctx context.Context, token, corpID string) (authpkg.ManagedIdentity, error) {
					tokenCaller, ok := caller.(tokenOverrideToolCaller)
					if !ok {
						return authpkg.ManagedIdentity{}, fmt.Errorf("request-scoped identity lookup unavailable")
					}
					result, err := tokenCaller.CallToolWithToken(ctx, token, "contact", "get_current_user_profile", nil)
					if err != nil {
						return authpkg.ManagedIdentity{}, fmt.Errorf("online identity lookup failed")
					}
					return authpkg.ManagedIdentityFromToolResult(result, corpID)
				},
			})
			if err != nil {
				return apperrors.NewAuth(err.Error())
			}
			if data == nil || data.CorpID == "" || data.UserID == "" {
				return apperrors.NewInternal("exchange did not return a complete identity")
			}
			ResetRuntimeTokenCache()
			clearCompatCache()
			profile := authpkg.TokenProfileSelector(data)
			cfg, err := authpkg.LoadProfiles(configDir)
			if err != nil {
				return apperrors.NewInternal("profile saved but current profile readback failed")
			}
			return writeAuthExchangeResult(cmd, map[string]any{
				"status": "profile_saved", "dwsProfile": profile, "corpId": data.CorpID, "userId": data.UserID,
				"clientId": data.ClientID, "expiresAt": data.ExpiresAt.UTC().Format(time.RFC3339),
				"currentProfile": cfg.CurrentProfile, "isCurrentProfile": cfg.CurrentProfile == profile,
				"useOnce": fmt.Sprintf("dws --profile %s <command>", profile),
			})
		},
	}
	cmd.Flags().String("code", "", "外部授权码；建议通过 --code-stdin 传递")
	cmd.Flags().Bool("code-stdin", false, "从标准输入读取授权码（与 --code 互斥）")
	cmd.Flags().String("client-id", "", "default 使用官方默认应用；具体 ID 使用对应授权应用；省略沿用配置或默认应用")
	cmd.Flags().String("expected-user-id", "", "预期 userId，仅作在线身份一致性校验")
	cmd.Flags().String("expected-corp-id", "", "预期 corpId，仅作在线身份一致性校验")
	cmd.Flags().String("uid", "", "预期 userId（同 --expected-user-id，不会覆盖在线身份）")
	// Keep the upstream public flag surface visible for existing users and
	// other editions; adding managed exchange must not hide their options.
	for _, name := range []string{"authorize-url", "token-url", "refresh-url", "redirect-url", "scopes"} {
		cmd.Flags().String(name, "", "Compatibility flag")
	}
	return cmd
}

func isAuthExchangeCommand(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Name() == "exchange" && cmd.Parent() != nil && cmd.Parent().Name() == "auth"
}

func resetAuthExchangeInvocationFlags(cmd *cobra.Command) {
	if !isAuthExchangeCommand(cmd) {
		return
	}
	for _, name := range []string{"code", "code-stdin", "client-id", "expected-user-id", "expected-corp-id", "uid", "authorize-url", "token-url", "refresh-url", "redirect-url", "scopes"} {
		if flag := cmd.Flags().Lookup(name); flag != nil {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		}
	}
	for _, name := range []string{"client-id", "client-secret"} {
		if flag := cmd.Root().PersistentFlags().Lookup(name); flag != nil {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		}
	}
}

// Cobra permits persistent options before the command. Respect both positions
// without letting root credential overrides mutate the exchange's application.
func authExchangeClientIDFlag(cmd *cobra.Command) (string, error) {
	local := cmd.Flags().Lookup("client-id")
	root := cmd.Root().PersistentFlags().Lookup("client-id")
	if local != nil && local.Changed {
		value := strings.TrimSpace(local.Value.String())
		if value == "" {
			return "", apperrors.NewValidation("--client-id must be default or a concrete application ID")
		}
		if root != nil && root.Changed && strings.TrimSpace(root.Value.String()) != value {
			return "", apperrors.NewValidation("conflicting --client-id values")
		}
		return value, nil
	}
	if root != nil && root.Changed {
		value := strings.TrimSpace(root.Value.String())
		if value == "" {
			return "", apperrors.NewValidation("--client-id must be default or a concrete application ID")
		}
		return value, nil
	}
	return "", nil
}

func writeAuthExchangeResult(cmd *cobra.Command, data map[string]any) error {
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if strings.EqualFold(format, "json") {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"success": true, "data": data})
	}
	if data["status"] == "planned" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "计划：解析授权应用 → 换票 → 在线身份核验 → 保存 Profile（未执行）")
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "[OK] 已登录并保存身份：%s\n当前默认身份：%s\n使用：%s\n", data["dwsProfile"], data["currentProfile"], data["useOnce"])
	return err
}
