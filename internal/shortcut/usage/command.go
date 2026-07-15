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

package usage

import (
	"fmt"
	"os"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/userdef"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewShortcutCommand builds the `dws shortcut` management command tree:
//
//	dws shortcut list [--service x]   # list built-in shortcuts
//	dws shortcut stats [--top N]      # high-frequency usage aggregation
//	dws shortcut stats --purge        # clear the usage log
func NewShortcutCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "shortcut",
		Short: "管理与洞察 shortcut（精选高保真命令）",
		Long: "查看内建 shortcut 列表，并基于本地使用统计洞察高频操作，" +
			"以便沉淀为自定义 shortcut（见 docs/shortcut-p2-design.md）。",
	}
	root.AddCommand(newListCommand(), newStatsCommand(), newSuggestCommand(), newAddCommand())
	return root
}

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出内建 shortcut 及其完整参数契约",
		Long: "列出独立于 Runtime Schema 的 shortcut catalog。每条记录包含公开参数、" +
			"必填/枚举约束、跨参数约束与可执行示例；也可用 dws <service> +<command> --help 查看单条契约。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, _ := cmd.Flags().GetString("service")
			rows := make([]shortcutListRow, 0)
			for _, s := range shortcut.All() {
				if svc != "" && s.Service != svc {
					continue
				}
				if s.Hidden {
					continue
				}
				rows = append(rows, newShortcutListRow(s))
			}
			return output.WriteCommandPayload(cmd, map[string]any{
				"catalog":        "shortcut",
				"runtime_schema": false,
				"count":          len(rows),
				"shortcuts":      rows,
			}, output.FormatJSON)
		},
	}
	cmd.Flags().String("service", "", "只列出指定服务的 shortcut")
	return cmd
}

type shortcutListRow struct {
	Service      string                `json:"service"`
	Command      string                `json:"command"`
	CLIPath      string                `json:"cli_path"`
	Product      string                `json:"product"`
	Risk         string                `json:"risk"`
	Confirmation string                `json:"confirmation"`
	Description  string                `json:"description"`
	Intent       string                `json:"intent,omitempty"`
	Flags        []shortcut.Flag       `json:"flags"`
	Constraints  []shortcut.Constraint `json:"constraints"`
	Examples     []string              `json:"examples"`
}

func newShortcutListRow(s shortcut.Shortcut) shortcutListRow {
	product := s.Product
	if product == "" {
		product = s.Service
	}
	risk := string(s.Risk)
	if risk == "" {
		risk = string(shortcut.RiskRead)
	}
	confirmation := "not_required"
	if risk != string(shortcut.RiskRead) {
		confirmation = "user_required"
	}
	flags := make([]shortcut.Flag, 0, len(s.Flags))
	for _, flag := range s.Flags {
		if flag.Hidden {
			continue
		}
		if flag.Type == "" {
			flag.Type = shortcut.FlagString
		}
		if flag.Enum == nil {
			flag.Enum = []string{}
		}
		flags = append(flags, flag)
	}
	constraints := append([]shortcut.Constraint{}, s.Constraints...)
	examples := append([]string{}, s.Tips...)
	return shortcutListRow{
		Service:      s.Service,
		Command:      s.Command,
		CLIPath:      s.Service + " " + s.Command,
		Product:      product,
		Risk:         risk,
		Confirmation: confirmation,
		Description:  s.Description,
		Intent:       s.Intent,
		Flags:        flags,
		Constraints:  constraints,
		Examples:     examples,
	}
}

func newStatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "基于本地使用统计展示高频操作",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if purge, _ := cmd.Flags().GetBool("purge"); purge {
				if err := Purge(); err != nil {
					return err
				}
				return output.WriteCommandPayload(cmd, map[string]any{
					"purged": true, "log": LogPath(),
				}, output.FormatJSON)
			}
			recs, err := Read()
			if err != nil {
				return err
			}
			groups := Aggregate(recs)
			top, _ := cmd.Flags().GetInt("top")
			if top > 0 && len(groups) > top {
				groups = groups[:top]
			}
			return output.WriteCommandPayload(cmd, map[string]any{
				"enabled":       Enabled(),
				"log":           LogPath(),
				"total_records": len(recs),
				"groups":        groups,
			}, output.FormatJSON)
		},
	}
	cmd.Flags().Int("top", 20, "展示前 N 个高频分组")
	cmd.Flags().Bool("purge", false, "清空本地使用统计日志")
	return cmd
}

// kebab turns a snake_case tool name into a "+kebab-case" command suggestion.
func kebab(tool string) string {
	return "+" + strings.ReplaceAll(tool, "_", "-")
}

// newSuggestCommand surfaces high-frequency usage groups as candidate custom
// shortcuts the user could distill (via `dws shortcut add`).
func newSuggestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "根据本地高频使用，推荐可沉淀的自定义 shortcut",
		Long: "扫描 ~/.dws/usage.jsonl，把高频的 (product, tool, 参数形状) 分组转成候选 shortcut，" +
			"并识别可固化的固定参数与需要保留为 flag 的可变参数。用 `dws shortcut add` 一键沉淀。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			recs, err := Read()
			if err != nil {
				return err
			}
			min, _ := cmd.Flags().GetInt("min")
			groups := Aggregate(recs)

			type candidate struct {
				SuggestedCommand string            `json:"suggested_command"`
				Service          string            `json:"service"`
				Product          string            `json:"product"`
				Tool             string            `json:"tool"`
				Count            int               `json:"count"`
				FixedArgs        map[string]string `json:"fixed_args,omitempty"`
				VariableArgs     []string          `json:"variable_args,omitempty"`
			}
			var cands []candidate
			for _, g := range groups {
				if g.Count < min {
					continue
				}
				var variable []string
				for _, k := range g.ArgKeys {
					if _, fixed := g.FixedArgs[k]; !fixed {
						variable = append(variable, k)
					}
				}
				cands = append(cands, candidate{
					SuggestedCommand: kebab(g.Tool),
					Service:          g.Product,
					Product:          g.Product,
					Tool:             g.Tool,
					Count:            g.Count,
					FixedArgs:        g.FixedArgs,
					VariableArgs:     variable,
				})
			}
			return output.WriteCommandPayload(cmd, map[string]any{
				"min":        min,
				"candidates": cands,
				"hint":       "沉淀示例: dws shortcut add --service <s> --command +<name> --tool <tool> --fixed k=v --var k=<flag>",
			}, output.FormatJSON)
		},
	}
	cmd.Flags().Int("min", 3, "最小出现次数阈值")
	return cmd
}

// newAddCommand distills a custom shortcut into ~/.dws/shortcuts/<svc>.<cmd>.yaml.
//
//	dws shortcut add --service chat --command +notify-team --tool send_message \
//	  --fixed open_conversation_id=cid_x --var text=text --risk write
func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "沉淀一个自定义 shortcut（写入 ~/.dws/shortcuts/*.yaml）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, _ := cmd.Flags().GetString("service")
			command, _ := cmd.Flags().GetString("command")
			product, _ := cmd.Flags().GetString("product")
			tool, _ := cmd.Flags().GetString("tool")
			risk, _ := cmd.Flags().GetString("risk")
			desc, _ := cmd.Flags().GetString("desc")
			intent, _ := cmd.Flags().GetString("intent")
			fixedKV, _ := cmd.Flags().GetStringArray("fixed")
			varKV, _ := cmd.Flags().GetStringArray("var")

			if service == "" || command == "" || tool == "" {
				return apperrors.NewValidation("必填: --service、--command（+前缀）、--tool")
			}
			if !strings.HasPrefix(command, "+") {
				command = "+" + command
			}
			if product == "" {
				product = service
			}

			spec := userdef.Spec{
				Version: 1, Service: service, Command: command, Product: product,
				Description: desc, Intent: intent, Risk: risk, Source: "manual",
				Exec: userdef.ExecSpec{Tool: tool, Bind: map[string]string{}},
			}
			for _, kv := range fixedKV {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return apperrors.NewValidation(fmt.Sprintf("--fixed 需 key=value 形式: %q", kv))
				}
				spec.Exec.Bind[k] = v
			}
			for _, kv := range varKV {
				k, flag, ok := strings.Cut(kv, "=")
				if !ok {
					return apperrors.NewValidation(fmt.Sprintf("--var 需 key=flag 形式: %q", kv))
				}
				spec.Exec.Bind[k] = "${" + flag + "}"
				spec.Flags = append(spec.Flags, userdef.FlagSpec{
					Name: flag, Type: "string", Required: true, Desc: k,
				})
			}
			if err := userdef.Validate(spec); err != nil {
				return apperrors.NewValidation(err.Error())
			}

			out, err := yaml.Marshal(spec)
			if err != nil {
				return err
			}
			if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
				return output.WriteCommandPayload(cmd, map[string]any{
					"dry_run": true, "yaml": string(out),
				}, output.FormatJSON)
			}

			path, err := userdef.FilePath(service, command)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			dir := userdef.Dir()
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, out, 0o600); err != nil {
				return err
			}
			return output.WriteCommandPayload(cmd, map[string]any{
				"saved": path, "command": "dws " + service + " " + command,
				"note": "重新运行 dws 后该 shortcut 即生效",
			}, output.FormatJSON)
		},
	}
	cmd.Flags().String("service", "", "服务名（顶层命令，必填）")
	cmd.Flags().String("command", "", "命令名，+前缀（必填）")
	cmd.Flags().String("product", "", "MCP server id（默认=service）")
	cmd.Flags().String("tool", "", "要调用的 MCP tool 名（必填）")
	cmd.Flags().String("risk", "read", "风险等级: read|write|high-risk-write")
	cmd.Flags().String("desc", "", "一行描述")
	cmd.Flags().String("intent", "", "自然语言描述（做什么/何时用）")
	cmd.Flags().StringArray("fixed", nil, "固定参数 key=value（可重复）")
	cmd.Flags().StringArray("var", nil, "可变参数 key=flag名（可重复，生成同名 flag）")
	cmd.Flags().Bool("dry-run", false, "只预览生成的 YAML，不写文件")
	return cmd
}
