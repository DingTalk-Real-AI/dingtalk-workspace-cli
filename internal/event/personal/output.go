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
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// MessageEventOutput is the stable business-facing output for personal
// message receive events. Schema output is generated from these tags so the
// documented fields cannot drift from the values written by consume.
type MessageEventOutput struct {
	Type                 string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID              string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp            int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID          string `json:"subscribe_id" description:"订阅 ID"`
	MessageID            string `json:"message_id" description:"开放消息 ID" format:"open_message_id"`
	ConversationID       string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Sender               string `json:"sender" description:"发送人展示名"`
	SenderOpenDingTalkID string `json:"sender_open_dingtalk_id" description:"发送人开放 ID" format:"open_dingtalk_id"`
	Content              string `json:"content" description:"消息正文"`
	CreateTime           string `json:"create_time" description:"消息创建时间"`
	EventTime            int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type ReadEventOutput struct {
	Type                 string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID              string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp            int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID          string `json:"subscribe_id" description:"订阅 ID"`
	MessageID            string `json:"message_id" description:"被读取消息的开放消息 ID" format:"open_message_id"`
	ConversationID       string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Reader               string `json:"reader" description:"读取消息的用户展示名"`
	ReaderOpenDingTalkID string `json:"reader_open_dingtalk_id" description:"读取消息的用户开放 ID" format:"open_dingtalk_id"`
	Sender               string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	ReadTime             string `json:"read_time" description:"消息读取时间"`
	EventTime            int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type RecallEventOutput struct {
	Type                   string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID                string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp              int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID            string `json:"subscribe_id" description:"订阅 ID"`
	MessageID              string `json:"message_id" description:"被撤回消息的开放消息 ID" format:"open_message_id"`
	ConversationID         string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Recaller               string `json:"recaller" description:"撤回消息的用户展示名"`
	RecallerOpenDingTalkID string `json:"recaller_open_dingtalk_id" description:"撤回消息的用户开放 ID" format:"open_dingtalk_id"`
	Sender                 string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID   string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	RecallTime             string `json:"recall_time" description:"消息撤回时间"`
	EventTime              int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type ReactionEventOutput struct {
	Type                   string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID                string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp              int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID            string `json:"subscribe_id" description:"订阅 ID"`
	MessageID              string `json:"message_id" description:"收到表情回应的开放消息 ID" format:"open_message_id"`
	ConversationID         string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Operator               string `json:"operator" description:"执行表情回应操作的用户展示名"`
	OperatorOpenDingTalkID string `json:"operator_open_dingtalk_id" description:"执行表情回应操作的用户开放 ID" format:"open_dingtalk_id"`
	ReactionName           string `json:"reaction_name" description:"表情回应名称"`
	ReactionText           string `json:"reaction_text" description:"表情回应文本"`
	OperationType          string `json:"operation_type" description:"表情回应操作类型"`
	OperationTime          string `json:"operation_time" description:"表情回应操作时间"`
	Sender                 string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID   string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	EventTime              int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type baseEventOutput struct {
	Type        string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID     string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp   int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID string `json:"subscribe_id" description:"订阅 ID"`
}

type personalEventData struct {
	EventID      string          `json:"eventId"`
	EventKey     string          `json:"eventKey"`
	OccurredAtMS int64           `json:"occurredAtMs"`
	SubID        string          `json:"subId"`
	Payload      json.RawMessage `json:"payload"`
}

type personalMessagePayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		CreateTime           string `json:"createTime"`
		Sender               string `json:"sender"`
		OpenMessageID        string `json:"openMessageId"`
		SenderOpenDingTalkID string `json:"senderOpenDingTalkId"`
		OpenConversationID   string `json:"openConversationId"`
		Content              string `json:"content"`
	} `json:"body"`
}

type personalReadPayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		MessageID            string `json:"openMessageId"`
		ConversationID       string `json:"openConversationId"`
		Reader               string `json:"reader"`
		ReaderOpenDingTalkID string `json:"readerOpenDingTalkId"`
		Sender               string `json:"sender"`
		SenderOpenDingTalkID string `json:"senderOpenDingTalkId"`
		ReadTime             string `json:"msgReadTime"`
	} `json:"body"`
}

type personalRecallPayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		MessageID              string `json:"openMessageId"`
		ConversationID         string `json:"openConversationId"`
		Recaller               string `json:"recaller"`
		RecallerOpenDingTalkID string `json:"recallerOpenDingTalkId"`
		Sender                 string `json:"sender"`
		SenderOpenDingTalkID   string `json:"senderOpenDingTalkId"`
		RecallTime             string `json:"msgRecallTime"`
	} `json:"body"`
}

type personalReactionPayload struct {
	EventTime int64                `json:"event_time"`
	Body      personalReactionBody `json:"body"`
}

type personalReactionBody struct {
	MessageID              string `json:"openSourceMessageId"`
	ConversationID         string `json:"openConversationId"`
	Operator               string `json:"oper"`
	OperatorOpenDingTalkID string `json:"-"`
	ReactionName           string `json:"emotionName"`
	ReactionText           string `json:"emotionText"`
	OperationType          string `json:"operateType"`
	OperationTime          string `json:"operateTime"`
	Sender                 string `json:"sender"`
	SenderOpenDingTalkID   string `json:"senderOpenDingTalkId"`
}

func (b *personalReactionBody) UnmarshalJSON(data []byte) error {
	// encoding/json otherwise falls back to case-insensitive field matching.
	// Read this protocol field from a map so only operOpenDingtalkId is accepted.
	type bodyAlias personalReactionBody
	var decoded bodyAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["operOpenDingtalkId"]; ok {
		if err := json.Unmarshal(raw, &decoded.OperatorOpenDingTalkID); err != nil {
			return fmt.Errorf("decode operOpenDingtalkId: %w", err)
		}
	}

	*b = personalReactionBody(decoded)
	return nil
}

// ProjectOutput converts the transport envelope into the stable personal
// event output. On malformed Data it returns the original envelope together
// with an error; the formatter logs the warning and still emits that envelope.
func ProjectOutput(ev transport.Event) (any, error) {
	data, err := decodePersonalEventData(ev.Data)
	if err != nil {
		return ev, fmt.Errorf("decode personal event data: %w", err)
	}

	eventType := firstNonEmptyOutput(ev.EventType, data.EventKey)
	eventID := firstNonEmptyOutput(data.EventID, ev.EventID)
	timestamp := data.OccurredAtMS
	if timestamp == 0 {
		timestamp = ev.EventBornTime
	}
	subscribeID := firstNonEmptyOutput(ev.SubscribeID, data.SubID)

	if isMessageReceiveEvent(eventType) {
		var payload personalMessagePayload
		if err := decodeRequiredPayload(data.Payload, &payload); err != nil {
			return ev, fmt.Errorf("decode personal message payload: %w", err)
		}
		return MessageEventOutput{
			Type:                 eventType,
			EventID:              eventID,
			Timestamp:            timestamp,
			SubscribeID:          subscribeID,
			MessageID:            payload.Body.OpenMessageID,
			ConversationID:       payload.Body.OpenConversationID,
			Sender:               payload.Body.Sender,
			SenderOpenDingTalkID: payload.Body.SenderOpenDingTalkID,
			Content:              payload.Body.Content,
			CreateTime:           payload.Body.CreateTime,
			EventTime:            payload.EventTime,
		}, nil
	}

	base := baseEventOutput{
		Type:        eventType,
		EventID:     eventID,
		Timestamp:   timestamp,
		SubscribeID: subscribeID,
	}
	switch {
	case isReadEvent(eventType):
		return projectReadEvent(ev, base, data.Payload)
	case isRecallEvent(eventType):
		return projectRecallEvent(ev, base, data.Payload)
	case isReactionEvent(eventType):
		return projectReactionEvent(ev, base, data.Payload)
	default:
		return ev, fmt.Errorf("unsupported personal event type %q", eventType)
	}
}

func projectReadEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalReadPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal read payload: %w", err)
	}
	return ReadEventOutput{
		Type:                 base.Type,
		EventID:              base.EventID,
		Timestamp:            base.Timestamp,
		SubscribeID:          base.SubscribeID,
		MessageID:            payload.Body.MessageID,
		ConversationID:       payload.Body.ConversationID,
		Reader:               payload.Body.Reader,
		ReaderOpenDingTalkID: payload.Body.ReaderOpenDingTalkID,
		Sender:               payload.Body.Sender,
		SenderOpenDingTalkID: payload.Body.SenderOpenDingTalkID,
		ReadTime:             payload.Body.ReadTime,
		EventTime:            payload.EventTime,
	}, nil
}

func projectRecallEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalRecallPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal recall payload: %w", err)
	}
	return RecallEventOutput{
		Type:                   base.Type,
		EventID:                base.EventID,
		Timestamp:              base.Timestamp,
		SubscribeID:            base.SubscribeID,
		MessageID:              payload.Body.MessageID,
		ConversationID:         payload.Body.ConversationID,
		Recaller:               payload.Body.Recaller,
		RecallerOpenDingTalkID: payload.Body.RecallerOpenDingTalkID,
		Sender:                 payload.Body.Sender,
		SenderOpenDingTalkID:   payload.Body.SenderOpenDingTalkID,
		RecallTime:             payload.Body.RecallTime,
		EventTime:              payload.EventTime,
	}, nil
}

func projectReactionEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalReactionPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal reaction payload: %w", err)
	}
	return ReactionEventOutput{
		Type:                   base.Type,
		EventID:                base.EventID,
		Timestamp:              base.Timestamp,
		SubscribeID:            base.SubscribeID,
		MessageID:              payload.Body.MessageID,
		ConversationID:         payload.Body.ConversationID,
		Operator:               payload.Body.Operator,
		OperatorOpenDingTalkID: payload.Body.OperatorOpenDingTalkID,
		ReactionName:           payload.Body.ReactionName,
		ReactionText:           payload.Body.ReactionText,
		OperationType:          payload.Body.OperationType,
		OperationTime:          payload.Body.OperationTime,
		Sender:                 payload.Body.Sender,
		SenderOpenDingTalkID:   payload.Body.SenderOpenDingTalkID,
		EventTime:              payload.EventTime,
	}, nil
}

func decodeRequiredPayload(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("payload is missing")
	}

	var payloadObject map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &payloadObject); err != nil {
		return err
	}
	if len(payloadObject) == 0 {
		return fmt.Errorf("payload is empty")
	}

	body, ok := payloadObject["body"]
	if !ok {
		return fmt.Errorf("payload body is missing")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return fmt.Errorf("payload body is missing")
	}
	var bodyObject map[string]json.RawMessage
	if err := json.Unmarshal(body, &bodyObject); err != nil {
		return fmt.Errorf("decode payload body: %w", err)
	}
	if len(bodyObject) == 0 {
		return fmt.Errorf("payload body is empty")
	}

	return json.Unmarshal(trimmed, target)
}

func decodePersonalEventData(raw string) (personalEventData, error) {
	encoded := []byte(strings.TrimSpace(raw))
	if len(encoded) == 0 {
		return personalEventData{}, fmt.Errorf("empty data")
	}

	// Some gateways wrap the JSON object in one or more JSON strings. Peel
	// those wrappers without changing the raw transport envelope.
	for depth := 0; depth < 2; depth++ {
		var quoted string
		if err := json.Unmarshal(encoded, &quoted); err != nil {
			break
		}
		encoded = []byte(strings.TrimSpace(quoted))
	}

	var data personalEventData
	if err := json.Unmarshal(encoded, &data); err != nil {
		return personalEventData{}, err
	}
	if data.EventKey == "" && data.EventID == "" && data.OccurredAtMS == 0 && data.SubID == "" && len(data.Payload) == 0 {
		return personalEventData{}, fmt.Errorf("data is not a personal event object")
	}
	return data, nil
}

func outputSchema(eventKey string) map[string]any {
	outputType := outputTypeForEvent(eventKey)
	properties := make(map[string]any, outputType.NumField())
	for i := 0; i < outputType.NumField(); i++ {
		field := outputType.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		property := map[string]any{
			"type": schemaType(field.Type),
		}
		if description := field.Tag.Get("description"); description != "" {
			property["description"] = description
		}
		if format := field.Tag.Get("format"); format != "" {
			property["format"] = format
		}
		if field.Tag.Get("additional_properties") == "true" {
			property["additionalProperties"] = true
		}
		if name == "type" {
			property["enum"] = []string{eventKey}
		}
		properties[name] = property
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

func transportEnvelopeSchema(eventKey string) map[string]any {
	eventType := reflect.TypeOf(transport.Event{})
	properties := make(map[string]any, eventType.NumField())
	for i := 0; i < eventType.NumField(); i++ {
		field := eventType.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		property := map[string]any{"type": schemaType(field.Type)}
		switch name {
		case "type":
			property["description"] = "transport frame 类型"
			property["enum"] = []string{string(transport.FrameTypeEvent)}
		case "event_type":
			property["description"] = "事件类型"
			property["enum"] = []string{eventKey}
		case "data":
			property["description"] = "服务端业务 payload JSON 字符串"
			property["content_media_type"] = "application/json"
		case "headers":
			property["description"] = "Stream transport headers"
			property["additionalProperties"] = map[string]any{"type": "string"}
		case "event_id":
			property["description"] = "transport 事件 ID"
		case "subscribe_id":
			property["description"] = "个人事件订阅 ID"
		}
		properties[name] = property
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

func outputTypeForEvent(eventKey string) reflect.Type {
	switch {
	case isMessageReceiveEvent(eventKey):
		return reflect.TypeOf(MessageEventOutput{})
	case isReadEvent(eventKey):
		return reflect.TypeOf(ReadEventOutput{})
	case isRecallEvent(eventKey):
		return reflect.TypeOf(RecallEventOutput{})
	case isReactionEvent(eventKey):
		return reflect.TypeOf(ReactionEventOutput{})
	default:
		return reflect.TypeOf(baseEventOutput{})
	}
}

func isReadEvent(eventKey string) bool {
	return eventKey == EventReadO2O || eventKey == EventReadGroup
}

func isRecallEvent(eventKey string) bool {
	return eventKey == EventRecallO2O || eventKey == EventRecallGroup
}

func isReactionEvent(eventKey string) bool {
	return eventKey == EventReactionO2O || eventKey == EventReactionGroup
}

func schemaType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Bool:
		return "boolean"
	default:
		return "object"
	}
}

func firstNonEmptyOutput(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
