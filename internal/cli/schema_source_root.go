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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
)

// SchemaSourceRuntimeAssembled is stamped on SchemaRegistry.Source for
// declare→ResolveSchemaBuild delivery.
const SchemaSourceRuntimeAssembled = "runtime-assembled"

// errSchemaSourceRootNotRegistered is returned when Catalog / ResolveMeta
// delivery is requested without a root factory.
var errSchemaSourceRootNotRegistered = fmt.Errorf(
	"schema source root factory is not registered; call RegisterSchemaSourceRoot (app.NewRootCommand or test helper)",
)

// schemaSourceRootHolder wraps the root factory so atomic.Value can store a
// typed nil factory without Store(nil).
type schemaSourceRootHolder struct {
	fn func() *cobra.Command
}

// schemaSourceRoot stores the distribution-owned Cobra tree factory used to
// assemble Schema at runtime (声明即 Catalog). Without a factory, delivery
// fails closed — there is no committed Catalog/gob fallback. Access only via
// loadSchemaSourceRootFn / storeSchemaSourceRootFn (or RegisterSchemaSourceRoot);
// never bare-write the atomic.
var schemaSourceRoot atomic.Value // *schemaSourceRootHolder

var (
	runtimeDeliverySchemaCatalogOnce      sync.Once
	runtimeDeliverySchemaCatalog          loadedSchemaCatalog
	runtimeDeliverySchemaCatalogErr       error
	runtimeDeliverySchemaCatalogLazyCount atomic.Uint64
)

func loadSchemaSourceRootFn() func() *cobra.Command {
	if v := schemaSourceRoot.Load(); v != nil {
		return v.(*schemaSourceRootHolder).fn
	}
	return nil
}

func storeSchemaSourceRootFn(fn func() *cobra.Command) {
	schemaSourceRoot.Store(&schemaSourceRootHolder{fn: fn})
}

// resetDeliverySchemaCatalogState clears the lazy Catalog delivery Once/caches
// so the next deliverySchemaCatalog() reassembles.
func resetDeliverySchemaCatalogState() {
	runtimeDeliverySchemaCatalogOnce = sync.Once{}
	runtimeDeliverySchemaCatalog = loadedSchemaCatalog{}
	runtimeDeliverySchemaCatalogErr = nil
	runtimeDeliverySchemaCatalogLazyCount.Store(0)
}

// resetSchemaDeliveryState clears ResolveMeta lookup state and Catalog delivery
// caches. Production registration calls this for idempotent re-register;
// tests may call the ForTest wrappers that delegate here.
func resetSchemaDeliveryState() {
	metaByCLIPathOnce = sync.Once{}
	metaByCLIPath = nil
	runtimeDeliverySchemaMetaIndexErr = nil
	runtimeDeliverySchemaMetaIndexLazyCount.Store(0)
	resetDeliverySchemaCatalogState()
}

// RegisterSchemaSourceRoot installs the root factory used by runtime Schema
// delivery (dws schema / ResolveMeta). Production registers from internal/app.
// Passing nil clears the factory (tests only) and resets lazy delivery / Meta state.
func RegisterSchemaSourceRoot(factory func() *cobra.Command) {
	storeSchemaSourceRootFn(factory)
	resetSchemaDeliveryState()
}

// SchemaSourceRootRegistered reports whether runtime assembly has a root factory.
func SchemaSourceRootRegistered() bool {
	return loadSchemaSourceRootFn() != nil
}

var (
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	resolveSchemaBuildForDelivery   = ResolveSchemaBuild
)

// assembleSchemaCatalogFromRoot is the declare→Catalog runtime path.
func assembleSchemaCatalogFromRoot(root *cobra.Command) (loadedSchemaCatalog, error) {
	if root == nil {
		return loadedSchemaCatalog{}, fmt.Errorf("schema source root is nil")
	}
	resolved, err := resolveSchemaBuildForDelivery(root)
	if err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("resolve Schema build: %w", err)
	}
	if err := resolveValidateParameterDelivery(resolved.bound, resolved.registry); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("validate Schema parameter binding delivery: %w", err)
	}
	registry := resolved.registry
	registry.Source = SchemaSourceRuntimeAssembled
	if err := loadCatalogValidateInterfaces(registry); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("validate Schema interface disposition: %w", err)
	}
	if err := loadCatalogValidateProvenance(registry); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("validate Schema provenance: %w", err)
	}
	index, err := registry.Index()
	if err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("build Schema index: %w", err)
	}
	surfaceHash := resolved.RegistryHash()
	// Content source_hash must match BuildSchemaCatalogSnapshot / CI dump.
	// The payload computed here seeds both the hash and the eagerly populated
	// Snapshot.Catalog/Tools maps, so map-based consumers never re-serialize.
	payload, err := registryToSnapshotPayloadFn(registry)
	if err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("serialize Schema Catalog snapshot: %w", err)
	}
	snapshot := SchemaCatalogSnapshot{
		Version:     SchemaCatalogSnapshotVersion,
		SurfaceHash: surfaceHash,
		Catalog:     payload.Catalog,
		Tools:       payload.Tools,
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	return loadedSchemaCatalog{
		Snapshot: snapshot,
		Registry: registry,
		Index:    index,
	}, nil
}

// deliverySchemaCatalog is the sole production Catalog loader. It lazily
// assembles via ResolveSchemaBuild and caches the ResolveMeta map. Without a
// factory it fails closed.
func deliverySchemaCatalog() loadedSchemaCatalog {
	runtimeDeliverySchemaCatalogOnce.Do(func() {
		runtimeDeliverySchemaCatalogLazyCount.Add(1)
		factory := loadSchemaSourceRootFn()
		if factory == nil {
			runtimeDeliverySchemaCatalogErr = errSchemaSourceRootNotRegistered
			installDeliveryCommandMeta(loadedSchemaCatalog{}, runtimeDeliverySchemaCatalogErr)
			return
		}
		root := factory()
		if root == nil {
			runtimeDeliverySchemaCatalogErr = fmt.Errorf("schema source root factory returned nil")
			installDeliveryCommandMeta(loadedSchemaCatalog{}, runtimeDeliverySchemaCatalogErr)
			return
		}
		runtimeDeliverySchemaCatalog, runtimeDeliverySchemaCatalogErr = assembleDeliverySchemaCatalogFn(root)
		if runtimeDeliverySchemaCatalogErr != nil {
			installDeliveryCommandMeta(loadedSchemaCatalog{}, runtimeDeliverySchemaCatalogErr)
			return
		}
		installDeliveryCommandMeta(runtimeDeliverySchemaCatalog, nil)
	})
	return runtimeDeliverySchemaCatalog
}

func deliverySchemaCatalogError() error {
	_ = deliverySchemaCatalog()
	return runtimeDeliverySchemaCatalogErr
}
