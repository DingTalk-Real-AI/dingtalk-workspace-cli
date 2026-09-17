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

func personalTodoData(eventKey string) string {
	body := map[string]any{
		"taskId":          "123456",
		"subject":         "待办标题",
		"creatorId":       "creator-staff-id",
		"executorIds":     []string{"executor-1", "executor-2"},
		"participantIds":  []string{"participant-1"},
		"priority":        int64(10),
		"statusStage":     int64(0),
		"planStartDate":   int64(1780630479000),
		"planFinishDate":  int64(1780630480000),
		"startDate":       int64(1780630479000),
		"finishDate":      int64(1780630480000),
		"description":     "任务描述",
		"source":          "TODO",
		"sourceId":        "source_xxx",
		"bizTag":          "tag_xxx",
		"parentId":        nil,
		"isMultiExecutor": false,
		"sceneType":       "PERSONAL",
		"createTime":      int64(1780630479000),
	}
	switch eventKey {
	case EventTodoTaskUpdated:
		body["subject"] = "更新后的标题"
		body["priority"] = int64(20)
		body["statusStage"] = int64(1)
		body["oldStatusStage"] = int64(0)
		body["finishDate"] = nil
		body["updateTime"] = int64(1780630480000)
	case EventTodoTaskDeleted:
		body = map[string]any{
			"taskId":     "123456",
			"subject":    "被删除的待办标题",
			"creatorId":  "creator-staff-id",
			"createTime": int64(1780630479000),
			"deleteTime": int64(1780630480000),
		}
	}
	data := map[string]any{
		"eventId":      "todo-event",
		"eventKey":     eventKey,
		"occurredAtMs": int64(1780630480123),
		"subId":        "todo-data-sub",
		"payload": map[string]any{
			"uid":         100001,
			"clientId":    "internal-client",
			"filterSubId": "internal-filter",
			"body":        body,
		},
	}
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

func TestCrossPlatformCoverageProjectOutputTodoEvents(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		projected, err := ProjectOutput(transport.Event{
			EventType:   EventTodoTaskCreated,
			SubscribeID: "outer-sub",
			Data:        personalTodoData(EventTodoTaskCreated),
		})
		if err != nil {
			t.Fatalf("ProjectOutput() error = %v", err)
		}
		got, ok := projected.(TodoTaskCreatedOutput)
		if !ok {
			t.Fatalf("ProjectOutput() type = %T", projected)
		}
		if got.Type != EventTodoTaskCreated || got.EventID != "todo-event" || got.SubscribeID != "outer-sub" ||
			got.TaskID != "123456" || got.Subject != "待办标题" || got.CreatorID != "creator-staff-id" ||
			len(got.ExecutorIDs) != 2 || len(got.ParticipantIDs) != 1 || got.StatusStage != 0 ||
			got.PlanStartDate == nil || *got.PlanStartDate != 1780630479000 || got.FinishDate == nil ||
			got.CreateTime != 1780630479000 || got.ParentID != nil {
			t.Fatalf("ProjectOutput() = %#v", got)
		}
	})

	t.Run("update", func(t *testing.T) {
		projected, err := ProjectOutput(transport.Event{
			EventType:   EventTodoTaskUpdated,
			SubscribeID: "outer-sub",
			Data:        personalTodoData(EventTodoTaskUpdated),
		})
		if err != nil {
			t.Fatalf("ProjectOutput() error = %v", err)
		}
		got, ok := projected.(TodoTaskUpdatedOutput)
		if !ok {
			t.Fatalf("ProjectOutput() type = %T", projected)
		}
		if got.Type != EventTodoTaskUpdated || got.TaskID != "123456" || got.Subject != "更新后的标题" ||
			got.Priority != 20 || got.StatusStage != 1 || got.OldStatusStage != 0 ||
			got.UpdateTime != 1780630480000 || got.FinishDate != nil {
			t.Fatalf("ProjectOutput() = %#v", got)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		for _, absent := range []string{"finish_date", "parent_id"} {
			if strings.Contains(string(encoded), `"`+absent+`"`) {
				t.Fatalf("Todo update output contains null optional field %q: %s", absent, encoded)
			}
		}
	})

	t.Run("delete", func(t *testing.T) {
		projected, err := ProjectOutput(transport.Event{
			EventType:   EventTodoTaskDeleted,
			SubscribeID: "outer-sub",
			Data:        personalTodoData(EventTodoTaskDeleted),
		})
		if err != nil {
			t.Fatalf("ProjectOutput() error = %v", err)
		}
		got, ok := projected.(TodoTaskDeletedOutput)
		if !ok {
			t.Fatalf("ProjectOutput() type = %T", projected)
		}
		if got.Type != EventTodoTaskDeleted || got.TaskID != "123456" || got.Subject != "被删除的待办标题" ||
			got.CreatorID != "creator-staff-id" || got.CreateTime != 1780630479000 || got.DeleteTime != 1780630480000 {
			t.Fatalf("ProjectOutput() = %#v", got)
		}
	})
}

func TestProjectOutputTodoRequiresTaskID(t *testing.T) {
	data := `{"eventKey":"user_todo_task_create","payload":{"body":{"subject":"missing id"}}}`
	ev := transport.Event{EventType: EventTodoTaskCreated, Data: data}
	projected, err := ProjectOutput(ev)
	if err == nil || !strings.Contains(err.Error(), "taskId is required") {
		t.Fatalf("ProjectOutput() error = %v, want taskId validation", err)
	}
	if got, ok := projected.(transport.Event); !ok || got.EventType != ev.EventType {
		t.Fatalf("ProjectOutput() fallback = %#v, want original envelope", projected)
	}
}

func TestCrossPlatformCoverageProjectTodoEventRejectsMalformedAndUnknownPayloads(t *testing.T) {
	ev := transport.Event{EventType: EventTodoTaskCreated}
	base := baseEventOutput{Type: EventTodoTaskCreated}

	projected, err := projectTodoEvent(ev, base, json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "decode personal Todo payload") {
		t.Fatalf("projectTodoEvent() malformed error = %v", err)
	}
	if got, ok := projected.(transport.Event); !ok || got.EventType != ev.EventType {
		t.Fatalf("projectTodoEvent() malformed fallback = %#v, want original envelope", projected)
	}

	base.Type = "user_todo_task_unknown"
	projected, err = projectTodoEvent(ev, base, json.RawMessage(`{"body":{"taskId":"123456"}}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported personal Todo event type") {
		t.Fatalf("projectTodoEvent() unknown error = %v", err)
	}
	if got, ok := projected.(transport.Event); !ok || got.EventType != ev.EventType {
		t.Fatalf("projectTodoEvent() unknown fallback = %#v, want original envelope", projected)
	}
}
