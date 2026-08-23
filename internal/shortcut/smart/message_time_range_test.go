// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func messageTimeRangeRuntime(t *testing.T, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"start", "start-time", "end", "end-time", "order", "sort"} {
		cmd.Flags().String(name, "", "")
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, shortcut.Shortcut{})
}

func TestCrossPlatformCoverageResolveMessageTimeRange(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, dingTalkMessageLocation)

	plain, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, nil), now)
	if err != nil || plain.configured || plain.order != "desc" || plain.start != nil || plain.end != nil {
		t.Fatalf("plain range = %#v, %v", plain, err)
	}
	if plain.metadata() != nil || plain.initialBoundary(now) != formatDingTalkMessageBoundary(now) {
		t.Fatalf("plain metadata/boundary = %#v, %q", plain.metadata(), plain.initialBoundary(now))
	}
	orderOnly, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, map[string]string{
		"order": "desc",
	}), now)
	if err != nil || !orderOnly.configured || orderOnly.start != nil || orderOnly.end != nil ||
		orderOnly.initialBoundary(now) != formatDingTalkMessageBoundary(now) {
		t.Fatalf("order-only range = %#v, %v", orderOnly, err)
	}

	ascending, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, map[string]string{
		"start-time": "2026-08-01",
		"end-time":   "2026-08-03 00:00:00",
		"sort":       " ASC ",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if ascending.direction() != "newer" || ascending.stopReason() != "range_end" {
		t.Fatalf("ascending direction/reason = %q, %q", ascending.direction(), ascending.stopReason())
	}
	if ascending.initialBoundary(now) != formatDingTalkMessageBoundary(*ascending.start) {
		t.Fatalf("ascending boundary = %q", ascending.initialBoundary(now))
	}
	metadata := ascending.metadata()
	if metadata["order"] != "asc" || metadata["semantics"] != messageTimeRangeSemantics || metadata["startTime"] == nil || metadata["endTime"] == nil {
		t.Fatalf("ascending metadata = %#v", metadata)
	}

	startOnly, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, map[string]string{
		"start": "2026-08-01T00:00:00+08:00",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if startOnly.end == nil || !startOnly.end.Equal(now) || startOnly.direction() != "older" || startOnly.stopReason() != "range_start" {
		t.Fatalf("start-only range = %#v", startOnly)
	}
	if startOnly.initialBoundary(now) != formatDingTalkMessageBoundary(now) {
		t.Fatalf("start-only boundary = %q", startOnly.initialBoundary(now))
	}

	fractional, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, map[string]string{
		"start": "2026-08-02T00:00:00.125+08:00",
		"end":   "2026-08-03T00:00:00.500+08:00",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if got := fractional.initialBoundary(now); got != "2026-08-03T00:00:00.5+08:00" {
		t.Fatalf("fractional boundary = %q", got)
	}
	fractionalMetadata := fractional.metadata()
	if fractionalMetadata["startTime"] != "2026-08-02T00:00:00.125+08:00" ||
		fractionalMetadata["endTime"] != "2026-08-03T00:00:00.5+08:00" {
		t.Fatalf("fractional metadata = %#v", fractionalMetadata)
	}

	for name, values := range map[string]map[string]string{
		"invalid start": {"start": "not-a-time"},
		"invalid end":   {"end": "not-a-time"},
		"reversed":      {"start": "2026-08-03", "end": "2026-08-02"},
		"asc no start":  {"end": "2026-08-03", "order": "asc"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveChatMessageTimeRange(messageTimeRangeRuntime(t, values), now); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCrossPlatformCoverageMessageTimeRangeParsesLocalAndEpochTimes(t *testing.T) {
	local, err := parseDingTalkMessageTime("2026-01-02 03:04:05")
	if err != nil {
		t.Fatal(err)
	}
	if local.Location().String() != "CST" || local.Format(time.RFC3339) != "2026-01-02T03:04:05+08:00" {
		t.Fatalf("local time = %s (%s)", local.Format(time.RFC3339), local.Location())
	}
	for _, value := range []any{
		local.Unix(), int(local.Unix()), int32(local.Unix()),
		local.UnixMilli(), float64(local.UnixMilli()), json.Number("1767294245000"), "1767294245000",
	} {
		parsed, ok := parseMessageTimestamp(value)
		if !ok || parsed.Unix() != local.Unix() {
			t.Fatalf("parse %v = %s, %v", value, parsed, ok)
		}
	}
	float32Seconds := float32(local.Unix())
	if parsed, ok := parseMessageTimestamp(float32Seconds); !ok || parsed.Unix() != int64(float32Seconds) {
		t.Fatalf("parse float32 %v = %s, %v", float32Seconds, parsed, ok)
	}
	if parsed, err := parseDingTalkMessageTime("2026-01-02T03:04:05+08:00"); err != nil || parsed.Unix() != local.Unix() {
		t.Fatalf("RFC3339 parse = %s, %v", parsed, err)
	}
	if _, err := parseDingTalkMessageTime("unsupported"); err == nil {
		t.Fatal("unsupported time unexpectedly parsed")
	}
	for _, value := range []any{json.Number("bad"), "bad", true, nil} {
		if parsed, ok := parseMessageTimestamp(value); ok || !parsed.IsZero() {
			t.Fatalf("invalid timestamp %v = %s, %v", value, parsed, ok)
		}
	}
}

func TestCrossPlatformCoverageMessageTimeRangeStableSortKeepsUnknownRows(t *testing.T) {
	messages := []map[string]any{
		{"openMessageId": "unknown"},
		{"openMessageId": "later", "createTime": "2026-01-02 00:00:00"},
		{"openMessageId": "same-a", "createTime": "2026-01-01 00:00:00"},
		{"openMessageId": "same-b", "createTime": "2026-01-01 00:00:00"},
	}
	sortMessagesByCreateTimeStable(messages, "asc")
	want := []string{"same-a", "same-b", "later", "unknown"}
	for index, id := range want {
		if messages[index]["openMessageId"] != id {
			t.Fatalf("sorted messages = %#v", messages)
		}
	}
	sortMessagesByCreateTimeStable(messages, "desc")
	if messages[0]["openMessageId"] != "later" || messages[len(messages)-1]["openMessageId"] != "unknown" {
		t.Fatalf("descending messages = %#v", messages)
	}
	before := append([]map[string]any(nil), messages...)
	sortMessagesByCreateTimeStable(messages, "sideways")
	for index := range before {
		if before[index]["openMessageId"] != messages[index]["openMessageId"] {
			t.Fatalf("invalid-order sort changed messages = %#v", messages)
		}
	}
}

func TestCrossPlatformCoverageMessageTimeRangeFiltering(t *testing.T) {
	start, _ := parseDingTalkMessageTime("2026-08-02 00:00:00")
	end, _ := parseDingTalkMessageTime("2026-08-04 00:00:00")
	items := []map[string]any{
		{"openMessageId": "before", "createTime": "2026-08-01 00:00:00"},
		{"openMessageId": "inside", "createTime": "2026-08-03 00:00:00"},
		{"openMessageId": "end", "createTime": "2026-08-04 00:00:00"},
		{"openMessageId": "unknown"},
	}

	plain := chatMessageTimeRange{}
	plainItems, terminal, failures := plain.filter(items)
	if len(plainItems) != len(items) || terminal || failures != nil {
		t.Fatalf("plain filter = %#v, %v, %#v", plainItems, terminal, failures)
	}

	desc := chatMessageTimeRange{start: &start, end: &end, order: "desc"}
	filtered, terminal, failures := desc.filter(items)
	if len(filtered) != 1 || filtered[0]["openMessageId"] != "inside" || !terminal || len(failures) != 1 {
		t.Fatalf("descending filter = %#v, %v, %#v", filtered, terminal, failures)
	}

	asc := chatMessageTimeRange{start: &start, end: &end, order: "asc"}
	filtered, terminal, failures = asc.filter(items[1:3])
	if len(filtered) != 1 || filtered[0]["openMessageId"] != "inside" || !terminal || failures != nil {
		t.Fatalf("ascending filter = %#v, %v, %#v", filtered, terminal, failures)
	}

	filtered, terminal, failures = desc.filter(items[1:2])
	if len(filtered) != 1 || terminal || failures != nil {
		t.Fatalf("clean filter = %#v, %v, %#v", filtered, terminal, failures)
	}

	fractionalStart, _ := parseDingTalkMessageTime("2026-08-03T00:00:00+08:00")
	fractionalEnd, _ := parseDingTalkMessageTime("2026-08-03T00:00:00.500+08:00")
	fractionalRange := chatMessageTimeRange{start: &fractionalStart, end: &fractionalEnd, order: "desc"}
	fractionalItems := []map[string]any{
		{"openMessageId": "inside-fraction", "createTime": "2026-08-03T00:00:00.250+08:00"},
		{"openMessageId": "end-fraction", "createTime": "2026-08-03T00:00:00.500+08:00"},
	}
	filtered, terminal, failures = fractionalRange.filter(fractionalItems)
	if len(filtered) != 1 || filtered[0]["openMessageId"] != "inside-fraction" || terminal || failures != nil {
		t.Fatalf("fractional filter = %#v, %v, %#v", filtered, terminal, failures)
	}
}

func TestCrossPlatformCoverageSearchMessageQueryRangeNumericInputs(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, dingTalkMessageLocation).UnixMilli()
	end := time.Date(2026, 8, 2, 0, 0, 0, 0, dingTalkMessageLocation).UnixMilli()
	queryRange := searchMessageQueryRange(map[string]any{
		"startTime": int(start),
		"endTime":   end,
	}, "asc")
	if queryRange["order"] != "asc" || queryRange["semantics"] != messageTimeRangeSemantics || queryRange["startTime"] == nil || queryRange["endTime"] == nil {
		t.Fatalf("query range = %#v", queryRange)
	}

	for _, value := range []any{int64(start), int(start), float64(start), json.Number("1785513600000")} {
		if parsed, ok := numericMillis(value); !ok || parsed != start {
			t.Fatalf("numericMillis(%T) = %d, %v", value, parsed, ok)
		}
	}
	for _, value := range []any{json.Number("bad"), "1785513600000", nil} {
		if parsed, ok := numericMillis(value); ok || parsed != 0 {
			t.Fatalf("invalid numericMillis(%T) = %d, %v", value, parsed, ok)
		}
	}
	queryRange = searchMessageQueryRange(map[string]any{"startTime": "bad", "endTime": true}, "desc")
	if len(queryRange) != 2 {
		t.Fatalf("invalid numeric query range = %#v", queryRange)
	}
}
