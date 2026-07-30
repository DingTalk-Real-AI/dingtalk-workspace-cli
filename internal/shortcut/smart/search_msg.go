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

package smart

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

// SearchMsg is the semantic message-search entry point. It exposes the native
// IM search dimensions, can exhaust cursor pagination, and enriches sparse
// search hits through list_messages_by_ids in chunks of 50. A later-page or
// enrichment failure never turns a partial result into a false success: the
// output carries an explicit failure ledger and complete=false.
var SearchMsg = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+search-msg",
	Product:     "im",
	Description: "多维搜索消息，可全量翻页并批量富化详情",
	Intent: "当你要按关键词、发送者、@对象、会话、消息类型或机器人来源组合搜索 IM 消息时使用；默认查询近 7 天，" +
		"也可指定精确起止时间。--page-all 会连续拉取游标页，默认再按消息 ID 分批富化详情；任何续页或富化失败都会保留已取得结果并返回逐项失败 ledger，绝不把截断结果标成完整。" +
		"--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘，按既有安全下载约定无需交互确认。",
	Risk: shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "group", Type: shortcut.FlagString, Desc: "单个会话 openConversationId"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "groups", Type: shortcut.FlagStringSlice, Desc: "多个会话 openConversationId"},
		{Name: "chat-id", Type: shortcut.FlagStringSlice, Desc: "--groups 的 lark-cli 对齐别名"},
		{Name: "senders", Type: shortcut.FlagStringSlice, Desc: "发送者 userId/openDingTalkId 列表"},
		{Name: "sender", Type: shortcut.FlagStringSlice, Desc: "--senders 的 lark-cli 对齐别名"},
		{Name: "at-me", Type: shortcut.FlagBool, Desc: "只搜索 @我 的消息"},
		{Name: "is-at-me", Type: shortcut.FlagBool, Desc: "--at-me 的 lark-cli 对齐别名"},
		{Name: "at-ids", Type: shortcut.FlagStringSlice, Desc: "@对象 userId/openDingTalkId 列表"},
		{Name: "message-type", Type: shortcut.FlagString, Desc: "下层消息类型过滤值（以当前 IM Schema 为准）"},
		{Name: "only-robot", Type: shortcut.FlagBool, Desc: "只搜索机器人消息"},
		{Name: "conversation-type", Type: shortcut.FlagString, Desc: "下层会话类型过滤值（以当前 IM Schema 为准）"},
		{Name: "chat-type", Type: shortcut.FlagString, Desc: "--conversation-type 的 lark-cli 对齐别名"},
		{Name: "days", Type: shortcut.FlagInt, Desc: "默认时间窗的回溯天数", Default: "7"},
		{Name: "start", Type: shortcut.FlagString, Desc: "精确开始时间（RFC3339，需与 --end 一起传）"},
		{Name: "end", Type: shortcut.FlagString, Desc: "精确结束时间（RFC3339，需与 --start 一起传）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量（1-100）", Default: "100"},
		{Name: "page-size", Type: shortcut.FlagInt, Desc: "--limit 的 lark-cli 对齐别名（1-100）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传上次的 nextCursor", Default: "0"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "--cursor 的 lark-cli 对齐别名"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动连续拉取所有游标页"},
		{Name: "page-limit", Type: shortcut.FlagInt, Desc: "--page-all 的最大页数（1-40）", Default: "20"},
		{Name: "no-enrich", Type: shortcut.FlagBool, Desc: "不再按消息 ID 批量查询完整详情"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出命中消息的 reaction（默认输出）"},
	}, chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintAtLeastOne,
			Flags:       []string{"query", "keyword", "group", "conversation-id", "id", "groups", "chat-id", "senders", "sender", "at-me", "is-at-me", "at-ids", "message-type", "only-robot", "conversation-type", "chat-type"},
			Description: "至少指定一个内容、身份、会话或消息类型过滤条件",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"start"},
			Description: "需与 --end 一起传",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"end"},
			Description: "需与 --start 一起传",
		},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"groups", "chat-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"senders", "sender"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"at-me", "is-at-me"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"conversation-type", "chat-type"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"limit", "page-size"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"cursor", "page-token"}},
	}, chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +search-msg --group <openConversationId> --query "changefree"`,
		`dws chat +search-msg --senders <openDingTalkId> --at-me --days 3 --page-all`,
	},
	Validate: validateSearchMsgWithResources,
	Execute: func(rt *shortcut.RuntimeContext) error {
		params, err := searchMsgParams(rt)
		if err != nil {
			return err
		}

		pageLimit := 1
		if rt.Bool("page-all") {
			pageLimit = rt.Int("page-limit")
		}
		cursor := rt.StrFirst("page-token", "cursor")
		messages := make([]map[string]any, 0)
		seen := map[string]bool{}
		failures := make([]map[string]any, 0)
		pagesFetched := 0
		complete := true
		hasMore := false
		nextCursor := ""

		for pagesFetched < pageLimit {
			params["cursor"] = cursor
			data, callErr := rt.CallMCPData("im", "search_messages", params)
			if callErr != nil {
				if pagesFetched == 0 {
					return callErr
				}
				failures = append(failures, map[string]any{
					"stage":  "search-page",
					"cursor": cursor,
					"error":  callErr.Error(),
				})
				complete = false
				break
			}
			pagesFetched++
			for _, message := range searchMsgItems(data) {
				messageID := strings.TrimSpace(fmt.Sprint(searchMsgMessageID(message)))
				if messageID != "" && messageID != "<nil>" {
					if seen[messageID] {
						continue
					}
					seen[messageID] = true
				}
				messages = append(messages, message)
			}

			page := chatmsg.Pagination(data)
			hasMoreValue, hasMoreKnown := page["hasMore"].(bool)
			nextCursor = strings.TrimSpace(fmt.Sprint(page["nextCursor"]))
			if !hasMoreKnown {
				if nextCursor != "" && nextCursor != "<nil>" {
					hasMoreValue = true
				} else {
					failures = append(failures, map[string]any{
						"stage": "search-pagination",
						"error": "下层未返回 hasMore 或 nextCursor，无法证明结果完整",
					})
					complete = false
					break
				}
			}
			hasMore = hasMoreValue
			if !rt.Bool("page-all") || !hasMore {
				complete = !hasMore
				break
			}
			if nextCursor == "" || nextCursor == "<nil>" || nextCursor == cursor {
				failures = append(failures, map[string]any{
					"stage": "search-page",
					"error": "下层返回 hasMore=true，但缺少可继续且会前进的 nextCursor",
				})
				complete = false
				break
			}
			cursor = nextCursor
		}
		if rt.Bool("page-all") && hasMore && pagesFetched == pageLimit {
			failures = append(failures, map[string]any{
				"stage": "search-page-limit",
				"error": fmt.Sprintf("达到 --page-limit=%d，仍有更多结果", pageLimit),
			})
			complete = false
		}

		enrichedCount := 0
		if !rt.Bool("no-enrich") && len(messages) > 0 {
			var enrichFailures []map[string]any
			messages, enrichedCount, enrichFailures = enrichSearchMessages(rt, messages)
			failures = append(failures, enrichFailures...)
			if len(enrichFailures) > 0 {
				complete = false
			}
		}

		results := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			results = append(results, searchMsgProjectWithReactions(m, !rt.Bool("no-reactions")))
		}
		payload := map[string]any{
			"count":         len(results),
			"messages":      results,
			"pagesFetched":  pagesFetched,
			"enrichedCount": enrichedCount,
			"complete":      complete,
			"hasMore":       hasMore,
			"failedCount":   len(failures),
			"failures":      failures,
		}
		if hasMore && nextCursor != "" && nextCursor != "<nil>" {
			payload["nextCursor"] = nextCursor
		}
		if rt.Bool("download-resources") {
			payload["resourceDownloads"] = chatshortcut.DownloadMessageResources(rt, messages, "")
		}
		return rt.Output(payload)
	},
}

func validateSearchMsgWithResources(rt *shortcut.RuntimeContext) error {
	if err := validateSearchMsg(rt); err != nil {
		return err
	}
	return chatshortcut.ValidateMessageResourceDownload(rt)
}

func validateSearchMsg(rt *shortcut.RuntimeContext) error {
	hasFilter := rt.StrFirst("query", "keyword", "group", "conversation-id", "id", "message-type", "conversation-type", "chat-type") != "" ||
		len(rt.StrSlice("groups")) > 0 ||
		len(rt.StrSlice("chat-id")) > 0 ||
		len(rt.StrSlice("senders")) > 0 ||
		len(rt.StrSlice("sender")) > 0 ||
		len(rt.StrSlice("at-ids")) > 0 ||
		rt.Bool("at-me") ||
		rt.Bool("is-at-me") ||
		rt.Bool("only-robot")
	if !hasFilter {
		return apperrors.NewValidation("至少指定一个过滤条件，例如 --query、--group、--senders、--at-me 或 --message-type")
	}
	if rt.Changed("start") != rt.Changed("end") {
		return apperrors.NewValidation("--start 与 --end 必须同时指定")
	}
	if days := rt.Int("days"); days < 1 || days > 3650 {
		return apperrors.NewValidation("--days 必须在 1-3650 之间")
	}
	limit := rt.IntFirst("limit", "page-size")
	if limit < 1 || limit > 100 {
		return apperrors.NewValidation("--limit 必须在 1-100 之间")
	}
	if pageLimit := rt.Int("page-limit"); pageLimit < 1 || pageLimit > 40 {
		return apperrors.NewValidation("--page-limit 必须在 1-40 之间")
	}
	return nil
}

func searchMsgParams(rt *shortcut.RuntimeContext) (map[string]any, error) {
	params := map[string]any{"limit": rt.IntFirst("limit", "page-size")}
	if value := rt.StrFirst("query", "keyword"); value != "" {
		params["keyword"] = value
	}
	conversationIDs := append([]string{}, rt.StrSlice("groups")...)
	conversationIDs = append(conversationIDs, rt.StrSlice("chat-id")...)
	if value := rt.StrFirst("group", "conversation-id", "id"); value != "" {
		conversationIDs = append(conversationIDs, value)
	}
	if values := uniqueSearchStrings(conversationIDs); len(values) > 0 {
		params["openConversationIds"] = values
	}
	senders := append([]string{}, rt.StrSlice("senders")...)
	senders = append(senders, rt.StrSlice("sender")...)
	appendSearchActorIDs(params, senders, "senderUserIds", "senderOpenDingTakIds")
	appendSearchActorIDs(params, rt.StrSlice("at-ids"), "atUserIds", "atOpenDingTakIds")
	if rt.Bool("at-me") || rt.Bool("is-at-me") {
		params["atMe"] = true
	}
	if value := rt.Str("message-type"); value != "" {
		params["messageType"] = value
	}
	if rt.Changed("only-robot") {
		params["onlyRobotMessages"] = rt.Bool("only-robot")
	}
	if value := rt.StrFirst("conversation-type", "chat-type"); value != "" {
		params["searchConvType"] = value
	}

	if rt.Changed("start") && rt.Changed("end") {
		start, err := time.Parse(time.RFC3339, rt.Str("start"))
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("--start 必须是 RFC3339 时间: %v", err))
		}
		end, err := time.Parse(time.RFC3339, rt.Str("end"))
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("--end 必须是 RFC3339 时间: %v", err))
		}
		if !end.After(start) {
			return nil, apperrors.NewValidation("--end 必须晚于 --start")
		}
		params["startTime"] = start.UnixMilli()
		params["endTime"] = end.UnixMilli()
	} else {
		now := time.Now()
		params["startTime"] = now.AddDate(0, 0, -rt.Int("days")).UnixMilli()
		params["endTime"] = now.UnixMilli()
	}
	return params, nil
}

func appendSearchActorIDs(params map[string]any, values []string, userKey, openIDKey string) {
	var userIDs, openIDs []string
	for _, value := range uniqueSearchStrings(values) {
		if len(value) > 0 && (value[0] == 'D' || value[0] == 'd') {
			openIDs = append(openIDs, value)
		} else {
			userIDs = append(userIDs, value)
		}
	}
	if len(userIDs) > 0 {
		params[userKey] = userIDs
	}
	if len(openIDs) > 0 {
		params[openIDKey] = openIDs
	}
}

func uniqueSearchStrings(values []string) []string {
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

func enrichSearchMessages(rt *shortcut.RuntimeContext, messages []map[string]any) ([]map[string]any, int, []map[string]any) {
	detailsByID := map[string]map[string]any{}
	failures := make([]map[string]any, 0)
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		if id := strings.TrimSpace(fmt.Sprint(searchMsgMessageID(message))); id != "" && id != "<nil>" {
			ids = append(ids, id)
		}
	}
	ids = uniqueSearchStrings(ids)
	for start := 0; start < len(ids); start += 50 {
		end := start + 50
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": chunk})
		if err != nil {
			failures = append(failures, map[string]any{
				"stage":      "message-enrichment",
				"messageIds": chunk,
				"error":      err.Error(),
			})
			continue
		}
		foundInChunk := map[string]bool{}
		for _, detail := range searchMsgItems(data) {
			if id := strings.TrimSpace(fmt.Sprint(searchMsgMessageID(detail))); id != "" && id != "<nil>" {
				detailsByID[id] = detail
				foundInChunk[id] = true
			}
		}
		missing := make([]string, 0)
		for _, id := range chunk {
			if !foundInChunk[id] {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			failures = append(failures, map[string]any{
				"stage":             "message-enrichment",
				"missingMessageIds": missing,
				"error":             "mget 未返回全部请求消息",
			})
		}
	}

	enriched := 0
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		id := strings.TrimSpace(fmt.Sprint(searchMsgMessageID(message)))
		detail := detailsByID[id]
		if detail == nil {
			out = append(out, message)
			continue
		}
		merged := make(map[string]any, len(message)+len(detail))
		for key, value := range message {
			merged[key] = value
		}
		for key, value := range detail {
			merged[key] = value
		}
		out = append(out, merged)
		enriched++
	}
	return out, enriched, failures
}

// searchMsgItems locates the message list inside a search_messages_by_keyword
// response, probing common container keys at the top level and nested under
// "result". Returns nil when no list is found.
func searchMsgItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	for _, root := range []map[string]any{data, searchMsgChildMap(data, "result")} {
		if root == nil {
			continue
		}
		if groups, ok := root["conversationMessagesList"].([]any); ok {
			return searchMsgFlattenGroups(groups)
		}
	}
	keys := []string{"list", "messages", "messageList", "items", "data", "records", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return searchMsgToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "messages", "messageList", "items", "data", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return searchMsgToMaps(arr)
				}
			}
		}
	}
	return nil
}

func searchMsgChildMap(data map[string]any, key string) map[string]any {
	if value, ok := data[key].(map[string]any); ok {
		return value
	}
	return nil
}

func searchMsgFlattenGroups(groups []any) []map[string]any {
	out := make([]map[string]any, 0)
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			continue
		}
		messages, ok := group["messages"].([]any)
		if !ok {
			continue
		}
		conversationID := strings.TrimSpace(fmt.Sprint(group["openConversationId"]))
		conversationTitle := strings.TrimSpace(fmt.Sprint(group["title"]))
		singleChat, hasSingleChat := group["singleChat"]
		for _, rawMessage := range messages {
			message, ok := rawMessage.(map[string]any)
			if !ok {
				continue
			}
			item := make(map[string]any, len(message)+3)
			for key, value := range message {
				item[key] = value
			}
			if _, exists := item["openConversationId"]; !exists &&
				conversationID != "" && conversationID != "<nil>" {
				item["openConversationId"] = conversationID
			}
			if _, exists := item["conversationTitle"]; !exists &&
				conversationTitle != "" && conversationTitle != "<nil>" {
				item["conversationTitle"] = conversationTitle
			}
			if _, exists := item["singleChat"]; !exists && hasSingleChat {
				item["singleChat"] = singleChat
			}
			out = append(out, item)
		}
	}
	return out
}

func searchMsgToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// searchMsgProject reshapes one matched message into {sender, time, text,
// messageId}, running text through the shared chatmsg cleaning (card/auto-reply
// JSON → readable, ciphertext → marker) and recursively expanding any forwarded
// chat record under "forwarded".
func searchMsgProject(m map[string]any) map[string]any {
	return searchMsgProjectWithReactions(m, true)
}

func searchMsgProjectWithReactions(m map[string]any, includeReactions bool) map[string]any {
	row := map[string]any{
		"sender":    searchMsgSender(m),
		"time":      searchMsgTime(m),
		"text":      searchMsgCleanText(m),
		"messageId": searchMsgMessageID(m),
	}
	if conversationID := chatmsg.ConversationID(m); conversationID != nil {
		row["conversationId"] = conversationID
	}
	if threadID := chatmsg.ThreadID(m); threadID != nil {
		row["threadId"] = threadID
	}
	if messageType := chatmsg.MessageType(m); messageType != nil {
		row["messageType"] = messageType
	}
	if updateTime := chatmsg.UpdateTime(m); updateTime != nil {
		row["updateTime"] = updateTime
	}
	if includeReactions {
		if reactions := chatmsg.Reactions(m); len(reactions) > 0 {
			row["reactions"] = reactions
		}
	}
	if quoted := chatmsg.QuotedMessage(m); len(quoted) > 0 {
		row["quotedMessage"] = quoted
	}
	if resources := chatmsg.ResourcesDeep(m); len(resources) > 0 {
		row["resourceRefs"] = resources
	}
	projectForwarded := func(item map[string]any) map[string]any {
		return searchMsgProjectWithReactions(item, includeReactions)
	}
	if forwarded := chatmsg.Forwarded(m, projectForwarded); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
}

// searchMsgCleanText runs searchMsgText's extraction through chatmsg.CleanText.
func searchMsgCleanText(m map[string]any) any {
	if s, ok := searchMsgText(m).(string); ok {
		return chatmsg.CleanText(s)
	}
	return searchMsgText(m)
}

// searchMsgSender reads a message's sender display name/id, tolerating the
// common sender keys the gateway may use (including a nested sender object). The
// literal string "null" (carried by forwarded sub-messages) and the empty string
// are both treated as absent so they never surface as the speaker.
func searchMsgSender(m map[string]any) any {
	norm := func(v any) string {
		if s := searchMsgString(v); s != "" && s != "null" {
			return s
		}
		return ""
	}
	for _, key := range []string{"senderName", "sender_name", "senderNick", "fromName", "senderStaffName"} {
		if v := norm(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"sender", "from", "senderUser"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"name", "nick", "userName", "staffName", "displayName"} {
				if v := norm(nested[k2]); v != "" {
					return v
				}
			}
		}
		if v := norm(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"senderId", "sender_id", "senderUserId", "senderStaffId", "openDingTalkId"} {
		if v := norm(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// searchMsgTime reads a message's send time, returning the raw value (usually
// epoch millis) under whichever candidate key is present.
func searchMsgTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "time", "msgTime", "createAt"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// searchMsgText reads a message's textual content, tolerating flat text keys and
// a nested content/text object.
func searchMsgText(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "body"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"content", "text", "msg"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"text", "content", "richText", "title"} {
				if v := searchMsgString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// searchMsgMessageID reads a message's identifier, tolerating the common id keys
// the gateway may use.
func searchMsgMessageID(m map[string]any) any {
	for _, key := range []string{"messageId", "message_id", "msgId", "msg_id", "openMessageId", "id"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// searchMsgString coerces a scalar JSON value to a trimmed string, returning ""
// for nil / non-scalar / empty values.
func searchMsgString(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func init() {
	shortcut.Register(SearchMsg)
}
