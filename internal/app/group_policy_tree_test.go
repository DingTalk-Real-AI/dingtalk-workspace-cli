// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

// TestCrossPlatformCoverageFinalCommandTreesDeclareGroupPolicy replaces the
// old helpers-only AST scan with an invariant over the two real assembly
// products: the deterministic distribution tree and a runtime tree after a
// nested plugin overlay has been merged.
func TestCrossPlatformCoverageFinalCommandTreesDeclareGroupPolicy(t *testing.T) {
	distribution := NewSchemaSourceRootCommand()
	for _, path := range []string{
		"sheet range read",
		"pat chmod",
		"plugin list",
		"chat +chat-messages",
	} {
		requireFinalCommandPath(t, distribution, path)
	}
	if err := cobracmd.ValidateGroupTree(distribution); err != nil {
		t.Fatalf("distribution command tree GroupPolicy invariant: %v", err)
	}

	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &rootLoadPlugins, func(root *cobra.Command, _ *pipeline.Engine, runner executor.Runner) []*cobra.Command {
		descriptor := conferencePluginDescriptor()
		return buildPluginCommands([]mcptypes.ServerDescriptor{descriptor}, runner, root)
	})
	runtime := NewRootCommand()
	requireFinalCommandPath(t, runtime, "conference camera open")
	if err := cobracmd.ValidateGroupTree(runtime); err != nil {
		t.Fatalf("runtime command tree GroupPolicy invariant: %v", err)
	}
}

func requireFinalCommandPath(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	command, remaining, err := root.Find(strings.Fields(path))
	if err != nil || command == nil || len(remaining) != 0 || command == root {
		t.Fatalf("final command path %q not assembled: command=%v remaining=%v err=%v", path, command, remaining, err)
	}
	return command
}
