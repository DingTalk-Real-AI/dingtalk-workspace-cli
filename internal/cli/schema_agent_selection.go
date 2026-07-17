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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var marshalManualAgentSelectionFixture = json.Marshal

const manualAgentSelectionFixtureVersion = 1

// ManualAgentSelectionCase is one reproducible model-evaluation assertion
// derived from reviewed Manual Agent hints. Positive cases require one exact
// canonical result; negative cases only forbid the command that owns the
// avoid_when text. CandidateCanonicals contains every bound tool in the same
// product, in stable order.
type ManualAgentSelectionCase struct {
	ID                  string   `json:"id"`
	ProductID           string   `json:"product_id"`
	Scenario            string   `json:"scenario"`
	ExpectedCanonical   string   `json:"expected_canonical,omitempty"`
	ForbiddenCanonical  string   `json:"forbidden_canonical,omitempty"`
	CandidateCanonicals []string `json:"candidate_canonicals"`
}

// ManualAgentSelectionFixture is the stable input to an optional live Agent
// evaluation. It is built from schema_hints/selection and the real bound
// command tree; it is not a second authored hint source.
type ManualAgentSelectionFixture struct {
	Version int                        `json:"version"`
	Cases   []ManualAgentSelectionCase `json:"cases"`
}

// ManualAgentSelectionReport records deterministic coverage and the exact
// fixture digest. It proves that all reviewed assertions are well-formed and
// executable; it deliberately does not claim that a language model understood
// their natural-language meaning.
type ManualAgentSelectionReport struct {
	Tools              int
	PositiveAssertions int
	NegativeAssertions int
	FixtureSHA256      string
}

// BuildManualAgentSelectionEvalFixture turns every use_when and avoid_when
// entry into a typed, reproducible evaluation case and validates it against
// the exact BoundCommandRegistry.
//
// Deterministic CI can prove full coverage, real command binding, and absence
// of literal contradictory expectations. Semantic command choice belongs to
// an opt-in live-model evaluation that consumes this fixture; this function
// never substitutes string matching for Agent behavior.
func BuildManualAgentSelectionEvalFixture(bound BoundCommandRegistry, hints ManualAgentHintSet) (ManualAgentSelectionFixture, ManualAgentSelectionReport, error) {
	fixture := ManualAgentSelectionFixture{Version: manualAgentSelectionFixtureVersion}
	report := ManualAgentSelectionReport{}
	expectedTools := make(map[string]bool, len(bound.Commands))
	candidatesByProduct := map[string][]string{}
	for _, command := range bound.Commands {
		canonical := strings.TrimSpace(command.CanonicalPath)
		expectedTools[canonical] = true
		productID, _, ok := strings.Cut(canonical, ".")
		if !ok || strings.TrimSpace(productID) == "" {
			return fixture, report, fmt.Errorf("agent_hints selection has invalid bound canonical path %q", canonical)
		}
		candidatesByProduct[productID] = append(candidatesByProduct[productID], canonical)
	}
	if err := validateManualAgentHintExactSet("selection tools", expectedTools, mapKeysManualAgentTools(hints.Tools)); err != nil {
		return fixture, report, err
	}
	for productID := range candidatesByProduct {
		sort.Strings(candidatesByProduct[productID])
	}

	canonicals := make([]string, 0, len(expectedTools))
	for canonical := range expectedTools {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)

	positiveExpectations := map[string]string{}
	positiveDisplays := map[string]string{}
	for _, canonical := range canonicals {
		command, ok := bound.ByCanonical[canonical]
		if !ok {
			return fixture, report, fmt.Errorf("agent_hints selection expected canonical %q is missing from BoundCommandRegistry.ByCanonical", canonical)
		}
		if err := validateManualAgentSelectionBinding(bound, canonical, command); err != nil {
			return fixture, report, err
		}

		hint := hints.Tools[canonical]
		if len(hint.UseWhen) == 0 {
			return fixture, report, fmt.Errorf("agent_hints tool %s requires at least one positive use_when selection assertion", canonical)
		}
		if len(hint.AvoidWhen) == 0 {
			return fixture, report, fmt.Errorf("agent_hints tool %s requires at least one negative avoid_when selection assertion", canonical)
		}
		productID, _, _ := strings.Cut(canonical, ".")
		candidates := candidatesByProduct[productID]
		for index, scenario := range hint.UseWhen {
			normalized := normalizeManualAgentSelectionScenario(scenario)
			if normalized == "" {
				return fixture, report, fmt.Errorf("agent_hints tool %s has an empty normalized use_when selection assertion", canonical)
			}
			if previous, exists := positiveExpectations[normalized]; exists {
				return fixture, report, fmt.Errorf("agent_hints use_when scenario %q has conflicting literal expectations %q and %q", positiveDisplays[normalized], previous, canonical)
			}
			positiveExpectations[normalized] = canonical
			positiveDisplays[normalized] = strings.TrimSpace(scenario)
			fixture.Cases = append(fixture.Cases, ManualAgentSelectionCase{
				ID:                  fmt.Sprintf("%s/use_when/%d", canonical, index),
				ProductID:           productID,
				Scenario:            strings.TrimSpace(scenario),
				ExpectedCanonical:   canonical,
				CandidateCanonicals: append([]string(nil), candidates...),
			})
			report.PositiveAssertions++
		}
		for index, scenario := range hint.AvoidWhen {
			normalized := normalizeManualAgentSelectionScenario(scenario)
			if normalized == "" {
				return fixture, report, fmt.Errorf("agent_hints tool %s has an empty normalized avoid_when selection assertion", canonical)
			}
			if expected := positiveExpectations[normalized]; expected == canonical {
				return fixture, report, fmt.Errorf("agent_hints tool %s has the same literal positive and negative selection scenario %q", canonical, strings.TrimSpace(scenario))
			}
			fixture.Cases = append(fixture.Cases, ManualAgentSelectionCase{
				ID:                  fmt.Sprintf("%s/avoid_when/%d", canonical, index),
				ProductID:           productID,
				Scenario:            strings.TrimSpace(scenario),
				ForbiddenCanonical:  canonical,
				CandidateCanonicals: append([]string(nil), candidates...),
			})
			report.NegativeAssertions++
		}
	}

	report.Tools = len(canonicals)
	digest, err := manualAgentSelectionFixtureDigest(fixture)
	if err != nil {
		return fixture, report, err
	}
	report.FixtureSHA256 = digest
	return fixture, report, nil
}

// ValidateManualAgentSelectionContract is the lightweight generator-facing
// gate. Callers that run a semantic model evaluation should use
// BuildManualAgentSelectionEvalFixture and pass the returned cases to it.
func ValidateManualAgentSelectionContract(bound BoundCommandRegistry, hints ManualAgentHintSet) (ManualAgentSelectionReport, error) {
	_, report, err := BuildManualAgentSelectionEvalFixture(bound, hints)
	return report, err
}

func validateManualAgentSelectionBinding(bound BoundCommandRegistry, canonical string, command BoundCommandSpec) error {
	if command.CanonicalPath != canonical {
		return fmt.Errorf("agent_hints selection canonical %q resolves to mismatched BoundCommandRegistry entry %q", canonical, command.CanonicalPath)
	}
	if command.PrimaryCommand == nil {
		return fmt.Errorf("agent_hints selection canonical %q has no bound primary Cobra command", canonical)
	}
	if !runnableSchemaLeaf(command.PrimaryCommand) {
		return fmt.Errorf("agent_hints selection canonical %q primary path %q is not a runnable Cobra leaf", canonical, command.PrimaryCLIPath)
	}
	primaryPath := normalizeSchemaCLIPath(command.PrimaryCLIPath)
	if primaryPath == "" {
		return fmt.Errorf("agent_hints selection canonical %q has an empty primary CLI path", canonical)
	}
	byPath, ok := bound.ByCLIPath[primaryPath]
	if !ok || byPath.CanonicalPath != canonical {
		return fmt.Errorf("agent_hints selection canonical %q primary path %q is not bound back to the same tool", canonical, primaryPath)
	}
	return nil
}

func normalizeManualAgentSelectionScenario(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func manualAgentSelectionFixtureDigest(fixture ManualAgentSelectionFixture) (string, error) {
	data, err := marshalManualAgentSelectionFixture(fixture)
	if err != nil {
		return "", fmt.Errorf("marshal Manual Agent selection fixture: %w", err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
