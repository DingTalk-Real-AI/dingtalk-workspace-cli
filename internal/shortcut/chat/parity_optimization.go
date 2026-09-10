package chat

import (
	"fmt"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

var messageResourceReferences = chatmsg.Resources

func copyChatBusinessFields(source map[string]any, keys ...string) map[string]any {
	row := map[string]any{}
	for _, key := range keys {
		if value, ok := source[key]; ok {
			row[key] = value
		}
	}
	return row
}

// StrictChatCollection accepts explicit collections only. An unrecognised
// successful response is not evidence of an empty business result.
func StrictChatCollection(data map[string]any, keys ...string) ([]map[string]any, error) {
	for depth := 0; depth < 8 && data != nil; depth++ {
		for _, key := range keys {
			value, present := data[key]
			if !present {
				continue
			}
			if rows, ok := value.([]map[string]any); ok {
				return rows, nil
			}
			array, ok := value.([]any)
			if !ok {
				return nil, apperrors.NewAPI("下游集合字段类型错误: " + key)
			}
			rows := make([]map[string]any, 0, len(array))
			for _, item := range array {
				row, ok := item.(map[string]any)
				if !ok || row == nil {
					return nil, apperrors.NewAPI("下游集合包含非对象记录: " + key)
				}
				rows = append(rows, row)
			}
			return rows, nil
		}
		if raw, ok := data["result"]; ok {
			switch raw.(type) {
			case []any, []map[string]any:
				return StrictChatCollection(map[string]any{"items": raw}, "items")
			}
		}
		child, ok := data["result"].(map[string]any)
		if !ok {
			child, ok = data["data"].(map[string]any)
		}
		if !ok {
			break
		}
		data = child
	}
	return nil, apperrors.NewAPI("下游缺少明确的集合字段，无法判定结果为空")
}

func ReadExactMessage(rt *shortcut.RuntimeContext, conversationID, messageID string) (map[string]any, error) {
	return exactChatMessage(rt, conversationID, messageID)
}

// OutputCheckedRead preserves the command's business payload while making an
// unproven/partial result nonzero. Explicit continuation remains in the payload.
func OutputCheckedRead(rt *shortcut.RuntimeContext, payload map[string]any) error {
	if err := rt.Output(payload); err != nil {
		return err
	}
	if complete, declared := payload["complete"].(bool); declared && !complete {
		return apperrors.NewAPI("结果未完成；请检查完整性、失败明细和续页信息", apperrors.WithReason("incomplete_result"))
	}
	return nil
}

func executeExactFeedGroupQuery(rt *shortcut.RuntimeContext) error {
	ids := uniqueShortcutStrings(append(rt.StrSlice("conversation-ids"), rt.StrSlice("feed-id")...))
	if len(ids) == 0 || len(ids) > 100 {
		return apperrors.NewValidation("--conversation-ids 去重后必须为1–100项")
	}
	categoryID := rt.IntFirst("category-id", "feed-group-id")
	category := strconv.Itoa(categoryID)
	data, err := rt.CallMCPData("im", "list_conversations_by_category", map[string]any{"categoryId": categoryID, "excludeMuted": rt.Bool("exclude-muted")})
	if err != nil {
		return err
	}
	conversations, err := categoryConversationsProject(data)
	if err != nil {
		return err
	}
	payload := feedGroupQueryProject(conversations, ids)
	missing, _ := payload["notFoundConversationIds"].([]string)
	exhausted, paginationMode, err := resolveCategoryConversationsPagination(data)
	if err != nil {
		return err
	}
	known := true
	// A terminal list for a nonexistent category cannot prove non-membership.
	if len(missing) > 0 && known && !exhausted {
		categories, readErr := rt.CallMCPData("im", "get_conv_categories_info", map[string]any{"categoryIds": []int64{int64(categoryID)}})
		verified := false
		if readErr == nil {
			if cats, e := StrictChatCollection(categories, "categories", "categoryList", "items", "list"); e == nil {
				for _, cat := range cats {
					if fmt.Sprint(cat["categoryId"]) == category {
						verified = true
					}
				}
			}
		}
		if !verified {
			known = false
		}
	}
	failures := []map[string]any{}
	unresolved := []string{}
	notFound := []string{}
	items, _ := payload["items"].([]map[string]any)
	for _, id := range missing {
		if known && !exhausted {
			notFound = append(notFound, id)
			continue
		}
		// A positive reverse membership proves this requested target even if
		// the category list cannot prove global exhaustion. A negative does not.
		reverse, readErr := rt.CallMCPData("im", "list_conv_categories_by_conv", map[string]any{"openConversationId": id})
		var cats []map[string]any
		if readErr == nil {
			cats, readErr = StrictChatCollection(reverse, "categories", "categoryList", "items")
		}
		found := false
		for _, cat := range cats {
			if fmt.Sprint(cat["categoryId"]) == category {
				found = true
			}
		}
		if found && !rt.Bool("exclude-muted") {
			items = append(items, map[string]any{"openConversationId": id, "membershipVerified": true})
		} else {
			unresolved = append(unresolved, id)
			reason := "成员关系的缺失或免打扰状态未经完整性合同证明"
			if readErr != nil {
				reason = readErr.Error()
			}
			failures = append(failures, map[string]any{"id": id, "stage": "membership", "error": reason})
		}
	}
	// Restore input order after reverse lookups and retain negative/unknown separately.
	ordered := feedGroupQueryProject(items, ids)
	payload["items"] = ordered["items"]
	payload["foundCount"] = ordered["foundCount"]
	payload["notFoundConversationIds"], payload["notFoundCount"] = notFound, len(notFound)
	payload["unresolvedConversationIds"], payload["unresolvedCount"] = unresolved, len(unresolved)
	payload["complete"], payload["ok"] = len(unresolved) == 0, len(unresolved) == 0 && len(notFound) == 0
	payload["paginationKnown"], payload["failures"], payload["failedCount"] = known, failures, len(failures)
	payload["paginationMode"] = paginationMode
	payload["sourceExhausted"] = known && !exhausted
	if known {
		payload["hasMore"] = exhausted
	}
	if !rt.Bool("no-detail") {
		if rows, ok := payload["items"].([]map[string]any); ok {
			if err := attachConversationDetails(rt, payload, rows); err != nil {
				return err
			}
		}
	}
	return OutputCheckedRead(rt, payload)
}

func executeOptimizedReadStatus(rt *shortcut.RuntimeContext) error {
	messageID := rt.Str("message-id")
	message, err := exactChatMessage(rt, rt.StrFirst("conversation-id", "group", "id"), messageID)
	if err != nil {
		return err
	}
	params := map[string]any{"openMessageId": messageID, "openConversationId": chatmsg.ConversationID(message)}
	targets := uniqueShortcutStrings(rt.StrSlice("users"))
	if rt.Changed("users") && len(targets) == 0 {
		return apperrors.NewValidation("显式--users不能为空")
	}
	userIDs, openIDs := splitIDs(targets)
	for _, id := range userIDs {
		resolved, err := resolveUserOpenDingTalkID(rt, id)
		if err != nil {
			return err
		}
		openIDs = append(openIDs, resolved)
	}
	openIDs = uniqueShortcutStrings(openIDs)
	if len(openIDs) > 0 {
		params["targetOpenDingTalkIds"] = openIDs
	}
	if rt.DryRun() {
		return rt.CallMCP("query_msg_read_status", params)
	}
	data, err := rt.CallMCPData("im", "query_msg_read_status", params)
	if err != nil {
		return err
	}
	// Validate the identity-bearing response without guessing a count or
	// converting unavailable identities to unread. The native payload remains.
	if len(openIDs) > 0 {
		allowed := map[string]bool{}
		for _, id := range openIDs {
			allowed[id] = true
		}
		var inspect func(any) error
		inspect = func(value any) error {
			switch v := value.(type) {
			case []any:
				for _, item := range v {
					if e := inspect(item); e != nil {
						return e
					}
				}
			case map[string]any:
				for _, key := range []string{"openDingTalkId", "openDingtalkId", "targetOpenDingTalkId"} {
					if id, ok := v[key].(string); ok && strings.TrimSpace(id) != "" && !allowed[id] {
						return apperrors.NewAPI("已读查询返回了非请求目标，拒绝输出")
					}
				}
				for _, item := range v {
					if e := inspect(item); e != nil {
						return e
					}
				}
			}
			return nil
		}
		if err := inspect(data); err != nil {
			return err
		}
	}
	return rt.Output(data)
}

func resolveDownloadIdentity(rt *shortcut.RuntimeContext) (kind, messageID, conversationID string, err error) {
	kind = rt.Str("type")
	messageID = rt.Str("message-id")
	conversationID = rt.Str("open-conversation-id")
	if messageID == "" {
		return kind, messageID, conversationID, nil
	}
	message, err := exactChatMessage(rt, conversationID, messageID)
	if err != nil {
		return "", "", "", err
	}
	conversationID = fmt.Sprint(chatmsg.ConversationID(message))
	resources := messageResourceReferences(message)
	var matched map[string]any
	for _, r := range resources {
		if fmt.Sprint(r["resourceId"]) == rt.StrFirst("resource-id", "file-key") {
			if matched != nil {
				return "", "", "", apperrors.NewAPI("资源ID在消息中不唯一")
			}
			matched = r
		}
	}
	if matched == nil {
		return "", "", "", apperrors.NewValidation("请求资源不属于指定消息或缺少可验证资源元数据")
	}
	resolved := fmt.Sprint(matched["type"])
	if kind == "image" || kind == "file" {
		if matched["contentType"] != kind {
			return "", "", "", apperrors.NewValidation("内容类型与消息资源不匹配")
		}
	} else if rt.Changed("type") && kind != resolved {
		return "", "", "", apperrors.NewValidation("资源ID类型与消息元数据不匹配")
	}
	if resolved != "mediaId" && resolved != "fileId" {
		return "", "", "", apperrors.NewAPI("消息资源ID类型未知")
	}
	return resolved, messageID, conversationID, nil
}

func attachConversationDetails(rt *shortcut.RuntimeContext, payload map[string]any, rows []map[string]any) error {
	failures := []map[string]any{}
	for _, row := range rows {
		id := shortcutString(row, "openConversationId", "conversationId")
		if id == "" {
			return apperrors.NewAPI("会话条目缺少稳定ID")
		}
		data, err := rt.CallMCPData("chat", "get_conversation_info", map[string]any{"openConversationId": id})
		var detail map[string]any
		if err == nil {
			for depth := 0; depth < 8; depth++ {
				if got := shortcutString(data, "openConversationId", "conversationId", "openCid"); got == id {
					detail = data
					break
				}
				child, ok := data["result"].(map[string]any)
				if !ok {
					child, ok = data["conversationInfo"].(map[string]any)
				}
				if !ok {
					break
				}
				data = child
			}
		}
		if detail == nil {
			failures = append(failures, map[string]any{"stage": "conversation-detail", "id": id, "reason": "read_failed_or_identity_mismatch"})
			continue
		}
		row["detail"] = detail
	}
	payload["enrichmentComplete"] = len(failures) == 0
	if len(failures) > 0 {
		prior, _ := payload["failures"].([]map[string]any)
		prior = append(prior, failures...)
		payload["failures"], payload["failedCount"], payload["complete"] = prior, len(prior), false
		return OutputCheckedRead(rt, payload)
	}
	return nil
}

func enrichFavoriteItems(rt *shortcut.RuntimeContext, items []map[string]any) []map[string]any {
	failures := []map[string]any{}
	ids := []string{}
	targets := map[string]map[string]any{}
	found := map[string]map[string]any{}
	for _, item := range items {
		id := shortcutString(item, "openMessageId", "messageId")
		if id == "" {
			failures = append(failures, map[string]any{"stage": "favorite-enrichment", "reason": "message_identity_missing"})
			continue
		}
		if targets[id] == nil {
			ids = append(ids, id)
		}
		targets[id] = item
	}
	for start := 0; start < len(ids); start += 50 {
		batch := ids[start:min(start+50, len(ids))]
		allowed := map[string]bool{}
		for _, id := range batch {
			allowed[id] = true
		}
		data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": batch})
		rows, known := mgetCollection(data)
		if err != nil || !known {
			failures = append(failures, map[string]any{"stage": "favorite-enrichment", "reason": "read_failed_or_unknown"})
			break
		}
		for _, row := range rows {
			id := mgetID(chatmsg.MessageID(row))
			if !allowed[id] {
				continue
			}
			expected := shortcutString(targets[id], "openConversationId", "conversationId")
			if expected != "" && mgetID(chatmsg.ConversationID(row)) != expected {
				failures = append(failures, map[string]any{"stage": "favorite-enrichment", "id": id, "reason": "conversation_mismatch"})
				continue
			}
			if found[id] != nil {
				failures = append(failures, map[string]any{"stage": "favorite-enrichment", "id": id, "reason": "duplicate_identity"})
				continue
			}
			found[id] = row
		}
	}
	valid := []map[string]any{}
	for _, id := range ids {
		if row := found[id]; row != nil {
			valid = append(valid, row)
		} else {
			failures = append(failures, map[string]any{"stage": "favorite-enrichment", "id": id, "reason": "not_returned"})
		}
	}
	// One enrichment budget across every favorite page/batch, not one budget per batch.
	if rt.Bool("with-threads") {
		ledger, views, _ := EnrichMessageDetails(rt, valid)
		if fs, ok := ledger["failures"].([]map[string]any); ok {
			failures = append(failures, fs...)
		}
		for _, id := range ids {
			targets[id]["enrichmentComplete"] = ledger["complete"]
			if view := views[id]; view != nil {
				targets[id]["thread"] = view
			}
		}
	} else if !rt.Bool("no-reactions") {
		_, _, rf := EnrichMessageReactions(rt, valid)
		failures = append(failures, rf...)
	}
	for _, row := range valid {
		targets[mgetID(chatmsg.MessageID(row))]["message"] = chatmsg.ProjectMessageV1(row, !rt.Bool("no-reactions"))
	}
	return failures
}
