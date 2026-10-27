// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// 群号/邀请链接三命令必须经最终 Schema 投递路径发布非空 Result，
// 且 full 与 compact 投影一致、运行时确实处于统一输出。
func TestCrossPlatformCoverageChatGroupJoinResultDelivery(t *testing.T) {
	for _, tc := range []struct {
		path       string
		effect     string
		properties []string
		outcomes   []string
	}{
		{"chat group join-by-group-id", "write", []string{"openConversationId", "status", "isMember", "alreadyMember"}, []string{"success", "pending", "failure"}},
		{"chat group get-info-by-invite-url", "read", []string{"groupInfo", "isMember"}, []string{"success", "failure"}},
		{"chat group join-by-invite-url", "write", []string{"openConversationId", "status", "isMember", "alreadyMember"}, []string{"success", "pending", "failure"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", tc.path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", tc.path, "--compact")
			result, ok := full["result"].(map[string]any)
			if !ok || len(result) == 0 {
				t.Fatalf("result must be published: %#v", full["result"])
			}
			if !schemaContractJSONEqual(result, compact["result"]) {
				t.Fatalf("result must reach both projections: full=%#v compact=%#v", full["result"], compact["result"])
			}
			if !schemaContractJSONEqual(result["outcomes"], tc.outcomes) {
				t.Fatalf("outcomes=%#v", result["outcomes"])
			}
			schema, ok := result["data_schema"].(map[string]any)
			if !ok {
				t.Fatalf("data_schema missing: %#v", result)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("data_schema.properties missing: %#v", schema)
			}
			for _, name := range []string{"success", "result", "errorCode", "errorMsg", "arguments"} {
				if _, ok := properties[name]; !ok {
					t.Fatalf("data_schema missing %q: %#v", name, properties)
				}
			}
			business, ok := properties["result"].(map[string]any)
			if !ok || business["type"] != "object" {
				t.Fatalf("data_schema.result must describe the downstream object: %#v", properties["result"])
			}
			businessProperties, ok := business["properties"].(map[string]any)
			if !ok {
				t.Fatalf("data_schema.result.properties missing: %#v", business)
			}
			for _, name := range tc.properties {
				if _, ok := properties[name]; ok {
					t.Fatalf("business field %q must be below data.result: %#v", name, properties)
				}
				if _, ok := businessProperties[name]; !ok {
					t.Fatalf("data_schema.result missing %q: %#v", name, businessProperties)
				}
			}
			if len(schemaContractStringSlice(schema["required"])) != 0 || len(schemaContractStringSlice(business["required"])) != 0 {
				t.Fatalf("optional downstream fields must not be required: %#v", schema)
			}
			if full["effect"] != tc.effect || full["confirmation"] != "not_required" {
				t.Fatalf("safety contract: effect=%#v confirmation=%#v", full["effect"], full["confirmation"])
			}
			root := NewRootCommand()
			cmd, _, err := root.Find(strings.Fields(tc.path))
			if err != nil || !output.UsesUnifiedResult(cmd) {
				t.Fatalf("Result published without unified runtime: %v", err)
			}
		})
	}
}
