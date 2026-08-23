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

package output

// Phase H：operation / pagination 装配助手（B147~B154；契约规范 §2.2/§3）。
// 落盘策略：轮8裁决⑩新文件——不编辑 envelope_test.go/emitter_test.go。

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// --- B150：operation.state 取值表测试 ---

func TestOperationStateValueTable(t *testing.T) {
	// state 取值表（§2.2）：规范三值 processing/completed/failed 是 Agent
	// 必须能分支的最小集；开放枚举容忍业务特化阶段（如 importing、queued）。
	// 表驱动：每个规范值装配合法、wire 原样、Validate 通过。
	normative := []struct {
		state string
		// timed_out 共存合法性由 ValidateTimeoutState 把守，这里只表 state 装配。
	}{
		{OperationStateProcessing},
		{OperationStateCompleted},
		{OperationStateFailed},
	}
	if len(normative) != 3 {
		t.Fatalf("operation.state normative table must cover exactly 3 values (§2.2), got %d", len(normative))
	}
	for _, tc := range normative {
		t.Run("state="+tc.state, func(t *testing.T) {
			op := &OperationInfo{ID: "t_1", State: tc.state, NextCommand: "dws op get t_1"}
			env, err := NewPendingEnvelopeForPolling(op)
			if err != nil {
				t.Fatalf("NewPendingEnvelopeForPolling(state=%q): %v", tc.state, err)
			}
			raw, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var decoded struct {
				Meta struct {
					Operation struct {
						State string `json:"state"`
					} `json:"operation"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("decode: %v\n%s", err, raw)
			}
			if decoded.Meta.Operation.State != tc.state {
				t.Fatalf("wire state = %q, want %q: %s", decoded.Meta.Operation.State, tc.state, raw)
			}
		})
	}
	// 开放枚举：业务特化阶段不在规范表内但可装配（框架不解释语义，§9）。
	op := &OperationInfo{ID: "t_2", State: "importing", NextCommand: "dws op get t_2"}
	if _, err := NewPendingEnvelopeForPolling(op); err != nil {
		t.Fatalf("specialized state must remain assemblable (open enum, §2.2): %v", err)
	}
}

// --- B148：next_command 非空可执行断言 ---

func TestPollingPendingEnvelopeRequiresNonEmptyNextCommand(t *testing.T) {
	// 轮询形态的恢复方式只有 next_command 一条出口（§2.2 规则2）：
	// 非空、无前后空白、单行——缺任一项即装配失败。
	cases := []struct {
		name string
		op   *OperationInfo
	}{
		{"nil operation", nil},
		{"empty next_command", &OperationInfo{ID: "t_1", State: OperationStateProcessing}},
		{"whitespace next_command", &OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "   "}},
		{"leading space", &OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: " dws op get t_1"}},
		{"trailing space", &OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1 "}},
		{"multi-line", &OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1\ndws op get t_1"}},
		{"carriage return", &OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1\r"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewPendingEnvelopeForPolling(tc.op)
			if err == nil {
				t.Fatalf("NewPendingEnvelopeForPolling(%+v) must fail (§2.2 rule 2), got %+v", tc.op, env)
			}
		})
	}
	// 合法形态：可执行单行命令装配通过。
	env, err := NewPendingEnvelopeForPolling(&OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1"})
	if err != nil {
		t.Fatalf("legal next_command rejected: %v", err)
	}
	if env.Meta == nil || env.Meta.Operation == nil || env.Meta.Operation.NextCommand != "dws op get t_1" {
		t.Fatalf("assembled envelope lost next_command: %+v", env)
	}
	// ValidateNextCommand 对空串不拦截（裸 pending 信封允许无 operation 明细），
	// 只对非空值做形态校验——严格性由装配助手把守。
	if err := (&OperationInfo{}).ValidateNextCommand(); err != nil {
		t.Fatalf("empty next_command must not be rejected by shape validator alone: %v", err)
	}
}

// --- B149：timed_out:true 时 state 保留断言（§2.2 防伪装）---

func TestTimedOutKeepsRealState(t *testing.T) {
	// §2.2 规则1（lark wiki_node_delete 教训）：轮询超时必须保持 state 真实值
	// 并置 timed_out:true；超时操作不得宣称完成态。
	cases := []struct {
		name    string
		op      *OperationInfo
		wantErr bool
	}{
		{"timed_out with real processing state",
			&OperationInfo{ID: "t_1", State: OperationStateProcessing, TimedOut: true, NextCommand: "dws op get t_1"}, false},
		{"timed_out with specialized real state",
			&OperationInfo{ID: "t_1", State: "importing", TimedOut: true, NextCommand: "dws op get t_1"}, false},
		{"timed_out with empty state",
			&OperationInfo{ID: "t_1", TimedOut: true, NextCommand: "dws op get t_1"}, true},
		{"timed_out claiming completed",
			&OperationInfo{ID: "t_1", State: OperationStateCompleted, TimedOut: true, NextCommand: "dws op get t_1"}, true},
		{"timed_out claiming success (case-insensitive)",
			&OperationInfo{ID: "t_1", State: "SUCCESS", TimedOut: true, NextCommand: "dws op get t_1"}, true},
		{"no timeout keeps completed legal",
			&OperationInfo{ID: "t_1", State: OperationStateCompleted, NextCommand: "dws op get t_1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.op.ValidateTimeoutState()
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateTimeoutState(%+v) must reject (§2.2 anti-spoof)", tc.op)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateTimeoutState(%+v) must accept: %v", tc.op, err)
			}
		})
	}
	// wire 层：timed_out:true 与真实 state 同现。
	env, err := NewPendingEnvelopeForPolling(&OperationInfo{ID: "t_1", State: OperationStateProcessing, TimedOut: true, NextCommand: "dws op get t_1"})
	if err != nil {
		t.Fatalf("assembling timed_out envelope: %v", err)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded struct {
		Meta struct {
			Operation struct {
				State    string `json:"state"`
				TimedOut bool   `json:"timed_out"`
			} `json:"operation"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	if decoded.Meta.Operation.State != OperationStateProcessing || !decoded.Meta.Operation.TimedOut {
		t.Fatalf("wire must keep real state + timed_out:true (§2.2), got %+v: %s", decoded.Meta.Operation, raw)
	}
}

// --- B147：NewPendingEnvelope 装配助手增强 / 轮询形态 ---

func TestPendingEnvelopeAssemblyAssistants(t *testing.T) {
	// 宽松构造器（B19）保持构造宽松：无 op / 空 next_command 均可构造。
	if env := NewPendingEnvelope(nil); env.Meta != nil || !env.OK || env.Outcome != OutcomePending {
		t.Fatalf("NewPendingEnvelope(nil) = %+v, want bare pending", env)
	}
	if env := NewPendingEnvelope(&OperationInfo{ID: "t_1"}); env.Meta == nil || env.Meta.Operation.ID != "t_1" {
		t.Fatalf("NewPendingEnvelope lost operation: %+v", env)
	}
	// 严格装配助手（B147）：轮询形态必须携带 op + 可执行 next_command。
	env, err := NewPendingEnvelopeForPolling(&OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1"})
	if err != nil {
		t.Fatalf("NewPendingEnvelopeForPolling: %v", err)
	}
	// I1 一致性：pending → ok:true。
	if !env.OK || env.Outcome != OutcomePending {
		t.Fatalf("polling pending envelope = ok:%v outcome:%q, want true/pending (I1)", env.OK, env.Outcome)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// 出口面：pending 落 stdout、exit 0。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf("pending must route stdout (stdout=%d stderr=%d)", stdout.Len(), stderr.Len())
	}
	if code := ExitCodeForEnvelope(env); code != 0 {
		t.Fatalf("pending exit code = %d, want 0 (I2)", code)
	}
}

// --- B151：import_flow 字段映射断言 ---

func TestImportFlowOperationFieldMapping(t *testing.T) {
	// 契约 §2.2 的 import_flow 场景映射：业务侧 task 终态 → operation 字段。
	// 框架只断言映射后的 wire 形态（§9 语义归装配侧）：
	//   task_id  → operation.id
	//   status   → operation.state
	//   timeout  → operation.timed_out（超时时 true，且 state 保留真实值）
	//   轮询命令 → operation.next_command
	cases := []struct {
		name      string
		taskID    string
		status    string
		timedOut  bool
		nextCmd   string
		wantTimed bool
		wantState string
		wantNext  string
	}{
		{"running task", "task_01", "processing", false, "dws op get task_01", false, "processing", "dws op get task_01"},
		{"timed-out task keeps real state", "task_02", "processing", true, "dws op get task_02", true, "processing", "dws op get task_02"},
		{"completed task", "task_03", "completed", false, "dws op get task_03", false, "completed", "dws op get task_03"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewPendingEnvelopeForPolling(&OperationInfo{
				ID:          tc.taskID,
				State:       tc.status,
				TimedOut:    tc.timedOut,
				NextCommand: tc.nextCmd,
			})
			if err != nil {
				t.Fatalf("NewPendingEnvelopeForPolling: %v", err)
			}
			raw, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var decoded struct {
				Meta struct {
					Operation struct {
						ID          string `json:"id"`
						State       string `json:"state"`
						TimedOut    bool   `json:"timed_out"`
						NextCommand string `json:"next_command"`
					} `json:"operation"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("decode: %v\n%s", err, raw)
			}
			op := decoded.Meta.Operation
			if op.ID != tc.taskID {
				t.Fatalf("operation.id = %q, want task_id %q: %s", op.ID, tc.taskID, raw)
			}
			if op.State != tc.wantState {
				t.Fatalf("operation.state = %q, want %q: %s", op.State, tc.wantState, raw)
			}
			if op.TimedOut != tc.wantTimed {
				t.Fatalf("operation.timed_out = %v, want %v: %s", op.TimedOut, tc.wantTimed, raw)
			}
			if op.NextCommand != tc.wantNext {
				t.Fatalf("operation.next_command = %q, want %q: %s", op.NextCommand, tc.wantNext, raw)
			}
		})
	}
}

// --- B152：NewPagination 装配助手 ---

func TestNewPaginationTwoStateCompleteness(t *testing.T) {
	// §3 两态完备：resumable（携带 next_token）与 exhausted（无 token）是
	// 仅有的两个状态；不设 stop_reason。装配助手强制两态互斥完备。
	cases := []struct {
		name      string
		exhausted bool
		token     string
		wantErr   bool
	}{
		{"resumable with token", false, "tok_2", false},
		{"exhausted without token", true, "", false},
		{"resumable missing token", false, "", true},
		{"exhausted carrying token", true, "tok_2", true},
		{"resumable whitespace-only token", false, "   ", true},
		{"exhausted whitespace token accepted as empty", true, "   ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pg, err := NewPagination(tc.exhausted, tc.token)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewPagination(exhausted=%v, token=%q) must fail (§3)", tc.exhausted, tc.token)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewPagination(exhausted=%v, token=%q): %v", tc.exhausted, tc.token, err)
			}
			if pg.EndpointExhausted != tc.exhausted {
				t.Fatalf("endpoint_exhausted = %v, want %v", pg.EndpointExhausted, tc.exhausted)
			}
			wantToken := strings.TrimSpace(tc.token)
			if pg.NextToken != wantToken {
				t.Fatalf("next_token = %q, want %q", pg.NextToken, wantToken)
			}
		})
	}
}

// --- B153：Pagination pages/items 字段 ---

func TestPaginationPagesItemsFields(t *testing.T) {
	// pages/items 为信息性计数（omitempty）：设置后 wire 携带，未设置则缺席。
	pg, err := NewPagination(false, "tok_2")
	if err != nil {
		t.Fatalf("NewPagination: %v", err)
	}
	pages, items := 2, 50
	pg.Pages = pages
	pg.Items = items
	raw, err := json.Marshal(pg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	if got, ok := decoded["pages"].(float64); !ok || got != 2 {
		t.Fatalf("pages = %v, want 2: %s", decoded["pages"], raw)
	}
	if got, ok := decoded["items"].(float64); !ok || got != 50 {
		t.Fatalf("items = %v, want 50: %s", decoded["items"], raw)
	}
	if decoded["next_token"] != "tok_2" || decoded["endpoint_exhausted"] != false {
		t.Fatalf("pagination wire lost core fields: %s", raw)
	}
	// 未设置时 omitempty 缺席。
	bare, err := NewPagination(true, "")
	if err != nil {
		t.Fatalf("NewPagination(exhausted): %v", err)
	}
	raw, err = json.Marshal(bare)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var bareDecoded map[string]any
	if err := json.Unmarshal(raw, &bareDecoded); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	if _, present := bareDecoded["pages"]; present {
		t.Fatalf("unset pages must be omitted: %s", raw)
	}
	if _, present := bareDecoded["items"]; present {
		t.Fatalf("unset items must be omitted: %s", raw)
	}
}

// --- B154：空结果 data:[] + count:0 + endpoint_exhausted 断言 ---

func TestEmptyResultSetWireAssertion(t *testing.T) {
	// AC-06/§3：空结果合法形态 = data:[] + meta.count:0 +
	// pagination.endpoint_exhausted:true。三要素齐备且 wire 原样。
	pg, err := NewPagination(true, "")
	if err != nil {
		t.Fatalf("NewPagination: %v", err)
	}
	env := NewSuccessEnvelope([]any{})
	env.Meta = &Meta{Count: NewCount(0), Pagination: pg}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	var decoded struct {
		OK      bool   `json:"ok"`
		Outcome string `json:"outcome"`
		Data    []any  `json:"data"`
		Meta    struct {
			Count      *int `json:"count"`
			Pagination struct {
				EndpointExhausted bool `json:"endpoint_exhausted"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not one complete JSON envelope: %v\n%s", err, stdout.String())
	}
	if !decoded.OK || decoded.Outcome != "success" {
		t.Fatalf("empty result = ok:%v outcome:%q, want true/success", decoded.OK, decoded.Outcome)
	}
	if decoded.Data == nil || len(decoded.Data) != 0 {
		t.Fatalf("data must be [] (not null, not missing): %s", stdout.String())
	}
	if decoded.Meta.Count == nil || *decoded.Meta.Count != 0 {
		t.Fatalf("meta.count = %v, want 0: %s", decoded.Meta.Count, stdout.String())
	}
	if !decoded.Meta.Pagination.EndpointExhausted {
		t.Fatalf("empty result must mark endpoint_exhausted:true (§3): %s", stdout.String())
	}
	// raw 字符串层面：data 必须是 [] 而非 null。
	if !strings.Contains(stdout.String(), `"data": []`) && !strings.Contains(stdout.String(), `"data":[]`) {
		t.Fatalf("data must serialize as [] on the wire: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"data": null`) || strings.Contains(stdout.String(), `"data":null`) {
		t.Fatalf("data must never be null (§2.5): %s", stdout.String())
	}
}
