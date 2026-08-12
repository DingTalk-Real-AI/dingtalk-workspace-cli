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
	"testing/fstest"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestAgentMetadataFixtureLoadsSplitDomains(t *testing.T) {
	// Production no longer embeds or ships schema_agent_metadata/*.json.
	// Runtime Agent metadata must stay empty; selection completeness now lives
	// in schema_catalog (see TestDeliverySchemaCatalogSelectionCompleteness).
	metadata := runtimeAgentMetadata()
	if len(metadata.Tools) != 0 || len(metadata.Products) != 0 || len(metadata.Domains) != 0 {
		t.Fatalf("retired Agent metadata snapshot must be empty: %#v", metadata)
	}
	// Temporary MapFS fixture only — exercises the retired split-domain loader
	// seam without depending on a committed schema_agent_metadata/ directory.
	fixture := fstest.MapFS{
		"schema_agent_metadata/index.json":  {Data: []byte(`{"domains":["sample"],"coverage":{"tools_with_metadata":1}}`)},
		"schema_agent_metadata/sample.json": {Data: []byte(`{"product_id":"sample","tools":{"sample.get":{"agent_summary":"S","use_when":["u"],"avoid_when":["a"],"examples":["dws sample get"],"interface_mode":"local","availability":"available"}}}`)},
	}
	loaded := loadAgentMetadataFixtureFrom(fixture)
	if len(loaded.Tools) != 1 || loaded.Tools["sample.get"].AgentSummary != "S" {
		t.Fatalf("fixture loader = %#v", loaded)
	}
}

// TestDeliverySchemaCatalogSelectionCompleteness replaces the retired
// schema_agent_metadata/*.json split-domain coverage gate: every delivered
// Catalog tool must carry non-empty selection routing, interface disposition,
// and examples that never bypass confirmation with --yes.
func TestDeliverySchemaCatalogSelectionCompleteness(t *testing.T) {
	if !deliverySchemaCatalogAvailable() {
		t.Fatalf("delivery schema Catalog is unavailable: %v", deliverySchemaCatalogError())
	}
	loaded := mustDeliverySchemaCatalogMaps(t)
	products := map[string]struct{}{}
	for canonical, tool := range loaded.Snapshot.Tools {
		product := schemaString(tool["product_id"])
		if product == "" {
			t.Errorf("tool %s missing product_id", canonical)
			continue
		}
		products[product] = struct{}{}
		if len(schemaStringSlice(tool["use_when"])) == 0 ||
			len(schemaStringSlice(tool["avoid_when"])) == 0 ||
			len(schemaStringSlice(tool["examples"])) == 0 {
			t.Errorf("tool %s has incomplete selection metadata: use_when=%v avoid_when=%v examples=%v",
				canonical, tool["use_when"], tool["avoid_when"], tool["examples"])
		}
		if schemaString(tool["interface_mode"]) == "" || schemaString(tool["availability"]) == "" {
			t.Errorf("tool %s has incomplete interface disposition: mode=%q availability=%q",
				canonical, schemaString(tool["interface_mode"]), schemaString(tool["availability"]))
		}
		for _, example := range schemaStringSlice(tool["examples"]) {
			if strings.Contains(" "+example+" ", " --yes ") {
				t.Errorf("tool %s example bypasses confirmation: %q", canonical, example)
			}
		}
	}
	if len(products) < 2 {
		t.Fatalf("catalog products = %d, want multi-product delivery", len(products))
	}
	if _, ok := loaded.Snapshot.Tools["calendar.create_calendar_event"]; !ok {
		t.Fatalf("calendar.create_calendar_event missing from catalog tools (%d total)", len(loaded.Snapshot.Tools))
	}
	if got, want := len(loaded.Snapshot.Tools), len(loaded.Index.CanonicalPaths()); got != want {
		t.Fatalf("catalog tools = %d, typed index = %d", got, want)
	}
}

func TestRuntimeSchemaIncludesAgentMetadata(t *testing.T) {
	// Agent selection / safety / interface facts declare on the leaf
	// ContractFinal and ProductDecl; the retired agent-metadata inject no
	// longer participates in assembly.
	root := buildRuntimeSchemaTestRoot()
	declareRuntimeSchemaTestRootDoc(t, root, nil)

	leaf, err := runtimeSchemaPayloadForTest(root, []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest(leaf): %v", err)
	}
	if leaf["effect"] != "write" || leaf["agent_metadata_source"] != "corecmd.contract" {
		t.Fatalf("leaf Agent metadata = %#v", leaf)
	}
	if leaf["interface_mode"] != "local" || leaf["availability"] != "available" || leaf["interface_reason"] != "test local implementation" {
		t.Fatalf("leaf interface disposition = %#v", leaf)
	}
	if examples, _ := leaf["examples"].([]string); len(examples) != 1 {
		t.Fatalf("leaf examples = %#v", leaf["examples"])
	}

	catalog, err := runtimeSchemaPayloadForTest(root, nil)
	if err != nil {
		t.Fatalf("runtimeSchemaPayloadForTest(catalog): %v", err)
	}
	summary, _ := catalog["agent_metadata"].(map[string]any)
	// Catalog-level Agent summary is derived from assembled products/tools
	// (ContractFinal / ProductDecl), not from an injected fixture source_hash.
	if summary["source"] != ProvenanceEmbeddedSkillMetadata {
		t.Fatalf("catalog Agent metadata summary = %#v", summary)
	}
	if summary["tools_with_metadata"] == nil || summary["products_with_metadata"] == nil {
		t.Fatalf("catalog Agent metadata summary missing coverage fields: %#v", summary)
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

	registry, err := schemaRegistryForTest(root)
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
	// Synthetic fixture declares ContractFinal/ProductDecl so the full export
	// exercises the production assembly path.
	root := buildRuntimeSchemaTestRoot()
	declareRuntimeSchemaTestRootDoc(t, root, nil)
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := registry.ToPayload()
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
	// interface_ref declares on the leaf ContractFinal; MCP pin is not a
	// parameter candidate source.
	root := buildRuntimeSchemaTestRoot()
	declareRuntimeSchemaTestRootDoc(t, root, func(payload *contract.ContractFinalPayload) {
		payload.Interface = &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeMCP,
			Availability: contract.InterfaceAvailable,
			Reason:       "reviewed RPC mapping",
			Ref:          &contract.InterfaceRefSpec{ProductID: "documents", RPCName: "create_doc_v2"},
		}
	})

	payload, err := runtimeSchemaPayloadForTest(root, []string{"doc.create_document"})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := payload["interface_ref"].(map[string]any)
	if ref["product_id"] != "documents" || ref["rpc_name"] != "create_doc_v2" {
		t.Fatalf("interface_ref = %#v", payload["interface_ref"])
	}
	if payload["interface_mode"] != contract.InterfaceModeMCP {
		t.Fatalf("interface_mode = %#v", payload["interface_mode"])
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
	AnnotateRuntimeFlag(create, "title", "title", "string", true)
	doc := &cobra.Command{Use: "doc", Short: "Docs"}
	doc.AddCommand(create)
	root.AddCommand(doc)
	return root
}
