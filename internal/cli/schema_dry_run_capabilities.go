// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// dryRunCapabilityGroup is a reviewed positive capability declaration. An
// absent canonical deliberately publishes no dry_run field.
type dryRunCapabilityGroup struct {
	PreviewKind    string
	CanonicalPaths []string
}

// reviewedDryRunCapabilityGroups contains only command-owned preview paths.
// Inheriting the root --dry-run flag or reaching the generic EchoRunner is not
// evidence of a stable capability and must never add a command to this list.
// CI executes each selected example and compares the observed preview kind to
// this reviewed declaration.
var reviewedDryRunCapabilityGroups = []dryRunCapabilityGroup{
	{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{
		"event.stop",
	}},
	{PreviewKind: DryRunPreviewPlan, CanonicalPaths: []string{
		"chat.download_media",
		"doc.download_file",
		"doc.import_get",
		"doc.media_insert",
		"doc.query_export_job",
		"doc.upload",
		"drive.download_file",
		"drive.upload",
		"sheet.filter_view_get_criteria",
		"sheet.filter_view_info",
		"sheet.filter_view_list_criteria",
		"sheet.media_upload",
		"sheet.submit_export_job",
		"sheet.write_image",
		"todo.add_todo_attachment",
	}},
}

var reviewedDryRunCapabilitiesLazy struct {
	once        sync.Once
	byCanonical map[string]DryRunSpec
	err         error
}

func loadReviewedDryRunCapabilities() (map[string]DryRunSpec, error) {
	reviewedDryRunCapabilitiesLazy.once.Do(func() {
		byCanonical := make(map[string]DryRunSpec)
		for _, group := range reviewedDryRunCapabilityGroups {
			spec := DryRunSpec{PreviewKind: group.PreviewKind}
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
	out := make(map[string]DryRunSpec, len(reviewedDryRunCapabilitiesLazy.byCanonical))
	for canonical, spec := range reviewedDryRunCapabilitiesLazy.byCanonical {
		out[canonical] = spec
	}
	return out, nil
}

// ReviewedDryRunCapabilities returns a defensive copy of the positive,
// reviewed capability registry for delivery gates.
func ReviewedDryRunCapabilities() (map[string]DryRunSpec, error) {
	return loadReviewedDryRunCapabilities()
}

func reviewedDryRunCapability(canonical string) (*DryRunSpec, error) {
	capabilities, err := loadReviewedDryRunCapabilities()
	if err != nil {
		return nil, err
	}
	spec, ok := capabilities[strings.TrimSpace(canonical)]
	if !ok {
		return nil, nil
	}
	return &spec, nil
}

// ValidateReviewedDryRunCapabilityDelivery proves that every positive source
// entry reaches the final typed registry and no serializer invents one. It
// deliberately imposes no minimum capability count or all-command coverage.
func ValidateReviewedDryRunCapabilityDelivery(registry SchemaRegistry) error {
	expected, err := loadReviewedDryRunCapabilities()
	if err != nil {
		return err
	}
	actual := make(map[string]DryRunSpec)
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
