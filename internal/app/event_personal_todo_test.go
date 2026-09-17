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
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoveragePersonalTodoEventListAndSchema(t *testing.T) {
	list := newEventListCommand()
	list.SilenceUsage = true
	list.SilenceErrors = true
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"--category", "todo"})
	if err := list.Execute(); err != nil {
		t.Fatalf("event list --category todo error = %v", err)
	}

	tests := []struct {
		eventKey   string
		properties []string
	}{
		{
			eventKey: personal.EventTodoTaskCreated,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "task_id", "subject", "creator_id",
				"executor_ids", "participant_ids", "priority", "status_stage", "plan_start_date",
				"plan_finish_date", "start_date", "finish_date", "description", "source", "source_id",
				"biz_tag", "parent_id", "is_multi_executor", "scene_type", "create_time",
			},
		},
		{
			eventKey: personal.EventTodoTaskUpdated,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "task_id", "subject", "creator_id",
				"executor_ids", "participant_ids", "priority", "status_stage", "old_status_stage",
				"plan_start_date", "plan_finish_date", "start_date", "finish_date", "description", "source",
				"source_id", "biz_tag", "parent_id", "is_multi_executor", "scene_type", "create_time", "update_time",
			},
		},
		{
			eventKey: personal.EventTodoTaskDeleted,
			properties: []string{
				"type", "event_id", "timestamp", "subscribe_id", "task_id", "subject", "creator_id", "create_time", "delete_time",
			},
		},
	}

	for _, tt := range tests {
		if !strings.Contains(listOut.String(), tt.eventKey) {
			t.Fatalf("Todo event list missing %s:\n%s", tt.eventKey, listOut.String())
		}
		schema := newEventSchemaCommand()
		schema.SilenceUsage = true
		schema.SilenceErrors = true
		var schemaOut bytes.Buffer
		schema.SetOut(&schemaOut)
		schema.SetArgs([]string{tt.eventKey, "--flatten"})
		if err := schema.Execute(); err != nil {
			t.Fatalf("event schema %s --flatten error = %v", tt.eventKey, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(schemaOut.Bytes(), &doc); err != nil {
			t.Fatalf("decode schema for %s: %v\n%s", tt.eventKey, err, schemaOut.String())
		}
		if doc["event_key"] != tt.eventKey || doc["category"] != "todo" || doc["rule_type"] != "all" || doc["jq_root_path"] != "." {
			t.Fatalf("schema document for %s = %#v", tt.eventKey, doc)
		}
		schemaBody, ok := doc["schema"].(map[string]any)
		if !ok {
			t.Fatalf("schema body for %s = %#v", tt.eventKey, doc["schema"])
		}
		properties, ok := schemaBody["properties"].(map[string]any)
		if !ok || len(properties) != len(tt.properties) {
			t.Fatalf("schema properties for %s = %#v, want %d fields", tt.eventKey, schemaBody["properties"], len(tt.properties))
		}
		for _, name := range tt.properties {
			if _, ok := properties[name]; !ok {
				t.Fatalf("schema properties for %s missing %s: %#v", tt.eventKey, name, properties)
			}
		}
	}
}

func TestCrossPlatformCoveragePersonalTodoRoleTypesCobraWiring(t *testing.T) {
	var got personalConsumeOptions
	testseam.Swap(t, &eventRunPersonalConsume, func(_ *cobra.Command, opts personalConsumeOptions) error {
		got = opts
		return nil
	})

	cmd := newEventConsumeCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{personal.EventTodoTaskCreated, "--role-types", "executor,creator", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("event consume Todo wiring error = %v", err)
	}
	if got.EventKey != personal.EventTodoTaskCreated || !reflect.DeepEqual(got.RoleTypes, []string{"executor", "creator"}) || !got.Common.DryRun {
		t.Fatalf("captured Todo options = %#v", got)
	}
}

func TestCrossPlatformCoverageEventConsumeSchemaIncludesTodoRoleTypes(t *testing.T) {
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"schema", "event consume"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema event consume Execute() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("schema output is not JSON: %v\n%s", err, out.String())
	}
	params, ok := doc["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("schema parameters = %#v", doc["parameters"])
	}
	roleTypes, ok := params["role-types"].(map[string]any)
	if !ok {
		t.Fatalf("schema parameters missing role-types: %#v", params)
	}
	if roleTypes["type"] != "array" || roleTypes["required"] != false {
		t.Fatalf("role-types schema = %#v, want optional array", roleTypes)
	}
	wantEnum := []any{"creator", "executor", "participant"}
	if !reflect.DeepEqual(roleTypes["enum"], wantEnum) {
		t.Fatalf("role-types enum = %#v, want %#v", roleTypes["enum"], wantEnum)
	}
	description, _ := roleTypes["description"].(string)
	for _, want := range []string{"creator", "executor", "participant"} {
		if !strings.Contains(description, want) {
			t.Fatalf("role-types description = %q, missing %q", description, want)
		}
	}
	provenance, ok := roleTypes["field_provenance"].(map[string]any)
	if !ok {
		t.Fatalf("role-types field provenance = %#v", roleTypes["field_provenance"])
	}
	property, ok := provenance["property"].(map[string]any)
	if !ok || property["source"] != "native_annotation" || property["value"] != "roleTypes" {
		t.Fatalf("role-types property provenance = %#v", provenance["property"])
	}
}

func TestCrossPlatformCoveragePreparePersonalTodoSubscriptions(t *testing.T) {
	identity := personal.Identity{CorpID: "corp", UserID: "self", ClientID: "client", SourceID: "open"}
	eventKeys := []string{
		personal.EventTodoTaskCreated,
		personal.EventTodoTaskUpdated,
		personal.EventTodoTaskDeleted,
	}
	for _, eventKey := range eventKeys {
		prepared, err := preparePersonalSubscription(identity, personalConsumeOptions{EventKey: eventKey})
		if err != nil {
			t.Fatalf("prepare %s error = %v", eventKey, err)
		}
		wantRule := map[string]any{"roleTypes": []string{"creator", "executor", "participant"}}
		if prepared.RuleType != "all" || !reflect.DeepEqual(prepared.Request.RuleParam, wantRule) || prepared.Request.Filter != nil {
			t.Fatalf("prepared %s = %#v, want all/%#v", eventKey, prepared, wantRule)
		}
	}

	prepared, err := preparePersonalSubscription(identity, personalConsumeOptions{
		EventKey:  personal.EventTodoTaskUpdated,
		RoleTypes: []string{"participant", "creator"},
	})
	if err != nil {
		t.Fatalf("prepare custom Todo roles error = %v", err)
	}
	wantRule := map[string]any{"roleTypes": []string{"creator", "participant"}}
	if !reflect.DeepEqual(prepared.Request.RuleParam, wantRule) {
		t.Fatalf("custom Todo rule = %#v, want %#v", prepared.Request.RuleParam, wantRule)
	}

	for name, opts := range map[string]personalConsumeOptions{
		"user":        {EventKey: personal.EventTodoTaskCreated, UserID: "user-1"},
		"group":       {EventKey: personal.EventTodoTaskCreated, GroupID: "cid-1"},
		"query":       {EventKey: personal.EventTodoTaskCreated, QueryCSV: "urgent"},
		"filter-json": {EventKey: personal.EventTodoTaskCreated, FilterJSON: `{"field":"subject","op":"eq","value":"urgent"}`},
		"invalid-role": {
			EventKey: personal.EventTodoTaskCreated, RoleTypes: []string{"owner"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := preparePersonalSubscription(identity, opts); err == nil {
				t.Fatalf("prepare invalid Todo options error = nil")
			}
		})
	}
}

func TestCrossPlatformCoveragePreparePersonalTodoMultiOptions(t *testing.T) {
	plans, err := preparePersonalMultiOptions(personalConsumeOptions{
		EventKeys: []string{
			personal.EventTodoTaskCreated,
			personal.EventTodoTaskUpdated,
			personal.EventTodoTaskDeleted,
		},
		RoleTypes: []string{"executor"},
	})
	if err != nil {
		t.Fatalf("prepare multi Todo options error = %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("Todo plans = %#v, want 3", plans)
	}
	for _, plan := range plans {
		prepared, err := preparePersonalSubscription(personal.Identity{}, plan)
		if err != nil {
			t.Fatalf("prepare %s error = %v", plan.EventKey, err)
		}
		want := map[string]any{"roleTypes": []string{"executor"}}
		if !reflect.DeepEqual(prepared.Request.RuleParam, want) {
			t.Fatalf("prepared %s rule = %#v, want %#v", plan.EventKey, prepared.Request.RuleParam, want)
		}
	}
}

func TestCrossPlatformCoveragePersonalTodoReuseRejectsRoleOverride(t *testing.T) {
	testseam.Swap(t, &personalGetSubscription, func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{
			SubscribeID: "todo-sub",
			EventKey:    personal.EventTodoTaskCreated,
			RuleType:    "all",
		}, nil
	})

	_, _, _, err := ensurePersonalSubscription(context.Background(), nil, personal.Identity{}, personalConsumeOptions{
		SubscribeID: "todo-sub",
		RoleTypes:   []string{"executor"},
	})
	if err == nil || !strings.Contains(err.Error(), "--role-types is not supported when reusing --subscribe-id") {
		t.Fatalf("reused Todo role override error = %v", err)
	}
}
