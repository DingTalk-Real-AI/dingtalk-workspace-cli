// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageFlagMigrationStrictJSONSchemaErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "schema validator rejects a trailing value",
			input:   coverageManifestJSON() + ` {}`,
			wantErr: "trailing JSON value",
		},
		{
			name:    "schema validator reports malformed trailing data",
			input:   coverageManifestJSON() + ` ?`,
			wantErr: "read trailing flag migration manifest data",
		},
		{
			name:    "schema validator reports an empty document",
			input:   "",
			wantErr: "read flag migration manifest value at $",
		},
		{
			name:    "schema validator rejects an unknown field",
			input:   `{"version":1,"migrations":[],"unexpected":true}`,
			wantErr: `unknown field "unexpected"`,
		},
		{
			name:    "schema validator reports a malformed field token",
			input:   `{"`,
			wantErr: "flag migration manifest",
		},
		{
			name:    "schema validator reports an unclosed object",
			input:   `{"version":1`,
			wantErr: "close flag migration manifest object",
		},
		{
			name:    "schema validator reports an unclosed array",
			input:   `{"version":1,"migrations":[`,
			wantErr: "close flag migration manifest array",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFlagMigrationJSONSchema([]byte(test.input))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateFlagMigrationJSONSchema() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageFlagMigrationStrictJSONTypeDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		schema  reflect.Type
		wantErr string
	}{
		{
			name:    "manifest must be an object",
			input:   `[]`,
			schema:  reflect.TypeOf(FlagMigrationManifest{}),
			wantErr: "must be an object, got array",
		},
		{
			name:    "migrations must be an array",
			input:   `{}`,
			schema:  reflect.TypeOf([]FlagMigration{}),
			wantErr: "must be an array, got object",
		},
		{
			name:    "integer rejects string",
			input:   `"one"`,
			schema:  reflect.TypeOf(int(0)),
			wantErr: "must be a number, got string",
		},
		{
			name:    "string rejects number",
			input:   `1`,
			schema:  reflect.TypeOf(""),
			wantErr: "must be a string, got number",
		},
		{
			name:    "string rejects boolean",
			input:   `false`,
			schema:  reflect.TypeOf(""),
			wantErr: "must be a string, got boolean",
		},
		{
			name:    "unsupported schema fails closed",
			input:   `1`,
			schema:  reflect.TypeOf(float64(0)),
			wantErr: "unsupported Go schema type float64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(test.input))
			decoder.UseNumber()
			err := validateMigrationJSONValue(decoder, "$", test.schema)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateMigrationJSONValue() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageFlagMigrationStrictJSONSchemaHelpers(t *testing.T) {
	type fieldFixture struct {
		DefaultName string
		Ignored     string `json:"-"`
		Tagged      string `json:"tagged,omitempty"`
	}
	fields := migrationJSONFields(reflect.TypeOf(fieldFixture{}))
	if _, exists := fields["Ignored"]; exists {
		t.Fatal("migrationJSONFields() retained a json:- field")
	}
	if fields["DefaultName"] != reflect.TypeOf("") || fields["tagged"] != reflect.TypeOf("") {
		t.Fatalf("migrationJSONFields() = %#v, want default and tagged string fields", fields)
	}

	if got := migrationJSONKindDescription(reflect.TypeOf(float64(0))); got != "the declared JSON type" {
		t.Fatalf("migrationJSONKindDescription(float64) = %q", got)
	}
	for _, test := range []struct {
		name  string
		token json.Token
		want  string
	}{
		{name: "closing delimiter", token: json.Delim('}'), want: `delimiter "}"`},
		{name: "unsupported token", token: struct{}{}, want: "struct {}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := migrationJSONTokenDescription(test.token); got != test.want {
				t.Fatalf("migrationJSONTokenDescription() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageFlagMigrationManifestParserEdges(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "initial malformed JSON",
			input:   `{`,
			wantErr: "unexpected EOF",
		},
		{
			name:    "multiple JSON values",
			input:   coverageManifestJSON() + ` {}`,
			wantErr: "trailing multiple JSON values",
		},
		{
			name:    "malformed trailing JSON",
			input:   coverageManifestJSON() + ` {`,
			wantErr: "read trailing flag migration manifest data",
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

func TestCrossPlatformCoverageFlagMigrationManifestRejectsEveryContractDrift(t *testing.T) {
	optionalCanonical := func() FlagMigrationManifest {
		manifest := coverageManifest(FlagMigrationPending)
		manifest.Migrations[0].Canonical.Before = FlagMigrationState{
			Present: true,
			Type:    "string",
			Scope:   "local",
		}
		return manifest
	}

	tests := []struct {
		name    string
		make    func() FlagMigrationManifest
		mutate  func(*FlagMigrationManifest)
		wantErr string
	}{
		{
			name:    "canonical flag name is exact",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Name = "--message-id" },
			wantErr: "canonical name must be an exact canonical flag",
		},
		{
			name:    "legacy and canonical names differ",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Name = m.Migrations[0].Legacy.Name },
			wantErr: "legacy and canonical flags must differ",
		},
		{
			name:    "legacy before state validates",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.Before.Scope = "global" },
			wantErr: "legacy before present state has invalid scope",
		},
		{
			name:    "legacy after state validates",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.Type = "" },
			wantErr: "legacy after present state requires type",
		},
		{
			name:    "canonical before state validates",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.Type = "" },
			wantErr: "canonical before present state requires type",
		},
		{
			name:    "canonical after state validates",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.After.Scope = "" },
			wantErr: "canonical after present state has invalid scope",
		},
		{
			name:    "legacy before remains present",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.Before = FlagMigrationState{} },
			wantErr: "legacy flag must remain present before and after migration",
		},
		{
			name:    "legacy after remains present",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After = FlagMigrationState{} },
			wantErr: "legacy flag must remain present before and after migration",
		},
		{
			name:    "legacy starts visible",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.Before.Hidden = true },
			wantErr: "legacy flag must migrate exactly from visible to hidden",
		},
		{
			name:    "legacy ends hidden",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.Hidden = false },
			wantErr: "legacy flag must migrate exactly from visible to hidden",
		},
		{
			name:    "canonical after remains present",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.After = FlagMigrationState{} },
			wantErr: "canonical flag must be present after migration",
		},
		{
			name:    "legacy after declares alias target",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.AliasOf = "other-id" },
			wantErr: `legacy flag after state must declare alias_of "message-id"`,
		},
		{
			name:    "legacy before cannot declare alias",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.Before.AliasOf = "message-id" },
			wantErr: "alias_of is only valid on the legacy after state",
		},
		{
			name:    "canonical before cannot declare alias",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.AliasOf = "message-id" },
			wantErr: "alias_of is only valid on the legacy after state",
		},
		{
			name:    "canonical after cannot declare alias",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.After.AliasOf = "message-id" },
			wantErr: "alias_of is only valid on the legacy after state",
		},
		{
			name:    "canonical type remains stable",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.Type = "stringSlice" },
			wantErr: "canonical flag type must remain unchanged",
		},
		{
			name:    "canonical shorthand remains stable",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.Shorthand = "m" },
			wantErr: "canonical flag shorthand must remain unchanged",
		},
		{
			name:    "canonical no-opt remains stable",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.NoOpt = "auto" },
			wantErr: "canonical flag no_opt must remain unchanged",
		},
		{
			name:    "canonical scope remains stable",
			make:    optionalCanonical,
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.Before.Scope = "inherited" },
			wantErr: "canonical flag scope must remain unchanged",
		},
		{
			name:    "legacy before type matches legacy after",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.Before.Type = "stringSlice" },
			wantErr: "legacy and canonical flag types must match exactly",
		},
		{
			name:    "legacy after type matches canonical after",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Canonical.After.Type = "stringSlice" },
			wantErr: "legacy and canonical flag types must match exactly",
		},
		{
			name:    "legacy shorthand remains stable",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.Shorthand = "x" },
			wantErr: "legacy flag shorthand must remain unchanged",
		},
		{
			name:    "legacy no-opt remains stable",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.NoOpt = "" },
			wantErr: "legacy flag no_opt must remain unchanged",
		},
		{
			name:    "legacy scope remains stable",
			make:    func() FlagMigrationManifest { return coverageManifest(FlagMigrationPending) },
			mutate:  func(m *FlagMigrationManifest) { m.Migrations[0].Legacy.After.Scope = "inherited" },
			wantErr: "legacy flag scope must remain unchanged",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := test.make()
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageFlagMigrationManifestRejectsAbsentAttributesAndTargetConflicts(t *testing.T) {
	stateMutations := []struct {
		name   string
		mutate func(*FlagMigrationState)
	}{
		{name: "type", mutate: func(s *FlagMigrationState) { s.Type = "string" }},
		{name: "required", mutate: func(s *FlagMigrationState) { s.Required = true }},
		{name: "hidden", mutate: func(s *FlagMigrationState) { s.Hidden = true }},
		{name: "shorthand", mutate: func(s *FlagMigrationState) { s.Shorthand = "m" }},
		{name: "no-opt", mutate: func(s *FlagMigrationState) { s.NoOpt = "auto" }},
		{name: "scope", mutate: func(s *FlagMigrationState) { s.Scope = "local" }},
		{name: "alias", mutate: func(s *FlagMigrationState) { s.AliasOf = "message-id" }},
	}
	for _, test := range stateMutations {
		t.Run("absent state rejects "+test.name, func(t *testing.T) {
			manifest := coverageManifest(FlagMigrationPending)
			test.mutate(&manifest.Migrations[0].Canonical.Before)
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "absent state must not declare flag attributes") {
				t.Fatalf("Validate() error = %v, want absent-state rejection", err)
			}
		})
	}

	manifest := coverageManifest(FlagMigrationPending)
	second := coverageManifest(FlagMigrationPending).Migrations[0]
	second.Canonical.Name = "canonical-two"
	second.Legacy.After.AliasOf = second.Canonical.Name
	manifest.Migrations = append(manifest.Migrations, second)
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "to both --message-id and --canonical-two") {
		t.Fatalf("Validate() error = %v, want one legacy to two canonical targets rejected", err)
	}
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsValidatesEveryInput(t *testing.T) {
	before := coverageMigrationSnapshot(coverageManifest(FlagMigrationPending).Migrations[0], false, false)
	empty := coverageEmptyManifest()

	tests := []struct {
		name       string
		current    Snapshot
		references map[string]Snapshot
		authority  FlagMigrationManifest
		candidate  FlagMigrationManifest
		wantErr    string
	}{
		{
			name:       "current snapshot",
			current:    Snapshot{},
			references: map[string]Snapshot{"merge-base": before},
			authority:  empty,
			candidate:  empty,
			wantErr:    "validate current interface snapshot",
		},
		{
			name:       "reference snapshot",
			current:    before,
			references: map[string]Snapshot{"merge-base": {}},
			authority:  empty,
			candidate:  empty,
			wantErr:    "validate merge-base interface snapshot",
		},
		{
			name:       "authority manifest",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  FlagMigrationManifest{},
			candidate:  empty,
			wantErr:    "validate approved flag migrations",
		},
		{
			name:       "candidate manifest",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  empty,
			candidate:  FlagMigrationManifest{},
			wantErr:    "validate candidate flag migrations",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompareAllWithFlagMigrations(test.current, test.references, test.authority, test.candidate)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("CompareAllWithFlagMigrations() error = %v, want %q", err, test.wantErr)
			}
		})
	}

	report, err := CompareAllWithFlagMigrations(before, map[string]Snapshot{"merge-base": before}, empty, empty)
	if err != nil || !report.Compatible {
		t.Fatalf("empty migration lifecycle = (%#v, %v), want compatible", report, err)
	}
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsLifecycleErrors(t *testing.T) {
	pending := coverageManifest(FlagMigrationPending)
	consumed := coverageManifest(FlagMigrationConsumed)
	empty := coverageEmptyManifest()
	migration := pending.Migrations[0]
	before := coverageMigrationSnapshot(migration, false, false)
	after := coverageMigrationSnapshot(migration, true, false)
	partial := coverageMigrationSnapshot(migration, false, false)
	partial.Commands[len(partial.Commands)-1].LocalFlags[0].Hidden = true

	modified := coverageManifest(FlagMigrationPending)
	modified.Migrations[0].Reason = "A different reviewed reason."

	tests := []struct {
		name       string
		current    Snapshot
		references map[string]Snapshot
		authority  FlagMigrationManifest
		candidate  FlagMigrationManifest
		wantErr    string
	}{
		{
			name:       "requires merge-base or main",
			current:    before,
			references: map[string]Snapshot{"stable": before},
			authority:  pending,
			candidate:  pending,
			wantErr:    "requires a main or merge-base reference",
		},
		{
			name:       "pending authority requires before base",
			current:    after,
			references: map[string]Snapshot{"merge-base": after},
			authority:  pending,
			candidate:  pending,
			wantErr:    "want exact before state for pending",
		},
		{
			name:       "consumed authority requires after base",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  consumed,
			candidate:  consumed,
			wantErr:    "want exact after state for consumed",
		},
		{
			name:       "missing base command is partial",
			current:    before,
			references: map[string]Snapshot{"merge-base": coverageRootOnlySnapshot()},
			authority:  pending,
			candidate:  pending,
			wantErr:    "is partial in merge-base",
		},
		{
			name:       "candidate cannot modify base receipt",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  modified,
			wantErr:    "candidate modified base-owned flag migration",
		},
		{
			name:       "candidate cannot remove pending receipt",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  empty,
			wantErr:    "candidate removed pending flag migration",
		},
		{
			name:       "candidate cannot falsely consume pending receipt",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  consumed,
			wantErr:    "candidate falsely consumed unchanged flag migration",
		},
		{
			name:       "candidate must consume completed receipt",
			current:    after,
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  pending,
			wantErr:    "without marking it consumed",
		},
		{
			name:       "candidate rejects partial application",
			current:    partial,
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  consumed,
			wantErr:    "candidate partially applied flag migration",
		},
		{
			name:       "consumed receipt rejects current revert",
			current:    before,
			references: map[string]Snapshot{"merge-base": after, "stable": before},
			authority:  consumed,
			candidate:  consumed,
			wantErr:    "candidate drifted from consumed flag migration",
		},
		{
			name:       "consumed receipt cannot be removed early",
			current:    after,
			references: map[string]Snapshot{"merge-base": after, "stable": before},
			authority:  consumed,
			candidate:  empty,
			wantErr:    "before every reference reached the after state",
		},
		{
			name:       "consumed receipt cannot revert to pending",
			current:    after,
			references: map[string]Snapshot{"merge-base": after, "stable": before},
			authority:  consumed,
			candidate:  pending,
			wantErr:    "back to pending",
		},
		{
			name:       "consumed receipt becomes stale",
			current:    after,
			references: map[string]Snapshot{"merge-base": after, "stable": after},
			authority:  consumed,
			candidate:  consumed,
			wantErr:    "is stale after all references reached the after state",
		},
		{
			name:       "candidate-added receipt starts pending",
			current:    before,
			references: map[string]Snapshot{"merge-base": before},
			authority:  empty,
			candidate:  consumed,
			wantErr:    "must start pending",
		},
		{
			name:       "candidate-added receipt matches base before",
			current:    before,
			references: map[string]Snapshot{"merge-base": after},
			authority:  empty,
			candidate:  pending,
			wantErr:    "does not match the merge-base before state",
		},
		{
			name:       "candidate-added receipt cannot self-authorize",
			current:    after,
			references: map[string]Snapshot{"merge-base": before},
			authority:  empty,
			candidate:  pending,
			wantErr:    "cannot authorize its own interface change",
		},
		{
			name:       "missing current command is partial",
			current:    coverageRootOnlySnapshot(),
			references: map[string]Snapshot{"merge-base": before},
			authority:  pending,
			candidate:  pending,
			wantErr:    "candidate partially applied flag migration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompareAllWithFlagMigrations(test.current, test.references, test.authority, test.candidate)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("CompareAllWithFlagMigrations() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsMainAuthorityAndCleanup(t *testing.T) {
	pending := coverageManifest(FlagMigrationPending)
	consumed := coverageManifest(FlagMigrationConsumed)
	empty := coverageEmptyManifest()
	migration := pending.Migrations[0]
	before := coverageMigrationSnapshot(migration, false, false)
	after := coverageMigrationSnapshot(migration, true, false)

	report, err := CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"main": before},
		pending,
		consumed,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("main-owned migration = (%#v, %v), want compatible", report, err)
	}

	report, err = CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "main": after},
		pending,
		consumed,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("merge-base precedence = (%#v, %v), want compatible", report, err)
	}

	report, err = CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": after, "stable": after},
		consumed,
		empty,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("consumed receipt cleanup = (%#v, %v), want compatible", report, err)
	}

	report, err = CompareAllWithFlagMigrations(
		before,
		map[string]Snapshot{"merge-base": before},
		empty,
		pending,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("pending receipt creation = (%#v, %v), want compatible", report, err)
	}
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsInheritedAndOptionalCanonical(t *testing.T) {
	t.Run("inherited exact state", func(t *testing.T) {
		pending := coverageManifest(FlagMigrationPending)
		pending.Migrations[0].Legacy.Before.Scope = "inherited"
		pending.Migrations[0].Legacy.After.Scope = "inherited"
		pending.Migrations[0].Canonical.After.Scope = "inherited"
		consumed := pending
		consumed.Migrations = append([]FlagMigration(nil), pending.Migrations...)
		consumed.Migrations[0].State = FlagMigrationConsumed
		before := coverageMigrationSnapshot(pending.Migrations[0], false, false)
		after := coverageMigrationSnapshot(pending.Migrations[0], true, false)

		report, err := CompareAllWithFlagMigrations(
			after,
			map[string]Snapshot{"merge-base": before, "stable": before},
			pending,
			consumed,
		)
		if err != nil || !report.Compatible {
			t.Fatalf("inherited migration = (%#v, %v), want compatible", report, err)
		}
	})

	t.Run("existing optional canonical becomes required", func(t *testing.T) {
		pending := coverageManifest(FlagMigrationPending)
		pending.Migrations[0].Canonical.Before = FlagMigrationState{
			Present: true,
			Type:    "string",
			Scope:   "local",
		}
		consumed := pending
		consumed.Migrations = append([]FlagMigration(nil), pending.Migrations...)
		consumed.Migrations[0].State = FlagMigrationConsumed
		before := coverageMigrationSnapshot(pending.Migrations[0], false, false)
		after := coverageMigrationSnapshot(pending.Migrations[0], true, false)

		ordinary := Compare(after, before, "merge-base")
		if !hasFlagChange(ordinary.Blocking, "flag_became_required", pending.Migrations[0].Command, pending.Migrations[0].Canonical.Name) {
			t.Fatalf("fixture did not create flag_became_required: %#v", ordinary.Blocking)
		}
		report, err := CompareAllWithFlagMigrations(
			after,
			map[string]Snapshot{"merge-base": before},
			pending,
			consumed,
		)
		if err != nil || !report.Compatible {
			t.Fatalf("optional-to-required migration = (%#v, %v), want compatible", report, err)
		}
	})
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsNormalizesAliasesAndKeepsOtherBreakage(t *testing.T) {
	pending := coverageManifest(FlagMigrationPending)
	consumed := coverageManifest(FlagMigrationConsumed)
	migration := pending.Migrations[0]
	before := coverageMigrationSnapshot(migration, false, true)
	after := coverageMigrationSnapshot(migration, true, true)

	ordinary := Compare(after, before, "merge-base")
	if !hasFlagChange(ordinary.Blocking, "flag_became_hidden", "dws chat deliver", migration.Legacy.Name) {
		t.Fatalf("alias fixture did not create alias-path finding: %#v", ordinary.Blocking)
	}
	report, err := CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before},
		pending,
		consumed,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("alias-path migration = (%#v, %v), want compatible", report, err)
	}

	beforeWithOther := before
	afterWithOther := after
	beforeWithOther.Commands = append(beforeWithOther.Commands, coverageCommand("dws files", nil, []Flag{{Name: "format", Type: "string"}}, nil))
	afterWithOther.Commands = append(afterWithOther.Commands, coverageCommand("dws files", nil, nil, nil))
	report, err = CompareAllWithFlagMigrations(
		afterWithOther,
		map[string]Snapshot{"merge-base": beforeWithOther},
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("unrelated breakage returned lifecycle error: %v", err)
	}
	if report.Compatible || !hasFlagChange(report.Comparisons[0].Blocking, "flag_removed", "dws files", "format") {
		t.Fatalf("unrelated breakage was hidden: %#v", report)
	}
}

func TestCrossPlatformCoverageCompareAllWithFlagMigrationsDoesNotAuthorizePartialReferenceOrRuleDrift(t *testing.T) {
	pending := coverageManifest(FlagMigrationPending)
	consumed := coverageManifest(FlagMigrationConsumed)
	migration := pending.Migrations[0]
	before := coverageMigrationSnapshot(migration, false, false)
	after := coverageMigrationSnapshot(migration, true, false)

	partialReference := coverageMigrationSnapshot(migration, false, false)
	partialReference.Commands[len(partialReference.Commands)-1].LocalFlags[0].Hidden = true
	report, err := CompareAllWithFlagMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": partialReference},
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("partial non-authority reference returned lifecycle error: %v", err)
	}
	if report.Compatible {
		t.Fatalf("partial reference was incorrectly authorized: %#v", report)
	}

	beforeWithoutRoot := coverageMigrationSnapshot(migration, false, false)
	afterWithoutRoot := coverageMigrationSnapshot(migration, true, false)
	beforeWithoutRoot.Commands = beforeWithoutRoot.Commands[len(beforeWithoutRoot.Commands)-1:]
	afterWithoutRoot.Commands = afterWithoutRoot.Commands[len(afterWithoutRoot.Commands)-1:]
	afterWithoutRoot.Rules.ExcludedFlags = []string{"help", "version"}
	report, err = CompareAllWithFlagMigrations(
		afterWithoutRoot,
		map[string]Snapshot{"merge-base": beforeWithoutRoot},
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("rule drift returned lifecycle error: %v", err)
	}
	if report.Compatible || !hasChangeKind(report.Comparisons[0].Blocking, "snapshot_rules_changed") {
		t.Fatalf("snapshot rule drift was incorrectly authorized: %#v", report)
	}
}

func coverageManifestJSON() string {
	return `{
  "version": 1,
  "migrations": [
    {
      "command": "dws chat send",
      "legacy": {
        "name": "legacy-id",
        "before": {"present": true, "type": "string", "required": true, "shorthand": "l", "no_opt": "auto", "scope": "local"},
        "after": {"present": true, "type": "string", "hidden": true, "shorthand": "l", "no_opt": "auto", "scope": "local", "alias_of": "message-id"}
      },
      "canonical": {
        "name": "message-id",
        "before": {"present": false},
        "after": {"present": true, "type": "string", "required": true, "scope": "local"}
      },
      "state": "pending",
      "reason": "Reviewed exact flag migration."
    }
  ]
}`
}

func coverageManifest(state string) FlagMigrationManifest {
	return FlagMigrationManifest{
		Version: FlagMigrationManifestVersion,
		Migrations: []FlagMigration{
			{
				Command: "dws chat send",
				Legacy: FlagMigrationSide{
					Name: "legacy-id",
					Before: FlagMigrationState{
						Present:   true,
						Type:      "string",
						Required:  true,
						Shorthand: "l",
						NoOpt:     "auto",
						Scope:     "local",
					},
					After: FlagMigrationState{
						Present:   true,
						Type:      "string",
						Hidden:    true,
						Shorthand: "l",
						NoOpt:     "auto",
						Scope:     "local",
						AliasOf:   "message-id",
					},
				},
				Canonical: FlagMigrationSide{
					Name:   "message-id",
					Before: FlagMigrationState{},
					After: FlagMigrationState{
						Present:  true,
						Type:     "string",
						Required: true,
						Scope:    "local",
					},
				},
				State:  state,
				Reason: "Reviewed exact flag migration.",
			},
		},
	}
}

func coverageEmptyManifest() FlagMigrationManifest {
	return FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}}
}

func coverageMigrationSnapshot(migration FlagMigration, after, commandAlias bool) Snapshot {
	legacyState := migration.Legacy.Before
	canonicalState := migration.Canonical.Before
	if after {
		legacyState = migration.Legacy.After
		canonicalState = migration.Canonical.After
	}
	local := []Flag{}
	inherited := []Flag{}
	add := func(name string, state FlagMigrationState) {
		if !state.Present {
			return
		}
		flag := Flag{
			Name:      name,
			Shorthand: state.Shorthand,
			Type:      state.Type,
			NoOpt:     state.NoOpt,
			Required:  state.Required,
			Hidden:    state.Hidden,
			AliasOf:   state.AliasOf,
		}
		if state.Scope == "inherited" {
			inherited = append(inherited, flag)
			return
		}
		local = append(local, flag)
	}
	add(migration.Legacy.Name, legacyState)
	add(migration.Canonical.Name, canonicalState)
	aliases := []string{}
	if commandAlias {
		aliases = []string{"deliver"}
	}
	return coverageSnapshot(
		coverageCommand("dws", nil, nil, nil),
		coverageCommand("dws chat", nil, nil, nil),
		coverageCommand(migration.Command, aliases, local, inherited),
	)
}

func coverageRootOnlySnapshot() Snapshot {
	return coverageSnapshot(coverageCommand("dws", nil, nil, nil))
}

func coverageSnapshot(commands ...Command) Snapshot {
	return Snapshot{
		SchemaVersion: SchemaVersion,
		Rules: Rules{
			ExcludedCommandSubtrees: append([]string(nil), excludedCommandSubtrees...),
			ExcludedFlags:           []string{"help"},
		},
		Commands: commands,
	}
}

func coverageCommand(path string, aliases []string, local, inherited []Flag) Command {
	if aliases == nil {
		aliases = []string{}
	}
	if local == nil {
		local = []Flag{}
	}
	if inherited == nil {
		inherited = []Flag{}
	}
	return Command{
		Path:           path,
		Runnable:       true,
		Aliases:        aliases,
		LocalFlags:     local,
		InheritedFlags: inherited,
	}
}
