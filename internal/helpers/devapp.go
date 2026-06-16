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
	"encoding/json"
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

	devAppListTool       = "list_open_dev_app"
	devAppGetTool        = "get_dev_app"
	devAppCreateTool     = "create_dev_app"
	devAppUpdateTool     = "update_dev_app"
	devAppDeleteTool     = "delete_dev_app"
	devAppDisableTool    = "disable_dev_app"
	devAppEnableTool     = "enable_dev_app"
	devAppCredentialsGet = "get_open_dev_app_credentials"

	devAppMemberListTool     = "list_dev_app_members"
	devAppMemberAddTool      = "add_dev_app_members"
	devAppMemberRemoveTool   = "remove_dev_app_members"
	devAppSecurityConfigTool = "update_dev_app_security_config"

	// 机器人能力（op-app MCP 工具，硬编码不走服务发现）。
	devAppRobotCreateTool    = "create_dingtalk_robot"
	devAppRobotSubmitTool    = "submit_robot_create_task"
	devAppRobotResultTool    = "query_robot_create_result"
	devAppRobotConfigGetTool = "get_open_dev_app_robot_config"
	devAppRobotConfigSetTool = "set_open_dev_app_robot_config"
	devAppRobotEnableTool    = "enable_open_dev_app_robot"
	devAppRobotOfflineTool   = "offline_open_dev_app_robot"

	// 版本发布能力（op-app MCP 工具，硬编码不走服务发现）。
	devAppVersionCreateTool  = "create_open_dev_app_version"
	devAppVersionListTool    = "list_open_dev_app_versions"
	devAppVersionDetailTool  = "get_open_dev_app_version_detail"
	devAppVersionPublishTool = "publish_open_dev_app_version"
	devAppVersionStatusTool  = "get_open_dev_app_version_status"

	// 事件订阅能力（op-app MCP 工具，硬编码不走服务发现）。
	devAppEventListTool        = "list_open_dev_app_events"
	devAppEventSubscribeTool   = "subscribe_open_dev_app_event"
	devAppEventUnsubscribeTool = "unsubscribe_open_dev_app_event"
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
		Long:              "管理开放平台开发者应用：查询、详情、创建、更新、启停、删除、权限、网页应用、成员、安全配置、机器人、版本发布和事件订阅。",
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

	credentials := &cobra.Command{
		Use:               "credentials",
		Short:             "开放平台应用凭证",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	credentials.AddCommand(newDevAppCredentialsGetCommand(runner))

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

	robot := &cobra.Command{
		Use:               "robot",
		Short:             "开放平台应用机器人能力",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	robot.AddCommand(
		newDevAppRobotCreateCommand(runner),
		newDevAppRobotSubmitCommand(runner),
		newDevAppRobotResultCommand(runner),
		newDevAppRobotConfigGetCommand(runner),
		newDevAppRobotConfigCommand(runner, "config", "创建或更新现有应用机器人配置", devAppRobotConfigSetTool),
		newDevAppRobotConfigCommand(runner, "enable", "启用现有应用机器人能力", devAppRobotEnableTool),
		newDevAppRobotOfflineCommand(runner),
		newDevAppRobotConnectCommand(runner),
	)

	version := &cobra.Command{
		Use:               "version",
		Short:             "开放平台应用版本发布",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	version.AddCommand(
		newDevAppVersionCreateCommand(runner),
		newDevAppVersionListCommand(runner),
		newDevAppVersionGetCommand(runner),
		newDevAppVersionCheckApprovalCommand(runner),
		newDevAppVersionPublishCommand(runner),
		newDevAppVersionStatusCommand(runner),
	)

	event := &cobra.Command{
		Use:               "event",
		Short:             "开放平台应用事件订阅",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	event.AddCommand(
		newDevAppEventListCommand(runner),
		newDevAppEventSubscribeCommand(runner, "subscribe", "订阅单个事件", devAppEventSubscribeTool),
		newDevAppEventSubscribeCommand(runner, "unsubscribe", "退订单个事件", devAppEventUnsubscribeTool),
	)

	root.AddCommand(
		newDevAppListCommand(runner),
		newDevAppGetCommand(runner),
		newDevAppCreateCommand(runner),
		newDevAppUpdateCommand(runner),
		newDevAppLifecycleCommand(runner, "delete", "删除开放平台企业内部应用", devAppDeleteTool),
		newDevAppLifecycleCommand(runner, "inactive", "停用开放平台企业内部应用", devAppDisableTool),
		newDevAppLifecycleCommand(runner, "active", "启用开放平台企业内部应用", devAppEnableTool),
		credentials,
		webapp,
		permission,
		member,
		security,
		robot,
		version,
		event,
	)
	return root
}

func newDevAppListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询开放平台企业内部应用列表",
		Example:           "  dws devapp list --name DemoApp --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{
				"pageSize": devAppIntFlag(cmd, "page-size"),
			}
			devAppPutString(params, "cursor", devAppStringFlag(cmd, "cursor"))
			devAppPutString(params, "name", devAppFlagOrFallback(cmd, "name", "keyword"))
			devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
			return runDevAppTool(runner, cmd, devAppListTool, params)
		},
	}
	cmd.Flags().Int("page-size", 20, "分页大小")
	cmd.Flags().String("cursor", "", "游标分页 cursor，首次查询为空")
	cmd.Flags().String("name", "", "应用名称关键词")
	cmd.Flags().String("keyword", "", "--name 的兼容别名")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("app-key", "", "appKey/clientId")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询开放平台企业内部应用详情",
		Example:           "  dws devapp get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppMcpLocatorParams(cmd, true)
			if len(params) == 0 {
				return devAppMcpLocatorRequired(true)
			}
			return runDevAppTool(runner, cmd, devAppGetTool, params)
		},
	}
	addDevAppMcpLocatorFlags(cmd, true)
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
			return runDevAppTool(runner, cmd, devAppCreateTool, params)
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
		Example:           "  dws devapp update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "update"); err != nil {
				return err
			}
			params := devAppMcpLocatorParams(cmd, false)
			if len(params) == 0 {
				return devAppMcpLocatorRequired(false)
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
			return runDevAppTool(runner, cmd, devAppUpdateTool, params)
		},
	}
	addDevAppMcpLocatorFlags(cmd, false)
	cmd.Flags().String("name", "", "新的应用名称")
	cmd.Flags().String("desc", "", "新的应用描述")
	cmd.Flags().String("icon", "", "新的应用图标 mediaId")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppCredentialsGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "读取开放平台应用凭证",
		Example:           "  dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppCredentialsLocatorParams(cmd)
			if len(params) == 0 {
				return devAppCredentialsLocatorRequired()
			}
			return runDevAppTool(runner, cmd, devAppCredentialsGet, params)
		},
	}
	addDevAppCredentialsLocatorFlags(cmd)
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
			params := devAppMcpLocatorParams(cmd, true)
			if len(params) == 0 {
				return devAppMcpLocatorRequired(true)
			}
			return runDevAppTool(runner, cmd, tool, params)
		},
	}
	addDevAppMcpLocatorFlags(cmd, true)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppWebappGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询网页应用配置",
		Example:           "  dws devapp webapp get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppMcpLocatorParams(cmd, false)
			if len(params) == 0 {
				return devAppMcpLocatorRequired(false)
			}
			return runDevAppTool(runner, cmd, "get_webapp_config", params)
		},
	}
	addDevAppMcpLocatorFlags(cmd, false)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppWebappConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "配置网页应用能力",
		Example:           "  dws devapp webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "webapp config"); err != nil {
				return err
			}
			params := devAppMcpLocatorParams(cmd, true)
			if len(params) == 0 {
				return devAppMcpLocatorRequired(true)
			}
			updates := 0
			if v := devAppStringFlag(cmd, "h5-page-type"); v != "" {
				params["h5PageType"] = v
				updates++
			}
			if v := devAppFlagOrFallback(cmd, "homepage-url", "homepage-link"); v != "" {
				params["homepageUrl"] = v
				updates++
			}
			if v := devAppFlagOrFallback(cmd, "pc-homepage-url", "pc-homepage-link"); v != "" {
				params["pcHomepageUrl"] = v
				updates++
			}
			if v := devAppFlagOrFallback(cmd, "omp-url", "omp-link"); v != "" {
				params["ompUrl"] = v
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("at least one webapp field is required: --h5-page-type, --homepage-url, --pc-homepage-url, or --omp-url")
			}
			return runDevAppTool(runner, cmd, "set_webapp_config", params)
		},
	}
	addDevAppMcpLocatorFlags(cmd, true)
	cmd.Flags().String("h5-page-type", "", "网页应用生效端/页面类型")
	cmd.Flags().String("homepage-url", "", "移动端首页地址")
	cmd.Flags().String("homepage-link", "", "--homepage-url 的兼容别名")
	_ = cmd.Flags().MarkHidden("homepage-link")
	cmd.Flags().String("pc-homepage-url", "", "PC 端首页地址")
	cmd.Flags().String("pc-homepage-link", "", "--pc-homepage-url 的兼容别名")
	_ = cmd.Flags().MarkHidden("pc-homepage-link")
	cmd.Flags().String("omp-url", "", "管理后台地址")
	cmd.Flags().String("omp-link", "", "--omp-url 的兼容别名")
	_ = cmd.Flags().MarkHidden("omp-link")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppPermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"search", "detail"},
		Short:             "查询开放平台应用权限列表",
		Example:           "  dws devapp permission list --unified-app-id UNIFIED_APP_ID --keyword 通讯录 --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := devAppPermissionLocatorParams(cmd)
			if len(params) == 0 {
				return devAppPermissionLocatorRequired()
			}
			devAppPutString(params, "keyword", devAppStringFlag(cmd, "keyword"))
			devAppPutString(params, "scopeValue", devAppFlagOrFallback(cmd, "scope", "permission"))
			devAppPutString(params, "authStatus", strings.ToUpper(devAppStringFlag(cmd, "status")))
			devAppPutString(params, "scopeType", strings.ToUpper(devAppStringFlag(cmd, "scope-type")))
			devAppPutString(params, "apiStatus", devAppStringFlag(cmd, "api-status"))
			devAppPutString(params, "cursor", devAppStringFlag(cmd, "cursor"))
			devAppPutInt(params, "pageSize", devAppIntFlag(cmd, "page-size"))
			return runDevAppTool(runner, cmd, "list_open_dev_app_permissions", params)
		},
	}
	addDevAppPermissionLocatorFlags(cmd)
	cmd.Flags().String("keyword", "", "权限名、权限点、接口名关键词")
	cmd.Flags().String("scope", "", "精确权限点 scopeValue")
	cmd.Flags().String("permission", "", "--scope 的兼容别名")
	_ = cmd.Flags().MarkHidden("permission")
	cmd.Flags().String("status", "ALL", "权限状态：ALL、AUTHED、UNAUTHED")
	cmd.Flags().String("scope-type", "", "权限一级类型：APP 或 SNS")
	cmd.Flags().String("api-status", "", "开发者后台 apiStatus 过滤")
	cmd.Flags().Int("page-size", 20, "返回数量上限")
	cmd.Flags().String("cursor", "", "游标分页 cursor，首次查询为空")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppPermissionAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "申请开放平台应用权限点",
		Example:           "  dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions Contact.User.mobile --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission add"); err != nil {
				return err
			}
			params := devAppPermissionLocatorParams(cmd)
			if len(params) == 0 {
				return devAppPermissionLocatorRequired()
			}
			scopes := devAppPermissionScopes(cmd)
			if len(scopes) == 0 {
				return apperrors.NewValidation("--permissions is required")
			}
			params["scopeValues"] = scopes
			return runDevAppTool(runner, cmd, "apply_open_dev_app_permissions", params)
		},
	}
	addDevAppPermissionLocatorFlags(cmd)
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
		Example:           "  dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permission Contact.User.mobile --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission remove"); err != nil {
				return err
			}
			params := devAppPermissionLocatorParams(cmd)
			if len(params) == 0 {
				return devAppPermissionLocatorRequired()
			}
			scope := devAppFlagOrFallback(cmd, "permission", "scope")
			if scope == "" {
				return apperrors.NewValidation("--permission is required")
			}
			params["scopeValue"] = scope
			return runDevAppTool(runner, cmd, "remove_open_dev_app_permission", params)
		},
	}
	addDevAppPermissionLocatorFlags(cmd)
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
			"--ip-whitelist 192.0.2.10 --redirect-url https://callback.example.invalid/callback --sso-url https://sso.example.invalid/sso --dry-run",
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
				params["ipWhitelist"] = values
			}
			if values := parseDevAppListFlag(cmd, "redirect-url"); len(values) > 0 {
				params["redirectUrls"] = values
			}
			if values := parseDevAppListFlag(cmd, "sso-url"); len(values) > 0 {
				params["ssoUrls"] = values
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

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

func newDevAppRobotCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "新建钉钉智能体机器人（一次性同步创建应用+机器人）",
		Example:           "  dws devapp robot create --app-name 我的智能体 --robot-name 小助手 --desc \"处理审批问答\" --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot create"); err != nil {
				return err
			}
			params, err := devAppRobotCreateParams(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppRobotCreateTool, params)
		},
	}
	registerDevAppRobotCreateFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppRobotSubmitCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "submit",
		Short:             "异步提交钉钉智能体机器人创建任务（支持失败重试）",
		Example:           "  dws devapp robot submit --app-name 我的智能体 --robot-name 小助手 --desc \"处理审批问答\" --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot submit"); err != nil {
				return err
			}
			params, err := devAppRobotCreateParams(cmd)
			if err != nil {
				return err
			}
			// submit_robot_create_task 的 schema 把图标字段标为必填（空值时服务端用默认图标），
			// 因此即使用户未提供也补空串占位。
			if _, ok := params["robotMediaId"]; !ok {
				params["robotMediaId"] = ""
			}
			if _, ok := params["previewMediaId"]; !ok {
				params["previewMediaId"] = ""
			}
			devAppPutString(params, "taskId", devAppStringFlag(cmd, "task-id"))
			return runDevAppTool(runner, cmd, devAppRobotSubmitTool, params)
		},
	}
	registerDevAppRobotCreateFlags(cmd)
	cmd.Flags().String("task-id", "", "失败重试时传入原 taskId；为空时服务端自动生成")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppRobotResultCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "result",
		Short:             "查询机器人异步创建任务结果",
		Example:           "  dws devapp robot result --task-id TASK_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := devAppStringFlag(cmd, "task-id")
			if taskID == "" {
				return apperrors.NewValidation("--task-id is required")
			}
			return runDevAppTool(runner, cmd, devAppRobotResultTool, map[string]any{"taskId": taskID})
		},
	}
	cmd.Flags().String("task-id", "", "提交创建任务时返回的 taskId (必填)")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppRobotConfigGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询现有应用的机器人配置",
		Example:           "  dws devapp robot get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppRobotConfigGetTool, map[string]any{"unifiedAppId": appID})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppRobotConfigCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Example:           "  dws devapp robot " + use + " --unified-app-id UNIFIED_APP_ID --name 小助手 --brief 审批助手 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot "+use); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params, updates, err := devAppRobotConfigParams(cmd, appID)
			if err != nil {
				return err
			}
			if updates == 0 {
				return apperrors.NewValidation("at least one robot config field is required, e.g. --name, --brief, --description, --icon, --outgoing-url, --event-url, --mode, --skills")
			}
			return runDevAppTool(runner, cmd, tool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	registerDevAppRobotConfigFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppRobotOfflineCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "offline",
		Short:             "停用现有应用的机器人能力",
		Example:           "  dws devapp robot offline --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot offline"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppRobotOfflineTool, map[string]any{"unifiedAppId": appID})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func registerDevAppRobotCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("app-name", "", "智能体应用名称，长度 2-20，企业内唯一 (必填)")
	cmd.Flags().String("robot-name", "", "承载机器人名称，用于客户端展示 (必填)")
	cmd.Flags().String("desc", "", "机器人功能描述，不超过 200 字 (必填)")
	cmd.Flags().String("icon", "", "机器人图标 mediaId；为空时使用默认图标")
	cmd.Flags().String("preview", "", "机器人预览图 mediaId；为空时复用图标")
}

func devAppRobotCreateParams(cmd *cobra.Command) (map[string]any, error) {
	appName := devAppStringFlag(cmd, "app-name")
	if appName == "" {
		return nil, apperrors.NewValidation("--app-name is required")
	}
	robotName := devAppStringFlag(cmd, "robot-name")
	if robotName == "" {
		return nil, apperrors.NewValidation("--robot-name is required")
	}
	desc := devAppStringFlag(cmd, "desc")
	if desc == "" {
		return nil, apperrors.NewValidation("--desc is required")
	}
	params := map[string]any{
		"appName":   appName,
		"robotName": robotName,
		"desc":      desc,
	}
	devAppPutString(params, "robotMediaId", devAppStringFlag(cmd, "icon"))
	devAppPutString(params, "previewMediaId", devAppStringFlag(cmd, "preview"))
	return params, nil
}

func registerDevAppRobotConfigFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "机器人名称")
	cmd.Flags().String("brief", "", "机器人简介")
	cmd.Flags().String("description", "", "机器人描述")
	cmd.Flags().String("icon", "", "机器人图标 mediaId")
	cmd.Flags().String("outgoing-url", "", "消息回调地址 (outgoingUrl)")
	cmd.Flags().String("event-url", "", "事件回调地址 (chatBotEventUrl)")
	cmd.Flags().Int("mode", 0, "机器人模式枚举")
	cmd.Flags().StringSlice("skills", nil, "技能列表，多个用逗号分隔")
	cmd.Flags().Bool("add-scope", false, "是否自动添加机器人相关权限")
	cmd.Flags().Bool("disable-ssl-verify", false, "回调地址是否关闭 SSL 校验")
	cmd.Flags().String("i18n-name", "", "机器人名称国际化 JSON，如 '{\"en_US\":\"Bot\"}'")
	cmd.Flags().String("i18n-brief", "", "机器人简介国际化 JSON")
	cmd.Flags().String("i18n-description", "", "机器人描述国际化 JSON")
}

func devAppRobotConfigParams(cmd *cobra.Command, appID string) (map[string]any, int, error) {
	params := map[string]any{"unifiedAppId": appID}
	updates := 0
	setString := func(key, flag string) {
		if v := devAppStringFlag(cmd, flag); v != "" {
			params[key] = v
			updates++
		}
	}
	setString("name", "name")
	setString("brief", "brief")
	setString("description", "description")
	setString("iconMediaId", "icon")
	setString("outgoingUrl", "outgoing-url")
	setString("chatBotEventUrl", "event-url")
	if cmd.Flags().Changed("mode") {
		params["mode"] = devAppIntFlag(cmd, "mode")
		updates++
	}
	if cmd.Flags().Changed("add-scope") {
		value, _ := cmd.Flags().GetBool("add-scope")
		params["isAddScope"] = value
		updates++
	}
	if cmd.Flags().Changed("disable-ssl-verify") {
		value, _ := cmd.Flags().GetBool("disable-ssl-verify")
		params["disableSSLVerify"] = value
		updates++
	}
	if values, _ := cmd.Flags().GetStringSlice("skills"); len(values) > 0 {
		params["skillList"] = values
		updates++
	}
	for _, item := range []struct{ key, flag string }{
		{"i18nName", "i18n-name"},
		{"i18nBrief", "i18n-brief"},
		{"i18nDescription", "i18n-description"},
	} {
		raw := devAppStringFlag(cmd, item.flag)
		if raw == "" {
			continue
		}
		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, 0, apperrors.NewValidation(fmt.Sprintf("--%s must be valid JSON object: %v", item.flag, err))
		}
		params[item.key] = parsed
		updates++
	}
	return params, updates, nil
}

// ---------------------------------------------------------------------------
// 版本发布能力
// ---------------------------------------------------------------------------

func newDevAppVersionCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "基于当前配置创建应用新版本",
		Example:           "  dws devapp version create --unified-app-id UNIFIED_APP_ID --version 1.0.1 --desc \"新增机器人能力\" --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "version create"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			devAppPutString(params, "version", devAppStringFlag(cmd, "version"))
			devAppPutString(params, "description", devAppStringFlag(cmd, "desc"))
			return runDevAppTool(runner, cmd, devAppVersionCreateTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("version", "", "版本号，如 1.0.1")
	cmd.Flags().String("desc", "", "版本描述")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppVersionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "按 cursor 查询应用版本列表",
		Example:           "  dws devapp version list --unified-app-id UNIFIED_APP_ID --page-size 20 --format json\n  dws devapp version list --unified-app-id UNIFIED_APP_ID --cursor NEXT_CURSOR --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{
				"unifiedAppId": appID,
				"pageSize":     devAppIntFlag(cmd, "page-size"),
			}
			devAppPutString(params, "cursor", devAppStringFlag(cmd, "cursor"))
			return runDevAppTool(runner, cmd, devAppVersionListTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("cursor", "", "服务端返回的不透明分页游标；首次查询留空")
	cmd.Flags().Int("page-size", 20, "分页大小")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppVersionGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询指定版本详情",
		Example:           "  dws devapp version get --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devAppVersionLocator(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppVersionDetailTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppVersionCheckApprovalCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "check-approval",
		Short:             "预检版本发布是否需要审批（不实际发布）",
		Example:           "  dws devapp version check-approval --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devAppVersionLocator(cmd)
			if err != nil {
				return err
			}
			// 复用 publish 工具的服务端预检模式：precheckOnly=true 只返回审批要求，不发布。
			params["precheckOnly"] = true
			return runDevAppTool(runner, cmd, devAppVersionPublishTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppVersionPublishCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "publish",
		Short:             "发布指定版本（含高敏权限需 --confirm-sensitive）",
		Example:           "  dws devapp version publish --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "version publish"); err != nil {
				return err
			}
			params, err := devAppVersionLocator(cmd)
			if err != nil {
				return err
			}
			params["precheckOnly"] = false
			if cmd.Flags().Changed("confirm-sensitive") {
				value, _ := cmd.Flags().GetBool("confirm-sensitive")
				params["confirmedSensitive"] = value
			}
			devAppPutString(params, "approverUserId", devAppStringFlag(cmd, "approver"))
			return runDevAppTool(runner, cmd, devAppVersionPublishTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	cmd.Flags().Bool("confirm-sensitive", false, "确认发布包含高敏权限的版本")
	cmd.Flags().String("approver", "", "灰度选人模式下指定审批人 userId")
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppVersionStatusCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "status",
		Short:             "查询版本发布/审批状态",
		Example:           "  dws devapp version status --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devAppVersionLocator(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppVersionStatusTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func addDevAppVersionLocatorFlags(cmd *cobra.Command) {
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("version-id", "", "版本 ID (必填)")
}

func devAppVersionLocator(cmd *cobra.Command) (map[string]any, error) {
	appID, err := requiredDevAppUnifiedID(cmd)
	if err != nil {
		return nil, err
	}
	versionID := devAppStringFlag(cmd, "version-id")
	if versionID == "" {
		return nil, apperrors.NewValidation("--version-id is required")
	}
	return map[string]any{"unifiedAppId": appID, "versionId": versionID}, nil
}

func addDevAppUnifiedIDFlag(cmd *cobra.Command) {
	cmd.Flags().String("unified-app-id", "", "统一应用 ID (必填)")
}

func requiredDevAppUnifiedID(cmd *cobra.Command) (string, error) {
	appID := devAppStringFlag(cmd, "unified-app-id")
	if appID == "" {
		return "", apperrors.NewValidation("--unified-app-id is required")
	}
	return appID, nil
}

// ---------------------------------------------------------------------------
// 事件订阅能力
// ---------------------------------------------------------------------------

func newDevAppEventListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询应用可订阅事件列表及订阅状态",
		Example:           "  dws devapp event list --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppEventListTool, map[string]any{"unifiedAppId": appID})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	return cmd
}

func newDevAppEventSubscribeCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Example:           "  dws devapp event " + use + " --unified-app-id UNIFIED_APP_ID --event-code user_add_org --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "event "+use); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			eventCode := devAppStringFlag(cmd, "event-code")
			if eventCode == "" {
				return apperrors.NewValidation("--event-code is required")
			}
			return runDevAppTool(runner, cmd, tool, map[string]any{"unifiedAppId": appID, "eventCode": eventCode})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("event-code", "", "事件码 eventCode，见 event list 返回 (必填)")
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
	result = normalizeDevAppServiceResult(result)
	return writeCommandPayload(cmd, result)
}

func normalizeDevAppServiceResult(result executor.Result) executor.Result {
	content, ok := result.Response["content"].(map[string]any)
	if !ok {
		return result
	}
	if success, ok := content["success"].(bool); !ok || !success {
		return result
	}
	value, ok := content["result"]
	if !ok || value == nil {
		return result
	}
	result.Response["content"] = value
	return result
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

func addDevAppMcpLocatorFlags(cmd *cobra.Command, includeName bool) {
	cmd.Flags().String("unified-app-id", "", "统一应用 ID")
	cmd.Flags().String("app-key", "", "appKey/clientId")
	if includeName {
		cmd.Flags().String("name", "", "应用名称关键词；写操作前必须唯一命中")
	}
}

func devAppMcpLocatorParams(cmd *cobra.Command, includeName bool) map[string]any {
	params := map[string]any{}
	devAppPutString(params, "unifiedAppId", devAppStringFlag(cmd, "unified-app-id"))
	devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
	if includeName {
		devAppPutString(params, "name", devAppStringFlag(cmd, "name"))
	}
	return params
}

func addDevAppCredentialsLocatorFlags(cmd *cobra.Command) {
	cmd.Flags().String("unified-app-id", "", "统一应用 ID")
	cmd.Flags().String("app-key", "", "appKey/clientId")
	cmd.Flags().String("name", "", "应用名称关键词；必须唯一命中")
}

func devAppCredentialsLocatorParams(cmd *cobra.Command) map[string]any {
	params := map[string]any{}
	devAppPutString(params, "unifiedAppId", devAppStringFlag(cmd, "unified-app-id"))
	devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
	devAppPutString(params, "appName", devAppStringFlag(cmd, "name"))
	return params
}

func devAppCredentialsLocatorRequired() error {
	return apperrors.NewValidation("one app locator is required: --unified-app-id, --app-key, or --name")
}

func devAppMcpLocatorRequired(includeName bool) error {
	if includeName {
		return apperrors.NewValidation("one app locator is required: --unified-app-id, --app-key, or --name")
	}
	return apperrors.NewValidation("one app locator is required: --unified-app-id or --app-key")
}

func addDevAppPermissionLocatorFlags(cmd *cobra.Command) {
	cmd.Flags().String("unified-app-id", "", "统一应用 ID")
}

func devAppPermissionLocatorParams(cmd *cobra.Command) map[string]any {
	params := map[string]any{}
	devAppPutString(params, "unifiedAppId", devAppStringFlag(cmd, "unified-app-id"))
	return params
}

func devAppPermissionLocatorRequired() error {
	return apperrors.NewValidation("one app locator is required: --unified-app-id")
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
