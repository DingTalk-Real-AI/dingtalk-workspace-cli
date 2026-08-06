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

import (
	"fmt"
	"sort"
	"strings"
)

// ToolIdentitySpec contains only command identity. It is intentionally
// separate from interface identity: an executable Cobra leaf and an RPC are
// related by InterfaceSpec, but are not interchangeable sources of truth.
type ToolIdentitySpec struct {
	ProductID       string
	SourceProductID string
	Name            string
	CLIName         string
	CanonicalPath   string
	Path            string
	CLIPath         string
	PrimaryCLIPath  string
	Group           string
	Aliases         []string
	IsAlias         bool
	Source          string
}

// RuntimeSchemaPositional describes one ordered CLI argument. Name is also
// used by RuntimeSchemaConstraints when a one-of group mixes flags and args.
type RuntimeSchemaPositional struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Variadic    bool   `json:"variadic,omitempty"`
	Index       int    `json:"index"`
}

// DryRunSpec is a positive capability declaration. A nil ToolSpec.DryRun
// means the command has not declared reviewed --dry-run support; the Schema
// does not publish a negative or inferred capability in that case.
//
// The whole object is one atomic contract field. Runtime execution remains
// owned by the command runner; Schema only projects the reviewed capability.
type DryRunSpec struct {
	PreviewKind string `json:"preview_kind"`
	RemoteReads bool   `json:"remote_reads,omitempty"`
}

const (
	DryRunPreviewInvocation = "invocation"
	DryRunPreviewRequest    = "request"
	DryRunPreviewPlan       = "plan"
	DryRunPreviewDiff       = "diff"
)

// Validate checks preview_kind against the closed reviewed set.
func (d DryRunSpec) Validate(canonical string) error {
	canonical = defaultString(strings.TrimSpace(canonical), "<unknown>")
	switch strings.TrimSpace(d.PreviewKind) {
	case DryRunPreviewInvocation, DryRunPreviewRequest, DryRunPreviewPlan, DryRunPreviewDiff:
		return nil
	case "":
		return fmt.Errorf("schema tool %s dry_run has no preview_kind", canonical)
	default:
		return fmt.Errorf("schema tool %s dry_run has unknown preview_kind %q", canonical, d.PreviewKind)
	}
}

// SafetySpec is the resolved operation behavior. This model deliberately does
// not impose a value lattice: precedence policy belongs to the resolver, so a
// reviewed higher-priority source may intentionally raise or lower a value.
type SafetySpec struct {
	Effect       string
	EffectSource string
	Risk         string
	Confirmation string
	Idempotency  string
}

// InterfaceRefSpec identifies the backing operation, independently from the
// executable command identity.
type InterfaceRefSpec struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

// InterfaceSpec describes whether and how the Agent may invoke the backing
// interface.
type InterfaceSpec struct {
	Ref          *InterfaceRefSpec
	Mode         string
	Availability string
	Reason       string
}

const (
	InterfaceModeMCP       = "mcp"
	InterfaceModeLocal     = "local"
	InterfaceModeComposite = "composite"

	InterfaceAvailable   = "available"
	InterfaceUnavailable = "unavailable"
)

// AgentExecutable reports whether the final contract permits an Agent to
// invoke this command. Interface mode describes the implementation mechanism;
// availability is the independent execution gate.
func (i InterfaceSpec) AgentExecutable() bool {
	if strings.TrimSpace(i.Availability) != InterfaceAvailable {
		return false
	}
	switch strings.TrimSpace(i.Mode) {
	case InterfaceModeMCP:
		return i.Ref != nil
	case InterfaceModeLocal:
		return i.Ref == nil
	case InterfaceModeComposite:
		return i.Ref == nil && strings.TrimSpace(i.Reason) != ""
	default:
		return false
	}
}

// Validate enforces the final interface-disposition conflict matrix. It does
// not prove that an MCP ref exists in the pinned interface registry; that
// exact lookup is performed by validateSchemaRegistryInterfaces.
func (i InterfaceSpec) Validate(canonical string) error {
	canonical = defaultString(strings.TrimSpace(canonical), "<unknown>")
	mode := strings.TrimSpace(i.Mode)
	availability := strings.TrimSpace(i.Availability)
	reason := strings.TrimSpace(i.Reason)

	if mode == InterfaceUnavailable {
		return fmt.Errorf("schema tool %s uses legacy interface_mode=unavailable; migrate to interface_mode=mcp, local, or composite with availability=unavailable", canonical)
	}
	switch mode {
	case InterfaceModeMCP, InterfaceModeLocal, InterfaceModeComposite:
	case "":
		return fmt.Errorf("schema tool %s has no interface mode", canonical)
	default:
		return fmt.Errorf("schema tool %s has unknown interface mode %q", canonical, mode)
	}
	switch availability {
	case InterfaceAvailable:
	case InterfaceUnavailable:
		if i.Ref != nil {
			return fmt.Errorf("schema tool %s with unavailable interface must not declare interface_ref", canonical)
		}
		if reason == "" {
			return fmt.Errorf("schema tool %s with unavailable interface must declare interface_reason", canonical)
		}
		return nil
	case "":
		return fmt.Errorf("schema tool %s has no interface availability", canonical)
	default:
		return fmt.Errorf("schema tool %s has unknown interface availability %q", canonical, availability)
	}

	switch mode {
	case InterfaceModeMCP:
		if i.Ref == nil {
			return fmt.Errorf("schema tool %s with interface mode mcp has no interface_ref", canonical)
		}
	case InterfaceModeLocal:
		if i.Ref != nil {
			return fmt.Errorf("schema tool %s with interface mode local must not declare interface_ref", canonical)
		}
	case InterfaceModeComposite:
		if i.Ref != nil {
			return fmt.Errorf("schema tool %s with interface mode composite must not declare a single interface_ref", canonical)
		}
		if reason == "" {
			return fmt.Errorf("schema tool %s with interface mode composite must declare interface_reason", canonical)
		}
	}
	return nil
}

// SelectionSpec contains Agent command-selection guidance. Product specs use
// the common summary/use/avoid/source subset; tool specs may use every field.
type SelectionSpec struct {
	AgentSummary       string
	AgentSummarySource string
	UseWhen            []string
	AvoidWhen          []string
	Prerequisites      []string
	Tips               []string
	WorkflowRefs       []string
	Examples           []string
	// ExampleDispositions narrows an exact example with a reviewed local or
	// stateful precondition from dry-run execution to contract validation.
	// It does not change the command's declared DryRun capability.
	ExampleDispositions []ExampleDisposition
	// Reviewed is a legacy-path (hints/registry) marker only. The Contract
	// declaration path must not set it: declared selection is final by
	// construction, and assembly rejects a declared payload carrying it.
	Reviewed       *bool
	SourceRefs     []string
	MetadataSource string
}

// Normalized returns a copy with trimmed unique guidance arrays. Guidance and
// examples are ordered authoring content; determinism comes from sorted
// registry navigation, not from rewriting semantically meaningful arrays.
func (s SelectionSpec) Normalized() SelectionSpec {
	out := s
	out.UseWhen = stableUniqueStrings(s.UseWhen)
	out.AvoidWhen = stableUniqueStrings(s.AvoidWhen)
	out.Prerequisites = stableUniqueStrings(s.Prerequisites)
	out.Tips = stableUniqueStrings(s.Tips)
	out.WorkflowRefs = stableUniqueStrings(s.WorkflowRefs)
	out.Examples = stableUniqueStrings(s.Examples)
	out.ExampleDispositions = cloneExampleDispositions(s.ExampleDispositions)
	out.SourceRefs = sortedUniqueStrings(s.SourceRefs)
	return out
}

// ExampleDispositionMode controls how an already contract-validated example
// is exercised by the Agent example gate.
type ExampleDispositionMode string

const (
	ExampleDispositionModeContract     ExampleDispositionMode = "contract"
	ExampleDispositionModeDryRun       ExampleDispositionMode = "dry_run"
	ExampleDispositionModeContractOnly ExampleDispositionMode = "contract_only"
)

// ExampleDispositionReasonCode is the closed taxonomy for reviewed
// contract-only exceptions to an explicit dry-run capability.
type ExampleDispositionReasonCode string

const (
	ExampleDispositionReasonLocalState        ExampleDispositionReasonCode = "local_state"
	ExampleDispositionReasonStatefulPreflight ExampleDispositionReasonCode = "stateful_preflight"
)

// ExampleDisposition narrows one exact example to contract-only validation.
// Index is a pointer so a missing index cannot silently select example zero.
type ExampleDisposition struct {
	Index      *int                         `json:"index"`
	Mode       ExampleDispositionMode       `json:"mode"`
	ReasonCode ExampleDispositionReasonCode `json:"reason_code"`
	Reason     string                       `json:"reason"`
	Reviewed   bool                         `json:"reviewed"`
}

func cloneExampleDispositions(in []ExampleDisposition) []ExampleDisposition {
	if len(in) == 0 {
		return nil
	}
	out := append([]ExampleDisposition(nil), in...)
	for i := range out {
		if out[i].Index != nil {
			index := *out[i].Index
			out[i].Index = &index
		}
	}
	return out
}

// ParamDecl is one parameter-level Schema fact declared on a command. It is
// stored at DeclareLeafMetadata time and applied as annotations at assembly
// time, when all flags are guaranteed to exist on the fully-built command tree.
type ParamDecl struct {
	Name          string
	Property      string
	Required      *bool
	InterfaceType string
	Description   string
	RequiredWhen  string
	Enum          []string
}

func defaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func stableUniqueStrings(values []string) []string {
	if values == nil {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sortedUniqueStrings(values []string) []string {
	out := stableUniqueStrings(values)
	if out == nil {
		return nil
	}
	sort.Strings(out)
	return out
}
