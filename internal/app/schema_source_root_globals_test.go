// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"os"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

// schemaSourceRootMarkerCaller is a recognizable ToolCaller used to detect
// InitDeps clobbering when NewSchemaSourceRootCommand builds the tree.
type schemaSourceRootMarkerCaller struct{}

func (schemaSourceRootMarkerCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{}, nil
}
func (schemaSourceRootMarkerCaller) Format() string { return "json" }
func (schemaSourceRootMarkerCaller) DryRun() bool   { return false }
func (schemaSourceRootMarkerCaller) Fields() string { return "" }
func (schemaSourceRootMarkerCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageSchemaSourceRootPreservesRuntimeProfile(t *testing.T) {
	previous := authpkg.RuntimeProfile()
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previous) })
	for _, args := range [][]string{
		{"dws", "schema"},
		{"dws", "--profile", "argv-profile", "schema"},
		{"dws", "--profile=argv-profile", "schema"},
	} {
		t.Run(args[1], func(t *testing.T) {
			testseam.Swap(t, &os.Args, args)
			const activeProfile = "active-invocation-profile"
			authpkg.SetRuntimeProfile(activeProfile)
			root := NewSchemaSourceRootCommand()
			if got := authpkg.RuntimeProfile(); got != activeProfile {
				t.Fatalf("Schema source construction changed runtime profile to %q, want %q", got, activeProfile)
			}
			resolved, err := cli.ResolveSchemaBuild(root)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.CommandCount() == 0 {
				t.Fatal("empty Schema assembly")
			}
			if got := authpkg.RuntimeProfile(); got != activeProfile {
				t.Fatalf("Schema assembly changed runtime profile to %q, want %q", got, activeProfile)
			}
		})
	}
}

// TestSchemaSourceRootPreservesRuntimeDepsAndPluginEndpoints ensures Schema
// assembly (NewSchemaSourceRootCommand / ResolveMeta delivery) does not call
// helpers.InitDeps or SetDynamicServers, which would wipe a live process's
// caller and plugin endpoints.
func TestSchemaSourceRootPreservesRuntimeDepsAndPluginEndpoints(t *testing.T) {
	const (
		pluginID       = "schema-source-root-plugin-marker"
		pluginEndpoint = "https://schema-source-root-plugin.example/mcp"
	)

	dynamicMu.RLock()
	previousEndpoints := dynamicEndpoints
	previousProducts := dynamicProducts
	previousAliases := dynamicAliases
	previousToolEndpoints := dynamicToolEndpoints
	dynamicMu.RUnlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicEndpoints = previousEndpoints
		dynamicProducts = previousProducts
		dynamicAliases = previousAliases
		dynamicToolEndpoints = previousToolEndpoints
		dynamicMu.Unlock()
	})

	marker := schemaSourceRootMarkerCaller{}
	helpers.InitDepsForTest(t, marker)
	SetDynamicServers(nil)
	AppendDynamicServer(mcptypes.ServerDescriptor{
		Key:      pluginID,
		Endpoint: pluginEndpoint,
		CLI: mcptypes.CLIOverlay{
			ID:      pluginID,
			Command: pluginID,
		},
	})

	root := NewSchemaSourceRootCommand()
	if root == nil {
		t.Fatal("NewSchemaSourceRootCommand returned nil")
	}

	if got := helpers.GetCaller(); got != marker {
		t.Fatalf("helpers.GetCaller() after NewSchemaSourceRootCommand = %T (%p), want marker caller preserved", got, got)
	}
	gotEndpoint, ok := directRuntimeEndpoint(pluginID, "")
	if !ok || gotEndpoint != pluginEndpoint {
		t.Fatalf("plugin endpoint after NewSchemaSourceRootCommand = %q ok=%v, want %q preserved", gotEndpoint, ok, pluginEndpoint)
	}

	resolved, err := cli.ResolveSchemaBuild(root)
	if err != nil {
		t.Fatalf("ResolveSchemaBuild after declaration-only root: %v", err)
	}
	if resolved.CommandCount() == 0 {
		t.Fatal("ResolveSchemaBuild command count is 0")
	}

	// Delivery path also builds NewSchemaSourceRootCommand via the factory.
	registerSchemaRuntimeDelivery()
	meta, ok := cli.ResolveMeta("dev app delete")
	if !ok || meta.Identity.Canonical == "" {
		t.Fatalf("ResolveMeta after registerSchemaRuntimeDelivery = %#v ok=%v", meta, ok)
	}
	if got := helpers.GetCaller(); got != marker {
		t.Fatalf("helpers.GetCaller() after ResolveMeta delivery = %T, want marker caller preserved", got)
	}
	gotEndpoint, ok = directRuntimeEndpoint(pluginID, "")
	if !ok || gotEndpoint != pluginEndpoint {
		t.Fatalf("plugin endpoint after ResolveMeta delivery = %q ok=%v, want %q preserved", gotEndpoint, ok, pluginEndpoint)
	}
}

// TestCrossPlatformCoverageSchemaSourceRootStaticServerVisibilityWithoutInject
// proves the declaration-only Schema source root does not need
// injectStaticServers to keep StaticServers products visible. VisibleProducts
// may be unset; StaticServers alone must still prevent
// hideNonDirectRuntimeCommands from marking those products Hidden — without
// mutating dynamic endpoints.
func TestCrossPlatformCoverageSchemaSourceRootStaticServerVisibilityWithoutInject(t *testing.T) {
	previous := edition.Get()
	t.Cleanup(func() { edition.Override(previous) })

	dynamicMu.RLock()
	previousEndpoints := dynamicEndpoints
	previousProducts := dynamicProducts
	previousAliases := dynamicAliases
	previousToolEndpoints := dynamicToolEndpoints
	dynamicMu.RUnlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicEndpoints = previousEndpoints
		dynamicProducts = previousProducts
		dynamicAliases = previousAliases
		dynamicToolEndpoints = previousToolEndpoints
		dynamicMu.Unlock()
	})

	edition.Override(&edition.Hooks{
		Name: "schema-source-root-static-visibility",
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{ID: "", Name: "ignored-empty", Endpoint: "https://schema-source-root-static.example/empty"},
				{ID: "  ", Name: "ignored-blank", Endpoint: "https://schema-source-root-static.example/blank"},
				{ID: "doc", Name: "Doc", Endpoint: "https://schema-source-root-static.example/doc"},
			}
		},
		// VisibleProducts intentionally nil: declaration-only visibility must
		// still derive from StaticServers without SetDynamicServers.
	})
	SetDynamicServers(nil)

	root := NewSchemaSourceRootCommand()
	var doc *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "doc" {
			doc = cmd
			break
		}
	}
	if doc == nil {
		t.Fatal("doc command missing from Schema source root")
	}
	if doc.Hidden {
		t.Fatal("declaration-only Schema source root hid doc despite StaticServers")
	}
	// Endpoint resolution may still read StaticServers directly; the invariant
	// is that declaration-only construction must not mutate the dynamic registry.
	dynamicMu.RLock()
	_, injected := dynamicProducts["doc"]
	dynamicMu.RUnlock()
	if injected {
		t.Fatal("declaration-only Schema source root must not inject StaticServers into dynamicProducts")
	}
}
