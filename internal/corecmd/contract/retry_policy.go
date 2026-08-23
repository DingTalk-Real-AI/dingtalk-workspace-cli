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

import (
	"fmt"
	"strings"
)

const RetryModeDeduplicationKey = "deduplication_key"

// RetryPolicySpec declares the reviewed condition that makes an otherwise
// non-idempotent operation safe to retry. KeyParameter names the CLI parameter;
// Schema assembly binds it to one explicit interface property before runtime.
// No flag-name or request-property inference is permitted.
type RetryPolicySpec struct {
	Mode                string `json:"mode"`
	KeyParameter        string `json:"key_parameter"`
	SamePayloadRequired bool   `json:"same_payload_required"`
}

// RetryDecision is the invocation-scoped safety result consumed by Agent and
// transport code. Static metadata remains unchanged; EffectiveIdempotency is
// derived only from the reviewed policy and the actual invocation arguments.
type RetryDecision struct {
	EffectiveIdempotency string `json:"effective_idempotency"`
	SafeToRetry          bool   `json:"safe_to_retry"`
	Reason               string `json:"reason"`
}

// NormalizeRetryPolicySpec validates and defensively copies a retry policy.
func NormalizeRetryPolicySpec(in *RetryPolicySpec, canonical string) (*RetryPolicySpec, error) {
	if in == nil {
		return nil, nil
	}
	canonical = defaultString(strings.TrimSpace(canonical), "<unknown>")
	out := &RetryPolicySpec{
		Mode:                strings.TrimSpace(in.Mode),
		KeyParameter:        strings.TrimSpace(in.KeyParameter),
		SamePayloadRequired: in.SamePayloadRequired,
	}
	if out.Mode != RetryModeDeduplicationKey {
		return nil, fmt.Errorf("schema tool %s retry_policy has unsupported mode %q", canonical, out.Mode)
	}
	if out.KeyParameter == "" {
		return nil, fmt.Errorf("schema tool %s retry_policy has no key_parameter", canonical)
	}
	if !out.SamePayloadRequired {
		return nil, fmt.Errorf("schema tool %s retry_policy must require the same payload", canonical)
	}
	return out, nil
}

// ResolveRetryDecision derives invocation safety without mutating the static
// command contract. argumentKey is the CLI parameter name for Agent calls or
// the explicitly declared interface property for transport calls.
func ResolveRetryDecision(idempotency string, policy *RetryPolicySpec, arguments map[string]any, argumentKey string) RetryDecision {
	switch strings.TrimSpace(idempotency) {
	case "idempotent":
		return RetryDecision{EffectiveIdempotency: "idempotent", SafeToRetry: true, Reason: "static_idempotent"}
	case "non_idempotent":
		return RetryDecision{EffectiveIdempotency: "non_idempotent", Reason: "static_non_idempotent"}
	case "unknown":
		return RetryDecision{EffectiveIdempotency: "unknown", Reason: "idempotency_unknown"}
	case "conditional":
		normalized, err := NormalizeRetryPolicySpec(policy, "<invocation>")
		if err != nil || normalized == nil {
			return RetryDecision{EffectiveIdempotency: "unknown", Reason: "invalid_retry_policy"}
		}
		argumentKey = strings.TrimSpace(argumentKey)
		if argumentKey == "" {
			return RetryDecision{EffectiveIdempotency: "unknown", Reason: "deduplication_key_binding_missing"}
		}
		value, ok := arguments[argumentKey].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return RetryDecision{EffectiveIdempotency: "non_idempotent", Reason: "deduplication_key_missing"}
		}
		return RetryDecision{EffectiveIdempotency: "idempotent", SafeToRetry: true, Reason: "deduplication_key_present"}
	default:
		return RetryDecision{EffectiveIdempotency: "unknown", Reason: "idempotency_unknown"}
	}
}
