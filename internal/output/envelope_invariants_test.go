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
	"strings"
	"testing"
)

// --- B21：IsOK() I1 框架计算函数 ---

func TestEnvelopeIsOKComputesFromOutcome(t *testing.T) {
	// IsOK 只看 outcome，不看（可能被命令层篡改的）OK 字段——
	// 「由框架计算，命令层不可写」（契约规范 §1 I1）。
	env := &Envelope{OK: true, Outcome: OutcomeFailure}
	if env.IsOK() {
		t.Fatalf("IsOK must ignore the tampered OK field: outcome=failure requires false")
	}
	env2 := &Envelope{OK: false, Outcome: OutcomeSuccess}
	if !env2.IsOK() {
		t.Fatalf("IsOK must compute from outcome: success requires true regardless of OK field")
	}
}

func TestEnvelopeIsOKNilAndInvalidOutcome(t *testing.T) {
	var env *Envelope
	if env.IsOK() {
		t.Fatalf("nil envelope must not be ok: no outcome means no success")
	}
	for _, bad := range []Outcome{"", "SUCCESS", "Failure", "ok", "partial"} {
		if (&Envelope{Outcome: bad}).IsOK() {
			t.Fatalf("IsOK(%q) = true, want false: I1 is defined only over the four canonical outcomes", bad)
		}
	}
}

// --- B22：I1 全四值穷举 ---

func TestInvariantI1ExhaustiveOverFourOutcomes(t *testing.T) {
	// I1：ok == (outcome ∈ {success, pending})，四值全穷举。
	cases := []struct {
		outcome Outcome
		wantOK  bool
	}{
		{OutcomeSuccess, true},
		{OutcomePending, true},
		{OutcomePartialFailure, false},
		{OutcomeFailure, false},
	}
	if len(cases) != 4 {
		t.Fatalf("I1 exhaustive table must cover exactly the four canonical outcomes, got %d", len(cases))
	}
	for _, c := range cases {
		// failure 形态附 error 以同时满足 I3（error 非空 ⇔ outcome==failure），
		// 使 Validate() 对 I1 一致信封整体放行。
		var errPtr *ErrorInfo
		if c.outcome == OutcomeFailure {
			errPtr = &ErrorInfo{Type: "api"}
		}
		env := &Envelope{OK: c.wantOK, Outcome: c.outcome, Error: errPtr}
		if got := env.IsOK(); got != c.wantOK {
			t.Fatalf("IsOK() for outcome=%q = %v, want %v (I1)", c.outcome, got, c.wantOK)
		}
		if env.OK != env.IsOK() {
			t.Fatalf("envelope ok=%v disagrees with framework-computed IsOK=%v for outcome=%q (I1)",
				env.OK, env.IsOK(), c.outcome)
		}
		if err := env.Validate(); err != nil {
			t.Fatalf("Validate() on I1-consistent outcome=%q envelope = %v, want nil", c.outcome, err)
		}
		// 反向：人为翻转 OK 字段 → Validate 必须报 I1 违反。
		tampered := &Envelope{OK: !c.wantOK, Outcome: c.outcome}
		if tampered.IsOK() != c.wantOK {
			t.Fatalf("IsOK must stay outcome-driven when OK field is tampered (outcome=%q)", c.outcome)
		}
		if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "I1") {
			t.Fatalf("Validate() on tampered outcome=%q envelope must report I1 violation, got %v", c.outcome, err)
		}
	}
}

func TestInvariantI1ConstructorAgreement(t *testing.T) {
	// 四个构造器产出的信封，其 OK 字段与 IsOK() 恒一致（I1 的装配端闭环）。
	envs := map[string]*Envelope{
		"success":         NewSuccessEnvelope(nil),
		"pending":         NewPendingEnvelope(nil),
		"partial_failure": NewPartialEnvelope(nil),
		"failure":         NewFailureEnvelope(&ErrorInfo{Type: "api"}),
	}
	for name, env := range envs {
		if env.OK != env.IsOK() {
			t.Fatalf("constructor %q: OK=%v but IsOK()=%v (I1)", name, env.OK, env.IsOK())
		}
	}
}

// --- B23：I3 双向穷举（error 非空 ⇔ outcome==failure）---

func TestInvariantI3BidirectionalExhaustive(t *testing.T) {
	// 双向穷举：4 个 outcome × {error 缺席, error 非空} = 8 组合，
	// 仅 outcome==failure ⇔ error 非空 的 2 组合合法（对角线两侧各一）。
	errInfo := &ErrorInfo{Type: "api", Code: 90018, Message: "too many requests"}
	for _, outcome := range canonicalOutcomes {
		for _, errPtr := range []*ErrorInfo{nil, errInfo} {
			env := &Envelope{
				OK:      outcome == OutcomeSuccess || outcome == OutcomePending,
				Outcome: outcome,
				Error:   errPtr,
			}
			errorPresent := env.Error != nil
			isFailure := env.Outcome == OutcomeFailure
			wantValid := errorPresent == isFailure
			err := env.Validate()
			if wantValid && err != nil {
				t.Fatalf("outcome=%q errorPresent=%v must satisfy I3, Validate()=%v", outcome, errorPresent, err)
			}
			if !wantValid {
				if err == nil {
					t.Fatalf("outcome=%q errorPresent=%v violates I3 but Validate() passed", outcome, errorPresent)
				}
				if !strings.Contains(err.Error(), "I3") {
					t.Fatalf("I3 violation must be reported as such, got %v", err)
				}
			}
		}
	}
}

func TestInvariantI3WireDirectionFromConstructors(t *testing.T) {
	// wire 方向断言：四类构造器产出的信封中，error 字段出现当且仅当
	// outcome==failure（「⇔」的两个方向都要显式断言）。
	envs := []*Envelope{
		NewSuccessEnvelope(map[string]any{"id": "x"}),
		NewPendingEnvelope(&OperationInfo{ID: "t_1", State: "processing"}),
		NewPartialEnvelope(map[string]any{"total": 1}),
		NewFailureEnvelope(&ErrorInfo{Type: "validation", Message: "bad"}),
	}
	for _, env := range envs {
		errorPresent := env.Error != nil
		isFailure := env.Outcome == OutcomeFailure
		if errorPresent != isFailure {
			t.Fatalf("constructor envelope outcome=%q: error present=%v, want == isFailure(%v) (I3)",
				env.Outcome, errorPresent, isFailure)
		}
	}
}
