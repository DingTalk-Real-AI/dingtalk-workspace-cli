// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package pat registers safe Shortcut routing for PAT authorization tasks.
package pat

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	patcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var safeAgentCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var (
	patBrowserPolicyConfigDir = patcore.BrowserPolicyConfigDir
	patSetBrowserPolicy       = patcore.SetBrowserPolicy
	patReadBrowserPolicy      = patcore.ReadStoredBrowserPolicy
)

func browserPolicyResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"本地 PAT 浏览器策略预览或已验证写入结果","properties":{"scope":{"type":"string","description":"写入范围：default 或 agent","enum":["default","agent"]},"openBrowser":{"type":"boolean","description":"策略是否允许打开浏览器"},"executed":{"type":"boolean","description":"本次是否实际写入本地策略"},"verified":{"type":"boolean","description":"是否对同一目标完成磁盘读回验证"}},"required":["scope","openBrowser","executed","verified"],"additionalProperties":false}`),
	}
}

func authorizeResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"由 routed 原子命令 pat chmod 持有的 PAT 授权计划或逐 scope 终态账本","properties":{"selected":{"type":"integer","description":"计划选择的 scope 数量"},"skipped":{"type":"integer","description":"计划跳过的 scope 数量"},"pending":{"type":"integer","description":"仍待授权的 scope 数量"},"verified":{"type":"boolean","description":"授权身份与逐 scope 终态是否已验证"}},"required":["selected","skipped","pending","verified"],"additionalProperties":false}`),
	}
}

var BrowserPolicy = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "pat",
	Command:       "+browser-policy",
	Product:       "pat",
	Description:   "安全配置 PAT 授权时是否允许打开本地浏览器",
	Intent:        "需要为默认策略或一个明确 Agent 配置 PAT 撞墙时是否允许打开本地浏览器时使用；支持无写入预览，实际写入需确认并对同一策略项完成磁盘读回。",
	Risk:          shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "pat", Name: "shortcut_browser_policy", CanonicalPath: "pat.shortcut_browser_policy",
			CLIPath: "pat +browser-policy", PrimaryCLIPath: "pat +browser-policy",
		},
		Description: "安全配置 PAT 授权时是否允许打开本地浏览器",
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
			Reason: "The Shortcut writes only the local PAT policy file, requires confirmation, supports a no-write request preview, omits agent identity from output, and verifies the exact persisted target.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "安全配置 PAT 授权时是否允许打开本地浏览器",
			UseWhen:      []string{"需要为默认策略或一个明确 Agent 配置 PAT 撞墙时是否允许打开本地浏览器时使用；支持无写入预览，实际写入需确认并对同一策略项完成磁盘读回。"},
			AvoidWhen:    []string{"需要预览或授予行为 scope 时使用原子命令 pat chmod；普通 OAuth 登录使用 auth"},
			Examples:     []string{`dws pat +browser-policy --enabled=false --dry-run --format json`},
		},
		Parameters: []contract.ParamDecl{
			{Name: "enabled"}, {Name: "agent-code"},
		},
		DryRun: &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
		Result: browserPolicyResult(),
	},
	Flags: []shortcut.Flag{
		{Name: "enabled", Type: shortcut.FlagBool, Required: true, Desc: "是否允许打开本地浏览器；必须显式传 true 或 false"},
		{Name: "agent-code", Type: shortcut.FlagString, Desc: "可选 Agent 标识必须为 1 到 64 位字母、数字、下划线或连字符；只用于定位本地策略项且不会出现在结果中"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"agent-code"}, Description: "可选 Agent 标识必须为 1 到 64 位字母、数字、下划线或连字符"},
	},
	Tips: []string{`dws pat +browser-policy --enabled=false --dry-run --format json`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		value := strings.TrimSpace(rt.Str("agent-code"))
		if value != "" && !safeAgentCodePattern.MatchString(value) {
			return apperrors.NewValidation("--agent-code 格式无效；只允许 1 到 64 位字母、数字、下划线或连字符")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		scope := "default"
		agentCode := strings.TrimSpace(rt.Str("agent-code"))
		if agentCode != "" {
			scope = "agent"
		}
		enabled := rt.Bool("enabled")
		if rt.DryRun() {
			return rt.Output(map[string]any{"scope": scope, "openBrowser": enabled, "executed": false, "verified": false})
		}
		configDir := patBrowserPolicyConfigDir()
		written, err := patSetBrowserPolicy(configDir, agentCode, enabled)
		if err != nil {
			return err
		}
		stored, err := patReadBrowserPolicy(configDir, agentCode)
		if err != nil {
			return err
		}
		if written.Scope != scope || stored.Scope != scope || written.OpenBrowser != enabled || stored.OpenBrowser != enabled || written.AgentCode != stored.AgentCode {
			return apperrors.NewInternal("PAT 浏览器策略写后读回不一致；结果未验证")
		}
		return rt.Output(map[string]any{"scope": scope, "openBrowser": enabled, "executed": true, "verified": true})
	},
}

var Authorize = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "pat",
	Command:       "+authorize",
	Product:       "pat",
	Description:   "将 PAT 行为 scope 授权路由到原子命令 pat chmod",
	Intent:        "命令提示缺少 PAT 行为授权时 classified=routed：先使用 pat chmod --dry-run 审核计划，用户确认后再由同一原子命令执行；+authorize 本身保持 unavailable。",
	Risk:          shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID: "pat", Name: "shortcut_authorize", CanonicalPath: "pat.shortcut_authorize",
			CLIPath: "pat +authorize", PrimaryCLIPath: "pat +authorize",
		},
		Description: "将 PAT 行为 scope 授权路由到原子命令 pat chmod",
		Interface: &contract.InterfaceSpec{
			Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceUnavailable,
			Reason: "classified=routed; pat chmod already owns the reviewed plan, fallback, pending, per-scope ledger, identity, and confirmation chain. The Shortcut runtime cannot reuse that Cobra/ToolCaller closure without duplicating safety semantics, and a live write remains blocked_fixture, so +authorize stays unavailable while routing to pat chmod.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "将 PAT 行为 scope 授权路由到原子命令 pat chmod",
			UseWhen:      []string{"命令提示缺少 PAT 行为授权时 classified=routed：先使用 pat chmod --dry-run 审核计划，用户确认后再由同一原子命令执行；+authorize 本身保持 unavailable。"},
			AvoidWhen:    []string{"不要向此 unavailable Shortcut 传入任何真实身份、会话、scope 或授权材料；本地浏览器策略使用 pat +browser-policy"},
			Examples:     []string{`dws pat +authorize --format json`},
		},
		Result: authorizeResult(),
	},
	Hidden:       true,
	Availability: shortcut.AvailabilityUnavailable,
	Execute: func(*shortcut.RuntimeContext) error {
		return apperrors.NewValidation("PAT 授权任务 classified=routed；+authorize 当前 unavailable，请使用审核后的原子命令 pat chmod，并先执行 --dry-run")
	},
}

func init() {
	shortcut.Register(BrowserPolicy, Authorize)
}
