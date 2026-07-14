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
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEmbeddedAgentMetadataLoadsSplitDomains(t *testing.T) {
	metadata := loadEmbeddedAgentMetadata()
	if len(metadata.Domains) < 2 {
		t.Fatalf("domains = %#v, want split product metadata", metadata.Domains)
	}
	if len(metadata.Tools) != metadata.Coverage.ToolsWithMetadata {
		t.Fatalf("tools = %d, coverage = %#v", len(metadata.Tools), metadata.Coverage)
	}
	if _, ok := metadata.Tools["calendar event create"]; !ok {
		t.Fatalf("calendar domain did not load: %#v", metadata.Domains)
	}
	coverage := metadata.Coverage
	if coverage.ToolsWithUseWhen != len(metadata.Tools) ||
		coverage.ToolsWithAvoidWhen != len(metadata.Tools) ||
		coverage.ToolsWithExamples != len(metadata.Tools) ||
		coverage.ToolsWithInterfaceMode != len(metadata.Tools) {
		t.Fatalf("selection metadata coverage = %#v, tools=%d", coverage, len(metadata.Tools))
	}
	for path, tool := range metadata.Tools {
		if len(tool.UseWhen) == 0 || len(tool.AvoidWhen) == 0 || len(tool.Examples) == 0 {
			t.Errorf("tool %s has incomplete selection metadata: %#v", path, tool)
		}
		if tool.InterfaceMode == "" || tool.Availability == "" {
			t.Errorf("tool %s has incomplete interface disposition: %#v", path, tool)
		}
		for _, example := range tool.Examples {
			if strings.Contains(" "+example+" ", " --yes ") {
				t.Errorf("tool %s example bypasses confirmation: %q", path, example)
			}
		}
	}
}

func TestAgentMetadataTypedAccessorRoundTripsProvenance(t *testing.T) {
	const encoded = `{
  "product_id": "calendar",
  "tools": {
    "calendar attendee update": {
      "agent_summary": "Update one attendee",
      "agent_summary_source": "reviewed-selection",
      "use_when": ["change an attendee"],
      "avoid_when": ["read attendees"],
      "prerequisites": ["event id"],
      "tips": ["verify the attendee id"],
      "effect": "write",
      "effect_source": "agent-hint",
      "risk": "low",
      "confirmation": "not_required",
      "idempotency": "non_idempotent",
      "workflow_refs": ["calendar-update"],
      "examples": ["dws calendar attendee update --event-id e1"],
      "reviewed": true,
      "source_refs": ["internal/cli/schema_hints/calendar.json"],
      "interface_ref": {"product_id": "calendar", "rpc_name": "update_attendee"},
      "interface_mode": "mcp",
      "availability": "available",
      "interface_reason": "reviewed RPC mapping",
      "field_provenance": {
        "risk": {
          "value": "low",
          "source": "reviewed.json",
          "precedence": "reviewed_explicit",
          "resolution": "highest_precedence",
          "review_reason": "reviewed downgrade",
          "candidates": [
            {"value": "low", "source": "reviewed.json", "precedence": "reviewed_explicit", "review_reason": "reviewed downgrade", "selected": true},
            {"value": "high", "source": "imported.json", "precedence": "imported", "selected": false}
          ],
          "overridden_candidates": [
            {"value": "medium", "source": "generated-default", "precedence": "inference_or_default", "selected": false}
          ]
        }
      }
    }
  }
}`
	var fragment embeddedAgentMetadataDomain
	if err := json.Unmarshal([]byte(encoded), &fragment); err != nil {
		t.Fatalf("decode generated Agent metadata: %v", err)
	}
	roundTrip, err := json.Marshal(fragment)
	if err != nil {
		t.Fatalf("encode generated Agent metadata: %v", err)
	}
	var decoded embeddedAgentMetadataDomain
	if err := json.Unmarshal(roundTrip, &decoded); err != nil {
		t.Fatalf("round-trip generated Agent metadata: %v", err)
	}

	metadataFixture := embeddedAgentMetadata{
		Products: map[string]agentProductMetadata{},
		Tools:    decoded.Tools,
	}

	safety, interfaceSpec, selection, provenance, ok := agentToolContractForPathsFromMetadata(metadataFixture, "missing", " calendar   attendee update ")
	if !ok {
		t.Fatal("typed Agent metadata lookup failed")
	}
	if safety != (SafetySpec{Effect: "write", EffectSource: "agent-hint", Risk: "low", Confirmation: "not_required", Idempotency: "non_idempotent"}) {
		t.Fatalf("safety = %#v", safety)
	}
	if interfaceSpec.Ref == nil || interfaceSpec.Ref.ProductID != "calendar" || interfaceSpec.Ref.RPCName != "update_attendee" || interfaceSpec.Mode != "mcp" || interfaceSpec.Availability != "available" || interfaceSpec.Reason != "reviewed RPC mapping" {
		t.Fatalf("interface = %#v", interfaceSpec)
	}
	if selection.AgentSummary != "Update one attendee" || selection.MetadataSource != embeddedAgentMetadataSource || selection.Reviewed == nil || !*selection.Reviewed || len(selection.Examples) != 1 {
		t.Fatalf("selection = %#v", selection)
	}
	risk := provenance["risk"]
	if string(risk.Value) != `"low"` || risk.Source != "reviewed.json" || risk.Precedence != "reviewed_explicit" || risk.Resolution != "highest_precedence" || risk.ReviewReason != "reviewed downgrade" {
		t.Fatalf("risk provenance = %#v", risk)
	}
	if len(risk.Candidates) != 2 || risk.Candidates[0].Selected == nil || !*risk.Candidates[0].Selected || risk.Candidates[1].Selected == nil || *risk.Candidates[1].Selected {
		t.Fatalf("risk candidates = %#v", risk.Candidates)
	}
	if string(risk.Candidates[1].Value) != `"high"` || risk.Candidates[1].Source != "imported.json" {
		t.Fatalf("overridden risk candidate = %#v", risk.Candidates[1])
	}
	if len(risk.OverriddenCandidates) != 1 || string(risk.OverriddenCandidates[0].Value) != `"medium"` || risk.OverriddenCandidates[0].Precedence != "inference_or_default" {
		t.Fatalf("legacy overridden candidates were dropped: %#v", risk.OverriddenCandidates)
	}

	// Accessors return detached typed values; callers cannot mutate the
	// embedded snapshot and accidentally change a later schema response.
	interfaceSpec.Ref.ProductID = "mutated"
	selection.UseWhen[0] = "mutated"
	risk.Candidates[0].Value[0] = 'x'
	provenance["risk"] = risk
	_, interfaceAgain, selectionAgain, provenanceAgain, _ := agentToolContractForPathsFromMetadata(metadataFixture, "calendar attendee update")
	if interfaceAgain.Ref.ProductID != "calendar" || selectionAgain.UseWhen[0] != "change an attendee" || string(provenanceAgain["risk"].Candidates[0].Value) != `"low"` {
		t.Fatalf("typed accessor leaked mutable state: interface=%#v selection=%#v provenance=%#v", interfaceAgain, selectionAgain, provenanceAgain)
	}

}

func TestAgentMetadataTypedAdapterProjectsInterfaceProvenance(t *testing.T) {
	selected := true
	legacyRef := func(value string) FieldProvenance {
		raw, _ := json.Marshal(value)
		return FieldProvenance{
			Value:      raw,
			Source:     "agent-metadata.json",
			Precedence: "explicit",
			Resolution: "highest_precedence",
			Candidates: []FieldCandidateProvenance{{
				Value:      append(json.RawMessage(nil), raw...),
				Source:     "agent-metadata.json",
				Precedence: "explicit",
				Selected:   &selected,
			}},
		}
	}
	mode := resolvedFieldProvenance("local", "reviewed.json", "", "reviewed_explicit", "highest_precedence", "reviewed local wrapper")

	metadataFixture := embeddedAgentMetadata{
		Products: map[string]agentProductMetadata{},
		Tools: map[string]agentToolMetadata{
			"calendar event get": {
				InterfaceRef:  &embeddedMCPInterfaceRef{ProductID: "calendar", RPCName: "get_event"},
				InterfaceMode: "mcp",
				Availability:  "available",
				FieldProvenance: map[string]FieldProvenance{
					"interface_ref": legacyRef("calendar.get_event"),
				},
			},
			"calendar helper run": {
				InterfaceMode:   "local",
				Availability:    "available",
				InterfaceReason: "reviewed local wrapper",
				FieldProvenance: map[string]FieldProvenance{
					"interface_ref":  legacyRef("<none>"),
					"interface_mode": mode,
				},
			},
			"calendar helper inspect": {
				InterfaceMode: "local",
				Availability:  "available",
				FieldProvenance: map[string]FieldProvenance{
					"interface_mode": mode,
				},
			},
		},
	}

	_, mcpInterface, _, mcpProvenance, ok := agentToolContractForPathsFromMetadata(metadataFixture, "calendar event get")
	if !ok {
		t.Fatal("mcp metadata lookup failed")
	}
	wantRef := `{"product_id":"calendar","rpc_name":"get_event"}`
	if got := string(mcpProvenance["interface_ref"].Value); got != wantRef {
		t.Fatalf("typed interface_ref winner = %s, want %s", got, wantRef)
	}
	if got := string(mcpProvenance["interface_ref"].Candidates[0].Value); got != wantRef {
		t.Fatalf("typed interface_ref candidate = %s, want %s", got, wantRef)
	}
	if err := validateFinalFieldProvenance("calendar.event_get", "interface_ref", mcpProvenance["interface_ref"], mcpInterface.Ref); err != nil {
		t.Fatalf("typed mcp provenance = %v", err)
	}
	for _, field := range []string{"interface_reason", "agent_summary"} {
		if _, exists := mcpProvenance[field]; exists {
			t.Fatalf("typed adapter invented absent %s provenance: %#v", field, mcpProvenance[field])
		}
	}

	_, localInterface, _, provenance, ok := agentToolContractForPathsFromMetadata(metadataFixture, "calendar helper run")
	if !ok {
		t.Fatal("calendar helper run metadata lookup failed")
	}
	if got := string(provenance["interface_ref"].Value); got != "null" {
		t.Fatalf("calendar helper run interface_ref winner = %s, want null", got)
	}
	if err := validateFinalFieldProvenance("calendar helper run", "interface_ref", provenance["interface_ref"], localInterface.Ref); err != nil {
		t.Fatalf("calendar helper run typed null provenance = %v", err)
	}

	_, _, _, provenance, ok = agentToolContractForPathsFromMetadata(metadataFixture, "calendar helper inspect")
	if !ok {
		t.Fatal("calendar helper inspect metadata lookup failed")
	}
	if _, exists := provenance["interface_ref"]; exists {
		t.Fatalf("typed adapter repaired missing interface_ref provenance: %#v", provenance["interface_ref"])
	}
}

func TestAgentMetadataTypedAdapterDoesNotLaunderInterfaceConflict(t *testing.T) {
	selected := true
	wrong, _ := json.Marshal("calendar.wrong_rpc")
	provenance := FieldProvenance{
		Value:      wrong,
		Source:     "bad.json",
		Precedence: "explicit",
		Resolution: "highest_precedence",
		Candidates: []FieldCandidateProvenance{{
			Value: wrong, Source: "bad.json", Precedence: "explicit", Selected: &selected,
		}},
	}
	projected := projectAgentInterfaceRefProvenance(provenance, &InterfaceRefSpec{ProductID: "calendar", RPCName: "get_event"})
	if string(projected.Value) != string(wrong) {
		t.Fatalf("conflicting winner was rewritten: %s", projected.Value)
	}
	if err := validateFinalFieldProvenance("calendar.event_get", "interface_ref", projected, &InterfaceRefSpec{ProductID: "calendar", RPCName: "get_event"}); err == nil {
		t.Fatal("conflicting interface_ref provenance unexpectedly validated")
	}
}

func TestAgentProductSelectionUsesTypedAccessor(t *testing.T) {
	provenance := resolvedFieldProvenance("Document operations", "manual", "manual.json", "reviewed_manual", "highest_precedence", "reviewed")
	metadataFixture := embeddedAgentMetadata{
		Products: map[string]agentProductMetadata{
			"doc": {
				AgentSummary:       "Document operations",
				AgentSummarySource: "reviewed-doc-routing",
				UseWhen:            []string{"create a document", "create a document"},
				AvoidWhen:          []string{"manage a spreadsheet"},
				SourceRefs:         []string{"z.md", "a.md"},
				FieldProvenance:    map[string]FieldProvenance{"agent_summary": provenance},
			},
		},
		Tools: map[string]agentToolMetadata{},
	}

	selection, ok := agentProductSelectionForIDsFromMetadata(metadataFixture, "missing", " doc ")
	if !ok || selection.AgentSummary != "Document operations" || selection.AgentSummarySource != "reviewed-doc-routing" || selection.MetadataSource != embeddedAgentMetadataSource {
		t.Fatalf("product selection = %#v, ok=%v", selection, ok)
	}
	if len(selection.UseWhen) != 1 || len(selection.SourceRefs) != 2 || selection.SourceRefs[0] != "a.md" {
		t.Fatalf("normalized product selection = %#v", selection)
	}
	_, deliveredProvenance, ok := agentProductContractForIDsFromMetadata(metadataFixture, "doc")
	if !ok || string(deliveredProvenance["agent_summary"].Value) != `"Document operations"` {
		t.Fatalf("product provenance = %#v, ok=%v", deliveredProvenance, ok)
	}
	selection.UseWhen[0] = "mutated"
	deliveredProvenance["agent_summary"] = FieldProvenance{}
	again, _ := agentProductSelectionForIDsFromMetadata(metadataFixture, "doc")
	if again.UseWhen[0] != "create a document" {
		t.Fatalf("product accessor leaked mutable state: %#v", again)
	}
	_, againProvenance, _ := agentProductContractForIDsFromMetadata(metadataFixture, "doc")
	if len(againProvenance["agent_summary"].Value) == 0 {
		t.Fatal("product accessor leaked mutable provenance state")
	}

}

func TestRuntimeSchemaIncludesEmbeddedAgentMetadata(t *testing.T) {
	agentFixture := embeddedAgentMetadata{
		Version:    1,
		SourceHash: "sha256:test",
		Products: map[string]agentProductMetadata{
			"doc": {
				AgentSummary:       "创建、读取和维护钉钉文档",
				AgentSummarySource: "test-source",
				UseWhen:            []string{"需要创建或读取文档"},
				SourceRefs:         []string{"skills/mono/SKILL.md"},
			},
		},
		Tools: map[string]agentToolMetadata{
			"doc create": {
				UseWhen:         []string{"新建文档"},
				AvoidWhen:       []string{"只需读取文档时"},
				Effect:          "write",
				EffectSource:    "command-verb",
				Examples:        []string{"dws doc create --title test"},
				SourceRefs:      []string{"skills/mono/references/products/doc.md"},
				InterfaceMode:   "local",
				Availability:    "available",
				InterfaceReason: "test local implementation",
			},
		},
	}
	mcpFixture := embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}}

	root := buildRuntimeSchemaTestRoot()
	leaf, err := runtimeSchemaPayloadForTestWithMetadata(root, []string{"doc.create_document"}, agentFixture, mcpFixture)
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest(leaf): %v", err)
	}
	if leaf["effect"] != "write" || leaf["agent_metadata_source"] != embeddedAgentMetadataSource {
		t.Fatalf("leaf Agent metadata = %#v", leaf)
	}
	if leaf["interface_mode"] != "local" || leaf["availability"] != "available" || leaf["interface_reason"] != "test local implementation" {
		t.Fatalf("leaf interface disposition = %#v", leaf)
	}
	if examples, _ := leaf["examples"].([]string); len(examples) != 1 {
		t.Fatalf("leaf examples = %#v", leaf["examples"])
	}

	catalog, err := runtimeSchemaPayloadForTestWithMetadata(root, nil, agentFixture, mcpFixture)
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest(catalog): %v", err)
	}
	summary, _ := catalog["agent_metadata"].(map[string]any)
	if summary["source_hash"] != "sha256:test" {
		t.Fatalf("catalog Agent metadata summary = %#v", summary)
	}
	products, _ := catalog["products"].([]map[string]any)
	doc := findSchemaProduct(products, "doc")
	if useWhen, _ := doc["use_when"].([]string); len(useWhen) != 1 {
		t.Fatalf("doc product use_when = %#v", doc["use_when"])
	}
	tools, _ := doc["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["effect"] != "write" {
		t.Fatalf("doc tool summaries = %#v", tools)
	}
	if _, exists := tools[0]["examples"]; exists {
		t.Fatalf("product summary must not include examples: %#v", tools[0])
	}

	registry, err := schemaRegistryForTestWithMetadata(root, agentFixture, mcpFixture)
	if err != nil {
		t.Fatalf("schemaRegistryForTest(): %v", err)
	}
	compact, err := registry.ToOverviewPayload()
	if err != nil {
		t.Fatalf("ToOverviewPayload(): %v", err)
	}
	compactProducts, _ := compact["products"].([]map[string]any)
	compactDoc := findSchemaProduct(compactProducts, "doc")
	if compactDoc["agent_summary"] != "创建、读取和维护钉钉文档" {
		t.Fatalf("compact product summary = %#v", compactDoc)
	}
	if _, exists := compactDoc["agent_source_refs"]; exists {
		t.Fatalf("compact product must omit provenance: %#v", compactDoc)
	}
	if _, exists := compactDoc["use_when"]; exists {
		t.Fatalf("compact product with summary must omit routing expansion: %#v", compactDoc)
	}
}

func TestRuntimeSchemaAllPayloadContainsFullLeafParameters(t *testing.T) {
	payload, err := runtimeSchemaAllPayloadForTest(buildRuntimeSchemaTestRoot())
	if err != nil {
		t.Fatal(err)
	}
	products := schemaMapSlice(payload["products"])
	doc := findSchemaProduct(products, "doc")
	tools := schemaMapSlice(doc["tools"])
	if len(tools) != 1 {
		t.Fatalf("runtime full export tools = %#v", tools)
	}
	if got := schemaString(tools[0]["canonical_path"]); got != "doc.create_document" {
		t.Fatalf("canonical path = %q", got)
	}
	parameters, ok := tools[0]["parameters"].(map[string]any)
	if !ok || parameters["title"] == nil {
		t.Fatalf("runtime full export parameters = %#v", tools[0]["parameters"])
	}
}

func TestRuntimeSchemaReportsEmbeddedInterfaceMetadata(t *testing.T) {
	mcpFixture := embeddedMCPMetadata{
		Version:        1,
		Source:         "cli-registry",
		SourceRevision: "revision-test",
		SourceHash:     "sha256:interface-test",
		Coverage: embeddedMCPMetadataCoverage{
			SurfaceScope:   "source_revision",
			SourceTools:    10,
			SurfaceTools:   2,
			MatchedTools:   1,
			UnmatchedTools: 1,
		},
		Tools: map[string]embeddedMCPToolMetadata{
			"doc.create_document": {Description: "创建文档"},
		},
	}
	agentFixture := emptyEmbeddedAgentMetadata()

	catalog, err := runtimeSchemaPayloadForTestWithMetadata(buildRuntimeSchemaTestRoot(), nil, agentFixture, mcpFixture)
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest(catalog): %v", err)
	}
	summary, _ := catalog["interface_metadata"].(map[string]any)
	if summary["source"] != "cli-registry" || summary["source_hash"] != "sha256:interface-test" || schemaTestInt(summary["tool_count"]) != 1 {
		t.Fatalf("interface metadata summary = %#v", summary)
	}
	coverage, _ := summary["coverage"].(map[string]any)
	if summary["source_revision"] != "revision-test" || coverage["surface_scope"] != "source_revision" || schemaTestInt(coverage["surface_tools"]) != 2 {
		t.Fatalf("interface metadata provenance = %#v", summary)
	}

	registry, err := schemaRegistryForTestWithMetadata(buildRuntimeSchemaTestRoot(), agentFixture, mcpFixture)
	if err != nil {
		t.Fatalf("schemaRegistryForTest(): %v", err)
	}
	compact, err := registry.ToOverviewPayload()
	if err != nil {
		t.Fatalf("ToOverviewPayload(): %v", err)
	}
	if compact["interface_metadata"] == nil {
		t.Fatalf("compact schema dropped interface metadata: %#v", compact)
	}
}

func schemaTestInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := json.Number(typed).Int64()
		return int(parsed)
	default:
		return 0
	}
}

func TestRuntimeSchemaUsesVersionedInterfaceRef(t *testing.T) {
	agentFixture := embeddedAgentMetadata{
		Tools: map[string]agentToolMetadata{
			"doc create": {
				InterfaceRef:  &embeddedMCPInterfaceRef{ProductID: "documents", RPCName: "create_doc_v2"},
				InterfaceMode: "mcp",
				Availability:  "available",
			},
		},
		Products: map[string]agentProductMetadata{},
	}
	mcpFixture := embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"documents.create_doc_v2": {
				Parameters: map[string]embeddedMCPParamMeta{
					"title": {Description: "MCP document title"},
				},
			},
		},
	}

	payload, err := runtimeSchemaPayloadForTestWithMetadata(buildRuntimeSchemaTestRoot(), []string{"doc.create_document"}, agentFixture, mcpFixture)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := payload["interface_ref"].(map[string]any)
	if ref["product_id"] != "documents" || ref["rpc_name"] != "create_doc_v2" {
		t.Fatalf("interface_ref = %#v", payload["interface_ref"])
	}
	parameters, _ := payload["parameters"].(map[string]any)
	title, _ := parameters["title"].(map[string]any)
	if title["interface_description"] != "MCP document title" {
		t.Fatalf("title metadata = %#v", title)
	}
}

func TestMCPRequiredParticipatesInSourcePrecedence(t *testing.T) {
	required := true
	agentFixture := emptyEmbeddedAgentMetadata()
	mcpFixture := embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"sample.list_items": {
				Parameters: map[string]embeddedMCPParamMeta{
					"limit": {Required: &required},
				},
			},
		},
	}

	root := &cobra.Command{Use: "dws"}
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	list.Flags().Int("limit", 0, "optional page size")
	AttachRuntimeSchema(list, "sample", "list_items", "test")
	sample := &cobra.Command{Use: "sample"}
	sample.AddCommand(list)
	root.AddCommand(sample)

	payload, err := runtimeSchemaPayloadForTestWithMetadata(root, []string{"sample.list_items"}, agentFixture, mcpFixture)
	if err != nil {
		t.Fatal(err)
	}
	parameters, _ := payload["parameters"].(map[string]any)
	limit, _ := parameters["limit"].(map[string]any)
	if limit["required"] != true {
		t.Fatalf("MCP required candidate did not win over the default: %#v", limit)
	}
}

func TestMCPDefaultDoesNotOverrideCLIDefault(t *testing.T) {
	agentFixture := emptyEmbeddedAgentMetadata()
	mcpFixture := embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"sample.list_items": {
				Parameters: map[string]embeddedMCPParamMeta{
					"limit": {Default: "50"},
				},
			},
		},
	}

	root := &cobra.Command{Use: "dws"}
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	list.Flags().Int("limit", 10, "optional page size")
	AttachRuntimeSchema(list, "sample", "list_items", "test")
	sample := &cobra.Command{Use: "sample"}
	sample.AddCommand(list)
	root.AddCommand(sample)

	payload, err := runtimeSchemaPayloadForTestWithMetadata(root, []string{"sample.list_items"}, agentFixture, mcpFixture)
	if err != nil {
		t.Fatal(err)
	}
	parameters, _ := payload["parameters"].(map[string]any)
	limit, _ := parameters["limit"].(map[string]any)
	if limit["default"] != "10" || limit["interface_default"] != "50" {
		t.Fatalf("CLI and interface defaults were not separated: %#v", limit)
	}
}

func findSchemaProduct(products []map[string]any, id string) map[string]any {
	for _, product := range products {
		if product["id"] == id {
			return product
		}
	}
	return nil
}

func buildRuntimeSchemaTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	create := &cobra.Command{Use: "create", Short: "Create document", Run: func(*cobra.Command, []string) {}}
	create.Flags().String("title", "", "Document title")
	AttachRuntimeSchema(create, "doc", "create_document", "runtime:doc")
	AnnotateRuntimeFlag(create, "title", "title", "string", true, "")
	doc := &cobra.Command{Use: "doc", Short: "Docs"}
	doc.AddCommand(create)
	root.AddCommand(doc)
	return root
}
