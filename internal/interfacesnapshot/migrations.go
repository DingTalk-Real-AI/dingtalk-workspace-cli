// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const FlagMigrationManifestVersion = 1

const (
	FlagMigrationPending  = "pending"
	FlagMigrationConsumed = "consumed"
)

type FlagMigrationManifest struct {
	Version    int             `json:"version"`
	Migrations []FlagMigration `json:"migrations"`
}

type FlagMigration struct {
	Command   string            `json:"command"`
	Legacy    FlagMigrationSide `json:"legacy"`
	Canonical FlagMigrationSide `json:"canonical"`
	State     string            `json:"state"`
	Reason    string            `json:"reason"`
}

type FlagMigrationSide struct {
	Name   string             `json:"name"`
	Before FlagMigrationState `json:"before"`
	After  FlagMigrationState `json:"after"`
}

type FlagMigrationState struct {
	Present   bool   `json:"present"`
	Type      string `json:"type,omitempty"`
	Required  bool   `json:"required,omitempty"`
	Hidden    bool   `json:"hidden,omitempty"`
	Shorthand string `json:"shorthand,omitempty"`
	NoOpt     string `json:"no_opt,omitempty"`
	Scope     string `json:"scope,omitempty"`
	AliasOf   string `json:"alias_of,omitempty"`
}

func ReadFlagMigrationManifest(r io.Reader) (FlagMigrationManifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return FlagMigrationManifest{}, fmt.Errorf("read flag migration manifest: %w", err)
	}
	var manifest FlagMigrationManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return FlagMigrationManifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return FlagMigrationManifest{}, fmt.Errorf("flag migration manifest contains trailing multiple JSON values")
		}
		return FlagMigrationManifest{}, fmt.Errorf("read trailing flag migration manifest data: %w", err)
	}
	if err := validateFlagMigrationJSONSchema(data); err != nil {
		return FlagMigrationManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return FlagMigrationManifest{}, err
	}
	return manifest, nil
}

func validateFlagMigrationJSONSchema(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateMigrationJSONValue(decoder, "$", reflect.TypeOf(FlagMigrationManifest{})); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("flag migration manifest contains trailing JSON value %v", token)
		}
		return fmt.Errorf("read trailing flag migration manifest data: %w", err)
	}
	return nil
}

func validateMigrationJSONValue(decoder *json.Decoder, path string, schema reflect.Type) error {
	return validateLabeledMigrationJSONValue(decoder, path, schema, "flag")
}

func validateLabeledMigrationJSONValue(decoder *json.Decoder, path string, schema reflect.Type, label string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read %s migration manifest value at %s: %w", label, path, err)
	}
	for schema.Kind() == reflect.Pointer {
		if token == nil {
			return migrationJSONTypeError(path, schema.Elem(), token, label)
		}
		schema = schema.Elem()
	}

	switch schema.Kind() {
	case reflect.Struct:
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
			return migrationJSONTypeError(path, schema, token, label)
		}
		fields := migrationJSONFields(schema)
		seen := make(map[string]bool, len(fields))
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return fmt.Errorf("read %s migration manifest field at %s: %w", label, path, keyErr)
			}
			// encoding/json guarantees object member names are string tokens.
			key := keyToken.(string)
			fieldSchema, exists := fields[key]
			if !exists {
				for canonical := range fields {
					if strings.EqualFold(key, canonical) {
						return fmt.Errorf(
							"%s migration manifest contains non-canonical field %q at %s (want %q)",
							label,
							key,
							path,
							canonical,
						)
					}
				}
				return fmt.Errorf("%s migration manifest contains unknown field %q at %s", label, key, path)
			}
			if seen[key] {
				return fmt.Errorf("%s migration manifest contains duplicate field %q at %s", label, key, path)
			}
			seen[key] = true
			if err := validateLabeledMigrationJSONValue(decoder, path+"."+key, fieldSchema, label); err != nil {
				return err
			}
		}
		if _, closeErr := decoder.Token(); closeErr != nil {
			return fmt.Errorf("close %s migration manifest object at %s: %w", label, path, closeErr)
		}
		return nil
	case reflect.Slice:
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
			return migrationJSONTypeError(path, schema, token, label)
		}
		for index := 0; decoder.More(); index++ {
			if err := validateLabeledMigrationJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index), schema.Elem(), label); err != nil {
				return err
			}
		}
		if _, closeErr := decoder.Token(); closeErr != nil {
			return fmt.Errorf("close %s migration manifest array at %s: %w", label, path, closeErr)
		}
		return nil
	case reflect.String:
		if _, ok := token.(string); !ok {
			return migrationJSONTypeError(path, schema, token, label)
		}
		return nil
	case reflect.Int:
		if _, ok := token.(json.Number); !ok {
			return migrationJSONTypeError(path, schema, token, label)
		}
		return nil
	case reflect.Bool:
		if _, ok := token.(bool); !ok {
			return migrationJSONTypeError(path, schema, token, label)
		}
		return nil
	default:
		return fmt.Errorf("%s migration manifest value at %s has unsupported Go schema type %s", label, path, schema)
	}
}

func migrationJSONFields(schema reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, schema.NumField())
	for index := 0; index < schema.NumField(); index++ {
		field := schema.Field(index)
		name := field.Tag.Get("json")
		if comma := strings.IndexByte(name, ','); comma >= 0 {
			name = name[:comma]
		}
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func migrationJSONTypeError(path string, want reflect.Type, token json.Token, label string) error {
	return fmt.Errorf(
		"%s migration manifest value at %s must be %s, got %s",
		label,
		path,
		migrationJSONKindDescription(want),
		migrationJSONTokenDescription(token),
	)
}

func migrationJSONKindDescription(schema reflect.Type) string {
	switch schema.Kind() {
	case reflect.Struct:
		return "an object"
	case reflect.Slice:
		return "an array"
	case reflect.String:
		return "a string"
	case reflect.Int:
		return "a number"
	case reflect.Bool:
		return "a boolean"
	default:
		return "the declared JSON type"
	}
}

func migrationJSONTokenDescription(token json.Token) string {
	if token == nil {
		return "null"
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			return "object"
		case '[':
			return "array"
		default:
			return fmt.Sprintf("delimiter %q", delimiter)
		}
	}
	switch token.(type) {
	case string:
		return "string"
	case json.Number, float64:
		return "number"
	case bool:
		return "boolean"
	default:
		return fmt.Sprintf("%T", token)
	}
}

func (m FlagMigrationManifest) Validate() error {
	if m.Version != FlagMigrationManifestVersion {
		return fmt.Errorf(
			"unsupported flag migration manifest version %d (want %d)",
			m.Version,
			FlagMigrationManifestVersion,
		)
	}
	if m.Migrations == nil {
		return fmt.Errorf("flag migration manifest migrations must be an array")
	}
	seen := make(map[string]bool, len(m.Migrations))
	legacyTargets := make(map[string]string, len(m.Migrations))
	for index, migration := range m.Migrations {
		if err := migration.validate(); err != nil {
			return fmt.Errorf("flag migration %d: %w", index, err)
		}
		key := migration.key()
		if seen[key] {
			return fmt.Errorf("flag migration %d duplicates %s", index, key)
		}
		seen[key] = true
		legacyKey := migration.Command + "\x00" + migration.Legacy.Name
		if canonical, exists := legacyTargets[legacyKey]; exists && canonical != migration.Canonical.Name {
			return fmt.Errorf(
				"flag migration %d maps %s --%s to both --%s and --%s",
				index,
				migration.Command,
				migration.Legacy.Name,
				canonical,
				migration.Canonical.Name,
			)
		}
		legacyTargets[legacyKey] = migration.Canonical.Name
	}
	return nil
}

func (m FlagMigration) validate() error {
	if !isExactCommandPath(m.Command) {
		return fmt.Errorf("command must be an exact command path rooted at dws: %q", m.Command)
	}
	if !isExactFlagName(m.Legacy.Name) {
		return fmt.Errorf("legacy name must be an exact legacy flag: %q", m.Legacy.Name)
	}
	if !isExactFlagName(m.Canonical.Name) {
		return fmt.Errorf("canonical name must be an exact canonical flag: %q", m.Canonical.Name)
	}
	if m.Legacy.Name == m.Canonical.Name {
		return fmt.Errorf("legacy and canonical flags must differ: --%s", m.Legacy.Name)
	}
	if strings.TrimSpace(m.Reason) == "" {
		return fmt.Errorf("migration must include a non-empty reason")
	}
	if m.Reason != strings.TrimSpace(m.Reason) {
		return fmt.Errorf("migration reason must be trimmed")
	}
	if m.State != FlagMigrationPending && m.State != FlagMigrationConsumed {
		return fmt.Errorf("invalid state %q", m.State)
	}
	if err := m.Legacy.Before.validate("legacy before"); err != nil {
		return err
	}
	if err := m.Legacy.After.validate("legacy after"); err != nil {
		return err
	}
	if err := m.Canonical.Before.validate("canonical before"); err != nil {
		return err
	}
	if err := m.Canonical.After.validate("canonical after"); err != nil {
		return err
	}
	if !m.Legacy.Before.Present || !m.Legacy.After.Present {
		return fmt.Errorf("legacy flag must remain present before and after migration")
	}
	if m.Legacy.Before.Hidden || !m.Legacy.After.Hidden {
		return fmt.Errorf("legacy flag must migrate exactly from visible to hidden")
	}
	if !m.Canonical.After.Present {
		return fmt.Errorf("canonical flag must be present after migration")
	}
	if m.Canonical.After.Hidden {
		return fmt.Errorf("canonical flag must remain visible")
	}
	// Requiredness belongs to the one logical parameter. The hidden legacy
	// spelling must not remain independently required, while the canonical
	// spelling inherits the exact before-state contract. An already-visible
	// canonical flag cannot change requiredness; an existing hidden canonical
	// placeholder may inherit it when promoted to the public spelling.
	if m.Legacy.After.Required {
		return fmt.Errorf("legacy compatibility alias must not remain independently required after migration")
	}
	if m.Legacy.Before.Required != m.Canonical.After.Required {
		return fmt.Errorf(
			"flag requiredness must be preserved from legacy before to canonical after",
		)
	}
	if m.Canonical.Before.Present &&
		!m.Canonical.Before.Hidden &&
		m.Canonical.Before.Required != m.Canonical.After.Required {
		return fmt.Errorf("canonical flag requiredness must remain unchanged when already present")
	}
	if m.Legacy.After.AliasOf != m.Canonical.Name {
		return fmt.Errorf(
			"legacy flag after state must declare alias_of %q",
			m.Canonical.Name,
		)
	}
	if m.Legacy.Before.AliasOf != "" || m.Canonical.Before.AliasOf != "" || m.Canonical.After.AliasOf != "" {
		return fmt.Errorf("alias_of is only valid on the legacy after state")
	}
	if m.Canonical.Before.Present {
		if m.Canonical.Before.Type != m.Canonical.After.Type {
			return fmt.Errorf("canonical flag type must remain unchanged")
		}
		if m.Canonical.Before.Shorthand != m.Canonical.After.Shorthand {
			return fmt.Errorf("canonical flag shorthand must remain unchanged")
		}
		if m.Canonical.Before.NoOpt != m.Canonical.After.NoOpt {
			return fmt.Errorf("canonical flag no_opt must remain unchanged")
		}
		if m.Canonical.Before.Scope != m.Canonical.After.Scope {
			return fmt.Errorf("canonical flag scope must remain unchanged")
		}
	}
	if m.Legacy.Before.Type != m.Legacy.After.Type || m.Legacy.After.Type != m.Canonical.After.Type {
		return fmt.Errorf("legacy and canonical flag types must match exactly")
	}
	if m.Legacy.Before.Shorthand != m.Legacy.After.Shorthand {
		return fmt.Errorf("legacy flag shorthand must remain unchanged")
	}
	if m.Legacy.Before.NoOpt != m.Legacy.After.NoOpt {
		return fmt.Errorf("legacy flag no_opt must remain unchanged")
	}
	if m.Legacy.Before.Scope != m.Legacy.After.Scope {
		return fmt.Errorf("legacy flag scope must remain unchanged")
	}
	return nil
}

func (s FlagMigrationState) validate(label string) error {
	if !s.Present {
		if s.Type != "" || s.Required || s.Hidden || s.Shorthand != "" || s.NoOpt != "" || s.Scope != "" || s.AliasOf != "" {
			return fmt.Errorf("%s absent state must not declare flag attributes", label)
		}
		return nil
	}
	if strings.TrimSpace(s.Type) == "" {
		return fmt.Errorf("%s present state requires type", label)
	}
	if s.Scope != "local" && s.Scope != "inherited" {
		return fmt.Errorf("%s present state has invalid scope %q", label, s.Scope)
	}
	return nil
}

func (m FlagMigration) key() string {
	return m.Command + "\x00" + m.Legacy.Name + "\x00" + m.Canonical.Name
}

func isExactCommandPath(path string) bool {
	return path != "" &&
		path == strings.TrimSpace(path) &&
		strings.Join(strings.Fields(path), " ") == path &&
		(path == "dws" || strings.HasPrefix(path, "dws ")) &&
		!strings.ContainsAny(path, "*?[]{}")
}

func isExactFlagName(name string) bool {
	return name != "" &&
		name == strings.TrimSpace(name) &&
		!strings.HasPrefix(name, "-") &&
		!strings.ContainsAny(name, "*?[]{} /\t\r\n")
}

// CompareAllWithFlagMigrations applies the ordinary compatibility policy and
// then consumes only exact, merge-base-owned flag migrations. Candidate-owned
// records participate in the lifecycle check, but never authorize their own
// interface change.
func CompareAllWithFlagMigrations(
	current Snapshot,
	references map[string]Snapshot,
	authority FlagMigrationManifest,
	candidate FlagMigrationManifest,
) (Report, error) {
	authorizations, err := AuthorizeFlagMigrations(current, references, authority, candidate)
	if err != nil {
		return Report{}, err
	}
	report := CompareAll(current, references)
	if len(authorizations) == 0 {
		return report, nil
	}

	for index := range report.Comparisons {
		comparison := &report.Comparisons[index]
		reference := references[comparison.Reference]
		filtered := comparison.Blocking[:0]
		for _, change := range comparison.Blocking {
			if flagMigrationAuthorizesChange(current, reference, change, authorizations) {
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

// AuthorizeFlagMigrations validates both snapshots and manifests, enforces the
// base-owned migration lifecycle, and returns only approvals that may authorize
// the current exact interface transition. A non-empty lifecycle requires both a
// main/merge-base authority reference and a stable reference. Candidate-added
// records never appear in the returned authorization set.
func AuthorizeFlagMigrations(
	current Snapshot,
	references map[string]Snapshot,
	authority FlagMigrationManifest,
	candidate FlagMigrationManifest,
) ([]FlagMigration, error) {
	if err := current.Validate(); err != nil {
		return nil, fmt.Errorf("validate current interface snapshot: %w", err)
	}
	for label, snapshot := range references {
		if err := snapshot.Validate(); err != nil {
			return nil, fmt.Errorf("validate %s interface snapshot: %w", label, err)
		}
	}
	if err := authority.Validate(); err != nil {
		return nil, fmt.Errorf("validate approved flag migrations: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("validate candidate flag migrations: %w", err)
	}
	return evaluateFlagMigrationLifecycle(current, references, authority, candidate)
}

type flagMigrationPhase string

const (
	flagMigrationBefore  flagMigrationPhase = "before"
	flagMigrationAfter   flagMigrationPhase = "after"
	flagMigrationPartial flagMigrationPhase = "partial"
)

func evaluateFlagMigrationLifecycle(
	current Snapshot,
	references map[string]Snapshot,
	authority FlagMigrationManifest,
	candidate FlagMigrationManifest,
) ([]FlagMigration, error) {
	if len(authority.Migrations) == 0 && len(candidate.Migrations) == 0 {
		return nil, nil
	}
	if _, ok := references["stable"]; !ok {
		return nil, fmt.Errorf("flag migration lifecycle requires a stable reference")
	}
	mergeBase, label, ok := flagMigrationAuthoritySnapshot(references)
	if !ok {
		return nil, fmt.Errorf("flag migration lifecycle requires a main or merge-base reference")
	}

	authorityByKey := flagMigrationIndex(authority)
	candidateByKey := flagMigrationIndex(candidate)
	authorizations := make([]FlagMigration, 0, len(authority.Migrations))

	for _, approved := range authority.Migrations {
		basePhase := matchFlagMigrationPhase(mergeBase, approved)
		wantBasePhase := flagMigrationBefore
		if approved.State == FlagMigrationConsumed {
			wantBasePhase = flagMigrationAfter
		}
		if basePhase != wantBasePhase {
			return nil, fmt.Errorf(
				"approved flag migration %s is %s in %s, want exact %s state for %s",
				approved.displayKey(),
				basePhase,
				label,
				wantBasePhase,
				approved.State,
			)
		}

		proposed, exists := candidateByKey[approved.key()]
		if exists && !sameFlagMigrationApproval(approved, proposed) {
			return nil, fmt.Errorf("candidate modified base-owned flag migration %s", approved.displayKey())
		}

		currentPhase := matchFlagMigrationPhase(current, approved)
		switch approved.State {
		case FlagMigrationPending:
			if !exists {
				return nil, fmt.Errorf("candidate removed pending flag migration %s", approved.displayKey())
			}
			switch currentPhase {
			case flagMigrationBefore:
				if proposed.State != FlagMigrationPending {
					return nil, fmt.Errorf("candidate falsely consumed unchanged flag migration %s", approved.displayKey())
				}
			case flagMigrationAfter:
				if proposed.State != FlagMigrationConsumed {
					return nil, fmt.Errorf("candidate completed flag migration %s without marking it consumed", approved.displayKey())
				}
				authorizations = append(authorizations, approved)
			default:
				return nil, fmt.Errorf("candidate partially applied flag migration %s", approved.displayKey())
			}
		case FlagMigrationConsumed:
			if currentPhase != flagMigrationAfter {
				return nil, fmt.Errorf("candidate drifted from consumed flag migration %s", approved.displayKey())
			}
			allReferencesAfter := true
			for _, reference := range references {
				if matchFlagMigrationPhase(reference, approved) != flagMigrationAfter {
					allReferencesAfter = false
					break
				}
			}
			if allReferencesAfter {
				if exists && proposed.State != FlagMigrationConsumed {
					return nil, fmt.Errorf("candidate changed consumed flag migration %s back to pending", approved.displayKey())
				}
				continue
			}
			if !exists {
				return nil, fmt.Errorf("candidate removed consumed flag migration %s before every reference reached the after state", approved.displayKey())
			}
			if proposed.State != FlagMigrationConsumed {
				return nil, fmt.Errorf("candidate changed consumed flag migration %s back to pending", approved.displayKey())
			}
			authorizations = append(authorizations, approved)
		}
	}

	for _, proposed := range candidate.Migrations {
		if _, exists := authorityByKey[proposed.key()]; exists {
			continue
		}
		if proposed.State != FlagMigrationPending {
			return nil, fmt.Errorf("candidate-added flag migration %s must start pending", proposed.displayKey())
		}
		if matchFlagMigrationPhase(mergeBase, proposed) != flagMigrationBefore {
			return nil, fmt.Errorf("candidate-added flag migration %s does not match the merge-base before state", proposed.displayKey())
		}
		if matchFlagMigrationPhase(current, proposed) != flagMigrationBefore {
			return nil, fmt.Errorf("candidate-added flag migration %s cannot authorize its own interface change", proposed.displayKey())
		}
	}

	return authorizations, nil
}

func flagMigrationAuthoritySnapshot(references map[string]Snapshot) (Snapshot, string, bool) {
	for _, label := range []string{"merge-base", "main"} {
		if snapshot, ok := references[label]; ok {
			return snapshot, label, true
		}
	}
	return Snapshot{}, "", false
}

func flagMigrationIndex(manifest FlagMigrationManifest) map[string]FlagMigration {
	index := make(map[string]FlagMigration, len(manifest.Migrations))
	for _, migration := range manifest.Migrations {
		index[migration.key()] = migration
	}
	return index
}

func sameFlagMigrationApproval(left, right FlagMigration) bool {
	left.State = ""
	right.State = ""
	return reflect.DeepEqual(left, right)
}

func matchFlagMigrationPhase(snapshot Snapshot, migration FlagMigration) flagMigrationPhase {
	command, exists := commandIndex(snapshot)[migration.Command]
	if !exists {
		return flagMigrationPartial
	}
	legacy := flagMigrationStateForCommand(command, migration.Legacy.Name)
	canonical := flagMigrationStateForCommand(command, migration.Canonical.Name)
	if legacy == migration.Legacy.Before && canonical == migration.Canonical.Before {
		return flagMigrationBefore
	}
	if legacy == migration.Legacy.After && canonical == migration.Canonical.After {
		return flagMigrationAfter
	}
	return flagMigrationPartial
}

func flagMigrationStateForCommand(command Command, name string) FlagMigrationState {
	for _, flag := range command.LocalFlags {
		if flag.Name == name {
			return flagMigrationState(flag, "local")
		}
	}
	for _, flag := range command.InheritedFlags {
		if flag.Name == name {
			return flagMigrationState(flag, "inherited")
		}
	}
	return FlagMigrationState{}
}

func flagMigrationState(flag Flag, scope string) FlagMigrationState {
	return FlagMigrationState{
		Present:   true,
		Type:      flag.Type,
		Required:  flag.Required,
		Hidden:    flag.Hidden,
		Shorthand: flag.Shorthand,
		NoOpt:     flag.NoOpt,
		Scope:     scope,
		AliasOf:   flag.AliasOf,
	}
}

func flagMigrationAuthorizesChange(
	current Snapshot,
	reference Snapshot,
	change Change,
	authorizations []FlagMigration,
) bool {
	canonicalPath := acceptedPathIndex(reference)[change.Path]
	if canonicalPath == "" {
		canonicalPath = change.Path
	}
	for _, migration := range authorizations {
		if canonicalPath != migration.Command ||
			matchFlagMigrationPhase(reference, migration) != flagMigrationBefore ||
			matchFlagMigrationPhase(current, migration) != flagMigrationAfter {
			continue
		}
		if change.Flag == migration.Legacy.Name && change.Kind == "flag_became_hidden" {
			return true
		}
		if change.Flag != migration.Canonical.Name {
			continue
		}
		if !migration.Canonical.Before.Present &&
			migration.Canonical.After.Required &&
			change.Kind == "required_flag_added" {
			return true
		}
		if migration.Canonical.Before.Present &&
			migration.Canonical.Before.Hidden &&
			!migration.Canonical.Before.Required &&
			migration.Canonical.After.Required &&
			change.Kind == "flag_became_required" {
			return true
		}
	}
	return false
}

func (m FlagMigration) displayKey() string {
	return fmt.Sprintf("%q --%s -> --%s", m.Command, m.Legacy.Name, m.Canonical.Name)
}
