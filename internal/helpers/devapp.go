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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	devAppProduct = "devapp"

	devAppMemberListTool     = "list_open_dev_app_members"
	devAppMemberAddTool      = "add_open_dev_app_members"
	devAppMemberRemoveTool   = "remove_open_dev_app_members"
	devAppSecurityConfigTool = "update_app_security_config"
)

func init() {
	RegisterPublic(func() Handler {
		return devAppHandler{}
	})
}

type devAppHandler struct{}

func (devAppHandler) Name() string {
	return "devapp"
}

func (devAppHandler) Command(runner executor.Runner) *cobra.Command {
	return newDevAppCommand(runner)
}

func newDevAppCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "devapp",
		Aliases:           []string{"app"},
		Short:             "开放平台应用",
		Long:              "管理开放平台开发者应用：查询、详情、创建、更新、启停、删除、权限、网页应用、成员和安全配置。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	webapp := &cobra.Command{
		Use:               "webapp",
		Short:             "开放平台网页应用配置",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	webapp.AddCommand(
		newDevAppWebappGetCommand(runner),
		newDevAppWebappConfigCommand(runner),
	)

	permission := &cobra.Command{
		Use:               "permission",
		Short:             "开放平台应用权限",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	permission.AddCommand(
		newDevAppPermissionListCommand(runner),
		newDevAppPermissionAddCommand(runner),
		newDevAppPermissionRemoveCommand(runner),
	)

	member := &cobra.Command{
		Use:               "member",
		Short:             "开放平台应用成员管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	member.AddCommand(
		newDevAppMemberListCommand(runner),
		newDevAppMemberAddCommand(runner),
		newDevAppMemberRemoveCommand(runner),
	)

	security := &cobra.Command{
		Use:               "security",
		Short:             "开放平台应用安全设置",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	security.AddCommand(newDevAppSecurityConfigCommand(runner))

	root.AddCommand(
		newDevAppListCommand(runner),
		newDevAppGetCommand(runner),
		newDevAppCreateCommand(runner),
		newDevAppUpdateCommand(runner),
		newDevAppLifecycleCommand(runner, "delete", "删除开放平台企业内部应用", "delete_inner_app"),
		newDevAppLifecycleCommand(runner, "inactive", "停用开放平台企业内部应用", "inactive_inner_app"),
		newDevAppLifecycleCommand(runner, "active", "启用开放平台企业内部应用", "active_inner_app"),
		webapp,
		permission,
		member,
		security,
	)
	return root
}

func newDevAppListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询开放平台企业内部应用列表",
		Example:           "  dws devapp list --name DemoApp --page 1 --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{
				"currentPage": devAppIntFlag(cmd, "page"),
				"pageSize":    devAppIntFlag(cmd, "page-size"),
			}
			devAppPutString(params, "appName", devAppFlagOrFallback(cmd, "name", "keyword"))
			devAppPutInt(params, "agentId", devAppIntFlag(cmd, "agent-id"))
			devAppPutInt(params, "appId", devAppIntFlag(cmd, "app-id"))
			devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
			devAppPutString(params, "customKey", devAppStringFlag(cmd, "custom-key"))
			devAppPutInt(params, "appGroupId", devAppIntFlag(cmd, "app-group-id"))
			devAppPutString(params, "creator", devAppStringFlag(cmd, "creator"))
			devAppPutString(params, "robotName", devAppStringFlag(cmd, "robot-name"))
			devAppPutInt(params, "developType", devAppIntFlag(cmd, "develop-type"))
			devAppPutInt(params, "filterCoolApp", devAppIntFlag(cmd, "filter-cool-app"))
			devAppPutString(params, "sortType", devAppStringFlag(cmd, "sort"))
			devAppPutString(params, "sortOrder", devAppStringFlag(cmd, "order"))
			return runDevAppTool(runner, cmd, "list_open_dev_apps_by_condition", params)
		},
	}
	cmd.Flags().Int("page", 1, "分页页码，从 1 开始")
	cmd.Flags().Int("page-size", 20, "分页大小")
	cmd.Flags().String("name", "", "应用名称关键词")
	cmd.Flags().String("keyword", "", "--name 的兼容别名")
	_ = cmd.Flags().MarkHidden("keyword")
	addDevAppLocatorFlagsWithoutName(cmd)
	cmd.Flags().Int("app-group-id", 0, "应用分组 ID")
	cmd.Flags().String("creator", "", "创建人名称关键词")
	cmd.Flags().String("robot-name", "", "机器人名称关键词")
	cmd.Flags().Int("develop-type", 0, "开发类型枚举；不确定时不要传")
	cmd.Flags().Int("filter-cool-app", 0, "酷应用过滤枚举；不确定时不要传")
	cmd.Flags().String("sort", "", "排序字段，如 gmt_modified")
	cmd.Flags().String("order", "", "排序方向 asc 或 desc")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询开放平台企业内部应用详情",
		Example:           "  dws devapp get --agent-id 123456 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppLocatorParams(cmd, true)
			if len(params) == 0 {
				return devAppLocatorRequired(true)
			}
			return runDevAppTool(runner, cmd, "get_open_dev_app_detail", params)
		},
	}
	addDevAppLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建开放平台企业内部应用",
		Example:           "  dws devapp create --name DemoApp --desc 内部应用 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "create"); err != nil {
				return err
			}
			appType := devAppStringFlag(cmd, "type")
			if appType != "" && appType != "internal" {
				return apperrors.NewValidation("--type currently only supports internal")
			}
			name := devAppStringFlag(cmd, "name")
			if name == "" {
				return apperrors.NewValidation("--name is required")
			}
			params := map[string]any{"appName": name}
			devAppPutString(params, "appDesc", devAppStringFlag(cmd, "desc"))
			devAppPutString(params, "appIcon", devAppStringFlag(cmd, "icon"))
			return runDevAppTool(runner, cmd, "create_inner_app", params)
		},
	}
	cmd.Flags().String("name", "", "应用名称 (必填)")
	cmd.Flags().String("desc", "", "应用描述")
	cmd.Flags().String("icon", "", "应用图标 mediaId")
	cmd.Flags().String("type", "internal", "应用类型；当前仅支持 internal")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             "修改开放平台企业内部应用基础信息",
		Example:           "  dws devapp update --agent-id 123456 --name DemoApp2 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "update"); err != nil {
				return err
			}
			params := devAppLocatorParams(cmd, false)
			if len(params) == 0 {
				return devAppLocatorRequired(false)
			}
			updates := 0
			if v := devAppStringFlag(cmd, "name"); v != "" {
				params["appName"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "desc"); v != "" {
				params["appDesc"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "icon"); v != "" {
				params["appIcon"] = v
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("at least one update field is required: --name, --desc, or --icon")
			}
			return runDevAppTool(runner, cmd, "update_inner_app", params)
		},
	}
	addDevAppLocatorFlagsWithoutName(cmd)
	cmd.Flags().String("name", "", "新的应用名称")
	cmd.Flags().String("desc", "", "新的应用描述")
	cmd.Flags().String("icon", "", "新的应用图标 mediaId")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppLifecycleCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, use); err != nil {
				return err
			}
			params := devAppLocatorParams(cmd, false)
			if len(params) == 0 {
				return devAppLocatorRequired(false)
			}
			return runDevAppTool(runner, cmd, tool, params)
		},
	}
	addDevAppLocatorFlagsWithoutName(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppWebappGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询网页应用配置",
		Example:           "  dws devapp webapp get --agent-id 123456 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppAppLocatorParams(cmd)
			if len(params) == 0 {
				return devAppAppLocatorRequired()
			}
			return runDevAppTool(runner, cmd, "get_webapp_config", params)
		},
	}
	addDevAppAppLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppWebappConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "配置网页应用能力",
		Example:           "  dws devapp webapp config --agent-id 123456 --homepage-link https://example.com --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "webapp config"); err != nil {
				return err
			}
			params := devAppAppLocatorParams(cmd)
			if len(params) == 0 {
				return devAppAppLocatorRequired()
			}
			updates := 0
			if v := devAppStringFlag(cmd, "h5-page-type"); v != "" {
				params["h5PageType"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "homepage-link"); v != "" {
				params["homepageLink"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "pc-homepage-link"); v != "" {
				params["pcHomepageLink"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "omp-link"); v != "" {
				params["ompLink"] = v
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("at least one webapp field is required: --h5-page-type, --homepage-link, --pc-homepage-link, or --omp-link")
			}
			return runDevAppTool(runner, cmd, "set_webapp_config", params)
		},
	}
	addDevAppAppLocatorFlags(cmd)
	cmd.Flags().String("h5-page-type", "", "网页应用生效端/页面类型")
	cmd.Flags().String("homepage-link", "", "移动端首页地址")
	cmd.Flags().String("pc-homepage-link", "", "PC 端首页地址")
	cmd.Flags().String("omp-link", "", "管理后台地址")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppPermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"search", "detail"},
		Short:             "查询开放平台应用权限列表",
		Example:           "  dws devapp permission list --agent-id 123456 --keyword 通讯录 --limit 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppAppLocatorParams(cmd)
			if len(params) == 0 {
				return devAppAppLocatorRequired()
			}
			devAppPutString(params, "keyword", devAppStringFlag(cmd, "keyword"))
			devAppPutString(params, "scopeValue", devAppFlagOrFallback(cmd, "scope", "permission"))
			devAppPutString(params, "authStatus", strings.ToUpper(devAppStringFlag(cmd, "status")))
			devAppPutString(params, "firstLevelType", strings.ToUpper(devAppStringFlag(cmd, "scope-type")))
			devAppPutString(params, "apiStatus", devAppStringFlag(cmd, "api-status"))
			devAppPutInt(params, "limit", devAppIntFlag(cmd, "limit"))
			devAppPutInt(params, "offset", devAppIntFlag(cmd, "offset"))
			return runDevAppTool(runner, cmd, "list_open_dev_app_permissions", params)
		},
	}
	addDevAppAppLocatorFlags(cmd)
	cmd.Flags().String("keyword", "", "权限名、权限点、接口名关键词")
	cmd.Flags().String("scope", "", "精确权限点 scopeValue")
	cmd.Flags().String("permission", "", "--scope 的兼容别名")
	_ = cmd.Flags().MarkHidden("permission")
	cmd.Flags().String("status", "ALL", "权限状态：ALL、AUTHED、UNAUTHED")
	cmd.Flags().String("scope-type", "", "权限一级类型：APP 或 SNS")
	cmd.Flags().String("api-status", "", "开发者后台 apiStatus 过滤")
	cmd.Flags().Int("limit", 20, "返回数量上限")
	cmd.Flags().Int("offset", 0, "返回偏移量")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppPermissionAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "申请开放平台应用权限点",
		Example:           "  dws devapp permission add --agent-id 123456 --permissions Contact.User.mobile --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission add"); err != nil {
				return err
			}
			params := devAppAppLocatorParams(cmd)
			if len(params) == 0 {
				return devAppAppLocatorRequired()
			}
			scopes := devAppPermissionScopes(cmd)
			if len(scopes) == 0 {
				return apperrors.NewValidation("--permissions is required")
			}
			params["scopeValues"] = scopes
			return runDevAppTool(runner, cmd, "apply_open_dev_app_permissions", params)
		},
	}
	addDevAppAppLocatorFlags(cmd)
	cmd.Flags().StringSlice("permissions", nil, "权限点 scopeValue，多个用逗号分隔")
	cmd.Flags().String("scope", "", "--permissions 的兼容别名，单个权限点 scopeValue")
	cmd.Flags().String("permission", "", "--permissions 的兼容别名，单个权限点 scopeValue")
	_ = cmd.Flags().MarkHidden("scope")
	_ = cmd.Flags().MarkHidden("permission")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppPermissionRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             "取消开放平台应用权限点",
		Example:           "  dws devapp permission remove --agent-id 123456 --permission Contact.User.mobile --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission remove"); err != nil {
				return err
			}
			params := devAppAppLocatorParams(cmd)
			if len(params) == 0 {
				return devAppAppLocatorRequired()
			}
			scope := devAppFlagOrFallback(cmd, "permission", "scope")
			if scope == "" {
				return apperrors.NewValidation("--permission is required")
			}
			params["scopeValue"] = scope
			return runDevAppTool(runner, cmd, "remove_open_dev_app_permission", params)
		},
	}
	addDevAppAppLocatorFlags(cmd)
	cmd.Flags().String("permission", "", "待取消权限点 scopeValue")
	cmd.Flags().String("scope", "", "--permission 的兼容别名")
	_ = cmd.Flags().MarkHidden("scope")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppMemberListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询开放平台应用成员",
		Example:           "  dws devapp member list --app-id <unifiedAppId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppMemberListTool, map[string]any{
				"unifiedAppId": appID,
			})
		},
	}
	cmd.Flags().String("app-id", "", "开放平台统一应用 ID (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppMemberAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "添加开放平台应用成员",
		Example:           "  dws devapp member add --app-id <unifiedAppId> --users userId1,userId2 --member-type DEVELOPER --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevAppMemberMutation(runner, cmd, devAppMemberAddTool, "member add")
		},
	}
	registerDevAppMemberMutationFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             "移除开放平台应用成员",
		Example:           "  dws devapp member remove --app-id <unifiedAppId> --users userId1,userId2 --member-type DEVELOPER --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevAppMemberMutation(runner, cmd, devAppMemberRemoveTool, "member remove")
		},
	}
	registerDevAppMemberMutationFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppSecurityConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "更新开放平台应用安全配置",
		Example: "  dws devapp security config --app-id <unifiedAppId> " +
			"--ip-whitelist 103.211.230.150 --redirect-url https://example.com/callback --sso-url https://example.com/sso --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "security config"); err != nil {
				return err
			}
			appID, err := requiredDevAppID(cmd)
			if err != nil {
				return err
			}

			params := map[string]any{"unifiedAppId": appID}
			if values := parseDevAppListFlag(cmd, "ip-whitelist"); len(values) > 0 {
				params["ipWhiteList"] = values
			}
			if values := parseDevAppListFlag(cmd, "redirect-url"); len(values) > 0 {
				params["redirectUrls"] = values
			}
			if values := parseDevAppListFlag(cmd, "sso-url"); len(values) > 0 {
				params["otherAuthUrls"] = values
			}
			if len(params) == 1 {
				return apperrors.NewValidation("one of --ip-whitelist, --redirect-url, or --sso-url is required")
			}
			return runDevAppTool(runner, cmd, devAppSecurityConfigTool, params)
		},
	}
	cmd.Flags().String("app-id", "", "开放平台统一应用 ID (必填)")
	cmd.Flags().String("ip-whitelist", "", "出口 IP 白名单，多个用逗号或分号分隔")
	cmd.Flags().String("redirect-url", "", "登录重定向 URL，多个用逗号或分号分隔")
	cmd.Flags().String("sso-url", "", "端内免登地址，多个用逗号或分号分隔")
	preferLegacyLeaf(cmd)
	return cmd
}

func registerDevAppMemberMutationFlags(cmd *cobra.Command) {
	cmd.Flags().String("app-id", "", "开放平台统一应用 ID (必填)")
	cmd.Flags().String("users", "", "成员 userId 列表，多个用逗号分隔 (必填)")
	cmd.Flags().String("member-type", "", "成员类型，如 DEVELOPER (必填)")
}

func runDevAppMemberMutation(runner executor.Runner, cmd *cobra.Command, tool, operation string) error {
	if err := devAppRequireWriteGuard(cmd, operation); err != nil {
		return err
	}
	appID, err := requiredDevAppID(cmd)
	if err != nil {
		return err
	}
	users, err := requiredDevAppUsers(cmd)
	if err != nil {
		return err
	}
	memberType, err := requiredDevAppMemberType(cmd)
	if err != nil {
		return err
	}

	params := map[string]any{
		"unifiedAppId":  appID,
		"memberUserIds": users,
		"memberType":    memberType,
	}
	return runDevAppTool(runner, cmd, tool, params)
}

func runDevAppTool(runner executor.Runner, cmd *cobra.Command, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		devAppProduct,
		tool,
		params,
	)
	invocation.DryRun = commandDryRun(cmd)
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func requiredDevAppID(cmd *cobra.Command) (string, error) {
	appID, _ := cmd.Flags().GetString("app-id")
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return "", apperrors.NewValidation("--app-id is required")
	}
	return appID, nil
}

func requiredDevAppUsers(cmd *cobra.Command) ([]string, error) {
	usersRaw, _ := cmd.Flags().GetString("users")
	if strings.TrimSpace(usersRaw) == "" {
		return nil, apperrors.NewValidation("--users is required")
	}
	users := splitDevAppList(usersRaw)
	if len(users) == 0 {
		return nil, apperrors.NewValidation("--users must contain at least one userId")
	}
	return users, nil
}

func requiredDevAppMemberType(cmd *cobra.Command) (string, error) {
	memberType, _ := cmd.Flags().GetString("member-type")
	memberType = strings.TrimSpace(memberType)
	if memberType == "" {
		return "", apperrors.NewValidation("--member-type is required")
	}
	return memberType, nil
}

func parseDevAppListFlag(cmd *cobra.Command, name string) []string {
	raw, _ := cmd.Flags().GetString(name)
	return splitDevAppList(raw)
}

func splitDevAppList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func addDevAppLocatorFlags(cmd *cobra.Command) {
	addDevAppLocatorFlagsWithoutName(cmd)
	cmd.Flags().String("name", "", "应用名称关键词；写操作前必须唯一命中")
}

func addDevAppLocatorFlagsWithoutName(cmd *cobra.Command) {
	addDevAppAppLocatorFlags(cmd)
	cmd.Flags().Int("app-id", 0, "兼容 appId")
	cmd.Flags().String("app-key", "", "appKey/clientId")
	cmd.Flags().String("custom-key", "", "customKey")
}

func addDevAppAppLocatorFlags(cmd *cobra.Command) {
	cmd.Flags().String("unified-app-id", "", "统一应用 ID")
	cmd.Flags().Int("agent-id", 0, "应用 agentId")
}

func devAppLocatorParams(cmd *cobra.Command, includeName bool) map[string]any {
	params := devAppAppLocatorParams(cmd)
	devAppPutInt(params, "appId", devAppIntFlag(cmd, "app-id"))
	devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
	devAppPutString(params, "customKey", devAppStringFlag(cmd, "custom-key"))
	if includeName {
		devAppPutString(params, "appName", devAppStringFlag(cmd, "name"))
	}
	return params
}

func devAppAppLocatorParams(cmd *cobra.Command) map[string]any {
	params := map[string]any{}
	devAppPutString(params, "unifiedAppId", devAppStringFlag(cmd, "unified-app-id"))
	devAppPutInt(params, "agentId", devAppIntFlag(cmd, "agent-id"))
	return params
}

func devAppLocatorRequired(includeName bool) error {
	if includeName {
		return apperrors.NewValidation("one app locator is required: --agent-id, --unified-app-id, --app-key, --custom-key, --app-id, or --name")
	}
	return apperrors.NewValidation("one app locator is required: --agent-id, --unified-app-id, --app-key, --custom-key, or --app-id")
}

func devAppAppLocatorRequired() error {
	return apperrors.NewValidation("one app locator is required: --agent-id or --unified-app-id")
}

func devAppRequireWriteGuard(cmd *cobra.Command, operation string) error {
	if commandDryRun(cmd) || devAppYes(cmd) {
		return nil
	}
	return apperrors.NewValidation(fmt.Sprintf("%s is a write operation; rerun with --dry-run to preview or --yes after confirmation", operation))
}

func devAppYes(cmd *cobra.Command) bool {
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags(), cmd.Root().PersistentFlags()} {
		if flags == nil || flags.Lookup("yes") == nil {
			continue
		}
		if value, err := flags.GetBool("yes"); err == nil && value {
			return true
		}
	}
	return false
}

func devAppPermissionScopes(cmd *cobra.Command) []string {
	values, _ := cmd.Flags().GetStringSlice("permissions")
	values = append(values, devAppFlagOrFallback(cmd, "scope", "permission"))
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range splitDevAppList(value) {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func devAppStringFlag(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return strings.TrimSpace(value)
}

func devAppIntFlag(cmd *cobra.Command, name string) int {
	value, _ := cmd.Flags().GetInt(name)
	return value
}

func devAppFlagOrFallback(cmd *cobra.Command, primary, fallback string) string {
	if value := devAppStringFlag(cmd, primary); value != "" {
		return value
	}
	return devAppStringFlag(cmd, fallback)
}

func devAppPutString(params map[string]any, key, value string) {
	if value != "" {
		params[key] = value
	}
}

func devAppPutInt(params map[string]any, key string, value int) {
	if value != 0 {
		params[key] = value
	}
}
