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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Whoami: print the CURRENT user's own profile as a clean projection — the
// "who am I" every agent needs before resolving other people. Unlike the 1:1
// +get-self (which dumps the verbose raw {result:[{orgEmployeeModel:{…}}]}),
// this projects to {name, userId, mobile, dept, org, email}.
//
// It calls the zero-arg get_current_user_profile (always "me", no --name),
// mirroring the resolution +my-attendance already relies on.
//
//	dws contact +me
var Whoami = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+me",
	Product:     "contact",
	Description: "查看我自己的通讯录资料（姓名/userId/手机/部门/组织，干净投影）",
	Intent: "当你（或 AI agent）需要知道「我是谁」——我自己的 userId、姓名、所在部门、组织、手机号，用于后续按名解析他人前先确定自己身份、或填充发起人信息时使用；" +
		"内部调用零参数的 get_current_user_profile（永远是「我」，无需传姓名），再把冗长的原始资料投影成 {name,userId,mobile,dept,org,email} 几个关键字段。" +
		"这是纯只读操作，不修改任何资料。",
	Risk: shortcut.RiskRead,
	Tips: []string{
		`dws contact +me`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return err
		}
		return rt.Output(whoamiProject(data))
	},
}

// whoamiProject digs the current user's core fields out of a
// get_current_user_profile response, tolerating the {result:[{orgEmployeeModel}]}
// envelope the gateway uses as well as flatter shapes.
func whoamiProject(data map[string]any) map[string]any {
	m := whoamiEmployeeModel(data)
	out := map[string]any{}
	if v := whoamiStr(m, "orgUserName", "name", "userName", "nick"); v != "" {
		out["name"] = v
	}
	if v := whoamiStr(m, "userId", "userid", "staffId"); v != "" {
		out["userId"] = v
	}
	if v := whoamiStr(m, "orgUserMobile", "mobile", "stateMobile"); v != "" {
		out["mobile"] = v
	}
	if v := whoamiStr(m, "orgAuthEmail", "email", "orgEmail"); v != "" {
		out["email"] = v
	}
	if v := whoamiStr(m, "orgName", "corpName"); v != "" {
		out["org"] = v
	}
	if dept := whoamiFirstDept(m); dept != "" {
		out["dept"] = dept
	}
	if len(out) == 0 {
		// Unrecognised shape — fall back to the raw payload rather than an empty
		// object.
		return data
	}
	return out
}

// whoamiEmployeeModel locates the org-employee record inside the profile
// response, probing the common containers.
func whoamiEmployeeModel(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	// {result:[{orgEmployeeModel:{…}}]}
	if arr, ok := data["result"].([]any); ok && len(arr) > 0 {
		if first, ok := arr[0].(map[string]any); ok {
			if em, ok := first["orgEmployeeModel"].(map[string]any); ok {
				return em
			}
			return first
		}
	}
	// {orgEmployeeModel:{…}} or {result:{…}} or the object itself.
	if em, ok := data["orgEmployeeModel"].(map[string]any); ok {
		return em
	}
	if res, ok := data["result"].(map[string]any); ok {
		if em, ok := res["orgEmployeeModel"].(map[string]any); ok {
			return em
		}
		return res
	}
	return data
}

// whoamiFirstDept reads the first department name from the depts list.
func whoamiFirstDept(m map[string]any) string {
	if arr, ok := m["depts"].([]any); ok {
		for _, d := range arr {
			if dm, ok := d.(map[string]any); ok {
				if v := whoamiStr(dm, "deptName", "name", "departmentName"); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// whoamiStr returns the first non-empty string among the candidate keys.
func whoamiStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func init() {
	shortcut.Register(Whoami)
}
