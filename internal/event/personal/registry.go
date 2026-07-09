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
	Public         bool           `json:"-"`
}

type SchemaDocument struct {
	EventKey       string         `json:"event_key"`
	DisplayName    string         `json:"display_name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	RuleType       string         `json:"rule_type"`
	RequiredParams []string       `json:"required_params"`
	JQRootPath     string         `json:"jq_root_path"`
	Schema         map[string]any `json:"schema"`
}

type RuleOptions struct {
	RuleType string
	UserID   string
	GroupID  string
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
		Public:         true,
	},
	{
		EventKey:       EventSingleChat,
		DisplayName:    "指定单聊消息",
		Description:    "当前用户与指定用户的单聊消息",
		Category:       "im",
		RuleType:       "singleChat",
		Status:         StatusEnabled,
		RequiredParams: []string{"user"},
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventInChat,
		DisplayName:    "指定群消息",
		Description:    "当前用户所在指定会话的消息",
		Category:       "im",
		RuleType:       "group",
		Status:         StatusEnabled,
		RequiredParams: []string{"group"},
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventFromUser,
		DisplayName:    "指定发送人消息",
		Description:    "当前用户收到的特别关注用户的消息",
		Category:       "im",
		RuleType:       "sender",
		Status:         StatusEnabled,
		RequiredParams: []string{"user"},
		Auth:           map[string]any{"identity": "user"},
		Public:         false,
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

func IsPublic(eventKey string) bool {
	def, ok := Lookup(eventKey)
	return ok && def.Public
}

func PublicAvailabilityError(eventKey string) error {
	return fmt.Errorf("event %s is not publicly available yet", eventKey)
}

func Catalog(category string, enabledOnly, includePending bool) []Definition {
	category = strings.TrimSpace(category)
	var out []Definition
	for _, def := range definitions {
		if !def.Public {
			continue
		}
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

func BuildSchemaDocument(def Definition) SchemaDocument {
	requiredParams := make([]string, 0, len(def.RequiredParams))
	requiredParams = append(requiredParams, def.RequiredParams...)
	return SchemaDocument{
		EventKey:       def.EventKey,
		DisplayName:    def.DisplayName,
		Description:    def.Description,
		Category:       def.Category,
		RuleType:       def.RuleType,
		RequiredParams: requiredParams,
		JQRootPath:     ".data | fromjson",
		Schema:         personalMessageSchema(def.EventKey),
	}
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
	userID := strings.TrimSpace(opts.UserID)
	groupID := strings.TrimSpace(opts.GroupID)
	switch def.RuleType {
	case "at":
		if userID != "" {
			return "", nil, fmt.Errorf("--user is only supported for %s", EventSingleChat)
		}
		if groupID != "" {
			return "", nil, fmt.Errorf("--group is only supported for %s", EventInChat)
		}
		return def.RuleType, map[string]any{}, nil
	case "singleChat":
		if groupID != "" {
			return "", nil, fmt.Errorf("--group is only supported for %s", EventInChat)
		}
		if userID == "" {
			return "", nil, fmt.Errorf("--user is required")
		}
		return def.RuleType, map[string]any{
			"targetUid":     userID,
			"targetUidType": "staffId",
		}, nil
	case "sender":
		if groupID != "" {
			return "", nil, fmt.Errorf("--group is only supported for %s", EventInChat)
		}
		if userID == "" {
			return "", nil, fmt.Errorf("--user is required")
		}
		return def.RuleType, map[string]any{
			"targetUid":     userID,
			"targetUidType": "staffId",
		}, nil
	case "group":
		if userID != "" {
			return "", nil, fmt.Errorf("--user is only supported for %s", EventSingleChat)
		}
		if groupID == "" {
			return "", nil, fmt.Errorf("--group is required")
		}
		return def.RuleType, map[string]any{
			"openConversationId": groupID,
		}, nil
	default:
		return "", nil, &SchemaPendingError{EventKey: eventKey}
	}
}

func BuildFilter(filterJSON string, queryCSV string) (any, string, error) {
	var parts []any
	filterJSON = strings.TrimSpace(filterJSON)
	if filterJSON != "" {
		var v any
		if err := json.Unmarshal([]byte(filterJSON), &v); err != nil {
			return nil, "", fmt.Errorf("--filter-json must be valid JSON: %w", err)
		}
		v = normalizeFilterAliases(v)
		parts = append(parts, v)
	}
	queries := splitCSV(queryCSV)
	if len(queries) > 0 {
		parts = append(parts, map[string]any{
			"field": "payload.body.content",
			"op":    "contains_any",
			"value": queries,
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

var filterFieldAliases = map[string]string{
	"content":                 "payload.body.content",
	"conversation_id":         "payload.body.openConversationId",
	"sender":                  "payload.body.sender",
	"sender_open_dingtalk_id": "payload.body.senderOpenDingTalkId",
}

func normalizeFilterAliases(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			if k == "field" {
				if raw, ok := value.(string); ok {
					if mapped, ok := filterFieldAliases[raw]; ok {
						value = mapped
					}
				}
			} else {
				value = normalizeFilterAliases(value)
			}
			out[k] = value
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = normalizeFilterAliases(value)
		}
		return out
	default:
		return v
	}
}

func personalMessageSchema(eventKey string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"description": "事件类型，固定为当前 event_key",
				"enum":        []string{eventKey},
			},
			"event_id": map[string]any{
				"type":        "string",
				"description": "事件 ID，可用于去重",
			},
			"timestamp": map[string]any{
				"type":        "integer",
				"description": "事件发生时间戳，对应 occurredAtMs",
				"format":      "timestamp_ms",
			},
			"subscribe_id": map[string]any{
				"type":        "string",
				"description": "订阅 ID，对应 subId",
			},
			"message_id": map[string]any{
				"type":        "string",
				"description": "开放消息 ID，对应 payload.body.openMessageId",
				"format":      "open_message_id",
			},
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "会话 ID，对应 payload.body.openConversationId",
				"format":      "open_conversation_id",
			},
			"sender": map[string]any{
				"type":        "string",
				"description": "发送人展示名，对应 payload.body.sender",
			},
			"sender_open_dingtalk_id": map[string]any{
				"type":        "string",
				"description": "发送人开放 ID，对应 payload.body.senderOpenDingTalkId",
				"format":      "open_dingtalk_id",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "消息正文，对应 payload.body.content",
			},
			"create_time": map[string]any{
				"type":        "string",
				"description": "消息创建时间，对应 payload.body.createTime",
			},
			"event_time": map[string]any{
				"type":        "integer",
				"description": "消息事件时间戳，对应 payload.event_time",
				"format":      "timestamp_ms",
			},
		},
	}
}

func IsSchemaPending(err error) bool {
	var pending *SchemaPendingError
	return errors.As(err, &pending)
}
