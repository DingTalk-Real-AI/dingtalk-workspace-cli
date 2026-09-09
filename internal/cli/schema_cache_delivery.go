// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

const defaultSchemaCacheLockTimeout = 250 * time.Millisecond

var (
	canonicalJSONMarshal = json.Marshal
	compactLeafMarshal   = jsonutil.MarshalIndent
)

// SchemaCacheIdentity is the complete identity of one cache generation. No
// value is learned from an on-disk envelope. Production does not embed this at
// compile time; each supported machine generates it from live declarations.
type SchemaCacheIdentity = schemareader.Identity

// SchemaCacheOptions configures production cache delivery. Enabled options are
// accepted for darwin/linux on amd64/arm64; tests may inject GOOS/GOARCH.
// AllowGenerate lets an empty identity be derived from the running binary's
// declarations on first schema use.
type SchemaCacheOptions struct {
	Enabled         bool
	AllowGenerate   bool
	Edition         string
	Identity        SchemaCacheIdentity
	GOOS            string
	GOARCH          string
	LockTimeout     time.Duration
	Counters        *schemacache.Counters
	RuntimeEligible func() bool
}

func (o SchemaCacheOptions) cacheEdition() string {
	if o.Identity.Edition != "" {
		return o.Identity.Edition
	}
	if strings.TrimSpace(o.Edition) != "" {
		return strings.TrimSpace(o.Edition)
	}
	return "open"
}

type schemaCacheRegistration struct {
	options SchemaCacheOptions
	runtime *schemaCacheRuntime
}

var (
	schemaCacheRegistrationValue atomic.Pointer[schemaCacheRegistration]
	schemaCacheRuntimeUncertain  atomic.Bool
)

// RegisterSchemaCacheOptions replaces the cache registration. Invalid options
// fail closed to disabled before schemacache.Open or any filesystem operation.
func RegisterSchemaCacheOptions(options SchemaCacheOptions) error {
	schemaCacheRuntimeUncertain.Store(false)
	if !options.Enabled {
		schemaCacheRegistrationValue.Store(&schemaCacheRegistration{})
		return nil
	}
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if err := validateSchemaCacheOptions(options); err != nil {
		schemaCacheRegistrationValue.Store(&schemaCacheRegistration{})
		return err
	}
	if options.LockTimeout == 0 {
		options.LockTimeout = defaultSchemaCacheLockTimeout
	}
	r := &schemaCacheRuntime{options: options, products: make(map[string]*schemaCacheProductLoad), payloads: make(map[string]*schemaCachePayloadLoad)}
	schemaCacheRegistrationValue.Store(&schemaCacheRegistration{options: options, runtime: r})
	return nil
}

// MarkSchemaCacheRuntimeUncertain disables persistent I/O for a process whose
// runtime surface was changed after registration (for example by a plugin).
func MarkSchemaCacheRuntimeUncertain() { schemaCacheRuntimeUncertain.Store(true) }

// SchemaCacheFastPathIdentity returns only the currently registered, eligible
// authority. Replacing the source factory or marking runtime uncertainty must
// disable early process delivery just as it disables ordinary cache loaders.
// This accessor never opens the cache or assembles declarations.
func SchemaCacheFastPathIdentity() (SchemaCacheIdentity, bool) {
	auditSchemaDeliveryAccess("fast path identity")
	runtime := activeSchemaCacheRuntime()
	if runtime == nil || !schemaCacheIdentityReady(runtime.options.Identity) {
		return SchemaCacheIdentity{}, false
	}
	return runtime.options.Identity, true
}

func schemaCacheSupportedTarget(goos, goarch string) bool {
	return schemacache.PersistentBackendEnabled(goos, goarch)
}

func validateSchemaCacheOptions(options SchemaCacheOptions) error {
	if !schemaCacheSupportedTarget(options.GOOS, options.GOARCH) {
		return fmt.Errorf("Schema cache v1 is disabled for %s/%s", options.GOOS, options.GOARCH)
	}
	if schemaCacheIdentityReady(options.Identity) {
		return nil
	}
	if options.AllowGenerate && schemaCacheIdentityAbsent(options.Identity) {
		if _, err := schemacache.EditionSHA256(options.cacheEdition()); err != nil {
			return err
		}
		return nil
	}
	return options.Identity.Validate()
}

func activeSchemaCacheRuntime() *schemaCacheRuntime {
	registration := schemaCacheRegistrationValue.Load()
	if registration == nil || registration.runtime == nil || !registration.options.Enabled || schemaCacheRuntimeUncertain.Load() {
		return nil
	}
	if eligible := registration.options.RuntimeEligible; eligible != nil && !eligible() {
		return nil
	}
	return registration.runtime
}

type schemaCacheRuntime struct {
	options   SchemaCacheOptions
	openOnce  sync.Once
	cache     *schemacache.Cache
	openErr   error
	metaOnce  sync.Once
	meta      schemaruntime.DecodedSchemaMeta
	metaErr   error
	freshMeta atomic.Pointer[schemaruntime.DecodedSchemaMeta]
	indexOnce sync.Once
	index     schemaruntime.DecodedSchemaPayloadIndex
	indexErr  error
	// freshIndex is seeded by the repair path after a republish so the next
	// read uses the rebuilt generation without reopening.
	freshIndex atomic.Pointer[schemaruntime.DecodedSchemaPayloadIndex]
	// prewarm holds the speculative read started at root-command build. It is
	// assigned before its goroutine starts and published by closing done, so
	// readers that observe a non-nil prewarm may read the fields after done.
	prewarm *schemaCachePrewarm
	// payloadHandle caches one open payloads file for the process lifetime so
	// the hot path pays one open instead of one open per range. Failures are
	// never cached: a repair must be able to retry the open.
	payloadHandleMu sync.Mutex
	payloadHandle   *schemacache.Registry
	productMu       sync.Mutex
	products        map[string]*schemaCacheProductLoad
	payloadMu       sync.Mutex
	payloads        map[string]*schemaCachePayloadLoad
	allOnce         sync.Once
	all             loadedSchemaCatalog
	allErr          error
	allMu           sync.RWMutex
	freshAll        map[string]any
}

// schemaCachePrewarm is the result of the speculative read-only cache probe
// started while Cobra builds and parses. It never creates directories.
type schemaCachePrewarm struct {
	done     chan struct{}
	cache    *schemacache.Cache
	index    schemaruntime.DecodedSchemaPayloadIndex
	indexErr error
	payloads *schemacache.Registry
}

// PrewarmSchemaCache starts the speculative payload read for the registered
// runtime. It is a no-op unless an enabled, eligible runtime is registered.
// The probe opens noCreate: a missing cache is left for the synchronous path.
func PrewarmSchemaCache() {
	registration := schemaCacheRegistrationValue.Load()
	if registration == nil || registration.runtime == nil || !registration.options.Enabled || schemaCacheRuntimeUncertain.Load() {
		return
	}
	if eligible := registration.options.RuntimeEligible; eligible != nil && !eligible() {
		return
	}
	r := registration.runtime
	if r.prewarm != nil || !schemaCacheIdentityReady(r.options.Identity) {
		return
	}
	pw := &schemaCachePrewarm{done: make(chan struct{})}
	r.prewarm = pw
	go func() {
		defer close(pw.done)
		options := []schemacache.Option{schemacache.WithNoCreate()}
		if r.options.Counters != nil {
			options = append(options, schemacache.WithCounters(r.options.Counters))
		}
		cache, err := schemacache.Open(r.cacheEdition(), options...)
		if err != nil {
			pw.indexErr = err
			return
		}
		index, err := schemareader.ReadPayloadIndex(cache, r.options.Identity)
		if err != nil {
			_ = cache.Close()
			pw.indexErr = err
			return
		}
		pw.cache, pw.index = cache, index
		// The handle is a bonus: a failure leaves the synchronous open in charge.
		if payloads, handleErr := cache.OpenPayloads(r.options.Identity.ExpectedIdentity(), r.options.Identity.Payload); handleErr == nil {
			pw.payloads = payloads
		}
	}()
}

type schemaCacheProductLoad struct {
	once    sync.Once
	ready   atomic.Bool
	product schemaruntime.DecodedSchemaProduct
	err     error
}

type schemaCachePayloadLoad struct {
	ready    atomic.Bool
	payloads schemaruntime.DecodedCommandPayloads
	err      error
}

func (r *schemaCacheRuntime) cacheEdition() string { return r.options.cacheEdition() }

func (r *schemaCacheRuntime) adoptGeneratedIdentity(identity SchemaCacheIdentity) {
	r.options.Identity = identity
	r.options.AllowGenerate = false
	r.options.Edition = identity.Edition
	if registration := schemaCacheRegistrationValue.Load(); registration != nil && registration.runtime == r {
		updated := *registration
		updated.options.Identity = identity
		updated.options.AllowGenerate = false
		updated.options.Edition = identity.Edition
		schemaCacheRegistrationValue.Store(&updated)
	}
}

func (r *schemaCacheRuntime) opened() (*schemacache.Cache, error) {
	if pw := r.prewarm; pw != nil {
		<-pw.done
		if pw.cache != nil {
			return pw.cache, nil
		}
	}
	r.openOnce.Do(func() {
		options := []schemacache.Option{}
		if r.options.Counters != nil {
			options = append(options, schemacache.WithCounters(r.options.Counters))
		}
		r.cache, r.openErr = schemacache.Open(r.cacheEdition(), options...)
	})
	return r.cache, r.openErr
}

func (r *schemaCacheRuntime) readMeta() (schemaruntime.DecodedSchemaMeta, error) {
	cache, err := r.opened()
	if err != nil {
		return schemaruntime.DecodedSchemaMeta{}, err
	}
	return schemareader.ReadMeta(cache, r.options.Identity)
}

func (r *schemaCacheRuntime) loadMeta() (schemaruntime.DecodedSchemaMeta, error) {
	if meta := r.freshMeta.Load(); meta != nil {
		return *meta, nil
	}
	r.metaOnce.Do(func() {
		runtimeDeliverySchemaMetaIndexLazyCount.Add(1)
		r.meta, r.metaErr = r.readMeta()
	})
	return r.meta, r.metaErr
}

func (r *schemaCacheRuntime) seedMeta(meta schemaruntime.DecodedSchemaMeta) {
	fresh := meta
	r.freshMeta.Store(&fresh)
}

func (r *schemaCacheRuntime) readPayloadIndex() (schemaruntime.DecodedSchemaPayloadIndex, error) {
	cache, err := r.opened()
	if err != nil {
		return schemaruntime.DecodedSchemaPayloadIndex{}, err
	}
	return schemareader.ReadPayloadIndex(cache, r.options.Identity)
}

func (r *schemaCacheRuntime) loadPayloadIndex() (schemaruntime.DecodedSchemaPayloadIndex, error) {
	if index := r.freshIndex.Load(); index != nil {
		return *index, nil
	}
	if pw := r.prewarm; pw != nil {
		<-pw.done
		if pw.indexErr == nil {
			return pw.index, nil
		}
	}
	r.indexOnce.Do(func() {
		r.index, r.indexErr = r.readPayloadIndex()
	})
	return r.index, r.indexErr
}

func (r *schemaCacheRuntime) seedPayloadIndex(index schemaruntime.DecodedSchemaPayloadIndex) {
	fresh := index
	r.freshIndex.Store(&fresh)
}

func (r *schemaCacheRuntime) descriptor(meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.ProductDescriptor, bool) {
	return schemareader.Descriptor(meta, productID)
}

func (r *schemaCacheRuntime) readProduct(meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.DecodedSchemaProduct, error) {
	cache, err := r.opened()
	if err != nil {
		return schemaruntime.DecodedSchemaProduct{}, err
	}
	return schemareader.ReadProduct(cache, r.options.Identity, meta, productID)
}

func (r *schemaCacheRuntime) loadProduct(meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.DecodedSchemaProduct, error) {
	r.productMu.Lock()
	load := r.products[productID]
	if load == nil {
		load = &schemaCacheProductLoad{}
		r.products[productID] = load
	}
	r.productMu.Unlock()
	load.once.Do(func() {
		load.product, load.err = r.readProduct(meta, productID)
		load.ready.Store(true)
	})
	return load.product, load.err
}

// payloadsHandle returns the process-lifetime payloads handle: the prewarmed
// one when the speculative probe succeeded, otherwise one authenticated open.
// Failures are not cached so a repair can retry.
func (r *schemaCacheRuntime) payloadsHandle() (*schemacache.Registry, error) {
	r.payloadHandleMu.Lock()
	defer r.payloadHandleMu.Unlock()
	if r.payloadHandle != nil {
		return r.payloadHandle, nil
	}
	if pw := r.prewarm; pw != nil {
		<-pw.done
		if pw.payloads != nil {
			r.payloadHandle = pw.payloads
			return r.payloadHandle, nil
		}
	}
	cache, err := r.opened()
	if err != nil {
		return nil, err
	}
	handle, err := cache.OpenPayloads(r.options.Identity.ExpectedIdentity(), r.options.Identity.Payload)
	if err != nil {
		return nil, err
	}
	r.payloadHandle = handle
	return handle, nil
}

// resetPayloadsHandle drops the shared handle after a repair publish: the
// handle may reference a replaced inode, and the next read must reopen the
// freshly published file. A prewarmed handle never adopted by payloadsHandle
// is closed here as well; Registry close is idempotent.
func (r *schemaCacheRuntime) resetPayloadsHandle() {
	r.payloadHandleMu.Lock()
	defer r.payloadHandleMu.Unlock()
	if r.payloadHandle != nil {
		_ = r.payloadHandle.Close()
		r.payloadHandle = nil
	}
	if pw := r.prewarm; pw != nil {
		<-pw.done
		if pw.payloads != nil {
			_ = pw.payloads.Close()
			pw.payloads = nil
		}
	}
}

func (r *schemaCacheRuntime) readCommandPayload(index schemaruntime.DecodedSchemaPayloadIndex, productID string) (schemaruntime.DecodedCommandPayloads, error) {
	handle, err := r.payloadsHandle()
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	return schemareader.ReadCommandPayloadRange(handle, r.options.Identity, index, productID)
}

// loadCommandPayload caches only a success. A failed read during a concurrent
// repair must not freeze in; the next call retries so ResolveMeta stays
// deterministic.
func (r *schemaCacheRuntime) loadCommandPayload(index schemaruntime.DecodedSchemaPayloadIndex, productID string) (schemaruntime.DecodedCommandPayloads, error) {
	r.payloadMu.Lock()
	load := r.payloads[productID]
	if load == nil {
		load = &schemaCachePayloadLoad{}
		r.payloads[productID] = load
	}
	r.payloadMu.Unlock()
	if load.ready.Load() {
		return load.payloads, load.err
	}
	r.payloadMu.Lock()
	defer r.payloadMu.Unlock()
	if load.ready.Load() {
		return load.payloads, load.err
	}
	payloads, err := r.readCommandPayload(index, productID)
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	load.payloads = payloads
	load.ready.Store(true)
	return load.payloads, nil
}

// resolveCommandMetaFromPayload resolves a complete CommandMeta using only the
// payload file: the pinned index locates the product and the shard header
// carries the full identity, Safety, and Selection. Meta is never read.
// ok=false with a nil error means the path is not a Schema command; any read
// failure surfaces an error so the caller can fall through to the repair path.
func (r *schemaCacheRuntime) resolveCommandMetaFromPayload(cliPath string) (schemaruntime.CommandMeta, bool, error) {
	index, err := r.loadPayloadIndex()
	if err != nil {
		return schemaruntime.CommandMeta{}, false, err
	}
	productID, ok := schemareader.IndexLocator(index, cliPath)
	if !ok {
		return schemaruntime.CommandMeta{}, false, nil
	}
	payloads, err := r.loadCommandPayload(index, productID)
	if err != nil {
		return schemaruntime.CommandMeta{}, false, err
	}
	m, ok := payloads.Commands[cliPath]
	return m, ok, nil
}

// readCommandMetaFromPayloadFresh re-reads the payload index after a repair and
// seeds it before resolving, mirroring the meta repair flow. Every read opens
// the file afresh so a generation replaced by another process is observed.
func (r *schemaCacheRuntime) readCommandMetaFromPayloadFresh(cliPath string) (any, error) {
	index, err := r.readPayloadIndex()
	if err != nil {
		return nil, err
	}
	r.seedPayloadIndex(index)
	productID, ok := schemareader.IndexLocator(index, cliPath)
	if !ok {
		return resolvedMeta{OK: false}, nil
	}
	cache, _ := r.opened()
	payloads, err := schemareader.ReadCommandPayload(cache, r.options.Identity, index, productID)
	if err != nil {
		return nil, fmt.Errorf("read command payload for %q: %w", cliPath, err)
	}
	m, ok := payloads.Commands[cliPath]
	return resolvedMeta{Meta: m, OK: ok}, nil
}

// renderedCompactLeaf serves one canonical compact leaf query from the command
// payload file without opening the registry or Meta. Any miss or read failure
// reports false so the caller falls through to the registry-backed path, which
// owns repair semantics. Alias queries miss deliberately: their render differs
// (is_alias/cli_path), so only the canonical dot path or a primary CLI path
// may be answered here.
func (r *schemaCacheRuntime) renderedCompactLeaf(raw string) ([]byte, bool) {
	index, err := r.loadPayloadIndex()
	if err != nil {
		return nil, false
	}
	productID, ok := schemareader.IndexLocator(index, raw)
	if !ok {
		return nil, false
	}
	payloads, err := r.loadCommandPayload(index, productID)
	if err != nil {
		return nil, false
	}
	canonical := strings.TrimSpace(raw)
	ref, ok := payloads.RenderedLeaf(canonical)
	if !ok {
		path := schemaruntime.NormalizeQueryCLIPath(raw)
		if m, found := payloads.Commands[path]; found && m.Identity.CLIPath == path {
			ref, ok = payloads.RenderedLeaf(m.Identity.Canonical)
		}
		if !ok {
			return nil, false
		}
	}
	blob, err := r.readRenderedLeaf(index, productID, ref)
	if err != nil {
		return nil, false
	}
	return blob, true
}

func (r *schemaCacheRuntime) readRenderedLeaf(index schemaruntime.DecodedSchemaPayloadIndex, productID string, ref schemaruntime.RenderedLeafRef) ([]byte, error) {
	handle, err := r.payloadsHandle()
	if err != nil {
		return nil, err
	}
	return schemareader.ReadRenderedLeafRange(handle, r.options.Identity, index, productID, ref)
}

func (r *schemaCacheRuntime) trustedHashes() schemaruntime.TrustedHashes {
	return schemaruntime.TrustedHashes{
		CatalogHash: "sha256:" + hex.EncodeToString(r.options.Identity.SourceSHA256[:]),
		SurfaceHash: "sha256:" + hex.EncodeToString(r.options.Identity.SurfaceSHA256[:]),
	}
}

func (r *schemaCacheRuntime) overviewPayload(meta schemaruntime.DecodedSchemaMeta) (map[string]any, error) {
	payload, err := meta.Overview.ToPayload()
	if err != nil {
		return nil, err
	}
	schemaruntime.StampTrustedHashes(payload, r.trustedHashes())
	return payload, nil
}

func (r *schemaCacheRuntime) loadOverviewPayload() (map[string]any, error) {
	meta, err := r.loadMeta()
	if err != nil {
		return nil, err
	}
	return r.overviewPayload(meta)
}

func schemaCacheLocator(meta schemaruntime.DecodedSchemaMeta, raw string) (string, bool) {
	return schemareader.Locator(meta, raw)
}

func (r *schemaCacheRuntime) queryPayload(meta schemaruntime.DecodedSchemaMeta, raw string, cached bool) (map[string]any, error) {
	productID, ok := schemaCacheLocator(meta, raw)
	if !ok {
		return nil, schemaruntime.UnknownPathError{Path: strings.TrimSpace(raw)}
	}
	var product schemaruntime.DecodedSchemaProduct
	var err error
	if cached {
		product, err = r.loadProduct(meta, productID)
	} else {
		product, err = r.readProduct(meta, productID)
		if err == nil {
			r.storeProduct(productID, product)
		}
	}
	if err != nil {
		return nil, err
	}
	payload, err := schemaruntime.RenderQueryWithProjectors(product.Registry, product.Index, raw, schemaruntime.QueryProjectors{
		ProductSummary: renderSchemaProductSummary,
		ToolSummary:    renderSchemaToolSummary,
	})
	if err == nil {
		return payload, nil
	}
	var unknown schemaruntime.UnknownPathError
	if errors.As(err, &unknown) {
		return nil, unknown
	}
	return nil, err
}

func (r *schemaCacheRuntime) loadQueryPayload(raw string) (map[string]any, error) {
	meta, err := r.loadMeta()
	if err != nil {
		return nil, err
	}
	return r.queryPayload(meta, raw, true)
}

func (r *schemaCacheRuntime) readQueryPayload(raw string) (map[string]any, error) {
	meta, err := r.readMeta()
	if err != nil {
		return nil, err
	}
	r.seedMeta(meta)
	return r.queryPayload(meta, raw, false)
}

func (r *schemaCacheRuntime) readAllPayload(meta schemaruntime.DecodedSchemaMeta, reuseProducts bool) (map[string]any, error) {
	cache, err := r.opened()
	if err != nil {
		return nil, err
	}
	registryFile, err := cache.OpenRegistry(r.options.Identity.ExpectedIdentity(), r.options.Identity.Registry)
	if err != nil {
		return nil, err
	}
	defer registryFile.Close()
	if err := registryFile.ValidateAggregate(); err != nil {
		return nil, err
	}
	registry := SchemaRegistry{Kind: meta.Kind, Level: meta.Level, Source: meta.Source, AgentMetadata: append([]byte(nil), meta.AgentMetadata...)}
	for _, descriptor := range meta.ProductDescriptors {
		var decoded schemaruntime.DecodedSchemaProduct
		var decodeErr error
		if reuseProducts {
			decoded, decodeErr = r.cachedProduct(descriptor.ProductID)
		}
		if decodeErr != nil || len(decoded.Registry.Products) == 0 {
			payload, readErr := registryFile.ReadRange(schemacache.RangeDescriptor{Offset: descriptor.Offset, Length: descriptor.Length, SHA256: descriptor.SHA256})
			if readErr != nil {
				return nil, readErr
			}
			decoded, decodeErr = schemaruntime.DecodeSchemaProductCache(payload, descriptor, meta)
			if decodeErr != nil {
				return nil, decodeErr
			}
			r.storeProduct(descriptor.ProductID, decoded)
		}
		registry.Products = append(registry.Products, decoded.Registry.Products[0])
	}
	index, err := registry.Index()
	if err != nil {
		return nil, err
	}
	payload, err := schemaruntime.RenderAll(index.Registry(), r.trustedHashes())
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (r *schemaCacheRuntime) loadAllPayload() (map[string]any, error) {
	r.allMu.RLock()
	fresh := r.freshAll
	r.allMu.RUnlock()
	if fresh != nil {
		return fresh, nil
	}
	r.allOnce.Do(func() {
		meta, err := r.loadMeta()
		if err != nil {
			r.allErr = err
			return
		}
		payload, err := r.readAllPayload(meta, true)
		if err != nil {
			r.allErr = err
			return
		}
		// Keep the already-rendered payload in the Snapshot catalog slot only as
		// an internal holder; normal cache hits never construct public Snapshot maps.
		r.all.Snapshot.Catalog = payload
	})
	if r.allErr != nil {
		return nil, r.allErr
	}
	return r.all.Snapshot.Catalog, nil
}

func (r *schemaCacheRuntime) seedAll(payload map[string]any) {
	r.allMu.Lock()
	r.freshAll = payload
	r.allMu.Unlock()
}

func (r *schemaCacheRuntime) cachedProduct(productID string) (schemaruntime.DecodedSchemaProduct, error) {
	r.productMu.Lock()
	load := r.products[productID]
	r.productMu.Unlock()
	if load == nil || !load.ready.Load() {
		return schemaruntime.DecodedSchemaProduct{}, fmt.Errorf("product %q is not loaded", productID)
	}
	return load.product, load.err
}

func (r *schemaCacheRuntime) storeProduct(productID string, product schemaruntime.DecodedSchemaProduct) {
	// Publish a completed immutable load. A prior failed or in-flight attempt
	// must not consume this successful repair through its already-used Once.
	load := &schemaCacheProductLoad{product: product}
	load.once.Do(func() {})
	load.ready.Store(true)
	r.productMu.Lock()
	r.products[productID] = load
	r.productMu.Unlock()
}

// repairSchemaCache is the sole miss/corruption coordinator. The lock-holder
// rechecks with low-level readers before touching the process-wide live Once.
func repairSchemaCache(r *schemaCacheRuntime, recheck func() (any, error)) (any, loadedSchemaCatalog, error) {
	if loaded := runtimeDeliveryLiveCatalog.Load(); loaded != nil {
		return nil, *loaded, nil
	}
	cache, openErr := r.opened()
	if openErr == nil {
		lock, lockErr := cache.AcquireLock(context.Background(), r.options.LockTimeout)
		if lockErr == nil {
			defer lock.Release()
			// The shared handle may reference an inode another process
			// replaced; drop it before rechecking so later reads reopen.
			r.resetPayloadsHandle()
			if value, err := recheck(); err == nil {
				return value, loadedSchemaCatalog{}, nil
			}
			loaded := deliverySchemaCatalog()
			if runtimeDeliverySchemaCatalogErr != nil {
				return nil, loadedSchemaCatalog{}, runtimeDeliverySchemaCatalogErr
			}
			r.publishGeneratedOrMatching(cache, loaded)
			return nil, loaded, nil
		}
		// Timeout and lock failures both preserve authoritative availability and
		// skip publication. No cache error may override a successful live result.
	}
	loaded := deliverySchemaCatalog()
	if runtimeDeliverySchemaCatalogErr != nil {
		return nil, loadedSchemaCatalog{}, runtimeDeliverySchemaCatalogErr
	}
	return nil, loaded, nil
}

func (r *schemaCacheRuntime) publishGeneratedOrMatching(cache *schemacache.Cache, loaded loadedSchemaCatalog) {
	if cache == nil {
		return
	}
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		return
	}
	identity := r.options.Identity
	if !artifacts.match(identity) {
		generated, genErr := IdentityFromArtifacts(r.cacheEdition(), artifacts)
		if genErr != nil {
			return
		}
		identity = generated
		r.adoptGeneratedIdentity(identity)
	} else if !schemaCacheIdentityReady(identity) {
		return
	}
	_ = persistLocalSchemaCacheIdentity(cache.Directory(), identity)
	_ = cache.Publish(identity.ExpectedIdentity(), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact())
}

// SchemaCacheArtifacts is the deterministic cache hand-off used by the
// identity generator. Payload slices are detached from assembly state.
type SchemaCacheArtifacts struct {
	Version            int
	SourceHash         string
	SurfaceHash        string
	Meta               []byte
	Registry           []byte
	Payload            []byte
	ProductCount       int
	MetaSHA256         [sha256.Size]byte
	RegistrySHA256     [sha256.Size]byte
	PayloadSHA256      [sha256.Size]byte
	ProductDescriptors []schemaruntime.ProductDescriptor
	registry           SchemaRegistry
	index              SchemaIndex
	locators           map[string]string
}

// BuildSchemaCacheArtifacts validates and snapshots one ResolvedSchemaBuild,
// then derives Meta and product shards from that exact typed registry.
func BuildSchemaCacheArtifacts(resolved ResolvedSchemaBuild) (SchemaCacheArtifacts, error) {
	snapshot, err := BuildSchemaCatalogSnapshot(resolved, SchemaCatalogBuildOptions{RegistryHash: resolved.RegistryHash()})
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	registry := resolved.registry
	registry.Source = SchemaSourceRuntimeAssembled
	// Snapshot already indexed this registry; Index cannot fail here.
	index, _ := registry.Index()
	return buildSchemaCacheArtifacts(index.Registry(), snapshot.SourceHash, snapshot.SurfaceHash)
}

func buildSchemaCacheArtifactsFromLoaded(loaded loadedSchemaCatalog) (SchemaCacheArtifacts, error) {
	return buildSchemaCacheArtifacts(loaded.Registry, loaded.Snapshot.SourceHash, loaded.Snapshot.SurfaceHash)
}

func buildSchemaCacheArtifacts(registry SchemaRegistry, sourceHash, surfaceHash string) (SchemaCacheArtifacts, error) {
	hashes, err := schemaCacheHashes(sourceHash, surfaceHash)
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	registry, err = canonicalSchemaCacheRegistry(registry)
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	index, err := registry.Index()
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	registry = index.Registry()
	// Index just succeeded; BuildSchemaOverview only fails on Index.
	overview, _ := schemaruntime.BuildSchemaOverview(registry)
	locators, err := schemaruntime.BuildSchemaProductLocators(registry)
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	rendered, err := renderCompactSchemaLeaves(registry, index)
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	built, err := schemaruntime.BuildSchemaCache(registry, schemaruntime.BuildCommandMetaLookup(registry), overview, locators, hashes, rendered)
	if err != nil {
		return SchemaCacheArtifacts{}, err
	}
	return SchemaCacheArtifacts{
		Version: SchemaCatalogSnapshotVersion, SourceHash: sourceHash, SurfaceHash: surfaceHash,
		Meta: append([]byte(nil), built.Meta...), Registry: append([]byte(nil), built.ProductShards...),
		Payload:      append([]byte(nil), built.PayloadShards...),
		ProductCount: len(built.Descriptors), MetaSHA256: sha256.Sum256(built.Meta), RegistrySHA256: built.RegistrySHA256,
		PayloadSHA256:      built.PayloadSHA256,
		ProductDescriptors: append([]schemaruntime.ProductDescriptor(nil), built.Descriptors...),
		registry:           registry, index: index, locators: locators,
	}, nil
}

// renderCompactSchemaLeaves pre-renders every canonical leaf's compact JSON
// output bytes through the same render, projection, and writer semantics as
// the schema command, so a cached leaf query is byte-identical to the live
// render without opening the registry shard.
func renderCompactSchemaLeaves(registry SchemaRegistry, index SchemaIndex) (map[string][]byte, error) {
	projectors := schemaruntime.QueryProjectors{
		ProductSummary: renderSchemaProductSummary,
		ToolSummary:    renderSchemaToolSummary,
	}
	rendered := make(map[string][]byte)
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			path := strings.TrimSpace(tool.Identity.CLIPath)
			canonical := strings.TrimSpace(tool.Identity.CanonicalPath)
			if path == "" || canonical == "" {
				continue
			}
			payload, err := schemaruntime.RenderQueryWithProjectors(registry, index, path, projectors)
			if err != nil {
				return nil, fmt.Errorf("render Schema leaf %q: %w", path, err)
			}
			data, err := compactLeafMarshal(stripSchemaPayloadCompact(payload), "", "  ")
			if err != nil {
				return nil, fmt.Errorf("render Schema leaf %q: %w", path, err)
			}
			rendered[canonical] = append(data, '\n')
		}
	}
	return rendered, nil
}

// canonicalSchemaCacheRegistry removes irrelevant JSON object insertion order
// from raw typed fields before deterministic protobuf encoding. Public Schema
// already treats these fields as JSON values; binding cache identity to their
// producer's map iteration order would make an otherwise identical build ID
// unstable across authoritative assemblies.
func canonicalSchemaCacheRegistry(registry SchemaRegistry) (SchemaRegistry, error) {
	var err error
	canonical := func(path string, raw json.RawMessage) json.RawMessage {
		if err != nil || raw == nil {
			return raw
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decodeErr := decoder.Decode(&value); decodeErr != nil {
			err = fmt.Errorf("canonicalize %s: %w", path, decodeErr)
			return nil
		}
		if trailingErr := decoder.Decode(&struct{}{}); trailingErr != io.EOF {
			err = fmt.Errorf("canonicalize %s: multiple JSON values", path)
			return nil
		}
		encoded, encodeErr := canonicalJSONMarshal(value)
		if encodeErr != nil {
			err = fmt.Errorf("canonicalize %s: %w", path, encodeErr)
			return nil
		}
		return encoded
	}
	canonicalProvenance := func(path string, source map[string]contract.FieldProvenance) map[string]contract.FieldProvenance {
		if source == nil {
			return nil
		}
		result := make(map[string]contract.FieldProvenance, len(source))
		for key, provenance := range source {
			provenance.Value = canonical(path+"."+key+".value", provenance.Value)
			provenance.Candidates = append([]contract.FieldCandidateProvenance(nil), provenance.Candidates...)
			for i := range provenance.Candidates {
				provenance.Candidates[i].Value = canonical(fmt.Sprintf("%s.%s.candidates[%d]", path, key, i), provenance.Candidates[i].Value)
			}
			provenance.OverriddenCandidates = append([]contract.FieldCandidateProvenance(nil), provenance.OverriddenCandidates...)
			for i := range provenance.OverriddenCandidates {
				provenance.OverriddenCandidates[i].Value = canonical(fmt.Sprintf("%s.%s.overridden_candidates[%d]", path, key, i), provenance.OverriddenCandidates[i].Value)
			}
			result[key] = provenance
		}
		return result
	}
	registry.AgentMetadata = canonical("agent_metadata", registry.AgentMetadata)
	registry.Products = append([]ProductSpec(nil), registry.Products...)
	for productIndex := range registry.Products {
		product := &registry.Products[productIndex]
		product.FieldProvenance = canonicalProvenance("product "+product.ID, product.FieldProvenance)
		product.Tools = append([]ToolSpec(nil), product.Tools...)
		for toolIndex := range product.Tools {
			tool := &product.Tools[toolIndex]
			path := "tool " + tool.Identity.CanonicalPath
			tool.FieldProvenance = canonicalProvenance(path, tool.FieldProvenance)
			tool.Parameters = append([]ParameterSpec(nil), tool.Parameters...)
			for parameterIndex := range tool.Parameters {
				parameter := &tool.Parameters[parameterIndex]
				parameterPath := path + " parameter " + parameter.Name
				parameter.Default = canonical(parameterPath+" default", parameter.Default)
				parameter.InterfaceDefault = canonical(parameterPath+" interface_default", parameter.InterfaceDefault)
				parameter.Example = canonical(parameterPath+" example", parameter.Example)
				parameter.FieldProvenance = canonicalProvenance(parameterPath, parameter.FieldProvenance)
			}
			if tool.Result != nil {
				result := *tool.Result
				result.DataSchema = canonical(path+" result data_schema", result.DataSchema)
				tool.Result = &result
			}
		}
	}
	if err != nil {
		return SchemaRegistry{}, err
	}
	return registry, nil
}

// RenderAll returns the public full-export projection represented by these
// exact artifacts without rebuilding the authoritative source tree.
func (a SchemaCacheArtifacts) RenderAll() (map[string]any, error) {
	return schemaruntime.RenderAll(a.registry, schemaruntime.TrustedHashes{CatalogHash: a.SourceHash, SurfaceHash: a.SurfaceHash})
}

// RenderOverview returns the public Meta-only overview projection.
func (a SchemaCacheArtifacts) RenderOverview() (map[string]any, error) {
	return schemaruntime.RenderOverview(a.registry, schemaruntime.TrustedHashes{CatalogHash: a.SourceHash, SurfaceHash: a.SurfaceHash})
}

// RenderQuery returns a product/group/leaf projection from the exact registry
// used to create the cache artifacts.
func (a SchemaCacheArtifacts) RenderQuery(path string) (map[string]any, error) {
	return schemaruntime.RenderQuery(a.registry, a.index, path)
}

// LocatorPaths returns a detached, sorted list of every authenticated path in
// Meta. It is primarily useful for exhaustive generation and parity gates.
func (a SchemaCacheArtifacts) LocatorPaths() []string {
	paths := make([]string, 0, len(a.locators))
	for path := range a.locators {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// ValidateRoundTrip proves the release hand-off before its digests can become
// binary trust anchors. It is intentionally a build-time operation: a cache
// hit authenticates bytes and validates the selected DTO, without reconstructing
// the complete public Catalog.
func (a SchemaCacheArtifacts) ValidateRoundTrip() error {
	meta, err := schemaruntime.DecodeSchemaMetaCache(a.Meta)
	if err != nil {
		return fmt.Errorf("decode generated Meta: %w", err)
	}
	meta.MaterializeCommandMeta()
	wantLookup := schemaruntime.BuildCommandMetaLookup(a.registry)
	if len(meta.CommandMetaByPath) != len(wantLookup) {
		return fmt.Errorf("generated Meta command count %d differs from authoritative Registry %d", len(meta.CommandMetaByPath), len(wantLookup))
	}
	for path, expected := range wantLookup {
		actual, ok := meta.CommandMetaByPath[path]
		if !ok || !reflect.DeepEqual(actual.Identity, expected.Identity) {
			return fmt.Errorf("generated Meta command identity differs at %q", path)
		}
	}
	if !reflect.DeepEqual(meta.LocatorProductByPath, a.locators) ||
		!reflect.DeepEqual(meta.ProductDescriptors, a.ProductDescriptors) {
		return fmt.Errorf("generated Meta projection differs from authoritative Registry")
	}
	wantOverview, err := a.registry.ToOverviewPayload()
	if err != nil {
		return err
	}
	gotOverview, err := meta.Overview.ToPayload()
	if err != nil || !reflect.DeepEqual(wantOverview, gotOverview) {
		return fmt.Errorf("generated Meta overview differs from authoritative Registry: %v", err)
	}
	registry, index, err := schemaruntime.DecodeAllSchemaProducts(a.Registry, meta)
	if err != nil {
		return fmt.Errorf("decode generated Registry: %w", err)
	}
	if !reflect.DeepEqual(registry, a.registry) {
		return fmt.Errorf("generated Registry differs from authoritative Registry")
	}
	for _, path := range a.LocatorPaths() {
		want, wantErr := a.RenderQuery(path)
		got, gotErr := schemaruntime.RenderQuery(registry, index, path)
		if wantErr != nil || gotErr != nil || !reflect.DeepEqual(want, got) {
			return fmt.Errorf("generated query %q differs from authoritative Registry: original=%v decoded=%v", path, wantErr, gotErr)
		}
	}
	return nil
}

func schemaCacheHashes(sourceHash, surfaceHash string) (schemaruntime.CacheHashes, error) {
	parse := func(name, value string) ([sha256.Size]byte, error) {
		var out [sha256.Size]byte
		if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
			return out, fmt.Errorf("%s is not an exact SHA-256 identity", name)
		}
		decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
		if err != nil {
			return out, fmt.Errorf("%s: %w", name, err)
		}
		copy(out[:], decoded)
		return out, nil
	}
	source, err := parse("source_hash", sourceHash)
	if err != nil {
		return schemaruntime.CacheHashes{}, err
	}
	surface, err := parse("surface_hash", surfaceHash)
	if err != nil {
		return schemaruntime.CacheHashes{}, err
	}
	return schemaruntime.CacheHashes{SourceSHA256: source, SurfaceSHA256: surface}, nil
}

func (a SchemaCacheArtifacts) match(identity SchemaCacheIdentity) bool {
	return a.Version == int(identity.CatalogSnapshotVersion) &&
		"sha256:"+hex.EncodeToString(identity.SourceSHA256[:]) == a.SourceHash &&
		"sha256:"+hex.EncodeToString(identity.SurfaceSHA256[:]) == a.SurfaceHash &&
		uint64(len(a.Meta)) == identity.Meta.EncodedLength && a.MetaSHA256 == identity.Meta.EncodedSHA256 &&
		uint64(len(a.Registry)) == identity.Registry.EncodedLength && a.RegistrySHA256 == identity.Registry.EncodedSHA256 &&
		uint64(len(a.Payload)) == identity.Payload.EncodedLength && a.PayloadSHA256 == identity.Payload.EncodedSHA256
}

// PayloadIndexPins derives the pinned payload index region identity from the
// artifact's self-describing prefix: the region length including the 4-byte
// prefix, and the region digest.
func (a SchemaCacheArtifacts) PayloadIndexPins() (uint64, [sha256.Size]byte, error) {
	if len(a.Payload) < 4 {
		return 0, [sha256.Size]byte{}, fmt.Errorf("payload artifact is shorter than its index prefix")
	}
	length := uint64(binary.BigEndian.Uint32(a.Payload[:4])) + 4
	if length > uint64(len(a.Payload)) {
		return 0, [sha256.Size]byte{}, fmt.Errorf("payload index region %d exceeds payload length %d", length, len(a.Payload))
	}
	return length, sha256.Sum256(a.Payload[:length]), nil
}

func (a SchemaCacheArtifacts) MetaArtifact() schemacache.Artifact {
	return schemacache.Artifact{Expectation: schemacache.ArtifactExpectation{
		Kind: schemacache.KindMeta, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(a.Meta)), DecodedLength: uint64(len(a.Meta)), EncodedSHA256: a.MetaSHA256,
	}, Payload: append([]byte(nil), a.Meta...)}
}

func (a SchemaCacheArtifacts) RegistryArtifact() schemacache.Artifact {
	return schemacache.Artifact{Expectation: schemacache.ArtifactExpectation{
		Kind: schemacache.KindRegistry, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(a.Registry)), DecodedLength: uint64(len(a.Registry)), EncodedSHA256: a.RegistrySHA256,
	}, Payload: append([]byte(nil), a.Registry...)}
}

func (a SchemaCacheArtifacts) PayloadArtifact() schemacache.Artifact {
	return schemacache.Artifact{Expectation: schemacache.ArtifactExpectation{
		Kind: schemacache.KindPayloads, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(a.Payload)), DecodedLength: uint64(len(a.Payload)), EncodedSHA256: a.PayloadSHA256,
	}, Payload: append([]byte(nil), a.Payload...)}
}
