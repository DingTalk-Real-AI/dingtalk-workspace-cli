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

func TestPostResponseHandlerMeta(t *testing.T) {
	h := PostResponseHandler{}
	if got := h.Name(); got != "postresponse" {
		t.Errorf("Name() = %q, want %q", got, "postresponse")
	}
	if got := h.Phase(); got != pipeline.PostResponse {
		t.Errorf("Phase() = %v, want %v", got, pipeline.PostResponse)
	}
}

func TestPostResponseHandlerEmptyContext(t *testing.T) {
	h := PostResponseHandler{}
	ctx := &pipeline.Context{}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestPostResponseHandlerNoSideEffects(t *testing.T) {
	h := PostResponseHandler{}
	ctx := &pipeline.Context{
		Command: "aitable.query_records",
		Response: map[string]any{
			"records": []any{
				map[string]any{"id": "rec001"},
			},
			"total": 1,
		},
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if ctx.Response["total"] != 1 {
		t.Error("PostResponseHandler should not mutate Response")
	}
}

func TestPostResponseHandlerNilResponse(t *testing.T) {
	h := PostResponseHandler{}
	ctx := &pipeline.Context{
		Command:  "todo.list",
		Response: nil,
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestPostResponseHandlerInEngine(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.Register(PostResponseHandler{})

	if !engine.HasHandlers(pipeline.PostResponse) {
		t.Fatal("engine should have PostResponse handler")
	}

	ctx := &pipeline.Context{
		Command:  "calendar.list_events",
		Response: map[string]any{"events": []any{}},
	}
	if err := engine.RunPhase(pipeline.PostResponse, ctx); err != nil {
		t.Fatalf("RunPhase(PostResponse) returned error: %v", err)
	}
}
