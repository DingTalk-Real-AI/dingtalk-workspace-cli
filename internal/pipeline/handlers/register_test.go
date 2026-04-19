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

func TestRegisterHandlerMeta(t *testing.T) {
	h := RegisterHandler{}
	if got := h.Name(); got != "register" {
		t.Errorf("Name() = %q, want %q", got, "register")
	}
	if got := h.Phase(); got != pipeline.Register {
		t.Errorf("Phase() = %v, want %v", got, pipeline.Register)
	}
}

func TestRegisterHandlerEmptyContext(t *testing.T) {
	h := RegisterHandler{}
	ctx := &pipeline.Context{}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestRegisterHandlerWithCommand(t *testing.T) {
	h := RegisterHandler{}
	ctx := &pipeline.Context{
		Command: "aitable",
		Schema: map[string]any{
			"properties": map[string]any{
				"spaceId": map[string]any{"type": "string"},
			},
		},
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestRegisterHandlerNoSideEffects(t *testing.T) {
	h := RegisterHandler{}
	ctx := &pipeline.Context{
		Command: "todo",
		Params:  map[string]any{"key": "value"},
	}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if ctx.Params["key"] != "value" {
		t.Error("RegisterHandler should not mutate Params")
	}
	if ctx.Command != "todo" {
		t.Error("RegisterHandler should not mutate Command")
	}
}

func TestRegisterHandlerInEngine(t *testing.T) {
	engine := pipeline.NewEngine()
	engine.Register(RegisterHandler{})

	if !engine.HasHandlers(pipeline.Register) {
		t.Fatal("engine should have Register handler")
	}

	ctx := &pipeline.Context{Command: "calendar"}
	if err := engine.RunPhase(pipeline.Register, ctx); err != nil {
		t.Fatalf("RunPhase(Register) returned error: %v", err)
	}
}
