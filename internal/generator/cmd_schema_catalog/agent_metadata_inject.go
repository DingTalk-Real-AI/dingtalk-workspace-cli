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

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/spf13/cobra"
)

var (
	generateCatalogAgentMetadata = agentmetadata.GenerateFromCommandRoot
	encodeCatalogAgentMetadata   = json.Marshal
	injectCatalogAgentMetadata   = cli.InstallBuildTimeAgentMetadataJSON
)

// installBuildTimeAgentMetadata generates Agent metadata in-memory via the
// shared agentmetadata pipeline and injects it into cli assembly for this
// CI/local Catalog dump only. Production Agent authority is leaf
// ContractFinal / ProductDecl; nothing is written under
// internal/cli/schema_agent_metadata/.
func installBuildTimeAgentMetadata(rootPath string, commandRoot *cobra.Command) error {
	metadata, _, projection, err := generateCatalogAgentMetadata(rootPath, commandRoot, agentmetadata.Options{})
	if err != nil {
		return err
	}
	encoded, err := encodeCatalogAgentMetadata(metadata)
	if err != nil {
		return fmt.Errorf("encode in-memory Agent metadata: %w", err)
	}
	if err := injectCatalogAgentMetadata(encoded); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "injected build-time Agent metadata: products=%d tools=%d surface_tools=%d\n",
		len(metadata.Products), len(metadata.Tools), projection.ToolCount)
	return nil
}
