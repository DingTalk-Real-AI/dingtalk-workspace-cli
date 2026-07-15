// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func TestReviewedPATDryRunCapabilityIsRemotePlan(t *testing.T) {
	capabilities, err := ReviewedDryRunCapabilities()
	if err != nil {
		t.Fatalf("ReviewedDryRunCapabilities() error = %v", err)
	}
	got, ok := capabilities["pat.batch_grant"]
	if !ok {
		t.Fatal("pat.batch_grant has no reviewed dry-run capability")
	}
	want := (DryRunSpec{PreviewKind: DryRunPreviewPlan, RemoteReads: true})
	if got != want {
		t.Fatalf("pat.batch_grant dry-run = %#v, want %#v", got, want)
	}
}

func TestPATRemotePlanExamplesHaveReviewedStatefulPreflightDisposition(t *testing.T) {
	hints, err := loadAgentHintsFromSelection(embeddedSelectionFS, embeddedSelectionGlob)
	if err != nil {
		t.Fatalf("loadAgentHintsFromSelection() error = %v", err)
	}
	tool, ok := hints.Tools["pat.batch_grant"]
	if !ok {
		t.Fatal("pat.batch_grant has no reviewed selection hint")
	}
	if got, want := len(tool.Examples), 2; got != want {
		t.Fatalf("pat.batch_grant examples = %d, want %d", got, want)
	}
	if got, want := len(tool.ExampleDispositions), len(tool.Examples); got != want {
		t.Fatalf("pat.batch_grant example dispositions = %d, want %d", got, want)
	}
	for index, disposition := range tool.ExampleDispositions {
		if disposition.Index == nil || *disposition.Index != index {
			t.Fatalf("disposition %d index = %#v", index, disposition.Index)
		}
		if disposition.Mode != ManualAgentExampleModeContractOnly ||
			disposition.ReasonCode != ManualAgentExampleReasonStatefulPreflight ||
			!disposition.Reviewed {
			t.Fatalf("disposition %d = %#v", index, disposition)
		}
	}
}
