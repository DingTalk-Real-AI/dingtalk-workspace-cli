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
	"regexp"
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
