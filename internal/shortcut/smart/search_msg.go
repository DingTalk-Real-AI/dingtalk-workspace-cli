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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// SearchMsg is the semantic message-search entry point. It exposes the native
// IM search dimensions, can exhaust cursor pagination, and enriches sparse
// search hits through list_messages_by_ids in chunks of 50. A later-page or
// enrichment failure never turns a partial result into a false success: the
// output carries an explicit failure ledger and complete=false.
const searchMsgIntent = "当你要按关键词、发送者、@对象、消息类型、机器人来源或会话范围组合搜索 IM 消息时使用；可搜索单个、多个或全部会话，会话与发送者过滤使用稳定 ID。默认查询近 7 天，也可指定精确起止时间和输出顺序。" +
	"显式指定会话时会先验证 CID，再执行有界全局扫描并在本地精确过滤，避免下层忽略非法 CID 或群聊 CID。" +
	"--page-all 会连续拉取游标页，默认再按消息 ID 分批富化详情；任何续页或富化失败都会保留已取得结果并返回逐项失败 ledger，绝不把截断结果标成完整。" +
	"--download-resources 使用安全本地路径、默认不覆盖和原子落盘。"

var SearchMsg = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+search-msg",
	Product:     "im",
	Description: "按稳定 ID、内容、时间等条件搜索消息，可校验会话范围、全量翻页并批量富化",
	Intent:      searchMsgIntent,
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_search_msg",
			CanonicalPath:  "chat.shortcut_search_msg",
			CLIPath:        "chat +search-msg",
			PrimaryCLIPath: "chat +search-msg",
		},
		Description: "按稳定 ID、内容、时间等条件搜索消息，可校验会话范围、全量翻页并批量富化",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed search adapter: it combines filters, cursor pagination, batched mget enrichment, stable projection, completeness accounting, and optional safe resource downloads.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按稳定 ID、内容、时间等条件搜索消息，可校验会话范围、全量翻页并批量富化",
			UseWhen:      []string{searchMsgIntent},
			AvoidWhen:    []string{"只想查看或导出一个指定会话的消息记录、且没有发送者、关键词、@对象或消息类型等主要筛选条件时使用 +chat-messages；已有精确消息 ID 时使用 +messages-mget"},
			Examples: []string{
				"dws chat +search-msg --query \"周报\" --senders <openDingTalkId> --days 3 --page-all",
				"dws chat +search-msg --query \"周报\" --senders <openDingTalkId> --days 3 --page-all --jq '.messages[] | {messageId, text}'",
			},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "--query 的兼容别名", Hidden: true},
		{Name: "text-query", Type: shortcut.FlagString, Desc: "--query 的兼容别名", Hidden: true},
		{Name: "group", Type: shortcut.FlagString, Desc: "单个会话 openConversationId"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "groups", Type: shortcut.FlagStringSlice, Desc: "多个会话 openConversationId"},
		{Name: "chat-id", Type: shortcut.FlagStringSlice, Desc: "--groups 的 lark-cli 对齐别名"},
		{Name: "chat-query", Type: shortcut.FlagStringSlice, Desc: "按群名唯一解析会话过滤条件（可选，可重复或逗号分隔）"},
		{Name: "senders", Type: shortcut.FlagStringSlice, Desc: "发送者 userId/openDingTalkId 列表"},
		{Name: "sender", Type: shortcut.FlagStringSlice, Desc: "--senders 的 lark-cli 对齐别名"},
		{Name: "sender-query", Type: shortcut.FlagStringSlice, Desc: "按姓名唯一解析发送者过滤条件（可选，可重复或逗号分隔）"},
		{Name: "at-me", Type: shortcut.FlagBool, Desc: "只搜索 @我 的消息"},
		{Name: "is-at-me", Type: shortcut.FlagBool, Desc: "--at-me 的 lark-cli 对齐别名"},
		{Name: "at-ids", Type: shortcut.FlagStringSlice, Desc: "@对象 userId/openDingTalkId 列表"},
		{Name: "message-type", Type: shortcut.FlagString, Desc: "下层消息类型过滤值（以当前 IM Schema 为准）"},
		{Name: "only-robot", Type: shortcut.FlagBool, Desc: "只搜索机器人消息"},
		{Name: "conversation-type", Type: shortcut.FlagString, Desc: "下层会话类型过滤值（以当前 IM Schema 为准）"},
		{Name: "chat-type", Type: shortcut.FlagString, Desc: "--conversation-type 的 lark-cli 对齐别名"},
		{Name: "days", Type: shortcut.FlagInt, Desc: "默认时间窗的回溯天数", Default: "7"},
		{Name: "start", Type: shortcut.FlagString, Desc: "精确开始时间（RFC3339，需与 --end/--end-time 一起传）"},
		{Name: "start-time", Type: shortcut.FlagString, Desc: "--start 的 lark-cli 对齐别名（RFC3339，需与 --end/--end-time 一起传）"},
		{Name: "end", Type: shortcut.FlagString, Desc: "精确结束时间（RFC3339，需与 --start/--start-time 一起传）"},
		{Name: "end-time", Type: shortcut.FlagString, Desc: "--end 的 lark-cli 对齐别名（RFC3339，需与 --start/--start-time 一起传）"},
		{Name: "order", Type: shortcut.FlagString, Enum: []string{"asc", "desc"}, Desc: "按消息创建时间稳定排列输出 asc/desc（可选，默认 desc）"},
		{Name: "sort", Type: shortcut.FlagString, Enum: []string{"asc", "desc"}, Desc: "--order 的 lark-cli 对齐别名（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量（1-100）", Default: "100"},
		{Name: "page-size", Type: shortcut.FlagInt, Desc: "--limit 的 lark-cli 对齐别名（1-100）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传上次的 nextCursor", Default: "0"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "--cursor 的 lark-cli 对齐别名"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动连续拉取所有游标页"},
		{Name: "page-limit", Type: shortcut.FlagInt, Desc: "--page-all 或显式会话范围本地扫描的最大页数（1-40）", Default: "20"},
		{Name: "no-enrich", Type: shortcut.FlagBool, Desc: "不再按消息 ID 批量查询完整详情"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出命中消息的 reaction（默认输出）"},
	}, chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintAtLeastOne,
			Flags:       []string{"query", "keyword", "text", "text-query", "group", "conversation-id", "id", "groups", "chat-id", "chat-query", "senders", "sender", "sender-query", "at-me", "is-at-me", "at-ids", "message-type", "only-robot", "conversation-type", "chat-type"},
			Description: "至少指定一个内容、身份、会话或消息类型过滤条件",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"start", "start-time"},
			Description: "需与 --end/--end-time 一起传",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"end", "end-time"},
			Description: "需与 --start/--start-time 一起传",
		},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"start", "start-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"end", "end-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"order", "sort"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"groups", "chat-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"senders", "sender"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"at-me", "is-at-me"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"conversation-type", "chat-type"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"query", "keyword", "text", "text-query"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"limit", "page-size"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"cursor", "page-token"}},
	}, chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +search-msg --group <openConversationId> --query "changefree"`,
		`dws chat +search-msg --senders <openDingTalkId> --at-me --days 3 --page-all`,
		`dws chat +search-msg --group <openConversationId> --query "changefree" --jq '.messages[] | {messageId, text}'`,
	},
	Validate: validateSearchMsgWithResources,
	Execute: func(rt *shortcut.RuntimeContext) error {
		params, resolvedFilters, err := searchMsgParams(rt)
		if err != nil {
			return err
		}
		requestedConversationIDs, _ := params["openConversationIds"].([]string)
		scopedSearch := len(requestedConversationIDs) > 0
		if scopedSearch {
			if err := validateSearchConversationScope(rt, requestedConversationIDs); err != nil {
				return err
			}
			// The downstream search currently drops invalid CID filters and does
			// not return group-scoped hits reliably. Scan the same filtered global
			// stream and apply the already-validated CID set locally instead.
			delete(params, "openConversationIds")
		}

		pageLimit := 1
		scanAllPages := rt.Bool("page-all") || scopedSearch
		if scanAllPages {
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
		paginationKnown := true

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
			pageMessages := searchMsgItems(data)
			if scopedSearch {
				var unverifiableMessageIDs []string
				pageMessages, unverifiableMessageIDs = chatmsg.FilterConversationScope(pageMessages, requestedConversationIDs)
				if len(unverifiableMessageIDs) > 0 {
					return searchScopeUnverifiedError(requestedConversationIDs, unverifiableMessageIDs)
				}
			}
			for _, message := range pageMessages {
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
					paginationKnown = false
					complete = false
					break
				}
			}
			hasMore = hasMoreValue
			if !scanAllPages || !hasMore {
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
		if scanAllPages && hasMore && pagesFetched == pageLimit {
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
		if scopedSearch {
			validatedMessages, unverifiableMessageIDs := chatmsg.FilterConversationScope(messages, requestedConversationIDs)
			if len(unverifiableMessageIDs) > 0 {
				return searchScopeUnverifiedError(requestedConversationIDs, unverifiableMessageIDs)
			}
			if len(validatedMessages) != len(messages) {
				return searchScopeViolationError(requestedConversationIDs, messages)
			}
			messages = validatedMessages
		}

		order := strings.ToLower(strings.TrimSpace(rt.StrFirst("order", "sort")))
		if order == "" {
			order = "desc"
		}
		sortMessagesByCreateTimeStable(messages, order)
		results := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			results = append(results, searchMsgProjectWithReactions(m, !rt.Bool("no-reactions")))
		}
		payload := map[string]any{
			"contractVersion": chatmsg.MessageListContractVersion,
			"count":           len(results),
			"messages":        results,
			"pagesFetched":    pagesFetched,
			"enrichedCount":   enrichedCount,
			"complete":        complete,
			"hasMore":         hasMore,
			"nextCursor":      "",
			"paginationKnown": paginationKnown,
			"failedCount":     len(failures),
			"failures":        failures,
			"queryRange":      searchMessageQueryRange(params, order),
		}
		if len(resolvedFilters.Senders) > 0 {
			payload["resolvedFilters"] = resolvedFilters
		}
		if scopedSearch {
			payload["scope"] = searchScopePayload(requestedConversationIDs, paginationKnown && !hasMore)
		}
		if hasMore && nextCursor != "" && nextCursor != "<nil>" {
			payload["nextCursor"] = nextCursor
		}
		if rt.Bool("download-resources") {
			chatshortcut.AttachMessageResourceDownloads(
				payload,
				chatshortcut.DownloadMessageResources(rt, messages, ""),
			)
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
	hasFilter := rt.StrFirst("query", "keyword", "text", "text-query", "group", "conversation-id", "id", "message-type", "conversation-type", "chat-type") != "" ||
		len(rt.StrSlice("groups")) > 0 ||
		len(rt.StrSlice("chat-id")) > 0 ||
		len(rt.StrSlice("chat-query")) > 0 ||
		len(rt.StrSlice("senders")) > 0 ||
		len(rt.StrSlice("sender")) > 0 ||
		len(rt.StrSlice("sender-query")) > 0 ||
		len(rt.StrSlice("at-ids")) > 0 ||
		rt.Bool("at-me") ||
		rt.Bool("is-at-me") ||
		rt.Bool("only-robot")
	if !hasFilter {
		return apperrors.NewValidation("至少指定一个过滤条件，例如 --query、--group、--senders、--at-me 或 --message-type")
	}
	startChanged := rt.Changed("start") || rt.Changed("start-time")
	endChanged := rt.Changed("end") || rt.Changed("end-time")
	if startChanged != endChanged {
		return apperrors.NewValidation("--start/--start-time 与 --end/--end-time 必须同时指定")
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

// searchResolvedFilters preserves the natural sender facts applied to the
// lower search. Message sender names are display labels and may differ from
// the directory name, so consumers must join the selected stable identity to
// each projected message's senderId instead of comparing those two names.
type searchResolvedFilters struct {
	Senders []targetresolver.UserResolution `json:"senders,omitempty"`
}

func searchMsgParams(rt *shortcut.RuntimeContext) (map[string]any, searchResolvedFilters, error) {
	params := map[string]any{"limit": rt.IntFirst("limit", "page-size")}
	resolvedFilters := searchResolvedFilters{}
	if value := rt.StrFirst("query", "keyword", "text", "text-query"); value != "" {
		params["keyword"] = value
	}
	conversationIDs := append([]string{}, rt.StrSlice("groups")...)
	conversationIDs = append(conversationIDs, rt.StrSlice("chat-id")...)
	if value := rt.StrFirst("group", "conversation-id", "id"); value != "" {
		conversationIDs = append(conversationIDs, value)
	}
	if queries := rt.StrSlice("chat-query"); len(queries) > 0 {
		for _, query := range queries {
			resolved, err := targetresolver.ResolveChatTarget(rt, "", query)
			if err != nil {
				return nil, searchResolvedFilters{}, err
			}
			conversationIDs = append(conversationIDs, resolved.Selected.OpenConversationID)
		}
	}
	if values := uniqueSearchStrings(conversationIDs); len(values) > 0 {
		params["openConversationIds"] = values
	}
	senders := append([]string{}, rt.StrSlice("senders")...)
	senders = append(senders, rt.StrSlice("sender")...)
	if queries := rt.StrSlice("sender-query"); len(queries) > 0 {
		resolvedUsers, err := targetresolver.ResolveUsers(rt, queries, targetresolver.IdentityAny)
		if err != nil {
			return nil, searchResolvedFilters{}, err
		}
		resolvedFilters.Senders = append(resolvedFilters.Senders, resolvedUsers...)
		for _, resolved := range resolvedUsers {
			identity := resolved.Selected.OpenDingTalkID
			if identity == "" {
				identity = resolved.Selected.UserID
			}
			senders = append(senders, identity)
		}
	}
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

	startValue := rt.StrFirst("start", "start-time")
	endValue := rt.StrFirst("end", "end-time")
	if startValue != "" && endValue != "" {
		start, err := time.Parse(time.RFC3339, startValue)
		if err != nil {
			return nil, searchResolvedFilters{}, apperrors.NewValidation(fmt.Sprintf("--start/--start-time 必须是 RFC3339 时间: %v", err))
		}
		end, err := time.Parse(time.RFC3339, endValue)
		if err != nil {
			return nil, searchResolvedFilters{}, apperrors.NewValidation(fmt.Sprintf("--end/--end-time 必须是 RFC3339 时间: %v", err))
		}
		if !end.After(start) {
			return nil, searchResolvedFilters{}, apperrors.NewValidation("--end 必须晚于 --start")
		}
		params["startTime"] = start.UnixMilli()
		params["endTime"] = end.UnixMilli()
	} else {
		now := time.Now()
		params["startTime"] = now.AddDate(0, 0, -rt.Int("days")).UnixMilli()
		params["endTime"] = now.UnixMilli()
	}
	return params, resolvedFilters, nil
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

func validateSearchConversationScope(rt *shortcut.RuntimeContext, conversationIDs []string) error {
	for _, conversationID := range conversationIDs {
		_, err := rt.CallMCPData("chat", "get_conversation_info", map[string]any{
			"openConversationId": conversationID,
		})
		if err == nil {
			continue
		}
		return helpers.NormalizeSearchConversationScopeError(conversationID, err)
	}
	return nil
}

func searchScopeUnverifiedError(conversationIDs, messageIDs []string) error {
	return apperrors.NewAPI(
		"搜索结果缺少 conversationId，无法证明会话过滤范围；已停止输出",
		apperrors.WithReason("search_conversation_scope_unverified"),
		apperrors.WithDetails(map[string]any{
			"requestedConversationIds": conversationIDs,
			"unverifiableMessageIds":   messageIDs,
		}),
		apperrors.WithRetryable(false),
		apperrors.WithHint("请保留 trace_id 并检查 IM 搜索服务是否返回 openConversationId"),
	)
}

func searchScopeViolationError(conversationIDs []string, messages []map[string]any) error {
	observed := make([]string, 0, len(messages))
	for _, message := range messages {
		conversationID := strings.TrimSpace(fmt.Sprint(chatmsg.ConversationID(message)))
		if conversationID == "" || conversationID == "<nil>" {
			continue
		}
		observed = append(observed, conversationID)
	}
	return apperrors.NewAPI(
		"消息富化结果超出请求的会话范围；已停止输出",
		apperrors.WithReason("search_conversation_scope_violation"),
		apperrors.WithDetails(map[string]any{
			"requestedConversationIds": conversationIDs,
			"observedConversationIds":  uniqueSearchStrings(observed),
		}),
		apperrors.WithRetryable(false),
	)
}

func searchScopePayload(conversationIDs []string, sourceComplete bool) map[string]any {
	return map[string]any{
		"requestedConversationIds": append([]string(nil), conversationIDs...),
		"targetsValidated":         true,
		"filterApplied":            true,
		"filterMode":               "client",
		"resultsWithinScope":       true,
		"sourceComplete":           sourceComplete,
	}
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
	return chatmsg.SearchItems(data)
}

// searchMsgProject adds search-only sender/time aliases and forwarded-message
// expansion to the shared message projection.
func searchMsgProject(m map[string]any) map[string]any {
	return searchMsgProjectWithReactions(m, true)
}

func searchMsgProjectWithReactions(m map[string]any, includeReactions bool) map[string]any {
	row := chatmsg.ProjectMessageV1(m, includeReactions)
	// Keep search-only aliases while messageId/text stay owned by the shared
	// projection used by typed message search.
	row["sender"] = searchMsgSender(m)
	row["time"] = searchMsgTime(m)
	if forwarded := chatmsg.Forwarded(m, func(item map[string]any) map[string]any {
		return searchMsgProjectWithReactions(item, includeReactions)
	}); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
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
			for _, nestedKey := range []string{"name", "nick", "userName", "staffName", "displayName"} {
				if v := norm(nested[nestedKey]); v != "" {
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

// searchMsgMessageID reads a message's identifier, tolerating the common id keys
// the gateway may use.
func searchMsgMessageID(m map[string]any) any {
	return chatmsg.MessageID(m)
}

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
