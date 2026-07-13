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

func TestRuntimeSchemaIncludesEmbeddedAgentMetadata(t *testing.T) {
	previous := runtimeEmbeddedAgentMetadata
	runtimeEmbeddedAgentMetadata = embeddedAgentMetadata{
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
	t.Cleanup(func() { runtimeEmbeddedAgentMetadata = previous })

	root := buildRuntimeSchemaTestRoot()
	leaf, err := runtimeSchemaPayload(root, []string{"doc.create_document"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload(leaf): %v", err)
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

	catalog, err := runtimeSchemaPayload(root, nil)
	if err != nil {
		t.Fatalf("runtimeSchemaPayload(catalog): %v", err)
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

	compact := compactSchemaOverviewPayload(catalog)
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

func TestRuntimeSchemaReportsEmbeddedInterfaceMetadata(t *testing.T) {
	previous := runtimeEmbeddedMCPMetadata
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
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
	t.Cleanup(func() { runtimeEmbeddedMCPMetadata = previous })

	catalog, err := runtimeSchemaPayload(buildRuntimeSchemaTestRoot(), nil)
	if err != nil {
		t.Fatalf("runtimeSchemaPayload(catalog): %v", err)
	}
	summary, _ := catalog["interface_metadata"].(map[string]any)
	if summary["source"] != "cli-registry" || summary["source_hash"] != "sha256:interface-test" || summary["tool_count"] != 1 {
		t.Fatalf("interface metadata summary = %#v", summary)
	}
	coverage, _ := summary["coverage"].(embeddedMCPMetadataCoverage)
	if summary["source_revision"] != "revision-test" || coverage.SurfaceScope != "source_revision" {
		t.Fatalf("interface metadata provenance = %#v", summary)
	}

	compact := compactSchemaOverviewPayload(catalog)
	if compact["interface_metadata"] == nil {
		t.Fatalf("compact schema dropped interface metadata: %#v", compact)
	}
}

func TestRuntimeSchemaUsesVersionedInterfaceRef(t *testing.T) {
	previousAgent := runtimeEmbeddedAgentMetadata
	previousInterface := runtimeEmbeddedMCPMetadata
	runtimeEmbeddedAgentMetadata = embeddedAgentMetadata{
		Tools: map[string]agentToolMetadata{
			"doc create": {
				InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "documents", RPCName: "create_doc_v2"},
			},
		},
		Products: map[string]agentProductMetadata{},
	}
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"documents.create_doc_v2": {
				Parameters: map[string]embeddedMCPParamMeta{
					"title": {Description: "MCP document title"},
				},
			},
		},
	}
	t.Cleanup(func() {
		runtimeEmbeddedAgentMetadata = previousAgent
		runtimeEmbeddedMCPMetadata = previousInterface
	})

	payload, err := runtimeSchemaPayload(buildRuntimeSchemaTestRoot(), []string{"doc.create_document"})
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

func TestMCPRequiredDoesNotPromoteOptionalCLIFlag(t *testing.T) {
	previousAgent := runtimeEmbeddedAgentMetadata
	previousInterface := runtimeEmbeddedMCPMetadata
	required := true
	runtimeEmbeddedAgentMetadata = emptyEmbeddedAgentMetadata()
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"sample.list_items": {
				Parameters: map[string]embeddedMCPParamMeta{
					"limit": {Required: &required},
				},
			},
		},
	}
	t.Cleanup(func() {
		runtimeEmbeddedAgentMetadata = previousAgent
		runtimeEmbeddedMCPMetadata = previousInterface
	})

	root := &cobra.Command{Use: "dws"}
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	list.Flags().Int("limit", 20, "optional page size")
	AttachRuntimeSchema(list, "sample", "list_items", "test")
	sample := &cobra.Command{Use: "sample"}
	sample.AddCommand(list)
	root.AddCommand(sample)

	payload, err := runtimeSchemaPayload(root, []string{"sample.list_items"})
	if err != nil {
		t.Fatal(err)
	}
	parameters, _ := payload["parameters"].(map[string]any)
	limit, _ := parameters["limit"].(map[string]any)
	if limit["required"] != false {
		t.Fatalf("optional CLI flag was promoted by MCP metadata: %#v", limit)
	}
}

func TestMCPDefaultDoesNotOverrideCLIDefault(t *testing.T) {
	previousAgent := runtimeEmbeddedAgentMetadata
	previousInterface := runtimeEmbeddedMCPMetadata
	runtimeEmbeddedAgentMetadata = emptyEmbeddedAgentMetadata()
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
		Tools: map[string]embeddedMCPToolMetadata{
			"sample.list_items": {
				Parameters: map[string]embeddedMCPParamMeta{
					"limit": {Default: "50"},
				},
			},
		},
	}
	t.Cleanup(func() {
		runtimeEmbeddedAgentMetadata = previousAgent
		runtimeEmbeddedMCPMetadata = previousInterface
	})

	root := &cobra.Command{Use: "dws"}
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	list.Flags().Int("limit", 10, "optional page size")
	AttachRuntimeSchema(list, "sample", "list_items", "test")
	sample := &cobra.Command{Use: "sample"}
	sample.AddCommand(list)
	root.AddCommand(sample)

	payload, err := runtimeSchemaPayload(root, []string{"sample.list_items"})
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
