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

const cacheUnsupportedMessage = "dws cache 不再支持：服务发现已下线，当前版本使用编译期静态端点目录；dws cache 仅保留为兼容入口，不会刷新端点。"

const cacheReplacementHint = "如遇 endpoint_not_resolved，请先执行 dws upgrade 获取包含最新 internal/syncdata 端点的版本；仍失败时检查 internal/syncdata.StaticServers() 是否覆盖目标 product/server。"

type cacheCompatNotice struct {
	Status      string `json:"status"`
	Command     string `json:"command"`
	Message     string `json:"message"`
	Replacement string `json:"replacement,omitempty"`
}

// newCacheCommand keeps a visible Deprecated compatibility surface for
// historical argv (refresh/status/clean). Behavior is a successful no-op notice.
// Skills must not teach this path. Deprecated leaves are excluded from Schema
// via cobra.IsAvailableCommand() — do not add schema_command_exclusions entries.
func newCacheCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "cache",
		Short:             "不再支持：服务发现缓存兼容入口",
		Long:              "此命令组仅为历史 argv 兼容保留。静态端点模式下无需服务发现缓存；Skill / Agent 请勿引导此路径。",
		Deprecated:        "不再支持；" + cacheUnsupportedMessage,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printCacheCompatNotice(cmd, "dws cache")
		},
	}
	for _, name := range []string{"refresh", "status", "clean"} {
		subName := name
		sub := &cobra.Command{
			Use:               subName,
			Short:             "不再支持：静态端点模式无需服务发现缓存",
			Deprecated:        "不再支持；" + cacheUnsupportedMessage,
			Args:              cobra.NoArgs,
			DisableAutoGenTag: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return printCacheCompatNotice(cmd, "dws cache "+subName)
			},
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func printCacheCompatNotice(cmd *cobra.Command, command string) error {
	notice := cacheCompatNotice{
		Status:      "deprecated",
		Command:     command,
		Message:     cacheUnsupportedMessage,
		Replacement: cacheReplacementHint,
	}
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return json.NewEncoder(cmd.OutOrStdout()).Encode(notice)
	case "pretty":
		data, _ := json.MarshalIndent(notice, "", "  ")
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n%s\n", notice.Command, notice.Message, notice.Replacement)
		return err
	}
}
