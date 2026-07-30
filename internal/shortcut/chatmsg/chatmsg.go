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
	"reflect"
	"regexp"
	"sort"
	"strings"
)

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
	for _, key := range []string{"text", "content", "msgContent", "message", "body", "plainText"} {
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
			for _, inner := range []string{"text", "content", "value"} {
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
	if value := Text(quoted); value != nil {
		out["text"] = value
	}
	if value := CreateTime(quoted); value != nil {
		out["createTime"] = value
	}
	if value := MessageType(quoted); value != nil {
		out["messageType"] = value
	}
	if resources := Resources(quoted); len(resources) > 0 {
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
	fileIDs := make([]string, 0)
	collectResourceIDs(m, "fileid", fileIDTextRE, &fileIDs)
	fileIDs = uniqueResourceIDs(fileIDs)
	sort.Strings(fileIDs)
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
		out = append(out, map[string]any{
			"type":       "mediaId",
			"resourceId": id,
			"download": map[string]any{
				"shortcut":  "+messages-resource-download",
				"arguments": arguments,
				"ready":     len(missing) == 0,
				"missing":   missing,
			},
		})
	}
	for _, id := range fileIDs {
		out = append(out, map[string]any{
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
		})
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

// ApplyMessagePagination publishes message-list completeness without claiming
// the lower response's nextCursor is a valid CLI input. DingTalk's executable
// message-list contract paginates with the boundary message createTime, so the
// resume object uses exactly that accepted parameter.
func ApplyMessagePagination(payload, data map[string]any, messages []map[string]any, direction string) {
	page := Pagination(data)
	if len(page) == 0 {
		return
	}
	if value, ok := page["hasMore"]; ok {
		payload["hasMore"] = value
	}
	if value, ok := page["complete"]; ok {
		payload["complete"] = value
	}
	hasMore, _ := page["hasMore"].(bool)
	if !hasMore || len(messages) == 0 {
		return
	}
	boundary := CreateTime(messages[len(messages)-1])
	if boundary == nil {
		return
	}
	next := map[string]any{"time": boundary}
	if strings.TrimSpace(direction) != "" {
		next["direction"] = direction
	}
	payload["nextPage"] = next
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
