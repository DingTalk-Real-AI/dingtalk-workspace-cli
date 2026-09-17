// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mail

import (
	"context"
	"fmt"
	"io"
	"maps"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mailWriteCall struct {
	tool string
	args map[string]any
}

type mailWriteContractCaller struct {
	responses map[string][]string
	errors    map[string]error
	calls     []mailWriteCall
}

func (caller *mailWriteContractCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls = append(caller.calls, mailWriteCall{tool: tool, args: maps.Clone(args)})
	if err := caller.errors[tool]; err != nil {
		return nil, err
	}
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, fmt.Errorf("unexpected Mail tool call: %s", tool)
	}
	text := queue[0]
	caller.responses[tool] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*mailWriteContractCaller) Format() string { return "json" }
func (*mailWriteContractCaller) DryRun() bool   { return false }
func (*mailWriteContractCaller) Fields() string { return "" }
func (*mailWriteContractCaller) JQ() string     { return "" }

func runConfirmedMailWriteContract(t *testing.T, caller *mailWriteContractCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(shortcut.Commands()...)
	root.SetArgs(append(args, "--yes", "--format", "json"))
	return root.Execute()
}

func mailDraftReadback(id, subject, body, sender, to, cc string) string {
	return fmt.Sprintf(`{"success":true,"message":{"id":%q,"subject":%q,"markdownBody":%q,"from":{"email":%q},"toRecipients":[{"email":%q}],"ccRecipients":[{"email":%q}]}}`, id, subject, body, sender, to, cc)
}

func TestCrossPlatformCoverageMailWriteHappyPathsUseReceiptThenExactReadback(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		responses map[string][]string
		wantTools []string
	}{
		{
			name: "draft create",
			args: []string{"mail", "+draft-create", "--from", "sender@example.invalid", "--to", "to@example.invalid", "--cc", "cc@example.invalid", "--subject", "subject", "--body", "body"},
			responses: map[string][]string{
				"create_draft":            {`{"success":true,"result":{"message":{"id":"draft-1"}}}`},
				"get_email_by_message_id": {mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid")},
			},
			wantTools: []string{"create_draft", "get_email_by_message_id"},
		},
		{
			name: "draft edit",
			args: []string{"mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-1", "--to", "to@example.invalid", "--cc", "cc@example.invalid", "--subject", "subject", "--body", "body"},
			responses: map[string][]string{
				"update_draft":            {`{"success":true,"result":{"message":{"id":"draft-1"}}}`},
				"get_email_by_message_id": {mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid")},
			},
			wantTools: []string{"update_draft", "get_email_by_message_id"},
		},
		{
			name: "template create core fields",
			args: []string{"mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body"},
			responses: map[string][]string{
				"create_user_message_template": {`{"success":true,"id":"template-1"}`},
				"get_user_message_template":    {`{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"body"}}`},
			},
			wantTools: []string{"create_user_message_template", "get_user_message_template"},
		},
		{
			name: "template update success plus requested id readback",
			args: []string{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--name", "name", "--subject", "subject", "--body", "body"},
			responses: map[string][]string{
				"update_user_message_template": {`{"success":true}`},
				"get_user_message_template":    {`{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"body"}}`},
			},
			wantTools: []string{"update_user_message_template", "get_user_message_template"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: tc.responses, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, tc.args...); err != nil {
				t.Fatalf("write execution failed: %v", err)
			}
			if len(caller.calls) != len(tc.wantTools) {
				t.Fatalf("calls=%d want=%d", len(caller.calls), len(tc.wantTools))
			}
			for index, tool := range tc.wantTools {
				if caller.calls[index].tool != tool {
					t.Fatalf("call[%d]=%s want=%s", index, caller.calls[index].tool, tool)
				}
			}
			if tc.name == "template create core fields" {
				if isDraft, present := caller.calls[0].args["isDraft"].(bool); !present || isDraft {
					t.Fatalf("template create default isDraft=%#v, want explicit false", caller.calls[0].args["isDraft"])
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMailTemplateCreateDraftFlagControlsExactPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
	}{
		{name: "canonical", flag: "--is-draft"},
		{name: "compatibility alias", flag: "--draft"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{
				"create_user_message_template": {`{"success":true,"id":"template-1"}`},
				"get_user_message_template":    {`{"success":true,"id":"template-1","name":"name","isDraft":true,"message":{"subject":"subject","markdownBody":"body"}}`},
			}, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, "mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body", tc.flag); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 2 || caller.calls[0].args["isDraft"] != true {
				t.Fatalf("template create calls=%#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailDraftWritesFailClosedOnReceiptAndReadback(t *testing.T) {
	tests := []struct {
		name      string
		writeBody string
		readBody  string
		writeErr  error
		wantCalls int
	}{
		{name: "empty write body", writeBody: "", wantCalls: 1},
		{name: "missing receipt", writeBody: `{"success":true}`, wantCalls: 1},
		{name: "malformed receipt", writeBody: `{"success":true,"result":{"message":"bad"}}`, wantCalls: 1},
		{name: "missing receipt id", writeBody: `{"success":true,"result":{"message":{"subject":"x"}}}`, wantCalls: 1},
		{name: "missing readback object", writeBody: `{"success":true,"result":{"message":{"id":"draft-1"}}}`, readBody: `{"success":true}`, wantCalls: 2},
		{name: "malformed readback object", writeBody: `{"success":true,"result":{"message":{"id":"draft-1"}}}`, readBody: `{"success":true,"message":"bad"}`, wantCalls: 2},
		{name: "wrong readback id", writeBody: `{"success":true,"result":{"message":{"id":"draft-1"}}}`, readBody: mailDraftReadback("other", "subject", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "request field mismatch", writeBody: `{"success":true,"result":{"message":{"id":"draft-1"}}}`, readBody: mailDraftReadback("draft-1", "other", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "recipient mismatch", writeBody: `{"success":true,"result":{"message":{"id":"draft-1"}}}`, readBody: mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "other@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "write failure", writeErr: fmt.Errorf("write failed"), wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{
				responses: map[string][]string{"create_draft": {tc.writeBody}, "get_email_by_message_id": {tc.readBody}},
				errors:    map[string]error{"create_draft": tc.writeErr},
			}
			err := runConfirmedMailWriteContract(t, caller, "mail", "+draft-create", "--from", "sender@example.invalid", "--to", "to@example.invalid", "--cc", "cc@example.invalid", "--subject", "subject", "--body", "body")
			if err == nil {
				t.Fatal("invalid receipt/readback unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailEditAndTemplateNegativeCallHistory(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		responses map[string][]string
		wantCalls int
	}{
		{name: "draft edit no change", args: []string{"mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-1"}, responses: map[string][]string{}, wantCalls: 0},
		{name: "draft edit wrong receipt id", args: []string{"mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-1", "--subject", "subject"}, responses: map[string][]string{"update_draft": {`{"success":true,"result":{"message":{"id":"other"}}}`}}, wantCalls: 1},
		{name: "template update no change", args: []string{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1"}, responses: map[string][]string{}, wantCalls: 0},
		{name: "template update missing success", args: []string{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--subject", "subject"}, responses: map[string][]string{"update_user_message_template": {`{}`}}, wantCalls: 1},
		{name: "template update wrong readback id", args: []string{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--subject", "subject"}, responses: map[string][]string{"update_user_message_template": {`{"success":true}`}, "get_user_message_template": {`{"success":true,"id":"other","name":"name","message":{"subject":"subject","markdownBody":"body"}}`}}, wantCalls: 2},
		{name: "template update field mismatch", args: []string{"mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--subject", "subject"}, responses: map[string][]string{"update_user_message_template": {`{"success":true}`}, "get_user_message_template": {`{"success":true,"id":"template-1","name":"name","message":{"subject":"other","markdownBody":"body"}}`}}, wantCalls: 2},
		{name: "template create missing id", args: []string{"mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body"}, responses: map[string][]string{"create_user_message_template": {`{"success":true}`}}, wantCalls: 1},
		{name: "template create wrong readback id", args: []string{"mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body"}, responses: map[string][]string{"create_user_message_template": {`{"success":true,"id":"template-1"}`}, "get_user_message_template": {`{"success":true,"id":"other","name":"name","message":{"subject":"subject","markdownBody":"body"}}`}}, wantCalls: 2},
		{name: "template create unverifiable from", args: []string{"mail", "+template-create", "--email", "sender@example.invalid", "--from", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body"}, responses: map[string][]string{}, wantCalls: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: tc.responses, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, tc.args...); err == nil {
				t.Fatal("invalid write unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailCursorPassthroughAndStall(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		tool     string
		response string
	}{
		{name: "thread list", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20", "--cursor", "cursor-1"}, tool: "list_mailbox_threads", response: `{"success":true,"result":{"conversations":[{"id":"thread-1"}],"hasMore":true,"nextCursor":"cursor-2"}}`},
		{name: "user search", args: []string{"mail", "+user-search", "--keyword", "fixture", "--limit", "20", "--cursor", "cursor-1"}, tool: "search_mail_users", response: `{"success":true,"users":[{"id":"user-1","email":"user@example.invalid"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "template list", args: []string{"mail", "+template-list", "--email", "mail@example.invalid", "--limit", "20", "--cursor", "cursor-1"}, tool: "list_user_message_templates", response: `{"success":true,"templates":[{"id":"template-1"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "contact list", args: []string{"mail", "+contact-list", "--email", "mail@example.invalid", "--limit", "20", "--cursor", "cursor-1"}, tool: "list_user_mail_contacts", response: `{"success":true,"contacts":[{"id":"contact-1"}],"hasMore":true,"nextCursor":"cursor-2"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{tc.tool: {tc.response}}, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, tc.args...); err != nil {
				t.Fatalf("execution failed: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].args["cursor"] != "cursor-1" {
				t.Fatalf("cursor was not passed through: %#v", caller.calls)
			}
		})
	}
	caller := &mailWriteContractCaller{
		responses: map[string][]string{"search_mail_users": {`{"success":true,"users":[{"id":"user-1","email":"user@example.invalid"}],"hasMore":true,"nextCursor":"cursor-1"}`}},
		errors:    map[string]error{},
	}
	if err := runConfirmedMailWriteContract(t, caller, "mail", "+user-search", "--keyword", "fixture", "--cursor", "cursor-1"); err == nil {
		t.Fatal("repeated cursor unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageMailValidationRejectsBeforeRemoteCall(t *testing.T) {
	tooManyIDs := make([]string, 101)
	for index := range tooManyIDs {
		tooManyIDs[index] = fmt.Sprintf("message-%d", index)
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "thread zero limit", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "0"}},
		{name: "thread high limit", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "101"}},
		{name: "thread bad start", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20", "--start", "bad"}},
		{name: "thread non utc", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20", "--start", "2026-08-18T10:00:00+08:00"}},
		{name: "thread reversed", args: []string{"mail", "+thread-list", "--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20", "--start", "2026-08-18T10:00:00Z", "--end", "2026-08-18T09:00:00Z"}},
		{name: "user zero limit", args: []string{"mail", "+user-search", "--keyword", "fixture", "--limit", "0"}},
		{name: "template nonnumeric limit", args: []string{"mail", "+template-list", "--email", "mail@example.invalid", "--limit", "bad"}},
		{name: "contact high limit", args: []string{"mail", "+contact-list", "--email", "mail@example.invalid", "--limit", "101"}},
		{name: "messages empty item", args: []string{"mail", "+messages", "--email", "mail@example.invalid", "--ids", "message-1", "--ids", " "}},
		{name: "messages too many", args: []string{"mail", "+messages", "--email", "mail@example.invalid", "--ids", strings.Join(tooManyIDs, ",")}},
		{name: "user empty keyword", args: []string{"mail", "+user-search", "--keyword", " "}},
		{name: "user empty employee number", args: []string{"mail", "+user-search", "--keyword", "fixture", "--employee-no", " "}},
		{name: "message empty id", args: []string{"mail", "+message", "--id", " "}},
		{name: "thread empty id", args: []string{"mail", "+thread", "--id", " "}},
		{name: "folder empty email", args: []string{"mail", "+folder-list", "--email", " "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, tc.args...); err == nil {
				t.Fatal("invalid input unexpectedly succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input reached remote calls: %d", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageMailLegacyStringLimitsStayCompatibleAndStrict(t *testing.T) {
	if err := mailValidatePageSize(mailRuntimeForCoverage(t, ThreadList, nil), "limit", false); err != nil {
		t.Fatalf("omitted optional integer limit should remain valid: %v", err)
	}

	tests := []struct {
		name        string
		declaration *shortcut.Shortcut
		args        []string
		tool        string
		response    string
		wantSize    string
		wantPresent bool
	}{
		{name: "user limit", declaration: &UserSearch, args: []string{"mail", "+user-search", "--keyword", "fixture", "--limit", "1"}, tool: "search_mail_users", response: `{"success":true,"users":[],"hasMore":false,"nextCursor":""}`, wantSize: "1", wantPresent: true},
		{name: "user omitted limit", declaration: &UserSearch, args: []string{"mail", "+user-search", "--keyword", "fixture"}, tool: "search_mail_users", response: `{"success":true,"users":[],"hasMore":false,"nextCursor":""}`},
		{name: "template limit", declaration: &TemplateList, args: []string{"mail", "+template-list", "--email", "mail@example.invalid", "--limit", "100"}, tool: "list_user_message_templates", response: `{"success":true,"templates":[],"hasMore":false,"nextCursor":""}`, wantSize: "100", wantPresent: true},
		{name: "contact limit", declaration: &ContactList, args: []string{"mail", "+contact-list", "--email", "mail@example.invalid", "--limit", "20"}, tool: "list_user_mail_contacts", response: `{"success":true,"contacts":[],"hasMore":false,"nextCursor":""}`, wantSize: "20", wantPresent: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			found := false
			for _, flag := range tc.declaration.Flags {
				if flag.Name != "limit" {
					continue
				}
				found = true
				if flag.Type != shortcut.FlagString || flag.Default != "" {
					t.Fatalf("declaration --limit type/default = %q/%q, want string/empty", flag.Type, flag.Default)
				}
			}
			if !found {
				t.Fatal("declaration missing --limit")
			}
			cmd := corecmd.New(shortcut.FromShortcut(*tc.declaration))
			if flag := cmd.Flags().Lookup("limit"); flag == nil || flag.Value.Type() != "string" || flag.DefValue != "" {
				t.Fatalf("Cobra --limit = %#v, want historical string with empty default", flag)
			}

			caller := &mailWriteContractCaller{responses: map[string][]string{tc.tool: {tc.response}}, errors: map[string]error{}}
			if err := runConfirmedMailWriteContract(t, caller, tc.args...); err != nil {
				t.Fatalf("compatible execution failed: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls=%d, want 1", len(caller.calls))
			}
			gotSize, present := caller.calls[0].args["size"]
			if present != tc.wantPresent || (present && gotSize != tc.wantSize) {
				t.Fatalf("size=%#v present=%v, want %q present=%v", gotSize, present, tc.wantSize, tc.wantPresent)
			}
		})
	}

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		values      map[string]string
	}{
		{name: "user execute guard", declaration: UserSearch, values: map[string]string{"keyword": "fixture", "limit": "bad"}},
		{name: "template execute guard", declaration: TemplateList, values: map[string]string{"email": "mail@example.invalid", "limit": "bad"}},
		{name: "contact execute guard", declaration: ContactList, values: map[string]string{"email": "mail@example.invalid", "limit": "bad"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
			helpers.InitDepsForTest(t, caller)
			rt := mailRuntimeForCoverage(t, tc.declaration, tc.values)
			if err := tc.declaration.Execute(rt); err == nil {
				t.Fatal("direct Execute accepted malformed string limit")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("direct Execute reached %d remote calls", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageMailLegacyStringLimitsRejectEveryInvalidClassWithZeroCalls(t *testing.T) {
	for _, leaf := range []struct {
		name string
		args []string
	}{
		{name: "user", args: []string{"mail", "+user-search", "--keyword", "fixture"}},
		{name: "template", args: []string{"mail", "+template-list", "--email", "mail@example.invalid"}},
		{name: "contact", args: []string{"mail", "+contact-list", "--email", "mail@example.invalid"}},
	} {
		for _, invalid := range []struct {
			name  string
			value string
		}{
			{name: "blank", value: " "},
			{name: "nonnumeric", value: "bad"},
			{name: "zero", value: "0"},
			{name: "negative", value: "-1"},
			{name: "too large", value: "101"},
		} {
			t.Run(leaf.name+"/"+invalid.name, func(t *testing.T) {
				caller := &mailWriteContractCaller{responses: map[string][]string{}, errors: map[string]error{}}
				args := append(append([]string{}, leaf.args...), "--limit", invalid.value)
				if err := runConfirmedMailWriteContract(t, caller, args...); err == nil {
					t.Fatal("invalid legacy string limit unexpectedly succeeded")
				}
				if len(caller.calls) != 0 {
					t.Fatalf("invalid legacy string limit reached %d remote calls", len(caller.calls))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageMailDraftEditOwnNegativeBranches(t *testing.T) {
	goodReceipt := `{"success":true,"result":{"message":{"id":"draft-1"}}}`
	goodReadback := mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid")
	for _, tc := range []struct {
		name      string
		writeBody string
		readBody  string
		errors    map[string]error
		wantCalls int
	}{
		{name: "receipt remote failure", writeBody: `{"success":false}`, wantCalls: 1},
		{name: "receipt missing", writeBody: `{"success":true}`, wantCalls: 1},
		{name: "receipt malformed", writeBody: `{"success":true,"result":{"message":[]}}`, wantCalls: 1},
		{name: "receipt wrong id", writeBody: `{"success":true,"result":{"message":{"id":"other"}}}`, wantCalls: 1},
		{name: "readback transport", writeBody: goodReceipt, errors: map[string]error{"get_email_by_message_id": fmt.Errorf("read failed")}, wantCalls: 2},
		{name: "readback missing", writeBody: goodReceipt, readBody: `{"success":true}`, wantCalls: 2},
		{name: "readback malformed", writeBody: goodReceipt, readBody: `{"success":true,"message":[]}`, wantCalls: 2},
		{name: "readback wrong id", writeBody: goodReceipt, readBody: mailDraftReadback("other", "subject", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "subject mismatch", writeBody: goodReceipt, readBody: mailDraftReadback("draft-1", "other", "body", "sender@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "body mismatch", writeBody: goodReceipt, readBody: mailDraftReadback("draft-1", "subject", "other", "sender@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "from mismatch", writeBody: goodReceipt, readBody: mailDraftReadback("draft-1", "subject", "body", "other@example.invalid", "to@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "to mismatch", writeBody: goodReceipt, readBody: mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "other@example.invalid", "cc@example.invalid"), wantCalls: 2},
		{name: "cc mismatch", writeBody: goodReceipt, readBody: mailDraftReadback("draft-1", "subject", "body", "sender@example.invalid", "to@example.invalid", "other@example.invalid"), wantCalls: 2},
		{name: "happy control", writeBody: goodReceipt, readBody: goodReadback, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{
				responses: map[string][]string{"update_draft": {tc.writeBody}, "get_email_by_message_id": {tc.readBody}},
				errors:    tc.errors,
			}
			if caller.errors == nil {
				caller.errors = map[string]error{}
			}
			err := runConfirmedMailWriteContract(t, caller, "mail", "+draft-edit", "--from", "sender@example.invalid", "--id", "draft-1", "--to", "to@example.invalid", "--cc", "cc@example.invalid", "--subject", "subject", "--body", "body")
			if tc.name == "happy control" {
				if err != nil {
					t.Fatalf("happy control: %v", err)
				}
			} else if err == nil {
				t.Fatal("negative branch unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailTemplateCreateOwnNegativeBranches(t *testing.T) {
	goodReadback := `{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"body"}}`
	for _, tc := range []struct {
		name       string
		createBody string
		readBody   string
		errors     map[string]error
		wantCalls  int
	}{
		{name: "missing success", createBody: `{"id":"template-1"}`, wantCalls: 1},
		{name: "remote failure", createBody: `{"success":false,"id":"template-1"}`, wantCalls: 1},
		{name: "missing id", createBody: `{"success":true}`, wantCalls: 1},
		{name: "readback transport", createBody: `{"success":true,"id":"template-1"}`, errors: map[string]error{"get_user_message_template": fmt.Errorf("read failed")}, wantCalls: 2},
		{name: "readback missing shape", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true}`, wantCalls: 2},
		{name: "readback wrong id", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true,"id":"other","name":"name","message":{"subject":"subject","markdownBody":"body"}}`, wantCalls: 2},
		{name: "name mismatch", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true,"id":"template-1","name":"other","message":{"subject":"subject","markdownBody":"body"}}`, wantCalls: 2},
		{name: "subject mismatch", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true,"id":"template-1","name":"name","message":{"subject":"other","markdownBody":"body"}}`, wantCalls: 2},
		{name: "body mismatch", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"other"}}`, wantCalls: 2},
		{name: "draft malformed", createBody: `{"success":true,"id":"template-1"}`, readBody: `{"success":true,"id":"template-1","name":"name","isDraft":"true","message":{"subject":"subject","markdownBody":"body"}}`, wantCalls: 2},
		{name: "happy control", createBody: `{"success":true,"id":"template-1"}`, readBody: goodReadback, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{"create_user_message_template": {tc.createBody}, "get_user_message_template": {tc.readBody}}, errors: tc.errors}
			if caller.errors == nil {
				caller.errors = map[string]error{}
			}
			err := runConfirmedMailWriteContract(t, caller, "mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body")
			if tc.name == "happy control" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("negative branch unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}

	t.Run("draft mismatch", func(t *testing.T) {
		caller := &mailWriteContractCaller{responses: map[string][]string{
			"create_user_message_template": {`{"success":true,"id":"template-1"}`},
			"get_user_message_template":    {`{"success":true,"id":"template-1","name":"name","isDraft":false,"message":{"subject":"subject","markdownBody":"body"}}`},
		}, errors: map[string]error{}}
		err := runConfirmedMailWriteContract(t, caller, "mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body", "--is-draft")
		if err == nil || len(caller.calls) != 2 {
			t.Fatalf("draft mismatch error=%v calls=%d", err, len(caller.calls))
		}
	})

	for _, flag := range []string{"--is-draft", "--draft"} {
		t.Run("explicit draft missing readback "+flag, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{
				"create_user_message_template": {`{"success":true,"id":"template-1"}`},
				"get_user_message_template":    {goodReadback},
			}, errors: map[string]error{}}
			err := runConfirmedMailWriteContract(t, caller, "mail", "+template-create", "--email", "sender@example.invalid", "--name", "name", "--subject", "subject", "--body", "body", flag)
			if err == nil || len(caller.calls) != 2 {
				t.Fatalf("missing draft readback error=%v calls=%d", err, len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageMailTemplateUpdateOwnNegativeBranches(t *testing.T) {
	goodReadback := `{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"body"}}`
	for _, tc := range []struct {
		name       string
		updateBody string
		readBody   string
		errors     map[string]error
		wantCalls  int
	}{
		{name: "missing success", updateBody: `{}`, wantCalls: 1},
		{name: "remote failure", updateBody: `{"success":false}`, wantCalls: 1},
		{name: "readback transport", updateBody: `{"success":true}`, errors: map[string]error{"get_user_message_template": fmt.Errorf("read failed")}, wantCalls: 2},
		{name: "readback missing shape", updateBody: `{"success":true}`, readBody: `{"success":true}`, wantCalls: 2},
		{name: "readback wrong id", updateBody: `{"success":true}`, readBody: `{"success":true,"id":"other","name":"name","message":{"subject":"subject","markdownBody":"body"}}`, wantCalls: 2},
		{name: "name mismatch", updateBody: `{"success":true}`, readBody: `{"success":true,"id":"template-1","name":"other","message":{"subject":"subject","markdownBody":"body"}}`, wantCalls: 2},
		{name: "subject mismatch", updateBody: `{"success":true}`, readBody: `{"success":true,"id":"template-1","name":"name","message":{"subject":"other","markdownBody":"body"}}`, wantCalls: 2},
		{name: "body mismatch", updateBody: `{"success":true}`, readBody: `{"success":true,"id":"template-1","name":"name","message":{"subject":"subject","markdownBody":"other"}}`, wantCalls: 2},
		{name: "happy control", updateBody: `{"success":true}`, readBody: goodReadback, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &mailWriteContractCaller{responses: map[string][]string{"update_user_message_template": {tc.updateBody}, "get_user_message_template": {tc.readBody}}, errors: tc.errors}
			if caller.errors == nil {
				caller.errors = map[string]error{}
			}
			err := runConfirmedMailWriteContract(t, caller, "mail", "+template-update", "--email", "sender@example.invalid", "--id", "template-1", "--name", "name", "--subject", "subject", "--body", "body")
			if tc.name == "happy control" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("negative branch unexpectedly succeeded")
			}
			if len(caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", len(caller.calls), tc.wantCalls)
			}
		})
	}
}
