// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProductionSchemaCacheOptionsRemaining(t *testing.T) {
	t.Setenv(schemaCacheTestEnv, "1")
	testseam.Swap(t, &schemaCacheGOOS, "windows")
	if _, ok := productionSchemaCacheOptions(); ok {
		t.Fatal("windows persistent backend enabled")
	}

	testseam.Swap(t, &schemaCacheGOOS, runtime.GOOS)
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
