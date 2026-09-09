// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestCrossPlatformCoverageProductionSchemaCacheIsNotCompileTimeEnabled(t *testing.T) {
	registerSchemaRuntimeDelivery()
	if identity, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatalf("production registered compile-time Schema cache identity: %#v", identity)
	}
	root := NewRootCommand()
	if root == nil {
		t.Fatal("NewRootCommand returned nil")
	}
	if identity, ok := cli.SchemaCacheFastPathIdentity(); ok {
		t.Fatalf("root construction enabled compile-time Schema cache identity: %#v", identity)
	}
}
