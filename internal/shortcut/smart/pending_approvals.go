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
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// PendingApprovals: READ-ONLY list of the approvals waiting for ME, projected to
// clean fields. Unlike `oa +approve-by` (which actually agrees to an approval),
// this shortcut only looks — it never approves / rejects / mutates anything.
//
// Steps (tool name + param keys copied verbatim from helpers/oa.go):
//
//  1. list my pending approvals via list_pending_approvals with starTime /
//     endTime as float64 milliseconds over a recent (~90 day) window, mirroring
//     `dws oa approval list-pending`; optional --limit maps to pageSize (float64).
//
//  2. defensively unwrap the returned instance list (multiple candidate
//     container keys, one nested level) and project each entry to a readable
//     shape {title, originatorName, processInstanceId, createTime} — every field
//     probed across several candidate keys.
//
//  3. if nothing is pending, report "当前没有待我审批的任务" instead of an empty
//     raw dump.
//
//     dws oa +pending
//     dws oa +pending --limit 10
var PendingApprovals = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+pending",
	Product:     "oa",
	Description: "只读列出待我审批的审批任务并投影为可读列表（只看不批）",
	Intent: "当你只想快速看一眼「待我审批」的审批任务清单——每条的标题、发起人、审批实例 ID 和创建时间——" +
		"而不想拿到一大坨原始字段时使用；内部拉取你近三个月待处理的审批单，再在本地投影出可读字段。" +
		"这是纯只读操作，只做列出与本地投影，绝不会同意、拒绝或以任何方式提交/修改审批（要一键通过请改用 `dws oa +approve-by`）；" +
		"若当前没有待你审批的任务则提示「当前没有待我审批的任务」。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Desc: "最多列出多少条（可选）", Required: false},
	},
	Tips: []string{
		`dws oa +pending`,
		`dws oa +pending --limit 10`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my pending approvals. starTime/endTime are float64
		// milliseconds; pageSize is optional. Keys copied verbatim from
		// helpers.list-pending (list_pending_approvals). Window: last ~90 days.
		now := time.Now()
		params := map[string]any{
			"starTime": float64(now.AddDate(0, 0, -90).UnixMilli()),
			"endTime":  float64(now.UnixMilli()),
		}
		if rt.Changed("limit") {
			if n := rt.Int("limit"); n > 0 {
				params["pageSize"] = float64(n)
			}
		}
		data, err := rt.CallMCPData("oa", "list_pending_approvals", params)
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
			return apperrors.NewValidation("当前没有待我审批的任务")
		}

		return rt.Output(map[string]any{"count": len(results), "pending": results})
	},
}

// pendingApprovalsOriginator reads the initiator's display name, tolerating the
// common field names the gateway may use.
func pendingApprovalsOriginator(m map[string]any) string {
	for _, key := range []string{"originatorName", "originatorUserName", "creatorName", "creator", "applicantName", "userName"} {
		if s := shortcutApproveStr(m[key]); s != "" {
			return s
		}
	}
	return ""
}

// pendingApprovalsCreateTime reads a pending approval's create time, returning
// the raw value (usually epoch millis) under whichever candidate key is present.
func pendingApprovalsCreateTime(m map[string]any) any {
	for _, key := range []string{"createTime", "gmtCreate", "createTimeStr", "startTime", "createdAt"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(PendingApprovals)
}
