package app

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDeapDevEndpointFollowsConfiguredMCPEnvironment(t *testing.T) {
	t.Setenv("DINGTALK_DEAP_DEV_MCP_URL", "")

	tests := []struct {
		name        string
		mcpBaseURL  string
		gatewayHost string
	}{
		{name: "production", mcpBaseURL: "https://mcp.dingtalk.com", gatewayHost: "mcp-gw.dingtalk.com"},
		{name: "pre-release", mcpBaseURL: "https://pre-mcp.dingtalk.com", gatewayHost: "pre-mcp-gw.dingtalk.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(tt.mcpBaseURL), 0o600); err != nil {
				t.Fatalf("write mcp_url: %v", err)
			}
			t.Setenv("DWS_CONFIG_DIR", configDir)

			endpoint, ok := directRuntimeEndpoint("deap-dev", "")
			if !ok {
				t.Fatal("expected deap-dev endpoint to resolve from the configured MCP environment")
			}
			parsed, err := url.Parse(endpoint)
			if err != nil {
				t.Fatalf("parse resolved endpoint: %v", err)
			}
			if parsed.Scheme != "https" || parsed.Host != tt.gatewayHost {
				t.Fatal("resolved endpoint did not use the expected environment gateway")
			}
			if parsed.Path != "/server/68e7e41374caa1336dc642bc3dd220de6f1e7077356dc0d4fc128f62d52d7d9b" {
				t.Fatal("resolved endpoint did not use the authoritative DEAP server path")
			}
			if parsed.RawQuery != "" || parsed.Fragment != "" {
				t.Fatal("resolved endpoint must not contain a query or fragment")
			}
		})
	}
}

func TestDeapDevEndpointEnvironmentOverrideStillWins(t *testing.T) {
	const override = "https://pre-mcp-gw.example.test/server/deap-dev"
	t.Setenv("DINGTALK_DEAP_DEV_MCP_URL", override)

	endpoint, ok := directRuntimeEndpoint("deap-dev", "")
	if !ok {
		t.Fatal("expected explicit deap-dev endpoint override to resolve")
	}
	if endpoint != override {
		t.Fatal("expected explicit deap-dev endpoint override to take priority")
	}
}

func TestDirectRuntimeProductIDsIncludeDeapDev(t *testing.T) {
	if !DirectRuntimeProductIDs()["deap-dev"] {
		t.Fatal("deap-dev must be reserved as a built-in direct-runtime product")
	}
}
