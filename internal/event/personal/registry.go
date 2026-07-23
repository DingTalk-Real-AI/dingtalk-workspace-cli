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
	EventMention       = "user_im_message_receive_at"
	EventSingleChat    = "user_im_message_receive_o2o"
	EventInChat        = "user_im_message_receive_group"
	EventFromUser      = "user_im_message_receive_user"
	EventReadO2O       = "user_im_message_read_o2o"
	EventReadGroup     = "user_im_message_read_group"
	EventRecallO2O     = "user_im_message_recall_o2o"
	EventRecallGroup   = "user_im_message_recall_group"
	EventReactionO2O   = "user_im_message_reaction_o2o"
	EventReactionGroup = "user_im_message_reaction_group"
)

const (
	StatusEnabled = "enabled"
	StatusPending = "pending"
)

type Definition struct {
	EventKey       string                `json:"event_key"`
	DisplayName    string                `json:"display_name"`
	Description    string                `json:"description"`
	Category       string                `json:"category"`
	RuleType       string                `json:"rule_type"`
	Status         string                `json:"status"`
	RequiredParams []string              `json:"required_params"`
	Constraints    *ParameterConstraints `json:"constraints,omitempty"`
	Auth           map[string]any        `json:"auth,omitempty"`
	Public         bool                  `json:"-"`
}

type ParameterConstraints struct {
	RequireOneOf      [][]string `json:"require_one_of,omitempty"`
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`
}

type SchemaDocument struct {
	EventKey       string                `json:"event_key"`
	DisplayName    string                `json:"display_name"`
	Description    string                `json:"description"`
	Category       string                `json:"category"`
	RuleType       string                `json:"rule_type"`
	RequiredParams []string              `json:"required_params"`
	Constraints    *ParameterConstraints `json:"constraints,omitempty"`
	JQRootPath     string                `json:"jq_root_path"`
	Schema         map[string]any        `json:"schema"`
}

type RuleOptions struct {
	RuleType       string
	UserID         string
	OpenDingTalkID string
	GroupID        string
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
		RequiredParams: nil,
		Constraints:    targetUIDConstraints(),
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
		Description:    "当前用户收到的指定用户发送的消息",
		Category:       "im",
		RuleType:       "sender",
		Status:         StatusEnabled,
		RequiredParams: nil,
		Constraints:    targetUIDConstraints(),
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventReadO2O,
		DisplayName:    "指定单聊消息已读",
		Description:    "当前用户在指定单聊中发送的消息被对方已读",
		Category:       "im",
		RuleType:       "singleChat",
		Status:         StatusEnabled,
		RequiredParams: nil,
		Constraints:    targetUIDConstraints(),
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventReadGroup,
		DisplayName:    "指定群消息已读",
		Description:    "当前用户在指定群聊中发送的消息被已读",
		Category:       "im",
		RuleType:       "group",
		Status:         StatusEnabled,
		RequiredParams: []string{"group"},
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventRecallO2O,
		DisplayName:    "指定单聊消息撤回",
		Description:    "指定单聊中的消息被撤回",
		Category:       "im",
		RuleType:       "singleChat",
		Status:         StatusEnabled,
		RequiredParams: nil,
		Constraints:    targetUIDConstraints(),
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventRecallGroup,
		DisplayName:    "指定群消息撤回",
		Description:    "指定群聊中的消息被撤回",
		Category:       "im",
		RuleType:       "group",
		Status:         StatusEnabled,
		RequiredParams: []string{"group"},
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventReactionO2O,
		DisplayName:    "指定单聊消息表情回应",
		Description:    "指定单聊中的消息收到表情回应（贴表情）",
		Category:       "im",
		RuleType:       "singleChat",
		Status:         StatusEnabled,
		RequiredParams: nil,
		Constraints:    targetUIDConstraints(),
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
	{
		EventKey:       EventReactionGroup,
		DisplayName:    "指定群消息表情回应",
		Description:    "指定群聊中的消息收到表情回应（贴表情）",
		Category:       "im",
		RuleType:       "group",
		Status:         StatusEnabled,
		RequiredParams: []string{"group"},
		Auth:           map[string]any{"identity": "user"},
		Public:         true,
	},
}

func targetUIDConstraints() *ParameterConstraints {
	return &ParameterConstraints{
		RequireOneOf:      [][]string{{"user", "open-dingtalk-id"}},
		MutuallyExclusive: [][]string{{"user", "open-dingtalk-id"}},
	}
}

func Definitions() []Definition {
	out := make([]Definition, 0, len(definitions))
	for _, def := range definitions {
		out = append(out, cloneDefinition(def))
	}
	return out
}

func Lookup(eventKey string) (Definition, bool) {
	for _, def := range definitions {
		if def.EventKey == eventKey {
			return cloneDefinition(def), true
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
		out = append(out, cloneDefinition(def))
	}
	return out
}

func BuildSchemaDocument(def Definition) SchemaDocument {
	return BuildSchemaDocumentForMode(def, false)
}

func BuildSchemaDocumentForMode(def Definition, flatten bool) SchemaDocument {
	requiredParams := make([]string, 0, len(def.RequiredParams))
	requiredParams = append(requiredParams, def.RequiredParams...)
	jqRootPath := ".data | fromjson"
	schema := transportEnvelopeSchema(def.EventKey)
	if flatten {
		jqRootPath = "."
		schema = outputSchema(def.EventKey)
	}
	return SchemaDocument{
		EventKey:       def.EventKey,
		DisplayName:    def.DisplayName,
		Description:    def.Description,
		Category:       def.Category,
		RuleType:       def.RuleType,
		RequiredParams: requiredParams,
		Constraints:    cloneParameterConstraints(def.Constraints),
		JQRootPath:     jqRootPath,
		Schema:         schema,
	}
}

func cloneDefinition(def Definition) Definition {
	def.RequiredParams = append([]string(nil), def.RequiredParams...)
	def.Constraints = cloneParameterConstraints(def.Constraints)
	if def.Auth != nil {
		def.Auth = cloneMap(def.Auth)
	}
	return def
}

func cloneParameterConstraints(in *ParameterConstraints) *ParameterConstraints {
	if in == nil {
		return nil
	}
	cloneGroups := func(groups [][]string) [][]string {
		out := make([][]string, len(groups))
		for i, group := range groups {
			out[i] = append([]string(nil), group...)
		}
		return out
	}
	return &ParameterConstraints{
		RequireOneOf:      cloneGroups(in.RequireOneOf),
		MutuallyExclusive: cloneGroups(in.MutuallyExclusive),
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
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
	userID := strings.TrimSpace(opts.UserID)
	openDingTalkID := strings.TrimSpace(opts.OpenDingTalkID)
	groupID := strings.TrimSpace(opts.GroupID)
	switch def.RuleType {
	case "at":
		if userID != "" {
			return "", nil, fmt.Errorf("--user is not supported for %s", eventKey)
		}
		if openDingTalkID != "" {
			return "", nil, fmt.Errorf("--open-dingtalk-id is not supported for %s", eventKey)
		}
		if groupID != "" {
			return "", nil, fmt.Errorf("--group is not supported for %s", eventKey)
		}
		return def.RuleType, map[string]any{}, nil
	case "singleChat":
		return buildTargetUIDRuleParam(def.RuleType, eventKey, userID, openDingTalkID, groupID)
	case "sender":
		return buildTargetUIDRuleParam(def.RuleType, eventKey, userID, openDingTalkID, groupID)
	case "group":
		if userID != "" {
			return "", nil, fmt.Errorf("--user is not supported for %s; use --group", eventKey)
		}
		if openDingTalkID != "" {
			return "", nil, fmt.Errorf("--open-dingtalk-id is not supported for %s; use --group", eventKey)
		}
		if groupID == "" {
			return "", nil, fmt.Errorf("--group is required for %s", eventKey)
		}
		return def.RuleType, map[string]any{
			"openConversationId": groupID,
		}, nil
	default:
		return "", nil, &SchemaPendingError{EventKey: eventKey}
	}
}

func buildTargetUIDRuleParam(ruleType, eventKey, userID, openDingTalkID, groupID string) (string, map[string]any, error) {
	if groupID != "" {
		return "", nil, fmt.Errorf("--group is not supported for %s; use --user or --open-dingtalk-id", eventKey)
	}
	if userID != "" && openDingTalkID != "" {
		return "", nil, fmt.Errorf("--user and --open-dingtalk-id are mutually exclusive for %s", eventKey)
	}
	if userID == "" && openDingTalkID == "" {
		return "", nil, fmt.Errorf("one of --user or --open-dingtalk-id is required for %s", eventKey)
	}
	if openDingTalkID != "" {
		return ruleType, map[string]any{
			"targetUid":     openDingTalkID,
			"targetUidType": "openDingtalkId",
		}, nil
	}
	return ruleType, map[string]any{
		"targetUid":     userID,
		"targetUidType": "staffId",
	}, nil
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

func isMessageReceiveEvent(eventKey string) bool {
	switch eventKey {
	case EventMention, EventSingleChat, EventInChat, EventFromUser:
		return true
	default:
		return false
	}
}

func IsSchemaPending(err error) bool {
	var pending *SchemaPendingError
	return errors.As(err, &pending)
}
