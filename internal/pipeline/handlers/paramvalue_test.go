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
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func makeSchema(properties map[string]any) map[string]any {
	return map[string]any{"properties": properties}
}

func TestParamValueHandlerBoolean(t *testing.T) {
	tests := []struct {
		input any
		want  any
	}{
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"no", false},
		{"No", false},
		{"on", true},
		{"off", false},
		{"1", true},
		{"0", false},
		{"True", true},
		{"FALSE", false},
		{"true", true},     // already correct string form
		{"false", false},   // already correct string form
		{true, true},       // already bool — no change
		{false, false},     // already bool — no change
		{"maybe", "maybe"}, // not a boolean — unchanged
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			ctx := &pipeline.Context{
				Params: map[string]any{"flag": tt.input},
				Schema: makeSchema(map[string]any{
					"flag": map[string]any{"type": "boolean"},
				}),
			}
			h := ParamValueHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			got := ctx.Params["flag"]
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParamValueHandlerInteger(t *testing.T) {
	tests := []struct {
		input any
		want  any
	}{
		{"1,000", int64(1000)},
		{"1_000", int64(1000)},
		{"1,000,000", int64(1000000)},
		{"42", "42"},   // no grouping — unchanged
		{42, 42},       // already int — unchanged
		{"abc", "abc"}, // not a number — unchanged
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			ctx := &pipeline.Context{
				Params: map[string]any{"count": tt.input},
				Schema: makeSchema(map[string]any{
					"count": map[string]any{"type": "integer"},
				}),
			}
			h := ParamValueHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			got := ctx.Params["count"]
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParamValueHandlerNumber(t *testing.T) {
	tests := []struct {
		input any
		want  any
	}{
		{"1,234.56", 1234.56},
		{"1_000.5", 1000.5},
		{"3.14", "3.14"}, // no grouping — unchanged
		{3.14, 3.14},     // already float — unchanged
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			ctx := &pipeline.Context{
				Params: map[string]any{"amount": tt.input},
				Schema: makeSchema(map[string]any{
					"amount": map[string]any{"type": "number"},
				}),
			}
			h := ParamValueHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			got := ctx.Params["amount"]
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParamValueHandlerDate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		format string
		want   string
	}{
		{
			name:   "slash date → ISO",
			input:  "2024/03/29",
			format: "date",
			want:   "2024-03-29",
		},
		{
			name:   "already ISO date — no change",
			input:  "2024-03-29",
			format: "date",
			want:   "2024-03-29",
		},
		{
			name:   "space datetime → RFC3339",
			input:  "2024-03-29 14:30:00",
			format: "date-time",
			want:   "2024-03-29T14:30:00Z",
		},
		{
			name:   "compact YYYYMMDD → ISO date",
			input:  "20240329",
			format: "date",
			want:   "2024-03-29",
		},
		{
			name:   "English short month → RFC3339",
			input:  "Mar 29, 2024",
			format: "date-time",
			want:   "2024-03-29T00:00:00Z",
		},
		{
			name:   "English full month → date",
			input:  "January 15, 2024",
			format: "date",
			want:   "2024-01-15",
		},
		{
			name:   "millisecond timestamp → RFC3339",
			input:  "1711699200000",
			format: "date-time",
			want:   "2024-03-29T08:00:00Z",
		},
		{
			name:   "already RFC3339 — no change",
			input:  "2024-03-29T14:30:00Z",
			format: "date-time",
			want:   "2024-03-29T14:30:00Z",
		},
		{
			name:   "ISO without timezone → RFC3339",
			input:  "2024-03-29T14:30:00",
			format: "date-time",
			want:   "2024-03-29T14:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Params: map[string]any{"date": tt.input},
				Schema: makeSchema(map[string]any{
					"date": map[string]any{"type": "string", "format": tt.format},
				}),
			}
			h := ParamValueHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			got, ok := ctx.Params["date"].(string)
			if !ok {
				t.Fatalf("result not a string: %v (%T)", ctx.Params["date"], ctx.Params["date"])
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParamValueHandlerEnum(t *testing.T) {
	tests := []struct {
		name  string
		input any
		enum  []any
		want  any
	}{
		{
			name:  "uppercase → canonical",
			input: "ACTIVE",
			enum:  []any{"active", "inactive"},
			want:  "active",
		},
		{
			name:  "mixed case → canonical",
			input: "Active",
			enum:  []any{"active", "inactive"},
			want:  "active",
		},
		{
			name:  "already correct — no change",
			input: "active",
			enum:  []any{"active", "inactive"},
			want:  "active",
		},
		{
			name:  "no match — unchanged",
			input: "deleted",
			enum:  []any{"active", "inactive"},
			want:  "deleted",
		},
		{
			name:  "non-string value — unchanged",
			input: 42,
			enum:  []any{"active"},
			want:  42,
		},
		{
			name:  "canonical preserves original case",
			input: "json",
			enum:  []any{"JSON", "XML", "CSV"},
			want:  "JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Params: map[string]any{"status": tt.input},
				Schema: makeSchema(map[string]any{
					"status": map[string]any{"type": "string", "enum": tt.enum},
				}),
			}
			h := ParamValueHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			if got := ctx.Params["status"]; got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParamValueHandlerMultipleParams(t *testing.T) {
	ctx := &pipeline.Context{
		Params: map[string]any{
			"verbose":    "yes",
			"count":      "1,000",
			"status":     "ACTIVE",
			"created_at": "2024/03/29",
		},
		Schema: makeSchema(map[string]any{
			"verbose":    map[string]any{"type": "boolean"},
			"count":      map[string]any{"type": "integer"},
			"status":     map[string]any{"type": "string", "enum": []any{"active", "inactive"}},
			"created_at": map[string]any{"type": "string", "format": "date"},
		}),
	}

	h := ParamValueHandler{}
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if got := ctx.Params["verbose"]; got != true {
		t.Errorf("verbose = %v, want true", got)
	}
	if got := ctx.Params["count"]; got != int64(1000) {
		t.Errorf("count = %v, want 1000", got)
	}
	if got := ctx.Params["status"]; got != "active" {
		t.Errorf("status = %q, want %q", got, "active")
	}
	if got := ctx.Params["created_at"]; got != "2024-03-29" {
		t.Errorf("created_at = %q, want %q", got, "2024-03-29")
	}
	if len(ctx.Corrections) != 4 {
		t.Errorf("corrections = %d, want 4", len(ctx.Corrections))
	}
}

func TestParamValueHandlerEmptyInputs(t *testing.T) {
	h := ParamValueHandler{}

	// nil params
	ctx := &pipeline.Context{Params: nil, Schema: makeSchema(map[string]any{})}
	if err := h.Handle(ctx); err != nil {
		t.Errorf("nil params: %v", err)
	}

	// nil schema
	ctx = &pipeline.Context{Params: map[string]any{"a": "b"}, Schema: nil}
	if err := h.Handle(ctx); err != nil {
		t.Errorf("nil schema: %v", err)
	}

	// schema without properties
	ctx = &pipeline.Context{Params: map[string]any{"a": "b"}, Schema: map[string]any{}}
	if err := h.Handle(ctx); err != nil {
		t.Errorf("no properties: %v", err)
	}
}

func TestParamValueHandlerNameAndPhase(t *testing.T) {
	h := ParamValueHandler{}
	if h.Name() != "paramvalue" {
		t.Errorf("Name() = %q, want %q", h.Name(), "paramvalue")
	}
	if h.Phase() != pipeline.PostParse {
		t.Errorf("Phase() = %v, want PostParse", h.Phase())
	}
}
