// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func aitableCompositeContract(command, description, useWhen, avoidWhen, example string) corecmd.ContractDecl {
	name := strings.TrimPrefix(command, "+")
	name = strings.ReplaceAll(name, "-", "_")
	cliPath := "aitable " + command
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_" + name,
			CanonicalPath:  "aitable.shortcut_" + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "DWS reviewed composite orchestration with explicit read-back success predicates, partial-effect reporting, and resumable checkpoints; no single MCP operation represents the full command contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    []string{avoidWhen},
			Examples:     []string{example},
		},
	}
}
