// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mail

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func runMailDeclarationForCoverage(t *testing.T, declaration shortcut.Shortcut, caller *mailWriteContractCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func mailRuntimeForCoverage(t *testing.T, declaration shortcut.Shortcut, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, declaration)
}

func TestCrossPlatformCoverageMailReadLeavesExerciseExactIdentityAndMailboxBranches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		responses map[string][]string
		errors    map[string]error
		wantErr   bool
		wantCalls int
	}{
		{name: "message explicit mailbox", args: []string{"mail", "+message", "--email", "mail@example.invalid", "--id", "message-1"}, responses: map[string][]string{"get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`}}, wantCalls: 1},
		{name: "message auto mailbox", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"emailAccounts":[{"email":"mail@example.invalid"}]}`}, "get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`}}, wantCalls: 2},
		{name: "message auto nested string mailbox", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"result":{"emailAccounts":["mail@example.invalid"]}}`}, "get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`}}, wantCalls: 2},
		{name: "mailbox transport", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{}, errors: map[string]error{"list_user_mailboxes": fmt.Errorf("mailbox transport")}, wantErr: true, wantCalls: 1},
		{name: "mailbox missing collection", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true}`}}, wantErr: true, wantCalls: 1},
		{name: "mailbox empty", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"emailAccounts":[]}`}}, wantErr: true, wantCalls: 1},
		{name: "mailbox missing identity", args: []string{"mail", "+message", "--id", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"emailAccounts":[{"name":"mailbox"}]}`}}, wantErr: true, wantCalls: 1},
		{name: "message transport", args: []string{"mail", "+message", "--email", "mail@example.invalid", "--id", "message-1"}, responses: map[string][]string{}, errors: map[string]error{"get_email_by_message_id": fmt.Errorf("message transport")}, wantErr: true, wantCalls: 1},
		{name: "message malformed", args: []string{"mail", "+message", "--email", "mail@example.invalid", "--id", "message-1"}, responses: map[string][]string{"get_email_by_message_id": {`{"success":true,"message":[]}`}}, wantErr: true, wantCalls: 1},
		{name: "message wrong identity", args: []string{"mail", "+message", "--email", "mail@example.invalid", "--id", "message-1"}, responses: map[string][]string{"get_email_by_message_id": {`{"success":true,"message":{"id":"other"}}`}}, wantErr: true, wantCalls: 1},
		{name: "messages ordered", args: []string{"mail", "+messages", "--email", "mail@example.invalid", "--ids", "message-1,message-2"}, responses: map[string][]string{"get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`, `{"success":true,"message":{"id":"message-2"}}`}}, wantCalls: 2},
		{name: "messages auto data mailbox", args: []string{"mail", "+messages", "--ids", "message-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"data":{"emailAccounts":[{"email":"mail@example.invalid"}]}}`}, "get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`}}, wantCalls: 2},
		{name: "messages second fails", args: []string{"mail", "+messages", "--email", "mail@example.invalid", "--ids", "message-1,message-2"}, responses: map[string][]string{"get_email_by_message_id": {`{"success":true,"message":{"id":"message-1"}}`, `{"success":true,"message":{"id":"other"}}`}}, wantErr: true, wantCalls: 2},
		{name: "messages mailbox resolution fails", args: []string{"mail", "+messages", "--ids", "message-1"}, responses: map[string][]string{}, errors: map[string]error{"list_user_mailboxes": fmt.Errorf("mailbox transport")}, wantErr: true, wantCalls: 1},
		{name: "thread exact", args: []string{"mail", "+thread", "--email", "mail@example.invalid", "--id", "thread-1"}, responses: map[string][]string{"get_thread": {`{"success":true,"conversation":{"id":"thread-1"}}`}}, wantCalls: 1},
		{name: "thread auto result mailbox", args: []string{"mail", "+thread", "--id", "thread-1"}, responses: map[string][]string{"list_user_mailboxes": {`{"success":true,"result":{"emailAccounts":[{"email":"mail@example.invalid"}]}}`}, "get_thread": {`{"success":true,"conversation":{"id":"thread-1"}}`}}, wantCalls: 2},
		{name: "thread transport", args: []string{"mail", "+thread", "--email", "mail@example.invalid", "--id", "thread-1"}, responses: map[string][]string{}, errors: map[string]error{"get_thread": fmt.Errorf("thread transport")}, wantErr: true, wantCalls: 1},
		{name: "thread mailbox resolution fails", args: []string{"mail", "+thread", "--id", "thread-1"}, responses: map[string][]string{}, errors: map[string]error{"list_user_mailboxes": fmt.Errorf("mailbox transport")}, wantErr: true, wantCalls: 1},
		{name: "thread missing result", args: []string{"mail", "+thread", "--email", "mail@example.invalid", "--id", "thread-1"}, responses: map[string][]string{"get_thread": {`{"success":true}`}}, wantErr: true, wantCalls: 1},
		{name: "thread wrong identity", args: []string{"mail", "+thread", "--email", "mail@example.invalid", "--id", "thread-1"}, responses: map[string][]string{"get_thread": {`{"success":true,"conversation":{"id":"other"}}`}}, wantErr: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: tc.responses, errors: tc.errors}
			if caller.responses == nil {
				caller.responses = map[string][]string{}
			}
			if caller.errors == nil {
				caller.errors = map[string]error{}
			}
			err := runConfirmedMailWriteContract(t, caller, tc.args...)
			if tc.wantErr && err == nil {
				t.Fatal("invalid read unexpectedly succeeded")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid read failed: %v", err)
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}

	if err := mailValidateMessageIDs(nil); err == nil {
		t.Fatal("empty ids unexpectedly valid")
	}
	caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
	helpers.InitDepsForTest(t, caller)
	rt := mailRuntimeForCoverage(t, Messages, map[string]string{"email": "mail@example.invalid", "ids": " "})
	if err := Messages.Execute(rt); err == nil {
		t.Fatal("defensive empty-id guard unexpectedly succeeded")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("defensive empty-id guard made %d calls", len(caller.calls))
	}
}

func TestCrossPlatformCoverageMailMailboxResolverSupportsReviewedShapesAndFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{
			name: "top string",
			data: map[string]any{"success": true, "emailAccounts": []any{" mail@example.invalid "}},
		},
		{
			name: "top object",
			data: map[string]any{"success": true, "emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}},
		},
		{
			name: "result string",
			data: map[string]any{"success": true, "result": map[string]any{"emailAccounts": []any{"mail@example.invalid"}}},
		},
		{
			name: "result object",
			data: map[string]any{"success": true, "result": map[string]any{"emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}}},
		},
		{
			name: "data string",
			data: map[string]any{"success": true, "data": map[string]any{"emailAccounts": []any{"mail@example.invalid"}}},
		},
		{
			name: "data object",
			data: map[string]any{"success": true, "data": map[string]any{"emailAccounts": []any{map[string]any{"email": "mail@example.invalid"}}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mailMailboxAddresses(tc.data)
			if err != nil || len(got) != 1 || got[0] != "mail@example.invalid" {
				t.Fatalf("addresses=%#v err=%v", got, err)
			}
		})
	}

	for name, data := range map[string]map[string]any{
		"remote failure":       {"success": false},
		"missing collection":   {"success": true},
		"malformed collection": {"success": true, "emailAccounts": map[string]any{}},
		"blank string":         {"success": true, "emailAccounts": []any{" "}},
		"missing object email": {"success": true, "emailAccounts": []any{map[string]any{"name": "mailbox"}}},
		"wrong object email":   {"success": true, "emailAccounts": []any{map[string]any{"email": 7}}},
		"bad later item":       {"success": true, "emailAccounts": []any{"mail@example.invalid", 7}},
		"multiple paths":       {"success": true, "emailAccounts": []any{"one@example.invalid"}, "result": map[string]any{"emailAccounts": []any{"two@example.invalid"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mailMailboxAddresses(data); err == nil {
				t.Fatal("invalid mailbox response unexpectedly succeeded")
			}
		})
	}
}

func TestCrossPlatformCoverageMailListLeavesFailAtOwningBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		tool      string
		response  string
		remoteErr bool
		wantErr   bool
	}{
		{name: "folder happy", args: []string{"mail", "+folder-list", "--email", "mail@example.invalid", "--folder", "parent-1"}, tool: "list_folders", response: `{"success":true,"folders":[{"id":"folder-1","displayName":"Inbox"}]}`},
		{name: "folder remote", args: []string{"mail", "+folder-list", "--email", "mail@example.invalid"}, tool: "list_folders", remoteErr: true, wantErr: true},
		{name: "folder malformed", args: []string{"mail", "+folder-list", "--email", "mail@example.invalid"}, tool: "list_folders", response: `{"success":true,"folders":{}}`, wantErr: true},
		{name: "tag happy", args: []string{"mail", "+tag-list", "--email", "mail@example.invalid"}, tool: "list_tags", response: `{"success":true,"tags":[{"id":"tag-1"}]}`},
		{name: "tag remote", args: []string{"mail", "+tag-list", "--email", "mail@example.invalid"}, tool: "list_tags", remoteErr: true, wantErr: true},
		{name: "tag malformed", args: []string{"mail", "+tag-list", "--email", "mail@example.invalid"}, tool: "list_tags", response: `{"success":true,"tags":{}}`, wantErr: true},
		{name: "thread malformed rows", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20"}, tool: "list_mailbox_threads", response: `{"success":true,"result":{"conversations":{},"nextCursor":""}}`, wantErr: true},
		{name: "thread missing page", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20"}, tool: "list_mailbox_threads", response: `{"success":true,"result":{"conversations":[{"id":"thread-1"}]}}`, wantErr: true},
		{name: "user malformed rows", args: []string{"mail", "+user-search", "--keyword", "fixture"}, tool: "search_mail_users", response: `{"success":true,"users":{}}`, wantErr: true},
		{name: "user missing page", args: []string{"mail", "+user-search", "--keyword", "fixture"}, tool: "search_mail_users", response: `{"success":true,"users":[{"id":"user-1"}]}`, wantErr: true},
		{name: "template malformed rows", args: []string{"mail", "+template-list", "--email", "mail@example.invalid", "--limit", "20"}, tool: "list_user_message_templates", response: `{"success":true,"templates":{}}`, wantErr: true},
		{name: "template missing page", args: []string{"mail", "+template-list", "--email", "mail@example.invalid", "--limit", "20"}, tool: "list_user_message_templates", response: `{"success":true,"templates":[{"id":"template-1"}]}`, wantErr: true},
		{name: "contact malformed rows", args: []string{"mail", "+contact-list", "--email", "mail@example.invalid", "--limit", "20"}, tool: "list_user_mail_contacts", response: `{"success":true,"contacts":{}}`, wantErr: true},
		{name: "contact missing page", args: []string{"mail", "+contact-list", "--email", "mail@example.invalid", "--limit", "20"}, tool: "list_user_mail_contacts", response: `{"success":true,"contacts":[{"id":"contact-1"}]}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{tc.tool: {tc.response}}, errors: map[string]error{}}
			if tc.remoteErr {
				caller.errors[tc.tool] = fmt.Errorf("remote failure")
			}
			err := runConfirmedMailWriteContract(t, caller, tc.args...)
			if tc.wantErr && err == nil {
				t.Fatal("invalid list response unexpectedly succeeded")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid list response failed: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageMailStrictHelpersCoverMalformedWireAndPagination(t *testing.T) {
	assertPanic := func(name string, fn func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			deferred := false
			defer func() {
				if recover() == nil {
					t.Fatal("unsupported declaration did not panic")
				}
				deferred = true
			}()
			fn()
			_ = deferred
		})
	}
	assertPanic("sensitive collection", func() { mailCollectionSensitivePaths("unknown") })
	assertPanic("identity collection", func() { mailCollectionIdentitySchema("unknown") })

	if _, err := mailRequireObject(map[string]any{}, "mail/get", "message"); err == nil {
		t.Fatal("object accepted missing success")
	}
	if object, err := mailRequireObject(map[string]any{"success": true, "message": map[string]any{"id": "message-1"}}, "mail/get", "message"); err != nil || object["id"] != "message-1" {
		t.Fatalf("valid object rejected: %#v %v", object, err)
	}
	if _, _, err := mailProjectValue("mail/users", "email", 7); err == nil {
		t.Fatal("numeric email unexpectedly projected")
	}
	if _, include, err := mailProjectValue("mail/users", "userId", " "); err != nil || include {
		t.Fatalf("blank user id include=%v err=%v", include, err)
	}
	if _, include, err := mailProjectValue("mail/test", "name", nil); err != nil || include {
		t.Fatalf("nil projection include=%v err=%v", include, err)
	}
	if _, include, err := mailProjectValue("mail/test", "name", " "); err != nil || include {
		t.Fatalf("blank projection include=%v err=%v", include, err)
	}
	if value, include, err := mailProjectValue("mail/test", "rank", 7); err != nil || !include || value != 7 {
		t.Fatalf("numeric projection value=%v include=%v err=%v", value, include, err)
	}

	for name, data := range map[string]map[string]any{
		"missing prefix":   {},
		"malformed prefix": {"result": []any{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := mailPage(data, "mail/page", "result", ""); err == nil {
				t.Fatal("invalid prefixed page unexpectedly succeeded")
			}
		})
	}
	for name, data := range map[string]map[string]any{
		"missing cursor":       {},
		"malformed cursor":     {"nextCursor": 7},
		"malformed hasMore":    {"nextCursor": "cursor-2", "hasMore": []any{}},
		"conflicting terminal": {"nextCursor": "cursor-2", "hasMore": false},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := mailPage(data, "mail/page", "", ""); err == nil {
				t.Fatal("invalid page unexpectedly succeeded")
			}
		})
	}
	if complete, next, err := mailPage(map[string]any{"nextCursor": "cursor-2"}, "mail/page", "", ""); err != nil || complete || next != "cursor-2" {
		t.Fatalf("continuing cursor rejected: %v %q %v", complete, next, err)
	}
	if _, _, err := mailPage(map[string]any{"nextCursor": "cursor-1"}, "mail/page", "", "cursor-1"); err == nil {
		t.Fatal("unstated hasMore repeated cursor unexpectedly succeeded")
	}
	if value, ok := mailBool("true"); !ok || !value {
		t.Fatal("string true not normalized")
	}
	if _, ok := mailBool(7); ok {
		t.Fatal("invalid boolean unexpectedly normalized")
	}
	if _, ok := mailLookup(map[string]any{"result": "bad"}, "result.items"); ok {
		t.Fatal("non-object intermediate lookup unexpectedly succeeded")
	}
	declaration := UserSearch
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	rt := shortcut.RuntimeContextForTest(cmd, declaration)
	if err := mailOutputPage(rt, "users", nil, true, "unexpected-next"); err == nil {
		t.Fatal("invalid terminal pagination unexpectedly stored")
	}
}

func TestCrossPlatformCoverageMailWriteVerificationHelpersAndDryRun(t *testing.T) {
	for name, message := range map[string]map[string]any{
		"missing addresses":        {},
		"wrong address collection": {"toRecipients": "bad"},
		"wrong address item":       {"toRecipients": []any{7}},
		"blank address item":       {"toRecipients": []any{map[string]any{"email": " "}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mailReadAddresses(message, "mail/verify", "toRecipients"); err == nil {
				t.Fatal("malformed recipients unexpectedly succeeded")
			}
		})
	}
	addresses, err := mailReadAddresses(map[string]any{"toRecipients": []any{" A@example.invalid ", map[string]any{"address": "b@example.invalid"}}}, "mail/verify", "toRecipients")
	if err != nil || len(addresses) != 2 {
		t.Fatalf("valid recipients rejected: %v %v", addresses, err)
	}
	if err := mailVerifyAddresses(map[string]any{"toRecipients": []any{"a@example.invalid"}}, "mail/verify", "toRecipients", []string{"a@example.invalid", "b@example.invalid"}); err == nil {
		t.Fatal("recipient length mismatch accepted")
	}
	if err := mailVerifyAddresses(map[string]any{"toRecipients": []any{"b@example.invalid"}}, "mail/verify", "toRecipients", []string{"a@example.invalid"}); err == nil {
		t.Fatal("recipient value mismatch accepted")
	}
	if err := mailVerifyAddresses(map[string]any{}, "mail/verify", "toRecipients", []string{"a@example.invalid"}); err == nil {
		t.Fatal("recipient parser error was not propagated")
	}
	if _, err := mailDraftMessage(map[string]any{"success": false}, "mail/create"); err == nil {
		t.Fatal("failed draft receipt unexpectedly succeeded")
	}
	for name, message := range map[string]map[string]any{
		"missing sender": {},
		"wrong sender":   {"from": []any{}},
		"blank sender":   {"from": " "},
	} {
		t.Run(name, func(t *testing.T) {
			if err := mailVerifySender(message, "mail/verify", "sender@example.invalid"); err == nil {
				t.Fatal("malformed sender unexpectedly accepted")
			}
		})
	}
	if err := mailVerifySender(map[string]any{"from": "sender@example.invalid"}, "mail/verify", "SENDER@example.invalid"); err != nil {
		t.Fatalf("case-insensitive sender rejected: %v", err)
	}

	for _, args := range [][]string{
		{"mail", "+draft-create", "--from", "sender@example.invalid", "--subject", "subject", "--dry-run"},
		{"mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-1", "--subject", "subject", "--dry-run"},
		{"mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body", "--dry-run"},
		{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--subject", "subject", "--dry-run"},
	} {
		caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
		if err := runConfirmedMailWriteContract(t, caller, args...); err != nil {
			t.Fatalf("dry-run failed: %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("dry-run made %d remote calls", len(caller.calls))
		}
	}

	for _, declaration := range []shortcut.Shortcut{DraftEdit, TemplateUpdate} {
		caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
		helpers.InitDepsForTest(t, caller)
		values := map[string]string{"email": "sender@example.invalid", "id": "template-1"}
		if declaration.Command == "+draft-edit" {
			values = map[string]string{"from": "sender@example.invalid", "id": "draft-1"}
		}
		rt := mailRuntimeForCoverage(t, declaration, values)
		if err := declaration.Execute(rt); err == nil {
			t.Fatal("execute-level no-change guard unexpectedly succeeded")
		}
		if len(caller.calls) != 0 {
			t.Fatalf("no-change guard made %d calls", len(caller.calls))
		}
	}

	for _, tc := range []struct {
		name     string
		readback string
	}{
		{name: "body mismatch", readback: mailDraftReadback("draft-1", "subject", "other", "sender@example.invalid", "to@example.invalid", "cc@example.invalid")},
		{name: "sender mismatch", readback: mailDraftReadback("draft-1", "subject", "body", "other@example.invalid", "to@example.invalid", "cc@example.invalid")},
		{name: "cc mismatch", readback: mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "to@example.invalid", "other@example.invalid")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{
				"create_draft":            {`{"success":true,"result":{"message":{"id":"draft-1"}}}`},
				"get_email_by_message_id": {tc.readback},
			}, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, "mail", "+draft-create", "--from", "sender@example.invalid", "--to", "to@example.invalid", "--cc", "cc@example.invalid", "--subject", "subject", "--body", "body"); err == nil {
				t.Fatal("draft verification mismatch unexpectedly succeeded")
			}
		})
	}

	caller := &mailWriteContractCaller{responses: map[string][]string{
		"create_user_message_template": {`{"success":true,"id":"template-1"}`},
		"get_user_message_template":    {`{"success":"false"}`},
	}, errors: map[string]error{}}
	if err := runConfirmedMailWriteContract(t, caller, "mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body"); err == nil {
		t.Fatal("failed template readback unexpectedly succeeded")
	}

	for _, tc := range []struct {
		declaration shortcut.Shortcut
		values      map[string]string
	}{
		{declaration: Message, values: map[string]string{"id": " "}},
		{declaration: FolderList, values: map[string]string{"email": " "}},
	} {
		rt := mailRuntimeForCoverage(t, tc.declaration, tc.values)
		if err := tc.declaration.Validate(rt); err == nil {
			t.Fatal("direct required-text validator accepted whitespace")
		}
	}
	rt := mailRuntimeForCoverage(t, UserSearch, map[string]string{})
	if err := UserSearch.Validate(rt); err == nil {
		t.Fatal("direct user-search validator accepted missing identities")
	}
}

func TestCrossPlatformCoverageMailTemplateReadbackRejectsBusinessFailure(t *testing.T) {
	caller := &mailWriteContractCaller{responses: map[string][]string{
		"get_user_message_template": {`{"id":"template-1"}`},
	}, errors: map[string]error{}}
	helpers.InitDepsForTest(t, caller)
	rt := mailRuntimeForCoverage(t, TemplateCreate, map[string]string{})
	if _, err := mailReadTemplate(rt, "mail@example.invalid", "template-1"); err == nil {
		t.Fatal("failed template detail response unexpectedly succeeded")
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls=%d want=1", len(caller.calls))
	}
}
