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
// declaration layer a field comes from.
//
// ResolveMeta projects CommandMeta from the lazily assembled SchemaRegistry
// (RegisterSchemaSourceRoot → ResolveSchemaBuild). Without a registered
// factory it fails closed — there is no schema_meta_index.gob fallback.

package cli

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
)

// CommandMeta is the complete runtime metadata view for a single command.
// Consumers read this struct; they never touch the raw catalog maps.
type CommandMeta = schemaruntime.CommandMeta

// CommandIdentity is the stable identity of a command.
type CommandIdentity = schemaruntime.CommandIdentity

// CommandSelection is the agent-facing selection metadata.
type CommandSelection = schemaruntime.CommandSelection

type CommandSafety = schemaruntime.CommandSafety

var (
	metaByCLIPathOnce sync.Once
	metaByCLIPath     map[string]CommandMeta
)

// Counter names retain "MetaIndex" for RuntimeSchemaMetadataLoadCounts
// compatibility; they now count ResolveMeta lookup init from assembled Catalog.
var (
	runtimeDeliverySchemaMetaIndexErr       error
	runtimeDeliverySchemaMetaIndexLazyCount atomic.Uint64
)

// installDeliveryCommandMeta materializes the ResolveMeta lookup from an
// assembled Catalog. Called from deliverySchemaCatalog's sync.Once so Meta
// shares assembly and subsequent ResolveMeta is a plain map read.
func installDeliveryCommandMeta(loaded loadedSchemaCatalog, err error) {
	runtimeDeliverySchemaMetaIndexLazyCount.Add(1)
	if err != nil {
		runtimeDeliverySchemaMetaIndexErr = err
		metaByCLIPath = nil
		metaByCLIPathOnce.Do(func() {})
		return
	}
	metaByCLIPath = buildMetaByCLIPath(loaded)
	runtimeDeliverySchemaMetaIndexErr = nil
	metaByCLIPathOnce.Do(func() {})
}

// panicIfMetaIndexUnusable fails closed when CommandMeta lookup could not be
// built. Callers must not treat this as "command missing".
func panicIfMetaIndexUnusable(err error) {
	if err == nil {
		return
	}
	panic(fmt.Sprintf("schema CommandMeta index is unusable: %v", err))
}

// buildMetaByCLIPath constructs the lookup from a loaded catalog.
// Prefer the typed Registry (production cold-start path). Fall back to
// Snapshot.Tools maps for unit tests that synthesize untyped fixtures.
func buildMetaByCLIPath(loaded loadedSchemaCatalog) map[string]CommandMeta {
	if len(loaded.Registry.Products) > 0 {
		return buildMetaByCLIPathFromRegistry(loaded.Registry)
	}
	return buildMetaByCLIPathFromSnapshotTools(loaded.Snapshot.Tools)
}

func buildMetaByCLIPathFromRegistry(registry SchemaRegistry) map[string]CommandMeta {
	return schemaruntime.BuildCommandMetaLookup(registry)
}

func buildMetaByCLIPathFromSnapshotTools(tools map[string]map[string]any) map[string]CommandMeta {
	lookup := make(map[string]CommandMeta)
	if tools == nil {
		return lookup
	}
	metas := make([]CommandMeta, 0, len(tools))
	for _, tool := range tools {
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
				AgentSummary:  schemaString(tool["agent_summary"]),
				UseWhen:       schemaStringSlice(tool["use_when"]),
				AvoidWhen:     schemaStringSlice(tool["avoid_when"]),
				Prerequisites: schemaStringSlice(tool["prerequisites"]),
				Tips:          schemaStringSlice(tool["tips"]),
				Examples:      schemaStringSlice(tool["examples"]),
			},
		}
		lookup[cliPath] = meta
		metas = append(metas, meta)
	}
	return registerCommandMetaAliases(lookup, metas)
}

// registerCommandMetaAliases fills compat alias paths. Primary paths always
// win; alias-vs-alias collisions resolve to the owner with the
// lexicographically smallest primary cli_path (map iteration is unstable).
func registerCommandMetaAliases(lookup map[string]CommandMeta, metas []CommandMeta) map[string]CommandMeta {
	return schemaruntime.RegisterCommandMetaAliases(lookup, metas)
}

// ResolveMeta returns the complete metadata for a command identified by its CLI
// path (e.g. "dev app delete") or one of its compat aliases (e.g. "report list"
// for "report inbox list"). Returns ok=false for commands not in the Schema
// surface (utility commands, hidden commands, shortcuts).
//
// A persistent hit authenticates and decodes Meta only, then performs an O(1)
// map lookup. Misses and corruption use the shared authoritative repair path.
func ResolveMeta(cliPath string) (CommandMeta, bool) {
	auditSchemaDeliveryAccess("ResolveMeta")
	cliPath = strings.TrimSpace(cliPath)
	if loaded := runtimeDeliveryLiveCatalog.Load(); loaded != nil {
		m, ok := metaByCLIPath[cliPath]
		return m, ok
	}
	if runtime := activeSchemaCacheRuntime(); runtime != nil {
		meta, err := runtime.loadMeta()
		if err == nil {
			m, ok := meta.CommandMetaByPath[cliPath]
			return m, ok
		}
		value, _, repairErr := repairSchemaCache(runtime, func() (any, error) {
			fresh, freshErr := runtime.readMeta()
			if freshErr != nil {
				return nil, freshErr
			}
			runtime.seedMeta(fresh)
			m, ok := fresh.CommandMetaByPath[cliPath]
			return resolvedMeta{Meta: m, OK: ok}, nil
		})
		if repairErr == nil && value != nil {
			result := value.(resolvedMeta)
			return result.Meta, result.OK
		}
	}
	_ = deliverySchemaCatalog()
	panicIfMetaIndexUnusable(runtimeDeliverySchemaMetaIndexErr)
	m, ok := metaByCLIPath[cliPath]
	return m, ok
}

type resolvedMeta struct {
	Meta CommandMeta
	OK   bool
}
