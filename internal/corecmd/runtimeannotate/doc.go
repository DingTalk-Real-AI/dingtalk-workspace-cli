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

// Package runtimeannotate owns Cobra dws.schema.* annotation writers and the
// RuntimeSchemaConstraints helpers used by the command framework.
//
// Package boundary:
//
//   - Types / DTO / ProductDecl → internal/corecmd/contract
//   - AnnotateRuntime* writers (this package) — no Catalog delivery
//   - ContractFinal cobra store + Register seam → internal/corecmd/contractfinal
//   - Catalog assembly / ResolveMeta (`RegisterSchemaSourceRoot` →
//     `ResolveSchemaBuild`); go:embed only for reviewed inputs → internal/cli
//     (root)
//
// Dependency direction: corecmd and corecmd/contractfinal import this package.
// This package must never import internal/cli (root or subpackages).
// Product / delivery code consumes this package directly, or via the cli
// root's package-local aliases (internal/cli/runtime_schema_seam.go).
package runtimeannotate
