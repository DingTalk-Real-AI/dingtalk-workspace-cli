// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMessagesSendA2UIDeliveryOptions(t *testing.T) {
	for _, target := range [][]string{
		{"--group", "cid"},
		{"--open-dingtalk-id", "DAAAAAAAAAAAiE"},
	} {
		for _, tc := range []struct {
			name    string
			flags   []string
			summary string
			forward bool
			invalid bool
		}{
			{name: "readable default", summary: "互动卡片"},
			{name: "explicit shareable card", flags: []string{"--card-summary", "Weekly report", "--support-forward"}, summary: "Weekly report", forward: true},
			{name: "explicit nonshareable card", flags: []string{"--card-summary", "Task result", "--support-forward=false"}, summary: "Task result"},
			{name: "explicit empty summary", flags: []string{"--card-summary", ""}, invalid: true},
			{name: "explicit whitespace summary", flags: []string{"--card-summary", " \t\n"}, invalid: true},
			{name: "invalid forwarding", flags: []string{"--support-forward=invalid"}, invalid: true},
		} {
			t.Run(target[0]+"/"+tc.name, func(t *testing.T) {
				f := &larkAlignmentCaller{}
				args := append([]string{"+messages-send", "--a2ui-messages", `["{\"version\":\"v1.0\"}"]`, "--yes"}, target...)
				args = append(args, tc.flags...)
				_, err := runChatParity(t, f, args...)
				if tc.invalid {
					if err == nil || len(f.calls) != 0 {
						t.Fatalf("err=%v calls=%v", err, f.calls)
					}
					return
				}
				if err != nil || len(f.calls) != 1 || f.calls[0].tool != "create_and_send_a2ui_card" {
					t.Fatalf("err=%v calls=%v", err, f.calls)
				}
				got := f.calls[0].args
				if got["summary"] != tc.summary || got["supportForward"] != tc.forward || got["cardProperty"] != nil {
					t.Fatalf("delivery metadata=%v", got)
				}
			})
		}
	}
	t.Run("invalid summary is rejected by the execution boundary", func(t *testing.T) {
		f := &larkAlignmentCaller{}
		rt := optimizationRuntime(t, f, MessagesSend, map[string]string{"a2ui-messages": `["message"]`, "card-summary": " "})
		if err := executeMessagesSendUserShare(rt, "cid", "", "a2ui"); err == nil || len(f.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, f.calls)
		}
	})
}

func TestCrossPlatformCoverageMessagesSendA2UIOptionsDoNotAffectText(t *testing.T) {
	for _, flag := range []string{"--support-forward=true", "--support-forward=false", "--card-summary=", "--biz-card-id=", "--request-id="} {
		t.Run(flag, func(t *testing.T) {
			f := &larkAlignmentCaller{}
			_, err := runChatParity(t, f, "+messages-send", "--group", "cid", "--text", "Ordinary message", flag, "--yes")
			if err == nil || len(f.calls) != 0 {
				t.Fatalf("card-only option reached text transport: err=%v calls=%v", err, f.calls)
			}
		})
	}
	f := &larkAlignmentCaller{}
	if _, err := runChatParity(t, f, "+messages-send", "--group", "cid", "--text", "Ordinary message", "--yes"); err != nil || len(f.calls) != 1 {
		t.Fatalf("ordinary text failed: err=%v calls=%v", err, f.calls)
	}
	for _, name := range []string{"summary", "supportForward", "a2uiMessages"} {
		if _, exists := f.calls[0].args[name]; exists {
			t.Fatalf("card field %s leaked into ordinary text: %v", name, f.calls)
		}
	}
}

func TestCrossPlatformCoverageMessagesSendA2UISummaryPreservesLongContent(t *testing.T) {
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
		{name: "explicit fallback", flags: []string{"--card-summary", "Task result"}, summary: "Task result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{}
			args := append([]string{"+messages-send", "--group", "cid", "--a2ui-messages", string(payload), "--yes"}, tc.flags...)
			if _, err := runChatParity(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 || caller.calls[0].args["summary"] != tc.summary || !reflect.DeepEqual(caller.calls[0].args["a2uiMessages"], []string{message}) {
				t.Fatal("creation fallback changed the full card content or used protocol JSON")
			}
		})
	}
}
