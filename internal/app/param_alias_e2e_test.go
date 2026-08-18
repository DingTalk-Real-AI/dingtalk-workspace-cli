// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type paramAliasToolCall struct {
	server string
	tool   string
	args   map[string]any
}

type paramAliasCaptureCaller struct {
	calls []paramAliasToolCall
}

func (c *paramAliasCaptureCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copyArgs := make(map[string]any, len(args))
	for key, value := range args {
		copyArgs[key] = value
	}
	c.calls = append(c.calls, paramAliasToolCall{server: server, tool: tool, args: copyArgs})
	text := c.paramAliasResponseForTool(tool)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

// paramAliasResponseForTool supplies deterministic, business-shape-valid
// responses for the complete-command equivalence matrix. Most commands only
// print the transport result and need an empty object; smart shortcuts that
// inspect a read response receive the smallest shape that lets their full RunE
// complete without falling back to a validation error.
func (c *paramAliasCaptureCaller) paramAliasResponseForTool(tool string) string {
	switch tool {
	case "list_calendar_events":
		return `{"result":{"events":[]}}`
	case "query_records":
		return `{"success":true,"status":"success","error":{},"data":{}}`
	case "search_mail_users":
		return `{"users":[{"name":"Fixture User","email":"fixture@example.com","id":"fixture-user"}]}`
	case "search_dept_by_keyword":
		return `{"deptList":[{"deptId":1,"name":"Fixture Dept"}]}`
	case "search_groups":
		return `{"result":{"items":[{"openConversationId":"fixture-conversation","title":"Fixture Group"}]}}`
	case "search_contact_by_key_word":
		return `{"result":[{"name":"Fixture User","userId":"fixture-user","openDingTalkId":"D-fixture-user"}]}`
	case "list_doc_versions":
		return `{"result":{"items":[{"version":3}]}}`
	case "revert_doc_version":
		return `{"revertedToVersion":3}`
	case "search_doc_templates":
		return `{"result":[{"templateId":"fixture-template-id"}]}`
	case "list_workflows":
		return `{"workflows":[]}`
	case "create_document":
		return `{"nodeId":"fixture-node"}`
	case "list_files":
		return `{"success":true,"result":{"files":[],"hasMore":false}}`
	case "list_recycle_items":
		return `{"success":true,"result":{"recycleItems":[{"recycleItemId":"recycle-1","originalName":"Fixture Node"}],"hasMore":false}}`
	case "get_star_list":
		return `{"success":true,"result":{"starList":[],"hasMore":false}}`
	case "list_file_versions":
		return `{"success":true,"result":{"versions":[{"version":3,"name":"Fixture Version"}],"hasMore":false}}`
	case "get_file_info":
		name := "Fixture Node"
		for index := len(c.calls) - 2; index >= 0; index-- {
			call := c.calls[index]
			switch call.tool {
			case "create_folder":
				if value, ok := call.args["name"].(string); ok {
					name = value
				}
				index = -1
			case "rename_document":
				if value, ok := call.args["newName"].(string); ok {
					name = value
				}
				index = -1
			}
		}
		encoded, _ := json.Marshal(map[string]any{"success": true, "result": map[string]any{"fileId": "node-1", "name": name}})
		return string(encoded)
	case "get_cover", "get_node_stats":
		return `{"success":true,"result":{"nodeId":"node-1"}}`
	case "get_file_publish_status":
		return `{"success":true,"result":{"fileId":"node-1","published":false}}`
	case "create_folder", "create_shortcut":
		return `{"success":true,"fileId":"node-1"}`
	case "delete_document", "mark_star", "unmark_star", "restore_recycle_item", "rename_document", "revert_file_version":
		return `{"success":true,"fileId":"node-1"}`
	case "set_file_publish":
		return `{"success":true}`
	case "download_file", "download_file_version":
		return `{"success":true,"result":{"downloadUrl":"http://invalid.test/fixture.bin","fileName":"fixture.bin"}}`
	case "get_document_content":
		for index := len(c.calls) - 2; index >= 0; index-- {
			call := c.calls[index]
			for _, key := range []string{"jsonml", "markdown"} {
				if content, ok := call.args[key].(string); ok {
					encoded, _ := json.Marshal(map[string]any{"revision": 1, key: content})
					return string(encoded)
				}
			}
		}
		return `{"revision":1}`
	default:
		return `{}`
	}
}

func (*paramAliasCaptureCaller) Format() string { return "json" }
func (*paramAliasCaptureCaller) DryRun() bool   { return false }
func (*paramAliasCaptureCaller) Fields() string { return "" }
func (*paramAliasCaptureCaller) JQ() string     { return "" }

// paramAliasCaptureRunner covers helpers (currently dev app) that dispatch
// through executor.Runner instead of edition.ToolCaller. Keeping both capture
// boundaries in one call list lets the matrix compare the final request shape
// without knowing which transport adapter a command uses.
type paramAliasCaptureRunner struct {
	caller *paramAliasCaptureCaller
}

func (r *paramAliasCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	copyArgs := make(map[string]any, len(invocation.Params))
	for key, value := range invocation.Params {
		copyArgs[key] = value
	}
	r.caller.calls = append(r.caller.calls, paramAliasToolCall{
		server: invocation.CanonicalProduct,
		tool:   invocation.Tool,
		args:   copyArgs,
	})
	invocation.Implemented = true
	return executor.Result{Invocation: invocation, Response: map[string]any{}}, nil
}

type paramAliasDryRunRejectRunner struct {
	attempts []executor.Invocation
}

func (r *paramAliasDryRunRejectRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.attempts = append(r.attempts, invocation)
	return executor.Result{}, stderrors.New("dry-run reached the injected command runner")
}

type paramAliasDryRunPreview struct {
	DryRun    bool           `json:"dry_run"`
	Executed  bool           `json:"executed"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// executeParamAliasDryRunE2E uses the existing root --dry-run barrier as a
// parameter-normalization probe. These commands do not publish command-owned
// dry-run capabilities in Schema; the test deliberately makes no such claim.
// A reject runner proves the preview stops before endpoint resolution,
// authentication, or transport execution.
func executeParamAliasDryRunE2E(t *testing.T, args ...string) (*pipeline.Context, paramAliasDryRunPreview, []executor.Invocation, error) {
	t.Helper()

	originalArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = originalArgs }()

	captureFile, err := os.CreateTemp(t.TempDir(), "param-alias-dry-run-*.json")
	if err != nil {
		t.Fatalf("create dry-run output capture: %v", err)
	}
	defer captureFile.Close()
	originalStdout := os.Stdout
	originalCaller := helpers.GetCaller()
	os.Stdout = captureFile
	defer func() {
		os.Stdout = originalStdout
		helpers.InitDeps(originalCaller)
	}()
	rejectRunner := &paramAliasDryRunRejectRunner{}
	originalRunnerFactory := rootNewCommandRunnerWithFlags
	rootNewCommandRunnerWithFlags = func(*GlobalFlags) executor.Runner {
		return rejectRunner
	}
	root := NewRootCommand()
	rootNewCommandRunnerWithFlags = originalRunnerFactory
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	ctx, executeErr := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if executeErr == nil {
		executeErr = root.Execute()
	}

	if err := captureFile.Sync(); err != nil {
		t.Fatalf("sync dry-run output capture: %v", err)
	}
	if _, err := captureFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind dry-run output capture: %v", err)
	}
	output, err := io.ReadAll(captureFile)
	if err != nil {
		t.Fatalf("read dry-run output capture: %v", err)
	}
	var preview paramAliasDryRunPreview
	if executeErr == nil {
		if err := json.Unmarshal(output, &preview); err != nil {
			t.Fatalf("decode dry-run preview: %v\noutput=%s", err, output)
		}
	}
	return ctx, preview, append([]executor.Invocation(nil), rejectRunner.attempts...), executeErr
}

func TestCrossPlatformCoverageFlagListDryRunStopsBeforeReadDispatch(t *testing.T) {
	_, preview, attempts, err := executeParamAliasDryRunE2E(t,
		"chat", "+flag-list", "--page-size", "20", "--cursor", "0", "--dry-run",
	)
	if err != nil {
		t.Fatalf("flag-list dry-run error = %v", err)
	}
	if len(attempts) != 0 {
		t.Fatalf("flag-list dry-run crossed dispatch boundary: %#v", attempts)
	}
	if !preview.DryRun || preview.Executed || preview.Tool != "list_message_favorites" {
		t.Fatalf("flag-list dry-run preview = %#v", preview)
	}
	if preview.Arguments["cursor"] != float64(0) || preview.Arguments["size"] != "20" {
		t.Fatalf("flag-list dry-run arguments = %#v", preview.Arguments)
	}
}

func executeParamAliasE2E(t *testing.T, caller *paramAliasCaptureCaller, args ...string) (*pipeline.Context, error) {
	t.Helper()
	originalArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = originalArgs }()

	originalRunnerFactory := rootNewCommandRunnerWithFlags
	rootNewCommandRunnerWithFlags = func(*GlobalFlags) executor.Runner {
		return &paramAliasCaptureRunner{caller: caller}
	}
	root := NewRootCommand()
	rootNewCommandRunnerWithFlags = originalRunnerFactory
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	originalCaller := helpers.GetCaller()
	helpers.InitDeps(caller)
	defer helpers.InitDeps(originalCaller)

	ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
	if err != nil {
		return ctx, err
	}
	return ctx, root.Execute()
}

func TestCrossPlatformCoverageBooleanStickyCannotBypassDestructiveConfirmation(t *testing.T) {
	tests := []struct {
		name           string
		confirmation   []string
		wantError      string
		wantCalls      int
		wantOriginal   string
		wantCorrection string
	}{
		{name: "bare yes confirms", confirmation: []string{"--yes"}, wantCalls: 1},
		{name: "glued false stays unconfirmed", confirmation: []string{"--yesfalse"}, wantError: "请添加 --yes 确认执行", wantOriginal: "--yesfalse", wantCorrection: "--yes=false"},
		{name: "glued true confirms", confirmation: []string{"--yestrue"}, wantCalls: 1, wantOriginal: "--yestrue", wantCorrection: "--yes=true"},
		{name: "detached false stays unconfirmed", confirmation: []string{"--yes", "false"}, wantError: "请添加 --yes 确认执行", wantOriginal: "--yes false", wantCorrection: "--yes=false"},
		{name: "detached no stays unconfirmed", confirmation: []string{"--yes", "no"}, wantError: "请添加 --yes 确认执行", wantOriginal: "--yes no", wantCorrection: "--yes=false"},
		{name: "detached zero stays unconfirmed", confirmation: []string{"--yes", "0"}, wantError: "请添加 --yes 确认执行", wantOriginal: "--yes 0", wantCorrection: "--yes=false"},
		{name: "detached true confirms", confirmation: []string{"--yes", "true"}, wantCalls: 1, wantOriginal: "--yes true", wantCorrection: "--yes=true"},
		{name: "detached yes confirms", confirmation: []string{"--yes", "yes"}, wantCalls: 1, wantOriginal: "--yes yes", wantCorrection: "--yes=true"},
		{name: "detached one confirms", confirmation: []string{"--yes", "1"}, wantCalls: 1, wantOriginal: "--yes 1", wantCorrection: "--yes=true"},
		{name: "explicit false remains unconfirmed", confirmation: []string{"--yes=false"}, wantError: "请添加 --yes 确认执行"},
		{name: "explicit true confirms", confirmation: []string{"--yes=true"}, wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &paramAliasCaptureCaller{}
			args := []string{
				"mail", "thread", "trash",
				"--email", "user@example.com",
				"--id", "conversation-1",
			}
			args = append(args, test.confirmation...)
			ctx, err := executeParamAliasE2E(t, caller, args...)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("confirmed command error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("command error = %v, want substring %q", err, test.wantError)
			}
			if test.wantCorrection == "" {
				if ctx != nil && len(ctx.Corrections) != 0 {
					t.Fatalf("confirmation spelling received corrections: %#v", ctx.Corrections)
				}
			} else if ctx == nil || len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != test.wantOriginal || ctx.Corrections[0].Corrected != test.wantCorrection {
				t.Fatalf("confirmation corrections = %#v, want %q -> %q", ctx, test.wantOriginal, test.wantCorrection)
			}
			if len(caller.calls) != test.wantCalls {
				t.Fatalf("destructive calls = %#v, want %d", caller.calls, test.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageParamAliasReadCommandFinalPayload(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	start := "2026-03-10T14:00:00+08:00"
	end := "2026-03-10T18:00:00+08:00"
	ctx, err := executeParamAliasE2E(t, caller,
		"calendar", "event", "list",
		"--date", start,
		"--end-time", end,
		"--calendar", "primary",
		"--max-results", "7",
		"--next-cursor", "cursor-1",
	)
	if err != nil {
		t.Fatalf("calendar alias E2E error = %v", err)
	}
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--date" || ctx.Corrections[0].Corrected != "--start" {
		t.Fatalf("calendar corrections = %#v, want only --date to be normalized centrally", ctx.Corrections)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_calendar_events" {
		t.Fatalf("calendar calls = %#v", caller.calls)
	}
	startMS, _ := cmdutil.ParseISOTimeToMillis("start", start)
	endMS, _ := cmdutil.ParseISOTimeToMillis("end", end)
	want := map[string]any{
		"startTime":  startMS,
		"endTime":    endMS,
		"calendarId": "primary",
		"limit":      7,
		"cursor":     "cursor-1",
	}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calendar payload = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestCrossPlatformCoverageParamAliasWriteCommandFinalPayload(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasE2E(t, caller,
		"chat", "message", "send",
		"--to-user", appFixtureCurrentDOpenID,
		"--text", "hello alias",
		"--idempotency-key", "alias-e2e",
	)
	if err != nil {
		t.Fatalf("chat write alias E2E error = %v", err)
	}
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--to-user" || ctx.Corrections[0].Corrected != "--user" {
		t.Fatalf("chat corrections = %#v", ctx.Corrections)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "send_personal_message" {
		t.Fatalf("chat calls = %#v", caller.calls)
	}
	payload := caller.calls[0].args
	if payload["receiverOpenDingTalkId"] != appFixtureCurrentDOpenID || payload["uuid"] != "alias-e2e" || payload["msgType"] != "markdown" {
		t.Fatalf("chat payload identity fields = %#v", payload)
	}
	content, _ := payload["content"].(string)
	if !strings.Contains(content, "hello alias") {
		t.Fatalf("chat payload content = %q", content)
	}
	for _, forbidden := range []string{"user", "to-user", "userId"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("chat payload leaked pre-normalization field %q: %#v", forbidden, payload)
		}
	}
}

func TestCrossPlatformCoverageChatMessageSendLegacyUUIDAliasFinalPayload(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	_, err := executeParamAliasE2E(t, caller,
		"chat", "message", "send",
		"--group", "fixture-conversation",
		"--text", "hello legacy uuid",
		"--uuid", "legacy-alias-e2e",
	)
	if err != nil {
		t.Fatalf("chat message send legacy uuid error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "send_personal_message" {
		t.Fatalf("chat calls = %#v", caller.calls)
	}
	payload := caller.calls[0].args
	if payload["uuid"] != "legacy-alias-e2e" || payload["openConversationId"] != "fixture-conversation" {
		t.Fatalf("chat legacy uuid payload = %#v", payload)
	}
	if _, exists := payload["idempotency-key"]; exists {
		t.Fatalf("chat payload leaked CLI-only idempotency-key: %#v", payload)
	}
}

func TestCrossPlatformCoverageChatReactionConversationAliasesReachCanonicalPayload(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		tool     string
		required []string
	}{
		{
			name:     "add emoji",
			command:  []string{"chat", "message", "add-emoji"},
			tool:     "add_emoji_reaction",
			required: []string{"--msg-id", "message-1", "--emoji", "like"},
		},
		{
			name:     "remove emoji",
			command:  []string{"chat", "message", "remove-emoji"},
			tool:     "remove_emoji_reaction",
			required: []string{"--msg-id", "message-1", "--emoji", "like"},
		},
		{
			name:    "add text emotion",
			command: []string{"chat", "message", "add-text-emotion"},
			tool:    "add_text_emotion",
			required: []string{
				"--msg-id", "message-1", "--emotion-id", "emotion-1",
				"--emotion-name", "like", "--text", "nice", "--background-id", "background-1",
			},
		},
		{
			name:    "remove text emotion",
			command: []string{"chat", "message", "remove-text-emotion"},
			tool:    "remove_text_emotion",
			required: []string{
				"--msg-id", "message-1", "--emotion-id", "emotion-1",
				"--emotion-name", "like", "--text", "nice", "--background-id", "background-1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canonicalArgs := append([]string(nil), test.command...)
			canonicalArgs = append(canonicalArgs, "--conversation-id", "conversation-1")
			canonicalArgs = append(canonicalArgs, test.required...)
			canonicalCaller := &paramAliasCaptureCaller{}
			if _, err := executeParamAliasE2E(t, canonicalCaller, canonicalArgs...); err != nil {
				t.Fatalf("canonical execution failed: %v", err)
			}
			if len(canonicalCaller.calls) != 1 || canonicalCaller.calls[0].tool != test.tool {
				t.Fatalf("canonical calls = %#v, want one %s call", canonicalCaller.calls, test.tool)
			}
			if canonicalCaller.calls[0].args["openConversationId"] != "conversation-1" {
				t.Fatalf("canonical payload = %#v", canonicalCaller.calls[0].args)
			}

			// Numeric --group-id is a different identifier domain and is covered
			// by TestAllReviewedParamAliasGuardsReachRuntimeContract.
			for _, alias := range []string{"chat-id", "open-conversation-id"} {
				t.Run(alias, func(t *testing.T) {
					aliasArgs := append([]string(nil), test.command...)
					aliasArgs = append(aliasArgs, "--"+alias, "conversation-1")
					aliasArgs = append(aliasArgs, test.required...)
					aliasCaller := &paramAliasCaptureCaller{}
					ctx, err := executeParamAliasE2E(t, aliasCaller, aliasArgs...)
					if err != nil {
						t.Fatalf("alias execution failed: %v", err)
					}
					if alias == "open-conversation-id" {
						if ctx == nil || len(ctx.Corrections) != 0 {
							t.Fatalf("alias corrections = %#v", ctx)
						}
					} else if ctx == nil || len(ctx.Corrections) != 1 || ctx.Corrections[0].Original != "--"+alias || ctx.Corrections[0].Corrected != "--conversation-id" {
						t.Fatalf("alias corrections = %#v", ctx)
					}
					if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
						t.Fatalf("final calls differ\ncanonical=%#v\nalias=%#v", canonicalCaller.calls, aliasCaller.calls)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageAllGeneratedChatParamAliasesReachRuntimeCobraContract(t *testing.T) {
	root := NewRootCommand()
	engine := newPipelineEngine()
	entries, err := cli.ReduceParamAliases(root)
	if err != nil {
		t.Fatalf("ReduceParamAliases() error = %v", err)
	}

	chatEntries := 0
	aliasCases := 0
	guardCases := map[pipeline.FlagProtection]int{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.CLIPath, "chat ") {
			continue
		}
		chatEntries++
		leaf := resolveParamLeaf(root, entry.CLIPath)
		if leaf == nil {
			t.Fatalf("generated chat parameter path %q is not runnable", entry.CLIPath)
		}

		aliases := make([]string, 0, len(entry.Aliases))
		for emitted := range entry.Aliases {
			aliases = append(aliases, emitted)
		}
		sort.Strings(aliases)
		for _, emitted := range aliases {
			emitted := emitted
			canonical := entry.Aliases[emitted]
			aliasCases++
			t.Run(entry.CLIPath+"/alias/"+emitted, func(t *testing.T) {
				value := paramFixtureValue(leaf, emitted, canonical)
				rawArgs := append(strings.Fields(entry.CLIPath), "--"+emitted, value)
				ctx, runErr := pipeline.RunPreParseArgs(root, engine, rawArgs)
				if runErr != nil {
					t.Fatalf("RunPreParseArgs(%v) error = %v", rawArgs, runErr)
				}
				if ctx == nil {
					t.Fatal("RunPreParseArgs returned nil context")
				}
				flagArgs := ctx.Args[len(strings.Fields(entry.CLIPath)):]
				if len(flagArgs) < 2 || flagArgs[0] != "--"+canonical || flagArgs[1] != value {
					t.Fatalf("runtime alias %q => %q produced args %v", emitted, canonical, ctx.Args)
				}
				if parseErr := leaf.ParseFlags(flagArgs); parseErr != nil {
					t.Fatalf("canonical Cobra ParseFlags(%v) error = %v", flagArgs, parseErr)
				}
			})
		}

		for _, guard := range []struct {
			protection pipeline.FlagProtection
			emitted    []string
		}{
			{protection: pipeline.FlagProtectionBlocked, emitted: entry.Blocked},
			{protection: pipeline.FlagProtectionAmbiguous, emitted: entry.Ambiguous},
		} {
			for _, emitted := range guard.emitted {
				emitted := emitted
				protection := guard.protection
				guardCases[protection]++
				t.Run(entry.CLIPath+"/"+string(protection)+"/"+emitted, func(t *testing.T) {
					value := paramFixtureValue(leaf, emitted, "did-you-mean:"+string(protection))
					rawArgs := append(strings.Fields(entry.CLIPath), "--"+emitted, value)
					ctx, runErr := pipeline.RunPreParseArgs(root, engine, rawArgs)
					if runErr != nil {
						t.Fatalf("RunPreParseArgs(%v) error = %v", rawArgs, runErr)
					}
					morphed := cmdutil.Morph(emitted)
					if ctx == nil || ctx.ProtectedFlags[morphed] != protection {
						t.Fatalf("runtime guard %q protection = %#v, want %s", emitted, ctx, protection)
					}
					assertLeftUnchanged(t, ctx, emitted, value)
					flagArgs := ctx.Args[len(strings.Fields(entry.CLIPath)):]
					if parseErr := leaf.ParseFlags(flagArgs); parseErr == nil || !strings.Contains(parseErr.Error(), "unknown flag") {
						t.Fatalf("guarded Cobra ParseFlags(%v) error = %v, want unknown flag", flagArgs, parseErr)
					}
				})
			}
		}
	}

	if chatEntries == 0 || aliasCases == 0 || guardCases[pipeline.FlagProtectionBlocked] == 0 || guardCases[pipeline.FlagProtectionAmbiguous] == 0 {
		t.Fatalf("chat parameter coverage is vacuous: entries=%d aliases=%d blocked=%d ambiguous=%d", chatEntries, aliasCases, guardCases[pipeline.FlagProtectionBlocked], guardCases[pipeline.FlagProtectionAmbiguous])
	}
	t.Logf("verified generated chat parameter routes: entries=%d aliases=%d blocked=%d ambiguous=%d", chatEntries, aliasCases, guardCases[pipeline.FlagProtectionBlocked], guardCases[pipeline.FlagProtectionAmbiguous])
}

func TestCrossPlatformCoverageIMUserIDHallucinationRoutes(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		// These paths are reduced by the reviewed user_id concept.
		{command: "chat +chat-role-query-user", want: "user"},
		{command: "chat +chat-role-set-user", want: "user"},
		{command: "chat +messages-list-direct", want: "user"},
		{command: "chat chmod", want: "user"},
		{command: "chat message list", want: "user"},
		{command: "chat message send", want: "user"},

		// These commands already own a hidden --userId compatibility flag.
		// The format/spelling handler rewrites --user-id to that real flag, and
		// the command's existing flagOrFallback wiring preserves its semantics.
		{command: "chat conversation-info", want: "userId"},
		{command: "chat group transfer-owner", want: "userId"},
		{command: "chat group-role query-user", want: "userId"},
		{command: "chat group-role remove-user", want: "userId"},
		{command: "chat group-role set-user", want: "userId"},
		{command: "chat group set-admin", want: "userId"},
		{command: "chat group-mute-member", want: "userId"},
		{command: "chat message read-status", want: "userId"},
		{command: "chat message search-advanced", want: "userId"},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			root := NewRootCommand()
			leaf := resolveParamLeaf(root, test.command)
			if leaf == nil {
				t.Fatalf("IM command %q is not runnable", test.command)
			}
			rawArgs := append(strings.Fields(test.command), "--user-id", "fixture-user")
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), rawArgs)
			if err != nil {
				t.Fatalf("RunPreParseArgs(%v) error = %v", rawArgs, err)
			}
			if ctx == nil {
				t.Fatal("RunPreParseArgs returned nil context")
			}
			flagArgs := ctx.Args[len(strings.Fields(test.command)):]
			if len(flagArgs) != 2 || flagArgs[0] != "--"+test.want || flagArgs[1] != "fixture-user" {
				t.Fatalf("--user-id route = %v, want --%s fixture-user", flagArgs, test.want)
			}
			if err := leaf.ParseFlags(flagArgs); err != nil {
				t.Fatalf("Cobra ParseFlags(%v) error = %v", flagArgs, err)
			}
		})
	}
}

func TestCrossPlatformCoverageHiddenIMListDirectRemainsOutsideCentralAliasTable(t *testing.T) {
	const command = "chat message list-direct"
	if _, ok := cli.LookupParamAlias(command); ok {
		t.Fatalf("hidden command %q unexpectedly entered the public generated alias table", command)
	}

	root := NewRootCommand()
	leaf := resolveParamLeaf(root, command)
	if leaf == nil || !leaf.Hidden {
		t.Fatalf("%q must remain a live hidden compatibility command", command)
	}
	rawArgs := append(strings.Fields(command), "--user-id", "fixture-user")
	ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), rawArgs)
	if err != nil {
		t.Fatalf("RunPreParseArgs(%v) error = %v", rawArgs, err)
	}
	if ctx == nil {
		t.Fatal("RunPreParseArgs returned nil context")
	}
	flagArgs := ctx.Args[len(strings.Fields(command)):]
	if err := leaf.ParseFlags(flagArgs); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("hidden command ParseFlags(%v) error = %v, want unknown flag", flagArgs, err)
	}
}

func TestCrossPlatformCoverageSelectedParamAliasesProduceCanonicalEquivalentDryRunPreviews(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		canonicalArgs   []string
		aliasArgs       []string
		wantCorrections int
		wantArgKeys     []string
	}{
		{
			name: "calendar read with multiple aliases",
			tool: "list_calendar_events",
			canonicalArgs: []string{
				"--dry-run", "calendar", "event", "list",
				"--start", "2026-03-10T14:00:00+08:00",
				"--end", "2026-03-10T18:00:00+08:00",
				"--calendar-id", "primary", "--limit", "7", "--cursor", "cursor-1",
			},
			aliasArgs: []string{
				"--dry-run", "calendar", "event", "list",
				"--date", "2026-03-10T14:00:00+08:00",
				"--end-time", "2026-03-10T18:00:00+08:00",
				"--calendar", "primary", "--max-results", "7", "--next-cursor", "cursor-1",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"calendarId", "cursor", "endTime", "limit", "startTime"},
		},
		{
			name: "chat write scoped recipient alias",
			tool: "send_personal_message",
			canonicalArgs: []string{
				"--dry-run", "chat", "message", "send",
				"--user", appFixtureCurrentDOpenID, "--text", "hello dry-run", "--uuid", "alias-dry-run",
			},
			aliasArgs: []string{
				"--dry-run", "chat", "message", "send",
				"--to-user", appFixtureCurrentDOpenID, "--text", "hello dry-run", "--uuid", "alias-dry-run",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"clawType", "content", "msgType", "receiverOpenDingTalkId", "uuid"},
		},
		{
			name: "mail write folder id concept alias",
			tool: "update_mail_folder",
			canonicalArgs: []string{
				"--dry-run", "mail", "folder", "update",
				"--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder",
			},
			aliasArgs: []string{
				"--dry-run", "mail", "folder", "update",
				"--email", "fixture@example.com", "--folder-id", "folder-1", "--name", "Fixture Folder",
			},
			wantCorrections: 1,
			wantArgKeys:     []string{"email", "id", "name"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, canonical, canonicalAttempts, canonicalErr := executeParamAliasDryRunE2E(t, test.canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("canonical dry-run failed: %v", canonicalErr)
			}
			ctx, alias, aliasAttempts, aliasErr := executeParamAliasDryRunE2E(t, test.aliasArgs...)
			if aliasErr != nil {
				t.Fatalf("alias dry-run failed: %v\ncontext=%#v", aliasErr, ctx)
			}

			if ctx == nil || len(ctx.Corrections) != test.wantCorrections {
				t.Fatalf("alias dry-run corrections = %#v, want %d", ctx, test.wantCorrections)
			}
			if len(canonicalAttempts) != 0 || len(aliasAttempts) != 0 {
				t.Fatalf("dry-run reached command runner\ncanonical=%#v\nalias=%#v", canonicalAttempts, aliasAttempts)
			}
			for label, preview := range map[string]paramAliasDryRunPreview{"canonical": canonical, "alias": alias} {
				if !preview.DryRun || preview.Executed {
					t.Fatalf("%s preview execution state = %#v", label, preview)
				}
				if preview.Tool != test.tool {
					t.Fatalf("%s preview tool = %q, want %q", label, preview.Tool, test.tool)
				}
				keys := make([]string, 0, len(preview.Arguments))
				for key := range preview.Arguments {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				if !reflect.DeepEqual(keys, test.wantArgKeys) {
					t.Fatalf("%s preview argument keys = %v, want %v", label, keys, test.wantArgKeys)
				}
			}
			if !reflect.DeepEqual(alias, canonical) {
				t.Fatalf("dry-run previews differ\ncanonical=%#v\nalias=%#v", canonical, alias)
			}
		})
	}
}

func TestCrossPlatformCoverageParamAliasCanonicalConflictFailsBeforeRunE(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	for _, args := range [][]string{
		{"calendar", "event", "list", "--date", "2026-03-10", "--start", "2026-03-11"},
		{"calendar", "event", "list", "--start", "2026-03-11", "--date", "2026-03-10"},
	} {
		root := NewRootCommand()
		root.SetArgs(args)
		originalCaller := helpers.GetCaller()
		helpers.InitDeps(caller)
		ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args)
		helpers.InitDeps(originalCaller)
		var conflict *pipeline.FlagConflictError
		if !stderrors.As(err, &conflict) {
			t.Fatalf("RunPreParseArgs(%v) error = %v, want FlagConflictError (ctx=%#v)", args, err, ctx)
		}
		if conflict.Canonical != "start" || !reflect.DeepEqual(conflict.Spellings, []string{"date", "start"}) {
			t.Fatalf("conflict = %#v", conflict)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("conflicting argv reached RunE/tool dispatch: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAllReviewedParamAliasGuardsReachRuntimeContract(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	paths := make(map[string]bool)
	for _, concept := range concepts.Concepts {
		for _, path := range concept.Commands {
			paths[path] = true
		}
	}
	sourceGuards := make(map[string]pipeline.FlagProtection)
	for _, override := range concepts.Overrides {
		paths[override.CommandPath] = true
		for _, emitted := range override.Block {
			sourceGuards[override.CommandPath+"\x00"+cmdutil.Morph(emitted)] = pipeline.FlagProtectionBlocked
		}
		for _, emitted := range override.Ambiguous {
			sourceGuards[override.CommandPath+"\x00"+cmdutil.Morph(emitted)] = pipeline.FlagProtectionAmbiguous
		}
	}
	orderedPaths := make([]string, 0, len(paths))
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)

	root := NewRootCommand()
	engine := newPipelineEngine()
	guardCounts := map[pipeline.FlagProtection]int{}
	testedGuards := make(map[string]pipeline.FlagProtection)
	for _, path := range orderedPaths {
		entry, ok := cli.LookupParamAlias(path)
		if !ok {
			continue
		}
		leaf := resolveParamLeaf(root, path)
		if leaf == nil {
			t.Fatalf("generated guard path %q is not runnable", path)
		}

		for _, protectionCase := range []struct {
			protection pipeline.FlagProtection
			emitted    []string
		}{
			{protection: pipeline.FlagProtectionBlocked, emitted: entry.Blocked},
			{protection: pipeline.FlagProtectionAmbiguous, emitted: entry.Ambiguous},
		} {
			for _, emitted := range protectionCase.emitted {
				protectionCase := protectionCase
				emitted := emitted
				key := path + "\x00" + cmdutil.Morph(emitted)
				if previous, duplicate := testedGuards[key]; duplicate {
					t.Fatalf("generated guard %q/%q is classified twice: %s and %s", path, emitted, previous, protectionCase.protection)
				}
				testedGuards[key] = protectionCase.protection
				guardCounts[protectionCase.protection]++

				t.Run(path+"/"+emitted, func(t *testing.T) {
					value := "FIXTURE_VALUE"
					pathArgs := strings.Fields(path)
					args := append(append([]string(nil), pathArgs...), "--"+emitted, value)
					ctx, runErr := pipeline.RunPreParseArgs(root, engine, args)
					if runErr != nil {
						t.Fatalf("RunPreParseArgs(%v) error = %v", args, runErr)
					}

					morphed := cmdutil.Morph(emitted)
					if ctx == nil || ctx.ProtectedFlags[morphed] != protectionCase.protection {
						t.Fatalf("guard protection = %#v, want %s for %q", ctx, protectionCase.protection, morphed)
					}
					assertLeftUnchanged(t, ctx, emitted, value)
					flagArgs := ctx.Args[len(pathArgs):]
					if parseErr := leaf.ParseFlags(flagArgs); parseErr == nil || !strings.Contains(parseErr.Error(), "unknown flag") {
						t.Fatalf("guarded Cobra ParseFlags(%v) error = %v, want unknown flag", flagArgs, parseErr)
					}
				})
			}
		}
	}

	for key, want := range sourceGuards {
		if got, ok := testedGuards[key]; !ok || got != want {
			t.Fatalf("reviewed source guard %q delivered as %s (present=%t), want %s", key, got, ok, want)
		}
	}
	if guardCounts[pipeline.FlagProtectionBlocked] == 0 || guardCounts[pipeline.FlagProtectionAmbiguous] == 0 {
		t.Fatalf("reviewed guard coverage is vacuous: blocked %d ambiguous %d", guardCounts[pipeline.FlagProtectionBlocked], guardCounts[pipeline.FlagProtectionAmbiguous])
	}
}

func TestCrossPlatformCoverageRepresentativeParamAliasGuardsReachFinalErrorsWithoutDispatch(t *testing.T) {
	for _, test := range []struct {
		path       string
		emitted    string
		protection pipeline.FlagProtection
		reason     string
	}{
		{path: "chat message list-by-sender", emitted: "time", protection: pipeline.FlagProtectionBlocked, reason: "blocked_flag"},
		{path: "drive list", emitted: "space", protection: pipeline.FlagProtectionAmbiguous, reason: "ambiguous_flag"},
	} {
		test := test
		t.Run(test.path+"/"+test.emitted, func(t *testing.T) {
			value := "FIXTURE_VALUE"
			args := append(strings.Fields(test.path), "--"+test.emitted, value)
			caller := &paramAliasCaptureCaller{}
			ctx, executeErr := executeParamAliasE2E(t, caller, args...)

			morphed := cmdutil.Morph(test.emitted)
			if ctx == nil || ctx.ProtectedFlags[morphed] != test.protection {
				t.Fatalf("guard protection = %#v, want %s for %q", ctx, test.protection, morphed)
			}
			assertLeftUnchanged(t, ctx, test.emitted, value)

			var appErr *apperrors.Error
			if !stderrors.As(executeErr, &appErr) {
				t.Fatalf("final error = %T %v, want *errors.Error", executeErr, executeErr)
			}
			if appErr.Category != apperrors.CategoryValidation || appErr.Reason != test.reason || apperrors.ExitCode(executeErr) != 3 {
				t.Fatalf("final error contract = category %q reason %q exit %d, want validation/%s/3", appErr.Category, appErr.Reason, apperrors.ExitCode(executeErr), test.reason)
			}
			if !strings.Contains(appErr.Message, "unknown flag: --"+test.emitted) || !strings.Contains(appErr.Message, "See 'dws "+test.path+" --help' for usage.") {
				t.Fatalf("final error message = %q", appErr.Message)
			}
			if !strings.Contains(appErr.Hint, "--"+test.emitted) || !strings.Contains(appErr.Hint, "--help") {
				t.Fatalf("final error hint = %q", appErr.Hint)
			}
			wantAction := "Run 'dws " + test.path + " --help' for valid flags"
			if !reflect.DeepEqual(appErr.Actions, []string{wantAction}) || len(appErr.AvailableFlags) == 0 || appErr.Cause == nil {
				t.Fatalf("final recovery fields = actions %v flags %v cause %v", appErr.Actions, appErr.AvailableFlags, appErr.Cause)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("guarded flag reached RunE/tool dispatch: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageFlagConflictErrorFormattingIsDeterministic(t *testing.T) {
	err := (&pipeline.FlagConflictError{Command: "dws demo", Canonical: "start", Spellings: []string{"start", "date"}}).Error()
	want := `conflicting parameter spellings for --start on "dws demo": --date, --start; pass exactly one spelling`
	if err != want {
		t.Fatalf("FlagConflictError = %q, want %q", err, want)
	}
}
