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
const searchMsgIntent = "当你要按关键词、发送者、@对象、消息类型、机器人来源或会话范围组合搜索 IM 消息时使用；可搜索单个、多个或全部会话。--group/--groups 接受群名或 openConversationId，--sender/--senders 接受姓名、userId 或 openDingTalkId：姓名优先唯一解析，稳定 ID 精确路由；通讯录无法分类时仍按原值 userId 执行并保留 identity_unverified，可交付精确命中但不能把原值升级为已验证身份或作完整否定结论。默认查询近 7 天，也可指定精确起止时间和输出顺序。" +
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
				"dws chat +search-msg --group \"项目群\" --sender \"测试用户甲\" --page-all",
				"dws chat +search-msg --query \"周报\" --senders <openDingTalkId> --days 3 --page-all",
			},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "--query 的兼容别名", Hidden: true},
		{Name: "text-query", Type: shortcut.FlagString, Desc: "--query 的兼容别名", Hidden: true},
		{Name: "group", Type: shortcut.FlagString, Desc: "单个群名或 openConversationId；自动唯一解析并校验"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "groups", Type: shortcut.FlagStringSlice, Desc: "多个群名或 openConversationId；可混合输入并逐项唯一解析"},
		{Name: "chat-id", Type: shortcut.FlagStringSlice, Desc: "--groups 的 lark-cli 对齐别名；只接受 openConversationId"},
		{Name: "chat-query", Type: shortcut.FlagStringSlice, Desc: "显式按群名唯一解析的兼容入口（可选，可重复或逗号分隔）"},
		{Name: "senders", Type: shortcut.FlagStringSlice, Desc: "多个发送者姓名、userId 或 openDingTalkId；姓名唯一解析，稳定 ID 精确路由，通讯录无法分类时按原值 userId 查询并保留身份未验证状态"},
		{Name: "sender", Type: shortcut.FlagStringSlice, Desc: "单个或多个发送者姓名、userId 或 openDingTalkId；--senders 的兼容别名，保留同样的三态解析与安全降级语义"},
		{Name: "sender-query", Type: shortcut.FlagStringSlice, Desc: "显式按姓名唯一解析的兼容入口（可选，可重复或逗号分隔）"},
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
		`dws chat +search-msg --group "项目群" --sender "测试用户甲" --page-all`,
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
		if len(resolvedFilters.Senders) > 0 {
			var unverifiableMessageIDs []string
			messages, unverifiableMessageIDs = filterSearchSenderScope(messages, resolvedFilters.Senders)
			if len(unverifiableMessageIDs) > 0 {
				return searchSenderScopeUnverifiedError(resolvedFilters.Senders, unverifiableMessageIDs)
			}
		}
		unverifiedSenderInputs := searchUnverifiedSenderInputs(resolvedFilters.Senders)
		if len(unverifiedSenderInputs) > 0 {
			failures = append(failures, map[string]any{
				"stage":  "sender_identity_verification",
				"inputs": unverifiedSenderInputs,
				"error":  "通讯录未能确认这些混合发送者参数是姓名还是 userId；已按精确 userId 执行，但不能据此作完整否定结论",
			})
			complete = false
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
			"timeCoverage":    searchMessageTimeCoverage(rt),
			"conclusionGuard": searchMessageConclusionGuard(rt, complete, len(results)),
		}
		if len(resolvedFilters.Chats) > 0 || len(resolvedFilters.Senders) > 0 {
			payload["resolvedFilters"] = resolvedFilters
		}
		if len(resolvedFilters.Senders) > 0 {
			payload["senderScope"] = map[string]any{
				"targetsResolved":    len(unverifiedSenderInputs) == 0,
				"filterApplied":      true,
				"filterMode":         "server_and_client",
				"resultsWithinScope": true,
			}
			if len(unverifiedSenderInputs) > 0 {
				payload["senderScope"].(map[string]any)["status"] = "identity_unverified"
				payload["senderScope"].(map[string]any)["unverifiedInputs"] = unverifiedSenderInputs
			}
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

func searchMessageTimeCoverage(rt *shortcut.RuntimeContext) map[string]any {
	source := "implicit_default"
	if rt.Changed("start") || rt.Changed("start-time") || rt.Changed("end") || rt.Changed("end-time") {
		source = "explicit_range"
	} else if rt.Changed("days") {
		source = "explicit_days"
	}

	coverage := map[string]any{
		"source":     source,
		"allHistory": false,
	}
	if source != "explicit_range" {
		coverage["days"] = rt.Int("days")
	}
	return coverage
}

func searchMessageConclusionGuard(rt *shortcut.RuntimeContext, complete bool, resultCount int) map[string]any {
	coverage := searchMessageTimeCoverage(rt)
	guard := map[string]any{
		"absenceWithinQueryRangeAllowed": complete,
		"absenceAcrossAllHistoryAllowed": false,
		"allHistoryCountAllowed":         false,
		"requiredActionForAllHistory":    "widen_time_range_or_reuse_complete_message_ledger",
	}
	if coverage["source"] == "implicit_default" && resultCount == 0 {
		guard["requiredAction"] = "widen_time_range_or_reuse_complete_message_ledger"
		guard["hint"] = "当前只证明默认 7 天时间窗内没有命中，不能据此判断全部历史没有。若已有完整会话消息，请解析稳定身份后复用现有消息；否则扩大时间范围。"
	}
	return guard
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
	if err := validateSearchMsgStrictConversationAliases(rt); err != nil {
		return err
	}
	return nil
}

// The conventional --group/--groups flags are intentionally dual-form. Only
// the explicit ID aliases remain strict so scripts that select them retain a
// deterministic contract.
func validateSearchMsgStrictConversationAliases(rt *shortcut.RuntimeContext) error {
	values := append([]string{}, rt.StrSlice("chat-id")...)
	if value := rt.StrFirst("conversation-id", "id"); value != "" {
		values = append(values, value)
	}
	for _, value := range uniqueSearchStrings(values) {
		if strings.HasPrefix(strings.ToLower(value), "cid") {
			continue
		}
		return apperrors.NewValidation(
			fmt.Sprintf("显式会话 ID 参数收到非 openConversationId 值 %q；已停止消息搜索", value),
			apperrors.WithReason("target_type_mismatch"),
			apperrors.WithOrigin("client"),
			apperrors.WithFailureStage("request_validation"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
			apperrors.WithHint(fmt.Sprintf("--chat-id/--conversation-id/--id 只接受 cid 开头的 openConversationId；群名请使用 --group %q 或 --groups %q。", value, value)),
			apperrors.WithDetails(map[string]any{
				"flags":         []string{"chat-id", "conversation-id", "id"},
				"expectedType":  "openConversationId",
				"providedValue": value,
			}),
		)
	}
	return nil
}

// searchResolvedFilters preserves the natural sender facts applied to the
// lower search. Message sender names are display labels and may differ from
// the directory name, so consumers must join the selected stable identity to
// each projected message's senderId instead of comparing those two names.
type searchResolvedFilters struct {
	Chats   []targetresolver.ChatResolution `json:"chats,omitempty"`
	Senders []targetresolver.UserResolution `json:"senders,omitempty"`
}

func searchMsgParams(rt *shortcut.RuntimeContext) (map[string]any, searchResolvedFilters, error) {
	params := map[string]any{"limit": rt.IntFirst("limit", "page-size")}
	resolvedFilters := searchResolvedFilters{}
	if value := rt.StrFirst("query", "keyword", "text", "text-query"); value != "" {
		params["keyword"] = value
	}
	conversationIDs := append([]string{}, rt.StrSlice("chat-id")...)
	if value := rt.StrFirst("conversation-id", "id"); value != "" {
		conversationIDs = append(conversationIDs, value)
	}
	groupTargets := append([]string{}, rt.StrSlice("groups")...)
	if value := rt.Str("group"); value != "" {
		groupTargets = append(groupTargets, value)
	}
	for _, target := range uniqueSearchStrings(groupTargets) {
		if strings.HasPrefix(strings.ToLower(target), "cid") {
			conversationIDs = append(conversationIDs, target)
			continue
		}
		resolved, err := targetresolver.ResolveChatTarget(rt, target, "")
		if err != nil {
			return nil, searchResolvedFilters{}, err
		}
		resolvedFilters.Chats = append(resolvedFilters.Chats, resolved)
		conversationIDs = append(conversationIDs, resolved.Selected.OpenConversationID)
	}
	if queries := rt.StrSlice("chat-query"); len(queries) > 0 {
		for _, query := range queries {
			resolved, err := targetresolver.ResolveChatTarget(rt, "", query)
			if err != nil {
				return nil, searchResolvedFilters{}, err
			}
			resolvedFilters.Chats = append(resolvedFilters.Chats, resolved)
			conversationIDs = append(conversationIDs, resolved.Selected.OpenConversationID)
		}
	}
	if values := uniqueSearchStrings(conversationIDs); len(values) > 0 {
		params["openConversationIds"] = values
	}
	senderTargets := append([]string{}, rt.StrSlice("senders")...)
	senderTargets = append(senderTargets, rt.StrSlice("sender")...)
	for _, target := range uniqueSearchStrings(senderTargets) {
		resolved, err := targetresolver.ResolveSenderTarget(rt, target, targetresolver.IdentityAny)
		if err != nil {
			return nil, searchResolvedFilters{}, err
		}
		resolvedFilters.Senders = append(resolvedFilters.Senders, resolved)
	}
	if queries := rt.StrSlice("sender-query"); len(queries) > 0 {
		resolvedUsers, err := targetresolver.ResolveUsers(rt, queries, targetresolver.IdentityAny)
		if err != nil {
			return nil, searchResolvedFilters{}, err
		}
		resolvedFilters.Senders = append(resolvedFilters.Senders, resolvedUsers...)
	}
	appendResolvedSearchActorIDs(params, resolvedFilters.Senders, "senderUserIds", "senderOpenDingTakIds")
	atUsers := resolveSearchStableActorTargets(rt, rt.StrSlice("at-ids"))
	appendResolvedSearchActorIDs(params, atUsers, "atUserIds", "atOpenDingTakIds")
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

func resolveSearchStableActorTargets(
	rt *shortcut.RuntimeContext,
	values []string,
) []targetresolver.UserResolution {
	resolvedUsers := make([]targetresolver.UserResolution, 0, len(values))
	for _, value := range uniqueSearchStrings(values) {
		// IdentityAny accepts every non-empty value as exactly one stable ID
		// family, and uniqueSearchStrings has already removed empty values.
		resolved, _ := targetresolver.ResolveStableUserTarget(rt, value, targetresolver.IdentityAny)
		resolvedUsers = append(resolvedUsers, resolved)
	}
	return resolvedUsers
}

// appendResolvedSearchActorIDs prefers openDingTalkId whenever directory
// resolution supplies it because the current message-search backend reliably
// applies the open-ID field but may ignore senderUserIds.
func appendResolvedSearchActorIDs(
	params map[string]any,
	resolutions []targetresolver.UserResolution,
	userKey, openIDKey string,
) {
	var userIDs, openIDs []string
	for _, resolved := range resolutions {
		openID := strings.TrimSpace(resolved.Selected.OpenDingTalkID)
		userID := strings.TrimSpace(resolved.Selected.UserID)
		if openID != "" {
			openIDs = append(openIDs, openID)
		} else if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	userIDs = uniqueSearchStrings(userIDs)
	openIDs = uniqueSearchStrings(openIDs)
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

func filterSearchSenderScope(
	messages []map[string]any,
	resolutions []targetresolver.UserResolution,
) ([]map[string]any, []string) {
	allowed := map[string]bool{}
	for _, resolution := range resolutions {
		if value := strings.TrimSpace(resolution.Selected.UserID); value != "" {
			allowed[value] = true
		}
		if value := strings.TrimSpace(resolution.Selected.OpenDingTalkID); value != "" {
			allowed[value] = true
		}
	}

	filtered := make([]map[string]any, 0, len(messages))
	unverifiable := make([]string, 0)
	for _, message := range messages {
		senderID := strings.TrimSpace(fmt.Sprint(chatmsg.SenderID(message)))
		if senderID == "" || senderID == "<nil>" {
			messageID := strings.TrimSpace(fmt.Sprint(searchMsgMessageID(message)))
			if messageID == "" || messageID == "<nil>" {
				messageID = "<unknown>"
			}
			unverifiable = append(unverifiable, messageID)
			continue
		}
		if allowed[senderID] {
			filtered = append(filtered, message)
		}
	}
	return filtered, uniqueSearchStrings(unverifiable)
}

func searchUnverifiedSenderInputs(
	resolutions []targetresolver.UserResolution,
) []string {
	values := make([]string, 0)
	for _, resolution := range resolutions {
		if targetresolver.IsUnverifiedUserIDResolution(resolution) {
			values = append(values, resolution.Query)
		}
	}
	return uniqueSearchStrings(values)
}

func searchSenderScopeUnverifiedError(
	resolutions []targetresolver.UserResolution,
	messageIDs []string,
) error {
	queries := make([]string, 0, len(resolutions))
	for _, resolution := range resolutions {
		queries = append(queries, resolution.Query)
	}
	return apperrors.NewAPI(
		"搜索结果缺少 senderId，无法证明发送者过滤范围；已停止输出",
		apperrors.WithReason("search_sender_scope_unverified"),
		apperrors.WithDetails(map[string]any{
			"requestedSenders":       uniqueSearchStrings(queries),
			"unverifiableMessageIds": messageIDs,
		}),
		apperrors.WithRetryable(false),
		apperrors.WithHint("请保留 trace_id 并检查 IM 搜索服务是否返回稳定 senderId"),
	)
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
