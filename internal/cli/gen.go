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

// gen.go is the single entry point for schema metadata generation. It isolates
// all //go:generate pragmas from business code so that:
//   - schema_agent_metadata.go / schema_catalog.go contain only types + embed.
//   - Generation is a standalone process (make generate-schema triggers this).
//   - The 6-input → 1-output contract is documented in one place.
//
// Generation inputs (authored, reviewed):
//   1. schema_command_registry/             identity (canonical/aliases/navigation)
//   2. schema_hints/metadata/*.json        safety (effect/risk/confirmation)
//   3. schema_hints/selection/*.json       selection (use_when/avoid_when)
//   4. schema_mcp_metadata.json            MCP server tool definitions
//   5. schema_parameter_bindings.json      parameter type/property mappings
//   6. cobra command tree (Go runtime)     flags/usage/required (reflected)
//
// Generation outputs (embedded at build):
//   - schema_agent_metadata/*.json         per-product agent metadata
//   - schema_catalog/                      per-product catalog shards

package cli

//go:generate go run ../generator/cmd_schema_agent_metadata -root ../.. -registry internal/cli/schema_command_registry -output-dir schema_agent_metadata -audit-output schema_agent_metadata_audit.json
// Rebuild all dependencies so the Catalog compiler cannot reuse the cli
// package cached by the preceding metadata generator with the old embedded
// JSON files.
//go:generate go run -a ../generator/cmd_schema_catalog -root ../.. -output schema_catalog
