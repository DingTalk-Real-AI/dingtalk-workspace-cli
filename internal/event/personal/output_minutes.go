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
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// MinutesEventOutput contains identifiers only; content is read separately with minutes get.
type MinutesEventOutput struct {
	Type                  string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID               string `json:"event_id" description:"平台事件 ID"`
	Timestamp             int64  `json:"timestamp" description:"平台事件时间戳" format:"timestamp_ms"`
	SubscribeID           string `json:"subscribe_id" description:"订阅 ID"`
	BizID                 string `json:"biz_id" description:"生产方业务事件标识，同一次事件重试保持不变"`
	EventTime             int64  `json:"event_time" description:"业务事件发生时间，重试保持不变" format:"timestamp_ms"`
	SchemaVersion         int    `json:"schema_version" description:"听记事件协议版本，首版为 1"`
	TaskUUID              string `json:"task_uuid" description:"听记稳定标识，用于 dws minutes get info 或 summary 的 --id 参数"`
	CreatorOpenDingtalkID string `json:"creator_open_dingtalk_id" description:"听记创建者开放身份标识：正 int64 转为十进制字符串以保持精度，非空字符串原样保留；与内部 UID 的映射待平台确认"`
}

func isMinutesEvent(key string) bool {
	return key == EventMinutesTaskCreated || key == EventMinutesSummaryGenerated
}

func projectMinutesEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload struct {
		BizID     string `json:"bizid"`
		EventTime int64  `json:"event_time"`
		Body      struct {
			SchemaVersion         int             `json:"schemaVersion"`
			TaskUUID              string          `json:"taskUuid"`
			CreatorOpenDingtalkID json.RawMessage `json:"creatorOpenDingtalkId"`
		} `json:"body"`
	}
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal minutes payload: %w", err)
	}
	if payload.Body.SchemaVersion != 1 {
		return ev, fmt.Errorf("decode personal minutes payload: schemaVersion must be 1")
	}
	if strings.TrimSpace(payload.Body.TaskUUID) == "" {
		return ev, fmt.Errorf("decode personal minutes payload: taskUuid is required")
	}
	creatorID, err := parseCreatorOpenDingtalkID(payload.Body.CreatorOpenDingtalkID)
	if err != nil {
		return ev, fmt.Errorf("decode personal minutes payload: %w", err)
	}
	return MinutesEventOutput{
		Type:                  base.Type,
		EventID:               base.EventID,
		Timestamp:             base.Timestamp,
		SubscribeID:           base.SubscribeID,
		BizID:                 payload.BizID,
		EventTime:             payload.EventTime,
		SchemaVersion:         payload.Body.SchemaVersion,
		TaskUUID:              payload.Body.TaskUUID,
		CreatorOpenDingtalkID: creatorID,
	}, nil
}

func parseCreatorOpenDingtalkID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("creatorOpenDingtalkId is required")
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		if n <= 0 {
			return "", fmt.Errorf("creatorOpenDingtalkId must be positive")
		}
		return strconv.FormatInt(n, 10), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("creatorOpenDingtalkId must not be blank")
		}
		return s, nil
	}
	return "", fmt.Errorf("creatorOpenDingtalkId must be a positive integer or non-blank string")
}
