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
	"strings"
	"testing"
	"time"
)

// TestErrorsPrintJSONFieldInventory protects the published legacy error wire.
// Unified type/subtype/outcome fields belong to internal/output and must not
// leak into commands that have not migrated.
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

	for _, want := range []string{
		`"category": "api"`,
		`"reason": "rate_limit"`,
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
	for _, forbidden := range []string{`"outcome"`, `"type"`, `"subtype"`} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unified field %s leaked into legacy wire: %s", forbidden, got)
		}
	}
}

func TestErrorsPrintJSONLegacyWireGolden(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewValidation("missing", WithReason("missing_required_flags"))); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	want := "{\n  \"error\": {\n    \"category\": \"validation\",\n    \"code\": 3,\n    \"message\": \"missing\",\n    \"reason\": \"missing_required_flags\"\n  }\n}\n"
	if got := b.String(); got != want {
		t.Fatalf("legacy error wire changed\n got: %q\nwant: %q", got, want)
	}
}

func TestErrorsPrintJSONReasonProjectionStaysLegacy(t *testing.T) {
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
		if !strings.Contains(got, `"reason": "confirmation_required"`) {
			t.Fatalf("expected confirmation_required reason, got %q", got)
		}
		if strings.Contains(got, `"subtype"`) || strings.Contains(got, `"type"`) {
			t.Fatalf("unified fields leaked into legacy wire: %q", got)
		}
	})

	t.Run("no reason omits reason", func(t *testing.T) {
		var b strings.Builder
		if err := PrintJSON(&b, NewAPI("plain")); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if strings.Contains(b.String(), `"reason"`) {
			t.Fatalf("reason must be omitted when Reason is empty, got %q", b.String())
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
	// Legacy reason distinguishes the confirmation subtype.
	var b strings.Builder
	if err := PrintJSON(&b, confirmation); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if !strings.Contains(b.String(), `"reason": "confirmation_required"`) {
		t.Fatalf("confirmation must carry reason, got %q", b.String())
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
	if !strings.Contains(b.String(), `"code": 5`) || !strings.Contains(b.String(), `"category": "internal"`) {
		t.Fatalf("partial error must not masquerade as a partial result: %q", b.String())
	}
}

func TestErrorsPrintJSONKeepsLegacyCategories(t *testing.T) {
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
			if !strings.Contains(got, `"category": "`+tc.want+`"`) {
				t.Fatalf("expected legacy category %q in %s", tc.want, got)
			}
			if strings.Contains(got, `"type"`) {
				t.Fatalf("unified type leaked into legacy JSON: %s", got)
			}
		})
	}
}

// TestErrorsWireStableFieldsSubset protects the legacy recovery fields that
// remain useful without changing the top-level envelope.
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
		`"category"`, `"reason"`, `"code"`, `"retryable"`, `"retry_after_seconds"`,
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
		name     string
		err      error
		category string
		code     string
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
			if !strings.Contains(got, `"category": "`+tc.category+`"`) || !strings.Contains(got, tc.code) {
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

func TestErrorsLegacyPrintJSONOmitsUnifiedOutcome(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	if err := PrintJSON(&b, NewInternal("x")); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if strings.Contains(b.String(), `"outcome"`) {
		t.Fatalf("legacy error JSON must not carry unified outcome, got %q", b.String())
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
