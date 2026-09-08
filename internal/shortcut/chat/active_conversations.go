// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

const (
	activeConversationsOperation     = "chat/search_messages_by_time_range"
	activeConversationsDefaultLimit  = 100
	activeConversationsMaxLimit      = 100
	activeConversationsDefaultPages  = 50
	activeConversationsMaxPages      = 500
	activeConversationsDefaultDelay  = 200
	activeConversationsMaxDelay      = 60000
	activeConversationsRangeSemantic = "[start,end)"
)

var activeConversationsLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Inspect the original fraction: time.Parse silently truncates digits beyond
// nanoseconds, which must not make a nonzero subsecond boundary look integral.
var activeConversationsNonzeroFraction = regexp.MustCompile(`[.,]0*[1-9]`)

// ActiveConversations returns the distinct direct and group conversations
// that contain at least one message in a requested time range. The lower API
// returns grouped message details, so this Shortcut owns cursor exhaustion,
// conversation deduplication, latest-time calculation, and completeness.
var ActiveConversations = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "chat",
	Command:       "+active-conversations",
	Product:       "chat",
	Description:   "列出指定时间以来有新消息的单聊和群聊",
	Intent:        "当你只需要知道从某个时间开始哪些单聊或群聊出现了新消息，而不需要读取消息正文时使用；最少只传 --start，CLI 会固定本次查询的结束时间、自动翻页、按 openConversationId 去重，并返回会话名称、类型和时间窗内最新消息时间。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_active_conversations",
			CanonicalPath:  "chat.shortcut_active_conversations",
			CLIPath:        "chat +active-conversations",
			PrimaryCLIPath: "chat +active-conversations",
		},
		Description: "列出指定时间以来有新消息的单聊和群聊",
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "Reviewed Chat Shortcut composite: the CLI owns time normalization, cursor exhaustion, conversation deduplication, latest-message aggregation, stable projection, and completeness metadata over chat/search_messages_by_time_range.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出指定时间以来有新消息的单聊和群聊",
			UseWhen:      []string{"当你只需要知道从某个时间开始哪些单聊或群聊出现了新消息，而不需要读取消息正文时使用；最少只传 --start，CLI 会固定本次查询的结束时间、自动翻页、按 openConversationId 去重，并返回会话名称、类型和时间窗内最新消息时间。"},
			AvoidWhen:    []string{"需要读取消息正文或按发送者、关键词、@对象筛选时使用 +search-msg；需要不区分是否有新消息的全部会话时使用 +conversation-list；监听未来消息时使用 event +listen-im"},
			Examples: []string{
				"dws chat +active-conversations --start \"2026-09-07T00:00:00+08:00\"",
				"dws chat +active-conversations --start \"2026-09-07T00:00:00+08:00\" --end \"2026-09-08T00:00:00+08:00\"",
			},
		},
		Parameters: []contract.ParamDecl{
			{Name: "start", Property: "startTime"},
			{Name: "end", Property: "endTime"},
			{Name: "limit", Property: "limit"},
			{Name: "cursor", Property: "cursor"},
		},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
			DataSchema: activeConversationsResultSchema(),
			SensitivePaths: []string{
				"conversations.conversationId", "conversations.name",
				"succeeded.conversations.conversationId", "succeeded.conversations.name",
			},
		},
		Pagination: &contract.PaginationSpec{
			Kind:                  contract.PaginationKindCursor,
			CursorParameter:       "cursor",
			MetaPath:              contract.PaginationMetaPath,
			EndpointExhaustedPath: contract.PaginationExhaustedPath,
			NextTokenPath:         contract.PaginationNextTokenPath,
		},
	},
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Required: true, Desc: "开始时间（包含），支持 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD；查询时间边界仅支持整秒，拒绝非零小数秒；--start 不能为空白；--end 必须晚于 --start"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间（不包含），格式同 --start；查询时间边界仅支持整秒，拒绝非零小数秒；不传时固定为本次查询当前时间向下取整秒，不包含当前未结束秒；有效整秒区间的 --end 必须晚于 --start；使用非首页 --cursor 时必须显式复用原查询的 --end"},
		{Name: "limit", Type: shortcut.FlagInt, Default: strconv.Itoa(activeConversationsDefaultLimit), Desc: "底层每页消息数量；--limit 必须在 1-100 之间"},
		{Name: "cursor", Type: shortcut.FlagString, Default: "0", Desc: "续页游标；使用非首页 --cursor 时必须显式复用原查询的 --end；续页应保持同一 profile、--start、--end 和 --limit（结果 pageSize）；续页批次不包含此前结果，complete 始终为 false"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: strconv.Itoa(activeConversationsDefaultPages), Desc: "自动分页安全上限；--page-limit 必须在 1-500 之间；达到上限仍有下一页时返回 complete=false 和 next_token"},
		{Name: "page-delay", Type: shortcut.FlagInt, Default: strconv.Itoa(activeConversationsDefaultDelay), Desc: "自动分页间隔毫秒数；--page-delay 必须在 0-60000 之间；默认 200，0 表示不等待；等待可取消"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start"}, Description: "--start 不能为空白"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "--end 必须晚于 --start"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "查询时间边界仅支持整秒，拒绝非零小数秒"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-100 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须在 1-500 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-delay"}, Description: "--page-delay 必须在 0-60000 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor", "end"}, Description: "使用非首页 --cursor 时必须显式复用原查询的 --end"},
	},
	Tips: []string{
		`dws chat +active-conversations --start "2026-09-07T00:00:00+08:00"`,
		`dws chat +active-conversations --start "2026-09-07 00:00:00" --end "2026-09-08 00:00:00"`,
		"分页中途失败返回 partial_failure（退出码 7）；已验证页面聚合保存在 data.succeeded[0]，失败页和原始诊断在 data.failed[0].error；处理错误后可用 meta.pagination.next_token 和相同 profile、start/end/limit 续查。",
		"会话 name 始终返回；下层未提供名称时 name 为空字符串、nameKnown=false。",
	},
	Validate: validateActiveConversations,
	Execute:  executeActiveConversations,
}

type activeConversationRange struct {
	start time.Time
	end   time.Time
}

type activeConversationRangeContextKey struct{}

type activeConversationState struct {
	conversationID   string
	name             string
	conversationType string
	latest           time.Time
}

func validateActiveConversations(rt *shortcut.RuntimeContext) error {
	queryRange, err := activeConversationTimeRange(rt)
	if err != nil {
		return err
	}
	if limit := rt.Int("limit"); limit < 1 || limit > activeConversationsMaxLimit {
		return apperrors.NewValidation("--limit 必须在 1-100 之间")
	}
	if pageLimit := rt.Int("page-limit"); pageLimit < 1 || pageLimit > activeConversationsMaxPages {
		return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
	}
	if delay := rt.Int("page-delay"); delay < 0 || delay > activeConversationsMaxDelay {
		return apperrors.NewValidation("--page-delay 必须在 0-60000 之间")
	}
	if cursor := strings.TrimSpace(rt.Str("cursor")); cursor != "" && cursor != "0" && (!rt.Changed("end") || strings.TrimSpace(rt.Str("end")) == "") {
		return apperrors.NewValidation("使用非首页 --cursor 时必须显式传入上次结果中的同一 --end")
	}
	// Keep the validated default end unchanged through execution and all pages.
	// Each validation replaces it, including when a Cobra command is reused.
	ctx := rt.Command().Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rt.Command().SetContext(context.WithValue(ctx, activeConversationRangeContextKey{}, queryRange))
	return nil
}

func executeActiveConversations(rt *shortcut.RuntimeContext) error {
	queryRange, err := activeConversationExecutionRange(rt)
	if err != nil {
		return err
	}
	cursor := strings.TrimSpace(rt.Str("cursor"))
	if cursor == "" {
		cursor = "0"
	}
	initialCursor := cursor
	seenCursors := map[string]bool{cursor: true}
	states := make(map[string]*activeConversationState)
	pagesFetched := 0
	endpointExhausted := false
	nextCursor := ""
	var pageFailure error

	for pagesFetched < rt.Int("page-limit") {
		delay := time.Duration(0)
		if pagesFetched > 0 {
			delay = time.Duration(rt.Int("page-delay")) * time.Millisecond
		}
		if err := waitActiveConversationPage(rt.Command().Context(), delay); err != nil {
			pageFailure = err
			break
		}
		data, callErr := rt.CallMCPReadData("chat", "search_messages_by_time_range", map[string]any{
			"startTime": queryRange.start.In(activeConversationsLocation).Format("2006-01-02 15:04:05"),
			"endTime":   queryRange.end.In(activeConversationsLocation).Format("2006-01-02 15:04:05"),
			"limit":     rt.Int("limit"),
			"cursor":    cursor,
		})
		if callErr != nil {
			pageFailure = callErr
			break
		}
		groups, more, next, pageErr := parseActiveConversationPage(data)
		if pageErr != nil {
			pageFailure = pageErr
			break
		}
		next = strings.TrimSpace(next)
		if more && (next == "" || next == "0" || seenCursors[next]) {
			pageFailure = activeConversationResponseError("invalid_pagination", "下层返回 hasMore=true，但 nextCursor 缺失、回到首页、停滞或形成循环")
			break
		}
		if err := mergeActiveConversationPage(states, groups, queryRange); err != nil {
			pageFailure = err
			break
		}
		pagesFetched++
		if !more {
			endpointExhausted = true
			nextCursor = ""
			break
		}
		nextCursor = next
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	if pageFailure != nil {
		if pagesFetched == 0 {
			return pageFailure
		}
		// Retry the failed page, never a cursor from an unvalidated response.
		nextCursor = cursor
	}

	rows, unknownTypes := projectActiveConversations(states)
	complete := initialCursor == "0" && endpointExhausted
	pagination, err := output.NewPagination(endpointExhausted, nextCursor)
	if err != nil {
		return activeConversationResponseError("invalid_pagination", err.Error())
	}
	pagination.Pages = pagesFetched
	pagination.Items = len(rows)
	payload := map[string]any{
		"start":            queryRange.start.In(activeConversationsLocation).Format(time.RFC3339),
		"end":              queryRange.end.In(activeConversationsLocation).Format(time.RFC3339),
		"rangeSemantics":   activeConversationsRangeSemantic,
		"count":            len(rows),
		"complete":         complete,
		"pagesFetched":     pagesFetched,
		"pageSize":         rt.Int("limit"),
		"unknownTypeCount": unknownTypes,
		"conversations":    rows,
	}
	meta := &output.Meta{Count: output.NewCount(len(rows)), Pagination: pagination}
	if pageFailure != nil {
		// These are two processing units: a verified aggregate batch and the
		// failed page. total is neither the number of pages nor conversations.
		payload["id"] = "completed-pages"
		partial, err := output.NewPartialData(2, []any{payload}, []output.PartialFailedEntry{{
			ID:    fmt.Sprintf("page:%d", pagesFetched+1),
			Error: activeConversationPageErrorInfo(pageFailure, pagesFetched+1, cursor),
		}}, nil)
		if err != nil {
			return err
		}
		return output.StoreResult(rt.Command().Context(), output.Partial(partial, output.WithMeta(meta)))
	}
	return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
}

func waitActiveConversationPage(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Stage only the current page's affected conversations. Nothing from a failed
// page may change the verified aggregate, including updates to existing IDs.
func mergeActiveConversationPage(states map[string]*activeConversationState, groups []map[string]any, queryRange activeConversationRange) error {
	staged := make(map[string]*activeConversationState, len(groups))
	for index, group := range groups {
		id := activeConversationString(group, "openConversationId")
		if _, stagedAlready := staged[id]; !stagedAlready {
			if current, exists := states[id]; exists {
				copyState := *current
				staged[id] = &copyState
			}
		}
		if err := mergeActiveConversationGroup(staged, group, index, queryRange); err != nil {
			return err
		}
	}
	for id, state := range staged {
		states[id] = state
	}
	return nil
}

func activeConversationTimeRange(rt *shortcut.RuntimeContext) (activeConversationRange, error) {
	return resolveActiveConversationTimeRange(rt.Str("start"), rt.Str("end"), time.Now())
}

func activeConversationExecutionRange(rt *shortcut.RuntimeContext) (activeConversationRange, error) {
	if ctx := rt.Command().Context(); ctx != nil {
		if queryRange, ok := ctx.Value(activeConversationRangeContextKey{}).(activeConversationRange); ok {
			return queryRange, nil
		}
	}
	// Standalone Execute callers still receive the same strict validation.
	return activeConversationTimeRange(rt)
}

func resolveActiveConversationTimeRange(startRaw, endRaw string, now time.Time) (activeConversationRange, error) {
	startRaw = strings.TrimSpace(startRaw)
	if startRaw == "" {
		return activeConversationRange{}, apperrors.NewValidation("--start 不能为空白")
	}
	start, err := parseActiveConversationBoundary(startRaw, "--start")
	if err != nil {
		return activeConversationRange{}, err
	}
	end := now.Truncate(time.Second)
	if endRaw = strings.TrimSpace(endRaw); endRaw != "" {
		end, err = parseActiveConversationBoundary(endRaw, "--end")
		if err != nil {
			return activeConversationRange{}, err
		}
	}
	if !end.After(start) {
		return activeConversationRange{}, apperrors.NewValidation("--end 必须晚于 --start")
	}
	return activeConversationRange{start: start, end: end}, nil
}

func parseActiveConversationBoundary(value, flag string) (time.Time, error) {
	parsed, err := parseActiveConversationTime(value)
	if err != nil {
		return time.Time{}, apperrors.NewValidation(flag + " 必须是 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD")
	}
	if activeConversationsNonzeroFraction.MatchString(value) {
		return time.Time{}, apperrors.NewValidation(flag + " 仅支持整秒，不能包含非零小数秒")
	}
	return parsed, nil
}

func parseActiveConversationTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, activeConversationsLocation); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time %q", value)
}

func parseActiveConversationPage(data map[string]any) ([]map[string]any, bool, string, error) {
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil, false, "", activeConversationResponseError("missing_result", "响应缺少 result 对象")
	}
	hasMore, ok := result["hasMore"].(bool)
	if !ok {
		return nil, false, "", activeConversationResponseError("missing_pagination", "响应缺少布尔型 result.hasMore，无法证明结果完整")
	}
	rawGroups := result["conversationMessagesList"]
	if rawGroups == nil && !hasMore {
		return []map[string]any{}, false, "", nil
	}
	groups, ok := rawGroups.([]any)
	if !ok {
		return nil, false, "", activeConversationResponseError("malformed_collection", "响应 result.conversationMessagesList 必须是数组")
	}
	items := make([]map[string]any, 0, len(groups))
	for index, value := range groups {
		group, ok := value.(map[string]any)
		if !ok {
			return nil, false, "", activeConversationResponseError("malformed_item", fmt.Sprintf("响应 result.conversationMessagesList[%d] 必须是对象", index))
		}
		items = append(items, group)
	}
	nextCursor := ""
	if value, exists := result["nextCursor"]; exists && value != nil {
		switch typed := value.(type) {
		case string:
			nextCursor = strings.TrimSpace(typed)
		case json.Number:
			if _, err := typed.Int64(); err != nil {
				return nil, false, "", activeConversationResponseError("invalid_pagination", "响应 result.nextCursor 必须是字符串或整数")
			}
			nextCursor = typed.String()
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || math.Abs(typed) >= 1<<53 {
				return nil, false, "", activeConversationResponseError("invalid_pagination", "响应 result.nextCursor 必须是字符串或精确整数")
			}
			nextCursor = strconv.FormatFloat(typed, 'f', 0, 64)
		default:
			return nil, false, "", activeConversationResponseError("invalid_pagination", "响应 result.nextCursor 必须是字符串或整数")
		}
		if nextCursor == "<nil>" || strings.EqualFold(nextCursor, "null") {
			nextCursor = ""
		}
	}
	if hasMore && nextCursor == "" {
		return nil, false, "", activeConversationResponseError("missing_next_cursor", "响应 result.hasMore=true，但缺少 result.nextCursor")
	}
	return items, hasMore, nextCursor, nil
}

func mergeActiveConversationGroup(states map[string]*activeConversationState, group map[string]any, index int, queryRange activeConversationRange) error {
	conversationID := activeConversationString(group, "openConversationId")
	if conversationID == "" {
		return activeConversationResponseError("missing_conversation_id", fmt.Sprintf("响应会话项 %d 缺少 openConversationId", index))
	}
	conversationType, err := activeConversationType(group)
	if err != nil {
		return activeConversationResponseError("malformed_conversation_type", fmt.Sprintf("会话 %s 的 singleChat 必须是布尔值", conversationID))
	}
	messages, err := activeConversationMessages(group)
	if err != nil {
		return activeConversationResponseError("malformed_messages", fmt.Sprintf("会话 %s 的 messages 必须是非空对象数组", conversationID))
	}
	latest := time.Time{}
	matched := false
	for messageIndex, message := range messages {
		messageTime, ok := parseActiveConversationTimestamp(chatmsg.CreateTime(message))
		if !ok {
			return activeConversationResponseError("invalid_message_time", fmt.Sprintf("会话 %s 的 messages[%d] 缺少可解析的消息时间", conversationID, messageIndex))
		}
		if messageTime.Before(queryRange.start) || !messageTime.Before(queryRange.end) {
			continue
		}
		if !matched || messageTime.After(latest) {
			latest = messageTime
		}
		matched = true
	}
	// An out-of-window group must neither create a conversation nor update an
	// existing conversation's timestamp or metadata. Its page still advances.
	if !matched {
		return nil
	}
	name := activeConversationString(group, "title", "conversationTitle", "conversationName", "name")
	state, exists := states[conversationID]
	if !exists {
		states[conversationID] = &activeConversationState{
			conversationID:   conversationID,
			name:             name,
			conversationType: conversationType,
			latest:           latest,
		}
		return nil
	}
	if state.conversationType == "unknown" && conversationType != "unknown" {
		state.conversationType = conversationType
	} else if conversationType != "unknown" && state.conversationType != conversationType {
		return activeConversationResponseError("conflicting_conversation_type", fmt.Sprintf("会话 %s 跨页返回了冲突的单聊/群聊类型", conversationID))
	}
	if latest.After(state.latest) {
		state.latest = latest
		if name != "" {
			state.name = name
		}
	} else if state.name == "" && name != "" {
		state.name = name
	}
	return nil
}

func activeConversationMessages(group map[string]any) ([]map[string]any, error) {
	raw, present := group["messages"]
	if !present {
		return nil, fmt.Errorf("missing messages")
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return nil, fmt.Errorf("messages must be a non-empty array")
	}
	messages := make([]map[string]any, 0, len(values))
	for _, value := range values {
		message, ok := value.(map[string]any)
		if !ok || len(message) == 0 {
			return nil, fmt.Errorf("message must be a non-empty object")
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func activeConversationType(group map[string]any) (string, error) {
	value, present := group["singleChat"]
	if !present || value == nil {
		return "unknown", nil
	}
	singleChat, ok := value.(bool)
	if !ok {
		return "", fmt.Errorf("singleChat must be boolean")
	}
	if singleChat {
		return "direct", nil
	}
	return "group", nil
}

func activeConversationString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		text, ok := value[key].(string)
		if ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func parseActiveConversationTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if parsed, err := parseActiveConversationTime(trimmed); err == nil {
			return parsed, true
		}
		numeric, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		return activeConversationUnixTime(numeric)
	case json.Number:
		numeric, err := typed.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return activeConversationUnixTime(numeric)
	case int:
		return activeConversationUnixTime(int64(typed))
	case int32:
		return activeConversationUnixTime(int64(typed))
	case int64:
		return activeConversationUnixTime(typed)
	case float32:
		return activeConversationFloatTime(float64(typed))
	case float64:
		return activeConversationFloatTime(typed)
	default:
		return time.Time{}, false
	}
}

func activeConversationFloatTime(value float64) (time.Time, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || math.Trunc(value) != value || value > math.MaxInt64 {
		return time.Time{}, false
	}
	return activeConversationUnixTime(int64(value))
}

func activeConversationUnixTime(value int64) (time.Time, bool) {
	if value <= 0 {
		return time.Time{}, false
	}
	if value < 1_000_000_000_000 {
		return time.Unix(value, 0), true
	}
	return time.UnixMilli(value), true
}

func projectActiveConversations(states map[string]*activeConversationState) ([]map[string]any, int) {
	values := make([]*activeConversationState, 0, len(states))
	for _, state := range states {
		values = append(values, state)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].latest.Equal(values[j].latest) {
			return values[i].conversationID < values[j].conversationID
		}
		return values[i].latest.After(values[j].latest)
	})
	rows := make([]map[string]any, 0, len(values))
	unknownTypes := 0
	for _, state := range values {
		row := map[string]any{
			"conversationId":    state.conversationID,
			"name":              state.name,
			"nameKnown":         state.name != "",
			"type":              state.conversationType,
			"latestMessageTime": state.latest.In(activeConversationsLocation).Format(time.RFC3339Nano),
		}
		if state.conversationType == "unknown" {
			unknownTypes++
		}
		rows = append(rows, row)
	}
	return rows, unknownTypes
}

func activeConversationResponseError(reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(activeConversationsOperation),
		apperrors.WithReason(reason),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
	)
}

func init() {
	shortcut.Register(ActiveConversations)
}
