// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func TestCrossPlatformCoverageBinaryIdentityRejectsUnsupportedAndUnboundedValues(t *testing.T) {
	digest := strings.Repeat("a", 64)
	valid := RawIdentity{Edition: "open", SourceSHA256: digest, SurfaceSHA256: digest, BuildID: digest, MetaSHA256: digest, RegistrySHA256: digest,
		MetaLength:     strconv.FormatUint(schemacache.MaxMetaFileSize-schemacache.HeaderSize, 10),
		RegistryLength: strconv.FormatUint(schemacache.MaxRegistryPayloadSize, 10)}
	identity, err := ParseIdentity(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RawIdentity){
		func(i *RawIdentity) { i.Edition = "../open" },
		func(i *RawIdentity) { i.BuildID = strings.Repeat("0", 64) },
		func(i *RawIdentity) { i.MetaSHA256 = strings.Repeat("0", 64) },
		func(i *RawIdentity) {
			i.MetaLength = strconv.FormatUint(schemacache.MaxMetaFileSize-schemacache.HeaderSize+1, 10)
		},
		func(i *RawIdentity) { i.RegistryLength = strconv.FormatUint(schemacache.MaxRegistryPayloadSize+1, 10) },
		func(i *RawIdentity) { i.RegistryLength = "18446744073709551615" },
	} {
		raw := valid
		change(&raw)
		if _, err := ParseIdentity(raw); err == nil {
			t.Fatalf("unsafe pinned identity accepted: %#v", raw)
		}
	}
	for _, change := range []func(*Identity){
		func(i *Identity) { i.CatalogSnapshotVersion = CatalogSnapshotVersion + 1 },
		func(i *Identity) { i.Meta.Kind = schemacache.KindRegistry },
		func(i *Identity) { i.Registry.Codec = 1 },
	} {
		candidate := identity
		change(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatal("unsupported identity accepted")
		}
	}
}
