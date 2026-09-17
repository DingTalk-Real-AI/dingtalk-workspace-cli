// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageRegisterSchemaRuntimeDelivery covers the production
// RegisterSchemaSourceRoot install path used by NewRootCommand.
func TestCrossPlatformCoverageRegisterSchemaRuntimeDelivery(t *testing.T) {
	registerSchemaRuntimeDelivery()
	if !cli.SchemaSourceRootRegistered() {
		t.Fatal("registerSchemaRuntimeDelivery did not install Schema source root")
	}
	meta, ok := cli.ResolveMeta("dev app delete")
	if !ok || meta.Identity.Canonical == "" {
		t.Fatalf("ResolveMeta after registerSchemaRuntimeDelivery = %#v ok=%v", meta, ok)
	}
	safety, ok := cli.SafetyForCLIPath("dev app delete")
	if !ok || safety.Effect == "" {
		t.Fatalf("SafetyForCLIPath after register = %#v ok=%v", safety, ok)
	}
	// Idempotent: Once must not panic or clear the factory.
	registerSchemaRuntimeDelivery()
	if !cli.SchemaSourceRootRegistered() {
		t.Fatal("second registerSchemaRuntimeDelivery cleared Schema source root")
	}
}

func TestCrossPlatformCoverageProductionSchemaCacheOptionsPlatformAndDisable(t *testing.T) {
	t.Setenv(schemaCacheTestEnv, "1")
	testseam.Swap(t, &schemaCacheGOOS, "windows")
	testseam.Swap(t, &schemaCacheGOARCH, "386")
	if _, ok := productionSchemaCacheOptions(); ok {
		t.Fatal("windows/386 cache options enabled")
	}
	testseam.Swap(t, &schemaCacheGOOS, "linux")
	testseam.Swap(t, &schemaCacheGOARCH, "amd64")
	t.Setenv(schemaCacheTestEnv, "1")
	t.Setenv(schemaCacheDisableEnv, "1")
	options, ok := productionSchemaCacheOptions()
	if !ok {
		t.Fatal("supported platform should still return options")
	}
	if options.RuntimeEligible == nil || options.RuntimeEligible() {
		t.Fatal("disable env still eligible")
	}
}
