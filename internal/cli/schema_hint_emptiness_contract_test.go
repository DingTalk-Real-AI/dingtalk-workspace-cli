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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// TestDeletedProductionSchemaHintFilesStayGone locks stage-3 cleanup: the
// retired production hint init files must not return.
func TestDeletedProductionSchemaHintFilesStayGone(t *testing.T) {
	deleted := []string{
		"schema_hints_aitable.go",
		"schema_hints_attendance.go",
		"schema_hints_ding.go",
		"schema_hints_helper_roots.go",
		"schema_hints_runtime.go",
		"schema_manual_hints.go",
		"schema_hint_dirs.go",
		"schema_hint_decls.go",
		"schema_hint_decls_generated.go",
		"schema_hints.go",
		"schema_wukong_agent_hints_audit.json",
	}
	for _, name := range deleted {
		path := filepath.Join(".", name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s must stay deleted after Manual Hint / Schema Hint retirement", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

// TestSchemaHintsDirectoryRetired locks the full schema_hints/ retirement:
// the directory must not reappear as a generation or runtime input.
func TestSchemaHintsDirectoryRetired(t *testing.T) {
	if _, err := os.Stat("schema_hints"); !os.IsNotExist(err) {
		t.Fatalf("schema_hints/ must stay deleted after HintFile retirement: %v", err)
	}
}

// TestDeliveryCatalogHasNoToolSchemaHintProvenance asserts the published
// Catalog no longer selects tool_schema_hint as a provenance winner or
// retained candidate after RegisterSchemaHints → contract.ParamDecl migration.
func TestDeliveryCatalogHasNoToolSchemaHintProvenance(t *testing.T) {
	if !deliverySchemaCatalogAvailable() {
		t.Fatal("delivery schema Catalog unavailable")
	}
	loaded := deliverySchemaCatalog()
	hits := make([]string, 0)
	for _, product := range loaded.Registry.Products {
		for _, tool := range product.Tools {
			hits = append(hits, toolSchemaHintProvenanceHits(tool.Identity.CanonicalPath, tool.FieldProvenance)...)
			for _, param := range tool.Parameters {
				prefix := tool.Identity.CanonicalPath + "." + param.Name
				hits = append(hits, toolSchemaHintProvenanceHits(prefix, param.FieldProvenance)...)
			}
		}
	}
	if total := len(hits); total != 0 {
		sample := hits
		if total > 20 {
			sample = hits[:20]
		}
		t.Fatalf("catalog field_provenance still references tool_schema_hint (count=%d); sample=%v", total, sample)
	}
}

func toolSchemaHintProvenanceHits(prefix string, provenance map[string]contract.FieldProvenance) []string {
	if len(provenance) == 0 {
		return nil
	}
	hits := make([]string, 0)
	fields := make([]string, 0, len(provenance))
	for field := range provenance {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		prov := provenance[field]
		if prov.Source == "tool_schema_hint" {
			hits = append(hits, prefix+"."+field+"#winner")
		}
		for i, candidate := range prov.Candidates {
			if candidate.Source == "tool_schema_hint" {
				hits = append(hits, prefix+"."+field+"#candidate["+strconv.Itoa(i)+"]")
			}
		}
		for i, candidate := range prov.OverriddenCandidates {
			if candidate.Source == "tool_schema_hint" {
				hits = append(hits, prefix+"."+field+"#overridden["+strconv.Itoa(i)+"]")
			}
		}
	}
	return hits
}
