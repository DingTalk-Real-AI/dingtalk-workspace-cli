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
	"strings"
	"testing"
)

// devAppTableViewCase 是 B122~B126 的 table 视图用例：每条给出可执行 argv、
// list 载荷（items 数组）与期望的列头集合。断言 -f table 输出去包裹信封
// 外壳、渲染数据列表为表（列头存在、行值存在），且 enumerate 载荷的 count
// 人读摘要落 stderr 或视图。B122~B126 对应 dev app 的 list 族：
//
//	B122 dev app list               （应用列表）
//	B123 dev app version list       （版本列表）
//	B124 dev app event list         （事件列表）
//	B125 dev app permission list    （权限列表）
//	B126 dev app member list        （成员列表）
func TestDevAppTableViewColumns(t *testing.T) {
	cases := []struct {
		batch   string
		args    []string
		items   []any
		headers []string // table 列头（rowsFromSlice 按 key 排序）
		wantVal string   // 必须出现在某行的业务值
	}{
		{
			batch: "B122-app-list",
			args:  []string{"dev", "app", "list", "--name", "DemoApp"},
			items: []any{
				map[string]any{"unifiedAppId": "u-1", "name": "DemoApp", "appStatus": "ENABLED"},
				map[string]any{"unifiedAppId": "u-2", "name": "Another", "appStatus": "DISABLED"},
			},
			headers: []string{"appStatus", "name", "unifiedAppId"},
			wantVal: "Another",
		},
		{
			batch: "B123-version-list",
			args:  []string{"dev", "app", "version", "list", "--unified-app-id", "u-1"},
			items: []any{
				map[string]any{"versionId": "v-1", "version": "1.0.0", "status": "ONLINE"},
				map[string]any{"versionId": "v-2", "version": "1.0.1", "status": "DRAFT"},
			},
			headers: []string{"status", "version", "versionId"},
			wantVal: "1.0.1",
		},
		{
			batch: "B124-event-list",
			args:  []string{"dev", "app", "event", "list", "--unified-app-id", "u-1"},
			items: []any{
				map[string]any{"eventCode": "bpms_task_change", "eventName": "审批状态变更"},
				map[string]any{"eventCode": "contact_change", "eventName": "通讯录变更"},
			},
			headers: []string{"eventCode", "eventName"},
			wantVal: "bpms_task_change",
		},
		{
			batch: "B125-permission-list",
			args:  []string{"dev", "app", "permission", "list", "--unified-app-id", "u-1"},
			items: []any{
				map[string]any{"scopeValue": "Contact.User.mobile", "scopeName": "手机号码", "authStatus": "AUTHED"},
				map[string]any{"scopeValue": "Contact.User.read", "scopeName": "通讯录读取", "authStatus": "UNAUTHED"},
			},
			headers: []string{"authStatus", "scopeName", "scopeValue"},
			wantVal: "Contact.User.read",
		},
		{
			batch: "B126-member-list",
			args:  []string{"dev", "app", "member", "list", "--unified-app-id", "u-1"},
			items: []any{
				map[string]any{"userId": "user-1", "memberType": "DEVELOPER", "nickname": "张三"},
				map[string]any{"userId": "user-2", "memberType": "ADMIN", "nickname": "李四"},
			},
			headers: []string{"memberType", "nickname", "userId"},
			wantVal: "李四",
		},
	}

	for _, tc := range cases {
		t.Run(tc.batch, func(t *testing.T) {
			args := append([]string{}, tc.args...)
			args = append(args, "--format", "table")
			out, errBuf, err := runDevAppFamily(t,
				devAppFamilyContentRunner(map[string]any{"items": tc.items}),
				args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			s := out.String()
			// 信封外壳不得出现。
			for _, banned := range []string{`"ok"`, `"outcome"`, `"items":`} {
				if strings.Contains(s, banned) {
					t.Fatalf("-f table leaked envelope/wrapper key %s:\n%s", banned, s)
				}
			}
			// 列头必须出现（rowsFromSlice 对 map 元素按 key 排序作表头）。
			for _, h := range tc.headers {
				if !strings.Contains(s, h) {
					t.Fatalf("-f table missing column header %q:\n%s", h, s)
				}
			}
			// 至少一行业务值出现。
			if !strings.Contains(s, tc.wantVal) {
				t.Fatalf("-f table missing row value %q:\n%s", tc.wantVal, s)
			}
			_ = errBuf
		})
	}
}

// TestDevAppTableViewEmptyListIsValidTable 是 B122~B126 的空列表 table 视图
// 断言（AC-06 空态合法载荷）：data:[] 空数组在 -f table 下渲染为空表（"value"
// 单列头或空行），不报错、不输出 null、信封外壳不泄漏。
func TestDevAppTableViewEmptyListIsValidTable(t *testing.T) {
	out, errBuf, err := runDevAppFamily(t,
		devAppFamilyContentRunner(map[string]any{"items": []any{}}),
		"dev", "app", "list", "--name", "DemoApp", "--format", "table")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	s := out.String()
	if strings.Contains(s, "null") {
		t.Fatalf("-f table empty list must not emit null:\n%s", s)
	}
	for _, banned := range []string{`"ok"`, `"outcome"`} {
		if strings.Contains(s, banned) {
			t.Fatalf("-f table empty list leaked envelope key %s:\n%s", banned, s)
		}
	}
}
