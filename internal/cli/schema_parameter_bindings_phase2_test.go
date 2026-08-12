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
	"testing"
)

func TestSchemaParameterBindingsPhase2RemainingInventory(t *testing.T) {
	bindings, err := LoadSchemaParameterBindings()
	if err != nil {
		t.Fatalf("LoadSchemaParameterBindings() error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("remaining active binding groups = %d, want 0 after ParamDecl.Property retirement", len(bindings))
	}
	snapshot, err := loadSchemaParameterBindingSnapshot()
	if err != nil {
		t.Fatalf("loadSchemaParameterBindingSnapshot() error = %v", err)
	}
	if len(snapshot.MappingExclusions) == 0 {
		t.Fatal("mapping exclusions ledger is empty; exclusion semantics must remain reviewed in schema_parameter_mapping_ledger.go")
	}
	if len(snapshot.Removals) == 0 {
		t.Fatal("removals ledger is empty; semantic deletions must remain reviewed in schema_parameter_mapping_ledger.go")
	}
	t.Logf("Phase 2 complete: 0 active tuples; %d mapping_exclusions; %d removals (Go ledger)", len(snapshot.MappingExclusions), len(snapshot.Removals))
}

func TestSchemaParameterBindingsPhase2ParamDeclPropertyOutranksBinding(t *testing.T) {
	binding := runtimeSchemaStringCandidate("fromJSON", "versioned_parameter_binding")
	paramDecl := runtimeSchemaStringCandidateAtRank(
		"fromParamDecl",
		"native_annotation",
		runtimeSchemaRankParamDeclProperty,
		runtimeSchemaPrecedenceNativeAnnotation,
	)
	winner, err := resolveRuntimeSchemaCandidate("property", binding, paramDecl)
	if err != nil {
		t.Fatalf("resolveRuntimeSchemaCandidate() error = %v", err)
	}
	if winner.Source != "native_annotation" || winner.Value != "fromParamDecl" {
		t.Fatalf("ParamDecl.Property dual-read winner = %#v, want native fromParamDecl", winner)
	}
}

func TestSchemaParameterBindingsJSONRetired(t *testing.T) {
	// Guard: do not reintroduce the empty bindings{} JSON audit table.
	if _, err := os.Stat("schema_parameter_bindings.json"); err == nil {
		t.Fatal("schema_parameter_bindings.json must stay deleted; use schema_parameter_mapping_ledger.go")
	}
}
