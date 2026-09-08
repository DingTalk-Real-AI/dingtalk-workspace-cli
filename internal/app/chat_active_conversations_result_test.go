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
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := []string{"schema", "--cli-path", "chat +active-conversations", "--format", "json"}
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
			if !strings.Contains(payload.Parameters["start"].Description, "整秒") ||
				!strings.Contains(payload.Parameters["start"].Description, "非零小数秒") ||
				!strings.Contains(payload.Parameters["end"].Description, "向下取整秒") {
				t.Fatalf("query time precision is not discoverable: start=%q end=%q", payload.Parameters["start"].Description, payload.Parameters["end"].Description)
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
