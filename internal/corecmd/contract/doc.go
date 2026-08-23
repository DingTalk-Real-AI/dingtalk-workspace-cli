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

// Package contract owns command-framework declaration DTOs and the
// ProductDecl registry. It is intentionally free of Cobra-keyed runtime
// stores and Catalog delivery code.
//
// This package is the sole definition point for:
//   - ContractFinalPayload (DTO only — store lives in corecmd/contractfinal)
//   - ProductDecl (+ string-keyed registry; not Cobra-keyed)
//   - SafetySpec / SelectionSpec / InterfaceSpec / DryRunSpec / identity /
//     positionals / ParamDecl
//   - FieldProvenance / FieldCandidateProvenance
//
// Package boundary:
//
//   - Types / ProductDecl → corecmd/contract (this package).
//   - Authoring wrapper → corecmd.ContractDecl (leaf-facing; nested fields are
//     these contract types). Name is ContractDecl, not SchemaDecl: "Schema" in
//     this repo means Catalog / ToolSpec delivery, not the author declaration.
//   - AnnotateRuntime* writers → internal/corecmd/runtimeannotate
//     (framework-owned; cli may thin re-export; corecmd must not import cli).
//   - ContractFinal cobra store + Register seam → internal/corecmd/contractfinal
//     (all callers register here directly; corecmd.New registers internally).
//   - Catalog assembly / ResolveMeta (`RegisterSchemaSourceRoot` →
//     `ResolveSchemaBuild`); go:embed only for reviewed inputs → internal/cli
//     (delivery root).
//
// Description declare vs delivery (not dual authority):
//
//   - Construction requires ContractDecl.Description (declaration evidence).
//   - Catalog delivery prefers Cobra Long when present → provenance cobra_help;
//     without Long, declared Description is delivered → contract_final.
//   - Title prefers declared ContractDecl/ContractFinal, then Cobra Short, then
//     MCP metadata. Declare is not "wire final value" for description when Long
//     exists; assembly stamps the real winner.
//
// Authoring path: corecmd.ContractDecl → ContractFinalPayload (via
// contractfinal seam); ProductDecl for product-level Agent routing.
// Provenance stamp for declared leaf Safety remains "corecmd.contract".
package contract
