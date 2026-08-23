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
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// dryRunCapabilityGroup is a reviewed positive capability declaration. An
// absent canonical deliberately publishes no dry_run field.
type dryRunCapabilityGroup struct {
	PreviewKind    string
	CanonicalPaths []string
}

// reviewedDryRunCapabilityGroups contains only command-owned preview paths
// for tools WITHOUT a Contract final declaration. Declared tools publish
// their dry_run capability from corecmd.ContractDecl (reviewed code) and are
// merged into the reviewed set at assembly time — no manual list entry.
// Inheriting the root --dry-run flag or reaching the generic EchoRunner is not
// evidence of a stable capability and must never add a command to this list.
// CI executes each selected example and compares the observed preview kind to
// this reviewed declaration.
var reviewedDryRunCapabilityGroups = []dryRunCapabilityGroup{
	// Declared tools publish dry_run from LeafSpec / Shortcut ContractDecl.
	// Manual entries remain only for tools that cannot yet declare ContractFinal.
}

var reviewedDryRunCapabilitiesLazy struct {
	once        sync.Once
	byCanonical map[string]contract.DryRunSpec
	err         error
}

// declaredDryRunCapabilities indexes dry_run capabilities sourced from
// Contract final declarations (canonical → spec). Populated by
// BindEffectiveCommandRegistry at command-tree bind time — every process
// that resolves the tree gets the reviewed set, not only processes that run
// Schema assembly. A declaration in reviewed code is itself the reviewed
// capability, so no manual allowlist entry is required.
var declaredDryRunCapabilities sync.Map // string → contract.DryRunSpec

// recordDeclaredDryRunCapability registers one Contract-declared dry_run
// capability. Conflicting re-declaration of the same canonical is a
// programming error surfaced at the next delivery gate read.
func recordDeclaredDryRunCapability(canonical string, spec contract.DryRunSpec) {
	canonical = strings.TrimSpace(canonical)
	if canonical == "" {
		return
	}
	declaredDryRunCapabilities.Store(canonical, spec)
}

// resetReviewedDryRunCapabilitiesLazy clears the lazy manual dry-run index so
// the next loadManualDryRunCapabilities() rebuilds from the current groups.
func resetReviewedDryRunCapabilitiesLazy() {
	reviewedDryRunCapabilitiesLazy = struct {
		once        sync.Once
		byCanonical map[string]contract.DryRunSpec
		err         error
	}{}
}

func loadManualDryRunCapabilities() (map[string]contract.DryRunSpec, error) {
	reviewedDryRunCapabilitiesLazy.once.Do(func() {
		byCanonical := make(map[string]contract.DryRunSpec)
		for _, group := range reviewedDryRunCapabilityGroups {
			spec := contract.DryRunSpec{PreviewKind: group.PreviewKind}
			if err := spec.Validate("<reviewed-dry-run-registry>"); err != nil {
				reviewedDryRunCapabilitiesLazy.err = err
				return
			}
			previous := ""
			for _, raw := range group.CanonicalPaths {
				canonical := strings.TrimSpace(raw)
				if canonical == "" {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("reviewed dry-run capability has empty canonical path")
					return
				}
				if previous != "" && canonical <= previous {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("reviewed dry-run capability paths for %s are not strictly sorted at %q", group.PreviewKind, canonical)
					return
				}
				previous = canonical
				if _, duplicate := byCanonical[canonical]; duplicate {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("duplicate reviewed dry-run capability %s", canonical)
					return
				}
				byCanonical[canonical] = spec
			}
		}
		reviewedDryRunCapabilitiesLazy.byCanonical = byCanonical
	})
	if reviewedDryRunCapabilitiesLazy.err != nil {
		return nil, reviewedDryRunCapabilitiesLazy.err
	}
	out := make(map[string]contract.DryRunSpec, len(reviewedDryRunCapabilitiesLazy.byCanonical))
	for canonical, spec := range reviewedDryRunCapabilitiesLazy.byCanonical {
		out[canonical] = spec
	}
	return out, nil
}

func loadReviewedDryRunCapabilities() (map[string]contract.DryRunSpec, error) {
	out, err := loadManualDryRunCapabilities()
	if err != nil {
		return nil, err
	}
	var mergeErr error
	declaredDryRunCapabilities.Range(func(key, value any) bool {
		canonical, ok := key.(string)
		if !ok {
			mergeErr = fmt.Errorf("declared dry-run capability has non-string key %v", key)
			return false
		}
		spec, ok := value.(contract.DryRunSpec)
		if !ok {
			mergeErr = fmt.Errorf("declared dry-run capability %s has non-contract.DryRunSpec value", canonical)
			return false
		}
		if manual, exists := out[canonical]; exists && manual != spec {
			mergeErr = fmt.Errorf("dry-run capability %s declared as %#v conflicts with manual reviewed entry %#v", canonical, spec, manual)
			return false
		}
		out[canonical] = spec
		return true
	})
	if mergeErr != nil {
		return nil, mergeErr
	}
	return out, nil
}

// ReviewedDryRunCapabilities returns a defensive copy of the positive,
// reviewed capability registry for delivery gates.
func ReviewedDryRunCapabilities() (map[string]contract.DryRunSpec, error) {
	return loadReviewedDryRunCapabilities()
}

// ValidateReviewedDryRunCapabilityDelivery proves that every positive source
// entry reaches the final typed registry and no serializer invents one. It
// deliberately imposes no minimum capability count or all-command coverage.
func ValidateReviewedDryRunCapabilityDelivery(registry SchemaRegistry) error {
	expected, err := loadReviewedDryRunCapabilities()
	if err != nil {
		return err
	}
	actual := make(map[string]contract.DryRunSpec)
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			if tool.DryRun != nil {
				actual[tool.Identity.CanonicalPath] = *tool.DryRun
			}
		}
	}
	var problems []string
	for canonical, want := range expected {
		got, ok := actual[canonical]
		if !ok {
			problems = append(problems, fmt.Sprintf("reviewed dry-run capability %s is missing from final Schema", canonical))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("Schema dry-run capability %s = %#v, want %#v", canonical, got, want))
		}
	}
	for canonical := range actual {
		if _, ok := expected[canonical]; !ok {
			problems = append(problems, fmt.Sprintf("Schema tool %s publishes an unreviewed dry-run capability", canonical))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
