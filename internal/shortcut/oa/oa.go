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

// Package oa registers declarative shortcuts for the DingTalk OA approval
// service, wrapping the raw MCP tools exposed by the dws oa helper.
package oa

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ListPending — 查询待我处理的审批 (list_pending_approvals)
var ListPending = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+list-pending",
	Product:     "oa",
	Description: "查询待我处理的审批（时间范围为 epoch 毫秒）",
	Intent:      "当你想知道自己当前有哪些审批还没处理、需要清理审批待办或按时间段/关键字盘点待审批单时使用；传入起止时间（epoch 毫秒，可选关键字与分页），返回待我审批的实例列表，是后续 +get 查详情、+approve/+reject 处理的入口。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagInt, Desc: "开始时间（epoch 毫秒）", Required: true},
		{Name: "end", Type: shortcut.FlagInt, Desc: "结束时间（epoch 毫秒）", Required: true},
		{Name: "page", Type: shortcut.FlagString, Desc: "分页页码"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页大小"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
	},
	Tips: []string{`dws oa +list-pending --start 1741536000000 --end 1741622399000 --query 报销`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"starTime": rt.Int("start"),
			"endTime":  rt.Int("end"),
		}
		if rt.Changed("page") {
			params["pageNum"] = rt.Str("page")
		}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		data, err := rt.CallMCPData("oa", "list_pending_approvals", params)
		if err != nil {
			return err
		}
		instances := listPendingProject(data)
		return rt.Output(map[string]any{"count": len(instances), "instances": instances})
	},
}

// listPendingProject reshapes the raw list_pending_approvals response into the
// same clean {processInstanceId, title, status, createTime} approval list as
// +list-executed/+list-submitted, so all approval-instance listings project
// identically. It reuses the shared oaInstance* defensive probes, tolerating
// response-shape and key-spelling drift.
func listPendingProject(data map[string]any) []map[string]any {
	raw := oaInstanceResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaInstanceProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// Detail — 获取审批实例详情 (get_processInstance_detail)
// Approve — 同意审批 (approve_processInstance)
// Reject — 拒绝审批 (reject_processInstance)
// Revoke — 撤销已发起的审批 (revoke_processInstance)
// Records — 获取审批操作记录 (get_processInstance_records)
// ListInitiated — 查询审批模板下已发起的审批记录 (list_initiated_instances)
// ListTasks — 查询待我审批的任务 ID (list_pending_tasks)
// ListForms — 获取当前用户可见的审批表单列表 (list_user_visible_process)
var ListForms = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+list-forms",
	Product:     "oa",
	Description: "获取当前用户可见的审批表单列表",
	Intent:      "当你想浏览自己有权限发起哪些审批模板、或需要为 +list-initiated 等操作枚举 processCode 时使用；无需关键字，按游标分页返回当前用户可见的全部审批表单，适合不确定表单名称时先整体看一遍。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "分页游标，首次传 0"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "每页大小，最大 100"},
	},
	Tips: []string{`dws oa +list-forms --cursor 0 --limit 100`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("oa", "list_user_visible_process", map[string]any{
			"cursor":   rt.Int("cursor"),
			"pageSize": rt.Int("limit"),
		})
		if err != nil {
			return err
		}
		forms := listFormsProject(data)
		return rt.Output(map[string]any{"count": len(forms), "forms": forms})
	},
}

// listFormsProject reshapes the raw list_user_visible_process response into a
// clean approval-form list ({processCode, name, iconUrl}) — the output-projection
// fidelity the framework applies to every list command. The list container and
// per-item field names are probed defensively across candidate keys so the
// projection tolerates response-shape drift rather than crashing or fabricating.
func listFormsProject(data map[string]any) []map[string]any {
	raw := oaFormResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaFormProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// oaFormResolveList locates the form list inside a response, tolerating a bare
// top-level array or nesting one level under a common envelope key.
func oaFormResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"result", "data", "list", "items", "processList", "forms"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "processList", "forms", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// oaFormProjectItem picks the stable identity/label fields of a single approval
// form, probing candidate key spellings so it survives snake/camel drift.
func oaFormProjectItem(m map[string]any) map[string]any {
	row := map[string]any{}
	if v, ok := oaFormFirst(m, "processCode", "process_code", "code"); ok {
		row["processCode"] = v
	}
	if v, ok := oaFormFirst(m, "name", "processName", "process_name", "flowTitle"); ok {
		row["name"] = v
	}
	if v, ok := oaFormFirst(m, "iconUrl", "icon_url", "iconName"); ok {
		row["iconUrl"] = v
	}
	return row
}

// oaFormFirst returns the first present candidate key's value.
func oaFormFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// SearchForms — 按关键字模糊搜索可见审批表单 (search_form)
var SearchForms = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+search-forms",
	Product:     "oa",
	Description: "按关键字模糊搜索当前用户可见的审批表单",
	Intent:      "当你已知想找的审批大致名称（如「报销」「请假」）、想快速定位对应表单及其 processCode 时使用，比 +list-forms 全量列举更高效；传入关键字，返回名称或 processCode 匹配的表单，供后续 +list-initiated 按模板查询。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字（匹配 processCode 或表单名称）", Required: true},
	},
	Tips: []string{`dws oa +search-forms --query 报销`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("oa", "search_form", map[string]any{
			"query": rt.Str("query"),
		})
		if err != nil {
			return err
		}
		forms := searchFormsProject(data)
		return rt.Output(map[string]any{"count": len(forms), "forms": forms})
	},
}

// searchFormsProject reshapes the raw search_form response into the same clean
// {processCode, name, iconUrl} list as +list-forms, so both approval-form
// listings project identically. It reuses the shared oaForm* defensive probes,
// tolerating response-shape and key-spelling drift.
func searchFormsProject(data map[string]any) []map[string]any {
	raw := oaFormResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaFormProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// DingInfo — 获取审批任务的被催办人 userId (oa_ding_user)
// ListExecuted — 获取当前用户已处理过的审批单列表 (get_done_tasks)
var ListExecuted = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+list-executed",
	Product:     "oa",
	Description: "获取当前用户已经处理过的审批单列表",
	Intent:      "当你想回顾自己历史上审批过（已同意/拒绝等）的单子、做复盘或查找某条已办审批时使用，区别于 +list-pending 的待办；按页码/关键字分页返回我已处理的审批单。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "分页页码"},
		{Name: "limit", Type: shortcut.FlagString, Default: "20", Desc: "每页大小"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
	},
	Tips: []string{`dws oa +list-executed --limit 20 --page 1 --query 报销`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"pageNumber": rt.Str("page"),
			"pageSize":   rt.Str("limit"),
		}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		data, err := rt.CallMCPData("oa", "get_done_tasks", params)
		if err != nil {
			return err
		}
		instances := listExecutedProject(data)
		return rt.Output(map[string]any{"count": len(instances), "instances": instances})
	},
}

// listExecutedProject reshapes the raw get_done_tasks response into a clean
// approval-instance list ({processInstanceId, title, status, createTime}) — the
// the clean output projection applied to every list command. The
// list container and per-item field names are probed defensively across
// candidate keys so the projection tolerates response-shape drift rather than
// crashing or fabricating data.
func listExecutedProject(data map[string]any) []map[string]any {
	raw := oaInstanceResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaInstanceProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// oaInstanceResolveList locates the approval-instance list inside a response,
// tolerating a bare top-level array or nesting one level under a common
// envelope key.
func oaInstanceResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"result", "data", "list", "items", "instances", "tasks"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "instances", "tasks", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// oaInstanceProjectItem picks the stable identity/label fields of a single
// approval instance, probing candidate key spellings so it survives
// snake/camel drift.
func oaInstanceProjectItem(m map[string]any) map[string]any {
	row := map[string]any{}
	if v, ok := oaFormFirst(m, "processInstanceId", "process_instance_id", "instanceId", "id"); ok {
		row["processInstanceId"] = v
	}
	if v, ok := oaFormFirst(m, "title", "processTitle", "process_title", "name"); ok {
		row["title"] = v
	}
	if v, ok := oaFormFirst(m, "status", "result", "processResult"); ok {
		row["status"] = v
	}
	if v, ok := oaFormFirst(m, "createTime", "create_time", "gmtCreate", "createdTime"); ok {
		row["createTime"] = v
	}
	return row
}

// ListSubmitted — 获取当前用户已发起的审批单列表 (get_submitted_instances)
var ListSubmitted = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+list-submitted",
	Product:     "oa",
	Description: "获取当前用户已发起的审批单列表",
	Intent:      "当你想查看自己提交发起的审批单及其审批进度（如某笔报销/请假审到哪一步）时使用；按页码/关键字分页返回我发起的审批单，可据此决定是否 +revoke 撤销或催办。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "分页页码"},
		{Name: "limit", Type: shortcut.FlagString, Default: "20", Desc: "每页大小"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
	},
	Tips: []string{`dws oa +list-submitted --limit 20 --page 1 --query 报销`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"pageNumber": rt.Str("page"),
			"pageSize":   rt.Str("limit"),
		}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		data, err := rt.CallMCPData("oa", "get_submitted_instances", params)
		if err != nil {
			return err
		}
		instances := listSubmittedProject(data)
		return rt.Output(map[string]any{"count": len(instances), "instances": instances})
	},
}

// listSubmittedProject reshapes the raw get_submitted_instances response into
// the same clean {processInstanceId, title, status, createTime} approval list
// as +list-executed, so both instance listings project identically. It reuses
// the shared oaInstance* defensive probes, tolerating response-shape and
// key-spelling drift.
func listSubmittedProject(data map[string]any) []map[string]any {
	raw := oaInstanceResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaInstanceProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ListCc — 获取抄送当前用户的审批单列表 (get_noticed_instances)
var ListCc = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+list-cc",
	Product:     "oa",
	Description: "获取抄送当前用户的审批单列表",
	Intent:      "当你想查看抄送给自己、需要知悉但无需审批的单子时使用；按页码/关键字分页返回抄送我的审批单列表，适合了解与自己相关但不用自己动手处理的审批动态。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "分页页码"},
		{Name: "limit", Type: shortcut.FlagString, Default: "20", Desc: "每页大小"},
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索"},
	},
	Tips: []string{`dws oa +list-cc --limit 20 --page 1 --query 报销`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"pageNumber": rt.Str("page"),
			"pageSize":   rt.Str("limit"),
		}
		if rt.Changed("query") {
			params["query"] = rt.Str("query")
		}
		data, err := rt.CallMCPData("oa", "get_noticed_instances", params)
		if err != nil {
			return err
		}
		instances := listCcProject(data)
		return rt.Output(map[string]any{"count": len(instances), "instances": instances})
	},
}

// listCcProject reshapes the raw get_noticed_instances response into the same
// clean {processInstanceId, title, status, createTime} approval list as the
// other instance listings, so all approval-instance listings project
// identically. It reuses the shared oaInstance* defensive probes, tolerating
// response-shape and key-spelling drift.
func listCcProject(data map[string]any) []map[string]any {
	raw := oaInstanceResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := oaInstanceProjectItem(m); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// RedirectTask — 转交审批任务给其他人 (redirect_task)
// Comment — 对审批实例添加评论 (dingflow_comments)
// CcNotice — 对审批实例进行抄送 (oa_cc_noticer)
// AppendTask — 对审批任务进行加签 (append_task)
// RevertActivities — 获取审批任务可回退的节点信息 (get_inst_revert_activities)
// RevertTask — 退回审批任务到指定节点 (revert_task)
func init() {
	shortcut.Register(
		ListPending,
		ListForms,
		SearchForms,
		ListExecuted,
		ListSubmitted,
		ListCc,
	)
}
