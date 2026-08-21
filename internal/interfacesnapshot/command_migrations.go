// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

const CommandMigrationManifestVersion = 1

const (
	CommandMigrationPending  = "pending"
	CommandMigrationConsumed = "consumed"

	CommandMigrationMove           = "command_move"
	CommandMigrationFlagExtraction = "flag_extraction"
)

// CommandMigrationManifest governs compatibility-preserving surface moves that
// cannot be represented as an in-command flag rename. The merge-base owns the
// authorization; the candidate copy is only a lifecycle receipt.
type CommandMigrationManifest struct {
	Version    int                `json:"version"`
	Migrations []CommandMigration `json:"migrations"`
}

type CommandMigration struct {
	Kind        string                 `json:"kind"`
	Legacy      CommandMigrationSide   `json:"legacy"`
	Replacement CommandMigrationSide   `json:"replacement"`
	LegacyFlag  CommandMigrationFlag   `json:"legacy_flag"`
	Schema      CommandMigrationSchema `json:"schema"`
	State       string                 `json:"state"`
	Reason      string                 `json:"reason"`
}

type CommandMigrationSide struct {
	Command string                `json:"command"`
	Before  CommandMigrationState `json:"before"`
	After   CommandMigrationState `json:"after"`
}

type CommandMigrationState struct {
	Present  bool `json:"present"`
	Runnable bool `json:"runnable,omitempty"`
	Hidden   bool `json:"hidden,omitempty"`
}

type CommandMigrationFlag struct {
	Name   string             `json:"name,omitempty"`
	Before FlagMigrationState `json:"before"`
	After  FlagMigrationState `json:"after"`
}

type CommandMigrationSchema struct {
	ProductID         string                      `json:"product_id"`
	SourceToolID      string                      `json:"source_tool_id"`
	ReplacementToolID string                      `json:"replacement_tool_id"`
	Parameters        []CommandParameterMigration `json:"parameters"`
}

type CommandParameterMigration struct {
	From                string                      `json:"from"`
	To                  string                      `json:"to,omitempty"`
	ReplacementConstant *CommandReplacementConstant `json:"replacement_constant,omitempty"`
}

type CommandReplacementConstant struct {
	Property string `json:"property"`
	Value    bool   `json:"value"`
}

func ReadCommandMigrationManifest(r io.Reader) (CommandMigrationManifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return CommandMigrationManifest{}, fmt.Errorf("read command migration manifest: %w", err)
	}
	var manifest CommandMigrationManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return CommandMigrationManifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return CommandMigrationManifest{}, fmt.Errorf("command migration manifest contains trailing multiple JSON values")
		}
		return CommandMigrationManifest{}, fmt.Errorf("read trailing command migration manifest data: %w", err)
	}
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.UseNumber()
	if err := validateLabeledMigrationJSONValue(strict, "$", reflect.TypeOf(CommandMigrationManifest{}), "command"); err != nil {
		return CommandMigrationManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return CommandMigrationManifest{}, err
	}
	return manifest, nil
}

func (m CommandMigrationManifest) Validate() error {
	if m.Version != CommandMigrationManifestVersion {
		return fmt.Errorf("unsupported command migration manifest version %d (want %d)", m.Version, CommandMigrationManifestVersion)
	}
	if m.Migrations == nil {
		return fmt.Errorf("command migration manifest migrations must be an array")
	}
	seen := make(map[string]bool, len(m.Migrations))
	for index, migration := range m.Migrations {
		if err := migration.validate(); err != nil {
			return fmt.Errorf("command migration %d: %w", index, err)
		}
		if seen[migration.key()] {
			return fmt.Errorf("command migration %d duplicates %s", index, migration.displayKey())
		}
		seen[migration.key()] = true
	}
	return nil
}

func (m CommandMigration) validate() error {
	if m.Kind != CommandMigrationMove && m.Kind != CommandMigrationFlagExtraction {
		return fmt.Errorf("invalid kind %q", m.Kind)
	}
	if !isExactCommandPath(m.Legacy.Command) || !isExactCommandPath(m.Replacement.Command) {
		return fmt.Errorf("legacy and replacement must be exact command paths rooted at dws")
	}
	if m.Legacy.Command == m.Replacement.Command {
		return fmt.Errorf("legacy and replacement command paths must differ")
	}
	if strings.TrimSpace(m.Reason) == "" || m.Reason != strings.TrimSpace(m.Reason) {
		return fmt.Errorf("migration must include a non-empty trimmed reason")
	}
	if m.State != CommandMigrationPending && m.State != CommandMigrationConsumed {
		return fmt.Errorf("invalid state %q", m.State)
	}
	for label, state := range map[string]CommandMigrationState{
		"legacy before":      m.Legacy.Before,
		"legacy after":       m.Legacy.After,
		"replacement before": m.Replacement.Before,
		"replacement after":  m.Replacement.After,
	} {
		if err := state.validate(label); err != nil {
			return err
		}
	}
	if !m.Legacy.Before.Present || !m.Legacy.Before.Runnable || m.Legacy.Before.Hidden ||
		!m.Legacy.After.Present || !m.Legacy.After.Runnable {
		return fmt.Errorf("legacy command must remain runnable and start visible")
	}
	if m.Replacement.Before.Present || !m.Replacement.After.Present || !m.Replacement.After.Runnable || m.Replacement.After.Hidden {
		return fmt.Errorf("replacement command must migrate exactly from absent to visible runnable")
	}
	if err := m.Schema.validate(m.Kind); err != nil {
		return err
	}
	switch m.Kind {
	case CommandMigrationMove:
		if strings.HasPrefix(m.Replacement.Command, m.Legacy.Command+" ") ||
			strings.HasPrefix(m.Legacy.Command, m.Replacement.Command+" ") {
			return fmt.Errorf("command_move legacy and replacement command paths must not have an ancestor relationship")
		}
		if !m.Legacy.After.Hidden {
			return fmt.Errorf("command_move legacy command must migrate exactly from visible to hidden")
		}
		if m.LegacyFlag != (CommandMigrationFlag{}) {
			return fmt.Errorf("command_move must not declare legacy_flag")
		}
		if m.Schema.SourceToolID != m.Schema.ReplacementToolID {
			return fmt.Errorf("command_move must retain one stable Schema tool identity")
		}
	case CommandMigrationFlagExtraction:
		if m.Legacy.After.Hidden || m.Legacy.Before != m.Legacy.After {
			return fmt.Errorf("flag_extraction legacy command must remain visible and unchanged")
		}
		if err := m.LegacyFlag.validate(); err != nil {
			return err
		}
		if m.Schema.SourceToolID == m.Schema.ReplacementToolID {
			return fmt.Errorf("flag_extraction requires a distinct replacement Schema tool")
		}
		if m.LegacyFlag.Before.Type != "bool" || m.LegacyFlag.After.Type != "bool" {
			return fmt.Errorf("flag_extraction legacy_flag must be an optional bool")
		}
		var extracted *CommandParameterMigration
		constantCount := 0
		for index := range m.Schema.Parameters {
			parameter := &m.Schema.Parameters[index]
			if parameter.ReplacementConstant == nil {
				continue
			}
			constantCount++
			if parameter.From == m.LegacyFlag.Name {
				extracted = parameter
			}
		}
		if constantCount != 1 {
			return fmt.Errorf("flag_extraction must declare exactly one replacement constant")
		}
		if extracted == nil {
			return fmt.Errorf("flag_extraction must map the exact extracted flag to replacement_constant")
		}
		if !extracted.ReplacementConstant.Value {
			return fmt.Errorf("flag_extraction v1 replacement_constant must be true")
		}
		wantNoOpt := strconv.FormatBool(extracted.ReplacementConstant.Value)
		if m.LegacyFlag.Before.NoOpt != wantNoOpt || m.LegacyFlag.After.NoOpt != wantNoOpt {
			return fmt.Errorf("flag_extraction legacy_flag no_opt must equal replacement constant %q", wantNoOpt)
		}
	}
	return nil
}

func (s CommandMigrationState) validate(label string) error {
	if !s.Present {
		if s.Runnable || s.Hidden {
			return fmt.Errorf("%s absent state must not declare command attributes", label)
		}
		return nil
	}
	if !s.Runnable {
		return fmt.Errorf("%s present state must be runnable", label)
	}
	return nil
}

func (f CommandMigrationFlag) validate() error {
	if !isExactFlagName(f.Name) {
		return fmt.Errorf("legacy_flag name must be an exact flag")
	}
	if err := f.Before.validate("legacy_flag before"); err != nil {
		return err
	}
	if err := f.After.validate("legacy_flag after"); err != nil {
		return err
	}
	if !f.Before.Present || !f.After.Present || f.Before.Hidden || !f.After.Hidden {
		return fmt.Errorf("legacy_flag must migrate exactly from visible to hidden while remaining present")
	}
	if f.Before.Required || f.After.Required {
		return fmt.Errorf("legacy_flag must remain optional before and after extraction")
	}
	before := f.Before
	after := f.After
	before.Hidden = false
	after.Hidden = false
	if before != after {
		return fmt.Errorf("legacy_flag may change only hidden visibility")
	}
	return nil
}

func (s CommandMigrationSchema) validate(kind string) error {
	for label, value := range map[string]string{
		"product_id":          s.ProductID,
		"source_tool_id":      s.SourceToolID,
		"replacement_tool_id": s.ReplacementToolID,
	} {
		if !isExactSchemaIdentifier(value) {
			return fmt.Errorf("schema %s must be an exact identifier", label)
		}
	}
	if s.Parameters == nil {
		return fmt.Errorf("schema parameters must be an array")
	}
	seenFrom := map[string]bool{}
	seenTo := map[string]bool{}
	seenConstantProperty := map[string]bool{}
	for index, parameter := range s.Parameters {
		if !isExactFlagName(parameter.From) {
			return fmt.Errorf("schema parameter %d from must be an exact parameter name", index)
		}
		if seenFrom[parameter.From] {
			return fmt.Errorf("schema parameter %d duplicates from %q", index, parameter.From)
		}
		seenFrom[parameter.From] = true
		if kind == CommandMigrationMove {
			if parameter.ReplacementConstant != nil {
				return fmt.Errorf("command_move schema parameter %d must not declare replacement_constant", index)
			}
			if !isExactFlagName(parameter.To) || parameter.To == parameter.From {
				return fmt.Errorf("command_move schema parameter %d requires a distinct exact to name", index)
			}
			if seenTo[parameter.To] {
				return fmt.Errorf("schema parameter %d duplicates to %q", index, parameter.To)
			}
			seenTo[parameter.To] = true
			continue
		}
		if kind != CommandMigrationFlagExtraction {
			return fmt.Errorf("invalid command migration kind %q", kind)
		}

		hasTo := parameter.To != ""
		hasConstant := parameter.ReplacementConstant != nil
		if !hasTo && !hasConstant {
			return fmt.Errorf("flag_extraction schema parameter %d requires an exact to or replacement_constant", index)
		}
		if hasTo && hasConstant {
			return fmt.Errorf("flag_extraction schema parameter %d must declare exactly one target", index)
		}
		if hasTo {
			if !isExactFlagName(parameter.To) {
				return fmt.Errorf("flag_extraction schema parameter %d to must be an exact parameter name", index)
			}
			if seenTo[parameter.To] {
				return fmt.Errorf("schema parameter %d duplicates to %q", index, parameter.To)
			}
			if seenConstantProperty[parameter.To] {
				return fmt.Errorf("schema parameter %d duplicates parameter target %q", index, parameter.To)
			}
			seenTo[parameter.To] = true
			continue
		}
		property := parameter.ReplacementConstant.Property
		if !isExactSchemaIdentifier(property) {
			return fmt.Errorf("flag_extraction schema parameter %d replacement_constant requires an exact property", index)
		}
		if seenConstantProperty[property] {
			return fmt.Errorf("schema parameter %d duplicates replacement_constant property %q", index, property)
		}
		if seenTo[property] {
			return fmt.Errorf("schema parameter %d duplicates parameter target %q", index, property)
		}
		seenConstantProperty[property] = true
	}
	return nil
}

func isExactSchemaIdentifier(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.ContainsAny(value, "*?[]{} /\\\t\r\n")
}

func (m CommandMigration) key() string {
	return strings.Join([]string{m.Kind, m.Legacy.Command, m.Replacement.Command, m.Schema.ProductID, m.Schema.SourceToolID, m.Schema.ReplacementToolID}, "\x00")
}

func (m CommandMigration) displayKey() string {
	return fmt.Sprintf("%s %q -> %q", m.Kind, m.Legacy.Command, m.Replacement.Command)
}

type commandMigrationPhase string

const (
	commandMigrationBefore  commandMigrationPhase = "before"
	commandMigrationAfter   commandMigrationPhase = "after"
	commandMigrationPartial commandMigrationPhase = "partial"
)

func AuthorizeCommandMigrations(
	current Snapshot,
	references map[string]Snapshot,
	authority CommandMigrationManifest,
	candidate CommandMigrationManifest,
) ([]CommandMigration, error) {
	if err := current.Validate(); err != nil {
		return nil, fmt.Errorf("validate current interface snapshot: %w", err)
	}
	for label, snapshot := range references {
		if err := snapshot.Validate(); err != nil {
			return nil, fmt.Errorf("validate %s interface snapshot: %w", label, err)
		}
	}
	if err := authority.Validate(); err != nil {
		return nil, fmt.Errorf("validate approved command migrations: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("validate candidate command migrations: %w", err)
	}
	type commandMoveTopologyCheck struct {
		label    string
		snapshot Snapshot
		manifest CommandMigrationManifest
	}
	topologyChecks := []commandMoveTopologyCheck{
		{label: "approved", snapshot: current, manifest: authority},
		{label: "candidate", snapshot: current, manifest: candidate},
	}
	if mergeBase, _, ok := flagMigrationAuthoritySnapshot(references); ok {
		topologyChecks = append(topologyChecks,
			commandMoveTopologyCheck{label: "approved base", snapshot: mergeBase, manifest: authority},
			commandMoveTopologyCheck{label: "candidate base", snapshot: mergeBase, manifest: candidate},
		)
	}
	for _, check := range topologyChecks {
		if err := validateCommandMoveLegacyLeaves(check.snapshot, check.manifest); err != nil {
			return nil, fmt.Errorf("validate %s command migration topology: %w", check.label, err)
		}
	}
	return evaluateCommandMigrationLifecycle(current, references, authority, candidate)
}

func validateCommandMoveLegacyLeaves(snapshot Snapshot, manifest CommandMigrationManifest) error {
	commands := commandIndex(snapshot)
	for _, migration := range manifest.Migrations {
		if migration.Kind != CommandMigrationMove {
			continue
		}
		if _, present := commands[migration.Legacy.Command]; !present {
			continue
		}
		for _, command := range snapshot.Commands {
			if strings.HasPrefix(command.Path, migration.Legacy.Command+" ") {
				return fmt.Errorf("command_move legacy command %q must be a leaf", migration.Legacy.Command)
			}
		}
	}
	return nil
}

func evaluateCommandMigrationLifecycle(
	current Snapshot,
	references map[string]Snapshot,
	authority CommandMigrationManifest,
	candidate CommandMigrationManifest,
) ([]CommandMigration, error) {
	if len(authority.Migrations) == 0 && len(candidate.Migrations) == 0 {
		return nil, nil
	}
	if _, ok := references["stable"]; !ok {
		return nil, fmt.Errorf("command migration lifecycle requires a stable reference")
	}
	mergeBase, label, ok := flagMigrationAuthoritySnapshot(references)
	if !ok {
		return nil, fmt.Errorf("command migration lifecycle requires a main or merge-base reference")
	}
	authorityByKey := commandMigrationIndex(authority)
	candidateByKey := commandMigrationIndex(candidate)
	authorizations := make([]CommandMigration, 0, len(authority.Migrations))
	for _, approved := range authority.Migrations {
		basePhase := matchCommandMigrationPhase(mergeBase, approved)
		wantBase := commandMigrationBefore
		if approved.State == CommandMigrationConsumed {
			wantBase = commandMigrationAfter
		}
		if basePhase != wantBase {
			return nil, fmt.Errorf("approved command migration %s is %s in %s, want exact %s state for %s", approved.displayKey(), basePhase, label, wantBase, approved.State)
		}
		proposed, exists := candidateByKey[approved.key()]
		if exists && !sameCommandMigrationApproval(approved, proposed) {
			return nil, fmt.Errorf("candidate modified base-owned command migration %s", approved.displayKey())
		}
		currentPhase := matchCommandMigrationPhase(current, approved)
		switch approved.State {
		case CommandMigrationPending:
			if !exists {
				return nil, fmt.Errorf("candidate removed pending command migration %s", approved.displayKey())
			}
			switch currentPhase {
			case commandMigrationBefore:
				if proposed.State != CommandMigrationPending {
					return nil, fmt.Errorf("candidate falsely consumed unchanged command migration %s", approved.displayKey())
				}
			case commandMigrationAfter:
				if proposed.State != CommandMigrationConsumed {
					return nil, fmt.Errorf("candidate completed command migration %s without marking it consumed", approved.displayKey())
				}
				authorizations = append(authorizations, approved)
			default:
				return nil, fmt.Errorf("candidate partially applied command migration %s", approved.displayKey())
			}
		case CommandMigrationConsumed:
			if currentPhase != commandMigrationAfter {
				return nil, fmt.Errorf("candidate drifted from consumed command migration %s", approved.displayKey())
			}
			allAfter := true
			for _, reference := range references {
				if matchCommandMigrationPhase(reference, approved) != commandMigrationAfter {
					allAfter = false
					break
				}
			}
			if allAfter {
				if exists && proposed.State != CommandMigrationConsumed {
					return nil, fmt.Errorf("candidate changed consumed command migration %s back to pending", approved.displayKey())
				}
				continue
			}
			if !exists || proposed.State != CommandMigrationConsumed {
				return nil, fmt.Errorf("candidate must retain consumed command migration %s until every reference reaches the after state", approved.displayKey())
			}
			authorizations = append(authorizations, approved)
		}
	}
	for _, proposed := range candidate.Migrations {
		if _, exists := authorityByKey[proposed.key()]; exists {
			continue
		}
		if proposed.State != CommandMigrationPending {
			return nil, fmt.Errorf("candidate-added command migration %s must start pending", proposed.displayKey())
		}
		if matchCommandMigrationPhase(mergeBase, proposed) != commandMigrationBefore {
			return nil, fmt.Errorf("candidate-added command migration %s does not match the merge-base before state", proposed.displayKey())
		}
		if matchCommandMigrationPhase(current, proposed) != commandMigrationBefore {
			return nil, fmt.Errorf("candidate-added command migration %s cannot authorize its own interface change", proposed.displayKey())
		}
	}
	return authorizations, nil
}

func commandMigrationIndex(manifest CommandMigrationManifest) map[string]CommandMigration {
	index := make(map[string]CommandMigration, len(manifest.Migrations))
	for _, migration := range manifest.Migrations {
		index[migration.key()] = migration
	}
	return index
}

func sameCommandMigrationApproval(left, right CommandMigration) bool {
	left.State = ""
	right.State = ""
	return reflect.DeepEqual(left, right)
}

func matchCommandMigrationPhase(snapshot Snapshot, migration CommandMigration) commandMigrationPhase {
	commands := commandIndex(snapshot)
	legacy := commandMigrationStateForCommand(commands, migration.Legacy.Command)
	replacement := commandMigrationStateForCommand(commands, migration.Replacement.Command)
	before := legacy == migration.Legacy.Before && replacement == migration.Replacement.Before
	after := legacy == migration.Legacy.After && replacement == migration.Replacement.After
	if migration.Kind == CommandMigrationFlagExtraction {
		command, exists := commands[migration.Legacy.Command]
		flagState := FlagMigrationState{}
		if exists {
			flagState = flagMigrationStateForCommand(command, migration.LegacyFlag.Name)
		}
		before = before && flagState == migration.LegacyFlag.Before
		after = after && flagState == migration.LegacyFlag.After
		if replacement, replacementExists := commands[migration.Replacement.Command]; replacementExists {
			after = after && commandMigrationReplacementConstantsMatch(replacement, migration)
		} else {
			after = false
		}
	}
	if before {
		return commandMigrationBefore
	}
	if after {
		return commandMigrationAfter
	}
	return commandMigrationPartial
}

func commandMigrationReplacementConstantsMatch(command Command, migration CommandMigration) bool {
	expected := make(map[string]bool)
	for _, parameter := range migration.Schema.Parameters {
		if parameter.ReplacementConstant == nil {
			continue
		}
		expected[parameter.ReplacementConstant.Property] = parameter.ReplacementConstant.Value
	}
	return reflect.DeepEqual(command.BoolConstParams, expected)
}

func commandMigrationStateForCommand(commands map[string]Command, path string) CommandMigrationState {
	command, exists := commands[path]
	if !exists {
		return CommandMigrationState{}
	}
	return CommandMigrationState{Present: true, Runnable: command.Runnable, Hidden: command.Hidden}
}

// CompareAllWithInterfaceMigrations applies both migration families to one
// ordinary report, so the two ledgers cannot mask each other's unrelated
// findings.
func CompareAllWithInterfaceMigrations(
	current Snapshot,
	references map[string]Snapshot,
	flagAuthority FlagMigrationManifest,
	flagCandidate FlagMigrationManifest,
	commandAuthority CommandMigrationManifest,
	commandCandidate CommandMigrationManifest,
) (Report, error) {
	flagAuthorizations, err := AuthorizeFlagMigrations(current, references, flagAuthority, flagCandidate)
	if err != nil {
		return Report{}, err
	}
	commandAuthorizations, err := AuthorizeCommandMigrations(current, references, commandAuthority, commandCandidate)
	if err != nil {
		return Report{}, err
	}
	report := CompareAll(current, references)
	for index := range report.Comparisons {
		comparison := &report.Comparisons[index]
		reference := references[comparison.Reference]
		filtered := comparison.Blocking[:0]
		for _, change := range comparison.Blocking {
			if flagMigrationAuthorizesChange(current, reference, change, flagAuthorizations) ||
				commandMigrationAuthorizesChange(current, reference, change, commandAuthorizations) {
				continue
			}
			filtered = append(filtered, change)
		}
		comparison.Blocking = filtered
		comparison.Compatible = len(filtered) == 0
	}
	report.Compatible = true
	for _, comparison := range report.Comparisons {
		if !comparison.Compatible {
			report.Compatible = false
			break
		}
	}
	return report, nil
}

func commandMigrationAuthorizesChange(current, reference Snapshot, change Change, authorizations []CommandMigration) bool {
	canonicalPath := acceptedPathIndex(reference)[change.Path]
	if canonicalPath == "" {
		canonicalPath = change.Path
	}
	for _, migration := range authorizations {
		if canonicalPath != migration.Legacy.Command ||
			matchCommandMigrationPhase(reference, migration) != commandMigrationBefore ||
			matchCommandMigrationPhase(current, migration) != commandMigrationAfter {
			continue
		}
		switch migration.Kind {
		case CommandMigrationMove:
			if change.Kind == "command_became_hidden" && change.Flag == "" {
				return true
			}
		case CommandMigrationFlagExtraction:
			if change.Kind == "flag_became_hidden" && change.Flag == migration.LegacyFlag.Name {
				return true
			}
		}
	}
	return false
}
