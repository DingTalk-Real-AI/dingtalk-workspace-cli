// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package app

import (
	"context"
	"net/url"
	"strings"
	"sync"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/netpolicy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/spf13/cobra"
)

func init() {
	for _, item := range []configmeta.ConfigItem{
		{Name: "DWS_PROXY", Category: configmeta.CategoryNetwork, Description: "认证/MCP/OpenAPI/SafeChat/localio 出口：auto、direct 或代理 URL", DefaultValue: "auto", Sensitive: true},
		{Name: "DWS_NO_PROXY", Category: configmeta.CategoryNetwork, Description: "直连目标 CSV；显式空值清空绕过列表"},
		{Name: "DWS_CA_BUNDLE", Category: configmeta.CategoryNetwork, Description: "追加到系统根证书池的 PEM 文件"},
	} {
		configmeta.Register(item)
	}
}

// networkInvocation captures file, environment and flag inputs once. Profile
// selection can then vary without rereading environment or sharing pools.
type networkInvocation struct {
	file      netpolicy.File
	env       netpolicy.Environment
	overrides netpolicy.Overrides
	dir       string
	policies  map[string]*netpolicy.Policy
	mu        sync.Mutex
}

func (n *networkInvocation) close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, p := range n.policies {
		p.CloseIdleConnections()
	}
}

func networkProfile(dir, selector string) (string, error) {
	if strings.Contains(selector, ",") {
		return "", nil
	} // runner resolves each selection
	p, err := profilemetadata.ResolveReadOnly(dir, selector)
	if err != nil {
		return "", err
	}
	if p == nil {
		if selector != "" {
			return "", apperrors.NewValidation("指定的 profile 没有可用的本地身份元数据")
		}
		return "", nil
	}
	return profilemetadata.IdentitySelector(p.CorpID, p.UserID), nil
}

func (n *networkInvocation) policy(selector string) (*netpolicy.Policy, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	key := ""
	if len(n.file.Profiles) > 0 {
		var err error
		key, err = networkProfile(n.dir, selector)
		if err != nil {
			return nil, err
		}
	}
	if p := n.policies[key]; p != nil {
		return p, nil
	}
	p, err := netpolicy.Resolve(n.file.Settings(key), n.overrides, n.env)
	if err == nil {
		n.policies[key] = p
	}
	return p, err
}

// Proxy overrides are request-scoped because they can contain credentials.
// Capture the distinction between absent and explicitly empty before clearing.
func consumeNetworkOverrides(root *cobra.Command) netpolicy.Overrides {
	var out netpolicy.Overrides
	for _, entry := range []struct {
		name string
		dest **string
	}{{"proxy", &out.Proxy}, {"no-proxy", &out.NoProxy}} {
		f := root.PersistentFlags().Lookup(entry.name)
		if f == nil || !f.Changed {
			continue
		}
		value := f.Value.String()
		*entry.dest = &value
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	return out
}

func configureNetworkInvocation(cmd *cobra.Command, flags *GlobalFlags, o netpolicy.Overrides) error {
	flags.network.close()
	flags.network = nil
	// Editing must remain possible even if an old network configuration cannot
	// be resolved (e.g. its CA file was moved). All writes validate in RunE.
	parent := cmd.Parent()
	isNetworkCommand := parent != nil && parent.Name() == "network" && parent.Parent() != nil && parent.Parent().Name() == "config"
	if isNetworkCommand && (cmd.Name() == "set" || cmd.Name() == "reset") {
		if cmd.Name() == "set" {
			flags.network = &networkInvocation{overrides: o}
		}
		return nil
	}
	dir := defaultConfigDir()
	f, err := netpolicy.Load(dir)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if isNetworkCommand && strings.Contains(flags.Profile, ",") {
		return apperrors.NewValidation("网络配置诊断只支持单个 profile")
	}
	// The saved view must work even when the current shell overrides are
	// invalid. Resolve it using only the same file snapshot.
	if isNetworkCommand && cmd.Name() == "show" && !corecmd.BoolFlag(cmd, "effective") {
		key, err := networkProfile(dir, flags.Profile)
		if err != nil {
			return apperrors.NewValidation(err.Error())
		}
		p, err := netpolicy.Resolve(f.Settings(key), netpolicy.Overrides{}, nil)
		if err != nil {
			return apperrors.NewValidation(err.Error())
		}
		cmd.SetContext(netpolicy.WithPolicy(cmd.Context(), p))
		return nil
	}
	// A typo must fail even without any profile-specific settings.
	if isNetworkCommand && flags.Profile != "" && len(f.Profiles) == 0 {
		if _, err := networkProfile(dir, flags.Profile); err != nil {
			return apperrors.NewValidation(err.Error())
		}
	}
	n := &networkInvocation{file: f, env: netpolicy.SnapshotEnvironment(), overrides: o, dir: dir, policies: map[string]*netpolicy.Policy{}}
	p, err := n.policy(flags.Profile)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	flags.network = n
	cmd.SetContext(netpolicy.WithPolicy(cmd.Context(), p))
	return nil
}

func (r *runtimeRunner) withNetworkContext(ctx context.Context, profile string) (context.Context, error) {
	if r == nil || r.globalFlags == nil || r.globalFlags.network == nil {
		return ctx, nil
	}
	p, err := r.globalFlags.network.policy(profile)
	if err != nil {
		return ctx, apperrors.NewValidation(err.Error())
	}
	return netpolicy.WithPolicy(ctx, p), nil
}

func networkContract(name, description string) corecmd.ContractDecl {
	path := "config network " + name
	example := "dws " + path
	switch name {
	case "show":
		example += " --effective"
	case "set":
		example += " --proxy auto --dry-run"
	case "reset":
		example += " --dry-run"
	case "explain":
		example += " --url https://storage.example.com/resource"
	}
	c := corecmd.ContractDecl{
		Description: description,
		Identity:    contract.ToolIdentitySpec{ProductID: "config", Name: "network_" + name, CanonicalPath: "config.network_" + name, CLIPath: path, PrimaryCLIPath: path},
		Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "命令仅计算或保存本地网络配置，不调用业务 RPC 或联网探测"},
		Selection:   contract.SelectionSpec{AgentSummary: description, UseWhen: []string{"需要" + description}, AvoidWhen: []string{"本命令不探测网络；不配置尚未接入的 Stream、白板或安装器"}, Examples: []string{example}},
	}
	if name == "set" || name == "reset" {
		c.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false}
	}
	return c
}

func newNetworkConfigCommand(flags *GlobalFlags) *cobra.Command {
	group := &cobra.Command{Use: "network", Short: "配置核心 HTTP 出口代理", Long: "配置认证、MCP/OpenAPI、SafeChat 和 localio 下载/上传的统一出口。尚未覆盖白板下载、Stream SDK、安装器或其他独立 HTTP 客户端，不能据此假设全部流量经过代理。"}
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling})
	readSafety := contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
	writeSafety := contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"}
	group.AddCommand(corecmd.New(corecmd.Spec{
		Use: "show", Short: "查看网络配置；--effective 显示脱敏生效来源", Safety: readSafety, Contract: networkContract("show", "离线查看网络配置及其生效来源"),
		Flags: []corecmd.FlagSpec{{Name: "effective", Kind: corecmd.KindBool, Usage: "显示最终生效配置及来源（不联网）"}},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.WriteJSON(cmd.OutOrStdout(), netpolicy.FromContext(cmd.Context()).Summary())
		},
	}))
	group.AddCommand(corecmd.New(corecmd.Spec{
		Use: "set", Short: "保存配置；使用全局 --proxy / --no-proxy 设置值", Safety: writeSafety, Contract: networkContract("set", "将非敏感网络配置原子保存到当前 profile；未登录时保存默认值"),
		Flags: []corecmd.FlagSpec{{Name: "ca-bundle", Usage: "追加系统信任的 PEM 文件路径；空值清空"}},
		RunE: func(cmd *cobra.Command, _ []string) error {
			o := flags.network.overrides
			flags.network.overrides = netpolicy.Overrides{}
			return editNetworkConfig(cmd, false, o)
		},
	}))
	group.AddCommand(corecmd.New(corecmd.Spec{
		Use: "reset", Short: "清除当前 profile 的网络配置", Safety: writeSafety, Contract: networkContract("reset", "清除当前 profile 的网络覆盖，恢复默认和环境配置"),
		RunE: func(cmd *cobra.Command, _ []string) error { return editNetworkConfig(cmd, true, netpolicy.Overrides{}) },
	}))
	group.AddCommand(corecmd.New(corecmd.Spec{
		Use: "explain", Short: "离线解释一个目标的出口路由", Safety: readSafety, Contract: networkContract("explain", "计算目标的代理/直连路由，隐藏路径和凭据；不解析 DNS、不联网"),
		Flags: []corecmd.FlagSpec{{Name: "url", Usage: "目标 HTTP(S) URL（仅用于离线路由计算）", Required: true, MarkRequired: true}},
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, _ := cmd.Flags().GetString("url")
			u, err := url.Parse(raw)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return apperrors.NewValidation("需要合法的 HTTP(S) 目标 URL")
			}
			e, err := netpolicy.FromContext(cmd.Context()).Explain(u)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			return output.WriteJSON(cmd.OutOrStdout(), e)
		},
	}))
	return group
}

func editNetworkConfig(cmd *cobra.Command, reset bool, o netpolicy.Overrides) error {
	if !reset && o.Proxy == nil && o.NoProxy == nil && !cmd.Flags().Changed("ca-bundle") {
		return apperrors.NewValidation("请指定 --proxy、--no-proxy 或 --ca-bundle")
	}
	dir := defaultConfigDir()
	if corecmd.BoolFlag(cmd, "dry-run") {
		return output.WriteJSON(cmd.OutOrStdout(), map[string]any{"dry_run": true, "preview_kind": "plan", "executed": false, "reset": reset, "saved": false})
	}
	selector, _ := cmd.Flags().GetString("profile")
	if strings.Contains(selector, ",") {
		return apperrors.NewValidation("网络配置修改只支持单个 profile")
	}
	key, err := networkProfile(dir, selector)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	lock, err := authpkg.AcquireDualLock(cmd.Context(), dir)
	if err != nil {
		return err
	}
	defer lock.Release()
	f, err := netpolicy.Load(dir)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	s := f.Default
	if key != "" {
		s = f.Profiles[key]
	}
	if reset {
		s = netpolicy.Settings{}
	} else {
		if o.Proxy != nil {
			s.Proxy = o.Proxy
		}
		if o.NoProxy != nil {
			s.NoProxy = o.NoProxy
		}
		if cmd.Flags().Changed("ca-bundle") {
			v, _ := cmd.Flags().GetString("ca-bundle")
			s.CABundle = &v
		}
	}
	if key == "" {
		f.Default = s
	} else {
		if f.Profiles == nil {
			f.Profiles = map[string]netpolicy.Settings{}
		}
		if reset {
			delete(f.Profiles, key)
		} else {
			f.Profiles[key] = s
		}
	}
	if err := netpolicy.Save(dir, f); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return output.WriteJSON(cmd.OutOrStdout(), map[string]any{"saved": true, "scope": key, "reset": reset})
}
