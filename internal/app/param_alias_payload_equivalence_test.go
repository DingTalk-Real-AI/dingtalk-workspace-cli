// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// paramAliasCompleteCommands is deliberately keyed by the exact reviewed
// fixture command path. Every argv is a complete, business-valid invocation:
// required companion flags are present, time and enum values are valid, and
// write commands use the capture caller rather than a real transport. The
// target canonical flag must occur exactly once so the test can replace only
// its spelling while holding every other input constant.
var paramAliasCompleteCommands = map[string][]string{
	"aitable +base-search":                     {"aitable", "+base-search", "--query", "fixture"},
	"aitable +field-get":                       {"aitable", "+field-get", "--base-id", "base-1", "--table-id", "table-1"},
	"aitable +list-tables":                     {"aitable", "+list-tables", "--base", "base-1"},
	"aitable +record-query":                    {"aitable", "+record-query", "--base-id", "base-1", "--table-id", "table-1", "--query", "fixture"},
	"aitable +record-share-url":                {"aitable", "+record-share-url", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1"},
	"aitable +table-get":                       {"aitable", "+table-get", "--base-id", "base-1"},
	"aitable record query":                     {"aitable", "record", "query", "--base-id", "base-1", "--table-id", "table-1", "--limit", "7"},
	"attendance check result":                  {"attendance", "check", "result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"attendance +check-result":                 {"attendance", "+check-result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"calendar event list":                      {"calendar", "event", "list", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"chat +bot-find":                           {"chat", "+bot-find", "--query", "fixture", "--limit", "7"},
	"chat +bot-search":                         {"chat", "+bot-search", "--name", "Fixture Bot", "--page", "2", "--size", "7"},
	"chat +category-create":                    {"chat", "+category-create", "--title", "Fixture Cat", "--yes"},
	"chat +category-rename":                    {"chat", "+category-rename", "--category-id", "7", "--title", "Renamed Cat", "--yes"},
	"chat +group-members":                      {"chat", "+group-members", "--group", "Fixture Group"},
	"chat +messages-list-direct":               {"chat", "+messages-list-direct", "--user", "user-1", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat +messages-list-unread-conversations": {"chat", "+messages-list-unread-conversations", "--count", "7", "--exclude-muted"},
	"chat +messages-send-by-webhook":           {"chat", "+messages-send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--text", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"chat +send-to-group":                      {"chat", "+send-to-group", "--group", "Fixture Group", "--text", "hello fixture", "--yes"},
	"chat +unread-chats":                       {"chat", "+unread-chats", "--count", "7", "--exclude-muted"},
	"chat bot find":                            {"chat", "bot", "find", "--query", "fixture", "--limit", "7"},
	"chat bot search":                          {"chat", "bot", "search", "--name", "Fixture Bot", "--page", "2", "--size", "7"},
	"chat category create":                     {"chat", "category", "create", "--title", "Fixture Cat", "--yes"},
	"chat category create-smart":               {"chat", "category", "create-smart", "--name", "Fixture Smart Category", "--keywords", "fixture,priority", "--yes"},
	"chat category rename":                     {"chat", "category", "rename", "--category-id", "7", "--title", "Renamed Cat", "--yes"},
	"chat group members":                       {"chat", "group", "members", "--id", "fixture-conversation"},
	"chat group members add":                   {"chat", "group", "members", "add", "--id", "fixture-conversation", "--users", "D-user-1"},
	"chat group members add-bot":               {"chat", "group", "members", "add-bot", "--id", "fixture-conversation", "--robot-code", "robot-1", "--yes"},
	"chat group members list-by-ids":           {"chat", "group", "members", "list-by-ids", "--id", "fixture-conversation", "--users", "D-user-1,D-user-2"},
	"chat group members remove":                {"chat", "group", "members", "remove", "--id", "fixture-conversation", "--users", "D-user-1", "--yes"},
	"chat group members remove-bot":            {"chat", "group", "members", "remove-bot", "--id", "fixture-conversation", "--bot-id", "bot-1", "--yes"},
	"chat group rename":                        {"chat", "group", "rename", "--id", "fixture-conversation", "--name", "Fixture Renamed Group", "--yes"},
	"chat group set-admin":                     {"chat", "group", "set-admin", "--group", "fixture-conversation", "--user", "user-1", "--yes"},
	"chat message add-emoji":                   {"chat", "message", "add-emoji", "--conversation-id", "fixture-conversation", "--msg-id", "message-1", "--emoji", "赞", "--yes"},
	"chat message add-favorite":                {"chat", "message", "add-favorite", "--open-message-id", "message-1", "--open-conversation-id", "fixture-conversation", "--yes"},
	"chat message combine-forward":             {"chat", "message", "combine-forward", "--src-conversation-id", "fixture-source", "--msg-ids", "message-1,message-2", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat message forward-topic":               {"chat", "message", "forward-topic", "--src-msg-id", "message-1", "--src-conversation-id", "fixture-source", "--src-thread-id", "convThread-fixture", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat message list":                        {"chat", "message", "list", "--group", "fixture-conversation", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat message list-all":                    {"chat", "message", "list-all", "--start", "2026-03-10 00:00:00", "--end", "2026-03-11 00:00:00"},
	"chat message list-by-sender":              {"chat", "message", "list-by-sender", "--sender-user-id", "user-1", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--limit", "7", "--cursor", "0"},
	"chat message list-favorites":              {"chat", "message", "list-favorites", "--cursor", "2", "--size", "7"},
	"chat message list-by-ids":                 {"chat", "message", "list-by-ids", "--msg-ids", "message-1,message-2"},
	"chat message list-unread-conversations":   {"chat", "message", "list-unread-conversations", "--count", "7", "--exclude-muted"},
	"chat message recall":                      {"chat", "message", "recall", "--conversation-id", "fixture-conversation", "--msg-id", "message-1", "--yes"},
	"chat message reply":                       {"chat", "message", "reply", "--conversation-id", "fixture-conversation", "--ref-msg-id", "message-1", "--ref-sender", "D-sender", "--text", "hello fixture", "--yes"},
	"chat message search-advanced":             {"chat", "message", "search-advanced", "--conversation-ids", "fixture-conversation", "--query", "fixture"},
	"chat message send":                        {"chat", "message", "send", "--user", "D-recipient", "--text", "hello fixture", "--uuid", "param-alias-equivalence", "--yes"},
	"chat message send-by-bot":                 {"chat", "message", "send-by-bot", "--robot-code", "robot-1", "--group", "fixture-conversation", "--title", "Fixture Alert", "--text", "@user-1 @user-2 fixture", "--at-user-ids", "user-1,user-2", "--yes"},
	"chat message send-by-webhook":             {"chat", "message", "send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--text", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"contact +dept-members":                    {"contact", "+dept-members", "--dept", "Fixture Dept"},
	"contact +list-sub-depts":                  {"contact", "+list-sub-depts", "--dept", "1"},
	"contact +resolve-dept":                    {"contact", "+resolve-dept", "--name", "Fixture Dept"},
	"contact +search-user":                     {"contact", "+search-user", "--query", "Fixture User"},
	"contact dept list-children":               {"contact", "dept", "list-children", "--dept", "1"},
	"contact user profile get":                 {"contact", "user", "profile", "get", "--staff-id", "user-1"},
	"dev app get":                              {"dev", "app", "get", "--unified-app-id", "app-1"},
	"devdoc article search":                    {"devdoc", "article", "search", "--query", "fixture", "--page", "2", "--size", "7"},
	"ding +receiver-status":                    {"ding", "+receiver-status", "--ding-id", "ding-1"},
	"ding message receiver-status":             {"ding", "message", "receiver-status", "--ding-id", "ding-1"},
	"ding message send":                        {"ding", "message", "send", "--robot-code", "robot-1", "--content", "fixture", "--users", "user-1", "--yes"},
	"doc +template-search":                     {"doc", "+template-search", "--query", "fixture", "--source", "MY", "--limit", "7"},
	"doc block insert":                         {"doc", "block", "insert", "--node", "node-1", "--text", "fixture paragraph", "--yes"},
	"doc block update":                         {"doc", "block", "update", "--node", "node-1", "--block-id", "block-1", "--text", "fixture paragraph", "--yes"},
	"drive info":                               {"drive", "info", "--node", "node-1", "--space-id", "space-1"},
	"drive list":                               {"drive", "list", "--folder", "folder-1", "--limit", "7"},
	"mail +find-mail-user":                     {"mail", "+find-mail-user", "--query", "fixture", "--limit", "7"},
	"mail folder update":                       {"mail", "folder", "update", "--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder", "--yes"},
	"mail message search":                      {"mail", "message", "search", "--email", "fixture@example.com", "--query", "subject:fixture"},
	"mail thread list":                         {"mail", "thread", "list", "--email", "fixture@example.com", "--folder", "folder-1", "--limit", "7"},
	"mail user search":                         {"mail", "user", "search", "--keyword", "fixture"},
	"oa +list-executed":                        {"oa", "+list-executed", "--limit", "7", "--page", "1"},
	"oa +search-forms":                         {"oa", "+search-forms", "--query", "fixture"},
	"oa approval search-forms":                 {"oa", "approval", "search-forms", "--query", "fixture"},
	"report list":                              {"report", "list", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-10T23:59:59+08:00"},
}

// A command can expose more than one mutually exclusive canonical route. In
// that case the shared command template above cannot contain every canonical
// flag at once, so select a fixture-specific complete invocation here.
var paramAliasCompleteCommandVariants = map[string]map[string][]string{
	"chat message list": {
		"user": {"chat", "message", "list", "--user", "user-1", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	},
	"chat message list-by-sender": {
		"sender-open-dingtalk-id": {"chat", "message", "list-by-sender", "--sender-open-dingtalk-id", "D-sender", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--limit", "7", "--cursor", "0"},
	},
	"chat message send": {
		"group":     {"chat", "message", "send", "--group", "fixture-conversation", "--text", "hello fixture", "--uuid", "param-alias-equivalence-group", "--yes"},
		"file-path": {"chat", "message", "send", "--group", "fixture-conversation", "--msg-type", "file", "--file-path", "../../go.mod", "--dentry-id", "1", "--space-id", "2", "--uuid", "param-alias-equivalence-file", "--yes"},
	},
}

// paramAliasNewIMCases is the exact set of aliases added by the reviewed IM
// optimization. The dedicated gate below requires every one to remain active
// in the embedded generated table and equivalent at the final transport.
var paramAliasNewIMCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "chat +bot-find", emitted: "name", canonical: "query"},
	{command: "chat bot find", emitted: "name", canonical: "query"},
	{command: "chat +bot-search", emitted: "query", canonical: "name"},
	{command: "chat +bot-search", emitted: "current-page", canonical: "page"},
	{command: "chat +category-create", emitted: "name", canonical: "title"},
	{command: "chat +category-rename", emitted: "name", canonical: "title"},
	{command: "chat +messages-list-direct", emitted: "start", canonical: "time"},
	{command: "chat +messages-list-unread-conversations", emitted: "limit", canonical: "count"},
	{command: "chat +messages-list-unread-conversations", emitted: "size", canonical: "count"},
	{command: "chat +messages-send-by-webhook", emitted: "at-user-ids", canonical: "at-users"},
	{command: "chat +unread-chats", emitted: "limit", canonical: "count"},
	{command: "chat +unread-chats", emitted: "size", canonical: "count"},
	{command: "chat bot search", emitted: "query", canonical: "name"},
	{command: "chat bot search", emitted: "current-page", canonical: "page"},
	{command: "chat category create", emitted: "name", canonical: "title"},
	{command: "chat category create-smart", emitted: "title", canonical: "name"},
	{command: "chat category rename", emitted: "name", canonical: "title"},
	{command: "chat message list", emitted: "start", canonical: "time"},
	{command: "chat message list-by-sender", emitted: "user-id", canonical: "sender-user-id"},
	{command: "chat message list-by-sender", emitted: "open-dingtalk-id", canonical: "sender-open-dingtalk-id"},
	{command: "chat message list-favorites", emitted: "limit", canonical: "size"},
	{command: "chat message list-unread-conversations", emitted: "limit", canonical: "count"},
	{command: "chat message list-unread-conversations", emitted: "size", canonical: "count"},
	{command: "chat message send", emitted: "file", canonical: "file-path"},
	{command: "chat message send-by-bot", emitted: "at-users", canonical: "at-user-ids"},
	{command: "chat message send-by-webhook", emitted: "at-user-ids", canonical: "at-users"},
}

// paramAliasRepresentativePayloadCases keeps final transport coverage across
// old concept aliases, command overrides, native compatibility flags, read and
// write commands, and different products. Every reviewed alias is still
// checked through the embedded PreParse delivery path and against a complete
// business-valid command template. The separate IM gate below continues to
// execute every alias introduced by the current IM optimization.
//
// Keeping the older 100+ aliases at the contract layer avoids rebuilding and
// executing the complete 800+ command Root twice per spelling under -race.
// That duplicated command construction was enough to push the pre-existing
// macOS app suite beyond its package-level 10-minute timeout.
var paramAliasRepresentativePayloadCases = map[string]bool{
	paramAliasPayloadCaseKey("aitable +record-query", "base"):         true, // concept alias on a shortcut read
	paramAliasPayloadCaseKey("attendance check result", "user-ids"):   true, // list-valued concept alias
	paramAliasPayloadCaseKey("calendar event list", "date"):           true, // time concept alias
	paramAliasPayloadCaseKey("chat message add-favorite", "msg-id"):   true, // scoped IM identifier alias
	paramAliasPayloadCaseKey("contact user profile get", "user-id"):   true, // native compatibility flag
	paramAliasPayloadCaseKey("devdoc article search", "current-page"): true, // command override
	paramAliasPayloadCaseKey("mail folder update", "folder-id"):       true, // write-command identifier alias
	paramAliasPayloadCaseKey("report list", "from-date"):              true, // date-range concept alias
}

func TestReviewedParamAliasesHaveCompleteTemplatesAndRepresentativeFinalPayloads(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	activeCommands := make(map[string]bool)
	activeCases := 0
	executedRepresentatives := make(map[string]bool)
	for _, fixture := range concepts.Fixture {
		if strings.HasPrefix(fixture.Expect, "did-you-mean:") {
			continue
		}
		activeCommands[fixture.Command] = true
		activeCases++
		complete, ok := paramAliasCompleteCommand(fixture.Command, fixture.Expect)
		if !ok {
			t.Errorf("reviewed active fixture %q/%q has no complete-command E2E template", fixture.Command, fixture.Emitted)
			continue
		}
		canonicalArgs := append([]string(nil), complete...)
		aliasArgs, replacements := replaceLongFlag(canonicalArgs, fixture.Expect, fixture.Emitted)
		if replacements != 1 {
			t.Errorf("complete command for %q/%q must contain canonical --%s exactly once; replacements=%d args=%v", fixture.Command, fixture.Emitted, fixture.Expect, replacements, canonicalArgs)
			continue
		}

		caseKey := paramAliasPayloadCaseKey(fixture.Command, fixture.Emitted)
		if !paramAliasRepresentativePayloadCases[caseKey] {
			continue
		}
		executedRepresentatives[caseKey] = true
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			aliasCaller := &paramAliasCaptureCaller{}
			ctx, aliasErr := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if aliasErr != nil {
				t.Fatalf("complete alias command failed: %v\nargs=%v\ncalls=%#v", aliasErr, aliasArgs, aliasCaller.calls)
			}
			if ctx == nil {
				t.Fatal("complete alias command skipped PreParse")
			}
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}

	if activeCases == 0 {
		t.Fatal("reviewed fixture contains no active alias cases")
	}
	for command := range paramAliasCompleteCommands {
		if !activeCommands[command] {
			t.Errorf("complete-command E2E template %q has no active reviewed fixture", command)
		}
	}
	for command := range activeCommands {
		if _, ok := paramAliasCompleteCommands[command]; !ok {
			t.Errorf("active reviewed command %q has no complete-command E2E template", command)
		}
	}
	if len(activeCommands) != len(paramAliasCompleteCommands) {
		t.Fatalf("complete-command coverage = %d templates for %d active commands (%d active cases)", len(paramAliasCompleteCommands), len(activeCommands), activeCases)
	}
	for caseKey := range paramAliasRepresentativePayloadCases {
		if !executedRepresentatives[caseKey] {
			t.Errorf("representative final-payload case %q has no active reviewed fixture", caseKey)
		}
	}
	if len(executedRepresentatives) != len(paramAliasRepresentativePayloadCases) {
		t.Fatalf("representative final-payload coverage = %d, want %d", len(executedRepresentatives), len(paramAliasRepresentativePayloadCases))
	}
}

func TestNewIMParamAliasesReachCanonicalEquivalentFinalPayloads(t *testing.T) {
	activeAliases := 0
	for _, test := range paramAliasNewIMCases {
		test := test
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(test.command, test.canonical)
			if !ok {
				t.Fatal("reviewed IM alias has no complete-command E2E template")
			}
			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, test.canonical, test.emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", test.canonical, replacements, canonicalArgs)
			}

			canonicalCaller := &paramAliasCaptureCaller{}
			if _, err := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...); err != nil {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", err, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			entry, exists := cli.LookupParamAlias(test.command)
			target, active := entry.ResolveAlias(test.emitted)
			if !exists || !active {
				return
			}
			if target != test.canonical {
				t.Fatalf("active reviewed IM alias --%s resolves to --%s, want --%s", test.emitted, target, test.canonical)
			}
			activeAliases++
			aliasCaller := &paramAliasCaptureCaller{}
			ctx, err := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if err != nil {
				t.Fatalf("complete alias command failed: %v\nargs=%v\ncalls=%#v", err, aliasArgs, aliasCaller.calls)
			}
			if ctx == nil {
				t.Fatal("complete alias command skipped PreParse")
			}
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}
	if activeAliases != len(paramAliasNewIMCases) {
		t.Fatalf("new IM aliases active in embedded table = %d, want %d", activeAliases, len(paramAliasNewIMCases))
	}
}

func paramAliasCompleteCommand(command, canonical string) ([]string, bool) {
	complete, ok := paramAliasCompleteCommands[command]
	if variants := paramAliasCompleteCommandVariants[command]; variants != nil {
		if variant, exists := variants[canonical]; exists {
			return variant, true
		}
	}
	return complete, ok
}

func paramAliasPayloadCaseKey(command, emitted string) string {
	return command + "\x00" + emitted
}

func executeParamAliasPayloadE2E(t *testing.T, caller *paramAliasCaptureCaller, args ...string) (*pipeline.Context, error) {
	t.Helper()
	return executeParamAliasE2E(t, caller, args...)
}

func replaceLongFlag(args []string, canonical, emitted string) ([]string, int) {
	out := append([]string(nil), args...)
	replacements := 0
	for index, arg := range out {
		if arg == "--"+canonical {
			out[index] = "--" + emitted
			replacements++
			continue
		}
		if strings.HasPrefix(arg, "--"+canonical+"=") {
			out[index] = "--" + emitted + strings.TrimPrefix(arg, "--"+canonical)
			replacements++
		}
	}
	return out, replacements
}
