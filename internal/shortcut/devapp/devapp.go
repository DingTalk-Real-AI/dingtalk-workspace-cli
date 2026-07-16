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

// Package devapp declares high-fidelity shortcuts for the DingTalk open-platform
// developer application ("devapp") MCP service, for the apps
// command surface. Tool names and parameter keys are copied verbatim from
// internal/helpers/devapp.go — do not invent tools or params here.
//
// Skipped tools (require async orchestration / polling, not a single MCP call):
//   - submit_robot_create_task  (异步提交机器人建号任务)
//   - query_robot_create_result (异步轮询建号结果)
//
// The robot-config i18n JSON-object params (i18nName/i18nBrief/i18nDescription)
// are also omitted: they need JSON-object parsing that a flat flag can't express.
package devapp

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const productDevApp = "devapp"

// applyCursor forwards --cursor/--page-size into params, matching the helper's
// pass-through pagination (pageSize defaults to 20, cursor omitted on page 1).
func applyCursor(rt *shortcut.RuntimeContext, params map[string]any) {
	if rt.Changed("cursor") {
		if cur := rt.Str("cursor"); cur != "" {
			params["cursor"] = cur
		}
	}
	size := rt.Int("page-size")
	if size < 1 {
		size = 20
	}
	params["pageSize"] = size
}

var cursorFlags = []shortcut.Flag{
	{Name: "cursor", Type: shortcut.FlagString, Desc: "游标令牌：首次查询留空，续翻传上次出参的 nextCursor"},
	{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "单页条数，默认 20"},
}

// ---------------------------------------------------------------------------
// 应用主体
// ---------------------------------------------------------------------------

// ListApp maps helper `list_dev_app`.
var ListApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+list",
	Product:     productDevApp,
	Description: "查询开放平台企业内部应用列表",
	Intent:      "当你要在开发者后台盘点或定位某个企业内部应用（例如按应用名、appKey、创建人或机器人名搜索，拿到其 unifiedAppId 以便后续查看详情、配置或发布）时使用；支持关键词过滤、排序和分页，返回应用列表。",
	Risk:        shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "应用名称关键词"},
		{Name: "app-key", Type: shortcut.FlagString, Desc: "按 appKey/clientId 过滤"},
		{Name: "app-group-id", Type: shortcut.FlagInt, Desc: "应用分组 ID"},
		{Name: "creator", Type: shortcut.FlagString, Desc: "创建人名称关键词"},
		{Name: "robot-name", Type: shortcut.FlagString, Desc: "机器人名称关键词"},
		{Name: "develop-type", Type: shortcut.FlagInt, Desc: "开发类型枚举；不确定时不要传"},
		{Name: "filter-cool-app", Type: shortcut.FlagInt, Desc: "酷应用过滤枚举；不确定时不要传"},
		{Name: "sort-type", Type: shortcut.FlagString, Desc: "排序字段，如 gmt_modified"},
		{Name: "sort-order", Type: shortcut.FlagString, Desc: "排序方向 asc 或 desc"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		applyCursor(rt, params)
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("app-key") {
			params["appKey"] = rt.Str("app-key")
		}
		if rt.Changed("app-group-id") {
			params["appGroupId"] = rt.Int("app-group-id")
		}
		if rt.Changed("creator") {
			params["creator"] = rt.Str("creator")
		}
		if rt.Changed("robot-name") {
			params["robotName"] = rt.Str("robot-name")
		}
		if rt.Changed("develop-type") {
			params["developType"] = rt.Int("develop-type")
		}
		if rt.Changed("filter-cool-app") {
			params["filterCoolApp"] = rt.Int("filter-cool-app")
		}
		if rt.Changed("sort-type") {
			params["sortType"] = rt.Str("sort-type")
		}
		if rt.Changed("sort-order") {
			params["sortOrder"] = rt.Str("sort-order")
		}
		data, err := rt.CallMCPData(productDevApp, "list_dev_app", params)
		if err != nil {
			return err
		}
		apps := listAppProject(data)
		return rt.Output(map[string]any{"count": len(apps), "apps": apps})
	},
}

// listAppProject reshapes list_dev_app into a clean app list
// ({unifiedAppId, name, appKey, agentId, status, gmtModified}) — output-projection
// clean output projection. The list container and per-item field names are probed
// defensively across candidate keys, so an unknown/empty shape yields an empty
// list rather than a crash or fabricated data.
func listAppProject(data map[string]any) []map[string]any {
	raw := listAppFindList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listAppFirst(m, "unifiedAppId", "unified_app_id"); ok {
			row["unifiedAppId"] = v
		}
		if v, ok := listAppFirst(m, "name", "appName", "app_name"); ok {
			row["name"] = v
		}
		if v, ok := listAppFirst(m, "appKey", "clientId", "app_key", "client_id"); ok {
			row["appKey"] = v
		}
		if v, ok := listAppFirst(m, "agentId", "agent_id"); ok {
			row["agentId"] = v
		}
		if v, ok := listAppFirst(m, "status", "appStatus", "app_status"); ok {
			row["status"] = v
		}
		if v, ok := listAppFirst(m, "gmtModified", "gmt_modified", "modifyTime", "modified_time"); ok {
			row["gmtModified"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listAppFindList locates the app list payload, tolerating a bare top-level
// array or nesting one level under a common envelope key.
func listAppFindList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, k := range []string{"list", "items", "apps", "appList", "result", "data"} {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "apps", "appList", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// listAppFirst returns the first present candidate key's value.
func listAppFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// GetApp maps helper `get_dev_app`.
var GetApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+get",
	Product:     productDevApp,
	Description: "查询开放平台企业内部应用详情",
	Intent:      "当你已知某应用的 unifiedAppId、需要查看它的完整配置信息（如名称、描述、图标、能力开关等）以便核对现状或作为修改前的依据时使用；输入 unifiedAppId，返回单个应用的详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dev_app", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// CreateApp maps helper `create_dev_app`.
var CreateApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+create",
	Product:     productDevApp,
	Description: "创建开放平台企业内部应用",
	Intent:      "当你要在开放平台从零新建一个企业内部应用（H5/机器人等的载体）时使用；传入应用名称、可选描述与图标 mediaId，会实际创建出一个新应用并返回其 unifiedAppId 供后续配置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "应用名称", Required: true},
		{Name: "desc", Type: shortcut.FlagString, Desc: "应用描述"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "应用图标 mediaId"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"name": rt.Str("name")}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		return rt.CallMCP("create_dev_app", params)
	},
}

// UpdateApp maps helper `update_dev_app`.
var UpdateApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+update",
	Product:     productDevApp,
	Description: "修改开放平台企业内部应用基础信息",
	Intent:      "当你要改动一个已存在应用的基础信息（更名、改描述或换图标）时使用；指定 unifiedAppId 及要更新的字段，会实际写回并覆盖对应的应用基础资料。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新的应用名称"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "新的应用描述"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "新的应用图标 mediaId"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		return rt.CallMCP("update_dev_app", params)
	},
}

// DeleteApp maps helper `delete_dev_app`.
var DeleteApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+delete",
	Product:     productDevApp,
	Description: "删除开放平台企业内部应用（不可逆）",
	Intent:      "当你确认要彻底废弃某个企业内部应用时使用；传入 unifiedAppId 会真实且不可逆地删除该应用及其配置，执行前务必确认无误。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_dev_app", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// EnableApp maps helper `enable_dev_app`.
var EnableApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+enable",
	Product:     productDevApp,
	Description: "启用开放平台企业内部应用",
	Intent:      "当某个应用处于停用状态、你要让它重新生效可用时使用；传入 unifiedAppId 会实际将应用状态切换为启用。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("enable_dev_app", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// DisableApp maps helper `disable_dev_app`.
var DisableApp = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+disable",
	Product:     productDevApp,
	Description: "停用开放平台企业内部应用",
	Intent:      "当你要临时下线某个应用、让它对用户不可用又不删除时使用；传入 unifiedAppId 会实际将应用状态切换为停用，可日后再启用恢复。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("disable_dev_app", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// GetCredentials maps helper `get_dev_app_credentials`.
var GetCredentials = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+credentials-get",
	Product:     productDevApp,
	Description: "读取开放平台应用凭证",
	Intent:      "当你需要拿到某应用的鉴权凭证（如 clientId/AppKey、clientSecret/AppSecret）以便在代码或调试中调用开放平台接口时使用；输入 unifiedAppId，返回该应用的凭证信息。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_dev_app_credentials", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// ---------------------------------------------------------------------------
// 网页应用配置
// ---------------------------------------------------------------------------

// WebappGet maps helper `get_extension_webapp_config`.
var WebappGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+webapp-get",
	Product:     productDevApp,
	Description: "查询网页应用配置",
	Intent:      "当你要查看某应用的网页（H5）能力现状，如移动端/PC 首页地址、管理后台地址等，以便核对或作为改配置前的参考时使用；输入 unifiedAppId，返回当前网页应用配置。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_extension_webapp_config", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// WebappConfig maps helper `set_extension_webapp_config`.
var WebappConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+webapp-config",
	Product:     productDevApp,
	Description: "配置网页应用能力",
	Intent:      "当你要为应用开通或调整网页（H5）入口，如设置移动端/PC 端首页 URL、管理后台地址或页面类型时使用；指定 unifiedAppId 及相应地址，会实际写入该应用的网页应用配置。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "h5-page-type", Type: shortcut.FlagString, Desc: "网页应用生效端/页面类型"},
		{Name: "homepage-url", Type: shortcut.FlagString, Desc: "移动端首页地址"},
		{Name: "pc-homepage-url", Type: shortcut.FlagString, Desc: "PC 端首页地址"},
		{Name: "omp-url", Type: shortcut.FlagString, Desc: "管理后台地址"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("h5-page-type") {
			params["h5PageType"] = rt.Str("h5-page-type")
		}
		if rt.Changed("homepage-url") {
			params["homepageUrl"] = rt.Str("homepage-url")
		}
		if rt.Changed("pc-homepage-url") {
			params["pcHomepageUrl"] = rt.Str("pc-homepage-url")
		}
		if rt.Changed("omp-url") {
			params["ompUrl"] = rt.Str("omp-url")
		}
		return rt.CallMCP("set_extension_webapp_config", params)
	},
}

// ---------------------------------------------------------------------------
// 权限
// ---------------------------------------------------------------------------

// PermissionList maps helper `list_dev_app_permissions`.
var PermissionList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-list",
	Product:     productDevApp,
	Description: "查询开放平台应用权限列表",
	Intent:      "当你要查看某应用已申请/可申请的 API 权限点及其授权状态（用于排查接口报权限错、或确认某 scopeValue 是否已开通）时使用；可按关键词、scopeValue、授权状态等过滤，返回权限点列表。",
	Risk:        shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "权限名、权限点、接口名关键词"},
		{Name: "scope-value", Type: shortcut.FlagString, Desc: "精确权限点 scopeValue"},
		{Name: "auth-status", Type: shortcut.FlagString, Default: "ALL", Desc: "权限状态：ALL、AUTHED、UNAUTHED"},
		{Name: "scope-type", Type: shortcut.FlagString, Desc: "权限一级类型：APP 或 SNS"},
		{Name: "api-status", Type: shortcut.FlagString, Desc: "开发者后台 apiStatus 过滤"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		if rt.Changed("scope-value") {
			params["scopeValue"] = rt.Str("scope-value")
		}
		if v := strings.ToUpper(rt.Str("auth-status")); v != "" {
			params["authStatus"] = v
		}
		if rt.Changed("scope-type") {
			params["scopeType"] = strings.ToUpper(rt.Str("scope-type"))
		}
		if rt.Changed("api-status") {
			params["apiStatus"] = rt.Str("api-status")
		}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_permissions", params)
		if err != nil {
			return err
		}
		permissions := permissionListProject(data)
		return rt.Output(map[string]any{"count": len(permissions), "permissions": permissions})
	},
}

// permissionListProject reshapes list_dev_app_permissions into a clean
// permission-point list ({scopeValue, scopeName, apiName, authStatus, scopeType})
// — clean output projection. The list container and per-item field
// names are probed defensively across candidate keys, so an unknown/empty shape
// yields an empty list rather than a crash or fabricated data.
func permissionListProject(data map[string]any) []map[string]any {
	raw := permissionListFindList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := permissionListFirst(m, "scopeValue", "scope_value", "permissionCode", "code"); ok {
			row["scopeValue"] = v
		}
		if v, ok := permissionListFirst(m, "scopeName", "scope_name", "permissionName", "name"); ok {
			row["scopeName"] = v
		}
		if v, ok := permissionListFirst(m, "apiName", "api_name", "interfaceName"); ok {
			row["apiName"] = v
		}
		if v, ok := permissionListFirst(m, "authStatus", "auth_status", "status"); ok {
			row["authStatus"] = v
		}
		if v, ok := permissionListFirst(m, "scopeType", "scope_type"); ok {
			row["scopeType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// permissionListFindList locates the permission list payload, tolerating a bare
// top-level array or nesting one level under a common envelope key.
func permissionListFindList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, k := range []string{"list", "items", "permissions", "permissionList", "scopes", "result", "data"} {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "permissions", "permissionList", "scopes", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// permissionListFirst returns the first present candidate key's value.
func permissionListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// PermissionAdd maps helper `apply_dev_app_permissions`.
var PermissionAdd = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-add",
	Product:     productDevApp,
	Description: "申请开放平台应用权限点",
	Intent:      "当应用调用某接口报缺少权限、你要为它开通一批 API 权限点时使用；传入 unifiedAppId 和 scopeValue 列表，会实际为该应用申请/授予这些权限。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "scope-values", Type: shortcut.FlagStringSlice, Desc: "权限点 scopeValue 列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"scopeValues":  rt.StrSlice("scope-values"),
		}
		return rt.CallMCP("apply_dev_app_permissions", params)
	},
}

// PermissionRemove maps helper `remove_dev_app_permissions`.
var PermissionRemove = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+permission-remove",
	Product:     productDevApp,
	Description: "取消开放平台应用权限点",
	Intent:      "当你要收回应用已开通的某些 API 权限（如安全收敛、下线不再使用的接口）时使用；传入 unifiedAppId 和待取消的 scopeValue 列表，会实际移除这些权限授权。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "scope-values", Type: shortcut.FlagStringSlice, Desc: "待取消权限点 scopeValue 列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"scopeValues":  rt.StrSlice("scope-values"),
		}
		return rt.CallMCP("remove_dev_app_permissions", params)
	},
}

// ---------------------------------------------------------------------------
// 成员
// ---------------------------------------------------------------------------

// MemberList maps helper `list_dev_app_members`.
var MemberList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-list",
	Product:     productDevApp,
	Description: "查询开放平台应用成员",
	Intent:      "当你要查看某应用有哪些成员及其角色（如谁是开发者/管理员），用于核对协作人员或权限归属时使用；输入 unifiedAppId，返回成员列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_dev_app_members", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// MemberAdd maps helper `add_dev_app_members`.
var MemberAdd = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-add",
	Product:     productDevApp,
	Description: "添加开放平台应用成员",
	Intent:      "当你要给某应用增加协作人员（如把某人加为开发者）时使用；传入 unifiedAppId、userId 列表和成员类型（如 DEVELOPER），会实际把这些人加入应用成员并赋予对应角色。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "user-ids", Type: shortcut.FlagStringSlice, Desc: "成员 userId 列表", Required: true},
		{Name: "member-type", Type: shortcut.FlagString, Desc: "成员类型，如 DEVELOPER", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"userIds":      rt.StrSlice("user-ids"),
			"memberType":   rt.Str("member-type"),
		}
		return rt.CallMCP("add_dev_app_members", params)
	},
}

// MemberRemove maps helper `remove_dev_app_members`.
var MemberRemove = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+member-remove",
	Product:     productDevApp,
	Description: "移除开放平台应用成员",
	Intent:      "当某人离职或不再参与、你要取消其对应用的访问/协作权限时使用；传入 unifiedAppId、userId 列表和成员类型，会实际把这些人从应用成员中移除。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "user-ids", Type: shortcut.FlagStringSlice, Desc: "成员 userId 列表", Required: true},
		{Name: "member-type", Type: shortcut.FlagString, Desc: "成员类型，如 DEVELOPER", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"userIds":      rt.StrSlice("user-ids"),
			"memberType":   rt.Str("member-type"),
		}
		return rt.CallMCP("remove_dev_app_members", params)
	},
}

// ---------------------------------------------------------------------------
// 安全配置
// ---------------------------------------------------------------------------

// SecurityConfig maps helper `update_dev_app_security_config`.
var SecurityConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+security-config",
	Product:     productDevApp,
	Description: "更新开放平台应用安全配置（整组覆盖，非追加）",
	Intent:      "当你要设置应用的安全策略，如出口 IP 白名单、登录重定向 URL、端内免登地址时使用；注意每项传入的是整组值会全量覆盖旧配置（非追加），所以要一次性传全，常用于配置 OAuth 回调或加固网络白名单。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "ip-whitelist", Type: shortcut.FlagStringSlice, Desc: "出口 IP 白名单（整组覆盖）"},
		{Name: "redirect-urls", Type: shortcut.FlagStringSlice, Desc: "登录重定向 URL（整组覆盖）"},
		{Name: "sso-urls", Type: shortcut.FlagStringSlice, Desc: "端内免登地址（整组覆盖）"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("ip-whitelist") {
			params["ipWhitelist"] = rt.StrSlice("ip-whitelist")
		}
		if rt.Changed("redirect-urls") {
			params["redirectUrls"] = rt.StrSlice("redirect-urls")
		}
		if rt.Changed("sso-urls") {
			params["ssoUrls"] = rt.StrSlice("sso-urls")
		}
		return rt.CallMCP("update_dev_app_security_config", params)
	},
}

// ---------------------------------------------------------------------------
// 机器人能力
// ---------------------------------------------------------------------------

// RobotGet maps helper `get_extension_robot_config`.
var RobotGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-get",
	Product:     productDevApp,
	Description: "查询现有应用的机器人配置",
	Intent:      "当你要查看某应用已有的机器人配置（名称、回调地址、模式 HTTPS/STREAM/AISKILL、技能等）以核对现状或作为改配置前的依据时使用；输入 unifiedAppId，返回当前机器人配置。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_extension_robot_config", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// RobotConfig maps helper `set_extension_robot_config` (upsert). The i18n
// JSON-object params are intentionally omitted (see package doc).
var RobotConfig = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-config",
	Product:     productDevApp,
	Description: "创建或更新现有应用的机器人配置（upsert）",
	Intent:      "当你要为应用开通机器人或调整其机器人设置（改名称/简介/图标、设消息与事件回调地址、切换 HTTPS/STREAM/AISKILL 模式、配技能、是否自动加权限或关 SSL 校验）时使用；按 unifiedAppId 以 upsert 方式写入机器人配置，会实际生效。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "机器人名称"},
		{Name: "brief", Type: shortcut.FlagString, Desc: "机器人简介"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "机器人描述"},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "机器人图标 mediaId"},
		{Name: "outgoing-url", Type: shortcut.FlagString, Desc: "消息回调地址"},
		{Name: "event-callback-url", Type: shortcut.FlagString, Desc: "事件回调地址"},
		{Name: "mode", Type: shortcut.FlagString, Enum: []string{"HTTPS", "STREAM", "AISKILL"}, Desc: "机器人模式：HTTPS / STREAM / AISKILL"},
		{Name: "skills", Type: shortcut.FlagStringSlice, Desc: "技能列表"},
		{Name: "add-scope", Type: shortcut.FlagBool, Desc: "是否自动添加机器人相关权限"},
		{Name: "disable-ssl-verify", Type: shortcut.FlagBool, Desc: "回调地址是否关闭 SSL 校验"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("name") {
			params["name"] = rt.Str("name")
		}
		if rt.Changed("brief") {
			params["brief"] = rt.Str("brief")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		if rt.Changed("icon-media-id") {
			params["iconMediaId"] = rt.Str("icon-media-id")
		}
		if rt.Changed("outgoing-url") {
			params["outgoingUrl"] = rt.Str("outgoing-url")
		}
		if rt.Changed("event-callback-url") {
			params["eventCallbackUrl"] = rt.Str("event-callback-url")
		}
		if rt.Changed("mode") {
			params["mode"] = strings.ToUpper(rt.Str("mode"))
		}
		if rt.Changed("skills") {
			params["skills"] = rt.StrSlice("skills")
		}
		if rt.Changed("add-scope") {
			params["addScope"] = rt.Bool("add-scope")
		}
		if rt.Changed("disable-ssl-verify") {
			params["disableSSLVerify"] = rt.Bool("disable-ssl-verify")
		}
		return rt.CallMCP("set_extension_robot_config", params)
	},
}

// RobotEnable maps helper `enable_dev_app_robot`.
var RobotEnable = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-enable",
	Product:     productDevApp,
	Description: "启用现有应用机器人能力（纯启用，无需配置字段）",
	Intent:      "当应用的机器人能力已配置但处于关闭状态、你只想把它打开生效时使用；仅传 unifiedAppId 即可，会实际启用该应用的机器人能力，无需再传配置字段。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("enable_dev_app_robot", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// RobotDisable maps helper `disable_dev_app_robot`.
var RobotDisable = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+robot-disable",
	Product:     productDevApp,
	Description: "停用现有应用的机器人能力",
	Intent:      "当你要临时关闭某应用的机器人（不再收发机器人消息）但保留其配置时使用；传入 unifiedAppId 会实际停用机器人能力，可日后再启用恢复。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("disable_dev_app_robot", map[string]any{"unifiedAppId": rt.Str("unified-app-id")})
	},
}

// ---------------------------------------------------------------------------
// 事件订阅
// ---------------------------------------------------------------------------

// EventList maps helper `list_dev_app_events`.
var EventList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-list",
	Product:     productDevApp,
	Description: "查询应用已订阅的事件列表",
	Intent:      "当你要确认某应用当前订阅了哪些事件回调（用于排查漏收事件、或退订前先查事件码）时使用；输入 unifiedAppId，可按事件码/名称关键词过滤并分页，返回已订阅事件列表。",
	Risk:        shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "事件搜索关键词，支持按事件码或事件名称模糊匹配"},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("keyword") {
			params["keyword"] = rt.Str("keyword")
		}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_events", params)
		if err != nil {
			return err
		}
		events := eventListProject(data)
		return rt.Output(map[string]any{"count": len(events), "events": events})
	},
}

// eventListProject reshapes list_dev_app_events into a clean subscribed-event
// list ({eventCode, eventName, status, gmtModified}) — output-projection
// clean output projection. The list container and per-item field names are probed
// defensively across candidate keys, so an unknown/empty shape yields an empty
// list rather than a crash or fabricated data.
func eventListProject(data map[string]any) []map[string]any {
	raw := eventListFindList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := eventListFirst(m, "eventCode", "event_code", "code"); ok {
			row["eventCode"] = v
		}
		if v, ok := eventListFirst(m, "eventName", "event_name", "name"); ok {
			row["eventName"] = v
		}
		if v, ok := eventListFirst(m, "status", "subscribeStatus", "subscribe_status"); ok {
			row["status"] = v
		}
		if v, ok := eventListFirst(m, "gmtModified", "gmt_modified", "modifyTime", "modified_time"); ok {
			row["gmtModified"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// eventListFindList locates the event list payload, tolerating a bare top-level
// array or nesting one level under a common envelope key.
func eventListFindList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, k := range []string{"list", "items", "events", "eventList", "result", "data"} {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "events", "eventList", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// eventListFirst returns the first present candidate key's value.
func eventListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// EventSubscribe maps helper `subscribe_dev_app_events`.
var EventSubscribe = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-subscribe",
	Product:     productDevApp,
	Description: "订阅应用事件回调",
	Intent:      "当你要让应用开始接收某些事件的回调推送（如通讯录变更、审批事件等）时使用；传入 unifiedAppId 和事件码列表，会实际为该应用登记这些事件订阅。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "event-codes", Type: shortcut.FlagStringSlice, Desc: "事件码列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"eventCodes":   rt.StrSlice("event-codes"),
		}
		return rt.CallMCP("subscribe_dev_app_events", params)
	},
}

// EventUnsubscribe maps helper `unsubscribe_dev_app_events`.
var EventUnsubscribe = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+event-unsubscribe",
	Product:     productDevApp,
	Description: "取消订阅应用事件",
	Intent:      "当你不再需要某些事件的回调推送、要停止接收它们时使用；传入 unifiedAppId 和事件码列表，会实际取消这些事件的订阅。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "event-codes", Type: shortcut.FlagStringSlice, Desc: "事件码列表", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"eventCodes":   rt.StrSlice("event-codes"),
		}
		return rt.CallMCP("unsubscribe_dev_app_events", params)
	},
}

// ---------------------------------------------------------------------------
// 版本发布
// ---------------------------------------------------------------------------

// VersionCreate maps helper `create_dev_app_version`.
var VersionCreate = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-create",
	Product:     productDevApp,
	Description: "基于当前配置创建应用新版本",
	Intent:      "当你改完应用配置、准备走发布流程前需要先打一个版本快照时使用；传入 unifiedAppId（可选显式版本号与描述，默认服务端自动递增），会实际创建一个新版本并返回 versionId 供后续预检和发布。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version", Type: shortcut.FlagString, Desc: "高级可选：显式版本号，如 1.0.1；默认由服务端自动递增"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "版本描述"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		if rt.Changed("version") {
			params["version"] = rt.Str("version")
		}
		if rt.Changed("desc") {
			params["desc"] = rt.Str("desc")
		}
		return rt.CallMCP("create_dev_app_version", params)
	},
}

// VersionList maps helper `list_dev_app_versions`.
var VersionList = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-list",
	Product:     productDevApp,
	Description: "分页查询应用版本列表",
	Intent:      "当你要查看某应用的历史版本（找某个 versionId、看各版本发布状态或回顾迭代记录）时使用；输入 unifiedAppId 并分页，返回版本列表。",
	Risk:        shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
	}, cursorFlags...),
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"unifiedAppId": rt.Str("unified-app-id")}
		applyCursor(rt, params)
		data, err := rt.CallMCPData(productDevApp, "list_dev_app_versions", params)
		if err != nil {
			return err
		}
		versions := versionListProject(data)
		return rt.Output(map[string]any{"count": len(versions), "versions": versions})
	},
}

// versionListProject reshapes list_dev_app_versions into a clean version list
// ({versionId, version, status, desc, gmtCreate}) — output-projection fidelity
// for clean output. The list container and per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func versionListProject(data map[string]any) []map[string]any {
	raw := versionListFindList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := versionListFirst(m, "versionId", "version_id", "id"); ok {
			row["versionId"] = v
		}
		if v, ok := versionListFirst(m, "version", "versionName", "version_name"); ok {
			row["version"] = v
		}
		if v, ok := versionListFirst(m, "status", "publishStatus", "publish_status", "versionStatus"); ok {
			row["status"] = v
		}
		if v, ok := versionListFirst(m, "desc", "description", "remark"); ok {
			row["desc"] = v
		}
		if v, ok := versionListFirst(m, "gmtCreate", "gmt_create", "createTime", "create_time"); ok {
			row["gmtCreate"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// versionListFindList locates the version list payload, tolerating a bare
// top-level array or nesting one level under a common envelope key.
func versionListFindList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, k := range []string{"list", "items", "versions", "versionList", "result", "data"} {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "versions", "versionList", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// versionListFirst returns the first present candidate key's value.
func versionListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// VersionGet maps helper `get_dev_app_version_detail`.
var VersionGet = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-get",
	Product:     productDevApp,
	Description: "查询指定版本详情",
	Intent:      "当你已知某个 versionId、要查看该版本的具体内容（版本号、描述、包含的配置等）以核对发布内容时使用；输入 unifiedAppId 和 versionId，返回单个版本的详情。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"versionId":    rt.Str("version-id"),
		}
		return rt.CallMCP("get_dev_app_version_detail", params)
	},
}

// VersionCheckApproval maps helper `publish_dev_app_version` in precheck mode
// (precheckOnly=true): it only returns approval requirements, does not publish.
var VersionCheckApproval = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-check-approval",
	Product:     productDevApp,
	Description: "预检版本发布是否需要审批（不实际发布）",
	Intent:      "当你在正式发布某版本前想先确认它是否会触发审批、是否含高敏权限等发布前置要求时使用；传入 unifiedAppId 和 versionId，仅做预检返回审批要求，不会真正发布。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"versionId":    rt.Str("version-id"),
			"precheckOnly": true,
		}
		return rt.CallMCP("publish_dev_app_version", params)
	},
}

// VersionPublish maps helper `publish_dev_app_version` (real publish,
// precheckOnly=false).
var VersionPublish = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-publish",
	Product:     productDevApp,
	Description: "发布指定版本（含高敏权限需 --confirmed-sensitive）",
	Intent:      "当你要把某个已创建的版本正式上线到线上环境时使用；传入 unifiedAppId 和 versionId，会实际触发发布（含高敏权限需加 --confirmed-sensitive 确认，灰度选人可指定审批人），可能进入审批流。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
		{Name: "confirmed-sensitive", Type: shortcut.FlagBool, Desc: "确认发布包含高敏权限的版本"},
		{Name: "approver-user-id", Type: shortcut.FlagString, Desc: "灰度选人模式下指定审批人 userId"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"versionId":    rt.Str("version-id"),
			"precheckOnly": false,
		}
		if rt.Changed("confirmed-sensitive") {
			params["confirmedSensitive"] = rt.Bool("confirmed-sensitive")
		}
		if rt.Changed("approver-user-id") {
			params["approverUserId"] = rt.Str("approver-user-id")
		}
		return rt.CallMCP("publish_dev_app_version", params)
	},
}

// VersionStatus maps helper `get_dev_app_version_status`.
var VersionStatus = shortcut.Shortcut{
	Service:     "devapp",
	Command:     "+version-status",
	Product:     productDevApp,
	Description: "查询版本发布/审批状态",
	Intent:      "当你已提交发布、想跟进某版本当前处于什么阶段（审批中、已发布、被驳回等）时使用；输入 unifiedAppId 和 versionId，返回该版本的发布/审批状态。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "unified-app-id", Type: shortcut.FlagString, Desc: "开放平台统一应用 ID", Required: true},
		{Name: "version-id", Type: shortcut.FlagString, Desc: "版本 ID", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"unifiedAppId": rt.Str("unified-app-id"),
			"versionId":    rt.Str("version-id"),
		}
		return rt.CallMCP("get_dev_app_version_status", params)
	},
}

func init() {
	shortcut.Register(
		ListApp,
		GetApp,
		CreateApp,
		UpdateApp,
		DeleteApp,
		EnableApp,
		DisableApp,
		GetCredentials,
		WebappGet,
		WebappConfig,
		PermissionList,
		PermissionAdd,
		PermissionRemove,
		MemberList,
		MemberAdd,
		MemberRemove,
		SecurityConfig,
		RobotGet,
		RobotConfig,
		RobotEnable,
		RobotDisable,
		EventList,
		EventSubscribe,
		EventUnsubscribe,
		VersionCreate,
		VersionList,
		VersionGet,
		VersionCheckApproval,
		VersionPublish,
		VersionStatus,
	)
}
