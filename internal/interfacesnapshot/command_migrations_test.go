// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type commandMigrationErrorReader struct{}

func (commandMigrationErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("command migration read failure")
}

func TestApprovedCommandMigrationManifestRemainsValid(t *testing.T) {
	manifest, err := os.Open("../../scripts/policy/interface-migrations/approved-command-migrations-v1.json")
	if err != nil {
		t.Fatalf("open approved command migration manifest: %v", err)
	}
	defer manifest.Close()
	if _, err := ReadCommandMigrationManifest(manifest); err != nil {
		t.Fatalf("approved command migration manifest is invalid: %v", err)
	}
}

func TestCrossPlatformCoverageReadCommandMigrationManifestFailsClosed(t *testing.T) {
	valid := commandMigrationManifestJSON()
	if _, err := ReadCommandMigrationManifest(strings.NewReader(valid)); err != nil {
		t.Fatalf("valid command migration manifest: %v", err)
	}
	validExtraction := flagExtractionCommandMigrationManifestJSON()
	if _, err := ReadCommandMigrationManifest(strings.NewReader(validExtraction)); err != nil {
		t.Fatalf("valid flag extraction command migration manifest: %v", err)
	}
	for _, test := range []struct {
		input string
		want  string
	}{
		{strings.Replace(valid, `"state": "pending"`, `"state": "pending", "allow": true`, 1), "unknown field"},
		{strings.Replace(valid, `"state": "pending"`, `"state": "pending", "state": "consumed"`, 1), "duplicate field"},
		{strings.Replace(valid, `"version": 1`, `"version": null`, 1), "must be"},
		{strings.Replace(valid, "dws chat message old", "dws chat *", 1), "exact command paths"},
		{strings.Replace(valid, `"kind": "command_move"`, `"kind": "anything"`, 1), "invalid kind"},
		{`{"version":1,"migrations":null}`, "must be an array"},
		{strings.Replace(validExtraction, `"replacement_constant":{"property":"convThreadEnabled","value":true}`, `"replacement_constant":null`, 1), "must be an object"},
		{strings.Replace(validExtraction, `,"value":true`, ``, 1), `must be true`},
		{strings.Replace(validExtraction, `"value":true`, `"value":"true"`, 1), "bool"},
	} {
		if _, err := ReadCommandMigrationManifest(strings.NewReader(test.input)); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("ReadCommandMigrationManifest() error=%v, want %q", err, test.want)
		}
	}
	if _, err := ReadCommandMigrationManifest(commandMigrationErrorReader{}); err == nil || !strings.Contains(err.Error(), "read failure") {
		t.Fatalf("reader error=%v", err)
	}
	for _, input := range []string{valid + ` {}`, valid + ` {`} {
		if _, err := ReadCommandMigrationManifest(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Fatalf("trailing input error=%v", err)
		}
	}
}

func TestCrossPlatformCoverageCommandMigrationValidationEdges(t *testing.T) {
	validMove := commandMigrationManifest(CommandMigrationPending).Migrations[0]
	validExtraction := commandMigrationManifest(CommandMigrationPending).Migrations[1]

	duplicate := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{validMove, validMove}}
	if err := duplicate.Validate(); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate error=%v", err)
	}
	if err := (CommandMigrationManifest{Version: CommandMigrationManifestVersion}).Validate(); err == nil || !strings.Contains(err.Error(), "must be an array") {
		t.Fatalf("nil migrations error=%v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*CommandMigration)
		want   string
	}{
		{"same command", func(m *CommandMigration) { m.Replacement.Command = m.Legacy.Command }, "must differ"},
		{"empty reason", func(m *CommandMigration) { m.Reason = " " }, "non-empty trimmed"},
		{"invalid state", func(m *CommandMigration) { m.State = "approved" }, "invalid state"},
		{"invalid command state", func(m *CommandMigration) { m.Replacement.Before.Runnable = true }, "absent state"},
		{"legacy contract", func(m *CommandMigration) { m.Legacy.Before.Hidden = true }, "remain runnable"},
		{"replacement contract", func(m *CommandMigration) { m.Replacement.After.Hidden = true }, "absent to visible"},
		{"schema contract", func(m *CommandMigration) { m.Schema.ProductID = "bad/id" }, "exact identifier"},
		{"move not hidden", func(m *CommandMigration) { m.Legacy.After.Hidden = false }, "must migrate exactly"},
		{"move flag", func(m *CommandMigration) { m.LegacyFlag.Name = "thread" }, "must not declare legacy_flag"},
		{"move tool identity", func(m *CommandMigration) { m.Schema.ReplacementToolID = "chat.new" }, "stable Schema tool"},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := validMove
			test.mutate(&migration)
			if err := migration.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error=%v, want %q", err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name   string
		mutate func(*CommandMigration)
		want   string
	}{
		{"legacy changed", func(m *CommandMigration) { m.Legacy.After.Hidden = true }, "remain visible"},
		{"invalid legacy flag", func(m *CommandMigration) { m.LegacyFlag.Name = "bad name" }, "exact flag"},
		{"same tool", func(m *CommandMigration) { m.Schema.ReplacementToolID = m.Schema.SourceToolID }, "distinct replacement"},
		{"wrong parameter", func(m *CommandMigration) { m.Schema.Parameters[3].From = "other" }, "exact extracted flag"},
		{"wrong type", func(m *CommandMigration) {
			m.LegacyFlag.Before.Type = "string"
			m.LegacyFlag.After.Type = "string"
		}, "optional bool"},
		{"wrong no opt", func(m *CommandMigration) {
			m.LegacyFlag.Before.NoOpt = "false"
			m.LegacyFlag.After.NoOpt = "false"
		}, "no_opt"},
		{"missing extracted mapping", func(m *CommandMigration) {
			m.Schema.Parameters = m.Schema.Parameters[:3]
		}, "exactly one replacement constant"},
		{"duplicate extracted mapping", func(m *CommandMigration) {
			m.Schema.Parameters = append(m.Schema.Parameters, m.Schema.Parameters[3])
		}, "duplicates from"},
		{"multiple constants", func(m *CommandMigration) {
			m.Schema.Parameters[0] = CommandParameterMigration{
				From: "name",
				ReplacementConstant: &CommandReplacementConstant{
					Property: "name",
					Value:    true,
				},
			}
		}, "exactly one replacement constant"},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := cloneCommandMigration(validExtraction)
			test.mutate(&migration)
			if err := migration.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error=%v, want %q", err, test.want)
			}
		})
	}
	falseConstant := cloneCommandMigration(validExtraction)
	falseConstant.Schema.Parameters[3].ReplacementConstant.Value = false
	falseConstant.LegacyFlag.Before.NoOpt = "false"
	falseConstant.LegacyFlag.After.NoOpt = "false"
	if err := falseConstant.validate(); err == nil || !strings.Contains(err.Error(), "must be true") {
		t.Fatalf("false replacement constant validation error=%v, want v1 true-only rejection", err)
	}

	for _, state := range []CommandMigrationState{
		{Runnable: true},
		{Present: true},
	} {
		if err := state.validate("state"); err == nil {
			t.Fatalf("invalid command state accepted: %#v", state)
		}
	}

	for _, test := range []struct {
		name   string
		mutate func(*CommandMigrationFlag)
		want   string
	}{
		{"name", func(f *CommandMigrationFlag) { f.Name = "bad name" }, "exact flag"},
		{"before state", func(f *CommandMigrationFlag) { f.Before = FlagMigrationState{Present: true} }, "requires type"},
		{"after state", func(f *CommandMigrationFlag) { f.After = FlagMigrationState{Present: true} }, "requires type"},
		{"visibility", func(f *CommandMigrationFlag) { f.After.Hidden = false }, "visible to hidden"},
		{"attribute", func(f *CommandMigrationFlag) { f.After.Type = "string" }, "only hidden visibility"},
	} {
		t.Run("flag "+test.name, func(t *testing.T) {
			flag := validExtraction.LegacyFlag
			test.mutate(&flag)
			if err := flag.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("flag validate error=%v, want %q", err, test.want)
			}
		})
	}

	validSchema := validMove.Schema
	for _, test := range []struct {
		name   string
		kind   string
		mutate func(*CommandMigrationSchema)
		want   string
	}{
		{"identifier", CommandMigrationMove, func(s *CommandMigrationSchema) { s.ProductID = "bad/id" }, "exact identifier"},
		{"replacement identifier", CommandMigrationMove, func(s *CommandMigrationSchema) { s.ReplacementToolID = "bad/id" }, "replacement_tool_id"},
		{"nil parameters", CommandMigrationMove, func(s *CommandMigrationSchema) { s.Parameters = nil }, "must be an array"},
		{"bad from", CommandMigrationMove, func(s *CommandMigrationSchema) { s.Parameters[0].From = "bad name" }, "exact parameter"},
		{"duplicate from", CommandMigrationMove, func(s *CommandMigrationSchema) { s.Parameters = append(s.Parameters, s.Parameters[0]) }, "duplicates from"},
		{"bad to", CommandMigrationMove, func(s *CommandMigrationSchema) { s.Parameters[0].To = s.Parameters[0].From }, "distinct exact"},
		{"duplicate to", CommandMigrationMove, func(s *CommandMigrationSchema) {
			s.Parameters = append(s.Parameters, CommandParameterMigration{From: "other", To: s.Parameters[0].To})
		}, "duplicates to"},
		{"move constant", CommandMigrationMove, func(s *CommandMigrationSchema) {
			s.Parameters[0].ReplacementConstant = &CommandReplacementConstant{Property: "property", Value: true}
		}, "must not declare replacement_constant"},
		{"invalid kind", "anything", func(*CommandMigrationSchema) {}, "invalid command migration kind"},
	} {
		t.Run("schema "+test.name, func(t *testing.T) {
			schema := validSchema
			schema.Parameters = append([]CommandParameterMigration(nil), validSchema.Parameters...)
			test.mutate(&schema)
			if err := schema.validate(test.kind); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("schema validate error=%v, want %q", err, test.want)
			}
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(*CommandMigrationSchema)
		want   string
	}{
		{"missing target", func(s *CommandMigrationSchema) { s.Parameters[0].To = "" }, "requires an exact to or replacement_constant"},
		{"both targets", func(s *CommandMigrationSchema) {
			s.Parameters[0].ReplacementConstant = &CommandReplacementConstant{Property: "name", Value: true}
		}, "exactly one target"},
		{"bad to", func(s *CommandMigrationSchema) { s.Parameters[0].To = "bad name" }, "to must be an exact parameter name"},
		{"bad constant property", func(s *CommandMigrationSchema) { s.Parameters[3].ReplacementConstant.Property = "bad/property" }, "exact property"},
		{"duplicate parameter target", func(s *CommandMigrationSchema) { s.Parameters[1].To = s.Parameters[0].To }, "duplicates to"},
		{"duplicate constant target", func(s *CommandMigrationSchema) {
			s.Parameters[0] = CommandParameterMigration{From: "name", ReplacementConstant: &CommandReplacementConstant{Property: "convThreadEnabled", Value: true}}
		}, "duplicates replacement_constant property"},
		{"constant before parameter target", func(s *CommandMigrationSchema) {
			s.Parameters[3].ReplacementConstant.Property = "name"
			s.Parameters[0], s.Parameters[3] = s.Parameters[3], s.Parameters[0]
		}, "duplicates parameter target"},
		{"parameter target before constant", func(s *CommandMigrationSchema) {
			s.Parameters[3].ReplacementConstant.Property = "name"
		}, "duplicates parameter target"},
	} {
		t.Run("extraction schema "+test.name, func(t *testing.T) {
			schema := cloneCommandMigration(validExtraction).Schema
			test.mutate(&schema)
			if err := schema.validate(CommandMigrationFlagExtraction); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("schema validate error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaAvailabilityMigrationLifecycle(t *testing.T) {
	pending := availabilityCommandMigrationManifest(CommandMigrationPending)
	consumed := availabilityCommandMigrationManifest(CommandMigrationConsumed)
	empty := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	before := testSnapshot(
		testCommand("dws"),
		testCommand("dws devapp"),
		testCommand("dws devapp +event-list"),
	)
	after := testSnapshot(
		testCommand("dws"),
		testCommand("dws devapp"),
		Command{Path: "dws devapp +event-list", Runnable: true, Hidden: true},
	)

	got, err := AuthorizeCommandMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		pending,
		consumed,
	)
	if err != nil || len(got) != 1 {
		t.Fatalf("availability lifecycle authorizations=%#v error=%v", got, err)
	}
	report, err := CompareAllWithInterfaceMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}},
		FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}},
		pending,
		consumed,
	)
	if err != nil || !report.Compatible {
		t.Fatalf("availability interface report compatible=%v error=%v report=%#v", report.Compatible, err, report)
	}
	if got, err := AuthorizeCommandMigrations(
		before,
		map[string]Snapshot{"merge-base": before, "stable": before},
		empty,
		pending,
	); err != nil || len(got) != 0 {
		t.Fatalf("candidate-added pending migration must remain inert: %#v, %v", got, err)
	}

	valid := pending.Migrations[0]
	compatibilityVisible := cloneCommandMigration(valid)
	compatibilityVisible.Legacy.After = compatibilityVisible.Legacy.Before
	if err := compatibilityVisible.validate(); err != nil {
		t.Fatalf("compatibility-visible availability migration must validate: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*CommandMigration)
		want   string
	}{
		{"legacy path", func(m *CommandMigration) { m.Legacy.Command = "dws devapp *" }, "legacy must be an exact"},
		{"replacement", func(m *CommandMigration) { m.Replacement.Command = "dws devapp other" }, "must not declare replacement"},
		{"absent after", func(m *CommandMigration) { m.Legacy.After = CommandMigrationState{} }, "visible to hidden or remain compatibility-visible"},
		{"legacy flag", func(m *CommandMigration) { m.LegacyFlag.Name = "legacy" }, "must not declare legacy_flag"},
		{"missing availability", func(m *CommandMigration) { m.Schema.Availability = nil }, "requires availability"},
		{"wrong availability", func(m *CommandMigration) { m.Schema.Availability.After = "available" }, "requires availability"},
		{"parameter mapping", func(m *CommandMigration) {
			m.Schema.Parameters = []CommandParameterMigration{{From: "cursor", To: "next-token"}}
		}, "must not declare"},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := cloneCommandMigration(valid)
			if valid.Schema.Availability != nil {
				change := *valid.Schema.Availability
				migration.Schema.Availability = &change
			}
			test.mutate(&migration)
			if err := migration.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("availability migration validation error=%v, want %q", err, test.want)
			}
		})
	}
	if got := valid.displayKey(); !strings.Contains(got, CommandMigrationAvailability) {
		t.Fatalf("availability display key = %q", got)
	}
	if got := matchCommandMigrationPhase(testSnapshot(testCommand("dws")), valid); got != commandMigrationPartial {
		t.Fatalf("missing availability command phase = %q", got)
	}
	parent := before
	parent.Commands = append(append([]Command(nil), before.Commands...), testCommand("dws devapp +event-list detail"))
	if err := validateCommandMoveLegacyLeaves(parent, pending); err == nil || !strings.Contains(err.Error(), "must be a leaf") {
		t.Fatalf("availability parent command error = %v", err)
	}
	moveSchema := commandMigrationManifest(CommandMigrationPending).Migrations[0].Schema
	moveSchema.Availability = &CommandAvailabilityChange{Before: "available", After: "unavailable"}
	if err := moveSchema.validate(CommandMigrationMove); err == nil || !strings.Contains(err.Error(), "must not declare schema availability") {
		t.Fatalf("ordinary move availability error = %v", err)
	}
}

func TestCrossPlatformCoverageCompatibilityVisibleAvailabilityMigrationLifecycle(t *testing.T) {
	classicPending := availabilityCommandMigrationManifest(CommandMigrationPending)
	steadyPending := compatibilityVisibleAvailabilityCommandMigrationManifest(CommandMigrationPending)
	steadyConsumed := compatibilityVisibleAvailabilityCommandMigrationManifest(CommandMigrationConsumed)
	empty := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	visible := testSnapshot(
		testCommand("dws"),
		testCommand("dws devapp"),
		testCommand("dws devapp +event-list"),
	)
	hidden := testSnapshot(
		testCommand("dws"),
		testCommand("dws devapp"),
		Command{Path: "dws devapp +event-list", Runnable: true, Hidden: true},
	)
	references := map[string]Snapshot{"merge-base": visible, "stable": visible}

	if got, err := AuthorizeCommandMigrations(visible, references, classicPending, steadyPending); err != nil || len(got) != 0 {
		t.Fatalf("pending compatibility refinement authorizations=%#v error=%v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(visible, references, steadyPending, steadyPending); err != nil || len(got) != 0 {
		t.Fatalf("pending compatibility receipt authorizations=%#v error=%v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(visible, references, steadyPending, steadyConsumed); err != nil || len(got) != 1 {
		t.Fatalf("consumed compatibility receipt authorizations=%#v error=%v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(visible, references, steadyConsumed, steadyConsumed); err != nil || len(got) != 1 {
		t.Fatalf("retained consumed compatibility receipt authorizations=%#v error=%v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(visible, references, empty, steadyPending); err != nil || len(got) != 0 {
		t.Fatalf("candidate-added compatibility receipt authorizations=%#v error=%v", got, err)
	}

	modified := compatibilityVisibleAvailabilityCommandMigrationManifest(CommandMigrationPending)
	modified.Migrations[0].Reason = "Changed reason."
	if _, err := AuthorizeCommandMigrations(visible, references, classicPending, modified); err == nil || !strings.Contains(err.Error(), "modified base-owned") {
		t.Fatalf("modified compatibility refinement error=%v", err)
	}
	if _, err := AuthorizeCommandMigrations(visible, references, classicPending, steadyConsumed); err == nil || !strings.Contains(err.Error(), "modified base-owned") {
		t.Fatalf("refinement and consumption in one change error=%v", err)
	}
	if _, err := AuthorizeCommandMigrations(hidden, references, steadyPending, steadyPending); err == nil || !strings.Contains(err.Error(), "drifted from compatibility-visible") {
		t.Fatalf("hidden compatibility command error=%v", err)
	}
	if _, err := AuthorizeCommandMigrations(visible, references, steadyConsumed, empty); err == nil || !strings.Contains(err.Error(), "must retain compatibility-visible") {
		t.Fatalf("removed consumed compatibility receipt error=%v", err)
	}
	if _, err := AuthorizeCommandMigrations(visible, references, steadyConsumed, steadyPending); err == nil || !strings.Contains(err.Error(), "back to pending") {
		t.Fatalf("consumed compatibility receipt reverted error=%v", err)
	}
}

func TestCrossPlatformCoveragePendingCommandMigrationRetargetLifecycle(t *testing.T) {
	approved := commandMigrationManifest(CommandMigrationPending)
	retargeted := retargetedCommandMigrationManifest(CommandMigrationPending)
	before := commandMigrationSnapshot(false, false)
	after := retargetedCommandMigrationSnapshot()
	references := map[string]Snapshot{"merge-base": before, "stable": before}

	got, err := AuthorizeCommandMigrations(before, references, approved, retargeted)
	if err != nil || len(got) != 0 {
		t.Fatalf("pending retarget authorizations=%#v error=%v", got, err)
	}
	consumed := retargetedCommandMigrationManifest(CommandMigrationConsumed)
	got, err = AuthorizeCommandMigrations(after, references, retargeted, consumed)
	if err != nil || len(got) != len(retargeted.Migrations) {
		t.Fatalf("retargeted migrations were not consumable: authorizations=%#v error=%v", got, err)
	}

	forked := CommandMigrationManifest{
		Version:    CommandMigrationManifestVersion,
		Migrations: append(append([]CommandMigration(nil), approved.Migrations...), retargeted.Migrations...),
	}
	if err := forked.Validate(); err == nil || !strings.Contains(err.Error(), "forks pending migration lineage") {
		t.Fatalf("parallel pending targets error=%v", err)
	}

	selfAuthorized := retargetedCommandMigrationSnapshot()
	if _, err := AuthorizeCommandMigrations(selfAuthorized, references, approved, retargeted); err == nil || !strings.Contains(err.Error(), "not in the exact before state") {
		t.Fatalf("retarget plus surface change error=%v", err)
	}
	stableAfter := map[string]Snapshot{"merge-base": before, "stable": after}
	if _, err := AuthorizeCommandMigrations(before, stableAfter, approved, retargeted); err == nil || !strings.Contains(err.Error(), "not in the exact before state") {
		t.Fatalf("retarget after stable rollout error=%v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*CommandMigration)
	}{
		{"consumed target", func(m *CommandMigration) { m.State = CommandMigrationConsumed }},
		{"legacy command", func(m *CommandMigration) { m.Legacy.Command = "dws chat message other" }},
		{"legacy state", func(m *CommandMigration) { m.Legacy.After.Hidden = false }},
		{"replacement state", func(m *CommandMigration) { m.Replacement.After.Hidden = true }},
		{"product", func(m *CommandMigration) { m.Schema.ProductID = "other" }},
		{"source tool", func(m *CommandMigration) {
			m.Schema.SourceToolID = "chat.other"
			m.Schema.ReplacementToolID = "chat.other"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := retargetedCommandMigrationManifest(CommandMigrationPending)
			test.mutate(&candidate.Migrations[0])
			if _, err := AuthorizeCommandMigrations(before, references, approved, candidate); err == nil {
				t.Fatal("invalid pending retarget accepted")
			}
		})
	}

	differentLegacy := cloneCommandMigration(approved.Migrations[0])
	differentLegacy.Legacy.Command = "dws chat message another"
	differentLegacy.Replacement.Command = "dws chat thread another"
	moveFork := CommandMigrationManifest{
		Version:    CommandMigrationManifestVersion,
		Migrations: []CommandMigration{approved.Migrations[0], differentLegacy},
	}
	if err := moveFork.Validate(); err == nil || !strings.Contains(err.Error(), "forks pending migration lineage") {
		t.Fatalf("Schema source tool fork error=%v", err)
	}
}

func TestCrossPlatformCoverageFlagExtractionRejectsRequiredLegacyFlag(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*CommandMigrationFlag)
	}{
		{"before required", func(flag *CommandMigrationFlag) { flag.Before.Required = true }},
		{"after required", func(flag *CommandMigrationFlag) { flag.After.Required = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := commandMigrationManifest(CommandMigrationPending).Migrations[1]
			test.mutate(&migration.LegacyFlag)

			if err := migration.validate(); err == nil || !strings.Contains(err.Error(), "optional") {
				t.Fatalf("required legacy flag validation error=%v, want optional-only rejection", err)
			}
		})
	}
}

func TestCrossPlatformCoverageCommandMoveRejectsRunnableParent(t *testing.T) {
	migration := commandMigrationManifest(CommandMigrationPending).Migrations[0]
	migration.Legacy.Command = "dws agoal"
	migration.Replacement.Command = "dws goal"
	migration.Schema = CommandMigrationSchema{
		ProductID:         "agoal",
		SourceToolID:      "agoal.root",
		ReplacementToolID: "agoal.root",
		Parameters:        []CommandParameterMigration{},
	}
	authority := CommandMigrationManifest{
		Version:    CommandMigrationManifestVersion,
		Migrations: []CommandMigration{migration},
	}
	consumed := authority
	consumed.Migrations = append([]CommandMigration(nil), authority.Migrations...)
	consumed.Migrations[0].State = CommandMigrationConsumed

	before := testSnapshot(
		testCommand("dws"),
		testCommand("dws agoal"),
		testCommand("dws agoal strategy"),
		testCommand("dws agoal strategy list"),
	)
	after := testSnapshot(
		testCommand("dws"),
		Command{Path: "dws agoal", Runnable: true, Hidden: true},
		testCommand("dws goal"),
	)

	_, err := AuthorizeCommandMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		authority,
		consumed,
	)
	if err == nil || !strings.Contains(err.Error(), "must be a leaf") {
		t.Fatalf("runnable parent command_move error=%v, want leaf-only rejection", err)
	}
}

func TestCrossPlatformCoverageCommandMoveAllowsReplacementParent(t *testing.T) {
	before := commandMigrationSnapshot(false, false)
	after := commandMigrationSnapshot(true, false)
	after.Commands = append(after.Commands, testCommand("dws chat topic new detail"))
	pending := singleCommandMigrationManifest(CommandMigrationPending)
	consumed := singleCommandMigrationManifest(CommandMigrationConsumed)
	if err := validateCommandMoveLegacyLeaves(testSnapshot(testCommand("dws")), pending); err != nil {
		t.Fatalf("absent legacy topology should remain a lifecycle concern: %v", err)
	}

	got, err := AuthorizeCommandMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("replacement parent command_move was rejected: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("replacement parent command_move authorizations=%d, want 1", len(got))
	}
}

func TestCrossPlatformCoverageCommandMoveRejectsAncestorOverlap(t *testing.T) {
	for _, test := range []struct {
		name        string
		legacy      string
		replacement string
	}{
		{
			name:        "legacy ancestor",
			legacy:      "dws chat message",
			replacement: "dws chat message list",
		},
		{
			name:        "replacement ancestor",
			legacy:      "dws chat message list",
			replacement: "dws chat message",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := commandMigrationManifest(CommandMigrationPending).Migrations[0]
			migration.Legacy.Command = test.legacy
			migration.Replacement.Command = test.replacement

			if err := migration.validate(); err == nil || !strings.Contains(err.Error(), "must not have an ancestor relationship") {
				t.Fatalf("overlapping command_move error=%v, want ancestor-overlap rejection", err)
			}
		})
	}
}

func TestCrossPlatformCoverageCommandMigrationLifecycleEdges(t *testing.T) {
	before := commandMigrationSnapshot(false, false)
	after := commandMigrationSnapshot(true, false)
	partial := commandMigrationSnapshot(false, false)
	partial.Commands = append(partial.Commands, testCommand("dws chat topic new"))
	pending := singleCommandMigrationManifest(CommandMigrationPending)
	consumed := singleCommandMigrationManifest(CommandMigrationConsumed)
	empty := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}

	if got, err := AuthorizeCommandMigrations(before, map[string]Snapshot{}, empty, empty); err != nil || len(got) != 0 {
		t.Fatalf("empty lifecycle=%#v, %v", got, err)
	}
	for _, test := range []struct {
		name       string
		current    Snapshot
		references map[string]Snapshot
		authority  CommandMigrationManifest
		candidate  CommandMigrationManifest
		want       string
	}{
		{"missing stable", before, map[string]Snapshot{"main": before}, pending, pending, "stable reference"},
		{"missing base", before, map[string]Snapshot{"stable": before}, pending, pending, "main or merge-base"},
		{"pending base after", after, map[string]Snapshot{"main": after, "stable": before}, pending, pending, "want exact before"},
		{"modified approval", before, map[string]Snapshot{"main": before, "stable": before}, pending, modifiedCommandManifest(pending), "modified base-owned"},
		{"pending removed", before, map[string]Snapshot{"main": before, "stable": before}, pending, empty, "removed pending"},
		{"false consumed", before, map[string]Snapshot{"main": before, "stable": before}, pending, consumed, "falsely consumed"},
		{"after pending receipt", after, map[string]Snapshot{"main": before, "stable": before}, pending, pending, "without marking it consumed"},
		{"partial", partial, map[string]Snapshot{"main": before, "stable": before}, pending, consumed, "partially applied"},
		{"consumed drift", before, map[string]Snapshot{"main": after, "stable": before}, consumed, consumed, "drifted from consumed"},
		{"early cleanup", after, map[string]Snapshot{"main": after, "stable": before}, consumed, empty, "must retain consumed"},
		{"consumed back to pending", after, map[string]Snapshot{"main": after, "stable": before}, consumed, pending, "must retain consumed"},
		{"inert consumed back to pending", after, map[string]Snapshot{"main": after, "stable": after}, consumed, pending, "back to pending"},
		{"candidate added consumed", after, map[string]Snapshot{"main": before, "stable": before}, empty, consumed, "must start pending"},
		{"candidate base mismatch", before, map[string]Snapshot{"main": after, "stable": before}, empty, pending, "does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := AuthorizeCommandMigrations(test.current, test.references, test.authority, test.candidate)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("lifecycle error=%v, want %q", err, test.want)
			}
		})
	}
	if got, err := AuthorizeCommandMigrations(after, map[string]Snapshot{"main": after, "stable": before}, consumed, consumed); err != nil || len(got) != 1 {
		t.Fatalf("retained consumed receipt=%#v, %v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(after, map[string]Snapshot{"main": after, "stable": after}, consumed, empty); err != nil || len(got) != 0 {
		t.Fatalf("cleaned stale receipt=%#v, %v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(after, map[string]Snapshot{"main": after, "stable": after}, consumed, consumed); err != nil || len(got) != 0 {
		t.Fatalf("retained inert receipt=%#v, %v", got, err)
	}
	if got, err := AuthorizeCommandMigrations(before, map[string]Snapshot{"main": before, "stable": before}, empty, pending); err != nil || len(got) != 0 {
		t.Fatalf("candidate pending plan=%#v, %v", got, err)
	}

	invalidSnapshot := before
	invalidSnapshot.SchemaVersion = 0
	for _, test := range []struct {
		name       string
		current    Snapshot
		references map[string]Snapshot
		authority  CommandMigrationManifest
		candidate  CommandMigrationManifest
	}{
		{"current", invalidSnapshot, map[string]Snapshot{"main": before}, empty, empty},
		{"reference", before, map[string]Snapshot{"main": invalidSnapshot}, empty, empty},
		{"authority", before, map[string]Snapshot{"main": before}, CommandMigrationManifest{}, empty},
		{"candidate", before, map[string]Snapshot{"main": before}, empty, CommandMigrationManifest{}},
	} {
		t.Run("invalid "+test.name, func(t *testing.T) {
			if _, err := AuthorizeCommandMigrations(test.current, test.references, test.authority, test.candidate); err == nil {
				t.Fatal("invalid authorization input accepted")
			}
		})
	}
	invalidFlags := FlagMigrationManifest{}
	validFlags := FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}}
	validCommands := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	if _, err := CompareAllWithInterfaceMigrations(before, map[string]Snapshot{"main": before}, invalidFlags, validFlags, validCommands, validCommands); err == nil {
		t.Fatal("combined compare accepted invalid flag lifecycle")
	}
	if _, err := CompareAllWithInterfaceMigrations(before, map[string]Snapshot{"main": before}, validFlags, validFlags, CommandMigrationManifest{}, validCommands); err == nil {
		t.Fatal("combined compare accepted invalid command lifecycle")
	}
	if commandMigrationAuthorizesChange(before, before, Change{Kind: "command_removed", Path: "dws unrelated"}, pending.Migrations) {
		t.Fatal("unrelated command change was authorized")
	}
}

func TestCrossPlatformCoverageCommandMigrationLifecycleAndExactFiltering(t *testing.T) {
	before := commandMigrationSnapshot(false, false)
	after := commandMigrationSnapshot(true, false)
	pending := commandMigrationManifest(CommandMigrationPending)
	consumed := commandMigrationManifest(CommandMigrationConsumed)
	emptyFlags := FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}}

	report, err := CompareAllWithInterfaceMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		emptyFlags,
		emptyFlags,
		pending,
		consumed,
	)
	if err != nil {
		t.Fatalf("governed command migration: %v", err)
	}
	if !report.Compatible {
		t.Fatalf("exact command migration remained blocking: %#v", report.Comparisons)
	}

	unrelated := commandMigrationSnapshot(true, true)
	report, err = CompareAllWithInterfaceMigrations(
		unrelated,
		map[string]Snapshot{"merge-base": before, "stable": before},
		emptyFlags,
		emptyFlags,
		pending,
		consumed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible || !hasChangeKind(report.Comparisons[0].Blocking, "flag_removed") {
		t.Fatalf("unrelated flag removal was hidden: %#v", report.Comparisons)
	}

	emptyCommands := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	if _, err := AuthorizeCommandMigrations(
		after,
		map[string]Snapshot{"merge-base": before, "stable": before},
		emptyCommands,
		pending,
	); err == nil || !strings.Contains(err.Error(), "cannot authorize its own interface change") {
		t.Fatalf("candidate self-authorization error=%v", err)
	}

	partial := commandMigrationSnapshot(true, false)
	partial.Commands = partial.Commands[:len(partial.Commands)-1]
	if _, err := AuthorizeCommandMigrations(
		partial,
		map[string]Snapshot{"merge-base": before, "stable": before},
		pending,
		consumed,
	); err == nil || !strings.Contains(err.Error(), "partially applied") {
		t.Fatalf("partial command migration error=%v", err)
	}
}

func TestCrossPlatformCoverageFlagExtractionRequiresReplacementConstantEvidence(t *testing.T) {
	before := commandMigrationSnapshot(false, false)
	after := commandMigrationSnapshot(true, false)
	pending := flagExtractionCommandMigrationManifest(CommandMigrationPending)
	consumed := flagExtractionCommandMigrationManifest(CommandMigrationConsumed)
	empty := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	migration := pending.Migrations[0]

	if phase := matchCommandMigrationPhase(before, migration); phase != commandMigrationBefore {
		t.Fatalf("before phase=%s, want %s", phase, commandMigrationBefore)
	}
	if phase := matchCommandMigrationPhase(after, migration); phase != commandMigrationAfter {
		t.Fatalf("after phase=%s, want %s", phase, commandMigrationAfter)
	}

	for _, test := range []struct {
		name   string
		values map[string]bool
	}{
		{name: "missing", values: nil},
		{name: "wrong property", values: map[string]bool{"otherProperty": true}},
		{name: "wrong value", values: map[string]bool{"convThreadEnabled": false}},
		{name: "extra", values: map[string]bool{"convThreadEnabled": true, "otherProperty": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := withCommandBoolConstParams(after, "dws chat topic create", test.values)
			if phase := matchCommandMigrationPhase(current, migration); phase != commandMigrationPartial {
				t.Fatalf("phase=%s, want %s", phase, commandMigrationPartial)
			}
			if _, err := AuthorizeCommandMigrations(
				current,
				map[string]Snapshot{"merge-base": before, "stable": before},
				pending,
				consumed,
			); err == nil || !strings.Contains(err.Error(), "partially applied") {
				t.Fatalf("constant evidence lifecycle error=%v, want partial rejection", err)
			}
			if _, err := AuthorizeCommandMigrations(
				current,
				map[string]Snapshot{"merge-base": before, "stable": before},
				empty,
				pending,
			); err == nil || !strings.Contains(err.Error(), "cannot authorize its own interface change") {
				t.Fatalf("constant evidence self-authorization error=%v", err)
			}
		})
	}
}

func commandMigrationManifestJSON() string {
	return `{
  "version": 1,
  "migrations": [{
    "kind": "command_move",
    "legacy": {
      "command": "dws chat message old",
      "before": {"present": true, "runnable": true},
      "after": {"present": true, "runnable": true, "hidden": true}
    },
    "replacement": {
      "command": "dws chat topic new",
      "before": {"present": false},
      "after": {"present": true, "runnable": true}
    },
    "schema": {
      "product_id": "chat",
      "source_tool_id": "chat.old",
      "replacement_tool_id": "chat.old",
      "parameters": [{"from": "old-id", "to": "new-id"}]
    },
    "state": "pending",
    "reason": "Reviewed command move."
  }]
}`
}

func flagExtractionCommandMigrationManifestJSON() string {
	return `{
  "version": 1,
  "migrations": [{
    "kind": "flag_extraction",
    "legacy": {
      "command": "dws chat group create",
      "before": {"present": true, "runnable": true},
      "after": {"present": true, "runnable": true}
    },
    "replacement": {
      "command": "dws chat topic create",
      "before": {"present": false},
      "after": {"present": true, "runnable": true}
    },
    "legacy_flag": {
      "name": "thread",
      "before": {"present": true, "type": "bool", "no_opt": "true", "scope": "local"},
      "after": {"present": true, "type": "bool", "hidden": true, "no_opt": "true", "scope": "local"}
    },
    "schema": {
      "product_id": "chat",
      "source_tool_id": "chat.create_group",
      "replacement_tool_id": "chat.create_topic",
      "parameters": [
        {"from":"name","to":"name"},
        {"from":"type","to":"type"},
        {"from":"users","to":"users"},
        {"from":"thread","replacement_constant":{"property":"convThreadEnabled","value":true}}
      ]
    },
    "state": "pending",
    "reason": "Reviewed flag extraction."
  }]
}`
}

func commandMigrationManifest(state string) CommandMigrationManifest {
	move := CommandMigration{
		Kind: CommandMigrationMove,
		Legacy: CommandMigrationSide{
			Command: "dws chat message old",
			Before:  CommandMigrationState{Present: true, Runnable: true},
			After:   CommandMigrationState{Present: true, Runnable: true, Hidden: true},
		},
		Replacement: CommandMigrationSide{
			Command: "dws chat topic new",
			Before:  CommandMigrationState{},
			After:   CommandMigrationState{Present: true, Runnable: true},
		},
		Schema: CommandMigrationSchema{
			ProductID:         "chat",
			SourceToolID:      "chat.old",
			ReplacementToolID: "chat.old",
			Parameters:        []CommandParameterMigration{{From: "old-id", To: "new-id"}},
		},
		State:  state,
		Reason: "Reviewed command move.",
	}
	extraction := CommandMigration{
		Kind: CommandMigrationFlagExtraction,
		Legacy: CommandMigrationSide{
			Command: "dws chat group create",
			Before:  CommandMigrationState{Present: true, Runnable: true},
			After:   CommandMigrationState{Present: true, Runnable: true},
		},
		Replacement: CommandMigrationSide{
			Command: "dws chat topic create",
			Before:  CommandMigrationState{},
			After:   CommandMigrationState{Present: true, Runnable: true},
		},
		LegacyFlag: CommandMigrationFlag{
			Name:   "thread",
			Before: FlagMigrationState{Present: true, Type: "bool", NoOpt: "true", Scope: "local"},
			After:  FlagMigrationState{Present: true, Type: "bool", Hidden: true, NoOpt: "true", Scope: "local"},
		},
		Schema: CommandMigrationSchema{
			ProductID:         "chat",
			SourceToolID:      "chat.create_group",
			ReplacementToolID: "chat.create_topic",
			Parameters: []CommandParameterMigration{
				{From: "name", To: "name"},
				{From: "type", To: "type"},
				{From: "users", To: "users"},
				{
					From: "thread",
					ReplacementConstant: &CommandReplacementConstant{
						Property: "convThreadEnabled",
						Value:    true,
					},
				},
			},
		},
		State:  state,
		Reason: "Reviewed flag extraction.",
	}
	return CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{move, extraction}}
}

func flagExtractionCommandMigrationManifest(state string) CommandMigrationManifest {
	manifest := commandMigrationManifest(state)
	manifest.Migrations = manifest.Migrations[1:]
	return manifest
}

func retargetedCommandMigrationManifest(state string) CommandMigrationManifest {
	manifest := commandMigrationManifest(state)
	manifest.Migrations[0].Replacement.Command = "dws chat thread new"
	manifest.Migrations[0].Schema.Parameters = []CommandParameterMigration{}
	manifest.Migrations[0].Reason = "Retarget the unstarted move to the thread command."
	manifest.Migrations[1].Replacement.Command = "dws chat thread create-group"
	manifest.Migrations[1].Schema.ReplacementToolID = "chat.create_thread_group"
	manifest.Migrations[1].Schema.Parameters = []CommandParameterMigration{{
		From: "thread",
		ReplacementConstant: &CommandReplacementConstant{
			Property: "convThreadEnabled",
			Value:    true,
		},
	}}
	manifest.Migrations[1].Reason = "Retarget the unstarted extraction to the thread command."
	return manifest
}

func cloneCommandMigration(source CommandMigration) CommandMigration {
	cloned := source
	if source.Schema.Parameters != nil {
		cloned.Schema.Parameters = make([]CommandParameterMigration, len(source.Schema.Parameters))
		copy(cloned.Schema.Parameters, source.Schema.Parameters)
	}
	for index, parameter := range source.Schema.Parameters {
		if parameter.ReplacementConstant == nil {
			continue
		}
		constant := *parameter.ReplacementConstant
		cloned.Schema.Parameters[index].ReplacementConstant = &constant
	}
	return cloned
}

func singleCommandMigrationManifest(state string) CommandMigrationManifest {
	manifest := commandMigrationManifest(state)
	manifest.Migrations = manifest.Migrations[:1]
	return manifest
}

func availabilityCommandMigrationManifest(state string) CommandMigrationManifest {
	return CommandMigrationManifest{
		Version: CommandMigrationManifestVersion,
		Migrations: []CommandMigration{{
			Kind: CommandMigrationAvailability,
			Legacy: CommandMigrationSide{
				Command: "dws devapp +event-list",
				Before:  CommandMigrationState{Present: true, Runnable: true},
				After:   CommandMigrationState{Present: true, Runnable: true, Hidden: true},
			},
			Schema: CommandMigrationSchema{
				ProductID:    "devapp",
				SourceToolID: "devapp.shortcut_event_list",
				Parameters:   []CommandParameterMigration{},
				Availability: &CommandAvailabilityChange{Before: "available", After: "unavailable"},
			},
			State:  state,
			Reason: "Fail closed until terminal pagination evidence is available.",
		}},
	}
}

func compatibilityVisibleAvailabilityCommandMigrationManifest(state string) CommandMigrationManifest {
	manifest := availabilityCommandMigrationManifest(state)
	manifest.Migrations[0].Legacy.After = manifest.Migrations[0].Legacy.Before
	return manifest
}

func modifiedCommandManifest(source CommandMigrationManifest) CommandMigrationManifest {
	modified := source
	modified.Migrations = append([]CommandMigration(nil), source.Migrations...)
	modified.Migrations[0].Reason = "Modified reason."
	return modified
}

func commandMigrationSnapshot(after, removeUnrelated bool) Snapshot {
	oldFlags := []Flag{{Name: "old-id", Type: "string", Required: true}, {Name: "keep", Type: "string"}}
	groupFlags := []Flag{{Name: "thread", Type: "bool", NoOpt: "true"}}
	commands := []Command{
		testCommand("dws"),
		testCommand("dws chat group create", groupFlags...),
		testCommand("dws chat message old", oldFlags...),
	}
	if after {
		commands[1].LocalFlags[0].Hidden = true
		commands[2].Hidden = true
		if removeUnrelated {
			commands[2].LocalFlags = commands[2].LocalFlags[:1]
		}
		topicCreate := testCommand("dws chat topic create")
		topicCreate.BoolConstParams = map[string]bool{"convThreadEnabled": true}
		commands = append(commands,
			topicCreate,
			testCommand("dws chat topic new", Flag{Name: "new-id", Type: "string", Required: true}),
		)
	}
	return testSnapshot(commands...)
}

func retargetedCommandMigrationSnapshot() Snapshot {
	snapshot := commandMigrationSnapshot(false, false)
	snapshot.Commands[1].LocalFlags[0].Hidden = true
	snapshot.Commands[2].Hidden = true
	threadCreate := testCommand("dws chat thread create-group")
	threadCreate.BoolConstParams = map[string]bool{"convThreadEnabled": true}
	snapshot.Commands = append(snapshot.Commands,
		threadCreate,
		testCommand("dws chat thread new"),
	)
	return snapshot
}

func withCommandBoolConstParams(snapshot Snapshot, path string, values map[string]bool) Snapshot {
	cloned := snapshot
	cloned.Commands = append([]Command(nil), snapshot.Commands...)
	for index := range cloned.Commands {
		if cloned.Commands[index].Path == path {
			cloned.Commands[index].BoolConstParams = values
			break
		}
	}
	return cloned
}
