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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// DoneApprovals: READ-ONLY list of the approval tasks I have already processed
// (my approval history), projected to clean fields. Unlike `oa +approve-by`
// (which agrees to an approval) or `oa +pending` (which lists what still waits
// for me), this shortcut only looks back at what I've already handled — it never
// approves / rejects / mutates anything.
//
// Steps (tool name + param keys copied verbatim from helpers/oa.go):
//
//  1. list my done approvals via get_done_tasks with pageNumber / pageSize as
//     float64, mirroring `dws oa approval list-executed`; optional --limit maps
//     to pageSize (float64). This tool takes NO time window (unlike
//     list_pending_approvals) — only pageNumber/pageSize/query.
//
//  2. defensively unwrap the returned instance list (reusing shortcutApproveItems
//     from approve.go, multiple candidate container keys + one nested level) and
//     project each entry to a readable shape {title, originatorName,
//     processInstanceId, createTime}, every field probed across candidate keys.
//
//  3. if nothing has been processed, report "没有已处理的审批记录" instead of an
//     empty raw dump.
//
//     dws oa +done-approvals
//     dws oa +done-approvals --limit 10
var DoneApprovals = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+done-approvals",
	Product:     "oa",
	Description: "只读列出我已处理过的审批任务（审批历史）并投影为可读列表",
	Intent: "当你只想快速回看「我已经处理过（同意/拒绝）」的审批任务历史——每条的标题、发起人、审批实例 ID 和创建时间——" +
		"而不想拿到一大坨原始字段时使用；内部拉取你的已办审批单，再在本地投影出可读字段。" +
		"这是纯只读操作，只做列出与本地投影，绝不会同意、拒绝或以任何方式提交/修改任何审批；" +
		"若没有任何已处理记录则提示「没有已处理的审批记录」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Desc: "最多列出多少条（可选）", Required: false},
	},
	Tips: []string{
		`dws oa +done-approvals`,
		`dws oa +done-approvals --limit 10`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my done approvals. pageNumber/pageSize are float64. Keys
		// copied verbatim from helpers.list-executed (get_done_tasks). This tool
		// carries no time window; pageNumber defaults to the first page.
		// pageSize is required by get_done_tasks — the 1:1 list-executed always
		// sends one (a missing/zero pageSize triggers a backend business error),
		// so default to 20 and let --limit override.
		params := map[string]any{
			"pageNumber": float64(1),
			"pageSize":   float64(20),
		}
		if rt.Changed("limit") {
			if n := rt.Int("limit"); n > 0 {
				params["pageSize"] = float64(n)
			}
		}
		data, err := rt.CallMCPData("oa", "get_done_tasks", params)
		if err != nil {
			return err
		}

		// Step 2 — project entries. shortcutApproveItems (approve.go) defensively
		// unwraps the list container (result/list/instances/items/data/records/
		// processList, incl. one nested level).
		items := shortcutApproveItems(data)
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				// shortcutApproveTitle probes title/processTitle/name/subject/…
				"title": shortcutApproveTitle(m),
				// shortcutApproveInstanceID probes processInstanceId/instanceId/…
				"processInstanceId": shortcutApproveInstanceID(m),
				"originatorName":    pendingApprovalsOriginator(m),
				"createTime":        pendingApprovalsCreateTime(m),
			})
		}

		// Step 3 — empty result guard.
		if len(results) == 0 {
			return apperrors.NewValidation("没有已处理的审批记录")
		}

		return rt.Output(map[string]any{"count": len(results), "done": results})
	},
}

func init() {
	shortcut.Register(DoneApprovals)
}
