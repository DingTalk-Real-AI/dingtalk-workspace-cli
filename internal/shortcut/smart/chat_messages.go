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
	"errors"
	"fmt"
	"math"
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

var dingTalkMessageLocation = time.FixedZone("CST", 8*60*60)

const (
	chatMessagesDefaultPageLimit = 50
	chatMessagesHardPageLimit    = 500
	chatMessagesAllPageSize      = 100
)

func formatDingTalkMessageBoundary(now time.Time) string {
	localized := now.In(dingTalkMessageLocation)
	if localized.Nanosecond() != 0 {
		return localized.Format(time.RFC3339Nano)
	}
	return localized.Format("2006-01-02 15:04:05")
}

// ChatMessages resolves one conversation, projects messages into the shared
// typed result contract, and optionally follows bounded continuation pages,
// downloads resources, or atomically exports the complete ledger as JSON.
//
//	dws chat +chat-messages --group <openconversation_id> --time "2025-03-01 00:00:00"
//	dws chat +chat-messages --user <userId> --time "2025-03-01 00:00:00" --limit 50
var ChatMessages = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-messages",
	Product:     "chat",
	Description: "读取指定群聊或单聊的消息记录，支持有界全量分页与原子 JSON 导出",
	Intent: "当你要读取或导出一个指定群聊或单聊的消息记录时使用；可附带发送者姓名解析，无稳定身份时保留全部消息，唯一解析出稳定身份后按 senderId 筛选同一次读取结果；" +
		"群聊的 --group 可传群名或 openConversationId，单聊可传 --user 或 --open-dingtalk-id，所有目标参数互斥且必须选一个。自然群名只在唯一解析后读取，多候选会返回结构化 candidates。" +
		"省略时间参数时默认从当前时间向前读取最近消息；兼容模式可用 --time/--direction，范围模式可用公开可选的 --start/--end/--order（兼容 --start-time/--end-time/--sort），范围语义为 [start,end)。" +
		"全量读取用 --page-all，并由 --page-limit/--max-results 保持有界；结果公开 complete、hasMore、nextPage、stopReason、截断和逐页失败，不能把部分结果称为完整。--output 把同一 ledger 原子写为工作目录内 JSON。" +
		"默认只读；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_messages",
			CanonicalPath:  "chat.shortcut_chat_messages",
			CLIPath:        "chat +chat-messages",
			PrimaryCLIPath: "chat +chat-messages",
		},
		Description: "读取指定群聊或单聊的消息记录，支持有界全量分页与原子 JSON 导出",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in Shortcut adapter: it routes group or direct-message history reads, projects a stable message shape, and optionally orchestrates safe resource downloads with a failure ledger.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "读取指定群聊或单聊的消息记录，支持有界全量分页与原子 JSON 导出",
			UseWhen: []string{"当你要读取或导出一个指定群聊或单聊的消息记录时使用；可附带发送者姓名解析，无稳定身份时保留全部消息，唯一解析出稳定身份后按 senderId 筛选同一次读取结果；" +
				"群聊的 --group 可传群名或 openConversationId，单聊可传 --user 或 --open-dingtalk-id，所有目标参数互斥且必须选一个。自然群名只在唯一解析后读取，多候选会返回结构化 candidates。" +
				"省略时间参数时默认从当前时间向前读取最近消息；兼容模式可用 --time/--direction，范围模式可用公开可选的 --start/--end/--order（兼容 --start-time/--end-time/--sort），范围语义为 [start,end)。" +
				"全量读取用 --page-all，并由 --page-limit/--max-results 保持有界；结果公开 complete、hasMore、nextPage、stopReason、截断和逐页失败，不能把部分结果称为完整。--output 把同一 ledger 原子写为工作目录内 JSON。" +
				"默认只读；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘。"},
			AvoidWhen: []string{"以发送者、关键词、@对象或消息类型为主的直接条件检索优先使用 +search-msg；已有一批精确消息 ID 时使用 +messages-mget。已选择会话读取时可在同一次调用附带发送者姓名，不需要再搜索消息"},
			Examples: []string{
				"dws chat +chat-messages --group <openConversationId> --direction older",
				"dws chat +chat-messages --group <openConversationId> --direction older --jq '.messages[] | {messageId, text}'",
			},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称或 openConversationId，与单聊目标互斥"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "按群名唯一解析目标会话（可选，与其他会话目标参数互斥）"},
		{Name: "user", Type: shortcut.FlagString, Desc: "单聊对方的 userId，与 --group 互斥"},
		{Name: "user-query", Type: shortcut.FlagString, Desc: "按姓名解析唯一 openDingTalkId 的兼容入口", Hidden: true},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "单聊对方的 openDingTalkId，与 --group/--user 互斥"},
		{Name: "sender-query", Type: shortcut.FlagStringSlice, Desc: "按姓名唯一解析发送者并筛选同一次会话读取结果（可选，可重复或逗号分隔）"},
		{Name: "time", Type: shortcut.FlagString, Desc: "时间边界，如 \"2025-03-01 00:00:00\"；--time 必须是 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD；省略时从当前时间向前读取最近消息"},
		{Name: "start", Type: shortcut.FlagString, Desc: "范围开始时间（可选、包含），支持 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD"},
		{Name: "start-time", Type: shortcut.FlagString, Desc: "--start 的 lark-cli 对齐别名（可选、包含）"},
		{Name: "end", Type: shortcut.FlagString, Desc: "范围结束时间（可选、不包含）；仅传开始时间时默认为当前时间"},
		{Name: "end-time", Type: shortcut.FlagString, Desc: "--end 的 lark-cli 对齐别名（可选、不包含）"},
		{Name: "order", Type: shortcut.FlagString, Enum: []string{"asc", "desc"}, Desc: "结果及范围遍历顺序 asc/desc（可选，默认 desc；asc 必须指定 --start/--start-time）"},
		{Name: "sort", Type: shortcut.FlagString, Enum: []string{"asc", "desc"}, Desc: "--order 的 lark-cli 对齐别名（可选；asc 必须指定 --start/--start-time）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页拉取的消息条数；显式页大小必须大于 0"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名", Hidden: true},
		{Name: "page-size", Type: shortcut.FlagInt, Desc: "--limit 的兼容别名", Hidden: true},
		{Name: "direction", Type: shortcut.FlagString, Enum: []string{"newer", "older"}, Desc: "时间方向 newer/older；省略时为 older，从时间边界向前读取"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "沿 typed nextPage.time 自动读取后续页；--page-limit 仅与 --page-all 一起使用且范围 1-500；--max-results 仅与 --page-all 一起使用且不能为负数"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "max-results", Type: shortcut.FlagInt, Desc: "--max-results 仅与 --page-all 一起使用且不能为负数；0 表示仅受页数上限约束"},
		{Name: "output", Shorthand: "o", Type: shortcut.FlagString, Desc: "把完整结构化 ledger 原子写入工作目录内的相对 JSON 文件"},
	}, chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "conversation-id", "id", "open-conversation-id", "chat-query", "user", "user-query", "open-dingtalk-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"limit", "size", "page-size"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"start", "start-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"end", "end-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"order", "sort"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"time", "start", "start-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"time", "end", "end-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"time", "order", "sort"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"direction", "start", "start-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"direction", "end", "end-time"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"direction", "order", "sort"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"time"}, Description: "--time 必须是 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"order", "sort"}, Description: "asc 必须指定 --start/--start-time"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "显式页大小必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "max-results"}, Description: "--max-results 仅与 --page-all 一起使用且不能为负数"},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"output", "overwrite"},
			Description: "--output 必须是工作目录内的相对 JSON 文件；默认不覆盖，--overwrite 仅与 --output 一起使用",
		},
	}, chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +chat-messages --group <openconversation_id> --time "2025-03-01 00:00:00"`,
		`dws chat +chat-messages --group <openconversation_id> --start "2025-03-01T00:00:00+08:00" --end "2025-03-02T00:00:00+08:00" --order asc --page-all`,
		`dws chat +chat-messages --user <userId> --time "2025-03-01 00:00:00" --page-all --page-limit 50`,
		`dws chat +chat-messages --group <openconversation_id> --direction older --page-all --output ./exports/messages.json`,
		`dws chat +chat-messages --group <openconversation_id> --direction older --jq '.messages[] | {messageId, text}'`,
	},
	Validate: validateChatMessages,
	Execute:  executeChatMessages,
}

func validateChatMessages(rt *shortcut.RuntimeContext) error {
	if err := chatshortcut.ValidateMessageResourceDownload(rt); err != nil {
		return err
	}
	if rt.Changed("time") && strings.TrimSpace(rt.Str("time")) != "" && !validChatTime(rt.Str("time")) {
		return localChatOptionError("invalid_time_boundary", "+chat-messages 的 --time 格式无效", "--time")
	}
	rangeFlagsChanged := rt.Changed("start") || rt.Changed("start-time") || rt.Changed("end") ||
		rt.Changed("end-time") || rt.Changed("order") || rt.Changed("sort")
	if rt.Changed("time") && rangeFlagsChanged {
		return apperrors.NewValidation("--time 兼容边界模式不能与 --start/--end/--order 范围模式混用")
	}
	if rt.Changed("direction") && rangeFlagsChanged {
		return apperrors.NewValidation("--direction 兼容方向不能与 --start/--end/--order 范围模式混用")
	}
	if _, err := resolveChatMessageTimeRange(rt, time.Now()); err != nil {
		return err
	}
	for _, name := range []string{"limit", "size", "page-size"} {
		if rt.Changed(name) && rt.Int(name) <= 0 {
			return localChatOptionError("invalid_page_size", "+chat-messages 的 --"+name+" 必须大于 0", "--"+name)
		}
	}
	if !rt.Bool("page-all") && (rt.Changed("page-limit") || rt.Changed("max-results")) {
		return apperrors.NewValidation("--page-limit/--max-results 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if pageLimit := rt.Int("page-limit"); pageLimit < 1 || pageLimit > chatMessagesHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
		if rt.Int("max-results") < 0 {
			return apperrors.NewValidation("--max-results 不能小于 0")
		}
	}
	if rt.Changed("output") {
		if err := chatshortcut.ValidateMessageExportOutput(rt.Str("output")); err != nil {
			return err
		}
	} else if rt.Bool("overwrite") {
		return apperrors.NewValidation("--overwrite 仅与 --output 一起使用")
	}
	return nil
}

type chatMessagesRequest struct {
	tool                   string
	params                 map[string]any
	direction              string
	fallbackConversationID string
	timeRange              chatMessageTimeRange
}

type chatMessagesSenderFilter struct {
	requested   bool
	applied     bool
	stableIDs   map[string]bool
	resolutions []targetresolver.UserResolution
	failure     map[string]any
}

// resolveOptionalChatMessagesSenderFilter is deliberately local to
// +chat-messages. Sender resolution is an optional post-read filter here: an
// unresolved name must not suppress the command's primary conversation-list
// result. +search-msg keeps its stricter semantics because sender identity is
// part of the lower search request there.
func resolveOptionalChatMessagesSenderFilter(rt *shortcut.RuntimeContext) chatMessagesSenderFilter {
	queries := rt.StrSlice("sender-query")
	filter := chatMessagesSenderFilter{
		requested: len(queries) > 0,
		stableIDs: map[string]bool{},
	}
	if !filter.requested {
		return filter
	}

	resolutions, err := targetresolver.ResolveUsers(rt, queries, targetresolver.IdentityAny)
	if err != nil {
		failure := map[string]any{
			"stage":   "sender_resolution",
			"queries": queries,
			"error":   err.Error(),
		}
		var typed *apperrors.Error
		if errors.As(err, &typed) {
			if typed.Reason != "" {
				failure["reason"] = typed.Reason
			}
			if len(typed.Details) > 0 {
				failure["details"] = typed.Details
			}
		}
		filter.failure = failure
		return filter
	}

	filter.resolutions = resolutions
	for _, resolution := range resolutions {
		for _, identity := range []string{
			resolution.Selected.UserID,
			resolution.Selected.OpenDingTalkID,
		} {
			if identity = strings.TrimSpace(identity); identity != "" {
				filter.stableIDs[identity] = true
			}
		}
	}
	// ResolveUsers(IdentityAny) only returns users extracted with at least one
	// stable userId/openDingTalkId and fails when no resolvable user remains.
	filter.applied = true
	return filter
}

// applyOptionalChatMessagesSenderFilter preserves the unfiltered message list
// when optional resolution fails. Successful resolution filters both the
// public projection and raw rows so exports and resource downloads cannot
// accidentally include messages outside the requested sender set.
func applyOptionalChatMessagesSenderFilter(
	rt *shortcut.RuntimeContext,
	payload map[string]any,
	rawItems []map[string]any,
	filter chatMessagesSenderFilter,
) []map[string]any {
	if !filter.requested || payload == nil {
		return rawItems
	}
	if !filter.applied {
		failures, _ := payload["failures"].([]map[string]any)
		priorFailures := len(failures)
		failures = append(failures, filter.failure)
		payload["failures"] = failures
		payload["failedCount"] = len(failures)
		payload["complete"] = false
		payload["partial"] = len(rawItems) > 0
		if priorFailures == 0 {
			payload["stopReason"] = "sender_resolution_failed"
		}
		return rawItems
	}

	filtered := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		identity := strings.TrimSpace(fmt.Sprint(chatmsg.SenderID(item)))
		if identity != "" && identity != "<nil>" && filter.stableIDs[identity] {
			filtered = append(filtered, item)
		}
	}
	payload["messages"] = projectChatMessages(filtered, !rt.Bool("no-reactions"))
	payload["count"] = len(filtered)
	payload["resolvedFilters"] = map[string]any{"senders": filter.resolutions}
	return filtered
}

func resolveChatMessagesRequest(rt *shortcut.RuntimeContext) (chatMessagesRequest, error) {
	groupID := strings.TrimSpace(rt.StrFirst("conversation-id", "id", "open-conversation-id"))
	userID := rt.Str("user")
	openID := rt.Str("open-dingtalk-id")
	if targetresolver.LooksLikeOpenConversationID(openID) {
		return chatMessagesRequest{}, apperrors.NewValidation(
			"--open-dingtalk-id 收到的是群 openConversationId；群聊请改用 --group（兼容别名 --chat）",
		)
	}
	if groupID == "" && (rt.Str("group") != "" || rt.Str("chat-query") != "") {
		resolved, err := targetresolver.ResolveChatTarget(rt, rt.Str("group"), rt.Str("chat-query"))
		if err != nil {
			return chatMessagesRequest{}, err
		}
		groupID = resolved.Selected.OpenConversationID
	}
	if query := rt.Str("user-query"); query != "" {
		resolved, err := targetresolver.ResolveUser(rt, query, targetresolver.IdentityOpenDingTalkID)
		if err != nil {
			return chatMessagesRequest{}, err
		}
		openID = resolved.Selected.OpenDingTalkID
	}

	now := time.Now()
	timeRange, err := resolveChatMessageTimeRange(rt, now)
	if err != nil {
		return chatMessagesRequest{}, err
	}
	direction := strings.TrimSpace(strings.ToLower(rt.Str("direction")))
	if direction == "" {
		direction = timeRange.direction()
	}
	params := map[string]any{
		"time":    timeRange.initialBoundary(now),
		"forward": direction == "newer",
	}
	if rt.Changed("time") && rt.Str("time") != "" {
		params["time"] = rt.Str("time")
	}
	if limit := rt.IntFirst("limit", "size", "page-size"); limit > 0 {
		params["limit"] = limit
	} else if rt.Bool("page-all") {
		params["limit"] = chatMessagesAllPageSize
	}

	request := chatMessagesRequest{params: params, direction: direction, timeRange: timeRange}
	switch {
	case groupID != "":
		request.tool = "list_conversation_message_v2"
		request.params["openconversation_id"] = groupID
		request.fallbackConversationID = groupID
	case openID != "":
		request.tool = "list_individual_chat_message"
		request.params["openDingTalkId"] = openID
	default:
		request.tool = "list_individual_chat_message"
		request.params["userId"] = userID
	}
	return request, nil
}

func executeChatMessages(rt *shortcut.RuntimeContext) error {
	request, err := resolveChatMessagesRequest(rt)
	if err != nil {
		return err
	}
	var payload map[string]any
	var rawItems []map[string]any
	if rt.Bool("page-all") {
		payload, rawItems, err = collectAllChatMessages(rt, request)
	} else {
		payload, rawItems, err = collectOneChatMessagesPage(rt, request)
	}
	if err != nil && (payload == nil || payload["pagesFetched"] == 0) {
		// A sender name is only an optional post-read filter. If the primary
		// message read never produced a page, do not make a misleading and
		// unnecessary directory request before returning the read failure.
		if payload != nil {
			if outputErr := rt.Output(payload); outputErr != nil {
				return outputErr
			}
		}
		return err
	}
	senderFilter := resolveOptionalChatMessagesSenderFilter(rt)
	rawItems = applyOptionalChatMessagesSenderFilter(rt, payload, rawItems, senderFilter)
	if err != nil {
		// Full-page collection returns its failure ledger together with a
		// non-zero error. Publish that ledger for diagnosis, but stop before
		// resource downloads or a requested export can look successful.
		if payload != nil {
			if outputErr := rt.Output(payload); outputErr != nil {
				return outputErr
			}
		}
		return err
	}
	if rt.Bool("download-resources") {
		chatshortcut.AttachMessageResourceDownloads(
			payload,
			chatshortcut.DownloadMessageResources(rt, rawItems, request.fallbackConversationID),
		)
	}
	if rt.Changed("output") {
		if rt.DryRun() {
			payload["export"] = map[string]any{
				"dryRun":    true,
				"format":    "json",
				"localPath": rt.Str("output"),
				"overwrite": rt.Bool("overwrite"),
			}
		} else {
			path, size, writeErr := chatshortcut.WriteMessageExportJSON(
				rt.Str("output"), rt.Bool("overwrite"), payload)
			if writeErr != nil {
				return writeErr
			}
			payload["export"] = map[string]any{
				"format":    "json",
				"localPath": path,
				"sizeBytes": size,
			}
		}
	}
	return rt.Output(payload)
}

func collectOneChatMessagesPage(rt *shortcut.RuntimeContext, request chatMessagesRequest) (map[string]any, []map[string]any, error) {
	data, err := rt.CallMCPData("chat", request.tool, request.params)
	if err != nil {
		return nil, nil, err
	}
	rawItems := chatMessageItems(data)
	items, terminalReached, rangeFailures := request.timeRange.filter(rawItems)
	sortMessagesByCreateTimeStable(items, request.timeRange.order)
	results := projectChatMessages(items, !rt.Bool("no-reactions"))
	payload := chatmsg.NewMessageListPayload(results)
	chatmsg.ApplyMessagePagination(payload, data, rawItems, request.direction)
	if metadata := request.timeRange.metadata(); metadata != nil {
		payload["queryRange"] = metadata
	}
	if len(rangeFailures) > 0 {
		failures, _ := payload["failures"].([]map[string]any)
		failures = append(failures, rangeFailures...)
		payload["failures"] = failures
		payload["failedCount"] = len(failures)
		payload["complete"] = false
		payload["partial"] = len(items) > 0
		payload["stopReason"] = "time_filter_error"
		return payload, items, nil
	}
	if terminalReached {
		payload["complete"] = true
		payload["hasMore"] = false
		payload["stopReason"] = request.timeRange.stopReason()
		delete(payload, "nextPage")
		return payload, items, nil
	}
	if payload["complete"] == true {
		payload["stopReason"] = "source_complete"
	} else {
		payload["stopReason"] = "single_page"
	}
	return payload, items, nil
}

func collectAllChatMessages(rt *shortcut.RuntimeContext, request chatMessagesRequest) (map[string]any, []map[string]any, error) {
	pageLimit := defaultChatPageLimit(rt.Int("page-limit"), chatMessagesDefaultPageLimit)
	maxResults := rt.Int("max-results")
	basePageSize, _ := request.params["limit"].(int)
	if basePageSize <= 0 {
		basePageSize = chatMessagesAllPageSize
	}
	seenIDs := map[string]bool{}
	seenCursors := map[string]bool{}
	allItems := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	paginationKnown := true
	complete := false
	hasMore := false
	stopReason := "source_complete"
	truncatedByPageLimit := false
	truncatedByResultLimit := false
	var nextPage map[string]any

	for pagesFetched < pageLimit {
		request.params["limit"] = basePageSize
		if maxResults > 0 {
			remaining := maxResults - len(allItems)
			if remaining < basePageSize {
				request.params["limit"] = remaining
			}
		}
		data, err := rt.CallMCPData("chat", request.tool, request.params)
		if err != nil {
			failures = append(failures, map[string]any{
				"page":  pagesFetched + 1,
				"stage": "read",
				"error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		rawItems := chatMessageItems(data)
		items, terminalReached, rangeFailures := request.timeRange.filter(rawItems)
		failures = append(failures, rangeFailures...)
		moreEligibleOnPage := false
		for _, item := range items {
			stableID := chatmsg.StableMessageID(item)
			if stableID != "" && seenIDs[stableID] {
				continue
			}
			if stableID != "" {
				seenIDs[stableID] = true
			}
			if maxResults > 0 && len(allItems) >= maxResults {
				moreEligibleOnPage = true
				continue
			}
			allItems = append(allItems, item)
		}
		if len(rangeFailures) > 0 {
			stopReason = "time_filter_error"
			break
		}
		if terminalReached {
			if maxResults > 0 && moreEligibleOnPage {
				truncatedByResultLimit = true
				hasMore = true
				stopReason = "result_limit"
			} else {
				complete = true
				hasMore = false
				stopReason = request.timeRange.stopReason()
			}
			break
		}

		page := chatmsg.Pagination(data)
		pageHasMore, hasMoreKnown := page["hasMore"].(bool)
		if !hasMoreKnown {
			paginationKnown = false
			failures = append(failures, map[string]any{
				"page":  pagesFetched,
				"stage": "pagination",
				"error": "下层未返回可靠的 hasMore，无法证明全量结果完整",
			})
			stopReason = "pagination_error"
			break
		}
		hasMore = pageHasMore

		if maxResults > 0 && len(allItems) >= maxResults {
			truncatedByResultLimit = pageHasMore || moreEligibleOnPage
			if truncatedByResultLimit {
				hasMore = true
				stopReason = "result_limit"
				if moreEligibleOnPage {
					failures = append(failures, map[string]any{
						"page":  pagesFetched,
						"stage": "pagination",
						"error": "达到 --max-results 时当前下层页仍有未返回消息，无法生成不跳项的安全续页游标",
					})
					stopReason = "pagination_error"
					break
				}
				cursorKey, boundary, cursorErr := chatMessagesNextCursorBoundary(page["nextCursor"])
				if cursorErr != nil {
					failures = append(failures, map[string]any{
						"page":  pagesFetched,
						"stage": "pagination",
						"error": "达到结果上限但 nextCursor 无效，无法安全续页: " + cursorErr.Error(),
					})
					stopReason = "pagination_error"
					break
				}
				seenCursors[cursorKey] = true
				nextPage = map[string]any{
					"direction":  request.direction,
					"time":       boundary,
					"nextCursor": page["nextCursor"],
				}
				break
			}
		}
		if !pageHasMore {
			complete = true
			hasMore = false
			stopReason = "source_complete"
			break
		}
		if len(rawItems) == 0 {
			failures = append(failures, map[string]any{
				"page":  pagesFetched,
				"stage": "pagination",
				"error": "下层返回 hasMore=true 但当前页没有消息",
			})
			stopReason = "pagination_error"
			break
		}
		cursorKey, boundary, cursorErr := chatMessagesNextCursorBoundary(page["nextCursor"])
		if cursorErr != nil {
			failures = append(failures, map[string]any{
				"page":  pagesFetched,
				"stage": "pagination",
				"error": "hasMore=true 但 nextCursor 无效，无法安全续页: " + cursorErr.Error(),
			})
			stopReason = "pagination_error"
			break
		}
		if seenCursors[cursorKey] {
			failures = append(failures, map[string]any{
				"page":  pagesFetched,
				"stage": "pagination",
				"error": "hasMore=true 但毫秒 nextCursor 停滞",
			})
			stopReason = "pagination_error"
			break
		}
		seenCursors[cursorKey] = true
		nextPage = map[string]any{
			"direction":  request.direction,
			"time":       boundary,
			"nextCursor": page["nextCursor"],
		}
		request.params["time"] = boundary
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	sortMessagesByCreateTimeStable(allItems, request.timeRange.order)
	results := projectChatMessages(allItems, !rt.Bool("no-reactions"))
	payload := chatmsg.NewMessageListPayload(results)
	if metadata := request.timeRange.metadata(); metadata != nil {
		payload["queryRange"] = metadata
	}
	payload["pagesFetched"] = pagesFetched
	payload["paginationKnown"] = paginationKnown
	payload["complete"] = complete && len(failures) == 0
	payload["hasMore"] = hasMore
	payload["stopReason"] = stopReason
	payload["truncatedByPageLimit"] = truncatedByPageLimit
	payload["truncatedByResultLimit"] = truncatedByResultLimit
	payload["failedCount"] = len(failures)
	payload["failures"] = failures
	payload["partial"] = len(failures) > 0 && len(results) > 0
	if hasMore && nextPage != nil {
		payload["nextPage"] = nextPage
	}
	if len(failures) > 0 {
		failureStage := "pagination"
		if stopReason == "read_failure" {
			failureStage = "read"
		} else if stopReason == "time_filter_error" {
			failureStage = "time_filter"
		}
		return payload, allItems, apperrors.NewAPI(
			fmt.Sprintf("全量消息读取未完成：%d 页成功，%d 个页面失败", pagesFetched, len(failures)),
			apperrors.WithOperation("chat/"+request.tool),
			apperrors.WithReason("chat_messages_incomplete"),
			apperrors.WithOrigin("mcp_gateway"),
			apperrors.WithFailureStage(failureStage),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(true),
			apperrors.WithHint("请根据 failures 和 nextPage 重试；失败 ledger 不会写入 --output 文件"),
			apperrors.WithDetails(map[string]any{
				"pagesFetched": pagesFetched,
				"failedCount":  len(failures),
				"failures":     failures,
				"partial":      len(results) > 0,
				"stopReason":   stopReason,
			}),
		)
	}
	return payload, allItems, nil
}

func projectChatMessages(items []map[string]any, includeReactions bool) []map[string]any {
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, projectChatMessageWithReactions(item, includeReactions))
	}
	return results
}

// chatMessagesNextCursorBoundary converts the authoritative millisecond
// cursor returned by DingTalk message-list tools into the exact RFC3339Nano
// boundary accepted by their time parameter. Projected createTime is only
// second precision and must never drive pagination: doing so skips messages
// when a page boundary splits several messages created within the same second.
func chatMessagesNextCursorBoundary(value any) (string, string, error) {
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
	cursorKey := strconv.FormatInt(millis, 10)
	boundary := time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
	return cursorKey, boundary, nil
}

// chatMessageItems defensively unwraps the message list from the response,
// tolerating the common container keys and one level of nesting under a
// "result"/"data" wrapper.
func chatMessageItems(data map[string]any) []map[string]any {
	return chatmsg.ListMessageItems(data)
}

// projectChatMessage reshapes one raw message into the clean
// {sender, text, createTime} projection, rendering card/auto-reply JSON and
// marking encrypted messages via chatmsg, and recursively expanding forwarded
// chat records under "forwarded".
func projectChatMessage(m map[string]any) map[string]any {
	return projectChatMessageWithReactions(m, true)
}

func projectChatMessageWithReactions(m map[string]any, includeReactions bool) map[string]any {
	return chatmsg.ProjectMessageV1(m, includeReactions)
}

func init() {
	shortcut.Register(ChatMessages)
}
