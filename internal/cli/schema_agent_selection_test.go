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

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildManualAgentSelectionEvalFixtureUsesReviewedScenariosAndRealCommands(t *testing.T) {
	bound := manualAgentSelectionBoundFixture()
	hints := manualAgentHintSetFixture()

	fixture, report, err := BuildManualAgentSelectionEvalFixture(bound, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentSelectionEvalFixture() error = %v", err)
	}
	if report.Tools != 1 || report.PositiveAssertions != 1 || report.NegativeAssertions != 1 {
		t.Fatalf("selection report = %+v", report)
	}
	if report.FixtureSHA256 == "" || len(fixture.Cases) != 2 {
		t.Fatalf("selection fixture = %+v, report = %+v", fixture, report)
	}
	positive, negative := fixture.Cases[0], fixture.Cases[1]
	if positive.ExpectedCanonical != "sample.search_items" || positive.ForbiddenCanonical != "" {
		t.Fatalf("positive case = %+v", positive)
	}
	if negative.ForbiddenCanonical != "sample.search_items" || negative.ExpectedCanonical != "" {
		t.Fatalf("negative case = %+v", negative)
	}
	if !reflect.DeepEqual(positive.CandidateCanonicals, []string{"sample.search_items"}) {
		t.Fatalf("positive candidates = %#v", positive.CandidateCanonicals)
	}

	_, repeated, err := BuildManualAgentSelectionEvalFixture(bound, hints)
	if err != nil || repeated.FixtureSHA256 != report.FixtureSHA256 {
		t.Fatalf("repeated fixture digest = %q, err=%v; want %q", repeated.FixtureSHA256, err, report.FixtureSHA256)
	}
}

func TestBuildManualAgentSelectionEvalFixtureRejectsFactualContractDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*BoundCommandRegistry, *ManualAgentHintSet)
		wantErr string
	}{
		{
			name: "missing reviewed tool",
			mutate: func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
				delete(hints.Tools, "sample.search_items")
			},
			wantErr: "selection tools do not exactly match",
		},
		{
			name: "unbound primary command",
			mutate: func(bound *BoundCommandRegistry, _ *ManualAgentHintSet) {
				item := bound.ByCanonical["sample.search_items"]
				item.PrimaryCommand = nil
				bound.ByCanonical["sample.search_items"] = item
			},
			wantErr: "no bound primary Cobra command",
		},
		{
			name: "literal positive negative contradiction",
			mutate: func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
				item := hints.Tools["sample.search_items"]
				item.AvoidWhen = append([]string(nil), item.UseWhen...)
				hints.Tools["sample.search_items"] = item
			},
			wantErr: "same literal positive and negative",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bound := manualAgentSelectionBoundFixture()
			hints := manualAgentHintSetFixture()
			test.mutate(&bound, &hints)
			_, _, err := BuildManualAgentSelectionEvalFixture(bound, hints)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestBuildManualAgentSelectionEvalFixtureRejectsOneScenarioWithTwoExpectedTools(t *testing.T) {
	bound := manualAgentSelectionBoundFixture()
	root := bound.ByCanonical["sample.search_items"].PrimaryCommand.Root()
	create := &cobra.Command{Use: "create", RunE: func(*cobra.Command, []string) error { return nil }}
	exactSchemaCommand(root, "sample item").AddCommand(create)
	createSpec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.create_item",
			PrimaryCLIPath: "sample item create",
		},
		PrimaryCommand: create,
	}
	bound.Commands = append(bound.Commands, createSpec)
	bound.ByCanonical[createSpec.CanonicalPath] = createSpec
	bound.ByCLIPath[createSpec.PrimaryCLIPath] = createSpec

	hints := manualAgentHintSetFixture()
	search := hints.Tools["sample.search_items"]
	createHint := search
	createHint.AgentSummary = "Create a sample item"
	createHint.Examples = []string{"dws sample item create"}
	createHint.UseWhen = append([]string(nil), search.UseWhen...)
	createHint.AvoidWhen = []string{"An existing sample item must be found"}
	hints.Tools[createSpec.CanonicalPath] = createHint

	_, _, err := BuildManualAgentSelectionEvalFixture(bound, hints)
	if err == nil || !strings.Contains(err.Error(), "conflicting literal expectations") {
		t.Fatalf("error = %v, want literal expectation conflict", err)
	}
}

func manualAgentSelectionBoundFixture() BoundCommandRegistry {
	_, leaf := manualSchemaHintTestTree()
	spec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.search_items",
			PrimaryCLIPath: "sample item search",
		},
		PrimaryCommand: leaf,
	}
	return BoundCommandRegistry{
		Commands:    []BoundCommandSpec{spec},
		ByCanonical: map[string]BoundCommandSpec{spec.CanonicalPath: spec},
		ByCLIPath:   map[string]BoundCommandSpec{spec.PrimaryCLIPath: spec},
	}
}
