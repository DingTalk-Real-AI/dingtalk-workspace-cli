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
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// contractConfirmSafetyAnnotation marks leaves whose RunE was wrapped by
// DeclareLeafMetadata with the same SafetySpec published to ContractFinal.
const contractConfirmSafetyAnnotation = "dws.contract.confirm_safety"

// contractValidateAnnotation marks leaves that installed DeclareLeafMetadata
// Validate. Validate runs inside the same RunE wrapper as ConfirmSafety (not
// PreRunE), so direct RunE / proxySubCmd call sites cannot skip it.
const contractValidateAnnotation = "dws.contract.validate"

// contractConfirmDeferredAnnotation marks user_required leaves that defer
// ConfirmSafety to the first deps.Caller.CallTool (no Validate). Local-checkable
// commands without an MCP caller must use Validate instead.
const contractConfirmDeferredAnnotation = "dws.contract.confirm_deferred"

// leaf.go 是叶子命令的统一构建框架（命令框架的 Leaf 门面）。
//
// 两种正式模式（共用 LeafSpec 词汇，产出同一 ContractFinal）：
//
//   - 完全托管模式 NewLeafCommand：声明 + 执行都归 corecmd（flag 注册、
//     参数投影、ConfirmSafety、派发）。新命令默认走此模式。
//   - 声明元数据模式 DeclareLeafMetadata：声明 Safety + Contract（AttachContract），
//     不注册 flag、不接管参数投影；可选 Validate 与 ConfirmSafety 同挂在
//     RunE 包装器内（Validate 在前，不是 PreRunE——直接调 RunE /
//     proxySubCmd 会跳过 PreRunE）。当 Safety.Confirmation=user_required 时
//     用**同一份** SafetySpec 包一层 ConfirmSafety，保证执行门禁与 Catalog
//     同源。用于执行体必须冻结的既有命令补声明，是迁移态。
//
// 每个 API 各自声明：
//
//   - 禁止参数化命令工厂用 use/short/tool 拼出多个真 API 的
//     *cobra.Command（含共享 Use/Short/Long/Example/RunE/DeclareLeafMetadata）。
//   - 允许私有 RunE 执行 helper（不返回 *cobra.Command），以及 Safety /
//     Interface 字面量缩写（声明调用点仍须写在每个命令旁）。
//   - hint/proxy 重定向不是业务叶子，不要求 ContractFinal。
//
// 声明 vs 执行（框架规则，详见 RFC §5.0 / 同源文档 §1.2）：
//
//   - 声明 = LeafSpec 数据字段：Flags、Constraints、Safety、ConstParams、
//     Use/Short/Long/Example。经 FromLeafSpec → corecmd.New 注册并
//     嵌入 dws.schema.*。
//   - 执行 = Validate / Call / RunE / PostMount。钩子消费已装配参数，不得
//     发明 CLI 表面（业务 flag/params）。
//   - 标注 = 声明字段装不下时的显式注解（如 write-guard 的 runtime_gate）。
//
// LeafSpec 把共性收敛为声明式结构：命令声明 flag 集合与绑定，框架统一校验、
// 装配与投影。默认派发走 MCP 直连；非 MCP 命令通过 Call 注入派发器。复杂
// 命令可用 RunE 逃生舱。
//
// 收敛纪律（Phase 2）：flag 注册、有效值回退链、required/约束、Safety 确认、
// toolArgs 装配、Schema 投影与帮助渲染均在 internal/corecmd；本文件只做
// LeafSpec→CommandSpec 映射与 dispatch 闭包。等价性由 catalog 漂移门禁与
// leaf/risk/约束单测共同兜底。
//
// 迁移纪律：迁入 LeafSpec 时 flag 名、默认值、usage、MarkFlagRequired、
// required 错误格式、toolArgs 键与值必须逐字保持一致。

// LeafFlagKind 是 flag 的值类型（corecmd.FlagKind 的别名）。
type LeafFlagKind = corecmd.FlagKind

const (
	// LeafString 字符串 flag（默认）。
	LeafString = corecmd.KindString
	// LeafInt 整型 flag；仅在值 != 0 时进入 toolArgs（putInt 语义）。
	LeafInt = corecmd.KindInt
	// LeafBool 布尔 flag；仅在用户显式提供（Changed）时进入 toolArgs，显式
	// false 也下发；不参与别名/env 回退链。
	LeafBool = corecmd.KindBool
	// LeafStringSlice 字符串列表 flag；仅在存在非空元素时进入 toolArgs，元素
	// 恒 TrimSpace 后过滤空串。
	LeafStringSlice = corecmd.KindStringSlice
)

// LeafFlag 声明一个 flag 的注册方式与到 MCP toolArgs 的绑定
// （corecmd.FlagSpec 的别名，字段含义见 command 定义）。
type LeafFlag = corecmd.FlagSpec

// LeafConstraintKind 是跨 flag 关系约束的类型（corecmd.ConstraintKind 的
// 别名）。取值与 shortcut 框架的 ConstraintKind 逐字一致。
type LeafConstraintKind = corecmd.ConstraintKind

const (
	// LeafAtLeastOne 要求 Flags 至少提供一个。
	LeafAtLeastOne = corecmd.AtLeastOne
	// LeafExactlyOne 要求 Flags 必须且只能提供一个。
	LeafExactlyOne = corecmd.ExactlyOne
	// LeafMutuallyExclusive 允许 Flags 中最多提供一个。
	LeafMutuallyExclusive = corecmd.MutuallyExclusive
)

// LeafConstraint 声明一组 flag 的关系约束（corecmd.Constraint 的别名）。框架
// 在 required 校验之后、Validate 钩子之前统一执行；「是否提供」的判定复用有效
// 值回退链（显式主 flag → 别名 → env），即只传兼容别名同样视为已提供。约束
// 同时投影到 Agent Runtime Schema 并渲染进 --help 的「参数约束」段。
type LeafConstraint = corecmd.Constraint

// LeafContract 是叶子 Contract 声明（corecmd.ContractDecl 别名）。
// 嵌套字段直接使用 contract.*（InterfaceSpec / ParamDecl / SelectionSpec 等），
// 不再保留平行 Decl 类型。
type LeafContract = corecmd.ContractDecl

// LeafSpec 是命令框架的 Leaf 声明门面（映射为 corecmd.Spec）。
//
// 声明面 = Contract 最终数据源：Flags（含 parameter 字段）、Constraints、
// Safety、ConstParams、Use/Short/Long/Example、Contract（Selection/Interface/…）。
// Catalog 组装透传 ContractFinal；声明路径不再引入评审并行字段。
//
// 执行面（不算声明）：Validate、Call、RunE、PostMount；Server/Tool 仅路由。
type LeafSpec struct {
	Use           string
	Short         string
	Long          string
	Example       string
	OutputRollout output.RolloutState

	// Server 非空时走 callMCPToolOnServer（显式 server 路由），否则走
	// callMCPTool（按 product 路由）。Call 非空时两者都被忽略。不是 CLI 声明。
	Server string
	Tool   string
	Flags  []LeafFlag
	// Constraints 是跨 flag 的关系约束（至少一个 / 恰好一个 / 互斥），由
	// command 统一校验并投影到 Runtime Schema 与 --help。复杂的条件式校验
	// 仍放 Validate 钩子（钩子本身不是约束声明）。
	Constraints []LeafConstraint

	// Safety 直接使用 Agent Runtime Schema 的安全模型。Confirmation 驱动
	// 运行时确认，其余字段原样进入 Schema；字段之间不做机械推导。
	Safety contract.SafetySpec

	// ConfirmFirst 为 true 时确认门先于 required/约束/Validate 校验执行
	//（devapp 旧版写守卫语义：写命令未带 --yes 时快速失败
	// confirmation_required，与参数完整性无关）。默认 false 保持 shortcut
	// 顺序（先校验，后端调用前再确认）。
	ConfirmFirst bool

	// ConstParams 是与 flag 无关的固定载荷（如 precheckOnly），在 flag 装配
	// 之后并入 toolArgs。载荷声明，不上用户 flag 表；从不满足 Required。
	ConstParams map[string]any

	// Contract 是叶子 ContractFinal 声明（identity/selection/interface/dry_run/…）。
	Contract LeafContract

	// Call 是执行体：非空时替代默认 MCP 派发。toolArgs 已由 Flags/ConstParams
	// 装配完成；Call 不应再写业务参数。分页等横切由领域工具处理，不进声明。
	Call       func(cmd *cobra.Command, tool string, args map[string]any) error
	ResultCall func(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error)

	// Validate 是编排钩子（条件式校验），不是声明面。单 flag 转换用
	// LeafFlag.Transform；可声明的互斥/至少一个应写 Constraints。
	Validate func(cmd *cobra.Command, args []string) error

	// RunE 非空时完全自定义执行体（逃生舱）；表面事实仍须 Flags/Contract 声明。
	RunE func(cmd *cobra.Command, args []string) error

	// PostMount 是挂载收尾钩子（领域工具等），不是声明面。
	// 业务 flag 必须写在 Flags；分页由领域工具注入。
	PostMount func(cmd *cobra.Command)
}

// NewLeafCommand 是命令框架的「完全托管模式」：经 FromLeafSpec 归一为
// corecmd.Spec 后交由统一构建器 corecmd.New 编排（flag 注册、
// 约束声明检查、Schema 投影、帮助渲染、required/约束校验、Safety 确认、
// toolArgs 装配）。本函数只保留 LeafSpec→CommandSpec 的映射与 MCP dispatch
// 闭包（callMCPTool/OnServer/Call）。所有 LeafSpec 命令（含 devapp 全部叶子）
// 由此统一流经 command 单一 spec 路径。
func NewLeafCommand(spec LeafSpec) *cobra.Command {
	return corecmd.New(FromLeafSpec(spec))
}

// DeclareLeafMetadata 是命令框架的「声明元数据模式」：把 LeafSpec 的声明面
// （Safety + Contract）挂到既有命令上——不注册 flag、不接管参数投影。可选的
// Validate 与 ConfirmSafety 同挂在 RunE 包装器内（Validate 在前），保证
// 本地可判定校验发生在确认之前（RFC §5.1 / §5.6），且直接调 RunE /
// proxySubCmd 委派不会跳过校验。
//
// user_required 确认时机：
//   - 提供 Validate：Validate → ConfirmSafety → 原 RunE（本地副作用命令）
//   - 未提供 Validate：原 RunE 先跑（含缺参校验），ConfirmSafety 推迟到
//     首次 deps.Caller.CallTool。无 Caller 时回退 ConfirmSafety → 原 RunE。
//     有 Caller 但 RunE 成功返回且从未 CallTool：fail-closed（禁止「成功
//     返回却未确认」——此时副作用可能已发生，事后 Confirm 太晚）。本地
//     副作用叶必须补 Validate，或把副作用放进 gated CallTool。
//
// 该模式是迁移态而非终态：命令具备条件时应升级为 NewLeafCommand。传入
// Flags/Constraints/ConstParams/Call/RunE/PostMount 或空 Contract 会 panic，
// 防止误用成半接管（Validate 是唯一允许的执行钩子）。
func DeclareLeafMetadata(cmd *cobra.Command, spec LeafSpec) *cobra.Command {
	if cmd == nil {
		panic("DeclareLeafMetadata: cmd is nil")
	}
	name := cmd.Name()
	if len(spec.Flags) > 0 {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): Flags must be empty (metadata-only mode)", name))
	}
	if len(spec.Constraints) > 0 {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): Constraints must be empty (metadata-only mode)", name))
	}
	if len(spec.ConstParams) > 0 {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): ConstParams must be empty (metadata-only mode)", name))
	}
	if spec.Call != nil {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): Call must be nil (metadata-only mode)", name))
	}
	if spec.ResultCall != nil {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): ResultCall must be nil (metadata-only mode)", name))
	}
	if spec.RunE != nil {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): RunE must be nil (metadata-only mode)", name))
	}
	if spec.PostMount != nil {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): PostMount must be nil (metadata-only mode)", name))
	}
	if spec.ConfirmFirst {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): ConfirmFirst must be false (metadata-only mode)", name))
	}
	if spec.Server != "" || spec.Tool != "" {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): Server/Tool must be empty (metadata-only mode)", name))
	}
	if spec.Contract.Empty() {
		panic(fmt.Sprintf("DeclareLeafMetadata(%q): Contract is required", name))
	}
	corecmd.AttachContract(cmd, spec.Safety, spec.Contract, cmd.Short, cmd.Long)
	if spec.OutputRollout != "" {
		output.SetCommandRollout(cmd, spec.OutputRollout)
	}

	confirm := strings.TrimSpace(spec.Safety.Confirmation) == "user_required"
	if spec.Validate == nil && !confirm {
		return cmd
	}
	// cmd.Annotations is always non-nil here: AttachContract above registers the
	// runtime-contract annotation on every declared leaf via the cli seam.
	rt := &contractRuntime{validate: spec.Validate, confirm: confirm}
	if confirm {
		rt.safety = spec.Safety
		cmd.Annotations[contractConfirmSafetyAnnotation] = "true"
		if spec.Validate == nil {
			cmd.Annotations[contractConfirmDeferredAnnotation] = "true"
		}
	}
	if spec.Validate != nil {
		cmd.Annotations[contractValidateAnnotation] = "true"
	}
	storeContractRuntime(cmd, rt)
	installContractRunEPipeline(cmd, rt)
	return cmd
}

// installContractRunEPipeline wraps RunE so Validate and ConfirmSafety share
// one layer (Validate first). Idempotent for the confirm annotation.
func installContractRunEPipeline(cmd *cobra.Command, rt *contractRuntime) {
	if cmd == nil || rt == nil {
		panic("installContractRunEPipeline: nil cmd/runtime")
	}
	inner := cmd.RunE
	if inner == nil {
		panic(fmt.Sprintf("installContractRunEPipeline(%q): RunE is nil", cmd.Name()))
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if rt.validate != nil {
			if err := rt.validate(c, args); err != nil {
				return err
			}
		}
		if !rt.confirm {
			return inner(c, args)
		}
		// OR across flagsets (root persistent --dry-run must not be shadowed by
		// a leaf-local --dry-run defaulting to false).
		if corecmd.BoolFlag(c, "dry-run") || (deps != nil && deps.Caller != nil && deps.Caller.DryRun()) {
			return inner(c, args)
		}
		if rt.validate != nil {
			// Local-checkable validation already passed; confirm then execute.
			if err := corecmd.ConfirmSafety(c, rt.safety); err != nil {
				return err
			}
			return inner(c, args)
		}
		// No Validate: defer confirm to first deps.Caller.CallTool so RunE-local
		// mustFlag checks can fail first. Without a caller, fall back to
		// confirm-then-run (commands on this path should add Validate).
		if deps == nil || deps.Caller == nil {
			if err := corecmd.ConfirmSafety(c, rt.safety); err != nil {
				return err
			}
			return inner(c, args)
		}
		prev := deps.Caller
		gate := &contractConfirmCaller{inner: prev, cmd: c, safety: rt.safety}
		deps.Caller = wrapContractConfirmCaller(gate, prev)
		defer func() { deps.Caller = prev }()
		if err := inner(c, args); err != nil {
			return err
		}
		if !gate.confirmed {
			// Fail closed: RunE already returned successfully. A post-RunE
			// ConfirmSafety cannot undo local side effects, and --yes would
			// falsely green-light them after the fact. Side-effect leaves must
			// declare Validate (confirm-before-RunE) or dispatch through the
			// gated CallTool path. Dry-run already short-circuits above (before
			// the deferred confirm wrapper is installed), so it cannot reach
			// this fail-closed branch.
			return fmt.Errorf("contract: user_required confirmation was never obtained via CallTool for %q; add Validate for local side effects or dispatch through deps.Caller.CallTool", c.Name())
		}
		return nil
	}
}

// contractConfirmCaller defers ConfirmSafety until the first CallTool, so
// DeclareLeafMetadata RunE can validate flags first.
type contractConfirmCaller struct {
	inner     edition.ToolCaller
	cmd       *cobra.Command
	safety    contract.SafetySpec
	confirmed bool
}

func wrapContractConfirmCaller(gate *contractConfirmCaller, inner edition.ToolCaller) edition.ToolCaller {
	if read, ok := inner.(edition.ReadToolCaller); ok {
		return &contractConfirmReadCaller{contractConfirmCaller: gate, read: read}
	}
	return gate
}

func (c *contractConfirmCaller) CallTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	if !c.confirmed {
		if err := corecmd.ConfirmSafety(c.cmd, c.safety); err != nil {
			return nil, err
		}
		c.confirmed = true
	}
	return c.inner.CallTool(ctx, productID, toolName, args)
}

func (c *contractConfirmCaller) Format() string { return c.inner.Format() }
func (c *contractConfirmCaller) DryRun() bool   { return c.inner.DryRun() }
func (c *contractConfirmCaller) Fields() string { return c.inner.Fields() }
func (c *contractConfirmCaller) JQ() string     { return c.inner.JQ() }

// contractConfirmReadCaller preserves optional ReadToolCaller without gating
// reads behind ConfirmSafety (RFC allows pre-confirm reads when needed).
type contractConfirmReadCaller struct {
	*contractConfirmCaller
	read edition.ReadToolCaller
}

func (c *contractConfirmReadCaller) CallReadTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	return c.read.CallReadTool(ctx, productID, toolName, args)
}

// HasContractConfirmSafety reports whether DeclareLeafMetadata installed the
// ConfirmSafety wrapper for a user_required SafetySpec.
func HasContractConfirmSafety(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil && cmd.Annotations[contractConfirmSafetyAnnotation] == "true"
}

// HasContractValidate reports whether DeclareLeafMetadata installed a Validate
// hook (confirm-after-Validate mode on the RunE wrapper).
func HasContractValidate(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil && cmd.Annotations[contractValidateAnnotation] == "true"
}

// HasContractConfirmDeferred reports whether ConfirmSafety is deferred to the
// first deps.Caller.CallTool (no Validate on the leaf).
func HasContractConfirmDeferred(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil && cmd.Annotations[contractConfirmDeferredAnnotation] == "true"
}

// FromLeafSpec 把 LeafSpec 归一为统一的 corecmd.Spec。契约字段
// （Flags/Constraints/Safety）与编排钩子（Validate/PostMount/RunE）直接透传；
// dispatch 收敛为一个闭包：Call 优先，其次显式 Server 路由，最后按 product
// 自动路由。RunE 逃生舱存在时不设 Dispatch（与旧行为一致）。
func FromLeafSpec(spec LeafSpec) corecmd.Spec {
	if spec.Call != nil && spec.ResultCall != nil {
		panic(fmt.Sprintf("command %q must not declare both Call and ResultCall", spec.Use))
	}
	cs := corecmd.Spec{
		Use:           spec.Use,
		Short:         spec.Short,
		Long:          spec.Long,
		Example:       spec.Example,
		OutputRollout: spec.OutputRollout,
		Flags:         spec.Flags,
		Constraints:   spec.Constraints,
		Safety:        spec.Safety,
		ConfirmFirst:  spec.ConfirmFirst,
		ConstParams:   spec.ConstParams,
		Contract:      spec.Contract,
		Validate:      spec.Validate,
		PostMount:     spec.PostMount,
		RunE:          spec.RunE,
	}
	if spec.RunE == nil {
		if spec.ResultCall != nil {
			cs.ResultInvoke = func(c *corecmd.Ctx, toolArgs map[string]any) (output.CommandResult, error) {
				return spec.ResultCall(c.Command(), spec.Tool, toolArgs)
			}
		} else {
			cs.Invoke = func(c *corecmd.Ctx, toolArgs map[string]any) error {
				if spec.Call != nil {
					return spec.Call(c.Command(), spec.Tool, toolArgs)
				}
				if spec.Server != "" {
					return callMCPToolOnServer(spec.Server, spec.Tool, toolArgs)
				}
				return callMCPTool(spec.Tool, toolArgs)
			}
		}
	}
	return cs
}
