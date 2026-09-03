// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chat

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const reviewedChatShortcutInterfaceReason = "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref."

// reviewedChatShortcutContractCommands is the exact reviewed migration ledger
// for Chat shortcuts that were delivered by the former committed Schema
// catalog but did not yet carry #830's typed Contract declaration. Exact names
// deliberately prevent a future shortcut from entering Schema by accident.
var reviewedChatShortcutContractCommands = map[string]struct{}{
	"+category-add-conversation":    {},
	"+category-list-conversations":  {},
	"+category-remove-conversation": {},
	"+chat-add-bot":                 {},
	"+chat-audit-join":              {},
	"+chat-create":                  {},
	"+chat-get-by-id":               {},
	"+chat-list":                    {},
	"+chat-members-get":             {},
	"+chat-mute-member":             {},
	"+chat-quit":                    {},
	"+chat-remove-bot":              {},
	"+chat-role-remove":             {},
	"+chat-role-remove-user":        {},
	"+chat-transfer-owner":          {},
	"+chat-update":                  {},
	"+chat-update-icon":             {},
	"+chat-update-settings":         {},
	"+conversation-clear-messages":  {},
	"+conversation-clear-red-point": {},
	"+conversation-hide":            {},
	"+conversation-mark-read":       {},
	"+conversation-mark-unread":     {},
	"+conversation-mute":            {},
	"+conversation-set-top":         {},
	"+feed-group-query-item":        {},
	"+flag-cancel":                  {},
	"+flag-create":                  {},
	"+flag-list":                    {},
	"+messages-add-emoji":           {},
	"+messages-add-text-emotion":    {},
	"+messages-batch-recall-by-bot": {},
	"+messages-batch-send-by-bot":   {},
	"+messages-combine-forward":     {},
	"+messages-create-text-emotion": {},
	"+messages-forward":             {},
	"+messages-forward-topic":       {},
	"+messages-list":                {},
	"+messages-recall":              {},
	"+messages-recall-by-bot":       {},
	"+messages-remove-emoji":        {},
	"+messages-remove-text-emotion": {},
	"+messages-reply":               {},
	"+messages-resource-download":   {},
	"+messages-resource-url":        {},
	"+messages-send-by-bot":         {},
	"+messages-set-pin":             {},
	"+messages-set-top":             {},
	"+messages-unset-pin":           {},
	"+messages-unset-top":           {},
}

// withReviewedChatShortcutContracts ports the previously reviewed Chat Schema
// records into #830's typed declaration model. Existing explicit Contracts are
// preserved. Missing Contracts must be listed in the exact ledger above.
func withReviewedChatShortcutContracts(values ...shortcut.Shortcut) []shortcut.Shortcut {
	out := make([]shortcut.Shortcut, len(values))
	for i, value := range values {
		out[i] = value
		if value.Contract.Empty() {
			if _, reviewed := reviewedChatShortcutContractCommands[value.Command]; reviewed {
				out[i].Safety = reviewedChatShortcutSafety(value.Risk)
				out[i].Contract = reviewedChatShortcutContract(value)
			}
		}
		out[i].Contract.Parameters = append(
			out[i].Contract.Parameters,
			reviewedChatPrimaryParamDecls(value.Command)...,
		)
	}
	return out
}

func reviewedChatPrimaryParamDecls(command string) []contract.ParamDecl {
	switch command {
	case "+messages-send-by-bot", "+messages-batch-send-by-bot", "+messages-send-by-webhook":
		return []contract.ParamDecl{renamedRequiredChatParam("content", "text")}
	case "+messages-reply":
		return []contract.ParamDecl{
			renamedRequiredChatParam("group", "conversationId"),
			renamedRequiredChatParam("content", "text"),
		}
	default:
		return nil
	}
}

// renamedRequiredChatParam preserves the pre-rename Schema property and
// requiredness. InterfaceType stays unset because these Shortcut parameters
// historically published only their Cobra string type.
func renamedRequiredChatParam(name, property string) contract.ParamDecl {
	required := true
	return contract.ParamDecl{
		Name:     name,
		Property: property,
		Required: &required,
	}
}

func reviewedChatShortcutSafety(risk shortcut.Risk) contract.SafetySpec {
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

func reviewedChatShortcutContract(value shortcut.Shortcut) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(value.Command, "+"), "-", "_")
	cliPath := value.Service + " " + value.Command
	aliases := make([]string, 0, len(value.Aliases))
	for _, alias := range value.Aliases {
		aliases = append(aliases, value.Service+" "+alias)
	}
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
			Reason:       reviewedChatShortcutInterfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: value.Description,
			UseWhen:      []string{description},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID:      value.Service,
			Name:           name,
			CanonicalPath:  value.Service + "." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
			Aliases:        aliases,
		},
	}
}
