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

package personal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	EventMention    = "user_im_message_receive_at"
	EventSingleChat = "user_im_message_receive_o2o"
	EventInChat     = "user_im_message_receive_group"
	EventFromUser   = "user_im_message_receive_user"
)

const (
	StatusEnabled = "enabled"
	StatusPending = "pending"
)

type Definition struct {
	EventKey       string         `json:"event_key"`
	DisplayName    string         `json:"display_name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	RuleType       string         `json:"rule_type"`
	Status         string         `json:"status"`
	RequiredParams []string       `json:"required_params"`
	Auth           map[string]any `json:"auth,omitempty"`
	FilterSchema   map[string]any `json:"filter_schema,omitempty"`
	PayloadSchema  map[string]any `json:"payload_schema,omitempty"`
}

type RuleOptions struct {
	RuleType           string
	PeerUserID         string
	PeerUnionID        string
	SenderUserID       string
	SenderUnionID      string
	OpenConversationID string
}

type SchemaPendingError struct {
	EventKey string
}

func (e *SchemaPendingError) Error() string {
	return fmt.Sprintf("%s schema is pending; try user_im_message_receive_at or user_im_message_receive_o2o first", e.EventKey)
}

var definitions = []Definition{
	{
		EventKey:       EventMention,
		DisplayName:    "@我的消息",
		Description:    "当前用户被 @ 的消息",
		Category:       "im",
		RuleType:       "at",
		Status:         StatusEnabled,
		RequiredParams: nil,
		Auth:           map[string]any{"identity": "user"},
		FilterSchema:   defaultFilterSchema(),
		PayloadSchema:  imMessagePayloadSchema(),
	},
	{
		EventKey:       EventSingleChat,
		DisplayName:    "指定单聊消息",
		Description:    "当前用户与指定用户的单聊消息",
		Category:       "im",
		RuleType:       "singleChat",
		Status:         StatusEnabled,
		RequiredParams: []string{"peer-user-id or peer-union-id"},
		Auth:           map[string]any{"identity": "user"},
		FilterSchema:   defaultFilterSchema(),
		PayloadSchema:  imMessagePayloadSchema(),
	},
	{
		EventKey:       EventInChat,
		DisplayName:    "指定群消息",
		Description:    "当前用户所在指定会话的消息",
		Category:       "im",
		RuleType:       "group",
		Status:         StatusEnabled,
		RequiredParams: []string{"open-conversation-id"},
		Auth:           map[string]any{"identity": "user"},
		FilterSchema:   defaultFilterSchema(),
		PayloadSchema:  imMessagePayloadSchema(),
	},
	{
		EventKey:       EventFromUser,
		DisplayName:    "指定发送人消息",
		Description:    "当前用户收到的指定发送人消息",
		Category:       "im",
		RuleType:       "sender",
		Status:         StatusEnabled,
		RequiredParams: []string{"sender-user-id or sender-union-id"},
		Auth:           map[string]any{"identity": "user"},
		FilterSchema:   defaultFilterSchema(),
		PayloadSchema:  imMessagePayloadSchema(),
	},
}

func Definitions() []Definition {
	out := append([]Definition(nil), definitions...)
	return out
}

func Lookup(eventKey string) (Definition, bool) {
	for _, def := range definitions {
		if def.EventKey == eventKey {
			return def, true
		}
	}
	return Definition{}, false
}

func Catalog(category string, enabledOnly, includePending bool) []Definition {
	category = strings.TrimSpace(category)
	var out []Definition
	for _, def := range definitions {
		if category != "" && def.Category != category {
			continue
		}
		if enabledOnly && def.Status != StatusEnabled {
			continue
		}
		if !includePending && def.Status == StatusPending {
			continue
		}
		out = append(out, def)
	}
	return out
}

func BuildRuleParam(eventKey string, opts RuleOptions) (ruleType string, ruleParam map[string]any, err error) {
	def, ok := Lookup(eventKey)
	if !ok {
		return "", nil, fmt.Errorf("unknown personal event key %q", eventKey)
	}
	if opts.RuleType != "" && opts.RuleType != def.RuleType {
		return "", nil, fmt.Errorf("--rule %q does not match %s rule %q", opts.RuleType, eventKey, def.RuleType)
	}
	if def.Status == StatusPending {
		return "", nil, &SchemaPendingError{EventKey: eventKey}
	}
	switch def.RuleType {
	case "at":
		return def.RuleType, map[string]any{}, nil
	case "singleChat":
		targetUidType, targetUid, err := oneOfTarget("--peer-user-id", opts.PeerUserID, "--peer-union-id", opts.PeerUnionID)
		if err != nil {
			return "", nil, err
		}
		return def.RuleType, map[string]any{
			"targetUid":     targetUid,
			"targetUidType": targetUidType,
		}, nil
	case "sender":
		targetUidType, targetUid, err := oneOfTarget("--sender-user-id", opts.SenderUserID, "--sender-union-id", opts.SenderUnionID)
		if err != nil {
			return "", nil, err
		}
		return def.RuleType, map[string]any{
			"targetUid":     targetUid,
			"targetUidType": targetUidType,
		}, nil
	case "group":
		openConversationID := strings.TrimSpace(opts.OpenConversationID)
		if openConversationID == "" {
			return "", nil, fmt.Errorf("--open-conversation-id is required")
		}
		return def.RuleType, map[string]any{
			"openConversationId": openConversationID,
		}, nil
	default:
		return "", nil, &SchemaPendingError{EventKey: eventKey}
	}
}

func BuildFilter(filterJSON string, keywordCSV string) (any, string, error) {
	var parts []any
	filterJSON = strings.TrimSpace(filterJSON)
	if filterJSON != "" {
		var v any
		if err := json.Unmarshal([]byte(filterJSON), &v); err != nil {
			return nil, "", fmt.Errorf("--filter-json must be valid JSON: %w", err)
		}
		parts = append(parts, v)
	}
	keywords := splitCSV(keywordCSV)
	if len(keywords) > 0 {
		parts = append(parts, map[string]any{
			"field": "message.text",
			"op":    "contains_any",
			"value": keywords,
		})
	}
	switch len(parts) {
	case 0:
		return nil, "", nil
	case 1:
		canon, err := CanonicalJSON(parts[0])
		return parts[0], canon, err
	default:
		v := map[string]any{"and": parts}
		canon, err := CanonicalJSON(v)
		return v, canon, err
	}
}

func IdempotencyKey(identity Identity, eventKey, ruleType string, ruleParam map[string]any, filterCanonical string) string {
	ruleCanonical, _ := CanonicalJSON(ruleParam)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		identity.Key(),
		eventKey,
		ruleType,
		ruleCanonical,
		filterCanonical,
	}, "\x00")))
	return "dws-cli-" + hex.EncodeToString(sum[:8])
}

func CanonicalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func oneOfTarget(leftName, left, rightName, right string) (targetUidType, targetUid string, err error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left != "" && right != "":
		return "", "", fmt.Errorf("%s and %s are mutually exclusive", leftName, rightName)
	case left != "":
		return "staffId", left, nil
	case right != "":
		return "unionId", right, nil
	default:
		return "", "", fmt.Errorf("one of %s or %s is required", leftName, rightName)
	}
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultFilterSchema() map[string]any {
	return map[string]any{
		"fields": []string{
			"message.text",
			"chat.openConversationId",
			"sender.userId",
			"sender.unionId",
		},
		"ops": []string{"eq", "contains", "contains_any", "in", "regex"},
	}
}

func imMessagePayloadSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "object"},
			"sender":  map[string]any{"type": "object"},
			"chat":    map[string]any{"type": "object"},
		},
	}
}

func IsSchemaPending(err error) bool {
	var pending *SchemaPendingError
	return errors.As(err, &pending)
}
