// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func newAuthExchangePrepareCommand() *cobra.Command {
	return corecmd.New(corecmd.Spec{
		Use: "exchange-prepare", Short: "创建第三方换票事务并返回授权参数",
		// Hidden: 第三方 IdP 集成入口，不出现在 dws auth --help 与 Agent Schema。
		// 框架下 Hidden 仅当叶子不声明 Contract.Identity 时才排除出 Schema；
		// 若未来补 Identity，必须恢复 reviewed exclusion 或明确接受进入 Agent 表面。
		Hidden: true,
		Long: `创建服务端 OIDC 登录事务，返回 provider、expires_in 和 authorization_request。
业务保存原 state，将完整授权参数原样交给 IdP/SDK；取得 code 并核对返回 state 后，
使用 auth token-exchange 兑换。code_verifier 只保存在服务端。
本命令不执行 SDK 登录，不保存或切换本地身份，也不接受应用凭据。
服务地址使用系统统一的 MCP 配置。
--dry-run 仅检查参数，不创建事务。`,
		Example:       "  dws auth exchange-prepare --provider ant --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
		Flags: []corecmd.FlagSpec{
			{Name: "provider", Usage: "Portal 登记的身份源名称（如 ant）", MarkRequired: true},
		},
		PostMount: func(cmd *cobra.Command) { cmd.Args = cobra.NoArgs },
		Validate:  func(cmd *cobra.Command, _ []string) error { _, err := oidcExchangeCommandProvider(cmd); return err },
		ResultInvoke: func(c *corecmd.Ctx, _ map[string]any) (output.CommandResult, error) {
			cmd := c.Command()
			defer resetAuthOIDCExchangeInvocationFlags(cmd)
			p, err := oidcExchangeCommandProvider(cmd)
			if err != nil {
				return nil, err
			}
			if dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run"); dryRun {
				return output.Success(map[string]any{"status": "planned", "steps": []string{"create_oidc_transaction", "return_authorization_request"}}, output.WithDryRun()), nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			tx, err := p.PrepareExchange(ctx)
			if err != nil {
				return nil, err
			}
			return output.Success(tx), nil
		},
	})
}

func newAuthTokenExchangeCommand(caller edition.ToolCaller) *cobra.Command {
	return corecmd.New(corecmd.Spec{
		Use: "token-exchange", Short: "使用第三方授权码和事务 state 换取 DWS Token",
		// Hidden: 第三方 IdP 集成入口，不出现在 dws auth --help 与 Agent Schema。
		// 框架下 Hidden 仅当叶子不声明 Contract.Identity 时才排除出 Schema；
		// 若未来补 Identity，必须恢复 reviewed exclusion 或明确接受进入 Agent 表面。
		Hidden: true,
		Long: `兑换 auth exchange-prepare 创建的同一事务。业务须先核对 IdP/SDK 返回的 state。
--provider 必须与事务一致；--code 为绑定该事务的一次性授权码，不是 BUC access token。
--code 与 --code-stdin 必须且只能选择一个。标准输入仅移除末尾一个换行，保留 code 原值。
可选 --client-id 和 --client-secret 必须一起出现且非空，仅传给 DWS 换票接口，
不改变 IdP 应用配置。不传时由服务端选择默认 DWS 应用并托管密钥。
默认通过统一结果返回现有 TokenData，不保存凭证。--save 在线核验完整身份后保存，
保留已有默认账号，输出保存结果而不回显 Token。首次保存可设为默认账号。
自定义应用在 --save 时将密钥与 Token 一起安全保存，供后续刷新使用；不写入全局应用配置。
服务地址使用系统统一的 MCP 配置；保存的 Token 绑定该地址，后续刷新沿用。
code/state 只兑换一次；失败或结果不确定时重新 exchange-prepare 并取码，不自动重试。
--dry-run 不读取 stdin、不联网、不保存。`,
		Example:       "  dws auth token-exchange --provider ant --code-stdin --state <state> --format json\n  dws auth token-exchange --provider ant --code <code> --state <state> --save --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
		Flags: []corecmd.FlagSpec{
			{Name: "provider", Usage: "与 exchange-prepare 相同的身份源", MarkRequired: true},
			{Name: "code", Usage: "本次事务的一次性授权码；建议使用 --code-stdin"},
			{Name: "code-stdin", Kind: corecmd.KindBool, Usage: "从标准输入读取授权码，与 --code 二选一"},
			{Name: "state", Usage: "exchange-prepare 返回的原始 state", MarkRequired: true},
			{Name: "save", Kind: corecmd.KindBool, Usage: "核验身份并保存凭证，保留已有默认账号，不回显 Token"},
		},
		PostMount: func(cmd *cobra.Command) {
			cmd.Args = cobra.NoArgs
			cmd.MarkFlagsOneRequired("code", "code-stdin")
			cmd.MarkFlagsMutuallyExclusive("code", "code-stdin")
		},
		Validate: validateOIDCTokenExchangeCommand,
		ResultInvoke: func(c *corecmd.Ctx, _ map[string]any) (output.CommandResult, error) {
			cmd := c.Command()
			defer resetAuthOIDCExchangeInvocationFlags(cmd)
			p, err := oidcExchangeCommandProvider(cmd)
			if err != nil {
				return nil, err
			}
			save, _ := cmd.Flags().GetBool("save")
			if dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run"); dryRun {
				steps := []string{"exchange_code_and_state"}
				if save {
					steps = append(steps, "verify_online_identity", "persist_exact_profile")
				}
				return output.Success(map[string]any{"status": "planned", "save": save, "steps": steps}, output.WithDryRun()), nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			code, _ := cmd.Flags().GetString("code")
			if stdin, _ := cmd.Flags().GetBool("code-stdin"); stdin {
				input, err := readOIDCCodeInput(ctx, cmd.InOrStdin())
				if err != nil {
					if apperrors.PreserveClassification(err) {
						return nil, err
					}
					return nil, apperrors.NewValidation("cannot read authorization code from stdin")
				}
				code = strings.TrimSuffix(string(input), "\n")
				if strings.HasSuffix(string(input), "\r\n") {
					code = strings.TrimSuffix(code, "\r")
				}
			}
			state, _ := cmd.Flags().GetString("state")
			if !save {
				data, err := p.Exchange(ctx, code, state)
				if err != nil {
					return nil, err
				}
				return output.Success(data), nil
			}
			tokenCaller, ok := caller.(tokenOverrideToolCaller)
			if !ok {
				return nil, apperrors.NewInternal("request-scoped identity lookup unavailable")
			}
			dir := defaultConfigDir()
			data, err := p.ExchangeAndSave(ctx, dir, code, state, func(ctx context.Context, token, corp string) (authpkg.ManagedIdentity, error) {
				result, err := tokenCaller.CallToolWithToken(ctx, token, "contact", "get_current_user_profile", nil)
				if err != nil {
					return authpkg.ManagedIdentity{}, err
				}
				return authpkg.ManagedIdentityFromToolResult(result, corp)
			})
			if err != nil {
				return nil, err
			}
			ResetRuntimeTokenCache()
			clearCompatCache()
			cfg, err := authpkg.LoadProfiles(dir)
			if err != nil {
				return nil, apperrors.NewInternal("profile saved but current profile readback failed")
			}
			profile := authpkg.TokenProfileSelector(data)
			return output.Success(map[string]any{"status": "profile_saved", "dwsProfile": profile, "corpId": data.CorpID, "userId": data.UserID, "expiresAt": data.ExpiresAt.UTC().Format(time.RFC3339), "currentProfile": cfg.CurrentProfile, "isCurrentProfile": profile == cfg.CurrentProfile}), nil
		},
	})
}

func validateOIDCTokenExchangeCommand(cmd *cobra.Command, _ []string) error {
	p, err := oidcExchangeCommandProvider(cmd)
	if err != nil {
		return err
	}
	code, _ := cmd.Flags().GetString("code")
	stdin, _ := cmd.Flags().GetBool("code-stdin")
	if stdin && cmd.Flags().Changed("code") {
		return apperrors.NewValidation("--code and --code-stdin are mutually exclusive")
	}
	if stdin {
		code = "stdin-pending"
	}
	state, _ := cmd.Flags().GetString("state")
	return p.ValidateExchange(code, state)
}

func oidcExchangeCommandProvider(cmd *cobra.Command) (*authpkg.OIDCLoginProvider, error) {
	provider, _ := cmd.Flags().GetString("provider")
	p := authpkg.NewOIDCLoginProvider(provider, authpkg.GetMCPBaseURL(), nil)
	id, secret := cmd.Flag("client-id"), cmd.Flag("client-secret")
	idSet, secretSet := id != nil && id.Changed, secret != nil && secret.Changed
	if cmd.Name() == "exchange-prepare" && (idSet || secretSet) {
		return nil, apperrors.NewValidation("application credentials belong to auth token-exchange, not exchange-prepare")
	}
	if idSet != secretSet {
		return nil, apperrors.NewValidation("--client-id and --client-secret must be provided together")
	}
	if idSet {
		p.ExchangeCredentials = &authpkg.OIDCExchangeCredentials{ClientID: id.Value.String(), ClientSecret: secret.Value.String()}
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func isAuthOIDCExchangeCommand(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Parent() != nil && cmd.Parent().Name() == "auth" && (cmd.Name() == "exchange-prepare" || cmd.Name() == "token-exchange")
}

func resetAuthOIDCExchangeInvocationFlags(cmd *cobra.Command) {
	if !isAuthOIDCExchangeCommand(cmd) {
		return
	}
	for _, name := range []string{"provider", "code", "code-stdin", "state", "save"} {
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
