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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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

func personalGroupMemberData(eventKey string) string {
	return fmt.Sprintf(`{
		"eventId":"group-member-event",
		"eventKey":%q,
		"occurredAtMs":1784782513647,
		"subId":"group-member-sub",
		"payload":{
			"uid":100001,
			"clientId":"internal-client",
			"corpid":"internal-corp",
			"bizid":"internal-biz",
			"filterSubId":"internal-filter",
			"body":{
				"operNick":"测试用户甲",
				"members":[
					{"nick":"测试用户乙","openDingTalkId":"member-open-id-1"},
					{"nick":"测试用户丙","openDingTalkId":"member-open-id-2"}
				],
				"operOpenDingtalkId":"operator-open-id",
				"openConversationId":"cid-group-1"
			},
			"event_time":1784782513502
		}
	}`, eventKey)
}

func personalOAData(eventKey string) string {
	body := map[string]any{
		"processInstanceId": "process-instance-1",
		"createTime":        int64(1785229100000),
		"processCode":       "PROC-TEST-1",
		"title":             "测试审批",
	}
	switch eventKey {
	case EventOAApprovalTaskCreated:
		body["taskId"] = "approval-task-1"
		body["status"] = "RUNNING"
	case EventOAApprovalTaskFinished:
		body["taskId"] = "approval-task-1"
		body["status"] = "FINISHED"
		body["result"] = "agree"
		body["finishTime"] = int64(1785229199000)
	case EventOAApprovalTaskRedirected:
		body["taskId"] = "approval-task-1"
		body["status"] = "FINISHED"
		body["result"] = "redirect"
		body["finishTime"] = int64(1785229199000)
	case EventOAApprovalInstanceStarted:
		body["status"] = "RUNNING"
	case EventOAApprovalInstanceCC:
		body["status"] = "RUNNING"
	case EventOAApprovalInstanceTerminated:
		body["status"] = "TERMINATED"
		body["finishTime"] = int64(1785229199000)
	case EventOAApprovalInstanceFinished:
		body["status"] = "FINISHED"
		body["result"] = "agree"
		body["finishTime"] = int64(1785229199000)
	}
	data := map[string]any{
		"eventId":      "oa-event",
		"eventKey":     eventKey,
		"occurredAtMs": int64(1785229200123),
		"subId":        "oa-data-sub",
		"payload": map[string]any{
			"uid":         100001,
			"CORPID":      "internal-corp",
			"clientId":    "internal-client",
			"filterSubId": "internal-filter",
			"bizid":       "internal-biz",
			"orgId":       100002,
			"sourceId":    "open",
			"body":        body,
			"event_time":  int64(1785229199000),
			"futureField": map[string]any{"nested": true},
		},
	}
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

func personalVoIPData() string {
	return `{
		"eventId":"voip-event",
		"eventKey":"user_voip_call_receive_invite",
		"occurredAtMs":1780630479124,
		"subId":"voip-data-sub",
		"payload":{
			"bizid":"VOIP_room-1_3559506650",
			"event_time":1780630479123,
			"corpid":"ding-callee-corp",
			"orgId":21001,
			"uid":3559506650,
			"filterSubId":"internal-filter",
			"body":{
				"callId":"call-1",
				"callerUid":"0147333457361236773",
				"callerCorpId":"ding-caller-corp",
				"calleeUid":"digital-3559506650",
				"calleeCorpId":"ding-callee-corp",
				"callType":"conference",
				"roomId":"room-1",
				"roomCode":"sensitive-code",
				"createTime":1780630479000
			}
		}
	}`
}

func TestCrossPlatformCoverageProjectTransportOutput(t *testing.T) {
	voip := transport.Event{
		Type:          transport.FrameTypeEvent,
		EventID:       "outer-event",
		EventBornTime: 1780630479000,
		EventType:     EventVoIPCallReceiveInvite,
		SubscribeID:   "outer-sub",
		Data: `{
			"eventId":"inner-event",
			"eventKey":"user_voip_call_receive_invite",
			"occurredAtMs":1780630479124,
			"subId":"inner-sub",
			"payload":{
				"room-code":"root-secret",
				"body":{
					"callId":"call-1",
					"roomCode":"body-secret",
					"items":[{"room_code":"nested-secret","keep":true},"text",7]
				}
			}
		}`,
	}

	projected, err := ProjectTransportOutput(voip)
	if err != nil {
		t.Fatalf("ProjectTransportOutput(VoIP) error = %v", err)
	}
	safe, ok := projected.(transport.Event)
	if !ok {
		t.Fatalf("ProjectTransportOutput(VoIP) type = %T, want transport.Event", projected)
	}
	if safe.Type != voip.Type || safe.EventID != voip.EventID || safe.EventBornTime != voip.EventBornTime || safe.SubscribeID != voip.SubscribeID {
		t.Fatalf("ProjectTransportOutput(VoIP) envelope = %#v, want transport identity from %#v", safe, voip)
	}
	if strings.Contains(strings.ToLower(safe.Data), "roomcode") || strings.Contains(safe.Data, "room_code") || strings.Contains(safe.Data, "room-code") || strings.Contains(safe.Data, "secret") {
		t.Fatalf("ProjectTransportOutput(VoIP) leaked a room code: %s", safe.Data)
	}
	if !strings.Contains(safe.Data, `"callId":"call-1"`) || !strings.Contains(safe.Data, `"keep":true`) {
		t.Fatalf("ProjectTransportOutput(VoIP) dropped non-sensitive payload: %s", safe.Data)
	}

	dataKeyOnly := voip
	dataKeyOnly.EventType = ""
	if _, err := ProjectTransportOutput(dataKeyOnly); err != nil {
		t.Fatalf("ProjectTransportOutput(data event key VoIP) error = %v", err)
	}

	nonVoIP := transport.Event{EventType: EventSingleChat, Data: personalMessageData(EventSingleChat)}
	gotNonVoIP, err := ProjectTransportOutput(nonVoIP)
	if err != nil || !reflect.DeepEqual(gotNonVoIP, nonVoIP) {
		t.Fatalf("ProjectTransportOutput(non-VoIP) = (%#v, %v), want unchanged event", gotNonVoIP, err)
	}
	malformedNonVoIP := transport.Event{EventType: EventSingleChat, Data: "not-json"}
	gotMalformedNonVoIP, err := ProjectTransportOutput(malformedNonVoIP)
	if err != nil || !reflect.DeepEqual(gotMalformedNonVoIP, malformedNonVoIP) {
		t.Fatalf("ProjectTransportOutput(malformed non-VoIP) = (%#v, %v), want unchanged event", gotMalformedNonVoIP, err)
	}

	malformedVoIP := transport.Event{
		EventType:     EventVoIPCallReceiveInvite,
		EventID:       "outer-event",
		EventBornTime: 17,
		SubscribeID:   "outer-sub",
		Data:          "not-json",
	}
	gotMalformedVoIP, err := ProjectTransportOutput(malformedVoIP)
	if err == nil || !strings.Contains(err.Error(), "safe transport output") {
		t.Fatalf("ProjectTransportOutput(malformed VoIP) error = %v", err)
	}
	if want := (baseEventOutput{Type: EventVoIPCallReceiveInvite, EventID: "outer-event", Timestamp: 17, SubscribeID: "outer-sub"}); !reflect.DeepEqual(gotMalformedVoIP, want) {
		t.Fatalf("ProjectTransportOutput(malformed VoIP) = %#v, want %#v", gotMalformedVoIP, want)
	}

	missingPayload := transport.Event{
		EventID:       "outer-event",
		EventBornTime: 17,
		SubscribeID:   "outer-sub",
		Data:          `{"eventId":"inner-event","eventKey":"user_voip_call_receive_invite","subId":"inner-sub"}`,
	}
	gotMissingPayload, err := ProjectTransportOutput(missingPayload)
	if err == nil || !strings.Contains(err.Error(), "redact personal VoIP payload") {
		t.Fatalf("ProjectTransportOutput(VoIP missing payload) error = %v", err)
	}
	if want := (baseEventOutput{Type: EventVoIPCallReceiveInvite, EventID: "inner-event", Timestamp: 17, SubscribeID: "outer-sub"}); !reflect.DeepEqual(gotMissingPayload, want) {
		t.Fatalf("ProjectTransportOutput(VoIP missing payload) = %#v, want %#v", gotMissingPayload, want)
	}

	if got := firstNonZeroOutput(0, 42, 7); got != 42 {
		t.Fatalf("firstNonZeroOutput(0, 42, 7) = %d, want 42", got)
	}
	if got := firstNonZeroOutput(0, 0); got != 0 {
		t.Fatalf("firstNonZeroOutput(0, 0) = %d, want 0", got)
	}
}

func TestCrossPlatformCoverageProjectTransportOutputMarshalFailure(t *testing.T) {
	testseam.Swap(t, &marshalPersonalTransportData, func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	})

	ev := transport.Event{
		EventBornTime: 19,
		Data:          `{"eventKey":"user_voip_call_receive_invite","payload":{"body":{"callId":"call-1"}}}`,
	}
	projected, err := ProjectTransportOutput(ev)
	if err == nil || !strings.Contains(err.Error(), "encode redacted personal VoIP transport data") {
		t.Fatalf("ProjectTransportOutput(marshal failure) error = %v", err)
	}
	if want := (baseEventOutput{Type: EventVoIPCallReceiveInvite, Timestamp: 19}); !reflect.DeepEqual(projected, want) {
		t.Fatalf("ProjectTransportOutput(marshal failure) = %#v, want %#v", projected, want)
	}
}

func TestCrossPlatformCoverageRedactVoIPRoomCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "null", input: "null", wantErr: true},
		{name: "invalid object", input: `{"field":`, wantErr: true},
		{name: "object containing null", input: `{"field":null}`, wantErr: true},
		{name: "invalid array", input: `[`, wantErr: true},
		{name: "array containing null", input: `[null]`, wantErr: true},
		{name: "invalid primitive", input: `tru`, wantErr: true},
		{name: "primitive", input: `"text"`},
		{name: "object", input: `{"roomCode":"secret","keep":{"room_code":"nested","value":true}}`},
		{name: "array", input: `[{"room-code":"secret","value":1},"keep"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redactVoIPRoomCode(json.RawMessage(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("redactVoIPRoomCode(%q) error = nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("redactVoIPRoomCode(%q) error = %v", tt.input, err)
			}
			if !json.Valid(got) {
				t.Fatalf("redactVoIPRoomCode(%q) returned invalid JSON: %s", tt.input, got)
			}
			lower := strings.ToLower(string(got))
			if strings.Contains(lower, "roomcode") || strings.Contains(lower, "room_code") || strings.Contains(lower, "room-code") || strings.Contains(lower, "secret") || strings.Contains(lower, "nested") {
				t.Fatalf("redactVoIPRoomCode(%q) leaked a room code: %s", tt.input, got)
			}
		})
	}
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
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, absent := range []string{"quoted_message", "forward_messages"} {
				if strings.Contains(string(encoded), `"`+absent+`"`) {
					t.Fatalf("ordinary message output contains optional field %q: %s", absent, encoded)
				}
			}
		})
	}
}

func TestProjectOutputPreservesQuotedMessageContext(t *testing.T) {
	data := `{
		"eventId":"quoted-event",
		"eventKey":"user_im_message_receive_group",
		"occurredAtMs":1784792292580,
		"subId":"quoted-sub",
		"payload":{
			"body":{
				"createTime":"2026-07-23 15:38:11",
				"sender":"郑御白",
				"openMessageId":"outer-message",
				"senderOpenDingTalkId":"outer-sender-open-id",
				"openConversationId":"target-conversation",
				"content":"引用回复",
				"quotedMessage":{
					"createTime":"2026-07-23 15:35:03",
					"sender":"null",
					"openMessageId":"quoted-message",
					"senderOpenDingTalkId":"quoted-sender-open-id",
					"openConversationId":"source-conversation",
					"content":"被引用的原消息"
				}
			},
			"event_time":1784792291637
		}
	}`

	projected, err := ProjectOutput(transport.Event{
		EventType:   EventInChat,
		SubscribeID: "outer-sub",
		Data:        data,
	})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got := projected.(MessageEventOutput)
	want := &MessageEventContext{
		MessageID:            "quoted-message",
		ConversationID:       "source-conversation",
		Sender:               "null",
		SenderOpenDingTalkID: "quoted-sender-open-id",
		Content:              "被引用的原消息",
		CreateTime:           "2026-07-23 15:35:03",
	}
	if !reflect.DeepEqual(got.QuotedMessage, want) {
		t.Fatalf("quoted_message = %#v, want %#v", got.QuotedMessage, want)
	}
	if got.ForwardMessages != nil {
		t.Fatalf("forward_messages = %#v, want nil", got.ForwardMessages)
	}
}

func TestProjectOutputPreservesMergedForwardContextAndMediaLocator(t *testing.T) {
	data := `{
		"eventId":"forward-event",
		"eventKey":"user_im_message_receive_group",
		"occurredAtMs":1784861030151,
		"subId":"forward-sub",
		"payload":{
			"body":{
				"createTime":"2026-07-24 10:43:49",
				"sender":"郑御白",
				"openMessageId":"outer-forward-message",
				"senderOpenDingTalkId":"outer-sender-open-id",
				"openConversationId":"target-conversation",
				"content":"Chat history between two users\nUser A:[Image]\nUser A:Forwarded chat record",
				"forwardMessages":[
					{
						"createTime":"2026-07-24 10:33:31",
						"sender":"null",
						"openMessageId":"image-message",
						"senderOpenDingTalkId":"image-sender-open-id",
						"openConversationId":"source-conversation",
						"content":"[图片消息](mediaId=media-1) 注意：如需下载使用dws chat message download-media命令下载"
					},
					{
						"createTime":"2026-07-24 10:34:46",
						"sender":"null",
						"openMessageId":"text-message",
						"openConversationId":"source-conversation",
						"content":"转发聊天记录"
					}
				]
			},
			"event_time":1784861029265
		}
	}`

	projected, err := ProjectOutput(transport.Event{
		EventType:   EventInChat,
		SubscribeID: "outer-sub",
		Data:        data,
	})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got := projected.(MessageEventOutput)
	want := []MessageEventContext{
		{
			MessageID:            "image-message",
			ConversationID:       "source-conversation",
			Sender:               "null",
			SenderOpenDingTalkID: "image-sender-open-id",
			Content:              "[图片消息](mediaId=media-1) 注意：如需下载使用dws chat message download-media命令下载",
			CreateTime:           "2026-07-24 10:33:31",
		},
		{
			MessageID:      "text-message",
			ConversationID: "source-conversation",
			Sender:         "null",
			Content:        "转发聊天记录",
			CreateTime:     "2026-07-24 10:34:46",
		},
	}
	if !reflect.DeepEqual(got.ForwardMessages, want) {
		t.Fatalf("forward_messages = %#v, want %#v", got.ForwardMessages, want)
	}
	if got.QuotedMessage != nil {
		t.Fatalf("quoted_message = %#v, want nil", got.QuotedMessage)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, wantFragment := range []string{
		`"forward_messages"`,
		`"message_id":"image-message"`,
		`"conversation_id":"source-conversation"`,
		`mediaId=media-1`,
	} {
		if !strings.Contains(string(encoded), wantFragment) {
			t.Fatalf("flattened merged-forward output missing %q: %s", wantFragment, encoded)
		}
	}
	for _, localizedDetection := range []string{"群聊的聊天记录", "的聊天记录"} {
		if strings.Contains(got.Content, localizedDetection) {
			t.Fatalf("test fixture should not require localized title %q for projection", localizedDetection)
		}
	}
}

func TestCrossPlatformCoverageProjectOutputGroupLifecycleEvents(t *testing.T) {
	for _, eventKey := range []string{EventGroupUpdated, EventGroupDisbanded} {
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

func TestCrossPlatformCoverageProjectOutputOAEvents(t *testing.T) {
	tests := []struct {
		eventKey string
		want     any
	}{
		{
			eventKey: EventOAApprovalTaskCreated,
			want: OAApprovalTaskCreatedOutput{
				Type:              EventOAApprovalTaskCreated,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				TaskID:            "approval-task-1",
				Title:             "测试审批",
				Status:            "RUNNING",
				CreateTime:        1785229100000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalTaskFinished,
			want: OAApprovalTaskFinishedOutput{
				Type:              EventOAApprovalTaskFinished,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				TaskID:            "approval-task-1",
				Title:             "测试审批",
				Status:            "FINISHED",
				Result:            "agree",
				CreateTime:        1785229100000,
				FinishTime:        1785229199000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalTaskRedirected,
			want: OAApprovalTaskRedirectedOutput{
				Type:              EventOAApprovalTaskRedirected,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				TaskID:            "approval-task-1",
				Title:             "测试审批",
				Status:            "FINISHED",
				Result:            "redirect",
				CreateTime:        1785229100000,
				FinishTime:        1785229199000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalInstanceStarted,
			want: OAApprovalInstanceStartedOutput{
				Type:              EventOAApprovalInstanceStarted,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				Title:             "测试审批",
				Status:            "RUNNING",
				CreateTime:        1785229100000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalInstanceCC,
			want: OAApprovalInstanceCCOutput{
				Type:              EventOAApprovalInstanceCC,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				Title:             "测试审批",
				Status:            "RUNNING",
				CreateTime:        1785229100000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalInstanceTerminated,
			want: OAApprovalInstanceTerminatedOutput{
				Type:              EventOAApprovalInstanceTerminated,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				Title:             "测试审批",
				Status:            "TERMINATED",
				CreateTime:        1785229100000,
				FinishTime:        1785229199000,
				EventTime:         1785229199000,
			},
		},
		{
			eventKey: EventOAApprovalInstanceFinished,
			want: OAApprovalInstanceFinishedOutput{
				Type:              EventOAApprovalInstanceFinished,
				EventID:           "oa-event",
				Timestamp:         1785229200123,
				SubscribeID:       "outer-sub",
				ProcessInstanceID: "process-instance-1",
				ProcessCode:       "PROC-TEST-1",
				Title:             "测试审批",
				Status:            "FINISHED",
				Result:            "agree",
				CreateTime:        1785229100000,
				FinishTime:        1785229199000,
				EventTime:         1785229199000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				EventID:       "outer-event",
				EventBornTime: 11,
				EventType:     tt.eventKey,
				SubscribeID:   "outer-sub",
				Data:          personalOAData(tt.eventKey),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			if !reflect.DeepEqual(projected, tt.want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", projected, tt.want)
			}
			assertNoInternalActionFields(t, projected)
		})
	}
}

func TestCrossPlatformCoverageProjectOutputVoIPCallReceiveInvite(t *testing.T) {
	projected, err := ProjectOutput(transport.Event{
		EventID:       "outer-event",
		EventBornTime: 11,
		EventType:     EventVoIPCallReceiveInvite,
		SubscribeID:   "outer-sub",
		Data:          personalVoIPData(),
	})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	want := VoIPCallReceiveInviteOutput{
		Type:         EventVoIPCallReceiveInvite,
		EventID:      "voip-event",
		Timestamp:    1780630479124,
		SubscribeID:  "outer-sub",
		BizID:        "VOIP_room-1_3559506650",
		CorpID:       "ding-callee-corp",
		OrgID:        21001,
		TargetUID:    3559506650,
		CallID:       "call-1",
		CallerUID:    "0147333457361236773",
		CallerCorpID: "ding-caller-corp",
		CalleeUID:    "digital-3559506650",
		CalleeCorpID: "ding-callee-corp",
		CallType:     "conference",
		RoomID:       "room-1",
		CreateTime:   1780630479000,
		EventTime:    1780630479123,
	}
	if !reflect.DeepEqual(projected, want) {
		t.Fatalf("ProjectOutput() = %#v, want %#v", projected, want)
	}

	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"biz_id":"VOIP_room-1_3559506650"`) || strings.Contains(string(encoded), "filterSubId") || strings.Contains(string(encoded), "sensitive-code") || strings.Contains(string(encoded), "room_code") {
		t.Fatalf("flattened VoIP output = %s, want biz_id without internal or sensitive fields", encoded)
	}
	if !strings.Contains(string(encoded), `"caller_uid":"0147333457361236773"`) || !strings.Contains(string(encoded), `"callee_uid":"digital-3559506650"`) {
		t.Fatalf("flattened VoIP output = %s, want string caller_uid/callee_uid", encoded)
	}
}

func TestCrossPlatformCoverageProjectOutputVoIPAcceptsLegacyNumericUserIdentifiers(t *testing.T) {
	legacy := strings.Replace(personalVoIPData(), `"callerUid":"0147333457361236773"`, `"callerUid":1000000001`, 1)
	legacy = strings.Replace(legacy, `"calleeUid":"digital-3559506650"`, `"calleeUid":3559506650`, 1)

	projected, err := ProjectOutput(transport.Event{EventType: EventVoIPCallReceiveInvite, Data: legacy})
	if err != nil {
		t.Fatalf("ProjectOutput() legacy numeric identifiers error = %v", err)
	}
	got, ok := projected.(VoIPCallReceiveInviteOutput)
	if !ok {
		t.Fatalf("ProjectOutput() type = %T, want VoIPCallReceiveInviteOutput", projected)
	}
	if got.CallerUID != "1000000001" || got.CalleeUID != "3559506650" {
		t.Fatalf("ProjectOutput() legacy identifiers = %q/%q", got.CallerUID, got.CalleeUID)
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsInvalidVoIPPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "missing", want: "payload is missing"},
		{name: "missing body", payload: `,"payload":{"bizid":"biz-1"}`, want: "payload body is missing"},
		{name: "missing bizid", payload: `,"payload":{"body":{"callId":"call-1","roomCode":"sensitive-code"}}`, want: "bizid is required"},
		{name: "invalid caller uid type", payload: `,"payload":{"bizid":"biz-1","body":{"callerUid":{}}}`, want: "VoIP user identifier must be a string or legacy integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := transport.Event{
				EventID:   "outer-event",
				EventType: EventVoIPCallReceiveInvite,
				Data:      fmt.Sprintf(`{"eventKey":%q%s}`, EventVoIPCallReceiveInvite, tt.payload),
			}
			projected, err := ProjectOutput(ev)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "decode personal VoIP payload") {
				t.Fatalf("ProjectOutput() error = %v, want VoIP context and %q", err, tt.want)
			}
			wantFallback := baseEventOutput{Type: EventVoIPCallReceiveInvite, EventID: "outer-event"}
			if got, ok := projected.(baseEventOutput); !ok || !reflect.DeepEqual(got, wantFallback) {
				t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, wantFallback)
			}
			encoded, marshalErr := json.Marshal(projected)
			if marshalErr != nil {
				t.Fatalf("Marshal(fallback) error = %v", marshalErr)
			}
			if strings.Contains(string(encoded), "sensitive-code") || strings.Contains(string(encoded), "roomCode") {
				t.Fatalf("ProjectOutput() fallback leaked room code: %s", encoded)
			}
		})
	}
}

func TestCrossPlatformCoverageProjectOutputVoIPMalformedEnvelopeUsesSafeFallback(t *testing.T) {
	ev := transport.Event{
		EventID:       "outer-event",
		EventBornTime: 1780630479124,
		EventType:     EventVoIPCallReceiveInvite,
		SubscribeID:   "outer-sub",
		Data:          `not-json-sensitive-code`,
	}
	projected, err := ProjectOutput(ev)
	if err == nil || !strings.Contains(err.Error(), "decode personal event data") {
		t.Fatalf("ProjectOutput() error = %v, want data decode context", err)
	}
	want := baseEventOutput{
		Type:        EventVoIPCallReceiveInvite,
		EventID:     "outer-event",
		Timestamp:   1780630479124,
		SubscribeID: "outer-sub",
	}
	if got, ok := projected.(baseEventOutput); !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, want)
	}
	encoded, marshalErr := json.Marshal(projected)
	if marshalErr != nil {
		t.Fatalf("Marshal(fallback) error = %v", marshalErr)
	}
	if strings.Contains(string(encoded), "sensitive-code") {
		t.Fatalf("ProjectOutput() fallback leaked malformed data: %s", encoded)
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsUnsupportedOAType(t *testing.T) {
	ev := transport.Event{EventID: "outer-event", EventType: "user_oa_approval_unknown"}
	projected, err := projectOAApprovalEvent(
		ev,
		baseEventOutput{Type: ev.EventType, EventID: ev.EventID},
		json.RawMessage(`{"body":{"processInstanceId":"process-instance-1"},"event_time":1}`),
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported personal OA event type "user_oa_approval_unknown"`) {
		t.Fatalf("projectOAApprovalEvent() error = %v", err)
	}
	if got, ok := projected.(transport.Event); !ok || !reflect.DeepEqual(got, ev) {
		t.Fatalf("projectOAApprovalEvent() fallback = %#v, want %#v", projected, ev)
	}
}

func TestCrossPlatformCoverageProjectOutputOADecodesDoublyWrappedJSONString(t *testing.T) {
	once, err := json.Marshal(personalOAData(EventOAApprovalTaskCreated))
	if err != nil {
		t.Fatal(err)
	}
	twice, err := json.Marshal(string(once))
	if err != nil {
		t.Fatal(err)
	}

	projected, err := ProjectOutput(transport.Event{Data: string(twice)})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got, ok := projected.(OAApprovalTaskCreatedOutput)
	if !ok {
		t.Fatalf("ProjectOutput() type = %T, want OAApprovalTaskCreatedOutput", projected)
	}
	if got.Type != EventOAApprovalTaskCreated || got.EventID != "oa-event" || got.SubscribeID != "oa-data-sub" {
		t.Fatalf("ProjectOutput() = %#v", got)
	}
}

func TestCrossPlatformCoverageProjectOutputGroupMemberEvents(t *testing.T) {
	for _, eventKey := range []string{EventGroupMemberAdded, EventGroupMemberExited} {
		t.Run(eventKey, func(t *testing.T) {
			projected, err := ProjectOutput(transport.Event{
				EventID:       "outer-event",
				EventBornTime: 11,
				EventType:     eventKey,
				SubscribeID:   "outer-sub",
				Data:          personalGroupMemberData(eventKey),
			})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			want := GroupMemberEventOutput{
				Type:                   eventKey,
				EventID:                "group-member-event",
				Timestamp:              1784782513647,
				SubscribeID:            "outer-sub",
				ConversationID:         "cid-group-1",
				Operator:               "测试用户甲",
				OperatorOpenDingTalkID: "operator-open-id",
				Members: []GroupMemberEventMember{
					{Nick: "测试用户乙", OpenDingTalkID: "member-open-id-1"},
					{Nick: "测试用户丙", OpenDingTalkID: "member-open-id-2"},
				},
				EventTime: 1784782513502,
			}
			if !reflect.DeepEqual(projected, want) {
				t.Fatalf("ProjectOutput() = %#v, want %#v", projected, want)
			}
			assertNoInternalActionFields(t, projected)
		})
	}
}

func TestCrossPlatformCoverageProjectOutputGroupMemberAllowsMissingOperator(t *testing.T) {
	data := strings.ReplaceAll(personalGroupMemberData(EventGroupMemberExited), `"operNick":"测试用户甲",`, "")
	data = strings.ReplaceAll(data, `"operOpenDingtalkId":"operator-open-id",`, "")
	projected, err := ProjectOutput(transport.Event{EventType: EventGroupMemberExited, Data: data})
	if err != nil {
		t.Fatalf("ProjectOutput() error = %v", err)
	}
	got := projected.(GroupMemberEventOutput)
	if got.Operator != "" || got.OperatorOpenDingTalkID != "" {
		t.Fatalf("operator fields = %q/%q, want empty", got.Operator, got.OperatorOpenDingTalkID)
	}
	if len(got.Members) != 2 {
		t.Fatalf("members = %#v, want two members", got.Members)
	}
}

func TestCrossPlatformCoverageProjectOutputGroupMemberRejectsLegacyOperatorOpenIDSpellings(t *testing.T) {
	for _, legacyField := range []string{"operOpenDingtlkId", "operOpenDingTalkId"} {
		t.Run(legacyField, func(t *testing.T) {
			data := strings.Replace(personalGroupMemberData(EventGroupMemberAdded), "operOpenDingtalkId", legacyField, 1)
			projected, err := ProjectOutput(transport.Event{EventType: EventGroupMemberAdded, Data: data})
			if err != nil {
				t.Fatalf("ProjectOutput() error = %v", err)
			}
			got := projected.(GroupMemberEventOutput)
			if got.OperatorOpenDingTalkID != "" {
				t.Fatalf("operator_open_dingtalk_id = %q, want empty for legacy field %s", got.OperatorOpenDingTalkID, legacyField)
			}
			if got.Members[0].OpenDingTalkID != "member-open-id-1" {
				t.Fatalf("members[0].open_dingtalk_id = %q, want protocol openDingTalkId value", got.Members[0].OpenDingTalkID)
			}
		})
	}
}

func TestCrossPlatformCoverageGroupMemberBodyUnmarshalErrors(t *testing.T) {
	var malformed personalGroupMemberBody
	if err := malformed.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("UnmarshalJSON() error = nil, want malformed object error")
	}

	tests := []struct {
		name string
		data string
	}{
		{name: "invalid members", data: `{"members":"invalid"}`},
		{name: "invalid operator open id", data: `{"operOpenDingtalkId":{"unexpected":true}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body personalGroupMemberBody
			if err := json.Unmarshal([]byte(tt.data), &body); err == nil {
				t.Fatal("json.Unmarshal() error = nil, want protocol error")
			}
		})
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsInvalidGroupMemberPayloads(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "missing conversation",
			data: `{"eventKey":"user_im_group_member_added","payload":{"body":{"members":[{"nick":"测试用户甲","openDingTalkId":"member-1"}]},"event_time":1}}`,
		},
		{
			name: "empty conversation",
			data: `{"eventKey":"user_im_group_member_added","payload":{"body":{"openConversationId":" ","members":[{"nick":"测试用户甲","openDingTalkId":"member-1"}]},"event_time":1}}`,
		},
		{
			name: "missing members",
			data: `{"eventKey":"user_im_group_member_added","payload":{"body":{"openConversationId":"cid-1"},"event_time":1}}`,
		},
		{
			name: "empty members",
			data: `{"eventKey":"user_im_group_member_added","payload":{"body":{"openConversationId":"cid-1","members":[]},"event_time":1}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := transport.Event{EventID: "outer-event", EventType: EventGroupMemberAdded, Data: tt.data}
			projected, err := ProjectOutput(ev)
			if err == nil {
				t.Fatal("ProjectOutput() error = nil, want group member validation error")
			}
			if got, ok := projected.(transport.Event); !ok || !reflect.DeepEqual(got, ev) {
				t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, ev)
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

func TestCrossPlatformCoverageProjectOutputRejectsInvalidOAPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "missing"},
		{name: "null", payload: `,"payload":null`},
		{name: "empty object", payload: `,"payload":{}`},
		{name: "array", payload: `,"payload":[]`},
		{name: "string", payload: `,"payload":"invalid"`},
		{name: "missing body", payload: `,"payload":{"event_time":1}`},
		{name: "null body", payload: `,"payload":{"body":null,"event_time":1}`},
		{name: "empty body", payload: `,"payload":{"body":{},"event_time":1}`},
	}
	for _, eventKey := range []string{
		EventOAApprovalTaskCreated,
		EventOAApprovalTaskFinished,
		EventOAApprovalTaskRedirected,
		EventOAApprovalInstanceStarted,
		EventOAApprovalInstanceCC,
		EventOAApprovalInstanceTerminated,
		EventOAApprovalInstanceFinished,
	} {
		for _, tt := range tests {
			t.Run(eventKey+"/"+tt.name, func(t *testing.T) {
				ev := transport.Event{
					EventID:   "outer-event",
					EventType: eventKey,
					Data:      fmt.Sprintf(`{"eventKey":%q%s}`, eventKey, tt.payload),
				}
				projected, err := ProjectOutput(ev)
				if err == nil {
					t.Fatal("ProjectOutput() error = nil, want OA payload validation error")
				}
				if !strings.Contains(err.Error(), "decode personal OA payload") {
					t.Fatalf("ProjectOutput() error = %v, want OA payload context", err)
				}
				got, ok := projected.(transport.Event)
				if !ok || !reflect.DeepEqual(got, ev) {
					t.Fatalf("ProjectOutput() fallback = %#v, want %#v", projected, ev)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageProjectOutputRejectsOAWithoutStableIDs(t *testing.T) {
	tests := []struct {
		name     string
		eventKey string
		body     string
		want     string
	}{
		{
			name:     "missing process instance",
			eventKey: EventOAApprovalInstanceStarted,
			body:     `{"status":"RUNNING"}`,
			want:     "processInstanceId is required",
		},
		{
			name:     "missing task",
			eventKey: EventOAApprovalTaskCreated,
			body:     `{"processInstanceId":"process-instance-1","status":"RUNNING"}`,
			want:     "taskId is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := transport.Event{
				EventID:   "outer-event",
				EventType: tt.eventKey,
				Data: fmt.Sprintf(
					`{"eventKey":%q,"payload":{"body":%s,"event_time":1}}`,
					tt.eventKey,
					tt.body,
				),
			}
			projected, err := ProjectOutput(ev)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ProjectOutput() error = %v, want %q", err, tt.want)
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
		EventGroupMemberAdded,
		EventGroupMemberExited,
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

func TestCrossPlatformCoverageSchemaReflectionSupportsNestedArraysAndPointers(t *testing.T) {
	type nested struct {
		Value  string `json:"value" description:"nested value" format:"nested_id"`
		Hidden string `json:"-"`
	}
	type fixture struct {
		Items []*nested      `json:"items" description:"nested items"`
		Meta  map[string]any `json:"meta" additional_properties:"true"`
	}

	schema := schemaForStruct(reflect.TypeOf((*fixture)(nil)))
	properties := schema["properties"].(map[string]any)
	if len(properties) != 2 {
		t.Fatalf("schema properties = %#v, want items and meta", properties)
	}
	items := properties["items"].(map[string]any)
	itemSchema := items["items"].(map[string]any)
	itemProperties := itemSchema["properties"].(map[string]any)
	if items["type"] != "array" || itemSchema["type"] != "object" || len(itemProperties) != 1 {
		t.Fatalf("nested items schema = %#v", items)
	}
	value := itemProperties["value"].(map[string]any)
	if value["type"] != "string" || value["description"] != "nested value" || value["format"] != "nested_id" {
		t.Fatalf("nested value schema = %#v", value)
	}
	meta := properties["meta"].(map[string]any)
	if meta["type"] != "object" || meta["additionalProperties"] != true {
		t.Fatalf("meta schema = %#v", meta)
	}
}
