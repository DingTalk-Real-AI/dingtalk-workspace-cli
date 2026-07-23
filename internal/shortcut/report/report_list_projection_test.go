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

package report

import (
	"encoding/json"
	"testing"
)

// TestReportEntryListResolveListSnakeShape guards against projection-data-loss:
// get_received_report_list / get_send_report_list nest the list under
// result.report_list (snake_case); the resolver must probe "report_list" or
// +inbox-list / +outbox-list silently return empty despite the backend having
// reports.
func TestReportEntryListResolveListSnakeShape(t *testing.T) {
	const raw = `{"result":{"report_list":[
		{"reportId":"r1","templateName":"日报"},
		{"reportId":"r2","templateName":"周报"}
	]}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := reportEntryListResolveList(data); len(got) != 2 {
		t.Fatalf("上下层数据不一致: 底层 result.report_list 有 2 条，resolver 返回 %d 条", len(got))
	}
}
