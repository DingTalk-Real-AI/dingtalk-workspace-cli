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
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/spf13/cobra"
)

func newPluginCommand() *cobra.Command {
	pluginCmd := newPlaceholderParent("plugin", "插件管理")

	pluginCmd.AddCommand(
		newPluginListCommand(),
		newPluginInstallCommand(),
		newPluginInfoCommand(),
		newPluginEnableCommand(),
		newPluginDisableCommand(),
		newPluginRemoveCommand(),
		newPluginValidateCommand(),
	)

	return pluginCmd
}

func newPluginListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "列出已安装插件",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			plugins := loader.ListInstalled()

			wantJSON, _ := cmd.Flags().GetBool("json")
			if wantJSON {
				return output.WriteJSON(cmd.OutOrStdout(), plugins)
			}

			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "暂无已安装插件")
				return nil
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-30s %-10s %-10s %-8s %s\n",
				"插件名", "版本", "类型", "状态", "描述")
			fmt.Fprintln(w, strings.Repeat("─", 80))
			for _, p := range plugins {
				pType := "三方"
				if p.Type == "managed" {
					pType = "官方"
				}
				status := "启用"
				if !p.Enabled {
					status = "禁用"
				}
				fmt.Fprintf(w, "%-30s %-10s %-10s %-8s %s\n",
					p.Name, p.Version, pType, status, p.Description)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

func newPluginInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "安装插件",
		Example: `  dws plugin install --dir ./conference
  dws plugin install --git https://github.com/DingTalk-Real-AI/conference.git`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dirPath, _ := cmd.Flags().GetString("dir")
			gitURL, _ := cmd.Flags().GetString("git")

			if dirPath == "" && gitURL == "" {
				return apperrors.NewValidation("请指定安装来源：--dir <目录> 或 --git <仓库地址>")
			}

			if gitURL != "" {
				return apperrors.NewValidation("git 安装暂未实现，请使用 --dir 从本地目录安装")
			}

			loader := plugin.NewLoader(RawVersion())
			p, err := loader.InstallFromDir(dirPath)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("安装失败: %v", err))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✅ 插件 %s (%s) 安装成功\n", p.Manifest.Name, p.Manifest.Version)
			return nil
		},
	}
	cmd.Flags().String("dir", "", "从本地目录安装插件")
	cmd.Flags().String("git", "", "从 Git 仓库安装插件")
	return cmd
}

func newPluginInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "info <插件名>",
		Short:             "查看插件详情",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			loader := plugin.NewLoader(RawVersion())
			plugins := loader.ListInstalled()

			for _, p := range plugins {
				if p.Name == name {
					w := cmd.OutOrStdout()
					fmt.Fprintf(w, "插件名:    %s\n", p.Name)
					fmt.Fprintf(w, "版本:      %s\n", p.Version)
					fmt.Fprintf(w, "类型:      %s\n", p.Type)
					fmt.Fprintf(w, "状态:      %s\n", enabledStr(p.Enabled))
					fmt.Fprintf(w, "路径:      %s\n", p.Path)
					if p.Description != "" {
						fmt.Fprintf(w, "描述:      %s\n", p.Description)
					}
					return nil
				}
			}
			return apperrors.NewValidation(fmt.Sprintf("插件 %q 未安装", name))
		},
	}
}

func newPluginEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "enable <插件名>",
		Short:             "启用插件",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			if err := loader.SetEnabled(args[0], true); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ 插件 %s 已启用\n", args[0])
			return nil
		},
	}
}

func newPluginDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "disable <插件名>",
		Short:             "禁用插件（官方插件可禁用但不可卸载）",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			if err := loader.SetEnabled(args[0], false); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ 插件 %s 已禁用\n", args[0])
			return nil
		},
	}
}

func newPluginRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove <插件名>",
		Short:             "卸载三方插件（官方插件禁止卸载）",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keepData, _ := cmd.Flags().GetBool("keep-data")
			loader := plugin.NewLoader(RawVersion())
			if err := loader.RemovePlugin(args[0], keepData); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ 插件 %s 已卸载\n", args[0])
			return nil
		},
	}
	cmd.Flags().Bool("keep-data", false, "保留插件数据目录")
	return cmd
}

func newPluginValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "validate <目录>",
		Short:             "校验 plugin.json",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			m, err := plugin.ParseManifest(dir + "/plugin.json")
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("解析失败: %v", err))
			}
			if err := m.Validate(RawVersion()); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("校验失败: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ plugin.json 校验通过: %s (%s)\n", m.Name, m.Version)
			return nil
		},
	}
}

func enabledStr(enabled bool) string {
	if enabled {
		return "启用"
	}
	return "禁用"
}
