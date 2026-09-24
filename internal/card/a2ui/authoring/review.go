// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package authoring

import (
	"sort"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
)

// VisualReview separates checks available from the A2UI file from judgments
// that require the intended content and an observed client render.
type VisualReview struct {
	StaticStatus            string                `json:"staticStatus"`
	Status                  string                `json:"status"`
	Diagnostics             []protocol.Diagnostic `json:"diagnostics"`
	AuthorChecks            []string              `json:"authorChecks"`
	ClientChecks            []string              `json:"clientChecks"`
	ClientRenderingVerified bool                  `json:"clientRenderingVerified"`
}

// ReviewVisual is bundled with DWS. It has no dependency on a design plugin.
// An empty diagnostics list does not certify composition or client appearance.
func ReviewVisual(messages []map[string]any, context VisualContext) VisualReview {
	diagnostics := VisualDiagnostics(messages, context)
	if diagnostics == nil {
		diagnostics = []protocol.Diagnostic{}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].InstancePath == diagnostics[j].InstancePath {
			return diagnostics[i].Code < diagnostics[j].Code
		}
		return diagnostics[i].InstancePath < diagnostics[j].InstancePath
	})
	staticStatus := "pass"
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			staticStatus = "fail"
			break
		}
		staticStatus = "pass_with_warnings"
	}
	return VisualReview{
		StaticStatus: staticStatus,
		Status:       "needs_client_review",
		Diagnostics:  diagnostics,
		AuthorChecks: []string{
			"Can the reader identify the main fact and its consequence within three seconds?",
			"Are facts that explain the same conclusion adjacent, with supporting details quieter than the conclusion?",
			"Does every status have a meaningful text label, and is semantic color grounded in the source facts?",
			"Are units, provenance, action labels, and required content complete and understandable?",
		},
		ClientChecks: []string{
			"Inspect the intended desktop and mobile widths for wrapping, clipping, alignment, and excessive empty space.",
			"Inspect light and dark themes for readable text, distinguishable status, and visible focus or interaction states.",
			"Verify the final card in the actual target client; a local HTML preview and a send receipt are not rendering evidence.",
		},
		ClientRenderingVerified: false,
	}
}
