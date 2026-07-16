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
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type cacheCompatNotice struct {
	Status      string `json:"status"`
	Command     string `json:"command"`
	Message     string `json:"message"`
	Replacement string `json:"replacement,omitempty"`
}

func newCacheCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "cache",
		Short:             "服务发现缓存兼容入口（静态端点模式已弃用）",
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, name := range []string{"refresh", "status", "clean"} {
		sub := &cobra.Command{
			Use:               name,
			Short:             "已弃用：静态端点模式无需服务发现缓存",
			Args:              cobra.NoArgs,
			DisableAutoGenTag: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return printCacheCompatNotice(cmd, name)
			},
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func printCacheCompatNotice(cmd *cobra.Command, command string) error {
	notice := cacheCompatNotice{
		Status:      "deprecated",
		Command:     "dws cache " + command,
		Message:     "服务发现已下线，当前版本使用编译期静态端点目录；dws cache 仅保留为兼容入口，不会刷新端点。",
		Replacement: "如遇 endpoint_not_resolved，请先执行 dws upgrade 获取包含最新 internal/syncdata 端点的版本；仍失败时检查 internal/syncdata.StaticServers() 是否覆盖目标 product/server。",
	}
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return json.NewEncoder(cmd.OutOrStdout()).Encode(notice)
	case "pretty":
		data, _ := json.MarshalIndent(notice, "", "  ")
		var err error
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n%s\n", notice.Command, notice.Message, notice.Replacement)
		return err
	}
}
