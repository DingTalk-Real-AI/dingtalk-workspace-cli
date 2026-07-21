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
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// fakeLookup returns a fixed table for the "dws demo cmd" command and ok=false
// for anything else, standing in for cli.LookupParamAlias in tests.
func fakeLookup(aliases map[string]string, blocked, ambiguous []string) func(string) (map[string]string, []string, []string, bool) {
	return func(raw string) (map[string]string, []string, []string, bool) {
		if raw != "dws demo cmd" {
			return nil, nil, nil, false
		}
		return aliases, blocked, ambiguous, true
	}
}

func newSemanticHandler() SemanticAliasHandler {
	return SemanticAliasHandler{
		Lookup: fakeLookup(
			map[string]string{"keyword": "query", "page-size": "limit"},
			[]string{"count"},
			[]string{"user-id"},
		),
	}
}

func TestSemanticAliasHandlerRewritesSynonym(t *testing.T) {
	ctx := &pipeline.Context{Command: "dws demo cmd", Args: []string{"--keyword", "hello"}}
	if err := newSemanticHandler().Handle(ctx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if want := []string{"--query", "hello"}; !reflect.DeepEqual(ctx.Args, want) {
		t.Fatalf("Args = %v, want %v", ctx.Args, want)
	}
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Kind != "semantic" || ctx.Corrections[0].Corrected != "--query" {
		t.Fatalf("correction = %#v", ctx.Corrections)
	}
}

func TestSemanticAliasHandlerPreservesEqualsValueSyntax(t *testing.T) {
	ctx := &pipeline.Context{Command: "dws demo cmd", Args: []string{"--pageSize=50"}}
	if err := newSemanticHandler().Handle(ctx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	// --pageSize morphs to page-size, which the table aliases to limit.
	if want := []string{"--limit=50"}; !reflect.DeepEqual(ctx.Args, want) {
		t.Fatalf("Args = %v, want %v", ctx.Args, want)
	}
}

func TestSemanticAliasHandlerLeavesBlockedAndAmbiguous(t *testing.T) {
	ctx := &pipeline.Context{Command: "dws demo cmd", Args: []string{"--count", "10", "--user-id", "u1"}}
	if err := newSemanticHandler().Handle(ctx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if want := []string{"--count", "10", "--user-id", "u1"}; !reflect.DeepEqual(ctx.Args, want) {
		t.Fatalf("blocked/ambiguous names must not be rewritten: Args = %v, want %v", ctx.Args, want)
	}
	if len(ctx.Corrections) != 0 {
		t.Fatalf("no corrections expected, got %#v", ctx.Corrections)
	}
}

func TestSemanticAliasHandlerNoOpCases(t *testing.T) {
	h := newSemanticHandler()

	// Unknown command → Lookup returns ok=false.
	ctx := &pipeline.Context{Command: "dws other", Args: []string{"--keyword", "x"}}
	_ = h.Handle(ctx)
	if !reflect.DeepEqual(ctx.Args, []string{"--keyword", "x"}) || len(ctx.Corrections) != 0 {
		t.Fatalf("unknown command must be a no-op: %v / %#v", ctx.Args, ctx.Corrections)
	}

	// Empty command path.
	ctx = &pipeline.Context{Command: "", Args: []string{"--keyword", "x"}}
	_ = h.Handle(ctx)
	if !reflect.DeepEqual(ctx.Args, []string{"--keyword", "x"}) {
		t.Fatalf("empty command must be a no-op: %v", ctx.Args)
	}

	// Nil Lookup (handler not wired).
	ctx = &pipeline.Context{Command: "dws demo cmd", Args: []string{"--keyword", "x"}}
	_ = SemanticAliasHandler{}.Handle(ctx)
	if !reflect.DeepEqual(ctx.Args, []string{"--keyword", "x"}) {
		t.Fatalf("nil Lookup must be a no-op: %v", ctx.Args)
	}

	// A real flag that also appears nowhere in the table is left alone.
	ctx = &pipeline.Context{Command: "dws demo cmd", Args: []string{"--query", "x", "positional", "-n"}}
	_ = h.Handle(ctx)
	if !reflect.DeepEqual(ctx.Args, []string{"--query", "x", "positional", "-n"}) {
		t.Fatalf("canonical/positional/short tokens must be untouched: %v", ctx.Args)
	}
}

func TestSplitFlagToken(t *testing.T) {
	cases := []struct {
		arg    string
		bare   string
		suffix string
		isFlag bool
	}{
		{"--query", "query", "", true},
		{"--limit=50", "limit", "=50", true},
		{"--pageSize", "pageSize", "", true},
		{"positional", "", "", false},
		{"-n", "", "", false},
		{"--", "", "", false},
		{"--=v", "", "", false},
	}
	for _, c := range cases {
		bare, suffix, isFlag := splitFlagToken(c.arg)
		if bare != c.bare || suffix != c.suffix || isFlag != c.isFlag {
			t.Fatalf("splitFlagToken(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.arg, bare, suffix, isFlag, c.bare, c.suffix, c.isFlag)
		}
	}
}
