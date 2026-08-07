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

// Phase G：partial_failure / unknown 通道（B139~B146；契约规范 §2.3）。
// 落盘策略：轮8裁决⑩新文件——不编辑 envelope_test.go/emitter_test.go。
// B143（partial 信封 → exit 7 出口映射）已由轮9 B34 覆盖
// （emitter_phase_b_test.go：TestInvariantI2ExhaustiveOverFourOutcomes 的
// partial==7 精确断言 + TestEmitterStreamRoutingConsistentWithExitCode 的
// stdout 通道 + exit 7 联动断言），本文件不重复造测试。

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// --- B139：PartialData 三通道明细类型（total/succeeded[]/failed[]/unknown[]）---

func TestPartialDataWireShapeAndKeyOrder(t *testing.T) {
	// wire 形态对齐契约 §2.3 示例：顶层键序 total/succeeded/failed/unknown
	// （声明序 = wire 序），空通道显式 [] 而非 null（§2.5 无 null 纪律）。
	pd, err := NewPartialData(2,
		[]any{map[string]any{"id": "a", "messageId": "m_1"}},
		nil, // nil failed 归一为 []
		[]PartialUnknownEntry{{ID: "c", Reason: "timeout after submit"}},
	)
	if err != nil {
		t.Fatalf("NewPartialData: %v", err)
	}
	raw, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("json.Marshal(PartialData): %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("PartialData wire is not valid JSON: %v\n%s", err, raw)
	}
	if got, ok := decoded["total"].(float64); !ok || got != 2 {
		t.Fatalf("total = %v, want 2: %s", decoded["total"], raw)
	}
	for _, channel := range []string{"succeeded", "failed", "unknown"} {
		arr, ok := decoded[channel].([]any)
		if !ok {
			t.Fatalf("channel %q must serialize as a JSON array: %s", channel, raw)
		}
		if arr == nil {
			t.Fatalf("channel %q must be [] not null: %s", channel, raw)
		}
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("partial channels must never emit null (§2.5): %s", raw)
	}
	// 键序断言：json.Decoder 保序读入。
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("PartialData must be an object: %v (%v)", tok, err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("decode value: %v", err)
		}
	}
	if want := []string{"total", "succeeded", "failed", "unknown"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("PartialData key order = %v, want %v (§2.3)", keys, want)
	}
}

func TestPartialFailedEntryCarriesErrorInfoShape(t *testing.T) {
	// failed 通道条目 = id + error（复用 §2.4 ErrorInfo，wire-stable 字段可分支）。
	entry := PartialFailedEntry{ID: "b", Error: &ErrorInfo{Type: "api", Code: 40001, Message: "invalid recipient"}}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal(failed entry): %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed entry wire invalid: %v\n%s", err, raw)
	}
	if decoded["id"] != "b" {
		t.Fatalf("failed entry id = %v: %s", decoded["id"], raw)
	}
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		t.Fatalf("failed entry must carry error detail (§2.3): %s", raw)
	}
	if errObj["type"] != "api" {
		t.Fatalf("failed entry error.type = %v: %s", errObj["type"], raw)
	}
}

// --- B141：全失败禁用 partial_failure（空 succeeded 拒绝构造）---

func TestNewPartialDataRejectsEmptySucceeded(t *testing.T) {
	// §2.3 规则4：全部失败时必须用 outcome:failure，禁止空 succeeded 的
	// partial_failure。构造期即拒绝。
	for _, tc := range []struct {
		name    string
		failed  []PartialFailedEntry
		unknown []PartialUnknownEntry
	}{
		{"all failed", []PartialFailedEntry{{ID: "b", Error: &ErrorInfo{Type: "api"}}}, nil},
		{"all unknown", nil, []PartialUnknownEntry{{ID: "c", Reason: "timeout"}}},
		{"failed and unknown only",
			[]PartialFailedEntry{{ID: "b", Error: &ErrorInfo{Type: "api"}}},
			[]PartialUnknownEntry{{ID: "c", Reason: "timeout"}}},
		{"empty everything", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total := len(tc.failed) + len(tc.unknown)
			pd, err := NewPartialData(total, nil, tc.failed, tc.unknown)
			if err == nil {
				t.Fatalf("NewPartialData with empty succeeded must be rejected (§2.3 rule 4), got %+v", pd)
			}
			if !strings.Contains(err.Error(), "succeeded") || !strings.Contains(err.Error(), "failure") {
				t.Fatalf("rejection must name the empty-succeeded/all-failure rule: %v", err)
			}
		})
	}
	// 全失败的正确形态：普通 failure 信封（构造合法、Validate 通过）。
	failure := NewFailureEnvelope(&ErrorInfo{Type: "api", Message: "all 3 items failed"})
	if err := failure.Validate(); err != nil {
		t.Fatalf("all-failed batch must use a legal failure envelope: %v", err)
	}
}

func TestEnvelopeValidateRejectsHandBuiltAllFailedPartial(t *testing.T) {
	// 绕过构造器手工装配的全失败 partial 同样被 Validate 拒绝
	// （校验集中，§2.3 规则4）。
	pd := &PartialData{
		Total:     2,
		Succeeded: []any{},
		Failed: []PartialFailedEntry{
			{ID: "a", Error: &ErrorInfo{Type: "api"}},
			{ID: "b", Error: &ErrorInfo{Type: "api"}},
		},
		Unknown: []PartialUnknownEntry{},
	}
	env := NewPartialEnvelope(pd)
	err := env.Validate()
	if err == nil {
		t.Fatal("Validate must reject all-failed partial_failure (§2.3 rule 4)")
	}
	if !strings.Contains(err.Error(), "partial_failure with empty succeeded") {
		t.Fatalf("error must name the rule: %v", err)
	}
}

func TestPartialDataTotalReconciliation(t *testing.T) {
	// total 对账：total != 三通道之和即拒绝（明细是 total 的完整划分）。
	_, err := NewPartialData(5, []any{map[string]any{"id": "a"}},
		[]PartialFailedEntry{{ID: "b", Error: &ErrorInfo{Type: "api"}}}, nil)
	if err == nil {
		t.Fatal("total=5 with 2 detailed entries must be rejected (§2.3 reconciliation)")
	}
	if !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("error must name reconciliation: %v", err)
	}
}

// --- B140：通道校验规则——unknown 条目禁止归入 succeeded/failed ---

func TestPartialDataRejectsCrossChannelDuplicateIDs(t *testing.T) {
	succeeded := func(id string) []any { return []any{map[string]any{"id": id, "messageId": "m_1"}} }
	failed := func(id string) []PartialFailedEntry {
		return []PartialFailedEntry{{ID: id, Error: &ErrorInfo{Type: "api"}}}
	}
	unknown := func(id string) []PartialUnknownEntry {
		return []PartialUnknownEntry{{ID: id, Reason: "timeout after submit"}}
	}
	cases := []struct {
		name      string
		succeeded []any
		failed    []PartialFailedEntry
		unknown   []PartialUnknownEntry
	}{
		{"unknown id also in succeeded", succeeded("c"), nil, unknown("c")},
		{"unknown id also in failed", nil, failed("c"), unknown("c")},
		{"failed id also in succeeded", succeeded("b"), failed("b"), nil},
		{"duplicate within unknown", succeeded("a"), nil,
			append(unknown("c"), PartialUnknownEntry{ID: "c", Reason: "second report"})},
		{"duplicate within succeeded", append(succeeded("a"), map[string]any{"id": "a"}), nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total := len(tc.succeeded) + len(tc.failed) + len(tc.unknown)
			_, err := NewPartialData(total, tc.succeeded, tc.failed, tc.unknown)
			if err == nil {
				t.Fatalf("cross/duplicate channel id must be rejected (§2.3 rule 2)")
			}
			if !strings.Contains(err.Error(), "one terminal channel") && !strings.Contains(err.Error(), "exactly one terminal channel") {
				t.Fatalf("error must name the one-channel-per-entry rule: %v", err)
			}
		})
	}
}

func TestPartialDataAcceptsDistinctChannelIDs(t *testing.T) {
	// 三通道 id 互不相交时合法（契约 §2.3 标准形态）。
	pd, err := NewPartialData(3,
		[]any{map[string]any{"id": "a", "messageId": "m_1"}},
		[]PartialFailedEntry{{ID: "b", Error: &ErrorInfo{Type: "api", Code: 40001}}},
		[]PartialUnknownEntry{{ID: "c", Reason: "timeout after submit"}},
	)
	if err != nil {
		t.Fatalf("legal three-channel data rejected: %v", err)
	}
	env := NewPartialEnvelope(pd)
	env.Meta = &Meta{Count: NewCount(3)}
	if err := env.Validate(); err != nil {
		t.Fatalf("legal partial envelope must pass Validate: %v", err)
	}
}

func TestPartialDataEntriesMustBeIdentifiable(t *testing.T) {
	// failed/unknown 条目 id 必须非空——条目可标识是跨通道互斥的唯一依据。
	if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}},
		[]PartialFailedEntry{{Error: &ErrorInfo{Type: "api"}}}, nil); err == nil {
		t.Fatal("failed entry without id must be rejected (§2.3)")
	}
	if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}},
		nil, []PartialUnknownEntry{{Reason: "timeout"}}); err == nil {
		t.Fatal("unknown entry without id must be rejected (§2.3)")
	}
}

// --- B144：unknown[] 条目 shape（id + reason）---

func TestPartialUnknownEntryShapeIDPlusReason(t *testing.T) {
	// shape 恰为 id + reason 两键（P0#9「报错却写入」类条目的诚实通道）。
	entry := PartialUnknownEntry{ID: "c", Reason: "timeout after submit"}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal(unknown entry): %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unknown entry wire invalid: %v\n%s", err, raw)
	}
	if len(decoded) != 2 {
		t.Fatalf("unknown entry must carry exactly id+reason, got %v: %s", decoded, raw)
	}
	if decoded["id"] != "c" || decoded["reason"] != "timeout after submit" {
		t.Fatalf("unknown entry = %v, want id+reason preserved", decoded)
	}
}

func TestPartialUnknownEntryRequiresReason(t *testing.T) {
	// 诚实通道必须说明不确定原因：空 reason 拒绝。
	if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}},
		nil, []PartialUnknownEntry{{ID: "c"}}); err == nil {
		t.Fatal("unknown entry without reason must be rejected (§2.3/P0#9)")
	}
	if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}},
		nil, []PartialUnknownEntry{{ID: "c", Reason: "   "}}); err == nil {
		t.Fatal("unknown entry with whitespace-only reason must be rejected (§2.3/P0#9)")
	}
}

// --- B146：succeeded 明细完整保留（构造器不接受丢弃明细）---

func TestNewPartialDataPreservesSucceededDetailsVerbatim(t *testing.T) {
	// 构造器不丢弃、不过滤、不重排任何 succeeded 条目（§2.3 规则1：
	// Agent 需要知道哪些已生效，避免重复提交）。
	succeeded := []any{
		map[string]any{"id": "a", "messageId": "m_1"},
		map[string]any{"id": "d", "messageId": "m_4", "extra": map[string]any{"nested": true}},
		"d-plain-string-entry", // 异构条目同样原样保留
	}
	pd, err := NewPartialData(3, succeeded, nil, nil)
	if err != nil {
		t.Fatalf("NewPartialData: %v", err)
	}
	if len(pd.Succeeded) != len(succeeded) {
		t.Fatalf("succeeded length = %d, want %d (no dropping)", len(pd.Succeeded), len(succeeded))
	}
	if !reflect.DeepEqual(pd.Succeeded, succeeded) {
		t.Fatalf("succeeded details altered:\n got %+v\nwant %+v", pd.Succeeded, succeeded)
	}
	// wire 层同样完整：异构条目（object 与 string）均可解码回原样。
	raw, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var generic struct {
		Succeeded []any `json:"succeeded"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("decode: %v\n%s", err, raw)
	}
	if len(generic.Succeeded) != 3 {
		t.Fatalf("wire succeeded length = %d, want 3: %s", len(generic.Succeeded), raw)
	}
	first, _ := generic.Succeeded[0].(map[string]any)
	if first == nil || first["messageId"] != "m_1" {
		t.Fatalf("wire lost succeeded detail fields: %s", raw)
	}
	if generic.Succeeded[2] != "d-plain-string-entry" {
		t.Fatalf("wire lost heterogeneous succeeded entry: %s", raw)
	}
}

func TestPartialDataRejectsNilSucceededEntries(t *testing.T) {
	// nil（null）条目混入明细即拒绝：完整保留意味着每条都是真实明细。
	if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}, nil}, nil, nil); err == nil {
		t.Fatal("nil succeeded entry must be rejected (§2.3 full preservation)")
	}
}

// --- B145：partial 装配样例——2 成 1 败 1 unknown（契约规范 §6 场景3）---

func TestPartialAssemblyExampleTwoOneOne(t *testing.T) {
	// 场景3 行为端：ok:false / rc=7 映射 / stderr 空；三通道明细完整落 stdout。
	pd, err := NewPartialData(4,
		[]any{
			map[string]any{"id": "a", "messageId": "m_1"},
			map[string]any{"id": "b", "messageId": "m_2"},
		},
		[]PartialFailedEntry{{ID: "c", Error: &ErrorInfo{Type: "api", Code: 40001, Message: "invalid recipient"}}},
		[]PartialUnknownEntry{{ID: "d", Reason: "timeout after submit"}},
	)
	if err != nil {
		t.Fatalf("NewPartialData: %v", err)
	}
	env := NewPartialEnvelope(pd)
	env.Meta = &Meta{Count: NewCount(4)}

	// 不变量面：I1 ok==false（partial 非 ok）、I3 无顶层 error、Validate 通过。
	if env.OK || env.Outcome != OutcomePartialFailure {
		t.Fatalf("partial envelope = ok:%v outcome:%q, want false/partial_failure", env.OK, env.Outcome)
	}
	if env.Error != nil {
		t.Fatalf("partial envelope must not carry top-level error (I3/§2.3)")
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// 出口面：明细落 stdout、stderr 零输出（§2.3）。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit partial: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("partial_failure must keep stderr empty (§2.3), got %d bytes: %s", stderr.Len(), stderr.String())
	}
	var decoded struct {
		OK      bool        `json:"ok"`
		Outcome string      `json:"outcome"`
		Data    PartialData `json:"data"`
		Meta    struct {
			Count *int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not one complete JSON envelope: %v\n%s", err, stdout.String())
	}
	if decoded.OK {
		t.Fatalf("partial ok = true, want false (§1): %s", stdout.String())
	}
	if decoded.Outcome != "partial_failure" {
		t.Fatalf("outcome = %q: %s", decoded.Outcome, stdout.String())
	}
	if decoded.Data.Total != 4 || len(decoded.Data.Succeeded) != 2 ||
		len(decoded.Data.Failed) != 1 || len(decoded.Data.Unknown) != 1 {
		t.Fatalf("channel counts = %d/%d/%d/%d, want 4/2/1/1: %s",
			decoded.Data.Total, len(decoded.Data.Succeeded), len(decoded.Data.Failed), len(decoded.Data.Unknown), stdout.String())
	}
	if decoded.Data.Failed[0].Error == nil || decoded.Data.Failed[0].Error.Type != "api" {
		t.Fatalf("failed entry lost error detail: %s", stdout.String())
	}
	if decoded.Data.Unknown[0].Reason != "timeout after submit" {
		t.Fatalf("unknown entry lost reason: %s", stdout.String())
	}
	if decoded.Meta.Count == nil || *decoded.Meta.Count != 4 {
		t.Fatalf("meta.count = %v, want 4: %s", decoded.Meta.Count, stdout.String())
	}

	// rc=7 映射（§4 partial 专用码；ExitCodeForEnvelope 与 errors 侧同源见 B209）。
	if code := ExitCodeForEnvelope(env); code != 7 {
		t.Fatalf("partial exit code = %d, want dedicated 7 (§4)", code)
	}
}

// --- B143 覆盖核实说明（不重复造测试）---
//
// partial 信封 → exit 7 的出口断言已由轮9 B34 落盘并评审通过：
//   - emitter_phase_b_test.go TestInvariantI2ExhaustiveOverFourOutcomes：
//     「partial_failure 专用码精确断言（非零且恰为 7）」；
//   - emitter_phase_b_test.go TestEmitterStreamRoutingConsistentWithExitCode：
//     partial_failure 走 stdout 通道且 exit 7 的联动断言。
// 本文件 TestPartialAssemblyExampleTwoOneOne 在装配样例内顺带复核 rc=7，
// 仅为场景自洽，不构成新的覆盖重复主张。
