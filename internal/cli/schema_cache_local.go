// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

const (
	localSchemaCacheIdentityVersion = 1
	schemaCacheFingerprintEnv       = "DWS_SCHEMA_CACHE_FINGERPRINT"
)

type localSchemaCacheIdentityRecord struct {
	Version            int    `json:"version"`
	Fingerprint        string `json:"fingerprint"`
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

var schemaCacheExecutable = os.Executable

// SchemaCacheBinaryFingerprint identifies the running executable so a persisted
// local identity cannot be reused by a different binary. Tests may pin the
// value with DWS_SCHEMA_CACHE_FINGERPRINT.
func SchemaCacheBinaryFingerprint() string {
	if override := strings.TrimSpace(os.Getenv(schemaCacheFingerprintEnv)); override != "" {
		return override
	}
	exe, err := schemaCacheExecutable()
	if err != nil {
		exe = "unknown-executable"
	}
	var canonical strings.Builder
	canonical.WriteString(exe)
	if info, statErr := os.Stat(exe); statErr == nil {
		fmt.Fprintf(&canonical, "\x00%d\x00%d", info.Size(), info.ModTime().UnixNano())
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		canonical.WriteByte(0)
		canonical.WriteString(bi.GoVersion)
		canonical.WriteByte(0)
		canonical.WriteString(bi.Main.Path)
		canonical.WriteByte(0)
		canonical.WriteString(bi.Main.Version)
		canonical.WriteByte(0)
		canonical.WriteString(bi.Main.Sum)
		for _, setting := range bi.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.time" || setting.Key == "vcs.modified" {
				canonical.WriteByte(0)
				canonical.WriteString(setting.Key)
				canonical.WriteByte('=')
				canonical.WriteString(setting.Value)
			}
		}
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:])
}

// LocalSchemaCacheIdentityFileName is the authenticated identity sidecar stored
// next to protobuf shards. The fingerprint binds the record to this binary.
func LocalSchemaCacheIdentityFileName(fingerprint string) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		fingerprint = "unknown"
	}
	return "identity." + fingerprint + ".json"
}

// TryLoadLocalSchemaCacheIdentity reads the identity sidecar for this binary
// from the edition cache directory. Missing files are a miss, not an error.
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
	fingerprint := SchemaCacheBinaryFingerprint()
	payload, err := os.ReadFile(filepath.Join(directory, LocalSchemaCacheIdentityFileName(fingerprint)))
	if err != nil {
		return SchemaCacheIdentity{}, err
	}
	var record localSchemaCacheIdentityRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return SchemaCacheIdentity{}, err
	}
	if record.Version != localSchemaCacheIdentityVersion || record.Fingerprint != fingerprint {
		return SchemaCacheIdentity{}, fmt.Errorf("schema cache identity sidecar does not match this binary")
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
	fingerprint := SchemaCacheBinaryFingerprint()
	raw := identityToRaw(identity)
	record := localSchemaCacheIdentityRecord{
		Version:            localSchemaCacheIdentityVersion,
		Fingerprint:        fingerprint,
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
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	finalName := LocalSchemaCacheIdentityFileName(fingerprint)
	staging, err := os.CreateTemp(directory, ".identity-*.tmp")
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
	return os.Rename(stagingPath, filepath.Join(directory, finalName))
}
