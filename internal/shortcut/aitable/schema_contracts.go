// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package aitable

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const reviewedAITableShortcutInterfaceReason = "Reviewed AI Table Shortcut adapter: the executable CLI owns validation, orchestration, output projection, and confirmation; no single pinned MCP interface represents the complete command contract."

// reviewedAITableShortcutContractCommands is the exact migration ledger for
// the 53 executable AI Table shortcuts that were hidden only because they
// predated typed Contract declarations. New shortcuts must declare Safety and
// Contract directly and must never be added to this migration ledger.
var reviewedAITableShortcutContractCommands = map[string]struct{}{
	"+advperm-disable":           {},
	"+advperm-enable":            {},
	"+attachment-upload":         {},
	"+base-copy":                 {},
	"+base-delete":               {},
	"+base-get-primary-doc-id":   {},
	"+base-update":               {},
	"+chart-delete":              {},
	"+chart-share-get":           {},
	"+chart-share-update":        {},
	"+chart-update":              {},
	"+dashboard-arrange":         {},
	"+dashboard-delete":          {},
	"+dashboard-share-get":       {},
	"+dashboard-share-update":    {},
	"+dashboard-update":          {},
	"+export-data":               {},
	"+field-delete":              {},
	"+field-update":              {},
	"+form-delete":               {},
	"+form-field-hide":           {},
	"+form-field-update":         {},
	"+form-share-update":         {},
	"+form-update":               {},
	"+import-data":               {},
	"+import-upload":             {},
	"+record-delete":             {},
	"+record-primary-doc-create": {},
	"+record-primary-doc-get":    {},
	"+record-update":             {},
	"+record-upsert":             {},
	"+role-create":               {},
	"+role-delete":               {},
	"+role-get":                  {},
	"+role-update":               {},
	"+section-create":            {},
	"+section-delete":            {},
	"+section-move-node":         {},
	"+section-rename":            {},
	"+section-reorder":           {},
	"+table-delete":              {},
	"+table-update":              {},
	"+view-delete":               {},
	"+view-duplicate":            {},
	"+view-lock":                 {},
	"+view-set-fill-color-rule":  {},
	"+view-set-frozen-cols":      {},
	"+view-set-row-height":       {},
	"+view-update":               {},
	"+workflow-disable":          {},
	"+workflow-enable":           {},
	"+workflow-get":              {},
	"+workflow-list":             {},
}

func withReviewedAITableShortcutContracts(values ...shortcut.Shortcut) []shortcut.Shortcut {
	out := make([]shortcut.Shortcut, len(values))
	for index, value := range values {
		out[index] = value
		if !value.Contract.Empty() {
			continue
		}
		if _, reviewed := reviewedAITableShortcutContractCommands[value.Command]; !reviewed {
			continue
		}
		out[index].Safety = reviewedAITableShortcutSafety(value.Risk)
		out[index].Contract = reviewedAITableShortcutContract(value)
	}
	return out
}

func reviewedAITableShortcutSafety(risk shortcut.Risk) contract.SafetySpec {
	switch risk {
	case shortcut.RiskWrite:
		return contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		}
	case shortcut.RiskHighWrite:
		return contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		}
	default:
		return contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		}
	}
}

func reviewedAITableShortcutContract(value shortcut.Shortcut) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(value.Command, "+"), "-", "_")
	cliPath := "aitable " + value.Command
	description := strings.TrimSpace(value.Intent)
	if description == "" {
		description = strings.TrimSpace(value.Description)
	}
	examples := append([]string(nil), value.Tips...)
	if len(examples) > 2 {
		examples = examples[:2]
	}
	return corecmd.ContractDecl{
		Title:       value.Description,
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       reviewedAITableShortcutInterfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: value.Description,
			UseWhen:      []string{description},
			AvoidWhen: []string{
				"只需未经适配的底层参数或原始响应时改用对应 aitable 原子命令；电子表格单元格操作改用 sheet",
			},
			Examples: examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           name,
			CanonicalPath:  "aitable." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
	}
}
