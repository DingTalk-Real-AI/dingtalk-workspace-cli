// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemafastpath"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// The official core entry point already owns the pre-execution identity
// snapshot and clitrack lifecycle. A plain, authenticated Schema hit may avoid
// command-tree construction without suppressing or duplicating that lifecycle.
func prepareSchemaFastPath(args, environment []string) (schemafastpath.Prepared, bool) {
	if len(args) < 2 || args[1] != "schema" {
		return schemafastpath.Prepared{}, false
	}
	options, ok := productionSchemaCacheOptions()
	if !ok || !options.Enabled || options.RuntimeEligible == nil || !options.RuntimeEligible() {
		return schemafastpath.Prepared{}, false
	}
	hooks := edition.Get()
	if hooks.IsEmbedded || hooks.ConfigDir != nil || hooks.AfterPersistentPreRun != nil {
		// Host configuration and execution hooks are owned by the normal root.
		// Do not infer their behavior from an otherwise matching Schema identity.
		return schemafastpath.Prepared{}, false
	}
	registerSchemaRuntimeDelivery()
	identity, ok := cli.SchemaCacheFastPathIdentity()
	if !ok || identity != options.Identity {
		return schemafastpath.Prepared{}, false
	}
	// Avoid reading the process environment for ordinary commands or disabled
	// development builds. Tests may provide an explicit invocation snapshot.
	if environment == nil {
		environment = os.Environ()
	}
	return schemafastpath.Prepare(options.Identity.Edition, &options.Identity, schemafastpath.Dependencies{
		Args: args, Environment: environment, Lstat: os.Lstat,
		OpenCache: func(edition string) (*schemacache.Cache, error) { return schemacache.Open(edition) },
	})
}
