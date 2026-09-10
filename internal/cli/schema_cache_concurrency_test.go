// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
)

func TestCrossPlatformCoverageSchemaMetaPublication(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	for i := 0; i < 20; i++ {
		restorePackageCLISchemaDeliveryForTest()
		done := make(chan struct{})
		go func() {
			deliverySchemaCatalog()
			close(done)
		}()
		for runtimeDeliveryLiveCatalog.Load() == nil {
			runtime.Gosched()
		}
		meta, ok := ResolveMeta("calendar event create")
		<-done
		if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
			t.Fatalf("published catalog has incomplete Meta at iteration %d: %#v, %v", i, meta, ok)
		}
	}
}

func TestCrossPlatformCoverageSchemaProductMemoizationRepair(t *testing.T) {
	r := &schemaCacheRuntime{products: map[string]*schemaCacheProductLoad{"calendar": {}}}
	inFlight := r.products["calendar"]
	if _, err := r.cachedProduct("calendar"); err == nil {
		t.Fatal("unfinished product reported ready")
	}
	// Inspecting an unfinished product must not consume its loader's Once.
	started := false
	inFlight.once.Do(func() { started = true })
	if !started {
		t.Fatal("--all inspection consumed a concurrent leaf loader")
	}
	inFlight.err = errors.New("previous read failed")
	inFlight.ready.Store(true)
	if _, err := r.cachedProduct("calendar"); err == nil {
		t.Fatal("failed product reported successful")
	}
	repaired := schemaruntime.DecodedSchemaProduct{Registry: SchemaRegistry{Products: []ProductSpec{{ID: "calendar"}}}}
	r.storeProduct("calendar", repaired)
	got, err := r.cachedProduct("calendar")
	if err != nil || len(got.Registry.Products) != 1 || got.Registry.Products[0].ID != "calendar" {
		t.Fatalf("successful repair did not replace failed memoization: %#v, %v", got, err)
	}
}

func TestCrossPlatformCoverageSchemaCacheConcurrentPrewarmPublish(t *testing.T) {
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	identity := coverageSchemaCacheIdentity()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: "linux", GOARCH: "amd64",
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			PrewarmSchemaCache()
		}()
	}
	wait.Wait()
	AwaitSchemaCachePrewarmForTest()
	runtimeCache := activeSchemaCacheRuntime()
	if runtimeCache == nil || runtimeCache.prewarm.Load() == nil {
		t.Fatal("concurrent prewarm did not publish exactly one probe")
	}
}

func TestCrossPlatformCoverageSchemaCacheAllowGenerateEmptyIdentity(t *testing.T) {
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open",
		GOOS: "linux", GOARCH: "amd64",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("generate-pending identity must not be a fast-path authority")
	}
	PrewarmSchemaCache()
	if activeSchemaCacheRuntime() == nil {
		t.Fatal("allow-generate runtime was not registered")
	}
}

func TestCrossPlatformCoverageSchemaCacheOptionsAcceptSupportedTargets(t *testing.T) {
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	for _, target := range []struct{ goos, goarch string }{
		{"darwin", "arm64"}, {"darwin", "amd64"},
		{"linux", "amd64"}, {"linux", "arm64"},
		{"windows", "amd64"}, {"windows", "arm64"},
	} {
		if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
			Enabled: true, AllowGenerate: true, Edition: "open",
			GOOS: target.goos, GOARCH: target.goarch,
		}); err != nil {
			t.Fatalf("%s/%s: %v", target.goos, target.goarch, err)
		}
	}
}

func TestCrossPlatformCoverageSchemaCacheOptionsRejectUnsupportedPlatform(t *testing.T) {
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, GOOS: "js", GOARCH: "wasm",
	})
	if err == nil || !strings.Contains(err.Error(), "js/wasm") {
		t.Fatalf("unsupported platform error = %v", err)
	}
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("rejected options still exposed a fast-path identity")
	}
}

func TestCrossPlatformCoverageSchemaCacheFastPathIdentityRequiresEligibleRuntime(t *testing.T) {
	t.Cleanup(func() {
		schemaCacheRuntimeUncertain.Store(false)
		_ = RegisterSchemaCacheOptions(SchemaCacheOptions{})
	})
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("disabled cache exposed a fast-path identity")
	}
	schemaCacheRegistrationValue.Store(&schemaCacheRegistration{
		options: SchemaCacheOptions{Enabled: true, AllowGenerate: true, Edition: "open"},
		runtime: &schemaCacheRuntime{options: SchemaCacheOptions{Enabled: true, AllowGenerate: true, Edition: "open"}},
	})
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("generate-pending identity must not be a fast-path authority")
	}
	if activeSchemaCacheRuntime() == nil {
		t.Fatal("generate-pending runtime should remain active")
	}
	MarkSchemaCacheRuntimeUncertain()
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("uncertain runtime still exposed a fast-path identity")
	}
	if activeSchemaCacheRuntime() != nil {
		t.Fatal("uncertain runtime still active")
	}
}

func TestCrossPlatformCoverageSchemaSourceRegistrationClearsCache(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	defer schemaCacheRegistrationValue.Store(&schemaCacheRegistration{})
	schemaCacheRegistrationValue.Store(&schemaCacheRegistration{
		options: SchemaCacheOptions{Enabled: true}, runtime: &schemaCacheRuntime{},
	})
	RegisterSchemaSourceRoot(nil)
	if activeSchemaCacheRuntime() != nil {
		t.Fatal("new source root retained a previous authority's persistent identity")
	}
}
