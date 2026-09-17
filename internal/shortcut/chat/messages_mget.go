// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

type mgetBatch struct {
	messages []map[string]any
	missing  []string
	failures []map[string]any
	requests int
	aborted  bool
}

func mgetRequestedIDs(rt *shortcut.RuntimeContext) []string {
	var ids []string
	for _, name := range []string{"msg-ids", "message-ids", "message-id"} {
		ids = append(ids, rt.StrSlice(name)...)
	}
	return uniqueShortcutStrings(ids)
}

func mgetID(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// Only the observed, typed openMsgId validation error permits subdivision.
// Authentication, throttling, transport and unrelated parameter errors must
// never multiply into one request per ID. A full binary split is at most 2N-1
// calls (99 for the public 50-ID limit), with no retry of an identical batch.
func mgetInvalidID(err error) bool {
	var typed *apperrors.Error
	return errors.As(err, &typed) && typed.Category == apperrors.CategoryAPI &&
		typed.ServerDiag.ServerErrorCode == "PARAM_ERROR" &&
		typed.Reason == "invalid_request" && strings.HasPrefix(typed.Message, "invalid openMsgId:")
}

func readMessagesMget(rt *shortcut.RuntimeContext, ids []string) (mgetBatch, error) {
	result := mgetBatch{messages: []map[string]any{}, missing: []string{}, failures: []map[string]any{}}
	found := map[string]map[string]any{}
	failed := map[string]string{}
	var stopped error
	var visit func([]string)
	visit = func(batch []string) {
		if stopped != nil {
			return
		}
		if err := rt.Command().Context().Err(); err != nil {
			stopped = err
			return
		}
		result.requests++
		data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": batch})
		if err != nil {
			if !mgetInvalidID(err) {
				stopped = err
				return
			}
			if len(batch) == 1 {
				failed[batch[0]] = "invalid_message_id"
				return
			}
			mid := len(batch) / 2
			visit(batch[:mid])
			visit(batch[mid:])
			return
		}
		items, known := mgetCollection(data)
		if !known {
			stopped = apperrors.NewAPI("批读响应缺少可识别消息集合", apperrors.WithReason("message_batch_shape_unknown"))
			return
		}
		allowed := map[string]bool{}
		for _, id := range batch {
			allowed[id] = true
		}
		for _, item := range items {
			id := mgetID(chatmsg.MessageID(item))
			// Do not leak an unexpected message or duplicate it in the output.
			if allowed[id] && found[id] == nil {
				found[id] = item
			}
		}
		for _, id := range batch {
			if found[id] == nil {
				failed[id] = "not_returned"
			}
		}
	}
	visit(ids)
	result.aborted = stopped != nil
	if stopped != nil && len(found) == 0 {
		return result, stopped
	}
	for _, id := range ids {
		if item := found[id]; item != nil {
			result.messages = append(result.messages, item)
			continue
		}
		reason := failed[id]
		if reason == "" {
			reason = "query_aborted"
		}
		message := "下层未返回该消息；不存在、不可访问或其他原因尚不能区分"
		if reason == "invalid_message_id" {
			message = "下层明确拒绝此消息 ID 的格式"
		}
		if reason == "query_aborted" {
			message = "查询因非 ID 错误停止；此项读取结果未知"
		}
		failure := map[string]any{"stage": "mget", "messageId": id, "reason": reason, "error": message}
		if reason == "query_aborted" {
			var typed *apperrors.Error
			if errors.As(stopped, &typed) {
				failure["causeCategory"] = string(typed.Category)
				failure["causeReason"] = typed.Reason
				failure["serverErrorCode"] = typed.ServerDiag.ServerErrorCode
			}
		}
		result.missing = append(result.missing, id)
		result.failures = append(result.failures, failure)
	}
	if len(found) == 0 {
		for _, reason := range failed {
			if reason == "invalid_message_id" {
				return result, apperrors.NewAPI("未取回任何消息，且下游拒绝了无效消息 ID", apperrors.WithReason("message_batch_failed"), apperrors.WithDetails(map[string]any{"requestedCount": len(ids), "foundCount": 0, "notFoundCount": len(ids), "complete": false, "batchRequests": result.requests, "failures": result.failures}))
			}
		}
	}
	return result, nil
}

// mgetCollection distinguishes a known empty collection from an unrecognized
// response; enrichment must not claim success based on an absent result field.
func mgetCollection(data map[string]any) ([]map[string]any, bool) {
	scopes := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if nested, ok := data[key].(map[string]any); ok {
			scopes = append(scopes, nested)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"messages", "replies", "items", "list", "records", "result", "data"} {
			if values, ok := scope[key].([]any); ok {
				out := make([]map[string]any, 0, len(values))
				for _, value := range values {
					item, ok := value.(map[string]any)
					if !ok {
						return nil, false
					}
					out = append(out, item)
				}
				return out, true
			}
		}
	}
	return nil, false
}

func enrichMessagesMget(rt *shortcut.RuntimeContext, messages []map[string]any) (map[string]any, map[string]map[string]any, []map[string]any) {
	ledger := map[string]any{"complete": true, "enrichedCount": 0, "reactionRequests": 0, "threadRequests": 0, "threadReplyCount": 0, "reactionSkipped": rt.Bool("no-reactions"), "threadSkipped": rt.Bool("no-threads"), "threadPerLimit": 10, "threadTotalLimit": 500, "threadRequestLimit": 50}
	failures := []map[string]any{}
	views := map[string]map[string]any{}
	resources := append([]map[string]any{}, messages...)
	all := append([]map[string]any{}, messages...)
	enriched := map[string]bool{}
	threadCache := map[string]map[string]any{}
	threadCount := 0
	lookupStopped := false
	if !rt.Bool("no-threads") {
		for _, message := range messages {
			id, thread, cid := mgetID(chatmsg.MessageID(message)), mgetID(chatmsg.ThreadID(message)), mgetID(chatmsg.ConversationID(message))
			if thread == "" {
				continue
			}
			key := cid + "\x00" + thread
			if view := threadCache[key]; view != nil {
				views[id] = view
				if view["rawReplies"] != nil {
					enriched[id] = true
				}
				continue
			}
			view := map[string]any{"threadId": thread, "conversationId": cid, "complete": false, "replies": []map[string]any{}, "limit": 10}
			views[id], threadCache[key] = view, view
			if cid == "" {
				failures = append(failures, map[string]any{"stage": "thread", "messageId": id, "reason": "conversation_missing"})
				continue
			}
			if threadCount >= 500 || ledger["threadRequests"].(int) >= 50 {
				view["stopReason"] = "total_limit"
				ledger["complete"] = false
				continue
			}
			ledger["threadRequests"] = ledger["threadRequests"].(int) + 1
			data, err := rt.CallMCPData("chat", "list_topic_replies", map[string]any{"openconversationId": cid, "topicId": thread, "pageSize": 10, "forward": false})
			if err != nil {
				failures = append(failures, map[string]any{"stage": "thread", "messageId": id, "reason": "lookup_failed"})
				lookupStopped = true
				break
			}
			items, known := mgetCollection(data)
			if !known {
				failures = append(failures, map[string]any{"stage": "thread", "messageId": id, "reason": "response_shape_unknown"})
				continue
			}
			page := chatmsg.Pagination(data)
			hasMore, pageKnown := page["hasMore"].(bool)
			view["paginationKnown"], view["hasMore"] = pageKnown, hasMore
			if hasMore {
				view["nextCursor"] = page["nextCursor"]
			}
			limit := min(10, 500-threadCount)
			overflow := len(items) > limit
			if overflow {
				items = items[:limit]
			}
			view["complete"] = pageKnown && !hasMore && !overflow
			if view["complete"] != true {
				ledger["complete"] = false
				view["stopReason"] = "bounded_or_unknown"
			}
			seen := map[string]bool{}
			replies := []map[string]any{}
			for _, item := range items {
				rid := mgetID(chatmsg.MessageID(item))
				if rid == "" || (mgetID(chatmsg.ConversationID(item)) != "" && mgetID(chatmsg.ConversationID(item)) != cid) || (mgetID(chatmsg.ThreadID(item)) != "" && mgetID(chatmsg.ThreadID(item)) != thread) {
					failures = append(failures, map[string]any{"stage": "thread", "messageId": id, "reason": "reply_identity_mismatch"})
					view["complete"] = false
					continue
				}
				if seen[rid] {
					continue
				}
				seen[rid] = true
				if mgetID(chatmsg.ConversationID(item)) == "" {
					item["openConversationId"] = cid
				}
				replies = append(replies, item)
			}
			view["rawReplies"] = replies
			threadCount += len(replies)
			all = append(all, replies...)
			resources = append(resources, replies...)
			enriched[id] = true
		}
	}
	if !rt.Bool("no-reactions") && !lookupStopped {
		requests, enrichedIDs, reactionFailures := EnrichMessageReactions(rt, all)
		ledger["reactionRequests"] = requests
		failures = append(failures, reactionFailures...)
		for id := range enrichedIDs {
			enriched[id] = true
		}
	}
	for _, view := range threadCache {
		if replies, ok := view["rawReplies"].([]map[string]any); ok {
			decrypt := decryptMessageItemsIfRequested(rt, replies)
			view["replies"] = projectMessageMapsWithReactions(replies, !rt.Bool("no-reactions"))
			applyMessageDecryptLedger(view, decrypt)
			if len(decrypt.failures) > 0 {
				view["complete"] = false
			}
			if view["complete"] != true {
				ledger["complete"] = false
			}
			delete(view, "rawReplies")
		}
	}
	if lookupStopped {
		ledger["stopReason"] = "lookup_failed"
	}
	if len(failures) > 0 {
		ledger["complete"] = false
	}
	ledger["failures"], ledger["failedCount"], ledger["threadReplyCount"] = failures, len(failures), threadCount
	ledger["enrichedCount"] = len(enriched)
	return ledger, views, resources
}

// EnrichMessageReactions is shared by exact reads, history, searches and Thread
// readers. It mutates only known messages and never turns a missing row into
// an empty reaction set.
func EnrichMessageReactions(rt *shortcut.RuntimeContext, all []map[string]any) (requests int, enriched map[string]bool, failures []map[string]any) {
	enriched = map[string]bool{}
	failures = []map[string]any{}
	byID := map[string][]map[string]any{}
	ids := []string{}
	for _, item := range all {
		id := mgetID(chatmsg.MessageID(item))
		if id == "" {
			continue
		}
		if byID[id] == nil {
			ids = append(ids, id)
		}
		byID[id] = append(byID[id], item)
	}
	for start := 0; start < len(ids); start += 20 {
		batch := ids[start:min(start+20, len(ids))]
		requests++
		data, err := rt.CallMCPData("im", "list_message_emotion_replies", map[string]any{"openMessageIds": batch})
		items, known := mgetCollection(data)
		if err != nil || !known {
			failures = append(failures, map[string]any{"stage": "reaction", "messageIds": batch, "reason": "lookup_failed_or_unknown"})
			break
		}
		returned := map[string]map[string]any{}
		for _, item := range items {
			returned[mgetID(chatmsg.MessageID(item))] = item
		}
		for _, id := range batch {
			item := returned[id]
			if item == nil {
				failures = append(failures, map[string]any{"stage": "reaction", "messageId": id, "reason": "not_returned"})
				continue
			}
			value, ok := item["emotionReplyList"].([]any)
			for _, entry := range value {
				if _, valid := entry.(map[string]any); !valid {
					ok = false
					break
				}
			}
			if !ok {
				failures = append(failures, map[string]any{"stage": "reaction", "messageId": id, "reason": "response_shape_unknown"})
				continue
			}
			for _, target := range byID[id] {
				target["emotionReplyList"] = value
			}
			enriched[id] = true
		}
	}
	return requests, enriched, failures
}

// EnrichMessageDetails reuses the exact-read bounded Thread/Reaction pipeline.
func EnrichMessageDetails(rt *shortcut.RuntimeContext, messages []map[string]any) (map[string]any, map[string]map[string]any, []map[string]any) {
	return enrichMessagesMget(rt, messages)
}
