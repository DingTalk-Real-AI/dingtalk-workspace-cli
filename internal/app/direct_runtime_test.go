package app

import "testing"

func TestDefaultPATServerDescriptorUsesBehaviorAuthorizationName(t *testing.T) {
	server := defaultPATServerDescriptor()
	if server.CLI.ID != "pat" {
		t.Fatalf("default PAT server id = %q, want pat", server.CLI.ID)
	}
	if server.DisplayName != "行为授权" {
		t.Fatalf("default PAT server display name = %q, want 行为授权", server.DisplayName)
	}
	if server.Endpoint != defaultPATMCPEndpoint {
		t.Fatalf("default PAT server endpoint = %q, want %q", server.Endpoint, defaultPATMCPEndpoint)
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
