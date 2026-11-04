// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"context"
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/spf13/cobra"
)

// AicardTemplateCaller is implemented by the host application's authenticated
// caller. Command construction and offline commands never perform discovery.
type AicardTemplateCaller interface {
	InvokeAicardTemplate(context.Context, string, string, map[string]any) (publishedmcp.ValidatedInvocationResult, error)
}

const aicardTemplateServiceEnv = "DWS_AICARD_TEMPLATE_MCP_ID"

func aicardTemplateServiceFlag() LeafFlag {
	return LeafFlag{Name: "mcp-id", EnvVar: aicardTemplateServiceEnv, Required: true, Trim: true,
		Usage:        "Published MCP service ID for the intended environment; set --mcp-id or DWS_AICARD_TEMPLATE_MCP_ID (no default)",
		RequiredHint: "flag --mcp-id is required (or set DWS_AICARD_TEMPLATE_MCP_ID)"}
}

func newAicardTemplateCommand() *cobra.Command {
	cmd := newGroupCommand(&cobra.Command{
		Use: "template", Short: "Create, retrieve, or revise a cloud A2UI template asset",
		Long: "Manage complete A2UI template files through the authenticated published MCP service. These operations do not send cards, change business bindings, or search by name. Configure the intended environment's service explicitly with --mcp-id or DWS_AICARD_TEMPLATE_MCP_ID; no cross-environment default is assumed.",
		RunE: groupRunE,
	})
	cmd.AddCommand(newAicardTemplateCreateCommand(), newAicardTemplateGetCommand(), newAicardTemplateUpdateCommand())
	return cmd
}

// aicardTemplateFileExampleDispositions keeps file-based write examples out of
// the offline Agent dry-run runner, which cannot supply a valid A2UI template.
func aicardTemplateFileExampleDispositions() []contract.ExampleDisposition {
	index := 0
	return []contract.ExampleDisposition{{
		Index: &index, Mode: contract.ExampleDispositionModeContractOnly,
		ReasonCode: contract.ExampleDispositionReasonLocalState,
		Reason:     "Dry-run requires a readable local file that passes complete A2UI validation",
		Reviewed:   true,
	}}
}

func newAicardTemplateCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "Save a complete A2UI file as a new cloud template",
		Long:    "Validate a UTF-8 A2UI message-array file (at most 768 KiB) and create a new template asset. The MCP file argument contains JSON text, not a path or Base64. Return the server-issued template ID and version. Does not send a card or activate a business binding. Dry-run performs only local validation. An uncertain write is never retried automatically.",
		Example: "  dws aicard template create --mcp-id <MCP_ID> --file template.a2ui.json --format json",
		Flags: []LeafFlag{
			{Name: "file", Required: true, Trim: true, Usage: "Path to a complete UTF-8 A2UI message-array JSON file"},
			aicardTemplateServiceFlag(),
		},
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "template.create", Group: "template", CanonicalPath: "aicard.template.create", CLIPath: "aicard template create", PrimaryCLIPath: "aicard template create"},
			Description: "Create a new cloud template from a validated complete A2UI file",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Validates a local file, resolves the selected published MCP service, and invokes create_aicard_template once"},
			Selection:   contract.SelectionSpec{AgentSummary: "Register a new reusable template asset and return its ID/version", UseWhen: []string{"Save a complete card as a new template", "Derive a separate template from an existing one"}, AvoidWhen: []string{"Use template update to revise the same ID; use chat to send a card"}, Examples: []string{"dws aicard template create --mcp-id <MCP_ID> --file template.a2ui.json"}, ExampleDispositions: aicardTemplateFileExampleDispositions()},
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Result:      aicardTemplateWriteResult(),
		},
		RunE: runAicardTemplateCreate,
	})
}

func newAicardTemplateGetCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "get", Short: "Retrieve a known template ID/version into a new file",
		Long:    "Read a template by its explicit ID and positive int64 version string. --out must be a new file distinct from the global --output destination; existing files are never overwritten. Template strings and keys must not contain unpaired UTF-16 surrogate escapes. The current service may fall back to its latest content for an absent version and does not report the actual version. versionConfirmed is therefore false; compare templateDigest with an approved business binding before strict use. Dry-run does not call the service or write a template file. This is not keyword search.",
		Example: "  dws aicard template get --mcp-id <MCP_ID> --template-id <ID> --version 1 --out template.a2ui.json --format json",
		Flags: []LeafFlag{
			{Name: "template-id", Required: true, Trim: true, Usage: "Server-issued template ID", Bind: "templateId"},
			{Name: "version", Required: true, Trim: true, Usage: "Requested template version: positive decimal int64 string"},
			{Name: "out", Required: true, Trim: true, Usage: "New destination file; refuses to overwrite an existing path"},
			aicardTemplateServiceFlag(),
		},
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "template.get", Group: "template", CanonicalPath: "aicard.template.get", CLIPath: "aicard template get", PrimaryCLIPath: "aicard template get"},
			Description: "Retrieve a known template into a new local file without claiming its actual version",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Invokes get_aicard_template through fresh published-MCP discovery and saves validated content locally"},
			Selection:   contract.SelectionSpec{AgentSummary: "Read a known template ID/version before reuse or editing", UseWhen: []string{"An explicit cloud template ID and version are available"}, AvoidWhen: []string{"Not a search by name or scene; resolve a missing ID from business configuration or the user"}, Examples: []string{"dws aicard template get --mcp-id <MCP_ID> --template-id <ID> --version 1 --out template.a2ui.json"}},
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: json.RawMessage(`{"type":"object","required":["executed","templateId","requestedVersion","versionConfirmed","filePath"],"properties":{"executed":{"type":"boolean","description":"Whether the remote retrieval was executed"},"templateId":{"type":"string","description":"Requested template ID"},"requestedVersion":{"type":"string","description":"Requested version; not a confirmed actual version"},"versionConfirmed":{"type":"boolean","const":false,"description":"False because the service does not report its actual version"},"filePath":{"type":"string","description":"Absolute destination path; written only after successful non-dry-run retrieval"},"templateDigest":{"type":"string","description":"Algorithm-tagged digest of normalized retrieved content"},"digestAlgorithm":{"type":"string","description":"Versioned normalization and hash algorithm"},"versionWarning":{"type":"string","description":"Current service fallback limitation"},"mcpId":{"type":"string","description":"Selected published MCP service"},"inputSchemaValidation":{"type":"string","description":"Evidence from fresh MCP input validation"},"inputSchemaDigest":{"type":"string","description":"Digest of the input schema used for the invocation"}}}`)},
		},
		RunE: runAicardTemplateGet,
	})
}

func newAicardTemplateUpdateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "update", Short: "Append a version to an existing cloud template",
		Long:    "Validate a complete UTF-8 A2UI file (at most 768 KiB) and append a new version under the supplied template ID. Requires the service's creator permissions. Does not activate the version, migrate existing cards, or update card data. Dry-run performs only local validation. The service has no compare-and-swap or idempotency contract; an uncertain write is never retried automatically.",
		Example: "  dws aicard template update --mcp-id <MCP_ID> --template-id <ID> --file revised.a2ui.json --format json",
		Flags: []LeafFlag{
			{Name: "template-id", Required: true, Trim: true, Usage: "Existing server-issued template ID", Bind: "templateId"},
			{Name: "file", Required: true, Trim: true, Usage: "Path to the complete revised A2UI message-array JSON file"},
			aicardTemplateServiceFlag(),
		},
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "non_idempotent"},
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "template.update", Group: "template", CanonicalPath: "aicard.template.update", CLIPath: "aicard template update", PrimaryCLIPath: "aicard template update"},
			Description: "Append a server-assigned version to an existing template asset",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "Validates a local file and invokes update_aicard_template once under the selected identity"},
			Selection:   contract.SelectionSpec{AgentSummary: "Revise a template while retaining its ID and leaving activation to business configuration", UseWhen: []string{"Explicitly revise an existing template asset"}, AvoidWhen: []string{"Use create for a separate derived asset; use chat message update-a2ui-card for a sent instance"}, Examples: []string{"dws aicard template update --mcp-id <MCP_ID> --template-id <ID> --file revised.a2ui.json"}, ExampleDispositions: aicardTemplateFileExampleDispositions()},
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest},
			Result:      aicardTemplateWriteResult(),
		},
		RunE: runAicardTemplateUpdate,
	})
}

func aicardTemplateWriteResult() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: json.RawMessage(`{"type":"object","required":["executed","templateDigest","digestAlgorithm"],"properties":{"executed":{"type":"boolean","description":"Whether the remote write was executed"},"templateId":{"type":"string","description":"Server-issued template ID; absent for create dry-run"},"version":{"type":"string","description":"Server-assigned version; absent during dry-run"},"templateDigest":{"type":"string","description":"Algorithm-tagged digest of normalized submitted content"},"digestAlgorithm":{"type":"string","description":"Versioned normalization and hash algorithm"},"mcpId":{"type":"string","description":"Selected published MCP service"},"inputSchemaValidation":{"type":"string","description":"Evidence from fresh MCP input validation"},"inputSchemaDigest":{"type":"string","description":"Digest of the input schema used for the invocation"}}}`)}
}
