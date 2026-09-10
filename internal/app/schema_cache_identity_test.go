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

func TestCrossPlatformCoverageProductionSchemaCacheEnablesOnWindows(t *testing.T) {
	oldOS, oldArch := schemaCacheGOOS, schemaCacheGOARCH
	t.Cleanup(func() {
		schemaCacheGOOS, schemaCacheGOARCH = oldOS, oldArch
		_ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{})
	})
	t.Setenv(schemaCacheTestEnv, "1")
	for _, target := range []struct{ goos, goarch string }{
		{"windows", "amd64"}, {"windows", "arm64"},
	} {
		schemaCacheGOOS, schemaCacheGOARCH = target.goos, target.goarch
		opts, ok := productionSchemaCacheOptions()
		if !ok || !opts.Enabled || !opts.AllowGenerate {
			t.Fatalf("%s/%s options = %#v ok=%v", target.goos, target.goarch, opts, ok)
		}
		if opts.GOOS != target.goos || opts.GOARCH != target.goarch {
			t.Fatalf("%s/%s target = %s/%s", target.goos, target.goarch, opts.GOOS, opts.GOARCH)
		}
		if err := cli.RegisterSchemaCacheOptions(opts); err != nil {
			t.Fatalf("%s/%s register: %v", target.goos, target.goarch, err)
		}
	}
	schemaCacheGOOS, schemaCacheGOARCH = "windows", "386"
	if opts, ok := productionSchemaCacheOptions(); ok {
		t.Fatalf("windows/386 must stay disabled: %#v", opts)
	}
}

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
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	isolateSchemaCacheHome(t)
	t.Setenv(schemaCacheTestEnv, "1")
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
	if runtime.GOOS == "windows" {
		cacheBase = filepath.Join(testHome, "AppData", "Local")
		t.Setenv("LOCALAPPDATA", cacheBase)
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
	sidecar := filepath.Join(cacheDir, cli.LocalSchemaCacheIdentityFileName())
	if info, err := os.Stat(sidecar); err != nil || info.Size() == 0 {
		t.Fatalf("missing local identity sidecar %s: info=%v err=%v", sidecar, info, err)
	}
	loaded, ok := cli.TryLoadLocalSchemaCacheIdentity(identity.Edition)
	if !ok || loaded.BuildID != identity.BuildID {
		t.Fatalf("loaded local identity = %#v ready=%v", loaded, ok)
	}
}

func TestCrossPlatformCoverageSchemaCacheUpgradeInvalidationABRegression(t *testing.T) {
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	isolateSchemaCacheHome(t)
	t.Setenv(schemaCacheTestEnv, "1")
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })

	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	meta, ok := cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("seed ResolveMeta = %#v, %v", meta, ok)
	}
	identity, ok := cli.SchemaCacheFastPathIdentity()
	if !ok {
		t.Fatal("seed identity missing")
	}
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(cache.Directory(), cli.LocalSchemaCacheIdentityFileName())
	_ = cache.Close()
	if _, err := os.Stat(identityPath); err != nil {
		t.Fatalf("seed identity.json missing: %v", err)
	}

	// Without invalidation, a reloaded process would keep serving A's sidecar.
	cli.RestorePackageCLISchemaDeliveryForTest()
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	if hit, hitOK := cli.SchemaCacheFastPathIdentity(); !hitOK || hit.BuildID != identity.BuildID {
		t.Fatalf("persisted A identity not reloaded: %#v ready=%v", hit, hitOK)
	}

	// Upgrade invalidation drops the sidecar; binary B regenerates.
	cli.InvalidatePersistedSchemaCacheIdentities()
	if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
		t.Fatalf("identity.json survived upgrade invalidation: %v", err)
	}
	cli.RestorePackageCLISchemaDeliveryForTest()
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	if _, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatal("fast path still active after upgrade invalidation")
	}
	meta, ok = cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("post-upgrade ResolveMeta = %#v, %v", meta, ok)
	}
	if _, ok := cli.SchemaCacheFastPathIdentity(); !ok {
		t.Fatal("binary B did not regenerate Schema identity after upgrade")
	}
}
