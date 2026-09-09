// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
)

func TestCrossPlatformCoverageChatActiveConversationsResultDeliveredInFullAndCompactSchema(t *testing.T) {
	const canonical = "chat.shortcut_active_conversations"
	wantResult, err := contract.NormalizeResultSpec(chat.ActiveConversations.Contract.Result, canonical)
	if err != nil || wantResult == nil {
		t.Fatalf("declared result missing or invalid: %v", err)
	}
	wantPagination, err := contract.NormalizePaginationSpec(chat.ActiveConversations.Contract.Pagination, canonical)
	if err != nil || wantPagination == nil {
		t.Fatalf("declared pagination missing or invalid: %v", err)
	}

	for _, tc := range []struct {
		name    string
		compact bool
	}{{name: "full"}, {name: "compact", compact: true}} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand()
			leaf, _, err := root.Find([]string{"chat", "+recent-conversations"})
			if err != nil {
				t.Fatal(err)
			}
			if err := leaf.ValidateRequiredFlags(); err != nil {
				t.Fatalf("Cobra still requires an explicit start: %v", err)
			}
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := []string{"schema", "--cli-path", "chat +recent-conversations", "--format", "json"}
			if tc.compact {
				args = append(args, "--compact")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("schema: %v; %s", err, stderr.String())
			}
			var payload struct {
				Result     *contract.ResultSpec     `json:"result"`
				Pagination *contract.PaginationSpec `json:"pagination"`
				Parameters map[string]struct {
					Type        string          `json:"type"`
					Required    bool            `json:"required"`
					CLIRequired bool            `json:"cli_required"`
					Default     json.RawMessage `json:"default"`
					Description string          `json:"description"`
				} `json:"parameters"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			gotResult, err := contract.NormalizeResultSpec(payload.Result, canonical)
			if err != nil || gotResult == nil {
				t.Fatalf("delivered result missing or invalid: %v", err)
			}
			if !reflect.DeepEqual(gotResult, wantResult) {
				t.Fatalf("delivered result differs from normalized declaration\ngot: %#v\nwant: %#v", gotResult, wantResult)
			}
			if !reflect.DeepEqual(payload.Pagination, wantPagination) {
				t.Fatalf("pagination = %#v, want %#v", payload.Pagination, wantPagination)
			}
			if payload.Pagination.Kind != contract.PaginationKindCursor || payload.Pagination.CursorParameter != "cursor" {
				t.Fatalf("missing cursor pagination contract: %#v", payload.Pagination)
			}
			if _, ok := payload.Parameters[payload.Pagination.CursorParameter]; !ok {
				t.Fatal("pagination cursor is not a discoverable parameter")
			}

			delay, ok := payload.Parameters["page-delay"]
			if !ok || delay.Type != "integer" {
				t.Fatalf("page-delay must be a discoverable integer parameter: %#v", delay)
			}
			// The current parameter wire contract publishes Cobra defaults as strings.
			var defaultDelay string
			if err := json.Unmarshal(delay.Default, &defaultDelay); err != nil || defaultDelay != "200" {
				t.Fatalf("page-delay default = %s, want 200: %v", delay.Default, err)
			}
			if !strings.Contains(delay.Description, "0-60000") {
				t.Fatalf("page-delay range is not discoverable: %q", delay.Description)
			}
			for _, name := range []string{"checkpoint", "resume"} {
				if _, ok := payload.Parameters[name]; ok || leaf.Flags().Lookup(name) != nil {
					t.Fatalf("removed persistence flag still exposed in Schema or Cobra: %s", name)
				}
			}
			budget, ok := payload.Parameters["total-timeout"]
			if !ok || budget.Type != "integer" || string(budget.Default) != `"300"` || !strings.Contains(budget.Description, "1-3600") {
				t.Fatalf("shared total timeout is not discoverable: %#v", budget)
			}
			if !strings.Contains(payload.Parameters["start"].Description, "整秒") ||
				!strings.Contains(payload.Parameters["start"].Description, "非零小数秒") ||
				!strings.Contains(payload.Parameters["end"].Description, "向下取整秒") {
				t.Fatalf("query time precision is not discoverable: start=%q end=%q", payload.Parameters["start"].Description, payload.Parameters["end"].Description)
			}
			start, ok := payload.Parameters["start"]
			if !ok || start.Type != "string" || start.Required || start.CLIRequired || !strings.Contains(start.Description, "24 小时") {
				t.Fatalf("optional rolling start is not discoverable: %#v", start)
			}
			// A rolling default must never be frozen into a concrete Schema date.
			if len(start.Default) != 0 && string(start.Default) != `""` {
				t.Fatalf("rolling start published a static default: %s", start.Default)
			}

			partialSupported := false
			for _, outcome := range gotResult.Outcomes {
				partialSupported = partialSupported || outcome == contract.ResultOutcomePartialFailure
			}
			if !partialSupported {
				t.Fatal("partial_failure outcome is not discoverable")
			}
			var dataSchema any
			if err := json.Unmarshal(gotResult.DataSchema, &dataSchema); err != nil {
				t.Fatal(err)
			}
			for name, wantType := range map[string]string{
				"pageSize": "integer", "name": "string", "nameKnown": "boolean",
				"succeeded": "array", "failed": "array", "failedPage": "integer", "failedCursor": "string",
			} {
				property := activeConversationsDeliveredSchemaProperty(dataSchema, name)
				if property == nil || property["type"] != wantType {
					t.Errorf("result property %s is not discoverable as %s: %#v", name, wantType, property)
				}
			}
			for _, name := range []string{"checkpointFile", "totalPagesFetched"} {
				if property := activeConversationsDeliveredSchemaProperty(dataSchema, name); property != nil {
					t.Errorf("removed persistence result property is still published: %s", name)
				}
			}
		})
	}
}

// Result branches may use oneOf or definitions; walk JSON Schema objects rather
// than binding this delivery check to a particular arrangement of those nodes.
func activeConversationsDeliveredSchemaProperty(node any, name string) map[string]any {
	switch value := node.(type) {
	case map[string]any:
		if properties, ok := value["properties"].(map[string]any); ok {
			if property, ok := properties[name].(map[string]any); ok {
				return property
			}
		}
		for _, child := range value {
			if property := activeConversationsDeliveredSchemaProperty(child, name); property != nil {
				return property
			}
		}
	case []any:
		for _, child := range value {
			if property := activeConversationsDeliveredSchemaProperty(child, name); property != nil {
				return property
			}
		}
	}
	return nil
}
