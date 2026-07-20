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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func personalMessageData(eventKey string) string {
	return fmt.Sprintf(`{
		"eventId":"data-event",
		"eventKey":%q,
		"occurredAtMs":1783483236995,
		"subId":"data-sub",
		"payload":{
			"body":{
				"createTime":"2026-07-08 12:00:35",
				"sender":"测试用户甲",
				"openMessageId":"msg-1",
				"senderOpenDingTalkId":"open-user-1",
				"openConversationId":"cid-1",
				"content":"在吗"
			},
			"event_time":1783483235983
		}
	}`, eventKey)
}

func personalReadData(eventKey, messageID, conversationID string) string {
	return fmt.Sprintf(`{
		"eventId":"read-event",
		"eventKey":%q,
		"occurredAtMs":1784008412182,
		"subId":"read-sub",
		"payload":{
			"bizid":"internal-bizid",
			"body":{
				"msgReadTime":"2026-07-14 13:53:31",
				"openConversationId":%q,
				"openMessageId":%q,
				"reader":"测试用户乙",
				"readerOpenDingTalkId":"reader-open-id",
				"sender":"测试用户甲",
				"senderOpenDingTalkId":"sender-open-id"
			},
			"clientId":"internal-client",
			"corpid":"internal-corp",
			"event_time":1784008411652,
			"filterSubId":"internal-filter",
			"uid":100001
		}
	}`, eventKey, conversationID, messageID)
}

func personalRecallData(eventKey, messageID, conversationID string) string {
	return fmt.Sprintf(`{
		"eventId":"recall-event",
		"eventKey":%q,
		"occurredAtMs":1784008592969,
		"subId":"recall-sub",
		"payload":{
			"bizid":"internal-bizid",
			"body":{
				"msgRecallTime":"2026-07-14 13:56:32",
				"openConversationId":%q,
				"openMessageId":%q,
				"recaller":"测试用户乙",
				"recallerOpenDingTalkId":"recaller-open-id",
				"sender":"测试用户乙",
				"senderOpenDingTalkId":"sender-open-id"
			},
			"clientId":"internal-client",
			"corpid":"internal-corp",
			"event_time":1784008592766,
			"filterSubId":"internal-filter",
			"uid":100001
		}
	}`, eventKey, conversationID, messageID)
}

func personalReactionData(eventKey, messageID, conversationID string) string {
	return fmt.Sprintf(`{
		"eventId":"reaction-event",
		"eventKey":%q,
		"occurredAtMs":1784008680072,
		"subId":"reaction-sub",
		"payload":{
			"bizid":"internal-bizid",
			"body":{
				"emotionName":"微笑",
				"emotionText":"微笑",
				"openConversationId":%q,
				"openSourceMessageId":%q,
				"oper":"测试用户乙",
				"operOpenDingtalkId":"operator-open-id",
				"operateTime":"2026-07-14 13:57:59",
				"operateType":"add",
				"sender":"测试用户甲",
				"senderOpenDingTalkId":"sender-open-id"
			},
			"clientId":"internal-client",
			"corpid":"internal-corp",
			"event_time":1784008679217,
			"filterSubId":"internal-filter",
			"uid":100001
		}
	}`, eventKey, conversationID, messageID)
}

func TestCrossPlatformCoverageProjectOutputMessageEvents(t *testing.T) {
	for _, eventKey := range []string{EventMention, EventSingleChat, EventInChat, EventFromUser, EventAllSingleChat, EventAllGroupChat} {
		t.Run(eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				Type:          transport.FrameTypeEvent,
				EventID:       "outer-event",
				EventBornTime: 11,
				EventType:     eventKey,
				SubscribeID:   "outer-sub",
				Data:          personalMessageData(eventKey),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			got, ok := projected.(MessageEventOutput)
			if !ok {
				t.Fatalf("ProjectOutput() type = %T", projected)
			}
			want := MessageEventOutput{
				Type:                 eventKey,
				EventID:              "data-event",
				Timestamp:            1783483236995,
				SubscribeID:          "outer-sub",
				MessageID:            "msg-1",
				ConversationID:       "cid-1",
				Sender:               "测试用户甲",
				SenderOpenDingTalkID: "open-user-1",
				Content:              "在吗",
				CreateTime:           "2026-07-08 12:00:35",
				EventTime:            1783483235983,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestCrossPlatformCoverageProjectOutputGroupLifecycleEvents(t *testing.T) {
	for _, eventKey := range []string{EventGroupUpdated, EventGroupMemberAdded, EventGroupMemberExited, EventGroupDisbanded} {
		t.Run(eventKey, func(t *testing.T) {
			data := fmt.Sprintf(`{
				"eventId":"group-event",
				"eventKey":%q,
				"occurredAtMs":1784009000000,
				"subId":"data-sub",
				"payload":{
					"uid":100001,
					"CORPID":"internal-corp",
					"clientId":"internal-client",
					"filterSubId":"internal-filter",
					"bizid":"internal-biz",
					"orgId":100002,
					"sourceId":"open",
					"event_time":1784008999000,
					"body":{
						"openConversationId":"cid-group-1",
						"title":"测试群新标题",
						"operator":{"uid":"business-user-1"}
					}
				}
			}`, eventKey)
			projected, err := ProjectOutput(transport.Event{
				EventID:     "outer-event",
				EventType:   eventKey,
				SubscribeID: "outer-sub",
				Data:        data,
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			got, ok := projected.(GroupLifecycleEventOutput)
			if !ok {
				t.Fatalf("ProjectOutput() type = %T", projected)
			}
			if got.Type != eventKey || got.EventID != "group-event" || got.Timestamp != 1784009000000 || got.SubscribeID != "outer-sub" {
				t.Fatalf("common fields = %#v", got)
			}
			for _, internal := range []string{"uid", "CORPID", "clientId", "filterSubId", "bizid", "orgId", "sourceId"} {
				if _, ok := got.Payload[internal]; ok {
					t.Fatalf("payload retained internal field %q: %#v", internal, got.Payload)
				}
			}
			body, ok := got.Payload["body"].(map[string]any)
			if !ok || body["title"] != "测试群新标题" {
				t.Fatalf("payload body = %#v", got.Payload["body"])
			}
			operator := body["operator"].(map[string]any)
			if operator["uid"] != "business-user-1" {
				t.Fatalf("nested business uid was removed: %#v", operator)
			}
		})
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsInvalidGroupLifecyclePayloads(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "missing", data: `{"eventKey":"user_im_group_updated"}`},
		{name: "null", data: `{"eventKey":"user_im_group_updated","payload":null}`},
		{name: "empty object", data: `{"eventKey":"user_im_group_updated","payload":{}}`},
		{name: "array", data: `{"eventKey":"user_im_group_updated","payload":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := transport.Event{EventID: "outer-event", EventType: EventGroupUpdated, Data: tt.data}
			projected, err := ProjectOutput(ev)
			if err == nil {
				t.Fatal("ProjectOutput() error = nil, want payload validation error")
			}
			if got, ok := projected.(transport.Event); !ok || !reflect.DeepEqual(got, ev) {
				t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, ev)
			}
		})
	}
}

func TestCrossPlatformCoverageProjectOutputDecodesWrappedJSONString(t *testing.T) {
	wrapped, err := json.Marshal(personalMessageData(EventSingleChat))
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectOutput(transport.Event{Data: string(wrapped)})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got := projected.(MessageEventOutput)
	if got.Content != "在吗" || got.Type != EventSingleChat || got.SubscribeID != "data-sub" || got.EventID != "data-event" {
		t.Fatalf("ProjectOutput() = %#v", got)
	}
}

func TestCrossPlatformCoverageProjectOutputFallsBackToTransportFields(t *testing.T) {
	projected, err := ProjectOutput(transport.Event{
		EventID:       "outer-event",
		EventBornTime: 123,
		EventType:     EventSingleChat,
		SubscribeID:   "outer-sub",
		Data:          `{"payload":{"body":{"content":"hello"}}}`,
	})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got := projected.(MessageEventOutput)
	if got.EventID != "outer-event" || got.Timestamp != 123 || got.SubscribeID != "outer-sub" || got.Type != EventSingleChat {
		t.Fatalf("fallback fields = %#v", got)
	}
}

func TestCrossPlatformCoverageProjectOutputReadEvents(t *testing.T) {
	for _, eventKey := range []string{EventReadO2O, EventReadGroup} {
		t.Run(eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				EventType: eventKey,
				Data:      personalReadData(eventKey, "read-message", "read-conversation"),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			want := ReadEventOutput{
				Type:                 eventKey,
				EventID:              "read-event",
				Timestamp:            1784008412182,
				SubscribeID:          "read-sub",
				MessageID:            "read-message",
				ConversationID:       "read-conversation",
				Reader:               "测试用户乙",
				ReaderOpenDingTalkID: "reader-open-id",
				Sender:               "测试用户甲",
				SenderOpenDingTalkID: "sender-open-id",
				ReadTime:             "2026-07-14 13:53:31",
				EventTime:            1784008411652,
			}
			if !reflect.DeepEqual(projected, want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", projected, want)
			}
			assertNoInternalActionFields(t, projected)
		})
	}
}

func TestCrossPlatformCoverageProjectOutputRecallEvents(t *testing.T) {
	for _, eventKey := range []string{EventRecallO2O, EventRecallGroup} {
		t.Run(eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				EventType: eventKey,
				Data:      personalRecallData(eventKey, "recall-message", "recall-conversation"),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			want := RecallEventOutput{
				Type:                   eventKey,
				EventID:                "recall-event",
				Timestamp:              1784008592969,
				SubscribeID:            "recall-sub",
				MessageID:              "recall-message",
				ConversationID:         "recall-conversation",
				Recaller:               "测试用户乙",
				RecallerOpenDingTalkID: "recaller-open-id",
				Sender:                 "测试用户乙",
				SenderOpenDingTalkID:   "sender-open-id",
				RecallTime:             "2026-07-14 13:56:32",
				EventTime:              1784008592766,
			}
			if !reflect.DeepEqual(projected, want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", projected, want)
			}
			assertNoInternalActionFields(t, projected)
		})
	}
}

func TestCrossPlatformCoverageProjectOutputReactionEvents(t *testing.T) {
	for _, eventKey := range []string{EventReactionO2O, EventReactionGroup} {
		t.Run(eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				EventType: eventKey,
				Data:      personalReactionData(eventKey, "reaction-message", "reaction-conversation"),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			want := ReactionEventOutput{
				Type:                   eventKey,
				EventID:                "reaction-event",
				Timestamp:              1784008680072,
				SubscribeID:            "reaction-sub",
				MessageID:              "reaction-message",
				ConversationID:         "reaction-conversation",
				Operator:               "测试用户乙",
				OperatorOpenDingTalkID: "operator-open-id",
				ReactionName:           "微笑",
				ReactionText:           "微笑",
				OperationType:          "add",
				OperationTime:          "2026-07-14 13:57:59",
				Sender:                 "测试用户甲",
				SenderOpenDingTalkID:   "sender-open-id",
				EventTime:              1784008679217,
			}
			if !reflect.DeepEqual(projected, want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", projected, want)
			}
			assertNoInternalActionFields(t, projected)
		})
	}
}

func TestCrossPlatformCoverageProjectOutputReactionRejectsLegacyOperatorOpenIDSpellings(t *testing.T) {
	for _, legacyField := range []string{"operOpenDingtlkId", "operOpenDingTalkId"} {
		t.Run(legacyField, func(t *testing.T) {
			data := strings.Replace(
				personalReactionData(EventReactionO2O, "reaction-message", "reaction-conversation"),
				"operOpenDingtalkId",
				legacyField,
				1,
			)
			projected, err := ProjectOutput(transport.Event{EventType: EventReactionO2O, Data: data})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			got := projected.(ReactionEventOutput)
			if got.OperatorOpenDingTalkID != "" {
				t.Fatalf("operator_open_dingtalk_id = %q, want empty for legacy field %s", got.OperatorOpenDingTalkID, legacyField)
			}
		})
	}
}

func assertNoInternalActionFields(t *testing.T, projected any) {
	t.Helper()
	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"payload", "uid", "corpid", "clientId", "filterSubId", "bizid"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("projected output leaked internal field %q: %s", field, raw)
		}
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsEmptyPayloads(t *testing.T) {
	eventKeys := []string{
		EventMention,
		EventSingleChat,
		EventInChat,
		EventFromUser,
		EventAllSingleChat,
		EventAllGroupChat,
		EventReadO2O,
		EventReadGroup,
		EventRecallO2O,
		EventRecallGroup,
		EventReactionO2O,
		EventReactionGroup,
	}
	payloads := []struct {
		name string
		json string
	}{
		{name: "missing", json: ""},
		{name: "null", json: `,"payload":null`},
		{name: "empty object", json: `,"payload":{}`},
		{name: "missing body", json: `,"payload":{"event_time":1}`},
		{name: "null body", json: `,"payload":{"body":null}`},
		{name: "empty body", json: `,"payload":{"body":{}}`},
	}

	for _, eventKey := range eventKeys {
		for _, payload := range payloads {
			t.Run(eventKey+"/"+payload.name, func(t *testing.T) {
				ev := transport.Event{
					EventID:   "outer-event",
					EventType: eventKey,
					Data:      fmt.Sprintf(`{"eventKey":%q%s}`, eventKey, payload.json),
				}
				projected, err := ProjectOutput(ev)
				if err == nil {
					t.Fatal("ProjectOutput() error = nil, want payload validation error")
				}
				got, ok := projected.(transport.Event)
				if !ok || !reflect.DeepEqual(got, ev) {
					t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, ev)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageProjectOutputMalformedDataReturnsRawEnvelope(t *testing.T) {
	ev := transport.Event{EventID: "outer-event", Data: "not-json"}
	projected, err := ProjectOutput(ev)
	if err == nil {
		t.Fatal("ProjectOutput() error = nil, want decode error")
	}
	got, ok := projected.(transport.Event)
	if !ok || !reflect.DeepEqual(got, ev) {
		t.Fatalf("ProjectOutput() fallback = %#v", projected)
	}
}
