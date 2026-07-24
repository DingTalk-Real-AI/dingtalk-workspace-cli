// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

// paramAliasCompleteCommands is deliberately keyed by the exact reviewed
// fixture command path. Every argv is a complete, business-valid invocation:
// required companion flags are present, time and enum values are valid, and
// write commands use the capture caller rather than a real transport. The
// target canonical flag must occur exactly once so the test can replace only
// its spelling while holding every other input constant.
var paramAliasCompleteCommands = map[string][]string{
	"aitable +base-search":         {"aitable", "+base-search", "--query", "fixture"},
	"aitable +field-get":           {"aitable", "+field-get", "--base-id", "base-1", "--table-id", "table-1"},
	"aitable +list-tables":         {"aitable", "+list-tables", "--base", "base-1"},
	"aitable +record-query":        {"aitable", "+record-query", "--base-id", "base-1", "--table-id", "table-1", "--query", "fixture"},
	"aitable +record-share-url":    {"aitable", "+record-share-url", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1"},
	"aitable +table-get":           {"aitable", "+table-get", "--base-id", "base-1"},
	"aitable record query":         {"aitable", "record", "query", "--base-id", "base-1", "--table-id", "table-1", "--limit", "7"},
	"attendance check result":      {"attendance", "check", "result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"calendar event list":          {"calendar", "event", "list", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"chat +group-members":          {"chat", "+group-members", "--group", "Fixture Group"},
	"chat group members":           {"chat", "group", "members", "--id", "fixture-conversation"},
	"chat group members add":       {"chat", "group", "members", "add", "--id", "fixture-conversation", "--users", "D-user-1"},
	"chat group members remove":    {"chat", "group", "members", "remove", "--id", "fixture-conversation", "--users", "D-user-1", "--yes"},
	"chat group rename":            {"chat", "group", "rename", "--id", "fixture-conversation", "--name", "Fixture Renamed Group", "--yes"},
	"chat group set-admin":         {"chat", "group", "set-admin", "--group", "fixture-conversation", "--user", "user-1", "--yes"},
	"chat message list":            {"chat", "message", "list", "--group", "fixture-conversation", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat message list-all":        {"chat", "message", "list-all", "--start", "2026-03-10 00:00:00", "--end", "2026-03-11 00:00:00"},
	"chat message search-advanced": {"chat", "message", "search-advanced", "--conversation-ids", "fixture-conversation", "--query", "fixture"},
	"chat message send":            {"chat", "message", "send", "--user", "D-recipient", "--text", "hello fixture", "--uuid", "param-alias-equivalence", "--yes"},
	"contact +dept-members":        {"contact", "+dept-members", "--dept", "Fixture Dept"},
	"contact +list-sub-depts":      {"contact", "+list-sub-depts", "--dept", "1"},
	"contact +resolve-dept":        {"contact", "+resolve-dept", "--name", "Fixture Dept"},
	"contact +search-user":         {"contact", "+search-user", "--query", "Fixture User"},
	"contact dept list-children":   {"contact", "dept", "list-children", "--dept", "1"},
	"contact user profile get":     {"contact", "user", "profile", "get", "--staff-id", "user-1"},
	"dev app get":                  {"dev", "app", "get", "--unified-app-id", "app-1"},
	"devdoc article search":        {"devdoc", "article", "search", "--query", "fixture", "--page", "2", "--size", "7"},
	"ding +receiver-status":        {"ding", "+receiver-status", "--ding-id", "ding-1"},
	"ding message receiver-status": {"ding", "message", "receiver-status", "--ding-id", "ding-1"},
	"ding message send":            {"ding", "message", "send", "--robot-code", "robot-1", "--content", "fixture", "--users", "user-1", "--yes"},
	"doc +template-search":         {"doc", "+template-search", "--query", "fixture", "--source", "MY", "--limit", "7"},
	"doc block insert":             {"doc", "block", "insert", "--node", "node-1", "--text", "fixture paragraph", "--yes"},
	"doc block update":             {"doc", "block", "update", "--node", "node-1", "--block-id", "block-1", "--text", "fixture paragraph", "--yes"},
	"drive info":                   {"drive", "info", "--node", "node-1", "--space-id", "space-1"},
	"drive list":                   {"drive", "list", "--folder", "folder-1", "--limit", "7"},
	"mail +find-mail-user":         {"mail", "+find-mail-user", "--query", "fixture", "--limit", "7"},
	"mail folder update":           {"mail", "folder", "update", "--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder", "--yes"},
	"mail message search":          {"mail", "message", "search", "--email", "fixture@example.com", "--query", "subject:fixture"},
	"mail thread list":             {"mail", "thread", "list", "--email", "fixture@example.com", "--folder", "folder-1", "--limit", "7"},
	"mail user search":             {"mail", "user", "search", "--keyword", "fixture"},
	"oa +list-executed":            {"oa", "+list-executed", "--limit", "7", "--page", "1"},
	"oa +search-forms":             {"oa", "+search-forms", "--query", "fixture"},
	"oa approval search-forms":     {"oa", "approval", "search-forms", "--query", "fixture"},
	"report list":                  {"report", "list", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-10T23:59:59+08:00"},
}

func TestReviewedParamAliasesProduceCanonicalEquivalentFinalPayloads(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	activeCommands := make(map[string]bool)
	activeCases := 0
	for _, fixture := range concepts.Fixture {
		if strings.HasPrefix(fixture.Expect, "did-you-mean:") {
			continue
		}
		activeCommands[fixture.Command] = true
		activeCases++
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommands[fixture.Command]
			if !ok {
				t.Fatalf("reviewed active fixture has no complete-command E2E template")
			}
			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, fixture.Expect, fixture.Emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", fixture.Expect, replacements, canonicalArgs)
			}

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			aliasCaller := &paramAliasCaptureCaller{}
			ctx, aliasErr := executeParamAliasE2E(t, aliasCaller, aliasArgs...)
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
	if len(activeCommands) != len(paramAliasCompleteCommands) {
		t.Fatalf("complete-command coverage = %d templates for %d active commands (%d active cases)", len(paramAliasCompleteCommands), len(activeCommands), activeCases)
	}
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
