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

package compat

import (
	"reflect"
	"sort"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

func TestApplyCrossEditionAliases_NilOverride(t *testing.T) {
	// Nil override must not panic.
	applyCrossEditionAliases(nil, "sheet.find_cells")
}

func TestApplyCrossEditionAliases_UnknownCanonicalPath(t *testing.T) {
	override := &market.CLIToolOverride{
		Flags: map[string]market.CLIFlagOverride{
			"text": {Alias: "find"},
		},
	}
	applyCrossEditionAliases(override, "unknown.tool")
	flag := override.Flags["text"]
	if len(flag.Aliases) != 0 {
		t.Fatalf("unexpected aliases injected for unknown path: %v", flag.Aliases)
	}
}

func TestApplyCrossEditionAliases_SheetFindCells_AddsQuery(t *testing.T) {
	override := &market.CLIToolOverride{
		Flags: map[string]market.CLIFlagOverride{
			"text": {Alias: "find"},
		},
	}
	applyCrossEditionAliases(override, "sheet.find_cells")
	flag := override.Flags["text"]
	if flag.Alias != "find" {
		t.Errorf("primary alias must not be overwritten, got %q", flag.Alias)
	}
	if !reflect.DeepEqual(flag.Aliases, []string{"query"}) {
		t.Errorf("expected hidden alias [query], got %v", flag.Aliases)
	}
}

func TestApplyCrossEditionAliases_DedupesAgainstExisting(t *testing.T) {
	// If server overlay already declared --query as an alias (shouldn't
	// happen today, but future-proofing), we must not duplicate it.
	override := &market.CLIToolOverride{
		Flags: map[string]market.CLIFlagOverride{
			"text": {Alias: "find", Aliases: []string{"query"}},
		},
	}
	applyCrossEditionAliases(override, "sheet.find_cells")
	flag := override.Flags["text"]
	if !reflect.DeepEqual(flag.Aliases, []string{"query"}) {
		t.Errorf("alias should be deduped, got %v", flag.Aliases)
	}
}

func TestApplyCrossEditionAliases_DedupesAgainstPrimary(t *testing.T) {
	// If the primary alias already happens to be "query" (e.g. hypothetical
	// inverse rename), we must not add it again as a hidden alias.
	override := &market.CLIToolOverride{
		Flags: map[string]market.CLIFlagOverride{
			"text": {Alias: "query"},
		},
	}
	applyCrossEditionAliases(override, "sheet.find_cells")
	flag := override.Flags["text"]
	if len(flag.Aliases) != 0 {
		t.Errorf("must not duplicate primary as alias, got %v", flag.Aliases)
	}
}

func TestApplyCrossEditionAliases_NilFlagsMap(t *testing.T) {
	// Override without any Flags map yet — function must initialize it.
	override := &market.CLIToolOverride{}
	applyCrossEditionAliases(override, "sheet.find_cells")
	if override.Flags == nil {
		// Map auto-initialized for the param.
		t.Fatalf("Flags map should have been initialized")
	}
	flag := override.Flags["text"]
	if !reflect.DeepEqual(flag.Aliases, []string{"query"}) {
		t.Errorf("expected [query] when injecting into empty Flags, got %v", flag.Aliases)
	}
}

func TestCrossEditionRegistry_NoEmptyAliases(t *testing.T) {
	// Lint: registry entries must not declare empty strings as aliases —
	// they'd be silently dropped by applyCrossEditionAliases and clutter
	// the table.
	for canonical, params := range crossEditionFlagAliases {
		for paramName, aliases := range params {
			for _, a := range aliases {
				if a == "" {
					t.Errorf("empty alias in %s.%s", canonical, paramName)
				}
			}
			// Ensure each alias list is unique within itself.
			seen := map[string]bool{}
			for _, a := range aliases {
				if seen[a] {
					t.Errorf("duplicate alias %q in %s.%s", a, canonical, paramName)
				}
				seen[a] = true
			}
			// Sortedness is not required but make sure deterministic
			// iteration order downstream isn't accidentally relied on.
			_ = sort.StringsAreSorted(aliases)
		}
	}
}
