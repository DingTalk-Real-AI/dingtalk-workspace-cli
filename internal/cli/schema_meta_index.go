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

package cli

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// SchemaMetaIndexVersion is the CommandMeta summary index format used by CI
// dumps (cmd_schema_catalog) and unit-test encode/decode fixtures. Production
// ResolveMeta projects from the runtime-assembled SchemaRegistry — there is
// no committed schema_meta_index.gob embed.
const SchemaMetaIndexVersion = 1

// SchemaMetaIndexSnapshot is the CommandMeta summary shape written by CI
// Catalog dumps for determinism checks. Runtime delivery does not embed or
// decode this artifact.
type SchemaMetaIndexSnapshot struct {
	Version     int                    `json:"version"`
	SourceHash  string                 `json:"source_hash"`
	SurfaceHash string                 `json:"surface_hash,omitempty"`
	Entries     []SchemaMetaIndexEntry `json:"entries"`
}

// SchemaMetaIndexEntry is one primary-path CommandMeta record. Aliases are
// expanded into the ResolveMeta lookup at decode time.
type SchemaMetaIndexEntry struct {
	CLIPath      string   `json:"cli_path"`
	Canonical    string   `json:"canonical_path"`
	Aliases      []string `json:"aliases,omitempty"`
	ProductID    string   `json:"product_id,omitempty"`
	Title        string   `json:"title,omitempty"`
	Effect       string   `json:"effect,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Confirmation string   `json:"confirmation,omitempty"`
	Idempotency  string   `json:"idempotency,omitempty"`
	AgentSummary string   `json:"agent_summary,omitempty"`
	UseWhen      []string `json:"use_when,omitempty"`
	AvoidWhen    []string `json:"avoid_when,omitempty"`
	Examples     []string `json:"examples,omitempty"`
}

// BuildSchemaMetaIndex extracts the ResolveMeta summary from a full Catalog
// snapshot. Entries are sorted by cli_path for deterministic generation.
func BuildSchemaMetaIndex(snapshot SchemaCatalogSnapshot) (SchemaMetaIndexSnapshot, error) {
	if snapshot.Version != SchemaCatalogSnapshotVersion {
		return SchemaMetaIndexSnapshot{}, fmt.Errorf("unsupported Schema Catalog snapshot version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.SourceHash) == "" {
		return SchemaMetaIndexSnapshot{}, fmt.Errorf("schema Catalog snapshot is missing source_hash")
	}
	entries := make([]SchemaMetaIndexEntry, 0, len(snapshot.Tools))
	seenCLI := make(map[string]string, len(snapshot.Tools))
	for canonical, tool := range snapshot.Tools {
		cliPath := strings.TrimSpace(schemaString(tool["cli_path"]))
		if cliPath == "" {
			return SchemaMetaIndexSnapshot{}, fmt.Errorf("tool %q is missing cli_path", canonical)
		}
		if prev, exists := seenCLI[cliPath]; exists {
			return SchemaMetaIndexSnapshot{}, fmt.Errorf("duplicate cli_path %q for %q and %q", cliPath, prev, canonical)
		}
		seenCLI[cliPath] = canonical
		toolCanonical := strings.TrimSpace(schemaString(tool["canonical_path"]))
		if toolCanonical == "" {
			toolCanonical = canonical
		}
		entries = append(entries, SchemaMetaIndexEntry{
			CLIPath:      cliPath,
			Canonical:    toolCanonical,
			Aliases:      schemaStringSlice(tool["aliases"]),
			ProductID:    schemaString(tool["product_id"]),
			Title:        schemaString(tool["title"]),
			Effect:       schemaString(tool["effect"]),
			Risk:         schemaString(tool["risk"]),
			Confirmation: schemaString(tool["confirmation"]),
			Idempotency:  schemaString(tool["idempotency"]),
			AgentSummary: schemaString(tool["agent_summary"]),
			UseWhen:      schemaStringSlice(tool["use_when"]),
			AvoidWhen:    schemaStringSlice(tool["avoid_when"]),
			Examples:     schemaStringSlice(tool["examples"]),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CLIPath < entries[j].CLIPath
	})
	return SchemaMetaIndexSnapshot{
		Version:     SchemaMetaIndexVersion,
		SourceHash:  snapshot.SourceHash,
		SurfaceHash: snapshot.SurfaceHash,
		Entries:     entries,
	}, nil
}

var (
	encodeSchemaMetaIndexFn    = encodeSchemaMetaIndexGob
	gobEncodeSchemaMetaIndex   = gobEncodeSchemaMetaIndexTo
	jsonMarshalSchemaMetaIndex = json.MarshalIndent
)

// EncodeSchemaMetaIndex marshals a CI/test SchemaMetaIndex dump (gob).
// Production ResolveMeta does not embed or load this artifact.
func EncodeSchemaMetaIndex(index SchemaMetaIndexSnapshot) ([]byte, error) {
	encoded, err := encodeSchemaMetaIndexFn(index)
	if err != nil {
		return nil, fmt.Errorf("encode schema meta index: %w", err)
	}
	return encoded, nil
}

func gobEncodeSchemaMetaIndexTo(index SchemaMetaIndexSnapshot, buf *bytes.Buffer) error {
	return gob.NewEncoder(buf).Encode(index)
}

func encodeSchemaMetaIndexGob(index SchemaMetaIndexSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	if err := gobEncodeSchemaMetaIndex(index, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeSchemaMetaIndex parses a CI/test gob meta index dump.
func DecodeSchemaMetaIndex(data []byte) (SchemaMetaIndexSnapshot, error) {
	if len(data) == 0 {
		return SchemaMetaIndexSnapshot{}, fmt.Errorf("decode schema meta index: empty payload")
	}
	var index SchemaMetaIndexSnapshot
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&index); err != nil {
		return SchemaMetaIndexSnapshot{}, fmt.Errorf("decode schema meta index: %w", err)
	}
	if err := validateSchemaMetaIndexSnapshot(index); err != nil {
		return SchemaMetaIndexSnapshot{}, err
	}
	return index, nil
}

// DecodeSchemaMetaIndexJSON parses a JSON meta index document. Kept for unit
// fixtures; runtime delivery uses gob via DecodeSchemaMetaIndex.
func DecodeSchemaMetaIndexJSON(data []byte) (SchemaMetaIndexSnapshot, error) {
	var index SchemaMetaIndexSnapshot
	if err := decodeStrictSchemaJSON(data, &index); err != nil {
		return SchemaMetaIndexSnapshot{}, fmt.Errorf("decode schema meta index: %w", err)
	}
	if err := validateSchemaMetaIndexSnapshot(index); err != nil {
		return SchemaMetaIndexSnapshot{}, err
	}
	return index, nil
}

func validateSchemaMetaIndexSnapshot(index SchemaMetaIndexSnapshot) error {
	if index.Version != SchemaMetaIndexVersion {
		return fmt.Errorf("unsupported schema meta index version %d", index.Version)
	}
	if strings.TrimSpace(index.SourceHash) == "" {
		return fmt.Errorf("schema meta index is missing source_hash")
	}
	if len(index.Entries) == 0 {
		return fmt.Errorf("schema meta index has no entries")
	}
	return nil
}

// commandMetaLookupFromIndex expands primary paths and aliases into the
// ResolveMeta O(1) map.
func commandMetaLookupFromIndex(index SchemaMetaIndexSnapshot) (map[string]CommandMeta, error) {
	lookup := make(map[string]CommandMeta, len(index.Entries)*2)
	metas := make([]CommandMeta, 0, len(index.Entries))
	for _, entry := range index.Entries {
		cliPath := strings.TrimSpace(entry.CLIPath)
		if cliPath == "" {
			return nil, fmt.Errorf("schema meta index entry is missing cli_path")
		}
		if _, exists := lookup[cliPath]; exists {
			return nil, fmt.Errorf("schema meta index duplicate cli_path %q", cliPath)
		}
		meta := CommandMeta{
			Identity: CommandIdentity{
				CLIPath:   cliPath,
				Canonical: strings.TrimSpace(entry.Canonical),
				Aliases:   append([]string(nil), entry.Aliases...),
				ProductID: entry.ProductID,
				Title:     entry.Title,
			},
			Safety: CommandSafety{
				Effect:       entry.Effect,
				Risk:         entry.Risk,
				Confirmation: entry.Confirmation,
				Idempotency:  entry.Idempotency,
			},
			Selection: CommandSelection{
				AgentSummary: entry.AgentSummary,
				UseWhen:      append([]string(nil), entry.UseWhen...),
				AvoidWhen:    append([]string(nil), entry.AvoidWhen...),
				Examples:     append([]string(nil), entry.Examples...),
			},
		}
		lookup[cliPath] = meta
		metas = append(metas, meta)
	}
	return registerCommandMetaAliases(lookup, metas), nil
}

// ValidateSchemaMetaIndexAgainstSnapshot proves the summary index matches the
// Identity / Safety / Selection projection of every ToolSpec in a Catalog
// snapshot (generation-time gate before writing the index).
func ValidateSchemaMetaIndexAgainstSnapshot(index SchemaMetaIndexSnapshot, snapshot SchemaCatalogSnapshot) error {
	if index.SourceHash != snapshot.SourceHash {
		return fmt.Errorf("schema meta index source_hash %q disagrees with catalog %q", index.SourceHash, snapshot.SourceHash)
	}
	if index.SurfaceHash != snapshot.SurfaceHash {
		return fmt.Errorf("schema meta index surface_hash %q disagrees with catalog %q", index.SurfaceHash, snapshot.SurfaceHash)
	}
	want := buildMetaByCLIPathFromSnapshotTools(snapshot.Tools)
	got, err := commandMetaLookupFromIndex(index)
	if err != nil {
		return err
	}
	if err := compareCommandMetaLookups(got, want); err != nil {
		return err
	}
	if len(index.Entries) != len(snapshot.Tools) {
		return fmt.Errorf("schema meta index entry count %d disagrees with catalog tools %d", len(index.Entries), len(snapshot.Tools))
	}
	return nil
}

// ValidateSchemaMetaIndexAgainstCatalog proves the summary index matches the
// Identity / Safety / Selection projection of every delivered ToolSpec.
func ValidateSchemaMetaIndexAgainstCatalog(index SchemaMetaIndexSnapshot, registry SchemaRegistry) error {
	want := buildMetaByCLIPathFromRegistry(registry)
	got, err := commandMetaLookupFromIndex(index)
	if err != nil {
		return err
	}
	if err := compareCommandMetaLookups(got, want); err != nil {
		return err
	}
	primaryCount := 0
	for _, product := range registry.Products {
		primaryCount += len(product.Tools)
	}
	if len(index.Entries) != primaryCount {
		return fmt.Errorf("schema meta index entry count %d disagrees with catalog tools %d", len(index.Entries), primaryCount)
	}
	return nil
}

func compareCommandMetaLookups(got, want map[string]CommandMeta) error {
	if len(got) != len(want) {
		return fmt.Errorf("schema meta index lookup size %d disagrees with catalog projection %d", len(got), len(want))
	}
	paths := make([]string, 0, len(want))
	for path := range want {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		wantMeta := want[path]
		gotMeta, ok := got[path]
		if !ok {
			return fmt.Errorf("schema meta index missing path %q", path)
		}
		if err := commandMetaEqual(gotMeta, wantMeta); err != nil {
			return fmt.Errorf("schema meta index path %q: %w", path, err)
		}
	}
	// Equal lengths + every want key present in got implies identical key sets;
	// no separate "unexpected path" pass is required.
	return nil
}

func commandMetaEqual(got, want CommandMeta) error {
	if got.Identity.CLIPath != want.Identity.CLIPath ||
		got.Identity.Canonical != want.Identity.Canonical ||
		got.Identity.ProductID != want.Identity.ProductID ||
		got.Identity.Title != want.Identity.Title ||
		!metaStringSlicesEqual(got.Identity.Aliases, want.Identity.Aliases) {
		return fmt.Errorf("identity mismatch: got %+v want %+v", got.Identity, want.Identity)
	}
	if got.Safety != want.Safety {
		return fmt.Errorf("safety mismatch: got %+v want %+v", got.Safety, want.Safety)
	}
	if got.Selection.AgentSummary != want.Selection.AgentSummary ||
		!metaStringSlicesEqual(got.Selection.UseWhen, want.Selection.UseWhen) ||
		!metaStringSlicesEqual(got.Selection.AvoidWhen, want.Selection.AvoidWhen) ||
		!metaStringSlicesEqual(got.Selection.Examples, want.Selection.Examples) {
		return fmt.Errorf("selection mismatch: got %+v want %+v", got.Selection, want.Selection)
	}
	return nil
}

func metaStringSlicesEqual(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

// EncodeSchemaMetaIndexJSON is retained for diagnostics / fixtures that need a
// human-readable projection of the same snapshot struct.
func EncodeSchemaMetaIndexJSON(index SchemaMetaIndexSnapshot) ([]byte, error) {
	encoded, err := jsonMarshalSchemaMetaIndex(index, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode schema meta index json: %w", err)
	}
	return append(encoded, '\n'), nil
}
