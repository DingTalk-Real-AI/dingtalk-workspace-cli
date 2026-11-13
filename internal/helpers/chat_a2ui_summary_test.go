// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func TestCrossPlatformCoverageA2UICardSummary(t *testing.T) {
	const payload = `["{\"version\":\"v1.0\",\"createSurface\":{\"surfaceId\":\"summary-test\"}}"]`
	for _, target := range [][]string{
		{"--conversation-id", "group-1"},
		{"--open-dingtalk-id", "DAAAAAAAAAAAiE"},
	} {
		for _, tc := range []struct {
			name    string
			flags   []string
			want    string
			invalid bool
		}{
			{name: "omitted uses readable metadata", want: "互动卡片"},
			{name: "explicit summary", flags: []string{"--summary", "Weekly report"}, want: "Weekly report"},
			{name: "preserve explicit text", flags: []string{"--summary", " Task result\nReady "}, want: " Task result\nReady "},
			{name: "explicit empty", flags: []string{"--summary", ""}, invalid: true},
			{name: "explicit whitespace", flags: []string{"--summary", "\t\n\u3000"}, invalid: true},
		} {
			t.Run(target[0]+"/"+tc.name, func(t *testing.T) {
				caller := &scriptedToolCaller{}
				args := append([]string{"message", "send-a2ui-card", "--content", payload}, target...)
				args = append(args, tc.flags...)
				err := runNativeCardUpdate(t, caller, args...)
				if tc.invalid {
					if err == nil || caller.calls != 0 {
						t.Fatalf("err=%v calls=%d", err, caller.calls)
					}
					return
				}
				if err != nil || caller.calls != 1 || caller.args["summary"] != tc.want {
					t.Fatalf("err=%v caller=%#v", err, caller)
				}
				if !reflect.DeepEqual(caller.args["a2uiMessages"], []string{`{"version":"v1.0","createSurface":{"surfaceId":"summary-test"}}`}) {
					t.Fatalf("protocol content changed: %#v", caller.args)
				}
			})
		}
	}
	t.Run("stdin and forwarding preserve the supplied summary", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		installScriptedCaller(t, caller)
		root := newChatCommand()
		root.SetIn(strings.NewReader(payload))
		root.SetArgs([]string{"message", "send-a2ui-card", "--conversation-id", "group-1", "--content", "-", "--summary", "Task result", "--support-forward"})
		if err := corecmd.ExecuteForTest(root); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.args["summary"] != "Task result" || caller.args["supportForward"] != true {
			t.Fatalf("caller=%#v", caller)
		}
	})
}

func TestCrossPlatformCoverageA2UICardSummaryPreservesLongContent(t *testing.T) {
	message := `{"version":"v1.0","createSurface":{"surfaceId":"long-summary","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","components":[{"id":"root","component":"Text","text":"` + strings.Repeat("Public result. ", 8192) + `[END]"}],"dataModel":{}}}`
	payload, err := json.Marshal([]string{message})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		flags   []string
		summary string
	}{
		{name: "compatibility fallback", summary: "互动卡片"},
		{name: "explicit fallback", flags: []string{"--summary", "Task result"}, summary: "Task result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			args := append([]string{"message", "send-a2ui-card", "--conversation-id", "group-1", "--content", string(payload)}, tc.flags...)
			if err := runNativeCardUpdate(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.args["summary"] != tc.summary || !reflect.DeepEqual(caller.args["a2uiMessages"], []string{message}) {
				t.Fatal("creation fallback changed the full card content or used protocol JSON")
			}
		})
	}
}
