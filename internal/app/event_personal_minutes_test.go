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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoveragePersonalMinutesCommands(t *testing.T) {
	list := newEventListCommand()
	var out bytes.Buffer
	list.SetOut(&out)
	list.SetArgs([]string{"--category", "minutes"})
	if err := corecmd.ExecuteForTest(list); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{personal.EventMinutesTaskCreated, personal.EventMinutesSummaryGenerated} {
		if !strings.Contains(out.String(), key) {
			t.Fatalf("missing event: %s", key)
		}
		for _, flatten := range []bool{false, true} {
			schema := newEventSchemaCommand()
			var schemaOut bytes.Buffer
			schema.SetOut(&schemaOut)
			args := []string{key}
			if flatten {
				args = append(args, "--flatten")
			}
			schema.SetArgs(args)
			if err := corecmd.ExecuteForTest(schema); err != nil {
				t.Fatal(err)
			}
			if flatten && strings.Contains(schemaOut.String(), `"creator_uid"`) {
				t.Fatalf("legacy creator_uid in schema: %s", schemaOut.String())
			}
			if flatten && (!strings.Contains(schemaOut.String(), "task_uuid") || !strings.Contains(schemaOut.String(), "creator_open_dingtalk_id")) {
				t.Fatalf("schema: %s", schemaOut.String())
			}
		}
		if err := validatePersonalBusinessEventOptions(key, personalConsumeOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, opts := range []personalConsumeOptions{{UserID: "u"}, {OpenDingTalkID: "u"}, {GroupID: "g"}, {QueryCSV: "q"}, {FilterJSON: `{}`}, {RoleTypes: []string{"creator"}}} {
			if err := validatePersonalBusinessEventOptions(key, opts); err == nil {
				t.Fatalf("accepted unsupported options: %#v", opts)
			}
		}
	}
	if strings.Contains(out.String(), personal.EventMention) {
		t.Fatal("minutes category includes IM event")
	}
}

func TestCrossPlatformCoveragePersonalMinutesCreatorIDTypeFallback(t *testing.T) {
	for _, key := range []string{personal.EventMinutesTaskCreated, personal.EventMinutesSummaryGenerated} {
		data := `{"eventId":"e1","eventKey":"` + key + `","occurredAtMs":1234,"subId":"s1","payload":{"bizid":"b1","event_time":1200,"corpid":"c","orgId":1,"uid":2,"body":{"schemaVersion":1,"taskUuid":"T1","creatorOpenDingtalkId":true}}}`
		result, err := personal.ProjectOutput(transport.Event{Data: data})
		if err == nil {
			t.Fatalf("%s: accepted boolean creatorOpenDingtalkId: %#v", key, result)
		}
		if !strings.Contains(err.Error(), "creatorOpenDingtalkId") {
			t.Fatalf("%s: wrong error: %v", key, err)
		}
	}
}
