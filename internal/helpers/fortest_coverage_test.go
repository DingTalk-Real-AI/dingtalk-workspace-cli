// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type fortestCoverageCaller struct{}

func (fortestCoverageCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{}, nil
}
func (fortestCoverageCaller) Format() string { return "json" }
func (fortestCoverageCaller) DryRun() bool   { return false }
func (fortestCoverageCaller) Fields() string { return "" }
func (fortestCoverageCaller) JQ() string     { return "" }

// TestCrossPlatformCoverageInitDepsForTestRestoresPriorDeps covers the
// ForTest deps restore helper used by declaration-only Schema probes.
func TestCrossPlatformCoverageInitDepsForTestRestoresPriorDeps(t *testing.T) {
	testseam.Protect(t, &deps)
	deps = nil

	marker := fortestCoverageCaller{}
	InitDepsForTest(t, marker)
	if got := GetCaller(); got != marker {
		t.Fatalf("GetCaller() during InitDepsForTest = %T, want marker", got)
	}
	if deps == nil {
		t.Fatal("InitDepsForTest left deps nil before test cleanup")
	}
}
