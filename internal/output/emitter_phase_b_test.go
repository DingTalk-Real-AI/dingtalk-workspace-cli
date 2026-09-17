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

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

var errEmitterInjected = errors.New("emitter injected write failure")

// emitterFailingWriter 记录被接受的字节并在写出时失败，
// 用于 B25「写一半失败不产生残缺 JSON」断言。
type emitterFailingWriter struct {
	acceptN  int // >0 时先接受 acceptN 字节再报错（模拟写到一半失败）
	accepted []byte
}

func (w *emitterFailingWriter) Write(p []byte) (int, error) {
	if w.acceptN > 0 && len(w.accepted) < w.acceptN {
		k := w.acceptN - len(w.accepted)
		if k > len(p) {
			k = len(p)
		}
		w.accepted = append(w.accepted, p[:k]...)
		return k, errEmitterInjected
	}
	return 0, errEmitterInjected
}

// emitterCountingWriter 统计 Write 调用次数并留存全部字节。
type emitterCountingWriter struct {
	writes int
	buf    bytes.Buffer
}

func (w *emitterCountingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.buf.Write(p)
}

// emitterShortWriter 接受一半字节但谎报成功（n < len(p), err == nil），
// Emitter 必须检出短写而不是把截断输出当成功。
type emitterShortWriter struct {
	n int
}

func (w *emitterShortWriter) Write(p []byte) (int, error) {
	w.n = len(p) / 2
	return w.n, nil
}

func newPartialEnvelopeFixture() *Envelope {
	return NewPartialEnvelope(map[string]any{
		"total":     3,
		"succeeded": []any{map[string]any{"id": "a", "messageId": "m_1"}},
		"failed":    []any{map[string]any{"id": "b", "error": map[string]any{"type": "api", "code": 40001}}},
		"unknown":   []any{map[string]any{"id": "c", "reason": "timeout after submit"}},
	})
}

// --- B24：Emitter 类型与构造 ---

func TestNewEmitterHandlesNilWriters(t *testing.T) {
	em := NewEmitter(nil, nil, FormatJSON, "", "")
	if em == nil {
		t.Fatal("NewEmitter returned nil")
	}
	// nil writer 等价 io.Discard：不 panic，写出成功且无副作用。
	if err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"})); err != nil {
		t.Fatalf("Emit with nil writers: %v", err)
	}
	if err := em.Emit(NewFailureEnvelope(&ErrorInfo{Type: "api"})); err != nil {
		t.Fatalf("Emit failure with nil writers: %v", err)
	}
}

func TestNewEmitterKeepsConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatTable, "id,name", ".data")
	if em.format != FormatTable || em.fields != "id,name" || em.jq != ".data" {
		t.Fatalf("NewEmitter dropped config: format=%q fields=%q jq=%q", em.format, em.fields, em.jq)
	}
}

// --- B26：成功信封 stdout 出口（恰为一个完整 JSON 文档）---

func TestEmitterSuccessEnvelopeGoesToStdoutOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewSuccessEnvelope(map[string]any{"todoId": "t_123"})
	env.Meta = &Meta{Count: NewCount(1)}

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit success: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("success envelope must not touch stderr, got %d bytes: %s", stderr.Len(), stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not exactly one complete JSON document: %v\n%s", err, stdout.String())
	}
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := dec.Decode(&map[string]any{}); err != nil {
		t.Fatalf("first JSON document invalid: %v", err)
	}
	if dec.More() {
		t.Fatalf("stdout must contain exactly one JSON document, got trailing content: %s", stdout.String())
	}
	if okVal, isBool := decoded["ok"].(bool); !isBool || !okVal {
		t.Fatalf("ok = %v (%T), want true (bool)", decoded["ok"], decoded["ok"])
	}
	if decoded["outcome"] != "success" {
		t.Fatalf("outcome = %v, want success", decoded["outcome"])
	}
}

func TestEmitterPendingEnvelopeGoesToStdoutOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewPendingEnvelope(&OperationInfo{ID: "task_abc", State: "processing"})

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit pending: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("pending envelope must not touch stderr, got %d bytes", stderr.Len())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if okVal, isBool := decoded["ok"].(bool); !isBool || !okVal {
		t.Fatalf("pending envelope ok = %v, want true", decoded["ok"])
	}
	if decoded["outcome"] != "pending" {
		t.Fatalf("outcome = %v, want pending", decoded["outcome"])
	}
}

// --- B27：失败信封 stderr 出口（stdout 严格零字节）---

func TestEmitterFailureEnvelopeGoesToStderrWithZeroStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewFailureEnvelope(&ErrorInfo{Type: "api", Subtype: "rate_limit", Code: 90018})

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit failure: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure envelope must leave stdout empty, got %d bytes: %s", stdout.Len(), stdout.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &decoded); err != nil {
		t.Fatalf("stderr failure envelope is not valid JSON: %v\n%s", err, stderr.String())
	}
	if okVal, isBool := decoded["ok"].(bool); !isBool || okVal {
		t.Fatalf("failure envelope ok = %v, want false (bool)", decoded["ok"])
	}
	if decoded["outcome"] != "failure" {
		t.Fatalf("outcome = %v, want failure", decoded["outcome"])
	}
	if _, hasError := decoded["error"]; !hasError {
		t.Fatalf("failure envelope must carry error detail: %s", stderr.String())
	}
}

func TestEmitterNilEnvelopeDegradesToFailureOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(nil); err != nil {
		t.Fatalf("Emit(nil): %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("nil envelope must leave stdout empty, got %d bytes", stdout.Len())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &decoded); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\n%s", err, stderr.String())
	}
	if decoded["outcome"] != "failure" {
		t.Fatalf("nil envelope must degrade to failure: %s", stderr.String())
	}
	// B208（轮8顺带核查①：兜底修正后同步强化）：降级信封必须是 I3
	// 合法形态——ok:false 恒真、error 明细存在且为 internal 类，
	// 兜底信封自身通过 Validate()。
	if okVal, isBool := decoded["ok"].(bool); !isBool || okVal {
		t.Fatalf("nil fallback ok = %v, want false (bool): %s", decoded["ok"], stderr.String())
	}
	errObj, hasErr := decoded["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("nil fallback must carry error detail (I3, B208): %s", stderr.String())
	}
	if errObj["type"] != "internal" {
		t.Fatalf("fallback error.type = %v, want internal: %s", errObj["type"], stderr.String())
	}
	if err := nilFallbackEnvelope().Validate(); err != nil {
		t.Fatalf("nil fallback envelope must pass Validate(), got %v", err)
	}
}

func TestEmitterFailureEnvelopeBypassesFormatAndJQ(t *testing.T) {
	// 失败信封恒以完整 JSON 信封落 stderr，不受 -f table/--jq 影响：
	// 失败信封无 data 载荷（I3），按 format 分发只会渲染空值，
	// Agent 必须总能读到结构化错误明细（§5.1/§2.4）。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatTable, "id", ".data")
	env := NewFailureEnvelope(&ErrorInfo{Type: "validation", Message: "bad flag"})

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit failure with table/jq config: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty for failure, got %d bytes", stdout.Len())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &decoded); err != nil {
		t.Fatalf("stderr failure envelope must stay full JSON regardless of format: %v\n%s", err, stderr.String())
	}
	if decoded["outcome"] != "failure" || decoded["error"] == nil {
		t.Fatalf("stderr lost failure envelope shape under -f table: %s", stderr.String())
	}
}

// --- B28：partial_failure 明细落 stdout、stderr 零输出 ---

func TestEmitterPartialFailureStdoutOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")

	if err := em.Emit(newPartialEnvelopeFixture()); err != nil {
		t.Fatalf("Emit partial_failure: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("partial_failure must keep stderr empty (§2.3), got %d bytes: %s", stderr.Len(), stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if okVal, isBool := decoded["ok"].(bool); !isBool || okVal {
		t.Fatalf("partial_failure ok = %v, want false (bool)", decoded["ok"])
	}
	if decoded["outcome"] != "partial_failure" {
		t.Fatalf("outcome = %v, want partial_failure", decoded["outcome"])
	}
	data, isMap := decoded["data"].(map[string]any)
	if !isMap {
		t.Fatalf("partial data must be the three-channel detail object: %s", stdout.String())
	}
	for _, channel := range []string{"total", "succeeded", "failed", "unknown"} {
		if _, ok := data[channel]; !ok {
			t.Fatalf("partial data missing %q channel: %s", channel, stdout.String())
		}
	}
}

// --- B25：渲染先进 buffer 再一次性写出 ---

func TestEmitterRenderErrorLeavesZeroBytes(t *testing.T) {
	// 渲染失败（marshal 报错）必须不向目标流泄漏任何字节。
	testseam.Swap(t, &marshalJSONIndent, func(any, string, string) ([]byte, error) {
		return nil, errEmitterInjected
	})
	target := &emitterFailingWriter{acceptN: 64}
	em := NewEmitter(target, io.Discard, FormatJSON, "", "")
	err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"}))
	if err == nil {
		t.Fatal("Emit must surface render failure")
	}
	if len(target.accepted) != 0 {
		t.Fatalf("render failure leaked %d bytes to the stream: %s", len(target.accepted), target.accepted)
	}
}

func TestEmitterWriteFailureLeavesZeroBytes(t *testing.T) {
	// 写出失败：buffer 先行意味着目标流要么收到完整文档、要么零字节。
	target := &emitterFailingWriter{} // 第一次 Write 即整体拒绝
	em := NewEmitter(target, io.Discard, FormatJSON, "", "")
	if err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"})); err == nil {
		t.Fatal("Emit must surface write failure")
	}
	if len(target.accepted) != 0 {
		t.Fatalf("write failure leaked %d bytes", len(target.accepted))
	}
}

func TestEmitterWriteOnceEmitsSingleWriteOfFullDocument(t *testing.T) {
	// 「一次性写出」的直接断言：目标流只收到一次 Write，
	// 且字节与独立重渲染结果逐字节一致（无分块流式拼接）。
	target := &emitterCountingWriter{}
	em := NewEmitter(target, io.Discard, FormatJSON, "", "")
	env := NewSuccessEnvelope(map[string]any{"id": "x"})
	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if target.writes != 1 {
		t.Fatalf("expected exactly one Write call (buffer-first single flush), got %d", target.writes)
	}
	var want bytes.Buffer
	if err := WriteEnvelopeTo(&want, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	if !bytes.Equal(target.buf.Bytes(), want.Bytes()) {
		t.Fatalf("streamed bytes differ from re-render:\n got: %s\nwant: %s", target.buf.String(), want.String())
	}
}

func TestEmitterDetectsShortWrite(t *testing.T) {
	// writer 谎报成功但只接受一半字节（短写）：Emitter 必须报
	// io.ErrShortWrite，绝不把截断（残缺）JSON 当成功。
	target := &emitterShortWriter{}
	em := NewEmitter(target, io.Discard, FormatJSON, "", "")
	err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"}))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Emit on short write = %v, want io.ErrShortWrite", err)
	}
}

// --- B29：meta.count 渲染位置（meta 层而非 data 层）---

func TestEmitterMetaCountRenderedInMetaLayer(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewSuccessEnvelope([]any{
		map[string]any{"id": "a"},
		map[string]any{"id": "b"},
		map[string]any{"id": "c"},
	})
	env.Meta = &Meta{Count: NewCount(3)}

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	raw := stdout.String()
	// count 只允许出现在 meta 层：wire 上 "count" 键恰出现一次。
	if got := strings.Count(raw, `"count"`); got != 1 {
		t.Fatalf(`"count" appears %d times on the wire, want exactly 1 (meta layer only): %s`, got, raw)
	}
	var decoded struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Count *int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	if decoded.Meta.Count == nil || *decoded.Meta.Count != 3 {
		t.Fatalf("meta.count = %v, want 3: %s", decoded.Meta.Count, raw)
	}
	for i, row := range decoded.Data {
		if _, hasCount := row["count"]; hasCount {
			t.Fatalf("data[%d] must not carry count (belongs to meta layer): %s", i, raw)
		}
	}
}

// --- B30：dry_run 顶层字段注入断言（契约规范 §6 场景6）---

func TestEmitterDryRunFieldInjectedAndOmitEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewSuccessEnvelope(map[string]any{"plan": "create app DemoApp"})
	env.DryRun = true

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit dry-run envelope: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("dry-run success envelope must not touch stderr, got %d bytes", stderr.Len())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	dryRun, isBool := decoded["dry_run"].(bool)
	if !isBool {
		t.Fatalf("dry_run must be a JSON boolean, got %v (%T): %s", decoded["dry_run"], decoded["dry_run"], stdout.String())
	}
	if !dryRun {
		t.Fatalf("dry_run = %v, want true (§6 场景6): %s", dryRun, stdout.String())
	}
	if okVal, _ := decoded["ok"].(bool); !okVal {
		t.Fatalf("dry-run preview keeps ok=true (exit 0 semantics): %s", stdout.String())
	}
}

func TestEmitterDryRunFalseStaysOmitted(t *testing.T) {
	// dry_run 为 false（非 dry-run）时必须整体缺席（omitempty，不输出 false）。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"})); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	raw := stdout.String()
	if strings.Contains(raw, `"dry_run"`) {
		t.Fatalf("dry_run=false must be omitted from wire (§2.5 omitempty): %s", raw)
	}
}

// --- B31：identity 字段（user/app）omitempty 注入断言（契约规范 §2.5）---

func TestEmitterIdentityInjectedAndOmitEmpty(t *testing.T) {
	for _, identity := range []string{"user", "app"} {
		t.Run(identity, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
			env := NewSuccessEnvelope(map[string]any{"id": "x"})
			env.Identity = identity

			if err := em.Emit(env); err != nil {
				t.Fatalf("Emit: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
				t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
			}
			if decoded["identity"] != identity {
				t.Fatalf("identity = %v, want %q: %s", decoded["identity"], identity, stdout.String())
			}
		})
	}
}

func TestEmitterIdentityEmptyStaysOmitted(t *testing.T) {
	// identity 未注入（空串）时字段缺席，wire 中不出现 "identity"。
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	if err := em.Emit(NewSuccessEnvelope(map[string]any{"id": "x"})); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if raw := stdout.String(); strings.Contains(raw, `"identity"`) {
		t.Fatalf("empty identity must be omitted (§2.5 omitempty): %s", raw)
	}
}

// --- B32：pending 信封出口完整化（ok:true + operation + exit 0 语义标注）---

func TestEmitterPendingOutletFullFormWithOperationAndExitZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
	env := NewPendingEnvelope(&OperationInfo{
		ID:          "task_abc",
		State:       "processing",
		TimedOut:    true, // §2.2：轮询超时保持真实 state + timed_out:true，禁止伪装 success
		NextCommand: "dws wiki task-result --task task_abc",
	})
	env.Data = map[string]any{"spaceId": "s_1", "nodeId": "n_9"}

	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit pending: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("pending envelope keeps stderr empty (§2.2 走 stdout), got %d bytes", stderr.Len())
	}
	var decoded struct {
		OK      bool   `json:"ok"`
		Outcome string `json:"outcome"`
		Data    any    `json:"data"`
		Meta    struct {
			Operation *OperationInfo `json:"operation"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !decoded.OK || decoded.Outcome != "pending" {
		t.Fatalf("pending envelope = ok:%v outcome:%q, want ok:true outcome:pending: %s",
			decoded.OK, decoded.Outcome, stdout.String())
	}
	op := decoded.Meta.Operation
	if op == nil {
		t.Fatalf("pending envelope must carry meta.operation (§2.2): %s", stdout.String())
	}
	if op.State != "processing" || !op.TimedOut {
		t.Fatalf("operation must keep real state + timed_out:true (§2.2 anti-spoof): got state=%q timed_out=%v", op.State, op.TimedOut)
	}
	if op.NextCommand != "dws wiki task-result --task task_abc" {
		t.Fatalf("next_command = %q, want executable recovery command (§2.2): %s", op.NextCommand, stdout.String())
	}
	if decoded.Data == nil {
		t.Fatalf("pending envelope data payload lost: %s", stdout.String())
	}
	// exit 0 语义标注（B32 与 B33 的耦合点）：异步受理 = ok:true → exit 0。
	if code := ExitCodeForEnvelope(env); code != 0 {
		t.Fatalf("pending envelope exit code = %d, want 0 (§1/§4/I2)", code)
	}
	// 非 json format 下 pending 同样走 stdout（数据通道），stderr 保持零字节。
	var outTable, errTable bytes.Buffer
	if err := NewEmitter(&outTable, &errTable, FormatTable, "", "").Emit(env); err != nil {
		t.Fatalf("Emit pending under -f table: %v", err)
	}
	if errTable.Len() != 0 {
		t.Fatalf("pending under -f table must keep stderr empty, got %d bytes", errTable.Len())
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("pending envelope must pass Validate(): %v", err)
	}
}

// --- B33：ExitCodeForEnvelope 的 outcome→exit code 映射 ---

func TestExitCodeForEnvelopeOutcomeCategoryMapping(t *testing.T) {
	cases := []struct {
		name string
		env  *Envelope
		want int
	}{
		{"success → 0", NewSuccessEnvelope(map[string]any{"id": "x"}), 0},
		{"pending → 0", NewPendingEnvelope(&OperationInfo{ID: "t_1", State: "processing"}), 0},
		{"partial_failure → 7", newPartialEnvelopeFixture(), 7},
		{"failure api → 1", NewFailureEnvelope(&ErrorInfo{Type: "api", Code: 90018}), 1},
		{"failure auth → 2", NewFailureEnvelope(&ErrorInfo{Type: "auth"}), 2},
		{"failure validation → 3", NewFailureEnvelope(&ErrorInfo{Type: "validation", Param: "--name"}), 3},
		{"failure discovery → 6", NewFailureEnvelope(&ErrorInfo{Type: "discovery"}), 6},
		{"failure internal → 5", NewFailureEnvelope(&ErrorInfo{Type: "internal"}), 5},
		{"failure unknown type → 5 (internal fallback)", NewFailureEnvelope(&ErrorInfo{Type: "mystery"}), 5},
		// confirmation 子类 → 3（AC-13；subtype 优先于 type，挂在任何 type 下恒 3）
		{"failure confirmation subtype on api → 3",
			NewFailureEnvelope(&ErrorInfo{Type: "api", Subtype: "confirmation_required",
				Actions: []string{"dws dev app delete --app-id a1 --yes"}}), 3},
		{"failure confirmation subtype on validation → 3",
			NewFailureEnvelope(&ErrorInfo{Type: "validation", Subtype: "confirmation_required"}), 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExitCodeForEnvelope(c.env); got != c.want {
				t.Fatalf("ExitCodeForEnvelope = %d, want %d", got, c.want)
			}
		})
	}
}

func TestExitCodeForEnvelopeNilEnvelopeIsInternal(t *testing.T) {
	// nil 信封与 WriteEnvelopeTo 兜底的 internal 类 failure 信封同源（B208）。
	if got := ExitCodeForEnvelope(nil); got != 5 {
		t.Fatalf("ExitCodeForEnvelope(nil) = %d, want 5 (internal-class fallback)", got)
	}
}

func TestExitCodeForEnvelopeIgnoresTamperedOKAndInvalidOutcome(t *testing.T) {
	// 只看 outcome（I1 同口径）：被篡改 OK 字段的失败信封仍非零。
	tampered := &Envelope{OK: true, Outcome: OutcomeFailure, Error: &ErrorInfo{Type: "api"}}
	if got := ExitCodeForEnvelope(tampered); got == 0 {
		t.Fatal("ExitCodeForEnvelope must ignore tampered OK field (outcome=failure is never exit 0)")
	}
	// 非法 outcome 属框架缺陷 → internal（5），不得落入 0。
	invalid := &Envelope{OK: true, Outcome: "succeeded"}
	if got := ExitCodeForEnvelope(invalid); got != 5 {
		t.Fatalf("ExitCodeForEnvelope(invalid outcome) = %d, want 5", got)
	}
}

// --- B34：I2 穷举断言——exit 0 ⇔ ok（对 ExitCodeForEnvelope 全组合）---

func TestInvariantI2ExhaustiveExitZeroIffOK(t *testing.T) {
	// I2：exit code == 0 ⇔ ok == true。穷举全部规范 outcome（合法装配形态，
	// 满足 I3）+ nil 信封 + 非法 outcome + 全部 failure 类别细分。
	legal := []struct {
		outcome Outcome
		env     *Envelope
	}{
		{OutcomeSuccess, NewSuccessEnvelope(map[string]any{"id": "x"})},
		{OutcomePending, NewPendingEnvelope(&OperationInfo{ID: "t_1", State: "processing"})},
		{OutcomePartialFailure, newPartialEnvelopeFixture()},
		{OutcomeFailure, NewFailureEnvelope(&ErrorInfo{Type: "api", Subtype: "rate_limit"})},
	}
	if len(legal) != 4 {
		t.Fatalf("I2 exhaustive table must cover exactly the four canonical outcomes, got %d", len(legal))
	}
	for _, c := range legal {
		wantOK := c.outcome == OutcomeSuccess || c.outcome == OutcomePending // I1
		code := ExitCodeForEnvelope(c.env)
		if (code == 0) != wantOK {
			t.Fatalf("outcome=%q: exit=%d but ok=%v (I2: exit 0 iff ok)", c.outcome, code, wantOK)
		}
		if (c.env.IsOK() != wantOK) || ((code == 0) != c.env.IsOK()) {
			t.Fatalf("outcome=%q: IsOK()=%v exit=%d disagree (I1/I2 cross-check)", c.outcome, c.env.IsOK(), code)
		}
	}

	// failure 全类别细分（api/auth/validation/discovery/internal/unknown/无明细）
	// 无一 exit 0：ok=false 分支的反向穷举。
	failureVariants := []*Envelope{
		NewFailureEnvelope(&ErrorInfo{Type: "api"}),
		NewFailureEnvelope(&ErrorInfo{Type: "auth"}),
		NewFailureEnvelope(&ErrorInfo{Type: "validation"}),
		NewFailureEnvelope(&ErrorInfo{Type: "discovery"}),
		NewFailureEnvelope(&ErrorInfo{Type: "internal"}),
		NewFailureEnvelope(&ErrorInfo{Type: "mystery"}),
		NewFailureEnvelope(&ErrorInfo{Subtype: "confirmation_required"}), // type 缺席 + confirmation 子类：仍 3，非 0
	}
	for i, env := range failureVariants {
		if got := ExitCodeForEnvelope(env); got == 0 {
			t.Fatalf("failure variant[%d] must never be exit 0 (I2)", i)
		}
	}

	// 边界组合：nil 信封与非法 outcome 同样非零（ok=false 语义）。
	if ExitCodeForEnvelope(nil) == 0 {
		t.Fatal("nil envelope must never be exit 0 (I2)")
	}
	if ExitCodeForEnvelope(&Envelope{OK: true, Outcome: "succeeded"}) == 0 {
		t.Fatal("invalid outcome must never be exit 0 (I2)")
	}
	// partial_failure 专用码精确断言（非零且恰为 7，§4）。
	if got := ExitCodeForEnvelope(newPartialEnvelopeFixture()); got != 7 {
		t.Fatalf("partial_failure exit code = %d, want dedicated 7 (§4)", got)
	}
}

// --- B32/B33 联动：Emitter 流路由与 ExitCodeForEnvelope 一致 ---

func TestEmitterStreamRoutingConsistentWithExitCode(t *testing.T) {
	// Emit 的流选择与退出码语义一一对应（exit 0 语义标注的行为端）：
	//   - ok:true（success/pending）→ stdout + exit 0（I2）；
	//   - partial_failure（ok:false）→ stdout（§2.3 stderr 零输出）+ exit 7；
	//   - failure（含 nil 降级）→ stderr + exit 非零，stdout 严格零字节。
	cases := []struct {
		name         string
		env          *Envelope
		wantStdout   bool // 信封数据通道是否落 stdout
		wantExitZero bool // I2：exit 0 ⇔ ok
	}{
		{"success", NewSuccessEnvelope(map[string]any{"id": "x"}), true, true},
		{"pending", NewPendingEnvelope(&OperationInfo{ID: "t_1", State: "processing"}), true, true},
		{"partial_failure", newPartialEnvelopeFixture(), true, false},
		{"failure", NewFailureEnvelope(&ErrorInfo{Type: "api"}), false, false},
		{"nil→fallback failure", nil, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			em := NewEmitter(&stdout, &stderr, FormatJSON, "", "")
			if err := em.Emit(c.env); err != nil {
				t.Fatalf("Emit: %v", err)
			}
			code := ExitCodeForEnvelope(c.env)
			if (code == 0) != c.wantExitZero {
				t.Fatalf("exit code = %d, wantExitZero=%v (I2)", code, c.wantExitZero)
			}
			if c.wantStdout {
				if stdout.Len() == 0 {
					t.Fatal("stdout-routed envelope must carry payload on stdout")
				}
				if stderr.Len() != 0 {
					t.Fatalf("stdout-routed envelope must keep stderr empty (§2.3/§5.1), got %d bytes", stderr.Len())
				}
			} else {
				if stdout.Len() != 0 {
					t.Fatalf("failure must leave stdout empty, got %d bytes", stdout.Len())
				}
				if stderr.Len() == 0 {
					t.Fatal("failure envelope must be emitted on stderr")
				}
			}
			// partial_failure 专用码精确断言（§4）：stdout 通道但非零。
			if c.env != nil && c.env.Outcome == OutcomePartialFailure && code != 7 {
				t.Fatalf("partial_failure exit code = %d, want dedicated 7", code)
			}
		})
	}
}

// --- B208：nil 兜底信封 I3 合法化（轮6裁决⑨）---

func TestNilFallbackEnvelopeCarriesInternalErrorInfo(t *testing.T) {
	// 兜底信封必须携带 internal 类 ErrorInfo（框架内部缺陷），
	// 自身通过 Validate()——不得再是被校验器拒绝的形态。
	env := nilFallbackEnvelope()
	if env.Error == nil {
		t.Fatal("nil fallback envelope must carry ErrorInfo (I3: error 非空 ⇔ failure)")
	}
	if env.Error.Type != "internal" {
		t.Fatalf("fallback error.type = %q, want internal", env.Error.Type)
	}
	if env.Error.Message != "nil envelope: framework fallback" {
		t.Fatalf("fallback error.message = %q, want %q", env.Error.Message, "nil envelope: framework fallback")
	}
	if env.OK || env.Outcome != OutcomeFailure {
		t.Fatalf("fallback envelope = ok:%v outcome:%q, want ok:false outcome:failure", env.OK, env.Outcome)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("nil fallback envelope must pass Validate(), got %v", err)
	}
}

func TestWriteEnvelopeToNilEnvelopeEmitsLegalFailure(t *testing.T) {
	// WriteEnvelopeTo 的 nil 兜底：不 panic、必产合法 failure，
	// error 明细在 wire 上可解码且类型/消息正确。
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, nil, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(nil) must not fail: %v", err)
	}
	var decoded struct {
		OK      bool       `json:"ok"`
		Outcome string     `json:"outcome"`
		Error   *ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("fallback envelope is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded.OK || decoded.Outcome != "failure" {
		t.Fatalf("fallback = ok:%v outcome:%q, want ok:false outcome:failure: %s", decoded.OK, decoded.Outcome, buf.String())
	}
	if decoded.Error == nil {
		t.Fatalf("fallback envelope must carry error detail on wire: %s", buf.String())
	}
	if decoded.Error.Type != "internal" || decoded.Error.Message != "nil envelope: framework fallback" {
		t.Fatalf("fallback error = %+v, want internal / nil envelope: framework fallback", decoded.Error)
	}
	// 兜底信封与 ExitCodeForEnvelope 同源：internal → 5（非零，I2）。
	if code := ExitCodeForEnvelope(nil); code == 0 {
		t.Fatal("nil fallback must map to non-zero exit code")
	}
}
