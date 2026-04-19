// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestPluginAuthRegistry(t *testing.T) {
	// Clean up after test
	defer func() {
		pluginAuthMu.Lock()
		delete(pluginAuthRegistry, "test-product")
		pluginAuthMu.Unlock()
	}()

	// Initially not found
	if _, ok := LookupPluginAuth("test-product"); ok {
		t.Error("expected LookupPluginAuth to return false for unregistered product")
	}

	// Register auth credentials
	auth := &PluginAuth{
		Token:          "sk-test-token-12345",
		ExtraHeaders:   map[string]string{"X-Custom": "value"},
		TrustedDomains: []string{"api.example.com", "*.example.com"},
	}
	RegisterPluginAuth("test-product", auth)

	// Now should be found
	got, ok := LookupPluginAuth("test-product")
	if !ok {
		t.Fatal("expected LookupPluginAuth to return true after registration")
	}
	if got != auth {
		t.Error("LookupPluginAuth returned different auth instance")
	}
	if got.Token != "sk-test-token-12345" {
		t.Errorf("Token = %q, want sk-test-token-12345", got.Token)
	}
	if got.ExtraHeaders["X-Custom"] != "value" {
		t.Errorf("ExtraHeaders[X-Custom] = %q, want value", got.ExtraHeaders["X-Custom"])
	}
	if len(got.TrustedDomains) != 2 {
		t.Errorf("TrustedDomains len = %d, want 2", len(got.TrustedDomains))
	}
}

func TestPluginAuthRegistryIsolation(t *testing.T) {
	// Clean up after test
	defer func() {
		pluginAuthMu.Lock()
		delete(pluginAuthRegistry, "product-a")
		delete(pluginAuthRegistry, "product-b")
		pluginAuthMu.Unlock()
	}()

	authA := &PluginAuth{Token: "token-a"}
	authB := &PluginAuth{Token: "token-b"}

	RegisterPluginAuth("product-a", authA)
	RegisterPluginAuth("product-b", authB)

	gotA, okA := LookupPluginAuth("product-a")
	gotB, okB := LookupPluginAuth("product-b")

	if !okA || !okB {
		t.Fatal("expected both products to be registered")
	}
	if gotA.Token != "token-a" {
		t.Errorf("product-a Token = %q, want token-a", gotA.Token)
	}
	if gotB.Token != "token-b" {
		t.Errorf("product-b Token = %q, want token-b", gotB.Token)
	}
}

func TestDeriveToolCLIName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"web_search", "web-search"},
		{"maps.search_poi", "search-poi"},
		{"maps.geo", "geo"},
		{"simple", "simple"},
		{"a.b.deep_nested_name", "deep-nested-name"},
		{"already-kebab", "already-kebab"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := deriveToolCLIName(tt.input)
			if got != tt.want {
				t.Errorf("deriveToolCLIName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRegisterPluginAuthFromHeaders(t *testing.T) {
	// Clean up after test
	defer func() {
		pluginAuthMu.Lock()
		delete(pluginAuthRegistry, "test-srv")
		pluginAuthMu.Unlock()
	}()

	srv := market.ServerDescriptor{
		Key:      "test-srv",
		Endpoint: "https://api.example.com/mcp/v1",
		CLI:      market.CLIOverlay{ID: "test-srv", Command: "test-srv"},
		AuthHeaders: map[string]string{
			"Authorization": "Bearer sk-my-secret-key",
			"X-Custom":      "custom-value",
		},
	}

	registerPluginAuthFromHeaders(srv)

	auth, ok := LookupPluginAuth("test-srv")
	if !ok {
		t.Fatal("expected auth to be registered after registerPluginAuthFromHeaders")
	}
	if auth.Token != "sk-my-secret-key" {
		t.Errorf("Token = %q, want sk-my-secret-key", auth.Token)
	}
	if auth.ExtraHeaders["X-Custom"] != "custom-value" {
		t.Errorf("ExtraHeaders[X-Custom] = %q, want custom-value", auth.ExtraHeaders["X-Custom"])
	}
	if len(auth.TrustedDomains) != 2 {
		t.Fatalf("TrustedDomains len = %d, want 2", len(auth.TrustedDomains))
	}
	if auth.TrustedDomains[0] != "api.example.com" {
		t.Errorf("TrustedDomains[0] = %q, want api.example.com", auth.TrustedDomains[0])
	}
}

func TestRegisterPluginAuthFromHeadersNoAuth(t *testing.T) {
	srv := market.ServerDescriptor{
		Key:      "no-auth-srv",
		Endpoint: "https://api.example.com/mcp/v1",
		CLI:      market.CLIOverlay{ID: "no-auth-srv"},
		AuthHeaders: map[string]string{
			"X-Custom": "custom-value",
		},
	}

	registerPluginAuthFromHeaders(srv)

	// Should not register because there's no Authorization header
	if _, ok := LookupPluginAuth("no-auth-srv"); ok {
		t.Error("expected no auth registration when Authorization header is missing")
	}
}

func TestBuildPluginAuthClient(t *testing.T) {
	base := transport.NewClient(nil)

	srv := market.ServerDescriptor{
		Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1/mcp",
		AuthHeaders: map[string]string{
			"Authorization": "Bearer sk-test-api-key",
			"X-Extra":       "extra-value",
		},
	}

	client := buildPluginAuthClient(base, srv)

	// Should return a different client instance
	if client == base {
		t.Error("expected buildPluginAuthClient to return a new client, not the base")
	}

	// Verify trusted domains
	if len(client.TrustedDomains) != 2 {
		t.Fatalf("TrustedDomains len = %d, want 2", len(client.TrustedDomains))
	}
	if client.TrustedDomains[0] != "dashscope.aliyuncs.com" {
		t.Errorf("TrustedDomains[0] = %q, want dashscope.aliyuncs.com", client.TrustedDomains[0])
	}
}

func TestBuildPluginAuthClientNoAuth(t *testing.T) {
	base := transport.NewClient(nil)

	srv := market.ServerDescriptor{
		Endpoint: "https://api.example.com/mcp/v1",
		AuthHeaders: map[string]string{
			"X-Custom": "custom-value",
		},
	}

	client := buildPluginAuthClient(base, srv)

	// Should return the base client when no Authorization header
	if client != base {
		t.Error("expected buildPluginAuthClient to return base client when no Authorization header")
	}
}
