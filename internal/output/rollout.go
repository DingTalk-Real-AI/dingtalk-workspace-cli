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
	ContractV2     ContractMode = "v2"
)

// RolloutState is internal release metadata. Consumers never select it. A
// command advances through validation and activation; rollback is performed by
// changing the command declaration/release, not by changing Agent argv.
type RolloutState string

const (
	RolloutLegacyOnly   RolloutState = "legacy_only"
	RolloutDualValidate RolloutState = "dual_validate"
	RolloutV2Active     RolloutState = "v2_active"
	RolloutV2Stable     RolloutState = "v2_stable"
	RolloutV2Only       RolloutState = "v2_only"
)

const rolloutAnnotation = "dws.output.rollout"

func ParseRolloutState(raw string) (RolloutState, error) {
	state := RolloutState(strings.TrimSpace(raw))
	switch state {
	case RolloutLegacyOnly, RolloutDualValidate, RolloutV2Active, RolloutV2Stable, RolloutV2Only:
		return state, nil
	default:
		return "", fmt.Errorf("invalid output rollout state %q", raw)
	}
}

// ValidateRolloutTransition protects the release order. It is consumed by CI
// against the checked-in migration ledger; it is not an end-user capability.
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
	case RolloutV2Active:
		return 2
	case RolloutV2Stable:
		return 3
	case RolloutV2Only:
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
	case RolloutV2Active, RolloutV2Stable, RolloutV2Only:
		return ContractV2
	default:
		return ContractLegacy
	}
}

func UsesV2(cmd *cobra.Command) bool { return ActiveContract(cmd) == ContractV2 }

// ValidateV2Format is retained for compatibility. All commands normalize an
// unknown presentation value to their fallback and emit a diagnostic warning.
func ValidateV2Format(cmd *cobra.Command) error {
	return nil
}
