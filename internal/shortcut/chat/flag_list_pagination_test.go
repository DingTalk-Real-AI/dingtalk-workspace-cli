// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageFlagListDryRunStopsBeforeRead(t *testing.T) {
	fake := &larkAlignmentCaller{dryRun: true}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+flag-list", "--page-size", "20", "--cursor", "0", "--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("flag-list dry-run made lower calls: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageFlagListPageAllUsesNumericCursorAndDeduplicates(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/list_message_favorites": {
			`{"result":{"items":[{"openMessageId":"msg-1","openConversationId":"cid-1","summary":"一"}],"hasMore":true,"nextCursor":7}}`,
			`{"result":{"items":[{"openMessageId":"msg-1","openConversationId":"cid-1","summary":"重复"},{"openMessageId":"msg-2","openConversationId":"cid-2","summary":"二"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+flag-list", "--page-size", "1", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].args["cursor"] != 0 || fake.calls[0].args["size"] != "1" || fake.calls[1].args["cursor"] != 7 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(2) || payload["pagesFetched"] != float64(2) || payload["complete"] != true || payload["hasMore"] != false {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageFlagListPageTokenAndPageLimit(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_message_favorites": `{"result":{"items":[{"openMessageId":"msg-2"}],"hasMore":true,"nextCursor":9}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+flag-list", "--page-token", "7", "--page-all", "--page-limit", "1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].args["cursor"] != 7 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["nextCursor"] != float64(9) || payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageFlagListFailureModes(t *testing.T) {
	t.Run("later read failure keeps partial result", func(t *testing.T) {
		fake := &larkAlignmentCaller{
			sequenceResponses: map[string][]string{
				"im/list_message_favorites": {`{"result":{"items":[{"openMessageId":"msg-1"}],"hasMore":true,"nextCursor":7}}`},
			},
			failProductToolAt: map[string]int{"im/list_message_favorites": 2},
		}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{"chat", "+flag-list", "--page-all"})
		if err := root.Execute(); err == nil {
			t.Fatal("expected later-page error")
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["count"] != float64(1) || payload["partial"] != true || payload["failedCount"] != float64(1) || payload["stopReason"] != "read_failure" {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("stalled numeric cursor fails closed", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_message_favorites": `{"result":{"items":[{"openMessageId":"msg-1"}],"hasMore":true,"nextCursor":7}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetArgs([]string{"chat", "+flag-list", "--cursor", "7", "--page-all"})
		if err := root.Execute(); err == nil {
			t.Fatal("stalled cursor unexpectedly succeeded")
		}
	})

	t.Run("full legacy page without pagination fails closed", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_message_favorites": `{"result":{"items":[{"openMessageId":"msg-1"}]}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetArgs([]string{"chat", "+flag-list", "--page-size", "1"})
		if err := root.Execute(); err == nil {
			t.Fatal("full legacy page unexpectedly succeeded")
		}
	})
}

func TestCrossPlatformCoverageFlagListPaginationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--page-size", "0"},
		{"--page-size", "31"},
		{"--size", "31"},
		{"--page-token", "not-a-number"},
		{"--page-limit", "2"},
		{"--page-all", "--page-limit", "0"},
		{"--page-all", "--page-limit", "501"},
		{"--cursor", "1", "--page-token", "2"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+flag-list"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}
	if _, err := flagListNextCursor(float64(1.5)); err == nil {
		t.Fatal("fractional cursor unexpectedly accepted")
	}
	if _, err := flagListNextCursor(struct{}{}); err == nil {
		t.Fatal("unsupported cursor unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageFlagListAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+flag-list"}, args...))
		err := root.Execute()
		if output.Len() == 0 {
			return nil, err
		}
		var payload map[string]any
		if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return payload, err
	}

	for _, token := range []string{"", "0"} {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/list_message_favorites": `{"result":{"items":[],"hasMore":false}}`,
		}}, "--page-token="+token)
		if err != nil || payload["complete"] != true {
			t.Fatalf("token=%q payload=%#v err=%v", token, payload, err)
		}
	}

	t.Run("first read failure", func(t *testing.T) {
		_, err := run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"im/list_message_favorites": 1}})
		if err == nil {
			t.Fatal("first read failure unexpectedly succeeded")
		}
	})

	t.Run("cursor without hasMore continues", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/list_message_favorites": {
				`{"result":{"items":[{"openMessageId":"m1"}],"nextCursor":2}}`,
				`{"result":{"items":[],"hasMore":false}}`,
			},
		}}, "--page-all")
		if err != nil || payload["pagesFetched"] != float64(2) || payload["complete"] != true {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	for _, tc := range []struct {
		name     string
		response string
		wantErr  bool
		stop     string
	}{
		{name: "single page continuation", response: `{"result":{"items":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":2}}`, stop: "single_page"},
		{name: "missing continuation", response: `{"result":{"items":[{"openMessageId":"m1"}],"hasMore":true}}`, wantErr: true, stop: "pagination_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"im/list_message_favorites": tc.response}})
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("output errors propagate", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_message_favorites": `{"result":{"items":[],"hasMore":false}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(chatOutputErrorWriter{err: errors.New("fixture output")})
		root.SetArgs([]string{"chat", "+flag-list"})
		if err := root.Execute(); err == nil {
			t.Fatal("output error was swallowed")
		}
	})

	if got := flagListItems(nil); len(got) != 0 {
		t.Fatalf("nil items = %#v", got)
	}
	if got := flagListItems(map[string]any{"result": map[string]any{"items": []any{"invalid", map[string]any{"id": "m1"}}}}); len(got) != 1 {
		t.Fatalf("projected items = %#v", got)
	}
	if got := firstNonEmptyMapString(map[string]any{}, "missing"); got != "" {
		t.Fatalf("missing identity = %q", got)
	}
}

func TestCrossPlatformCoverageFlagListCursorTypeEdges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		ok    bool
	}{
		{name: "nil", value: nil, ok: true},
		{name: "int", value: int(1), ok: true},
		{name: "negative int", value: int(-1)},
		{name: "int64", value: int64(2), ok: true},
		{name: "negative int64", value: int64(-1)},
		{name: "float64", value: float64(3), ok: true},
		{name: "negative float64", value: float64(-1)},
		{name: "json number", value: json.Number("4"), ok: true},
		{name: "invalid json number", value: json.Number("bad")},
		{name: "empty string", value: "", ok: true},
		{name: "string", value: "5", ok: true},
		{name: "invalid string", value: "bad"},
		{name: "unsupported", value: struct{}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := flagListNextCursor(tc.value)
			if (err == nil) != tc.ok {
				t.Fatalf("value=%#v err=%v", tc.value, err)
			}
		})
	}
}
