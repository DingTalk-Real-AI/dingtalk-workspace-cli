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

// This file declares the 7 datasource shortcuts for the aitable service:
// create / update / sync / sync-status / get-config / list-sources /
// get-fields. Each maps 1:1 onto a datasource MCP tool on the "aitable"
// server. datasource-type is passed through without CLI-side enum
// validation; source-config is validated as a JSON object but passed
// through as a raw string (MCP types it as string).

package aitable

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ─────────────────────────────────────────────────────────────
// datasource: 数据源同步管理（server: aitable）
// ─────────────────────────────────────────────────────────────

// DatasourceCreate 为指定 AI 表格创建数据源同步配置（create_datasource）。
var DatasourceCreate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-create",
	Product:     serverMain,
	Description: "为指定 AI 表格创建数据源同步配置，创建一张数据源表并触发首次全量同步。返回新建数据源表 ID 和同步任务 ID。",
	Intent:      "当用户需要将外部数据源（如审批数据）接入 AI 表格时使用。创建后返回的表 ID 可用于后续同步、更新配置或查询状态。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "non_idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_create",
			CanonicalPath:  "aitable.shortcut_datasource_create",
			CLIPath:        "aitable +datasource-create",
			PrimaryCLIPath: "aitable +datasource-create",
		},
		Description: "为指定 AI 表格创建数据源同步配置，创建一张数据源表并触发首次全量同步。返回新建数据源表 ID 和同步任务 ID。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "为指定 AI 表格创建数据源同步配置，创建一张数据源表并触发首次全量同步。返回新建数据源表 ID 和同步任务 ID。",
			UseWhen:      []string{"当用户需要将外部数据源（如审批数据）接入 AI 表格时使用。创建后返回的表 ID 可用于后续同步、更新配置或查询状态。"},
			AvoidWhen: []string{
				"目标 Base 已有数据源表且仅需更新配置时（改用 +datasource-update）",
				"仅需触发已有数据源表的同步时（改用 +datasource-sync）",
			},
			Examples: []string{
				`dws aitable +datasource-create --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
				`dws aitable +datasource-create --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"time_range","startDate":"2025-01-01","endDate":"2025-12-31","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}' --auto`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID（通过 +base-list / +base-search 获取）", Required: true},
		{Name: "datasource-type", Type: shortcut.FlagString, Desc: "数据源类型，目前支持审批（OA）", Required: true},
		{Name: "source-config", Type: shortcut.FlagString, Desc: "源配置 JSON 字符串。字段分为两类：须从 +datasource-list-sources 结果原样透传的字段（必填）：processCode（审批流程编码）、name（展示名称）、iconUrl（图标 URL）、url（跳转链接）；调用方自行设置的字段：dataType（必填，time_range/start_time/recent_time）、recentDays（dataType=recent_time 时有效，7d/30d/1y，默认 30d）、startDate（dataType=time_range/start_time 时有效，yyyy-MM-dd，默认 30 天前）、endDate（dataType=time_range 时有效，yyyy-MM-dd，默认当天）、keepRemovedFields（是否保留已删除字段，默认 false）。约定：syncAll 固定为 true；splitParentTableField 与 enableDataSyncOaDetailList 为下游内部字段，无需传入", Required: true},
		{Name: "auto", Type: shortcut.FlagBool, Desc: "是否开启自动同步，默认 false；创建新数据源表时该字段始终下发给下游"},
		{Name: "field-ids", Type: shortcut.FlagStringSlice, Desc: "需要同步的字段 ID 列表，不传时保持现有配置（创建时默认为全部字段）"},
		{Name: "auto-sync-setting", Type: shortcut.FlagString, Desc: "自动同步频率配置 JSON 字符串，仅在 --auto=true 时生效。字段：syncType（必填，hourly=按小时间隔，scheduled=定时触发）、hourlyInterval（syncType=hourly 时必填，正整数小时）、scheduleType（syncType=scheduled 时必填，daily/weekly/monthly）、timeValue（syncType=scheduled 时必填，HH:mm）、selectedMonthDays（scheduleType=monthly 时必填，每月几号触发，1-31）、selectedWeekdays（scheduleType=weekly 时必填，每周哪几天触发，1=周一…7=周日）、skipNonWorkingDay（可选，默认 false）。不传时使用下游默认自动同步策略"},
	},
	Tips: []string{
		`dws aitable +datasource-create --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
		`dws aitable +datasource-create --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"time_range","startDate":"2025-01-01","endDate":"2025-12-31","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}' --auto`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if _, err := parseJSONObject("source-config", rt.Str("source-config")); err != nil {
			return err
		}
		params := map[string]any{
			"baseId":         rt.Str("base-id"),
			"datasourceType": rt.Str("datasource-type"),
			"sourceConfig":   rt.Str("source-config"),
		}
		params["auto"] = rt.Bool("auto")
		if rt.Changed("field-ids") {
			raw := rt.StrSlice("field-ids")
			cleaned := trimNonEmpty(raw)
			if len(cleaned) == 0 {
				return fmt.Errorf("--field-ids 显式提供时不能为空，如需保持默认请勿传入")
			}
			params["fieldIds"] = cleaned
		}
		if rt.Changed("auto-sync-setting") {
			v := rt.Str("auto-sync-setting")
			if v == "" {
				return fmt.Errorf("--auto-sync-setting 显式提供时不能为空，如需保持默认请勿传入")
			}
			if _, err := parseJSONObject("auto-sync-setting", v); err != nil {
				return err
			}
			params["autoSyncSetting"] = v
		}
		data, err := rt.CallMCPData(serverMain, "create_datasource", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceUpdate 更新已有数据源表的同步配置（update_datasource_config）。
var DatasourceUpdate = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-update",
	Product:     serverMain,
	Description: "更新指定 AI 表格中已有数据源表的同步配置，支持更新源配置、自动同步开关和同步字段选择。更新后触发一次同步。仅适用于数据源表。",
	Intent:      "当用户需要修改已有数据源表的配置（如更换审批模板、调整同步字段、开关自动同步）时使用。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "non_idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_update",
			CanonicalPath:  "aitable.shortcut_datasource_update",
			CLIPath:        "aitable +datasource-update",
			PrimaryCLIPath: "aitable +datasource-update",
		},
		Description: "更新指定 AI 表格中已有数据源表的同步配置，支持更新源配置、自动同步开关和同步字段选择。更新后触发一次同步。仅适用于数据源表。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "更新指定 AI 表格中已有数据源表的同步配置，支持更新源配置、自动同步开关和同步字段选择。更新后触发一次同步。仅适用于数据源表。",
			UseWhen:      []string{"当用户需要修改已有数据源表的配置（如更换审批模板、调整同步字段、开关自动同步）时使用。"},
			AvoidWhen: []string{
				"需要创建新数据源表时（改用 +datasource-create）",
				"仅需触发同步不改配置时（改用 +datasource-sync）",
			},
			Examples: []string{
				`dws aitable +datasource-update --base-id BASE123 --table-id TBL456 --auto`,
				`dws aitable +datasource-update --base-id BASE123 --table-id TBL456 --source-config '{"processCode":"PROC-YYYY","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "已存在的数据源表 ID（通过 +base-get / +table-list 获取，仅允许传入 sync=true 的数据源表）", Required: true},
		{Name: "source-config", Type: shortcut.FlagString, Desc: "可选。新的源配置 JSON 字符串。不传时保持原有配置不变；传入时整体覆盖。字段分为两类：须从 +datasource-list-sources 结果原样透传的字段（必填）：processCode（审批流程编码）、name（展示名称）、iconUrl（图标 URL）、url（跳转链接）；调用方自行设置的字段：dataType（必填，time_range/start_time/recent_time）、recentDays（dataType=recent_time 时有效，7d/30d/1y，默认 30d）、startDate（dataType=time_range/start_time 时有效，yyyy-MM-dd，默认 30 天前）、endDate（dataType=time_range 时有效，yyyy-MM-dd，默认当天）、keepRemovedFields（默认 false）。约定：syncAll 固定为 true；splitParentTableField 与 enableDataSyncOaDetailList 为下游内部字段，无需传入"},
		{Name: "auto", Type: shortcut.FlagBool, Desc: "可选。是否开启自动同步；仅显式设置时下发给下游，省略时保持原有自动同步开关不变"},
		{Name: "field-ids", Type: shortcut.FlagStringSlice, Desc: "需要同步的字段 ID 列表，不传时保持现有配置（创建时默认为全部字段）"},
		{Name: "auto-sync-setting", Type: shortcut.FlagString, Desc: "可选。自动同步频率配置 JSON 字符串，仅在显式设置 --auto=true 时生效；省略时保持原有自动同步频率配置。字段：syncType（必填，hourly=按小时间隔，scheduled=定时触发）、hourlyInterval（syncType=hourly 时必填，正整数小时）、scheduleType（syncType=scheduled 时必填，daily/weekly/monthly）、timeValue（syncType=scheduled 时必填，HH:mm）、selectedMonthDays（scheduleType=monthly 时必填，每月几号触发，1-31）、selectedWeekdays（scheduleType=weekly 时必填，每周哪几天触发，1=周一…7=周日）、skipNonWorkingDay（可选，默认 false）"},
	},
	Tips: []string{
		`dws aitable +datasource-update --base-id BASE123 --table-id TBL456 --auto`,
		`dws aitable +datasource-update --base-id BASE123 --table-id TBL456 --source-config '{"processCode":"PROC-YYYY","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		if rt.Changed("source-config") {
			if _, err := parseJSONObject("source-config", rt.Str("source-config")); err != nil {
				return err
			}
			params["sourceConfig"] = rt.Str("source-config")
		}
		if rt.Changed("auto") {
			params["auto"] = rt.Bool("auto")
		}
		if rt.Changed("field-ids") {
			raw := rt.StrSlice("field-ids")
			cleaned := trimNonEmpty(raw)
			if len(cleaned) == 0 {
				return fmt.Errorf("--field-ids 显式提供时不能为空，如需保持默认请勿传入")
			}
			params["fieldIds"] = cleaned
		}
		if rt.Changed("auto-sync-setting") {
			v := rt.Str("auto-sync-setting")
			if v == "" {
				return fmt.Errorf("--auto-sync-setting 显式提供时不能为空，如需保持默认请勿传入")
			}
			if _, err := parseJSONObject("auto-sync-setting", v); err != nil {
				return err
			}
			params["autoSyncSetting"] = v
		}
		if !rt.Changed("source-config") && !rt.Changed("auto") && !rt.Changed("field-ids") && !rt.Changed("auto-sync-setting") {
			return fmt.Errorf("至少需要一个配置变更：--source-config、--auto、--field-ids 或 --auto-sync-setting；仅触发同步请使用 +datasource-sync")
		}
		data, err := rt.CallMCPData(serverMain, "update_datasource_config", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceSync 对数据源表触发一次手动同步（run_datasource_sync）。
var DatasourceSync = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-sync",
	Product:     serverMain,
	Description: "对指定 AI 表格中的数据源表触发一次手动同步。单次最多 5 张表，每张表独立提交，部分失败不影响其他表。该工具仅触发任务即返回，不会等待同步完成。返回结果包含文档链接，用户可打开文档查看同步进度与最终数据。同步运行中（errorCode=4014）属于幂等冲突，会被标记为 failed 并允许调用方稍后重试。非数据源表（sync=false）不能用此工具触发同步，会以参数错误返回。",
	Intent:      "当用户需要手动触发已有数据源表的同步（而非创建或更新配置）时使用。同步任务 ID 可通过 +datasource-sync-status 查询结果。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "non_idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_sync",
			CanonicalPath:  "aitable.shortcut_datasource_sync",
			CLIPath:        "aitable +datasource-sync",
			PrimaryCLIPath: "aitable +datasource-sync",
		},
		Description: "对指定 AI 表格中的数据源表触发一次手动同步。单次最多 5 张表，每张表独立提交，部分失败不影响其他表。该工具仅触发任务即返回，不会等待同步完成。返回结果包含文档链接，用户可打开文档查看同步进度与最终数据。同步运行中（errorCode=4014）属于幂等冲突，会被标记为 failed 并允许调用方稍后重试。非数据源表（sync=false）不能用此工具触发同步，会以参数错误返回。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "对指定 AI 表格中的数据源表触发一次手动同步。单次最多 5 张表，每张表独立提交，部分失败不影响其他表。该工具仅触发任务即返回，不会等待同步完成。返回结果包含文档链接，用户可打开文档查看同步进度与最终数据。同步运行中（errorCode=4014）属于幂等冲突，会被标记为 failed 并允许调用方稍后重试。非数据源表（sync=false）不能用此工具触发同步，会以参数错误返回。",
			UseWhen:      []string{"当用户需要手动触发已有数据源表的同步（而非创建或更新配置）时使用。同步任务 ID 可通过 +datasource-sync-status 查询结果。"},
			AvoidWhen: []string{
				"需要创建新数据源表时（改用 +datasource-create）",
				"需要更新配置时（改用 +datasource-update）",
			},
			Examples: []string{
				`dws aitable +datasource-sync --base-id BASE123 --table-ids TBL1,TBL2`,
				`dws aitable +datasource-sync --base-id BASE123 --table-ids TBL1`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "table-ids", Type: shortcut.FlagStringSlice, Desc: "待触发同步的数据源表 ID 列表（通过 +base-get / +table-list 获取，仅允许 sync=true 的表，1-5 个）", Required: true},
	},
	Tips: []string{
		`dws aitable +datasource-sync --base-id BASE123 --table-ids TBL1,TBL2`,
		`dws aitable +datasource-sync --base-id BASE123 --table-ids TBL1`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		rawTableIDs := rt.StrSlice("table-ids")
		tableIDs := trimNonEmpty(rawTableIDs)
		if len(tableIDs) < 1 || len(tableIDs) > 5 {
			return fmt.Errorf("--table-ids requires 1-5 table IDs, got %d", len(tableIDs))
		}
		params := map[string]any{
			"baseId":   rt.Str("base-id"),
			"tableIds": tableIDs,
		}
		data, err := rt.CallMCPData(serverMain, "run_datasource_sync", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceSyncStatus 查询数据源表同步任务状态（get_datasource_sync_status）。
var DatasourceSyncStatus = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-sync-status",
	Product:     serverMain,
	Description: "按任务 ID 查询指定数据源表的同步任务状态。与 +datasource-sync / +datasource-create / +datasource-update 配对使用：这些指令触发同步后返回 taskId，本指令通过 taskId 查询最终结果。支持批量查询（单次最多 5 个 taskId），整体仍返回 success；需遍历 tasks 数组按单条 status 判断真实结果。任务状态包括：RUNNING（同步进行中）、FINISHED（同步完成）、FAILED（同步失败）。失败时会返回 errorCode 和 errorMessage 供排查。",
	Intent:      "当用户触发同步后需要按 taskId 查询同步是否完成、成功或失败时使用。支持批量查询（单次最多 5 个任务 ID）。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_sync_status",
			CanonicalPath:  "aitable.shortcut_datasource_sync_status",
			CLIPath:        "aitable +datasource-sync-status",
			PrimaryCLIPath: "aitable +datasource-sync-status",
		},
		Description: "按任务 ID 查询指定数据源表的同步任务状态。与 +datasource-sync / +datasource-create / +datasource-update 配对使用：这些指令触发同步后返回 taskId，本指令通过 taskId 查询最终结果。支持批量查询（单次最多 5 个 taskId），整体仍返回 success；需遍历 tasks 数组按单条 status 判断真实结果。任务状态包括：RUNNING（同步进行中）、FINISHED（同步完成）、FAILED（同步失败）。失败时会返回 errorCode 和 errorMessage 供排查。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按任务 ID 查询指定数据源表的同步任务状态。与 +datasource-sync / +datasource-create / +datasource-update 配对使用：这些指令触发同步后返回 taskId，本指令通过 taskId 查询最终结果。支持批量查询（单次最多 5 个 taskId），整体仍返回 success；需遍历 tasks 数组按单条 status 判断真实结果。任务状态包括：RUNNING（同步进行中）、FINISHED（同步完成）、FAILED（同步失败）。失败时会返回 errorCode 和 errorMessage 供排查。",
			UseWhen:      []string{"当用户触发同步后需要按 taskId 查询同步是否完成、成功或失败时使用。支持批量查询（单次最多 5 个任务 ID）。"},
			AvoidWhen: []string{
				"需要触发同步时（改用 +datasource-sync）",
			},
			Examples: []string{
				`dws aitable +datasource-sync-status --base-id BASE123 --table-id TBL456 --task-ids TASK1,TASK2`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "数据源表 ID（通过 +base-get / +table-list 获取，仅允许传入 sync=true 的表）", Required: true},
		{Name: "task-ids", Type: shortcut.FlagStringSlice, Desc: "待查询的同步任务 ID 列表（由 +datasource-sync / +datasource-create / +datasource-update 返回）。单次最多 5 个，超出请拆分多次调用。", Required: true},
	},
	Tips: []string{
		`dws aitable +datasource-sync-status --base-id BASE123 --table-id TBL456 --task-ids TASK1,TASK2`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		rawTaskIDs := rt.StrSlice("task-ids")
		taskIDs := trimNonEmpty(rawTaskIDs)
		if len(taskIDs) < 1 || len(taskIDs) > 5 {
			return fmt.Errorf("--task-ids requires 1-5 task IDs, got %d", len(taskIDs))
		}
		params["taskIds"] = taskIDs
		data, err := rt.CallMCPData(serverMain, "get_datasource_sync_status", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceGetConfig 获取数据源表的同步配置信息（get_datasource_config）。
var DatasourceGetConfig = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-get-config",
	Product:     serverMain,
	Description: "获取指定数据源表的同步配置信息，包括源配置、是否全量同步、是否自动同步、同步状态等。仅适用于数据源表（sync=true），普通表会返回错误。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。返回的 sourceConfig 包含数据源连接信息（如审批模板 ID、源表 ID 等）。",
	Intent:      "当用户需要查看已有数据源表的配置详情（如确认当前同步的审批模板、字段选择、自动同步状态）时使用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_get_config",
			CanonicalPath:  "aitable.shortcut_datasource_get_config",
			CLIPath:        "aitable +datasource-get-config",
			PrimaryCLIPath: "aitable +datasource-get-config",
		},
		Description: "获取指定数据源表的同步配置信息，包括源配置、是否全量同步、是否自动同步、同步状态等。仅适用于数据源表（sync=true），普通表会返回错误。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。返回的 sourceConfig 包含数据源连接信息（如审批模板 ID、源表 ID 等）。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取指定数据源表的同步配置信息，包括源配置、是否全量同步、是否自动同步、同步状态等。仅适用于数据源表（sync=true），普通表会返回错误。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。返回的 sourceConfig 包含数据源连接信息（如审批模板 ID、源表 ID 等）。",
			UseWhen:      []string{"当用户需要查看已有数据源表的配置详情（如确认当前同步的审批模板、字段选择、自动同步状态）时使用。"},
			AvoidWhen: []string{
				"需要更新配置时（改用 +datasource-update）",
				"需要查询同步任务状态时（改用 +datasource-sync-status）",
			},
			Examples: []string{
				`dws aitable +datasource-get-config --base-id BASE123 --table-id TBL456`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "数据源表 ID（通过 +base-get / +table-list 获取，仅允许传入 sync=true 的表）", Required: true},
	},
	Tips: []string{
		`dws aitable +datasource-get-config --base-id BASE123 --table-id TBL456`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":  rt.Str("base-id"),
			"tableId": rt.Str("table-id"),
		}
		data, err := rt.CallMCPData(serverMain, "get_datasource_config", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceListSources 列出指定数据源类型可用的来源（list_datasource_sources）。
var DatasourceListSources = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-list-sources",
	Product:     serverMain,
	Description: "列出指定 Base 下可用的数据源条目。仅支持 OA 审批数据源（datasourceType=OA）。返回的每条条目包含 result 字段（下游原始 JSON 字符串）和 sourceType 字段（OA 审批对应 2，仅供参考）。OA 审批场景下 result 为包含 approvals 数组的 JSON 字符串，每个 approval 包含 processCode、name、iconUrl、url、keepRemovedFields、splitParentTableField 等字段。须原样透传至 sourceConfig 的字段（仅以下 4 个）：processCode、name、iconUrl、url；调用方自行设置的字段（即使 result 中有值也不透传）：keepRemovedFields、splitParentTableField；enableDataSyncOaDetailList 为下游内部字段，无需传入 sourceConfig。调用方应自行解析 result，提取目标模板字段后构造 sourceConfig 传入 +datasource-create。",
	Intent:      "当用户需要查看某类数据源（如审批）的可用来源信息、获取 result/processCode 以便创建或更新数据源配置时使用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_list_sources",
			CanonicalPath:  "aitable.shortcut_datasource_list_sources",
			CLIPath:        "aitable +datasource-list-sources",
			PrimaryCLIPath: "aitable +datasource-list-sources",
		},
		Description: "列出指定 Base 下可用的数据源条目。仅支持 OA 审批数据源（datasourceType=OA）。返回的每条条目包含 result 字段（下游原始 JSON 字符串）和 sourceType 字段（OA 审批对应 2，仅供参考）。OA 审批场景下 result 为包含 approvals 数组的 JSON 字符串，每个 approval 包含 processCode、name、iconUrl、url、keepRemovedFields、splitParentTableField 等字段。须原样透传至 sourceConfig 的字段（仅以下 4 个）：processCode、name、iconUrl、url；调用方自行设置的字段（即使 result 中有值也不透传）：keepRemovedFields、splitParentTableField；enableDataSyncOaDetailList 为下游内部字段，无需传入 sourceConfig。调用方应自行解析 result，提取目标模板字段后构造 sourceConfig 传入 +datasource-create。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定 Base 下可用的数据源条目。仅支持 OA 审批数据源（datasourceType=OA）。返回的每条条目包含 result 字段（下游原始 JSON 字符串）和 sourceType 字段（OA 审批对应 2，仅供参考）。OA 审批场景下 result 为包含 approvals 数组的 JSON 字符串，每个 approval 包含 processCode、name、iconUrl、url、keepRemovedFields、splitParentTableField 等字段。须原样透传至 sourceConfig 的字段（仅以下 4 个）：processCode、name、iconUrl、url；调用方自行设置的字段（即使 result 中有值也不透传）：keepRemovedFields、splitParentTableField；enableDataSyncOaDetailList 为下游内部字段，无需传入 sourceConfig。调用方应自行解析 result，提取目标模板字段后构造 sourceConfig 传入 +datasource-create。",
			UseWhen:      []string{"当用户需要查看某类数据源（如审批）的可用来源信息、获取 result/processCode 以便创建或更新数据源配置时使用。"},
			AvoidWhen: []string{
				"需要创建数据源表时（改用 +datasource-create）",
				"需要获取数据源字段结构时（改用 +datasource-get-fields）",
			},
			Examples: []string{
				`dws aitable +datasource-list-sources --base-id BASE123 --datasource-type OA`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "datasource-type", Type: shortcut.FlagString, Desc: "数据源类型，目前支持审批（OA）", Required: true},
	},
	Tips: []string{
		`dws aitable +datasource-list-sources --base-id BASE123 --datasource-type OA`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"baseId":         rt.Str("base-id"),
			"datasourceType": rt.Str("datasource-type"),
		}
		data, err := rt.CallMCPData(serverMain, "list_datasource_sources", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

// DatasourceGetFields 获取指定数据源来源的可同步字段列表（get_datasource_fields）。
var DatasourceGetFields = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+datasource-get-fields",
	Product:     serverMain,
	Description: "获取指定数据源下可供同步的字段列表，用于在 +datasource-create / +datasource-update 中决定同步哪些字段。传入从 +datasource-list-sources 获取的 sourceConfig。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。",
	Intent:      "当用户需要查看某数据源来源有哪些可同步字段、以便在创建或更新数据源时指定 field-ids 时使用。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_datasource_get_fields",
			CanonicalPath:  "aitable.shortcut_datasource_get_fields",
			CLIPath:        "aitable +datasource-get-fields",
			PrimaryCLIPath: "aitable +datasource-get-fields",
		},
		Description: "获取指定数据源下可供同步的字段列表，用于在 +datasource-create / +datasource-update 中决定同步哪些字段。传入从 +datasource-list-sources 获取的 sourceConfig。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取指定数据源下可供同步的字段列表，用于在 +datasource-create / +datasource-update 中决定同步哪些字段。传入从 +datasource-list-sources 获取的 sourceConfig。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。",
			UseWhen:      []string{"当用户需要查看某数据源来源有哪些可同步字段、以便在创建或更新数据源时指定 field-ids 时使用。"},
			AvoidWhen: []string{
				"需要列出可用来源时（改用 +datasource-list-sources）",
				"需要创建数据源表时（改用 +datasource-create）",
			},
			Examples: []string{
				`dws aitable +datasource-get-fields --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "目标 Base ID", Required: true},
		{Name: "datasource-type", Type: shortcut.FlagString, Desc: "数据源类型，目前支持审批（OA）", Required: true},
		{Name: "source-config", Type: shortcut.FlagString, Desc: "源配置 JSON 字符串。结构同 +datasource-create 的 --source-config，需含 processCode、name、iconUrl、url、dataType 及对应时间字段", Required: true},
	},
	Tips: []string{
		`dws aitable +datasource-get-fields --base-id BASE123 --datasource-type OA --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if _, err := parseJSONObject("source-config", rt.Str("source-config")); err != nil {
			return err
		}
		params := map[string]any{
			"baseId":         rt.Str("base-id"),
			"datasourceType": rt.Str("datasource-type"),
			"sourceConfig":   rt.Str("source-config"),
		}
		data, err := rt.CallMCPData(serverMain, "get_datasource_fields", params)
		if err != nil {
			return err
		}
		return rt.Output(data)
	},
}

func init() {
	shortcut.Register(withReviewedAITableShortcutContracts(
		DatasourceCreate,
		DatasourceUpdate,
		DatasourceSync,
		DatasourceSyncStatus,
		DatasourceGetConfig,
		DatasourceListSources,
		DatasourceGetFields,
	)...)
}
