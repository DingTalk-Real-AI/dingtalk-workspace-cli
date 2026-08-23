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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

var marshalAgentSelectionFixture = json.Marshal

const agentSelectionFixtureVersion = 1

// AgentSelectionCase is one reproducible model-evaluation assertion derived
// from ContractFinal Selection prose. Positive cases require one exact
// canonical result; negative cases only forbid the command that owns the
// avoid_when text. CandidateCanonicals contains every bound tool in the same
// product, in stable order.
type AgentSelectionCase struct {
	ID                  string   `json:"id"`
	ProductID           string   `json:"product_id"`
	Scenario            string   `json:"scenario"`
	ExpectedCanonical   string   `json:"expected_canonical,omitempty"`
	ForbiddenCanonical  string   `json:"forbidden_canonical,omitempty"`
	CandidateCanonicals []string `json:"candidate_canonicals"`
}

// AgentSelectionFixture is the stable input to an optional live Agent
// evaluation. It is built from contract.ProductDecl/ContractFinal selection prose and
// the real bound command tree; it is not a second authored hint source.
type AgentSelectionFixture struct {
	Version int                  `json:"version"`
	Cases   []AgentSelectionCase `json:"cases"`
}

// AgentSelectionReport records deterministic coverage and the exact fixture
// digest. It proves that all reviewed assertions are well-formed and
// executable; it deliberately does not claim that a language model understood
// their natural-language meaning.
type AgentSelectionReport struct {
	Tools              int
	PositiveAssertions int
	NegativeAssertions int
	FixtureSHA256      string
}

// BuildAgentSelectionEvalFixture turns every ContractFinal use_when and
// avoid_when entry into a typed, reproducible evaluation case and validates it
// against the exact BoundCommandRegistry.
func BuildAgentSelectionEvalFixture(bound BoundCommandRegistry) (AgentSelectionFixture, AgentSelectionReport, error) {
	fixture := AgentSelectionFixture{Version: agentSelectionFixtureVersion}
	report := AgentSelectionReport{}
	expectedTools := make(map[string]bool, len(bound.Commands))
	candidatesByProduct := map[string][]string{}
	for _, command := range bound.Commands {
		canonical := strings.TrimSpace(command.CanonicalPath)
		expectedTools[canonical] = true
		if !contractfinal.HasRuntimeContractFinal(command.PrimaryCommand) {
			return fixture, report, fmt.Errorf("agent selection expected canonical %q has no ContractFinal declaration", canonical)
		}
		productID, _, ok := strings.Cut(canonical, ".")
		if !ok || strings.TrimSpace(productID) == "" {
			return fixture, report, fmt.Errorf("agent selection has invalid bound canonical path %q", canonical)
		}
		candidatesByProduct[productID] = append(candidatesByProduct[productID], canonical)
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
			return fixture, report, fmt.Errorf("agent selection expected canonical %q is missing from BoundCommandRegistry.ByCanonical", canonical)
		}
		if err := validateAgentSelectionBinding(bound, canonical, command); err != nil {
			return fixture, report, err
		}

		selection := contractFinalToolSelection(command.PrimaryCommand)
		if len(selection.UseWhen) == 0 {
			return fixture, report, fmt.Errorf("ContractFinal tool %s requires at least one positive use_when selection assertion", canonical)
		}
		if len(selection.AvoidWhen) == 0 {
			return fixture, report, fmt.Errorf("ContractFinal tool %s requires at least one negative avoid_when selection assertion", canonical)
		}
		productID, _, _ := strings.Cut(canonical, ".")
		candidates := candidatesByProduct[productID]
		for index, scenario := range selection.UseWhen {
			normalized := normalizeAgentSelectionScenario(scenario)
			if normalized == "" {
				return fixture, report, fmt.Errorf("ContractFinal tool %s has an empty normalized use_when selection assertion", canonical)
			}
			if previous, exists := positiveExpectations[normalized]; exists {
				return fixture, report, fmt.Errorf("use_when scenario %q has conflicting literal expectations %q and %q", positiveDisplays[normalized], previous, canonical)
			}
			positiveExpectations[normalized] = canonical
			positiveDisplays[normalized] = strings.TrimSpace(scenario)
			fixture.Cases = append(fixture.Cases, AgentSelectionCase{
				ID:                  fmt.Sprintf("%s/use_when/%d", canonical, index),
				ProductID:           productID,
				Scenario:            strings.TrimSpace(scenario),
				ExpectedCanonical:   canonical,
				CandidateCanonicals: append([]string(nil), candidates...),
			})
			report.PositiveAssertions++
		}
		for index, scenario := range selection.AvoidWhen {
			normalized := normalizeAgentSelectionScenario(scenario)
			if normalized == "" {
				return fixture, report, fmt.Errorf("ContractFinal tool %s has an empty normalized avoid_when selection assertion", canonical)
			}
			if expected := positiveExpectations[normalized]; expected == canonical {
				return fixture, report, fmt.Errorf("ContractFinal tool %s has the same literal positive and negative selection scenario %q", canonical, strings.TrimSpace(scenario))
			}
			fixture.Cases = append(fixture.Cases, AgentSelectionCase{
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
	digest, err := agentSelectionFixtureDigest(fixture)
	if err != nil {
		return fixture, report, err
	}
	report.FixtureSHA256 = digest
	return fixture, report, nil
}

// ValidateAgentSelectionContract is the lightweight generator-facing gate.
func ValidateAgentSelectionContract(bound BoundCommandRegistry) (AgentSelectionReport, error) {
	_, report, err := BuildAgentSelectionEvalFixture(bound)
	return report, err
}

// contractFinalToolSelection synthesizes selection assertions of a declared
// tool from its ContractFinal declaration.
func contractFinalToolSelection(command *cobra.Command) AgentToolSelection {
	out := AgentToolSelection{Reviewed: true, Revision: "contract", Reason: "Contract final declaration (corecmd.ContractDecl)"}
	payload, ok := contractfinal.RuntimeContractFinal(command)
	if !ok || payload.Selection == nil {
		return out
	}
	selection := payload.Selection
	out.AgentSummary = selection.AgentSummary
	out.UseWhen = selection.UseWhen
	out.AvoidWhen = selection.AvoidWhen
	out.Examples = selection.Examples
	out.ExampleDispositions = selection.ExampleDispositions
	return out
}

func validateAgentSelectionBinding(bound BoundCommandRegistry, canonical string, command BoundCommandSpec) error {
	if command.CanonicalPath != canonical {
		return fmt.Errorf("agent selection canonical %q resolves to mismatched BoundCommandRegistry entry %q", canonical, command.CanonicalPath)
	}
	if command.PrimaryCommand == nil {
		return fmt.Errorf("agent selection canonical %q has no bound primary Cobra command", canonical)
	}
	if !runnableSchemaLeaf(command.PrimaryCommand) {
		return fmt.Errorf("agent selection canonical %q primary path %q is not a runnable Cobra leaf", canonical, command.PrimaryCLIPath)
	}
	primaryPath := normalizeSchemaCLIPath(command.PrimaryCLIPath)
	if primaryPath == "" {
		return fmt.Errorf("agent selection canonical %q has an empty primary CLI path", canonical)
	}
	byPath, ok := bound.ByCLIPath[primaryPath]
	if !ok || byPath.CanonicalPath != canonical {
		return fmt.Errorf("agent selection canonical %q primary path %q is not bound back to the same tool", canonical, primaryPath)
	}
	return nil
}

func normalizeAgentSelectionScenario(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func agentSelectionFixtureDigest(fixture AgentSelectionFixture) (string, error) {
	data, err := marshalAgentSelectionFixture(fixture)
	if err != nil {
		return "", fmt.Errorf("marshal Agent selection fixture: %w", err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
