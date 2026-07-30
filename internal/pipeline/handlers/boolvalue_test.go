// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handlers

import (
	"errors"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestBoolValueHandler(t *testing.T) {
	boolFlags := []pipeline.FlagInfo{{Name: "yes", Shorthand: "y", Type: "bool"}}
	tests := []struct {
		name        string
		args        []string
		flags       []pipeline.FlagInfo
		protected   []string
		want        []string
		corrections int
		conflict    bool
	}{
		{name: "long false", args: []string{"--yes", "false"}, flags: boolFlags, want: []string{"--yes=false"}, corrections: 1},
		{name: "long mixed-case false", args: []string{"--yes", "FaLsE"}, flags: boolFlags, want: []string{"--yes=false"}, corrections: 1},
		{name: "long true synonym", args: []string{"--yes", "yes"}, flags: boolFlags, want: []string{"--yes=true"}, corrections: 1},
		{name: "shorthand zero", args: []string{"-y", "0"}, flags: boolFlags, want: []string{"--yes=false"}, corrections: 1},
		{name: "shorthand equals synonym", args: []string{"-y=off"}, flags: boolFlags, want: []string{"--yes=false"}, corrections: 1},
		{name: "preserves following flags", args: []string{"--yes", "off", "--format", "json"}, flags: boolFlags, want: []string{"--yes=false", "--format", "json"}, corrections: 1},
		{name: "explicit equals stays native", args: []string{"--yes=false"}, flags: boolFlags, want: []string{"--yes=false"}},
		{name: "explicit equals synonym normalizes", args: []string{"--yes=No"}, flags: boolFlags, want: []string{"--yes=false"}, corrections: 1},
		{name: "bare bool stays native", args: []string{"--yes"}, flags: boolFlags, want: []string{"--yes"}},
		{name: "invalid literal is positional", args: []string{"--yes", "maybe"}, flags: boolFlags, want: []string{"--yes", "maybe"}},
		{name: "invalid inline literal stays native", args: []string{"--yes=maybe"}, flags: boolFlags, want: []string{"--yes=maybe"}},
		{name: "non bool flag is unchanged", args: []string{"--name", "false"}, flags: []pipeline.FlagInfo{{Name: "name", Type: "string"}}, want: []string{"--name", "false"}},
		{name: "unknown flag is unchanged", args: []string{"--confirm", "false"}, flags: boolFlags, want: []string{"--confirm", "false"}},
		{name: "shorthand cluster is unchanged", args: []string{"-vy", "false"}, flags: boolFlags, want: []string{"-vy", "false"}},
		{name: "protected bool is unchanged", args: []string{"--yes", "false"}, flags: boolFlags, protected: []string{"yes"}, want: []string{"--yes", "false"}},
		{name: "protected noncanonical bool uses morphed key", args: []string{"--dry_run", "false"}, flags: []pipeline.FlagInfo{{Name: "dry_run", Type: "bool"}}, protected: []string{"dry-run"}, want: []string{"--dry_run", "false"}},
		{name: "stops at double dash", args: []string{"--", "--yes", "false"}, flags: boolFlags, want: []string{"--", "--yes", "false"}},
		{name: "no specs", args: []string{"--yes", "false"}, want: []string{"--yes", "false"}},
		{name: "identical repeated values remain valid", args: []string{"--yes", "--yes=true", "--yes", "yes"}, flags: boolFlags, want: []string{"--yes", "--yes=true", "--yes=true"}, corrections: 1},
		{name: "contradictory detached values fail", args: []string{"--yes", "true", "--yes", "false"}, flags: boolFlags, want: []string{"--yes=true", "--yes=false"}, corrections: 2, conflict: true},
		{name: "bare and explicit false fail", args: []string{"--yes", "--yes=false"}, flags: boolFlags, want: []string{"--yes", "--yes=false"}, conflict: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := &pipeline.Context{
				Args:      append([]string(nil), test.args...),
				FlagSpecs: test.flags,
			}
			for _, protected := range test.protected {
				ctx.ProtectFlag(protected, pipeline.FlagProtectionBlocked)
			}
			handler := BoolValueHandler{}
			err := handler.Handle(ctx)
			var conflict *pipeline.BoolValueConflictError
			if test.conflict {
				if !errors.As(err, &conflict) {
					t.Fatalf("Handle() error = %v, want BoolValueConflictError", err)
				}
				if conflict.Flag != "yes" || !reflect.DeepEqual(conflict.Values, []string{"false", "true"}) {
					t.Fatalf("conflict = %#v", conflict)
				}
			} else if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if !reflect.DeepEqual(ctx.Args, test.want) {
				t.Fatalf("Args = %#v, want %#v", ctx.Args, test.want)
			}
			if len(ctx.Corrections) != test.corrections {
				t.Fatalf("Corrections = %#v, want %d", ctx.Corrections, test.corrections)
			}
			if test.corrections > 0 {
				correction := ctx.Corrections[0]
				if correction.Handler != "boolvalue" || correction.Kind != "explicit-bool" || correction.Field != "--yes" {
					t.Fatalf("correction metadata = %#v", correction)
				}
			}
		})
	}
}

func TestMatchBooleanFlagToken(t *testing.T) {
	longNames := map[string]pipeline.FlagInfo{"yes": {Name: "yes", Shorthand: "y", Type: "bool"}}
	shortNames := map[string]pipeline.FlagInfo{"y": longNames["yes"]}
	tests := []struct {
		argument string
		value    string
		hasValue bool
		matched  bool
	}{
		{argument: "--yes", matched: true},
		{argument: "--yes=false", value: "false", hasValue: true, matched: true},
		{argument: "-y", matched: true},
		{argument: "-y=true", value: "true", hasValue: true, matched: true},
		{argument: "-vy"},
		{argument: "--unknown=false"},
		{argument: "yes"},
		{argument: "--"},
	}
	for _, test := range tests {
		t.Run(test.argument, func(t *testing.T) {
			spec, value, hasValue, matched := matchBooleanFlagToken(test.argument, longNames, shortNames)
			if value != test.value || hasValue != test.hasValue || matched != test.matched {
				t.Fatalf("matchBooleanFlagToken(%q) = %#v, %q, %v, %v", test.argument, spec, value, hasValue, matched)
			}
			if matched && spec.Name != "yes" {
				t.Fatalf("matched spec = %#v", spec)
			}
		})
	}
}

func TestBoolValueHandlerMeta(t *testing.T) {
	handler := BoolValueHandler{}
	if handler.Name() != "boolvalue" || handler.Phase() != pipeline.PreParse {
		t.Fatalf("handler metadata = %q/%v", handler.Name(), handler.Phase())
	}
}
