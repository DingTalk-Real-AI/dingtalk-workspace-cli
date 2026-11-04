// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package personal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func buildCalendarRuleParam(opts RuleOptions) (string, map[string]any, error) {
	if strings.TrimSpace(opts.UserID) != "" || strings.TrimSpace(opts.OpenDingTalkID) != "" ||
		strings.TrimSpace(opts.GroupID) != "" || opts.RoleTypes != nil {
		return "", nil, fmt.Errorf("--user/--open-dingtalk-id/--group/--role-types are not supported for %s", EventCalendarEventChanged)
	}
	params := map[string]any{}
	for _, dimension := range []struct {
		flag   string
		key    string
		values []string
	}{
		{"calendar-ids", "calendarIds", opts.CalendarIDs},
		{"change-types", "changeTypes", opts.ChangeTypes},
	} {
		// Absence means wildcard. A supplied empty array means no matches.
		if dimension.values == nil {
			continue
		}
		values := make([]string, 0, len(dimension.values))
		seen := map[string]bool{}
		for _, value := range dimension.values {
			value = strings.TrimSpace(value)
			if value == "" {
				return "", nil, fmt.Errorf("--%s must not contain an empty value", dimension.flag)
			}
			if dimension.key == "changeTypes" && value != "*" && value != "eventCreated" && value != "eventUpdated" && value != "eventCancelled" {
				return "", nil, fmt.Errorf("--change-types only supports eventCreated, eventUpdated, eventCancelled or *")
			}
			if !seen[value] {
				values = append(values, value)
				seen[value] = true
			}
		}
		if seen["*"] && len(values) > 1 {
			return "", nil, fmt.Errorf("--%s cannot combine * with specific values", dimension.flag)
		}
		sort.Strings(values)
		params[dimension.key] = values
	}
	return "all", params, nil
}

// CalendarEventOutput separates the calendar resource ID from the transport
// event ID. BizID is the stable delivery identity to use across MQ retries.
type CalendarEventOutput struct {
	Type            string `json:"type" description:"事件类型，固定为 user_calendar_event_changed"`
	EventID         string `json:"event_id" description:"事件中心事件 ID；业务投递去重使用 biz_id"`
	Timestamp       int64  `json:"timestamp" description:"事件中心事件时间戳" format:"timestamp_ms"`
	SubscribeID     string `json:"subscribe_id" description:"个人事件订阅 ID"`
	BizID           string `json:"biz_id" description:"业务投递唯一 ID；重试保持不变，按此幂等，不按日程 ID 去重"`
	EventTime       int64  `json:"event_time" description:"业务事件发生时间；重试保持不变，不表示消息顺序" format:"timestamp_ms"`
	ChangeType      string `json:"change_type" description:"日程变化类型：eventCreated、eventUpdated、eventCancelled；保留未来类型"`
	CalendarID      string `json:"calendar_id" description:"OpenAPI 日历本 ID；primary 表示目标用户的主日历"`
	CalendarEventID string `json:"calendar_event_id" description:"OpenAPI 日程资源 ID；取消后可能已不可查询，不是投递去重键"`
	OperatorUID     string `json:"operator_uid,omitempty" description:"可选操作人内部 uid 字符串；缺失时省略，不以目标用户代替"`
}

// calendarOperatorUID keeps flattened IDs as strings while accepting legacy
// Long payloads without converting them through float64.
type calendarOperatorUID string

func (id *calendarOperatorUID) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*id = calendarOperatorUID(value)
		return nil
	}
	var legacy int64
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("calendar operator uid must be a string or legacy integer: %w", err)
	}
	*id = calendarOperatorUID(strconv.FormatInt(legacy, 10))
	return nil
}

func projectCalendarEvent(base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload struct {
		BizID     string `json:"bizid"`
		EventTime *int64 `json:"event_time"`
		Body      struct {
			ChangeType  string              `json:"changeType"`
			CalendarID  string              `json:"calendarId"`
			EventID     string              `json:"eventId"`
			OperatorUID calendarOperatorUID `json:"operatorUid"`
		} `json:"body"`
	}
	// Routing uid/orgId are deliberately not decoded into an untyped float or
	// published as business fields. The original transport retains their bytes.
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return base, fmt.Errorf("decode personal calendar payload: %w", err)
	}
	if payload.EventTime == nil || strings.TrimSpace(payload.BizID) == "" || strings.TrimSpace(payload.Body.ChangeType) == "" ||
		strings.TrimSpace(payload.Body.CalendarID) == "" || strings.TrimSpace(payload.Body.EventID) == "" {
		return base, fmt.Errorf("decode personal calendar payload: bizid, event_time, changeType, calendarId and eventId are required")
	}
	return CalendarEventOutput{
		Type: base.Type, EventID: base.EventID, Timestamp: base.Timestamp, SubscribeID: base.SubscribeID,
		BizID: payload.BizID, EventTime: *payload.EventTime,
		ChangeType: payload.Body.ChangeType, CalendarID: payload.Body.CalendarID,
		CalendarEventID: payload.Body.EventID, OperatorUID: string(payload.Body.OperatorUID),
	}, nil
}
