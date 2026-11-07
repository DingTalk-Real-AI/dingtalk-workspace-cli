// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
)

func TestCrossPlatformCoverageScopedTokenUsesLoginEndpointBeforePersistence(t *testing.T) {
	const productionEndpoint = "https://mcp-gw.dingtalk.com/server/contact?route=profile"
	for _, tc := range []struct {
		name            string
		mcpOverride     string
		registeredURL   string
		productOverride string
		pluginOwned     bool
		pluginToken     string
		wantEndpoint    string
	}{
		{name: "domestic_pre", mcpOverride: "https://pre-mcp.dingtalk.com", wantEndpoint: "https://pre-mcp-gw.dingtalk.com/server/contact?route=profile"},
		{name: "international_pre", mcpOverride: "https://pre-mcp.dingtalk.io", wantEndpoint: "https://pre-mcp-gw.dingtalk.io/server/contact?route=profile"},
		{name: "no_override", wantEndpoint: productionEndpoint},
		{name: "explicit_product_endpoint", mcpOverride: "https://pre-mcp.dingtalk.com", productOverride: "https://mcp-gw.dingtalk.com/server/explicit", wantEndpoint: "https://mcp-gw.dingtalk.com/server/explicit"},
		{name: "third_party_endpoint", mcpOverride: "https://pre-mcp.dingtalk.com", registeredURL: "https://contact.example.test/mcp", wantEndpoint: "https://contact.example.test/mcp"},
		{name: "anonymous_plugin_gateway", mcpOverride: "https://pre-mcp.dingtalk.com", pluginOwned: true, wantEndpoint: productionEndpoint},
		{name: "authenticated_plugin_gateway", mcpOverride: "https://pre-mcp.dingtalk.com", pluginOwned: true, pluginToken: "plugin-token", wantEndpoint: productionEndpoint},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", configDir)
			t.Setenv("DINGTALK_CONTACT_MCP_URL", tc.productOverride)
			t.Cleanup(authpkg.PushMCPBaseURLOverride(""))
			testseam.Swap(t, &dynamicEndpoints, map[string]string{})
			testseam.Swap(t, &dynamicProducts, map[string]bool{})
			testseam.Swap(t, &dynamicAliases, map[string]string{})
			testseam.Swap(t, &dynamicToolEndpoints, map[string]string{})
			testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
			registeredURL := tc.registeredURL
			if registeredURL == "" {
				registeredURL = productionEndpoint
			}
			// The command tree registers endpoints before auth login applies --pre-url.
			SetDynamicServers([]mcptypes.ServerDescriptor{{
				Key: "contact", Endpoint: registeredURL,
				CLI: mcptypes.CLIOverlay{ID: "contact", Command: "contact"},
			}})
			if tc.pluginOwned {
				RegisterPluginAuth("contact", &PluginAuth{Token: tc.pluginToken})
			}
			t.Cleanup(authpkg.PushMCPBaseURLOverride(tc.mcpOverride))

			previousProfile := authpkg.RuntimeProfile()
			authpkg.SetRuntimeProfile("old-profile-must-not-be-resolved")
			t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
			testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
				t.Error("scoped token call resolved the old profile")
				return AccessTokenSnapshot{}, errors.New("old profile must not be read")
			})
			testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
				t.Error("scoped token call prefetched the old token")
				return "", errors.New("old token must not be read")
			})
			calls := 0
			testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, endpoint, tool string, _ map[string]any) (transport.ToolCallResult, error) {
				calls++
				if endpoint != tc.wantEndpoint {
					t.Errorf("endpoint = %q, want %q", endpoint, tc.wantEndpoint)
				}
				if tool != "get_current_user_profile" || client.AuthToken != "new-login-token" {
					t.Error("identity lookup did not retain the requested tool and scoped token")
				}
				return transport.ToolCallResult{Content: map[string]any{"result": []any{}}}, nil
			})
			flags := &GlobalFlags{Token: "old-token"}
			runner := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}
			_, err := runner.RunWithToken(context.Background(), executor.NewHelperInvocation(
				"overlay.contact.get_current_user_profile", "contact", "get_current_user_profile", nil,
			), "new-login-token")
			if err != nil {
				t.Fatalf("RunWithToken: %v", err)
			}
			if calls != 1 {
				t.Fatalf("transport calls = %d, want 1", calls)
			}
			if authpkg.RuntimeProfile() != "old-profile-must-not-be-resolved" || flags.Token != "old-token" {
				t.Error("scoped token call changed runtime identity")
			}
			if _, err := os.Stat(filepath.Join(configDir, "mcp_url")); !os.IsNotExist(err) {
				t.Fatalf("scoped endpoint routing persisted mcp_url: %v", err)
			}
		})
	}
}
