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

// gen.go isolates //go:generate pragmas from business code.
//
// Schema Catalog delivery is NOT generated here. Production assembles Catalog
// at runtime via ResolveSchemaBuild (声明即 Catalog); see schema_source_root.go
// and internal/app/schema_source_register.go. cmd_schema_catalog remains a
// CI/determinism tool (make generate-schema / check-generated-drift) and must
// not be a committed delivery step.
//
// Inputs consumed by runtime assembly:
//   1. leaf ContractFinal.Identity         identity (canonical/aliases/navigation),
//                                          collected from the live Cobra tree
//   2. contract.ProductDecl + leaf ContractFinal    Agent routing / selection prose
//                                          (Interface / ParamDecl own interface facts)
//   3. schema_parameter_mapping_ledger.go  mapping exclusions / removals (Go)
//                                         (active bindings retired; ParamDecl.Property owns delivery)
//   4. param_concepts.json + schema       reviewed parameter synonym policy
//   5. command_path_fallbacks.json + schema
//                                        reviewed recovery-only invalid paths
//   6. cobra command tree (Go runtime)     flags/usage/required (reflected)
//
// schema_hints/, schema_agent_metadata/, schema_command_registry/,
// schema_mcp_metadata.json, and schema_mcp_service_review(.json|/ledger) are
// retired.
//
// Remaining generated output from this file:
//   - param_aliases_generated.go           per-command parameter normalization
//   - command_path_fallbacks_generated.go  invalid-path recovery normalization

package cli

//go:generate go run ../generator/cmd_param_aliases -root ../.. -output param_aliases_generated.go
//go:generate go run ../generator/cmd_command_path_fallbacks -root ../.. -output command_path_fallbacks_generated.go
