// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"runtime"
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
