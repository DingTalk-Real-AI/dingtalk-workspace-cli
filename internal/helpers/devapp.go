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
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询应用已订阅的事件列表",
		Example: "  dws dev app event list --unified-app-id UNIFIED_APP_ID --page-size 20 --format json",
		Tool:    devAppEventListTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "keyword", Usage: "事件搜索关键词，支持按事件码或事件名称模糊匹配", Bind: "keyword", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		// cursor/pageSize 由 devAppApplyCursorParams 注入（page-size 默认 20、floor 20）。
		Call: devAppCallCursor(runner),
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppEventListTool)
			registerDevAppCursorFlags(cmd)
		},
	})
}

func newDevAppEventSubscribeCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "subscribe",
		Short:   "订阅应用事件回调",
		Example: "  dws dev app event subscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Tool:    devAppEventSubscribeTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "event subscribe"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppEventCodes(cmd); err != nil {
				return err
			}
			return nil
		},
		// eventCodes 数组由 Call 解析注入（与手写 params["eventCodes"] 等价）。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["eventCodes"] = parseDevAppListFlag(cmd, "event-codes")
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppEventSubscribeTool)
			cmd.Flags().String("event-codes", "", "事件码，多个用逗号或分号分隔")
		},
	})
}

func newDevAppEventUnsubscribeCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "unsubscribe",
		Short:   "取消订阅应用事件",
		Example: "  dws dev app event unsubscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Tool:    devAppEventUnsubscribeTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "event unsubscribe"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppEventCodes(cmd); err != nil {
				return err
			}
			return nil
		},
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["eventCodes"] = parseDevAppListFlag(cmd, "event-codes")
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppEventUnsubscribeTool)
			cmd.Flags().String("event-codes", "", "事件码，多个用逗号或分号分隔")
		},
	})
}

func newDevAppListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询开放平台企业内部应用列表",
		Example: "  dws dev app list --name DemoApp --page-size 20 --format json",
		Tool:    devAppListTool,
		Flags: []LeafFlag{
			{Name: "name", Usage: "应用名称关键词", Bind: "name", Trim: true, OmitEmpty: true, Aliases: []string{"keyword"}},
			{Name: "app-key", Usage: "按 appKey/clientId 过滤", Bind: "appKey", Trim: true, OmitEmpty: true},
			{Name: "app-group-id", Usage: "应用分组 ID", Kind: LeafInt, Bind: "appGroupId"},
			{Name: "creator", Usage: "创建人名称关键词", Bind: "creator", Trim: true, OmitEmpty: true},
			{Name: "robot-name", Usage: "机器人名称关键词", Bind: "robotName", Trim: true, OmitEmpty: true},
			{Name: "develop-type", Usage: "开发类型枚举；不确定时不要传", Kind: LeafInt, Bind: "developType"},
			{Name: "filter-cool-app", Usage: "酷应用过滤枚举；不确定时不要传", Kind: LeafInt, Bind: "filterCoolApp"},
			{Name: "sort-type", Usage: "排序字段，如 gmt_modified", Bind: "sortType", Trim: true, OmitEmpty: true},
			{Name: "sort-order", Usage: "排序方向 asc 或 desc", Bind: "sortOrder", Trim: true, OmitEmpty: true},
		},
		// cursor/pageSize 由 devAppApplyCursorParams 注入（退役 devAppApplyCursorParams 之外，
		// 本命令同时退役 devAppPutString/devAppPutInt/devAppFlagOrFallback 全套手搓 helper）。
		Call: devAppCallCursor(runner),
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppListTool)
			registerDevAppCursorFlags(cmd)
		},
	})
}

func newDevAppGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询开放平台企业内部应用详情",
		Example: "  dws dev app get --unified-app-id UNIFIED_APP_ID --format json\n  dws dev app get --app-key APP_KEY --format json",
		Tool:    devAppGetTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（与 --app-key 二选一）", Bind: "unifiedAppId", Trim: true, OmitEmpty: true},
			{Name: "app-key", Usage: "按 appKey/clientId 查询应用详情（与 --unified-app-id 二选一）", Bind: "appKey", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			// 二选一：与原 buildDevAppGetParams 的判空等价（该 helper 已随迁移移除）。
			if devAppStringFlag(cmd, "unified-app-id") == "" && devAppStringFlag(cmd, "app-key") == "" {
				return apperrors.NewValidation("请传入 --unified-app-id 或 --app-key")
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppGetTool),
	})
}

func newDevAppCreateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "create",
		Short:   "创建开放平台企业内部应用",
		Example: "  dws dev app create --name DemoApp --desc 内部应用 --dry-run --format json",
		Tool:    devAppCreateTool,
		Flags: []LeafFlag{
			{Name: "name", Usage: "应用名称 (必填)", Bind: "name", Trim: true},
			{Name: "desc", Usage: "应用描述", Bind: "desc", Trim: true, OmitEmpty: true},
			{Name: "icon-media-id", Usage: "应用图标 mediaId", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "create"); err != nil {
				return err
			}
			if devAppStringFlag(cmd, "name") == "" {
				return apperrors.NewValidation("--name 为必填")
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppCreateTool),
	})
}

func newDevAppUpdateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "update",
		Short:   "修改开放平台企业内部应用基础信息",
		Example: "  dws dev app update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --dry-run --format json",
		Tool:    devAppUpdateTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "name", Usage: "新的应用名称", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "desc", Usage: "新的应用描述", Bind: "desc", Trim: true, OmitEmpty: true},
			{Name: "icon-media-id", Usage: "新的应用图标 mediaId", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "update"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if devAppStringFlag(cmd, "name") == "" && devAppStringFlag(cmd, "desc") == "" &&
				devAppStringFlag(cmd, "icon-media-id") == "" {
				return apperrors.NewValidation("至少提供一项待更新字段：--name、--desc 或 --icon-media-id")
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppUpdateTool),
	})
}

func newDevAppCredentialsGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "读取开放平台应用凭证",
		Example: "  dws dev app credentials get --unified-app-id UNIFIED_APP_ID --format json",
		Tool:    devAppCredentialsGetTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppCredentialsGetTool),
	})
}

func newDevAppLifecycleCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   use,
		Short: short,
		Tool:  tool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			// 与手写版顺序一致：先写操作守卫，再校验 unified-app-id。
			if err := devAppRequireWriteGuard(cmd, use); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(tool),
	})
}

// newDevAppDeleteCommand is delete with a danger tier: deleting an app is
// irreversible, so beyond the write guard it requires --confirm-name to match
// the located app's real name. This guards against "located the wrong app and
// deleted it" — the agent must first know the name (via `get`/dry-run) before
// it can delete. The match is verified client-side (a `get` then compare),
// standard practice for destructive CLI ops (gh repo delete, gcloud).
//
// 保留手写、不迁 LeafSpec：含 get-then-compare 的 confirm-name 多步二次确认，
// 属多步编排，非声明式 flag 范畴（见 PLAN A2）。
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
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询网页应用配置",
		Example: "  dws dev app webapp get --unified-app-id UNIFIED_APP_ID --format json",
		Tool:    devAppWebappGetTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppWebappGetTool),
	})
}

func newDevAppWebappConfigCommand(runner executor.Runner) *cobra.Command {
	const op = "webapp config"
	return NewLeafCommand(LeafSpec{
		Use:     "config",
		Short:   "配置网页应用能力",
		Example: "  dws dev app webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com --dry-run --format json",
		Tool:    devAppWebappSetTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "h5-page-type", Usage: "网页应用生效端/页面类型", Bind: "h5PageType", Trim: true, OmitEmpty: true},
			{Name: "homepage-url", Usage: "移动端首页地址", Bind: "homepageUrl", Trim: true, OmitEmpty: true},
			{Name: "pc-homepage-url", Usage: "PC 端首页地址", Bind: "pcHomepageUrl", Trim: true, OmitEmpty: true},
			{Name: "omp-url", Usage: "管理后台地址", Bind: "ompUrl", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, op); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			// 至少一项配置：与手写版 updates==0 判定等价。
			if devAppStringFlag(cmd, "h5-page-type") == "" && devAppStringFlag(cmd, "homepage-url") == "" &&
				devAppStringFlag(cmd, "pc-homepage-url") == "" && devAppStringFlag(cmd, "omp-url") == "" {
				return apperrors.NewValidation("至少提供一项网页应用配置：--h5-page-type、--homepage-url、--pc-homepage-url 或 --omp-url")
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppWebappSetTool),
	})
}

func newDevAppPermissionListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询开放平台应用权限列表",
		Example: "  dws dev app permission list --unified-app-id UNIFIED_APP_ID --keyword 通讯录 --page-size 20 --format json",
		Tool:    devAppPermissionListTool,
		// 命令级别名 "search" 由 PostMount 设回（LeafSpec 无 Command Aliases 字段）。
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "keyword", Usage: "权限名、权限点、接口名关键词", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "scope-value", Usage: "精确权限点 scopeValue", Bind: "scopeValue", Trim: true, OmitEmpty: true},
			{Name: "auth-status", Usage: "权限状态：ALL、AUTHED、UNAUTHED", Default: "ALL", Bind: "authStatus", Trim: true, OmitEmpty: true, Transform: func(raw string) (any, error) { return strings.ToUpper(raw), nil }},
			{Name: "scope-type", Usage: "权限一级类型：APP 或 SNS", Bind: "scopeType", Trim: true, OmitEmpty: true, Transform: func(raw string) (any, error) { return strings.ToUpper(raw), nil }},
			{Name: "api-status", Usage: "开发者后台 apiStatus 过滤", Bind: "apiStatus", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call: devAppCallCursor(runner),
		PostMount: func(cmd *cobra.Command) {
			cmd.Aliases = []string{"search"}
			devAppLeafMeta(cmd, devAppPermissionListTool)
			registerDevAppCursorFlags(cmd)
		},
	})
}

func newDevAppPermissionAddCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "add",
		Short:   "申请开放平台应用权限点",
		Example: "  dws dev app permission add --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile --dry-run --format json",
		Tool:    devAppPermissionAddTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission add"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if len(devAppPermissionScopes(cmd)) == 0 {
				return apperrors.NewValidation("--scope-values 为必填")
			}
			return nil
		},
		// scope-values 经 devAppPermissionScopes 解析为 []string，由 Call 注入。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["scopeValues"] = devAppPermissionScopes(cmd)
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppPermissionAddTool)
			cmd.Flags().String("scope-values", "", "权限点 scopeValue，多个用逗号或分号分隔")
		},
	})
}

func newDevAppPermissionRemoveCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "remove",
		Short:   "取消开放平台应用权限点",
		Example: "  dws dev app permission remove --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile,qyapi_robot_sendmsg --dry-run --format json",
		Tool:    devAppPermissionRmTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "permission remove"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if len(devAppPermissionScopes(cmd)) == 0 {
				return apperrors.NewValidation("--scope-values 为必填")
			}
			return nil
		},
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["scopeValues"] = devAppPermissionScopes(cmd)
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppPermissionRmTool)
			cmd.Flags().String("scope-values", "", "待取消权限点 scopeValue，多个用逗号或分号分隔")
		},
	})
}

func newDevAppMemberListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询开放平台应用成员",
		Example: "  dws dev app member list --unified-app-id <unifiedAppId>",
		Tool:    devAppMemberListTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppMemberListTool),
	})
}

func newDevAppMemberAddCommand(runner executor.Runner) *cobra.Command {
	const op = "member add"
	return NewLeafCommand(LeafSpec{
		Use:     "add",
		Short:   "添加开放平台应用成员",
		Example: "  dws dev app member add --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Tool:    devAppMemberAddTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "member-type", Usage: "成员类型，如 DEVELOPER (必填)", Bind: "memberType", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, op); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppUsers(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppMemberType(cmd); err != nil {
				return err
			}
			return nil
		},
		// userIds 由 Call 解析注入（user-ids / member-user-ids 别名回退）。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			users, _ := requiredDevAppUsers(cmd)
			params["userIds"] = users
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppMemberAddTool)
			cmd.Flags().String("user-ids", "", "成员 userId 列表，多个用逗号分隔 (必填)")
			cmd.Flags().String("member-user-ids", "", "成员 userId 列表，多个用逗号分隔 (兼容旧参数)")
			_ = cmd.Flags().MarkHidden("member-user-ids")
		},
	})
}

func newDevAppMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	const op = "member remove"
	return NewLeafCommand(LeafSpec{
		Use:     "remove",
		Short:   "移除开放平台应用成员",
		Example: "  dws dev app member remove --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Tool:    devAppMemberRemoveTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "member-type", Usage: "成员类型，如 DEVELOPER (必填)", Bind: "memberType", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, op); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppUsers(cmd); err != nil {
				return err
			}
			if _, err := requiredDevAppMemberType(cmd); err != nil {
				return err
			}
			return nil
		},
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			users, _ := requiredDevAppUsers(cmd)
			params["userIds"] = users
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppMemberRemoveTool)
			cmd.Flags().String("user-ids", "", "成员 userId 列表，多个用逗号分隔 (必填)")
			cmd.Flags().String("member-user-ids", "", "成员 userId 列表，多个用逗号分隔 (兼容旧参数)")
			_ = cmd.Flags().MarkHidden("member-user-ids")
		},
	})
}

func newDevAppSecurityConfigCommand(runner executor.Runner) *cobra.Command {
	const op = "security config"
	return NewLeafCommand(LeafSpec{
		Use:   "config",
		Short: "更新开放平台应用安全配置",
		Example: "  dws dev app security config --unified-app-id <unifiedAppId> " +
			"--ip-whitelist 192.0.2.10 --redirect-urls https://callback.example.invalid/callback --sso-urls https://sso.example.invalid/sso --dry-run",
		Tool: devAppSecurityConfigTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, op); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			// 至少一项：与手写 updates==0 判定等价。
			if len(parseDevAppListFlag(cmd, "ip-whitelist")) == 0 &&
				len(parseDevAppListFlag(cmd, "redirect-urls")) == 0 &&
				len(parseDevAppListFlag(cmd, "sso-urls")) == 0 {
				return apperrors.NewValidation("至少提供一项安全配置：--ip-whitelist、--redirect-urls 或 --sso-urls")
			}
			return nil
		},
		// 三个列表 flag 经 parseDevAppListFlag 解析，非空才入参（与手写一致）。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			if v := parseDevAppListFlag(cmd, "ip-whitelist"); len(v) > 0 {
				params["ipWhitelist"] = v
			}
			if v := parseDevAppListFlag(cmd, "redirect-urls"); len(v) > 0 {
				params["redirectUrls"] = v
			}
			if v := parseDevAppListFlag(cmd, "sso-urls"); len(v) > 0 {
				params["ssoUrls"] = v
			}
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppSecurityConfigTool)
			cmd.Flags().String("ip-whitelist", "", "出口 IP 白名单，多个用逗号或分号分隔（整组覆盖，非追加）")
			cmd.Flags().String("redirect-urls", "", "登录重定向 URL，多个用逗号或分号分隔（整组覆盖，非追加）")
			cmd.Flags().String("sso-urls", "", "端内免登地址，多个用逗号或分号分隔（整组覆盖，非追加）")
		},
	})
}

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

// 保留手写、不迁 LeafSpec：devAppRobotCreateParams 自定义构造 + icon/preview
// 空串占位 + 失败重试 taskId 编排，非声明式 flag 范畴（见 PLAN A2）。
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

// 保留手写、不迁 LeafSpec：按 taskId 轮询异步任务结果的多步编排，
// 非声明式 flag 范畴（见 PLAN A2）。
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
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询现有应用的机器人配置",
		Example: "  dws dev app robot get --unified-app-id UNIFIED_APP_ID --format json",
		Tool:    devAppRobotConfigGetTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppRobotConfigGetTool),
	})
}

// newDevAppRobotConfigCommand is the upsert command for an app's robot config:
// one command for both "首次创建" and "更新" — the create-vs-update decision is
// the upstream tool's job, not the CLI's (see docs/upstream-todo.md, where the
// old create/update tools merge into one `set_dev_app_robot_config`).
// `enable` (pure enable, no config fields) is a separate command.
//
// 保留手写、不迁 LeafSpec：devAppRobotConfigParams 含 mode enum 校验、2 个
// Bool Changed 语义、skills 列表、3 个 i18n JSON 解析 + 至少一项计数，迁需给
// LeafSpec 加 LeafBool/Changed/enum/JSON，框架膨胀收益为负（见 PLAN A2）。
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
	return NewLeafCommand(LeafSpec{
		Use:     "enable",
		Short:   "启用现有应用机器人能力（纯启用，无需配置字段）",
		Example: "  dws dev app robot enable --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Tool:    devAppRobotEnableTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot enable"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppRobotEnableTool),
	})
}

func newDevAppRobotOfflineCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "disable",
		Short:   "停用现有应用的机器人能力",
		Example: "  dws dev app robot disable --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Tool:    devAppRobotOfflineTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "robot disable"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppRobotOfflineTool),
	})
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
	return NewLeafCommand(LeafSpec{
		Use:     "create",
		Short:   "基于当前配置创建应用新版本",
		Example: "  dws dev app version create --unified-app-id UNIFIED_APP_ID --desc \"新增机器人能力\" --dry-run --format json",
		Tool:    devAppVersionCreateTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "version", Usage: "高级可选：显式版本号，如 1.0.1；默认不传，由服务端基于最新已发布版本自动递增", Bind: "version", Trim: true, OmitEmpty: true},
			{Name: "desc", Usage: "版本描述", Bind: "desc", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "version create"); err != nil {
				return err
			}
			if _, err := requiredDevAppUnifiedID(cmd); err != nil {
				return err
			}
			return nil
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppVersionCreateTool),
	})
}

func newDevAppVersionListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "分页查询应用版本列表",
		Example: "  dws dev app version list --unified-app-id UNIFIED_APP_ID --page-size 20 --format json",
		Tool:    devAppVersionListTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := requiredDevAppUnifiedID(cmd)
			return err
		},
		Call: devAppCallCursor(runner),
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppVersionListTool)
			registerDevAppCursorFlags(cmd)
		},
	})
}

func newDevAppVersionGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询指定版本详情",
		Example: "  dws dev app version get --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Tool:    devAppVersionDetailTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := devAppVersionLocator(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppVersionDetailTool),
	})
}

func newDevAppVersionCheckApprovalCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "check-approval",
		Short:   "预检版本发布是否需要审批（不实际发布）",
		Example: "  dws dev app version check-approval --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Tool:    devAppVersionPublishTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := devAppVersionLocator(cmd)
			return err
		},
		// 复用 publish 工具的服务端预检模式：precheckOnly=true 只返回审批要求，不发布。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["precheckOnly"] = true
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: devAppMeta(devAppVersionPublishTool),
	})
}

func newDevAppVersionPublishCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "publish",
		Short:   "发布指定版本（含高敏权限需 --confirmed-sensitive）",
		Example: "  dws dev app version publish --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --dry-run --format json",
		Tool:    devAppVersionPublishTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true},
			{Name: "approver-user-id", Usage: "灰度选人模式下指定审批人 userId", Bind: "approverUserId", Trim: true, OmitEmpty: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			if err := devAppRequireWriteGuard(cmd, "version publish"); err != nil {
				return err
			}
			if _, err := devAppVersionLocator(cmd); err != nil {
				return err
			}
			return nil
		},
		// precheckOnly=false（真发布）+ confirmed-sensitive 的 Changed() 语义由 Call 处理
		//（框架无 LeafBool/Changed 语义，Bool flag 在 PostMount 注册）。
		Call: func(cmd *cobra.Command, tool string, params map[string]any) error {
			params["precheckOnly"] = false
			if cmd.Flags().Changed("confirmed-sensitive") {
				value, _ := cmd.Flags().GetBool("confirmed-sensitive")
				params["confirmedSensitive"] = value
			}
			return runDevAppTool(runner, cmd, tool, params)
		},
		PostMount: func(cmd *cobra.Command) {
			devAppLeafMeta(cmd, devAppVersionPublishTool)
			cmd.Flags().Bool("confirmed-sensitive", false, "确认发布包含高敏权限的版本")
		},
	})
}

func newDevAppVersionStatusCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "status",
		Short:   "查询版本发布/审批状态",
		Example: "  dws dev app version status --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Tool:    devAppVersionStatusTool,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true},
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := devAppVersionLocator(cmd)
			return err
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppVersionStatusTool),
	})
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

// devAppLeafMeta 是 devapp 叶子命令统一的 PostMount 收尾：设置 NoArgs /
// DisableAutoGenTag，并调用 preferLegacyLeaf + annotateDevAppTool。供迁移到
// LeafSpec 的命令在 LeafSpec.PostMount 里复用，保持与手写版逐字等价。
func devAppLeafMeta(cmd *cobra.Command, tool string) {
	cmd.Args = cobra.NoArgs
	cmd.DisableAutoGenTag = true
	preferLegacyLeaf(cmd)
	annotateDevAppTool(cmd, tool)
}

// devAppCall 返回统一派发闭包（替代各命令重复的 Call: runDevAppTool 透传）。
func devAppCall(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		return runDevAppTool(runner, cmd, tool, params)
	}
}

// devAppCallCursor 同上，但先经 devAppApplyCursorParams 注入 cursor/pageSize。
func devAppCallCursor(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		devAppApplyCursorParams(cmd, params)
		return runDevAppTool(runner, cmd, tool, params)
	}
}

// devAppMeta 返回纯收尾 PostMount 闭包（无额外 flag 的命令用）。
func devAppMeta(tool string) func(*cobra.Command) {
	return func(cmd *cobra.Command) { devAppLeafMeta(cmd, tool) }
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
			steps = append(steps, devAppRobotConnectStep(clientID, unifiedAppID))
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

// devAppRobotConnectStep advertises the local-debug connect command. The
// preferred form is `--unified-app-id`, which reuses `dev app credentials get`
// to fetch clientSecret at runtime — the secret never appears in argv, so it
// stays hidden from `ps` / journald / shell history. Only when unifiedAppID is
// unavailable do we fall back to `--robot-client-id`, and even then we point
// the caller at the safe path in doneWhen instead of hardcoding a
// clientSecret placeholder into the command string.
func devAppRobotConnectStep(clientID, unifiedAppID string) map[string]any {
	var command, doneWhen string
	if unifiedAppID != "" {
		command = fmt.Sprintf("dws dev connect --unified-app-id %s --format json", unifiedAppID)
		doneWhen = "本地 Stream 建联成功，进程保持运行；密钥由 credentials get 后台取回，命令行不出现 clientSecret"
	} else {
		if clientID == "" {
			clientID = "<clientId>"
		}
		command = fmt.Sprintf("dws dev connect --robot-client-id %s --format json", clientID)
		doneWhen = "本地 Stream 建联成功；建议改用 --unified-app-id <uappid>，避免 clientSecret 出现在命令行被 ps 看到"
	}
	step := devAppNextStep(devAppStep{
		ID:       "connect_local",
		Command:  command,
		DoneWhen: doneWhen,
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

// 应用定位：写操作统一只用 --unified-app-id；dev app get 额外支持只读 --app-key。
// --name 定位已下线（列表搜索的 --name/--app-key 是过滤参数、不在此列）。
// 写操作与其它 app 作用域命令共用 addDevAppUnifiedIDFlag + requiredDevAppUnifiedID。

func devAppRequireWriteGuard(cmd *cobra.Command, operation string) error {
	if commandDryRun(cmd) || devAppYes(cmd) {
		return nil
	}
	return apperrors.NewValidation(
		fmt.Sprintf("%s 是写操作；加 --dry-run 预览，或确认后加 --yes 执行", operation),
		apperrors.WithReason("confirmation_required"),
		apperrors.WithHint("先确认目标应用及变更影响；用户明确同意后以相同参数追加 --yes"),
		apperrors.WithActions("确认目标应用和变更内容", "获得用户确认后使用 --yes 执行"),
	)
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
