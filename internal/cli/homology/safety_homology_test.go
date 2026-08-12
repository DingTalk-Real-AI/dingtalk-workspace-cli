// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package homology

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// homologyProbeCaller is a throwaway ToolCaller for Execute-based confirmation
// probes. Schema source roots stay declaration-only (no InitDeps); the probe
// path initializes deps locally so leaf RunE wrappers do not nil-deref.
type homologyProbeCaller struct{}

func (homologyProbeCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{}, nil
}
func (homologyProbeCaller) Format() string { return "json" }
func (homologyProbeCaller) DryRun() bool   { return false }
func (homologyProbeCaller) Fields() string { return "" }
func (homologyProbeCaller) JQ() string     { return "" }

// TestUserRequiredSafetyHomologyWithRuntimeGate proves Catalog Safety and the
// executable confirmation gate share one source for every live user_required leaf:
//
//  1. ContractFinal.Safety.Confirmation == live ToolSpec.Confirmation
//  2. Runtime gate is installed: DeclareLeafMetadata (HasContractConfirmSafety),
//     Sheet protect marker, or framework NewCommand/Shortcut RunE (verified by
//     closed-stdin Execute → confirmation_required / 用户取消了操作 without --yes)
func TestUserRequiredSafetyHomologyWithRuntimeGate(t *testing.T) {
	// Declaration-only Schema roots intentionally skip InitDeps. Probes Execute
	// live leaves (including deprecated doc wrappers), so install a local
	// throwaway caller without touching SetDynamicServers. Restore the prior
	// deps pointer (including nil) — InitDeps(previousCaller) cannot.
	helpers.InitDepsForTest(t, homologyProbeCaller{})

	root := app.NewSchemaSourceRootCommand()
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "")
	}

	liveReg, err := cli.AssembleSchemaRegistry(root)
	if err != nil {
		t.Fatalf("AssembleSchemaRegistry: %v", err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry: %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry: %v", err)
	}

	liveByCanon := make(map[string]string) // canonical → confirmation
	liveToolByCanon := make(map[string]cli.ToolSpec)
	for _, product := range liveReg.Products {
		for _, tool := range product.Tools {
			liveByCanon[tool.Identity.CanonicalPath] = strings.TrimSpace(tool.Safety.Confirmation)
			liveToolByCanon[tool.Identity.CanonicalPath] = tool
		}
	}

	type row struct {
		canonical string
		cliPath   string
		gate      string
		detail    string
	}
	var fails []row
	var okRows []row
	checked := 0

	canonOrder := make([]string, 0, len(bound.Commands))
	boundByCanon := make(map[string]cli.BoundCommandSpec, len(bound.Commands))
	for _, cmd := range bound.Commands {
		if cmd.CanonicalPath == "" {
			continue
		}
		canonOrder = append(canonOrder, cmd.CanonicalPath)
		boundByCanon[cmd.CanonicalPath] = cmd
	}
	sort.Strings(canonOrder)

	for _, canonical := range canonOrder {
		cmd := boundByCanon[canonical]
		leaf := cmd.PrimaryCommand
		if leaf == nil {
			fails = append(fails, row{canonical, cmd.PrimaryCLIPath, "", "nil PrimaryCommand"})
			continue
		}
		final, hasFinal := contractfinal.RuntimeContractFinal(leaf)
		if !hasFinal || final.Safety == nil {
			continue
		}
		conf := strings.TrimSpace(final.Safety.Confirmation)
		if conf != "user_required" {
			continue
		}
		checked++
		cliPath := cmd.PrimaryCLIPath

		liveConf, ok := liveByCanon[canonical]
		if !ok {
			fails = append(fails, row{canonical, cliPath, "", "absent from live registry"})
			continue
		}
		if liveConf != "user_required" {
			fails = append(fails, row{canonical, cliPath, "", fmt.Sprintf(
				"ContractFinal=user_required but live ToolSpec.Confirmation=%q", liveConf)})
			continue
		}

		gate := ""
		switch {
		case helpers.HasContractConfirmSafety(leaf) && helpers.HasSheetMutationConfirmationGuard(leaf):
			gate = "declare_leaf+sheet_marker"
		case helpers.HasContractConfirmSafety(leaf):
			gate = "declare_leaf_confirm"
		case helpers.HasSheetMutationConfirmationGuard(leaf):
			gate = "sheet_protect"
		default:
			// NewLeafCommand / Shortcut: NewCommand registers ContractFinal from the
			// same contract.SafetySpec it wires into ConfirmSafety. Prove that contract.SafetySpec is
			// still user_required-operable (closed stdin → confirmation gate).
			leaf.SetIn(strings.NewReader(""))
			leaf.SetOut(io.Discard)
			leaf.SetErr(io.Discard)
			if err := corecmd.ConfirmSafety(leaf, *final.Safety); !isConfirmationGateError(err) {
				fails = append(fails, row{canonical, cliPath, "", fmt.Sprintf(
					"framework leaf: ConfirmSafety(ContractFinal.Safety) = %v, want confirmation gate", err)})
				continue
			}
			// Execute without --yes must not succeed (confirm-first or validate-then-confirm).
			if err := probeConfirmationGate(leaf); err == nil {
				fails = append(fails, row{canonical, cliPath, "",
					"framework leaf: Execute without --yes succeeded"})
				continue
			}
			gate = "framework_confirm_safety"
		}
		okRows = append(okRows, row{canonical, cliPath, gate, "ok"})
	}

	if checked == 0 {
		t.Fatal("no user_required ContractFinal leaves found")
	}
	if len(fails) != 0 {
		for _, f := range fails {
			t.Errorf("%s (%s): %s", f.canonical, f.cliPath, f.detail)
		}
		t.Fatalf("Safety homology failed: %d/%d user_required leaves", len(fails), checked)
	}

	byGate := map[string]int{}
	for _, r := range okRows {
		byGate[r.gate]++
	}
	t.Logf("user_required Safety homology OK: %d leaves gates=%v", checked, byGate)

	// Sheet transitional dual gate: every Sheet --yes-only protect marker must
	// also carry DeclareLeafMetadata ConfirmSafety. A bare sheet_protect leaf
	// would mean the outer guard was dropped from the homology pair (or the
	// contract wrap never installed), silently widening authorization.
	if bare := byGate["sheet_protect"]; bare != 0 {
		t.Fatalf("transitional Sheet dual gate broken: %d leaf(ves) have sheet_protect without declare_leaf_confirm", bare)
	}
	if dual := byGate["declare_leaf+sheet_marker"]; dual == 0 {
		t.Fatal("expected declare_leaf+sheet_marker leaves for Sheet transitional dual confirmation gate")
	}

	// Confirm/validate order is asserted by behavior, not by a claim of intent:
	// a "deferred" annotation can still confirm first at runtime. For every
	// user_required leaf that publishes a required parameter, an argument-less
	// invocation must report that missing parameter — reaching confirmation
	// instead is the RFC §5.1 inversion.
	//
	// Two forms declare guard-first ordering deliberately and are exempt: a Spec
	// ConfirmFirst field (the devapp write contract asserted by
	// TestEveryDevAppWriteCommandRequiresGuard) and the reviewed Sheet
	// confirmationGuards table, whose wrapper is --yes-only by design. Both are
	// declarations, not inferences, so the exemption cannot be reached by
	// accident.
	var orderFails []row
	for _, r := range okRows {
		leaf := boundByCanon[r.canonical].PrimaryCommand
		if leaf == nil || !requiresParameter(liveToolByCanon[r.canonical]) {
			continue
		}
		if corecmd.HasDeclaredConfirmFirst(leaf) || helpers.HasSheetMutationConfirmationGuard(leaf) {
			continue
		}
		if err := probeConfirmationGate(leaf); isConfirmationGateError(err) {
			orderFails = append(orderFails, row{r.canonical, r.cliPath, r.gate,
				fmt.Sprintf("confirms before reporting a missing required parameter: %v", err)})
		}
	}
	if len(orderFails) != 0 {
		for _, f := range orderFails {
			t.Errorf("%s (%s): %s", f.canonical, f.cliPath, f.detail)
		}
		t.Fatalf("confirm/validate order invariant failed: %d user_required leaf(ves)", len(orderFails))
	}
}

// requiresParameter reports whether the delivered ToolSpec publishes at least
// one required parameter, i.e. whether an argument-less invocation is expected
// to fail validation before anything else.
func requiresParameter(tool cli.ToolSpec) bool {
	for _, param := range tool.Parameters {
		if param.Required || param.CLIRequired {
			return true
		}
	}
	return false
}

func probeConfirmationGate(leaf *cobra.Command) error {
	root := leaf.Root()
	if root == nil {
		root = leaf
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	// Rebuild args from CommandPath relative to root.
	path := strings.TrimSpace(strings.TrimPrefix(leaf.CommandPath(), root.CommandPath()))
	fields := strings.Fields(path)
	root.SetArgs(fields)
	return root.Execute()
}

// isConfirmationGateError reports whether err is the confirmation gate itself.
//
// It classifies on the machine-readable reason rather than on prose: a Sheet
// destructive leaf's missing-flag error embeds the example
// "（获得用户确认后加 --yes）", so substring matching reported the confirmation gate
// for a correct validation error. The decline path is the one case that carries
// no reason, so it is matched by its exact message.
func isConfirmationGateError(err error) bool {
	if err == nil {
		return false
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) && strings.TrimSpace(typed.Reason) == "confirmation_required" {
		return true
	}
	return strings.Contains(err.Error(), "用户取消了操作")
}
