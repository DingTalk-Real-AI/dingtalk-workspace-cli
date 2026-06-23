package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestDefaultPATServerDescriptorUsesBehaviorAuthorizationName(t *testing.T) {
	server := defaultPATServerDescriptor()
	if server.CLI.ID != "pat" {
		t.Fatalf("default PAT server id = %q, want pat", server.CLI.ID)
	}
	if server.DisplayName != "行为授权" {
		t.Fatalf("default PAT server display name = %q, want 行为授权", server.DisplayName)
	}
	if server.Endpoint != defaultPATMCPEndpoint() {
		t.Fatalf("default PAT server endpoint = %q, want %q", server.Endpoint, defaultPATMCPEndpoint())
	}
}

func TestDirectRuntimeProductIDsIncludesDefaultPAT(t *testing.T) {
	dynamicMu.Lock()
	previousProducts := dynamicProducts
	dynamicProducts = nil
	dynamicMu.Unlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicProducts = previousProducts
		dynamicMu.Unlock()
	})

	ids := DirectRuntimeProductIDs()
	if !ids["pat"] {
		t.Fatalf("DirectRuntimeProductIDs() missing default pat product: %#v", ids)
	}
}

func TestDirectRuntimeProductIDsIncludesDevappHelper(t *testing.T) {
	withCleanDynamicRegistry(t)

	ids := DirectRuntimeProductIDs()
	if !ids["devapp"] {
		t.Fatalf("DirectRuntimeProductIDs() missing devapp helper product: %#v", ids)
	}
}

func TestDirectRuntimeEndpoint_DevappEnvOverrideWithoutRegistry(t *testing.T) {
	withCleanDynamicRegistry(t)
	t.Setenv("DINGTALK_DEVAPP_MCP_URL", "https://example.test/server/devapp")

	assertEndpoint(t, "devapp", "list_dev_app", "https://example.test/server/devapp")
}

func TestDirectRuntimeEndpoint_DevappEnvOverridePreservesQuery(t *testing.T) {
	withCleanDynamicRegistry(t)
	t.Setenv("DINGTALK_DEVAPP_MCP_URL", "https://example.test/server/devapp?key=secret")

	assertEndpoint(t, "devapp", "list_dev_app", "https://example.test/server/devapp?key=secret")
}

func TestDirectRuntimeEndpoint_DevappDynamicServerDoesNotOverrideHardcoded(t *testing.T) {
	withCleanDynamicRegistry(t)
	SetDynamicServers([]market.ServerDescriptor{
		{
			Endpoint: "https://example.test/server/devapp-supplement",
			CLI: market.CLIOverlay{
				ID:      "devapp",
				Command: "devapp",
			},
		},
	})

	assertEndpoint(t, "devapp", "list_dev_app", devappMCPEndpoint())
}

func TestDirectRuntimeEndpoint_DevappEditionSupplementDoesNotOverrideHardcoded(t *testing.T) {
	withCleanDynamicRegistry(t)
	prev := edition.Get()
	edition.Override(&edition.Hooks{
		Name: "wukong",
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{
					ID:       "devapp",
					Name:     "开放平台应用管理",
					Endpoint: "https://example.test/server/devapp-edition-supplement?key=secret",
					Prefixes: []string{"devapp", "app"},
				},
			}
		},
	})
	t.Cleanup(func() { edition.Override(prev) })

	assertEndpoint(t, "devapp", "list_dev_app", devappMCPEndpoint())
}

func TestDirectRuntimeEndpoint_DevappEditionStaticDoesNotOverrideHardcoded(t *testing.T) {
	withCleanDynamicRegistry(t)
	prev := edition.Get()
	edition.Override(&edition.Hooks{
		Name: "wukong",
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{
					ID:       "devapp",
					Name:     "开放平台应用管理",
					Endpoint: "https://example.test/server/devapp-edition-static",
					Prefixes: []string{"devapp", "app"},
				},
			}
		},
	})
	t.Cleanup(func() { edition.Override(prev) })

	assertEndpoint(t, "devapp", "list_dev_app", devappMCPEndpoint())
}

func TestDirectRuntimeEndpoint_DevappEnvOverrideWinsOverEditionSupplement(t *testing.T) {
	withCleanDynamicRegistry(t)
	t.Setenv("DINGTALK_DEVAPP_MCP_URL", "https://example.test/server/devapp-env")
	prev := edition.Get()
	edition.Override(&edition.Hooks{
		Name: "wukong",
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{
				{
					ID:       "devapp",
					Name:     "开放平台应用管理",
					Endpoint: "https://example.test/server/devapp-edition-supplement",
				},
			}
		},
	})
	t.Cleanup(func() { edition.Override(prev) })

	assertEndpoint(t, "devapp", "list_dev_app", "https://example.test/server/devapp-env")
}

func TestDirectRuntimeEndpoint_DefaultPATFallbackWhenRegistryMissing(t *testing.T) {
	withCleanDynamicRegistry(t)
	assertEndpoint(t, "pat", "", defaultPATMCPEndpoint())
}

func TestDirectRuntimeEndpoint_DefaultPATFallbackUsesConfiguredMCPBaseURL(t *testing.T) {
	withCleanDynamicRegistry(t)

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "mcp_url"), []byte("http://127.0.0.1:54321/base"), 0o600); err != nil {
		t.Fatalf("WriteFile(mcp_url) error = %v", err)
	}
	t.Setenv("DWS_CONFIG_DIR", tmpDir)

	assertEndpoint(t, "pat", "", "http://127.0.0.1:54321/base/server/"+defaultPATServerID)
}

func TestDirectRuntimeEndpoint_PATDiscoveryOverrideWinsOverBuiltInFallback(t *testing.T) {
	withCleanDynamicRegistry(t)
	customEndpoint := "https://example.com/server/custom-pat"
	SetDynamicServers([]market.ServerDescriptor{
		{
			Endpoint: customEndpoint,
			CLI: market.CLIOverlay{
				ID:      "pat",
				Command: "pat",
			},
		},
	})
	assertEndpoint(t, "pat", "", customEndpoint)
}

func TestNormalizeDirectRuntimeProductIDPreservesLegacyHiddenVendorRouting(t *testing.T) {
	dynamicMu.Lock()
	previousAliases := dynamicAliases
	dynamicAliases = nil
	dynamicMu.Unlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicAliases = previousAliases
		dynamicMu.Unlock()
	})

	cases := map[string]string{
		"tb":                       "teambition",
		"dingtalk-discovery":       "discovery",
		"dingtalk-oa-plus":         "oa",
		"dingtalk-ai-sincere-hire": "ai-sincere-hire",
	}

	for input, want := range cases {
		if got := normalizeDirectRuntimeProductID(input); got != want {
			t.Fatalf("normalizeDirectRuntimeProductID(%q) = %q, want %q", input, got, want)
		}
	}
}
