// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func minutesSmartContract(command, description, useWhen string, avoidWhen, examples, aliases []string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	identityAliases := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		identityAliases = append(identityAliases, "minutes "+alias)
	}
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           name,
			CanonicalPath:  "minutes." + name,
			CLIPath:        "minutes " + command,
			PrimaryCLIPath: "minutes " + command,
			Aliases:        identityAliases,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The executable Shortcut owns strict Minutes response validation, orchestration, completeness and final output; no single RPC represents the final command contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
	}
}

// finalizeMinutesSmartShortcut keeps Minutes shortcuts that live in the smart
// package under the same declare=delivery contract as the Minutes package.
func finalizeMinutesSmartShortcut(value shortcut.Shortcut) shortcut.Shortcut {
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
			if !smartMinutesContains(constraint.Flags, flag.Name) || strings.Contains(flag.Desc, evidence) {
				continue
			}
			flag.Desc = strings.TrimRight(flag.Desc, "；。 ") + "；约束：" + evidence
		}
		for parameterIndex := range value.Contract.Parameters {
			parameter := &value.Contract.Parameters[parameterIndex]
			if !smartMinutesContains(constraint.Flags, parameter.Name) || strings.Contains(parameter.Description, evidence) {
				continue
			}
			parameter.Description = strings.TrimRight(parameter.Description, "；。 ") + "；约束：" + evidence
		}
	}
	return value
}

func smartMinutesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
