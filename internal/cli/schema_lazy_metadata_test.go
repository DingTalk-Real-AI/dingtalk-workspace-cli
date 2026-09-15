// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const schemaLazyMetadataChildEnv = "DWS_SCHEMA_LAZY_METADATA_CHILD"
const schemaLazyCatalogChildEnv = "DWS_SCHEMA_LAZY_CATALOG_CHILD"

// TestRuntimeSchemaMetadataLoadsOnlyOnDemand runs in a fresh test process so
// no earlier Schema test can initialize a sync.Once. The child observes the
// state immediately after package init, then exercises concurrent first use.
func TestRuntimeSchemaMetadataLoadsOnlyOnDemand(t *testing.T) {
	if os.Getenv(schemaLazyMetadataChildEnv) == "1" {
		// Package-cli TestMain may assemble Catalog via cmd_schema_catalog dump
		// decode; that must not force Agent / parameter-binding loads.
		agentBefore := runtimeAgentMetadataLazyLoadCount.Load()
		paramBefore := runtimeSchemaParameterBindingsLazyLoadCount.Load()
		if agentBefore != 0 {
			t.Fatalf("Agent metadata loaded during package init: %d", agentBefore)
		}
		if paramBefore != 0 {
			t.Fatalf("parameter bindings loaded during package init: %d", paramBefore)
		}

		var wait sync.WaitGroup
		for range 32 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_ = runtimeAgentMetadata()
				_, _ = runtimeSchemaParameterBindingData()
			}()
		}
		wait.Wait()

		if got := runtimeAgentMetadataLazyLoadCount.Load(); got != 1 {
			t.Fatalf("Agent metadata lazy load count = %d, want 1", got)
		}
		if got := runtimeSchemaParameterBindingsLazyLoadCount.Load(); got != 1 {
			t.Fatalf("parameter bindings lazy load count = %d, want 1", got)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeSchemaMetadataLoadsOnlyOnDemand$", "-test.count=1")
	command.Env = append(os.Environ(), schemaLazyMetadataChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("lazy metadata child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

// TestDeliverySchemaCatalogProductionDecodeLoadsOnlyOnce uses a separate fresh
// process because sync.Once is intentionally not resettable. The test does not
// require the committed snapshot to be valid: a successful decode and a
// fail-closed decode must both be attempted exactly once under concurrent first
// use.
func TestDeliverySchemaCatalogProductionDecodeLoadsOnlyOnce(t *testing.T) {
	if os.Getenv(schemaLazyCatalogChildEnv) == "1" {
		if got := RuntimeSchemaMetadataLoadCounts().Catalog; got != 0 {
			t.Fatalf("Catalog loaded during package init: %d", got)
		}

		var wait sync.WaitGroup
		for range 32 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_ = deliverySchemaCatalogError()
			}()
		}
		wait.Wait()

		if got := RuntimeSchemaMetadataLoadCounts().Catalog; got != 1 {
			t.Fatalf("Catalog lazy load count = %d, want 1", got)
		}
		_ = deliverySchemaCatalogError()
		if got := RuntimeSchemaMetadataLoadCounts().Catalog; got != 1 {
			t.Fatalf("Catalog reload count = %d, want 1", got)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestDeliverySchemaCatalogProductionDecodeLoadsOnlyOnce$", "-test.count=1")
	command.Env = append(os.Environ(), schemaLazyCatalogChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("lazy Catalog child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}
