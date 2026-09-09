// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProductionSchemaCacheIsNotCompileTimeEnabled(t *testing.T) {
	registerSchemaRuntimeDelivery()
	if identity, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatalf("production registered compile-time Schema cache identity: %#v", identity)
	}
	root := NewRootCommand()
	if root == nil {
		t.Fatal("NewRootCommand returned nil")
	}
	if identity, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatalf("root construction enabled compile-time Schema cache identity: %#v", identity)
	}
}

func TestCrossPlatformCoverageSchemaCacheLocalGenerateWriteHitCorruptRepair(t *testing.T) {
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	isolateSchemaCacheHome(t)
	t.Setenv(schemaCacheTestEnv, "1")
	t.Setenv("DWS_SCHEMA_CACHE_FINGERPRINT", "coverage-local-generate")
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })

	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	if _, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatal("empty local identity must generate on first schema use, not at registration")
	}

	meta, ok := cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("generate-path ResolveMeta = %#v, %v", meta, ok)
	}
	identity, ok := cli.SchemaCacheFastPathIdentity()
	if !ok {
		t.Fatal("first schema use did not adopt a generated identity")
	}
	live, err := cli.DeliverySchemaCacheArtifactsForTest()
	if err != nil {
		t.Fatal(err)
	}
	fromLive, err := cli.IdentityFromArtifacts(identity.Edition, live)
	if err != nil {
		t.Fatal(err)
	}
	if fromLive.BuildID != identity.BuildID {
		t.Fatalf("local identity drifted from live artifacts: generated=%x live=%x", identity.BuildID, fromLive.BuildID)
	}
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := cache.Directory()
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	assertSchemaCacheArtifactsPresent(t, cacheDir, identity)

	cli.RestorePackageCLISchemaDeliveryForTest()
	var factoryCalls atomic.Int64
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	hit, hitOK := cli.SchemaCacheFastPathIdentity()
	if !hitOK || hit.BuildID != identity.BuildID {
		t.Fatalf("reloaded local identity = %#v ready=%v", hit, hitOK)
	}
	meta, ok = cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("cache-hit ResolveMeta = %#v, %v", meta, ok)
	}
	if factoryCalls.Load() != 0 {
		t.Fatalf("cache-hit ResolveMeta invoked Cobra factory %d times", factoryCalls.Load())
	}

	if err := os.WriteFile(filepath.Join(cacheDir, "payloads.shards.cache"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	factoryCalls.Store(0)
	cli.RestorePackageCLISchemaDeliveryForTest()
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	meta, ok = cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("corrupt-repair ResolveMeta = %#v, %v", meta, ok)
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("corrupt-cache repair assembled %d times, want 1", factoryCalls.Load())
	}
	assertSchemaCacheArtifactsPresent(t, cacheDir, identity)
}

func isolateSchemaCacheHome(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	testHome, err := os.MkdirTemp(home, ".dws-schema-cache-prod-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(testHome) })
	t.Setenv("HOME", testHome)
	cacheBase := filepath.Join(testHome, ".cache")
	if runtime.GOOS == "darwin" {
		cacheBase = filepath.Join(testHome, "Library", "Caches")
	}
	if err := os.MkdirAll(cacheBase, 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", cacheBase)
	}
	// Pin the cache location so a leftover /var/cache/dws or
	// /Library/Caches/dws on the host cannot intercept generate/hit/repair.
	t.Setenv("DWS_SCHEMA_CACHE_DIR", cacheBase)
}

func assertSchemaCacheArtifactsPresent(t *testing.T, cacheDir string, identity cli.SchemaCacheIdentity) {
	t.Helper()
	for _, name := range []string{"meta.cache", "registry.shards.cache", "payloads.shards.cache"} {
		info, err := os.Stat(filepath.Join(cacheDir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing schema cache artifact %s: info=%v err=%v", name, info, err)
		}
	}
	sidecar := filepath.Join(cacheDir, cli.LocalSchemaCacheIdentityFileName(cli.SchemaCacheBinaryFingerprint()))
	if info, err := os.Stat(sidecar); err != nil || info.Size() == 0 {
		t.Fatalf("missing local identity sidecar %s: info=%v err=%v", sidecar, info, err)
	}
	loaded, ok := cli.TryLoadLocalSchemaCacheIdentity(identity.Edition)
	if !ok || loaded.BuildID != identity.BuildID {
		t.Fatalf("loaded local identity = %#v ready=%v", loaded, ok)
	}
}
