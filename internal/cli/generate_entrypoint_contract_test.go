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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageGoGenerateDirectivesStayInUnifiedEntryPoint(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/cli: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || name == "gen.go" {
			continue
		}
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range bytes.Split(content, []byte("\n")) {
			if strings.HasPrefix(strings.TrimSpace(string(line)), "//go:generate") {
				t.Errorf("%s contains //go:generate; all directives must stay in gen.go", name)
			}
		}
	}

	content, err := os.ReadFile("gen.go")
	if err != nil {
		t.Fatalf("read gen.go: %v", err)
	}
	for _, generator := range []string{
		"cmd_schema_catalog",
		"cmd_param_aliases",
		"cmd_command_path_fallbacks",
	} {
		if !bytes.Contains(content, []byte("//go:generate go run")) || !bytes.Contains(content, []byte(generator)) {
			t.Errorf("gen.go does not register %s", generator)
		}
	}
	if bytes.Contains(content, []byte("cmd_schema_agent_metadata")) {
		t.Error("gen.go must not regenerate retired schema_agent_metadata/")
	}
	for _, line := range bytes.Split(content, []byte("\n")) {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "//go:generate") && strings.Contains(trimmed, "schema_agent_metadata") {
			t.Errorf("go:generate must not target schema_agent_metadata: %s", trimmed)
		}
	}
}
