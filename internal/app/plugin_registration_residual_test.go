// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/userdef"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

type schemaSourceContextKey struct{}

func TestSchemaSourceRootPropagatesContextWithoutLoadingPlugins(t *testing.T) {
	previous := rootLoadPlugins
	t.Cleanup(func() { rootLoadPlugins = previous })

	pluginLoads := 0
	rootLoadPlugins = func(*cobra.Command, *pipeline.Engine, executor.Runner) []*cobra.Command {
		pluginLoads++
		return nil
	}
	wantContext := context.WithValue(context.Background(), schemaSourceContextKey{}, "schema")
	root := NewSchemaSourceRootCommand(wantContext)
	if root.Context() != wantContext {
		t.Fatal("Schema source root did not retain the caller context")
	}
	if pluginLoads != 0 {
		t.Fatalf("Schema source root loaded runtime plugins %d times", pluginLoads)
	}
}

func TestCollectPluginServerCandidatesSortsAndSkipsInvalidStdio(t *testing.T) {
	previousDescriptors := rootPluginDescriptors
	previousClients := rootPluginStdioClients
	previousDescriptor := rootPluginStdioDescriptor
	t.Cleanup(func() {
		rootPluginDescriptors = previousDescriptors
		rootPluginStdioClients = previousClients
		rootPluginStdioDescriptor = previousDescriptor
	})

	first := &plugin.Plugin{Manifest: plugin.Manifest{Name: "first"}}
	second := &plugin.Plugin{Manifest: plugin.Manifest{Name: "second"}}
	wantContext := &plugin.UserContext{UserID: "user", CorpID: "corp"}
	client := transport.NewStdioClient("unused", nil, nil)

	rootPluginDescriptors = func(owner *plugin.Plugin) []mcptypes.ServerDescriptor {
		if owner == first {
			return []mcptypes.ServerDescriptor{{Key: "same"}, {Key: " beta "}}
		}
		return []mcptypes.ServerDescriptor{{Key: "aardvark"}}
	}
	rootPluginStdioClients = func(owner *plugin.Plugin, gotContext *plugin.UserContext) []plugin.StdioServerClient {
		if gotContext != wantContext {
			t.Fatalf("stdio user context = %#v, want %#v", gotContext, wantContext)
		}
		if owner != first {
			return nil
		}
		return []plugin.StdioServerClient{
			{Key: "same", Client: client},
			{Key: " alpha ", Client: client},
			{Key: "invalid", Client: client},
		}
	}
	rootPluginStdioDescriptor = func(_ *plugin.Plugin, stdio plugin.StdioServerClient) (mcptypes.ServerDescriptor, bool) {
		if stdio.Key == "invalid" {
			return mcptypes.ServerDescriptor{}, false
		}
		return mcptypes.ServerDescriptor{Key: stdio.Key}, true
	}

	candidates := collectPluginServerCandidates([]*plugin.Plugin{first, second}, wantContext)
	if len(candidates) != 5 {
		t.Fatalf("candidate count = %d, want 5", len(candidates))
	}
	gotKeys := make([]string, 0, len(candidates))
	gotKinds := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		gotKeys = append(gotKeys, candidate.descriptor.Key)
		if candidate.stdioClient == nil {
			gotKinds = append(gotKinds, "http")
		} else {
			gotKinds = append(gotKinds, "stdio")
			if candidate.stdioClient.Client != client {
				t.Fatal("stdio candidate did not retain its client")
			}
		}
	}
	if want := []string{" alpha ", " beta ", "same", "same", "aardvark"}; !reflect.DeepEqual(gotKeys, want) {
		t.Fatalf("candidate keys = %#v, want %#v", gotKeys, want)
	}
	if want := []string{"stdio", "http", "http", "stdio", "http"}; !reflect.DeepEqual(gotKinds, want) {
		t.Fatalf("candidate transports = %#v, want %#v", gotKinds, want)
	}
}

func TestPluginDescriptorBlankIdentityAndDistributionOwnership(t *testing.T) {
	isolatePluginRuntime(t)

	blank := mcptypes.ServerDescriptor{
		Key: " ",
		CLI: mcptypes.CLIOverlay{
			ID:      " ",
			Command: " ",
			Aliases: []string{"", "  "},
		},
	}
	if claims := pluginDescriptorIdentityClaims(blank); len(claims) != 0 {
		t.Fatalf("blank descriptor claims = %#v, want none", claims)
	}
	if rootName := pluginDescriptorRootName(blank); rootName != "" {
		t.Fatalf("blank descriptor root = %q", rootName)
	}
	owner := &plugin.Plugin{Manifest: plugin.Manifest{Name: "blank"}}
	accepted := selectPluginServerCandidates(
		&cobra.Command{Use: "dws"},
		[]pluginServerCandidate{
			{owner: owner, descriptor: mcptypes.ServerDescriptor{CLI: mcptypes.CLIOverlay{Skip: true}}},
			{owner: owner, descriptor: blank},
		},
	)
	if len(accepted) != 1 {
		t.Fatalf("blank descriptor candidates = %#v, want one accepted candidate", accepted)
	}

	if distributionRootOwns(nil, "visible") {
		t.Fatal("nil root claimed a command")
	}
	root := &cobra.Command{Use: "dws"}
	visible := &cobra.Command{Use: "visible", Aliases: []string{" visible-alias "}}
	hiddenFallback := &cobra.Command{Use: "conference", Hidden: true}
	hiddenOwned := &cobra.Command{Use: "hidden-owned", Hidden: true}
	pluginOwned := &cobra.Command{Use: "plugin-owned", Aliases: []string{"plugin-alias"}}
	cmdutil.MarkPluginSource(pluginOwned)
	root.AddCommand(visible, hiddenFallback, hiddenOwned, pluginOwned)

	for _, name := range []string{"visible", "visible-alias", "hidden-owned"} {
		if !distributionRootOwns(root, name) {
			t.Errorf("distribution root did not claim %q", name)
		}
	}
	for _, name := range []string{"conference", "plugin-owned", "plugin-alias", "missing"} {
		if distributionRootOwns(root, name) {
			t.Errorf("distribution root unexpectedly claimed %q", name)
		}
	}
}

func TestReplaceableFallbackIdentitySurvivesDistributionConflictChecks(t *testing.T) {
	isolatePluginRuntime(t)
	SetDynamicServers([]mcptypes.ServerDescriptor{
		{
			Key:      "conference",
			Endpoint: "https://example.com/conference/mcp",
			CLI:      mcptypes.CLIOverlay{ID: "conference"},
		},
		{
			Key:      "chat",
			Endpoint: "https://example.com/chat/mcp",
			CLI:      mcptypes.CLIOverlay{ID: "chat"},
		},
	})

	root := &cobra.Command{Use: "dws"}
	root.AddCommand(&cobra.Command{Use: "conference", Hidden: true})
	distributionProducts := DirectRuntimeProductIDs()

	conferenceDescriptor := mcptypes.ServerDescriptor{
		Key:         "conference-local",
		DisplayName: "conference/conference-local",
		CLI:         mcptypes.CLIOverlay{ID: "conference-local", Command: "conference"},
	}
	if pluginDescriptorConflictsWithDistribution(root, conferenceDescriptor, distributionProducts) {
		t.Fatal("replaceable fallback identity blocked plugin server selection")
	}

	chatDescriptor := mcptypes.ServerDescriptor{
		Key:         "chat-local",
		DisplayName: "chat/chat-local",
		CLI:         mcptypes.CLIOverlay{ID: "chat-local", Command: "chat"},
	}
	if !pluginDescriptorConflictsWithDistribution(root, chatDescriptor, distributionProducts) {
		t.Fatal("non-replaceable distribution product no longer conflicts")
	}

	reservedDescriptor := mcptypes.ServerDescriptor{
		Key:         "auth-local",
		DisplayName: "auth/auth-local",
		CLI:         mcptypes.CLIOverlay{ID: "auth-local", Command: "auth"},
	}
	if !pluginDescriptorConflictsWithDistribution(root, reservedDescriptor, distributionProducts) {
		t.Fatal("reserved command name no longer conflicts")
	}

	first := &plugin.Plugin{Manifest: plugin.Manifest{Name: "conference"}}
	second := &plugin.Plugin{Manifest: plugin.Manifest{Name: "other"}}
	accepted := selectPluginServerCandidates(root, []pluginServerCandidate{
		{owner: first, descriptor: conferenceDescriptor},
		{
			owner: second,
			descriptor: mcptypes.ServerDescriptor{
				Key:         "conference-other",
				DisplayName: "other/conference-other",
				CLI:         mcptypes.CLIOverlay{ID: "conference-other", Command: "conference"},
			},
		},
	})
	if len(accepted) != 1 {
		t.Fatalf("accepted candidates = %d, want the first conference plugin only", len(accepted))
	}
	if accepted[0].owner != first {
		t.Fatalf("accepted owner = %q, want the first conference plugin", accepted[0].owner.Manifest.Name)
	}
}

func TestAddPluginCommandsSafeFiltersConflictingAliases(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(&cobra.Command{Use: "taken"})
	command := &cobra.Command{
		Use:     "extension",
		Aliases: []string{"", "extension", "auth", "taken", "shared", " shared ", " okay "},
	}
	addPluginCommandsSafe(root, []*cobra.Command{
		command,
		{Use: "shared"},
		{Use: "other", Aliases: []string{"extension"}},
	})

	if want := []string{"shared", "okay"}; !reflect.DeepEqual(command.Aliases, want) {
		t.Fatalf("filtered aliases = %#v, want %#v", command.Aliases, want)
	}
	if child := findDirectChild(root, "shared"); child != nil {
		t.Fatal("an accepted alias was also registered as a plugin primary command")
	}
	other := findDirectChild(root, "other")
	if other == nil || len(other.Aliases) != 0 {
		t.Fatalf("later plugin aliases = %#v", other)
	}
}

func TestStdioRunnerReportsToolsListFailureAndMissingTool(t *testing.T) {
	isolatePluginRuntime(t)
	previousInit := runnerStdioEnsureInitialized
	previousList := runnerStdioListTools
	previousCall := runnerStdioCallTool
	t.Cleanup(func() {
		runnerStdioEnsureInitialized = previousInit
		runnerStdioListTools = previousList
		runnerStdioCallTool = previousCall
	})

	client := transport.NewStdioClient("unused", nil, nil)
	RegisterStdioClient("plugin/server", client)
	runnerStdioEnsureInitialized = func(*transport.StdioClient, context.Context) error { return nil }
	toolCalls := 0
	runnerStdioCallTool = func(*transport.StdioClient, context.Context, string, map[string]any) (transport.ToolCallResult, error) {
		toolCalls++
		return transport.ToolCallResult{}, nil
	}
	runner := &runtimeRunner{}
	invocation := executor.Invocation{CanonicalProduct: "overlay-id", Tool: "wanted"}

	listFailure := errors.New("list failed")
	runnerStdioListTools = func(*transport.StdioClient, context.Context) (transport.ToolsListResult, error) {
		return transport.ToolsListResult{}, listFailure
	}
	_, err := runner.executeStdioInvocationAtEndpoint(context.Background(), "stdio://plugin/server", invocation)
	assertPluginRuntimeError(t, err, apperrors.CategoryAPI, "tools/list", "stdio_tools_list_error")

	runnerStdioListTools = func(*transport.StdioClient, context.Context) (transport.ToolsListResult, error) {
		return transport.ToolsListResult{Tools: []transport.ToolDescriptor{{Name: "other"}}}, nil
	}
	_, err = runner.executeStdioInvocationAtEndpoint(context.Background(), "stdio://plugin/server", invocation)
	assertPluginRuntimeError(t, err, apperrors.CategoryValidation, "", "plugin_tool_not_found")
	if toolCalls != 0 {
		t.Fatalf("tools/call attempts after tools/list failures = %d", toolCalls)
	}
}

func TestStdioManifestDescriptorAndRegistrationFailClosed(t *testing.T) {
	isolatePluginRuntime(t)
	p := &plugin.Plugin{
		Manifest: plugin.Manifest{
			Name: "broken-plugin",
			MCPServers: map[string]*plugin.MCPServer{
				"local": {CLI: json.RawMessage(`{`)},
			},
		},
	}
	server := plugin.StdioServerClient{
		Key:    "local",
		Client: transport.NewStdioClient("unused", nil, nil),
	}
	if descriptor, ok := stdioServerDescriptorFromManifest(p, server); ok || !reflect.ValueOf(descriptor).IsZero() {
		t.Fatalf("invalid descriptor = (%#v, %v), want zero, false", descriptor, ok)
	}
	if descriptor := registerStdioServerFromManifest(p, server); !reflect.ValueOf(descriptor).IsZero() {
		t.Fatalf("invalid registered descriptor = %#v, want zero", descriptor)
	}
	if _, ok := LookupStdioClient("broken-plugin/local"); ok {
		t.Fatal("invalid stdio manifest registered a client")
	}
}

func TestLegacyCommandsContinueWhenUserShortcutLoadFails(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	shortcutDir := filepath.Join(configDir, "shortcuts")
	if err := os.MkdirAll(shortcutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shortcutDir, "broken.yaml"), []byte("version: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, loadErrors := userdef.Load(); len(loadErrors) == 0 {
		t.Fatal("malformed shortcut fixture did not fail to load")
	}

	runner := executor.EchoRunner{}
	caller := newToolCallerAdapter(runner, &GlobalFlags{})
	if commands := newLegacyPublicCommands(runner, caller, true); len(commands) == 0 {
		t.Fatal("legacy commands were dropped after a user shortcut load error")
	}
}

func findDirectChild(root *cobra.Command, name string) *cobra.Command {
	for _, command := range root.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return nil
}

func assertPluginRuntimeError(
	t *testing.T,
	err error,
	wantCategory apperrors.Category,
	wantOperation string,
	wantReason string,
) {
	t.Helper()
	var appError *apperrors.Error
	if !errors.As(err, &appError) {
		t.Fatalf("runtime error = %#v, want structured app error", err)
	}
	if appError.Category != wantCategory ||
		appError.Operation != wantOperation ||
		appError.Reason != wantReason {
		t.Fatalf("runtime error = %#v, want category=%q operation=%q reason=%q", appError, wantCategory, wantOperation, wantReason)
	}
}
