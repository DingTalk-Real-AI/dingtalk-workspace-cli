// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// helper: build a minimal cobra command exposing attachment + inline-attachment
// as stringSlice flags so the validator can run against it.
func newMailSendStub() *cobra.Command {
	cmd := &cobra.Command{Use: "send"}
	cmd.Flags().StringSlice("attachment", nil, "attachment paths")
	cmd.Flags().StringSlice("inline-attachment", nil, "inline attachment paths")
	return cmd
}

func TestValidateMailAttachmentFiles_AcceptsRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.pdf")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd := newMailSendStub()
	if err := cmd.Flags().Set("attachment", path); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := validateMailAttachmentFiles(cmd, nil); err != nil {
		t.Fatalf("expected nil for existing file, got %v", err)
	}
}

func TestValidateMailAttachmentFiles_RejectsMissingFile(t *testing.T) {
	cmd := newMailSendStub()
	if err := cmd.Flags().Set("attachment", "/tmp/this_does_not_exist_xyz123.pdf"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := validateMailAttachmentFiles(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error for missing attachment")
	}
	if !strings.Contains(err.Error(), "cannot read attachment") {
		t.Fatalf("unexpected error wording: %v", err)
	}
}

func TestValidateMailAttachmentFiles_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	cmd := newMailSendStub()
	if err := cmd.Flags().Set("attachment", dir); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := validateMailAttachmentFiles(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error for directory attachment")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("unexpected error wording: %v", err)
	}
}

func TestValidateMailAttachmentFiles_RejectsMissingInline(t *testing.T) {
	cmd := newMailSendStub()
	if err := cmd.Flags().Set("inline-attachment", "/tmp/no_such_image_zyx999.png"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := validateMailAttachmentFiles(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error for missing inline attachment")
	}
	if !strings.Contains(err.Error(), "cannot read inline attachment") {
		t.Fatalf("unexpected error wording: %v", err)
	}
}

func TestValidateMailAttachmentFiles_NoFlagsRegistered(t *testing.T) {
	// e.g. a command without either flag (defensive) should not blow up.
	cmd := &cobra.Command{Use: "noop"}
	if err := validateMailAttachmentFiles(cmd, nil); err != nil {
		t.Fatalf("expected nil when flags absent, got %v", err)
	}
}

// ── search_mail_users email fallback ──────────────────────────

type fakeMailboxRunner struct {
	called  bool
	gotTool string
	resp    map[string]any
	err     error
}

func (f *fakeMailboxRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	f.called = true
	f.gotTool = inv.Tool
	if f.err != nil {
		return executor.Result{}, f.err
	}
	return executor.Result{Invocation: inv, Response: f.resp}, nil
}

func newMailUserSearchStub() *cobra.Command {
	cmd := &cobra.Command{Use: "search"}
	cmd.Flags().String("email", "", "mailbox")
	cmd.Flags().String("keyword", "", "keyword")
	return cmd
}

func TestMailUserSearchEmailFallback_NoOpWhenEmailProvided(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{
		"emailAccounts": []any{
			map[string]any{"email": "first@example.com"},
		},
	}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	if err := cmd.Flags().Set("email", "user@example.com"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := pre(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if runner.called {
		t.Fatal("runner should not be invoked when --email is set")
	}
	if got, _ := cmd.Flags().GetString("email"); got != "user@example.com" {
		t.Fatalf("email mutated: got %q", got)
	}
}

func TestMailUserSearchEmailFallback_FillsFromFirstMailbox(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{
		"emailAccounts": []any{
			map[string]any{"email": "first@example.com", "type": "ENTERPRISE"},
			map[string]any{"email": "second@example.com"},
		},
	}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	if err := pre(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !runner.called || runner.gotTool != "list_user_mailboxes" {
		t.Fatalf("expected list_user_mailboxes call, runner=%+v", runner)
	}
	got, _ := cmd.Flags().GetString("email")
	if got != "first@example.com" {
		t.Fatalf("fallback email = %q, want first@example.com", got)
	}
}

func TestMailUserSearchEmailFallback_HandlesWrappedResultEnvelope(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{
		"result": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "wrapped@example.com"},
			},
		},
	}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	if err := pre(cmd, nil); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got, _ := cmd.Flags().GetString("email"); got != "wrapped@example.com" {
		t.Fatalf("email = %q", got)
	}
}

func TestMailUserSearchEmailFallback_NoMailboxReturnsValidation(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{"emailAccounts": []any{}}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	err := pre(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error when no mailbox returned")
	}
	if !strings.Contains(err.Error(), "could not auto-detect a mailbox") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestMailUserSearchEmailFallback_PropagatesRunnerError(t *testing.T) {
	runner := &fakeMailboxRunner{err: errors.New("boom")}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	err := pre(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "auto-detect mailbox") {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
}

// ── installMailHook composition ────────────────────────────────

func TestInstallMailHook_NoOpForOtherProduct(t *testing.T) {
	cmd := newMailSendStub()
	originalCalled := false
	cmd.PreRunE = func(*cobra.Command, []string) error { originalCalled = true; return nil }
	installMailHook(cmd, "chat", "send_email", nil)
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !originalCalled {
		t.Fatal("original PreRunE should still run")
	}
	// Setting a bad attachment should NOT fail since hook is no-op for chat.
	if err := cmd.Flags().Set("attachment", "/tmp/no_such_path_for_chat.bin"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("non-mail product must not validate attachments: %v", err)
	}
}

func TestInstallMailHook_ChainsExistingPreRunE(t *testing.T) {
	cmd := newMailSendStub()
	originalCalled := false
	cmd.PreRunE = func(*cobra.Command, []string) error {
		originalCalled = true
		return nil
	}
	installMailHook(cmd, "mail", "send_email", nil)
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !originalCalled {
		t.Fatal("original PreRunE was dropped")
	}
}

func TestInstallMailHook_BailsIfChainedPreRunEFails(t *testing.T) {
	cmd := newMailSendStub()
	cmd.PreRunE = func(*cobra.Command, []string) error { return errors.New("original boom") }
	installMailHook(cmd, "mail", "send_email", nil)
	err := cmd.PreRunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "original boom") {
		t.Fatalf("expected original PreRunE error to bubble, got %v", err)
	}
}

func TestInstallMailHook_AttachesToAllAttachmentTools(t *testing.T) {
	tools := []string{
		"send_email",
		"create_reply_draft",
		"create_replyall_draft",
		"create_forward_draft",
		"create_draft",
		"update_draft",
	}
	for _, tool := range tools {
		cmd := newMailSendStub()
		installMailHook(cmd, "mail", tool, nil)
		if err := cmd.Flags().Set("attachment", "/tmp/no_such_file_for_"+tool); err != nil {
			t.Fatal(err)
		}
		err := cmd.PreRunE(cmd, nil)
		if err == nil {
			t.Fatalf("tool %s: expected attachment validation to fire", tool)
		}
	}
}

func TestInstallMailHook_NilCmdSafe(t *testing.T) {
	// Defensive: should not panic.
	installMailHook(nil, "mail", "send_email", nil)
}

// ── extractFirstMailboxEmail wrap handling ─────────────────────

func TestExtractFirstMailboxEmail_HandlesRuntimeRunnerWrapping(t *testing.T) {
	// Mirrors runtimeRunner.executeInvocation: Response is wrapped as
	// {"endpoint": "...", "content": {emailAccounts: [...]}}.
	wrapped := map[string]any{
		"endpoint": "https://mcp.example.com",
		"content": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "real@example.com", "type": "PERSONAL"},
			},
		},
	}
	if got := extractFirstMailboxEmail(wrapped); got != "real@example.com" {
		t.Fatalf("wrapped extraction failed: got %q", got)
	}
}

func TestExtractFirstMailboxEmail_HandlesNestedResultUnderContent(t *testing.T) {
	wrapped := map[string]any{
		"content": map[string]any{
			"result": map[string]any{
				"emailAccounts": []any{
					map[string]any{"email": "nested@example.com"},
				},
			},
		},
	}
	if got := extractFirstMailboxEmail(wrapped); got != "nested@example.com" {
		t.Fatalf("nested extraction failed: got %q", got)
	}
}

func TestExtractFirstMailboxEmail_TextBlockFallback(t *testing.T) {
	wrapped := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"emailAccounts":[{"email":"text@example.com"}]}`},
		},
	}
	if got := extractFirstMailboxEmail(wrapped); got != "text@example.com" {
		t.Fatalf("text-block extraction failed: got %q", got)
	}
}

func TestExtractFirstMailboxEmail_ReturnsEmptyForEmptyAccounts(t *testing.T) {
	if extractFirstMailboxEmail(map[string]any{"content": map[string]any{"emailAccounts": []any{}}}) != "" {
		t.Fatal("expected empty for no accounts")
	}
	if extractFirstMailboxEmail(nil) != "" {
		t.Fatal("expected empty for nil")
	}
}

// ── pickMailboxForUserSearch tier preference ───────────────────

func TestPickMailboxForUserSearch_PrefersEnterprise(t *testing.T) {
	resp := map[string]any{
		"content": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "personal@dingtalk.com", "type": "PERSONAL"},
				map[string]any{"email": "biz@corp.com", "type": "ENTERPRISE"},
			},
		},
	}
	email, kind := pickMailboxForUserSearch(resp)
	if email != "biz@corp.com" {
		t.Fatalf("expected enterprise pick, got %q", email)
	}
	if kind != mailboxKindEnterprise {
		t.Fatalf("expected enterprise kind, got %v", kind)
	}
}

func TestPickMailboxForUserSearch_FallsBackToPersonalWithKind(t *testing.T) {
	resp := map[string]any{
		"content": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "personal@dingtalk.com", "type": "PERSONAL"},
			},
		},
	}
	email, kind := pickMailboxForUserSearch(resp)
	if email != "personal@dingtalk.com" {
		t.Fatalf("expected personal email, got %q", email)
	}
	if kind != mailboxKindPersonal {
		t.Fatalf("expected personal kind, got %v", kind)
	}
}

func TestPickMailboxForUserSearch_EmptyResponse(t *testing.T) {
	email, kind := pickMailboxForUserSearch(nil)
	if email != "" || kind != mailboxKindUnknown {
		t.Fatalf("expected empty unknown, got email=%q kind=%v", email, kind)
	}
}

func TestMailUserSearchEmailFallback_PersonalMailboxEmitsSkipMarker(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{
		"content": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "personal@dingtalk.com", "type": "PERSONAL"},
			},
		},
	}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	err := pre(cmd, nil)
	if err == nil {
		t.Fatal("expected validation error to short-circuit personal mailbox")
	}
	if !strings.Contains(err.Error(), "PAT_MEDIUM_RISK_NO_PERMISSION") {
		t.Fatalf("missing skip marker, got %v", err)
	}
	// Confirm we did NOT mutate --email when refusing.
	if got, _ := cmd.Flags().GetString("email"); got != "" {
		t.Fatalf("email should remain unset, got %q", got)
	}
}

func TestMailUserSearchEmailFallback_PrefersEnterpriseOverPersonal(t *testing.T) {
	runner := &fakeMailboxRunner{resp: map[string]any{
		"content": map[string]any{
			"emailAccounts": []any{
				map[string]any{"email": "p@dingtalk.com", "type": "PERSONAL"},
				map[string]any{"email": "biz@corp.com", "type": "ENTERPRISE"},
			},
		},
	}}
	pre := newMailUserSearchEmailFallback(runner)
	cmd := newMailUserSearchStub()
	if err := pre(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, _ := cmd.Flags().GetString("email"); got != "biz@corp.com" {
		t.Fatalf("fallback email = %q, want biz@corp.com", got)
	}
}
