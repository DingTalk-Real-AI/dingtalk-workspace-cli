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

package oa

import (
	"encoding/json"
	"testing"
)

// TestListFormsProjectProcessCodeListShape guards against the projection-data-loss
// class: list_user_visible_process nests the forms under result.processCodeList.
// The resolver MUST probe that exact key — otherwise the whole list silently
// projects to empty (exit 0, no error envelope) while the backend has data.
func TestListFormsProjectProcessCodeListShape(t *testing.T) {
	// Faithful list_user_visible_process shape (as returned by the backend).
	const raw = `{"result":{"processCodeList":[
		{"processCode":"PROC-1","processName":"请假","dirName":"假勤管理"},
		{"processCode":"PROC-2","processName":"补卡申请","dirName":"假勤管理"},
		{"processCode":"PROC-3","processName":"加班","dirName":"假勤管理"}
	],"totalCount":-1},"success":true}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	forms := listFormsProject(data)
	if len(forms) != 3 {
		t.Fatalf("上下层数据不一致: 底层有 3 张表单，投影返回 %d 张 (forms=%v)", len(forms), forms)
	}
	for _, f := range forms {
		if f["processCode"] == nil || f["name"] == nil {
			t.Fatalf("projected form missing processCode/name: %v", f)
		}
	}
}

// TestListFormsProjectBareArrayShape ensures a bare top-level array still works.
func TestListFormsProjectBareArrayShape(t *testing.T) {
	const raw = `{"result":[
		{"processCode":"PROC-9","processName":"通用审批"}
	]}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if forms := listFormsProject(data); len(forms) != 1 {
		t.Fatalf("bare array shape: want 1 form, got %d (%v)", len(forms), forms)
	}
}

// TestOAInstanceResolveListValuesShape guards the shared resolver behind
// +list-pending / +list-executed / +list-submitted / +list-cc. Those approval
// instance tools nest the list under result.values; the resolver must probe
// "values" or all four commands silently return empty despite backend records.
func TestOAInstanceResolveListValuesShape(t *testing.T) {
	const raw = `{"result":{"hasMore":false,"values":[
		{"processInstanceId":"i-1","title":"测试用户的请假"},
		{"processInstanceId":"i-2","title":"测试用户的报销"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := oaInstanceResolveList(data); len(got) != 2 {
		t.Fatalf("上下层数据不一致: 底层 result.values 有 2 条，resolver 返回 %d 条", len(got))
	}
	// End-to-end through a real projection that uses the shared resolver.
	if instances := listSubmittedProject(data); len(instances) != 2 {
		t.Fatalf("listSubmittedProject: want 2, got %d (%v)", len(instances), instances)
	}
}
