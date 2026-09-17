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
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// MyAttendance: show MY punch-in (打卡流水) records for TODAY in one step.
//
// Steps:
//  1. resolve the current logged-in user's userId via the contact server's
//     zero-arg get_current_user_profile (no --name needed — it's always "me");
//  2. compute today's window [00:00 today, 00:00 tomorrow) in the local
//     timezone and format both bounds as "yyyy-MM-dd HH:mm:ss" (the format the
//     query_check_record helper feeds the tool);
//  3. call query_check_record on the attendance-wukong server with the exact
//     nested QueryCheckRecordRequest shape used by
//     `dws attendance check record`, then print via rt.Output.
//
// This replaces the manual dance of looking up your own userId, then running
// `dws attendance check record --users <id> --start <today> --end <today>`.
// Read-only; it never modifies any attendance data.
//
//	dws attendance +my-attendance
var MyAttendance = shortcut.Shortcut{
	OutputRollout: output.RolloutLegacyOnly,
	Service:       "attendance",
	Command:       "+my-attendance",
	Product:       "attendance",
	Description:   "查我今天的考勤打卡记录（打卡流水，自动解析当前用户）",
	Intent: "当你想快速看自己今天的打卡流水（几点上下班打卡、打卡地址/定位方式）、又不想先查自己的 userId " +
		"再手动填写今天的起止时间时使用；内部先取当前登录用户的 userId，再按本地时区算出今天 00:00 到次日 00:00 的时间窗，" +
		"最后查询你今天的打卡流水记录。只读操作，不会修改任何考勤数据；今天若还没有任何打卡则返回空结果。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "attendance",
			Name:           "shortcut_my_attendance",
			CanonicalPath:  "attendance.shortcut_my_attendance",
			CLIPath:        "attendance +my-attendance",
			PrimaryCLIPath: "attendance +my-attendance",
		},
		Description: "查我今天的考勤打卡记录（打卡流水，自动解析当前用户）",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "Historical executable Schema compatibility: this command remains callable, but the reviewed Shortcut catalog keeps it non-public until a known-nonempty current-day fixture closes the false-empty proof gap.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查我今天的考勤打卡记录（打卡流水，自动解析当前用户）",
			UseWhen:      []string{"当你想快速看自己今天的打卡流水（几点上下班打卡、打卡地址/定位方式）、又不想先查自己的 userId 再手动填写今天的起止时间时使用；内部先取当前登录用户的 userId，再按本地时区算出今天 00:00 到次日 00:00 的时间窗，最后查询你今天的打卡流水记录。只读操作，不会修改任何考勤数据；今天若还没有任何打卡则返回空结果。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws attendance +my-attendance"},
		},
	},
	Flags: []shortcut.Flag{},
	Tips: []string{
		`dws attendance +my-attendance`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the current user's own userId (zero-arg "我的" API).
		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return err
		}
		userID := strictAttendanceCurrentUserID(profile)
		if userID == "" {
			return apperrors.NewValidation(
				"没能解析出当前登录用户的 userId，无法查询你的打卡记录；请确认已登录后重试。")
		}

		// Step 2 — today's window [00:00 today, 00:00 tomorrow) in local time,
		// formatted as the tool expects (yyyy-MM-dd HH:mm:ss).
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		end := start.AddDate(0, 0, 1)
		const layout = "2006-01-02 15:04:05"

		// Step 3 — query my punch-in records for today. The nested
		// QueryCheckRecordRequest shape + field names mirror the
		// `dws attendance check record` helper exactly.
		data, err := rt.CallMCPData("attendance-wukong", "query_check_record", map[string]any{
			"QueryCheckRecordRequest": map[string]any{
				"userIds":       []string{userID},
				"checkDateFrom": start.Format(layout),
				"checkDateTo":   end.Format(layout),
			},
		})
		if err != nil {
			return err
		}
		return outputStrictAttendanceRecords(rt, data)
	},
}

// strictAttendanceCurrentUserID extracts the current user's userId from a
// get_current_user_profile response. The gateway returns one of several shapes,
// so probe them defensively (mirrors helpers.getCurrentUserID):
//   - {"result": [ {"orgEmployeeModel": {"userId": ...}} ]}
//   - {"result": {"userId": ...}}
func strictAttendanceCurrentUserID(data map[string]any) string {
	if data == nil {
		return ""
	}
	success, ok := data["success"].(bool)
	if !ok || !success {
		return ""
	}
	identities := map[string]struct{}{}
	collect := func(item map[string]any) bool {
		var direct, nested string
		if value, present := item["userId"]; present {
			text, valid := value.(string)
			if !valid || strings.TrimSpace(text) == "" {
				return false
			}
			direct = strings.TrimSpace(text)
		}
		if value, present := item["orgEmployeeModel"]; present {
			object, valid := value.(map[string]any)
			if !valid || object == nil {
				return false
			}
			text, valid := object["userId"].(string)
			if !valid || strings.TrimSpace(text) == "" {
				return false
			}
			nested = strings.TrimSpace(text)
		}
		if direct == "" && nested == "" {
			return false
		}
		if direct != "" && nested != "" && direct != nested {
			return false
		}
		if direct != "" {
			identities[direct] = struct{}{}
		} else {
			identities[nested] = struct{}{}
		}
		return true
	}
	switch result := data["result"].(type) {
	case []any:
		if len(result) == 0 {
			return ""
		}
		for _, item := range result {
			m, ok := item.(map[string]any)
			if !ok || !collect(m) {
				return ""
			}
		}
	case map[string]any:
		if !collect(result) {
			return ""
		}
	default:
		return ""
	}
	if len(identities) != 1 {
		return ""
	}
	var identity string
	for candidate := range identities {
		identity = candidate
	}
	return identity
}

// myAttendanceCurrentUserID preserves the compatibility resolver used by
// older non-Attendance smart workflows. New Attendance shortcuts must use the
// strict resolver above so a missing business success flag cannot consume a
// stale result.
func myAttendanceCurrentUserID(data map[string]any) string {
	if data == nil {
		return ""
	}
	switch result := data["result"].(type) {
	case []any:
		for _, item := range result {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if oem, ok := m["orgEmployeeModel"].(map[string]any); ok {
				if uid, ok := oem["userId"].(string); ok && uid != "" {
					return uid
				}
			}
			if uid, ok := m["userId"].(string); ok && uid != "" {
				return uid
			}
		}
	case map[string]any:
		if uid, ok := result["userId"].(string); ok && uid != "" {
			return uid
		}
		if oem, ok := result["orgEmployeeModel"].(map[string]any); ok {
			if uid, ok := oem["userId"].(string); ok && uid != "" {
				return uid
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(MyAttendance)
}
