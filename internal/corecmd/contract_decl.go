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

package corecmd

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ContractDecl is the authoring-time leaf contract declaration.
//
// Naming: this is not a Catalog / ToolSpec "Schema" object. Authors declare
// leaf Contract facts (selection / interface / parameters / dry-run /
// identity prose). AttachContract converts once into
// contract.ContractFinalPayload; Catalog assembly pass-throughs that payload.
// Nested fields reuse contract.* types directly so authoring cannot drift from
// the registry model.
//
// Description declare vs delivery (not dual authority, not "declare = wire"):
//   - Construction requires Description (declaration evidence / fail-closed).
//   - Catalog delivery: Cobra Long wins when present → provenance cobra_help;
//     without Long, declared Description is delivered → contract_final.
//   - Title: declared Title/ContractFinal first, then Cobra Short, then MCP.
//
// The payload stores the declared text; assembly stamps the real winner.
type ContractDecl struct {
	Title       string
	Description string // required at construction; Catalog may prefer Cobra Long
	Positionals []contract.RuntimeSchemaPositional
	Parameters  []contract.ParamDecl
	DryRun      *contract.DryRunSpec
	Result      *contract.ResultSpec
	Interface   *contract.InterfaceSpec
	Selection   contract.SelectionSpec
	Identity    contract.ToolIdentitySpec
}

// validateContractDecl enforces authoring-time homology for declared commands.
// A declared Contract is the sole final source for its fields: downstream
// catalog/Agent gates hard-require description and the selection prose for
// every effective tool — so a declaration missing any of these fields could
// only fail later, opaquely, in generated artifacts. Failing at construction
// keeps the error next to the authoring mistake and prevents silent drift
// between cobra prose (Short/Long/Example) and the published Contract values.
//
// Identity is required on every declared Contract as the leaf's
// self-description: identity is collected from ContractFinal on the live
// leaves, so an incomplete or inconsistent declared Identity fails collection,
// binding, and policy downstream.
func validateContractDecl(spec Spec) {
	if spec.Contract.empty() {
		return
	}
	missing := make([]string, 0, 8)
	if strings.TrimSpace(spec.Contract.Description) == "" {
		missing = append(missing, "Contract.Description")
	}
	if strings.TrimSpace(spec.Contract.Selection.AgentSummary) == "" {
		missing = append(missing, "Contract.Selection.AgentSummary")
	}
	if len(spec.Contract.Selection.UseWhen) == 0 {
		missing = append(missing, "Contract.Selection.UseWhen")
	}
	if len(spec.Contract.Selection.AvoidWhen) == 0 {
		missing = append(missing, "Contract.Selection.AvoidWhen")
	}
	if len(spec.Contract.Selection.Examples) == 0 {
		missing = append(missing, "Contract.Selection.Examples")
	}
	// Interface is an unconditional catalog required key for declared tools
	// (no hints fallback). Spec.Safety is validated separately and is
	// the single source for both runtime confirmation and the Schema block.
	if iface := spec.Contract.Interface; iface == nil ||
		strings.TrimSpace(iface.Mode) == "" || strings.TrimSpace(iface.Availability) == "" {
		missing = append(missing, "Contract.Interface (mode/availability)")
	} else if (iface.Mode == contract.InterfaceModeComposite || iface.Availability == contract.InterfaceUnavailable) &&
		strings.TrimSpace(iface.Reason) == "" {
		missing = append(missing, "Contract.Interface.Reason")
	}
	id := spec.Contract.Identity
	productID := strings.TrimSpace(id.ProductID)
	name := strings.TrimSpace(id.Name)
	canonical := strings.TrimSpace(id.CanonicalPath)
	cliPath := strings.TrimSpace(id.CLIPath)
	primary := strings.TrimSpace(id.PrimaryCLIPath)
	if productID == "" {
		missing = append(missing, "Contract.Identity.ProductID")
	}
	if name == "" {
		missing = append(missing, "Contract.Identity.Name")
	}
	if canonical == "" {
		missing = append(missing, "Contract.Identity.CanonicalPath")
	}
	if cliPath == "" && primary == "" {
		missing = append(missing, "Contract.Identity.CLIPath")
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf(
			"command %q declares Contract but is missing %s: a declared Contract is the final source and must carry the full reviewed prose (no hints fallback)",
			spec.Use, strings.Join(missing, ", ")))
	}
	wantCanonical := productID + "." + name
	if canonical != wantCanonical {
		panic(fmt.Sprintf(
			"command %q Contract.Identity.CanonicalPath %q must equal ProductID.Name %q",
			spec.Use, canonical, wantCanonical))
	}
	if cliPath != "" && primary != "" && cliPath != primary {
		panic(fmt.Sprintf(
			"command %q Contract.Identity.CLIPath %q and PrimaryCLIPath %q must agree on the primary leaf path",
			spec.Use, cliPath, primary))
	}
}

// Empty reports whether no ContractDecl field was authored.
func (s ContractDecl) Empty() bool {
	return s.empty()
}

// empty reports whether no ContractDecl field was authored.
func (s ContractDecl) empty() bool {
	if strings.TrimSpace(s.Title) != "" || strings.TrimSpace(s.Description) != "" {
		return false
	}
	if len(s.Positionals) > 0 {
		return false
	}
	if len(s.Parameters) > 0 {
		return false
	}
	if s.DryRun != nil && strings.TrimSpace(s.DryRun.PreviewKind) != "" {
		return false
	}
	if s.Result != nil {
		return false
	}
	if s.Interface != nil {
		iface := s.Interface
		if strings.TrimSpace(iface.Mode) != "" || strings.TrimSpace(iface.Availability) != "" ||
			strings.TrimSpace(iface.Reason) != "" || iface.Ref != nil {
			return false
		}
	}
	if strings.TrimSpace(s.Selection.AgentSummary) != "" || len(s.Selection.UseWhen) > 0 ||
		len(s.Selection.AvoidWhen) > 0 || len(s.Selection.Examples) > 0 ||
		len(s.Selection.Prerequisites) > 0 || len(s.Selection.Tips) > 0 ||
		len(s.Selection.WorkflowRefs) > 0 {
		return false
	}
	if strings.TrimSpace(s.Identity.ProductID) != "" || strings.TrimSpace(s.Identity.SourceProductID) != "" ||
		strings.TrimSpace(s.Identity.Name) != "" || strings.TrimSpace(s.Identity.CLIName) != "" ||
		strings.TrimSpace(s.Identity.CanonicalPath) != "" || strings.TrimSpace(s.Identity.Path) != "" ||
		strings.TrimSpace(s.Identity.CLIPath) != "" || strings.TrimSpace(s.Identity.PrimaryCLIPath) != "" ||
		strings.TrimSpace(s.Identity.Group) != "" || strings.TrimSpace(s.Identity.Source) != "" ||
		len(s.Identity.Aliases) > 0 || s.Identity.IsAlias {
		return false
	}
	return true
}
