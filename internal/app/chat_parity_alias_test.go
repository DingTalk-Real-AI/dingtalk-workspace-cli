// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package app

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatParityAliasesShareCobraAndSchemaIdentity(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	for alias, primary := range map[string]string{
		"+chat-messages-list": "+chat-messages", "+messages-search": "+search-msg",
		"+threads-messages-list": "+thread-replies", "+messages-resources-download": "+messages-resource-download",
		"+message-read-users": "+messages-read-status", "+feed-shortcut-list": "+conversation-list-top",
		"+feed-group-list": "+category-list", "+feed-group-list-item": "+category-list-conversations",
	} {
		command, args, err := root.Find([]string{"chat", alias})
		if err != nil || len(args) != 0 {
			t.Fatalf("%s not runnable: %v %v", alias, err, args)
		}
		owner, _, err := root.Find([]string{"chat", primary})
		if err != nil || command != owner {
			t.Fatalf("%s differs from %s", alias, primary)
		}
		a, ok := cli.ResolveMeta("chat " + alias)
		if !ok {
			t.Fatalf("alias missing in Schema %s", alias)
		}
		b, ok := cli.ResolveMeta("chat " + primary)
		if !ok || !reflect.DeepEqual(a.Identity, b.Identity) || !reflect.DeepEqual(a.Safety, b.Safety) {
			t.Fatalf("alias Schema drift %s", alias)
		}
	}
	for _, path := range []string{
		"chat +feed-shortcut-create", "chat +feed-shortcut-remove", "chat +messages-edit",
		"chat +conversation-set-top", "chat message edit", "chat message send-by-bot", "chat message reply",
		"chat thread reply", "chat group members add-bot", "chat group share-invite", "chat message send-a2ui-card", "chat message update-a2ui-card",
	} {
		cmd, args, err := root.Find(strings.Fields(path))
		if err != nil || len(args) != 0 || !cmd.Runnable() {
			t.Fatalf("new/legacy route missing %s: %v", path, err)
		}
		if _, ok := cli.ResolveMeta(path); !ok {
			t.Fatalf("Schema missing %s", path)
		}
	}
}

func TestCrossPlatformCoverageChatParityNewResultsReachCompactSchema(t *testing.T) {
	for _, path := range []string{"chat +feed-shortcut-create", "chat +feed-shortcut-remove", "chat +messages-edit"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if full["result"] == nil || !reflect.DeepEqual(full["result"], compact["result"]) {
			t.Fatalf("%s Result not delivered identically: full=%#v compact=%#v", path, full["result"], compact["result"])
		}
	}
}

func TestCrossPlatformCoverageMgetFlagsAndSchemaMatch(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	cmd, _, err := root.Find([]string{"chat", "+messages-mget"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"msg-ids", "message-ids", "message-id"} {
		f := cmd.Flags().Lookup(name)
		if f == nil || f.Value.Type() != "stringSlice" {
			t.Fatalf("alias %s not executable as list", name)
		}
	}
	full := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-mget")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-mget", "--compact")
	if full["result"] != nil || compact["result"] != nil {
		t.Fatal("legacy mget must not publish a unified result before wire migration")
	}
	params := schemaContractMap(full["parameters"])
	if params["msg-ids"]["required"] != true || params["no-threads"]["type"] != "boolean" {
		t.Fatalf("Schema drift: %#v", params)
	}
	if !strings.Contains(schemaContractString(params["msg-ids"]["description"]), "--message-id") {
		t.Fatal("alias not discoverable")
	}
}

func TestCrossPlatformCoverageReplyOptionalConversationAndMentionsReachSchema(t *testing.T) {
	for _, path := range []string{"chat +messages-reply"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		params := schemaContractMap(full["parameters"])
		if params["group"]["required"] == true || params["group"]["property"] != "conversationId" {
			t.Fatalf("optional target drift: %#v", params["group"])
		}
		if params["content"]["required"] != true || params["at-open-dingtalk-ids"]["type"] != "array" || params["at-all"]["type"] != "boolean" {
			t.Fatal("reply surface not delivered")
		}
	}
}

func TestCrossPlatformCoverageThreadTimeKeepsHistoricalFormat(t *testing.T) {
	tool := executeShortcutSchemaQuery(t, "--cli-path", "chat +thread-replies")
	parameters := schemaContractMap(tool["parameters"])
	if got := schemaContractString(parameters["time"]["format"]); got != "" {
		t.Fatalf("historical --time accepts RFC3339 and local dates; must not narrow to %q", got)
	}
	if got := schemaContractString(parameters["page-token"]["format"]); got != "" {
		t.Fatalf("millisecond cursor must not inherit date-time format: %q", got)
	}
}
