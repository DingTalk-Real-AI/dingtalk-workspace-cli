// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"strings"
	"testing"
)

func TestValidateInterfaceDispositionsMatrix(t *testing.T) {
	ref := &InterfaceRef{ProductID: "calendar", RPCName: "get_event"}
	for _, test := range []struct {
		name string
		tool ToolMetadata
		want string
	}{
		{name: "mcp available", tool: ToolMetadata{InterfaceMode: "mcp", Availability: "available", InterfaceRef: ref}},
		{name: "local available", tool: ToolMetadata{InterfaceMode: "local", Availability: "available"}},
		{name: "composite available", tool: ToolMetadata{InterfaceMode: "composite", Availability: "available", InterfaceReason: "orchestrates operations"}},
		{name: "mcp unavailable", tool: ToolMetadata{InterfaceMode: "mcp", Availability: "unavailable", InterfaceReason: "RPC retired"}},
		{name: "local unavailable", tool: ToolMetadata{InterfaceMode: "local", Availability: "unavailable", InterfaceReason: "compatibility command frozen"}},
		{name: "composite unavailable", tool: ToolMetadata{InterfaceMode: "composite", Availability: "unavailable", InterfaceReason: "workflow retired"}},
		{name: "unavailable is not mode", tool: ToolMetadata{InterfaceMode: "unavailable", Availability: "unavailable", InterfaceReason: "retired"}, want: "legacy interface_mode=unavailable; migrate"},
		{name: "mcp available without ref", tool: ToolMetadata{InterfaceMode: "mcp", Availability: "available"}, want: "available mcp without interface_ref"},
		{name: "local available with ref", tool: ToolMetadata{InterfaceMode: "local", Availability: "available", InterfaceRef: ref}, want: "available local but declares interface_ref"},
		{name: "composite available with ref", tool: ToolMetadata{InterfaceMode: "composite", Availability: "available", InterfaceReason: "orchestrates operations", InterfaceRef: ref}, want: "available composite but declares a single interface_ref"},
		{name: "composite available without reason", tool: ToolMetadata{InterfaceMode: "composite", Availability: "available"}, want: "available composite without interface_reason"},
		{name: "unavailable with ref", tool: ToolMetadata{InterfaceMode: "mcp", Availability: "unavailable", InterfaceReason: "retired", InterfaceRef: ref}, want: "unavailable but declares interface_ref"},
		{name: "unavailable without reason", tool: ToolMetadata{InterfaceMode: "local", Availability: "unavailable"}, want: "unavailable without interface_reason"},
		{name: "missing availability", tool: ToolMetadata{InterfaceMode: "local"}, want: "unsupported availability"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateInterfaceDispositions(File{Tools: map[string]ToolMetadata{"calendar event get": test.tool}})
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateInterfaceDispositions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateInterfaceDispositions() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFinalizeUnavailableDispositionClearsRefAndRecordsNoneWinner(t *testing.T) {
	file := File{Tools: map[string]ToolMetadata{
		"conference camera open": {
			InterfaceMode:       "mcp",
			interfaceModeRank:   selectionRankReviewedExplicit,
			interfaceModeOrigin: "reviewed.json",
			Availability:        "unavailable",
			availabilityRank:    selectionRankReviewedExplicit,
			availabilityOrigin:  "reviewed.json",
			InterfaceReason:     "compatibility command frozen",
			InterfaceRef:        &InterfaceRef{ProductID: "conference", RPCName: "open_camera"},
			interfaceRefRank:    selectionRankImported,
			interfaceRefOrigin:  "pinned-interface.json",
			FieldProvenance: map[string]FieldProvenance{
				"interface_ref": {
					Value:      "conference.open_camera",
					Source:     "pinned-interface.json",
					Precedence: "imported",
					Resolution: "highest_precedence",
					Candidates: []FieldCandidateProvenance{{
						Value:      "conference.open_camera",
						Source:     "pinned-interface.json",
						Precedence: "imported",
						Selected:   true,
					}},
				},
			},
		},
	}}

	finalizeInterfaceDispositions(&file)
	tool := file.Tools["conference camera open"]
	if tool.InterfaceRef != nil {
		t.Fatalf("InterfaceRef = %#v, want nil", tool.InterfaceRef)
	}
	provenance, ok := tool.FieldProvenance["interface_ref"]
	if !ok || provenance.Value != nil || provenance.Resolution != "interface_disposition_matrix" {
		t.Fatalf("interface_ref provenance = %#v", provenance)
	}
	if len(provenance.Candidates) != 1 || !provenance.Candidates[0].Selected || provenance.Candidates[0].Value != nil {
		t.Fatalf("interface_ref candidates = %#v", provenance.Candidates)
	}
	if !strings.Contains(provenance.ReviewReason, "forbids Agent invocation") {
		t.Fatalf("interface_ref review reason = %q", provenance.ReviewReason)
	}
	if len(provenance.OverriddenCandidates) != 1 || provenance.OverriddenCandidates[0].Value != "conference.open_camera" {
		t.Fatalf("overridden interface_ref candidates = %#v", provenance.OverriddenCandidates)
	}
}

func TestGenerateRejectsLegacyUnavailableModeWithMigrationError(t *testing.T) {
	root := t.TempDir()
	writePrecedenceFixture(t, root, "calendar event get")
	writeFixture(t, root, "internal/cli/schema_hints/legacy.json", hintFixture("explicit", "legacy-interface", "calendar event get", `{
      "interface_mode": "unavailable",
      "availability": "unavailable",
      "interface_reason": "legacy fixture"
    }`))

	_, _, err := generateFromSources(precedenceOptions(root, map[string]string{
		"calendar event get": "calendar event get",
	}))
	if err == nil || !strings.Contains(err.Error(), "legacy interface_mode=unavailable; migrate") || !strings.Contains(err.Error(), "availability=unavailable") {
		t.Fatalf("Generate() error = %v, want explicit interface mode migration guidance", err)
	}
}
