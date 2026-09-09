// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageMessageLedgersPublishResultAndContinuation(t *testing.T) {
	if ChatMessages.OutputRollout != output.RolloutDualValidate || ChatMessages.Contract.Pagination != nil {
		t.Fatalf("chat-messages must remain dual without a misleading single cursor: rollout=%q pagination=%#v", ChatMessages.OutputRollout, ChatMessages.Contract.Pagination)
	}
	if SearchMsg.OutputRollout != output.RolloutDualValidate || SearchMsg.Contract.Pagination == nil {
		t.Fatalf("search-msg rollout/pagination = %q / %#v", SearchMsg.OutputRollout, SearchMsg.Contract.Pagination)
	}
	for _, declaration := range []struct {
		name   string
		result *contract.ResultSpec
	}{
		{name: ChatMessages.Command, result: ChatMessages.Contract.Result},
		{name: SearchMsg.Command, result: SearchMsg.Contract.Result},
	} {
		normalized, err := contract.NormalizeResultSpec(declaration.result, declaration.name)
		if err != nil {
			t.Fatalf("%s Result contract: %v", declaration.name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(normalized.DataSchema, &schema); err != nil {
			t.Fatalf("%s decode Result contract: %v", declaration.name, err)
		}
		message := schema["properties"].(map[string]any)["messages"].(map[string]any)["items"].(map[string]any)
		textSchema := message["properties"].(map[string]any)["text"].(map[string]any)
		if got := textSchema["type"]; !reflect.DeepEqual(got, []any{"string", "null"}) {
			t.Fatalf("%s message text type = %#v, want nullable string", declaration.name, got)
		}
	}

	messagePage := map[string]any{
		"hasMore":  true,
		"nextPage": map[string]any{"time": "2026-09-03T00:00:00Z", "direction": "older"},
	}
	attachChatMessageContinuation(messagePage, "chat +chat-messages")
	actions := messagePage["nextActions"].([]map[string]any)
	if len(actions) != 1 || actions[0]["cliPath"] != "chat +chat-messages" || actions[0]["reuseArguments"] != true || actions[0]["ready"] != false {
		t.Fatalf("message continuation = %#v", actions)
	}
	if patch := actions[0]["arguments"].(map[string]any); patch["time"] != "2026-09-03T00:00:00Z" || patch["direction"] != "older" {
		t.Fatalf("compatibility continuation patch = %#v", patch)
	}

	rangePage := map[string]any{
		"hasMore":  true,
		"nextPage": map[string]any{"time": "2026-09-03T00:00:00Z", "direction": "older"},
		"queryRange": map[string]any{
			"startTime": "2026-09-01T00:00:00Z",
			"endTime":   "2026-09-04T00:00:00Z",
			"order":     "desc",
		},
	}
	attachChatMessageContinuation(rangePage, "chat +chat-messages")
	rangePatch := rangePage["nextActions"].([]map[string]any)[0]["arguments"].(map[string]any)
	if rangePatch["start"] != "2026-09-01T00:00:00Z" || rangePatch["end"] != "2026-09-03T00:00:00Z" || rangePatch["order"] != "desc" {
		t.Fatalf("range continuation patch = %#v", rangePatch)
	}
	if _, conflict := rangePatch["time"]; conflict {
		t.Fatalf("range continuation introduced conflicting --time: %#v", rangePatch)
	}

	searchPage := map[string]any{"hasMore": true, "nextCursor": "cursor-2"}
	attachChatMessageContinuation(searchPage, "chat +search-msg")
	actions = searchPage["nextActions"].([]map[string]any)
	if len(actions) != 1 || actions[0]["arguments"].(map[string]any)["cursor"] != "cursor-2" {
		t.Fatalf("search continuation = %#v", actions)
	}
	root := newPlatformCoverageRoot()
	allActions := append(messagePage["nextActions"].([]map[string]any), rangePage["nextActions"].([]map[string]any)...)
	allActions = append(allActions, searchPage["nextActions"].([]map[string]any)...)
	for index, action := range allActions {
		command, remaining, err := root.Find(strings.Fields(action["cliPath"].(string)))
		if err != nil || len(remaining) != 0 || command == nil || !command.Runnable() {
			t.Fatalf("nextActions[%d] does not bind: command=%v remaining=%v err=%v", index, command, remaining, err)
		}
		for name := range action["arguments"].(map[string]any) {
			if command.Flags().Lookup(name) == nil && command.InheritedFlags().Lookup(name) == nil {
				t.Errorf("nextActions[%d] argument %q is not executable by %q", index, name, action["cliPath"])
			}
		}
	}

	complete := map[string]any{"hasMore": false}
	attachChatMessageContinuation(complete, "chat +chat-messages")
	if actions := complete["nextActions"].([]map[string]any); len(actions) != 0 {
		t.Fatalf("complete result published continuation: %#v", actions)
	}
}
