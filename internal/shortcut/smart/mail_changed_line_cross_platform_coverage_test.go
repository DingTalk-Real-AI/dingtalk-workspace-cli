// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func smartMailRuntimeForCoverage(t *testing.T, declaration shortcut.Shortcut, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func TestCrossPlatformCoverageSmartMailStrictWireBranches(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{SearchMail, FindMailUser} {
		rt := smartMailRuntimeForCoverage(t, declaration, map[string]string{"query": " "})
		if err := declaration.Validate(rt); err == nil {
			t.Fatal("direct required query validator accepted whitespace")
		}
	}
	if err := smartMailSuccess(map[string]any{}, "mail/test"); err == nil {
		t.Fatal("empty response unexpectedly succeeded")
	}
	if err := smartMailSuccess(map[string]any{"success": false}, "mail/test"); err == nil {
		t.Fatal("explicit failure unexpectedly succeeded")
	}
	if err := smartMailSuccess(map[string]any{"success": "true"}, "mail/test"); err != nil {
		t.Fatalf("string success rejected: %v", err)
	}
	if _, ok := smartMailLookup(map[string]any{"result": "bad"}, "result.items"); ok {
		t.Fatal("non-object lookup unexpectedly succeeded")
	}
	if _, err := smartMailRows(map[string]any{}, "mail/test", "items", "id"); err == nil {
		t.Fatal("rows accepted invalid response")
	}
	if _, err := smartMailRows(map[string]any{"success": true, "items": []any{map[string]any{"name": "missing id"}}}, "mail/test", "items", "id"); err == nil {
		t.Fatal("rows accepted missing identity")
	}

	items, err := smartMailSearchRows(map[string]any{"success": true, "messages": []any{}, "total": 0, "nextCursor": "$"}, "mail/search")
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty search rejected: %v %v", items, err)
	}
	if _, err := smartMailSearchRows(map[string]any{"success": true, "messages": []any{}, "total": -1, "nextCursor": "$"}, "mail/search"); err == nil {
		t.Fatal("negative total unexpectedly succeeded")
	}
	if _, err := smartMailSearchRows(map[string]any{"success": true, "messages": []any{map[string]any{"subject": "missing id"}}, "total": 1, "nextCursor": "$"}, "mail/search"); err == nil {
		t.Fatal("search row without id unexpectedly succeeded")
	}
	if smartMailEmptySearchSentinel(map[string]any{}) {
		t.Fatal("empty object treated as reviewed sentinel")
	}
	if !smartMailEmptySearchSentinel(map[string]any{"tags": nil}) {
		t.Fatal("reviewed null-only sentinel rejected")
	}

	for _, tc := range []struct {
		value any
		want  int
		ok    bool
	}{
		{value: 7, want: 7, ok: true},
		{value: float64(8), want: 8, ok: true},
		{value: 1.5, ok: false},
		{value: "9", want: 9, ok: true},
		{value: []any{}, ok: false},
	} {
		got, ok := smartMailInt(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("smartMailInt(%#v)=(%d,%v), want (%d,%v)", tc.value, got, ok, tc.want, tc.ok)
		}
	}

	for name, data := range map[string]map[string]any{
		"missing prefix":   {},
		"malformed prefix": {"result": []any{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := smartMailPage(data, "mail/page", "result", ""); err == nil {
				t.Fatal("invalid prefixed page unexpectedly succeeded")
			}
		})
	}
	for name, data := range map[string]map[string]any{
		"missing cursor":    {},
		"wrong cursor":      {"nextCursor": 7},
		"wrong hasMore":     {"nextCursor": "cursor-2", "hasMore": []any{}},
		"conflicting true":  {"nextCursor": "", "hasMore": true},
		"conflicting false": {"nextCursor": "cursor-2", "hasMore": false},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := smartMailPage(data, "mail/page", "", ""); err == nil {
				t.Fatal("invalid page unexpectedly succeeded")
			}
		})
	}
	if complete, next, err := smartMailPage(map[string]any{"nextCursor": "$"}, "mail/page", "", ""); err != nil || !complete || next != "" {
		t.Fatalf("terminal page rejected: %v %q %v", complete, next, err)
	}
	if complete, next, err := smartMailPage(map[string]any{"nextCursor": "cursor-2"}, "mail/page", "", ""); err != nil || complete || next != "cursor-2" {
		t.Fatalf("continuing page rejected: %v %q %v", complete, next, err)
	}
	if _, _, err := smartMailPage(map[string]any{"nextCursor": "cursor-1"}, "mail/page", "", "cursor-1"); err == nil {
		t.Fatal("unstated hasMore repeated cursor unexpectedly succeeded")
	}
	if value, ok := smartMailBool("true"); !ok || !value {
		t.Fatal("string true not normalized")
	}
	if value, ok := smartMailBool("false"); !ok || value {
		t.Fatal("string false not normalized")
	}
	if _, ok := smartMailBool(7); ok {
		t.Fatal("invalid boolean unexpectedly normalized")
	}
}

func TestCrossPlatformCoverageSmartMailSchemaAndOutputBoundaries(t *testing.T) {
	assertPanic := func(name string, fn func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("unsupported schema did not panic")
				}
			}()
			fn()
		})
	}
	assertPanic("sensitive collection", func() { smartMailSensitivePaths("unknown") })
	assertPanic("identity collection", func() { smartMailIdentitySchema("unknown") })

	declaration := SearchMail
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	rt := shortcut.RuntimeContextForTest(cmd, declaration)
	if err := smartMailOutputPage(rt, "messages", nil, true, "unexpected-next"); err == nil {
		t.Fatal("invalid terminal pagination unexpectedly stored")
	}
}

func TestCrossPlatformCoverageSmartMailSenderAndProjectionVariants(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
		want string
	}{
		{name: "top string", data: map[string]any{"success": true, "emailAccounts": []any{" mail@example.invalid "}}, want: "mail@example.invalid"},
		{name: "top object", data: map[string]any{"success": true, "emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}}, want: "mail@example.invalid"},
		{name: "result string", data: map[string]any{"success": true, "result": map[string]any{"emailAccounts": []any{"mail@example.invalid"}}}, want: "mail@example.invalid"},
		{name: "result object", data: map[string]any{"success": true, "result": map[string]any{"emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}}}, want: "mail@example.invalid"},
		{name: "data string", data: map[string]any{"success": true, "data": map[string]any{"emailAccounts": []any{"mail@example.invalid"}}}, want: "mail@example.invalid"},
		{name: "data object", data: map[string]any{"success": true, "data": map[string]any{"emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}}}, want: "mail@example.invalid"},
	} {
		t.Run("mailbox "+tc.name, func(t *testing.T) {
			got, err := searchMailMailboxAddresses(tc.data)
			if err != nil || len(got) != 1 || got[0] != tc.want {
				t.Fatalf("addresses=%#v want=%q err=%v", got, tc.want, err)
			}
		})
	}
	for name, data := range map[string]map[string]any{
		"remote failure":   {"success": false},
		"missing":          {"success": true},
		"malformed object": {"success": true, "result": map[string]any{"emailAccounts": []any{map[string]any{"email": 7}}}},
		"bad later item":   {"success": true, "emailAccounts": []any{"mail@example.invalid", 7}},
		"multiple paths":   {"success": true, "emailAccounts": []any{"one@example.invalid"}, "data": map[string]any{"emailAccounts": []any{"two@example.invalid"}}},
	} {
		t.Run("mailbox "+name, func(t *testing.T) {
			if _, err := searchMailMailboxAddresses(data); err == nil {
				t.Fatal("invalid mailbox response unexpectedly succeeded")
			}
		})
	}

	for _, tc := range []struct {
		message map[string]any
		want    any
	}{
		{message: map[string]any{"from": "sender@example.invalid"}, want: "sender@example.invalid"},
		{message: map[string]any{"from": map[string]any{"name": "Sender", "email": "sender@example.invalid"}}, want: "Sender <sender@example.invalid>"},
		{message: map[string]any{"from": map[string]any{"email": "sender@example.invalid"}}, want: "sender@example.invalid"},
		{message: map[string]any{"from": map[string]any{"name": "Sender"}}, want: "Sender"},
		{message: map[string]any{"sender": "sender@example.invalid"}, want: "sender@example.invalid"},
		{message: map[string]any{"from": " ", "sender": "sender@example.invalid"}, want: "sender@example.invalid"},
	} {
		got, err := searchMailFrom(tc.message)
		if err != nil || got != tc.want {
			t.Fatalf("sender=%#v want=%#v err=%v", got, tc.want, err)
		}
	}
	for _, message := range []map[string]any{{}, {"from": " "}, {"from": nil}, {"from": map[string]any{}}, {"from": 7}} {
		if _, err := searchMailFrom(message); err == nil {
			t.Fatal("malformed sender unexpectedly succeeded")
		}
	}
	if got := searchMailFirstString(map[string]any{"id": "  stable-id  "}, "id"); got != "stable-id" {
		t.Fatalf("string normalization=%q", got)
	}
	if got := searchMailFirstAny(map[string]any{"first": " ", "second": " value "}, "first", "second"); got != "value" {
		t.Fatalf("any string normalization=%#v", got)
	}
	if got := searchMailFirstAny(map[string]any{"value": 7}, "value"); got != 7 {
		t.Fatalf("numeric projection=%#v", got)
	}

	projected, err := findMailUserProjection(map[string]any{
		"id": " user-1 ", "email": " user@example.invalid ", "name": " User ", "nickname": " Nick ",
		"employeeNo": " E1 ", "jobTitle": " Engineer ", "workLocation": " HQ ",
	})
	if err != nil || projected["id"] != "user-1" || projected["email"] != "user@example.invalid" || projected["name"] != "User" || projected["employeeNo"] != "E1" {
		t.Fatalf("normalized user projection=%#v err=%v", projected, err)
	}
	if got := findMailUserAny(map[string]any{"first": " ", "second": 7}, "first", "second"); got != 7 {
		t.Fatalf("find user any=%#v", got)
	}
	if got := findMailUserAny(map[string]any{}, "missing"); got != nil {
		t.Fatalf("missing optional value=%#v", got)
	}
}

func TestCrossPlatformCoverageSmartMailLeafFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		responses   map[string]string
		errors      map[string]error
		wantCalls   int
	}{
		{name: "search mailbox malformed", declaration: SearchMail, args: []string{"--query", "fixture"}, responses: map[string]string{"list_user_mailboxes": `{"success":true,"emailAccounts":{}}`}, wantCalls: 1},
		{name: "search mailbox blank string", declaration: SearchMail, args: []string{"--query", "fixture"}, responses: map[string]string{"list_user_mailboxes": `{"success":true,"emailAccounts":[" "]}`}, wantCalls: 1},
		{name: "search mailbox malformed item", declaration: SearchMail, args: []string{"--query", "fixture"}, responses: map[string]string{"list_user_mailboxes": `{"success":true,"emailAccounts":[7]}`}, wantCalls: 1},
		{name: "search mailbox empty", declaration: SearchMail, args: []string{"--query", "fixture"}, responses: map[string]string{"list_user_mailboxes": `{"success":true,"emailAccounts":[]}`}, wantCalls: 1},
		{name: "search rows malformed", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":{}}`}, wantCalls: 1},
		{name: "search sender malformed", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":[]}],"hasMore":false,"nextCursor":""}`}, wantCalls: 1},
		{name: "search page missing", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}]}`}, wantCalls: 1},
		{name: "find rows malformed", declaration: FindMailUser, args: []string{"--query", "fixture"}, responses: map[string]string{"search_mail_users": `{"success":true,"users":{}}`}, wantCalls: 1},
		{name: "find page missing", declaration: FindMailUser, args: []string{"--query", "fixture"}, responses: map[string]string{"search_mail_users": `{"success":true,"users":[{"id":"user-1"}]}`}, wantCalls: 1},
		{name: "triage inbox folders malformed", declaration: TriageMail, args: []string{"--email", "mail@example.invalid"}, responses: map[string]string{"list_folders": `{"success":true,"folders":{}}`}, wantCalls: 1},
		{name: "triage rows malformed", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":{}}`}, wantCalls: 1},
		{name: "triage sender malformed", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":[]}],"hasMore":false,"nextCursor":""}`}, wantCalls: 1},
		{name: "triage page missing", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}]}`}, wantCalls: 1},
		{name: "triage remote failure", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, responses: map[string]string{}, errors: map[string]error{"search_emails": fmt.Errorf("search transport")}, wantCalls: 1},
		{name: "triage mailbox failure", declaration: TriageMail, responses: map[string]string{"list_user_mailboxes": `{"success":true,"emailAccounts":{}}`}, wantCalls: 1},
		{name: "recent rows malformed", declaration: RecentMail, args: []string{"--email", "mail@example.invalid", "--folder", "folder-1"}, responses: map[string]string{"list_mailbox_threads": `{"success":true,"result":{"conversations":{}}}`}, wantCalls: 1},
		{name: "recent sender malformed", declaration: RecentMail, args: []string{"--email", "mail@example.invalid", "--folder", "folder-1"}, responses: map[string]string{"list_mailbox_threads": `{"success":true,"result":{"conversations":[{"id":"thread-1","senders":[{}]}]}}`}, wantCalls: 1},
		{name: "recent page malformed", declaration: RecentMail, args: []string{"--email", "mail@example.invalid", "--folder", "folder-1"}, responses: map[string]string{"list_mailbox_threads": `{"success":true,"result":{"conversations":[{"id":"thread-1","senders":[{"email":"sender@example.invalid"}]}],"nextCursor":7}}`}, wantCalls: 1},
		{name: "unread rows malformed", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid"}, responses: map[string]string{"search_emails": `{"success":true,"messages":{}}`}, wantCalls: 1},
		{name: "unread sender malformed", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":[]}],"hasMore":false,"nextCursor":""}`}, wantCalls: 1},
		{name: "unread page missing", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid"}, responses: map[string]string{"search_emails": `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}]}`}, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: tc.responses, errors: tc.errors}
			if err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("invalid leaf response unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageSmartMailInboxAndSenderFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message map[string]any
		want    any
		wantErr bool
	}{
		{name: "name and address", message: map[string]any{"senders": []any{map[string]any{"name": "Sender", "email": "sender@example.invalid"}}}, want: []string{"Sender <sender@example.invalid>"}},
		{name: "address only", message: map[string]any{"senders": []any{map[string]any{"email": "sender@example.invalid"}}}, want: []string{"sender@example.invalid"}},
		{name: "name only", message: map[string]any{"senders": []any{map[string]any{"name": "Sender"}}}, want: []string{"Sender"}},
		{name: "wrong sender collection", message: map[string]any{"senders": "bad"}, wantErr: true},
		{name: "fallback sender", message: map[string]any{"senders": []any{}, "from": "sender@example.invalid"}, want: "sender@example.invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := recentMailSenders(tc.message)
			if tc.wantErr && err == nil {
				t.Fatal("malformed senders unexpectedly succeeded")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid senders failed: %v", err)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("senders=%#v want=%#v", got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		responses   map[string]string
		wantErr     bool
		wantCalls   int
	}{
		{
			name: "triage resolves mailbox and inbox", declaration: TriageMail,
			responses: map[string]string{
				"list_user_mailboxes": `{"success":true,"emailAccounts":[{"email":"mail@example.invalid"}]}`,
				"list_folders":        `{"success":true,"folders":[{"id":"folder-1","displayName":"Inbox"}]}`,
				"search_emails":       `{"success":true,"messages":[],"total":0,"hasMore":false,"nextCursor":""}`,
			}, wantCalls: 3,
		},
		{
			name: "search resolves wrapped string mailbox", declaration: SearchMail, args: []string{"--query", "fixture"},
			responses: map[string]string{
				"list_user_mailboxes": `{"success":true,"result":{"emailAccounts":["mail@example.invalid"]}}`,
				"search_emails":       `{"success":true,"messages":[],"total":0,"hasMore":false,"nextCursor":""}`,
			}, wantCalls: 2,
		},
		{
			name: "triage resolves string mailbox and inbox", declaration: TriageMail,
			responses: map[string]string{
				"list_user_mailboxes": `{"success":true,"emailAccounts":["mail@example.invalid"]}`,
				"list_folders":        `{"success":true,"folders":[{"id":"folder-1","displayName":"Inbox"}]}`,
				"search_emails":       `{"success":true,"messages":[],"total":0,"hasMore":false,"nextCursor":""}`,
			}, wantCalls: 3,
		},
		{
			name: "recent resolves inbox", declaration: RecentMail, args: []string{"--email", "mail@example.invalid"},
			responses: map[string]string{
				"list_folders":         `{"success":true,"folders":[{"id":"folder-1","displayName":"Inbox"}]}`,
				"list_mailbox_threads": `{"success":true,"result":{"conversations":[],"hasMore":false,"nextCursor":""}}`,
			}, wantCalls: 2,
		},
		{
			name: "inbox not found", declaration: TriageMail, args: []string{"--email", "mail@example.invalid"},
			responses: map[string]string{"list_folders": `{"success":true,"folders":[{"id":"folder-1","displayName":"Archive"}]}`},
			wantErr:   true, wantCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: tc.responses}
			err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...)
			if tc.wantErr && err == nil {
				t.Fatal("invalid inbox resolution unexpectedly succeeded")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid inbox resolution failed: %v", err)
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}

	for _, tc := range []struct {
		name     string
		response string
		reason   string
	}{
		{name: "blank sender", response: `{"success":true,"messages":[{"id":"message-1","from":" "}],"hasMore":false,"nextCursor":""}`, reason: "malformed_sender"},
		{name: "missing sender", response: `{"success":true,"messages":[{"id":"message-1"}],"hasMore":false,"nextCursor":""}`, reason: "missing_sender"},
	} {
		t.Run(tc.name+" typed reason", func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: map[string]string{"search_emails": tc.response}}
			err := runSmartMailDeclaration(t, SearchMail, caller, "--email", "mail@example.invalid", "--query", "fixture")
			typed, ok := err.(*apperrors.Error)
			if err == nil || !ok || typed.Reason != tc.reason || len(caller.calls) != 1 {
				reason := ""
				if ok {
					reason = typed.Reason
				}
				t.Fatalf("err=%v reason=%q calls=%d", err, reason, len(caller.calls))
			}
		})
	}
}
