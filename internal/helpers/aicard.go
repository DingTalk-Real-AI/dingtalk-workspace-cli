// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func newAicardCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "aicard",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-aicard"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("A2UI card design guide", "dingtalk-aicard", "references/design.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Look up contracts, validate structure, or preview a card to yourself when creating an A2UI file",
			UseWhen:      []string{"Create or modify a DingTalk A2UI card file"},
			AvoidWhen:    []string{"Use chat for ordinary messages"},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use: "aicard", Short: "Inspect, validate, and preview DingTalk A2UI cards",
		Long: "Look up field contracts by name, validate files offline, or send a card to the signed-in user. See the dingtalk-aicard Skill for authoring guidance.",
		RunE: groupRunE, SuggestionsMinimumDistance: 2,
	})
	root.AddCommand(newAicardLintCommand(), newAicardExplainCommand(), newAicardPreviewCommand())
	return root
}

func newAicardLintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "lint", Short: "Validate A2UI syntax and protocol structure",
		Long: `Check JSON syntax and the bundled A2UI protocol offline, including DingTalk extensions, field names, required fields, types, and enums.
Use --preflight new-card or resources for explicit offline checks; valid still represents only the Schema result, while preflight has its own result.
General lint does not check reference closure. The new-card preflight also checks the final snapshot root, reachable references, duplicate IDs within a message, and static cycles; unreachable components produce warnings.
Binding initial values, template expansion, function results, and design effects still need runtime verification. Python, an installed Skill, and a Profile are not required.
Specify exactly one of --file and --self-check. --emit and --fragment require --file and are mutually exclusive.
Successful results are in data; structural failures exit nonzero and report details in error.details. Passing does not prove client rendering.`,
		Example: "  dws aicard lint --file card.a2ui.json --emit\n  dws aicard lint --self-check",
		Args:    cobra.NoArgs, RunE: runAicardLint,
	}
	cmd.Flags().String("file", "", "Path to a JSON file containing an A2UI message array")
	cmd.Flags().Bool("self-check", false, "Check the embedded protocol, index, and local references")
	cmd.Flags().Bool("fragment", false, "Accept a single component, component array, or message; incompatible with --emit")
	cmd.Flags().String("preflight", "", "Run new-card or resources preflight after structural validation; does not send")
	cmd.Flags().Bool("emit", false, "On success, return a JSON string array in data.a2uiMessages; write no file")
	runtimeannotate.AnnotateRuntimeConstraints(cmd, runtimeannotate.RuntimeSchemaConstraints{
		RequireOneOf:      [][]string{{"file", "self-check"}},
		MutuallyExclusive: [][]string{{"file", "self-check"}, {"self-check", "emit"}, {"self-check", "fragment"}, {"emit", "fragment"}, {"preflight", "self-check"}, {"preflight", "fragment"}},
	})
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "lint", CanonicalPath: "aicard.lint", CLIPath: "aicard lint", PrimaryCLIPath: "aicard lint"},
			Description: "Validate A2UI syntax and protocol structure offline; optionally return transport-ready message strings",
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "Uses the protocol embedded in the binary; does not access the network or execute card functions"},
			Selection: contract.SelectionSpec{
				AgentSummary: "Validate syntax and protocol structure of an existing A2UI file",
				UseWhen:      []string{"An A2UI file exists and needs structural validation or serialization for sending"},
				AvoidWhen:    []string{"Use aicard explain for field contracts and aicard preview to send a real preview"},
				Examples:     []string{"dws aicard lint --file card.a2ui.json", "dws aicard lint --file card.a2ui.json --emit"},
			},
			Result: &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"preflight":{"type":"object","description":"Explicit preflight result; its valid is independent of Schema and includes diagnostics and unverified boundaries"},"valid":{"type":"boolean","description":"Whether syntax and protocol structure passed"},"diagnostics":{"type":"array","description":"Structural diagnostics"},"renderingVerified":{"type":"boolean","description":"Whether client rendering evidence exists; always false for local validation"},"a2uiMessages":{"type":"array","items":{"type":"string"},"description":"Returned only after successful emit; each item is a serialized message"},"ready":{"type":"boolean","description":"Returned only by self-check; whether the embedded protocol is complete"},"manifest":{"type":"object","description":"Returned only by self-check; protocol manifest"}}}`)},
		},
	})
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	return cmd
}

func newAicardExplainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "explain <name> [name...]", Short: "Inspect components, functions, Tokens, or common types",
		Long:    "Return fields, a minimal example, and common pitfalls by name. Misspellings receive suggestions. Reads the embedded protocol index without network access; no Profile, Python, or installed Skill is needed.",
		Example: "  dws aicard explain Tabs\n  dws aicard explain promptText -f json",
		Args:    cobra.MinimumNArgs(1), RunE: runAicardExplain,
	}
	cmd.Flags().Bool("compact", false, "Deduplicate repeated structures in batch results; single-name output is unchanged")
	runtimeannotate.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{Name: "name", Type: "string", Index: 0, Required: true, Variadic: true, Description: "Name of a component, function, Token type, Token value, or common type"})
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "explain", CanonicalPath: "aicard.explain", CLIPath: "aicard explain", PrimaryCLIPath: "aicard explain"},
			Description: "Return field contracts for one or more A2UI components, functions, Tokens, or common types",
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "Reads the protocol index generated into the binary"},
			Selection: contract.SelectionSpec{
				AgentSummary: "Look up fields, examples, and pitfalls by name before authoring a card",
				UseWhen:      []string{"Confirm required fields, binding slots, child references, or function arguments"},
				AvoidWhen:    []string{"Use aicard lint to validate files; use the Skill component index to browse all components"},
				Examples:     []string{"dws aicard explain Tabs", "dws aicard explain promptText"},
			},
			Result: &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: json.RawMessage(`{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","description":"component, function, token, token-item, type, or bundle"},"name":{"type":"string","description":"Canonical name in the protocol"},"contracts":{"type":"array","description":"Contracts returned by a batch lookup"},"formatVersion":{"type":"integer","description":"Batch output format version"},"definitions":{"type":"object","description":"Optional deduplicated definitions referenced by $contractRef; do not copy into A2UI"},"fields":{"type":"array","description":"Fields of a component or common type"},"args":{"type":"array","description":"Function arguments"},"items":{"type":"array","description":"Token values"},"example":{"description":"Protocol-derived example; placeholders must be replaced"},"notes":{"type":"array","description":"Common pitfalls"}}}`)},
		},
	})
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	return cmd
}

func newAicardPreviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "preview", Short: "Send a real A2UI card to the signed-in user",
		Long: `Validate the new-card file, resolve the current Profile's user identity, and send to that user's one-to-one conversation.
The file must include createSurface and the public catalogId; this command does not modify the source file.
--dry-run checks only the local file; it neither resolves an identity nor sends. Request acceptance does not prove delivery or rendering.
Any openTaskId in the card receipt is not valid for chat message query-send-status; this command reports request acceptance only.
The send API uses PROCESSING status; use chat message update-a2ui-card for later lifecycle transitions.
Card buttons may still trigger real business actions. This command accepts no other recipient.`,
		Example: "  dws aicard preview --file card.a2ui.json --dry-run\n  dws aicard preview --file card.a2ui.json --summary \"Card preview\"",
		Args:    cobra.NoArgs, RunE: runAicardPreview,
	}
	cmd.Flags().String("file", "", "Path to a complete new-card A2UI message-array JSON file")
	cmd.Flags().String("summary", "Card preview", "Conversation-list summary")
	_ = cmd.MarkFlagRequired("file")
	runtimeannotate.AnnotateRuntimeRequiredFlags(cmd, "file")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "preview", CanonicalPath: "aicard.preview", CLIPath: "aicard preview", PrimaryCLIPath: "aicard preview"},
			Description: "Validate and send an A2UI card to the signed-in user's one-to-one conversation",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Combines local validation, current-user lookup, ID conversion, and the existing A2UI send API"},
			Selection: contract.SelectionSpec{
				AgentSummary: "Send a complete new card to the current user when a preview is requested",
				UseWhen:      []string{"The user wants to inspect a card in the DingTalk client"},
				AvoidWhen:    []string{"Use aicard lint to check only the file; use chat message send-a2ui-card for another person or a group"},
				Examples:     []string{"dws aicard preview --file card.a2ui.json", "dws aicard preview --file card.a2ui.json --dry-run"},
			},
			Result: &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: json.RawMessage(`{"type":"object","properties":{"requestAccepted":{"type":"boolean","description":"Whether the server explicitly accepted the send request"},"deliveryVerified":{"type":"boolean","description":"Whether delivery was confirmed by readback; false for this command"},"renderingVerified":{"type":"boolean","description":"Whether client rendering evidence exists; false for this command"},"flowStatus":{"type":"string","description":"Lifecycle status at send time"},"requestId":{"type":"string","description":"Identifier for this send request"},"bizCardId":{"type":"string","description":"Business card ID for this creation"},"receipt":{"type":"object","description":"Server receipt, including available card and task identifiers"},"executed":{"type":"boolean","description":"Whether the send API was actually called"}}}`)},
		},
	})
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	return cmd
}
