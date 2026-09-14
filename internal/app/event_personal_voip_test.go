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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

func TestCrossPlatformCoveragePersonalVoIPEventListSchemaAndValidation(t *testing.T) {
	list := newEventListCommand()
	list.SilenceUsage = true
	list.SilenceErrors = true
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"--category", "voip"})
	if err := list.Execute(); err != nil {
		t.Fatalf("event list --category voip error = %v", err)
	}
	if !strings.Contains(listOut.String(), personal.EventVoIPCallReceiveInvite) {
		t.Fatalf("VoIP event list missing %s:\n%s", personal.EventVoIPCallReceiveInvite, listOut.String())
	}
	if strings.Contains(listOut.String(), personal.EventMention) || strings.Contains(listOut.String(), personal.EventOAApprovalTaskCreated) {
		t.Fatalf("VoIP category list leaked another category:\n%s", listOut.String())
	}

	schema := newEventSchemaCommand()
	schema.SilenceUsage = true
	schema.SilenceErrors = true
	var schemaOut bytes.Buffer
	schema.SetOut(&schemaOut)
	schema.SetArgs([]string{personal.EventVoIPCallReceiveInvite, "--flatten"})
	if err := schema.Execute(); err != nil {
		t.Fatalf("event schema %s --flatten error = %v", personal.EventVoIPCallReceiveInvite, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(schemaOut.Bytes(), &doc); err != nil {
		t.Fatalf("decode VoIP schema: %v\n%s", err, schemaOut.String())
	}
	if doc["event_key"] != personal.EventVoIPCallReceiveInvite || doc["category"] != "voip" || doc["rule_type"] != "all" || doc["jq_root_path"] != "." {
		t.Fatalf("VoIP schema document = %#v", doc)
	}
	schemaBody, ok := doc["schema"].(map[string]any)
	if !ok {
		t.Fatalf("VoIP schema body = %#v", doc["schema"])
	}
	properties, ok := schemaBody["properties"].(map[string]any)
	if !ok {
		t.Fatalf("VoIP schema properties = %#v", schemaBody["properties"])
	}
	for _, name := range []string{"biz_id", "call_id", "caller_uid", "callee_uid", "room_id", "event_time"} {
		if _, ok := properties[name].(map[string]any); !ok {
			t.Fatalf("VoIP schema property %s = %#v", name, properties[name])
		}
	}
	if _, ok := properties["room_code"]; ok {
		t.Fatalf("VoIP schema unexpectedly exposes sensitive room_code: %#v", properties["room_code"])
	}
	for _, name := range []string{"caller_uid", "callee_uid"} {
		property := properties[name].(map[string]any)
		if property["type"] != "string" {
			t.Fatalf("VoIP schema property %s type = %#v, want string", name, property["type"])
		}
	}

	if err := validatePersonalBusinessEventOptions(personal.EventVoIPCallReceiveInvite, personalConsumeOptions{}); err != nil {
		t.Fatalf("VoIP event without target/filter options error = %v", err)
	}
	for name, opts := range map[string]personalConsumeOptions{
		"--user":             {UserID: "user-1"},
		"--open-dingtalk-id": {OpenDingTalkID: "open-user-1"},
		"--group":            {GroupID: "cid-1"},
		"--query":            {QueryCSV: "urgent"},
		"--filter-json":      {FilterJSON: `{"field":"content","op":"eq","value":"urgent"}`},
	} {
		err := validatePersonalBusinessEventOptions(personal.EventVoIPCallReceiveInvite, opts)
		if err == nil || !strings.Contains(err.Error(), name+" not supported for VoIP event "+personal.EventVoIPCallReceiveInvite) {
			t.Fatalf("VoIP %s validation error = %v", name, err)
		}
	}
}
