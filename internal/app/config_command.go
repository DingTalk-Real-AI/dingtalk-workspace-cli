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
	"fmt"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "配置管理",
		Long:              "管理 DWS CLI 的配置项。查看所有支持的环境变量及其当前值。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigListCommand())
	return cmd
}

func newConfigListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有可用配置项",
		Long:  "显示 DWS CLI 支持的全部环境变量配置项，包括名称、分类、描述和默认值。",
		RunE:  runConfigList,
	}
	cmd.Flags().String("category", "", "按分类过滤 (core|auth|network|security|runtime|debug|external)")
	cmd.Flags().Bool("show-values", false, "显示配置项的当前实际值 (敏感信息会脱敏)")
	cmd.Flags().Bool("show-hidden", false, "包含隐藏的内部调试配置项")
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

func runConfigList(cmd *cobra.Command, _ []string) error {
	category, _ := cmd.Flags().GetString("category")
	showValues, _ := cmd.Flags().GetBool("show-values")
	showHidden, _ := cmd.Flags().GetBool("show-hidden")
	jsonOut, _ := cmd.Flags().GetBool("json")

	var items []configmeta.ConfigItem
	if category != "" {
		items = configmeta.ByCategory(configmeta.Category(category))
	} else {
		items = configmeta.All()
	}

	if !showHidden {
		items = filterVisible(items)
	}

	if jsonOut {
		return writeConfigJSON(cmd, items, showValues)
	}
	return writeConfigTable(cmd, items, showValues)
}

func filterVisible(items []configmeta.ConfigItem) []configmeta.ConfigItem {
	out := make([]configmeta.ConfigItem, 0, len(items))
	for _, item := range items {
		if !item.Hidden {
			out = append(out, item)
		}
	}
	return out
}

func writeConfigJSON(cmd *cobra.Command, items []configmeta.ConfigItem, showValues bool) error {
	type jsonItem struct {
		Name         string `json:"name"`
		Category     string `json:"category"`
		Description  string `json:"description"`
		DefaultValue string `json:"default_value,omitempty"`
		Example      string `json:"example,omitempty"`
		Sensitive    bool   `json:"sensitive,omitempty"`
		CurrentValue string `json:"current_value,omitempty"`
		IsSet        bool   `json:"is_set"`
	}

	result := make([]jsonItem, 0, len(items))
	for _, item := range items {
		ji := jsonItem{
			Name:         item.Name,
			Category:     string(item.Category),
			Description:  item.Description,
			DefaultValue: item.DefaultValue,
			Example:      item.Example,
			Sensitive:    item.Sensitive,
		}
		val, ok := configmeta.Resolve(item.Name)
		ji.IsSet = ok
		if showValues && ok {
			ji.CurrentValue = val
		}
		result = append(result, ji)
	}

	return output.WriteJSON(cmd.OutOrStdout(), map[string]any{
		"kind":    "config_list",
		"count":   len(result),
		"configs": result,
	})
}

func writeConfigTable(cmd *cobra.Command, items []configmeta.ConfigItem, showValues bool) error {
	w := cmd.OutOrStdout()

	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "没有找到匹配的配置项。")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if showValues {
		_, _ = fmt.Fprintln(tw, "分类\t配置项\t描述\t默认值\t当前值")
		_, _ = fmt.Fprintln(tw, "────\t──────\t────\t──────\t──────")
	} else {
		_, _ = fmt.Fprintln(tw, "分类\t配置项\t描述\t默认值")
		_, _ = fmt.Fprintln(tw, "────\t──────\t────\t──────")
	}

	for _, item := range items {
		def := item.DefaultValue
		if def == "" {
			def = "(空)"
		}
		if showValues {
			val, ok := configmeta.Resolve(item.Name)
			display := "(未设置)"
			if ok {
				display = val
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.Category, item.Name, item.Description, def, display)
		} else {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				item.Category, item.Name, item.Description, def)
		}
	}

	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n共 %d 个配置项。使用 --show-values 查看当前值，--show-hidden 显示隐藏项。\n", len(items))
	return nil
}
