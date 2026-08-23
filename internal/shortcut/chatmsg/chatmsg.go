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

// Package chatmsg holds the shared, read-only projection helpers for DingTalk
// message-list responses (list_individual_chat_message,
// list_conversation_message_v2, search_at_me_message, search_messages_by_keyword,
// list_topic_replies, …). Several shortcuts reshape those raw responses into a
// clean speaker/text/time list; centralising the fiddly bits here keeps them
// consistent and fixed in one place:
//
//   - Sender: the display name lives under the bare "sender" key, forwarded
//     entries carry the literal string "null", and some responses nest the
//     speaker in a {name:…} object — all handled here.
//   - Text: out-of-office auto-replies / cards arrive as raw rich-content JSON,
//     and card/robot messages arrive as undecryptable ciphertext; CleanText
//     renders the former to readable text and marks the latter, WITHOUT ever
//     rewriting ordinary text that merely contains a JSON fragment.
//   - Forwarded: a forwarded chat record ("聊天记录") hides its real per-message
//     bodies in forwardMessages while the top-level content is a lossy summary.
package chatmsg

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MessageListContractVersion identifies the additive, compatibility-preserving
// public envelope shared by message list/search/mget/thread projections.
const MessageListContractVersion = "im.message-list.v1"

// MessageResultContract is the reviewed additive contract shared by message
// list, search, mget, @me and thread projections. Keep this descriptor small:
// Runtime owns the values, while Skill references and policy checks consume a
// copy of these field names so prose cannot silently invent another result
// shape.
type MessageResultContract struct {
	Version        string
	MessageFields  []string
	EnvelopeFields []string
}

var messageResultContractV1 = MessageResultContract{
	Version: MessageListContractVersion,
	MessageFields: []string{
		"messageId",
		"conversationId",
		"threadId",
		"sender",
		"senderId",
		"senderType",
		"messageType",
		"text",
		"createTime",
		"updateTime",
		"reactions",
		"quotedMessage",
		"forwarded",
		"resourceRefs",
	},
	EnvelopeFields: []string{
		"contractVersion",
		"messages",
		"count",
		"resolvedFilters",
		"queryRange",
		"pagesFetched",
		"paginationKnown",
		"complete",
		"hasMore",
		"nextPage",
		"stopReason",
		"truncated",
		"truncatedByPageLimit",
		"truncatedByResultLimit",
		"failedCount",
		"failures",
		"partial",
		"scope",
		"resourceDownloads",
	},
}

// CurrentMessageResultContract returns defensive copies so callers cannot
// mutate the process-wide reviewed descriptor.
func CurrentMessageResultContract() MessageResultContract {
	contract := messageResultContractV1
	contract.MessageFields = append([]string(nil), contract.MessageFields...)
	contract.EnvelopeFields = append([]string(nil), contract.EnvelopeFields...)
	return contract
}

// NewMessageListPayload initializes the common result ledger before a caller
// adds pagination or resource-download facts.
func NewMessageListPayload(messages []map[string]any) map[string]any {
	if messages == nil {
		messages = []map[string]any{}
	}
	return map[string]any{
		"contractVersion": MessageListContractVersion,
		"messages":        messages,
		"count":           len(messages),
		"pagesFetched":    0,
		"paginationKnown": false,
		"complete":        false,
		"hasMore":         false,
		"failedCount":     0,
		"failures":        []map[string]any{},
		"partial":         false,
		"truncated":       false,
	}
}

// ApplyTruncation publishes the stable aggregate bit while preserving the
// established reason-specific fields for compatibility and diagnosis.
func ApplyTruncation(payload map[string]any) {
	if payload == nil {
		return
	}
	byPage, _ := payload["truncatedByPageLimit"].(bool)
	byItems, _ := payload["truncatedByResultLimit"].(bool)
	payload["truncated"] = byPage || byItems
}

// ListMessageItems returns message rows from the common list response envelopes.
func ListMessageItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	scopes := []map[string]any{data}
	for _, wrapper := range []string{"result", "data"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			scopes = append(scopes, inner)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"messages", "list", "items", "records", "data", "result"} {
			if rows, ok := scope[key].([]any); ok {
				items := messageMaps(rows)
				if len(items) > 0 {
					return items
				}
			}
		}
	}
	return nil
}

// SearchMessageItems flattens grouped search results into stable message rows.
func SearchMessageItems(data map[string]any) []map[string]any {
	return SearchItems(data)
}

func messageMaps(rows []any) []map[string]any {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if item, ok := row.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items
}

// StableMessageID returns the normalized message identity used for
// cross-page deduplication. An empty value means the lower response did not
// publish a stable identity; callers must keep that row rather than guessing.
func StableMessageID(message map[string]any) string {
	value := MessageID(message)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// Sender reads a message's speaker display name, tolerating common sender-name
// keys. The message-list responses carry the display name under the bare
// "sender" key (verified live), so it is probed first; the remaining aliases and
// the *Id fallbacks keep the projection resilient to other shapes. The literal
// string "null" (forwarded entries) and the empty string are treated as absent,
// and a nested {name:…} sender object yields its display name rather than the
// raw object.
func Sender(m map[string]any) any {
	for _, key := range []string{"sender", "senderName", "senderNick", "nick", "senderStaffName", "userName", "name", "senderId", "senderStaffId", "senderOpenDingTalkId"} {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t == "" || t == "null" {
				continue
			}
			return t
		case map[string]any:
			// Nested sender object: extract a display-name field; never return
			// the raw map (it would surface a JSON object and block fallbacks).
			if name := senderDisplayName(t); name != "" {
				return name
			}
			continue
		default:
			// Scalar id (e.g. numeric) — usable as-is.
			return v
		}
	}
	return nil
}

// senderDisplayName extracts a human name from a nested sender object.
func senderDisplayName(m map[string]any) string {
	for _, k := range []string{"name", "nick", "userName", "staffName", "displayName", "senderName"} {
		if s, ok := m[k].(string); ok {
			if s = strings.TrimSpace(s); s != "" && s != "null" {
				return s
			}
		}
	}
	return ""
}

// Text reads a message's textual content (tolerating common text keys and one
// level of nesting) and runs it through CleanText.
func Text(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "msg", "body", "plainText"} {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return CleanText(t)
			}
		case map[string]any:
			for _, inner := range []string{"text", "content", "richText", "title", "value"} {
				if s, ok := t[inner].(string); ok && s != "" {
					return CleanText(s)
				}
			}
		}
	}
	return nil
}

// CreateTime reads a message's create/send time under whichever candidate key is
// present, returning the raw value.
func CreateTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "createAt", "timestamp", "time"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// MessageID preserves the stable message identity needed by follow-up reply,
// reaction, resource and deduplication operations.
func MessageID(m map[string]any) any {
	return firstMessageValue(m, "openMessageId", "openMsgId", "messageId", "message_id", "msgId", "msg_id", "id")
}

// ConversationID preserves the stable conversation identity carried by list
// and search responses.
func ConversationID(m map[string]any) any {
	return firstMessageValue(m, "openConversationId", "openconversation_id", "conversationId", "conversation_id", "chatId", "chat_id")
}

// ThreadID preserves the stable topic/thread identity needed to continue from
// a message-list result into the thread-replies command.
func ThreadID(m map[string]any) any {
	return firstMessageValue(
		m,
		"openConvThreadId",
		"openConversationThreadId",
		"threadId",
		"thread_id",
		"topicId",
		"topic_id",
	)
}

// MessageType preserves the lower message type when present.
func MessageType(m map[string]any) any {
	return firstMessageValue(m, "msgType", "messageType", "message_type", "type")
}

// SenderID preserves the stable sender identity without replacing the legacy
// scalar sender display field. Nested sender records and both userId families
// are accepted because list/search/mget currently expose different shapes.
func SenderID(m map[string]any) any {
	for _, key := range []string{"sender", "from", "senderUser"} {
		if nested, ok := m[key].(map[string]any); ok {
			if value := firstMessageValue(nested,
				"openDingTalkId", "openDingtalkId", "userId", "senderId", "id"); value != nil {
				return value
			}
		}
	}
	return firstMessageValue(m,
		"senderOpenDingTalkId", "senderOpenDingtalkId", "senderUserId",
		"senderId", "sender_id", "senderStaffId", "openDingTalkId", "userId")
}

// SenderType returns only an explicitly published lower sender type. It does
// not guess that every sender identity is a user because bot/system messages
// can share the same generic senderId key.
func SenderType(m map[string]any) any {
	for _, key := range []string{"sender", "from", "senderUser"} {
		if nested, ok := m[key].(map[string]any); ok {
			if value := firstMessageValue(nested, "senderType", "type", "entityType"); value != nil {
				return value
			}
		}
	}
	return firstMessageValue(m, "senderType", "sender_type", "fromType", "from_type")
}

// ProjectMessageV1 is the single compatibility-preserving core projection for
// list, search, mget, @me, and thread readers. Public wrappers may retain
// legacy aliases such as time or msgType, but the underlying identity,
// context, reaction, quote, forward, and resource semantics come from here.
func ProjectMessageV1(m map[string]any, includeReactions bool) map[string]any {
	ownedResources := Resources(m)
	row := map[string]any{
		"sender":     Sender(m),
		"text":       projectedResourceText(m, ownedResources),
		"createTime": CreateTime(m),
	}
	if value := MessageID(m); value != nil {
		row["messageId"] = value
	}
	if value := ConversationID(m); value != nil {
		row["conversationId"] = value
	}
	if value := ThreadID(m); value != nil {
		row["threadId"] = value
	}
	if value := SenderID(m); value != nil {
		row["senderId"] = value
	}
	if value := SenderType(m); value != nil {
		row["senderType"] = value
	}
	if value := MessageType(m); value != nil {
		row["messageType"] = value
	}
	if value := UpdateTime(m); value != nil {
		row["updateTime"] = value
	}
	if includeReactions {
		if reactions := Reactions(m); len(reactions) > 0 {
			row["reactions"] = reactions
		}
	}
	if quoted := QuotedMessage(m); len(quoted) > 0 {
		row["quotedMessage"] = quoted
	}
	if resources := ResourcesDeep(m); len(resources) > 0 {
		row["resourceRefs"] = resources
	}
	projectForwarded := func(item map[string]any) map[string]any {
		return ProjectMessageV1(item, includeReactions)
	}
	if forwarded := Forwarded(m, projectForwarded); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
}

// QuotedMessage projects one level of quoted/replied-to context. It is
// deliberately non-recursive: a reply chain may be arbitrarily deep or even
// cyclic after gateway reshaping, while an Agent primarily needs the quoted
// message's stable identity, speaker, readable body and time.
func QuotedMessage(m map[string]any) map[string]any {
	var quoted map[string]any
	for _, key := range []string{"quotedMessage", "replyMessage", "quoted", "replyToMessage"} {
		if value, ok := m[key].(map[string]any); ok {
			quoted = value
			break
		}
	}
	if quoted == nil {
		return nil
	}
	out := map[string]any{}
	if value := MessageID(quoted); value != nil {
		out["messageId"] = value
	}
	if value := ConversationID(quoted); value != nil {
		out["conversationId"] = value
	}
	if value := ThreadID(quoted); value != nil {
		out["threadId"] = value
	}
	if value := Sender(quoted); value != nil {
		out["sender"] = value
	}
	resources := Resources(quoted)
	if value := projectedResourceText(quoted, resources); value != nil {
		out["text"] = value
	}
	if value := CreateTime(quoted); value != nil {
		out["createTime"] = value
	}
	if value := MessageType(quoted); value != nil {
		out["messageType"] = value
	}
	if len(resources) > 0 {
		out["resourceRefs"] = resources
	}
	return out
}

func firstMessageValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		value, ok := m[key]
		if !ok || value == nil {
			continue
		}
		if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

// Resources extracts actionable media and drive-file references from both
// structured message fields and the textual mediaId/fileId notation returned
// by older DingTalk message APIs. Every reference publishes the exact Shortcut
// arguments already known from the message, plus ready=false and missing fields
// when a follow-up lookup is still required. This shared shape is used by list,
// search, mget, quoted messages and thread replies.
func Resources(m map[string]any) []map[string]any {
	if m == nil {
		return nil
	}
	mediaIDs := make([]string, 0)
	collectResourceIDs(m, "mediaid", mediaIDTextRE, &mediaIDs)
	mediaIDs = uniqueResourceIDs(mediaIDs)
	sort.Strings(mediaIDs)
	mediaNames := make(map[string]resourceNameCandidate)
	collectResourceNames(m, "mediaid", mediaNames)
	fileIDs := make([]string, 0)
	collectResourceIDs(m, "fileid", fileIDTextRE, &fileIDs)
	fileIDs = uniqueResourceIDs(fileIDs)
	sort.Strings(fileIDs)
	fileNames := make(map[string]resourceNameCandidate)
	collectResourceNames(m, "fileid", fileNames)
	if len(mediaIDs) == 0 && len(fileIDs) == 0 {
		return nil
	}

	messageID := strings.TrimSpace(fmt.Sprint(MessageID(m)))
	conversationID := strings.TrimSpace(fmt.Sprint(ConversationID(m)))
	if messageID == "<nil>" {
		messageID = ""
	}
	if conversationID == "<nil>" {
		conversationID = ""
	}

	out := make([]map[string]any, 0, len(mediaIDs)+len(fileIDs))
	for _, id := range mediaIDs {
		arguments := map[string]any{
			"type":        "mediaId",
			"resource-id": id,
		}
		missing := make([]string, 0, 2)
		if messageID != "" {
			arguments["message-id"] = messageID
		} else {
			missing = append(missing, "message-id")
		}
		if conversationID != "" {
			arguments["open-conversation-id"] = conversationID
		} else {
			missing = append(missing, "open-conversation-id")
		}
		resource := map[string]any{
			"type":       "mediaId",
			"resourceId": id,
			"download": map[string]any{
				"shortcut":  "+messages-resource-download",
				"arguments": arguments,
				"ready":     len(missing) == 0,
				"missing":   missing,
			},
		}
		if candidate, ok := mediaNames[id]; ok {
			resource["name"] = candidate.name
		}
		out = append(out, resource)
	}
	for _, id := range fileIDs {
		resource := map[string]any{
			"type":       "fileId",
			"resourceId": id,
			"download": map[string]any{
				"shortcut": "+messages-resource-download",
				"arguments": map[string]any{
					"type":        "fileId",
					"resource-id": id,
				},
				"ready":   true,
				"missing": []string{},
			},
		}
		if candidate, ok := fileNames[id]; ok {
			resource["name"] = candidate.name
		}
		out = append(out, resource)
	}
	return out
}

// ResourcesDeep returns resources from a message and each nested quoted,
// replied-to or forwarded message. Every nested resource is projected from the
// child message that owns it, so its download arguments never reuse the parent
// message ID. A missing child conversation ID inherits the enclosing
// conversation because quoted and forwarded records often omit that duplicate
// field.
func ResourcesDeep(m map[string]any) []map[string]any {
	return resourcesDeep(m, "", 0)
}

const maxResourceMessageDepth = 32

func resourcesDeep(m map[string]any, inheritedConversationID string, depth int) []map[string]any {
	if m == nil || depth > maxResourceMessageDepth {
		return nil
	}
	conversationID := strings.TrimSpace(fmt.Sprint(ConversationID(m)))
	if conversationID == "" || conversationID == "<nil>" {
		conversationID = inheritedConversationID
	}
	owned := m
	if ConversationID(m) == nil && conversationID != "" {
		owned = make(map[string]any, len(m)+1)
		for key, value := range m {
			owned[key] = value
		}
		owned["openConversationId"] = conversationID
	}
	out := append([]map[string]any(nil), Resources(owned)...)
	if depth == maxResourceMessageDepth {
		return out
	}
	for _, child := range nestedMessageChildren(m) {
		out = append(out, resourcesDeep(child, conversationID, depth+1)...)
	}
	return out
}

var mediaIDTextRE = regexp.MustCompile(`(?i)\bmedia[_-]?id\s*[:=]\s*["']?([^"'\s)\]}>,]+)`)
var fileIDTextRE = regexp.MustCompile(`(?i)\bfile[_-]?id\s*[:=]\s*["']?([^"'\s)\]}>,]+)`)
var fileNameAndIDTextRE = regexp.MustCompile(`(?i)\[文件\]\s*([^\r\n]*?)\s+file[_-]?id\s*[:=]\s*["']?([^"'\s)\]}>,]+)`)
var legacyResourceDownloadHintRE = regexp.MustCompile(`\s*注意：如需下载使用dws\s+(?:chat message download-media|drive download)命令下载\s*`)

// projectedResourceText removes only the exact, machine-generated download
// hint emitted by older IM APIs. The readable resource marker and ID remain in
// text, while resourceRefs publishes the current executable download command.
// Text without an owned mediaId/fileId is left byte-for-byte unchanged so an
// ordinary user sentence mentioning a command can never be rewritten.
func projectedResourceText(m map[string]any, resources []map[string]any) any {
	value := Text(m)
	text, ok := value.(string)
	if !ok || len(resources) == 0 ||
		(!mediaIDTextRE.MatchString(text) && !fileIDTextRE.MatchString(text)) {
		return value
	}
	return strings.TrimSpace(legacyResourceDownloadHintRE.ReplaceAllString(text, ""))
}

type resourceNameCandidate struct {
	name     string
	priority int
}

const (
	resourceNamePriorityText       = 1
	resourceNamePriorityStructured = 2
)

// collectResourceNames keeps a resource ID paired with a name only when both
// are present in the same structured object or in the legacy, machine-shaped
// "[文件] name fileId: id" text. This deliberately does not borrow a generic
// message title or sender name: an unknown resource name is safer than a
// plausible but incorrect one.
func collectResourceNames(value any, targetKey string, out map[string]resourceNameCandidate) {
	switch typed := value.(type) {
	case map[string]any:
		directIDs := directResourceIDs(typed, targetKey)
		if len(directIDs) == 1 {
			if name := directResourceName(typed, targetKey); name != "" {
				recordResourceName(out, directIDs[0], name, resourceNamePriorityStructured)
			}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if isNestedMessageBoundaryKey(normalizeMessageKey(key)) {
				continue
			}
			collectResourceNames(typed[key], targetKey, out)
		}
	case []any:
		for _, child := range typed {
			collectResourceNames(child, targetKey, out)
		}
	case []map[string]any:
		for _, child := range typed {
			collectResourceNames(child, targetKey, out)
		}
	case string:
		if targetKey == "fileid" {
			for _, match := range fileNameAndIDTextRE.FindAllStringSubmatch(typed, -1) {
				name := strings.TrimSpace(match[1])
				id := resourceIDScalar(match[2])
				if name != "" && id != "" {
					recordResourceName(out, id, name, resourceNamePriorityText)
				}
			}
		}
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			if json.Unmarshal([]byte(trimmed), &decoded) == nil {
				collectResourceNames(decoded, targetKey, out)
			}
		}
	}
}

func directResourceIDs(value map[string]any, targetKey string) []string {
	resourceType := normalizeMessageKey(strings.TrimSpace(fmt.Sprint(
		firstMessageValue(value, "resourceType", "resource_type"))))
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ids := make([]string, 0, 1)
	for _, key := range keys {
		normalizedKey := normalizeMessageKey(key)
		if normalizedKey != targetKey &&
			!(normalizedKey == "resourceid" && resourceType == targetKey) {
			continue
		}
		if id := resourceIDScalar(value[key]); id != "" {
			ids = append(ids, id)
		}
	}
	return uniqueResourceIDs(ids)
}

func directResourceName(value map[string]any, targetKey string) string {
	for _, wanted := range []string{"filename", "resourcename", "originalfilename"} {
		if name := directResourceString(value, wanted); name != "" {
			return name
		}
	}
	// A bare "name" is accepted only inside an explicit resource envelope.
	// Message rows also commonly contain a sender/group name, which must never
	// become the attachment filename merely because the row has a resource ID.
	resourceType := normalizeMessageKey(strings.TrimSpace(fmt.Sprint(
		firstMessageValue(value, "resourceType", "resource_type"))))
	if resourceType == targetKey && directResourceString(value, "resourceid") != "" {
		return directResourceString(value, "name")
	}
	return ""
}

func directResourceString(value map[string]any, wanted string) string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if normalizeMessageKey(key) != wanted {
			continue
		}
		if text, ok := value[key].(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func recordResourceName(
	out map[string]resourceNameCandidate,
	id, name string,
	priority int,
) {
	id = resourceIDScalar(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return
	}
	if current, ok := out[id]; ok && current.priority >= priority {
		return
	}
	out[id] = resourceNameCandidate{name: name, priority: priority}
}

func collectResourceIDs(value any, targetKey string, textPattern *regexp.Regexp, out *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		resourceType := strings.TrimSpace(fmt.Sprint(firstMessageValue(typed, "resourceType", "resource_type")))
		for key, child := range typed {
			normalizedKey := normalizeMessageKey(key)
			if normalizedKey == targetKey ||
				(normalizedKey == "resourceid" && strings.EqualFold(resourceType, targetKey)) {
				if id := resourceIDScalar(child); id != "" {
					*out = append(*out, id)
				}
			}
			if isNestedMessageBoundaryKey(normalizedKey) {
				continue
			}
			collectResourceIDs(child, targetKey, textPattern, out)
		}
	case []any:
		for _, child := range typed {
			collectResourceIDs(child, targetKey, textPattern, out)
		}
	case []map[string]any:
		for _, child := range typed {
			collectResourceIDs(child, targetKey, textPattern, out)
		}
	case string:
		for _, match := range textPattern.FindAllStringSubmatch(typed, -1) {
			if len(match) > 1 {
				if id := resourceIDScalar(match[1]); id != "" {
					*out = append(*out, id)
				}
			}
		}
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			if json.Unmarshal([]byte(trimmed), &decoded) == nil {
				collectResourceIDs(decoded, targetKey, textPattern, out)
			}
		}
	}
}

func normalizeMessageKey(key string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
}

func isNestedMessageBoundaryKey(key string) bool {
	switch key {
	case "quotedmessage", "replymessage", "quoted", "replytomessage",
		"forwardmessages", "forwardedmessages", "forwarded":
		return true
	default:
		return false
	}
}

func nestedMessageMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return []map[string]any{typed}
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if child, ok := item.(map[string]any); ok {
				out = append(out, child)
			}
		}
		return out
	case string:
		var decoded any
		if json.Unmarshal([]byte(strings.TrimSpace(typed)), &decoded) == nil {
			return nestedMessageMaps(decoded)
		}
	}
	return nil
}

func nestedMessageChildren(value any) []map[string]any {
	out := make([]map[string]any, 0)
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isNestedMessageBoundaryKey(normalizeMessageKey(key)) {
				out = append(out, nestedMessageMaps(child)...)
				continue
			}
			out = append(out, nestedMessageChildren(child)...)
		}
	case []any:
		for _, child := range typed {
			out = append(out, nestedMessageChildren(child)...)
		}
	case []map[string]any:
		for _, child := range typed {
			out = append(out, nestedMessageChildren(child)...)
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			if json.Unmarshal([]byte(trimmed), &decoded) == nil {
				out = append(out, nestedMessageChildren(decoded)...)
			}
		}
	}
	return out
}

func resourceIDScalar(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(text), `"'`)
}

func uniqueResourceIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// UpdateTime reads an edited message's update time. Gateways sometimes echo
// createTime as updateTime even when the message was never edited; omit that
// duplicate so Agents do not infer a nonexistent edit.
func UpdateTime(m map[string]any) any {
	createTime := CreateTime(m)
	for _, key := range []string{"updateTime", "modifiedTime", "gmtModified", "editTime"} {
		if v, ok := m[key]; ok && v != nil {
			if createTime != nil && reflect.DeepEqual(v, createTime) {
				return nil
			}
			return v
		}
	}
	return nil
}

// Reactions normalises DingTalk's inline emotionReplyList into one compact,
// Agent-friendly block. Unlike Lark, DingTalk already returns these reactions
// with message-list responses, so this projection performs no extra network
// request.
//
// Output shape:
//
//	"reactions": {
//	  "counts":  [{"emoji": "赞", "count": 3}],
//	  "details": [{"emoji": "赞", "replyUsers": ["..."]}]
//	}
func Reactions(m map[string]any) map[string]any {
	var raw []any
	for _, key := range []string{"emotionReplyList", "reactionList", "reactions"} {
		switch value := m[key].(type) {
		case []any:
			raw = value
		case []map[string]any:
			raw = make([]any, 0, len(value))
			for _, item := range value {
				raw = append(raw, item)
			}
		}
		if len(raw) > 0 {
			break
		}
	}
	if len(raw) == 0 {
		return nil
	}

	counts := make([]map[string]any, 0, len(raw))
	details := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		emoji := firstReactionValue(entry, "emoji", "emojiName", "reactionType", "emotionName", "text")
		users := reactionUsers(entry)
		count := reactionCount(entry, len(users))
		if emoji == nil && count == 0 && len(users) == 0 {
			continue
		}

		countRow := map[string]any{"count": count}
		detailRow := map[string]any{}
		if emoji != nil {
			countRow["emoji"] = emoji
			detailRow["emoji"] = emoji
		}
		if len(users) > 0 {
			detailRow["replyUsers"] = users
		}
		counts = append(counts, countRow)
		if len(detailRow) > 0 {
			details = append(details, detailRow)
		}
	}
	if len(counts) == 0 && len(details) == 0 {
		return nil
	}
	out := map[string]any{}
	if len(counts) > 0 {
		out["counts"] = counts
	}
	if len(details) > 0 {
		out["details"] = details
	}
	return out
}

func firstReactionValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		value, ok := m[key]
		if !ok || value == nil {
			continue
		}
		if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

func reactionUsers(m map[string]any) []any {
	for _, key := range []string{"replyUsers", "users", "operators"} {
		switch value := m[key].(type) {
		case []any:
			return value
		case []string:
			out := make([]any, 0, len(value))
			for _, item := range value {
				out = append(out, item)
			}
			return out
		}
	}
	return nil
}

func reactionCount(m map[string]any, fallback int) any {
	for _, key := range []string{"count", "replyCount", "reactionCount"} {
		switch value := m[key].(type) {
		case int, int32, int64, float32, float64, json.Number:
			return value
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return fallback
}

// ApplyPagination carries lower-layer completeness facts into a projected
// Shortcut payload. It intentionally preserves cursor values only in the
// command output (where callers need them to continue); audit reports redact
// those values and retain only their presence.
func ApplyPagination(payload, data map[string]any) {
	for key, value := range Pagination(data) {
		payload[key] = value
	}
}

// ApplyMessagePagination publishes message-list completeness and converts the
// authoritative millisecond nextCursor into the RFC3339Nano time boundary
// accepted by the executable message-list command. Projected createTime is
// deliberately not used because it is only second precision.
func ApplyMessagePagination(payload, data map[string]any, messages []map[string]any, direction string) {
	payload["contractVersion"] = MessageListContractVersion
	payload["pagesFetched"] = 1
	payload["enrichedCount"] = 0
	payload["failedCount"] = 0
	payload["failures"] = []map[string]any{}
	payload["hasMore"] = false
	payload["complete"] = false
	page := Pagination(data)
	if len(page) == 0 {
		payload["paginationKnown"] = false
		payload["failedCount"] = 1
		payload["failures"] = []map[string]any{{
			"stage": "pagination",
			"error": "下层未返回可靠的 hasMore/nextCursor，无法证明结果完整",
		}}
		return
	}
	value, hasMoreKnown := page["hasMore"]
	if !hasMoreKnown {
		payload["paginationKnown"] = false
		payload["failedCount"] = 1
		payload["failures"] = []map[string]any{{
			"stage": "pagination",
			"error": "下层仅返回 cursor、未返回 hasMore，无法证明结果完整",
		}}
		return
	}
	payload["paginationKnown"] = true
	payload["hasMore"] = value
	if value, ok := page["complete"]; ok {
		payload["complete"] = value
	}
	hasMore, _ := page["hasMore"].(bool)
	if !hasMore {
		return
	}
	if len(messages) == 0 {
		payload["failedCount"] = 1
		payload["failures"] = []map[string]any{{
			"stage": "pagination",
			"error": "下层返回 hasMore=true 但当前页没有消息",
		}}
		return
	}
	_, boundary, err := messagePaginationCursorBoundary(page["nextCursor"])
	if err != nil {
		payload["failedCount"] = 1
		payload["failures"] = []map[string]any{{
			"stage": "pagination",
			"error": "下层返回 hasMore=true，但 nextCursor 无效: " + err.Error(),
		}}
		return
	}
	next := map[string]any{"time": boundary, "nextCursor": page["nextCursor"]}
	if strings.TrimSpace(direction) != "" {
		next["direction"] = direction
	}
	payload["nextPage"] = next
}

func messagePaginationCursorBoundary(value any) (string, string, error) {
	var millis int64
	switch typed := value.(type) {
	case int:
		millis = int64(typed)
	case int32:
		millis = int64(typed)
	case int64:
		millis = typed
	case float32:
		asFloat := float64(typed)
		if math.IsNaN(asFloat) || math.IsInf(asFloat, 0) || asFloat <= 0 || math.Trunc(asFloat) != asFloat || asFloat > math.MaxInt64 {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = int64(asFloat)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed <= 0 || math.Trunc(typed) != typed || typed > math.MaxInt64 {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = parsed
	default:
		return "", "", fmt.Errorf("缺少毫秒级分页游标")
	}
	if millis <= 0 {
		return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
	}
	key := strconv.FormatInt(millis, 10)
	boundary := time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
	return key, boundary, nil
}

// Pagination extracts hasMore/nextCursor from the response root or a common
// result/data envelope. When hasMore is present it also emits the explicit
// inverse "complete", making truncation hard for an Agent to overlook.
func Pagination(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	scopes := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if inner, ok := data[key].(map[string]any); ok {
			scopes = append(scopes, inner)
		}
	}
	for _, scope := range scopes {
		out := map[string]any{}
		for _, key := range []string{"hasMore", "has_more"} {
			if value, ok := scope[key].(bool); ok {
				out["hasMore"] = value
				out["complete"] = !value
				break
			}
		}
		for _, key := range []string{"nextCursor", "next_cursor", "nextToken", "next_token", "pageToken", "page_token"} {
			if value, ok := scope[key]; ok && paginationValuePresent(value) {
				out["nextCursor"] = value
				break
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func paginationValuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case json.Number:
		number, err := typed.Float64()
		return err != nil || number != 0
	case string:
		return strings.TrimSpace(typed) != "" && strings.TrimSpace(typed) != "0"
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return true
	}
}

// Forwarded projects the nested messages of a forwarded chat record. The caller
// supplies its own per-message projection so each command keeps its own row
// shape; project is applied recursively, so multi-level forwards expand too.
func Forwarded(m map[string]any, project func(map[string]any) map[string]any) []map[string]any {
	raw, ok := m["forwardMessages"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if sub, ok := e.(map[string]any); ok {
			out = append(out, project(sub))
		}
	}
	return out
}

// CleanText makes a message body human-readable WITHOUT ever rewriting ordinary
// text. It only transforms a body that is a genuine DingTalk structured message:
//
//   - Encrypted card/robot ciphertext (base64 + "||v||t||len" trailer) → a clear
//     "[加密消息]" marker instead of the raw base64.
//   - A rich-content card (out-of-office auto-reply, link/preview card, …) whose
//     lines include at least one recognised rich-content block → the readable
//     text extracted from those blocks, with the card's decorative JSON lines and
//     "empty" placeholders dropped.
//
// Crucially, if NO line is a recognised rich-content block (e.g. ordinary text
// that merely embeds a `{"approved":false}` fragment), the original string is
// returned verbatim — a JSON line is never silently dropped.
func CleanText(s string) string {
	if IsEncrypted(s) {
		return "[加密消息，无法解码]"
	}

	// Fast path: no JSON delimiters at all — the overwhelming common case.
	if !strings.ContainsAny(s, "{[") {
		return s
	}

	lines := strings.Split(s, "\n")
	isJSON := make([]bool, len(lines))
	isDecoration := make([]bool, len(lines))
	extracted := make([][]string, len(lines))
	anyExtracted := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
			continue
		}
		var v any
		if json.Unmarshal([]byte(t), &v) != nil {
			continue
		}
		isJSON[i] = true
		isDecoration[i] = isKnownRichDecoration(v)
		if texts := richItemTexts(v); len(texts) > 0 {
			extracted[i] = texts
			anyExtracted = true
		}
	}

	// No recognised rich-content block anywhere → treat the whole body as plain
	// text (which may merely contain a JSON fragment) and return it untouched.
	if !anyExtracted {
		return s
	}

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if len(extracted[i]) > 0 {
			out = append(out, extracted[i]...)
			continue
		}
		// In card mode, drop only JSON shapes known to be card decoration.
		// Unrecognised JSON may be user-authored message content and must remain
		// verbatim even when another line contains a rich-content block.
		if isJSON[i] && isDecoration[i] {
			continue
		}
		if t := strings.TrimSpace(line); t == "" || t == "empty" {
			continue
		}
		out = append(out, line)
	}
	// anyExtracted is true here, so out always holds at least one non-empty
	// extracted text — the joined result is never empty.
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// isKnownRichDecoration recognises the two decoration records emitted alongside
// DingTalk rich-content bodies. Keep this deliberately narrow: an arbitrary JSON
// object in the same message is user content unless its shape is known here.
func isKnownRichDecoration(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}
	_, hasPreviewURL := m["previewUrl"]
	_, hasTitle := m["title"]
	_, hasAutoLayout := m["autoLayout"]
	_, hasEnableForward := m["enableForward"]
	return (hasPreviewURL && hasTitle) || (hasAutoLayout && hasEnableForward)
}

// richItemTexts walks a decoded DingTalk rich-content blob and returns the
// readable text carried by its rich-content items (items[].data.text). It only
// harvests item bodies, so decorative fields (card titles, preview URLs, layout
// config) contribute nothing and are dropped. An empty result means "not a
// recognised rich-content block".
func richItemTexts(node any) []string {
	var texts []string
	var walk func(n any)
	walk = func(n any) {
		switch t := n.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			if items, ok := t["items"].([]any); ok {
				for _, it := range items {
					mm, ok := it.(map[string]any)
					if !ok {
						continue
					}
					data, ok := mm["data"].(map[string]any)
					if !ok {
						continue
					}
					if s, ok := data["text"].(string); ok {
						if s = strings.TrimSpace(s); s != "" {
							texts = append(texts, s)
						}
					}
				}
			}
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(node)
	return texts
}

// encryptedTrailerRE matches DingTalk's encrypted-message trailer
// "||<version>||<type>||<len>" (e.g. "||2||1||196") anchored at the end.
var encryptedTrailerRE = regexp.MustCompile(`\|\|\d+\|\|\d+\|\|\d+\s*$`)

// IsEncrypted reports whether a message body is a raw DingTalk encrypted-message
// ciphertext: a base64 blob (DingTalk wraps it across several lines) followed by
// the "||v||t||len" trailer. It is intentionally strict — both the trailer and a
// pure-base64 body are required — so ordinary text (CJK, punctuation, …) never
// trips it.
func IsEncrypted(s string) bool {
	s = strings.TrimSpace(s)
	if !encryptedTrailerRE.MatchString(s) {
		return false
	}
	body := strings.TrimSpace(encryptedTrailerRE.ReplaceAllString(s, ""))
	if len(body) < 32 {
		return false
	}
	for _, r := range body {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '+', r == '/', r == '=', r == '\n', r == '\r', r == ' ', r == '\t':
		default:
			return false
		}
	}
	return true
}
