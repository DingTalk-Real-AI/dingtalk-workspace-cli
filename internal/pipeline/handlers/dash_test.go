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

func TestDashHandler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       []string
		want        string
		corrections int
	}{
		{
			name:        "single dash to double dash",
			args:        []string{"-name", "test"},
			flags:       []string{"name"},
			want:        "--name test",
			corrections: 1,
		},
		{
			name:        "single dash with = syntax",
			args:        []string{"-name=test"},
			flags:       []string{"name"},
			want:        "--name=test",
			corrections: 1,
		},
		{
			name:        "multiple single-dash flags",
			args:        []string{"-email", "a@b.com", "-query", "test"},
			flags:       []string{"email", "query"},
			want:        "--email a@b.com --query test",
			corrections: 2,
		},
		{
			name:        "mixed single and double dash",
			args:        []string{"-name", "test", "--limit", "10"},
			flags:       []string{"name", "limit"},
			want:        "--name test --limit 10",
			corrections: 1,
		},
		{
			name:        "short flag -h is untouched",
			args:        []string{"-h"},
			flags:       []string{"help"},
			want:        "-h",
			corrections: 0,
		},
		{
			name:        "short flag -v is untouched",
			args:        []string{"-v"},
			flags:       []string{"verbose"},
			want:        "-v",
			corrections: 0,
		},
		{
			name:        "short flag -f is untouched",
			args:        []string{"-f", "json"},
			flags:       []string{"format"},
			want:        "-f json",
			corrections: 0,
		},
		{
			name:        "double dash is untouched",
			args:        []string{"--name", "test"},
			flags:       []string{"name"},
			want:        "--name test",
			corrections: 0,
		},
		{
			name:        "already correct flags are untouched",
			args:        []string{"--name", "test", "--limit", "10"},
			flags:       []string{"name", "limit"},
			want:        "--name test --limit 10",
			corrections: 0,
		},
		{
			name:        "single char with value",
			args:        []string{"-n", "test"},
			flags:       []string{"name"},
			want:        "-n test",
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
			name:        "single dash value not a known short flag",
			args:        []string{"-foo", "bar"},
			flags:       []string{"foo"},
			want:        "--foo bar",
			corrections: 1,
		},
		{
			name:        "negative number is untouched",
			args:        []string{"--limit", "-10"},
			flags:       []string{"limit"},
			want:        "--limit -10",
			corrections: 0,
		},
		{
			name:        "negative decimal is untouched",
			args:        []string{"--ratio", "-3.14"},
			flags:       []string{"ratio"},
			want:        "--ratio -3.14",
			corrections: 0,
		},
		{
			name:        "bare single dash",
			args:        []string{"-"},
			flags:       []string{"limit"},
			want:        "-",
			corrections: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: flagSpecs(tt.flags...),
			}
			h := DashHandler{}
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
				if c.Kind != "dash" {
					t.Errorf("correction kind = %q, want %q", c.Kind, "dash")
				}
			}
		})
	}
}

func TestDashHandlerNameAndPhase(t *testing.T) {
	h := DashHandler{}
	if h.Name() != "dash" {
		t.Errorf("Name() = %q, want %q", h.Name(), "dash")
	}
	if h.Phase() != pipeline.PreParse {
		t.Errorf("Phase() = %v, want PreParse", h.Phase())
	}
}

func TestTryFixSingleDash(t *testing.T) {
	tests := []struct {
		name       string
		arg        string
		shorthands map[rune]bool
		want       string
		ok         bool
	}{
		{
			name: "standard single dash",
			arg:  "-name",
			want: "--name",
			ok:   true,
		},
		{
			name: "single dash = syntax",
			arg:  "-name=test",
			want: "--name=test",
			ok:   true,
		},
		{
			name: "short flag single char",
			arg:  "-h",
			want: "",
			ok:   false,
		},
		{
			name: "already double dash",
			arg:  "--name",
			want: "",
			ok:   false,
		},
		{
			name:  "empty string",
			arg:   "",
			want:  "",
			ok:    false,
		},
		{
			name:  "bare single dash",
			arg:   "-",
			want:  "",
			ok:    false,
		},
		{
			name: "negative number is not a flag",
			arg:  "-10",
			want: "",
			ok:   false,
		},
		{
			name: "negative decimal is not a flag",
			arg:  "-3.14",
			want: "",
			ok:   false,
		},
		{
			name: "not a flag (no dash)",
			arg:  "value",
			want: "",
			ok:   false,
		},
		{
			name: "any multi-char single dash gets converted",
			arg:  "-unknown",
			want: "--unknown",
			ok:   true,
		},
		{
			name: "POSIX combined short flags are left intact",
			arg:  "-vf",
			shorthands: map[rune]bool{
				'v': true,
				'f': true,
			},
			want: "",
			ok:   false,
		},
		{
			name: "mixed short+non-short char is converted",
			arg:  "-vn",
			shorthands: map[rune]bool{
				'v': true,
			},
			want: "--vn",
			ok:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tryFixSingleDash(tt.arg, tt.shorthands)
			if ok != tt.ok {
				t.Errorf("tryFixSingleDash(%q) ok = %v, want %v", tt.arg, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("tryFixSingleDash(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}
