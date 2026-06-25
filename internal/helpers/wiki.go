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
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return wikiHandler{}
	})
}

// wikiHandler only fills command behavior the service-discovery envelope cannot
// express yet. The rest of the wiki surface remains owned by dynamic config.
type wikiHandler struct{}

func (wikiHandler) Name() string {
	return "wiki"
}

func (wikiHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "wiki",
		Short:             "知识库扩展命令（合并到 dws wiki 命令树）",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	addWikiProxyCommands(root, runner)

	space := &cobra.Command{
		Use:               "space",
		Short:             "知识库空间",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	space.AddCommand(
		newWikiSpaceCreateCommand(runner),
		newWikiSpaceGetCommand(runner),
		newWikiSpaceListCommand(runner),
		newWikiSpaceSearchCommand(runner),
		newWikiSpaceDeleteCommand(runner),
	)
	root.AddCommand(space)

	member := &cobra.Command{
		Use:               "member",
		Short:             "知识库成员",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	member.AddCommand(
		newWikiMemberAddCommand(runner),
		newWikiMemberUpdateCommand(runner),
		newWikiMemberListCommand(runner),
		newWikiMemberRemoveCommand(runner),
	)
	root.AddCommand(member)
	return root
}

func newWikiSpaceCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建知识库",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := wikiRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			if description := wikiFlagOrFallback(cmd, "desc", "description"); description != "" {
				params["description"] = description
			}
			if icon := wikiFlagOrFallback(cmd, "icon"); icon != "" {
				params["icon"] = icon
			}
			return runWikiTool(cmd, runner, "create_wikiSpace", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", "知识库名称 (必填)")
	cmd.Flags().String("desc", "", "知识库描述")
	addWikiHiddenStringFlag(cmd, "description", "--desc 的兼容别名")
	cmd.Flags().String("icon", "", "知识库图标标识")
	return cmd
}

func newWikiSpaceGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查看知识库详情",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "id", "space", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			return runWikiTool(cmd, runner, "get_wikiSpace", map[string]any{
				"workspaceId": workspaceID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	addWikiHiddenStringFlag(cmd, "id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "space", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspaceId", "--workspace 的兼容别名")
	return cmd
}

func newWikiSpaceListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"ls"},
		Short:             "列出知识库",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if spaceType := wikiFlagOrFallback(cmd, "type"); spaceType != "" {
				if spaceType == "orgSpace" || spaceType == "mySpace" {
					driveParams := map[string]any{"spaceType": spaceType}
					if limit := wikiFlagOrFallback(cmd, "limit", "page-size"); limit != "" {
						if n, err := strconv.Atoi(limit); err == nil {
							driveParams["maxResults"] = n
						} else {
							driveParams["maxResults"] = limit
						}
					}
					if pageToken := wikiFlagOrFallback(cmd, "cursor", "page-token"); pageToken != "" {
						driveParams["nextToken"] = pageToken
					}
					return runWikiProductTool(cmd, runner, "drive", "list_spaces", driveParams)
				}
				params["wikiSpaceType"] = spaceType
			}
			if limit := wikiFlagOrFallback(cmd, "limit", "page-size"); limit != "" {
				params["pageSize"] = limit
			}
			if pageToken := wikiFlagOrFallback(cmd, "cursor", "page-token"); pageToken != "" {
				params["pageToken"] = pageToken
			}
			return runWikiTool(cmd, runner, "list_wikiSpaces", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("type", "orgWikiSpace", "知识库类型: myWikiSpace / orgWikiSpace")
	cmd.Flags().String("limit", "", "每页数量 1-50 (默认 20)")
	cmd.Flags().String("page-size", "", "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("cursor", "", "分页游标")
	addWikiHiddenStringFlag(cmd, "page-token", "--cursor 的兼容别名")
	return cmd
}

func newWikiSpaceSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "搜索知识库",
		Example: `  dws wiki space search --query 产品文档
  dws wiki space search --query 技术方案 --limit 20
  dws wiki space search --type myWikiSpace`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := wikiFlagOrFallback(cmd, "query", "keyword")
			spaceType, _ := cmd.Flags().GetString("type")
			limit, _ := cmd.Flags().GetString("limit")

			params := map[string]any{}
			if strings.TrimSpace(limit) != "" {
				params["pageSize"] = limit
			}

			if strings.TrimSpace(keyword) != "" {
				params["keyword"] = strings.TrimSpace(keyword)
				return runWikiTool(cmd, runner, "search_wikiSpaces", params)
			}

			if strings.TrimSpace(spaceType) == "myWikiSpace" {
				params["wikiSpaceType"] = "myWikiSpace"
				return runWikiTool(cmd, runner, "list_wikiSpaces", params)
			}

			return apperrors.NewValidation("--query/--keyword is required unless --type myWikiSpace is specified")
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "搜索关键词")
	addWikiHiddenStringFlag(cmd, "keyword", "--query 的兼容别名")
	cmd.Flags().String("type", "", "知识库类型；仅 --type myWikiSpace 支持无 keyword 查询个人知识库")
	cmd.Flags().String("limit", "", "返回数量 1-20 (默认 10)")
	return cmd
}

func newWikiSpaceDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             "删除知识库",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId", "id")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, "知识库", workspaceID) {
				return nil
			}
			return runWikiTool(cmd, runner, "delete_wikiSpace", map[string]any{
				"workspaceId": workspaceID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addWikiWorkspaceFlag(cmd)
	return cmd
}

func newWikiMemberAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "添加知识库成员",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "id", "space", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			user, err := wikiRequiredFlagOrFallback(cmd, "users", "user")
			if err != nil {
				return err
			}
			role, err := wikiRequiredFlag(cmd, "role")
			if err != nil {
				return err
			}
			return runWikiTool(cmd, runner, "add_member", map[string]any{
				"workspaceId": workspaceID,
				"userIds":     wikiCSV(user),
				"roleId":      strings.ToUpper(strings.TrimSpace(role)),
			})
		},
	}
	preferLegacyLeaf(cmd)
	addWikiWorkspaceFlag(cmd)
	cmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	addWikiHiddenStringFlag(cmd, "user", "--users 的兼容别名")
	cmd.Flags().String("role", "", "权限角色 (必填)")
	return cmd
}

func newWikiMemberUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             "更新知识库成员权限",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "id", "space", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			user, err := wikiRequiredFlagOrFallback(cmd, "users", "user", "uid")
			if err != nil {
				return err
			}
			role, err := wikiRequiredFlag(cmd, "role")
			if err != nil {
				return err
			}
			return runWikiTool(cmd, runner, "update_member", map[string]any{
				"workspaceId": workspaceID,
				"userIds":     wikiCSV(user),
				"roleId":      strings.ToUpper(strings.TrimSpace(role)),
			})
		},
	}
	preferLegacyLeaf(cmd)
	addWikiWorkspaceFlag(cmd)
	cmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	addWikiHiddenStringFlag(cmd, "user", "--users 的兼容别名")
	addWikiHiddenStringFlag(cmd, "uid", "--users 的兼容别名")
	cmd.Flags().String("role", "", "权限角色 (必填)")
	return cmd
}

func newWikiMemberListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"ls"},
		Short:             "查询知识库成员",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			params := map[string]any{"workspaceId": workspaceID}
			if maxResults := wikiIntFlagOrFallback(cmd, "limit", "max-results", "page-size"); maxResults > 0 {
				params["maxResults"] = maxResults
			}
			if filterRole, _ := cmd.Flags().GetString("filter-role"); strings.TrimSpace(filterRole) != "" {
				params["filterRoleIds"] = wikiCSV(filterRole)
			}
			return runWikiTool(cmd, runner, "list_member", params)
		},
	}
	preferLegacyLeaf(cmd)
	addWikiMemberListWorkspaceFlag(cmd)
	cmd.Flags().Int("limit", 0, "返回成员数上限")
	cmd.Flags().Int("max-results", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("max-results")
	cmd.Flags().Int("page-size", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("filter-role", "", "按角色过滤，逗号分隔")
	return cmd
}

func newWikiMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Aliases:           []string{"rm"},
		Short:             "移除知识库成员",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			user, err := wikiRequiredFlagOrFallback(cmd, "users", "user", "uid")
			if err != nil {
				return err
			}
			return runWikiTool(cmd, runner, "remove_member", map[string]any{
				"workspaceId": workspaceID,
				"userIds":     wikiCSV(user),
			})
		},
	}
	preferLegacyLeaf(cmd)
	addWikiMemberListWorkspaceFlag(cmd)
	cmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	addWikiHiddenStringFlag(cmd, "user", "--users 的兼容别名")
	addWikiHiddenStringFlag(cmd, "uid", "--users 的兼容别名")
	return cmd
}

func newWikiNodeListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"ls"},
		Short:             "列出知识库节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			params := map[string]any{"workspaceId": workspaceID}
			if folder := wikiFlagOrFallback(cmd, "folder", "node", "parent-id"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			if limit := wikiIntFlagOrFallback(cmd, "limit", "page-size"); limit > 0 {
				params["pageSize"] = limit
			}
			if cursor := wikiFlagOrFallback(cmd, "cursor", "page-token"); cursor != "" {
				params["pageToken"] = cursor
			}
			return runWikiProductTool(cmd, runner, "doc", "list_nodes", params)
		},
	}
	preferLegacyLeaf(cmd)
	addWikiNodeWorkspaceFlag(cmd)
	cmd.Flags().String("folder", "", "父节点 nodeId")
	addWikiHiddenStringFlag(cmd, "node", "--folder 的兼容别名")
	addWikiHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	cmd.Flags().Int("limit", 0, "每页数量")
	cmd.Flags().Int("page-size", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("cursor", "", "分页游标")
	addWikiHiddenStringFlag(cmd, "page-token", "--cursor 的兼容别名")
	return cmd
}

func newWikiNodeCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "在知识库中创建节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			name, err := wikiRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{
				"workspaceId": workspaceID,
				"name":        name,
			}
			if nodeType := wikiFlagOrFallback(cmd, "type"); nodeType != "" {
				params["type"] = nodeType
			}
			if folder := wikiFlagOrFallback(cmd, "folder", "parent-id"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			return runWikiProductTool(cmd, runner, "doc", "create_file", params)
		},
	}
	preferLegacyLeaf(cmd)
	addWikiNodeWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "节点名称 (必填)")
	cmd.Flags().String("type", "adoc", "节点类型: adoc / asheet / folder / axls")
	cmd.Flags().String("folder", "", "父节点 nodeId")
	addWikiHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	return cmd
}

func newWikiNodeCopyCommand(runner executor.Runner) *cobra.Command {
	return newWikiNodeTransferCommand(runner, "copy", "copy_document")
}

func newWikiNodeMoveCommand(runner executor.Runner) *cobra.Command {
	return newWikiNodeTransferCommand(runner, "move", "move_document")
}

func newWikiNodeTransferCommand(runner executor.Runner, use, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             use + " 知识库节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			nodeID, err := wikiRequiredFlagOrFallback(cmd, "node", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"nodeId":      normalizeDocNodeID(nodeID),
				"workspaceId": workspaceID,
			}
			if folder := wikiFlagOrFallback(cmd, "folder", "parent-id", "parent-node-id", "parent-folder-id"); folder != "" {
				params["targetFolderId"] = normalizeDocNodeID(folder)
			}
			return runWikiProductTool(cmd, runner, "doc", tool, params)
		},
	}
	preferLegacyLeaf(cmd)
	addWikiNodeWorkspaceFlag(cmd)
	addWikiNodeIDFlags(cmd)
	cmd.Flags().String("folder", "", "目标文件夹 nodeId")
	addWikiHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	addWikiHiddenStringFlag(cmd, "parent-node-id", "--folder 的兼容别名")
	addWikiHiddenStringFlag(cmd, "parent-folder-id", "--folder 的兼容别名")
	return cmd
}

func newWikiNodeDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             "删除知识库节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId"); err != nil {
				return err
			}
			nodeID, err := wikiRequiredFlagOrFallback(cmd, "node", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, "知识库节点", nodeID) {
				return nil
			}
			return runWikiProductTool(cmd, runner, "doc", "delete_document", map[string]any{
				"nodeId": normalizeDocNodeID(nodeID),
			})
		},
	}
	preferLegacyLeaf(cmd)
	addWikiNodeWorkspaceFlag(cmd)
	addWikiNodeIDFlags(cmd)
	cmd.Flags().BoolP("yes", "y", false, "跳过确认直接删除")
	return cmd
}

func newWikiNodeSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "在知识库中搜索节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := wikiRequiredFlagOrFallback(cmd, "workspace", "workspace-id", "workspaceId")
			if err != nil {
				return err
			}
			query := wikiFlagOrFallback(cmd, "query", "keyword")
			if query == "" {
				return apperrors.NewValidation("--query is required")
			}
			params := map[string]any{
				"keyword":      query,
				"workspaceIds": []string{workspaceID},
			}
			if values, _ := cmd.Flags().GetStringSlice("extensions"); len(values) > 0 {
				params["extensions"] = splitWikiStringSlice(values)
			}
			if limit := wikiIntFlagOrFallback(cmd, "limit"); limit > 0 {
				params["pageSize"] = limit
			}
			if cursor := wikiFlagOrFallback(cmd, "cursor", "page-token"); cursor != "" {
				params["pageToken"] = cursor
			}
			return runWikiProductTool(cmd, runner, "doc", "search_documents", params)
		},
	}
	preferLegacyLeaf(cmd)
	addWikiNodeWorkspaceFlag(cmd)
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	addWikiHiddenStringFlag(cmd, "keyword", "--query 的兼容别名")
	cmd.Flags().StringSlice("extensions", nil, "按扩展名过滤，逗号分隔")
	cmd.Flags().Int("limit", 0, "每页数量")
	cmd.Flags().String("cursor", "", "分页游标")
	addWikiHiddenStringFlag(cmd, "page-token", "--cursor 的兼容别名")
	return cmd
}

func runWikiTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	return runWikiProductTool(cmd, runner, "wiki", tool, params)
}

func runWikiProductTool(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		product,
		tool,
		params,
	)
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, invocation)
	}
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func addWikiWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	addWikiHiddenStringFlag(cmd, "id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "space", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspaceId", "--workspace 的兼容别名")
}

func addWikiMemberListWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	addWikiHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspaceId", "--workspace 的兼容别名")
}

func addWikiNodeWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	addWikiHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	addWikiHiddenStringFlag(cmd, "workspaceId", "--workspace 的兼容别名")
}

func addWikiNodeIDFlags(cmd *cobra.Command) {
	cmd.Flags().String("node", "", "节点 ID 或 URL (必填)")
	addWikiHiddenStringFlag(cmd, "node-id", "--node 的兼容别名")
	addWikiHiddenStringFlag(cmd, "doc-id", "--node 的兼容别名")
	addWikiHiddenStringFlag(cmd, "file-id", "--node 的兼容别名")
}

func addWikiHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}

func wikiRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	value, _ := cmd.Flags().GetString(name)
	value = strings.TrimSpace(value)
	if value == "" {
		return "", apperrors.NewValidation("--" + name + " is required")
	}
	return value, nil
}

func wikiRequiredFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	if value := wikiFlagOrFallback(cmd, primary, aliases...); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation("--" + primary + " is required")
}

func wikiFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	names := append([]string{primary}, aliases...)
	for _, name := range names {
		value, err := cmd.Flags().GetString(name)
		if err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func wikiIntFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int {
	names := append([]string{primary}, aliases...)
	for _, name := range names {
		value, err := cmd.Flags().GetInt(name)
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func wikiCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func splitWikiStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if item := strings.TrimSpace(part); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func wikiUnsupportedCommand(name string) error {
	return fmt.Errorf("unsupported wiki command: %s", name)
}
