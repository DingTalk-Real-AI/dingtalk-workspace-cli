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
	"bufio"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func newDrivePermissionExtraCommands() []*cobra.Command {
	transferOwnerCmd := &cobra.Command{
		Use:   "transfer-owner",
		Short: "转交所有者",
		Long: `转交文档或知识库的所有者给指定用户。此操作不可逆，执行前会要求确认。

支持两种转交粒度：
  节点级（--node）：转交指定文档、文件夹或文件的所有者
  空间级（--workspace）：转交整个知识库的所有者

--node 和 --workspace 至少提供一个；同时提供时以 --node 为准。

使用 --yes 可跳过交互确认，但必须显式提供 --reserve-role 和 --recursive。`,
		Example: `  dws drive permission transfer-owner --node DOC_ID --new-owner uid123
  dws drive permission transfer-owner --workspace WS_ID --new-owner uid123
  dws drive permission transfer-owner --node DOC_ID --new-owner uid123 --reserve-role EDITOR --recursive=false --yes`,
		RunE: runDrivePermissionTransferOwner,
	}
	transferOwnerCmd.Flags().String("node", "", "目标节点 ID 或 URL（与 --workspace 至少提供一个）")
	transferOwnerCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（与 --node 至少提供一个）")
	transferOwnerCmd.Flags().String("new-owner", "", "新所有者的用户 userId (必填)")
	transferOwnerCmd.Flags().String("reserve-role", "", "原所有者保留角色: MANAGER / EDITOR / DOWNLOADER / READER / NONE")
	transferOwnerCmd.Flags().Bool("recursive", false, "是否递归变更所有子节点的所有者")

	applyInfoCmd := &cobra.Command{
		Use:   "apply-info",
		Short: "查询节点可申请的角色与审批人",
		Long: `查询指定节点当前用户可申请的权限角色和审批人。

返回 availableRoles[].roleId 可作为 permission apply 的 --role，
approvers[].userId 可作为 --users。`,
		Example: `  dws drive permission apply-info --node DOC_ID`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "query_permission_apply_info", map[string]any{"nodeId": nodeID})
		},
	}
	applyInfoCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")

	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "发起权限申请",
		Long: `向指定节点的审批人发起权限申请，系统会真实通知审批人。

支持的角色: EDITOR / DOWNLOADER / READER。
建议先执行 permission apply-info 获取可申请角色和审批人。
执行前必须确认资源、角色、审批人和申请理由。`,
		Example: `  dws drive permission apply --node DOC_ID --role READER --users uid1
  dws drive permission apply --node DOC_ID --role EDITOR --users uid1,uid2 --reason "需要编辑该文档" --yes`,
		RunE: runDrivePermissionApply,
	}
	applyCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	applyCmd.Flags().String("role", "", "申请角色: EDITOR / DOWNLOADER / READER (必填)")
	applyCmd.Flags().String("users", "", "审批人 userId 列表，逗号分隔 (必填)")
	applyCmd.Flags().String("user", "", "")
	_ = applyCmd.Flags().MarkHidden("user")
	applyCmd.Flags().String("notify-mode", "", "通知方式: DEFAULT / MSG_ACCOUNT / SINGLE_CHAT (选填)")
	applyCmd.Flags().String("reason", "", "申请理由，最长 200 字符 (选填)")

	commands := []*cobra.Command{applyInfoCmd, applyCmd, transferOwnerCmd}
	for _, command := range commands {
		addHiddenNodeAliases(command)
	}
	return commands
}

func runDrivePermissionTransferOwner(cmd *cobra.Command, _ []string) error {
	nodeID, _ := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
	if nodeID == "" && workspaceID == "" {
		return fmt.Errorf("--node or --workspace is required (specify at least one)")
	}
	if err := validateRequiredFlags(cmd, "new-owner"); err != nil {
		return err
	}
	newOwnerID := strings.TrimSpace(mustGetFlag(cmd, "new-owner"))
	if newOwnerID == "" {
		return fmt.Errorf("flag --new-owner is required")
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "转交所有者")
		if nodeID != "" {
			deps.Out.PrintKeyValue("目标节点", nodeID)
		} else {
			deps.Out.PrintKeyValue("目标知识库", workspaceID)
		}
		deps.Out.PrintKeyValue("新所有者", newOwnerID)
		return nil
	}

	yes, _ := cmd.Flags().GetBool("yes")
	if yes {
		if !cmd.Flags().Changed("reserve-role") {
			return fmt.Errorf("--reserve-role is required when using --yes")
		}
		if !cmd.Flags().Changed("recursive") {
			return fmt.Errorf("--recursive is required when using --yes; specify --recursive=true or --recursive=false")
		}
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	reserveRole, err := driveTransferReserveRole(cmd, reader)
	if err != nil {
		return err
	}
	recursive, err := driveTransferRecursive(cmd, reader)
	if err != nil {
		return err
	}
	target := nodeID
	if target == "" {
		target = workspaceID
	}
	if !confirmDriveTransferOwner(cmd, reader, target, newOwnerID, reserveRole, recursive) {
		return nil
	}

	toolArgs := map[string]any{"newOwnerId": newOwnerID}
	if nodeID != "" {
		toolArgs["nodeId"] = nodeID
	} else {
		toolArgs["workspaceId"] = workspaceID
	}
	if reserveRole != "" {
		toolArgs["reserveOldOwnerRole"] = reserveRole
	}
	if recursive {
		toolArgs["recursiveChange"] = true
	}
	return callMCPToolOnServer("doc", "transfer_owner", toolArgs)
}

func driveTransferReserveRole(cmd *cobra.Command, reader *bufio.Reader) (string, error) {
	if cmd.Flags().Changed("reserve-role") {
		role := normalizePermissionRole(mustGetFlag(cmd, "reserve-role"))
		if !stringInSet(role, "MANAGER", "EDITOR", "DOWNLOADER", "READER", "NONE") {
			return "", fmt.Errorf("invalid --reserve-role %q: use MANAGER, EDITOR, DOWNLOADER, READER, or NONE", role)
		}
		if role == "NONE" {
			return "", nil
		}
		return role, nil
	}

	output := cmd.ErrOrStderr()
	fmt.Fprintln(output, "请选择转交后原所有者保留的角色：")
	fmt.Fprintln(output, "  1. MANAGER")
	fmt.Fprintln(output, "  2. EDITOR")
	fmt.Fprintln(output, "  3. DOWNLOADER")
	fmt.Fprintln(output, "  4. READER")
	fmt.Fprintln(output, "  5. NONE")
	fmt.Fprint(output, "请输入选项编号 (1-5): ")
	answer, _ := reader.ReadString('\n')
	switch strings.TrimSpace(answer) {
	case "1":
		return "MANAGER", nil
	case "2":
		return "EDITOR", nil
	case "3":
		return "DOWNLOADER", nil
	case "4":
		return "READER", nil
	case "5":
		return "", nil
	default:
		return "", fmt.Errorf("invalid reserve-role selection; enter 1-5")
	}
}

func driveTransferRecursive(cmd *cobra.Command, reader *bufio.Reader) (bool, error) {
	if cmd.Flags().Changed("recursive") {
		return cmd.Flags().GetBool("recursive")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "是否递归变更所有子节点的所有者？(y/n): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func confirmDriveTransferOwner(cmd *cobra.Command, reader *bufio.Reader, target, newOwner, reserveRole string, recursive bool) bool {
	if yes, _ := cmd.Flags().GetBool("yes"); yes {
		return true
	}
	reserveDisplay := reserveRole
	if reserveDisplay == "" {
		reserveDisplay = "NONE"
	}
	output := cmd.ErrOrStderr()
	fmt.Fprintf(output, "即将转交所有者：目标=%s，新所有者=%s，原所有者角色=%s，递归=%t\n", target, newOwner, reserveDisplay, recursive)
	fmt.Fprint(output, "确认执行？(yes/no): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		return true
	}
	fmt.Fprintln(output, "Operation cancelled")
	return false
}

func runDrivePermissionApply(cmd *cobra.Command, _ []string) error {
	nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return err
	}
	if err := validateRequiredFlags(cmd, "role"); err != nil {
		return err
	}
	role := normalizePermissionRole(mustGetFlag(cmd, "role"))
	if !stringInSet(role, "EDITOR", "DOWNLOADER", "READER") {
		return fmt.Errorf("invalid --role %q: use EDITOR, DOWNLOADER, or READER", role)
	}
	userIDs, err := collectUserIDs(cmd)
	if err != nil {
		return err
	}
	toolArgs := map[string]any{
		"nodeId":    nodeID,
		"roleId":    role,
		"receivers": userIDs,
	}
	if notifyMode := normalizePermissionRole(mustGetFlag(cmd, "notify-mode")); notifyMode != "" {
		if !stringInSet(notifyMode, "DEFAULT", "MSG_ACCOUNT", "SINGLE_CHAT") {
			return fmt.Errorf("invalid --notify-mode %q: use DEFAULT, MSG_ACCOUNT, or SINGLE_CHAT", notifyMode)
		}
		toolArgs["notifyMode"] = notifyMode
	}
	if reason := strings.TrimSpace(mustGetFlag(cmd, "reason")); reason != "" {
		if utf8.RuneCountInString(reason) > 200 {
			return fmt.Errorf("--reason must not exceed 200 characters")
		}
		toolArgs["reason"] = reason
	}

	if !deps.Caller.DryRun() && !confirmDangerousAction(cmd, "submit permission request", fmt.Sprintf("node=%s role=%s approvers=%s", nodeID, role, strings.Join(userIDs, ","))) {
		return nil
	}
	return callMCPToolOnServer("drive", "apply_permission", toolArgs)
}

func newDriveStarCommand() *cobra.Command {
	starCmd := &cobra.Command{
		Use:   "star",
		Short: "文档收藏管理",
		Long:  `管理文档资源收藏：添加收藏、取消收藏和查看收藏列表。`,
		RunE:  groupRunE,
	}

	addCmd := &cobra.Command{
		Use:     "add",
		Short:   "收藏文档",
		Example: `  dws drive star add --node <nodeId_or_URL>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "mark_star", map[string]any{"nodeId": nodeID})
		},
	}
	addCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")

	removeCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "取消收藏文档",
		Example: `  dws drive star remove --node <nodeId_or_URL>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "unmark_star", map[string]any{"nodeId": nodeID})
		},
	}
	removeCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "获取收藏列表",
		Long:  `获取当前用户收藏的文档资源，支持分页、排序和资源类型筛选。`,
		Example: `  dws drive star list
  dws drive star list --content-types doc,sheet
  dws drive star list --resource-types DENTRY --limit 10`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolArgs := map[string]any{}
			if value, _ := cmd.Flags().GetInt("limit"); value > 0 {
				if value > 20 {
					return fmt.Errorf("--limit must not exceed 20")
				}
				toolArgs["limit"] = value
			}
			if value, _ := cmd.Flags().GetString("cursor"); value != "" {
				toolArgs["cursor"] = value
			}
			if value, _ := cmd.Flags().GetString("order-by"); value != "" {
				if value != "createTime" {
					return fmt.Errorf("invalid --order-by %q: use createTime", value)
				}
				toolArgs["orderBy"] = value
			}
			if value, _ := cmd.Flags().GetString("sort"); value != "" {
				value = strings.ToLower(value)
				if !stringInSet(value, "asc", "desc") {
					return fmt.Errorf("invalid --sort %q: use asc or desc", value)
				}
				toolArgs["sortType"] = value
			}
			if value, _ := cmd.Flags().GetStringSlice("resource-types"); len(value) > 0 {
				for index := range value {
					value[index] = strings.ToUpper(strings.TrimSpace(value[index]))
					if !stringInSet(value[index], "DENTRY", "TEAM", "WORKSPACE") {
						return fmt.Errorf("invalid --resource-types value %q", value[index])
					}
				}
				toolArgs["supportResourceTypes"] = value
			}
			if value, _ := cmd.Flags().GetStringSlice("content-types"); len(value) > 0 {
				for index := range value {
					value[index] = strings.ToLower(strings.TrimSpace(value[index]))
					if !stringInSet(value[index], "doc", "sheet", "ppt", "whiteboard", "mind", "notable", "pdf", "other", "folder", "workspace", "team") {
						return fmt.Errorf("invalid --content-types value %q", value[index])
					}
				}
				toolArgs["contentTypes"] = value
			}
			return callMCPToolOnServer("drive", "get_star_list", toolArgs)
		},
	}
	listCmd.Flags().Int("limit", 0, "每页条数 (默认 20，最大 20)")
	listCmd.Flags().String("cursor", "", "分页游标")
	listCmd.Flags().String("order-by", "", "排序字段: createTime")
	listCmd.Flags().String("sort", "", "排序方向: asc|desc")
	listCmd.Flags().StringSlice("resource-types", nil, "资源大类: DENTRY,TEAM,WORKSPACE")
	listCmd.Flags().StringSlice("content-types", nil, "内容类型: doc,sheet,ppt,whiteboard,mind,notable,pdf,other,folder,workspace,team")

	for _, command := range []*cobra.Command{addCmd, removeCmd} {
		addHiddenNodeAliases(command)
	}
	starCmd.AddCommand(addCmd, removeCmd, listCmd)
	return starCmd
}

func addHiddenNodeAliases(command *cobra.Command) {
	for _, name := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		command.Flags().String(name, "", "")
		_ = command.Flags().MarkHidden(name)
	}
}

func stringInSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
