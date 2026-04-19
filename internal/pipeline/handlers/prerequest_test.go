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

package handlers

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestPreRequestHandlerMeta(t *testing.T) {
	h := PreRequestHandler{}
	if got := h.Name(); got != "prerequest" {
		t.Errorf("Name() = %q, want %q", got, "prerequest")
	}
	if got := h.Phase(); got != pipeline.PreRequest {
		t.Errorf("Phase() = %v, want %v", got, pipeline.PreRequest)
	}
}

func TestPreRequestHandlerEmptyContext(t *testing.T) {
	h := PreRequestHandler{}
	ctx := &pipeline.Context{}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestPreRequestHandlerNoSideEffects(t *testing.T) {
	h := PreRequestHandler{}
	ctx := &pipeline.Context{
		Command: "aitable.query_records",
		Params: map[string]any{
			"spaceId":     "sp001",
			"datasheetId": "ds001",
		},
		Payload: map[string]any{
			"spaceId":     "sp001",
			"datasheetId": "ds001",
		},
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if ctx.Params["spaceId"] != "sp001" {
		t.Error("PreRequestHandler should not mutate Params")
	}
	if ctx.Payload["spaceId"] != "sp001" {
		t.Error("PreRequestHandler should not mutate Payload")
	}
}

func TestPreRequestHandlerNilPayload(t *testing.T) {
	h := PreRequestHandler{}
	ctx := &pipeline.Context{
		Command: "chat.send_message",
		Params:  map[string]any{"userId": "u001"},
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestPreRequestHandlerInEngine(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.Register(PreRequestHandler{})

	if !engine.HasHandlers(pipeline.PreRequest) {
		t.Fatal("engine should have PreRequest handler")
	}

	ctx := &pipeline.Context{
		Command: "todo.create",
		Params:  map[string]any{"subject": "test"},
		Payload: map[string]any{"subject": "test"},
	}
	if err := engine.RunPhase(pipeline.PreRequest, ctx); err != nil {
		t.Fatalf("RunPhase(PreRequest) returned error: %v", err)
	}
}
