// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRenderSelectionGuidanceUsesResolvedFields(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	renderSelectionGuidance(cmd, CommandSelection{
		UseWhen:       []string{" use reviewed route "},
		AvoidWhen:     []string{"avoid a different product"},
		Prerequisites: []string{"resolve the target ID"},
		Tips:          []string{"prefer structured output"},
	})
	rendered := out.String()
	for _, want := range []string{
		"When to use:\n  - use reviewed route",
		"Avoid when:\n  - avoid a different product",
		"Prerequisites:\n  - resolve the target ID",
		"Tips:\n  - prefer structured output",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered guidance missing %q: %q", want, rendered)
		}
	}
}

func TestCrossPlatformCoverageRenderHelpReferences(t *testing.T) {
	cmd := &cobra.Command{Use: "service"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	renderHelpReferences(cmd, contract.HelpReferences{
		RelatedSkills: []string{"dingtalk-chat"},
		Documentation: []contract.HelpDocumentation{{Label: "Chat guide", URL: "https://example.com/chat"}},
	})
	rendered := out.String()
	for _, want := range []string{"Related skills:\n  - dingtalk-chat", "Documentation:\n  - Chat guide: https://example.com/chat"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered references missing %q: %q", want, rendered)
		}
	}
}

func TestCrossPlatformCoverageRenderHelpAffordancesBranches(t *testing.T) {
	RenderHelpAffordances(nil)
	root := &cobra.Command{Use: "dws"}
	RenderHelpAffordances(root)

	meta, ok := ResolveMeta("dev app delete")
	if !ok || strings.TrimSpace(meta.Identity.ProductID) == "" {
		t.Fatalf("ResolveMeta(dev app delete) = %#v, ok=%v", meta, ok)
	}
	productID := meta.Identity.ProductID
	previous, hadPrevious := contract.LookupProductDecl(productID)
	t.Cleanup(func() {
		contract.ClearProductDeclForTest(productID)
		if hadPrevious {
			contract.RegisterProductDecl(previous)
		}
	})
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: productID,
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Manage developer applications",
			UseWhen:      []string{"the target is a developer application"},
			AvoidWhen:    []string{"the target belongs to another product"},
		},
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{{
				Label: "Developer application guide",
				URL:   "https://example.com/devapp",
			}},
		},
	})

	dev := &cobra.Command{Use: "dev"}
	app := &cobra.Command{Use: "app"}
	leaf := &cobra.Command{Use: "delete"}
	root.AddCommand(dev)
	dev.AddCommand(app)
	app.AddCommand(leaf)
	var leafOut bytes.Buffer
	leaf.SetOut(&leafOut)
	RenderHelpAffordances(leaf)
	for _, want := range []string{
		"When to use:",
		"Safety: effect=destructive",
		"Related skills:\n  - dingtalk-misc",
		"Developer application guide: https://example.com/devapp",
	} {
		if !strings.Contains(leafOut.String(), want) {
			t.Fatalf("leaf affordance missing %q: %q", want, leafOut.String())
		}
	}
	if got := commandCLIPath(leaf); got != "dev app delete" {
		t.Fatalf("commandCLIPath = %q, want dev app delete", got)
	}

	service := &cobra.Command{Use: productID}
	root.AddCommand(service)
	var serviceOut bytes.Buffer
	service.SetOut(&serviceOut)
	RenderHelpAffordances(service)
	if !strings.Contains(serviceOut.String(), "Related skills:\n  - dingtalk-misc") {
		t.Fatalf("service references missing: %q", serviceOut.String())
	}

	unknownGroup := &cobra.Command{Use: "unknown-group"}
	unknownLeaf := &cobra.Command{Use: "unknown-leaf"}
	root.AddCommand(unknownGroup)
	unknownGroup.AddCommand(unknownLeaf)
	RenderHelpAffordances(unknownLeaf)

	unknownService := &cobra.Command{Use: "unknown-service"}
	root.AddCommand(unknownService)
	RenderHelpAffordances(unknownService)

	var emptyOut bytes.Buffer
	renderGuidanceList(&emptyOut, "Empty:", []string{" ", ""})
	if emptyOut.Len() != 0 {
		t.Fatalf("empty guidance rendered output: %q", emptyOut.String())
	}
}
