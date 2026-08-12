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

package agentmetadata_test

import (
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
)

func TestCrossPlatformCoverageGenerateProductionAgentMetadataPipeline(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := app.NewSchemaSourceRootCommand()
	metadata, stats, projection, err := agentmetadata.GenerateFromCommandRoot(repositoryRoot, root, agentmetadata.Options{})
	if err != nil {
		t.Fatalf("GenerateFromCommandRoot() error = %v", err)
	}
	if len(metadata.Tools) != len(projection.CanonicalToolPaths) || stats.Tools == 0 {
		t.Fatalf("generated metadata mismatch: tools=%d registry=%d stats=%d", len(metadata.Tools), len(projection.CanonicalToolPaths), stats.Tools)
	}
	if audit := agentmetadata.BuildAudit(metadata, stats); audit.SourceHash == "" {
		t.Fatal("generated audit has an empty source hash")
	}
}
