// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

const messageTimeRangeSemantics = "[start,end)"

type chatMessageTimeRange struct {
	configured bool
	start      *time.Time
	end        *time.Time
	order      string
}

func resolveChatMessageTimeRange(rt *shortcut.RuntimeContext, now time.Time) (chatMessageTimeRange, error) {
	startValue := strings.TrimSpace(rt.StrFirst("start", "start-time"))
	endValue := strings.TrimSpace(rt.StrFirst("end", "end-time"))
	order := strings.ToLower(strings.TrimSpace(rt.StrFirst("order", "sort")))
	configured := startValue != "" || endValue != "" || order != ""
	if order == "" {
		order = "desc"
	}

	result := chatMessageTimeRange{configured: configured, order: order}
	if startValue != "" {
		parsed, err := parseDingTalkMessageTime(startValue)
		if err != nil {
			return chatMessageTimeRange{}, localChatOptionError(
				"invalid_start_time", "+chat-messages 的开始时间格式无效", "--start/--start-time")
		}
		result.start = &parsed
	}
	if endValue != "" {
		parsed, err := parseDingTalkMessageTime(endValue)
		if err != nil {
			return chatMessageTimeRange{}, localChatOptionError(
				"invalid_end_time", "+chat-messages 的结束时间格式无效", "--end/--end-time")
		}
		result.end = &parsed
	}
	if result.start != nil && result.end != nil && !result.end.After(*result.start) {
		return chatMessageTimeRange{}, apperrors.NewValidation("--end/--end-time 必须晚于 --start/--start-time")
	}
	if order == "asc" && result.start == nil {
		return chatMessageTimeRange{}, apperrors.NewValidation("升序读取必须指定 --start/--start-time，避免把最近一页倒序后误报为最早消息")
	}
	if result.start != nil && result.end == nil {
		effectiveEnd := now
		result.end = &effectiveEnd
	}
	return result, nil
}

func (r chatMessageTimeRange) initialBoundary(now time.Time) string {
	if !r.configured {
		return formatDingTalkMessageBoundary(now.Truncate(time.Second))
	}
	if r.order == "asc" && r.start != nil {
		return formatDingTalkMessageBoundary(*r.start)
	}
	if r.end != nil {
		return formatDingTalkMessageBoundary(*r.end)
	}
	return formatDingTalkMessageBoundary(now)
}

func (r chatMessageTimeRange) direction() string {
	if r.order == "asc" {
		return "newer"
	}
	return "older"
}

func (r chatMessageTimeRange) stopReason() string {
	if r.order == "asc" {
		return "range_end"
	}
	return "range_start"
}

func (r chatMessageTimeRange) metadata() map[string]any {
	if !r.configured {
		return nil
	}
	result := map[string]any{
		"order":     r.order,
		"semantics": messageTimeRangeSemantics,
	}
	if r.start != nil {
		result["startTime"] = r.start.Format(time.RFC3339Nano)
	}
	if r.end != nil {
		result["endTime"] = r.end.Format(time.RFC3339Nano)
	}
	return result
}

func (r chatMessageTimeRange) filter(items []map[string]any) ([]map[string]any, bool, []map[string]any) {
	if r.start == nil && r.end == nil {
		return items, false, nil
	}
	filtered := make([]map[string]any, 0, len(items))
	unknown := make([]string, 0)
	terminalReached := false
	for _, item := range items {
		messageTime, ok := parseMessageTimestamp(chatmsg.CreateTime(item))
		if !ok {
			unknown = append(unknown, chatmsg.StableMessageID(item))
			continue
		}
		if r.start != nil && messageTime.Before(*r.start) {
			if r.order == "desc" {
				terminalReached = true
			}
			continue
		}
		if r.end != nil && !messageTime.Before(*r.end) {
			if r.order == "asc" {
				terminalReached = true
			}
			continue
		}
		filtered = append(filtered, item)
	}
	if len(unknown) == 0 {
		return filtered, terminalReached, nil
	}
	return filtered, terminalReached, []map[string]any{{
		"stage":      "time_filter",
		"messageIds": unknown,
		"error":      "消息缺少可解析的 createTime，无法证明时间范围结果完整",
	}}
}

func parseDingTalkMessageTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, dingTalkMessageLocation); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported message time %q", value)
}

func parseMessageTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if parsed, err := parseDingTalkMessageTime(trimmed); err == nil {
			return parsed, true
		}
		if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixMessageTime(numeric), true
		}
	case json.Number:
		if numeric, err := typed.Int64(); err == nil {
			return unixMessageTime(numeric), true
		}
	case float64:
		return unixMessageTime(int64(typed)), true
	case float32:
		return unixMessageTime(int64(typed)), true
	case int:
		return unixMessageTime(int64(typed)), true
	case int64:
		return unixMessageTime(typed), true
	case int32:
		return unixMessageTime(int64(typed)), true
	}
	return time.Time{}, false
}

func unixMessageTime(value int64) time.Time {
	if value > -1_000_000_000_000 && value < 1_000_000_000_000 {
		return time.Unix(value, 0)
	}
	return time.UnixMilli(value)
}

func sortMessagesByCreateTimeStable(messages []map[string]any, order string) {
	order = strings.ToLower(strings.TrimSpace(order))
	if order != "asc" && order != "desc" {
		return
	}
	sort.SliceStable(messages, func(i, j int) bool {
		left, leftOK := parseMessageTimestamp(chatmsg.CreateTime(messages[i]))
		right, rightOK := parseMessageTimestamp(chatmsg.CreateTime(messages[j]))
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK || left.Equal(right) {
			return false
		}
		if order == "asc" {
			return left.Before(right)
		}
		return left.After(right)
	})
}

func searchMessageQueryRange(params map[string]any, order string) map[string]any {
	result := map[string]any{
		"order":     order,
		"semantics": messageTimeRangeSemantics,
	}
	if start, ok := numericMillis(params["startTime"]); ok {
		result["startTime"] = time.UnixMilli(start).In(dingTalkMessageLocation).Format(time.RFC3339)
	}
	if end, ok := numericMillis(params["endTime"]); ok {
		result["endTime"] = time.UnixMilli(end).In(dingTalkMessageLocation).Format(time.RFC3339)
	}
	return result
}

func numericMillis(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
