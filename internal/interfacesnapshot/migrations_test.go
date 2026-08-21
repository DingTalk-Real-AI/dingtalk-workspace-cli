// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

type flagMigrationErrorReader struct{}

func (flagMigrationErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("injected read failure")
}

const validFlagMigrationManifestJSON = `{
  "version": 1,
  "migrations": [
    {
      "command": "dws chat message recall",
      "legacy": {
        "name": "msg-id",
        "before": {"present": true, "type": "string", "required": true, "scope": "local"},
        "after": {"present": true, "type": "string", "hidden": true, "scope": "local", "alias_of": "message-id"}
      },
      "canonical": {
        "name": "message-id",
        "before": {"present": false},
        "after": {"present": true, "type": "string", "required": true, "scope": "local"}
      },
      "state": "pending",
      "reason": "Reviewed message identifier flag migration."
    }
  ]
}`

const validRequirednessMigrationManifestJSON = `{
  "version": 1,
  "migrations": [
    {
      "kind": "requiredness_change",
      "command": "dws report entry submit",
      "flag": {
        "name": "to-user-ids",
        "before": {"present": true, "type": "string", "scope": "local"},
        "after": {"present": true, "type": "string", "required": true, "scope": "local"}
      },
      "state": "pending",
      "reason": "Reject report submissions that have no visible recipient."
    }
  ]
}`

func optionalFlagMigrationManifestJSON() string {
	manifest := strings.Replace(
		validFlagMigrationManifestJSON,
		`"before": {"present": true, "type": "string", "required": true, "scope": "local"}`,
		`"before": {"present": true, "type": "string", "scope": "local"}`,
		1,
	)
	return strings.Replace(
		manifest,
		`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
		`"after": {"present": true, "type": "string", "scope": "local"}`,
		1,
	)
}

func hiddenCanonicalFlagMigrationManifestJSON() string {
	return strings.Replace(
		validFlagMigrationManifestJSON,
		`"before": {"present": false}`,
		`"before": {"present": true, "type": "string", "hidden": true, "scope": "local"}`,
		1,
	)
}

func TestApprovedFlagMigrationManifestRemainsValid(t *testing.T) {
	manifest, err := os.Open("../../scripts/policy/interface-migrations/approved-flag-migrations-v1.json")
	if err != nil {
		t.Fatalf("open approved flag migration manifest: %v", err)
	}
	defer manifest.Close()

	if _, err := ReadFlagMigrationManifest(manifest); err != nil {
		t.Fatalf("approved flag migration manifest is invalid: %v", err)
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestRejectsUnknownFields(t *testing.T) {
	_, err := ReadFlagMigrationManifest(strings.NewReader(`{
  "version": 1,
  "migrations": [],
  "allow_all_chat_flags": true
}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ReadFlagMigrationManifest() error = %v, want unknown field rejection", err)
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestRejectsDuplicateFields(t *testing.T) {
	for _, input := range []string{
		`{"version": 999, "version": 1, "migrations": []}`,
		strings.Replace(validFlagMigrationManifestJSON, `"state": "pending"`, `"state": "consumed", "state": "pending"`, 1),
	} {
		_, err := ReadFlagMigrationManifest(strings.NewReader(input))
		if err == nil || !strings.Contains(err.Error(), "duplicate field") {
			t.Fatalf("ReadFlagMigrationManifest() error = %v, want duplicate field rejection", err)
		}
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestRejectsNonCanonicalFields(t *testing.T) {
	for _, input := range []string{
		`{"Version": 1, "migrations": []}`,
		`{"version": 1, "Version": 1, "migrations": []}`,
		strings.Replace(validFlagMigrationManifestJSON, `"state": "pending"`, `"State": "pending"`, 1),
	} {
		_, err := ReadFlagMigrationManifest(strings.NewReader(input))
		if err == nil || !strings.Contains(err.Error(), "non-canonical field") {
			t.Fatalf("ReadFlagMigrationManifest() error = %v, want non-canonical field rejection", err)
		}
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestRejectsNullScalars(t *testing.T) {
	for _, input := range []string{
		strings.Replace(validFlagMigrationManifestJSON, `"required": true`, `"required": null`, 1),
		strings.Replace(validFlagMigrationManifestJSON, `"scope": "local"`, `"scope": "local", "shorthand": null`, 1),
		`{"version": null, "migrations": []}`,
	} {
		_, err := ReadFlagMigrationManifest(strings.NewReader(input))
		if err == nil || !strings.Contains(err.Error(), "must be") || !strings.Contains(err.Error(), "null") {
			t.Fatalf("ReadFlagMigrationManifest() error = %v, want scalar null rejection", err)
		}
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestRejectsWrongJSONTypes(t *testing.T) {
	for _, input := range []string{
		strings.Replace(validFlagMigrationManifestJSON, `"required": true`, `"required": "true"`, 1),
		strings.Replace(validFlagMigrationManifestJSON, `"scope": "local"`, `"scope": false`, 1),
		`{"version": "1", "migrations": []}`,
		`{"version": 1, "migrations": {}}`,
	} {
		if _, err := ReadFlagMigrationManifest(strings.NewReader(input)); err == nil {
			t.Fatal("ReadFlagMigrationManifest() accepted a field with the wrong JSON type")
		}
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestReportsReaderFailure(t *testing.T) {
	_, err := ReadFlagMigrationManifest(flagMigrationErrorReader{})
	if err == nil || !strings.Contains(err.Error(), "injected read failure") {
		t.Fatalf("ReadFlagMigrationManifest() error = %v, want reader failure", err)
	}
}

func TestCrossPlatformCoverageReadFlagMigrationManifestValidatesExactEntries(t *testing.T) {
	if _, err := ReadFlagMigrationManifest(strings.NewReader(validFlagMigrationManifestJSON)); err != nil {
		t.Fatalf("ReadFlagMigrationManifest(valid) error = %v", err)
	}
	if _, err := ReadFlagMigrationManifest(strings.NewReader(optionalFlagMigrationManifestJSON())); err != nil {
		t.Fatalf("ReadFlagMigrationManifest(optional rename) error = %v", err)
	}
	if _, err := ReadFlagMigrationManifest(strings.NewReader(hiddenCanonicalFlagMigrationManifestJSON())); err != nil {
		t.Fatalf("ReadFlagMigrationManifest(hidden canonical promotion) error = %v", err)
	}
	if _, err := ReadFlagMigrationManifest(strings.NewReader(validRequirednessMigrationManifestJSON)); err != nil {
		t.Fatalf("ReadFlagMigrationManifest(requiredness change) error = %v", err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "unsupported version",
			input:   strings.Replace(validFlagMigrationManifestJSON, `"version": 1`, `"version": 2`, 1),
			wantErr: "unsupported flag migration manifest version",
		},
		{
			name:    "missing migrations array",
			input:   `{"version": 1}`,
			wantErr: "migrations must be an array",
		},
		{
			name:    "null migrations array",
			input:   `{"version": 1, "migrations": null}`,
			wantErr: "migrations must be an array",
		},
		{
			name:    "wildcard command",
			input:   strings.Replace(validFlagMigrationManifestJSON, "dws chat message recall", "dws chat *", 1),
			wantErr: "exact command path",
		},
		{
			name:    "wildcard flag",
			input:   strings.Replace(validFlagMigrationManifestJSON, `"name": "msg-id"`, `"name": "msg-*"`, 1),
			wantErr: "exact legacy flag",
		},
		{
			name:    "empty reason",
			input:   strings.Replace(validFlagMigrationManifestJSON, "Reviewed message identifier flag migration.", "", 1),
			wantErr: "non-empty reason",
		},
		{
			name:    "invalid state",
			input:   strings.Replace(validFlagMigrationManifestJSON, `"state": "pending"`, `"state": "approved"`, 1),
			wantErr: "invalid state",
		},
		{
			name:    "legacy removed",
			input:   strings.Replace(validFlagMigrationManifestJSON, `"after": {"present": true, "type": "string", "hidden": true, "scope": "local", "alias_of": "message-id"}`, `"after": {"present": false}`, 1),
			wantErr: "legacy flag must remain present",
		},
		{
			name:    "canonical hidden",
			input:   strings.Replace(validFlagMigrationManifestJSON, `"after": {"present": true, "type": "string", "required": true, "scope": "local"}`, `"after": {"present": true, "type": "string", "required": true, "hidden": true, "scope": "local"}`, 1),
			wantErr: "canonical flag must remain visible",
		},
		{
			name: "required legacy becomes optional canonical",
			input: strings.Replace(
				validFlagMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "scope": "local"}`,
				1,
			),
			wantErr: "requiredness must be preserved from legacy before to canonical after",
		},
		{
			name: "optional legacy becomes required canonical",
			input: strings.Replace(
				optionalFlagMigrationManifestJSON(),
				`"after": {"present": true, "type": "string", "scope": "local"}`,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				1,
			),
			wantErr: "requiredness must be preserved from legacy before to canonical after",
		},
		{
			name: "existing canonical changes requiredness",
			input: strings.Replace(
				validFlagMigrationManifestJSON,
				`"before": {"present": false}`,
				`"before": {"present": true, "type": "string", "scope": "local"}`,
				1,
			),
			wantErr: "canonical flag requiredness must remain unchanged when already present",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadFlagMigrationManifest(strings.NewReader(test.input))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ReadFlagMigrationManifest() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageRequirednessMigrationRejectsInexactContracts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "unknown kind",
			input:   strings.Replace(validRequirednessMigrationManifestJSON, `"requiredness_change"`, `"anything"`, 1),
			wantErr: "invalid kind",
		},
		{
			name:    "missing flag name",
			input:   strings.Replace(validRequirednessMigrationManifestJSON, `"to-user-ids"`, `""`, 1),
			wantErr: "exact flag",
		},
		{
			name: "invalid before state",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"before": {"present": true, "type": "string", "scope": "local"}`,
				`"before": {"present": true, "type": "string", "scope": "anything"}`,
				1,
			),
			wantErr: "flag before present state has invalid scope",
		},
		{
			name: "invalid after state",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "required": true, "scope": "anything"}`,
				1,
			),
			wantErr: "flag after present state has invalid scope",
		},
		{
			name: "before already required",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"before": {"present": true, "type": "string", "scope": "local"}`,
				`"before": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				1,
			),
			wantErr: "optional to required",
		},
		{
			name: "after remains optional",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "scope": "local"}`,
				1,
			),
			wantErr: "optional to required",
		},
		{
			name: "type drift",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "stringSlice", "required": true, "scope": "local"}`,
				1,
			),
			wantErr: "must preserve every flag attribute except requiredness",
		},
		{
			name: "visibility drift",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "required": true, "hidden": true, "scope": "local"}`,
				1,
			),
			wantErr: "must stay visible",
		},
		{
			name: "alias relation",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local", "alias_of": "other"}`,
				1,
			),
			wantErr: "must not declare alias_of",
		},
		{
			name: "rename fields cannot be combined",
			input: strings.Replace(
				validRequirednessMigrationManifestJSON,
				`"flag": {`,
				`"legacy": {"name":"old","before":{"present":false},"after":{"present":false}}, "flag": {`,
				1,
			),
			wantErr: "must not declare legacy or canonical",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadFlagMigrationManifest(strings.NewReader(test.input))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ReadFlagMigrationManifest() error = %v, want %q", err, test.wantErr)
			}
		})
	}

	missingFlag := FlagMigration{
		Kind:    FlagMigrationRequirednessChange,
		Command: "dws report entry submit",
		State:   FlagMigrationPending,
		Reason:  "Missing exact flag state.",
	}
	if err := missingFlag.validate(); err == nil || !strings.Contains(err.Error(), "exact flag") {
		t.Fatalf("missing requiredness flag error = %v", err)
	}

	rename, err := ReadFlagMigrationManifest(strings.NewReader(validFlagMigrationManifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	rename.Migrations[0].Kind = FlagMigrationRename
	rename.Migrations[0].Flag = &FlagMigrationSide{
		Name:   "unrelated",
		Before: FlagMigrationState{Present: true, Type: "string", Scope: "local"},
		After:  FlagMigrationState{Present: true, Type: "string", Required: true, Scope: "local"},
	}
	if err := rename.Validate(); err == nil || !strings.Contains(err.Error(), "must not declare flag requiredness fields") {
		t.Fatalf("rename with requiredness fields error = %v", err)
	}
}

func TestCrossPlatformCoverageRequirednessMigrationLifecycleAndAuthorization(t *testing.T) {
	pending, err := ReadFlagMigrationManifest(strings.NewReader(validRequirednessMigrationManifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	consumed := pending
	consumed.Migrations = append([]FlagMigration(nil), pending.Migrations...)
	consumed.Migrations[0].State = FlagMigrationConsumed
	migration := pending.Migrations[0]
	before := requirednessMigrationSnapshot(migration, false)
	after := requirednessMigrationSnapshot(migration, true)

	ordinary := Compare(after, before, "merge-base")
	if !hasFlagChange(ordinary.Blocking, "flag_became_required", migration.Command, migration.Flag.Name) {
		t.Fatalf("fixture did not produce flag_became_required: %#v", ordinary.Blocking)
	}
	report, err := CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		pending,
		consumed,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("governed requiredness change = (%#v, %v), want compatible", report, err)
	}
	partial := after
	partial.Commands = append([]Command(nil), after.Commands...)
	partial.Commands[len(partial.Commands)-1].LocalFlags = append(
		[]Flag(nil),
		after.Commands[len(after.Commands)-1].LocalFlags...,
	)
	partial.Commands[len(partial.Commands)-1].LocalFlags[0].Type = "stringSlice"
	if phase := matchFlagMigrationPhase(partial, migration); phase != flagMigrationPartial {
		t.Fatalf("drifted requiredness migration phase = %s, want partial", phase)
	}

	beforeWithUnrelated := before
	beforeWithUnrelated.Commands = append([]Command(nil), before.Commands...)
	beforeWithUnrelated.Commands[len(beforeWithUnrelated.Commands)-1].LocalFlags = append(
		beforeWithUnrelated.Commands[len(beforeWithUnrelated.Commands)-1].LocalFlags,
		Flag{Name: "unrelated", Type: "string"},
	)
	afterWithUnrelated := after
	afterWithUnrelated.Commands = append([]Command(nil), after.Commands...)
	afterWithUnrelated.Commands[len(afterWithUnrelated.Commands)-1].LocalFlags = append(
		afterWithUnrelated.Commands[len(afterWithUnrelated.Commands)-1].LocalFlags,
		Flag{Name: "unrelated", Type: "string", Required: true},
	)
	report, err = CompareAllWithFlagMigrations(
		afterWithUnrelated,
		map[string]Snapshot{"merge-base": beforeWithUnrelated, "stable": beforeWithUnrelated},
		pending,
		consumed,
	)
	if err != nil || report.Compatible || !hasFlagChange(report.Comparisons[0].Blocking, "flag_became_required", migration.Command, "unrelated") {
		t.Fatalf("unrelated requiredness change was hidden: report=%#v err=%v", report, err)
	}

	empty := FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}}
	if _, err := CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		empty,
		pending,
	); err == nil || !strings.Contains(err.Error(), "cannot authorize its own interface change") {
		t.Fatalf("candidate self-authorization error = %v", err)
	}
}

func requirednessMigrationSnapshot(migration FlagMigration, after bool) Snapshot {
	state := migration.Flag.Before
	if after {
		state = migration.Flag.After
	}
	flag := Flag{
		Name:      migration.Flag.Name,
		Type:      state.Type,
		Required:  state.Required,
		Hidden:    state.Hidden,
		Shorthand: state.Shorthand,
		NoOpt:     state.NoOpt,
	}
	command := testCommandWithFlagScopes(migration.Command, nil, nil)
	if state.Scope == "inherited" {
		command.InheritedFlags = []Flag{flag}
	} else {
		command.LocalFlags = []Flag{flag}
	}
	return testSnapshot(testCommand("dws"), command)
}

func TestCrossPlatformCoverageFlagMigrationManifestRejectsDuplicateAndInexactContracts(t *testing.T) {
	manifest, err := ReadFlagMigrationManifest(strings.NewReader(validFlagMigrationManifestJSON))
	if err != nil {
		t.Fatal(err)
	}

	duplicate := manifest
	duplicate.Migrations = append(duplicate.Migrations, duplicate.Migrations[0])
	if err := duplicate.Validate(); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate migration error = %v", err)
	}

	whitespaceReason := manifest
	whitespaceReason.Migrations = append([]FlagMigration(nil), manifest.Migrations...)
	whitespaceReason.Migrations[0].Reason = "  reviewed  "
	if err := whitespaceReason.Validate(); err == nil || !strings.Contains(err.Error(), "trimmed") {
		t.Fatalf("whitespace reason error = %v", err)
	}

	canonicalDrift := manifest
	canonicalDrift.Migrations = append([]FlagMigration(nil), manifest.Migrations...)
	canonicalDrift.Migrations[0].Canonical.Before = FlagMigrationState{
		Present:  true,
		Type:     "string",
		Required: true,
		Scope:    "local",
	}
	canonicalDrift.Migrations[0].Canonical.After.Type = "stringSlice"
	if err := canonicalDrift.Validate(); err == nil || !strings.Contains(err.Error(), "canonical flag type") {
		t.Fatalf("canonical type drift error = %v", err)
	}

	_, err = ReadFlagMigrationManifest(strings.NewReader(validFlagMigrationManifestJSON + ` {}`))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON error = %v", err)
	}

	requiredness, err := ReadFlagMigrationManifest(strings.NewReader(validRequirednessMigrationManifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	overlap := manifest
	overlap.Migrations = append([]FlagMigration(nil), manifest.Migrations...)
	rename := overlap.Migrations[0]
	rename.Command = requiredness.Migrations[0].Command
	rename.Canonical.Name = requiredness.Migrations[0].Flag.Name
	rename.Legacy.After.AliasOf = rename.Canonical.Name
	overlap.Migrations[0] = rename
	overlap.Migrations = append(overlap.Migrations, requiredness.Migrations[0])
	if err := overlap.Validate(); err == nil || !strings.Contains(err.Error(), "overlaps rename migration") {
		t.Fatalf("requiredness and rename overlap error = %v", err)
	}
}

func TestCrossPlatformCoverageAuthorizeFlagMigrationsReturnsExactBaseOwnedApproval(t *testing.T) {
	pending := coverageManifest(FlagMigrationPending)
	consumed := coverageManifest(FlagMigrationConsumed)
	migration := pending.Migrations[0]
	before := coverageMigrationSnapshot(migration, false, false)
	after := coverageMigrationSnapshot(migration, true, false)

	got, err := AuthorizeFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("AuthorizeFlagMigrations() error = %v", err)
	}
	if !reflect.DeepEqual(got, pending.Migrations) {
		t.Fatalf("AuthorizeFlagMigrations() = %#v, want exact base-owned approval %#v", got, pending.Migrations)
	}
}
