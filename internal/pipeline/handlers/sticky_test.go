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

// flagSpecs is the legacy helper used by alias/paramname tests. It
// produces string-typed FlagInfo entries which is fine for those
// handlers because they don't consult the type.
//
// Sticky tests should use flagSpecsTyped (below) — the post-hardening
// sticky decision depends on Type/Format/Enum.
func flagSpecs(names ...string) []pipeline.FlagInfo {
	specs := make([]pipeline.FlagInfo, len(names))
	for i, name := range names {
		specs[i] = pipeline.FlagInfo{Name: name, Type: "string"}
	}
	return specs
}

// flagSpec is a compact builder used by sticky tests to declare typed
// flags inline. Type defaults to "string" when unset.
type flagSpec struct {
	name   string
	typ    string
	format string
	enum   []string
}

func specs(in ...flagSpec) []pipeline.FlagInfo {
	out := make([]pipeline.FlagInfo, len(in))
	for i, s := range in {
		t := s.typ
		if t == "" {
			t = "string"
		}
		out[i] = pipeline.FlagInfo{
			Name:   s.name,
			Type:   t,
			Format: s.format,
			Enum:   s.enum,
		}
	}
	return out
}

func TestStickyHandler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       []pipeline.FlagInfo
		want        string
		corrections int
	}{
		{
			name:        "basic split --limit100 (int)",
			args:        []string{"--limit100"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--limit 100",
			corrections: 1,
		},
		{
			name:        "no split when flag takes value separately",
			args:        []string{"--limit", "100"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--limit 100",
			corrections: 0,
		},
		{
			name:        "no split when = syntax used",
			args:        []string{"--limit=100"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--limit=100",
			corrections: 0,
		},
		{
			name:        "no split when flag name is not known",
			args:        []string{"--unknown100"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--unknown100",
			corrections: 0,
		},
		{
			name:        "longest prefix wins",
			args:        []string{"--user-id123"},
			flags:       specs(flagSpec{name: "user"}, flagSpec{name: "user-id", typ: "int"}),
			want:        "--user-id 123",
			corrections: 1,
		},
		{
			name:        "multiple sticky args in one invocation",
			args:        []string{"--limit100", "--offset50"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}, flagSpec{name: "offset", typ: "int"}),
			want:        "--limit 100 --offset 50",
			corrections: 2,
		},
		{
			name: "mixed sticky and normal args",
			args: []string{"--limit100", "--name", "test", "--offset50"},
			flags: specs(
				flagSpec{name: "limit", typ: "int"},
				flagSpec{name: "name"},
				flagSpec{name: "offset", typ: "int"},
			),
			want:        "--limit 100 --name test --offset 50",
			corrections: 2,
		},
		{
			name:        "single dash prefix is ignored",
			args:        []string{"-l100"},
			flags:       specs(flagSpec{name: "l", typ: "int"}),
			want:        "-l100",
			corrections: 0,
		},
		{
			name:        "empty args",
			args:        []string{},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "",
			corrections: 0,
		},
		{
			name:        "bare double dash",
			args:        []string{"--"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--",
			corrections: 0,
		},
		{
			name:        "no flag specs available",
			args:        []string{"--limit100"},
			flags:       nil,
			want:        "--limit100",
			corrections: 0,
		},
		{
			name:        "exact flag name is not split",
			args:        []string{"--limit"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--limit",
			corrections: 0,
		},
		{
			name:        "boolean-like value splits when type is bool",
			args:        []string{"--verbosetrue"},
			flags:       specs(flagSpec{name: "verbose", typ: "bool"}),
			want:        "--verbose true",
			corrections: 1,
		},
		{
			name:        "hyphenated flag name with numeric suffix",
			args:        []string{"--page-size50"},
			flags:       specs(flagSpec{name: "page-size", typ: "int"}, flagSpec{name: "page", typ: "int"}),
			want:        "--page-size 50",
			corrections: 1,
		},

		// --- Hardening cases: typo flags MUST NOT be misinterpreted ---
		{
			name: "typo --starttime1 not split (date-time format)",
			args: []string{"--starttime1", "2026-02-07"},
			flags: specs(flagSpec{
				name: "start", typ: "string", format: "date-time",
			}),
			want:        "--starttime1 2026-02-07",
			corrections: 0,
		},
		{
			name: "glued ISO date splits cleanly (date-time format)",
			args: []string{"--start2026-02-07"},
			flags: specs(flagSpec{
				name: "start", typ: "string", format: "date-time",
			}),
			want:        "--start 2026-02-07",
			corrections: 1,
		},
		{
			name:        "int flag rejects alpha suffix",
			args:        []string{"--limitabc"},
			flags:       specs(flagSpec{name: "limit", typ: "int"}),
			want:        "--limitabc",
			corrections: 0,
		},
		{
			name:        "bool flag rejects non-literal suffix",
			args:        []string{"--verbosehello"},
			flags:       specs(flagSpec{name: "verbose", typ: "bool"}),
			want:        "--verbosehello",
			corrections: 0,
		},
		{
			name: "string + enum: suffix hits enum splits",
			args: []string{"--statusapproved"},
			flags: specs(flagSpec{
				name: "status", typ: "string", enum: []string{"approved", "pending"},
			}),
			want:        "--status approved",
			corrections: 1,
		},
		{
			name: "string + enum: suffix not in enum refuses split",
			args: []string{"--statusunknown"},
			flags: specs(flagSpec{
				name: "status", typ: "string", enum: []string{"approved", "pending"},
			}),
			want:        "--statusunknown",
			corrections: 0,
		},
		{
			name: "email format: suffix containing @ splits",
			args: []string{"--emailfoo@bar.com"},
			flags: specs(flagSpec{
				name: "email", typ: "string", format: "email",
			}),
			want:        "--email foo@bar.com",
			corrections: 1,
		},
		{
			name: "email format: suffix without @ refuses split",
			args: []string{"--emailalice"},
			flags: specs(flagSpec{
				name: "email", typ: "string", format: "email",
			}),
			want:        "--emailalice",
			corrections: 0,
		},
		{
			name:        "stringSlice: refuses to split (ambiguous)",
			args:        []string{"--tagsfoo,bar"},
			flags:       specs(flagSpec{name: "tags", typ: "stringSlice"}),
			want:        "--tagsfoo,bar",
			corrections: 0,
		},
		{
			name:        "plain string + no metadata: alpha suffix refuses split",
			args:        []string{"--nameJohn"},
			flags:       specs(flagSpec{name: "name"}),
			want:        "--nameJohn",
			corrections: 0,
		},
		{
			name:        "plain string + no metadata: digit-led suffix splits",
			args:        []string{"--name123"},
			flags:       specs(flagSpec{name: "name"}),
			want:        "--name 123",
			corrections: 1,
		},
		{
			name:        "duration type: digit-led suffix splits",
			args:        []string{"--timeout30s"},
			flags:       specs(flagSpec{name: "timeout", typ: "duration"}),
			want:        "--timeout 30s",
			corrections: 1,
		},
		{
			name:        "duration type: alpha suffix refuses split",
			args:        []string{"--timeoutever"},
			flags:       specs(flagSpec{name: "timeout", typ: "duration"}),
			want:        "--timeoutever",
			corrections: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string{}, tt.args...),
				FlagSpecs: tt.flags,
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
