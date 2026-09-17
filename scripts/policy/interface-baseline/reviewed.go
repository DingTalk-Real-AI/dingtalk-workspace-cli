// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import "github.com/spf13/pflag"

// flagTypeChange identifies one flag type migration exactly: which command,
// which flag, and in which direction. Nothing is matched by wildcard — an entry
// that differs in any of the four fields does not apply.
//
// CommandPath is the display form produced by displayPath ("dws minutes
// permission apply"), not the dot-separated internal path. Writing the internal
// form here would silently disable the exemption instead of failing loudly, so
// TestReviewedFlagTypeChangeKeysUseDisplayPaths recomputes every key through
// displayPath rather than trusting the spelling below.
//
// This table is duplicated in internal/interfacesnapshot/reviewed.go and the two
// must stay identical; the interfacesnapshot package has a test that fails if
// they drift. The duplication is forced rather than chosen:
// check-authoritative-interface-baselines.sh copies this whole directory into a
// worktree checked out at a historical revision and builds it there, so nothing
// here may import a package this branch adds — that build would fail with an
// unresolved import instead of reporting a compatibility result.
type flagTypeChange struct {
	CommandPath string
	Flag        string
	From        string
	To          string
}

// reviewedFlagTypeChanges enumerates the individually reviewed flag type
// migrations this gate accepts. A flag type is part of the published CLI
// contract, and a type swap cannot be proven safe from the type names alone —
// whether old invocations still parse depends on the concrete value domain the
// command enforces. So a migration is accepted only when this exact command,
// flag, and old→new pair appear here. Adding an entry is a contract decision
// and belongs in review, not in a feature change.
//
// Entries are direction-sensitive by construction: "string" → "int" is a
// separate key from "int" → "string" and only the reviewed direction is
// accepted. An int → string migration is not automatically safe: it must keep
// accepting every historically successful numeric spelling and constrain any
// newly accepted strings in RunE.
//
// The table alone does not admit a change: reviewedFlagTypeChange is consulted
// only when nothing else about the flag moved. See the callers in
// checkCompatibility and mergeContracts.
var reviewedFlagTypeChanges = map[flagTypeChange]struct{}{
	// "dws minutes permission apply --policy" moved from a String flag parsed
	// with strconv.ParseInt(v, 10, 64) in RunE to a native Int flag, keeping the
	// same [2,4] domain check in RunE.
	//
	// This is admissible because the historical set of *successful* invocations
	// is a subset of the new one, not merely similar to it:
	//
	//   - Old: the value had to parse as base-10 and land in [2,4]. That is
	//     exactly "2", "3", "4" plus sign/leading-zero spellings of them
	//     ("+3", "03", "003", ...).
	//   - New: pflag's intValue.Set parses with strconv.ParseInt(s, 0, 64) —
	//     base 0 — and RunE still enforces [2,4]. Every base-10 spelling above
	//     resolves to the same number under base 0, so no historical success
	//     becomes a failure.
	//   - Base 0 additionally accepts "0x3" and friends, which the old parser
	//     rejected. That widens the accepted set; it cannot break a caller who
	//     was already succeeding.
	//   - Values outside [2,4] and non-numeric values still fail. Only the
	//     failure *message and timing* move, from RunE to flag parsing.
	//
	// The flag's default value changes from "" to "0" as an unavoidable
	// consequence of the type. Neither gate compares defaults, and a default is
	// not reachable by a caller here because RunE requires the flag to be
	// explicitly set, so this is not a contract change.
	{CommandPath: "dws minutes permission apply", Flag: "policy", From: "string", To: "int"}: {},

	// "dws chat message update-card --flow-status" moves from a native Int flag
	// to String so the new A2UI engine can accept its reviewed status names. The
	// default streaming path still parses with base 0 and enforces [1,5], which
	// preserves every numeric spelling accepted by the historical pflag Int.
	// A2UI additionally accepts the decimal compatibility values 1-9 and maps
	// them to the corresponding enum names before transport.
	{CommandPath: "dws chat message update-card", Flag: "flow-status", From: "int", To: "string"}: {},
}

// reviewedFlagTypeChange reports whether this exact command, flag and direction
// is a reviewed migration. Callers must additionally establish that the rest of
// the flag's contract is unchanged before skipping the failure.
func reviewedFlagTypeChange(commandPath, flag, from, to string) bool {
	_, reviewed := reviewedFlagTypeChanges[flagTypeChange{
		CommandPath: commandPath,
		Flag:        flag,
		From:        from,
		To:          to,
	}]
	return reviewed
}

// flagContractOtherwiseChanged reports whether anything checkCompatibility
// enforces about a flag, other than its type, regressed against the baseline. A
// reviewed type migration is only accepted when this returns false, so the
// exemption cannot carry an unrelated regression in behind it.
//
// Every condition here mirrors one of the checks that follow the type check in
// checkCompatibility and must stay in step with them. The pairing is anchored by
// tests that bundle each regression with the reviewed type change and require
// the type failure to reappear, so a condition that drifts out of sync fails
// loudly instead of silently widening the exemption.
func flagContractOtherwiseChanged(expected flagContract, actual *pflag.Flag, actualScope string) bool {
	if expected.Shorthand != "" && actual.Shorthand != expected.Shorthand {
		return true
	}
	if expected.RequiredSet && !expected.Required && isRequiredFlag(actual) {
		return true
	}
	if expected.HiddenSet && !expected.Hidden && actual.Hidden {
		return true
	}
	if expected.NoOptSet && actual.NoOptDefVal != expected.NoOpt {
		return true
	}
	if expected.ScopeSet && expected.Scope == "persistent" && actualScope == "local" {
		return true
	}
	return false
}

// mergedFlagContractOtherwiseChanged is the mergeContracts counterpart of
// flagContractOtherwiseChanged, comparing two recorded contracts rather than a
// contract against a live pflag definition. It mirrors the checks that follow
// the type check in mergeContracts.
func mergedFlagContractOtherwiseChanged(oldFlag, newFlag flagContract) bool {
	if oldFlag.Shorthand != "" && oldFlag.Shorthand != newFlag.Shorthand {
		return true
	}
	if oldFlag.RequiredSet && !oldFlag.Required && newFlag.Required {
		return true
	}
	if oldFlag.HiddenSet && !oldFlag.Hidden && newFlag.Hidden {
		return true
	}
	if oldFlag.NoOptSet && newFlag.NoOptSet && oldFlag.NoOpt != newFlag.NoOpt {
		return true
	}
	if oldFlag.ScopeSet && oldFlag.Scope == "persistent" && newFlag.Scope == "local" {
		return true
	}
	return false
}
