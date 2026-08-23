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
	"io/fs"
	"strings"
)

// agentMetadataDomain is the retired split-domain JSON fragment shape. Tests
// keep a MapFS loader for regression coverage; production never embeds or loads
// schema_agent_metadata/*.json.
type agentMetadataDomain struct {
	ProductID string                       `json:"product_id"`
	Tools     map[string]agentToolMetadata `json:"tools"`
}

// loadAgentMetadataFixtureFrom is a test-only seam for MapFS fixtures that
// exercise the retired split-domain JSON shape.
func loadAgentMetadataFixtureFrom(source fs.FS) agentMetadata {
	var metadata agentMetadata
	index, err := fs.ReadFile(source, "schema_agent_metadata/index.json")
	if err != nil || json.Unmarshal(index, &metadata) != nil {
		return emptyAgentMetadata()
	}
	metadata.Tools = map[string]agentToolMetadata{}
	for _, domain := range metadata.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" || strings.Contains(domain, "/") || strings.Contains(domain, "\\") {
			return emptyAgentMetadata()
		}
		data, err := fs.ReadFile(source, "schema_agent_metadata/"+domain+".json")
		if err != nil {
			return emptyAgentMetadata()
		}
		var fragment agentMetadataDomain
		if err := json.Unmarshal(data, &fragment); err != nil || strings.TrimSpace(fragment.ProductID) != domain {
			return emptyAgentMetadata()
		}
		for path, tool := range fragment.Tools {
			metadata.Tools[path] = tool
		}
	}
	if metadata.Products == nil {
		metadata.Products = map[string]agentProductMetadata{}
	}
	return metadata
}
