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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler { return mailHandler{} })
}

// mailHandler contributes the hardcoded `mail message list` leaf. The MCP
// backend has no dedicated "list by folder" tool — wukong implements it by
// calling search_emails with a synthesised KQL query (folderId:<id>). The
// envelope/discovery layer cannot express that query construction (pipeline
// $flag templates resolve raw flag values and do not support string
// interpolation), so it lives here as a helper leaf. MergeCommandTree folds
// this single leaf into the envelope-driven mail tree without disturbing the
// other mail commands.
type mailHandler struct{}

func (mailHandler) Name() string { return "mail" }

func (mailHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "mail",
		Short:             i18n.T("邮箱"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	message := &cobra.Command{
		Use:               "message",
		Short:             i18n.T("邮件管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	message.AddCommand(newMailMessageListCommand(runner))
	root.AddCommand(message)
	return root
}

// newMailMessageListCommand builds `mail message list`, listing emails in a
// folder via search_emails with query=folderId:<id> (default inbox=2).
func newMailMessageListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("列出文件夹中的邮件"),
		Example: "  dws mail message list --email user@company.com  # 默认列出收件箱\n" +
			"  dws mail message list --email user@company.com --folder-id 1 --limit 50",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			email := strings.TrimSpace(firstNonEmptyFlag(cmd, "email"))
			if email == "" {
				return apperrors.NewValidation("missing required flag(s): --email")
			}
			folderID := strings.TrimSpace(firstNonEmptyFlag(cmd, "folder-id", "folder"))
			if folderID == "" {
				folderID = "2" // inbox
			}
			params := map[string]any{
				"email": email,
				"query": "folderId:" + folderID,
			}
			if size := strings.TrimSpace(firstNonEmptyFlag(cmd, "limit", "size", "page-size")); size != "" {
				params["size"] = size
			}
			if cursor := strings.TrimSpace(firstNonEmptyFlag(cmd, "cursor")); cursor != "" {
				params["cursor"] = cursor
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "mail", "search_emails", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "mail", "search_emails", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("email", "", i18n.T("邮件所属邮箱地址 (必填)"))
	cmd.Flags().String("folder-id", "", i18n.T("文件夹 ID (可选, 默认收件箱 2)"))
	cmd.Flags().String("folder", "", i18n.T("--folder-id 的别名"))
	cmd.Flags().String("limit", "", i18n.T("每页返回数量 (可选)"))
	cmd.Flags().String("size", "", i18n.T("--limit 的别名"))
	cmd.Flags().String("page-size", "", i18n.T("--limit 的别名"))
	cmd.Flags().String("cursor", "", i18n.T("分页游标 (可选)"))
	return cmd
}

// firstNonEmptyFlag returns the first non-empty string flag value among names.
func firstNonEmptyFlag(cmd *cobra.Command, names ...string) string {
	for _, n := range names {
		if v, err := cmd.Flags().GetString(n); err == nil && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
