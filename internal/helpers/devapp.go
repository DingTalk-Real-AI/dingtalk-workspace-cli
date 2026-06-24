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

	// 工具名 = 服务端 op-app 网关**实际注册**的名字（已用 tools 真实联调逐个核对）。
	// 注意：服务端这批命名本身并不统一——前缀 dev_app 与 extension 混用、list 用复数、
	// permission 用 apply 而 member 用 add、robot 建号流程保留旧名（submit_robot_create_task/
	// query_robot_create_result）。CLI 这里**对齐服务端现状以跑通联调**，不在 CLI 做映射；
	// 命名统一是服务端待办，详见 obsidian《dev 命令树 - 服务端 MCP 待改造清单》。
	// 集中声明、调用点不写字面量，避免拼写漂移。
	//
	// 应用主体 + 子资源（凭证/网页/权限）。
	devAppListTool           = "list_dev_app"
	devAppGetTool            = "get_dev_app"
	devAppCreateTool         = "create_dev_app"
	devAppUpdateTool         = "update_dev_app"
	devAppDeleteTool         = "delete_dev_app"
	devAppEnableTool         = "enable_dev_app"
	devAppDisableTool        = "disable_dev_app"
	devAppCredentialsGetTool = "get_dev_app_credentials"
	devAppWebappGetTool      = "get_extension_webapp_config"
	devAppWebappSetTool      = "set_extension_webapp_config"
	devAppPermissionListTool = "list_dev_app_permissions"
	devAppPermissionAddTool  = "apply_dev_app_permissions"
	devAppPermissionRmTool   = "remove_dev_app_permissions"

	devAppMemberListTool     = "list_dev_app_members"
	devAppMemberAddTool      = "add_dev_app_members"
	devAppMemberRemoveTool   = "remove_dev_app_members"
	devAppSecurityConfigTool = "update_dev_app_security_config"

	// 机器人能力（op-app MCP 工具，硬编码不走服务发现）。
	devAppRobotSubmitTool    = "submit_robot_create_task"
	devAppRobotResultTool    = "query_robot_create_result"
	devAppRobotConfigGetTool = "get_extension_robot_config"
	// 上游待合并：create/update 两个 tool 合成一个 upsert（建/改判断在服务端）。
	// 见 docs/upstream-todo.md。上游上线前 CLI 调此名待联调。
	devAppRobotConfigUpsertTool = "set_extension_robot_config"
	devAppRobotEnableTool       = "enable_dev_app_robot"
	devAppRobotOfflineTool      = "disable_dev_app_robot"

	// 事件订阅能力（op-app MCP 工具，服务端新增）。
	devAppEventListTool        = "list_dev_app_events"
	devAppEventSubscribeTool   = "subscribe_dev_app_events"
	devAppEventUnsubscribeTool = "unsubscribe_dev_app_events"

	// 版本发布能力（op-app MCP 工具，硬编码不走服务发现）。
	devAppVersionCreateTool  = "create_dev_app_version"
	devAppVersionListTool    = "list_dev_app_versions"
	devAppVersionDetailTool  = "get_dev_app_version_detail"
	devAppVersionPublishTool = "publish_dev_app_version"
	devAppVersionStatusTool  = "get_dev_app_version_status"
)

// newDevAppCommand builds the `app` subtree of `dws dev`. The cobra path is
// `dws dev app ...` while the MCP product id stays "devapp" — the id is a
// backend contract (SupplementServers/StaticServers injection key and the
// pinned op-app endpoint), decoupled from the user-facing command name.
func newDevAppCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "app",
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
		newDevAppRobotSubmitCommand(runner),
		newDevAppRobotResultCommand(runner),
		newDevAppRobotConfigGetCommand(runner),
		newDevAppRobotConfigCommand(runner),
		newDevAppRobotEnableCommand(runner),
		newDevAppRobotOfflineCommand(runner),
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
		newDevAppEventSubscribeCommand(runner),
		newDevAppEventUnsubscribeCommand(runner),
	)

	root.AddCommand(
		newDevAppListCommand(runner),
		newDevAppGetCommand(runner),
		newDevAppCreateCommand(runner),
		newDevAppUpdateCommand(runner),
		newDevAppDeleteCommand(runner),
		newDevAppLifecycleCommand(runner, "disable", "停用开放平台企业内部应用", devAppDisableTool),
		newDevAppLifecycleCommand(runner, "enable", "启用开放平台企业内部应用", devAppEnableTool),
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

// ---------------------------------------------------------------------------
// 事件订阅能力（服务端新增 list/subscribe/unsubscribe）
// ---------------------------------------------------------------------------

func newDevAppEventListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询应用已订阅的事件列表",
		Example:           "  dws dev app event list --unified-app-id UNIFIED_APP_ID --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			devAppPutString(params, "keyword", devAppStringFlag(cmd, "keyword"))
			devAppApplyCursorParams(cmd, params)
			return runDevAppTool(runner, cmd, devAppEventListTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("keyword", "", "事件搜索关键词，支持按事件码或事件名称模糊匹配")
	registerDevAppCursorFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppEventListTool)
	return cmd
}

func newDevAppEventSubscribeCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "subscribe",
		Short:             "订阅应用事件回调",
		Example:           "  dws dev app event subscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "event subscribe"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			// 服务端 subscribe/unsubscribe 工具收 eventCodes 数组（事件码列表）。
			eventCodes, err := requiredDevAppEventCodes(cmd)
			if err != nil {
				return err
			}
			params["eventCodes"] = eventCodes
			return runDevAppTool(runner, cmd, devAppEventSubscribeTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("event-codes", "", "事件码，多个用逗号或分号分隔")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppEventSubscribeTool)
	return cmd
}

func newDevAppEventUnsubscribeCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "unsubscribe",
		Short:             "取消订阅应用事件",
		Example:           "  dws dev app event unsubscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "event unsubscribe"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			eventCodes, err := requiredDevAppEventCodes(cmd)
			if err != nil {
				return err
			}
			params["eventCodes"] = eventCodes
			return runDevAppTool(runner, cmd, devAppEventUnsubscribeTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("event-codes", "", "事件码，多个用逗号或分号分隔")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppEventUnsubscribeTool)
	return cmd
}

func newDevAppListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询开放平台企业内部应用列表",
		Example:           "  dws dev app list --name DemoApp --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			devAppApplyCursorParams(cmd, params)
			devAppPutString(params, "name", devAppFlagOrFallback(cmd, "name", "keyword"))
			devAppPutString(params, "appKey", devAppStringFlag(cmd, "app-key"))
			devAppPutInt(params, "appGroupId", devAppIntFlag(cmd, "app-group-id"))
			devAppPutString(params, "creator", devAppStringFlag(cmd, "creator"))
			devAppPutString(params, "robotName", devAppStringFlag(cmd, "robot-name"))
			devAppPutInt(params, "developType", devAppIntFlag(cmd, "develop-type"))
			devAppPutInt(params, "filterCoolApp", devAppIntFlag(cmd, "filter-cool-app"))
			devAppPutString(params, "sortType", devAppStringFlag(cmd, "sort-type"))
			devAppPutString(params, "sortOrder", devAppStringFlag(cmd, "sort-order"))
			return runDevAppTool(runner, cmd, devAppListTool, params)
		},
	}
	registerDevAppCursorFlags(cmd)
	cmd.Flags().String("name", "", "应用名称关键词")
	cmd.Flags().String("keyword", "", "--name 的兼容别名")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("app-key", "", "按 appKey/clientId 过滤")
	cmd.Flags().Int("app-group-id", 0, "应用分组 ID")
	cmd.Flags().String("creator", "", "创建人名称关键词")
	cmd.Flags().String("robot-name", "", "机器人名称关键词")
	cmd.Flags().Int("develop-type", 0, "开发类型枚举；不确定时不要传")
	cmd.Flags().Int("filter-cool-app", 0, "酷应用过滤枚举；不确定时不要传")
	cmd.Flags().String("sort-type", "", "排序字段，如 gmt_modified")
	cmd.Flags().String("sort-order", "", "排序方向 asc 或 desc")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppListTool)
	return cmd
}

func newDevAppGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询开放平台企业内部应用详情",
		Example:           "  dws dev app get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			return runDevAppTool(runner, cmd, devAppGetTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppGetTool)
	return cmd
}

func newDevAppCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建开放平台企业内部应用",
		Example:           "  dws dev app create --name DemoApp --desc 内部应用 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "create"); err != nil {
				return err
			}
			name := devAppStringFlag(cmd, "name")
			if name == "" {
				return apperrors.NewValidation("--name 为必填")
			}
			params := map[string]any{"name": name}
			devAppPutString(params, "desc", devAppStringFlag(cmd, "desc"))
			devAppPutString(params, "iconMediaId", devAppStringFlag(cmd, "icon-media-id"))
			return runDevAppTool(runner, cmd, devAppCreateTool, params)
		},
	}
	cmd.Flags().String("name", "", "应用名称 (必填)")
	cmd.Flags().String("desc", "", "应用描述")
	cmd.Flags().String("icon-media-id", "", "应用图标 mediaId")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppCreateTool)
	return cmd
}

func newDevAppUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             "修改开放平台企业内部应用基础信息",
		Example:           "  dws dev app update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "update"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			updates := 0
			if v := devAppStringFlag(cmd, "name"); v != "" {
				params["name"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "desc"); v != "" {
				params["desc"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "icon-media-id"); v != "" {
				params["iconMediaId"] = v
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("至少提供一项待更新字段：--name、--desc 或 --icon-media-id")
			}
			return runDevAppTool(runner, cmd, devAppUpdateTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("name", "", "新的应用名称")
	cmd.Flags().String("desc", "", "新的应用描述")
	cmd.Flags().String("icon-media-id", "", "新的应用图标 mediaId")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppUpdateTool)
	return cmd
}

func newDevAppCredentialsGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "读取开放平台应用凭证",
		Example:           "  dws dev app credentials get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			return runDevAppTool(runner, cmd, devAppCredentialsGetTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppCredentialsGetTool)
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
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			return runDevAppTool(runner, cmd, tool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, tool)
	return cmd
}

// newDevAppDeleteCommand is delete with a danger tier: deleting an app is
// irreversible, so beyond the write guard it requires --confirm-name to match
// the located app's real name. This guards against "located the wrong app and
// deleted it" — the agent must first know the name (via `get`/dry-run) before
// it can delete. The match is verified client-side (a `get` then compare),
// standard practice for destructive CLI ops (gh repo delete, gcloud).
func newDevAppDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             "删除开放平台企业内部应用（不可逆，需 --confirm-name 二次确认）",
		Example:           "  dws dev app delete --unified-app-id UNIFIED_APP_ID --confirm-name 应用名 --yes --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "delete"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			// Dry-run previews the delete without requiring confirmation —
			// the agent uses it (or `get`) to read the app name first.
			if commandDryRun(cmd) {
				return runDevAppTool(runner, cmd, devAppDeleteTool, params)
			}
			// Real execution: require --confirm-name and verify it matches.
			confirmName := devAppStringFlag(cmd, "confirm-name")
			if confirmName == "" {
				return apperrors.NewValidation("删除不可逆，需二次确认：先用 `dev app get` 看应用名，再加 --confirm-name=<应用名>")
			}
			actualName, err := devAppFetchAppName(runner, cmd, params)
			if err != nil {
				return err
			}
			// 读不到应用名时 fail-closed：不可逆删除不能在无法校验 --confirm-name
			// 的情况下放行，否则二次确认形同虚设。
			if actualName == "" {
				return apperrors.NewValidation("无法读取应用名以校验 --confirm-name，已中止删除；请确认 --unified-app-id 正确，或先用 --dry-run / `dev app get` 预览")
			}
			if confirmName != actualName {
				return apperrors.NewValidation(fmt.Sprintf("名称不匹配：--confirm-name=%q 但定位到的应用名是 %q，已中止删除", confirmName, actualName))
			}
			return runDevAppTool(runner, cmd, devAppDeleteTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("confirm-name", "", "二次确认：必须与被删应用的名称一致（不可逆操作的防误删）")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppDeleteTool)
	return cmd
}

// devAppFetchAppName resolves the located app's name via get_dev_app
// so delete can verify --confirm-name. Returns "" if the name can't be found;
// the caller treats "" as fail-closed (aborts the irreversible delete) rather
// than silently proceeding.
func devAppFetchAppName(runner executor.Runner, cmd *cobra.Command, locator map[string]any) (string, error) {
	inv := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		devAppProduct,
		devAppGetTool,
		locator,
	)
	result, err := runner.Run(cmd.Context(), inv)
	if err != nil {
		return "", err
	}
	// get_dev_app 返回的应用名字段是 name（credentials 才用 appName）；
	// 取 name、appName 兜底，否则 delete 永远读不到名、二次确认必然 fail-closed。
	if name := devAppExtractString(result.Response, "name"); name != "" {
		return name, nil
	}
	return devAppExtractString(result.Response, "appName"), nil
}

// devAppExtractString descends the helper response (content → result) and reads
// a string field. Returns "" if absent.
func devAppExtractString(response map[string]any, key string) string {
	node := response
	if inner, ok := node["content"].(map[string]any); ok {
		node = inner
	}
	if inner, ok := node["result"].(map[string]any); ok {
		node = inner
	}
	if v, ok := node[key].(string); ok {
		return v
	}
	return ""
}

func newDevAppWebappGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询网页应用配置",
		Example:           "  dws dev app webapp get --unified-app-id UNIFIED_APP_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			return runDevAppTool(runner, cmd, devAppWebappGetTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppWebappGetTool)
	return cmd
}

func newDevAppWebappConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "配置网页应用能力",
		Example:           "  dws dev app webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "webapp config"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			updates := 0
			if v := devAppStringFlag(cmd, "h5-page-type"); v != "" {
				params["h5PageType"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "homepage-url"); v != "" {
				params["homepageUrl"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "pc-homepage-url"); v != "" {
				params["pcHomepageUrl"] = v
				updates++
			}
			if v := devAppStringFlag(cmd, "omp-url"); v != "" {
				params["ompUrl"] = v
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("至少提供一项网页应用配置：--h5-page-type、--homepage-url、--pc-homepage-url 或 --omp-url")
			}
			return runDevAppTool(runner, cmd, devAppWebappSetTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("h5-page-type", "", "网页应用生效端/页面类型")
	cmd.Flags().String("homepage-url", "", "移动端首页地址")
	cmd.Flags().String("pc-homepage-url", "", "PC 端首页地址")
	cmd.Flags().String("omp-url", "", "管理后台地址")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppWebappSetTool)
	return cmd
}

func newDevAppPermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"search"},
		Short:             "查询开放平台应用权限列表",
		Example:           "  dws dev app permission list --unified-app-id UNIFIED_APP_ID --keyword 通讯录 --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			devAppPutString(params, "keyword", devAppStringFlag(cmd, "keyword"))
			devAppPutString(params, "scopeValue", devAppStringFlag(cmd, "scope-value"))
			devAppPutString(params, "authStatus", strings.ToUpper(devAppStringFlag(cmd, "auth-status")))
			devAppPutString(params, "scopeType", strings.ToUpper(devAppStringFlag(cmd, "scope-type")))
			devAppPutString(params, "apiStatus", devAppStringFlag(cmd, "api-status"))
			devAppApplyCursorParams(cmd, params)
			return runDevAppTool(runner, cmd, devAppPermissionListTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("keyword", "", "权限名、权限点、接口名关键词")
	cmd.Flags().String("scope-value", "", "精确权限点 scopeValue")
	cmd.Flags().String("auth-status", "ALL", "权限状态：ALL、AUTHED、UNAUTHED")
	cmd.Flags().String("scope-type", "", "权限一级类型：APP 或 SNS")
	cmd.Flags().String("api-status", "", "开发者后台 apiStatus 过滤")
	registerDevAppCursorFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppPermissionListTool)
	return cmd
}

func newDevAppPermissionAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "申请开放平台应用权限点",
		Example:           "  dws dev app permission add --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission add"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			scopes := devAppPermissionScopes(cmd)
			if len(scopes) == 0 {
				return apperrors.NewValidation("--scope-values 为必填")
			}
			params["scopeValues"] = scopes
			return runDevAppTool(runner, cmd, devAppPermissionAddTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("scope-values", "", "权限点 scopeValue，多个用逗号或分号分隔")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppPermissionAddTool)
	return cmd
}

func newDevAppPermissionRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             "取消开放平台应用权限点",
		Example:           "  dws dev app permission remove --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile,qyapi_robot_sendmsg --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission remove"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			scopes := devAppPermissionScopes(cmd)
			if len(scopes) == 0 {
				return apperrors.NewValidation("--scope-values 为必填")
			}
			params["scopeValues"] = scopes
			return runDevAppTool(runner, cmd, devAppPermissionRmTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("scope-values", "", "待取消权限点 scopeValue，多个用逗号或分号分隔")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppPermissionRmTool)
	return cmd
}

func newDevAppMemberListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询开放平台应用成员",
		Example:           "  dws dev app member list --unified-app-id <unifiedAppId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppMemberListTool, map[string]any{
				"unifiedAppId": appID,
			})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppMemberListTool)
	return cmd
}

func newDevAppMemberAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "添加开放平台应用成员",
		Example:           "  dws dev app member add --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevAppMemberMutation(runner, cmd, devAppMemberAddTool, "member add")
		},
	}
	registerDevAppMemberMutationFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppMemberAddTool)
	return cmd
}

func newDevAppMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             "移除开放平台应用成员",
		Example:           "  dws dev app member remove --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevAppMemberMutation(runner, cmd, devAppMemberRemoveTool, "member remove")
		},
	}
	registerDevAppMemberMutationFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppMemberRemoveTool)
	return cmd
}

func newDevAppSecurityConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "更新开放平台应用安全配置",
		Example: "  dws dev app security config --unified-app-id <unifiedAppId> " +
			"--ip-whitelist 192.0.2.10 --redirect-urls https://callback.example.invalid/callback --sso-urls https://sso.example.invalid/sso --dry-run",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "security config"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}

			params := map[string]any{"unifiedAppId": appID}
			updates := 0
			if values := parseDevAppListFlag(cmd, "ip-whitelist"); len(values) > 0 {
				params["ipWhitelist"] = values
				updates++
			}
			if values := parseDevAppListFlag(cmd, "redirect-urls"); len(values) > 0 {
				params["redirectUrls"] = values
				updates++
			}
			if values := parseDevAppListFlag(cmd, "sso-urls"); len(values) > 0 {
				params["ssoUrls"] = values
				updates++
			}
			if updates == 0 {
				return apperrors.NewValidation("至少提供一项安全配置：--ip-whitelist、--redirect-urls 或 --sso-urls")
			}
			return runDevAppTool(runner, cmd, devAppSecurityConfigTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("ip-whitelist", "", "出口 IP 白名单，多个用逗号或分号分隔（整组覆盖，非追加）")
	cmd.Flags().String("redirect-urls", "", "登录重定向 URL，多个用逗号或分号分隔（整组覆盖，非追加）")
	cmd.Flags().String("sso-urls", "", "端内免登地址，多个用逗号或分号分隔（整组覆盖，非追加）")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppSecurityConfigTool)
	return cmd
}

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

func newDevAppRobotSubmitCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "submit",
		Short:             "异步提交钉钉智能体机器人创建任务（支持失败重试）",
		Example:           "  dws dev app robot submit --name 我的智能体 --robot-name 小助手 --desc \"处理审批问答\" --dry-run --format json",
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
			// submit_robot_create 的 schema 把图标字段标为必填（空值时服务端用默认图标），
			// 因此即使用户未提供也补空串占位。
			if _, ok := params["iconMediaId"]; !ok {
				params["iconMediaId"] = ""
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
	annotateDevAppTool(cmd, devAppRobotSubmitTool)
	return cmd
}

func newDevAppRobotResultCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "result",
		Short:             "查询机器人异步创建任务结果",
		Example:           "  dws dev app robot result --task-id TASK_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := devAppStringFlag(cmd, "task-id")
			if taskID == "" {
				return apperrors.NewValidation("--task-id 为必填")
			}
			return runDevAppTool(runner, cmd, devAppRobotResultTool, map[string]any{"taskId": taskID})
		},
	}
	cmd.Flags().String("task-id", "", "提交创建任务时返回的 taskId (必填)")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppRobotResultTool)
	return cmd
}

func newDevAppRobotConfigGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询现有应用的机器人配置",
		Example:           "  dws dev app robot get --unified-app-id UNIFIED_APP_ID --format json",
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
	annotateDevAppTool(cmd, devAppRobotConfigGetTool)
	return cmd
}

// newDevAppRobotConfigCommand is the upsert command for an app's robot config:
// one command for both "首次创建" and "更新" — the create-vs-update decision is
// the upstream tool's job, not the CLI's (see docs/upstream-todo.md, where the
// old create/update tools merge into one `set_dev_app_robot_config`).
// `enable` (pure enable, no config fields) is a separate command.
func newDevAppRobotConfigCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "创建或更新现有应用的机器人配置（upsert）",
		Example:           "  dws dev app robot config --unified-app-id UNIFIED_APP_ID --name 小助手 --brief 审批助手 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot config"); err != nil {
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
				return apperrors.NewValidation("至少提供一项机器人配置字段，如 --name、--brief、--desc、--icon-media-id、--outgoing-url、--event-callback-url、--mode、--skills")
			}
			return runDevAppTool(runner, cmd, devAppRobotConfigUpsertTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	registerDevAppRobotConfigFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppRobotConfigUpsertTool)
	return cmd
}

// newDevAppRobotEnableCommand enables an app's robot capability. Unlike config,
// it needs no config fields — pure enable, only the app locator.
func newDevAppRobotEnableCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "enable",
		Short:             "启用现有应用机器人能力（纯启用，无需配置字段）",
		Example:           "  dws dev app robot enable --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot enable"); err != nil {
				return err
			}
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			return runDevAppTool(runner, cmd, devAppRobotEnableTool, map[string]any{"unifiedAppId": appID})
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppRobotEnableTool)
	return cmd
}

func newDevAppRobotOfflineCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "disable",
		Short:             "停用现有应用的机器人能力",
		Example:           "  dws dev app robot disable --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot disable"); err != nil {
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
	annotateDevAppTool(cmd, devAppRobotOfflineTool)
	return cmd
}

func registerDevAppRobotCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "智能体应用名称，长度 2-20，企业内唯一 (必填)")
	cmd.Flags().String("robot-name", "", "承载机器人名称，用于客户端展示 (必填)")
	cmd.Flags().String("desc", "", "机器人功能描述，不超过 200 字 (必填)")
	cmd.Flags().String("icon-media-id", "", "机器人图标 mediaId；为空时使用默认图标")
	cmd.Flags().String("preview-media-id", "", "机器人预览图 mediaId；为空时复用图标")
}

func devAppRobotCreateParams(cmd *cobra.Command) (map[string]any, error) {
	name := devAppStringFlag(cmd, "name")
	if name == "" {
		return nil, apperrors.NewValidation("--name 为必填")
	}
	robotName := devAppStringFlag(cmd, "robot-name")
	if robotName == "" {
		return nil, apperrors.NewValidation("--robot-name 为必填")
	}
	desc := devAppStringFlag(cmd, "desc")
	if desc == "" {
		return nil, apperrors.NewValidation("--desc 为必填")
	}
	params := map[string]any{
		"name":      name,
		"robotName": robotName,
		"desc":      desc,
	}
	devAppPutString(params, "iconMediaId", devAppStringFlag(cmd, "icon-media-id"))
	devAppPutString(params, "previewMediaId", devAppStringFlag(cmd, "preview-media-id"))
	return params, nil
}

func registerDevAppRobotConfigFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "机器人名称")
	cmd.Flags().String("brief", "", "机器人简介")
	cmd.Flags().String("desc", "", "机器人描述")
	cmd.Flags().String("icon-media-id", "", "机器人图标 mediaId")
	cmd.Flags().String("outgoing-url", "", "消息回调地址")
	cmd.Flags().String("event-callback-url", "", "事件回调地址")
	cmd.Flags().String("mode", "", "机器人模式：HTTPS / STREAM / AISKILL")
	cmd.Flags().String("skills", "", "技能列表，多个用逗号或分号分隔")
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
	setString("desc", "desc")
	setString("iconMediaId", "icon-media-id")
	setString("outgoingUrl", "outgoing-url")
	setString("eventCallbackUrl", "event-callback-url")
	if cmd.Flags().Changed("mode") {
		mode := strings.ToUpper(strings.TrimSpace(devAppStringFlag(cmd, "mode")))
		switch mode {
		case "HTTPS", "STREAM", "AISKILL":
			params["mode"] = mode
		default:
			return nil, 0, apperrors.NewValidation("--mode 仅支持 HTTPS、STREAM、AISKILL")
		}
		updates++
	}
	if cmd.Flags().Changed("add-scope") {
		value, _ := cmd.Flags().GetBool("add-scope")
		params["addScope"] = value
		updates++
	}
	if cmd.Flags().Changed("disable-ssl-verify") {
		value, _ := cmd.Flags().GetBool("disable-ssl-verify")
		params["disableSSLVerify"] = value
		updates++
	}
	if values := parseDevAppListFlag(cmd, "skills"); len(values) > 0 {
		params["skills"] = values
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
			return nil, 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是合法 JSON 对象：%v", item.flag, err))
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
		Example:           "  dws dev app version create --unified-app-id UNIFIED_APP_ID --desc \"新增机器人能力\" --dry-run --format json",
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
			devAppPutString(params, "desc", devAppStringFlag(cmd, "desc"))
			return runDevAppTool(runner, cmd, devAppVersionCreateTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("version", "", "高级可选：显式版本号，如 1.0.1；默认不传，由服务端基于最新已发布版本自动递增")
	cmd.Flags().String("desc", "", "版本描述")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppVersionCreateTool)
	return cmd
}

func newDevAppVersionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "分页查询应用版本列表",
		Example:           "  dws dev app version list --unified-app-id UNIFIED_APP_ID --page-size 20 --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, err := requiredDevAppUnifiedID(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"unifiedAppId": appID}
			devAppApplyCursorParams(cmd, params)
			return runDevAppTool(runner, cmd, devAppVersionListTool, params)
		},
	}
	addDevAppUnifiedIDFlag(cmd)
	registerDevAppCursorFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppVersionListTool)
	return cmd
}

func newDevAppVersionGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             "查询指定版本详情",
		Example:           "  dws dev app version get --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
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
	annotateDevAppTool(cmd, devAppVersionDetailTool)
	return cmd
}

func newDevAppVersionCheckApprovalCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "check-approval",
		Short:             "预检版本发布是否需要审批（不实际发布）",
		Example:           "  dws dev app version check-approval --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devAppVersionLocator(cmd)
			if err != nil {
				return err
			}
			// 复用 publish 工具的服务端预检模式：precheckOnly=true 只返回审批
			// 要求，不发布。原上游参数名 dryRun 与 CLI --dry-run 撞名，已改名
			// precheckOnly（见 docs/upstream-todo.md，需上游同步改）。
			params["precheckOnly"] = true
			return runDevAppTool(runner, cmd, devAppVersionPublishTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppVersionPublishTool)
	return cmd
}

func newDevAppVersionPublishCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "publish",
		Short:             "发布指定版本（含高敏权限需 --confirmed-sensitive）",
		Example:           "  dws dev app version publish --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --dry-run --format json",
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
			// precheckOnly=false 表示真发布。命名故意避开 CLI 的 --dry-run：
			// 上游接口原参数名是 dryRun，与 CLI 全局 --dry-run 撞名，会让读
			// params 的 agent 误判（publish --dry-run 时回显 dryRun:false）。
			// 见 docs/upstream-todo.md：上游需把入参 dryRun 改名 precheckOnly。
			params["precheckOnly"] = false
			if cmd.Flags().Changed("confirmed-sensitive") {
				value, _ := cmd.Flags().GetBool("confirmed-sensitive")
				params["confirmedSensitive"] = value
			}
			devAppPutString(params, "approverUserId", devAppStringFlag(cmd, "approver-user-id"))
			return runDevAppTool(runner, cmd, devAppVersionPublishTool, params)
		},
	}
	addDevAppVersionLocatorFlags(cmd)
	cmd.Flags().Bool("confirmed-sensitive", false, "确认发布包含高敏权限的版本")
	cmd.Flags().String("approver-user-id", "", "灰度选人模式下指定审批人 userId")
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, devAppVersionPublishTool)
	return cmd
}

func newDevAppVersionStatusCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "status",
		Short:             "查询版本发布/审批状态",
		Example:           "  dws dev app version status --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
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
	annotateDevAppTool(cmd, devAppVersionStatusTool)
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
		return nil, apperrors.NewValidation("--version-id 为必填")
	}
	return map[string]any{"unifiedAppId": appID, "versionId": versionID}, nil
}

// addDevAppUnifiedIDFlag registers the canonical app locator. --unified-app-id
// is the single app identifier across the whole dev app tree (agent-id/app-id/
// custom-key locators were intentionally removed).
func addDevAppUnifiedIDFlag(cmd *cobra.Command) {
	cmd.Flags().String("unified-app-id", "", "开放平台统一应用 ID（必填）")
}

func requiredDevAppUnifiedID(cmd *cobra.Command) (string, error) {
	appID := devAppStringFlag(cmd, "unified-app-id")
	if appID == "" {
		return "", apperrors.NewValidation("--unified-app-id 为必填")
	}
	return appID, nil
}

func registerDevAppMemberMutationFlags(cmd *cobra.Command) {
	addDevAppUnifiedIDFlag(cmd)
	cmd.Flags().String("user-ids", "", "成员 userId 列表，多个用逗号分隔 (必填)")
	cmd.Flags().String("member-user-ids", "", "成员 userId 列表，多个用逗号分隔 (兼容旧参数)")
	_ = cmd.Flags().MarkHidden("member-user-ids")
	cmd.Flags().String("member-type", "", "成员类型，如 DEVELOPER (必填)")
}

func runDevAppMemberMutation(runner executor.Runner, cmd *cobra.Command, tool, operation string) error {
	if err := devAppRequireWriteGuard(cmd, operation); err != nil {
		return err
	}
	appID, err := requiredDevAppUnifiedID(cmd)
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
		"unifiedAppId": appID,
		"userIds":      users,
		"memberType":   memberType,
	}
	return runDevAppTool(runner, cmd, tool, params)
}

// annotateDevAppTool tags a leaf command with the MCP tool name it invokes, so
// `dws schema dev.app...` can resolve command → tool → live op-app schema
// without re-deriving the mapping. The annotation is the single source of truth
// for the schema renderer (see internal/cli/dev_schema.go).
func annotateDevAppTool(cmd *cobra.Command, tool string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["mcp-tool"] = tool
	cmd.Annotations["mcp-source"] = "op-app"
	return cmd
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
	// Unwrap the ServiceResult envelope and apply per-tool response fixes before
	// rendering, so agents read the inner payload directly and pretty-annotation
	// walks the already-normalized content.
	result = normalizeDevAppToolResult(tool, normalizeDevAppServiceResult(result))
	if devAppPrettyWanted(cmd) {
		devAppPrettyAnnotate(tool, result.Response)
	}
	return writeCommandPayload(cmd, result)
}

// normalizeDevAppServiceResult unwraps the op-app ServiceResult envelope
// ({content:{success:true, result:{...}}}) down to its inner result, so a
// successful tool call renders its payload directly instead of the wrapper.
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

// normalizeDevAppToolResult applies per-tool response shape fixes: flatten
// remove-permission's removedScopeValues to a string array, stamp explicit
// lifecycle booleans, and enrich async robot creation results with next steps.
func normalizeDevAppToolResult(tool string, result executor.Result) executor.Result {
	content, ok := result.Response["content"].(map[string]any)
	if !ok {
		return result
	}
	switch tool {
	case devAppPermissionRmTool:
		normalizeDevAppScopeValueArray(content, "removedScopeValues")
	case devAppDisableTool:
		if _, ok := content["disabled"]; !ok {
			content["disabled"] = true
		}
	case devAppEnableTool:
		if _, ok := content["enabled"]; !ok {
			content["enabled"] = true
		}
	case devAppVersionPublishTool:
		normalizeDevAppVersionApproval(content)
	case devAppRobotResultTool:
		normalizeDevAppRobotResult(content)
	}
	return result
}

func normalizeDevAppVersionApproval(content map[string]any) {
	candidates, ok := content["approvalCandidates"].([]any)
	if !ok || len(candidates) == 0 {
		return
	}
	options := make([]map[string]any, 0, len(candidates))
	for i, raw := range candidates {
		candidate, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		userID := devAppFirstContentString(candidate, "userId", "userID", "userid", "staffId")
		name := devAppFirstContentString(candidate, "name", "userName", "displayName", "nick", "nickName")
		mainAdmin := devAppContentBool(candidate, "mainAdmin")
		label := devAppApprovalCandidateLabel(name, userID, mainAdmin)
		if label == "" {
			label = fmt.Sprintf("候选审批人 %d", i+1)
		}
		option := map[string]any{
			"index":     i + 1,
			"key":       devAppOptionKey(i),
			"label":     label,
			"name":      name,
			"userId":    userID,
			"mainAdmin": mainAdmin,
		}
		options = append(options, option)
	}
	if len(options) == 0 {
		return
	}

	content["approvalOptions"] = options

	approvalMode := strings.ToUpper(devAppContentString(content, "approvalMode"))
	if approvalMode != "SELECT_APPROVER" {
		return
	}

	unifiedAppID := devAppContentString(content, "unifiedAppId")
	if unifiedAppID == "" {
		unifiedAppID = "<unifiedAppId>"
	}
	versionID := devAppContentString(content, "versionId")
	if versionID == "" {
		versionID = "<versionId>"
	}
	// 预渲染一段"原样照抄即可"的审批人列表：序号复用 approvalOptions[].key
	// （A-Z 后转数字），label 已是「姓名（userId: xxx）」。agent 直接展示
	// approvalPromptText 即可，无需自己遍历 approvalOptions——此前有 agent 误把
	// approvalOptions 当成 [{options:[...]}]、取空后只回退显示 userId，姓名全丢。
	title := fmt.Sprintf("版本发布需要审批，请选择一位审批人（共 %d 位）：", len(options))
	var promptBuilder strings.Builder
	promptBuilder.WriteString(title)
	for _, opt := range options {
		key, _ := opt["key"].(string)
		label, _ := opt["label"].(string)
		promptBuilder.WriteString(fmt.Sprintf("\n%s. %s", key, label))
	}
	promptText := promptBuilder.String()

	content["completionState"] = "WAITING_FOR_APPROVER_SELECTION"
	content["actionRequired"] = "select_approver"
	content["mustAskUser"] = true
	content["requiresUserInput"] = true
	content["terminal"] = false
	content["approvalPromptText"] = promptText
	content["message"] = "版本发布需要选择审批人；请原样展示 approvalPromptText 的完整内容，等待用户选择，不要只显示 userId、不要自行截取、不能默认取第一个"
	content["nextSteps"] = []map[string]any{
		{
			"id":                "select_approver",
			"blocking":          true,
			"requiresUserInput": true,
			"doneWhen":          "用户从 approvalOptions 中选择一位审批人，得到对应 userId",
		},
		devAppNextStep(devAppStep{
			ID:            "publish_version",
			Command:       fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id %s --approver-user-id <selectedUserId> --yes --format json", unifiedAppID, versionID),
			DryRunCommand: fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id %s --approver-user-id <selectedUserId> --dry-run --format json", unifiedAppID, versionID),
			DoneWhen:      "approvalSubmitted=true、versionStatus=AUDIT 或 processStatus=UNDER_REVIEW 表示已提交审批；published=true 表示已发布",
			Blocking:      true,
		}),
	}
}

func devAppApprovalCandidateLabel(name, userID string, mainAdmin bool) string {
	label := strings.TrimSpace(name)
	switch {
	case label != "" && userID != "":
		label = fmt.Sprintf("%s（userId: %s）", label, userID)
	case label == "" && userID != "":
		label = "userId: " + userID
	}
	if label != "" && mainAdmin {
		label += "（主管理员）"
	}
	return label
}

func devAppOptionKey(index int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if index >= 0 && index < len(letters) {
		return string(letters[index])
	}
	return fmt.Sprintf("%d", index+1)
}

func normalizeDevAppRobotResult(content map[string]any) {
	status := strings.ToUpper(devAppContentString(content, "status"))
	if status == "" {
		return
	}

	taskID := devAppContentString(content, "taskId")
	clientID := devAppFirstContentString(content, "clientId", "appKey")
	clientSecret := devAppFirstContentString(content, "clientSecret", "appSecret")
	unifiedAppID := devAppContentString(content, "unifiedAppId")
	localConnectReady := clientID != "" && clientSecret != ""

	lifecycle := map[string]any{
		"status":                 status,
		"localConnectReady":      false,
		"localOnlyReady":         false,
		"publicUseReady":         false,
		"requiresVersionPublish": false,
		"robotTaskDone":          false,
		"overallComplete":        false,
	}
	var steps []map[string]any

	switch status {
	case "WAITING":
		lifecycle["phase"] = "creating"
		lifecycle["completionGate"] = "robot_result"
		if interval := content["intervalSeconds"]; interval != nil {
			lifecycle["retryAfterSeconds"] = interval
		}
		steps = append(steps, devAppRobotPollStep(taskID))
	case "SUCCESS", "APPROVAL_REQUIRED":
		lifecycle["phase"] = "created_pending_publish"
		lifecycle["localConnectReady"] = localConnectReady
		lifecycle["localOnlyReady"] = localConnectReady
		lifecycle["requiresVersionPublish"] = true
		lifecycle["robotTaskDone"] = true
		if unifiedAppID == "" {
			lifecycle["completionGate"] = "provide_unified_app_id"
			lifecycle["blockingStepIds"] = []string{"provide_unified_app_id"}
			steps = append(steps, devAppRobotProvideUnifiedAppIDStep())
			devAppMarkMissingUnifiedAppIDBlocked(content)
		} else {
			lifecycle["completionGate"] = "version_publish"
			lifecycle["blockingStepIds"] = devAppRobotPublishStepIDs()
			steps = append(steps, devAppRobotPublishSteps(unifiedAppID)...)
			devAppMarkVersionPublishBlocked(content)
		}
		if localConnectReady {
			steps = append(steps, devAppRobotConnectStep(clientID))
		}
	case "FAIL":
		lifecycle["phase"] = "failed"
		lifecycle["robotTaskDone"] = true
		lifecycle["completionGate"] = "retry_robot_submit"
		steps = append(steps, devAppRobotRetryStep(taskID, true))
	case "EXPIRED":
		lifecycle["phase"] = "expired"
		lifecycle["robotTaskDone"] = true
		lifecycle["completionGate"] = "retry_robot_submit"
		steps = append(steps, devAppRobotRetryStep(taskID, false))
	default:
		lifecycle["phase"] = "unknown"
	}

	content["lifecycle"] = lifecycle
	if len(steps) > 0 {
		content["nextSteps"] = steps
	}
}

func devAppMarkVersionPublishBlocked(content map[string]any) {
	content["completionState"] = "BLOCKED_BY_VERSION_PUBLISH"
	content["mustContinue"] = true
	content["actionRequired"] = "submit_version_publish"
	content["message"] = "本地建联可用，但线上发布/审批未完成；必须继续执行 blocking nextSteps"
	content["terminal"] = false
}

func devAppMarkMissingUnifiedAppIDBlocked(content map[string]any) {
	content["completionState"] = "BLOCKED_BY_MISSING_UNIFIED_APP_ID"
	content["mustContinue"] = true
	content["mustAskUser"] = true
	content["actionRequired"] = "provide_unified_app_id"
	content["message"] = "缺少明确来源的 unifiedAppId，不能用 clientId/appKey 反查后写版本；请提供 dev app create 或 robot result 返回的 unifiedAppId"
	content["terminal"] = false
}

func devAppRobotPublishSteps(appID string) []map[string]any {
	steps := []map[string]any{
		devAppNextStep(devAppStep{
			ID:            "create_version",
			Command:       fmt.Sprintf("dws dev app version create --unified-app-id %s --desc \"发布机器人能力\" --yes --format json", appID),
			DryRunCommand: fmt.Sprintf("dws dev app version create --unified-app-id %s --desc \"发布机器人能力\" --dry-run --format json", appID),
			DoneWhen:      "返回 versionId",
			Blocking:      true,
		}),
		devAppNextStep(devAppStep{
			ID:       "check_approval",
			Command:  fmt.Sprintf("dws dev app version check-approval --unified-app-id %s --version-id <versionId> --format json", appID),
			DoneWhen: "返回 requiresApproval、approvalMode、approvalCandidates 等审批信息",
			Blocking: true,
		}),
		devAppNextStep(devAppStep{
			ID:                "publish_version",
			Command:           fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id <versionId> --yes --format json", appID),
			DryRunCommand:     fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id <versionId> --dry-run --format json", appID),
			DoneWhen:          "published=true 表示已发布；approvalSubmitted=true、versionStatus=AUDIT 或 processStatus=UNDER_REVIEW 表示已提交审批；SELECT_APPROVER 时必须先让用户从 approvalCandidates 选择审批人后追加 --approver-user-id",
			RequiresUserInput: true,
			Blocking:          true,
		}),
		devAppNextStep(devAppStep{
			ID:       "wait_release",
			Command:  fmt.Sprintf("dws dev app version status --unified-app-id %s --version-id <versionId> --format json", appID),
			DoneWhen: "versionStatus=RELEASE 表示已生效；versionStatus=AUDIT 或 processStatus=UNDER_REVIEW 表示已提交审批，等待审批通过",
			Blocking: true,
		}),
	}
	return steps
}

func devAppRobotPublishStepIDs() []string {
	return []string{"create_version", "check_approval", "publish_version", "wait_release"}
}

func devAppRobotProvideUnifiedAppIDStep() map[string]any {
	return map[string]any{
		"id":                "provide_unified_app_id",
		"blocking":          true,
		"requiresUserInput": true,
		"doneWhen":          "用户提供 dev app create 或 robot result 返回的明确 unifiedAppId；不能用 clientId/appKey 自动反查后继续写版本",
	}
}

func devAppRobotPollStep(taskID string) map[string]any {
	if taskID == "" {
		taskID = "<taskId>"
	}
	return devAppNextStep(devAppStep{
		ID:       "poll_robot_result",
		Command:  fmt.Sprintf("dws dev app robot result --task-id %s --format json", taskID),
		DoneWhen: "status 变为 SUCCESS、APPROVAL_REQUIRED、FAIL 或 EXPIRED",
		Blocking: true,
	})
}

func devAppRobotRetryStep(taskID string, reuseTaskID bool) map[string]any {
	taskIDFlag := ""
	if reuseTaskID {
		if taskID == "" {
			taskID = "<taskId>"
		}
		taskIDFlag = " --task-id " + taskID
	}
	return devAppNextStep(devAppStep{
		ID:            "retry_robot_submit",
		Command:       fmt.Sprintf("dws dev app robot submit --name <name> --robot-name <robotName> --desc <desc>%s --yes --format json", taskIDFlag),
		DryRunCommand: fmt.Sprintf("dws dev app robot submit --name <name> --robot-name <robotName> --desc <desc>%s --dry-run --format json", taskIDFlag),
		DoneWhen:      "返回新的 WAITING taskId；FAIL 场景优先复用原 taskId，EXPIRED 场景重新提交",
		Blocking:      true,
	})
}

func devAppRobotConnectStep(clientID string) map[string]any {
	if clientID == "" {
		clientID = "<clientId>"
	}
	step := devAppNextStep(devAppStep{
		ID:       "connect_local",
		Command:  fmt.Sprintf("dws dev connect --robot-client-id %s --robot-client-secret <clientSecret-from-result> --format json", clientID),
		DoneWhen: "本地 Stream 建联成功，进程保持运行；clientSecret 只使用返回值，不写入命令建议",
	})
	step["sensitiveFields"] = []string{"clientSecret"}
	step["optional"] = true
	step["scope"] = "local_debug_only"
	return step
}

// devAppStep describes one nextSteps entry. Using named fields keeps call sites
// self-documenting instead of relying on a trailing pair of positional bools.
type devAppStep struct {
	ID                string
	Command           string
	DryRunCommand     string
	DoneWhen          string
	RequiresUserInput bool
	Blocking          bool
}

func devAppNextStep(step devAppStep) map[string]any {
	out := map[string]any{
		"id":                step.ID,
		"requiresUserInput": step.RequiresUserInput,
		"blocking":          step.Blocking,
		"doneWhen":          step.DoneWhen,
	}
	if step.Command != "" {
		out["command"] = step.Command
	}
	if step.DryRunCommand != "" {
		out["dryRunCommand"] = step.DryRunCommand
	}
	return out
}

func devAppFirstContentString(content map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := devAppContentString(content, key); value != "" {
			return value
		}
	}
	return ""
}

func devAppContentString(content map[string]any, key string) string {
	value, ok := content[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func devAppContentBool(content map[string]any, key string) bool {
	value, ok := content[key]
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

// normalizeDevAppScopeValueArray rewrites an array of scope objects (or strings)
// into a flat string array of scopeValues, leaving the field untouched if any
// element is an unexpected shape.
func normalizeDevAppScopeValueArray(content map[string]any, key string) {
	values, ok := content[key].([]any)
	if !ok {
		return
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				out = append(out, typed)
			}
		case map[string]any:
			if scopeValue, _ := typed["scopeValue"].(string); scopeValue != "" {
				out = append(out, scopeValue)
			}
		}
	}
	if len(out) == len(values) {
		content[key] = out
	}
}

func requiredDevAppUsers(cmd *cobra.Command) ([]string, error) {
	usersRaw, _ := cmd.Flags().GetString("user-ids")
	if strings.TrimSpace(usersRaw) == "" {
		usersRaw, _ = cmd.Flags().GetString("member-user-ids")
	}
	if strings.TrimSpace(usersRaw) == "" {
		return nil, apperrors.NewValidation("--user-ids 为必填")
	}
	users := splitDevAppList(usersRaw)
	if len(users) == 0 {
		return nil, apperrors.NewValidation("--user-ids 至少包含一个 userId")
	}
	return users, nil
}

func requiredDevAppMemberType(cmd *cobra.Command) (string, error) {
	memberType, _ := cmd.Flags().GetString("member-type")
	memberType = strings.TrimSpace(memberType)
	if memberType == "" {
		return "", apperrors.NewValidation("--member-type 为必填")
	}
	return memberType, nil
}

func parseDevAppListFlag(cmd *cobra.Command, name string) []string {
	raw, _ := cmd.Flags().GetString(name)
	return splitDevAppList(raw)
}

func requiredDevAppEventCodes(cmd *cobra.Command) ([]string, error) {
	eventCodes := parseDevAppListFlag(cmd, "event-codes")
	if len(eventCodes) == 0 {
		return nil, apperrors.NewValidation("--event-codes 为必填")
	}
	return eventCodes, nil
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

// 应用定位统一只用 --unified-app-id（--app-key / --name 定位已下线，列表搜索的
// --name/--app-key 是过滤参数、不在此列）。所有 app 作用域命令共用 addDevAppUnifiedIDFlag
// + requiredDevAppUnifiedID。

func devAppRequireWriteGuard(cmd *cobra.Command, operation string) error {
	if commandDryRun(cmd) || devAppYes(cmd) {
		return nil
	}
	return apperrors.NewValidation(fmt.Sprintf("%s 是写操作；加 --dry-run 预览，或确认后加 --yes 执行", operation))
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

func devAppCloneParams(src map[string]any) map[string]any {
	out := make(map[string]any, len(src)+1)
	for k, v := range src {
		out[k] = v
	}
	return out
}

func devAppPermissionScopes(cmd *cobra.Command) []string {
	values := parseDevAppListFlag(cmd, "scope-values")
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
