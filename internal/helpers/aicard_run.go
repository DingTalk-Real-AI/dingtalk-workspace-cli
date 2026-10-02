// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"io/fs"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/skills"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func aicardProtocol() (*a2ui.Protocol, error) {
	// The path is a fixed subdirectory of the embedded Skill filesystem.
	source, _ := fs.Sub(skills.FS, "multi/dingtalk-aicard")
	return a2ui.Bundled(source)
}

var aicardLoadProtocol = aicardProtocol
var aicardLint = (*a2ui.Protocol).Lint
var aicardPreflight = (*a2ui.Protocol).Preflight

func aicardFailure(cmd *cobra.Command, kind, message string, details map[string]any) error {
	return output.StoreResult(cmd.Context(), output.Failure(&output.ErrorInfo{Type: kind, Message: message, Details: details}))
}

func aicardReport(cmd *cobra.Command, report a2ui.Report) error {
	if report.Valid {
		return output.StoreResult(cmd.Context(), output.Success(report))
	}
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	var details map[string]any
	_ = json.Unmarshal(b, &details) // b was produced by json.Marshal above.
	return aicardFailure(cmd, "validation", "A2UI syntax or protocol structure is invalid", details)
}

func runAicardLint(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	self, _ := cmd.Flags().GetBool("self-check")
	fragment, _ := cmd.Flags().GetBool("fragment")
	emit, _ := cmd.Flags().GetBool("emit")
	checkMode, _ := cmd.Flags().GetString("preflight")
	if checkMode != "" && (checkMode != "new-card" && checkMode != "resources" || self || fragment) {
		return aicardFailure(cmd, "validation", "Preflight requires a complete message file; use new-card or resources", nil)
	}
	if (strings.TrimSpace(file) == "") == !self || self && (fragment || emit) || fragment && emit {
		return aicardFailure(cmd, "validation", "Specify exactly one of --file and --self-check; --emit and --fragment are mutually exclusive and require --file", nil)
	}
	p, err := aicardLoadProtocol()
	if err != nil {
		return aicardFailure(cmd, "internal", "Cannot load the embedded protocol: "+err.Error(), nil)
	}
	if self {
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{"ready": true, "manifest": p.Manifest(), "renderingVerified": false}))
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return aicardFailure(cmd, "validation", "Cannot read file: "+err.Error(), nil)
	}
	report, err := aicardLint(p, data, fragment, emit || checkMode != "")
	if err != nil {
		return aicardFailure(cmd, "internal", "Structural validation could not be completed: "+err.Error(), nil)
	}
	if report.Valid && checkMode != "" {
		checked, err := aicardPreflight(p, report.A2UIMessages, checkMode)
		if err != nil {
			return aicardFailure(cmd, "validation", err.Error(), nil)
		}
		report.Preflight = checked
		if !emit {
			report.A2UIMessages = nil
		}
		if checked["valid"] != true {
			b, _ := json.Marshal(report)
			var details map[string]any
			_ = json.Unmarshal(b, &details)
			return aicardFailure(cmd, "validation", "Offline preflight failed; valid represents only the Schema result", details)
		}
	}
	return aicardReport(cmd, report)
}

func runAicardExplain(cmd *cobra.Command, args []string) error {
	p, err := aicardLoadProtocol()
	if err != nil {
		return aicardFailure(cmd, "internal", "Cannot load the embedded protocol: "+err.Error(), nil)
	}
	compact, _ := cmd.Flags().GetBool("compact")
	if len(args) > 1 {
		result := p.ExplainMany(args, compact)
		for _, name := range args {
			if p.Explain(name)["kind"] == "unknown" {
				return aicardFailure(cmd, "validation", "At least one name is not registered in the protocol", result)
			}
		}
		return output.StoreResult(cmd.Context(), output.Success(result))
	}
	result := p.Explain(args[0])
	if result["kind"] == "unknown" {
		return aicardFailure(cmd, "validation", "The name is not registered in the protocol", result)
	}
	return output.StoreResult(cmd.Context(), output.Success(result))
}

func runAicardPreview(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	summary, _ := cmd.Flags().GetString("summary")
	if strings.TrimSpace(file) == "" {
		return aicardFailure(cmd, "validation", "--file is required", nil)
	}
	if strings.TrimSpace(summary) == "" {
		return aicardFailure(cmd, "validation", "--summary must not be empty", nil)
	}
	p, err := aicardLoadProtocol()
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	report, err := aicardLint(p, data, false, true)
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	if !report.Valid {
		return aicardReport(cmd, report)
	}
	if err := p.CheckNewCard(report.A2UIMessages); err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	if commandDryRun(cmd) {
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{
			"executed": false, "requestAccepted": false, "deliveryVerified": false, "renderingVerified": false,
			"flowStatus": "PROCESSING", "summary": summary, "a2uiMessages": report.A2UIMessages,
		}, output.WithDryRun()))
	}
	userID, err := getCurrentUserID(cmd.Context())
	if err != nil {
		return err
	}
	receiver, err := resolveOpenDingTalkID(cmd.Context(), userID)
	if err != nil {
		return err
	}
	requestID, bizCardID := uuid.NewString(), uuid.NewString()
	args := map[string]any{
		"requestId": requestID, "bizCardId": bizCardID, "protocolVersion": "1.0",
		"supportForward": false, "flowStatus": defaultA2UIFlowStatus,
		"a2uiMessages": report.A2UIMessages, "summary": summary, "receiverOpenDingTalkId": receiver,
	}
	raw, err := CallMCPToolDataOnServer(cmd.Context(), "im", "create_and_send_a2ui_card", args)
	if err != nil {
		return aicardFailure(cmd, "api", "No confirmable send receipt; delivery state is unknown: "+err.Error(), map[string]any{"requestId": requestID, "bizCardId": bizCardID, "retryAutomatically": false})
	}
	receipt, ok := raw.(map[string]any)
	if !ok {
		return aicardFailure(cmd, "api", "Send response is not an object; delivery state is unknown, so do not retry automatically", map[string]any{"requestId": requestID, "bizCardId": bizCardID})
	}
	accepted, explicit := receipt["success"].(bool)
	if !explicit || !accepted {
		return aicardFailure(cmd, "api", "Send response did not explicitly confirm acceptance; retain the receipt and investigate before retrying", map[string]any{"requestId": requestID, "bizCardId": bizCardID, "receipt": receipt})
	}
	result := map[string]any{
		"requestAccepted": true, "executed": true, "deliveryVerified": false, "renderingVerified": false,
		"flowStatus": "PROCESSING", "requestId": requestID, "bizCardId": bizCardID, "receipt": receipt,
	}
	// A card creation openTaskId is not a current-user message send task.
	// Keep it in the receipt; query_message_send_status cannot verify this card.
	return output.StoreResult(cmd.Context(), output.Success(result))
}
