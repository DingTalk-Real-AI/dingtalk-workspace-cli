// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProductionSchemaCacheOptionsRemaining(t *testing.T) {
	t.Setenv(schemaCacheTestEnv, "1")
	testseam.Swap(t, &schemaCacheGOOS, "windows")
	testseam.Swap(t, &schemaCacheGOARCH, "386")
	if _, ok := productionSchemaCacheOptions(); ok {
		t.Fatal("windows/386 persistent backend enabled")
	}

	testseam.Swap(t, &schemaCacheGOOS, "linux")
	testseam.Swap(t, &schemaCacheGOARCH, "amd64")
	t.Setenv(schemaCacheDisableEnv, "1")
	options, ok := productionSchemaCacheOptions()
	if !ok {
		t.Fatal("supported backend missing options")
	}
	if options.RuntimeEligible() {
		t.Fatal("DWS_SCHEMA_CACHE_DISABLE still eligible")
	}

	t.Setenv(schemaCacheDisableEnv, "")
	previous := edition.Get()
	edition.Override(&edition.Hooks{
		Name: previous.Name,
		RegisterExtraCommands: func(*cobra.Command, edition.ToolCaller) {
		},
	})
	t.Cleanup(func() { edition.Override(previous) })
	options, ok = productionSchemaCacheOptions()
	if !ok {
		t.Fatal("plugin edition missing options")
	}
	if options.RuntimeEligible() {
		t.Fatal("RegisterExtraCommands edition still eligible")
	}
}

func TestCrossPlatformCoverageApplyProductionSchemaCachePrewarm(t *testing.T) {
	t.Setenv(schemaCacheTestEnv, "1")
	t.Setenv(schemaCacheDisableEnv, "")
	testseam.Swap(t, &schemaCacheGOOS, "linux")
	testseam.Swap(t, &schemaCacheGOARCH, "amd64")
	schemacache.UseMemoryOpenForTest(t)
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })

	editionName := "open"
	if hooks := edition.Get(); hooks != nil && strings.TrimSpace(hooks.Name) != "" {
		editionName = hooks.Name
	}
	digest := sha256.Sum256([]byte("app-prewarm-identity"))
	hexDigest := hex.EncodeToString(digest[:])
	cache, err := schemacache.Open(editionName)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{
		"version": 1, "edition": editionName,
		"source_sha256": hexDigest, "surface_sha256": hexDigest, "build_id": hexDigest,
		"meta_length": "1", "meta_sha256": hexDigest,
		"registry_length": "1", "registry_sha256": hexDigest,
		"payload_length": "1", "payload_sha256": hexDigest,
		"payload_index_length": "1", "payload_index_sha256": hexDigest,
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(cache.Directory(), cli.LocalSchemaCacheIdentityFileName())
	if err := os.WriteFile(name, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	applyProductionSchemaCache()
	if _, ok := cli.SchemaCacheFastPathIdentity(); !ok {
		t.Fatal("local sidecar did not become fast-path identity")
	}
	cli.AwaitSchemaCachePrewarmForTest()
}
