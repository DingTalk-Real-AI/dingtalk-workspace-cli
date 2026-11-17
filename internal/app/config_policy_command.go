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
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cmdpolicy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func newConfigPolicyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "policy",
		Short:             "指令裁剪策略",
		Long:              "查看当前二进制内置的指令裁剪策略。策略由发行方编译进二进制，本机无法修改。",
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(cmd, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	cmd.AddCommand(newConfigPolicyShowCommand())
	return cmd
}

// newConfigPolicyShowCommand reports the command restriction policy bound to
// the running command tree. It is read-only, makes no network call, and stays
// available when the policy is active or invalid so a denial can be diagnosed.
func newConfigPolicyShowCommand() *cobra.Command {
	return corecmd.New(corecmd.Spec{
		Use:   "show",
		Short: "查看当前生效的指令裁剪策略",
		Long: `显示当前二进制内置的指令裁剪策略：状态、规则、被拒绝的命令、通道入口状态和诊断提示。

state 取值：
  disabled  未启用裁剪，行为与标准版一致
  active    裁剪生效；未被任何规则放行的命令会被拒绝
  invalid   策略非法；除 help、version 和本命令外全部拒绝

本命令只读取本机状态，不发起网络请求。`,
		Example:       "  dws config policy show --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: corecmd.ContractDecl{
			Description: "显示当前二进制内置的指令裁剪策略：状态、规则、被拒绝的命令、通道入口状态和诊断提示；只读取本机状态，不发起网络请求",
			Identity: contract.ToolIdentitySpec{
				ProductID: "config", Name: "policy_show", CanonicalPath: "config.policy_show",
				CLIPath: "config policy show", PrimaryCLIPath: "config policy show",
			},
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeLocal, Availability: contract.InterfaceAvailable,
				Reason: "命令仅读取本进程内的指令裁剪策略快照，不调用 MCP 接口",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看当前二进制的指令裁剪策略，以及哪些命令被拒绝和原因",
				UseWhen: []string{
					"命令返回 command_denied，需要确认是哪条规则拒绝以及当前放行范围",
					"开始任务前需要确认当前二进制是否启用了指令裁剪",
				},
				AvoidWhen: []string{
					"查看某条命令的参数和用法时使用 dws schema，不要使用本命令",
					"本命令不能修改策略；策略由发行方编译进二进制",
				},
				Examples: []string{"dws config policy show --format json"},
			},
			Result: configPolicyShowResultSpec(),
		},
		PostMount: func(cmd *cobra.Command) { cmd.Args = cobra.NoArgs },
		ResultInvoke: func(c *corecmd.Ctx, _ map[string]any) (output.CommandResult, error) {
			return output.Success(commandPolicyReport(cmdpolicy.For(c.Command()))), nil
		},
	})
}

// configPolicyShowResultSpec is the reviewed return contract of
// `dws config policy show`. It must stay in step with commandPolicyReport; the
// app-level test validates a live report against this schema.
func configPolicyShowResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"当前命令树绑定的指令裁剪策略诊断报告",
			"properties":{
				"state":{"type":"string","enum":["disabled","active","invalid"],"description":"策略状态：disabled 未启用；active 生效；invalid 策略非法，除 help、version 和本命令外全部拒绝"},
				"source":{"type":"string","description":"提供策略的发行版名称；未启用时为空"},
				"invalid_reason":{"type":"string","description":"state 为 invalid 时的原因；其他状态为空"},
				"extensions_disabled":{"type":"boolean","description":"是否因策略启用而停止加载运行时插件、插件 Hook 和用户自定义 shortcut"},
				"rules":{"type":"array","description":"生效的规则；任一条规则放行即放行","items":{
					"type":"object",
					"description":"一条裁剪规则；规则内各条件须同时满足",
					"properties":{
						"name":{"type":"string","description":"规则名，规则集内唯一"},
						"description":{"type":"string","description":"规则说明，仅展示"},
						"deny_message":{"type":"string","description":"发行方为本条规则配置的拒绝指引，会附加到拒绝错误的 hint 和 details；未配置时为空"},
						"allow":{"type":"array","description":"允许的命令路径 glob；为空表示不限制路径","items":{"type":"string"}},
						"deny":{"type":"array","description":"拒绝的命令路径 glob；仅在本条规则内优先于 allow","items":{"type":"string"}},
						"max_effect":{"type":"string","description":"effect 上限（含）：read、write、destructive；为空不设上限"},
						"max_risk":{"type":"string","description":"risk 上限（含）：low、medium、high；为空不设上限"},
						"allow_unannotated":{"type":"boolean","description":"是否放行缺少 effect 或 risk 声明的命令"},
						"allow_channels":{"type":"array","description":"本条规则点名开放的通道入口精确路径","items":{"type":"string"}}
					},
					"required":["name","deny_message","allow","deny","max_effect","max_risk","allow_unannotated","allow_channels"],
					"additionalProperties":false
				}},
				"denied_count":{"type":"integer","description":"被拒绝的可执行入口数量"},
				"denied_paths":{"type":"array","description":"被拒绝的可执行入口，按路径排序","items":{
					"type":"object",
					"description":"一个被拒绝的命令入口",
					"properties":{
						"path":{"type":"string","description":"命令路径，以 / 分隔且不含 dws 根名"},
						"reason_code":{"type":"string","description":"稳定的拒绝原因码"},
						"reason":{"type":"string","description":"拒绝原因说明"},
						"rule_name":{"type":"string","description":"作出该拒绝的规则名；由多条规则共同拒绝或与规则无关时为空"}
					},
					"required":["path","reason_code","reason","rule_name"],
					"additionalProperties":false
				}},
				"channels":{"type":"array","description":"通道入口（通用调用、插件管理、自更新命令）及其状态","items":{
					"type":"object",
					"description":"一个通道入口的状态",
					"properties":{
						"path":{"type":"string","description":"通道入口的命令路径"},
						"allowed":{"type":"boolean","description":"是否被规则点名开放"},
						"rule_name":{"type":"string","description":"开放它的规则名；未开放时为空"}
					},
					"required":["path","allowed","rule_name"],
					"additionalProperties":false
				}},
				"hints":{"type":"array","description":"规则集的非阻断诊断提示","items":{
					"type":"object",
					"description":"一条诊断提示",
					"properties":{
						"kind":{"type":"string","description":"提示类型：deny_overridden 或 rule_allows_all"},
						"path":{"type":"string","description":"相关命令路径；与具体命令无关时为空"},
						"rule":{"type":"string","description":"相关规则名"},
						"other_rule":{"type":"string","description":"deny_overridden 中放行该命令的规则名；其他类型为空"},
						"message":{"type":"string","description":"提示说明"}
					},
					"required":["kind","path","rule","other_rule","message"],
					"additionalProperties":false
				}}
			},
			"required":["state","source","invalid_reason","extensions_disabled","rules","denied_count","denied_paths","channels","hints"],
			"additionalProperties":false
		}`),
	}
}

// commandPolicyReport projects a policy snapshot into the diagnostic payload.
// Every list is always present (possibly empty) so consumers need no nil
// handling.
func commandPolicyReport(snapshot *cmdpolicy.Snapshot) map[string]any {
	rules := make([]map[string]any, 0)
	for _, rule := range snapshot.Rules() {
		rules = append(rules, map[string]any{
			"name":              rule.Name,
			"description":       rule.Description,
			"deny_message":      rule.DenyMessage,
			"allow":             nonNilStrings(rule.Allow),
			"deny":              nonNilStrings(rule.Deny),
			"max_effect":        rule.MaxEffect,
			"max_risk":          rule.MaxRisk,
			"allow_unannotated": rule.AllowUnannotated,
			"allow_channels":    nonNilStrings(rule.AllowChannels),
		})
	}
	denied := make([]map[string]any, 0)
	for _, item := range snapshot.DeniedPaths() {
		denied = append(denied, map[string]any{
			"path":        item.Path,
			"reason_code": item.ReasonCode,
			"reason":      item.Reason,
			"rule_name":   item.RuleName,
		})
	}
	channels := make([]map[string]any, 0)
	for _, channel := range snapshot.Channels() {
		channels = append(channels, map[string]any{
			"path":      channel.Path,
			"allowed":   channel.Allowed,
			"rule_name": channel.RuleName,
		})
	}
	hints := make([]map[string]any, 0)
	for _, hint := range snapshot.Hints() {
		hints = append(hints, map[string]any{
			"kind":       hint.Kind,
			"path":       hint.Path,
			"rule":       hint.Rule,
			"other_rule": hint.OtherRule,
			"message":    hint.Message,
		})
	}
	return map[string]any{
		"state":          string(snapshot.State()),
		"source":         snapshot.Source(),
		"invalid_reason": snapshot.InvalidReason(),
		// While the policy is enabled, runtime plugins, plugin hooks, and
		// user-defined shortcuts are not loaded.
		"extensions_disabled": snapshot.Enabled(),
		"rules":               rules,
		"denied_count":        len(denied),
		"denied_paths":        denied,
		"channels":            channels,
		"hints":               hints,
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
