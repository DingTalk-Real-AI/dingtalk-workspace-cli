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

// Phase C 收尾（B41/B42；契约规范 §5.2）。落盘策略：轮8裁决⑩新文件——
// 不编辑 emitter_phase_c_test.go 既有断言。
//
// 既有覆盖对照：raw≠json 逐字节差异与 raw=writeRaw(data) 透传已由
// TestRawPassThroughEnvelopeDiffersFromJSONByteForByte 覆盖；jq 优先于
// -f table 已由 TestJQOverridesFormatAndEvaluatesFullEnvelope 覆盖。
// 本文件补全：raw 适用域（仅 stdout 数据通道）+ raw×fields / raw×jq
// 穷举断言（B41），以及 jq × 全六格式穷举 + jq 对 fields 的优先级（B42）。

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// --- B41：-f raw 透传完整语义——适用域穷举 ---

func TestRawDispatchScopeIsStdoutDataChannelOnly(t *testing.T) {
	// §5.2 分发矩阵的适用域仅 stdout 数据通道（轮8裁决⑪）：携带数据载荷的
	// outcome（success/pending/partial_failure）在 raw 下透传 data；failure
	// 信封即使被配置为 raw 也恒以完整 JSON 走 stderr、stdout 严格零字节。
	cases := []struct {
		name string
		env  *Envelope
	}{
		{"success", phaseCListFixture()},
		{"pending", newRawPendingFixture()},
		{"partial_failure", newRawPartialFixture(t)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			em := NewEmitter(&stdout, &stderr, FormatRaw, "", "")
			if err := em.Emit(c.env); err != nil {
				t.Fatalf("Emit raw: %v", err)
			}
			if stdout.Len() == 0 {
				t.Fatal("data-carrying envelope under raw must emit on stdout")
			}
			if stderr.Len() != 0 {
				t.Fatalf("data channel must keep stderr empty, got %d bytes: %s", stderr.Len(), stderr.String())
			}
			// raw 输出不含信封外壳键（透传的是 data 载荷本身）。
			if strings.Contains(stdout.String(), `"outcome"`) {
				t.Fatalf("raw must pass through data only, no envelope wrapper: %s", stdout.String())
			}
			// 透传等价于 writeRaw(data)（与分发点单一渲染一致）。
			var direct bytes.Buffer
			if err := Write(&direct, FormatRaw, c.env.Data); err != nil {
				t.Fatalf("Write(raw, data): %v", err)
			}
			if !bytes.Equal(stdout.Bytes(), direct.Bytes()) {
				t.Fatalf("raw stdout must be verbatim writeRaw(data):\n got: %s\nwant: %s", stdout.String(), direct.String())
			}
		})
	}
	// failure 信封：raw 配置被绕过（轮8裁决⑪）——stderr 完整 JSON、stdout 零。
	t.Run("failure bypasses raw", func(t *testing.T) {
		failure := NewFailureEnvelope(&ErrorInfo{Type: "api", Code: 40001, Message: "boom"})
		var stdout, stderr bytes.Buffer
		em := NewEmitter(&stdout, &stderr, FormatRaw, "id", ".data")
		if err := em.Emit(failure); err != nil {
			t.Fatalf("Emit raw failure: %v", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("failure must leave stdout empty under any format, got %d bytes", stdout.Len())
		}
		var decoded map[string]any
		if err := json.Unmarshal(stderr.Bytes(), &decoded); err != nil {
			t.Fatalf("failure under raw must stay full JSON on stderr: %v\n%s", err, stderr.String())
		}
		if decoded["outcome"] != "failure" || decoded["error"] == nil {
			t.Fatalf("failure envelope lost wire-stable fields under raw bypass: %s", stderr.String())
		}
	})
	// 分发点层同判：WriteEnvelopeTo（无流路由签名）下 failure + raw 也恒为
	// 完整 JSON 信封——raw 的适用域不含失败通道。
	t.Run("dispatch point also bypasses raw for failure", func(t *testing.T) {
		failure := NewFailureEnvelope(&ErrorInfo{Type: "validation", Param: "--name"})
		var buf bytes.Buffer
		if err := WriteEnvelopeTo(&buf, failure, FormatRaw, "", ""); err != nil {
			t.Fatalf("WriteEnvelopeTo failure raw: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("failure under raw at dispatch point must be full JSON: %v\n%s", err, buf.String())
		}
		if decoded["outcome"] != "failure" {
			t.Fatalf("outcome = %v: %s", decoded["outcome"], buf.String())
		}
	})
}

func newRawPendingFixture() *Envelope {
	// pending 形态携带 data 载荷：raw 适用域覆盖所有携带数据的 outcome。
	env := NewPendingEnvelope(&OperationInfo{ID: "t_1", State: OperationStateProcessing, NextCommand: "dws op get t_1"})
	env.Data = map[string]any{"taskId": "t_1", "accepted": true}
	return env
}

func newRawPartialFixture(t *testing.T) *Envelope {
	t.Helper()
	pd, err := NewPartialData(2,
		[]any{map[string]any{"id": "a", "messageId": "m_1"}},
		[]PartialFailedEntry{{ID: "b", Error: &ErrorInfo{Type: "api", Code: 40001}}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewPartialData: %v", err)
	}
	return NewPartialEnvelope(pd)
}

// --- B41：raw × fields / raw × jq 穷举 ---

func TestRawTimesFieldsProjectsDataBeforePassThrough(t *testing.T) {
	// --fields 先投影数据载荷再按 format 分发（成功通道联动）：raw 下
	// fields 作用于 data 载荷本身——透传的是投影后的载荷，信封外壳仍缺席。
	env := phaseCListFixture()
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, env, FormatRaw, "id", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(raw, fields=id): %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("raw×fields output must be decodable JSON of projected data: %v\n%s", err, buf.String())
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %s", len(rows), buf.String())
	}
	for i, row := range rows {
		if len(row) != 1 {
			t.Fatalf("row %d must be projected to id only: %v", i, row)
		}
		if _, has := row["id"]; !has {
			t.Fatalf("row %d lost id after projection: %v", i, row)
		}
	}
	// 信封外壳键不出现（raw 恒只透传 data 通道）。
	if strings.Contains(buf.String(), `"outcome"`) || strings.Contains(buf.String(), `"meta"`) {
		t.Fatalf("raw×fields must not emit envelope wrapper: %s", buf.String())
	}
}

func TestRawTimesJQJQWinsAndEvaluatesFullEnvelope(t *testing.T) {
	// --jq 优先于 format（含 raw）：对**完整信封**求值后直接输出结果——
	// raw×jq 下 jq 表达式可触及信封外壳键（.ok/.outcome），证明求值对象是
	// 完整信封而非 raw 的 data 载荷。
	for _, tc := range []struct {
		jq   string
		want string
	}{
		{".ok", "true"},
		{".outcome", `"success"`},
		{".meta.pagination.next_token", `"cursor_xyz"`},
	} {
		var buf bytes.Buffer
		if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatRaw, "", tc.jq); err != nil {
			t.Fatalf("WriteEnvelopeTo(raw, jq=%q): %v", tc.jq, err)
		}
		if got := strings.TrimSpace(buf.String()); got != tc.want {
			t.Fatalf("raw×jq %q = %q, want %q (jq must win over raw on the full envelope)", tc.jq, got, tc.want)
		}
	}
	// raw×jq×fields 三者同设：jq 仍恒赢——输出是 jq 求值结果，fields 不得
	// 先行改写求值输入（jq 优先级链在分发点入口，fields 投影只在无 jq 分支）。
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatRaw, "id", ".outcome"); err != nil {
		t.Fatalf("WriteEnvelopeTo(raw, fields+jq): %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != `"success"` {
		t.Fatalf("raw×fields×jq = %q, want \"success\" (jq wins over both)", got)
	}
}

func TestRawStringPayloadVerbatimPassThrough(t *testing.T) {
	// raw 完整语义的字符串分支：data 为字符串时 raw 输出净化后的原文本
	// （writeRaw 字符串分支），不是 JSON 引号包裹——这是「透传」的第二形态。
	env := NewSuccessEnvelope("hello raw world")
	var rawBuf, jsonBuf bytes.Buffer
	if err := WriteEnvelopeTo(&rawBuf, env, FormatRaw, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(raw): %v", err)
	}
	if err := WriteEnvelopeTo(&jsonBuf, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(json): %v", err)
	}
	if got := strings.TrimSpace(rawBuf.String()); got != "hello raw world" {
		t.Fatalf("raw string payload = %q, want verbatim text", got)
	}
	if strings.Contains(rawBuf.String(), `"hello`) {
		t.Fatalf("raw string branch must not JSON-quote the payload: %s", rawBuf.String())
	}
	// json 形态仍是完整信封（两形态互不污染）。
	if !strings.Contains(jsonBuf.String(), `"outcome"`) {
		t.Fatalf("json must keep the envelope wrapper: %s", jsonBuf.String())
	}
}

// --- B42：--jq 优先于 format——全六格式穷举对完整信封求值 ---

func TestJQOverridesAllSixFormatsEvaluatingFullEnvelope(t *testing.T) {
	// jq 优先级对 §5.2 矩阵全部六格式成立：无论 format 是什么，输出恒为
	// jq 对完整信封的求值结果——matrix 各行（table/pretty/csv/ndjson/raw/
	// json）全部被 jq 短路。
	formats := []Format{FormatJSON, FormatTable, FormatPretty, FormatCSV, FormatNDJSON, FormatRaw}
	for _, format := range formats {
		t.Run("format="+string(format), func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteEnvelopeTo(&buf, phaseCListFixture(), format, "", ".outcome"); err != nil {
				t.Fatalf("WriteEnvelopeTo(%s, jq=.outcome): %v", format, err)
			}
			if got := strings.TrimSpace(buf.String()); got != `"success"` {
				t.Fatalf("format %s × jq = %q, want \"success\" (jq must win over every format)", format, got)
			}
		})
	}
	// Emitter 路径同判（双向出口共用分发点）：-f csv + jq 下 stdout 仍是
	// jq 结果而非裸记录。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatCSV, "", ".meta.count")
	if err := em.Emit(phaseCListFixture()); err != nil {
		t.Fatalf("Emit csv+jq: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("Emitter csv×jq = %q, want 3", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("jq path must keep stderr empty, got %d bytes: %s", stderr.Len(), stderr.String())
	}
}

func TestJQWinsOverFieldsAsWell(t *testing.T) {
	// jq 与 fields 同设：jq 恒赢，fields 不得改写 jq 的求值输入。
	// 求值对象是可触及 meta 的完整信封——若 fields 先行投影，.meta 将缺席。
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatJSON, "id", ".meta.count"); err != nil {
		t.Fatalf("WriteEnvelopeTo(fields+jq): %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "3" {
		t.Fatalf("fields×jq = %q, want 3 (jq must win over fields on the full envelope)", got)
	}
}

func TestJQInvalidExpressionFailsCleanly(t *testing.T) {
	// jq 表达式非法时求值失败即报错（validation 类）——不静默降级回 format
	// 输出，也不泄漏半截渲染（buffer-first）。
	var buf bytes.Buffer
	err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatTable, "", ".data[")
	if err == nil {
		t.Fatalf("invalid jq expression must fail, got output: %s", buf.String())
	}
	if !strings.Contains(err.Error(), "jq") {
		t.Fatalf("error must name jq: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("invalid jq must leave zero bytes on the target (buffer-first): %s", buf.String())
	}
}
