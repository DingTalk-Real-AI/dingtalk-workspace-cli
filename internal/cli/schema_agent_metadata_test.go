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

import "testing"

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
}

func TestRuntimeSchemaIncludesEmbeddedAgentMetadata(t *testing.T) {
	previous := runtimeEmbeddedAgentMetadata
	runtimeEmbeddedAgentMetadata = embeddedAgentMetadata{
		Version:    1,
		SourceHash: "sha256:test",
		Products: map[string]agentProductMetadata{
			"doc": {UseWhen: []string{"需要创建或读取文档"}},
		},
		Tools: map[string]agentToolMetadata{
			"doc create": {
				UseWhen:      []string{"新建文档"},
				Effect:       "write",
				EffectSource: "command-verb",
				Examples:     []string{"dws doc create --title test"},
				SourceRefs:   []string{"skills/mono/references/products/doc.md"},
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
	if useWhen, _ := compactDoc["use_when"].([]string); len(useWhen) != 1 {
		t.Fatalf("compact product use_when = %#v", compactDoc["use_when"])
	}
}

func TestHelperSchemaIncludesEmbeddedAgentMetadata(t *testing.T) {
	previous := runtimeEmbeddedAgentMetadata
	runtimeEmbeddedAgentMetadata = embeddedAgentMetadata{
		Products: map[string]agentProductMetadata{
			"dev": {UseWhen: []string{"配置开放平台应用"}},
		},
		Tools: map[string]agentToolMetadata{
			"dev app robot config": {
				UseWhen:  []string{"配置机器人"},
				Effect:   "write",
				Examples: []string{"dws dev app robot config --unified-app-id app"},
			},
		},
	}
	t.Cleanup(func() { runtimeEmbeddedAgentMetadata = previous })

	root := buildHelperTestTree()
	leaf, ok, err := renderHelperSchema(root, "dev app robot config")
	if err != nil || !ok {
		t.Fatalf("renderHelperSchema() = ok:%v err:%v", ok, err)
	}
	if leaf["effect"] != "write" {
		t.Fatalf("helper leaf Agent metadata = %#v", leaf)
	}
	if examples, _ := leaf["examples"].([]string); len(examples) != 1 {
		t.Fatalf("helper leaf examples = %#v", leaf["examples"])
	}

	productPayload, ok, err := renderHelperSchema(root, "dev")
	if err != nil || !ok {
		t.Fatalf("renderHelperSchema(product) = ok:%v err:%v", ok, err)
	}
	product, _ := productPayload["product"].(map[string]any)
	if useWhen, _ := product["use_when"].([]string); len(useWhen) != 1 {
		t.Fatalf("helper product use_when = %#v", product["use_when"])
	}
}

func TestRuntimeSchemaReportsEmbeddedInterfaceMetadata(t *testing.T) {
	previous := runtimeEmbeddedMCPMetadata
	runtimeEmbeddedMCPMetadata = embeddedMCPMetadata{
		Version:    1,
		Source:     "cli-registry",
		SourceHash: "sha256:interface-test",
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

	compact := compactSchemaOverviewPayload(catalog)
	if compact["interface_metadata"] == nil {
		t.Fatalf("compact schema dropped interface metadata: %#v", compact)
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
