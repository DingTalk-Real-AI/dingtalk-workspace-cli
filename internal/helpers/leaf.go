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

package helpers

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cmdcore"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// leaf.go 是叶子命令的统一构建框架。
//
// 现状问题：每个叶子命令手写 cobra.Command，required 校验、别名回退、环境
// 变量回退、值转换、参数装配、派发调用在各个产品文件里各写一份，行为难以
// 保持一致。LeafSpec 把这些共性收敛为声明式结构：命令只声明「flag 集合 +
// 绑定关系」，框架统一执行。默认派发走 MCP 直连（callMCPTool）；非 MCP
// 命令（如 devapp 走 executor.Runner）通过 LeafSpec.Call 注入自己的派发器，
// 复用同一套 flag/校验/装配逻辑。复杂命令可通过 LeafSpec.RunE 完全自定义
// （逃生舱），不在框架适用范围内强行套用。
//
// 收敛纪律（Phase 1）：flag 注册、有效值回退链、required 校验、约束校验/声明
// 检查、Risk 写确认、toolArgs 装配、Runtime Schema 投影、帮助渲染均已下沉到
// internal/cmdcore 共享基座；本文件只保留 LeafSpec 外壳（含 MCP dispatch 字段）
// 与 NewLeafCommand 编排。dispatch（callMCPTool/callMCPToolOnServer/Call）仍留
// 在 helpers。由 check-generated-drift（catalog 零漂移）与命令兼容性检查兜底
// 证明等价——cmdcore 是纯抽取，行为逐字保持不变。
//
// 迁移纪律：从手写命令迁移到 LeafSpec 时，flag 名、默认值、usage 文案、
// MarkFlagRequired、required 错误格式、toolArgs 键与值必须逐字保持一致。

// LeafFlagKind 是 flag 的值类型（cmdcore.FlagKind 的别名）。
type LeafFlagKind = cmdcore.FlagKind

const (
	// LeafString 字符串 flag（默认）。
	LeafString = cmdcore.KindString
	// LeafInt 整型 flag；仅在值 != 0 时进入 toolArgs（putInt 语义）。
	LeafInt = cmdcore.KindInt
	// LeafBool 布尔 flag；仅在用户显式提供（Changed）时进入 toolArgs，显式
	// false 也下发；不参与别名/env 回退链。
	LeafBool = cmdcore.KindBool
	// LeafStringSlice 字符串列表 flag；仅在存在非空元素时进入 toolArgs，元素
	// 恒 TrimSpace 后过滤空串。
	LeafStringSlice = cmdcore.KindStringSlice
)

// LeafFlag 声明一个 flag 的注册方式与到 MCP toolArgs 的绑定
// （cmdcore.FlagSpec 的别名，字段含义见 cmdcore 定义）。
type LeafFlag = cmdcore.FlagSpec

// LeafRisk 声明叶子命令的副作用等级（cmdcore.Risk 的别名）。取值与 shortcut
// 框架的 Risk 逐字一致。
type LeafRisk = cmdcore.Risk

const (
	// LeafRiskRead 只读操作，从不提示。空值即视为 LeafRiskRead。
	LeafRiskRead = cmdcore.RiskRead
	// LeafRiskWrite 变更状态；未加 --yes 时提示确认。
	LeafRiskWrite = cmdcore.RiskWrite
	// LeafRiskHighWrite 破坏性/不可逆操作；未加 --yes 时提示确认。
	LeafRiskHighWrite = cmdcore.RiskHighWrite
)

// LeafConstraintKind 是跨 flag 关系约束的类型（cmdcore.ConstraintKind 的
// 别名）。取值与 shortcut 框架的 ConstraintKind 逐字一致。
type LeafConstraintKind = cmdcore.ConstraintKind

const (
	// LeafAtLeastOne 要求 Flags 至少提供一个。
	LeafAtLeastOne = cmdcore.AtLeastOne
	// LeafExactlyOne 要求 Flags 必须且只能提供一个。
	LeafExactlyOne = cmdcore.ExactlyOne
	// LeafMutuallyExclusive 允许 Flags 中最多提供一个。
	LeafMutuallyExclusive = cmdcore.MutuallyExclusive
)

// LeafConstraint 声明一组 flag 的关系约束（cmdcore.Constraint 的别名）。框架
// 在 required 校验之后、Validate 钩子之前统一执行；「是否提供」的判定复用有效
// 值回退链（显式主 flag → 别名 → env），即只传兼容别名同样视为已提供。约束
// 同时投影到 Agent Runtime Schema 并渲染进 --help 的「参数约束」段。
type LeafConstraint = cmdcore.Constraint

// LeafSpec 声明一个 MCP 直连叶子命令。契约字段（Flags/Constraints/Risk）由
// cmdcore 共享基座统一处理；dispatch 字段（Server/Tool/Call/RunE）与
// PostMount/Validate 编排逻辑留在 helpers。
type LeafSpec struct {
	Use     string
	Short   string
	Long    string
	Example string

	// Server 非空时走 callMCPToolOnServer（显式 server 路由），否则走
	// callMCPTool（按 product 路由）。Call 非空时两者都被忽略。
	Server string
	Tool   string
	Flags  []LeafFlag
	// Constraints 是跨 flag 的关系约束（至少一个 / 恰好一个 / 互斥），由
	// cmdcore 统一校验并投影到 Runtime Schema 与 --help。复杂的条件式校验
	// 仍放 Validate 钩子。
	Constraints []LeafConstraint

	// Risk 声明副作用等级，驱动执行前的写确认（对齐 shortcut 框架的
	// Risk 语义）。默认 LeafRiskRead：只读，从不提示。LeafRiskWrite /
	// LeafRiskHighWrite 在未加 --yes 且非 --dry-run 时提示确认，用户拒绝
	// 则中止且不派发。
	Risk LeafRisk

	// Call 是可插拔派发函数，非空时替代默认的 callMCPTool/callMCPToolOnServer。
	// 供非 MCP 直连命令（如 devapp 走 executor.Runner）复用本框架：调用方用
	// 闭包捕获自己的 runner/派发器即可。签名与默认路径一致——收到框架装配好
	// 的 toolArgs，自行派发。
	Call func(cmd *cobra.Command, tool string, args map[string]any) error

	// Validate 是跨 flag 校验钩子（如时间区间、互斥关系），在 required
	// 校验之后、toolArgs 装配之前执行；nil 时跳过。单 flag 的格式转换
	// 应放在 LeafFlag.Transform，不要放进 Validate。
	Validate func(cmd *cobra.Command, args []string) error

	// RunE 非空时完全自定义执行体（逃生舱），框架只负责注册 flag。
	RunE func(cmd *cobra.Command, args []string) error

	// PostMount 在 flag 注册完成之后、RunE 设定之前对构建好的 cmd 做最终
	// 调整（设置 Args/DisableAutoGenTag、调用 annotate/preferLegacy 等）。
	// 无论是否使用 RunE 逃生舱都会执行。对标 lark shortcut 的 PostMount。
	PostMount func(cmd *cobra.Command)
}

// NewLeafCommand 按 LeafSpec 构建叶子命令：flag 注册、约束声明检查、Schema
// 投影、帮助渲染、required/约束校验、Risk 写确认、toolArgs 装配全部委托
// cmdcore；本函数只负责编排顺序与 MCP dispatch（callMCPTool/OnServer/Call）。
func NewLeafCommand(spec LeafSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.Use,
		Short:   spec.Short,
		Long:    spec.Long,
		Example: spec.Example,
	}
	cmdcore.RegisterFlags(cmd, spec.Flags)
	cmdcore.ValidateConstraintDecls(spec.Use, spec.Flags, spec.Constraints)
	cmdcore.AnnotateConstraints(cmd, spec.Constraints)
	if help := cmdcore.ConstraintHelp(spec.Constraints); help != "" {
		cmd.Long = strings.TrimRight(cmd.Long, "\n") + help
	}
	if spec.PostMount != nil {
		spec.PostMount(cmd)
	}
	if spec.RunE != nil {
		cmd.RunE = spec.RunE
		return cmd
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := cmdcore.ValidateRequired(cmd, spec.Flags); err != nil {
			return err
		}
		if err := cmdcore.ValidateConstraints(cmd, spec.Flags, spec.Constraints); err != nil {
			return err
		}
		if spec.Validate != nil {
			if err := spec.Validate(cmd, args); err != nil {
				return err
			}
		}
		toolArgs, err := cmdcore.BuildArgs(cmd, spec.Flags)
		if err != nil {
			return err
		}
		if !cmdcore.ConfirmRisk(cmd, spec.Risk) {
			return apperrors.NewValidation("用户取消了操作")
		}
		if spec.Call != nil {
			return spec.Call(cmd, spec.Tool, toolArgs)
		}
		if spec.Server != "" {
			return callMCPToolOnServer(spec.Server, spec.Tool, toolArgs)
		}
		return callMCPTool(spec.Tool, toolArgs)
	}
	return cmd
}
