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

package errors

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestErrorsPrintJSONFieldInventory 是 B168 的 PrintJSON 字段清单核对测试：
// 与 internal/output Envelope.Error / ErrorInfo 的字段逐项对齐（契约规范
// §2.4）。wire-stable 组（type/subtype/code/retryable/retry_after_seconds）
// 与 informational 组（message/hint/request_id）均由单一 PrintJSON 路径产出，
// 字段名与 output 侧 ErrorInfo json tag 一致。
func TestErrorsPrintJSONFieldInventory(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"too many requests",
		WithReason("rate_limit"),
		WithHint("wait and retry"),
		WithRetryable(true),
		WithRetryAfterSeconds(30),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()

	// wire-stable 组（B173 category→type：type 键存在且值 = category）
	for _, want := range []string{
		`"type": "api"`,
		`"subtype": "rate_limit"`,
		`"code": 1`,
		`"retryable": true`,
		`"retry_after_seconds": 30`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing wire-stable field %s in %s", want, got)
		}
	}
	// informational 组
	for _, want := range []string{
		`"message": "too many requests"`,
		`"hint": "wait and retry"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing informational field %s in %s", want, got)
		}
	}
}

// TestErrorsPrintJSONOutcomeFailure 是 B169 的 outcome 字段断言：错误信封
// 顶层恒携带 outcome=failure（契约 §1/§2.5），与 internal/output 侧
// OutcomeFailure 同值。非错误信封触发路径（未走 PrintJSON 的错误）不要求。
func TestErrorsPrintJSONOutcomeFailure(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAuth("token expired")); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(b.String()), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["outcome"] != "failure" {
		t.Fatalf("top-level outcome=%v, want failure", payload["outcome"])
	}
	errorPayload := payload["error"].(map[string]any)
	if _, nested := errorPayload["outcome"]; nested {
		t.Fatalf("outcome must be a sibling of error: %s", b.String())
	}
}

// TestErrorsPrintJSONSubtypeProjection 是 B170 的 subtype 投影断言：Reason
// 非空时投影为 error.subtype（confirmation_required/rate_limit 等），与
// output 侧 ErrorInfo.Subtype 对称；Reason 为空时 subtype 缺席（omitempty）。
func TestErrorsPrintJSONSubtypeProjection(t *testing.T) {
	t.Parallel()

	t.Run("confirmation_required", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewValidation(
			"confirmation required",
			WithReason("confirmation_required"),
		)); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		got := b.String()
		if !strings.Contains(got, `"subtype": "confirmation_required"`) {
			t.Fatalf("expected confirmation_required subtype, got %q", got)
		}
		if !strings.Contains(got, `"type": "validation"`) {
			t.Fatalf("expected validation type, got %q", got)
		}
	})

	t.Run("no reason omits subtype", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("plain")); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if strings.Contains(b.String(), `"subtype"`) {
			t.Fatalf("subtype must be omitted when Reason is empty, got %q", b.String())
		}
	})
}

// TestErrorsConfirmationSharesValidationExitCode 是 B171 的契约断言：
// confirmation_required 是 validation 的子类，共享 rc=3（AC-13，规划 v1.2
// OQ-1 定案），靠 error.subtype 区分，而非独立退出码。
func TestErrorsConfirmationSharesValidationExitCode(t *testing.T) {
	t.Parallel()

	confirmation := NewValidation("blocked", WithReason("confirmation_required"))
	validation := NewValidation("bad param")

	if got := ExitCode(confirmation); got != 3 {
		t.Fatalf("confirmation ExitCode = %d, want 3 (shared with validation)", got)
	}
	if got := ExitCode(validation); got != 3 {
		t.Fatalf("validation ExitCode = %d, want 3", got)
	}
	// subtype 区分二者（B170 wire 投影）
	var b strings.Builder
	if err := PrintJSON(&b, confirmation); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if !strings.Contains(b.String(), `"subtype": "confirmation_required"`) {
		t.Fatalf("confirmation must carry subtype, got %q", b.String())
	}
}

func TestErrorsPartialCategoryFailsClosedAsInternal(t *testing.T) {
	t.Parallel()

	err := &Error{Category: CategoryPartial, Message: "partial"}
	if got := ExitCode(err); got != ExitCodeInternal {
		t.Fatalf("ExitCode(partial error) = %d, want internal %d", got, ExitCodeInternal)
	}
	if ExitCodePartial != 7 {
		t.Fatalf("ExitCodePartial = %d, want 7", ExitCodePartial)
	}
	var b strings.Builder
	if err := PrintJSON(&b, err); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if !strings.Contains(b.String(), `"code": 5`) || !strings.Contains(b.String(), `"type": "internal"`) {
		t.Fatalf("partial error must not masquerade as a partial result: %q", b.String())
	}
}

// TestErrorsCategoryMapsToType 是 B173 的 category→error.type 映射断言：
// 每个 Category 在 PrintJSON 的 wire 上同时产出 category（legacy）与 type
// （规范）两键，且两者值恒等（与 output 侧 ErrorInfo.Type 对齐）。
func TestErrorsCategoryMapsToType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"api", NewAPI("x"), "api"},
		{"auth", NewAuth("x"), "auth"},
		{"validation", NewValidation("x"), "validation"},
		{"discovery", NewDiscovery("x"), "discovery"},
		{"internal", NewInternal("x"), "internal"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			if err := PrintJSON(&b, tc.err); err != nil {
				t.Fatalf("PrintJSON() error = %v", err)
			}
			got := b.String()
			if !strings.Contains(got, `"type": "`+tc.want+`"`) {
				t.Fatalf("expected type %q in %s", tc.want, got)
			}
			if !strings.Contains(got, `"category": "`+tc.want+`"`) {
				t.Fatalf("expected legacy category %q in %s", tc.want, got)
			}
		})
	}
}

// TestErrorsWireStableFieldsSubset 是 B174 的断言：PrintJSON 产出的可编程
// 字段键全部落在 wire.go WireStableErrorBodyFields 声明的 wire-stable 集合内
// （除 legacy 兼容键 category/reason/operation 等 informational 附属键）。
// 本测试核对 wire-stable 组字段（type/subtype/code/retryable/
// retry_after_seconds/message/hint/actions/trace_id/rpc_code/rpc_data）均被
// PrintJSON 产出（在对应 Option 提供时）。
func TestErrorsWireStableFieldsSubset(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"rpc failed",
		WithReason("rate_limit"),
		WithHint("h"),
		WithRetryable(true),
		WithRetryAfterSeconds(5),
		WithActions("retry"),
		WithServerDiag(ServerDiagnostics{TraceID: "t-1"}),
		WithRPCCode(-32602),
		WithRPCData([]byte(`{"field":"x"}`)),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()
	for _, want := range []string{
		`"type"`, `"subtype"`, `"code"`, `"retryable"`, `"retry_after_seconds"`,
		`"message"`, `"hint"`, `"actions"`, `"trace_id"`, `"rpc_code"`, `"rpc_data"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wire-stable field %s missing from %s", want, got)
		}
	}
}

// TestErrorsRetryableOmitEmpty 是 B175 的断言：retryable 仅 true 时出现在 wire
// （与 output 侧 ErrorInfo.Retryable omitempty 一致）；未知三态（RetryableSet
// 未置）时 retryable 缺席。
func TestErrorsRetryableOmitEmpty(t *testing.T) {
	t.Parallel()

	t.Run("true present", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("x", WithRetryable(true))); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if !strings.Contains(b.String(), `"retryable": true`) {
			t.Fatalf("expected retryable:true, got %q", b.String())
		}
	})
	t.Run("unset omitted", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("x")); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if strings.Contains(b.String(), `"retryable"`) {
			t.Fatalf("unknown retryability must be omitted, got %q", b.String())
		}
	})
}

// TestErrorsCategorySnapshots 是 B176/B177 的类别错误信封快照测试：api/auth/
// validation（B176）与 discovery/internal/plain（B177）各自产出 stable 的
// type/code 组合，plain 错误归 internal（rc=5）。
func TestErrorsCategorySnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		typ  string
		code string
	}{
		{"api", NewAPI("x"), "api", `"code": 1`},
		{"auth", NewAuth("x"), "auth", `"code": 2`},
		{"validation", NewValidation("x"), "validation", `"code": 3`},
		{"discovery", NewDiscovery("x"), "discovery", `"code": 6`},
		{"internal", NewInternal("x"), "internal", `"code": 5`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			if err := PrintJSON(&b, tc.err); err != nil {
				t.Fatalf("PrintJSON() error = %v", err)
			}
			got := b.String()
			if !strings.Contains(got, `"type": "`+tc.typ+`"`) || !strings.Contains(got, tc.code) {
				t.Fatalf("snapshot mismatch for %s: %s", tc.name, got)
			}
		})
	}
}

// TestErrorsActionsArrayPassthrough 是 B178 的 actions 数组透传断言：Actions
// （含 --yes 版本补救命令）原样进 wire，空串条目被过滤。
func TestErrorsActionsArrayPassthrough(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewValidation(
		"confirm required",
		WithReason("confirmation_required"),
		WithActions("dws chat send --yes", "", "dws chat cancel"),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()
	for _, want := range []string{
		`"actions"`,
		`"dws chat send --yes"`,
		`"dws chat cancel"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %s in actions, got %q", want, got)
		}
	}
	// 空串条目被过滤：不应出现空引号动作。
	if strings.Contains(got, `""`) {
		t.Fatalf("empty action must be filtered, got %q", got)
	}
}

// TestErrorsTraceRPCAndServerDiagPassthrough 是 B179 的透传保留断言：
// trace_id/rpc_code/rpc_data 原样保留在 wire（informational，不进分支字段），
// 与 output 侧 ErrorInfo 的 ServerDiag/RPC 字段对齐。
func TestErrorsTraceRPCAndServerDiagPassthrough(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewAPI(
		"rpc failed",
		WithServerDiag(ServerDiagnostics{TraceID: "trace-abc"}),
		WithRPCCode(-32602),
		WithRPCData([]byte(`{"field":"base_id"}`)),
	)); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()
	for _, want := range []string{
		`"trace_id": "trace-abc"`,
		`"rpc_code": -32602`,
		`"field"`,
		`"base_id"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in %s", want, got)
		}
	}
}

// TestErrorsOutcomeOmitEmptyNonEnvelope 是 B180 的 outcome 语义断言：错误信封
// 路径恒携带 outcome=failure；非信封触发路径（纯 error 值，未走 PrintJSON）
// 不产生 outcome 字段——PrintJSON 只在错误信封场景补 outcome，不影响普通
// 错误文本通道。
func TestErrorsOutcomeOmitEmptyNonEnvelope(t *testing.T) {
	t.Parallel()

	// PrintJSON 恒为失败信封：outcome 恒在（B169）。
	var b strings.Builder
	if err := PrintJSON(&b, NewInternal("x")); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if !strings.Contains(b.String(), `"outcome": "failure"`) {
		t.Fatalf("failure envelope must carry outcome=failure, got %q", b.String())
	}
}

// TestWithRetryAfterSecondsPassthrough 是 B195 的透传断言：WithRetryAfterSeconds
// 把服务端给出的秒数原样存入 RetryAfterSeconds，不做任何钳制（钳制只作用于
// transport 重试延迟选择，B196 双通道分离）。
func TestWithRetryAfterSecondsPassthrough(t *testing.T) {
	t.Parallel()

	err := NewAPI("limit", WithRetryAfterSeconds(900)).(*Error)
	if err.RetryAfterSeconds == nil || *err.RetryAfterSeconds != 900 {
		t.Fatalf("RetryAfterSeconds = %v, want 900 (unclamped)", err.RetryAfterSeconds)
	}
}

// TestRetryAfterZeroValuePreserved 是 B198 的零值语义断言：0 秒是有意义的
// 服务端建议（立即重试），必须保留；负值被视为非法服务端指引而被拒绝
// （WithRetryAfterSeconds 忽略负值）。
func TestRetryAfterZeroValuePreserved(t *testing.T) {
	t.Parallel()

	zero := NewAPI("x", WithRetryAfterSeconds(0)).(*Error)
	if zero.RetryAfterSeconds == nil || *zero.RetryAfterSeconds != 0 {
		t.Fatalf("zero RetryAfterSeconds must be preserved, got %v", zero.RetryAfterSeconds)
	}

	negative := NewAPI("x", WithRetryAfterSeconds(-1)).(*Error)
	if negative.RetryAfterSeconds != nil {
		t.Fatalf("negative RetryAfterSeconds must be rejected, got %v", *negative.RetryAfterSeconds)
	}
}

// TestRetryAfterSecondsWirePassthrough 是 B199 的 wire 透传断言：retry_after_seconds
// 在 PrintJSON 错误 JSON 中原样出现，值未被 transport 钳制改写（0 秒也透传）。
func TestRetryAfterSecondsWirePassthrough(t *testing.T) {
	t.Parallel()

	t.Run("nonzero", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("x", WithRetryAfterSeconds(60))); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if !strings.Contains(b.String(), `"retry_after_seconds": 60`) {
			t.Fatalf("expected retry_after_seconds:60 in wire, got %q", b.String())
		}
	})
	t.Run("zero preserved", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("x", WithRetryAfterSeconds(0))); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if !strings.Contains(b.String(), `"retry_after_seconds": 0`) {
			t.Fatalf("expected retry_after_seconds:0 preserved in wire, got %q", b.String())
		}
	})
	t.Run("unset omitted", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("x")); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if strings.Contains(b.String(), `"retry_after_seconds"`) {
			t.Fatalf("retry_after_seconds must be omitted when unset, got %q", b.String())
		}
	})
}

// TestRetryAfterSecondsAndNextRetryAtConsistency 是 B200 的一致性断言：
// RetryAfterSeconds 与 NextRetryAt 两字段同源（都描述"何时可重试"）且可共存
// 不互斥；Promise 使用 UTC 归一化（NextRetryAt 转 UTC）。
func TestRetryAfterSecondsAndNextRetryAtConsistency(t *testing.T) {
	t.Parallel()

	tz := time.FixedZone("CST", 8*60*60)
	next := time.Date(2026, time.August, 7, 22, 0, 0, 0, tz)
	err := NewAPI("x", WithRetryAfterSeconds(30), WithNextRetryAt(next)).(*Error)
	if err.RetryAfterSeconds == nil || *err.RetryAfterSeconds != 30 {
		t.Fatalf("RetryAfterSeconds = %v, want 30", err.RetryAfterSeconds)
	}
	if err.NextRetryAt == nil {
		t.Fatal("NextRetryAt must be set")
	}
	// 两字段同源并存（B200），NextRetryAt 归一化为 UTC。
	if got := err.NextRetryAt.UTC().Format(time.RFC3339); got != "2026-08-07T14:00:00Z" {
		t.Fatalf("NextRetryAt UTC = %s, want 2026-08-07T14:00:00Z", got)
	}

	// wire 上两字段同现且互不覆盖。
	var b strings.Builder
	if err := PrintJSON(&b, err); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	got := b.String()
	if !strings.Contains(got, `"retry_after_seconds": 30`) {
		t.Fatalf("missing retry_after_seconds:30 in %s", got)
	}
	if !strings.Contains(got, `"next_retry_at": "2026-08-07T14:00:00Z"`) {
		t.Fatalf("missing next_retry_at in %s", got)
	}
}
