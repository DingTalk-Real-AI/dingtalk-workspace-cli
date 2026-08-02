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

// command_meta.go provides the unified metadata consumption API. All runtime
// consumers (help, schema, agent selection, skill generation) call ResolveMeta
// to get a CommandMeta struct — one function, one struct, no need to know which
// of the 6 generation layers a field comes from.
//
// This is the "simple consumption" half of the generation/consumption split:
//   - Generation (gen.go + internal/generator/): 6 inputs → catalog snapshot.
//   - Consumption (this file): catalog snapshot → ResolveMeta → CommandMeta.

package cli

import (
	"sort"
	"strings"
	"sync"
)

// CommandMeta is the complete runtime metadata view for a single command.
// Consumers read this struct; they never touch the raw catalog maps.
type CommandMeta struct {
	Identity  CommandIdentity
	Safety    CommandSafety
	Selection CommandSelection
}

// CommandIdentity is the stable identity of a command.
type CommandIdentity struct {
	CLIPath   string   // "dev app delete"
	Canonical string   // "dev.delete_dev_app"
	Aliases   []string // ["search", ...]
	ProductID string   // "devapp"
	Title     string   // one-line description
}

// CommandSelection is the agent-facing selection metadata.
type CommandSelection struct {
	AgentSummary string
	UseWhen      []string
	AvoidWhen    []string
	Examples     []string
}

var (
	metaByCLIPathOnce sync.Once
	metaByCLIPath     map[string]CommandMeta
)

// initMetaByCLIPath builds the cli_path → CommandMeta lookup from the embedded
// catalog. Runs once (sync.Once); the catalog is already decoded at package init.
func initMetaByCLIPath() {
	metaByCLIPath = buildMetaByCLIPath(embeddedSchemaCatalog())
}

// buildMetaByCLIPath constructs the lookup from a loaded catalog snapshot.
// Split from initMetaByCLIPath so malformed-snapshot guards stay testable.
func buildMetaByCLIPath(loaded loadedSchemaCatalog) map[string]CommandMeta {
	lookup := make(map[string]CommandMeta)
	if loaded.Snapshot.Tools == nil {
		return lookup
	}
	metas := make([]CommandMeta, 0, len(loaded.Snapshot.Tools))
	for _, tool := range loaded.Snapshot.Tools {
		cliPath := schemaString(tool["cli_path"])
		if cliPath == "" {
			continue
		}
		meta := CommandMeta{
			Identity: CommandIdentity{
				CLIPath:   cliPath,
				Canonical: schemaString(tool["canonical_path"]),
				Aliases:   schemaStringSlice(tool["aliases"]),
				ProductID: schemaString(tool["product_id"]),
				Title:     schemaString(tool["title"]),
			},
			Safety: CommandSafety{
				Effect:       schemaString(tool["effect"]),
				Risk:         schemaString(tool["risk"]),
				Confirmation: schemaString(tool["confirmation"]),
				Idempotency:  schemaString(tool["idempotency"]),
			},
			Selection: CommandSelection{
				AgentSummary: schemaString(tool["agent_summary"]),
				UseWhen:      schemaStringSlice(tool["use_when"]),
				AvoidWhen:    schemaStringSlice(tool["avoid_when"]),
				Examples:     schemaStringSlice(tool["examples"]),
			},
		}
		lookup[cliPath] = meta
		metas = append(metas, meta)
	}
	// Register compat alias paths (e.g. "report list") against the same
	// metadata in a second pass, sorted by primary cli_path: primary paths
	// always win (registered above, aliases only fill vacancies), and an
	// alias-vs-alias collision resolves deterministically to the owner with
	// the lexicographically smallest primary path — Snapshot.Tools is a map,
	// so relying on iteration order would make the winner vary per process.
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Identity.CLIPath < metas[j].Identity.CLIPath
	})
	for _, meta := range metas {
		for _, alias := range meta.Identity.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == meta.Identity.CLIPath {
				continue
			}
			if _, exists := lookup[alias]; !exists {
				lookup[alias] = meta
			}
		}
	}
	return lookup
}

// ResolveMeta returns the complete metadata for a command identified by its CLI
// path (e.g. "dev app delete") or one of its compat aliases (e.g. "report list"
// for "report inbox list"). Returns ok=false for commands not in the embedded
// catalog (utility commands, hidden commands, shortcuts).
func ResolveMeta(cliPath string) (CommandMeta, bool) {
	metaByCLIPathOnce.Do(initMetaByCLIPath)
	m, ok := metaByCLIPath[strings.TrimSpace(cliPath)]
	return m, ok
}
