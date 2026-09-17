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
	"io"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resolveCommandFormat 是 helpers 侧的 format 解析桥接（B44，WS1 改动点4）：
// 从 cmd flags 统一解析输出 format——显式非空 --format 恒优先，其次 --json
// 布尔简写，两者皆无落 fallback json（轮11-DEV 裁决函数
// output.ResolveFormatWithJSONShorthand，含未知值归一化降级）。与主漏斗
// callMCPToolInternalOpts 的 output.ParseFormat（caller 字符串路径，轮8 A3）
// 共用同一 normalizeFormat 归一化规则——cmd 路径与 caller 路径收敛于同一
// 单一事实源，不重造第二套解析。nil cmd 返回 fallback（不 panic）。
func resolveCommandFormat(cmd *cobra.Command) output.Format {
	return output.ResolveFormatWithJSONShorthand(cmd, output.FormatJSON)
}

// writeCommandPayload 按当前 format 分发命令载荷（B57，WS1 改动点4）：
// format 经 resolveCommandFormat（B44 桥接）解析，替换原固定 FormatJSON 兜底；
// --fields/--jq 全局过滤同路联动；数据出口走 cmd.OutOrStdout()（可被测试
// 重定向，不硬编码 os.Stdout）。nil cmd 按 json 渲染进 io.Discard（与
// output.WriteCommandPayload 的容错口径一致）。
func writeCommandPayload(cmd *cobra.Command, payload any) error {
	format := resolveCommandFormat(cmd)
	if cmd == nil {
		return output.Write(io.Discard, format, payload)
	}
	return output.WriteFiltered(cmd.OutOrStdout(), format, payload, output.ResolveFields(cmd), output.ResolveJQ(cmd))
}

// writeEnvelope 是 helpers 侧的统一信封装配出口（B58，WS1 改动点2）：信封
// 渲染先进 buffer 再写 cmd 流（buffer-first 由 internal/output 承载），按
// outcome 分流：
//
//   - success / pending / partial_failure → 数据通道：经 output.WriteEnvelope
//     走完整 format 分发矩阵（含未知 format 降级 + stderr warning，AC-09）；
//   - failure（含 nil 信封降级）→ 错误通道：经 Emitter 落 cmd.ErrOrStderr()，
//     stdout 严格零字节（AC-11），失败信封恒完整 JSON 且绕过 format/jq/fields
//     （轮8裁决⑪）。
//
// 本函数只做装配分流，不重定义信封类型或渲染规则。
func writeEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	if env != nil && env.Outcome != output.OutcomeFailure {
		return output.WriteEnvelope(cmd, env, output.FormatJSON)
	}
	w, errW := io.Writer(io.Discard), io.Writer(io.Discard)
	if cmd != nil {
		w = cmd.OutOrStdout()
		errW = cmd.ErrOrStderr()
	}
	return output.NewEmitter(w, errW, output.FormatJSON, "", "").Emit(env)
}

func preferLegacyLeaf(cmd *cobra.Command) {
	cli.SetOverridePriority(cmd, 100)
}

func commandDryRun(cmd *cobra.Command) bool {
	return commandBoolFlag(cmd, "dry-run")
}

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	rootFlags := cmd.Root().PersistentFlags()
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags(), rootFlags} {
		flag := flags.Lookup(name)
		if flag == nil {
			continue
		}
		value, err := flags.GetBool(name)
		if err == nil {
			return value
		}
	}
	return false
}
