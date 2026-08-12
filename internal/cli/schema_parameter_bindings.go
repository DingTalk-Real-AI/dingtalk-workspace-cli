// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

const runtimeSchemaFlagBindingPropertyAnnotation = "dws.schema.binding.property"

// Property delivery has migrated from active bindings to ParamDecl.Property:
//
//   - Property delivery is owned by leaf Contract.Parameters (ParamDecl.Property
//     → dws.schema.property / native_annotation). There is no committed
//     schema_parameter_bindings.json and no empty bindings{} audit table.
//   - Mapping exclusions and semantic removals live in
//     schema_parameter_mapping_ledger.go (reviewed Go constants).
//   - ValidateSchemaParameterBindingDelivery still joins exclusions/removals to
//     the final bound SchemaRegistry.
//   - Do not record ParamDecl migrations under Removals: that ledger means
//     "must not deliver a property anymore". Migration keeps delivery via
//     ParamDecl.
//   - BuildEffectiveCommandRegistry does not load this ledger; exclusion
//     validation runs at BindEffectiveCommandRegistry and catalog assembly.

type schemaParameterBindingSnapshot struct {
	// Bindings is retained only so dual-read / delivery audit can still prove
	// there are no versioned_parameter_binding winners. It is always empty in
	// production; property delivery is ParamDecl.Property.
	Bindings          map[string]map[string]string
	Removals          map[string]schemaParameterBindingRemoval
	MappingExclusions map[string]string
}

// schemaParameterBindingRemoval records a semantically meaningful deletion
// from a previous reviewed baseline. ReplacedBy, when present, must name an
// exact active binding key (no active bindings remain).
type schemaParameterBindingRemoval struct {
	Reason     string
	ReplacedBy string
	Reviewed   bool
}

var runtimeSchemaParameterBindingsLazy struct {
	once     sync.Once
	snapshot schemaParameterBindingSnapshot
	err      error
}

var runtimeSchemaParameterBindingsLazyLoadCount atomic.Uint64

var schemaParameterBindingData = runtimeSchemaParameterBindingData

func loadSchemaParameterBindingSnapshot() (schemaParameterBindingSnapshot, error) {
	snapshot := schemaParameterBindingSnapshot{
		Bindings:          map[string]map[string]string{},
		Removals:          reviewedSchemaParameterBindingRemovals,
		MappingExclusions: reviewedSchemaParameterMappingExclusions,
	}
	if err := validateSchemaParameterBindingSnapshot(snapshot); err != nil {
		return schemaParameterBindingSnapshot{}, err
	}
	return snapshot, nil
}

func runtimeSchemaParameterBindingData() (schemaParameterBindingSnapshot, error) {
	runtimeSchemaParameterBindingsLazy.once.Do(func() {
		runtimeSchemaParameterBindingsLazyLoadCount.Add(1)
		runtimeSchemaParameterBindingsLazy.snapshot, runtimeSchemaParameterBindingsLazy.err = loadSchemaParameterBindingSnapshot()
	})
	return runtimeSchemaParameterBindingsLazy.snapshot, runtimeSchemaParameterBindingsLazy.err
}

// ValidateSchemaParameterBindings is the production validation gate for the
// reviewed mapping ledger (exclusions + removals). Build and generator
// entrypoints call this before any candidate resolution.
func ValidateSchemaParameterBindings() error {
	_, err := schemaParameterBindingData()
	return err
}

func validateSchemaParameterBindingSnapshot(snapshot schemaParameterBindingSnapshot) error {
	if len(snapshot.Bindings) != 0 {
		return fmt.Errorf("schema parameter active bindings must remain empty; use ParamDecl.Property for property delivery")
	}
	for key, removal := range snapshot.Removals {
		if err := validateSchemaParameterBindingAuditKey(key); err != nil {
			return fmt.Errorf("schema parameter binding removal: %w", err)
		}
		if !removal.Reviewed || strings.TrimSpace(removal.Reason) == "" || removal.Reason != strings.TrimSpace(removal.Reason) {
			return fmt.Errorf("schema parameter binding removal %q must be reviewed with an exact non-empty reason", key)
		}
		if removal.ReplacedBy != strings.TrimSpace(removal.ReplacedBy) {
			return fmt.Errorf("schema parameter binding removal %q has non-canonical replaced_by %q", key, removal.ReplacedBy)
		}
		if removal.ReplacedBy != "" {
			return fmt.Errorf("schema parameter binding removal %q replacement %q is not active (active bindings are retired)", key, removal.ReplacedBy)
		}
	}
	for key, reason := range snapshot.MappingExclusions {
		if err := validateSchemaParameterBindingAuditKey(key); err != nil {
			return fmt.Errorf("schema parameter mapping exclusion: %w", err)
		}
		if _, removed := snapshot.Removals[key]; removed {
			return fmt.Errorf("schema parameter mapping exclusion %q is also recorded as a removal", key)
		}
		if strings.TrimSpace(reason) == "" || reason != strings.TrimSpace(reason) {
			return fmt.Errorf("schema parameter mapping exclusion %q must have an exact non-empty reason", key)
		}
	}
	if len(snapshot.MappingExclusions) == 0 {
		return fmt.Errorf("schema parameter mapping exclusions ledger must remain non-empty")
	}
	return nil
}

// ValidateSchemaParameterBindingDelivery proves that every reviewed mapping
// ledger entry reaches the final public typed registry it was written for.
func ValidateSchemaParameterBindingDelivery(bound BoundCommandRegistry, registry SchemaRegistry) error {
	snapshot, err := schemaParameterBindingData()
	if err != nil {
		return err
	}
	return validateSchemaParameterBindingDelivery(snapshot, bound, registry)
}

func validateSchemaParameterBindingDelivery(snapshot schemaParameterBindingSnapshot, bound BoundCommandRegistry, registry SchemaRegistry) error {
	publicBound := make(map[string]BoundCommandSpec)
	for _, command := range bound.Commands {
		if command.Visibility == SchemaVisibilityPublic {
			publicBound[command.CanonicalPath] = command
		}
	}
	tools := make(map[string]ToolSpec)
	problems := make([]string, 0)
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.Identity.CanonicalPath)
			if canonical == "" {
				problems = append(problems, "final SchemaRegistry contains a tool with empty canonical_path")
				continue
			}
			if _, duplicate := tools[canonical]; duplicate {
				problems = append(problems, fmt.Sprintf("final SchemaRegistry contains duplicate canonical tool %s", canonical))
				continue
			}
			tools[canonical] = tool
		}
	}

	active := make(map[string]string)
	for canonical, bindings := range snapshot.Bindings {
		for flagName, property := range bindings {
			active[runtimeSchemaParameterMappingKey(canonical, flagName)] = property
		}
	}
	activeKeys := sortedSchemaParameterBindingKeys(active)
	for _, key := range activeKeys {
		canonical, flagName, _ := strings.Cut(key, " --")
		property := active[key]
		command, boundOK := publicBound[canonical]
		if !boundOK {
			problems = append(problems, fmt.Sprintf("active parameter binding %q does not reference a public bound command", key))
			continue
		}
		if command.PrimaryCommand == nil || runtimeCommandFlag(command.PrimaryCommand, flagName) == nil {
			problems = append(problems, fmt.Sprintf("active parameter binding %q does not reference an exact bound Cobra flag", key))
		}
		parameter, ok := finalSchemaParameterByName(tools[canonical], flagName)
		if !ok {
			problems = append(problems, fmt.Sprintf("active parameter binding %q does not reference an exact final public Schema parameter", key))
			continue
		}
		if parameter.Property != property {
			problems = append(problems, fmt.Sprintf("active parameter binding %q property = %q, final Schema = %q", key, property, parameter.Property))
		}
		provenance, ok := parameter.FieldProvenance["property"]
		if !ok || !schemaParameterProvenanceHasStringCandidate(provenance, "versioned_parameter_binding", property) {
			problems = append(problems, fmt.Sprintf("active parameter binding %q final property provenance has no exact versioned_parameter_binding candidate", key))
		}
	}

	exclusionKeys := make([]string, 0, len(snapshot.MappingExclusions))
	for key := range snapshot.MappingExclusions {
		exclusionKeys = append(exclusionKeys, key)
	}
	sort.Strings(exclusionKeys)
	for _, key := range exclusionKeys {
		canonical, flagName, _ := strings.Cut(key, " --")
		command, ok := publicBound[canonical]
		if !ok {
			problems = append(problems, fmt.Sprintf("parameter mapping exclusion %q does not reference a public bound command", key))
			continue
		}
		if command.PrimaryCommand == nil || runtimeCommandFlag(command.PrimaryCommand, flagName) == nil {
			problems = append(problems, fmt.Sprintf("parameter mapping exclusion %q does not reference an exact bound Cobra flag", key))
		}
		parameter, ok := finalSchemaParameterByName(tools[canonical], flagName)
		if !ok {
			problems = append(problems, fmt.Sprintf("parameter mapping exclusion %q does not reference an exact final public Schema parameter", key))
			continue
		}
		if parameter.Property != "" {
			problems = append(problems, fmt.Sprintf("parameter mapping exclusion %q delivered property %q, want omitted", key, parameter.Property))
		}
		provenance, ok := parameter.FieldProvenance["property"]
		if !ok || provenance.Source != "reviewed_mapping_exclusion" || strings.TrimSpace(provenance.ReviewReason) == "" {
			problems = append(problems, fmt.Sprintf("parameter mapping exclusion %q final provenance is not the reviewed exclusion", key))
		}
	}

	removalKeys := make([]string, 0, len(snapshot.Removals))
	for key := range snapshot.Removals {
		removalKeys = append(removalKeys, key)
	}
	sort.Strings(removalKeys)
	for _, key := range removalKeys {
		canonical, flagName, _ := strings.Cut(key, " --")
		if _, ok := bound.ByCanonical[canonical]; !ok {
			problems = append(problems, fmt.Sprintf("parameter binding removal %q has a stale canonical path", key))
			continue
		}
		if parameter, exists := finalSchemaParameterByName(tools[canonical], flagName); exists {
			problems = append(problems, fmt.Sprintf("parameter binding removal %q is still delivered with property %q", key, parameter.Property))
		}
	}

	for canonical, tool := range tools {
		for _, parameter := range tool.Parameters {
			key := runtimeSchemaParameterMappingKey(canonical, parameter.Name)
			provenance, ok := parameter.FieldProvenance["property"]
			if !ok {
				continue
			}
			switch provenance.Source {
			case "versioned_parameter_binding":
				if property, exists := active[key]; !exists || property != parameter.Property {
					problems = append(problems, fmt.Sprintf("final Schema parameter %q claims versioned binding provenance without an exact active manifest entry", key))
				}
			case "reviewed_mapping_exclusion":
				if _, exists := snapshot.MappingExclusions[key]; !exists || parameter.Property != "" {
					problems = append(problems, fmt.Sprintf("final Schema parameter %q claims mapping exclusion provenance without an exact reviewed exclusion", key))
				}
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("schema parameter binding delivery invariant failed with %d problem(s):\n - %s", len(problems), strings.Join(problems, "\n - "))
}

func finalSchemaParameterByName(tool ToolSpec, flagName string) (ParameterSpec, bool) {
	for _, parameter := range tool.Parameters {
		if parameter.Name == flagName {
			return parameter, true
		}
	}
	return ParameterSpec{}, false
}

func schemaParameterProvenanceHasStringCandidate(provenance contract.FieldProvenance, source, value string) bool {
	if provenance.Source == source {
		var selected string
		return json.Unmarshal(provenance.Value, &selected) == nil && selected == value
	}
	for _, candidate := range provenance.Candidates {
		if candidate.Source != source {
			continue
		}
		var candidateValue string
		if json.Unmarshal(candidate.Value, &candidateValue) == nil && candidateValue == value {
			return true
		}
	}
	return false
}

func sortedSchemaParameterBindingKeys(bindings map[string]string) []string {
	keys := make([]string, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateSchemaParameterBindingAuditKey(key string) error {
	if key == "" || key != strings.TrimSpace(key) {
		return fmt.Errorf("invalid exact binding key %q", key)
	}
	canonical, flagName, ok := strings.Cut(key, " --")
	if !ok || strings.Contains(flagName, " --") || !commandRegistryCanonicalPattern.MatchString(canonical) || flagName == "" || strings.ContainsAny(flagName, " \t\r\n") {
		return fmt.Errorf("invalid exact binding key %q", key)
	}
	return nil
}

// LoadSchemaParameterBindings returns a defensive copy of the reviewed
// active public flag-to-interface bindings. Active bindings are empty;
// property delivery comes from ParamDecl.Property. Mapping
// exclusions and removals remain on the Go mapping ledger (not returned here).
func LoadSchemaParameterBindings() (map[string]map[string]string, error) {
	snapshot, err := schemaParameterBindingData()
	if err != nil {
		return nil, err
	}
	source := snapshot.Bindings
	bindings := make(map[string]map[string]string, len(source))
	for canonical, parameters := range source {
		bindings[canonical] = make(map[string]string, len(parameters))
		for flagName, propertyName := range parameters {
			bindings[canonical][flagName] = propertyName
		}
	}
	return bindings, nil
}
