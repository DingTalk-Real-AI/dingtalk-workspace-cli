// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"errors"
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
			name: "canonical remains optional",
			input: strings.Replace(
				validFlagMigrationManifestJSON,
				`"after": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				`"after": {"present": true, "type": "string", "scope": "local"}`,
				1,
			),
			wantErr: "canonical flag must be required after migration",
		},
		{
			name: "canonical was already required",
			input: strings.Replace(
				validFlagMigrationManifestJSON,
				`"before": {"present": false}`,
				`"before": {"present": true, "type": "string", "required": true, "scope": "local"}`,
				1,
			),
			wantErr: "canonical flag must be absent or optional before migration",
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
		Present: true,
		Type:    "string",
		Scope:   "local",
	}
	canonicalDrift.Migrations[0].Canonical.After.Type = "stringSlice"
	if err := canonicalDrift.Validate(); err == nil || !strings.Contains(err.Error(), "canonical flag type") {
		t.Fatalf("canonical type drift error = %v", err)
	}

	_, err = ReadFlagMigrationManifest(strings.NewReader(validFlagMigrationManifestJSON + ` {}`))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON error = %v", err)
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
