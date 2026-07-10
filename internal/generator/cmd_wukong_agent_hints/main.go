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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/wukonghints"
)

func main() {
	var envelopeDir string
	var surfacePath string
	var outputPath string
	var auditOutputPath string
	var repository string
	var revision string
	var channel string
	var maxExamples int
	flag.StringVar(&envelopeDir, "envelope-dir", "", "Wukong versioned envelope directory")
	flag.StringVar(&surfacePath, "surface", "internal/cli/schema_command_surface.json", "DWS public command-surface snapshot")
	flag.StringVar(&outputPath, "output", "skills/mono/schema-hints/imported/wukong.json", "Sanitized versioned Agent hint output")
	flag.StringVar(&auditOutputPath, "audit-output", "internal/cli/schema_wukong_agent_hints_audit.json", "Import coverage and unmatched-path audit output")
	flag.StringVar(&repository, "repository", "dws-wukong", "Source repository identity")
	flag.StringVar(&revision, "revision", "", "Immutable source revision")
	flag.StringVar(&channel, "channel", "prod", "Envelope channel")
	flag.IntVar(&maxExamples, "max-examples", 3, "Maximum imported examples per tool")
	flag.Parse()

	result, err := wukonghints.Import(wukonghints.Options{
		EnvelopeDir: envelopeDir,
		SurfacePath: surfacePath,
		Repository:  repository,
		Revision:    revision,
		Channel:     channel,
		MaxExamples: maxExamples,
	})
	if err != nil {
		fail(err)
	}
	if err := writeJSON(outputPath, result.Hints); err != nil {
		fail(err)
	}
	if strings.TrimSpace(auditOutputPath) != "" {
		if err := writeJSON(auditOutputPath, result.Audit); err != nil {
			fail(err)
		}
	}
	coverage := result.Hints.Coverage
	_, _ = fmt.Fprintf(os.Stderr,
		"generated Wukong Agent hints: output=%s products=%d/%d tools=%d/%d unmatched=%d source=%s\n",
		outputPath,
		coverage.MatchedProducts,
		coverage.SourceProducts,
		coverage.MatchedTools,
		coverage.EligibleTools,
		coverage.UnmatchedTools,
		result.Hints.Source.Revision,
	)
}

func writeJSON(path string, value any) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-wukong-agent-hints: %v\n", err)
	os.Exit(1)
}
