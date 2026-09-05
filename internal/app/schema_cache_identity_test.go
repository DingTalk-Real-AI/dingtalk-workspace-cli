// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestProductionSchemaCacheIdentityParsingFailsClosed(t *testing.T) {
	restore := saveSchemaCacheBuildVars()
	t.Cleanup(restore)
	valid := strings.Repeat("a", 64)
	schemaCacheEdition = "open"
	schemaCacheSourceSHA256 = valid
	schemaCacheSurfaceSHA256 = valid
	schemaCacheBuildID = valid
	schemaCacheMetaLength = "123"
	schemaCacheMetaSHA256 = valid
	schemaCacheRegistryLength = "456"
	schemaCacheRegistrySHA256 = valid
	schemaCacheGOOS, schemaCacheGOARCH = "linux", "amd64"

	options, ok := productionSchemaCacheOptions()
	if !ok || !options.Enabled || options.Identity.Edition != "open" || options.Identity.Meta.EncodedLength != 123 || options.Identity.Registry.EncodedLength != 456 {
		t.Fatalf("valid identity rejected: %#v, %v", options, ok)
	}

	for _, test := range []struct {
		name string
		set  func()
	}{
		{"empty", func() { schemaCacheBuildID = "" }},
		{"uppercase hex", func() { schemaCacheBuildID = strings.Repeat("A", 64) }},
		{"short hex", func() { schemaCacheBuildID = strings.Repeat("a", 63) }},
		{"signed decimal", func() { schemaCacheMetaLength = "+1" }},
		{"zero decimal", func() { schemaCacheMetaLength = "0" }},
		{"leading zero decimal", func() { schemaCacheMetaLength = "01" }},
		{"unknown target", func() { schemaCacheGOOS, schemaCacheGOARCH = "windows", "amd64" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			restoreCase := saveSchemaCacheBuildVars()
			defer restoreCase()
			test.set()
			if _, enabled := productionSchemaCacheOptions(); enabled {
				t.Fatal("invalid production identity enabled persistent cache")
			}
		})
	}
}

func TestProductionSchemaCacheRuntimeEligibility(t *testing.T) {
	restore := saveSchemaCacheBuildVars()
	t.Cleanup(restore)
	previousEdition := edition.Get()
	t.Cleanup(func() { edition.Override(previousEdition) })
	valid := strings.Repeat("b", 64)
	schemaCacheEdition = "open"
	schemaCacheSourceSHA256 = valid
	schemaCacheSurfaceSHA256 = valid
	schemaCacheBuildID = valid
	schemaCacheMetaLength = "1"
	schemaCacheMetaSHA256 = valid
	schemaCacheRegistryLength = "1"
	schemaCacheRegistrySHA256 = valid
	schemaCacheGOOS, schemaCacheGOARCH = "darwin", "arm64"
	options, ok := productionSchemaCacheOptions()
	if !ok || options.RuntimeEligible == nil || !options.RuntimeEligible() {
		t.Fatal("open release identity should be eligible")
	}
	t.Setenv(schemaCacheDisableEnv, "1")
	if options.RuntimeEligible() {
		t.Fatal("disable environment did not fail closed")
	}
	t.Setenv(schemaCacheDisableEnv, "")
	edition.Override(&edition.Hooks{Name: "internal"})
	if options.RuntimeEligible() {
		t.Fatal("edition mismatch did not fail closed")
	}
	edition.Override(&edition.Hooks{Name: "open", RegisterExtraCommands: func(_ *cobra.Command, _ edition.ToolCaller) {}})
	if options.RuntimeEligible() {
		t.Fatal("Schema-affecting overlay did not fail closed")
	}
}

// Local aliases are intentionally not used: this helper only snapshots ldflag
// variables and platform seams without touching registration Once state.
func saveSchemaCacheBuildVars() func() {
	editionValue, source, surface, buildID := schemaCacheEdition, schemaCacheSourceSHA256, schemaCacheSurfaceSHA256, schemaCacheBuildID
	metaLength, metaHash := schemaCacheMetaLength, schemaCacheMetaSHA256
	registryLength, registryHash := schemaCacheRegistryLength, schemaCacheRegistrySHA256
	goos, goarch := schemaCacheGOOS, schemaCacheGOARCH
	return func() {
		schemaCacheEdition, schemaCacheSourceSHA256, schemaCacheSurfaceSHA256, schemaCacheBuildID = editionValue, source, surface, buildID
		schemaCacheMetaLength, schemaCacheMetaSHA256 = metaLength, metaHash
		schemaCacheRegistryLength, schemaCacheRegistrySHA256 = registryLength, registryHash
		schemaCacheGOOS, schemaCacheGOARCH = goos, goarch
	}
}
