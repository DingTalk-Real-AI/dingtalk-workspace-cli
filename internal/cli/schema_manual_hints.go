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
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const manualSchemaHintVersion = 1
const manualSchemaHintSchemaRef = "./schema_manual_hints.schema.json"

const (
	runtimeSchemaManualIdentityAnnotation  = "dws.schema.manual.identity"
	runtimeSchemaManualIdentityReason      = "dws.schema.manual.identity_reason"
	runtimeSchemaManualParameterAnnotation = "dws.schema.manual.parameter"
	runtimeSchemaManualReasonAnnotation    = "dws.schema.manual.reason"
)

//go:embed schema_hints/metadata/*.json
var embeddedMetadataFS embed.FS

//go:embed schema_hints/selection/*.json
var embeddedSelectionFS embed.FS

const (
	embeddedMetadataGlob  = "schema_hints/metadata/*.json"
	embeddedSelectionGlob = "schema_hints/selection/*.json"
)

// ManualSchemaHintSnapshot is the human-owned bridge from an existing
// public Cobra leaf to Schema. It cannot create commands, flags, exclusions, or
// interfaces: every referenced command and flag is checked against the live
// tree before any annotation is applied.
type ManualSchemaHintSnapshot struct {
	Schema     string                    `json:"$schema"`
	Version    int                       `json:"version"`
	Commands   []ManualSchemaCommandHint `json:"commands"`
	AgentHints ManualAgentHintSet        `json:"agent_hints"`
}

// ManualAgentHintSet keeps reviewed Agent-selection prose in the same manual
// source as command and parameter overrides without making that prose an
// identity annotation. Generators may read this section; they must never write
// it back or use it to change executable, parameter, safety, or interface
// facts.
type ManualAgentHintSet struct {
	Revisions map[string]ManualAgentHintRevision `json:"revisions,omitempty"`
	Products  map[string]ManualAgentProductHint  `json:"products,omitempty"`
	Tools     map[string]ManualAgentToolHint     `json:"tools,omitempty"`
}

// ManualAgentHintRevision records how one reviewed authoring batch was
// produced. Product and tool entries reference it by the enclosing map key so
// later focused revisions do not relabel unchanged prose.
type ManualAgentHintRevision struct {
	GeneratedBy   string `json:"generated_by"`
	Model         string `json:"model,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Reason        string `json:"reason"`
}

// ManualAgentProductHint defines product-level routing prose.
type ManualAgentProductHint struct {
	AgentSummary string   `json:"agent_summary"`
	UseWhen      []string `json:"use_when"`
	AvoidWhen    []string `json:"avoid_when"`
	Reviewed     bool     `json:"reviewed"`
	Revision     string   `json:"revision"`
	Reason       string   `json:"reason"`
	Evidence     []string `json:"evidence"`
}

// ManualAgentToolHint defines the reviewed prose used to select and
// demonstrate one existing effective command. ExampleDispositions contains
// only test eligibility exceptions; it cannot change identity, parameters,
// safety, or interface disposition.
type ManualAgentToolHint struct {
	AgentSummary        string                          `json:"agent_summary"`
	UseWhen             []string                        `json:"use_when"`
	AvoidWhen           []string                        `json:"avoid_when"`
	Examples            []string                        `json:"examples"`
	ExampleDispositions []ManualAgentExampleDisposition `json:"example_dispositions,omitempty"`
	Reviewed            bool                            `json:"reviewed"`
	Revision            string                          `json:"revision"`
	Reason              string                          `json:"reason"`
	Evidence            []string                        `json:"evidence"`
}

// ManualAgentExampleMode controls only how an already contract-validated
// example is exercised. Contract validation is the default. dry_run is used
// only when the final ToolSpec publishes an explicit reviewed capability;
// contract_only remains a precise reviewed exception for such a capability
// whose runtime preconditions cannot be exercised safely and deterministically
// in the isolated test process.
type ManualAgentExampleMode string

const (
	ManualAgentExampleModeContract     ManualAgentExampleMode = "contract"
	ManualAgentExampleModeDryRun       ManualAgentExampleMode = "dry_run"
	ManualAgentExampleModeContractOnly ManualAgentExampleMode = "contract_only"
)

// ManualAgentExampleReasonCode is a closed taxonomy for reviewed
// contract-only exceptions to an explicit dry-run capability. Safety and
// confirmation remain independently enforced and cannot be weakened through
// an example disposition. There is deliberately no generic "skip" value: a
// reviewer must identify the concrete precondition.
type ManualAgentExampleReasonCode string

const (
	ManualAgentExampleReasonLocalState        ManualAgentExampleReasonCode = "local_state"
	ManualAgentExampleReasonStatefulPreflight ManualAgentExampleReasonCode = "stateful_preflight"
)

// ManualAgentExampleDisposition narrows one exact example with an explicit
// typed dry-run capability to contract-only. Index is a pointer so a missing
// JSON field cannot silently select example zero.
type ManualAgentExampleDisposition struct {
	Index      *int                         `json:"index"`
	Mode       ManualAgentExampleMode       `json:"mode"`
	ReasonCode ManualAgentExampleReasonCode `json:"reason_code"`
	Reason     string                       `json:"reason"`
	Reviewed   bool                         `json:"reviewed"`
}

// ManualAgentExampleExecution is one resolved example and its effective test
// mode. Contract validation has already succeeded when this value is returned.
type ManualAgentExampleExecution struct {
	CanonicalPath string
	Index         int
	Example       string
	Mode          ManualAgentExampleMode
	DryRun        *DryRunSpec
	ReasonCode    ManualAgentExampleReasonCode
	Reason        string
	Source        ManualAgentExampleDispositionSource
}

// ManualAgentExampleDispositionSource distinguishes the normal typed-contract
// classification from narrow reviewed manual exceptions.
type ManualAgentExampleDispositionSource string

const (
	ManualAgentExampleDispositionDefault        ManualAgentExampleDispositionSource = "default"
	ManualAgentExampleDispositionReviewedManual ManualAgentExampleDispositionSource = "reviewed_manual"
)

// ManualAgentExampleExecutionPlan is a stable, typed report used by the
// exhaustive real-Cobra dry-run test. Counts are derived only from the final
// typed dry-run capability and committed reviewed dispositions; runtime
// failures can never add implicit skips.
type ManualAgentExampleExecutionPlan struct {
	Examples             []ManualAgentExampleExecution
	Total                int
	Contract             int
	DryRun               int
	ContractOnly         int
	ReviewedContractOnly int
	ContractOnlyByReason map[ManualAgentExampleReasonCode]int
}

// ManualSchemaCommandHint opts one exact existing Cobra leaf into
// Schema and optionally reviews its CLI-facing parameter projection.
type ManualSchemaCommandHint struct {
	CLIPath       string                               `json:"cli_path"`
	CanonicalPath string                               `json:"canonical_path"`
	Reason        string                               `json:"reason"`
	Reviewed      bool                                 `json:"reviewed"`
	Parameters    map[string]ManualSchemaParameterHint `json:"parameters,omitempty"`
}

// ManualSchemaParameterHint changes only Schema annotations on a real
// flag. Pointer fields distinguish an omitted override from an explicit false.
type ManualSchemaParameterHint struct {
	Description   *string `json:"description,omitempty"`
	Property      *string `json:"property,omitempty"`
	InterfaceType *string `json:"interface_type,omitempty"`
	Required      *bool   `json:"required,omitempty"`
	RequiredWhen  *string `json:"required_when,omitempty"`
}

// ManualSchemaHintReport records the exact reviewed inputs applied to
// a command tree. It is useful to generators and tests; no runtime discovery is
// performed.
type ManualSchemaHintReport struct {
	Commands   []string
	Parameters []string
}

var (
	manualSchemaHintsOnce     sync.Once
	manualSchemaHintsSnapshot ManualSchemaHintSnapshot
	manualSchemaHintsErr      error
	loadManualSchemaHints     = embeddedManualSchemaHints
	loadManualCommandRegistry = loadEmbeddedCommandRegistry
	marshalManualParameter    = json.Marshal
	applyManualParameter      = annotateManualSchemaParameter
)

// ApplyEmbeddedManualSchemaHints applies the committed human review
// file to an already-built Cobra tree. The operation is deterministic and
// idempotent.
func ApplyEmbeddedManualSchemaHints(root *cobra.Command) (ManualSchemaHintReport, error) {
	snapshot, err := loadManualSchemaHints()
	if err != nil {
		return ManualSchemaHintReport{}, err
	}
	return applyManualSchemaHints(root, snapshot)
}

func embeddedManualSchemaHints() (ManualSchemaHintSnapshot, error) {
	manualSchemaHintsOnce.Do(func() {
		manualSchemaHintsSnapshot, manualSchemaHintsErr = loadManualSchemaHintsFromHintDirs(
			embeddedMetadataFS, embeddedMetadataGlob,
			embeddedSelectionFS, embeddedSelectionGlob,
		)
	})
	return manualSchemaHintsSnapshot, manualSchemaHintsErr
}

func decodeManualSchemaHints(data []byte) (ManualSchemaHintSnapshot, error) {
	var snapshot ManualSchemaHintSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, fmt.Errorf("decode manual Schema hints: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return snapshot, fmt.Errorf("decode manual Schema hints: %w", err)
	}
	if snapshot.Version != manualSchemaHintVersion {
		return snapshot, fmt.Errorf("unsupported manual Schema hint version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Schema) != manualSchemaHintSchemaRef {
		return snapshot, fmt.Errorf("manual Schema hints must declare $schema=%q", manualSchemaHintSchemaRef)
	}
	if err := ValidateManualAgentHintSet(snapshot.AgentHints, nil, nil); err != nil {
		return snapshot, fmt.Errorf("validate manual Agent hints: %w", err)
	}
	return snapshot, nil
}

// DecodeManualSchemaHintSource strictly decodes the reviewed manual input for
// build-time generators and policy checks. Callers receive data only; this
// function never writes or normalizes the source file in place.
func DecodeManualSchemaHintSource(data []byte) (ManualSchemaHintSnapshot, error) {
	return decodeManualSchemaHints(data)
}

// ValidateManualAgentHintSet validates the reviewed Agent prose and, when
// expected sets are supplied, requires exact product and canonical-tool
// coverage. It is intentionally independent from generated Catalog data.
func ValidateManualAgentHintSet(hints ManualAgentHintSet, expectedProducts, expectedTools map[string]bool) error {
	if len(hints.Revisions) == 0 {
		return fmt.Errorf("agent_hints.revisions must not be empty")
	}
	for rawRevision, provenance := range hints.Revisions {
		revision := strings.TrimSpace(rawRevision)
		if revision == "" || revision != rawRevision {
			return fmt.Errorf("agent_hints has invalid revision key %q", rawRevision)
		}
		generatedBy := strings.TrimSpace(provenance.GeneratedBy)
		if generatedBy == "" || strings.TrimSpace(provenance.Reason) == "" {
			return fmt.Errorf("agent_hints revision %q requires generated_by and reason", revision)
		}
		if strings.EqualFold(generatedBy, "ai") && (strings.TrimSpace(provenance.Model) == "" || strings.TrimSpace(provenance.PromptVersion) == "") {
			return fmt.Errorf("AI agent_hints revision %q requires model and prompt_version", revision)
		}
	}

	for rawProductID, hint := range hints.Products {
		productID := strings.TrimSpace(rawProductID)
		if productID == "" || productID != rawProductID || strings.ContainsAny(productID, ". \t\r\n") {
			return fmt.Errorf("agent_hints has invalid product key %q", rawProductID)
		}
		if err := validateManualAgentHintFields("product "+productID, hint.AgentSummary, hint.UseWhen, hint.AvoidWhen, nil, hint.Reviewed, hint.Revision, hint.Reason, hint.Evidence, hints.Revisions); err != nil {
			return err
		}
	}
	for rawCanonical, hint := range hints.Tools {
		canonical := strings.TrimSpace(rawCanonical)
		if canonical != rawCanonical {
			return fmt.Errorf("agent_hints has invalid canonical tool key %q", rawCanonical)
		}
		if _, _, ok := splitManualSchemaCanonicalPath(canonical); !ok {
			return fmt.Errorf("agent_hints has invalid canonical tool key %q", rawCanonical)
		}
		if err := validateManualAgentHintFields("tool "+canonical, hint.AgentSummary, hint.UseWhen, hint.AvoidWhen, hint.Examples, hint.Reviewed, hint.Revision, hint.Reason, hint.Evidence, hints.Revisions); err != nil {
			return err
		}
		if len(hint.Examples) == 0 {
			return fmt.Errorf("agent_hints tool %s requires non-empty examples", canonical)
		}
		if len(hint.Examples) > 2 {
			return fmt.Errorf("agent_hints tool %s has %d examples; maximum is 2", canonical, len(hint.Examples))
		}
		for _, example := range hint.Examples {
			argv, err := tokenizeManualAgentExample(example)
			if err != nil {
				return fmt.Errorf("agent_hints tool %s example has invalid argv syntax: %w", canonical, err)
			}
			if len(argv) < 2 || argv[0] != "dws" {
				return fmt.Errorf("agent_hints tool %s example must start with dws: %q", canonical, example)
			}
			for _, argument := range argv[1:] {
				if argument == "--yes" || strings.HasPrefix(argument, "--yes=") {
					return fmt.Errorf("agent_hints tool %s example must not bypass confirmation with --yes", canonical)
				}
				if argument == "--help" || strings.HasPrefix(argument, "--help=") || argument == "-h" || strings.HasPrefix(argument, "-h=") {
					return fmt.Errorf("agent_hints tool %s example must demonstrate execution, not only --help", canonical)
				}
			}
		}
		if err := validateManualAgentExampleDispositions(canonical, hint.Examples, hint.ExampleDispositions); err != nil {
			return err
		}
	}

	if err := validateManualAgentHintExactSet("products", expectedProducts, mapKeysManualAgentProducts(hints.Products)); err != nil {
		return err
	}
	if err := validateManualAgentHintExactSet("tools", expectedTools, mapKeysManualAgentTools(hints.Tools)); err != nil {
		return err
	}
	return nil
}

func validateManualAgentExampleDispositions(canonical string, examples []string, dispositions []ManualAgentExampleDisposition) error {
	seen := make(map[int]bool, len(dispositions))
	for _, disposition := range dispositions {
		if disposition.Index == nil {
			return fmt.Errorf("agent_hints tool %s example disposition requires index", canonical)
		}
		index := *disposition.Index
		if index < 0 || index >= len(examples) {
			return fmt.Errorf("agent_hints tool %s example disposition index %d is out of range for %d examples", canonical, index, len(examples))
		}
		if seen[index] {
			return fmt.Errorf("agent_hints tool %s has duplicate example disposition index %d", canonical, index)
		}
		seen[index] = true
		if !disposition.Reviewed {
			return fmt.Errorf("agent_hints tool %s example disposition index %d must be reviewed", canonical, index)
		}
		if disposition.Mode != ManualAgentExampleModeContractOnly {
			return fmt.Errorf("agent_hints tool %s example disposition index %d has invalid mode %q; only %q may narrow an explicit dry_run capability", canonical, index, disposition.Mode, ManualAgentExampleModeContractOnly)
		}
		if !validManualAgentExampleReasonCode(disposition.ReasonCode) {
			return fmt.Errorf("agent_hints tool %s example disposition index %d has invalid reason_code %q", canonical, index, disposition.ReasonCode)
		}
		if strings.TrimSpace(disposition.Reason) == "" {
			return fmt.Errorf("agent_hints tool %s example disposition index %d requires a non-empty reason", canonical, index)
		}
	}
	return nil
}

func validManualAgentExampleReasonCode(code ManualAgentExampleReasonCode) bool {
	switch code {
	case ManualAgentExampleReasonLocalState,
		ManualAgentExampleReasonStatefulPreflight:
		return true
	default:
		return false
	}
}

func validateManualAgentHintFields(scope, summary string, useWhen, avoidWhen, examples []string, reviewed bool, revision, reason string, evidence []string, revisions map[string]ManualAgentHintRevision) error {
	if !reviewed {
		return fmt.Errorf("agent_hints %s must be reviewed", scope)
	}
	revision = strings.TrimSpace(revision)
	if _, ok := revisions[revision]; revision == "" || !ok {
		return fmt.Errorf("agent_hints %s references unknown revision %q", scope, revision)
	}
	if strings.TrimSpace(summary) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("agent_hints %s requires agent_summary and reason", scope)
	}
	for name, values := range map[string][]string{
		"use_when":   useWhen,
		"avoid_when": avoidWhen,
		"evidence":   evidence,
	} {
		if err := validateNonEmptyManualAgentStrings(scope, name, values); err != nil {
			return err
		}
	}
	if examples != nil {
		if err := validateNonEmptyManualAgentStrings(scope, "examples", examples); err != nil {
			return err
		}
	}
	return nil
}

func validateNonEmptyManualAgentStrings(scope, field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("agent_hints %s requires non-empty %s", scope, field)
	}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("agent_hints %s has an empty %s entry", scope, field)
		}
		if seen[value] {
			return fmt.Errorf("agent_hints %s has duplicate %s entry %q", scope, field, value)
		}
		seen[value] = true
	}
	return nil
}

func validateManualAgentHintExactSet(scope string, expected, actual map[string]bool) error {
	if expected == nil {
		return nil
	}
	missing := make([]string, 0)
	unexpected := make([]string, 0)
	for key, include := range expected {
		if include && !actual[key] {
			missing = append(missing, key)
		}
	}
	for key := range actual {
		if !expected[key] {
			unexpected = append(unexpected, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	if len(missing) != 0 || len(unexpected) != 0 {
		return fmt.Errorf("agent_hints %s do not exactly match EffectiveCommandRegistry: missing=%v unexpected=%v", scope, missing, unexpected)
	}
	return nil
}

func mapKeysManualAgentProducts(values map[string]ManualAgentProductHint) map[string]bool {
	keys := make(map[string]bool, len(values))
	for key := range values {
		keys[key] = true
	}
	return keys
}

func mapKeysManualAgentTools(values map[string]ManualAgentToolHint) map[string]bool {
	keys := make(map[string]bool, len(values))
	for key := range values {
		keys[key] = true
	}
	return keys
}

// ValidateManualAgentHintExamples is the pre-generation validation layer. It
// binds every authored example to the exact reviewed primary/alias path and
// rejects flags and required arguments not accepted by the live Cobra command.
// Final typed constraints, dry-run capability, and exact delivery coverage are
// validated later by BuildManualAgentExampleExecutionPlan after generated
// Agent metadata exists. It never executes an example.
func ValidateManualAgentHintExamples(bound BoundCommandRegistry, hints ManualAgentHintSet) error {
	_, err := buildManualAgentExampleExecutionPlan(bound, nil, hints)
	return err
}

// ValidateEmbeddedManualAgentExampleDelivery is the final generation gate. It
// validates the committed Manual Agent hints against the fully assembled typed
// SchemaRegistry, after generated Agent metadata has been compiled in. This is
// deliberately stronger than the pre-generation path/flag validation above.
func ValidateEmbeddedManualAgentExampleDelivery(bound BoundCommandRegistry, registry SchemaRegistry) (ManualAgentExampleExecutionPlan, error) {
	snapshot, err := loadManualSchemaHints()
	if err != nil {
		return ManualAgentExampleExecutionPlan{}, err
	}
	return BuildManualAgentExampleExecutionPlan(bound, registry, snapshot.AgentHints)
}

// BuildManualAgentExampleExecutionPlan validates every example against its
// real BoundCommand/Cobra contract. Runtime dry-run execution is opt-in and
// comes only from the final typed ToolSpec; a globally accepted --dry-run flag
// is not capability evidence. Only unavoidable local-state or stateful-
// preflight requirements may narrow an explicit capability through an exact
// reviewed manual disposition. Callers must never turn a runtime failure into
// an implicit skip.
func BuildManualAgentExampleExecutionPlan(bound BoundCommandRegistry, registry SchemaRegistry, hints ManualAgentHintSet) (ManualAgentExampleExecutionPlan, error) {
	tools := make(map[string]ToolSpec, len(bound.Commands))
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.Identity.CanonicalPath)
			if canonical == "" {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("typed SchemaRegistry contains a tool with empty canonical path")
			}
			if _, duplicate := tools[canonical]; duplicate {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("typed SchemaRegistry contains duplicate tool %q", canonical)
			}
			tools[canonical] = tool
		}
	}
	return buildManualAgentExampleExecutionPlan(bound, tools, hints)
}

func buildManualAgentExampleExecutionPlan(bound BoundCommandRegistry, typedTools map[string]ToolSpec, hints ManualAgentHintSet) (ManualAgentExampleExecutionPlan, error) {
	plan := ManualAgentExampleExecutionPlan{
		ContractOnlyByReason: map[ManualAgentExampleReasonCode]int{},
	}
	canonicalPaths := make([]string, 0, len(hints.Tools))
	for canonical := range hints.Tools {
		canonicalPaths = append(canonicalPaths, canonical)
	}
	sort.Strings(canonicalPaths)
	for _, canonical := range canonicalPaths {
		spec, ok := bound.ByCanonical[canonical]
		if !ok {
			return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints example references unknown canonical tool %q", canonical)
		}
		hint := hints.Tools[canonical]
		var typedTool ToolSpec
		if typedTools != nil {
			var found bool
			typedTool, found = typedTools[canonical]
			if !found {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints example tool %q is missing from final typed SchemaRegistry", canonical)
			}
		}
		if err := validateManualAgentExampleDispositions(canonical, hint.Examples, hint.ExampleDispositions); err != nil {
			return ManualAgentExampleExecutionPlan{}, err
		}
		dispositions := make(map[int]ManualAgentExampleDisposition, len(hint.ExampleDispositions))
		for _, disposition := range hint.ExampleDispositions {
			dispositions[*disposition.Index] = disposition
		}
		paths := []manualAgentExamplePath{{Path: spec.PrimaryCLIPath, Argv: strings.Fields(spec.PrimaryCLIPath), Command: spec.PrimaryCommand}}
		for _, alias := range spec.AliasCommands {
			paths = append(paths, manualAgentExamplePath{Path: alias.Path, Argv: strings.Fields(alias.Path), Command: alias.Command})
		}
		sort.Slice(paths, func(i, j int) bool {
			if len(paths[i].Argv) != len(paths[j].Argv) {
				return len(paths[i].Argv) > len(paths[j].Argv)
			}
			return paths[i].Path < paths[j].Path
		})
		for index, example := range hint.Examples {
			argv, err := tokenizeManualAgentExample(example)
			if err != nil {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example has invalid argv syntax: %w", canonical, err)
			}
			remainder, matched, ok := matchManualAgentExamplePath(argv, paths)
			if !ok {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example does not use its reviewed primary/alias path: %q", canonical, example)
			}
			if matched.Command == nil {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s reviewed path %q has no bound Cobra command", canonical, matched.Path)
			}
			constraints, err := strictCompatibilityConstraints(matched.Command, canonical)
			if err != nil {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example for %q has invalid executable constraints: %w", canonical, matched.Path, err)
			}
			if typedTools != nil {
				constraints.MutuallyExclusive = append(constraints.MutuallyExclusive, typedTool.Constraints.MutuallyExclusive...)
				constraints.RequireOneOf = append(constraints.RequireOneOf, typedTool.Constraints.RequireOneOf...)
				constraints.RequireTogether = append(constraints.RequireTogether, typedTool.Constraints.RequireTogether...)
				constraints = normalizeRuntimeSchemaConstraints(constraints)
			}
			positionals, err := strictCompatibilityPositionals(matched.Command)
			if err != nil {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example for %q has invalid executable positionals: %w", canonical, matched.Path, err)
			}
			if typedTools != nil {
				positionals = mergeManualAgentExamplePositionals(positionals, typedTool.Positionals)
			}
			if err := validateManualAgentExampleCobraContract(matched.Command, remainder, constraints, positionals); err != nil {
				return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example for %q: %w", canonical, matched.Path, err)
			}

			execution := ManualAgentExampleExecution{
				CanonicalPath: canonical,
				Index:         index,
				Example:       example,
				Mode:          ManualAgentExampleModeContract,
				Source:        ManualAgentExampleDispositionDefault,
			}
			if typedTool.DryRun != nil {
				dryRun := *typedTool.DryRun
				execution.DryRun = &dryRun
				execution.Mode = ManualAgentExampleModeDryRun
			}
			disposition, hasManualDisposition := dispositions[index]
			if hasManualDisposition {
				if typedTools != nil && execution.DryRun == nil {
					return ManualAgentExampleExecutionPlan{}, fmt.Errorf("agent_hints tool %s example disposition index %d narrows no explicit dry_run capability", canonical, index)
				}
				execution.Mode = disposition.Mode
				execution.ReasonCode = disposition.ReasonCode
				execution.Reason = strings.TrimSpace(disposition.Reason)
				execution.Source = ManualAgentExampleDispositionReviewedManual
				plan.ContractOnly++
				plan.ReviewedContractOnly++
				plan.ContractOnlyByReason[disposition.ReasonCode]++
			} else if execution.DryRun != nil {
				plan.DryRun++
			} else {
				plan.Contract++
			}
			plan.Examples = append(plan.Examples, execution)
			plan.Total++
		}
	}
	for canonical := range typedTools {
		if _, ok := hints.Tools[canonical]; !ok {
			return ManualAgentExampleExecutionPlan{}, fmt.Errorf("final typed SchemaRegistry tool %q has no Manual Agent hint examples", canonical)
		}
	}
	return plan, nil
}

type manualAgentExamplePath struct {
	Path    string
	Argv    []string
	Command *cobra.Command
}

func matchManualAgentExamplePath(argv []string, paths []manualAgentExamplePath) ([]string, manualAgentExamplePath, bool) {
	if len(argv) == 0 || argv[0] != "dws" {
		return nil, manualAgentExamplePath{}, false
	}
	for index := range paths {
		if paths[index].Argv == nil {
			paths[index].Argv = strings.Fields(strings.TrimSpace(paths[index].Path))
		}
		pathArgv := paths[index].Argv
		if len(pathArgv) == 0 || len(argv) < len(pathArgv)+1 {
			continue
		}
		matches := true
		for offset := range pathArgv {
			if argv[offset+1] != pathArgv[offset] {
				matches = false
				break
			}
		}
		if matches {
			return argv[len(pathArgv)+1:], paths[index], true
		}
	}
	return nil, manualAgentExamplePath{}, false
}

// tokenizeManualAgentExample parses one deliberately small, shell-free argv
// grammar. Quotes and backslash escaping are supported for readable values,
// while shell control operators, expansion, redirection, and the "--"
// terminator fail closed. Angle-bracket placeholders are data tokens, not
// redirection operators.
func tokenizeManualAgentExample(input string) ([]string, error) {
	var (
		argv         []string
		current      strings.Builder
		quote        byte
		tokenStarted bool
	)
	flush := func() {
		if tokenStarted {
			argv = append(argv, current.String())
			current.Reset()
			tokenStarted = false
		}
	}
	for index := 0; index < len(input); index++ {
		character := input[index]
		if quote != 0 {
			if character == quote {
				quote = 0
				continue
			}
			if quote == '"' && character == '\\' {
				if index+1 >= len(input) {
					return nil, fmt.Errorf("trailing escape in double-quoted value")
				}
				index++
				current.WriteByte(input[index])
				continue
			}
			if quote == '"' && (character == '`' || character == '$') {
				return nil, fmt.Errorf("shell expansion is not allowed")
			}
			current.WriteByte(character)
			continue
		}

		switch character {
		case ' ', '\t':
			flush()
		case '\r', '\n':
			return nil, fmt.Errorf("unquoted newline shell operator is not allowed")
		case '\'', '"':
			quote = character
			tokenStarted = true
		case '\\':
			if index+1 >= len(input) {
				return nil, fmt.Errorf("trailing escape")
			}
			index++
			current.WriteByte(input[index])
			tokenStarted = true
		case '<':
			placeholder, next, ok := manualAgentPlaceholderAt(input, index)
			if !ok {
				return nil, fmt.Errorf("shell redirection operator %q is not allowed", string(character))
			}
			current.WriteString(placeholder)
			tokenStarted = true
			index = next
		case '>', ';', '|', '&', '(', ')':
			return nil, fmt.Errorf("shell operator %q is not allowed", string(character))
		case '`', '$':
			return nil, fmt.Errorf("shell expansion is not allowed")
		case '#':
			return nil, fmt.Errorf("shell comments are not allowed")
		default:
			current.WriteByte(character)
			tokenStarted = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted value")
	}
	flush()
	for _, argument := range argv {
		if argument == "--" {
			return nil, fmt.Errorf("the -- argument terminator is not allowed")
		}
	}
	return argv, nil
}

// ParseManualAgentExampleArgv exposes the same deliberately shell-free argv
// parser used by Manual Agent hint validation. Contract tests use it to run
// reviewed examples through Cobra without invoking a shell or maintaining a
// second parser with different escaping rules.
func ParseManualAgentExampleArgv(input string) ([]string, error) {
	return tokenizeManualAgentExample(input)
}

func manualAgentPlaceholderAt(input string, start int) (string, int, bool) {
	endOffset := strings.IndexByte(input[start+1:], '>')
	if endOffset < 1 {
		return "", start, false
	}
	end := start + 1 + endOffset
	body := input[start+1 : end]
	for index := 0; index < len(body); index++ {
		character := body[index]
		if strings.ContainsRune(" \t\r\n<>;&|`$()#'\"\\", rune(character)) {
			return "", start, false
		}
	}
	return input[start : end+1], end, true
}

func validateManualAgentExampleCobraContract(command *cobra.Command, arguments []string, constraints RuntimeSchemaConstraints, positionalSpecs []RuntimeSchemaPositional) error {
	if command == nil {
		return fmt.Errorf("bound Cobra command is nil")
	}
	providedFacts := map[string]bool{}
	positionals := make([]string, 0)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return fmt.Errorf("the -- argument terminator is not allowed")
		}
		if strings.HasPrefix(argument, "--") {
			nameAndValue := strings.TrimPrefix(argument, "--")
			name, _, hasValue := strings.Cut(nameAndValue, "=")
			if name == "" {
				return fmt.Errorf("invalid empty long flag")
			}
			if name == "help" {
				return fmt.Errorf("must demonstrate execution, not only --help")
			}
			flag := runtimeCommandFlag(command, name)
			if flag == nil {
				return fmt.Errorf("uses unknown flag --%s", name)
			}
			providedFacts[flag.Name] = true
			if !hasValue && flag.NoOptDefVal == "" {
				if index+1 >= len(arguments) {
					return fmt.Errorf("flag --%s requires a value", name)
				}
				index++
			}
			continue
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			positionals = append(positionals, argument)
			continue
		}

		shorthandsAndValue := strings.TrimPrefix(argument, "-")
		shorthands, _, hasExplicitValue := strings.Cut(shorthandsAndValue, "=")
		for offset := 0; offset < len(shorthands); offset++ {
			shorthand := shorthands[offset : offset+1]
			if shorthand[0] >= 0x80 {
				return fmt.Errorf("uses invalid non-ASCII shorthand flag")
			}
			if shorthand == "h" {
				return fmt.Errorf("must demonstrate execution, not only -h")
			}
			flag := runtimeCommandFlagByShorthand(command, shorthand)
			if flag == nil {
				return fmt.Errorf("uses unknown shorthand flag -%s", shorthand)
			}
			providedFacts[flag.Name] = true
			if flag.NoOptDefVal == "" {
				if offset+1 < len(shorthands) || hasExplicitValue {
					break
				}
				if index+1 >= len(arguments) {
					return fmt.Errorf("shorthand flag -%s requires a value", shorthand)
				}
				index++
				break
			}
		}
	}
	missingRequired := make([]string, 0)
	visitManualAgentCommandFlags(command, func(flag *pflag.Flag) {
		if flag != nil && len(flag.Annotations[cobra.BashCompOneRequiredFlag]) > 0 && !providedFacts[flag.Name] {
			missingRequired = append(missingRequired, "--"+flag.Name)
		}
	})
	if len(missingRequired) != 0 {
		sort.Strings(missingRequired)
		return fmt.Errorf("missing required flag(s): %s", strings.Join(missingRequired, ", "))
	}
	missingPositionals := make([]string, 0)
	for _, positional := range positionalSpecs {
		provided := positional.Index >= 0 && positional.Index < len(positionals)
		if positional.Variadic {
			provided = positional.Index >= 0 && len(positionals) > positional.Index
		}
		if provided && strings.TrimSpace(positional.Name) != "" {
			providedFacts[strings.TrimLeft(strings.TrimSpace(positional.Name), "-")] = true
		}
		if positional.Required && !provided {
			missingPositionals = append(missingPositionals, strings.TrimSpace(positional.Name))
		}
	}
	if len(missingPositionals) != 0 {
		sort.Strings(missingPositionals)
		return fmt.Errorf("missing required positional argument(s): %s", strings.Join(missingPositionals, ", "))
	}
	if err := validateManualAgentExampleConstraints(providedFacts, constraints); err != nil {
		return err
	}
	// Runtime positional annotations are the stable Cobra contract and can
	// express flag/positional alternatives. Do not call a closure-backed Args
	// validator in that case: invoking it without mutating the live FlagSet
	// would observe default captured flag values (for example pat chmod's
	// --products alternative) and report a false missing positional. Commands
	// without typed positional facts still use their real Cobra Args validator.
	if command.Args != nil && len(positionalSpecs) == 0 {
		if err := command.Args(command, positionals); err != nil {
			return fmt.Errorf("invalid positional arguments: %w", err)
		}
	}
	return nil
}

func mergeManualAgentExamplePositionals(groups ...[]RuntimeSchemaPositional) []RuntimeSchemaPositional {
	byIdentity := map[string]RuntimeSchemaPositional{}
	for _, group := range groups {
		for _, positional := range group {
			key := fmt.Sprintf("%d\x00%s", positional.Index, strings.TrimSpace(positional.Name))
			if _, exists := byIdentity[key]; !exists {
				byIdentity[key] = positional
			}
		}
	}
	result := make([]RuntimeSchemaPositional, 0, len(byIdentity))
	for _, positional := range byIdentity {
		result = append(result, positional)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Index != result[j].Index {
			return result[i].Index < result[j].Index
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func validateManualAgentExampleConstraints(provided map[string]bool, constraints RuntimeSchemaConstraints) error {
	for _, group := range constraints.RequireOneOf {
		if !manualAgentExampleAnyFlagProvided(provided, group) {
			return fmt.Errorf("missing require_one_of flags: %s", manualAgentExampleFlagGroup(group))
		}
	}
	for _, group := range constraints.RequireTogether {
		providedCount := manualAgentExampleProvidedFlagCount(provided, group)
		if providedCount != 0 && providedCount != len(group) {
			return fmt.Errorf("incomplete require_together flags: %s", manualAgentExampleFlagGroup(group))
		}
	}
	for _, group := range constraints.MutuallyExclusive {
		if manualAgentExampleProvidedFlagCount(provided, group) > 1 {
			return fmt.Errorf("mutually_exclusive flags used together: %s", manualAgentExampleFlagGroup(group))
		}
	}
	return nil
}

func manualAgentExampleAnyFlagProvided(provided map[string]bool, group []string) bool {
	return manualAgentExampleProvidedFlagCount(provided, group) > 0
}

func manualAgentExampleProvidedFlagCount(provided map[string]bool, group []string) int {
	count := 0
	seen := map[string]bool{}
	for _, raw := range group {
		name := strings.TrimLeft(strings.TrimSpace(raw), "-")
		if name != "" && !seen[name] && provided[name] {
			seen[name] = true
			count++
		}
	}
	return count
}

func manualAgentExampleFlagGroup(group []string) string {
	flags := make([]string, 0, len(group))
	for _, raw := range group {
		if name := strings.TrimLeft(strings.TrimSpace(raw), "-"); name != "" {
			flags = append(flags, "--"+name)
		}
	}
	sort.Strings(flags)
	return strings.Join(flags, ", ")
}

func visitManualAgentCommandFlags(command *cobra.Command, visit func(*pflag.Flag)) {
	seen := map[string]bool{}
	visitSet := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			if seen[flag.Name] {
				return
			}
			seen[flag.Name] = true
			visit(flag)
		})
	}
	visitSet(command.Flags())
	for current := command; current != nil; current = current.Parent() {
		visitSet(current.PersistentFlags())
	}
}

func runtimeCommandFlagByShorthand(command *cobra.Command, shorthand string) *pflag.Flag {
	if command == nil || len(shorthand) != 1 {
		return nil
	}
	if flag := command.Flags().ShorthandLookup(shorthand); flag != nil {
		return flag
	}
	for current := command; current != nil; current = current.Parent() {
		if flag := current.PersistentFlags().ShorthandLookup(shorthand); flag != nil {
			return flag
		}
	}
	return nil
}

type validatedManualSchemaHint struct {
	hint    ManualSchemaCommandHint
	command *cobra.Command
}

func applyManualSchemaHints(root *cobra.Command, snapshot ManualSchemaHintSnapshot) (ManualSchemaHintReport, error) {
	if root == nil {
		return ManualSchemaHintReport{}, fmt.Errorf("apply manual Schema hints: root is nil")
	}
	if snapshot.Version != manualSchemaHintVersion {
		return ManualSchemaHintReport{}, fmt.Errorf("unsupported manual Schema hint version %d", snapshot.Version)
	}
	reviewedRegistry, err := loadManualCommandRegistry()
	if err != nil {
		return ManualSchemaHintReport{}, fmt.Errorf("load reviewed Schema command registry: %w", err)
	}

	validated := make([]validatedManualSchemaHint, 0, len(snapshot.Commands))
	seenPaths := map[string]bool{}
	for _, raw := range snapshot.Commands {
		hint := raw
		hint.CLIPath = normalizeSchemaCLIPath(hint.CLIPath)
		hint.CanonicalPath = strings.TrimSpace(hint.CanonicalPath)
		hint.Reason = strings.TrimSpace(hint.Reason)
		if hint.CLIPath == "" || strings.ContainsAny(hint.CLIPath, "*?[]") {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint has invalid exact cli_path %q", raw.CLIPath)
		}
		if seenPaths[hint.CLIPath] {
			return ManualSchemaHintReport{}, fmt.Errorf("duplicate manual Schema hint for %q", hint.CLIPath)
		}
		seenPaths[hint.CLIPath] = true
		if !hint.Reviewed || hint.Reason == "" {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q is not reviewed or has no reason", hint.CLIPath)
		}
		_, _, ok := splitManualSchemaCanonicalPath(hint.CanonicalPath)
		if !ok {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q has invalid canonical_path %q", hint.CLIPath, hint.CanonicalPath)
		}
		match, resolveErr := resolveExactCobraPath(root, hint.CLIPath)
		if resolveErr != nil {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q cannot be resolved exactly: %w", hint.CLIPath, resolveErr)
		}
		command := match.Command
		if command == nil {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q does not resolve to an existing Cobra command", hint.CLIPath)
		}
		if !publicRunnableSchemaLeaf(command) {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q must target a public runnable Cobra leaf", hint.CLIPath)
		}
		if productID, toolName, _ := runtimeSchemaAnnotations(command); productID != "" || toolName != "" {
			existing := strings.Trim(strings.TrimSpace(productID)+"."+strings.TrimSpace(toolName), ".")
			if existing != hint.CanonicalPath {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q canonical path %q conflicts with existing canonical path %q", hint.CLIPath, hint.CanonicalPath, existing)
			}
		}
		if identity, ok := reviewedRegistry.ByCLIPath[hint.CLIPath]; ok && identity.CanonicalPath != hint.CanonicalPath {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q canonical path %q conflicts with reviewed CommandRegistry canonical path %q", hint.CLIPath, hint.CanonicalPath, identity.CanonicalPath)
		}
		if match.UsedAlias {
			namePath := normalizeSchemaCLIPath(strings.Join(commandPathParts(command), " "))
			identity, ok := reviewedRegistry.ByCLIPath[namePath]
			if !ok {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q uses a Cobra alias, but real command path %q is not present in reviewed CommandRegistry", hint.CLIPath, namePath)
			}
			if identity.CanonicalPath != hint.CanonicalPath {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q canonical path %q conflicts with real command path %q canonical path %q", hint.CLIPath, hint.CanonicalPath, namePath, identity.CanonicalPath)
			}
		}
		for flagName, parameter := range hint.Parameters {
			flagName = strings.TrimSpace(flagName)
			if flagName == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q contains an empty flag name", hint.CLIPath)
			}
			if runtimeCommandFlag(command, flagName) == nil {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q references missing flag --%s", hint.CLIPath, flagName)
			}
			if parameter.Description == nil && parameter.Property == nil && parameter.InterfaceType == nil && parameter.Required == nil && parameter.RequiredWhen == nil {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has no Schema overrides", hint.CLIPath, flagName)
			}
			if parameter.Description != nil && strings.TrimSpace(*parameter.Description) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty description override", hint.CLIPath, flagName)
			}
			if parameter.Property != nil && strings.TrimSpace(*parameter.Property) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty property override", hint.CLIPath, flagName)
			}
			if parameter.InterfaceType != nil && !supportedManualSchemaInterfaceType(*parameter.InterfaceType) {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has unsupported interface_type %q", hint.CLIPath, flagName, *parameter.InterfaceType)
			}
			if parameter.RequiredWhen != nil && strings.TrimSpace(*parameter.RequiredWhen) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty required_when override", hint.CLIPath, flagName)
			}
		}
		validated = append(validated, validatedManualSchemaHint{hint: hint, command: command})
	}

	report := ManualSchemaHintReport{}
	for _, item := range validated {
		annotateManualSchemaIdentity(item.command, item.hint.CanonicalPath, item.hint.Reason)
		report.Commands = append(report.Commands, item.hint.CLIPath)
		flagNames := make([]string, 0, len(item.hint.Parameters))
		for flagName := range item.hint.Parameters {
			flagNames = append(flagNames, flagName)
		}
		sort.Strings(flagNames)
		for _, flagName := range flagNames {
			parameter := item.hint.Parameters[flagName]
			if err := applyManualParameter(item.command, flagName, parameter, item.hint.Reason); err != nil {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s: %w", item.hint.CLIPath, flagName, err)
			}
			report.Parameters = append(report.Parameters, item.hint.CLIPath+" --"+flagName)
		}
	}
	sort.Strings(report.Commands)
	return report, nil
}

// annotateManualSchemaIdentity keeps reviewed implementation evidence in a
// source-owned annotation. It deliberately does not rewrite code-owned runtime
// annotations: EffectiveCommandRegistry owns the selected identity, and the
// Cobra binder requires every manual/native annotation to agree with it before
// retaining the evidence in provenance.
func annotateManualSchemaIdentity(cmd *cobra.Command, canonicalPath, reason string) {
	if cmd == nil {
		return
	}
	setRuntimeCommandAnnotation(cmd, runtimeSchemaManualIdentityAnnotation, strings.TrimSpace(canonicalPath))
	setRuntimeCommandAnnotation(cmd, runtimeSchemaManualIdentityReason, strings.TrimSpace(reason))
}

func runtimeManualSchemaIdentity(cmd *cobra.Command) (productID, toolName, reason string, ok bool) {
	if cmd == nil || cmd.Annotations == nil {
		return "", "", "", false
	}
	canonicalPath := strings.TrimSpace(cmd.Annotations[runtimeSchemaManualIdentityAnnotation])
	productID, toolName, ok = splitManualSchemaCanonicalPath(canonicalPath)
	if !ok {
		return "", "", "", false
	}
	reason = strings.TrimSpace(cmd.Annotations[runtimeSchemaManualIdentityReason])
	return productID, toolName, reason, true
}

// annotateManualSchemaParameter stores the reviewed value in a source-owned
// annotation instead of overwriting the annotations used by bindings, Cobra
// adapters, and code-owned runtime metadata. The renderer resolves these
// independent candidates field by field and can therefore retain provenance.
func annotateManualSchemaParameter(command *cobra.Command, flagName string, hint ManualSchemaParameterHint, reason string) error {
	flagName = strings.TrimSpace(flagName)
	if command == nil {
		return fmt.Errorf("command is nil")
	}
	if flagName == "" || runtimeCommandFlag(command, flagName) == nil {
		return fmt.Errorf("flag --%s is missing", flagName)
	}
	hint = normalizeManualSchemaParameterHint(hint)
	encoded, err := marshalManualParameter(hint)
	if err != nil {
		return fmt.Errorf("encode reviewed parameter: %w", err)
	}
	setRuntimeCommandAnnotation(command, runtimeManualSchemaParameterKey(runtimeSchemaManualParameterAnnotation, flagName), string(encoded))
	setRuntimeCommandAnnotation(command, runtimeManualSchemaParameterKey(runtimeSchemaManualReasonAnnotation, flagName), strings.TrimSpace(reason))
	return nil
}

func runtimeManualSchemaParameter(command *cobra.Command, flagName string) (ManualSchemaParameterHint, string, bool, error) {
	if command == nil || command.Annotations == nil {
		return ManualSchemaParameterHint{}, "", false, nil
	}
	flagName = strings.TrimSpace(flagName)
	raw := strings.TrimSpace(command.Annotations[runtimeManualSchemaParameterKey(runtimeSchemaManualParameterAnnotation, flagName)])
	if raw == "" {
		return ManualSchemaParameterHint{}, "", false, nil
	}
	var hint ManualSchemaParameterHint
	if err := json.Unmarshal([]byte(raw), &hint); err != nil {
		return ManualSchemaParameterHint{}, "", false, fmt.Errorf("decode reviewed manual parameter hint: %w", err)
	}
	reason := strings.TrimSpace(command.Annotations[runtimeManualSchemaParameterKey(runtimeSchemaManualReasonAnnotation, flagName)])
	return normalizeManualSchemaParameterHint(hint), reason, true, nil
}

func runtimeManualSchemaParameterKey(prefix, flagName string) string {
	return strings.TrimSpace(prefix) + "." + strings.TrimSpace(flagName)
}

func normalizeManualSchemaParameterHint(hint ManualSchemaParameterHint) ManualSchemaParameterHint {
	hint.Description = trimmedManualSchemaString(hint.Description)
	hint.Property = trimmedManualSchemaString(hint.Property)
	hint.InterfaceType = trimmedManualSchemaString(hint.InterfaceType)
	hint.RequiredWhen = trimmedManualSchemaString(hint.RequiredWhen)
	return hint
}

func trimmedManualSchemaString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func supportedManualSchemaInterfaceType(value string) bool {
	switch strings.TrimSpace(value) {
	case "string", "array", "object", "integer", "number", "boolean":
		return true
	default:
		return false
	}
}

func splitManualSchemaCanonicalPath(path string) (string, string, bool) {
	path = strings.TrimSpace(path)
	productID, toolName, ok := strings.Cut(path, ".")
	productID = strings.TrimSpace(productID)
	toolName = strings.TrimSpace(toolName)
	if !ok || productID == "" || toolName == "" || strings.ContainsAny(productID+toolName, " \t\r\n") {
		return "", "", false
	}
	return productID, toolName, true
}

func publicRunnableSchemaLeaf(command *cobra.Command) bool {
	if command == nil || !command.Runnable() || command.HasSubCommands() {
		return false
	}
	for current := command; current != nil; current = current.Parent() {
		if current.Hidden {
			return false
		}
	}
	return true
}
