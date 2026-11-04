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

package personal

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoverageMinutesEvents(t *testing.T) {
	for _, key := range []string{EventMinutesTaskCreated, EventMinutesSummaryGenerated} {
		t.Run(key, func(t *testing.T) {
			def, ok := Lookup(key)
			if !ok || def.Category != "minutes" || !def.Public || def.Status != StatusEnabled || def.Auth["identity"] != "user" {
				t.Fatalf("definition: %#v", def)
			}
			rule, param, err := BuildRuleParam(key, RuleOptions{})
			if err != nil || rule != "all" || param == nil || len(param) != 0 {
				t.Fatalf("rule: %s %#v %v", rule, param, err)
			}
			req := (&Client{}).buildCreateRequest(CreateSubscriptionRequest{EventKey: key, RuleType: rule, RuleParam: param})
			if req.FilterRule != "{}" || req.EventKey != key {
				t.Fatalf("request: %#v", req)
			}
			data := `{"eventId":"platform-event","eventKey":"` + key + `","occurredAtMs":1234,"subId":"sub-1","payload":{"bizid":"business-event","event_time":1200,"corpid":"ding_example","orgId":21001,"uid":20002,"body":{"schemaVersion":1,"taskUuid":"TASK_UUID_EXAMPLE","creatorOpenDingtalkId":9007199254740993}}}`
			result, err := ProjectOutput(transport.Event{Data: data})
			if err != nil {
				t.Fatal(err)
			}
			got := result.(MinutesEventOutput)
			if got.Type != key || got.EventID != "platform-event" || got.BizID != "business-event" || got.Timestamp != 1234 || got.EventTime != 1200 || got.SubscribeID != "sub-1" || got.TaskUUID != "TASK_UUID_EXAMPLE" || got.CreatorOpenDingtalkID != "9007199254740993" || got.SchemaVersion != 1 {
				t.Fatalf("output: %#v", got)
			}
			strData := strings.Replace(data, `"creatorOpenDingtalkId":9007199254740993`, `"creatorOpenDingtalkId":"open_id_abc123"`, 1)
			strResult, err := ProjectOutput(transport.Event{Data: strData})
			if err != nil {
				t.Fatalf("string creator ID: %v", err)
			}
			if strResult.(MinutesEventOutput).CreatorOpenDingtalkID != "open_id_abc123" {
				t.Fatalf("string creator ID output: %#v", strResult)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			props := outputSchema(key)["properties"].(map[string]any)
			if len(props) != len(fields) {
				t.Fatalf("schema/output mismatch: %#v %#v", props, fields)
			}
			for name := range fields {
				if _, ok := props[name]; !ok {
					t.Fatalf("missing schema: %s", name)
				}
			}
			for _, legacy := range []string{"creator_uid", "creatorUid"} {
				if _, exists := props[legacy]; exists {
					t.Fatalf("legacy field in schema: %s", legacy)
				}
				if _, exists := fields[legacy]; exists {
					t.Fatalf("legacy field in output: %s", legacy)
				}
			}
			if props["creator_open_dingtalk_id"].(map[string]any)["type"] != "string" {
				t.Fatal("creator open ID schema must be string")
			}
			for _, tc := range []struct{ old, new, want string }{
				{`"schemaVersion":1`, `"schemaVersion":2`, "schemaVersion"},
				{`"taskUuid":"TASK_UUID_EXAMPLE"`, `"taskUuid":" "`, "taskUuid"},
				{`"creatorOpenDingtalkId":9007199254740993`, `"creatorOpenDingtalkId":0`, "creatorOpenDingtalkId"},
				{`"creatorOpenDingtalkId":9007199254740993`, `"creatorUid":9007199254740993`, "creatorOpenDingtalkId"},
				{`"creatorOpenDingtalkId":9007199254740993`, `"creatorOpenDingtalkId":""`, "creatorOpenDingtalkId"},
				{`"creatorOpenDingtalkId":9007199254740993`, `"creatorOpenDingtalkId":null`, "creatorOpenDingtalkId"},
				{`"creatorOpenDingtalkId":9007199254740993`, `"creatorOpenDingtalkId":true`, "creatorOpenDingtalkId"},
			} {
				_, err := ProjectOutput(transport.Event{Data: strings.Replace(data, tc.old, tc.new, 1)})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("invalid %s: %v", tc.want, err)
				}
			}
			for _, raw := range []string{`{`, `{"eventKey":"` + key + `","payload":null}`, `{"eventKey":"` + key + `","payload":{}}`} {
				if _, err := ProjectOutput(transport.Event{EventType: key, Data: raw}); err == nil {
					t.Fatalf("accepted invalid: %s", raw)
				}
			}
		})
	}
}
