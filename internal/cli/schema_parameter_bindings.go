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
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
)

const (
	schemaParameterBindingsVersion          = 3
	schemaParameterBindingsBaselineManifest = "schema-parameter-bindings-v3"
)

const runtimeSchemaFlagBindingPropertyAnnotation = "dws.schema.binding.property"

//go:embed schema_parameter_bindings.json
var embeddedSchemaParameterBindingsJSON []byte

type schemaParameterBindingSnapshot struct {
	Version           int                                         `json:"version"`
	Baseline          schemaParameterBindingBaseline              `json:"baseline"`
	Bindings          map[string]map[string]string                `json:"bindings"`
	Corrections       map[string]schemaParameterBindingCorrection `json:"corrections,omitempty"`
	Removals          map[string]schemaParameterBindingRemoval    `json:"removals,omitempty"`
	MappingExclusions map[string]string                           `json:"mapping_exclusions,omitempty"`
}

// schemaParameterBindingBaseline reviews the complete active binding set as
// one deterministic manifest. The hash is content-addressed: adding, removing,
// renaming, or remapping any active canonical/flag/property tuple requires an
// explicit baseline review instead of hundreds of low-value per-row records.
type schemaParameterBindingBaseline struct {
	Manifest string `json:"manifest"`
	SHA256   string `json:"sha256"`
	Reason   string `json:"reason"`
	Reviewed bool   `json:"reviewed"`
}

// schemaParameterBindingCorrection is a reviewed audit record for a binding
// whose historical property was wrong. It does not participate in resolution;
// Bindings remains the only source of non-empty versioned property mappings.
type schemaParameterBindingCorrection struct {
	OldProperty string `json:"old_property"`
	NewProperty string `json:"new_property"`
	Reason      string `json:"reason"`
	Reviewed    bool   `json:"reviewed"`
}

// schemaParameterBindingRemoval records a semantically meaningful deletion
// from a previous reviewed baseline. ReplacedBy, when present, must name an
// exact active binding key in the v3 manifest.
type schemaParameterBindingRemoval struct {
	Reason     string `json:"reason"`
	ReplacedBy string `json:"replaced_by,omitempty"`
	Reviewed   bool   `json:"reviewed"`
}

var runtimeSchemaParameterBindingsLazy struct {
	once     sync.Once
	snapshot schemaParameterBindingSnapshot
	err      error
}

var runtimeSchemaParameterBindingsLazyLoadCount atomic.Uint64

func decodeSchemaParameterBindings(data []byte) (schemaParameterBindingSnapshot, error) {
	var snapshot schemaParameterBindingSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return schemaParameterBindingSnapshot{}, fmt.Errorf("decode reviewed Schema parameter bindings: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return schemaParameterBindingSnapshot{}, fmt.Errorf("decode reviewed Schema parameter bindings: %w", err)
	}
	if err := validateSchemaParameterBindingSnapshot(snapshot); err != nil {
		return schemaParameterBindingSnapshot{}, err
	}
	return snapshot, nil
}

func loadSchemaParameterBindings() (schemaParameterBindingSnapshot, error) {
	return decodeSchemaParameterBindings(embeddedSchemaParameterBindingsJSON)
}

func runtimeSchemaParameterBindingData() (schemaParameterBindingSnapshot, error) {
	runtimeSchemaParameterBindingsLazy.once.Do(func() {
		runtimeSchemaParameterBindingsLazyLoadCount.Add(1)
		runtimeSchemaParameterBindingsLazy.snapshot, runtimeSchemaParameterBindingsLazy.err = loadSchemaParameterBindings()
	})
	return runtimeSchemaParameterBindingsLazy.snapshot, runtimeSchemaParameterBindingsLazy.err
}

// ValidateEmbeddedSchemaParameterBindings is the production validation gate
// for the reviewed binding source. Build and generator entrypoints call this
// before any candidate resolution so malformed input can never degrade to an
// empty binding set and flag-name inference.
func ValidateEmbeddedSchemaParameterBindings() error {
	_, err := runtimeSchemaParameterBindingData()
	return err
}

// ValidateSchemaParameterBindingsSource validates an alternate source with
// the same strict decoder used in production. It is intended for source audit
// tools and tests; runtime resolution always consumes the embedded source.
func ValidateSchemaParameterBindingsSource(data []byte) error {
	_, err := decodeSchemaParameterBindings(data)
	return err
}

func validateSchemaParameterBindingSnapshot(snapshot schemaParameterBindingSnapshot) error {
	if snapshot.Version != schemaParameterBindingsVersion {
		return fmt.Errorf("unsupported schema parameter bindings version %d", snapshot.Version)
	}
	baseline := snapshot.Baseline
	if strings.TrimSpace(baseline.Manifest) != schemaParameterBindingsBaselineManifest || baseline.Manifest != strings.TrimSpace(baseline.Manifest) {
		return fmt.Errorf("schema parameter bindings baseline must declare manifest %q", schemaParameterBindingsBaselineManifest)
	}
	if !baseline.Reviewed || strings.TrimSpace(baseline.Reason) == "" || baseline.Reason != strings.TrimSpace(baseline.Reason) {
		return fmt.Errorf("schema parameter bindings baseline must be reviewed with an exact non-empty reason")
	}
	manifestHash, err := schemaParameterBindingManifestHash(snapshot.Bindings)
	if err != nil {
		return err
	}
	if baseline.SHA256 != manifestHash {
		return fmt.Errorf("schema parameter bindings baseline hash = %q, want exact active manifest %q", baseline.SHA256, manifestHash)
	}

	active := make(map[string]string)
	for canonical, bindings := range snapshot.Bindings {
		for flagName, property := range bindings {
			active[runtimeSchemaParameterMappingKey(canonical, flagName)] = property
		}
	}
	for key, correction := range snapshot.Corrections {
		if err := validateSchemaParameterBindingAuditKey(key); err != nil {
			return fmt.Errorf("schema parameter binding correction: %w", err)
		}
		oldProperty := strings.TrimSpace(correction.OldProperty)
		newProperty := strings.TrimSpace(correction.NewProperty)
		if oldProperty == "" || newProperty == "" || oldProperty == newProperty || oldProperty != correction.OldProperty || newProperty != correction.NewProperty {
			return fmt.Errorf("schema parameter binding correction %q has invalid old/new properties", key)
		}
		if !correction.Reviewed || strings.TrimSpace(correction.Reason) == "" || correction.Reason != strings.TrimSpace(correction.Reason) {
			return fmt.Errorf("schema parameter binding correction %q must be reviewed with an exact non-empty reason", key)
		}
		if got := active[key]; got != newProperty {
			return fmt.Errorf("schema parameter binding correction %q new_property = %q, active manifest = %q", key, newProperty, got)
		}
	}
	for key, removal := range snapshot.Removals {
		if err := validateSchemaParameterBindingAuditKey(key); err != nil {
			return fmt.Errorf("schema parameter binding removal: %w", err)
		}
		if _, exists := active[key]; exists {
			return fmt.Errorf("schema parameter binding removal %q still exists in the active manifest", key)
		}
		if !removal.Reviewed || strings.TrimSpace(removal.Reason) == "" || removal.Reason != strings.TrimSpace(removal.Reason) {
			return fmt.Errorf("schema parameter binding removal %q must be reviewed with an exact non-empty reason", key)
		}
		if removal.ReplacedBy != strings.TrimSpace(removal.ReplacedBy) {
			return fmt.Errorf("schema parameter binding removal %q has non-canonical replaced_by %q", key, removal.ReplacedBy)
		}
		if removal.ReplacedBy != "" {
			if err := validateSchemaParameterBindingAuditKey(removal.ReplacedBy); err != nil {
				return fmt.Errorf("schema parameter binding removal %q replaced_by: %w", key, err)
			}
			if _, exists := active[removal.ReplacedBy]; !exists {
				return fmt.Errorf("schema parameter binding removal %q replacement %q is not active", key, removal.ReplacedBy)
			}
		}
	}
	for key, reason := range snapshot.MappingExclusions {
		if err := validateSchemaParameterBindingAuditKey(key); err != nil {
			return fmt.Errorf("schema parameter mapping exclusion: %w", err)
		}
		if _, exists := active[key]; exists {
			return fmt.Errorf("schema parameter mapping exclusion %q conflicts with the active manifest", key)
		}
		if _, removed := snapshot.Removals[key]; removed {
			return fmt.Errorf("schema parameter mapping exclusion %q is also recorded as a removal", key)
		}
		if strings.TrimSpace(reason) == "" || reason != strings.TrimSpace(reason) {
			return fmt.Errorf("schema parameter mapping exclusion %q must have an exact non-empty reason", key)
		}
	}
	return nil
}

// ValidateSchemaParameterBindingDelivery proves that every reviewed binding
// input reaches the final public typed registry it was written for. Source
// validation alone cannot catch a typo that is internally hash-consistent;
// this production invariant joins the v3 manifest to the bound Cobra command
// and its delivered ParameterSpec before any Catalog is serialized.
func ValidateSchemaParameterBindingDelivery(bound BoundCommandRegistry, registry SchemaRegistry) error {
	snapshot, err := runtimeSchemaParameterBindingData()
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

func schemaParameterProvenanceHasStringCandidate(provenance FieldProvenance, source, value string) bool {
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

func schemaParameterBindingManifestHash(bindings map[string]map[string]string) (string, error) {
	if len(bindings) == 0 {
		return "", fmt.Errorf("schema parameter bindings active manifest is empty")
	}
	type bindingRow struct {
		CanonicalPath string `json:"canonical_path"`
		FlagName      string `json:"flag_name"`
		Property      string `json:"property"`
	}
	rows := make([]bindingRow, 0)
	for canonical, parameters := range bindings {
		if canonical == "" || canonical != strings.TrimSpace(canonical) || !commandRegistryCanonicalPattern.MatchString(canonical) {
			return "", fmt.Errorf("schema parameter bindings contains invalid canonical path %q", canonical)
		}
		if len(parameters) == 0 {
			return "", fmt.Errorf("schema parameter bindings canonical %q contains no bindings", canonical)
		}
		for flagName, property := range parameters {
			if flagName == "" || flagName != strings.TrimSpace(flagName) || strings.ContainsAny(flagName, " \t\r\n") {
				return "", fmt.Errorf("schema parameter bindings %s contains invalid flag name %q", canonical, flagName)
			}
			if property == "" || property != strings.TrimSpace(property) {
				return "", fmt.Errorf("schema parameter binding %s --%s has invalid property %q", canonical, flagName, property)
			}
			rows = append(rows, bindingRow{CanonicalPath: canonical, FlagName: flagName, Property: property})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CanonicalPath != rows[j].CanonicalPath {
			return rows[i].CanonicalPath < rows[j].CanonicalPath
		}
		if rows[i].FlagName != rows[j].FlagName {
			return rows[i].FlagName < rows[j].FlagName
		}
		return rows[i].Property < rows[j].Property
	})
	encoded, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("encode Schema parameter binding manifest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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

func applyRuntimeSchemaParameterBindingsFrom(cmd *cobra.Command, canonical string, bindings map[string]map[string]string) {
	for flagName, propertyName := range bindings[strings.TrimSpace(canonical)] {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagBindingPropertyAnnotation, strings.TrimSpace(propertyName))
		}
	}
}

// EmbeddedSchemaParameterBindings returns a defensive copy of the reviewed,
// active public flag-to-interface bindings used by Catalog generation.
func EmbeddedSchemaParameterBindings() (map[string]map[string]string, error) {
	snapshot, err := runtimeSchemaParameterBindingData()
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
