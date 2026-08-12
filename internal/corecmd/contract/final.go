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

package contract

// ContractFinalPayload is the Contract-authored final Schema leaf overlay.
// Registered in-process by the framework via the corecmd/contractfinal seam;
// Schema assembly reads it as pass-through. No JSON bridge. Treat as
// read-only after Register.
//
// The Cobra-keyed runtime store and Register live in
// internal/corecmd/contractfinal (not this DTO package). AnnotateRuntime*
// writers live in internal/corecmd/runtimeannotate. All callers register via
// contractfinal.RegisterRuntimeContractFinal directly (corecmd.New registers
// internally).
type ContractFinalPayload struct {
	Title       string
	Description string
	Positionals []RuntimeSchemaPositional
	Parameters  []ParamDecl
	Safety      *SafetySpec
	DryRun      *DryRunSpec
	Result      *ResultSpec
	Pagination  *PaginationSpec
	Interface   *InterfaceSpec
	Selection   *SelectionSpec
	Identity    *ToolIdentitySpec
}
