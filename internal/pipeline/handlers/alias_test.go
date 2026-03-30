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

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"userId", "user-id"},
		{"UserName", "user-name"},
		{"user_name", "user-name"},
		{"USER_ID", "user-id"},
		{"pageSize", "page-size"},
		{"user-id", "user-id"},
		{"limit", "limit"},
		{"ID", "id"},
		{"userID", "user-id"},
		{"HTMLParser", "html-parser"},
		{"getHTTPResponse", "get-http-response"},
		{"a", "a"},
		{"", ""},
		{"ABC", "abc"},
		{"already-kebab", "already-kebab"},
		{"with spaces", "with-spaces"},
		{"__leading_underscores", "leading-underscores"},
		{"mixedCamel_and_snake", "mixed-camel-and-snake"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toKebabCase(tt.input)
			if got != tt.want {
				t.Errorf("toKebabCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAliasHandler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       []string
		want        string
		corrections int
	}{
		{
			name:        "camelCase to kebab-case",
			args:        []string{"--userId", "123"},
			flags:       []string{"user-id"},
			want:        "--user-id 123",
			corrections: 1,
		},
		{
			name:        "snake_case to kebab-case",
			args:        []string{"--user_name", "alice"},
			flags:       []string{"user-name"},
			want:        "--user-name alice",
			corrections: 1,
		},
		{
			name:        "UPPER to lower",
			args:        []string{"--USER-ID", "456"},
			flags:       []string{"user-id"},
			want:        "--user-id 456",
			corrections: 1,
		},
		{
			name:        "PascalCase to kebab-case",
			args:        []string{"--PageSize", "20"},
			flags:       []string{"page-size"},
			want:        "--page-size 20",
			corrections: 1,
		},
		{
			name:        "already correct — no change",
			args:        []string{"--user-id", "789"},
			flags:       []string{"user-id"},
			want:        "--user-id 789",
			corrections: 0,
		},
		{
			name:        "unknown flag — left untouched",
			args:        []string{"--fooBar", "val"},
			flags:       []string{"user-id"},
			want:        "--fooBar val",
			corrections: 0,
		},
		{
			name:        "with = syntax",
			args:        []string{"--userId=123"},
			flags:       []string{"user-id"},
			want:        "--user-id=123",
			corrections: 1,
		},
		{
			name:        "multiple flags mixed",
			args:        []string{"--userId", "1", "--pageSize", "10", "--name", "test"},
			flags:       []string{"user-id", "page-size", "name"},
			want:        "--user-id 1 --page-size 10 --name test",
			corrections: 2,
		},
		{
			name:        "single dash is ignored",
			args:        []string{"-v"},
			flags:       []string{"verbose"},
			want:        "-v",
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
			name:        "normalised form not in known set",
			args:        []string{"--unknownFlag"},
			flags:       []string{"limit"},
			want:        "--unknownFlag",
			corrections: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: flagSpecs(tt.flags...),
			}
			h := AliasHandler{}
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
				if c.Kind != "alias" {
					t.Errorf("correction kind = %q, want %q", c.Kind, "alias")
				}
			}
		})
	}
}

func TestAliasHandlerNameAndPhase(t *testing.T) {
	h := AliasHandler{}
	if h.Name() != "alias" {
		t.Errorf("Name() = %q, want %q", h.Name(), "alias")
	}
	if h.Phase() != pipeline.PreParse {
		t.Errorf("Phase() = %v, want PreParse", h.Phase())
	}
}
