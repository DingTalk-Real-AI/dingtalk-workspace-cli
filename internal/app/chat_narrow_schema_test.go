// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageChatNarrowSchemaReplacesRoutineHelp(t *testing.T) {
	for _, testCase := range []struct {
		path             string
		parameters       []string
		resultProperties []string
		sensitivePaths   []string
		pagination       bool
	}{
		{
			path:             "chat +conversation-list",
			parameters:       []string{"limit", "cursor", "page-all"},
			resultProperties: []string{"conversations", "conversations[].openConversationId", "complete", "truncatedByPageLimit", "truncatedByResultLimit", "nextActions"},
			pagination:       true,
		},
		{
			path:             "chat +chat-messages",
			parameters:       []string{"group", "user", "time", "page-all"},
			resultProperties: []string{"messages", "messages[].messageId", "messages[].conversationId", "messages[].sender", "messages[].text", "messages[].createTime", "complete", "nextActions"},
		},
		{
			path:             "chat +search-msg",
			parameters:       []string{"query", "group", "sender", "days", "cursor"},
			resultProperties: []string{"messages", "messages[].messageId", "messages[].conversationId", "messages[].sender", "messages[].text", "messages[].createTime", "complete", "warnings", "resourceDownloads", "nextActions"},
			pagination:       true,
		},
		{
			path:             "chat message list-all",
			parameters:       []string{"start", "end", "cursor", "page-all", "no-reactions"},
			resultProperties: []string{"result", "result.conversationMessagesList", "count", "complete", "queryRange", "nextActions"},
			pagination:       true,
		},
		{
			path:             "chat +messages-resource-url",
			parameters:       []string{"resource-id", "message-id", "open-conversation-id"},
			resultProperties: []string{"resourceUrl", "headers"},
			sensitivePaths:   []string{"resourceUrl", "headers", "result.resourceUrl", "result.headers"},
		},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			root := NewRootCommand()
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stdout)
			root.SetArgs([]string{
				"schema", "--cli-path", testCase.path, "--compact",
				"--jq", "{cli_path,parameters,constraints,confirmation}", "--format", "json",
			})
			if err := root.Execute(); err != nil {
				t.Fatalf("narrow parameter Schema: %v\n%s", err, stdout.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("decode narrow parameter Schema: %v\n%s", err, stdout.String())
			}
			if payload["cli_path"] != testCase.path || payload["confirmation"] != "not_required" {
				t.Fatalf("narrow parameter Schema = %#v", payload)
			}
			parameters, ok := payload["parameters"].(map[string]any)
			if !ok {
				t.Fatalf("parameters = %#v", payload["parameters"])
			}
			for _, parameter := range testCase.parameters {
				if _, exists := parameters[parameter]; !exists {
					t.Errorf("%s missing parameter %q", testCase.path, parameter)
				}
			}
			if _, leaked := payload["description"]; leaked {
				t.Fatalf("narrow projection leaked unrequested description: %#v", payload)
			}

			// These leaves entered the mandatory dual-validation phase from
			// legacy_only. Catalog intentionally withholds Result/Pagination until
			// activation; verify the reviewed declaration on ContractFinal instead
			// of weakening that publication gate or changing the public wire.
			command, remaining, err := root.Find(strings.Fields(testCase.path))
			if err != nil || command == nil || len(remaining) != 0 || !command.Runnable() {
				t.Fatalf("Result declaration route %q does not bind: command=%v remaining=%v err=%v", testCase.path, command, remaining, err)
			}
			if rollout := output.CommandRollout(command); rollout != output.RolloutDualValidate {
				t.Fatalf("%s rollout = %q, want dual_validate", testCase.path, rollout)
			}
			final, ok := contractfinal.RuntimeContractFinal(command)
			if !ok || final.Result == nil {
				t.Fatalf("%s missing declared ContractFinal Result: %#v", testCase.path, final)
			}
			normalized, err := contract.NormalizeResultSpec(final.Result, testCase.path)
			if err != nil {
				t.Fatalf("%s normalize declared Result: %v", testCase.path, err)
			}
			if (final.Pagination != nil) != testCase.pagination {
				t.Fatalf("%s declared Pagination presence=%v, want %v", testCase.path, final.Pagination != nil, testCase.pagination)
			}
			var dataSchema map[string]any
			if err := json.Unmarshal(normalized.DataSchema, &dataSchema); err != nil {
				t.Fatalf("%s decode declared data_schema: %v", testCase.path, err)
			}
			for _, propertyPath := range testCase.resultProperties {
				if !resultSchemaHasProperty(dataSchema, propertyPath) {
					t.Errorf("%s declared Result missing property %q", testCase.path, propertyPath)
				}
			}
			if len(testCase.sensitivePaths) > 0 {
				got := make(map[string]bool)
				for _, path := range normalized.SensitivePaths {
					got[path] = true
				}
				for _, path := range testCase.sensitivePaths {
					if !got[path] {
						t.Errorf("%s Result missing sensitive path %q", testCase.path, path)
					}
				}
			}
		})
	}
}

func resultSchemaHasProperty(schema map[string]any, path string) bool {
	for _, segment := range strings.Split(path, ".") {
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return false
		}
		isArray := strings.HasSuffix(segment, "[]")
		segment = strings.TrimSuffix(segment, "[]")
		next, ok := properties[segment].(map[string]any)
		if !ok {
			return false
		}
		if isArray {
			items, ok := next["items"].(map[string]any)
			if !ok {
				return false
			}
			next = items
		}
		schema = next
	}
	return true
}

func TestCrossPlatformCoverageChatResultNextActionRoutesAreExecutable(t *testing.T) {
	root := NewRootCommand()
	for cliPath, arguments := range map[string][]string{
		"chat +conversation-list": {"cursor", "limit"},
		"chat +chat-messages":     {"group", "direction", "time"},
		"chat +search-msg":        {"cursor"},
		"chat message list-all":   {"start", "end", "cursor", "page-all"},
	} {
		command, remaining, err := root.Find(strings.Fields(cliPath))
		if err != nil || len(remaining) != 0 || command == nil || !command.Runnable() {
			t.Fatalf("nextAction route %q does not bind: command=%v remaining=%v err=%v", cliPath, command, remaining, err)
		}
		for _, argument := range arguments {
			if command.Flags().Lookup(argument) == nil && command.InheritedFlags().Lookup(argument) == nil {
				t.Errorf("nextAction route %q does not accept argument %q", cliPath, argument)
			}
		}
	}
}

func TestCrossPlatformCoverageChatLifecycleGoldenRoutesUsePublicSchemaFlags(t *testing.T) {
	for _, testCase := range []struct {
		path         string
		flags        []string
		confirmation string
	}{
		{path: "chat +chat-create", flags: []string{"name", "member-query", "users"}, confirmation: "user_required"},
		{path: "chat +chat-dismiss", flags: []string{"group"}, confirmation: "user_required"},
		{path: "chat +chat-role-add", flags: []string{"group", "name"}, confirmation: "user_required"},
		{path: "chat +chat-role-set-user", flags: []string{"group", "user", "role-ids"}, confirmation: "user_required"},
		{path: "chat +chat-role-query-user", flags: []string{"group", "user"}, confirmation: "not_required"},
		{path: "chat +chat-role-remove-user", flags: []string{"group", "user", "role-ids"}, confirmation: "user_required"},
		{path: "chat +chat-role-remove", flags: []string{"group", "role-id"}, confirmation: "user_required"},
		{path: "chat +category-create", flags: []string{"title"}, confirmation: "user_required"},
		{path: "chat +category-add-conversation", flags: []string{"group", "category-ids"}, confirmation: "user_required"},
		{path: "chat +category-list-conversations", flags: []string{"category-id"}, confirmation: "not_required"},
		{path: "chat +category-remove-conversation", flags: []string{"group", "category-ids"}, confirmation: "user_required"},
		{path: "chat +category-rename", flags: []string{"category-id", "title"}, confirmation: "user_required"},
		{path: "chat +category-delete", flags: []string{"category-id"}, confirmation: "user_required"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			root := NewRootCommand()
			command, remaining, err := root.Find(strings.Fields(testCase.path))
			if err != nil || command == nil || len(remaining) != 0 || !command.Runnable() {
				t.Fatalf("Golden Route leaf does not bind: command=%v remaining=%v err=%v", command, remaining, err)
			}
			for _, name := range testCase.flags {
				flag := command.Flags().Lookup(name)
				if flag == nil {
					flag = command.InheritedFlags().Lookup(name)
				}
				if flag == nil {
					t.Fatalf("public Golden Route flag --%s does not exist", name)
				}
				if flag.Hidden {
					t.Fatalf("Golden Route leaked hidden flag --%s", name)
				}
			}

			var stdout bytes.Buffer
			root = NewRootCommand()
			root.SetOut(&stdout)
			root.SetErr(&stdout)
			root.SetArgs([]string{
				"schema", "--cli-path", testCase.path, "--compact",
				"--jq", "{cli_path,parameters,constraints,confirmation}", "--format", "json",
			})
			if err := root.Execute(); err != nil {
				t.Fatalf("narrow lifecycle Schema: %v\n%s", err, stdout.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("decode lifecycle Schema: %v\n%s", err, stdout.String())
			}
			if payload["cli_path"] != testCase.path || payload["confirmation"] != testCase.confirmation {
				t.Fatalf("lifecycle Schema identity/safety = %#v", payload)
			}
			parameters, ok := payload["parameters"].(map[string]any)
			if !ok {
				t.Fatalf("lifecycle parameters = %#v", payload["parameters"])
			}
			for _, name := range testCase.flags {
				if _, exists := parameters[name]; !exists {
					t.Errorf("narrow lifecycle Schema missing --%s", name)
				}
			}
		})
	}
}
