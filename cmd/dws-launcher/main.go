// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"os"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/launcher"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

var (
	schemaCacheEdition        string
	schemaCacheSourceSHA256   string
	schemaCacheSurfaceSHA256  string
	schemaCacheBuildID        string
	schemaCacheMetaLength     string
	schemaCacheMetaSHA256     string
	schemaCacheRegistryLength string
	schemaCacheRegistrySHA256 string
	version                   = "dev"
	commit                    = "unknown"
	edition                   = "open"
	coreSHA256                = "0000000000000000000000000000000000000000000000000000000000000000"
	coreSize                  = "0"
)

func main() {
	size, err := strconv.ParseInt(coreSize, 10, 64)
	if err != nil {
		size = -1
	}
	identity, identityErr := schemareader.ParseIdentity(schemareader.RawIdentity{
		Edition: schemaCacheEdition, SourceSHA256: schemaCacheSourceSHA256, SurfaceSHA256: schemaCacheSurfaceSHA256,
		BuildID: schemaCacheBuildID, MetaLength: schemaCacheMetaLength, MetaSHA256: schemaCacheMetaSHA256,
		RegistryLength: schemaCacheRegistryLength, RegistrySHA256: schemaCacheRegistrySHA256,
	})
	var schemaIdentity *schemareader.Identity
	if identityErr == nil {
		schemaIdentity = &identity
	}
	os.Exit(launcher.Main(launcher.Options{SchemaIdentity: schemaIdentity, Version: version, Commit: commit, Edition: edition, CoreSHA256: coreSHA256, CoreSize: size}))
}
