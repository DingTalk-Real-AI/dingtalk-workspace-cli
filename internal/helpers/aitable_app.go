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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const aitableAppMCPServer = "aitable"

func newAitableAppCommand() *cobra.Command {
	appCmd := newGroupCommand(&cobra.Command{
		Use:   "app",
		Short: "应用模式管理",
		Long:  "管理当前 Base 唯一面向用户的应用模式 App、页面和页面 Widget。",
		RunE:  groupRunE,
	})
	pageCmd := newGroupCommand(&cobra.Command{Use: "page", Short: "应用页面管理", RunE: groupRunE})
	widgetCmd := newGroupCommand(&cobra.Command{Use: "widget", Short: "应用页面组件管理", RunE: groupRunE})

	appGetCmd := NewLeafCommand(LeafSpec{
		Use:   "get",
		Short: "获取应用信息",
		Long: `获取指定 Base 唯一面向用户的应用模式 App 及页面摘要。
如果底层尚无 App，本次调用会幂等创建默认 App，并在结果中返回 created=true；因此该命令不是纯读操作。
返回的 app.appId 是构造应用模式在线访问 URL 的编码标识，不是内部数字主键。`,
		Example:       "  dws aitable app get --base-id BASE_ID",
		Tool:          "get_app",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppConditionalWriteSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
		},
		Contract: aitableAppContract(
			"app_get", "aitable app get", "get_app",
			"获取 Base 唯一应用模式 App 及页面摘要；不存在时幂等创建默认 App。",
			"获取应用模式 App、appId 和页面摘要；若尚无 App，会先创建默认 App。",
			[]string{"需要定位 Base 的应用模式 App、构造应用 URL或取得页面摘要时；接受无 App 时自动初始化默认 App"},
			[]string{"只需普通 Base 信息时用 base get；开放平台应用管理不属于 AITable AppMode"},
			[]string{"dws aitable app get --base-id <BASE_ID>"},
			[]contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true)},
			aitableAppGetResultSpec(),
		),
	})

	appUpdateCmd := NewLeafCommand(LeafSpec{
		Use:   "update",
		Short: "更新应用外观配置",
		Long: `更新指定 Base 唯一 App 的名称、外观、图标、导航布局或主题。
至少提供一个待更新字段，未提供的字段保持不变；--icon 是完整 JSON 对象并整体替换原图标。
如果底层尚无 App，服务端会先幂等创建默认 App，再应用本次更新。主题类字符串按 MCP 原样透传。`,
		Example:       "  dws aitable app update --base-id BASE_ID --name \"销售应用\"",
		Tool:          "update_app",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppIdempotentWriteSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			{Name: "name", Usage: "新的 App 名称", Bind: "name", Trim: true, OmitEmpty: true},
			{Name: "appearance", Usage: "新的外观模式，按 MCP 枚举原样传递", Bind: "appearance", Trim: true, OmitEmpty: true},
			aitableAppJSONObjectFlag("icon", "icon", "新的 App 图标 JSON 对象，整体替换且 type 必填", false, validateAitableAppTypedObject("icon")),
			{Name: "nav-theme-type", Usage: "新的导航主题类型，按 MCP 枚举原样传递", Bind: "navThemeType", Trim: true, OmitEmpty: true},
			{Name: "navigation-layout", Usage: "新的导航布局，按 MCP 枚举原样传递", Bind: "navigationLayout", Trim: true, OmitEmpty: true},
			{Name: "theme-type", Usage: "新的主题类型，按 MCP 枚举原样传递", Bind: "themeType", Trim: true, OmitEmpty: true},
		},
		Constraints: []LeafConstraint{{
			Kind:        corecmd.AtLeastOne,
			Flags:       []string{"name", "appearance", "icon", "nav-theme-type", "navigation-layout", "theme-type"},
			Description: "--name、--appearance、--icon、--nav-theme-type、--navigation-layout、--theme-type 至少提供一个",
		}},
		Contract: aitableAppContract(
			"app_update", "aitable app update", "update_app",
			"更新 Base 唯一应用模式 App 的外观配置；不存在时幂等创建默认 App。",
			"更新应用模式 App 的名称、图标、外观、导航或主题。",
			[]string{"需要修改应用模式 App 外观配置时；无 App 时允许服务端自动初始化"},
			[]string{"页面元数据用 app page update；Widget 配置用 app widget update"},
			[]string{"dws aitable app update --base-id <BASE_ID> --name \"销售应用\""},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("name", "name", "string", false),
				aitableAppParam("appearance", "appearance", "string", false),
				aitableAppParam("icon", "icon", "object", false),
				aitableAppParam("nav-theme-type", "navThemeType", "string", false),
				aitableAppParam("navigation-layout", "navigationLayout", "string", false),
				aitableAppParam("theme-type", "themeType", "string", false),
			},
			aitableAppStateResultSpec(),
		),
	})

	pageCreateCmd := NewLeafCommand(LeafSpec{
		Use:   "create",
		Short: "创建应用页面",
		Long: `在指定 Base 的应用模式 App 中创建一个仪表盘页面，并同时创建同 ID 的 Dashboard。
如果底层尚无 App，服务端会先幂等创建默认 App。一个 App 最多包含 100 个 Page。
--before-page-id 指定时插入到该页面之前，省略时追加到末尾；--icon 和 --background 是 JSON 对象。`,
		Example:       "  dws aitable app page create --base-id BASE_ID --name \"经营总览\"",
		Tool:          "create_app_page",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppCreateSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			{Name: "name", Usage: "页面名称 (必填)", Bind: "name", Trim: true, Required: true},
			{Name: "before-page-id", Usage: "插入到此页面之前；省略或空值表示追加到末尾", Bind: "beforePageId", Trim: true, OmitEmpty: true},
			aitableAppJSONObjectFlag("icon", "icon", "页面图标 JSON 对象，type 必填", false, validateAitableAppTypedObject("icon")),
			aitableAppJSONObjectFlag("background", "background", "页面背景 JSON 对象，type 必填", false, validateAitableAppTypedObject("background")),
		},
		Contract: aitableAppContract(
			"app_page_create", "aitable app page create", "create_app_page",
			"在应用模式 App 中创建仪表盘页面，并返回新 pageId。",
			"在应用模式 App 中创建顶级仪表盘页面。",
			[]string{"需要新增应用页面，已确认目标 Base 和唯一页面名称时"},
			[]string{"普通 Dashboard 创建用 dashboard create；AI Page JSON 页面属于另一套能力"},
			[]string{"dws aitable app page create --base-id <BASE_ID> --name \"经营总览\""},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("name", "name", "string", true),
				aitableAppParam("before-page-id", "beforePageId", "string", false),
				aitableAppParam("icon", "icon", "object", false),
				aitableAppParam("background", "background", "object", false),
			},
			aitableAppPageResultSpec(),
		),
	})

	pageGetCmd := NewLeafCommand(LeafSpec{
		Use:           "get",
		Short:         "获取应用页面详情",
		Long:          "获取指定应用模式仪表盘页面及其 Widget 摘要。pageId 同时也是对应 Dashboard 的 ID；当前只支持底层 type=page 页面。",
		Example:       "  dws aitable app page get --base-id BASE_ID --page-id PAGE_ID",
		Tool:          "get_app_page",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableSafetyRead(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag(), aitableAppPageIDFlag()},
		Contract: aitableAppContract(
			"app_page_get", "aitable app page get", "get_app_page",
			"获取应用模式页面详情及 Widget 摘要。",
			"获取已知 pageId 的应用页面元数据和 Widget 摘要。",
			[]string{"已知应用模式 pageId，需要查看页面详情或发现 Widget 时"},
			[]string{"列出所有页面用 app page list；普通 Dashboard 用 dashboard get"},
			[]string{"dws aitable app page get --base-id <BASE_ID> --page-id <PAGE_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
			},
			aitableAppPageResultSpec(),
		),
	})

	pageListCmd := NewLeafCommand(LeafSpec{
		Use:   "list",
		Short: "列出应用页面",
		Long: `一次性列出指定 Base 唯一 App 中的全部支持页面，按导航显示顺序返回，最多 100 个。
如果底层尚无 App，本次调用会幂等创建默认空 App；该命令不返回 appId，需要 appId 时调用 app get。`,
		Example:       "  dws aitable app page list --base-id BASE_ID",
		Tool:          "list_app_pages",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppConditionalWriteSafety(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag()},
		Contract: aitableAppContract(
			"app_page_list", "aitable app page list", "list_app_pages",
			"列出应用模式 App 页面；不存在 App 时幂等创建默认空 App。",
			"按导航顺序列出应用页面；无 App 时会初始化默认空 App。",
			[]string{"需要发现应用模式 pageId 或核对页面导航顺序时；接受无 App 时自动初始化"},
			[]string{"已知 pageId 查详情用 app page get；普通 Dashboard 目录用 base get"},
			[]string{"dws aitable app page list --base-id <BASE_ID>"},
			[]contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true)},
			aitableAppPageListResultSpec(),
		),
	})

	pageUpdateCmd := NewLeafCommand(LeafSpec{
		Use:   "update",
		Short: "更新应用页面",
		Long: `更新指定应用页面的名称、背景、图标或导航隐藏状态，未提供的字段保持不变。
--background 和 --icon 是完整 JSON 对象并整体替换；--hidden-menu=false 会被明确发送。
页面排序请使用 app page move。`,
		Example:       "  dws aitable app page update --base-id BASE_ID --page-id PAGE_ID --name \"经营看板\"",
		Tool:          "update_app_page",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppIdempotentWriteSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			aitableAppPageIDFlag(),
			{Name: "name", Usage: "新的页面名称", Bind: "name", Trim: true, OmitEmpty: true},
			aitableAppJSONObjectFlag("background", "background", "新的页面背景 JSON 对象，整体替换且 type 必填", false, validateAitableAppTypedObject("background")),
			aitableAppJSONObjectFlag("icon", "icon", "新的页面图标 JSON 对象，整体替换且 type 必填", false, validateAitableAppTypedObject("icon")),
			{Name: "hidden-menu", Usage: "是否在导航菜单中隐藏页面；显式 false 表示重新显示", Kind: LeafBool, Bind: "hiddenMenu"},
		},
		Constraints: []LeafConstraint{{
			Kind:        corecmd.AtLeastOne,
			Flags:       []string{"name", "background", "icon", "hidden-menu"},
			Description: "--name、--background、--icon、--hidden-menu 至少提供一个",
		}},
		Contract: aitableAppContract(
			"app_page_update", "aitable app page update", "update_app_page",
			"更新应用页面名称、背景、图标或导航隐藏状态。",
			"更新应用页面元数据；页面排序使用 app page move。",
			[]string{"需要修改已知页面的名称、背景、图标或导航可见性时"},
			[]string{"调整页面顺序用 app page move；修改 Widget 用 app widget update"},
			[]string{"dws aitable app page update --base-id <BASE_ID> --page-id <PAGE_ID> --name \"经营看板\""},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("name", "name", "string", false),
				aitableAppParam("background", "background", "object", false),
				aitableAppParam("icon", "icon", "object", false),
				aitableAppParam("hidden-menu", "hiddenMenu", "boolean", false),
			},
			aitableAppPageResultSpec(),
		),
	})

	pageMoveCmd := NewLeafCommand(LeafSpec{
		Use:           "move",
		Short:         "调整应用页面顺序",
		Long:          "调整应用页面的导航顺序。--before-page-id 指定时将 pageId 移到该页面之前，省略或空值时移动到末尾。",
		Example:       "  dws aitable app page move --base-id BASE_ID --page-id PAGE_ID --before-page-id TARGET_PAGE_ID",
		Tool:          "move_app_page",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppIdempotentWriteSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			aitableAppPageIDFlag(),
			{Name: "before-page-id", Usage: "移动到此页面之前；省略或空值表示移动到末尾", Bind: "beforePageId", Trim: true, OmitEmpty: true},
		},
		Contract: aitableAppContract(
			"app_page_move", "aitable app page move", "move_app_page",
			"调整应用页面导航顺序。",
			"将应用页面移到另一页面之前或导航末尾。",
			[]string{"需要调整已知 pageId 的应用导航顺序时"},
			[]string{"更新页面名称、背景、图标或隐藏状态用 app page update"},
			[]string{"dws aitable app page move --base-id <BASE_ID> --page-id <PAGE_ID> --before-page-id <TARGET_PAGE_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("before-page-id", "beforePageId", "string", false),
			},
			aitableAppStateResultSpec(),
		),
	})

	pageDeleteCmd := NewLeafCommand(LeafSpec{
		Use:   "delete",
		Short: "删除应用页面",
		Long: `删除指定应用模式页面及其全部 Widget，并保留 App 本体。删除不可逆。
允许删除最后一个 type=page 页面；删除后 App 仍存在且页面列表为空。`,
		Example:       "  dws aitable app page delete --base-id BASE_ID --page-id PAGE_ID",
		Tool:          "delete_app_page",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableSafetyDestructive(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag(), aitableAppPageIDFlag()},
		Contract: aitableAppContract(
			"app_page_delete", "aitable app page delete", "delete_app_page",
			"删除应用页面及其全部 Widget（不可逆，需确认）。",
			"删除已确认的应用页面及其全部 Widget。",
			[]string{"用户明确要求删除已核对的应用页面，并理解其全部 Widget 会级联删除时"},
			[]string{"只删除单个 Widget 用 app widget delete；普通 Dashboard 删除用 dashboard delete"},
			[]string{"dws aitable app page delete --base-id <BASE_ID> --page-id <PAGE_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
			},
			aitableAppPageDeleteResultSpec(),
		),
	})

	widgetCreateCmd := NewLeafCommand(LeafSpec{
		Use:   "create",
		Short: "创建应用页面 Widget",
		Long: `在应用模式仪表盘页面中创建 Widget，并同时写入页面 Dashboard 布局。
--config 是完整 Widget 配置对象且 chartType 必填；--layout 的 x/y/w/h 必填。
根布局按 48 列坐标系处理；非根 parentId 使用其容器自己的坐标系。创建非幂等，CLI 不自动重试。`,
		Example:       `  dws aitable app widget create --base-id BASE_ID --page-id PAGE_ID --config '{"chartType":"AI_ANALYZE"}' --layout '{"x":0,"y":0,"w":48,"h":8}'`,
		Tool:          "create_app_widget",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppCreateSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			aitableAppPageIDFlag(),
			{Name: "name", Usage: "Widget 名称；省略时使用服务端默认名称", Bind: "name", Trim: true, OmitEmpty: true},
			aitableAppJSONObjectFlag("config", "config", "Widget 完整配置 JSON 对象，chartType 必填", true, validateAitableAppWidgetConfig),
			aitableAppJSONObjectFlag("layout", "layout", "Widget 布局 JSON 对象，x/y/w/h 必填，parentId 可选", true, validateAitableAppWidgetLayout),
		},
		Contract: aitableAppContract(
			"app_widget_create", "aitable app widget create", "create_app_widget",
			"在应用页面创建 Widget 并写入 Dashboard 布局。",
			"使用完整 config 和 layout 在应用页面创建 Widget。",
			[]string{"需要在已知应用页面新增 Widget，且已有合法 chartType 配置和 48 列布局时"},
			[]string{"普通 Dashboard 图表用 chart create；不知道 config 时先用 chart widgets-example"},
			[]string{`dws aitable app widget create --base-id <BASE_ID> --page-id <PAGE_ID> --config '{"chartType":"AI_ANALYZE"}' --layout '{"x":0,"y":0,"w":48,"h":8}'`},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("name", "name", "string", false),
				aitableAppParam("config", "config", "object", true),
				aitableAppParam("layout", "layout", "object", true),
			},
			aitableAppWidgetResultSpec(),
		),
	})

	widgetGetCmd := NewLeafCommand(LeafSpec{
		Use:           "get",
		Short:         "获取应用页面 Widget",
		Long:          "获取应用模式仪表盘页面中指定 Widget 的名称、完整配置与布局。当前只支持 type=page 页面下的 Dashboard Widget。",
		Example:       "  dws aitable app widget get --base-id BASE_ID --page-id PAGE_ID --widget-id WIDGET_ID",
		Tool:          "get_app_widget",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableSafetyRead(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag(), aitableAppPageIDFlag(), aitableAppWidgetIDFlag()},
		Contract: aitableAppContract(
			"app_widget_get", "aitable app widget get", "get_app_widget",
			"获取应用页面 Widget 的名称、配置与布局。",
			"获取已知 widgetId 的完整配置与布局。",
			[]string{"已知应用页面和 widgetId，需要读取配置或布局时"},
			[]string{"列出页面全部 Widget 用 app widget list；普通 Dashboard Chart 用 chart get"},
			[]string{"dws aitable app widget get --base-id <BASE_ID> --page-id <PAGE_ID> --widget-id <WIDGET_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("widget-id", "widgetId", "string", true),
			},
			aitableAppWidgetResultSpec(),
		),
	})

	widgetListCmd := NewLeafCommand(LeafSpec{
		Use:   "list",
		Short: "列出应用页面 Widget",
		Long: `一次性列出应用模式页面中的全部 Widget 摘要，按底层稳定存储顺序返回。
该工具只读取已有 App，不会自动创建；单个 Page 最多返回 1000 个 Widget，超过上限时返回错误。`,
		Example:       "  dws aitable app widget list --base-id BASE_ID --page-id PAGE_ID",
		Tool:          "list_page_widgets",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableSafetyRead(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag(), aitableAppPageIDFlag()},
		Contract: aitableAppContract(
			"app_widget_list", "aitable app widget list", "list_page_widgets",
			"列出应用页面中的全部 Widget 摘要。",
			"列出页面 Widget，获取 widgetId 和稳定存储顺序。",
			[]string{"需要发现某个应用页面的 Widget 或取得 widgetId 时"},
			[]string{"已知 widgetId 查完整配置用 app widget get；普通 Dashboard 图表列表用 dashboard get"},
			[]string{"dws aitable app widget list --base-id <BASE_ID> --page-id <PAGE_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
			},
			aitableAppWidgetListResultSpec(),
		),
	})

	widgetUpdateCmd := NewLeafCommand(LeafSpec{
		Use:   "update",
		Short: "更新应用页面 Widget",
		Long: `更新应用模式页面 Widget 的名称、完整配置或布局，至少提供一项。
--config 一旦提供会整体替换原配置且 chartType 必填；应先用 app widget get 读取当前配置。
--layout 一旦提供必须包含 x/y/w/h，只更新当前 Widget 的布局。`,
		Example:       "  dws aitable app widget update --base-id BASE_ID --page-id PAGE_ID --widget-id WIDGET_ID --name \"销售趋势\"",
		Tool:          "update_app_widget",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableAppIdempotentWriteSafety(),
		Flags: []LeafFlag{
			aitableAppBaseIDFlag(),
			aitableAppPageIDFlag(),
			aitableAppWidgetIDFlag(),
			{Name: "name", Usage: "新的 Widget 名称", Bind: "name", Trim: true, OmitEmpty: true},
			aitableAppJSONObjectFlag("config", "config", "新的 Widget 完整配置 JSON 对象，整体替换且 chartType 必填", false, validateAitableAppWidgetConfig),
			aitableAppJSONObjectFlag("layout", "layout", "新的 Widget 布局 JSON 对象，x/y/w/h 必填", false, validateAitableAppWidgetLayout),
		},
		Constraints: []LeafConstraint{{
			Kind:        corecmd.AtLeastOne,
			Flags:       []string{"name", "config", "layout"},
			Description: "--name、--config、--layout 至少提供一个",
		}},
		Contract: aitableAppContract(
			"app_widget_update", "aitable app widget update", "update_app_widget",
			"更新应用页面 Widget 的名称、完整配置或布局。",
			"更新 Widget；config 是全量替换，修改前应先读取当前配置。",
			[]string{"需要修改已知 Widget 的名称、完整 config 或布局时"},
			[]string{"只查询用 app widget get；普通 Dashboard Chart 用 chart update"},
			[]string{"dws aitable app widget update --base-id <BASE_ID> --page-id <PAGE_ID> --widget-id <WIDGET_ID> --name \"销售趋势\""},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("widget-id", "widgetId", "string", true),
				aitableAppParam("name", "name", "string", false),
				aitableAppParam("config", "config", "object", false),
				aitableAppParam("layout", "layout", "object", false),
			},
			aitableAppWidgetResultSpec(),
		),
	})

	widgetDeleteCmd := NewLeafCommand(LeafSpec{
		Use:           "delete",
		Short:         "删除应用页面 Widget",
		Long:          "删除应用模式页面中指定 Widget，并同步移除其 Dashboard 布局项。删除不可逆。",
		Example:       "  dws aitable app widget delete --base-id BASE_ID --page-id PAGE_ID --widget-id WIDGET_ID",
		Tool:          "delete_app_widget",
		OutputRollout: output.RolloutUnifiedActive,
		ResultCall:    callAitableAppResult,
		Safety:        aitableSafetyDestructive(),
		Flags:         []LeafFlag{aitableAppBaseIDFlag(), aitableAppPageIDFlag(), aitableAppWidgetIDFlag()},
		Contract: aitableAppContract(
			"app_widget_delete", "aitable app widget delete", "delete_app_widget",
			"删除应用页面 Widget 及其布局项（不可逆，需确认）。",
			"删除已确认的应用页面 Widget 并同步清理布局。",
			[]string{"用户明确要求删除已核对的应用页面 Widget 时"},
			[]string{"删除整个页面及其所有 Widget 用 app page delete；普通 Dashboard Chart 用 chart delete"},
			[]string{"dws aitable app widget delete --base-id <BASE_ID> --page-id <PAGE_ID> --widget-id <WIDGET_ID>"},
			[]contract.ParamDecl{
				aitableAppParam("base-id", "baseId", "string", true),
				aitableAppParam("page-id", "pageId", "string", true),
				aitableAppParam("widget-id", "widgetId", "string", true),
			},
			aitableAppWidgetDeleteResultSpec(),
		),
	})

	pageCmd.AddCommand(pageCreateCmd, pageGetCmd, pageListCmd, pageUpdateCmd, pageMoveCmd, pageDeleteCmd)
	widgetCmd.AddCommand(widgetCreateCmd, widgetGetCmd, widgetListCmd, widgetUpdateCmd, widgetDeleteCmd)
	appCmd.AddCommand(appGetCmd, appUpdateCmd, pageCmd, widgetCmd)
	return appCmd
}

func callAitableAppResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	raw, err := CallMCPToolDataOnServer(cmd.Context(), aitableAppMCPServer, tool, args)
	if err != nil {
		return nil, err
	}
	envelope, ok := raw.(map[string]any)
	if !ok || envelope == nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("aitable/%s 返回值不是 JSON 对象", tool))
	}
	data, ok := envelope["data"]
	if !ok {
		return nil, apperrors.NewInternal(fmt.Sprintf("aitable/%s 返回值缺少 data", tool))
	}
	if _, ok := data.(map[string]any); !ok {
		return nil, apperrors.NewInternal(fmt.Sprintf("aitable/%s 返回值 data 不是 JSON 对象", tool))
	}
	return output.Success(data), nil
}

func aitableAppBaseIDFlag() LeafFlag {
	return LeafFlag{
		Name: "base-id", Usage: "所属 Base 的唯一标识 (必填)", Bind: "baseId",
		Trim: true, Required: true, Aliases: []string{"base"},
	}
}

func aitableAppPageIDFlag() LeafFlag {
	return LeafFlag{Name: "page-id", Usage: "应用页面 ID (必填)", Bind: "pageId", Trim: true, Required: true}
}

func aitableAppWidgetIDFlag() LeafFlag {
	return LeafFlag{Name: "widget-id", Usage: "页面 Widget ID (必填)", Bind: "widgetId", Trim: true, Required: true}
}

func aitableAppJSONObjectFlag(name, bind, usage string, required bool, validate func(map[string]any) error) LeafFlag {
	return LeafFlag{
		Name: name, Usage: usage, Bind: bind, Required: required, Trim: true, OmitEmpty: !required,
		Transform: func(raw string) (any, error) {
			value, err := parseAitableObjectFlag(name, raw)
			if err != nil {
				return nil, err
			}
			if validate != nil {
				if err := validate(value); err != nil {
					return nil, err
				}
			}
			return value, nil
		},
	}
}

func validateAitableAppTypedObject(flag string) func(map[string]any) error {
	return func(value map[string]any) error {
		if err := requireAitableAppString(value, "type", "--"+flag); err != nil {
			return err
		}
		for _, field := range []string{"id", "url", "image", "value"} {
			if raw, ok := value[field]; ok {
				if _, ok := raw.(string); !ok {
					return fmt.Errorf("--%s.%s 必须是字符串", flag, field)
				}
			}
		}
		return nil
	}
}

func validateAitableAppWidgetConfig(value map[string]any) error {
	return requireAitableAppString(value, "chartType", "--config")
}

func validateAitableAppWidgetLayout(value map[string]any) error {
	for _, field := range []string{"x", "y", "w", "h"} {
		raw, ok := value[field]
		if !ok {
			return fmt.Errorf("--layout.%s 为必填数字字段", field)
		}
		if _, ok := raw.(json.Number); !ok {
			return fmt.Errorf("--layout.%s 必须是数字", field)
		}
	}
	if raw, ok := value["parentId"]; ok {
		if _, ok := raw.(string); !ok {
			return fmt.Errorf("--layout.parentId 必须是字符串")
		}
	}
	return nil
}

func requireAitableAppString(value map[string]any, field, flag string) error {
	raw, ok := value[field]
	if !ok {
		return fmt.Errorf("%s.%s 为必填字符串字段", flag, field)
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("%s.%s 必须是非空字符串", flag, field)
	}
	return nil
}

func aitableAppConditionalWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func aitableAppIdempotentWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func aitableAppCreateSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "non_idempotent",
	}
}

func aitableAppContract(
	name, cliPath, rpc, description, summary string,
	useWhen, avoidWhen, examples []string,
	parameters []contract.ParamDecl,
	result *contract.ResultSpec,
) LeafContract {
	return LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           name,
			CanonicalPath:  "aitable." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
		Description: description,
		Interface:   aitableMCPInterface(rpc),
		Selection: contract.SelectionSpec{
			AgentSummary: summary,
			UseWhen:      useWhen,
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
		Parameters: parameters,
		Result:     result,
	}
}

func aitableAppParam(name, property, interfaceType string, required bool) contract.ParamDecl {
	return contract.ParamDecl{
		Name: name, Property: property, InterfaceType: interfaceType, Required: boolPtr(required),
	}
}

func aitableAppGetResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"应用模式 App 查询或初始化结果",
  "properties":{
    "created":{"type":"boolean","description":"本次调用是否新建了默认 App"},
    "app":{
      "type":"object",
      "description":"Base 唯一面向用户的应用模式 App",
      "properties":{
        "appId":{"type":"string","description":"用于应用模式在线访问 URL 的编码标识"},
        "name":{"type":"string","description":"App 显示名称"},
        "appearance":{"type":"string","description":"App 外观模式"},
        "themeType":{"type":"string","description":"App 主题类型"},
        "navigationLayout":{"type":"string","description":"App 导航布局"},
        "pages":{"type":"array","description":"App 页面摘要列表","items":{"type":"object","additionalProperties":true}}
      },
      "required":["appId","pages"],
      "additionalProperties":true
    }
  },
  "required":["created","app"],
  "additionalProperties":true
}`),
	}
}

func aitableAppResultSpec(schema string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(schema),
	}
}

func aitableAppStateResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"更新或移动页面后的应用模式 App 状态",
  "properties":{
    "appearance":{"type":"string","description":"App 外观模式"},
    "name":{"type":"string","description":"App 显示名称"},
    "navThemeType":{"type":"string","description":"App 导航主题类型"},
    "navigationLayout":{"type":"string","description":"App 导航布局"},
    "pages":{"type":"array","description":"按导航顺序排列的应用页面摘要","items":{"type":"object","additionalProperties":true}},
    "themeType":{"type":"string","description":"App 主题类型"}
  },
  "required":["name","pages"],
  "additionalProperties":true
}`)
}

func aitableAppPageResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用模式仪表盘页面详情",
  "properties":{
    "background":{"type":"object","description":"页面背景配置","properties":{"image":{"type":"string","description":"背景图片 URL"},"type":{"type":"string","description":"背景类型"},"value":{"type":"string","description":"背景颜色或资源值"}},"additionalProperties":true},
    "hiddenMenu":{"type":"boolean","description":"页面是否在导航菜单中隐藏"},
    "icon":{"type":"object","description":"页面图标配置","properties":{"id":{"type":"string","description":"图标资源 ID"},"type":{"type":"string","description":"图标类型"},"url":{"type":"string","description":"图标资源 URL"}},"additionalProperties":true},
    "layout":{"type":"array","description":"页面 Dashboard 布局条目","items":{"type":"object","additionalProperties":true}},
    "pageId":{"type":"string","description":"页面 ID，同时也是对应 Dashboard ID"},
    "pageName":{"type":"string","description":"页面显示名称"},
    "type":{"type":"string","description":"页面类型，当前为 page"},
    "widgets":{"type":"array","description":"页面 Widget 详情或摘要列表","items":{"type":"object","additionalProperties":true}}
  },
  "required":["pageId","pageName","type","layout","widgets"],
  "additionalProperties":true
}`)
}

func aitableAppPageListResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用页面列表",
  "properties":{
    "pages":{"type":"array","description":"按导航顺序排列的页面摘要","items":{"type":"object","properties":{"hiddenMenu":{"type":"boolean","description":"页面是否在导航菜单中隐藏"},"pageId":{"type":"string","description":"页面 ID，同时也是对应 Dashboard ID"},"pageName":{"type":"string","description":"页面显示名称"},"type":{"type":"string","description":"页面类型，当前为 page"}},"required":["pageId","pageName","type"],"additionalProperties":true}},
    "total":{"type":"integer","description":"返回的页面总数"}
  },
  "required":["pages","total"],
  "additionalProperties":true
}`)
}

func aitableAppPageDeleteResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用页面删除确认",
  "properties":{
    "deletedPageId":{"type":"string","description":"已删除的页面 ID"},
    "deletedWidgetCount":{"type":"integer","description":"随页面级联删除的 Widget 数量"}
  },
  "required":["deletedPageId","deletedWidgetCount"],
  "additionalProperties":true
}`)
}

func aitableAppWidgetResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用页面 Widget 详情",
  "properties":{
    "config":{"type":"object","description":"Widget 完整配置","additionalProperties":true},
    "layout":{"type":"object","description":"Widget 布局","properties":{"h":{"type":"number","description":"布局高度"},"i":{"type":"string","description":"布局项 ID，通常等于 widgetId"},"parentId":{"type":"string","description":"可选父容器 ID"},"w":{"type":"number","description":"布局宽度"},"x":{"type":"number","description":"布局横坐标"},"y":{"type":"number","description":"布局纵坐标"}},"required":["x","y","w","h"],"additionalProperties":true},
    "pageId":{"type":"string","description":"所属应用页面 ID"},
    "widgetId":{"type":"string","description":"Widget ID"},
    "widgetName":{"type":"string","description":"Widget 显示名称"},
    "widgetType":{"type":"string","description":"Widget 类型，取自 config.chartType"}
  },
  "required":["config","layout","pageId","widgetId","widgetName","widgetType"],
  "additionalProperties":true
}`)
}

func aitableAppWidgetListResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用页面 Widget 摘要列表",
  "properties":{
    "pageId":{"type":"string","description":"所属应用页面 ID"},
    "total":{"type":"integer","description":"返回的 Widget 总数"},
    "widgets":{"type":"array","description":"按稳定存储顺序返回的 Widget 摘要","items":{"type":"object","properties":{"widgetId":{"type":"string","description":"Widget ID"},"widgetName":{"type":"string","description":"Widget 显示名称"},"widgetType":{"type":"string","description":"Widget 类型"}},"required":["widgetId","widgetName","widgetType"],"additionalProperties":true}}
  },
  "required":["pageId","total","widgets"],
  "additionalProperties":true
}`)
}

func aitableAppWidgetDeleteResultSpec() *contract.ResultSpec {
	return aitableAppResultSpec(`{
  "type":"object",
  "description":"应用页面 Widget 删除确认",
  "properties":{"deletedWidgetId":{"type":"string","description":"已删除的 Widget ID"}},
  "required":["deletedWidgetId"],
  "additionalProperties":true
}`)
}
