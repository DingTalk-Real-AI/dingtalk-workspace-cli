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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func flagSpecs(names ...string) []pipeline.FlagInfo {
	specs := make([]pipeline.FlagInfo, len(names))
	for i, name := range names {
		specs[i] = pipeline.FlagInfo{Name: name, Type: "string"}
	}
	return specs
}

func TestStickyHandler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       []string
		want        string
		corrections int
	}{
		{
			name:        "basic split --limit100",
			args:        []string{"--limit100"},
			flags:       []string{"limit"},
			want:        "--limit 100",
			corrections: 1,
		},
		{
			name:        "no split when flag takes value separately",
			args:        []string{"--limit", "100"},
			flags:       []string{"limit"},
			want:        "--limit 100",
			corrections: 0,
		},
		{
			name:        "no split when = syntax used",
			args:        []string{"--limit=100"},
			flags:       []string{"limit"},
			want:        "--limit=100",
			corrections: 0,
		},
		{
			name:        "no split when flag name is not known",
			args:        []string{"--unknown100"},
			flags:       []string{"limit"},
			want:        "--unknown100",
			corrections: 0,
		},
		{
			name:        "split with string value",
			args:        []string{"--nameJohn"},
			flags:       []string{"name"},
			want:        "--name John",
			corrections: 1,
		},
		{
			name:        "longest prefix wins",
			args:        []string{"--user-id123"},
			flags:       []string{"user", "user-id"},
			want:        "--user-id 123",
			corrections: 1,
		},
		{
			name:        "multiple sticky args in one invocation",
			args:        []string{"--limit100", "--offset50"},
			flags:       []string{"limit", "offset"},
			want:        "--limit 100 --offset 50",
			corrections: 2,
		},
		{
			name:        "mixed sticky and normal args",
			args:        []string{"--limit100", "--name", "test", "--offset50"},
			flags:       []string{"limit", "name", "offset"},
			want:        "--limit 100 --name test --offset 50",
			corrections: 2,
		},
		{
			name:        "single dash prefix is ignored",
			args:        []string{"-l100"},
			flags:       []string{"l"},
			want:        "-l100",
			corrections: 0,
		},
		{
			name:        "empty args",
			args:        []string{},
			flags:       []string{"limit"},
			want:        "",
			corrections: 0,
		},
		{
			name:        "bare double dash",
			args:        []string{"--"},
			flags:       []string{"limit"},
			want:        "--",
			corrections: 0,
		},
		{
			name:        "no flag specs available",
			args:        []string{"--limit100"},
			flags:       []string{},
			want:        "--limit100",
			corrections: 0,
		},
		{
			name:        "exact flag name is not split",
			args:        []string{"--limit"},
			flags:       []string{"limit"},
			want:        "--limit",
			corrections: 0,
		},
		{
			name:        "boolean-like value still splits",
			args:        []string{"--verbosetrue"},
			flags:       []string{"verbose"},
			want:        "--verbose true",
			corrections: 1,
		},
		{
			name:        "hyphenated flag name with numeric suffix",
			args:        []string{"--page-size50"},
			flags:       []string{"page-size", "page"},
			want:        "--page-size 50",
			corrections: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: flagSpecs(tt.flags...),
			}
			h := StickyHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}
			got := strings.Join(ctx.Args, " ")
			if got != tt.want {
				t.Errorf("Args = %q, want %q", got, tt.want)
			}
			if len(ctx.Corrections) != tt.corrections {
				t.Errorf("Corrections count = %d, want %d", len(ctx.Corrections), tt.corrections)
			}
			for _, c := range ctx.Corrections {
				if c.Kind != "sticky" {
					t.Errorf("correction kind = %q, want %q", c.Kind, "sticky")
				}
				if c.Handler != "sticky" {
					t.Errorf("correction handler = %q, want %q", c.Handler, "sticky")
				}
			}
		})
	}
}

func TestStickyHandlerNameAndPhase(t *testing.T) {
	h := StickyHandler{}
	if h.Name() != "sticky" {
		t.Errorf("Name() = %q, want %q", h.Name(), "sticky")
	}
	if h.Phase() != pipeline.PreParse {
		t.Errorf("Phase() = %v, want PreParse", h.Phase())
	}
}
