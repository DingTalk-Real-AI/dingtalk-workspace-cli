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

package interfacesnapshot

// flagTypeChange identifies one flag type migration exactly: which command,
// which flag, and in which direction. Nothing is matched by wildcard — an entry
// that differs in any of the four fields does not apply.
//
// CommandPath is the canonical Command.Path form, which always includes the root
// command name ("dws minutes permission apply"). It is deliberately *not* the
// alias-expanded accepted path: a command reachable through an alias enters
// compareEffectiveFlags once per accepted spelling, and keying on the accepted
// path would let every alias spelling bypass the exemption and re-report the
// change.
//
// This table is duplicated in scripts/policy/interface-baseline/reviewed.go and
// the two must stay identical; TestCrossPlatformCoverageReviewedFlagTypeTableMatchesInterfaceBaseline
// fails if they drift. The duplication is forced rather than chosen:
// check-authoritative-interface-baselines.sh copies the whole
// scripts/policy/interface-baseline directory into a worktree checked out at a
// historical revision and builds it there, so that copy cannot import a package
// this branch adds.
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
// accepted. The reverse is not a candidate for this table — widening an int
// flag back to string lets values the parser used to reject reach RunE, which
// is a genuine loosening of validation rather than a type migration.
//
// The table alone does not admit a change: reviewedFlagTypeChange is consulted
// only when nothing else about the flag moved. See compareEffectiveFlags.
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

// flagContractOtherwiseChanged reports whether anything this gate checks about a
// flag, other than its type, differs between the two revisions. A reviewed type
// migration is only accepted when this returns false, so the exemption cannot
// carry an unrelated regression in behind it.
//
// Only the fields compareEffectiveFlags actually enforces are compared. Default
// is deliberately excluded: a string → int migration always changes it ("" to
// "0"), so including it would make every entry in the table dead. Deprecated is
// excluded for the same reason it is not a blocking change on its own.
func flagContractOtherwiseChanged(oldFlag, newFlag Flag) bool {
	if !oldFlag.Required && newFlag.Required {
		return true
	}
	if oldFlag.Shorthand != "" && newFlag.Shorthand != oldFlag.Shorthand {
		return true
	}
	if oldFlag.NoOpt != "" && newFlag.NoOpt != oldFlag.NoOpt {
		return true
	}
	if !oldFlag.Hidden && newFlag.Hidden {
		return true
	}
	if oldFlag.AliasOf != "" && newFlag.AliasOf != oldFlag.AliasOf {
		return true
	}
	return false
}
