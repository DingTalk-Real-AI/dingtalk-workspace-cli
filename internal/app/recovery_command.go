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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const recoveryUnsupportedMessage = "dws recovery 不再支持：失败快照恢复计划/执行/闭环已下线，请改用 doctor / schema / 对应业务命令排查。"

type recoveryCompatNotice struct {
	Status  string `json:"status"`
	Command string `json:"command"`
	Message string `json:"message"`
}

// newRecoveryCommand keeps a visible Deprecated compatibility surface for
// historical argv and Interface Integrity. Behavior is unchanged: every leaf
// returns an explicit unsupported notice. Skills must not teach this path.
func newRecoveryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "recovery",
		Short:             "不再支持：错误恢复辅助命令（兼容入口）",
		Long:              "此命令组仅为历史 argv 兼容保留，不再读取失败快照或生成恢复计划。Skill / Agent 请勿引导此路径。",
		Deprecated:        "不再支持；" + recoveryUnsupportedMessage,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printRecoveryUnsupported(cmd, "dws recovery")
		},
	}

	planCmd := &cobra.Command{
		Use:               "plan",
		Short:             "不再支持：基于失败快照生成恢复计划",
		Deprecated:        "不再支持；" + recoveryUnsupportedMessage,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printRecoveryUnsupported(cmd, "dws recovery plan")
		},
	}
	planCmd.Flags().Bool("last", false, "旧版兼容参数；recovery 不再支持")
	planCmd.Flags().String("event-id", "", "旧版兼容参数；recovery 不再支持")

	executeCmd := &cobra.Command{
		Use:               "execute",
		Short:             "不再支持：生成面向 Agent 的恢复分析包",
		Deprecated:        "不再支持；" + recoveryUnsupportedMessage,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printRecoveryUnsupported(cmd, "dws recovery execute")
		},
	}
	executeCmd.Flags().Bool("last", false, "旧版兼容参数；recovery 不再支持")
	executeCmd.Flags().String("event-id", "", "旧版兼容参数；recovery 不再支持")

	finalizeCmd := &cobra.Command{
		Use:               "finalize",
		Short:             "不再支持：回写恢复闭环结果",
		Deprecated:        "不再支持；" + recoveryUnsupportedMessage,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printRecoveryUnsupported(cmd, "dws recovery finalize")
		},
	}
	finalizeCmd.Flags().String("event-id", "", "旧版兼容参数；recovery 不再支持")
	finalizeCmd.Flags().String("outcome", "", "旧版兼容参数；recovery 不再支持")
	finalizeCmd.Flags().String("execution-file", "", "旧版兼容参数；recovery 不再支持")

	cmd.AddCommand(planCmd, executeCmd, finalizeCmd)
	return cmd
}

func printRecoveryUnsupported(cmd *cobra.Command, command string) error {
	notice := recoveryCompatNotice{
		Status:  "unsupported",
		Command: command,
		Message: recoveryUnsupportedMessage,
	}
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(notice); err != nil {
			return err
		}
		return apperrors.NewValidation(recoveryUnsupportedMessage)
	case "pretty":
		data, _ := json.MarshalIndent(notice, "", "  ")
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(data)); err != nil {
			return err
		}
		return apperrors.NewValidation(recoveryUnsupportedMessage)
	default:
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", notice.Command, notice.Message); err != nil {
			return err
		}
		return apperrors.NewValidation(recoveryUnsupportedMessage)
	}
}
