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

package a2a

import (
	"context"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// TestResolveAccessToken_ExplicitPassthrough verifies that a non-empty
// explicitToken wins without touching any on-disk config.
func TestResolveAccessToken_ExplicitPassthrough(t *testing.T) {
	tok, err := ResolveAccessToken(context.Background(), "/non/existent/dir", "  bearer-xyz  ")
	if err != nil {
		t.Fatalf("ResolveAccessToken(explicit) error = %v", err)
	}
	if tok != "bearer-xyz" {
		t.Fatalf("ResolveAccessToken(explicit) = %q, want %q", tok, "bearer-xyz")
	}
}

// TestIdentityHeaders_ContainsRequiredKeys verifies IdentityHeaders returns
// a non-nil map. Deeper header content (x-dws-agent-id / x-dingtalk-source)
// depends on ~/.dws/identity.json which may not be writable in every CI
// environment; when the resolver returns an empty map we log-and-skip so
// the test is hermetic everywhere.
func TestIdentityHeaders_ContainsRequiredKeys(t *testing.T) {
	headers := IdentityHeaders()
	if headers == nil {
		t.Fatal("IdentityHeaders returned nil map")
	}
	if len(headers) == 0 {
		t.Skip("identity resolver returned no headers in this environment; env lacks ~/.dws/identity.json")
	}
	// When identity is resolvable, at least one of these keys MUST appear.
	for _, k := range []string{"x-dws-agent-id", "x-dingtalk-source"} {
		if _, ok := headers[k]; ok {
			return
		}
	}
	t.Fatalf("IdentityHeaders missing x-dws-agent-id and x-dingtalk-source; got keys: %v", keysOf(headers))
}

// TestHostOwnsPATFlow_FromAgentCode verifies the a2a facade mirrors the
// canonical rule: host-owned PAT mode is keyed ONLY off
// DINGTALK_DWS_AGENTCODE. No claw-type / DINGTALK_AGENT derivation exists
// in the a2a surface anymore (open-source `claw-type` is hard-wired to
// `openClaw` by the edition hook; see pkg/edition/default.go).
func TestHostOwnsPATFlow_FromAgentCode(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "agt-cursor")
	if !HostOwnsPATFlow() {
		t.Fatalf("HostOwnsPATFlow() = false after DINGTALK_DWS_AGENTCODE=agt-cursor, want true")
	}
	t.Setenv(authpkg.AgentCodeEnv, "")
	if HostOwnsPATFlow() {
		t.Fatalf("HostOwnsPATFlow() = true with no DINGTALK_DWS_AGENTCODE, want false")
	}
}

// TestRegisterLookupPluginAuth_Roundtrip verifies registry Register and
// Lookup agree on the same product id keyspace.
func TestRegisterLookupPluginAuth_Roundtrip(t *testing.T) {
	const productID = "a2a_test_product_roundtrip"
	want := &PluginAuth{
		Token:          "bearer-abc",
		ExtraHeaders:   map[string]string{"x-plugin-ver": "1"},
		TrustedDomains: []string{"example.test"},
	}
	Register(productID, want)

	got, ok := Lookup(productID)
	if !ok {
		t.Fatal("Lookup returned ok=false after Register")
	}
	if got != want {
		t.Fatalf("Lookup returned %p, want %p (same pointer via alias)", got, want)
	}

	_, ok = Lookup("never-registered-product")
	if ok {
		t.Fatal("Lookup returned ok=true for unregistered product id")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
