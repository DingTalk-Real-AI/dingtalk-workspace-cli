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

package contract

import "testing"

func TestCrossPlatformCoverageNormalizeRetryPolicySpec(t *testing.T) {
	if got, err := NormalizeRetryPolicySpec(nil, "chat.send"); err != nil || got != nil {
		t.Fatalf("nil NormalizeRetryPolicySpec() = %#v, %v", got, err)
	}
	got, err := NormalizeRetryPolicySpec(&RetryPolicySpec{
		Mode:                " deduplication_key ",
		KeyParameter:        " uuid ",
		SamePayloadRequired: true,
	}, "chat.send")
	if err != nil {
		t.Fatalf("NormalizeRetryPolicySpec() error = %v", err)
	}
	if got.Mode != RetryModeDeduplicationKey || got.KeyParameter != "uuid" || !got.SamePayloadRequired {
		t.Fatalf("NormalizeRetryPolicySpec() = %#v", got)
	}
	if _, err := NormalizeRetryPolicySpec(&RetryPolicySpec{Mode: RetryModeDeduplicationKey, SamePayloadRequired: true}, "chat.send"); err == nil {
		t.Fatal("empty key_parameter must fail closed")
	}
	if _, err := NormalizeRetryPolicySpec(&RetryPolicySpec{Mode: RetryModeDeduplicationKey, KeyParameter: "uuid"}, "chat.send"); err == nil {
		t.Fatal("same_payload_required=false must fail closed")
	}
	if _, err := NormalizeRetryPolicySpec(&RetryPolicySpec{Mode: "inferred", KeyParameter: "uuid", SamePayloadRequired: true}, "chat.send"); err == nil {
		t.Fatal("unsupported retry mode must fail closed")
	}
}

func TestCrossPlatformCoverageResolveRetryDecision(t *testing.T) {
	policy := &RetryPolicySpec{Mode: RetryModeDeduplicationKey, KeyParameter: "uuid", SamePayloadRequired: true}
	if got := ResolveRetryDecision("conditional", policy, map[string]any{"uuid": "request-1"}, "uuid"); !got.SafeToRetry || got.EffectiveIdempotency != "idempotent" {
		t.Fatalf("conditional with key = %#v", got)
	}
	for name, args := range map[string]map[string]any{
		"missing":    {},
		"empty":      {"uuid": "  "},
		"non-string": {"uuid": 123},
	} {
		t.Run(name, func(t *testing.T) {
			got := ResolveRetryDecision("conditional", policy, args, "uuid")
			if got.SafeToRetry || got.EffectiveIdempotency != "non_idempotent" {
				t.Fatalf("decision = %#v", got)
			}
		})
	}
	if got := ResolveRetryDecision("idempotent", nil, nil, ""); !got.SafeToRetry || got.EffectiveIdempotency != "idempotent" {
		t.Fatalf("static idempotent = %#v", got)
	}
	if got := ResolveRetryDecision("non_idempotent", nil, nil, ""); got.SafeToRetry || got.EffectiveIdempotency != "non_idempotent" {
		t.Fatalf("static non-idempotent = %#v", got)
	}
	if got := ResolveRetryDecision("unknown", nil, nil, ""); got.SafeToRetry || got.EffectiveIdempotency != "unknown" {
		t.Fatalf("unknown = %#v", got)
	}
	if got := ResolveRetryDecision("conditional", policy, map[string]any{"uuid": "request-1"}, ""); got.SafeToRetry || got.EffectiveIdempotency != "unknown" || got.Reason != "deduplication_key_binding_missing" {
		t.Fatalf("missing key binding = %#v", got)
	}
	if got := ResolveRetryDecision("conditional", nil, nil, ""); got.SafeToRetry || got.Reason != "invalid_retry_policy" {
		t.Fatalf("invalid conditional policy = %#v", got)
	}
	if got := ResolveRetryDecision("retryable", nil, nil, ""); got.SafeToRetry || got.EffectiveIdempotency != "unknown" {
		t.Fatalf("unknown legacy value = %#v", got)
	}
}
