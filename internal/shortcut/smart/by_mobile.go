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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ByMobile: find a person by phone number and return their full profile in one
// step.
//
// Steps: look the mobile up via search_user_by_mobile → resolve to a userId
// (error clearly if nobody is bound to that number) → fetch full detail via
// get_user_info_by_user_ids. Replaces `contact user search-mobile` (copy the
// userId) → `contact user get --ids <id>`.
//
//	dws contact +by-mobile --mobile 13800138000
var ByMobile = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+by-mobile",
	Product:     "contact",
	Description: "按手机号查询某人的完整资料（自动解析 userId 后取详情）",
	Intent: "当你只知道对方手机号、想一步拿到其完整资料（部门、职位、联系方式、是否管理员等）而不想先按手机号搜出 userId 再单独查详情时使用；" +
		"内部先用手机号在通讯录里查出对应的 userId，若没有人绑定该手机号会明确报错，再用该 userId 取完整详情。这是纯只读操作，不会修改任何数据。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "mobile", Type: shortcut.FlagString, Desc: "手机号", Required: true},
	},
	Tips: []string{`dws contact +by-mobile --mobile 13800138000`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := rt.RequireAll("mobile"); err != nil {
			return err
		}
		mobile := rt.Str("mobile")

		// Step 1 — look up the userId bound to this mobile number.
		// search_user_by_mobile takes {mobile} (see helpers.contact
		// search-mobile). The gateway shape is not guaranteed, so probe common
		// containers/field names defensively.
		data, err := rt.CallMCPData("contact", "search_user_by_mobile", map[string]any{
			"mobile": mobile,
		})
		if err != nil {
			return err
		}
		userID := byMobileExtractUserID(data)
		if userID == "" {
			return apperrors.NewValidation(
				fmt.Sprintf("没找到绑定手机号 %q 的用户；确认号码正确、且该人在当前组织通讯录中。", mobile))
		}

		// Step 2 — fetch and print the full profile of the resolved user.
		// get_user_info_by_user_ids takes {user_id_list} (see helpers.contact
		// user get).
		return rt.CallMCP("get_user_info_by_user_ids", map[string]any{
			"user_id_list": []string{userID},
		})
	},
}

// byMobileExtractUserID pulls a single userId out of a search_user_by_mobile
// response. The exact shape is not contractually fixed, so it probes a few
// likely containers (top-level, result, data, user) and field names
// (userId/userid/user_id), tolerating both string and numeric encodings.
func byMobileExtractUserID(data map[string]any) string {
	if data == nil {
		return ""
	}
	if id := byMobileUserIDFromMap(data); id != "" {
		return id
	}
	for _, key := range []string{"result", "data", "user", "userInfo"} {
		switch v := data[key].(type) {
		case map[string]any:
			if id := byMobileUserIDFromMap(v); id != "" {
				return id
			}
		case []any:
			for _, it := range v {
				if m, ok := it.(map[string]any); ok {
					if id := byMobileUserIDFromMap(m); id != "" {
						return id
					}
				}
			}
		case string:
			if v != "" {
				return v
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

func byMobileUserIDFromMap(m map[string]any) string {
	for _, key := range []string{"userId", "userid", "user_id", "userID", "id"} {
		switch v := m[key].(type) {
		case string:
			if v != "" && v != "0" {
				return v
			}
		case float64:
			if v != 0 {
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(ByMobile)
}
