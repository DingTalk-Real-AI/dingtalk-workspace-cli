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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
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
	// Product-level Agent routing Decl (migrated from selection/devapp.json
	// products.devapp). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "devapp",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理钉钉开放平台企业内部应用、成员、权限、机器人、事件与版本",
			UseWhen: []string{
				"请求涉及企业内部应用的查询、创建、配置、成员权限、机器人、事件订阅或版本管理",
			},
			AvoidWhen: []string{
				"开放平台接口文档搜索使用 devdoc；普通钉钉业务数据使用对应产品命令",
			},
		},
	})
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
	cmdutil.MarkGroup(root)

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
	cmdutil.MarkGroup(webapp)
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
	cmdutil.MarkGroup(permission)
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
	cmdutil.MarkGroup(credentials)
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
	cmdutil.MarkGroup(member)
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
	cmdutil.MarkGroup(security)
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
	cmdutil.MarkGroup(robot)
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
	cmdutil.MarkGroup(version)
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
	cmdutil.MarkGroup(event)
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
		newDevAppDisableCommand(runner),
		newDevAppEnableCommand(runner),
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
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "keyword", Usage: "事件搜索关键词，支持按事件码或事件名称模糊匹配", Bind: "keyword", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "list_dev_app_events",
				CanonicalPath:  "dev.list_dev_app_events",
				CLIPath:        "dev app event list",
				PrimaryCLIPath: "dev app event list",
			},
			Description: "查询应用已订阅的事件列表",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "列出或搜索应用可订阅的事件",
				UseWhen:      []string{"需要查事件码、事件名称或当前事件列表时"},
				AvoidWhen:    []string{"订阅或退订事件使用对应写命令"},
				Examples:     []string{`dws dev app event list --unified-app-id <unifiedAppId> --keyword "审批" --page-size 20`},
			},
		},
		Call:      devAppCallCursor(runner),
		PostMount: devAppMetaCursor(devAppEventListTool),
	})
}

func newDevAppEventSubscribeCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "subscribe",
		Short:   "订阅应用事件回调",
		Example: "  dws dev app event subscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Tool:    devAppEventSubscribeTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "event-codes", Usage: "事件码，多个用逗号或分号分隔", Bind: "eventCodes", Trim: true, Required: true, RequiredHint: "--event-codes 为必填", Transform: transformDevAppListParam},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "subscribe_dev_app_events",
				CanonicalPath:  "dev.subscribe_dev_app_events",
				CLIPath:        "dev app event subscribe",
				PrimaryCLIPath: "dev app event subscribe",
			},
			Description: "订阅应用事件回调",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "为应用订阅指定事件码",
				UseWhen:      []string{"已确认事件码并需要新增事件订阅时"},
				AvoidWhen:    []string{"查询事件码或已有订阅时先使用 dev app event list"},
				Examples:     []string{"dws dev app event subscribe --unified-app-id <unifiedAppId> --event-codes bpms_task_change --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppEventSubscribeTool),
	})
}

func newDevAppEventUnsubscribeCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "unsubscribe",
		Short:   "取消订阅应用事件",
		Example: "  dws dev app event unsubscribe --unified-app-id UNIFIED_APP_ID --event-codes bpms_task_change --dry-run --format json",
		Tool:    devAppEventUnsubscribeTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "event-codes", Usage: "事件码，多个用逗号或分号分隔", Bind: "eventCodes", Trim: true, Required: true, RequiredHint: "--event-codes 为必填", Transform: transformDevAppListParam},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "unsubscribe_dev_app_events",
				CanonicalPath:  "dev.unsubscribe_dev_app_events",
				CLIPath:        "dev app event unsubscribe",
				PrimaryCLIPath: "dev app event unsubscribe",
			},
			Description: "取消订阅应用事件",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "取消应用的指定事件订阅",
				UseWhen:      []string{"需要停止接收一个或多个已订阅事件时"},
				AvoidWhen:    []string{"只是查看事件订阅时使用 dev app event list"},
				Examples:     []string{"dws dev app event unsubscribe --unified-app-id <unifiedAppId> --event-codes bpms_task_change --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppEventUnsubscribeTool),
	})
}

func newDevAppListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询开放平台企业内部应用列表",
		Example: "  dws dev app list --name DemoApp --page-size 20 --format json",
		Tool:    devAppListTool,
		Safety:  devAppSafetyRead(),
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
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "list_dev_app",
				CanonicalPath:  "dev.list_dev_app",
				CLIPath:        "dev app list",
				PrimaryCLIPath: "dev app list",
			},
			Description: "查询开放平台企业内部应用列表",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "按条件分页查询开放平台应用",
				UseWhen:      []string{"需要按名称、创建人或应用键筛选应用时"},
				AvoidWhen:    []string{"已经持有明确 unifiedAppId 时使用 dev app get"},
				Examples:     []string{`dws dev app list --name "DemoApp" --page-size 20 --format json`},
			},
		},
		Call:      devAppCallCursor(runner),
		PostMount: devAppMetaCursor(devAppListTool),
	})
}

func newDevAppGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询开放平台企业内部应用详情",
		Example: "  dws dev app get --unified-app-id UNIFIED_APP_ID --format json\n  dws dev app get --app-key APP_KEY --format json",
		Tool:    devAppGetTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（与 --app-key 二选一）", Bind: "unifiedAppId", Trim: true, OmitEmpty: true},
			{Name: "app-key", Usage: "按 appKey/clientId 查询应用详情（与 --unified-app-id 二选一）", Bind: "appKey", Trim: true, OmitEmpty: true},
		},
		// 二选一走 Validate 而非类型化 Constraints：发布 constraints 会改变已
		// 交付的 Schema 契约（merge-base 为 null），本 PR 承诺零契约变更。
		// 声明化发布留给独立的契约变更 PR。
		Validate: func(cmd *cobra.Command, args []string) error {
			if devAppStringFlag(cmd, "unified-app-id") == "" && devAppStringFlag(cmd, "app-key") == "" {
				return apperrors.NewValidation("请传入 --unified-app-id 或 --app-key")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_dev_app",
				CanonicalPath:  "dev.get_dev_app",
				CLIPath:        "dev app get",
				PrimaryCLIPath: "dev app get",
			},
			Description: "查询开放平台企业内部应用详情",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定开放平台应用详情",
				UseWhen:      []string{"已知 unifiedAppId 或 appKey 并需要核对应用配置或状态时"},
				AvoidWhen:    []string{"需要搜索或分页浏览多个应用时使用 dev app list"},
				Examples: []string{
					"dws dev app get --unified-app-id <unifiedAppId> --format json",
					"dws dev app get --app-key <appKey> --format json",
				},
			},
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
		Safety:  devAppSafetyHighWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "name", Usage: "应用名称 (必填)", Bind: "name", Trim: true, Required: true, RequiredHint: "--name 为必填"},
			{Name: "desc", Usage: "应用描述", Bind: "desc", Trim: true, OmitEmpty: true},
			{Name: "icon-media-id", Usage: "应用图标 mediaId", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_dev_app",
				CanonicalPath:  "dev.create_dev_app",
				CLIPath:        "dev app create",
				PrimaryCLIPath: "dev app create",
			},
			Description: "创建开放平台企业内部应用",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建钉钉开放平台应用并返回 unifiedAppId/凭证信息",
				UseWhen:      []string{"需要新建企业内部应用或三方个人应用并拿到 unifiedAppId"},
				AvoidWhen:    []string{"应用已存在只需改信息时用 dev app update", "只查文档时用 devdoc"},
				Examples:     []string{`dws dev app create --name "我的 AI 机器人" --desc "接 opencode" --dry-run --format json`},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppCreateTool),
	})
}

func newDevAppUpdateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "update",
		Short:   "修改开放平台企业内部应用基础信息",
		Long:    "修改开放平台企业内部应用基础信息：名称、描述或图标至少提供一项。",
		Example: "  dws dev app update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --dry-run --format json",
		Tool:    devAppUpdateTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "name", Usage: "新的应用名称", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "desc", Usage: "新的应用描述", Bind: "desc", Trim: true, OmitEmpty: true},
			{Name: "icon-media-id", Usage: "新的应用图标 mediaId", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
		},
		// 至少一项走 Validate 而非类型化 Constraints：零契约变更（见 get 命令注释）。
		Validate: func(cmd *cobra.Command, args []string) error {
			if devAppStringFlag(cmd, "name") == "" && devAppStringFlag(cmd, "desc") == "" &&
				devAppStringFlag(cmd, "icon-media-id") == "" {
				return apperrors.NewValidation("至少提供一项待更新字段：--name、--desc 或 --icon-media-id")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "update_dev_app",
				CanonicalPath:  "dev.update_dev_app",
				CLIPath:        "dev app update",
				PrimaryCLIPath: "dev app update",
			},
			Description: "修改开放平台企业内部应用基础信息",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "更新开放平台应用名称/描述/图标等基础信息",
				UseWhen:      []string{"已有 unifiedAppId，需要修改应用基础信息"},
				AvoidWhen:    []string{"创建新应用用 create；停用/启用用 disable/enable；改安全配置用 security config"},
				Examples:     []string{"dws dev app update --unified-app-id <unifiedAppId> --name <新名称> --dry-run --format json"},
			},
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
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_dev_app_credentials",
				CanonicalPath:  "dev.get_dev_app_credentials",
				CLIPath:        "dev app credentials get",
				PrimaryCLIPath: "dev app credentials get",
			},
			Description: "读取开放平台应用凭证",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "读取指定应用的客户端凭证",
				UseWhen:      []string{"已知 unifiedAppId 且需要 clientId 或 clientSecret 时"},
				AvoidWhen:    []string{"不要把凭证内容用于普通应用详情查询或写入日志"},
				Examples:     []string{"dws dev app credentials get --unified-app-id <unifiedAppId> --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppCredentialsGetTool),
	})
}

func newDevAppDisableCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:    "disable",
		Short:  "停用开放平台企业内部应用",
		Tool:   devAppDisableTool,
		Safety: devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "disable_dev_app",
				CanonicalPath:  "dev.disable_dev_app",
				CLIPath:        "dev app disable",
				PrimaryCLIPath: "dev app disable",
			},
			Description: "停用开放平台企业内部应用",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "停用指定开放平台应用",
				UseWhen:      []string{"需要让应用暂时不可用但保留配置时"},
				AvoidWhen:    []string{"永久删除应用使用 dev app delete"},
				Examples:     []string{"dws dev app disable --unified-app-id <unifiedAppId> --dry-run --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppDisableTool),
	})
}

func newDevAppEnableCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:    "enable",
		Short:  "启用开放平台企业内部应用",
		Tool:   devAppEnableTool,
		Safety: devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "enable_dev_app",
				CanonicalPath:  "dev.enable_dev_app",
				CLIPath:        "dev app enable",
				PrimaryCLIPath: "dev app enable",
			},
			Description: "启用开放平台企业内部应用",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "启用指定开放平台应用",
				UseWhen:      []string{"需要恢复一个已停用应用时"},
				AvoidWhen:    []string{"启用应用内机器人能力使用 dev app robot enable"},
				Examples:     []string{"dws dev app enable --unified-app-id <unifiedAppId> --dry-run --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppEnableTool),
	})
}

// newDevAppDeleteCommand is delete with a danger tier: deleting an app is
// irreversible, so beyond the high-write confirmation it requires
// --confirm-name to match the located app's real name. This guards against
// "located the wrong app and deleted it" — the agent must first know the name
// (via `get`/dry-run) before it can delete. The match is verified client-side
// (a `get` then compare), standard practice for destructive CLI ops
// (gh repo delete, gcloud).
//
// LeafSpec+RunE：confirm-name 的 get-then-compare 多步二次确认属多步编排，
// 由自定义 RunE 承载；框架仍按同一 SafetySpec 执行确认门并发布 Schema。
func newDevAppDeleteCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "delete",
		Short:   "删除开放平台企业内部应用（不可逆，需 --confirm-name 二次确认）",
		Example: "  dws dev app delete --unified-app-id UNIFIED_APP_ID --confirm-name 应用名 --yes --format json",
		Tool:    devAppDeleteTool,
		Safety:  devAppSafetyDestructive(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "confirm-name", Usage: "二次确认：必须与被删应用的名称一致（不可逆操作的防误删）", Bind: "confirmName", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "delete_dev_app",
				CanonicalPath:  "dev.delete_dev_app",
				CLIPath:        "dev app delete",
				PrimaryCLIPath: "dev app delete",
			},
			Description: "删除开放平台企业内部应用（不可逆，需 --confirm-name 二次确认）",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "删除开放平台企业内部应用（不可恢复）",
				UseWhen:      []string{"用户明确要求永久删除应用，且接受不可恢复后果"},
				AvoidWhen:    []string{"只需临时下架时用 dev app disable", "用户未确认应用名/影响范围时不要删除"},
				Examples:     []string{"dws dev app delete --unified-app-id <unified-app-id> --dry-run --format json"},
			},
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
		PostMount: devAppMeta(devAppDeleteTool),
	})
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
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_extension_webapp_config",
				CanonicalPath:  "dev.get_extension_webapp_config",
				CLIPath:        "dev app webapp get",
				PrimaryCLIPath: "dev app webapp get",
			},
			Description: "查询网页应用配置",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定应用的网页入口配置",
				UseWhen:      []string{"需要核对应用 H5 或 PC 首页地址时"},
				AvoidWhen:    []string{"修改网页入口时使用 dev app webapp config"},
				Examples:     []string{"dws dev app webapp get --unified-app-id <unifiedAppId> --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppWebappGetTool),
	})
}

func newDevAppWebappConfigCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "config",
		Short:   "配置网页应用能力",
		Example: "  dws dev app webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com --dry-run --format json",
		Tool:    devAppWebappSetTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "h5-page-type", Usage: "网页应用生效端/页面类型", Bind: "h5PageType", Trim: true, OmitEmpty: true},
			{Name: "homepage-url", Usage: "移动端首页地址", Bind: "homepageUrl", Trim: true, OmitEmpty: true},
			{Name: "pc-homepage-url", Usage: "PC 端首页地址", Bind: "pcHomepageUrl", Trim: true, OmitEmpty: true},
			{Name: "omp-url", Usage: "管理后台地址", Bind: "ompUrl", Trim: true, OmitEmpty: true},
		},
		// 至少一项走 Validate 而非类型化 Constraints：零契约变更（见 get 命令注释）。
		Validate: func(cmd *cobra.Command, args []string) error {
			if devAppStringFlag(cmd, "h5-page-type") == "" && devAppStringFlag(cmd, "homepage-url") == "" &&
				devAppStringFlag(cmd, "pc-homepage-url") == "" && devAppStringFlag(cmd, "omp-url") == "" {
				return apperrors.NewValidation("至少提供一项网页应用配置：--h5-page-type、--homepage-url、--pc-homepage-url 或 --omp-url")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "set_extension_webapp_config",
				CanonicalPath:  "dev.set_extension_webapp_config",
				CLIPath:        "dev app webapp config",
				PrimaryCLIPath: "dev app webapp config",
			},
			Description: "配置网页应用能力",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建或更新应用网页入口配置",
				UseWhen:      []string{"需要设置 H5、PC 首页或 OMP 地址时"},
				AvoidWhen:    []string{"只查看当前网页配置时使用 dev app webapp get"},
				Examples:     []string{"dws dev app webapp config --unified-app-id <unifiedAppId> --homepage-url https://example.com --dry-run"},
			},
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
		Safety:  devAppSafetyRead(),
		// 命令级别名 "search" 由 PostMount 设回（LeafSpec 无 Command Aliases 字段）。
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "keyword", Usage: "权限名、权限点、接口名关键词", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "scope-value", Usage: "精确权限点 scopeValue", Bind: "scopeValue", Trim: true, OmitEmpty: true},
			{Name: "auth-status", Usage: "权限状态：ALL、AUTHED、UNAUTHED", Default: "ALL", Bind: "authStatus", Trim: true, OmitEmpty: true, Transform: func(raw string) (any, error) { return strings.ToUpper(raw), nil }},
			{Name: "scope-type", Usage: "权限一级类型：APP 或 SNS", Bind: "scopeType", Trim: true, OmitEmpty: true, Transform: func(raw string) (any, error) { return strings.ToUpper(raw), nil }},
			{Name: "api-status", Usage: "开发者后台 apiStatus 过滤", Bind: "apiStatus", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "list_dev_app_permissions",
				CanonicalPath:  "dev.list_dev_app_permissions",
				CLIPath:        "dev app permission list",
				PrimaryCLIPath: "dev app permission list",
			},
			Description: "查询开放平台应用权限列表",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询应用权限及其授权状态",
				UseWhen:      []string{"需要按关键词、范围或状态查权限时"},
				AvoidWhen:    []string{"新增或移除权限使用对应 permission 写命令"},
				Examples:     []string{`dws dev app permission list --unified-app-id <unifiedAppId> --keyword "通讯录" --page-size 20`},
			},
		},
		Call: devAppCallCursor(runner),
		PostMount: func(cmd *cobra.Command) {
			cmd.Aliases = []string{"search"}
			devAppMetaCursor(devAppPermissionListTool)(cmd)
		},
	})
}

func newDevAppPermissionAddCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "add",
		Short:   "申请开放平台应用权限点",
		Example: "  dws dev app permission add --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile --dry-run --format json",
		Tool:    devAppPermissionAddTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "scope-values", Usage: "权限点 scopeValue，多个用逗号或分号分隔", Bind: "scopeValues", Trim: true, Required: true, RequiredHint: "--scope-values 为必填", Transform: transformDevAppScopeValues},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "apply_dev_app_permissions",
				CanonicalPath:  "dev.apply_dev_app_permissions",
				CLIPath:        "dev app permission add",
				PrimaryCLIPath: "dev app permission add",
			},
			Description: "申请开放平台应用权限点",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "为应用申请一个或多个接口权限",
				UseWhen:      []string{"已确认 scopeValue 并需要新增权限时"},
				AvoidWhen:    []string{"查找或核对已有权限时使用 dev app permission list"},
				Examples:     []string{"dws dev app permission add --unified-app-id <unifiedAppId> --scope-values Contact.User.mobile --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppPermissionAddTool),
	})
}

func newDevAppPermissionRemoveCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "remove",
		Short:   "取消开放平台应用权限点",
		Example: "  dws dev app permission remove --unified-app-id UNIFIED_APP_ID --scope-values Contact.User.mobile,qyapi_robot_sendmsg --dry-run --format json",
		Tool:    devAppPermissionRmTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "scope-values", Usage: "待取消权限点 scopeValue，多个用逗号或分号分隔", Bind: "scopeValues", Trim: true, Required: true, RequiredHint: "--scope-values 为必填", Transform: transformDevAppScopeValues},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "remove_dev_app_permissions",
				CanonicalPath:  "dev.remove_dev_app_permissions",
				CLIPath:        "dev app permission remove",
				PrimaryCLIPath: "dev app permission remove",
			},
			Description: "取消开放平台应用权限点",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "移除应用的一个或多个接口权限",
				UseWhen:      []string{"需要撤销已知 scopeValue 的权限时"},
				AvoidWhen:    []string{"只查看权限状态时使用 dev app permission list"},
				Examples:     []string{"dws dev app permission remove --unified-app-id <unifiedAppId> --scope-values Contact.User.mobile --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppPermissionRmTool),
	})
}

func newDevAppMemberListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list",
		Short:   "查询开放平台应用成员",
		Example: "  dws dev app member list --unified-app-id <unifiedAppId>",
		Tool:    devAppMemberListTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "list_dev_app_members",
				CanonicalPath:  "dev.list_dev_app_members",
				CLIPath:        "dev app member list",
				PrimaryCLIPath: "dev app member list",
			},
			Description: "查询开放平台应用成员",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定应用的协作成员",
				UseWhen:      []string{"需要查看应用开发者和管理员清单时"},
				AvoidWhen:    []string{"新增或移除成员使用对应 member 写命令"},
				Examples:     []string{"dws dev app member list --unified-app-id <unifiedAppId>"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppMemberListTool),
	})
}

func newDevAppMemberAddCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "add",
		Short:   "添加开放平台应用成员",
		Example: "  dws dev app member add --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Tool:    devAppMemberAddTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "user-ids", Usage: "成员 userId 列表，多个用逗号分隔 (必填)", Bind: "userIds", Trim: true, Required: true, RequiredHint: "--user-ids 为必填", Aliases: []string{"member-user-ids"}, Transform: transformDevAppListParam},
			{Name: "member-type", Usage: "成员类型，如 DEVELOPER (必填)", Bind: "memberType", Trim: true, Required: true, RequiredHint: "--member-type 为必填"},
		},
		// 纯分隔符输入（如 ","）通过 Required 但解析为空：保持旧版「至少包含一个」
		// 拦截。member-user-ids 是 user-ids 的注册别名，需一并检查。
		Validate: func(cmd *cobra.Command, args []string) error {
			if len(parseDevAppListFlag(cmd, "user-ids")) == 0 && len(parseDevAppListFlag(cmd, "member-user-ids")) == 0 {
				return apperrors.NewValidation("--user-ids 至少包含一个 userId")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "add_dev_app_members",
				CanonicalPath:  "dev.add_dev_app_members",
				CLIPath:        "dev app member add",
				PrimaryCLIPath: "dev app member add",
			},
			Description: "添加开放平台应用成员",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "向应用添加开发者或管理员成员",
				UseWhen:      []string{"需要授予指定用户应用协作角色时"},
				AvoidWhen:    []string{"只查看现有成员时使用 dev app member list"},
				Examples:     []string{"dws dev app member add --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppMemberAddTool),
	})
}

func newDevAppMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "remove",
		Short:   "移除开放平台应用成员",
		Example: "  dws dev app member remove --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run",
		Tool:    devAppMemberRemoveTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "user-ids", Usage: "成员 userId 列表，多个用逗号分隔 (必填)", Bind: "userIds", Trim: true, Required: true, RequiredHint: "--user-ids 为必填", Aliases: []string{"member-user-ids"}, Transform: transformDevAppListParam},
			{Name: "member-type", Usage: "成员类型，如 DEVELOPER (必填)", Bind: "memberType", Trim: true, Required: true, RequiredHint: "--member-type 为必填"},
		},
		// 纯分隔符输入（如 ","）通过 Required 但解析为空：保持旧版「至少包含一个」
		// 拦截。member-user-ids 是 user-ids 的注册别名，需一并检查。
		Validate: func(cmd *cobra.Command, args []string) error {
			if len(parseDevAppListFlag(cmd, "user-ids")) == 0 && len(parseDevAppListFlag(cmd, "member-user-ids")) == 0 {
				return apperrors.NewValidation("--user-ids 至少包含一个 userId")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "remove_dev_app_members",
				CanonicalPath:  "dev.remove_dev_app_members",
				CLIPath:        "dev app member remove",
				PrimaryCLIPath: "dev app member remove",
			},
			Description: "移除开放平台应用成员",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "从应用移除开发者或管理员成员",
				UseWhen:      []string{"需要撤销指定用户的应用协作角色时"},
				AvoidWhen:    []string{"删除应用本身或只查看成员时不要使用"},
				Examples:     []string{"dws dev app member remove --unified-app-id <unifiedAppId> --user-ids userId1,userId2 --member-type DEVELOPER --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppMemberRemoveTool),
	})
}

func newDevAppSecurityConfigCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "config",
		Short: "更新开放平台应用安全配置",
		Example: "  dws dev app security config --unified-app-id <unifiedAppId> " +
			"--ip-whitelist 192.0.2.10 --redirect-urls https://callback.example.invalid/callback --sso-urls https://sso.example.invalid/sso --dry-run",
		Tool:   devAppSecurityConfigTool,
		Safety: devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "ip-whitelist", Usage: "出口 IP 白名单，多个用逗号或分号分隔（整组覆盖，非追加）", Bind: "ipWhitelist", Trim: true, Transform: transformDevAppListParam},
			{Name: "redirect-urls", Usage: "登录重定向 URL，多个用逗号或分号分隔（整组覆盖，非追加）", Bind: "redirectUrls", Trim: true, Transform: transformDevAppListParam},
			{Name: "sso-urls", Usage: "端内免登地址，多个用逗号或分号分隔（整组覆盖，非追加）", Bind: "ssoUrls", Trim: true, Transform: transformDevAppListParam},
		},
		// 至少一项走 Validate 而非类型化 Constraints：零契约变更（见 get 命令注释）。
		Validate: func(cmd *cobra.Command, args []string) error {
			if len(parseDevAppListFlag(cmd, "ip-whitelist")) == 0 &&
				len(parseDevAppListFlag(cmd, "redirect-urls")) == 0 &&
				len(parseDevAppListFlag(cmd, "sso-urls")) == 0 {
				return apperrors.NewValidation("至少提供一项安全配置：--ip-whitelist、--redirect-urls 或 --sso-urls")
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "update_dev_app_security_config",
				CanonicalPath:  "dev.update_dev_app_security_config",
				CLIPath:        "dev app security config",
				PrimaryCLIPath: "dev app security config",
			},
			Description: "更新开放平台应用安全配置",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "更新应用 IP 白名单、回调和单点登录地址",
				UseWhen:      []string{"需要调整应用安全相关 URL 或白名单时"},
				AvoidWhen:    []string{"普通应用名称和描述更新使用 dev app update"},
				Examples:     []string{"dws dev app security config --unified-app-id <unifiedAppId> --ip-whitelist 192.0.2.10 --redirect-urls https://callback.example.invalid/callback --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppSecurityConfigTool),
	})
}

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

// LeafSpec+RunE：devAppRobotCreateParams 的 icon/preview 空串占位与失败重试
// taskId 编排由自定义 RunE 承载；框架按 SafetySpec 执行确认门，必填仍由
// devAppRobotCreateParams 报错，Required 标记仅用于 Schema/help 投影。
func newDevAppRobotSubmitCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "submit",
		Short:   "异步提交钉钉智能体机器人创建任务（支持失败重试）",
		Example: "  dws dev app robot submit --name 我的智能体 --robot-name 小助手 --desc \"处理审批问答\" --dry-run --format json",
		Tool:    devAppRobotSubmitTool,
		Safety:  devAppSafetyHighWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "name", Usage: "智能体应用名称，长度 2-20，企业内唯一 (必填)", Bind: "name", Trim: true, Required: true, RequiredHint: "--name 为必填"},
			{Name: "robot-name", Usage: "承载机器人名称，用于客户端展示 (必填)", Bind: "robotName", Trim: true, Required: true, RequiredHint: "--robot-name 为必填"},
			{Name: "desc", Usage: "机器人功能描述，不超过 200 字 (必填)", Bind: "desc", Trim: true, Required: true, RequiredHint: "--desc 为必填"},
			{Name: "icon-media-id", Usage: "机器人图标 mediaId；为空时使用默认图标", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
			{Name: "preview-media-id", Usage: "机器人预览图 mediaId；为空时复用图标", Bind: "previewMediaId", Trim: true, OmitEmpty: true},
			{Name: "task-id", Usage: "失败重试时传入原 taskId；为空时服务端自动生成", Bind: "taskId", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "submit_robot_create_task",
				CanonicalPath:  "dev.submit_robot_create_task",
				CLIPath:        "dev app robot submit",
				PrimaryCLIPath: "dev app robot submit",
			},
			Description: "异步提交钉钉智能体机器人创建任务（支持失败重试）",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "提交机器人建号异步任务",
				UseWhen:      []string{"需要创建新的机器人账号并取得 taskId 时"},
				AvoidWhen:    []string{"已有任务应先用 robot result 轮询，不能重复提交"},
				Examples:     []string{`dws dev app robot submit --name "我的智能体" --robot-name "小助手" --desc "处理审批问答" --dry-run`},
			},
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
		PostMount: devAppMeta(devAppRobotSubmitTool),
	})
}

func newDevAppRobotResultCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "result",
		Short:   "查询机器人异步创建任务结果",
		Example: "  dws dev app robot result --task-id TASK_ID --format json",
		Tool:    devAppRobotResultTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "task-id", Usage: "提交创建任务时返回的 taskId (必填)", Bind: "taskId", Trim: true, Required: true, RequiredHint: "--task-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "query_robot_create_result",
				CanonicalPath:  "dev.query_robot_create_result",
				CLIPath:        "dev app robot result",
				PrimaryCLIPath: "dev app robot result",
			},
			Description: "查询机器人异步创建任务结果",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询机器人建号异步任务结果",
				UseWhen:      []string{"已有 taskId，需要轮询 WAITING、SUCCESS 或审批状态时"},
				AvoidWhen:    []string{"没有建号任务时先使用 dev app robot submit"},
				Examples:     []string{"dws dev app robot result --task-id <taskId> --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppRobotResultTool),
	})
}

func newDevAppRobotConfigGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询现有应用的机器人配置",
		Example: "  dws dev app robot get --unified-app-id UNIFIED_APP_ID --format json",
		Tool:    devAppRobotConfigGetTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_extension_robot_config",
				CanonicalPath:  "dev.get_extension_robot_config",
				CLIPath:        "dev app robot get",
				PrimaryCLIPath: "dev app robot get",
			},
			Description: "查询现有应用的机器人配置",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定应用的机器人配置和状态",
				UseWhen:      []string{"需要判断机器人是否未配置、离线或在线时"},
				AvoidWhen:    []string{"查询机器人建号异步任务使用 dev app robot result"},
				Examples:     []string{"dws dev app robot get --unified-app-id <unifiedAppId> --format json"},
			},
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
// LeafSpec+RunE：devAppRobotConfigParams 的 mode enum 校验、Bool Changed 语义、
// skills 列表、i18n JSON 解析与至少一项计数由自定义 RunE 承载；框架按
// SafetySpec 执行确认门并发布 Schema。
func newDevAppRobotConfigCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "config",
		Short:   "创建或更新现有应用的机器人配置（upsert）",
		Example: "  dws dev app robot config --unified-app-id UNIFIED_APP_ID --name 小助手 --brief 审批助手 --dry-run --format json",
		Tool:    devAppRobotConfigUpsertTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "name", Usage: "机器人名称", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "brief", Usage: "机器人简介", Bind: "brief", Trim: true, OmitEmpty: true},
			{Name: "desc", Usage: "机器人描述", Bind: "desc", Trim: true, OmitEmpty: true},
			{Name: "icon-media-id", Usage: "机器人图标 mediaId", Bind: "iconMediaId", Trim: true, OmitEmpty: true},
			{Name: "outgoing-url", Usage: "消息回调地址", Bind: "outgoingUrl", Trim: true, OmitEmpty: true},
			{Name: "event-callback-url", Usage: "事件回调地址", Bind: "eventCallbackUrl", Trim: true, OmitEmpty: true},
			{Name: "mode", Usage: "机器人模式：HTTPS / STREAM / AISKILL", Bind: "mode", Trim: true, OmitEmpty: true},
			{Name: "skills", Usage: "技能列表，多个用逗号或分号分隔", Bind: "skills", Trim: true, OmitEmpty: true},
			{Name: "add-scope", Usage: "是否自动添加机器人相关权限", Kind: LeafBool, Bind: "addScope", OmitEmpty: true},
			{Name: "disable-ssl-verify", Usage: "回调地址是否关闭 SSL 校验", Kind: LeafBool, Bind: "disableSSLVerify", OmitEmpty: true},
			{Name: "i18n-name", Usage: "机器人名称国际化 JSON，如 '{\"en_US\":\"Bot\"}'", Bind: "i18nName", Trim: true, OmitEmpty: true},
			{Name: "i18n-brief", Usage: "机器人简介国际化 JSON", Bind: "i18nBrief", Trim: true, OmitEmpty: true},
			{Name: "i18n-description", Usage: "机器人描述国际化 JSON", Bind: "i18nDescription", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "set_extension_robot_config",
				CanonicalPath:  "dev.set_extension_robot_config",
				CLIPath:        "dev app robot config",
				PrimaryCLIPath: "dev app robot config",
			},
			Description: "创建或更新现有应用的机器人配置（upsert）",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "创建或更新应用的机器人能力配置",
				UseWhen:      []string{"已有 unifiedAppId 并需要配置机器人名称、回调或技能时"},
				AvoidWhen:    []string{"机器人建号使用 robot submit，本地建联使用 dev connect"},
				Examples:     []string{`dws dev app robot config --unified-app-id <unifiedAppId> --name "小助手" --brief "审批助手" --dry-run`},
			},
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
		PostMount: devAppMeta(devAppRobotConfigUpsertTool),
	})
}

// newDevAppRobotEnableCommand enables an app's robot capability. Unlike config,
// it needs no config fields — pure enable, only the app locator.
func newDevAppRobotEnableCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "enable",
		Short:   "启用现有应用机器人能力（纯启用，无需配置字段）",
		Example: "  dws dev app robot enable --unified-app-id UNIFIED_APP_ID --dry-run --format json",
		Tool:    devAppRobotEnableTool,
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "enable_dev_app_robot",
				CanonicalPath:  "dev.enable_dev_app_robot",
				CLIPath:        "dev app robot enable",
				PrimaryCLIPath: "dev app robot enable",
			},
			Description: "启用现有应用机器人能力（纯启用，无需配置字段）",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "启用指定应用的机器人能力",
				UseWhen:      []string{"机器人配置存在但处于 OFFLINE 时"},
				AvoidWhen:    []string{"建立本地 Stream 连接使用 dev connect"},
				Examples:     []string{"dws dev app robot enable --unified-app-id <unifiedAppId> --dry-run"},
			},
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
		Safety:  devAppSafetyWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "disable_dev_app_robot",
				CanonicalPath:  "dev.disable_dev_app_robot",
				CLIPath:        "dev app robot disable",
				PrimaryCLIPath: "dev app robot disable",
			},
			Description: "停用现有应用的机器人能力",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "停用指定应用的机器人能力",
				UseWhen:      []string{"需要暂时让应用机器人离线时"},
				AvoidWhen:    []string{"停用整个应用使用 dev app disable"},
				Examples:     []string{"dws dev app robot disable --unified-app-id <unifiedAppId> --dry-run"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppRobotOfflineTool),
	})
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
		Safety:  devAppSafetyHighWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "version", Usage: "高级可选：显式版本号，如 1.0.1；默认不传，由服务端基于最新已发布版本自动递增", Bind: "version", Trim: true, OmitEmpty: true},
			{Name: "desc", Usage: "版本描述", Bind: "desc", Trim: true, OmitEmpty: true},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "create_dev_app_version",
				CanonicalPath:  "dev.create_dev_app_version",
				CLIPath:        "dev app version create",
				PrimaryCLIPath: "dev app version create",
			},
			Description: "基于当前配置创建应用新版本",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "为应用当前配置创建待发布版本",
				UseWhen:      []string{"配置变更完成后需要生成版本进入发布流程时"},
				AvoidWhen:    []string{"只是查看已有版本时使用 dev app version list"},
				Examples:     []string{`dws dev app version create --unified-app-id <unifiedAppId> --desc "新增机器人能力" --dry-run`},
			},
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
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "list_dev_app_versions",
				CanonicalPath:  "dev.list_dev_app_versions",
				CLIPath:        "dev app version list",
				PrimaryCLIPath: "dev app version list",
			},
			Description: "分页查询应用版本列表",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "分页列出应用的历史和待发布版本",
				UseWhen:      []string{"需要查找 versionId 或浏览版本记录时"},
				AvoidWhen:    []string{"已知版本并需详情或状态时使用 get 或 status"},
				Examples:     []string{"dws dev app version list --unified-app-id <unifiedAppId> --page-size 20"},
			},
		},
		Call:      devAppCallCursor(runner),
		PostMount: devAppMetaCursor(devAppVersionListTool),
	})
}

func newDevAppVersionGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "get",
		Short:   "查询指定版本详情",
		Example: "  dws dev app version get --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Tool:    devAppVersionDetailTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true, Required: true, RequiredHint: "--version-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_dev_app_version_detail",
				CanonicalPath:  "dev.get_dev_app_version_detail",
				CLIPath:        "dev app version get",
				PrimaryCLIPath: "dev app version get",
			},
			Description: "查询指定版本详情",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定应用版本的详细内容",
				UseWhen:      []string{"已知 versionId 并需要核对版本配置时"},
				AvoidWhen:    []string{"查看发布进度时使用 dev app version status"},
				Examples:     []string{"dws dev app version get --unified-app-id <unifiedAppId> --version-id <versionId>"},
			},
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
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true, Required: true, RequiredHint: "--version-id 为必填"},
		},
		ConstParams: map[string]any{"precheckOnly": true},
		Call:        devAppCall(runner),
		PostMount:   devAppMeta(devAppVersionPublishTool),
	})
}

func newDevAppVersionPublishCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "publish",
		Short:   "发布指定版本（含高敏权限需 --confirmed-sensitive）",
		Example: "  dws dev app version publish --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --dry-run --format json",
		Tool:    devAppVersionPublishTool,
		Safety:  devAppSafetyHighWrite(),
		// devapp 旧版写守卫为 guard-first：确认门先于参数校验。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true, Required: true, RequiredHint: "--version-id 为必填"},
			{Name: "approver-user-id", Usage: "灰度选人模式下指定审批人 userId", Bind: "approverUserId", Trim: true, OmitEmpty: true},
			{Name: "confirmed-sensitive", Usage: "确认发布包含高敏权限的版本", Kind: LeafBool, Bind: "confirmedSensitive"},
		},
		ConstParams: map[string]any{"precheckOnly": false},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "publish_dev_app_version",
				CanonicalPath:  "dev.publish_dev_app_version",
				CLIPath:        "dev app version publish",
				PrimaryCLIPath: "dev app version publish",
			},
			Description: "发布指定版本（含高敏权限需 --confirmed-sensitive）",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "发布开放平台应用指定版本（可先预检）",
				UseWhen:      []string{"版本已创建，需要预检或正式发布到线上"},
				AvoidWhen:    []string{"还没有 versionId 时先 version create / list / get", "含高敏权限但尚未确认时不要正式发布"},
				Examples:     []string{"dws dev app version publish --unified-app-id <unifiedAppId> --version-id <versionId> --dry-run --format json"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppVersionPublishTool),
	})
}

func newDevAppVersionStatusCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "status",
		Short:   "查询版本发布/审批状态",
		Example: "  dws dev app version status --unified-app-id UNIFIED_APP_ID --version-id VERSION_ID --format json",
		Tool:    devAppVersionStatusTool,
		Safety:  devAppSafetyRead(),
		Flags: []LeafFlag{
			{Name: "unified-app-id", Usage: "开放平台统一应用 ID（必填）", Bind: "unifiedAppId", Trim: true, Required: true, RequiredHint: "--unified-app-id 为必填"},
			{Name: "version-id", Usage: "版本 ID (必填)", Bind: "versionId", Trim: true, Required: true, RequiredHint: "--version-id 为必填"},
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "get_dev_app_version_status",
				CanonicalPath:  "dev.get_dev_app_version_status",
				CLIPath:        "dev app version status",
				PrimaryCLIPath: "dev app version status",
			},
			Description: "查询版本发布/审批状态",
			DryRun:      devAppDryRun,
			Interface:   devAppCompositeInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定应用版本的发布或审批状态",
				UseWhen:      []string{"需要判断版本是否已发布、审核中或受阻时"},
				AvoidWhen:    []string{"需要版本配置详情时使用 dev app version get"},
				Examples:     []string{"dws dev app version status --unified-app-id <unifiedAppId> --version-id <versionId>"},
			},
		},
		Call:      devAppCall(runner),
		PostMount: devAppMeta(devAppVersionStatusTool),
	})
}

func requiredDevAppUnifiedID(cmd *cobra.Command) (string, error) {
	appID := devAppStringFlag(cmd, "unified-app-id")
	if appID == "" {
		return "", apperrors.NewValidation("--unified-app-id 为必填")
	}
	return appID, nil
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
// 参数装配由 Flags/ConstParams 完成；本闭包只负责执行与响应处理。
func devAppCall(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		return runDevAppTool(runner, cmd, tool, params)
	}
}

// devAppMeta 返回纯收尾 PostMount 闭包（无额外 flag 的命令用）。
func devAppMeta(tool string) func(*cobra.Command) {
	return func(cmd *cobra.Command) { devAppLeafMeta(cmd, tool) }
}

// devAppCompositeInterfaceReason 是 devapp 全树共用的评审 interface 说明（非 pin
// MCP 元数据的远程适配器），作为 contract.InterfaceSpec.Reason 的最终发布值。
const devAppCompositeInterfaceReason = "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command."

// devapp 的 SafetySpec 直接对齐 Agent Runtime Schema。函数每次返回一个值
// 副本，避免共享可变状态；四个字段彼此独立，不从 effect 或 risk 推导。
func devAppSafetyRead() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func devAppSafetyWrite() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func devAppSafetyHighWrite() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func devAppSafetyDestructive() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

// devAppDryRun 是 devapp 全树共用的 dry_run 最终声明：本地拼装调用预览
// （invocation preview），dry-run 不发起任何远端读。
var devAppDryRun = &contract.DryRunSpec{
	PreviewKind: "invocation",
	RemoteReads: false,
}

// devAppCompositeInterface 是 devapp 全树共用的 Interface 最终声明。
func devAppCompositeInterface() *contract.InterfaceSpec {
	return &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: devAppCompositeInterfaceReason}
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

// transformDevAppListParam splits a comma/semicolon list for LeafFlag.Transform.
// Empty input (including separator-only) returns nil so optional flags omit the
// key from toolArgs. Required flags that collapse to empty are rejected by
// corecmd.BuildArgs after transform.
func transformDevAppListParam(raw string) (any, error) {
	values := splitDevAppList(raw)
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

// transformDevAppScopeValues preserves the double-split semantics (each
// comma-separated token may itself be a list).
func transformDevAppScopeValues(raw string) (any, error) {
	values := splitDevAppList(raw)
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range splitDevAppList(value) {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// 应用定位：写操作统一只用 --unified-app-id；dev app get 额外支持只读 --app-key。
// --name 定位已下线（列表搜索的 --name/--app-key 是过滤参数、不在此列）。
// 写叶确认门已声明化为 SafetySpec.Confirmation=user_required。

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
