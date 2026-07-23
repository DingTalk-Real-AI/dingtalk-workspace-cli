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
	metaByCLIPath = make(map[string]CommandMeta)
	loaded := embeddedSchemaCatalog()
	if loaded.Snapshot.Tools == nil {
		return
	}
	for _, tool := range loaded.Snapshot.Tools {
		cliPath := catalogStringVal(tool, "cli_path")
		if cliPath == "" {
			continue
		}
		meta := CommandMeta{
			Identity: CommandIdentity{
				CLIPath:   cliPath,
				Canonical: catalogStringVal(tool, "canonical_path"),
				ProductID: catalogStringVal(tool, "product_id"),
				Title:     catalogStringVal(tool, "title"),
			},
			Safety: CommandSafety{
				Effect:       catalogStringVal(tool, "effect"),
				Risk:         catalogStringVal(tool, "risk"),
				Confirmation: catalogStringVal(tool, "confirmation"),
				Idempotency:  catalogStringVal(tool, "idempotency"),
			},
			Selection: CommandSelection{
				AgentSummary: catalogStringVal(tool, "agent_summary"),
				UseWhen:      catalogStringSliceVal(tool, "use_when"),
				AvoidWhen:    catalogStringSliceVal(tool, "avoid_when"),
				Examples:     catalogStringSliceVal(tool, "examples"),
			},
		}
		metaByCLIPath[cliPath] = meta
	}
}

// catalogStringSliceVal reads a []string field from a catalog tool map.
func catalogStringSliceVal(tool map[string]any, key string) []string {
	raw, ok := tool[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ResolveMeta returns the complete metadata for a command identified by its CLI
// path (e.g. "dev app delete"). Returns ok=false for commands not in the
// embedded catalog (utility commands, hidden commands, shortcuts).
func ResolveMeta(cliPath string) (CommandMeta, bool) {
	metaByCLIPathOnce.Do(initMetaByCLIPath)
	m, ok := metaByCLIPath[strings.TrimSpace(cliPath)]
	return m, ok
}
