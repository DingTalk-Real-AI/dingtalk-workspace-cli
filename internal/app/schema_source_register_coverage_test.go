// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
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
