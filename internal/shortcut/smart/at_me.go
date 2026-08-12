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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const (
	atMeDefaultPageLimit = 50
	atMeHardPageLimit    = 500
)

// AtMe: pull the messages that recently @-mentioned ME across chats in one step.
//
// Steps:
//  1. compute the look-back window [now-Nd, now] in local time and express both
//     bounds as epoch millis. N defaults to 7 days and is overridable via
//     --days; mirroring `dws chat message list-mentions`, which feeds startTime/
//     endTime as epoch millis to search_at_me_message.
//  2. call search_at_me_message on the chat server with startTime/endTime/limit/
//     cursor — the exact parameter names + first-page defaults (limit 50,
//     cursor "0") used by helpers.chatMessageListMentionsCmd.
//  3. defensively project each returned message down to {sender, time, text,
//     conversation} (multiple candidate keys per field) and print via rt.Output
//     so it honours --format/--jq/--fields. When the response carries no
//     recognisable message list we fall back to printing the raw payload.
//
// This replaces manually working out the millisecond time window and copying
// the list-mentions incantation. The default path only searches and reshapes;
// --download-resources additionally writes resource files locally.
//
//	dws chat +at-me
//	dws chat +at-me --days 3
var AtMe = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+at-me",
	Product:     "chat",
	Description: "查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话）",
	Intent: "当你想快速看回最近谁在群里或单聊里 @了你、但不想手动把起止时间换算成毫秒、也不想记 list-mentions 的一堆参数时使用；" +
		"内部按本地时区算出「最近 N 天」（默认 7 天，可用 --days 调整回溯天数）的时间窗，搜索这段时间内 @我 的消息，" +
		"再在本地把每条消息投影成发送人、时间、内容、所在会话四个关键字段。" +
		"默认只读且不会发送、撤回或标记任何消息；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘，按既有安全下载约定无需交互确认。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_at_me",
			CanonicalPath:  "chat.shortcut_at_me",
			CLIPath:        "chat +at-me",
			PrimaryCLIPath: "chat +at-me",
		},
		Description: "查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话）",
			UseWhen:      []string{"当你想快速看回最近谁在群里或单聊里 @了你、但不想手动把起止时间换算成毫秒、也不想记 list-mentions 的一堆参数时使用；内部按本地时区算出「最近 N 天」（默认 7 天，可用 --days 调整回溯天数）的时间窗，搜索这段时间内 @我 的消息，再在本地把每条消息投影成发送人、时间、内容、所在会话四个关键字段。默认只读且不会发送、撤回或标记任何消息；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘，按既有安全下载约定无需交互确认。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws chat +at-me",
				"dws chat +at-me --days 3",
			},
		},
	},
	Flags: append(append([]shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "仅查看指定群；可传 openConversationId 或群名"},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "--group 的旧版自然名称入口", Hidden: true},
		{Name: "group-query", Type: shortcut.FlagString, Desc: "--chat-query 的兼容别名", Hidden: true},
		{Name: "days", Type: shortcut.FlagInt, Desc: "回溯天数（默认 7）；--days 必须在 1-3650 之间", Default: "7", Required: false},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量（默认 50）；--limit 必须大于 0", Default: "50"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传上次的 nextCursor", Default: "0"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "沿 nextCursor 自动读取全部 @我 消息；--page-limit 仅与 --page-all 一起使用且范围 1-500；--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
	}, shortcut.AutoPageControlFlags()...), chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append(append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"group", "chat-query", "group-query"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"days"}, Description: "--days 必须在 1-3650 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlConstraints()...), chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +at-me`,
		`dws chat +at-me --days 3`,
		`dws chat +at-me --group "项目群"`,
		`dws chat +at-me --days 30 --page-all --page-limit 50`,
	},
	Validate: validateAtMe,
	Execute:  executeAtMe,
}

func executeAtMe(rt *shortcut.RuntimeContext) error {
	groupID := ""
	directTarget := strings.TrimSpace(rt.Str("group"))
	queryTarget := strings.TrimSpace(rt.StrFirst("chat-query", "group-query"))
	if directTarget != "" || queryTarget != "" {
		resolved, err := targetresolver.ResolveChatTarget(rt, directTarget, queryTarget)
		if err != nil {
			return err
		}
		groupID = resolved.Selected.OpenConversationID
	}

	// Step 1 — look-back window [now-Nd, now] in epoch millis. Validation has
	// already constrained days to the published 1-3650 range.
	days := rt.Int("days")
	now := time.Now()
	startMs := now.AddDate(0, 0, -days).UnixMilli()
	endMs := now.UnixMilli()

	// Step 2 — search @me messages. startTime/endTime/limit/cursor and the
	// first-page defaults (limit 50, cursor "0") mirror
	// helpers.chatMessageListMentionsCmd's search_at_me_message call.
	params := map[string]any{
		"startTime": startMs,
		"endTime":   endMs,
		"limit":     rt.Int("limit"),
		"cursor":    rt.Str("cursor"),
	}
	if groupID != "" {
		params["openConversationId"] = groupID
	}
	var items []map[string]any
	var payload map[string]any
	var readErr error
	if rt.Bool("page-all") {
		payload, items, readErr = readAllAtMePages(rt, params)
		if payload == nil {
			return readErr
		}
	} else {
		data, err := rt.CallMCPData("chat", "search_at_me_message", params)
		if err != nil {
			return err
		}
		items = atMeMessageItems(data)
		payload = atMePayload(items, !rt.Bool("no-reactions"))
		chatmsg.ApplyPagination(payload, data)
		payload["pagesFetched"] = 1
		if payload["complete"] == true {
			payload["stopReason"] = "source_complete"
		} else {
			payload["stopReason"] = "single_page"
		}
	}

	results := make([]map[string]any, 0, len(items))
	if projected, ok := payload["messages"].([]map[string]any); ok {
		results = projected
	}
	if rt.Bool("download-resources") && readErr == nil {
		payload["resourceDownloads"] = chatshortcut.DownloadMessageResources(rt, items, groupID)
	}
	if len(results) == 0 {
		payload["messages"] = []map[string]any{}
		payload["items"] = []map[string]any{}
	}
	if err := rt.Output(payload); err != nil {
		return err
	}
	return readErr
}

func validateAtMe(rt *shortcut.RuntimeContext) error {
	if err := chatshortcut.ValidateMessageResourceDownload(rt); err != nil {
		return err
	}
	days := rt.Int("days")
	if days < 1 || days > 3650 {
		return localChatOptionError("invalid_lookback_window", "+at-me 的 --days 必须在 1-3650 之间", "--days")
	}
	if rt.Int("limit") <= 0 {
		return localChatOptionError("invalid_page_size", "+at-me 的 --limit 必须大于 0", "--limit")
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > atMeHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	if err := shortcut.ValidateAutoPageControls(rt); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return nil
}

func atMePayload(items []map[string]any, includeReactions bool) map[string]any {
	results := make([]map[string]any, 0, len(items))
	for _, message := range items {
		results = append(results, atMeProjectWithReactions(message, includeReactions))
	}
	payload := chatmsg.NewMessageListPayload(results)
	payload["items"] = atMeCompatibilityItems(results)
	return payload
}

func readAllAtMePages(rt *shortcut.RuntimeContext, baseParams map[string]any) (map[string]any, []map[string]any, error) {
	pageLimit := defaultChatPageLimit(rt.Int("page-limit"), atMeDefaultPageLimit)
	cursor := strings.TrimSpace(fmt.Sprint(baseParams["cursor"]))
	if cursor == "" || cursor == "<nil>" {
		cursor = "0"
	}
	seenCursors := map[string]bool{cursor: true}
	seenMessages := map[string]bool{}
	allItems := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	complete := false
	hasMore := false
	nextCursor := ""
	stopReason := "source_complete"
	truncatedByPageLimit := false
	truncatedByResultLimit := false

	for pagesFetched < pageLimit {
		if pagesFetched > 0 {
			if err := shortcut.WaitAutoPageDelay(rt); err != nil {
				failures = append(failures, map[string]any{
					"page": pagesFetched + 1, "stage": "delay", "cursor": cursor, "error": err.Error(),
				})
				stopReason = "delay_interrupted"
				break
			}
		}
		params := make(map[string]any, len(baseParams))
		for key, value := range baseParams {
			params[key] = value
		}
		params["cursor"] = cursor
		data, err := rt.CallMCPData("chat", "search_at_me_message", params)
		if err != nil {
			if pagesFetched == 0 {
				return nil, nil, err
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "cursor": cursor, "error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageItems := atMeMessageItems(data)
		for _, item := range pageItems {
			id := chatmsg.StableMessageID(item)
			if id != "" && seenMessages[id] {
				continue
			}
			if id != "" {
				seenMessages[id] = true
			}
			if maxItems := rt.Int("max-items"); maxItems > 0 && len(allItems) >= maxItems {
				truncatedByResultLimit = true
				continue
			}
			allItems = append(allItems, item)
		}

		page := chatmsg.Pagination(data)
		pageHasMore, known := page["hasMore"].(bool)
		if !known {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "@我消息下层未返回可靠的 hasMore，无法证明结果完整",
			})
			stopReason = "pagination_error"
			break
		}
		hasMore = pageHasMore
		if !hasMore {
			complete = !truncatedByResultLimit
			nextCursor = ""
			if truncatedByResultLimit {
				hasMore = true
				stopReason = "result_limit"
			} else {
				stopReason = "source_complete"
			}
			break
		}
		nextCursor = atMeCursorString(page["nextCursor"])
		if nextCursor == "" || seenCursors[nextCursor] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "@我消息下层返回 hasMore=true，但 nextCursor 缺失、无效或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
		if maxItems := rt.Int("max-items"); maxItems > 0 && len(allItems) >= maxItems {
			truncatedByResultLimit = true
			stopReason = "result_limit"
			break
		}
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit && !truncatedByResultLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	payload := atMePayload(allItems, !rt.Bool("no-reactions"))
	payload["pagesFetched"] = pagesFetched
	payload["paginationKnown"] = true
	payload["complete"] = complete && len(failures) == 0
	payload["hasMore"] = hasMore
	payload["stopReason"] = stopReason
	payload["truncatedByPageLimit"] = truncatedByPageLimit
	payload["truncatedByResultLimit"] = truncatedByResultLimit
	payload["failedCount"] = len(failures)
	payload["failures"] = failures
	payload["partial"] = len(failures) > 0 && len(allItems) > 0
	chatmsg.ApplyTruncation(payload)
	if hasMore && nextCursor != "" {
		payload["nextCursor"] = nextCursor
	}
	if len(failures) == 0 {
		return payload, allItems, nil
	}
	return payload, allItems, apperrors.NewAPI(
		fmt.Sprintf("@我消息分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
		apperrors.WithOperation("chat/search_at_me_message"),
		apperrors.WithReason("at_me_incomplete"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(true),
		apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
	)
}

func atMeCursorString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || text == "0" {
		return ""
	}
	return text
}

// atMeCompatibilityItems preserves the common list/items projection used by
// older Agent snippets while keeping messages as the canonical v1 contract.
// Its conversation field is an object so `.items[].conversation.name` is safe.
func atMeCompatibilityItems(messages []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		item := make(map[string]any, len(message))
		for key, value := range message {
			item[key] = value
		}
		conversation := map[string]any{}
		if name := atMeString(message["conversation"]); name != "" {
			conversation["name"] = name
		}
		if id := atMeString(message["conversationId"]); id != "" {
			conversation["openConversationId"] = id
		}
		item["conversation"] = conversation
		items = append(items, item)
	}
	return items
}

// atMeMessageItems locates the message list inside a search_at_me_message
// response, probing common container keys at the top level and nested under
// "result". Returns nil when no list is found.
func atMeMessageItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	// Preferred real shape (verified against the live gateway):
	//   {result:{conversationMessagesList:[{title,openConversationId,messages:[…]}]}}
	// The messages are nested two levels deep and grouped per conversation, so
	// flatten every group into a single message list while carrying the group's
	// conversation title/id down onto each message.
	for _, root := range []map[string]any{data, atMeChildMap(data, "result")} {
		if root == nil {
			continue
		}
		if groups, ok := root["conversationMessagesList"].([]any); ok {
			return atMeFlattenGroups(groups)
		}
	}
	keys := []string{"list", "messages", "messageList", "items", "data", "records", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return atMeToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "messages", "messageList", "items", "data", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return atMeToMaps(arr)
				}
			}
		}
	}
	return nil
}

// atMeChildMap returns data[key] as a map, or nil.
func atMeChildMap(data map[string]any, key string) map[string]any {
	if m, ok := data[key].(map[string]any); ok {
		return m
	}
	return nil
}

// atMeFlattenGroups flattens conversationMessagesList groups into a single
// message list, injecting the group's conversation title / id onto each message
// (when the message itself lacks them) so the projection can show a readable
// conversation.
func atMeFlattenGroups(groups []any) []map[string]any {
	var out []map[string]any
	for _, g := range groups {
		grp, ok := g.(map[string]any)
		if !ok {
			continue
		}
		title := atMeString(grp["title"])
		cid := atMeString(grp["openConversationId"])
		msgs, ok := grp["messages"].([]any)
		if !ok {
			continue
		}
		for _, mm := range msgs {
			m, ok := mm.(map[string]any)
			if !ok {
				continue
			}
			if title != "" {
				if _, has := m["conversationTitle"]; !has {
					m["conversationTitle"] = title
				}
			}
			if cid != "" {
				if _, has := m["openConversationId"]; !has {
					m["openConversationId"] = cid
				}
			}
			out = append(out, m)
		}
	}
	return out
}

func atMeToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// atMeProject reshapes one @me message into {sender, time, text, conversation},
// running text through the shared chatmsg cleaning (card/auto-reply JSON →
// readable, ciphertext → marker) and recursively expanding any forwarded chat
// record under "forwarded".
func atMeProject(m map[string]any) map[string]any {
	return atMeProjectWithReactions(m, true)
}

func atMeProjectWithReactions(m map[string]any, includeReactions bool) map[string]any {
	row := map[string]any{
		"sender":       atMeSender(m),
		"time":         atMeTime(m),
		"text":         atMeCleanText(m),
		"conversation": atMeConversation(m),
	}
	if messageID := chatmsg.MessageID(m); messageID != nil {
		row["messageId"] = messageID
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
		return atMeProjectWithReactions(item, includeReactions)
	}
	if forwarded := chatmsg.Forwarded(m, projectForwarded); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
}

// atMeCleanText runs atMeText's extraction through chatmsg.CleanText so
// card/auto-reply JSON and ciphertext render readable instead of leaking raw.
func atMeCleanText(m map[string]any) any {
	if s, ok := atMeText(m).(string); ok {
		return chatmsg.CleanText(s)
	}
	return atMeText(m)
}

// atMeSender reads a message's sender display name/id, tolerating the common
// sender keys the gateway may use (including a nested sender object). The literal
// string "null" (carried by forwarded sub-messages) and the empty string are
// both treated as absent so they never surface as the speaker.
func atMeSender(m map[string]any) any {
	norm := func(v any) string {
		if s := atMeString(v); s != "" && s != "null" {
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

// atMeTime reads a message's send time, returning the raw value (usually epoch
// millis) under whichever candidate key is present.
func atMeTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "time", "msgTime", "createAt"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// atMeText reads a message's textual content, tolerating flat text keys and a
// nested content/text object.
func atMeText(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "body"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"content", "text", "msg"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"text", "content", "richText", "title"} {
				if v := atMeString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// atMeConversation reads the conversation (chat) this message belongs to,
// preferring a readable title and falling back to the conversation id.
func atMeConversation(m map[string]any) any {
	for _, key := range []string{"conversationTitle", "chatTitle", "groupName", "conversationName", "title"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"openConversationId", "conversationId", "conversation_id", "cid", "chatId"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// atMeString coerces a scalar JSON value to a trimmed string, returning "" for
// nil / non-scalar / empty values.
func atMeString(v any) string {
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
	shortcut.Register(AtMe)
}
