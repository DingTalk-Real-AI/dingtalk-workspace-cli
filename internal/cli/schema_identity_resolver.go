// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

// commandRegistryIdentityProvenance records one selected registry identity and
// any agreeing implementation evidence. Native/manual annotations never
// compete for identity here: the Cobra binder has already rejected every
// disagreement before a ToolSpec can be assembled.
func commandRegistryIdentityProvenance(command BoundCommandSpec) FieldProvenance {
	canonical := strings.TrimSpace(command.CanonicalPath)
	value, _ := json.Marshal(canonical)
	selected := true
	precedence := commandIdentityPrecedence(command.Source)
	candidates := []FieldCandidateProvenance{{
		Value:        append(json.RawMessage(nil), value...),
		Source:       command.Source,
		SourceRef:    command.PrimaryCLIPath,
		Precedence:   precedence,
		ReviewReason: command.ReviewReason,
		Selected:     &selected,
	}}

	// Cobra does not expose an annotation-only interface in older versions, so
	// collect evidence directly while retaining the exact reviewed CLI path.
	appendEvidence := func(path string, leaf *cobra.Command) {
		if leaf == nil {
			return
		}
		if productID, toolName, sourceRef := runtimeSchemaAnnotations(leaf); productID != "" && toolName != "" {
			nativeValue, _ := json.Marshal(productID + "." + toolName)
			unselected := false
			candidates = append(candidates, FieldCandidateProvenance{
				Value:      nativeValue,
				Source:     "native_annotation",
				SourceRef:  defaultString(strings.TrimSpace(sourceRef), path),
				Precedence: "native_annotation",
				Selected:   &unselected,
			})
		}
		if productID, toolName, reason, ok := runtimeManualSchemaIdentity(leaf); ok {
			manualValue, _ := json.Marshal(productID + "." + toolName)
			// A manual-only CommandSpec is already represented by the selected
			// candidate above. Keep a second candidate only when manual review is
			// corroborating a registry-owned identity.
			if command.Source != "reviewed_manual_hint" {
				unselected := false
				candidates = append(candidates, FieldCandidateProvenance{
					Value:        manualValue,
					Source:       "reviewed_manual_hint",
					SourceRef:    path,
					Precedence:   "reviewed_manual",
					ReviewReason: reason,
					Selected:     &unselected,
				})
			}
		}
	}

	appendEvidence(command.PrimaryCLIPath, command.PrimaryCommand)
	for _, alias := range command.AliasCommands {
		appendEvidence(alias.Path, alias.Command)
	}
	return FieldProvenance{
		Value:        value,
		Source:       command.Source,
		SourceRef:    command.PrimaryCLIPath,
		Precedence:   precedence,
		Resolution:   "registry_identity",
		ReviewReason: command.ReviewReason,
		Candidates:   candidates,
	}
}

func commandIdentityPrecedence(source string) string {
	switch strings.TrimSpace(source) {
	case "reviewed_manual_hint":
		return "reviewed_manual"
	default:
		return "command_registry"
	}
}
