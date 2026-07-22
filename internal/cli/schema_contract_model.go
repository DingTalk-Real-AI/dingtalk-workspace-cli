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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// SchemaRegistry is the resolved, typed Agent contract. Source adapters and
// precedence resolvers populate this model; wire payloads are rendered only at
// the outer boundary by ToPayload. This keeps map[string]any out of the
// contract assembly path.
type SchemaRegistry struct {
	Kind              string
	Level             string
	Source            string
	Products          []ProductSpec
	InterfaceMetadata json.RawMessage
	AgentMetadata     json.RawMessage
	Extensions        map[string]json.RawMessage
}

// ProductSpec is one deterministic product grouping in SchemaRegistry.
type ProductSpec struct {
	ID              string
	Name            string
	Description     string
	Runtime         bool
	Tools           []ToolSpec
	Selection       SelectionSpec
	FieldProvenance map[string]FieldProvenance
	Extensions      map[string]json.RawMessage
}

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

// ToolSpec is the single assembled representation of a command contract.
// Safety, interface and selection are typed sub-models even though ToPayload
// preserves the historical flat JSON shape for compatibility.
type ToolSpec struct {
	Identity        ToolIdentitySpec
	Display         string
	Title           string
	Description     string
	MetadataSource  string
	Parameters      []ParameterSpec
	Constraints     RuntimeSchemaConstraints
	Positionals     []RuntimeSchemaPositional
	DryRun          *DryRunSpec
	Safety          SafetySpec
	Interface       InterfaceSpec
	Selection       SelectionSpec
	FieldProvenance map[string]FieldProvenance
	Extensions      map[string]json.RawMessage
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

// ParameterSpec is one resolved CLI flag projection. Name is the CLI flag key
// used in the legacy parameters object. Default and Example are RawMessage
// because JSON literals are polymorphic; they are still typed JSON values and
// never participate in precedence as map[string]any.
type ParameterSpec struct {
	Name                 string
	Type                 string
	Description          string
	Property             string
	Required             bool
	CLIRequired          bool
	RequiredWhen         string
	Default              json.RawMessage
	InterfaceDefault     json.RawMessage
	Example              json.RawMessage
	Format               string
	Enum                 []string
	InterfaceDescription string
	InterfaceType        string
	FieldProvenance      map[string]FieldProvenance
	Extensions           map[string]json.RawMessage
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
	Reviewed           *bool
	SourceRefs         []string
	MetadataSource     string
}

// FieldProvenance records how one final field was selected. Value is raw JSON
// so provenance can describe strings, booleans and structured extension
// values without weakening the resolved ToolSpec itself.
type FieldProvenance struct {
	Value                json.RawMessage            `json:"value,omitempty"`
	Source               string                     `json:"source"`
	SourceRef            string                     `json:"source_ref,omitempty"`
	Precedence           string                     `json:"precedence,omitempty"`
	Resolution           string                     `json:"resolution"`
	ReviewReason         string                     `json:"review_reason,omitempty"`
	Candidates           []FieldCandidateProvenance `json:"candidates,omitempty"`
	OverriddenCandidates []FieldCandidateProvenance `json:"overridden_candidates,omitempty"`
}

// FieldCandidateProvenance retains one winning or non-winning source value.
type FieldCandidateProvenance struct {
	Value        json.RawMessage `json:"value,omitempty"`
	Source       string          `json:"source"`
	SourceRef    string          `json:"source_ref,omitempty"`
	Precedence   string          `json:"precedence,omitempty"`
	ReviewReason string          `json:"review_reason,omitempty"`
	Selected     *bool           `json:"selected,omitempty"`
}

// Final provenance coverage is an explicit contract, not a truthiness check.
// These fields all pass through a precedence resolver and therefore must keep
// a winner even when the delivered value is false, "", [], or null.
var requiredToolProvenanceFields = [...]string{
	"canonical_path",
	"effect",
	"risk",
	"confirmation",
	"idempotency",
	"interface_ref",
	"interface_mode",
	"availability",
	"agent_summary",
}

var conditionalSelectionProvenanceFields = [...]string{
	"use_when",
	"avoid_when",
	"prerequisites",
	"tips",
	"workflow_refs",
	"examples",
}

var requiredParameterProvenanceFields = [...]string{
	"property",
	"type",
	"description",
	"required",
	"required_when",
}

// RuntimeToolSpecInput is the typed hand-off from runtime source resolution to
// contract assembly. Adapters resolve candidates first, then call
// ToolSpecFromRuntime exactly once; payload rendering does no further merging.
type RuntimeToolSpecInput struct {
	Identity        ToolIdentitySpec
	Display         string
	Title           string
	Description     string
	MetadataSource  string
	Parameters      []ParameterSpec
	Constraints     RuntimeSchemaConstraints
	Positionals     []RuntimeSchemaPositional
	DryRun          *DryRunSpec
	Safety          SafetySpec
	Interface       InterfaceSpec
	Selection       SelectionSpec
	FieldProvenance map[string]FieldProvenance
	Extensions      map[string]json.RawMessage
}

// SchemaIndex is the immutable navigation view over one normalized registry.
// It mirrors Lark's typed catalog split: SchemaRegistry owns data, SchemaIndex
// owns deterministic identity/path resolution, and serializers only render a
// resolved ToolSpec.
type SchemaIndex struct {
	registry    SchemaRegistry
	byProduct   map[string]int
	byCanonical map[string]schemaToolLocation
	byToolPath  map[string]string
	byCLIPath   map[string]string
}

type schemaToolLocation struct {
	product int
	tool    int
}

// SchemaSnapshotPayload is the final wire boundary for the release snapshot.
// Catalog summaries and full Tools are both derived from the same typed
// ToolSpec instances, preventing the two snapshot layers from drifting.
type SchemaSnapshotPayload struct {
	Catalog map[string]any
	Tools   map[string]map[string]any
}

var (
	snapshotToolSummary    = ToolSpec.ToSummaryPayload
	snapshotToolPayload    = ToolSpec.ToPayload
	snapshotProductSummary = ProductSpec.ToSummaryPayload
)

// SchemaRegistryFromRuntime constructs and validates the one registry shared
// by leaf lookup, Catalog summaries and full export.
func SchemaRegistryFromRuntime(source string, products []ProductSpec) (SchemaRegistry, error) {
	registry := SchemaRegistry{
		Kind:     "schema",
		Level:    "catalog",
		Source:   strings.TrimSpace(source),
		Products: products,
	}.Sorted()
	if _, err := registry.Index(); err != nil {
		return SchemaRegistry{}, err
	}
	return registry, nil
}

// ToolSpecFromRuntime validates and normalizes the fully resolved runtime
// input. It does not choose between sources; that is the resolver's job.
func ToolSpecFromRuntime(input RuntimeToolSpecInput) (ToolSpec, error) {
	return toolSpecFromResolvedInput(input)
}

// toolSpecFromSnapshot validates a delivered winner exactly as serialized. It
// deliberately does not repair provenance: a snapshot whose winner differs
// from the final field must fail closed in the production loader.
func toolSpecFromSnapshot(input RuntimeToolSpecInput) (ToolSpec, error) {
	return toolSpecFromResolvedInput(input)
}

func toolSpecFromResolvedInput(input RuntimeToolSpecInput) (ToolSpec, error) {
	spec := ToolSpec(input).normalized()
	if err := spec.Validate(); err != nil {
		return ToolSpec{}, err
	}
	return spec, nil
}

func (t ToolSpec) provenanceValue(field string) (any, bool) {
	switch field {
	case "canonical_path":
		return t.Identity.CanonicalPath, true
	case "title":
		return t.Title, true
	case "description":
		return t.Description, true
	case "metadata_source":
		return t.MetadataSource, true
	case "dry_run":
		return t.DryRun, true
	case "effect":
		return t.Safety.Effect, true
	case "effect_source":
		return t.Safety.EffectSource, true
	case "risk":
		return t.Safety.Risk, true
	case "confirmation":
		return t.Safety.Confirmation, true
	case "idempotency":
		return t.Safety.Idempotency, true
	case "interface_ref":
		return t.Interface.Ref, true
	case "interface_mode":
		return t.Interface.Mode, true
	case "availability":
		return t.Interface.Availability, true
	case "interface_reason":
		return t.Interface.Reason, true
	case "agent_summary":
		return t.Selection.AgentSummary, true
	case "use_when":
		return t.Selection.UseWhen, true
	case "avoid_when":
		return t.Selection.AvoidWhen, true
	case "prerequisites":
		return t.Selection.Prerequisites, true
	case "tips":
		return t.Selection.Tips, true
	case "workflow_refs":
		return t.Selection.WorkflowRefs, true
	case "examples":
		return t.Selection.Examples, true
	case "reviewed":
		return t.Selection.Reviewed, true
	default:
		return nil, false
	}
}

func (p ParameterSpec) provenanceValue(field string) (any, bool) {
	switch field {
	case "name":
		return p.Name, true
	case "type":
		return p.Type, true
	case "description":
		return p.Description, true
	case "property":
		return p.Property, true
	case "required":
		return p.Required, true
	case "cli_required":
		return p.CLIRequired, true
	case "required_when":
		return p.RequiredWhen, true
	case "default":
		return p.Default, true
	case "interface_default":
		return p.InterfaceDefault, true
	case "example":
		return p.Example, true
	case "format":
		return p.Format, true
	case "enum":
		return p.Enum, true
	case "interface_description":
		return p.InterfaceDescription, true
	case "interface_type":
		return p.InterfaceType, true
	default:
		return nil, false
	}
}

// Index validates all registry identities and returns deterministic navigation
// indexes. Canonical paths, primary CLI paths and reviewed aliases must each
// resolve unambiguously.
func (r SchemaRegistry) Index() (SchemaIndex, error) {
	r = r.Sorted()
	index := SchemaIndex{
		registry:    r,
		byProduct:   make(map[string]int, len(r.Products)),
		byCanonical: make(map[string]schemaToolLocation),
		byToolPath:  make(map[string]string),
		byCLIPath:   make(map[string]string),
	}
	for productIndex, product := range r.Products {
		if product.ID == "" {
			return SchemaIndex{}, fmt.Errorf("schema product id is empty")
		}
		if _, exists := index.byProduct[product.ID]; exists {
			return SchemaIndex{}, fmt.Errorf("duplicate schema product %q", product.ID)
		}
		index.byProduct[product.ID] = productIndex
		for field, provenance := range product.FieldProvenance {
			if value, ok := product.provenanceValue(field); ok {
				if err := validateFinalFieldProvenance("product "+product.ID, field, provenance, value); err != nil {
					return SchemaIndex{}, err
				}
			}
		}
		for toolIndex, tool := range product.Tools {
			if err := tool.Validate(); err != nil {
				return SchemaIndex{}, err
			}
			if err := validateCanonicalToolIdentity(tool); err != nil {
				return SchemaIndex{}, err
			}
			if tool.Identity.ProductID != product.ID {
				return SchemaIndex{}, fmt.Errorf("tool %s belongs to product %q, not containing product %q", tool.Identity.CanonicalPath, tool.Identity.ProductID, product.ID)
			}
			canonical := tool.Identity.CanonicalPath
			if _, exists := index.byCanonical[canonical]; exists {
				return SchemaIndex{}, fmt.Errorf("duplicate schema tool %q", canonical)
			}
			if existing, exists := index.byToolPath[canonical]; exists && existing != canonical {
				return SchemaIndex{}, fmt.Errorf("canonical path %q conflicts with contract path for %s", canonical, existing)
			}
			index.byCanonical[canonical] = schemaToolLocation{product: productIndex, tool: toolIndex}
			toolPaths := []string{tool.Identity.Path}
			if tool.Identity.SourceProductID != "" && tool.Identity.SourceProductID != tool.Identity.ProductID {
				toolPaths = append(toolPaths, tool.Identity.SourceProductID+"."+tool.Identity.Name)
			}
			for _, path := range sortedUniqueStrings(toolPaths) {
				if path == canonical {
					continue
				}
				if existing, exists := index.byToolPath[path]; exists && existing != canonical {
					return SchemaIndex{}, fmt.Errorf("schema contract path %q resolves to both %s and %s", path, existing, canonical)
				}
				if _, exists := index.byCanonical[path]; exists {
					return SchemaIndex{}, fmt.Errorf("schema contract path %q conflicts with a canonical path", path)
				}
				index.byToolPath[path] = canonical
			}
			paths := append([]string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}, tool.Identity.Aliases...)
			for _, path := range sortedUniqueStrings(paths) {
				path = normalizeSchemaCLIPath(path)
				if existing, exists := index.byCLIPath[path]; exists && existing != canonical {
					return SchemaIndex{}, fmt.Errorf("schema CLI path %q resolves to both %s and %s", path, existing, canonical)
				}
				index.byCLIPath[path] = canonical
			}
		}
	}
	return index, nil
}

// validateCanonicalToolIdentity applies only to ToolSpecs stored in the
// registry. Alias lookup creates a detached query view later, where cli_path
// and is_alias intentionally change; that projection is never allowed back
// into SchemaRegistry, --all, or the embedded Catalog.
func validateCanonicalToolIdentity(tool ToolSpec) error {
	id := tool.Identity
	canonical := strings.TrimSpace(id.CanonicalPath)
	cliPath := normalizeSchemaCLIPath(id.CLIPath)
	primaryCLIPath := normalizeSchemaCLIPath(id.PrimaryCLIPath)
	if cliPath != primaryCLIPath {
		return fmt.Errorf("canonical Schema tool %s cli_path %q must equal primary_cli_path %q", canonical, cliPath, primaryCLIPath)
	}
	if id.IsAlias {
		return fmt.Errorf("canonical Schema tool %s must have is_alias=false", canonical)
	}
	return nil
}

// Registry returns the detached normalized registry owned by this index.
func (i SchemaIndex) Registry() SchemaRegistry { return i.registry }

// Product resolves one product by exact stable ID.
func (i SchemaIndex) Product(id string) (ProductSpec, bool) {
	position, ok := i.byProduct[strings.TrimSpace(id)]
	if !ok {
		return ProductSpec{}, false
	}
	return i.registry.Products[position], true
}

// Resolve resolves either an exact canonical path or a normalized CLI path.
// No inference or fuzzy matching occurs in this layer.
func (i SchemaIndex) Resolve(path string) (ToolSpec, bool) {
	path = strings.TrimSpace(path)
	canonical := path
	location, ok := i.byCanonical[canonical]
	if !ok {
		canonical, ok = i.byToolPath[path]
	}
	if !ok {
		canonical, ok = i.byCLIPath[normalizeSchemaCLIPath(path)]
		if !ok {
			return ToolSpec{}, false
		}
	}
	location = i.byCanonical[canonical]
	return i.registry.Products[location.product].Tools[location.tool], true
}

// ResolveQuery adds compatibility for dotted and slash-separated CLI paths at
// the user-facing query boundary. Resolve remains strict because Registry
// validation uses it to detect missing canonical identities without falling
// through to a similarly spelled CLI path.
func (i SchemaIndex) ResolveQuery(path string) (ToolSpec, bool) {
	if tool, ok := i.Resolve(path); ok {
		return tool, true
	}
	canonical, ok := i.byCLIPath[normalizeSchemaQueryCLIPath(path)]
	if !ok {
		return ToolSpec{}, false
	}
	location := i.byCanonical[canonical]
	return i.registry.Products[location.product].Tools[location.tool], true
}

// CanonicalPaths returns the complete tool identity set in stable order.
func (i SchemaIndex) CanonicalPaths() []string {
	paths := make([]string, 0, len(i.byCanonical))
	for path := range i.byCanonical {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// ToSnapshotPayload derives the progressive Catalog and full tool map from one
// normalized registry. This is the direct typed replacement for separately
// assembling runtimeToolSummary and runtimeToolPayload.
func (r SchemaRegistry) ToSnapshotPayload() (SchemaSnapshotPayload, error) {
	index, err := r.Index()
	if err != nil {
		return SchemaSnapshotPayload{}, err
	}
	r = index.Registry()
	products := make([]map[string]any, 0, len(r.Products))
	tools := make(map[string]map[string]any, len(index.byCanonical))
	for _, product := range r.Products {
		summaries := make([]map[string]any, 0, len(product.Tools))
		for _, tool := range product.Tools {
			summary, summaryErr := snapshotToolSummary(tool)
			if summaryErr != nil {
				return SchemaSnapshotPayload{}, summaryErr
			}
			full, fullErr := snapshotToolPayload(tool)
			if fullErr != nil {
				return SchemaSnapshotPayload{}, fullErr
			}
			summaries = append(summaries, summary)
			tools[tool.Identity.CanonicalPath] = full
		}
		productPayload, productErr := snapshotProductSummary(product)
		if productErr != nil {
			return SchemaSnapshotPayload{}, productErr
		}
		productPayload["tools"] = summaries
		productPayload["tool_count"] = len(summaries)
		products = append(products, productPayload)
	}
	catalog, err := extensionsPayload(r.Extensions)
	if err != nil {
		return SchemaSnapshotPayload{}, err
	}
	catalog["kind"] = defaultString(r.Kind, "schema")
	catalog["level"] = defaultString(r.Level, "catalog")
	catalog["count"] = len(products)
	catalog["tool_count"] = len(tools)
	catalog["products"] = products
	if r.Source != "" {
		catalog["source"] = r.Source
	}
	if err := putRawJSON(catalog, "interface_metadata", r.InterfaceMetadata); err != nil {
		return SchemaSnapshotPayload{}, fmt.Errorf("interface_metadata: %w", err)
	}
	if err := putRawJSON(catalog, "agent_metadata", r.AgentMetadata); err != nil {
		return SchemaSnapshotPayload{}, fmt.Errorf("agent_metadata: %w", err)
	}
	return SchemaSnapshotPayload{Catalog: catalog, Tools: tools}, nil
}

// Validate checks structural invariants only. It intentionally does not
// constrain precedence outcomes such as risk or required values.
func (t ToolSpec) Validate() error {
	id := t.Identity
	if id.ProductID == "" {
		return fmt.Errorf("tool product_id is empty")
	}
	if id.Name == "" {
		return fmt.Errorf("tool name is empty")
	}
	wantCanonical := id.ProductID + "." + id.Name
	if id.CanonicalPath != wantCanonical {
		return fmt.Errorf("tool canonical_path %q does not match %q", id.CanonicalPath, wantCanonical)
	}
	if id.CLIPath == "" {
		return fmt.Errorf("tool %s cli_path is empty", id.CanonicalPath)
	}
	seen := make(map[string]bool, len(t.Parameters))
	for _, parameter := range t.Parameters {
		if parameter.Name == "" {
			return fmt.Errorf("tool %s has parameter with empty name", id.CanonicalPath)
		}
		if seen[parameter.Name] {
			return fmt.Errorf("tool %s has duplicate parameter %q", id.CanonicalPath, parameter.Name)
		}
		seen[parameter.Name] = true
		if err := validateRawJSON(parameter.Default, "default", id.CanonicalPath, parameter.Name); err != nil {
			return err
		}
		if err := validateRawJSON(parameter.InterfaceDefault, "interface_default", id.CanonicalPath, parameter.Name); err != nil {
			return err
		}
		if err := validateRawJSON(parameter.Example, "example", id.CanonicalPath, parameter.Name); err != nil {
			return err
		}
	}
	if t.Interface.Ref != nil {
		if strings.TrimSpace(t.Interface.Ref.ProductID) == "" || strings.TrimSpace(t.Interface.Ref.RPCName) == "" {
			return fmt.Errorf("tool %s has incomplete interface_ref", id.CanonicalPath)
		}
	}
	if t.DryRun != nil {
		if err := t.DryRun.Validate(id.CanonicalPath); err != nil {
			return err
		}
	}
	if t.Interface.Mode != "" || t.Interface.Availability != "" || t.Interface.Reason != "" || t.Interface.Ref != nil {
		if err := t.Interface.Validate(id.CanonicalPath); err != nil {
			return err
		}
	}
	for field, provenance := range t.FieldProvenance {
		if value, ok := t.provenanceValue(field); ok {
			if err := validateFinalFieldProvenance(id.CanonicalPath, field, provenance, value); err != nil {
				return err
			}
		}
	}
	for _, parameter := range t.Parameters {
		for field, provenance := range parameter.FieldProvenance {
			if value, ok := parameter.provenanceValue(field); ok {
				if err := validateFinalFieldProvenance(id.CanonicalPath+" parameter "+parameter.Name, field, provenance, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateFinalFieldProvenance(owner, field string, provenance FieldProvenance, finalValue any) error {
	expected, err := json.Marshal(finalValue)
	if err != nil {
		return fmt.Errorf("%s field %s cannot encode final provenance value: %w", owner, field, err)
	}
	if len(provenance.Value) == 0 || !json.Valid(provenance.Value) || !equalJSONValues(provenance.Value, expected) {
		return fmt.Errorf("%s field %s provenance winner does not equal final value: winner=%s final=%s", owner, field, string(provenance.Value), string(expected))
	}
	if strings.TrimSpace(provenance.Source) == "" || strings.TrimSpace(provenance.Precedence) == "" || strings.TrimSpace(provenance.Resolution) == "" {
		return fmt.Errorf("%s field %s provenance winner is incomplete", owner, field)
	}
	if len(provenance.Candidates) == 0 {
		return fmt.Errorf("%s field %s provenance has no candidates", owner, field)
	}
	selected := 0
	for _, candidate := range provenance.Candidates {
		if len(candidate.Value) == 0 || !json.Valid(candidate.Value) {
			return fmt.Errorf("%s field %s has a provenance candidate with invalid value", owner, field)
		}
		if candidate.Selected == nil || !*candidate.Selected {
			continue
		}
		selected++
		if !json.Valid(candidate.Value) || !equalJSONValues(candidate.Value, expected) {
			return fmt.Errorf("%s field %s selected provenance candidate does not equal final value", owner, field)
		}
		if strings.TrimSpace(candidate.Source) != strings.TrimSpace(provenance.Source) ||
			strings.TrimSpace(candidate.Precedence) != strings.TrimSpace(provenance.Precedence) {
			return fmt.Errorf("%s field %s selected provenance candidate disagrees with winner", owner, field)
		}
	}
	for _, candidate := range provenance.OverriddenCandidates {
		if len(candidate.Value) == 0 || !json.Valid(candidate.Value) {
			return fmt.Errorf("%s field %s has an overridden provenance candidate with invalid value", owner, field)
		}
		if candidate.Selected != nil && *candidate.Selected {
			return fmt.Errorf("%s field %s has a selected overridden provenance candidate", owner, field)
		}
	}
	if selected != 1 {
		return fmt.Errorf("%s field %s provenance has %d selected candidates, want 1", owner, field, selected)
	}
	return nil
}

// equalJSONValues compares decoded JSON types and values while preserving the
// lexical representation of numbers through json.Number. Formatting and
// equivalent string escapes are intentionally ignored; 1 and 1.0 remain
// distinct provenance values.
func equalJSONValues(left, right []byte) bool {
	// Generated Schema values are already canonical JSON in the common path.
	// Avoid allocating two decoded object graphs for identical bytes; keep the
	// semantic decoder fallback for whitespace, escape, and representation
	// differences. json.Valid preserves the previous behavior for equal but
	// malformed input.
	if bytes.Equal(left, right) {
		return json.Valid(left)
	}
	decode := func(raw []byte) (any, bool) {
		if !json.Valid(raw) {
			return nil, false
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		_ = decoder.Decode(&value)
		return value, true
	}
	leftValue, leftOK := decode(left)
	rightValue, rightOK := decode(right)
	return leftOK && rightOK && reflect.DeepEqual(leftValue, rightValue)
}

func validateRawJSON(value json.RawMessage, field, canonical, parameter string) error {
	if len(value) == 0 || json.Valid(value) {
		return nil
	}
	return fmt.Errorf("tool %s parameter %q has invalid JSON %s", canonical, parameter, field)
}

// Sorted returns a detached, deterministic registry. Products are ordered by
// ID, tools by canonical path then CLI path, and parameters by CLI flag name.
func (r SchemaRegistry) Sorted() SchemaRegistry {
	out := r
	out.Products = append([]ProductSpec(nil), r.Products...)
	for i := range out.Products {
		out.Products[i] = out.Products[i].normalized()
	}
	sort.Slice(out.Products, func(i, j int) bool {
		return out.Products[i].ID < out.Products[j].ID
	})
	return out
}

func (p ProductSpec) normalized() ProductSpec {
	out := p
	out.ID = strings.TrimSpace(out.ID)
	out.Name = strings.TrimSpace(out.Name)
	out.Tools = append([]ToolSpec(nil), p.Tools...)
	for i := range out.Tools {
		out.Tools[i] = out.Tools[i].normalized()
	}
	sort.Slice(out.Tools, func(i, j int) bool {
		left, right := out.Tools[i].Identity, out.Tools[j].Identity
		if left.CanonicalPath != right.CanonicalPath {
			return left.CanonicalPath < right.CanonicalPath
		}
		return left.CLIPath < right.CLIPath
	})
	out.Selection = out.Selection.normalized()
	out.FieldProvenance = cloneFieldProvenance(out.FieldProvenance)
	return out
}

func (p ProductSpec) provenanceValue(field string) (any, bool) {
	switch field {
	case "agent_summary":
		return p.Selection.AgentSummary, true
	case "use_when":
		return p.Selection.UseWhen, true
	case "avoid_when":
		return p.Selection.AvoidWhen, true
	default:
		return nil, false
	}
}

func (t ToolSpec) normalized() ToolSpec {
	out := t
	id := out.Identity
	id.ProductID = strings.TrimSpace(id.ProductID)
	id.SourceProductID = strings.TrimSpace(id.SourceProductID)
	id.Name = strings.TrimSpace(id.Name)
	id.CLIName = strings.TrimSpace(id.CLIName)
	id.CanonicalPath = strings.TrimSpace(id.CanonicalPath)
	if id.CanonicalPath == "" && id.ProductID != "" && id.Name != "" {
		id.CanonicalPath = id.ProductID + "." + id.Name
	}
	id.Path = strings.TrimSpace(id.Path)
	if id.Path == "" {
		id.Path = id.CanonicalPath
	}
	id.CLIPath = normalizeSchemaCLIPath(id.CLIPath)
	id.PrimaryCLIPath = normalizeSchemaCLIPath(id.PrimaryCLIPath)
	if id.PrimaryCLIPath == "" {
		id.PrimaryCLIPath = id.CLIPath
	}
	id.Group = strings.TrimSpace(id.Group)
	id.Source = strings.TrimSpace(id.Source)
	id.Aliases = sortedUniqueStrings(id.Aliases)
	out.Identity = id
	out.Parameters = append([]ParameterSpec(nil), t.Parameters...)
	for i := range out.Parameters {
		out.Parameters[i] = out.Parameters[i].normalized()
	}
	sort.Slice(out.Parameters, func(i, j int) bool {
		return out.Parameters[i].Name < out.Parameters[j].Name
	})
	out.Constraints = normalizeRuntimeSchemaConstraints(t.Constraints)
	if t.DryRun != nil {
		dryRun := *t.DryRun
		dryRun.PreviewKind = strings.TrimSpace(dryRun.PreviewKind)
		out.DryRun = &dryRun
	}
	out.Positionals = append([]RuntimeSchemaPositional(nil), t.Positionals...)
	sort.Slice(out.Positionals, func(i, j int) bool {
		if out.Positionals[i].Index != out.Positionals[j].Index {
			return out.Positionals[i].Index < out.Positionals[j].Index
		}
		return out.Positionals[i].Name < out.Positionals[j].Name
	})
	out.Selection = out.Selection.normalized()
	return out
}

func (p ParameterSpec) normalized() ParameterSpec {
	out := p
	out.Name = strings.TrimSpace(out.Name)
	out.Type = strings.TrimSpace(out.Type)
	out.Property = strings.TrimSpace(out.Property)
	out.Enum = stableUniqueStrings(out.Enum)
	return out
}

func (s SelectionSpec) normalized() SelectionSpec {
	out := s
	// Guidance and examples are ordered authoring content. Preserve first-seen
	// order like Lark's typed affordance model; determinism comes from sorted
	// registry navigation, not from rewriting semantically meaningful arrays.
	out.UseWhen = stableUniqueStrings(s.UseWhen)
	out.AvoidWhen = stableUniqueStrings(s.AvoidWhen)
	out.Prerequisites = stableUniqueStrings(s.Prerequisites)
	out.Tips = stableUniqueStrings(s.Tips)
	out.WorkflowRefs = stableUniqueStrings(s.WorkflowRefs)
	out.Examples = stableUniqueStrings(s.Examples)
	out.SourceRefs = sortedUniqueStrings(s.SourceRefs)
	return out
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

// cloneOptionalStrings preserves the semantic difference between an omitted
// list (nil) and an explicitly authored empty list (non-nil, length zero).
// Selection precedence treats [] as a real winner, so that presence must
// survive the generated metadata adapter and every typed projection.
func cloneOptionalStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func sortedUniqueStrings(values []string) []string {
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
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// ToPayload renders the compatible flat schema payload after deterministically
// sorting the typed model.
func (r SchemaRegistry) ToPayload() (map[string]any, error) {
	index, err := r.Index()
	if err != nil {
		return nil, err
	}
	r = index.Registry()
	products := make([]map[string]any, 0, len(r.Products))
	toolCount := 0
	for _, product := range r.Products {
		payload, err := product.ToPayload()
		if err != nil {
			return nil, err
		}
		products = append(products, payload)
		toolCount += len(product.Tools)
	}
	payload, err := extensionsPayload(r.Extensions)
	if err != nil {
		return nil, err
	}
	payload["kind"] = defaultString(r.Kind, "schema")
	payload["level"] = defaultString(r.Level, "catalog")
	payload["count"] = len(products)
	payload["tool_count"] = toolCount
	payload["products"] = products
	if r.Source != "" {
		payload["source"] = r.Source
	}
	if err := putRawJSON(payload, "interface_metadata", r.InterfaceMetadata); err != nil {
		return nil, fmt.Errorf("interface_metadata: %w", err)
	}
	if err := putRawJSON(payload, "agent_metadata", r.AgentMetadata); err != nil {
		return nil, fmt.Errorf("agent_metadata: %w", err)
	}
	return payload, nil
}

// ToOverviewPayload renders the small first-hop product index directly from
// the resolved registry. It is a presentation projection only: it never reads
// Agent metadata or any other source after assembly.
func (r SchemaRegistry) ToOverviewPayload() (map[string]any, error) {
	index, err := r.Index()
	if err != nil {
		return nil, err
	}
	r = index.Registry()
	products := make([]map[string]any, 0, len(r.Products))
	toolCount := 0
	for _, product := range r.Products {
		entry := map[string]any{
			"id":          product.ID,
			"tool_count":  len(product.Tools),
			"schema_path": product.ID,
		}
		if summary := strings.TrimSpace(product.Selection.AgentSummary); summary != "" {
			entry["agent_summary"] = summary
		} else if len(product.Selection.UseWhen) > 0 {
			entry["use_when"] = []string{product.Selection.UseWhen[0]}
		} else if product.Description != "" {
			entry["description"] = product.Description
		}
		products = append(products, entry)
		toolCount += len(product.Tools)
	}
	payload := map[string]any{
		"kind":       defaultString(r.Kind, "schema"),
		"level":      "products",
		"count":      len(products),
		"tool_count": toolCount,
		"products":   products,
	}
	if r.Source != "" {
		payload["source"] = r.Source
	}
	if err := putRawJSON(payload, "interface_metadata", r.InterfaceMetadata); err != nil {
		return nil, fmt.Errorf("interface_metadata: %w", err)
	}
	if err := putRawJSON(payload, "agent_metadata", r.AgentMetadata); err != nil {
		return nil, fmt.Errorf("agent_metadata: %w", err)
	}
	return payload, nil
}

// ToPayload renders one product and its full tools.
func (p ProductSpec) ToPayload() (map[string]any, error) {
	p = p.normalized()
	tools := make([]map[string]any, 0, len(p.Tools))
	for _, tool := range p.Tools {
		payload, err := tool.ToPayload()
		if err != nil {
			return nil, err
		}
		tools = append(tools, payload)
	}
	payload, err := extensionsPayload(p.Extensions)
	if err != nil {
		return nil, err
	}
	payload["id"] = p.ID
	payload["name"] = p.Name
	payload["description"] = p.Description
	payload["tool_count"] = len(tools)
	payload["tools"] = tools
	if p.Runtime {
		payload["runtime"] = true
	}
	applySelectionPayload(payload, p.Selection, false)
	if len(p.FieldProvenance) > 0 {
		value, valueErr := typedJSONValue(p.FieldProvenance)
		if valueErr != nil {
			return nil, valueErr
		}
		payload["field_provenance"] = value
	}
	return payload, nil
}

// ToSummaryPayload renders one product with progressive tool summaries. Both
// this view and the full product payload are projections of the same ToolSpec
// slice; neither re-resolves annotations or metadata.
func (p ProductSpec) ToSummaryPayload() (map[string]any, error) {
	p = p.normalized()
	tools := make([]map[string]any, 0, len(p.Tools))
	for _, tool := range p.Tools {
		payload, err := tool.ToSummaryPayload()
		if err != nil {
			return nil, err
		}
		tools = append(tools, payload)
	}
	payload, err := extensionsPayload(p.Extensions)
	if err != nil {
		return nil, err
	}
	payload["id"] = p.ID
	payload["name"] = p.Name
	payload["description"] = p.Description
	payload["tool_count"] = len(tools)
	payload["tools"] = tools
	if p.Runtime {
		payload["runtime"] = true
	}
	applySelectionPayload(payload, p.Selection, false)
	if len(p.FieldProvenance) > 0 {
		value, valueErr := typedJSONValue(p.FieldProvenance)
		if valueErr != nil {
			return nil, valueErr
		}
		payload["field_provenance"] = value
	}
	return payload, nil
}

// ToPayload renders one full leaf contract in the existing flat JSON shape.
func (t ToolSpec) ToPayload() (map[string]any, error) {
	t = t.normalized()
	if err := t.Validate(); err != nil {
		return nil, err
	}
	payload, err := extensionsPayload(t.Extensions)
	if err != nil {
		return nil, err
	}
	id := t.Identity
	payload["name"] = id.Name
	payload["cli_name"] = id.CLIName
	payload["canonical_path"] = id.CanonicalPath
	payload["path"] = id.Path
	payload["cli_path"] = id.CLIPath
	payload["primary_cli_path"] = id.PrimaryCLIPath
	payload["is_alias"] = id.IsAlias
	payload["source"] = id.Source
	payload["product_id"] = id.ProductID
	payload["display"] = t.Display
	payload["title"] = t.Title
	payload["description"] = t.Description
	setOptionalString(payload, "source_product_id", id.SourceProductID)
	setOptionalString(payload, "group", id.Group)
	setOptionalString(payload, "metadata_source", t.MetadataSource)
	if len(id.Aliases) > 0 {
		payload["aliases"] = append([]string(nil), id.Aliases...)
	}
	parameters := make(map[string]any, len(t.Parameters))
	for _, parameter := range t.Parameters {
		entry, parameterErr := parameter.ToPayload()
		if parameterErr != nil {
			return nil, fmt.Errorf("render %s parameter %s: %w", id.CanonicalPath, parameter.Name, parameterErr)
		}
		parameters[parameter.Name] = entry
	}
	payload["parameters"] = parameters
	payload["has_parameters"] = len(parameters) > 0
	payload["parameter_count"] = len(parameters)
	if !runtimeSchemaConstraintsEmpty(t.Constraints) {
		value, _ := typedJSONValue(t.Constraints)
		payload["constraints"] = value
	}
	if len(t.Positionals) > 0 {
		value, _ := typedJSONValue(t.Positionals)
		payload["positionals"] = value
	}
	if t.DryRun != nil {
		value, _ := typedJSONValue(t.DryRun)
		payload["dry_run"] = value
	}
	applySafetyPayload(payload, t.Safety)
	applyInterfacePayload(payload, t.Interface)
	applySelectionPayload(payload, t.Selection, true)
	if len(t.FieldProvenance) > 0 {
		value, valueErr := typedJSONValue(t.FieldProvenance)
		if valueErr != nil {
			return nil, valueErr
		}
		payload["field_provenance"] = value
	}
	return payload, nil
}

// ToSummaryPayload projects a ToolSpec without parameters and leaf-only
// execution detail. It is derived from the same typed ToolSpec, never assembled
// independently.
func (t ToolSpec) ToSummaryPayload() (map[string]any, error) {
	payload, err := t.ToPayload()
	if err != nil {
		return nil, err
	}
	for _, key := range []string{
		"parameters", "has_parameters", "parameter_count", "constraints",
		"positionals", "examples", "effect_source", "agent_source_refs",
		"field_provenance", "path", "source", "product_id", "display", "is_alias",
	} {
		delete(payload, key)
	}
	return payload, nil
}

// ToPayload renders one parameter in the existing parameters.<flag> shape.
func (p ParameterSpec) ToPayload() (map[string]any, error) {
	p = p.normalized()
	payload, err := extensionsPayload(p.Extensions)
	if err != nil {
		return nil, err
	}
	payload["type"] = p.Type
	payload["description"] = p.Description
	payload["required"] = p.Required
	setOptionalString(payload, "property", p.Property)
	if p.CLIRequired {
		payload["cli_required"] = true
	}
	setOptionalString(payload, "required_when", p.RequiredWhen)
	if err := putRawJSON(payload, "default", p.Default); err != nil {
		return nil, err
	}
	if err := putRawJSON(payload, "interface_default", p.InterfaceDefault); err != nil {
		return nil, err
	}
	if err := putRawJSON(payload, "example", p.Example); err != nil {
		return nil, err
	}
	setOptionalString(payload, "format", p.Format)
	if len(p.Enum) > 0 {
		payload["enum"] = append([]string(nil), p.Enum...)
	}
	setOptionalString(payload, "interface_description", p.InterfaceDescription)
	setOptionalString(payload, "interface_type", p.InterfaceType)
	if len(p.FieldProvenance) > 0 {
		value, valueErr := typedJSONValue(p.FieldProvenance)
		if valueErr != nil {
			return nil, valueErr
		}
		payload["field_provenance"] = value
	}
	return payload, nil
}

func applySafetyPayload(payload map[string]any, safety SafetySpec) {
	setOptionalString(payload, "effect", safety.Effect)
	setOptionalString(payload, "effect_source", safety.EffectSource)
	setOptionalString(payload, "risk", safety.Risk)
	setOptionalString(payload, "confirmation", safety.Confirmation)
	setOptionalString(payload, "idempotency", safety.Idempotency)
}

func applyInterfacePayload(payload map[string]any, spec InterfaceSpec) {
	if spec.Ref != nil {
		payload["interface_ref"] = map[string]any{
			"product_id": spec.Ref.ProductID,
			"rpc_name":   spec.Ref.RPCName,
		}
	}
	setOptionalString(payload, "interface_mode", spec.Mode)
	setOptionalString(payload, "availability", spec.Availability)
	setOptionalString(payload, "interface_reason", spec.Reason)
}

func applySelectionPayload(payload map[string]any, selection SelectionSpec, full bool) {
	setOptionalString(payload, "agent_summary", selection.AgentSummary)
	setOptionalString(payload, "agent_summary_source", selection.AgentSummarySource)
	setOptionalStrings(payload, "use_when", selection.UseWhen)
	setOptionalStrings(payload, "avoid_when", selection.AvoidWhen)
	if full {
		setOptionalStrings(payload, "prerequisites", selection.Prerequisites)
		setOptionalStrings(payload, "tips", selection.Tips)
		setOptionalStrings(payload, "workflow_refs", selection.WorkflowRefs)
		setOptionalStrings(payload, "examples", selection.Examples)
		if selection.Reviewed != nil {
			payload["reviewed"] = *selection.Reviewed
		}
	}
	setOptionalStrings(payload, "agent_source_refs", selection.SourceRefs)
	setOptionalString(payload, "agent_metadata_source", selection.MetadataSource)
}

func setOptionalString(payload map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		payload[key] = value
	}
}

func setOptionalStrings(payload map[string]any, key string, values []string) {
	if values = stableUniqueStrings(values); values != nil {
		payload[key] = values
	}
}

func defaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func extensionsPayload(extensions map[string]json.RawMessage) (map[string]any, error) {
	payload := make(map[string]any, len(extensions))
	keys := make([]string, 0, len(extensions))
	for key := range extensions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := putRawJSON(payload, key, extensions[key]); err != nil {
			return nil, fmt.Errorf("extension %s: %w", key, err)
		}
	}
	return payload, nil
}

func putRawJSON(payload map[string]any, key string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	value, err := rawJSONValue(raw)
	if err != nil {
		return err
	}
	payload[key] = value
	return nil
}

func rawJSONValue(raw json.RawMessage) (any, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	_ = decoder.Decode(&value)
	return value, nil
}

func typedJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return rawJSONValue(data)
}
