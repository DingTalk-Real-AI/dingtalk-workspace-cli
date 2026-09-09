// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSchemaCacheIdentityAndLocalRemaining(t *testing.T) {
	if got := (SchemaCacheOptions{}).cacheEdition(); got != "open" {
		t.Fatalf("empty cacheEdition = %q", got)
	}

	if err := validateSchemaCacheOptions(SchemaCacheOptions{
		AllowGenerate: true, Edition: "NOT VALID", GOOS: "linux", GOARCH: "amd64",
	}); err == nil {
		t.Fatal("invalid generated edition accepted")
	}
	if err := validateSchemaCacheOptions(SchemaCacheOptions{
		GOOS: "linux", GOARCH: "amd64", Identity: SchemaCacheIdentity{Edition: "open"},
	}); err == nil {
		t.Fatal("incomplete identity accepted")
	}

	if _, err := IdentityFromArtifacts("NOT VALID", SchemaCacheArtifacts{}); err == nil {
		t.Fatal("invalid edition identity accepted")
	}
	if _, err := IdentityFromArtifacts("open", SchemaCacheArtifacts{SourceHash: "nope"}); err == nil {
		t.Fatal("invalid source hash accepted")
	}
	badHex := "sha256:" + strings.Repeat("zz", 32)
	if _, err := IdentityFromArtifacts("open", SchemaCacheArtifacts{
		SourceHash: "sha256:" + strings.Repeat("ab", 32), SurfaceHash: badHex,
	}); err == nil {
		t.Fatal("invalid surface hash accepted")
	}
	if _, err := IdentityFromArtifacts("open", SchemaCacheArtifacts{
		SourceHash: "sha256:" + strings.Repeat("ab", 32), SurfaceHash: "sha256:" + strings.Repeat("cd", 32),
		Payload: []byte{0, 0},
	}); err == nil {
		t.Fatal("short payload pins accepted")
	}
	huge := make([]byte, 8)
	huge[3] = 100
	if _, err := IdentityFromArtifacts("open", SchemaCacheArtifacts{
		SourceHash: "sha256:" + strings.Repeat("ab", 32), SurfaceHash: "sha256:" + strings.Repeat("cd", 32),
		Payload: huge,
	}); err == nil {
		t.Fatal("overflow payload pins accepted")
	}

	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &marshalSchemaCacheFileDescriptor, func() ([]byte, error) {
		return nil, errors.New("forced descriptor marshal")
	})
	if _, err := IdentityFromArtifacts("open", artifacts); err == nil {
		t.Fatal("forced descriptor marshal succeeded")
	}

	testseam.Swap(t, &marshalSchemaCacheFileDescriptor, marshalSchemaCacheFileDescriptorDefault)
	testseam.Swap(t, &readSchemaCacheBuildInfo, func() (*debug.BuildInfo, bool) { return nil, false })
	if _, err := IdentityFromArtifacts("open", artifacts); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &readSchemaCacheBuildInfo, func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Deps: []*debug.Module{{Path: "google.golang.org/protobuf", Version: "v1.36.11"}}}, true
	})
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if schemaCacheIdentityAbsent(identity) {
		t.Fatal("valid identity reported absent")
	}

	if LocalSchemaCacheIdentityFileName("  ") != "identity.unknown.json" {
		t.Fatal("empty fingerprint file name")
	}
	ensureSchemaCacheOpenable(t)
	if _, ok := TryLoadLocalSchemaCacheIdentity("  "); ok {
		t.Fatal("blank edition loaded")
	}
	coverageSchemaCacheHome(t)
	if _, ok := TryLoadLocalSchemaCacheIdentity("open"); ok {
		t.Fatal("missing cache loaded")
	}
	created, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := TryLoadLocalSchemaCacheIdentity("open"); ok {
		t.Fatal("empty cache sidecar loaded")
	}

	t.Setenv(schemaCacheFingerprintEnv, "")
	if exe, exeErr := os.Executable(); exeErr == nil {
		testseam.Swap(t, &schemaCacheExecutable, func() (string, error) { return exe, nil })
		if SchemaCacheBinaryFingerprint() == "" {
			t.Fatal("empty fingerprint from real executable")
		}
	}
	testseam.Swap(t, &schemaCacheExecutable, func() (string, error) { return "", errors.New("no exe") })
	testseam.Swap(t, &readSchemaCacheBuildInfo, func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.25",
			Main:      debug.Module{Path: "example.com/mod", Version: "v0.0.0", Sum: "h1:x"},
			Settings:  []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.time", Value: "t"}, {Key: "vcs.modified", Value: "true"}},
		}, true
	})
	if SchemaCacheBinaryFingerprint() == "" {
		t.Fatal("empty fingerprint")
	}

	dir := t.TempDir()
	if _, err := loadLocalSchemaCacheIdentity(dir); err == nil {
		t.Fatal("missing sidecar loaded")
	}
	t.Setenv(schemaCacheFingerprintEnv, "coverage-sidecar")
	name := filepath.Join(dir, LocalSchemaCacheIdentityFileName(SchemaCacheBinaryFingerprint()))
	if err := os.WriteFile(name, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLocalSchemaCacheIdentity(dir); err == nil {
		t.Fatal("garbage sidecar loaded")
	}
	if err := os.WriteFile(name, []byte(`{"version":99,"fingerprint":"coverage-sidecar"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLocalSchemaCacheIdentity(dir); err == nil {
		t.Fatal("fingerprint mismatch sidecar loaded")
	}
	raw := identityToRaw(identity)
	record := localSchemaCacheIdentityRecord{
		Version: localSchemaCacheIdentityVersion, Fingerprint: SchemaCacheBinaryFingerprint(),
		Edition: raw.Edition, SourceSHA256: "nope", SurfaceSHA256: raw.SurfaceSHA256, BuildID: raw.BuildID,
		MetaLength: raw.MetaLength, MetaSHA256: raw.MetaSHA256, RegistryLength: raw.RegistryLength,
		RegistrySHA256: raw.RegistrySHA256, PayloadLength: raw.PayloadLength, PayloadSHA256: raw.PayloadSHA256,
		PayloadIndexLength: raw.PayloadIndexLength, PayloadIndexSHA256: raw.PayloadIndexSHA256,
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLocalSchemaCacheIdentity(dir); err == nil {
		t.Fatal("invalid parsed sidecar loaded")
	}

	if err := persistLocalSchemaCacheIdentity(dir, SchemaCacheIdentity{}); err == nil {
		t.Fatal("invalid persist succeeded")
	}
	if err := persistLocalSchemaCacheIdentity(dir, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLocalSchemaCacheIdentity(dir); err != nil {
		t.Fatal(err)
	}
	opened, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	if err := persistLocalSchemaCacheIdentity(opened.Directory(), identity); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if loaded, ok := TryLoadLocalSchemaCacheIdentity("open"); !ok || loaded.Edition != identity.Edition {
		t.Fatalf("TryLoad after persist = %#v ok=%v", loaded, ok)
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(string, string) (localIdentityTempFile, error) {
		return nil, errors.New("create")
	})
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("create persist succeeded")
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(dir, pattern string) (localIdentityTempFile, error) {
		return os.CreateTemp(dir, pattern)
	})
	testseam.Swap(t, &schemaCacheJSONMarshal, func(any) ([]byte, error) { return nil, errors.New("forced json") })
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("forced json persist succeeded")
	}

	tmpName := filepath.Join(dir, "sidecar.tmp")
	testseam.Swap(t, &schemaCacheJSONMarshal, json.Marshal)
	testseam.Swap(t, &createLocalIdentityTempFile, func(string, string) (localIdentityTempFile, error) {
		return failLocalIdentityTemp{name: tmpName, chmod: errors.New("chmod")}, nil
	})
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("chmod persist succeeded")
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(string, string) (localIdentityTempFile, error) {
		return failLocalIdentityTemp{name: tmpName, write: errors.New("write")}, nil
	})
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("write persist succeeded")
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(string, string) (localIdentityTempFile, error) {
		return failLocalIdentityTemp{name: tmpName, sync: errors.New("sync")}, nil
	})
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("sync persist succeeded")
	}
	testseam.Swap(t, &createLocalIdentityTempFile, func(string, string) (localIdentityTempFile, error) {
		return failLocalIdentityTemp{name: tmpName, close: errors.New("close")}, nil
	})
	if err := persistLocalSchemaCacheIdentity(dir, identity); err == nil {
		t.Fatal("close persist succeeded")
	}
}

func TestCrossPlatformCoverageSchemaCachePublishGeneratedRemaining(t *testing.T) {
	ensureSchemaCacheOpenable(t)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	restorePackageCLISchemaDeliveryForTest()
	r := &schemaCacheRuntime{options: SchemaCacheOptions{AllowGenerate: true, Edition: "open"}}
	r.publishGeneratedOrMatching(nil, loadedSchemaCatalog{})
	r.publishGeneratedOrMatching(&schemacache.Cache{}, loadedSchemaCatalog{})

	loaded := deliverySchemaCatalog()
	coverageSchemaCacheHome(t)
	goos, goarch := coverageCacheGOOSARCH()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, AllowGenerate: true, Edition: "open", GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	registered := activeSchemaCacheRuntime()
	if registered == nil {
		t.Fatal("allow-generate runtime missing")
	}
	cache, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	registered.publishGeneratedOrMatching(cache, loaded)
	if !schemaCacheIdentityReady(registered.options.Identity) {
		t.Fatal("generated identity was not adopted")
	}

	r.options.Edition = "NOT VALID"
	r.publishGeneratedOrMatching(cache, loaded)

	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	matching := coverageIdentityFromArtifacts(t, artifacts)
	matching.Edition = ""
	matching.BuildID = [sha256.Size]byte{}
	r.options.Identity = matching
	r.publishGeneratedOrMatching(cache, loaded)
}

type failLocalIdentityTemp struct {
	name  string
	chmod error
	write error
	sync  error
	close error
}

func (f failLocalIdentityTemp) Chmod(os.FileMode) error { return f.chmod }
func (f failLocalIdentityTemp) Write(p []byte) (int, error) {
	if f.write != nil {
		return 0, f.write
	}
	return len(p), nil
}
func (f failLocalIdentityTemp) Sync() error  { return f.sync }
func (f failLocalIdentityTemp) Close() error { return f.close }
func (f failLocalIdentityTemp) Name() string { return f.name }
