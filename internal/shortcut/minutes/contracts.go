// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func minutesContract(command, description, useWhen string, avoidWhen []string, examples []string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           name,
			CanonicalPath:  "minutes." + name,
			CLIPath:        "minutes " + command,
			PrimaryCLIPath: "minutes " + command,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The executable Shortcut owns validation, orchestration, completeness and verification across one or more Minutes RPCs; no single RPC represents the final command contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
	}
}

func withMinutesDryRun(decl corecmd.ContractDecl, kind string, remoteReads bool) corecmd.ContractDecl {
	decl.DryRun = &contract.DryRunSpec{PreviewKind: kind, RemoteReads: remoteReads}
	return decl
}

func minutesDryRunPayload(kind, operation string, payload map[string]any) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["operation"] = operation
	payload["dry_run"] = true
	payload["dryRun"] = true
	payload["preview_kind"] = kind
	payload["executed"] = false
	return payload
}

// finalizeMinutesShortcuts keeps the human Shortcut declaration and the final
// Agent contract on one source of truth. Custom validation is prose-only in the
// Schema wire, so publish its exact evidence on every affected flag as well.
func finalizeMinutesShortcuts(values ...shortcut.Shortcut) []shortcut.Shortcut {
	finalized := make([]shortcut.Shortcut, len(values))
	for index, value := range values {
		value.Contract.Selection.AgentSummary = value.Description
		value.Contract.Selection.UseWhen = []string{value.Intent}
		for _, constraint := range value.Constraints {
			if constraint.Kind != shortcut.ConstraintCustom {
				continue
			}
			evidence := strings.TrimSpace(constraint.Description)
			if evidence == "" {
				continue
			}
			for flagIndex := range value.Flags {
				flag := &value.Flags[flagIndex]
				if !containsString(constraint.Flags, flag.Name) || strings.Contains(flag.Desc, evidence) {
					continue
				}
				flag.Desc = strings.TrimRight(flag.Desc, "；。 ") + "；约束：" + evidence
			}
			for parameterIndex := range value.Contract.Parameters {
				parameter := &value.Contract.Parameters[parameterIndex]
				if !containsString(constraint.Flags, parameter.Name) || strings.Contains(parameter.Description, evidence) {
					continue
				}
				parameter.Description = strings.TrimRight(parameter.Description, "；。 ") + "；约束：" + evidence
			}
		}
		finalized[index] = value
	}
	return finalized
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
