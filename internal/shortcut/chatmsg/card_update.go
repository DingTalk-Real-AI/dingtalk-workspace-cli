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

package chatmsg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var (
	ErrCardUpdateNotApplied = errors.New("streaming card update was not applied")
	ErrCardUpdateUnverified = errors.New("streaming card update could not be verified")
	ErrCardUpdateBizIDDrift = errors.New("streaming card update returned a different bizId")
)

// NormalizeCardBizID performs only format-independent checks. bizId is an
// opaque server-issued identifier; a stricter character or prefix contract
// must not be invented by the CLI without an authoritative API declaration.
func NormalizeCardBizID(raw string) (string, error) {
	bizID := strings.TrimSpace(raw)
	if bizID == "" {
		return "", fmt.Errorf("--biz-id 不能为空")
	}
	for _, r := range bizID {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", fmt.Errorf("--biz-id 必须是 send-card 返回的单个不透明 ID，不能包含空白或控制字符")
		}
	}
	if isCardBizIDPlaceholder(bizID) {
		return "", fmt.Errorf("--biz-id 仍是占位符 %q；请传入 send-card 实际返回的 bizId", bizID)
	}
	return bizID, nil
}

func isCardBizIDPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "bizid", "biz-id", "your-biz-id", "your_biz_id",
		"<bizid>", "<biz-id>", "<your-biz-id>",
		"{bizid}", "{biz-id}", "${bizid}", "${biz-id}":
		return true
	default:
		return false
	}
}

// VerifyStreamingCardUpdate requires affirmative evidence that the requested
// write took effect. update_streaming_card may acknowledge an applied write
// with success=true without returning an updated flag or affected count.
func VerifyStreamingCardUpdate(requestedBizID string, response map[string]any) (string, error) {
	requestedBizID = strings.TrimSpace(requestedBizID)
	observation := cardUpdateObservation{bizIDs: map[string]struct{}{}}
	observeCardUpdate(response, &observation)

	for responseBizID := range observation.bizIDs {
		if requestedBizID != "" && responseBizID != requestedBizID {
			return "", fmt.Errorf("%w: requested %q, response %q", ErrCardUpdateBizIDDrift, requestedBizID, responseBizID)
		}
	}
	if observation.positiveEvidence != "" && observation.negativeEvidence != "" {
		return "", fmt.Errorf("%w: conflicting evidence %s and %s", ErrCardUpdateUnverified, observation.positiveEvidence, observation.negativeEvidence)
	}
	if observation.positiveEvidence != "" {
		return observation.positiveEvidence, nil
	}
	if observation.negativeEvidence != "" {
		return "", fmt.Errorf("%w: %s", ErrCardUpdateNotApplied, observation.negativeEvidence)
	}
	return "", ErrCardUpdateUnverified
}

type cardUpdateObservation struct {
	bizIDs           map[string]struct{}
	positiveEvidence string
	negativeEvidence string
}

func observeCardUpdate(value any, observation *cardUpdateObservation) {
	switch typed := value.(type) {
	case map[string]any:
		observeCardUpdateMap(typed, observation)
	case []any:
		for _, child := range typed {
			observeCardUpdate(child, observation)
		}
	case bool:
		if typed {
			setPositiveCardUpdateEvidence(observation, "result=true")
		} else {
			setNegativeCardUpdateEvidence(observation, "result=false")
		}
	}
}

func observeCardUpdateMap(value map[string]any, observation *cardUpdateObservation) {
	for _, key := range []string{"bizId", "bizID", "biz_id"} {
		if candidate, ok := value[key].(string); ok && strings.TrimSpace(candidate) != "" {
			observation.bizIDs[strings.TrimSpace(candidate)] = struct{}{}
		}
	}

	for _, key := range []string{"updated", "applied"} {
		if applied, ok := value[key].(bool); ok {
			if applied {
				setPositiveCardUpdateEvidence(observation, key+"=true")
			} else {
				setNegativeCardUpdateEvidence(observation, key+"=false")
			}
		}
	}
	for _, key := range []string{"affectedCount", "updatedCount", "modifiedCount"} {
		if count, ok := cardUpdateCount(value[key]); ok {
			if count > 0 {
				setPositiveCardUpdateEvidence(observation, fmt.Sprintf("%s=%d", key, count))
			} else if count == 0 {
				setNegativeCardUpdateEvidence(observation, fmt.Sprintf("%s=%d", key, count))
			}
		}
	}
	errorCode, hasErrorCode := value["errorCode"]
	errorCodeEmpty := hasErrorCode && cardUpdateErrorCodeEmpty(errorCode)
	if hasErrorCode && !errorCodeEmpty {
		setNegativeCardUpdateEvidence(observation, "errorCode=non-empty")
	}
	if success, ok := value["success"].(bool); ok {
		if success {
			// Record success=true only when the same response envelope explicitly
			// includes its business-error field. A non-empty code is already
			// negative evidence above, so the two signals reject the conflict.
			if hasErrorCode {
				setPositiveCardUpdateEvidence(observation, "success=true")
			}
		} else {
			setNegativeCardUpdateEvidence(observation, "success=false")
		}
	}

	// Only documented response envelopes are traversed. This prevents an
	// unrelated extension field containing "updated":true from proving the
	// business write.
	for _, key := range []string{"result", "data", "response", "card"} {
		if child, exists := value[key]; exists {
			observeCardUpdate(child, observation)
		}
	}
}

func cardUpdateErrorCodeEmpty(value any) bool {
	if value == nil {
		return true
	}
	code, ok := value.(string)
	return ok && strings.TrimSpace(code) == ""
}

func setPositiveCardUpdateEvidence(observation *cardUpdateObservation, evidence string) {
	if observation.positiveEvidence == "" {
		observation.positiveEvidence = evidence
	}
}

func setNegativeCardUpdateEvidence(observation *cardUpdateObservation, evidence string) {
	if observation.negativeEvidence == "" {
		observation.negativeEvidence = evidence
	}
}

func cardUpdateCount(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float32:
		return int64(typed), float32(int64(typed)) == typed
	case float64:
		return int64(typed), float64(int64(typed)) == typed
	case json.Number:
		count, err := typed.Int64()
		return count, err == nil
	default:
		return 0, false
	}
}
