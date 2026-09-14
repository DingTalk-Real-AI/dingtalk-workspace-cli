package helpers

import (
	"fmt"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/spf13/cobra"
)

// runChatMessageListPageAll mirrors the single-page RunE argument assembly
// (mutual exclusion, default time, direction resolution, per-page limit) and
// hands the request to the time-boundary sweep instead. The single-page path
// stays byte-identical in the command's RunE.
func runChatMessageListPageAll(cmd *cobra.Command, opts pagedCommandOptions) error {
	groupID := flagOrFallback(cmd, "conversation-id", "group", "id", "chat")
	userID, _ := cmd.Flags().GetString("user")
	openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
	specified := 0
	if groupID != "" {
		specified++
	}
	if userID != "" {
		specified++
	}
	if openDingTalkID != "" {
		specified++
	}
	if specified > 1 {
		return fmt.Errorf("--conversation-id, --user and --open-dingtalk-id are mutually exclusive, specify exactly one")
	}
	if specified == 0 {
		return fmt.Errorf("--conversation-id, --user or --open-dingtalk-id is required")
	}
	if openDingTalkID != "" {
		if err := targetresolver.ValidateExplicitOpenDingTalkID("--open-dingtalk-id", openDingTalkID); err != nil {
			return err
		}
	}
	if userID != "" && isOpenDingTalkID(userID) {
		openDingTalkID = userID
		userID = ""
	}
	timeVal := mustGetFlag(cmd, "time")
	defaultForward := true
	if timeVal == "" {
		timeVal = defaultChatMessageListTime()
		defaultForward = false
	}
	forward, err := resolveMessageForward(cmd, defaultForward)
	if err != nil {
		return err
	}
	direction := "older"
	if forward {
		direction = "newer"
	}
	params := map[string]any{"time": timeVal, "forward": forward}
	if v := chatIntFlagOrFallback(cmd, "limit", "size"); v > 0 {
		params["limit"] = v
	}
	if groupID != "" {
		params["openconversation_id"] = groupID
		return readAllConversationMessages(cmd, opts, "chat", "list_conversation_message_v2", params, direction)
	}
	if userID != "" {
		params["userId"] = userID
	} else {
		params["openDingTalkId"] = openDingTalkID
	}
	return readAllConversationMessages(cmd, opts, "chat", "list_individual_chat_message", params, direction)
}

// readAllConversationMessages iterates the time-boundary message-list protocol
// (next page = boundary derived from the millisecond nextCursor) until the
// source is complete or a reviewed safety bound stops it. Mirrors the shortcut
// layer readAllDirectMessages semantics; the single-page path stays in RunE.
func readAllConversationMessages(cmd *cobra.Command, opts pagedCommandOptions, serverID, toolName string, params map[string]any, direction string) error {
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run": true,
			"request": map[string]any{"server": serverID, "name": toolName, "args": params},
			"paging": map[string]any{
				"pageAll":   true,
				"pageLimit": opts.pageLimit,
				"maxItems":  opts.maxItems,
				"pageDelay": opts.delayMS,
			},
		})
	}
	seenCursors := map[string]bool{}
	seenMessages := map[string]bool{}
	allItems := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	complete := false
	hasMore := false
	stopReason := "source_complete"
	truncatedByPageLimit := false
	truncatedByResultLimit := false
	paginationKnown := true
	var nextPage map[string]any
	userLimit, hasUserLimit := params["limit"].(int)

	for pagesFetched < opts.pageLimit {
		if opts.maxItems > 0 {
			// Clamp each page to the remaining --max-items budget so an
			// honouring lower layer can never overshoot into in-page
			// truncation; the post-loop trim stays a violation safety net.
			remaining := opts.maxItems - len(allItems)
			if hasUserLimit && userLimit < remaining {
				params["limit"] = userLimit
			} else {
				params["limit"] = remaining
			}
		}
		raw, err := CallMCPToolDataOnServer(cmd.Context(), serverID, toolName, params)
		if err != nil {
			if pagesFetched == 0 {
				return err
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		data, _ := raw.(map[string]any)
		if data == nil {
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "error": "下层返回非对象响应",
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		projected := projectChatMessagesPayload(data, false)
		pageMessages, _ := projected["messages"].([]map[string]any)
		for _, item := range pageMessages {
			id := chatmsg.StableMessageID(item)
			if id != "" && seenMessages[id] {
				continue
			}
			if id != "" {
				seenMessages[id] = true
			}
			allItems = append(allItems, item)
		}

		pagination := map[string]any{}
		chatmsg.ApplyMessagePagination(pagination, data, pageMessages, direction)
		known, _ := pagination["paginationKnown"].(bool)
		if !known {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "下层未返回可靠的 hasMore，无法证明结果完整",
			})
			paginationKnown = false
			stopReason = "pagination_error"
			break
		}
		hasMore, _ = pagination["hasMore"].(bool)
		if !hasMore {
			complete = true
			nextPage = nil
			stopReason = "source_complete"
			break
		}
		next, _ := pagination["nextPage"].(map[string]any)
		boundary, _ := next["time"].(string)
		cursorKey := fmt.Sprint(next["nextCursor"])
		if boundary == "" || cursorKey == "" || cursorKey == "<nil>" {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "下层返回 hasMore=true，但 nextCursor 无效或当前页没有消息",
			})
			paginationKnown = false
			stopReason = "pagination_error"
			break
		}
		if seenCursors[cursorKey] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "毫秒 nextCursor 停滞，继续翻页将重复同一结果集",
			})
			paginationKnown = false
			stopReason = "pagination_error"
			break
		}
		seenCursors[cursorKey] = true
		nextPage = map[string]any{
			"direction": direction, "time": boundary, "nextCursor": next["nextCursor"],
		}
		params["time"] = boundary

		if opts.maxItems > 0 && len(allItems) >= opts.maxItems {
			truncatedByResultLimit = true
			stopReason = "result_limit"
			hasMore = true
			break
		}
		if opts.delayMS > 0 {
			if err := sleepPagedCommandDelay(cmd.Context(), time.Duration(opts.delayMS)*time.Millisecond); err != nil {
				failures = append(failures, map[string]any{
					"page": pagesFetched + 1, "stage": "delay", "error": err.Error(),
				})
				stopReason = "read_failure"
				break
			}
		}
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= opts.pageLimit && !truncatedByResultLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	if opts.maxItems > 0 && len(allItems) > opts.maxItems {
		// Lower layer violated the clamped limit. Truncate to the requested
		// budget, but publish no resume boundary: the page-tail cursor would
		// skip the dropped tail messages, so the safe answer is no nextPage.
		allItems = allItems[:opts.maxItems]
		truncatedByResultLimit = true
		stopReason = "result_limit"
		nextPage = nil
	}
	payload := chatmsg.NewMessageListPayload(allItems)
	payload["pagesFetched"] = pagesFetched
	payload["paginationKnown"] = paginationKnown
	payload["complete"] = complete && len(failures) == 0 && !truncatedByResultLimit
	payload["hasMore"] = hasMore
	payload["stopReason"] = stopReason
	payload["truncatedByPageLimit"] = truncatedByPageLimit
	payload["truncatedByResultLimit"] = truncatedByResultLimit
	chatmsg.ApplyTruncation(payload)
	payload["failedCount"] = len(failures)
	payload["failures"] = failures
	payload["partial"] = len(failures) > 0 && len(allItems) > 0
	if hasMore && nextPage != nil {
		payload["nextPage"] = nextPage
	}
	if err := writeCommandPayload(cmd, payload); err != nil {
		return err
	}
	if len(failures) == 0 {
		return nil
	}
	return apperrors.NewAPI(
		fmt.Sprintf("会话消息分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
		apperrors.WithOperation(serverID+"/"+toolName),
		apperrors.WithReason("messages_list_incomplete"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(true),
		apperrors.WithHint("请根据 failures 和 nextPage 重试"),
	)
}
