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

package edition

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
)

func TestClawTypeDefaultsToOSSValue(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "")
	prev := Get()
	defer Override(prev)

	Override(defaultHooks())
	if got := ClawType(); got != DefaultOSSClawType {
		t.Fatalf("ClawType() = %q, want %q", got, DefaultOSSClawType)
	}
}

func TestClawTypeUsesOverlayValue(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "")
	prev := Get()
	defer Override(prev)

	Override(&Hooks{Name: "overlay", ClawTypeValue: "wukong"})
	if got := ClawType(); got != "wukong" {
		t.Fatalf("ClawType() = %q, want overlay value %q", got, "wukong")
	}
}

func TestClawTypeUsesValidAgentProduct(t *testing.T) {
	t.Setenv(agentproduct.EnvName, " qwenwork ")
	prev := Get()
	defer Override(prev)

	Override(&Hooks{Name: "overlay", ClawTypeValue: "wukong"})
	if got := ClawType(); got != "qwenwork" {
		t.Fatalf("ClawType() = %q, want qwenwork", got)
	}
}

func TestClawTypeInvalidAgentProductFallsBackToOverlay(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "qwen work")
	prev := Get()
	defer Override(prev)

	Override(&Hooks{Name: "overlay", ClawTypeValue: "wukong"})
	if got := ClawType(); got != "wukong" {
		t.Fatalf("ClawType() = %q, want overlay fallback wukong", got)
	}
}

func TestOpenStaticServersIncludesCoreProducts(t *testing.T) {
	servers := openStaticServers()
	byID := make(map[string]ServerInfo, len(servers))
	for _, server := range servers {
		byID[server.ID] = server
	}

	required := []string{"aitable", "aitable-helper", "calendar", "todo", "doc", "chat", "mail", "oa"}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Errorf("openStaticServers() missing required product %q", id)
		}
	}

	helper := byID["aitable-helper"]
	if helper.Endpoint == "" {
		t.Fatal("aitable-helper has empty endpoint")
	}
}

func TestOpenVisibleProductsExcludesCompatibilityOnlyCommands(t *testing.T) {
	visible := openVisibleProducts()
	byID := make(map[string]bool, len(visible))
	for _, id := range visible {
		byID[id] = true
	}
	if byID["conference"] {
		t.Fatal("conference must remain hidden compatibility-only and not appear in VisibleProducts")
	}

	for _, server := range openStaticServers() {
		if server.ID == "conference" {
			t.Fatal("conference must remain compatibility-only and not be added to StaticServers")
		}
	}
	if byID["mcp-meta"] {
		t.Fatal("mcp-meta is helper-only and must not appear in VisibleProducts")
	}
	if byID["drive-internal"] {
		t.Fatal("drive-internal is helper-only and must not appear in VisibleProducts")
	}
	if byID["dingtalk-file"] {
		t.Fatal("dingtalk-file is helper-only and must not appear in VisibleProducts")
	}
}

func TestOpenSupplementServersIncludesMCPMeta(t *testing.T) {
	servers := openSupplementServers()
	foundMCPMeta := false
	foundWhiteboard := false
	foundRecruit := false
	foundDriveInternal := false
	foundDingTalkFile := false
	for _, server := range servers {
		if server.ID == "recruit" {
			foundRecruit = server.Endpoint == "https://mcp-gw.dingtalk.com/server/f69b54ada16c57b603c0e5e1c36f464ba73dcee28d64bb701ff2682c259c0cff" &&
				len(server.Prefixes) == 2 && server.Prefixes[0] == "recruit" && server.Prefixes[1] == "job"
		}
		if server.ID == "whiteboard" {
			foundWhiteboard = server.Endpoint == "https://mcp-gw.dingtalk.com/server/whiteboard"
		}
		if server.ID == "drive-internal" {
			foundDriveInternal = true
			if server.Endpoint != "https://mcp-gw.dingtalk.com/server/e48ff8134b3e4ff6fe3a9cbae8b440869083f0213bd8879c91b080e703162e02" {
				t.Fatalf("drive-internal endpoint = %q, want the registered internal capability endpoint", server.Endpoint)
			}
			if len(server.Prefixes) != 0 {
				t.Fatal("drive-internal must remain helper-only without command prefixes")
			}
		}
		if server.ID == "dingtalk-file" {
			foundDingTalkFile = true
			if server.Endpoint != "https://mcp-gw.dingtalk.com/server/d48b09ddafc89bf921b777ff428f8fc88b14805ccdd9680e02b7be318e7ed4b4" {
				t.Fatalf("dingtalk-file endpoint = %q, want the registered file service endpoint", server.Endpoint)
			}
			if len(server.Prefixes) != 0 {
				t.Fatal("dingtalk-file must remain helper-only without command prefixes")
			}
		}
		if server.ID != "mcp-meta" {
			continue
		}
		foundMCPMeta = true
		if server.Endpoint == "" {
			t.Fatal("mcp-meta has empty endpoint")
		}
		if len(server.Prefixes) != 0 {
			t.Fatal("mcp-meta must remain helper-only without command prefixes")
		}
	}
	if !foundMCPMeta {
		t.Fatal("openSupplementServers() missing mcp-meta")
	}
	if !foundWhiteboard {
		t.Fatal("openSupplementServers() missing helper-only whiteboard endpoint")
	}
	if !foundRecruit {
		t.Fatal("openSupplementServers() missing explicitly wired recruit endpoint")
	}
	if !foundDriveInternal {
		t.Fatal("openSupplementServers() missing helper-only drive-internal endpoint")
	}
	if !foundDingTalkFile {
		t.Fatal("openSupplementServers() missing helper-only dingtalk-file endpoint")
	}
}

func TestCrossPlatformCoverageOpenSupplementServersExcludesRetiredEduEndpoints(t *testing.T) {
	retiredProducts := map[string]bool{
		"edu-contact":     true,
		"edu-group":       true,
		"edu-app":         true,
		"edu-familygroup": true,
		"college-contact": true,
	}

	for _, server := range openSupplementServers() {
		if retiredProducts[server.ID] {
			t.Errorf("openSupplementServers() still exposes retired endpoint %q", server.ID)
		}
		for _, prefix := range server.Prefixes {
			if retiredProducts[prefix] {
				t.Errorf("openSupplementServers() endpoint %q still routes retired prefix %q", server.ID, prefix)
			}
		}
	}
}
