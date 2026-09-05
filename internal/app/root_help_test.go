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

package app

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestRootHelpHidesCompatibilityOnlyCommands(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root help: %v\n%s", err, out.String())
	}
	help := out.String()
	if strings.Contains(help, "● conference") {
		t.Fatalf("root help should hide conference compatibility command:\n%s", help)
	}
	for _, want := range []string{
		"● dev",
		"• upgrade",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestRootHelpShowsFeedbackEntry(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root help: %v\n%s", err, out.String())
	}
	help := out.String()
	// The label stays Chinese regardless of the host locale: the rest of this
	// listing is hardcoded Chinese, so a translated label would show up as a
	// lone English line on any host whose LANG is not zh_*.
	for _, want := range []string{"Feedback:", "使用体验反馈问卷", feedbackFormURL} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	// The form URL is longer than the help rule width; it must stay on a
	// single unbroken line so terminals keep recognizing it as a hyperlink.
	if !strings.Contains(help, "\n    "+feedbackFormURL+"\n") {
		t.Fatalf("feedback URL must occupy one unwrapped line:\n%s", help)
	}
	if !strings.HasSuffix(strings.TrimSpace(help), feedbackFormURL) {
		t.Fatalf("Feedback must remain the final root Help section:\n%s", help)
	}
}

func TestRootHelpShowsAgentQuickstartAndSafetyModel(t *testing.T) {
	help := executeHelpOutput(t, "--help")
	for _, want := range []string{
		"Agent Quickstart:",
		"dws <service> --help",
		"dws <path> --help",
		`dws schema --cli-path "<path>" --compact -f json`,
		"Prefer structured output",
		"Use dry-run only when the leaf explicitly supports it",
		"Never add --yes without explicit user confirmation",
		"Safety model:",
		"effect=read|write|destructive",
		"risk=low|medium|high",
		"confirmation=not_required|user_required",
		"idempotency=idempotent|retryable|non_idempotent|unknown",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root Help missing %q:\n%s", want, help)
		}
	}
	if strings.Index(help, "Agent Quickstart:") > strings.Index(help, "Feedback:") ||
		strings.Index(help, "Safety model:") > strings.Index(help, "Feedback:") {
		t.Fatalf("Agent guidance must render before final Feedback section:\n%s", help)
	}
}

func TestLeafHelpRendersSelectionSafetyAndReferences(t *testing.T) {
	for _, tc := range []struct {
		path         string
		wantEffect   string
		wantConfirm  string
		wantSkill    string
		wantDocument string
	}{
		{path: "dev app list", wantEffect: "read", wantConfirm: "not_required", wantSkill: "dingtalk-misc", wantDocument: "references/devapp.md"},
		{path: "chat message send", wantEffect: "write", wantConfirm: "not_required", wantSkill: "dingtalk-chat", wantDocument: "references/chat.md"},
		{path: "dev app delete", wantEffect: "destructive", wantConfirm: "user_required", wantSkill: "dingtalk-misc", wantDocument: "references/devapp.md"},
		{path: "contract record list", wantEffect: "read", wantConfirm: "not_required", wantSkill: "dingtalk-misc", wantDocument: "references/contract.md"},
		{path: "contract subject delete", wantEffect: "destructive", wantConfirm: "user_required", wantSkill: "dingtalk-misc", wantDocument: "references/contract.md"},
		{path: "contact org apply-list", wantEffect: "write", wantConfirm: "not_required", wantSkill: "dingtalk-contact", wantDocument: "references/contact.md"},
	} {
		t.Run(strings.ReplaceAll(tc.path, " ", "_"), func(t *testing.T) {
			root := NewRootCommand()
			meta, ok := cli.ResolveMeta(tc.path)
			if !ok {
				t.Fatalf("ResolveMeta(%q) missing", tc.path)
			}
			out := executeHelpOutputWithRoot(t, root, append(strings.Fields(tc.path), "--help")...)
			for _, want := range []string{
				"When to use:", meta.Selection.UseWhen[0],
				"Avoid when:", meta.Selection.AvoidWhen[0],
				"Safety: effect=" + tc.wantEffect,
				"confirmation=" + tc.wantConfirm,
				"idempotency=" + meta.Safety.Idempotency,
				"Related skills:", tc.wantSkill,
				"Documentation:", tc.wantDocument,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("%s Help missing %q:\n%s", tc.path, want, out)
				}
			}
			for _, once := range []string{"When to use:", "Avoid when:", "Safety:", "Related skills:", "Documentation:"} {
				if count := strings.Count(out, once); count != 1 {
					t.Fatalf("%s Help contains %q %d times, want once:\n%s", tc.path, once, count, out)
				}
			}
			if tc.wantConfirm == "user_required" && !strings.Contains(out, "Do not use --yes until the user explicitly confirms") {
				t.Fatalf("%s Help missing explicit --yes confirmation boundary:\n%s", tc.path, out)
			}
		})
	}
}

func TestServiceAndHelpCommandRenderReferencesOnce(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "--help"},
		{"im", "--help"},
		{"help", "chat"},
		{"chat", "message", "send", "--help"},
		{"help", "chat", "message", "send"},
	} {
		out := executeHelpOutput(t, args...)
		for _, heading := range []string{"Related skills:", "Documentation:"} {
			if count := strings.Count(out, heading); count != 1 {
				t.Fatalf("dws %s contains %q %d times, want once:\n%s", strings.Join(args, " "), heading, count, out)
			}
		}
		if !strings.Contains(out, "dingtalk-chat") || !strings.Contains(out, "references/chat.md") {
			t.Fatalf("dws %s missing inherited chat references:\n%s", strings.Join(args, " "), out)
		}
	}
}

func TestCrossPlatformCoverageChatHelpUsesShortcutFirstTiers(t *testing.T) {
	help := executeHelpOutput(t, "chat", "--help")
	for _, want := range []string{
		"选择顺序：",
		"优先使用 +shortcut 完成用户任务",
		"Featured Shortcuts:",
		"Atomic API Resources:",
		"More Chat Shortcuts:",
		"当前有 93 个 canonical Shortcut",
		"本页展示 26 个高频入口",
		"另有 67 个低频正式入口",
		"dws shortcut list --service chat --format json",
		`dws schema --cli-path "chat +<command>" --compact -f json`,
		"+dm",
		"+chat-search",
		"+chat-update",
		"+flag-list",
		"+messages-send",
		"group",
		"message",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("chat shortcut-first Help missing %q:\n%s", want, help)
		}
	}
	for _, hiddenFromRoot := range []string{
		"+conversation-mute ",
		"+category-create ",
		"+chat-list-all ",
		"  mute                    ",
		"  conversation-info       ",
	} {
		if strings.Contains(help, hiddenFromRoot) {
			t.Fatalf("chat root Help unexpectedly lists non-featured %q:\n%s", hiddenFromRoot, help)
		}
	}

	leafHelp := executeHelpOutput(t, "chat", "+conversation-mute", "--help")
	if !strings.Contains(leafHelp, "dws chat +conversation-mute") ||
		!strings.Contains(leafHelp, "Safety:") {
		t.Fatalf("catalog Shortcut exact Help must remain available:\n%s", leafHelp)
	}
}

func TestCrossPlatformCoverageAtomicHelpPointsToPreferredShortcut(t *testing.T) {
	for _, tc := range []struct {
		path  string
		owner string
	}{
		{path: "chat mute", owner: "chat +conversation-mute"},
		{path: "chat category create", owner: "chat +category-create"},
		{path: "chat message send", owner: "chat +messages-send"},
		{path: "im group rename", owner: "chat +chat-update"},
	} {
		out := executeHelpOutput(t, append(strings.Fields(tc.path), "--help")...)
		for _, want := range []string{
			"Preferred Shortcut:",
			"dws " + tc.owner,
			"默认使用 Shortcut",
			"只有需要 Shortcut 未公开的底层参数或原始响应",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s atomic Help missing %q:\n%s", tc.path, want, out)
			}
		}
	}
}

func TestCrossPlatformCoverageChatHelpGuidanceSurvivesAliasAndHelpCommand(t *testing.T) {
	for _, args := range [][]string{{"im", "--help"}, {"help", "chat"}} {
		out := executeHelpOutput(t, args...)
		for _, want := range []string{
			"优先使用 +shortcut 完成用户任务",
			"Featured Shortcuts:",
			"dws shortcut list --service chat --format json",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("dws %s missing Shortcut-first guidance %q:\n%s", strings.Join(args, " "), want, out)
			}
		}
	}
}

func TestProgrammaticLeafHelpRendersAffordancesOnce(t *testing.T) {
	root := NewRootCommand()
	target, remaining, err := root.Find([]string{"chat", "message", "send"})
	if err != nil || target == nil || len(remaining) != 0 {
		t.Fatalf("find chat message send: target=%v remaining=%v err=%v", target, remaining, err)
	}
	var out bytes.Buffer
	root.SetOut(&out)
	target.SetOut(&out)
	if err := target.Help(); err != nil {
		t.Fatalf("programmatic leaf Help: %v", err)
	}
	for _, heading := range []string{"When to use:", "Avoid when:", "Safety:", "Related skills:", "Documentation:"} {
		if count := strings.Count(out.String(), heading); count != 1 {
			t.Fatalf("programmatic Help contains %q %d times, want once:\n%s", heading, count, out.String())
		}
	}
}

func TestLeafAliasUsesPrimarySelectionAndReferences(t *testing.T) {
	primary, ok := cli.ResolveMeta("report inbox list")
	if !ok || len(primary.Selection.UseWhen) == 0 {
		t.Fatalf("primary report metadata incomplete: %#v", primary)
	}
	aliased, ok := cli.ResolveMeta("report list")
	if !ok || aliased.Identity.Canonical != primary.Identity.Canonical {
		t.Fatalf("alias metadata = %#v, want canonical %q", aliased, primary.Identity.Canonical)
	}
	for _, path := range []string{"report inbox list", "report list"} {
		out := executeHelpOutput(t, append(strings.Fields(path), "--help")...)
		for _, want := range []string{primary.Selection.UseWhen[0], "Safety:", "dingtalk-misc", "references/report.md"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s Help missing primary affordance %q:\n%s", path, want, out)
			}
		}
	}
}

func TestPublicServicesDeclareExistingHelpReferences(t *testing.T) {
	root := NewRootCommand()
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	const documentationPrefix = "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/"
	for _, service := range visibleMCPRootCommands(root) {
		decl, ok := contract.LookupProductDecl(service.Name())
		if !ok {
			t.Errorf("public service %q has no ProductDecl", service.Name())
			continue
		}
		if len(decl.HelpReferences.RelatedSkills) == 0 || len(decl.HelpReferences.Documentation) == 0 {
			t.Errorf("public service %q HelpReferences = %#v, want skill and documentation", service.Name(), decl.HelpReferences)
			continue
		}
		for _, skill := range decl.HelpReferences.RelatedSkills {
			skillPath := filepath.Join(repositoryRoot, "skills", "multi", skill, "SKILL.md")
			if _, err := os.Stat(skillPath); err != nil {
				t.Errorf("public service %q related Skill %q does not exist: %v", service.Name(), skill, err)
			}
		}
		for _, document := range decl.HelpReferences.Documentation {
			parsed, err := url.Parse(document.URL)
			if err != nil || parsed.Scheme != "https" || !strings.HasPrefix(document.URL, documentationPrefix) {
				t.Errorf("public service %q documentation URL is not a stable HTTPS repository link: %q", service.Name(), document.URL)
				continue
			}
			relativePath := strings.TrimPrefix(document.URL, documentationPrefix)
			if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath))); err != nil {
				t.Errorf("public service %q documentation %q does not exist: %v", service.Name(), relativePath, err)
			}
		}
	}
}

func TestAgentVisibleLeavesHaveCompleteRenderableSafetyAndReferences(t *testing.T) {
	registry, err := cli.AssembleSchemaRegistry(NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatalf("assemble Schema registry: %v", err)
	}
	validEffect := map[string]bool{"read": true, "write": true, "destructive": true}
	validRisk := map[string]bool{"low": true, "medium": true, "high": true}
	validConfirmation := map[string]bool{"not_required": true, "user_required": true}
	validIdempotency := map[string]bool{"idempotent": true, "retryable": true, "non_idempotent": true, "unknown": true}
	leaves := 0
	for _, product := range registry.Products {
		decl, ok := contract.LookupProductDecl(product.ID)
		if !ok || len(decl.HelpReferences.RelatedSkills) == 0 || len(decl.HelpReferences.Documentation) == 0 {
			t.Errorf("Schema product %q has no complete HelpReferences", product.ID)
		}
		for _, tool := range product.Tools {
			leaves++
			safety := cli.CommandSafety{
				Effect: tool.Safety.Effect, Risk: tool.Safety.Risk,
				Confirmation: tool.Safety.Confirmation, Idempotency: tool.Safety.Idempotency,
			}
			if !validEffect[safety.Effect] || !validRisk[safety.Risk] ||
				!validConfirmation[safety.Confirmation] || !validIdempotency[safety.Idempotency] {
				t.Errorf("leaf %q has incomplete/invalid Safety: %#v", tool.Identity.CLIPath, safety)
			}
			if !safety.ShouldRender() {
				t.Errorf("leaf %q Safety is hidden: %#v", tool.Identity.CLIPath, safety)
			}
		}
	}
	if leaves == 0 {
		t.Fatal("assembled Schema contains no Agent-visible leaves")
	}
}

func executeHelpOutput(t *testing.T, args ...string) string {
	t.Helper()
	return executeHelpOutputWithRoot(t, NewRootCommand(), args...)
}

func executeHelpOutputWithRoot(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("dws %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// The feedback entry is deliberately root-only: this CLI is driven mostly by
// AI agents, and repeating a survey link in every subcommand help would be
// pure context noise. Guard the boundary so a future refactor cannot move the
// rendering into the shared subcommand help path unnoticed.
func TestSubcommandHelpOmitsFeedbackEntry(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"chat", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("chat help: %v\n%s", err, out.String())
	}
	if help := out.String(); strings.Contains(help, feedbackFormURL) {
		t.Fatalf("subcommand help must not carry the feedback URL:\n%s", help)
	}
}

func TestCalendarEventCreateHelpKeepsRoomsStringMetavar(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"calendar", "event", "create", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("calendar event create --help: %v\n%s", err, out.String())
	}

	help := out.String()
	if !strings.Contains(help, "--rooms string") {
		t.Fatalf("calendar event create help missing string metavar for --rooms:\n%s", help)
	}
	if strings.Contains(help, "--rooms room search") {
		t.Fatalf("calendar event create help treated description text as --rooms metavar:\n%s", help)
	}
}

func TestRootKeepsMainBranchChatCompatibilityCommands(t *testing.T) {
	root := NewRootCommand()
	for _, path := range []string{
		"chat send",
		"chat history",
		"im send",
		"im history",
	} {
		command, remaining, err := root.Find(strings.Fields(path))
		if err != nil {
			t.Fatalf("find %s: %v", path, err)
		}
		if len(remaining) != 0 || !command.Hidden || !command.Runnable() {
			t.Fatalf("%s compatibility contract: remaining=%v hidden=%v runnable=%v", path, remaining, command.Hidden, command.Runnable())
		}
	}
	for _, tc := range []struct {
		args []string
		hint string
	}{
		{args: []string{"chat", "send", "--group", "cid-stable", "--text", "hello"}, hint: "dws chat message send"},
		{args: []string{"im", "send", "--group", "cid-stable", "--text", "hello"}, hint: "dws chat message send"},
		{args: []string{"chat", "history", "--group", "cid-stable", "--limit", "20"}, hint: "dws chat message list --conversation-id <GROUP_OPEN_CONVERSATION_ID>"},
		{args: []string{"im", "history", "--group", "cid-stable", "--limit", "20"}, hint: "dws chat message list --conversation-id <GROUP_OPEN_CONVERSATION_ID>"},
	} {
		command := NewRootCommand()
		command.SilenceErrors = true
		command.SilenceUsage = true
		command.SetArgs(tc.args)
		err := command.Execute()
		var structured *apperrors.Error
		if !stderrors.As(err, &structured) || structured.Category != apperrors.CategoryValidation ||
			structured.Reason != "unknown_subcommand" || !strings.Contains(structured.Hint, tc.hint) {
			t.Fatalf("dws %s error = %#v, want migration hint %q", strings.Join(tc.args, " "), structured, tc.hint)
		}
	}

	listDirect := mustFindCommand(t, root, "chat", "message", "list-direct")
	for _, flag := range []string{"user", "open-dingtalk-id", "time", "forward", "limit"} {
		if listDirect.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message list-direct missing --%s", flag)
		}
	}

	mediaGroup := mustFindCommand(t, root, "chat", "media")
	if mediaGroup.Deprecated == "" || mediaGroup.Hidden || !mediaGroup.Runnable() {
		t.Fatalf("chat media compatibility contract: deprecated=%q hidden=%v runnable=%v", mediaGroup.Deprecated, mediaGroup.Hidden, mediaGroup.Runnable())
	}
	mediaUpload := mustFindCommand(t, mediaGroup, "upload")
	if mediaUpload.Deprecated == "" || mediaUpload.Hidden || !mediaUpload.Runnable() {
		t.Fatalf("chat media upload compatibility contract: deprecated=%q hidden=%v runnable=%v", mediaUpload.Deprecated, mediaUpload.Hidden, mediaUpload.Runnable())
	}
	for _, flag := range []string{"file", "type"} {
		if mediaUpload.Flags().Lookup(flag) == nil {
			t.Fatalf("chat media upload missing --%s", flag)
		}
	}

	mustFindCommand(t, root, "contact", "get")
	mustFindCommand(t, root, "contact", "search")
	mustFindCommand(t, root, "contact", "user", "list")
	mustFindCommand(t, root, "conference", "meeting", "reserve")
}

func TestChatHelpAndSchemaHideRetiredMediaUpload(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "--help"},
		{"chat", "media", "--help"},
	} {
		root := NewRootCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("dws %s: %v\n%s", strings.Join(args, " "), err, output.String())
		}
		for _, line := range strings.Split(output.String(), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && (fields[0] == "media" || fields[0] == "upload") {
				t.Fatalf("dws %s exposes retired command in Help line %q:\n%s", strings.Join(args, " "), line, output.String())
			}
		}
	}

	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"schema", "--cli-path", "chat media upload", "--format", "json"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("retired chat media upload remains queryable from Schema:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), "unknown runtime schema path") {
		t.Fatalf("retired chat media upload Schema error = %v, want unknown path", err)
	}
}

func TestRootChatMediaUploadWithoutAppCredentialsReturnsMigrationValidation(t *testing.T) {
	for _, key := range []string{"DWS_CLIENT_ID", "DWS_CLIENT_SECRET"} {
		value, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, value)
				return
			}
			_ = os.Unsetenv(key)
		})
		if _, exists := os.LookupEnv(key); exists {
			t.Fatalf("%s is still set", key)
		}
	}
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	filePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(filePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	commandArgs := []string{
		"chat", "media", "upload",
		"--file", filePath,
		"--type", "image",
	}
	testseam.Swap(t, &os.Args, append([]string{"dws"}, commandArgs...))

	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(commandArgs)
	err := root.Execute()
	if err == nil {
		t.Fatalf("chat media upload succeeded without app credentials:\n%s", output.String())
	}

	var typed *apperrors.Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("chat media upload error type = %T, want *errors.Error: %v", err, err)
	}
	if typed.Category != apperrors.CategoryValidation {
		t.Fatalf("chat media upload category = %q, want %q", typed.Category, apperrors.CategoryValidation)
	}
	if exitCode := apperrors.ExitCode(err); exitCode != 3 {
		t.Fatalf("chat media upload exit code = %d, want 3", exitCode)
	}

	got := output.String() + "\n" + err.Error()
	for _, want := range []string{"已下线", "chat message send --msg-type file --file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("chat media upload migration output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"DWS_CLIENT_ID",
		"DWS_CLIENT_SECRET",
		"缺少应用凭证",
		"AppSecret",
		"clientSecret",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("chat media upload returned credential error %q:\n%s", forbidden, got)
		}
	}
}

func TestRootKeepsContactWukongCompatibilityCommands(t *testing.T) {
	root := NewRootCommand()
	label := mustFindCommand(t, root, "contact", "label")
	if label.Hidden {
		t.Fatal("contact label should be visible as a real command group")
	}
	if !containsString(label.Aliases, "role") {
		t.Fatal("contact label missing role alias")
	}
	mustFindCommand(t, root, "contact", "label", "get")
	mustFindCommand(t, root, "contact", "label", "list")
	mustFindCommand(t, root, "contact", "label", "list-members")
	mustFindCommand(t, root, "contact", "label", "create")
	mustFindCommand(t, root, "contact", "label", "find")
	mustFindCommand(t, root, "contact", "label", "search")
	mustFindCommand(t, root, "contact", "label", "info")
	mustFindCommand(t, root, "contact", "label", "detail")
	mustFindCommand(t, root, "contact", "label", "list-all")

	getSelf := mustFindCommand(t, root, "contact", "user", "get-self")
	for _, alias := range []string{"self", "me", "whoami", "current"} {
		if !containsString(getSelf.Aliases, alias) {
			t.Fatalf("contact user get-self missing alias %q", alias)
		}
	}

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "label list",
			args: []string{"--dry-run", "contact", "label", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "label get",
			args: []string{"--dry-run", "contact", "label", "get", "--names", "admin,finance"},
			want: []string{"search_label_by_name", "labelNames", "admin", "finance"},
		},
		{
			name: "label members",
			args: []string{"--dry-run", "contact", "label", "list-members", "--id", "123"},
			want: []string{"get_label_members_by_labelId", "labelId", "123"},
		},
		{
			name: "label create role",
			args: []string{"--dry-run", "contact", "label", "create", "--name", "管理员", "--type", "role", "--parent-id", "12345", "--yes"},
			want: []string{"add_label", "parentId", "labelModel", "name", "管理员"},
		},
		{
			name: "label create group",
			args: []string{"--dry-run", "contact", "label", "create", "--name", "管理层", "--type", "group", "--yes"},
			want: []string{"add_label", "parentId", "labelModel", "name", "管理层"},
		},
		{
			name: "role shim",
			args: []string{"--dry-run", "contact", "role", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "label fuzzy shim",
			args: []string{"--dry-run", "contact", "label", "find", "--names", "admin"},
			want: []string{"search_label_by_name", "labelNames", "admin"},
		},
		{
			name: "label detail shim",
			args: []string{"--dry-run", "contact", "label", "detail", "--id", "123"},
			want: []string{"get_label_members_by_labelId", "labelId", "123"},
		},
		{
			name: "contact search shim",
			args: []string{"--dry-run", "contact", "search", "--query", "admin"},
			want: []string{"search_contact_by_key_word", "keyword", "admin"},
		},
		{
			name: "contact find shim",
			args: []string{"--dry-run", "contact", "find", "--query", "admin"},
			want: []string{"search_contact_by_key_word", "keyword", "admin"},
		},
		{
			name: "contact list defaults to label list",
			args: []string{"--dry-run", "contact", "list"},
			want: []string{"get_org_labels"},
		},
		{
			name: "contact list department members",
			args: []string{"--dry-run", "contact", "list", "--depts", "1"},
			want: []string{"get_dept_members_by_deptId", "deptIds", "1"},
		},
		{
			name: "contact get user details",
			args: []string{"--dry-run", "contact", "get", "--ids", "user1"},
			want: []string{"get_user_info_by_user_ids", "user_id_list", "user1"},
		},
		{
			name: "contact get label by name",
			args: []string{"--dry-run", "contact", "get", "--names", "admin"},
			want: []string{"search_label_by_name", "labelNames", "admin"},
		},
		{
			name: "contact self shim",
			args: []string{"--dry-run", "contact", "self"},
			want: []string{"get_current_user_profile"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeRootCaptureStdout(t, tc.args)
			if err != nil {
				t.Fatalf("Execute(%v) error = %v\n%s", tc.args, err, got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("Execute(%v) output missing %q:\n%s", tc.args, want, got)
				}
			}
		})
	}
}

func TestRootKeepsContactOrgInviteApplyAdminCommands(t *testing.T) {
	root := NewRootCommand()

	// 命令树结构：exclusive-account 新组、org 组 9 个新子命令、dept 组 invite-audit。
	mustFindCommand(t, root, "contact", "exclusive-account")
	mustFindCommand(t, root, "contact", "exclusive-account", "disable")
	mustFindCommand(t, root, "contact", "exclusive-account", "enable")
	mustFindCommand(t, root, "contact", "org", "invite-switch")
	mustFindCommand(t, root, "contact", "org", "invite-audit")
	mustFindCommand(t, root, "contact", "org", "invite-info")
	mustFindCommand(t, root, "contact", "org", "invite-list")
	mustFindCommand(t, root, "contact", "org", "apply-list")
	mustFindCommand(t, root, "contact", "org", "apply-approve")
	mustFindCommand(t, root, "contact", "org", "apply-reject")
	mustFindCommand(t, root, "contact", "org", "apply-block")
	mustFindCommand(t, root, "contact", "org", "apply-remove")
	mustFindCommand(t, root, "contact", "dept", "invite-audit")

	approve := mustFindCommand(t, root, "contact", "org", "apply-approve")
	if !containsString(approve.Aliases, "approve") {
		t.Fatal("contact org apply-approve missing approve alias")
	}

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "exclusive account disable",
			args: []string{"--dry-run", "contact", "exclusive-account", "disable", "--staff-id", "user001"},
			want: []string{"exclusive_account_set_status", "uesrId", "user001", "disable"},
		},
		{
			name: "exclusive account enable",
			args: []string{"--dry-run", "contact", "exclusive-account", "enable", "--staff-id", "user001"},
			want: []string{"exclusive_account_set_status", "uesrId", "user001", "enable"},
		},
		{
			name: "org invite switch",
			args: []string{"--dry-run", "contact", "org", "invite-switch", "--open", "true", "--search-invite", "false"},
			want: []string{"set_org_invite_switch", `"open": true`, `"searchInviteSwitch": false`},
		},
		{
			name: "org invite audit open",
			args: []string{"--dry-run", "contact", "org", "invite-audit", "--no-audit"},
			want: []string{"set_org_apply_audit", `"auditType": 0`},
		},
		{
			name: "org invite audit close",
			args: []string{"--dry-run", "contact", "org", "invite-audit", "--no-audit=false"},
			want: []string{"set_org_apply_audit", `"auditType": 1`},
		},
		{
			name: "org invite info",
			args: []string{"--dry-run", "contact", "org", "invite-info"},
			want: []string{"get_org_invite_info"},
		},
		{
			name: "org invite list defaults",
			args: []string{"--dry-run", "contact", "org", "invite-list"},
			want: []string{"list_team_invite", `"status": 1`, `"size": 20`},
		},
		{
			name: "org invite list paging",
			args: []string{"--dry-run", "contact", "org", "invite-list", "--status", "2", "--cursor", "33", "--size", "50"},
			want: []string{"list_team_invite", `"status": 2`, `"cursor": 33`, `"size": 50`},
		},
		{
			name: "org apply list all",
			args: []string{"--dry-run", "contact", "org", "apply-list", "--status", "0"},
			want: []string{"query_org_apply_list", `"status": 0`},
		},
		{
			name: "org apply approve",
			args: []string{"--dry-run", "contact", "org", "apply-approve", "--id", "123"},
			want: []string{"approve_org_apply", `"id": 123`},
		},
		{
			name: "org apply reject",
			args: []string{"--dry-run", "contact", "org", "apply-reject", "--id", "123", "--reason", "不符合入职条件"},
			want: []string{"reject_org_apply", `"id": 123`, "reason", "不符合入职条件"},
		},
		{
			name: "org apply block",
			args: []string{"--dry-run", "contact", "org", "apply-block", "--id", "123", "--reason", "恶意重复申请"},
			want: []string{"block_org_apply", `"id": 123`, "reason", "恶意重复申请"},
		},
		{
			name: "org apply remove",
			args: []string{"--dry-run", "contact", "org", "apply-remove", "--id", "123"},
			want: []string{"remove_org_apply", `"id": 123`},
		},
		{
			name: "dept invite audit",
			args: []string{"--dry-run", "contact", "dept", "invite-audit", "--dept", "12345", "--no-audit"},
			want: []string{"set_dept_apply_audit", `"deptId": 12345`, `"auditType": 0`},
		},
		{
			name: "dept invite audit emp apply join",
			args: []string{"--dry-run", "contact", "dept", "invite-audit", "--dept", "12345", "--no-audit", "--emp-apply-join-dept", "true"},
			want: []string{"set_dept_apply_audit", `"deptId": 12345`, `"auditType": 0`, `"empApplyJoinDept": true`},
		},
		{
			// pflag 布尔 flag 不吞并后置值：--no-audit true 中的 "true" 是位置参数，
			// contactNoAuditArgs 需将其归并回 flag 值而不是拒绝。
			name: "org invite audit positional bool tolerance",
			args: []string{"--dry-run", "contact", "org", "invite-audit", "--no-audit", "false"},
			want: []string{"set_org_apply_audit", `"auditType": 1`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeRootCaptureStdout(t, tc.args)
			if err != nil {
				t.Fatalf("Execute(%v) error = %v\n%s", tc.args, err, got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("Execute(%v) output missing %q:\n%s", tc.args, want, got)
				}
			}
		})
	}

	// 负例：--no-audit 必须显式传值（dry-run 已绕过确认 gate，能命中参数校验）。
	if got, err := executeRootCaptureStdout(t, []string{"--dry-run", "contact", "org", "invite-audit"}); err == nil || !strings.Contains(err.Error(), "--no-audit") {
		t.Fatalf("org invite-audit without --no-audit should fail with a no-audit hint, got err = %v\n%s", err, got)
	}
	if got, err := executeRootCaptureStdout(t, []string{"--dry-run", "contact", "org", "invite-list", "--status", "9"}); err == nil || !strings.Contains(err.Error(), "--status") {
		t.Fatalf("org invite-list --status 9 should fail with a status hint, got err = %v\n%s", err, got)
	}
	if got, err := executeRootCaptureStdout(t, []string{"--dry-run", "contact", "org", "apply-reject", "--id", "123"}); err == nil || !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("org apply-reject without --reason should fail with a reason hint, got err = %v\n%s", err, got)
	}
}

// contactWriteOpsCall 记录一次 MCP 写调用的完整入参。
type contactWriteOpsCall struct {
	server string
	tool   string
	args   map[string]any
}

// contactWriteOpsRecorder 是记录型 ToolCaller，供确认门禁测试断言
// 工具名与参数（区别于只验证参数组装的 --dry-run 用例）。
type contactWriteOpsRecorder struct {
	calls []contactWriteOpsCall
}

func (r *contactWriteOpsRecorder) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	r.calls = append(r.calls, contactWriteOpsCall{server: server, tool: tool, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true}`}}}, nil
}

func (*contactWriteOpsRecorder) Format() string { return "json" }
func (*contactWriteOpsRecorder) DryRun() bool   { return false }
func (*contactWriteOpsRecorder) Fields() string { return "" }
func (*contactWriteOpsRecorder) JQ() string     { return "" }

// executeContactWriteOp 走非 dry-run 真实执行路径：os.Args 需含产品名 contact
// 供 resolveProductID 路由 MCP Server；stdin 传空 reader 模拟非交互环境
// （EOF 时 ConfirmSafety 失败关闭）。
func executeContactWriteOp(t *testing.T, recorder *contactWriteOpsRecorder, args []string) error {
	t.Helper()
	testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
	root := NewRootCommand()
	helpers.InitDepsForTest(t, recorder)
	root.SetIn(strings.NewReader(""))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return root.Execute()
}

// TestContactWriteOpsConfirmationGate 是 user_required 写操作的端到端回归：
// 上方成功用例全部传 --dry-run（绕过确认 gate），无法证明真实执行路径上
// 未确认会在 MCP 调用前被拒绝。这里覆盖三类高风险写操作：
//   - apply-remove：删除申请记录，不可恢复；
//   - apply-block：屏蔽会拉黑申请人；
//   - disable：停用企业账号会阻止其登录钉钉。
func TestContactWriteOpsConfirmationGate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		tool     string
		wantArgs map[string]any
	}{
		{
			name:     "apply remove is not recoverable",
			args:     []string{"contact", "org", "apply-remove", "--id", "123"},
			tool:     "remove_org_apply",
			wantArgs: map[string]any{"id": int64(123)},
		},
		{
			name:     "apply block blacklists the applicant",
			args:     []string{"contact", "org", "apply-block", "--id", "123", "--reason", "恶意重复申请"},
			tool:     "block_org_apply",
			wantArgs: map[string]any{"id": int64(123), "reason": "恶意重复申请"},
		},
		{
			name: "exclusive account disable blocks sign-in",
			args: []string{"contact", "exclusive-account", "disable", "--staff-id", "user001"},
			tool: "exclusive_account_set_status",
			// uesrId 是 MCP 工具运行时入参的历史字段名（平台 inputMappings 笔误）。
			wantArgs: map[string]any{"uesrId": "user001", "status": "disable"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("unconfirmed rejects before any MCP call", func(t *testing.T) {
				recorder := &contactWriteOpsRecorder{}
				err := executeContactWriteOp(t, recorder, tc.args)
				if err == nil {
					t.Fatalf("%s without confirmation must be rejected", tc.name)
				}
				if !apperrors.IsConfirmationRequired(err) {
					t.Fatalf("%s error = %v, want reason confirmation_required", tc.name, err)
				}
				if len(recorder.calls) != 0 {
					t.Fatalf("%s must not reach MCP before confirmation, got calls: %+v", tc.name, recorder.calls)
				}
			})

			t.Run("explicit yes executes exactly the expected tool call", func(t *testing.T) {
				recorder := &contactWriteOpsRecorder{}
				err := executeContactWriteOp(t, recorder, append(tc.args, "--yes"))
				if err != nil {
					t.Fatalf("%s with --yes failed: %v", tc.name, err)
				}
				if len(recorder.calls) != 1 {
					t.Fatalf("%s with --yes want exactly 1 MCP call, got %d: %+v", tc.name, len(recorder.calls), recorder.calls)
				}
				call := recorder.calls[0]
				if call.server != "contact" || call.tool != tc.tool {
					t.Fatalf("%s call = %s/%s, want contact/%s", tc.name, call.server, call.tool, tc.tool)
				}
				if !reflect.DeepEqual(call.args, tc.wantArgs) {
					t.Fatalf("%s args = %#v, want %#v", tc.name, call.args, tc.wantArgs)
				}
			})
		})
	}
}

// TestContactApplyListSchemaProjectionMatchesRuntimeBehavior 是 apply-list
// 安全语义的回归：查询会把服务端未读申请标记为已读（Long 帮助明示），
// 最终 Schema 投影必须声明 Effect=write，不能把有副作用的查询伪装成纯
// 读取；同时副作用仅清除未读标记，Confirmation 应保持 not_required，
// 运行时也不得出现确认 gate。投影与运行行为任一侧回归都会被本测试拦截。
func TestContactApplyListSchemaProjectionMatchesRuntimeBehavior(t *testing.T) {
	t.Run("schema projection discloses read-marking side effect", func(t *testing.T) {
		meta, ok := cli.ResolveMeta("contact org apply-list")
		if !ok {
			t.Fatal("ResolveMeta(contact org apply-list) missing")
		}
		if meta.Safety.Effect != "write" {
			t.Fatalf("apply-list effect = %q, want write (query marks unread applies as read)", meta.Safety.Effect)
		}
		if meta.Safety.Risk != "low" || meta.Safety.Confirmation != "not_required" || meta.Safety.Idempotency != "idempotent" {
			t.Fatalf("apply-list safety = %+v, want low/not_required/idempotent", meta.Safety)
		}
		if !strings.Contains(meta.Selection.AgentSummary, "标记为已读") {
			t.Fatalf("apply-list agent summary %q must disclose the read-marking side effect", meta.Selection.AgentSummary)
		}
	})

	t.Run("not_required projection runs without confirmation gate", func(t *testing.T) {
		recorder := &contactWriteOpsRecorder{}
		err := executeContactWriteOp(t, recorder, []string{"contact", "org", "apply-list"})
		if err != nil {
			t.Fatalf("apply-list without --yes must not hit the confirmation gate: %v", err)
		}
		if len(recorder.calls) != 1 {
			t.Fatalf("apply-list want exactly 1 MCP call, got %d: %+v", len(recorder.calls), recorder.calls)
		}
		call := recorder.calls[0]
		if call.server != "contact" || call.tool != "query_org_apply_list" {
			t.Fatalf("apply-list call = %s/%s, want contact/query_org_apply_list", call.server, call.tool)
		}
		if want := map[string]any{"status": int64(1), "size": int64(20)}; !reflect.DeepEqual(call.args, want) {
			t.Fatalf("apply-list args = %#v, want %#v", call.args, want)
		}
	})
}

func TestChatConversationFileUploadUsesANewPath(t *testing.T) {
	root := NewRootCommand()
	fileCmd := mustFindCommand(t, root, "chat", "file")
	if !fileCmd.Hidden {
		t.Fatal("historical chat file group should remain hidden")
	}
	legacyUpload := mustFindCommand(t, root, "chat", "file", "upload")
	if !legacyUpload.Hidden {
		t.Fatal("historical chat file upload should remain hidden")
	}

	conversationFileCmd := mustFindCommand(t, root, "chat", "conversation-file")
	if conversationFileCmd.Hidden {
		t.Fatal("chat conversation-file should be visible")
	}
	upload := mustFindCommand(t, root, "chat", "conversation-file", "upload")
	if upload.Hidden {
		t.Fatal("chat conversation-file upload should be visible")
	}
	for _, flag := range []string{"conversation-id", "group", "user", "open-dingtalk-id", "file", "file-path", "file-name", "md5", "idempotency-key"} {
		if upload.Flags().Lookup(flag) == nil {
			t.Fatalf("chat conversation-file upload missing flag --%s", flag)
		}
	}
	for _, flag := range []string{"url", "uuid"} {
		if upload.Flags().Lookup(flag) != nil {
			t.Fatalf("new chat conversation-file upload must not inherit historical --%s", flag)
		}
	}

	send := mustFindCommand(t, root, "chat", "message", "send")
	for _, flag := range []string{"msg-type", "file-path"} {
		if send.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message send missing --%s", flag)
		}
	}
	idempotencyKey := send.Flags().Lookup("idempotency-key")
	if idempotencyKey == nil {
		t.Fatal("chat message send missing --idempotency-key")
	}
	legacyUUID := send.Flags().Lookup("uuid")
	if legacyUUID == nil || !legacyUUID.Hidden {
		t.Fatalf("chat message send --uuid hidden = %#v, want hidden compatibility flag", legacyUUID)
	}
	if got := legacyUUID.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != "idempotency-key" {
		t.Fatalf("chat message send --uuid alias_of = %#v, want idempotency-key", got)
	}
	if got := legacyUUID.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
		t.Fatalf("chat message send --uuid alias_origin = %#v, want %s", got, runtimeannotate.FlagAliasOriginCorecmdV1)
	}

	got, err := executeRootCaptureStdout(t, []string{
		"chat", "file", "upload",
		"--group", "cid",
		"--url", "https://example.com/report.pdf",
		"--file-name", "report.pdf",
	})
	if err == nil {
		t.Fatalf("historical chat file upload error = nil, want downline error\n%s", got)
	}
	got = got + "\n" + err.Error()
	for _, want := range []string{"已下线", "upload_conversation_file_by_url", "chat message send --msg-type file --file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("historical chat file upload output missing %q:\n%s", want, got)
		}
	}
}

func TestCalendarEventListDryRunPreviewsOnly(t *testing.T) {
	got, err := executeRootCaptureStdout(t, []string{
		"--dry-run", "calendar", "event", "list",
		"--start", "2026-07-07T00:00:00+08:00",
		"--end", "2026-07-07T01:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("calendar event list --dry-run error = %v\n%s", err, got)
	}
	for _, want := range []string{"list_calendar_events", "startTime", "endTime"} {
		if !strings.Contains(got, want) {
			t.Fatalf("calendar dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestCalendarEventShareInfoDryRunPreviewsOnly(t *testing.T) {
	got, err := executeRootCaptureStdout(t, []string{
		"--dry-run", "calendar", "event", "share-info",
		"--id", "EVT_001",
		"--language", "zh-CN",
		"--calendar-id", "primary",
	})
	if err != nil {
		t.Fatalf("calendar event share-info --dry-run error = %v\n%s", err, got)
	}
	for _, want := range []string{"get_event_share_info", "eventId", "EVT_001", "zh-CN", "primary"} {
		if !strings.Contains(got, want) {
			t.Fatalf("calendar event share-info dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestCalendarEventShareInfoRequiresEventID(t *testing.T) {
	got, err := executeRootCaptureStdout(t, []string{
		"--dry-run", "calendar", "event", "share-info",
	})
	if err == nil {
		t.Fatalf("calendar event share-info without --id: expected error, got nil\n%s", got)
	}
	if strings.Contains(got, "\"executed\": true") {
		t.Fatalf("share-info without --id must not execute:\n%s", got)
	}
}

func TestCalendarEventShareInfoOmitsOptionalArgs(t *testing.T) {
	got, err := executeRootCaptureStdout(t, []string{
		"--dry-run", "calendar", "event", "share-info",
		"--id", "EVT_001",
	})
	if err != nil {
		t.Fatalf("calendar event share-info --dry-run with only --id error = %v\n%s", err, got)
	}
	if !strings.Contains(got, "\"eventId\"") {
		t.Fatalf("calendar event share-info dry-run output missing eventId:\n%s", got)
	}
	for _, unwanted := range []string{"\"calendarId\"", "\"language\""} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("calendar event share-info dry-run with only --id should not contain %q:\n%s", unwanted, got)
		}
	}
}

func TestRootKeepsSVIPChatCompatibilityFlags(t *testing.T) {
	root := NewRootCommand()

	listBySender := mustFindCommand(t, root, "chat", "message", "list-by-sender")
	if listBySender.Flags().Lookup("sender") == nil {
		t.Fatal("chat message list-by-sender missing hidden --sender alias")
	}

	searchAdvanced := mustFindCommand(t, root, "chat", "message", "search-advanced")
	for _, flag := range []string{"sender", "senders", "sender-ids", "message-type", "only-robot", "conversation-type"} {
		if searchAdvanced.Flags().Lookup(flag) == nil {
			t.Fatalf("chat message search-advanced missing --%s", flag)
		}
	}
}

func TestCacheCommandDeprecatedCompatStub(t *testing.T) {
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"cache", "refresh"})
	if err != nil || cmd == nil || cmd == root {
		t.Fatalf("dws cache refresh compatibility stub missing: %v", err)
	}
	if cmd.Hidden || cmd.Deprecated == "" {
		t.Fatalf("cache refresh must be visible Deprecated: hidden=%v deprecated=%q", cmd.Hidden, cmd.Deprecated)
	}
}

func TestInjectStaticServersMergesStaticAndSupplementServers(t *testing.T) {
	previous := edition.Get()
	defer edition.Override(previous)
	defer SetDynamicServers(nil)

	edition.Override(&edition.Hooks{
		Name: "test",
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{
				ID:       "static-test",
				Name:     "Static Test",
				Endpoint: "https://static.example/server/static-test",
				Prefixes: []string{"static-alias"},
			}}
		},
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{
				ID:       "supplement-test",
				Name:     "Supplement Test",
				Endpoint: "https://supplement.example/server/supplement-test",
				Prefixes: []string{"supplement-alias"},
			}}
		},
	})

	injectStaticServers()

	for _, tc := range []struct {
		productID string
		endpoint  string
	}{
		{"static-test", "https://static.example/server/static-test"},
		{"static-alias", "https://static.example/server/static-test"},
		{"supplement-test", "https://supplement.example/server/supplement-test"},
		{"supplement-alias", "https://supplement.example/server/supplement-test"},
	} {
		got, ok := directRuntimeEndpoint(tc.productID, "")
		if !ok || got != tc.endpoint {
			t.Fatalf("directRuntimeEndpoint(%q) = %q, %v; want %q, true", tc.productID, got, ok, tc.endpoint)
		}
	}
}

func TestCrossPlatformCoverageStaticDingTalkEndpointsFollowConfiguredMCPBaseURL(t *testing.T) {
	previous := edition.Get()
	defer edition.Override(previous)
	defer SetDynamicServers(nil)

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte("https://pre-mcp.dingtalk.io\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(mcp_url) error = %v", err)
	}

	edition.Override(&edition.Hooks{
		Name: "test",
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{
				ID:       "contact",
				Name:     "Contact",
				Endpoint: "https://mcp-gw.dingtalk.com/server/contact?key=abc",
				Prefixes: []string{"user"},
			}}
		},
	})

	injectStaticServers()

	for _, productID := range []string{"contact", "user"} {
		got, ok := directRuntimeEndpoint(productID, "")
		want := "https://pre-mcp-gw.dingtalk.io/server/contact?key=abc"
		if !ok || got != want {
			t.Fatalf("directRuntimeEndpoint(%q) = %q, %v; want %q, true", productID, got, ok, want)
		}
	}
}

func TestCrossPlatformCoverageDingTalkEndpointsFollowSelectedTokenRegion(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	mcpURLPath := filepath.Join(configDir, "mcp_url")

	if err := os.WriteFile(mcpURLPath, []byte("https://pre-mcp.dingtalk.io\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(mcp_url) error = %v", err)
	}
	endpoint := "https://pre-mcp-gw.dingtalk.io/server/contact?key=abc"
	if got, want := activeDingTalkGatewayEndpointForLoginRegion(endpoint, authpkg.LoginRegionDefault), "https://pre-mcp-gw.dingtalk.com/server/contact?key=abc"; got != want {
		t.Fatalf("domestic profile endpoint = %q, want %q", got, want)
	}
	if got := activeDingTalkGatewayEndpointForLoginRegion(endpoint, authpkg.LoginRegionInternational); got != endpoint {
		t.Fatalf("international profile endpoint = %q, want %q", got, endpoint)
	}

	if err := os.WriteFile(mcpURLPath, []byte("https://mcp.dingtalk.com\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(mcp_url) error = %v", err)
	}
	if got, want := activeDingTalkGatewayEndpointForLoginRegion("https://mcp-gw.dingtalk.com/server/contact", authpkg.LoginRegionInternational), "https://mcp-gw.dingtalk.io/server/contact"; got != want {
		t.Fatalf("international profile endpoint from domestic config = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoverageDingTalkEndpointUsesLoginScopedMCPOverride(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	restore := authpkg.PushMCPBaseURLOverride("https://pre-mcp.dingtalk.io")
	defer restore()

	got := activeDingTalkGatewayEndpointForLoginRegion(
		"https://mcp-gw.dingtalk.com/server/contact",
		authpkg.LoginRegionDefault,
	)
	want := "https://pre-mcp-gw.dingtalk.io/server/contact"
	if got != want {
		t.Fatalf("login-scoped endpoint = %q, want %q", got, want)
	}
}

func mustFindCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cmd := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("missing command path %q under %q", strings.Join(path, " "), cmd.CommandPath())
		}
		cmd = next
	}
	return cmd
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func executeRootCaptureStdout(t *testing.T, args []string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error = %v", err)
	}
	os.Stdout = writePipe

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	_ = writePipe.Close()
	os.Stdout = oldStdout
	captured, readErr := io.ReadAll(readPipe)
	if readErr != nil {
		t.Fatalf("read stdout pipe error = %v", readErr)
	}
	return out.String() + string(captured), execErr
}
