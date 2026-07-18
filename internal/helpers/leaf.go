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
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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
// 迁移纪律：从手写命令迁移到 LeafSpec 时，flag 名、默认值、usage 文案、
// MarkFlagRequired、required 错误格式、toolArgs 键与值必须逐字保持一致，
// 由 check-generated-drift（catalog 零漂移）与命令兼容性检查兜底证明等价。

// LeafFlagKind 是 flag 的值类型。
type LeafFlagKind int

const (
	// LeafString 字符串 flag（默认）。
	LeafString LeafFlagKind = iota
	// LeafInt64 整型 flag；仅在值 > 0 时进入 toolArgs（分页游标语义）。
	LeafInt64
	// LeafInt 整型 flag（注册为 cobra Int）；仅在值 != 0 时进入 toolArgs，
	// 对应手写「putInt 仅在非零才入参」语义（如 devapp app-group-id）。
	LeafInt
)

// LeafFlag 声明一个 flag 的注册方式与到 MCP toolArgs 的绑定。
type LeafFlag struct {
	Name    string       // flag 名（kebab-case）
	Usage   string       // 注册 usage 文案
	Kind    LeafFlagKind // 值类型，默认 LeafString
	Default string       // 注册默认值（仅 LeafString 有效）

	// Required 为 true 时在 RunE 期校验有效值非空。普通 Required 汇聚为
	// cmdutil.ValidateRequiredFlags 兼容的统一报错；配置 EnvVar 时回退读
	// 环境变量，仍为空则报 RequiredHint（或默认文案）。
	Required     bool
	RequiredHint string
	// MarkRequired 为 true 时调用 cobra MarkFlagRequired（catalog required
	// 投影的硬下限），cobra 会在 RunE 之前先行报错。
	MarkRequired bool

	Aliases []string // 隐藏别名，主 flag 为空时按序回退（字符串）
	EnvVar  string   // 有效值为空时回退读取的环境变量
	// ArgDefault：注册默认值为空、但 toolArgs 需要兜底的场景（如 list type
	// 注册默认 "ALL" 之外的旧命令）；有效值为空时以 ArgDefault 入参。
	ArgDefault string
	// Bind 是 toolArgs 的键；为空时使用 Name。
	Bind string
	// Transform 把字符串有效值转为入参值；nil 时原样入参。返回
	// (nil, nil) 表示跳过该键（用于「可空数值：为空或解析失败都不入参」
	// 的手写语义）。
	Transform func(raw string) (any, error)
	// OmitEmpty 为 true 时有效值为空则不进入 toolArgs（LeafInt64 恒为
	// 「值 > 0 才入参」，忽略此字段）。
	OmitEmpty bool
	// Trim 为 true 时对有效值做 strings.TrimSpace（主 flag/别名/env 统一），
	// 对应手写 devAppStringFlag 恒 trim 的语义；亦使「纯空白」值在 required
	// 校验中视为空。
	Trim bool
}

// LeafSpec 声明一个 MCP 直连叶子命令。
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

// NewLeafCommand 按 LeafSpec 构建叶子命令。
func NewLeafCommand(spec LeafSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.Use,
		Short:   spec.Short,
		Long:    spec.Long,
		Example: spec.Example,
	}
	for _, flag := range spec.Flags {
		switch flag.Kind {
		case LeafInt64:
			cmd.Flags().Int64(flag.Name, 0, flag.Usage)
		case LeafInt:
			cmd.Flags().Int(flag.Name, 0, flag.Usage)
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Usage)
		}
		for _, alias := range flag.Aliases {
			cmd.Flags().String(alias, "", flag.Usage+" (alias)")
			_ = cmd.Flags().MarkHidden(alias)
		}
		if flag.MarkRequired {
			_ = cmd.MarkFlagRequired(flag.Name)
		}
	}
	if spec.PostMount != nil {
		spec.PostMount(cmd)
	}
	if spec.RunE != nil {
		cmd.RunE = spec.RunE
		return cmd
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := leafValidateRequired(cmd, spec); err != nil {
			return err
		}
		if spec.Validate != nil {
			if err := spec.Validate(cmd, args); err != nil {
				return err
			}
		}
		toolArgs, err := leafArgs(cmd, spec)
		if err != nil {
			return err
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

// leafValidateRequired 复现手写命令的 required 语义：普通 Required 统一报
// 「missing required flag(s)」；带 EnvVar 的 Required 单独报 RequiredHint。
// 普通组先于环境变量组校验，保持与手写顺序一致。
func leafValidateRequired(cmd *cobra.Command, spec LeafSpec) error {
	var plain []string
	for _, flag := range spec.Flags {
		if flag.Required && flag.EnvVar == "" && flag.RequiredHint == "" {
			plain = append(plain, flag.Name)
		}
	}
	if err := validateRequiredFlags(cmd, plain...); err != nil {
		return err
	}
	for _, flag := range spec.Flags {
		if !flag.Required || (flag.EnvVar == "" && flag.RequiredHint == "") {
			continue
		}
		if leafEffectiveValue(cmd, flag) == "" {
			hint := flag.RequiredHint
			if hint == "" {
				hint = fmt.Sprintf("flag --%s is required", flag.Name)
			}
			return fmt.Errorf("%s", hint)
		}
	}
	return nil
}

// leafArgs 按绑定关系装配 toolArgs。
func leafArgs(cmd *cobra.Command, spec LeafSpec) (map[string]any, error) {
	toolArgs := map[string]any{}
	for _, flag := range spec.Flags {
		bind := flag.Bind
		if bind == "" {
			bind = flag.Name
		}
		if flag.Kind == LeafInt64 {
			if v, _ := cmd.Flags().GetInt64(flag.Name); v > 0 {
				toolArgs[bind] = v
			}
			continue
		}
		if flag.Kind == LeafInt {
			if v, _ := cmd.Flags().GetInt(flag.Name); v != 0 {
				toolArgs[bind] = v
			}
			continue
		}
		effective := leafEffectiveValue(cmd, flag)
		if effective == "" && flag.ArgDefault != "" {
			effective = flag.ArgDefault
		}
		if effective == "" && flag.OmitEmpty {
			continue
		}
		if flag.Transform != nil {
			value, err := flag.Transform(effective)
			if err != nil {
				return nil, err
			}
			if value == nil {
				continue
			}
			toolArgs[bind] = value
			continue
		}
		toolArgs[bind] = effective
	}
	return toolArgs, nil
}

// leafEffectiveValue 按「主 flag → 别名 → 环境变量」顺序取有效值；Trim 为
// true 时对结果统一 TrimSpace。
func leafEffectiveValue(cmd *cobra.Command, flag LeafFlag) string {
	v := leafRawValue(cmd, flag)
	if flag.Trim {
		v = strings.TrimSpace(v)
	}
	return v
}

// leafRawValue 取未 trim 的原始有效值（主 flag → 别名 → 环境变量）。
func leafRawValue(cmd *cobra.Command, flag LeafFlag) string {
	if v := mustGetFlag(cmd, flag.Name); v != "" {
		return v
	}
	for _, alias := range flag.Aliases {
		if v, _ := cmd.Flags().GetString(alias); v != "" {
			return v
		}
	}
	if flag.EnvVar != "" {
		return os.Getenv(flag.EnvVar)
	}
	return ""
}
