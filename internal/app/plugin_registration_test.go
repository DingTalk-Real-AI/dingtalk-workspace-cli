// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
)

func isolatePluginRuntime(t *testing.T) {
	t.Helper()
	dynamicMu.Lock()
	previousEndpoints := dynamicEndpoints
	previousProducts := dynamicProducts
	previousAliases := dynamicAliases
	previousToolEndpoints := dynamicToolEndpoints
	dynamicEndpoints = nil
	dynamicProducts = nil
	dynamicAliases = nil
	dynamicToolEndpoints = nil
	dynamicMu.Unlock()

	stdioMu.Lock()
	previousStdio := stdioClients
	stdioClients = make(map[string]*transport.StdioClient)
	stdioMu.Unlock()

	pluginAuthMu.Lock()
	previousPluginAuth := pluginAuthRegistry
	pluginAuthRegistry = make(map[string]*PluginAuth)
	pluginAuthMu.Unlock()

	t.Cleanup(func() {
		StopAllStdioClients()
		dynamicMu.Lock()
		dynamicEndpoints = previousEndpoints
		dynamicProducts = previousProducts
		dynamicAliases = previousAliases
		dynamicToolEndpoints = previousToolEndpoints
		dynamicMu.Unlock()
		stdioMu.Lock()
		stdioClients = previousStdio
		stdioMu.Unlock()
		pluginAuthMu.Lock()
		pluginAuthRegistry = previousPluginAuth
		pluginAuthMu.Unlock()
	})
}

func TestRegisterPluginHTTPServerDoesNotProbeEndpoint(t *testing.T) {
	isolatePluginRuntime(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	registerPluginHTTPServer(mcptypes.ServerDescriptor{
		Key:      "offline-http",
		Endpoint: server.URL,
		CLI: mcptypes.CLIOverlay{
			ID:      "offline-http",
			Command: "offline-http",
		},
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("plugin endpoint calls during registration = %d, want 0", got)
	}
	if endpoint, ok := directRuntimeEndpoint("offline-http", ""); !ok || endpoint != server.URL {
		t.Fatalf("registered endpoint = (%q, %v), want (%q, true)", endpoint, ok, server.URL)
	}
}

func TestRegisterStdioServerFromManifestDoesNotStartProcess(t *testing.T) {
	isolatePluginRuntime(t)
	marker := t.TempDir() + "/started"
	pluginRoot := t.TempDir()
	if err := os.WriteFile(pluginRoot+"/overlay.json", []byte(`{
		"id":"local",
		"command":"lazy-stdio",
		"groups":{"health":{"description":"health checks"}},
		"toolOverrides":{"ping":{"cliName":"ping","group":"health"}}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client := transport.NewStdioClient("/bin/sh", []string{
		"-c", fmt.Sprintf("printf started > %q", marker),
	}, nil)
	p := &plugin.Plugin{
		Manifest: plugin.Manifest{
			Name:        "lazy-stdio",
			Description: "lazy stdio test",
			MCPServers: map[string]*plugin.MCPServer{
				"local": {
					Type:    "stdio",
					Command: "unused",
					CLI:     json.RawMessage(`"overlay.json"`),
				},
			},
		},
		Root: pluginRoot,
	}
	descriptor := registerStdioServerFromManifest(p, plugin.StdioServerClient{Key: "local", Client: client})
	commands := buildPluginCommands([]mcptypes.ServerDescriptor{descriptor}, executor.EchoRunner{}, nil)
	root := pluginTestRoot(commands...)
	root.SetArgs([]string{"lazy-stdio", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("lazy stdio help: %v", err)
	}
	requirePluginChild(t, commands[0], "health", "ping")

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("stdio process started during registration: stat error = %v", err)
	}
	if descriptor.Endpoint != StdioEndpoint("lazy-stdio", "local") {
		t.Fatalf("descriptor endpoint = %q", descriptor.Endpoint)
	}
	if _, ok := LookupStdioClient("lazy-stdio/local"); !ok {
		t.Fatal("stdio client was not registered for lazy execution")
	}
}
