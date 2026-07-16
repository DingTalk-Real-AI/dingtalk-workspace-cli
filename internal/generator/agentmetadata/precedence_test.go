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

package agentmetadata

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAgentMetadataScalarSourcePrecedenceMatrix(t *testing.T) {
	tests := []struct {
		rank       int
		precedence string
	}{
		{selectionRankReviewedManual, selectionPrecedenceReviewedManual},
		{selectionRankReviewedExplicit, selectionPrecedenceReviewedExplicit},
		{selectionRankExplicit, selectionPrecedenceExplicit},
		{selectionRankImported, selectionPrecedenceImported},
		{selectionRankUnreviewedExplicit, selectionPrecedenceUnreviewedExplicit},
		{selectionRankSkill, selectionPrecedenceSkill},
		{selectionRankMCPFallback, selectionPrecedenceMCPFallback},
		{selectionRankDefault, selectionPrecedenceDefault},
	}
	for index, test := range tests {
		if got := selectionPrecedence(test.rank); got != test.precedence {
			t.Fatalf("rank %d precedence = %q, want %q", test.rank, got, test.precedence)
		}
		if got := precedenceRank(test.precedence); got != test.rank {
			t.Fatalf("precedence %q rank = %d, want %d", test.precedence, got, test.rank)
		}
		if index > 0 && tests[index-1].rank <= test.rank {
			t.Fatalf("scalar source precedence is not strictly descending at %q > %q", tests[index-1].precedence, test.precedence)
		}
	}
}

func TestReviewedHintSourceRanksEveryAuthoredFieldWithoutPerToolDuplication(t *testing.T) {
	if got := hintSelectionRank(HintSource{Kind: "explicit", Name: "reviewed-baseline", Reviewed: true}, nil); got != selectionRankReviewedExplicit {
		t.Fatalf("reviewed source rank = %d, want %d", got, selectionRankReviewedExplicit)
	}
	unreviewed := false
	if got := hintSelectionRank(HintSource{Kind: "explicit", Reviewed: true}, &unreviewed); got != selectionRankUnreviewedExplicit {
		t.Fatalf("per-tool reviewed=false rank = %d, want %d", got, selectionRankUnreviewedExplicit)
	}
	reason := hintCandidateReviewReason(HintSource{Kind: "explicit", Name: "reviewed-baseline", Reviewed: true}, HintTool{})
	if !strings.Contains(reason, "reviewed Agent hint source") {
		t.Fatalf("source-level review reason = %q", reason)
	}
}

func TestGenerateUsesSourcePrecedenceForSelectionAndSafety(t *testing.T) {
	root := t.TempDir()
	writePrecedenceFixture(t, root, "calendar attendee update")
	writeFixture(t, root, "internal/cli/schema_hints/10-imported.json", hintFixture("imported", "imported-review", "calendar attendee update", `{
      "agent_summary": "imported summary",
      "risk": "high",
      "confirmation": "user_required"
    }`))
	writeFixture(t, root, "internal/cli/schema_hints/20-unreviewed.json", hintFixture("explicit", "generated-baseline", "calendar attendee update", `{
      "agent_summary": "unreviewed baseline summary",
      "risk": "medium",
      "confirmation": "not_required",
      "reviewed": false
    }`))
	writeFixture(t, root, "internal/cli/schema_hints/30-reviewed.json", hintFixture("explicit", "reviewed-selection", "calendar attendee update", `{
      "agent_summary": "reviewed explicit summary",
      "risk": "low",
      "confirmation": "not_required",
      "reviewed": true
    }`))

	metadata, _, err := generateFromSources(precedenceOptions(root, map[string]string{
		"calendar attendee update": "calendar attendee update",
	}))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	tool := metadata.Tools["calendar attendee update"]
	if tool.AgentSummary != "reviewed explicit summary" || tool.AgentSummarySource != "reviewed-selection" {
		t.Fatalf("selection winner = %q from %q", tool.AgentSummary, tool.AgentSummarySource)
	}
	if tool.Risk != "low" || tool.Confirmation != "not_required" {
		t.Fatalf("reviewed explicit safety did not override imported values: %s/%s", tool.Risk, tool.Confirmation)
	}
	riskProvenance := tool.FieldProvenance["risk"]
	if riskProvenance.Value != "low" || riskProvenance.Source != "internal/cli/schema_hints/30-reviewed.json" || riskProvenance.Precedence != "reviewed_explicit" || riskProvenance.Resolution != "highest_precedence" {
		t.Fatalf("risk provenance = %#v", riskProvenance)
	}
	if riskProvenance.ReviewReason == "" {
		t.Fatalf("risk provenance has no review reason: %#v", riskProvenance)
	}
	if !hasCandidate(riskProvenance.Candidates, "high", "internal/cli/schema_hints/10-imported.json", "imported", false) {
		t.Fatalf("risk provenance lost imported candidate: %#v", riskProvenance)
	}
	if !hasCandidate(riskProvenance.Candidates, "low", "internal/cli/schema_hints/30-reviewed.json", "reviewed_explicit", true) {
		t.Fatalf("risk provenance did not mark its winner: %#v", riskProvenance)
	}
	confirmationProvenance := tool.FieldProvenance["confirmation"]
	if confirmationProvenance.Source != "internal/cli/schema_hints/30-reviewed.json" || confirmationProvenance.Precedence != "reviewed_explicit" {
		t.Fatalf("confirmation provenance = %#v", confirmationProvenance)
	}
}

func TestGenerateRejectsSamePrecedenceScalarConflict(t *testing.T) {
	tests := []struct {
		name      string
		first     string
		second    string
		wantField string
	}{
		{name: "selection", first: `{"agent_summary":"first summary"}`, second: `{"agent_summary":"second summary"}`, wantField: "agent_summary"},
		{name: "selection empty versus nonempty", first: `{"agent_summary":""}`, second: `{"agent_summary":"second summary"}`, wantField: "agent_summary"},
		{name: "safety", first: `{"risk":"high"}`, second: `{"risk":"low"}`, wantField: "risk"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writePrecedenceFixture(t, root, "calendar attendee update")
			writeFixture(t, root, "internal/cli/schema_hints/a.json", hintFixture("explicit", "review-a", "calendar attendee update", test.first))
			writeFixture(t, root, "internal/cli/schema_hints/b.json", hintFixture("explicit", "review-b", "calendar attendee update", test.second))

			_, _, err := generateFromSources(precedenceOptions(root, map[string]string{
				"calendar attendee update": "calendar attendee update",
			}))
			if err == nil || !strings.Contains(err.Error(), "field "+test.wantField) || !strings.Contains(err.Error(), "a.json") || !strings.Contains(err.Error(), "b.json") {
				t.Fatalf("Generate() error = %v, want source-aware same-precedence conflict", err)
			}
		})
	}
}

func TestGenerateRejectsLowerPrecedenceConflictHiddenByHigherWinner(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{name: "lower values in path order", left: "high", right: "low"},
		{name: "lower values reverse path order", left: "low", right: "high"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writePrecedenceFixture(t, root, "calendar attendee update")
			writeFixture(t, root, "internal/cli/schema_hints/10-import-a.json", hintFixture(
				"imported", "import-a", "calendar.attendee_update_a", fmt.Sprintf(`{"risk":%q}`, test.left),
			))
			writeFixture(t, root, "internal/cli/schema_hints/20-import-b.json", hintFixture(
				"imported", "import-b", "calendar.attendee_update_b", fmt.Sprintf(`{"risk":%q}`, test.right),
			))
			writeFixture(t, root, "internal/cli/schema_hints/30-reviewed.json", `{
  "version": 1,
  "source": {"kind": "explicit", "name": "reviewed-winner"},
  "tools": {
    "calendar.attendee_update_a": {"risk": "medium", "reviewed": true},
    "calendar.attendee_update_b": {"risk": "medium", "reviewed": true}
  }
}`)

			_, _, err := generateFromSources(precedenceOptions(root, map[string]string{
				"calendar attendee update":   "calendar attendee update",
				"calendar.attendee_update_a": "calendar attendee update",
				"calendar.attendee_update_b": "calendar attendee update",
			}))
			if err == nil || !strings.Contains(err.Error(), "field risk") ||
				!strings.Contains(err.Error(), "10-import-a.json") || !strings.Contains(err.Error(), "20-import-b.json") ||
				!strings.Contains(err.Error(), "precedence 200") {
				t.Fatalf("Generate() error = %v, want masked imported-rank conflict", err)
			}
		})
	}
}

func TestGenerateRejectsSamePrecedenceInterfaceConflictAfterAliasReconciliation(t *testing.T) {
	tests := []struct {
		name      string
		first     string
		second    string
		wantField string
	}{
		{
			name:      "interface ref",
			first:     `{"interface_ref":{"product_id":"calendar","rpc_name":"update_attendee"}}`,
			second:    `{"interface_ref":{"product_id":"calendar","rpc_name":"replace_attendee"}}`,
			wantField: "interface_ref",
		},
		{
			name:      "interface disposition",
			first:     `{"interface_mode":"local"}`,
			second:    `{"interface_mode":"mcp"}`,
			wantField: "interface_mode",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writePrecedenceFixture(t, root, "calendar attendee update")
			writeFixture(t, root, "internal/cli/schema_hints/aliases.json", fmt.Sprintf(`{
  "version": 1,
  "source": {"kind": "explicit", "name": "alias-review"},
  "tools": {
    "calendar.attendee_update_old": %s,
    "calendar.attendee_update_new": %s
  }
}`, test.first, test.second))

			_, _, err := generateFromSources(precedenceOptions(root, map[string]string{
				"calendar attendee update":     "calendar attendee update",
				"calendar.attendee_update_old": "calendar attendee update",
				"calendar.attendee_update_new": "calendar attendee update",
			}))
			if err == nil || !strings.Contains(err.Error(), "field "+test.wantField) || !strings.Contains(err.Error(), "calendar attendee update") {
				t.Fatalf("Generate() error = %v, want %s alias conflict", err, test.wantField)
			}
		})
	}
}

func TestReviewedLocalDispositionClearsLowerPrecedenceMCPRef(t *testing.T) {
	root := t.TempDir()
	writePrecedenceFixture(t, root, "calendar attendee update")
	writeFixture(t, root, "internal/cli/schema_hints/10-imported.json", hintFixture("imported", "imported-interface", "calendar attendee update", `{
      "interface_ref": {"product_id": "calendar", "rpc_name": "update_attendee"},
      "interface_mode": "mcp",
      "availability": "available"
    }`))
	writeFixture(t, root, "internal/cli/schema_hints/20-reviewed.json", hintFixture("explicit", "reviewed-interface", "calendar attendee update", `{
      "interface_mode": "local",
      "interface_reason": "The CLI wrapper is executable but has no pinned MCP operation.",
      "availability": "available",
      "reviewed": true
    }`))

	metadata, _, err := generateFromSources(precedenceOptions(root, map[string]string{
		"calendar attendee update": "calendar attendee update",
	}))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	tool := metadata.Tools["calendar attendee update"]
	if tool.InterfaceMode != "local" || tool.InterfaceRef != nil {
		t.Fatalf("interface = mode %q ref %#v, want reviewed local without direct ref", tool.InterfaceMode, tool.InterfaceRef)
	}
	refProvenance, exists := tool.FieldProvenance["interface_ref"]
	if !exists || refProvenance.Value != nil || refProvenance.Resolution != "interface_disposition_matrix" || len(refProvenance.OverriddenCandidates) == 0 {
		t.Fatalf("interface_ref disposition provenance = %#v", refProvenance)
	}
	provenance := tool.FieldProvenance["interface_mode"]
	if provenance.Value != "local" || provenance.Source != "internal/cli/schema_hints/20-reviewed.json" || provenance.Precedence != "reviewed_explicit" {
		t.Fatalf("interface_mode provenance = %#v", provenance)
	}
}

func TestInterfaceDispositionResolutionSurvivesAliasReconciliation(t *testing.T) {
	root := t.TempDir()
	writePrecedenceFixture(t, root, "calendar attendee update")
	writeFixture(t, root, "internal/cli/schema_hints/10-imported.json", hintFixture("imported", "legacy-local", "calendar.update_legacy", `{
      "interface_ref": {"product_id": "calendar", "rpc_name": "update_attendee"},
      "interface_mode": "local",
      "availability": "available",
      "interface_reason": "legacy wrapper"
    }`))
	writeFixture(t, root, "internal/cli/schema_hints/20-reviewed.json", hintFixture("explicit", "reviewed-mcp", "calendar.update_current", `{
      "interface_mode": "mcp",
      "availability": "available",
      "reviewed": true
    }`))

	metadata, _, err := generateFromSources(precedenceOptions(root, map[string]string{
		"calendar attendee update": "calendar attendee update",
		"calendar.update_legacy":   "calendar attendee update",
		"calendar.update_current":  "calendar attendee update",
	}))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	tool := metadata.Tools["calendar attendee update"]
	if tool.InterfaceMode != "mcp" || tool.InterfaceRef == nil || tool.InterfaceRef.ProductID != "calendar" || tool.InterfaceRef.RPCName != "update_attendee" {
		t.Fatalf("reconciled interface = mode %q ref %#v", tool.InterfaceMode, tool.InterfaceRef)
	}
	if provenance := tool.FieldProvenance["interface_ref"]; provenance.Value != "calendar.update_attendee" || provenance.Precedence != "imported" {
		t.Fatalf("reconciled ref provenance = %#v", provenance)
	}
}

func TestSafetyPrecedenceAllowsExplicitDowngrade(t *testing.T) {
	tests := []struct {
		name        string
		left        ToolMetadata
		right       ToolMetadata
		wantRisk    string
		wantConfirm string
	}{
		{
			name: "reviewed explicit low overrides imported high",
			left: ToolMetadata{
				Risk:               "high",
				riskRank:           selectionRankImported,
				riskOrigin:         "imported.json",
				Confirmation:       "user_required",
				confirmationRank:   selectionRankImported,
				confirmationOrigin: "imported.json",
			},
			right: ToolMetadata{
				Risk:               "low",
				riskRank:           selectionRankReviewedExplicit,
				riskOrigin:         "reviewed.json",
				Confirmation:       "not_required",
				confirmationRank:   selectionRankReviewedExplicit,
				confirmationOrigin: "reviewed.json",
			},
			wantRisk:    "low",
			wantConfirm: "not_required",
		},
		{
			name: "explicit low overrides imported high",
			left: ToolMetadata{
				Risk:               "high",
				riskRank:           selectionRankImported,
				riskOrigin:         "imported.json",
				Confirmation:       "user_required",
				confirmationRank:   selectionRankImported,
				confirmationOrigin: "imported.json",
			},
			right: ToolMetadata{
				Risk:               "low",
				riskRank:           selectionRankExplicit,
				riskOrigin:         "explicit.json",
				Confirmation:       "not_required",
				confirmationRank:   selectionRankExplicit,
				confirmationOrigin: "explicit.json",
			},
			wantRisk:    "low",
			wantConfirm: "not_required",
		},
		{
			name: "explicit high remains above imported low",
			left: ToolMetadata{
				Risk:               "high",
				riskRank:           selectionRankExplicit,
				riskOrigin:         "explicit.json",
				Confirmation:       "user_required",
				confirmationRank:   selectionRankExplicit,
				confirmationOrigin: "explicit.json",
			},
			right: ToolMetadata{
				Risk:               "low",
				riskRank:           selectionRankImported,
				riskOrigin:         "imported.json",
				Confirmation:       "not_required",
				confirmationRank:   selectionRankImported,
				confirmationOrigin: "imported.json",
			},
			wantRisk:    "high",
			wantConfirm: "user_required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merged, err := mergeToolMetadata(test.left, test.right, "calendar attendee update")
			if err != nil {
				t.Fatalf("mergeToolMetadata() error = %v", err)
			}
			if merged.Risk != test.wantRisk || merged.Confirmation != test.wantConfirm {
				t.Fatalf("safety = %s/%s, want %s/%s", merged.Risk, merged.Confirmation, test.wantRisk, test.wantConfirm)
			}
		})
	}
}

func TestReviewedAuthoredEmptyScalarsOverrideLowerValues(t *testing.T) {
	root := t.TempDir()
	writePrecedenceFixture(t, root, "calendar attendee update")
	writeFixture(t, root, "internal/cli/schema_hints/10-imported.json", hintFixture("imported", "legacy", "calendar attendee update", `{
      "agent_summary": "legacy summary",
      "effect": "write",
      "risk": "high",
      "confirmation": "user_required",
      "idempotency": "unknown",
      "interface_mode": "local",
      "availability": "available",
      "interface_reason": "legacy wrapper"
    }`))
	writeFixture(t, root, "internal/cli/schema_hints/20-reviewed.json", hintFixture("explicit", "reviewed-empty", "calendar attendee update", `{
      "agent_summary": "",
      "effect": "",
      "risk": "",
      "confirmation": "",
      "idempotency": "",
      "interface_reason": "",
      "reviewed": true,
      "review_reason": "reviewed clearing of stale scalar metadata"
    }`))

	metadata, _, err := generateFromSources(precedenceOptions(root, map[string]string{
		"calendar attendee update": "calendar attendee update",
	}))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	tool := metadata.Tools["calendar attendee update"]
	values := map[string]string{
		"agent_summary":    tool.AgentSummary,
		"effect":           tool.Effect,
		"risk":             tool.Risk,
		"confirmation":     tool.Confirmation,
		"idempotency":      tool.Idempotency,
		"interface_reason": tool.InterfaceReason,
	}
	for field, value := range values {
		if value != "" {
			t.Errorf("%s = %q, want authored empty", field, value)
		}
		provenance := tool.FieldProvenance[field]
		if provenance.Value != "" || provenance.Source != "internal/cli/schema_hints/20-reviewed.json" || provenance.Precedence != "reviewed_explicit" {
			t.Errorf("%s provenance = %#v", field, provenance)
			continue
		}
		selected := 0
		for _, candidate := range provenance.Candidates {
			if candidate.Selected {
				selected++
				if candidate.Value != "" {
					t.Errorf("%s selected candidate = %#v", field, candidate)
				}
			}
		}
		if selected != 1 {
			t.Errorf("%s selected candidates = %d, provenance %#v", field, selected, provenance)
		}
	}
}

func TestHintToolPreservesExplicitEmptyAndNullPresence(t *testing.T) {
	var hint HintTool
	if err := json.Unmarshal([]byte(`{
  "agent_summary": "",
  "effect": "",
  "risk": "",
  "confirmation": "",
  "idempotency": "",
  "interface_ref": null,
  "interface_mode": "",
  "availability": "",
  "interface_reason": ""
}`), &hint); err != nil {
		t.Fatal(err)
	}
	if !hint.agentSummaryPresent || !hint.effectPresent || !hint.riskPresent ||
		!hint.confirmationPresent || !hint.idempotencyPresent || !hint.interfaceRefPresent ||
		!hint.interfaceModePresent || !hint.availabilityPresent || !hint.interfaceReasonPresent {
		t.Fatalf("decoded hint lost authored presence: %#v", hint)
	}

	left := ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "calendar", RPCName: "update_attendee"},
		interfaceRefPresent: true,
		interfaceRefRank:    selectionRankImported,
		interfaceRefOrigin:  "imported.json",
	}
	right := ToolMetadata{
		interfaceRefPresent: true,
		interfaceRefRank:    selectionRankReviewedExplicit,
		interfaceRefOrigin:  "reviewed.json",
	}
	merged, err := mergeToolMetadata(left, right, "calendar attendee update")
	if err != nil {
		t.Fatalf("mergeToolMetadata() error = %v", err)
	}
	if merged.InterfaceRef != nil || !merged.interfaceRefPresent {
		t.Fatalf("merged interface ref = %#v present=%v, want authored null", merged.InterfaceRef, merged.interfaceRefPresent)
	}
	provenance := merged.FieldProvenance["interface_ref"]
	if provenance.Value != nil || provenance.Source != "reviewed.json" || provenance.Precedence != "reviewed_explicit" {
		t.Fatalf("null interface_ref provenance = %#v", provenance)
	}
}

func hasCandidate(candidates []FieldCandidateProvenance, value, source, precedence string, selected bool) bool {
	for _, candidate := range candidates {
		if candidate.Value == value && candidate.Source == source && candidate.Precedence == precedence && candidate.Selected == selected {
			return true
		}
	}
	return false
}

func writePrecedenceFixture(t *testing.T, root, commandPath string) {
	t.Helper()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/calendar.md", "# Calendar\n\n```bash\ndws "+commandPath+" --id example\n```\n")
}

func precedenceOptions(root string, toolPaths map[string]string) Options {
	return Options{
		Root:             root,
		SkillPath:        "skills/mono/SKILL.md",
		ProductsDir:      "skills/mono/references/products",
		IntentGuidePath:  "skills/mono/references/intent-guide.md",
		HintsDir:         "internal/cli/schema_hints",
		ToolPaths:        toolPaths,
		ProductIDs:       map[string]bool{"calendar": true},
		SurfaceToolCount: 1,
	}
}

func hintFixture(kind, name, path, body string) string {
	return fmt.Sprintf(`{
  "version": 1,
  "source": {"kind": %q, "name": %q},
  "tools": {%q: %s}
}`, kind, name, path, body)
}
