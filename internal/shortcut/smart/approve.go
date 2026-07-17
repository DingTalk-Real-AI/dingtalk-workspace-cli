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

package smart

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Approve: agree to ONE of MY pending OA approvals by matching a keyword in its
// title / order number, in a single command (钉钉原生编排能力).
//
// Steps (all tool names + params copied verbatim from helpers/oa.go):
//
//  1. list my pending approvals via list_pending_approvals (starTime / endTime
//     as float64 millis over a recent window, plus query=keyword for
//     server-side narrowing — mirroring `dws oa approval list-pending`);
//
//  2. scan the returned instances and keep the ones whose title / order number
//     contains --keyword: none → "没找到待审批单据", many → list candidates
//     (标题 + processInstanceId) so the caller can be more specific, exactly one
//     → take its processInstanceId;
//
//  3. resolve the pending taskId for that instance via list_pending_tasks
//     (processInstanceId), then agree via approve_processInstance
//     (processInstanceId + taskId as float64, optional remark) —
//     mirroring `dws oa approval tasks` + `dws oa approval approve`.
//
//     dws oa +approve-by --keyword 报销
//     dws oa +approve-by --keyword 出差单 --comment "同意"
var Approve = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+approve-by",
	Product:     "oa",
	Description: "按关键词把我的一条待审批单据一键通过（自动定位实例与任务 ID）",
	Intent: "当你只记得某张待你审批单据的标题或单号关键词、想直接把它审批通过，却不想先翻待办列表、" +
		"复制审批实例 ID 再查任务 ID 时使用；内部先拉取你近三个月待处理的审批单，按标题/单号包含关键词匹配：" +
		"没匹配到会提示「没找到待审批单据」，匹配到多条会列出候选(标题+实例ID)让你写得更精确，" +
		"唯一命中时先取出它的待审批任务 ID，再以「同意」提交。这会真实提交审批决策且不可撤回，请确认关键词足够精确。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "keyword", Type: shortcut.FlagString, Desc: "待审批单据的单号或标题关键词", Required: true},
		{Name: "comment", Type: shortcut.FlagString, Desc: "审批意见（可选）", Required: false},
	},
	Tips: []string{
		`dws oa +approve-by --keyword 报销`,
		`dws oa +approve-by --keyword 出差单 --comment "同意"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		keyword := strings.TrimSpace(rt.Str("keyword"))
		if keyword == "" {
			return apperrors.NewValidation("请用 --keyword 提供待审批单据的单号或标题关键词")
		}

		// Step 1 — list my pending approvals. starTime/endTime are float64
		// milliseconds and query narrows server-side, mirroring
		// helpers.list-pending (list_pending_approvals). Window: last ~90 days.
		now := time.Now()
		listArgs := map[string]any{
			"starTime": float64(now.AddDate(0, 0, -90).UnixMilli()),
			"endTime":  float64(now.UnixMilli()),
			"query":    keyword,
		}
		data, err := rt.CallMCPData("oa", "list_pending_approvals", listArgs)
		if err != nil {
			return err
		}

		// Step 2 — match by title / order number substring.
		matches := shortcutApproveMatch(data, keyword)
		switch {
		case len(matches) == 0:
			return apperrors.NewValidation(fmt.Sprintf(
				"没找到待审批单据：待我处理的审批里没有标题/单号包含 %q 的单据。", keyword))
		case len(matches) > 1:
			return apperrors.NewValidation(fmt.Sprintf(
				"%q 匹配到 %d 条待审批单据，请用更精确的关键词，或用 `dws oa approval approve --instance-id --task-id` 指定：%s",
				keyword, len(matches), strings.Join(shortcutApproveLabels(matches), "；")))
		}

		instanceID := matches[0].instanceID

		// Step 3a — resolve the pending taskId for this instance via
		// list_pending_tasks (processInstanceId), mirroring `dws oa approval tasks`.
		tasksData, err := rt.CallMCPData("oa", "list_pending_tasks", map[string]any{
			"processInstanceId": instanceID,
		})
		if err != nil {
			return err
		}
		taskID := shortcutApproveTaskID(tasksData)
		if taskID == "" {
			return apperrors.NewValidation(fmt.Sprintf(
				"没能拿到 %q 的待审批任务 ID，请用 `dws oa approval tasks --instance-id %s` 查看后手动 approve。",
				matches[0].title, instanceID))
		}

		// Step 3b — agree. taskId is passed as float64 and remark is optional,
		// mirroring helpers.approve (approve_processInstance).
		taskIDNum, err := strconv.ParseFloat(taskID, 64)
		if err != nil {
			return apperrors.NewValidation(fmt.Sprintf("任务 ID %q 不是合法数字，无法审批。", taskID))
		}
		approveArgs := map[string]any{
			"processInstanceId": instanceID,
			"taskId":            taskIDNum,
		}
		if c := strings.TrimSpace(rt.Str("comment")); c != "" {
			approveArgs["remark"] = c
		}
		return rt.CallMCP("approve_processInstance", approveArgs)
	},
}

// shortcutApproveInstance is the minimal identity we need from a pending-approval
// entry: its processInstanceId (fed to approve/tasks) and a human title/order
// number for matching + disambiguation.
type shortcutApproveInstance struct {
	instanceID string
	title      string
}

// shortcutApproveMatch walks a list_pending_approvals response and returns the
// instances whose title / order number contains keyword (case-insensitive). The
// gateway wraps the list under one of several common container keys, so we probe
// them defensively (mirroring latestMinutesItems / shortcutTodoCards).
func shortcutApproveMatch(data map[string]any, keyword string) []shortcutApproveInstance {
	items := shortcutApproveItems(data)
	kw := strings.ToLower(keyword)
	var out []shortcutApproveInstance
	seen := map[string]bool{}
	for _, m := range items {
		id := shortcutApproveInstanceID(m)
		if id == "" || seen[id] {
			continue
		}
		title := shortcutApproveTitle(m)
		// Match on either the visible title/order number or the instance id.
		if !strings.Contains(strings.ToLower(title), kw) &&
			!strings.Contains(strings.ToLower(id), kw) {
			continue
		}
		seen[id] = true
		if title == "" {
			title = id
		}
		out = append(out, shortcutApproveInstance{instanceID: id, title: title})
	}
	return out
}

// shortcutApproveItems pulls the list of pending-approval entries out of the
// response, probing common container shapes before scanning for a bare list.
func shortcutApproveItems(data map[string]any) []map[string]any {
	for _, key := range []string{"result", "list", "instances", "items", "data", "records", "processList"} {
		if arr, ok := data[key].([]any); ok {
			return shortcutApproveToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "instances", "items", "records", "result", "processList"} {
				if arr, ok := inner[k2].([]any); ok {
					return shortcutApproveToMaps(arr)
				}
			}
		}
	}
	return nil
}

func shortcutApproveToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutApproveInstanceID reads an entry's processInstanceId (the value the
// helper feeds to --instance-id), tolerating a few common field names and both
// string / numeric JSON encodings.
func shortcutApproveInstanceID(m map[string]any) string {
	for _, key := range []string{"processInstanceId", "processInstanceID", "instanceId", "businessId"} {
		if s := shortcutApproveStr(m[key]); s != "" {
			return s
		}
	}
	return ""
}

// shortcutApproveTitle reads a human-facing title / order number for matching
// and disambiguation, probing common field names.
func shortcutApproveTitle(m map[string]any) string {
	for _, key := range []string{"title", "processTitle", "name", "subject", "businessId", "processCode"} {
		if s := shortcutApproveStr(m[key]); s != "" {
			return s
		}
	}
	return ""
}

// shortcutApproveTaskID walks a list_pending_tasks response and returns the
// first pending taskId (the value the helper feeds to --task-id).
func shortcutApproveTaskID(data map[string]any) string {
	items := shortcutApproveItems(data)
	// list_pending_tasks may also return a bare {"result": [...]}/task list; the
	// probe above covers those. Some gateways return a single task object.
	if len(items) == 0 {
		if id := shortcutApproveTaskIDField(data); id != "" {
			return id
		}
		if r, ok := data["result"].(map[string]any); ok {
			if id := shortcutApproveTaskIDField(r); id != "" {
				return id
			}
		}
		return ""
	}
	for _, m := range items {
		if id := shortcutApproveTaskIDField(m); id != "" {
			return id
		}
	}
	return ""
}

func shortcutApproveTaskIDField(m map[string]any) string {
	for _, key := range []string{"taskId", "taskID", "activityId"} {
		if s := shortcutApproveStr(m[key]); s != "" {
			return s
		}
	}
	return ""
}

// shortcutApproveStr coerces a JSON value into a string, tolerating string and
// numeric encodings (taskId / instance id can arrive as either).
func shortcutApproveStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	}
	return ""
}

func shortcutApproveLabels(items []shortcutApproveInstance) []string {
	out := make([]string, 0, len(items))
	for _, c := range items {
		out = append(out, fmt.Sprintf("%s(instanceId=%s)", c.title, c.instanceID))
	}
	return out
}

func init() {
	shortcut.Register(Approve)
}
