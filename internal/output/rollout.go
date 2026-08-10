// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ContractMode is an internal renderer identity. It is deliberately not a CLI
// flag: one command has exactly one externally active contract in one release.
type ContractMode string

const (
	ContractLegacy ContractMode = "legacy"
	// ContractUnified is the single framework-owned result shape. It is not a
	// consumer-selectable protocol version: a command either still renders its
	// historical legacy shape or has been promoted to the unified result.
	ContractUnified ContractMode = "unified"
)

// RolloutState is internal release metadata. Consumers never select it. A
// command advances through validation and activation; rollback is performed by
// changing the command declaration/release, not by changing Agent argv.
type RolloutState string

const (
	RolloutLegacyOnly    RolloutState = "legacy_only"
	RolloutDualValidate  RolloutState = "dual_validate"
	RolloutUnifiedActive RolloutState = "unified_active"
	RolloutUnifiedStable RolloutState = "unified_stable"
	RolloutUnifiedOnly   RolloutState = "unified_only"
)

const rolloutAnnotation = "dws.output.rollout"

func ParseRolloutState(raw string) (RolloutState, error) {
	state := RolloutState(strings.TrimSpace(raw))
	switch state {
	case RolloutLegacyOnly, RolloutDualValidate, RolloutUnifiedActive, RolloutUnifiedStable, RolloutUnifiedOnly:
		return state, nil
	default:
		return "", fmt.Errorf("invalid output rollout state %q", raw)
	}
}

// ValidateRolloutTransition defines the release order for rollout tooling. It
// is not an end-user capability. CI/ledger enforcement is wired separately.
func ValidateRolloutTransition(from, to RolloutState, rollback bool) error {
	if _, err := ParseRolloutState(string(from)); err != nil {
		return err
	}
	if _, err := ParseRolloutState(string(to)); err != nil {
		return err
	}
	fromRank, toRank := rolloutRank(from), rolloutRank(to)
	if fromRank == toRank || toRank == fromRank+1 {
		return nil
	}
	if rollback && toRank < fromRank {
		return nil
	}
	if toRank < fromRank {
		return fmt.Errorf("output rollout transition %s -> %s is a rollback and requires explicit rollback approval", from, to)
	}
	return fmt.Errorf("output rollout transition %s -> %s skips intermediate states", from, to)
}

func rolloutRank(state RolloutState) int {
	switch state {
	case RolloutLegacyOnly:
		return 0
	case RolloutDualValidate:
		return 1
	case RolloutUnifiedActive:
		return 2
	case RolloutUnifiedStable:
		return 3
	case RolloutUnifiedOnly:
		return 4
	default:
		return -1
	}
}

func SetCommandRollout(cmd *cobra.Command, state RolloutState) {
	if cmd == nil {
		return
	}
	if _, err := ParseRolloutState(string(state)); err != nil {
		panic(err)
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[rolloutAnnotation] = string(state)
}

// CommandRollout fails closed. Merely linking Framework 2.0 into the binary
// cannot change an undeclared command's wire contract.
func CommandRollout(cmd *cobra.Command) RolloutState {
	if cmd != nil && cmd.Annotations != nil {
		if state, err := ParseRolloutState(cmd.Annotations[rolloutAnnotation]); err == nil {
			return state
		}
	}
	return RolloutLegacyOnly
}

// ActiveContract is the only contract exposed by a command in this release.
func ActiveContract(cmd *cobra.Command) ContractMode {
	switch CommandRollout(cmd) {
	case RolloutUnifiedActive, RolloutUnifiedStable, RolloutUnifiedOnly:
		return ContractUnified
	default:
		return ContractLegacy
	}
}

func UsesUnifiedResult(cmd *cobra.Command) bool { return ActiveContract(cmd) == ContractUnified }

// ValidateUnifiedFormat is retained for compatibility. All commands normalize an
// unknown presentation value to their fallback and emit a diagnostic warning.
func ValidateUnifiedFormat(cmd *cobra.Command) error {
	return nil
}
