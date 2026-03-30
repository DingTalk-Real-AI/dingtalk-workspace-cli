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

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
		{"limit", "limt", 1},
		{"limit", "limti", 2},
		{"user-id", "user-di", 2},
		{"page", "pge", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"→"+tt.b, func(t *testing.T) {
			got := levenshtein(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			// Verify symmetry.
			gotRev := levenshtein(tt.b, tt.a)
			if gotRev != got {
				t.Errorf("asymmetric: (%q,%q)=%d but (%q,%q)=%d", tt.a, tt.b, got, tt.b, tt.a, gotRev)
			}
		})
	}
}

func TestParamNameHandler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       []string
		want        string
		corrections int
	}{
		{
			name:        "typo correction — limt → limit",
			args:        []string{"--limt", "10"},
			flags:       []string{"limit", "offset", "name"},
			want:        "--limit 10",
			corrections: 1,
		},
		{
			name:        "transposition — limti → limit",
			args:        []string{"--limti", "10"},
			flags:       []string{"limit", "offset"},
			want:        "--limit 10",
			corrections: 1,
		},
		{
			name:        "known flag unchanged",
			args:        []string{"--limit", "10"},
			flags:       []string{"limit"},
			want:        "--limit 10",
			corrections: 0,
		},
		{
			name:        "too far — no correction",
			args:        []string{"--xyz", "10"},
			flags:       []string{"limit"},
			want:        "--xyz 10",
			corrections: 0,
		},
		{
			name:        "ambiguous — two equally close",
			args:        []string{"--nae", "val"},
			flags:       []string{"name", "nave"},
			want:        "--nae val",
			corrections: 0,
		},
		{
			name:        "short flag tighter threshold",
			args:        []string{"--id", "1"},
			flags:       []string{"id"},
			want:        "--id 1",
			corrections: 0,
		},
		{
			name:        "short flag one edit away",
			args:        []string{"--ix", "1"},
			flags:       []string{"id"},
			want:        "--id 1",
			corrections: 1,
		},
		{
			name:        "short flag two edits away rejected",
			args:        []string{"--xy", "1"},
			flags:       []string{"id"},
			want:        "--xy 1",
			corrections: 0,
		},
		{
			name:        "with = syntax",
			args:        []string{"--limt=10"},
			flags:       []string{"limit"},
			want:        "--limit=10",
			corrections: 1,
		},
		{
			name:        "empty args",
			args:        []string{},
			flags:       []string{"limit"},
			want:        "",
			corrections: 0,
		},
		{
			name:        "single dash ignored",
			args:        []string{"-l"},
			flags:       []string{"limit"},
			want:        "-l",
			corrections: 0,
		},
		{
			name:        "missing char — offse → offset",
			args:        []string{"--offse", "0"},
			flags:       []string{"offset", "limit"},
			want:        "--offset 0",
			corrections: 1,
		},
		{
			name:        "extra char — offsett → offset",
			args:        []string{"--offsett", "0"},
			flags:       []string{"offset", "limit"},
			want:        "--offset 0",
			corrections: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: flagSpecs(tt.flags...),
			}
			h := ParamNameHandler{}
			if err := h.Handle(ctx); err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			got := strings.Join(ctx.Args, " ")
			if got != tt.want {
				t.Errorf("Args = %q, want %q", got, tt.want)
			}
			if len(ctx.Corrections) != tt.corrections {
				t.Errorf("Corrections = %d, want %d", len(ctx.Corrections), tt.corrections)
			}
			for _, c := range ctx.Corrections {
				if c.Kind != "fuzzy" {
					t.Errorf("correction kind = %q, want %q", c.Kind, "fuzzy")
				}
			}
		})
	}
}

func TestParamNameHandlerNameAndPhase(t *testing.T) {
	h := ParamNameHandler{}
	if h.Name() != "paramname" {
		t.Errorf("Name() = %q, want %q", h.Name(), "paramname")
	}
	if h.Phase() != pipeline.PreParse {
		t.Errorf("Phase() = %v, want PreParse", h.Phase())
	}
}
