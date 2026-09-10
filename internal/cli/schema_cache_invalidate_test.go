// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func TestCrossPlatformCoverageInvalidateSchemaCacheIdentitiesScopedToDWSSchema(t *testing.T) {
	root := t.TempDir()
	schemaTree := filepath.Join(root, "dws", "schema", "abcd", "v1")
	foreignTree := filepath.Join(root, "other-app")
	if err := os.MkdirAll(schemaTree, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(foreignTree, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(foreignTree, "identity.json")
	drop := filepath.Join(schemaTree, "identity.json")
	legacy := filepath.Join(schemaTree, "identity.old.json")
	wide := filepath.Join(root, "identity.json")
	for _, p := range []string{keep, drop, legacy, wide} {
		if err := os.WriteFile(p, []byte("{\"version\":1}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	oldGOOS, oldUser := schemaCacheRuntimeGOOS, schemaCacheUserCacheDir
	schemaCacheUserCacheDir = func() (string, error) { return root, nil }
	t.Cleanup(func() {
		schemaCacheRuntimeGOOS, schemaCacheUserCacheDir = oldGOOS, oldUser
	})
	t.Setenv("DWS_SCHEMA_CACHE_DIR", root)
	t.Setenv("DWS_SCHEMA_CACHE_SHARED_DIR", "")
	t.Setenv("ProgramData", filepath.Join(root, "ProgramData"))

	for _, goos := range []string{"linux", "darwin", "windows", runtime.GOOS, "plan9"} {
		schemaCacheRuntimeGOOS = func() string { return goos }
		_ = schemaCacheInvalidationBases()
	}
	schemaCacheRuntimeGOOS = func() string { return runtime.GOOS }

	InvalidatePersistedSchemaCacheIdentities()

	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("dws/schema identity.json remained: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy identity sidecar remained: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("foreign identity.json was deleted: %v", err)
	}
	if _, err := os.Stat(wide); err != nil {
		t.Fatalf("wide-base identity.json was deleted: %v", err)
	}
}

func TestCrossPlatformCoverageUpgradeInvalidationClearsPersistedIdentityForABRegeneration(t *testing.T) {
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	ensureSchemaCacheOpenable(t)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	coverageSchemaCacheHome(t)
	goos, goarch := coverageCacheGOOSARCH()

	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })

	loaded := deliverySchemaCatalog()
	runtime := activeSchemaCacheRuntime()
	if runtime == nil {
		t.Fatal("runtime missing")
	}
	opened, err := runtime.opened()
	if err != nil {
		t.Fatal(err)
	}
	runtime.publishGeneratedOrMatching(opened, loaded)
	identity := runtime.optionsSnapshot().Identity
	if !schemaCacheIdentityReady(identity) {
		t.Fatal("identity A was not generated")
	}
	identityPath := filepath.Join(opened.Directory(), LocalSchemaCacheIdentityFileName())
	if _, err := os.Stat(identityPath); err != nil {
		t.Fatalf("identity.json missing after publish: %v", err)
	}

	// Simulate binary B after upgrade invalidation: sidecar cleared, AllowGenerate
	// remains true, ResolveMeta regenerates from live declarations.
	InvalidatePersistedSchemaCacheIdentities()
	if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
		t.Fatalf("upgrade invalidation left identity.json: %v", err)
	}

	restorePackageCLISchemaDeliveryForTest()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := SchemaCacheFastPathIdentity(); ok {
		t.Fatal("cleared identity must not enable fast path before regeneration")
	}
	meta, ok := ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("post-invalidation ResolveMeta = %#v ok=%v", meta, ok)
	}
	if _, ok := SchemaCacheFastPathIdentity(); !ok {
		t.Fatal("binary B did not regenerate identity after upgrade invalidation")
	}
	if _, err := os.Stat(identityPath); err != nil {
		t.Fatalf("regenerated identity.json missing: %v", err)
	}
	_ = identity
}

func TestCrossPlatformCoverageBinaryBuildIDMismatchMissesAndInvalidatesSidecar(t *testing.T) {
	dir := t.TempDir()
	oldDigest := schemaCacheBinaryDigest
	t.Cleanup(func() { schemaCacheBinaryDigest = oldDigest })

	stampA := sha256.Sum256([]byte("binary-stamp-A"))
	stampB := sha256.Sum256([]byte("binary-stamp-B"))
	schemaCacheBinaryDigest = func() [sha256.Size]byte { return stampA }

	identity := coverageSchemaCacheIdentity()
	if err := persistLocalSchemaCacheIdentity(dir, identity); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(dir, LocalSchemaCacheIdentityFileName())
	payload, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	var record localSchemaCacheIdentityRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	if record.BinaryBuildID != hex.EncodeToString(stampA[:]) {
		t.Fatalf("persisted binary_build_id = %q want stamp A", record.BinaryBuildID)
	}
	if _, err := loadLocalSchemaCacheIdentity(dir); err != nil {
		t.Fatalf("stamp A must load its own sidecar: %v", err)
	}

	// Keep cache artifacts + identity.json written by binary A; run as binary B.
	schemaCacheBinaryDigest = func() [sha256.Size]byte { return stampB }
	if _, err := loadLocalSchemaCacheIdentity(dir); err == nil {
		t.Fatal("binary B must not load binary A's identity sidecar")
	}
	if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
		t.Fatalf("mismatched identity.json should be invalidated: %v", err)
	}

	// Binary B regenerates a sidecar bound to its own stamp (not A's schema seal).
	identityB := coverageSchemaCacheIdentity()
	identityB.BuildID = sha256.Sum256([]byte("binary-B-build"))
	if err := persistLocalSchemaCacheIdentity(dir, identityB); err != nil {
		t.Fatal(err)
	}
	refreshed, err := loadLocalSchemaCacheIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.BuildID != identityB.BuildID {
		t.Fatalf("regenerated build %x want %x", refreshed.BuildID, identityB.BuildID)
	}
	payload, err = os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	if record.BinaryBuildID != hex.EncodeToString(stampB[:]) {
		t.Fatalf("regenerated binary_build_id = %q want stamp B", record.BinaryBuildID)
	}
}
