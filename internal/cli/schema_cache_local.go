// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

const (
	localSchemaCacheIdentityVersion = 1
	localSchemaCacheIdentityName    = "identity.json"
	legacyIdentitySidecarGlob       = "identity.*.json"
)

type localSchemaCacheIdentityRecord struct {
	Version            int    `json:"version"`
	Edition            string `json:"edition"`
	SourceSHA256       string `json:"source_sha256"`
	SurfaceSHA256      string `json:"surface_sha256"`
	BuildID            string `json:"build_id"`
	MetaLength         string `json:"meta_length"`
	MetaSHA256         string `json:"meta_sha256"`
	RegistryLength     string `json:"registry_length"`
	RegistrySHA256     string `json:"registry_sha256"`
	PayloadLength      string `json:"payload_length"`
	PayloadSHA256      string `json:"payload_sha256"`
	PayloadIndexLength string `json:"payload_index_length"`
	PayloadIndexSHA256 string `json:"payload_index_sha256"`
}

type localIdentityTempFile interface {
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

var (
	schemaCacheJSONMarshal      = json.Marshal
	createLocalIdentityTempFile = func(dir, pattern string) (localIdentityTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
	removeLegacyIdentitySidecar = os.Remove
	globLegacyIdentitySidecars  = filepath.Glob
	schemaCacheRuntimeGOOS      = func() string { return runtime.GOOS }
	schemaCacheUserCacheDir     = os.UserCacheDir
)

// LocalSchemaCacheIdentityFileName is the stable per-edition identity sidecar
// stored next to protobuf shards. Cache identity is the content hashes inside
// the record (source/surface/build_id and artifact digests), not a binary
// fingerprint and not a per-fingerprint filename.
func LocalSchemaCacheIdentityFileName() string {
	return localSchemaCacheIdentityName
}

// TryLoadLocalSchemaCacheIdentity reads the per-edition identity sidecar from
// the edition cache directory. Missing files are a miss, not an error. Leftover
// fingerprint-suffixed sidecars are ignored and never used as a lookup key.
func TryLoadLocalSchemaCacheIdentity(edition string) (SchemaCacheIdentity, bool) {
	edition = strings.TrimSpace(edition)
	if edition == "" {
		return SchemaCacheIdentity{}, false
	}
	cache, err := schemacache.Open(edition, schemacache.WithNoCreate())
	if err != nil {
		return SchemaCacheIdentity{}, false
	}
	defer cache.Close()
	identity, err := loadLocalSchemaCacheIdentity(cache.Directory())
	if err != nil {
		return SchemaCacheIdentity{}, false
	}
	return identity, true
}

func loadLocalSchemaCacheIdentity(directory string) (SchemaCacheIdentity, error) {
	payload, err := os.ReadFile(filepath.Join(directory, LocalSchemaCacheIdentityFileName()))
	if err != nil {
		return SchemaCacheIdentity{}, err
	}
	var record localSchemaCacheIdentityRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return SchemaCacheIdentity{}, err
	}
	if record.Version != localSchemaCacheIdentityVersion {
		return SchemaCacheIdentity{}, fmt.Errorf("schema cache identity sidecar version %d is unsupported", record.Version)
	}
	identity, err := schemareader.ParseIdentity(schemareader.RawIdentity{
		Edition:            record.Edition,
		SourceSHA256:       record.SourceSHA256,
		SurfaceSHA256:      record.SurfaceSHA256,
		BuildID:            record.BuildID,
		MetaLength:         record.MetaLength,
		MetaSHA256:         record.MetaSHA256,
		RegistryLength:     record.RegistryLength,
		RegistrySHA256:     record.RegistrySHA256,
		PayloadLength:      record.PayloadLength,
		PayloadSHA256:      record.PayloadSHA256,
		PayloadIndexLength: record.PayloadIndexLength,
		PayloadIndexSHA256: record.PayloadIndexSHA256,
	})
	if err != nil {
		return SchemaCacheIdentity{}, err
	}
	return identity, nil
}

func persistLocalSchemaCacheIdentity(directory string, identity SchemaCacheIdentity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	raw := identityToRaw(identity)
	record := localSchemaCacheIdentityRecord{
		Version:            localSchemaCacheIdentityVersion,
		Edition:            raw.Edition,
		SourceSHA256:       raw.SourceSHA256,
		SurfaceSHA256:      raw.SurfaceSHA256,
		BuildID:            raw.BuildID,
		MetaLength:         raw.MetaLength,
		MetaSHA256:         raw.MetaSHA256,
		RegistryLength:     raw.RegistryLength,
		RegistrySHA256:     raw.RegistrySHA256,
		PayloadLength:      raw.PayloadLength,
		PayloadSHA256:      raw.PayloadSHA256,
		PayloadIndexLength: raw.PayloadIndexLength,
		PayloadIndexSHA256: raw.PayloadIndexSHA256,
	}
	payload, err := schemaCacheJSONMarshal(record)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	staging, err := createLocalIdentityTempFile(directory, ".identity-*.tmp")
	if err != nil {
		return err
	}
	stagingPath := staging.Name()
	defer os.Remove(stagingPath)
	if err := staging.Chmod(0o600); err != nil {
		_ = staging.Close()
		return err
	}
	if _, err := staging.Write(payload); err != nil {
		_ = staging.Close()
		return err
	}
	if err := staging.Sync(); err != nil {
		_ = staging.Close()
		return err
	}
	if err := staging.Close(); err != nil {
		return err
	}
	if err := os.Rename(stagingPath, filepath.Join(directory, LocalSchemaCacheIdentityFileName())); err != nil {
		return err
	}
	removeLegacyFingerprintIdentitySidecars(directory)
	return nil
}

// removeLegacyFingerprintIdentitySidecars deletes leftover
// identity.<fingerprint>.json files. identity.json itself never matches this
// glob. Failures are ignored so a leftover cannot block a successful persist.
func removeLegacyFingerprintIdentitySidecars(directory string) {
	matches, err := globLegacyIdentitySidecars(filepath.Join(directory, legacyIdentitySidecarGlob))
	if err != nil {
		return
	}
	canonical := LocalSchemaCacheIdentityFileName()
	for _, name := range matches {
		if filepath.Base(name) == canonical {
			continue
		}
		_ = removeLegacyIdentitySidecar(name)
	}
}

// InvalidatePersistedSchemaCacheIdentities deletes identity.json and legacy
// identity.*.json files under every candidate .../dws/schema tree. It never
// recurses a wide cache base (LOCALAPPDATA, ProgramData root, /var/cache) so
// unrelated apps' identity files stay untouched. dws upgrade calls this after
// replacing the executable so binary B cannot keep serving binary A's Schema.
func InvalidatePersistedSchemaCacheIdentities() {
	for _, base := range schemaCacheInvalidationBases() {
		clearSchemaTreeIdentities(filepath.Join(base, "dws", "schema"))
	}
}

func schemaCacheInvalidationBases() []string {
	seen := make(map[string]struct{})
	var bases []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		cleaned := filepath.Clean(raw)
		if !filepath.IsAbs(cleaned) {
			return
		}
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		bases = append(bases, cleaned)
	}
	add(os.Getenv("DWS_SCHEMA_CACHE_DIR"))
	add(os.Getenv("DWS_SCHEMA_CACHE_SHARED_DIR"))
	switch schemaCacheRuntimeGOOS() {
	case "linux":
		add("/var/cache/dws")
	case "darwin":
		add("/Library/Caches/dws")
	case "windows":
		if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
			add(filepath.Join(pd, "dws"))
		}
	}
	if cache, err := schemaCacheUserCacheDir(); err == nil {
		add(cache)
	}
	return bases
}

func clearSchemaTreeIdentities(schemaTree string) {
	info, err := os.Stat(schemaTree)
	if err != nil || !info.IsDir() {
		return
	}
	// Require the precise .../dws/schema leaf pair before deleting anything.
	if filepath.Base(schemaTree) != "schema" || filepath.Base(filepath.Dir(schemaTree)) != "dws" {
		return
	}
	_ = filepath.WalkDir(schemaTree, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == localSchemaCacheIdentityName || (strings.HasPrefix(name, "identity.") && strings.HasSuffix(name, ".json")) {
			_ = os.Remove(path)
		}
		return nil
	})
}
